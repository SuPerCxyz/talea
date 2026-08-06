// Package output 提供表格/JSON/CSV/Markdown 输出。
package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"

	"github.com/talea/talea/internal/model"
)

// Format 输出格式。
type Format string

const (
	FormatTable    Format = "table"
	FormatJSON     Format = "json"
	FormatJSONL    Format = "jsonl"
	FormatCSV      Format = "csv"
	FormatMarkdown Format = "markdown"
)

// SessionView 是列表输出的扁平视图。
type SessionView struct {
	Agent            string `json:"agent"`
	AgentInstance    string `json:"agent_instance"`
	SessionID        string `json:"session_id"`
	FirstQuestion    string `json:"first_question"`
	StartedAt        string `json:"started_at"`
	EndedAt          string `json:"ended_at"`
	LastActivityAt   string `json:"last_activity_at"`
	Duration         string `json:"duration"`
	WorkingDirectory string `json:"working_directory"`
	WorkingDirExists bool   `json:"working_dir_exists"`
	GitBranch        string `json:"git_branch"`
	ProjectName      string `json:"project_name"`
	Tokens           string `json:"tokens"`
	HasTokenUsage    bool   `json:"has_token_usage"`
	IsSubagent       bool   `json:"is_subagent"`
	Activity         string `json:"activity"`
}

// ViewOf 构造会话视图。
func ViewOf(s *model.Session) SessionView {
	return SessionView{
		Agent:            string(s.AgentID),
		AgentInstance:    s.AgentInstanceID,
		SessionID:        s.SessionID,
		FirstQuestion:    s.FirstQuestion,
		StartedAt:        fmtTime(s.StartedAt),
		EndedAt:          fmtTime(s.EndedAt),
		LastActivityAt:   fmtTime(s.LastActivityAt),
		Duration:         FormatDuration(s.Duration),
		WorkingDirectory: s.WorkingDirectory,
		WorkingDirExists: s.WorkingDirExists,
		GitBranch:        s.GitBranch,
		ProjectName:      s.ProjectName,
		Tokens:           tokenString(s),
		HasTokenUsage:    s.HasTokenUsage,
		IsSubagent:       s.IsSubagent,
		Activity:         activityText(s.Activity),
	}
}

