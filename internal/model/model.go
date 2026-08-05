// Package model 定义 Talea 的统一数据模型，不依赖任何 Agent 实现。
package model

import "time"

// AgentID 是开放式的 Agent 标识字符串，允许新增 Agent 时定义新值。
type AgentID string

const (
	AgentClaudeCode AgentID = "claude-code"
	AgentCodexCLI   AgentID = "codex-cli"
	AgentOpenCode   AgentID = "opencode"
)

// AgentInstance 描述同一个 Agent 的一个安装实例或数据目录。
type AgentInstance struct {
	InstanceID string
	AgentID    AgentID

	DisplayName string
	Vendor      string

	ExecutablePath string
	Version        string

	DataDirectory string
	ConfigPath    string
}

// Capability 声明适配器支持的能力，界面根据能力动态展示。
type Capability string

const (
	CapabilityDiscoverSessions Capability = "discover_sessions"
	CapabilityReadMessages     Capability = "read_messages"
	CapabilityResume           Capability = "resume"

	CapabilityWorkingDirectory Capability = "working_directory"
	CapabilitySubagents        Capability = "subagents"
	CapabilityActiveDetection  Capability = "active_detection"
	CapabilityIncrementalIndex Capability = "incremental_index"

	CapabilityTokenSummary     Capability = "token_summary"
	CapabilityTokenTimeline    Capability = "token_timeline"
	CapabilityContextHistory   Capability = "context_history"
	CapabilityCompactionEvents Capability = "compaction_events"
	CapabilityCostTimeline     Capability = "cost_timeline"

	CapabilityExactStartTime Capability = "exact_start_time"
	CapabilityExactEndTime   Capability = "exact_end_time"
)

// AdapterInfo 描述适配器的静态信息。
type AdapterInfo struct {
	ID           AgentID
	DisplayName  string
	Version      string
	Capabilities []Capability
}

// TimeSource 记录时间字段的提取来源。
type TimeSource string

const (
	TimeSourceProcessStart TimeSource = "process_start"
	TimeSourceProcessExit  TimeSource = "process_exit"
	TimeSourceSessionMeta  TimeSource = "session_metadata"
	TimeSourceFirstUserMsg TimeSource = "first_user_message"
	TimeSourceFirstEvent   TimeSource = "first_event"
	TimeSourceLastActivity TimeSource = "last_activity"
	TimeSourceFileMtime    TimeSource = "file_mtime"
	TimeSourceUnknown      TimeSource = "unknown"
)

// ActivityState 描述会话当前活动状态。
type ActivityState string

const (
	ActivityActive         ActivityState = "active"
	ActivityPossiblyActive ActivityState = "possibly_active"
	ActivityInactive       ActivityState = "inactive"
	ActivityUnknown        ActivityState = "unknown"
)

// Session 是统一会话模型。
type Session struct {
	AgentID         AgentID
	AgentInstanceID string
	SessionID       string

	FormatName    string
	FormatVersion string

	FirstQuestion           string
	FirstQuestionSource     string
	FirstQuestionConfidence float64

	StartedAt       *time.Time
	EndedAt         *time.Time
	LastActivityAt  *time.Time
	Duration        *time.Duration
	StartTimeSource TimeSource
	EndTimeSource   TimeSource

	WorkingDirectory string
	WorkingDirSource string
	WorkingDirExists bool

	ProjectName string
	GitRoot     string
	GitBranch   string
	GitRemote   string

	MessageCount     int64
	UserMessageCount int64
	ToolCallCount    int64

	ParentSessionID string
	IsSubagent      bool

	Activity ActivityState

	SourcePath   string
	SourceID     string
	SourceMtime  int64
	SourceSize   int64
	SourceOffset int64

	HasTokenUsage bool
	TokenUsage    *TokenUsage

	ResumeProgram string
	ResumeArgs    []string

	IndexedAt time.Time
	UpdatedAt time.Time
}
