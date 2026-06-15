#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CBI_BIN="${CBI_BIN:-$ROOT/bin/cbi}"
if [[ ! -x "$CBI_BIN" ]]; then
  CBI_BIN="$ROOT/cbi"
fi
if [[ ! -x "$CBI_BIN" ]]; then
  echo "cbi binary not found. Run: go build -o bin/cbi ." >&2
  exit 1
fi

CHANNEL="${CAPTAINBI_SMOKE_OPEN_CHANNEL_ID:-${CAPTAINBI_OPEN_CHANNEL_ID:-}}"
if [[ -z "$CHANNEL" ]]; then
  echo "CAPTAINBI_SMOKE_OPEN_CHANNEL_ID or CAPTAINBI_OPEN_CHANNEL_ID is required" >&2
  exit 2
fi

read -r START_TS END_TS REPORT_DATE < <(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
now = datetime.now(timezone.utc)
start = now - timedelta(days=1)
print(int(start.timestamp()), int(now.timestamp()), now.strftime("%Y%m%d"))
PY
)

run() {
  local redacted=()
  local redact_next=0
  for arg in "$@"; do
    if [[ "$redact_next" == "1" ]]; then
      redacted+=("***")
      redact_next=0
      continue
    fi
    redacted+=("$arg")
    if [[ "$arg" == "--open-channel-id" ]]; then
      redact_next=1
    fi
  done
  echo "==> cbi ${redacted[*]}" >&2
  "$CBI_BIN" --machine --format json --summary "$@"
}

run auth token
run +sites
run +shops
run --open-channel-id "$CHANNEL" goods list --page 1 --rows 20 --start-modified-time "$START_TS" --end-modified-time "$END_TS"
run --open-channel-id "$CHANNEL" sales orders --page 1 --rows 20 --start-modified-time "$START_TS" --end-modified-time "$END_TS"
run --open-channel-id "$CHANNEL" finance store-daily --page 1 --rows 20 --report-date "$REPORT_DATE"
run --open-channel-id "$CHANNEL" ads advertise-campaign-report
run --open-channel-id "$CHANNEL" fba inventory --page 1 --rows 20 --start-modified-time "$START_TS" --end-modified-time "$END_TS"
run --open-channel-id "$CHANNEL" monitor reviews --page 1 --rows 20
