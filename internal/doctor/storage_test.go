package doctor

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

// baseAdapter 是最小可用 fake adapter，不实现存储形态可选能力。
type baseAdapter struct {
	id       model.AgentID
	insts    []model.AgentInstance
	sessions int
}

func (b *baseAdapter) ID() model.AgentID { return b.id }

func (b *baseAdapter) Info() model.AdapterInfo {
	return model.AdapterInfo{ID: b.id, DisplayName: string(b.id)}
}

func (b *baseAdapter) Detect(context.Context) ([]model.AgentInstance, error) {
	return b.insts, nil
}

func (b *baseAdapter) Discover(context.Context, model.AgentInstance) ([]adapters.SessionSource, error) {
	return make([]adapters.SessionSource, b.sessions), nil
}

func (b *baseAdapter) ParseMetadata(
	context.Context, model.AgentInstance, adapters.SessionSource,
) (*model.Session, error) {
	return nil, nil
}

// probeAdapter 额外实现 storageProber 可选能力。
type probeAdapter struct {
	baseAdapter
	shape    string
	shapeErr error
}

func (p *probeAdapter) StorageShape(context.Context, model.AgentInstance) (string, error) {
	return p.shape, p.shapeErr
}

func testInstance() model.AgentInstance {
	return model.AgentInstance{
		InstanceID:     "i1",
		AgentID:        model.AgentOpenCode,
		ExecutablePath: "/usr/bin/opencode",
		Version:        "2.0.12",
		DataDirectory:  "/tmp/opencode-data",
	}
}

// newTestApp 构造仅注册指定 fake adapter 的 App，索引库路径指向不存在的文件。
func newTestApp(t *testing.T, ads ...adapters.Adapter) *app.App {
	t.Helper()
	reg := adapters.NewRegistry()
	for _, ad := range ads {
		if err := reg.Register(ad); err != nil {
			t.Fatal(err)
		}
	}
	return &app.App{Registry: reg, Config: config.Default(), Paths: config.Paths{}}
}

// storageChecks 过滤出存储形态行。
func storageChecks(rep Report) []Check {
	var out []Check
	for _, c := range rep.Checks {
		if strings.HasSuffix(c.Agent, " 存储形态") {
			out = append(out, c)
		}
	}
	return out
}

func mustSingleStorageCheck(t *testing.T, rep Report) Check {
	t.Helper()
	checks := storageChecks(rep)
	if len(checks) != 1 {
		t.Fatalf("expected 1 storage check, got %d: %+v", len(checks), checks)
	}
	return checks[0]
}

func TestRunReportsSessionV2StorageShape(t *testing.T) {
	a := newTestApp(t, &probeAdapter{
		baseAdapter: baseAdapter{id: "opencode", insts: []model.AgentInstance{testInstance()}, sessions: 3},
		shape:       shapeSessionV2,
	})
	rep, err := Run(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	c := mustSingleStorageCheck(t, rep)
	if c.Level != LevelOK {
		t.Fatalf("expected OK, got %s (%s)", c.Level, c.Text)
	}
	if !strings.Contains(c.Text, "session_v2") || !strings.Contains(c.Text, "3 个会话") {
		t.Fatalf("text = %q", c.Text)
	}
}

func TestRunReportsLegacyStorageShape(t *testing.T) {
	a := newTestApp(t, &probeAdapter{
		baseAdapter: baseAdapter{id: "opencode", insts: []model.AgentInstance{testInstance()}, sessions: 5},
		shape:       shapeLegacy,
	})
	rep, err := Run(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	c := mustSingleStorageCheck(t, rep)
	if c.Level != LevelOK {
		t.Fatalf("expected OK, got %s (%s)", c.Level, c.Text)
	}
	if !strings.Contains(c.Text, "传统 session 表") || !strings.Contains(c.Text, "5 个会话") {
		t.Fatalf("text = %q", c.Text)
	}
}

func TestRunStorageShapeUnreadable(t *testing.T) {
	a := newTestApp(t, &probeAdapter{
		baseAdapter: baseAdapter{id: "opencode", insts: []model.AgentInstance{testInstance()}, sessions: 2},
		shapeErr:    errors.New("只读打开数据库失败"),
	})
	rep, err := Run(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	c := mustSingleStorageCheck(t, rep)
	if c.Level != LevelWarn {
		t.Fatalf("expected WARN, got %s (%s)", c.Level, c.Text)
	}
	if !strings.Contains(c.Text, "无法确定存储形态") || !strings.Contains(c.Text, "只读打开数据库失败") {
		t.Fatalf("text = %q", c.Text)
	}
	// 不得报告成功
	if strings.Contains(c.Text, "session_v2 主表") || strings.Contains(c.Text, "传统 session 表") {
		t.Fatalf("unreadable shape must not report success: %q", c.Text)
	}
}

func TestRunStorageShapeNotImplemented(t *testing.T) {
	a := newTestApp(t, &baseAdapter{
		id:       "claude-code",
		insts:    []model.AgentInstance{testInstance()},
		sessions: 4,
	})
	rep, err := Run(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	if checks := storageChecks(rep); len(checks) != 0 {
		t.Fatalf("adapter without StorageShape must not emit shape line: %+v", checks)
	}
}

func TestRunStorageShapeSkippedWhenNotInstalled(t *testing.T) {
	a := newTestApp(t, &probeAdapter{
		baseAdapter: baseAdapter{id: "opencode"},
		shape:       shapeSessionV2,
	})
	rep, err := Run(context.Background(), a, "")
	if err != nil {
		t.Fatal(err)
	}
	if checks := storageChecks(rep); len(checks) != 0 {
		t.Fatalf("not-installed agent must not emit shape line: %+v", checks)
	}
	found := false
	for _, c := range rep.Checks {
		if c.Agent == "opencode" && c.Text == "未检测到安装" && c.Level == LevelWarn {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected existing 未检测到安装 warning, checks: %+v", rep.Checks)
	}
}
