import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { isTerminal, useCancelTask, usePullRequests, useTaskEvents, useTasks, type Task, type TaskEvent } from "@collabos/core";
import { Badge, Button, Code } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId, useUi } from "../hooks";
import { StatusBadge } from "../cards/CardView";

export function TasksTab() {
  const { t } = useTranslation();
  const tasks = useTasks();
  const prs = usePullRequests();
  const ui = useUi();
  const dir = useDirectory();
  const list = (tasks.data ?? []).filter((x) => x.kind === "code");
  const selected = list.find((x) => x.id === ui.selectedTaskId);
  if (selected) return <TaskDetail task={selected} onBack={() => ui.actions.selectTask(null)} />;
  return (
    <div className="flex flex-col gap-4 p-3">
      <section>
        <div className="mb-2 px-1 text-caption font-medium text-fg-subtle">{t("dock.taskList")}</div>
        {!list.length && <div className="px-1 text-body text-fg-subtle">{t("dock.noTasks")}</div>}
        <div className="flex flex-col gap-1.5">
          {list.map((task) => (
            <button
              key={task.id}
              onClick={() => ui.actions.selectTask(task.id)}
              className="flex cursor-pointer flex-col gap-1 rounded-card border border-border bg-surface px-3 py-2 text-left hover:bg-surface-2"
              data-testid="dock-task"
            >
              <div className="flex items-center gap-2">
                <StatusBadge status={task.status} />
                <span className="truncate text-caption text-fg-subtle">@{dir.agent(task.target_agent_id)?.handle}</span>
              </div>
              <span className="line-clamp-2 text-body">{task.title}</span>
            </button>
          ))}
        </div>
      </section>
      <section>
        <div className="mb-2 px-1 text-caption font-medium text-fg-subtle">{t("dock.prs")}</div>
        {!prs.data?.length && <div className="px-1 text-body text-fg-subtle">{t("dock.noPrs")}</div>}
        {(prs.data ?? []).map((pr) => (
          <div key={pr.id} className="mb-1.5 flex items-center gap-2 rounded-card border border-border bg-surface px-3 py-2" data-testid="dock-pr">
            <span className="text-body font-medium">#{pr.number}</span>
            <Badge tone={pr.status === "merged" ? "success" : pr.status === "open" ? "accent" : "neutral"}>{t(`pr.${pr.status}`)}</Badge>
            <Code className="truncate">{pr.head_commit.slice(0, 8)}</Code>
          </div>
        ))}
      </section>
    </div>
  );
}

function eventLine(e: TaskEvent, t: (k: string, o?: Record<string, unknown>) => string): { text: string; tone: string } | null {
  const p = e.payload;
  const s = (k: string) => (typeof p[k] === "string" ? String(p[k]) : "");
  switch (e.type) {
    case "status":
      return { text: t("event.status", { to: t(`status.${s("to")}`) }) + (s("reason_code") ? ` (${s("reason_code")})` : ""), tone: "text-term-muted" };
    case "progress": {
      if (s("kind") === "phase") return { text: s("text"), tone: "text-term-fg" };
      if (s("kind") === "log") return { text: s("text"), tone: s("phase") === "message" ? "text-term-fg" : "text-term-muted" };
      const u = typeof p.update === "object" && p.update !== null ? p.update : {};
      const title = "title" in u && typeof u.title === "string" ? u.title : "";
      const status = "status" in u && typeof u.status === "string" ? u.status : "";
      return title ? { text: `› ${title} ${status}`, tone: "text-term-muted" } : null;
    }
    case "permission_requested":
      return { text: t("event.permission", { paths: Array.isArray(p.paths) ? p.paths.join(", ") : "" }), tone: "text-warn" };
    case "permission_decided":
      return { text: t("event.decided", { decision: s("decision") }), tone: "text-warn" };
    case "policy_blocked":
      return { text: t("event.blocked", { code: s("code"), target: s("target") }), tone: "text-danger" };
    case "pr_opened":
      return { text: t("event.pr"), tone: "text-success" };
    case "merged":
      return { text: t("event.merged", { commit: s("merge_commit").slice(0, 10) }), tone: "text-success" };
    case "completed":
    case "submitted":
    case "dispatched":
    case "queued":
    case "reconnected":
    case "daemon_lost":
      return { text: t(`event.${e.type}`), tone: "text-term-muted" };
    default:
      return null;
  }
}

function TaskDetail({ task, onBack }: { task: Task; onBack: () => void }) {
  const { t } = useTranslation();
  const ev = useTaskEvents(task.id);
  const dir = useDirectory();
  const me = useMyId();
  const cancel = useCancelTask();
  const end = useRef<HTMLDivElement>(null);
  const n = ev.data?.length ?? 0;
  useEffect(() => end.current?.scrollIntoView({ block: "end" }), [n]);
  const agent = dir.agent(task.target_agent_id);
  const canCancel = !isTerminal(task.status) && (task.created_by === me || task.target_owner_id === me);
  return (
    <div className="flex h-full flex-col gap-3 p-3" data-testid="task-detail">
      <div className="flex items-center gap-2">
        <Button size="sm" variant="ghost" onClick={onBack}>
          {t("common.back")}
        </Button>
        <StatusBadge status={task.status} />
        <div className="flex-1" />
        {canCancel && (
          <Button size="sm" variant="ghost" onClick={() => cancel.mutate(task.id)} data-testid="task-cancel">
            {t("dock.cancel")}
          </Button>
        )}
      </div>
      <div className="text-title font-semibold">{task.title}</div>
      <dl className="grid grid-cols-3 gap-x-2 gap-y-1 text-caption">
        <dt className="text-fg-subtle">{t("dock.requester")}</dt>
        <dd className="col-span-2">{dir.userName(task.created_by)}</dd>
        <dt className="text-fg-subtle">{t("dock.executor")}</dt>
        <dd className="col-span-2">{agent ? `@${agent.handle} · ${dir.ownerName(agent)}` : "—"}</dd>
        <dt className="text-fg-subtle">{t("card.grant")}</dt>
        <dd className="col-span-2 font-mono">{task.grant.resources.join(", ")}</dd>
        <dt className="text-fg-subtle">{t("dock.approvers")}</dt>
        <dd className="col-span-2">{task.approvers.map((a) => a.name).join("、")}</dd>
        <dt className="text-fg-subtle">{t("dock.tokens")}</dt>
        <dd className="col-span-2" data-testid="task-tokens">
          {task.used_tokens.toLocaleString()} / {task.budget.max_tokens.toLocaleString()}
        </dd>
        <dt className="text-fg-subtle">{t("dock.depth")}</dt>
        <dd className="col-span-2">
          {task.depth} / {task.max_depth}
        </dd>
        {task.reason_code && (
          <>
            <dt className="text-fg-subtle">{t("dock.reason")}</dt>
            <dd className="col-span-2 text-danger">{t(`reason.${task.reason_code}`, { defaultValue: task.reason_code })}</dd>
          </>
        )}
      </dl>
      <div className="min-h-0 flex-1 overflow-y-auto rounded-card bg-term-bg p-3 font-mono text-mono" data-testid="terminal">
        {(ev.data ?? []).map((e) => {
          const l = eventLine(e, t);
          if (!l) return null;
          return (
            <div key={e.seq} className={"whitespace-pre-wrap break-words " + l.tone}>
              <span className="text-term-muted">{new Date(e.created_at).toLocaleTimeString()} </span>
              {l.text}
            </div>
          );
        })}
        <div ref={end} />
      </div>
    </div>
  );
}
