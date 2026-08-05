package cli

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/cost"
	"github.com/talea/talea/internal/doctor"
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
	)
	cmd := &cobra.Command{
		Use:   "usage <session-id>",
		Short: "Token 汇总",
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

			u, err := usage.Load(ctx, db, sess.AgentInstanceID, sess.SessionID)
			if err != nil {
				// 无 usage 记录
				fmt.Println("当前 Agent 会话格式未提供 Token 使用数据")
				return nil
			}
			if u == nil {
				fmt.Println("当前 Agent 会话格式未提供 Token 使用数据")
				return nil
			}

			fmt.Printf("会话：%s\n", sess.SessionID)
			fmt.Printf("Agent：%s\n", sess.AgentID)
			printUsage("输入", u.InputTokens)
			printUsage("输出", u.OutputTokens)
			printUsage("总计", u.TotalTokens)
			printUsage("缓存读", u.CacheReadTokens)
			printUsage("缓存写", u.CacheWriteTokens)
			printUsage("推理", u.ReasoningTokens)
			printUsage("请求数", u.RequestCount)
			printUsage("上下文峰值", u.PeakContextTokens)
			if u.DirectChildTokens != nil {
				printUsage("直接子会话", u.DirectChildTokens)
			}
			if u.DescendantTokens != nil {
				printUsage("所有后代会话", u.DescendantTokens)
			}
			fmt.Printf("数据来源：%s\n", u.Source)
			fmt.Printf("完整性：%s\n", u.Completeness)
			if u.IsEstimated {
				fmt.Println("注：部分数值为估算值")
			}
			// 费用估算（仅当配置启用）
			if a.Config.Usage.EstimateCost {
				if micros, currency, at, ok := cost.Estimate(u, a.Config.Usage.Pricing); ok {
					fmt.Printf("估算费用：%s（%s，价格快照 %s）\n",
						cost.Format(*micros, currency), currency, at.Format("2006-01-02 15:04"))
					fmt.Println("注：估算费用不替代供应商账单")
				} else {
					fmt.Println("估算费用：无法估算（缺少价格表或模型信息）")
				}
			}

			if detailsFlag {
				events, err := timeline.List(ctx, db, timeline.Query{
					AgentInstanceID: sess.AgentInstanceID,
					SessionID:       sess.SessionID,
					Limit:           100,
				})
				if err == nil && len(events) > 0 {
					fmt.Println("\n时间线事件（前 100 条）：")
					for _, e := range events {
						fmt.Printf("  %s  %s  %s\n", fmtTS(e.Timestamp), e.EventType, e.Model)
					}
				}
			}
			_ = includeSubagents
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().BoolVar(&detailsFlag, "details", false, "显示时间线明细")
	cmd.Flags().BoolVar(&includeSubagents, "include-subagents", false, "包含子 Agent")
	return cmd
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
	)
	cmd := &cobra.Command{
		Use:   "timeline <session-id>",
		Short: "Token 时间线",
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

			events, err := timeline.List(ctx, db, timeline.Query{
				AgentInstanceID: sess.AgentInstanceID,
				SessionID:       sess.SessionID,
				Limit:           500,
			})
			if err != nil {
				return err
			}
			if len(events) == 0 {
				fmt.Println("该会话没有 Token 时间线数据")
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
			fmt.Fprintf(writer, "会话：%s  请求：%d  累计总计：%s  上下文峰值：%s\n",
				sess.SessionID, summary.RequestCount,
				humanP(summary.CumulativeTotal), humanP(summary.PeakContext))
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
				if err := writeEvents(writer, events, output.Format(formatFlag)); err != nil {
					return err
				}
			default:
				return fmt.Errorf("不支持的 group-by：%s", groupBy)
			}
			_ = bucket
			_ = aroundPeak
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "聚合维度：turn/request")
	cmd.Flags().StringVar(&bucket, "bucket", "", "时间桶：1m/5m/15m/1h")
	cmd.Flags().BoolVar(&aroundPeak, "around-peak", false, "峰值附近")
	cmd.Flags().StringVar(&formatFlag, "format", "table", "输出格式：table/json/csv/markdown")
	cmd.Flags().StringVar(&outputFile, "output", "", "输出文件")
	cmd.Flags().BoolVar(&byModelFlag, "by-model", false, "按模型汇总")
	cmd.Flags().BoolVar(&contextFlag, "context", false, "上下文窗口曲线")
	cmd.Flags().BoolVar(&insightsFlag, "insights", false, "Token 消耗洞察")
	return cmd
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
		fmt.Fprintf(w, "%-24s  %-8s  %-10s  %-10s  %-10s\n", "模型", "请求", "输入", "输出", "总计")
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
		fmt.Println("没有上下文窗口数据")
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
	fmt.Fprintf(w, "上下文窗口曲线（%d 个采样点）：\n\n", len(pts))
	fmt.Fprintf(w, "%-10s  %-10s  %-10s  %-12s\n", "时间", "上下文", "上限", "变化")
	for _, p := range pts {
		fmt.Fprintf(w, "%-10s  %-10s  %-10s  %+s\n",
			time.Unix(p.Timestamp, 0).Format("15:04:05"),
			human(p.Context), human(p.ContextLimit), signedHuman(p.Change))
	}
	if len(comps) > 0 {
		fmt.Fprintln(w, "\n上下文压缩：")
		for _, c := range comps {
			label := "明确压缩"
			if c.IsInferred {
				label = "可能发生上下文压缩"
			}
			fmt.Fprintf(w, "  %s：压缩前 %s，压缩后 %s，减少 %s，压缩率 %.1f%%（%s）\n",
				time.Unix(c.Timestamp, 0).Format("15:04:05"),
				human(c.Before), human(c.After), human(c.Reduced), c.Ratio*100, label)
		}
	}
	return nil
}

