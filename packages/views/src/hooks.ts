import { useCore, type Session } from "@collabos/core";

/** Current session (views must not import zustand directly). */
export function useSession(): Session | null {
  return useCore().store((s) => s.session);
}

export function useMyId(): string {
  return useCore().store((s) => s.session?.userId ?? "");
}

export function useUi() {
  const { store } = useCore();
  return {
    theme: store((s) => s.theme),
    dockTab: store((s) => s.dockTab),
    dockOpen: store((s) => s.dockOpen),
    selectedTaskId: store((s) => s.selectedTaskId),
    connected: store((s) => s.connected),
    actions: store.getState(),
  };
}
