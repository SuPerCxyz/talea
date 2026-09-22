## Purpose

保证 OpenCode 上游把会话与消息写入新存储表后，Talea 仍依据真实本地数据完成会话发现、元数据解析、消息预览、时间线与恢复，同时维持 Agent 数据库只读、单会话容错与“未知不等于 0”的数据口径。

## ADDED Requirements

### Requirement: OpenCode session discovery SHALL follow the actual storage shape

Talea SHALL 依据数据库自身存在的表决定 OpenCode 会话发现来源：存在 `session_v2` 表时以它为主来源，并同时纳入只存在于传统 `session` 表的会话；不存在 `session_v2` 表时 SHALL 使用传统 `session` 表。发现结果 SHALL 按会话 ID 去重，同一会话不得出现两次，也不得因表形态差异被丢弃。

#### Scenario: Sessions written only to session_v2

- **WHEN** OpenCode 只把新会话写入 `session_v2`，传统 `session` 表不再更新
- **THEN** Talea SHALL 发现这些会话，并在列表中展示其标题、工作目录与最后活动时间

#### Scenario: Legacy-only sessions are preserved

- **WHEN** 某会话只存在于传统 `session` 表而不在 `session_v2` 中
- **THEN** Talea SHALL 仍发现该会话，且不得因并集查询重复发现同一会话

#### Scenario: Legacy database without session_v2

- **WHEN** 数据库不存在 `session_v2` 表
- **THEN** Talea SHALL 继续使用传统 `session` 表完成发现，行为与既有版本一致

#### Scenario: Storage flavor cannot be determined

- **WHEN** 表存在性探测因数据库不可读而失败
- **THEN** Talea SHALL 回退到传统 `session` 表路径，保留已有索引，并且不因该失败退出应用

### Requirement: The incremental cursor SHALL be invalidated when the storage shape changes

OpenCode 的增量发现游标 SHALL 携带其建立时的存储形态。当当前形态与游标记录的形态不一致时——包括没有形态字段的既有旧游标——Talea SHALL 忽略旧高水位并回退一次完整发现，再写回携带新形态的游标，使得只写入新表但 `time_updated` 早于既有水位的会话不被永久跳过。形态一致时 SHALL 继续使用既有高水位与重叠窗口做增量发现。回退路径产生的无形态游标 SHALL 被容忍并在下一轮触发一次完整发现，不得形成每次同步都完整发现的死循环。

#### Scenario: Legacy cursor built before the storage migration

- **WHEN** 索引中保存的游标建立于上游把会话迁移到 `session_v2` 之前，且新表中存在 `time_updated` 早于该游标水位的会话
- **THEN** Talea SHALL 忽略该旧水位并执行一次完整发现，使这些会话进入索引

#### Scenario: Shape unchanged keeps incremental discovery

- **WHEN** 当前存储形态与游标记录的形态一致
- **THEN** Talea SHALL 继续按既有高水位与重叠窗口做增量发现，不退化为每次全量扫描

#### Scenario: Cursor without a shape does not loop

- **WHEN** 增量失败回退路径产生了一个不带形态字段的游标
- **THEN** 下一轮 SHALL 至多多做一次完整发现并写回带形态的游标，不得每次都完整发现

### Requirement: OpenCode message reading SHALL route by session data

Talea SHALL 按会话自身的数据选择消息读取路径：会话在传统 `message` 表存在消息时 SHALL 继续使用 `message`/`part`；不存在时 SHALL 使用 `session_message`。存量会话的首问、消息预览和时间线结果 SHALL 保持与既有实现一致。

#### Scenario: Legacy session keeps its message path

- **WHEN** 会话在 `message` 表存在消息，同时 `session_message` 中也存在不完整的对应记录
- **THEN** Talea SHALL 使用 `message`/`part` 读取，首问与时间线不得因投影表不完整而缺失

#### Scenario: New session with no legacy message rows

- **WHEN** 会话在 `message`/`part` 中没有任何行，但 `session_message` 中存在消息
- **THEN** Talea SHALL 从 `session_message` 读取首问与消息预览，并正常生成时间线

### Requirement: OpenCode v2 timeline SHALL preserve token data semantics

v2 时间线 SHALL 使用 `assistant.data.tokens` 作为单次增量并按增量聚合，会话级汇总 SHALL 使用 `session_v2.tokens_*` 累计值；两者 SHALL NOT 相加。v2 消息不存在上下文快照字段时，时间线的上下文快照与累计总量 SHALL 保持未知（空值）而不是 0。事件来源标识 SHALL 与 v1 路径可区分，避免 schema 切换前后重复计数。

#### Scenario: Per-message tokens are increments

- **WHEN** `session_message` 中的 assistant 消息带有 `tokens`
- **THEN** Talea SHALL 将其作为该次请求的增量计入时间线，且 SHALL NOT 与会话级 `tokens_*` 累计值相加

#### Scenario: Context snapshot is absent in v2

- **WHEN** v2 消息中不存在 `step-finish` 类型的上下文快照
- **THEN** 时间线事件的上下文快照与累计总量 SHALL 保持未知，而不是显示 0

#### Scenario: v1 and v2 sources do not double count

- **WHEN** 同一会话先后经过 v1 与 v2 两条读取路径
- **THEN** 事件来源标识 SHALL 使用不同前缀，索引 SHALL NOT 因前缀相同或路径切换重复累计

### Requirement: OpenCode access SHALL remain read-only and fault tolerant

所有 OpenCode 读取 SHALL 继续使用只读 SQLite 连接与 busy timeout；任一会话或任一行解析失败 SHALL NOT 影响其他会话，也 SHALL NOT 导致应用退出。恢复命令 SHALL 保持参数数组形式，不使用 shell。

#### Scenario: Single row parse failure

- **WHEN** 某条 `session_message.data` 不是合法 JSON
- **THEN** Talea SHALL 跳过该行并继续处理同一会话与其它会话

#### Scenario: Read-only connection

- **WHEN** Talea 读取 OpenCode 数据库
- **THEN** 连接 SHALL 为只读模式，且 SHALL NOT checkpoint 或修改该数据库
