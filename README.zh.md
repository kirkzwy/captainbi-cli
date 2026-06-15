# captainbi-cli

CaptainBI OpenAPI 命令行客户端。主命令是 `cbi`，同时保留 `captainbi` 作为别名。

## 当前状态

项目目前处于 Agent-ready 早期可用阶段：

- Go + Cobra 单二进制 CLI。
- 使用 `OpenAPI -> Registry 元数据 -> 业务域命令` 的生成架构。
- 已接入 CaptainBI OpenAPI 当前 65 个接口元数据。
- 内置 token 缓存、`scope=all` token 请求、敏感信息脱敏、20 次/分钟限流和 429 退避重试。
- 已完成真实只读 smoke：认证、站点、店铺、商品、订单、财务、广告、FBA、Review。
- 已提供通用 `api`、业务域命令、快捷命令、`schema` 和 `doctor` 命令。

接口按 6 个业务域组织：

| 业务域 | 命令 | 接口数 |
| --- | --- | ---: |
| 商品信息 | `cbi goods` | 11 |
| 销售数据 | `cbi sales` | 5 |
| 财务数据 | `cbi finance` | 19 |
| FBA 数据 | `cbi fba` | 6 |
| 广告数据 | `cbi ads` | 18 |
| 监控与口碑 | `cbi monitor` | 6 |

## 快速开始

```bash
# 需要先安装 Go 1.23+
go build -o bin/cbi .

# 配置凭证。不要通过普通命令行参数传递 client_secret。
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | ./bin/cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive

# 获取并缓存 token
./bin/cbi auth token

# 查看站点和店铺
./bin/cbi +sites
./bin/cbi +shops
```

> 如果在 Codex/Agent 环境中使用，不要假设外部终端的 `export` 会进入 Agent 进程。推荐用上面的 `--client-secret-stdin` 写入本机 keychain，或用 `--client-secret-file` / `CAPTAINBI_ACCESS_TOKEN`。

## 命令层级

### 1. 快捷命令

快捷命令面向高频只读场景，适合人和 Agent 直接使用。

```bash
cbi +shops
cbi +sites
cbi +orders --channel main --start 1781424057 --end 1781510457
cbi +goods --channel main --modified-since 1781424057 --modified-until 1781510457
cbi +finance-daily --channel main --date 20260615
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

cbi ads campaign-report \
  --open-channel-id oc_xxx
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
| `--confirm` | 确认执行危险写入或同步类接口 |
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

## 安全策略

- `client_secret` 不通过普通 flag 传递，避免出现在 shell history 或进程列表中。
- token、secret、authorization、OpenChannelId 在 dry-run、错误和配置展示中会脱敏。
- 危险 POST 接口必须显式传 `--confirm`。
- `--dry-run` 永远不会发送请求。
- 真实接口契约检查必须显式执行 `cbi doctor contract`，默认测试不触发真实请求。

写入类接口风险等级：

| 风险等级 | 行为 |
| --- | --- |
| `read` | 只读接口，直接执行 |
| `write_safe` | 写入接口，交互提示，可用 `--yes` 跳过 |
| `write_dangerous` | 危险写入，必须 `--confirm` |
| `sync_trigger` | 会触发同步，必须 `--confirm` 并显示警告 |

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
- 大数据任务先用 `--summary` 判断规模，再用 `--output-file` 保存完整数据。
- 店铺级接口优先使用 `--channel <alias>`，不要在日志中输出原始 OpenChannelId。
- 失败时读取 `error_code`、`kind`、`hint`、`api_code`、`api_msg` 来决定是否重试或补参数。
- `page_rows` 分页不强依赖 CaptainBI 返回总数字段；以 `len(data) < rows` 作为主要结束条件。

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
- POST 接口第一版统一使用 `--data` JSON，还没有做字段级 flags。
- npm/goreleaser 发布链路已配置骨架，尚未正式发布 release。
