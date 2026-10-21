// Package approval is the Layer-1 policy enforcement point (PRD 7.2.1). It
// runs inside the daemon for every ACP permission request: requests inside
// the grant are allowed locally, requests leaving the worktree or matching
// the defense-in-depth denylist are denied, and everything else (outside the
// granted resources/actions) is escalated to an eligible human as a card.
package approval

import (
	"path/filepath"
	"strings"

	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Verdict is the local decision.
type Verdict string

const (
	Allow    Verdict = "allow"
	Deny     Verdict = "deny"
	Escalate Verdict = "escalate"
)

// Result explains a verdict.
type Result struct {
	Verdict Verdict
	Code    string
	Reason  string
}

// Policy holds the command denylist. The denylist is defense-in-depth only;
// the OS sandbox (Layer 2) is the real boundary for commands.
type Policy struct {
	DenySubstrings []string
}

// Default returns the standard policy.
func Default() Policy {
	return Policy{DenySubstrings: []string{"rm -rf /", "sudo ", "mkfs", "chmod -r 777", ":(){", "git push", "dd if="}}
}

// ActionFor maps an ACP tool kind to a grant action.
func ActionFor(kind string) string {
	switch kind {
	case "read", "search":
		return "read"
	case "edit", "delete", "move":
		return "edit"
	case "execute":
		return "execute"
	case "fetch":
		return "network"
	default:
		return kind
	}
}

// Decide evaluates req against grant. root is the task worktree.
func (p Policy) Decide(g protocol.Grant, req protocol.PermissionRequest, root string) Result {
	for _, path := range req.Paths {
		if !Within(root, path) {
			return Result{Deny, delegation.CodeScopeExceeded, "path outside worktree: " + path}
		}
	}
	cmd := strings.ToLower(req.Command)
	for _, bad := range p.DenySubstrings {
		if cmd != "" && strings.Contains(cmd, bad) {
			return Result{Deny, delegation.CodeActionForbidden, "command matches denylist: " + bad}
		}
	}
	action := ActionFor(req.Kind)
	if !delegation.HasAction(g, action) {
		return Result{Escalate, delegation.CodeActionForbidden, "action not granted: " + action}
	}
	for _, path := range req.Paths {
		rel := Rel(root, path)
		if !delegation.Allows(g, rel) {
			return Result{Escalate, delegation.CodeScopeExceeded, "path outside granted resources: " + rel}
		}
	}
	return Result{Allow, "", "within grant"}
}

// Within reports whether path is inside root (lexically; callers resolve symlinks).
func Within(root, path string) bool {
	if root == "" {
		return false
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	return err == nil && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Rel returns path relative to root using forward slashes.
func Rel(root, path string) string {
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, path)
	}
	rel, err := filepath.Rel(filepath.Clean(root), filepath.Clean(path))
	if err != nil {
		return path
	}
	return filepath.ToSlash(rel)
}

// Widen returns g with extra resources added (after a human approval).
func Widen(g protocol.Grant, resources []string, action string) protocol.Grant {
	out := g
	out.Resources = append(append([]string{}, g.Resources...), resources...)
	if action != "" && !delegation.HasAction(g, action) {
		out.Actions = append(append([]string{}, g.Actions...), action)
	}
	return out
}
