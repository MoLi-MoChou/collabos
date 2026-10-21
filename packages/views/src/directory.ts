import { useMemo } from "react";
import { useAgents, useMembers, type Agent, type Member } from "@collabos/core";

export interface Directory {
  members: Member[];
  agents: Agent[];
  userName(id: string): string;
  agent(id: string): Agent | undefined;
  agentByHandle(h: string): Agent | undefined;
  ownerName(agent: Agent | undefined): string;
}

/** Resolves user and agent ids to display names. */
export function useDirectory(): Directory {
  const m = useMembers();
  const a = useAgents();
  return useMemo(() => {
    const members = m.data ?? [];
    const agents = a.data ?? [];
    const userName = (id: string) => members.find((x) => x.id === id)?.name ?? "—";
    return {
      members,
      agents,
      userName,
      agent: (id: string) => agents.find((x) => x.id === id),
      agentByHandle: (h: string) => agents.find((x) => x.handle === h),
      ownerName: (ag: Agent | undefined) => (ag ? userName(ag.owner_id) : "—"),
    };
  }, [m.data, a.data]);
}
