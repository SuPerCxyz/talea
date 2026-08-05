# Talea

Trace the session. Resume the work.

Talea is a local-first Linux session index, token timeline analyzer,
and resume launcher for AI coding agents.

## 这是什么

Talea 统一索引本机多个 AI Coding Agent 的历史会话，让你不用记住用的是哪个
Agent、会话 ID、工作目录、开始时间或恢复命令，一条 `talea` 命令即可找到并
恢复之前的工作。

## 名称含义

音乐术语 *talea*（等节奏中反复出现的节奏模式）：每个 AI Agent 会话是一段
工作乐章，每条消息、工具调用、模型请求是时间线上的节拍，Token 消耗反映会话
上下文的密度变化，会话恢复表示回到之前的工作位置继续推进。

## 解决的问题

- 不同 Agent 的会话记录相互独立，难以统一查找
- 忘记当时用的是哪个 Agent、会话 ID 或工作目录
- Agent 自动标题无法表达用户最初的提问
- 无法跨 Agent 搜索历史对话
- 难以知道会话消耗了多少 Token、上下文何时膨胀或压缩
- 原目录移动/重命名后难以恢复

## 支持的 Agent

| Agent | 数据目录 | 恢复命令 |
|-------|---------|---------|
| Claude Code | `~/.claude/projects/...` | `claude --resume <id>` |
| Codex CLI | `~/.codex/sessions/...` | `codex resume <id>` |
| OpenCode | `~/.local/share/opencode/opencode.db` | `opencode run -s <id>` |

支持 linux/amd64 与 linux/arm64，构建为单二进制，无 Python/Node/浏览器/外部
数据库依赖。

## 安装

```text
# 从源码构建
make build
sudo install -m 0755 bin/talea /usr/local/bin/talea
```

## 快速开始

```text
# 建立索引
talea index

# 打开 TUI
talea

# 跨 Agent 搜索
talea search "multipath"

# 列出最近会话
talea list

# 恢复指定会话（干跑查看参数）
talea open --agent claude-code <session-id> --dry-run

# 当前目录最近会话
talea last

# 环境诊断
talea doctor
```

## CLI 命令

```text
talea                          # TUI 主界面
talea list                     # 列表（--agent/--cwd/--today/--active/--sort/--limit）
talea search "关键词"            # 跨 Agent 全文搜索
talea open <session-id>         # 恢复会话（--dry-run/--cwd/--agent）
talea last                      # 当前目录最近会话
talea index                     # 增量索引（--rebuild/--metadata-only）
talea usage <session-id>        # Token 汇总（--details/--include-subagents）
talea timeline <session-id>     # Token 时间线（--group-by/--bucket/--around-peak）
talea doctor                    # 环境诊断（--json/--agent）
talea config path|init|validate
talea version
```

输出格式支持 `--format table|json|jsonl|csv|markdown`。

## 首次提问

会话列表最重要的识别字段是「第一次提问」：会话中第一条真实用户消息，过滤
系统提醒、AGENTS.md 注入块、环境上下文、工具输出等内部内容。无法识别时显示
「未识别到有效用户提问」。

## 开始与结束时间

- 开始时间优先级：会话元数据创建时间 → 第一条真实用户消息 → 第一条有效事件
  → 文件 mtime。
- 结束时间优先级：真实进程退出时间 → Agent 明确保存的结束时间 → 最后一条
  有效事件 → 文件 mtime。用最后活动代替结束时间时详情页标注「结束依据」。

## Token 汇总

精确值/估算值/未知值严格区分：缺失显示「未知」而非 0。禁止重复累计 Token，
usage 事件通过 `source_identity` 去重。子 Agent Token 独立保存，默认不合并
到主会话。

## 恢复原理

选择会话后：检查 Agent 与目录 → 应用路径映射 → 构造原生恢复命令（参数数组，
无 shell 拼接）→ 切到原目录 → `syscall.Exec` 替换进程。

## 数据目录

```text
索引：${XDG_DATA_HOME:-~/.local/share}/talea/index.db （权限 0600）
配置：${XDG_CONFIG_HOME:-~/.config}/talea/config.toml
缓存：${XDG_CACHE_HOME:-~/.cache}/talea/
```

## 隐私

不上传数据、不默认联网、不启用遥测、不收集用户信息、不检查在线更新。索引
文件权限 0600，敏感信息默认遮罩，日志不记录完整消息与环境变量。

## 新增 Agent

新增一个 `internal/adapters/<name>/` 包并在 `registry.go` 注册即可，核心代码
无 `switch agent` 硬编码。详见 `docs/adapters/adding-an-agent.md`。

## 故障排查

```text
talea doctor        # 检查环境与索引状态
talea config validate
```

## 开发与测试

```text
make build && make test && make lint
go test ./...
```

详细架构见 `docs/architecture.md`，Token 模型见 `docs/token-model.md`。
