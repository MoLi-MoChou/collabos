package service

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// openPullRequest records the PR the executing daemon pushed and opens a
// named-approver merge gate. The server never holds Git credentials: the
// merge itself runs on the approver's daemon.
func (s *Service) openPullRequest(ctx context.Context, t db.Task, c protocol.DaemonTaskCompleted) {
	p, err := s.Q.GetProject(ctx, t.ProjectID)
	if err != nil {
		return
	}
	url := c.PRURL
	if p.Provider == "local" || url == "" {
		url = fmt.Sprintf("local:%s#%s", p.RemoteUrl, c.Branch)
	}
	base := nonEmpty(c.BaseBranch, p.DefaultBranch)
	pr, err := s.Q.CreatePullRequest(ctx, db.CreatePullRequestParams{
		WorkspaceID: t.WorkspaceID, ProjectID: p.ID, TaskID: t.ID, Provider: p.Provider, Url: url,
		Branch: c.Branch, BaseBranch: base, HeadCommit: c.Commit, DiffStat: c.DiffStat,
	})
	if err != nil {
		s.log.Error("create pr", "err", err)
		return
	}
	approvers := approversOf(t)
	ids := make([]string, 0, len(approvers))
	for _, a := range approvers {
		ids = append(ids, a.ID)
	}
	ap, err := s.Q.CreateApproval(ctx, db.CreateApprovalParams{TaskID: t.ID, PrID: pr.ID, Action: "merge", Rule: "named", Eligible: util.JSON(ids), ApprovedCommit: c.Commit})
	if err != nil {
		s.log.Error("create approval", "err", err)
		return
	}
	s.appendEvent(ctx, t.ID, "pr_opened", prDTO(pr))
	s.Audit(ctx, t.WorkspaceID, "daemon", util.UUIDString(t.DaemonID), "pr.open", "pull_request", util.UUIDString(pr.ID), map[string]any{"branch": c.Branch, "commit": c.Commit})
	_, _ = s.systemCard(ctx, t.RoomID, fmt.Sprintf("PR #%d 等待合并审批：%s", pr.Number, t.Title), Card{
		"type": "merge", "state": "pending", "task_id": util.UUIDString(t.ID), "approval_id": util.UUIDString(ap.ID),
		"pr_id": util.UUIDString(pr.ID), "number": pr.Number, "branch": pr.Branch, "base_branch": pr.BaseBranch,
		"commit": pr.HeadCommit, "diff_stat": pr.DiffStat, "url": pr.Url, "eligible": ids, "approvers": approvers,
		"summary": c.Summary,
	})
	s.dock(t.WorkspaceID, "tasks")
}

// MergeDecisionInput is a named approver's decision.
type MergeDecisionInput struct {
	Decision string `json:"decision"`
	Comment  string `json:"comment"`
}

// DecideMerge records an approver's decision. Approval asks one of the
// approver's own online daemons (which has the repo) to perform the merge.
func (s *Service) DecideMerge(ctx context.Context, wsID, userID, approvalID string, in MergeDecisionInput) (TaskDTO, error) {
	id, err := util.ParseUUID(approvalID)
	if err != nil {
		return TaskDTO{}, apperr.ErrNotFound
	}
	ap, err := s.Q.GetApproval(ctx, id)
	if err != nil {
		return TaskDTO{}, apperr.ErrNotFound
	}
	t, err := s.getTask(ctx, wsID, util.UUIDString(ap.TaskID))
	if err != nil {
		return TaskDTO{}, err
	}
	var eligible []string
	_ = json.Unmarshal(ap.Eligible, &eligible)
	if !contains(eligible, userID) {
		s.Audit(ctx, t.WorkspaceID, "user", userID, "merge.denied_not_eligible", "approval", approvalID, nil)
		return TaskDTO{}, apperr.Forbidden("approval.not_eligible", "only the named approvers can decide this merge")
	}
	pr, err := s.Q.GetPullRequest(ctx, ap.PrID)
	if err != nil {
		return TaskDTO{}, apperr.ErrNotFound
	}
	msg, hasCard := s.findCard(ctx, t.RoomID, "merge", "approval_id", approvalID)
	switch in.Decision {
	case "reject":
		if _, err := s.Q.DecideApproval(ctx, db.DecideApprovalParams{ID: id, Status: "rejected", DecidedBy: util.MustUUID(userID), Comment: in.Comment}); err != nil {
			return TaskDTO{}, apperr.ErrAlreadyDone
		}
		_, _ = s.Q.SetPullRequestMerged(ctx, db.SetPullRequestMergedParams{ID: pr.ID, Status: "closed"})
		nt, _ := s.setStatus(ctx, t, protocol.StatusCanceled, "merge_rejected")
		s.Audit(ctx, t.WorkspaceID, "user", userID, "merge.reject", "pull_request", util.UUIDString(pr.ID), map[string]any{"comment": in.Comment})
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "rejected", "decided_by": userID, "comment": in.Comment})
		}
		s.dock(t.WorkspaceID, "tasks")
		return TaskDTOOf(nt), nil
	case "approve":
	default:
		return TaskDTO{}, apperr.BadRequest("decision must be approve or reject")
	}
	p, err := s.Q.GetProject(ctx, t.ProjectID)
	if err != nil {
		return TaskDTO{}, err
	}
	executor, ok := s.approverDaemon(ctx, t.WorkspaceID, userID, p.RemoteUrl)
	if !ok {
		return TaskDTO{}, apperr.Conflict("approval.no_daemon", "approve from a device: none of your online daemons has this repository (collab repo add)")
	}
	if _, err := s.Q.DecideApproval(ctx, db.DecideApprovalParams{ID: id, Status: "approved", DecidedBy: util.MustUUID(userID), Comment: in.Comment, ExecutorDaemonID: executor.ID}); err != nil {
		return TaskDTO{}, apperr.ErrAlreadyDone
	}
	s.Audit(ctx, t.WorkspaceID, "user", userID, "merge.approve", "pull_request", util.UUIDString(pr.ID), map[string]any{"approved_commit": ap.ApprovedCommit, "executor": util.UUIDString(executor.ID)})
	s.Daemons.Send(util.UUIDString(executor.ID), protocol.Msg(protocol.TypeMergeExecute, protocol.MergeExecute{
		ApprovalID: approvalID, TaskID: util.UUIDString(t.ID), RemoteURL: p.RemoteUrl, Provider: p.Provider,
		Branch: pr.Branch, BaseBranch: pr.BaseBranch, ApprovedCommit: ap.ApprovedCommit, PRNumber: pr.Number,
		Message: fmt.Sprintf("Merge PR #%d: %s (CollabOS task %s)", pr.Number, t.Title, util.UUIDString(t.ID)),
	}))
	if hasCard {
		_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "merging", "decided_by": userID})
	}
	return TaskDTOOf(t), nil
}

