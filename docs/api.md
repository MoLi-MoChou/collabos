# CollabOS 开放 API 与 SDK

- 机器可读的接口规格：`GET /api/openapi.yaml`（OpenAPI 3.1，源文件为 `server/internal/handler/openapi.yaml`）。CI 中的 `TestOpenAPICoversRoutes` 会确保每条路由都写进了规格。
- 所有路由同时提供在 `/api/...` 和 `/api/v1/...` 下。外部集成请使用 `/api/v1`。
- 错误体格式统一为 `{"error": "...", "code": "..."}`。

## 认证

| 令牌 | 前缀 | 获取方式 | 有效期 |
|---|---|---|---|
| 访问令牌 | `cos_u_` | 登录（`POST /api/auth/dev-login`）或刷新 | 12h（`COLLAB_ACCESS_TOKEN_TTL`） |
| 刷新令牌 | `cor_` | 与访问令牌一起签发 | 30 天；每次使用都会换发新令牌，旧令牌随即失效 |
| 个人 API 令牌 | `coa_` | `POST /api/tokens {"label":"ci"}`，明文只返回这一次 | 1 年；可以通过 `DELETE /api/tokens/{id}` 吊销 |
| 设备令牌 | `cos_n_` | 设备配对时签发 | 设备 owner 可以通过 `DELETE /api/daemons/{id}` 吊销，吊销后立即断开连接 |

调用时带上请求头 `Authorization: Bearer <token>`。凡是工作区范围的接口，还要带 `X-Workspace-ID`。

刷新：`POST /api/auth/refresh {"refresh_token":"cor_…"}` 返回 `{"token","refresh_token","expires_at"}`。
登出：`POST /api/auth/logout {"refresh_token":"…"}`，会同时吊销当前的访问令牌和这里传入的刷新令牌。

## TypeScript SDK

```ts
import { createCollabClient } from "@collabos/core";

const c = createCollabClient({ baseUrl: "https://collab.example.com", token: process.env.COLLAB_TOKEN!, workspaceId: "…" });
const { task } = await c.createDelegation({
  room_id, project_id, target_agent_id,
  title: "新增 multiply", prompt: "…",
  grant: { resources: ["src/**"], actions: ["read", "edit"] },
  depends_on: [otherTaskId], approval_rule: "ALL", acceptance: ["新增单测通过"],
});
const trace = await c.trace(task.trace_id);   // 链上的任务、事件、PR 和审计
const usage = await c.usage();                // 本月用量与预算
```

每个响应都会用 zod 校验。如果服务端返回了新增字段，旧版 SDK 也照常可用。传入 `refreshToken` 后，遇到 401 会自动刷新一次令牌并重试。

## V0.4 新增路由一览

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | /api/tasks | 新增字段 `depends_on`、`approval_rule`（ANY_ONE/ALL）、`acceptance`、`trace_id`（可选，留空则新建） |
| GET | /api/traces/{trace_id} | 按 trace 查询任务、事件、PR 和审计 |
| POST | /api/tasks/{id}/nudge | 催办（同一人对同一任务 10 分钟内只能催一次） |
| POST | /api/tasks/{id}/budget | 扩大因预算暂停的任务的预算（委托方或付费方） |
| GET | /api/usage | 本月用量，按委托方、owner、Bot 分组，并附预算 |
| PUT | /api/budgets | 设置团队预算（仅 owner）或个人预算（仅本人） |
| PATCH | /api/agents/{id}/policy | 自主级别与 owner 约束 |
| POST | /api/agents/{id}/patrol | 只读巡检（场景 D） |
| PATCH | /api/projects/{id}/settings | 可见性、测试命令、webhook 密钥（仅 lead） |
| GET/POST/DELETE | /api/projects/{id}/members[/{uid}] | 项目成员与角色 |
| POST | /api/projects/{id}/memory | 编辑 MEMORY.md：lead 编辑直接确认，其他人的编辑需等待确认 |
| POST | /api/memory/{id}/taint | 把该版本标记为污染：它之后的版本和引用过它的任务会一并标记 |
| GET | /api/memory/{id}/references | 列出引用过该版本的任务 |
| POST | /api/messages/{id}/actions | 富消息交互：以点击者身份记录，作为 user_input 回流，不会执行任何平台操作 |
| POST | /api/messages/{id}/to-task | 把 Bot 发现转为委托草稿（仅 lead） |
| PATCH | /api/workspace/im | 外部 IM 出站通知（slack/feishu/dingtalk，仅 owner） |
| POST | /api/webhooks/github/{project_id} | GitHub webhook（HMAC-SHA256 签名，按 delivery 去重） |

## GitHub webhook

1. 项目 lead 在「记忆」页的项目设置中填写 webhook 密钥。
2. 在 GitHub 仓库的 Settings → Webhooks 中：
   - Payload URL 填 `https://<server>/api/webhooks/github/<project_id>`；
   - Content type 选 `application/json`；
   - Secret 填同一个密钥；
   - 事件勾选 Pull requests。
3. 平台状态优先：
   - PR 在 GitHub 上被合并：任务完成，原因为 `merged_on_platform`；
   - PR 被关闭：任务取消，原因为 `pr_closed_on_platform`；
   - PR 头提交发生变化：原批准作废，任务失败，原因为 `commit_mismatch`。
