---
name: captainbi-ads
description: "Advertising structure and report commands for CaptainBI CLI. WHEN use for: campaigns, ad groups, keywords, placement, search terms, ACOS/ROAS, clicks, impressions, spend, and campaign diagnostics. WHEN NOT: do not use for organic orders, product master data, FBA inventory, finance reports, or reviews unless comparing with ad performance."
---

# CaptainBI Ads

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for ad campaigns, ad groups, keywords, placement reports, search term reports, ACOS/ROAS/cost/click/impression analysis.

## WHEN NOT

Do not use for organic orders, product master data, FBA inventory, or reviews unless the task compares ad performance with those domains.

## Command Choice

- Campaign structure: `cbi --channel <alias> +ads-campaigns --modified-since <ts> --modified-until <ts> --type <1|2|3> --summary --machine`.
- Campaign report: `cbi --channel <alias> +ads-campaign-report --date <YYYYMMDD> --summary --machine`.
- Ad group report: `cbi ads advertise-group-report --channel <alias>`.
- Keyword report: `cbi ads advertise-keyword-report --channel <alias>`.
- Search term reports: `cbi ads search-term-placement-report`, `cbi ads search-term-keywords-report`.

## Examples

```bash
cbi --channel main +ads-campaigns --modified-since 1781424057 --modified-until 1781510457 --type 1 --summary --machine
cbi --channel main +ads-campaign-report --date 20260615 --summary --machine
cbi --channel main ads advertise-keyword-report --format json --output-file ads-keywords.json
```

## Notes

- Empty ad report rows can be valid. Check `ok` and `rows`.
- Advertising type is required: `1` sponsored products, `2` sponsored brands, `3` sponsored display.
- For “ACOS 异常” questions, start with campaign report, then drill down to ad group/keyword reports.
