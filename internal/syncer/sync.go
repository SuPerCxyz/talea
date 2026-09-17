// Package syncer 提供「读取前同步索引」的编排能力，供 CLI 与 TUI 共用，
// 保证会话列表、搜索与恢复读取时新会话立即可见。
package syncer

import (
	"context"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/search"
)

// Stage 表示 TUI 可理解的同步阶段。
type Stage uint8

const (
	// StageDetecting 检查本地 Agent。
	StageDetecting Stage = iota
	// StageSyncing 同步会话记录。
	StageSyncing
	// StagePreparing 准备会话列表。
	StagePreparing
)

// ProgressFunc 接收同步阶段切换通知。
type ProgressFunc func(Stage)

// Sync 执行一次增量索引并同步 FTS 与活动状态。
// db 必须已由调用方打开并 Migrate。
// 错误按顺序传播（增量索引 → FTS 表同步 → FTS 填充）；
// 活动状态刷新失败仅记录，不阻塞读取。
func Sync(ctx context.Context, a *app.App, db *index.DB) error {
	return SyncWithProgress(ctx, a, db, nil)
}

// SyncWithProgress 执行同步并在真实阶段切换时调用 progress。
func SyncWithProgress(ctx context.Context, a *app.App, db *index.DB, progress ProgressFunc) error {
	report(progress, StageDetecting)
	ix := &index.Indexer{
		App: a,
		DB:  db,
		Progress: func(stage index.ProgressStage) {
			if stage == index.ProgressIndexing {
				report(progress, StageSyncing)
			}
		},
	}
	results, err := ix.Run(ctx)
	if err != nil {
		return err
	}
	report(progress, StagePreparing)
	if err := search.Ensure(ctx, db); err != nil {
		return err
	}
	changed := false
	for _, result := range results {
		changed = changed || result.Changed
	}
	needFTS := changed
	if !needFTS {
		needFTS, err = search.NeedsPopulate(ctx, db)
		if err != nil {
			return err
		}
	}
	if needFTS {
		if err := search.Populate(ctx, db); err != nil {
			return err
		}
	}
	// 重新检测活动状态（进程运行中/最近更新），保证读取时状态准确
	_, _ = ix.RefreshActivities(ctx)
	return nil
}

func report(progress ProgressFunc, stage Stage) {
	if progress != nil {
		progress(stage)
	}
}
