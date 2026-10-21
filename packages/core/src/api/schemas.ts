import { z } from "zod";

/**
 * Parses untrusted server data. On failure it logs and returns the fallback
 * instead of throwing, so an older client keeps working against a newer
 * server (conventions §2.2). Never use `as T` on responses.
 */
export function parseWithFallback<T>(schema: z.ZodType<T>, data: unknown, fallback: T, what = "response"): T {
  const r = schema.safeParse(data);
  if (r.success) return r.data;
  console.warn(`[collabos] could not parse ${what}`, r.error.issues.slice(0, 3));
  return fallback;
}

const str = z.string().catch("");
const num = z.number().catch(0);

export const TaskStatus = z
  .enum([
    "submitted",
    "pending_intake",
    "accepted",
    "rejected",
    "queued",
    "working",
    "running_offline",
    "input_required",
    "approval_required",
    "completed",
    "failed",
    "canceled",
    "unknown",
  ])
  .catch("unknown");
export type TaskStatus = z.infer<typeof TaskStatus>;

export const Grant = z.object({
  resources: z.array(z.string()).catch([]),
  actions: z.array(z.string()).catch([]),
  network: z.array(z.string()).nullable().catch([]).transform((v) => v ?? []),
  expires_at: z.string().optional(),
});
export type Grant = z.infer<typeof Grant>;

export const Budget = z.object({ max_tokens: num, max_duration_min: num });
export type Budget = z.infer<typeof Budget>;

export const Approver = z.object({ id: z.string(), name: str, role: str });
export type Approver = z.infer<typeof Approver>;

export const Usage = z.object({ input_tokens: num, output_tokens: num, total_tokens: num }).partial();

export const TaskResult = z
  .object({
    outcome: str,
    summary: str,
    branch: str,
    commit: str,
    diff_stat: str,
    error: str,
    reason_code: str,
    acp_version: num,
    duration_ms: num,
    usage: Usage.nullable().optional(),
    diff: str,
    tests: z.object({ command: str, passed: z.boolean().catch(false), output: str }).nullable().optional(),
  })
  .partial();

export const Task = z.object({
  id: z.string(),
  room_id: str,
  project_id: str,
  parent_task_id: str,
  created_by: str,
  source_agent_id: str,
  target_agent_id: str,
  target_owner_id: str,
  daemon_id: str,
  kind: z.enum(["code", "chat"]).catch("code"),
  title: str,
  prompt: str,
  status: TaskStatus,
  grant: Grant.catch({ resources: [], actions: [], network: [] }),
  budget: Budget.catch({ max_tokens: 0, max_duration_min: 0 }),
  approvers: z.array(Approver).catch([]),
  depth: num,
  max_depth: num,
  reason_code: str,
  used_tokens: num,
  result: TaskResult.catch({}),
  trace_id: str,
  depends_on: z.array(z.string()).nullable().catch([]).transform((v) => v ?? []),
  approval_rule: z.enum(["ANY_ONE", "ALL"]).catch("ANY_ONE"),
  autonomy: str,
  acceptance: z.array(z.string()).nullable().catch([]).transform((v) => v ?? []),
  tainted: z.boolean().catch(false),
  decision_deadline: str,
  created_at: str,
  updated_at: str,
});
export type Task = z.infer<typeof Task>;

export const TaskEvent = z.object({
  task_id: z.string(),
  seq: z.number(),
  type: str,
  payload: z.record(z.string(), z.unknown()).catch({}),
  created_at: str,
});
export type TaskEvent = z.infer<typeof TaskEvent>;

export const Mention = z.object({ type: z.enum(["user", "agent"]).catch("user"), id: str, handle: str, kind: str.optional() });

export const Message = z.object({
  id: z.string(),
  room_id: z.string(),
  seq: z.number(),
  sender_type: z.enum(["user", "agent", "system"]).catch("system"),
  sender_id: str,
  client_msg_id: str,
  body: str,
  mentions: z.array(Mention).catch([]),
  card: z.record(z.string(), z.unknown()).nullable().catch(null),
  trust: str,
  rich: z.object({ lang: str, version: str, source: str, agent_id: str, owner_id: str, trust: str }).nullable().catch(null).optional(),
  created_at: str,
});
export type Message = z.infer<typeof Message>;

export const Draft = z.object({
  title: str,
  prompt: str,
  grant: Grant,
  budget: Budget,
  approvers: z.array(Approver).catch([]),
  max_depth: num,
});
export type Draft = z.infer<typeof Draft>;

export const Member = z.object({ id: z.string(), name: str, email: str, handle: str, role: str });
export type Member = z.infer<typeof Member>;

export const Agent = z.object({
  id: z.string(),
  owner_id: str,
  daemon_id: str,
  name: str,
  handle: str,
  kind: z.enum(["chief", "specialist"]).catch("specialist"),
  runtime: str,
  model: str,
  intake_mode: z.enum(["manual", "allowlist", "closed"]).catch("manual"),
  intake_allowlist: z.array(z.string()).catch([]),
  daily_quota: num,
  online: z.boolean().catch(false),
  autonomy: z.enum(["supervised", "standard", "autonomous"]).catch("standard"),
  max_concurrent: num,
  max_duration_min: num,
  allowed_actions: z.array(z.string()).nullable().catch([]).transform((v) => v ?? []),
  allow_network: z.boolean().catch(false),
  intake_timeout_min: num,
  daily_cost_units: num,
});
export type Agent = z.infer<typeof Agent>;

