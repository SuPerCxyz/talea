# Talea — AGENTS.md

Talea 是一个面向 Linux 的本地优先 AI Coding Agent 会话索引、搜索、预览、Token 分析和恢复工具。

标语：Trace the session. Resume the work.（循迹会话，续写未完。）

## 项目约束（必须始终遵守）

1. **只读原则**：任何情况下不得修改/重命名/删除 Agent 原始会话文件或数据库。OpenCode 数据库只读打开，Claude/Codex 会话文件只读解析。
2. **隐私原则**：不上传数据，不默认联网，不启用遥测，不收集用户信息，不检查在线更新。所有网络功能必须显式关闭。
3. **索引权限**：索引文件权限必须为 `0600`，数据目录 `0700`。
4. **容错**：单 Agent 解析失败不影响其他 Agent；单会话/单文件损坏不导致应用退出。
5. **可扩展架构**：核心代码禁止出现 `switch agent` 硬编码三选一逻辑。Agent 特有逻辑必须封装在适配器（`internal/adapters/<agent>`）中，通过注册表注册。
6. **数据口径**：精确值/估算值/未知值必须严格区分。Token 数据缺失显示「未知」而非 0。禁止重复累计 Token。子 Agent Token 不默认合并到主会话。
7. **命令安全**：所有外部命令通过参数数组执行；禁止拼接 `sh -c`。恢复使用 `syscall.Exec`。
8. **版本约束**：支持 linux/amd64 与 linux/arm64，构建为单二进制（使用 modernc.org/sqlite，纯 Go 无 cgo）。不得依赖 Python/Node/浏览器/外部数据库/常驻服务。

## 环境事实（实测，2026-08-05）

| Agent | 版本 | 数据目录 | 恢复命令 |
|-------|------|---------|---------|
| Claude Code | 2.1.216 | `~/.claude/projects/<enc-cwd>/<sessionId>.jsonl` | `claude --resume <id>` |
| Codex CLI | 0.146.0 | `~/.codex/sessions/<Y>/<M>/<D>/rollout-*.jsonl` | `codex resume <id>` |
| OpenCode | 1.18.13 | `~/.local/share/opencode/opencode.db`（SQLite ~6GB） | `opencode run -s <id>` |

格式细节见 `docs/formats/{claude-code,codex-cli,opencode}.md`。环境变更后（如版本升级）需重新调查并更新文档，禁止凭经验硬编码未确认格式。

## 目录结构

```
cmd/talea/                 # main 入口
internal/
  model/                   # Session/TokenUsage/TimelineEvent 等统一模型
  adapters/                # adapter.go(接口) + registry.go + claude/ codex/ opencode/
  config/                  # TOML 配置（默认值 + 校验）
  index/                   # SQLite schema、迁移、增量索引、usage 去重
  search/                  # FTS5 全文搜索
  resume/                  # 恢复命令构造 + 路径映射
  security/                # 脱敏、ANSI 清理、路径安全
  timeline/                # Token 时间线聚合、时间桶、用户轮次
  usage/                   # Token 汇总、去重、子 Agent 聚合
  doctor/                  # 环境诊断
  preview/                 # 对话预览
  app/                     # 业务编排（与 TUI 分离）
  cli/                     # cobra 命令
  tui/                     # Bubble Tea 界面
  version/
migrations/                # 幂等 SQL 迁移
docs/                      # 架构、隐私、token、格式文档
testdata/{claude,codex,opencode}/   # 脱敏测试夹具
```

## 分层规则

- `model/` 不依赖任何 Agent 实现。
- `adapters/` 只依赖 `model/`，输出 `Session` 等模型，不直接写数据库。
- `index/` 从 adapters 拿到数据写入 SQLite，不感知 Agent 细节。
- `timeline/`、`usage/` 只依赖 `model/` + `index/` 查询。
- `tui/` 只通过 `app/` 层访问业务逻辑。
- 新增 Agent = 新增一个 `internal/adapters/<name>/` 包 + 在 `registry.go` 注册，不改核心。

## 代码规范

- 使用 `context.Context`；错误用 `%w` 包装。
- 正确关闭文件与数据库（defer Close）。
- 避免全局可变状态；核心逻辑与 TUI 分离。
- 函数 ≤ 50 行，文件 ≤ 300 行，圈复杂度 ≤ 10，禁止魔法数字。
- 路径处理支持中文和特殊字符，使用 `filepath` 而非字符串拼接。
- 敏感信息（API token、密码、私钥、连接串）默认遮罩；日志不记录完整消息与环境变量。

## 命令约定

- 常用：`make build` / `make test` / `make lint` / `go run ./cmd/talea`
- 测试：`go test ./...`；性能：`go test -bench` 与 `testdata/gen` 生成 1000/10000 会话、10 万事件夹具。
- 提交：遵循全局提交规范（summary ≤ 50 字符，正文每行 ≤ 72，说明改动原因与影响）。不主动 push/merge。

## 开发/调查流程

1. 新需求或格式变更：先读 `docs/` 与对应 `docs/formats/*.md`。
2. 修改 Agent 解析：先只读采样真实数据确认结构，更新格式文档，再实现。
3. 实现后运行相关测试与 golangci-lint；完成后按「验证结果、未验证项、风险」汇报。
4. 测试夹具必须脱敏（替换路径、邮箱、token、用户内容）。

## 验收门禁（P0）

- `go vet` 通过、`golangci-lint` 通过、`go test ./...` 通过。
- linux/amd64 与 linux/arm64 均构建通过。
- `talea doctor` 识别本机 Agent；`talea list` 展示全部必填字段；`talea search` 中文可用；`talea open --dry-run` 输出正确目录与参数；TUI Enter 正确恢复。
- 单损坏文件不退出；Agent 数据库只读；索引权限 0600。
