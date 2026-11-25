package model

import "time"

// TurnState 描述会话最近一轮的通用回合状态。
// 适配器负责把各 Agent 的原生事件映射到这里，核心层不感知 Agent 差异。
type TurnState string

const (
	TurnStateWorking      TurnState = "working"
	TurnStateWaitingInput TurnState = "waiting_input"
	TurnStateCompleted    TurnState = "completed"
	TurnStateFailed       TurnState = "failed"
	TurnStateInterrupted  TurnState = "interrupted"
	TurnStateIdle         TurnState = "idle"
	TurnStateUnknown      TurnState = "unknown"
)

// Valid 判断状态是否属于受支持的通用取值。
func (s TurnState) Valid() bool {
	switch s {
	case TurnStateWorking, TurnStateWaitingInput, TurnStateCompleted,
		TurnStateFailed, TurnStateInterrupted, TurnStateIdle, TurnStateUnknown:
		return true
	default:
		return false
	}
}

// TurnStateSource 描述回合状态的来源。
type TurnStateSource string

const (
	// TurnStateSourceNative 表示状态直接来自 Agent 会话记录中的原生事件。
	TurnStateSourceNative TurnStateSource = "native"
	// TurnStateSourceInferred 表示状态由消息/活动等间接证据推断。
	TurnStateSourceInferred TurnStateSource = "inferred"
	// TurnStateSourceHook 表示状态来自用户显式配置的 hook/plugin 精确事件。
	TurnStateSourceHook TurnStateSource = "hook"
	// TurnStateSourceUnknown 表示没有可用来源。
	TurnStateSourceUnknown TurnStateSource = "unknown"
)

// Valid 判断来源是否属于受支持的通用取值。
func (s TurnStateSource) Valid() bool {
	switch s {
	case TurnStateSourceNative, TurnStateSourceInferred,
		TurnStateSourceHook, TurnStateSourceUnknown:
		return true
	default:
		return false
	}
}

// TurnStateRecord 是索引中持久化的回合状态。
type TurnStateRecord struct {
	AgentInstanceID string
	SessionID       string
	State           TurnState
	StateAt         *time.Time
	Source          TurnStateSource
	Detail          string
}

// TurnStateKey 返回回合状态在索引中的复合键。
func TurnStateKey(agentInstanceID, sessionID string) string {
	return agentInstanceID + "\x00" + sessionID
}
