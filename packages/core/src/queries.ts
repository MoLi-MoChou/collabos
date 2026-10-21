import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useApi, useCore } from "./context";
import { qk } from "./keys";
import type { Draft, Message } from "./api/schemas";
import { upsertMessage } from "./realtime/apply";

function useWs(): string {
  const { store } = useCore();
  return store((s) => s.session?.workspaceId ?? "");
}

export function useMe() {
  const api = useApi();
  return useQuery({ queryKey: qk.me(), queryFn: () => api.me() });
}
export function useMembers() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.members(ws), queryFn: () => api.members(), enabled: !!ws });
}
export function useProjects() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.projects(ws), queryFn: () => api.projects(), enabled: !!ws });
}
export function useRooms() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.rooms(ws), queryFn: () => api.rooms(), enabled: !!ws });
}
export function useMessages(roomId: string) {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.messages(ws, roomId), queryFn: () => api.messages(roomId), enabled: !!ws && !!roomId });
}
export function useAgents() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.agents(ws), queryFn: () => api.agents(), enabled: !!ws });
}
export function useDaemons() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.daemons(ws), queryFn: () => api.daemons(), enabled: !!ws });
}
export function useTasks() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.tasks(ws), queryFn: () => api.tasks(), enabled: !!ws });
}
export function useTaskEvents(taskId: string | null) {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.events(ws, taskId ?? ""), queryFn: () => api.taskEvents(taskId ?? ""), enabled: !!ws && !!taskId });
}
export function usePullRequests() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.prs(ws), queryFn: () => api.pullRequests(), enabled: !!ws });
}
export function useMemory(projectId: string) {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.memory(ws, projectId), queryFn: () => api.memory(projectId), enabled: !!ws && !!projectId });
}
export function useAudit() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.audit(ws), queryFn: () => api.audit(), enabled: !!ws, refetchOnMount: "always", refetchInterval: 5000 });
}

/** Sends a chat message; client_msg_id makes retries idempotent. */
export function useSendMessage(roomId: string) {
  const api = useApi();
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { body: string; clientMsgId: string }) => api.sendMessage(roomId, { body: v.body, client_msg_id: v.clientMsgId }),
    onSuccess: (m: Message) => qc.setQueryData<Message[]>(qk.messages(ws, roomId), (l) => upsertMessage(l, m)),
  });
}

/** Generic action mutation that refreshes tasks afterwards (server-confirmed, no optimism). */
function useAction<V>(fn: (v: V) => Promise<unknown>) {
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: fn,
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.tasks(ws) });
      void qc.invalidateQueries({ queryKey: qk.prs(ws) });
    },
  });
}

export function useConfirmDraft() {
  const api = useApi();
  return useAction((v: { messageId: string; draft?: Draft }) => api.confirmDraft(v.messageId, v.draft));
}
export function useDiscardDraft() {
  const api = useApi();
  return useAction((messageId: string) => api.discardDraft(messageId));
}
export function useDecideIntake() {
  const api = useApi();
  return useAction((v: { taskId: string; decision: "accept" | "reject"; comment?: string; modelKey?: "source" | "target" }) => api.decideIntake(v.taskId, v.decision, v.comment, v.modelKey));
}
export function useDecidePermission() {
  const api = useApi();
  return useAction((v: { permissionId: string; decision: "allow" | "deny"; comment?: string }) => api.decidePermission(v.permissionId, v.decision, v.comment));
}
export function useDecideMerge() {
  const api = useApi();
  return useAction((v: { approvalId: string; decision: "approve" | "reject"; comment?: string }) => api.decideMerge(v.approvalId, v.decision, v.comment));
}
export function useCancelTask() {
  const api = useApi();
  return useAction((taskId: string) => api.cancelTask(taskId));
}
export function useDecideMemory() {
  const api = useApi();
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { memoryId: string; decision: "confirm" | "reject"; comment?: string }) => api.decideMemory(v.memoryId, v.decision, v.comment),
    onSettled: () => void qc.invalidateQueries({ queryKey: ["ws", ws, "memory"] }),
  });
}
export function useUpdateIntake() {
  const api = useApi();
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { agentId: string; mode: string; allowlist: string[]; daily_quota: number }) =>
      api.updateIntake(v.agentId, { mode: v.mode, allowlist: v.allowlist, daily_quota: v.daily_quota }),
    onSettled: () => void qc.invalidateQueries({ queryKey: qk.agents(ws) }),
  });
}
export function usePair() {
  const api = useApi();
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { name: string; agents: { handle: string; kind: string }[] }) => api.pair(v),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.daemons(ws) });
      void qc.invalidateQueries({ queryKey: qk.agents(ws) });
    },
  });
}
export function useCreateProject() {
  const api = useApi();
  const ws = useWs();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (v: { name: string; remote_url: string; default_branch?: string }) => api.createProject(v),
    onSettled: () => {
      void qc.invalidateQueries({ queryKey: qk.projects(ws) });
      void qc.invalidateQueries({ queryKey: qk.rooms(ws) });
    },
  });
}

