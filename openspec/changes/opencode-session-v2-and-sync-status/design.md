## Context

OpenCode 2.0.12 引入 `session_v2` 与 `session_message` 作为新的写入目标，传统 `session`/`message`/`part` 冻结在 2026-09-18。实测同一数据库内三种形态并存：

- `session_v2` 494 行 ⊃ `session` 384 行，仅 3 个会话只存在于传统表（`message` 中有数据但未进入 `session_v2`）。
- 113 个仅存在于 `session_v2` 的会话在 `message`/`part` 中为 0 行，消息全在 `session_message`。
- 传统表会话在 `session_message` 中的覆盖不完整且不等价（样例 `message=734` 对 `session_message=377`），因此不能整体改用 `session_message`。

Talea 现有适配器的 11 处 SQL 全部硬编码传统表，`docs/formats/opencode.md` 的验证基线为 v2.0.7（当时 `session_v2` 尚不存在）。

## Goals / Non-Goals

**Goals:**

- 让 2026-09-19 之后的 OpenCode 会话可被发现、展示、搜索、查看时间线与恢复。
- 让 384 个存量会话的行为与索引结果保持不变。
- 保持 Agent 数据库只读、单会话容错、未知值不显示为 0、不重复累计 Token。
- 让后台同步的进行与完成可被用户感知。

**Non-Goals:**

- 不读取 `event` 表（事件溯源日志，与 `session_message` 冗余）。
- 不合并子会话 Token 到父会话，不合并两套消息投影。
- 不执行任何 OpenCode 数据迁移或写入，不修改其 schema。
- 不改变首屏无缓存时的加载卡行为，不改 Claude/Codex 适配器。

## Decisions

### 1. 用表存在性做能力探测，而不是按版本号分支

通过 `sqlite_master` 是否存在 `session_v2` 决定发现查询形态，而不是解析 `opencode --version`。理由：版本号与实际 schema 可能因 release channel、降级或数据迁移不一致，表存在性是数据库自身的事实；同时兼容尚未出现 `session_v2` 的旧库与未来可能移除传统表的库。

替代方案是按语义版本号判断，被否决：版本探测依赖外部进程且无法反映真实 schema。

### 2. 发现查询以 `session_v2` 为主并补回遗留传统行

存在 `session_v2` 时使用带 `UNION` 的查询：`session_v2` 全量，加上不在 `session_v2` 中的 `session` 行。`UNION` 天然按 `(id)` 去重，避免同一会话被发现两次；增量路径在子查询外层施加 `time_updated >= ?` 高水位过滤，保持现有游标语义与 5 分钟重叠窗口不变。

替代方案是只读 `session_v2`，被否决：会静默丢失实测存在的 3 个传统遗留会话，违反单会话不丢失的容错要求。

### 3. 消息路径按“会话在传统 `message` 表是否有数据”分流

以数据本身而非表存在性分流：会话在 `message` 表有行则走 `message`/`part`，否则走 `session_message`。这样 384 个存量会话继续走已验证路径（其 `session_message` 覆盖不完整），113 个新会话走新路径，且未来若上游恢复写传统表可自动跟随。

替代方案是按 `session_v2` 表存在性统一切换，被否决：存量会话的 `session_message` 数据不完整，会导致时间线和首问内容缺失。

### 4. Token 语义按来源分层，v2 时间线不伪造上下文快照

会话级汇总读 `session_v2.tokens_*`（会话累计），消息级时间线读 `assistant.data.tokens`（单次增量）并按增量聚合，两者永不相加。v1 通过 `step-finish.tokens.total` 提供上下文快照；v2 消息中不存在该字段，因此时间线的 `ContextAfter`/`CumulativeTotal` 保持空（未知），而不是写 0。

理由：项目数据口径要求精确值/估算值/未知值严格区分，缺失显示未知而非 0。

### 5. 幂等来源标识沿用既有前缀并为 v2 消息单独命名

v2 时间线事件的 `SourceIdentity` 使用 `opencode-v2msg:`、`opencode-v2tool:`、`opencode-v2comp:` 前缀，与 v1 的 `opencode-msg:`/`opencode-tool:`/`opencode-compact:` 区分，避免同一会话在 schema 切换前后产生跨前缀重复或碰撞。

### 6. 后台同步状态条复用真实阶段，完成反馈由索引前后计数差得出

状态条显示 `syncer` 已有的三个真实阶段，不引入虚假百分比；完成反馈在同步前后分别读取会话计数，差值为 0 时显示“无变化”，非 0 时显示新增/更新数量，并在短暂展示后自动隐藏，避免长期占用一行。

理由：与 `improve-tui-loading-state` 已确认的“不显示无法由真实同步流程支持的百分比或数量进度”约束一致——数量来自真实计数差，阶段来自真实回调。

## Risks / Trade-offs

- [上游再次变更表名] → 已用表存在性探测兜底，`talea doctor` 会报告存储形态，可快速定位；探测失败时回退传统表并保留旧索引，不导致应用退出。
- [`UNION` 查询在 9.9GB 库上的开销] → 仅对 `id`、`time_updated`、`title` 三列做去重，`session_v2` 有 `PRIMARY KEY` 与 `time_updated` 可用索引路径；增量路径仍走高水位过滤。
- [存量会话 `session_message` 数据不完整] → 已按数据分流规避，存量会话不走新路径。
- [v2 时间线缺少上下文快照会导致图表与 v1 不可比] → 明确标记为未知，优于伪造 0；在文档中说明差异。
- [完成反馈依赖会话计数，计数查询增加一次索引读] → 只在同步完成时执行一次，且读 Talea 自有索引而非 Agent 数据库。

## Migration Plan

无需迁移 Agent 数据库。Talea 自有索引在下一次同步时通过既有增量游标重新发现：游标基于 `session_v2` 与传统表并集的 `(time_updated, session_id)`，旧游标仍有效（时间戳语义不变）；若旧游标导致漏读，游标缺失或无效时已有回退到完整发现的路径。存量会话索引结果不变。

## Open Questions

无阻塞项。`session_v2.fork_session_id`/`fork_boundary`、`time_suspended` 等新增列是否影响会话语义，留待后续按真实需求单独调查，本 change 不消费这些列。