func showInsights(ctx context.Context, db *index.DB, sess *model.Session) error {
	rep, err := insights.Generate(ctx, db, sess.AgentInstanceID, sess.SessionID)
	if err != nil {
		return err
	}
	fmt.Println("会话 Token 洞察")
	if len(rep.Insights) == 0 {
		fmt.Println("未检测到明显异常消耗模式。")
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

func newDoctorCmd() *cobra.Command {
	var (
		jsonFlag  bool
		agentFlag string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "环境诊断",
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
	cmd.Flags().BoolVar(&jsonFlag, "json", false, "JSON 输出")
	cmd.Flags().StringVar(&agentFlag, "agent", "", "仅诊断指定 Agent")
	return cmd
}

func printUsage(label string, v *int64) {
	if v == nil {
		fmt.Printf("%s：未知\n", label)
		return
	}
	fmt.Printf("%s：%s\n", label, human(*v))
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
		parts = append(parts, fmt.Sprintf("总计 %s", human(*e.TotalTokens)))
	}
	if e.InputTokens != nil {
		parts = append(parts, fmt.Sprintf("输入 %s", human(*e.InputTokens)))
	}
	if e.OutputTokens != nil {
		parts = append(parts, fmt.Sprintf("输出 %s", human(*e.OutputTokens)))
	}
	if e.ContextAfter != nil {
		parts = append(parts, fmt.Sprintf("上下文 %s", human(*e.ContextAfter)))
	}
	if e.ToolName != "" {
		parts = append(parts, "工具 "+e.ToolName)
	}
	if e.UserPromptPreview != "" {
		parts = append(parts, preview(e.UserPromptPreview))
	}
	return joinParts(parts)
}

func fmtTS(t *time.Time) string {
	if t == nil {
		return "未知"
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
	default:
		for _, e := range events {
			fmt.Fprintf(w, "%s  %-18s  %s\n", fmtTS(e.Timestamp), string(e.EventType), describeEvent(e))
		}
		return nil
	}
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
			"轮次", "提问", "请求", "工具", "Token", "")
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
