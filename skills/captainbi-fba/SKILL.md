---
name: captainbi-fba
description: FBA inventory, fees, shipments and monitor commands for CaptainBI CLI.
---

# CaptainBI FBA

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for FBA inventory, FBA shipment lists, abnormal distribution fees, storage fees, and FBA ASIN monitoring.

## WHEN NOT

Do not use for customer orders, product reviews, advertising, or finance reports unless inventory context is needed.

## Command Choice

- Inventory: `cbi fba inventory --channel <alias> --start-modified-time <ts> --end-modified-time <ts> --summary`.
- Shipments: `cbi fba shipments --channel <alias>`.
- Storage fee: `cbi fba storage-fee --channel <alias>`.
- ASIN monitor: `cbi fba asin-monitor --channel <alias>`.
- Sync shipment is `sync_trigger`; use `--dry-run` first and `--confirm` only after explicit approval.

## Examples

```bash
cbi --channel main fba inventory --page 1 --rows 100 --start-modified-time 1781424057 --end-modified-time 1781510457 --summary --machine
cbi --channel main fba inventory --start-modified-time 1781424057 --end-modified-time 1781510457 --page-all --max-records 100 --machine
```

## Notes

- Inventory smoke returned real data with `data` as an array.
- Use `--output-file` for inventory exports.
