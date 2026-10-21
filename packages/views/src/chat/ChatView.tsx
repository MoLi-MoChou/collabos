import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { completeMention, mentionQuery, parseCard, segments, useMessages, useRooms, useSendMessage, type Message } from "@collabos/core";
import { Avatar, Badge, Button, Textarea, cn } from "@collabos/ui";
import { useDirectory } from "../directory";
import { useMyId, useUi } from "../hooks";
import { IconHash, IconPanel, IconPlus, IconSend } from "../icons";
import { CardView } from "../cards/CardView";
import { RichMessage } from "../rich/RichMessage";
import { Fenced } from "./Fenced";

const WINDOW = 150;
const ROW_STYLE = { contentVisibility: "auto", containIntrinsicSize: "auto 80px" } as const;

export function ChatView({ roomId }: { roomId: string }) {
  const { t } = useTranslation();
  const rooms = useRooms();
  const msgs = useMessages(roomId);
  const ui = useUi();
  const room = rooms.data?.find((r) => r.id === roomId);
  const end = useRef<HTMLDivElement>(null);
  const count = msgs.data?.length ?? 0;
  // FR-CLI-101: only the newest `windowSize` rows are mounted; older ones load on demand,
  // and off-screen rows skip layout/paint via content-visibility.
  const [windowSize, setWindowSize] = useState(WINDOW);
  const visible = (msgs.data ?? []).slice(-windowSize);
  useEffect(() => {
    end.current?.scrollIntoView({ block: "end" });
  }, [count]);
  return (
    <div className="flex h-full flex-col">
      <header className="relative flex h-12 shrink-0 items-center justify-center px-4">
        <div className="flex items-center gap-1.5 rounded-pill bg-surface px-3.5 py-1.5 shadow-card">
          <IconHash className="text-fg-subtle" width={14} height={14} />
          <span className="text-body font-semibold" data-testid="room-title">{room?.name}</span>
        </div>
        <Button
          size="icon"
          variant={ui.dockOpen ? "secondary" : "ghost"}
          className="absolute right-4"
          onClick={() => ui.actions.toggleDock()}
          data-testid="dock-toggle"
          aria-label={ui.dockOpen ? t("chat.hideDock") : t("chat.showDock")}
          title={ui.dockOpen ? t("chat.hideDock") : t("chat.showDock")}
        >
          <IconPanel />
        </Button>
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto" data-testid="messages">
        <div className="mx-auto flex max-w-215 flex-col gap-3 px-6 pt-2 pb-6">
          {count === 0 && !msgs.isLoading && <div className="py-10 text-center text-body text-fg-subtle">{t("chat.empty")}</div>}
          {count > windowSize && (
            <button className="mx-auto cursor-pointer rounded-pill px-3 py-1 text-caption text-accent hover:bg-hover" onClick={() => setWindowSize(windowSize + WINDOW)} data-testid="load-earlier">
              {t("chat.earlier", { n: count - windowSize })}
            </button>
          )}
          {visible.map((m) => (
            <div key={m.id} style={ROW_STYLE}>
              <MessageRow m={m} />
            </div>
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
  const myId = useMyId();
  const card = parseCard(m.card);
  const agent = m.sender_type === "agent" ? dir.agent(m.sender_id) : undefined;
  const name = m.sender_type === "user" ? dir.userName(m.sender_id) : m.sender_type === "agent" ? (agent?.name ?? "agent") : "CollabOS";
  const time = m.created_at ? new Date(m.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }) : "";
  if (m.sender_type === "system" && card) {
    return (
      <div className="cos-rise pl-9" data-testid={`card-${card.type}`} data-message-id={m.id}>
        <div className="max-w-160">
          <CardView card={card} messageId={m.id} body={m.body} />
        </div>
      </div>
    );
  }
  const mine = m.sender_type === "user" && m.sender_id === myId;
  const text = segments(m.body).map((s, i) =>
    s.mention ? (
      <span key={i} className={mine ? "font-medium underline decoration-1 underline-offset-2" : "font-medium text-accent"}>
        {s.text}
      </span>
    ) : (
      <span key={i}>{s.text}</span>
    ),
  );
  if (mine) {
    return (
      <div className="cos-rise flex flex-col items-end" data-testid="message" data-sender={m.sender_type} data-mine="true">
        <div className="ml-24 whitespace-pre-wrap break-words rounded-bubble bg-bubble-me px-3.5 py-2 text-body-lg text-bubble-me-fg" title={time}>
          {text}
        </div>
        {card && <div className="mt-2 w-full max-w-160"><CardView card={card} messageId={m.id} body={m.body} /></div>}
      </div>
    );
  }
  return (
    <div className="cos-rise flex items-end gap-2" data-testid="message" data-sender={m.sender_type}>
      <Avatar name={name} kind={m.sender_type} />
      <div className="mr-24 min-w-0">
        <div className="mb-1 flex items-center gap-1.5 pl-1">
          <span className={"text-caption font-medium " + (agent ? "text-accent" : "text-fg-muted")}>{name}</span>
          {agent && <Badge>{t("chat.botOf", { owner: dir.ownerName(agent) })}</Badge>}
          {m.trust === "untrusted" && m.sender_type === "agent" && <Badge tone="warn">{t("chat.untrusted")}</Badge>}
          <span className="text-tiny text-fg-subtle">{time}</span>
        </div>
        {m.body.trim() && <div className="w-fit max-w-full whitespace-pre-wrap break-words rounded-bubble bg-bubble px-3.5 py-2 text-body-lg text-bubble-fg">{m.body.includes("```") ? <Fenced body={m.body} /> : text}</div>}
        {m.rich && <div className="mt-2 w-full max-w-160"><RichMessage message={m} /></div>}
        {card && <div className="mt-2 w-full max-w-160"><CardView card={card} messageId={m.id} body={m.body} /></div>}
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
    <div className="mx-auto w-full max-w-215 px-6 pb-4">
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
      <div className="relative flex items-end gap-2 rounded-panel bg-surface p-1.5 shadow-popover">
        {options.length > 0 && (
          <div className="absolute bottom-full left-3 mb-2 w-72 overflow-hidden rounded-card bg-surface shadow-popover" data-testid="mention-menu">
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
        <span className="mb-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-pill bg-tag text-fg-muted" aria-hidden>
          <IconPlus width={14} height={14} />
        </span>
        <Textarea
          ref={ref}
          rows={1}
          data-testid="composer"
          className={cn("max-h-40 min-h-8 flex-1 bg-transparent px-1 py-1.5 shadow-none focus:shadow-none")}
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
        <Button
          size="icon"
          variant="accent"
          className="mb-0.5 h-7 w-7 shrink-0"
          onClick={onSend}
          disabled={!text.trim()}
          data-testid="send"
          aria-label={t("chat.send")}
          title={t("chat.send")}
        >
          <IconSend width={15} height={15} />
        </Button>
      </div>
      <div className="pt-1.5 text-center text-tiny text-fg-subtle">{t("chat.hint")}</div>
    </div>
  );
}
