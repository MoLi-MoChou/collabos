import { createContext, useContext, type ReactNode } from "react";
import type { ApiClient } from "./api/client";
import type { AppStore } from "./store";

export interface CoreContext {
  api: ApiClient;
  store: AppStore;
}

const Ctx = createContext<CoreContext | null>(null);

export function CoreProvider({ value, children }: { value: CoreContext; children: ReactNode }) {
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useCore(): CoreContext {
  const c = useContext(Ctx);
  if (!c) throw new Error("CoreProvider missing");
  return c;
}

export function useApi(): ApiClient {
  return useCore().api;
}

