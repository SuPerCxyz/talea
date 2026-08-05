// Package app 负责业务编排，将适配器、索引、搜索、恢复组合成上层能力。
// TUI 与 CLI 都通过本层访问业务逻辑。
package app

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/adapters/codex"
	"github.com/talea/talea/internal/adapters/opencode"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
)

// App 是业务编排入口。
type App struct {
	Registry *adapters.Registry
	Config   *config.Config
	Paths    config.Paths
}

// New 创建 App。
func New(ctx context.Context) (*App, error) {
	paths := config.ResolvePaths()
	cfg, err := config.Load(paths.ConfigPath)
	if err != nil {
		return nil, err
	}
	reg := adapters.NewRegistry()
	if err := registerBuiltins(reg); err != nil {
		return nil, err
	}
	return &App{Registry: reg, Config: cfg, Paths: paths}, nil
}

// DetectInstances 探测全部已启用 Agent 的实例。
func (a *App) DetectInstances(ctx context.Context) ([]model.AgentInstance, error) {
	var out []model.AgentInstance
	for _, ad := range a.Registry.All() {
		if agentCfg, ok := a.Config.Agents[string(ad.Info().ID)]; ok && !agentCfg.Enabled {
			continue
		}
		insts, err := ad.Detect(ctx)
		if err != nil {
			continue
		}
		out = append(out, insts...)
	}
	return out, nil
}

// SessionResult 描述一次会话解析结果（用于容错汇总）。
type SessionResult struct {
	Session *model.Session
	Err     error
}

// DiscoverSessions 发现并解析全部会话元数据。
func (a *App) DiscoverSessions(ctx context.Context) ([]SessionResult, error) {
	insts, err := a.DetectInstances(ctx)
	if err != nil {
		return nil, err
	}
	var results []SessionResult
	for _, inst := range insts {
		ad, ok := a.Registry.Get(inst.AgentID)
		if !ok {
			continue
		}
		sources, err := ad.Discover(ctx, inst)
		if err != nil {
			results = append(results, SessionResult{Err: fmt.Errorf("%s: %w", inst.AgentID, err)})
			continue
		}
		for _, src := range sources {
			sess, err := ad.ParseMetadata(ctx, inst, src)
			if err != nil {
				results = append(results, SessionResult{Err: fmt.Errorf("%s %s: %w", inst.AgentID, src.SessionID, err)})
				continue
			}
			results = append(results, SessionResult{Session: sess})
		}
	}
	return results, nil
}

// ResolveWorkingDirs 补齐目录存在性并读取 Git 信息。
func (a *App) ResolveWorkingDirs(ctx context.Context, sessions []*model.Session) {
	for _, s := range sessions {
		if s.WorkingDirectory == "" {
			continue
		}
		s.WorkingDirExists = dirExists(s.WorkingDirectory)
		if !s.WorkingDirExists {
			continue
		}
		if s.GitBranch == "" || s.GitRoot == "" {
			fillGitInfo(s)
		}
	}
}

func registerBuiltins(reg *adapters.Registry) error {
	reg.Register(claude.New())
	reg.Register(codex.New())
	reg.Register(opencode.New())
	return nil
}

// SortSessions 按 default_sort 排序。
func (a *App) SortSessions(sessions []*model.Session) {
	sort.SliceStable(sessions, func(i, j int) bool {
		return a.after(sessions[i], sessions[j])
	})
}

func (a *App) after(x, y *model.Session) bool {
	switch a.Config.General.DefaultSort {
	case "started_at":
		return tsOf(x.StartedAt) > tsOf(y.StartedAt)
	case "tokens":
		return tokenOf(x.TokenUsage) > tokenOf(y.TokenUsage)
	case "name":
		return strings.Compare(x.SessionID, y.SessionID) < 0
	default: // last_activity
		return tsOf(x.LastActivityAt) > tsOf(y.LastActivityAt)
	}
}

func tsOf(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func tokenOf(u *model.TokenUsage) int64 {
	if u == nil || u.TotalTokens == nil {
		return 0
	}
	return *u.TotalTokens
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}
