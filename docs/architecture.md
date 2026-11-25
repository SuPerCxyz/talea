# 架构文档

Talea 采用「适配器 + 统一模型 + 存储层」的分层架构，核心原则是 Agent 特
有逻辑全部封装在适配器中，核心代码不做 Agent 硬编码。

## 分层

```
┌─────────────────────────────────────────────┐
│  CLI (cobra)          TUI (Bubble Tea)      │
├─────────────────────────────────────────────┤
│              app（业务编排）                  │
├────────────┬──────────┬──────────┬───────────┤
│ adapters   │  index   │  search  │  resume   │
│ claude     │ SQLite   │  FTS5    │ 路径映射  │
│ codex      │ 增量索引  │  trigram │ syscall   │
│ opencode   │ 0600     │          │           │
│ (可扩展)    │          │          │           │
├────────────┴──────────┴──────────┴───────────┤
│              model（统一数据模型）            │
└─────────────────────────────────────────────┘
```

- `model/` 定义 Session、TokenUsage、TimelineEvent 等统一模型，不依赖任何
  Agent 实现。
- `adapters/` 只依赖 `model/`，输出统一模型，不直接写数据库。
- `index/` 从适配器拿到数据写入 SQLite，不感知 Agent 细节。
- `timeline/`、`usage/` 只依赖 `model/` + `index/` 查询。
- `status/` 依赖 `model/` + `index/`（会话查询复用 `search/`），负责回合状态聚合、
  展示态、状态事件文件与 `status --json` 视图；不包含 Agent 分支。
- `tui/` 只通过 `app/` 层访问业务逻辑；dashboard 复用会话详情与恢复流程。

## 适配器架构

见 `docs/adapters/architecture.md`。核心接口：

```go
type Adapter interface {
    Info() AdapterInfo
    Detect(ctx) ([]AgentInstance, error)
    Discover(ctx, instance) ([]SessionSource, error)
    ParseMetadata(ctx, instance, source) (*Session, error)
}
```

可选能力接口通过类型断言发现（`adapters.As[T]`），新增能力不破坏旧适配器。

## 存储

- SQLite（modernc.org/sqlite，纯 Go，无 cgo）。
- 索引文件 `~/.local/share/talea/index.db`，权限 0600，WAL 模式。
- 核心表：`sessions`、`session_usage`、`usage_timeline_events`，以及独立的
  `session_turn_state`（回合状态派生数据，主键与 `sessions` 一致）。
- schema 版本通过 `schema_meta` 管理，迁移幂等；新增表使用
  `CREATE TABLE IF NOT EXISTS`，旧库首次运行自动补齐。

## 索引流程

```
本地来源指纹未变化 → 跳过发现与解析
来源变化 → Discover/增量 Discover → ParseMetadata → UpsertMany(事务写入)
     → 时间线事件索引（source_identity 去重）
```

增量依据：Talea 本地来源指纹、`(agent_instance_id, session_id)` 主键和源记录
mtime/size。Claude/Codex 的来源指纹由已知文件及父目录元数据组成；OpenCode
额外保存 `(time_updated, session_id)` 高水位，查询失败或状态不确定时回退完整发现。
来源未变化则跳过；变化时仅重新解析候选中实际变化的会话。

## 状态聚合与看板

```
适配器 ParseMetadata → Session.TurnState/TurnStateAt/TurnStateSource
  → index.session_turn_state → status.Build（合并阈值与精确事件）
  → talea dashboard / talea status / status --json
```

- 精确事件来自用户显式配置的 hooks/plugin，写入 Talea 数据目录的
  `status-events.jsonl`（文件 0600、目录 0700）；不修改 Agent 配置。
- 事件时间不早于索引回合状态时间时才覆盖推断结果。
- 看板按固定分组顺序展示，通过 `tea.Tick` 增量刷新；刷新期间跳过重叠 tick。
- “已完成”仅表示本轮结束并等待下一步，不做语义任务完成判断。

## 恢复流程

```
查找会话 → 构造 Plan（路径映射/覆盖目录）→ LookPath
  → 参数数组构造 → Chdir → syscall.Exec 替换进程
```

不使用 `sh -c`，不嵌套 shell，参数通过数组传递，避免注入。

## 容错

- 单文件解析失败：跳过并记录错误，不中断整体。
- 单 Agent 失败：不影响其他 Agent。
- 数据库 busy：使用 busy_timeout，锁定时保留旧索引。
- 源文件删除：保留索引记录（通过 source 状态区分）。

## 性能设计

- TUI 先显示已有索引，来源同步在后台刷新；首次没有缓存时才阻塞在加载态。
- 索引库保存来源指纹、OpenCode 游标和 FTS 脏状态，重复启动不重复扫描、写活动状态或重建 FTS。
- 元数据索引只读一次文件首尾与关键字段，不加载全文。
- FTS5 trigram tokenizer 支持中文 3 字以上匹配；短词用 LIKE 兜底。
- 时间线分页查询；图表使用聚合数据。
- 索引可取消（context）。
