---
name: captainbi-finance
description: Finance reports, transactions, VAT, costs, expenses and claims commands for CaptainBI CLI.
---

# CaptainBI Finance

Read `captainbi-shared` first.

- Store daily report: `cbi finance store-daily --report-date YYYYMMDD`
- Store monthly report: `cbi finance store-monthly --report-date YYYYMM`
- ASIN report: `cbi finance asin-daily` or `cbi finance asin-monthly`
- Transactions: `cbi finance store-transactions` or `cbi finance asin-transactions`
- VAT: `cbi finance vat`
- Payment and performance: `payment-record`, `store-performance`, `storewide-performance`
- Writes: `set-cost` and `set-rule` require `--confirm`.
