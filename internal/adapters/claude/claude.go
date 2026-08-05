// Package claude 实现 Claude Code 适配器。
package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/extract"
	"github.com/talea/talea/internal/model"
)

const (
	displayName = "Claude Code"
	formatName  = "claude-jsonl"
)

// Adapter 实现 Claude Code 会话读取。
type Adapter struct{}

// New 创建适配器。
func New() *Adapter { return &Adapter{} }

// Info 返回适配器信息与能力。
func (a *Adapter) Info() model.AdapterInfo {
	return model.AdapterInfo{
		ID:          model.AgentClaudeCode,
		DisplayName: displayName,
		Version:     versionOf(),
		Capabilities: []model.Capability{
			model.CapabilityDiscoverSessions,
			model.CapabilityReadMessages,
			model.CapabilityResume,
			model.CapabilityWorkingDirectory,
			model.CapabilitySubagents,
			model.CapabilityActiveDetection,
			model.CapabilityIncrementalIndex,
			model.CapabilityTokenSummary,
			model.CapabilityTokenTimeline,
		},
	}
}

func versionOf() string {
	p, err := exec.LookPath("claude")
	if err != nil {
		return ""
	}
	cmd := exec.Command(p, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// Detect 探测本机安装。
func (a *Adapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	bin, err := exec.LookPath("claude")
	if err != nil {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".claude")
	inst := model.AgentInstance{
		InstanceID:     "claude-code-default",
		AgentID:        model.AgentClaudeCode,
		DisplayName:    displayName,
		Vendor:         "Anthropic",
		ExecutablePath: bin,
		Version:        versionOf(),
		DataDirectory:  dataDir,
		ConfigPath:     filepath.Join(dataDir, "settings.json"),
	}
	return []model.AgentInstance{inst}, nil
}

// Discover 发现 projects 目录下的会话文件。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	projectsDir := filepath.Join(inst.DataDirectory, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []adapters.SessionSource
	for _, dir := range entries {
		if !dir.IsDir() {
			continue
		}
		files, err := os.ReadDir(filepath.Join(projectsDir, dir.Name()))
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			sessionID := strings.TrimSuffix(f.Name(), ".jsonl")
			out = append(out, adapters.SessionSource{
				SessionID: sessionID,
				Path:      filepath.Join(projectsDir, dir.Name(), f.Name()),
				SourceID:  sessionID,
				Mtime:     info.ModTime().Unix(),
				Size:      info.Size(),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// jsonLine 是 JSONL 行的宽松结构。
type jsonLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"sessionId"`
	GitBranch string          `json:"gitBranch"`
	Message   json.RawMessage `json:"message"`
}

// ParseMetadata 流式解析单个会话文件元数据。
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
		SessionID:       src.SessionID,
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
		usageSum         = &model.TokenUsage{Source: model.UsageSourceMessageMetadata}
	)
	for {
		o, ok, err := r.Next()
		if err != nil {
			continue // 单行损坏跳过
		}
		if !ok {
			break
		}
		var line jsonLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		ts, hasTS := parseTime(line.Timestamp)
		if hasTS && (lastTs == nil || ts.After(*lastTs)) {
			lastTs = &ts
		}
		if s.WorkingDirectory == "" && line.Cwd != "" {
			s.WorkingDirectory = line.Cwd
			s.WorkingDirSource = string(model.TimeSourceSessionMeta)
		}
		if s.GitBranch == "" {
			s.GitBranch = line.GitBranch
		}
		switch line.Type {
		case "user":
			s.UserMessageCount++
			if !firstQuestionSet {
				if q, ok := questionFromLine(&line); ok {
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
		case "assistant":
			s.MessageCount++
			if u := parseUsage(line.Message); u != nil {
				// input 为累计上下文值，output 为增量
				if u.InputTokens != nil && *u.InputTokens > 0 {
					usageSum.InputTokens = u.InputTokens
				}
				if u.CacheReadTokens != nil {
					usageSum.CacheReadTokens = u.CacheReadTokens
				}
				if u.CacheWriteTokens != nil {
					usageSum.CacheWriteTokens = u.CacheWriteTokens
				}
				if u.ReasoningTokens != nil {
					usageSum.ReasoningTokens = addInt(usageSum.ReasoningTokens, u.ReasoningTokens)
				}
				if u.OutputTokens != nil {
					usageSum.OutputTokens = addInt(usageSum.OutputTokens, u.OutputTokens)
				}
				if m, ok := u.RawFields["model"].(string); ok && m != "" {
					if s.FormatVersion == "" {
						s.FormatVersion = m
					}
					if usageSum.RawFields == nil {
						usageSum.RawFields = map[string]any{}
					}
					usageSum.RawFields["model"] = m
				}
			}
		}
	}

	if s.StartedAt == nil {
		s.StartTimeSource = model.TimeSourceFileMtime
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
	if usageSum.InputTokens != nil && usageSum.OutputTokens != nil {
		total := *usageSum.InputTokens + *usageSum.OutputTokens
		usageSum.TotalTokens = &total
	}
	if usageSum.TotalTokens != nil || usageSum.InputTokens != nil || usageSum.OutputTokens != nil {
		s.HasTokenUsage = true
		s.TokenUsage = usageSum
	}
	s.UpdatedAt = time.Now()
	return s, nil
}

// questionFromLine 从 user 消息提取首次提问。
func questionFromLine(line *jsonLine) (string, bool) {
	if len(line.Message) == 0 {
		return "", false
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(line.Message, &m); err != nil {
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
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return "", false
	}
	var texts []string
	for _, b := range blocks {
		if b.Type == "text" && b.Text != "" {
			texts = append(texts, b.Text)
		}
	}
	return extract.FirstNonInjected(texts)
}

// parseTime 解析 ISO8601 时间或 epoch。
func parseTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		if n > 1e12 {
			return time.UnixMilli(n), true
		}
		return time.Unix(n, 0), true
	}
	return time.Time{}, false
}

// usageMsg 是 assistant 消息里的 usage 字段。
// input_tokens 为累计上下文值，output_tokens 为单次增量。
type usageMsg struct {
	Input         *int64 `json:"input_tokens"`
	Output        *int64 `json:"output_tokens"`
	CacheRead     *int64 `json:"cache_read_input_tokens"`
	CacheCreation *int64 `json:"cache_creation_input_tokens"`
	Reasoning     *int64 `json:"reasoning_tokens"`
}

func parseUsage(msg json.RawMessage) *model.TokenUsage {
	if len(msg) == 0 {
		return nil
	}
	var m struct {
		Usage usageMsg `json:"usage"`
		Model string   `json:"model"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		return nil
	}
	u := &model.TokenUsage{Source: model.UsageSourceMessageMetadata}
	u.InputTokens = m.Usage.Input
	u.OutputTokens = m.Usage.Output
	u.CacheReadTokens = m.Usage.CacheRead
	u.CacheWriteTokens = m.Usage.CacheCreation
	u.ReasoningTokens = m.Usage.Reasoning
	if m.Model != "" {
		u.RawFields = map[string]any{"model": m.Model}
	}
	if u.InputTokens == nil && u.OutputTokens == nil {
		return nil
	}
	return u
}

func addInt(a, b *int64) *int64 {
	if a == nil && b == nil {
		return nil
	}
	if a == nil {
		v := *b
		return &v
	}
	if b == nil {
		v := *a
		return &v
	}
	v := *a + *b
	return &v
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
		var line jsonLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		if !opts.ShowSystem && line.Type == "system" {
			continue
		}
		m := adapters.Message{Role: line.Type}
		if ts, ok := parseTime(line.Timestamp); ok {
			m.Timestamp = ts.Unix()
		}
		m.Content = messageText(&line)
		msgs = append(msgs, m)
	}
	r.Close()
	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}
	return &sliceIterator{msgs: msgs}, nil
}

func messageText(line *jsonLine) string {
	if len(line.Message) == 0 {
		return ""
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(line.Message, &m); err != nil {
		return ""
	}
	var str string
	if err := json.Unmarshal(m.Content, &str); err == nil {
		return str
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(m.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		if b.Text != "" {
			parts = append(parts, b.Text)
		}
	}
	return strings.Join(parts, "\n")
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

// BuildResumeCommand 构造恢复命令。
func (a *Adapter) BuildResumeCommand(s model.Session, cwd string) (adapters.Command, error) {
	if s.SessionID == "" {
		return adapters.Command{}, fmt.Errorf("会话 ID 为空")
	}
	return adapters.Command{
		Program: "claude",
		Args:    []string{"--resume", s.SessionID},
	}, nil
}

// LoadUsage 返回会话 Token 汇总。
func (a *Adapter) LoadUsage(ctx context.Context, s model.Session) (*model.TokenUsage, error) {
	if s.TokenUsage != nil {
		return s.TokenUsage, nil
	}
	return nil, nil
}

// IterateUsageEvents 从 JSONL 提取时间线事件。
// user 消息生成 user_message 事件；assistant 消息带 usage 生成 request 事件。
func (a *Adapter) IterateUsageEvents(
	ctx context.Context,
	s model.Session,
) (adapters.UsageEventIterator, error) {
	r, err := extract.OpenJSONL(s.SourcePath)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	var events []*model.UsageTimelineEvent
	seq := int64(0)
	for {
		o, ok, err := r.Next()
		if err != nil {
			continue
		}
		if !ok {
			break
		}
		var line jsonLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		ts, hasTS := parseTime(line.Timestamp)
		if line.Type == "user" {
			preview, hasQ := questionFromLine(&line)
			if !hasQ {
				preview = ""
			}
			ev := &model.UsageTimelineEvent{
				AgentInstanceID:   s.AgentInstanceID,
				SessionID:         s.SessionID,
				EventID:           "claude-user-" + itoa(seq),
				EventType:         model.UsageEventUserMessage,
				Sequence:          seq,
				UserPromptPreview: preview,
				Source:            model.UsageSourceMessageMetadata,
				Completeness:      model.UsageMissing,
				SourceIdentity:    "claude-msg-" + itoa(seq) + "-user",
			}
			if hasTS {
				ev.Timestamp = &ts
			}
			events = append(events, ev)
			seq++
			continue
		}
		if line.Type == "assistant" {
			u := parseUsage(line.Message)
			if u == nil {
				continue
			}
			ev := &model.UsageTimelineEvent{
				AgentInstanceID: s.AgentInstanceID,
				SessionID:       s.SessionID,
				EventType:       model.UsageEventRequest,
				Sequence:        seq,
				InputTokens:     u.InputTokens,
				OutputTokens:    u.OutputTokens,
				TotalTokens:     u.TotalTokens,
				Source:          model.UsageSourceMessageMetadata,
				Completeness:    model.UsageComplete,
				SourceIdentity:  "claude-req-" + itoa(seq),
			}
			if hasTS {
				ev.Timestamp = &ts
			}
			events = append(events, ev)
			seq++
		}
	}
	return &eventIterator{events: events}, nil
}

type eventIterator struct {
	events []*model.UsageTimelineEvent
	idx    int
}

func (it *eventIterator) Next() (*model.UsageTimelineEvent, bool, error) {
	if it.idx >= len(it.events) {
		return nil, false, nil
	}
	e := it.events[it.idx]
	it.idx++
	return e, true, nil
}

func (it *eventIterator) Close() error { return nil }

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[pos:])
}

// DetectActivity 检测会话活动状态（进程 + 文件更新时间）。
func (a *Adapter) DetectActivity(ctx context.Context, s model.Session) (model.ActivityState, error) {
	return adapters.ProcessActivityDetector{Executable: "claude"}.DetectActivity(ctx, s)
}

// ResolveSessionRelations 通过 subagents/ 目录推断父子关系。
// 子会话文件路径 <projectDir>/<parentId>/subagents/agent-<id>.jsonl，
// 父会话 ID 从路径父目录得出。
func (a *Adapter) ResolveSessionRelations(
	ctx context.Context,
	sessions []model.Session,
) ([]model.SessionRelation, error) {
	var out []model.SessionRelation
	for _, s := range sessions {
		if s.SourcePath == "" {
			continue
		}
		// 子会话文件位于 <parentDir>/<parentId>/subagents/<file>.jsonl
		parts := strings.Split(filepath.ToSlash(s.SourcePath), "/")
		if len(parts) < 2 {
			continue
		}
		subagentsIdx := -1
		for i := len(parts) - 1; i >= 0; i-- {
			if parts[i] == "subagents" {
				subagentsIdx = i
				break
			}
		}
		if subagentsIdx <= 0 {
			continue
		}
		parentID := parts[subagentsIdx-1]
		if parentID == "" || parentID == s.SessionID {
			continue
		}
		out = append(out, model.SessionRelation{
			ParentAgentInstanceID: s.AgentInstanceID,
			ParentSessionID:       parentID,
			ChildAgentInstanceID:  s.AgentInstanceID,
			ChildSessionID:        s.SessionID,
		})
	}
	return out, nil
}
