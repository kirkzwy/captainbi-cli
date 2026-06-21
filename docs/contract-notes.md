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

本轮主路径改为 GitHub Release npm tarball 安装，不依赖 npm registry：

```bash
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.2.4/captainbi-cli-0.2.4.tgz
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

v0.2.3 记录：

- GitHub Release assets 已生成。
- 本地 npm tarball smoke 已通过。
- `npm install -g github:kirkzwy/captainbi-cli#v0.2.3` 在 GitHub Actions 中失败，npm 在 Git clone lifecycle 临时目录执行 `node install.js` 时找不到入口文件。
- 因此 v0.2.4 改为 GitHub Release npm tarball URL 作为主安装路径，仍不走 npm registry。

v0.2.4 验证结果：

- GitHub Release workflow 通过，包含 npm tarball 上传、本地 tarball smoke、GitHub Release npm tarball 安装 smoke。
- Release assets 包含 6 个平台二进制包、`checksums.txt` 和 `captainbi-cli-0.2.4.tgz`。
- 本机 `/tmp` 前缀安装不带代理时会长时间无输出，判断卡在 npm/GitHub 网络阶段。
- 本机显式设置 `HTTP_PROXY`、`HTTPS_PROXY`、`ALL_PROXY`、`NODE_USE_ENV_PROXY=1` 后安装通过，并验证 `cbi --version` 与 `cbi doctor local --machine --format json`。
- 真实 Agent 中的只读任务选择、参数补齐、错误恢复和输出保存。

## 2026-06-21 v0.3.0 请求体与写入契约修正

对官方 OpenAPI 重新解析后确认：

- 65 个接口中有 36 个定义了 `requestBody`。
- 其中 28 个是 GET form-body，8 个是 POST form-body。
- 官方 media type 写成 `application/form-data`。真实 API 验证表明：GET 不读取 multipart body，字段必须转为 query；POST 规范化为带 boundary 的 `multipart/form-data`。
- 旧生成器完全忽略 `requestBody`，旧客户端固定发送 JSON，因此“65 个命令已注册”不等于“65 个接口均按官方契约可用”。
- 官方成功响应统一以业务字段 `code=200` 表示；HTTP 200 且 `code != 200` 必须作为 `API_BUSINESS_ERROR`，不能按空数据成功处理。

v0.3.0 自动验证已经覆盖：

- 65 个 response schema 读取官方 OpenAPI，而不是按业务域推测字段。
- 36 个 request body、28 个 GET body-to-query、8 个 POST multipart 数量断言。
- GET body-derived query 的 `page/rows` 自动分页和续页。
- 8 个 POST 均完成 `dry-run -> request_hash -> 本地 mock multipart 写入`。
- `cbi api POST` 不再能绕过风险等级；未知 raw write 需要额外 `--unsafe-raw-write`。
- Agent 模式中的所有写入均需要 `--confirm-request`，并禁止 `--channel all`。
- request hash 绑定 method、path、query、body、content type、channel、risk 和 Registry 版本；15 分钟过期且发送前一次性消费。
- 无效 alias、无效 OpenChannelId、坏时间戳、HTTP 200 业务错误分别进入稳定 subtype。

真实写入验收采用三类代表接口，不自动触碰 FBM 发货、成本或运营费用：

- `write_safe`：设置测试商品运营人员，回读后恢复。
- `write_dangerous`：设置测试商品分组，回读后恢复。
- `sync_trigger`：同步允许重复同步的测试 FBA 货件。

使用 `scripts/smoke/write_guarded.sh prepare|apply|prepare-restore|restore` 分阶段执行。真实写入结果、回读和恢复在取得本地测试 fixture 与用户逐项批准前仍属于待验证项。

## 2026-06-21 Codex v0.3.0 预发布回归

- 使用本地模拟 Release 资产完成 npm tarball `postinstall`、下载 URL、checksum、解压、wrapper、`cbi version 0.3.0` 和 `doctor local` 验证。
- 在隔离 HOME 中执行 `npx skills add . -y -g`，8 个 CaptainBI skills 均落入 Codex 兼容目录。
- Skills installer 同时探测 PromptScript 时报告其不支持 global install；本轮已按用户要求移除 WorkBuddy/其他 Agent 验收，该提示不影响 Codex 安装结果。
- 使用本机真实凭证和 alias 完成 Codex 只读回归：店铺、商品、广告活动日报、FBA 货件、店铺财务月报全部 `ok=true`。
- 广告日报首次使用 GET multipart 时返回 `report date 字段是必须的`；改为 query 后成功，证明 28 个 GET 文档 body 字段必须在传输层转为 query。
- raw `--params` 的大整数已改用 `json.Decoder.UseNumber()`，避免日期被格式化为科学计数法。
- 六个平台组合（darwin/linux/windows × amd64/arm64）交叉编译通过。
