import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useDaemons, useMe, useProjects, useRooms, useCore } from "@collabos/core";
import { Button, Dot, Spinner } from "@collabos/ui";
import { useNavigation } from "./navigation";
import { useMyId, useUi } from "./hooks";
import { ChatView } from "./chat/ChatView";
import { Dock } from "./dock/Dock";
import { SetupView } from "./SetupView";

/** Chat-first layout: left rail, room chat, right dock. */
export function AppShell() {
  const { t } = useTranslation();
  const nav = useNavigation();
  const rooms = useRooms();
  const projects = useProjects();
  const ui = useUi();

  const roomId = nav.roomId ?? rooms.data?.[0]?.id ?? null;
  useEffect(() => {
    if (!nav.roomId && rooms.data?.[0]) nav.goRoom(rooms.data[0].id);
  }, [nav, rooms.data]);

  if (projects.isLoading || rooms.isLoading) {
    return (
      <div className="flex h-full items-center justify-center">
        <Spinner />
      </div>
    );
  }
  return (
    <div className="flex h-full bg-bg text-fg">
      <Rail activeRoom={roomId} />
      <main className="flex min-w-0 flex-1 flex-col">
        {!projects.data?.length || !roomId ? <SetupView /> : <ChatView roomId={roomId} />}
      </main>
      {ui.dockOpen && roomId && (
        <aside className="flex w-96 shrink-0 flex-col border-l border-border bg-rail" data-testid="dock">
          <Dock roomId={roomId} />
        </aside>
      )}
      {!ui.connected && <div className="fixed bottom-3 left-1/2 -translate-x-1/2 rounded-pill bg-warn-soft px-3 py-1 text-caption text-warn">{t("shell.reconnecting")}</div>}
    </div>
  );
}

function Rail({ activeRoom }: { activeRoom: string | null }) {
  const { t } = useTranslation();
  const nav = useNavigation();
  const rooms = useRooms();
  const me = useMe();
  const daemons = useDaemons();
  const myId = useMyId();
  const { store } = useCore();
  const ui = useUi();
  const session = store((s) => s.session);
  const ws = me.data?.workspaces.find((w) => w.id === session?.workspaceId);
  const mine = (daemons.data ?? []).filter((d) => d.owner_id === myId);
  const online = mine.filter((d) => d.status === "online");
  return (
    <nav className="flex w-60 shrink-0 flex-col border-r border-border bg-rail">
      <div className="border-b border-border px-4 py-3">
        <div className="truncate text-title font-semibold" data-testid="workspace-name">{ws?.name ?? "CollabOS"}</div>
        {ws && (
          <div className="mt-0.5 text-caption text-fg-subtle">
            {t("shell.invite")} <span className="font-mono text-fg-muted" data-testid="invite-code">{ws.invite_code}</span>
          </div>
        )}
      </div>
      <div className="px-3 pt-3 pb-1 text-caption font-medium text-fg-subtle">{t("shell.rooms")}</div>
      <div className="flex flex-col gap-0.5 px-2">
        {(rooms.data ?? []).map((r) => (
          <button
            key={r.id}
            onClick={() => nav.goRoom(r.id)}
            className={
              "flex cursor-pointer items-center gap-2 rounded-control px-2 py-1.5 text-left text-body " +
              (r.id === activeRoom ? "bg-accent-soft text-accent" : "text-fg-muted hover:bg-surface-2")
            }
          >
            <span className="text-fg-subtle">#</span>
            <span className="truncate">{r.name}</span>
          </button>
        ))}
      </div>
      <div className="flex-1" />
      <div className="m-2 rounded-card border border-border bg-surface p-3" data-testid="device-status">
        <div className="flex items-center gap-2 text-body font-medium">
          <Dot tone={online.length ? "success" : "neutral"} />
          {online.length ? t("shell.deviceOnline") : t("shell.deviceOffline")}
        </div>
        <div className="mt-1 text-caption text-fg-subtle">
          {mine.length ? mine.map((d) => d.name).join(" · ") : t("shell.noDevice")}
        </div>
      </div>
      <div className="flex items-center gap-1 border-t border-border px-2 py-2">
        <span className="flex-1 truncate px-1 text-caption text-fg-muted">{session?.name}</span>
        <Button size="sm" variant="ghost" data-testid="theme-toggle" onClick={() => ui.actions.setTheme(ui.theme === "light" ? "dark" : "light")}>
          {ui.theme === "light" ? t("shell.dark") : t("shell.light")}
        </Button>
        <Button size="sm" variant="ghost" onClick={() => store.getState().setSession(null)}>
          {t("shell.logout")}
        </Button>
      </div>
    </nav>
  );
}
