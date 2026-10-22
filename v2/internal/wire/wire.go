// Package wire holds the JSON shapes shared by server, daemon and MCP client.
package wire

import "time"

// Frame is one WebSocket message between server and daemon.
type Frame struct {
	Type string `json:"type"` // hello | call | progress | result | cancel
	// hello (daemon -> server)
	Agent    string `json:"agent,omitempty"`
	Hostname string `json:"hostname,omitempty"`
	OS       string `json:"os,omitempty"`
	Active   []string `json:"active,omitempty"`
	// call (server -> daemon)
	Call *Call `json:"call,omitempty"`
	// progress / result / cancel
	ID     string `json:"id,omitempty"`
	Text   string `json:"text,omitempty"`
	Status string `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// CodeContext lets the callee see the caller's code.
type CodeContext struct {
	Remote string `json:"remote,omitempty"`
	Branch string `json:"branch,omitempty"`
	Head   string `json:"head,omitempty"`
	Base   string `json:"base,omitempty"` // commit the patch applies to
	Patch  string `json:"patch,omitempty"`
	Note   string `json:"note,omitempty"`
}

// Call statuses.
const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
	StatusCanceled  = "canceled"
)

// Terminal reports whether a status is final.
func Terminal(s string) bool {
	return s == StatusSucceeded || s == StatusFailed || s == StatusCanceled
}

// Call is one agent-to-agent request.
type Call struct {
	ID         string       `json:"id"`
	FromDevice string       `json:"from_device"`
	FromOwner  string       `json:"from_owner"`
	ToDevice   string       `json:"to_device"`
	ToOwner    string       `json:"to_owner"`
	Prompt     string       `json:"prompt"`
	Context    *CodeContext `json:"context,omitempty"`
	TimeoutSec int          `json:"timeout_sec"`
	Status     string       `json:"status"`
	Result     string       `json:"result,omitempty"`
	Error      string       `json:"error,omitempty"`
	Progress   string       `json:"progress,omitempty"`
	CreatedAt  time.Time    `json:"created_at"`
	StartedAt  *time.Time   `json:"started_at,omitempty"`
	FinishedAt *time.Time   `json:"finished_at,omitempty"`
}

// Device is a registered host as listed to callers.
type Device struct {
	Name     string     `json:"name"`
	Owner    string     `json:"owner"`
	Online   bool       `json:"online"`
	Agent    string     `json:"agent,omitempty"`
	Hostname string     `json:"hostname,omitempty"`
	OS       string     `json:"os,omitempty"`
	Self     bool       `json:"self,omitempty"`
	LastSeen *time.Time `json:"last_seen,omitempty"`
}

// CallRequest is POST /api/calls.
type CallRequest struct {
	Target     string       `json:"target"`
	Prompt     string       `json:"prompt"`
	Context    *CodeContext `json:"context,omitempty"`
	TimeoutSec int          `json:"timeout_sec,omitempty"`
}
