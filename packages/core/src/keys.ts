/** Query keys. Workspace-scoped data always carries wsId. */
export const qk = {
  me: () => ["me"] as const,
  members: (ws: string) => ["ws", ws, "members"] as const,
  projects: (ws: string) => ["ws", ws, "projects"] as const,
  rooms: (ws: string) => ["ws", ws, "rooms"] as const,
  messages: (ws: string, room: string) => ["ws", ws, "messages", room] as const,
  agents: (ws: string) => ["ws", ws, "agents"] as const,
  daemons: (ws: string) => ["ws", ws, "daemons"] as const,
  tasks: (ws: string) => ["ws", ws, "tasks"] as const,
  task: (ws: string, id: string) => ["ws", ws, "task", id] as const,
  events: (ws: string, id: string) => ["ws", ws, "events", id] as const,
  prs: (ws: string) => ["ws", ws, "prs"] as const,
  memory: (ws: string, project: string) => ["ws", ws, "memory", project] as const,
  audit: (ws: string) => ["ws", ws, "audit"] as const,
  usage: (ws: string) => ["ws", ws, "usage"] as const,
  trace: (ws: string, id: string) => ["ws", ws, "trace", id] as const,
  projectMembers: (ws: string, project: string) => ["ws", ws, "project-members", project] as const,
};
