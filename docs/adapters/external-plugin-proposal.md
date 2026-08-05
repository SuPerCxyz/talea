# 外部适配器协议

Talea 支持外部适配器进程，通过标准输入输出交换 JSON，不加载不受信任的
共享库。命名约定：`talea-adapter-<name>`，放在 PATH 中即被自动发现。

## 协议（JSON Lines over stdio）

适配器进程从 stdin 读取请求行，向 stdout 写响应行。

请求：

```json
{"method": "info"}
{"method": "detect"}
{"method": "discover", "instance": {"agent_id": "...", "instance_id": "...", "data_directory": "..."}}
{"method": "parse", "instance": {...}, "source": {"session_id": "...", "path": "...", "source_id": "..."}}
{"method": "messages", "session": {...}, "options": {"limit": 20, "show_system": false}}
{"method": "usage", "session": {...}}
{"method": "timeline", "session": {...}}
```

响应：

```json
{"ok": true, "result": ...}
{"ok": false, "error": "..."}
```

## 方法

| 方法 | 参数 | 返回 |
|------|------|------|
| `info` | 无 | `AdapterInfo`（id/display_name/capabilities） |
| `detect` | 无 | `[]AgentInstance` |
| `discover` | instance | `[]SessionSource` |
| `parse` | instance + source | `Session`（完整字段，snake_case JSON） |
| `messages` | session + options | `[]Message`（role/content/timestamp） |
| `usage` | session | `TokenUsage`（input/output/total/cache...） |
| `timeline` | session | `[]UsageTimelineEvent`（event_type/sequence/tokens...） |

字段名使用 snake_case（`session_id`、`input_tokens` 等），与 `model.Session` /
`model.TokenUsage` / `model.UsageTimelineEvent` 的 json tag 一致。

## 示例

参考 `scripts/talea-adapter-example/main.go`（实现全部 7 个方法）。

## 实现要点

1. 适配器进程启动一次，常驻处理多请求。
2. 每次响应必须是完整 JSON 单行。
3. `detect` 未安装时返回空数组而非错误。
4. 异常必须通过 `{"ok":false}` 返回，不写 stderr（stderr 保留给日志）。
5. 进程超时由调用方（plugin.Client）控制。
6. `parse` 返回的 Session 用于完整元数据；插件不支持 `parse` 时 Talea
   回退到 Discover 提供的基础字段。

## 安全

- 不加载共享库，只交换 JSON。
- 适配器执行在用户权限内，其行为等价于用户运行任意可执行文件。
- 只有 PATH 中的 `talea-adapter-*` 会被加载。

## 插件 SDK

第三方插件只需实现上述 JSON 协议即可被 Talea 完整识别（元数据、消息、
Token 汇总、时间线）。无需引入 Talea 依赖，任何语言皆可实现。

