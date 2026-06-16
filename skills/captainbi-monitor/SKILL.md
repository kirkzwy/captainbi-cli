---
name: captainbi-monitor
description: Review, feedback, bad review and hijack monitoring commands for CaptainBI CLI.
---

# CaptainBI Monitor

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for reviews, feedback, bad review monitoring, follow-up/hijack records, and business reports from CaptainBI monitoring endpoints.

## WHEN NOT

Do not use for order status, finance reports, advertising metrics, or FBA inventory unless monitoring data needs correlation.

## Command Choice

- Reviews: `cbi monitor reviews --channel <alias> --page 1 --rows 100`.
- Feedback: `cbi monitor feedback --channel <alias>`.
- Bad review summary: `cbi monitor bad-review-summary --channel <alias>`.
- Hijacked/followup: `cbi monitor hijacked-record`, `cbi monitor followup`.
- Business report: `cbi monitor business-report --channel <alias>`.

## Examples

```bash
cbi --channel main monitor reviews --page 1 --rows 100 --summary --machine
cbi --channel main monitor bad-review-summary --page-all --max-records 100 --machine
```

## Notes

- Review smoke returned `0` rows successfully; treat empty rows as “no data in range”, not failure.
- Use `goods list` if review output needs product title/ASIN enrichment.
