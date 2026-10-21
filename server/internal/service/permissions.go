package service

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/approval"
	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// eligibleForPermission are the humans who may decide an in-flight
// permission request: the resource owner (target agent's owner) and the requester.
func eligibleForPermission(t db.Task) []string {
	out := []string{util.UUIDString(t.TargetOwnerID)}
	if t.CreatedBy != t.TargetOwnerID {
		out = append(out, util.UUIDString(t.CreatedBy))
	}
	return out
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (s *Service) permissionEscalated(ctx context.Context, t db.Task, p protocol.PermissionRequest) {
	code := delegation.CodeScopeExceeded
	switch p.Layer {
	case "action":
		code = delegation.CodeActionForbidden
	case "autonomy":
		code = "autonomy_supervised"
	}
	rels := make([]string, 0, len(p.Paths))
	for _, x := range p.Paths {
		rels = append(rels, approval.Rel(p.Worktree, x))
	}
	pr, err := s.Q.CreatePermissionRequest(ctx, db.CreatePermissionRequestParams{
		TaskID: t.ID, RequestID: p.RequestID, Kind: p.Kind, Title: p.Title, Paths: util.JSON(rels),
		Command: p.Command, ReasonCode: code, Status: "pending",
	})
	if err != nil {
		s.log.Error("permission insert", "err", err)
		return
	}
	if pr.Status != "pending" {
		return
	}
	layer := nonEmpty(p.Layer, "scope")
	_ = s.Q.SetPermissionMeta(ctx, db.SetPermissionMetaParams{ID: pr.ID, Layer: layer, TimeoutAt: s.deadline()})
	if t.ReasonCode != delegation.CodeBudgetExceeded {
		if _, err := s.setStatus(ctx, t, protocol.StatusApproval, code); err != nil {
			s.log.Warn("permission status", "err", err)
		}
	}
	s.appendEvent(ctx, t.ID, "permission_requested", map[string]any{"permission_id": util.UUIDString(pr.ID), "kind": p.Kind, "paths": rels, "command": p.Command, "reason_code": code})
	s.Audit(ctx, t.WorkspaceID, "agent", util.UUIDString(t.TargetAgentID), "permission.escalate", "task", util.UUIDString(t.ID), map[string]any{"paths": rels, "kind": p.Kind, "code": code})
	_, _ = s.systemCard(ctx, t.RoomID, "需要审批：agent 请求超出授权范围的操作", Card{
		"type": "permission", "state": "pending", "task_id": util.UUIDString(t.ID), "permission_id": util.UUIDString(pr.ID),
		"kind": p.Kind, "title": p.Title, "paths": rels, "command": p.Command, "reason_code": code,
		"eligible": eligibleForPermission(t), "layer": layer, "rule": "ANY_ONE",
		// FR-CLI-301: who the request comes from and how far to trust it.
		"source_agent_id": util.UUIDString(t.TargetAgentID), "trust": "untrusted", "trace_id": t.TraceID,
	})
	s.notify(ctx, t.WorkspaceID, t.RoomID, "需要审批：agent 请求 "+p.Title)
}

// PermissionDecisionInput is a human decision on a permission card.
type PermissionDecisionInput struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// DecidePermission records a human decision and relays it to the daemon.
// Approving a scope_exceeded request widens the task grant by exactly the
// requested paths.
func (s *Service) DecidePermission(ctx context.Context, wsID, userID, permID string, in PermissionDecisionInput) (TaskDTO, error) {
	id, err := util.ParseUUID(permID)
	if err != nil {
		return TaskDTO{}, apperr.ErrNotFound
	}
	pr, err := s.Q.GetPermissionRequest(ctx, id)
	if err != nil {
		return TaskDTO{}, apperr.ErrNotFound
	}
	t, err := s.getTask(ctx, wsID, util.UUIDString(pr.TaskID))
	if err != nil {
		return TaskDTO{}, err
	}
	if !contains(eligibleForPermission(t), userID) {
		return TaskDTO{}, apperr.Forbidden("permission.not_eligible", "you are not an eligible approver for this request")
	}
	if in.Decision != "allow" && in.Decision != "deny" {
		return TaskDTO{}, apperr.BadRequest("decision must be allow or deny")
	}
	if in.Decision == "deny" && strings.TrimSpace(in.Comment) == "" {
		return TaskDTO{}, apperr.ErrReasonRequired
	}
	status := "approved"
	if in.Decision == "deny" {
		status = "denied"
	}
	pr, err = s.Q.DecidePermissionRequest(ctx, db.DecidePermissionRequestParams{ID: id, Status: status, DecidedBy: util.MustUUID(userID), Comment: in.Comment})
	if err != nil {
		return TaskDTO{}, apperr.ErrAlreadyDone
	}
	var grant protocol.Grant
	_ = util.Unmarshal(t.GrantSpec, &grant)
	var paths []string
	_ = json.Unmarshal(pr.Paths, &paths)
	if status == "approved" {
		grant = approval.Widen(grant, paths, approval.ActionFor(pr.Kind))
		t, _ = s.Q.UpdateTaskGrant(ctx, db.UpdateTaskGrantParams{ID: t.ID, GrantSpec: util.JSON(grant)})
	}
	s.sendDecision(t, pr, grant)
	s.appendEvent(ctx, t.ID, "permission_decided", map[string]any{"permission_id": permID, "decision": in.Decision, "by": userID})
	s.Audit(ctx, t.WorkspaceID, "user", userID, "permission."+status, "task", util.UUIDString(t.ID), map[string]any{"permission_id": permID, "paths": paths, "comment": in.Comment})
	s.patchPermissionCard(ctx, t, permID, status, userID)
	if t.ReasonCode == ReasonDecisionTimout {
		// The run was suspended at the timeout; let it continue with the decision.
		s.Daemons.Send(util.UUIDString(t.DaemonID), s.frame(t, protocol.TypeTaskResume, protocol.TaskPause{TaskID: util.UUIDString(t.ID)}))
	}
	pending, _ := s.Q.ListPendingPermissions(ctx, t.ID)
	if len(pending) == 0 && t.Status == string(protocol.StatusApproval) && t.ReasonCode != delegation.CodeBudgetExceeded {
		to := protocol.StatusWorking
		if !s.Daemons.Online(util.UUIDString(t.DaemonID)) {
			to = protocol.StatusRunningOffline
		}
		t, _ = s.setStatus(ctx, t, to, "")
	}
	return TaskDTOOf(t), nil
}

func (s *Service) sendDecision(t db.Task, pr db.PermissionRequest, grant protocol.Grant) {
	dec := protocol.DecisionAllow
	code := ""
	if pr.Status == "denied" {
		dec, code = protocol.DecisionDeny, pr.ReasonCode
	}
	s.Daemons.Send(util.UUIDString(t.DaemonID), protocol.Msg(protocol.TypePermissionDecision, protocol.PermissionDecision{
		RequestID: pr.RequestID, TaskID: util.UUIDString(t.ID), Decision: dec, ReasonCode: code, Reason: pr.Comment,
		DecidedBy: util.UUIDString(pr.DecidedBy), Grant: &grant,
	}))
}

// resendDecisions re-delivers decisions for a reconnected daemon's live
// task; the daemon ignores request ids it no longer waits on.
func (s *Service) resendDecisions(ctx context.Context, t db.Task) {
	rows, _ := s.Q.ListDecidedPermissions(ctx, t.ID)
	var grant protocol.Grant
	_ = util.Unmarshal(t.GrantSpec, &grant)
	for _, pr := range rows {
		s.sendDecision(t, pr, grant)
	}
}

func (s *Service) patchPermissionCard(ctx context.Context, t db.Task, permID, state, by string) {
	msgs, err := s.Q.ListMessages(ctx, db.ListMessagesParams{RoomID: t.RoomID, Seq: 0, Limit: 100000})
	if err != nil {
		return
	}
	for _, m := range msgs {
		c := cardOf(m)
		if c["type"] == "permission" && c["permission_id"] == permID {
			_, _, _ = s.patchCard(ctx, m.ID, Card{"state": state, "decided_by": by})
		}
	}
}

func (s *Service) findCard(ctx context.Context, roomID pgtype.UUID, typ, key, val string) (db.Message, bool) {
	msgs, err := s.Q.ListMessages(ctx, db.ListMessagesParams{RoomID: roomID, Seq: 0, Limit: 100000})
	if err != nil {
		return db.Message{}, false
	}
	for i := len(msgs) - 1; i >= 0; i-- {
		c := cardOf(msgs[i])
		if c["type"] == typ && c[key] == val {
			return msgs[i], true
		}
	}
	return db.Message{}, false
}
