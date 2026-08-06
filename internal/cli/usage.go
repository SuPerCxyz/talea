package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/chart"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/cost"
	"github.com/talea/talea/internal/doctor"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/insights"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/timeline"
	"github.com/talea/talea/internal/usage"
)

func newUsageCmd() *cobra.Command {
	var (
		agentFlag        string
		detailsFlag      bool
		includeSubagents bool
		metricsFlag      bool
	)
	cmd := &cobra.Command{
		Use:   "usage <session-id>",
		Short: i18n.Tr("token usage summary", "Token 汇总"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			db, err := index.Open(a.Paths.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Migrate(ctx); err != nil {
				return err
			}

			if metricsFlag {
				return showMetrics(ctx, db, sess)
			}
			u, err := usage.Load(ctx, db, sess.AgentInstanceID, sess.SessionID)
			if err != nil || u == nil {
				fmt.Println(i18n.Tr("this agent session format has no token usage data", "当前 Agent 会话格式未提供 Token 使用数据"))
				return nil
			}

			fmt.Printf("%s %s\n", i18n.Tr("Session:", "会话："), sess.SessionID)
			fmt.Printf("%s %s\n", i18n.Tr("Agent:", "Agent："), sess.AgentID)
			printUsage(i18n.Tr("Input", "输入"), u.InputTokens)
			printUsage(i18n.Tr("Output", "输出"), u.OutputTokens)
			printUsage(i18n.Tr("Total", "总计"), u.TotalTokens)
			printUsage(i18n.Tr("Cache read", "缓存读"), u.CacheReadTokens)
			printUsage(i18n.Tr("Cache write", "缓存写"), u.CacheWriteTokens)
			printUsage(i18n.Tr("Reasoning", "推理"), u.ReasoningTokens)
			printUsage(i18n.Tr("Requests", "请求数"), u.RequestCount)
			printUsage(i18n.Tr("Peak context", "上下文峰值"), u.PeakContextTokens)
			if u.DirectChildTokens != nil {
				printUsage(i18n.Tr("Direct sub-sessions", "直接子会话"), u.DirectChildTokens)
			}
			if u.DescendantTokens != nil {
				printUsage(i18n.Tr("All descendants", "所有后代会话"), u.DescendantTokens)
			}
			fmt.Printf("%s %s\n", i18n.Tr("Data source:", "数据来源："), u.Source)
			fmt.Printf("%s %s\n", i18n.Tr("Completeness:", "完整性："), u.Completeness)
			if u.IsEstimated {
				fmt.Println(i18n.Tr("Note: some values are estimates", "注：部分数值为估算值"))
			}
			// 费用估算（仅当配置启用）
			if a.Config.Usage.EstimateCost {
				if micros, currency, at, ok := cost.Estimate(u, a.Config.Usage.Pricing); ok {
					fmt.Printf(i18n.Tr("Estimated cost: %s (%s, price snapshot %s)\n", "估算费用：%s（%s，价格快照 %s）\n"),
						cost.Format(*micros, currency), currency, at.Format("2006-01-02 15:04"))
					fmt.Println(i18n.Tr("Note: estimated cost is not a substitute for vendor billing", "注：估算费用不替代供应商账单"))
				} else {
					fmt.Println(i18n.Tr("Estimated cost: cannot estimate (missing price table or model info)", "估算费用：无法估算（缺少价格表或模型信息）"))
				}
			}

			if detailsFlag {
				events, err := timeline.List(ctx, db, timeline.Query{
					AgentInstanceID: sess.AgentInstanceID,
					SessionID:       sess.SessionID,
					Limit:           200,
				})
				if err == nil && len(events) > 0 {
					fmt.Println("\n" + i18n.Tr("Request-level details:", "请求级明细："))
					fmt.Printf("  %-10s  %-14s  %-8s  %-8s  %-8s  %-8s\n",
						i18n.Tr("Time", "时间"), i18n.Tr("Model", "模型"), i18n.Tr("Input", "输入"),
						i18n.Tr("Output", "输出"), i18n.Tr("Cache read", "缓存读"), i18n.Tr("Total", "总计"))
					for _, e := range events {
						if e.EventType != model.UsageEventRequest {
							continue
						}
						fmt.Printf("  %-10s  %-14s  %-8s  %-8s  %-8s  %-8s\n",
							fmtTS(e.Timestamp), truncModel(e.Model),
							optHuman(e.InputTokens), optHuman(e.OutputTokens),
							optHuman(e.CacheReadTokens), optHuman(e.TotalTokens))
					}
				}
			}
			_ = includeSubagents
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("agent ID", "Agent 标识"))
	cmd.Flags().BoolVar(&detailsFlag, "details", false, i18n.Tr("show timeline details", "显示时间线明细"))
	cmd.Flags().BoolVar(&includeSubagents, "include-subagents", false, i18n.Tr("include sub-agents", "包含子 Agent"))
	cmd.Flags().BoolVar(&metricsFlag, "metrics", false, i18n.Tr("token rate and share metrics", "Token 速率与占比指标"))
	return cmd
}

// showMetrics 展示 Token 速率/缓存/模型占比指标。
func showMetrics(ctx context.Context, db *index.DB, sess *model.Session) error {
	m, err := timeline.ComputeMetrics(ctx, db, sess.AgentInstanceID, sess.SessionID)
	if err != nil {
		return err
	}
	fmt.Printf("%s %s\n", i18n.Tr("Session:", "会话："), sess.SessionID)
	fmt.Printf("%s %s\n", i18n.Tr("Duration:", "时长："), humanDur(m.DurationSeconds))
	fmt.Printf("%s %.0f\n", i18n.Tr("Tokens/min:", "Token/分钟："), m.TokenPerMinute)
	fmt.Printf("%s %s\n", i18n.Tr("Cumulative tokens:", "累计 Token："), human(m.CumulativeTotal))
	fmt.Printf("%s %.1f%%  %s %.1f%%\n", i18n.Tr("Input share:", "输入占比："), m.InputShare*100, i18n.Tr("Output share:", "输出占比："), m.OutputShare*100)
	fmt.Printf("%s %.1f%%\n", i18n.Tr("Cache utilization:", "缓存利用率："), m.CacheUtilization*100)
	fmt.Printf("%s %d\n", i18n.Tr("Requests:", "请求数："), m.Requests)
	if len(m.ModelShare) > 0 {
		fmt.Println("\n" + i18n.Tr("Model share:", "模型占比："))
		total := int64(0)
		for _, v := range m.ModelShare {
			total += v
		}
		if total > 0 {
			for name, v := range m.ModelShare {
				fmt.Printf("  %-24s %s %.1f%%\n", truncModel(name), chart.Ratio(float64(v)/float64(total), 20), float64(v)/float64(total)*100)
			}
		}
	}
	return nil
}

func humanDur(sec int64) string {
	if sec <= 0 {
		return i18n.Tr("unknown", "未知")
	}
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	if h > 0 {
		return fmt.Sprintf("%dh%dm%ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm%ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

func newTimelineCmd() *cobra.Command {
	var (
		agentFlag    string
		groupBy      string
		bucket       string
		aroundPeak   bool
		formatFlag   string
		outputFile   string
		byModelFlag  bool
		contextFlag  bool
		insightsFlag bool
		chartFlag    string
	)
	cmd := &cobra.Command{
		Use:   "timeline <session-id>",
		Short: i18n.Tr("token timeline", "Token 时间线"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			db, err := index.Open(a.Paths.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Migrate(ctx); err != nil {
				return err
			}

			// 独立视图优先
			if byModelFlag {
				return showByModel(ctx, db, sess, outputFile, formatFlag)
			}
			if contextFlag {
				return showContext(ctx, db, sess, outputFile, formatFlag)
			}
			if insightsFlag {
				return showInsights(ctx, db, sess)
			}
			if chartFlag != "" {
				return showChart(ctx, db, sess, chartFlag)
			}

			events, err := timeline.List(ctx, db, timeline.Query{
				AgentInstanceID: sess.AgentInstanceID,
				SessionID:       sess.SessionID,
				Limit:           500,
			})
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Println(i18n.Tr("this session has no token timeline data", "该会话没有 Token 时间线数据"))
				return nil
			}

			summary, _ := timeline.Aggregate(ctx, db, sess.AgentInstanceID, sess.SessionID)
			writer := os.Stdout
			var f *os.File
			if outputFile != "" {
				f, err = os.Create(outputFile)
				if err != nil {
					return err
				}
				defer f.Close()
				writer = f
			}
			fmt.Fprintf(writer, "%s %s  %s %d  %s %s  %s %s\n",
				i18n.Tr("Session:", "会话："), sess.SessionID,
				i18n.Tr("Requests:", "请求："), summary.RequestCount,
				i18n.Tr("Cumulative total:", "累计总计："), humanP(summary.CumulativeTotal),
				i18n.Tr("Peak context:", "上下文峰值："), humanP(summary.PeakContext))
			fmt.Fprintln(writer)

			switch groupBy {
			case "turn":
				turns, err := timeline.GroupByTurns(ctx, db, sess.AgentInstanceID, sess.SessionID)
				if err != nil {
					return err
				}
				if err := writeTurns(writer, turns, output.Format(formatFlag)); err != nil {
					return err
				}
			case "request", "":
				if bucket != "" {
					if err := writeBuckets(writer, ctx, db, sess, bucket, aroundPeak, output.Format(formatFlag)); err != nil {
						return err
					}
				} else {
					if err := writeEvents(writer, events, output.Format(formatFlag)); err != nil {
						return err
					}
				}
			default:
				return errors.New(i18n.Trf("unsupported group-by: %s", "不支持的 group-by：%s", groupBy))
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("agent ID", "Agent 标识"))
	cmd.Flags().StringVar(&groupBy, "group-by", "", i18n.Tr("aggregation dimension: turn/request", "聚合维度：turn/request"))
	cmd.Flags().StringVar(&bucket, "bucket", "", i18n.Tr("time bucket: 1m/5m/15m/1h", "时间桶：1m/5m/15m/1h"))
	cmd.Flags().BoolVar(&aroundPeak, "around-peak", false, i18n.Tr("around peak", "峰值附近"))
	cmd.Flags().StringVar(&formatFlag, "format", "table", i18n.Tr("output format: table/json/csv/markdown", "输出格式：table/json/csv/markdown"))
	cmd.Flags().StringVar(&outputFile, "output", "", i18n.Tr("output file", "输出文件"))
	cmd.Flags().BoolVar(&byModelFlag, "by-model", false, i18n.Tr("summarize by model", "按模型汇总"))
	cmd.Flags().BoolVar(&contextFlag, "context", false, i18n.Tr("context window curve", "上下文窗口曲线"))
	cmd.Flags().BoolVar(&insightsFlag, "insights", false, i18n.Tr("token consumption insights", "Token 消耗洞察"))
	cmd.Flags().StringVar(&chartFlag, "chart", "", i18n.Tr("chart: rate/cumulative/context", "图表：rate(每桶速率)/cumulative(累计)/context(上下文)"))
	return cmd
}

// showChart 渲染终端图表。
func showChart(ctx context.Context, db *index.DB, sess *model.Session, chartKind string) error {
	switch chartKind {
	case "rate", "cumulative":
		return showRateChart(ctx, db, sess, chartKind)
	case "context":
		pts, err := timeline.ContextCurve(ctx, db, sess.AgentInstanceID, sess.SessionID, 60)
		if err != nil {
			return err
		}
		if len(pts) == 0 {
			return errors.New(i18n.Tr("no context data", "没有上下文数据"))
		}
		vals := make([]float64, len(pts))
		for i, p := range pts {
			vals[i] = float64(p.Context)
		}
		fmt.Println(i18n.Tr("Context curve:", "上下文曲线："))
		fmt.Println(chart.Line(vals, 60))
		return nil
	default:
		return errors.New(i18n.Trf("unknown chart type: %s (available rate/cumulative/context)", "未知图表类型：%s（可用 rate/cumulative/context）", chartKind))
	}
}

// showRateChart 渲染每桶 Token 速率柱状图或累计曲线。
func showRateChart(ctx context.Context, db *index.DB, sess *model.Session, kind string) error {
	buckets, err := timeline.GroupByBucket(ctx, db, sess.AgentInstanceID, sess.SessionID, timeline.Bucket5m)
	if err != nil {
		return err
	}
	if len(buckets) == 0 {
		return errors.New(i18n.Tr("no aggregable data", "没有可聚合的数据"))
	}
	if kind == "cumulative" {
		vals := make([]float64, len(buckets))
		var cum int64
		for i, b := range buckets {
			cum += b.TotalTokens
			vals[i] = float64(cum)
		}
		fmt.Println(i18n.Tr("Cumulative token curve (5-min buckets):", "累计 Token 曲线（每 5 分钟桶）："))
		fmt.Println(chart.Line(vals, 60))
		return nil
	}
	// rate：每桶 Token，转成 Token/分钟
	vals := make([]float64, len(buckets))
	labels := make([]string, len(buckets))
	for i, b := range buckets {
		vals[i] = float64(b.TotalTokens) / 5.0
		labels[i] = b.Start.Format("15:04")
	}
	fmt.Println(i18n.Tr("Tokens/min bar chart (5-min buckets):", "Token/分钟 柱状图（每 5 分钟桶）："))
	fmt.Println(chart.Bar(vals, labels, 8))
	return nil
}

func showByModel(ctx context.Context, db *index.DB, sess *model.Session, outputFile, formatFlag string) error {
	sums, err := timeline.ByModel(ctx, db, sess.AgentInstanceID, sess.SessionID)
	if err != nil {
		return err
	}
	w := os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	switch output.Format(formatFlag) {
	case output.FormatJSON:
		return json.NewEncoder(w).Encode(sums)
	case output.FormatCSV:
		cw := csv.NewWriter(w)
		defer cw.Flush()
		cw.Write([]string{"model", "requests", "input", "output", "total", "cache_read", "reasoning"})
		for _, m := range sums {
			cw.Write([]string{m.Model, fmt.Sprint(m.Requests), fmt.Sprint(m.InputTokens),
				fmt.Sprint(m.OutputTokens), fmt.Sprint(m.TotalTokens), fmt.Sprint(m.CacheRead), fmt.Sprint(m.Reasoning)})
		}
		return nil
	default:
		fmt.Fprintf(w, "%-24s  %-8s  %-10s  %-10s  %-10s\n",
			i18n.Tr("Model", "模型"), i18n.Tr("Requests", "请求"),
			i18n.Tr("Input", "输入"), i18n.Tr("Output", "输出"), i18n.Tr("Total", "总计"))
		for _, m := range sums {
			fmt.Fprintf(w, "%-24s  %-8d  %-10s  %-10s  %-10s\n",
				m.Model, m.Requests, human(m.InputTokens), human(m.OutputTokens), human(m.TotalTokens))
		}
		return nil
	}
}

func showContext(ctx context.Context, db *index.DB, sess *model.Session, outputFile, formatFlag string) error {
	pts, err := timeline.ContextCurve(ctx, db, sess.AgentInstanceID, sess.SessionID, 100)
	if err != nil {
		return err
	}
	if len(pts) == 0 {
		fmt.Println(i18n.Tr("no context window data", "没有上下文窗口数据"))
		return nil
	}
	w := os.Stdout
	if outputFile != "" {
		f, err := os.Create(outputFile)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}
	comps, _ := timeline.DetectCompactions(ctx, db, sess.AgentInstanceID, sess.SessionID)
	fmt.Fprintf(w, i18n.Tr("Context window curve (%d sample points):\n\n", "上下文窗口曲线（%d 个采样点）：\n\n"), len(pts))
	fmt.Fprintf(w, "%-10s  %-10s  %-10s  %-12s\n",
		i18n.Tr("Time", "时间"), i18n.Tr("Context", "上下文"), i18n.Tr("Limit", "上限"), i18n.Tr("Change", "变化"))
	for _, p := range pts {
		fmt.Fprintf(w, "%-10s  %-10s  %-10s  %+s\n",
			time.Unix(p.Timestamp, 0).Format("15:04:05"),
			human(p.Context), human(p.ContextLimit), signedHuman(p.Change))
	}
	if len(comps) > 0 {
		fmt.Fprintln(w, "\n"+i18n.Tr("Context compaction:", "上下文压缩："))
		for _, c := range comps {
			label := i18n.Tr("explicit compaction", "明确压缩")
			if c.IsInferred {
				label = i18n.Tr("possible compaction", "可能发生上下文压缩")
			}
			if c.Before > 0 && c.After > 0 {
				fmt.Fprintf(w, i18n.Tr("  %s: before %s, after %s, reduced %s, ratio %.1f%% (%s)\n", "  %s：压缩前 %s，压缩后 %s，减少 %s，压缩率 %.1f%%（%s）\n"),
					time.Unix(c.Timestamp, 0).Format("15:04:05"),
					human(c.Before), human(c.After), human(c.Reduced), c.Ratio*100, label)
			} else {
				// 明确压缩事件但无前后数值（Agent 仅记录事件）
				fmt.Fprintf(w, i18n.Tr("  %s: (%s)\n", "  %s：（%s）\n"), time.Unix(c.Timestamp, 0).Format("15:04:05"), label)
			}
		}
	}
	return nil
}

func showInsights(ctx context.Context, db *index.DB, sess *model.Session) error {
	rep, err := insights.Generate(ctx, db, sess.AgentInstanceID, sess.SessionID)
	if err != nil {
		return err
	}
	fmt.Println(i18n.Tr("Session token insights", "会话 Token 洞察"))
	if len(rep.Insights) == 0 {
		fmt.Println(i18n.Tr("no obvious abnormal consumption patterns detected.", "未检测到明显异常消耗模式。"))
		return nil
	}
	for _, ins := range rep.Insights {
		fmt.Printf("- %s\n", ins.Text)
	}
	return nil
}

func signedHuman(n int64) string {
	sign := "+"
	if n < 0 {
		sign = "-"
		n = -n
	}
	return sign + human(n)
}

func optHuman(p *int64) string {
	if p == nil {
		return i18n.Tr("unknown", "未知")
	}
	return human(*p)
}

func truncModel(m string) string {
	if m == "" {
		return i18n.Tr("unknown", "未知")
	}
	r := []rune(m)
	if len(r) > 14 {
		return string(r[:13]) + "…"
	}
	return m
}

// writeBuckets 输出时间桶聚合。
func writeBuckets(w io.Writer, ctx context.Context, db *index.DB, sess *model.Session, bucketFlag string, aroundPeak bool, format output.Format) error {
	buckets, err := timeline.GroupByBucket(ctx, db, sess.AgentInstanceID, sess.SessionID, timeline.BucketSize(bucketFlag))
	if err != nil {
		return err
	}
	if len(buckets) == 0 {
		fmt.Fprintln(w, i18n.Tr("no aggregable request data", "没有可聚合的请求数据"))
		return nil
	}
	if aroundPeak {
		// 只显示峰值桶及其前后一个桶
		maxIdx := 0
		for i := 1; i < len(buckets); i++ {
			if buckets[i].TotalTokens > buckets[maxIdx].TotalTokens {
				maxIdx = i
			}
		}
		lo := maxIdx - 1
		if lo < 0 {
			lo = 0
		}
		hi := maxIdx + 2
		if hi > len(buckets) {
			hi = len(buckets)
		}
		buckets = buckets[lo:hi]
	}
	switch format {
	case output.FormatJSON:
		return json.NewEncoder(w).Encode(buckets)
	case output.FormatCSV:
		cw := csv.NewWriter(w)
		defer cw.Flush()
		cw.Write([]string{"start", "end", "requests", "input", "output", "total", "cache_read", "reasoning"})
		for _, b := range buckets {
			cw.Write([]string{b.Start.Format("15:04"), b.End.Format("15:04"),
				fmt.Sprint(b.Requests), fmt.Sprint(b.InputTokens), fmt.Sprint(b.OutputTokens),
				fmt.Sprint(b.TotalTokens), fmt.Sprint(b.CacheRead), fmt.Sprint(b.Reasoning)})
		}
		return nil
	default:
		fmt.Fprintf(w, "%-8s  %-8s  %-8s  %-10s  %-10s  %-10s\n",
			i18n.Tr("Start", "开始"), i18n.Tr("End", "结束"), i18n.Tr("Requests", "请求"),
			i18n.Tr("Input", "输入"), i18n.Tr("Output", "输出"), i18n.Tr("Total", "总计"))
		for _, b := range buckets {
			fmt.Fprintf(w, "%-8s  %-8s  %-8d  %-10s  %-10s  %-10s\n",
				b.Start.Format("15:04"), b.End.Format("15:04"), b.Requests,
				human(b.InputTokens), human(b.OutputTokens), human(b.TotalTokens))
		}
		return nil
	}
}

func newDoctorCmd() *cobra.Command {
	var (
		jsonFlag  bool
		agentFlag string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: i18n.Tr("environment diagnostics", "环境诊断"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			rep, err := doctor.Run(ctx, a, agentFlag)
			if err != nil {
				return err
			}
			if jsonFlag {
				data, _ := rep.JSON()
				fmt.Println(string(data))
				return nil
			}
			fmt.Println("Talea Doctor")
			rep.Print()
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, i18n.Tr("JSON output", "JSON 输出"))
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("diagnose only the given agent", "仅诊断指定 Agent"))
	return cmd
}

func printUsage(label string, v *int64) {
	if v == nil {
		fmt.Printf(i18n.Tr("%s: unknown\n", "%s：未知\n"), label)
		return
	}
	fmt.Printf(i18n.Tr("%s: %s\n", "%s：%s\n"), label, human(*v))
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

func describeEvent(e timeline.Event) string {
	var parts []string
	if e.TotalTokens != nil {
		parts = append(parts, i18n.Trf("total %s", "总计 %s", human(*e.TotalTokens)))
	}
	if e.InputTokens != nil {
		parts = append(parts, i18n.Trf("input %s", "输入 %s", human(*e.InputTokens)))
	}
	if e.OutputTokens != nil {
		parts = append(parts, i18n.Trf("output %s", "输出 %s", human(*e.OutputTokens)))
	}
	if e.ContextAfter != nil {
		parts = append(parts, i18n.Trf("context %s", "上下文 %s", human(*e.ContextAfter)))
	}
	if e.ToolName != "" {
		parts = append(parts, i18n.Tr("tool ", "工具 ")+e.ToolName)
	}
	if e.UserPromptPreview != "" {
		parts = append(parts, preview(e.UserPromptPreview))
	}
	return joinParts(parts)
}

func fmtTS(t *time.Time) string {
	if t == nil {
		return i18n.Tr("unknown", "未知")
	}
	return t.Format("15:04:05")
}

func preview(s string) string {
	r := []rune(s)
	if len(r) > 40 {
		return string(r[:40]) + "…"
	}
	return s
}

func joinParts(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += "  "
		}
		out += p
	}
	return out
}

// writeEvents 输出请求级时间线。
func writeEvents(w io.Writer, events []timeline.Event, format output.Format) error {
	switch format {
	case output.FormatJSON:
		views := make([]map[string]any, 0, len(events))
		for _, e := range events {
			views = append(views, map[string]any{
				"time":      fmtTS(e.Timestamp),
				"type":      string(e.EventType),
				"model":     e.Model,
				"total":     ptrValue(e.TotalTokens),
				"input":     ptrValue(e.InputTokens),
				"output":    ptrValue(e.OutputTokens),
				"reasoning": ptrValue(e.ReasoningTokens),
			})
		}
		return json.NewEncoder(w).Encode(views)
	case output.FormatCSV:
		cw := csv.NewWriter(w)
		defer cw.Flush()
		cw.Write([]string{"time", "type", "model", "total", "input", "output", "reasoning"})
		for _, e := range events {
			cw.Write([]string{fmtTS(e.Timestamp), string(e.EventType), e.Model,
				intStr(e.TotalTokens), intStr(e.InputTokens), intStr(e.OutputTokens), intStr(e.ReasoningTokens)})
		}
		return nil
	case output.FormatMarkdown:
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n",
			i18n.Tr("Time", "时间"), i18n.Tr("Type", "类型"), i18n.Tr("Model", "模型"),
			i18n.Tr("Total", "总计"), i18n.Tr("Input", "输入"), i18n.Tr("Output", "输出"), i18n.Tr("Reasoning", "推理"))
		fmt.Fprintf(w, "|------|------|------|------|------|------|------|\n")
		for _, e := range events {
			fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s |\n",
				fmtTS(e.Timestamp), string(e.EventType), mdCell(e.Model),
				mdCell(intStr(e.TotalTokens)), mdCell(intStr(e.InputTokens)),
				mdCell(intStr(e.OutputTokens)), mdCell(intStr(e.ReasoningTokens)))
		}
		return nil
	default:
		for _, e := range events {
			fmt.Fprintf(w, "%s  %-18s  %s\n", fmtTS(e.Timestamp), string(e.EventType), describeEvent(e))
		}
		return nil
	}
}

func mdCell(s string) string {
	s = strings.ReplaceAll(s, "|", "\\|")
	s = strings.ReplaceAll(s, "\n", " ")
	if s == "" {
		return "-"
	}
	return s
}

// writeTurns 输出用户轮次聚合。
func writeTurns(w io.Writer, turns []timeline.TurnUsage, format output.Format) error {
	switch format {
	case output.FormatJSON:
		return json.NewEncoder(w).Encode(turns)
	case output.FormatCSV:
		cw := csv.NewWriter(w)
		defer cw.Flush()
		cw.Write([]string{"turn", "prompt", "started_at", "ended_at", "requests", "total", "input", "output", "reasoning"})
		for _, t := range turns {
			cw.Write([]string{fmt.Sprint(t.Index), t.Prompt,
				t.StartedAt.Format("15:04:05"), t.EndedAt.Format("15:04:05"),
				fmt.Sprint(t.Requests), fmt.Sprint(t.Total), fmt.Sprint(t.Input),
				fmt.Sprint(t.Output), fmt.Sprint(t.Reasoning)})
		}
		return nil
	default:
		fmt.Fprintf(w, "%-5s  %-40s  %-8s  %-6s  %-6s  %s\n",
			i18n.Tr("Turn", "轮次"), i18n.Tr("Question", "提问"), i18n.Tr("Requests", "请求"),
			i18n.Tr("Tools", "工具"), i18n.Tr("Token", "Token"), "")
		for _, t := range turns {
			fmt.Fprintf(w, "%-5d  %-40s  %-8d  %-6d  %s\n",
				t.Index, preview(t.Prompt), t.Requests, 0, human(t.Total))
		}
		return nil
	}
}

func humanP(n int64) string {
	if n == 0 {
		return "0"
	}
	return human(n)
}

func ptrValue(p *int64) any {
	if p == nil {
		return nil
	}
	return *p
}

func intStr(p *int64) string {
	if p == nil {
		return ""
	}
	return fmt.Sprint(*p)
}
