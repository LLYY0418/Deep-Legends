# R127 执行台账：总览战绩列表加载慢、图标长时间是文字占位

版本 **0.12.17**（`desktop/package.json` 与 `desktop/package-lock.json` 两处均已递增；上一轮 R124/R126 为 0.12.16）。
工单：`docs/history/worklists/WORKLIST-R127-OVERVIEW-MATCH-LIST-SLOW-LOAD.md`（诊断人 Claude，执行人 GPT）。
执行范围：P0 全部 4 条、P1-a 全部 4 条、P1-b 全部 3 条、P1-c 全部 5 条。P2 三条按工单「不要求本轮处理」未动。

---

## 0. 本轮最重要的一条新证据（工单里没有的）

工单 P1-c 猜测「OP.GG 的 `created_at` 如果是开局时间」。用仓库里已有的**真实抓包** `backend/testdata/r112-opgg-average-tiers.txt`（20 行，含 `created_at` 与 `game_length`）做相邻对局重叠检验——同一名玩家的相邻两局不可能在时间上重叠：

| 假设 | 不可能的重叠 | 相邻间隔 |
|---|---|---|
| `created_at` = 开局时间 | **3 处**（-21s、-105s、-574s） | 出现负值 |
| `created_at` = 结束时间 | **0 处** | 158s–63341s，全部合理 |

结论：**OP.GG 的 `created_at` 是「结束时间」口径**，不是开局时间。这比工单的猜测更靠后——旧实现拿 Riot 的 `gameCreation`（房间创建，比开局还早 BP + 读条 2–4 分钟）去比 OP.GG 的结束时间，差值 = BP + 读条 + 整场时长 ≈ 25–35 分钟，远超 180 秒容差，所以 23 场一场都对不上。这条检验已固化成测试 `TestR127OPGGCreatedAtIsGameEndNotStart`，抓包被换掉或结论被推翻都会红。

---

## 1. P0　诊断盲区

| 项 | 改动 |
|---|---|
| P0-1 | `backend/features.go`：`local_request_client` 的 reason 白名单加 `failed`；endpoint 白名单加 `image`、`friends`、`pro-players`、`section-loader`。`backend/web/runtime.js` 的传输层原本已经放行这些 endpoint，是后端在拒收 |
| P0-2 | `backend/web/image-queue.js` 失败上报补 `queueWaitMs`（入队→拿到名额）、`loadMs`（名额→load/error）、`activeSlowCount`（当时占着名额超过 3 秒的图片数）、`imageSource`（`lcu`/`communitydragon`/`ddragon`/`gtimg`，只报来源类别不报路径）；顺带把前端 10 秒超时的 `errorKind` 从一律 `network` 改成 `timeout`。`features.go` 落 `queue_wait_ms`/`load_ms`/`active_slow_count`/`image_source`，且只在 `endpoint=image` 时写，其它端点不会凭空多出图片字段。采样口径未变（仍是 `local_request_client` 全局 10 秒一条） |
| P0-3 | `match_tiers_overview_batch` 增加 `durationMs`（第一批请求发出→最后一批返回），前端 `recordMatchTierOverviewBatch` 与后端 `features.go` 两侧同步 |
| P0-4 | `backend/r127_test.go`：4 条「上报 → 落日志」断言 + 1 条「白名单没有放宽过头」（未知 reason 仍 400 并留 `client_diagnostic_rejected`；未白名单的 endpoint 被丢弃而不是原样落盘，资源路径不泄漏）。`backend/web/r127.test.cjs`：字段能穿过 `runtime.js` 真正落到 POST body |

**对抗变异（已实跑验证）**：把 `"failed"` 从白名单删掉后重跑，`TestR127ImageFailureDiagnosticReachesLog`、`TestR127ClientDiagnosticAcceptsNewFailureEndpoints`、`TestR127DiagnosticAllowlistStaysClosed` 三条全部 FAIL（`status = 400, want 204`），随后已还原。

## 2. P1-a　慢图不能堵住快图

1. **分道**（`image-queue.js`）：远程来源单独限额 `REMOTE_LANE_LIMIT = 2`；本机道用满剩余名额。**与工单示例（本地 4 + 远程 2 = 6）的差别**：总名额保持 `IMAGE_QUEUE_LIMIT = 5` 不变，本机道 = 5 − 在途远程数，即远程满载时本机仍有 3 个名额、没有远程图时本机拿满 5 个。这样纯本机页面（国服总览的常态）吞吐不比改动前低，同时 5 张图 + 1 条 SSE 正好不超过浏览器同源 6 条连接。远程道满员时是 `continue`（跳过这张）而不是 `break`，否则慢图又会堵住后面的快图。
   分道判据：URL 带 `source=` 且不是 `lcu` → 远程；另外**强化符文图标虽然走不带 `source=` 的 `/api/image?path=…&art=2`，但后端会在 LCU 400 后回退 CommunityDragon**，所以按 `augments/icons` 路径与 `art=2` 标记提前归入远程道——不这么做，本轮真正卡死队列的那批图根本不在远程道里。
