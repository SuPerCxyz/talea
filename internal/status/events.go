package status

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/talea/talea/internal/model"
)

const (
	// EventFileName 是可选精确事件文件，位于 Talea 数据目录。
	EventFileName = "status-events.jsonl"
	// MaxEventReadBytes 限制尾部读取大小；事件量很小，避免超大文件拖慢刷新。
	MaxEventReadBytes = 1 << 20
)

// Event 是 hook / plugin 写入的精确状态事件。
type Event struct {
	AgentID         string          `json:"agent_id"`
	AgentInstanceID string          `json:"agent_instance_id,omitempty"`
	SessionID       string          `json:"session_id"`
	State           model.TurnState `json:"state"`
	At              time.Time       `json:"at"`
	Detail          string          `json:"detail,omitempty"`
}

// EventIndex 是每个会话最新精确事件的索引。
type EventIndex struct {
	byInstance map[string]Event
	byAgent    map[string]Event
}

// AppendEvent 追加一条 JSONL 事件。目录 0700、文件 0600。
func AppendEvent(path string, event Event) error {
	if path == "" {
		return fmt.Errorf("状态事件路径为空")
	}
	dir := filepath.Dir(path)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("创建状态事件目录: %w", err)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return fmt.Errorf("设置状态事件目录权限 0700: %w", err)
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o600)
	if err != nil {
		return fmt.Errorf("打开状态事件文件: %w", err)
	}
	defer f.Close()
	if err := f.Chmod(0o600); err != nil {
		return fmt.Errorf("设置状态事件文件权限 0600: %w", err)
	}
	if info, statErr := f.Stat(); statErr == nil && info.Size() > 0 {
		var last [1]byte
		if _, readErr := f.ReadAt(last[:], info.Size()-1); readErr == nil && last[0] != '\n' {
			// 兼容用户/其他工具写入但缺少结尾换行的文件，避免两行 JSON 粘连。
			if _, writeErr := f.Write([]byte{'\n'}); writeErr != nil {
				return fmt.Errorf("修复状态事件行尾: %w", writeErr)
			}
		}
	}
	line, err := json.Marshal(event)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if _, err := f.Write(line); err != nil {
		return fmt.Errorf("写入状态事件: %w", err)
	}
	return nil
}

// LoadLatestEvents 读取事件文件尾部，返回每个会话的最新有效事件。
// 文件不存在或为空时返回空索引；无法解析的行跳过。
func LoadLatestEvents(path string) (EventIndex, error) {
	idx := EventIndex{
		byInstance: map[string]Event{},
		byAgent:    map[string]Event{},
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return idx, nil
		}
		return idx, err
	}
	defer f.Close()

	offset := int64(0)
	if info, statErr := f.Stat(); statErr == nil && info.Size() > MaxEventReadBytes {
		offset = info.Size() - MaxEventReadBytes
		if _, seekErr := f.Seek(offset, io.SeekStart); seekErr != nil {
			return idx, seekErr
		}
	}
	data, err := io.ReadAll(io.LimitReader(f, MaxEventReadBytes))
	if err != nil {
		return idx, err
	}
	lines := strings.Split(string(data), "\n")
	if offset > 0 && len(lines) > 0 {
		lines = lines[1:] // 丢弃被截断的首行
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}
		if !event.State.Valid() || event.State == model.TurnStateUnknown ||
			event.SessionID == "" || event.AgentID == "" || event.At.IsZero() {
			continue
		}
		event.AgentID = string(CanonicalAgent(event.AgentID))
		idx.add(event)
	}
	return idx, nil
}

func (idx *EventIndex) add(event Event) {
	key := eventKey(event.AgentInstanceID, event.AgentID, event.SessionID)
	if event.AgentInstanceID != "" {
		if old, ok := idx.byInstance[key]; !ok || !event.At.Before(old.At) {
			idx.byInstance[key] = event
		}
		return
	}
	if old, ok := idx.byAgent[key]; !ok || !event.At.Before(old.At) {
		idx.byAgent[key] = event
	}
}

// Latest 查找会话的最新精确事件：优先实例精确匹配，再按 Agent + 会话回退。
func (idx EventIndex) Latest(agentID model.AgentID, instanceID, sessionID string) (Event, bool) {
	if event, ok := idx.byInstance[eventKey(instanceID, string(agentID), sessionID)]; ok {
		return event, true
	}
	event, ok := idx.byAgent[eventKey("", string(agentID), sessionID)]
	return event, ok
}

func eventKey(instanceID, agentID, sessionID string) string {
	return instanceID + "\x00" + agentID + "\x00" + sessionID
}
