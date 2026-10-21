//go:build linux

package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/collabos/collabos/server/internal/daemon/egress"
	"github.com/collabos/collabos/server/internal/daemon/sandbox"
)

// Real bwrap + netbridge + proxy: the sandbox has no network except the
// proxy, which only reaches allowlisted hosts.
func TestNetbridgeInBubblewrap(t *testing.T) {
	if !(sandbox.Bubblewrap{}).Available() {
		t.Skip("bwrap unavailable")
	}
	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl unavailable")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "collab")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, out)
	}
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = io.WriteString(w, "upstream-ok") }))
	defer up.Close()
	upAddr := strings.TrimPrefix(up.URL, "http://")
	sock := filepath.Join(dir, "proxy.sock")
	l, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatal(err)
	}
	p := &egress.Proxy{Allow: egress.Allowlist{"allowed.test"}, Dial: func(ctx context.Context, n, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, n, upAddr)
	}}
	go func() { _ = p.Serve(l) }()

	script := `curl -s -x http://127.0.0.1:18080 http://allowed.test/; echo;
curl -s -o /dev/null -w '%{http_code}' -x http://127.0.0.1:18080 http://blocked.test/; echo;
curl -s -m 3 http://` + upAddr + `/ || echo direct-failed`
	spec := sandbox.Spec{Argv: []string{bin, "netbridge", "--unix", sock, "--", "sh", "-c", script}, Dir: dir, Writable: []string{dir}, ReadOnly: []string{dir}}
	cmd, err := (sandbox.Bubblewrap{}).Command(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v %s", err, out)
	}
	got := string(out)
	for _, want := range []string{"upstream-ok", "403", "direct-failed"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}
