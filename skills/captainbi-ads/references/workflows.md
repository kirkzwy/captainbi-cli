# Ads Workflows

Use `captainbi-shared` first.

## Campaign Spend Summary

```bash
cbi --channel <alias> ads advertise-campaign-report \
  --summary \
  --machine --format json
```

## Keyword Drilldown

```bash
cbi --channel <alias> ads advertise-keyword-report \
  --page-all \
  --max-records 1000 \
  --output-file ads-keywords.ndjson \
  --format ndjson \
  --machine
```

For ACOS/ROAS issues, start at campaign level, then ad group, then keyword/search term.
