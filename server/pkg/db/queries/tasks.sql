-- name: CreateTask :one
INSERT INTO tasks (workspace_id, room_id, project_id, source_message_id, parent_task_id, idempotency_key,
    created_by, source_agent_id, target_agent_id, target_owner_id, daemon_id, kind, title, prompt, status,
    grant_spec, budget, billing, approvers, depth, max_depth)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21)
RETURNING *;

-- name: GetTaskByIdempotency :one
SELECT * FROM tasks WHERE workspace_id = $1 AND idempotency_key = $2;

-- name: GetTask :one
SELECT * FROM tasks WHERE id = $1;

-- name: ListTasks :many
SELECT * FROM tasks WHERE workspace_id = $1 ORDER BY created_at DESC LIMIT 200;

-- name: ListTasksForDaemonByStatus :many
SELECT * FROM tasks WHERE daemon_id = $1 AND status = ANY(@statuses::text[]) ORDER BY created_at;

-- name: UpdateTaskStatus :one
UPDATE tasks SET status = $2, reason_code = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: UpdateTaskGrant :one
UPDATE tasks SET grant_spec = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: UpdateTaskBilling :one
UPDATE tasks SET billing = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: AddTaskUsage :one
UPDATE tasks SET used_tokens = $2, updated_at = now() WHERE id = $1 RETURNING *;

-- name: SetTaskResult :one
UPDATE tasks SET status = $2, result = $3, updated_at = now() WHERE id = $1 RETURNING *;

-- name: AppendTaskEvent :one
INSERT INTO task_events (task_id, seq, type, payload)
VALUES ($1, (SELECT COALESCE(MAX(seq), 0) + 1 FROM task_events WHERE task_id = $1), $2, $3)
RETURNING *;

-- name: ListTaskEvents :many
SELECT * FROM task_events WHERE task_id = $1 AND seq > $2 ORDER BY seq LIMIT 5000;
