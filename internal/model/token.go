package model

import "time"

// UsageSource 表示 Token 数据的来源。
type UsageSource string

const (
	UsageSourceMessageMetadata UsageSource = "message_metadata"
	UsageSourceSessionMetadata UsageSource = "session_metadata"
	UsageSourceAgentDatabase   UsageSource = "agent_database"
	UsageSourceLocalLog        UsageSource = "local_log"
	UsageSourceCalculated      UsageSource = "calculated"
	UsageSourceInferred        UsageSource = "inferred"
	UsageSourceUnknown         UsageSource = "unknown"
)

// UsageCompleteness 描述 Token 数据的完整性。
type UsageCompleteness string

const (
	UsageComplete UsageCompleteness = "complete"
	UsagePartial  UsageCompleteness = "partial"
	UsageMissing  UsageCompleteness = "missing"
	UsageUnknown  UsageCompleteness = "unknown"
)

// UsageValueMode 描述单个 usage 数值的累加语义。
type UsageValueMode string

const (
	UsageValueDelta      UsageValueMode = "delta"
	UsageValueCumulative UsageValueMode = "cumulative"
	UsageValueSnapshot   UsageValueMode = "snapshot"
)

// TokenUsage 是会话级 Token 汇总。所有可空字段用指针区分「0」与「未知」。
type TokenUsage struct {
	InputTokens  *int64
	OutputTokens *int64
	TotalTokens  *int64

	CacheReadTokens  *int64
	CacheWriteTokens *int64
	ReasoningTokens  *int64
	ToolTokens       *int64

	RequestCount *int64

	PeakContextTokens *int64
	MaxInputTokens    *int64
	MaxOutputTokens   *int64
	MaxTotalTokens    *int64

	SelfTokens        *int64
	DirectChildTokens *int64
	DescendantTokens  *int64

	EstimatedCostMicros *int64
	Currency            string
	PricingModel        string
	PricingSnapshotAt   *time.Time

	Source       UsageSource
	Completeness UsageCompleteness
	IsEstimated  bool

	RawFields map[string]any
}

// Int64Ptr 返回指向 v 的指针，用于明确区分 0 与未知。
func Int64Ptr(v int64) *int64 { return &v }

// SessionRelation 描述父子会话关系。
type SessionRelation struct {
	ParentAgentInstanceID string
	ParentSessionID       string
	ChildAgentInstanceID  string
	ChildSessionID        string
}
