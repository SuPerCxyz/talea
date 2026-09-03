## Why

每次启动 `talea` 都会执行本地会话同步，但当前加载态沿用正常列表页标题 `Talea · Agent Sessions`，用户无法直观看出当前正在进行的工作。需要让等待期间的状态与真实同步阶段对应，并保持加载期间的退出反馈。

## What Changes

- 将 TUI 加载态改为固定的三阶段状态卡：检查本地 Agent、同步会话记录、准备会话列表。
- 以完成、进行中、待处理三种状态动态更新阶段行，并保留 spinner。
- 将主文案改为当前阶段的用户可理解描述，移除“首次加载”表述。
- 同步提供阶段进度回调；默认 `Sync` 调用保持兼容。
- 增加加载态阶段切换及失败反馈的定向测试。

## Capabilities

### New Capabilities

- `tui-loading-state`: 为 TUI 启动同步提供清晰、动态且与真实工作阶段一致的加载反馈。

### Modified Capabilities

无。

## Impact

- 影响 `internal/tui` 的加载态渲染与消息处理。
- 影响 `internal/syncer` 与 `internal/index` 的内部阶段通知，不改变同步结果或错误语义。
- 不新增依赖，不修改 Web 只读页面，不读取或写入 Agent 原始数据方式。
