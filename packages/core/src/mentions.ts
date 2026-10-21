const RE = /@([a-zA-Z0-9][a-zA-Z0-9_-]{1,31})/g;

/** Unique lower-case handles mentioned in text (mirrors the server parser). */
export function parseMentions(text: string): string[] {
  const out: string[] = [];
  for (const m of text.matchAll(RE)) {
    const h = (m[1] ?? "").toLowerCase();
    if (h && !out.includes(h)) out.push(h);
  }
  return out;
}

/** Returns the partial handle being typed at the caret, or null. */
export function mentionQuery(text: string, caret: number): string | null {
  const before = text.slice(0, caret);
  const m = /(^|\s)@([a-zA-Z0-9_-]*)$/.exec(before);
  return m ? (m[2] ?? "").toLowerCase() : null;
}

/** Replaces the partial mention at the caret with the full handle. */
export function completeMention(text: string, caret: number, handle: string): { text: string; caret: number } {
  const before = text.slice(0, caret).replace(/@([a-zA-Z0-9_-]*)$/, `@${handle} `);
  return { text: before + text.slice(caret), caret: before.length };
}

/** Splits text into plain and mention segments for rendering. */
export function segments(text: string): { text: string; mention: boolean }[] {
  const out: { text: string; mention: boolean }[] = [];
  let last = 0;
  for (const m of text.matchAll(RE)) {
    const i = m.index ?? 0;
    if (i > last) out.push({ text: text.slice(last, i), mention: false });
    out.push({ text: m[0], mention: true });
    last = i + m[0].length;
  }
  if (last < text.length) out.push({ text: text.slice(last), mention: false });
  return out;
}
