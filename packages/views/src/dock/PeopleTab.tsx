import { useState } from "react";
import { useTranslation } from "react-i18next";
import { ApiError, useDaemons, usePair, useUpdateIntake, type Agent, type PairResult } from "@collabos/core";
import { Avatar, Badge, Button, Code, Dialog, DialogContent, DialogDescription, DialogTitle, DialogTrigger, Dot, Input, Label } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId } from "../hooks";
import { useApi } from "@collabos/core";

export function PeopleTab() {
  const { t } = useTranslation();
  const dir = useDirectory();
  const daemons = useDaemons();
  const me = useMyId();
  return (
    <div className="flex flex-col gap-4 p-3">
      <section>
        <div className="mb-2 px-1 text-caption font-medium text-fg-subtle">{t("dock.members")}</div>
        {dir.members.map((m) => (
          <div key={m.id} className="flex items-center gap-2 px-1 py-1.5" data-testid="member">
            <Avatar size="sm" name={m.name} />
            <span className="text-body">{m.name}</span>
            <span className="text-caption text-fg-subtle">@{m.handle}</span>
            {m.role === "owner" && <Badge>{t("dock.owner")}</Badge>}
          </div>
        ))}
      </section>
      <section>
        <div className="mb-2 px-1 text-caption font-medium text-fg-subtle">{t("dock.agents")}</div>
        <div className="flex flex-col gap-1.5">
          {dir.agents.map((a) => (
            <AgentRow key={a.id} a={a} mine={a.owner_id === me} />
          ))}
        </div>
      </section>
      <section>
        <div className="mb-2 flex items-center px-1">
          <span className="flex-1 text-caption font-medium text-fg-subtle">{t("dock.devices")}</span>
          <PairDialog />
        </div>
        {(daemons.data ?? []).map((d) => (
          <div key={d.id} className="mb-1.5 rounded-card border border-border bg-surface px-3 py-2" data-testid="device" data-status={d.status}>
            <div className="flex items-center gap-2">
              <Dot tone={d.status === "online" ? "success" : "neutral"} />
              <span className="text-body font-medium">{d.name}</span>
              <span className="text-caption text-fg-subtle">{dir.userName(d.owner_id)}</span>
            </div>
            {d.capabilities.os && (
              <div className="mt-1 flex flex-wrap gap-1">
                <Badge>{d.capabilities.os}/{d.capabilities.arch}</Badge>
                <Badge tone={d.capabilities.sandbox === "none" ? "danger" : "success"}>{t("dock.sandbox", { name: d.capabilities.sandbox })}</Badge>
                {(d.capabilities.agents ?? []).map((x) => (
                  <Badge key={x} tone="accent">{x}</Badge>
                ))}
              </div>
            )}
          </div>
        ))}
      </section>
    </div>
  );
}

