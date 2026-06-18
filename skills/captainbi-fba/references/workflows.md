# FBA Workflows

Use `captainbi-shared` first.

## Inventory Summary

```bash
cbi --channel <alias> +inventory \
  --modified-since <unix_seconds> \
  --modified-until <unix_seconds> \
  --summary \
  --machine --format json
```

## Inventory Export

```bash
cbi --channel <alias> +inventory \
  --modified-since <unix_seconds> \
  --modified-until <unix_seconds> \
  --page-all \
  --output-file fba-inventory.ndjson \
  --format ndjson \
  --machine
```

Shipment sync is `sync_trigger`; only use `--confirm` after explicit user approval.
