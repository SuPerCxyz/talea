## Why

本机 OpenCode 已升级到 2.0.12，上游把会话主表从 `session` 迁移到 `session_v2`，消息主表从 `message`/`part` 迁移到 `session_message`，且传统 `session`/`message`/`part` 三表自 2026-09-18 起不再写入。Talea 适配器仍只读传统三表，导致 2026-09-19 之后创建的 113 个 OpenCode 会话完全不被发现，已发现的 384 个会话也无法再读到新消息。实测证据：`session` 384 行（`max(time_updated)=1789700605279`）、`session_v2` 494 行（持续写入）、新会话在 `message`/`part` 中为 0 行而 `session_message` 有 2739 行；Talea 索引中 `opencode` 恰好 384 条，与冻结的传统表完全吻合。

同时，存在缓存索引时启动 TUI 只渲染一行灰色次要文案，后台同步进度和完成与否几乎不可感知。

## What Changes

- OpenCode 会话发现按能力探测选择表：存在 `session_v2` 时以它为主表，并补回只存在于传统 `session` 的遗留行（实测 3 条），避免漏读与重复计数；不存在时保持传统 `session` 路径。
- OpenCode 会话元数据优先读 `session_v2`（字段更完整、更新），缺失时回退 `session`；接受 `title` 等新增可空列。
- 消息读取按会话数据分流：会话在传统 `message` 表有数据则继续走 `message`/`part`，否则走 `session_message`，保证 384 个存量会话行为不变且新会话可读。
- `session_message` 支持首次提问提取、消息预览和 Token 时间线：`user.data.text` 提取首问，`assistant.data.content[]` 的 `text/reasoning/tool` 生成时间线事件，`assistant.data.tokens` 为单次增量按增量聚合，`assistant.data.model.id` 提供模型名，`type=compaction` 生成压缩事件。
- Token 口径保持分离：会话级用 `session_v2.tokens_*`（累计），消息级用 `session_message.data.tokens`（增量），两者不重复累计；v2 没有 `step-finish` 上下文快照，时间线的上下文快照字段 SHALL 保持不可用而不是 0。
- TUI 后台同步状态条改为醒目的边框状态块，展示真实同步阶段、spinner 与当前阶段说明。
- 同步完成后顶部状态条 SHALL 短暂显示新增/更新会话数量再自动隐藏，提供完成反馈。
- `talea doctor` SHALL 输出当前 OpenCode 数据库识别到的存储形态（v1 传统表 / v2 `session_v2` 主表），便于下次上游 schema 变更时快速定位。
- TUI 在 SQL 层遵守 `general.include_subagents`（默认 `false`），与 `talea list` 一致地隐藏子 Agent 会话；v2 上游数据含大量 `parent_id` 非空的子会话，此前被平铺显示。
- 移除 TUI 列表 500 条硬编码截断，改为全量加载并由 `list.Model` 分页承担，避免静默丢弃更早的会话（当前非子会话 548 条已超该上限）。
- 更新 `docs/formats/opencode.md` 至 2.0.12 实测结论。

## Capabilities

### New Capabilities

- `opencode-v2-storage-compat`: 保证 OpenCode 上游存储迁移后，会话发现、元数据、消息、时间线与恢复仍依据真实本地数据可用，且不违反只读与数据口径约束。
- `tui-sync-status`: 后台同步期间与完成后，向用户提供醒目、准确、可理解的状态反馈。
- `tui-session-list`: TUI 会话列表遵守子会话显示配置且不静默截断，使用户能看到全部应展示的会话。
- `doctor-diagnostics`: `talea doctor` 报告 Agent 本地存储形态，帮助定位上游 schema 变更。

### Modified Capabilities

无。`codex-latest-resume-opencode-v2` 已定义的路径解析与可空元数据要求保持有效，本 change 不修改其安全边界；`improve-tui-loading-state` 定义的首屏加载卡行为保持不变，本 change 只作用于有缓存时的后台同步状态条。

## Impact

- 影响 `internal/adapters/opencode` 的会话发现、元数据解析、消息加载与时间线事件生成及其测试。
- 影响 `internal/tui` 的后台同步状态渲染、完成反馈、列表高度计算与列表加载及其测试。
- 影响 `internal/search` 的查询字段（子会话排除与不限制条数），现有调用方行为不变。
- 影响 `internal/doctor` 的 OpenCode 存储形态报告及其测试。
- 更新 `docs/formats/opencode.md` 与 README 环境事实。
- 不新增依赖，不读取或写入 Agent 原始数据（数据库保持 `mode=ro`），不改变恢复命令的参数数组与 `syscall.Exec` 安全边界，不修改 Claude/Codex 适配器。
