# doctor-diagnostics Specification

## Purpose
让用户与维护者在 Agent 上游存储结构变更时，能通过 `talea doctor` 快速确认 Talea 实际读取的存储形态，缩短“看不到记录”类问题的定位时间。
## Requirements
### Requirement: doctor SHALL report the detected storage shape

`talea doctor` SHALL 报告每个被检测 Agent 的本地存储形态；对 OpenCode SHALL 明确指出识别到的是传统 `session` 表路径还是 `session_v2` 主表路径，并附带发现的会话数量。

#### Scenario: OpenCode v2 storage detected

- **WHEN** OpenCode 数据库存在 `session_v2` 表
- **THEN** `talea doctor` SHALL 输出识别为 v2 主表存储形态，并显示发现的会话数

#### Scenario: Legacy storage detected

- **WHEN** OpenCode 数据库不存在 `session_v2` 表
- **THEN** `talea doctor` SHALL 输出识别为传统表存储形态

#### Scenario: Storage shape unreadable

- **WHEN** OpenCode 数据库无法只读打开
- **THEN** `talea doctor` SHALL 输出明确的警告说明无法确定存储形态，而不是报告成功

#### Scenario: Agent not installed

- **WHEN** 未检测到 OpenCode 安装
- **THEN** `talea doctor` SHALL 沿用既有的“未检测到安装”提示，不输出存储形态行