function AgentRow({ a, mine }: { a: Agent; mine: boolean }) {
  const { t } = useTranslation();
  const dir = useDirectory();
  const update = useUpdateIntake();
  const [mode, setMode] = useState(a.intake_mode);
  const [quota, setQuota] = useState(String(a.daily_quota));
  const [allow, setAllow] = useState<string[]>(a.intake_allowlist);
  const [edit, setEdit] = useState(false);
  return (
    <div className="rounded-card border border-border bg-surface px-3 py-2" data-testid="agent" data-handle={a.handle}>
      <div className="flex items-center gap-2">
        <Avatar size="sm" name={a.name} kind="agent" />
        <span className="text-body font-medium">@{a.handle}</span>
        <Badge>{t(`agent.${a.kind}`)}</Badge>
        <Dot tone={a.online ? "success" : "neutral"} />
        <span className="flex-1" />
        <span className="text-caption text-fg-subtle">{dir.ownerName(a)}</span>
      </div>
      {a.kind === "specialist" && (
        <div className="mt-1 flex items-center gap-2 text-caption text-fg-muted">
          <span>{t(`intake.${a.intake_mode}`)}</span>
          <span>·</span>
          <span>{t("dock.quota", { n: a.daily_quota })}</span>
          <span className="flex-1" />
          {mine && (
            <Button size="sm" variant="ghost" onClick={() => setEdit(!edit)} data-testid="intake-edit">
              {t("dock.editIntake")}
            </Button>
          )}
        </div>
      )}
      {edit && (
        <div className="mt-2 flex flex-col gap-2 border-t border-border pt-2">
          <div className="flex gap-1">
            {(["manual", "allowlist", "closed"] as const).map((m) => (
              <Button key={m} size="sm" variant={mode === m ? "primary" : "secondary"} onClick={() => setMode(m)} data-testid={`intake-mode-${m}`}>
                {t(`intake.${m}`)}
              </Button>
            ))}
          </div>
          {mode === "allowlist" && (
            <div className="flex flex-wrap gap-1">
              {dir.members
                .filter((m) => m.id !== a.owner_id)
                .map((m) => {
                  const on = allow.includes(m.id);
                  return (
                    <button
                      key={m.id}
                      onClick={() => setAllow(on ? allow.filter((x) => x !== m.id) : [...allow, m.id])}
                      className={"cursor-pointer rounded-pill border px-2 py-0.5 text-caption " + (on ? "border-accent bg-accent-soft text-accent" : "border-border text-fg-muted")}
                    >
                      {m.name}
                    </button>
                  );
                })}
            </div>
          )}
          <div className="flex items-center gap-2">
            <Label>{t("dock.quotaLabel")}</Label>
            <Input className="h-7 w-20" value={quota} onChange={(e) => setQuota(e.target.value)} data-testid="intake-quota" />
            <span className="flex-1" />
            <Button
              size="sm"
              variant="primary"
              data-testid="intake-save"
              onClick={() =>
                update.mutate(
                  { agentId: a.id, mode, allowlist: allow, daily_quota: Number(quota) || 0 },
                  { onSuccess: () => setEdit(false) },
                )
              }
            >
              {t("common.save")}
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function PairDialog() {
  const { t } = useTranslation();
  const pair = usePair();
  const api = useApi();
  const [name, setName] = useState("");
  const [chief, setChief] = useState("");
  const [coder, setCoder] = useState("");
  const [res, setRes] = useState<PairResult | null>(null);
  const server = api.baseUrl || window.location.origin;
  return (
    <Dialog onOpenChange={(o) => !o && setRes(null)}>
      <DialogTrigger asChild>
        <Button size="sm" variant="ghost" data-testid="pair-open">
          {t("pair.open")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogTitle>{t("pair.title")}</DialogTitle>
        <DialogDescription>{t("pair.desc")}</DialogDescription>
        {res ? (
          <div className="mt-4 flex flex-col gap-2" data-testid="pair-result">
            <div className="text-body">{t("pair.run")}</div>
            <pre className="overflow-x-auto whitespace-pre-wrap break-all rounded-control bg-term-bg p-3 font-mono text-mono text-term-fg" data-testid="pair-command">
              {`collab login --server ${server} --token ${res.node_token}\ncollab repo add /path/to/checkout\ncollab run`}
            </pre>
            <div className="text-caption text-warn">{t("pair.once")}</div>
          </div>
        ) : (
          <form
            className="mt-4 flex flex-col gap-3"
            onSubmit={(e) => {
              e.preventDefault();
              const agents = [
                ...(chief ? [{ handle: chief, kind: "chief" }] : []),
                ...(coder ? [{ handle: coder, kind: "specialist" }] : []),
              ];
              pair.mutate({ name, agents }, { onSuccess: setRes });
            }}
          >
            <div className="flex flex-col gap-1">
              <Label>{t("pair.name")}</Label>
              <Input required value={name} onChange={(e) => setName(e.target.value)} data-testid="pair-name" />
            </div>
            <div className="flex gap-2">
              <div className="flex flex-1 flex-col gap-1">
                <Label>{t("pair.chief")}</Label>
                <Input value={chief} onChange={(e) => setChief(e.target.value)} data-testid="pair-chief" />
              </div>
              <div className="flex flex-1 flex-col gap-1">
                <Label>{t("pair.coder")}</Label>
                <Input value={coder} onChange={(e) => setCoder(e.target.value)} data-testid="pair-coder" />
              </div>
            </div>
            {pair.error && <div className="text-caption text-danger">{pair.error instanceof ApiError ? pair.error.message : t("common.error")}</div>}
            <Button variant="primary" type="submit" disabled={pair.isPending} className="justify-center" data-testid="pair-submit">
              {t("pair.submit")}
            </Button>
          </form>
        )}
        <div className="mt-3 text-caption text-fg-subtle">
          {t("pair.note")} <Code>{"COLLAB_HOME"}</Code>
        </div>
      </DialogContent>
    </Dialog>
  );
}
