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

export const LoginResult = z.object({ token: z.string(), user_id: z.string(), name: str, email: str, handle: str, workspace_id: z.string() });
export type LoginResult = z.infer<typeof LoginResult>;

export const PairResult = z.object({ daemon: Daemon, node_token: z.string(), agents: z.array(Agent).catch([]) });
export type PairResult = z.infer<typeof PairResult>;

export const ApiError = z.object({ error: str, code: str });

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

export const Card = z.discriminatedUnion("type", [DraftCard, IntakeCard, PermissionCard, MergeCard, MemoryCard, TaskCard, NoticeCard]);
export type Card = z.infer<typeof Card>;
export type DraftCard = z.infer<typeof DraftCard>;
export type IntakeCard = z.infer<typeof IntakeCard>;
export type PermissionCard = z.infer<typeof PermissionCard>;
export type MergeCard = z.infer<typeof MergeCard>;
export type MemoryCard = z.infer<typeof MemoryCard>;
export type NoticeCard = z.infer<typeof NoticeCard>;

/** Parses a message card; unknown card types render as plain messages. */
export function parseCard(raw: unknown): Card | null {
  if (!raw) return null;
  const r = Card.safeParse(raw);
  return r.success ? r.data : null;
}
