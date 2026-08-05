# Codex CLI 会话格式调查

## 已验证信息

| 项 | 值 |
|----|-----|
| 版本 | 0.146.0（`codex --version` 实测，显示 codex-cli 0.146.0） |
| 二进制 | `~/.npm-global/bin/codex` |
| 数据目录 | `~/.codex/` |
| 会话文件 | `~/.codex/sessions/<YYYY>/<MM>/<DD>/rollout-<timestamp>-<sessionId>.jsonl` |
| 其他文件 | `history.jsonl`（仅用户输入记录）、`logs_2.sqlite`（结构化日志，非会话主数据） |
| 格式 | JSONL，每行一个 JSON 对象 |

## 行类型（实测采样）

```json
{"type": "session_meta", "timestamp": "...", "payload": {
  "id": "019fc636-...",
  "session_id": "019fc636-...",
  "cwd": "/home/superc/code/my-skills",
  "git": {"commit_hash": "...", "branch": "master", "repository_url": "ssh://git@.../my-skills.git"},
  "model_provider": "custom",
  "cli_version": "0.146.0",
  "history_mode": "legacy",
  "originator": "codex-tui"
}}
```

其余行类型：`event_msg`（含 `task_started`、`token_count` 等）、`response_item`（含 `message`、`custom_tool_call_output` 等）、`world_state`、`turn_context`。

## response_item.message 结构（用户消息）

```json
{"type": "response_item", "timestamp": "...", "payload": {
  "type": "message",
  "role": "user",
  "id": "msg_...",
  "content": [
    {"type": "input_text", "text": "..."},
    {"type": "input_text", "text": "<environment_context>..."}
  ]
}}
```

assistant 消息含 `internal_chat_message_metadata_passthrough` 等字段。

## token_count 事件（事件类型，已实测）

```json
{"type": "event_msg", "payload": {"type": "token_count", "info": {
  "total_token_usage": {
    "input_tokens": 21154,
    "cached_input_tokens": 0,
    "cache_write_input_tokens": 0,
    "output_tokens": 370,
    "reasoning_output_tokens": 107,
    "total_tokens": 21524
  },
  "last_token_usage": {...},
  "model_c..." : "..."
}}}
```

注意：`total_token_usage` 与 `last_token_usage` 并存，且 total 可能是**累计值**，需按 §13.3 去重识别。

## 字段可用性

| 字段 | 状态 |
|------|------|
| 会话 ID | session_meta.payload.session_id / id |
| 创建时间 | session_meta.timestamp 或文件名 timestamp |
| 最后活动 | 文件最后一条记录 timestamp |
| 工作目录 | session_meta.payload.cwd |
| Git | session_meta.payload.git（branch/commit/repository_url） |
| 模型 Provider | session_meta.payload.model_provider |
| 版本 | session_meta.payload.cli_version |
| 用户消息 | response_item message role=user content input_text |
| Token | event_msg token_count |

## 首次提问提取

- 按序扫描，找到第一条 `response_item` 且 `payload.type=="message"` 且 `role=="user"` 的记录。
- **必须过滤注入块**：实测首条 user 消息 content 包含：
  - `# AGENTS.md instructions ... <INSTRUCTIONS>...</INSTRUCTIONS>` 块
  - `<environment_context><cwd>...</cwd>...` 块
- 取剩余 `input_text` 块拼接。

## 恢复命令

```
codex resume <session-id>
```

实测 `codex resume --help` 确认：`codex resume [SESSION_ID]` 或 `codex resume --last`。

## 已知限制 / 未确认项

- `history.jsonl` 只记录用户输入，不能作为完整会话来源（仅可辅助）。
- `logs_2.sqlite` 主要是日志（target=feedback_tags 等），非会话主数据；不作为解析来源。
- token_count 中 total/last 语义需要更多样本验证累计 vs 增量。
- 会话文件按天分目录，Discover 需递归遍历。

## 测试样本来源

真实环境 `~/.codex/sessions/2026/08/03/rollout-2026-08-03T14-01-19-019fc636-*.jsonl`（213 行）等只读采样。

## 验证状态

- [x] 真实环境验证：目录结构、session_meta、user 消息结构、注入块、token_count、恢复命令。
- [ ] 兼容性假设（未验证）：老版本字段差异、历史模式（history_mode）其他取值、非 interactive 会话。
