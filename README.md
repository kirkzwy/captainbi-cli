# captainbi-cli

CaptainBI OpenAPI command-line client. The main command is `cbi`; `captainbi`
is kept as an alias.

## Status

This repository is an initial implementation scaffold for the CaptainBI CLI:

- Go + Cobra single binary.
- OpenAPI -> Registry metadata -> generated service commands.
- Built-in token caching, redaction, 20 req/min rate limiting and 429 retry.
- Generic `api`, generated domain commands, shortcuts, schema and doctor commands.

## Quick Start

```bash
# Build from source after installing Go 1.23+
go build -o bin/cbi .

# Configure credentials. Do not pass secrets as command-line flags.
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin

cbi auth token
cbi +sites
cbi +shops
```

## Command Layers

```bash
# Shortcuts
cbi +shops
cbi +sites
cbi +orders --open-channel-id oc_xxx

# Generated business-domain commands
cbi goods list --open-channel-id oc_xxx --start-modified-time 1700000000 --end-modified-time 1700100000
cbi finance store-daily --open-channel-id oc_xxx --report-date 20240101
cbi ads campaign-report --open-channel-id oc_xxx

# Generic API escape hatch
cbi api GET /v1/open_user/get_site_list

# Schema
cbi schema finance.store-daily
```

## Environment Variables

| Name | Purpose |
| --- | --- |
| `CAPTAINBI_CLIENT_ID` | APPID/client_id |
| `CAPTAINBI_CLIENT_SECRET` | client_secret, preferred for CI or one-off runs |
| `CAPTAINBI_BASE_URL` | API base URL, defaults to `https://openapi.captainbi.com` |
| `CAPTAINBI_OPEN_CHANNEL_ID` | default OpenChannelId |
| `CAPTAINBI_RATE_LIMIT` | requests per minute, defaults to 20 |

## Safety

- Secrets and tokens are redacted in dry-run, config display and errors.
- Dangerous POST endpoints require `--confirm`.
- `--dry-run` never sends a request.
- Live contract checks are opt-in via `cbi doctor contract`.

## Development

```bash
go test ./...
go run ./tools/gen-registry
```
