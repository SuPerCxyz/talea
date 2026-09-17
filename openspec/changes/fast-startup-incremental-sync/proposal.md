## Why

每次启动 Talea 都会先执行同步，再显示会话列表；即使 Agent 数据没有变化，也会重复发现来源、刷新活动状态和准备索引。用户需要的是基于 Talea 本地索引立即可用，并在后台只检查和同步可能变化的来源，同时继续保持 Agent 原始数据只读。

## What Changes

- TUI 启动时优先读取已有 Talea 索引并显示缓存列表；首次无缓存时保留现有加载态。
- 在 Talea 索引库保存本地来源扫描状态，使用 OpenCode 数据库/WAL 与 Claude/Codex 已知来源文件和目录的指纹判断是否需要重新发现。
- 为 OpenCode 保存 `(time_updated, session_id)` 游标，只查询变化候选；游标缺失或不可靠时保守回退全量发现。
- 活动状态只更新状态实际变化的会话，避免重复写入整张 `sessions` 表。
- 缓存列表读取使用已索引的目录与 Git 字段，避免启动时重复执行无必要的文件和 Git 检查；无变化时跳过不必要的 FTS 同步。
- 后台同步完成后刷新列表，保留选择状态；同步失败时保留旧列表并显示明确的过期/失败反馈。
- 增加扫描状态、游标、TUI 后台刷新、活动状态幂等和回退路径的定向测试。

## Capabilities

### New Capabilities

- `fast-startup-incremental-sync`: 基于 Talea 本地索引的快速启动、来源变化检测和后台增量同步。

### Modified Capabilities

无。

## Impact

- 影响 `internal/tui` 的启动与后台刷新状态、`internal/syncer` 的同步编排、`internal/index` 的扫描状态与迁移、`internal/adapters/opencode` 的增量发现、`internal/search` 的 FTS 同步条件，以及相关测试。
- 只新增或迁移 Talea 自己的索引状态；不修改 Claude/Codex 会话文件或 OpenCode 数据库，不新增网络、常驻服务或外部依赖。
- `talea list`、`talea go` 的同步语义保持不变；本次快速缓存优先行为限定在默认 TUI 启动路径。
