import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const dir = join(__dirname, "themes");
const tokens = (file: string) =>
  [...readFileSync(join(dir, file), "utf8").matchAll(/(--[a-z0-9-]+)\s*:/g)].map((m) => m[1] ?? "").sort();

describe("themes", () => {
  it("every theme defines the same tokens", () => {
    const files = readdirSync(dir).filter((f) => f.endsWith(".css"));
    expect(files.length).toBeGreaterThanOrEqual(2);
    const base = tokens(files[0]!);
    for (const f of files) expect(tokens(f), f).toEqual(base);
  });

  it("tailwind theme maps every color token", () => {
    const css = readFileSync(join(__dirname, "styles.css"), "utf8");
    for (const t of tokens("light.css").filter((t) => t.startsWith("--cos-color-"))) {
      expect(css, t).toContain(`${t.replace("--cos-", "--")}: var(${t})`);
    }
  });

  it("no raw colors outside themes/", () => {
    const walk = (d: string): string[] =>
      readdirSync(d).flatMap((f) => {
        const p = join(d, f);
        return statSync(p).isDirectory() ? (f === "themes" ? [] : walk(p)) : [p];
      });
    for (const f of walk(__dirname).filter((f) => f.endsWith(".css"))) {
      expect(readFileSync(f, "utf8"), f).not.toMatch(/#[0-9a-fA-F]{3,8}\b|rgba?\(|hsla?\(/);
    }
  });
});
