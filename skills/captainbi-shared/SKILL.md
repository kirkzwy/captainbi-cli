---
name: captainbi-shared
description: CaptainBI CLI shared rules for authentication, rate limits, output and safety.
---

# CaptainBI Shared

Always use `cbi` for CaptainBI OpenAPI work.

## Authentication

1. Configure credentials with `cbi config init --client-id <id> --client-secret-stdin`.
2. Fetch a token with `cbi auth token`.
3. Use `cbi +shops` to discover `OpenChannelId` for store-scoped APIs.

## Rate Limit

CaptainBI free API limit is 20 requests per minute. Prefer shortcuts and
`--page-all`; the CLI rate limits requests and retries 429 responses.

## Safety

Use `--dry-run` before write commands. Dangerous write commands require
`--confirm`; sync commands require `--confirm` and may trigger external state.

## Agent Mode

Use `--machine --format json` for pure JSON output.
