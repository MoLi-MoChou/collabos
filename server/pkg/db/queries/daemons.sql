-- name: CreateDaemon :one
INSERT INTO daemons (workspace_id, owner_id, name, node_token_hash)
VALUES ($1, $2, $3, $4) RETURNING *;

-- name: GetDaemonByToken :one
SELECT * FROM daemons WHERE node_token_hash = $1 AND revoked_at IS NULL;

-- name: GetDaemon :one
SELECT * FROM daemons WHERE id = $1;

-- name: ListDaemons :many
SELECT * FROM daemons WHERE workspace_id = $1 ORDER BY created_at;

-- name: ListDaemonsForOwner :many
SELECT * FROM daemons WHERE workspace_id = $1 AND owner_id = $2 ORDER BY created_at;

-- name: SetDaemonStatus :exec
UPDATE daemons SET status = $2, last_seen_at = now() WHERE id = $1;

-- name: SetDaemonCapabilities :exec
UPDATE daemons SET capabilities = $2, last_seen_at = now() WHERE id = $1;

-- name: TouchDaemon :exec
UPDATE daemons SET last_seen_at = now() WHERE id = $1;

-- name: AdvanceDaemonAck :one
UPDATE daemons SET last_acked_seq = GREATEST(last_acked_seq, $2) WHERE id = $1 RETURNING last_acked_seq;

-- name: DeleteDaemon :exec
DELETE FROM daemons WHERE id = $1 AND workspace_id = $2;
