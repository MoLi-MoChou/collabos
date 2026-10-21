//go:build !windows

package agent

import (
	"errors"
	"os"
	"syscall"
)

// acquireLock takes an exclusive flock so only one daemon runs per home (FR-DAE-102).
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, errors.New("another collab daemon is already running for this COLLAB_HOME")
	}
	return f, nil
}

func releaseLock(f *os.File) {
	if f != nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
}
