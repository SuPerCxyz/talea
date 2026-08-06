package cli

import (
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func int64p(n int64) *int64 { return &n }

func TestGoRow(t *testing.T) {
	start := time.Date(2026, 8, 5, 16, 41, 0, 0, time.Local)
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
				FirstQuestion:    "修复登录 bug",
				WorkingDirectory: "/home/user/proj",
				TokenUsage:       &model.TokenUsage{TotalTokens: int64p(1400000)},
			},
			want: []string{"claude-code", "08-05 16:41", "1.40M", "~/proj", "修复登录 bug"},
		},
		{
			name: "empty optional",
			sess: &model.Session{
				AgentID:   model.AgentOpenCode,
				SessionID: "ses_xyz",
			},
			want: []string{"opencode", "", "未知", "", ""},
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
