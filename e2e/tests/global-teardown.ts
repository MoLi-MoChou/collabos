import { readdirSync } from "node:fs";
import { kill, RUN } from "./env";

export default function teardown(): void {
  if (process.env.E2E_KEEP) return;
  for (const f of readdirSync(RUN).filter((f) => f.endsWith(".pid"))) kill(f.replace(/\.pid$/, ""));
}
