# 新增 Agent 指南

Talea 的 Agent 支持通过适配器架构扩展。新增一个 Agent 不需要修改核心代码。

## 1. 定义 Agent ID

在 `internal/model/model.go` 的 `AgentID` 常量区添加：

```go
const AgentMyAgent model.AgentID = "my-agent"
```

`AgentID` 是开放字符串类型，允许任意新值。

## 2. 创建适配器包

```
internal/adapters/myagent/myagent.go
```

实现 `Adapter` 接口：

```go
type Adapter struct{}

func New() *Adapter { return &Adapter{} }

func (a *Adapter) Info() model.AdapterInfo {
    return model.AdapterInfo{
        ID: model.AgentMyAgent,
        DisplayName: "My Agent",
        Version: versionOf(),
        Capabilities: []model.Capability{
            model.CapabilityDiscoverSessions,
            model.CapabilityReadMessages,
            model.CapabilityResume,
            model.CapabilityWorkingDirectory,
            model.CapabilityTokenSummary,
        },
    }
}
```

## 3. 探测与发现

- `Detect`：定位可执行文件与数据目录，返回 `[]AgentInstance`。
- `Discover`：枚举会话来源，返回 `[]SessionSource`（含 session ID、路径、
  mtime、size）。

## 4. 解析元数据

`ParseMetadata` 返回统一 `Session`：

- **首次提问**：读取第一条真实 user 消息，过滤系统提醒、AGENTS 注入块、
  环境上下文。用 `internal/adapters/extract` 的共享函数。
- **开始时间**：优先会话元数据创建时间，其次第一条用户消息。
- **工作目录**：会话元数据中的 cwd 或事件中的 cwd 字段。
- **Token**：填充 `TokenUsage`，缺失用 nil（显示「未知」）。

## 5. 注册

在 `internal/app/app.go` 的 `registerBuiltins` 中添加：

```go
if err := reg.Register(myagent.New()); err != nil {
    return err
}
```

## 6. 可选能力

按需实现可选接口：

```go
// 消息预览
func (a *Adapter) LoadMessages(ctx, session, opts) (adapters.MessageIterator, error)

// 恢复命令
func (a *Adapter) BuildResumeCommand(session, cwd) (adapters.Command, error)

// Token 汇总
func (a *Adapter) LoadUsage(ctx, session) (*model.TokenUsage, error)

// Token 时间线
func (a *Adapter) IterateUsageEvents(ctx, session) (adapters.UsageEventIterator, error)

// 父子会话
func (a *Adapter) ResolveSessionRelations(ctx, sessions) ([]model.SessionRelation, error)

// 活动检测
func (a *Adapter) DetectActivity(ctx, session) (model.ActivityState, error)
```

## 7. 增加测试夹具

在 `testdata/<agent>/` 放脱敏样例。夹具必须替换真实路径、邮箱、token、
用户内容。

## 8. 编写格式文档

`docs/formats/<agent>.md` 说明：已验证版本、数据目录、字段、Token 口径、
恢复命令、已知限制、测试样本来源、验证状态。

## 9. 避免真实数据泄漏

- 日志不记录完整消息与环境变量。
- 夹具必须脱敏。
- 索引文件权限 0600。

## 10. 处理未知格式

`ParseMetadata` 遇到无法识别的格式版本时应返回错误（调用方会跳过并记录），
或在 `Session.FormatVersion` 标注未知，由上层决定显示策略。
