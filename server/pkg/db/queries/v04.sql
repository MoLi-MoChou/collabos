-- name: SetTaskExtras :one
UPDATE tasks SET trace_id = $2, depends_on = $3, approval_rule = $4, autonomy = $5, acceptance = $6, updated_at = now()
WHERE id = $1 RETURNING *;

-- name: ListTasksByTrace :many
SELECT * FROM tasks WHERE workspace_id = $1 AND trace_id = $2 ORDER BY created_at;

-- name: ListChildTasks :many
SELECT * FROM tasks WHERE parent_task_id = $1 ORDER BY created_at;

-- name: ListTasksDependingOn :many
SELECT * FROM tasks WHERE workspace_id = $1 AND jsonb_exists(depends_on, $2::text) ORDER BY created_at;

-- name: CountRunningForAgent :one
SELECT count(*) FROM tasks WHERE target_agent_id = $1
  AND status IN ('working', 'running_offline', 'approval_required', 'input_required', 'unknown')
  AND kind = 'code' AND reason_code <> 'awaiting_merge';

-- name: UpdateTaskBudget :one
UPDATE tasks SET budget = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetTaskTainted :exec
UPDATE tasks SET tainted = $2, updated_at = now() WHERE id = $1;

-- name: SetTaskDeadline :exec
UPDATE tasks SET decision_deadline = $2 WHERE id = $1;

-- name: ListOverdueTasks :many
SELECT * FROM tasks WHERE decision_deadline IS NOT NULL AND decision_deadline < now()
  AND status IN ('pending_intake', 'approval_required', 'input_required');

-- name: ListTasksByStatusForWorkspace :many
SELECT * FROM tasks WHERE workspace_id = $1 AND status = ANY(@statuses::text[]) ORDER BY created_at;

-- name: UpdateAgentPolicy :one
UPDATE agents SET autonomy = $2, max_concurrent = $3, max_duration_min = $4, allowed_actions = $5,
  allow_network = $6, intake_timeout_min = $7, daily_cost_units = $8
WHERE id = $1 RETURNING *;

-- name: SetApprovalMeta :one
UPDATE approvals SET category = $2, rule = $3, timeout_at = $4 WHERE id = $1 RETURNING *;

-- name: AddApprovalDecision :one
UPDATE approvals SET decisions = decisions || $2::jsonb WHERE id = $1 AND status = 'pending' RETURNING *;

-- name: SetPermissionMeta :exec
UPDATE permission_requests SET layer = $2, timeout_at = $3 WHERE id = $1;

-- name: ListOverduePermissions :many
SELECT * FROM permission_requests WHERE status = 'pending' AND timeout_at IS NOT NULL AND timeout_at < now();

-- name: MarkPermissionTimedOut :one
UPDATE permission_requests SET timeout_at = NULL WHERE id = $1 RETURNING *;

-- name: SetProjectSettings :one
UPDATE projects SET webhook_secret = $2, test_command = $3, visibility = $4 WHERE id = $1 RETURNING *;

-- name: AddProjectMember :exec
INSERT INTO project_members (project_id, user_id, role) VALUES ($1, $2, $3)
ON CONFLICT (project_id, user_id) DO UPDATE SET role = EXCLUDED.role;

-- name: RemoveProjectMember :exec
DELETE FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: ListProjectMembers :many
SELECT pm.project_id, pm.user_id, pm.role, u.name, u.handle FROM project_members pm JOIN users u ON u.id = pm.user_id
WHERE pm.project_id = $1 ORDER BY pm.created_at;

-- name: GetProjectMemberRole :one
SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2;

-- name: SetMessageRich :one
UPDATE messages SET rich = $2 WHERE id = $1 RETURNING *;

-- name: InsertRichAction :one
INSERT INTO rich_actions (message_id, action_id, user_id, value) VALUES ($1, $2, $3, $4)
ON CONFLICT (message_id, action_id, user_id) DO NOTHING RETURNING *;

-- name: SetMemoryTaint :one
UPDATE memory_versions SET tainted = $2, taint_reason = $3 WHERE id = $1 RETURNING *;

-- name: SetMemoryTrust :exec
UPDATE memory_versions SET trust = $2 WHERE id = $1;

-- name: InsertContextRef :exec
INSERT INTO context_refs (task_id, kind, ref_id, trust) VALUES ($1, $2, $3, $4) ON CONFLICT DO NOTHING;

