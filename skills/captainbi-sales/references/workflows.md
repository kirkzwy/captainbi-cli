# Sales Workflows

Use `captainbi-shared` first.

## Recent Orders Summary

```bash
cbi --channel <alias> +orders \
  --start <unix_seconds> \
  --end <unix_seconds> \
  --summary \
  --machine --format json
```

## Full Orders Export

```bash
cbi --channel <alias> sales orders \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --page-all \
  --max-records 1000 \
  --output-file orders.ndjson \
  --format ndjson \
  --machine
```

Empty rows may be valid. Use `partial` and `pages_failed` to decide whether to retry.

## Upload FBM Shipping

This is an asynchronous dangerous write. Never use a real order as a generic smoke test.

```bash
cbi --channel <alias> sales upload-fbm-shipping \
  --data '{"data":[{"amazon_order_id":"<order>","carrier_code":"UPS","shipping_method":"Ground","shipper_tracking_number":"<tracking>","amazon_order_item_code":"<item>","quantity":1}]}' \
  --dry-run --machine --format json
```

After the user approves the exact order, tracking number and quantity, execute with `--confirm-request <request_hash>`. Read `feedId` from the response, then poll `sales fbm-shipping-status --feed-id <feedId>` until `DONE` or a terminal failure. Never retry an unknown network outcome without checking the order and feed status first.
