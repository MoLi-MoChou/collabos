// Package worktree manages per-task git worktrees and the git operations the
// daemon performs on the user's behalf (commit, push, verified merge).
package worktree

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Git runs git in dir and returns trimmed stdout.
func Git(ctx context.Context, dir string, args ...string) (string, error) {
	out, err := gitRaw(ctx, dir, args...)
	return strings.TrimSpace(out), err
}

func gitRaw(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return out.String(), fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(errb.String()))
	}
	return out.String(), nil
}

// Branch is the task branch name.
func Branch(taskID string) string { return "collabos/task-" + taskID }

// Path is the task worktree path inside repo.
func Path(repo, taskID string) string {
	return filepath.Join(repo, ".collabos", "worktrees", "task-"+taskID)
}

// Create fetches origin and adds a fresh worktree on a new task branch.
func Create(ctx context.Context, repo, taskID, base string) (string, error) {
	if _, err := Git(ctx, repo, "fetch", "origin", base); err != nil {
		return "", err
	}
	if err := ensureExcluded(repo); err != nil {
		return "", err
	}
	wt := Path(repo, taskID)
	_ = Remove(ctx, repo, taskID)
	if err := os.MkdirAll(filepath.Dir(wt), 0o755); err != nil {
		return "", err
	}
	if _, err := Git(ctx, repo, "worktree", "add", "-B", Branch(taskID), wt, "origin/"+base); err != nil {
		return "", err
	}
	return wt, nil
}

func ensureExcluded(repo string) error {
	gitDir, err := Git(context.Background(), repo, "rev-parse", "--git-common-dir")
	if err != nil {
		return err
	}
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(repo, gitDir)
	}
	p := filepath.Join(gitDir, "info", "exclude")
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), ".collabos/") {
		return nil
	}
	_ = os.MkdirAll(filepath.Dir(p), 0o755)
	return os.WriteFile(p, append(b, []byte("\n.collabos/\n")...), 0o644)
}

// GitDir returns the repository's common .git directory.
func GitDir(repo string) string {
	d, err := Git(context.Background(), repo, "rev-parse", "--git-common-dir")
	if err != nil {
		return filepath.Join(repo, ".git")
	}
	if !filepath.IsAbs(d) {
		d = filepath.Join(repo, d)
	}
	return filepath.Clean(d)
}

// Remove deletes the worktree and the local task branch.
func Remove(ctx context.Context, repo, taskID string) error {
	wt := Path(repo, taskID)
	_, err1 := Git(ctx, repo, "worktree", "remove", "--force", wt)
	_ = os.RemoveAll(wt)
	_, _ = Git(ctx, repo, "worktree", "prune")
	_, _ = Git(ctx, repo, "branch", "-D", Branch(taskID))
	if err1 != nil && !strings.Contains(err1.Error(), "is not a working tree") {
		return err1
	}
	return nil
}

// Changed lists paths changed in the worktree (repo-relative, forward slashes).
func Changed(ctx context.Context, wt string) ([]string, error) {
	out, err := gitRaw(ctx, wt, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, err
	}
	var files []string
	parts := strings.Split(out, "\x00")
	for i := 0; i < len(parts); i++ {
		p := parts[i]
		if len(p) < 4 {
			continue
		}
		code := p[:2]
		files = append(files, p[3:])
		if code[0] == 'R' || code[0] == 'C' {
			i++ // skip rename source
		}
	}
	return files, nil
}

// Revert discards changes to the given paths.
func Revert(ctx context.Context, wt string, paths []string) {
	for _, p := range paths {
		if _, err := Git(ctx, wt, "checkout", "HEAD", "--", p); err != nil {
			_ = os.RemoveAll(filepath.Join(wt, p))
		}
	}
}

// CommitAll commits every change; returns "" when there is nothing to commit.
func CommitAll(ctx context.Context, wt, msg string) (string, error) {
	if _, err := Git(ctx, wt, "add", "-A"); err != nil {
		return "", err
	}
	if out, _ := Git(ctx, wt, "status", "--porcelain"); out == "" {
		return "", nil
	}
	if _, err := Git(ctx, wt, append(identity(ctx, wt, "CollabOS Agent", "agent@collabos.local"), "commit", "-m", msg)...); err != nil {
		return "", err
	}
	return Git(ctx, wt, "rev-parse", "HEAD")
}

