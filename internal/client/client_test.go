package client

import (
	"context"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/core"
)

func TestParseRetryAfterSeconds(t *testing.T) {
	if got := parseRetryAfter("7"); got != 7*time.Second {
		t.Fatalf("retry after = %s", got)
	}
}

func TestEncodeMultipartBody(t *testing.T) {
	body, contentType, err := encodeBody(map[string]any{
		"shipment_ids": "FBA1,FBA2",
		"data":         []any{map[string]any{"sku": "TEST", "quantity": 2}},
	}, "multipart/form-data")
	if err != nil {
		t.Fatal(err)
	}
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType != "multipart/form-data" {
		t.Fatalf("content type = %q, params=%v, err=%v", contentType, params, err)
	}
	reader := multipart.NewReader(strings.NewReader(string(body)), params["boundary"])
	values := map[string]string{}
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(part)
		values[part.FormName()] = string(b)
	}
	if values["shipment_ids"] != "FBA1,FBA2" || values["data"] != `[{"quantity":2,"sku":"TEST"}]` {
		t.Fatalf("unexpected multipart values: %#v", values)
	}
}

func TestHTTP200BusinessError(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"code":-1,"msg":"bad parameter","data":[]}`)
	}))
	defer server.Close()
	cfg := &core.Config{BaseURL: server.URL, RateLimit: 6000}
	c := New(cfg, func(context.Context, bool) (string, error) { return "token", nil })
	_, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/test"})
	var businessErr *BusinessError
	if !errors.As(err, &businessErr) {
		t.Fatalf("expected BusinessError, got %T %v", err, err)
	}
	if businessErr.APICode() != float64(-1) || businessErr.APIMessage() != "bad parameter" {
		t.Fatalf("unexpected business error: %#v", businessErr)
	}
}

func TestParseRetryAfterHTTPDate(t *testing.T) {
	future := time.Now().Add(10 * time.Second).UTC().Format(http.TimeFormat)
	got := parseRetryAfter(future)
	if got <= 0 || got > 11*time.Second {
		t.Fatalf("retry after date = %s", got)
	}
}
