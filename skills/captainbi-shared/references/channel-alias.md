# Channel Alias Rules

Discover shops:

```bash
cbi +shops --machine --format json
```

Save aliases:

```bash
cbi config channels add main '<open_channel_id>'
cbi config channels list --machine --format json
```

Daily calls:

```bash
cbi --channel main +orders --start <unix_seconds> --end <unix_seconds> --summary --machine --format json
cbi --channel all +finance-daily --date <YYYYMMDD> --summary --machine --format json
```

Rules:

- Use readable aliases such as `main`, `us-main`, `uk-main`.
- Do not show raw OpenChannelId in user-facing text.
- If `--channel all` returns partial success, keep successful shop results and report failed aliases with their `hint`.
- If no alias exists, ask the user to run `+shops` and choose a shop.
