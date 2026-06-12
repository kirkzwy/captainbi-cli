package core

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/zalando/go-keyring"
)

const (
	AppName        = "captainbi"
	DefaultBaseURL = "https://openapi.captainbi.com"
	DefaultRate    = 20

	EnvClientID      = "CAPTAINBI_CLIENT_ID"
	EnvClientSecret  = "CAPTAINBI_CLIENT_SECRET"
	EnvBaseURL       = "CAPTAINBI_BASE_URL"
	EnvOpenChannelID = "CAPTAINBI_OPEN_CHANNEL_ID"
	EnvRateLimit     = "CAPTAINBI_RATE_LIMIT"
)

type Config struct {
	ClientID        string    `json:"client_id,omitempty"`
	BaseURL         string    `json:"base_url,omitempty"`
	OpenChannelID   string    `json:"open_channel_id,omitempty"`
	RateLimit       int       `json:"rate_limit,omitempty"`
	AccessToken     string    `json:"access_token,omitempty"`
	TokenType       string    `json:"token_type,omitempty"`
	TokenExpiry     time.Time `json:"token_expiry,omitempty"`
	UsePlainSecret  bool      `json:"use_plain_secret,omitempty"`
	PlainSecretHint string    `json:"plain_secret_hint,omitempty"`
}

func ConfigDir() (string, error) {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, AppName), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", AppName), nil
}

func ConfigPath() (string, error) {
	dir, err := ConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	cfg := &Config{BaseURL: DefaultBaseURL, RateLimit: DefaultRate}
	if b, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(b, cfg); err != nil {
			return nil, err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	ApplyEnv(cfg)
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.RateLimit <= 0 {
		cfg.RateLimit = DefaultRate
	}
	return cfg, nil
}

func SaveConfig(cfg *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o600)
}

func ApplyEnv(cfg *Config) {
	if v := os.Getenv(EnvClientID); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv(EnvBaseURL); v != "" {
		cfg.BaseURL = v
	}
	if v := os.Getenv(EnvOpenChannelID); v != "" {
		cfg.OpenChannelID = v
	}
	if v := os.Getenv(EnvRateLimit); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			cfg.RateLimit = n
		}
	}
}

func SaveClientSecret(clientID, secret string) error {
	if secret == "" || clientID == "" {
		return nil
	}
	return keyring.Set(AppName, clientID, secret)
}

func LoadClientSecret(cfg *Config) (string, error) {
	if v := os.Getenv(EnvClientSecret); v != "" {
		return v, nil
	}
	if cfg.ClientID == "" {
		return "", errors.New("client_id is not configured")
	}
	secret, err := keyring.Get(AppName, cfg.ClientID)
	if err != nil {
		return "", errors.New("client_secret is not available; run `cbi config init --client-secret-stdin` or set CAPTAINBI_CLIENT_SECRET")
	}
	return secret, nil
}

func DeleteClientSecret(clientID string) {
	if clientID != "" {
		_ = keyring.Delete(AppName, clientID)
	}
}