// FormatDuration 输出持续时长。
func FormatDuration(d *time.Duration) string {
	if d == nil {
		return "未知"
	}
	total := int64(*d / time.Second)
	if total < 60 {
		return fmt.Sprintf("%ds", total)
	}
	h := total / 3600
	m := (total % 3600) / 60
	s := total % 60
	if h >= 48 {
		return fmt.Sprintf("%dd %dh", h/24, h%24)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}

// FormatActivity 输出活动状态文本。
func FormatActivity(a model.ActivityState) string {
	return activityText(a)
}

func activityText(a model.ActivityState) string {
	switch a {
	case model.ActivityActive:
		return "进行中"
	case model.ActivityPossiblyActive:
		return "可能进行中"
	case model.ActivityInactive:
		return "已结束"
	default:
		return "未知"
	}
}

// FormatTokens 将 Token 数值转为可读文本。
func FormatTokens(u *model.TokenUsage) string {
	if u == nil {
		return "未知"
	}
	var total int64
	switch {
	case u.TotalTokens != nil:
		total = *u.TotalTokens
	case u.InputTokens != nil && u.OutputTokens != nil:
		total = *u.InputTokens + *u.OutputTokens
	case u.InputTokens != nil:
		total = *u.InputTokens
	case u.OutputTokens != nil:
		total = *u.OutputTokens
	default:
		return "未知"
	}
	return humanNumber(total)
}

func tokenString(s *model.Session) string {
	if s == nil || !s.HasTokenUsage || s.TokenUsage == nil {
		return "未知"
	}
	u := s.TokenUsage
	var total int64
	switch {
	case u.TotalTokens != nil:
		total = *u.TotalTokens
	case u.InputTokens != nil && u.OutputTokens != nil:
		total = *u.InputTokens + *u.OutputTokens
	case u.InputTokens != nil:
		total = *u.InputTokens
	case u.OutputTokens != nil:
		total = *u.OutputTokens
	default:
		return "未知"
	}
	return humanNumber(total)
}

func humanNumber(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.2fM", float64(n)/1_000_000)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1_000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("01-02 15:04")
}

// Write 输出会话列表。
func Write(w io.Writer, sessions []*model.Session, format Format) error {
	views := make([]SessionView, 0, len(sessions))
	for _, s := range sessions {
		views = append(views, ViewOf(s))
	}
	switch format {
	case FormatJSON:
		return writeJSON(w, views)
	case FormatJSONL:
		return writeJSONL(w, views)
	case FormatCSV:
		return writeCSV(w, views)
	case FormatMarkdown:
		return writeMarkdown(w, views)
	default:
		return writeTable(w, views)
	}
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func writeJSONL(w io.Writer, views []SessionView) error {
	enc := json.NewEncoder(w)
	for _, v := range views {
		if err := enc.Encode(v); err != nil {
			return err
		}
	}
	return nil
}

func writeCSV(w io.Writer, views []SessionView) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	header := []string{"agent", "session_id", "first_question", "started_at", "ended_at",
		"last_activity_at", "duration", "working_directory", "working_dir_exists",
		"git_branch", "tokens", "activity"}
	if err := cw.Write(header); err != nil {
		return err
	}
	for _, v := range views {
		row := []string{v.Agent, v.SessionID, v.FirstQuestion, v.StartedAt, v.EndedAt,
			v.LastActivityAt, v.Duration, v.WorkingDirectory, fmt.Sprintf("%v", v.WorkingDirExists),
			v.GitBranch, v.Tokens, v.Activity}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	return nil
}

func writeMarkdown(w io.Writer, views []SessionView) error {
	fmt.Fprintf(w, "| Agent | Session ID | 开始 | 结束 | Time | Token | 目录 | 首次提问 |\n")
	fmt.Fprintf(w, "|-------|------------|------|------|------|-------|------|---------|\n")
	for _, v := range views {
		fmt.Fprintf(w, "| %s | %s | %s | %s | %s | %s | %s | %s |\n",
			v.Agent, v.SessionID, v.StartedAt, v.EndedAt, v.Duration, v.Tokens, shortDir(v.WorkingDirectory), oneLine(v.FirstQuestion))
	}
	return nil
}

func writeTable(w io.Writer, views []SessionView) error {
	headers := []string{"Agent", "Session ID", "Start", "End", "Time", "Tokens", "CWD", "First Question"}
	n := len(headers)
	widths := make([]int, n)
	for i, h := range headers {
		widths[i] = runeLen(h)
	}
	// 除 First Question（最后一列）外，各列宽度取内容最大值，保证单行完整显示
	maxFirst := 0
	rows := make([][]string, len(views))
	for i, v := range views {
		row := []string{v.Agent, v.SessionID, v.StartedAt, v.EndedAt, v.Duration, v.Tokens,
			shortDir(v.WorkingDirectory), oneLine(v.FirstQuestion)}
		rows[i] = row
		for j := 0; j < n-1; j++ {
			if w := runeLen(row[j]); w > widths[j] {
				widths[j] = w
			}
		}
		if w := runeLen(row[n-1]); w > maxFirst {
			maxFirst = w
		}
	}
	// First Question 允许两行：宽度取内容与固定上限的较小值（上限 100）
	firstW := maxFirst
	if firstW > 100 {
		firstW = 100
	}
	widths[n-1] = firstW

	sep := make([]string, n)
	for i, h := range headers {
		sep[i] = strings.Repeat("-", max(widths[i], len(h)))
	}
	fmt.Fprintf(w, "%s\n", align(headers, widths))
	fmt.Fprintf(w, "%s\n", align(sep, widths))
	for _, row := range rows {
		lines := wrapFirst(row[n-1], firstW)
		for li, line := range lines {
			cur := make([]string, n)
			if li == 0 {
				copy(cur, row[:n-1])
			}
			cur[n-1] = line
			fmt.Fprintf(w, "%s\n", align(cur, widths))
		}
	}
	return nil
}

// wrapFirst 将单行文本按宽度换行，最多两行。
// 第一行占满宽度且不加省略号（内容续到第二行）；
// 仅当第二行仍有剩余时才在末尾加省略号。
func wrapFirst(s string, w int) []string {
	if s == "" {
		return []string{""}
	}
	if runeLen(s) <= w {
		return []string{s}
	}
	first := truncateAt(s, w)
	rest := restAfter(s, w)
	if rest == "" {
		return []string{first}
	}
	second := rest
	if runeLen(rest) > w {
		second = truncateAt(rest, w-1) + "…"
	}
	return []string{first, second}
}

// restAfter 返回按宽度截断 w 后剩余的字符串。
func restAfter(s string, w int) string {
	runes := []rune(s)
	width := 0
	for i, r := range runes {
		rw := runewidth.RuneWidth(r)
		if width+rw > w {
			return string(runes[i:])
		}
		width += rw
	}
	return ""
}

// truncateAt 截断字符串到指定显示宽度（不含省略号）。
func truncateAt(s string, w int) string {
	if runeLen(s) <= w {
		return s
	}
	var out []rune
	width := 0
	for _, rn := range s {
		if width+runewidth.RuneWidth(rn) > w {
			break
		}
		out = append(out, rn)
		width += runewidth.RuneWidth(rn)
	}
	return string(out)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func align(cells []string, widths []int) string {
	var sb strings.Builder
	for i, c := range cells {
		if i > 0 {
			sb.WriteString(" │ ")
		}
		sb.WriteString(pad(c, widths[i]))
	}
	return sb.String()
}

func pad(s string, w int) string {
	if runeLen(s) >= w {
		return truncate(s, w)
	}
	return s + strings.Repeat(" ", w-runeLen(s))
}

func runeLen(s string) int {
	return runewidth.StringWidth(s)
}

func truncate(s string, w int) string {
	r := []rune(s)
	if runewidth.StringWidth(s) <= w {
		return s
	}
	// 按显示宽度截断，保留 w-1 列 + 省略号
	var out []rune
	width := 0
	for _, rn := range r {
		if width+runewidth.RuneWidth(rn) > w-1 {
			break
		}
		out = append(out, rn)
		width += runewidth.RuneWidth(rn)
	}
	return string(out) + "…"
}

func shortDir(d string) string {
	if d == "" {
		return ""
	}
	if strings.HasPrefix(d, "/home/") {
		rest := strings.TrimPrefix(d, "/home/")
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 {
			return "~/" + parts[1]
		}
		return "~"
	}
	return d
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return strings.TrimSpace(s)
}
