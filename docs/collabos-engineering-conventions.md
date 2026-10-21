# CollabOS 工程规范 v0.2（参照 Multica 改编）

> v0.2 变更：第 9 节 ADR 全部定案（shadcn+Radix、golang-migrate、tygo、TanStack Router、daemon 双形态分发）；移除旧风格参考稿引用（参考对象统一为 Multica）；新增 2.5 设计 token 硬性规则；视觉风格暂定为 Grok Bot 式占位主题。

> 标记：**【借鉴】** 直接沿用 Multica 的做法（用自己的话重述）；**【改编】** 思路来自 Multica，但按 CollabOS 技术栈改写；**【新增】** CollabOS 独有。
> 依据：multica-ai/multica 主干（2026-10-21 浅克隆，/workspace/research/multica），各条后注明出处文件。
> 版权说明：Multica 使用 Multica License（Apache-2.0 + 附加条件，限制 UI 代码复用与托管）。**本仓库不得复制 Multica 的任何源码**，尤其是 apps/、packages/ui、packages/views 下的代码；本文只借鉴工程方法，文中片段均为新写的示意代码。

---

## 0. Multica 实际约定速查（调研证据）

| 主题 | Multica 的做法 | 出处 |
| --- | --- | --- |
| 智能体指令 | 根目录 AGENTS.md 是唯一规则源，CLAUDE.md 只有一行 `@AGENTS.md`；子平台另有嵌套 AGENTS.md | CLAUDE.md、AGENTS.md §Scope、apps/mobile/AGENTS.md |
| Monorepo | pnpm workspace + turbo；依赖版本统一走 `catalog:`；共享包导出原始 TS，由 app 编译；turbo 把 ci.yml 计入 globalDependencies | pnpm-workspace.yaml、turbo.json、AGENTS.md §Sharing |
| 包边界 | `core`（无 UI，含 API client、Query hooks、Zustand store）、`ui`（原语 + 样式，不导入 core）、`views`（业务页面，不定义 store，不依赖路由库）；依赖方向 views → core + ui | AGENTS.md §Sharing、packages/{core,ui,views} |
| 平台适配 | 导航通过 `NavigationAdapter`，存储通过 `StorageAdapter`，框架 API 只放在 app 层 | AGENTS.md §Sharing |
| 状态 | 服务端数据归 TanStack Query；客户端状态归 Zustand；WS 事件用来 patch 或 invalidate Query 缓存，不写进 store；query key 带 wsId；乐观更新有严格条件；创建/删除要等服务端确认 | AGENTS.md §State Rules |
| API 兼容 | 前端用 zod schema + `parseWithFallback` 解析响应，禁止 `as T`；未知枚举有兜底；旧桌面客户端可能连新后端 | AGENTS.md §API Compatibility、packages/core/api/schema.ts |
| UI | shadcn/Base UI 原语，用 `pnpm ui:add` 添加；语义 token 和 `--text-*` 字号刻度定义在 packages/ui/styles/tokens.css | AGENTS.md §Web/Desktop UI、packages/ui/components.json、packages/ui/components/ui/ |
| i18n | i18next + react-i18next；locales 含 en/zh-Hans/ja/ko/fr；有自定义 eslint 规则禁止硬编码文案，有 parity 测试保证各语言 key 一致 | packages/views/locales/、eslint-i18n-guard.mjs、locales/parity.test.ts |
| Go 布局 | `cmd/{server,migrate,multica(CLI+daemon),maintenance,backfill_*}`；`internal/` 按领域分包（handler、middleware、realtime、daemon、daemonws、events、scheduler、logger 等）；`pkg/` 放可复用库（agent 适配器、db、protocol、redact 等） | server/cmd、server/internal、server/pkg |
| 路由 | Chi；路由集中在 cmd/server/router.go；中间件顺序：RequestID → ClientMetadata → RequestLogger → metrics | server/cmd/server/router.go:1406-1413 |
| 错误 | handler 内统一用 `writeJSON`/`writeError`，错误体为 `{"error":msg,"code":code}`，code 稳定、供前端翻译；功能被禁用时返回 403，不返回 503 | server/internal/handler/handler.go:544-600 |
| 日志 | 标准库 `log/slog`，`logger.NewLogger(component)` 按组件命名 | server/internal/logger/logger.go |
| 数据库 | PostgreSQL + pgx/v5 + sqlc（查询在 pkg/db/queries，生成到 pkg/db/generated）；自研迁移 runner（cmd/migrate，advisory lock）；编号 up/down SQL 文件（已超 600 对） | server/sqlc.yaml、server/migrations/、server/cmd/migrate/main.go |
| 迁移规则 | **不建外键、不做级联**，由应用层在事务中维护关系；索引一律 `CREATE INDEX CONCURRENTLY`，每个索引单独一个迁移文件；条件 DDL 必须幂等 | AGENTS.md §Database |
| 多租户 | 查询按 workspace_id 过滤；请求头 `X-Workspace-ID` 选择工作区；membership 控制访问；assignee 是多态的（`assignee_type` + `assignee_id`） | AGENTS.md §Backend UUID |
| UUID | handler 内区分三种来源：资源参数走 loader 解析，纯 UUID 输入用 `parseUUIDOrBadRequest`，可信数据往返用 `parseUUID` | AGENTS.md §Backend UUID |
| 实时 | `internal/realtime`（hub、membership、Redis relay、分片流）面向浏览器；`internal/daemonws`（hub + RPC completion）面向守护进程；daemon 侧对应 wsrpc.go | server/internal/realtime/、daemonws/、daemon/wsrpc.go |
| Daemon | `internal/daemon`：config、client、reconcile、gc、health、auto_update、prompt、repocache（仓库缓存）、execenv（执行环境）、processtree、terminal_report_queue；智能体适配器在 `pkg/agent`，每个 CLI 一个文件 | server/internal/daemon/、server/pkg/agent/ |
| 测试 | 测试与实现放在一起；每个行为只在一个测试层覆盖；修 bug 先写失败的回归测试；Go DB 测试用 testutil fixtures；默认测试不调用真实 CLI，真实智能体冒烟测试用 build tag `agentintegration` + 环境变量开关 | AGENTS.md §Testing、server/internal/testutil/ |
| 本地环境 | `make up/status/down/destroy`；每个 git worktree 共享 PG 实例、但用独立数据库和端口（`.env.worktree`） | AGENTS.md §Development、CONTRIBUTING.md |
| 校验命令 | `pnpm typecheck/lint/test`、`make test`、`make check`、Playwright E2E；SQL 改动后跑 `make sqlc` | AGENTS.md §Development |
| CI | ci.yml 先按路径算出需要跑的作业（scripts/ci-scope.mjs），再跑：校验生成的 sqlc 是否最新、turbo build/typecheck/lint、分片的 views 测试；另有 release、desktop-smoke、ui-performance 等工作流 | .github/workflows/ci.yml、.github/ci-paths.json |
| 提交/PR | 原子化 Conventional Commits，常带内部工单号（如 `MUL-8060 fix(daemon): …`）；PR 模板要求写变更类型、测试步骤和「thinking path」 | AGENTS.md §Change、.github/PULL_REQUEST_TEMPLATE.md、近期 commit |
| 发布 | 只有推送 semver tag 才触发发布（工作流不提供手动触发）；先跑 Go 测试 + govulncheck（失败即阻断，仅允许按 tag 豁免）；默认 patch 版本；发版节奏每周 1–2 次 | .github/RELEASING.md、release.yml |
| 代码风格 | 代码注释用英文；不加内部兼容垫片或双写（API 响应兼容除外）；注释只解释不明显的原因，不写事件经过 | AGENTS.md §Change |

