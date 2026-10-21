// Package sandbox is PEP layer 2 (PRD 7.2.2): it wraps the agent process in
// an OS sandbox so that commands the agent runs internally (which ACP never
// shows us) cannot write outside the task worktree or read hidden paths.
//
// Backends: bubblewrap on Linux, Seatbelt (sandbox-exec) on macOS, and none
// on Windows, where only self-delegation is allowed.
package sandbox

import (
	"context"
	"os"
	"os/exec"
	"runtime"

	"github.com/collabos/collabos/server/pkg/protocol"
)

// Spec describes the confinement for one agent process.
type Spec struct {
	Argv []string
	Dir  string
	Env  []string
	// Writable paths are bind-mounted read-write; everything else is read-only.
	Writable []string
	// ReadOnly paths are re-exposed read-only after Hidden paths are masked.
	ReadOnly []string
	// Hidden paths are masked (e.g. $HOME, other repos) before Writable/ReadOnly.
	Hidden []string
	// Network allows outbound network (the model API needs it).
	Network bool
}

// Backend builds sandboxed commands.
type Backend interface {
	Name() string
	Available() bool
	Command(ctx context.Context, s Spec) (*exec.Cmd, error)
}

// Detect returns the best backend for this OS.
func Detect() Backend {
	// COLLAB_SANDBOX forces a backend: "none" opts out (the daemon then only
	// accepts its owner's own tasks); "bwrap"/"seatbelt" require that backend.
	switch os.Getenv("COLLAB_SANDBOX") {
	case "none":
		return None{}
	case "bwrap", "bubblewrap":
		return Bubblewrap{}
	case "seatbelt":
		return Seatbelt{}
	}
	switch runtime.GOOS {
	case "linux":
		if b := (Bubblewrap{}); b.Available() {
			return b
		}
	case "darwin":
		if b := (Seatbelt{}); b.Available() {
			return b
		}
	}
	return None{}
}

// None runs the process unconfined; only acceptable for self-delegation.
type None struct{}

// Name implements Backend.
func (None) Name() string { return protocol.SandboxNone }

// Available implements Backend.
func (None) Available() bool { return true }

// Command implements Backend.
func (None) Command(ctx context.Context, s Spec) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, s.Argv[0], s.Argv[1:]...)
	cmd.Dir, cmd.Env = s.Dir, s.Env
	return cmd, nil
}
