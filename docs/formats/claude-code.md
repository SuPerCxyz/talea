# Claude Code 会话格式调查

## 已验证信息

| 项 | 值 |
|----|-----|
| 版本 | 2.1.216（`claude --version` 实测） |
| 二进制 | `~/.npm-global/bin/claude` |
| 数据目录 | `~/.claude/` |
| 会话文件 | `~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl` |
| 子会话 | `~/.claude/projects/<encoded-cwd>/<sessionId>/subagents/agent-*.jsonl` |
| 编码规则 | cwd 路径中 `/` → `-`（如 `/home/superc/code/pentimento` → `-home-superc-code-pentimento`） |
| 格式 | JSONL，每行一个 JSON 对象 |

注意：`~/.claude/sessions/` 目录在实测机上存在但为空；实际会话存储在 `projects/` 下。本调查以 `projects/` 为准。

## 单行结构（实测）

```json
{
  "type": "user" | "assistant" | "system" | "summary",
  "timestamp": "2026-07-17T04:57:32.179Z",
  "cwd": "/home/superc/code/pentimento",
  "sessionId": "3818b566-96ac-4f46-99eb-32346319749e",
  "gitBranch": "master",
  "isSidechain": false,
  "version": "v2",
  "uuid": "...",
  "parentUuid": null,
  "message": {
    "role": "user",
    "content": "..."  // string 或数组
  }
}
```

assistant 行 message 含额外字段：

```json
{
  "message": {
    "role": "assistant",
    "content": [{"type": "text", "text": "..."}],
    "id": "msg_...",
    "model": "glm-5.2",
    "usage": {"input_tokens": 0, "output_tokens": 0}
  }
}
```

## 字段可用性

| 字段 | 状态 |
|------|------|
| 会话 ID | `sessionId`（顶层，已确认） |
| 创建时间 | 文件第一条记录 timestamp，无单独元数据 |
| 最后活动 | 文件最后一条记录 timestamp |
| 工作目录 | `cwd`（顶层，已确认） |
| Git 分支 | `gitBranch`（顶层） |
| 用户消息 | type=user 的 message.content（string 或数组） |
| 工具调用 | 数组 content 中的 tool_use / tool_result 块 |
| Token usage | assistant 消息 message.usage，字段为 input_tokens/output_tokens |
| 子会话关系 | `parentUuid` / `isSidechain` + subagents/ 目录 |

## 首次提问提取

- 按 timestamp 排序，找到第一条 `type=="user"` 且 content 非空的记录。
- content 为 string 时直接取；为数组时取各块 text 拼接。
- 需过滤以 `<` 开头的系统提醒块（实测首条 user 内容为纯文本，未见注入块；但为兼容，保留过滤逻辑）。

## Token 字段含义

- `usage.input_tokens` / `usage.output_tokens` 为**单次请求增量值**（实测样本中为 0/0，表示该版本对部分模型未填充）。
- 未观察到 cache/reasoning/tool 字段；若缺失则按「未知」处理，不得显示为 0。

## 恢复命令

```
claude --resume <sessionId>
```

实测 `claude --help` 确认存在 `--resume`。在会话文件所在目录之外使用需先 `cd` 到原 cwd（Talea 负责 chdir）。

## 已知限制 / 未确认项

- `usage` 字段在部分模型/版本下为 0/0，不能作为有效 Token 数据。
- content 数组各块类型（tool_use 等）细节未逐一枚举。
- summary 类型记录的结构未采样。
- 无独立会话元数据文件，创建/结束时间需从首尾记录推断。

## 测试样本来源

真实环境 `~/.claude/projects/-home-superc-code-pentimento/3818b566-*.jsonl` 只读采样（含 4261 行）。夹具需脱敏后放入 `testdata/claude/`。

## 验证状态

- [x] 真实环境验证：目录、单行结构、cwd、sessionId、时间戳、首条 user 消息、assistant usage、恢复命令。
- [ ] 兼容性假设（未验证）：老版本格式、content 数组全类型、summary 结构。
