// Package plugin 支持外部适配器协议。
//
// 外部适配器是实现 `talea-adapter-<name>` 的可执行文件，通过标准输入输出
// 交换 JSON 完成发现/解析/读取。不加载不受信任的共享库。
//
// 协议（JSON Lines）：
//
//	请求: {"method":"info"} / {"method":"detect"} / {"method":"discover","instance":{...}}
//	响应: {"ok":true,"result":...} 或 {"ok":false,"error":"..."}
package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/model"
)

const prefix = "talea-adapter-"

// Client 管理一个外部适配器进程。
type Client struct {
	Name    string
	Path    string
	cmd     *exec.Cmd
	stdin   *bufio.Writer
	stdout  *bufio.Scanner
	started bool
}

// request 是协议请求。
type request struct {
	Method   string                       `json:"method"`
	Instance *model.AgentInstance         `json:"instance,omitempty"`
	Source   *adapters.SessionSource      `json:"source,omitempty"`
	Session  *model.Session               `json:"session,omitempty"`
	Options  *adapters.MessageLoadOptions `json:"options,omitempty"`
}

// response 是协议响应。
type response struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// DiscoverPlugins 扫描 PATH 与数据目录中的外部适配器。
func DiscoverPlugins() []string {
	var out []string
	seen := map[string]bool{}
	dirs := pathDirs()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasPrefix(e.Name(), prefix) {
				continue
			}
			full := filepath.Join(dir, e.Name())
			if seen[full] {
				continue
			}
			seen[full] = true
			out = append(out, full)
		}
	}
	return out
}

func pathDirs() []string {
	var dirs []string
	for _, d := range filepath.SplitList(os.Getenv("PATH")) {
		if d != "" {
			dirs = append(dirs, d)
		}
	}
	return dirs
}

// NewClient 创建外部适配器客户端。
func NewClient(path string) *Client {
	return &Client{Path: path}
}

// Start 启动外部适配器进程。
func (c *Client) Start(ctx context.Context) error {
	if c.started {
		return nil
	}
	cmd := exec.CommandContext(ctx, c.Path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动外部适配器 %s: %w", c.Path, err)
	}
	c.cmd = cmd
	c.stdin = bufio.NewWriter(stdin)
	c.stdout = bufio.NewScanner(stdout)
	c.stdout.Buffer(make([]byte, 64*1024), 16*1024*1024)
	c.started = true
	return nil
}

// call 发送请求并读取响应。
func (c *Client) call(method string, instance *model.AgentInstance, source *adapters.SessionSource) (json.RawMessage, error) {
	req := request{Method: method, Instance: instance, Source: source}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	if err := c.stdin.Flush(); err != nil {
		return nil, err
	}
	if !c.stdout.Scan() {
		if err := c.stdout.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("外部适配器 %s 无响应", c.Path)
	}
	var resp response
	if err := json.Unmarshal(c.stdout.Bytes(), &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("外部适配器 %s: %s", c.Path, resp.Error)
	}
	return resp.Result, nil
}

// Info 获取适配器信息。
func (c *Client) Info() (model.AdapterInfo, error) {
	raw, err := c.call("info", nil, nil)
	if err != nil {
		return model.AdapterInfo{}, err
	}
	var info model.AdapterInfo
	if err := json.Unmarshal(raw, &info); err != nil {
		return model.AdapterInfo{}, err
	}
	return info, nil
}

// Detect 探测实例。
func (c *Client) Detect() ([]model.AgentInstance, error) {
	raw, err := c.call("detect", nil, nil)
	if err != nil {
		return nil, err
	}
	var insts []model.AgentInstance
	if err := json.Unmarshal(raw, &insts); err != nil {
		return nil, err
	}
	return insts, nil
}

// Discover 发现会话。
func (c *Client) Discover(inst model.AgentInstance) ([]adapters.SessionSource, error) {
	raw, err := c.call("discover", &inst, nil)
	if err != nil {
		return nil, err
	}
	var sources []adapters.SessionSource
	if err := json.Unmarshal(raw, &sources); err != nil {
		return nil, err
	}
	return sources, nil
}

// ParseMetadata 解析会话元数据（完整字段）。
func (c *Client) ParseMetadata(inst model.AgentInstance, src adapters.SessionSource) (*model.Session, error) {
	raw, err := c.call("parse", &inst, &src)
	if err != nil {
		return nil, err
	}
	var s model.Session
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// callWith 发送携带 session/options 的请求。
func (c *Client) callWith(method string, session *model.Session, opts *adapters.MessageLoadOptions) (json.RawMessage, error) {
	req := request{Method: method, Session: session, Options: opts}
	line, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	return c.sendLine(line)
}

// sendLine 写入一行并读取响应。
func (c *Client) sendLine(line []byte) (json.RawMessage, error) {
	if _, err := c.stdin.Write(append(line, '\n')); err != nil {
		return nil, err
	}
	if err := c.stdin.Flush(); err != nil {
		return nil, err
	}
	if !c.stdout.Scan() {
		if err := c.stdout.Err(); err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("外部适配器 %s 无响应", c.Path)
	}
	var resp response
	if err := json.Unmarshal(c.stdout.Bytes(), &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("外部适配器 %s: %s", c.Path, resp.Error)
	}
	return resp.Result, nil
}

// LoadMessagesFull 读取消息预览（携带 session + options）。
func (c *Client) LoadMessagesFull(s model.Session, opts adapters.MessageLoadOptions) ([]adapters.Message, error) {
	raw, err := c.callWith("messages", &s, &opts)
	if err != nil {
		return nil, err
	}
	var msgs []adapters.Message
	if err := json.Unmarshal(raw, &msgs); err != nil {
		return nil, err
	}
	return msgs, nil
}

// LoadUsage 读取会话 Token 汇总。
func (c *Client) LoadUsage(s model.Session) (*model.TokenUsage, error) {
	raw, err := c.callWith("usage", &s, nil)
	if err != nil {
		return nil, err
	}
	var u model.TokenUsage
	if err := json.Unmarshal(raw, &u); err != nil {
		return nil, err
	}
	return &u, nil
}

// IterateUsageEvents 读取时间线事件。
func (c *Client) IterateUsageEvents(s model.Session) ([]*model.UsageTimelineEvent, error) {
	raw, err := c.callWith("timeline", &s, nil)
	if err != nil {
		return nil, err
	}
	var events []*model.UsageTimelineEvent
	if err := json.Unmarshal(raw, &events); err != nil {
		return nil, err
	}
	return events, nil
}

// Close 关闭进程。
func (c *Client) Close() error {
	if !c.started || c.cmd == nil {
		return nil
	}
	if c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
	}
	return c.cmd.Wait()
}

var _ = time.Now
