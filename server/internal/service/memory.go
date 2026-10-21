package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
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
	entry := fmt.Sprintf("\n## %s · %s\n- %s\n- task: %s\n", time.Now().Format("2006-01-02"), t.Title, strings.ReplaceAll(summary, "\n", "\n  "), util.UUIDString(t.ID))
	mv, err := s.Q.CreateMemoryVersion(ctx, db.CreateMemoryVersionParams{ProjectID: p.ID, Content: strings.TrimRight(base, "\n") + "\n" + entry, Status: "pending", SourceTaskID: t.ID, ProposedBy: "agent:" + util.UUIDString(t.TargetAgentID)})
	if err != nil {
		s.log.Error("memory propose", "err", err)
		return
	}
	_, _ = s.systemCard(ctx, t.RoomID, fmt.Sprintf("MEMORY.md v%d 建议更新，等待项目负责人确认", mv.Version), Card{
		"type": "memory", "state": "pending", "memory_id": util.UUIDString(mv.ID), "project_id": util.UUIDString(p.ID),
		"version": mv.Version, "lead_id": util.UUIDString(p.LeadID), "addition": entry, "task_id": util.UUIDString(t.ID),
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
	mv, err = s.Q.DecideMemoryVersion(ctx, db.DecideMemoryVersionParams{ID: id, Status: status, ConfirmedBy: util.MustUUID(userID)})
	if err != nil {
		return MemoryDTO{}, apperr.ErrAlreadyDone
	}
	s.Audit(ctx, p.WorkspaceID, "user", userID, "memory."+status, "memory", memoryID, map[string]any{"version": mv.Version})
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
