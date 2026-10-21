import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import zh from "./zh-Hans.json";
import en from "./en.json";

function keys(o: object, prefix = ""): string[] {
  return Object.entries(o).flatMap(([k, v]) => (typeof v === "object" && v !== null ? keys(v, `${prefix}${k}.`) : [`${prefix}${k}`]));
}

describe("locales", () => {
  it("zh-Hans and en have identical keys", () => {
    expect(keys(zh).sort()).toEqual(keys(en).sort());
  });
  it("every static t() key used in views exists", () => {
    const walk = (d: string): string[] => readdirSync(d).flatMap((f) => (statSync(join(d, f)).isDirectory() ? walk(join(d, f)) : [join(d, f)]));
    const all = new Set(keys(zh));
    const src = join(__dirname, "..");
    for (const f of walk(src).filter((f) => f.endsWith(".tsx"))) {
      for (const m of readFileSync(f, "utf8").matchAll(/\bt\("([a-zA-Z0-9_.]+)"/g)) expect(all.has(m[1]!), `${m[1]} in ${f}`).toBe(true);
    }
  });
});
