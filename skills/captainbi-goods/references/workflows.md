# Goods Workflows

Use `captainbi-shared` first.

## Product Change Check

```bash
cbi --channel <alias> +goods \
  --modified-since <unix_seconds> \
  --modified-until <unix_seconds> \
  --page-all \
  --max-records 500 \
  --summary \
  --machine --format json
```

For full export:

```bash
cbi --channel <alias> goods list \
  --start-modified-time <unix_seconds> \
  --end-modified-time <unix_seconds> \
  --page-all \
  --output-file goods.ndjson \
  --format ndjson \
  --machine
```

## Shop Initialization

```bash
cbi +shops --machine --format json
cbi config channels add <alias> '<open_channel_id>'
```

Treat missing `max_result` as normal. Trust CLI metadata such as `pages_fetched`, `pages_failed` and `partial`.

## Set Product Operator

```bash
cbi --channel <alias> goods set-operate-user \
  --goods-id <amazon_goods_id> \
  --operation-user-admin-id <operator_id> \
  --dry-run --machine --format json
```

`--goods-id` is the `amazon_goods_id` returned by `goods items`, not an undocumented local `id` field. Show the preview to the user. After approval, rerun the unchanged command with `--confirm-request <request_hash>`, then query `goods items` and verify `operation_user_admin_id`. Preserve the old operator ID before changing it when rollback may be needed.

## Set Product Group

```bash
cbi goods set-group --goods-id '<amazon_goods_id>' --group-id '<group_id>' \
  --dry-run --machine --format json
```

This endpoint is account-level and does not require OpenChannelId. After approval and execution, query `goods items` to verify `group_id`. Preserve the original group for rollback.

## Add Or Edit Group

```bash
cbi goods edit-group --group-name '<name>' \
  --dry-run --machine --format json
```

Add omits `--group-id`; edit includes it. Query `cbi goods groups --channel <alias>` before and after. Creating a group has no delete endpoint, so do not use production data for smoke tests.

## Set Shop Operation Mode

Use `cbi --channel <alias> goods set-shop-operation-mode`. Omitting `--operation-user-admin-id` selects shop mode; providing it selects goods mode. Always record the current mode from `+shops`, preview, approve, execute and restore when testing.
