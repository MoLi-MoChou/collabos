import { useState } from "react";
import { useTranslation } from "react-i18next";
import {
  ApiError,
  useEditMemory,
  useProjectMembers,
  useProjectSettings,
  useSetBudget,
  useSetProjectMember,
  useStartPatrol,
  useTaintMemory,
  useUpdateAgentPolicy,
  useUsage,
  validReason,
  type Agent,
  type MemoryVersion,
  type Project,
} from "@collabos/core";
import { Badge, Button, Input, Label, Textarea } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId } from "../hooks";

function Err({ e }: { e: unknown }) {
  const { t } = useTranslation();
  if (!e) return null;
  return <div className="text-caption text-danger">{e instanceof ApiError ? e.message : t("common.error")}</div>;
}

/** Cost view (PRD 6.4): month-to-date usage by requester, owner and bot, plus budgets. */
export function UsageTab() {
  const { t } = useTranslation();
  const dir = useDirectory();
  const me = useMyId();
  const u = useUsage();
  const set = useSetBudget();
  const team = u.data?.budgets.find((b) => b.scope === "workspace");
  const mine = u.data?.budgets.find((b) => b.scope === "user" && b.subject_id === me);
  const [teamMax, setTeamMax] = useState("");
  const [myMax, setMyMax] = useState("");
  const isOwner = dir.members.find((m) => m.id === me)?.role === "owner";
  const rows = (title: string, list: { subject_id: string; tokens: number; tasks: number }[], name: (id: string) => string) => (
    <section className="flex flex-col gap-1">
      <div className="px-1 text-tiny font-medium text-fg-subtle">{title}</div>
      {list.length === 0 && <div className="px-1 text-caption text-fg-subtle">{t("usage.none")}</div>}
      {list.map((r) => (
        <div key={r.subject_id} className="flex items-center gap-2 px-1 text-caption" data-testid="usage-row">
          <span className="flex-1 truncate">{name(r.subject_id)}</span>
          <span className="text-fg-subtle">{t("usage.tasks", { n: r.tasks })}</span>
          <span className="w-20 text-right tabular-nums">{r.tokens.toLocaleString()}</span>
        </div>
      ))}
    </section>
  );
  const budgetRow = (label: string, b: typeof team, val: string, setVal: (v: string) => void, scope: "workspace" | "user", can: boolean) => (
    <div className="flex flex-col gap-1 rounded-card bg-surface p-2 shadow-card" data-testid={`budget-${scope}`}>
      <div className="flex items-center gap-2 text-caption">
        <span className="flex-1 font-medium">{label}</span>
        {b && <span className="tabular-nums text-fg-muted">{b.used.toLocaleString()} / {b.max_tokens.toLocaleString()}</span>}
      </div>
      {b && (
        <div className="h-1.5 w-full overflow-hidden rounded-pill bg-tag">
          <div className={"h-full " + (b.used >= b.max_tokens ? "bg-danger" : b.used * 10 >= b.max_tokens * 9 ? "bg-warn" : "bg-accent")} style={{ width: `${Math.min(100, (100 * b.used) / Math.max(1, b.max_tokens))}%` }} />
        </div>
      )}
      {can && (
        <div className="flex items-center gap-1.5">
          <Input type="number" className="h-7" value={val} placeholder={b ? String(b.max_tokens) : "1000000"} onChange={(e) => setVal(e.target.value)} />
          <Button size="sm" variant="secondary" disabled={!Number(val) || set.isPending} onClick={() => set.mutate({ scope, maxTokens: Number(val) })} data-testid={`budget-save-${scope}`}>
            {t("usage.save")}
          </Button>
        </div>
      )}
    </div>
  );
  return (
    <div className="flex flex-col gap-4 p-3" data-testid="usage-tab">
      <div className="flex items-baseline gap-2 px-1">
        <span className="text-body font-semibold">{t("usage.title")}</span>
        <span className="text-caption text-fg-subtle" data-testid="usage-total">{t("usage.total", { total: (u.data?.total ?? 0).toLocaleString() })}</span>
      </div>
      <section className="flex flex-col gap-2">
        <div className="px-1 text-tiny font-medium text-fg-subtle">{t("usage.budgets")}</div>
        {budgetRow(t("usage.team"), team, teamMax, setTeamMax, "workspace", isOwner)}
        {budgetRow(t("usage.mine"), mine, myMax, setMyMax, "user", true)}
        <Err e={set.error} />
      </section>
      {rows(t("usage.byRequester"), u.data?.by_requester ?? [], dir.userName)}
      {rows(t("usage.byOwner"), u.data?.by_owner ?? [], dir.userName)}
      {rows(t("usage.byAgent"), u.data?.by_agent ?? [], (id) => "@" + (dir.agent(id)?.handle ?? id.slice(0, 8)))}
    </div>
  );
}

