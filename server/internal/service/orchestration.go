package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/internal/taskstate"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Reason codes added in V0.4.
const (
	ReasonWaitingDeps    = "waiting_dependencies"
	ReasonDepFailed      = "dependency_failed"
	ReasonConcurrency    = "concurrency_limit"
	ReasonAwaitingMerge  = "awaiting_merge"
	ReasonDecisionTimout = "decision_timeout"
	ReasonIntakeTimeout  = "intake_timeout"
	ReasonGrantExpired   = "grant_expired"
	ReasonHostOffline    = "host_offline"
	// ReasonDelegationInvalid: a chief emitted a malformed delegation block.
	ReasonDelegationInvalid = "delegation_invalid"
)

// NewTraceID returns a fresh trace id (FR-SRV-602).
func NewTraceID() string { return "tr-" + protocol.NewID()[:24] }

func jsonStrings(b []byte) []string {
	out := []string{}
	_ = util.Unmarshal(b, &out)
	return out
}

// depsState reports whether a task's dependencies block it or failed.
func (s *Service) depsState(ctx context.Context, t db.Task) (blocked bool, failed string) {
	for _, id := range jsonStrings(t.DependsOn) {
		uid, err := util.ParseUUID(id)
		if err != nil {
			continue
		}
		d, err := s.Q.GetTask(ctx, uid)
		if err != nil {
			continue
		}
		switch protocol.TaskStatus(d.Status) {
		case protocol.StatusCompleted:
		case protocol.StatusFailed, protocol.StatusCanceled, protocol.StatusRejected:
			return false, id
		default:
			blocked = true
		}
	}
	return blocked, ""
}

func grantExpired(t db.Task) bool {
	var g protocol.Grant
	_ = util.Unmarshal(t.GrantSpec, &g)
	if g.ExpiresAt == "" {
		return false
	}
	exp, err := time.Parse(time.RFC3339, g.ExpiresAt)
	return err == nil && time.Now().After(exp)
}

// gate decides whether an accepted task may be dispatched now. It returns
// (queueReason, terminalReason): a non-empty queueReason keeps it queued, a
// non-empty terminalReason ends it.
func (s *Service) gate(ctx context.Context, t db.Task) (string, string) {
	if t.Kind != protocol.KindCode {
		return "", ""
	}
	if grantExpired(t) {
		return "", ReasonGrantExpired
	}
	blocked, failed := s.depsState(ctx, t)
	if failed != "" {
		return "", ReasonDepFailed
	}
	if blocked {
		return ReasonWaitingDeps, ""
	}
	a, err := s.Q.GetAgent(ctx, t.TargetAgentID)
	if err == nil && a.MaxConcurrent > 0 {
		n, _ := s.Q.CountRunningForAgent(ctx, a.ID)
		if n >= int64(a.MaxConcurrent) {
			return ReasonConcurrency, ""
		}
	}
	if code := s.budgetBlock(ctx, t); code != "" {
		return code, ""
	}
	return "", ""
}

// queue parks a task with a reason, recording an event once per reason.
func (s *Service) queue(ctx context.Context, t db.Task, reason string) db.Task {
	if t.Status == string(protocol.StatusQueued) && t.ReasonCode == reason {
		return t
	}
	nt, err := s.setStatus(ctx, t, protocol.StatusQueued, reason)
	if err != nil {
		return t
	}
	s.appendEvent(ctx, t.ID, "queued", map[string]any{"reason": reason})
	return nt
}

// onTerminal runs after a task reaches a final state: it wakes dependents,
// frees a concurrency slot and fans out to IM.
func (s *Service) onTerminal(t db.Task) {
	ctx := context.Background()
	deps, _ := s.Q.ListTasksDependingOn(ctx, db.ListTasksDependingOnParams{WorkspaceID: t.WorkspaceID, Column2: util.UUIDString(t.ID)})
	for _, d := range deps {
		if d.Status == string(protocol.StatusQueued) {
			s.dispatch(ctx, d)
		}
	}
	s.releaseSlot(ctx, t)
}

