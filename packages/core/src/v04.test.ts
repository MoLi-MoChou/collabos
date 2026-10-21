import { describe, expect, it } from "vitest";
import { ApiClient, createCollabClient, diffLines, parseCard, splitFences, validReason } from "./index";

describe("text helpers", () => {
  it("splits fences", () => {
    const p = splitFences("a\n```diff\n+x\n-y\n```\nb");
    expect(p.map((x) => x.code === null)).toEqual([true, false, true]);
    expect(p[1]).toMatchObject({ lang: "diff", code: "+x\n-y" });
  });
  it("classifies diff lines", () => {
    expect(diffLines("diff --git a b\n@@ -1 +1 @@\n+a\n-b\n c").map((l) => l.kind)).toEqual(["file", "hunk", "add", "del", "ctx"]);
  });
  it("requires a reason", () => {
    expect(validReason(" ")).toBe(false);
    expect(validReason("wrong approach")).toBe(true);
  });
});

describe("V0.4 cards", () => {
  it("parses budget, nudge and agent_post cards", () => {
    expect(parseCard({ type: "budget", task_id: "t", scope: "task", used: 11, max: 10, eligible: ["u"] })).toMatchObject({ state: "pending", used: 11 });
    expect(parseCard({ type: "nudge", task_id: "t", by: "u", waiting_on: null })).toMatchObject({ waiting_on: [] });
    expect(parseCard({ type: "agent_post", task_id: "t", agent_id: "a", owner_id: "o" })).toMatchObject({ state: "open" });
    expect(parseCard({ type: "merge", task_id: "t", approval_id: "a", rule: "ALL", decisions: [{ by: "u", decision: "approve" }] })).toMatchObject({ rule: "ALL" });
  });
});

function fakeFetch(handler: (url: string, init: RequestInit) => { status: number; body: unknown }) {
  const calls: { url: string; auth: string }[] = [];
  const f = (async (url: string, init: RequestInit) => {
    const h = new Headers(init.headers);
    calls.push({ url, auth: h.get("Authorization") ?? "" });
    const r = handler(url, init);
    return new Response(JSON.stringify(r.body), { status: r.status });
  }) as unknown as typeof fetch;
  return { f, calls };
}

describe("token refresh", () => {
  it("rotates once on 401 and retries", async () => {
    let token = "old";
    let refreshed = 0;
    const { f, calls } = fakeFetch((url, init) => {
      if (url.endsWith("/api/auth/refresh")) {
        refreshed++;
        expect(JSON.parse(String(init.body))).toEqual({ refresh_token: "r1" });
        return { status: 200, body: { token: "new", refresh_token: "r2", expires_at: "" } };
      }
      const auth = new Headers(init.headers).get("Authorization");
      return auth === "Bearer new" ? { status: 200, body: [] } : { status: 401, body: { error: "expired", code: "auth.unauthorized" } };
    });
    let out = "";
    const api = new ApiClient({ baseUrl: "", fetch: f, getToken: () => token, getWorkspaceId: () => "w", getRefreshToken: () => "r1", onTokens: (t) => ((token = t.token), (out = t.refresh_token)) });
    await Promise.all([api.tasks(), api.agents()]);
    expect(refreshed).toBe(1);
    expect(out).toBe("r2");
    expect(calls.filter((c) => c.auth === "Bearer new").length).toBe(2);
  });
  it("logs out when refresh fails", async () => {
    let gone = false;
    const { f } = fakeFetch(() => ({ status: 401, body: { error: "x", code: "auth.unauthorized" } }));
    const api = new ApiClient({ baseUrl: "", fetch: f, getToken: () => "t", getWorkspaceId: () => "w", getRefreshToken: () => "r", onUnauthorized: () => (gone = true) });
    await expect(api.me()).rejects.toThrow();
    expect(gone).toBe(true);
  });
});

describe("SDK", () => {
  it("sends the API token and workspace", async () => {
    const { f, calls } = fakeFetch(() => ({ status: 200, body: { period: "2026-10", total: 5, by_requester: null, by_owner: [], by_agent: [], budgets: [] } }));
    const c = createCollabClient({ baseUrl: "https://x.test/", token: "coa_abc", workspaceId: "w", fetch: f });
    const u = await c.usage();
    expect(u.total).toBe(5);
    expect(u.by_requester).toEqual([]);
    expect(calls[0]).toEqual({ url: "https://x.test/api/usage", auth: "Bearer coa_abc" });
  });
});

import { QueryClient } from "@tanstack/react-query";
import { attachSnapshotCache, snapshotOf, type SnapshotStore } from "./cache";

describe("snapshot cache", () => {
  it("only snapshots workspace data and trims messages", () => {
    expect(snapshotOf(["me"], {})).toBeUndefined();
    expect((snapshotOf(["ws", "w", "messages", "r"], Array.from({ length: 300 }, (_, i) => i)) as number[]).length).toBe(200);
  });
  it("persists and hydrates", async () => {
    const m = new Map<string, unknown>();
    const store: SnapshotStore = { get: async (k) => m.get(k), set: async (k, v) => void m.set(k, v) };
    const a = new QueryClient();
    const stop = await attachSnapshotCache(a, store, "w");
    a.setQueryData(["ws", "w", "tasks"], [{ id: "t1" }]);
    await new Promise((r) => setTimeout(r, 600));
    stop();
    const b = new QueryClient();
    await attachSnapshotCache(b, store, "w");
    expect(b.getQueryData(["ws", "w", "tasks"])).toEqual([{ id: "t1" }]);
  });
});
