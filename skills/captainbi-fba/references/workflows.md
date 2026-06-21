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

## Sync Shipment

```bash
cbi --channel <alias> fba sync-shipment \
  --shipment-ids '<shipment_id[,shipment_id...]>' \
  --dry-run --machine --format json
```

The command accepts at most 5000 comma-separated IDs. Show the exact IDs to the user and execute the unchanged request with `--confirm-request <request_hash>` only after approval. Query `fba shipments` after the trigger; synchronization can be asynchronous, so report pending state rather than repeatedly triggering it.
