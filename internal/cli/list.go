package cli

import (
	"fmt"
	"os"
	"sort"

	"github.com/talea/talea/internal/i18n"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/cli/output"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

func newListCmd() *cobra.Command {
	var (
		agentFlag        string
		cwdFlag          string
		projectFlag      string
		branchFlag       string
		todayFlag        bool
		activeFlag       bool
		includeSubagents bool
		sortFlag         string
		limitFlag        int
		formatFlag       string
	)
	cmd := &cobra.Command{
		Use:   "list",
		Short: i18n.Tr("list sessions", "列出会话"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
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
			if err := autoIndex(ctx, a, db); err != nil {
				return err
			}

			q := search.Query{
				Agent:   agentFlag,
				Cwd:     cwdFlag,
				Project: projectFlag,
				Branch:  branchFlag,
				Limit:   limitFlag,
			}
			if todayFlag {
				q.SinceDays = 1
			}
			results, err := search.List(ctx, db, q)
			if err != nil {
				return err
			}
			var sessions []*model.Session
			for i := range results {
				sessions = append(sessions, &results[i].Session)
			}
		effectiveSort := sortFlag
		if effectiveSort == "" {
			effectiveSort = a.Config.General.DefaultSort
		}
		if effectiveSort == "" || effectiveSort == "last_activity" {
			// 默认按会话结束时间降序（与 talea go 一致）
			sort.SliceStable(sessions, func(i, j int) bool {
				return endTs(sessions[i]) > endTs(sessions[j])
			})
		} else {
			a.Config.General.DefaultSort = effectiveSort
			a.SortSessions(sessions)
		}
			if !includeSubagents {
				filtered := sessions[:0]
				for _, s := range sessions {
					if !s.IsSubagent {
						filtered = append(filtered, s)
					}
				}
				sessions = filtered
			}
			if activeFlag {
				filtered := sessions[:0]
				for _, s := range sessions {
					if s.Activity == model.ActivityActive || s.Activity == model.ActivityPossiblyActive {
						filtered = append(filtered, s)
					}
				}
				sessions = filtered
			}
			if len(sessions) == 0 {
				return nil
			}
			return output.Write(os.Stdout, sessions, output.Format(formatFlag))
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("filter by agent", "按 Agent 过滤"))
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", i18n.Tr("filter by working directory prefix", "按工作目录前缀过滤"))
	cmd.Flags().StringVar(&projectFlag, "project", "", i18n.Tr("filter by project name", "按项目名称过滤"))
	cmd.Flags().StringVar(&branchFlag, "branch", "", i18n.Tr("filter by git branch", "按 Git 分支过滤"))
	cmd.Flags().BoolVar(&todayFlag, "today", false, i18n.Tr("today only", "仅今天"))
	cmd.Flags().BoolVar(&activeFlag, "active", false, i18n.Tr("active sessions only", "仅活动会话"))
	cmd.Flags().BoolVar(&includeSubagents, "include-subagents", false, i18n.Tr("include sub-agent sessions", "包含子 Agent 会话"))
	cmd.Flags().StringVar(&sortFlag, "sort", "", i18n.Tr("sort: last_activity/started_at/tokens/name", "排序：last_activity/started_at/tokens/name"))
	cmd.Flags().IntVar(&limitFlag, "limit", 30, i18n.Tr("max items (default recent 30)", "最多条数（默认最近 30 个）"))
	cmd.Flags().StringVar(&formatFlag, "format", "table", i18n.Tr("output format: table/json/jsonl/csv/markdown", "输出格式：table/json/jsonl/csv/markdown"))
	return cmd
}

func newSearchCmd() *cobra.Command {
	var (
		agentFlag   string
		cwdFlag     string
		projectFlag string
		branchFlag  string
		sinceFlag   int
		limitFlag   int
		formatFlag  string
	)
	cmd := &cobra.Command{
		Use:   "search [keyword]",
		Short: i18n.Tr("full-text search across agents", "跨 Agent 全文搜索"),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
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
			if err := autoIndex(ctx, a, db); err != nil {
				return err
			}

			q := search.Query{
				Term:      args[0],
				Agent:     agentFlag,
				Cwd:       cwdFlag,
				Project:   projectFlag,
				Branch:    branchFlag,
				SinceDays: sinceFlag,
				Limit:     limitFlag,
			}
			results, err := search.Search(ctx, db, q)
			if err != nil {
				return err
			}
			var sessions []*model.Session
			for i := range results {
				sessions = append(sessions, &results[i].Session)
			}
			if len(sessions) == 0 {
				return nil
			}
			return output.Write(os.Stdout, sessions, output.Format(formatFlag))
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("filter by agent", "按 Agent 过滤"))
	cmd.Flags().StringVar(&cwdFlag, "cwd", "", i18n.Tr("filter by working directory prefix", "按工作目录前缀过滤"))
	cmd.Flags().StringVar(&projectFlag, "project", "", i18n.Tr("filter by project name", "按项目名称过滤"))
	cmd.Flags().StringVar(&branchFlag, "branch", "", i18n.Tr("filter by git branch", "按 Git 分支过滤"))
	cmd.Flags().IntVar(&sinceFlag, "since", 0, i18n.Tr("recent N days", "最近 N 天"))
	cmd.Flags().IntVar(&limitFlag, "limit", 0, i18n.Tr("max items", "最多条数"))
	cmd.Flags().StringVar(&formatFlag, "format", "table", i18n.Tr("output format: table/json/jsonl/csv/markdown", "输出格式：table/json/jsonl/csv/markdown"))
	return cmd
}

func newIndexCmd() *cobra.Command {
	var (
		rebuildFlag  bool
		metadataOnly bool
	)
	cmd := &cobra.Command{
		Use:   "index",
		Short: i18n.Tr("incrementally index agent sessions", "增量索引 Agent 会话"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			if metadataOnly {
				a.Config.Index.MetadataOnly = true
			}
			db, err := index.Open(a.Paths.DBPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if err := db.Migrate(ctx); err != nil {
				return err
			}
			results, err := (&index.Indexer{App: a, DB: db, Force: rebuildFlag}).Run(ctx)
			if err != nil {
				return err
			}
			// 同步全文索引
			if err := search.Ensure(ctx, db); err != nil {
				return err
			}
			if rebuildFlag {
				if err := search.Rebuild(ctx, db); err != nil {
					return err
				}
			} else if err := search.Populate(ctx, db); err != nil {
				return err
			}
			if !metadataOnly {
				ix := &index.Indexer{App: a, DB: db}
				if n, err := ix.ResolveSubagentRelations(ctx); err == nil && n > 0 {
					fmt.Fprintf(os.Stdout, i18n.Tr("sub-agent aggregation: %d relations\n", "子 Agent 聚合：%d 条关系\n"), n)
				}
				if n, err := ix.RefreshActivities(ctx); err == nil && n > 0 {
					fmt.Fprintf(os.Stdout, i18n.Tr("activity detection: %d sessions\n", "活动状态检测：%d 个会话\n"), n)
				}
			}
			for _, r := range results {
				status := "OK"
				if r.Errors > 0 {
					status = fmt.Sprintf("ERROR %d", r.Errors)
				}
				fmt.Fprintf(os.Stdout, i18n.Tr("%s: added %d, updated %d, skipped %d, %s\n", "%s：新增 %d，更新 %d，跳过 %d，%s\n"),
					r.AgentID, r.Added, r.Updated, r.Skipped, status)
				for _, m := range r.ErrorMsgs {
					fmt.Fprintf(os.Stderr, "  - %s\n", m)
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&rebuildFlag, "rebuild", false, i18n.Tr("full rebuild index", "全量重建索引"))
	cmd.Flags().BoolVar(&metadataOnly, "metadata-only", false, i18n.Tr("metadata only", "仅索引元数据"))
	return cmd
}

func newConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: i18n.Tr("configuration management", "配置管理"),
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "path",
		Short: i18n.Tr("show config path", "显示配置路径"),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := config.ResolvePaths()
			fmt.Println(p.ConfigPath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "init",
		Short: i18n.Tr("generate default config file", "生成默认配置文件"),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := config.ResolvePaths()
			if _, err := os.Stat(p.ConfigPath); err == nil {
				return fmt.Errorf(i18n.Tr("config already exists: %s", "配置已存在：%s"), p.ConfigPath)
			}
			if err := os.MkdirAll(p.ConfigDir, 0o700); err != nil {
				return err
			}
			if err := os.WriteFile(p.ConfigPath, []byte(defaultConfigTOML()), 0o600); err != nil {
				return err
			}
			fmt.Printf(i18n.Tr("config written: %s\n", "已生成配置：%s\n"), p.ConfigPath)
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "validate",
		Short: i18n.Tr("validate config", "校验配置"),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := config.ResolvePaths()
			cfg, err := config.Load(p.ConfigPath)
			if err != nil {
				return err
			}
			_ = cfg
			fmt.Println(i18n.Tr("config valid:", "配置有效："), p.ConfigPath)
			return nil
		},
	})
	return cmd
}

func defaultConfigTOML() string {
	return `[general]
default_sort = "last_activity"
include_subagents = false
show_system_messages = false
time_format = "2006-01-02 15:04"
timezone = "local"

[index]
metadata_only = false
index_assistant_messages = true
index_tool_output = false
max_message_bytes = 1048576
max_tool_output_bytes = 262144

[agents.claude-code]
enabled = true
data_dirs = []

[agents.codex-cli]
enabled = true
data_dirs = []

[agents.opencode]
enabled = true
data_dirs = []

[search]
max_results = 200
preview_message_limit = 20

[usage]
enabled = true
store_request_details = true
estimate_cost = false
include_subagents_by_default = false

[privacy]
redact_secrets_in_preview = true
redact_secrets_in_export = false
`
}
