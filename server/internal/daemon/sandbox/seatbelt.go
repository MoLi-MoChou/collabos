package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/collabos/collabos/server/pkg/protocol"
)

// Seatbelt confines with macOS sandbox-exec. Reads are allowed except for
// Hidden paths; writes are allowed only under Writable paths and temp dirs.
//
// NOTE: implemented to the documented SBPL semantics but not runtime-verified
// in CI or on the Linux dev box.
type Seatbelt struct{}

// Name implements Backend.
func (Seatbelt) Name() string { return protocol.SandboxSeatbelt }

// Available reports whether sandbox-exec exists.
func (Seatbelt) Available() bool {
	_, err := exec.LookPath("sandbox-exec")
	return err == nil
}

func sbplString(s string) string {
	return `"` + strings.ReplaceAll(strings.ReplaceAll(s, `\`, `\\`), `"`, `\"`) + `"`
}

// Profile renders the SBPL profile (exported for tests).
func (Seatbelt) Profile(s Spec) string {
	var b strings.Builder
	b.WriteString("(version 1)\n(deny default)\n")
	b.WriteString("(allow process-exec process-fork signal sysctl-read mach-lookup ipc-posix-shm iokit-open)\n")
	b.WriteString("(allow file-read*)\n")
	for _, h := range s.Hidden {
		fmt.Fprintf(&b, "(deny file-read* (subpath %s))\n", sbplString(h))
	}
	for _, p := range append(append([]string{}, s.ReadOnly...), s.Writable...) {
		fmt.Fprintf(&b, "(allow file-read* (subpath %s))\n", sbplString(p))
	}
	b.WriteString("(allow file-write* (literal \"/dev/null\") (literal \"/dev/tty\") (subpath \"/private/tmp\") (subpath \"/private/var/folders\")")
	for _, p := range s.Writable {
		fmt.Fprintf(&b, " (subpath %s)", sbplString(p))
	}
	b.WriteString(")\n")
	if s.Network {
		b.WriteString("(allow network*)\n")
	} else {
		b.WriteString("(allow network* (remote unix-socket))\n")
	}
	return b.String()
}

// Command implements Backend.
func (sb Seatbelt) Command(ctx context.Context, s Spec) (*exec.Cmd, error) {
	if len(s.Argv) == 0 {
		return nil, errors.New("sandbox: empty argv")
	}
	args := append([]string{"-p", sb.Profile(s)}, s.Argv...)
	cmd := exec.CommandContext(ctx, "sandbox-exec", args...)
	cmd.Dir, cmd.Env = s.Dir, s.Env
	return cmd, nil
}
