import { execFileSync } from "node:child_process";
import { readFileSync } from "node:fs";
import { join, resolve } from "node:path";

export const ROOT = resolve(import.meta.dirname, "../..");
export const RUN = join(ROOT, "e2e/.run");
export const PORT = Number(process.env.E2E_PORT ?? 8092);
export const SERVER = `http://localhost:${PORT}`;
export const REMOTE = join(RUN, "remote.git");

export function sh(cmd: string, args: string[], env: Record<string, string> = {}): string {
  return execFileSync(cmd, args, { cwd: ROOT, env: { ...process.env, ...env }, encoding: "utf8" });
}

export function pid(name: string): number {
  return Number(readFileSync(join(RUN, `${name}.pid`), "utf8").trim());
}

export function kill(name: string, signal: NodeJS.Signals = "SIGTERM"): void {
  try {
    process.kill(pid(name), signal);
  } catch {
    /* already gone */
  }
}

export function startDaemon(name: string, token: string, repo: string, env: Record<string, string> = {}): void {
  sh(join(ROOT, "scripts/daemon.sh"), [RUN, name, SERVER, token, join(RUN, repo), REMOTE], env);
}

export function restartDaemon(name: string, env: Record<string, string> = {}): void {
  sh("bash", ["-c", `COLLAB_HOME='${RUN}/home-${name}' nohup '${ROOT}/server/bin/collab' run >>'${RUN}/daemon-${name}.log' 2>&1 & echo $! >'${RUN}/daemon-${name}.pid'`], env);
}

export function startServer(): void {
  sh("bash", [join(RUN, "server-start.sh")]);
}

export function remoteFile(path: string, branch = "main"): string {
  try {
    return sh("git", ["--git-dir", REMOTE, "show", `${branch}:${path}`]);
  } catch {
    return "";
  }
}
