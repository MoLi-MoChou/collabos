//go:build windows

package runner

import (
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

type lockedBuf struct {
	mu sync.Mutex
	b  strings.Builder
}

func (l *lockedBuf) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *lockedBuf) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Len()
}

// Pause suspends the whole tree (cmd.exe -> ping), so output stops; resume restarts it.
func TestControlSuspendsProcessTreeWindows(t *testing.T) {
	out := &lockedBuf{}
	cmd := exec.Command("cmd.exe", "/c", "ping -n 60 127.0.0.1")
	cmd.Stdout = out
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer killProcessGroup(cmd)
	time.Sleep(2500 * time.Millisecond)
	c := &Control{}
	c.attach(cmd, nil)
	if !c.Pause() || !c.Paused() {
		t.Fatal("pause failed")
	}
	time.Sleep(300 * time.Millisecond)
	before := out.Len()
	time.Sleep(2500 * time.Millisecond)
	if out.Len() != before {
		t.Fatalf("output kept growing while paused: %d -> %d", before, out.Len())
	}
	if !c.Resume() || c.Paused() {
		t.Fatal("resume failed")
	}
	time.Sleep(2500 * time.Millisecond)
	if out.Len() == before {
		t.Fatal("no output after resume")
	}
}
