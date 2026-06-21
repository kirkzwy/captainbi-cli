package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/lockfile"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Data        struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	} `json:"data"`
}

type TokenError struct {
	StatusCode       int
	ErrorCode        string `json:"error"`
	ErrorDescription string `json:"error_description"`
	Code             any    `json:"code"`
	Msg              string `json:"msg"`
}

func (e *TokenError) Error() string {
	if e.ErrorDescription != "" {
		return e.ErrorDescription
	}
	if e.Msg != "" {
		return e.Msg
	}
	if e.ErrorCode != "" {
		return e.ErrorCode
	}
	return fmt.Sprintf("token request failed with http %d", e.StatusCode)
}

func GetToken(ctx context.Context, cfg *core.Config, force bool) (string, error) {
	if os.Getenv(core.EnvAccessToken) != "" && cfg.AccessToken != "" {
		return cfg.AccessToken, nil
	}
	if !force && cfg.AccessToken != "" && time.Until(cfg.TokenExpiry) > time.Minute {
		return cfg.AccessToken, nil
	}
	unlock, err := acquireTokenLock(ctx)
	if err != nil {
		return "", err
	}
	defer unlock()
	if !force {
		if fresh, err := core.LoadConfig(); err == nil && fresh.AccessToken != "" && time.Until(fresh.TokenExpiry) > time.Minute {
			cfg.AccessToken = fresh.AccessToken
			cfg.TokenType = fresh.TokenType
			cfg.TokenExpiry = fresh.TokenExpiry
			return cfg.AccessToken, nil
		}
	}
	secret, err := core.LoadClientSecret(cfg)
	if err != nil {
		return "", err
	}
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", cfg.ClientID)
	form.Set("client_secret", secret)
	form.Set("scope", "all")
	endpoint := strings.TrimRight(cfg.BaseURL, "/") + "/oauth2/token"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		tokenErr := &TokenError{StatusCode: resp.StatusCode}
		_ = json.Unmarshal(body, tokenErr)
		if tokenErr.ErrorCode == "" && tokenErr.Code == nil && tokenErr.Msg == "" && tokenErr.ErrorDescription == "" {
			tokenErr.ErrorDescription = "token request failed"
		}
		return "", tokenErr
	}
	var tr TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tr); err != nil {
		return "", err
	}
	token := tr.AccessToken
	tokenType := tr.TokenType
	expires := tr.ExpiresIn
	if token == "" {
		token = tr.Data.AccessToken
		tokenType = tr.Data.TokenType
		expires = tr.Data.ExpiresIn
	}
	if token == "" {
		return "", errors.New("token response did not contain access_token")
	}
	if expires <= 0 {
		expires = 3600
	}
	if tokenType == "" {
		tokenType = "bearer"
	}
	cfg.AccessToken = token
	cfg.TokenType = tokenType
	cfg.TokenExpiry = time.Now().Add(time.Duration(expires) * time.Second)
	_ = core.SaveConfig(cfg)
	return token, nil
}

func acquireTokenLock(ctx context.Context) (func(), error) {
	dir, err := core.ConfigDir()
	if err != nil {
		return nil, err
	}
	return lockfile.Acquire(ctx, filepath.Join(dir, "token.lock"))
}
