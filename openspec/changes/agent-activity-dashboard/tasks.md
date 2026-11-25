# Tasks

## 1. 状态模型与索引

- [x] 1.1 在 `model.Session` 增加回合状态、时间、来源、原因字段，并定义通用状态常量；运行 `go test ./internal/model`（无测试时以 `go build ./...` 验证编译）
- [x] 1.2 在索引迁移中新增幂等的 `session_turn_state` 表及索引；用临时数据库执行迁移并断言表存在
- [x] 1.3 在会话写入路径（批量 upsert、单条 upsert、插入、导入）同步写入回合状态；补充索引测试，验证写入、覆盖和旧会话无状态时的默认值
- [x] 1.4 增加批量加载回合状态的索引 API，并验证按 `(agent_instance_id, session_id)` 正确合并、缺失时返回未知

## 2. 适配器状态提取

- [x] 2.1 Claude Code 适配器识别 `end_turn`、`turn_duration`、`tool_use`、用户消息、API 错误；补充夹具测试覆盖完成、进行中、失败和未知
- [x] 2.2 Codex CLI 适配器识别 `task_started`、`task_complete`、`turn_aborted` 和可用的审批请求事件；补充夹具测试覆盖完成、进行中、中断/失败和未知
- [x] 2.3 OpenCode 适配器读取最新 `session_message`/legacy message 与 `idle_outcome`；补充 v2 与 legacy 测试覆盖完成、失败、中断、进行中和未知
- [x] 2.4 确认核心层没有新增 `switch agent` 分支，适配器测试全部通过

## 3. 状态聚合与精确事件

- [x] 3.1 实现 `internal/status`：合并回合状态、活动状态与时间阈值，产出展示态、是否需介入、来源和原因；单测覆盖各状态、阈值边界、已完成窗口和未知状态
- [x] 3.2 实现状态事件 JSONL 追加与尾部读取，保证目录 0700、文件 0600、坏行跳过、读取上限；单测覆盖权限、坏行和最新事件提取
- [x] 3.3 实现精确事件覆盖规则：事件不早于回合状态时间才生效；单测覆盖新事件覆盖、旧事件忽略和不同会话隔离
- [x] 3.4 验证没有事件文件/空文件/超大文件时状态聚合仍正常

## 4. 常驻 TUI 看板

- [x] 4.1 新增 `talea dashboard` 入口与常驻模型，接入增量同步、自动刷新、刷新去重和退出恢复流程；冒烟测试验证自动刷新与退出后恢复选择
- [x] 4.2 实现分组、排序、汇总计数、过滤和滚动；单测覆盖分组顺序、组内最近活动倒序、空数据与未知状态
- [x] 4.3 实现看板行渲染、状态颜色/图标/推断标识、窄宽终端截断；渲染测试断言关键状态与摘要可见
- [x] 4.4 接入现有详情页、Enter/恢复键和错误提示；测试验证详情打开、返回和单 Agent 失败不影响其他行

## 5. CLI 与文档

- [x] 5.1 新增 `talea status` 文本、`--short` 与 `--json` 输出；单测校验 JSON 字段、计数和短输出
- [x] 5.2 新增隐藏/低噪声的 `talea hook` 入口，映射 Claude Code hooks 与 OpenCode plugin 事件并写入状态事件文件；单测覆盖权限等待、完成、失败、坏输入时仍成功退出
- [x] 5.3 在 README 与 `docs/` 记录状态语义、`dashboard`/`status`/`hook` 用法、Claude hooks 与 OpenCode plugin 配置片段，并明确“本轮完成不等于任务完成”
- [x] 5.4 在 `talea doctor` 或帮助文本中补充可选 hooks 的发现/提示（如适用），确保不自动修改 Agent 配置

## 6. 集成验证

- [x] 6.1 使用临时索引库和合成会话跑通 `status --json`、`status --short`、dashboard 渲染；确认状态计数一致
- [x] 6.2 运行 `go vet ./...`、`go test ./...`，并完成 linux/amd64 与 linux/arm64 构建
- [x] 6.3 验证不修改 Agent 原始文件/数据库，状态事件文件权限为 0600；记录未验证项与环境限制
