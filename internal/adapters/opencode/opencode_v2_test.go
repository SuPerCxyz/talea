package opencode

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// 形态探测、发现查询与元数据解析的 v2 兼容性测试。

// TestStorageShapeReportsFlavor 覆盖 StorageShape 两种形态与不可读错误（任务 1.1）。
func TestStorageShapeReportsFlavor(t *testing.T) {
	ctx := context.Background()
	inst := model.AgentInstance{
		InstanceID:    "t",
		AgentID:       model.AgentOpenCode,
		DataDirectory: t.TempDir(),
	}

	t.Setenv("OPENCODE_DB", createV2FixtureDB(t))
	if got, err := New().StorageShape(ctx, inst); err != nil || got != shapeSessionV2 {
		t.Fatalf("v2 形态: got=%q err=%v", got, err)
	}

	t.Setenv("OPENCODE_DB", createFixtureDB(t))
	if got, err := New().StorageShape(ctx, inst); err != nil || got != shapeSession {
		t.Fatalf("传统形态: got=%q err=%v", got, err)
	}

	t.Setenv("OPENCODE_DB", writeGarbageDB(t))
	if got, err := New().StorageShape(ctx, inst); err == nil || got != "" {
		t.Fatalf("不可读数据库应返回错误: got=%q err=%v", got, err)
	}
}

// TestProbeTablesUnreadableFallsBackToLegacy 覆盖探测失败回退（任务 1.1 / 1.4）。
func TestProbeTablesUnreadableFallsBackToLegacy(t *testing.T) {
	ctx := context.Background()
	bad := writeGarbageDB(t)

	if flags := probeTablesAt(ctx, bad); flags.session || flags.sessionV2 {
		t.Fatalf("不可读探测应回退零值: %+v", flags)
	}
	if _, err := storageShapeAt(ctx, bad); err == nil {
		t.Fatal("不可读数据库的形态探测应返回错误")
	}
	// 回退后按传统 session 构造查询，不因探测失败直接退出
	query, args := discoverySQL(probeTablesAt(ctx, bad), nil)
	if query != discoverySelect+tableSession || len(args) != 0 {
		t.Fatalf("回退查询=%q args=%v", query, args)
	}

	t.Setenv("OPENCODE_DB", bad)
	sources, err := New().Discover(ctx, model.AgentInstance{
		InstanceID:    "t",
		AgentID:       model.AgentOpenCode,
		DataDirectory: t.TempDir(),
	})
	if err == nil || len(sources) != 0 {
		t.Fatalf("不可读数据库应报错且不返回数据: sources=%v err=%v", sources, err)
	}
}

// TestDiscoverySQLLegacyUnchanged 保证无 session_v2 时查询与既有实现一致（任务 1.2 / 1.3）。
func TestDiscoverySQLLegacyUnchanged(t *testing.T) {
	query, args := discoverySQL(tableFlags{}, nil)
	if query != discoverySelect+tableSession || args != nil {
		t.Fatalf("全量查询=%q args=%v", query, args)
	}
	query, args = discoverySQL(tableFlags{session: true}, nil)
	if query != discoverySelect+tableSession || args != nil {
		t.Fatalf("传统表全量查询=%q args=%v", query, args)
	}
	cutoff := int64(123456)
	query, args = discoverySQL(tableFlags{session: true}, &cutoff)
	want := discoverySelect + tableSession + discoveryCutoffWhere
	if query != want || len(args) != 1 || args[0] != cutoff {
		t.Fatalf("传统表增量查询=%q args=%v", query, args)
	}
}

// TestDiscoverySQLUsesUnionWithOuterCutoff 断言并集形状与外层高水位过滤（任务 1.2 / 1.3）。
func TestDiscoverySQLUsesUnionWithOuterCutoff(t *testing.T) {
	cutoff := int64(999)
	query, args := discoverySQL(tableFlags{session: true, sessionV2: true}, &cutoff)
	if !containsAll(query, "UNION", "NOT EXISTS", "session_v2", discoveryCutoffWhere) {
		t.Fatalf("并集查询缺少预期片段: %q", query)
	}
	// 高水位过滤必须在并集子查询之外
	if strings.Index(query, "UNION") > strings.Index(query, discoveryCutoffWhere) {
		t.Fatalf("cutoff 过滤未施加在外层: %q", query)
	}
	if len(args) != 1 || args[0] != cutoff {
		t.Fatalf("args=%v", args)
	}
}

// TestDiscoverV2OnlyTable 覆盖只有 session_v2 的库（任务 1.4）。
func TestDiscoverV2OnlyTable(t *testing.T) {
	got := discoverIDs(t, createV2FixtureDB(t))
	want := map[string]bool{
		"ses_new": true, "ses_v2null": true, "ses_v2empty": true, "ses_v2inject": true,
	}
	assertSameIDs(t, got, want)
	if got["ses_new"] != 1789800100000 {
		t.Fatalf("ses_new mtime=%d", got["ses_new"])
	}
}

// TestDiscoverLegacyOnlyTable 覆盖只有传统 session 的库（任务 1.4）。
func TestDiscoverLegacyOnlyTable(t *testing.T) {
	got := discoverIDs(t, createFixtureDB(t))
	assertSameIDs(t, got, map[string]bool{"ses_0001": true, "ses_nullable": true})
}

