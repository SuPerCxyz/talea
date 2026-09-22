package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

func newDB(t *testing.T) *index.DB {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(context.Background(), db); err != nil {
		t.Fatal(err)
	}
	return db
}

func insertSession(t *testing.T, db *index.DB, s *model.Session) {
	t.Helper()
	if err := db.UpsertSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func mkSession(id, question, cwd, agent string) *model.Session {
	now := time.Now()
	return &model.Session{
		AgentID:          model.AgentID(agent),
		AgentInstanceID:  agent + "-inst",
		SessionID:        id,
		FirstQuestion:    question,
		WorkingDirectory: cwd,
		StartedAt:        &now,
		EndedAt:          &now,
		LastActivityAt:   &now,
		Activity:         model.ActivityInactive,
		IndexedAt:        now,
		UpdatedAt:        now,
	}
}

func TestSearchByIDAndFTS(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("abc-123", "修复 multipath 残留问题", "/home/alice/code/cinder", "claude-code"))
	insertSession(t, db, mkSession("def-456", "分析疏散虚机后设备残留", "/home/alice/code/nov", "opencode"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 会话 ID 精确匹配
	res, err := Search(ctx, db, Query{Term: "abc-123", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "abc-123" {
		t.Fatalf("id search: %d results", len(res))
	}

	// 中文 3 字以上 trigram 搜索
	res, err = Search(ctx, db, Query{Term: "multipath", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected multipath match")
	}

	// 中文搜索
	res, err = Search(ctx, db, Query{Term: "疏散虚机", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) == 0 {
		t.Fatal("expected 疏散虚机 match")
	}
}

func TestPopulateUpdatesDirtyFTSRow(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	s := mkSession("dirty-1", "旧问题", "/home/alice", "claude-code")
	insertSession(t, db, s)
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	s.FirstQuestion = "新问题 multipath"
	if err := db.UpsertSession(ctx, s); err != nil {
		t.Fatal(err)
	}
	need, err := NeedsPopulate(ctx, db)
	if err != nil || !need {
		t.Fatalf("dirty FTS state: need=%v err=%v", need, err)
	}
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	need, err = NeedsPopulate(ctx, db)
	if err != nil || need {
		t.Fatalf("clean FTS state: need=%v err=%v", need, err)
	}
	results, err := Search(ctx, db, Query{Term: "multipath", Limit: 10})
	if err != nil || len(results) != 1 || results[0].Session.SessionID != s.SessionID {
		t.Fatalf("updated search results=%v err=%v", results, err)
	}
}

func TestSearchLoadsResumeLaunchArgs(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	s := mkSession("resume-args", "resume args", "/home/alice", "codex-cli")
	s.ResumeLaunchArgs = []string{"--sandbox", "workspace-write"}
	insertSession(t, db, s)
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}
	results, err := Search(ctx, db, Query{Term: "resume args", Limit: 1})
	if err != nil || len(results) != 1 {
		t.Fatalf("results=%v err=%v", results, err)
	}
	if !sameArgs(results[0].Session.ResumeLaunchArgs, s.ResumeLaunchArgs) {
		t.Fatalf("resume args=%v want=%v", results[0].Session.ResumeLaunchArgs, s.ResumeLaunchArgs)
	}
	if _, err := db.SQL().ExecContext(ctx,
		`UPDATE sessions SET resume_launch_args_json='not-json' WHERE session_id=?`, s.SessionID); err != nil {
		t.Fatal(err)
	}
	results, err = Search(ctx, db, Query{Term: "resume args", Limit: 1})
	if err != nil || len(results) != 1 || len(results[0].Session.ResumeLaunchArgs) != 0 {
		t.Fatalf("invalid stored args should be ignored: results=%v err=%v", results, err)
	}
}

func sameArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// TestByIDPrefix 验证按 session_id 前缀查找（不经 FTS，短前缀可用），
// 以及 agent 过滤与不存在的场景。
func TestByIDPrefix(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15W", "q1", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15X", "q2", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("abc-123", "q3", "/home/alice/code", "claude-code"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 前缀唯一
	res, err := ByIDPrefix(ctx, db, "ses_0a037a156ffeHPmbe15W", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "ses_0a037a156ffeHPmbe15W" {
		t.Fatalf("unique prefix: got %d results", len(res))
	}

	// 前缀多候选 + agent 过滤
	res, err = ByIDPrefix(ctx, db, "ses_0a037a156ffeHPmbe15", "opencode", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("prefix+agent: got %d results", len(res))
	}

	// 前缀不存在
	res, err = ByIDPrefix(ctx, db, "nope", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("missing prefix: got %d results", len(res))
	}
}

// TestSearchTermAndAgent 回归：Term（FTS）与 Agent 过滤组合时参数占位符不得错位，
// 否则 agent 值会传给 EXISTS 的 MATCH、ftsTerm 传给 agent_id 比较导致 0 结果。
func TestSearchTermAndAgent(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("ses_0a037a156ffeHPmbe15W", "multipath 残留", "/home/alice", "opencode"))
	insertSession(t, db, mkSession("abc-123", "multipath 清理", "/home/alice/code/cinder", "claude-code"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := Search(ctx, db, Query{Term: "ses_0a037a156ffeHPmbe15W", Agent: "opencode", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "ses_0a037a156ffeHPmbe15W" {
		t.Fatalf("term+agent: got %d results", len(res))
	}

	res, err = Search(ctx, db, Query{Term: "multipath", Agent: "claude-code", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.AgentID != "claude-code" {
		t.Fatalf("keyword+agent: got %d results", len(res))
	}
}

func TestSearchFilters(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("s1", "问题一", "/home/alice/code/a", "claude-code"))
	insertSession(t, db, mkSession("s2", "问题二", "/home/alice/code/b", "opencode"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := Search(ctx, db, Query{Agent: "opencode", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.AgentID != "opencode" {
		t.Fatalf("agent filter: %d results", len(res))
	}

	res, err = Search(ctx, db, Query{Cwd: "/home/alice/code/b", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 1 || res[0].Session.SessionID != "s2" {
		t.Fatalf("cwd filter: %d results", len(res))
	}
}

// TestSearchDirPrefixNormalization 回归：目录过滤需规范化相对路径（./、../）与末尾斜杠，
// 且为精确匹配：只命中工作目录 == 指定目录的会话，不包含子目录。
func TestSearchDirPrefixNormalization(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("s1", "问题一", "/home/alice/code/a", "claude-code"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// 将测试会话的工作目录改为相对于当前进程 cwd 的某路径，便于用相对写法过滤
	absTarget := filepath.Join(cwd, "relproj")
	insertSession(t, db, mkSession("s2", "问题二", absTarget, "opencode"))
	// 子目录中的会话不应被父目录过滤命中
	insertSession(t, db, mkSession("s3", "问题三", filepath.Join(absTarget, "sub"), "opencode"))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		dir  string
		want int
	}{
		{"exact absolute", absTarget, 1},
		{"trailing slash", absTarget + "/", 1},
		{"relative with dot", filepath.Join(".", "relproj"), 1},
		{"parent then child", filepath.Join("..", filepath.Base(cwd), "relproj"), 1},
		{"subdir not matched", filepath.Join(absTarget, "sub"), 1},
		{"parent does not match subdir", absTarget, 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := Search(ctx, db, Query{Cwd: c.dir, Limit: 10})
			if err != nil {
				t.Fatal(err)
			}
			if len(res) != c.want {
				t.Fatalf("dir %q: got %d results, want %d", c.dir, len(res), c.want)
			}
		})
	}
}

func TestListEmpty(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	res, err := List(ctx, db, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 0 {
		t.Fatalf("expected empty, got %d", len(res))
	}
}

// TestSearchOrderByEndTime 回归：所有搜索结果必须按结束时间倒序
// （兜底顺序 ended_at > started_at > last_activity_at），
// 不得依赖 last_activity_at，且关键词搜索同样适用。
func TestSearchOrderByEndTime(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	base := time.Date(2026, 8, 26, 12, 0, 0, 0, time.Local)

	mk := func(id string, started, ended, last *time.Time) *model.Session {
		s := mkSession(id, "multipath 排序验证", "/home/alice/code/sort", "claude-code")
		s.StartedAt = started
		s.EndedAt = ended
		s.LastActivityAt = last
		return s
	}
	at := func(h int) *time.Time {
		ts := base.Add(time.Duration(h) * time.Hour)
		return &ts
	}
	// last_activity 与 ended 顺序刻意相反，证明排序键不是 last_activity_at
	insertSession(t, db, mk("s_old", at(0), at(1), at(9)))
	insertSession(t, db, mk("s_nofallback", nil, nil, at(2))) // 仅 last_activity 兜底
	insertSession(t, db, mk("s_new", at(0), at(5), at(5)))
	insertSession(t, db, mk("s_startfallback", at(8), nil, at(0))) // 无结束时间回退开始时间
	insertSession(t, db, mk("s_mid", at(0), at(3), at(7)))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	want := []string{"s_startfallback", "s_new", "s_mid", "s_nofallback", "s_old"}
	for _, tc := range []struct {
		name string
		q    Query
	}{
		{"no term", Query{Limit: 10}},
		{"with term (FTS)", Query{Term: "multipath", Limit: 10}},
		{"with term (FTS zh)", Query{Term: "排序验证", Limit: 10}},
		{"with term (LIKE)", Query{Term: "排序", Limit: 10}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			res, err := Search(ctx, db, tc.q)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, r := range res {
				got = append(got, r.Session.SessionID)
			}
			if len(got) != len(want) {
				t.Fatalf("got %d results, want %d: %v", len(got), len(want), got)
			}
			for i := range want {
				if got[i] != want[i] {
					t.Fatalf("order mismatch:\n got  %v\n want %v", got, want)
				}
			}
		})
	}
}

// TestByIDPrefixOrderByEndTime 验证 ID 前缀多候选时也按结束时间倒序。
func TestByIDPrefixOrderByEndTime(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	base := time.Now()

	mkAt := func(id string, offset time.Duration) *model.Session {
		s := mkSession(id, "q", "/home/alice", "opencode")
		end := base.Add(offset)
		s.EndedAt = &end
		s.LastActivityAt = &base // last_activity 固定，排序只应由 ended 决定
		return s
	}
	insertSession(t, db, mkAt("pfx_early", 1*time.Hour))
	insertSession(t, db, mkAt("pfx_late", 6*time.Hour))
	if err := Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	res, err := ByIDPrefix(ctx, db, "pfx_", "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(res) != 2 {
		t.Fatalf("got %d results, want 2", len(res))
	}
	if res[0].Session.SessionID != "pfx_late" || res[1].Session.SessionID != "pfx_early" {
		t.Fatalf("order: [%s, %s], want [pfx_late, pfx_early]",
			res[0].Session.SessionID, res[1].Session.SessionID)
	}
}

// TestSearchExcludeSubagents 验证 SQL 层子会话过滤：默认查询返回全部会话
// （现有调用方如 talea list 在结果层自行按 IsSubagent 过滤，行为不变），
// ExcludeSubagents=true 时仅返回主会话。
func TestSearchExcludeSubagents(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	insertSession(t, db, mkSession("main-1", "主会话问题", "/home/alice/x", "opencode"))
	sub := mkSession("sub-1", "You are a subagent spawned by another session", "/home/alice/x", "opencode")
	sub.IsSubagent = true
	insertSession(t, db, sub)

	all, err := List(ctx, db, Query{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("默认查询行为必须不变（含子会话，由调用方过滤）, got %d 条", len(all))
	}

	mainOnly, err := List(ctx, db, Query{Limit: 10, ExcludeSubagents: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(mainOnly) != 1 || mainOnly[0].Session.SessionID != "main-1" || mainOnly[0].Session.IsSubagent {
		t.Fatalf("ExcludeSubagents 应仅返回主会话, got %+v", mainOnly)
	}
}

// TestSearchUnlimited 验证 Unlimited 覆盖 Limit 截断，默认 LIMIT 语义保持不变。
func TestSearchUnlimited(t *testing.T) {
	ctx := context.Background()
	db := newDB(t)
	for i := 0; i < 5; i++ {
		insertSession(t, db, mkSession(fmt.Sprintf("u-%d", i), "unlimited test", "/home/alice/x", "opencode"))
	}

	limited, err := List(ctx, db, Query{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(limited) != 2 {
		t.Fatalf("默认 Limit=2 仍应截断, got %d 条", len(limited))
	}

	all, err := List(ctx, db, Query{Limit: 2, Unlimited: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 5 {
		t.Fatalf("Unlimited 应返回全部会话, got %d 条", len(all))
	}
}

// TestLimitClause 锁定 Limit 语义：0/负数取默认上限而非“不限制”，
// Unlimited 时不追加 LIMIT 子句。
func TestLimitClause(t *testing.T) {
	for _, tc := range []struct {
		name string
		q    Query
		want int
	}{
		{name: "zero uses default", q: Query{}, want: defaultSearchLimit},
		{name: "negative uses default", q: Query{Limit: -1}, want: defaultSearchLimit},
		{name: "explicit limit", q: Query{Limit: 7}, want: 7},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sql, args := tc.q.limitClause()
			if sql != " LIMIT ?" || len(args) != 1 || args[0] != tc.want {
				t.Fatalf("limitClause(%+v) = %q %v, want LIMIT with %d", tc.q, sql, args, tc.want)
			}
		})
	}
	sql, args := Query{Unlimited: true}.limitClause()
	if sql != "" || len(args) != 0 {
		t.Fatalf("Unlimited 应不追加 LIMIT, got %q %v", sql, args)
	}
}