export const Capabilities = z
  .object({
    os: str,
    arch: str,
    hostname: str,
    sandbox: str,
    version: str,
    agents: z.array(z.string()).nullable().catch([]),
    tools: z.record(z.string(), z.string()).nullable().catch({}),
    repos: z.array(z.object({ remote_url: str, path: str })).nullable().catch([]),
  })
  .partial();

export const Daemon = z.object({
  id: z.string(),
  name: str,
  owner_id: str,
  status: z.enum(["online", "offline"]).catch("offline"),
  capabilities: Capabilities.catch({}),
  last_seen_at: str,
});
export type Daemon = z.infer<typeof Daemon>;

export const Project = z.object({
  id: z.string(),
  name: str,
  remote_url: str,
  default_branch: str,
  provider: str,
  lead_id: str,
  room_id: str,
  visibility: z.enum(["workspace", "restricted"]).catch("workspace"),
  test_command: str,
  webhook_path: str,
  webhook_configured: z.boolean().catch(false),
  my_role: str,
});
export type Project = z.infer<typeof Project>;

export const Room = z.object({ id: z.string(), project_id: str, name: str, last_seq: num });
export type Room = z.infer<typeof Room>;

export const PullRequest = z.object({
  id: z.string(),
  project_id: str,
  task_id: str,
  number: num,
  provider: str,
  url: str,
  branch: str,
  base_branch: str,
  head_commit: str,
  diff_stat: str,
  status: z.enum(["open", "merged", "closed"]).catch("open"),
  merge_commit: str,
  created_at: str,
});
export type PullRequest = z.infer<typeof PullRequest>;

export const MemoryVersion = z.object({
  id: z.string(),
  project_id: str,
  version: num,
  content: str,
  status: z.enum(["pending", "confirmed", "rejected"]).catch("pending"),
  source_task_id: str,
  proposed_by: str,
  confirmed_by: str,
  trust: str,
  tainted: z.boolean().catch(false),
  taint_reason: str,
  created_at: str,
});
export type MemoryVersion = z.infer<typeof MemoryVersion>;

export const AuditEntry = z.object({
  id: z.number(),
  actor_type: str,
  actor_id: str,
  action: str,
  target_type: str,
  target_id: str,
  detail: z.unknown(),
  created_at: str,
});
export type AuditEntry = z.infer<typeof AuditEntry>;

export const Workspace = z.object({ id: z.string(), name: str, invite_code: str, role: str });
export const Me = z.object({ id: z.string(), name: str, email: str, handle: str, workspaces: z.array(Workspace).catch([]) });
export type Me = z.infer<typeof Me>;

export const LoginResult = z.object({ token: z.string(), refresh_token: str.optional(), expires_at: str.optional(), user_id: z.string(), name: str, email: str, handle: str, workspace_id: z.string() });
export type LoginResult = z.infer<typeof LoginResult>;

export const PairResult = z.object({ daemon: Daemon, node_token: z.string(), agents: z.array(Agent).catch([]) });
export type PairResult = z.infer<typeof PairResult>;

export const ApiError = z.object({ error: str, code: str });

export const TokenPair = z.object({ token: z.string(), refresh_token: z.string(), expires_at: str });
export const ApiToken = z.object({ id: z.string(), label: str, token: str.optional(), expires_at: str, created_at: str });
export type ApiToken = z.infer<typeof ApiToken>;
export const BudgetRow = z.object({ id: str, scope: z.enum(["workspace", "user"]).catch("workspace"), subject_id: str, max_tokens: num, used: num, period: str });
export type BudgetRow = z.infer<typeof BudgetRow>;
export const UsageRow = z.object({ subject_id: str, tokens: num, tasks: num });
export const UsageView = z.object({
  period: str,
  total: num,
  by_requester: z.array(UsageRow).nullable().catch([]).transform((v) => v ?? []),
  by_owner: z.array(UsageRow).nullable().catch([]).transform((v) => v ?? []),
  by_agent: z.array(UsageRow).nullable().catch([]).transform((v) => v ?? []),
  budgets: z.array(BudgetRow).nullable().catch([]).transform((v) => v ?? []),
});
export type UsageView = z.infer<typeof UsageView>;
export const ProjectMember = z.object({ user_id: z.string(), name: str, handle: str, role: z.enum(["lead", "writer", "viewer"]).catch("viewer") });
export type ProjectMember = z.infer<typeof ProjectMember>;
export const TraceView = z.object({
  trace_id: str,
  tasks: z.array(Task).nullable().catch([]).transform((v) => v ?? []),
  events: z.array(TaskEvent).nullable().catch([]).transform((v) => v ?? []),
  pull_requests: z.array(PullRequest).nullable().catch([]).transform((v) => v ?? []),
  audit: z.array(AuditEntry).nullable().catch([]).transform((v) => v ?? []),
});
export type TraceView = z.infer<typeof TraceView>;
export const TaintReport = z.object({ versions: z.array(num).catch([]), tasks: z.array(z.string()).catch([]) });

