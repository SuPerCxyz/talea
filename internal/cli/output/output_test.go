package output

import (
	"strings"
	"testing"
	"time"

	"github.com/talea/talea/internal/model"
)

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   *time.Duration
		want string
	}{
		{nil, "未知"},
		{sec(38), "38s"},
		{sec(720), "12m 0s"},
		{sec(5160), "1h 26m"},
		{sec(183600), "2d 3h"},
	}
	for _, c := range cases {
		if got := FormatDuration(c.in); got != c.want {
			t.Fatalf("got %q want %q", got, c.want)
		}
	}
}

func sec(n int64) *time.Duration {
	d := time.Duration(n) * time.Second
	return &d
}

func TestTokenString(t *testing.T) {
	in := int64(1500000)
	u := &model.TokenUsage{TotalTokens: &in}
	s := &model.Session{HasTokenUsage: true, TokenUsage: u}
	if got := tokenString(s); got != "1.50M" {
		t.Fatalf("got %q", got)
	}
	// 无 usage 显示未知而非 0
	s2 := &model.Session{}
	if got := tokenString(s2); got != "未知" {
		t.Fatalf("got %q", got)
	}
}

func TestViewOf(t *testing.T) {
	s := &model.Session{
		AgentID:          model.AgentClaudeCode,
		SessionID:        "abc",
		FirstQuestion:    "q",
		WorkingDirectory: "/home/alice/code/x",
		WorkingDirExists: true,
		Activity:         model.ActivityInactive,
	}
	v := ViewOf(s)
	if v.Tokens != "未知" {
		t.Fatalf("tokens: %q", v.Tokens)
	}
	if v.Activity != "已结束" {
		t.Fatalf("activity: %q", v.Activity)
	}
}

func TestTruncateHead(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
		want string
	}{
		{name: "short", in: "abc", w: 10, want: "abc"},
		{name: "exact", in: "1234567890", w: 10, want: "1234567890"},
		{name: "long keeps tail", in: "/home/alice/code/recode", w: 13, want: "…/code/recode"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := truncateHead(c.in, c.w)
			if got != c.want {
				t.Errorf("truncateHead(%q,%d) = %q, want %q", c.in, c.w, got, c.want)
			}
			if runeLen(got) > c.w {
				t.Errorf("结果宽度 %d 超过上限 %d: %q", runeLen(got), c.w, got)
			}
		})
	}
}

func TestWriteTableDynamicWidth(t *testing.T) {
	sessions := []*model.Session{
		{
			AgentID:          model.AgentClaudeCode,
			SessionID:        "abcdef",
			FirstQuestion:    strings.Repeat("这是一个很长的问题用于验证表格输出中首次提问列单行截断显示 ", 5),
			StartedAt:        timePtr(2026, 8, 5, 16, 41),
			WorkingDirectory: "/home/alice/code/very-long-project-name",
		},
	}
	var buf strings.Builder
	if err := Write(&buf, sessions, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// claude 应完整显示（不截断）
	if !strings.Contains(out, "claude") {
		t.Errorf("Agent 列应完整显示 claude，实际: %q", out)
	}
	// First Question 应单行（数据行数 = 表头 + 分隔线 + 1 数据行）
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("应 3 行输出（表头+分隔+1 数据行，单行显示），实际 %d 行", len(lines))
	}
}

func timePtr(y, m, d, h, mi int) *time.Time {
	t := time.Date(y, time.Month(m), d, h, mi, 0, 0, time.Local)
	return &t
}
