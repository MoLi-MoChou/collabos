# CollabOS v2 — 让不同电脑上的编码智能体互相调用

你在自己电脑上用 opencode / Claude Code / Codex / Gemini CLI 开发，跟它说“叫 zhangsan 去 review 一下”，
它就通过 `collab` MCP 工具把请求（连同你仓库的改动补丁、remote、分支）发到服务器，服务器转给 zhangsan
电脑上的 `collab daemon`，daemon 用那台机器配置的 ACP 智能体（默认 `opencode acp`，全部权限自动放行）
干完活，把最终回复原路送回，你的智能体拿到结果继续干。没有审批，所有调用都记录在服务器上。

```
你的 agent ──MCP(stdio)── collab mcp ──HTTP──▶ relay server ──WebSocket──▶ collab daemon ──ACP──▶ 对方的 agent
```

一个二进制 `collab` 包含三个角色：`collab server`（中转+记录，Postgres）、`collab daemon`（每台机器一个）、`collab mcp`（给你的智能体用）。

## 每台电脑的设置

1. 管理员在服务器上给设备发令牌：
   `docker exec collabos-v2-relay-1 collab server add-device kai-windows --owner kai`
2. 在电脑上：
   ```
   collab login --server http://SERVER:8190 --token cdv_...
   collab agent opencode            # 或 claude / codex / gemini / grok，或 collab agent -- 自定义命令 参数
   collab model opencode/minimax-m3.1-flash-free   # 可选，仅 opencode
   collab repo add D:\code\myproj   # 可选：别人发来的补丁会应用到这个仓库的临时 worktree 上
   collab daemon                    # 保持运行，接收别人的调用
   collab status                    # 查看在线设备
   ```
3. 把 MCP 加到你的智能体里（下面任选其一）。

### opencode（`~/.config/opencode/opencode.json`）
```json
{
  "mcp": {
    "collab": { "type": "local", "command": ["collab", "mcp"], "enabled": true }
  }
}
```
Windows 上 `command` 写完整路径，例如 `["C:\\Users\\you\\collabos\\bin\\collab.exe", "mcp"]`。

### Claude Code
```
claude mcp add collab --scope user -- collab mcp
```
远程调用可能要几分钟，建议设置 `MCP_TOOL_TIMEOUT=1800000`（毫秒）。

### Codex（`~/.codex/config.toml`）
```toml
[mcp_servers.collab]
command = "collab"
args = ["mcp"]
tool_timeout_sec = 1800
```

### Gemini CLI（`~/.gemini/settings.json`）
```json
{ "mcpServers": { "collab": { "command": "collab", "args": ["mcp"], "timeout": 1800000 } } }
```

## MCP 工具

| 工具 | 作用 |
|---|---|
| `list_agents` | 列出所有设备、主人、在线状态、智能体 |
| `call_agent(target, prompt, cwd?, include_code?, base?, timeout_minutes?)` | target 填人名（zhangsan）或设备名。默认附带 `cwd` 仓库相对 upstream/merge-base 的全部改动（含未提交和未跟踪文件，不动你的 index）。阻塞直到对方回复，期间发 MCP progress 通知 |
| `get_call_result(call_id, wait_seconds?)` | 客户端超时时继续等待/取结果 |
| `cancel_call(call_id)` | 取消 |

命令行也能直接用：`collab agents`、`collab call zhangsan "review 一下" --cwd .`、`collab calls`。

## 被调用方怎么看到代码
补丁写在工作目录的 `CHANGES.patch`，同时放进提示词。如果被调用方 `collab repo add` 过同一个 remote，
daemon 会 fetch、在 base 提交上开临时 worktree（`./code`）并应用补丁，智能体能看到完整代码；结束后删除。

## 安全提示
bypass 模式：被调用的智能体在对方电脑上不经确认执行命令和改文件（在临时目录里）。只把令牌发给信任的人。
服务器目前是 HTTP，令牌明文传输，正式使用请加 HTTPS 反代。

## 环境变量
`COLLAB_HOME`（默认 `~/.collab`）、`COLLAB_SERVER`/`COLLAB_TOKEN`（覆盖配置）、`COLLAB_MCP_MAX_WAIT`（call_agent 最长阻塞，默认 30m；客户端工具超时更短时调小，超时后用 get_call_result）。
