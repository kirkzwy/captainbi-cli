# CaptainBI Contract Notes

> 只记录人工 smoke 的结论，不粘贴真实 token、OpenChannelId、店铺名、订单号、ASIN/SKU 明细或完整响应样例。

## Smoke 命令

```bash
go build -o bin/cbi .
CAPTAINBI_SMOKE_OPEN_CHANNEL_ID=*** scripts/smoke/read_only.sh
```

## 待人工确认

| 项目 | 结论 | 备注 |
| --- | --- | --- |
| token 请求参数 | 已确认 | `scope=all` 为必填；缺少时 token 接口返回 `invalid_client` |
| token 结构 | 部分确认 | CLI 可解析并缓存 `token_type=bearer`、约 7200 秒有效期；原始 JSON 层级仍需二次确认 |
| 错误结构 | 部分确认 | token 缺少 `scope=all` 时返回 OAuth 错误：`error=invalid_client`、`error_description=Invalid client authentication` |
| 分页字段 | 部分确认 | `goods list` 单页响应 `data` 为数组；该接口未返回 `max_result`，不能强依赖总数字段 |
| 时间戳单位 | 已确认 | `start_modified_time/end_modified_time` 使用秒级 Unix timestamp 可成功返回数据 |
| OpenChannelId 要求 | 部分确认 | `+sites`、`+shops` 不需要；goods/sales/finance/ads/fba/monitor 本次 smoke 使用店铺级 OpenChannelId 均成功 |
| 429 行为 | TODO | 未触发限流；仍需确认是否返回 `Retry-After` 或业务错误码 |

## 2026-06-15 只读 Smoke 结果

执行命令：

```bash
CAPTAINBI_SMOKE_OPEN_CHANNEL_ID=*** scripts/smoke/read_only.sh
```

结果摘要：

| 步骤 | 结果 | 行数 |
| --- | --- | ---: |
| `auth token` | 成功 | - |
| `+sites` | 成功 | 23 |
| `+shops` | 成功 | 1 |
| `goods list` | 成功 | 20 |
| `sales orders` | 成功 | 20 |
| `finance store-daily` | 成功 | 1 |
| `ads advertise-campaign-report` | 成功 | 0 |
| `fba inventory` | 成功 | 20 |
| `monitor reviews` | 成功 | 0 |

## 已内置保护

- `scripts/smoke/read_only.sh` 只调用只读接口。
- 输出默认使用 `--summary`，不输出完整业务明细。
- 凭证和 OpenChannelId 只从环境变量或本地配置读取，不进入仓库。
