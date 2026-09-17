package index

import (
	"context"
	"fmt"
	"sort"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

func trackedPaths(tracked map[string]TrackedSource, instanceID string) []string {
	seen := map[string]bool{}
	for _, source := range tracked {
		if source.AgentInstanceID == instanceID && source.SourcePath != "" {
			seen[source.SourcePath] = true
		}
	}
	return sortedPaths(seen)
}

func sourcePaths(sources []adapters.SessionSource) []string {
	seen := map[string]bool{}
	for _, source := range sources {
		if source.Path != "" {
			seen[source.Path] = true
		}
	}
	return sortedPaths(seen)
}

func mergePaths(a, b []string) []string {
	seen := map[string]bool{}
	for _, path := range a {
		seen[path] = true
	}
	for _, path := range b {
		seen[path] = true
	}
	return sortedPaths(seen)
}

func sortedPaths(paths map[string]bool) []string {
	out := make([]string, 0, len(paths))
	for path := range paths {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

func discoverSources(
	ctx context.Context,
	ad adapters.Adapter,
	inst model.AgentInstance,
	state adapters.DiscoveryState,
) ([]adapters.SessionSource, adapters.DiscoveryState, error) {
	if inc, ok := adapters.As[adapters.IncrementalDiscoverer](ad); ok {
		sources, next, err := inc.DiscoverIncremental(ctx, inst, state)
		if err == nil {
			return sources, next, nil
		}
		// 增量查询失败时回退完整发现，避免游标问题造成漏索引。
		sources, fullErr := ad.Discover(ctx, inst)
		if fullErr != nil {
			return nil, adapters.DiscoveryState{}, fmt.Errorf("增量和完整发现均失败: %w", fullErr)
		}
		return sources, adapters.DiscoveryState{
			Cursor: inc.CursorFromSources(inst, sources),
		}, nil
	}
	sources, err := ad.Discover(ctx, inst)
	return sources, adapters.DiscoveryState{}, err
}
