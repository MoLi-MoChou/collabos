import type { TaskStatus } from "./api/schemas";

export type StatusTone = "neutral" | "accent" | "success" | "warn" | "danger";

/** Visual tone for each unified task status (PRD 6.2). */
export function statusTone(s: TaskStatus): StatusTone {
  switch (s) {
    case "completed":
      return "success";
    case "working":
    case "accepted":
    case "queued":
      return "accent";
    case "pending_intake":
    case "approval_required":
    case "input_required":
    case "running_offline":
      return "warn";
    case "failed":
    case "rejected":
    case "unknown":
      return "danger";
    default:
      return "neutral";
  }
}

export function isTerminal(s: TaskStatus): boolean {
  return s === "completed" || s === "failed" || s === "canceled" || s === "rejected";
}
