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

func Test429RetryUsesRetryAfterHeader(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("content-type", "application/json")
		if calls == 1 {
			w.Header().Set("Retry-After", "7")
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = io.WriteString(w, `{"code":429,"msg":"slow down"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":[]}`)
	}))
	defer server.Close()
	c := New(&core.Config{BaseURL: server.URL, RateLimit: 6000}, func(context.Context, bool) (string, error) { return "token", nil })
	waits := []time.Duration{}
	c.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	if _, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/test"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(waits) != 1 || waits[0] != 7*time.Second {
		t.Fatalf("calls=%d waits=%v", calls, waits)
	}
}

func Test429FallbackDelayUsesJitter(t *testing.T) {
	identity := func(value time.Duration) time.Duration { return value }
	if got := retryDelay(1, &StatusError{StatusCode: 429}, identity); got != 5*time.Second {
		t.Fatalf("first fallback delay=%s", got)
	}
	for _, base := range []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second} {
		got := jitterDuration(base)
		if got < base-base/5 || got > base+base/5 {
			t.Fatalf("jitter %s outside expected range for %s", got, base)
		}
	}
}

func TestCaptainBIRateLimitBusinessCodeRetriesWithoutTokenRefresh(t *testing.T) {
	t.Setenv(core.EnvConfigDir, t.TempDir())
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.Header().Set("content-type", "application/json")
		if calls < 4 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":100910,"msg":"too frequent"}`)
			return
		}
		_, _ = io.WriteString(w, `{"code":200,"msg":"ok","data":[]}`)
	}))
	defer server.Close()
	authCalls := []bool{}
	c := New(&core.Config{BaseURL: server.URL, RateLimit: 6000}, func(_ context.Context, force bool) (string, error) {
		authCalls = append(authCalls, force)
		return "token", nil
	})
	waits := []time.Duration{}
	c.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}
	c.jitter = func(delay time.Duration) time.Duration { return delay }
	if _, err := c.Do(context.Background(), Request{Method: http.MethodGet, Path: "/test"}); err != nil {
		t.Fatal(err)
	}
	if calls != 4 || len(authCalls) != 1 || authCalls[0] {
		t.Fatalf("calls=%d authCalls=%v", calls, authCalls)
	}
	wantWaits := []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}
	if len(waits) != len(wantWaits) {
		t.Fatalf("waits=%v", waits)
	}
	for i := range wantWaits {
		if waits[i] != wantWaits[i] {
			t.Fatalf("waits=%v", waits)
		}
	}
}
