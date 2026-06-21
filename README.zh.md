# captainbi-cli

CaptainBI OpenAPI 命令行客户端。主命令是 `cbi`，同时保留 `captainbi` 作为别名。

## 当前状态

项目目前处于 Agent-ready 可测试阶段：

- Go + Cobra 单二进制 CLI。
- 使用 `OpenAPI -> Registry 元数据 -> 业务域命令` 的生成架构。
- 已接入 CaptainBI OpenAPI 当前 65 个接口元数据。
- 内置 token 缓存、`scope=all` token 请求、敏感信息脱敏、20 次/分钟限流和 429 退避重试。
- 已完成真实只读 smoke：认证、站点、店铺、商品、订单、财务、广告、FBA、Review。
- 已提供通用 `api`、业务域命令、快捷命令、`schema` 和 `doctor` 命令。
- 已具备 GitHub Release / npm wrapper / Agent Skills 骨架；当前阶段优先走 GitHub 安装，不依赖 npm registry 发布。
- Registry 已保留 65 个接口的官方响应 schema 和 36 个文档 request body；真实契约要求 28 个 GET body 字段在线路上转为 query，8 个 POST 使用 multipart。
- Agent 写入采用绑定 payload、15 分钟有效且一次性消费的 dry-run 审批 hash。

接口按 6 个业务域组织：

| 业务域 | 命令 | 接口数 |
| --- | --- | ---: |
| 商品信息 | `cbi goods` | 11 |
| 销售数据 | `cbi sales` | 5 |
| 财务数据 | `cbi finance` | 19 |
| FBA 数据 | `cbi fba` | 6 |
| 广告数据 | `cbi ads` | 18 |
| 监控与口碑 | `cbi monitor` | 6 |

## Agent 快速开始

```bash
# 当前内部/私有项目阶段优先使用 GitHub Release npm tarball 安装
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.0/captainbi-cli-0.3.0.tgz
cbi --version
cbi doctor local --machine --format json

# 如果环境支持 skills installer，加载 CaptainBI skills
npx skills add kirkzwy/captainbi-cli -y -g

# 配置凭证。不要通过普通命令行参数传递 client_secret。
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive

# 获取并缓存 token
cbi auth token --machine --format json

# 查看站点和店铺
cbi +sites --machine --format json
cbi +shops --machine --format json

# 保存店铺别名后跑第一个只读任务
cbi config channels add main '<open_channel_id>'
cbi --channel main +goods --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
```

私有仓库或 GitHub 限流场景，安装前先配置访问 token：

```bash
export GITHUB_TOKEN='<github_token>'
export CAPTAINBI_CLI_GITHUB_TOKEN='<github_token>'
```

需要代理的网络环境，安装前显式配置：

```bash
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
export ALL_PROXY=http://127.0.0.1:7890
export NODE_USE_ENV_PROXY=1
```

如果 `npm install github:...` 在特定环境仍卡住，可直接下载 GitHub Release 二进制：

```bash
curl -L -o cbi.tar.gz https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.0/captainbi-cli_0.3.0_darwin_arm64.tar.gz
tar -xzf cbi.tar.gz
./cbi --version
```

> 如果在 Codex/Agent 环境中使用，不要假设外部终端的 `export` 会进入 Agent 进程。推荐用上面的 `--client-secret-stdin` 写入本机 keychain，或用 `--client-secret-file` / `CAPTAINBI_ACCESS_TOKEN`。

源码构建仅作为开发路径：

```bash
go build -buildvcs=false -o bin/cbi .
```

## 命令层级

### 1. 快捷命令

快捷命令面向高频只读场景，适合人和 Agent 直接使用。

```bash
cbi +shops
cbi +sites
cbi +orders --channel main --start 1781424057 --end 1781510457
cbi +goods --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +finance-daily --channel main --date 20260615
cbi +inventory --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +ads-campaign-report --channel main --date 20260615 --summary
cbi +reviews --channel main --summary
cbi +store-transactions --channel main --start 20260601 --end 20260615
```

从 `+shops` 获取 `open_channel_id` 后，建议保存为别名：

```bash
cbi config channels add main '<open_channel_id>'
cbi config channels list
```

### 2. 业务域命令

