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
| 限流行为 | 已确认 | `get_site_list` 突发请求在第 82 次首次受限；CaptainBI 返回 HTTP 401、业务码 `100910` 和“请求频率过快”，而不是 HTTP 429 |

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

- `write_safe`：优先重复设置当前店铺模式做幂等验收；只有商品已有可恢复运营人员时才使用运营人员变更。
- `write_dangerous`：设置测试商品分组，回读后恢复。
- `sync_trigger`：同步允许重复同步的测试 FBA 货件。

使用 `scripts/smoke/write_guarded.sh prepare|apply|prepare-restore|restore` 分阶段执行。真实写入结果、回读和恢复在取得本地测试 fixture 与用户逐项批准前仍属于待验证项。

本次真实只读 fixture 显示店铺当前为店铺模式，55 个商品明细的运营人员均为 0；因此默认 `CAPTAINBI_WRITE_SAFE_KIND=shop-mode`，避免执行无法证明可恢复的运营人员变更。商品中存在多个非零分组，可选择不同分组做变更/回滚；货件中存在 CLOSED 状态候选。

## 2026-06-21 Codex v0.3.0 预发布回归

- 使用本地模拟 Release 资产完成 npm tarball `postinstall`、下载 URL、checksum、解压、wrapper、`cbi version 0.3.0` 和 `doctor local` 验证。
- 在隔离 HOME 中执行 `npx skills add . -y -g`，8 个 CaptainBI skills 均落入 Codex 兼容目录。
- Skills installer 同时探测 PromptScript 时报告其不支持 global install；本轮已按用户要求移除 WorkBuddy/其他 Agent 验收，该提示不影响 Codex 安装结果。

## 2026-06-21 Registry 运行时覆盖与输入路径安全

- `registry check` 已对真实官方 OpenAPI 验证为 65 个 operation，与有效 Registry 的 65 个命令一致。
- `registry update` 从项目主分支下载由官方 OpenAPI 生成的 metadata，经兼容性与风险不可降级校验后原子安装到私有配置目录；`registry reset` 可恢复内置版本。
- 损坏的托管覆盖会回退内置 Registry 并由 `doctor local` 返回 warning；显式 `CAPTAINBI_REGISTRY_FILE` 无效时直接失败，避免静默使用错误配置。
- `--params-file`、`--data-file`、`--channel-file` 仅允许 cwd 内相对普通文件，并阻止绝对路径、父目录穿越和逃逸符号链接；绝对路径内容改走 stdin。

## 2026-06-21 复合分页与范围批次

- Registry 在 `page_rows` 外增加 `rangeType`：34 个接口使用 `modified_time_window`，16 个接口使用 `report_date`，15 个接口无外层范围。
- `--page-all` 对修改时间自动拆成不重叠的 31 天窗口；`--range-start/--range-end` 支持匹配格式的日范围 `YYYYMMDD` 和月范围 `YYYYMM`。
- partial、页数限制和 `--max-records` 截断统一返回 `next_window/next_page/next_offset`；对应 `--resume-from-window/--resume-from-page/--resume-offset` 可无损续拉。
- 修正旧逻辑在页中达到 `--max-records` 后直接跳到下一页、可能漏掉当前页剩余记录的问题。

真实只读验证：40 天商品范围被拆为 2 个窗口、共 3 页、105 行，`partial=false`；两天财务日报生成 2 个 report_date 批次，`partial=false`。原始数据仅保存在本机 `/tmp`，文档不记录店铺或商品明细。

## 2026-06-21 Agent 危险写入白名单与 429 退避

- Agent 的 `write_dangerous`/`sync_trigger` 除一次性审批 hash 外，还必须通过 `config write-allowlist` 显式放行注册命令；未放行时不会消费 hash。
- dry-run 返回 `policy.allowlist_required/allowlisted/allow_command`，便于用户在批准前检查策略状态。
- 429 mock 已验证优先遵循 `Retry-After`；无 header 时使用 5/15/45 秒基础退避加正负 20% jitter，服务端 Retry-After 上限为 5 分钟。

