package claude

import (
	"encoding/json"
	"time"

	"github.com/talea/talea/internal/model"
)

// turnSnapshot 记录解析过程中最近一次有效回合状态。
type turnSnapshot struct {
	state  model.TurnState
	at     *time.Time
	source model.TurnStateSource
	detail string
}

// apply 按事件时间更新回合状态；较旧的事件不覆盖较新的状态。
func (t *turnSnapshot) apply(state model.TurnState, source model.TurnStateSource, detail string, at time.Time, hasAt bool) {
	if hasAt {
		if t.at != nil && at.Before(*t.at) {
			return
		}
		ts := at
		t.at = &ts
	} else if t.at != nil {
		return
	}
	t.state = state
	t.source = source
	t.detail = detail
}

// assistantMeta 是 assistant 消息中与回合状态相关的字段。
type assistantMeta struct {
	StopReason        string `json:"stop_reason"`
	IsAPIErrorMessage bool   `json:"isAPIErrorMessage"`
	Content           []struct {
		Type string `json:"type"`
	} `json:"content"`
}

// parseAssistantMeta 提取 stop_reason、tool_use 与 API 错误标识。
func parseAssistantMeta(msg json.RawMessage) assistantMeta {
	var m assistantMeta
	if len(msg) == 0 {
		return m
	}
	_ = json.Unmarshal(msg, &m)
	return m
}

// hasToolUse 判断消息内容中是否包含 tool_use 块。
func (m assistantMeta) hasToolUse() bool {
	for _, block := range m.Content {
		if block.Type == "tool_use" {
			return true
		}
	}
	return false
}
