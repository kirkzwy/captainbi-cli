# Sales Workflows

Use `captainbi-shared` first.

## Recent Orders Summary

```bash
cbi --channel <alias> +orders \
  --start <unix_seconds> \
  --end <unix_seconds> \
  --summary \
  --machine --format json
```

## Full Orders Export

```bash
cbi --channel <alias> sales orders \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --page-all \
  --max-records 1000 \
  --output-file orders.ndjson \
  --format ndjson \
  --machine
```

Empty rows may be valid. Use `partial` and `pages_failed` to decide whether to retry.
