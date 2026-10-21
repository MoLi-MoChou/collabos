import { useTranslation } from "react-i18next";
import { useDecideMemory, useMemory, useProjects, useRooms } from "@collabos/core";
import { Badge, Button } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId } from "../hooks";

export function MemoryTab({ roomId }: { roomId: string }) {
  const { t } = useTranslation();
  const rooms = useRooms();
  const projects = useProjects();
  const room = rooms.data?.find((r) => r.id === roomId);
  const project = projects.data?.find((p) => p.id === room?.project_id);
  const mem = useMemory(project?.id ?? "");
  const decide = useDecideMemory();
  const me = useMyId();
  const dir = useDirectory();
  const versions = mem.data ?? [];
  const current = versions.find((v) => v.status === "confirmed");
  const pending = versions.filter((v) => v.status === "pending");
  const lead = project?.lead_id === me;
  return (
    <div className="flex flex-col gap-3 p-3" data-testid="memory-tab">
      <div className="flex items-center gap-2 px-1 text-caption text-fg-subtle">
        <span className="flex-1">{t("memory.lead", { name: dir.userName(project?.lead_id ?? "") })}</span>
        {current && <Badge tone="success" data-testid="memory-version">{t("memory.version", { v: current.version })}</Badge>}
      </div>
      {pending.map((v) => (
        <div key={v.id} className="rounded-card bg-surface shadow-ring-warn p-3" data-testid="memory-pending">
          <div className="mb-2 flex items-center gap-2">
            <Badge tone="warn">{t("memory.pending", { v: v.version })}</Badge>
            <span className="flex-1" />
            {lead && (
              <>
                <Button size="sm" variant="ghost" onClick={() => decide.mutate({ memoryId: v.id, decision: "reject" })}>
                  {t("card.memory.reject")}
                </Button>
                <Button size="sm" variant="primary" onClick={() => decide.mutate({ memoryId: v.id, decision: "confirm" })}>
                  {t("card.memory.confirm")}
                </Button>
              </>
            )}
          </div>
          <pre className="max-h-48 overflow-y-auto whitespace-pre-wrap font-mono text-mono">{v.content}</pre>
        </div>
      ))}
      <pre className="whitespace-pre-wrap rounded-card bg-surface shadow-card p-3 font-mono text-mono" data-testid="memory-content">
        {current?.content ?? ""}
      </pre>
      <div className="px-1 text-caption text-fg-subtle">{t("memory.history")}</div>
      {versions.map((v) => (
        <div key={v.id} className="flex items-center gap-2 px-1 text-caption">
          <span className="font-mono">{`v${v.version}`}</span>
          <Badge tone={v.status === "confirmed" ? "success" : v.status === "pending" ? "warn" : "danger"}>{t(`card.memory.${v.status}`)}</Badge>
          <span className="text-fg-subtle">{v.proposed_by.startsWith("agent:") ? t("memory.byAgent") : t("memory.bySystem")}</span>
        </div>
      ))}
    </div>
  );
}
