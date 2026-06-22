#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
CBI_BIN="${CBI_BIN:-$ROOT/bin/cbi}"
if [[ ! -x "$CBI_BIN" ]]; then
  echo "cbi binary not found. Run: go build -buildvcs=false -o bin/cbi ." >&2
  exit 1
fi

MODE="${1:-}"
case "$MODE" in
  prepare|apply|prepare-restore|restore) ;;
  *) echo "usage: $0 prepare|apply|prepare-restore|restore" >&2; exit 2 ;;
esac

required=(
  CAPTAINBI_WRITE_CHANNEL_ALIAS
  CAPTAINBI_WRITE_AMAZON_GOODS_ID
  CAPTAINBI_WRITE_TARGET_GROUP_ID
  CAPTAINBI_WRITE_ORIGINAL_GROUP_ID
  CAPTAINBI_WRITE_SHIPMENT_ID
)
SAFE_KIND="${CAPTAINBI_WRITE_SAFE_KIND:-shop-mode}"
case "$SAFE_KIND" in
  shop-mode) ;;
  operator)
    required+=(CAPTAINBI_WRITE_TARGET_OPERATOR_ID CAPTAINBI_WRITE_ORIGINAL_OPERATOR_ID)
    ;;
  *) echo "CAPTAINBI_WRITE_SAFE_KIND must be shop-mode or operator" >&2; exit 2 ;;
esac
for name in "${required[@]}"; do
  if [[ -z "${!name:-}" ]]; then
    echo "$name is required" >&2
    exit 2
  fi
done

CHANNEL="$CAPTAINBI_WRITE_CHANNEL_ALIAS"
GOODS_ID="$CAPTAINBI_WRITE_AMAZON_GOODS_ID"
TARGET_OPERATOR="${CAPTAINBI_WRITE_TARGET_OPERATOR_ID:-}"
ORIGINAL_OPERATOR="${CAPTAINBI_WRITE_ORIGINAL_OPERATOR_ID:-}"
TARGET_GROUP="$CAPTAINBI_WRITE_TARGET_GROUP_ID"
ORIGINAL_GROUP="$CAPTAINBI_WRITE_ORIGINAL_GROUP_ID"
SHIPMENT_ID="$CAPTAINBI_WRITE_SHIPMENT_ID"
OUTPUT_DIR="${CAPTAINBI_WRITE_OUTPUT_DIR:-${TMPDIR:-/tmp}/captainbi-write-smoke}"
mkdir -p "$OUTPUT_DIR"
chmod 700 "$OUTPUT_DIR"
export CBI_AGENT=1

read -r START_TS END_TS < <(python3 - <<'PY'
from datetime import datetime, timedelta, timezone
now = datetime.now(timezone.utc)
print(int((now - timedelta(days=30)).timestamp()), int(now.timestamp()))
PY
)

run_json() {
  local output="$1"
  shift
  "$CBI_BIN" --machine --format json "$@" >"$OUTPUT_DIR/$output"
  chmod 600 "$OUTPUT_DIR/$output"
}

require_allowlist() {
  local configured
  configured="$("$CBI_BIN" config write-allowlist list --machine --format json)"
  for ref in "$@"; do
    if ! jq -e --arg ref "$ref" '.data | index($ref) != null' <<<"$configured" >/dev/null; then
      echo "Agent write policy blocks $ref. Review it, then run: cbi config write-allowlist add $ref" >&2
      exit 2
    fi
  done
}

preview_targets() {
  if [[ "$SAFE_KIND" == operator ]]; then
    run_json operator-preview.json --channel "$CHANNEL" goods set-operate-user \
      --goods-id "$GOODS_ID" --operation-user-admin-id "$TARGET_OPERATOR" --dry-run
  else
    run_json shop-mode-preview.json --channel "$CHANNEL" goods set-shop-operation-mode --dry-run
  fi
  run_json group-preview.json goods set-group \
    --goods-id "$GOODS_ID" --group-id "$TARGET_GROUP" --dry-run
  run_json shipment-preview.json --channel "$CHANNEL" fba sync-shipment \
    --shipment-ids "$SHIPMENT_ID" --dry-run
}

