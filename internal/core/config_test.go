package core

import (
	"os"
	"testing"
)

func TestApplyEnv(t *testing.T) {
	t.Setenv(EnvClientID, "cid")
	t.Setenv(EnvBaseURL, "https://example.test")
	t.Setenv(EnvOpenChannelID, "ocid")
	t.Setenv(EnvRateLimit, "7")
	cfg := &Config{}
	ApplyEnv(cfg)
	if cfg.ClientID != "cid" || cfg.BaseURL != "https://example.test" || cfg.OpenChannelID != "ocid" || cfg.RateLimit != 7 {
		t.Fatalf("env not applied: %+v", cfg)
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
