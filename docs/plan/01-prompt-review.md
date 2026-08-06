# 提示词检查报告

对项目规格（Talea 完整需求）进行逐段检查，记录不一致、风险、优先级冲突与执行建议。

## 1. 检查结论总览

| 维度 | 结论 |
|------|------|
| 可行性 | 高，规格完整、自洽，可落地 |
| 规模 | 大（P0 约 30 项，P1 约 14 项，P2 约 10 项），必须分阶段交付 |
| 主要风险 | 三个 Agent 格式差异大、SQLite 只读访问与增量去重、超大数据库性能 |
| 需修正点 | 4 处与真实环境不一致，2 处内部矛盾，1 处依赖选型需澄清 |
| 必须先做 | 只读格式调查（已完成）→ 计划（本文）→ 骨架 + 三适配器 + 测试 |

## 2. 与真实环境不一致的假设（已验证修正）

### 2.1 版本号（实测）

| Agent | 规格未指定 | 实测版本 | 备注 |
|-------|-----------|---------|------|
| Claude Code | — | 2.1.223 | `~/.npm-global/bin/claude` |
| Codex CLI | — | 0.146.0 | `~/.npm-global/bin/codex` |
| OpenCode | — | 1.18.14 | `~/.npm-global/bin/opencode` |

版本号必须写入 `AdapterInfo.Version`，并在 `docs/formats/*.md` 中记录「已验证版本」。

### 2.2 数据目录（实测）

| Agent | 实测目录 | 结构 |
|-------|---------|------|
| Claude Code | `~/.claude/projects/<encoded-cwd>/<sessionId>.jsonl` | JSONL，每行一条记录；子会话在 `<sessionId>/subagents/*.jsonl` |
| Codex CLI | `~/.codex/sessions/<YYYY>/<MM>/<DD>/rollout-*.jsonl` | JSONL，首行为 session_meta |
| OpenCode | `~/.local/share/opencode/opencode.db` | SQLite（约 6GB + 578MB WAL） |

**风险修正**：规格第 9 条「会话 ID 位置」等调查项已经实测确认，详见 `docs/formats/*.md`。

### 2.3 OpenCode 数据库体积

规格未提示体积量级。实测 `opencode.db` 约 6GB，WAL 约 578MB。这直接影响：

- 只读打开必须是唯一方案（已要求）；
- 禁止整库复制；
- 增量索引必须基于 `time_updated` / mtime 而非全表扫描；
- busy timeout 与 WAL 快照读是必须项。

### 2.4 `sqlite3` CLI 缺失

开发机未安装 `sqlite3` 命令行工具。P0 调查阶段用 Python `sqlite3`（只读模式）完成。最终工具本体零 Python 依赖，仅作为调查手段。

## 3. 内部矛盾与需澄清项

### 3.1 `talea go` 的 `--cwd` 语义（规格 §25 vs §30）

- §25 `talea open --agent claude-code <session-id>`：进入会话原目录。
- §30 错误示例提示 `talea open 8f463a2e --cwd /home/user/code/cinder`：`--cwd` 覆盖原目录。
- **命令已合并**：`talea open` 并入 `talea go`（`--cwd`/`--dry-run`；session id 前缀自动匹配，无需指定 agent）。

**决定**：`--cwd` 表示「显式目标目录」，优先级高于会话原目录与路径映射。此为规格自然解读，写入 CLI 文档。

### 3.2 FTS5 与 `--metadata-only`

§29 元数据模式「不保存完整对话正文」与 §20 全文搜索「用户消息匹配」存在张力。FTS5 默认只索引 `first_question` 等元数据字段；`index_assistant_messages` / `index_user_messages` 配置项决定是否索引正文。`--metadata-only` 时不写 FTS5 正文索引。

### 3.3 活动状态判定（§12）

规格允许多种依据但未给优先级。**决定**：以进程存在（PID + 可执行路径校验）为最高证据，其次文件持续更新（需要更新间隔阈值），最后会话元数据状态。纯 mtime 不能长期判 active。

### 3.4 依赖选型需澄清