preview_restore() {
  if [[ "$SAFE_KIND" == operator ]]; then
    run_json operator-restore-preview.json --channel "$CHANNEL" goods set-operate-user \
      --goods-id "$GOODS_ID" --operation-user-admin-id "$ORIGINAL_OPERATOR" --dry-run
  fi
  run_json group-restore-preview.json goods set-group \
    --goods-id "$GOODS_ID" --group-id "$ORIGINAL_GROUP" --dry-run
}

verify_state() {
  run_json shops.json +shops
  run_json goods-items.json --channel "$CHANNEL" goods items \
    --page 1 --rows 100 --start-modified-time "$START_TS" --end-modified-time "$END_TS" --page-all
  run_json shipments.json --channel "$CHANNEL" fba shipments \
    --page 1 --rows 100 --start-modified-time "$START_TS" --end-modified-time "$END_TS" --page-all
}

case "$MODE" in
  prepare)
    verify_state
    preview_targets
    echo "Write previews created in $OUTPUT_DIR. Stop and obtain explicit user approval." >&2
    ;;
  apply)
    if [[ "$SAFE_KIND" == operator ]]; then
      : "${CAPTAINBI_WRITE_OPERATOR_HASH:?CAPTAINBI_WRITE_OPERATOR_HASH is required}"
    else
      : "${CAPTAINBI_WRITE_SHOP_MODE_HASH:?CAPTAINBI_WRITE_SHOP_MODE_HASH is required}"
    fi
    : "${CAPTAINBI_WRITE_GROUP_HASH:?CAPTAINBI_WRITE_GROUP_HASH is required}"
    : "${CAPTAINBI_WRITE_SHIPMENT_HASH:?CAPTAINBI_WRITE_SHIPMENT_HASH is required}"
    require_allowlist goods.set-group fba.sync-shipment
    if [[ "$SAFE_KIND" == operator ]]; then
      run_json operator-result.json --channel "$CHANNEL" goods set-operate-user \
        --goods-id "$GOODS_ID" --operation-user-admin-id "$TARGET_OPERATOR" --confirm-request "$CAPTAINBI_WRITE_OPERATOR_HASH"
    else
      run_json shop-mode-result.json --channel "$CHANNEL" goods set-shop-operation-mode \
        --confirm-request "$CAPTAINBI_WRITE_SHOP_MODE_HASH"
    fi
    run_json group-result.json goods set-group \
      --goods-id "$GOODS_ID" --group-id "$TARGET_GROUP" --confirm-request "$CAPTAINBI_WRITE_GROUP_HASH"
    run_json shipment-result.json --channel "$CHANNEL" fba sync-shipment \
      --shipment-ids "$SHIPMENT_ID" --confirm-request "$CAPTAINBI_WRITE_SHIPMENT_HASH"
    verify_state
    echo "Writes sent and verification data saved in $OUTPUT_DIR. Review before preparing rollback." >&2
    ;;
  prepare-restore)
    preview_restore
    echo "Rollback previews created in $OUTPUT_DIR. Stop and obtain explicit user approval." >&2
    ;;
  restore)
    if [[ "$SAFE_KIND" == operator ]]; then
      : "${CAPTAINBI_WRITE_OPERATOR_RESTORE_HASH:?CAPTAINBI_WRITE_OPERATOR_RESTORE_HASH is required}"
    fi
    : "${CAPTAINBI_WRITE_GROUP_RESTORE_HASH:?CAPTAINBI_WRITE_GROUP_RESTORE_HASH is required}"
    require_allowlist goods.set-group
    if [[ "$SAFE_KIND" == operator ]]; then
      run_json operator-restore-result.json --channel "$CHANNEL" goods set-operate-user \
        --goods-id "$GOODS_ID" --operation-user-admin-id "$ORIGINAL_OPERATOR" --confirm-request "$CAPTAINBI_WRITE_OPERATOR_RESTORE_HASH"
    fi
    run_json group-restore-result.json goods set-group \
      --goods-id "$GOODS_ID" --group-id "$ORIGINAL_GROUP" --confirm-request "$CAPTAINBI_WRITE_GROUP_RESTORE_HASH"
    verify_state
    echo "Rollback sent and verification data saved in $OUTPUT_DIR." >&2
    ;;
esac
