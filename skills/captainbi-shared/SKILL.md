---
name: captainbi-shared
description: "CaptainBI CLI shared rules for authentication, channels, output parsing, errors, rate limits and safety. WHEN use for: any CaptainBI CLI task, first-run setup, credential recovery, channel selection, output parsing, pagination, or safety decisions. WHEN NOT: do not use for Amazon SP-API-only, Feishu-only, logistics-file-only, or non-CaptainBI tasks."
---

# CaptainBI Shared

Use this skill before any CaptainBI domain skill.

## References

- `references/agent-quick-start.md`: first-run Agent SOP.
- `references/auth-setup.md`: credential and token recovery.
- `references/output-contract.md`: success/error JSON contract.
- `references/channel-alias.md`: channel alias rules.
- `references/error-recovery.md`: retry and repair decisions.
- `references/safety.md`: read-only default and write protection.

## WHEN

Use `cbi` when the task needs CaptainBI/OpenAPI data: shops, sites, goods, orders, finance, FBA, ads, reviews, feedback, monitoring, or CaptainBI-generated business reports.

## WHEN NOT

Do not use CaptainBI for Amazon SP-API-only tasks, logistics files, Feishu-only records, or write/sync operations unless the user explicitly asks for CaptainBI changes.

## Authentication

- Configure once: `printf '%s' '<CAPTAINBI_CLIENT_SECRET>' | cbi config init --client-id '<CAPTAINBI_CLIENT_ID>' --client-secret-stdin --non-interactive`.
- Token requests require `scope=all`; the CLI sends it automatically.
- Check status: `cbi auth status --machine`.
- If auth fails, read `hint`, then refresh credentials with `cbi config init --client-secret-stdin`.

## Channels

- Discover stores: `cbi +shops --machine --format json`.
- Save aliases: `cbi config channels add <alias> <open_channel_id>`.
- Prefer `--channel <alias>` for daily work.
- Use `--channel all` only after aliases are configured.
- Avoid printing raw OpenChannelId in user-visible logs.

## Output

- Use `--machine --format json` for Agent calls.
- For large data, first run `--summary`; then use `--output-file` for full data.
- On failure, parse `error_code`, `kind`, `hint`, `api_code`, `api_msg`, `request_id`, `retryable`.
- For pagination, prefer `--page-all --max-records <n>`; resume with `--resume-from-page <page>`.

## Safety

- Read operations are safe.
- Write operations require explicit user intent.
- Use `--dry-run` before any POST.
- In Agent mode, every write requires `--confirm-request <hash>` from the exact current dry-run preview.
- Stop after dry-run and ask the user to approve the preview. Never reuse the hash automatically without that approval.
- Writes cannot use `--channel all`; approve one channel and payload at a time.
- Never output token, client_secret, authorization header, or raw OpenChannelId.

## Rate Limits

- Default local limit is 20 req/min.
- Inspect local state: `cbi rate-limit status --machine`.
- If `retryable=true`, wait according to `retry_after_ms` when present.