发布前本地矩阵：`go test -race ./...` 通过，总覆盖率 53.3%（CI 门槛提升到 50%）；golangci-lint v2.12.2 为 0 issue；govulncheck v1.1.4 未发现已知漏洞；darwin/linux/windows 的 amd64/arm64 六平台构建、8 个 Skills 校验、npm tarball 和 doctor smoke 均通过。

GitHub 首次在 Go 1.24.13 上运行 govulncheck 时发现 9 个可达的 2026 年标准库漏洞，最高要求 Go 1.25.11 修复。项目最低版本因此提升到 Go 1.25，CI/Release 固定 1.25.11；未添加漏洞忽略规则。
- 使用本机真实凭证和 alias 完成 Codex 只读回归：店铺、商品、广告活动日报、FBA 货件、店铺财务月报全部 `ok=true`。
- 广告日报首次使用 GET multipart 时返回 `report date 字段是必须的`；改为 query 后成功，证明 28 个 GET 文档 body 字段必须在传输层转为 query。
- raw `--params` 的大整数已改用 `json.Decoder.UseNumber()`，避免日期被格式化为科学计数法。
- 六个平台组合（darwin/linux/windows × amd64/arm64）交叉编译通过。

## 2026-06-22 真实写入与限流契约

经用户逐项检查 dry-run 并明确批准后，使用单店铺 alias 完成代表性真实写入验证：

- `write_safe` 店铺运营模式使用当前值做幂等写入，返回业务码 `200`，回读仍为预期模式。
- `sync_trigger` 使用 CLOSED 货件触发同步，返回业务码 `200`；临时 `fba.sync-shipment` 白名单已在请求后立即移除。
- `write_dangerous` 商品分组请求被服务端拒绝，返回 `api_code=100002`、`api_msg=请输入正确的goods_id`；同步请求当时尚未发送，商品分组未发生变化，临时 `goods.set-group` 白名单已移除。
- 商品明细同时返回 `id` 与 `amazon_goods_id`。真实错误表明写入接口接受的“商品 id”不能仅凭字段名推断；下一次必须使用修正后的精确 dry-run 重新取得用户批准，旧 hash 不得复用。

真实只读压力测试结论：

- `get_site_list` 前 81 次突发请求成功，第 82 次首次进入服务端限流，随后同一测试窗口内持续返回限流错误。
- CaptainBI 实际使用 HTTP 401 + 业务码 `100910` + “请求频率过快”，而不是 HTTP 429。
- 响应没有可解析的 `Retry-After`；CLI 因此使用 5/15/45 秒基础退避加 jitter。
- 客户端已在 token 刷新前识别 `100910`，避免把限流误判为认证失败；真实回归稳定输出退出码 4、`RATE_LIMIT_EXCEEDED`、`retryable=true` 和 `api_code=100910`。
- 官方手册只说明免费套餐为 20 次/分钟，没有说明 `100910` 的恢复窗口；本次突发测试约 86 分钟后仍受限，因此 Agent 不应通过刷新 token 规避，也不应在没有新 dry-run 和用户批准时重放写入。
- 修正为 `amazon_goods_id` 的商品分组请求经用户批准并通过请求体比对后执行，但四次 HTTP 尝试均在业务写入前被 `100910` 拒绝；CLI 消费 approval hash 后返回退出码 4，临时白名单已移除，未重放请求。

后续账户已开通 250 次/分钟套餐，服务端限流解除。CLI 默认值和本机持久化配置均调整为 250；其他套餐可用 `cbi config rate-limit <n>` 持久化，或使用 `CAPTAINBI_RATE_LIMIT` / `--rate-limit` 做进程级覆盖。上面的 20 次/分钟与长时间 `100910` 结论仅代表当时免费套餐压力测试。

限流解除后的最终真实写入闭环：

