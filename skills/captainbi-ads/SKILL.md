---
name: captainbi-ads
description: Advertising structure and report commands for CaptainBI CLI.
---

# CaptainBI Ads

Read `captainbi-shared` first.
For task recipes, read `references/workflows.md`.

## WHEN

Use for ad campaigns, ad groups, keywords, placement reports, search term reports, ACOS/ROAS/cost/click/impression analysis.

## WHEN NOT

Do not use for organic orders, product master data, FBA inventory, or reviews unless the task compares ad performance with those domains.

## Command Choice

- Campaign structure: `cbi ads advertise-campaign --channel <alias>`.
- Campaign report: `cbi ads advertise-campaign-report --channel <alias> --summary`.
- Ad group report: `cbi ads advertise-group-report --channel <alias>`.
- Keyword report: `cbi ads advertise-keyword-report --channel <alias>`.
- Search term reports: `cbi ads search-term-placement-report`, `cbi ads search-term-keywords-report`.

## Examples

```bash
cbi --channel main ads advertise-campaign-report --summary --machine
cbi --channel main ads advertise-keyword-report --format json --output-file ads-keywords.json
```

## Notes

- Empty ad report rows can be valid. Check `ok` and `rows`.
- For “ACOS 异常” questions, start with campaign report, then drill down to ad group/keyword reports.
