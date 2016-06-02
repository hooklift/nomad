package allocdir

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/unix"

	"github.com/hashicorp/go-multierror"
)

const (
	// secretDirTmpfsSize is the size of the tmpfs per task in MBs
	secretDirTmpfsSize = 1
)

// Bind mounts the shared directory into the task directory. Must be root to
// run.
func (d *AllocDir) mountSharedDir(taskDir string) error {
	if err := os.MkdirAll(taskDir, 0777); err != nil {
		return err
	}

	return syscall.Mount(d.SharedDir, taskDir, "", syscall.MS_BIND, "")
}

func (d *AllocDir) unmountSharedDir(dir string) error {
	return syscall.Unmount(dir, 0)
}

// createSecretDir creates the secrets dir folder at the given path using a
// tmpfs
func (d *AllocDir) createSecretDir(dir string) error {
	// Only mount the tmpfs if we are root
	if unix.Geteuid() == 0 {
		if err := os.MkdirAll(dir, 0777); err != nil {
			return err
		}

		var flags uintptr
		flags = syscall.MS_NOEXEC
		options := fmt.Sprintf("size=%dm", secretDirTmpfsSize)
		err := syscall.Mount("tmpfs", dir, "tmpfs", flags, options)
		return os.NewSyscallError("mount", err)
	}

	return os.MkdirAll(dir, 0777)
}

// createSecretDir removes the secrets dir folder
func (d *AllocDir) removeSecretDir(dir string) error {
	if unix.Geteuid() == 0 {
		if err := syscall.Unmount(dir, 0); err != nil {
			return os.NewSyscallError("unmount", err)
		}
	}

	return os.RemoveAll(dir)
}

// MountSpecialDirs mounts the dev and proc file system from the host to the
// chroot
func (d *AllocDir) MountSpecialDirs(taskDir string) error {
	// Mount dev
	dev := filepath.Join(taskDir, "dev")
	if !d.pathExists(dev) {
		if err := os.MkdirAll(dev, 0777); err != nil {
			return fmt.Errorf("Mkdir(%v) failed: %v", dev, err)
		}

		if err := syscall.Mount("none", dev, "devtmpfs", syscall.MS_RDONLY, ""); err != nil {
			return fmt.Errorf("Couldn't mount /dev to %v: %v", dev, err)
		}
	}

	// Mount proc
	proc := filepath.Join(taskDir, "proc")
	if !d.pathExists(proc) {
		if err := os.MkdirAll(proc, 0777); err != nil {
			return fmt.Errorf("Mkdir(%v) failed: %v", proc, err)
		}

		if err := syscall.Mount("none", proc, "proc", syscall.MS_RDONLY, ""); err != nil {
			return fmt.Errorf("Couldn't mount /proc to %v: %v", proc, err)
		}
	}

	return nil
}

// unmountSpecialDirs unmounts the dev and proc file system from the chroot
func (d *AllocDir) UnmountSpecialDirs(taskDir string) error {
	dirs := []string{"dev", "proc"}
	errs := new(multierror.Error)

	for _, dir := range dirs {
		dirPath := filepath.Join(taskDir, dir)
		if !d.pathExists(dirPath) {
			continue
		}

		if err := syscall.Unmount(dirPath, 0); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("Failed to unmount %q (%v): %v", dir, dirPath, err))
		} else if err := os.RemoveAll(dirPath); err != nil {
			errs = multierror.Append(errs, fmt.Errorf("Failed to delete %q directory (%v): %v", dir, dirPath, err))
		}
	}

	return errs.ErrorOrNil()
}
