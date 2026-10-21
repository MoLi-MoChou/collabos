-- name: UpsertUser :one
INSERT INTO users (email, name, handle) VALUES ($1, $2, $3)
ON CONFLICT (email) DO UPDATE SET name = EXCLUDED.name
RETURNING *;

-- name: GetUser :one
SELECT * FROM users WHERE id = $1;

-- name: CreateAuthToken :exec
INSERT INTO auth_tokens (token_hash, user_id, expires_at) VALUES ($1, $2, $3);

-- name: GetUserByToken :one
SELECT u.* FROM auth_tokens t JOIN users u ON u.id = t.user_id
WHERE t.token_hash = $1 AND t.expires_at > now();

-- name: GetUserByHandle :one
SELECT * FROM users WHERE handle = $1;
