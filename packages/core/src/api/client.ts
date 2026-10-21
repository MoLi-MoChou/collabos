import { z } from "zod";
import * as S from "./schemas";

/** Error returned by the API with a stable code. */
export class ApiError extends Error {
  constructor(
    public status: number,
    public code: string,
    message: string,
  ) {
    super(message);
  }
}

export interface ApiClientOptions {
  baseUrl: string;
  getToken: () => string | null;
  getWorkspaceId: () => string | null;
  onUnauthorized?: () => void;
  /** Refresh token for transparent access-token rotation on 401 (FR-SRV-101). */
  getRefreshToken?: () => string | null;
  onTokens?: (t: { token: string; refresh_token: string }) => void;
  fetch?: typeof fetch;
}

const REQUIRED: unique symbol = Symbol("required");

const list = <T extends z.ZodType>(item: T) => z.array(item);

/** Thin typed REST client; every response is validated with zod. */
export class ApiClient {
  constructor(private o: ApiClientOptions) {}

  get baseUrl(): string {
    return this.o.baseUrl;
  }

  private refreshing: Promise<boolean> | null = null;

  /** Rotates the refresh token once; concurrent 401s share one refresh. */
  private refresh(): Promise<boolean> {
    const rt = this.o.getRefreshToken?.();
    if (!rt) return Promise.resolve(false);
    this.refreshing ??= (async () => {
      try {
        const f = this.o.fetch ?? fetch;
        const res = await f(this.o.baseUrl + "/api/auth/refresh", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ refresh_token: rt }) });
        const r = S.TokenPair.safeParse(await res.json().catch(() => null));
        if (!res.ok || !r.success) return false;
        this.o.onTokens?.(r.data);
        return true;
      } finally {
        setTimeout(() => (this.refreshing = null), 0);
      }
    })();
    return this.refreshing;
  }

  private async req<T>(method: string, path: string, schema: z.ZodType<T>, fallback: T | typeof REQUIRED, body?: unknown, retried = false): Promise<T> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    const tok = this.o.getToken();
    if (tok) headers.Authorization = `Bearer ${tok}`;
    const ws = this.o.getWorkspaceId();
    if (ws) headers["X-Workspace-ID"] = ws;
    const f = this.o.fetch ?? fetch;
    const res = await f(this.o.baseUrl + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    const data: unknown = await res.json().catch(() => ({}));
    if (res.status === 401 && !retried && !path.startsWith("/api/auth/") && (await this.refresh())) {
      return this.req(method, path, schema, fallback, body, true);
    }
    if (!res.ok) {
      const e = S.parseWithFallback(S.ApiError, data, { error: res.statusText, code: "common.http_" + res.status }, "error");
      if (res.status === 401) this.o.onUnauthorized?.();
      throw new ApiError(res.status, e.code, e.error);
    }
    if (fallback === REQUIRED) {
      const r = schema.safeParse(data);
      if (!r.success) throw new ApiError(0, "common.bad_response", "unexpected response from " + path);
      return r.data;
    }
    return S.parseWithFallback(schema, data, fallback, path);
  }

  devLogin(input: { email: string; name?: string; handle?: string; invite_code?: string; password?: string }) {
    return this.req("POST", "/api/auth/dev-login", S.LoginResult, { token: "", user_id: "", name: "", email: "", handle: "", workspace_id: "" }, input);
  }
  me() {
    return this.req("GET", "/api/me", S.Me, { id: "", name: "", email: "", handle: "", workspaces: [] });
  }
  members() {
    return this.req("GET", "/api/members", list(S.Member), []);
  }
  projects() {
    return this.req("GET", "/api/projects", list(S.Project), []);
  }
  createProject(input: { name: string; remote_url: string; default_branch?: string; provider?: string; lead_id?: string }) {
    return this.req("POST", "/api/projects", S.Project, REQUIRED, input);
  }
  rooms() {
    return this.req("GET", "/api/rooms", list(S.Room), []);
  }
  messages(roomId: string, after = 0) {
    return this.req("GET", `/api/rooms/${roomId}/messages?after=${after}`, list(S.Message), []);
  }
  sendMessage(roomId: string, input: { client_msg_id: string; body: string }) {
    return this.req("POST", `/api/rooms/${roomId}/messages`, S.Message, REQUIRED, input);
  }
  confirmDraft(messageId: string, draft?: S.Draft) {
    return this.req("POST", `/api/messages/${messageId}/confirm-draft`, z.object({ task: S.Task, duplicate: z.boolean().catch(false) }), REQUIRED, draft ? { draft } : {});
  }
  discardDraft(messageId: string) {
    return this.req("POST", `/api/messages/${messageId}/discard-draft`, z.unknown(), null);
  }
  agents() {
    return this.req("GET", "/api/agents", list(S.Agent), []);
  }
  updateIntake(agentId: string, input: { mode: string; allowlist: string[]; daily_quota: number }) {
    return this.req("PATCH", `/api/agents/${agentId}/intake`, S.Agent, REQUIRED, input);
  }
  daemons() {
    return this.req("GET", "/api/daemons", list(S.Daemon), []);
  }
  pair(input: { name: string; agents: { handle: string; name?: string; kind: string }[] }) {
    return this.req("POST", "/api/daemons", S.PairResult, REQUIRED, input);
  }
  tasks() {
    return this.req("GET", "/api/tasks", list(S.Task), []);
  }
  task(id: string) {
    return this.req("GET", `/api/tasks/${id}`, S.Task, REQUIRED);
  }
  taskEvents(id: string, after = 0) {
    return this.req("GET", `/api/tasks/${id}/events?after=${after}`, list(S.TaskEvent), []);
  }
  decideIntake(taskId: string, decision: "accept" | "reject", comment = "", modelKey: "source" | "target" = "source") {
    return this.req("POST", `/api/tasks/${taskId}/intake`, S.Task, REQUIRED, { decision, comment, model_key: modelKey });
  }
  cancelTask(taskId: string) {
    return this.req("POST", `/api/tasks/${taskId}/cancel`, S.Task, REQUIRED);
  }
  decidePermission(permissionId: string, decision: "allow" | "deny", comment = "") {
    return this.req("POST", `/api/permissions/${permissionId}/decide`, S.Task, REQUIRED, { decision, comment });
  }
  pullRequests() {
    return this.req("GET", "/api/pull-requests", list(S.PullRequest), []);
  }
  decideMerge(approvalId: string, decision: "approve" | "reject", comment = "") {
    return this.req("POST", `/api/approvals/${approvalId}/decide`, S.Task, REQUIRED, { decision, comment });
  }
  memory(projectId: string) {
    return this.req("GET", `/api/projects/${projectId}/memory`, list(S.MemoryVersion), []);
  }
  decideMemory(memoryId: string, decision: "confirm" | "reject", comment = "") {
    return this.req("POST", `/api/memory/${memoryId}/decide`, S.MemoryVersion, REQUIRED, { decision, comment });
  }
  // ---- V0.4 ----
  logout(refreshToken?: string) {
    return this.req("POST", "/api/auth/logout", z.unknown(), null, { refresh_token: refreshToken ?? "" });
  }
  apiTokens() {
    return this.req("GET", "/api/tokens", list(S.ApiToken), []);
  }
  createApiToken(label: string) {
    return this.req("POST", "/api/tokens", S.ApiToken, REQUIRED, { label });
  }
  revokeApiToken(id: string) {
    return this.req("DELETE", `/api/tokens/${id}`, z.unknown(), null);
  }
  createDelegation(input: {
    room_id: string;
    project_id?: string;
    target_agent_id: string;
    title: string;
    prompt: string;
    grant: { resources: string[]; actions: string[]; network?: string[] };
    budget?: { max_tokens: number; max_duration_min: number };
    approvers?: { id: string; name?: string }[];
    depends_on?: string[];
    approval_rule?: "ANY_ONE" | "ALL";
    acceptance?: string[];
    idempotency_key?: string;
  }) {
    return this.req("POST", "/api/tasks", z.object({ task: S.Task, duplicate: z.boolean().catch(false) }), REQUIRED, input);
  }
  updateProjectSettings(projectId: string, input: { visibility?: string; test_command?: string; webhook_secret?: string }) {
    return this.req("PATCH", `/api/projects/${projectId}/settings`, S.Project, REQUIRED, input);
  }
  projectMembers(projectId: string) {
    return this.req("GET", `/api/projects/${projectId}/members`, list(S.ProjectMember), []);
  }
  setProjectMember(projectId: string, userId: string, role: "lead" | "writer" | "viewer") {
    return this.req("POST", `/api/projects/${projectId}/members`, z.unknown(), null, { user_id: userId, role });
  }
  removeProjectMember(projectId: string, userId: string) {
    return this.req("DELETE", `/api/projects/${projectId}/members/${userId}`, z.unknown(), null);
  }
  editMemory(projectId: string, content: string) {
    return this.req("POST", `/api/projects/${projectId}/memory`, S.MemoryVersion, REQUIRED, { content });
  }
  taintMemory(memoryId: string, reason: string) {
    return this.req("POST", `/api/memory/${memoryId}/taint`, S.TaintReport, REQUIRED, { reason });
  }
  memoryReferences(memoryId: string) {
    return this.req("GET", `/api/memory/${memoryId}/references`, list(S.Task), []);
  }
  richAction(messageId: string, actionId: string, value?: unknown) {
    return this.req("POST", `/api/messages/${messageId}/actions`, S.Message, REQUIRED, { action_id: actionId, value: value ?? null });
  }
  postToTask(messageId: string, targetAgentId: string) {
    return this.req("POST", `/api/messages/${messageId}/to-task`, z.unknown(), null, { target_agent_id: targetAgentId });
  }
  updateAgentPolicy(agentId: string, input: { autonomy: string; max_concurrent: number; max_duration_min: number; allowed_actions: string[]; allow_network: boolean; intake_timeout_min: number; daily_cost_units: number }) {
    return this.req("PATCH", `/api/agents/${agentId}/policy`, S.Agent, REQUIRED, input);
  }
  startPatrol(agentId: string, roomId: string, prompt: string) {
    return this.req("POST", `/api/agents/${agentId}/patrol`, S.Task, REQUIRED, { room_id: roomId, prompt });
  }
  revokeDaemon(daemonId: string) {
    return this.req("DELETE", `/api/daemons/${daemonId}`, z.unknown(), null);
  }
  nudge(taskId: string) {
    return this.req("POST", `/api/tasks/${taskId}/nudge`, z.unknown(), null);
  }
  expandBudget(taskId: string, maxTokens: number, comment = "") {
    return this.req("POST", `/api/tasks/${taskId}/budget`, S.Task, REQUIRED, { max_tokens: maxTokens, comment });
  }
  trace(traceId: string) {
    return this.req("GET", `/api/traces/${traceId}`, S.TraceView, REQUIRED);
  }
  usage() {
    return this.req("GET", "/api/usage", S.UsageView, REQUIRED);
  }
  setBudget(scope: "workspace" | "user", maxTokens: number) {
    return this.req("PUT", "/api/budgets", S.BudgetRow, REQUIRED, { scope, max_tokens: maxTokens });
  }
  setIM(kind: "slack" | "feishu" | "dingtalk" | "", url: string) {
    return this.req("PATCH", "/api/workspace/im", z.unknown(), null, { kind, url });
  }
  audit() {
    return this.req("GET", "/api/audit", list(S.AuditEntry), []);
  }
}
