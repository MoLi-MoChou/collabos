package taskstate

import (
	"testing"

	p "github.com/collabos/collabos/server/pkg/protocol"
)

func TestTransitions(t *testing.T) {
	cases := []struct {
		from, to S
		ok       bool
	}{
		{p.StatusSubmitted, p.StatusPendingIntake, true},
		{p.StatusSubmitted, p.StatusAccepted, true},
		{p.StatusSubmitted, p.StatusRejected, true},
		{p.StatusPendingIntake, p.StatusAccepted, true},
		{p.StatusPendingIntake, p.StatusRejected, true},
		{p.StatusAccepted, p.StatusQueued, true},
		{p.StatusAccepted, p.StatusWorking, true},
		{p.StatusQueued, p.StatusWorking, true},
		{p.StatusWorking, p.StatusRunningOffline, true},
		{p.StatusRunningOffline, p.StatusWorking, true},
		{p.StatusRunningOffline, p.StatusUnknown, true},
		{p.StatusUnknown, p.StatusCompleted, true},
		{p.StatusWorking, p.StatusApproval, true},
		{p.StatusApproval, p.StatusWorking, true},
		{p.StatusApproval, p.StatusCompleted, true},
		{p.StatusWorking, p.StatusCompleted, true},
		{p.StatusCompleted, p.StatusWorking, false},
		{p.StatusRejected, p.StatusAccepted, false},
		{p.StatusFailed, p.StatusCompleted, false},
		{p.StatusCanceled, p.StatusWorking, false},
		{p.StatusSubmitted, p.StatusWorking, false},
		{p.StatusPendingIntake, p.StatusWorking, false},
		{p.StatusWorking, p.StatusUnknown, false},
		{p.StatusWorking, p.StatusWorking, true},
	}
	for _, c := range cases {
		err := Transition(c.from, c.to)
		if (err == nil) != c.ok {
			t.Errorf("%s -> %s: got err=%v, want ok=%v", c.from, c.to, err, c.ok)
		}
	}
}

func TestTerminalHasNoExits(t *testing.T) {
	for _, s := range []S{p.StatusCompleted, p.StatusFailed, p.StatusCanceled, p.StatusRejected} {
		if !IsTerminal(s) || len(transitions[s]) != 0 {
			t.Errorf("%s should be terminal without exits", s)
		}
	}
}

func TestFromOutcome(t *testing.T) {
	if FromOutcome(p.OutcomeCompleted) != p.StatusCompleted || FromOutcome("weird") != p.StatusFailed || FromOutcome(p.OutcomeCanceled) != p.StatusCanceled {
		t.Fatal("outcome mapping wrong")
	}
}
