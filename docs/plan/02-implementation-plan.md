# Talea 实施计划

Talea — Trace the session. Resume the work.

本计划基于真实环境只读调查（见 `docs/formats/*.md`）与提示词检查（见 `docs/plan/01-prompt-review.md`）制定。

## 0. 环境调查摘要（已实测）

| 项 | 值 |
|----|-----|
| Go | 1.26.0 linux/amd64 |
| Claude Code | 2.1.216，数据 `~/.claude/projects/<enc-cwd>/<id>.jsonl` |
| Codex CLI | 0.146.0，数据 `~/.codex/sessions/<Y>/<M>/<D>/rollout-*.jsonl` |
| OpenCode | 1.18.13，数据 `~/.local/share/opencode/opencode.db`（SQLite ~6GB） |
| XDG | 全空 → 使用默认路径 |
| 恢复命令 | `claude --resume <id>` / `codex resume <id>` / `opencode run -s <id>` |
| sqlite3 CLI | 未安装（调查用 Python 只读完成，项目本体零 Python 依赖） |

关键实测发现：

1. **Claude Code**：JSONL，`cwd`、`sessionId`、`gitBranch`、ISO8601 时间戳在每条记录顶层；首条 user 消息为 string 内容。
2. **Codex CLI**：首行 `session_meta` 含 `cwd`、`git`、`model_provider`、`cli_version`；user 消息前会注入 AGENTS.md `<INSTRUCTIONS>` 与 `<environment_context>` 块（必须过滤）；`token_count` 事件含 total/last usage。
3. **OpenCode**：SQLite。`session` 表含 directory/path/title/tokens_*/time_*/parent_id；`message`/`part` 表含 JSON 数据，part 的 `step-finish` 携带 tokens；user 消息正文在 `part.text`。
4. **Token 口径风险（必须去重）**：OpenCode `session` 表 tokens 为汇总；`step-finish` tokens 为分步值；Claude usage 在 assistant 消息内；Codex token_count 事件累计。三者都是「累加易错」的口径。

## 1. 项目骨架

```
talea/
├── cmd/talea/main.go          # 入口
├── internal/                   # 全部业务代码
│   ├── model/                  # Session / TokenUsage / Timeline 模型
│   ├── adapters/               # adapter.go, registry.go, claude/, codex/, opencode/
│   ├── config/                 # TOML 配置
│   ├── index/                  # SQLite schema + 迁移 + 增量索引
│   ├── search/                 # FTS5 搜索
│   ├── resume/                 # 恢复命令构造 + 路径映射
│   ├── security/               # 脱敏、ANSI 清理、路径安全
│   ├── timeline/               # 时间线聚合、桶、轮次
│   ├── usage/                  # Token 汇总/去重
│   ├── doctor/                 # 环境诊断
│   ├── preview/                # 对话预览
│   ├── app/                    # 业务编排
│   ├── cli/                    # cobra 命令
│   ├── tui/                    # Bubble Tea 界面
│   └── version/                # 版本信息
├── migrations/                 # SQL 迁移文件（幂等）
├── docs/                       # 文档（本目录）
├── testdata/                   # 脱敏测试夹具
├── scripts/
├── .github/workflows/
├── .golangci.yml
├── .goreleaser.yml
├── go.mod / go.sum
├── LICENSE
├── README.md
└── AGENTS.md
```

模块边界要求：核心逻辑与 TUI 分离；解析器（adapters）与存储层（index）分离；时间线/usage 仅依赖模型，不依赖具体 Agent。

## 2. 技术选型

| 领域 | 选择 | 理由 |
|------|------|------|
| TUI | `charmbracelet/bubbletea` + `bubbles` + `lipgloss` | 规格指定 |
| SQLite | `modernc.org/sqlite`（纯 Go，无 cgo） | 单二进制、cross-compile amd64/arm64、只读 URI 支持 |
| 全文搜索 | SQLite FTS5（`unicode61`，验证中文，必要时 `trigram`） | 无外部依赖 |
| 文件监听 | `fsnotify` | 可选优化，非必需 |
| 配置 | TOML | 规格指定 |
| CLI | `spf13/cobra` | 成熟、生态完整 |
| 构建发布 | GoReleaser | 规格指定 |
| Lint | golangci-lint | 规格指定 |

P0 前期两个技术验证点（做骨架时一并完成）：
1. `modernc.org/sqlite` 只读打开 + WAL 数据库（OpenCode 场景）是否正常，busy timeout 行为；
2. FTS5 对中文连续文本的匹配效果，确定 tokenizer。

