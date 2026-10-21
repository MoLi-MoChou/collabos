DROP TRIGGER IF EXISTS audit_log_no_update ON audit_log;
DROP FUNCTION IF EXISTS audit_log_append_only();
DROP TABLE IF EXISTS audit_log, memory_versions, approvals, pull_requests, permission_requests,
    task_events, tasks, messages, rooms, agents, projects, daemons, workspace_members,
    workspaces, auth_tokens, users;