// releaseSlot dispatches the oldest task waiting on the agent's concurrency limit.
func (s *Service) releaseSlot(ctx context.Context, t db.Task) {
	queued, _ := s.Q.ListTasksForDaemonByStatus(ctx, db.ListTasksForDaemonByStatusParams{DaemonID: t.DaemonID, Statuses: []string{string(protocol.StatusQueued)}})
	for _, q := range queued {
		if q.TargetAgentID == t.TargetAgentID && q.ReasonCode == ReasonConcurrency {
			if nt := s.dispatch(ctx, q); nt.Status == string(protocol.StatusWorking) {
				return
			}
		}
	}
}

// subDelegate turns a ```delegation block in a specialist's reply into a
// child delegation (multi-level delegation, PRD 6.3). Agent output is
// untrusted, so it becomes a draft for the agent's owner unless the agent
// runs at "autonomous" level, in which case it is submitted directly; the
// child's grant must be a subset of the parent's either way.
func (s *Service) subDelegate(ctx context.Context, t db.Task, summary string) {
	block, _, ok := ParseDelegationBlock(summary)
	if !ok {
		return
	}
	r, err := s.Q.GetRoom(ctx, t.RoomID)
	if err != nil {
		return
	}
	if t.Depth >= t.MaxDepth {
		_, _ = s.systemCard(ctx, r.ID, "子委托被拒绝（depth_exceeded）："+block.Title, Card{"type": "notice", "code": delegation.CodeDepthExceeded, "task_id": util.UUIDString(t.ID)})
		return
	}
	target, err := s.Q.GetAgentByHandle(ctx, db.GetAgentByHandleParams{WorkspaceID: t.WorkspaceID, Handle: block.Target})
	if err != nil || target.Kind != "specialist" || target.ID == t.TargetAgentID {
		_, _ = s.systemCard(ctx, r.ID, "子委托目标 @"+block.Target+" 无效", Card{"type": "notice", "code": "unknown_target", "task_id": util.UUIDString(t.ID)})
		return
	}
	var pg protocol.Grant
	_ = util.Unmarshal(t.GrantSpec, &pg)
	g := protocol.Grant{Resources: block.Resources, Actions: pg.Actions, Network: []string{}, ExpiresAt: pg.ExpiresAt}
	if len(g.Resources) == 0 {
		g.Resources = pg.Resources
	}
	pb := budgetOf(t)
	remaining := pb.MaxTokens - t.UsedTokens
	b := protocol.Budget{MaxTokens: remaining / 2, MaxDurationMin: pb.MaxDurationMin}
	prompt := nonEmpty(block.Prompt, block.Title)
	owner := util.UUIDString(t.TargetOwnerID)
	a, _ := s.Q.GetAgent(ctx, t.TargetAgentID)
	if a.Autonomy == "autonomous" {
		in := DelegationInput{
			RoomID: util.UUIDString(r.ID), ProjectID: util.UUIDString(r.ProjectID), ParentTaskID: util.UUIDString(t.ID),
			SourceAgentID: util.UUIDString(t.TargetAgentID), TargetAgentID: util.UUIDString(target.ID),
			IdempotencyKey: "sub:" + util.UUIDString(t.ID) + ":" + target.Handle, Title: block.Title, Prompt: prompt,
			Grant: g, Budget: b, Approvers: approversOf(t), MaxDepth: int(t.MaxDepth), TraceID: t.TraceID,
		}
		if _, _, err := s.CreateDelegation(ctx, util.UUIDString(t.WorkspaceID), owner, in); err != nil {
			_, _ = s.systemCard(ctx, r.ID, "子委托失败："+err.Error(), Card{"type": "notice", "code": apperr.As(err).Code, "task_id": util.UUIDString(t.ID)})
		}
		return
	}
	d := Draft{Title: block.Title, Prompt: prompt, Grant: g, Budget: b, Approvers: approversOf(t), MaxDepth: int(t.MaxDepth)}
	src, _ := s.Q.GetMessage(ctx, t.SourceMessageID)
	m, err := s.createDraft(ctx, r, owner, src, target, d)
	if err == nil {
		_, _, _ = s.patchCard(ctx, m.ID, Card{
			"parent_task_id": util.UUIDString(t.ID), "source_agent_id": util.UUIDString(t.TargetAgentID),
			"trace_id": t.TraceID, "proposed_by": "agent", "trust": "untrusted",
		})
	}
}

