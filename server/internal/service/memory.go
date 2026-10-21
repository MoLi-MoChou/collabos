package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// proposeMemory creates a pending MEMORY.md version that the project lead
// must confirm before it is used as trusted context.
func (s *Service) proposeMemory(ctx context.Context, t db.Task, summary string) {
	p, err := s.Q.GetProject(ctx, t.ProjectID)
	if err != nil {
		return
	}
	base := ""
	if cur, err := s.Q.LatestConfirmedMemory(ctx, p.ID); err == nil {
		base = cur.Content
	}
	summary = strings.TrimSpace(summary)
	if r := []rune(summary); len(r) > 600 {
		summary = string(r[:600]) + "…"
	}
	if summary == "" {
		summary = "（无摘要）"
	}
	entry := s.handover(ctx, t, summary)
	mv, err := s.Q.CreateMemoryVersion(ctx, db.CreateMemoryVersionParams{ProjectID: p.ID, Content: strings.TrimRight(base, "\n") + "\n" + entry, Status: "pending", SourceTaskID: t.ID, ProposedBy: "agent:" + util.UUIDString(t.TargetAgentID)})
	if err != nil {
		s.log.Error("memory propose", "err", err)
		return
	}
	// Knowledge from another owner's bot is "external_bot"; own bots are "auto_generated" (PRD 11.4).
	trust := "auto_generated"
	if t.CreatedBy != t.TargetOwnerID {
		trust = "external_bot"
	}
	_ = s.Q.SetMemoryTrust(ctx, db.SetMemoryTrustParams{ID: mv.ID, Trust: trust})
	_, _ = s.systemCard(ctx, t.RoomID, fmt.Sprintf("MEMORY.md v%d 建议更新，等待项目负责人确认", mv.Version), Card{
		"type": "memory", "state": "pending", "memory_id": util.UUIDString(mv.ID), "project_id": util.UUIDString(p.ID),
		"version": mv.Version, "lead_id": util.UUIDString(p.LeadID), "addition": entry, "task_id": util.UUIDString(t.ID),
		"trust": trust, "trace_id": t.TraceID,
	})
	s.dock(t.WorkspaceID, "memory")
}

// ListMemory returns all MEMORY.md versions of a project.
func (s *Service) ListMemory(ctx context.Context, wsID, projectID string) ([]MemoryDTO, error) {
	pid, err := util.ParseUUID(projectID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, pid)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return nil, apperr.ErrNotFound
	}
	rows, err := s.Q.ListMemoryVersions(ctx, pid)
	if err != nil {
		return nil, err
	}
	out := make([]MemoryDTO, 0, len(rows))
	for _, m := range rows {
		out = append(out, memoryDTO(m))
	}
	return out, nil
}

// MemoryDecisionInput is the lead's decision.
type MemoryDecisionInput struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// DecideMemory lets the project lead confirm or reject a suggestion.
func (s *Service) DecideMemory(ctx context.Context, wsID, userID, memoryID string, in MemoryDecisionInput) (MemoryDTO, error) {
	id, err := util.ParseUUID(memoryID)
	if err != nil {
		return MemoryDTO{}, apperr.ErrNotFound
	}
	mv, err := s.Q.GetMemoryVersion(ctx, id)
	if err != nil {
		return MemoryDTO{}, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, mv.ProjectID)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return MemoryDTO{}, apperr.ErrNotFound
	}
	if util.UUIDString(p.LeadID) != userID {
		return MemoryDTO{}, apperr.Forbidden("memory.not_lead", "only the project lead can confirm MEMORY.md")
	}
	status := map[string]string{"confirm": "confirmed", "reject": "rejected"}[in.Decision]
	if status == "" {
		return MemoryDTO{}, apperr.BadRequest("decision must be confirm or reject")
	}
	if status == "rejected" && strings.TrimSpace(in.Comment) == "" {
		return MemoryDTO{}, apperr.ErrReasonRequired
	}
	mv, err = s.Q.DecideMemoryVersion(ctx, db.DecideMemoryVersionParams{ID: id, Status: status, ConfirmedBy: util.MustUUID(userID)})
	if err != nil {
		return MemoryDTO{}, apperr.ErrAlreadyDone
	}
	if status == "confirmed" {
		_ = s.Q.SetMemoryTrust(ctx, db.SetMemoryTrustParams{ID: mv.ID, Trust: "confirmed"})
		mv.Trust = "confirmed"
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "memory."+status, "memory", memoryID, map[string]any{"version": mv.Version, "comment": in.Comment})
	rooms, _ := s.Q.ListRooms(ctx, p.WorkspaceID)
	for _, r := range rooms {
		if r.ProjectID == p.ID {
			if m, ok := s.findCard(ctx, r.ID, "memory", "memory_id", memoryID); ok {
				_, _, _ = s.patchCard(ctx, m.ID, Card{"state": status, "decided_by": userID})
			}
		}
	}
	s.dock(p.WorkspaceID, "memory")
	return memoryDTO(mv), nil
}

