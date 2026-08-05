package chart

import (
	"strings"
	"testing"
)

func TestBar(t *testing.T) {
	out := Bar([]float64{1, 5, 3}, []string{"a", "b", "c"}, 5)
	if out == "" {
		t.Fatal("empty bar")
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 6 {
		t.Fatalf("expected >=6 lines, got %d", len(lines))
	}
	// 最高的柱（值5）在所有行都有█（最高行）
	if !strings.Contains(lines[0], "█") {
		t.Fatalf("tallest bar should reach top: %q", lines[0])
	}
}

func TestBarEmpty(t *testing.T) {
	if out := Bar(nil, nil, 5); out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

func TestLine(t *testing.T) {
	out := Line([]float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}, 10)
	if out == "" {
		t.Fatal("empty line")
	}
	// 递增序列应从低到高
	if strings.HasPrefix(out, "█") {
		t.Fatalf("increasing line should start low: %q", out)
	}
	if !strings.HasSuffix(out, "█") {
		t.Fatalf("increasing line should end high: %q", out)
	}
}

func TestLineEmpty(t *testing.T) {
	if out := Line(nil, 10); out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

func TestRatio(t *testing.T) {
	out := Ratio(0.5, 10)
	if len([]rune(out)) != 10 {
		t.Fatalf("ratio length=%d", len([]rune(out)))
	}
	// 0.5 -> 一半█
	if strings.Count(out, "█") != 5 {
		t.Fatalf("ratio filled=%d", strings.Count(out, "█"))
	}
}

func TestRatioClamp(t *testing.T) {
	if Ratio(2.0, 10) != strings.Repeat("█", 10) {
		t.Fatal("ratio should clamp to 1")
	}
	if Ratio(-1, 10) != strings.Repeat("░", 10) {
		t.Fatal("ratio should clamp to 0")
	}
}

func TestCompact(t *testing.T) {
	cases := []struct {
		v    float64
		want string
	}{
		{500, "500"},
		{5000, "5.0K"},
		{5_000_000, "5.0M"},
	}
	for _, c := range cases {
		if got := compact(c.v); got != c.want {
			t.Fatalf("compact(%v)=%q want %q", c.v, got, c.want)
		}
	}
}
