# Authentication Setup

CaptainBI OAuth uses `client_credentials` with `scope=all`. The CLI sends `scope=all` automatically.

Preferred setup:

```bash
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive
```

Headless alternatives:

```bash
cbi config init --client-id "$CAPTAINBI_CLIENT_ID" --client-secret-from-env --non-interactive
cbi config init --client-id "$CAPTAINBI_CLIENT_ID" --client-secret-file /path/to/secret.txt --non-interactive
CAPTAINBI_ACCESS_TOKEN='<token>' cbi +sites --machine
```

Recovery:

- `invalid_client`: verify APPID/client_secret, then rerun config init.
- expired token or 401: run `cbi auth token --machine`.
- missing keychain in CI: use `CAPTAINBI_CLIENT_SECRET`, `CAPTAINBI_ACCESS_TOKEN`, or `--client-secret-file`.

Never print raw token or client_secret.
