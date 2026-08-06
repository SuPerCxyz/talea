# Talea 后续增强路线图

基于 2026-08 开源同类项目调研，对比 Talea 现有能力得出的可补充功能清单。
本文件仅登记候选，**未排期实现**。优先级为建议值，实现前需重新评估。

## 1. 调研对比（2026-08-06，GitHub）

| 项目 | 语言/星 | 核心能力 | 与 Talea 差异 |
|------|--------|---------|--------------|
| Piebald-AI/splitrail | Rust / 216★ | 实时 Token 追踪 + 成本监控，支持 OpenCode 等多 agent，status line | 全局实时统计 |
| gega-dkv/agent-usage-stats | TS | 20 个 agent 归一化到 SQLite，CLI + Web + 桌面三端 | 跨 agent 归一化 + 多端仪表盘 |
| ak5/ai-session-analyzer (asa) | Rust | analyze / resume / fork 会话 | fork 能力 |
| Tomatio13/session-analyzer | Python | 工具使用统计、skill 使用、生产力指标 | 工具调用维度聚合 |
| sukenshah/claude-code-analyzer | TS | Token + 成本按 session/turn/project 聚合 | 项目维度报表 |
| asupc/claude-meter | JS | status line hook + Web dashboard 实时追踪 | shell 状态行集成 |
| hoodini/tokana | TS | 缓存节约分析、billing vs 实际上下文、live meter | 缓存效率维度 |
| prathambdevx/claude-session-manager | TS | resume/fork + kanban 看板 + scoped agents | fork + 看板管理 |

## 2. Talea 现有覆盖

- 会话索引 / 搜索 / 预览 / 恢复（Claude Code / Codex CLI / OpenCode + 外部适配器协议）。
- 单会话 Token 汇总、时间线、上下文曲线、压缩检测、费用估算。
- TUI（列表 + 聚合详情页）、Web 只读视图。
- 标签 / 收藏 / 备注、export/import、watch、run、i18n 双语。

## 3. 可补充功能清单

### P0 — 高价值低成本

1. **`talea stats` 全局统计报表**
   - 按 Agent / 项目 / 日期 / 周聚合 Token 与费用。
   - 数据已在库（`session_usage` + `usage_timeline_events`），SQL `GROUP BY` 即可。
   - 复用 `internal/cost` 费用估算；含缓存命中率。
   - 输出 table/json/markdown。
   - 数据来源：`internal/usage`、`internal/timeline` 聚合函数。

### P1 — 中价值

2. **工具使用统计**
   - `talea stats --by tool` 或在 `talea timeline` 增加工具维度。
   - `usage_timeline_events.tool_name` 已有，SQL 聚合即可。

3. **会话 fork**
   - `talea fork <id>` 基于历史会话创建新会话。
   - OpenCode 原生支持 `run --fork`；其他 agent 需确认能力。

4. **Web 仪表盘增强**
   - 现有 `talea web` 增加 stats 页（按项目 / 时间聚合图表）。

### P2 — 可选

5. **shell 状态行集成**
   - `talea watch --status` 实时展示当前会话 Token 消耗（claude-meter/splitrail 风格）。

6. **HTML 报告导出**
   - `talea stats --format html` 生成交互式报表（token-report 风格）。

7. **MCP server**
   - 暴露 Talea search/stats 给其他 Agent 工具。

8. **趋势 / 异常检测增强**
   - 在 `internal/insights` 基础上扩展"会话间重复错误 / 漂移"检测（drift-watch 风格，较重）。

## 4. 约束一致性

以上候选均符合 `docs/plan/02-implementation-plan.md` §35 边界：
本地优先，不涉及云同步、用户账号、在线服务、多租户、远程上传。

## 5. 实施顺序建议

P0（`talea stats`）为最优先：数据完备、实现成本低，直接补上同类项目
最大的功能空白。后续按 P1 → P2 择机实现。
