# CaptainBI Agent 快速开始

当一个新的 Agent 需要日常查询 CaptainBI，并在用户明确批准后执行写入时，优先走这条路径。

1. 安装 CLI：

```bash
npm install -g https://github.com/kirkzwy/captainbi-cli/releases/download/v0.3.0/captainbi-cli-0.3.0.tgz
cbi --version
cbi doctor local --machine --format json
```

如果是私有仓库或遇到 GitHub 限流：

```bash
export GITHUB_TOKEN='<github_token>'
export CAPTAINBI_CLI_GITHUB_TOKEN='<github_token>'
```

如果网络需要走代理：

```bash
export HTTP_PROXY=http://127.0.0.1:7890
export HTTPS_PROXY=http://127.0.0.1:7890
export ALL_PROXY=http://127.0.0.1:7890
export NODE_USE_ENV_PROXY=1
```

2. 如果宿主环境支持 skills 安装器，安装 skills：

```bash
npx skills add kirkzwy/captainbi-cli -y -g
```

如果宿主环境不支持这个安装器，就直接加载本仓库里的 `skills/` 目录。

3. 配置凭证，避免把密钥写进命令历史：

```bash
printf '%s' "$CAPTAINBI_CLIENT_SECRET" | cbi config init \
  --client-id "$CAPTAINBI_CLIENT_ID" \
  --client-secret-stdin \
  --non-interactive
```

4. 获取 token 并发现店铺：

```bash
cbi auth token --machine --format json
cbi +shops --machine --format json
```

5. 保存店铺别名：

```bash
cbi config channels add main '<open_channel_id>'
cbi doctor local --machine --format json
```

6. 跑第一个只读任务：

```bash
cbi --channel main +goods --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
cbi --channel main +inventory --modified-since <unix_seconds> --modified-until <unix_seconds> --summary --machine --format json
cbi --channel main +ads-campaign-report --date <YYYYMMDD> --summary --machine --format json
cbi --channel main +reviews --summary --machine --format json
```

Agent 默认规则：

- 优先使用 `--machine --format json`。
- 优先使用 `--channel <alias>`，不要直接传裸的 OpenChannelId。
- 大数据量任务先跑 `--summary`。
- 全量导出优先配合 `--output-file`。
- 除非用户明确批准具体动作，否则不要执行写入命令。

7. 用户明确要求写入时，先查看 schema 并生成预览：

```bash
cbi schema goods.set-operate-user --machine --format json
cbi --channel main goods set-operate-user \
  --goods-id <goods_id> \
  --operation-user-admin-id <operator_id> \
  --dry-run --machine --format json
```

执行到这里必须停下，把 method、path、channel alias 和 body 交给用户确认。用户批准完全相同的请求后，才能执行：

```bash
cbi --channel main goods set-operate-user \
  --goods-id <goods_id> \
  --operation-user-admin-id <operator_id> \
  --confirm-request <request_hash> \
  --machine --format json
```

写入成功后查询受影响资源进行回读验证。请求发生变化、hash 过期或已使用时，必须重新 dry-run。