// ---- chat cards ----
export const DraftCard = z.object({
  type: z.literal("delegation_draft"),
  state: z.enum(["draft", "submitted", "discarded"]).catch("draft"),
  requester_id: str,
  target_agent_id: str,
  target_handle: str,
  target_owner_id: str,
  task_id: str.optional(),
  draft: Draft,
});
export const IntakeCard = z.object({
  type: z.literal("intake"),
  state: z.enum(["pending", "accepted", "rejected"]).catch("pending"),
  task_id: z.string(),
  owner_id: str,
  requester_id: str,
  agent_handle: str,
  title: str,
  grant: Grant.catch({ resources: [], actions: [], network: [] }),
  reason_code: str.optional(),
});
export const PermissionCard = z.object({
  type: z.literal("permission"),
  state: z.enum(["pending", "approved", "denied"]).catch("pending"),
  task_id: z.string(),
  permission_id: z.string(),
  kind: str,
  title: str,
  paths: z.array(z.string()).catch([]),
  command: str,
  reason_code: str,
  eligible: z.array(z.string()).catch([]),
  decided_by: str.optional(),
  layer: str.optional(),
  trust: str.optional(),
  source_agent_id: str.optional(),
  trace_id: str.optional(),
});
export const MergeCard = z.object({
  type: z.literal("merge"),
  state: z.enum(["pending", "merging", "merged", "rejected", "failed"]).catch("pending"),
  task_id: z.string(),
  approval_id: z.string(),
  pr_id: str,
  number: num,
  branch: str,
  base_branch: str,
  commit: str,
  diff_stat: str,
  url: str,
  eligible: z.array(z.string()).catch([]),
  approvers: z.array(Approver).catch([]),
  summary: str.optional(),
  merge_commit: str.optional(),
  error: str.optional(),
  decided_by: str.optional(),
  rule: z.enum(["ANY_ONE", "ALL"]).catch("ANY_ONE"),
  category: str.optional(),
  decisions: z.array(z.object({ by: str, decision: str, comment: str.optional() })).nullable().catch([]).transform((v) => v ?? []),
  summary_trust: str.optional(),
  source_agent_id: str.optional(),
  trace_id: str.optional(),
  tests: z.object({ command: str, passed: z.boolean().catch(false), output: str }).nullable().optional(),
});
export const MemoryCard = z.object({
  type: z.literal("memory"),
  state: z.enum(["pending", "confirmed", "rejected"]).catch("pending"),
  memory_id: z.string(),
  project_id: str,
  version: num,
  lead_id: str,
  addition: str,
  task_id: str.optional(),
});
export const TaskCard = z.object({ type: z.literal("task"), task_id: z.string() });
export const NoticeCard = z.object({ type: z.literal("notice"), code: str, task_id: str.optional() });
export const BudgetCard = z.object({
  type: z.literal("budget"),
  state: z.enum(["pending", "approved"]).catch("pending"),
  task_id: z.string(),
  scope: str,
  used: num,
  max: num,
  eligible: z.array(z.string()).catch([]),
  decided_by: str.optional(),
});
export const NudgeCard = z.object({ type: z.literal("nudge"), task_id: z.string(), by: str, waiting_on: z.array(z.string()).nullable().catch([]).transform((v) => v ?? []) });
export const AgentPostCard = z.object({
  type: z.literal("agent_post"),
  state: z.enum(["open", "converted"]).catch("open"),
  task_id: str,
  agent_id: str,
  owner_id: str,
  trace_id: str.optional(),
  converted_task_id: str.optional(),
});
export const AgentReportCard = z.object({ type: z.literal("agent_report"), task_id: z.string() });
export const RichReplyCard = z.object({ type: z.literal("rich_reply"), message_id: str, action_id: str });

export const Card = z.discriminatedUnion("type", [DraftCard, IntakeCard, PermissionCard, MergeCard, MemoryCard, TaskCard, NoticeCard, BudgetCard, NudgeCard, AgentPostCard, AgentReportCard, RichReplyCard]);
export type Card = z.infer<typeof Card>;
export type DraftCard = z.infer<typeof DraftCard>;
export type IntakeCard = z.infer<typeof IntakeCard>;
export type PermissionCard = z.infer<typeof PermissionCard>;
export type MergeCard = z.infer<typeof MergeCard>;
export type MemoryCard = z.infer<typeof MemoryCard>;
export type NoticeCard = z.infer<typeof NoticeCard>;
export type BudgetCard = z.infer<typeof BudgetCard>;
export type NudgeCard = z.infer<typeof NudgeCard>;
export type AgentPostCard = z.infer<typeof AgentPostCard>;

/** Parses a message card; unknown card types render as plain messages. */
export function parseCard(raw: unknown): Card | null {
  if (!raw) return null;
  const r = Card.safeParse(raw);
  return r.success ? r.data : null;
}
