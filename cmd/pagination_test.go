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
	if resp["has_more"] != true || intFrom(resp["next_page"]) != 1 || intFrom(resp["next_offset"]) != 1 || intFrom(resp["fetched_rows"]) != 1 {
		t.Fatalf("unexpected max-records metadata: %#v", resp)
	}
	resumed, err := executeRequest(context.Background(), c, pageRowsMethod(), client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "2"}}, requestOptions{pageAll: true, maxRecords: 1, resumePage: 1, resumeOffset: 1, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	data := resumed["data"].([]any)
	if len(data) != 1 || intFrom(data[0].(map[string]any)["id"]) != 2 {
		t.Fatalf("resume offset lost data: %#v", resumed)
	}
}

func TestExecuteRequestSplitsModifiedTimeWindows(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seen := [][2]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, [2]string{r.URL.Query().Get("start_modified_time"), r.URL.Query().Get("end_modified_time")})
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"window": len(seen)}}})
	}))
	defer server.Close()
	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	m := pageRowsMethod()
	m.Pagination.RangeType = "modified_time_window"
	m.Pagination.WindowDays = 31
	m.Params = []registry.Param{{Name: "start_modified_time", Location: "query"}, {Name: "end_modified_time", Location: "query"}}
	start := int64(1_700_000_000)
	end := start + 40*24*60*60
	resp, err := executeRequest(context.Background(), c, m, client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "100", "start_modified_time": strconv.FormatInt(start, 10), "end_modified_time": strconv.FormatInt(end, 10)}}, requestOptions{pageAll: true, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || intFrom(resp["windows_completed"]) != 2 || intFrom(resp["fetched_rows"]) != 2 {
		t.Fatalf("unexpected window split: seen=%#v resp=%#v", seen, resp)
	}
	firstEnd, _ := strconv.ParseInt(seen[0][1], 10, 64)
	secondStart, _ := strconv.ParseInt(seen[1][0], 10, 64)
	if firstEnd+1 != secondStart {
		t.Fatalf("windows overlap or have a gap: %#v", seen)
	}
}

func TestExecuteRequestReportDateRangeAndResumeWindow(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	seen := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("report_date")
		seen = append(seen, date)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"date": date}}})
	}))
	defer server.Close()
	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	m := pageRowsMethod()
	m.Pagination.RangeType = "report_date"
	m.Params = []registry.Param{{Name: "report_date", Location: "query"}}
	resp, err := executeRequest(context.Background(), c, m, client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "100"}}, requestOptions{pageAll: true, rangeStart: "20260601", rangeEnd: "20260603", resumeWindow: 2, pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != 2 || seen[0] != "20260602" || seen[1] != "20260603" || intFrom(resp["windows_total"]) != 3 {
		t.Fatalf("unexpected report range resume: seen=%#v resp=%#v", seen, resp)
	}
	months, err := reportDateValues("202601", "202603")
	if err != nil || len(months) != 3 || months[2] != "202603" {
		t.Fatalf("monthly range failed: values=%#v err=%v", months, err)
	}
}

func TestExecuteRequestRangePartialFailure(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		date := r.URL.Query().Get("report_date")
		if date == "20260602" {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": -1, "msg": "range failed", "data": []any{}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 200, "msg": "ok", "data": []map[string]any{{"date": date}}})
	}))
	defer server.Close()
	c := client.New(&core.Config{BaseURL: server.URL, RateLimit: 10000}, func(context.Context, bool) (string, error) { return "token", nil })
	m := pageRowsMethod()
	m.Pagination.RangeType = "report_date"
	m.Params = []registry.Param{{Name: "report_date", Location: "query"}}
	resp, err := executeRequest(context.Background(), c, m, client.Request{Method: "GET", Path: "/items", Query: map[string]string{"rows": "100"}}, requestOptions{pageAll: true, rangeStart: "20260601", rangeEnd: "20260603", pageDelay: time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if resp["partial"] != true || intFrom(resp["failed_at_window"]) != 2 || intFrom(resp["next_window"]) != 2 || intFrom(resp["next_page"]) != 1 {
		t.Fatalf("unexpected range partial metadata: %#v", resp)
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
