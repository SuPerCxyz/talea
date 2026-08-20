package app

import (
	"errors"
	"strings"

	"github.com/talea/talea/internal/model"
)

// 错误哨兵，供上层映射退出码。
var (
	ErrAgentRequired   = errors.New("agent name is required")
	ErrAgentAmbiguous  = errors.New("agent name is ambiguous")
	ErrAgentUnknown    = errors.New("unknown agent")
)

// ResolveAgent 将用户输入的 agent 名称解析为注册表中的 AgentID。
// 匹配规则（忽略大小写与分隔符）：
//  1. 与规范 ID 或显示名归一化后精确匹配；
//  2. 无匹配时按唯一前缀匹配（如 codex → codex-cli）；
//  3. 多个候选或无法匹配时返回错误并列出可用 Agent。
func (a *App) ResolveAgent(name string) (model.AgentID, error) {
	norm := normalizeAgentName(name)
	if norm == "" {
		return "", ErrAgentRequired
	}

	type cand struct {
		id   model.AgentID
		norm string
	}
	var cands []cand
	seenNorm := map[string]model.AgentID{}
	for _, ad := range a.Registry.All() {
		info := ad.Info()
		names := []string{string(info.ID), info.DisplayName}
		for _, n := range names {
			nrm := normalizeAgentName(n)
			if nrm == "" {
				continue
			}
			if prev, ok := seenNorm[nrm]; ok {
				if prev == info.ID {
					// 同一 Agent 的 ID 与显示名归一化相同，只保留一条
					continue
				}
			}
			seenNorm[nrm] = info.ID
			cands = append(cands, cand{info.ID, nrm})
		}
	}

	// 精确匹配（归一化后）
	var exact []model.AgentID
	for _, c := range cands {
		if c.norm == norm {
			exact = append(exact, c.id)
		}
	}
	switch len(exact) {
	case 1:
		return exact[0], nil
	case 0:
		// 继续尝试唯一前缀
	default:
		return "", ErrAgentAmbiguous
	}

	// 唯一前缀匹配
	var prefixes []model.AgentID
	for _, c := range cands {
		if strings.HasPrefix(c.norm, norm) {
			prefixes = append(prefixes, c.id)
		}
	}
	switch len(prefixes) {
	case 1:
		return prefixes[0], nil
	case 0:
		return "", ErrAgentUnknown
	default:
		return "", ErrAgentAmbiguous
	}
}

// normalizeAgentName 归一化 agent 名称：小写并去掉空格、连字符、下划线、点。
func normalizeAgentName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch r {
		case ' ', '-', '_', '.':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// AgentNames 返回注册表中全部 Agent 的规范 ID。
func (a *App) AgentNames() []string {
	var names []string
	for _, ad := range a.Registry.All() {
		names = append(names, string(ad.Info().ID))
	}
	return names
}
