<div align="center">

<img src="assets/talea-logo.png" alt="Talea logo" width="140"/>

# Talea

**Trace the session. Resume the work.**（循迹会话，续写未完。）

为 AI Coding Agent 打造的本地优先会话索引、Token 时间线分析与恢复工具。

[简体中文](README.zh-CN.md) · [English](README.md)

[![Go](https://img.shields.io/badge/Go-1.25%2B-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: MIT](https://img.shields.io/github/license/SuPerCxyz/talea)](LICENSE)
[![Release](https://img.shields.io/github/v/release/SuPerCxyz/talea)](https://github.com/SuPerCxyz/talea/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/SuPerCxyz/talea/ci.yml?branch=master)](https://github.com/SuPerCxyz/talea/actions)
[![Platform](https://img.shields.io/badge/platform-linux%20%7C%20macos%20%7C%20windows-8892b0)](https://github.com/SuPerCxyz/talea/releases)

</div>

---

Talea 统一索引本机所有 AI Coding Agent 的会话历史，让你无需记住用的是哪个 Agent、
会话 ID、工作目录或恢复命令，一条命令即可查找、分析并**恢复**之前的工作。它只读
打开 Agent 数据，一切都在本机完成，不修改任何原始文件。

## 功能特性

- 🔎 **跨 Agent 全文搜索** — SQLite FTS5，支持中文（trigram）。
- ▶️ **一条命令恢复** — 完整或前缀会话 ID 自动匹配；通过原生
  `claude --resume` / `codex resume` / `opencode -s` 在 Agent 自己的 TUI 中恢复。
- 📊 **Token 时间线与成本分析** — 请求级时间线、模型汇总、上下文窗口曲线与压缩
  检测、终端图表、费用估算。
- 🖥️ **交互式 TUI** — 会话列表 + 聚合详情页（上下文曲线、Token 图表、`t` 键展开
  用户轮次）。
- 🌐 **界面语言** — 默认英文；终端 locale 以 `zh` 开头时自动切换中文。
- 🔌 **可扩展** — 通过 `internal/adapters/<name>` 包或外部 `talea-adapter-<name>`
  可执行文件（JSON Lines over stdio，任意语言）扩展新 Agent。
- 🔒 **隐私安全** — Agent 数据只读、索引文件权限 0600、预览脱敏、无遥测、不联网。
- 🚀 **高性能** — 增量索引 + 断点续读；1 万会话二次索引约 0.8s。

## 支持的 Agent

| Agent | 数据来源 | 恢复命令 |
|-------|---------|---------|
| Claude Code | `~/.claude/projects/<enc-cwd>/<sessionId>.jsonl` | `claude --resume <id>` |
| Codex CLI | `~/.codex/sessions/<Y>/<M>/<D>/rollout-*.jsonl` | `codex resume <id>` |
| OpenCode | `~/.local/share/opencode/opencode.db` (SQLite) | `opencode -s <id>` |
| 任意 | 外部 `talea-adapter-<name>` 插件 | — |

## 安装

### 预编译二进制

从 [Releases 页面](https://github.com/SuPerCxyz/talea/releases) 下载
`talea_<版本>_linux_<arch>.tar.gz` 及 checksums（amd64 / arm64）：

```sh
tar xzf talea_*.tar.gz
sudo install -m 0755 talea /usr/local/bin/talea
talea version
```

### 源码构建

需要 Go 1.25+（无 cgo、无外部依赖）：

```sh
git clone https://github.com/SuPerCxyz/talea.git
cd talea
make build
sudo install -m 0755 bin/talea /usr/local/bin/talea
```

## 快速开始

```sh
# 建立索引
talea index

# 打开 TUI
talea

# 跨 Agent 搜索
talea search "multipath"

# 列出最近会话
talea list

# 按完整或前缀会话 ID 恢复（--dry-run 预览命令）
talea go <session-id> --dry-run
talea go <session-id>

# 交互式选择会话，限定在指定目录子树内
talea go --dir /home/user/myproject
```

## TUI 界面

```text
talea          # 会话列表，最新结束在前（最近 500 条）
talea --dir /path   # 仅列出 /path 目录下的会话
```

- 会话固定按结束时间倒序排列（最新在前），不受配置 `default_sort` 影响。
- `↑` / `↓` 选择，`Enter` 恢复，`d` 详情，`o` 在详情页恢复，`/` 过滤
  （输入后按 `Enter` 应用过滤，之后按 `Enter`/`o` 进入），`q` 退出。
- 详情页聚合展示：会话信息（双列）、第一次提问、用户轮次（`t` 键展开/收起）、
  **上下文窗口曲线**（面积图，带 Token y 轴与时间轴）、按模型汇总、Token 图表、
  Token 汇总、子 Agent 会话。
- 新会话自动出现——TUI 后台自动增量索引，无需手动执行。

## 命令参考

| 命令 | 说明 |
|------|------|
| `talea` | 打开 TUI（`--dir` 限定为指定目录下的会话） |
| `talea list` | 列出会话（`--agent/--cwd/--project/--branch/--today/--active/--sort/--limit/--format`） |
| `talea search <关键词>` | 跨 Agent 全文搜索（`--agent/--cwd/--since/--format`） |
| `talea go [session-id]` | 按完整/前缀 ID 恢复，或交互式选择（`--cwd/--dir/--dry-run`） |
| `talea last` | 当前目录最近会话 |
| `talea index` | 增量索引（`--rebuild/--metadata-only`） |
| `talea usage <id>` | Token 汇总（`--details/--include-subagents/--metrics`） |
| `talea timeline <id>` | Token 时间线（`--group-by/--bucket/--around-peak/--by-model/--context/--insights/--chart`） |
| `talea preview <id>` | 对话预览（`--limit/--system/--tail`） |
| `talea doctor` | 环境诊断（`--json/--agent`） |
| `talea run <agent>` | 包装启动 Agent 并记录真实进程时间 |
| `talea watch` | 监听数据目录，变化时增量索引（`--interval`） |
| `talea web` | 本地只读 Web 视图（仅 localhost，`--port`） |
| `talea tag` | 标签 / 收藏 / 备注（`tag list|favorite|note`） |
| `talea export` / `talea import` | 离线多设备迁移（JSON） |
| `talea config` | `config path|init|validate` |

输出格式：`--format table|json|jsonl|csv|markdown`。

## Token 分析

```sh
# 请求级时间线（含日期）
talea timeline <session-id>

# 按用户轮次聚合
talea timeline <session-id> --group-by turn

# 5 分钟时间桶，峰值附近
talea timeline <session-id> --bucket 5m --around-peak

# 按模型汇总
talea timeline <session-id> --by-model

# 上下文窗口曲线 + 压缩检测
talea timeline <session-id> --context

# 本地规则洞察
talea timeline <session-id> --insights

# 费用估算（配置开启）
talea usage <session-id>
```

精确值 / 估算值 / 未知值严格区分——缺失显示「未知」而非 0。时间线事件按
`source_identity` 去重；子 Agent Token 独立保存，默认不合并到主会话。

## 界面语言

默认英文；当 `LANG` / `LC_ALL` / `LC_MESSAGES` / `LANGUAGE` 以 `zh` 开头时自动切换
中文，覆盖 TUI、CLI 输出、命令帮助与错误消息。

## 配置

`talea config init` 生成 `~/.config/talea/config.toml`。要点：

```toml
[general]
default_sort = "last_activity"

[usage]
estimate_cost = false        # 启用费用估算

[usage.pricing.custom-model]
currency = "USD"
input_per_million = 3.0
output_per_million = 15.0
cache_read_per_million = 0.3

[path_mapping]               # 目录迁移/重命名映射
"/old/project" = "/new/project"
```

## 数据与隐私

- Agent 原始会话文件与数据库**只读**打开，绝不修改、重命名或删除。
- 索引：`${XDG_DATA_HOME:-~/.local/share}/talea/index.db`，文件权限 0600、目录 0700。
- 不上传、不遥测、不检查在线更新。网络功能显式开启（如 `talea web` 仅绑定 localhost）。

## 扩展 Talea

- **内置**：新增 `internal/adapters/<name>/` 包并在
  `internal/adapters/registry.go` 注册；核心代码无 `switch agent`。
- **外部插件**：将实现 `talea-adapter-<name>` 协议（JSON Lines over stdio，
  方法 `info/detect/discover/parse/messages/usage/timeline`）的可执行文件放入
  `PATH`。详见 `docs/adapters/`。

## 路线图

见 [`docs/plan/03-enhancement-roadmap.md`](docs/plan/03-enhancement-roadmap.md)：
规划中的 `talea stats` 全局报表、会话 fork、Web 仪表盘增强等。

## FAQ

**我的数据会上传吗？** 不会。一切本地运行且只读。

**需要 Python/Node/浏览器吗？** 不需要——单个 Go 二进制，零外部运行时依赖。

**目录被移动/重命名后能恢复吗？** 可以。`talea go <id> --cwd <dir>` 或配置
`[path_mapping]`；原目录不存在时 `talea` 会提示输入新目录。

## 开发

```sh
make build    # 构建 bin/talea
make test     # go test ./...
make lint     # golangci-lint
make vet      # go vet ./...
```

欢迎提交 issue 或 pull request。

## 许可证

[MIT](LICENSE) © 2026 superc
