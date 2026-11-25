package cli

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/status"
	"github.com/talea/talea/internal/syncer"
)

func newStatusCmd() *cobra.Command {
	var (
		jsonOut bool
		short   bool
	)
	cmd := &cobra.Command{
		Use:   "status",
		Short: i18n.Tr("show current agent turn status", "查看当前 Agent 回合状态"),
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
			// 同步失败不阻止读取旧索引；错误作为快照警告输出。
			report, syncErr := syncer.SyncWithReport(ctx, a, db, nil)
			snap, err := status.Build(ctx, db, status.Options{
				EventsPath:       filepath.Join(a.Paths.DataDir, status.EventFileName),
				IncludeSubagents: a.Config.General.IncludeSubagents,
			})
			if err != nil {
				return err
			}
			if syncErr != nil {
				snap.Warnings = append(snap.Warnings, "sync: "+syncErr.Error())
			}
			snap.Warnings = append(snap.Warnings, report.Warnings()...)

			out := cmd.OutOrStdout()
			switch {
			case jsonOut:
				enc := json.NewEncoder(out)
				enc.SetIndent("", "  ")
				return enc.Encode(snap)
			case short:
				fmt.Fprintln(out, snap.Short())
			default:
				fmt.Fprint(out, snap.Text())
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, i18n.Tr("stable JSON output", "稳定的 JSON 输出"))
	cmd.Flags().BoolVar(&short, "short", false, i18n.Tr("single-line summary for status bars", "状态栏单行汇总"))
	return cmd
}
