package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/doctor"
	"github.com/talea/talea/internal/i18n"
)

func newDoctorCmd() *cobra.Command {
	var (
		jsonFlag  bool
		agentFlag string
	)
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: i18n.Tr("environment diagnostics", "环境诊断"),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()
			a, err := app.New(ctx)
			if err != nil {
				return err
			}
			rep, err := doctor.Run(ctx, a, agentFlag)
			if err != nil {
				return err
			}
			if jsonFlag {
				data, _ := rep.JSON()
				fmt.Println(string(data))
				return nil
			}
			fmt.Println("Talea Doctor")
			rep.Print()
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonFlag, "json", false, i18n.Tr("JSON output", "JSON 输出"))
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("diagnose only the given agent", "仅诊断指定 Agent"))
	return cmd
}