2. **主机级退避**（`backend/asset_cache.go`）：新增 `loadAssetFromHost`，同一远程主机连续 3 次 `DeadlineExceeded` 后 60 秒内该主机直接快速失败（`errCachedAssetFailure`），并记一条 `asset_host_backoff`（host、连续超时次数、退避秒数）。任何非超时结果（成功、404、连接被拒）都说明主机可达，计数与退避立即清零；`Canceled`（浏览器自己取消）不计入。远程调用点已全部改走它：`loadChampionRemoteAsset`、`loadCommunityDragonAsset`、`serveCommunityDragonImage`。
   **口径说明**：工单写的是「`loadAsset()` 对 `DeadlineExceeded` 也记失败，但……更合理的是按主机退避」。本轮按后半句实现为主机退避，**没有**给单个 key 加超时负缓存——handler 的 8 秒整体预算耗尽时，排在后面的候选会立刻拿到 `DeadlineExceeded`，按 key 缓存会把这种「没轮到」误记成「这个资源不可用」。
3. **远程超时缩短**（`backend/champion_images.go`）：新增 `publicRemoteImageTimeout = 3s`，`fetchDirect` 按路径用 `remoteImageBudget()` 取预算——小图标 3 秒，原画/加载图（路径含 `splash`/`centered`/`loadscreen`/`loading/`/`/skin/big`/`skinanimation`）仍是 8 秒。工单只论证了小图标 3 秒够用（ddragon 150–250ms、communitydragon 270–650ms），把 1–2MB 的皮肤原画也压到 3 秒会在慢网络下把原画改坏，所以按体积分开。handler 层整体预算 `publicImageTimeout` 仍是 8 秒。
4. **查清 LCU 400**（`backend/champions.go`、`backend/main.go`）：`augment_icon_fetch` 增加 `lcu_status`——`0` = 没有 LCU 映射或未连接客户端（根本没问）、`-1` = 问了但没有 HTTP 状态（超时/内容不是图片/命中失败缓存）、其它 = LCU 真实状态码。为此把 `loadChampionAssetFromClient` 拆出带状态的 `clientAssetStatus`，并让 `/api/image` 的回退路径也上报（生产日志里的 400 正是这条路径，以前它一条日志都不发）。`augmentIconPathTemplate` 现在也认客户端路径前缀，模板记作 `client-game/…`、`client/…`，仍然只记模板不记具体图标 ID。

## 3. P1-b　国服平均段位

1. **只要段位的模式**（`backend/rank_insights.go`、`backend/gameplay.go`）：`playerRankScoreWithCacheStatus` 与 `loadRanksWithFallback` 增加变参 `tierOnly`（既有调用点与护栏测试一个字都不用改）。`tierOnly` 时：`rankSourcesTierOnly` 把 SGP 提到最前（实测 60–130ms，LCU 中位 550ms、最长 2.4 秒），并且 LCU 结果即使「胜负场未完整验证」也直接采用，不再补一次 SGP（`ranked_data_source_decision` 的 `fallback_reason` 记 `tier-only-prefers-sgp`）。match-tiers 端点是唯一传 `true` 的调用方。
   **缓存隔离**：`rankScoreCacheKeyScoped` 给 tier-only 结果加 `|tier-only` 作用域，个人资料页等需要胜负场的路径不会读到没有胜负场的条目（工单明确要求「不影响个人资料页」）。
2. **并发** `matchTiersRankConcurrency` 4 → 8。注意这个常量同时被专精符文的对手段位查询复用，已在注释里写明。
3. **前端并行 + 立即回填**（`backend/web/gameplay.js`）：批次改为最多 `MATCH_TIERS_PARALLEL_BATCHES = 2` 批同时在途；每批返回就把「参与者已全部有结果」的对局立刻回填，不等其它批次；出错时已回填的结果保留，只把没算出来的标失败等重试；`recordMatchTierOverviewBatch` 带上 `durationMs`。

## 4. P1-c　韩服平均段位

