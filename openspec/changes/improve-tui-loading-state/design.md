## Context

当前 TUI 在启动时通过后台命令调用 `syncer.Sync`，加载视图只复用正常列表标题并显示一个固定 spinner 文案。同步编排实际包含 Agent 探测、会话索引、搜索索引准备和活动状态刷新，但后台命令目前没有向 TUI 传递阶段信息。

## Goals / Non-Goals

**Goals:**

- 用固定高度的三行阶段卡表达真实同步进度。
- 让阶段更新从后台同步任务传回 Bubble Tea 主循环，避免直接并发修改 UI 模型。
- 保留现有 `Sync` 调用方和同步结果、错误语义。
- 对中文和英文提供自然且不暗示“首次加载”的文案。

**Non-Goals:**

- 不显示估算百分比、剩余时间或虚构的 Agent 级进度。
- 不改 Web 只读页面、列表页标题、索引数据口径或同步顺序。
- 不新增依赖或引入独立加载组件。

## Decisions

### Use coarse real stages instead of percentage progress

同步流程只提供阶段边界，没有稳定的总工作量，因而选择三个可验证的粗粒度阶段：Agent 探测、会话同步、列表准备。阶段开始时发送一次通知；完成后由下一阶段的通知或最终列表消息推进 UI。这样不会把不可知的会话数量伪装成百分比。

### Keep progress delivery inside the existing command flow

为索引器增加可选的阶段回调，在同步编排层映射为 TUI 使用的三阶段事件。TUI 创建 Bubble Tea program 后注入 `Program.Send`，后台同步通过它向主循环发送阶段消息；未注入回调时现有 CLI 和测试调用保持原有行为。

### Render a stable three-row card

加载视图固定显示标题、三行阶段、当前阶段说明和退出提示。行状态使用完成、进行中、待处理三种视觉标记；不改变 spinner，仅将其旁边的文案绑定到当前阶段。正常列表继续使用原来的 `Talea · Agent Sessions` 标题。

## Risks / Trade-offs

- [阶段信息是粗粒度的] → 不展示不可信的百分比；当前阶段文案明确说明正在执行的用户可理解工作。
- [后台任务可能在退出后继续运行] → 保持现有后台同步生命周期与退出行为不变，本次只让消息发送在程序终止后安全成为 no-op。
- [终端颜色能力不同] → 复用现有 AdaptiveColor 与 spinner，状态含义同时由符号和文字表达。
