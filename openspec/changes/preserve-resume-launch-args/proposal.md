## Why

Talea 当前恢复会话时只使用固定的 Agent 命令，丢失了原会话的权限和运行模式。例如以 Codex 特权模式启动的会话，恢复后可能回到默认审批/沙箱策略，导致行为不一致甚至无法继续工作。会话来源中已经存在部分可验证的有效模式信息，应由 Talea 保存并在恢复时安全复用。

## What Changes

- 为会话索引持久化经过适配器白名单过滤的恢复参数，而不是复制任意原始命令行或消息内容。
- Codex 从 `turn_context` 恢复 `approval_policy` 与 `sandbox_policy`，将特权组合转换为当前 CLI 支持的等价参数。
- Claude Code 从 `permission-mode` 记录恢复已验证的权限模式。
- 恢复命令构造统一使用持久化参数，覆盖 TUI、`talea go` 和 `--dry-run`。
- 为未来 Agent 增加可选的恢复参数提供能力；没有可验证参数的 Agent 保持现有默认恢复命令。
- 老索引和缺少模式信息的会话继续使用默认参数；未知字段不得自动扩大权限。
- 增加解析、持久化、迁移、命令顺序和安全白名单测试，并更新格式文档。

## Capabilities

### New Capabilities

- `resume-launch-args`: 在恢复会话时复用来源中可验证且经过白名单过滤的 Agent 启动参数。

### Modified Capabilities

无。

## Impact

- 影响 `internal/model`、`internal/index` 的会话字段与迁移、Claude/Codex 适配器解析、`internal/adapters.Resumer` 能力、恢复命令构造及相关 CLI/TUI 测试。
- 只读取 Agent 原始会话数据，不修改 Agent 文件或数据库；恢复仍通过参数数组和 `syscall.Exec` 执行。
- 不新增依赖，不改变没有可验证启动参数的会话恢复行为。
