# Spec Delta

## Purpose

让用户在单个常驻看板中看到所有受支持 Agent 会话的当前回合状态，快速判断哪些正在工作、哪些已经结束、哪些在等待用户介入或已经失败。

## ADDED Requirements

### Requirement: 回合状态提取

系统 SHALL 从各 Agent 的既有会话记录中提取最近一次回合状态，并标记状态来源为原生事件、推断或未知。状态取值 SHALL 至少覆盖：进行中、等待输入、已完成、失败、中断、空闲、未知。

#### Scenario: Claude Code 回合完成

- **WHEN** Claude Code 会话的最后一条有效事件为 `assistant.stop_reason=end_turn` 或 `system.turn_duration`
- **THEN** 系统 SHALL 将该会话标记为已完成

#### Scenario: Claude Code 回合进行中

- **WHEN** Claude Code 会话的最后一条有效事件为用户消息或带 `tool_use` 的助手消息
- **THEN** 系统 SHALL 将该会话标记为进行中

#### Scenario: Codex 回合完成或中断

- **WHEN** Codex 会话记录中出现 `task_complete` 或 `turn_aborted`
- **THEN** 系统 SHALL 分别标记为已完成或中断/失败

#### Scenario: OpenCode 回合完成或失败

- **WHEN** OpenCode 会话最新的状态消息为 `idle`，其 outcome 为 `succeeded`、`failed` 或 `interrupted`
- **THEN** 系统 SHALL 分别标记为已完成、失败或中断

#### Scenario: 无法判断

- **WHEN** 会话没有可识别的回合事件
- **THEN** 系统 SHALL 标记为未知，不得推断为已完成

### Requirement: 是否需要用户介入

系统 SHALL 区分“需要你介入”“进行中”“已完成”“失败或中断”“空闲”五类展示状态。等待输入、失败、中断 SHALL 视为需要介入；进行中超过配置的安静阈值且仍可能运行时，系统 SHALL 标记为“可能等待”，并明确该状态为推断而非确定。

#### Scenario: 精确等待事件

- **WHEN** 收到 Claude Code 的权限/通知 hook 或 OpenCode 的权限等待事件
- **THEN** 系统 SHALL 将该会话标记为等待输入，且来源为精确事件

#### Scenario: 长时间无输出

- **WHEN** 会话状态为进行中，且超过安静阈值没有新的回合事件
- **THEN** 系统 SHALL 标记为可能等待，并在看板中显示推断标识

#### Scenario: 本轮完成不等于任务完成

- **WHEN** 系统将会话标记为已完成
- **THEN** 展示文案 SHALL 表达“本轮结束/等待下一步”，不得声称语义任务已完成

### Requirement: 常驻看板

系统 SHALL 提供 `talea dashboard` 常驻 TUI 看板，自动增量刷新，按状态分组展示会话，并在顶部展示各状态汇总计数。看板 SHALL 默认按最近活动倒序排列，用户 SHALL 能进入现有会话详情页或恢复选中会话。

#### Scenario: 自动刷新

- **WHEN** 看板运行且底层会话记录发生变化
- **THEN** 看板 SHALL 在下一次刷新周期内更新对应状态，无需用户重启

#### Scenario: 分组与计数

- **WHEN** 看板加载多个状态的会话
- **THEN** 看板 SHALL 按需要介入、进行中、已完成、失败或中断、空闲分组，并显示各组数量

#### Scenario: 状态不可用时保留数据

- **WHEN** 某次刷新中某个 Agent 的数据源读取失败
- **THEN** 看板 SHALL 保留其他 Agent 的状态，并显示该数据源的错误提示，不得整体退出

#### Scenario: 进入详情与恢复

- **WHEN** 用户选中会话并按 Enter 或恢复键
- **THEN** 系统 SHALL 打开现有详情页或执行现有恢复流程

### Requirement: 一次性状态输出

系统 SHALL 提供 `talea status` 命令输出当前状态汇总，并 SHALL 支持 `--json` 输出稳定结构，供 tmux 状态栏、脚本或后续界面复用。JSON SHALL 包含生成时间、各状态计数和每个会话的状态、来源、原因、Agent、会话 ID、工作目录与最近活动时间。

#### Scenario: JSON 输出

- **WHEN** 用户执行 `talea status --json`
- **THEN** 系统 SHALL 输出包含汇总与逐会话状态的合法 JSON

#### Scenario: 简短输出

- **WHEN** 用户执行 `talea status --short`
- **THEN** 系统 SHALL 输出适合状态栏的单行汇总

### Requirement: 可选精确事件

系统 SHALL 接受用户显式配置的 Claude Code hooks 与 OpenCode plugin 写入的本地状态事件，用于精确识别权限等待、提问等待、结束与失败。事件 SHALL 写入 Talea 自有数据目录，权限为 0600，目录为 0700；系统 SHALL NOT 自动修改任何 Agent 配置或会话文件。

#### Scenario: 较新的精确事件优先

- **WHEN** 精确事件时间不早于会话记录中的回合状态时间
- **THEN** 系统 SHALL 使用精确事件覆盖推断状态

#### Scenario: 旧事件不覆盖新状态

- **WHEN** 精确事件时间早于会话记录中更新的回合状态
- **THEN** 系统 SHALL 忽略该事件

#### Scenario: Hook 写入失败不影响 Agent

- **WHEN** `talea hook` 无法解析输入或写入事件文件
- **THEN** 命令 SHALL 以成功状态退出，不得阻塞或中断调用它的 Agent

### Requirement: 只读与容错

本能力 SHALL NOT 修改、重命名或删除任何 Agent 原始会话文件或数据库。单个 Agent、单个会话或单个事件损坏 SHALL NOT 导致看板或状态命令退出。

#### Scenario: 单个数据源失败

- **WHEN** 某个 Agent 的数据源不可读
- **THEN** 其他 Agent 的状态仍 SHALL 正常展示

#### Scenario: 损坏事件行

- **WHEN** 状态事件文件中存在无法解析的行
- **THEN** 系统 SHALL 跳过该行并继续处理后续事件
