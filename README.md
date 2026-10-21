# CollabOS

团队群聊里的多 Agent 协作：每位成员在自己的电脑上运行 `collab` 守护进程，托管自己的 Bot（Chief / 专家）。
在群聊里 `@` 别人的 Bot 即发起**跨所有者委托**：对方所有者接单 → Bot 在对方设备的 OS 沙箱、独立 git worktree 中执行
→ 越权操作弹出授权卡 → 推送分支并开 PR → **具名审批人**用自己的设备与自己的 Git 凭据合并 → 结果写入待确认的 MEMORY.md。

服务器只做协调：**不持有 Git 凭据，也不执行合并**。所有决定写入只追加的审计日志。

## 仓库结构

| 路径 | 内容 |
| --- | --- |
| `server/` | Go：`cmd/server`（API + WebSocket）、`cmd/collab`（守护进程 CLI）、`cmd/migrate`；`migrations/`；`pkg/protocol`（线协议，tygo 生成 TS） |
| `apps/web` | Vite + React + TanStack Router 的 Web 应用 |
| `packages/core` | API 客户端、zod 模式、TanStack Query hooks、实时缓存同步、zustand 客户端状态 |
| `packages/views` | 平台无关的界面（不依赖路由/zustand，通过 NavigationAdapter 注入） |
| `packages/ui` | 仅用设计令牌的组件库（浅色/深色主题） |
| `tooling/eslint` | 自定义规则：`no-raw-style`、`no-literal-text` |
| `e2e/` | Playwright 端到端测试（真实浏览器、真实守护进程、真实 opencode） |
| `docs/` | PRD V0.4、工程约定、截图 `docs/screens/`、`docs/verification.md` |

## 前置条件

- Go ≥ 1.27，Node ≥ 20.19，pnpm 10（`corepack enable`）
- PostgreSQL ≥ 15（本地开发默认 `postgres://collab:collab@localhost:5432/collab`）
- 运行 Agent 的设备：`git`、[opencode](https://opencode.ai)（`opencode acp`，可用免费模型）、Linux 上的 `bwrap`（bubblewrap）

```bash
# 例：Debian/Ubuntu
sudo apt-get install -y postgresql bubblewrap git
sudo -u postgres psql -c "CREATE ROLE collab LOGIN PASSWORD 'collab' CREATEDB" -c "CREATE DATABASE collab OWNER collab"
curl -fsSL https://opencode.ai/install | bash
```

## 快速开始

```bash
pnpm install
cd server && go build -o bin/server ./cmd/server && go build -o bin/collab ./cmd/collab && go build -o bin/migrate ./cmd/migrate && cd ..
export DATABASE_URL='postgres://collab:collab@localhost:5432/collab?sslmode=disable'
server/bin/migrate up

# 开发：后端 :8080 + Vite :5173（代理 /api、/ws）
server/bin/server &
pnpm dev            # 打开 http://localhost:5173
```

生产方式（单端口）：`pnpm build` 后 `COLLAB_WEB_DIR=apps/web/dist COLLAB_ALLOWED_ORIGIN=http://localhost:8080 server/bin/server`。

### 连接一台设备

1. 在 Web 里登录（开发登录：邮箱即身份），创建项目（填写 Git 远端，如 `/srv/git/calc.git` 或 GitHub 地址）。
2. 右侧「成员 / 设备」→「配对设备」，填写设备名与 Bot 用户名，得到一次性节点令牌。
3. 在那台设备上：

```bash
collab login --server http://localhost:8080 --token cos_n_...
collab repo add ~/code/calc          # 本地克隆，远端需与项目一致
collab run
```

同一台机器跑多个守护进程时，为每个设置不同的 `COLLAB_HOME`。

### 一键本地栈 / 预览

```bash
scripts/stack.sh /tmp/collab-stack 8092 collab_stack   # 新库 + 示例仓库 + 生产构建
scripts/daemon.sh /tmp/collab-stack zs http://localhost:8092 cos_n_... /tmp/collab-stack/repo-a /tmp/collab-stack/remote.git
./start-preview.sh                                     # 公网预览（见 PREVIEW.md）
```

## 检查

```bash
cd server && go vet ./... && go test ./...      # 需要 DATABASE_URL 的测试会自动跳过
pnpm typecheck && pnpm lint && pnpm test && pnpm build
pnpm e2e                                         # 需要 Postgres、opencode、bwrap、Chrome（CHROME_PATH）
```

## 配置

| 变量 | 默认 | 说明 |
| --- | --- | --- |
| `DATABASE_URL` | — | Postgres 连接串 |
| `COLLAB_ADDR` | `:8080` | 监听地址 |
| `COLLAB_AUTO_MIGRATE` | `false` | 启动时执行迁移 |
| `COLLAB_WEB_DIR` | — | 提供 SPA 静态文件 |
| `COLLAB_ALLOWED_ORIGIN` | `http://localhost:5173` | CORS / WebSocket Origin |
| `COLLAB_DEV_LOGIN` | `true` | 开发登录（无密码，邮箱即身份） |
| `COLLAB_DEV_LOGIN_PASSWORD` | — | 设置后登录需提供此访问口令（公网预览必须设置） |
| `COLLAB_DEV_LOGIN_EXISTING_ONLY` | `false` | 只允许已有账号登录（禁止注册/改名/加入工作区） |
| `COLLAB_OFFLINE_GRACE` | `10m` | 设备离线多久后任务置为 unknown |
| `COLLAB_DEFAULT_MODEL` | `opencode/deepseek-v4.1-flash-free` | opencode 模型 |
| `COLLAB_HOME`（设备） | `~/.collabos` | 守护进程配置与日志目录 |
| `COLLAB_SANDBOX`（设备） | 自动 | `none` / `bwrap` / `seatbelt` 强制选择 |

## 安全模型（摘要）

- 第一层（守护进程策略）：授权 glob + 动作；工作树之外/黑名单命令直接拒绝；越权升级为人工授权卡；运行结束后回滚授权外的改动。
- 第二层（OS 沙箱）：Linux bwrap（根只读、$HOME/仓库 tmpfs 遮蔽、仅 worktree 可写、pid/ipc/uts 隔离）；macOS Seatbelt。
  **网络未隔离**（模型 API 需要）。无沙箱的设备不接受跨所有者任务（`sandbox_unavailable`）。
- 合并：服务器只验证；由具名审批人自己的在线设备执行 `fetch → 校验分支头 == 已批准提交 → merge --no-ff → push`。
- 详见 `docs/collabos-prd-v0.4.md`、`docs/verification.md`。
