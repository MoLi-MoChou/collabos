import { expect, type Browser, type Locator, type Page } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import { ROOT } from "./env";

export const SCREENS = join(ROOT, "docs/screens");
mkdirSync(SCREENS, { recursive: true });

export interface Person {
  email: string;
  name: string;
  handle: string;
  page: Page;
}

export async function newPerson(browser: Browser, email: string, name: string, handle: string): Promise<Person> {
  const ctx = await browser.newContext();
  const page = await ctx.newPage();
  page.on("pageerror", (e) => console.log(`[${handle}] pageerror`, e.message));
  return { email, name, handle, page };
}

export async function login(p: Person, invite = "", password = ""): Promise<void> {
  await p.page.goto("/");
  await p.page.locator("#email").fill(p.email);
  await p.page.locator("#name").fill(p.name);
  await p.page.locator("#handle").fill(p.handle);
  if (invite) await p.page.locator("#invite").fill(invite);
  if (password) await p.page.locator("#password").fill(password);
  await p.page.getByTestId("login-form").locator("button[type=submit]").click();
  await expect(p.page.getByTestId("login-form")).toHaveCount(0);
}

export async function shot(page: Page, name: string): Promise<void> {
  await page.screenshot({ path: join(SCREENS, `${name}.png`) });
}

export async function send(p: Person, text: string): Promise<void> {
  const box = p.page.getByTestId("composer");
  await box.fill(text);
  await p.page.getByTestId("send").click();
  await expect(p.page.getByTestId("pending-send")).toHaveCount(0);
}

/** Sends a delegation message and confirms the draft card; returns the task id. */
export async function delegate(p: Person, text: string, opts: { title: string; resources?: string }): Promise<string> {
  const before = await p.page.getByTestId("draft-card").count();
  await send(p, text);
  const draft = p.page.getByTestId("draft-card").nth(before);
  await expect(draft).toHaveAttribute("data-state", "draft");
  await draft.getByTestId("draft-title").fill(opts.title);
  if (opts.resources) await draft.getByTestId("draft-resources").fill(opts.resources);
  await draft.getByTestId("draft-confirm").click();
  await expect(draft).toHaveAttribute("data-state", "submitted");
  await expect(draft).toHaveAttribute("data-task-id", /.+/);
  return (await draft.getAttribute("data-task-id")) ?? "";
}

export function card(page: Page, kind: string, taskId: string): Locator {
  return page.locator(`[data-testid="${kind}-card"][data-task-id="${taskId}"]`).last();
}

export function notice(page: Page, taskId: string): Locator {
  return page.locator(`[data-testid="notice"][data-task-id="${taskId}"]`).last();
}

export async function openTask(page: Page, taskId: string): Promise<Locator> {
  await page.getByTestId("tab-tasks").click();
  const detail = page.getByTestId("task-detail");
  if (await detail.count()) await detail.getByRole("button").first().click();
  await page.locator(`[data-testid="dock-task"][data-task-id="${taskId}"]`).click();
  return page.getByTestId("task-detail");
}

export async function waitStatus(page: Page, taskId: string, status: RegExp, timeout = 8 * 60_000): Promise<void> {
  const line = page.locator(`[data-testid="dock-task"][data-task-id="${taskId}"] [data-testid="task-status"]`);
  await page.getByTestId("tab-tasks").click();
  const detail = page.getByTestId("task-detail");
  if (await detail.count()) await detail.getByRole("button").first().click();
  await expect(line).toHaveAttribute("data-status", status, { timeout });
}

/** Calls the API with the page's own session, to probe server-side authorization. */
export async function apiAs(page: Page, method: string, path: string, body?: unknown): Promise<{ status: number; code: string; json: Record<string, unknown> }> {
  return page.evaluate(
    async ({ method, path, body }) => {
      const s = JSON.parse(localStorage.getItem("collabos.session") ?? "{}");
      const r = await fetch(path, {
        method,
        headers: { "Content-Type": "application/json", Authorization: `Bearer ${s.token}`, "X-Workspace-ID": s.workspaceId },
        body: body === undefined ? undefined : JSON.stringify(body),
      });
      const j = await r.json().catch(() => ({}));
      return { status: r.status, code: String(j.code ?? ""), json: j };
    },
    { method, path, body },
  );
}

export async function pair(p: Person, device: string, chief: string, coder: string): Promise<string> {
  await p.page.getByTestId("tab-people").click();
  await p.page.getByTestId("pair-open").click();
  await p.page.getByTestId("pair-name").fill(device);
  if (chief) await p.page.getByTestId("pair-chief").fill(chief);
  if (coder) await p.page.getByTestId("pair-coder").fill(coder);
  await p.page.getByTestId("pair-submit").click();
  const cmd = await p.page.getByTestId("pair-command").innerText();
  const token = /cos_n_[A-Za-z0-9]+/.exec(cmd)?.[0] ?? "";
  expect(token).not.toBe("");
  return token;
}

/** Clicks a reject/deny button and fills the required reason (PRD 7.3). */
export async function rejectWith(scope: Locator, testId: string, reason: string): Promise<void> {
  await scope.getByTestId(testId).click();
  await scope.getByTestId("reason-input").fill(reason);
  await scope.getByTestId("reason-confirm").click();
}
