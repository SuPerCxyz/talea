package cli

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/watch"
)

func newWatchCmd() *cobra.Command {
	var intervalFlag int
	cmd := &cobra.Command{
		Use:   "watch",
		Short: i18n.Tr("watch agent data dirs and index on change", "监听 Agent 数据目录，变化时增量索引"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
			defer stop()

			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			return watch.Run(ctx, a, watch.Options{
				Interval: time.Duration(intervalFlag) * time.Second,
			})
		},
	}
	cmd.Flags().IntVar(&intervalFlag, "interval", 2, i18n.Tr("event merge window (seconds)", "事件合并窗口（秒）"))
	return cmd
}
