import { expect, test } from "@playwright/test";
import { apiAs, card, delegate, login, newPerson, notice, openTask, pair, send, shot, waitStatus, type Person } from "./helpers";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { kill, remoteFile, REMOTE, restartDaemon, RUN, startDaemon, startServer } from "./env";

test.describe.configure({ mode: "serial" });

let zs: Person; // 张三: workspace owner, project lead, requester
let ls: Person; // 李四: owns @ls-coder (bwrap sandbox)
let ww: Person; // 王五: owns @ww-coder on a device without a sandbox
let invite = "";

test.beforeAll(async ({ browser }) => {
  zs = await newPerson(browser, "zhangsan@demo.local", "张三", "zhangsan");
  ls = await newPerson(browser, "lisi@demo.local", "李四", "lisi");
  ww = await newPerson(browser, "wangwu@demo.local", "王五", "wangwu");
});

test("登录、创建项目、邀请成员", async () => {
  await zs.page.goto("/");
  await shot(zs.page, "01-login");
  await login(zs);
  await expect(zs.page.getByTestId("project-form")).toBeVisible();
  await zs.page.locator("#pname").fill("calc");
  await zs.page.locator("#premote").fill(REMOTE);
  await shot(zs.page, "02-setup");
  await zs.page.getByTestId("project-form").locator("button[type=submit]").click();
  await expect(zs.page.getByTestId("room-title")).toContainText("calc");
  invite = (await zs.page.getByTestId("invite-code").innerText()).trim();
  expect(invite).toMatch(/^[0-9a-f]{12}$/);
  await login(ls, invite);
  await login(ww, invite);
  await expect(ls.page.getByTestId("room-title")).toContainText("calc");
  await zs.page.getByTestId("tab-people").click();
  await expect(zs.page.getByTestId("member")).toHaveCount(3);
});

test("配对三台设备并启动守护进程", async () => {
  startDaemon("zs", await pair(zs, "zs-laptop", "zs-chief", "zs-coder"), "repo-a");
  await shot(zs.page, "03-pair");
  await zs.page.keyboard.press("Escape");
  startDaemon("ls", await pair(ls, "ls-laptop", "", "ls-coder"), "repo-b");
  await ls.page.keyboard.press("Escape");
  startDaemon("ww", await pair(ww, "ww-desktop", "", "ww-coder"), "repo-c", { COLLAB_SANDBOX: "none" });
  await ww.page.keyboard.press("Escape");
  await expect(zs.page.locator('[data-testid="device"][data-status="online"]')).toHaveCount(3, { timeout: 30_000 });
  await expect(zs.page.getByTestId("device").filter({ hasText: "ww-desktop" })).toContainText("none");
  await expect(zs.page.getByTestId("device").filter({ hasText: "ls-laptop" })).toContainText("bubblewrap");
  await expect(ls.page.getByTestId("device-status")).toHaveAttribute("data-status", "online");
  await shot(zs.page, "04-people-devices");
});

test("跨所有者委托：接单 → 越权授权卡 → 非审批人被拒 → 具名审批人合并 → MEMORY.md 确认", async () => {
  const id = await delegate(
    zs,
    "@ls-coder 请在 src/math.ts 中新增并导出函数 multiply(a: number, b: number): number，然后在 README.md 末尾追加一行：- multiply",
    { title: "新增 multiply", resources: "src/**" },
  );
  await shot(zs.page, "05-draft-submitted");
  const intake = card(ls.page, "intake", id);
  await expect(intake).toHaveAttribute("data-state", "pending");
  await expect(card(zs.page, "intake", id).getByTestId("intake-accept")).toHaveCount(0); // only the owner decides
  expect((await apiAs(zs.page, "POST", `/api/tasks/${id}/intake`, { decision: "accept" })).status).toBe(403);
  await shot(ls.page, "06-intake-pending");
  await intake.getByTestId("intake-accept").click();
  await expect(intake).toHaveAttribute("data-state", "accepted");

  const perm = card(zs.page, "permission", id);
  await expect(perm).toHaveAttribute("data-state", "pending", { timeout: 6 * 60_000 });
  await expect(perm).toContainText("README.md");
  await shot(zs.page, "07-permission-card");
  await perm.getByTestId("permission-allow").click();
  await expect(perm).toHaveAttribute("data-state", "approved");

  const merge = card(zs.page, "merge", id);
  await expect(merge).toHaveAttribute("data-state", "pending", { timeout: 8 * 60_000 });
  await waitStatus(zs.page, id, /approval_required/);
  const lsMerge = card(ls.page, "merge", id);
  await expect(lsMerge.getByTestId("merge-approve")).toHaveCount(0);
  await expect(lsMerge).toContainText(/仅具名审批人/);
  const approval = (await merge.getAttribute("data-approval-id")) ?? "";
  expect(await apiAs(ls.page, "POST", `/api/approvals/${approval}/decide`, { decision: "approve" })).toMatchObject({ status: 403 });
  await shot(zs.page, "08-merge-pending");
  await merge.getByTestId("merge-approve").click();
  await expect(merge).toHaveAttribute("data-state", "merged", { timeout: 2 * 60_000 });
  await waitStatus(zs.page, id, /completed/);
  expect(remoteFile("src/math.ts")).toContain("multiply");
  expect(remoteFile("README.md")).toContain("multiply");

  const term = await openTask(zs.page, id);
  await expect(term.getByTestId("terminal")).toContainText(/PR|merged|已合并/);
  // The sandbox had no network of its own; the model API went through the egress allowlist.
  await expect(term.getByTestId("terminal")).toContainText("sandbox network isolated");
  await expect(term.getByTestId("terminal")).toContainText(/network allowed: (models\.)?opencode\.ai/);
  await shot(zs.page, "09-task-detail-terminal");

  const mem = card(zs.page, "memory", id);
  await expect(mem).toHaveAttribute("data-state", "pending");
  await expect(card(ls.page, "memory", id).getByTestId("memory-confirm")).toHaveCount(0);
  const memId = (await mem.getAttribute("data-memory-id")) ?? "";
  expect((await apiAs(ls.page, "POST", `/api/memory/${memId}/decide`, { decision: "confirm" })).status).toBe(403);
  await mem.getByTestId("memory-confirm").click();
  await expect(mem).toHaveAttribute("data-state", "confirmed");
  await zs.page.getByTestId("tab-memory").click();
  await expect(zs.page.getByTestId("memory-version")).toContainText("2");
  await expect(zs.page.getByTestId("memory-content")).toContainText("新增 multiply");
  await shot(zs.page, "10-memory");
});

