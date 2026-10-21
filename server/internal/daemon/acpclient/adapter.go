package acpclient

import "fmt"

// Supported protocol versions. CollabOS targets v2 and falls back to v1.
const (
	PreferredVersion = 2
	MinVersion       = 1
)

// Adapter hides wire differences between ACP versions so the rest of the
// daemon speaks one internal model.
type Adapter interface {
	Version() int
	// ResumeMethod is the method used to continue an existing session.
	ResumeMethod() string
	// AuthMethod is the method used to authenticate.
	AuthMethod() string
	// PermissionSelected builds the response for a chosen option.
	PermissionSelected(optionID string) any
	// PermissionCancelled builds the response when the client declines to choose.
	PermissionCancelled() any
}

type v1Adapter struct{}

func (v1Adapter) Version() int         { return 1 }
func (v1Adapter) ResumeMethod() string { return "session/load" }
func (v1Adapter) AuthMethod() string   { return "authenticate" }
func (v1Adapter) PermissionSelected(id string) any {
	return map[string]any{"outcome": map[string]any{"outcome": "selected", "optionId": id}}
}
func (v1Adapter) PermissionCancelled() any {
	return map[string]any{"outcome": map[string]any{"outcome": "cancelled"}}
}

// v2Adapter follows the renames noted in PRD 6.5. The permission response
// shape is kept identical to v1 until verified against the published v2
// schema (tracked as a week-2 item).
type v2Adapter struct{ v1Adapter }

func (v2Adapter) Version() int         { return 2 }
func (v2Adapter) ResumeMethod() string { return "session/resume" }
func (v2Adapter) AuthMethod() string   { return "auth/login" }

// AdapterFor returns the adapter for a negotiated version.
func AdapterFor(version int) (Adapter, error) {
	switch version {
	case 2:
		return v2Adapter{}, nil
	case 1:
		return v1Adapter{}, nil
	}
	return nil, fmt.Errorf("acp: unsupported protocol version %d (supported %d..%d)", version, MinVersion, PreferredVersion)
}

// Negotiate picks the version to use given the agent's answer to an
// initialize request that offered PreferredVersion.
func Negotiate(agentVersion int) (Adapter, error) {
	if agentVersion > PreferredVersion {
		// Spec: the agent must answer with a version the client offered or lower.
		return nil, fmt.Errorf("acp: agent answered version %d above offered %d", agentVersion, PreferredVersion)
	}
	return AdapterFor(agentVersion)
}
