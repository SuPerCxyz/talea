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

// indexTimelineEvents 索引会话的时间线事件。
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
	for {
		e, ok, err := it.Next()
		if err != nil {
			return err
		}
		if !ok {
			break
		}
		if _, err := ix.DB.UpsertTimelineEvent(ctx, e); err != nil {
			return err
		}
	}
	return nil
}

// ResolveSubagentRelations 聚合子 Agent Token 到父会话。
// 返回聚合的关系数。单条失败不中止。
func (ix *Indexer) ResolveSubagentRelations(ctx context.Context) (int, error) {
	insts, err := ix.App.DetectInstances(ctx)
	if err != nil {
		return 0, err
	}
	// 先加载全部已索引会话
	all, err := ix.loadAllSessions(ctx)
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
		var sessions []model.Session
		for _, s := range all {
			if s.AgentInstanceID == inst.InstanceID {
				sessions = append(sessions, *s)
			}
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

// loadAllSessions 读取全部索引会话（精简字段）。
func (ix *Indexer) loadAllSessions(ctx context.Context) ([]*model.Session, error) {
	rows, err := ix.DB.SQL().QueryContext(ctx,
		`SELECT agent_id, agent_instance_id, session_id, source_path FROM sessions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.Session
	for rows.Next() {
		s := &model.Session{}
		if err := rows.Scan(&s.AgentID, &s.AgentInstanceID, &s.SessionID, &s.SourcePath); err != nil {
			continue
		}
		out = append(out, s)
	}
	return out, nil
}

// RefreshActivities 重新检测全部会话的活动状态并写回。
func (ix *Indexer) RefreshActivities(ctx context.Context) (int, error) {
	all, err := ix.loadAllSessions(ctx)
	if err != nil {
		return 0, err
	}
	updated := 0
	for _, s := range all {
		ad, ok := ix.App.Registry.Get(s.AgentID)
		if !ok {
			continue
		}
		det, ok := adapters.As[adapters.ActivityDetector](ad)
		if !ok {
			continue
		}
		state, err := det.DetectActivity(ctx, *s)
		if err != nil {
			continue
		}
		if _, err := ix.DB.SetActivity(ctx, s.AgentInstanceID, s.SessionID, state); err == nil {
			updated++
		}
	}
	return updated, nil
}

var _ = adapters.Command{}
