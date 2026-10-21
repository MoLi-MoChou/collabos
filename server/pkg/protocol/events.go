package protocol

import "encoding/json"

// Event types (PRD 9.1).
const (
	TypeClientHandshake      = "CLIENT_HANDSHAKE"
	TypeServerHandshakeAck   = "SERVER_HANDSHAKE_ACK"
	TypeHeartbeatPing        = "HEARTBEAT_PING"
	TypeHeartbeatPong        = "HEARTBEAT_PONG"
	TypeAck                  = "ACK"
	TypeDelegationDispatch   = "DELEGATION_DISPATCH"
	TypeTaskCancel           = "TASK_CANCEL"
	TypeDaemonProgressStream = "DAEMON_PROGRESS_STREAM"
	TypeDaemonTaskCompleted  = "DAEMON_TASK_COMPLETED"
	TypePermissionRequest    = "PERMISSION_REQUEST"
	TypePermissionDecision   = "PERMISSION_DECISION"
	TypeGrantUpdate          = "GRANT_UPDATE"
	TypeDaemonPolicyBlocked  = "DAEMON_POLICY_BLOCKED"
	TypeMergeExecute         = "MERGE_EXECUTE"
	TypeMergeResult          = "MERGE_RESULT"
	TypeTaskPause            = "TASK_PAUSE"
	TypeTaskResume           = "TASK_RESUME"
	// Server -> UI.
	TypeTaskEvent   = "TASK_EVENT"
	TypeTaskUpdated = "TASK_UPDATED"
	TypeDaemonState = "DAEMON_STATE"
	TypeRoomMessage = "ROOM_MESSAGE"
	TypeDockChanged = "DOCK_CHANGED"
)

// ClientType distinguishes UI and daemon connections.
type ClientType string

const (
	ClientUI     ClientType = "ui"
	ClientDaemon ClientType = "daemon"
)

// Sandbox levels a daemon can offer (PRD 7.2.4).
const (
	SandboxNone       = "none"
	SandboxBubblewrap = "bubblewrap"
	SandboxSeatbelt   = "seatbelt"
)

// RepoInfo is a local checkout a daemon can work on.
type RepoInfo struct {
	RemoteURL string `json:"remote_url"`
	Path      string `json:"path"`
}

// Capabilities is what a daemon detected on its host.
type Capabilities struct {
	OS       string            `json:"os"`
	Arch     string            `json:"arch"`
	Hostname string            `json:"hostname"`
	Tools    map[string]string `json:"tools"`
	Agents   []string          `json:"agents"`
	Sandbox  string            `json:"sandbox"`
	Repos    []RepoInfo        `json:"repos"`
	Version  string            `json:"version"`
}

// ClientHandshake is the first frame a client sends.
type ClientHandshake struct {
	ClientType   ClientType    `json:"client_type"`
	Capabilities *Capabilities `json:"capabilities,omitempty"`
	// ActiveTasks are tasks still running on the daemon.
	ActiveTasks []string `json:"active_tasks"`
	// JournaledTasks have unacknowledged frames that will be replayed.
	JournaledTasks []string `json:"journaled_tasks"`
}

// ServerHandshakeAck confirms a connection.
type ServerHandshakeAck struct {
	SessionID         string `json:"session_id"`
	HeartbeatInterval int    `json:"heartbeat_interval"`
	ServerTime        string `json:"server_time"`
	DaemonID          string `json:"daemon_id,omitempty"`
	LastAckedSeq      int64  `json:"last_acked_seq"`
}

// Ack confirms the server persisted daemon frames up to AckSeq.
type Ack struct {
	AckSeq int64 `json:"ack_seq"`
}

// Grant limits what a delegation may touch (PRD 6.1). Resources are globs
// relative to the repository root ("src/**", "README.md", "**").
type Grant struct {
	Resources []string `json:"resources"`
	Actions   []string `json:"actions"`
	Network   []string `json:"network"`
	ExpiresAt string   `json:"expires_at,omitempty"`
}

// Budget caps a delegation.
type Budget struct {
	MaxTokens      int64 `json:"max_tokens"`
	MaxDurationMin int   `json:"max_duration_min"`
}

// Billing records who pays for what (PRD 6.4).
type Billing struct {
	Compute  string `json:"compute"`
	ModelKey string `json:"model_key"`
}

