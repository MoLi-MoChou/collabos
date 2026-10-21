package service

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/delegation"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
	"github.com/collabos/collabos/server/pkg/protocol"
)

// Budget scopes (FR-SRV-601): workspace (team), user (payer of the model key)
// and the per-delegation budget stored on the task.
const (
	ScopeWorkspace = "workspace"
	ScopeUser      = "user"
)

func monthStart(now time.Time) pgtype.Timestamptz {
	u := now.UTC()
	return pgtype.Timestamptz{Time: time.Date(u.Year(), u.Month(), 1, 0, 0, 0, 0, time.UTC), Valid: true}
}

func period(now time.Time) string { return now.UTC().Format("2006-01") }

func billingOf(t db.Task) protocol.Billing {
	var b protocol.Billing
	_ = util.Unmarshal(t.Billing, &b)
	return b
}

// modelKeyBy is "source" when the requester's key pays, else "target".
func modelKeyBy(t db.Task) string {
	if billingOf(t).ModelKey == util.UUIDString(t.CreatedBy) && t.CreatedBy != t.TargetOwnerID {
		return "source"
	}
	return "target"
}

// payer is the user whose model key pays for the task.
func payer(t db.Task) string {
	if modelKeyBy(t) == "source" {
		return util.UUIDString(t.CreatedBy)
	}
	return util.UUIDString(t.TargetOwnerID)
}

// recordUsage appends a ledger row; known=false records "unknown" usage
// rather than estimating (FR-DAE-503).
func (s *Service) recordUsage(ctx context.Context, t db.Task, delta int64, known bool) {
	if delta <= 0 && known {
		return
	}
	err := s.Q.InsertUsage(ctx, db.InsertUsageParams{
		WorkspaceID: t.WorkspaceID, TaskID: t.ID, RequesterID: t.CreatedBy, OwnerID: t.TargetOwnerID,
		AgentID: t.TargetAgentID, Tokens: delta, Known: known, ComputeBy: "target", ModelKeyBy: modelKeyBy(t),
	})
	if err != nil {
		s.log.Error("usage ledger", "err", err)
	}
}

type budgetState struct {
	Budget db.Budget
	Used   int64
}

// budgetsFor returns workspace and payer budgets that apply to t.
func (s *Service) budgetsFor(ctx context.Context, t db.Task) []budgetState {
	rows, err := s.Q.ListBudgets(ctx, t.WorkspaceID)
	if err != nil {
		return nil
	}
	since := monthStart(time.Now())
	out := []budgetState{}
	for _, b := range rows {
		switch b.Scope {
		case ScopeWorkspace:
			used, _ := s.Q.SumUsageWorkspaceSince(ctx, db.SumUsageWorkspaceSinceParams{WorkspaceID: t.WorkspaceID, CreatedAt: since})
			out = append(out, budgetState{b, used})
		case ScopeUser:
			if util.UUIDString(b.SubjectID) != payer(t) {
				continue
			}
			used, _ := s.Q.SumUsagePayerSince(ctx, db.SumUsagePayerSinceParams{WorkspaceID: t.WorkspaceID, RequesterID: b.SubjectID, CreatedAt: since})
			out = append(out, budgetState{b, used})
		}
	}
	return out
}

// budgetBlock returns budget_exceeded when a team or payer budget is used up.
func (s *Service) budgetBlock(ctx context.Context, t db.Task) string {
	for _, b := range s.budgetsFor(ctx, t) {
		if b.Budget.MaxTokens > 0 && b.Used >= b.Budget.MaxTokens {
			return delegation.CodeBudgetExceeded
		}
	}
	return ""
}

// checkBudgets warns at 90% and pauses at 100% for task, payer and team budgets.
func (s *Service) checkBudgets(ctx context.Context, t db.Task) {
	b := budgetOf(t)
	id := util.UUIDString(t.ID)
	if b.MaxTokens > 0 {
		if t.UsedTokens >= b.MaxTokens {
			s.pauseForBudget(ctx, t, "task", t.UsedTokens, b.MaxTokens)
			return
		}
		if t.UsedTokens*10 >= b.MaxTokens*9 {
			if _, loaded := s.warned.LoadOrStore(id, struct{}{}); !loaded {
				s.budgetWarn(ctx, t, "task", t.UsedTokens, b.MaxTokens)
			}
		}
	}
	now := time.Now()
	for _, st := range s.budgetsFor(ctx, t) {
		max := st.Budget.MaxTokens
		if max <= 0 {
			continue
		}
		if st.Used >= max {
			s.pauseForBudget(ctx, t, st.Budget.Scope, st.Used, max)
			return
		}
		if st.Used*10 >= max*9 && st.Budget.WarnedPeriod != period(now) {
			_ = s.Q.SetBudgetWarned(ctx, db.SetBudgetWarnedParams{ID: st.Budget.ID, WarnedPeriod: period(now)})
			s.budgetWarn(ctx, t, st.Budget.Scope, st.Used, max)
		}
	}
}

