CREATE INDEX CONCURRENTLY IF NOT EXISTS messages_room_seq_idx ON messages (room_id, seq DESC);
