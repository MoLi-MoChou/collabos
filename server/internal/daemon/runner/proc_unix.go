//go:build !windows

package runner

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// killProcessGroup kills the agent and everything it spawned.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}

// signalGroup stops (SIGSTOP) or continues (SIGCONT) the process group.
func signalGroup(cmd *exec.Cmd, stop bool) bool {
	if cmd.Process == nil {
		return false
	}
	sig := syscall.SIGCONT
	if stop {
		sig = syscall.SIGSTOP
	}
	return syscall.Kill(-cmd.Process.Pid, sig) == nil
}
