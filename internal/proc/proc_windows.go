//go:build windows

package proc

import (
	"os/exec"
	"strconv"
	"syscall"
)

// ConfigureForGroup configures a command to run in a dedicated process group.
func ConfigureForGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

// KillTree terminates a process and all of its children on Windows.
func KillTree(pid int) error {
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(pid))
	return cmd.Run()
}
