## Purpose

让 Talea 恢复会话时复用来源中明确记录的权限和运行模式，同时对未知或不可信参数保持保守，支持未来 Agent 通过适配器扩展相同能力。

## ADDED Requirements

### Requirement: Resuming a session SHALL reuse its verified launch arguments

当会话来源中存在适配器确认过的恢复参数时，Talea SHALL 将这些参数持久化到自己的索引，并在 TUI、`talea go` 和 `--dry-run` 的恢复命令中使用相同的参数数组。没有恢复参数的旧会话 SHALL 保持现有默认命令。

#### Scenario: Indexed session has verified launch arguments

- **WHEN** 会话已索引出非空的白名单恢复参数并执行恢复
- **THEN** 生成的 Agent 命令包含这些参数，且参数顺序和参数边界保持为独立数组项

#### Scenario: Legacy session has no launch arguments

- **WHEN** 旧索引或会话来源没有恢复参数
- **THEN** Talea 使用现有固定恢复命令，不因缺少字段而失败

#### Scenario: Dry run shows restored arguments

- **WHEN** 用户使用 `talea go <session> --dry-run`
- **THEN** 输出的参数包含实际恢复时会传递的已验证参数

### Requirement: Codex policy records SHALL map to current safe command-line options

Talea SHALL 从 Codex `turn_context` 中读取最近有效的 `approval_policy` 与 `sandbox_policy.type`，只接受已知枚举值。`approval_policy=never` 与 `sandbox_policy.type=danger-full-access` 同时存在时，Talea SHALL 使用当前 Codex CLI 的等价特权参数；单独存在时 SHALL 分别恢复为对应的审批或沙箱参数。

#### Scenario: Codex full privilege mode

- **WHEN** 最近有效 Codex 上下文为 `approval_policy=never` 且沙箱为 `danger-full-access`
- **THEN** 恢复命令包含 `--dangerously-bypass-approvals-and-sandbox`，并位于 `resume` 子命令之前

#### Scenario: Codex partial policy

- **WHEN** Codex 只记录了已知审批策略或已知沙箱策略
- **THEN** 恢复命令只添加对应的 `--ask-for-approval` 或 `--sandbox` 参数，不猜测另一项

#### Scenario: Codex policy is unknown

- **WHEN** Codex 策略字段缺失或取值不在白名单内
- **THEN** Talea 忽略该字段，不自动添加特权参数，并保留默认恢复行为

### Requirement: Claude Code permission records SHALL map to explicit permission modes

Talea SHALL 从 Claude Code `permission-mode` 记录读取最近有效且属于 CLI 已知枚举的模式，并通过 `--permission-mode <mode>` 恢复。`bypassPermissions` 等高权限模式只有在原始记录明确存在时才可恢复。

#### Scenario: Claude bypass mode

- **WHEN** 最近有效 Claude Code 权限模式为 `bypassPermissions`
- **THEN** 恢复命令包含 `--permission-mode bypassPermissions`

#### Scenario: Claude ordinary mode

- **WHEN** 最近有效 Claude Code 权限模式为 `acceptEdits`、`auto`、`manual`、`dontAsk` 或 `plan`
- **THEN** 恢复命令包含对应的显式 permission mode

#### Scenario: Claude mode is unknown

- **WHEN** 权限模式缺失或不在白名单内
- **THEN** Talea 不添加权限参数并使用默认恢复行为

### Requirement: Agent-specific launch argument support SHALL be extensible and conservative

恢复参数解析和构造 SHALL 封装在 Agent 适配器能力中，核心恢复流程不得硬编码 Agent 三选一分支。未实现该能力的 Agent（包括当前无法从来源确认权限参数的 OpenCode） SHALL 继续使用基础恢复命令；未来适配器可以提供自己的白名单参数映射。

#### Scenario: Agent without launch-argument provider

- **WHEN** Agent 适配器未提供可验证恢复参数
- **THEN** Talea 使用该 Agent 的现有恢复命令，不影响恢复功能

#### Scenario: Future Agent provider

- **WHEN** 新 Agent 适配器提供恢复参数解析和白名单构造能力
- **THEN** 核心索引和恢复流程可以持久化并使用该参数，而无需新增 Agent 条件分支

### Requirement: Restored arguments SHALL preserve safety boundaries

Talea SHALL 只持久化适配器生成的白名单参数，禁止将任意原始命令行、会话正文、环境变量或未验证字段直接作为恢复参数。恢复 SHALL 继续通过参数数组执行，不使用 Shell 拼接。

#### Scenario: Untrusted source text contains flags

- **WHEN** 会话正文或未知 JSON 字段中出现类似命令行参数文本
- **THEN** Talea 不将其写入或传给恢复进程

#### Scenario: Privileged mode is explicitly recorded

- **WHEN** 来源结构化字段明确记录了已支持的特权模式
- **THEN** Talea 可以恢复该模式，但不扩大为来源未声明的其他权限
