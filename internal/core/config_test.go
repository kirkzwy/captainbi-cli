package core

import (
	"os"
	"path/filepath"
	"testing"
)

func TestApplyEnv(t *testing.T) {
	t.Setenv(EnvClientID, "cid")
	t.Setenv(EnvAccessToken, "access-token")
	t.Setenv(EnvBaseURL, "https://example.test")
	t.Setenv(EnvOpenChannelID, "ocid")
	t.Setenv(EnvRateLimit, "7")
	cfg := &Config{}
	ApplyEnv(cfg)
	if cfg.ClientID != "cid" || cfg.BaseURL != "https://example.test" || cfg.OpenChannelID != "ocid" || cfg.RateLimit != 7 {
		t.Fatalf("env not applied: %+v", cfg)
	}
	if cfg.AccessToken != "access-token" || cfg.TokenType != "bearer" || cfg.TokenExpiry.IsZero() {
		t.Fatalf("access token env not applied: %+v", cfg)
	}
}

func TestConfigDirExplicitOverride(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "captainbi-state")
	t.Setenv(EnvConfigDir, dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(t.TempDir(), "ignored"))
	got, err := ConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	if got != dir {
		t.Fatalf("ConfigDir = %q, want %q", got, dir)
	}
	if err := CheckConfigDirWritable(); err != nil {
		t.Fatalf("override should be writable: %v", err)
	}
}

func TestSaveConfigAtomicRoundTrip(t *testing.T) {
	t.Setenv(EnvConfigDir, t.TempDir())
	want := &Config{ClientID: "client", BaseURL: "https://example.test", RateLimit: 20, Channels: map[string]string{"main": "channel"}}
	if err := SaveConfig(want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got.ClientID != want.ClientID || got.Channels["main"] != "channel" {
		t.Fatalf("round trip = %#v", got)
	}
	matches, err := filepath.Glob(filepath.Join(os.Getenv(EnvConfigDir), ".config-*.tmp"))
	if err != nil || len(matches) != 0 {
		t.Fatalf("temporary config files remain: %v %v", matches, err)
	}
}

func TestConfigPathUsesXDG(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	path, err := ConfigPath()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected path to be inside temp dir and not exist yet: %s", path)
	}
}

func TestLoadClientSecretFromFile(t *testing.T) {
	path := t.TempDir() + "/secret.txt"
	if err := os.WriteFile(path, []byte("  secret-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := LoadClientSecret(&Config{PlainSecretFile: path})
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret-value" {
		t.Fatalf("secret = %q", got)
	}
}
