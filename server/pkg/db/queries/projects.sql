-- name: CreateProject :one
INSERT INTO projects (workspace_id, name, remote_url, default_branch, provider, lead_id)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetProject :one
SELECT * FROM projects WHERE id = $1;

-- name: ListProjects :many
SELECT * FROM projects WHERE workspace_id = $1 ORDER BY created_at;
