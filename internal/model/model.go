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
	InstanceID string  `json:"instance_id"`
	AgentID    AgentID `json:"agent_id"`

	DisplayName string `json:"display_name"`
	Vendor      string `json:"vendor"`

	ExecutablePath string `json:"executable_path"`
	Version        string `json:"version"`

	DataDirectory string `json:"data_directory"`
	ConfigPath    string `json:"config_path"`
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
	ID           AgentID      `json:"id"`
	DisplayName  string       `json:"display_name"`
	Version      string       `json:"version"`
	Capabilities []Capability `json:"capabilities"`
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
	AgentID         AgentID `json:"agent_id"`
	AgentInstanceID string  `json:"agent_instance_id"`
	SessionID       string  `json:"session_id"`

	FormatName    string `json:"format_name"`
	FormatVersion string `json:"format_version"`

	FirstQuestion           string  `json:"first_question"`
	FirstQuestionSource     string  `json:"first_question_source"`
	FirstQuestionConfidence float64 `json:"first_question_confidence"`

	// LastUserPrompt 是会话最后一次用户消息的预览（TUI 展示用，非持久化字段）。
	LastUserPrompt string `json:"last_user_prompt,omitempty"`

	StartedAt       *time.Time     `json:"started_at"`
	EndedAt         *time.Time     `json:"ended_at"`
	LastActivityAt  *time.Time     `json:"last_activity_at"`
	Duration        *time.Duration `json:"duration"`
	StartTimeSource TimeSource     `json:"start_time_source"`
	EndTimeSource   TimeSource     `json:"end_time_source"`

	WorkingDirectory string `json:"working_directory"`
	WorkingDirSource string `json:"working_dir_source"`
	WorkingDirExists bool   `json:"working_dir_exists"`

	ProjectName string `json:"project_name"`
	GitRoot     string `json:"git_root"`
	GitBranch   string `json:"git_branch"`
	GitRemote   string `json:"git_remote"`

	MessageCount     int64 `json:"message_count"`
	UserMessageCount int64 `json:"user_message_count"`
	ToolCallCount    int64 `json:"tool_call_count"`

	ParentSessionID string `json:"parent_session_id"`
	IsSubagent      bool   `json:"is_subagent"`

	Activity ActivityState `json:"activity_state"`

	SourcePath   string `json:"source_path"`
	SourceID     string `json:"source_id"`
	SourceMtime  int64  `json:"source_mtime"`
	SourceSize   int64  `json:"source_size"`
	SourceOffset int64  `json:"source_offset"`

	HasTokenUsage bool        `json:"has_token_usage"`
	TokenUsage    *TokenUsage `json:"token_usage,omitempty"`

	ResumeProgram string   `json:"resume_program"`
	ResumeArgs    []string `json:"resume_args"`

	IndexedAt time.Time `json:"indexed_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