1. **诊断**（`backend/rank_insights.go`）：`opgg_match_tiers_result` 增加 `rows`、`rows_with_tier`（20 行里几行真有段位）、`matched_bases`（命中用的是 end/start/creation 哪个基准）、`cache_hits`、`opgg_requests`、`source_lag_seconds`（OP.GG 最新一行比 Riot 最新一场早多少秒），以及每场未匹配对局与最近一行的 `unmatched_time_gap_seconds` / `unmatched_duration_gap_seconds`（只记秒数，不记任何 ID）。
2. **对齐时间基准**：`riotMatchInfo` 里已解析的 `gameStartTimestamp`/`gameEndTimestamp` 现在透传到 `gameplayMatch.StartedAt/EndedAt`（缺结束时间时用开局 + 时长兜底，不编造），前端请求带 `startAt`/`endAt`，后端 `matchTierMatchRequest` 收下；`matchOPGGAverageTierDetailed` 把三个基准分别和 OP.GG 的 `created_at` 比，取差值最小的，并把命中基准写进诊断。旧的两参数 `matchOPGGAverageTier` 保留为兼容入口，既有测试无需改动。
   **夹具**：OP.GG 一侧用真实抓包，Riot 一侧按「结束 = `created_at`、开局 = 结束 − 时长、房间创建 = 开局 − 180 秒」构造，7 场全部命中且命中基准全是 `end`。**对抗变异**：清掉 `StartAt`/`EndAt`（即改回只用 `gameCreation`）后同一批数据 0 命中，测试 FAIL。
   *诚实说明*：Riot 一侧是按上面第 0 节的检验结论构造的，不是同一场对局两边原始时间的真实配对抓包（仓库里没有）。生产日志里的 `matched_bases` 与 `unmatched_time_gap_seconds` 可以在下一份真机日志里直接证实或推翻。
3. **长期缓存**（新增 `backend/match_tier_cache.go`）：按 gameId 把匹配到的平均段位落盘 7 天（`match-tiers` 目录，最多 4000 条 / 8MiB），命中时 `handleRiotMatchTiers` 直接返回、**一个 OP.GG 请求都不发**；只缓存确实匹配到段位的结果，「暂时查不到」不写盘。键前缀 `kr-match-tier-v1|KR_` 已加进 `championCacheDiskAllowed` 白名单。
   **隐私声明同步**（红线）：`features.go` 的 `stores` 增加对应条目、`quality_test.go` 的 `privacyStoreDirectoryCoverage` 登记 `match-tiers`、`README.md` 增补一段；结构性隐私测试 `TestPrivacyStoresDeclareEveryLocalDirectory` 通过。
4. **提前取 + 合并请求**：`startOPGGGameTiers` 在韩服总览加载时（`riot_api.go`，紧挨既有的 `startOPGGHistoricalRanks`）后台并行预热 OP.GG 第一页，只发 GET，不在总览里发平均段位 action；`opggGameTiers` 自身的 flight 合并保证不重复取页。前端韩服分支加 120ms 合并窗口（`MATCH_TIER_KR_COALESCE_MS`），把观测器分几次回调送进来的同屏对局攒成一次请求，收集与占位同步完成，并发调用只会有一批真正发出去。
5. **来源过旧的明确提示**：OP.GG 最新一行比 Riot 最新一场早超过 30 分钟（`opggSourceStaleSeconds`）时，未匹配场次返回 `{sourceStale:true}`，前端显示「来源暂未收录」并在 tooltip 说明「OP.GG 不会自动更新，不是查询失败」，不再是一条让人以为出 bug 的横线。

## 5. 验证

- **Go 全量**：`go test ./backend -count=1` → ok（199.5s）；`go test ./backend -race -count=1` → ok（246.2s）。`go vet ./backend` 干净，`gofmt` 干净。
- **前端全量**：`node --test backend/web/*.test.cjs desktop/*.test.cjs` → 896 条，**895 通过，1 条既有失败**（见下）。
- **变异测试实跑记录**：① 删白名单 `failed` → 3 条 Go 测试 FAIL；② 删分道闸门 → 20 张本地图 0 完成、远程占满 5 个名额；③ 并行批次改回串行 → 只有 1 批在途；④ 删 `runtime.js` 新字段放行 → POST body 里三个字段消失；⑤ 匹配改回只用 `gameCreation` → 真实抓包 0 命中。
- **改动过的既有测试（都是被本轮契约变化直接要求，不是为了让测试变绿）**：
  - `backend/r107_test.go`：`TestR107ImageBudgetReachesUnderlyingTransport` 的基准从写死的 7–8.1 秒改成按路径取 `remoteImageBudget`（护栏本身「外层预算必须真的传到底层 Transport」保留）。
