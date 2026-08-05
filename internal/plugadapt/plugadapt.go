// Package plugadapt 将外部适配器客户端包装为内部 Adapter。
package plugadapt

import (
	"context"
	"sync"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/plugin"
)

// Adapter 包装一个外部适配器进程。
type Adapter struct {
	mu     sync.Mutex
	client *plugin.Client
	info   model.AdapterInfo
}

// New 创建外部适配器包装。
func New(path string) (*Adapter, error) {
	client := plugin.NewClient(path)
	if err := client.Start(context.Background()); err != nil {
		return nil, err
	}
	info, err := client.Info()
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	if info.ID == "" {
		_ = client.Close()
		return nil, &adapters.ErrInvalidPlugin{Path: path}
	}
	return &Adapter{client: client, info: info}, nil
}

// Info 返回适配器信息。
func (a *Adapter) Info() model.AdapterInfo { return a.info }

// Detect 探测实例。
func (a *Adapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client.Detect()
}

// Discover 发现会话。
func (a *Adapter) Discover(ctx context.Context, inst model.AgentInstance) ([]adapters.SessionSource, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client.Discover(inst)
}

// ParseMetadata 解析元数据（外部协议当前支持 detect/discover/info，
// 元数据解析由 index 层回退到 Discover 的 SourceID 推断）。
func (a *Adapter) ParseMetadata(
	ctx context.Context,
	inst model.AgentInstance,
	src adapters.SessionSource,
) (*model.Session, error) {
	// 最小可用实现：由 Discover 提供的字段构造 Session
	return &model.Session{
		AgentID:         a.info.ID,
		AgentInstanceID: inst.InstanceID,
		SessionID:       src.SessionID,
		SourcePath:      src.Path,
		SourceID:        src.SourceID,
		SourceMtime:     src.Mtime,
		SourceSize:      src.Size,
	}, nil
}

// Close 关闭进程。
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client.Close()
}
