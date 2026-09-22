package opencode

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// incrementalWithCursor 以指定游标执行一次增量发现。
func incrementalWithCursor(
	t *testing.T,
	dbPath string,
	cursor string,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	t.Helper()
	t.Setenv("OPENCODE_DB", dbPath)
	return New().DiscoverIncremental(context.Background(), model.AgentInstance{
		InstanceID:    "t",
		AgentID:       model.AgentOpenCode,
		DataDirectory: filepath.Dir(dbPath),
	}, adapters.DiscoveryState{Cursor: cursor})
}

// sourceIDs 返回来源列表的会话 ID 集合。
func sourceIDs(sources []adapters.SessionSource) map[string]int {
	out := make(map[string]int, len(sources))
	for _, src := range sources {
		out[src.SessionID]++
	}
	return out
}

// cursorShape 解析返回游标的存储形态。
func cursorShape(t *testing.T, raw string) string {
	t.Helper()
	cur, err := decodeCursor(raw)
	if err != nil {
		t.Fatalf("解码返回游标失败: %v", err)
	}
	return cur.Shape
}

// TestDiscoverIncrementalLegacyCursorTriggersFullDiscovery 是本次 bug 的回归测试：
// 旧格式游标（无 shape）建立于只读 session 表时代，其水位来自上游已冻结的 session 表；
// 只在 session_v2 且 time_updated 早于 cutoff 的历史会话若沿用增量过滤会被永久跳过。
func TestDiscoverIncrementalLegacyCursorTriggersFullDiscovery(t *testing.T) {
	// 旧格式游标：水位 = 最大 time_updated，JSON 中没有 shape 字段
	legacyCursor := `{"time_updated":1789800700000,"session_id":"ses_v2inject"}`
	sources, state, err := incrementalWithCursor(t, createV2FixtureDB(t), legacyCursor)
	if err != nil {
		t.Fatal(err)
	}

	got := sourceIDs(sources)
	// 形态不匹配 → 完整发现：早于 cutoff(1789800400000) 的会话必须包含进来
	want := map[string]bool{
		"ses_new": true, "ses_v2null": true, "ses_v2empty": true, "ses_v2inject": true,
	}
	if len(got) != len(want) {
		t.Fatalf("应触发完整发现，发现数=%d ids=%v", len(got), got)
	}
	for id := range want {
		if got[id] != 1 {
			t.Fatalf("会话 %s 缺失或重复: %v", id, got)
		}
	}
	// 回归目标：只在 session_v2、time_updated 早于既有高水位的会话不得被漏掉
	if _, ok := got["ses_new"]; !ok {
		t.Fatal("session_v2 中早于旧高水位的会话被漏掉")
	}
	if got := cursorShape(t, state.Cursor); got != shapeSessionV2 {
		t.Fatalf("补偿后的游标 shape=%q want=%q", got, shapeSessionV2)
	}
}

// TestDiscoverIncrementalShapeV2ToLegacyTriggersFullDiscovery 覆盖形态从 v2 变回传统。
func TestDiscoverIncrementalShapeV2ToLegacyTriggersFullDiscovery(t *testing.T) {
	cursor := `{"time_updated":1785919370604,"session_id":"ses_0001","shape":"session_v2"}`
	sources, state, err := incrementalWithCursor(t, createFixtureDB(t), cursor)
	if err != nil {
		t.Fatal(err)
	}
	got := sourceIDs(sources)
	// 增量本会过滤掉 ses_nullable（远早于水位），形态补偿后应完整发现 2 条
	if len(got) != 2 || got["ses_0001"] != 1 || got["ses_nullable"] != 1 {
		t.Fatalf("应触发完整发现: %v", got)
	}
	if got := cursorShape(t, state.Cursor); got != shapeSession {
		t.Fatalf("补偿后的游标 shape=%q want=%q", got, shapeSession)
	}
}

// TestDiscoverIncrementalShapeLegacyToV2TriggersFullDiscovery 覆盖形态从传统迁到 v2。
func TestDiscoverIncrementalShapeLegacyToV2TriggersFullDiscovery(t *testing.T) {
	cursor := `{"time_updated":1789800700000,"session_id":"ses_v2inject","shape":"session"}`
	sources, state, err := incrementalWithCursor(t, createV2FixtureDB(t), cursor)
	if err != nil {
		t.Fatal(err)
	}
	if got := sourceIDs(sources); len(got) != 4 {
		t.Fatalf("应触发完整发现: %v", got)
	}
	if got := cursorShape(t, state.Cursor); got != shapeSessionV2 {
		t.Fatalf("补偿后的游标 shape=%q want=%q", got, shapeSessionV2)
	}
}

