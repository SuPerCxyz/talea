# Proposal

## Why

Talea 目前只能判断 Agent 进程是否存在、会话文件是否刚更新，无法回答“哪个会话正在工作、本轮是否结束、是否在等我确认、是否失败”。同时运行多个 Claude Code / Codex / OpenCode 窗口时，用户需要在多个终端之间来回确认，容易漏掉等待权限或已经结束的任务。

## What Changes

- 新增只读状态聚合能力：从各 Agent 已有会话记录中提取“本轮状态”（进行中 / 等待输入 / 已完成 / 失败 / 中断 / 空闲 / 未知），并标注精确或推断来源。
- 新增 `talea dashboard` 常驻 TUI 看板：自动增量刷新，按“需要你 / 进行中 / 已完成 / 失败或中断 / 空闲”分组，展示 Agent、项目、任务、最近活动、Token 与会话 ID，并可进入现有详情页或恢复会话。
- 新增 `talea status` 一次性状态查询，支持文本与 `--json`，供 tmux 状态栏、脚本或后续 Web 看板复用。
- 新增可选 `talea hook` 事件入口：Claude Code hooks 与 OpenCode plugin 可把权限等待、提问等待、结束、失败等精确事件写入 Talea 自有状态文件；不自动修改任何 Agent 配置。
- 新增索引侧回合状态存储，不改变既有 `activity_state`、会话列表排序、搜索与恢复行为。
- 明确状态语义：没有任务级完成信号时，“已完成”表示本轮结束并等待下一步，不声称语义任务完成。

## Capabilities

### New Capabilities

- `agent-status-dashboard`: 聚合各 Agent 会话的回合状态、判断是否需要用户介入，并通过常驻 TUI 看板、`status --json` 和可选 hook 事件呈现。

### Modified Capabilities

（无）

## Impact

- 数据模型：`model.Session` 增加回合状态字段；索引新增 `session_turn_state` 表，保持旧库可幂等迁移。
- 适配器：Claude Code、Codex CLI、OpenCode 在解析元数据时提取最后回合状态；核心层不按 Agent 分支。
- TUI：新增 dashboard 视图，复用现有详情页与恢复流程；默认不改变原会话列表入口。
- CLI：新增 `dashboard`、`status`、`hook` 命令。
- 本地数据：新增 `status-events.jsonl`（权限 0600，目录 0700），仅由用户显式配置的 hooks/plugin 写入。
- 不新增网络、遥测、外部服务或运行时依赖；不修改 Agent 原始会话文件与数据库。
