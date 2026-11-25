// Package status 聚合各 Agent 会话的回合状态，并生成看板与 status 输出所需的展示态。
package status

import (
	"time"

	"github.com/talea/talea/internal/model"
)

// DisplayState 是面向用户的展示状态。
type DisplayState string

const (
	// StateWaiting 表示等待用户输入或确认（包括推断的“可能等待”）。
	StateWaiting DisplayState = "waiting"
	// StateWorking 表示回合仍在进行。
	StateWorking DisplayState = "working"
	// StateCompleted 表示本轮已结束，不表示语义任务完成。
	StateCompleted DisplayState = "completed"
	// StateFailed 表示失败或中断。
	StateFailed DisplayState = "failed"
	// StateIdle 表示已空闲或超出近期窗口。
	StateIdle DisplayState = "idle"
	// StateUnknown 表示无法判断，不得猜测为已完成。
	StateUnknown DisplayState = "unknown"
)

// Confidence 表示状态判断的确定程度。
type Confidence string

const (
	ConfidenceExact    Confidence = "exact"
	ConfidenceInferred Confidence = "inferred"
	ConfidenceUnknown  Confidence = "unknown"
)

// Counts 是各展示状态的汇总计数。
type Counts struct {
	NeedsAttention int `json:"needs_attention"`
	Waiting        int `json:"waiting"`
	Working        int `json:"working"`
	Completed      int `json:"completed"`
	Failed         int `json:"failed"`
	Idle           int `json:"idle"`
	Unknown        int `json:"unknown"`
	Total          int `json:"total"`
}

// SessionStatus 是单个会话的状态视图。
type SessionStatus struct {
	AgentID          model.AgentID `json:"agent_id"`
	Agent            string        `json:"agent"`
	AgentInstanceID  string        `json:"agent_instance_id"`
	SessionID        string        `json:"session_id"`
	ProjectName      string        `json:"project_name"`
	WorkingDirectory string        `json:"working_directory"`

	State          DisplayState          `json:"state"`
	Label          string                `json:"label"`
	TurnState      model.TurnState       `json:"turn_state"`
	Source         model.TurnStateSource `json:"source"`
	Confidence     Confidence            `json:"confidence"`
	Reason         string                `json:"reason"`
	NeedsAttention bool                  `json:"needs_attention"`

	StateAt        *time.Time `json:"state_at"`
	LastActivityAt *time.Time `json:"last_activity_at"`
	TotalTokens    *int64     `json:"total_tokens"`

	// Session 保留原始会话，供 TUI 进入详情与恢复；不进入 JSON。
	Session *model.Session `json:"-"`
}

// Snapshot 是一次状态聚合结果。
type Snapshot struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Counts      Counts          `json:"counts"`
	Sessions    []SessionStatus `json:"sessions"`
	Warnings    []string        `json:"warnings,omitempty"`
}

// DefaultRecentWindow 是看板/status 默认展示的最近会话窗口。
const DefaultRecentWindow = 24 * time.Hour

// Options 控制一次状态聚合。
type Options struct {
	Now              time.Time
	EventsPath       string
	Agent            string
	Cwd              string
	IncludeSubagents bool
	// RecentWindow 限制进入快照的会话；零值或负值使用 DefaultRecentWindow。
	RecentWindow time.Duration
}
