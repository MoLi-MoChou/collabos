-- name: CreatePermissionRequest :one
INSERT INTO permission_requests (task_id, request_id, kind, title, paths, command, reason_code, status)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
ON CONFLICT (task_id, request_id) DO UPDATE SET kind = EXCLUDED.kind
RETURNING *;

-- name: GetPermissionRequest :one
SELECT * FROM permission_requests WHERE id = $1;

-- name: DecidePermissionRequest :one
UPDATE permission_requests SET status = $2, decided_by = $3, comment = $4, decided_at = now()
WHERE id = $1 AND status = 'pending' RETURNING *;

-- name: ListPendingPermissions :many
SELECT * FROM permission_requests WHERE task_id = $1 AND status = 'pending';

-- name: CreatePullRequest :one
INSERT INTO pull_requests (workspace_id, project_id, task_id, number, provider, url, branch, base_branch, head_commit, diff_stat)
VALUES ($1, $2, $3, (SELECT COALESCE(MAX(number), 0) + 1 FROM pull_requests WHERE project_id = $2), $4, $5, $6, $7, $8, $9)
ON CONFLICT (task_id) DO UPDATE SET head_commit = EXCLUDED.head_commit, diff_stat = EXCLUDED.diff_stat, updated_at = now()
RETURNING *;

-- name: GetPullRequest :one
SELECT * FROM pull_requests WHERE id = $1;

-- name: GetPullRequestByTask :one
SELECT * FROM pull_requests WHERE task_id = $1;

-- name: ListPullRequests :many
SELECT * FROM pull_requests WHERE workspace_id = $1 ORDER BY created_at DESC;

-- name: SetPullRequestMerged :one
UPDATE pull_requests SET status = $2, merge_commit = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: CreateApproval :one
INSERT INTO approvals (task_id, pr_id, action, rule, eligible, approved_commit)
VALUES ($1, $2, $3, $4, $5, $6) RETURNING *;

-- name: GetApproval :one
SELECT * FROM approvals WHERE id = $1;

-- name: GetApprovalByTask :one
SELECT * FROM approvals WHERE task_id = $1 ORDER BY created_at DESC LIMIT 1;

-- name: DecideApproval :one
UPDATE approvals SET status = $2, decided_by = $3, comment = $4, executor_daemon_id = $5, decided_at = now()
WHERE id = $1 AND status = 'pending' RETURNING *;

-- name: SetApprovalStatus :one
UPDATE approvals SET status = $2 WHERE id = $1 RETURNING *;

-- name: ListPendingApprovalsForWorkspace :many
SELECT a.* FROM approvals a JOIN tasks t ON t.id = a.task_id
WHERE t.workspace_id = $1 AND a.status = 'pending';

-- name: ListDecidedPermissions :many
SELECT * FROM permission_requests WHERE task_id = $1 AND status <> 'pending' ORDER BY decided_at;
