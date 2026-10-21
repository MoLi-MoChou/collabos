//go:build windows

package runner

import (
	"os/exec"
	"strconv"
)

func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the process tree with taskkill /T.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

// signalGroup is not supported on Windows: pause/resume reports false and the
// server keeps the task paused at the decision layer only.
func signalGroup(cmd *exec.Cmd, stop bool) bool { return false }
