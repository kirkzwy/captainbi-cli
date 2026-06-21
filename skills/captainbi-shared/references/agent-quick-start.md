# CaptainBI Agent Quick Start

Use this path when a new Agent needs daily CaptainBI read access and explicitly approved writes.

1. Install the CLI:

```bash
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.0/captainbi-cli-0.3.0.tgz
cbi --version
cbi doctor local --machine --format json
```

For private repositories or GitHub rate limits:

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

2. Install skills when the host supports skill installation:

```bash
npx skills add kirkzwy/captainbi-cli -y -g
```

If the host does not support that installer, load the `skills/` directory from this repository.

3. Configure credentials without putting secrets in command history:

```bash
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive
```

4. Fetch token and discover shops:

```bash
cbi auth token --machine --format json
cbi +shops --machine --format json
```

5. Save a channel alias:

```bash
cbi config channels add main '<open_channel_id>'
cbi doctor local --machine --format json
```

6. Run the first read-only task:

```bash
cbi --channel main +goods --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
cbi --channel main +inventory --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
cbi --channel main +ads-campaign-report --date <YYYYMMDD> --summary --machine --format json
cbi --channel main +reviews --summary --machine --format json
```

Agent defaults:

- Prefer `--machine --format json`.
- Prefer `--channel <alias>` instead of raw OpenChannelId.
- Use `--summary` before large pulls.
- Use `--output-file` for full exports.
- Do not run write commands unless the user explicitly approves the exact action.
- Keep file inputs under cwd and use relative paths. Pipe absolute-path content through `--params-stdin` or `--data-stdin`.
- If `doctor local` reports a Registry warning, run `cbi registry check`; use `cbi registry update` only after reviewing the result.

7. For an explicitly requested write, inspect its schema and generate a preview:

```bash
cbi schema goods.set-operate-user --machine --format json
cbi --channel main goods set-operate-user \
  --goods-id <amazon_goods_id> \
  --operation-user-admin-id <operator_id> \
  --dry-run --machine --format json
```

Stop here. Show the preview to the user. After the user approves the exact body and channel, run the unchanged command with the current hash:

```bash
cbi --channel main goods set-operate-user \
  --goods-id <amazon_goods_id> \
  --operation-user-admin-id <operator_id> \
  --confirm-request <request_hash> \
  --machine --format json
```

After success, query the affected resource and report the verified result. A changed, expired or reused hash requires a new dry-run.
