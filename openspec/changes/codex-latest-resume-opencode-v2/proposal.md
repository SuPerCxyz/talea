## Why

Codex 会话可以在创建后通过 `codex resume <id> --yolo` 切换到更高权限的运行策略，Talea 恢复时必须使用会话中最后一个已验证策略，而不是早期策略。与此同时，当前本机已升级到 OpenCode v2，适配器仍只按旧版固定路径和可空字段假设读取数据库，导致自定义数据库位置及部分合法 v2 会话存在兼容风险。

## What Changes

- 增加 Codex“普通策略 → 后续 `resume --yolo`”的回归覆盖，确保最新有效 `turn_context` 覆盖旧策略并沿用现有白名单参数。
- OpenCode 通过 v2 CLI 的数据库路径解析能力读取默认或 `OPENCODE_DB` 指定的数据库，同时保留旧版固定路径回退。
- OpenCode 适配器兼容 v2 `session` 表中的可空字段，并将默认零值 Token 保守识别为未知，避免把未知显示成精确的 0。
- 增加 OpenCode v2 兼容性测试并更新版本、路径、表结构和恢复命令文档。

## Capabilities

### New Capabilities

- `agent-runtime-compatibility`: 保证 Codex 最新运行策略和 OpenCode v2 本地数据源可以被安全索引与恢复。

### Modified Capabilities

无。已有 `resume-launch-args` change 已定义恢复参数白名单；本 change 增加其时序回归覆盖，不改变参数安全边界。

## Impact

- 影响 `internal/adapters/codex`、`internal/adapters/opencode` 及其测试。
- 更新 OpenCode/Codex 格式文档和 README 中的环境事实。
- 不新增依赖，不修改 Agent 原始文件或数据库，不改变恢复的参数数组和 `syscall.Exec` 安全边界。