## 3. 实施阶段

### Phase 0：技术验证 + 骨架

- [x] 验证 modernc sqlite 只读 + WAL + busy
- [x] 验证 FTS5 中文搜索（trigram，见 §5 风险清单）
- [x] go mod init，引入依赖
- [x] `cmd/talea/main.go` + version 包
- [x] `internal/model/*.go`：Session、TokenUsage、TimelineEvent、ActivityState、TimeSource 等（规格 §8/§13/§14）
- [x] `internal/config`：默认配置 + TOML 加载 + validate
- [x] `talea version` 可运行

### Phase 1：适配器与解析（核心）

- [x] `internal/adapters/adapter.go`：AdapterInfo、AgentInstance、能力声明、可选接口（§7）
- [x] `internal/adapters/registry.go`：注册表，无 switch 硬编码
- [x] `internal/adapters/claude`：Discover/ParseMetadata/LoadMessages/LoadUsage/Resume
- [x] `internal/adapters/codex`：同上，含 AGENTS.md 注入过滤
- [x] `internal/adapters/opencode`：只读 SQLite，session/message/part
- [x] 首次提问提取器（`internal/adapters` 共享函数 + 各适配器定制）：§9 全部场景
- [x] 时间提取：StartTime/EndTime 优先级（§10）
- [x] 工作目录提取 + Git 信息（§11）
- [x] 测试夹具 `testdata/{claude,codex,opencode}/*`（脱敏）
- [x] 适配器单元测试 + 集成测试

### Phase 2：存储与索引

- [x] `internal/index/schema.go`：三张表 + 索引（§18）
- [x] `internal/index/migrate.go`：schema version、幂等迁移、WAL、备份
- [x] `internal/index/open.go`：0600 权限、单实例写
- [x] `internal/index/incremental.go`：偏移断点续读（source_offset 持久化 +
      LastCompleteLineOffset 检测截断/尾行暂存，mtime+size 变化触发重解析）
- [x] `internal/index/usage.go`：session_usage 表写入 + source_identity 去重
- [x] `talea index` / `talea index --rebuild` / `--metadata-only`
- [x] 增量索引测试：追加、截断、替换、删除、同 ID 跨 Agent（incremental_test.go）

### Phase 3：搜索

- [x] `internal/search`：FTS5 建表 + 权重（§20）+ 过滤参数
- [x] `talea search "kw" --agent --cwd --since`
- [x] 中文搜索测试

### Phase 4：CLI 命令面

- [x] `talea list`（--agent/--cwd/--today/--active/--include-subagents/--sort/--limit/--format）
- [x] `talea open`（含 --dry-run、--cwd 覆盖、路径映射、目录缺失交互）
- [x] `talea usage` / `talea timeline`（P1 前置的 CLI 骨架）
- [x] `talea last` / `talea config path|init|validate`
- [x] `talea doctor`（骨架 → Phase 6 完善）
- [x] 退出码规范（§26）
- [x] JSON/table/csv/markdown 输出

### Phase 5：恢复 + 目录缺失

- [x] `internal/resume`：构造命令（参数数组）、`syscall.Exec`、LookPath
- [x] 路径映射最长前缀匹配（`[path_mapping]`）
- [x] 原目录不存在交互：映射/当前目录/仅查看/复制命令/取消
- [x] 命令注入安全测试（恶意 session id / cwd / 引号 / 元字符 / ANSI 注入）

### Phase 6：Doctor

- [x] 三 Agent 可执行文件/版本/数据目录/格式/会话数/能力检查
- [x] 索引状态、FTS5、0600 权限
- [x] `--json` / `--agent`
- [x] 警告分级与聚合

### Phase 7：TUI

- [x] 会话列表（列动态隐藏、中文宽度、排序、搜索过滤）
- [x] 详情页对话预览（p 键，按需加载 + ANSI 清理 + 脱敏）
- [x] Enter 恢复流程
- [x] 活动状态、进行中标识
- [x] Token 时间线页（P1 完整；P0 预留快捷键）

### Phase 8：P1 — Token 汇总与时间线（已完成）

