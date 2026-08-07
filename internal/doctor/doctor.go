// Package doctor 实现环境诊断。
package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
			if runtime.GOOS == "windows" {
				// Windows 无 Unix 权限位语义，跳过权限检查
				rep.addOK("索引权限", "Windows（跳过）")
			} else {
				mode := fi.Mode().Perm()
				if mode&0o077 == 0 {
					rep.addOK("索引权限", fmt.Sprintf("0600 (%s)", mode.String()))
				} else {
					rep.addWarn("索引权限", fmt.Sprintf("非 0600：%s", mode.String()))
				}
			}
		}
	}

	for _, ad := range a.Registry.All() {
		info := ad.Info()
		if agentFilter != "" && string(info.ID) != agentFilter {
			continue
		}
		// generic 为模板适配器，无默认安装，跳过诊断
		if string(info.ID) == "generic-jsonl" {
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
			checkIndexHealth(ctx, db, &rep)
			db.Close()
		}
	}

	// 配置与路径映射检查
	checkConfig(ctx, a, &rep)

	sort.Slice(rep.Checks, func(i, j int) bool {
		if rep.Checks[i].Agent != rep.Checks[j].Agent {
			return rep.Checks[i].Agent < rep.Checks[j].Agent
		}
		return rep.Checks[i].Text < rep.Checks[j].Text
	})
	return rep, nil
}

// checkIndexHealth 检查索引完整性：未知格式、不完整 usage、损坏。
func checkIndexHealth(ctx context.Context, db *index.DB, rep *Report) {
	// 未知格式
	var unknownFormat int
	err := db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE format_name IS NULL OR format_name = ''`).Scan(&unknownFormat)
	if err == nil && unknownFormat > 0 {
		rep.addWarn("索引格式", fmt.Sprintf("%d 个会话格式未知", unknownFormat))
	} else if err == nil {
		rep.addOK("索引格式", "全部已识别")
	}

	// 不完整 usage
	var partialUsage int
	err = db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM session_usage WHERE completeness = 'partial' OR completeness = 'unknown'`).Scan(&partialUsage)
	if err == nil && partialUsage > 0 {
		rep.addWarn("Token 完整性", fmt.Sprintf("%d 个会话 Token 数据不完整", partialUsage))
	}

	// 子 Agent 会话（应默认隐藏）
	var subagents int
	err = db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sessions WHERE is_subagent = 1`).Scan(&subagents)
	if err == nil && subagents > 0 {
		rep.addOK("子 Agent 会话", fmt.Sprintf("%d 个（默认隐藏）", subagents))
	}

	// FTS 完整性
	var ftsCount int
	err = db.SQL().QueryRowContext(ctx,
		`SELECT COUNT(*) FROM session_fts`).Scan(&ftsCount)
	if err == nil {
		rep.addOK("FTS 索引", fmt.Sprintf("%d 条", ftsCount))
	} else {
		rep.addWarn("FTS 索引", "未初始化，请执行 talea index")
	}
}

// checkConfig 检查配置与路径映射冲突。
func checkConfig(ctx context.Context, a *app.App, rep *Report) {
	// 路径映射前缀冲突：一个源前缀是另一个的严格前缀，映射结果不同则告警
	keys := make([]string, 0, len(a.Config.PathMapping))
	for k := range a.Config.PathMapping {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	conflict := false
	for i := 0; i < len(keys); i++ {
		for j := i + 1; j < len(keys); j++ {
			if strings.HasPrefix(keys[j], keys[i]+"/") {
				conflict = true
			}
		}
	}
	if conflict {
		rep.addWarn("路径映射", "检测到前缀重叠的路径映射，可能产生意外替换")
	} else if len(keys) > 0 {
		rep.addOK("路径映射", fmt.Sprintf("%d 条，无冲突", len(keys)))
	}

	// 配置存在性
	if _, err := os.Stat(a.Paths.ConfigPath); os.IsNotExist(err) {
		rep.addOK("配置", "使用默认值（未生成配置文件）")
	} else {
		rep.addOK("配置", a.Paths.ConfigPath)
	}
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
