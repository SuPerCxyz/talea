package doctor

import (
	"context"
	"fmt"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// 存储形态识别结果，与 OpenCode 适配器 StorageShape 的返回约定一致。
const (
	shapeSessionV2 = "session_v2"
	shapeLegacy    = "session"
)

// storageProber 是可选能力：报告 Agent 本地数据库的存储形态。
// Go 接口为结构化满足，未实现该能力的适配器会被静默跳过。
type storageProber interface {
	StorageShape(ctx context.Context, inst model.AgentInstance) (string, error)
}

// addStorageShape 在会话发现行之后追加存储形态行。
// 会话数复用已发现的 sources 长度，不额外查询数据库；
// adapter 未实现 storageProber 时不产生任何输出。
func addStorageShape(ctx context.Context, rep *Report, ad adapters.Adapter,
	agentName string, inst model.AgentInstance, sessionCount int) {
	p, ok := ad.(storageProber)
	if !ok {
		return
	}
	label := fmt.Sprintf("%s 存储形态", agentName)
	shape, err := p.StorageShape(ctx, inst)
	if err != nil {
		rep.addWarn(label, fmt.Sprintf("无法确定存储形态：%v", err))
		return
	}
	switch shape {
	case shapeSessionV2:
		rep.addOK(label, fmt.Sprintf("session_v2 主表（%d 个会话）", sessionCount))
	case shapeLegacy:
		rep.addOK(label, fmt.Sprintf("传统 session 表（%d 个会话）", sessionCount))
	default:
		// 未预期的返回值：不得报告成功
		rep.addWarn(label, fmt.Sprintf("无法确定存储形态：未知形态 %q", shape))
	}
}