业务域命令由 Registry 元数据自动注册，按 CaptainBI 业务域分组。

```bash
cbi goods list \
  --open-channel-id oc_xxx \
  --start-modified-time 1700000000 \
  --end-modified-time 1700100000

cbi finance store-daily \
  --open-channel-id oc_xxx \
  --report-date 20240101

cbi ads advertise-campaign-report \
  --open-channel-id oc_xxx \
  --report-date 20240101
```

### 3. 通用 API

如果业务域命令暂时不能覆盖某个特殊调用，可以使用通用 API 入口。

```bash
cbi api GET /v1/open_user/get_site_list

cbi api GET /v1/open_goods/get_goods_list \
  --open-channel-id oc_xxx \
  --params '{"page":1,"rows":100,"start_modified_time":1700000000,"end_modified_time":1700100000}'
```

### 4. Schema 查看

调用前可以查看接口参数、分页规则和风险等级。

```bash
cbi schema finance.store-daily
cbi schema goods.list --jq '.params'
```

## 常用参数

| 参数 | 说明 |
| --- | --- |
| `--open-channel-id` | 店铺密钥，也可使用 `CAPTAINBI_OPEN_CHANNEL_ID` |
| `--format` | 输出格式：`json`、`ndjson`、`table`、`csv` |
| `--jq` | 使用 gojq 表达式过滤 JSON 输出 |
| `--machine` | 机器模式：纯结构化输出，适合 Agent/脚本解析 |
| `--dry-run` | 只展示请求结构，不发送请求 |
| `--page-all` | 自动分页，当前完整支持 `page_rows` 类型 |
| `--page-limit` | 自动分页的最大页数，默认 10 |
| `--page-delay` | 自动分页的页间延迟，默认 3000ms |
| `--max-records` | 自动分页最多返回多少条 |
| `--resume-from-page` | 从指定页继续自动分页 |
| `--summary` | 只输出行数和字段列表，适合 Agent 探测数据规模 |
| `--output-file` | 将完整结果写入文件，stdout 只返回文件路径和行数 |
| `--channel` | 使用 `config channels` 中的店铺别名，也可用 `all` |
| `--confirm` | 仅保留给交互式 TTY 的兼容确认；Agent 不使用 |
| `--confirm-request` | 使用当前 dry-run 生成的精确请求 hash 批准 Agent 写入 |
| `--rate-limit` | 覆盖默认限流，单位为请求数/分钟 |

## 环境变量

| 变量 | 说明 |
| --- | --- |
| `CAPTAINBI_CLIENT_ID` | CaptainBI APPID / client_id |
| `CAPTAINBI_CLIENT_SECRET` | CaptainBI client_secret，适合 CI 或一次性运行 |
| `CAPTAINBI_BASE_URL` | API 域名，默认 `https://openapi.captainbi.com` |
| `CAPTAINBI_OPEN_CHANNEL_ID` | 默认店铺密钥 |
| `CAPTAINBI_RATE_LIMIT` | 请求限流，默认 20 次/分钟 |
| `CAPTAINBI_ACCESS_TOKEN` | 直接注入已有 access token，跳过 token 获取 |
| `CAPTAINBI_CONFIG_DIR` | 配置、token、锁和写入预览使用的私有可写目录 |
| `CAPTAINBI_REGISTRY_FILE` | 显式指定兼容 Registry metadata；日常优先使用 `cbi registry update` |

## 安全策略

- `client_secret` 不通过普通 flag 传递，避免出现在 shell history 或进程列表中。
- token、secret、authorization、OpenChannelId 在 dry-run、错误和配置展示中会脱敏。
- Agent 写入必须先 `--dry-run`，由用户确认完全相同的预览后，再传 `--confirm-request <request_hash>`。
- hash 15 分钟过期，并在发送前消费，不能重放。
- 写入不支持 `--channel all`，必须逐店铺、逐 payload 审批。
- 未知 raw 非 GET 调用还必须显式传 `--unsafe-raw-write`。
- `--dry-run` 永远不会发送请求。
- 真实接口契约检查必须显式执行 `cbi doctor contract`，默认测试不触发真实请求。
- `--params-file`、`--data-file`、`--channel-file` 只读取当前工作目录内的相对普通文件；绝对路径内容应通过 stdin 传入。