// ---------- timeouts (PRD 3.2: pause on timeout, never auto-approve) ----------

// RunSweeper periodically applies decision timeouts until ctx ends.
func (s *Service) RunSweeper(ctx context.Context) {
	t := time.NewTicker(s.Cfg.SweepInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.Sweep(ctx)
		}
	}
}

// Sweep applies overdue decision deadlines once.
func (s *Service) Sweep(ctx context.Context) {
	tasks, _ := s.Q.ListOverdueTasks(ctx)
	for _, t := range tasks {
		_ = s.Q.SetTaskDeadline(ctx, db.SetTaskDeadlineParams{ID: t.ID})
		switch protocol.TaskStatus(t.Status) {
		case protocol.StatusPendingIntake:
			nt, err := s.setStatus(ctx, t, protocol.StatusRejected, ReasonIntakeTimeout)
			if err != nil {
				continue
			}
			s.Audit(ctx, t.WorkspaceID, "system", "timeout", "intake.timeout", "task", util.UUIDString(t.ID), nil)
			s.updateIntakeCard(ctx, nt, "rejected", ReasonIntakeTimeout, "system")
			s.notify(ctx, t.WorkspaceID, t.RoomID, "委托接单超时，已自动拒绝："+t.Title)
		default:
			s.appendEvent(ctx, t.ID, "decision_timeout", map[string]any{"status": t.Status})
			s.Audit(ctx, t.WorkspaceID, "system", "timeout", "approval.timeout", "task", util.UUIDString(t.ID), nil)
			_, _ = s.systemCard(ctx, t.RoomID, "审批超时：任务保持暂停，不会自动通过："+t.Title, Card{"type": "notice", "code": ReasonDecisionTimout, "task_id": util.UUIDString(t.ID)})
			s.notify(ctx, t.WorkspaceID, t.RoomID, "审批超时（保持暂停，不自动通过）："+t.Title)
		}
	}
	perms, _ := s.Q.ListOverduePermissions(ctx)
	for _, p := range perms {
		if _, err := s.Q.MarkPermissionTimedOut(ctx, p.ID); err != nil {
			continue
		}
		t, err := s.Q.GetTask(ctx, p.TaskID)
		if err != nil || taskstate.IsTerminal(protocol.TaskStatus(t.Status)) {
			continue
		}
		// The agent is already blocked on the decision; suspend it so it holds
		// no CPU and cannot proceed on its own.
		s.Daemons.Send(util.UUIDString(t.DaemonID), s.frame(t, protocol.TypeTaskPause, protocol.TaskPause{TaskID: util.UUIDString(t.ID), Reason: ReasonDecisionTimout}))
		nt, _ := s.setStatus(ctx, t, protocol.StatusApproval, ReasonDecisionTimout)
		s.appendEvent(ctx, t.ID, "decision_timeout", map[string]any{"permission_id": util.UUIDString(p.ID)})
		s.Audit(ctx, t.WorkspaceID, "system", "timeout", "permission.timeout", "task", util.UUIDString(t.ID), map[string]any{"permission_id": util.UUIDString(p.ID)})
		_, _ = s.systemCard(ctx, nt.RoomID, "权限请求超时：任务已暂停，等待审批人决定（不会自动通过）", Card{"type": "notice", "code": ReasonDecisionTimout, "task_id": util.UUIDString(t.ID)})
		s.notify(ctx, t.WorkspaceID, t.RoomID, "权限请求超时，任务已暂停："+t.Title)
	}
}

func (s *Service) deadline() pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: time.Now().Add(s.Cfg.DecisionTimeout), Valid: true}
}

// frame builds a server→daemon frame that carries the task's trace id.
func (s *Service) frame(t db.Task, typ string, payload any) protocol.Envelope {
	return protocol.MsgTrace(typ, t.TraceID, payload)
}

// ---------- chase (催办, FR-CLI-302) ----------

