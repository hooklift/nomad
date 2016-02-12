package simulator

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/nomad/jobspec"
	"github.com/hashicorp/nomad/nomad/structs"
	"github.com/hashicorp/nomad/scheduler"

	"github.com/hashicorp/nomad/command"
)

type SimCommand struct {
	command.Meta
}

func (c *SimCommand) Help() string {
	helpText := `
Usage: nomad simulator -nodeList <path> -jobList <path> [-outFile <path>]
  -nodeList <path>
    A path to a file containing a list of node specification file paths, one per line.
  -jobList <path>
    A path to a file containing a list of job specification file paths, one per line.
  [-outFile <path>]
    A path where the output JSON file will be created into. If not specified, default
    filename is simulator_output.json, at the location where binary was run.
    Example:
      $ nomad simulator -nodeList nodes.txt -jobList jobs.txt
`
	return strings.TrimSpace(helpText)
}

func (c *SimCommand) Synopsis() string {
	return "Run a simulator to get metrics about the placement of jobs"
}

func (c *SimCommand) Run(args []string) int {

	// A flag which will contain as a single string the path to a file containing nodes' configuration file paths.
	var nodeListPath string
	// Nodes' paths parsed from nodes' flag string.
	var nodesPaths []string
	// A flag which will contain as a single string the path to a file containing jobs' configuration file paths.
	var jobListPath string
	// Jobs' paths parsed from jobs' flag string.
	var jobsPaths []string
	// Path for the output file.
	var outFile string

	flags := flag.NewFlagSet("sim", flag.ContinueOnError)
	flags.Usage = func() { c.Ui.Error(c.Help()) }

	flags.StringVar(&nodeListPath, "nodeList", "", "nodeList")
	flags.StringVar(&jobListPath, "jobList", "", "jobList")
	flags.StringVar(&outFile, "outfile", "", "outfile")

	if err := flags.Parse(args); err != nil {
		return 1
	}

	// Retrieve the nodes' config file paths from the file given as argument.
	if nodeListPath != "" {
		var err error
		nodesPaths, err = readPaths(nodeListPath)
		noErr(err)
	} else {
		return 1
	}

	// Retrieve the jobs' config file paths from the file given as argument.
	if jobListPath != "" {
		var err error
		jobsPaths, err = readPaths(jobListPath)
		noErr(err)
	} else {
		return 1
	}

	if outFile == "" {
		outFile = "simulator_output.json"
	}

	// Stores the Nodes' pointers.
	var nodes []*structs.Node
	// Stores the the Jobs' pointers.
	var jobs []*structs.Job

	// Stores the node IDs, useful for retrieving Nodes from the harness internal state
	// and Allocations related to them.
	var nodeIDs []string

	// Load the Nodes' configuration files.
	for _, nodePath := range nodesPaths {
		current, err := LoadNodeFile(nodePath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error loading configuration from %s: %s", nodePath, err))
			return 1
		}
		nodes = append(nodes, current)
	}

	// Load the Jobs' configuration files.
	for _, jobPath := range jobsPaths {
		current, err := jobspec.ParseFile(jobPath)
		if err != nil {
			c.Ui.Error(fmt.Sprintf("Error parsing job file %s: %s", jobPath, err))
			return 1
		}
		jobs = append(jobs, current)
	}

	// Create a new SimHarness, in the same fashion that the Harness is
	// used for Unit Testing, since that is some sort of a simulation for
	// specific features, in our case, scheduling and placement simulation,
	// but with user inputted configurations instead of mocked values.
	h := NewSimHarness()
	// Initialize the mutex. This will lock Job scheduling operations.
	// Without this, and while using the harness logic, a problem arises in
	// which two Jobs are being concurrently allocated and they result in a
	// situation of resource over-consumption for a given Node.
	mutex := &sync.Mutex{}

	for _, node := range nodes {
		noErr(h.State.UpsertNode(h.NextIndex(), node))
		nodeIDs = append(nodeIDs, node.ID)
	}

	log.Println("Running Nomad simulator...")

	// Jobs will be processed iteratively in the order they are present at
	// input, but each one in a separate goroutine, so the real order in
	// which they finish processing is variable.
	// After each Job is processed, metrics will be extracted from
	// the present cluster state.
	// ...
	// This is a channel where a snapshot of the simulator will be sent for
	// metrics extraction after each job has processed.
	snapshotChan := make(chan *SimSnapshot)

	// For each job...
	for _, job := range jobs {
		// Run a separate goroutine...
		go func(job *structs.Job) {
			// Here's where starts what counts as 'processing' a Job, which comprises
			// adding it to the State, and then scheduling it. Start counting time from
			// this point in time, until it is done scheduling. The 'finishing' time is
			// the start time + the time it takes to the last Allocation to take place
			// (each Allocation has a time, so add whichever is the longest time).
			startTimestamp := int64(time.Now().UnixNano())

			// Upsert into the Harness Jobs parsed from configuration files.
			noErr(h.State.UpsertJob(h.NextIndex(), job))

			// Create mock Evaluations to register the Jobs.
			eval := &structs.Evaluation{
				ID:          structs.GenerateUUID(),
				Priority:    job.Priority,
				TriggeredBy: structs.EvalTriggerJobRegister,
				JobID:       job.ID,
			}

			// Lock the mutex. This will make safe the Job scheduling operations.
			// Without this, and while using the harness logic, a problem arises in
			// which two Jobs are being concurrently allocated and they result in a
			// situation of resource over-consumption for a given Node.
			mutex.Lock()

			// The usual rules apply for allocation:
			// Jobs must belong to same region of nodes, otherwise won't allocate.
			// Jobs must have a datacenter related to the one in the nodes, otherwise won't allocate.
			// Jobs' task drivers must be related to the ones in nodes, otherwise won't allocate.
			// ...
			// For specifically evaluating the bin packing, its better if all jobs and nodes belong
			// to a single datacenter, in the same region, with the same task drivers, so all nodes
			// are eligible for scheduling, and the bin packing can be seen more evidently.

			// Process the evaluation, depending on the type of scheduler
			// designated for the job. Modify this if a custom scheduler is
			// to be used.
			switch job.Type {
			case structs.JobTypeSystem:
				noErr(h.Process(scheduler.NewSystemScheduler, eval))
			case structs.JobTypeBatch:
				noErr(h.Process(scheduler.NewBatchScheduler, eval))
			case structs.JobTypeService:
				noErr(h.Process(scheduler.NewServiceScheduler, eval))
			}

			// Unlock the mutex after scheduling is done for a particular Job.
			mutex.Unlock()

			// A snapshot of the simulator after each Job has been processed contains the
			// ID to its Nodes, the plans, and the internal state.
			simSnapshot := &SimSnapshot{
				Eval:    eval,
				Plans:   h.Plans,
				State:   h.State,
				NodeIDs: nodeIDs,
				Time:    startTimestamp,
			}

			// Send the snapshot to a metrics evaluation loop so that metrics can be
			// extracted concurrently.
			snapshotChan <- simSnapshot
		}(job)
	}

	var outputNodes []*SimNode
	for _, nodeID := range nodeIDs {
		node, err := h.State.NodeByID(nodeID)
		noErr(err)
		outputNodes = append(outputNodes, parseNode(node))
	}

	var jobEvaluations []*JobEvaluationMetrics

	// This is the metrics evaluation loop.
	for iteration := 0; iteration < len(jobs); iteration++ {
		snapshot := <-snapshotChan

		jobEvaluationMetrics := getMetrics(snapshot)

		jobEvaluations = append(jobEvaluations, jobEvaluationMetrics)
	}

	// Structure to serialize the final simulator output.
	simOutput := &SimOutput{
		Nodes:          outputNodes,
		JobEvaluations: jobEvaluations,
	}

	marshald, _ := json.MarshalIndent(simOutput, "", "\t")
	err := ioutil.WriteFile(outFile, marshald, 0755)
	noErr(err)
	return 0
}

// readPaths reads a whole file containing paths to files as lines into memory
// and returns a slice of its lines, separaing the paths.
func readPaths(path string) ([]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var paths []string
	scanner := bufio.NewScanner(file)
	// Read the file, line by line, until EOF.
	for scanner.Scan() {
		path := scanner.Text()
		// Remove leading or trailing spaces or tabs from paths read.
		path = strings.Trim(path, " \t")
		// Ignore blank lines. If there were lines only containing spaces
		// or tab characters, those were trimmed in the last step.
		if path == "" {
			continue
		}
		paths = append(paths, path)
	}
	return paths, scanner.Err()
}