func scopeLabel(scope string) string {
	return map[string]string{"task": "委托预算", ScopeWorkspace: "团队预算", ScopeUser: "个人预算"}[scope]
}

func (s *Service) budgetWarn(ctx context.Context, t db.Task, scope string, used, max int64) {
	text := fmt.Sprintf("%s已用 %d%%（%d / %d tokens）：%s", scopeLabel(scope), used*100/max, used, max, t.Title)
	s.appendEvent(ctx, t.ID, "budget_warning", map[string]any{"scope": scope, "used": used, "max": max})
	s.Audit(ctx, t.WorkspaceID, "system", "budget", "budget.warn", "task", util.UUIDString(t.ID), map[string]any{"scope": scope, "used": used, "max": max})
	_, _ = s.systemCard(ctx, t.RoomID, text, Card{"type": "notice", "code": "budget_warning", "task_id": util.UUIDString(t.ID), "scope": scope, "used": used, "max": max})
	s.notify(ctx, t.WorkspaceID, t.RoomID, text)
}

// pauseForBudget suspends a running task at 100%: the daemon SIGSTOPs the
// agent and the task waits in approval_required for a budget expansion.
func (s *Service) pauseForBudget(ctx context.Context, t db.Task, scope string, used, max int64) {
	if t.ReasonCode == delegation.CodeBudgetExceeded {
		return
	}
	switch protocol.TaskStatus(t.Status) {
	case protocol.StatusWorking, protocol.StatusRunningOffline, protocol.StatusApproval:
	default:
		return
	}
	nt, err := s.setStatus(ctx, t, protocol.StatusApproval, delegation.CodeBudgetExceeded)
	if err != nil {
		return
	}
	s.Daemons.Send(util.UUIDString(t.DaemonID), s.frame(t, protocol.TypeTaskPause, protocol.TaskPause{TaskID: util.UUIDString(t.ID), Reason: delegation.CodeBudgetExceeded}))
	s.Audit(ctx, t.WorkspaceID, "system", "budget", "task.budget_paused", "task", util.UUIDString(t.ID), map[string]any{"scope": scope, "used": used, "max": max})
	eligible := budgetApprovers(nt)
	text := fmt.Sprintf("%s用尽，任务已暂停，等待审批扩容：%s", scopeLabel(scope), t.Title)
	_, _ = s.systemCard(ctx, t.RoomID, text, Card{
		"type": "budget", "state": "pending", "task_id": util.UUIDString(t.ID), "scope": scope,
		"used": used, "max": max, "eligible": eligible,
	})
	s.notify(ctx, t.WorkspaceID, t.RoomID, text)
}

// budgetApprovers are the requester and the payer.
func budgetApprovers(t db.Task) []string {
	out := []string{util.UUIDString(t.CreatedBy)}
	if p := payer(t); p != out[0] {
		out = append(out, p)
	}
	return out
}

// BudgetExpansion raises a paused task's budget.
type BudgetExpansion struct {
	MaxTokens int64  `json:"max_tokens"`
	Comment   string `json:"comment"`
}

