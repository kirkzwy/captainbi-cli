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
# Preferred Agent path for this private/internal phase
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.2.4/captainbi-cli-0.2.4.tgz
cbi --version
cbi doctor local --machine --format json

# Optional skill installation when the host supports it
npx skills add kirkzwy/captainbi-cli -y -g

# Configure credentials. Do not pass secrets as command-line flags.
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive

cbi auth token
cbi +sites
cbi +shops
cbi config channels add main '<open_channel_id>'
cbi --channel main +goods --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
```

For private repositories or GitHub rate limits, configure access before install:

```bash
export GITHUB_TOKEN='<github_token>'
export CAPTAINBI_CLI_GITHUB_TOKEN='<github_token>'
```

For proxy-based networks:

```bash
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
export ALL_PROXY=http://127.0.0.1:7890
export NODE_USE_ENV_PROXY=1
```

Fallback without npm GitHub install:

```bash
curl -L -o cbi.tar.gz https://github.com/kirkzwy/captainbi-cli/releases/download/v0.2.4/captainbi-cli_0.2.4_darwin_arm64.tar.gz
tar -xzf cbi.tar.gz
./cbi --version
```

Build from source only for development:

```bash
go build -buildvcs=false -o bin/cbi .
```

## Command Layers

```bash
# Shortcuts
cbi +shops
cbi +sites
cbi +orders --channel main --start 1781424057 --end 1781510457
cbi +goods --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +finance-daily --channel main --date 20260615
cbi +inventory --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +ads-campaign-report --channel main --summary
cbi +reviews --channel main --summary
cbi +store-transactions --channel main --start 20260601 --end 20260615

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
- Use `CBI_AGENT=1` when the host should receive machine-friendly errors by default.
- Use `--summary` before large pulls; use `--output-file` for full data.
- Page-all for `page_rows` endpoints stops on `len(data) < rows`; `max_result` is optional.
- Read `meta.has_more` and `meta.next_page` to decide whether to continue; resume long pulls with `--resume-from-page`.
- Success output uses `ok/data/meta`; failure output uses `ok/error/meta`.
- Read `error.kind`, `error.subtype`, `error.hint`, `error.api_code` and `error.api_msg` on failures.

## Development

```bash
go test ./...
go run ./tools/gen-registry
go build -buildvcs=false -o bin/cbi .
```
