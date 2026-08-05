# 本地 Web 只读视图

`talea web` 启动一个仅监听 `127.0.0.1` 的只读 Web 服务，用于在浏览器中
浏览会话列表与详情。无任何写操作、无遥测、不对外暴露。

```text
talea web                      # 默认端口 7690
talea web --port 8080          # 指定端口
```

## API

| 端点 | 说明 |
|------|------|
| `/` | 简易 HTML 页面（会话列表 + 搜索） |
| `/api/sessions` | 会话列表（`?q=` 关键词，`?agent=` 过滤） |
| `/api/session?id=<id>` | 会话详情（含 usage、tags、favorite、note） |
| `/api/timeline?id=<id>` | 请求级 Token 时间线 |
| `/api/tags` | 收藏会话列表 |

## 安全

- 只绑定 `127.0.0.1`，不监听外部接口。
- 只读查询，不提供任何修改能力。
- 数据从本地索引读取，不上传。
- Ctrl+C 优雅退出。

## 实现

`internal/web/` 包：`Server.Handler()` 返回 http.Handler，
`Listen(ctx, port)` 返回 localhost listener。
