// Package delegation implements the delegation protocol rules (PRD 6.1-6.4):
// grant subset, depth and budget limits, intake policy and reason codes.
package delegation

import (
	"path"
	"strings"
	"time"

	p "github.com/collabos/collabos/server/pkg/protocol"
)

// Reason codes (PRD 6.3).
const (
	CodeUnauthorized       = "unauthorized"
	CodeIntakeDenied       = "intake_denied"
	CodeQuotaExceeded      = "quota_exceeded"
	CodeScopeExceeded      = "scope_exceeded"
	CodeActionForbidden    = "action_forbidden"
	CodeSandboxDenied      = "sandbox_denied"
	CodeSandboxUnavailable = "sandbox_unavailable"
	CodeBudgetExceeded     = "budget_exceeded"
	CodeDepthExceeded      = "depth_exceeded"
	CodeDuplicate          = "duplicate"
)

// Error carries a reason code.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Code + ": " + e.Msg }

func fail(code, msg string) *Error { return &Error{Code: code, Msg: msg} }

// MatchResource reports whether a repo-relative path matches a glob.
// "**" matches everything, "dir/**" matches anything under dir, other
// patterns use path.Match.
func MatchResource(pattern, rel string) bool {
	rel = strings.TrimPrefix(path.Clean("/"+rel), "/")
	pattern = strings.TrimPrefix(pattern, "./")
	if pattern == "**" {
		return true
	}
	if strings.HasSuffix(pattern, "/**") {
		dir := strings.TrimSuffix(pattern, "/**")
		return rel == dir || strings.HasPrefix(rel, dir+"/")
	}
	ok, _ := path.Match(pattern, rel)
	return ok
}

// Allows reports whether the grant covers rel.
func Allows(g p.Grant, rel string) bool {
	for _, r := range g.Resources {
		if MatchResource(r, rel) {
			return true
		}
	}
	return false
}

// HasAction reports whether an action is granted.
func HasAction(g p.Grant, a string) bool {
	for _, x := range g.Actions {
		if x == a {
			return true
		}
	}
	return false
}

// covers reports whether parent pattern covers child pattern.
func covers(parent, child string) bool {
	if parent == "**" || parent == child {
		return true
	}
	if strings.HasSuffix(parent, "/**") {
		dir := strings.TrimSuffix(parent, "/**")
		c := strings.TrimSuffix(child, "/**")
		return c == dir || strings.HasPrefix(c, dir+"/")
	}
	if !strings.ContainsAny(child, "*?[") {
		return MatchResource(parent, child)
	}
	return false
}

// IsSubset reports whether child does not widen parent (resources, actions,
// network and expiry).
func IsSubset(child, parent p.Grant) bool {
	for _, c := range child.Resources {
		ok := false
		for _, pr := range parent.Resources {
			if covers(pr, c) {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	for _, a := range child.Actions {
		if !HasAction(parent, a) {
			return false
		}
	}
	for _, n := range child.Network {
		found := false
		for _, pn := range parent.Network {
			if pn == n {
				found = true
			}
		}
		if !found {
			return false
		}
	}
	if parent.ExpiresAt != "" {
		pe, err1 := time.Parse(time.RFC3339, parent.ExpiresAt)
		ce, err2 := time.Parse(time.RFC3339, child.ExpiresAt)
		if err1 == nil && (err2 != nil || ce.After(pe)) {
			return false
		}
	}
	return true
}

// Request is a delegation being validated.
type Request struct {
	Grant    p.Grant
	Budget   p.Budget
	Depth    int
	MaxDepth int
}

// Parent is the delegation a sub-delegation derives from.
type Parent struct {
	Grant      p.Grant
	Budget     p.Budget
	UsedTokens int64
	MaxDepth   int
}

// Validate applies depth, grant-subset and budget rules.
func Validate(r Request, parent *Parent) *Error {
	if r.MaxDepth <= 0 {
		r.MaxDepth = 3
	}
	if parent != nil && parent.MaxDepth > 0 && r.MaxDepth > parent.MaxDepth {
		r.MaxDepth = parent.MaxDepth
	}
	if r.Depth > r.MaxDepth {
		return fail(CodeDepthExceeded, "delegation depth exceeds max_depth")
	}
	if len(r.Grant.Resources) == 0 || len(r.Grant.Actions) == 0 {
		return fail(CodeScopeExceeded, "grant must list resources and actions")
	}
	if parent != nil {
		if !IsSubset(r.Grant, parent.Grant) {
			return fail(CodeScopeExceeded, "grant must be a subset of the parent grant")
		}
		if parent.Budget.MaxTokens > 0 {
			remaining := parent.Budget.MaxTokens - parent.UsedTokens
			if r.Budget.MaxTokens == 0 || r.Budget.MaxTokens > remaining {
				return fail(CodeBudgetExceeded, "budget exceeds parent's remaining budget")
			}
		}
	}
	return nil
}

// Intake modes.
const (
	IntakeManual    = "manual"
	IntakeAllowlist = "allowlist"
	IntakeClosed    = "closed"
)

// IntakePolicy is the target owner's policy for one agent.
type IntakePolicy struct {
	Mode       string
	Allowlist  []string
	DailyQuota int
}

// IntakeInput describes an incoming delegation.
type IntakeInput struct {
	RequesterID   string
	OwnerID       string
	AcceptedToday int64
	Sandbox       string
}

// IntakeOutcome is the automatic intake result.
type IntakeOutcome string

const (
	IntakeAccept  IntakeOutcome = "accept"
	IntakePending IntakeOutcome = "pending"
	IntakeReject  IntakeOutcome = "reject"
)

// DecideIntake applies the policy. Self-delegation is always accepted.
func DecideIntake(pol IntakePolicy, in IntakeInput) (IntakeOutcome, string) {
	if in.RequesterID == in.OwnerID {
		return IntakeAccept, ""
	}
	if in.Sandbox == "" || in.Sandbox == p.SandboxNone {
		return IntakeReject, CodeSandboxUnavailable
	}
	if pol.Mode == IntakeClosed {
		return IntakeReject, CodeIntakeDenied
	}
	if pol.DailyQuota > 0 && in.AcceptedToday >= int64(pol.DailyQuota) {
		return IntakeReject, CodeQuotaExceeded
	}
	if pol.Mode == IntakeAllowlist {
		for _, id := range pol.Allowlist {
			if id == in.RequesterID {
				return IntakeAccept, ""
			}
		}
	}
	return IntakePending, ""
}
