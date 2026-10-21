import { describe, expect, it } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { z } from "zod";
import { Message, Task, parseCard, parseWithFallback } from "./api/schemas";
import { applyEnvelope, upsertMessage } from "./realtime/apply";
import { completeMention, mentionQuery, parseMentions, segments } from "./mentions";
import { qk } from "./keys";
import { createAppStore } from "./store";
import { memoryStorage } from "./storage";

const msg = (seq: number, id = `m${seq}`) => Message.parse({ id, room_id: "r", seq, sender_type: "user", body: `b${seq}`, mentions: [], card: null });

describe("schemas", () => {
  it("unknown status falls back to unknown", () => {
    const t = Task.parse({ id: "t", status: "teleporting", grant: {}, budget: {}, approvers: [], result: {} });
    expect(t.status).toBe("unknown");
  });
  it("parseWithFallback returns fallback on garbage", () => {
    expect(parseWithFallback(z.array(z.string()), { nope: 1 }, ["x"])).toEqual(["x"]);
  });
  it("cards parse by type and ignore unknown types", () => {
    expect(parseCard({ type: "task", task_id: "t1" })?.type).toBe("task");
    expect(parseCard({ type: "hologram" })).toBeNull();
    expect(parseCard(null)).toBeNull();
  });
});

describe("realtime", () => {
  it("upsertMessage dedupes and orders by seq", () => {
    const l = upsertMessage(upsertMessage([msg(2)], msg(1)), { ...msg(2), body: "edited" });
    expect(l.map((m) => m.seq)).toEqual([1, 2]);
    expect(l[1]?.body).toBe("edited");
  });
  it("ROOM_MESSAGE patches the cache", () => {
    const qc = new QueryClient();
    qc.setQueryData(qk.messages("w", "r"), [msg(1)]);
    applyEnvelope(qc, "w", { type: "ROOM_MESSAGE", trace_id: "", msg_id: "x", timestamp: "", payload: msg(2) });
    expect(qc.getQueryData<Message[]>(qk.messages("w", "r"))?.length).toBe(2);
  });
});

describe("mentions", () => {
  it("parses unique handles", () => {
    expect(parseMentions("@Bob-coder hi @bob-coder and @li")).toEqual(["bob-coder", "li"]);
  });
  it("autocompletes at caret", () => {
    const text = "ask @bo";
    expect(mentionQuery(text, text.length)).toBe("bo");
    expect(completeMention(text, text.length, "bob-coder").text).toBe("ask @bob-coder ");
    expect(mentionQuery("email a@b", 9)).toBeNull();
  });
  it("segments text", () => {
    expect(segments("hi @x1 there").map((s) => s.mention)).toEqual([false, true, false]);
  });
});

describe("store", () => {
  it("persists session and prefs through the storage adapter", () => {
    const st = memoryStorage();
    const s = createAppStore(st);
    s.getState().setSession({ token: "t", userId: "u", name: "n", handle: "h", workspaceId: "w" });
    s.getState().setTheme("dark");
    const again = createAppStore(st);
    expect(again.getState().session?.workspaceId).toBe("w");
    expect(again.getState().theme).toBe("dark");
  });
});
