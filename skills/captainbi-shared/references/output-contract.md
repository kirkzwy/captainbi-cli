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
    "windows_total": 0,
    "windows_started": 0,
    "windows_completed": 0,
    "rate_limit_wait_ms": 0,
    "channel": "",
    "output_file": "",
    "format": "json",
    "streaming": false
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

Control-plane commands such as `auth`, `config show`, `doctor local`, JSON `schema`, `rate-limit status`, and `registry` use the same success envelope in Agent mode. During v0.x their original fields also remain at the top level for compatibility; new integrations should read `.data`. Direct-consumption artifacts such as `tools export` and `schema --format openai-tool` remain unwrapped.

For `--machine --format csv|table|ndjson` without `--output-file`, stdout is the pure requested data stream. Stderr ends with exactly one compact success status line:

```json
{"ok":true,"meta":{"format":"ndjson","rows":100,"partial":false,"has_more":false,"streaming":true}}
```

Redacted verbose/debug diagnostics may precede this line, so consumers must parse the final stderr line as the completion status. Errors continue to use the normal error envelope on stderr and a non-zero exit code.

With `--output-file`, the file contains only the requested data format and stdout contains the JSON success envelope. Its `data` includes `output_file`, `format`, and `rows`; its `meta` preserves pagination, resume, partial, rate-limit, and streaming fields. Output files are written through a private temporary file and installed with `0600` permissions on supported systems.

CSV and table include every actual response field. Registry columns only influence ordering; remaining fields are appended deterministically. CSV preserves full values and encodes nested values as compact JSON. Table is a terminal view and truncates cells beyond 40 display columns. Empty CSV responses use response-schema fields as the header when available.

Multi-channel results include `meta.channels_total`, `meta.channels_succeeded`, `meta.channels_failed`, summed `meta.rows`, and `meta.partial`. Mixed success exits 0 with `partial=true`. When every channel fails, the command exits 1 with `CHANNEL_BATCH_FAILED` and preserves per-channel details under `data.channels`.

Stdout is for data. Stderr is for errors, diagnostics, progress and debug logs.

Stable error subtypes:

- `AUTH_MISSING_CREDENTIALS`
- `AUTH_INVALID_CLIENT`
- `AUTH_TOKEN_REFRESH_FAILED`
- `CHANNEL_MISSING`
- `CHANNEL_ALIAS_NOT_FOUND`
- `CHANNEL_INVALID`
- `CHANNEL_BATCH_FAILED`
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

For a paginated or date-range result, continue only when `meta.has_more=true`. Reuse the unchanged filters and pass `meta.next_window`, `meta.next_page`, and `meta.next_offset` through `--resume-from-window`, `--resume-from-page`, and `--resume-offset`. `windows_started/windows_completed` count windows processed by the current invocation, while `windows_total` describes the complete requested range. `modified_time_window` ranges are split into non-overlapping 31-day windows; a wider request without `--page-all` is rejected locally. `report_date` batches use inclusive `--range-start/--range-end` values in matching `YYYYMMDD` or `YYYYMM` format.

For a single-channel read-only `--format ndjson --page-all` call without `--jq`, `--summary`, or `--limit`, each page is emitted as soon as it is fetched and `meta.streaming=true`. Transformed, summarized, limited, and multi-channel calls retain aggregate behavior and return `meta.streaming=false`. If a later page fails, already emitted rows remain valid, the final meta is marked `partial=true`, and the normal `next_*` cursor identifies the resume point.
