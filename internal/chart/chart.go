// Package chart 提供终端 ASCII 图表渲染（柱状图、折线图、堆叠比例、面积曲线）。
package chart

import (
	"fmt"
	"strings"

	"github.com/mattn/go-runewidth"
)

// Bar 是柱状图渲染。values 对应各柱数值，labels 为可选标签。
// height 为图表高度（行数）。左侧带 y 轴刻度，顶部带柱值刻度，底部带时间标签。
func Bar(values []float64, labels []string, height int) string {
	return bar(values, labels, height, 0)
}

// BarW 是带最大列数的柱状图渲染。maxCols>0 时抽样到不超过 maxCols 列。
func BarW(values []float64, labels []string, height, maxCols int) string {
	return bar(values, labels, height, maxCols)
}

func bar(values []float64, labels []string, height, maxCols int) string {
	if height <= 0 {
		height = 10
	}
	if len(values) == 0 {
		return ""
	}
	// 抽样到 maxCols 列（每列 2 字符：█+空格）
	if maxCols > 0 && len(values) > maxCols {
		step := float64(len(values)) / float64(maxCols)
		sampled := make([]float64, 0, maxCols)
		sampledLabels := make([]string, 0, maxCols)
		for i := 0; i < maxCols; i++ {
			idx := int(float64(i) * step)
			if idx >= len(values) {
				idx = len(values) - 1
			}
			sampled = append(sampled, values[idx])
			if len(labels) == len(values) {
				sampledLabels = append(sampledLabels, labels[idx])
			}
		}
		values, labels = sampled, sampledLabels
	}
	maxV := 0.0
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	yWidth := axisWidth(maxV)

	var sb strings.Builder
	// 顶部到低的柱（r=0 顶，r=height-1 底）
	for r := 0; r < height; r++ {
		var frac float64
		if height > 1 {
			frac = float64(height-1-r) / float64(height-1)
		} else {
			frac = 0 // height==1 时只有底行，所有值都显示
		}
		threshold := maxV * frac
		sb.WriteString(axisLabel(r, height, maxV, yWidth))
		for _, v := range values {
			if v >= threshold {
				sb.WriteString("█")
			} else {
				sb.WriteString(" ")
			}
			sb.WriteString(" ")
		}
		sb.WriteString("\n")
	}
	// 数值刻度（柱多时降采样）
	step := 1
	for len(values)/step > 12 {
		step++
	}
	sb.WriteString(strings.Repeat(" ", yWidth))
	for i, v := range values {
		if i%step != 0 {
			sb.WriteString("  ")
			continue
		}
		sb.WriteString(fmt.Sprintf("%s ", compact(v)))
	}
	sb.WriteString("\n")
	// 标签（柱多时降采样，保留完整 HH:MM）
	if len(labels) > 0 {
		sb.WriteString(strings.Repeat(" ", yWidth))
		for i, l := range labels {
			if i%step != 0 {
				sb.WriteString("   ")
				continue
			}
			sb.WriteString(trunc(l, 5) + " ")
		}
		sb.WriteString("\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

// Area 是面积曲线图渲染。values 为数据点，labels 为对应时间标签。
// height 为图表高度（行数），yWidth 为 y 轴宽度（<=0 自动），plotWidth 为绘图列宽。
// 左侧 y 轴显示 Token 刻度，底部为降采样时间轴。
func Area(values []float64, labels []string, height, yWidth, plotWidth int) string {
	if height <= 0 {
		height = 8
	}
	if plotWidth <= 0 {
		plotWidth = 60
	}
	if len(values) == 0 {
		return ""
	}
	// 抽样到 plotWidth 列
	if len(values) > plotWidth {
		step := float64(len(values)) / float64(plotWidth)
		sampled := make([]float64, 0, plotWidth)
		sampledLabels := make([]string, 0, plotWidth)
		for i := 0; i < plotWidth; i++ {
			idx := int(float64(i) * step)
			if idx >= len(values) {
				idx = len(values) - 1
			}
			sampled = append(sampled, values[idx])
			if len(labels) == len(values) {
				sampledLabels = append(sampledLabels, labels[idx])
			}
		}
		values, labels = sampled, sampledLabels
	} else if len(labels) != len(values) {
		labels = make([]string, len(values))
	}
	maxV := 0.0
	for _, v := range values {
		if v > maxV {
			maxV = v
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	if yWidth <= 0 {
		yWidth = axisWidth(maxV)
	}

	var sb strings.Builder
	for row := 0; row < height; row++ {
		// 第 0 行为顶部
		frac := float64(height-1-row) / float64(height-1)
		threshold := maxV * frac
		sb.WriteString(axisLabel(row, height, maxV, yWidth))
		for _, v := range values {
			if v >= threshold {
				sb.WriteString("█")
			} else {
				sb.WriteString(" ")
			}
		}
		sb.WriteString("\n")
	}
	// 底部时间轴：每格至少 6 列（5 列 HH:MM + 1 列分隔），标签不超过 14 个
	step := len(values) / 14
	if step < 6 {
		step = 6
	}
	if step < 1 {
		step = 1
	}
	for i := 0; i < len(values); i += step {
		w := step
		if i+w > len(values) {
			w = len(values) - i
		}
		// 剩余宽度不足 3 列时不渲染残缺标签
		if w < 3 {
			sb.WriteString(strings.Repeat(" ", w))
			continue
		}
		lw := w - 1
		if lw < 1 {
			lw = 1
		}
		label := cut(labels[i/step], lw)
		sb.WriteString(label)
		if pad := w - runewidth.StringWidth(label); pad > 0 {
			sb.WriteString(strings.Repeat(" ", pad))
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

// cut 返回字符串前 n 个显示宽度字符（无省略号，用于时间轴对齐）。
func cut(s string, n int) string {
	if runewidth.StringWidth(s) <= n {
		return s
	}
	var out []rune
	width := 0
	for _, rn := range s {
		if width+runewidth.RuneWidth(rn) > n {
			break
		}
		out = append(out, rn)
		width += runewidth.RuneWidth(rn)
	}
	return string(out)
}

// axisWidth 计算 y 轴刻度宽度（含 1 空格分隔）。
func axisWidth(maxV float64) int {
	w := runewidth.StringWidth(compact(maxV))
	if w < 3 {
		w = 3
	}
	return w + 1
}

// axisLabel 生成某行的 y 轴刻度（右侧留 1 空格）。顶部 max，中部 max/2，底部 0。
func axisLabel(row, rowCount int, maxV float64, yWidth int) string {
	label := ""
	switch {
	case row == 0:
		label = compact(maxV)
	case rowCount > 1 && row == rowCount/2:
		label = compact(maxV / 2)
	case row == rowCount-1:
		label = "0"
	}
	if label == "" {
		return strings.Repeat(" ", yWidth)
	}
	return padLeft(label, yWidth)
}

func padLeft(s string, w int) string {
	if runewidth.StringWidth(s) >= w {
		return s
	}
	return strings.Repeat(" ", w-runewidth.StringWidth(s)) + s
}

// Line 是折线图渲染。points 为数据点，width 为图表宽度。
// 返回多行文本，用 ▁▂▃▄▅▆▇█ 表示值高低。
func Line(points []float64, width int) string {
	if width <= 0 {
		width = 40
	}
	if len(points) == 0 {
		return ""
	}
	// 抽样到 width 个点
	step := float64(len(points)) / float64(width)
	sampled := make([]float64, 0, width)
	for i := 0; i < width; i++ {
		idx := int(float64(i) * step)
		if idx >= len(points) {
			idx = len(points) - 1
		}
		sampled = append(sampled, points[idx])
	}
	maxV := 0.0
	for _, v := range sampled {
		if v > maxV {
			maxV = v
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	const bars = "▁▂▃▄▅▆▇█"
	barsRunes := []rune(bars)
	var sb strings.Builder
	for _, v := range sampled {
		idx := int(float64(len(barsRunes)-1) * v / maxV)
		if idx < 0 {
			idx = 0
		}
		if idx >= len(barsRunes) {
			idx = len(barsRunes) - 1
		}
		sb.WriteString(string(barsRunes[idx]))
	}
	return sb.String()
}

// Ratio 是占比横条渲染。ratio 0~1，width 为条宽。
func Ratio(ratio float64, width int) string {
	if width <= 0 {
		width = 20
	}
	if ratio < 0 {
		ratio = 0
	}
	if ratio > 1 {
		ratio = 1
	}
	filled := int(ratio * float64(width))
	return strings.Repeat("█", filled) + strings.Repeat("░", width-filled)
}

func compact(v float64) string {
	switch {
	case v >= 1_000_000_000:
		return fmt.Sprintf("%.1fG", v/1_000_000_000)
	case v >= 1_000_000:
		return fmt.Sprintf("%.1fM", v/1_000_000)
	case v >= 1_000:
		return fmt.Sprintf("%.1fK", v/1_000)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
