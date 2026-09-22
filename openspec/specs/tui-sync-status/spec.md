# tui-sync-status Specification

## Purpose
在存在本地缓存索引时，让用户以醒目的单行状态条清楚感知后台同步正在进行、处于哪个真实阶段，以及同步完成后的实际变化结果。
## Requirements
### Requirement: Background sync status SHALL be visually prominent

存在缓存索引并执行后台同步时，TUI SHALL 在标题下方渲染背景高亮的单行状态条，以背景色块 + 加粗与普通列表文本区分，并显示真实同步阶段、spinner 与当前阶段说明；同步阶段 SHALL 以按真实阶段分段的进度条表达（已完成段 █、当前段 ▓、未到段 ░）。阶段语义 SHALL NOT 只依赖颜色，且 SHALL NOT 显示无法由真实同步流程支持的百分比或数量进度。

#### Scenario: Sync in progress

- **WHEN** Talea 启动后在后台同步会话索引
- **THEN** 顶部 SHALL 显示背景高亮的单行状态条，其中包含 spinner、按真实阶段分段的进度条（已完成 █、当前 ▓、未到 ░）与当前阶段说明，且视觉显著性高于普通列表文本

#### Scenario: Stage updates during sync

- **WHEN** 后台同步进入下一阶段
- **THEN** 状态条 SHALL 更新为新的当前阶段，进度条分段填充随阶段推进而变化（已完成段 █ 延伸、当前段显示 ▓、未到段保持 ░）；填充粒度 SHALL 等于阶段粒度，不做阶段内部分填充

#### Scenario: Status block does not overflow the list

- **WHEN** 同步状态条显示时
- **THEN** 会话列表可用高度 SHALL 按状态条实际渲染行数扣减，列表与底部快捷键提示不得被推出可视区域

#### Scenario: Narrow terminal degrades to a single line

- **WHEN** 状态条单行内容超出终端可用宽度（如 40 列）
- **THEN** 进行中状态 SHALL 按「spinner + 进度条 + 当前阶段说明（核心，进度条不参与裁剪、三段结构必须保留）→ 总标题 → 省略号截断」的优先级裁剪；核心在极窄档放不下时进度条可缩格，但三段结构 SHALL 保留、仍不做阶段内部分填充；完成/失败文案超宽时以省略号截断；任何档位 SHALL 保持单行，不换行、不横向溢出，且核心信息始终保留

### Requirement: Sync completion SHALL provide real feedback

后台同步完成后，TUI SHALL 显示基于真实会话计数差的完成提示：计数发生变化时显示变化数量，无变化时显示无变化；该提示 SHALL 在短暂展示后自动隐藏，不长期占用列表空间。

#### Scenario: Sessions were added or updated

- **WHEN** 后台同步后会话计数较同步前增加
- **THEN** 顶部 SHALL 显示包含真实变化数量的完成提示，并在短暂展示后自动隐藏

#### Scenario: Nothing changed

- **WHEN** 后台同步完成后会话计数没有变化
- **THEN** 顶部 SHALL 显示无变化的完成提示，不得显示虚假的数量增长

#### Scenario: Completion notice auto-hides

- **WHEN** 完成提示已经展示一段时间
- **THEN** 提示 SHALL 自动隐藏，列表高度随之恢复，且不残留占位空行

### Requirement: Sync failure SHALL remain distinct and visible

后台同步失败时，TUI SHALL 显示明确的失败提示并说明当前为缓存数据；失败提示 SHALL 与进行中状态和完成状态在文案与视觉上均可区分，并持续可见直到用户操作或下次同步。

#### Scenario: Sync fails with cached list

- **WHEN** 后台同步返回错误
- **THEN** 顶部 SHALL 显示失败提示与错误信息，且不得同时显示进行中的 spinner 或完成提示

