package config

import (
	"path/filepath"
	"testing"
)

func TestResolvePathsAbsoluteXDG(t *testing.T) {
	t.Setenv("HOME", "/home/bench")
	t.Setenv("XDG_DATA_HOME", "/var/lib/talea-data")
	t.Setenv("XDG_CONFIG_HOME", "/etc/talea")
	t.Setenv("XDG_CACHE_HOME", "/var/cache/talea")

	p := ResolvePaths()
	if p.DataDir != "/var/lib/talea-data/talea" {
		t.Fatalf("DataDir=%q", p.DataDir)
	}
	if p.ConfigDir != "/etc/talea/talea" {
		t.Fatalf("ConfigDir=%q", p.ConfigDir)
	}
	if p.DBPath != "/var/lib/talea-data/talea/index.db" {
		t.Fatalf("DBPath=%q", p.DBPath)
	}
}

func TestResolvePathsRelativeXDG(t *testing.T) {
	t.Setenv("HOME", "/home/bench")
	t.Setenv("XDG_DATA_HOME", "data-x")
	t.Setenv("XDG_CONFIG_HOME", "conf-x")
	t.Setenv("XDG_CACHE_HOME", "cache-x")

	p := ResolvePaths()
	if p.DataDir != filepath.Join("/home/bench", "data-x", "talea") {
		t.Fatalf("DataDir=%q", p.DataDir)
	}
}

func TestResolvePathsDefaults(t *testing.T) {
	t.Setenv("HOME", "/home/bench")
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_CACHE_HOME", "")

	p := ResolvePaths()
	if p.DataDir != "/home/bench/.local/share/talea" {
		t.Fatalf("DataDir=%q", p.DataDir)
	}
	if p.ConfigDir != "/home/bench/.config/talea" {
		t.Fatalf("ConfigDir=%q", p.ConfigDir)
	}
}