// DiffStat summarizes the commit.
func DiffStat(ctx context.Context, wt, commit string) string {
	out, _ := Git(ctx, wt, "show", "--stat", "--format=", commit)
	return out
}

// Diff returns the commit's patch, truncated to max bytes (diff preview, FR-CLI-203).
func Diff(ctx context.Context, wt, commit string, max int) string {
	out, _ := gitRaw(ctx, wt, "show", "--format=", "--no-color", commit)
	if len(out) > max {
		out = out[:max] + "\n… (diff truncated)\n"
	}
	return out
}

// GC removes task worktrees and branches left behind by crashed runs, except
// those in keep (PRD 7.2.3 worktree GC). It returns the removed task ids.
func GC(ctx context.Context, repo string, keep map[string]bool) []string {
	dir := filepath.Join(repo, ".collabos", "worktrees")
	entries, _ := os.ReadDir(dir)
	var removed []string
	for _, e := range entries {
		id, ok := strings.CutPrefix(e.Name(), "task-")
		if !ok || !e.IsDir() || keep[id] {
			continue
		}
		if Remove(ctx, repo, id) == nil {
			removed = append(removed, id)
		}
	}
	_, _ = Git(ctx, repo, "worktree", "prune")
	return removed
}

// Push pushes the task branch to origin using the local user's credentials.
func Push(ctx context.Context, wt, branch string) error {
	_, err := Git(ctx, wt, "push", "--force-with-lease", "origin", "HEAD:refs/heads/"+branch)
	return err
}

// MergeResult is the outcome of a verified merge.
type MergeResult struct {
	HeadCommit  string
	MergeCommit string
}

// VerifiedMerge merges origin/branch into origin/base only if the branch head
// equals approved, then pushes base. It runs in a temporary worktree with the
// local user's (approver's) own git credentials.
func VerifiedMerge(ctx context.Context, repo, branch, base, approved, msg string) (MergeResult, error) {
	if _, err := Git(ctx, repo, "fetch", "origin", base, branch); err != nil {
		return MergeResult{}, err
	}
	head, err := Git(ctx, repo, "rev-parse", "origin/"+branch)
	if err != nil {
		return MergeResult{}, err
	}
	res := MergeResult{HeadCommit: head}
	if head != approved {
		return res, fmt.Errorf("branch head %s does not match approved commit %s", head, approved)
	}
	if err := ensureExcluded(repo); err != nil {
		return res, err
	}
	tmp := filepath.Join(repo, ".collabos", "merge-"+approved[:12])
	_, _ = Git(ctx, repo, "worktree", "remove", "--force", tmp)
	defer func() {
		_, _ = Git(context.Background(), repo, "worktree", "remove", "--force", tmp)
		_ = os.RemoveAll(tmp)
	}()
	if _, err := Git(ctx, repo, "worktree", "add", "--detach", tmp, "origin/"+base); err != nil {
		return res, err
	}
	if _, err := Git(ctx, tmp, append(identity(ctx, tmp, "CollabOS Merge", "merge@collabos.local"), "merge", "--no-ff", "-m", msg, approved)...); err != nil {
		_, _ = Git(ctx, tmp, "merge", "--abort")
		return res, err
	}
	mc, err := Git(ctx, tmp, "rev-parse", "HEAD")
	if err != nil {
		return res, err
	}
	if _, err := Git(ctx, tmp, "push", "origin", "HEAD:refs/heads/"+base); err != nil {
		return res, err
	}
	res.MergeCommit = mc
	return res, nil
}

// identity uses the local git identity, falling back to a CollabOS one.
func identity(ctx context.Context, dir, name, email string) []string {
	if e, _ := Git(ctx, dir, "config", "user.email"); e != "" {
		return nil
	}
	return []string{"-c", "user.name=" + name, "-c", "user.email=" + email}
}
