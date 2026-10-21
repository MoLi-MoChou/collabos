-- No foreign keys or cascades by convention: relationships are kept in application code.
CREATE TABLE users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    handle TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE auth_tokens (
    token_hash TEXT PRIMARY KEY,
    user_id UUID NOT NULL,
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspaces (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    invite_code TEXT NOT NULL UNIQUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE workspace_members (
    workspace_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    joined_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (workspace_id, user_id)
);

CREATE TABLE daemons (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    name TEXT NOT NULL,
    node_token_hash TEXT NOT NULL UNIQUE,
    status TEXT NOT NULL DEFAULT 'offline',
    capabilities JSONB NOT NULL DEFAULT '{}'::jsonb,
    last_acked_seq BIGINT NOT NULL DEFAULT 0,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    name TEXT NOT NULL,
    remote_url TEXT NOT NULL,
    default_branch TEXT NOT NULL DEFAULT 'main',
    provider TEXT NOT NULL DEFAULT 'local',
    lead_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE agents (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    daemon_id UUID NOT NULL,
    name TEXT NOT NULL,
    handle TEXT NOT NULL,
    kind TEXT NOT NULL DEFAULT 'specialist',
    runtime TEXT NOT NULL DEFAULT 'opencode',
    model TEXT NOT NULL DEFAULT '',
    intake_mode TEXT NOT NULL DEFAULT 'manual',
    intake_allowlist JSONB NOT NULL DEFAULT '[]'::jsonb,
    daily_quota INTEGER NOT NULL DEFAULT 20,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, handle)
);

CREATE TABLE rooms (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    project_id UUID,
    name TEXT NOT NULL,
    last_seq BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE messages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    room_id UUID NOT NULL,
    seq BIGINT NOT NULL,
    sender_type TEXT NOT NULL,
    sender_id UUID,
    client_msg_id TEXT,
    body TEXT NOT NULL DEFAULT '',
    mentions JSONB NOT NULL DEFAULT '[]'::jsonb,
    card JSONB,
    trust TEXT NOT NULL DEFAULT 'user_input',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (room_id, seq),
    UNIQUE (room_id, client_msg_id)
);

-- A task is the board item and its delegation (PRD 6.1). Sub-delegations
-- reference parent_task_id.
CREATE TABLE tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    room_id UUID,
    project_id UUID,
    source_message_id UUID,
    parent_task_id UUID,
    idempotency_key TEXT NOT NULL,
    created_by UUID NOT NULL,
    source_agent_id UUID,
    target_agent_id UUID NOT NULL,
    target_owner_id UUID NOT NULL,
    daemon_id UUID NOT NULL,
    kind TEXT NOT NULL DEFAULT 'code',
    title TEXT NOT NULL,
    prompt TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'submitted',
    grant_spec JSONB NOT NULL DEFAULT '{}'::jsonb,
    budget JSONB NOT NULL DEFAULT '{}'::jsonb,
    billing JSONB NOT NULL DEFAULT '{}'::jsonb,
    approvers JSONB NOT NULL DEFAULT '[]'::jsonb,
    depth INTEGER NOT NULL DEFAULT 1,
    max_depth INTEGER NOT NULL DEFAULT 3,
    reason_code TEXT NOT NULL DEFAULT '',
    used_tokens BIGINT NOT NULL DEFAULT 0,
    result JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (workspace_id, idempotency_key)
);

CREATE TABLE task_events (
    id BIGSERIAL PRIMARY KEY,
    task_id UUID NOT NULL,
    seq INTEGER NOT NULL,
    type TEXT NOT NULL,
    payload JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (task_id, seq)
);

CREATE TABLE permission_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL,
    request_id TEXT NOT NULL,
    kind TEXT NOT NULL,
    title TEXT NOT NULL,
    paths JSONB NOT NULL DEFAULT '[]'::jsonb,
    command TEXT NOT NULL DEFAULT '',
    reason_code TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'pending',
    decided_by UUID,
    comment TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ,
    UNIQUE (task_id, request_id)
);

CREATE TABLE pull_requests (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    project_id UUID NOT NULL,
    task_id UUID NOT NULL UNIQUE,
    number INTEGER NOT NULL,
    provider TEXT NOT NULL,
    url TEXT NOT NULL DEFAULT '',
    branch TEXT NOT NULL,
    base_branch TEXT NOT NULL,
    head_commit TEXT NOT NULL,
    diff_stat TEXT NOT NULL DEFAULT '',
    status TEXT NOT NULL DEFAULT 'open',
    merge_commit TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (project_id, number)
);

CREATE TABLE approvals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_id UUID NOT NULL,
    pr_id UUID NOT NULL,
    action TEXT NOT NULL,
    rule TEXT NOT NULL DEFAULT 'ANY_ONE',
    eligible JSONB NOT NULL DEFAULT '[]'::jsonb,
    status TEXT NOT NULL DEFAULT 'pending',
    approved_commit TEXT NOT NULL DEFAULT '',
    decided_by UUID,
    comment TEXT NOT NULL DEFAULT '',
    executor_daemon_id UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ
);

CREATE TABLE memory_versions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL,
    version INTEGER NOT NULL,
    content TEXT NOT NULL,
    status TEXT NOT NULL,
    source_task_id UUID,
    proposed_by TEXT NOT NULL,
    confirmed_by UUID,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    decided_at TIMESTAMPTZ
);

CREATE TABLE audit_log (
    id BIGSERIAL PRIMARY KEY,
    workspace_id UUID NOT NULL,
    actor_type TEXT NOT NULL,
    actor_id TEXT NOT NULL,
    action TEXT NOT NULL,
    target_type TEXT NOT NULL,
    target_id TEXT NOT NULL,
    detail JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE FUNCTION audit_log_append_only() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_log is append-only';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_log_no_update BEFORE UPDATE OR DELETE ON audit_log
    FOR EACH ROW EXECUTE FUNCTION audit_log_append_only();
