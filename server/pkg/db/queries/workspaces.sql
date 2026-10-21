-- name: CreateWorkspace :one
INSERT INTO workspaces (name, invite_code) VALUES ($1, $2) RETURNING *;

-- name: GetWorkspaceByInvite :one
SELECT * FROM workspaces WHERE invite_code = $1;

-- name: GetWorkspace :one
SELECT * FROM workspaces WHERE id = $1;

-- name: AddMember :exec
INSERT INTO workspace_members (workspace_id, user_id, role) VALUES ($1, $2, $3)
ON CONFLICT DO NOTHING;

-- name: ListWorkspacesForUser :many
SELECT w.*, m.role FROM workspaces w JOIN workspace_members m ON m.workspace_id = w.id
WHERE m.user_id = $1 ORDER BY w.created_at;

-- name: GetMemberRole :one
SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2;

-- name: ListMembers :many
SELECT u.id, u.name, u.email, u.handle, m.role FROM workspace_members m JOIN users u ON u.id = m.user_id
WHERE m.workspace_id = $1 ORDER BY m.joined_at;
