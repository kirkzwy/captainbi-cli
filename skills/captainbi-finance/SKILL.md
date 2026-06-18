---
name: captainbi-finance
description: "Finance reports, transactions, VAT, costs, expenses, claims and profit commands for CaptainBI CLI. WHEN use for: store/ASIN profit, daily/monthly finance reports, transactions, VAT, claims, costs, and operating expenses. WHEN NOT: do not use for raw order lists, ad keyword performance, FBA inventory, or reviews unless finance correlation is required."
---

# CaptainBI Finance

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for sales/profit reports, store or ASIN finance analysis, transactions, VAT, payment records, claims, storage fees, costs and operating expenses.

## WHEN NOT

Do not use for raw order lists, ad keyword performance, FBA inventory, or review monitoring unless the workflow needs finance correlation.

## Command Choice

- Store daily finance by order dimension: `cbi +finance-daily --channel <alias> --date YYYYMMDD --summary`.
- Store monthly: `cbi finance store-monthly --channel <alias> --report-date YYYYMM`.
- ASIN daily: `cbi finance asin-daily --channel <alias> --report-date YYYYMMDD`.
- Transactions: `cbi +store-transactions --channel <alias> --start YYYYMMDD --end YYYYMMDD --page-all`.
- Claims/VAT: `cbi finance claims`, `cbi finance vat`.
- Cost/rule writes require dry-run and `--confirm`.

## Examples

```bash
cbi --channel main +finance-daily --date 20260615 --summary --machine
cbi --channel main finance store-daily --report-date 20260615 --page-all --max-records 100 --machine
cbi --channel main +store-transactions --start 20260601 --end 20260615 --summary --machine
```

## Notes

- `report_date` uses `YYYYMMDD`; monthly endpoints use `YYYYMM`.
- Finance reports can have many fields; use `--summary` before full output.
