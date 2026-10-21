# 验证记录

环境：Linux x86_64（本机 box），Go 1.27、Node 22、pnpm 10、PostgreSQL、bubblewrap、opencode（免费模型 `opencode/deepseek-v4.1-flash-free`）、Google Chrome。

## 自动化检查（均通过）

| 检查 | 命令 |
| --- | --- |
| Go 静态检查与测试（含 `-race`，含对真实 Postgres 的服务层集成测试） | `cd server && go vet ./... && go test -race ./...` |
| 迁移从零执行 / 回滚 / 再执行 | `server/bin/migrate up && down && up`（CI 中同样执行） |
| 前端类型检查 / Lint（含自定义令牌与 i18n 规则）/ 单测 / 构建 | `pnpm typecheck && pnpm lint && pnpm test && pnpm build` |
| 端到端（9 个场景，真实浏览器 + 3 个真实守护进程 + 真实 opencode + bwrap） | `pnpm e2e` |

## 端到端覆盖（e2e/tests/collab.spec.ts）

1. 开发登录、创建项目（自动建群与 MEMORY.md v1）、邀请码加入。
2. UI 配对三台设备，`collab login / repo add / run`，设备能力（bubblewrap / none）展示。
3. 跨所有者委托：草稿卡 → 确认 → 只有所有者能接单（API 403 验证）→ bwrap 中执行 → 越权编辑 README.md 触发授权卡 → 允许 → 推分支开 PR → 非审批人无按钮且 API 403 → 具名审批人合并（由审批人自己的守护进程执行，远端 main 含改动）→ MEMORY.md v2 由非负责人确认被拒（403）、负责人确认成功。
4. 拒绝越权（被拒改动不进入分支）与驳回合并（远端 main 不变）。
5. 拒绝接单、关闭接单（intake_denied）、超出每日配额（quota_exceeded）、无沙箱设备（sandbox_unavailable）、白名单自动接单。
6. 服务器宕机：任务执行期间杀掉服务器，UI 显示重连；守护进程把帧写入 SQLite 日志，服务器恢复后回放（日志 `replayed journal frames=N`），任务正常结束并合并。
7. 守护进程崩溃（SIGKILL）：执行中任务被标记 `daemon_lost`；离线期间的新任务排队（queued），重启后派发；从任务面板取消任务。
8. Chief Bot：在群聊里 @自己的 Chief，Chief 回复并起草对他人专家 Bot 的委托草稿卡。
9. 审计日志（接单、拒绝、授权、合并、记忆确认等）与深色主题。

10. GitHub 提供方（`E2E_GITHUB_REPO=MoLi-MoChou/collabos-sandbox-test pnpm e2e`）：项目远端为 GitHub 时自动识别 provider；执行方守护进程推分支并用本人 gh 凭据 `gh pr create`；具名审批人的守护进程执行 `gh pr merge --match-head-commit <已批准提交>`；GitHub 上 PR 状态为 MERGED，main 含改动。实测 PR：https://github.com/MoLi-MoChou/collabos-sandbox-test/pull/1

截图：`docs/screens/`。

## E2E 中发现并修复的问题

- 服务器在发送握手确认之前就下发排队任务/重发决定，导致守护进程报 `bad handshake ack` 并丢失任务 → 改为确认之后再发送。
- 守护进程崩溃后，日志中未完成任务没有完成帧，服务器会一直等待 → 启动时为这些任务补写 `daemon_lost` 完成帧。
- `useEffect` 返回 `scrollIntoView()` 的返回值导致任务详情页崩溃 → 修复。
- Tailwind `@theme inline` 令牌自引用导致工具类为空 → 主题原始值改名为 `--cos-*`，并在测试中校验映射。

## 未在运行时验证的部分

- **macOS Seatbelt 沙箱**：SBPL 配置文件已实现并有单元测试（生成内容），但未在 macOS 上实际运行。
- **Windows**：守护进程可编译（进程组处理有 Windows 实现），但没有 OS 沙箱，未在 Windows 上运行；无沙箱设备只能接自己的任务。
- 出站白名单已在 Linux 上实测：`cmd/collab/netbridge_test.go` 在真实 bwrap 中验证允许域名可达、非白名单返回 403、直连失败；E2E 中 opencode 经代理访问 `opencode.ai` / `models.opencode.ai` 完成任务。
- **macOS**：Seatbelt 下暂不做出站白名单（沙箱共享网络），未运行时验证。