// Approver is a named person allowed to approve.
type Approver struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// Task kinds.
const (
	KindCode = "code"
	KindChat = "chat"
	// KindPatrol is a read-only proactive run whose findings an agent posts (scenario D).
	KindPatrol = "patrol"
)

// ContextItem is one trust-labelled piece of context (PRD 11.4).
type ContextItem struct {
	Type  string `json:"type"`
	Trust string `json:"trust"`
	Text  string `json:"text"`
	// Source names where the item came from (e.g. "MEMORY.md v3 confirmed by 张三").
	Source string `json:"source,omitempty"`
}

// DelegationDispatch asks a daemon to run a task.
type DelegationDispatch struct {
	TaskID     string        `json:"task_id"`
	Kind       string        `json:"kind"`
	Title      string        `json:"title"`
	Prompt     string        `json:"prompt"`
	RemoteURL  string        `json:"remote_url,omitempty"`
	BaseBranch string        `json:"base_branch,omitempty"`
	Provider   string        `json:"provider,omitempty"`
	Agent      string        `json:"agent"`
	Model      string        `json:"model,omitempty"`
	Grant      Grant         `json:"grant"`
	Budget     Budget        `json:"budget"`
	Context    []ContextItem `json:"context"`
	CrossOwner bool          `json:"cross_owner"`
	// TraceID follows the delegation chain end to end (FR-SRV-602).
	TraceID      string `json:"trace_id,omitempty"`
	ParentTaskID string `json:"parent_task_id,omitempty"`
	Depth        int    `json:"depth,omitempty"`
	MaxDepth     int    `json:"max_depth,omitempty"`
	// Autonomy is supervised|standard|autonomous (PRD 3.2, 6.3).
	Autonomy string `json:"autonomy,omitempty"`
	// Acceptance criteria the agent must address (PRD 6.1).
	Acceptance []string `json:"acceptance,omitempty"`
	// TestCommand, when set by the project lead, runs in the sandbox after the agent.
	TestCommand string `json:"test_command,omitempty"`
	// Roster lists specialists the agent may sub-delegate to (multi-level delegation).
	Roster []string `json:"roster,omitempty"`
}

// TaskPause suspends a running task (budget or decision timeout); TaskResume continues it.
type TaskPause struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
}

// TestResult is the outcome of the project's test command (PRD 9.1 tests).
type TestResult struct {
	Command string `json:"command"`
	Passed  bool   `json:"passed"`
	Output  string `json:"output"`
}

// RichUI is an OpenUI Lang rich message carried in ACP _meta.collabos.ui (PRD 7.1.5).
type RichUI struct {
	Lang    string `json:"lang"`
	Version string `json:"version"`
	Source  string `json:"source"`
}

// TaskCancel asks a daemon to stop a task.
type TaskCancel struct {
	TaskID string `json:"task_id"`
	Reason string `json:"reason,omitempty"`
}

// ProgressKind classifies a progress event.
type ProgressKind string

const (
	ProgressPhase     ProgressKind = "phase"
	ProgressACPUpdate ProgressKind = "acp_update"
	ProgressLog       ProgressKind = "log"
	ProgressUsage     ProgressKind = "usage"
)

// DaemonProgress streams one execution step.
type DaemonProgress struct {
	TaskID string          `json:"task_id"`
	Kind   ProgressKind    `json:"kind"`
	Phase  string          `json:"phase,omitempty"`
	Text   string          `json:"text,omitempty"`
	Update json.RawMessage `json:"update,omitempty" tstype:"unknown"`
	Usage  *Usage          `json:"usage,omitempty"`
}

