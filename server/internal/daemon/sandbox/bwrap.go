package sandbox

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"

	"github.com/collabos/collabos/server/pkg/protocol"
)

// Bubblewrap confines with bwrap: the root filesystem is read-only, Hidden
// paths are replaced by empty tmpfs, Writable paths are bound read-write, and
// PID/IPC/UTS namespaces are unshared. Without Network the net namespace is
// unshared too.
type Bubblewrap struct{}

// Name implements Backend.
func (Bubblewrap) Name() string { return protocol.SandboxBubblewrap }

// Available reports whether bwrap exists and can create namespaces here.
func (Bubblewrap) Available() bool {
	p, err := exec.LookPath("bwrap")
	if err != nil {
		return false
	}
	return exec.Command(p, "--ro-bind", "/", "/", "--dev", "/dev", "--unshare-pid", "true").Run() == nil
}

// Args returns the bwrap argument list (exported for tests).
func (Bubblewrap) Args(s Spec) []string {
	args := []string{"--die-with-parent", "--new-session", "--unshare-pid", "--unshare-ipc", "--unshare-uts",
		"--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp"}
	if !s.Network {
		args = append(args, "--unshare-net")
	}
	// Mask shallow paths first so deeper binds survive.
	hidden := append([]string{}, s.Hidden...)
	sort.Slice(hidden, func(i, j int) bool { return len(hidden[i]) < len(hidden[j]) })
	for _, h := range hidden {
		if exists(h) {
			args = append(args, "--tmpfs", h)
		}
	}
	binds := [][2]string{}
	for _, p := range s.ReadOnly {
		binds = append(binds, [2]string{"--ro-bind", p})
	}
	for _, p := range s.Writable {
		binds = append(binds, [2]string{"--bind", p})
	}
	sort.SliceStable(binds, func(i, j int) bool { return len(binds[i][1]) < len(binds[j][1]) })
	for _, b := range binds {
		if exists(b[1]) {
			args = append(args, b[0], b[1], b[1])
		}
	}
	if s.Dir != "" {
		args = append(args, "--chdir", s.Dir)
	}
	return append(append(args, "--"), s.Argv...)
}

// Command implements Backend.
func (b Bubblewrap) Command(ctx context.Context, s Spec) (*exec.Cmd, error) {
	if len(s.Argv) == 0 {
		return nil, errors.New("sandbox: empty argv")
	}
	bin, err := exec.LookPath(s.Argv[0])
	if err == nil {
		s.Argv[0] = bin
		// The binary must stay visible inside the sandbox.
		s.ReadOnly = append(s.ReadOnly, filepath.Dir(resolve(bin)))
	}
	cmd := exec.CommandContext(ctx, "bwrap", b.Args(s)...)
	cmd.Dir, cmd.Env = s.Dir, s.Env
	return cmd, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

func resolve(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return p
}
