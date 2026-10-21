package service

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

var inFlight = []string{string(protocol.StatusWorking), string(protocol.StatusRunningOffline), string(protocol.StatusUnknown), string(protocol.StatusApproval)}

// DaemonConnected records capabilities, reconciles in-flight tasks and
// redispatches queued ones. It returns the last acknowledged seq.
func (s *Service) DaemonConnected(ctx context.Context, d db.Daemon, hs protocol.ClientHandshake) int64 {
	id := util.UUIDString(d.ID)
	s.graceMu.Lock()
	if t := s.grace[id]; t != nil {
		t.Stop()
		delete(s.grace, id)
	}
	s.graceMu.Unlock()
	if hs.Capabilities != nil {
		_ = s.Q.SetDaemonCapabilities(ctx, db.SetDaemonCapabilitiesParams{ID: d.ID, Capabilities: util.JSON(hs.Capabilities)})
	}
	_ = s.Q.SetDaemonStatus(ctx, db.SetDaemonStatusParams{ID: d.ID, Status: "online"})
	s.broadcast(d.WorkspaceID, protocol.TypeDaemonState, protocol.DaemonState{DaemonID: id, Status: "online"})
	s.Audit(ctx, d.WorkspaceID, "daemon", id, "daemon.connect", "daemon", id, map[string]any{"active": hs.ActiveTasks, "journaled": hs.JournaledTasks})

	known := map[string]bool{}
	for _, t := range hs.ActiveTasks {
		known[t] = true
	}
	journaled := map[string]bool{}
	for _, t := range hs.JournaledTasks {
		journaled[t] = true
	}
	tasks, _ := s.Q.ListTasksForDaemonByStatus(ctx, db.ListTasksForDaemonByStatusParams{DaemonID: d.ID, Statuses: inFlight})
	for _, t := range tasks {
		tid := util.UUIDString(t.ID)
		switch {
		case known[tid]:
			if t.Status == string(protocol.StatusRunningOffline) || t.Status == string(protocol.StatusUnknown) {
				_, _ = s.setStatus(ctx, t, protocol.StatusWorking, "")
				s.appendEvent(ctx, t.ID, "reconnected", map[string]any{"daemon_id": id})
			}
			s.resendDecisions(ctx, t)
		case journaled[tid]:
			// the replayed journal will deliver the outcome
		case t.Status == string(protocol.StatusApproval):
			// waiting on a merge decision; nothing runs on the daemon
		default:
			_, _ = s.setStatus(ctx, t, protocol.StatusFailed, "daemon_lost")
			s.appendEvent(ctx, t.ID, "daemon_lost", map[string]any{"daemon_id": id})
		}
	}
	queued, _ := s.Q.ListTasksForDaemonByStatus(ctx, db.ListTasksForDaemonByStatusParams{DaemonID: d.ID, Statuses: []string{string(protocol.StatusQueued)}})
	for _, t := range queued {
		s.dispatch(ctx, t)
	}
	s.dock(d.WorkspaceID, "devices")
	fresh, err := s.Q.GetDaemon(ctx, d.ID)
	if err != nil {
		return d.LastAckedSeq
	}
	return fresh.LastAckedSeq
}

// DaemonDisconnected moves working tasks to running_offline and starts the
// offline grace timer, after which they become unknown.
func (s *Service) DaemonDisconnected(d db.Daemon) {
	ctx := context.Background()
	id := util.UUIDString(d.ID)
	_ = s.Q.SetDaemonStatus(ctx, db.SetDaemonStatusParams{ID: d.ID, Status: "offline"})
	s.broadcast(d.WorkspaceID, protocol.TypeDaemonState, protocol.DaemonState{DaemonID: id, Status: "offline"})
	s.Audit(ctx, d.WorkspaceID, "daemon", id, "daemon.disconnect", "daemon", id, nil)
	tasks, _ := s.Q.ListTasksForDaemonByStatus(ctx, db.ListTasksForDaemonByStatusParams{DaemonID: d.ID, Statuses: []string{string(protocol.StatusWorking)}})
	for _, t := range tasks {
		_, _ = s.setStatus(ctx, t, protocol.StatusRunningOffline, "")
	}
	s.graceMu.Lock()
	defer s.graceMu.Unlock()
	if old := s.grace[id]; old != nil {
		old.Stop()
	}
	s.grace[id] = time.AfterFunc(s.Cfg.OfflineGrace, func() {
		s.graceMu.Lock()
		delete(s.grace, id)
		s.graceMu.Unlock()
		if s.Daemons.Online(id) {
			return
		}
		ts, _ := s.Q.ListTasksForDaemonByStatus(context.Background(), db.ListTasksForDaemonByStatusParams{DaemonID: d.ID, Statuses: []string{string(protocol.StatusRunningOffline)}})
		for _, t := range ts {
			_, _ = s.setStatus(context.Background(), t, protocol.StatusUnknown, "offline_grace_expired")
		}
	})
	s.dock(d.WorkspaceID, "devices")
}

