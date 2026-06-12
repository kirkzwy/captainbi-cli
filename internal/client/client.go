package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/security"
)

type AuthFunc func(context.Context, bool) (string, error)

type Client struct {
	cfg        *core.Config
	httpClient *http.Client
	limiter    *rate.Limiter
	auth       AuthFunc
}

type Request struct {
	Method        string
	Path          string
	Query         map[string]string
	Body          any
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
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, &Error{Kind: "rate_limit", Err: err}
	}
	var body io.Reader
	if request.Body != nil {
		b, err := json.Marshal(request.Body)
		if err != nil {
			return nil, &Error{Kind: "business", Err: err}
		}
		body = bytes.NewReader(b)
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
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(request.Method), u.String(), body)
	if err != nil {
		return nil, &Error{Kind: "business", Err: err}
	}
	httpReq.Header.Set("authorization", "bearer "+token)
	httpReq.Header.Set("content-type", "application/json")
	if request.OpenChannelID != "" {
		httpReq.Header.Set("OpenChannelId", request.OpenChannelID)
	}

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			delay := []time.Duration{5 * time.Second, 15 * time.Second, 45 * time.Second}[attempt-1]
			select {
			case <-ctx.Done():
				return nil, &Error{Kind: "network", Err: ctx.Err()}
			case <-time.After(delay):
			}
		}
		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, &Error{Kind: "network", Err: err}
		}
		result, err := decodeResponse(resp)
		if resp.StatusCode == http.StatusTooManyRequests {
			lastErr = &StatusError{StatusCode: resp.StatusCode, Body: result}
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, &StatusError{StatusCode: resp.StatusCode, Body: result}
		}
		if err != nil {
			return nil, &Error{Kind: "network", Err: err}
		}
		return result, nil
	}
	return nil, &Error{Kind: "rate_limit", Err: lastErr}
}

func decodeResponse(resp *http.Response) (map[string]any, error) {
	defer resp.Body.Close()
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
	Kind string
	Err  error
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

type StatusError struct {
	StatusCode int
	Body       map[string]any
}

func (e *StatusError) Error() string {
	if msg, ok := e.Body["msg"].(string); ok && msg != "" {
		return fmt.Sprintf("http %d: %s", e.StatusCode, security.RedactValue(msg))
	}
	return fmt.Sprintf("http %d", e.StatusCode)
}

func isStatus(err error, code int) bool {
	var se *StatusError
	return errors.As(err, &se) && se.StatusCode == code
}
