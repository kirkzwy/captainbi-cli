# Error Recovery

Read `error.kind`, `error.subtype`, `error.hint`, `error.retryable`, `error.retry_after_ms`, `error.api_code`, `error.api_msg`, and `error.request_id`.

Common recovery:

- `auth`: run `cbi auth status --machine`, then `cbi auth token --machine` or refresh credentials.
- `rate_limit`: wait `retry_after_ms` when present; otherwise reduce concurrency or `--rate-limit`.
- `network`: retry later; for 5xx use exponential backoff.
- `business` with OpenChannelId hint: pass `--channel <alias>` or configure channel alias.
- missing required flag: rerun with `--help`, then pass the required time/date/page parameter.
- `confirmation_required`: do not proceed unless the user explicitly approves the write/sync operation.

CaptainBI OAuth errors may use `error/error_description`; business APIs may use `code/msg`. The CLI maps both to `api_code/api_msg`.
