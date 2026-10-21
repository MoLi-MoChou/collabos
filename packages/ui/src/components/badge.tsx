import * as React from "react";
import { cn } from "../cn";

const tones = {
  neutral: "bg-tag text-tag-fg",
  accent: "bg-accent-soft text-accent",
  success: "bg-success-soft text-success",
  warn: "bg-warn-soft text-warn",
  danger: "bg-danger-soft text-danger",
} as const;

export type Tone = keyof typeof tones;

export function Badge({ tone = "neutral", className, ...props }: React.HTMLAttributes<HTMLSpanElement> & { tone?: Tone }) {
  return (
    <span
      className={cn("inline-flex items-center gap-1 rounded-sm px-1.5 py-px text-tiny font-medium whitespace-nowrap", tones[tone], className)}
      {...props}
    />
  );
}

export function Dot({ tone = "neutral" }: { tone?: Tone }) {
  const bg = { neutral: "bg-fg-subtle", accent: "bg-accent", success: "bg-success", warn: "bg-warn", danger: "bg-danger" }[tone];
  return <span className={cn("inline-block h-2 w-2 rounded-pill", bg)} />;
}
