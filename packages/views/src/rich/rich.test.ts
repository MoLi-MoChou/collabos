import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { RICH_WHITELIST } from "./library";

describe("OpenUI whitelist", () => {
  it("matches the server whitelist exactly", () => {
    const go = readFileSync(fileURLToPath(new URL("../../../../server/internal/service/richmsg.go", import.meta.url)), "utf8");
    const block = /RichComponents = map\[string\]bool\{([^}]*)\}/.exec(go)?.[1] ?? "";
    const server = [...block.matchAll(/"(\w+)": true/g)].map((m) => m[1]).sort();
    expect([...RICH_WHITELIST].sort()).toEqual(server);
  });
});
