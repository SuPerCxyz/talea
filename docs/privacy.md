# 隐私与安全

Talea 是本地优先工具，隐私原则是最高优先级。

## 原则

1. 不修改 Agent 原始会话数据。
2. 不上传任何数据。
3. 默认不建立网络连接。
4. 不依赖云服务。
5. 不启用遥测。
6. 不收集用户信息。
7. 不发送崩溃报告。
8. 不默认检查在线更新。
9. 所有网络功能必须显式关闭。

## 数据存储

| 内容 | 路径 | 权限 |
|------|------|------|
| 索引 | `~/.local/share/talea/index.db` | 0600 |
| 配置 | `~/.config/talea/config.toml` | 0600 |
| 数据目录 | `~/.local/share/talea/` | 0700 |

索引文件创建时即 chmod 0600，数据目录 0700。

## 敏感信息保护

会话可能包含 API Token、密码、私钥、内网地址、客户信息、源代码、数据库
连接串、Shell 命令、环境变量。Talea：

- 日志不记录完整消息与环境变量。
- 默认遮罩常见密钥格式（`internal/security`）：
  - `sk-/pk-/rk-` 前缀 API Key
  - `api_key=` / `secret=` / `password=` / `token=`
  - `-----BEGIN ... PRIVATE KEY-----`
  - `Bearer <token>`
  - URL 中的 `user:password@`
- 不索引二进制附件。
- 不索引无限大的工具输出（`max_tool_output_bytes` 限制）。
- 导出时明确提示可能包含敏感内容。

## 防御措施

- **命令注入**：所有外部命令通过参数数组执行，禁止 `sh -c` 拼接；
  恢复使用 `syscall.Exec`。恶意 Session ID 或工作目录无法注入。
- **ANSI 注入**：预览内容清理 ANSI 控制序列（`security.StripANSI`）。
- **符号链接**：路径使用 `filepath` 处理，不信任可写目录中的链接。
- **数据库**：OpenCode 数据库只读打开（`mode=ro`），带 busy timeout。

## 元数据模式

```text
talea index --metadata-only
```

只保存 Agent、会话 ID、首次提问、时间、工作目录、Git 信息、Token 汇总，
不保存完整对话正文。

## 导出提示

`talea export` 等导出操作会包含会话内容，导出前应确认
文件中不包含需保密的密钥或客户信息。
