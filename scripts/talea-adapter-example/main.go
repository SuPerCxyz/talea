// Command talea-adapter-example 是一个外部适配器示例实现。
//
// 实现 talea 外部适配器协议（JSON Lines over stdio）：
//
//	{"method":"info"}       -> 返回 AdapterInfo
//	{"method":"detect"}     -> 返回 AgentInstance 列表
//	{"method":"discover"}   -> 返回 SessionSource 列表
//	{"method":"parse"}      -> 解析会话元数据（完整字段）
//	{"method":"messages"}   -> 返回消息预览
//	{"method":"usage"}      -> 返回会话 Token 汇总
//	{"method":"timeline"}   -> 返回时间线事件
//
// 用法：编译后放入 PATH，命名为 talea-adapter-<name>。
// 数据源：$EXAMPLE_PLUGIN_DIR 目录下的 JSONL（Claude 风格结构）。
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type request struct {
	Method   string          `json:"method"`
	Instance json.RawMessage `json:"instance,omitempty"`
	Source   json.RawMessage `json:"source,omitempty"`
	Session  json.RawMessage `json:"session,omitempty"`
	Options  json.RawMessage `json:"options,omitempty"`
}

type response struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

type line struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Cwd       string          `json:"cwd"`
	SessionID string          `json:"sessionId"`
	Message   json.RawMessage `json:"message"`
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	w := bufio.NewWriter(os.Stdout)
	for sc.Scan() {
		lineBytes := sc.Bytes()
		if len(lineBytes) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(lineBytes, &req); err != nil {
			write(w, response{OK: false, Error: "invalid request: " + err.Error()})
			continue
		}
		switch req.Method {
		case "info":
			write(w, response{OK: true, Result: map[string]any{
				"id":           "example-plugin",
				"display_name": "Example Plugin",
				"capabilities": []string{
					"discover_sessions", "read_messages", "resume",
					"working_directory", "token_summary", "token_timeline",
				},
			}})
		case "detect":
			write(w, response{OK: true, Result: []map[string]any{}})
		case "discover":
			write(w, response{OK: true, Result: discoverSources()})
		case "parse":
			src := parseSource(req.Source)
			s := parseSession(src)
			write(w, response{OK: true, Result: s})
		case "messages":
			sess := parseSessionReq(req.Session)
			msgs := loadMessages(sess)
			write(w, response{OK: true, Result: msgs})
		case "usage":
			sess := parseSessionReq(req.Session)
			write(w, response{OK: true, Result: loadUsage(sess)})
		case "timeline":
			sess := parseSessionReq(req.Session)
			write(w, response{OK: true, Result: loadTimeline(sess)})
		default:
			write(w, response{OK: false, Error: fmt.Sprintf("unknown method: %s", req.Method)})
		}
	}
}

func discoverSources() []map[string]any {
	dir := os.Getenv("EXAMPLE_PLUGIN_DIR")
	var sources []map[string]any
	if dir == "" {
		return sources
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return sources
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, _ := e.Info()
		sources = append(sources, map[string]any{
			"session_id": strings.TrimSuffix(e.Name(), ".jsonl"),
			"path":       filepath.Join(dir, e.Name()),
			"source_id":  e.Name(),
			"mtime":      info.ModTime().Unix(),
			"size":       info.Size(),
		})
	}
	return sources
}

func parseSource(raw json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

func parseSession(src map[string]any) map[string]any {
	path, _ := src["path"].(string)
	sid, _ := src["session_id"].(string)
	s := map[string]any{
		"agent_id":                  "example-plugin",
		"agent_instance_id":         "example-default",
		"session_id":                sid,
		"format_name":               "example-jsonl",
		"source_path":               path,
		"source_id":                 src["source_id"],
		"first_question":            "示例插件会话 " + sid,
		"first_question_source":     "user_message",
		"first_question_confidence": 1.0,
		"activity_state":            "inactive",
		"working_directory":         exampleCwd(path),
		"working_dir_source":        "session_metadata",
	}
	return s
}

func exampleCwd(path string) string {
	if dir := os.Getenv("EXAMPLE_PLUGIN_CWD"); dir != "" {
		return dir
	}
	return filepath.Dir(path)
}

func parseSessionReq(raw json.RawMessage) map[string]any {
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return map[string]any{}
	}
	return m
}

// loadMessages 读取 JSONL 中 user/assistant 消息。
func loadMessages(sess map[string]any) []map[string]any {
	path, _ := sess["source_path"].(string)
	var msgs []map[string]any
	f, err := os.Open(path)
	if err != nil {
		return msgs
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue
		}
		if l.Type != "user" && l.Type != "assistant" {
			continue
		}
		msgs = append(msgs, map[string]any{
			"role":      l.Type,
			"content":   messageText(l.Message),
			"timestamp": 0,
		})
	}
	return msgs
}

func messageText(raw json.RawMessage) string {
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	var str string
	if err := json.Unmarshal(m.Content, &str); err == nil {
		return str
	}
	return ""
}

// loadUsage 汇总 assistant usage 的 input（累计）/ output（增量）。
func loadUsage(sess map[string]any) map[string]any {
	path, _ := sess["source_path"].(string)
	var input, output int64
	f, err := os.Open(path)
	if err != nil {
		return map[string]any{}
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue
		}
		if l.Type != "assistant" {
			continue
		}
		var m struct {
			Usage struct {
				Input  *int64 `json:"input_tokens"`
				Output *int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(l.Message, &m); err != nil {
			continue
		}
		if m.Usage.Input != nil && *m.Usage.Input > 0 {
			input = *m.Usage.Input // 累计
		}
		if m.Usage.Output != nil {
			output += *m.Usage.Output // 增量
		}
	}
	total := input + output
	return map[string]any{
		"input_tokens":  input,
		"output_tokens": output,
		"total_tokens":  total,
		"usage_source":  "message_metadata",
		"completeness":  "complete",
	}
}

// loadTimeline 生成简单的请求级事件。
func loadTimeline(sess map[string]any) []map[string]any {
	path, _ := sess["source_path"].(string)
	var events []map[string]any
	f, err := os.Open(path)
	if err != nil {
		return events
	}
	defer f.Close()
	seq := int64(0)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err != nil {
			continue
		}
		if l.Type != "assistant" {
			continue
		}
		var m struct {
			Usage struct {
				Input  *int64 `json:"input_tokens"`
				Output *int64 `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(l.Message, &m); err != nil {
			continue
		}
		events = append(events, map[string]any{
			"event_type":        "request",
			"sequence":          seq,
			"input_tokens":      m.Usage.Input,
			"output_tokens":     m.Usage.Output,
			"usage_source":      "message_metadata",
			"completeness":      "complete",
			"source_identity":   fmt.Sprintf("example-req-%d", seq),
			"agent_instance_id": "example-default",
			"session_id":        sess["session_id"],
		})
		seq++
	}
	return events
}

func write(w *bufio.Writer, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = w.Write(append(b, '\n'))
	_ = w.Flush()
}
