// Package web 提供本地只读 Web 视图。
//
// 服务只在 localhost 监听，仅提供只读查询（列表/详情/时间线/标签），
// 不包含任何写操作，不启用遥测或外部访问。
package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
	"github.com/talea/talea/internal/model"
	"github.com/talea/talea/internal/search"
	"github.com/talea/talea/internal/tags"
	"github.com/talea/talea/internal/timeline"
	"github.com/talea/talea/internal/usage"
)

// Server 是本地只读 Web 服务。
type Server struct {
	App *app.App
	DB  *index.DB
}

// Handler 返回 HTTP handler。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session", s.handleSession)
	mux.HandleFunc("/api/timeline", s.handleTimeline)
	mux.HandleFunc("/api/tags", s.handleTags)
	return mux
}

// Listen 监听 localhost 并服务，返回地址。
func (s *Server) Listen(ctx context.Context, port int) (net.Listener, error) {
	addr := "127.0.0.1:" + strconv.Itoa(port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()
	return ln, nil
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(pageHTML))
}

// sessionListItem 是列表项的 JSON 结构。
type sessionListItem struct {
	Agent          string `json:"agent"`
	SessionID      string `json:"session_id"`
	FirstQuestion  string `json:"first_question"`
	StartedAt      string `json:"started_at"`
	LastActivityAt string `json:"last_activity_at"`
	Duration       string `json:"duration"`
	WorkingDir     string `json:"working_directory"`
	Tokens         string `json:"tokens"`
	Activity       string `json:"activity"`
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := search.Query{Limit: 200}
	if v := r.URL.Query().Get("agent"); v != "" {
		q.Agent = v
	}
	if v := r.URL.Query().Get("q"); v != "" {
		q.Term = v
	}
	results, err := search.List(ctx, s.DB, q)
	if err != nil {
		writeJSONErr(w, err)
		return
	}
	var items []sessionListItem
	for _, res := range results {
		s := res.Session
		items = append(items, sessionListItem{
			Agent:          string(s.AgentID),
			SessionID:      s.SessionID,
			FirstQuestion:  truncateRunes(s.FirstQuestion, 160),
			StartedAt:      fmtTime(s.StartedAt),
			LastActivityAt: fmtTime(s.LastActivityAt),
			Duration:       durText(s.Duration),
			WorkingDir:     s.WorkingDirectory,
			Tokens:         tokenText(s.TokenUsage),
			Activity:       string(s.Activity),
		})
	}
	writeJSON(w, items)
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONErr(w, fmt.Errorf("缺少 id 参数"))
		return
	}
	results, err := search.ByIDPrefix(ctx, s.DB, id, "", 5)
	if err != nil {
		writeJSONErr(w, err)
		return
	}
	var sess *model.Session
	if len(results) > 0 {
		sess = &results[0].Session
	}
	if sess == nil {
		writeJSONErr(w, fmt.Errorf("未找到会话"))
		return
	}
	u, _ := usage.Load(ctx, s.DB, sess.AgentInstanceID, sess.SessionID)
	m, _ := tags.Get(ctx, s.DB, sess.AgentInstanceID, sess.SessionID)

	detail := map[string]any{
		"session":  sess,
		"usage":    u,
		"tags":     m.Tags,
		"favorite": m.Favorite,
		"note":     m.Note,
	}
	writeJSON(w, detail)
}

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSONErr(w, fmt.Errorf("缺少 id 参数"))
		return
	}
	results, err := search.ByIDPrefix(ctx, s.DB, id, "", 5)
	if err != nil {
		writeJSONErr(w, err)
		return
	}
	var sess *model.Session
	if len(results) > 0 {
		sess = &results[0].Session
	}
	if sess == nil {
		writeJSONErr(w, fmt.Errorf("未找到会话"))
		return
	}
	events, err := timeline.List(ctx, s.DB, timeline.Query{
		AgentInstanceID: sess.AgentInstanceID,
		SessionID:       sess.SessionID,
		Limit:           500,
	})
	if err != nil {
		writeJSONErr(w, err)
		return
	}
	writeJSON(w, events)
}

func (s *Server) handleTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	favs, _ := tags.Favorites(ctx, s.DB)
	writeJSON(w, map[string]any{"favorites": favs})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_ = json.NewEncoder(w).Encode(v)
}

func writeJSONErr(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusBadRequest)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
}

func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02 15:04")
}

func durText(d *time.Duration) string {
	if d == nil {
		return ""
	}
	return d.Round(time.Second).String()
}

func tokenText(u *model.TokenUsage) string {
	if u == nil || u.TotalTokens == nil {
		return ""
	}
	n := *u.TotalTokens
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

const pageHTML = `<!DOCTYPE html>
<html lang="zh">
<head>
<meta charset="utf-8">
<title>Talea · 会话</title>
<style>
body { font-family: monospace; margin: 2rem; background:#111; color:#ddd; }
table { border-collapse: collapse; width: 100%; table-layout: auto; }
th, td { text-align: left; padding: 6px 10px; border-bottom: 1px solid #333; }
td.nowrap { white-space: nowrap; }
td.q { white-space: normal; max-width: 480px; }
a { color: #7c9; text-decoration: none; }
input { background:#222; color:#ddd; border:1px solid #555; padding:6px; }
h1 { color:#e8e; }
</style>
</head>
<body>
<h1>Talea · Agent Sessions</h1>
<form method="get">
  <input type="text" name="q" placeholder="搜索关键词">
  <input type="submit" value="搜索">
</form>
<div id="sessions">加载中…</div>
<script>
const params = new URLSearchParams(location.search);
const q = params.get('q') || '';
async function load() {
  const url = '/api/sessions' + (q ? '?q=' + encodeURIComponent(q) : '');
  const res = await fetch(url);
  const items = await res.json();
  if (!items.length) { document.getElementById('sessions').textContent = '无会话'; return; }
  let html = '<table><tr><th>Agent</th><th>开始</th><th>时长</th><th>Token</th><th>目录</th><th>首次提问</th></tr>';
  for (const it of items) {
    const href = '/api/session?id=' + encodeURIComponent(it.session_id);
    html += '<tr><td class="nowrap">' + it.agent + '</td><td class="nowrap">' + it.started_at + '</td><td class="nowrap">' + it.duration +
      '</td><td class="nowrap">' + it.tokens + '</td><td class="nowrap">' + it.working_directory + '</td><td class="q"><a href="' + href + '">' + it.first_question + '</a></td></tr>';
  }
  html += '</table>';
  document.getElementById('sessions').innerHTML = html;
}
load();
</script>
</body>
</html>`
