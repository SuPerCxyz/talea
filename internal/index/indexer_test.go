package index

import (
	"context"
	"errors"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/model"
)

func TestIndexCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	db := newTestDB(t)
	cancel()
	_, err := (&Indexer{App: &app.App{Registry: adapters.NewRegistry()}, DB: db}).Run(ctx)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
}

type fallbackDiscoveryAdapter struct {
	source adapters.SessionSource
}

func (a *fallbackDiscoveryAdapter) ID() model.AgentID { return "fallback" }

func (a *fallbackDiscoveryAdapter) Info() model.AdapterInfo {
	return model.AdapterInfo{ID: a.ID()}
}

func (a *fallbackDiscoveryAdapter) Detect(context.Context) ([]model.AgentInstance, error) {
	return nil, nil
}

func (a *fallbackDiscoveryAdapter) Discover(context.Context, model.AgentInstance) ([]adapters.SessionSource, error) {
	return []adapters.SessionSource{a.source}, nil
}

func (a *fallbackDiscoveryAdapter) ParseMetadata(context.Context, model.AgentInstance, adapters.SessionSource) (*model.Session, error) {
	return nil, nil
}

func (a *fallbackDiscoveryAdapter) DiscoverIncremental(
	context.Context, model.AgentInstance, adapters.DiscoveryState,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	return nil, adapters.DiscoveryState{}, errors.New("cursor unavailable")
}

func (a *fallbackDiscoveryAdapter) CursorFromSources(model.AgentInstance, []adapters.SessionSource) string {
	return "fallback-cursor"
}

func TestDiscoverSourcesFallsBackToCompleteDiscovery(t *testing.T) {
	adapter := &fallbackDiscoveryAdapter{source: adapters.SessionSource{SessionID: "s"}}
	sources, state, err := discoverSources(context.Background(), adapter,
		model.AgentInstance{InstanceID: "i"}, adapters.DiscoveryState{Cursor: "old"})
	if err != nil || len(sources) != 1 || state.Cursor != "fallback-cursor" {
		t.Fatalf("sources=%v state=%+v err=%v", sources, state, err)
	}
}
