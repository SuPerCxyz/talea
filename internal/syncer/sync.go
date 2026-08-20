// Package syncer 提供「读取前同步索引」的编排能力，供 CLI 与 TUI 共用，
// 保证会话列表、搜索与恢复读取时新会话立即可见。
package syncer

import (
	"context"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/search"
)

// Sync 执行一次增量索引并同步 FTS 与活动状态。
// db 必须已由调用方打开并 Migrate。
// 错误按顺序传播（增量索引 → FTS 表同步 → FTS 填充）；
// 活动状态刷新失败仅记录，不阻塞读取。
func Sync(ctx context.Context, a *app.App, db *index.DB) error {
	if _, err := (&index.Indexer{App: a, DB: db}).Run(ctx); err != nil {
		return err
	}
	if err := search.Ensure(ctx, db); err != nil {
		return err
	}
	if err := search.Populate(ctx, db); err != nil {
		return err
	}
	// 重新检测活动状态（进程运行中/最近更新），保证读取时状态准确
	_, _ = (&index.Indexer{App: a, DB: db}).RefreshActivities(ctx)
	return nil
}
