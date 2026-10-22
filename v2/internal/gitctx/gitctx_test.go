package gitctx

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
}

func TestCaptureAndWorkspace(t *testing.T) {
	ctx := context.Background()
	origin := t.TempDir()
	git(t, origin, "init", "-q", "--bare", "-b", "main")
	caller := t.TempDir()
	git(t, caller, "clone", "-q", origin, ".")
	os.WriteFile(filepath.Join(caller, "a.txt"), []byte("one\n"), 0o644)
	git(t, caller, "add", ".")
	git(t, caller, "commit", "-qm", "base")
	git(t, caller, "push", "-q", "origin", "HEAD:main")
	git(t, caller, "checkout", "-qb", "feature")
	os.WriteFile(filepath.Join(caller, "a.txt"), []byte("two\n"), 0o644)
	git(t, caller, "commit", "-qam", "change")
	os.WriteFile(filepath.Join(caller, "b.txt"), []byte("new\n"), 0o644) // untracked

	cc, err := Capture(ctx, caller, "")
	if err != nil {
		t.Fatal(err)
	}
	if cc.Branch != "feature" || !strings.Contains(cc.Patch, "+two") || !strings.Contains(cc.Patch, "b.txt") {
		t.Fatalf("%+v", cc)
	}
	callee := t.TempDir()
	git(t, callee, "clone", "-q", origin, ".")
	dir, note, cleanup := Workspace(ctx, t.TempDir(), "c1", callee, cc)
	defer cleanup()
	b, err := os.ReadFile(filepath.Join(dir, "code", "a.txt"))
	if err != nil || string(b) != "two\n" {
		t.Fatalf("checkout: %q %v (%s)", b, err, note)
	}
	if _, err := os.Stat(filepath.Join(dir, "code", "b.txt")); err != nil {
		t.Fatal("untracked file missing")
	}
	if _, err := os.Stat(filepath.Join(dir, "CHANGES.patch")); err != nil {
		t.Fatal("patch file missing")
	}
	// without a clone: patch only
	dir2, note2, c2 := Workspace(ctx, t.TempDir(), "c2", "", cc)
	defer c2()
	if !strings.Contains(note2, "patch") {
		t.Fatal(note2)
	}
	if _, err := os.Stat(filepath.Join(dir2, "CHANGES.patch")); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureNotARepo(t *testing.T) {
	if _, err := Capture(context.Background(), t.TempDir(), ""); err == nil {
		t.Fatal("expected error")
	}
}
