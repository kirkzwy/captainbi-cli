# Output Contract v1

Agent calls should use:

```bash
cbi ... --machine --format json
```

Success:

```json
{
  "ok": true,
  "data": {},
  "meta": {
    "hints": [],
    "alerts": [],
    "count": 0,
    "rows": 0,
    "pages_fetched": 0,
    "pages_failed": 0,
    "partial": false,
    "has_more": false,
	"next_window": null,
    "next_page": null,
	"next_offset": 0,
	"windows_total": 1,
	"windows_completed": 1,
    "rate_limit_wait_ms": 0,
    "channel": "",
    "output_file": ""
  }
}
```

Failure:

```json
{
  "ok": false,
  "error": {
    "kind": "business",
    "subtype": "API_BUSINESS_ERROR",
    "message": "",
    "hint": "",
    "retryable": false,
    "retry_after_ms": 0,
    "api_code": null,
    "api_msg": "",
    "request_id": ""
  },
  "meta": {
    "exit_code": 1,
    "hints": [],
    "alerts": []
  }
}
```

Compatibility fields such as `error_code`, `kind`, `message`, `hint`, `api_code`, and `api_msg` may also appear at the top level. Prefer the nested `error` object when available.

Stdout is for data. Stderr is for errors, diagnostics, progress and debug logs.

Stable error subtypes:

- `AUTH_MISSING_CREDENTIALS`
- `AUTH_INVALID_CLIENT`
- `AUTH_TOKEN_REFRESH_FAILED`
- `CHANNEL_MISSING`
- `CHANNEL_ALIAS_NOT_FOUND`
- `CHANNEL_INVALID`
- `VALIDATION_REQUIRED_FLAG`
- `VALIDATION_BAD_PARAM`
- `INPUT_PATH_UNSAFE`
- `RATE_LIMIT_EXCEEDED`
- `HTTP_5XX`
- `NETWORK_FAILED`
- `CONFIRMATION_REQUIRED`
- `WRITE_CONFIRMATION_MISMATCH`
- `WRITE_CONFIRMATION_EXPIRED`
- `WRITE_CONFIRMATION_REPLAY`
- `WRITE_MULTI_CHANNEL_FORBIDDEN`
- `WRITE_NOT_ALLOWLISTED`
- `API_BUSINESS_ERROR`

Write dry-run data includes:

```json
{
  "dry_run": true,
  "method": "POST",
  "path": "/v1/...",
  "content_type": "multipart/form-data",
  "risk_level": "write_dangerous",
  "channel": "main",
  "body": {},
  "approval": {
    "required": true,
    "request_hash": "...",
    "expires_at": "...",
    "confirm_flag": "--confirm-request"
  },
  "policy": {
    "allowlist_required": true,
    "allowlisted": false,
    "allow_command": "goods.set-group"
  }
}
```

For a paginated or date-range result, continue only when `meta.has_more=true`. Reuse the unchanged filters and pass `meta.next_window`, `meta.next_page`, and `meta.next_offset` through `--resume-from-window`, `--resume-from-page`, and `--resume-offset`. `modified_time_window` ranges are split into non-overlapping 31-day windows; `report_date` batches use inclusive `--range-start/--range-end` values in matching `YYYYMMDD` or `YYYYMM` format.
