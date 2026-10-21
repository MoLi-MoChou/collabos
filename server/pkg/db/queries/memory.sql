-- name: LatestConfirmedMemory :one
SELECT * FROM memory_versions WHERE project_id = $1 AND status = 'confirmed' AND NOT tainted ORDER BY version DESC LIMIT 1;

-- name: CreateMemoryVersion :one
INSERT INTO memory_versions (project_id, version, content, status, source_task_id, proposed_by)
VALUES ($1, (SELECT COALESCE(MAX(version), 0) + 1 FROM memory_versions WHERE project_id = $1), $2, $3, $4, $5)
RETURNING *;

-- name: GetMemoryVersion :one
SELECT * FROM memory_versions WHERE id = $1;

-- name: ListMemoryVersions :many
SELECT * FROM memory_versions WHERE project_id = $1 ORDER BY version DESC;

-- name: DecideMemoryVersion :one
UPDATE memory_versions SET status = $2, confirmed_by = $3, decided_at = now()
WHERE id = $1 AND status = 'pending' RETURNING *;

-- name: AppendAudit :exec
INSERT INTO audit_log (workspace_id, actor_type, actor_id, action, target_type, target_id, detail)
VALUES ($1, $2, $3, $4, $5, $6, $7);

-- name: ListAudit :many
SELECT * FROM audit_log WHERE workspace_id = $1 ORDER BY id DESC LIMIT $2;
