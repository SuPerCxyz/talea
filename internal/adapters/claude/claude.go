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
			model.CapabilityIncrementalIndex,
			model.CapabilityTokenSummary,
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
				accumulateUsage(usageSum, u)
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
type usageMsg struct {
	Input  *int64 `json:"input_tokens"`
	Output *int64 `json:"output_tokens"`
}

func parseUsage(msg json.RawMessage) *model.TokenUsage {
	if len(msg) == 0 {
		return nil
	}
	var m struct {
		Usage usageMsg `json:"usage"`
	}
	if err := json.Unmarshal(msg, &m); err != nil {
		return nil
	}
	u := &model.TokenUsage{Source: model.UsageSourceMessageMetadata}
	u.InputTokens = m.Usage.Input
	u.OutputTokens = m.Usage.Output
	if m.Usage.Input != nil && m.Usage.Output != nil {
		total := *m.Usage.Input + *m.Usage.Output
		u.TotalTokens = &total
	}
	if u.InputTokens == nil && u.OutputTokens == nil {
		return nil
	}
	return u
}

func accumulateUsage(sum, add *model.TokenUsage) {
	sum.InputTokens = addInt(sum.InputTokens, add.InputTokens)
	sum.OutputTokens = addInt(sum.OutputTokens, add.OutputTokens)
	sum.TotalTokens = addInt(sum.TotalTokens, add.TotalTokens)
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
