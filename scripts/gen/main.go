// Command gen 生成性能测试夹具（会话 / 事件）。
//
// 用法：
//
//	go run ./scripts/gen -sessions 1000 -dir /tmp/talea-bench
//	go run ./scripts/gen -sessions 10000 -dir /tmp/talea-bench
//	go run ./scripts/gen -events 100000 -dir /tmp/talea-bench
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"time"
)

func main() {
	sessions := flag.Int("sessions", 0, "生成的会话数（0 跳过）")
	events := flag.Int("events", 0, "单个会话的事件数（生成 events 夹具，0 跳过）")
	dir := flag.String("dir", "/tmp/talea-bench", "输出目录")
	flag.Parse()

	if err := os.MkdirAll(*dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *sessions > 0 {
		if err := genSessions(*dir, *sessions); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
	if *events > 0 {
		if err := genEvents(*dir, *events); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	}
}

// genSessions 生成 N 个 Claude 格式会话文件。
func genSessions(dir string, n int) error {
	projectDir := filepath.Join(dir, "projects", "-bench-proj")
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		return err
	}
	base := time.Now()
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("bench-%08d-%s", i, randHex(8))
		path := filepath.Join(projectDir, id+".jsonl")
		var lines []map[string]any
		start := base.Add(-time.Duration(i) * time.Hour)
		lines = append(lines, map[string]any{
			"type": "user", "timestamp": start.Format(time.RFC3339),
			"cwd": "/home/bench/proj", "sessionId": id, "gitBranch": "bench",
			"message": map[string]any{"role": "user",
				"content": fmt.Sprintf("基准测试会话 %d 的问题：分析磁盘残留原因", i)},
		})
		for j := 0; j < 10; j++ {
			ts := start.Add(time.Duration(j) * time.Minute)
			lines = append(lines, map[string]any{
				"type": "assistant", "timestamp": ts.Format(time.RFC3339),
				"cwd": "/home/bench/proj", "sessionId": id, "gitBranch": "bench",
				"message": map[string]any{"role": "assistant",
					"content": []map[string]any{{"type": "text", "text": "回复 " + fmt.Sprint(j)}},
					"usage":   map[string]any{"input_tokens": 5000 + j, "output_tokens": 200 + j}},
			})
		}
		if err := writeJSONL(path, lines); err != nil {
			return err
		}
	}
	fmt.Printf("生成 %d 个会话 -> %s\n", n, projectDir)
	return nil
}

// genEvents 生成单个超大会话（OpenCode 风格，含 n 个 step-finish 事件）。
// 输出到 <dir>/opencode-bench/session.jsonl，可直接被 generic 适配器发现。
func genEvents(dir string, n int) error {
	outDir := filepath.Join(dir, "opencode-bench")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(outDir, "big-session.jsonl")
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	base := time.Now().Add(-time.Duration(n) * time.Second)
	enc := json.NewEncoder(f)
	// 首条 user 消息
	userLine := map[string]any{
		"type": "user", "timestamp": base.Format(time.RFC3339Nano),
		"cwd": "/home/bench/big", "sessionId": "big-session",
		"message": map[string]any{"role": "user", "content": "超大会话基准测试"},
	}
	if err := enc.Encode(userLine); err != nil {
		return err
	}
	// n 条 assistant + step-finish 事件
	var cumulative int64
	for i := 0; i < n; i++ {
		ts := base.Add(time.Duration(i) * time.Second)
		cumulative += int64(i%1000) + 100
		assistant := map[string]any{
			"type": "assistant", "timestamp": ts.Format(time.RFC3339Nano),
			"cwd": "/home/bench/big", "sessionId": "big-session",
			"message": map[string]any{"role": "assistant",
				"content": []map[string]any{{"type": "text", "text": "回复 " + fmt.Sprint(i)}},
				"usage":   map[string]any{"input_tokens": cumulative, "output_tokens": int64(i%50) + 10}},
		}
		if err := enc.Encode(assistant); err != nil {
			return err
		}
	}
	fmt.Printf("生成 1 个超大会话（%d 事件）-> %s\n", n, path)
	return nil
}

func writeJSONL(path string, lines []map[string]any) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, l := range lines {
		if err := enc.Encode(l); err != nil {
			return err
		}
	}
	return nil
}

// randHex 生成 n 位随机十六进制串。
func randHex(n int) string {
	const hex = "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = hex[rand.IntN(16)]
	}
	return string(b)
}
