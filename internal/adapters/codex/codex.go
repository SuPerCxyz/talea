// Package codex 实现 Codex CLI 适配器。
package codex

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
	displayName = "Codex CLI"
	formatName  = "codex-rollout-jsonl"
)

// Adapter 实现 Codex CLI 会话读取。
type Adapter struct{}

// New 创建适配器。
func New() *Adapter { return &Adapter{} }

// Info 返回适配器信息与能力。
func (a *Adapter) Info() model.AdapterInfo {
	return model.AdapterInfo{
		ID:          model.AgentCodexCLI,
		DisplayName: displayName,
		Version:     versionOf(),
		Capabilities: []model.Capability{
			model.CapabilityDiscoverSessions,
			model.CapabilityReadMessages,
			model.CapabilityResume,
			model.CapabilityWorkingDirectory,
			model.CapabilityActiveDetection,
			model.CapabilityIncrementalIndex,
			model.CapabilityTokenSummary,
			model.CapabilityTokenTimeline,
		},
	}
}

func versionOf() string {
	p, err := exec.LookPath("codex")
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
	bin, err := exec.LookPath("codex")
	if err != nil {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".codex")
	inst := model.AgentInstance{
		InstanceID:     "codex-cli-default",
		AgentID:        model.AgentCodexCLI,
		DisplayName:    displayName,
		Vendor:         "OpenAI",
		ExecutablePath: bin,
		Version:        versionOf(),
		DataDirectory:  dataDir,
		ConfigPath:     filepath.Join(dataDir, "config.toml"),
	}
	return []model.AgentInstance{inst}, nil
}

