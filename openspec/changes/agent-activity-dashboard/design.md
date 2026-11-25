# Design

## Context

See `proposal.md` for motivation and `specs/agent-status-dashboard/spec.md` for the behavior contract.

现有索引只保存 `activity_state`（进程/文件活跃度），适配器在 `ParseMetadata` 中已经顺序读取会话记录。Claude Code 使用 JSONL，Codex CLI 使用 rollout JSONL，OpenCode 使用只读 SQLite；三者都能从最近事件得到“回合”级状态，但只有 OpenCode 与 Codex 的部分事件是显式终态。Claude Code 的权限等待不写入会话记录，需要可选 hook 才能精确识别。

Talea 的硬约束：单二进制、无 cgo、无外部服务、不写 Agent 原始数据、核心层不能出现 `switch agent`。

## Goals / Non-Goals

**Goals:**

- 在不改变现有会话列表、搜索、恢复语义的前提下，增加回合状态存储与聚合。
- 提供常驻 TUI 看板、`status --json`、可选本地 hook 事件入口。
- 状态计算可解释：区分原生事件、推断、精确 hook，并保留原因。
- 单个 Agent/会话/事件失败不影响整体。

**Non-Goals:**

- 不启动、停止、steer 或编排 Agent。
- 不做 Web 看板、tmux pane 跳转、桌面/手机通知（首版）。
- 不做语义任务完成判断，也不调用 LLM 判断“任务是否完成”。
- 不自动安装/修改 Claude Code、Codex、OpenCode 的配置。
- 不读取或保存 Agent 的完整消息内容用于状态判断。

## Decisions

### 1. 回合状态放在独立表，而不是扩展 `sessions`

新增 `session_turn_state(agent_instance_id, session_id, turn_state, state_at, state_source, state_detail, updated_at)`，主键与会话一致。

理由：

- `sessions` 已被搜索、导出、同步、TUI 多处 SQL 读取；把状态列加进 `sessions` 需要改动所有 SELECT/INSERT/UPDATE 和导出格式。
- 回合状态是短生命周期派生数据，独立表可以按需批量加载、整表重建，失败时不影响会话主数据。
- 仍然使用同一 SQLite 文件、同一事务写入，保持单文件和一致性。

替代方案：直接给 `sessions` 加列。改动面更大，且导出/导入需要额外兼容处理；不采用。

### 2. 适配器输出通用回合状态，核心只做聚合

`model.Session` 增加 `TurnState`、`TurnStateAt`、`TurnStateSource`、`TurnStateDetail`。各适配器在自己的 `ParseMetadata` 中把最近事件映射为通用值：

- Claude Code：`assistant.stop_reason=end_turn` / `system.turn_duration` → completed；`tool_use` 或用户消息 → working；`isApiErrorMessage` → failed。
- Codex CLI：`task_started` → working；`task_complete` → completed；`turn_aborted` → interrupted/failed；审批请求事件 → waiting_input。
- OpenCode：最新 `session_message` 为 `idle` 时按 outcome 映射；最后为 `assistant`/`user` 时 → working；legacy message 路径按最后角色兜底。

核心 `internal/status` 只读取通用字段，不出现 Agent 分支。新增 Agent 时只需在适配器填入字段。

### 3. 状态由“回合状态 + 时间 + 精确事件”合成

`internal/status` 生成展示态：

- 精确 hook 事件时间不早于 `TurnStateAt` 时覆盖适配器状态。
- `working` 在安静阈值内保持“进行中”；超过阈值但仍在活跃窗口内显示“可能等待”。
- `completed` 在最近窗口内显示“已完成”，更早归入“空闲”。
- `failed` / `interrupted` 直接进入“失败或中断”。
- 未知状态不猜测为完成。

阈值使用具名常量并集中在 `internal/status`，避免散落魔法数字；首版默认：安静阈值 2 分钟、过期窗口 30 分钟、已完成展示窗口 24 小时。

替代方案：把阈值做成配置。首版不做，避免增加配置面和文档负担；后续有需要再加。

### 4. 精确事件使用本地 JSONL，不引入 daemon

新增隐藏的 `talea hook` 命令：从 stdin 读取 Claude Code hook 或 OpenCode plugin 的 JSON，映射成统一事件后追加到 `${DataDir}/status-events.jsonl`，文件 0600、目录 0700。看板读取文件尾部并在内存中保留每个会话的最新事件。

理由：

- 不需要常驻服务、socket 或网络端口。
- hook 进程只做一次短写，不阻塞 Agent。
- 写入失败时命令仍以 0 退出，避免影响 Agent。
- 不自动改配置；文档给出 Claude hooks 与 OpenCode plugin 的配置片段。

替代方案：本地 HTTP 服务或 Unix socket。需要 daemon 生命周期、端口/权限和更多故障模式；首版不采用。

### 5. 看板使用自定义 Bubble Tea 模型，不复用会话列表组件

现有 `list.Model` 不支持分组标题和两行状态卡片。看板使用扁平 `boardRow`（分组标题 + 会话行）和独立光标/滚动，复用现有 `detailModel`、样式与恢复流程。

刷新通过 `tea.Tick` 触发，使用现有 `syncer.Sync` 做增量索引；刷新中跳过重叠 tick，避免并发写索引。

替代方案：改造现有会话列表为多视图。会污染列表语义和测试；不采用。

## Risks / Trade-offs

- [Claude 权限等待无法零配置精确识别] → 显示“可能等待”并标注推断；提供可选 hooks。
- [回合完成被误读为任务完成] → 文案固定为“本轮结束/等待下一步”，并在文档中说明。
- [常驻刷新加重 I/O] → 只做增量同步；刷新中跳过；默认 3 秒；已完成旧会话折叠。
- [Hook 配置是外部写操作] → 不自动安装；只提供 `talea hook` 和文档片段，用户显式配置。
- [新表与旧库迁移] → `CREATE TABLE IF NOT EXISTS` 幂等迁移；旧库首次运行自动创建。
- [状态事件文件增长] → 读取尾部固定上限；事件量小；后续可加轮转。
- [Codex 审批事件版本差异] → 识别到事件才标精确等待，否则回退推断。

## Migration Plan

1. 启动时执行幂等迁移，新增 `session_turn_state` 表。
2. 现有会话在下次增量索引/解析时写入状态；未解析的旧会话显示未知，不影响使用。
3. 回滚只需停止使用新命令；旧表保留，不影响既有功能。
4. 不修改 Agent 数据，无需 Agent 侧迁移。

## Open Questions

（无）