// handover builds the structured handover summary (FR-SRV-501): goal,
// outcome, changes, tests, PR and trace, pending the lead's confirmation.
func (s *Service) handover(ctx context.Context, t db.Task, summary string) string {
	var res protocol.DaemonTaskCompleted
	_ = util.Unmarshal(t.Result, &res)
	var b strings.Builder
	fmt.Fprintf(&b, "\n## %s · %s\n", time.Now().Format("2006-01-02"), t.Title)
	fmt.Fprintf(&b, "- 结论：%s\n", strings.ReplaceAll(summary, "\n", "\n  "))
	if a, err := s.Q.GetAgent(ctx, t.TargetAgentID); err == nil {
		fmt.Fprintf(&b, "- 执行：@%s\n", a.Handle)
	}
	if st := strings.TrimSpace(res.DiffStat); st != "" {
		lines := strings.Split(st, "\n")
		fmt.Fprintf(&b, "- 改动：%s\n", strings.TrimSpace(lines[len(lines)-1]))
	}
	if res.Tests != nil {
		state := "通过"
		if !res.Tests.Passed {
			state = "失败"
		}
		fmt.Fprintf(&b, "- 测试：`%s` %s\n", res.Tests.Command, state)
	}
	if pr, err := s.Q.GetPullRequestByTask(ctx, t.ID); err == nil {
		fmt.Fprintf(&b, "- PR：#%d %s（%s）\n", pr.Number, pr.Url, pr.Status)
	}
	fmt.Fprintf(&b, "- task: %s · trace: %s\n", util.UUIDString(t.ID), t.TraceID)
	return b.String()
}

// MemoryEditInput is a manual MEMORY.md edit.
type MemoryEditInput struct {
	Content string `json:"content"`
}

// EditMemory proposes a manual edit. The lead's own edit is confirmed at
// once; anyone else's waits for the lead.
func (s *Service) EditMemory(ctx context.Context, wsID, userID, projectID string, in MemoryEditInput) (MemoryDTO, error) {
	pid, err := util.ParseUUID(projectID)
	if err != nil {
		return MemoryDTO{}, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, pid)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return MemoryDTO{}, apperr.ErrNotFound
	}
	role := s.projectRole(ctx, p, userID)
	if role == "" || role == RoleViewer {
		return MemoryDTO{}, apperr.Forbidden("project.read_only", "you cannot edit this project's memory")
	}
	if strings.TrimSpace(in.Content) == "" || len(in.Content) > 200_000 {
		return MemoryDTO{}, apperr.BadRequest("content is required (max 200 KB)")
	}
	status := "pending"
	if role == RoleLead {
		status = "confirmed"
	}
	mv, err := s.Q.CreateMemoryVersion(ctx, db.CreateMemoryVersionParams{ProjectID: p.ID, Content: in.Content, Status: status, ProposedBy: "user:" + userID})
	if err != nil {
		return MemoryDTO{}, err
	}
	if status == "confirmed" {
		mv, _ = s.Q.DecideMemoryVersion(ctx, db.DecideMemoryVersionParams{ID: mv.ID, Status: "confirmed", ConfirmedBy: util.MustUUID(userID)})
		_ = s.Q.SetMemoryTrust(ctx, db.SetMemoryTrustParams{ID: mv.ID, Trust: "confirmed"})
		mv.Status, mv.Trust = "confirmed", "confirmed"
	} else {
		_ = s.Q.SetMemoryTrust(ctx, db.SetMemoryTrustParams{ID: mv.ID, Trust: "user_input"})
		if r, ok := s.projectRoom(ctx, p); ok {
			_, _ = s.systemCard(ctx, r.ID, fmt.Sprintf("MEMORY.md v%d 手动修改，等待项目负责人确认", mv.Version), Card{
				"type": "memory", "state": "pending", "memory_id": util.UUIDString(mv.ID), "project_id": projectID,
				"version": mv.Version, "lead_id": util.UUIDString(p.LeadID), "addition": "（整篇手动编辑）", "trust": "user_input",
			})
		}
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "memory.edit", "memory", util.UUIDString(mv.ID), map[string]any{"version": mv.Version, "status": status})
	s.dock(p.WorkspaceID, "memory")
	return memoryDTO(mv), nil
}

