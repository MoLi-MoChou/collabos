import { create } from "zustand";
import { z } from "zod";
import type { StorageAdapter } from "./storage";
import { parseWithFallback } from "./api/schemas";

/** Persisted login session. */
export const Session = z.object({
  token: z.string(),
  userId: z.string(),
  name: z.string(),
  handle: z.string(),
  workspaceId: z.string(),
});
export type Session = z.infer<typeof Session>;

export type DockTab = "tasks" | "people" | "memory" | "audit";
export type Theme = "light" | "dark";

interface AppState {
  session: Session | null;
  theme: Theme;
  dockTab: DockTab;
  dockOpen: boolean;
  selectedTaskId: string | null;
  connected: boolean;
  setSession(s: Session | null): void;
  setTheme(t: Theme): void;
  setDockTab(t: DockTab): void;
  toggleDock(): void;
  selectTask(id: string | null): void;
  setConnected(v: boolean): void;
}

const KEY_SESSION = "collabos.session";
const KEY_PREFS = "collabos.prefs";
const Prefs = z.object({ theme: z.enum(["light", "dark"]).catch("light"), dockOpen: z.boolean().catch(true) });

function readJSON(storage: StorageAdapter, key: string): unknown {
  try {
    const v = storage.get(key);
    return v ? JSON.parse(v) : null;
  } catch {
    return null;
  }
}

/** Creates the client-state store. Server data never goes here (TanStack Query owns it). */
export function createAppStore(storage: StorageAdapter) {
  const session = parseWithFallback(Session.nullable(), readJSON(storage, KEY_SESSION), null, "session");
  const prefs = parseWithFallback(Prefs, readJSON(storage, KEY_PREFS) ?? {}, { theme: "light" as const, dockOpen: true }, "prefs");
  return create<AppState>((set, get) => {
    const savePrefs = () => storage.set(KEY_PREFS, JSON.stringify({ theme: get().theme, dockOpen: get().dockOpen }));
    return {
      session,
      theme: prefs.theme,
      dockTab: "tasks",
      dockOpen: prefs.dockOpen,
      selectedTaskId: null,
      connected: false,
      setSession(s) {
        if (s) storage.set(KEY_SESSION, JSON.stringify(s));
        else storage.remove(KEY_SESSION);
        set({ session: s, selectedTaskId: null });
      },
      setTheme(theme) {
        set({ theme });
        savePrefs();
      },
      setDockTab(dockTab) {
        set({ dockTab, dockOpen: true });
        savePrefs();
      },
      toggleDock() {
        set({ dockOpen: !get().dockOpen });
        savePrefs();
      },
      selectTask(id) {
        set({ selectedTaskId: id, ...(id ? { dockTab: "tasks" as const, dockOpen: true } : {}) });
      },
      setConnected(connected) {
        set({ connected });
      },
    };
  });
}

export type AppStore = ReturnType<typeof createAppStore>;
