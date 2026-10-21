# PRD V0.4 覆盖表

本文把 [PRD V0.4](./collabos-prd-v0.4.md) 的每条 FR 和各节中的要求逐条对应到实现和测试。凡未实现或只部分实现的条目，都写明原因。

**路径约定**
- `S:` 表示 `server/`。
- `W:` 表示前端，即 `packages/` 与 `apps/web/`。
- `E2E:` 表示 `e2e/tests/collab.spec.ts` 中的场景。
- `IT:` 表示 `server/internal/service/*_integration_test.go`（真实 PostgreSQL + 伪 daemon WebSocket）。

**状态说明**
- ✅：已实现并有测试。
- ◐：部分实现，原因见「说明」列。
- ✗：未实现，原因见「说明」列。

## 7.1 客户端

| 条目 | 状态 | 实现 | 测试 | 说明 |
|---|---|---|---|---|
| FR-CLI-101 虚拟滚动 | ◐ | W: `views/src/chat/ChatView.tsx` 只挂载最新 150 条消息，更早的按需加载；行使用 `content-visibility:auto`。任务终端只渲染最近 500 行。 | E2E 全部场景（列表渲染） | 未做真正的虚拟列表库。PRD 12.1 把「1 万条无掉帧」列为阶段 2 目标、不作门槛；当前窗口化方案已限制挂载的 DOM 数量。 |
| FR-CLI-102 Markdown/高亮/diff/Mermaid | ◐ | W: `chat/Fenced.tsx`（围栏代码块）、`cards/DiffBlock.tsx`（逐行着色 diff）、`core/src/text.ts`（`splitFences`/`diffLines`） | `core/src/v04.test.ts` | 未做 GFM 全量渲染、语法级高亮和 Mermaid 图：这些依赖较大的第三方渲染库，而且会扩大 Bot 文本的 XSS 面。富展示统一走 OpenUI 白名单（7.1.5），Mermaid 源码按代码块显示。 |
| FR-CLI-103 @ 联想 | ✅ | W: `core/src/mentions.ts`、Composer | `core/src/core.test.ts`、E2E | 旧有实现 |
| FR-CLI-104 行内任务卡 | ✅ | W: `CardView.tsx` TaskLine；`dock/TasksTab.tsx` 详情与终端日志 | E2E | 旧有实现 |
| FR-CLI-105 交付物入口 | ✅ | 合并卡展示 PR 号、分支、提交、diffstat，并可展开完整 diff（`DiffToggle`，来自 `DaemonTaskCompleted.Diff`，上限 60 KB） | E2E、`worktree/gc_test.go` TestDiffAndGC | |
| FR-CLI-201 抽屉可收起并持久化 | ✅ | W: `core/src/store.ts`（prefs） | E2E | 旧有实现 |
| FR-CLI-202 成员与 Agent 网络 | ✅ | W: `dock/PeopleTab.tsx`：在线状态、准入摘要；新增自主级别徽标与策略编辑 | E2E V0.4 场景 | |
| FR-CLI-203 任务与 PR | ✅ | 合并卡增加测试结果徽标（`tests`）、审批规则、类别与 diff 预览 | IT TestApprovalAllRuleAndGitHubWebhook、E2E | |
| FR-CLI-204 MEMORY.md 预览与引用 | ✅ | W: `dock/MemoryTab.tsx`：版本、可信级别、编辑、污染标记；S: `GET /api/memory/{id}/references` | IT TestProjectAccessTaintTokensIM、E2E V0.4 | |
| FR-CLI-301 审批卡内容 | ✅ | 合并卡与权限卡展示目标、具名审批人、规则（ANY_ONE / ALL 及已批进度）和来源。来源包括 Bot、owner、可信级别 `untrusted`，权限卡另有拦截层级。 | IT TestApprovalAllRuleAndGitHubWebhook、E2E | |
| FR-CLI-302 名单内显示批准/驳回，否则只显示催办 | ✅ | W: `NudgeButton`；S: `POST /api/tasks/{id}/nudge`（同一用户对同一任务 10 分钟内限一次），由服务端校验 | IT TestTimeoutsPauseAndNeverApprove、E2E 拒绝路径（王五催办） | |
| FR-CLI-303 驳回必须填写理由 | ✅ | S: 接单拒绝、权限拒绝、合并驳回、记忆驳回、污染标记缺少理由时，均返回 `common.reason_required`；W: `RejectWithReason` | IT（多处）、E2E 拒绝路径 | |
| FR-CLI-304 决定后只读 | ✅ | 卡片状态由服务端回写 | E2E | 旧有实现 |
| FR-CLI-305 审批人侧合并 | ✅ | 审批人自己的 daemon 执行 `VerifiedMerge` 或 `gh pr merge --match-head-commit` | E2E、`github.spec.ts` | 旧有实现 |
| FR-CLI-401 本地缓存 | ✅ | W: `core/src/cache.ts`：按工作区把房间、消息（最新 200 条）、任务、Agent、成员、项目快照到 IndexedDB，启动时回填并标记为陈旧 | `core/src/v04.test.ts` snapshot cache | |
| FR-CLI-402 断线提示与退避重连 | ✅ | W: `core/src/realtime/client.ts` | E2E 服务器宕机 | 旧有实现 |
| 7.1.5 Bot 富消息 | ✅ | S: `service/richmsg.go`：白名单校验（禁止 HTML/iframe/外链），来源为 `_meta.collabos.ui` 或 ```openui 围栏，并保留纯文本降级。交互接口 `POST /api/messages/{id}/actions` 以点击者身份鉴权，回流内容按 `user_input` 处理，同一用户同一动作只记一次，**从不执行平台操作**。W: `views/src/rich/library.tsx` 基于 `@openuidev/react-lang`（MIT），只注册同一组白名单组件，并标注来源 Bot 与 owner。 | `v04_unit_test.go` TestValidateRichWhitelist/TestExtractRich；IT TestSubDelegationAutonomyAndPatrol；`views/src/rich/render.test.tsx`；`rich.test.ts`（前后端白名单必须一致） | 没有外链图片组件，因此不需要图片代理 |

## 7.2 daemon

| 条目 | 状态 | 实现 | 测试 | 说明 |
|---|---|---|---|---|
| FR-DAE-101 单二进制、多平台 | ◐ | `cmd/collab`，交叉编译由 CI 覆盖 | CI build | macOS 与 Windows 只做了编译验证，未在真机运行 |
| FR-DAE-102 分发形态与单实例 | ◐ | 单实例：`daemon/agent/lock_unix.go` 通过 flock 实现（Windows 用独占打开） | `daemon/agent/lock_test.go` TestSingleInstanceLock | 不做 Tauri sidecar：本环境无法构建和签名桌面安装包。独立安装形态即 `collab` 二进制本身。 |
| FR-DAE-103 单任务 CPU/内存上限 | ✅ | `runner.limitArgv` 用 prlimit 限制 CPU 秒数、地址空间和进程数，配置项为 `COLLAB_TASK_CPU_SECONDS` / `COLLAB_TASK_MEM_MB` / `COLLAB_TASK_MAX_PROCS` | `runner/v04_test.go` TestLimitArgv | 系统没有 prlimit 时不限制，并在能力清单中如实上报 |
| FR-DAE-201/202 环境探测与能力上报 | ✅ | `agent.go` 探测 git、opencode、node/npm/pnpm/go/python3/cargo/java 和 prlimit | E2E（设备卡显示能力） | |
| FR-DAE-301 每个委派一个 worktree | ✅ | `worktree.Create` | `git_test.go` | 旧有实现 |
| FR-DAE-302 独立身份、测试、trailer、本人凭据开 PR | ✅ | 提交 trailer 包括 `CollabOS-Task`、`Delegation-Id` 和 `Trace-Id`。项目设置了测试命令时，`runTests` 会在同一沙箱内离线运行它，结果写入 PR 正文和合并卡。 | `runner/v04_test.go`、IT | |
| FR-DAE-303 回收与定期 GC | ✅ | 每个任务结束后执行 `worktree.Remove`；daemon 启动时运行 `worktree.GC`，跳过日志中仍未完成的任务 | `gc_test.go` TestDiffAndGC | |
| 7.2.4 两层 PEP | ◐ | 第一层：ACP fs/terminal 的路径规范化与 grant 判定。第二层：Linux 用 bwrap 并通过 `--unshare-net` 和代理白名单控制出站；macOS 用 Seatbelt。另新增「监督」自主级别，非只读调用一律转为权限请求（层级 `autonomy`）。 | `sandbox_test.go`、`egress_test.go`、`netbridge_test.go`、`acpclient` 测试 | 未用 Landlock/seccomp，改用 bwrap（同样基于命名空间）。Windows 只支持本人委派，并如实标注。 |
| FR-DAE-403 拦截即时上报 | ✅ | 旧有实现 | E2E | |
| FR-DAE-501/502 只走 ACP | ✅ | 旧有实现 | `acpclient` 测试、E2E | |
| FR-DAE-503 用量取不到时标注未知 | ✅ | S: 没有用量的任务记一条 `known=false` 的账，不做估算 | IT（Usage 合计） | |
| FR-DAE-601/603 日志与补报 | ✅ | 旧有实现 | `journal_test.go`、E2E 服务器宕机 | |
| FR-DAE-602 断网时继续执行，审批步骤暂停 | ✅ | 旧有实现。新增：决策超时、预算用尽或监督模式下，通过 `TASK_PAUSE` 用 SIGSTOP 暂停整个进程组，决定后用 `TASK_RESUME` 恢复。 | `runner/v04_test.go` TestControlPausesProcessGroup（真实进程）、IT | |

## 8. 服务端

| 条目 | 状态 | 实现 | 测试 | 说明 |
|---|---|---|---|---|
| FR-SRV-101 短期访问令牌 + 刷新；设备令牌可吊销 | ✅ | `service/tokens.go`：访问令牌默认 12 小时，刷新令牌 30 天并轮换，旧刷新令牌一次失效，另有 `logout`。个人 API 令牌（`coa_`）可吊销。设备吊销接口为 `DELETE /api/daemons/{id}`（仅 owner），吊销后立即断开连接。W: 遇 401 时自动刷新一次，并发请求共享同一次刷新。 | IT TestProjectAccessTaintTokensIM；`core/src/v04.test.ts` token refresh | |
| FR-SRV-102 心跳与离线宽限 | ✅ | 读超时为 3 个心跳周期加 5 秒，超时判定离线；任务进入 `running_offline`，宽限期结束后转为 `unknown` | E2E 守护进程崩溃 | 旧有实现 |
| FR-SRV-103 多实例中继 | ✗ | — | — | PRD 将其列为阶段 2+ 的后续项，MVP 采用单实例内存 hub |
| FR-SRV-201/202 消息与幂等 | ✅ | 旧有实现 | IT | |
| FR-SRV-203 上下文打包与可信级别 | ✅ | 每段上下文都带 `source` 和 `trust`，引用关系写入 `context_refs`；被污染的版本不再下发 | IT TestProjectAccessTaintTokensIM | |
| FR-SRV-301 统一状态机 | ✅ | `taskstate` | `taskstate_test.go` | |
| FR-SRV-302 路由、准入、离线排队 | ✅ | 目标离线时任务进入 `queued`，原因为 `host_offline`。Chief 运行也会排队，并提示「宿主离线，已排队」。 | E2E | |
| FR-SRV-303 Git webhook 校准 | ✅ | `service/webhook.go`：`POST /api/webhooks/github/{project}` 用 HMAC-SHA256 验签，按 delivery 去重。`merged` 时以平台为准完成任务；`closed` 时取消任务；`synchronize` 时头提交变化，批准作废。 | IT TestApprovalAllRuleAndGitHubWebhook、TestGitHubSignature | 真实 GitHub 投递需要公网 URL，本环境没有，因此用签名一致的合成负载测试。GitLab 未实现，因为它的验签方式不同、而本仓库只接入了 GitHub。 |
| FR-SRV-401 不可逆操作统一审批 | ✅ | 合并改动 `migrations/` 或 `.sql` 时，归入 `db_migration` 类别，且多审批人时强制 ALL 规则 | `TestApprovalCategory`、IT | |
| FR-SRV-402 审批人身份校验；不可信来源必须审批 | ✅ | 由服务端校验名单。Agent 提出的子委派在默认的「标准」级别下只生成交给其 owner 的草稿；富消息交互不执行任何操作。 | IT TestSubDelegationAutonomyAndPatrol | |
| FR-SRV-403 只追加审计 | ✅ | 新增的审计动作包括 merge.approve_partial、budget.*、memory.taint、token 与 device 吊销等 | IT、E2E 审计 | |
| FR-SRV-404 不合并、不存 Git 凭据 | ✅ | 不变 | 代码审查 | |
| FR-SRV-501 交接摘要（待确认） | ✅ | `memory.handover` 生成结构化条目（结论、执行 Bot、改动、测试、PR、trace），可信级别为 `auto_generated`，状态为待确认 | E2E 主流程 | |
| FR-SRV-502 MEMORY.md 版本化，变更需负责人确认 | ✅ | `POST /api/projects/{id}/memory`：负责人直接确认，其他人提交待确认；污染标记和引用追溯 | IT、E2E V0.4 | |
| FR-SRV-601 三级预算，90% 警告、100% 暂停 | ✅ | `service/budget.go` 支持委派、用户（付费方）、团队三级预算。用到 90% 时发 `budget_warning` 通知；用到 100% 时进入 `approval_required/budget_exceeded`，daemon 暂停执行，并发出预算卡。委派方或付费方可以扩容。团队或个人预算用尽时，新任务进入排队，调高预算后自动恢复。 | IT TestTraceDependenciesConcurrencyBudgets | |
| FR-SRV-602 trace_id 贯穿 | ✅ | trace_id 依次经过：委派（继承或新建）、帧信封 `trace_id`、daemon 上行帧（`MsgTrace`）、提交 trailer、PR 正文、审批卡和审计。按 trace 查询用 `GET /api/traces/{id}`。 | IT（trace 视图）、`runner/v04_test.go`、E2E V0.4 | |

## 6. 业务规则

| 条目 | 状态 | 实现 / 测试 | 说明 |
|---|---|---|---|
| 6.1 `goal.acceptance` | ✅ | 写入任务和派发帧，作为提示词中的「验收标准」 | |
| 6.1 `billing` | ◐ | 接单时选择 `model_key: source/target`，账记在付费方名下 | 平台**只记账**。免费模型不需要 Key，所以没有实现 Key 注入（PRD 6.4 也规定 Key 不得上云）。 |
| 6.1 `approval_policy.rule` ANY_ONE / ALL | ✅ | IT TestApprovalAllRuleAndGitHubWebhook | |
| 6.1 `grant.expires_at` | ✅ | 派发前由 `gate` 检查，过期取消（`grant_expired`） | |
| 6.1 多级委派与依赖 | ✅ | `depends_on`：依赖完成后自动派发，依赖失败则取消（`dependency_failed`）。子委派的 grant 必须是父委派的子集，并受 `max_depth` 约束。 | IT TestTraceDependenciesConcurrencyBudgets、TestSubDelegationAutonomyAndPatrol |
| 6.2 状态机 | ✅ | 已有 | |
| 6.3 原因码 | ✅ | 新增 waiting_dependencies、dependency_failed、concurrency_limit、awaiting_merge、decision_timeout、intake_timeout、grant_expired、host_offline，均有界面文案 | |
| 6.4 准入约束（最大 grant、禁网、最长时长、同时执行数、每日成本） | ✅ | `PATCH /api/agents/{id}/policy`；`CreateDelegation` 会应用这些约束 | IT（network 拒绝、并发排队） |
| 6.4 接单超时 | ✅ | 由 `RunSweeper` 处理，超时自动拒绝（`intake_timeout`） | IT TestTimeoutsPauseAndNeverApprove |
| 自主等级与超时接管 | ✅ | 三个级别：supervised / standard / autonomous。审批超时后**任务保持暂停**，并提醒和通知，**从不自动批准**。 | IT TestTimeoutsPauseAndNeverApprove、E2E V0.4 |
| 6.6 审批类别、头提交校验、结果校准 | ✅ | 见 FR-SRV-303/401 | |

## 5. 场景

| 场景 | 状态 | 测试 |
|---|---|---|
| 5.0 MVP 主流程 | ✅ | E2E 跨所有者委托；GitHub 版见 `github.spec.ts` |
| 5.1 越权与拦截 | ✅ | E2E |
| 5.2 跨所有者委派 | ✅ | E2E |
| 5.3 进入共享项目（项目权限） | ✅ | `service/access.go` 定义角色 lead/writer/viewer 和 workspace/restricted 两种可见性。非成员看不到受限项目的房间；viewer 只能读。见 IT 和 E2E V0.4。 |
| 5.4 Agent 主动推进（巡检） | ✅ | `POST /api/agents/{id}/patrol` 只读运行，生成 `agent_post` 富卡，负责人可「转为任务」（生成草稿）。见 IT TestSubDelegationAutonomyAndPatrol。 |
| 5.5 离线与恢复 | ✅ | E2E 服务器宕机、守护进程崩溃 |

## 11. 安全

| 条目 | 状态 | 实现 / 测试 |
|---|---|---|
| 11.3 检查维度（含预算） | ✅ | 依次经过 `gate`（grant 时效 → 依赖 → 并发 → 预算）、准入和项目访问控制 |
| 11.4 来源标记 | ✅ | 消息、记忆、上下文和卡片都带 `trust` / `source` |
| 11.4 不可越权触发 | ✅ | 富消息交互不执行平台操作；Agent 发起的子委派在「标准」级别只生成交给 owner 的草稿；预算暂停期间不会被权限决定恢复执行 |
| 11.4 指令与数据分离 | ✅ | 提示词中，上下文段落带 `source` 和 `trust` 属性，并有明确边界 |
| 11.4 知识入库隔离 | ✅ | 外部 Bot 结论标为 `external_bot` 并进入待确认，`LatestConfirmedMemory` 排除污染版本 |
| 11.4 可追溯与回滚 | ✅ | `POST /api/memory/{id}/taint` 标记该版本及之后所有版本，同时标记引用过它们的任务。IT 与 E2E 均覆盖。 |

## 14. 路线图中的阶段 2/3 项

| 条目 | 状态 | 说明 |
|---|---|---|
| 外部 IM（飞书、Slack、钉钉） | ◐ | **出站通知**已实现：`PATCH /api/workspace/im`（仅 owner 可配置，主机须在白名单内，须为 https，本机 http 仅用于测试）；在接单、审批、预算、催办、超时等事件时异步推送，负载中不含任何密钥（IT TestProjectAccessTaintTokensIM 使用本地 webhook 接收）。**入站**（在 IM 里直接审批）未实现：需要公网回调地址和各平台的应用注册，本环境两者都没有，而且注册应用属于外部账号操作。 |
| 公开 API / SDK | ✅ | 所有路由同时挂在 `/api/v1`；接口规格见 `GET /api/openapi.yaml`（`TestOpenAPICoversRoutes` 保证每条路由都已文档化）；SDK 为 `createCollabClient`（`@collabos/core`）。说明见 [api.md](./api.md)。 |
| 追踪可视化 | ◐ | 任务详情中提供按 trace 展开的委派链（层级缩进）和 PR；火焰图属阶段 2+，未做 |
| 多实例中继、知识图谱、平台内结算、企业审计 | ✗ | 属阶段 2+/3 或开放问题，不在本轮范围 |
| Tauri 桌面端 | ✗ | 需要桌面签名与打包环境，本环境无法验证 |
