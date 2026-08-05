# OpenCode 会话格式调查

## 已验证信息

| 项 | 值 |
|----|-----|
| 版本 | 1.18.13（`opencode --version` 实测） |
| 二进制 | `~/.npm-global/bin/opencode` |
| 数据目录 | `~/.local/share/opencode/` |
| 数据库 | `opencode.db`（SQLite，实测约 6GB，WAL 约 578MB） |
| 配置 | `~/.config/opencode/opencode.json` |
| 表结构 | session / message / part / project / workspace 等 |

## 表结构（实测）

### session

```sql
id TEXT
project_id TEXT
workspace_id TEXT
parent_id TEXT
slug TEXT
directory TEXT
path TEXT
title TEXT
version TEXT
metadata TEXT
cost REAL
tokens_input INTEGER
tokens_output INTEGER
tokens_reasoning INTEGER
tokens_cache_read INTEGER
tokens_cache_write INTEGER
time_created INTEGER   -- epoch ms
time_updated INTEGER   -- epoch ms
time_compacting INTEGER
time_archived INTEGER
agent TEXT
model TEXT
```

### message

```sql
id TEXT
session_id TEXT
time_created INTEGER
time_updated INTEGER
data TEXT  -- JSON
```

message.data 示例：

```json
{"role": "user", "time": {"created": 1785919267599}, "agent": "build",
 "model": {"providerID": "relay-opencode-go", "modelID": "opencode-go/deepseek-v4-flash"},
 "summary": {"diffs": []}}

{"parentID": "msg_...", "role": "assistant", "mode": "build", "agent": "build",
 "path": {"cwd": "/home/superc/code/talea", "root": "/"},
 "tokens": {"total": 61181, "input": 58587, "output": 46, ...}, "cost": 0, ...}
```

### part

```sql
id TEXT
message_id TEXT
session_id TEXT
time_created INTEGER
data TEXT  -- JSON
```

part.data 类型（实测）：`text`、`reasoning`、`tool`、`step-start`、`step-finish`。

`step-finish` 含 tokens：`{"type": "step-finish", "reason": "tool-calls", "tokens": {"total": 61181, "input": 58587, "output": 46, ...}}`

## 字段可用性

| 字段 | 状态 |
|------|------|
| 会话 ID | session.id（ses_ 前缀） |
| 创建时间 | session.time_created（epoch ms） |
| 最后活动 | session.time_updated |
| 工作目录 | session.directory / path |
| 标题 | session.title（自动生成） |
| 模型 | session.model（JSON：modelID/providerID） |
| 父子会话 | session.parent_id |
| Token 汇总 | session.tokens_input/output/cache_read/cache_write/reasoning |
| 用户消息 | message.data.role=user，正文在关联 part 的 text 块 |
| 时间线 | part 的 step-start/step-finish tokens |

## 首次提问提取

- 按 message.time_created 升序，找到第一条 role=user 的 message。
- 正文为关联 part 中 type=text 的 text 拼接。
- 无系统注入块（与 Codex 不同），但保留通用过滤逻辑。

## Token 字段含义

- session 表 tokens_* 为**会话级汇总**（累计）。
- step-finish 的 tokens 为**上下文快照**（total 为累计上下文，input 为本次增量，
  已实测确认 2026-08-05）。
- 两者并存时必须按 §13.3 去重，避免重复累计。

## 恢复命令

```
opencode run -s <session-id>
```

实测 `opencode run --help` 确认 `-s/--session` 参数。另有 `opencode session list` 可用作校验。

## 只读访问要求

- 必须使用 `file:...?mode=ro` URI 只读打开。
- 必须设置 busy timeout（WAL 模式，其他进程可能持有锁）。
- 数据库很大（6GB），禁止整库复制；增量索引基于 `time_updated`。
- 只读打开不 checkpoint WAL，可读到已提交但未 checkpoint 的数据——符合预期。

## 已知限制 / 未确认项

- project/workspace 表的用途（可能与目录关联）未展开。
- session.path 与 directory 的关系未确认（样本中 path 为 `home/...` 无前导斜杠，directory 为完整路径）。
- 子会话（parent_id 非空）的 usage 是否被计入父会话 tokens_* 需确认（当前按独立会话处理）。

## 测试样本来源

真实环境 `~/.local/share/opencode/opencode.db` 只读查询（session/message/part）。夹具需从真实数据脱敏导出小样本，或手工构造等价结构。

## 验证状态

- [x] 真实环境验证：数据库表结构、session 字段、message/part 结构、step-finish tokens、恢复命令。
- [x] 真实环境验证（2026-08-05 补充）：**WAL 可见性**（只读连接读到 20:13 的新会话，
      WAL 578MB 未 checkpoint）、**子会话**（27 个带 parent_id，父会话 time_updated
      覆盖子会话更新）、step-finish total 为累计上下文。
- [ ] 兼容性假设（未验证）：老版本 schema、子会话 usage 与父会话 tokens_* 的精确关系。
