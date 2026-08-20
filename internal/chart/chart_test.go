package chart

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
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

func TestArea(t *testing.T) {
	out := Area([]float64{1, 2, 3, 4, 5, 4, 3, 2, 1}, nil, 9, 6, 9)
	if out == "" {
		t.Fatal("empty area")
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 10 {
		t.Fatalf("expected >=10 lines (9 plot + time axis), got %d", len(lines))
	}
	// 顶部应有 y 轴 max 刻度
	if !strings.Contains(lines[0], "5") {
		t.Fatalf("top axis should show max label: %q", lines[0])
	}
	// 底部时间轴行存在
	last := lines[len(lines)-1]
	if last == "" {
		t.Fatal("expected time axis row")
	}
}

func TestAreaEmpty(t *testing.T) {
	if out := Area(nil, nil, 8, 0, 40); out != "" {
		t.Fatalf("expected empty, got %q", out)
	}
}

// TestAreaStretch 数据点少于绘图宽度时应拉伸占满，且时间轴带 y 轴前缀对齐。
func TestAreaStretch(t *testing.T) {
	vals := []float64{100, 300, 200}
	labels := []string{"09:00", "09:05", "09:10"}
	plotW := 10
	yW := AxisWidth(300)
	out := Area(vals, labels, 9, yW, plotW)
	rows := strings.Split(out, "\n")
	if len(rows) != 10 {
		t.Fatalf("expected 10 rows (9 plot + 1 time axis), got %d", len(rows))
	}
	for _, r := range rows[:9] {
		if runewidth.StringWidth(r) != yW+plotW {
			t.Fatalf("plot row width=%d want %d: %q", runewidth.StringWidth(r), yW+plotW, r)
		}
	}
	// 时间轴行应与绘图区同宽且带 y 轴前缀
	if got := runewidth.StringWidth(rows[9]); got != yW+plotW {
		t.Fatalf("time axis width=%d want %d", got, yW+plotW)
	}
	if !strings.HasPrefix(rows[9], strings.Repeat(" ", yW)) {
		t.Fatalf("time axis should be offset by yWidth: %q", rows[9])
	}
}

// TestBarStretch 柱数少于目标列时应拉伸占满宽度。
func TestBarStretch(t *testing.T) {
	vals := []float64{128.7, 47.6, 334.8, 49.3}
	labels := []string{"09:20", "09:25", "09:30", "09:35"}
	maxCols := 40
	yW := AxisWidth(334.8)
	out := BarW(vals, labels, 6, maxCols)
	rows := strings.Split(out, "\n")
	if len(rows) < 8 {
		t.Fatalf("expected >=8 rows, got %d", len(rows))
	}
	// 柱行（含 y 轴）总宽应为 yWidth + 2*maxCols
	if got := runewidth.StringWidth(rows[0]); got != yW+maxCols*2 {
		t.Fatalf("bar row width=%d want %d", got, yW+maxCols*2)
	}
	// 数值刻度与时间标签均带 y 轴前缀
	for _, r := range rows[6:] {
		if runewidth.StringWidth(r) < yW {
			t.Fatalf("row width=%d < yWidth=%d: %q", runewidth.StringWidth(r), yW, r)
		}
	}
}

func TestBarHasYAxis(t *testing.T) {
	out := Bar([]float64{1, 5, 3}, []string{"a", "b", "c"}, 5)
	lines := strings.Split(out, "\n")
	// 顶部行应有 max 刻度（5）
	if !strings.Contains(lines[0], "5") {
		t.Fatalf("top axis should show max label: %q", lines[0])
	}
	// 底部柱行应有 0 刻度（紧跟柱体）
	if !strings.Contains(out, "0█") {
		t.Fatalf("expected bottom axis 0 label before bars, lines=%q", lines)
	}
}

func TestAxisLabel(t *testing.T) {
	if got := strings.TrimSpace(axisLabel(0, 9, 500_000, 7)); got != "500.0K" {
		t.Fatalf("top axis=%q", got)
	}
	if got := strings.TrimSpace(axisLabel(8, 9, 500_000, 7)); got != "0" {
		t.Fatalf("bottom axis=%q", got)
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
