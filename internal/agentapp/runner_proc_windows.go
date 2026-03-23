//go:build windows

package agentapp

import (
	"os/exec"
	"strconv"
	"syscall"
)

func configureCmdProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
}

func killProcessTree(pid int) {
	if pid <= 0 {
		return
	}
	c := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(pid))
	_ = c.Run()
}
