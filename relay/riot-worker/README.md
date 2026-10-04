# Riot 中转服务

由服务所有者部署。软件中的 `backend/riot_relay.go` 地址列表默认为空；此时没有环境变量或用户 Key 就仍显示未配置，不会请求占位域名。

1. 注册 Cloudflare，安装 Wrangler 4.36.0 或更新版本：`npm i -g wrangler`，执行 `wrangler login`。
2. 在本目录运行 `wrangler secret put RIOT_API_KEY`，交互输入自己的 Key。不要把 Key 放进文件、命令参数或仓库。
3. 运行 `wrangler deploy`。`wrangler.toml` 的限流 namespace_id 在同一 Cloudflare 账号中应独立；已有相同编号时换一个正整数。
4. 在 Cloudflare 为 Worker 绑定自己的 HTTPS 域名。国内经常无法访问 `*.workers.dev`，使用自有域名。
5. 将域名（例如 `https://你的域名`，无路径）加入 `backend/riot_relay.go` 的 `riotRelayAddresses`，重新构建、发布软件。软件会自动追加 `/r/asia/…` 或 `/r/kr/…`。

默认 development key 每24小时失效，不适合稳定中转。Personal key 的配额为每秒20次、每2分钟100次，仅适合本人或小型私有社区；公开发布的服务应申请 production key。依据：[Riot Developer Portal](https://developer.riotgames.com/docs/portal)。软件用户可保存自己的 Key，优先直连。

Worker 只允许实际使用的固定 Riot GET 路由，禁止任意代理和重定向。账号缓存1小时，比赛详情/时间线7天，比赛ID/段位/熟练度60秒。使用可淘汰、到期的 HTTP Cache API 缓存公开查询结果，不使用KV/D1/R2，也不写请求者IP、Riot ID或URL到应用日志；缓存key为摘要。Cloudflare调用日志默认关闭，自定义日志只含类别、状态码、缓存命中。Riot 429的同路径退避也使用短期缓存。

每个来源IP配置每分钟120次；使用[Cloudflare Rate Limiting binding](https://developers.cloudflare.com/workers/runtime-apis/bindings/rate-limit/)，计数由Cloudflare按接入地点维护、最终一致。共享IP会共享额度，这不是全球精确计费。响应缓存也是接入地点本地缓存，见[Cache API](https://developers.cloudflare.com/workers/runtime-apis/cache/)。这些边界不进入软件界面。

验证：`npm test`（Node原生fetch/Response，模拟Riot、Cache和限流binding，不使用真实Key）。实际部署、域名连通性、配额及中国网络体验需部署后核对。