- 复核发现早期本地 fixture 每行带 `export ` 前缀，临时更新脚本未命中变量行，导致三份所谓“修正预览”仍使用未文档化本地 `id`；这些请求均被服务端参数校验或限流拒绝，没有错误写入。
- fixture 真正改为 `amazon_goods_id` 后，写入前回读唯一命中商品且处于原分组；用户批准的 fresh 请求与预览 method/path/body/risk/policy 完全一致。
- 设置目标分组返回业务码 `200`，回读确认商品进入目标分组；经用户再次批准 rollback 预览后恢复原分组，回读确认原分组匹配且目标分组不再匹配。
- `goods.set-group` 临时白名单已移除，最终白名单为空。真实写入脚本变量改名为 `CAPTAINBI_WRITE_AMAZON_GOODS_ID`，避免再次混淆本地 `id`。

## 2026-06-23 v0.3.1 WorkBuddy 日常可用性修复

WorkBuddy v0.3.0 只读回归暴露的必要修复：

- `+ads-campaigns` 补齐 `--modified-since/--modified-until/--type 1|2|3`，Registry 按真实 API 契约将三个参数标记为必填。
- 不带 `--page-all` 的超 31 天修改时间请求在本地返回 `VALIDATION_BAD_PARAM`，不再依赖服务端模糊错误。
- 修复 `--max-records` 刚好落在窗口末尾时 `windows_completed` 少计数的问题，避免 Agent 重复续拉。
- 多店铺聚合统一返回店铺成功/失败数、汇总行数和 `partial`；全部店铺失败时返回 `CHANNEL_BATCH_FAILED` 和非零退出码。
- auth/config/doctor/schema JSON/rate-limit/registry 机器输出提供 `ok/data/meta`，v0.x 期间同时保留原顶层字段。
- `tools export` 和 `schema --format openai-tool` 保持裸生成物，避免破坏 Agent tool loader。

本轮按用户要求不重复 WorkBuddy P8-1~P8-6 真实写入，也不执行 429/100910 压力测试。

## 2026-06-23 v0.3.3 多格式输出与 NDJSON 流式契约

- 通用数据格式固定为 `json|ndjson|table|csv`；无效格式在执行命令和发送 HTTP 前返回 `VALIDATION_BAD_PARAM`。`openai-tool` 继续仅用于 `schema`，`tools export` 的 `openai|claude` 保持独立裸产物。
- Agent 的 JSON 输出继续使用 `ok/data/meta`。CSV/table/NDJSON 在未使用 `--output-file` 时保持 stdout 为纯数据，并在 stderr 最后一行返回单个成功 meta JSON；verbose/debug 诊断只能出现在该行之前。
- CSV/table 不再受旧的域级 `tableColumns` 限制而静默丢字段：实际响应字段全部输出，Registry 字段只控制优先顺序，嵌套值编码为紧凑 JSON。table 单元格按 Unicode 显示宽度对齐，超过 40 宽度时仅展示截断值。
- `--output-file` 通过同目录私有临时文件写入并原子替换，权限为 `0600`；stdout 状态 envelope 保留 `partial/has_more/next_*/pages_*/windows_*/rate_limit_wait_ms`，不再只返回路径和行数。
- 单店铺只读 `--format ndjson --page-all` 通过分页 page sink 逐页写出，不再等待所有数据聚合。`--jq`、`--summary`、`--limit` 和多店铺调用保留聚合路径，并用 `meta.streaming=false` 明示。
- 流式 sink 写失败会立即停止后续请求；后续 API 页失败时保留已写记录，返回 `partial=true` 和续拉游标。输出文件路径只有在请求得到可用结果并成功关闭文件后才安装最终文件。
- 本轮不增加 `pretty`，强化后的 `table` 继续承担终端展示；不从 OpenAPI 长 description 自动生成友好表头，避免破坏稳定字段名。
- 使用本机真实凭证和专用 smoke alias 完成六域只读格式验证：goods CSV、orders table、finance JSON、FBA NDJSON page-all、ads CSV、reviews table 均成功；FBA 返回 `streaming=true`，六个临时输出文件权限均为 `0600`。业务数据仅写入临时目录并在验证后删除。
