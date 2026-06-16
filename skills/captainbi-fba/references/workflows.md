# FBA Workflows

Use `captainbi-shared` first.

## Inventory Summary

```bash
cbi --channel <alias> fba inventory \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --summary \
  --machine --format json
```

## Inventory Export

```bash
cbi --channel <alias> fba inventory \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --page-all \
  --output-file fba-inventory.ndjson \
  --format ndjson \
  --machine
```

Shipment sync is `sync_trigger`; only use `--confirm` after explicit user approval.
