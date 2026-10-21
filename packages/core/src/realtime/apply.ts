import type { QueryClient } from "@tanstack/react-query";
import { Message, TaskEvent, parseWithFallback } from "../api/schemas";
import { qk } from "../keys";
import type { Envelope } from "./protocol.gen";
import { z } from "zod";

/** Inserts or replaces a message, keeping seq order. */
export function upsertMessage(list: Message[] | undefined, m: Message): Message[] {
  const out = (list ?? []).filter((x) => x.id !== m.id);
  out.push(m);
  return out.sort((a, b) => a.seq - b.seq);
}

/** Appends an event if its seq is new. */
export function appendEvent(list: TaskEvent[] | undefined, e: TaskEvent): TaskEvent[] {
  const cur = list ?? [];
  if (cur.some((x) => x.seq === e.seq)) return cur;
  return [...cur, e].sort((a, b) => a.seq - b.seq);
}

const DockChanged = z.object({ section: z.string() });

/**
 * Applies a realtime envelope to the query cache: patch where the payload is
 * complete, otherwise invalidate. Server data never goes to Zustand.
 */
export function applyEnvelope(qc: QueryClient, ws: string, e: Envelope): void {
  switch (e.type) {
    case "ROOM_MESSAGE": {
      const m = Message.safeParse(e.payload);
      if (!m.success) return;
      qc.setQueryData<Message[]>(qk.messages(ws, m.data.room_id), (l) => upsertMessage(l, m.data));
      return;
    }
    case "TASK_EVENT": {
      const ev = TaskEvent.safeParse(e.payload);
      if (!ev.success) return;
      qc.setQueryData<TaskEvent[]>(qk.events(ws, ev.data.task_id), (l) => (l ? appendEvent(l, ev.data) : l));
      return;
    }
    case "TASK_UPDATED": {
      const id = parseWithFallback(z.object({ task_id: z.string() }), e.payload, { task_id: "" }, "TASK_UPDATED").task_id;
      void qc.invalidateQueries({ queryKey: qk.tasks(ws) });
      if (id) void qc.invalidateQueries({ queryKey: qk.task(ws, id) });
      return;
    }
    case "DAEMON_STATE":
      void qc.invalidateQueries({ queryKey: qk.daemons(ws) });
      void qc.invalidateQueries({ queryKey: qk.agents(ws) });
      return;
    case "DOCK_CHANGED": {
      const s = parseWithFallback(DockChanged, e.payload, { section: "" }, "DOCK_CHANGED").section;
      const map: Record<string, readonly unknown[][]> = {
        tasks: [[...qk.tasks(ws)], [...qk.prs(ws)]],
        devices: [[...qk.daemons(ws)], [...qk.agents(ws)]],
        members: [[...qk.members(ws)]],
        projects: [[...qk.projects(ws)], [...qk.rooms(ws)]],
        memory: [["ws", ws, "memory"]],
      };
      for (const key of map[s] ?? []) void qc.invalidateQueries({ queryKey: key });
      void qc.invalidateQueries({ queryKey: qk.audit(ws) });
      return;
    }
  }
}
