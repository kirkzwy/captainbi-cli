package cmd

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/kirkzwy/captainbi-cli/internal/client"
	"github.com/kirkzwy/captainbi-cli/internal/registry"
	"github.com/kirkzwy/captainbi-cli/internal/security"
)

type requestBatch struct {
	Index int
	Start string
	End   string
	Req   client.Request
}

func executeRequest(ctx context.Context, c *client.Client, m registry.Method, req client.Request, opts requestOptions) (map[string]any, error) {
	if !opts.pageAll {
		if opts.rangeStart != "" || opts.rangeEnd != "" || opts.resumeWindow > 1 || opts.resumeOffset > 0 {
			return nil, typedH("business", "range and resume options require --page-all", "add --page-all or remove the range/resume options")
		}
		return c.Do(ctx, req)
	}
	batches, err := buildRequestBatches(req, m, opts)
	if err != nil {
		return nil, err
	}
	if len(batches) == 0 {
		batches = []requestBatch{{Index: 1, Req: cloneRequest(req)}}
	}
	startWindow := opts.resumeWindow
	if startWindow <= 0 {
		startWindow = 1
	}
	if startWindow > len(batches) {
		return nil, typedH("business", fmt.Sprintf("resume window %d exceeds %d available windows", startWindow, len(batches)), "use --resume-from-window from the previous response")
	}
	startPage := opts.resumePage
	if startPage <= 0 {
		startPage = 1
	}
	if opts.resumeOffset < 0 {
		return nil, typedH("business", "resume offset must be >= 0", "use the non-negative next_offset returned by the previous response")
	}

	rows := 0
	if m.Pagination.Type == "page_rows" {
		rowsText := requestParam(req, m, "rows")
		if rowsText == "" {
			rowsText = "100"
		}
		rows, _ = strconv.Atoi(rowsText)
		if rows <= 0 {
			rows = 100
		}
	}
	limit := opts.pageLimit
	if limit < 0 {
		limit = 10
	}
	delay := opts.pageDelay
	if delay <= 0 {
		delay = 3 * time.Second
	}

	all := []any{}
	var envelope map[string]any
	pagesFetched, pagesFailed := 0, 0
	windowsStarted, windowsCompleted := 0, 0
	failedWindow, failedPage := 0, 0
	partialError := ""
	hasMore := false
	nextWindow, nextPage, nextOffset := 0, 0, 0
	requestCount := 0

stop:
	for batchIndex := startWindow - 1; batchIndex < len(batches); batchIndex++ {
		batch := batches[batchIndex]
		windowsStarted++
		page := 1
		if batchIndex == startWindow-1 {
			page = startPage
		}
		for {
			if limit > 0 && pagesFetched >= limit {
				hasMore, nextWindow, nextPage = true, batch.Index, page
				break stop
			}
			current := cloneRequest(batch.Req)
			if m.Pagination.Type == "page_rows" {
				setRequestParam(&current, m, "rows", rows)
				setRequestParam(&current, m, "page", page)
			}
			if requestCount > 0 {
				if err := waitRequestDelay(ctx, delay); err != nil {
					return nil, err
				}
			}
			requestCount++
			resp, err := c.Do(ctx, current)
			if err != nil {
				pagesFailed++
				failedWindow, failedPage = batch.Index, page
				partialError = err.Error()
				if len(all) == 0 {
					return nil, err
				}
				hasMore, nextWindow, nextPage = true, batch.Index, page
				break stop
			}
			pagesFetched++
			if envelope == nil {
				envelope = resp
			}
			data, _ := resp["data"].([]any)
			if raw, ok := resp["data"]; ok && raw != nil && data == nil {
				structureErr := typedH("business", "paginated response data must be an array", "check the endpoint response contract before using --page-all")
				if len(all) == 0 {
					return nil, structureErr
				}
				pagesFailed++
				failedWindow, failedPage = batch.Index, page
				partialError = structureErr.Error()
				hasMore, nextWindow, nextPage = true, batch.Index, page
				break stop
			}

			offset := 0
			if batchIndex == startWindow-1 && page == startPage {
				offset = opts.resumeOffset
			}
			if offset > len(data) {
				return nil, typedH("business", fmt.Sprintf("resume offset %d exceeds page size %d", offset, len(data)), "use next_offset from the previous response without changing rows or filters")
			}
			available := data[offset:]
			if opts.maxRecords > 0 {
				remaining := opts.maxRecords - len(all)
				if remaining <= 0 {
					hasMore, nextWindow, nextPage, nextOffset = true, batch.Index, page, offset
					break stop
				}
				if len(available) > remaining {
					all = append(all, available[:remaining]...)
					hasMore, nextWindow, nextPage, nextOffset = true, batch.Index, page, offset+remaining
					break stop
				}
			}
			all = append(all, available...)

			pageComplete := m.Pagination.Type != "page_rows" || len(data) < rows
			if !pageComplete {
				maxResult := intFrom(resp[m.Pagination.TotalField])
				pageComplete = maxResult > 0 && page*rows >= maxResult
			}
			if opts.maxRecords > 0 && len(all) >= opts.maxRecords {
				if !pageComplete {
					hasMore, nextWindow, nextPage = true, batch.Index, page+1
				} else if batchIndex+1 < len(batches) {
					hasMore, nextWindow, nextPage = true, batches[batchIndex+1].Index, 1
				}
				break stop
			}
			if pageComplete {
				windowsCompleted++
				break
			}
			page++
		}
	}

	if envelope == nil {
		envelope = map[string]any{}
	}
	envelope["data"] = all
	envelope["page_all"] = true
	envelope["fetched_rows"] = len(all)
	envelope["pages_fetched"] = pagesFetched
	envelope["pages_failed"] = pagesFailed
	envelope["partial"] = pagesFailed > 0
	envelope["has_more"] = hasMore
	envelope["windows_total"] = len(batches)
	envelope["windows_started"] = windowsStarted
	envelope["windows_completed"] = windowsCompleted
	envelope["resume_from_window"] = startWindow
	envelope["resume_from_page"] = startPage
	envelope["resume_offset"] = opts.resumeOffset
	if m.Pagination.RangeType != "" {
		envelope["range_type"] = m.Pagination.RangeType
	}
	if hasMore {
		envelope["next_window"] = nextWindow
		envelope["next_page"] = nextPage
		envelope["next_offset"] = nextOffset
	}
	if failedPage > 0 {
		envelope["failed_at_window"] = failedWindow
		envelope["failed_at_page"] = failedPage
		envelope["partial_error"] = security.RedactString(partialError)
	}
	return envelope, nil
}

