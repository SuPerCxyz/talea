## Purpose

让 Talea 在 Agent 会话运行策略发生变化或 Agent 本地存储格式升级时，仍能依据可验证数据安全地索引、展示和恢复会话。

## ADDED Requirements

### Requirement: Codex SHALL use the latest verified runtime policy

当一个 Codex rollout 中存在多个有效的 `turn_context` 策略记录时，Talea SHALL 使用按会话记录顺序最后出现的有效审批策略和沙箱策略生成恢复参数。恢复参数 SHALL 继续经过 Codex 白名单校验，并使用当前 CLI 支持的语义等价参数；未知字段不得覆盖已知值或扩大权限。

#### Scenario: Later resume switches to yolo policy

- **WHEN** rollout 先记录普通审批/沙箱策略，随后记录 `approval_policy=never` 与 `sandbox_policy.type=danger-full-access`
- **THEN** Talea 索引会话的恢复参数 SHALL 为 `--dangerously-bypass-approvals-and-sandbox`，并 SHALL 在 `resume <session-id>` 之前传递

#### Scenario: Later partial policy does not invent the other policy

- **WHEN** 后续 `turn_context` 只包含一个已知策略字段，另一个字段缺失或未知
- **THEN** Talea SHALL 只恢复后续已知字段，不得从缺失字段推断另一项权限

### Requirement: OpenCode SHALL resolve its current local database path safely

Talea SHALL 支持 OpenCode v2 CLI 解析出的数据库路径及 `OPENCODE_DB` 覆盖路径；相对覆盖路径 SHALL 按 OpenCode 数据目录解释。路径解析失败或旧版 CLI 不提供路径命令时，Talea SHALL 回退到现有数据目录下的 `opencode.db`。OpenCode 数据库 SHALL 始终以只读方式打开。

#### Scenario: OpenCode v2 default database

- **WHEN** OpenCode v2 使用默认数据目录和数据库
- **THEN** Talea SHALL 发现、解析和恢复该数据库中的会话，并使用 `opencode -s <session-id>`

#### Scenario: OpenCode database override

- **WHEN** 用户通过 `OPENCODE_DB` 指定绝对或相对数据库路径
- **THEN** Talea SHALL 从该路径发现会话，且不得读取默认数据库代替它

#### Scenario: Database path resolution falls back

- **WHEN** 路径探测命令不可用或返回无效路径
- **THEN** Talea SHALL 使用兼容旧版的 `<data-directory>/opencode.db` 路径，并保留单 Agent 容错行为

### Requirement: OpenCode v2 session metadata SHALL preserve conservative data semantics

Talea SHALL 接受 OpenCode v2 `session` 表中的合法可空元数据字段。Token 汇总只有在来源明确提供非零用量时才可标记为已知；仅有数据库默认零值的会话 SHALL 显示为未知，而不是精确的 0。v2 新增的投影表不得破坏对仍保留的 `session/message/part` 会话数据的读取。

#### Scenario: Nullable OpenCode metadata

- **WHEN** OpenCode 会话的 `path`、`agent` 或 `model` 字段为 NULL
- **THEN** Talea SHALL 仍能解析会话，并将缺失字段保留为空值

#### Scenario: Default zero tokens

- **WHEN** OpenCode 会话的 Token 列全部为数据库默认 0 且没有明确用量
- **THEN** Talea SHALL 不标记 Token 用量为已知 0

#### Scenario: V2 projection tables coexist

- **WHEN** OpenCode v2 数据库同时存在 `session_message` 等投影表和传统 `session/message/part` 表
- **THEN** Talea SHALL 忽略不需要的额外表而继续解析传统会话数据，不因额外表或列失败
