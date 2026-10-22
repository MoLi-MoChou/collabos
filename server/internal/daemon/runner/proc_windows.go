//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"unsafe"

	"golang.org/x/sys/windows"
)

func setProcessGroup(cmd *exec.Cmd) {}

// killProcessGroup kills the process tree with taskkill /T.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
}

var (
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	ntSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	ntResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

// signalGroup suspends (stop) or resumes the agent and all its descendants,
// the Windows equivalent of SIGSTOP/SIGCONT on the process group.
func signalGroup(cmd *exec.Cmd, stop bool) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	proc := ntResumeProcess
	if stop {
		proc = ntSuspendProcess
	}
	if proc.Find() != nil {
		return false
	}
	ok := false
	for _, pid := range processTree(uint32(cmd.Process.Pid)) {
		h, err := windows.OpenProcess(windows.PROCESS_SUSPEND_RESUME, false, pid)
		if err != nil {
			continue
		}
		if r, _, _ := proc.Call(uintptr(h)); r == 0 {
			ok = true
		}
		_ = windows.CloseHandle(h)
	}
	return ok
}

// processTree returns root and every live descendant pid.
func processTree(root uint32) []uint32 {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return []uint32{root}
	}
	defer windows.CloseHandle(snap)
	children := map[uint32][]uint32{}
	var e windows.ProcessEntry32
	e.Size = uint32(unsafe.Sizeof(e))
	for err = windows.Process32First(snap, &e); err == nil; err = windows.Process32Next(snap, &e) {
		if e.ProcessID != e.ParentProcessID {
			children[e.ParentProcessID] = append(children[e.ParentProcessID], e.ProcessID)
		}
	}
	out, seen := []uint32{root}, map[uint32]bool{root: true}
	for i := 0; i < len(out); i++ {
		for _, c := range children[out[i]] {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
		}
	}
	return out
}
