# OpenCode 会话格式调查

## 已验证信息

| 项 | 值 |
|----|-----|
| 版本 | 2.0.12（`opencode --version` 实测，2026-09-22；2026-09-18 前基线为 2.0.7） |
| 二进制 | `~/.npm-global/bin/opencode` |
| 数据目录 | `opencode debug paths data` 输出的目录 |
| 数据库 | `opencode debug paths db` 输出的 SQLite 路径（本机 `~/.local/share/opencode/opencode.db`，约 9.9GB，WAL）；支持 `OPENCODE_DB` 覆盖 |
| 配置 | `~/.config/opencode/opencode.json` |
| 表结构 | 传统 `session`/`message`/`part`（2026-09-18 起冻结）与 v2 `session_v2`/`session_message`/`event` 并存 |

### 存储迁移事实（2026-09-22 实测）

- `session` 384 行已冻结，`max(time_updated) = 1789700605279`（2026-09-18）。
- `session_v2` 494 行持续写入，`session_v2` ⊃ `session`；仅 3 个会话只在 `session`
  中（它们在 `message` 有数据）。
- 113 个仅在 `session_v2` 的会话在 `message`/`part` 中 0 行，消息全在 `session_message`。
- 存量会话的 `session_message` 覆盖**不完整不等价**（样例 `message=734` vs
  `session_message=377`），因此不能整体改用 `session_message`。
- 上游是否写入以数据库自身的表存在性为准：Talea 通过 `sqlite_master` 探测
  `session_v2`，不按 `opencode --version` 分支。

## 表结构（实测）

### session（传统主表，2026-09-18 起冻结）

```sql
id TEXT
project_id TEXT
workspace_id TEXT
parent_id TEXT
slug TEXT
directory TEXT          -- NOT NULL
path TEXT
title TEXT              -- NOT NULL
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
permission TEXT
```

### session_v2（v2.0.12 主表，持续写入）

```sql
id TEXT                 -- PRIMARY KEY
project_id TEXT
workspace_id TEXT
parent_id TEXT
slug TEXT
directory TEXT          -- NOT NULL（与 session 相同）
path TEXT
title TEXT              -- 可空（session.title 为 NOT NULL）
version TEXT
metadata TEXT
cost REAL
tokens_input INTEGER    -- 会话级累计
tokens_output INTEGER
tokens_reasoning INTEGER
tokens_cache_read INTEGER
tokens_cache_write INTEGER
time_created INTEGER   -- epoch ms
time_updated INTEGER   -- epoch ms
time_compacting INTEGER
time_archived INTEGER
agent TEXT              -- 可空
model TEXT              -- 可空
permission TEXT
fork_session_id TEXT    -- v2 新增（本 change 不消费）
fork_boundary INTEGER
time_suspended INTEGER
resume_attempts INTEGER
time_idle INTEGER
time_viewed INTEGER
idle_outcome TEXT
```

Talea 的元数据解析优先读 `session_v2`，行缺失或无该表时回退 `session`；
发现查询为两者并集：`session_v2` 全量 + 不在 `session_v2` 的传统遗留行（`UNION` +
`NOT EXISTS` 按 `id` 去重），高水位过滤施加在并集子查询外层。

### message（传统消息表，冻结）

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

### part（传统消息分块表，冻结）

```sql
id TEXT
message_id TEXT
session_id TEXT
time_created INTEGER
data TEXT  -- JSON
```

part.data 类型（实测）：`text`、`reasoning`、`tool`、`step-start`、`step-finish`。

`step-finish` 含 tokens：`{"type": "step-finish", "reason": "tool-calls", "tokens": {"total": 61181, "input": 58587, "output": 46, ...}}`

### session_message（v2 消息主表，持续写入）

```sql
id TEXT
session_id TEXT
type TEXT    -- user/assistant/idle/synthetic/system/compaction/model-switched/agent-switched
seq INTEGER
time_created INTEGER   -- epoch ms
time_updated INTEGER
data TEXT   -- JSON
```

每行是**一条完整消息**（不是分块）。`data` 结构（实测样例，已脱敏）：

```json
// type=user
{"time":{"created":1789800001000},"text":"新会话的第一个问题","files":[],"agents":[]}

// type=assistant
{"agent":"build","mode":"build",
 "model":{"id":"m1","providerID":"mock","variant":"v"},
 "content":[{"type":"text","text":"正在查看文件。"},
   {"type":"tool","id":"call_1","name":"bash",
     "state":{"status":"completed","input":{"filePath":"/path/main.go"},
       "content":[],"metadata":{}},
     "time":{"created":1789800002100,"completed":1789800002900}}],
 "finish":"stop","cost":0.01,
 "tokens":{"input":232,"output":44,"reasoning":0,"cache":{"read":100,"write":10}},
 "time":{"created":1789800002000}}

// type=compaction
{"status":"compacted","reason":"auto","summary":"...","recent":"...",
 "time":{"created":1789800004000}}
```

`content[]` 内容块类型实测：`text`、`reasoning`、`tool`；tool 块以
`state.status == "completed"` 判定结束，`state.input.filePath` 提供文件路径，
块级 `id` 为工具调用 ID（`call_...`），`name` 为工具名。

## 消息读取分流规则

按**会话数据**而不是表存在性分流：

1. 会话在传统 `message` 表有行 → 走 `message`/`part`（384 个存量会话行为完全不变，
   因为其 `session_message` 覆盖不完整）；
2. 否则 → 走 `session_message`（113 个新会话唯一可读路径）；
3. 上游未来若恢复写传统表可自动跟随，无需改代码。

