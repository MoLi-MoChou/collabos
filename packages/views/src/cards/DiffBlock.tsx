import { diffLines } from "@collabos/core";

/** Unified diff with per-line colouring (merge card diff preview, FR-CLI-203). */
export function DiffBlock({ diff, max = 400 }: { diff: string; max?: number }) {
  const lines = diffLines(diff);
  const shown = lines.slice(0, max);
  return (
    <pre className="max-h-96 overflow-auto rounded-control bg-term-bg py-2 font-mono text-mono text-term-fg" data-testid="diff-block">
      {shown.map((l, i) => (
        <div key={i} data-kind={l.kind} className={"px-3 " + (l.kind === "add" ? "bg-success-soft text-success" : l.kind === "del" ? "bg-danger-soft text-danger" : l.kind === "hunk" ? "text-accent" : l.kind === "file" ? "font-semibold" : "")}>
          {l.text || " "}
        </div>
      ))}
      {lines.length > max && <div className="px-3 text-fg-subtle">…</div>}
    </pre>
  );
}