---

## 1. 仓库结构【改编】

```
collabos/
├── AGENTS.md                 # 唯一规则源【借鉴】
├── CLAUDE.md                 # 仅一行 @AGENTS.md【借鉴】
├── CONTRIBUTING.md
├── Makefile                  # up/down/destroy/check/sqlc/test【借鉴】
├── pnpm-workspace.yaml       # catalog: 统一版本【借鉴】
├── turbo.json
├── apps/
│   ├── web/                  # Vite + React SPA + TanStack Router（替代 Next.js）【改编】
│   └── desktop/              # Tauri 2（src-tauri/ 为 Rust 壳）【改编】
├── packages/
│   ├── core/                 # API client、zod schema、Query hooks、Zustand、i18n、realtime
│   ├── ui/                   # shadcn(Radix) 原语 + themes/（token 主题文件）
│   ├── views/                # 群聊、看板、任务、委派、Bot 管理等业务视图
│   └── richmsg/              # OpenUI 白名单组件库 + 渲染器封装【新增】
├── server/                   # 单一 Go module
│   ├── cmd/
│   │   ├── server/           # API + WS 服务
│   │   ├── migrate/          # 迁移 runner
│   │   └── collab/           # CLI + daemon 同一个二进制（collab daemon start）
│   ├── internal/
│   │   ├── handler/          # HTTP handler，按资源分文件
│   │   ├── middleware/
│   │   ├── service/          # 业务逻辑（handler 不直接拼 SQL）
│   │   ├── realtime/         # 浏览器/桌面端 WS hub
│   │   ├── daemonws/         # 守护进程 WS hub + RPC
│   │   ├── delegation/       # 委派协议状态机【新增】
│   │   ├── approval/         # 审批与权限决策【新增】
│   │   ├── events/           # 进程内事件总线
│   │   ├── config/ logger/ testutil/
│   │   └── daemon/           # daemon 主循环、reconcile、gc
│   │       ├── acpclient/    # ACP JSON-RPC 客户端【新增】
│   │       ├── sandbox/      # git worktree 沙箱【改编自 repocache/execenv】
│   │       └── journal/      # SQLite 本地日志/断点续传【新增】
│   ├── pkg/
│   │   ├── protocol/         # server↔daemon 消息类型（Go 与 TS 共用的规范来源）
│   │   ├── db/{queries,generated}/
│   │   └── redact/           # 日志脱敏
│   ├── migrations/           # NNNN_name.up.sql / .down.sql
│   └── sqlc.yaml
├── e2e/                      # Playwright
├── docs/                     # ADR、协议文档
└── .github/{workflows,PULL_REQUEST_TEMPLATE.md,RELEASING.md}
```

