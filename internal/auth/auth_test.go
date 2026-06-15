package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

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