test("拒绝路径：所有者拒绝越权、审批人驳回合并", async () => {
  const id = await delegate(
    zs,
    "@ls-coder 请在 src/math.ts 中新增并导出函数 subtract(a: number, b: number): number，然后在 README.md 末尾追加一行：- subtract",
    { title: "新增 subtract", resources: "src/**" },
  );
  await card(ls.page, "intake", id).getByTestId("intake-accept").click();
  const perm = card(ls.page, "permission", id);
  await expect(perm).toHaveAttribute("data-state", "pending", { timeout: 6 * 60_000 });
  await perm.getByTestId("permission-deny").click();
  await expect(perm).toHaveAttribute("data-state", "denied");
  const merge = card(zs.page, "merge", id);
  await expect(merge).toHaveAttribute("data-state", "pending", { timeout: 8 * 60_000 });
  expect(remoteFile("README.md", `collabos/task-${id}`)).not.toContain("subtract"); // the denied edit never reached the branch
  await merge.getByTestId("merge-reject").click();
  await expect(merge).toHaveAttribute("data-state", "rejected");
  await waitStatus(zs.page, id, /canceled/);
  expect(remoteFile("src/math.ts")).not.toContain("subtract");
  await shot(zs.page, "11-merge-rejected");
});

test("拒绝路径：拒绝接单、关闭接单、超出配额、无沙箱", async () => {
  const a = await delegate(zs, "@ls-coder 请删除整个 src 目录", { title: "删除 src" });
  const intake = card(ls.page, "intake", a);
  await intake.getByTestId("intake-reject").click();
  await expect(intake).toHaveAttribute("data-state", "rejected");
  await waitStatus(zs.page, a, /rejected/);

  // Owner closes intake.
  await ls.page.getByTestId("tab-people").click();
  const agent = ls.page.locator('[data-testid="agent"][data-handle="ls-coder"]');
  await agent.getByTestId("intake-edit").click();
  await agent.getByTestId("intake-mode-closed").click();
  await agent.getByTestId("intake-save").click();
  await expect(agent).toContainText("不接单");
  const b = await delegate(zs, "@ls-coder 给 README 加标题", { title: "关闭时委托" });
  await expect(notice(zs.page, b)).toHaveAttribute("data-code", "intake_denied");

  // Daily quota of 1 is already used up today.
  await agent.getByTestId("intake-edit").click();
  await agent.getByTestId("intake-mode-manual").click();
  await agent.getByTestId("intake-quota").fill("1");
  await agent.getByTestId("intake-save").click();
  const c = await delegate(zs, "@ls-coder 给 README 加标题", { title: "超配额委托" });
  await expect(notice(zs.page, c)).toHaveAttribute("data-code", "quota_exceeded");

  // A device without an OS sandbox never takes another owner's work.
  const d = await delegate(zs, "@ww-coder 给 README 加标题", { title: "无沙箱委托" });
  await expect(notice(zs.page, d)).toHaveAttribute("data-code", "sandbox_unavailable");
  await shot(zs.page, "12-rejections");

  // Allowlist 张三 with unlimited quota for the next tests (auto-accept).
  await agent.getByTestId("intake-edit").click();
  await agent.getByTestId("intake-mode-allowlist").click();
  await agent.getByRole("button", { name: "张三" }).click();
  await agent.getByTestId("intake-quota").fill("0");
  await agent.getByTestId("intake-save").click();
  await expect(agent).toContainText("白名单");
});

