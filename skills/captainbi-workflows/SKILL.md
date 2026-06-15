---
name: captainbi-workflows
description: Cross-domain CaptainBI CLI workflows for daily reports, inventory summaries and advertising diagnostics.
---

# CaptainBI Workflows

Read `captainbi-shared` first, then the relevant domain skills.

## Daily Store Snapshot

Use when the user asks for a daily store overview.

```bash
cbi --channel main +finance-daily --date YYYYMMDD --summary --machine
cbi --channel main +orders --start <start_ts> --end <end_ts> --summary --machine
cbi --channel main ads advertise-campaign-report --summary --machine
```

## Inventory Check

Use when the user asks for stock, FBA inventory, or replenishment context.

```bash
cbi --channel main fba inventory --start-modified-time <start_ts> --end-modified-time <end_ts> --summary --machine
cbi --channel main +goods --modified-since <start_ts> --modified-until <end_ts> --summary --machine
```

## Advertising Diagnosis

Use when the user asks why ACOS/cost/ROAS changed.

```bash
cbi --channel main ads advertise-campaign-report --summary --machine
cbi --channel main ads advertise-keyword-report --format json --output-file ads-keywords.json
```

## Error Recovery

- Missing channel: run `cbi +shops`, then `cbi config channels add <alias> <open_channel_id>`.
- Missing date/time: ask for the date range, then convert modified time windows to second-level Unix timestamps.
- Large data: use `--summary` first, then `--output-file`.
