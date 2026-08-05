// Package preview 提供对话预览的加载与渲染。
package preview

import (
	"context"
	"fmt"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/security"
)

// Options 控制预览加载。
type Options struct {
	Limit      int  // 最多消息数
	ShowSystem bool // 是否显示系统消息
	Redact     bool // 是否遮罩敏感信息
}

// MessageView 是渲染用消息单元。
type MessageView struct {
	Role      string
	Timestamp string
	Content   string
	IsSystem  bool
}

// Load 通过适配器加载会话消息预览。
func Load(ctx context.Context, a adapters.Adapter, s model.Session, opts Options) ([]MessageView, error) {
	loader, ok := adapters.As[adapters.MessageLoader](a)
	if !ok {
		return nil, fmt.Errorf("agent %s 不支持消息预览", s.AgentID)
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	it, err := loader.LoadMessages(ctx, s, adapters.MessageLoadOptions{
		Limit:      limit,
		ShowSystem: opts.ShowSystem,
	})
	if err != nil {
		return nil, err
	}
	defer it.Close()

	var out []MessageView
	for {
		m, ok, err := it.Next()
		if err != nil {
			return out, err
		}
		if !ok {
			break
		}
		content := security.StripANSI(m.Content)
		if opts.Redact {
			content = security.RedactSecrets(content)
		}
		out = append(out, MessageView{
			Role:      roleText(m.Role),
			Timestamp: fmtTime(m.Timestamp),
			Content:   content,
			IsSystem:  m.IsSystem,
		})
	}
	return out, nil
}

func roleText(role string) string {
	switch role {
	case "user":
		return "用户"
	case "assistant":
		return "助手"
	case "system":
		return "系统"
	default:
		return role
	}
}

func fmtTime(sec int64) string {
	if sec == 0 {
		return ""
	}
	return time.Unix(sec, 0).Format("15:04:05")
}
