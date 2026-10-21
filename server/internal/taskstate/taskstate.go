// Package taskstate implements the unified task/delegation state machine
// (PRD 6.2). running_offline is a sub-state of working.
package taskstate

import (
	"fmt"

	p "github.com/collabos/collabos/server/pkg/protocol"
)

// S is a task status.
type S = p.TaskStatus

var transitions = map[S][]S{
	p.StatusSubmitted:      {p.StatusPendingIntake, p.StatusAccepted, p.StatusRejected, p.StatusCanceled},
	p.StatusPendingIntake:  {p.StatusAccepted, p.StatusRejected, p.StatusCanceled},
	p.StatusAccepted:       {p.StatusQueued, p.StatusWorking, p.StatusCanceled},
	p.StatusQueued:         {p.StatusWorking, p.StatusCanceled},
	p.StatusWorking:        {p.StatusRunningOffline, p.StatusInputRequired, p.StatusApproval, p.StatusCompleted, p.StatusFailed, p.StatusCanceled},
	p.StatusRunningOffline: {p.StatusWorking, p.StatusUnknown, p.StatusApproval, p.StatusCompleted, p.StatusFailed, p.StatusCanceled},
	p.StatusInputRequired:  {p.StatusWorking, p.StatusCanceled},
	// approval_required covers both in-flight permission requests and the
	// final merge gate, which ends in completed.
	p.StatusApproval: {p.StatusWorking, p.StatusRunningOffline, p.StatusCompleted, p.StatusFailed, p.StatusCanceled},
	p.StatusUnknown:  {p.StatusWorking, p.StatusApproval, p.StatusCompleted, p.StatusFailed},
}

// IsTerminal reports whether s is a final state.
func IsTerminal(s S) bool {
	switch s {
	case p.StatusCompleted, p.StatusFailed, p.StatusCanceled, p.StatusRejected:
		return true
	}
	return false
}

// CanTransition reports whether from -> to is allowed.
func CanTransition(from, to S) bool {
	for _, t := range transitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Transition validates from -> to; staying put is always allowed.
func Transition(from, to S) error {
	if from == to {
		return nil
	}
	if !CanTransition(from, to) {
		return fmt.Errorf("taskstate: illegal transition %s -> %s", from, to)
	}
	return nil
}

// FromOutcome maps a daemon outcome to a status.
func FromOutcome(o p.TaskOutcome) S {
	switch o {
	case p.OutcomeCompleted:
		return p.StatusCompleted
	case p.OutcomeCanceled:
		return p.StatusCanceled
	default:
		return p.StatusFailed
	}
}
