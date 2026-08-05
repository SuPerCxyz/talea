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

## 示例

参考 `scripts/talea-adapter-example/main.go`。

## 实现要点

1. 适配器进程启动一次，常驻处理多请求。
2. 每次响应必须是完整 JSON 单行。
3. `detect` 未安装时返回空数组而非错误。
4. 异常必须通过 `{"ok":false}` 返回，不写 stderr（stderr 保留给日志）。
5. 进程超时由调用方（plugin.Client）控制。

## 安全

- 不加载共享库，只交换 JSON。
- 适配器执行在用户权限内，其行为等价于用户运行任意可执行文件。
- 只有 PATH 中的 `talea-adapter-*` 会被加载。

## 未来扩展

协议可扩展 `parse` / `messages` / `usage` / `timeline` 方法，返回统一模型
的 JSON。当前版本（P2 研究阶段）支持 info/detect/discover。
