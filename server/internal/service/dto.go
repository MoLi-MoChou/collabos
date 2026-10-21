package service

import (
	"encoding/json"

	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// TaskDTO is the API shape of a task.
type TaskDTO struct {
	ID              string          `json:"id"`
	WorkspaceID     string          `json:"workspace_id"`
	RoomID          string          `json:"room_id"`
	ProjectID       string          `json:"project_id"`
	SourceMessageID string          `json:"source_message_id"`
	ParentTaskID    string          `json:"parent_task_id"`
	CreatedBy       string          `json:"created_by"`
	SourceAgentID   string          `json:"source_agent_id"`
	TargetAgentID   string          `json:"target_agent_id"`
	TargetOwnerID   string          `json:"target_owner_id"`
	DaemonID        string          `json:"daemon_id"`
	Kind            string          `json:"kind"`
	Title           string          `json:"title"`
	Prompt          string          `json:"prompt"`
	Status          string          `json:"status"`
	Grant           json.RawMessage `json:"grant"`
	Budget          json.RawMessage `json:"budget"`
	Billing         json.RawMessage `json:"billing"`
	Approvers       json.RawMessage `json:"approvers"`
	Depth           int32           `json:"depth"`
	MaxDepth        int32           `json:"max_depth"`
	ReasonCode      string          `json:"reason_code"`
	UsedTokens      int64           `json:"used_tokens"`
	Result          json.RawMessage `json:"result"`
	CreatedAt       string          `json:"created_at"`
	UpdatedAt       string          `json:"updated_at"`
}

// TaskDTOOf maps a task row.
func TaskDTOOf(t db.Task) TaskDTO {
	u := util.UUIDString
	return TaskDTO{
		ID: u(t.ID), WorkspaceID: u(t.WorkspaceID), RoomID: u(t.RoomID), ProjectID: u(t.ProjectID),
		SourceMessageID: u(t.SourceMessageID), ParentTaskID: u(t.ParentTaskID), CreatedBy: u(t.CreatedBy),
		SourceAgentID: u(t.SourceAgentID), TargetAgentID: u(t.TargetAgentID), TargetOwnerID: u(t.TargetOwnerID),
		DaemonID: u(t.DaemonID), Kind: t.Kind, Title: t.Title, Prompt: t.Prompt, Status: t.Status,
		Grant: raw(t.GrantSpec, "{}"), Budget: raw(t.Budget, "{}"), Billing: raw(t.Billing, "{}"),
		Approvers: raw(t.Approvers, "[]"), Depth: t.Depth, MaxDepth: t.MaxDepth, ReasonCode: t.ReasonCode,
		UsedTokens: t.UsedTokens, Result: raw(t.Result, "{}"), CreatedAt: util.TS(t.CreatedAt), UpdatedAt: util.TS(t.UpdatedAt),
	}
}

// DaemonDTO is the API shape of a daemon.
type DaemonDTO struct {
	ID           string          `json:"id"`
	Name         string          `json:"name"`
	OwnerID      string          `json:"owner_id"`
	Status       string          `json:"status"`
	Capabilities json.RawMessage `json:"capabilities"`
	LastSeenAt   string          `json:"last_seen_at"`
}

func daemonDTO(d db.Daemon, online bool) DaemonDTO {
	status := "offline"
	if online {
		status = "online"
	}
	return DaemonDTO{ID: util.UUIDString(d.ID), Name: d.Name, OwnerID: util.UUIDString(d.OwnerID), Status: status, Capabilities: raw(d.Capabilities, "{}"), LastSeenAt: util.TS(d.LastSeenAt)}
}

// AgentDTO is the API shape of an agent.
type AgentDTO struct {
	ID              string   `json:"id"`
	OwnerID         string   `json:"owner_id"`
	DaemonID        string   `json:"daemon_id"`
	Name            string   `json:"name"`
	Handle          string   `json:"handle"`
	Kind            string   `json:"kind"`
	Runtime         string   `json:"runtime"`
	Model           string   `json:"model"`
	IntakeMode      string   `json:"intake_mode"`
	IntakeAllowlist []string `json:"intake_allowlist"`
	DailyQuota      int32    `json:"daily_quota"`
	Online          bool     `json:"online"`
}

func (s *Service) agentDTO(a db.Agent) AgentDTO {
	allow := []string{}
	_ = util.Unmarshal(a.IntakeAllowlist, &allow)
	return AgentDTO{
		ID: util.UUIDString(a.ID), OwnerID: util.UUIDString(a.OwnerID), DaemonID: util.UUIDString(a.DaemonID),
		Name: a.Name, Handle: a.Handle, Kind: a.Kind, Runtime: a.Runtime, Model: a.Model,
		IntakeMode: a.IntakeMode, IntakeAllowlist: allow, DailyQuota: a.DailyQuota,
		Online: s.Daemons.Online(util.UUIDString(a.DaemonID)),
	}
}

// MessageDTO is the API shape of a chat message.
type MessageDTO struct {
	ID          string          `json:"id"`
	RoomID      string          `json:"room_id"`
	Seq         int64           `json:"seq"`
	SenderType  string          `json:"sender_type"`
	SenderID    string          `json:"sender_id"`
	ClientMsgID string          `json:"client_msg_id"`
	Body        string          `json:"body"`
	Mentions    json.RawMessage `json:"mentions"`
	Card        json.RawMessage `json:"card"`
	Trust       string          `json:"trust"`
	CreatedAt   string          `json:"created_at"`
}

func messageDTO(m db.Message) MessageDTO {
	return MessageDTO{
		ID: util.UUIDString(m.ID), RoomID: util.UUIDString(m.RoomID), Seq: m.Seq, SenderType: m.SenderType,
		SenderID: util.UUIDString(m.SenderID), ClientMsgID: m.ClientMsgID.String, Body: m.Body,
		Mentions: raw(m.Mentions, "[]"), Card: raw(m.Card, "null"), Trust: m.Trust, CreatedAt: util.TS(m.CreatedAt),
	}
}

// PullRequestDTO is the API shape of a PR.
type PullRequestDTO struct {
	ID          string `json:"id"`
	ProjectID   string `json:"project_id"`
	TaskID      string `json:"task_id"`
	Number      int32  `json:"number"`
	Provider    string `json:"provider"`
	URL         string `json:"url"`
	Branch      string `json:"branch"`
	BaseBranch  string `json:"base_branch"`
	HeadCommit  string `json:"head_commit"`
	DiffStat    string `json:"diff_stat"`
	Status      string `json:"status"`
	MergeCommit string `json:"merge_commit"`
	CreatedAt   string `json:"created_at"`
}

func prDTO(p db.PullRequest) PullRequestDTO {
	return PullRequestDTO{util.UUIDString(p.ID), util.UUIDString(p.ProjectID), util.UUIDString(p.TaskID), p.Number, p.Provider, p.Url, p.Branch, p.BaseBranch, p.HeadCommit, p.DiffStat, p.Status, p.MergeCommit, util.TS(p.CreatedAt)}
}

// MemoryDTO is the API shape of a MEMORY.md version.
type MemoryDTO struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Version      int32  `json:"version"`
	Content      string `json:"content"`
	Status       string `json:"status"`
	SourceTaskID string `json:"source_task_id"`
	ProposedBy   string `json:"proposed_by"`
	ConfirmedBy  string `json:"confirmed_by"`
	CreatedAt    string `json:"created_at"`
}

func memoryDTO(m db.MemoryVersion) MemoryDTO {
	return MemoryDTO{util.UUIDString(m.ID), util.UUIDString(m.ProjectID), m.Version, m.Content, m.Status, util.UUIDString(m.SourceTaskID), m.ProposedBy, util.UUIDString(m.ConfirmedBy), util.TS(m.CreatedAt)}
}

// ProjectDTO is the API shape of a project.
type ProjectDTO struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	RemoteURL     string `json:"remote_url"`
	DefaultBranch string `json:"default_branch"`
	Provider      string `json:"provider"`
	LeadID        string `json:"lead_id"`
	RoomID        string `json:"room_id"`
}

func eventDTO(e db.TaskEvent) protocol.TaskEvent {
	return protocol.TaskEvent{TaskID: util.UUIDString(e.TaskID), Seq: e.Seq, Type: e.Type, Payload: raw(e.Payload, "{}"), CreatedAt: util.TS(e.CreatedAt)}
}
