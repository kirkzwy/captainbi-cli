---
name: captainbi-finance
description: Finance reports, transactions, VAT, costs, expenses, claims and profit commands for CaptainBI CLI.
---

# CaptainBI Finance

Read `captainbi-shared` first.

## WHEN

Use for sales/profit reports, store or ASIN finance analysis, transactions, VAT, payment records, claims, storage fees, costs and operating expenses.

## WHEN NOT

Do not use for raw order lists, ad keyword performance, FBA inventory, or review monitoring unless the workflow needs finance correlation.

## Command Choice

- Store daily finance by order dimension: `cbi +finance-daily --channel <alias> --date YYYYMMDD --summary`.
- Store monthly: `cbi finance store-monthly --channel <alias> --report-date YYYYMM`.
- ASIN daily: `cbi finance asin-daily --channel <alias> --report-date YYYYMMDD`.
- Transactions: `cbi finance store-transactions --channel <alias> --page-all`.
- Claims/VAT: `cbi finance claims`, `cbi finance vat`.
- Cost/rule writes require dry-run and `--confirm`.

## Examples

```bash
cbi --channel main +finance-daily --date 20260615 --summary --machine
cbi --channel main finance store-daily --report-date 20260615 --page-all --max-records 100 --machine
```

## Notes

- `report_date` uses `YYYYMMDD`; monthly endpoints use `YYYYMM`.
- Finance reports can have many fields; use `--summary` before full output.
