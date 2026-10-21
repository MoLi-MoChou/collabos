CREATE INDEX CONCURRENTLY IF NOT EXISTS usage_ledger_ws_created_idx ON usage_ledger (workspace_id, created_at);
