//+build linux
package driver

import (
	"encoding/json"
	"fmt"
	"log"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/nomad/client/allocdir"
	"github.com/hashicorp/nomad/client/config"
	"github.com/hashicorp/nomad/client/driver/executor"
	cstructs "github.com/hashicorp/nomad/client/driver/structs"
	"github.com/hashicorp/nomad/client/fingerprint"
	"github.com/hashicorp/nomad/helper/discover"
	"github.com/hashicorp/nomad/helper/fields"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/mitchellh/mapstructure"
)

const (
	// The key populated in Node Attributes to indicate the presence of the Hooklift
	// driver
	hookliftDriverAttr = "driver.hooklift"
)

type HookliftDriverRootFS struct {
	URL         string `mapstructure:"url"`
	Destination string `mapstructure:"destination"`
}

type HookliftDriverConfig struct {
	Command string               `mapstructure:"command"`
	Args    []string             `mapstructure:"args"`
	RootFS  HookliftDriverRootFS `mapstructure:"rootfs"`
	Options []string             `mapstructure:"options"`
}

// hookliftPID is a struct to map the pid running the process to systemd-nspawn process.
type hookliftPID struct {
	PluginConfig   *PluginReattachConfig
	AllocDir       *allocdir.AllocDir
	ExecutorPid    int
	KillTimeout    time.Duration
	MaxKillTimeout time.Duration
}

// hookliftHandle is returned from Start/Open as a handle to the running task.
type hookliftHandle struct {
	pluginClient    *plugin.Client
	executorPid     int
	isolationConfig *cstructs.IsolationConfig
	executor        executor.Executor
	allocDir        *allocdir.AllocDir
	logger          *log.Logger
	killTimeout     time.Duration
	maxKillTimeout  time.Duration
	waitCh          chan *cstructs.WaitResult
	doneCh          chan struct{}
}

func (h *hookliftHandle) run() {
	// Wait for Nomad executor process to finish launching task.
	ps, err := h.executor.Wait()
	close(h.doneCh)

	if ps.ExitCode == 0 && err != nil {
		if e := killProcess(h.executorPid); e != nil {
			h.logger.Printf("[ERROR] driver.hooklift: error killing user process: %v", e)
		}

		if e := h.allocDir.UnmountAll(); e != nil {
			h.logger.Printf("[ERROR] driver.hooklift: unmounting dev, proc and alloc dirs failed: %v", e)
		}
	}

	// Wait for the task to exit.
	h.waitCh <- cstructs.NewWaitResult(ps.ExitCode, 0, err)
	close(h.waitCh)

	// Task exitted, removing services.
	if err := h.executor.DeregisterServices(); err != nil {
		h.logger.Printf("[ERROR] driver.hooklift: failed to deregister services: %v", err)
	}

	// Exitting executor process.
	if err := h.executor.Exit(); err != nil {
		h.logger.Printf("[ERROR] driver.hooklift: error killing executor: %v", err)
	}
	h.pluginClient.Kill()
}

// ID returns an opaque handle that can be used to re-open the handle.
func (h *hookliftHandle) ID() string {
	// Return a handle to the PID
	pid := &hookliftPID{
		PluginConfig:   NewPluginReattachConfig(h.pluginClient.ReattachConfig()),
		KillTimeout:    h.killTimeout,
		MaxKillTimeout: h.maxKillTimeout,
		ExecutorPid:    h.executorPid,
		AllocDir:       h.allocDir,
	}
	data, err := json.Marshal(pid)
	if err != nil {
		h.logger.Printf("[ERROR] driver.hooklift: failed to marshal hooklift PID to JSON: %s", err)
	}
	return fmt.Sprintf("Hooklift:%s", string(data))
}

// WaitCh is used to return a channel used wait for task completion.
func (h *hookliftHandle) WaitCh() chan *cstructs.WaitResult {
	return h.waitCh
}

