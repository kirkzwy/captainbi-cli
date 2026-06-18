package cmd

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/core"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
)

func TestExecuteRequestPageRowsWithoutTotalField(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		rows := []map[string]any{}
		if page <= 2 {
			rows = append(rows, map[string]any{"id": page})
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": rows})
	}))
	defer server.Close()

	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	resp, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "1"}}, requestOptions{pageAll: true, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if resp["partial"] != false || intFrom(resp["pages_fetched"]) != 3 || intFrom(resp["fetched_rows"]) != 2 {
		t.Fatalf("unexpected pagination metadata: %#v", resp)
	}
	if resp["has_more"] != false {
		t.Fatalf("has_more = %#v, want false: %#v", resp["has_more"], resp)
	}
}

func TestExecuteRequestPageRowsPartialFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page == 2 {
			http.Error(w, `{"code":5001,"msg":"boom"}`, http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"id": page}}})
	}))
	defer server.Close()

	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	resp, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "1"}}, requestOptions{pageAll: true, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if resp["partial"] != true || intFrom(resp["pages_failed"]) != 1 || intFrom(resp["failed_at_page"]) != 2 || intFrom(resp["fetched_rows"]) != 1 {
		t.Fatalf("unexpected partial metadata: %#v", resp)
	}
	if resp["has_more"] != true || intFrom(resp["next_page"]) != 2 {
		t.Fatalf("unexpected partial continuation metadata: %#v", resp)
	}
}

func TestExecuteRequestPageRowsResumeFromPage(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.Query().Get("page"))
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{}})
	}))
	defer server.Close()

	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	resp, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "100"}}, requestOptions{pageAll: true, resumePage: 4})
	if err != nil {
		t.Fatal(err)
	}
	if got := seen[0]; got != "4" {
		t.Fatalf("first page = %s", got)
	}
	if intFrom(resp["resume_from_page"]) != 4 {
		t.Fatalf("resume_from_page missing: %#v", resp)
	}
}

func TestExecuteRequestPageRowsHasMoreOnPageLimit(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"id": r.URL.Query().Get("page")}}})
	}))
	defer server.Close()

	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	resp, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "1"}}, requestOptions{pageAll: true, pageLimit: 1, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if resp["has_more"] != true || intFrom(resp["next_page"]) != 2 {
		t.Fatalf("unexpected has_more metadata: %#v", resp)
	}
}

func TestExecuteRequestPageRowsHasMoreOnMaxRecords(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"id": 1}, {"id": 2}}})
	}))
	defer server.Close()

	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	resp, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "2"}}, requestOptions{pageAll: true, maxRecords: 1, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if resp["has_more"] != true || intFrom(resp["next_page"]) != 2 || intFrom(resp["fetched_rows"]) != 1 {
		t.Fatalf("unexpected max-records metadata: %#v", resp)
	}
}

func pageRowsMethod() registry.Method {
	return registry.Method{
		HTTPMethod: "GET",
		FullPath:   "/items",
		Pagination: registry.Pagination{Type: "page_rows", TotalField: "max_result"},
		RiskLevel:  "read",
	}
}