func (s *Service) frameLock(id string) *sync.Mutex {
	m, _ := s.frameMu.LoadOrStore(id, &sync.Mutex{})
	return m.(*sync.Mutex)
}

// HandleDaemonFrame processes one daemon frame and returns replies. Frames
// carrying seq come from the daemon's journal: already-acknowledged seqs are
// re-acked without reprocessing, so replay after reconnect is idempotent.
func (s *Service) HandleDaemonFrame(ctx context.Context, d db.Daemon, e protocol.Envelope) []protocol.Envelope {
	if e.Type == protocol.TypeHeartbeatPing {
		_ = s.Q.TouchDaemon(ctx, d.ID)
		return []protocol.Envelope{protocol.Msg(protocol.TypeHeartbeatPong, map[string]any{})}
	}
	l := s.frameLock(util.UUIDString(d.ID))
	l.Lock()
	defer l.Unlock()
	if e.Seq > 0 {
		cur, err := s.Q.GetDaemon(ctx, d.ID)
		if err == nil && e.Seq <= cur.LastAckedSeq {
			return []protocol.Envelope{protocol.Msg(protocol.TypeAck, protocol.Ack{AckSeq: cur.LastAckedSeq})}
		}
	}
	s.process(ctx, d, e)
	if e.Seq > 0 {
		acked, err := s.Q.AdvanceDaemonAck(ctx, db.AdvanceDaemonAckParams{ID: d.ID, LastAckedSeq: e.Seq})
		if err == nil {
			return []protocol.Envelope{protocol.Msg(protocol.TypeAck, protocol.Ack{AckSeq: acked})}
		}
	}
	return nil
}

func (s *Service) daemonTask(ctx context.Context, d db.Daemon, taskID string) (db.Task, bool) {
	id, err := util.ParseUUID(taskID)
	if err != nil {
		return db.Task{}, false
	}
	t, err := s.Q.GetTask(ctx, id)
	if err != nil || t.DaemonID != d.ID {
		s.log.Warn("frame for foreign task", "daemon", util.UUIDString(d.ID), "task", taskID)
		return db.Task{}, false
	}
	return t, true
}

func (s *Service) process(ctx context.Context, d db.Daemon, e protocol.Envelope) {
	switch e.Type {
	case protocol.TypeDaemonProgressStream:
		var p protocol.DaemonProgress
		if e.Into(&p) != nil {
			return
		}
		t, ok := s.daemonTask(ctx, d, p.TaskID)
		if !ok {
			return
		}
		if p.Kind == protocol.ProgressUsage && p.Usage != nil {
			s.applyUsage(ctx, t, p.Usage)
			return
		}
		s.appendEvent(ctx, t.ID, "progress", p)
	case protocol.TypeDaemonPolicyBlocked:
		var p protocol.PolicyBlocked
		if e.Into(&p) != nil {
			return
		}
		if t, ok := s.daemonTask(ctx, d, p.TaskID); ok {
			s.appendEvent(ctx, t.ID, "policy_blocked", p)
			s.Audit(ctx, t.WorkspaceID, "daemon", util.UUIDString(d.ID), "policy.blocked", "task", p.TaskID, p)
		}
	case protocol.TypePermissionRequest:
		var p protocol.PermissionRequest
		if e.Into(&p) != nil {
			return
		}
		if t, ok := s.daemonTask(ctx, d, p.TaskID); ok {
			s.permissionEscalated(ctx, t, p)
		}
	case protocol.TypeDaemonTaskCompleted:
		var c protocol.DaemonTaskCompleted
		if e.Into(&c) != nil {
			return
		}
		if t, ok := s.daemonTask(ctx, d, c.TaskID); ok {
			s.taskCompleted(ctx, t, c)
		}
	case protocol.TypeMergeResult:
		var m protocol.MergeResult
		if e.Into(&m) != nil {
			return
		}
		s.mergeResult(ctx, d, m)
	default:
		s.log.Warn("unknown daemon frame", "type", e.Type)
	}
}

func budgetOf(t db.Task) protocol.Budget {
	var b protocol.Budget
	_ = util.Unmarshal(t.Budget, &b)
	return b
}