不做 mobile 和独立 docs 站，首期不需要。

---

## 2. 前端规范

### 2.1 包边界【借鉴 + 改编】
- **依赖方向**：`views → core + ui (+ richmsg)`；`core` 与 `ui` 互不依赖；`richmsg → ui`，`richmsg` 不依赖 core。
- **core**：不能出现 React DOM 组件、`localStorage`、`import.meta.env`。持久化注入 `StorageAdapter`；Tauri 下用 tauri-plugin-store 实现，Web 下用 localStorage 实现。
- **views**：不定义 store，不直接导入 `react-router` 或 `@tauri-apps/*`。导航用 `NavigationAdapter` + `<AppLink>`，平台能力（通知、文件、托盘）用 `PlatformAdapter` 注入。【改编：Electron 换成 Tauri】
- **共享包**导出原始 TS（不预编译），由 app 的 Vite 编译。
- **依赖版本**：各 workspace 声明自己直接使用的依赖，版本统一写在 `catalog:`。

### 2.2 状态与数据【借鉴】
- **服务端数据**：TanStack Query 管理。
- **客户端状态**：Zustand 管理（筛选、草稿、弹窗、面板布局）。只持久化偏好、草稿、布局。
- **query key**：工作区范围的数据必须带 `wsId`。
- **WS 事件**只做两件事：patch 或 invalidate Query 缓存，绝不把服务端数据写进 Zustand。
- **乐观更新**：仅限结果可预测、回滚简单的字段级修改，例如拖卡片换列。
- **必须等服务端确认**的操作：创建、删除、审批通过/拒绝、发起委派。
- **消息发送**：显示 pending 状态，失败可重试。
- **响应解析**：一律经 zod schema + `parseWithFallback`，禁止 `as T`；未知枚举映射为 `unknown` 兜底，桌面端可能比服务端旧。

