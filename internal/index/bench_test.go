package index

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

// BenchmarkUpsertSessions 基准：批量写入会话。
func BenchmarkUpsertSessions(b *testing.B) {
	db, err := Open(filepath.Join(b.TempDir(), "index.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		b.Fatal(err)
	}
	now := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := &model.Session{
			AgentID:          model.AgentClaudeCode,
			AgentInstanceID:  "bench",
			SessionID:        "bench-" + time.Now().Format("150405.000000000"),
			FirstQuestion:    "基准问题",
			WorkingDirectory: "/bench",
			IndexedAt:        now,
			UpdatedAt:        now,
		}
		if err := db.UpsertSession(context.Background(), s); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkTimelineInsert 基准：插入时间线事件。
func BenchmarkTimelineInsert(b *testing.B) {
	db, err := Open(filepath.Join(b.TempDir(), "index.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		b.Fatal(err)
	}
	base := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ts := base.Add(time.Duration(i) * time.Millisecond)
		e := &model.UsageTimelineEvent{
			AgentInstanceID: "bench",
			SessionID:       "s1",
			EventType:       model.UsageEventRequest,
			Timestamp:       &ts,
			Sequence:        int64(i),
			TotalTokens:     int64Ptr(int64(i * 100)),
			SourceIdentity:  "ev-" + time.Now().Format("150405.000000000") + "-" + itoa(i),
		}
		if _, err := db.UpsertTimelineEvent(context.Background(), e); err != nil {
			b.Fatal(err)
		}
	}
}

type noChangeBenchmarkAdapter struct {
	path string
}

func (a *noChangeBenchmarkAdapter) ID() model.AgentID { return "benchmark-agent" }

func (a *noChangeBenchmarkAdapter) Info() model.AdapterInfo {
	return model.AdapterInfo{ID: a.ID()}
}

func (a *noChangeBenchmarkAdapter) Detect(context.Context) ([]model.AgentInstance, error) {
	return []model.AgentInstance{{InstanceID: "benchmark", AgentID: a.ID(), DataDirectory: filepath.Dir(a.path)}}, nil
}

func (a *noChangeBenchmarkAdapter) Discover(context.Context, model.AgentInstance) ([]adapters.SessionSource, error) {
	info, err := os.Stat(a.path)
	if err != nil {
		return nil, err
	}
	return []adapters.SessionSource{{SessionID: "session", Path: a.path, Mtime: info.ModTime().UnixNano(), Size: info.Size()}}, nil
}

func (a *noChangeBenchmarkAdapter) ParseMetadata(context.Context, model.AgentInstance, adapters.SessionSource) (*model.Session, error) {
	now := time.Now()
	return &model.Session{
		AgentID:         a.ID(),
		AgentInstanceID: "benchmark",
		SessionID:       "session",
		SourcePath:      a.path,
		Activity:        model.ActivityInactive,
		IndexedAt:       now,
		UpdatedAt:       now,
	}, nil
}

func BenchmarkIndexerNoChange(b *testing.B) {
	root := b.TempDir()
	path := filepath.Join(root, "session.jsonl")
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		b.Fatal(err)
	}
	db, err := Open(filepath.Join(root, "index.db"))
	if err != nil {
		b.Fatal(err)
	}
	defer db.Close()
	if err := db.Migrate(context.Background()); err != nil {
		b.Fatal(err)
	}
	reg := adapters.NewRegistry()
	if err := reg.Register(&noChangeBenchmarkAdapter{path: path}); err != nil {
		b.Fatal(err)
	}
	a := &app.App{Registry: reg, Config: config.Default()}
	ix := &Indexer{App: a, DB: db}
	if _, err := ix.Run(context.Background()); err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ix.Run(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(buf[pos:])
}
