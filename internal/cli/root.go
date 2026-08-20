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
	var (
		pathFlag  string
		agentFlag string
	)
	root := &cobra.Command{
		Use:     "talea",
		Short:   "Trace the session. Resume the work.",
		Long:    i18n.Tr("Talea — a local-first AI coding agent session index, search, token analysis and resume tool.", "Talea — 本地优先的 AI Coding Agent 会话索引、搜索、Token 分析与恢复工具。"),
		Version: version.String(),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 无子命令时打开 TUI
			return tui.Run(cmd.Context(), pathFlag, agentFlag)
		},
	}
	root.SetVersionTemplate("{{.Version}}\n")
	root.Flags().StringVarP(&pathFlag, "path", "p", "", i18n.Tr("filter sessions in this exact directory (TUI)", "仅列出该目录下的会话（精确匹配，不含子目录）"))
	root.Flags().StringVarP(&agentFlag, "agent", "a", "", i18n.Tr("filter sessions by agent (TUI)", "按 Agent 过滤会话（TUI）"))
	root.AddCommand(newListCmd())
	root.AddCommand(newGoCmd())
	root.AddCommand(newIndexCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newConfigCmd())
	root.AddCommand(newWebCmd())
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
