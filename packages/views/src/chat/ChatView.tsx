import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { completeMention, mentionQuery, parseCard, segments, useMessages, useRooms, useSendMessage, type Message } from "@collabos/core";
import { Avatar, Badge, Button, Textarea, cn } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useUi } from "../hooks";
import { CardView } from "../cards/CardView";

export function ChatView({ roomId }: { roomId: string }) {
  const { t } = useTranslation();
  const rooms = useRooms();
  const msgs = useMessages(roomId);
  const ui = useUi();
  const room = rooms.data?.find((r) => r.id === roomId);
  const end = useRef<HTMLDivElement>(null);
  const count = msgs.data?.length ?? 0;
  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [count]);
  return (
    <div className="flex h-full flex-col">
      <header className="flex h-12 items-center gap-2 border-b border-border px-5">
        <span className="text-fg-subtle">#</span>
        <span className="text-title font-semibold" data-testid="room-title">{room?.name}</span>
        <div className="flex-1" />
        <Button size="sm" variant="ghost" onClick={() => ui.actions.toggleDock()} data-testid="dock-toggle">
          {ui.dockOpen ? t("chat.hideDock") : t("chat.showDock")}
        </Button>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="messages">
        <div className="mx-auto flex max-w-3xl flex-col gap-4 px-5 py-5">
          {count === 0 && !msgs.isLoading && <div className="py-10 text-center text-body text-fg-subtle">{t("chat.empty")}</div>}
          {(msgs.data ?? []).map((m) => (
            <MessageRow key={m.id} m={m} />
          ))}
          <div ref={end} />
        </div>
      </div>
      <Composer roomId={roomId} />
    </div>
  );
}

function MessageRow({ m }: { m: Message }) {
  const { t } = useTranslation();
  const dir = useDirectory();
  const card = parseCard(m.card);
  const agent = m.sender_type === "agent" ? dir.agent(m.sender_id) : undefined;
  const name = m.sender_type === "user" ? dir.userName(m.sender_id) : m.sender_type === "agent" ? (agent?.name ?? "agent") : "CollabOS";
  const time = m.created_at ? new Date(m.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "";
  if (m.sender_type === "system" && card) {
    return (
      <div className="pl-11" data-testid={`card-${card.type}`} data-message-id={m.id}>
        <CardView card={card} messageId={m.id} body={m.body} />
      </div>
    );
  }
  return (
    <div className="flex gap-3" data-testid="message" data-sender={m.sender_type}>
      <Avatar name={name} kind={m.sender_type} />
      <div className="min-w-0 flex-1">
        <div className="flex items-baseline gap-2">
          <span className="text-body font-semibold">{name}</span>
          {agent && <Badge tone="accent">{t("chat.botOf", { owner: dir.ownerName(agent) })}</Badge>}
          {m.trust === "untrusted" && m.sender_type === "agent" && <Badge>{t("chat.untrusted")}</Badge>}
          <span className="text-caption text-fg-subtle">{time}</span>
        </div>
        <div className="mt-0.5 whitespace-pre-wrap break-words text-body-lg">
          {segments(m.body).map((s, i) =>
            s.mention ? (
              <span key={i} className="rounded-sm bg-accent-soft px-1 text-accent">
                {s.text}
              </span>
            ) : (
              <span key={i}>{s.text}</span>
            ),
          )}
        </div>
        {card && <div className="mt-2"><CardView card={card} messageId={m.id} body={m.body} /></div>}
      </div>
    </div>
  );
}

function Composer({ roomId }: { roomId: string }) {
  const { t } = useTranslation();
  const send = useSendMessage(roomId);
  const dir = useDirectory();
  const rooms = useRooms();
  const room = rooms.data?.find((r) => r.id === roomId);
  const [text, setText] = useState("");
  const [caret, setCaret] = useState(0);
  const [pending, setPending] = useState<{ body: string; id: string } | null>(null);
  const ref = useRef<HTMLTextAreaElement>(null);
  const q = mentionQuery(text, caret);
  const options = useMemo(() => {
    if (q === null) return [];
    const all = [
      ...dir.agents.map((a) => ({ handle: a.handle, label: a.name, sub: dir.ownerName(a), agent: true })),
      ...dir.members.map((m) => ({ handle: m.handle, label: m.name, sub: "", agent: false })),
    ];
    return all.filter((o) => o.handle.startsWith(q)).slice(0, 6);
  }, [q, dir]);

  const submit = (body: string, id: string) => {
    setPending({ body, id });
    send.mutate(
      { body, clientMsgId: id },
      {
        onSuccess: () => setPending(null),
      },
    );
  };
  const onSend = () => {
    const body = text.trim();
    if (!body) return;
    setText("");
    submit(body, crypto.randomUUID());
  };
  const pick = (handle: string) => {
    const r = completeMention(text, caret, handle);
    setText(r.text);
    setCaret(r.caret);
    requestAnimationFrame(() => {
      ref.current?.focus();
      ref.current?.setSelectionRange(r.caret, r.caret);
    });
  };

  return (
    <div className="mx-auto w-full max-w-3xl px-5 pb-5">
      {pending && (
        <div className="mb-2 flex items-center gap-2 text-caption text-fg-muted" data-testid="pending-send">
          <span className="truncate">{pending.body}</span>
          {send.isError ? (
            <Button size="sm" variant="ghost" onClick={() => submit(pending.body, pending.id)}>
              {t("chat.retry")}
            </Button>
          ) : (
            <span>{t("chat.sending")}</span>
          )}
        </div>
      )}
      <div className="relative rounded-panel border border-border bg-surface p-3 shadow-card">
        {options.length > 0 && (
          <div className="absolute bottom-full left-3 mb-2 w-72 overflow-hidden rounded-card border border-border bg-surface shadow-popover" data-testid="mention-menu">
            {options.map((o) => (
              <button
                key={o.handle}
                className="flex w-full cursor-pointer items-center gap-2 px-3 py-2 text-left text-body hover:bg-surface-2"
                onMouseDown={(e) => {
                  e.preventDefault();
                  pick(o.handle);
                }}
              >
                <Avatar size="sm" name={o.label} kind={o.agent ? "agent" : "user"} />
                <span className="font-medium">@{o.handle}</span>
                {o.sub && <span className="truncate text-caption text-fg-subtle">{t("chat.botOf", { owner: o.sub })}</span>}
              </button>
            ))}
          </div>
        )}
        <Textarea
          ref={ref}
          rows={2}
          data-testid="composer"
          className={cn("border-0 px-1 focus:border-0")}
          placeholder={t("chat.placeholder", { room: room?.name ?? "" })}
          value={text}
          onChange={(e) => {
            setText(e.target.value);
            setCaret(e.target.selectionStart);
          }}
          onSelect={(e) => setCaret(e.currentTarget.selectionStart)}
          onKeyDown={(e) => {
            if (e.key === "Enter" && !e.shiftKey && !e.nativeEvent.isComposing) {
              e.preventDefault();
              if (options.length > 0 && options[0]) pick(options[0].handle);
              else onSend();
            }
          }}
        />
        <div className="flex items-center justify-between pt-1">
          <span className="text-caption text-fg-subtle">{t("chat.hint")}</span>
          <Button size="sm" variant="primary" onClick={onSend} disabled={!text.trim()} data-testid="send">
            {t("chat.send")}
          </Button>
        </div>
      </div>
    </div>
  );
}