- **新增测试**：`backend/r127_test.go` 14 条（P0 诊断 4 条、主机退避 2 条、远程预算、`lcu_status`、只要段位模式、真实抓包时间基准 2 条、差值诊断、长期缓存 2 条）；`backend/web/r127.test.cjs` 9 条（分道防饿死 + 变异、失败上报字段、超时口径、传输白名单 + 变异、国服批次并行 + 变异 + 部分失败、韩服同屏合并与来源未收录）。
  - `desktop/overview-render.test.cjs`：三处 harness 补 `MATCH_TIER_KR_COALESCE_MS: 0`、`MATCH_TIERS_PARALLEL_BATCHES: 2`，以及失败路径新用到的 `scheduleMatchTierRetry` / `recordMatchTierOverviewBatch` 桩（缺任何一个都会在函数体里抛 ReferenceError，被 catch 吞掉后表现为「请求数为 0」这种误导性失败）；诊断断言补 `durationMs`。
  - `backend/opgg_insights_test.go` 的 `TestLoadRiotOverviewLoadsOPGGHistoryWithoutMatchTierRequests`：**这里确实放松了一条旧不变量**——原来断言「总览期间对 op.gg 只发 GET、总共 1 次请求」，而 P1-c.4 要求的并行预热必然会在后台发一次对局列表 POST。现在断言「历史段位 GET + 预热身份 GET + 恰好 1 次对局 POST」，并等两个后台 goroutine 落定后再计数以免竞态。仍然保留的是：总览本身不等待预热、返回值里不带平均段位、预热只取一页。
- **自己踩到并修掉的问题**：新写的测试辅助函数让三个 provider 共享一个无锁 slice，`observeAssetFetch` 的 10 秒延迟 flush 在别的测试期间并发追加，被 `-race` 抓到；改成带锁的 `r127EventCollector` 并按事件名过滤后 `-race` 全绿。

## 6. 本轮没有证实的（需要真机 + 下一份 diagnostics）

本机是 macOS，没有连用户的 Windows LCU，也没有国内网络环境，以下验收项**无法在这里实测**，只能等真机日志：

1. P0：人为让 communitydragon 不可达后，日志里出现 `local_request_client endpoint=image reason=failed` 且带 `queueWaitMs`、`client_diagnostic_rejected` 不再出现。
2. P1-a：同条件下英雄头像/召唤师技能/装备在列表出现后 2 秒内全部显示为图片，只有缺失的符文图标显示占位。
3. P1-b：31 个去重玩家、缓存全冷时 `match_tiers_overview_batch.durationMs` ≤ 2.5 秒。
4. P1-c：`opgg_match_tiers_result.matched / matches` 接近 OP.GG 页面实际带段位的比例（约 7/20）而不再是 0；`matched_bases` 应当几乎全是 `end`——**如果实际是 `start` 或 `creation`，说明第 0 节的重叠检验结论需要修正，届时以真机日志为准**；重开同一个人时 `opgg_requests: 0`。
5. `augment_icon_fetch.lcu_status` 的真实分布（400 是路径大小写、`_large/_small` 候选，还是客户端真的没打包），本轮只补上了观测手段，没有改候选策略。

## 7. 顺带发现的、与 R127 无关的既有问题（未处理）

- `desktop/overview-render.test.cjs` 的 `R56 工具页状态、确认、下拉与领奖契约完整` 在当前工作区**先于本轮就已失败**：断言 `.facade-left [data-facade-banners]`，而 `backend/web/suite.js` 在工作区里已被并行的 facade 迁移工单（R123–R126）改掉了这个入口（HEAD 里 3 处，工作区 0 处，该文件有 71 增 / 182 删的未提交改动）。本轮没有碰 `suite.js`，也没有替这条工单做决定。
- 执行期间 `backend/position_contract_probe_test.go` 有一个与本工单无关的编译错误（`positionProbeVerdict` 参数类型不匹配），会阻断整个测试包编译；期间由并行会话自行修好，`backend/position_contract_probe.go` 目前仍未 gofmt。
- `installer/` 与 `tools/` 是独立 Go 模块，本轮未改动，未单独跑测试。
- **收尾时的前端状态说明**：本轮最后一次完整验证（约 20:20）是 897 条 / 895 通过 / 只有下面这条 R56 失败。之后并行会话在 20:46 大改了 `backend/web/champions.js`（656 行变更，把 `renderMeasurementTechnique` 等挪进新的 `backend/web/shared.js`）与 `champions.css`，前端失败数随即涨到 19 条，全部集中在海克斯详情（R116-B/E/F）、`R117 CSS variables`、`R117 core frontend contracts`、`2024 rarity UI`、R56 这些**本轮没有碰过的文件**上（我改的 `backend/web/gameplay.js` 停在 20:13，Go 侧全绿）。这些属于并行工单的在途状态，本轮不代为修复。

