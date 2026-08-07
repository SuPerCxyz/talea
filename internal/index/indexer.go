package index

import (
	"context"
	"fmt"
	"os"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/adapters/extract"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/model"
)

// Indexer 编排发现、解析与增量写入。
type Indexer struct {
	App   *app.App
	DB    *DB
	Force bool // 全量重建（忽略增量跳过）

	// Instances 可选：注入固定的 Agent 实例列表（测试用）。
	// 为空时通过 App.DetectInstances 探测。
	Instances []model.AgentInstance
}

// Result 是单 Agent 的索引结果。
type Result struct {
	AgentID   model.AgentID
	Added     int
	Updated   int
	Skipped   int
	Errors    int
	ErrorMsgs []string
}

// Run 执行索引，返回按 Agent 汇总的结果。
func (ix *Indexer) Run(ctx context.Context) ([]Result, error) {
	tracked, err := ix.DB.LoadTracked(ctx)
	if err != nil {
		return nil, err
	}
	insts, err := ix.App.DetectInstances(ctx)
	if err != nil {
		return nil, err
	}
	if len(ix.Instances) > 0 {
		insts = ix.Instances
	}

	var out []Result
	for _, inst := range insts {
		if err := ctx.Err(); err != nil {
			return out, err
		}
		res := Result{AgentID: inst.AgentID}
		ad, ok := ix.App.Registry.Get(inst.AgentID)
		if !ok {
			continue
		}
		sources, err := ad.Discover(ctx, inst)
		if err != nil {
			res.Errors++
			res.ErrorMsgs = append(res.ErrorMsgs, err.Error())
			out = append(out, res)
			continue
		}
		var batch []*model.Session
		for _, src := range sources {
			if err := ctx.Err(); err != nil {
				return out, err
			}
			key := inst.InstanceID + "\x00" + src.SessionID
			t, known := tracked[key]
			if known && !ix.Force && t.SourceMtime == src.Mtime && t.SourceSize == src.Size {
				res.Skipped++
				continue
			}
			// 文件被截断（新 size 小于已记录偏移）：mtime 已变化，走重解析分支。
			_ = t
			sess, err := ad.ParseMetadata(ctx, inst, src)
			if err != nil {
				if os.IsNotExist(err) {
					res.Skipped++
					continue
				}
				res.Errors++
				res.ErrorMsgs = append(res.ErrorMsgs, fmt.Sprintf("%s/%s: %v", inst.AgentID, src.SessionID, err))
				continue
			}
			sess.IndexedAt = sess.UpdatedAt
			// 断点续读：记录文件解析偏移（JSONL 源），用于截断/尾行暂存检测
			if off, err := extract.LastCompleteLineOffset(sess.SourcePath); err == nil {
				sess.SourceOffset = off
			}
			ix.App.ResolveWorkingDirs(ctx, []*model.Session{sess})
			batch = append(batch, sess)
		}
		if len(batch) > 0 {
			st, err := ix.DB.UpsertMany(ctx, batch)
			if err != nil {
				res.Errors++
				res.ErrorMsgs = append(res.ErrorMsgs, err.Error())
			}
			res.Added += st.Added
			res.Updated += st.Updated
			res.Errors += st.Errors
			for _, e := range st.Errs {
				res.ErrorMsgs = append(res.ErrorMsgs, e.Error())
			}
			// 时间线事件（独立于会话批处理，单个会话失败不中止）
			for _, sess := range batch {
				if err := ctx.Err(); err != nil {
					return out, err
				}
				if ix.Force {
					// 重建时清除旧事件，避免 INSERT OR IGNORE 保留过期数据
					_ = ix.DB.ClearTimelineEvents(ctx, sess.AgentInstanceID, sess.SessionID)
				}
				if err := ix.indexTimelineEvents(ctx, sess, ad); err != nil {
					res.Errors++
					res.ErrorMsgs = append(res.ErrorMsgs, fmt.Sprintf("%s/%s timeline: %v", inst.AgentID, sess.SessionID, err))
				}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// indexTimelineEvents 索引会话的时间线事件（单事务批量写入）。
func (ix *Indexer) indexTimelineEvents(ctx context.Context, sess *model.Session, ad adapters.Adapter) error {
	prov, ok := adapters.As[adapters.UsageTimelineProvider](ad)
	if !ok {
		return nil
	}
	it, err := prov.IterateUsageEvents(ctx, *sess)
	if err != nil {
		return err
	}
	defer it.Close()
	var batch []*model.UsageTimelineEvent
	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		if _, err := ix.DB.UpsertTimelineEvents(ctx, batch); err != nil {
			return err
		}
		batch = batch[:0]
		return nil
	}
	for {
		e, ok, err := it.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		batch = append(batch, e)
		if len(batch) >= 500 {
			if err := flush(); err != nil {
				return err
			}
		}
	}
	return flush()
}

// ResolveSubagentRelations 聚合子 Agent Token 到父会话。
// 返回聚合的关系数。单条失败不中止。
func (ix *Indexer) ResolveSubagentRelations(ctx context.Context) (int, error) {
	insts, err := ix.App.DetectInstances(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, inst := range insts {
		ad, ok := ix.App.Registry.Get(inst.AgentID)
		if !ok {
			continue
		}
		prov, ok := adapters.As[adapters.SubagentProvider](ad)
		if !ok {
			continue
		}
		// 按实例查询，避免加载全量会话后 O(n) 过滤
		sessions, err := ix.loadSessionsByInstance(ctx, inst.InstanceID)
		if err != nil {
			continue
		}
		rels, err := prov.ResolveSessionRelations(ctx, sessions)
		if err != nil {
			continue
		}
		for _, rel := range rels {
			if err := ix.DB.AggregateChildTokens(ctx, rel); err == nil {
				count++
			}
		}
	}
	return count, nil
}

// loadSessionsByInstance 读取指定实例的已索引会话（精简字段）。
func (ix *Indexer) loadSessionsByInstance(ctx context.Context, instanceID string) ([]model.Session, error) {
	rows, err := ix.DB.SQL().QueryContext(ctx,
		`SELECT agent_id, agent_instance_id, session_id, source_path FROM sessions
		 WHERE agent_instance_id = ?`, instanceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.Session
	for rows.Next() {
		var s model.Session
		if err := rows.Scan(&s.AgentID, &s.AgentInstanceID, &s.SessionID, &s.SourcePath); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// RefreshActivities 重新检测全部会话的活动状态并写回。
// 性能优化：
//   - 每个 Agent 只做一次进程检测（O(/proc)）。
//   - 无任何 Agent 进程运行时，单条 SQL 批量标记 inactive。
//   - 有进程运行时，仅在受影响 Agent 的会话上按文件 mtime 判定。
func (ix *Indexer) RefreshActivities(ctx context.Context) (int, error) {
	// 预计算每个 Agent 的进程状态（每个 executable 一次）
	processActive := map[model.AgentID]bool{}
	anyActive := false
	for _, ad := range ix.App.Registry.All() {
		info := ad.Info()
		exe := executableOf(info.ID)
		active := exe != "" && adapters.AgentProcessRunning(exe)
		processActive[info.ID] = active
		if active {
			anyActive = true
		}
	}

	// 无任何 Agent 运行：批量标记 inactive（快路径）
	if !anyActive {
		n, err := ix.DB.SetAllActivity(ctx, model.ActivityInactive)
		if err != nil {
			return 0, err
		}
		return int(n), nil
	}

	// 有 Agent 运行：先批量标 inactive，再对最近更新的会话标 possibly_active
	// （单条 SQL，避免逐会话 os.Stat）
	updated := 0
	for _, ad := range ix.App.Registry.All() {
		info := ad.Info()
		if !processActive[info.ID] {
			continue
		}
		if _, err := ix.DB.SetAllActivityByAgent(ctx, info.ID, model.ActivityInactive); err != nil {
			return 0, err
		}
		// 最近 30 秒内更新的会话标记为可能进行中
		n, err := ix.DB.SetRecentActive(ctx, info.ID, 30)
		if err == nil {
			updated += int(n)
		}
	}
	return updated, nil
}

// executableOf 返回 Agent 的进程可执行文件名。
func executableOf(id model.AgentID) string {
	switch id {
	case model.AgentClaudeCode:
		return "claude"
	case model.AgentCodexCLI:
		return "codex"
	case model.AgentOpenCode:
		return "opencode"
	default:
		return ""
	}
}

var _ = adapters.Command{}
