import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  statusTone,
  useConfirmDraft,
  useDecideIntake,
  useDecideMemory,
  useDecideMerge,
  useDecidePermission,
  useDiscardDraft,
  useTasks,
  type Card as CardT,
  type DraftCard,
  type IntakeCard,
  type MemoryCard,
  type MergeCard,
  type NoticeCard,
  type PermissionCard,
  type Task,
} from "@collabos/core";
import { Badge, Button, Card, CardBody, CardFooter, CardHeader, Code, Input, Label, Textarea } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId, useUi } from "../hooks";

export function CardView({ card, messageId, body }: { card: CardT; messageId: string; body: string }) {
  switch (card.type) {
    case "delegation_draft":
      return <DraftView card={card} messageId={messageId} />;
    case "intake":
      return <IntakeView card={card} />;
    case "permission":
      return <PermissionView card={card} />;
    case "merge":
      return <MergeView card={card} />;
    case "memory":
      return <MemoryView card={card} />;
    case "task":
      return <TaskLine taskId={card.task_id} fallback={body} />;
    case "notice":
      return <NoticeView card={card} body={body} />;
  }
}

function useTask(id: string | undefined): Task | undefined {
  const tasks = useTasks();
  return id ? tasks.data?.find((t) => t.id === id) : undefined;
}

function ErrorText({ error }: { error: unknown }) {
  const { t } = useTranslation();
  if (!error) return null;
  return <div className="text-caption text-danger" data-testid="card-error">{error instanceof ApiError ? error.message : t("common.error")}</div>;
}

export function StatusBadge({ status }: { status: Task["status"] }) {
  const { t } = useTranslation();
  return (
    <Badge tone={statusTone(status)} data-testid="task-status" data-status={status}>
      {t(`status.${status}`)}
    </Badge>
  );
}

function TaskLine({ taskId, fallback }: { taskId: string; fallback: string }) {
  const { t } = useTranslation();
  const task = useTask(taskId);
  const ui = useUi();
  const dir = useDirectory();
  const agent = task ? dir.agent(task.target_agent_id) : undefined;
  return (
    <button
      className="flex w-full cursor-pointer items-center gap-2 rounded-card bg-surface px-3 py-2 text-left shadow-card transition-colors hover:bg-hover"
      onClick={() => ui.actions.selectTask(taskId)}
      data-testid="task-line"
      data-task-id={taskId}
    >
      {task && <StatusBadge status={task.status} />}
      <span className="min-w-0 flex-1 truncate text-body">{task ? task.title : fallback}</span>
      {agent && <span className="text-caption text-fg-subtle">@{agent.handle}</span>}
      <span className="text-caption text-accent">{t("card.details")}</span>
    </button>
  );
}

function GrantView({ resources, actions }: { resources: string[]; actions: string[] }) {
  const { t } = useTranslation();
  return (
    <div className="flex flex-wrap items-center gap-1.5 text-caption text-fg-muted">
      <span>{t("card.grant")}</span>
      {resources.map((r) => (
        <Code key={r}>{r}</Code>
      ))}
      <span className="text-fg-subtle">·</span>
      {actions.map((a) => (
        <Badge key={a}>{t(`action.${a}`, { defaultValue: a })}</Badge>
      ))}
    </div>
  );
}

