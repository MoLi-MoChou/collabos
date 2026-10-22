//go:build windows

package agent

import (
	"errors"
	"os"

	"golang.org/x/sys/windows"
)

// acquireLock takes an exclusive, non-blocking LockFileEx lock so only one
// daemon runs per home (FR-DAE-102). The OS drops the lock when the process
// exits, so a crashed or killed daemon never leaves a stale lock behind.
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	ol := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, ol); err != nil {
		_ = f.Close()
		return nil, errors.New("another collab daemon is already running for this COLLAB_HOME")
	}
	return f, nil
}

func releaseLock(f *os.File) {
	if f != nil {
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, new(windows.Overlapped))
		_ = f.Close()
	}
}
