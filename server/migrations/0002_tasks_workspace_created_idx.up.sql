CREATE INDEX CONCURRENTLY IF NOT EXISTS tasks_workspace_created_idx ON tasks (workspace_id, created_at DESC);
