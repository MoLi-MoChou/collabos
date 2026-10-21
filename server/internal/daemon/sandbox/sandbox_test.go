package sandbox

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBwrapArgsOrder(t *testing.T) {
	d := t.TempDir()
	wt := filepath.Join(d, "repo", "wt")
	_ = os.MkdirAll(wt, 0o755)
	args := Bubblewrap{}.Args(Spec{Argv: []string{"x"}, Dir: wt, Hidden: []string{filepath.Join(d, "repo")}, Writable: []string{wt}})
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "--unshare-net") {
		t.Fatal("network should be unshared by default")
	}
	if strings.Index(joined, "--tmpfs "+filepath.Join(d, "repo")) > strings.Index(joined, "--bind "+wt) {
		t.Fatal("hidden mask must come before writable bind")
	}
}

func TestSeatbeltProfile(t *testing.T) {
	p := Seatbelt{}.Profile(Spec{Hidden: []string{"/Users/a"}, Writable: []string{"/Users/a/r/wt"}, Network: true})
	for _, want := range []string{"(deny default)", `(deny file-read* (subpath "/Users/a"))`, `(subpath "/Users/a/r/wt")`, "(allow network*)"} {
		if !strings.Contains(p, want) {
			t.Fatalf("profile missing %q:\n%s", want, p)
		}
	}
}

// TestBwrapConfines runs a real bubblewrap sandbox when available.
func TestBwrapConfines(t *testing.T) {
	if runtime.GOOS != "linux" || !(Bubblewrap{}).Available() {
		t.Skip("bubblewrap not available")
	}
	d := t.TempDir()
	repo := filepath.Join(d, "repo")
	wt := filepath.Join(repo, "wt")
	secret := filepath.Join(repo, "secret.txt")
	_ = os.MkdirAll(wt, 0o755)
	_ = os.WriteFile(secret, []byte("s3cret"), 0o644)
	outside := filepath.Join(d, "outside.txt")
	script := "echo ok > " + wt + "/in.txt; echo bad > " + outside + " 2>/dev/null; cat " + secret + " 2>/dev/null; true"
	cmd, err := Bubblewrap{}.Command(context.Background(), Spec{Argv: []string{"sh", "-c", script}, Dir: wt, Hidden: []string{repo}, Writable: []string{wt}})
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bwrap: %v %s", err, out)
	}
	if strings.Contains(string(out), "s3cret") {
		t.Fatal("hidden file was readable")
	}
	if _, err := os.Stat(filepath.Join(wt, "in.txt")); err != nil {
		t.Fatal("worktree write failed")
	}
	if _, err := os.Stat(outside); err == nil {
		t.Fatal("write outside worktree succeeded")
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip()
	}
}
