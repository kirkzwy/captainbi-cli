package core

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/zalando/go-keyring"

	"github.com/kirkzwy/captainbi-cli/internal/lockfile"
)

const (
	AppName        = "captainbi"
	DefaultBaseURL = "https://openapi.captainbi.com"
	DefaultRate    = 250

	EnvClientID       = "CAPTAINBI_CLIENT_ID"
	EnvClientSecret   = "CAPTAINBI_CLIENT_SECRET"
	EnvAccessToken    = "CAPTAINBI_ACCESS_TOKEN"
	EnvBaseURL        = "CAPTAINBI_BASE_URL"
	EnvOpenChannelID  = "CAPTAINBI_OPEN_CHANNEL_ID"
	EnvRateLimit      = "CAPTAINBI_RATE_LIMIT"
	EnvConfigDir      = "CAPTAINBI_CONFIG_DIR"
	EnvWriteAllowlist = "CAPTAINBI_WRITE_ALLOWLIST"
)

type Config struct {
	ClientID        string            `json:"client_id,omitempty"`
	BaseURL         string            `json:"base_url,omitempty"`
	OpenChannelID   string            `json:"open_channel_id,omitempty"`
	RateLimit       int               `json:"rate_limit,omitempty"`
	AccessToken     string            `json:"access_token,omitempty"`
	TokenType       string            `json:"token_type,omitempty"`
	TokenExpiry     time.Time         `json:"token_expiry,omitempty"`
	UsePlainSecret  bool              `json:"use_plain_secret,omitempty"`
	PlainSecretFile string            `json:"plain_secret_file,omitempty"`
	PlainSecretHint string            `json:"plain_secret_hint,omitempty"`
	Channels        map[string]string `json:"channels,omitempty"`
	WriteAllowlist  []string          `json:"write_allowlist,omitempty"`
}

func ConfigDir() (string, error) {
	if configured := os.Getenv(EnvConfigDir); configured != "" {
		if filepath.IsAbs(configured) {
			return filepath.Clean(configured), nil
		}
		return filepath.Abs(configured)
	}
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

func CheckConfigDirWritable() error {
	dir, err := ConfigDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".write-check-*")
	if err != nil {
		return err
	}
	name := f.Name()
	if err := f.Close(); err != nil {
		_ = os.Remove(name)
		return err
	}
	return os.Remove(name)
}

func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	cfg := &Config{BaseURL: DefaultBaseURL, RateLimit: DefaultRate}
	// #nosec G304 -- path is the dedicated config path selected by the user or OS config directory.
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
	if cfg.Channels == nil {
		cfg.Channels = map[string]string{}
	}
	cfg.WriteAllowlist = uniqueStrings(cfg.WriteAllowlist)
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
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	release, err := lockfile.Acquire(ctx, filepath.Join(filepath.Dir(path), "config.lock"))
	if err != nil {
		return err
	}
	defer release()
	// #nosec G117 -- token cache is intentionally persisted only in the private 0600 config file.
	b, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func ApplyEnv(cfg *Config) {
	if v := os.Getenv(EnvClientID); v != "" {
		cfg.ClientID = v
	}
	if v := os.Getenv(EnvAccessToken); v != "" {
		cfg.AccessToken = v
		cfg.TokenType = "bearer"
		cfg.TokenExpiry = time.Now().Add(24 * time.Hour)
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
	if v := os.Getenv(EnvWriteAllowlist); v != "" {
		cfg.WriteAllowlist = uniqueStrings(strings.Split(v, ","))
	}
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
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
	if cfg.PlainSecretFile != "" {
		b, err := os.ReadFile(cfg.PlainSecretFile)
		if err != nil {
			return "", err
		}
		if secret := string(bytesTrimSpace(b)); secret != "" {
			return secret, nil
		}
		return "", errors.New("client_secret file is empty")
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

func bytesTrimSpace(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\n' || b[0] == '\r' || b[0] == '\t') {
		b = b[1:]
	}
	for len(b) > 0 {
		last := b[len(b)-1]
		if last != ' ' && last != '\n' && last != '\r' && last != '\t' {
			break
		}
		b = b[:len(b)-1]
	}
	return b
}

func KeyringAvailable() bool {
	testUser := "__captainbi_keyring_probe__"
	if err := keyring.Set(AppName, testUser, "ok"); err != nil {
		return false
	}
	_, err := keyring.Get(AppName, testUser)
	_ = keyring.Delete(AppName, testUser)
	return err == nil
}

func DeleteClientSecret(clientID string) {
	if clientID != "" {
		_ = keyring.Delete(AppName, clientID)
	}
}
