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
}

const REQUIRED: unique symbol = Symbol("required");

const list = <T extends z.ZodType>(item: T) => z.array(item);

/** Thin typed REST client; every response is validated with zod. */
export class ApiClient {
  constructor(private o: ApiClientOptions) {}

  get baseUrl(): string {
    return this.o.baseUrl;
  }

  private async req<T>(method: string, path: string, schema: z.ZodType<T>, fallback: T | typeof REQUIRED, body?: unknown): Promise<T> {
    const headers: Record<string, string> = { "Content-Type": "application/json" };
    const tok = this.o.getToken();
    if (tok) headers.Authorization = `Bearer ${tok}`;
    const ws = this.o.getWorkspaceId();
    if (ws) headers["X-Workspace-ID"] = ws;
    const res = await fetch(this.o.baseUrl + path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    const data: unknown = await res.json().catch(() => ({}));
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
    return this.req("POST", "/api/projects", S.Project, { id: "", name: "", remote_url: "", default_branch: "", provider: "", lead_id: "", room_id: "" }, input);
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
  decideIntake(taskId: string, decision: "accept" | "reject", comment = "") {
    return this.req("POST", `/api/tasks/${taskId}/intake`, S.Task, REQUIRED, { decision, comment });
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
  decideMemory(memoryId: string, decision: "confirm" | "reject") {
    return this.req("POST", `/api/memory/${memoryId}/decide`, S.MemoryVersion, REQUIRED, { decision });
  }
  audit() {
    return this.req("GET", "/api/audit", list(S.AuditEntry), []);
  }
}
