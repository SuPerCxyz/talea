package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	previewpkg "github.com/talea/talea/internal/preview"
)

func newPreviewCmd() *cobra.Command {
	var (
		agentFlag  string
		limitFlag  int
		systemFlag bool
		tailFlag   bool
	)
	cmd := &cobra.Command{
		Use:   "preview <session-id>",
		Short: "对话预览",
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
			ad, ok := a.Registry.Get(sess.AgentID)
			if !ok {
				return exitError{code: ExitFormatUnsup, msg: "会话格式不支持"}
			}
			// tail 模式取末尾，默认取开头
			msgs, err := previewpkg.Load(ctx, ad, *sess, previewpkg.Options{
				Limit:      limitFlag,
				ShowSystem: systemFlag,
				Redact:     a.Config.Privacy.RedactSecretsInPreview,
				Head:       !tailFlag,
			})
			if err != nil {
				return err
			}
			for _, m := range msgs {
				fmt.Printf("[%s] %s\n", m.Role, m.Timestamp)
				fmt.Println(m.Content)
				fmt.Println()
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")
	cmd.Flags().IntVar(&limitFlag, "limit", 20, "消息条数")
	cmd.Flags().BoolVar(&systemFlag, "system", false, "包含系统消息")
	cmd.Flags().BoolVar(&tailFlag, "tail", false, "查看最后几条（默认查看开头）")
	return cmd
}
