CREATE INDEX CONCURRENTLY IF NOT EXISTS tasks_daemon_status_idx ON tasks (daemon_id, status);
