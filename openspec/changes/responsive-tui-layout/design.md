## Context

列表模型已经把终端宽度传给 Bubble Tea 的 list delegate，delegate 会在最终渲染时按视口截断文本；当前问题是 `itemDesc` 在此之前把首次提问和最近消息固定截断为 100 个字符。列表元数据行的固定列宽是既有对齐约束，不能改为自适应列。

详情页通过 viewport 保存聚合内容，窗口变化时会使内容缓存失效，但模型汇总、用户轮次和子 Agent 行仍直接使用固定格式化宽度。

## Goals / Non-Goals

**Goals:**

- 移除列表问题文本的 100 字符预截断，让现有 list delegate 负责按当前终端宽度裁剪。
- 使用显示列宽而不是字节数或 rune 数来布局详情页可变文本列。
- 在宽屏增加可读文本空间，在窄屏保证行宽不超过 viewport。
- 保持元数据字段、数据口径、交互按键和现有图表行为不变。

**Non-Goals:**

- 不改变列表元数据字段的固定宽度。
- 不修改 CLI 表格、Web 页面、数据库或 Agent 原始数据读取方式。
- 不增加横向滚动、字体缩放或新的外部依赖。

## Decisions

### Let the existing list delegate own list clipping

仅移除 `itemDesc` 的固定 100 字符上限，不引入按窗口重建列表项的额外状态。Bubble Tea list delegate 已按当前 list 宽度对每个描述行截断，因此窗口 resize 后自然使用新宽度，同时避免破坏过滤状态和当前选择。

### Keep metadata widths stable

保留现有 `itemTitle`/`metaLine` 固定字段宽度，确保不同会话的 Start、End、Time、Token、Cache、Path、Branch 纵向对齐。宽度变化只影响问题文本行。

### Allocate detail table remainder to text columns

详情页模型汇总保留数字列的最小可读宽度，将剩余空间分配给模型列；用户轮次和子 Agent 行保留时间、序号和 ID 等固定部分，将剩余空间分配给问题文本。所有动态列均使用 `runewidth` 计算并在不足时省略。

### Test rendered plain widths

测试使用无 ANSI 的渲染结果和 `runewidth.StringWidth` 检查宽屏增长、窄屏不溢出、中文双宽字符及 resize 后内容变化，避免只验证内部字段或函数调用。

## Risks / Trade-offs

- [极窄终端无法同时展示所有详情字段] → 保留固定字段的最小宽度，并对可变文本做省略；不引入横向滚动。
- [详情表格的数字值可能超过预设宽度] → 使用足够的最小数字列宽，并在测试中覆盖较大值，避免动态列计算产生负宽度。
- [列表 delegate 的截断发生在样式渲染前后可能受 ANSI 影响] → 断言最终列表视图的显示宽度，并复用 Bubble Tea 已有 delegate 行为。
