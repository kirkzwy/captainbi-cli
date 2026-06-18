---
name: captainbi-fba
description: "FBA inventory, fees, shipments and monitor commands for CaptainBI CLI. WHEN use for: FBA inventory, shipment lists, storage fees, abnormal distribution fees, and FBA ASIN monitoring. WHEN NOT: do not use for customer orders, product reviews, advertising metrics, or finance reports unless inventory context is needed."
---

# CaptainBI FBA

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for FBA inventory, FBA shipment lists, abnormal distribution fees, storage fees, and FBA ASIN monitoring.

## WHEN NOT

Do not use for customer orders, product reviews, advertising, or finance reports unless inventory context is needed.

## Command Choice

- Inventory: `cbi +inventory --channel <alias> --modified-since <ts> --modified-until <ts> --summary --machine`.
- Shipments: `cbi fba shipments --channel <alias>`.
- Storage fee: `cbi fba storage-fee --channel <alias>`.
- ASIN monitor: `cbi fba asin-monitor --channel <alias>`.
- Sync shipment is `sync_trigger`; use `--dry-run` first and `--confirm` only after explicit approval.

## Examples

```bash
cbi --channel main +inventory --modified-since 1781424057 --modified-until 1781510457 --summary --machine
cbi --channel main +inventory --modified-since 1781424057 --modified-until 1781510457 --page-all --max-records 100 --machine
```

## Notes

- Inventory smoke returned real data with `data` as an array.
- Use `--output-file` for inventory exports.
