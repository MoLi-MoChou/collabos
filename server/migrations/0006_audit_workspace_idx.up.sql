CREATE INDEX CONCURRENTLY IF NOT EXISTS audit_workspace_idx ON audit_log (workspace_id, id DESC);