func buildRequestBatches(req client.Request, m registry.Method, opts requestOptions) ([]requestBatch, error) {
	switch m.Pagination.RangeType {
	case "modified_time_window":
		return modifiedTimeBatches(req, m)
	case "report_date":
		return reportDateBatches(req, m, opts.rangeStart, opts.rangeEnd)
	default:
		if opts.rangeStart != "" || opts.rangeEnd != "" {
			return nil, typedH("business", "this endpoint does not support report_date ranges", "remove --range-start/--range-end or choose an endpoint with pagination.rangeType=report_date")
		}
		return []requestBatch{{Index: 1, Req: cloneRequest(req)}}, nil
	}
}

func modifiedTimeBatches(req client.Request, m registry.Method) ([]requestBatch, error) {
	startName, endName := "start_modified_time", "end_modified_time"
	if !methodHasParam(m, startName) && methodHasParam(m, "start_report_time") {
		startName, endName = "start_report_time", "end_report_time"
	}
	startText, endText := requestParam(req, m, startName), requestParam(req, m, endName)
	if startText == "" || endText == "" {
		return []requestBatch{{Index: 1, Req: cloneRequest(req)}}, nil
	}
	start, err := strconv.ParseInt(startText, 10, 64)
	if err != nil {
		return nil, typedH("business", startName+" must be unix seconds", "pass integer unix timestamps for the modified-time range")
	}
	end, err := strconv.ParseInt(endText, 10, 64)
	if err != nil || end < start {
		return nil, typedH("business", endName+" must be unix seconds and >= "+startName, "pass an ordered modified-time range")
	}
	days := m.Pagination.WindowDays
	if days <= 0 {
		days = 31
	}
	windowSeconds := int64(days) * 24 * 60 * 60
	batches := []requestBatch{}
	for cursor := start; cursor <= end; {
		windowEnd := cursor + windowSeconds - 1
		if windowEnd > end {
			windowEnd = end
		}
		current := cloneRequest(req)
		setRequestParam(&current, m, startName, cursor)
		setRequestParam(&current, m, endName, windowEnd)
		batches = append(batches, requestBatch{Index: len(batches) + 1, Start: strconv.FormatInt(cursor, 10), End: strconv.FormatInt(windowEnd, 10), Req: current})
		cursor = windowEnd + 1
	}
	return batches, nil
}

func reportDateBatches(req client.Request, m registry.Method, start, end string) ([]requestBatch, error) {
	if start == "" && end == "" {
		return []requestBatch{{Index: 1, Req: cloneRequest(req)}}, nil
	}
	if start == "" || end == "" {
		return nil, typedH("business", "--range-start and --range-end must be used together", "pass an inclusive YYYYMMDD or YYYYMM range")
	}
	values, err := reportDateValues(start, end)
	if err != nil {
		return nil, err
	}
	batches := make([]requestBatch, 0, len(values))
	for _, value := range values {
		current := cloneRequest(req)
		setRequestParam(&current, m, "report_date", value)
		batches = append(batches, requestBatch{Index: len(batches) + 1, Start: value, End: value, Req: current})
	}
	return batches, nil
}

func reportDateValues(start, end string) ([]string, error) {
	if len(start) != len(end) || (len(start) != 6 && len(start) != 8) {
		return nil, typedH("business", "report date range must use matching YYYYMMDD or YYYYMM values", "use daily values such as 20260601..20260630 or monthly values such as 202601..202612")
	}
	layout := "20060102"
	if len(start) == 6 {
		layout = "200601"
	}
	startDate, err := time.Parse(layout, start)
	if err != nil {
		return nil, typedH("business", "invalid --range-start date", "use a real calendar date in YYYYMMDD or YYYYMM format")
	}
	endDate, err := time.Parse(layout, end)
	if err != nil || endDate.Before(startDate) {
		return nil, typedH("business", "invalid or reversed --range-end date", "use an end date on or after --range-start")
	}
	values := []string{}
	for value := startDate; !value.After(endDate); {
		values = append(values, value.Format(layout))
		if len(values) > 3660 {
			return nil, typedH("business", "report date range is too large", "split the request into ranges of at most 3660 days or months")
		}
		if len(start) == 6 {
			value = value.AddDate(0, 1, 0)
		} else {
			value = value.AddDate(0, 0, 1)
		}
	}
	return values, nil
}

func cloneRequest(req client.Request) client.Request {
	copyReq := req
	copyReq.Query = map[string]string{}
	for key, value := range req.Query {
		copyReq.Query[key] = value
	}
	if body, ok := req.Body.(map[string]any); ok {
		copyBody := map[string]any{}
		for key, value := range body {
			copyBody[key] = value
		}
		copyReq.Body = copyBody
	}
	return copyReq
}

func methodHasParam(m registry.Method, name string) bool {
	for _, param := range m.Params {
		if param.Name == name {
			return true
		}
	}
	return false
}

func waitRequestDelay(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
