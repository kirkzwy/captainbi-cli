package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/core"
)

func TestGetTokenSendsScopeAll(t *testing.T) {
	t.Setenv(core.EnvClientSecret, "client-secret")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var gotScope string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/oauth2/token" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		gotScope = r.Form.Get("scope")
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "access-token",
			TokenType:   "bearer",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	cfg := &core.Config{ClientID: "client-id", BaseURL: server.URL}
	token, err := GetToken(context.Background(), cfg, true)
	if err != nil {
		t.Fatal(err)
	}
	if token != "access-token" {
		t.Fatalf("token = %q", token)
	}
	if gotScope != "all" {
		t.Fatalf("scope = %q, want all", gotScope)
	}
}

func TestGetTokenUsesLockAndFreshCache(t *testing.T) {
	t.Setenv(core.EnvClientSecret, "client-secret")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		time.Sleep(150 * time.Millisecond)
		_ = json.NewEncoder(w).Encode(TokenResponse{
			AccessToken: "access-token",
			TokenType:   "bearer",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	cfg1 := &core.Config{ClientID: "client-id", BaseURL: server.URL}
	cfg2 := &core.Config{ClientID: "client-id", BaseURL: server.URL}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, cfg := range []*core.Config{cfg1, cfg2} {
		wg.Add(1)
		go func(cfg *core.Config) {
			defer wg.Done()
			token, err := GetToken(context.Background(), cfg, false)
			if err != nil {
				errs <- err
				return
			}
			if token != "access-token" {
				errs <- errors.New("unexpected token")
			}
		}(cfg)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", got)
	}
}

func TestGetTokenReturnsOAuthErrorFields(t *testing.T) {
	t.Setenv(core.EnvClientSecret, "client-secret")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid_client", "error_description": "Invalid client authentication"})
	}))
	defer server.Close()

	cfg := &core.Config{ClientID: "client-id", BaseURL: server.URL}
	_, err := GetToken(context.Background(), cfg, true)
	var tokenErr *TokenError
	if !errors.As(err, &tokenErr) {
		t.Fatalf("expected TokenError, got %T %v", err, err)
	}
	if tokenErr.ErrorCode != "invalid_client" || tokenErr.ErrorDescription != "Invalid client authentication" {
		t.Fatalf("unexpected token error: %+v", tokenErr)
	}
}