-- name: ListContextRefsByRef :many
SELECT * FROM context_refs WHERE kind = $1 AND ref_id = ANY(@refs::text[]);

-- name: ListContextRefsForTask :many
SELECT * FROM context_refs WHERE task_id = $1 ORDER BY created_at;

-- name: UpsertBudget :one
INSERT INTO budgets (workspace_id, scope, subject_id, max_tokens) VALUES ($1, $2, $3, $4)
ON CONFLICT (workspace_id, scope, subject_id) DO UPDATE SET max_tokens = EXCLUDED.max_tokens, warned_period = ''
RETURNING *;

-- name: ListBudgets :many
SELECT * FROM budgets WHERE workspace_id = $1 ORDER BY scope, created_at;

-- name: SetBudgetWarned :exec
UPDATE budgets SET warned_period = $2 WHERE id = $1;

-- name: InsertUsage :exec
INSERT INTO usage_ledger (workspace_id, task_id, requester_id, owner_id, agent_id, tokens, known, compute_by, model_key_by)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9);

-- name: SumUsageWorkspaceSince :one
SELECT COALESCE(sum(tokens), 0)::bigint FROM usage_ledger WHERE workspace_id = $1 AND created_at >= $2;

-- name: SumUsagePayerSince :one
SELECT COALESCE(sum(tokens), 0)::bigint FROM usage_ledger
WHERE workspace_id = $1 AND created_at >= $3
  AND ((model_key_by = 'source' AND requester_id = $2) OR (model_key_by = 'target' AND owner_id = $2));

-- name: SumUsageAgentSince :one
SELECT COALESCE(sum(tokens), 0)::bigint FROM usage_ledger WHERE agent_id = $1 AND created_at >= $2;

-- name: UsageByRequester :many
SELECT requester_id AS subject_id, COALESCE(sum(tokens), 0)::bigint AS tokens, count(DISTINCT task_id)::bigint AS tasks
FROM usage_ledger WHERE workspace_id = $1 AND created_at >= $2 GROUP BY requester_id;

-- name: UsageByOwner :many
SELECT owner_id AS subject_id, COALESCE(sum(tokens), 0)::bigint AS tokens, count(DISTINCT task_id)::bigint AS tasks
FROM usage_ledger WHERE workspace_id = $1 AND created_at >= $2 GROUP BY owner_id;

-- name: UsageByAgent :many
SELECT agent_id AS subject_id, COALESCE(sum(tokens), 0)::bigint AS tokens, count(DISTINCT task_id)::bigint AS tasks
FROM usage_ledger WHERE workspace_id = $1 AND created_at >= $2 GROUP BY agent_id;

-- name: CreateTokenKind :exec
INSERT INTO auth_tokens (token_hash, user_id, expires_at, kind, label) VALUES ($1, $2, $3, $4, $5);

-- name: GetActiveToken :one
SELECT * FROM auth_tokens WHERE token_hash = $1 AND expires_at > now() AND revoked_at IS NULL;

-- name: RevokeToken :exec
UPDATE auth_tokens SET revoked_at = now() WHERE token_hash = $1 AND revoked_at IS NULL;

-- name: ListApiTokens :many
SELECT * FROM auth_tokens WHERE user_id = $1 AND kind = 'api' AND revoked_at IS NULL ORDER BY created_at DESC;

-- name: RevokeDaemon :exec
UPDATE daemons SET revoked_at = now(), status = 'offline' WHERE id = $1;

-- name: SetWorkspaceIM :one
UPDATE workspaces SET im_webhook_url = $2, im_kind = $3 WHERE id = $1 RETURNING *;

-- name: InsertWebhookDelivery :one
INSERT INTO webhook_deliveries (delivery_id, project_id, event) VALUES ($1, $2, $3)
ON CONFLICT (delivery_id) DO NOTHING RETURNING delivery_id;

-- name: GetPullRequestByNumber :one
SELECT * FROM pull_requests WHERE project_id = $1 AND number = $2;

-- name: GetPullRequestByBranch :one
SELECT * FROM pull_requests WHERE project_id = $1 AND branch = $2;

-- name: SetPullRequestStatus :one
UPDATE pull_requests SET status = $2, merge_commit = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: ListAuditForTargets :many
SELECT * FROM audit_log WHERE workspace_id = $1 AND target_id = ANY(@targets::text[]) ORDER BY id;
