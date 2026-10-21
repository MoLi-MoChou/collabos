-- name: CreateRoom :one
INSERT INTO rooms (workspace_id, project_id, name) VALUES ($1, $2, $3) RETURNING *;

-- name: GetRoom :one
SELECT * FROM rooms WHERE id = $1;

-- name: ListRooms :many
SELECT * FROM rooms WHERE workspace_id = $1 ORDER BY created_at;

-- name: NextRoomSeq :one
UPDATE rooms SET last_seq = last_seq + 1 WHERE id = $1 RETURNING last_seq;

-- name: InsertMessage :one
INSERT INTO messages (room_id, seq, sender_type, sender_id, client_msg_id, body, mentions, card, trust)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9) RETURNING *;

-- name: GetMessageByClientID :one
SELECT * FROM messages WHERE room_id = $1 AND client_msg_id = $2;

-- name: GetMessage :one
SELECT * FROM messages WHERE id = $1;

-- name: UpdateMessageCard :one
UPDATE messages SET card = $2 WHERE id = $1 RETURNING *;

-- name: UpdateMessageBody :one
UPDATE messages SET body = $2 WHERE id = $1 RETURNING *;

-- name: ListMessages :many
SELECT * FROM messages WHERE room_id = $1 AND seq > $2 ORDER BY seq LIMIT $3;
