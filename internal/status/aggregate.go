package status

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
)

// Build 从索引批量加载会话与回合状态，合并精确事件后生成快照。
// 单个事件文件或回合状态表读取失败只产生警告，不影响会话状态展示。
func Build(ctx context.Context, db *index.DB, opts Options) (Snapshot, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	snap := Snapshot{GeneratedAt: now}

	states, err := db.LoadTurnStates(ctx)
	if err != nil {
		snap.Warnings = append(snap.Warnings, err.Error())
		states = map[string]model.TurnStateRecord{}
	}
	events := EventIndex{}
	if opts.EventsPath != "" {
		loaded, eerr := LoadLatestEvents(opts.EventsPath)
		if eerr != nil {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("status events: %v", eerr))
		} else {
			events = loaded
		}
	}

	results, err := search.List(ctx, db, search.Query{
		Agent:            opts.Agent,
		Cwd:              opts.Cwd,
		ExcludeSubagents: !opts.IncludeSubagents,
		Unlimited:        true,
	})
	if err != nil {
		return Snapshot{}, err
	}

	window := opts.RecentWindow
	if window <= 0 {
		window = DefaultRecentWindow
	}
	snap.Sessions = make([]SessionStatus, 0, len(results))
	for i := range results {
		sess := results[i].Session
		rec := states[model.TurnStateKey(sess.AgentInstanceID, sess.SessionID)]
		event, ok := events.Latest(sess.AgentID, sess.AgentInstanceID, sess.SessionID)
		var eventPtr *Event
		if ok {
			eventPtr = &event
		}
		ss := Evaluate(sess, rec, eventPtr, now)
		if !visibleInWindow(ss, now, window) {
			continue
		}
		snap.Sessions = append(snap.Sessions, ss)
	}
	sort.SliceStable(snap.Sessions, func(i, j int) bool {
		ti, tj := snap.Sessions[i].LastActivityAt, snap.Sessions[j].LastActivityAt
		if ti == nil || tj == nil {
			if ti == nil && tj != nil {
				return false
			}
			if ti != nil && tj == nil {
				return true
			}
		} else if !ti.Equal(*tj) {
			return ti.After(*tj)
		}
		return snap.Sessions[i].SessionID < snap.Sessions[j].SessionID
	})
	snap.Counts = countOf(snap.Sessions)
	return snap, nil
}

// visibleInWindow 判断会话是否进入当前状态快照。
// 最近可见时间取 StateAt 与 LastActivityAt 中较新的值；无时间的等待/失败/进行中
// 会话保留，其余超过窗口的会话（包括旧 idle/unknown/completed）不展示。
func visibleInWindow(ss SessionStatus, now time.Time, window time.Duration) bool {
	at := latestVisibleTime(ss)
	if at == nil {
		switch ss.State {
		case StateWaiting, StateFailed, StateWorking:
			return true
		default:
			return false
		}
	}
	return now.Sub(*at) <= window
}

func latestVisibleTime(ss SessionStatus) *time.Time {
	if ss.StateAt == nil {
		return ss.LastActivityAt
	}
	if ss.LastActivityAt == nil {
		return ss.StateAt
	}
	if ss.StateAt.After(*ss.LastActivityAt) {
		return ss.StateAt
	}
	return ss.LastActivityAt
}

func countOf(sessions []SessionStatus) Counts {
	var c Counts
	c.Total = len(sessions)
	for _, ss := range sessions {
		switch ss.State {
		case StateWaiting:
			c.Waiting++
		case StateWorking:
			c.Working++
		case StateCompleted:
			c.Completed++
		case StateFailed:
			c.Failed++
		case StateIdle:
			c.Idle++
		default:
			c.Unknown++
		}
	}
	c.NeedsAttention = c.Waiting + c.Failed
	return c
}
