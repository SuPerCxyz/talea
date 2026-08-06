// Package generic 提供通用 JSONL 适配器，作为新增 Agent 的模板实现。
//
// 用途：
//  1. 展示新增 Agent 适配器的最小实现方式（参考 docs/adapters/adding-an-agent.md）。
//  2. 可处理任何「JSONL 每行含 type/timestamp/message」结构的会话，
//     通过 SessionSource 的 SourceID 识别 session。
//
// 注意：本适配器为模板/示例实现，未绑定真实 Agent 格式，字段名基于
// Claude Code 通用结构（type, timestamp, cwd, sessionId, message）。
package generic

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/extract"
	"github.com/talea/talea/internal/model"
)

const (
	displayName = "Generic JSONL"
	formatName  = "generic-jsonl"
)

// Adapter 实现通用 JSONL 适配器。
type Adapter struct{}

// New 创建适配器。
func New() *Adapter { return &Adapter{} }

// ID 返回适配器标识（常量，不触发外部探测）。
func (a *Adapter) ID() model.AgentID { return "generic-jsonl" }

// Info 返回适配器信息与能力。
func (a *Adapter) Info() model.AdapterInfo {
	return model.AdapterInfo{
		ID:          "generic-jsonl",
		DisplayName: displayName,
		Capabilities: []model.Capability{
			model.CapabilityDiscoverSessions,
			model.CapabilityReadMessages,
			model.CapabilityResume,
			model.CapabilityWorkingDirectory,
			model.CapabilityIncrementalIndex,
		},
	}
}

// Detect 探测：由配置指定数据目录（data_dirs）。
func (a *Adapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	// 通用适配器无默认安装路径，仅当配置显式启用时被索引。
	return nil, nil
}

// Discover 扫描配置的数据目录下的 *.jsonl。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	if inst.DataDirectory == "" {
		return nil, nil
	}
	entries, err := os.ReadDir(inst.DataDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []adapters.SessionSource
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		out = append(out, adapters.SessionSource{
			Path:     filepath.Join(inst.DataDirectory, e.Name()),
			SourceID: e.Name(),
			Mtime:    info.ModTime().Unix(),
			Size:     info.Size(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// line 是通用 JSONL 行的宽松结构。
type line struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
}

// ParseMetadata 解析通用 JSONL 会话。
func (a *Adapter) ParseMetadata(
	ctx context.Context,
	inst model.AgentInstance,
	src adapters.SessionSource,
) (*model.Session, error) {
	r, err := extract.OpenJSONL(src.Path)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	s := &model.Session{
		AgentID:         inst.AgentID,
		AgentInstanceID: inst.InstanceID,
		SessionID:       "", // 先读 JSONL 内 sessionId，再回退文件名
		FormatName:      formatName,
		SourcePath:      src.Path,
		SourceID:        src.SourceID,
		SourceMtime:     src.Mtime,
		SourceSize:      src.Size,
		Activity:        model.ActivityInactive,
	}

	var (
		firstQuestionSet bool
		lastTs           *time.Time
	)
	for {
		o, ok, err := r.Next()
		if err != nil {
			continue
		}
		if !ok {
			break
		}
		var l line
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &l); err != nil {
				continue
			}
		}
		ts, hasTS := parseTime(l.Timestamp)
		if hasTS && (lastTs == nil || ts.After(*lastTs)) {
			lastTs = &ts
		}
		if s.SessionID == "" && l.SessionID != "" {
			s.SessionID = l.SessionID
		}
		if s.WorkingDirectory == "" && l.Cwd != "" {
			s.WorkingDirectory = l.Cwd
			s.WorkingDirSource = string(model.TimeSourceSessionMeta)
		}
		if l.Type == "user" && !firstQuestionSet {
			if q, ok := questionFromLine(&l); ok {
				s.FirstQuestion = q
				s.FirstQuestionSource = "user_message"
				s.FirstQuestionConfidence = 1.0
				firstQuestionSet = true
				if hasTS {
					s.StartedAt = &ts
					s.StartTimeSource = model.TimeSourceFirstUserMsg
				}
			}
		}
	}

	if s.StartedAt == nil {
		s.StartTimeSource = model.TimeSourceFileMtime
	}
	if s.SessionID == "" {
		s.SessionID = sessionIDOf(src)
	}
	if lastTs != nil {
		s.LastActivityAt = lastTs
		s.EndedAt = lastTs
		s.EndTimeSource = model.TimeSourceLastActivity
		if s.StartedAt != nil {
			d := lastTs.Sub(*s.StartedAt)
			s.Duration = &d
		}
	}
	s.UpdatedAt = time.Now()
	return s, nil
}

func sessionIDOf(src adapters.SessionSource) string {
	if src.SessionID != "" {
		return src.SessionID
	}
	base := filepath.Base(src.Path)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

// questionFromLine 从 user 消息提取首次提问。
func questionFromLine(l *line) (string, bool) {
	if len(l.Message) == 0 {
		return "", false
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(l.Message, &m); err != nil {
		return "", false
	}
	var str string
	if err := json.Unmarshal(m.Content, &str); err == nil {
		cleaned := extract.StripInjectedContent(str)
		if cleaned != "" && !extract.IsInjectedBlock(cleaned) {
			return cleaned, true
		}
		return "", false
	}
	return "", false
}

func parseTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// LoadMessages 读取消息预览。
func (a *Adapter) LoadMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	r, err := extract.OpenJSONL(s.SourcePath)
	if err != nil {
		return nil, err
	}
	var msgs []adapters.Message
	for {
		o, ok, err := r.Next()
		if err != nil {
			continue
		}
		if !ok {
			break
		}
		var l line
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &l); err != nil {
				continue
			}
		}
		m := adapters.Message{Role: l.Type}
		if ts, ok := parseTime(l.Timestamp); ok {
			m.Timestamp = ts.Unix()
		}
		m.Content = messageText(&l)
		msgs = append(msgs, m)
	}
	r.Close()
	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}
	return &sliceIterator{msgs: msgs}, nil
}

func messageText(l *line) string {
	if len(l.Message) == 0 {
		return ""
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(l.Message, &m); err != nil {
		return ""
	}
	var str string
	if err := json.Unmarshal(m.Content, &str); err == nil {
		return str
	}
	return ""
}

type sliceIterator struct {
	msgs []adapters.Message
	idx  int
}

func (it *sliceIterator) Next() (adapters.Message, bool, error) {
	if it.idx >= len(it.msgs) {
		return adapters.Message{}, false, nil
	}
	m := it.msgs[it.idx]
	it.idx++
	return m, true, nil
}

func (it *sliceIterator) Close() error { return nil }

// BuildResumeCommand 构造恢复命令（需配置程序名）。
func (a *Adapter) BuildResumeCommand(s model.Session, cwd string) (adapters.Command, error) {
	if s.ResumeProgram == "" {
		return adapters.Command{}, fmt.Errorf("generic 适配器需要配置恢复程序")
	}
	return adapters.Command{Program: s.ResumeProgram, Args: s.ResumeArgs}, nil
}