写入类接口风险等级：

| 风险等级 | 行为 |
| --- | --- |
| `read` | 只读接口，直接执行 |
| `write_safe` | TTY 可交互确认；Agent 必须使用审批 hash |
| `write_dangerous` | Agent 必须使用审批 hash |
| `sync_trigger` | Agent 必须使用审批 hash，并显示外部同步警告 |

## Doctor

本地检查不需要真实凭证：

```bash
cbi doctor local --machine
```

真实契约检查需要凭证，并会请求 CaptainBI API：

```bash
cbi doctor contract --sample 5
```

## 真实 Smoke

只读 smoke 不会写入 CaptainBI 数据：

```bash
go build -buildvcs=false -o bin/cbi .
CAPTAINBI_SMOKE_OPEN_CHANNEL_ID='<open_channel_id>' scripts/smoke/read_only.sh
```

错误契约和真实行为记录见 `docs/contract-notes.md`。

## Agent 使用建议

- 默认使用 `--machine --format json`。
- 也可以设置 `CBI_AGENT=1`，让错误输出默认进入机器友好 JSON。
- 大数据任务先用 `--summary` 判断规模，再用 `--output-file` 保存完整数据。
- 店铺级接口优先使用 `--channel <alias>`，不要在日志中输出原始 OpenChannelId。
- 成功输出读取 `ok/data/meta`；失败输出读取 `ok/error/meta`。
- 失败时优先读取 `error.kind`、`error.subtype`、`error.hint`、`error.api_code`、`error.api_msg` 来决定是否重试或补参数。
- 即使 HTTP 为 200，只要 CaptainBI `code != 200`，CLI 也会返回失败，不能把空 data 当作成功。
- 翻页时优先读取 `meta.has_more` 和 `meta.next_page`，不要自行推断是否还有下一页。
- `page_rows` 分页不强依赖 CaptainBI 返回总数字段；以 `len(data) < rows` 作为主要结束条件。

## Agent 写入流程

```bash
# 1. 仅生成预览
cbi --channel main goods set-operate-user \
  --goods-id <goods_id> --operation-user-admin-id <operator_id> \
  --dry-run --machine --format json

# 2. 停下并请用户确认 method、path、channel 和 body

# 3. 请求不变时，使用 data.approval.request_hash 执行
cbi --channel main goods set-operate-user \
  --goods-id <goods_id> --operation-user-admin-id <operator_id> \
  --confirm-request <request_hash> --machine --format json
```

写入后必须查询受影响资源做回读验证。payload 改变、hash 过期或结果不确定时，重新生成预览，不得重放。

维护者可使用 `scripts/smoke/write_guarded.sh prepare|apply|prepare-restore|restore` 分阶段执行真实写入验收。该脚本需要专用测试对象，且不会自动跨过任何审批节点。

## Registry 更新

```bash
cbi registry check --machine --format json
cbi registry update --machine --format json
# 恢复二进制内置 Registry
cbi registry reset --machine --format json
```

运行时更新只接受保持旧命令兼容、不会降低写入风险等级、不会移除 OpenChannelId 要求的 metadata。`doctor local` 会显示有效版本、内置版本、覆盖文件路径和回退告警。

## 开发

```bash
# 运行测试
go test ./...

# 从 CaptainBI OpenAPI 重新生成 Registry 和端点文档
go run ./tools/gen-registry

# 构建本地二进制
go build -o bin/cbi .
```

生成产物：

- `internal/registry/captainbi_meta.json`：65 个接口的 Registry 元数据。
- `docs/endpoints.md`：接口、命令、分页和风险等级速查表。

## 当前限制

- `--page-all` 当前完整支持 `page_rows` 分页，并支持 `--max-records` 和 `--resume-from-page`。
- `modified_time_window` 和 `report_date` 已进入 Registry，但目前仍按单次请求执行。
- 简单 form-data body 字段已生成独立 flags；数组/对象 body 使用 `--data`、stdin 或文件输入。
- npm registry 暂不作为当前主路径；Agent 测试阶段使用 GitHub Release npm tarball：`npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.0/captainbi-cli-0.3.0.tgz`。
