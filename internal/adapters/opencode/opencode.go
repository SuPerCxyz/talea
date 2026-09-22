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
	"github.com/talea/talea/internal/model"
)

const (
	displayName               = "OpenCode"
	formatName                = "opencode-sqlite"
	dbFileName                = "opencode.db"
	cursorOverlapMillis int64 = 5 * 60 * 1000
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

// opencodeDatabasePath 解析 OpenCode 当前使用的数据库路径。
// v2 优先由 CLI 解析，旧版或探测失败时回退到固定路径。
func opencodeDatabasePath(ctx context.Context, inst model.AgentInstance) string {
	if override := strings.TrimSpace(os.Getenv("OPENCODE_DB")); override != "" {
		if filepath.IsAbs(override) {
			return filepath.Clean(override)
		}
		return filepath.Join(inst.DataDirectory, override)
	}
	if path := cliDatabasePath(ctx); path != "" {
		return path
	}
	return filepath.Join(inst.DataDirectory, dbFileName)
}

func cliDatabasePath(ctx context.Context) string {
	bin, err := exec.LookPath("opencode")
	if err != nil {
		return ""
	}
	out, err := exec.CommandContext(ctx, bin, "debug", "paths", "db").Output()
	if err != nil {
		return ""
	}
	path := strings.TrimSpace(string(out))
	if path == "" || !filepath.IsAbs(path) {
		return ""
	}
	return filepath.Clean(path)
}

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

// Discover 发现数据库中的会话；按表形态选择 session_v2 并集或传统 session。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	return a.discoverAt(ctx, opencodeDatabasePath(ctx, inst), nil)
}

// DiscoverIncremental 只查询高水位附近可能变化的会话。
// 游标携带建立时的存储形态；形态变化（含旧版无 shape 的游标）时旧高水位不再可信，
// 忽略游标并回退一次完整发现，避免永久跳过新表中的历史会话。
func (a *Adapter) DiscoverIncremental(
	ctx context.Context,
	inst model.AgentInstance,
	state adapters.DiscoveryState,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	dbPath := opencodeDatabasePath(ctx, inst)
	shape := probeTablesAt(ctx, dbPath).shape()
	if state.Cursor == "" {
		return a.fullDiscoverState(ctx, inst, shape)
	}
	cur, err := decodeCursor(state.Cursor)
	if err != nil {
		return nil, adapters.DiscoveryState{}, err
	}
	if cur.Shape != shape {
		// 一次性形态迁移补偿：数据源结构已迁移，旧高水位（或无 shape 的旧游标）
		// 会过滤掉新表中时间戳更早的历史会话，忽略它并做一次完整发现。
		return a.fullDiscoverState(ctx, inst, shape)
	}
	return a.incrementalDiscover(ctx, inst, dbPath, shape, cur)
}

// fullDiscoverState 执行完整发现，并返回携带存储形态的游标；
// 无来源时保持空游标（与既有空游标语义一致），避免写出无法解码的游标。
func (a *Adapter) fullDiscoverState(
	ctx context.Context,
	inst model.AgentInstance,
	shape string,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	sources, err := a.Discover(ctx, inst)
	if err != nil {
		return nil, adapters.DiscoveryState{}, err
	}
	max := cursorOfSources(sources, shape)
	cursor := ""
	if max.SessionID != "" {
		cursor = encodeCursor(max)
	}
	return sources, adapters.DiscoveryState{Cursor: cursor}, nil
}

// incrementalDiscover 在形态一致时执行既有高水位增量发现（含 5 分钟重叠窗口）。
func (a *Adapter) incrementalDiscover(
	ctx context.Context,
	inst model.AgentInstance,
	dbPath string,
	shape string,
	cur sourceCursor,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	cutoff := cur.TimeUpdated - cursorOverlapMillis
	// 高水位过滤施加在并集子查询外层，游标与 5 分钟重叠窗口语义保持不变。
	sources, err := a.discoverAt(ctx, dbPath, &cutoff)
	if err != nil {
		return nil, adapters.DiscoveryState{}, err
	}
	next := cursorOfSources(sources, shape)
	if cur.after(next) {
		next = cur
	}
	return sources, adapters.DiscoveryState{Cursor: encodeCursor(next)}, nil
}

// CursorFromSources 返回来源列表中的最大 (time_updated, session_id)。
// 刻意不携带 shape：该接口无 ctx、无法探测存储形态。这是保守降级——增量失败回退
// 路径产生无 shape 游标时，下次增量会判定形态不匹配而做一次完整发现；而
// DiscoverIncremental 写回的游标总是带 shape，因此不会死循环，也不会漏。
func (a *Adapter) CursorFromSources(_ model.AgentInstance, sources []adapters.SessionSource) string {
	max := cursorOfSources(sources, "")
	if max.SessionID == "" {
		return ""
	}
	return encodeCursor(max)
}