```ts
// packages/core/api/schema/task.ts —— 示意代码（新写）
export const TaskStatus = z.enum(["backlog","todo","doing","review","done"]).catch("backlog");
export const Task = z.object({
  id: z.string().uuid(),
  title: z.string(),
  status: TaskStatus,
  assignee: z.object({ type: z.enum(["member","bot"]).catch("member"), id: z.string() }).nullable().default(null),
});
```

### 2.3 UI 与样式【借鉴 + 改编】
- **原语**：使用 shadcn/ui（**Radix**，见 ADR-001），用 `pnpm ui:add` 放进 `packages/ui/components/ui/`。业务组合放在 views。
- **样式**：Tailwind v4 + 语义 token（见 2.5）。字号用按角色命名的 `--text-*`（如 `--text-body`、`--text-caption`），不直接用 `text-sm` 这类刻度。【改编】
- **视觉风格**：正式风格暂缓决定。当前占位主题为「Grok Bot 式」：干净、中性、现代的助手应用风格，浅色与深色两套；以后换风格只需替换主题文件。
- **文案规则**【借鉴】：默认不写描述文字，不复述标题或按钮。权限、成本、破坏性后果必须写出来。

### 2.4 i18n【借鉴】
- i18next；首期语言 `zh-Hans` + `en`。
- ESLint 规则禁止 views 中出现硬编码文案。
- parity 测试保证两种语言的 key 一致。

### 2.5 设计 token 硬性规则【新增】
- **颜色、圆角、间距、字体、阴影只能通过语义 token 使用。** token 是 CSS 变量，定义在 `packages/ui/themes/*.css`，例如 `--color-bg`、`--color-fg-muted`、`--color-accent`、`--color-danger`、`--radius-card`、`--space-3`、`--text-body`、`--shadow-popover`。
- Tailwind 主题（`@theme`）只映射到这些变量，组件里只写 `bg-surface`、`text-fg-muted`、`rounded-card` 这类类名。
- **禁止**：组件中出现裸 hex/rgb/hsl、Tailwind 任意值（`bg-[#..]`、`p-[13px]`、`rounded-[6px]`），以及默认调色板类（`bg-zinc-900`、`text-blue-500`）。
- **由 lint 强制**：ESLint 规则（自写或用 eslint-plugin-tailwindcss 加自定义规则）扫描 TSX；stylelint 禁止 themes/ 以外的 CSS 出现颜色字面量。CI 不通过即阻断。
- **主题即一组 token**：`themes/light.css`、`themes/dark.css` 各自定义全部变量，通过 `data-theme` 切换。以后换风格 = 新增或替换主题文件，组件零改动。新增 token 时必须在所有主题中补齐，由测试校验各主题的 key 一致。
- **OpenUI 白名单组件**同样只能使用 token，Bot 输出中不允许携带颜色或样式值。

```css
/* packages/ui/themes/light.css —— 示意（新写，占位值） */
[data-theme="light"] {
  --color-bg: oklch(0.99 0 0);
  --color-surface: oklch(0.97 0 0);
  --color-fg: oklch(0.21 0 0);
  --color-fg-muted: oklch(0.55 0 0);
  --color-accent: oklch(0.21 0 0);
  --radius-card: 12px;
  --shadow-popover: 0 8px 24px oklch(0 0 0 / 0.08);
}
```

### 2.6 Bot 富消息（OpenUI）【新增】
- **包**：`packages/richmsg` 封装 `@openuidev/react-lang`，用 `createLibrary` 注册**白名单组件**。组件全部用 packages/ui 实现，禁止引入 `@openuidev/react-ui` 的成品样式。
- **两类组件严格分开**：

| 类别 | 组件 | 来源 |
| --- | --- | --- |
| 平台卡片 | ApprovalCard、TaskCard、DelegationCard、DiffView | **只能由服务端结构化消息渲染**，不进 OpenUI 库，Bot 写同名组件时直接报错 |
| Bot 可生成 | Form、Table、Chart、Checklist、Choice、Code、Markdown、Callout | OpenUI Lang |

