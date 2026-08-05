# Token 数据模型

本文档说明 Talea 的 Token 数据模型与口径约定。

## 核心原则

1. 精确值 / 估算值 / 未知值严格区分。
2. 缺失显示「未知」而非 0。
3. 禁止把累计 Token 和单次请求 Token 重复相加。
4. 禁止把上下文大小当作累计 Token。
5. 子 Agent Token 不默认合并到主会话。
6. 无法解释的字段保存到 `RawFields`，不错误映射。

## 模型

`TokenUsage` 全部数值字段使用 `*int64`，`nil` 表示「未知」：

| 字段 | 含义 |
|------|------|
| InputTokens | 累计输入 Token |
| OutputTokens | 累计输出 Token |
| TotalTokens | 累计总 Token |
| CacheReadTokens | 缓存读取 Token |
| CacheWriteTokens | 缓存写入 Token |
| ReasoningTokens | 推理 Token |
| ToolTokens | 工具相关 Token |
| RequestCount | 请求次数 |
| PeakContextTokens | 上下文峰值 |
| SelfTokens / DirectChildTokens / DescendantTokens | 自/直接子/后代 |

来源与完整性：

```go
type UsageSource string      // message_metadata / agent_database / calculated / inferred ...
type UsageCompleteness string // complete / partial / missing / unknown
```

## 累加语义

`UsageValueMode` 区分三种取值：

- `delta`：单次增量，可直接累加。
- `cumulative`：会话累计值，只取最后一次。
- `snapshot`：上下文快照，不能用于累加。

典型场景（OpenCode）：`step-finish.tokens.total` 是上下文快照（含累计输入），
`session.tokens_input` 是会话汇总。两者口径不同，禁止直接相加。

## 去重

`usage_timeline_events.source_identity` 保证同一事件的幂等写入：
重复写入被 `INSERT OR IGNORE` 忽略。事件由适配器为每次解析生成稳定的
身份标识（如 `opencode-part:<part_id>`）。

## 会话累计

会话累计值取「最后一条 request 事件的 total_tokens」而非所有事件之和，
避免把上下文快照累加。

## 时间线

`UsageTimelineEvent` 记录每个请求/工具/子 Agent/压缩事件，包含：
- 每次请求的 input/output/cache/reasoning
- 上下文 before/after/limit
- 模型与 provider
- 来源与完整性标记

轮次聚合（`timeline.GroupByTurns`）将一次用户消息到下一次用户消息之间
的所有请求归入同一轮次。

## 费用估算（P1，默认关闭）

- 使用整数微货币单位（`EstimatedCostMicros`），不用 float64 存金额。
- 价格表带版本与生效时间；价格变化不修改旧估算。
- 无法确认模型版本时不估算。
- 本地免费模型不显示费用。
