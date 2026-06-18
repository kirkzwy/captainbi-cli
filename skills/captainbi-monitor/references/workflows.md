# Monitor Workflows

Use `captainbi-shared` first.

## Reviews Watch

```bash
cbi --channel <alias> +reviews \
  --summary \
  --machine --format json
```

## Bad Review Monitor

```bash
cbi --channel <alias> monitor bad-review-summary \
  --page-all \
  --max-records 500 \
  --machine --format json
```

Empty review rows can mean there were no matching records. Treat it as success when `ok=true`.
