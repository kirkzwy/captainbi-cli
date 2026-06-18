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
| 分页字段 | 已确认 | `goods list` 单页响应 `data` 为数组；该接口未返回 `max_result`，分页不能强依赖总数字段 |
| 时间戳单位 | 已确认 | `start_modified_time/end_modified_time` 使用秒级 Unix timestamp 可成功返回数据 |
| OpenChannelId 要求 | 部分确认 | `+sites`、`+shops` 不需要；goods/sales/finance/ads/fba/monitor 本次 smoke 使用店铺级 OpenChannelId 均成功 |
| 429 行为 | 部分确认 | 快速请求 `get_site_list` 30 次均返回 200，未触发 429；仍不能据此判断无全局限流 |

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

## 2026-06-15 错误契约 Smoke 结果

| 场景 | 结果 | 结构结论 |
| --- | --- | --- |
| 缺 OpenChannelId | 本地结构化错误 | `error_code=BUSINESS`、`kind=business`、有 `hint` |
| 无效 OpenChannelId | CaptainBI 返回 401 | `api_code=100903`、`api_msg=open_channel_id 未找到`、无 `request_id` |
| 缺 `report_date` | 本地结构化错误 | `message=required flag --report-date is missing`、有 `hint` |
| `rows=999` | 本地结构化错误 | CLI 根据 Registry `Max=100` 拦截，不发送 API 请求 |

## 2026-06-15 Agent 日常参数验证

| 命令 | 结果 |
| --- | --- |
| `cbi config channels add smoke ***` | 成功，本机已配置 alias |
| `cbi --channel smoke +goods --modified-since <ts> --modified-until <ts> --summary` | 成功 |
| `cbi --channel smoke +orders --start <ts> --end <ts> --summary` | 成功 |
| `cbi --channel smoke +finance-daily --date <YYYYMMDD> --summary` | 成功 |
| `cbi --channel all +goods ... --summary` | 成功 |
| `cbi --channel smoke goods list --page-all --max-records 50 ...` | 成功，返回 50 行、`pages_fetched=3`、`partial=false` |

## 已内置保护

- `scripts/smoke/read_only.sh` 只调用只读接口。
- 输出默认使用 `--summary`，不输出完整业务明细。
- 凭证和 OpenChannelId 只从环境变量或本地配置读取，不进入仓库。

## 2026-06-16 Agent Output Contract v1

已在 CLI 本地单测中固定：

- Agent 推荐使用 `--machine --format json`。
- `CBI_AGENT=1` 时，错误输出也会进入机器友好 JSON。
- 成功响应在 Agent 模式下输出：
  - `ok=true`
  - `data`
  - `meta.hints`
  - `meta.alerts`
  - `meta.count`
  - `meta.rows`
  - 可选 `meta.pages_fetched/pages_failed/partial/rate_limit_wait_ms/channel/output_file`
- 失败响应在 Agent 模式下输出：
  - `ok=false`
  - `error.kind`
  - `error.subtype`
  - `error.message`
  - `error.hint`
  - `error.retryable`
  - `error.retry_after_ms`
  - `error.api_code`
  - `error.api_msg`
  - `error.request_id`
  - `meta.exit_code`
- 为兼容旧调用，`error_code/kind/message/hint/api_code/api_msg` 仍保留在顶层。

仍需人工真实验证：

- CaptainBI 业务接口是否稳定返回 `request_id` 或等价 trace 字段。
- 更多业务错误是否全部遵循 `code/msg`。
- 真实 429 是否带 `Retry-After`。
- 多 Agent 并发长任务下，token lock 与跨进程 rate-limit 是否符合预期。

## 2026-06-18 GitHub 安装与 Agent 测试准备

本轮主路径改为 GitHub tag 安装，不依赖 npm registry：

```bash
npm install -g github:kirkzwy/captainbi-cli#v0.2.3
cbi --version
cbi doctor local --machine --format json
```

安装链路补强：

- `npm/install.js` 下载 GitHub Release asset 前输出 URL。
- `curl` 增加连接超时、总超时和重试。
- 支持 `CAPTAINBI_CLI_GITHUB_TOKEN` / `GITHUB_TOKEN` 访问私有仓库或规避 GitHub 限流。
- 代理环境需要显式设置 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NODE_USE_ENV_PROXY=1`。
- 如果 `npm install github:...` 在本机环境无输出卡住，使用 `--foreground-scripts` 观察 postinstall；仍失败时改走 GitHub Release 二进制 fallback。

Agent 输出契约补充：

- `page_rows` 分页响应增加 `has_more` 和 `next_page`。
- `meta` 同步暴露 `has_more`、`next_page`、`pages_fetched`、`pages_failed`、`partial`。
- 错误 `error.subtype` 固定为稳定枚举，便于 Agent 自愈。

新增只读快捷命令：

- `+inventory`
- `+ads-campaigns`
- `+ads-campaign-report`
- `+reviews`
- `+store-transactions`

仍需在 tag `v0.2.3` 发布后验证：

- GitHub Release workflow 的 npm GitHub install smoke。
- 本机 `/tmp` 前缀安装 smoke。
- 真实 Agent 中的只读任务选择、参数补齐、错误恢复和输出保存。
