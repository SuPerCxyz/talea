// Package doctor 实现环境诊断。
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/talea/talea/internal/adapters"
	"github.com/talea/talea/internal/app"
	"github.com/talea/talea/internal/index"
)

// Level 诊断级别。
type Level string

const (
	LevelOK    Level = "OK"
	LevelWarn  Level = "WARN"
	LevelError Level = "ERROR"
)

// Check 是一条诊断结果。
type Check struct {
	Agent string `json:"agent,omitempty"`
	Level Level  `json:"level"`
	Text  string `json:"text"`
}

// Report 诊断报告。
type Report struct {
	Checks []Check `json:"checks"`
}

// Run 执行诊断。
func Run(ctx context.Context, a *app.App, agentFilter string) (Report, error) {
	var rep Report
	rep.addOK("Talea 索引", a.Paths.DBPath)

	if _, err := os.Stat(a.Paths.DBPath); err == nil {
		fi, err := os.Stat(a.Paths.DBPath)
		if err == nil {
			mode := fi.Mode().Perm()
			if mode&0o077 == 0 {
				rep.addOK("索引权限", fmt.Sprintf("0600 (%s)", mode.String()))
			} else {
				rep.addWarn("索引权限", fmt.Sprintf("非 0600：%s", mode.String()))
			}
		}
	}

	for _, ad := range a.Registry.All() {
		info := ad.Info()
		if agentFilter != "" && string(info.ID) != agentFilter {
			continue
		}
		insts, err := ad.Detect(ctx)
		if err != nil {
			rep.addError(info.DisplayName, err.Error())
			continue
		}
		if len(insts) == 0 {
			rep.addWarn(info.DisplayName, "未检测到安装")
			continue
		}
		for _, inst := range insts {
			rep.addOK(fmt.Sprintf("%s 可执行文件", info.DisplayName), inst.ExecutablePath)
			if inst.Version != "" {
				rep.addOK(fmt.Sprintf("%s 版本", info.DisplayName), inst.Version)
			} else {
				rep.addWarn(fmt.Sprintf("%s 版本", info.DisplayName), "无法获取")
			}
			sources, err := ad.Discover(ctx, inst)
			if err != nil {
				rep.addWarn(fmt.Sprintf("%s 数据目录", info.DisplayName), err.Error())
				continue
			}
			rep.addOK(fmt.Sprintf("%s 会话发现", info.DisplayName),
				fmt.Sprintf("%s 目录 %d 个会话", inst.DataDirectory, len(sources)))
		}
	}

	// 索引状态
	if _, err := os.Stat(a.Paths.DBPath); err == nil {
		db, err := index.Open(a.Paths.DBPath)
		if err == nil {
			n, _ := db.Count(ctx)
			rep.addOK("索引会话数", fmt.Sprintf("%d 个", n))
			db.Close()
		}
	}

	sort.Slice(rep.Checks, func(i, j int) bool {
		if rep.Checks[i].Agent != rep.Checks[j].Agent {
			return rep.Checks[i].Agent < rep.Checks[j].Agent
		}
		return rep.Checks[i].Text < rep.Checks[j].Text
	})
	return rep, nil
}

func (r *Report) addOK(agent, text string) {
	r.Checks = append(r.Checks, Check{Agent: agent, Level: LevelOK, Text: text})
}

func (r *Report) addWarn(agent, text string) {
	r.Checks = append(r.Checks, Check{Agent: agent, Level: LevelWarn, Text: text})
}

func (r *Report) addError(agent, text string) {
	r.Checks = append(r.Checks, Check{Agent: agent, Level: LevelError, Text: text})
}

// JSON 输出报告。
func (r Report) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// Print 打印人类可读报告。
func (r Report) Print() {
	for _, c := range r.Checks {
		prefix := "[OK]"
		switch c.Level {
		case LevelWarn:
			prefix = "[WARN]"
		case LevelError:
			prefix = "[ERROR]"
		}
		if c.Agent != "" {
			fmt.Printf("%s %s: %s\n", prefix, c.Agent, c.Text)
		} else {
			fmt.Printf("%s %s\n", prefix, c.Text)
		}
	}
}

var _ = adapters.Command{}
var _ = exec.Command
var _ = filepath.Join
var _ = strings.TrimSpace