- **CLI 框架**：规格要求「轻量、成熟的 Go CLI 框架」。候选：`cobra`（成熟、生态大）、`urfave/cli`（轻量）、`kong`（声明式）。**建议 cobra**，生态与子命令、flag 支持最完整；P0 仅需有限子命令。
- **SQLite 驱动**：规格要求单二进制、无外部数据库。`mattn/go-sqlite3` 需要 cgo；`modernc.org/sqlite` 纯 Go、无需 CGO、交叉编译 amd64/arm64 更稳。**建议 modernc.org/sqlite**，但需验证其对 WAL + 只读模式的支持（P0 骨架阶段验证）。
- **图表绘制**：规格要求终端图表。纯 ASCII/Unicode 块字符自绘即可，不引入 chart 库。

## 4. 范围与优先级检查

### 4.1 P0 范围偏大但可行

P0 共 30 项。其中 Token 模型（27/28 项）仅要求「数据模型预留」，即定义类型、schema、不要求完整时间线实现——这合理，降低 P0 风险。

### 4.2 明确不在 P0 实现（规格 §35）

云同步、用户账号、在线服务、多租户、远程上传均不实现。**检查通过**，无越界风险。

### 4.3 `talea run`（§36）归属

规格将其列为「后续提供」但未标注 P0/P1。建议归入 P1（因为依赖 PID 包装、信号转发、索引关联，非读取会话必要路径）。

## 5. 核心原则可实现性检查

| 原则 | 可实现性 | 关键实现点 |
|------|---------|-----------|
| 只读 | 高 | OpenCode 用 `mode=ro` URI；Claude/Codex 只读打开文件 |
| 单 Agent 失败不影响其他 | 高 | 适配器隔离 + 错误聚合 |
| 首次提问准确 | 中 | 需过滤 system/instructions/environment_context 块（Codex 已实测 AGENTS.md 注入） |
| Token 去重 | 中 | 需 `source_identity` 去重 + delta/cumulative 识别 |
| 原生恢复 | 高 | `claude --resume <id>` / `codex resume <id>` / `opencode -s <id>` 均已验证 |
| 权限 0600 | 高 | 索引文件 chmod |

**首次提问风险**：Codex 实测首条 user 消息包含 `AGENTS.md instructions` 与 `<environment_context>` 注入块，必须在提取时剥离，否则首次提问被污染。

## 6. 验收标准可行性

| 验收项 | 可行性 | 备注 |
|--------|--------|------|
| `talea doctor` 识别 Agent | 高 | 三 Agent 均已安装 |
| `talea list` 展示字段 | 高 | 格式实测可提取 |
| `talea search` 跨 Agent | 高 | FTS5 可实现 |
| `talea go --dry-run` | 高 | 恢复命令已验证（`talea open` 已并入 go） |
| 中文显示/搜索 | 中 | FTS5 默认 tokenizer 对中文不理想，需验证 `unicode61` 或自定义方案 |
| 单损坏文件不退出 | 高 | 逐行容错 |
| 权限 0600 | 高 | 显式 chmod |
| go vet / golangci-lint / amd64+arm64 | 高 | 常规工程 |

**中文搜索风险**：FTS5 的默认 tokenizer 按空格/标点切分，中文连续串可能被整体切分。需在 P0 验证 `unicode61` 或是否使用 `trigram` tokenizer（`--tokenize trigram`，SQLite 3.34+）。此为 P0 技术验证项之一。

## 7. 结论

- 规格总体完整、可实现，**没有需要推翻的根本缺陷**；
- 修正了 4 处与真实环境不一致的假设；
- 明确 3 处内部矛盾的处理决定；
- 指定 SQLite 驱动与 CLI 框架选型；
- 将 `talea run` 归入 P1；
- 将「中文全文搜索 tokenizer」「modernc 只读 WAL 支持」列为 P0 前期技术验证项。

风险排序（按影响）：OpenCode 数据库体积/锁 > Token 去重正确性 > 中文 FTS5 > 首次提问污染 > 活动状态误判。

下一步：按 `docs/plan/02-implementation-plan.md` 执行。
