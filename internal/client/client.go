package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/lockfile"
	"github.com/kirkzwy/captainbi-cli/internal/security"
)

type AuthFunc func(context.Context, bool) (string, error)

type Client struct {
	cfg        *core.Config
	httpClient *http.Client
	limiter    *rate.Limiter
	auth       AuthFunc
	lastWait   time.Duration
	wait       func(context.Context, time.Duration) error
	jitter     func(time.Duration) time.Duration
}

type Request struct {
	Method        string
	Path          string
	Query         map[string]string
	Body          any
	ContentType   string
	OpenChannelID string
}

func New(cfg *core.Config, auth AuthFunc) *Client {
	rateLimit := cfg.RateLimit
	if rateLimit <= 0 {
		rateLimit = core.DefaultRate
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		limiter:    rate.NewLimiter(rate.Every(time.Minute/time.Duration(rateLimit)), 1),
		auth:       auth,
		wait:       waitContext,
		jitter:     jitterDuration,
	}
}

func (c *Client) Do(ctx context.Context, req Request) (map[string]any, error) {
	token, err := c.auth(ctx, false)
	if err != nil {
		return nil, &Error{Kind: "auth", Err: err}
	}
	resp, err := c.do(ctx, req, token)
	if isStatus(err, http.StatusUnauthorized) {
		token, err = c.auth(ctx, true)
		if err != nil {
			return nil, &Error{Kind: "auth", Err: err}
		}
		resp, err = c.do(ctx, req, token)
	}
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *Client) do(ctx context.Context, request Request, token string) (map[string]any, error) {
	c.lastWait = 0
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, &Error{Kind: "rate_limit", Err: err}
	}
	waited, err := c.waitCrossProcess(ctx)
	if err != nil {
		return nil, &Error{Kind: "rate_limit", Err: err, Retryable: true}
	}
	c.lastWait += waited
	bodyBytes, contentType, err := encodeBody(request.Body, request.ContentType)
	if err != nil {
		return nil, &Error{Kind: "business", Err: err}
	}
	u, err := url.Parse(strings.TrimRight(c.cfg.BaseURL, "/") + "/" + strings.TrimLeft(request.Path, "/"))
	if err != nil {
		return nil, &Error{Kind: "business", Err: err}
	}
	q := u.Query()
	for k, v := range request.Query {
		if v != "" {
			q.Set(k, v)
		}
	}
	u.RawQuery = q.Encode()

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := retryDelay(attempt, lastErr, c.jitter)
			if err := c.wait(ctx, delay); err != nil {
				return nil, &Error{Kind: "network", Err: err}
			}
			c.lastWait += delay
		}
		var body io.Reader
		if bodyBytes != nil {
			body = bytes.NewReader(bodyBytes)
		}
		httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(request.Method), u.String(), body)
		if err != nil {
			return nil, &Error{Kind: "business", Err: err}
		}
		httpReq.Header.Set("authorization", "bearer "+token)
		if contentType != "" {
			httpReq.Header.Set("content-type", contentType)
		}
		if request.OpenChannelID != "" {
			httpReq.Header.Set("OpenChannelId", request.OpenChannelID)
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, &Error{Kind: "network", Err: err}
		}
		result, err := decodeResponse(resp)
		closeErr := resp.Body.Close()
		if err == nil {
			err = closeErr
		}
		if resp.StatusCode == http.StatusTooManyRequests || captainBIRateLimited(result) {
			lastErr = &StatusError{StatusCode: resp.StatusCode, Body: result, Retryable: true, RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"))}
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, &StatusError{StatusCode: resp.StatusCode, Body: result}
		}
		if err != nil {
			return nil, &Error{Kind: "network", Err: err}
		}
		if !businessSuccess(result) {
			return nil, &BusinessError{Body: result}
		}
		return result, nil
	}
	retryAfter := time.Duration(0)
	var se *StatusError
	if errors.As(lastErr, &se) {
		retryAfter = se.RetryAfter
	}
	return nil, &Error{Kind: "rate_limit", Err: lastErr, Retryable: true, RetryAfter: retryAfter}
}

func captainBIRateLimited(body map[string]any) bool {
	if body == nil {
		return false
	}
	return fmt.Sprint(body["code"]) == "100910"
}

func retryDelay(attempt int, lastErr error, jitter func(time.Duration) time.Duration) time.Duration {
	var statusErr *StatusError
	if errors.As(lastErr, &statusErr) && statusErr.RetryAfter > 0 {
		return min(statusErr.RetryAfter, 5*time.Minute)
	}
	var base time.Duration
	switch attempt {
	case 1:
		base = 5 * time.Second
	case 2:
		base = 15 * time.Second
	default:
		base = 45 * time.Second
	}
	return jitter(base)
}

