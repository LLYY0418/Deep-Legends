# R208 执行账本

日期：2026-10-04。工单：[R208](WORKLIST-R208-RIOT-RELAY-APP-LEVEL-429-COOLDOWN-DAILY-QUOTA-AND-SHARED-IP-LIMIT.md)。执行基线为 R206 `e2116023`，工作区同时包含 R207；当前 desktop/package.json 与 lockfile 为 **0.12.71**，保持该合并版本，不重复递增。用户明确暂停桌面构建和发布；本轮只运行测试/静态检查，以及工单明确要求的 Worker 部署。

## P1 应用级冷却

- Worker 解析 `X-Rate-Limit-Type`。application 的标记按 Riot host 摘要保存（asia/kr 分开），位于响应缓存之前，因此其他路径和已缓存路径都返回429；method/service/缺失或未知类型按 host + path 冷却。缺失 Retry-After 默认5秒，支持秒数和 HTTP 日期，范围1–86400秒。
- 标记保存绝对截止时间，命中返回向上取整的剩余秒数，不再每次重新返回初始间隔。过期或损坏标记不延长冷却。应用与 IP 同时限流时优先返回应用的剩余时间，所有请求仍计入 IP binding。
- 返回 `X-Relay-Cooldown: application|method|service`；IP429保持无此头、Retry-After60。Worker 自定义日志仅增 `limit_type`，原有 category/status/cache_hit 保留，observability 仍关闭。冷却和缓存使用 Cloudflare 接入地点缓存，并不声称全球强一致。
- Go 的 `riotRelays` 是所有 provider/玩家共用状态。application 按工单 P1-2要求暂停整个 Riot 功能（软件端跨 host），Worker 按 P1-1要求区分 host；IP同样全局暂停中转。method/service只阻断同路径。请求排队前与获得本地配额后都检查共享状态，阻止排队请求继续访问中转；已经发出的请求不能撤回。
- relay429不写入个人直连的本地冷却桶；保存用户 Key 后继续优先直连 Riot。

## P2 免费额度与诊断

