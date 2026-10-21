//go:build windows

package agent

import (
	"errors"
	"os"
)

// acquireLock uses an exclusive create as a best-effort single-instance lock.
func acquireLock(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return nil, errors.New("another collab daemon is already running (remove " + path + " if stale)")
	}
	return f, nil
}

func releaseLock(f *os.File) {
	if f != nil {
		name := f.Name()
		_ = f.Close()
		_ = os.Remove(name)
	}
}
