// Package app 负责业务编排，将适配器、索引、搜索、恢复组合成上层能力。
// TUI 与 CLI 都通过本层访问业务逻辑。
package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/claude"
	"github.com/talea/talea/internal/adapters/codex"
	"github.com/talea/talea/internal/adapters/generic"
	"github.com/talea/talea/internal/adapters/opencode"
	"github.com/talea/talea/internal/config"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/plugadapt"
	"github.com/talea/talea/internal/plugin"
)

// App 是业务编排入口。
type App struct {
	Registry *adapters.Registry
	Config   *config.Config
	Paths    config.Paths

	// DetectInstances 结果缓存（5 秒 TTL）
	detectMu    sync.Mutex
	detectCache []model.AgentInstance
	detectAt    time.Time
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
	registerPlugins(ctx, reg)
	return &App{Registry: reg, Config: cfg, Paths: paths}, nil
}

// registerPlugins 注册 PATH 中的外部适配器。
// 插件加载失败不影响内置适配器。
func registerPlugins(ctx context.Context, reg *adapters.Registry) {
	for _, path := range plugin.DiscoverPlugins() {
		ad, err := plugadapt.New(path)
		if err != nil {
			continue
		}
		if err := reg.Register(ad); err != nil {
			_ = ad.Close()
		}
	}
}

// DetectInstances 探测全部已启用 Agent 的实例。
// 结果缓存 5 秒，避免一次索引流程中多次重复启动外部进程探测。
func (a *App) DetectInstances(ctx context.Context) ([]model.AgentInstance, error) {
	a.detectMu.Lock()
	if a.detectCache != nil && time.Since(a.detectAt) < 5*time.Second {
		// 返回副本，避免调用方修改缓存
		out := make([]model.AgentInstance, len(a.detectCache))
		copy(out, a.detectCache)
		a.detectMu.Unlock()
		return out, nil
	}
	a.detectMu.Unlock()

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

	a.detectMu.Lock()
	a.detectCache = out
	a.detectAt = time.Now()
	a.detectMu.Unlock()
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
// 相同工作目录只 stat / git 一次，结果按会话缺省字段回填，避免 N 次外部命令。
func (a *App) ResolveWorkingDirs(ctx context.Context, sessions []*model.Session) {
	type gitInfo struct {
		root, branch, remote string
	}
	cache := make(map[string]gitInfo)
	for _, s := range sessions {
		if s.WorkingDirectory == "" {
			continue
		}
		dir := s.WorkingDirectory
		s.WorkingDirExists = dirExists(dir)
		if !s.WorkingDirExists {
			continue
		}
		if s.GitBranch != "" && s.GitRoot != "" {
			continue
		}
		g, ok := cache[dir]
		if !ok {
			g = fillGitInfoDir(dir)
			cache[dir] = g
		}
		if s.GitRoot == "" {
			s.GitRoot = g.root
		}
		if s.GitBranch == "" {
			s.GitBranch = g.branch
		}
		if s.GitRemote == "" {
			s.GitRemote = g.remote
		}
		if s.ProjectName == "" && g.root != "" {
			s.ProjectName = filepath.Base(g.root)
		}
	}
}

func registerBuiltins(reg *adapters.Registry) error {
	if err := reg.Register(claude.New()); err != nil {
		return err
	}
	if err := reg.Register(codex.New()); err != nil {
		return err
	}
	if err := reg.Register(opencode.New()); err != nil {
		return err
	}
	if err := reg.Register(generic.New()); err != nil {
		return err
	}
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
