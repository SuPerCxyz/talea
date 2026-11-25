package cli

import (
	"io"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/status"
)

// maxHookInputBytes 限制单次 hook 输入，避免异常调用占用内存。
const maxHookInputBytes = 1 << 20

func newHookCmd() *cobra.Command {
	var (
		agentFlag    string
		instanceFlag string
		sessionFlag  string
		stateFlag    string
		detailFlag   string
	)
	cmd := &cobra.Command{
		Use:    "hook",
		Short:  i18n.Tr("receive optional agent hook events", "接收可选的 Agent hook 事件"),
		Hidden: true,
		Args:   cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			// 该命令是 Agent hook 的旁路：任何输入/写入失败都必须静默成功，
			// 不能阻塞或中断调用它的 Agent。
			raw, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxHookInputBytes))
			if err != nil {
				return nil
			}
			event, err := status.EventFromHook(raw, time.Now())
			if err != nil {
				// 无 stdin 或 stdin 不完整时，允许完全通过参数上报。
				if agentFlag == "" || sessionFlag == "" || stateFlag == "" {
					return nil
				}
				event = status.Event{
					AgentID:   agentFlag,
					SessionID: sessionFlag,
					State:     model.TurnState(stateFlag),
					At:        time.Now(),
				}
			}
			applyHookOverrides(&event, agentFlag, instanceFlag, sessionFlag, stateFlag, detailFlag)
			if !event.State.Valid() || event.State == model.TurnStateUnknown || event.SessionID == "" {
				return nil
			}
			path := filepath.Join(config.ResolvePaths().DataDir, status.EventFileName)
			_ = status.AppendEvent(path, event)
			return nil
		},
	}
	cmd.Flags().StringVar(&agentFlag, "agent", "", i18n.Tr("agent id (optional)", "Agent ID（可选）"))
	cmd.Flags().StringVar(&instanceFlag, "instance", "", i18n.Tr("agent instance id (optional)", "Agent 实例 ID（可选）"))
	cmd.Flags().StringVar(&sessionFlag, "session", "", i18n.Tr("session id (optional)", "会话 ID（可选）"))
	cmd.Flags().StringVar(&stateFlag, "state", "", i18n.Tr("turn state (optional)", "回合状态（可选）"))
	cmd.Flags().StringVar(&detailFlag, "detail", "", i18n.Tr("short reason (optional)", "简短原因（可选）"))
	return cmd
}

// applyHookOverrides 用显式参数覆盖 stdin 事件中的同名字段。
func applyHookOverrides(event *status.Event, agent, instance, session, state, detail string) {
	if agent != "" {
		event.AgentID = string(status.CanonicalAgent(agent))
	}
	if instance != "" {
		event.AgentInstanceID = instance
	}
	if session != "" {
		event.SessionID = session
	}
	if state != "" {
		event.State = model.TurnState(state)
	}
	if detail != "" {
		event.Detail = detail
	}
}
