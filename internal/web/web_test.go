package web

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/talea/talea/internal/index"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	db, err := index.Open(filepath.Join(t.TempDir(), "index.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	return &Server{DB: db}
}

func TestSessionsEndpoint(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/sessions", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	var items []sessionListItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	// 空库返回空数组
	if len(items) != 0 {
		t.Fatalf("items=%d", len(items))
	}
}

func TestSessionEndpointMissingID(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/api/session", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 400 {
		t.Fatalf("code=%d, want 400", rec.Code)
	}
}

func TestIndexPage(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("code=%d", rec.Code)
	}
	if len(rec.Body.Bytes()) < 100 {
		t.Fatal("page too small")
	}
}
