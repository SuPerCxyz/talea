package cli

import (
	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/tui"
)

func newDashboardCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "dashboard",
		Short: i18n.Tr("open the live agent activity dashboard", "打开常驻 Agent 活动看板"),
		RunE: func(cmd *cobra.Command, args []string) error {
			return tui.RunDashboard(cmd.Context())
		},
	}
}
