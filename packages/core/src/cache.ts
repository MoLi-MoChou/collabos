import type { QueryClient } from "@tanstack/react-query";

/**
 * Offline snapshot cache (FR-CLI-401): the latest rooms, messages and tasks
 * are written to IndexedDB per workspace and hydrated on start, so the app
 * shows the last known state immediately (and while the server is down).
 * Server data still wins: hydrated entries are marked stale and refetched.
 */
export interface SnapshotStore {
  get(key: string): Promise<unknown>;
  set(key: string, value: unknown): Promise<void>;
}

const CACHED = new Set(["rooms", "messages", "tasks", "agents", "members", "projects"]);
const MAX_MESSAGES = 200;

export function idbStore(name = "collabos-cache"): SnapshotStore | null {
  if (typeof indexedDB === "undefined") return null;
  const db = new Promise<IDBDatabase>((resolve, reject) => {
    const r = indexedDB.open(name, 1);
    r.onupgradeneeded = () => r.result.createObjectStore("kv");
    r.onsuccess = () => resolve(r.result);
    r.onerror = () => reject(r.error ?? new Error("indexedDB open failed"));
  });
  const run = <T,>(mode: IDBTransactionMode, f: (s: IDBObjectStore) => IDBRequest<T>) =>
    db.then(
      (d) =>
        new Promise<T>((resolve, reject) => {
          const req = f(d.transaction("kv", mode).objectStore("kv"));
          req.onsuccess = () => resolve(req.result);
          req.onerror = () => reject(req.error ?? new Error("indexedDB request failed"));
        }),
    );
  return {
    get: (k) => run("readonly", (s) => s.get(k)),
    set: (k, v) => run("readwrite", (s) => s.put(v, k)).then(() => undefined),
  };
}

/** Which query keys are snapshotted; messages are trimmed to the newest MAX_MESSAGES. */
export function snapshotOf(key: readonly unknown[], data: unknown): unknown {
  if (key[0] !== "ws" || !CACHED.has(String(key[2]))) return undefined;
  if (key[2] === "messages" && Array.isArray(data)) return data.slice(-MAX_MESSAGES);
  return data;
}

/** Hydrates the query client for a workspace, then keeps the snapshot updated. */
export async function attachSnapshotCache(qc: QueryClient, store: SnapshotStore, ws: string): Promise<() => void> {
  const raw = await store.get(`index:${ws}`).catch(() => null);
  const index = Array.isArray(raw) ? raw.filter((x): x is string => typeof x === "string") : null;
  for (const k of index ?? []) {
    const v = await store.get(k).catch(() => undefined);
    const key: unknown = JSON.parse(k.slice(k.indexOf(":") + 1));
    if (v !== undefined && Array.isArray(key) && qc.getQueryData(key) === undefined) qc.setQueryData(key, v, { updatedAt: 1 });
  }
  const keys = new Set(index ?? []);
  let timer: ReturnType<typeof setTimeout> | undefined;
  const dirty = new Map<string, unknown>();
  const flush = () => {
    for (const [k, v] of dirty) void store.set(k, v).catch(() => undefined);
    dirty.clear();
    void store.set(`index:${ws}`, [...keys]).catch(() => undefined);
  };
  return qc.getQueryCache().subscribe((e) => {
    if (e.type !== "updated" || e.query.queryKey[1] !== ws) return;
    const snap = snapshotOf(e.query.queryKey, e.query.state.data);
    if (snap === undefined) return;
    const k = `q:${JSON.stringify(e.query.queryKey)}`;
    keys.add(k);
    dirty.set(k, snap);
    clearTimeout(timer);
    timer = setTimeout(flush, 500);
  });
}
