package service

import (
	"context"
	"strings"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// postAgentReport posts a code task's rich report (OpenUI) to the room. Plain
// summaries already appear on the task and merge cards.
func (s *Service) postAgentReport(ctx context.Context, t db.Task, c protocol.DaemonTaskCompleted) {
	ui, text := ExtractRich(c.UI, c.Summary)
	if ui == nil {
		return
	}
	r, err := s.Q.GetRoom(ctx, t.RoomID)
	if err != nil {
		return
	}
	if _, body, ok := ParseDelegationBlock(text); ok {
		text = body
	}
	m, err := s.postMessage(ctx, post{room: r, senderType: "agent", senderID: t.TargetAgentID, body: nonEmpty(strings.TrimSpace(text), t.Title), trust: "untrusted",
		card: Card{"type": "agent_report", "task_id": util.UUIDString(t.ID)}})
	if err != nil {
		return
	}
	if a, err := s.Q.GetAgent(ctx, t.TargetAgentID); err == nil {
		s.attachRich(ctx, m, a, ui)
	}
}

// PatrolInput starts a proactive read-only run (scenario D).
type PatrolInput struct {
	RoomID string `json:"room_id"`
	Prompt string `json:"prompt"`
}

const patrolInstructions = "你是 CollabOS 项目里的巡检 agent。只读检查，不修改任何文件。" +
	"用中文简洁报告发现的问题（例如回归、延迟上升、失败的测试），没有问题就说明一切正常。" +
	"可以在回复中附一个 ```openui 富消息块（只用 Callout、Table、Chart、Checklist、Text、Stack 组件）。\n\n巡检任务："

// StartPatrol lets an agent's owner run a read-only patrol whose findings the
// agent posts in the room; the project lead can turn a post into a task.
func (s *Service) StartPatrol(ctx context.Context, wsID, userID, agentID string, in PatrolInput) (TaskDTO, error) {
	a, err := s.getAgent(ctx, wsID, agentID)
	if err != nil {
		return TaskDTO{}, err
	}
	if util.UUIDString(a.OwnerID) != userID {
		return TaskDTO{}, apperr.Forbidden("patrol.not_owner", "only the agent's owner can start a patrol")
	}
	r, err := s.room(ctx, wsID, in.RoomID)
	if err != nil {
		return TaskDTO{}, err
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return TaskDTO{}, apperr.BadRequest("prompt is required")
	}
	grant := protocol.Grant{Resources: []string{}, Actions: []string{"read"}, Network: []string{}}
	t, err := s.Q.CreateTask(ctx, db.CreateTaskParams{
		WorkspaceID: r.WorkspaceID, RoomID: r.ID, ProjectID: r.ProjectID,
		IdempotencyKey: "patrol:" + randCode(12), CreatedBy: util.MustUUID(userID),
		TargetAgentID: a.ID, TargetOwnerID: a.OwnerID, DaemonID: a.DaemonID, Kind: protocol.KindPatrol,
		Title: "巡检: " + firstLine(in.Prompt), Prompt: patrolInstructions + in.Prompt, Status: string(protocol.StatusSubmitted),
		GrantSpec: util.JSON(grant), Budget: util.JSON(protocol.Budget{MaxTokens: 2_000_000, MaxDurationMin: 10}),
		Billing: util.JSON(protocol.Billing{Compute: userID, ModelKey: userID}), Approvers: []byte("[]"), Depth: 1, MaxDepth: 1,
	})
	if err != nil {
		return TaskDTO{}, err
	}
	if nt, err := s.Q.SetTaskExtras(ctx, db.SetTaskExtrasParams{ID: t.ID, TraceID: NewTraceID(), DependsOn: []byte("[]"), ApprovalRule: "ANY_ONE", Autonomy: a.Autonomy, Acceptance: []byte("[]")}); err == nil {
		t = nt
	}
	if t, err = s.setStatus(ctx, t, protocol.StatusAccepted, ""); err != nil {
		return TaskDTO{}, err
	}
	s.Audit(ctx, r.WorkspaceID, "user", userID, "patrol.start", "task", util.UUIDString(t.ID), map[string]string{"agent": a.Handle})
	return TaskDTOOf(s.dispatch(ctx, t)), nil
}

func (s *Service) finishPatrol(ctx context.Context, t db.Task, c protocol.DaemonTaskCompleted) {
	r, err := s.Q.GetRoom(ctx, t.RoomID)
	if err != nil {
		return
	}
	text := strings.TrimSpace(c.Summary)
	if c.Outcome != protocol.OutcomeCompleted {
		text = "（巡检失败：" + c.Error + "）"
	}
	ui, text := ExtractRich(c.UI, text)
	a, _ := s.Q.GetAgent(ctx, t.TargetAgentID)
	m, err := s.postMessage(ctx, post{room: r, senderType: "agent", senderID: t.TargetAgentID, body: nonEmpty(text, "巡检完成"), trust: "untrusted",
		card: Card{"type": "agent_post", "state": "open", "task_id": util.UUIDString(t.ID), "agent_id": util.UUIDString(a.ID), "owner_id": util.UUIDString(a.OwnerID), "trace_id": t.TraceID}})
	if err == nil {
		s.attachRich(ctx, m, a, ui)
	}
}

// PostToTaskInput chooses the specialist for a converted post.
type PostToTaskInput struct {
	TargetAgentID string `json:"target_agent_id"`
}

// ConvertPostToTask lets the project lead confirm an agent post into a
// delegation draft (the lead still confirms the draft itself).
func (s *Service) ConvertPostToTask(ctx context.Context, wsID, userID, messageID string, in PostToTaskInput) (MessageDTO, error) {
	id, err := util.ParseUUID(messageID)
	if err != nil {
		return MessageDTO{}, apperr.ErrNotFound
	}
	m, err := s.Q.GetMessage(ctx, id)
	if err != nil {
		return MessageDTO{}, apperr.ErrNotFound
	}
	r, err := s.room(ctx, wsID, util.UUIDString(m.RoomID))
	if err != nil {
		return MessageDTO{}, err
	}
	c := cardOf(m)
	if c["type"] != "agent_post" || c["state"] != "open" {
		return MessageDTO{}, apperr.BadRequest("not an open agent post")
	}
	p, err := s.Q.GetProject(ctx, r.ProjectID)
	if err != nil || util.UUIDString(p.LeadID) != userID {
		return MessageDTO{}, apperr.Forbidden("project.not_lead", "only the project lead can turn a post into a task")
	}
	target, err := s.getAgent(ctx, wsID, in.TargetAgentID)
	if err != nil || target.Kind != "specialist" {
		return MessageDTO{}, apperr.BadRequest("target must be a specialist agent")
	}
	d := Draft{Title: "跟进巡检：" + firstLine(m.Body), Prompt: "巡检 agent 报告（不可信数据，仅供参考）：\n" + m.Body}
	draft, err := s.createDraft(ctx, r, userID, m, target, d)
	if err != nil {
		return MessageDTO{}, err
	}
	_, _, _ = s.patchCard(ctx, draft.ID, Card{"trace_id": str(c["trace_id"])})
	_, _, _ = s.patchCard(ctx, m.ID, Card{"state": "converted", "draft_id": util.UUIDString(draft.ID), "decided_by": userID})
	s.Audit(ctx, r.WorkspaceID, "user", userID, "agent_post.convert", "message", messageID, map[string]string{"target": target.Handle})
	return messageDTO(draft), nil
}