function DraftView({ card, messageId }: { card: DraftCard; messageId: string }) {
  const { t } = useTranslation();
  const me = useMyId();
  const dir = useDirectory();
  const confirm = useConfirmDraft();
  const discard = useDiscardDraft();
  const [title, setTitle] = useState(card.draft.title);
  const [prompt, setPrompt] = useState(card.draft.prompt);
  const [resources, setResources] = useState(card.draft.grant.resources.join(", "));
  const [approvers, setApprovers] = useState<string[]>(card.draft.approvers.map((a) => a.id));
  const mine = card.requester_id === me;
  const editable = mine && card.state === "draft";
  const target = dir.agent(card.target_agent_id);
  const submit = () => {
    const d = {
      ...card.draft,
      title,
      prompt,
      grant: { ...card.draft.grant, resources: resources.split(",").map((s) => s.trim()).filter(Boolean) },
      approvers: approvers.map((id) => ({ id, name: dir.userName(id), role: "approver" })),
    };
    confirm.mutate({ messageId, draft: d });
  };
  return (
    <Card data-testid="draft-card" data-state={card.state} data-task-id={card.task_id}>
      <CardHeader>
        <Badge tone="accent">{t("card.draft.badge")}</Badge>
        <span className="text-body font-medium">{t("card.draft.to", { handle: card.target_handle, owner: dir.ownerName(target) })}</span>
        <div className="flex-1" />
        {card.state !== "draft" && <Badge tone={card.state === "submitted" ? "success" : "neutral"}>{t(`card.draft.${card.state}`)}</Badge>}
      </CardHeader>
      <CardBody className="flex flex-col gap-3">
        {editable ? (
          <>
            <div className="flex flex-col gap-1">
              <Label>{t("card.draft.title")}</Label>
              <Input value={title} onChange={(e) => setTitle(e.target.value)} data-testid="draft-title" />
            </div>
            <div className="flex flex-col gap-1">
              <Label>{t("card.draft.prompt")}</Label>
              <Textarea rows={3} value={prompt} onChange={(e) => setPrompt(e.target.value)} />
            </div>
            <div className="flex flex-col gap-1">
              <Label>{t("card.draft.resources")}</Label>
              <Input value={resources} onChange={(e) => setResources(e.target.value)} data-testid="draft-resources" className="font-mono" />
            </div>
            <div className="flex flex-col gap-1">
              <Label>{t("card.draft.approvers")}</Label>
              <div className="flex flex-wrap gap-1.5" data-testid="draft-approvers">
                {dir.members.map((m) => {
                  const on = approvers.includes(m.id);
                  return (
                    <button
                      key={m.id}
                      type="button"
                      data-approver={m.handle}
                      aria-pressed={on}
                      onClick={() => setApprovers(on ? approvers.filter((x) => x !== m.id) : [...approvers, m.id])}
                      className={
                        "cursor-pointer rounded-pill px-2.5 py-0.5 text-caption transition-colors " +
                        (on ? "bg-fg text-bg" : "bg-tag text-fg-muted hover:bg-active")
                      }
                    >
                      {m.name}
                    </button>
                  );
                })}
              </div>
            </div>
            <div className="text-caption text-fg-subtle">
              {t("card.draft.budget", { tokens: card.draft.budget.max_tokens.toLocaleString(), minutes: card.draft.budget.max_duration_min })}
            </div>
          </>
        ) : (
          <>
            <div className="text-body font-medium">{card.draft.title}</div>
            <GrantView resources={card.draft.grant.resources} actions={card.draft.grant.actions} />
            {card.task_id && <TaskLine taskId={card.task_id} fallback={card.draft.title} />}
          </>
        )}
        <ErrorText error={confirm.error ?? discard.error} />
      </CardBody>
      {editable && (
        <CardFooter>
          <span className="flex-1 text-caption text-fg-subtle">{t("card.draft.footer")}</span>
          <Button size="sm" variant="ghost" onClick={() => discard.mutate(messageId)} disabled={discard.isPending}>
            {t("card.draft.discard")}
          </Button>
          <Button size="sm" variant="primary" onClick={submit} disabled={confirm.isPending || approvers.length === 0} data-testid="draft-confirm">
            {t("card.draft.confirm")}
          </Button>
        </CardFooter>
      )}
    </Card>
  );
}

