# captainbi-cli

CaptainBI OpenAPI command-line client. The main command is `cbi`; `captainbi`
is kept as an alias.

## Status

This repository is an Agent-ready CaptainBI CLI:

- Go + Cobra single binary.
- OpenAPI -> Registry metadata -> generated service commands.
- Built-in token caching, OAuth `scope=all`, redaction, 250 req/min rate limiting and 429 retry.
- Generic `api`, generated domain commands, shortcuts, schema and doctor commands.
- Real read-only smoke has passed for auth, sites, shops, goods, orders, finance, ads, FBA and reviews.
- Registry preserves all 65 official response schemas and 36 documented request bodies. Real contract tests encode the 28 GET body fields as query parameters and the 8 POST bodies as multipart.
- Agent writes use payload-bound, expiring dry-run approval hashes and one-channel-at-a-time protection.

## Quick Start

```bash
# Preferred Agent path for this private/internal phase
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.1/captainbi-cli-0.3.1.tgz
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
curl -L -o cbi.tar.gz https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.1/captainbi-cli_0.3.1_darwin_arm64.tar.gz
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
cbi +ads-campaigns --channel main --modified-since 1781424057 --modified-until 1781510457 --type 1 --summary
cbi +ads-campaign-report --channel main --date 20260615 --summary
cbi +reviews --channel main --summary
cbi +store-transactions --channel main --start 20260601 --end 20260615

# Generated business-domain commands
cbi goods list --channel main --start-modified-time 1700000000 --end-modified-time 1700100000
cbi finance store-daily --channel main --report-date 20240101
cbi ads advertise-campaign-report --channel main --report-date 20260615

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
| `CAPTAINBI_RATE_LIMIT` | requests per minute, defaults to 250 for the current paid plan |
| `CAPTAINBI_ACCESS_TOKEN` | inject an existing access token and skip token retrieval |
| `CAPTAINBI_CONFIG_DIR` | private writable directory for config, token cache, locks and write previews |
| `CAPTAINBI_REGISTRY_FILE` | explicit compatible Registry metadata override; normally use `cbi registry update` |
| `CAPTAINBI_WRITE_ALLOWLIST` | comma-separated process-level Agent permissions for registered dangerous writes |

## Safety

- Secrets and tokens are redacted in dry-run, config display and errors.
- Agent writes require `--dry-run`, explicit user approval, then the unchanged request with `--confirm-request <request_hash>`.
- Agent `write_dangerous` and `sync_trigger` commands must also be explicitly enabled with `cbi config write-allowlist add <domain.command>`.
- Approval hashes expire after 15 minutes, are consumed before sending and cannot be replayed.
- Writes cannot use `--channel all`; approve one channel and payload at a time.
- Unknown raw non-GET calls require `--unsafe-raw-write` in addition to the approval flow.
- `--dry-run` never sends a request.
- Live contract checks are opt-in via `cbi doctor contract`.
- Prefer `--channel <alias>` over raw OpenChannelId in daily Agent workflows.
- `--params-file`, `--data-file` and `--channel-file` only read relative regular files inside the current working directory. Pipe absolute-path content through stdin.

## Agent Usage

Persist another account-plan limit with `cbi config rate-limit <requests-per-minute>`; `--rate-limit` and `CAPTAINBI_RATE_LIMIT` remain per-process overrides.

- Use `--machine --format json` for structured output.
- Control commands expose `ok/data/meta` in machine JSON while retaining their legacy top-level fields throughout v0.x. Generated tool artifacts remain unwrapped.
- Use `CBI_AGENT=1` when the host should receive machine-friendly errors by default.
- Use `--summary` before large pulls; use `--output-file` for full data.
- Page-all for `page_rows` endpoints stops on `len(data) < rows`; `max_result` is optional.
- Read `meta.has_more` and the `next_window/next_page/next_offset` cursor to decide whether to continue; pass them back through the matching `--resume-*` flags.
- `--page-all` automatically splits modified-time spans into non-overlapping 31-day windows. For report endpoints, use inclusive `--range-start/--range-end` in matching `YYYYMMDD` or `YYYYMM` format.
- A modified-time request beyond the endpoint window limit is rejected locally unless `--page-all` is present.
- For `--channel all`, read `channels_total/channels_succeeded/channels_failed`, summed `rows`, and `partial`; an all-failed batch exits non-zero with `CHANNEL_BATCH_FAILED`.
- Success output uses `ok/data/meta`; failure output uses `ok/error/meta`.
- Read `error.kind`, `error.subtype`, `error.hint`, `error.api_code` and `error.api_msg` on failures.
- CaptainBI `code != 200` is a failed call even when HTTP status is 200.

## Agent Write Flow

```bash
# 1. Preview only
cbi --channel main goods set-operate-user \
  --goods-id <amazon_goods_id> --operation-user-admin-id <operator_id> \
  --dry-run --machine --format json

# 2. Stop and obtain explicit user approval for the exact preview

# 3. Send the unchanged request with data.approval.request_hash
cbi --channel main goods set-operate-user \
  --goods-id <amazon_goods_id> --operation-user-admin-id <operator_id> \
  --confirm-request <request_hash> --machine --format json
```

After a write, query the affected resource and verify the result. If the payload changes, the hash expires, or the outcome is uncertain, generate a new preview instead of replaying it. For goods write commands, `--goods-id` means the `amazon_goods_id` returned by `cbi goods items`; do not use an undocumented local `id` field.

Maintainers can run the staged real-write acceptance with `scripts/smoke/write_guarded.sh prepare|apply|prepare-restore|restore`. It requires dedicated test fixtures and never crosses an approval boundary automatically.

For a dangerous Agent write, review and enable only the exact registered command before applying the approved preview:

```bash
cbi config write-allowlist add goods.set-group
cbi config write-allowlist list --machine --format json
# Remove the permission when the workflow is finished
cbi config write-allowlist remove goods.set-group
```

## Registry Updates

```bash
cbi registry check --machine --format json
cbi registry update --machine --format json
# Restore the Registry embedded in the binary
cbi registry reset --machine --format json
```

`registry update` installs only metadata that preserves existing commands and does not lower write risk or OpenChannelId requirements. `doctor local` reports the effective/embedded versions, override path and any fallback warning.

## Development

```bash
go test ./...
go test -race ./...
go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.12.2 run ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./...
go run ./tools/gen-registry
go build -buildvcs=false -o bin/cbi .
```
