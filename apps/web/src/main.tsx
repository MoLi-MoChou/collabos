import { StrictMode, useEffect, useMemo, useState } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider, useQueryClient } from "@tanstack/react-query";
import { createRootRoute, createRoute, createRouter, Outlet, RouterProvider, useNavigate, useParams } from "@tanstack/react-router";
import { z } from "zod";
import { ApiClient, applyEnvelope, createAppStore, CoreProvider, initI18n, RealtimeClient, type StorageAdapter } from "@collabos/core";
import { AppShell, LoginView, NavigationProvider, resources, type NavigationAdapter } from "@collabos/views";
import "./app.css";

const storage: StorageAdapter = {
  get: (k) => window.localStorage.getItem(k),
  set: (k, v) => window.localStorage.setItem(k, v),
  remove: (k) => window.localStorage.removeItem(k),
};
const lang = window.localStorage.getItem("collabos.lang") ?? (navigator.language.startsWith("zh") ? "zh-Hans" : "en");
initI18n(resources, lang);

const store = createAppStore(storage);
const api = new ApiClient({
  baseUrl: "",
  getToken: () => store.getState().session?.token ?? null,
  getWorkspaceId: () => store.getState().session?.workspaceId ?? null,
  onUnauthorized: () => store.getState().setSession(null),
  getRefreshToken: () => store.getState().session?.refreshToken ?? null,
  onTokens: (t) => {
    const s = store.getState().session;
    if (s) store.getState().setSession({ ...s, token: t.token, refreshToken: t.refresh_token });
  },
});
const queryClient = new QueryClient({ defaultOptions: { queries: { staleTime: 30_000, retry: 1 } } });

const Config = z.object({ password_required: z.boolean().catch(false) });

function useTheme() {
  const theme = store((s) => s.theme);
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
  }, [theme]);
}

function Realtime() {
  const session = store((s) => s.session);
  const qc = useQueryClient();
  useEffect(() => {
    if (!session) return;
    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const rt = new RealtimeClient({
      url: `${proto}//${window.location.host}/ws`,
      token: session.token,
      workspaceId: session.workspaceId,
      onEnvelope: (e) => applyEnvelope(qc, session.workspaceId, e),
      onStatus: (c) => store.getState().setConnected(c),
      onResync: () => void qc.invalidateQueries(),
    });
    rt.start();
    return () => rt.stop();
  }, [session, qc]);
  return null;
}

function Gate({ roomId }: { roomId: string | null }) {
  useTheme();
  const session = store((s) => s.session);
  const navigate = useNavigate();
  const [needsPassword, setNeedsPassword] = useState(false);
  useEffect(() => {
    void fetch("/api/config")
      .then((r) => r.json())
      .then((j: unknown) => setNeedsPassword(Config.parse(j).password_required))
      .catch(() => undefined);
  }, []);
  const nav = useMemo<NavigationAdapter>(
    () => ({
      roomId,
      goRoom: (id) => void navigate({ to: "/rooms/$roomId", params: { roomId: id } }),
      goHome: () => void navigate({ to: "/" }),
    }),
    [roomId, navigate],
  );
  return (
    <NavigationProvider value={nav}>
      {session ? (
        <>
          <Realtime />
          <AppShell key={session.workspaceId + session.userId} />
        </>
      ) : (
        <LoginView needsPassword={needsPassword} />
      )}
    </NavigationProvider>
  );
}

const rootRoute = createRootRoute({ component: () => <Outlet /> });
const homeRoute = createRoute({ getParentRoute: () => rootRoute, path: "/", component: () => <Gate roomId={null} /> });
function RoomPage() {
  const { roomId } = useParams({ from: "/rooms/$roomId" });
  return <Gate roomId={roomId} />;
}
const roomRoute = createRoute({ getParentRoute: () => rootRoute, path: "/rooms/$roomId", component: RoomPage });
const router = createRouter({ routeTree: rootRoute.addChildren([homeRoute, roomRoute]) });

declare module "@tanstack/react-router" {
  interface Register {
    router: typeof router;
  }
}

const el = document.getElementById("root");
if (el) {
  createRoot(el).render(
    <StrictMode>
      <QueryClientProvider client={queryClient}>
        <CoreProvider value={{ api, store }}>
          <RouterProvider router={router} />
        </CoreProvider>
      </QueryClientProvider>
    </StrictMode>,
  );
}