// Nudge lets any workspace member who is not an eligible approver remind the
// people a task is waiting on. It is rate limited per task and user.
func (s *Service) Nudge(ctx context.Context, wsID, userID, taskID string) (map[string]any, error) {
	t, err := s.getTask(ctx, wsID, taskID)
	if err != nil {
		return nil, err
	}
	var waiting []string
	switch protocol.TaskStatus(t.Status) {
	case protocol.StatusPendingIntake:
		waiting = []string{util.UUIDString(t.TargetOwnerID)}
	case protocol.StatusApproval, protocol.StatusInputRequired:
		for _, a := range approversOf(t) {
			waiting = append(waiting, a.ID)
		}
		if t.ReasonCode != ReasonAwaitingMerge {
			waiting = eligibleForPermission(t)
		}
	default:
		return nil, apperr.Conflict("nudge.not_waiting", "task is not waiting on a decision")
	}
	key := taskID + "/" + userID
	s.nudgeMu.Lock()
	last := s.nudged[key]
	if time.Since(last) < 10*time.Minute {
		s.nudgeMu.Unlock()
		return nil, apperr.Conflict("nudge.too_soon", "already chased in the last 10 minutes")
	}
	s.nudged[key] = time.Now()
	s.nudgeMu.Unlock()
	u, _ := s.Q.GetUser(ctx, util.MustUUID(userID))
	names := []string{}
	for _, id := range waiting {
		if w, err := s.Q.GetUser(ctx, util.MustUUID(id)); err == nil {
			names = append(names, "@"+w.Handle)
		}
	}
	text := fmt.Sprintf("%s 催办：%s 正在等待 %s 处理", u.Name, t.Title, strings.Join(names, " "))
	_, _ = s.systemCard(ctx, t.RoomID, text, Card{"type": "nudge", "task_id": taskID, "by": userID, "waiting_on": waiting})
	s.Audit(ctx, t.WorkspaceID, "user", userID, "task.nudge", "task", taskID, map[string]any{"waiting_on": waiting})
	s.notify(ctx, t.WorkspaceID, t.RoomID, text)
	return map[string]any{"ok": true, "waiting_on": waiting}, nil
}

// ---------- traces (FR-SRV-602) ----------

// TraceView lists everything recorded under one trace id.
type TraceView struct {
	TraceID string               `json:"trace_id"`
	Tasks   []TaskDTO            `json:"tasks"`
	Events  []protocol.TaskEvent `json:"events"`
	PRs     []PullRequestDTO     `json:"pull_requests"`
	Audit   []AuditEntry         `json:"audit"`
}

// GetTrace returns tasks, events, PRs and audit entries for a trace.
func (s *Service) GetTrace(ctx context.Context, wsID, traceID string) (TraceView, error) {
	ws := util.MustUUID(wsID)
	rows, err := s.Q.ListTasksByTrace(ctx, db.ListTasksByTraceParams{WorkspaceID: ws, TraceID: traceID})
	if err != nil {
		return TraceView{}, err
	}
	if len(rows) == 0 {
		return TraceView{}, apperr.ErrNotFound
	}
	v := TraceView{TraceID: traceID, Tasks: []TaskDTO{}, Events: []protocol.TaskEvent{}, PRs: []PullRequestDTO{}, Audit: []AuditEntry{}}
	targets := []string{}
	for _, t := range rows {
		v.Tasks = append(v.Tasks, TaskDTOOf(t))
		targets = append(targets, util.UUIDString(t.ID))
		evs, _ := s.Q.ListTaskEvents(ctx, db.ListTaskEventsParams{TaskID: t.ID, Seq: 0})
		for _, e := range evs {
			v.Events = append(v.Events, eventDTO(e))
		}
		if pr, err := s.Q.GetPullRequestByTask(ctx, t.ID); err == nil {
			v.PRs = append(v.PRs, prDTO(pr))
			targets = append(targets, util.UUIDString(pr.ID))
		}
		if ap, err := s.Q.GetApprovalByTask(ctx, t.ID); err == nil {
			targets = append(targets, util.UUIDString(ap.ID))
		}
	}
	au, _ := s.Q.ListAuditForTargets(ctx, db.ListAuditForTargetsParams{WorkspaceID: ws, Targets: targets})
	for _, a := range au {
		v.Audit = append(v.Audit, auditDTO(a))
	}
	return v, nil
}
