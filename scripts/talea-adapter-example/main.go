// Command talea-adapter-example 是一个外部适配器示例实现。
//
// 实现 talea 外部适配器协议（JSON Lines over stdio）：
//
//	{"method":"info"}      -> 返回 AdapterInfo
//	{"method":"detect"}    -> 返回 AgentInstance 列表
//	{"method":"discover"}  -> 返回 SessionSource 列表
//
// 用法：编译后放入 PATH，命名为 talea-adapter-<name>。
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
}

type response struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result,omitempty"`
	Error  string `json:"error,omitempty"`
}

func main() {
	sc := bufio.NewScanner(os.Stdin)
	sc.Buffer(make([]byte, 64*1024), 16*1024*1024)
	w := bufio.NewWriter(os.Stdout)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var req request
		if err := json.Unmarshal(line, &req); err != nil {
			write(w, response{OK: false, Error: "invalid request: " + err.Error()})
			continue
		}
		switch req.Method {
		case "info":
			write(w, response{OK: true, Result: map[string]any{
				"id":           "example-plugin",
				"display_name": "Example Plugin",
				"capabilities": []string{"discover_sessions"},
			}})
		case "detect":
			write(w, response{OK: true, Result: []map[string]any{}})
		case "discover":
			// 扫描 $EXAMPLE_PLUGIN_DIR 下的 *.jsonl
			dir := os.Getenv("EXAMPLE_PLUGIN_DIR")
			var sources []map[string]any
			if dir != "" {
				entries, err := os.ReadDir(dir)
				if err == nil {
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
				}
			}
			write(w, response{OK: true, Result: sources})
		default:
			write(w, response{OK: false, Error: fmt.Sprintf("unknown method: %s", req.Method)})
		}
	}
}

func write(w *bufio.Writer, resp response) {
	b, err := json.Marshal(resp)
	if err != nil {
		return
	}
	_, _ = w.Write(append(b, '\n'))
	_ = w.Flush()
}
