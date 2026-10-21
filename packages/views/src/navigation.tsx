import { createContext, useContext, type ReactNode } from "react";

/** Platform navigation injected by the app; views never import a router. */
export interface NavigationAdapter {
  roomId: string | null;
  goRoom(roomId: string): void;
  goHome(): void;
}

const Ctx = createContext<NavigationAdapter | null>(null);

export function NavigationProvider({ value, children }: { value: NavigationAdapter; children: ReactNode }) {
  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useNavigation(): NavigationAdapter {
  const c = useContext(Ctx);
  if (!c) throw new Error("NavigationProvider missing");
  return c;
}
