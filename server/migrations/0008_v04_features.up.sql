-- PRD V0.4 features beyond the MVP: tracing, dependencies, autonomy,
-- budgets and usage ledger, webhooks, project access, taint tracking,
-- rich messages, refresh/API tokens, IM notifications.

ALTER TABLE tasks
    ADD COLUMN trace_id TEXT NOT NULL DEFAULT '',
    ADD COLUMN depends_on JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN tainted BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN decision_deadline TIMESTAMPTZ,
    ADD COLUMN approval_rule TEXT NOT NULL DEFAULT 'ANY_ONE',
    ADD COLUMN autonomy TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN acceptance JSONB NOT NULL DEFAULT '[]'::jsonb;

ALTER TABLE agents
    ADD COLUMN autonomy TEXT NOT NULL DEFAULT 'standard',
    ADD COLUMN max_concurrent INTEGER NOT NULL DEFAULT 2,
    ADD COLUMN max_duration_min INTEGER NOT NULL DEFAULT 60,
    ADD COLUMN allowed_actions JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN allow_network BOOLEAN NOT NULL DEFAULT true,
    ADD COLUMN intake_timeout_min INTEGER NOT NULL DEFAULT 1440,
    ADD COLUMN daily_cost_units BIGINT NOT NULL DEFAULT 0;

ALTER TABLE approvals
    ADD COLUMN category TEXT NOT NULL DEFAULT 'merge_protected',
    ADD COLUMN decisions JSONB NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN timeout_at TIMESTAMPTZ;

ALTER TABLE permission_requests
    ADD COLUMN layer TEXT NOT NULL DEFAULT 'acp',
    ADD COLUMN timeout_at TIMESTAMPTZ;

ALTER TABLE projects
    ADD COLUMN webhook_secret TEXT NOT NULL DEFAULT '',
    ADD COLUMN test_command TEXT NOT NULL DEFAULT '',
    ADD COLUMN visibility TEXT NOT NULL DEFAULT 'workspace';

ALTER TABLE messages ADD COLUMN rich JSONB;

ALTER TABLE memory_versions
    ADD COLUMN tainted BOOLEAN NOT NULL DEFAULT false,
    ADD COLUMN taint_reason TEXT NOT NULL DEFAULT '',
    ADD COLUMN trust TEXT NOT NULL DEFAULT 'confirmed';

ALTER TABLE auth_tokens
    ADD COLUMN kind TEXT NOT NULL DEFAULT 'access',
    ADD COLUMN label TEXT NOT NULL DEFAULT '',
    ADD COLUMN revoked_at TIMESTAMPTZ;

ALTER TABLE daemons ADD COLUMN revoked_at TIMESTAMPTZ;

ALTER TABLE workspaces
    ADD COLUMN im_webhook_url TEXT NOT NULL DEFAULT '',
    ADD COLUMN im_kind TEXT NOT NULL DEFAULT '';

CREATE TABLE project_members (
    project_id UUID NOT NULL,
    user_id UUID NOT NULL,
    role TEXT NOT NULL DEFAULT 'writer',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (project_id, user_id)
);

CREATE TABLE budgets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    workspace_id UUID NOT NULL,
    scope TEXT NOT NULL,
    subject_id UUID,
    max_tokens BIGINT NOT NULL,
    warned_period TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE NULLS NOT DISTINCT (workspace_id, scope, subject_id)
);

CREATE TABLE usage_ledger (
    id BIGSERIAL PRIMARY KEY,
    workspace_id UUID NOT NULL,
    task_id UUID NOT NULL,
    requester_id UUID NOT NULL,
    owner_id UUID NOT NULL,
    agent_id UUID NOT NULL,
    tokens BIGINT NOT NULL,
    known BOOLEAN NOT NULL DEFAULT true,
    compute_by TEXT NOT NULL DEFAULT 'target',
    model_key_by TEXT NOT NULL DEFAULT 'source',
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE context_refs (
    task_id UUID NOT NULL,
    kind TEXT NOT NULL,
    ref_id TEXT NOT NULL,
    trust TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (task_id, kind, ref_id)
);

CREATE TABLE rich_actions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    message_id UUID NOT NULL,
    action_id TEXT NOT NULL,
    user_id UUID NOT NULL,
    value JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (message_id, action_id, user_id)
);

CREATE TABLE webhook_deliveries (
    delivery_id TEXT PRIMARY KEY,
    project_id UUID NOT NULL,
    event TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