// ---- V0.4 ----
function useInvalidate() {
  const ws = useWs();
  const qc = useQueryClient();
  return (...keys: string[]) => keys.forEach((k) => void qc.invalidateQueries({ queryKey: ["ws", ws, k] }));
}
export function useUsage() {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.usage(ws), queryFn: () => api.usage(), enabled: !!ws });
}
export function useTrace(traceId: string | null) {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.trace(ws, traceId ?? ""), queryFn: () => api.trace(traceId ?? ""), enabled: !!ws && !!traceId });
}
export function useProjectMembers(projectId: string) {
  const api = useApi();
  const ws = useWs();
  return useQuery({ queryKey: qk.projectMembers(ws, projectId), queryFn: () => api.projectMembers(projectId), enabled: !!ws && !!projectId });
}
export function useApiTokens() {
  const api = useApi();
  return useQuery({ queryKey: ["tokens"], queryFn: () => api.apiTokens() });
}
export function useNudge() {
  const api = useApi();
  return useMutation({ mutationFn: (taskId: string) => api.nudge(taskId) });
}
export function useExpandBudget() {
  const api = useApi();
  return useAction((v: { taskId: string; maxTokens: number; comment?: string }) => api.expandBudget(v.taskId, v.maxTokens, v.comment));
}
export function useSetBudget() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({ mutationFn: (v: { scope: "workspace" | "user"; maxTokens: number }) => api.setBudget(v.scope, v.maxTokens), onSettled: () => inv("usage", "tasks") });
}
export function useRichAction() {
  const api = useApi();
  return useMutation({ mutationFn: (v: { messageId: string; actionId: string; value?: unknown }) => api.richAction(v.messageId, v.actionId, v.value) });
}
export function usePostToTask() {
  const api = useApi();
  return useMutation({ mutationFn: (v: { messageId: string; targetAgentId: string }) => api.postToTask(v.messageId, v.targetAgentId) });
}
export function useUpdateAgentPolicy() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({
    mutationFn: (v: { agentId: string } & Parameters<typeof api.updateAgentPolicy>[1]) => {
      const { agentId, ...rest } = v;
      return api.updateAgentPolicy(agentId, rest);
    },
    onSettled: () => inv("agents"),
  });
}
export function useStartPatrol() {
  const api = useApi();
  return useAction((v: { agentId: string; roomId: string; prompt: string }) => api.startPatrol(v.agentId, v.roomId, v.prompt));
}
export function useProjectSettings() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({
    mutationFn: (v: { projectId: string; visibility?: string; test_command?: string; webhook_secret?: string }) => {
      const { projectId, ...rest } = v;
      return api.updateProjectSettings(projectId, rest);
    },
    onSettled: () => inv("projects", "rooms"),
  });
}
export function useSetProjectMember() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({
    mutationFn: (v: { projectId: string; userId: string; role: "lead" | "writer" | "viewer" | null }) =>
      v.role ? api.setProjectMember(v.projectId, v.userId, v.role) : api.removeProjectMember(v.projectId, v.userId),
    onSettled: () => inv("project-members", "projects"),
  });
}
export function useEditMemory() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({ mutationFn: (v: { projectId: string; content: string }) => api.editMemory(v.projectId, v.content), onSettled: () => inv("memory") });
}
export function useTaintMemory() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({ mutationFn: (v: { memoryId: string; reason: string }) => api.taintMemory(v.memoryId, v.reason), onSettled: () => inv("memory", "tasks") });
}
export function useRevokeDaemon() {
  const api = useApi();
  const inv = useInvalidate();
  return useMutation({ mutationFn: (id: string) => api.revokeDaemon(id), onSettled: () => inv("daemons", "agents") });
}
