package index

import (
	"context"
	"fmt"
	"os"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/model"
)

// Indexer 编排发现、解析与增量写入。
type Indexer struct {
	App   *app.App
	DB    *DB
	Force bool // 全量重建（忽略增量跳过）
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
		}
		out = append(out, res)
	}
	return out, nil
}

var _ = adapters.Command{}