// Update is used to update the task if possible and update task related
// configurations.
func (h *hookliftHandle) Update(task *structs.Task) error {
	// Store the updated kill timeout.
	h.killTimeout = GetKillTimeout(task.KillTimeout, h.maxKillTimeout)
	h.executor.UpdateTask(task)

	// Update is not possible
	return nil
}

// Kill is used to stop the task.
func (h *hookliftHandle) Kill() error {
	h.executor.ShutDown()
	select {
	case <-h.doneCh:
		return nil
	case <-time.After(h.killTimeout):
		return h.executor.Exit()
	}
}

// Stats returns aggregated stats of the driver.
func (h *hookliftHandle) Stats() (*cstructs.TaskResourceUsage, error) {
	return h.executor.Stats()
}

// HookliftDriver uses systemd-nspawn to run tasks using a global rootfs. The rootfs is downloaded and placed
// directly on the host. It is then mounted read-only as the base image for tasks running using this driver.
type HookliftDriver struct {
	DriverContext
	fingerprint.StaticFingerprinter
}

// NewHookliftDriver is used to create a new systemd-nspawn driver.
func NewHookliftDriver(ctx *DriverContext) Driver {
	return &HookliftDriver{DriverContext: *ctx}
}

func (d *HookliftDriver) Start(ctx *ExecContext, task *structs.Task) (DriverHandle, error) {
	var driverConfig HookliftDriverConfig
	if err := mapstructure.WeakDecode(task.Config, &driverConfig); err != nil {
		return nil, err
	}

	// Get the command to be ran
	command := driverConfig.Command
	if err := validateCommand(command, "args"); err != nil {
		return nil, err
	}

	// Get the task directory for storing the executor logs.
	taskDir, ok := ctx.AllocDir.TaskDirs[task.Name]
	if !ok {
		return nil, fmt.Errorf("Could not find task directory for task: %q", task.Name)
	}

	// Appends env variables defined using the job's env block
	for k, v := range d.taskEnv.EnvMap() {
		driverConfig.Options = append(driverConfig.Options, fmt.Sprintf("--setenv=%v=%v", k, v))
	}

	bin, err := discover.NomadExecutable()
	if err != nil {
		return nil, fmt.Errorf("unable to find the nomad binary: %v", err)
	}

	pluginLogFile := filepath.Join(taskDir, fmt.Sprintf("%s-executor.out", task.Name))
	pluginConfig := &plugin.ClientConfig{
		Cmd: exec.Command(bin, "executor", pluginLogFile),
	}

	execImpl, pluginClient, err := createExecutor(pluginConfig, d.config.LogOutput, d.config)
	if err != nil {
		return nil, err
	}
	executorCtx := &executor.ExecutorContext{
		TaskEnv:  d.taskEnv,
		Driver:   "hooklift",
		AllocDir: ctx.AllocDir,
		AllocID:  ctx.AllocID,
		Task:     task,
	}

	absPath, err := GetAbsolutePath("systemd-nspawn")
	if err != nil {
		return nil, err
	}

	args := driverConfig.Options
	args = append(args, driverConfig.Command)
	args = append(args, driverConfig.Args...)
	ps, err := execImpl.LaunchCmd(&executor.ExecCommand{
		Cmd:            absPath,
		Args:           args,
		FSIsolation:    false,
		ResourceLimits: true,
		User:           task.User,
	}, executorCtx)

	if err != nil {
		pluginClient.Kill()
		return nil, err
	}

	maxKill := d.DriverContext.config.MaxKillTimeout
	h := &hookliftHandle{
		pluginClient:    pluginClient,
		executor:        execImpl,
		executorPid:     ps.Pid,
		isolationConfig: ps.IsolationConfig,
		allocDir:        ctx.AllocDir,
		logger:          d.logger,
		killTimeout:     GetKillTimeout(task.KillTimeout, maxKill),
		maxKillTimeout:  maxKill,
		doneCh:          make(chan struct{}),
		waitCh:          make(chan *cstructs.WaitResult, 1),
	}
	if h.executor.SyncServices(consulContext(d.config, "")); err != nil {
		h.logger.Printf("[ERROR] driver.hooklift: error registering services for task: %q: %v", task.Name, err)
	}
	go h.run()
	return h, nil
}