- probe和实际请求统一识别非 application/json（支持带 charset 的 JSON）、正文 error1027及 JSON code/error.code1027，返回 `quota_exhausted`、503与现有“战绩服务暂时不可用”。没有在界面新增说明文案。
- 全局中转熔断到下一次 UTC00:00（北京时间08:00），期间不探测、不尝试其他玩家或其他路径。个人 Key 不受影响。UTC边界使用带+08:00的时间测试；多个配置 origin的精确匹配及未知 origin隔离也覆盖。状态为进程内共享，不持久化熔断期限。
- `riot_relay_probe result=quota_exhausted`记录状态和实际耗时。`riot_relay_request_summary`按有实际出站请求的10分钟窗口记录 requests、rate_limited、failures分类和窗口时长，包括探测，不计本地被拦截的请求；定时器在没有后续访问时也能输出该窗口。没有 URL、Key、Riot ID、PUUID或IP字段。
- [Worker README](../relay/riot-worker/README.md)补后台指标、免费每日10万/UTC重置、Paid每月最低5美元含1000万请求，以及额度与缓存计费区别；依据 [Cloudflare limits](https://developers.cloudflare.com/workers/platform/limits/) 与 [pricing](https://developers.cloudflare.com/workers/platform/pricing/)。Riot限流依据 [Developer Portal](https://developer.riotgames.com/docs/portal)，未将生产Key的某个起始额度作为固定限额写死。
- Worker受控的429/502/503改为JSON响应，避免普通限流/网络失败被新的非JSON判据误判为每日额度用尽。白名单外404保持原行为。

## P3 共享 IP 与缓存

- binding配置改为300次/60秒。Node模拟验证同一IP第301次429、其他IP仍可用；不对线上服务发301次请求。
- 既有比赛缓存链 memory → disk v2/v1 → provider.get 已覆盖中转，无需改写缓存读取顺序。新增测试在中转日额度冷却下仍命中磁盘，matches_from_disk=1且网络请求=0。
- Go模拟六场战绩：四场成功后IP429，返回并保留四场预览；59秒内没有出站请求；第60秒恢复，只补两场，先前成功详情各只请求一次。账号/列表与详情的既有缓存继续生效。
- 前端直接编译实际 api/loadOverview 实现，流式429带 cooldownScope=ip与60秒；验证保留四场、不出现错误或toast、59999ms不重试、60000ms请求同一页并完整显示六场。503 quota_exhausted显示固定不可用文案，不进入429重试定时器。生产前端复用已有退避流程，未修改 R207 同时编辑的 gameplay.js。

## 验证

- Worker：12/12通过，见 [worker.log](history/reports/r208/worker.log)。覆盖application其他路径/缓存、跨host、过期/剩余秒、method/service/缺失头、IP301以及双重限流。
- 新增Go定向测试、既有R206 relay回归均通过；新增前端2项与R206不可用文案回归通过。
- 两项指定源码变异都由行为断言杀死，非编译失败，见 [mutations.json](history/reports/r208/mutations.json)、[Worker变异日志](history/reports/r208/mutant-worker.log)、[Go变异日志](history/reports/r208/mutant-go.log)。仅在临时副本/Go overlay中变异，正式源码保持正确实现。
- `go test ./backend -count=1`：通过，250.470秒，见 [go-full.log](history/reports/r208/go-full.log)。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`（仅使用tap报告格式）：1163项，1162通过、1项平台门禁跳过、0失败，272.576秒；见 [node-full.log](history/reports/r208/node-full.log)。包含当前 R207 并行改动的回归。
- `go test -race ./backend -run '^(TestR208|TestR206Relay)' -count=1`：通过，2.421秒；见 [go-race.log](history/reports/r208/go-race.log)。
- `go vet ./backend` 与最终 `git diff --check` 均通过，见 [checks.json](history/reports/r208/checks.json) 与原始日志。

## Worker 部署与实测

- 使用 Wrangler4.147.0执行 `npx wrangler deploy`，成功上传并绑定自定义域名。版本 `3135e103-6727-4084-83d1-a0fd292f3504`，binding确认300 requests/60s；见 [deploy.log](history/reports/r208/deploy.log)。未读取、重写或输出RIOT_API_KEY Secret。
- 实测 `https://riot.yinxiaobia.net/r/kr/lol/status/v4/platform-data`：200，application/json，正文可解析为JSON；白名单外 `https://riot.yinxiaobia.net/r/kr/lol/challenges/v1/challenges/config`：404，Not found。原始核验元数据与UTC时间见 [deployment.json](history/reports/r208/deployment.json)。
- 代码与自动验证、Worker部署均已完成。保持工单进行中，等待用户真机验收及恢复后的合并桌面发布。

## 构建、发布与真机边界

未构建桌面程序/安装包，未发布GitHub Release、推送或更改tag。Worker部署属于R208明确要求的在线服务修复，独立于暂停的桌面发布。恢复后与R206/R207合并，按R199证据规则发布。

用户真机验收仍待：不保存个人Key时打开绝活哥/职业页战绩；隔几天查看Cloudflare每日请求数。没有真实触发Riot应用限流或Cloudflare日额度用尽，自动回放不替代真实配额事件。


## 0.12.71 合并正式发布

2026-10-04 用户要求「发布新版本」，恢复构建/打包/发布。本工单随 **0.12.71 public Latest** 正式发布，release id **403010042**，`draft=false`、`prerelease=false`、`isLatest=true`，匿名 Latest 清单为 **0.12.71**。源码 `ea6d64c99a3e4ab86f9d9055a7a3fc4b300de4e6`，指纹 `696da05d0ad0`。正式 Windows 构建与完整质量/真实升级流水线全部通过，公开附件逐字节校验通过，旧 Release/草稿/标签保留。完整发布与附件证据见 [合并发布账本](history/ledgers/release-0.12.71-execution-ledger.md)。原有用户真机验收待办保持，不以 runner 结果提前关闭工单。
