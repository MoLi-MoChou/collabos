// Package gitctx captures the caller's code state (remote, branch, head and
// a patch of every change since a base commit, including uncommitted and
// untracked files) and recreates it on the callee.
package gitctx

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/collabos/collabos/v2/internal/wire"
)

// Git runs git in dir and returns trimmed stdout.
func Git(ctx context.Context, dir string, env []string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		msg := ""
		if ee, ok := err.(*exec.ExitError); ok {
			msg = strings.TrimSpace(string(ee.Stderr))
		}
		return "", fmt.Errorf("git %s: %v %s", strings.Join(args, " "), err, msg)
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// Capture describes dir's repository. base may be empty: then the merge-base
// with the upstream (or origin's default branch) is used, falling back to HEAD
// so only uncommitted work is included. The real index is never touched.
func Capture(ctx context.Context, dir, base string) (*wire.CodeContext, error) {
	top, err := Git(ctx, dir, nil, "rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("%s is not inside a git repository", dir)
	}
	c := &wire.CodeContext{}
	c.Remote, _ = Git(ctx, top, nil, "remote", "get-url", "origin")
	c.Branch, _ = Git(ctx, top, nil, "rev-parse", "--abbrev-ref", "HEAD")
	c.Head, _ = Git(ctx, top, nil, "rev-parse", "HEAD")
	if base == "" {
		for _, ref := range []string{"@{upstream}", "origin/HEAD", "origin/main", "origin/master"} {
			if mb, err := Git(ctx, top, nil, "merge-base", "HEAD", ref); err == nil && mb != "" {
				base = mb
				break
			}
		}
	}
	if base == "" {
		base = "HEAD"
	}
	if c.Head == "" { // no commits yet
		base = ""
	}
	if base != "" {
		if c.Base, err = Git(ctx, top, nil, "rev-parse", base); err != nil {
			return nil, err
		}
	}
	// Stage everything into a throwaway index to include untracked files.
	tmp, err := os.MkdirTemp("", "collab-index-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	env := []string{"GIT_INDEX_FILE=" + filepath.Join(tmp, "index")}
	if c.Head != "" {
		if _, err := Git(ctx, top, env, "read-tree", "HEAD"); err != nil {
			return nil, err
		}
	}
	if _, err := Git(ctx, top, env, "add", "-A"); err != nil {
		return nil, err
	}
	args := []string{"diff", "--cached", "--binary", "--no-color", "--no-ext-diff"}
	if c.Base != "" {
		args = append(args, c.Base)
	} else {
		args = append(args, "--root")
	}
	c.Patch, err = Git(ctx, top, env, args...)
	if err != nil {
		return nil, err
	}
	if c.Patch != "" {
		c.Patch += "\n"
	}
	return c, nil
}

// Workspace prepares a directory on the callee showing the caller's code:
// when repoPath (a local clone of the same project) has or can fetch the base
// commit, a detached worktree at base with the patch applied; otherwise an
// empty directory. The patch is always written to CHANGES.patch next to it.
// It returns the directory, a note for the prompt and a cleanup func.
func Workspace(ctx context.Context, scratch, id, repoPath string, cc *wire.CodeContext) (string, string, func()) {
	root := filepath.Join(scratch, id)
	_ = os.MkdirAll(root, 0o755)
	cleanup := func() {
		if repoPath != "" {
			_, _ = Git(context.Background(), repoPath, nil, "worktree", "remove", "--force", filepath.Join(root, "code"))
			_, _ = Git(context.Background(), repoPath, nil, "worktree", "prune")
		}
		_ = os.RemoveAll(root)
	}
	patchFile := filepath.Join(root, "CHANGES.patch")
	if cc != nil && cc.Patch != "" {
		_ = os.WriteFile(patchFile, []byte(cc.Patch), 0o644)
	}
	if cc == nil || repoPath == "" || cc.Base == "" {
		return root, "No checkout of the caller's repository is available on this machine; review from the patch.", cleanup
	}
	if _, err := Git(ctx, repoPath, nil, "cat-file", "-e", cc.Base+"^{commit}"); err != nil {
		_, _ = Git(ctx, repoPath, nil, "fetch", "--quiet", "origin")
		if cc.Branch != "" && cc.Branch != "HEAD" {
			_, _ = Git(ctx, repoPath, nil, "fetch", "--quiet", "origin", cc.Branch)
		}
	}
	code := filepath.Join(root, "code")
	if _, err := Git(ctx, repoPath, nil, "worktree", "add", "--detach", code, cc.Base); err != nil {
		return root, "Base commit " + short(cc.Base) + " is not available in the local clone; review from the patch.", cleanup
	}
	if cc.Patch != "" {
		if _, err := Git(ctx, code, nil, "apply", "--whitespace=nowarn", patchFile); err != nil {
			return root, "The patch did not apply to the local clone (" + err.Error() + "); review from CHANGES.patch. ./code is the base commit.", cleanup
		}
	}
	return root, "./code is a checkout of the caller's working tree (base " + short(cc.Base) + " + CHANGES.patch applied).", cleanup
}

func short(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}