func (d *HookliftDriver) Open(ctx *ExecContext, handleID string) (DriverHandle, error) {
	// Parse the handle
	pidBytes := []byte(strings.TrimPrefix(handleID, "Hooklift:"))
	id := &hookliftPID{}
	if err := json.Unmarshal(pidBytes, id); err != nil {
		return nil, fmt.Errorf("failed to parse Hooklift handle '%s': %v", handleID, err)
	}

	pluginConfig := &plugin.ClientConfig{
		Reattach: id.PluginConfig.PluginConfig(),
	}
	exec, pluginClient, err := createExecutor(pluginConfig, d.config.LogOutput, d.config)
	if err != nil {
		d.logger.Println("[ERROR] driver.hooklift: error connecting to plugin so destroying plugin pid and user pid")
		if e := destroyPlugin(id.PluginConfig.Pid, id.ExecutorPid); e != nil {
			d.logger.Printf("[ERROR] driver.hooklift: error destroying plugin and executor pid: %v", e)
		}
		return nil, fmt.Errorf("error connecting to plugin: %v", err)
	}

	ver, _ := exec.Version()
	d.logger.Printf("[DEBUG] driver.hooklift: version of executor: %v", ver.Version)

	// Return a driver handle
	h := &hookliftHandle{
		pluginClient:   pluginClient,
		executorPid:    id.ExecutorPid,
		allocDir:       id.AllocDir,
		executor:       exec,
		logger:         d.logger,
		killTimeout:    id.KillTimeout,
		maxKillTimeout: id.MaxKillTimeout,
		doneCh:         make(chan struct{}),
		waitCh:         make(chan *cstructs.WaitResult, 1),
	}
	if h.executor.SyncServices(consulContext(d.config, "")); err != nil {
		h.logger.Printf("[ERROR] driver.hooklift: error registering services: %v", err)
	}
	go h.run()
	return h, nil
}

func (d *HookliftDriver) Validate(config map[string]interface{}) error {
	fd := &fields.FieldData{
		Raw: config,
		Schema: map[string]*fields.FieldSchema{
			"command": &fields.FieldSchema{
				Type:     fields.TypeString,
				Required: true,
			},
			"rootfs": &fields.FieldSchema{
				Type: fields.TypeArray,
			},
			"args": &fields.FieldSchema{
				Type: fields.TypeArray,
			},
			"options": &fields.FieldSchema{
				Type: fields.TypeArray,
			},
		},
	}

	if err := fd.Validate(); err != nil {
		return err
	}
	return nil
}

// Fingerprint test whether a systemd-nspawn is found on the host.
func (d *HookliftDriver) Fingerprint(config *config.Config, node *structs.Node) (bool, error) {
	// Get the current status so that we can log any debug messages only if the
	// state changes
	_, currentlyEnabled := node.Attributes[hookliftDriverAttr]

	// Only enable if we are root when running on non-windows systems.
	if runtime.GOOS != "windows" && syscall.Geteuid() != 0 {
		if currentlyEnabled {
			d.logger.Printf("[DEBUG] driver.hooklift: must run as root user, disabling")
		}
		delete(node.Attributes, hookliftDriverAttr)
		return false, nil
	}

	outBytes, err := exec.Command("systemd-nspawn", "--version").Output()
	if err != nil {
		d.logger.Printf("[DEBUG] driver.hooklift: error %v", err)
		delete(node.Attributes, hookliftDriverAttr)
		return false, nil
	}

	d.logger.Printf("[INFO] driver.hooklift: systemd-nspawn version %q found", string(outBytes[:]))

	node.Attributes[hookliftDriverAttr] = "1"
	node.Attributes["driver.hooklift.version"] = fmt.Sprintf("systemd-nspawn %s", string(outBytes[:]))

	return true, nil
}
