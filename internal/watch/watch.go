// Package watch 提供 Agent 数据目录的文件监听与增量索引。
//
// 这是可选优化，不是正常工作必要条件。talea 正常读取不依赖常驻守护进程。
package watch

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
)

// Options 控制监听行为。
type Options struct {
	Interval time.Duration // 事件合并窗口（防抖）
}

// Run 监听 Agent 数据目录，变化时增量索引，直到 ctx 取消。
func Run(ctx context.Context, a *app.App, opts Options) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer func() { _ = watcher.Close() }()

	// 收集要监听的目录
	dirs := dataDirs(a)
	if len(dirs) == 0 {
		return fmt.Errorf("没有可监听的 Agent 数据目录")
	}
	added := 0
	var lastErr error
	for _, d := range dirs {
		if err := watchRecursive(watcher, d, 0); err == nil {
			added++
		} else {
			lastErr = err
		}
	}
	if added == 0 {
		if lastErr != nil {
			return fmt.Errorf("无法监听 Agent 数据目录（系统 inotify 限制）: %w", lastErr)
		}
		return fmt.Errorf("没有可监听的 Agent 数据目录")
	}

	interval := opts.Interval
	if interval <= 0 {
		interval = 2 * time.Second
	}

	// 打开索引库
	db, err := index.Open(a.Paths.DBPath)
	if err != nil {
		return err
	}
	defer db.Close()
	if err := db.Migrate(ctx); err != nil {
		return err
	}

	fmt.Printf("talea watch: 监听 %d 个目录（Ctrl+C 退出）\n", added)

	// 事件合并：收集变更，interval 后统一触发一次索引
	var pending bool
	timer := time.NewTimer(interval)
	timer.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case err := <-watcher.Errors:
			if err != nil {
				fmt.Fprintf(os.Stderr, "watch: %v\n", err)
			}
		case ev := <-watcher.Events:
			// 忽略临时文件与编辑器锁
			if isIgnored(ev.Name) {
				continue
			}
			pending = true
			timer.Reset(interval)
		case <-timer.C:
			if !pending {
				continue
			}
			pending = false
			runIndex(ctx, a, db)
		}
	}
}

// runIndex 执行一次增量索引并汇报。
func runIndex(ctx context.Context, a *app.App, db *index.DB) {
	ix := &index.Indexer{App: a, DB: db}
	results, err := ix.Run(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "watch: 索引失败: %v\n", err)
		return
	}
	for _, r := range results {
		if r.Added > 0 || r.Updated > 0 {
			fmt.Printf("watch: %s 新增 %d 更新 %d\n", r.AgentID, r.Added, r.Updated)
		}
	}
}

// dataDirs 收集各 Agent 数据目录。
func dataDirs(a *app.App) []string {
	var dirs []string
	insts, err := a.DetectInstances(context.Background())
	if err != nil {
		return dirs
	}
	for _, inst := range insts {
		if inst.DataDirectory != "" {
			dirs = append(dirs, inst.DataDirectory)
		}
	}
	return dirs
}

// watchRecursive 递归添加目录监听，depth 限制深度避免过深耗尽 inotify。
// 默认最多 2 层：数据目录本身 + 其直接子目录（项目目录）。
// 深层变化依赖定期全量重扫（runIndex 是幂等增量）。
func watchRecursive(w *fsnotify.Watcher, root string, depth int) error {
	if depth > 2 {
		return nil
	}
	if err := w.Add(root); err != nil {
		return err
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() && !isIgnored(e.Name()) {
			_ = watchRecursive(w, filepath.Join(root, e.Name()), depth+1)
		}
	}
	return nil
}

// isIgnored 过滤临时文件、锁、日志等噪声。
func isIgnored(name string) bool {
	base := filepath.Base(name)
	if strings.HasPrefix(base, ".") {
		return true
	}
	for _, suffix := range []string{".tmp", ".lock", ".swp", "-wal", "-shm", ".log"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

var _ = model.AgentID("")
