//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

// ConfigureForGroup configures a command to run in a dedicated process group.
func ConfigureForGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// KillTree terminates a process group and the main process.
func KillTree(pid int) error {
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil {
		_ = syscall.Kill(pid, syscall.SIGKILL)
	}
	return nil
}
