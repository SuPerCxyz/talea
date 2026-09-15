## Purpose

让 Talea TUI 在不同终端列宽下保持可读、无横向溢出，并利用宽屏展示更完整的会话文本。

## ADDED Requirements

### Requirement: Session list keeps fixed metadata and adapts question text

会话列表 SHALL 保持 Agent、Start、End、Time、Token、Cache、Path、Branch 元数据字段的固定列宽和对齐方式；首次提问与最近消息 SHALL 按当前列表可用宽度显示，不得再受固定 100 字符上限限制。

#### Scenario: Wide terminal shows more question text

- **WHEN** TUI 在宽于默认列表内容宽度的终端中显示一条超过 100 个字符的问题
- **THEN** 问题文本 SHALL 在列表可用范围内显示超过 100 个字符的内容，且元数据行的固定字段位置不变

#### Scenario: Narrow terminal prevents horizontal overflow

- **WHEN** TUI 终端宽度不足以显示完整问题或最近消息
- **THEN** 每行文本 SHALL 按终端显示宽度安全截断并显示省略号，不得超出列表视口

#### Scenario: CJK text uses display width

- **WHEN** 问题或最近消息包含中文等双宽字符
- **THEN** 截断结果 SHALL 按终端显示列宽计算，不得因字节数或字符数计算错误而溢出

#### Scenario: Window resize updates list text

- **WHEN** 用户在 TUI 运行期间改变终端窗口宽度
- **THEN** 列表问题文本 SHALL 使用新的可用宽度重新渲染，并保持当前选择、过滤和恢复操作可用

### Requirement: Detail tables fit the current viewport

详情页的模型汇总、用户轮次和子 Agent 会话行 SHALL 根据当前 viewport 宽度调整可变文本列，并在空间不足时安全截断；详情页已有的图表宽度适配行为 SHALL 保持不变。

#### Scenario: Model summary expands on a wide viewport

- **WHEN** 详情页 viewport 宽度增加
- **THEN** 模型名称列 SHALL 使用可用剩余空间，数字列保持可读且整行不超过 viewport 宽度

#### Scenario: Turns and sub-agent rows fit a narrow viewport

- **WHEN** 详情页 viewport 宽度不足以显示完整提问或子 Agent 信息
- **THEN** 可变文本列 SHALL 按显示宽度截断，整行 SHALL 不超过 viewport 宽度

#### Scenario: Detail content reflows after resize

- **WHEN** 用户在详情页改变终端窗口宽度
- **THEN** 模型汇总、用户轮次和子 Agent 行 SHALL 使用新宽度重新生成，且滚动视图继续可用
