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

// ID 返回适配器标识。
func (a *Adapter) ID() model.AgentID { return a.info.ID }

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

// ParseMetadata 解析会话元数据（完整字段，走插件 parse 方法）。
// 若插件不支持 parse 方法，回退到 Discover 提供的基础字段。
func (a *Adapter) ParseMetadata(
	ctx context.Context,
	inst model.AgentInstance,
	src adapters.SessionSource,
) (*model.Session, error) {
	a.mu.Lock()
	s, err := a.client.ParseMetadata(inst, src)
	a.mu.Unlock()
	if err != nil {
		// 插件不支持 parse 时回退基础字段
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
	return s, nil
}

// LoadMessages 读取消息预览（走插件 messages 方法）。
func (a *Adapter) LoadMessages(
	ctx context.Context,
	s model.Session,
	opts adapters.MessageLoadOptions,
) (adapters.MessageIterator, error) {
	a.mu.Lock()
	msgs, err := a.client.LoadMessagesFull(s, opts)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &msgIterator{msgs: msgs}, nil
}

// LoadUsage 读取会话 Token 汇总（走插件 usage 方法）。
func (a *Adapter) LoadUsage(ctx context.Context, s model.Session) (*model.TokenUsage, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client.LoadUsage(s)
}

// IterateUsageEvents 读取时间线事件（走插件 timeline 方法）。
func (a *Adapter) IterateUsageEvents(ctx context.Context, s model.Session) (adapters.UsageEventIterator, error) {
	a.mu.Lock()
	events, err := a.client.IterateUsageEvents(s)
	a.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return &eventIterator{events: events}, nil
}

type msgIterator struct {
	msgs []adapters.Message
	idx  int
}

func (it *msgIterator) Next() (adapters.Message, bool, error) {
	if it.idx >= len(it.msgs) {
		return adapters.Message{}, false, nil
	}
	m := it.msgs[it.idx]
	it.idx++
	return m, true, nil
}

func (it *msgIterator) Close() error { return nil }

type eventIterator struct {
	events []*model.UsageTimelineEvent
	idx    int
}

func (it *eventIterator) Next() (*model.UsageTimelineEvent, bool, error) {
	if it.idx >= len(it.events) {
		return nil, false, nil
	}
	e := it.events[it.idx]
	it.idx++
	return e, true, nil
}

func (it *eventIterator) Close() error { return nil }

// Close 关闭进程。
func (a *Adapter) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.client.Close()
}
