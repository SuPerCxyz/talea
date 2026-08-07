package cli

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

func int64p(n int64) *int64 { return &n }

// mkGoSession 构造测试会话。
func mkGoSession(id, cwd string, end time.Time) *model.Session {
	start := end.Add(-time.Hour)
	return &model.Session{
		AgentID:          model.AgentOpenCode,
		SessionID:        id,
		FirstQuestion:    "test session " + id,
		WorkingDirectory: cwd,
		StartedAt:        &start,
		EndedAt:          &end,
		LastActivityAt:   &end,
		Activity:         model.ActivityInactive,
		IndexedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}
}

func TestQueryGoSessionsDirFilter(t *testing.T) {
	ctx := context.Background()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := search.Ensure(ctx, db); err != nil {
		t.Fatal(err)
	}

	base := time.Date(2026, 8, 5, 16, 0, 0, 0, time.Local)
	// 目标目录下的会话
	insertGoSession(t, db, mkGoSession("ses_a", "/home/user/nexora", base.Add(time.Minute)))
	insertGoSession(t, db, mkGoSession("ses_b", "/home/user/nexora/frontend", base.Add(2*time.Minute)))
	// 其他目录的会话
	insertGoSession(t, db, mkGoSession("ses_c", "/home/user/other", base.Add(3*time.Minute)))
	if err := search.Populate(ctx, db); err != nil {
		t.Fatal(err)
	}

	// 不指定目录：返回全部 3 个
	all, err := queryGoSessions(ctx, db, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 3 {
		t.Fatalf("no dir filter: got %d sessions, want 3", len(all))
	}

	// 指定目录：只返回该目录前缀下的会话
	filtered, err := queryGoSessions(ctx, db, "/home/user/nexora")
	if err != nil {
		t.Fatal(err)
	}
	if len(filtered) != 2 {
		t.Fatalf("dir filter: got %d sessions, want 2", len(filtered))
	}
	for _, s := range filtered {
		if len(s.WorkingDirectory) < len("/home/user/nexora") ||
			s.WorkingDirectory[:len("/home/user/nexora")] != "/home/user/nexora" {
			t.Errorf("session %s working dir %q outside filter", s.SessionID, s.WorkingDirectory)
		}
	}

	// 子目录过滤
	sub, err := queryGoSessions(ctx, db, "/home/user/nexora/frontend")
	if err != nil {
		t.Fatal(err)
	}
	if len(sub) != 1 || sub[0].SessionID != "ses_b" {
		t.Fatalf("sub dir filter: got %d sessions, want ses_b only", len(sub))
	}
}

func insertGoSession(t *testing.T, db *index.DB, s *model.Session) {
	t.Helper()
	if err := db.UpsertSession(context.Background(), s); err != nil {
		t.Fatal(err)
	}
}

func TestGoRow(t *testing.T) {
	i18n.Set(i18n.LangZh)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })
	start := time.Date(2026, 8, 5, 16, 41, 0, 0, time.Local)
	end := time.Date(2026, 8, 5, 17, 0, 0, 0, time.Local)
	dur := 19 * time.Minute
	cases := []struct {
		name string
		sess *model.Session
		want []string
	}{
		{
			name: "full fields",
			sess: &model.Session{
				AgentID:          model.AgentClaudeCode,
				SessionID:        "abc123",
				StartedAt:        &start,
				EndedAt:          &end,
				Duration:         &dur,
				FirstQuestion:    "修复登录 bug",
				WorkingDirectory: "/home/user/proj",
				HasTokenUsage:    true,
				TokenUsage:       &model.TokenUsage{TotalTokens: int64p(1400000)},
			},
			want: []string{"claudecode", "abc123", "08-05 16:41", "08-05 17:00", "19m 0s", "1.40M", "~/proj", "修复登录 bug"},
		},
		{
			name: "empty optional",
			sess: &model.Session{
				AgentID:   model.AgentOpenCode,
				SessionID: "ses_xyz",
			},
			want: []string{"opencode", "ses_xyz", "", "", "未知", "未知", "", ""},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := goRow(c.sess)
			if len(got) != len(c.want) {
				t.Fatalf("行列数 %d != 期望 %d", len(got), len(c.want))
			}
			for i := range c.want {
				if got[i] != c.want[i] {
					t.Errorf("列 %d: got %q, want %q", i, got[i], c.want[i])
				}
			}
		})
	}
}
