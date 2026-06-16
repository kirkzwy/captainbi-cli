# Finance Workflows

Use `captainbi-shared` first.

## Store Daily Finance

```bash
cbi --channel <alias> +finance-daily \
  --date <YYYYMMDD> \
  --summary \
  --machine --format json
```

## ASIN Daily Finance

```bash
cbi --channel <alias> finance asin-daily \
  --report-date <YYYYMMDD> \
  --page-all \
  --max-records 1000 \
  --output-file finance-asin.ndjson \
  --format ndjson \
  --machine
```

For profit or fee questions, start with store daily/monthly reports before drilling into ASIN-level data.