时间线事件的幂等来源标识：v1 用 `opencode-msg:`/`opencode-tool:`/`opencode-compact:`/
`opencode-part:`；v2 用 `opencode-v2msg:`/`opencode-v2tool:`/`opencode-v2comp:`，
避免 schema 切换前后重复计数。

## 字段可用性

| 字段 | 来源 | 状态 |
|------|------|------|
| 会话 ID | `session_v2.id`（`ses_` 前缀），回退 `session.id` | 可用 |
| 创建时间 | `time_created`（epoch ms） | 可用 |
| 最后活动 | `time_updated`（epoch ms） | 可用 |
| 工作目录 | `directory`（NOT NULL）；`path` 关系未确认 | 部分确认 |
| 标题 | `title`（自动生成，v2 可空） | 可用，可能为 NULL |
| 模型 | `model`（JSON，可 NULL）；消息级取 `data.model.id` | 可用，可能为 NULL |
| 父子会话 | `parent_id` | 可用，子会话 usage 归属未确认 |
| Token 汇总 | `session_v2.tokens_*`（会话累计） | 可用，全零视为未知 |
| 用户消息 | `session_message` 中 `type='user'`，正文在 `data.text` | 可用 |
| 时间线 | `session_message` 的 `data.content[]`（tool）与 `data.tokens`（增量） | 可用，无上下文快照 |

## 首次提问提取

- **传统路径**：按 `message.time_created` 升序取第一条 `role=user` 的 message，
  正文为关联 `part` 中 `type=text` 的 `text` 拼接。
- **v2 路径**：按 `session_message.seq` 升序取第一条 `type='user'` 的 `data.text`。
- 两条路径都复用通用注入块过滤（`extract.FirstNonInjected`）。

## Token 字段含义

- `session_v2.tokens_*`（同 `session.tokens_*`）为**会话级累计汇总**；v2 数据库默认
  零值不能单独证明用量已知，Talea 仅在存在正数用量时标记为已知，否则显示未知而非 0。
- `session_message.assistant.data.tokens` 为**单次请求增量**（实测 input 值为 232 /
  44487 这类单请求量级），时间线按增量聚合。
- 会话累计与消息增量**禁止相加**。
- v2 没有 `step-finish`，不存在上下文快照：时间线 `ContextAfter`/`CumulativeTotal`/
  `TotalTokens` 保持**空（未知）而不是 0**；传统路径仍由 `step-finish.tokens.total`
  提供快照（total=累计上下文，input=本次增量，2026-08-05 实测）。
- 两条路径并存时按来源前缀去重，避免重复累计。

## 恢复命令

```
opencode -s <session-id>
```

`opencode` 无子命令时默认进入 TUI，`-s/--session <id>` 直接恢复指定会话（实测 2.0.7
进入会话且不要求消息参数；2.0.12 沿用）。注意：`opencode run -s <id>` 是"带消息运行"
模式，不带消息会报 `You must provide a message or a command`，不用于会话恢复。

Talea 按以下顺序解析数据库路径：`OPENCODE_DB`、`opencode debug paths db`、
`<data-directory>/opencode.db` 旧版回退。存储形态由 `sqlite_master` 中是否存在
`session_v2` 决定（`StorageShape` 对外报告 `session_v2` 或 `session`）；探测失败回退
传统 `session` 路径。所有打开均为 SQLite 只读连接，并设置 busy timeout。

## 只读访问要求

- 必须使用 `file:...?mode=ro` URI 只读打开。
- 必须设置 busy timeout（WAL 模式，其他进程可能持有锁）。
- 数据库可能很大（本机 9.9GB），禁止整库复制；Talea 先用数据库/WAL 本地指纹判断是否
  变化，变化时使用 `(time_updated, session_id)` 高水位和安全重叠窗口查询候选，游标缺失
  或查询不可靠时回退完整发现。
- 只读打开不 checkpoint WAL，可读到已提交但未 checkpoint 的数据——符合预期。

## 已知限制 / 未确认项

- `session_message` 在存量会话上覆盖不完整，已按数据分流规避；投影表不合并。
- `event` 表（事件溯源日志）与 `session_message` 冗余，不读取。
- `session.path` 与 `directory` 的关系未确认（样本中 path 为 `home/...` 无前导斜杠）。
- 子会话（`parent_id` 非空）的 usage 是否被计入父会话 `tokens_*` 需确认（当前按独立
  会话处理）。
- `session_v2` 的 `fork_session_id`/`fork_boundary`/`time_suspended` 等新列语义未调查，
  当前不消费。
- 上游再次变更表名时依赖表存在性探测与 `talea doctor` 的存储形态报告定位。

## 测试样本来源

夹具在 `t.TempDir()` 内动态生成等价结构（`session_v2`/`session_message` DDL 与 JSON
样例来自实测、已脱敏）；不引入真实 9.9GB 数据库或真实用户内容。真实环境数据库仅通过
`opencode debug paths db` 定位并只读查询。

## 验证状态

- [x] 真实环境验证（2026-08-05）：2.0.7 表结构、`session`/`message`/`part`、
      `step-finish` tokens、恢复命令与 `OPENCODE_DB` 路径解析。
- [x] 真实环境验证（2026-09-18）：WAL 可见性、`session_message` 与传统表并存、
      可空元数据、会话 Token 汇总与增量高水位查询。
- [x] 真实环境验证（2026-09-22）：2.0.12 下 `session_v2` 494 行 vs `session` 384 行
      冻结、113 个新会话消息只在 `session_message`、存量会话 `session_message` 覆盖
      不完整、`assistant.data.tokens` 为单次增量、`title` 可空、v2 无 `step-finish`。
- [ ] 兼容性假设（未验证）：老版本 schema、子会话 usage 与父会话 `tokens_*` 的精确
      关系、`fork_*` 等新列语义。
