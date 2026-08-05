package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/doctor"
	"github.com/talea/talea/internal/index"
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
		agentFlag  string
		groupBy    string
		bucket     string
		aroundPeak bool
		formatFlag string
		outputFile string
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
			fmt.Printf("会话：%s  请求：%d  总计：%s  上下文峰值：%s\n",
				sess.SessionID, summary.RequestCount, human(summary.TotalTokens), human(summary.PeakContext))
			fmt.Println()

			switch groupBy {
			case "turn", "request", "":
				for _, e := range events {
					fmt.Printf("%s  %-18s  %s\n", fmtTS(e.Timestamp), string(e.EventType), describeEvent(e))
				}
			default:
				return fmt.Errorf("不支持的 group-by：%s", groupBy)
			}
			_ = bucket
			_ = aroundPeak
			_ = formatFlag
			_ = outputFile
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().StringVar(&groupBy, "group-by", "", "聚合维度：turn/request")
	cmd.Flags().StringVar(&bucket, "bucket", "", "时间桶：1m/5m/15m/1h")
	cmd.Flags().BoolVar(&aroundPeak, "around-peak", false, "峰值附近")
	cmd.Flags().StringVar(&formatFlag, "format", "table", "输出格式：table/json/csv/markdown")
	cmd.Flags().StringVar(&outputFile, "output", "", "输出文件")
	return cmd
}

func newDoctorCmd() *cobra.Command {
	var (
		jsonFlag   bool
		agentFlag  string
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

