package model

import "time"

// UsageEventType 表示时间线事件类型。
type UsageEventType string

const (
	UsageEventUserMessage      UsageEventType = "user_message"
	UsageEventAssistantMessage UsageEventType = "assistant_message"

	UsageEventRequest     UsageEventType = "request"
	UsageEventResponse    UsageEventType = "response"
	UsageEventUsageUpdate UsageEventType = "usage_update"

	UsageEventToolStart UsageEventType = "tool_start"
	UsageEventToolEnd   UsageEventType = "tool_end"

	UsageEventSubagentStart UsageEventType = "subagent_start"
	UsageEventSubagentEnd   UsageEventType = "subagent_end"

	UsageEventCompactionStart UsageEventType = "compaction_start"
	UsageEventCompactionEnd   UsageEventType = "compaction_end"
	UsageEventSummaryCreated  UsageEventType = "summary_created"

	UsageEventModelChanged  UsageEventType = "model_changed"
	UsageEventSessionResume UsageEventType = "session_resume"
	UsageEventSessionPause  UsageEventType = "session_pause"

	UsageEventRetry UsageEventType = "retry"
	UsageEventError UsageEventType = "error"
)

// UsageTimelineEvent 描述时间线上单个 usage 相关事件。
type UsageTimelineEvent struct {
	EventID         string
	AgentInstanceID string
	SessionID       string

	Timestamp *time.Time
	Sequence  int64
	Duration  *time.Duration

	EventType UsageEventType

	RequestID       string
	ResponseID      string
	MessageID       string
	ParentMessageID string
	ToolCallID      string
	SubagentID      string

	Model    string
	Provider string

	InputTokens      *int64
	OutputTokens     *int64
	TotalTokens      *int64
	CacheReadTokens  *int64
	CacheWriteTokens *int64
	ReasoningTokens  *int64
	ToolTokens       *int64

	ContextBefore *int64
	ContextAfter  *int64
	ContextLimit  *int64

	CumulativeInput  *int64
	CumulativeOutput *int64
	CumulativeTotal  *int64

	UserPromptPreview string
	ToolName          string
	FilePath          string
	CommandPreview    string

	SourceIdentity string
	ValueMode      UsageValueMode
	Source         UsageSource
	Completeness   UsageCompleteness
	IsEstimated    bool

	RawFields map[string]any
}

// ConversationTurnUsage 描述一次用户轮次的 Token 使用聚合。
type ConversationTurnUsage struct {
	TurnIndex int64

	StartedAt *time.Time
	EndedAt   *time.Time
	Duration  *time.Duration

	UserMessageID string
	PromptPreview string

	InputTokens      int64
	OutputTokens     int64
	TotalTokens      int64
	CacheReadTokens  int64
	CacheWriteTokens int64
	ReasoningTokens  int64

	RequestCount  int64
	ToolCallCount int64
	SubagentCount int64

	PeakContextTokens int64
}