test("服务器宕机：守护进程离线记账，重连后回放", async () => {
  const id = await delegate(zs, "@ls-coder 请在 src/math.ts 中新增并导出函数 divide(a: number, b: number): number（b 为 0 时抛出错误）", {
    title: "新增 divide",
  });
  await waitStatus(zs.page, id, /working/, 3 * 60_000); // allowlist → auto accepted, dispatched
  kill("server");
  await expect(zs.page.getByTestId("reconnecting")).toBeVisible({ timeout: 30_000 });
  await shot(zs.page, "13-server-offline");
  await zs.page.waitForTimeout(25_000); // the agent keeps working; frames go to the journal
  startServer();
  await expect(zs.page.getByTestId("reconnecting")).toHaveCount(0, { timeout: 60_000 });
  // The daemon journaled the frames it could not deliver and replayed them.
  await expect.poll(() => readFileSync(join(RUN, "daemon-ls.log"), "utf8"), { timeout: 60_000 }).toMatch(/replayed journal .*frames=[1-9]/);
  await expect(card(zs.page, "merge", id)).toHaveAttribute("data-state", "pending", { timeout: 8 * 60_000 });
  const term = await openTask(zs.page, id);
  await expect(term.getByTestId("terminal")).toContainText(/PR/);
  await shot(zs.page, "14-replayed");
  await card(zs.page, "merge", id).getByTestId("merge-approve").click();
  await expect(card(zs.page, "merge", id)).toHaveAttribute("data-state", "merged", { timeout: 2 * 60_000 });
  expect(remoteFile("src/math.ts")).toContain("divide");
});

test("守护进程崩溃：任务标记 daemon_lost；离线时排队，重启后派发；取消任务", async () => {
  const id = await delegate(zs, "@ls-coder 请在 src/math.ts 中新增并导出函数 power(a: number, b: number): number", { title: "新增 power" });
  await waitStatus(zs.page, id, /working/, 3 * 60_000);
  kill("daemon-ls", "SIGKILL");
  await zs.page.getByTestId("tab-people").click();
  await expect(zs.page.locator('[data-testid="device"][data-status="online"]')).toHaveCount(2, { timeout: 60_000 });
  const q = await delegate(zs, "@ls-coder 请在 src/math.ts 中新增并导出函数 mod(a: number, b: number): number", { title: "新增 mod" });
  await waitStatus(zs.page, q, /queued/, 60_000);
  await shot(zs.page, "15-queued");
  restartDaemon("ls");
  await waitStatus(zs.page, id, /failed/, 60_000);
  const d = await openTask(zs.page, id);
  await expect(d).toContainText(/设备丢失任务|daemon_lost/);
  await waitStatus(zs.page, q, /working/, 2 * 60_000);
  const dq = await openTask(zs.page, q);
  await dq.getByTestId("task-cancel").click();
  await waitStatus(zs.page, q, /canceled/, 60_000);
  await shot(zs.page, "16-daemon-lost-canceled");
});

test("Chief 在群聊中规划并起草委托", async () => {
  const before = await zs.page.getByTestId("draft-card").count();
  const agentMsgs = await zs.page.locator('[data-testid="message"][data-sender="agent"]').count();
  await send(zs, "@zs-chief 我们需要在 src/math.ts 中新增导出函数 negate(a: number): number。这件事请交给李四的专家 Bot ls-coder 来做，请直接起草委托。");
  await expect(zs.page.getByTestId("draft-card")).toHaveCount(before); // no direct mention, so no immediate draft
  await expect.poll(() => zs.page.locator('[data-testid="message"][data-sender="agent"]').count(), { timeout: 5 * 60_000 }).toBeGreaterThan(agentMsgs);
  await expect(zs.page.getByTestId("draft-card")).toHaveCount(before + 1, { timeout: 5 * 60_000 });
  const draft = zs.page.getByTestId("draft-card").last();
  await expect(draft).toContainText("ls-coder");
  await shot(zs.page, "17-chief-draft");
  await draft.getByRole("button", { name: "丢弃" }).click();
  await expect(draft).toHaveAttribute("data-state", "discarded");
});

test("审计日志与深色主题", async () => {
  await zs.page.getByTestId("tab-audit").click();
  for (const a of ["intake.accept", "intake.reject", "permission.approved", "permission.denied", "merge.approve", "merge.reject", "merge.executed", "memory.confirmed"]) {
    await expect(zs.page.locator(`[data-testid="audit-row"][data-action="${a}"]`).first()).toBeVisible();
  }
  await shot(zs.page, "18-audit");
  await zs.page.getByTestId("theme-toggle").click();
  await expect(zs.page.locator("html")).toHaveAttribute("data-theme", "dark");
  await zs.page.getByTestId("tab-tasks").click();
  await shot(zs.page, "19-dark");
});
