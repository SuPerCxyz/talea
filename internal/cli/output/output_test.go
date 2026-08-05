package output

import (
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
		AgentID:           model.AgentClaudeCode,
		SessionID:         "abc",
		FirstQuestion:     "q",
		WorkingDirectory:  "/home/alice/code/x",
		WorkingDirExists:  true,
		Activity:          model.ActivityInactive,
	}
	v := ViewOf(s)
	if v.Tokens != "未知" {
		t.Fatalf("tokens: %q", v.Tokens)
	}
	if v.Activity != "已结束" {
		t.Fatalf("activity: %q", v.Activity)
	}
}
