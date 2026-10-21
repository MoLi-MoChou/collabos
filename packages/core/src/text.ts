/** Text helpers shared by every client (kept out of views so they are testable). */

export interface FencePart {
  text: string;
  code: string | null;
  lang: string;
}

/** Splits text into plain parts and ``` fenced code blocks. */
export function splitFences(body: string): FencePart[] {
  const out: FencePart[] = [];
  const re = /```([\w+-]*)\n([\s\S]*?)```/g;
  let last = 0;
  for (let m = re.exec(body); m; m = re.exec(body)) {
    if (m.index > last) out.push({ text: body.slice(last, m.index), code: null, lang: "" });
    out.push({ text: "", code: (m[2] ?? "").replace(/\n$/, ""), lang: m[1] ?? "" });
    last = m.index + m[0].length;
  }
  if (last < body.length) out.push({ text: body.slice(last), code: null, lang: "" });
  return out;
}

export type DiffKind = "add" | "del" | "hunk" | "file" | "ctx";

/** Classifies unified-diff lines for rendering. */
export function diffLines(diff: string): { kind: DiffKind; text: string }[] {
  return diff.split("\n").map((text) => {
    if (text.startsWith("+++") || text.startsWith("---") || text.startsWith("diff --git")) return { kind: "file" as const, text };
    if (text.startsWith("@@")) return { kind: "hunk" as const, text };
    if (text.startsWith("+")) return { kind: "add" as const, text };
    if (text.startsWith("-")) return { kind: "del" as const, text };
    return { kind: "ctx" as const, text };
  });
}

/** True when a rejection/denial comment is acceptable (server requires one). */
export function validReason(s: string): boolean {
  return s.trim().length >= 2;
}
