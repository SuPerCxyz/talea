package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/tags"
)

func newTagCmd() *cobra.Command {
	var agentFlag string
	cmd := &cobra.Command{
		Use:   "tag <session-id> [标签...]",
		Short: "会话标签 / 收藏 / 备注",
		Args:  cobra.ArbitraryArgs,
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
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			if len(args) > 1 {
				// 设置标签
				if err := tags.SetTags(ctx, db, sess.AgentInstanceID, sess.SessionID, strings.Join(args[1:], ",")); err != nil {
					return err
				}
				fmt.Println("标签已更新")
				return nil
			}
			// 查看
			m, err := tags.Get(ctx, db, sess.AgentInstanceID, sess.SessionID)
			if err != nil {
				return err
			}
			fmt.Printf("会话：%s\n", sess.SessionID)
			if m.Favorite {
				fmt.Println("收藏：是")
			} else {
				fmt.Println("收藏：否")
			}
			if len(m.Tags) > 0 {
				fmt.Printf("标签：%s\n", strings.Join(m.Tags, ", "))
			} else {
				fmt.Println("标签：无")
			}
			if m.Note != "" {
				fmt.Printf("备注：%s\n", m.Note)
			} else {
				fmt.Println("备注：无")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", "Agent 标识")

	cmd.AddCommand(&cobra.Command{
		Use:   "list [标签]",
		Short: "列出收藏或指定标签的会话",
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
			var refs []tags.SessionRef
			if len(args) > 0 {
				refs, err = tags.ByTag(ctx, db, args[0])
			} else {
				refs, err = tags.Favorites(ctx, db)
			}
			if err != nil {
				return err
			}
			if len(refs) == 0 {
				fmt.Println("没有匹配的会话")
				return nil
			}
			for _, r := range refs {
				fmt.Printf("%s  %s\n", r.AgentInstanceID, r.SessionID)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "favorite <session-id> [on|off]",
		Short: "设置收藏",
		Args:  cobra.MinimumNArgs(1),
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
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			on := len(args) <= 1 || args[1] != "off"
			if err := tags.SetFavorite(ctx, db, sess.AgentInstanceID, sess.SessionID, on); err != nil {
				return err
			}
			fmt.Println("收藏已更新")
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "note <session-id> [文本]",
		Short: "设置或清除备注",
		Args:  cobra.MinimumNArgs(1),
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
			sess, err := findSession(ctx, a, args[0], agentFlag)
			if err != nil {
				return err
			}
			note := ""
			if len(args) > 1 {
				note = strings.Join(args[1:], " ")
			}
			if err := tags.SetNote(ctx, db, sess.AgentInstanceID, sess.SessionID, note); err != nil {
				return err
			}
			fmt.Println("备注已更新")
			return nil
		},
	})
	return cmd
}
