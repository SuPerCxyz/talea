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
	InputTokens  *int64 `json:"input_tokens"`
	OutputTokens *int64 `json:"output_tokens"`
	TotalTokens  *int64 `json:"total_tokens"`

	CacheReadTokens  *int64 `json:"cache_read_tokens"`
	CacheWriteTokens *int64 `json:"cache_write_tokens"`
	ReasoningTokens  *int64 `json:"reasoning_tokens"`
	ToolTokens       *int64 `json:"tool_tokens"`

	RequestCount *int64 `json:"request_count"`

	PeakContextTokens *int64 `json:"peak_context_tokens"`
	MaxInputTokens    *int64 `json:"max_input_tokens"`
	MaxOutputTokens   *int64 `json:"max_output_tokens"`
	MaxTotalTokens    *int64 `json:"max_total_tokens"`

	SelfTokens        *int64 `json:"self_tokens"`
	DirectChildTokens *int64 `json:"direct_child_tokens"`
	DescendantTokens  *int64 `json:"descendant_tokens"`

	EstimatedCostMicros *int64     `json:"estimated_cost_micros"`
	Currency            string     `json:"currency"`
	PricingModel        string     `json:"pricing_model"`
	PricingSnapshotAt   *time.Time `json:"pricing_snapshot_at"`

	Source       UsageSource       `json:"usage_source"`
	Completeness UsageCompleteness `json:"completeness"`
	IsEstimated  bool              `json:"is_estimated"`

	RawFields map[string]any `json:"raw_fields,omitempty"`
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