- **禁止**：HtmlView、iframe、任意 URL 图片（必须经服务端图片代理）、自动刷新的 Query（或者设下限 ≥60s 并加配额）。
- **承载方式**：ACP 消息的 `_meta.collabos.ui = { lang: "openui", version: "1.0", source }`，正文保留纯文本降级内容；服务端校验后存为 `message.rich`。
- **动作回流**：`onAction` 和 `toolProvider` 只能调用 `core/api` 里的 `richmsg.action` 端点，服务端以**点击者身份**鉴权，高风险操作转入审批。`@ToAssistant` 回流的内容标记为「用户输入」，进入注入防护流程。`@OpenUrl` 必须二次确认。
- **UI 角标**：每条富消息显示来源 Bot + owner。
- **组件库 prompt 生成**：`pnpm richmsg:prompt` 生成 prompt，产物提交到 `server/internal/daemon/prompts/`，由 daemon 注入 ACP 会话；CI 校验产物是否最新（同 sqlc 的做法）。

---

## 3. Go 服务端规范

### 3.1 布局与依赖【借鉴】
- **单一 Go module**：`server/`。`cmd/` 只做组装（读配置、建连接池、挂路由、优雅关停），业务放在 `internal/`。
- **`pkg/`** 只放无业务状态、可被 daemon 和 server 共用的库，例如 protocol、db、redact。
- **路由**集中在 `cmd/server/router.go`（Chi）。
- **中间件顺序**：RequestID → ClientMetadata（版本、平台）→ RequestLogger → Metrics → Auth → Workspace → handler。【借鉴】

### 3.2 Handler / Service【改编】
- **handler** 只负责解析、鉴权入口和序列化；业务和事务放 service。Multica 的 handler 较重，CollabOS 选择更明确地分层。
- **错误响应**统一 `{"error": "<英文兜底>", "code": "<稳定码>"}`，前端按 code 翻译。【借鉴】
- **service 层**返回带 code 的领域错误，由 handler 映射成 HTTP 状态：
  - 被禁用的功能返回 403 + code，不返回 503。【借鉴】
  - 委派拒绝原因码直接复用 PRD 6.11.7。【新增】

```go
// internal/apperr/apperr.go —— 示意代码（新写）
type Error struct{ Status int; Code, Msg string }
func (e *Error) Error() string { return e.Code + ": " + e.Msg }
var ErrGrantExceedsParent = &Error{403, "delegation.grant_exceeds_parent", "grant must be a subset of parent grant"}
```

- **UUID 区分三种来源**【借鉴】：
  - 资源参数：通过 loader 解析，并同时完成权限检查。
  - 纯 UUID 输入：`parseUUIDOrBadRequest`。
  - 可信内部数据：`mustUUID`。

### 3.3 数据库【借鉴】
- PostgreSQL 17 + pgx/v5 + sqlc（`emit_json_tags`、`emit_empty_slices`）。
- SQL 改动后必须跑 `make sqlc`；CI 校验生成代码已提交且是最新。
- **迁移**：`server/migrations/NNNN_name.{up,down}.sql`，使用 **golang-migrate**（ADR-002）；`cmd/migrate` 只是它的薄封装。Multica 用的是自研 runner + advisory lock。【改编】
- **不建外键、不做级联**，关系在 service 事务里维护。
- 索引用 `CONCURRENTLY`，每个索引单独一个文件。
- DDL 要幂等。
- **所有业务表**带 `workspace_id` 并按它过滤；请求头 `X-Workspace-ID`。
- **assignee 多态**：`assignee_type ∈ {member, bot}`。【借鉴】
- **跨 owner 委派**：委派记录同时带 `from_workspace_id`/`to_workspace_id`、`from_owner`/`to_owner`；任何读取必须满足两边之一的 membership 加 grant 检查。【新增】

### 3.4 日志、配置、context【借鉴 + 改编】
- **日志**：`log/slog`，每个组件一个命名 logger；令牌、ACP 内容、prompt 必须经 `pkg/redact` 脱敏后才能写日志。
- **配置**：只读环境变量，集中在 `internal/config`，启动时校验，缺失则快速失败；`.env.example` 列全所有变量。【改编：Multica 分散在各处，CollabOS 选择集中】
- **context**：每个阻塞调用都传 `ctx`；handler 用 `r.Context()`；后台任务从根 ctx 派生，在 shutdown 时取消。不把业务数据放进 context，只放 requestID、actor、workspace。
- **健康检查**：`GET /health` 返回 `pid`、`commit`、`started_at`，方便 `make up` 确认当前进程就是刚启动的那个。【借鉴，CONTRIBUTING.md】

