package index

import (
	"context"
	"errors"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
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
