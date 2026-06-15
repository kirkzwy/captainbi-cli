# captainbi-cli

CaptainBI OpenAPI command-line client. The main command is `cbi`; `captainbi`
is kept as an alias.

## Status

This repository is an early Agent-ready CaptainBI CLI:

- Go + Cobra single binary.
- OpenAPI -> Registry metadata -> generated service commands.
- Built-in token caching, OAuth `scope=all`, redaction, 20 req/min rate limiting and 429 retry.
- Generic `api`, generated domain commands, shortcuts, schema and doctor commands.
- Real read-only smoke has passed for auth, sites, shops, goods, orders, finance, ads, FBA and reviews.

## Quick Start

```bash
# Build from source after installing Go 1.24+
go build -buildvcs=false -o bin/cbi .

# Configure credentials. Do not pass secrets as command-line flags.
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive

cbi auth token
cbi +sites
cbi +shops
cbi config channels add main '<open_channel_id>'
```

## Command Layers

```bash
# Shortcuts
cbi +shops
cbi +sites
cbi +orders --channel main --start 1781424057 --end 1781510457
cbi +goods --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +finance-daily --channel main --date 20260615

# Generated business-domain commands
cbi goods list --channel main --start-modified-time 1700000000 --end-modified-time 1700100000
cbi finance store-daily --channel main --report-date 20240101
cbi ads campaign-report --channel main

# Generic API escape hatch
cbi api GET /v1/open_user/get_site_list

# Schema
cbi schema finance.store-daily
cbi schema finance.store-daily --format openai-tool
cbi tools export --format openai
```

## Environment Variables

| Name | Purpose |
| --- | --- |
| `CAPTAINBI_CLIENT_ID` | APPID/client_id |
| `CAPTAINBI_CLIENT_SECRET` | client_secret, preferred for CI or one-off runs |
| `CAPTAINBI_BASE_URL` | API base URL, defaults to `https://openapi.captainbi.com` |
| `CAPTAINBI_OPEN_CHANNEL_ID` | default OpenChannelId |
| `CAPTAINBI_RATE_LIMIT` | requests per minute, defaults to 20 |
| `CAPTAINBI_ACCESS_TOKEN` | inject an existing access token and skip token retrieval |

## Safety

- Secrets and tokens are redacted in dry-run, config display and errors.
- Dangerous POST endpoints require `--confirm`.
- `--dry-run` never sends a request.
- Live contract checks are opt-in via `cbi doctor contract`.
- Prefer `--channel <alias>` over raw OpenChannelId in daily Agent workflows.

## Agent Usage

- Use `--machine --format json` for structured output.
- Use `--summary` before large pulls; use `--output-file` for full data.
- Page-all for `page_rows` endpoints stops on `len(data) < rows`; `max_result` is optional.
- Resume long pulls with `--resume-from-page`.
- Read `error_code`, `kind`, `hint`, `api_code` and `api_msg` on failures.

## Development

```bash
go test ./...
go run ./tools/gen-registry
go build -buildvcs=false -o bin/cbi .
```
