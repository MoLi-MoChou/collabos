import { cn } from "../cn";

const palette = ["bg-accent", "bg-success", "bg-warn", "bg-danger", "bg-fg-muted"] as const;

function hash(s: string): number {
  let h = 0;
  for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return h;
}

export function Avatar({ name, kind = "user", size = "md" }: { name: string; kind?: "user" | "agent" | "system"; size?: "sm" | "md" }) {
  const letter = (name.trim()[0] ?? "?").toUpperCase();
  const bg = kind === "system" ? "bg-fg-subtle" : palette[hash(name) % palette.length];
  return (
    <span
      className={cn(
        "inline-flex shrink-0 items-center justify-center font-semibold text-accent-fg",
        kind === "agent" ? "rounded-control" : "rounded-pill",
        size === "sm" ? "h-6 w-6 text-caption" : "h-8 w-8 text-body",
        bg,
      )}
    >
      {letter}
    </span>
  );
}