// Discover 递归发现 sessions/<Y>/<M>/<D>/rollout-*.jsonl。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	sessionsDir := filepath.Join(inst.DataDirectory, "sessions")
	if _, err := os.Stat(sessionsDir); err != nil {
		return nil, nil
	}
	var out []adapters.SessionSource
	err := filepath.WalkDir(sessionsDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		name := d.Name()
		if !strings.HasPrefix(name, "rollout-") || !strings.HasSuffix(name, ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		out = append(out, adapters.SessionSource{
			SessionID: sessionIDFromFilename(path),
			Path:      path,
			SourceID:  name,
			Mtime:     info.ModTime().Unix(),
			Size:      info.Size(),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

// rolloutLine 是 rollout JSONL 的宽松结构。
type rolloutLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

// sessionMeta 是 session_meta payload。
type sessionMeta struct {
	ID            string `json:"id"`
	SessionID     string `json:"session_id"`
	Cwd           string `json:"cwd"`
	CLIVersion    string `json:"cli_version"`
	ModelProvider string `json:"model_provider"`
	Git           struct {
		Branch        string `json:"branch"`
		CommitHash    string `json:"commit_hash"`
		RepositoryURL string `json:"repository_url"`
	} `json:"git"`
}

// ParseMetadata 解析单个 rollout 文件元数据。
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
		requestCount     int64
	)
	for {
		o, ok, err := r.Next()
		if err != nil {
			continue
		}
		if !ok {
			break
		}
		var line rolloutLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		ts, hasTS := parseTime(line.Timestamp)
		if hasTS && (lastTs == nil || ts.After(*lastTs)) {
			lastTs = &ts
		}
		if line.Type == "session_meta" {
			var meta sessionMeta
			if err := json.Unmarshal(line.Payload, &meta); err == nil {
				if meta.SessionID != "" {
					s.SessionID = meta.SessionID
				} else if meta.ID != "" {
					s.SessionID = meta.ID
				}
				if s.SessionID == "" && src.SessionID != "" {
					s.SessionID = src.SessionID
				}
				if s.SessionID == "" {
					// 从文件名推测：rollout-<ts>-<uuid>
					s.SessionID = sessionIDFromFilename(src.Path)
				}
				s.WorkingDirectory = meta.Cwd
				s.WorkingDirSource = string(model.TimeSourceSessionMeta)
				s.GitBranch = meta.Git.Branch
				s.GitRemote = meta.Git.RepositoryURL
				s.FormatVersion = meta.CLIVersion
				if hasTS {
					s.StartedAt = &ts
					s.StartTimeSource = model.TimeSourceSessionMeta
				}
			}
		}
		if line.Type == "response_item" {
			var item struct {
				Type    string            `json:"type"`
				Role    string            `json:"role"`
				Content []json.RawMessage `json:"content"`
			}
			_ = json.Unmarshal(line.Payload, &item)
			if item.Type == "message" && item.Role == "user" && !firstQuestionSet {
				if q, ok := userQuestion(item.Content); ok {
					s.FirstQuestion = q
					s.FirstQuestionSource = "user_message"
					s.FirstQuestionConfidence = 1.0
					firstQuestionSet = true
					if s.StartedAt == nil && hasTS {
						s.StartedAt = &ts
						s.StartTimeSource = model.TimeSourceFirstUserMsg
					}
				}
			}
			if item.Role == "user" {
				s.UserMessageCount++
			}
		}
		if line.Type == "event_msg" {
			var ev struct {
				Type string          `json:"type"`
				Info json.RawMessage `json:"info"`
			}
			_ = json.Unmarshal(line.Payload, &ev)
			if ev.Type == "token_count" {
				total, _ := parseTokenCount(ev.Info)
				if total != nil && total.InputTokens != nil && *total.InputTokens > 0 {
					// total_token_usage 为会话累计值：直接覆盖（取最后一个）
					usageSum.InputTokens = total.InputTokens
					usageSum.OutputTokens = total.OutputTokens
					usageSum.TotalTokens = total.TotalTokens
					usageSum.CacheReadTokens = total.CacheReadTokens
					usageSum.CacheWriteTokens = total.CacheWriteTokens
					usageSum.ReasoningTokens = total.ReasoningTokens
					requestCount++
				}
			}
		}
	}

	if s.SessionID == "" {
		s.SessionID = sessionIDFromFilename(src.Path)
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
	if usageSum.TotalTokens != nil || usageSum.InputTokens != nil || usageSum.OutputTokens != nil {
		s.HasTokenUsage = true
		usageSum.RequestCount = &requestCount
		s.TokenUsage = usageSum
	}
	s.UpdatedAt = time.Now()
	return s, nil
}

// userQuestion 从 content 块提取第一条真实提问，过滤 AGENTS 注入块。
func userQuestion(content []json.RawMessage) (string, bool) {
	var texts []string
	for _, c := range content {
		var block struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal(c, &block); err != nil {
			continue
		}
		if block.Type == "input_text" && block.Text != "" {
			texts = append(texts, block.Text)
		}
	}
	cleaned := extract.MergeTextBlocks(texts)
	if cleaned == "" {
		return "", false
	}
	return cleaned, true
}

// tokenCountInfo 是 token_count 事件的 info。
// total_token_usage 为会话累计值，last_token_usage 为本次请求增量。
type tokenCountInfo struct {
	TotalTokenUsage tokenUsage `json:"total_token_usage"`
	LastTokenUsage  tokenUsage `json:"last_token_usage"`
}

type tokenUsage struct {
	InputTokens           *int64 `json:"input_tokens"`
	CachedInputTokens     *int64 `json:"cached_input_tokens"`
	CacheWriteInputTokens *int64 `json:"cache_write_input_tokens"`
	OutputTokens          *int64 `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
	TotalTokens           *int64 `json:"total_tokens"`
}

// parseTokenCount 返回 (累计值, 本次增量)。累计值用于会话汇总，增量用于请求级。
func parseTokenCount(info json.RawMessage) (*model.TokenUsage, *model.TokenUsage) {
	var t tokenCountInfo
	if err := json.Unmarshal(info, &t); err != nil {
		return nil, nil
	}
	total := tokenUsageToModel(t.TotalTokenUsage)
	last := tokenUsageToModel(t.LastTokenUsage)
	if total == nil && last == nil {
		return nil, nil
	}
	return total, last
}

func tokenUsageToModel(t tokenUsage) *model.TokenUsage {
	u := &model.TokenUsage{Source: model.UsageSourceMessageMetadata}
	u.InputTokens = t.InputTokens
	u.OutputTokens = t.OutputTokens
	u.TotalTokens = t.TotalTokens
	u.CacheReadTokens = t.CachedInputTokens
	u.CacheWriteTokens = t.CacheWriteInputTokens
	u.ReasoningTokens = t.ReasoningOutputTokens
	if u.InputTokens == nil && u.OutputTokens == nil && u.TotalTokens == nil {
		return nil
	}
	return u
}

// sessionIDFromFilename 从 rollout-<ts>-<uuid>.jsonl 提取 uuid。
func sessionIDFromFilename(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, ".jsonl")
	parts := strings.Split(name, "-")
	// rollout-2026-08-03T14-01-19-019fc636-5bf9-73b3-859f-b835fe86b564
	// 前面是日期段，uuid 在后半部分；直接取最后一段不可靠。
	// 尝试找到像 uuid 的连续片段。
	idx := -1
	for i, p := range parts {
		if len(p) == 8 {
			idx = i
			break
		}
	}
	if idx >= 0 && idx+4 < len(parts) {
		uuid := strings.Join(parts[idx:idx+5], "-")
		if len(uuid) >= 30 {
			return uuid
		}
	}
	return name
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
		var line rolloutLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		if line.Type != "response_item" {
			continue
		}
		var item struct {
			Type    string            `json:"type"`
			Role    string            `json:"role"`
			Content []json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(line.Payload, &item); err != nil || item.Type != "message" {
			continue
		}
		var texts []string
		for _, c := range item.Content {
			var block struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if err := json.Unmarshal(c, &block); err == nil && block.Text != "" {
				texts = append(texts, block.Text)
			}
		}
		m := adapters.Message{Role: item.Role, Content: strings.Join(texts, "\n")}
		if ts, ok := parseTime(line.Timestamp); ok {
			m.Timestamp = ts.Unix()
		}
		msgs = append(msgs, m)
	}
	r.Close()
	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}
	return &sliceIterator{msgs: msgs}, nil
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
		Program: "codex",
		Args:    []string{"resume", s.SessionID},
	}, nil
}

// LoadUsage 返回会话 Token 汇总。
func (a *Adapter) LoadUsage(ctx context.Context, s model.Session) (*model.TokenUsage, error) {
	if s.TokenUsage != nil {
		return s.TokenUsage, nil
	}
	return nil, nil
}

// IterateUsageEvents 从 rollout JSONL 提取时间线事件。
// user 消息生成 user_message 事件；token_count 事件生成 request 事件。
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
		var line rolloutLine
		if b, err := json.Marshal(o); err == nil {
			if err := json.Unmarshal(b, &line); err != nil {
				continue
			}
		}
		ts, hasTS := parseTime(line.Timestamp)
		if line.Type == "response_item" {
			var item struct {
				Type    string            `json:"type"`
				Role    string            `json:"role"`
				Content []json.RawMessage `json:"content"`
			}
			_ = json.Unmarshal(line.Payload, &item)
			if item.Type == "message" && item.Role == "user" {
				q, _ := userQuestion(item.Content)
				ev := &model.UsageTimelineEvent{
					AgentInstanceID:   s.AgentInstanceID,
					SessionID:         s.SessionID,
					EventType:         model.UsageEventUserMessage,
					Sequence:          seq,
					UserPromptPreview: q,
					Source:            model.UsageSourceMessageMetadata,
					Completeness:      model.UsageMissing,
					SourceIdentity:    "codex-msg-" + itoa(seq),
				}
				if hasTS {
					ev.Timestamp = &ts
				}
				events = append(events, ev)
				seq++
			}
		}
		if line.Type == "event_msg" {
			var ev struct {
				Type string          `json:"type"`
				Info json.RawMessage `json:"info"`
			}
			_ = json.Unmarshal(line.Payload, &ev)
			if ev.Type == "token_count" {
				total, last := parseTokenCount(ev.Info)
				if total == nil {
					continue
				}
				e := &model.UsageTimelineEvent{
					AgentInstanceID: s.AgentInstanceID,
					SessionID:       s.SessionID,
					EventType:       model.UsageEventRequest,
					Sequence:        seq,
					Source:          model.UsageSourceMessageMetadata,
					Completeness:    model.UsageComplete,
					SourceIdentity:  "codex-req-" + itoa(seq),
				}
				// last_token_usage 为本次请求增量（input/output）
				if last != nil {
					e.InputTokens = last.InputTokens
					e.OutputTokens = last.OutputTokens
					e.ReasoningTokens = last.ReasoningTokens
					e.CacheReadTokens = last.CacheReadTokens
					e.CacheWriteTokens = last.CacheWriteTokens
				}
				// total_token_usage 为会话累计上下文
				if total.TotalTokens != nil {
					e.ContextAfter = total.TotalTokens
					e.CumulativeTotal = total.TotalTokens
				}
				if hasTS {
					e.Timestamp = &ts
				}
				events = append(events, e)
				seq++
			}
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
	return adapters.ProcessActivityDetector{Executable: "codex"}.DetectActivity(ctx, s)
}
