package service

import (
	"context"

	"github.com/collabos/collabos/server/internal/apperr"
	"github.com/collabos/collabos/server/internal/util"
	db "github.com/collabos/collabos/server/pkg/db/generated"
)

// Autonomy levels (PRD 3.2 / phase 2):
//   - supervised: every non-read tool call asks a human, even inside the grant;
//   - standard:   calls inside the grant run, anything outside asks (default);
//   - autonomous: additionally, sub-delegations the agent proposes are
//     submitted without a draft (still subject to the target's intake, the
//     grant-subset rule and the merge gate, which is never automatic).
var autonomyLevels = map[string]bool{"supervised": true, "standard": true, "autonomous": true}

// AgentPolicyInput is the owner's intake constraints and autonomy (PRD 6.4).
type AgentPolicyInput struct {
	Autonomy         string   `json:"autonomy"`
	MaxConcurrent    int32    `json:"max_concurrent"`
	MaxDurationMin   int32    `json:"max_duration_min"`
	AllowedActions   []string `json:"allowed_actions"`
	AllowNetwork     bool     `json:"allow_network"`
	IntakeTimeoutMin int32    `json:"intake_timeout_min"`
	DailyCostUnits   int64    `json:"daily_cost_units"`
}

// UpdateAgentPolicy changes autonomy and intake constraints (owner only).
func (s *Service) UpdateAgentPolicy(ctx context.Context, wsID, userID, agentID string, in AgentPolicyInput) (AgentDTO, error) {
	a, err := s.getAgent(ctx, wsID, agentID)
	if err != nil {
		return AgentDTO{}, err
	}
	if util.UUIDString(a.OwnerID) != userID {
		return AgentDTO{}, apperr.ErrForbidden
	}
	if !autonomyLevels[in.Autonomy] {
		return AgentDTO{}, apperr.BadRequest("autonomy must be supervised, standard or autonomous")
	}
	if in.MaxConcurrent < 0 || in.MaxDurationMin < 0 || in.IntakeTimeoutMin < 0 || in.DailyCostUnits < 0 {
		return AgentDTO{}, apperr.BadRequest("limits must be >= 0")
	}
	if in.AllowedActions == nil {
		in.AllowedActions = []string{}
	}
	for _, x := range in.AllowedActions {
		switch x {
		case "read", "edit", "execute", "delete", "move", "fetch":
		default:
			return AgentDTO{}, apperr.BadRequest("unknown action: " + x)
		}
	}
	up, err := s.Q.UpdateAgentPolicy(ctx, db.UpdateAgentPolicyParams{
		ID: a.ID, Autonomy: in.Autonomy, MaxConcurrent: in.MaxConcurrent, MaxDurationMin: in.MaxDurationMin,
		AllowedActions: util.JSON(in.AllowedActions), AllowNetwork: in.AllowNetwork,
		IntakeTimeoutMin: in.IntakeTimeoutMin, DailyCostUnits: in.DailyCostUnits,
	})
	if err != nil {
		return AgentDTO{}, err
	}
	s.Audit(ctx, a.WorkspaceID, "user", userID, "agent.policy", "agent", agentID, in)
	s.dock(a.WorkspaceID, "devices")
	return s.agentDTO(up), nil
}