function IntakeView({ card }: { card: IntakeCard }) {
  const { t } = useTranslation();
  const me = useMyId();
  const dir = useDirectory();
  const decide = useDecideIntake();
  const task = useTask(card.task_id);
  const owner = card.owner_id === me;
  return (
    <Card data-testid="intake-card" data-state={card.state} data-task-id={card.task_id}>
      <CardHeader>
        <Badge tone={card.state === "pending" ? "warn" : card.state === "accepted" ? "success" : "danger"}>{t(`card.intake.${card.state}`)}</Badge>
        <span className="text-body">{t("card.intake.from", { requester: dir.userName(card.requester_id), handle: card.agent_handle })}</span>
      </CardHeader>
      <CardBody className="flex flex-col gap-2">
        <div className="text-body font-medium">{card.title}</div>
        <GrantView resources={card.grant.resources} actions={card.grant.actions} />
        {task && (
          <div className="text-caption text-fg-subtle">
            {t("card.intake.approvers", { names: task.approvers.map((a) => a.name).join("、") })}
          </div>
        )}
        {card.reason_code && <div className="text-caption text-danger">{t(`reason.${card.reason_code}`, { defaultValue: card.reason_code })}</div>}
        <ErrorText error={decide.error} />
      </CardBody>
      {card.state === "pending" && (
        <CardFooter>
          <span className="flex-1 text-caption text-fg-subtle">{owner ? t("card.intake.ownerHint") : t("card.intake.waiting", { owner: dir.userName(card.owner_id) })}</span>
          {owner && (
            <>
              <Button size="sm" variant="ghost" onClick={() => decide.mutate({ taskId: card.task_id, decision: "reject" })} data-testid="intake-reject">
                {t("card.intake.reject")}
              </Button>
              <Button size="sm" variant="primary" onClick={() => decide.mutate({ taskId: card.task_id, decision: "accept" })} disabled={decide.isPending} data-testid="intake-accept">
                {t("card.intake.accept")}
              </Button>
            </>
          )}
        </CardFooter>
      )}
    </Card>
  );
}

function PermissionView({ card }: { card: PermissionCard }) {
  const { t } = useTranslation();
  const me = useMyId();
  const dir = useDirectory();
  const decide = useDecidePermission();
  const eligible = card.eligible.includes(me);
  return (
    <Card data-testid="permission-card" data-state={card.state} data-task-id={card.task_id} data-permission-id={card.permission_id} className={card.state === "pending" ? "shadow-ring-warn" : ""}>
      <CardHeader>
        <Badge tone={card.state === "pending" ? "warn" : card.state === "approved" ? "success" : "danger"}>{t(`card.permission.${card.state}`)}</Badge>
        <span className="flex-1" />
        <span className="text-caption text-fg-muted">{t("card.permission.eligible", { names: card.eligible.map(dir.userName).join("、") })}</span>
      </CardHeader>
      <CardBody className="flex flex-col gap-2">
        <div className="text-title font-semibold">{t(`card.permission.title_${card.reason_code}`, { defaultValue: t("card.permission.title_scope_exceeded") })}</div>
        <div className="rounded-control bg-tag px-3 py-2 font-mono text-mono">
          {card.paths.map((p) => (
            <div key={p}>{p}</div>
          ))}
          {card.command && <div>$ {card.command}</div>}
          {!card.paths.length && !card.command && <div>{card.title}</div>}
        </div>
        <div className="text-caption text-fg-subtle">{t(`reason.${card.reason_code}`, { defaultValue: card.reason_code })}</div>
        {card.decided_by && <div className="text-caption text-fg-muted">{t("card.decidedBy", { name: dir.userName(card.decided_by) })}</div>}
        <ErrorText error={decide.error} />
      </CardBody>
      {card.state === "pending" && (
        <CardFooter>
          <span className="flex-1 text-caption text-fg-subtle">{eligible ? t("card.permission.hint") : t("card.permission.notEligible")}</span>
          {eligible && (
            <>
              <Button size="sm" variant="ghost" onClick={() => decide.mutate({ permissionId: card.permission_id, decision: "deny" })} data-testid="permission-deny">
                {t("card.permission.deny")}
              </Button>
              <Button size="sm" variant="primary" onClick={() => decide.mutate({ permissionId: card.permission_id, decision: "allow" })} disabled={decide.isPending} data-testid="permission-allow">
                {t("card.permission.allow")}
              </Button>
            </>
          )}
        </CardFooter>
      )}
    </Card>
  );
}

