// Package adapters 定义 Agent 适配器的接口与注册表。
package adapters

import (
	"context"

	"github.com/talea/talea/internal/model"
)

// SessionSource 标识一个可解析的会话来源（文件或数据库记录）。
type SessionSource struct {
	SessionID string
	Path      string
	SourceID  string
	Mtime     int64
	Size      int64
	Offset    int64
}

// Command 描述一条要执行的外部命令。
type Command struct {
	Program string
	Args    []string
}

// Adapter 是所有 Agent 适配器必须实现的基础接口。
type Adapter interface {
	Info() model.AdapterInfo

	Detect(ctx context.Context) ([]model.AgentInstance, error)

	Discover(
		ctx context.Context,
		instance model.AgentInstance,
	) ([]SessionSource, error)

	ParseMetadata(
		ctx context.Context,
		instance model.AgentInstance,
		source SessionSource,
	) (*model.Session, error)
}

// MessageLoadOptions 控制消息预览加载。
type MessageLoadOptions struct {
	Limit      int
	ShowSystem bool
}

// Message 是预览用消息单元。
type Message struct {
	Role       string
	Timestamp  int64
	Content    string
	IsSystem   bool
	ToolName   string
	HasTool    bool
}

// MessageIterator 提供消息流式读取。
type MessageIterator interface {
	Next() (Message, bool, error)
	Close() error
}

// MessageLoader 可选能力：读取会话消息。
type MessageLoader interface {
	LoadMessages(
		ctx context.Context,
		session model.Session,
		options MessageLoadOptions,
	) (MessageIterator, error)
}

// Resumer 可选能力：构造恢复命令。
type Resumer interface {
	BuildResumeCommand(
		session model.Session,
		cwd string,
	) (Command, error)
}

// UsageProvider 可选能力：读取 Token 汇总。
type UsageProvider interface {
	LoadUsage(
		ctx context.Context,
		session model.Session,
	) (*model.TokenUsage, error)
}

// UsageTimelineProvider 可选能力：迭代 usage 时间线事件。
type UsageTimelineProvider interface {
	IterateUsageEvents(
		ctx context.Context,
		session model.Session,
	) (UsageEventIterator, error)
}

// UsageEventIterator 提供 usage 事件流式读取。
type UsageEventIterator interface {
	Next() (*model.UsageTimelineEvent, bool, error)
	Close() error
}

// SubagentProvider 可选能力：解析会话父子关系。
type SubagentProvider interface {
	ResolveSessionRelations(
		ctx context.Context,
		sessions []model.Session,
	) ([]model.SessionRelation, error)
}

// ActivityDetector 可选能力：检测会话活动状态。
type ActivityDetector interface {
	DetectActivity(
		ctx context.Context,
		session model.Session,
	) (model.ActivityState, error)
}
