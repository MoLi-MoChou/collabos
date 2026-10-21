CREATE INDEX CONCURRENTLY IF NOT EXISTS memory_project_idx ON memory_versions (project_id, version DESC);