// Usage mirrors the token usage an agent reports.
type Usage struct {
	InputTokens      int64 `json:"input_tokens"`
	OutputTokens     int64 `json:"output_tokens"`
	ThoughtTokens    int64 `json:"thought_tokens"`
	CachedReadTokens int64 `json:"cached_read_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
}

// TaskOutcome is the terminal result kind reported by a daemon.
type TaskOutcome string

const (
	OutcomeCompleted TaskOutcome = "completed"
	OutcomeFailed    TaskOutcome = "failed"
	OutcomeCanceled  TaskOutcome = "canceled"
)

// DaemonTaskCompleted reports the end of a task's execution.
type DaemonTaskCompleted struct {
	TaskID     string      `json:"task_id"`
	Outcome    TaskOutcome `json:"outcome"`
	StopReason string      `json:"stop_reason,omitempty"`
	Summary    string      `json:"summary,omitempty"`
	Branch     string      `json:"branch,omitempty"`
	BaseBranch string      `json:"base_branch,omitempty"`
	Commit     string      `json:"commit,omitempty"`
	Pushed     bool        `json:"pushed"`
	PRURL      string      `json:"pr_url,omitempty"`
	DiffStat   string      `json:"diff_stat,omitempty"`
	Error      string      `json:"error,omitempty"`
	ReasonCode string      `json:"reason_code,omitempty"`
	Usage      *Usage      `json:"usage,omitempty"`
	ACPVersion int         `json:"acp_version,omitempty"`
	DurationMs int64       `json:"duration_ms"`
	TraceID    string      `json:"trace_id,omitempty"`
	Tests      *TestResult `json:"tests,omitempty"`
	UI         *RichUI     `json:"ui,omitempty"`
	Diff       string      `json:"diff,omitempty"`
}

// PermissionRequest forwards an agent's session/request_permission.
type PermissionRequest struct {
	RequestID string   `json:"request_id"`
	TaskID    string   `json:"task_id"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Paths     []string `json:"paths"`
	Command   string   `json:"command,omitempty"`
	Worktree  string   `json:"worktree"`
	Layer     string   `json:"layer"`
}

// Decision values for permission requests.
const (
	DecisionAllow   = "allow"
	DecisionDeny    = "deny"
	DecisionPending = "pending"
)

// PermissionDecision answers a PermissionRequest.
type PermissionDecision struct {
	RequestID  string `json:"request_id"`
	TaskID     string `json:"task_id"`
	Decision   string `json:"decision"`
	ReasonCode string `json:"reason_code,omitempty"`
	Reason     string `json:"reason,omitempty"`
	DecidedBy  string `json:"decided_by"`
	Grant      *Grant `json:"grant,omitempty"`
}

// PolicyBlocked reports a local PEP denial.
type PolicyBlocked struct {
	TaskID string `json:"task_id"`
	Code   string `json:"code"`
	Layer  string `json:"layer"`
	Target string `json:"target"`
}

// MergeExecute asks the approver's daemon to merge with the approver's own credentials.
type MergeExecute struct {
	ApprovalID     string `json:"approval_id"`
	TaskID         string `json:"task_id"`
	RemoteURL      string `json:"remote_url"`
	Provider       string `json:"provider"`
	Branch         string `json:"branch"`
	BaseBranch     string `json:"base_branch"`
	ApprovedCommit string `json:"approved_commit"`
	PRNumber       int32  `json:"pr_number"`
	PRURL          string `json:"pr_url,omitempty"`
	Message        string `json:"message"`
}

// MergeResult reports the merge outcome.
type MergeResult struct {
	ApprovalID  string `json:"approval_id"`
	TaskID      string `json:"task_id"`
	OK          bool   `json:"ok"`
	HeadCommit  string `json:"head_commit"`
	MergeCommit string `json:"merge_commit,omitempty"`
	Error       string `json:"error,omitempty"`
}

// TaskEvent is a persisted task event broadcast to UI clients.
type TaskEvent struct {
	TaskID    string          `json:"task_id"`
	Seq       int32           `json:"seq"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload" tstype:"unknown"`
	CreatedAt string          `json:"created_at"`
}

// TaskStatus values; see internal/taskstate for transitions.
type TaskStatus string

const (
	StatusSubmitted      TaskStatus = "submitted"
	StatusPendingIntake  TaskStatus = "pending_intake"
	StatusAccepted       TaskStatus = "accepted"
	StatusRejected       TaskStatus = "rejected"
	StatusQueued         TaskStatus = "queued"
	StatusWorking        TaskStatus = "working"
	StatusRunningOffline TaskStatus = "running_offline"
	StatusInputRequired  TaskStatus = "input_required"
	StatusApproval       TaskStatus = "approval_required"
	StatusCompleted      TaskStatus = "completed"
	StatusFailed         TaskStatus = "failed"
	StatusCanceled       TaskStatus = "canceled"
	StatusUnknown        TaskStatus = "unknown"
)

// TaskUpdated tells UI clients a task changed.
type TaskUpdated struct {
	TaskID string     `json:"task_id"`
	Status TaskStatus `json:"status"`
}

// DaemonState tells UI clients a daemon went online/offline.
type DaemonState struct {
	DaemonID string `json:"daemon_id"`
	Status   string `json:"status"`
}

// DockChanged tells UI clients to refetch a dock section.
type DockChanged struct {
	Section string `json:"section"`
}