func (s *Service) approverDaemon(ctx context.Context, ws pgtype.UUID, userID, remote string) (db.Daemon, bool) {
	rows, err := s.Q.ListDaemonsForOwner(ctx, db.ListDaemonsForOwnerParams{WorkspaceID: ws, OwnerID: util.MustUUID(userID)})
	if err != nil {
		return db.Daemon{}, false
	}
	for _, d := range rows {
		if !s.Daemons.Online(util.UUIDString(d.ID)) {
			continue
		}
		for _, r := range daemonCaps(d).Repos {
			if r.RemoteURL == remote {
				return d, true
			}
		}
	}
	return db.Daemon{}, false
}

// mergeResult verifies the merged head is exactly the approved commit
// before recording the merge and completing the task.
func (s *Service) mergeResult(ctx context.Context, d db.Daemon, m protocol.MergeResult) {
	id, err := util.ParseUUID(m.ApprovalID)
	if err != nil {
		return
	}
	ap, err := s.Q.GetApproval(ctx, id)
	if err != nil || ap.ExecutorDaemonID != d.ID || ap.Status != "approved" {
		s.log.Warn("unexpected merge result", "approval", m.ApprovalID)
		return
	}
	t, err := s.Q.GetTask(ctx, ap.TaskID)
	if err != nil {
		return
	}
	pr, _ := s.Q.GetPullRequest(ctx, ap.PrID)
	msg, hasCard := s.findCard(ctx, t.RoomID, "merge", "approval_id", m.ApprovalID)
	if m.HeadCommit != "" && m.HeadCommit != ap.ApprovedCommit {
		_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: id, Status: "stale"})
		nt, _ := s.setStatus(ctx, t, protocol.StatusFailed, "commit_mismatch")
		s.Audit(ctx, t.WorkspaceID, "daemon", util.UUIDString(d.ID), "merge.commit_mismatch", "pull_request", util.UUIDString(pr.ID), m)
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "failed", "error": "分支头与已批准的提交不一致，拒绝合并"})
		}
		_ = nt
		return
	}
	if !m.OK {
		_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: id, Status: "pending"})
		s.Audit(ctx, t.WorkspaceID, "daemon", util.UUIDString(d.ID), "merge.failed", "pull_request", util.UUIDString(pr.ID), m)
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "pending", "error": m.Error})
		}
		return
	}
	_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: id, Status: "executed"})
	pr, _ = s.Q.SetPullRequestMerged(ctx, db.SetPullRequestMergedParams{ID: pr.ID, Status: "merged", MergeCommit: m.MergeCommit})
	nt, err := s.setStatus(ctx, t, protocol.StatusCompleted, "")
	if err != nil {
		return
	}
	s.appendEvent(ctx, t.ID, "merged", map[string]any{"merge_commit": m.MergeCommit, "approved_commit": ap.ApprovedCommit})
	s.Audit(ctx, t.WorkspaceID, "daemon", util.UUIDString(d.ID), "merge.executed", "pull_request", util.UUIDString(pr.ID), map[string]any{"merge_commit": m.MergeCommit, "verified_commit": m.HeadCommit})
	if hasCard {
		_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "merged", "merge_commit": m.MergeCommit})
	}
	var res protocol.DaemonTaskCompleted
	_ = util.Unmarshal(nt.Result, &res)
	s.proposeMemory(ctx, nt, res.Summary)
	s.dock(t.WorkspaceID, "tasks")
}

// ListPullRequests lists PRs.
func (s *Service) ListPullRequests(ctx context.Context, wsID string) ([]PullRequestDTO, error) {
	rows, err := s.Q.ListPullRequests(ctx, util.MustUUID(wsID))
	if err != nil {
		return nil, err
	}
	out := make([]PullRequestDTO, 0, len(rows))
	for _, p := range rows {
		out = append(out, prDTO(p))
	}
	return out, nil
}
