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
			DefaultSort:         "last_activity",
			IncludeSubagents:    false,
			ShowSystemMessages:  false,
			TimeFormat:          "2006-01-02 15:04",
			Timezone:            "local",
		},
		Index: Index{
			MetadataOnly:        false,
			IndexAssistantMessages: true,
			IndexToolOutput:     false,
			MaxMessageBytes:     1048576,
			MaxToolOutputBytes:  262144,
		},
		Agents: map[string]AgentConfig{
			"claude-code": {Enabled: true},
			"codex-cli":   {Enabled: true},
			"opencode":    {Enabled: true},
		},
		Search: Search{
			MaxResults:           200,
			PreviewMessageLimit:  20,
		},
		Usage: Usage{
			Enabled:                  true,
			StoreRequestDetails:      true,
			EstimateCost:             false,
			IncludeSubagentsByDefault: false,
		},
		Privacy: Privacy{
			RedactSecretsInPreview: true,
			RedactSecretsInExport:  false,
		},
		PathMapping: map[string]string{},
	}
}

type Config struct {
	General     General                  `toml:"general"`
	Index       Index                    `toml:"index"`
	Agents      map[string]AgentConfig   `toml:"agents"`
	Search      Search                   `toml:"search"`
	Usage       Usage                    `toml:"usage"`
	Privacy     Privacy                  `toml:"privacy"`
	PathMapping map[string]string        `toml:"path_mapping"`
}

type General struct {
	DefaultSort        string `toml:"default_sort"`
	IncludeSubagents   bool   `toml:"include_subagents"`
	ShowSystemMessages bool   `toml:"show_system_messages"`
	TimeFormat         string `toml:"time_format"`
	Timezone           string `toml:"timezone"`
}

type Index struct {
	MetadataOnly            bool   `toml:"metadata_only"`
	IndexAssistantMessages  bool   `toml:"index_assistant_messages"`
	IndexToolOutput         bool   `toml:"index_tool_output"`
	MaxMessageBytes         int64  `toml:"max_message_bytes"`
	MaxToolOutputBytes      int64  `toml:"max_tool_output_bytes"`
}

type AgentConfig struct {
	Enabled  bool     `toml:"enabled"`
	DataDirs []string `toml:"data_dirs"`
}

type Search struct {
	MaxResults          int `toml:"max_results"`
	PreviewMessageLimit int `toml:"preview_message_limit"`
}

type Usage struct {
	Enabled                   bool   `toml:"enabled"`
	StoreRequestDetails       bool   `toml:"store_request_details"`
	EstimateCost              bool   `toml:"estimate_cost"`
	IncludeSubagentsByDefault bool   `toml:"include_subagents_by_default"`
	Pricing                   Pricing `toml:"pricing"`
}

type Pricing struct {
	CustomModel map[string]ModelPrice `toml:"custom-model"`
}

type ModelPrice struct {
	Currency           string  `toml:"currency"`
	InputPerMillion    float64 `toml:"input_per_million"`
	OutputPerMillion   float64 `toml:"output_per_million"`
	CacheReadPerMillion float64 `toml:"cache_read_per_million"`
	CacheWritePerMillion float64 `toml:"cache_write_per_million"`
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
func ResolvePaths() Paths {
	configHome := envOr("XDG_CONFIG_HOME", ".config")
	dataHome := envOr("XDG_DATA_HOME", ".local/share")
	cacheHome := envOr("XDG_CACHE_HOME", ".cache")
	home := homeDir()

	p := Paths{
		ConfigDir:  filepath.Join(home, configHome, "talea"),
		DataDir:    filepath.Join(home, dataHome, "talea"),
		CacheDir:   filepath.Join(home, cacheHome, "talea"),
	}
	p.ConfigPath = filepath.Join(p.ConfigDir, "config.toml")
	p.DBPath = filepath.Join(p.DataDir, "index.db")
	return p
}

func homeDir() string {
	if h, err := os.UserHomeDir(); err == nil {
		return h
	}
	return "~"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
