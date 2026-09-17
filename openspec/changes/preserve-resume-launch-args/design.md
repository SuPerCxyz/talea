## Context

当前 `Session` 已有未持久化的恢复相关字段，但索引数据库不会保存恢复参数；Claude、Codex 和 OpenCode 适配器分别构造固定的恢复命令。只读调查确认 Codex `turn_context` 包含 `approval_policy` / `sandbox_policy`，Claude JSONL 包含 `permission-mode` 记录；OpenCode 的已验证 `session` 表没有原始启动参数或权限模式字段。

本设计只读取 Agent 来源，扩展 Talea 自己的索引 schema 和适配器恢复逻辑。权限参数是高风险输入，必须经过适配器白名单和参数数组边界控制。

## Goals / Non-Goals

**Goals:**

- 保存并恢复 Codex、Claude Code 当前格式中可验证的有效权限模式。
- 让 TUI、`talea go` 和 dry-run 共享同一份持久化恢复参数。
- 保持旧索引、未知字段和未支持 Agent 的默认恢复行为。
- 允许未来适配器增加自己的恢复参数提供能力，不修改核心 Agent 分支。

**Non-Goals:**

- 不还原无法从来源确认的任意原始 shell 命令行、环境变量、profile、插件或用户自定义参数。
- 不为 OpenCode 猜测 `--auto`；除非未来格式调查确认可靠字段，否则保持 `opencode -s <id>`。
- 不修改 Agent 原始文件/数据库，不引入启动参数配置界面。

## Decisions

### 1. 增加独立的 ResumeLaunchArgs 字段

在统一 `Session` 模型中增加 `ResumeLaunchArgs []string`，语义为“恢复命令的额外参数”，不包含程序名、会话 ID、`resume` 子命令或 `--resume` 基础参数。它与已有 generic 适配器使用的 `ResumeProgram/ResumeArgs` 分离，避免改变外部适配器兼容行为。

索引数据库新增 `resume_launch_args_json` 列，保存 JSON 数组；旧行默认空数组。搜索查询、TUI 和 CLI 读取同一字段，恢复入口不再重新解析 Agent 原始文件。

### 2. 适配器负责解析、白名单和命令顺序

不把 Agent 参数解析放入核心索引器。Claude/Codex 的 `ParseMetadata` 在扫描结构化记录时得到规范化参数；各自的 `BuildResumeCommand` 再执行最终白名单校验并放置参数：

- Codex 的全局选项放在 `resume <id>` 之前；`never + danger-full-access` 规范化为当前 CLI 的 `--dangerously-bypass-approvals-and-sandbox`，部分策略分别使用 `--ask-for-approval` / `--sandbox`。
- Claude 的 `--permission-mode <mode>` 放在 `--resume <id>` 之前，只接受 CLI 已确认的 permission mode 枚举。
- OpenCode 和未提供能力的未来 Agent 保持原命令。

解析使用最近有效的结构化策略记录，未知值清空对应部分而不是降级为高权限。恢复时再次校验持久化数组，防止旧库或人工修改的索引内容绕过白名单。

### 3. 用可选适配器能力支持未来 Agent

新增可选的恢复参数提供接口或等价适配器能力，核心只消费 `Session.ResumeLaunchArgs`。未来 Agent 可以在自己的适配器中从已确认字段生成参数；核心索引和 TUI/CLI 恢复路径无需新增 Agent 条件分支。

替代方案是在 `resume.Build` 或 CLI 中按 Agent ID 添加 `switch`，实现快但违反可扩展架构，也会让未知 Agent 的权限行为散落在核心，不采用。

### 4. 通过本地 schema 迁移兼容旧索引

将 Talea 索引 schema 版本提升到 3，沿用已有版本备份和幂等迁移；新列默认 `[]`。迁移不触碰 Agent 数据。旧索引第一次升级后，已有会话没有恢复参数，直到下一次适配器重新解析来源才可能获得新字段。

## Risks / Trade-offs

- [恢复特权参数会重新启用高权限] → 只接受结构化白名单字段；未知、缺失或无法验证时不添加参数。
- [Agent CLI 选项和会话格式未来变化] → 参数映射集中在适配器，格式文档和适配器测试随版本调查更新。
- [旧索引缺少恢复参数] → schema 默认空数组，保留现有恢复命令；重新索引后逐步补齐。
- [Codex 原始 `--yolo` 文本未必保存] → 恢复当前等价的规范化 CLI 参数，而不是声称还原原始拼写。
- [OpenCode 没有已验证权限字段] → 不猜测 `--auto` 或其他特权参数，保留基础恢复行为。

## Migration Plan

1. 迁移 Talea 索引 schema，增加 `resume_launch_args_json`，旧行填充空数组并生成版本备份。
2. 新索引或发生源文件变化时，Claude/Codex 解析并保存规范化恢复参数。
3. 恢复命令从索引读取参数，经适配器白名单校验后执行；dry-run 显示相同参数。
4. 若迁移或参数解析失败，保留旧索引和无额外参数的默认恢复路径。

## Open Questions

无。OpenCode 权限参数是否可恢复已按当前已验证 schema 保守处理；未来格式调查可以单独增加适配器能力。