/** Autonomy level and owner constraints for a bot (PRD 6.3 / 7.2), plus patrol. */
export function AgentPolicyEditor({ a, roomId }: { a: Agent; roomId: string }) {
  const { t } = useTranslation();
  const up = useUpdateAgentPolicy();
  const patrol = useStartPatrol();
  const [autonomy, setAutonomy] = useState(a.autonomy);
  const [conc, setConc] = useState(String(a.max_concurrent || 2));
  const [dur, setDur] = useState(String(a.max_duration_min || 0));
  const [net, setNet] = useState(a.allow_network);
  const [intake, setIntake] = useState(String(a.intake_timeout_min || 1440));
  const [prompt, setPrompt] = useState("");
  return (
    <div className="mt-2 flex flex-col gap-2 border-t border-border pt-2" data-testid="policy-editor">
      <Label>{t("policy.autonomy")}</Label>
      <div className="flex flex-wrap gap-1">
        {(["supervised", "standard", "autonomous"] as const).map((m) => (
          <Button key={m} size="sm" variant={autonomy === m ? "primary" : "secondary"} onClick={() => setAutonomy(m)} data-testid={`autonomy-${m}`}>
            {t(`policy.${m}`)}
          </Button>
        ))}
      </div>
      <div className="grid grid-cols-2 items-center gap-1.5 text-caption">
        <span>{t("policy.maxConcurrent")}</span>
        <Input className="h-7" value={conc} onChange={(e) => setConc(e.target.value)} />
        <span>{t("policy.maxDuration")}</span>
        <Input className="h-7" value={dur} onChange={(e) => setDur(e.target.value)} />
        <span>{t("policy.intakeTimeout")}</span>
        <Input className="h-7" value={intake} onChange={(e) => setIntake(e.target.value)} />
        <label className="col-span-2 flex items-center gap-2">
          <input type="checkbox" checked={net} onChange={(e) => setNet(e.target.checked)} />
          {t("policy.network")}
        </label>
      </div>
      <div className="flex justify-end">
        <Button
          size="sm"
          variant="primary"
          disabled={up.isPending}
          data-testid="policy-save"
          onClick={() =>
            up.mutate({
              agentId: a.id,
              autonomy,
              max_concurrent: Number(conc) || 0,
              max_duration_min: Number(dur) || 0,
              allowed_actions: a.allowed_actions,
              allow_network: net,
              intake_timeout_min: Number(intake) || 0,
              daily_cost_units: a.daily_cost_units,
            })
          }
        >
          {t("policy.save")}
        </Button>
      </div>
      <Err e={up.error} />
      {roomId && (
        <div className="flex items-center gap-1.5">
          <Input className="h-7" placeholder={t("policy.patrolPrompt")} value={prompt} onChange={(e) => setPrompt(e.target.value)} data-testid="patrol-prompt" />
          <Button size="sm" variant="secondary" disabled={!prompt.trim() || patrol.isPending} onClick={() => patrol.mutate({ agentId: a.id, roomId, prompt }, { onSuccess: () => setPrompt("") })} data-testid="patrol-start">
            {t("policy.patrol")}
          </Button>
        </div>
      )}
      <Err e={patrol.error} />
    </div>
  );
}

