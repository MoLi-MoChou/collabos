import { execFileSync } from "node:child_process";
import { join } from "node:path";
import { expect, test } from "@playwright/test";
import { apiAs, card, delegate, login, newPerson, shot, waitStatus, type Person } from "./helpers";
import { kill, restartDaemon, ROOT, RUN, sh } from "./env";

// Real GitHub provider: runs only with E2E_GITHUB_REPO=owner/name and a logged-in gh.
const REPO = process.env.E2E_GITHUB_REPO ?? "";
const REMOTE = `https://github.com/${REPO}.git`;
test.describe.configure({ mode: "serial" });
test.skip(!REPO, "set E2E_GITHUB_REPO to run the GitHub provider flow");

let zs: Person;
let ls: Person;

function gh(args: string[]): string {
  return execFileSync("gh", args, { encoding: "utf8" });
}

test("GitHub：推分支 → gh pr create → 具名审批人 gh pr merge --match-head-commit", async ({ browser }) => {
  for (const [who, home] of [["gh-a", "zs"], ["gh-b", "ls"]] as const) {
    sh("git", ["clone", "-q", REMOTE, join(RUN, who)]);
    kill(`daemon-${home}`);
    sh(join(ROOT, "server/bin/collab"), ["repo", "add", join(RUN, who), "--remote", REMOTE], { COLLAB_HOME: join(RUN, `home-${home}`) });
    restartDaemon(home);
  }
  zs = await newPerson(browser, "zhangsan@demo.local", "张三", "zhangsan");
  ls = await newPerson(browser, "lisi@demo.local", "李四", "lisi");
  await login(zs);
  await login(ls);
  const p = await apiAs(zs.page, "POST", "/api/projects", { name: "calc-gh", remote_url: REMOTE });
  expect(p.status).toBe(200);
  expect(p.json.provider).toBe("github");
  const room = String(p.json.room_id);
  await zs.page.goto(`/rooms/${room}`);
  await ls.page.goto(`/rooms/${room}`);
  await zs.page.getByTestId("tab-people").click();
  await expect(zs.page.locator('[data-testid="device"][data-status="online"]')).toHaveCount(3, { timeout: 60_000 });

  const stamp = Date.now();
  const id = await delegate(zs, `@ls-coder 请在 src/math.ts 中新增并导出函数 square${stamp}(a: number): number，返回 a*a。只修改这个文件。`, {
    title: `GitHub e2e square${stamp}`,
    resources: "src/**",
  });
  const intake = card(ls.page, "intake", id);
  if (await intake.getByTestId("intake-accept").count()) await intake.getByTestId("intake-accept").click();
  const merge = card(zs.page, "merge", id);
  await expect(merge).toHaveAttribute("data-state", "pending", { timeout: 8 * 60_000 });
  const prs = JSON.parse(gh(["pr", "list", "-R", REPO, "--state", "open", "--json", "url,headRefName,headRefOid"])) as { url: string; headRefName: string; headRefOid: string }[];
  const pr = prs.find((x) => x.headRefName === `collabos/task-${id}`);
  expect(pr, "PR opened on GitHub").toBeTruthy();
  await expect(merge.getByTestId("merge-commit")).toHaveText(pr!.headRefOid.slice(0, 10));
  await shot(zs.page, "20-github-pr-pending");
  await merge.getByTestId("merge-approve").click();
  await expect(merge).toHaveAttribute("data-state", "merged", { timeout: 3 * 60_000 });
  await waitStatus(zs.page, id, /completed/);
  const view = JSON.parse(gh(["pr", "view", pr!.url, "--json", "state,mergeCommit"])) as { state: string; mergeCommit: { oid: string } };
  expect(view.state).toBe("MERGED");
  const main = gh(["api", `repos/${REPO}/contents/src/math.ts`, "-H", "Accept: application/vnd.github.raw"]);
  expect(main).toContain(`square${stamp}`);
  console.log("GITHUB_PR", pr!.url);
  await shot(zs.page, "21-github-merged");
});
