import { cn } from "../cn";

/** Soft-tinted round avatar (DS Bot style). Agents get a squircle. */
const palette = [
  "bg-accent-soft text-accent",
  "bg-success-soft text-success",
  "bg-warn-soft text-warn",
  "bg-danger-soft text-danger",
  "bg-tag text-fg-muted",
] as const;

function hash(s: string): number {
  let h = 0;
  for (const c of s) h = (h * 31 + c.charCodeAt(0)) >>> 0;
  return h;
}

export function avatarTone(name: string): number {
  return hash(name) % palette.length;
}

export function Avatar({ name, kind = "user", size = "md" }: { name: string; kind?: "user" | "agent" | "system"; size?: "sm" | "md" | "lg" }) {
  const letter = (name.trim()[0] ?? "?").toUpperCase();
  const tone = kind === "system" ? "bg-tag text-fg-subtle" : palette[avatarTone(name)];
  return (
    <span
      className={cn(
        "inline-flex shrink-0 select-none items-center justify-center font-semibold",
        kind === "agent" ? "rounded-squircle" : "rounded-pill",
        size === "sm" ? "h-6 w-6 text-tiny" : size === "lg" ? "h-14 w-14 text-heading" : "h-7 w-7 text-caption",
        tone,
      )}
    >
      {letter}
    </span>
  );
}
