# tui-session-list Specification

## Purpose
让 TUI 会话列表遵守既有的子会话显示配置，并且不静默丢弃超出硬编码上限的会话，使用户看到的列表与 `talea list` 及索引真实内容一致。
## Requirements
### Requirement: TUI SHALL honor the subagent display configuration

TUI 会话列表 SHALL 遵守 `general.include_subagents` 配置，与 `talea list` 保持一致：默认（`false`）时 SHALL 隐藏 `is_subagent` 为真的会话，为 `true` 时 SHALL 显示全部会话。过滤 SHALL 在查询层完成，不得先取受限条数再丢弃，从而导致可展示的会话少于应展示数量。

#### Scenario: Subagents hidden by default

- **WHEN** 配置为默认值 `include_subagents = false`，且索引中存在子 Agent 会话
- **THEN** TUI 列表 SHALL NOT 显示子 Agent 会话，且 SHALL 与 `talea list` 的可见条目一致

#### Scenario: Subagents shown when enabled

- **WHEN** 配置为 `include_subagents = true`
- **THEN** TUI 列表 SHALL 显示子 Agent 会话，与 `talea list --include-subagents` 一致

#### Scenario: Other callers keep their behavior

- **WHEN** 为查询新增子会话排除能力
- **THEN** 既有调用方（`talea list`、`talea go`、搜索、同步）的行为 SHALL 保持不变

### Requirement: TUI list SHALL NOT silently truncate sessions

TUI 会话列表 SHALL NOT 因硬编码条数上限而静默丢弃更早的会话；应展示的会话 SHALL 全部可浏览，翻页由列表组件承担。若因性能原因确需保留上限，则上限 SHALL 可配置且 SHALL 在列表上给出可见的截断提示。

#### Scenario: More sessions than the former hard limit

- **WHEN** 索引中应展示的会话数超过原硬编码上限（500）
- **THEN** TUI SHALL 显示全部应展示的会话，最早的会话 SHALL 可通过翻页访问

#### Scenario: Full load does not degrade startup noticeably

- **WHEN** 全量加载当前规模的会话列表
- **THEN** 加载耗时 SHALL 与原上限口径持平（处于测量噪声内），且 SHALL NOT 引入新的配置项

