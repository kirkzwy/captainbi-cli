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

## Store Transactions

```bash
cbi --channel <alias> +store-transactions \
  --start <YYYYMMDD> \
  --end <YYYYMMDD> \
  --summary \
  --machine --format json
```

## Set Product Cost

Treat cost and currency fields as financial data. Read the current goods item first and preserve all original values for rollback.

```bash
cbi --channel <alias> finance set-cost \
  --data '{"data":[{"sku":"<sku>","purchasing_cost":"<amount>","purchasing_cost_currency_code":<id>,"fba_cost":<amount>,"fba_cost_currency_code":<id>,"fbm_cost":<amount>,"fbm_cost_currency_code":<id>}]}' \
  --dry-run --machine --format json
```

After explicit approval, execute with `--confirm-request <request_hash>`, then query goods item finance fields and verify every changed value. Do not infer currency IDs.

## Set Operating Expense Rule

Inspect `cbi schema finance.set-rule` because CaptainBI uses legacy field names such as `miaoshu`, `jine`, `kaishi`, `chongfu` and `leixing`. Resolve classification, payment object and applicant IDs with `finance classify` before previewing. Execute only after the user approves amount, recurrence and date; verify through `finance operating-expenses`.
