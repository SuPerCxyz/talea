# Agent 活动状态与看板

`talea dashboard` 与 `talea status` 从各 Agent 已有的会话记录中读取“回合状态”，
帮助你在多个终端之间判断：谁还在工作、谁已经结束、谁可能在等你确认。

> **“已完成”表示本轮结束并等待下一步，不表示语义任务完成。**
> Talea 不做任务级完成判断，也不调用 LLM。

## 状态取值

| 通用回合状态 | 展示状态 | 含义 | 需要你介入 |
|-------------|---------|------|-----------|
| `working` | 进行中 / 可能等待 | 回合仍在执行；超过安静阈值后转为推断的“可能等待” | 仅“可能等待” |
| `waiting_input` | 等待你确认 | 权限请求、提问或确认等待 | 是 |
| `completed` | 本轮结束 | 当前回合已结束，等待下一步输入 | 否 |
| `failed` | 失败 | 错误或异常终止 | 是 |
| `interrupted` | 已中断 | 用户中断或回合被取消 | 是 |
| `idle` | 空闲 | 会话空闲；超出 24h 的旧会话不进入默认输出 | 否 |
| `unknown` | 未知 | 没有可识别的回合事件；不会猜测为完成 | 否 |

状态来源分为：

- `native`：Agent 会话记录中的原生终态/事件（如 Claude `end_turn`、Codex
  `task_complete`、OpenCode `idle_outcome`）。
- `inferred`：由最后一条消息或时间阈值推断（可能等待、长时间无输出）；活动状态只用于
  补充说明，不会把无法识别的回合猜测为已完成。
- `hook`：用户显式配置的 Claude Code hooks / OpenCode plugin 写入的精确事件。
- `unknown`：无法判断。

时间阈值集中在 `internal/status`：

| 常量 | 默认值 | 作用 |
|------|-------|------|
| `QuietThreshold` | 2 分钟 | `working` 超过该时间无新回合事件 → “可能等待”（推断） |
| `ActiveWindow` | 30 分钟 | 超过该时间仍无事件 → 归入空闲 |
| `CompletedWindow` | 24 小时 | `completed` 在该窗口内显示“本轮结束”，更早不进入默认输出 |

精确 hook 事件只有在其时间**不早于**索引中的回合状态时间时才覆盖推断结果。

看板与 `talea status` 默认只展示最近 **24 小时**的会话：可见时间取
`state_at` 与 `last_activity_at` 中较新的值。没有时间的 `waiting_input` /
`failed` / `working` 会话仍保留；超过窗口的 idle、unknown、completed 等旧会话
不进入输出，汇总计数也只统计实际展示的会话。该窗口可通过内部
`status.Options.RecentWindow` 调整。

## 命令

```sh
talea dashboard          # 常驻看板：3 秒自动增量刷新
talea status             # 一次性文本状态
talea status --short     # 状态栏单行汇总
talea status --json      # 稳定 JSON，供脚本 / tmux / 后续界面复用
```

看板按键：`↑/↓` 或 `j/k` 选择，`Enter`/`d` 详情，`o` 恢复会话，`/` 过滤，
`Esc` 返回或清除过滤，`q` 退出。详情页复用会话列表的详情与恢复流程。

`status --json` 结构示例（省略部分会话字段）：

```json
{
  "generated_at": "2026-11-24T10:00:00+08:00",
  "counts": {
    "needs_attention": 1,
    "waiting": 1,
    "working": 2,
    "completed": 3,
    "failed": 0,
    "idle": 4,
    "unknown": 0,
    "total": 10
  },
  "sessions": [
    {
      "agent_id": "claude-code",
      "agent": "claude-code",
      "agent_instance_id": "claude-code-default",
      "session_id": "3818b566-...",
      "project_name": "talea",
      "working_directory": "/home/user/code/talea",
      "state": "waiting",
      "label": "Waiting for you",
      "turn_state": "waiting_input",
      "source": "hook",
      "confidence": "exact",
      "reason": "notification: permission_prompt",
      "needs_attention": true,
      "state_at": "2026-11-24T09:59:30+08:00",
      "last_activity_at": "2026-11-24T09:59:31+08:00",
      "total_tokens": 12345
    }
  ]
}
```

## 可选精确事件（不自动安装）

