import type { Envelope } from "./protocol.gen";

export interface RealtimeOptions {
  url: string; // ws(s)://host/ws
  token: string;
  workspaceId: string;
  onEnvelope: (e: Envelope) => void;
  onStatus?: (connected: boolean) => void;
  /** Called after a reconnect so callers can refetch what they missed. */
  onResync?: () => void;
}

function isEnvelope(v: unknown): v is Envelope {
  return typeof v === "object" && v !== null && "type" in v && typeof v.type === "string";
}

/** UI WebSocket with heartbeat and exponential-backoff reconnect. */
export class RealtimeClient {
  private ws: WebSocket | null = null;
  private timer: ReturnType<typeof setInterval> | null = null;
  private retry = 0;
  private closed = false;
  private everConnected = false;

  constructor(private o: RealtimeOptions) {}

  start(): void {
    this.closed = false;
    this.connect();
  }

  stop(): void {
    this.closed = true;
    if (this.timer) clearInterval(this.timer);
    this.ws?.close();
  }

  private connect(): void {
    const u = `${this.o.url}?token=${encodeURIComponent(this.o.token)}&workspace_id=${encodeURIComponent(this.o.workspaceId)}`;
    const ws = new WebSocket(u);
    this.ws = ws;
    ws.onopen = () => {
      this.retry = 0;
      this.o.onStatus?.(true);
      if (this.everConnected) this.o.onResync?.();
      this.everConnected = true;
      this.timer = setInterval(() => {
        if (ws.readyState === WebSocket.OPEN) {
          ws.send(JSON.stringify({ type: "HEARTBEAT_PING", trace_id: "", msg_id: crypto.randomUUID(), timestamp: new Date().toISOString(), payload: {} }));
        }
      }, 20000);
    };
    ws.onmessage = (ev) => {
      try {
        const data: unknown = JSON.parse(String(ev.data));
        if (isEnvelope(data)) this.o.onEnvelope(data);
      } catch {
        /* ignore malformed frames */
      }
    };
    ws.onclose = () => {
      if (this.timer) clearInterval(this.timer);
      this.o.onStatus?.(false);
      if (this.closed) return;
      const delay = Math.min(15000, 500 * 2 ** this.retry++);
      setTimeout(() => !this.closed && this.connect(), delay);
    };
  }
}
