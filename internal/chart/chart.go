// Package chart 提供终端 ASCII 图表渲染（柱状图、折线图、堆叠比例）。
package chart

import (
	"fmt"
	"strings"
)

// Bar 是柱状图渲染。values 对应各柱数值，labels 为可选标签。
// height 为图表高度（行数）。返回渲染后的多行文本。
func Bar(values []float64, labels []string, height int) string {
	if height <= 0 {
		height = 10
	}
	if len(values) == 0 {
		return ""
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

	var sb strings.Builder
	// 顶部到低的柱
	for row := height; row > 0; row-- {
		threshold := maxV * float64(row) / float64(height)
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
