import { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useDaemons, useMe, useProjects, useRooms, useCore } from "@collabos/core";
import { Avatar, Button, Dot, Spinner } from "@collabos/ui";
import { IconDevice, IconHash, IconLogo, IconLogout, IconMoon, IconSun } from "./icons";
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
      <main className="flex min-w-0 flex-1 flex-col bg-bg">
        {!projects.data?.length || !roomId ? <SetupView /> : <ChatView roomId={roomId} />}
      </main>
      {ui.dockOpen && roomId && (
        <aside className="cos-rise flex w-95 shrink-0 flex-col border-l border-border bg-bg" data-testid="dock">
          <Dock roomId={roomId} />
        </aside>
      )}
      {!ui.connected && (
        <div data-testid="reconnecting" className="fixed bottom-24 left-1/2 -translate-x-1/2 rounded-pill bg-surface px-3 py-1 text-caption text-warn shadow-popover">
          {t("shell.reconnecting")}
        </div>
      )}
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
    <nav className="flex w-61 shrink-0 flex-col border-r border-border bg-rail">
      <div className="flex h-12 items-center gap-2 px-4">
        <IconLogo className="text-accent" />
        <span className="truncate text-body font-semibold" data-testid="workspace-name">{ws?.name ?? "CollabOS"}</span>
      </div>
      {ws && (
        <div className="mx-3 mb-2 flex items-center justify-between rounded-card px-2 py-1 text-tiny text-fg-subtle">
          <span>{t("shell.invite")}</span>
          <span className="rounded-sm bg-tag px-1.5 py-px font-mono text-tag-fg" data-testid="invite-code">{ws.invite_code}</span>
        </div>
      )}
      <div className="px-4 pt-2 pb-1 text-tiny font-medium text-fg-subtle">{t("shell.rooms")}</div>
      <div className="flex flex-col gap-0.5 overflow-y-auto px-2">
        {(rooms.data ?? []).map((r) => {
          const active = r.id === activeRoom;
          return (
            <button
              key={r.id}
              onClick={() => nav.goRoom(r.id)}
              aria-current={active ? "page" : undefined}
              className={
                "flex cursor-pointer items-center gap-2.5 rounded-card px-2 py-2 text-left transition-colors " +
                (active ? "bg-active text-fg" : "text-fg hover:bg-hover")
              }
            >
              <span className="flex h-8 w-8 shrink-0 items-center justify-center rounded-pill bg-surface text-fg-muted shadow-card">
                <IconHash />
              </span>
              <span className="truncate text-body font-medium">{r.name}</span>
            </button>
          );
        })}
      </div>
      <div className="flex-1" />
      <div
        className="mx-3 mb-2 flex items-center gap-2 rounded-pill bg-surface px-3 py-2 shadow-card"
        data-testid="device-status"
        data-status={online.length ? "online" : "offline"}
        title={mine.map((d) => d.name).join(" · ")}
      >
        <span className="relative text-fg-muted">
          <IconDevice />
          <span className="absolute -right-0.5 -bottom-0.5"><Dot tone={online.length ? "success" : "neutral"} /></span>
        </span>
        <span className="min-w-0 flex-1">
          <span className="block text-caption font-medium">{online.length ? t("shell.deviceOnline") : t("shell.deviceOffline")}</span>
          <span className="block truncate text-tiny text-fg-subtle">{mine.length ? mine.map((d) => d.name).join(" · ") : t("shell.noDevice")}</span>
        </span>
      </div>
      <div className="flex items-center gap-1 px-3 pb-3">
        <Avatar name={session?.name ?? "?"} size="sm" />
        <span className="flex-1 truncate px-1 text-caption text-fg-muted">{session?.name}</span>
        <Button
          size="icon"
          variant="ghost"
          data-testid="theme-toggle"
          aria-label={ui.theme === "light" ? t("shell.dark") : t("shell.light")}
          title={ui.theme === "light" ? t("shell.dark") : t("shell.light")}
          onClick={() => ui.actions.setTheme(ui.theme === "light" ? "dark" : "light")}
        >
          {ui.theme === "light" ? <IconMoon /> : <IconSun />}
        </Button>
        <Button size="icon" variant="ghost" aria-label={t("shell.logout")} title={t("shell.logout")} onClick={() => store.getState().setSession(null)}>
          <IconLogout />
        </Button>
      </div>
    </nav>
  );
}
