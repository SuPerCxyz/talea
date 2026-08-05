package adapters

import (
	"fmt"
	"sort"
	"sync"

	"github.com/talea/talea/internal/model"
)

// Registry 维护 Agent 适配器的注册与查询。
type Registry struct {
	mu       sync.RWMutex
	adapters map[model.AgentID]Adapter
}

// NewRegistry 创建空注册表。
func NewRegistry() *Registry {
	return &Registry{adapters: make(map[model.AgentID]Adapter)}
}

// Register 注册一个适配器。重复注册同一 AgentID 会返回错误。
func (r *Registry) Register(a Adapter) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	id := a.Info().ID
	if _, exists := r.adapters[id]; exists {
		return fmt.Errorf("适配器 %q 已注册", id)
	}
	r.adapters[id] = a
	return nil
}

// Get 返回指定 AgentID 的适配器。
func (r *Registry) Get(id model.AgentID) (Adapter, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	a, ok := r.adapters[id]
	return a, ok
}

// All 返回全部适配器（按 ID 排序，保证输出稳定）。
func (r *Registry) All() []Adapter {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Adapter, 0, len(r.adapters))
	for _, a := range r.adapters {
		out = append(out, a)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Info().ID < out[j].Info().ID
	})
	return out
}

// HasCapability 判断适配器是否声明某项能力。
func HasCapability(info model.AdapterInfo, cap model.Capability) bool {
	for _, c := range info.Capabilities {
		if c == cap {
			return true
		}
	}
	return false
}

// As 将适配器断言为可选能力接口，失败时返回 (nil, false)。
func As[T any](a Adapter) (T, bool) {
	v, ok := a.(T)
	return v, ok
}
