# 费用估算

费用估算是可选功能，默认关闭。

## 原则

1. 默认关闭（`[usage] estimate_cost = false`）。
2. 使用整数最小计价单位保存（微货币单位 `EstimatedCostMicros`），
   不用 float64 保存货币金额。
3. 价格表带版本与生效时间（`PricingSnapshotAt`）。
4. 价格变化不自动修改旧估算。
5. 无法确认模型版本时不估算。
6. 本地免费模型不错误显示费用。
7. 缓存读写使用独立价格。
8. 明确显示「估算费用」。
9. 不声称等同供应商账单。
10. 不联网读取用户账单。

## 配置示例

```toml
[usage]
enabled = true
estimate_cost = true

[usage.pricing.custom-model]
currency = "USD"
input_per_million = 1.0
output_per_million = 4.0
cache_read_per_million = 0.1
cache_write_per_million = 1.25
```

## 模型定价

自定义价格表按模型名匹配（`[usage.pricing.custom-model]` 键为模型 ID）。
估算逻辑：

```
cost_micros = input/1e6*input_price + output/1e6*output_price
            + cache_read/1e6*cache_read_price + cache_write/1e6*cache_write_price
```

结果转为微单位整数保存。若模型未在价格表或无法确认模型版本，则不估算，
`IsEstimated` 保持 false。

## 快照

估算写入时记录 `PricingSnapshotAt`（价格生效时间）。后续价格表更新不会
重算历史估算；需要重算时重新生成索引。

## 状态

当前版本（P0/P1 核心）已实现数据模型与配置占位。完整的按请求费用估算
归入 P1 后续增强。
