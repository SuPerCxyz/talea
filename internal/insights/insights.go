// Package insights 基于本地规则生成可追溯的 Token 消耗洞察。
package insights

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/timeline"
)

// Insight 是一条分析结论。
type Insight struct {
	Type string // high-request / context-overflow / compaction / growth / caching / model-switch
	Text string
}

// Report 是洞察报告。
type Report struct {
	Insights []Insight
}

// Generate 基于时间线与上下文数据生成洞察。
// 所有结论都来自聚合结果，可定位到具体事件。
func Generate(ctx context.Context, db *index.DB, instanceID, sessionID string) (Report, error) {
	var rep Report

	events, err := timeline.List(ctx, db, timeline.Query{
		AgentInstanceID: instanceID,
		SessionID:       sessionID,
		Limit:           100000,
	})
	if err != nil {
		return rep, err
	}

	// 1. 单次请求高于 P95
	if highs := p95HighRequests(events); len(highs) > 0 {
		rep.Insights = append(rep.Insights, Insight{
			Type: "high-request",
			Text: fmt.Sprintf("%d 次请求 Token 消耗高于会话 P95", len(highs)),
		})
	}

	// 2. 上下文超过窗口 80%
	if overflows := contextOverflow(events); overflows > 0 {
		rep.Insights = append(rep.Insights, Insight{
			Type: "context-overflow",
			Text: fmt.Sprintf("%d 次请求上下文达到窗口上限的 80%% 以上", overflows),
		})
	}

	// 3. 上下文压缩
	if comps, err := timeline.DetectCompactions(ctx, db, instanceID, sessionID); err == nil && len(comps) > 0 {
		rep.Insights = append(rep.Insights, Insight{
			Type: "compaction",
			Text: fmt.Sprintf("检测到 %d 次上下文压缩，最大压缩 %s Token（%.1f%%）",
				len(comps), human(comps[0].Reduced), comps[0].Ratio*100),
		})
	}

	// 4. 短时间 Token 快速增长
	if growth := rapidGrowth(events); growth {
		rep.Insights = append(rep.Insights, Insight{
			Type: "growth",
			Text: "检测到短时间内的 Token 快速增长",
		})
	}

	// 5. 缓存长期为零
	if cacheZero(events) {
		rep.Insights = append(rep.Insights, Insight{
			Type: "caching",
			Text: "缓存读取长期为零，可能未启用提示缓存",
		})
	}

	// 6. 相同文件重复读取
	if repeats := repeatedFileReads(events); len(repeats) > 0 {
		for _, f := range repeats {
			rep.Insights = append(rep.Insights, Insight{
				Type: "repeated-read",
				Text: fmt.Sprintf("文件被重复读取 %d 次：%s", f.Count, f.Path),
			})
		}
	}

	return rep, nil
}

// FileReadStat 记录同一文件的读取次数。
type FileReadStat struct {
	Path  string
	Count int
}

// repeatedFileReads 检测同一文件被重复读取（≥3 次）。
func repeatedFileReads(events []timeline.Event) []FileReadStat {
	counts := map[string]int{}
	for _, e := range events {
		if e.EventType == "tool_end" && e.ToolName == "read" && e.FilePath != "" {
			counts[e.FilePath]++
		}
	}
	var out []FileReadStat
	for path, n := range counts {
		if n >= 3 {
			out = append(out, FileReadStat{Path: path, Count: n})
		}
	}
	// 按次数降序，最多报 5 个
	sort.Slice(out, func(i, j int) bool { return out[i].Count > out[j].Count })
	if len(out) > 5 {
		out = out[:5]
	}
	return out
}

// p95HighRequests 找出超过会话 P95 的请求数。
// P95 取排序后索引 len*95/100 的值；单尖峰被计入自身时使用 P90 兜底。
func p95HighRequests(events []timeline.Event) []timeline.Event {
	var totals []int64
	for _, e := range events {
		if e.EventType == "request" && e.TotalTokens != nil {
			totals = append(totals, *e.TotalTokens)
		}
	}
	if len(totals) < 10 {
		return nil
	}
	sorted := append([]int64(nil), totals...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	idx := len(sorted) * 95 / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	threshold := sorted[idx]
	// 若阈值被极端值抬高导致无法检出，退到 P90
	if countAbove(sorted, threshold) == 0 && idx > 0 {
		idx = idx * 90 / 100
		threshold = sorted[idx]
	}

	var out []timeline.Event
	for _, e := range events {
		if e.EventType == "request" && e.TotalTokens != nil && *e.TotalTokens > threshold {
			out = append(out, e)
		}
	}
	return out
}

func countAbove(sorted []int64, threshold int64) int {
	n := 0
	for _, v := range sorted {
		if v > threshold {
			n++
		}
	}
	return n
}

func contextOverflow(events []timeline.Event) int {
	n := 0
	for _, e := range events {
		if e.ContextAfter != nil && e.ContextLimit != nil && *e.ContextLimit > 0 {
			if float64(*e.ContextAfter)/float64(*e.ContextLimit) > 0.8 {
				n++
			}
		}
	}
	return n
}

func rapidGrowth(events []timeline.Event) bool {
	// 连续 3 次请求累计输入增长超过 50k
	var prevTotal int64
	growthCount := 0
	for _, e := range events {
		if e.EventType != "request" || e.TotalTokens == nil {
			continue
		}
		if prevTotal > 0 && *e.TotalTokens-prevTotal > 50_000 {
			growthCount++
			if growthCount >= 3 {
				return true
			}
		} else {
			growthCount = 0
		}
		prevTotal = *e.TotalTokens
	}
	return false
}

func cacheZero(events []timeline.Event) bool {
	requests := 0
	withCache := 0
	for _, e := range events {
		if e.EventType != "request" {
			continue
		}
		requests++
		if e.CacheReadTokens != nil && *e.CacheReadTokens > 0 {
			withCache++
		}
	}
	return requests > 10 && withCache == 0
}

func human(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

var _ = time.Time{}