// applyUsage records cumulative token usage and enforces the budget.
func (s *Service) applyUsage(ctx context.Context, t db.Task, u *protocol.Usage) bool {
	total := u.TotalTokens
	if total == 0 {
		total = u.InputTokens + u.OutputTokens + u.ThoughtTokens
	}
	if total > t.UsedTokens {
		t, _ = s.Q.AddTaskUsage(ctx, db.AddTaskUsageParams{ID: t.ID, UsedTokens: total})
		s.broadcast(t.WorkspaceID, protocol.TypeTaskUpdated, protocol.TaskUpdated{TaskID: util.UUIDString(t.ID), Status: protocol.TaskStatus(t.Status)})
	}
	b := budgetOf(t)
	if b.MaxTokens > 0 && t.UsedTokens > b.MaxTokens {
		if nt, err := s.setStatus(ctx, t, protocol.StatusFailed, delegation.CodeBudgetExceeded); err == nil {
			s.Daemons.Send(util.UUIDString(t.DaemonID), protocol.Msg(protocol.TypeTaskCancel, protocol.TaskCancel{TaskID: util.UUIDString(t.ID), Reason: delegation.CodeBudgetExceeded}))
			s.Audit(ctx, t.WorkspaceID, "system", "budget", "task.budget_exceeded", "task", util.UUIDString(t.ID), map[string]any{"used": t.UsedTokens, "max": b.MaxTokens})
			_, _ = s.systemCard(ctx, nt.RoomID, "任务超出预算已终止："+nt.Title, Card{"type": "notice", "code": delegation.CodeBudgetExceeded, "task_id": util.UUIDString(t.ID)})
		}
		return true
	}
	return false
}

func (s *Service) taskCompleted(ctx context.Context, t db.Task, c protocol.DaemonTaskCompleted) {
	s.appendEvent(ctx, t.ID, "completed", c)
	result := util.JSON(c)
	if c.Usage != nil && s.applyUsage(ctx, t, c.Usage) {
		_, _ = s.Q.SetTaskResult(ctx, db.SetTaskResultParams{ID: t.ID, Status: string(protocol.StatusFailed), Result: result})
		return
	}
	t, _ = s.Q.GetTask(ctx, t.ID)
	if t.Status == string(protocol.StatusCanceled) || t.Status == string(protocol.StatusFailed) {
		return
	}
	if t.Kind == protocol.KindChat {
		s.finishChiefRun(ctx, t, c)
		to := protocol.StatusCompleted
		if c.Outcome != protocol.OutcomeCompleted {
			to = protocol.StatusFailed
		}
		if nt, err := s.setStatus(ctx, t, to, c.ReasonCode); err == nil {
			_, _ = s.Q.SetTaskResult(ctx, db.SetTaskResultParams{ID: nt.ID, Status: nt.Status, Result: result})
		}
		return
	}
	switch {
	case c.Outcome == protocol.OutcomeCompleted && c.Commit != "" && c.Pushed:
		nt, err := s.setStatus(ctx, t, protocol.StatusApproval, "")
		if err != nil {
			return
		}
		_, _ = s.Q.SetTaskResult(ctx, db.SetTaskResultParams{ID: nt.ID, Status: nt.Status, Result: result})
		s.openPullRequest(ctx, nt, c)
	case c.Outcome == protocol.OutcomeCompleted:
		nt, err := s.setStatus(ctx, t, protocol.StatusCompleted, "")
		if err != nil {
			return
		}
		_, _ = s.Q.SetTaskResult(ctx, db.SetTaskResultParams{ID: nt.ID, Status: nt.Status, Result: result})
		_, _ = s.systemCard(ctx, nt.RoomID, "任务完成（无代码改动）："+nt.Title, Card{"type": "task", "task_id": util.UUIDString(nt.ID)})
		s.proposeMemory(ctx, nt, c.Summary)
	default:
		to := protocol.StatusFailed
		if c.Outcome == protocol.OutcomeCanceled {
			to = protocol.StatusCanceled
		}
		nt, err := s.setStatus(ctx, t, to, c.ReasonCode)
		if err != nil {
			return
		}
		_, _ = s.Q.SetTaskResult(ctx, db.SetTaskResultParams{ID: nt.ID, Status: nt.Status, Result: result})
		_, _ = s.systemCard(ctx, nt.RoomID, "任务失败："+nt.Title+"："+c.Error, Card{"type": "notice", "code": nonEmpty(c.ReasonCode, "failed"), "task_id": util.UUIDString(nt.ID)})
	}
	s.dock(t.WorkspaceID, "tasks")
}

func nonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func approversOf(t db.Task) []protocol.Approver {
	var a []protocol.Approver
	_ = json.Unmarshal(t.Approvers, &a)
	return a
}
