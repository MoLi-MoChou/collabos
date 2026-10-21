package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// VerifyGitHubSignature checks X-Hub-Signature-256 ("sha256=<hex>").
func VerifyGitHubSignature(secret string, body []byte, header string) bool {
	if secret == "" || !strings.HasPrefix(header, "sha256=") {
		return false
	}
	want, err := hex.DecodeString(strings.TrimPrefix(header, "sha256="))
	if err != nil {
		return false
	}
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return hmac.Equal(m.Sum(nil), want)
}

// SignGitHub computes the X-Hub-Signature-256 header value (tests, tooling).
func SignGitHub(secret string, body []byte) string {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(body)
	return "sha256=" + hex.EncodeToString(m.Sum(nil))
}

type ghPullEvent struct {
	Action      string `json:"action"`
	PullRequest struct {
		Number         int32  `json:"number"`
		Merged         bool   `json:"merged"`
		MergeCommitSHA string `json:"merge_commit_sha"`
		Head           struct {
			Ref string `json:"ref"`
			SHA string `json:"sha"`
		} `json:"head"`
	} `json:"pull_request"`
}

// GitHubWebhook calibrates PR state from GitHub (FR-SRV-303, PRD 6.5:
// on disagreement the Git platform wins). Deliveries are deduplicated.
func (s *Service) GitHubWebhook(ctx context.Context, projectID, event, delivery, signature string, body []byte) (map[string]any, error) {
	pid, err := util.ParseUUID(projectID)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	p, err := s.Q.GetProject(ctx, pid)
	if err != nil {
		return nil, apperr.ErrNotFound
	}
	if !VerifyGitHubSignature(p.WebhookSecret, body, signature) {
		return nil, apperr.Forbidden("webhook.bad_signature", "invalid webhook signature")
	}
	if delivery != "" {
		if _, err := s.Q.InsertWebhookDelivery(ctx, db.InsertWebhookDeliveryParams{DeliveryID: delivery, ProjectID: p.ID, Event: event}); err != nil {
			return map[string]any{"duplicate": true}, nil
		}
	}
	if event == "ping" {
		return map[string]any{"ok": true}, nil
	}
	if event != "pull_request" {
		return map[string]any{"ignored": event}, nil
	}
	var ev ghPullEvent
	if err := json.Unmarshal(body, &ev); err != nil {
		return nil, apperr.BadRequest("invalid payload")
	}
	pr, err := s.Q.GetPullRequestByNumber(ctx, db.GetPullRequestByNumberParams{ProjectID: p.ID, Number: ev.PullRequest.Number})
	if err != nil {
		pr, err = s.Q.GetPullRequestByBranch(ctx, db.GetPullRequestByBranchParams{ProjectID: p.ID, Branch: ev.PullRequest.Head.Ref})
		if err != nil {
			return map[string]any{"ignored": "unknown pull request"}, nil
		}
	}
	t, err := s.Q.GetTask(ctx, pr.TaskID)
	if err != nil {
		return nil, err
	}
	ap, _ := s.Q.GetApprovalByTask(ctx, t.ID)
	apID := util.UUIDString(ap.ID)
	msg, hasCard := s.findCard(ctx, t.RoomID, "merge", "approval_id", apID)
	result := map[string]any{"pr": pr.Number, "action": ev.Action}
	switch {
	case ev.Action == "closed" && ev.PullRequest.Merged:
		if pr.Status == "merged" {
			result["calibrated"] = false
			return result, nil
		}
		_, _ = s.Q.SetPullRequestStatus(ctx, db.SetPullRequestStatusParams{ID: pr.ID, Status: "merged", MergeCommit: ev.PullRequest.MergeCommitSHA})
		if ap.Status == "pending" || ap.Status == "approved" {
			_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: ap.ID, Status: "executed"})
		}
		if t.Status == string(protocol.StatusApproval) || t.Status == string(protocol.StatusUnknown) {
			_, _ = s.setStatus(ctx, t, protocol.StatusCompleted, "merged_on_platform")
		}
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "merged", "merge_commit": ev.PullRequest.MergeCommitSHA, "calibrated_by": "webhook"})
		}
		result["calibrated"] = true
	case ev.Action == "closed":
		if pr.Status != "open" {
			return result, nil
		}
		_, _ = s.Q.SetPullRequestStatus(ctx, db.SetPullRequestStatusParams{ID: pr.ID, Status: "closed"})
		if ap.Status == "pending" {
			_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: ap.ID, Status: "stale"})
		}
		if t.Status == string(protocol.StatusApproval) {
			_, _ = s.setStatus(ctx, t, protocol.StatusCanceled, "pr_closed_on_platform")
		}
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "rejected", "comment": "PR 已在 GitHub 上关闭", "calibrated_by": "webhook"})
		}
		result["calibrated"] = true
	case ev.Action == "synchronize" && ev.PullRequest.Head.SHA != "" && ev.PullRequest.Head.SHA != ap.ApprovedCommit && ap.Status == "pending":
		// New commits after the gate opened: the approval no longer covers the head.
		_, _ = s.Q.SetApprovalStatus(ctx, db.SetApprovalStatusParams{ID: ap.ID, Status: "stale"})
		_, _ = s.setStatus(ctx, t, protocol.StatusFailed, "commit_mismatch")
		if hasCard {
			_, _, _ = s.patchCard(ctx, msg.ID, Card{"state": "failed", "error": "PR 分支有新提交，审批作废", "calibrated_by": "webhook"})
		}
		result["calibrated"] = true
	}
	s.Audit(ctx, p.WorkspaceID, "system", "webhook", "pr.webhook", "pull_request", util.UUIDString(pr.ID), map[string]any{"event": event, "action": ev.Action, "delivery": delivery, "calibrated": result["calibrated"]})
	s.dock(p.WorkspaceID, "tasks")
	return result, nil
}