func (s *Service) projectRoom(ctx context.Context, p db.Project) (db.Room, bool) {
	rooms, _ := s.Q.ListRooms(ctx, p.WorkspaceID)
	for _, r := range rooms {
		if r.ProjectID == p.ID {
			return r, true
		}
	}
	return db.Room{}, false
}

// TaintInput marks polluted knowledge.
type TaintInput struct {
	Reason string `json:"reason"`
}

// TaintReport lists what a tainted memory version reached.
type TaintReport struct {
	Versions []int32  `json:"versions"`
	Tasks    []string `json:"tasks"`
}

// TaintMemory marks a version and every later version derived from it as
// polluted, then flags every task whose context used any of them (PRD 11.4).
// Tainted versions are never served as context again.
func (s *Service) TaintMemory(ctx context.Context, wsID, userID, memoryID string, in TaintInput) (TaintReport, error) {
	id, err := util.ParseUUID(memoryID)
	if err != nil {
		return TaintReport{}, apperr.ErrNotFound
	}
	mv, err := s.Q.GetMemoryVersion(ctx, id)
	if err != nil {
		return TaintReport{}, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, mv.ProjectID)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return TaintReport{}, apperr.ErrNotFound
	}
	if util.UUIDString(p.LeadID) != userID {
		return TaintReport{}, apperr.Forbidden("memory.not_lead", "only the project lead can mark knowledge as polluted")
	}
	if strings.TrimSpace(in.Reason) == "" {
		return TaintReport{}, apperr.ErrReasonRequired
	}
	all, err := s.Q.ListMemoryVersions(ctx, p.ID)
	if err != nil {
		return TaintReport{}, err
	}
	rep := TaintReport{Versions: []int32{}, Tasks: []string{}}
	refs := []string{}
	for _, v := range all {
		if v.Version < mv.Version {
			continue
		}
		_, _ = s.Q.SetMemoryTaint(ctx, db.SetMemoryTaintParams{ID: v.ID, Tainted: true, TaintReason: in.Reason})
		rep.Versions = append(rep.Versions, v.Version)
		refs = append(refs, util.UUIDString(v.ID))
	}
	rows, _ := s.Q.ListContextRefsByRef(ctx, db.ListContextRefsByRefParams{Kind: "memory", Refs: refs})
	seen := map[string]bool{}
	for _, r := range rows {
		tid := util.UUIDString(r.TaskID)
		if seen[tid] {
			continue
		}
		seen[tid] = true
		_ = s.Q.SetTaskTainted(ctx, db.SetTaskTaintedParams{ID: r.TaskID, Tainted: true})
		s.appendEvent(ctx, r.TaskID, "tainted", map[string]any{"memory_id": memoryID, "reason": in.Reason})
		rep.Tasks = append(rep.Tasks, tid)
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "memory.taint", "memory", memoryID, map[string]any{"reason": in.Reason, "versions": rep.Versions, "tasks": rep.Tasks})
	if r, ok := s.projectRoom(ctx, p); ok {
		_, _ = s.systemCard(ctx, r.ID, fmt.Sprintf("MEMORY.md v%d 起被标记为污染：%s（影响 %d 个任务）", mv.Version, in.Reason, len(rep.Tasks)), Card{
			"type": "notice", "code": "memory_tainted", "memory_id": memoryID, "versions": rep.Versions, "tasks": rep.Tasks,
		})
	}
	s.dock(p.WorkspaceID, "memory")
	s.dock(p.WorkspaceID, "tasks")
	return rep, nil
}

// MemoryReferences lists the tasks whose context used a memory version.
func (s *Service) MemoryReferences(ctx context.Context, wsID, memoryID string) ([]TaskDTO, error) {
	id, err := util.ParseUUID(memoryID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	mv, err := s.Q.GetMemoryVersion(ctx, id)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, mv.ProjectID)
	if err != nil || util.UUIDString(p.WorkspaceID) != wsID {
		return nil, apperr.ErrNotFound
	}
	rows, err := s.Q.ListContextRefsByRef(ctx, db.ListContextRefsByRefParams{Kind: "memory", Refs: []string{memoryID}})
	if err != nil {
		return nil, err
	}
	out := []TaskDTO{}
	for _, r := range rows {
		if t, err := s.Q.GetTask(ctx, r.TaskID); err == nil {
			out = append(out, TaskDTOOf(t))
		}
	}
	return out, nil
}
