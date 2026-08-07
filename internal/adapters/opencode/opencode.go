// Package opencode 实现 OpenCode 适配器（SQLite 只读）。
package opencode

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/extract"
	"github.com/talea/talea/internal/model"
)

const (
	displayName = "OpenCode"
	formatName  = "opencode-sqlite"
	dbFileName  = "opencode.db"
)

// Adapter 实现 OpenCode 会话读取。
type Adapter struct{}

// New 创建适配器。
func New() *Adapter { return &Adapter{} }

// ID 返回适配器标识（常量，不触发外部探测）。
func (a *Adapter) ID() model.AgentID { return model.AgentOpenCode }

// Info 返回适配器信息与能力。
func (a *Adapter) Info() model.AdapterInfo {
	return model.AdapterInfo{
		ID:          model.AgentOpenCode,
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

// versionOf 缓存版本探测结果，避免每次 Detect 重复启动外部进程。
var versionOnce = sync.OnceValue(func() string {
	p, err := exec.LookPath("opencode")
	if err != nil {
		return ""
	}
	cmd := exec.Command(p, "--version")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
})

func versionOf() string { return versionOnce() }

// Detect 探测本机安装。
func (a *Adapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return nil, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataDir := filepath.Join(home, ".local", "share", "opencode")
	if v := os.Getenv("XDG_DATA_HOME"); v != "" {
		dataDir = filepath.Join(v, "opencode")
	}
	inst := model.AgentInstance{
		InstanceID:     "opencode-default",
		AgentID:        model.AgentOpenCode,
		DisplayName:    displayName,
		Vendor:         "opencode.ai",
		ExecutablePath: bin,
		Version:        versionOf(),
		DataDirectory:  dataDir,
		ConfigPath:     filepath.Join(home, ".config", "opencode", "opencode.json"),
	}
	return []model.AgentInstance{inst}, nil
}

// openRO 只读打开数据库。
func openRO(path string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)&_pragma=query_only(1)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

// Discover 发现数据库中的会话。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	dbPath := filepath.Join(inst.DataDirectory, dbFileName)
	if _, err := os.Stat(dbPath); err != nil {
		return nil, nil
	}
	db, err := openRO(dbPath)
	if err != nil {
		return nil, fmt.Errorf("只读打开 OpenCode 数据库失败（保留旧索引）: %w", err)
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx,
		`SELECT id, time_updated, coalesce(length(title),0) FROM session`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []adapters.SessionSource
	for rows.Next() {
		var (
			id    string
			mtime int64
			size  int64
		)
		if err := rows.Scan(&id, &mtime, &size); err != nil {
			continue
		}
		out = append(out, adapters.SessionSource{
			SessionID: id,
			Path:      dbPath,
			SourceID:  "opencode-session:" + id,
			Mtime:     mtime,
			Size:      size,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SessionID < out[j].SessionID })
	return out, nil
}

// sessionRow 对应 session 表。
type sessionRow struct {
	ID               string
	Directory        string
	Path             string
	Title            string
	ParentID         sql.NullString
	Model            string
	Agent            string
	TokensInput      sql.NullInt64
	TokensOutput     sql.NullInt64
	TokensReasoning  sql.NullInt64
	TokensCacheRead  sql.NullInt64
	TokensCacheWrite sql.NullInt64
	TimeCreated      int64
	TimeUpdated      int64
}

// ParseMetadata 解析单个会话元数据。
func (a *Adapter) ParseMetadata(
	ctx context.Context,
	inst model.AgentInstance,
	src adapters.SessionSource,
) (*model.Session, error) {
	db, err := openRO(src.Path)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	row := db.QueryRowContext(ctx,
		`SELECT id, directory, path, title, parent_id, model, agent,
		        tokens_input, tokens_output, tokens_reasoning,
		        tokens_cache_read, tokens_cache_write,
		        time_created, time_updated
		 FROM session WHERE id = ?`, src.SessionID)

	var s sessionRow
	if err := row.Scan(&s.ID, &s.Directory, &s.Path, &s.Title, &s.ParentID,
		&s.Model, &s.Agent, &s.TokensInput, &s.TokensOutput, &s.TokensReasoning,
		&s.TokensCacheRead, &s.TokensCacheWrite, &s.TimeCreated, &s.TimeUpdated); err != nil {
		return nil, err
	}

	out := &model.Session{
		AgentID:         inst.AgentID,
		AgentInstanceID: inst.InstanceID,
		SessionID:       s.ID,
		FormatName:      formatName,
		SourcePath:      src.Path,
		SourceID:        src.SourceID,
		SourceMtime:     src.Mtime,
		SourceSize:      src.Size,
		Activity:        model.ActivityInactive,
	}

	created := time.UnixMilli(s.TimeCreated)
	updated := time.UnixMilli(s.TimeUpdated)
	out.StartedAt = &created
	out.StartTimeSource = model.TimeSourceSessionMeta
	out.LastActivityAt = &updated
	out.EndedAt = &updated
	out.EndTimeSource = model.TimeSourceLastActivity
	d := updated.Sub(created)
	out.Duration = &d

	if s.Directory != "" {
		out.WorkingDirectory = s.Directory
		out.WorkingDirSource = string(model.TimeSourceSessionMeta)
	}
	out.ParentSessionID = s.ParentID.String
	out.IsSubagent = s.ParentID.Valid && s.ParentID.String != ""

	if s.Agent != "" {
		out.FormatVersion = s.Agent
	}
	if s.Title != "" && s.Title != "New session" {
		// title 为 Agent 自动生成标题，仅作为会话说明，不用于首次提问
		_ = s.Title
	}

	// Token 汇总（会话级，完整）
	if s.TokensInput.Valid || s.TokensOutput.Valid {
		out.HasTokenUsage = true
		u := &model.TokenUsage{Source: model.UsageSourceAgentDatabase, Completeness: model.UsageComplete}
		if s.TokensInput.Valid {
			u.InputTokens = &s.TokensInput.Int64
		}
		if s.TokensOutput.Valid {
			u.OutputTokens = &s.TokensOutput.Int64
		}
		if s.TokensReasoning.Valid {
			u.ReasoningTokens = &s.TokensReasoning.Int64
		}
		if s.TokensCacheRead.Valid {
			u.CacheReadTokens = &s.TokensCacheRead.Int64
		}
		if s.TokensCacheWrite.Valid {
			u.CacheWriteTokens = &s.TokensCacheWrite.Int64
		}
		if u.InputTokens != nil && u.OutputTokens != nil {
			total := *u.InputTokens + *u.OutputTokens
			u.TotalTokens = &total
		}
		out.TokenUsage = u
	}

	// 首次提问：读取最早 user 消息的 text part
	out.FirstQuestion, out.FirstQuestionSource, out.FirstQuestionConfidence = a.firstQuestion(ctx, db, s.ID)

	out.UpdatedAt = time.Now()
	return out, nil
}

// firstQuestion 读取最早 user 消息正文。
func (a *Adapter) firstQuestion(ctx context.Context, db *sql.DB, sessionID string) (string, string, float64) {
	row := db.QueryRowContext(ctx,
		`SELECT m.id FROM message m
		 WHERE m.session_id = ? AND json_extract(m.data, '$.role') = 'user'
		 ORDER BY m.time_created ASC LIMIT 1`, sessionID)
	var msgID string
	if err := row.Scan(&msgID); err != nil {
		return "", "none", 0
	}
	rows, err := db.QueryContext(ctx,
		`SELECT data FROM part WHERE message_id = ? ORDER BY time_created ASC`, msgID)
	if err != nil {
		return "", "none", 0
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p struct {
			Type string `json:"type"`
			Text string `json:"text"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		if p.Type == "text" && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	first, ok := extract.FirstNonInjected(texts)
	if !ok {
		return "", "user_message_no_text", 0
	}
	return first, "user_message", 1.0
}

// LoadMessages 读取消息预览。
func (a *Adapter) LoadMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	db, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created FROM message WHERE session_id = ? ORDER BY time_created ASC`, s.SessionID)
	if err != nil {
		db.Close()
		return nil, err
	}
	type msgInfo struct {
		ID   string
		Time int64
	}
	var msgs []msgInfo
	for rows.Next() {
		var m msgInfo
		if err := rows.Scan(&m.ID, &m.Time); err != nil {
			continue
		}
		msgs = append(msgs, m)
	}
	rows.Close()
	db.Close()

	if opts.Limit > 0 && len(msgs) > opts.Limit {
		msgs = msgs[len(msgs)-opts.Limit:]
	}

	// 重新打开数据库按需加载正文
	db2, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	var out []adapters.Message
	for _, m := range msgs {
		var role string
		db2.QueryRowContext(ctx, `SELECT json_extract(data,'$.role') FROM message WHERE id=?`, m.ID).Scan(&role)
		pRows, err := db2.QueryContext(ctx, `SELECT data FROM part WHERE message_id=? ORDER BY time_created ASC`, m.ID)
		if err != nil {
			continue
		}
		var parts []string
		for pRows.Next() {
			var raw string
			pRows.Scan(&raw)
			var p struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}
			if json.Unmarshal([]byte(raw), &p) == nil && p.Type == "text" {
				parts = append(parts, p.Text)
			}
		}
		pRows.Close()
		out = append(out, adapters.Message{
			Role:      role,
			Timestamp: m.Time / 1000,
			Content:   strings.Join(parts, "\n"),
		})
	}
	db2.Close()
	return &sliceIterator{msgs: out}, nil
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
		Program: "opencode",
		Args:    []string{"-s", s.SessionID},
	}, nil
}

// LoadUsage 返回会话 Token 汇总。
func (a *Adapter) LoadUsage(ctx context.Context, s model.Session) (*model.TokenUsage, error) {
	if s.TokenUsage != nil {
		return s.TokenUsage, nil
	}
	return nil, nil
}

// IterateUsageEvents 从 message/part 表提取时间线事件。
// user 消息生成 user_message 事件；step-finish 生成 request 事件（携带 tokens）。
// source_identity 使用 message_id / part_id 保证幂等去重。
func (a *Adapter) IterateUsageEvents(
	ctx context.Context,
	s model.Session,
) (adapters.UsageEventIterator, error) {
	db, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	var events []*model.UsageTimelineEvent
	seq := int64(0)

	// 1) user 消息事件
	rows, err := db.QueryContext(ctx,
		`SELECT id, time_created, json_extract(data, '$.role')
		 FROM message WHERE session_id=? ORDER BY time_created ASC, id ASC`, s.SessionID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var (
			msgID   string
			created int64
			role    string
		)
		if err := rows.Scan(&msgID, &created, &role); err != nil {
			continue
		}
		if role != "user" {
			continue
		}
		ts := time.UnixMilli(created)
		preview := a.messagePreview(ctx, db, msgID)
		events = append(events, &model.UsageTimelineEvent{
			AgentInstanceID:   s.AgentInstanceID,
			SessionID:         s.SessionID,
			EventID:           "msg-" + msgID,
			EventType:         model.UsageEventUserMessage,
			Timestamp:         &ts,
			Sequence:          seq,
			MessageID:         msgID,
			Source:            model.UsageSourceMessageMetadata,
			Completeness:      model.UsageComplete,
			UserPromptPreview: preview,
			SourceIdentity:    "opencode-msg:" + msgID,
		})
		seq++
	}
	rows.Close()

	// 2) step-finish request 事件
	rows2, err := db.QueryContext(ctx,
		`SELECT p.id, p.message_id, p.time_created, p.data
		 FROM part p
		 WHERE p.session_id = ?
		 ORDER BY p.time_created ASC, p.id ASC`, s.SessionID)
	if err != nil {
		return nil, err
	}
	defer rows2.Close()
	for rows2.Next() {
		var (
			partID, msgID string
			created       int64
			raw           string
		)
		if err := rows2.Scan(&partID, &msgID, &created, &raw); err != nil {
			continue
		}
		var p struct {
			Type   string          `json:"type"`
			Tokens json.RawMessage `json:"tokens"`
			Tool   string          `json:"tool"`
			CallID string          `json:"callID"`
			State  json.RawMessage `json:"state"`
			Auto   *bool           `json:"auto"`
		}
		if err := json.Unmarshal([]byte(raw), &p); err != nil {
			continue
		}
		ts := time.UnixMilli(created)

		// 明确压缩事件：compaction part（spec §14.6）
		if p.Type == "compaction" {
			ev := &model.UsageTimelineEvent{
				AgentInstanceID: s.AgentInstanceID,
				SessionID:       s.SessionID,
				EventID:         partID,
				EventType:       model.UsageEventCompactionStart,
				Timestamp:       &ts,
				Sequence:        seq,
				MessageID:       msgID,
				Source:          model.UsageSourceMessageMetadata,
				Completeness:    model.UsageComplete,
				SourceIdentity:  "opencode-compact:" + partID,
				RawFields:       map[string]any{"auto": p.Auto != nil && *p.Auto},
			}
			events = append(events, ev)
			seq++
			continue
		}

		// 工具调用事件：tool part 生成 tool_start/tool_end
		if p.Type == "tool" {
			evType := model.UsageEventToolStart
			filePath := ""
			if len(p.State) > 0 {
				var st struct {
					Status string `json:"status"`
					Input  struct {
						FilePath string `json:"filePath"`
					} `json:"input"`
				}
				if json.Unmarshal(p.State, &st) == nil {
					if st.Status == "completed" {
						evType = model.UsageEventToolEnd
					}
					filePath = st.Input.FilePath
				}
			}
			ev := &model.UsageTimelineEvent{
				AgentInstanceID: s.AgentInstanceID,
				SessionID:       s.SessionID,
				EventID:         partID,
				EventType:       evType,
				Timestamp:       &ts,
				Sequence:        seq,
				MessageID:       msgID,
				ToolCallID:      p.CallID,
				ToolName:        p.Tool,
				FilePath:        filePath,
				Source:          model.UsageSourceMessageMetadata,
				Completeness:    model.UsageComplete,
				SourceIdentity:  "opencode-tool:" + partID,
			}
			events = append(events, ev)
			seq++
			continue
		}

		if p.Type != "step-finish" {
			continue
		}
		var tok struct {
			Total     *int64 `json:"total"`
			Input     *int64 `json:"input"`
			Output    *int64 `json:"output"`
			Reasoning *int64 `json:"reasoning"`
		}
		if err := json.Unmarshal(p.Tokens, &tok); err != nil {
			continue
		}
		ts = time.UnixMilli(created)
		ev := &model.UsageTimelineEvent{
			AgentInstanceID: s.AgentInstanceID,
			SessionID:       s.SessionID,
			EventID:         partID,
			EventType:       model.UsageEventRequest,
			Timestamp:       &ts,
			Sequence:        seq,
			MessageID:       msgID,
			Model:           a.messageModel(ctx, db, msgID),
			InputTokens:     tok.Input,
			OutputTokens:    tok.Output,
			TotalTokens:     tok.Total,
			ReasoningTokens: tok.Reasoning,
			Source:          model.UsageSourceMessageMetadata,
			Completeness:    model.UsageComplete,
			SourceIdentity:  "opencode-part:" + partID,
		}
		// OpenCode step-finish total 为上下文快照（累计），
		// input 为本次请求增量。将 total 映射为 ContextAfter 与 CumulativeTotal。
		if tok.Total != nil {
			ev.ContextAfter = tok.Total
			ev.CumulativeTotal = tok.Total
		}
		events = append(events, ev)
		seq++
	}
	return &eventIterator{events: events}, nil
}

// DetectActivity 检测会话活动状态（进程 + 文件更新时间）。
func (a *Adapter) DetectActivity(ctx context.Context, s model.Session) (model.ActivityState, error) {
	return adapters.ProcessActivityDetector{Executable: "opencode"}.DetectActivity(ctx, s)
}

// messageModel 从消息 data 提取模型名。
func (a *Adapter) messageModel(ctx context.Context, db *sql.DB, msgID string) string {
	var raw string
	err := db.QueryRowContext(ctx,
		`SELECT json_extract(data, '$.modelID') FROM message WHERE id=?`, msgID).Scan(&raw)
	if err != nil || raw == "" {
		return ""
	}
	return raw
}

// messagePreview 提取消息文本摘要。
func (a *Adapter) messagePreview(ctx context.Context, db *sql.DB, msgID string) string {
	rows, err := db.QueryContext(ctx,
		`SELECT data FROM part WHERE message_id=? AND json_extract(data,'$.type')='text'
		 ORDER BY time_created ASC`, msgID)
	if err != nil {
		return ""
	}
	defer rows.Close()
	var texts []string
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var p struct {
			Text string `json:"text"`
		}
		if json.Unmarshal([]byte(raw), &p) == nil && p.Text != "" {
			texts = append(texts, p.Text)
		}
	}
	joined := strings.Join(texts, "\n")
	runes := []rune(joined)
	if len(runes) > 200 {
		return string(runes[:200])
	}
	return joined
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
