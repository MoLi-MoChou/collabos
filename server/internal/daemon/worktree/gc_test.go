package worktree

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffAndGC(t *testing.T) {
	ctx := context.Background()
	_, clone := setup(t)
	wt, err := Create(ctx, clone, "keep", "main")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Create(ctx, clone, "stale", "main"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(wt, "README.md"), []byte(strings.Repeat("line\n", 200)), 0o644)
	c, err := CommitAll(ctx, wt, "big")
	if err != nil {
		t.Fatal(err)
	}
	if d := Diff(ctx, wt, c, 1<<20); !strings.Contains(d, "+line") || strings.Contains(d, "truncated") {
		t.Fatalf("diff: %q", d[:80])
	}
	if d := Diff(ctx, wt, c, 100); !strings.HasSuffix(d, "(diff truncated)\n") {
		t.Fatalf("diff not truncated: %q", d)
	}
	removed := GC(ctx, clone, map[string]bool{"keep": true})
	if strings.Join(removed, ",") != "stale" {
		t.Fatalf("gc removed %v", removed)
	}
	if _, err := os.Stat(Path(clone, "keep")); err != nil {
		t.Fatal("kept worktree removed")
	}
	if _, err := os.Stat(Path(clone, "stale")); !os.IsNotExist(err) {
		t.Fatal("stale worktree survived")
	}
}
