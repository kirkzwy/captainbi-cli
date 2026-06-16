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
    "subtype": "BUSINESS",
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
