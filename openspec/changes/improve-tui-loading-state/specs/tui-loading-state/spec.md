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

### Requirement: Loading card makes the current stage visually legible

加载态 SHALL 使用清晰的视觉层级呈现三阶段：当前阶段必须比已完成和待处理阶段更醒目；加载卡片 SHALL 在宽终端中保持可读的最大宽度，在窄终端中收缩到可用宽度且不产生横向溢出。阶段语义不得只依赖颜色。

#### Scenario: Active stage is emphasized

- **WHEN** 当前同步阶段为任意一个阶段
- **THEN** 当前阶段 SHALL 使用加粗和高对比视觉样式，并保留明确的进行中标记；已完成和待处理阶段 SHALL 保持可区分

#### Scenario: Loading card fits a narrow terminal

- **WHEN** 终端宽度小于加载卡片的默认可读宽度
- **THEN** 加载卡片 SHALL 收缩到终端可用宽度，阶段名称、当前说明和退出提示不得横向溢出

#### Scenario: Wide terminal avoids an oversized text block

- **WHEN** 终端宽度明显大于加载文案所需宽度
- **THEN** 加载卡片 SHALL 保持受控的最大宽度并居中，避免文字被拉伸到难以阅读的超宽区域

#### Scenario: Loading title is centered and prominent

- **WHEN** 加载卡片显示正常同步状态或同步失败状态
- **THEN** `Talea` 标题 SHALL 在卡片内容区域水平居中，并使用比辅助文案更醒目的标题样式

#### Scenario: Loading title uses a clean hierarchy

- **WHEN** 加载卡片显示正常同步状态或同步失败状态
- **THEN** 加载标题 SHALL 使用居中的单行 `Talea`，通过加粗、高对比度和留白突出显示，且不得使用块状字模或下划线装饰

### Requirement: Keyboard hints remain legible across terminal widths

会话列表底部的快捷键提示 SHALL 清晰区分按键与操作说明；当一行无法容纳全部提示时 SHALL 自动换行，不能静默截断有效快捷键。

#### Scenario: Wide terminal shows all keyboard hints

- **WHEN** 会话列表在宽度足够的终端中显示
- **THEN** 底部提示 SHALL 显示 `enter`、`d`、`o`、`esc`、`t` 和 `q` 的完整按键及说明，并使用清晰的视觉层级和分隔

#### Scenario: Narrow terminal wraps keyboard hints

- **WHEN** 终端宽度不足以容纳全部快捷键提示
- **THEN** 底部提示 SHALL 按多行换行显示全部快捷键，不得因为默认帮助组件的宽度截断而隐藏后续操作

### Requirement: Loading state preserves exit and failure feedback

加载期间 TUI SHALL 保持退出操作可用；同步失败时 SHALL 退出正常加载状态并显示明确的失败信息，而不是继续显示等待中的成功状态。

#### Scenario: User quits while loading

- **WHEN** 用户在任意加载阶段按下 `q` 或 `Ctrl+C`
- **THEN** TUI 退出，且加载态不触发恢复会话或打开详情操作

#### Scenario: Synchronization fails

- **WHEN** 任意真实同步阶段返回错误
- **THEN** TUI 显示同步失败信息和退出提示，并停止显示进行中的正常加载反馈
