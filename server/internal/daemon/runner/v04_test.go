package runner

import (
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/collabos/collabos/server/pkg/protocol"
)

func TestCommitMessageAndPRBodyCarryTrace(t *testing.T) {
	d := protocol.DelegationDispatch{TaskID: "t1", TraceID: "tr-9", Title: "fix"}
	m := CommitMessage(d)
	for _, want := range []string{"CollabOS-Task: t1", "Delegation-Id: t1", "Trace-Id: tr-9"} {
		if !strings.Contains(m, want) {
			t.Fatalf("commit message lacks %q: %s", want, m)
		}
	}
	b := PRBody(d, protocol.DaemonTaskCompleted{Summary: "did it", Tests: &protocol.TestResult{Command: "go test", Passed: false}})
	if !strings.Contains(b, "did it") || !strings.Contains(b, "FAILED") || !strings.Contains(b, "Trace-Id: `tr-9`") {
		t.Fatalf("pr body: %s", b)
	}
}

func TestLimitArgv(t *testing.T) {
	argv := []string{"sh", "-c", "x"}
	if got := limitArgv(Limits{}, argv); len(got) != 3 {
		t.Fatalf("no limits must not wrap: %v", got)
	}
	if _, err := exec.LookPath("prlimit"); err != nil {
		t.Skip("prlimit not installed")
	}
	got := strings.Join(limitArgv(Limits{CPUSeconds: 5, MemoryMB: 64, MaxProcs: 10}, argv), " ")
	for _, want := range []string{"--cpu=5", "--as=67108864", "--nproc=10", "-- sh -c x"} {
		if !strings.Contains(got, want) {
			t.Fatalf("%q lacks %q", got, want)
		}
	}
}

// Pause/Resume stop and continue the real process group (FR-CLI-402 / budget pause).
func TestControlPausesProcessGroup(t *testing.T) {
	cmd := exec.Command("sh", "-c", "sleep 30")
	setProcessGroup(cmd)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	c := &Control{}
	c.attach(cmd, nil)
	if !c.Pause() || !c.Paused() {
		t.Fatal("pause failed")
	}
	time.Sleep(50 * time.Millisecond)
	if st := procState(t, cmd.Process.Pid); st != "T" {
		t.Fatalf("state after pause = %q", st)
	}
	if !c.Resume() || c.Paused() {
		t.Fatal("resume failed")
	}
	time.Sleep(50 * time.Millisecond)
	if st := procState(t, cmd.Process.Pid); st == "T" {
		t.Fatal("still stopped after resume")
	}
}

func procState(t *testing.T, pid int) string {
	out, err := exec.Command("ps", "-o", "stat=", "-p", itoa(pid)).Output()
	if err != nil {
		t.Skip("ps unavailable")
	}
	return strings.TrimSpace(string(out))[:1]
}

func itoa(n int) string { return strconv.Itoa(n) }