// TestDiscoverIncrementalEmptyCursorCarriesShape 覆盖首次发现（游标为空）返回带形态的游标。
func TestDiscoverIncrementalEmptyCursorCarriesShape(t *testing.T) {
	sources, state, err := incrementalWithCursor(t, createV2FixtureDB(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceIDs(sources)) != 4 {
		t.Fatalf("首次发现: %v", sourceIDs(sources))
	}
	if got := cursorShape(t, state.Cursor); got != shapeSessionV2 {
		t.Fatalf("空游标首次发现 shape=%q want=%q", got, shapeSessionV2)
	}

	sources, state, err = incrementalWithCursor(t, createFixtureDB(t), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sourceIDs(sources)) != 2 {
		t.Fatalf("传统库首次发现: %v", sourceIDs(sources))
	}
	if got := cursorShape(t, state.Cursor); got != shapeSession {
		t.Fatalf("传统库空游标 shape=%q want=%q", got, shapeSession)
	}
}

// TestDiscoverIncrementalSameShapeUsesHighWatermark 断言形态一致时仍走增量高水位，
// 不得退化成每次都完整发现；且返回游标继续携带形态。
func TestDiscoverIncrementalSameShapeUsesHighWatermark(t *testing.T) {
	dbPath := createV2FixtureDB(t)
	_, state, err := incrementalWithCursor(t, dbPath, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := cursorShape(t, state.Cursor); got != shapeSessionV2 {
		t.Fatalf("首次游标 shape=%q", got)
	}

	sources, next, err := incrementalWithCursor(t, dbPath, state.Cursor)
	if err != nil {
		t.Fatal(err)
	}
	got := sourceIDs(sources)
	// cutoff = 1789800700000 - 5min = 1789800400000：更早的 ses_new / ses_v2null 不在窗口内
	if len(got) != 2 || got["ses_v2empty"] != 1 || got["ses_v2inject"] != 1 {
		t.Fatalf("形态一致应保持增量窗口语义: %v", got)
	}
	if got := cursorShape(t, next.Cursor); got != shapeSessionV2 {
		t.Fatalf("增量游标 shape=%q", got)
	}
}

// TestDecodeCursorToleratesLegacyFormat 断言旧格式游标（无 shape）反序列化不报错。
func TestDecodeCursorToleratesLegacyFormat(t *testing.T) {
	cur, err := decodeCursor(`{"time_updated":1789700605279,"session_id":"ses_old"}`)
	if err != nil {
		t.Fatalf("旧格式游标不应报错: %v", err)
	}
	if cur.Shape != "" || cur.TimeUpdated != 1789700605279 || cur.SessionID != "ses_old" {
		t.Fatalf("旧格式游标解码=%+v", cur)
	}

	cur, err = decodeCursor(`{"time_updated":1,"session_id":"ses_1","shape":"session_v2"}`)
	if err != nil {
		t.Fatal(err)
	}
	if cur.Shape != shapeSessionV2 {
		t.Fatalf("带形态游标解码=%+v", cur)
	}
}

// TestDiscoverIncrementalEmptyDatabaseKeepsEmptyCursor 覆盖空库边界：
// 完整发现无来源时必须返回空游标（不得写出 session_id 为空的游标，否则下次解码报错、
// 每次同步都会记一次增量失败），且再次以空游标调用仍正常。
func TestDiscoverIncrementalEmptyDatabaseKeepsEmptyCursor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "opencode.db")
	db, err := sql.Open("sqlite", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(sessionV2DDL); err != nil {
		db.Close()
		t.Fatal(err)
	}
	db.Close()

	sources, state, err := incrementalWithCursor(t, path, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(sources) != 0 || state.Cursor != "" {
		t.Fatalf("空库: sources=%v cursor=%q", sources, state.Cursor)
	}
	sources, _, err = incrementalWithCursor(t, path, state.Cursor)
	if err != nil {
		t.Fatalf("空游标复用失败: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("空库复用: %v", sources)
	}
}
