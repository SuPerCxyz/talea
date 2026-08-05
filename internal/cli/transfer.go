package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/transfer"
)

func newExportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "export <文件>",
		Short: "导出全部会话到 JSON（含标签/备注）",
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
			if err := transfer.Export(ctx, db, args[0]); err != nil {
				return err
			}
			fmt.Println("已导出：", args[0])
			return nil
		},
	}
	return cmd
}

func newImportCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "import <文件>",
		Short: "从 JSON 导入会话（已存在跳过）",
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
			n, err := transfer.Import(ctx, db, args[0])
			if err != nil {
				return err
			}
			fmt.Printf("已导入 %d 个会话\n", n)
			return nil
		},
	}
	return cmd
}