/** Shared-project access (PRD 6.5): visibility, roles, test command and webhook secret. */
export function ProjectAccessPanel({ project }: { project: Project }) {
  const { t } = useTranslation();
  const dir = useDirectory();
  const members = useProjectMembers(project.id);
  const setMember = useSetProjectMember();
  const settings = useProjectSettings();
  const [testCmd, setTestCmd] = useState(project.test_command);
  const [secret, setSecret] = useState("");
  const lead = project.my_role === "lead";
  const listed = new Set((members.data ?? []).map((m) => m.user_id));
  return (
    <section className="flex flex-col gap-2 rounded-card bg-surface p-3 shadow-card" data-testid="project-access">
      <div className="flex items-center gap-2">
        <span className="flex-1 text-caption font-medium">{t("project.members")}</span>
        <Badge tone={project.visibility === "restricted" ? "warn" : "neutral"} data-testid="project-visibility">{t(`project.${project.visibility}`)}</Badge>
        {lead && (
          <Button size="sm" variant="ghost" onClick={() => settings.mutate({ projectId: project.id, visibility: project.visibility === "restricted" ? "workspace" : "restricted" })} data-testid="visibility-toggle">
            {t(`project.${project.visibility === "restricted" ? "workspace" : "restricted"}`)}
          </Button>
        )}
      </div>
      {(members.data ?? []).map((m) => (
        <div key={m.user_id} className="flex items-center gap-2 text-caption" data-testid="project-member">
          <span className="flex-1">{m.name}</span>
          {lead && m.role !== "lead" ? (
            <>
              <select className="h-7 rounded-control bg-tag px-1" value={m.role} onChange={(e) => setMember.mutate({ projectId: project.id, userId: m.user_id, role: e.target.value === "viewer" ? "viewer" : "writer" })}>
                <option value="writer">{t("project.writer")}</option>
                <option value="viewer">{t("project.viewer")}</option>
              </select>
              <Button size="sm" variant="ghost" onClick={() => setMember.mutate({ projectId: project.id, userId: m.user_id, role: null })}>
                {t("project.remove")}
              </Button>
            </>
          ) : (
            <Badge>{t(`project.${m.role}`)}</Badge>
          )}
        </div>
      ))}
      {lead &&
        dir.members
          .filter((m) => !listed.has(m.id))
          .map((m) => (
            <div key={m.id} className="flex items-center gap-2 text-caption text-fg-muted">
              <span className="flex-1">{m.name}</span>
              <Button size="sm" variant="ghost" onClick={() => setMember.mutate({ projectId: project.id, userId: m.id, role: "writer" })} data-testid="project-member-add">
                {t("project.add")}
              </Button>
            </div>
          ))}
      {lead && (
        <div className="grid grid-cols-3 items-center gap-1.5 text-caption">
          <span>{t("project.testCommand")}</span>
          <Input className="h-7 font-mono" value={testCmd} onChange={(e) => setTestCmd(e.target.value)} data-testid="project-test-command" />
          <Button size="sm" variant="secondary" onClick={() => settings.mutate({ projectId: project.id, test_command: testCmd })}>
            {t("project.save")}
          </Button>
          <span>{t("project.webhookSecret")}</span>
          <Input className="h-7" type="password" value={secret} onChange={(e) => setSecret(e.target.value)} />
          <Button size="sm" variant="secondary" disabled={!secret} onClick={() => settings.mutate({ projectId: project.id, webhook_secret: secret }, { onSuccess: () => setSecret("") })}>
            {t("project.save")}
          </Button>
          {project.webhook_path && (
            <span className="col-span-3 break-all text-fg-subtle" data-testid="webhook-path">
              {t("project.webhook")}: <code>{project.webhook_path}</code> {project.webhook_configured ? "✓" : ""}
            </span>
          )}
        </div>
      )}
      <Err e={setMember.error ?? settings.error} />
    </section>
  );
}

/** Lead edits MEMORY.md directly; others propose (pending). */
export function MemoryEditor({ projectId, current }: { projectId: string; current: string }) {
  const { t } = useTranslation();
  const edit = useEditMemory();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState(current);
  if (!open)
    return (
      <Button size="sm" variant="ghost" onClick={() => (setText(current), setOpen(true))} data-testid="memory-edit">
        {t("memory2.edit")}
      </Button>
    );
  return (
    <div className="flex flex-col gap-1.5">
      <Textarea rows={8} className="font-mono" value={text} onChange={(e) => setText(e.target.value)} data-testid="memory-editor" />
      <div className="flex justify-end gap-1.5">
        <Button size="sm" variant="ghost" onClick={() => setOpen(false)}>
          {t("common.cancel")}
        </Button>
        <Button size="sm" variant="primary" disabled={edit.isPending || text === current} onClick={() => edit.mutate({ projectId, content: text }, { onSuccess: () => setOpen(false) })} data-testid="memory-save">
          {t("memory2.save")}
        </Button>
      </div>
      <Err e={edit.error} />
    </div>
  );
}

/** Taint tracing (PRD 11.4): mark a version polluted; later versions and referencing tasks are flagged. */
export function TaintButton({ v }: { v: MemoryVersion }) {
  const { t } = useTranslation();
  const taint = useTaintMemory();
  const [open, setOpen] = useState(false);
  const [reason, setReason] = useState("");
  if (v.tainted) return <Badge tone="danger" title={v.taint_reason} data-testid="memory-tainted">{t("memory2.tainted")}</Badge>;
  if (!open)
    return (
      <button className="cursor-pointer text-caption text-danger" onClick={() => setOpen(true)} data-testid="memory-taint">
        {t("memory2.taint")}
      </button>
    );
  return (
    <form
      className="flex items-center gap-1"
      onSubmit={(e) => {
        e.preventDefault();
        if (validReason(reason)) taint.mutate({ memoryId: v.id, reason });
      }}
    >
      <Input className="h-7 w-36" autoFocus placeholder={t("memory2.taintReason")} value={reason} onChange={(e) => setReason(e.target.value)} data-testid="taint-reason" />
      <Button size="sm" variant="danger" type="submit" disabled={!validReason(reason) || taint.isPending} data-testid="taint-confirm">
        {t("memory2.taint")}
      </Button>
    </form>
  );
}