// cursorOfSources 返回来源列表中的最大游标，并写入指定存储形态（空串表示不携带形态）。
func cursorOfSources(sources []adapters.SessionSource, shape string) sourceCursor {
	var max sourceCursor
	for _, src := range sources {
		candidate := sourceCursor{TimeUpdated: src.Mtime, SessionID: src.SessionID}
		if candidate.after(max) {
			max = candidate
		}
	}
	max.Shape = shape
	return max
}

func encodeCursor(c sourceCursor) string {
	raw, _ := json.Marshal(c)
	return string(raw)
}

type sourceCursor struct {
	TimeUpdated int64  `json:"time_updated"`
	SessionID   string `json:"session_id"`
	// Shape 记录游标建立时的存储形态。形态变化说明数据源结构已迁移，
	// 旧高水位不再可信，必须回退一次完整发现，否则会永久跳过新表中的历史会话。
	Shape string `json:"shape,omitempty"`
}

func (c sourceCursor) after(other sourceCursor) bool {
	return c.TimeUpdated > other.TimeUpdated ||
		(c.TimeUpdated == other.TimeUpdated && c.SessionID > other.SessionID)
}

// decodeCursor 解析游标；旧格式游标没有 shape 字段（值为 ""），属合法输入，
// 不因此报错——形态不匹配的处理由 DiscoverIncremental 负责。
func decodeCursor(raw string) (sourceCursor, error) {
	var c sourceCursor
	if err := json.Unmarshal([]byte(raw), &c); err != nil {
		return sourceCursor{}, fmt.Errorf("OpenCode 游标无效: %w", err)
	}
	if c.TimeUpdated < 0 || c.SessionID == "" {
		return sourceCursor{}, fmt.Errorf("OpenCode 游标字段无效")
	}
	return c, nil
}

func (a *Adapter) discoverQuery(ctx context.Context, dbPath, query string, args ...any) ([]adapters.SessionSource, error) {
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	db, err := openRO(dbPath)
	if err != nil {
		return nil, fmt.Errorf("只读打开 OpenCode 数据库失败（保留旧索引）: %w", err)
	}
	defer db.Close()
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []adapters.SessionSource
	for rows.Next() {
		var id string
		var mtime, size int64
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
	return out, rows.Err()
}

// sessionRow 对应 session / session_v2 两表共有的会话元数据列。
type sessionRow struct {
	ID               string
	Directory        string
	Path             sql.NullString
	Title            sql.NullString
	ParentID         sql.NullString
	Model            sql.NullString
	Agent            sql.NullString
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

	// 元数据优先读 session_v2，表缺失或行缺失时回退传统 session。
	s, err := scanSessionRow(ctx, db, src.SessionID)
	if err != nil {
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

	if s.Agent.Valid && s.Agent.String != "" {
		out.FormatVersion = s.Agent.String
	}
	if s.Title.Valid && s.Title.String != "" && s.Title.String != "New session" {
		// title 为 Agent 自动生成标题，仅作为会话说明，不用于首次提问
		_ = s.Title.String
	}

	// Token 汇总（会话级，完整）
	if hasKnownTokenUsage(s) {
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

func hasKnownTokenUsage(s sessionRow) bool {
	return positiveToken(s.TokensInput) ||
		positiveToken(s.TokensOutput) ||
		positiveToken(s.TokensReasoning) ||
		positiveToken(s.TokensCacheRead) ||
		positiveToken(s.TokensCacheWrite)
}

func positiveToken(v sql.NullInt64) bool {
	return v.Valid && v.Int64 > 0
}

// firstQuestion 读取最早 user 消息正文：按会话数据分流到传统或 v2 路径。
func (a *Adapter) firstQuestion(ctx context.Context, db *sql.DB, sessionID string) (string, string, float64) {
	if useLegacyMessages(ctx, db, sessionID) {
		return a.legacyFirstQuestion(ctx, db, sessionID)
	}
	return a.v2FirstQuestion(ctx, db, sessionID)
}

// LoadMessages 读取消息预览：按会话数据分流到传统或 v2 路径。
func (a *Adapter) LoadMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	probe, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	legacy := useLegacyMessages(ctx, probe, s.SessionID)
	probe.Close()
	if legacy {
		return a.loadLegacyMessages(ctx, s, opts)
	}
	return a.v2LoadMessages(ctx, s, opts)
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

// IterateUsageEvents 提取时间线事件：按会话数据分流到传统 message/part 或 v2 session_message。
func (a *Adapter) IterateUsageEvents(
	ctx context.Context,
	s model.Session,
) (adapters.UsageEventIterator, error) {
	db, err := openRO(s.SourcePath)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	if useLegacyMessages(ctx, db, s.SessionID) {
		return a.iterateLegacyUsageEvents(ctx, db, s)
	}
	return a.iterateV2UsageEvents(ctx, db, s)
}

// DetectActivity 检测会话活动状态（进程 + 文件更新时间）。
func (a *Adapter) DetectActivity(ctx context.Context, s model.Session) (model.ActivityState, error) {
	return adapters.ProcessActivityDetector{Executable: "opencode"}.DetectActivity(ctx, s)
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
