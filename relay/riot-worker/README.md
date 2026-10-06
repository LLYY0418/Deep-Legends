# Riot 中转服务

当前服务已部署为 `deep-legends-riot-relay`，绑定 `https://riot.yinxiaobia.net`。用户已在 Cloudflare 配置 `RIOT_API_KEY` Secret 并确认类型为 production；软件自 0.12.70 起内置此公开地址。Key 不进入源码或安装包。

1. 注册 Cloudflare，安装 Wrangler 4.36.0 或更新版本：`npm i -g wrangler`，执行 `wrangler login`。
2. 在本目录运行 `wrangler secret put RIOT_API_KEY`，交互输入自己的 Key。不要把 Key 放进文件、命令参数或仓库。
3. 运行 `wrangler deploy`。`wrangler.toml` 的限流 namespace_id 在同一 Cloudflare 账号中应独立；已有相同编号时换一个正整数。
4. 在 Cloudflare 为 Worker 绑定自己的 HTTPS 域名。国内经常无法访问 `*.workers.dev`，使用自有域名。
5. 部署到其他域名时，将域名（例如 `https://你的域名`，无路径）加入 `backend/riot_relay.go` 的 `riotRelayAddresses`，重新构建、发布软件。软件会按平台及接口类型追加 `/r/<平台或集群>/…`。

默认 development key 每24小时失效，不适合稳定中转。Personal key 的配额为每秒20次、每2分钟100次，仅适合本人或小型私有社区；公开发布的服务应申请 production key。依据：[Riot Developer Portal](https://developer.riotgames.com/docs/portal)。软件用户可保存自己的 Key，优先直连。

Worker 只允许实际使用的固定 Riot GET 路由，禁止任意代理和重定向。账号缓存1小时，比赛详情/时间线7天，比赛ID/段位/熟练度60秒。使用可淘汰、到期的 HTTP Cache API 缓存公开查询结果，不使用KV/D1/R2，也不写请求者IP、Riot ID或URL到应用日志；缓存key为摘要。Cloudflare调用日志默认关闭，自定义日志只含类别、状态码、缓存命中及限流类型 `limit_type`。支持 15 个 Riot 平台和 americas / asia / europe / sea 四个 Match-V5 集群；Account-V1 仅使用前三个入口，SEA 平台账号解析走 asia。软件发送经白名单核对的 `X-Riot-Platform` 枚举，响应缓存与 application 冷却均按主机和平台隔离（旧客户端按路由分组），method / service（含无类型头）再按路径隔离；返回剩余 `Retry-After` 和 `X-Relay-Cooldown`。缺失 `Retry-After` 默认5秒。冷却标记也使用短期缓存，和响应缓存一样只在接入地点生效。

每个来源IP配置每分钟300次；使用[Cloudflare Rate Limiting binding](https://developers.cloudflare.com/workers/runtime-apis/bindings/rate-limit/)，计数由Cloudflare按接入地点维护、最终一致。共享IP会共享额度，这不是全球精确计费。响应缓存也是接入地点本地缓存，见[Cache API](https://developers.cloudflare.com/workers/runtime-apis/cache/)。这些边界不进入软件界面。

验证：`npm test`（Node原生fetch/Response，模拟Riot、Cache和限流binding，不使用真实Key）。实际部署、域名连通性、配额及中国网络体验需部署后核对。

在 Cloudflare 后台打开「Workers & Pages → deep-legends-riot-relay → 指标（Metrics）」，按每日时间范围查看 Requests 请求数；缓存命中也算 Worker 请求，只节省 Riot 配额。免费版每天100,000次，UTC 0点（北京时间08:00）重置；用尽后 Cloudflare 返回1027错误页。软件遇到非 JSON 中转响应或1027后记 `quota_exhausted`，直到下一个 UTC 0点停止中转；用户保存自己的 Key 仍优先直连。见 [Workers 限额](https://developers.cloudflare.com/workers/platform/limits/)。

接近每日10万次时，可升级 Workers Paid，每月最低5美元，含每月1,000万次请求（另有CPU用量额度与超额费用）；见 [Workers 定价](https://developers.cloudflare.com/workers/platform/pricing/)。软件诊断 `riot_relay_request_summary` 每个有实际请求的10分钟窗口记录 `requests`、`rate_limited` 与 `failures` 分类，不记身份或完整URL；被本机冷却拦下的请求不计入。此计数用于估算单台软件的访问量，全服务用量以 Cloudflare 指标为准。

R221（原重号 R220）的 15 平台路由已于 2026-10-05 发布阶段重新部署。生产公开状态接口 15/15 返回有效 JSON，非法路由被拒绝；未读取或替换既有 Secret。真实玩家接口、Windows 游戏客户端及中国网络体验仍待验收。见 [0.12.73 发布记录](../../docs/history/ledgers/release-0.12.73-execution-ledger.md)。

R231：`GET /health` 返回 204，无上游请求、无身份参数。桌面探测只等响应头，每个 origin 限时 8 秒；旧 Worker 返回 404 时兼容回退 status 路径。部署新增健康路径前需用户批准。
