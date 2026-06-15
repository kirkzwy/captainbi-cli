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
| token 结构 | TODO | 确认 `access_token/token_type/expires_in` 是否在顶层或 `data` 内 |
| 错误结构 | TODO | 确认 CaptainBI `code/msg/request_id` 字段名 |
| 分页字段 | TODO | 确认 `data` 是否总为数组，`max_result` 是否稳定表示总数 |
| 时间戳单位 | TODO | 确认 `start_modified_time/end_modified_time` 使用秒还是毫秒 |
| OpenChannelId 要求 | TODO | 确认各域只读接口是否均需店铺级 OpenChannelId |
| 429 行为 | TODO | 确认是否返回 `Retry-After` 或业务错误码 |

## 已内置保护

- `scripts/smoke/read_only.sh` 只调用只读接口。
- 输出默认使用 `--summary`，不输出完整业务明细。
- 凭证和 OpenChannelId 只从环境变量或本地配置读取，不进入仓库。
