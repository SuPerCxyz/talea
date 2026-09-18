## Context

现有 Codex 适配器已经从 rollout JSONL 的 `turn_context` 读取白名单策略，并将最后一次解析到的有效值写入 `Session.ResumeLaunchArgs`；索引和恢复入口也已经持久化、复用该字段。本 change 主要补齐时序回归覆盖，并使 OpenCode 适配器适配本机已验证的 v2 CLI/SQLite 行为。

OpenCode v2 仍保留 `session`、`message`、`part` 传统数据表，同时增加 `session_message` 等投影表。适配器继续使用传统表读取消息和 Token 时间线，避免在新旧投影之间重复累计；只对 v2 的路径解析和可空元数据做兼容补强。

## Goals / Non-Goals

**Goals:**

- 锁定 Codex 最新有效策略覆盖旧策略的恢复行为。
- 使用 OpenCode v2 自己解析出的数据库路径，同时兼容旧版固定路径。
- 让合法 NULL 元数据和 v2 默认 Token 零值不导致解析失败或虚假精确值。
- 保持 Agent 数据库只读、参数数组执行和单 Agent 容错。

**Non-Goals:**

- 不从会话正文、环境变量或 shell history 恢复任意原始启动参数。
- 不将 OpenCode `session_message` 投影表与传统消息表合并，也不恢复无法验证来源的 `--auto`。
- 不修改 OpenCode 数据库 schema、WAL 或任何 Agent 原始数据。

## Decisions

### 1. Codex 保持“顺序扫描、最后有效值胜出”

JSONL 是追加记录，现有顺序扫描已经能表达“最后一次启动策略”。只增加包含两个策略阶段的回归测试，不引入按时间排序或原始命令行解析，避免把会话正文误当作权限来源。

### 2. OpenCode 路径采用显式覆盖、CLI 探测、旧路径回退

先处理 `OPENCODE_DB`，使测试和无路径子命令的旧版本可用；未覆盖时调用 `opencode debug paths db` 获取 v2 的真实路径；命令失败时回退到 `<DataDirectory>/opencode.db`。所有外部命令继续使用参数数组，不使用 shell。

替代方案是仅拼接固定路径，改动更小但无法支持 OpenCode v2 文档承诺的数据库覆盖和 release channel 路径，因此不采用。

### 3. 用 `sql.NullString` 接受可空字段，按非零值判断 Token 已知性

OpenCode v2 的 `path`、`agent`、`model` 允许 NULL，适配器扫描后以空字符串进入统一模型。v2 Token 列的默认 0 不能单独证明 Agent 已报告用量，因此仅当任一 Token 字段大于 0 时设置 `HasTokenUsage` 和汇总。

替代方案是把 NULL 扫描错误传播出去或把全部 0 当作有效用量，前者损坏单会话容错，后者违反未知值与精确零值的项目口径。

### 4. 用脱敏 SQLite 夹具覆盖 v2 额外表

测试夹具增加 v2 投影表和 NULL 元数据，但仍只验证传统表解析；这样可以证明额外 schema 不破坏现有读取，而不把真实数据库或用户内容带入仓库。

## Risks / Trade-offs

- [CLI 路径命令启动外部进程增加少量探测成本] → 仅在 OpenCode 发现阶段调用，并在失败时快速回退；恢复仍不启动额外探测。
- [全部 Token 为 0 但 Agent 确实报告了零用量] → 保守显示未知，符合当前“缺失不显示 0”的数据口径。
- [未来 OpenCode 移除传统表] → 当前适配器会保留旧索引并报告解析错误；届时单独调查 v2 投影语义，不在本 change 猜测迁移。

## Migration Plan

无需迁移 Agent 数据库。Talea 自有索引在下一次同步时按来源路径和元数据重新解析；旧索引没有恢复参数时继续沿用既有默认恢复命令。