### 3.5 实时通道【借鉴】
- **两个 hub 分开**：
  - `realtime`：客户端订阅，按 workspace/频道，权限变化时踢出订阅。
  - `daemonws`：守护进程连接，任务下发 + RPC，请求带 id，完成后回执。
- 单实例时用内存 hub；多实例时加 Redis relay（二期）。
- **事件契约**放在 `pkg/protocol`，用 **tygo**（ADR-003）生成 TS 类型到 `packages/core/realtime/protocol.gen.ts`；CI 校验生成文件是否最新。【新增】

---

## 4. Daemon 规范

| 模块 | 职责 | 标记 |
| --- | --- | --- |
| `daemon` 主循环 | 登录（PAT，有效期 90 天）、连接 daemonws、心跳、reconcile、自动更新（自托管默认关闭） | 【借鉴】daemon/{client,reconcile,health,auto_update}.go |
| `acpclient` | 统一的 ACP JSON-RPC 客户端：initialize、session/new·resume、prompt、cancel、权限请求；兼容 v1/v2 的差异集中在一处；`_meta` 用于扩展 | 【改编】Multica 每个 CLI 一个适配器，CollabOS 只走 ACP |
| `runtimes/` | 每种 Runtime 一个文件，只描述可执行文件发现、启动参数和能力；不写协议逻辑 | 【改编】pkg/agent |
| `sandbox` | 每个任务一个 git worktree（`<cache>/repos/<repo>.git` bare cache + `worktrees/<task-id>`）；worktree 用独立 git 身份配置；任务结束后 gc；路径白名单 | 【改编】repocache、execenv、CONTRIBUTING 中的 worktree 身份隔离 |
| `journal` | SQLite（modernc.org/sqlite，纯 Go，不用 CGO）记录本地事件：任务阶段、ACP 消息游标、未上报的终态。断线时先写 journal，重连后按序补发，实现幂等上报（用 delegation 的 idempotency key） | 【新增】对应 Multica 的 terminal_report_queue 思路 |
| `approval bridge` | ACP 的 `session/request_permission` 转发到服务端审批，等待结果；超时默认拒绝 | 【新增】 |
| `processtree` / gc | 杀进程要杀整棵进程树；清理磁盘占用 | 【借鉴】 |

- **默认测试**不得调用真实 CLI，用 fake ACP agent（testdata 里的小程序）。
- **真实冒烟测试**：`-tags=acpintegration` 且 `COLLAB_RUN_REAL_AGENT_SMOKE=1` 才运行。【借鉴】

---

## 5. 桌面端（Tauri）【改编】
- `apps/desktop` = Vite 前端（复用 views）+ `src-tauri`（Rust）。
- Rust 侧只做窗口、托盘、通知、深链、安全存储（keyring）、启停本机 daemon。
- **daemon 双形态分发**（ADR-005）：随 Tauri 打包为 sidecar；同时提供独立安装（Homebrew / 安装脚本 / 系统服务）供无界面主机使用。两种形态是同一个二进制、同一套配置目录；检测到已有独立 daemon 在运行时，桌面端不再拉起 sidecar，以免同机重复。**业务逻辑不写在 Rust。**
- **Tauri capabilities** 最小化：前端只开放必要的 command；禁止 shell 插件的任意执行。
- **自定义标题栏**：拖拽区与交互控件分开（对应 Multica 的 DragStrip 约定）。
- **兼容性**：已安装的旧桌面端会连新服务端，所以 API 响应必须向后兼容（见 2.2）。

---

