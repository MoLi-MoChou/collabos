package agent

import (
	"os"
	"path/filepath"
	"testing"
)

// FR-DAE-102: only one daemon instance per machine (per COLLAB_HOME).
func TestSingleInstanceLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "daemon.lock")
	a, err := acquireLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := acquireLock(p); err == nil {
		t.Fatal("second instance acquired the lock")
	}
	releaseLock(a)
	b, err := acquireLock(p)
	if err != nil {
		t.Fatalf("lock not released: %v", err)
	}
	releaseLock(b)
}

// A lock file left behind by a killed daemon must not block the next start.
func TestStaleLockFileIsReclaimed(t *testing.T) {
	p := filepath.Join(t.TempDir(), "daemon.lock")
	if err := os.WriteFile(p, []byte("stale"), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := acquireLock(p)
	if err != nil {
		t.Fatalf("stale lock file blocked startup: %v", err)
	}
	releaseLock(f)
}
