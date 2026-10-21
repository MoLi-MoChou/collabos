import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Renderer } from "@openuidev/react-lang";
import { useRichAction, type Message } from "@collabos/core";
import { Badge } from "@collabos/ui";
import { RichActionContext, richLibrary } from "./library";
import { useDirectory } from "../directory";

/** Renders a validated OpenUI message with its provenance (bot + owner, untrusted). */
export function RichMessage({ message }: { message: Message }) {
  const { t } = useTranslation();
  const dir = useDirectory();
  const act = useRichAction();
  const [done, setDone] = useState<string | null>(null);
  const rich = message.rich;
  if (!rich || rich.lang !== "openui") return null;
  const agent = dir.agent(rich.agent_id);
  return (
    <div className="flex flex-col gap-2 rounded-card bg-surface px-3 py-2 shadow-card" data-testid="rich-message">
      <div className="flex items-center gap-1.5 text-caption text-fg-subtle">
        <Badge tone="warn">{t("rich.untrusted")}</Badge>
        <span>{t("rich.source", { handle: agent?.handle ?? "bot", owner: dir.userName(rich.owner_id) })}</span>
      </div>
      <RichActionContext.Provider
        value={{
          disabled: act.isPending || done !== null,
          onAction: (actionId, value) => act.mutate({ messageId: message.id, actionId, value }, { onSuccess: () => setDone(actionId) }),
        }}
      >
        <Renderer response={rich.source} library={richLibrary} isStreaming={false} />
      </RichActionContext.Provider>
      {done && <div className="text-caption text-fg-subtle" data-testid="rich-done">{t("rich.sent")}</div>}
      {act.error && <div className="text-caption text-danger">{t("common.error")}</div>}
    </div>
  );
}
