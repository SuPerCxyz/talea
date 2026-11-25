package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/i18n"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/status"
)

func boardStatus(agent model.AgentID, id string, state status.DisplayState, at time.Time) status.SessionStatus {
	question := "task " + id
	sess := &model.Session{
		AgentID:          agent,
		AgentInstanceID:  "inst",
		SessionID:        id,
		ProjectName:      "proj-" + id,
		WorkingDirectory: "/work/" + id,
		FirstQuestion:    question,
		LastActivityAt:   &at,
	}
	turn := model.TurnStateWorking
	switch state {
	case status.StateWaiting:
		turn = model.TurnStateWaitingInput
	case status.StateFailed:
		turn = model.TurnStateFailed
	}
	ss := status.SessionStatus{
		AgentID:          agent,
		AgentInstanceID:  "inst",
		SessionID:        id,
		ProjectName:      sess.ProjectName,
		WorkingDirectory: sess.WorkingDirectory,
		State:            state,
		TurnState:        turn,
		Source:           model.TurnStateSourceNative,
		Confidence:       status.ConfidenceExact,
		LastActivityAt:   &at,
		Label:            status.StateLabel(status.SessionStatus{State: state, TurnState: turn, Confidence: status.ConfidenceExact}),
		Session:          sess,
	}
	return ss
}

func TestGroupStatusesOrderAndSort(t *testing.T) {
	i18n.Set(i18n.LangEn)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })
	now := time.Now()
	sessions := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "idle", status.StateIdle, now.Add(-4*time.Hour)),
		boardStatus(model.AgentClaudeCode, "work-old", status.StateWorking, now.Add(-10*time.Minute)),
		boardStatus(model.AgentClaudeCode, "wait", status.StateWaiting, now.Add(-time.Minute)),
		boardStatus(model.AgentCodexCLI, "done", status.StateCompleted, now.Add(-2*time.Hour)),
		boardStatus(model.AgentOpenCode, "fail", status.StateFailed, now.Add(-3*time.Hour)),
		boardStatus(model.AgentOpenCode, "unknown", status.StateUnknown, now.Add(-5*time.Hour)),
		boardStatus(model.AgentClaudeCode, "work-new", status.StateWorking, now.Add(-time.Minute)),
	}
	groups := groupStatuses(sessions, "")
	if len(groups) != len(boardGroupOrder) {
		t.Fatalf("groups=%d", len(groups))
	}
	wantOrder := []status.DisplayState{
		status.StateWaiting, status.StateWorking, status.StateCompleted,
		status.StateFailed, status.StateIdle, status.StateUnknown,
	}
	for i, want := range wantOrder {
		if groups[i].state != want {
			t.Fatalf("group %d=%q want %q", i, groups[i].state, want)
		}
	}
	if len(groups[1].sessions) != 2 || groups[1].sessions[0].SessionID != "work-new" ||
		groups[1].sessions[1].SessionID != "work-old" {
		t.Fatalf("组内排序错误: %+v", groups[1].sessions)
	}
	rows := buildBoardRows(groups)
	if len(rows) != 6+len(sessions) {
		t.Fatalf("rows=%d", len(rows))
	}
	if !rows[0].header || rows[0].title != "Needs you (1)" {
		t.Fatalf("首个分组行=%+v", rows[0])
	}
}

func TestGroupStatusesEmptyAndFilter(t *testing.T) {
	groups := groupStatuses(nil, "")
	if len(groups) != len(boardGroupOrder) {
		t.Fatalf("空数据分组=%d", len(groups))
	}
	if rows := buildBoardRows(groups); len(rows) != 0 {
		t.Fatalf("空数据不应有行: %+v", rows)
	}

	now := time.Now()
	sessions := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "abc", status.StateCompleted, now),
		boardStatus(model.AgentOpenCode, "xyz", status.StateWorking, now),
	}
	if groups := groupStatuses(sessions, "abc"); len(groupStatuses(sessions, "abc")) != 6 {
		t.Fatalf("groups=%d", len(groups))
	}
	got := groupStatuses(sessions, "abc")
	if len(got[2].sessions) != 1 || got[2].sessions[0].SessionID != "abc" {
		t.Fatalf("过滤结果=%+v", got)
	}
	if got := groupStatuses(sessions, "opencode"); len(got[1].sessions) != 1 {
		t.Fatalf("按 Agent 过滤失败: %+v", got)
	}
}

func TestDashboardRenderShowsSummaryAndStatus(t *testing.T) {
	i18n.Set(i18n.LangEn)
	t.Cleanup(func() { i18n.Set(i18n.LangEn) })
	now := time.Now()
	sessions := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "ses_wait", status.StateWaiting, now.Add(-time.Minute)),
		boardStatus(model.AgentCodexCLI, "ses_done", status.StateCompleted, now.Add(-time.Hour)),
	}
	m := newDashboard(nil, nil, nil, "", status.Snapshot{
		Counts:   status.Counts{NeedsAttention: 1, Waiting: 1, Completed: 1, Total: 2},
		Sessions: sessions,
	}, nil)
	m.width = 120
	m.height = 40
	out := ansi.Strip(m.View())
	for _, want := range []string{
		"Needs you 1", "Completed (turn ended) 1", "Waiting for you",
		"Turn ended", "claude-code", "ses_wait", "task ses_wait",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("View 缺少 %q:\n%s", want, out)
		}
	}
}

