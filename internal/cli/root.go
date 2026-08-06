// Package cli 实现 talea 的命令行界面。
package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/tui"
	"github.com/talea/talea/internal/version"
)

// Exit 码约定。
const (
	ExitOK             = 0
	ExitError          = 1
	ExitUsage          = 2
	ExitNotFound       = 3
	ExitAgentMissing   = 4
	ExitNoWorkdir      = 5
	ExitFormatUnsup    = 6
	ExitIndexCorrupt   = 7
	ExitCapMissing     = 8
	ExitDataIncomplete = 9
)

// Execute 运行 CLI，返回退出码。
func Execute() int {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		// 识别带退出码的错误（exitError 值或指针），否则统一 ExitError
		switch e := err.(type) {
		case exitError:
			fmt.Fprintln(os.Stderr, "talea:", e.msg)
			return e.code
		case *exitError:
			fmt.Fprintln(os.Stderr, "talea:", e.msg)
			return e.code
		}
		fmt.Fprintln(os.Stderr, "talea:", err)
		return ExitError
	}
	return ExitOK
}

// NewRootCmd 构建根命令。
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "talea",
		Short:   "Trace the session. Resume the work.",
		Long:    i18n.Tr("Talea — a local-first AI coding agent session index, search, preview, token analysis and resume tool.", "Talea — 本地优先的 AI Coding Agent 会话索引、搜索、预览、Token 分析与恢复工具。"),
		Version: version.String(),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 无子命令时打开 TUI
			return tui.Run(cmd.Context())
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.AddCommand(newListCmd())
	root.AddCommand(newSearchCmd())
	root.AddCommand(newGoCmd())
	root.AddCommand(newLastCmd())
	root.AddCommand(newIndexCmd())
	root.AddCommand(newUsageCmd())
	root.AddCommand(newTimelineCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newRunCmd())
	root.AddCommand(newPreviewCmd())
	root.AddCommand(newTagCmd())
	root.AddCommand(newWebCmd())
	root.AddCommand(newWatchCmd())
	root.AddCommand(newExportCmd())
	root.AddCommand(newImportCmd())
	root.AddCommand(&cobra.Command{
		Use:   "version",
		Short: i18n.Tr("version info", "版本信息"),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println(version.String())
			return nil
		},
	})
	return root
}
