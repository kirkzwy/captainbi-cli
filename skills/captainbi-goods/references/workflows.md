# Goods Workflows

Use `captainbi-shared` first.

## Product Change Check

```bash
cbi --channel <alias> +goods \
  --modified-since <unix_seconds> \
  --modified-until <unix_seconds> \
  --page-all \
  --max-records 500 \
  --summary \
  --machine --format json
```

For full export:

```bash
cbi --channel <alias> goods list \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --page-all \
  --output-file goods.ndjson \
  --format ndjson \
  --machine
```

## Shop Initialization

```bash
cbi +shops --machine --format json
cbi config channels add <alias> '<open_channel_id>'
```

Treat missing `max_result` as normal. Trust CLI metadata such as `pages_fetched`, `pages_failed` and `partial`.
