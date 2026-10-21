import { join } from "node:path";
import { ROOT, RUN, PORT, sh } from "./env";

export default function setup(): void {
  sh(join(ROOT, "scripts/stack.sh"), [RUN, String(PORT), "collab_e2e"], { COLLAB_OFFLINE_GRACE: "10m" });
}
