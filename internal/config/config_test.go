package config

import (
	"encoding/base64"
	"path/filepath"
	"testing"
)

func TestLoadConfigUsesDefaults(t *testing.T) {
	t.Setenv("HOME", "/tmp/myclaw-home")
	t.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTPAddr != ":8080" {
		t.Fatalf("HTTPAddr = %q", cfg.HTTPAddr)
	}
	if cfg.DataDir != "/tmp/myclaw-home/.myclaw" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SQLitePath != "/tmp/myclaw-home/.myclaw/myclaw.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
	if cfg.LogLevel != "info" {
		t.Fatalf("LogLevel = %q", cfg.LogLevel)
	}
	if len(cfg.ChannelMasterKey) != 32 {
		t.Fatalf("ChannelMasterKey length = %d", len(cfg.ChannelMasterKey))
	}
}

func TestLoadConfigUsesExplicitDataDirForDefaultSQLitePath(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("CHANNEL_DATA_DIR", "/var/lib/myclaw")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.DataDir != "/var/lib/myclaw" {
		t.Fatalf("DataDir = %q", cfg.DataDir)
	}
	if cfg.SQLitePath != "/var/lib/myclaw/myclaw.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
}

func TestLoadConfigUsesExplicitSQLitePathWhenSet(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("CHANNEL_DATA_DIR", "/var/lib/myclaw")
	t.Setenv("CHANNEL_SQLITE_PATH", "/custom/channel.db")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.SQLitePath != "/custom/channel.db" {
		t.Fatalf("SQLitePath = %q", cfg.SQLitePath)
	}
}

func TestBotWorkspacePathUsesBotScopedWorkspaceUnderDataDir(t *testing.T) {
	cfg := Config{DataDir: "/var/lib/myclaw"}

	got := cfg.BotWorkspacePath("bot_123")

	want := filepath.Join("/var/lib/myclaw", "bots", "bot_123", "workspace")
	if got != want {
		t.Fatalf("BotWorkspacePath() = %q, want %q", got, want)
	}
}

func TestLoadConfigRequiresMasterKey(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestLoadConfigRejectsInvalidMasterKeyLength(t *testing.T) {
	t.Setenv("CHANNEL_MASTER_KEY", base64.StdEncoding.EncodeToString([]byte("short")))

	_, err := Load()
	if err == nil {
		t.Fatal("expected invalid key length error")
	}
}
