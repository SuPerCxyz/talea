// Package config 负责 TOML 配置的加载、默认值与校验。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

// Default 返回带默认值的配置。
func Default() *Config {
	return &Config{
		General: General{
			DefaultSort:        "last_activity",
			IncludeSubagents:   false,
			ShowSystemMessages: false,
			TimeFormat:         "2006-01-02 15:04",
			Timezone:           "local",
		},
		Index: Index{
			MetadataOnly:           false,
			IndexAssistantMessages: true,
			IndexToolOutput:        false,
			MaxMessageBytes:        1048576,
			MaxToolOutputBytes:     262144,
		},
		Agents: map[string]AgentConfig{
			"claude-code": {Enabled: true},
			"codex-cli":   {Enabled: true},
			"opencode":    {Enabled: true},
		},
		Search: Search{
			MaxResults:          200,
			PreviewMessageLimit: 20,
		},
		Privacy: Privacy{
			RedactSecretsInPreview: true,
			RedactSecretsInExport:  false,
		},
		PathMapping: map[string]string{},
	}
}

type Config struct {
	General     General                `toml:"general"`
	Index       Index                  `toml:"index"`
	Agents      map[string]AgentConfig `toml:"agents"`
	Search      Search                 `toml:"search"`
	Privacy     Privacy                `toml:"privacy"`
	PathMapping map[string]string      `toml:"path_mapping"`
}

type General struct {
	DefaultSort        string `toml:"default_sort"`
	IncludeSubagents   bool   `toml:"include_subagents"`
	ShowSystemMessages bool   `toml:"show_system_messages"`
	TimeFormat         string `toml:"time_format"`
	Timezone           string `toml:"timezone"`
}

type Index struct {
	MetadataOnly           bool  `toml:"metadata_only"`
	IndexAssistantMessages bool  `toml:"index_assistant_messages"`
	IndexToolOutput        bool  `toml:"index_tool_output"`
	MaxMessageBytes        int64 `toml:"max_message_bytes"`
	MaxToolOutputBytes     int64 `toml:"max_tool_output_bytes"`
}

type AgentConfig struct {
	Enabled  bool     `toml:"enabled"`
	DataDirs []string `toml:"data_dirs"`
}

type Search struct {
	MaxResults          int `toml:"max_results"`
	PreviewMessageLimit int `toml:"preview_message_limit"`
}

type Privacy struct {
	RedactSecretsInPreview bool `toml:"redact_secrets_in_preview"`
	RedactSecretsInExport  bool `toml:"redact_secrets_in_export"`
}

// Load 从 path 加载配置，文件不存在时返回默认配置。
func Load(path string) (*Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return nil, fmt.Errorf("读取配置 %s: %w", path, err)
	}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("解析配置 %s: %w", path, err)
	}
	return cfg, nil
}

// Paths 返回 Talea 各类目录路径。
type Paths struct {
	ConfigDir  string
	ConfigPath string
	DataDir    string
	DBPath     string
	CacheDir   string
}

// ResolvePaths 基于 XDG 环境变量计算路径。
// 遵循 XDG Base Directory 规范：XDG_* 为绝对路径时直接使用，
// 否则拼接到 HOME 下。
func ResolvePaths() Paths {
	home := homeDir()
	configHome := xdgHome("XDG_CONFIG_HOME", home, ".config")
	dataHome := xdgHome("XDG_DATA_HOME", home, ".local/share")
	cacheHome := xdgHome("XDG_CACHE_HOME", home, ".cache")

	p := Paths{
		ConfigDir: filepath.Join(configHome, "talea"),
		DataDir:   filepath.Join(dataHome, "talea"),
		CacheDir:  filepath.Join(cacheHome, "talea"),
	}
	p.ConfigPath = filepath.Join(p.ConfigDir, "config.toml")
	p.DBPath = filepath.Join(p.DataDir, "index.db")
	return p
}

// xdgHome 解析单个 XDG 目录：绝对路径直接用，相对路径拼到 home。
func xdgHome(env, home, def string) string {
	if v := os.Getenv(env); v != "" {
		if filepath.IsAbs(v) {
			return v
		}
		return filepath.Join(home, v)
	}
	return filepath.Join(home, def)
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil && h != "" {
		return h
	}
	// UserHomeDir 失败时回退到 HOME 环境变量（覆盖大多数场景）
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	// 最后回退到当前目录，避免产生字面 "~" 路径
	if wd, err := os.Getwd(); err == nil {
		return wd
	}
	return "."
}