// ExpandTaskBudget approves a budget expansion and resumes the task.
func (s *Service) ExpandTaskBudget(ctx context.Context, wsID, userID, taskID string, in BudgetExpansion) (TaskDTO, error) {
	t, err := s.getTask(ctx, wsID, taskID)
	if err != nil {
		return TaskDTO{}, err
	}
	if !contains(budgetApprovers(t), userID) {
		return TaskDTO{}, apperr.Forbidden("budget.not_eligible", "only the requester or the payer can expand this budget")
	}
	b := budgetOf(t)
	if in.MaxTokens <= b.MaxTokens || in.MaxTokens <= t.UsedTokens {
		return TaskDTO{}, apperr.BadRequest("max_tokens must exceed the current budget and usage")
	}
	b.MaxTokens = in.MaxTokens
	t, err = s.Q.UpdateTaskBudget(ctx, db.UpdateTaskBudgetParams{ID: t.ID, Budget: util.JSON(b)})
	if err != nil {
		return TaskDTO{}, err
	}
	s.warned.Delete(taskID)
	s.Audit(ctx, t.WorkspaceID, "user", userID, "budget.expand", "task", taskID, map[string]any{"max_tokens": in.MaxTokens, "comment": in.Comment})
	if m, ok := s.findCard(ctx, t.RoomID, "budget", "task_id", taskID); ok {
		_, _, _ = s.patchCard(ctx, m.ID, Card{"state": "approved", "decided_by": userID, "max": in.MaxTokens})
	}
	return TaskDTOOf(s.resumeIfPaused(ctx, t)), nil
}

// resumeIfPaused continues a budget-paused task once no budget blocks it.
func (s *Service) resumeIfPaused(ctx context.Context, t db.Task) db.Task {
	if t.Status != string(protocol.StatusApproval) || t.ReasonCode != delegation.CodeBudgetExceeded {
		return t
	}
	b := budgetOf(t)
	if (b.MaxTokens > 0 && t.UsedTokens >= b.MaxTokens) || s.budgetBlock(ctx, t) != "" {
		return t
	}
	reason, to := "", protocol.StatusWorking
	if pending, _ := s.Q.ListPendingPermissions(ctx, t.ID); len(pending) > 0 {
		to, reason = protocol.StatusApproval, delegation.CodeScopeExceeded
	} else if !s.Daemons.Online(util.UUIDString(t.DaemonID)) {
		to = protocol.StatusRunningOffline
	}
	nt, err := s.setStatus(ctx, t, to, reason)
	if err != nil {
		return t
	}
	s.Daemons.Send(util.UUIDString(t.DaemonID), s.frame(t, protocol.TypeTaskResume, protocol.TaskPause{TaskID: util.UUIDString(t.ID)}))
	s.appendEvent(ctx, t.ID, "resumed", map[string]any{"reason": "budget_expanded"})
	return nt
}

// BudgetInput sets a team or personal monthly token budget.
type BudgetInput struct {
	Scope     string `json:"scope"`
	SubjectID string `json:"subject_id"`
	MaxTokens int64  `json:"max_tokens"`
}

// BudgetDTO is a budget with its month-to-date usage.
type BudgetDTO struct {
	ID        string `json:"id"`
	Scope     string `json:"scope"`
	SubjectID string `json:"subject_id"`
	MaxTokens int64  `json:"max_tokens"`
	Used      int64  `json:"used"`
	Period    string `json:"period"`
}

// SetBudget upserts a budget. Team budgets need the workspace owner; a
// personal budget is set by that user. Raising a budget resumes paused tasks.
func (s *Service) SetBudget(ctx context.Context, wsID, userID string, in BudgetInput) (BudgetDTO, error) {
	ws := util.MustUUID(wsID)
	var subject pgtype.UUID
	switch in.Scope {
	case ScopeWorkspace:
		role, _ := s.RequireMember(ctx, wsID, userID)
		if role != "owner" {
			return BudgetDTO{}, apperr.Forbidden("budget.not_owner", "only the workspace owner can set the team budget")
		}
	case ScopeUser:
		if in.SubjectID == "" {
			in.SubjectID = userID
		}
		if in.SubjectID != userID {
			return BudgetDTO{}, apperr.Forbidden("budget.not_self", "a personal budget can only be set by that user")
		}
		subject = util.MustUUID(userID)
	default:
		return BudgetDTO{}, apperr.BadRequest("scope must be workspace or user")
	}
	if in.MaxTokens <= 0 {
		return BudgetDTO{}, apperr.BadRequest("max_tokens must be positive")
	}
	b, err := s.Q.UpsertBudget(ctx, db.UpsertBudgetParams{WorkspaceID: ws, Scope: in.Scope, SubjectID: subject, MaxTokens: in.MaxTokens})
	if err != nil {
		return BudgetDTO{}, err
	}
	s.Audit(ctx, ws, "user", userID, "budget.set", "budget", util.UUIDString(b.ID), in)
	paused, _ := s.Q.ListTasksByStatusForWorkspace(ctx, db.ListTasksByStatusForWorkspaceParams{WorkspaceID: ws, Statuses: []string{string(protocol.StatusApproval), string(protocol.StatusQueued)}})
	for _, t := range paused {
		switch {
		case t.ReasonCode == delegation.CodeBudgetExceeded && t.Status == string(protocol.StatusApproval):
			s.resumeIfPaused(ctx, t)
		case t.ReasonCode == delegation.CodeBudgetExceeded:
			s.dispatch(ctx, t)
		}
	}
	s.dock(ws, "usage")
	list, _ := s.ListBudgets(ctx, wsID)
	for _, x := range list {
		if x.ID == util.UUIDString(b.ID) {
			return x, nil
		}
	}
	return BudgetDTO{}, nil
}

