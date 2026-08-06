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

func TestWrapFirst(t *testing.T) {
	cases := []struct {
		name string
		in   string
		w    int
		want int // 期望行数
	}{
		{name: "short", in: "你好", w: 10, want: 1},
		{name: "exact", in: "1234567890", w: 10, want: 1},
		{name: "two lines", in: "中文内容足够长需要换行显示两行内容", w: 10, want: 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := wrapFirst(c.in, c.w)
			if len(got) != c.want {
				t.Errorf("wrapFirst(%q,%d) = %d 行 %v, want %d", c.in, c.w, len(got), got, c.want)
			}
			for _, line := range got {
				if runeLen(line) > c.w {
					t.Errorf("行宽度 %d 超过列宽 %d: %q", runeLen(line), c.w, line)
				}
			}
			if len(got) > 1 && strings.Contains(got[0], "…") {
				t.Errorf("第一行不应有省略号: %q", got[0])
			}
		})
	}
}

func TestWrapFirstEllipsisOnLastLine(t *testing.T) {
	long := strings.Repeat("这是一个非常长的问题用于验证最后一行超出时省略号只在末尾出现 ", 5)
	got := wrapFirst(long, 20)
	if len(got) != 2 {
		t.Fatalf("应两行，实际 %d 行: %v", len(got), got)
	}
	if strings.Contains(got[0], "…") {
		t.Errorf("第一行不应有省略号: %q", got[0])
	}
	if !strings.Contains(got[1], "…") {
		t.Errorf("第二行超宽应有省略号: %q", got[1])
	}
}

func TestWriteTableDynamicWidth(t *testing.T) {
	sessions := []*model.Session{
		{
			AgentID:          model.AgentClaudeCode,
			SessionID:        "abcdef",
			FirstQuestion:    strings.Repeat("这是一个很长的问题用于验证表格输出中首次提问列会换行显示为两行且不超过列宽 ", 3),
			StartedAt:        timePtr(2026, 8, 5, 16, 41),
			WorkingDirectory: "/home/alice/code/very-long-project-name",
		},
	}
	var buf strings.Builder
	if err := Write(&buf, sessions, FormatTable); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	// claude-code 应完整显示（不截断）
	if !strings.Contains(out, "claude-code") {
		t.Errorf("Agent 列应完整显示 claude-code，实际: %q", out)
	}
	// 长目录应完整显示
	if !strings.Contains(out, "very-long-project-name") {
		t.Errorf("CWD 应完整显示长目录")
	}
	// First Question 应产生两行
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 4 {
		t.Fatalf("应有两行输出（表头+分隔+内容2行），实际 %d 行", len(lines))
	}
}

func timePtr(y, m, d, h, mi int) *time.Time {
	t := time.Date(y, time.Month(m), d, h, mi, 0, 0, time.Local)
	return &t
}
