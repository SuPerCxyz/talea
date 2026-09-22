package opencode

import (
	"context"
	"strings"
	"testing"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

// collectEvents 拉取全部时间线事件。
func collectEvents(t *testing.T, it adapters.UsageEventIterator, err error) []*model.UsageTimelineEvent {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	var out []*model.UsageTimelineEvent
	for {
		ev, ok, nerr := it.Next()
		if nerr != nil {
			t.Fatal(nerr)
		}
		if !ok {
			return out
		}
		out = append(out, ev)
	}
}

// collectMessages 拉取全部消息预览。
func collectMessages(t *testing.T, it adapters.MessageIterator, err error) []adapters.Message {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
	defer it.Close()
	var out []adapters.Message
	for {
		m, ok, nerr := it.Next()
		if nerr != nil {
			t.Fatal(nerr)
		}
		if !ok {
			return out
		}
		out = append(out, m)
	}
}

// testSession 返回只带来源路径与会话 ID 的会话对象。
func testSession(path, id string) model.Session {
	return model.Session{SessionID: id, SourcePath: path}
}

// TestMessageRoutingByDataPresence 覆盖按数据分流（任务 3.1）。
func TestMessageRoutingByDataPresence(t *testing.T) {
	ctx := context.Background()

	legacyDB, err := openRO(createFixtureDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer legacyDB.Close()
	if !useLegacyMessages(ctx, legacyDB, "ses_0001") {
		t.Fatal("message 表有行的会话应走传统路径")
	}
	if useLegacyMessages(ctx, legacyDB, "ses_unknown") {
		t.Fatal("message 表无行的会话应走 v2 路径")
	}

	v2DB, err := openRO(createV2FixtureDB(t))
	if err != nil {
		t.Fatal(err)
	}
	defer v2DB.Close()
	if useLegacyMessages(ctx, v2DB, "ses_new") {
		t.Fatal("message 表不存在时应走 v2 路径")
	}
}

// TestV2FirstQuestion 覆盖 v2 首问提取与注入过滤（任务 3.2）。
func TestV2FirstQuestion(t *testing.T) {
	path := createV2FixtureDB(t)
	a := New()
	ctx := context.Background()
	inst := model.AgentInstance{InstanceID: "t", AgentID: model.AgentOpenCode}

	s, err := a.ParseMetadata(ctx, inst, adapters.SessionSource{SessionID: "ses_new", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "新会话的第一个问题" ||
		s.FirstQuestionSource != "user_message" || s.FirstQuestionConfidence != 1.0 {
		t.Fatalf("首问=%q source=%q conf=%v", s.FirstQuestion, s.FirstQuestionSource, s.FirstQuestionConfidence)
	}

	s, err = a.ParseMetadata(ctx, inst, adapters.SessionSource{SessionID: "ses_v2inject", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "真实提问" || s.FirstQuestionSource != "user_message" {
		t.Fatalf("注入过滤后首问=%q source=%q", s.FirstQuestion, s.FirstQuestionSource)
	}

	s, err = a.ParseMetadata(ctx, inst, adapters.SessionSource{SessionID: "ses_v2empty", Path: path})
	if err != nil {
		t.Fatal(err)
	}
	if s.FirstQuestion != "" || s.FirstQuestionSource != "none" || s.FirstQuestionConfidence != 0 {
		t.Fatalf("无消息会话首问=%q source=%q", s.FirstQuestion, s.FirstQuestionSource)
	}
}

// TestV2LoadMessagesPreview 覆盖 v2 消息预览与 Limit（任务 3.1 / 3.3）。
func TestV2LoadMessagesPreview(t *testing.T) {
	path := createV2FixtureDB(t)
	it, err := New().LoadMessages(context.Background(),
		testSession(path, "ses_new"), adapters.MessageLoadOptions{})
	msgs := collectMessages(t, it, err)
	if len(msgs) != 3 {
		t.Fatalf("消息数=%d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != typeUser || msgs[0].Content != "新会话的第一个问题" || msgs[0].Timestamp != 1789800001 {
		t.Fatalf("user 消息=%+v", msgs[0])
	}
	// assistant 只拼 type=text 的文本，工具块不进入正文
	if msgs[1].Role != typeAssistant || msgs[1].Content != "正在查看文件。" {
		t.Fatalf("assistant 消息=%+v", msgs[1])
	}
	if msgs[2].Content != "完成。" {
		t.Fatalf("assistant 消息=%+v", msgs[2])
	}

	limitedIt, err := New().LoadMessages(context.Background(),
		testSession(path, "ses_new"), adapters.MessageLoadOptions{Limit: 1})
	limited := collectMessages(t, limitedIt, err)
	if len(limited) != 1 || limited[0].Content != "完成。" {
		t.Fatalf("limit=1 结果=%+v", limited)
	}
}

// TestLegacyLoadMessagesUnchanged 保证存量会话仍走 message/part 且内容不变（任务 3.1 / 3.6）。
func TestLegacyLoadMessagesUnchanged(t *testing.T) {
	path := createFixtureDB(t)
	it, err := New().LoadMessages(context.Background(),
		testSession(path, "ses_0001"), adapters.MessageLoadOptions{})
	msgs := collectMessages(t, it, err)
	if len(msgs) != 2 {
		t.Fatalf("消息数=%d: %+v", len(msgs), msgs)
	}
	if msgs[0].Role != typeUser || msgs[0].Content != "请查看还有哪些未完成的任务" {
		t.Fatalf("user 消息=%+v", msgs[0])
	}
	if msgs[1].Role != typeAssistant || msgs[1].Content != "好的，已查看。" {
		t.Fatalf("assistant 消息=%+v", msgs[1])
	}
}

// TestV2TimelineEvents 覆盖 user/tool/request/compaction 事件与 v2 来源前缀（任务 3.4 / 3.5）。
func TestV2TimelineEvents(t *testing.T) {
	path := createV2FixtureDB(t)
	tlIt, err := New().IterateUsageEvents(context.Background(), testSession(path, "ses_new"))
	events := collectEvents(t, tlIt, err)
	if len(events) != 5 {
		t.Fatalf("事件数=%d: %+v", len(events), eventIDs(events))
	}

	user := events[0]
	if user.EventType != model.UsageEventUserMessage || user.EventID != "v2msg-sm_u1" ||
		user.SourceIdentity != "opencode-v2msg:sm_u1" ||
		user.UserPromptPreview != "新会话的第一个问题" {
		t.Fatalf("user 事件=%+v", user)
	}

	tool := events[1]
	if tool.EventType != model.UsageEventToolEnd || tool.EventID != "v2tool-sm_a1-1" ||
		tool.SourceIdentity != "opencode-v2tool:sm_a1:1" ||
		tool.ToolCallID != "call_1" || tool.ToolName != "bash" ||
		tool.FilePath != "/home/alice/code/nexora/main.go" {
		t.Fatalf("tool 事件=%+v", tool)
	}

	req := events[2]
	if req.EventType != model.UsageEventRequest || req.EventID != "v2req-sm_a1" ||
		req.SourceIdentity != "opencode-v2msg:sm_a1" || req.Model != "m1" ||
		req.InputTokens == nil || *req.InputTokens != 232 ||
		req.CacheReadTokens == nil || *req.CacheReadTokens != 100 ||
		req.CacheWriteTokens == nil || *req.CacheWriteTokens != 10 {
		t.Fatalf("request 事件=%+v", req)
	}

	compact := events[4]
	if compact.EventType != model.UsageEventCompactionStart || compact.EventID != "v2comp-sm_c1" ||
		compact.SourceIdentity != "opencode-v2comp:sm_c1" {
		t.Fatalf("compaction 事件=%+v", compact)
	}

	for i, ev := range events {
		if int64(i) != ev.Sequence {
			t.Fatalf("事件 %d 序号=%d", i, ev.Sequence)
		}
		if !strings.HasPrefix(ev.SourceIdentity, "opencode-v2") {
			t.Fatalf("v2 事件使用了非 v2 前缀: %q", ev.SourceIdentity)
		}
		if ev.ContextAfter != nil || ev.CumulativeTotal != nil || ev.TotalTokens != nil {
			t.Fatalf("v2 事件上下文快照必须保持未知: %+v", ev)
		}
	}
	if ids := eventIDs(events); len(ids) != len(events) {
		t.Fatalf("事件 ID 重复: %v", ids)
	}
}

// TestV2TimelineTokensAreIncrements 覆盖单次增量口径与上下文快照未知（任务 3.4 / 3.5）。
func TestV2TimelineTokensAreIncrements(t *testing.T) {
	path := createV2FixtureDB(t)
	tlIt, err := New().IterateUsageEvents(context.Background(), testSession(path, "ses_new"))
	events := collectEvents(t, tlIt, err)

	var input, output, reasoning, reqs int64
	for _, ev := range events {
		if ev.EventType != model.UsageEventRequest {
			continue
		}
		reqs++
		if ev.InputTokens != nil {
			input += *ev.InputTokens
		}
		if ev.OutputTokens != nil {
			output += *ev.OutputTokens
		}
		if ev.ReasoningTokens != nil {
			reasoning += *ev.ReasoningTokens
		}
		if ev.ContextAfter != nil || ev.CumulativeTotal != nil || ev.TotalTokens != nil {
			t.Fatalf("v2 无 step-finish 快照，必须为未知而不是 0: %+v", ev)
		}
	}
	// 两条 assistant 消息的增量：232+44487 / 44+1000 / 0+500
	if reqs != 2 || input != 44719 || output != 1044 || reasoning != 500 {
		t.Fatalf("增量聚合 reqs=%d input=%d output=%d reasoning=%d", reqs, input, output, reasoning)
	}
	// 会话级 tokens_*（5000/3000）不得混入时间线增量
	if input == 5000 || input == 5000+44719 {
		t.Fatalf("会话累计与消息增量疑似相加: input=%d", input)
	}
}

// TestLegacyTimelineUnchanged 保证传统路径事件与上下文快照行为不变（任务 3.6）。
func TestLegacyTimelineUnchanged(t *testing.T) {
	path := createFixtureDB(t)
	db := openTestDB(t, path)
	// 补充 step-finish part：传统路径的上下文快照来源
	execAll(t, db, []string{
		`INSERT INTO part (id, message_id, session_id, time_created, time_updated, data)
		 VALUES ('prt_003', 'msg_002', 'ses_0001', 1785740001000, 1785740001000,
		  '{"type":"step-finish","reason":"stop",
		    "tokens":{"total":150000,"input":120000,"output":30000}}')`,
	})

	tlIt, err := New().IterateUsageEvents(context.Background(), testSession(path, "ses_0001"))
	events := collectEvents(t, tlIt, err)
	if len(events) != 2 {
		t.Fatalf("事件数=%d: %+v", len(events), eventIDs(events))
	}

	user := events[0]
	if user.EventType != model.UsageEventUserMessage ||
		user.SourceIdentity != "opencode-msg:msg_001" ||
		user.UserPromptPreview != "请查看还有哪些未完成的任务" {
		t.Fatalf("传统 user 事件=%+v", user)
	}

	req := events[1]
	if req.EventType != model.UsageEventRequest ||
		req.SourceIdentity != "opencode-part:prt_003" ||
		req.InputTokens == nil || *req.InputTokens != 120000 ||
		req.TotalTokens == nil || *req.TotalTokens != 150000 ||
		req.ContextAfter == nil || *req.ContextAfter != 150000 ||
		req.CumulativeTotal == nil || *req.CumulativeTotal != 150000 {
		t.Fatalf("传统 request 事件=%+v", req)
	}
}

// eventIDs 返回全部事件 ID，用于重复性断言。
func eventIDs(events []*model.UsageTimelineEvent) []string {
	ids := make([]string, 0, len(events))
	for _, ev := range events {
		ids = append(ids, ev.EventID)
	}
	return ids
}