// ListBudgets lists budgets with month-to-date usage.
func (s *Service) ListBudgets(ctx context.Context, wsID string) ([]BudgetDTO, error) {
	ws := util.MustUUID(wsID)
	rows, err := s.Q.ListBudgets(ctx, ws)
	if err != nil {
		return nil, err
	}
	since := monthStart(time.Now())
	out := make([]BudgetDTO, 0, len(rows))
	for _, b := range rows {
		var used int64
		if b.Scope == ScopeWorkspace {
			used, _ = s.Q.SumUsageWorkspaceSince(ctx, db.SumUsageWorkspaceSinceParams{WorkspaceID: ws, CreatedAt: since})
		} else {
			used, _ = s.Q.SumUsagePayerSince(ctx, db.SumUsagePayerSinceParams{WorkspaceID: ws, RequesterID: b.SubjectID, CreatedAt: since})
		}
		out = append(out, BudgetDTO{ID: util.UUIDString(b.ID), Scope: b.Scope, SubjectID: util.UUIDString(b.SubjectID), MaxTokens: b.MaxTokens, Used: used, Period: period(time.Now())})
	}
	return out, nil
}

// UsageRow is one line of the cost view.
type UsageRow struct {
	SubjectID string `json:"subject_id"`
	Tokens    int64  `json:"tokens"`
	Tasks     int64  `json:"tasks"`
}

// UsageView is the cost view (双方可见的记账, PRD 6.4).
type UsageView struct {
	Period      string      `json:"period"`
	Total       int64       `json:"total"`
	ByRequester []UsageRow  `json:"by_requester"`
	ByOwner     []UsageRow  `json:"by_owner"`
	ByAgent     []UsageRow  `json:"by_agent"`
	Budgets     []BudgetDTO `json:"budgets"`
}

// Usage returns month-to-date usage grouped by requester, owner and agent.
func (s *Service) Usage(ctx context.Context, wsID string) (UsageView, error) {
	ws := util.MustUUID(wsID)
	since := monthStart(time.Now())
	v := UsageView{Period: period(time.Now()), ByRequester: []UsageRow{}, ByOwner: []UsageRow{}, ByAgent: []UsageRow{}}
	v.Total, _ = s.Q.SumUsageWorkspaceSince(ctx, db.SumUsageWorkspaceSinceParams{WorkspaceID: ws, CreatedAt: since})
	if rows, err := s.Q.UsageByRequester(ctx, db.UsageByRequesterParams{WorkspaceID: ws, CreatedAt: since}); err == nil {
		for _, r := range rows {
			v.ByRequester = append(v.ByRequester, UsageRow{util.UUIDString(r.SubjectID), r.Tokens, r.Tasks})
		}
	}
	if rows, err := s.Q.UsageByOwner(ctx, db.UsageByOwnerParams{WorkspaceID: ws, CreatedAt: since}); err == nil {
		for _, r := range rows {
			v.ByOwner = append(v.ByOwner, UsageRow{util.UUIDString(r.SubjectID), r.Tokens, r.Tasks})
		}
	}
	if rows, err := s.Q.UsageByAgent(ctx, db.UsageByAgentParams{WorkspaceID: ws, CreatedAt: since}); err == nil {
		for _, r := range rows {
			v.ByAgent = append(v.ByAgent, UsageRow{util.UUIDString(r.SubjectID), r.Tokens, r.Tasks})
		}
	}
	var err error
	v.Budgets, err = s.ListBudgets(ctx, wsID)
	return v, err
}