func TestDashboardZeroWidthFallsBackToDefault(t *testing.T) {
	now := time.Now()
	m := newDashboard(nil, nil, nil, "", status.Snapshot{
		Sessions: []status.SessionStatus{boardStatus(model.AgentClaudeCode, "ses_1", status.StateCompleted, now)},
	}, nil)
	m.width = 0
	out := ansi.Strip(m.View())
	if !strings.Contains(out, "ses_1") || !strings.Contains(out, "Turn ended") {
		t.Fatalf("零宽度时未回退默认宽度:\n%s", out)
	}
}

func TestDashboardTaskLineShowsReasonForAttention(t *testing.T) {
	now := time.Now()
	cases := []struct {
		name   string
		state  status.DisplayState
		reason string
		key    string
	}{
		{
			name:   "waiting permission",
			state:  status.StateWaiting,
			reason: "permission prompt",
			key:    "permission",
		},
		{
			name:   "possibly waiting",
			state:  status.StateWaiting,
			reason: "possibly waiting: no new turn events for 3m",
			key:    "possibly waiting",
		},
		{
			name:   "failed",
			state:  status.StateFailed,
			reason: "API error",
			key:    "API error",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ss := boardStatus(model.AgentClaudeCode, "ses_1", tc.state, now)
			ss.Reason = tc.reason
			lines := boardRowLines(boardRow{session: ss}, 160, false)
			if len(lines) != 2 {
				t.Fatalf("行数=%d want=2", len(lines))
			}
			task := ansi.Strip(lines[1])
			if !strings.Contains(task, "task ses_1") || !strings.Contains(task, " · ") ||
				!strings.Contains(task, tc.key) {
				t.Fatalf("任务行缺少原因 %q: %q", tc.key, task)
			}
		})
	}
}

func TestDashboardTaskLineOmitsReasonForCompleted(t *testing.T) {
	now := time.Now()
	ss := boardStatus(model.AgentClaudeCode, "ses_1", status.StateCompleted, now)
	ss.Reason = "turn ended 2m ago; waiting for next step"
	task := ansi.Strip(boardRowLines(boardRow{session: ss}, 160, false)[1])
	if !strings.Contains(task, "task ses_1") || strings.Contains(task, "turn ended") {
		t.Fatalf("完成行不应追加 reason: %q", task)
	}
}

func TestDashboardNarrowRenderingFitsWidth(t *testing.T) {
	now := time.Now()
	ss := boardStatus(model.AgentClaudeCode, "ses_with_a_very_long_identifier", status.StateWaiting, now)
	ss.Label = "Possibly waiting (inferred)"
	ss.Confidence = status.ConfidenceInferred
	for _, width := range []int{20, 40, 80} {
		for _, line := range boardRowLines(boardRow{session: ss}, width, false) {
			if got := runewidth.StringWidth(ansi.Strip(line)); got > width {
				t.Fatalf("width=%d line 宽度=%d: %q", width, got, ansi.Strip(line))
			}
		}
	}
}

func TestDashboardDetailOpenBackAndResume(t *testing.T) {
	now := time.Now()
	sessions := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "ses_1", status.StateCompleted, now),
	}
	m := newDashboard(nil, nil, nil, "", status.Snapshot{Sessions: sessions}, nil)
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.detail == nil {
		t.Fatal("Enter 应打开详情")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.detail != nil {
		t.Fatal("Esc 应返回看板")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'o'}})
	if m.picked == nil || m.picked.SessionID != "ses_1" {
		t.Fatalf("恢复选择=%+v", m.picked)
	}
}

func TestDashboardRefreshErrorKeepsRows(t *testing.T) {
	now := time.Now()
	sessions := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "ses_1", status.StateWorking, now),
	}
	m := newDashboard(nil, nil, nil, "", status.Snapshot{Sessions: sessions}, nil)
	before := len(m.rows)
	m.Update(dashboardRefreshedMsg{err: errors.New("claude source unavailable")})
	if m.refreshErr == nil || len(m.rows) != before {
		t.Fatalf("刷新失败后 rows=%d err=%v", len(m.rows), m.refreshErr)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if m.cursor != 0 {
		t.Fatalf("cursor=%d", m.cursor)
	}
}

func TestDashboardRefreshAppliesSnapshotAndPreservesSelection(t *testing.T) {
	now := time.Now()
	old := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "a", status.StateWorking, now),
		boardStatus(model.AgentClaudeCode, "b", status.StateCompleted, now.Add(-time.Hour)),
	}
	m := newDashboard(nil, nil, nil, "", status.Snapshot{Sessions: old}, nil)
	m.cursor = 1
	next := []status.SessionStatus{
		boardStatus(model.AgentClaudeCode, "b", status.StateCompleted, now.Add(-time.Hour)),
		boardStatus(model.AgentClaudeCode, "a", status.StateCompleted, now),
	}
	snap := status.Snapshot{Sessions: next, Counts: status.Counts{Completed: 2, Total: 2}}
	m.Update(dashboardRefreshedMsg{snapshot: &snap})
	if len(m.snapshot.Sessions) != 2 {
		t.Fatalf("快照未更新: %+v", m.snapshot)
	}
	if ss, ok := selectedBoardSession(m.rows, m.cursor); !ok || ss.SessionID != "b" {
		t.Fatalf("选择未保留: cursor=%d rows=%+v", m.cursor, m.rows)
	}
}

func TestDashboardTickStartsRefreshOnce(t *testing.T) {
	m := newDashboard(nil, nil, nil, "", status.Snapshot{}, nil)
	m.loading = true
	_, cmd := m.Update(dashboardTickMsg{})
	if !m.refreshing {
		t.Fatal("tick 应启动刷新")
	}
	if cmd == nil {
		t.Fatal("tick 应续订下一次刷新")
	}
	m.Update(dashboardTickMsg{})
	if !m.refreshing {
		t.Fatal("重叠 tick 不应结束刷新状态")
	}
}
