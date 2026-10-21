-- name: CreateAgent :one
INSERT INTO agents (workspace_id, owner_id, daemon_id, name, handle, kind, runtime, model)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8) RETURNING *;

-- name: GetAgent :one
SELECT * FROM agents WHERE id = $1;

-- name: GetAgentByHandle :one
SELECT * FROM agents WHERE workspace_id = $1 AND handle = $2;

-- name: ListAgents :many
SELECT * FROM agents WHERE workspace_id = $1 ORDER BY created_at;

-- name: UpdateIntakePolicy :one
UPDATE agents SET intake_mode = $2, intake_allowlist = $3, daily_quota = $4 WHERE id = $1 RETURNING *;

-- name: CountAcceptedToday :one
SELECT count(*) FROM tasks
WHERE target_agent_id = $1 AND created_by <> target_owner_id
  AND status NOT IN ('submitted', 'pending_intake', 'rejected')
  AND created_at >= date_trunc('day', now());
