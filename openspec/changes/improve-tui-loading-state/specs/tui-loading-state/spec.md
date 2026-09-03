## Purpose

为每次启动时的本地会话同步提供清晰、可信的阶段反馈，让用户知道 Talea 正在执行的工作以及何时可以使用会话列表。

## ADDED Requirements

### Requirement: TUI loading state shows actionable stages

在会话列表可用前，TUI SHALL 显示标题为 `Talea` 的加载状态卡，并按顺序显示三个阶段：检查本地 Agent、同步会话记录、准备会话列表。

#### Scenario: Initial loading state

- **WHEN** Talea 启动并开始同步本地数据
- **THEN** TUI 显示三个阶段，当前阶段为“检查本地 Agent”，其余阶段标记为待处理，并显示 spinner 和当前阶段说明

#### Scenario: Stage transition

- **WHEN** 一个同步阶段完成且下一个阶段开始
- **THEN** 已完成阶段显示完成标记、当前阶段显示进行中标记、后续阶段显示待处理标记，且主文案与当前阶段一致

#### Scenario: Synchronization completes

- **WHEN** 本地同步、搜索索引准备和列表加载完成
- **THEN** TUI 退出加载状态并显示会话列表，不再显示加载卡

### Requirement: Loading feedback is localized and accurate

加载态 SHALL 使用当前语言显示用户可理解的阶段和辅助文案；辅助文案不得暗示只有首次启动才会耗时，也不得显示无法由真实同步流程支持的百分比或数量进度。

#### Scenario: Chinese locale

- **WHEN** 当前界面语言为中文
- **THEN** 加载态显示中文阶段名称、当前阶段说明、数据量较大时可能需要一点时间的提示和退出提示

#### Scenario: English locale

- **WHEN** 当前界面语言为英文
- **THEN** 加载态显示对应的英文阶段名称、当前阶段说明、可能需要一点时间的提示和退出提示

### Requirement: Loading state preserves exit and failure feedback

加载期间 TUI SHALL 保持退出操作可用；同步失败时 SHALL 退出正常加载状态并显示明确的失败信息，而不是继续显示等待中的成功状态。

#### Scenario: User quits while loading

- **WHEN** 用户在任意加载阶段按下 `q` 或 `Ctrl+C`
- **THEN** TUI 退出，且加载态不触发恢复会话或打开详情操作

#### Scenario: Synchronization fails

- **WHEN** 任意真实同步阶段返回错误
- **THEN** TUI 显示同步失败信息和退出提示，并停止显示进行中的正常加载反馈
