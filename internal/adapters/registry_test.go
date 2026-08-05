package adapters

import (
	"context"
	"testing"

	"github.com/talea/talea/internal/model"
)

type fakeAdapter struct {
	id model.AgentID
}

func (f *fakeAdapter) Info() model.AdapterInfo {
	return model.AdapterInfo{
		ID:           f.id,
		DisplayName:  string(f.id),
		Capabilities: []model.Capability{model.CapabilityDiscoverSessions},
	}
}

func (f *fakeAdapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	return nil, nil
}

func (f *fakeAdapter) Discover(ctx context.Context, inst model.AgentInstance) ([]SessionSource, error) {
	return nil, nil
}

func (f *fakeAdapter) ParseMetadata(ctx context.Context, inst model.AgentInstance, src SessionSource) (*model.Session, error) {
	return nil, nil
}

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	f1 := &fakeAdapter{id: "agent-a"}
	if err := r.Register(f1); err != nil {
		t.Fatal(err)
	}
	got, ok := r.Get("agent-a")
	if !ok {
		t.Fatal("expected to find agent-a")
	}
	if got.Info().ID != "agent-a" {
		t.Fatalf("got %q", got.Info().ID)
	}
	if _, ok := r.Get("agent-b"); ok {
		t.Fatal("should not find agent-b")
	}
}

func TestRegistryDuplicateRejected(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeAdapter{id: "agent-a"})
	if err := r.Register(&fakeAdapter{id: "agent-a"}); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}

func TestRegistryAllSorted(t *testing.T) {
	r := NewRegistry()
	r.Register(&fakeAdapter{id: "z-agent"})
	r.Register(&fakeAdapter{id: "a-agent"})
	all := r.All()
	if len(all) != 2 {
		t.Fatalf("len: %d", len(all))
	}
	if all[0].Info().ID != "a-agent" {
		t.Fatalf("first: %q", all[0].Info().ID)
	}
}

func TestHasCapability(t *testing.T) {
	info := model.AdapterInfo{
		Capabilities: []model.Capability{model.CapabilityResume},
	}
	if !HasCapability(info, model.CapabilityResume) {
		t.Fatal("expected resume capability")
	}
	if HasCapability(info, model.CapabilityTokenSummary) {
		t.Fatal("did not expect token summary")
	}
}

func TestAs(t *testing.T) {
	type fakeResumer interface {
		BuildResumeCommand() string
	}
	// fakeAdapter 不实现 fakeResumer
	if v, ok := As[fakeResumer](&fakeAdapter{id: "x"}); ok {
		t.Fatalf("unexpected success: %v", v)
	}
	// 反向验证：实现了 fakeResumer 的 Adapter 应被识别
	r := &resumeCapableAdapter{}
	if _, ok := As[fakeResumer](r); !ok {
		t.Fatal("expected type assertion success")
	}
}

type resumeCapableAdapter struct{}

func (r *resumeCapableAdapter) BuildResumeCommand() string { return "resume" }
func (r *resumeCapableAdapter) Info() model.AdapterInfo {
	return model.AdapterInfo{ID: "resume-capable"}
}
func (r *resumeCapableAdapter) Detect(ctx context.Context) ([]model.AgentInstance, error) {
	return nil, nil
}
func (r *resumeCapableAdapter) Discover(ctx context.Context, inst model.AgentInstance) ([]SessionSource, error) {
	return nil, nil
}
func (r *resumeCapableAdapter) ParseMetadata(ctx context.Context, inst model.AgentInstance, src SessionSource) (*model.Session, error) {
	return nil, nil
}
