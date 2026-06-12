# CaptainBI CLI 迭代开发计划（优化版）

## Summary

基于当前空目录从零开发 CaptainBI CLI，采用 **Go + Cobra 单二进制**，主二进制名 `cbi`（同时保留 `captainbi` 别名），仓库名 `captainbi-cli`。参考 `larksuite/cli` 的主架构（三层命令体系、Registry 元数据驱动、命令自动注册）与 `mowenxd/cli` 的轻量 npm 分发方式及 Agent 友好设计。目标是接入 CaptainBI OpenAPI 当前 **65 个接口**：商品 11、销售 5、财务 19、FBA 6、广告 18、其他 6。

参考依据：[CaptainBI OpenAPI](https://doc.captainbi.com/openapi.json)、[CaptainBI 调用说明](https://www.captainbi.com/amz_faq_list-217.html)、[larksuite/cli](https://github.com/larksuite/cli)、[mowenxd/cli](https://github.com/mowenxd/cli)。

**关键约束**：CaptainBI 免费版 API 限制 **20 次/分钟**，所有设计决策必须围绕此硬约束展开。

---

## 可参考范围

- 参考 `larksuite/cli`：

  - Go/Cobra 命令树、`config/auth/api/schema/completion/doctor` 分层。

  - **三层命令体验**：快捷命令（`+verb`）、端点命令（从 spec 自动生成）、通用 `api` 调用。

  - **Registry 中间元数据格式**：不直接消费 OpenAPI spec，而是先转换为 `meta_data_default.json`（service → resource → method 树形结构），再由 `cmd/service/service.go` 遍历注册命令。这是架构核心。

  - JSON/table/csv 输出、`--jq`、`--dry-run`、分页聚合、测试注入思路。

  - `schema` 命令：调用前查看参数结构，对 Agent 尤其重要。

  - npm 包只作为安装 Go 二进制的薄壳。

- 参考 `mowenxd/cli`：

  - 更小规模的 npm wrapper、postinstall 下载二进制、skills 文档组织。

  - README 的“人类使用 + Agent 使用”说明方式。

  - `--machine`** 输出模式**：去掉人类友好装饰，只输出纯净结构化数据，供 Agent 可靠解析。

  - **Skills 中的“决策树”描述**：不只列命令，还提供“什么场景用什么命令”的决策路径。

- 不直接照搬：

  - 飞书 OAuth、scope、身份切换、插件系统、事件订阅、复杂多应用模型。

  - 与 CaptainBI 无关的业务命令和 SDK 依赖。

  - 大段源码复制；如少量复用 MIT 片段，保留 LICENSE 与 derived 注释。

---

## 架构设计

### Registry 中间元数据格式

不直接从 `openapi.json` 生成命令代码，而是先转换为项目自有的 Registry 元数据格式。这层中间表示解耦了上游 spec 变动，并可补充 OpenAPI 中缺失的语义信息。

```json
{
  "services": [
    {
      "name": "goods",
      "displayName": "商品信息",
      "domain": "goods",
      "resources": {
        "goods_list": {
          "methods": {
            "get_goods_list": {
              "httpMethod": "GET",
              "fullPath": "/v1/open_goods/get_goods_list",
              "description": "获取商品基础数据",
              "parameters": {
                "open_channel_id": { "location": "header", "required": true },
                "page": { "location": "query", "type": "integer", "default": 1 },
                "rows": { "location": "query", "type": "integer", "default": 100, "max": 100 },
                "start_modified_time": { "location": "query", "type": "timestamp" },
                "end_modified_time": { "location": "query", "type": "timestamp" }
              },
              "pagination": { "type": "page_rows", "maxRows": 100, "totalField": "max_result" },
              "riskLevel": "read",
              "requiresOpenChannelId": true
            }
          }
        }
      }
    }
  ]
}
```

元数据中的关键扩展字段：

| 字段 | 用途 |
| --- | --- |
| `domain` | 业务域归属，决定一级命令分组 |
| `pagination.type` | 分页协议：`page_rows` / `date_range` / `none` |
| `riskLevel` | 风险等级：`read` / `write_safe` / `write_dangerous` / `sync_trigger` |
| `requiresOpenChannelId` | 是否必须带店铺密钥 |

### 命令树按业务域组织

不按路径前缀扁平展开，而是按 6 个业务域分组为一级子命令：

| 业务域 | 命令组 | 覆盖路径前缀 | 接口数 |
| --- | --- | --- | --- |
| 商品信息 | `cbi goods` | `open_user`, `open_goods`, `open_goods_relevant` | 11 |
| 销售数据 | `cbi sales` | `open_order` | 5 |
| 财务数据 | `cbi finance` | `open_channel_finance`, `open_finance`, `open_goods_finance` | 19 |
| FBA 数据 | `cbi fba` | `open_fba`, 部分 `open_finance` | 6 |
| 广告数据 | `cbi ads` | `open_cpc` | 18 |
| 监控与口碑 | `cbi monitor` | `open_goods`（review/feedback/跟卖） | 6 |

最终命令形态示例：

```bash
# 业务域命令（自动生成，人类友好）
cbi goods list                    # 商品基础数据
cbi goods items                   # 商品扩展数据
cbi finance store-daily           # 店铺日报（下单维度）
cbi ads campaign-report           # 广告活动报告
cbi fba inventory                 # FBA库存

# 快捷命令（手写，封装高频操作）
cbi +shops                        # 获取店铺列表
cbi +orders --start ... --end ... # 订单分页聚合
cbi +finance-daily --scope store  # 财务日报

# 通用 API（逃生舱，任意端点）
cbi api GET /v1/open_channel_finance/get_analysis_by_order --params '{...}'

# Schema 查看（Agent 友好）
cbi schema finance.store-daily    # 查看参数结构
```

### 三层命令体系对照

| 层级 | 命名规则 | 用途 | 示例 |
| --- | --- | --- | --- |
| 快捷命令 | `+verb` 前缀 | 人和 AI 都友好，带智能默认值和限速 | `cbi +shops` |
| 业务域命令 | `domain resource method` | 从 Registry 自动生成，1:1 映射 API | `cbi finance store-daily` |
| Raw API | `api --method --url` | 完全自由调用任意端点 | `cbi api GET /v1/...` |

---

## Key Changes

- 项目骨架：

  - 初始化 Go module `captainbi-cli`，主二进制名 `cbi`（别名 `captainbi`），Go 版本 `>=1.23`。

  - 建立核心包：配置、认证、HTTP client（内置令牌桶限流器）、Registry 元数据、输出格式、命令生成。

  - 建立 npm 包外壳：`package.json`、`scripts/install.js`、`scripts/run.js`，后续发布跨平台二进制。

  - 项目目录结构：

    ```plaintext
    captainbi-cli/
    ├── main.go
    ├── go.mod
    ├── cmd/
    │   ├── root.go                    # 根命令，注册所有子命令
    │   ├── auth/                      # 认证命令
    │   ├── config/                    # 配置管理
    │   ├── schema/                    # 查看 API 参数结构
    │   ├── api/                       # 通用 API 调用
    │   ├── doctor/                    # 健康检查 / 契约测试
    │   └── service/                   # 从 Registry 自动注册的业务域命令
    ├── internal/
    │   ├── registry/
    │   │   ├── loader.go              # 加载 Registry 元数据
    │   │   ├── captainbi_meta.json    # 嵌入的 API 元数据（65 接口）
    │   │   └── schema.go             # 元数据 schema 定义
    │   ├── auth/                      # OAuth2 client_credentials 认证
    │   ├── client/
    │   │   ├── client.go             # HTTP 客户端
    │   │   ├── ratelimit.go          # 令牌桶限流器（20 req/min）
    │   │   └── retry.go             # 429/401 退避重试
    │   ├── core/                      # 配置结构体、常量
    │   ├── output/                    # JSON/Table/CSV/NDJSON 格式化
    │   ├── paginate/                  # 分页聚合（page_rows / date_range）
    │   ├── validate/                  # 参数校验
    │   └── keychain/                  # 凭证安全存储
    ├── tools/
    │   └── gen-registry/             # OpenAPI → Registry 转换脚本
    ├── skills/                        # AI Agent 技能描述
    │   ├── captainbi-shared/
    │   ├── captainbi-goods/
    │   ├── captainbi-finance/
    │   ├── captainbi-ads/
    │   ├── captainbi-sales/
    │   ├── captainbi-fba/
    │   └── captainbi-monitor/
    ├── docs/
    │   └── endpoints.md              # 65 接口速查表
    ├── npm/                           # npm 分发外壳
    └── .goreleaser.yml
    ```

- 认证与配置：

  - `cbi config init`：录入 `client_id`，通过 stdin 或交互录入 `client_secret`，不允许命令行明文 secret。

  - `cbi auth token/status/logout`：调用 `POST https://openapi.captainbi.com/oauth2/token` 获取 token，缓存 token,secret/token 优先存 OS keychain.

  - **Token 静默刷新**：HTTP Client 拦截 401 响应时，自动用缓存的 client_id/secret 重新获取 token，重试原请求（仅一次）。

  - 支持环境变量覆盖：`CAPTAINBI_CLIENT_ID`、`CAPTAINBI_CLIENT_SECRET`、`CAPTAINBI_BASE_URL`、`CAPTAINBI_OPEN_CHANNEL_ID`、`CAPTAINBI_RATE_LIMIT`。

- Rate Limit 策略（**核心基础设施，I0 即内置**）：

  - HTTP Client 层内置令牌桶限流器（`golang.org/x/time/rate`），默认 20 req/min，可通过 `--rate-limit` 或 `CAPTAINBI_RATE_LIMIT` 覆盖。

  - `--page-all` 默认启用 `--page-delay 3s`（每页间隔 3 秒），确保安全边界。

  - 收到 429 响应时自动指数退避重试（最多 3 次，间隔 5s/15s/45s）。

  - 分页进度输出：`[3/10] Fetching page 3... (rate: 18/20 remaining)`。

- API 命令设计：

  - 通用调用：`cbi api <METHOD> <PATH> --params '{}' --data '{}' --open-channel-id xxx --dry-run`。

  - 全量端点命令按业务域生成，例如 `cbi goods list`、`cbi finance store-daily`、`cbi ads campaign-report`。

  - Registry 中 query/header 参数自动生成 flags;`authorization` 自动注入；`OpenChannelId` 从 flag/env/config 获取。

  - 分页行为由 Registry 元数据中的 `pagination.type` 决定：

    - `page_rows`：循环翻页，`--page-all --page-limit --page-delay`

    - `date_range`：循环日期区间（财务月报类）

    - `none`：无分页

  - POST 写入类第一版统一支持 `--data` JSON，后续再补细粒度 flags。

  - 风险等级由 Registry 元数据中的 `riskLevel` 决定：

    - `read`：直接执行

    - `write_safe`：提示但不强制确认

    - `write_dangerous`：强制 `--confirm` 或 `--dry-run`

    - `sync_trigger`：强制确认 + 警告外部副作用

- Schema 命令：

  - `cbi schema <domain>.<method>`：查看接口的完整参数结构、分页规则、风险等级。

  - 输出格式：

    ```plaintext
    GET /v1/open_channel_finance/get_analysis_by_order
    Domain: finance | Risk: read | Pagination: page_rows (max 100)
    Headers: authorization (auto), OpenChannelId (required)
    Query:
      report_date  string  required  格式 YYYYMMDD
      page         int     optional  默认 1
      rows         int     optional  默认 100，最大 100
    ```

- 快捷命令：

  - `cbi +shops`：获取店铺并展示 `title/open_channel_id/site_id/status`。

  - `cbi +sites`：获取站点。

  - `cbi +orders --open-channel-id ... --start ... --end ...`：封装订单分页。

  - `cbi +goods --open-channel-id ... --start ... --end ...`：封装商品基础数据。

  - `cbi +finance-daily --scope store|asin --dimension order|finance --date YYYYMMDD`：封装财务日报。

  - 快捷命令只封装高频读接口，不封装写接口；内置限速逻辑。

- 输出与安全：

  - 默认 JSON；支持 `--format json|ndjson|table|csv`、`--jq`、`--output`。

  - `--machine`** 模式**（或检测 `CI=true` / `NO_COLOR=1`）：

    - 纯 JSON 输出，无 ANSI 颜色码

    - 错误输出到 stderr，数据输出到 stdout

    - 不显示进度条和交互提示

    - exit code 语义：0=成功， 1=业务错误， 2=认证错误， 3=网络错误， 4=Rate Limit

  - 错误统一输出结构：`code/message/hint/request_id`，不打印 token、client_secret、完整 authorization.

  - `--dry-run` 只展示 method/path/query/body/header 名称，敏感 header 脱敏。

- 文档与 Agent 友好：

  - README 包含安装、初始化、认证、常用命令、全量接口策略、Rate Limit 说明。

  - Skills 按 6 个业务域拆分，每个 SKILL.md 包含：概念说明、决策树、快捷命令表、API Resources 列表、权限要求。

  - `skills/captainbi-shared/SKILL.md` 包含认证配置、Rate Limit 警告、通用规则。

  - 生成 `docs/endpoints.md`，列出 65 个接口、命令名、必填参数、分页规则、风险等级。

---

## Iterations

### I0 脚手架 + 认证闭环（合并原 I0/I1）

- 建 Go/Cobra 根命令、版本、help、completion、基础测试。

- 完成 config/auth/client/token 缓存/token 静默刷新。

- HTTP Client 内置令牌桶限流器（20 req/min）和 429 退避重试。

- 建 npm wrapper（先支持本地构建，不急于发布）。

- 用 mock server 覆盖 token 成功、失败、过期、429 限流、敏感信息脱敏。

- **Done 条件**：

  1. `cbi auth token` 用真实 client_id/secret 获取 token 并缓存到 `~/.config/captainbi/`

  2. `cbi api GET /v1/open_user/get_site_list` 返回 JSON 且包含至少 1 个站点

  3. `cbi auth status` 显示 token 有效期剩余时间

  4. 连续快速发 25 次请求，客户端自动限速，不触发服务端 429

### I1 Registry 元数据格式 + 转换脚本

- 定义 Registry JSON Schema（含 domain、pagination、riskLevel、requiresOpenChannelId）。

- 编写 `tools/gen-registry`：读取 `openapi.json`，输出 `captainbi_meta.json`。

- 人工补充 OpenAPI 中缺失的语义信息（分页类型、风险等级、业务域归属）。

- **Done 条件**：

  1. 65 个接口全部转换为 Registry JSON，通过 JSON Schema 校验

  2. 每个接口的 `pagination.type`、`riskLevel`、`domain` 字段均已标注

  3. 转换脚本可重复运行，输出幂等

### I2 命令自动注册 + 通用 API + Schema

- 实现 `cmd/service/service.go`：遍历 Registry 元数据，按 domain 分组动态注册 Cobra 命令。

- 实现 `cbi api`：支持 GET/POST、query/body、OpenChannelId、dry-run、输出格式。

- 实现 `cbi schema <domain>.<method>`：从 Registry 元数据输出参数结构。

- **Done 条件**：

  1. 65 个端点命令全部可 `--help`，显示正确的参数列表和描述

  2. `cbi api GET /v1/open_user/get_site_list --dry-run` 正确输出请求结构

  3. `cbi schema goods.list` 输出完整参数表

### I3 Rate Limit + 分页 + 输出格式

- 实现 `--page-all`：根据 `pagination.type` 执行不同分页策略。

- `page_rows` 类型：循环翻页，检查 `max_result`，自动限速。

- `date_range` 类型：循环日期区间（财务月报）。

- 实现 table/csv/ndjson 输出；财务大字段默认 JSON，table 只显示核心列。

- 分页进度输出和 `--page-limit` 安全阀。

- **Done 条件**：

  1. `cbi goods list --page-all` 在 20 req/min 限制下正确聚合多页（mock 5 页）

  2. 中途遇到 429 后自动退避并恢复继续

  3. `--format table` 输出对齐的表格，`--format csv` 输出有效 CSV

  4. `--page-limit 3` 在第 3 页后停止

### I4 快捷命令 + Doctor

- 增加 `+shops/+sites/+orders/+goods/+finance-daily` 快捷命令。

- 快捷命令只封装高频读接口，内置限速逻辑，不封装写接口。

- 实现 `cbi doctor --check-schema`：对每个接口发最小请求，验证响应结构与 Registry 一致。

- **Done 条件**：

  1. `cbi +shops` 返回店铺列表，含 `title/open_channel_id/site_id/status`

  2. `cbi +finance-daily --scope store --date 20240101` 返回财务日报数据

  3. `cbi doctor --check-schema` 对至少 5 个接口完成契约验证

### I5 写入类保护

- 接入 POST 写入接口，行为由 Registry 中 `riskLevel` 决定。

- `write_dangerous`：无 `--confirm` 时拒绝执行，输出预览和警告。

- `sync_trigger`：强制确认 + 警告“此操作将触发外部同步”。

- 覆盖：设置成本、运营费用、商品分组、FBM 发货上传、同步货件。

- **Done 条件**：

  1. `cbi finance set-cost --data '{...}'` 无 `--confirm` 时拒绝执行并输出预览

  2. `cbi fba sync-shipment --confirm` 执行前显示警告信息

  3. `cbi goods set-group --dry-run` 只输出请求结构不发送

### I6 Skills 文档 + Agent 友好

- 编写 6 个业务域的 SKILL.md + `captainbi-shared/SKILL.md`。

- 每个 SKILL.md 包含：YAML frontmatter、Core Concepts、决策树、Shortcuts 表、API Resources 列表。

- `captainbi-shared` 包含认证流程、Rate Limit 警告、通用 flags、`--machine` 模式说明。

- 实现 `--machine` 输出模式和语义化 exit code。

- 生成 `docs/endpoints.md`。

- **Done 条件**：

  1. 7 个 SKILL.md 文件完整，格式通过 frontmatter 校验

  2. Agent 按 SKILL.md 中的决策树可正确选择命令（人工验证 3 个场景）

  3. `cbi +shops --machine` 输出纯 JSON 到 stdout,exit code 为 0

### I7 发布与质量

- goreleaser 配置：linux/darwin/windows × amd64/arm64。

- npm postinstall 下载对应平台二进制、checksum 校验。

- GitHub Actions:lint、test、build、release.

- README、CHANGELOG、LICENSE 补齐。

- **Done 条件**：

  1. `goreleaser release --snapshot` 产出 6 个平台二进制

  2. `npm install -g captainbi-cli` 后 `cbi --version` 正确输出

  3. CI 全绿，覆盖率 > 70%

---

## Skills 文档结构

```plaintext
skills/
├── captainbi-shared/
│   └── SKILL.md              # 认证、配置、Rate Limit、通用规则
├── captainbi-goods/
│   ├── SKILL.md              # 商品域：概念、决策树、命令列表
│   └── references/
│       ├── shops.md          # +shops 详细用法
│       └── goods.md          # +goods 详细用法
├── captainbi-finance/
│   ├── SKILL.md              # 财务域（最复杂，19 接口）
│   └── references/
│       ├── store-daily.md
│       ├── asin-monthly.md
│       └── transactions.md
├── captainbi-ads/
│   ├── SKILL.md              # 广告域（18 接口）
│   └── references/
│       └── campaign-report.md
├── captainbi-sales/
│   ├── SKILL.md              # 销售域
│   └── references/
│       └── orders.md
├── captainbi-fba/
│   ├── SKILL.md              # FBA域
│   └── references/
│       └── inventory.md
└── captainbi-monitor/
    ├── SKILL.md              # 监控域（Review/Feedback/跟卖）
    └── references/
        └── reviews.md
```

### Shared SKILL.md 关键内容

```yaml
---
name: captainbi-shared
version: 1.0.0
description: "CaptainBI CLI 共享基础：认证、配置、通用规则。所有业务域 Skill 使用前必须先读取本文件。"
metadata:
  requires:
    bins: ["cbi"]
  cliHelp: "cbi --help"
---
```

```markdown
## ⚠️ Rate Limit

CaptainBI API 限制 20 次/分钟（免费版）。规划操作时：
- 避免在一个任务中连续调用超过 15 个接口
- 使用 `--page-all` 时 CLI 会自动限速（每页间隔 3s），单次分页操作可能耗时较长
- 优先使用快捷命令（已内置限速逻辑），避免手动循环调用
- 如需大量数据，优先用 `--format ndjson --output file.ndjson` 落盘，避免重复请求

## 认证流程

1. `cbi config init` → 录入 client_id 和 client_secret
2. `cbi auth token` → 获取 access_token（自动缓存）
3. 后续命令自动注入 authorization header
4. 店铺级接口需要 `--open-channel-id`，可通过 `cbi +shops` 获取

## 通用 Flags

| Flag | 说明 |
|------|------|
| `--open-channel-id` | 店铺密钥（必填于店铺级接口） |
| `--format` | 输出格式：json/ndjson/table/csv |
| `--jq` | JQ 表达式过滤输出 |
| `--output` | 输出到文件 |
| `--dry-run` | 只展示请求结构，不发送 |
| `--page-all` | 自动分页聚合 |
| `--page-limit` | 最大分页数（安全阀） |
| `--machine` | Agent 模式：纯 JSON、无装饰、语义 exit code |
| `--confirm` | 确认写入操作 |
| `--rate-limit` | 覆盖默认限速（次/分钟） |
```

### 业务域 SKILL.md 决策树示例（Finance）

```markdown
## 决策路径

- 想看店铺整体利润（按天）？→ `cbi finance store-daily --dimension order --date YYYYMMDD`
- 想看店铺整体利润（按月）？→ `cbi finance store-monthly --dimension order --date YYYYMM`
- 想看某个 ASIN 的利润？→ `cbi finance asin-daily --date YYYYMMDD`
- 想看交易流水明细？→ `cbi finance transactions`
- 想看 VAT 报告？→ `cbi finance vat`
- 想看运营费用详情？→ `cbi finance operating-expenses`
- 想看回款记录？→ `cbi finance payment-record`
- 想看店铺绩效？→ `cbi finance performance`
- 想看 FBA 索赔？→ `cbi finance claims`
- 想设置产品成本？→ `cbi finance set-cost --confirm`（⚠️ write_dangerous）
- 想设置运营费用？→ `cbi finance set-rule --confirm`（⚠️ write_dangerous）
- 下单维度 vs 财务维度？→ `--dimension order` 看下单时间归属，`--dimension finance` 看财务结算归属
```

---

## Test Plan

- 单元测试：

  - 配置读写、keychain fallback、环境变量优先级。

  - token 请求、token 缓存、过期刷新（401 自动重试）、错误脱敏。

  - **令牌桶限流器**：并发请求被正确排队，不超过 20 req/min。

  - Registry 转换器：65 个 path 全部转换，必填参数不丢失，pagination/riskLevel 正确标注。

  - 命令自动注册：65 个端点命令全部注册，help 文本正确。

  - 参数解析：`--params`、`--data`、stdin、`@file`、`--open-channel-id`。

  - 分页聚合：`page_rows` 和 `date_range` 两种策略的边界情况。

- 集成测试：

  - mock CaptainBI server 覆盖认证、分页、GET、POST、错误码、**429 Rate Limit**。

  - `cbi api GET /v1/open_user/get_site_list` dry-run 和真实 mock 调用。

  - `--page-all` 聚合多页，并限制 `--page-limit`。

  - **429 退避恢复**：mock server 在第 3 页返回 429，验证客户端退避后继续。

  - **401 静默刷新**：mock server 返回 401，验证客户端自动刷新 token 并重试。

- CLI 快照测试：

  - `--help`、缺必填参数、未知命令建议、JSON/table/csv 输出稳定。

  - `--machine` 模式输出格式稳定。

  - exit code 语义正确（0/1/2/3/4）。

- 契约测试（`cbi doctor`）：

  - 对每个接口发最小请求，验证响应 `code` 字段存在、`data` 类型正确、分页接口有 `max_result`。

  - 可在 CI 中定期运行（需真实凭证，标记为 optional job）。

- 安全测试：

  - 日志、错误、dry-run、测试快照中不得出现 token、client_secret、authorization 明文。

  - POST 风险命令没有 `--confirm` 时不得真实发送。

  - `--machine` 模式下 stderr 不泄露敏感信息。

---

## 风险等级对照表

| 接口 | 风险等级 | 行为 |
| --- | --- | --- |
| 所有 GET 接口（55 个） | `read` | 直接执行 |
| `set_channel_operation_mode` | `write_safe` | 提示但不强制确认 |
| `set_goods_operate_user` | `write_safe` | 提示但不强制确认 |
| `set_goods_group` | `write_dangerous` | 强制 `--confirm` |
| `edit_goods_group` | `write_dangerous` | 强制 `--confirm` |
| `upload_fbm_order_ship_info` | `write_dangerous` | 强制 `--confirm` |
| `set_rule`（运营费用） | `write_dangerous` | 强制 `--confirm` |
| `set_goods_cost`（产品成本） | `write_dangerous` | 强制 `--confirm` |
| `sync_shipment` | `sync_trigger` | 强制确认 + 警告外部副作用 |

---

## Assumptions

- 第一版采用 `Go+Cobra`，命令深度为“全量 API + 少量快捷命令”，命令树按业务域组织。

- CaptainBI 当前 OpenAPI 以 65 个接口为准；后续通过 `tools/gen-registry` 刷新 Registry 元数据，避免人工维护全量命令。

- Token 接口按 OAuth2 client_credentials 思路接入；若实际返回字段与文档不同，认证模块用兼容解析集中处理。Token 过期时 HTTP Client 自动静默刷新。

- 写入类接口先用 `--data` JSON 接入，等真实样例稳定后再增加字段级 flags。风险等级在 Registry 元数据中静态标注。

- 默认不内置任何用户密钥、店铺密钥或示例真实账号；所有示例使用占位符。

- **Rate Limit 20 req/min 是第一优先级约束**，所有涉及多次请求的功能（分页、批量、doctor）必须内置限速。

- 主二进制名 `cbi`，仓库名 `captainbi-cli`，npm 包名 `captainbi-cli`。

---

## 与原计划的关键差异总结

| 维度 | 原计划 | 优化版 |
| --- | --- | --- |
| 架构核心 | 直接从 OpenAPI 生成命令代码 | 增加 Registry 中间元数据层 |
| 命令组织 | 按路径前缀扁平展开 | 按 6 个业务域分组 |
| Rate Limit | 未提及 | I0 即内置，令牌桶 + 429 退避 |
| 分页策略 | 统一 `page_rows` | 区分 `page_rows` / `date_range` / `none` |
| 风险控制 | I6 才处理 | 元数据层标注，生成器自动挂载保护 |
| Schema 命令 | 无 | I2 实现，Agent 调用前可查参数 |
| Doctor 命令 | 无 | I4 实现，契约测试 |
| Agent 输出 | 无 | `--machine` 模式 + 语义 exit code |
| Skills 结构 | 提及但未细化 | 6 域拆分 + 决策树 + references |
| 迭代验收 | 只描述“做什么” | 每个迭代有明确 Done 条件 |
| 二进制名 | `captainbi`（9字符） | `cbi`（3字符）+ 别名 |
| Token 刷新 | 提及但未明确策略 | 401 自动静默刷新，I0 实现 |