- [x] `internal/usage`：三 Agent 汇总、delta/cumulative 去重、峰值、子 Agent
- [x] `internal/timeline`：事件模型、请求/轮次/上下文/压缩、时间桶、图表聚合
- [x] `talea usage --details` / `talea timeline --group-by --bucket --around-peak`
- [x] CSV/JSON/Markdown 导出
- [x] 费用估算（`[usage]` 配置，默认关，整数微计价单位）
- [x] 本地规则 Token 洞察（`internal/insights`）
- [x] 按模型汇总 / 上下文曲线 / 压缩检测
- [x] `talea run`（PID 包装启动）
- [x] 新增 generic JSONL 适配器作为扩展模板

### Phase 9：质量收尾（已完成）

- [x] golangci-lint 配置 + 全绿（v2.12.2，0 issues）
- [x] go vet
- [x] 性能测试（1000/10000 会话、10 万事件）
- [x] 安全测试组
- [x] GitHub Actions CI（lint + test + build amd64/arm64）
- [x] GoReleaser 配置
- [x] README、privacy、token 文档
- [x] `talea doctor` 自检（在真实环境验证三 Agent）

### P1 增强与 P2（已完成）

- [x] 活动状态检测（/proc 进程 + 文件更新时间，`list --active` 生效）
- [x] 目录缺失交互处理（open 五选项，非 TTY 自动取消）
- [x] search/list 过滤补全（--project / --branch）
- [x] doctor 增强（索引格式/Token 完整性/FTS/路径映射冲突）
- [x] 会话标签 / 收藏 / 备注（`talea tag`）
- [x] 外部适配器协议（`talea-adapter-<name>` JSON stdio，
      7 方法：info/detect/discover/parse/messages/usage/timeline）
- [x] `talea run` 真实进程时间回写（process_start/process_exit）
- [x] 本地 Web 只读视图（`talea web`，仅 127.0.0.1）
- [x] 多设备离线导入导出（`talea export` / `talea import`）
- [x] `talea watch`：fsnotify 监听数据目录 + 事件合并增量索引（可选，非必需）

## 4. 关键设计决定（已确定）

1. **Agent 识别**：`AgentID` 为开放字符串类型，注册表驱动，核心代码无 `switch agent`。
2. **实例区分**：主键 `(agent_instance_id, session_id)`，支持同 Agent 多数据目录。
3. **Token 区分**：`*int64` 空值表示「未知」，绝不当 0；`UsageValueMode` 区分 delta/cumulative；`source_identity` 去重。
4. **首次提问**：独立提取器 + 过滤注入块 + 来源/置信度记录。
5. **恢复**：参数数组 + `syscall.Exec` 替换进程，无嵌套 shell。
6. **数据库权限**：索引文件创建即 `0600`，目录 0700。
7. **容错**：单会话/单文件错误聚合上报，不中断全局。

## 5. 未确认格式（风险清单）

| 项 | 状态 |
|----|------|
| Claude input_tokens 累计语义 | **已实测确认**（2026-08-05）：累计上下文，output 增量 |
| Codex `total_token_usage` 累计语义 | **已实测确认**：total=累计 / last=增量 |
| OpenCode WAL 未 checkpoint 数据读可见性 | **已实测确认**：只读连接可见 |
| OpenCode 子 session / 父 time_updated 覆盖 | **已实测确认**：27 个父子会话，父会话时间覆盖子会话 |
| OpenCode step-finish total 语义 | **已实测确认**：累计上下文快照 |
| Claude subagent usage 口径 | **已实测确认**：与主会话结构一致 |
| Claude 消息 id 字段在各版本的一致性 | 需更多版本样本 |
| FTS5 中文 tokenizer 行为 | Phase 0 验证（trigram 3 字 + LIKE 短词兜底） |

## 6. 验收对照

- P0 完成：`doctor` / `list` / `search` / `open --dry-run` / TUI Enter 恢复，全绿。
- P1 完成：`usage` / `timeline` 全字段展示。

## 7. 风险与缓解

| 风险 | 缓解 |
|------|------|
| OpenCode 6GB 库只读扫描慢 | time_updated 增量 + 只读快照 + 索引缓存 |
| Token 口径去重出错 | source_identity + 单测覆盖累加场景 |
| 中文搜索差 | trigram tokenizer 备选 + 搜索测试 |
| 首次提问被注入块污染 | 过滤规则 + 夹具测试 |
| 活动状态误判 | 多证据加权，低置信降级 |

## 8. 执行说明

每个阶段完成：运行相关测试 → 汇报新增文件、设计决定、未确认格式、风险。最终交付可编译、可运行、可测试、可发布的完整仓库。