function MergeView({ card }: { card: MergeCard }) {
  const { t } = useTranslation();
  const me = useMyId();
  const dir = useDirectory();
  const decide = useDecideMerge();
  const eligible = card.eligible.includes(me);
  const open = card.state === "pending";
  const tone = card.state === "merged" ? "success" : card.state === "pending" || card.state === "merging" ? "accent" : "danger";
  return (
    <Card data-testid="merge-card" data-state={card.state} data-task-id={card.task_id} data-approval-id={card.approval_id}>
      <CardHeader>
        <Badge tone={tone}>{t(`card.merge.${card.state}`)}</Badge>
        <span className="text-body font-medium">{t("card.merge.pr", { number: card.number })}</span>
        <span className="flex-1" />
        <span className="text-caption text-fg-muted">{t("card.merge.approvers", { names: card.approvers.map((a) => a.name).join("、") })}</span>
      </CardHeader>
      <CardBody className="flex flex-col gap-2">
        <div className="flex flex-wrap items-center gap-1.5 text-caption text-fg-muted">
          <Code>{card.branch}</Code>
          <span>→</span>
          <Code>{card.base_branch}</Code>
          <span>{t("card.merge.commit")}</span>
          <Code data-testid="merge-commit">{card.commit.slice(0, 10)}</Code>
        </div>
        {card.diff_stat && <pre className="overflow-x-auto rounded-control bg-term-bg px-3 py-2 font-mono text-mono text-term-fg">{card.diff_stat}</pre>}
        {card.summary && <div className="whitespace-pre-wrap text-body text-fg-muted">{card.summary}</div>}
        {card.merge_commit && <div className="text-caption text-success">{t("card.merge.mergedAs", { commit: card.merge_commit.slice(0, 10) })}</div>}
        {card.error && <div className="text-caption text-danger">{card.error}</div>}
        {card.decided_by && <div className="text-caption text-fg-muted">{t("card.decidedBy", { name: dir.userName(card.decided_by) })}</div>}
        <ErrorText error={decide.error} />
      </CardBody>
      {open && (
        <CardFooter>
          <span className="flex-1 text-caption text-fg-subtle">{eligible ? t("card.merge.hint") : t("card.merge.locked")}</span>
          {eligible && (
            <>
              <Button size="sm" variant="ghost" onClick={() => decide.mutate({ approvalId: card.approval_id, decision: "reject" })} data-testid="merge-reject">
                {t("card.merge.reject")}
              </Button>
              <Button size="sm" variant="success" onClick={() => decide.mutate({ approvalId: card.approval_id, decision: "approve" })} disabled={decide.isPending} data-testid="merge-approve">
                {t("card.merge.approve")}
              </Button>
            </>
          )}
        </CardFooter>
      )}
    </Card>
  );
}

function MemoryView({ card }: { card: MemoryCard }) {
  const { t } = useTranslation();
  const me = useMyId();
  const dir = useDirectory();
  const decide = useDecideMemory();
  const lead = card.lead_id === me;
  return (
    <Card data-testid="memory-card" data-state={card.state} data-task-id={card.task_id} data-memory-id={card.memory_id}>
      <CardHeader>
        <Badge tone={card.state === "pending" ? "warn" : card.state === "confirmed" ? "success" : "danger"}>{t(`card.memory.${card.state}`)}</Badge>
        <span className="text-body font-medium">{t("card.memory.title", { version: card.version })}</span>
      </CardHeader>
      <CardBody>
        <pre className="whitespace-pre-wrap rounded-control bg-tag px-3 py-2 font-mono text-mono text-fg">{card.addition.trim()}</pre>
        <ErrorText error={decide.error} />
      </CardBody>
      {card.state === "pending" && (
        <CardFooter>
          <span className="flex-1 text-caption text-fg-subtle">{lead ? t("card.memory.hint") : t("card.memory.waiting", { lead: dir.userName(card.lead_id) })}</span>
          {lead && (
            <>
              <Button size="sm" variant="ghost" onClick={() => decide.mutate({ memoryId: card.memory_id, decision: "reject" })}>
                {t("card.memory.reject")}
              </Button>
              <Button size="sm" variant="primary" onClick={() => decide.mutate({ memoryId: card.memory_id, decision: "confirm" })} data-testid="memory-confirm">
                {t("card.memory.confirm")}
              </Button>
            </>
          )}
        </CardFooter>
      )}
    </Card>
  );
}

function NoticeView({ card, body }: { card: NoticeCard; body: string }) {
  const { t } = useTranslation();
  const danger = !["", "info"].includes(card.code);
  return (
    <div
      className={"mx-auto flex w-fit max-w-full items-center gap-1.5 rounded-pill px-3 py-1 text-caption " + (danger ? "bg-danger-soft text-danger" : "text-fg-subtle")}
      data-testid="notice"
      data-code={card.code}
      data-task-id={card.task_id}
    >
      <span className="font-medium">{t(`reason.${card.code}`, { defaultValue: card.code })}</span>
      <span className="truncate text-fg-muted">{body}</span>
    </div>
  );
}