// TestDiscoverUnionNoDuplicateNoLoss 覆盖两表并存：不重复、不丢失（任务 1.2 / 1.4）。
func TestDiscoverUnionNoDuplicateNoLoss(t *testing.T) {
	path := createFixtureDB(t)
	addSessionV2Tables(t, path)
	got := discoverIDs(t, path)
	assertSameIDs(t, got, map[string]bool{
		"ses_0001": true, "ses_nullable": true, "ses_v2only": true, "ses_legacy_new": true,
	})
	// 同 id 两表都有时取 session_v2 行的时间戳
	if got["ses_0001"] != 1785999999999 {
		t.Fatalf("ses_0001 mtime=%d，应取 session_v2 行", got["ses_0001"])
	}
}

// TestDiscoverIncrementalV2HighWatermark 覆盖并集增量查询的游标与重叠窗口（任务 1.3）。
func TestDiscoverIncrementalV2HighWatermark(t *testing.T) {
	ctx := context.Background()
	path := createFixtureDB(t)
	addSessionV2Tables(t, path)
	t.Setenv("OPENCODE_DB", path)

	inst := model.AgentInstance{
		InstanceID:    "t",
		AgentID:       model.AgentOpenCode,
		DataDirectory: filepath.Dir(path),
	}
	a := New()
	sources, state, err := a.DiscoverIncremental(ctx, inst, adapters.DiscoveryState{})
	if err != nil || len(sources) != 4 || state.Cursor == "" {
		t.Fatalf("首次发现: sources=%d cursor=%q err=%v", len(sources), state.Cursor, err)
	}

	candidates, next, err := a.DiscoverIncremental(ctx, inst, state)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]int, len(candidates))
	for _, src := range candidates {
		got[src.SessionID]++
	}
	// cutoff = 1786000000001 - 5min = 1785999700001
	want := map[string]bool{"ses_0001": true, "ses_v2only": true, "ses_legacy_new": true}
	for id := range want {
		if got[id] != 1 {
			t.Fatalf("候选 %s 出现 %d 次: %v", id, got[id], got)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("窗口外会话不应出现: %v", got)
	}
	cur, err := decodeCursor(next.Cursor)
	if err != nil || cur.TimeUpdated != 1786000000001 {
		t.Fatalf("next cursor=%+v err=%v", cur, err)
	}
}

// TestParseMetadataPrefersSessionV2 覆盖 v2 行优先与可空 title（任务 2.1 / 2.2 / 2.3）。
func TestParseMetadataPrefersSessionV2(t *testing.T) {
	path := createFixtureDB(t)
	addSessionV2Tables(t, path)
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode},
		adapters.SessionSource{SessionID: "ses_0001", Path: path})
	if err != nil {
		t.Fatal(err) // v2 行 title 为 NULL，不得 Scan 报错
	}
	if s.WorkingDirectory != "/home/alice/v2dir" {
		t.Fatalf("应取 session_v2 行的目录: %q", s.WorkingDirectory)
	}
	if !s.HasTokenUsage || s.TokenUsage == nil || s.TokenUsage.InputTokens == nil ||
		*s.TokenUsage.InputTokens != 700 {
		t.Fatalf("应取 session_v2 行的 token: %+v", s.TokenUsage)
	}
}

// TestParseMetadataFallsBackToLegacyRow 覆盖 v2 表存在但行缺失时回退（任务 2.1 / 2.3）。
func TestParseMetadataFallsBackToLegacyRow(t *testing.T) {
	path := createFixtureDB(t)
	addSessionV2Tables(t, path)
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode},
		adapters.SessionSource{SessionID: "ses_nullable", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkingDirectory != "/home/alice/code/nexora" {
		t.Fatalf("应回退传统表目录: %q", s.WorkingDirectory)
	}
	if s.HasTokenUsage || s.TokenUsage != nil {
		t.Fatalf("全零 token 应保持未知: %+v", s.TokenUsage)
	}
}

// TestParseMetadataV2OnlySession 覆盖 v2 形态库的元数据与已知 token（任务 2.1 / 2.2）。
func TestParseMetadataV2OnlySession(t *testing.T) {
	path := createV2FixtureDB(t)
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode},
		adapters.SessionSource{SessionID: "ses_new", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkingDirectory != "/home/alice/code/nexora" || s.FormatVersion != "build" {
		t.Fatalf("cwd=%q format=%q", s.WorkingDirectory, s.FormatVersion)
	}
	if !s.HasTokenUsage || s.TokenUsage.InputTokens == nil || *s.TokenUsage.InputTokens != 5000 {
		t.Fatalf("token 汇总=%+v", s.TokenUsage)
	}
}

// TestParseMetadataV2NullableTitleUnknownTokens 覆盖可空 title 与默认零值 token（任务 2.2 / 2.3）。
func TestParseMetadataV2NullableTitleUnknownTokens(t *testing.T) {
	path := createV2FixtureDB(t)
	s, err := New().ParseMetadata(context.Background(),
		model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode},
		adapters.SessionSource{SessionID: "ses_v2null", Path: path})
	if err != nil {
		t.Fatalf("title 为 NULL 不应报错: %v", err)
	}
	if s.HasTokenUsage || s.TokenUsage != nil {
		t.Fatalf("默认零值 token 应保持未知: %+v", s.TokenUsage)
	}
	if s.FormatVersion != "" {
		t.Fatalf("agent 为空时 format=%q", s.FormatVersion)
	}
}

// assertSameIDs 断言发现结果的会话 ID 集合完全一致。
func assertSameIDs(t *testing.T, got map[string]int64, want map[string]bool) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("发现数量 got=%d want=%d: %v", len(got), len(want), got)
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Fatalf("丢失会话 %s: %v", id, got)
		}
	}
}

// containsAll 判断字符串是否包含全部子串。
func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