func jitterDuration(base time.Duration) time.Duration {
	delta := base / 5
	value, err := rand.Int(rand.Reader, big.NewInt(int64(2*delta)+1))
	if err != nil {
		return base
	}
	return base - delta + time.Duration(value.Int64())
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func encodeBody(body any, requestedContentType string) ([]byte, string, error) {
	if body == nil {
		return nil, "", nil
	}
	if requestedContentType == "" || requestedContentType == "application/json" {
		b, err := json.Marshal(body)
		return b, "application/json", err
	}
	if requestedContentType != "multipart/form-data" && requestedContentType != "application/form-data" {
		return nil, "", fmt.Errorf("unsupported request content type %q", requestedContentType)
	}
	fields, ok := body.(map[string]any)
	if !ok {
		return nil, "", errors.New("multipart request body must be a JSON object")
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := fields[key]
		var text string
		switch typed := value.(type) {
		case string:
			text = typed
		default:
			b, err := json.Marshal(value)
			if err != nil {
				_ = writer.Close()
				return nil, "", fmt.Errorf("encode multipart field %s: %w", key, err)
			}
			text = string(b)
		}
		if err := writer.WriteField(key, text); err != nil {
			_ = writer.Close()
			return nil, "", err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), writer.FormDataContentType(), nil
}

func businessSuccess(body map[string]any) bool {
	if body == nil {
		return true
	}
	value, ok := body["code"]
	if !ok || value == nil || fmt.Sprint(value) == "" {
		return true
	}
	switch code := value.(type) {
	case float64:
		return int(code) == 200
	case int:
		return code == 200
	case json.Number:
		return code.String() == "200"
	case string:
		return strings.TrimSpace(code) == "200"
	default:
		return fmt.Sprint(code) == "200"
	}
}

func (c *Client) LastRateLimitWait() time.Duration {
	return c.lastWait
}

func (c *Client) waitCrossProcess(ctx context.Context) (time.Duration, error) {
	dir, err := core.ConfigDir()
	if err != nil {
		return 0, err
	}
	interval := time.Minute / time.Duration(c.cfg.RateLimit)
	lockDir := filepath.Join(dir, "rate_limiter.lock")
	stateFile := filepath.Join(dir, "rate_limiter.next")
	start := time.Now()
	release, err := lockfile.Acquire(ctx, lockDir)
	if err != nil {
		return 0, err
	}
	defer release()

	var next time.Time
	// #nosec G304 -- stateFile is fixed under the private CaptainBI config directory.
	if b, err := os.ReadFile(stateFile); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && n > 0 {
			next = time.Unix(0, n)
		}
	}
	now := time.Now()
	if next.After(now) {
		wait := next.Sub(now)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(wait):
		}
	}
	newNext := time.Now().Add(interval)
	_ = os.WriteFile(stateFile, []byte(strconv.FormatInt(newNext.UnixNano(), 10)), 0o600)
	return time.Since(start), nil
}

func RateLimitStatus(cfg *core.Config) (map[string]any, error) {
	dir, err := core.ConfigDir()
	if err != nil {
		return nil, err
	}
	stateFile := filepath.Join(dir, "rate_limiter.next")
	lockFile := filepath.Join(dir, "rate_limiter.lock")
	out := map[string]any{
		"rate_limit_per_minute": cfg.RateLimit,
		"lock_file":             lockFile,
		"state_file":            stateFile,
	}
	if cfg.RateLimit <= 0 {
		out["rate_limit_per_minute"] = core.DefaultRate
	}
	// #nosec G304 -- stateFile is fixed under the private CaptainBI config directory.
	if b, err := os.ReadFile(stateFile); err == nil {
		if n, err := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64); err == nil && n > 0 {
			next := time.Unix(0, n)
			wait := time.Until(next)
			if wait < 0 {
				wait = 0
			}
			out["next_request_at"] = next.Format(time.RFC3339Nano)
			out["wait_ms"] = wait.Milliseconds()
			return out, nil
		}
	}
	out["next_request_at"] = ""
	out["wait_ms"] = 0
	return out, nil
}

func decodeResponse(resp *http.Response) (map[string]any, error) {
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	ct, _, _ := mime.ParseMediaType(resp.Header.Get("content-type"))
	if ct != "" && ct != "application/json" && len(b) == 0 {
		return map[string]any{"status": resp.StatusCode}, nil
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return map[string]any{"status": resp.StatusCode, "body": string(b)}, nil
	}
	return out, nil
}

type Error struct {
	Kind       string
	Err        error
	Retryable  bool
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type StatusError struct {
	StatusCode int
	Body       map[string]any
	Retryable  bool
	RetryAfter time.Duration
}

type BusinessError struct {
	Body map[string]any
}

func (e *BusinessError) Error() string {
	if e == nil {
		return "CaptainBI business error"
	}
	if msg, ok := e.Body["msg"].(string); ok && msg != "" {
		return security.RedactString(msg)
	}
	return "CaptainBI business error"
}

func (e *BusinessError) APICode() any {
	if e == nil || e.Body == nil {
		return nil
	}
	return e.Body["code"]
}

func (e *BusinessError) APIMessage() string {
	if e == nil || e.Body == nil {
		return ""
	}
	return fmt.Sprint(e.Body["msg"])
}

func (e *StatusError) Error() string {
	if msg, ok := e.Body["msg"].(string); ok && msg != "" {
		return fmt.Sprintf("http %d: %s", e.StatusCode, security.RedactValue(msg))
	}
	return fmt.Sprintf("http %d", e.StatusCode)
}

func (e *StatusError) APICode() any {
	if e == nil || e.Body == nil {
		return nil
	}
	if v, ok := e.Body["error"]; ok {
		return v
	}
	return e.Body["code"]
}

func (e *StatusError) APIMessage() string {
	if e == nil || e.Body == nil {
		return ""
	}
	if msg, ok := e.Body["msg"].(string); ok {
		return msg
	}
	if msg, ok := e.Body["message"].(string); ok {
		return msg
	}
	if msg, ok := e.Body["error_description"].(string); ok {
		return msg
	}
	return ""
}

func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if seconds, err := strconv.Atoi(v); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	if when, err := http.ParseTime(v); err == nil {
		wait := time.Until(when)
		if wait > 0 {
			return wait
		}
	}
	return 0
}

func isStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == code
}
