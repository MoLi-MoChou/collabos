import * as React from "react";
import { cn } from "../cn";

export function Spinner({ className }: { className?: string }) {
  return <span className={cn("inline-block h-3.5 w-3.5 animate-spin rounded-pill border-2 border-border border-t-accent", className)} />;
}

export function Separator({ className }: { className?: string }) {
  return <div className={cn("h-px w-full bg-border", className)} />;
}

export function Kbd({ className, ...props }: React.HTMLAttributes<HTMLElement>) {
  return <kbd className={cn("rounded-sm border border-border bg-surface-2 px-1.5 font-mono text-caption text-fg-muted", className)} {...props} />;
}

export function Code({ className, ...props }: React.HTMLAttributes<HTMLElement>) {
  return <code className={cn("rounded-sm bg-surface-2 px-1 font-mono text-mono text-fg", className)} {...props} />;
}