Claude Code 的权限等待不会写入会话记录，Codex/OpenCode 的部分事件也随版本变化。
如果需要精确识别，可显式配置 hook/plugin，把事件写入 Talea 自己的
`${XDG_DATA_HOME:-~/.local/share}/talea/status-events.jsonl`：

- 文件权限 `0600`，目录 `0700`。
- 读取时只读取文件尾部固定上限（约 1 MiB），并跳过无法解析的行；事件量通常很小。
- Talea **不会**修改、安装或重命名任何 Agent 配置与会话文件。
- hook/plugin 进程需要能找到 `talea` 可执行文件（PATH 或绝对路径）。
- `talea hook` 对坏 JSON、缺少字段、无效状态或写入失败一律静默退出 0，
  避免阻塞或中断 Agent。

### Claude Code hooks 示例

在 `~/.claude/settings.json` 中按你的 Claude Code 版本加入（示例仅覆盖状态相关
事件，matcher 细节以官方 hooks 文档为准）：

```json
{
  "hooks": {
    "Notification": [
      { "hooks": [ { "type": "command", "command": "talea hook" } ] }
    ],
    "PermissionRequest": [
      { "hooks": [ { "type": "command", "command": "talea hook" } ] }
    ],
    "Stop": [
      { "hooks": [ { "type": "command", "command": "talea hook" } ] }
    ],
    "StopFailure": [
      { "hooks": [ { "type": "command", "command": "talea hook" } ] }
    ],
    "SessionEnd": [
      { "hooks": [ { "type": "command", "command": "talea hook" } ] }
    ]
  }
}
```

`talea hook` 读取 Claude 的 stdin JSON（`session_id`、`hook_event_name`、
`notification_type` 等）并映射为等待/进行中/完成/失败/空闲事件。`Notification`
仅在缺失类型或类型为 `permission_prompt` / `idle_prompt` / `elicitation_dialog`
时视为等待；`auth_success` 等通知会被忽略。它只读取状态字段，不保存消息正文。

### OpenCode plugin 示例

在 OpenCode 插件目录（例如 `~/.config/opencode/plugin/talea-status.js`）中放置
一个显式启用的事件转发插件。下面仅为示意，事件名与插件 API 需按你的 OpenCode
版本核对；Talea 不会自动写入该文件。

```js
import { spawn } from "node:child_process"

const forwarded = new Set([
  "permission.asked",
  "session.idle",
  "session.error",
  "message.updated",
])

export const TaleaStatus = async ({ event }) => {
  if (!event?.type || !forwarded.has(event.type)) return

  const child = spawn("talea", ["hook"], {
    stdio: ["pipe", "ignore", "ignore"],
    detached: true,
  })
  child.on("error", () => {})
  child.stdin.on("error", () => {})
  child.stdin.end(JSON.stringify(event))
  child.unref()
}
```

Talea 直接接受 OpenCode 原始事件形状：

```json
{"type":"session.idle","properties":{"sessionID":"ses_...","outcome":"succeeded"}}
{"type":"permission.asked","properties":{"info":{"id":"ses_..."}}}
```

`session.idle` 没有 outcome 时按“本轮结束”处理；`properties.outcome` /
`properties.status` 为 `failed`、`interrupted` 等时映射为对应状态，未知非空值回退为
空闲。

也可以直接调用带参数的入口：

```sh
printf '%s' '{"agent":"opencode","session_id":"ses_...","state":"waiting_input"}' \
  | talea hook
talea hook --agent opencode --session ses_... --state completed
```

### 事件格式

`status-events.jsonl` 每行一个 JSON：

```json
{"agent_id":"claude-code","session_id":"...","state":"waiting_input","at":"2026-11-24T10:00:00Z","detail":"notification: permission_prompt"}
```

支持的状态：`working`、`waiting_input`、`completed`、`failed`、`interrupted`、
`idle`。无法解析的行会被跳过，不影响后续事件。

`talea hook` 的输入还兼容顶层 `type` + `properties.sessionID/sessionId/info.id`
（OpenCode 原始事件），以及顶层 `session_id` / `sessionId`（Claude hooks）。

## 只读与容错

- Agent 原始会话文件与数据库只读打开，不修改、不重命名、不删除。
- 单个 Agent/会话/事件损坏不会导致看板或 `talea status` 退出。
- 刷新失败时看板保留上一次数据，并在顶部提示错误。
