# Error Recovery

Read `error.kind`, `error.subtype`, `error.hint`, `error.retryable`, `error.retry_after_ms`, `error.api_code`, `error.api_msg`, and `error.request_id`.

For pagination and date ranges, read `meta.has_more`, `meta.next_window`, `meta.next_page`, and `meta.next_offset`. If `has_more=true`, reuse the same filters with the matching `--resume-from-window`, `--resume-from-page`, and `--resume-offset` values.

Common recovery:

- `auth`: run `cbi auth status --machine`, then `cbi auth token --machine` or refresh credentials.
- `rate_limit`: wait `retry_after_ms` when present; otherwise reduce concurrency or `--rate-limit`.
- `network`: retry later; for 5xx use exponential backoff.
- `business` with OpenChannelId hint: pass `--channel <alias>` or configure channel alias.
- missing required flag: rerun with `--help`, then pass the required time/date/page parameter.
- `confirmation_required`: do not proceed unless the user explicitly approves the write/sync operation.
- `CHANNEL_MISSING`: run `cbi +shops`, then configure or pass `--channel <alias>`.
- `CHANNEL_ALIAS_NOT_FOUND`: run `cbi config channels list`; use a configured alias. Raw IDs belong in `--open-channel-id`.
- `CHANNEL_INVALID`: verify the alias/OpenChannelId with `cbi +shops`, then update channel config.
- `CHANNEL_BATCH_FAILED`: inspect `data.channels`; fix failed aliases individually and retry only those channels.
- `VALIDATION_REQUIRED_FLAG`: use command `--help` examples and pass the missing flag.
- `VALIDATION_BAD_PARAM`: inspect `cbi schema <domain.command>` and correct the value.
- A modified-time range beyond the endpoint window limit requires `--page-all`; keep the same filters when resuming.
- `INPUT_PATH_UNSAFE`: use a relative file under the current working directory, or pipe absolute-path content through stdin.
- `API_BUSINESS_ERROR`: HTTP may still be 200. Read `api_code/api_msg`, correct the request and do not treat empty data as success.
- `WRITE_CONFIRMATION_MISMATCH`: the request changed after preview; generate a new dry-run.
- `WRITE_CONFIRMATION_EXPIRED`: the 15-minute preview expired; generate a new dry-run.
- `WRITE_CONFIRMATION_REPLAY`: the hash was already consumed or is missing; inspect the previous call before retrying.
- `WRITE_MULTI_CHANNEL_FORBIDDEN`: select one channel alias and approve each write separately.
- `WRITE_NOT_ALLOWLISTED`: review the risk and exact command, then ask the user before adding that `domain.command` with `cbi config write-allowlist add`.

CaptainBI OAuth errors may use `error/error_description`; business APIs may use `code/msg`. The CLI maps both to `api_code/api_msg`.