---

## 8. 复审轮：对抗性代码评审后的修复（同一天完成）

改完之后请了一个只读评审代理逐条挑刺，发现 5 个必须修的问题，全部已修并补了回归锁：

1. **主机退避被聚合错误抹掉**（阻断）：`serveCommunityDragonImage` 外层的 `cdragon-resolved:` 也走了 `loadAssetFromHost`，内层候选连续超时刚建立退避，外层返回的 `errCommunityImageCandidatesExhausted` 立刻把它清零——退避在这条最主要的生产路径上等于没有。修法：外层聚合键改回 `loadAsset`（每个候选已在内层各自归因），并且 `observeAssetHostResult` 对 `errCachedAssetFailure` 与 `errCommunityImageCandidatesExhausted` 既不清零也不累加。回归锁 `TestR127HostBackoffSurvivesAggregateErrorsAndSparesCachedAssets`。
2. **退避误伤已缓存的图**（阻断）：退避判定原本在 `loadAsset` 之前，主机一抖动，内存/磁盘里已有的图标也会变成占位。修法：把判定挪进 loader 内部，只在真的要出网时生效；并且只有「真的出过网」的结果才算主机的新鲜证据（`reachedHost`），否则一次缓存命中就会把退避抹掉。同一条回归锁覆盖这两点。
3. **预热会拉满 4 页**（阻断）：`startOPGGGameTiers` 传 `oldest=0`，而覆盖判断与提前停止条件都要求 `oldest>0`，于是一路拉到 `opggGamesMaxPages`，前台请求还得等同一个 flight。修法：`opggGameTiers` 增加变参 `maxPages`，预热传 1。回归锁：总览预热用例现在喂真实的 20 行抓包（游标非空），断言只发 **1** 次对局 POST。
4. **`/api/skin-art`、`/api/prestige-image` 被当成本地图**（阻断）：这两个端点没有 `source=` 参数，后端却去腾讯官方图片域名取原画，皮肤列表回退时照样能占满 5 个名额。修法：加入远程道判据，`imageSource` 也如实报 `gtimg` 而不是 `lcu`。
5. **一批失败连累另一批**（阻断）：国服并行批次用 `Promise.all`，任一批 reject 就立刻进 catch/finally，清掉全部 flight 占位，而另一批还在跑；失败批涉及的对局还被摘掉 pending 标记且不安排重试，整屏永久显示横线。修法：改成每批各自 try/catch，失败批的 ref 记入 `failedRefs`，`applyReadyGames` 遇到失败 ref 的对局直接跳过（宁可用残缺数据算出偏掉的平均值），全部批次落定后只把没算出来的标失败、保留 pending 并 `scheduleMatchTierRetry`；`match_tiers_overview_batch` 在有批次失败时如实报 `reason: "failed"`（白名单同步加了 `failed`）。回归锁：`backend/web/r127.test.cjs` 的「一批失败不连累另一批」。

另外修掉的三条非阻断问题：

6. **预热测试是假阳性**：原来的 fixture 只返回历史段位片段，OP.GG 身份解析必然失败，预热在发对局请求之前就退出了，「总览期间不发 POST」这条断言其实什么都没盖到。现在 fixture 同时给出可解析的身份页与真实 20 行对局页，断言变成「历史 GET + 身份 GET + 恰好 1 次对局 POST」，顺带钉死第 3 条。
7. **韩服合并窗口的补扫绕过失败冷却**：补扫只看了缓存与 flight，没看 `nextRetryAt`，会把仍在 30 秒冷却里的对局重新塞进请求、提前烧掉重试次数。已补上冷却检查。
8. **`opggNearestRowGap` 拼错了证据**：时间差与时长差分别取全表最小值，可能来自不同行，拼出一个不存在的组合。改成只报「时间最近的那一行」的两个差值。
9. **「保留 7 天」名不副实**：过期条目只是读的时候当没有，文件仍留在目录里（目录未满时淘汰逻辑不会碰它）。改成读到过期就删除文件，与 `stores` 声明和 README 一致。

复审轮之后的最终验证：`go test ./backend -race -count=1` → ok（246.4s）；`go vet ./backend` 干净；`gofmt` 干净；`node --test backend/web/*.test.cjs desktop/*.test.cjs` → 897 条，895 通过，仍只有第 7 节那条与本轮无关的 R56 既有失败。测试总数更新为 `backend/r127_test.go` **14** 条、`backend/web/r127.test.cjs` **9** 条。
