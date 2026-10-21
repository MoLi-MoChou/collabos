package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := Git(context.Background(), dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// setup creates a bare remote with one commit on main and a clone.
func setup(t *testing.T) (remote, clone string) {
	d := t.TempDir()
	remote = filepath.Join(d, "remote.git")
	seed := filepath.Join(d, "seed")
	clone = filepath.Join(d, "clone")
	mustGit(t, d, "init", "--bare", "-b", "main", remote)
	mustGit(t, d, "init", "-b", "main", seed)
	_ = os.WriteFile(filepath.Join(seed, "README.md"), []byte("hi\n"), 0o644)
	mustGit(t, seed, "add", ".")
	mustGit(t, seed, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-m", "init")
	mustGit(t, seed, "remote", "add", "origin", remote)
	mustGit(t, seed, "push", "origin", "main")
	mustGit(t, d, "clone", remote, clone)
	return remote, clone
}

func TestWorktreeCommitPushMerge(t *testing.T) {
	ctx := context.Background()
	_, clone := setup(t)
	wt, err := Create(ctx, clone, "t1", "main")
	if err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(wt, "src"), 0o755)
	_ = os.WriteFile(filepath.Join(wt, "src", "a.txt"), []byte("a\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wt, "secret.txt"), []byte("x\n"), 0o644)
	_ = os.WriteFile(filepath.Join(wt, "README.md"), []byte("changed\n"), 0o644)
	ch, _ := Changed(ctx, wt)
	if strings.Join(ch, ",") != "README.md,secret.txt,src/a.txt" {
		t.Fatalf("changed = %v", ch)
	}
	Revert(ctx, wt, []string{"README.md"})
	Revert(ctx, wt, []string{"secret.txt"})
	commit, err := CommitAll(ctx, wt, "task")
	if err != nil || commit == "" {
		t.Fatalf("commit: %q %v", commit, err)
	}
	if err := Push(ctx, wt, Branch("t1")); err != nil {
		t.Fatal(err)
	}
	if err := Remove(ctx, clone, "t1"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatal("worktree not removed")
	}
	// Wrong approved commit is refused.
	if _, err := VerifiedMerge(ctx, clone, Branch("t1"), "main", "0000000000000000000000000000000000000000", "m"); err == nil {
		t.Fatal("merge of unapproved head must fail")
	}
	res, err := VerifiedMerge(ctx, clone, Branch("t1"), "main", commit, "merge t1")
	if err != nil {
		t.Fatal(err)
	}
	mustGit(t, clone, "fetch", "origin")
	if got := mustGit(t, clone, "rev-parse", "origin/main"); got != res.MergeCommit {
		t.Fatalf("origin/main=%s merge=%s", got, res.MergeCommit)
	}
	files := mustGit(t, clone, "ls-tree", "-r", "--name-only", "origin/main")
	if files != "README.md\nsrc/a.txt" {
		t.Fatalf("files on main: %q", files)
	}
}