## 6. 测试【借鉴】
- 测试与实现放在一起：core 测逻辑，views 测组件接线与可访问性，apps 测平台接线，e2e 测端到端流程。
- 每个行为只在一个层测试；修 bug 先写失败的回归测试。
- 不依赖 DOM 的测试文件首行写 `// @vitest-environment node`。
- views 测试不 mock 路由库；store 按 Zustand 的形状 mock；API mock 在 `@collabos/core/api` 这一层。
- **Go DB 测试**：`internal/testutil` 提供 fixtures 和 `Call(h, req).Want(200).JSON(&out)` 风格的辅助函数（自己实现）。断言写在测试里，不藏在 fixture 中。
- **新增**：
  - 委派状态机用表驱动测试覆盖 PRD 6.11.4 的全部转移。
  - richmsg 做「恶意 Bot 输出」fixture 测试：伪造 ApprovalCard、HtmlView、超深嵌套、外链图片。

---

## 7. 本地开发与校验【借鉴】
- **环境**：`make up / status / down / destroy`（down 保留数据，destroy 删除数据）；每个 worktree 用独立数据库和端口（`.env.worktree`）。
- **校验命令**：

| 范围 | 命令 |
| --- | --- |
| 前端 | `pnpm typecheck && pnpm lint && pnpm test` |
| Go | `make test`（含 `go vet`、`golangci-lint`） |
| 全量 | `make check` |
| E2E | `pnpm exec playwright test` |
| 桌面端 | `pnpm --filter desktop tauri build --debug`（**在用户自己的机器上验证构建**） |

- **汇报**：提交时说明实际跑了哪些检查、跳过了哪些。

---

## 8. CI / 提交 / 发布

- **CI**【借鉴】：
  - 先按路径过滤要跑的作业。
  - Go：test + vet + lint + 「生成代码是否最新」（sqlc、protocol TS、richmsg prompt）。
  - 前端：`turbo build typecheck lint test`（turbo 缓存把 ci.yml 计入 hash）。
  - E2E 冒烟。
  - desktop-smoke（Tauri 三平台构建）。
- **提交**【借鉴】：原子化 Conventional Commits，`type(scope): 描述`，scope 取包名或领域（daemon、acp、delegation、richmsg、views…）。可选工单号前缀 `COS-123`。
- **分支**【新增】：`feat/…`、`fix/…`，PR squash 合并到 main。
- **PR 模板**【借鉴】：做了什么、关联 issue、变更类型、实际跑过的测试、思考路径，以及安全影响（新增项：是否涉及权限、委派、Bot 生成内容）。
- **发布**【借鉴】：
  - 只能从 main 上已评审的 commit 推送 `vX.Y.Z` tag 触发；工作流不提供手动触发。
  - 发布前跑 Go 测试 + govulncheck，失败即阻断，只允许按 tag 临时豁免并留记录。
  - 默认 patch 版本。
  - **一次 tag 同时发布**：server 镜像、daemon/CLI 二进制、Tauri 安装包。
  - 首期 `0.x`。
- **代码风格**【借鉴】：
  - 注释用英文，只写不明显的原因。
  - 不加内部兼容垫片或双写（API 响应兼容除外）。
  - 规则变化与代码变化同一个 PR 更新 AGENTS.md。

---

## 9. 已定 ADR（2026-10-21 定案）

| 编号 | 决策 | 备注 |
| --- | --- | --- |
| ADR-001 | shadcn/ui 基于 **Radix** | 组件放 packages/ui/components/ui，用 `pnpm ui:add` 添加 |
| ADR-002 | 数据库迁移用 **golang-migrate** | 编号 up/down 文件；CONCURRENTLY 索引一文件一语句，在事务外执行 |
| ADR-003 | Go→TS 类型生成用 **tygo** | 来源是 server/pkg/protocol；产物提交入库，CI 校验是否最新 |
| ADR-004 | Web 路由用 **TanStack Router** | 只在 apps/ 使用；views 通过 NavigationAdapter 访问 |
| ADR-005 | daemon **同时**以 Tauri sidecar 和独立安装两种形态分发 | 同一二进制，单机只运行一个实例（锁文件 + 本地端口探测） |
| ADR-006 | 视觉风格暂缓，先用 Grok Bot 式中性主题占位；一切样式走 token（2.5） | 换风格 = 换主题文件 |
