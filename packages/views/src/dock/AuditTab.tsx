import { useTranslation } from "react-i18next";
import { useAudit } from "@collabos/core";
import { useDirectory } from "../directory";

/** Append-only audit trail of every decision in the workspace. */
export function AuditTab() {
  const { t } = useTranslation();
  const audit = useAudit();
  const dir = useDirectory();
  const actor = (type: string, id: string) =>
    type === "user" ? dir.userName(id) : type === "agent" ? `@${dir.agent(id)?.handle ?? id.slice(0, 8)}` : t(`audit.actor.${type}`, { defaultValue: type });
  return (
    <div className="flex flex-col gap-1 p-3" data-testid="audit-tab">
      <div className="mb-1 px-1 text-caption text-fg-subtle">{t("audit.hint")}</div>
      {(audit.data ?? []).map((e) => (
        <div key={e.id} className="flex items-baseline gap-2 rounded-control px-2 py-1.5 text-caption hover:bg-hover" data-testid="audit-row" data-action={e.action}>
          <span className="shrink-0 text-fg-subtle">{new Date(e.created_at).toLocaleTimeString()}</span>
          <span className="shrink-0 font-medium">{actor(e.actor_type, e.actor_id)}</span>
          <span className="font-mono text-fg-muted">{e.action}</span>
        </div>
      ))}
    </div>
  );
}
