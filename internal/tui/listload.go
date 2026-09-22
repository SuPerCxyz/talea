package tui

import (
	"context"
	"sort"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/timeline"
)

// loadTuiSessions 加载 TUI 会话列表，dir 非空时仅保留该目录下的会话，
// agent 非空时仅保留该 Agent 的会话。
// 按配置 general.include_subagents 过滤子 Agent 会话（默认隐藏，与 talea list 一致）；
// 全量加载不设条数上限，浏览完整列表由 list.Model 分页承担。
// 固定按结束时间倒序排列（最新结束在前），不受配置 default_sort 影响，
// 与 talea list / talea go 保持一致。
// 返回会话列表及对应的 usage 汇总（key=agent_instance_id\x00session_id）。
func loadTuiSessions(ctx context.Context, a *app.App, db *index.DB, dir, agent string) ([]*model.Session, map[string]timeline.SessionUsageRow, error) {
	results, err := search.List(ctx, db, search.Query{
		Cwd:   dir,
		Agent: agent,
		// 在 SQL 层遵守配置 general.include_subagents（默认隐藏子会话，与 talea list 一致）
		ExcludeSubagents: !includeSubagents(a),
		// 全量加载不设条数上限（分页由 list.Model 承担），避免静默截断
		Unlimited: true,
	})
	if err != nil {
		return nil, nil, err
	}
	sessions := make([]*model.Session, 0, len(results))
	for i := range results {
		sessions = append(sessions, &results[i].Session)
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return endTs(sessions[i]) > endTs(sessions[j])
	})
	// 批量填充最近一次用户消息与 usage（供 TUI 展示）
	fillLastUserPrompts(ctx, db, sessions)
	usages := fillUsages(ctx, db, sessions)
	return sessions, usages, nil
}

// includeSubagents 返回配置是否要求展示子 Agent 会话
// （general.include_subagents 默认 false；配置对象缺失时保守按默认隐藏处理）。
func includeSubagents(a *app.App) bool {
	return a != nil && a.Config != nil && a.Config.General.IncludeSubagents
}

// fillUsages 批量查询会话的 Token 汇总（含缓存字段）。
func fillUsages(ctx context.Context, db *index.DB, sessions []*model.Session) map[string]timeline.SessionUsageRow {
	if db == nil || len(sessions) == 0 {
		return nil
	}
	keys := make([][2]string, 0, len(sessions))
	seen := map[string]bool{}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if seen[key] {
			continue
		}
		seen[key] = true
		keys = append(keys, [2]string{s.AgentInstanceID, s.SessionID})
	}
	usages, err := timeline.UsageBySession(ctx, db, keys)
	if err != nil {
		return nil
	}
	return usages
}

// fillLastUserPrompts 批量查询会话的最后一次用户消息并写入 LastUserPrompt。
func fillLastUserPrompts(ctx context.Context, db *index.DB, sessions []*model.Session) {
	if db == nil || len(sessions) == 0 {
		return
	}
	keys := make([][2]string, 0, len(sessions))
	seen := map[string]int{}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = len(keys)
		keys = append(keys, [2]string{s.AgentInstanceID, s.SessionID})
	}
	prompts, err := timeline.LastUserPromptBySession(ctx, db, keys)
	if err != nil {
		return
	}
	for _, s := range sessions {
		if s.SessionID == "" {
			continue
		}
		key := s.AgentInstanceID + "\x00" + s.SessionID
		if p, ok := prompts[key]; ok {
			s.LastUserPrompt = p
		}
	}
}

// endTs 返回会话结束时间的 Unix 秒（无结束时间依次用开始时间、最后活动时间兜底）。
func endTs(s *model.Session) int64 {
	if s.EndedAt != nil {
		return s.EndedAt.Unix()
	}
	if s.StartedAt != nil {
		return s.StartedAt.Unix()
	}
	if s.LastActivityAt != nil {
		return s.LastActivityAt.Unix()
	}
	return 0
}
