package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/web"
)

func newWebCmd() *cobra.Command {
	var portFlag int
	cmd := &cobra.Command{
		Use:   "web",
		Short: i18n.Tr("local read-only web view (localhost only)", "本地只读 Web 视图（仅 localhost）"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()

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

			srv := &web.Server{App: a, DB: db}
			ln, err := srv.Listen(ctx, portFlag)
			if err != nil {
				return err
			}
			addr := ln.Addr().String()
			fmt.Printf(i18n.Tr("Talea read-only view: http://%s (localhost only, Ctrl+C to quit)\n", "Talea 只读视图：http://%s （仅本机访问，Ctrl+C 退出）\n"), addr)
			httpSrv := &http.Server{Handler: srv.Handler()}
			errCh := make(chan error, 1)
			go func() { errCh <- httpSrv.Serve(ln) }()
			select {
			case <-ctx.Done():
				_ = httpSrv.Close()
				fmt.Println(i18n.Tr("\nstopped", "\n已停止"))
				return nil
			case err := <-errCh:
				if err != nil && err != http.ErrServerClosed {
					return err
				}
				return nil
			}
		},
	}
	cmd.Flags().IntVar(&portFlag, "port", 7690, i18n.Tr("listen port (127.0.0.1 only)", "监听端口（仅 127.0.0.1）"))
	return cmd
}
