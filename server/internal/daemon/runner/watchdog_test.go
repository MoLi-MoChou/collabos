package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/collabos/collabos/server/internal/daemon/sandbox"
	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/pkg/protocol"
)

func TestWatchdogFiresWhenIdle(t *testing.T) {
	w := newWatchdog(80*time.Millisecond, nil)
	ctx, cancel := context.WithCancel(context.Background())
	stop := w.start(cancel)
	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("watchdog did not fire")
	}
	if !stop() {
		t.Fatal("expected fired")
	}
}

func TestWatchdogTouchHoldAndPauseKeepAlive(t *testing.T) {
	var paused atomic.Bool
	w := newWatchdog(100*time.Millisecond, paused.Load)
	ctx, cancel := context.WithCancel(context.Background())
	stop := w.start(cancel)
	for i := 0; i < 5; i++ { // activity
		time.Sleep(40 * time.Millisecond)
		w.touch()
	}
	w.hold() // human decision pending
	time.Sleep(250 * time.Millisecond)
	w.release()
	paused.Store(true) // suspended by the owner
	time.Sleep(250 * time.Millisecond)
	paused.Store(false)
	w.touch()
	if ctx.Err() != nil || stop() {
		t.Fatal("watchdog fired despite activity / hold / pause")
	}
}

func TestWatchdogDisabled(t *testing.T) {
	w := newWatchdog(0, nil)
	stop := w.start(func() { t.Error("fired while disabled") })
	time.Sleep(30 * time.Millisecond)
	if stop() {
		t.Fatal("fired")
	}
}

func TestLastErrorInLogs(t *testing.T) {
	dir := t.TempDir()
	log := strings.Join([]string{
		`timestamp=x level=INFO message=stream session.id=ses_1`,
		`timestamp=x level=ERROR message="stream error" session.id=ses_2 error.error="other"`,
		`timestamp=x level=ERROR message="stream error" session.id=ses_1 error.error="AI_APICallError: Rate limit exceeded. Please try again later."`,
	}, "\n")
	if err := os.WriteFile(filepath.Join(dir, "a.log"), []byte(log), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := lastErrorInLogs(dir, "ses_1"); !strings.Contains(got, "Rate limit exceeded") {
		t.Fatalf("got %q", got)
	}
	if got := lastErrorInLogs(dir, "ses_9"); got != "" {
		t.Fatalf("got %q", got)
	}
}

// A stand-in agent process that completes the ACP handshake and then goes
// silent on session/prompt, like opencode stuck retrying a rate-limited model.
const silentAgent = `#!/usr/bin/env python3
import sys, json
for line in sys.stdin:
    m = json.loads(line)
    i, meth = m.get("id"), m.get("method")
    if meth == "initialize":
        r = {"protocolVersion": 1, "agentInfo": {"name": "silent", "version": "0"}}
    elif meth == "session/new":
        r = {"sessionId": "ses_silent"}
    else:
        if meth == "session/prompt":
            sys.stderr.write("ERROR model: Rate limit exceeded\n"); sys.stderr.flush()
        continue
    sys.stdout.write(json.dumps({"jsonrpc": "2.0", "id": i, "result": r}) + "\n"); sys.stdout.flush()
`

func TestRunFailsInactiveAgentWithReasonCode(t *testing.T) {
	if _, err := os.Stat("/usr/bin/python3"); err != nil {
		t.Skip("python3 required")
	}
	t.Setenv("HOME", t.TempDir())
	dir := t.TempDir()
	bin := filepath.Join(dir, "agent")
	if err := os.WriteFile(bin, []byte(silentAgent), 0o755); err != nil {
		t.Fatal(err)
	}
	var phases []string
	_ = phases
	start := time.Now()
	res := Run(context.Background(), Job{
		Task:              protocol.DelegationDispatch{TaskID: "t-idle", Kind: protocol.KindChat, Prompt: "hi"},
		Sandbox:           sandbox.None{},
		Opencode:          bin,
		WorkDir:           dir,
		InactivityTimeout: 300 * time.Millisecond,
		Hooks:             Hooks{Progress: func(p protocol.DaemonProgress) { phases = append(phases, p.Text) }},
	})
	if res.Outcome != protocol.OutcomeFailed || res.ReasonCode != delegation.CodeAgentInactive {
		t.Fatalf("outcome=%s code=%s err=%s", res.Outcome, res.ReasonCode, res.Error)
	}
	if !strings.Contains(res.Error, "Rate limit exceeded") {
		t.Fatalf("error does not surface agent error: %q", res.Error)
	}
	if time.Since(start) > 20*time.Second {
		t.Fatal("took too long")
	}
}
