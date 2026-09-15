# WORKLIST-R89 韩服玩家查询：慢 + 装备图标不出（目标：彻底不依赖客户端）

诊断日期 2026-09-14。证据来源：用户提供的三份诊断日志（`lol-loot-diagnostics-0913-1646/1951/2329.jsonl`，去重后 28540 条），以及**在沙箱里把仓库当前代码真编译成二进制、在没有任何 LCU 的环境下真跑真 curl 的实测**（下文标注「沙箱实测」的都是这一类，不是推断）。

**2026-09-14 补充证据（第二轮，改变了执行顺序，见 0.4）**：用户按要求关客户端、停留 60 秒后提供了一张真机截图 + 一份新日志（`lol-loot-diagnostics-0914-1101.jsonl`）。这轮证据**推翻了 A 组能单独解释「装备格空白」的假设**，把 E-4 从「顺手补」升级为「A 组落地前必须先做」。执行方看到这份工单时请先看 0.4，再决定顺序。

---

## 0. 先纠正三个前提（这三条会直接改变修法方向）

### 0.1 韩服链路**早就不依赖客户端**了，唯一被客户端影响的是「图标资源通道」

`gameplay.go:711-713` 有明确分流：

```go
// 韩服玩家（英雄榜单点击、顶部搜索选择韩服、或此前打开的韩服页签）：
// 直接走 Riot 官方 API，不依赖本机客户端。
if strings.EqualFold(reference.Region, riotRegionKR) || (request.PlayerRef == "" && request.GameName != "" && request.Region == riotRegionKR) {
```

沙箱实测（无 LCU、无客户端进程）：`/api/gameplay/items` → **HTTP 200、249650 字节、868 件装备、冷启 1.72s、二次 3ms**；`/api/gameplay/perks` 200；`/api/gameplay/summoner-spells` 200。**所以「装备目录拿不到」这个猜想是假的，目录链路是健康的。** 真正出问题的是目录拿到之后那 100+ 张图标的加载通道，见 A 组。

### 0.2 **不是 Riot API key 等级的问题**

三份日志里 13 次 `riot_overview_cost` 采样，全部是：

```
rate_limited_count: 0   matches_failed: 0   first_error_kind: ""   matches_loaded: 20/20
```

全程没有一次 429，也没有一次 `riot_local_rate_limited`。本地保守限速（`riot_api.go:259-262`，15 次/秒、90 次/2 分钟）在单次搜索（约 25 次调用）下压根没被触发。

**但 key 等级是 C 组提速的前置条件**：Personal Key 上限 20/s、100/2min，所以 `riot_api.go:1072` 的 `semaphore := make(chan struct{}, 4)` 才只敢开 4。production key（500/10s、30000/10min）之后这个数字可以直接拉到 15~20。所以结论是「**现在的慢不是 key 造成的，但 key 是把它做快的前提**」。

**用户已在申请生产 key，这里把话说清楚给执行方看**：`riotAPIKeyCipher`/`riotAPIKey`（`riot_api.go:44-49`）是构建时用 `-ldflags` 注入进 exe 的，**分发出去的每一个安装包默认用的是同一把 key**（除非用户自己设 `RIOT_API_KEY` 环境变量覆盖，见 `riot_api.go:115-120`）。也就是说 20/s、100/2min 这个额度是**全体安装量共享的一个池子**。单机测不会撞上限（本节已证明），但装机量上去之后，多个用户同时查询会先于任何单人体验触顶——production key 抬高的是这个共享天花板，不是这一次搜索的墙钟。C-1-1 提高并发这条改动**必须等 production key 到位后再拉高倍数**，personal key 阶段维持 4~8，否则会把本地限速的排队点从「看不见」变成「用户能感知到」。

### 0.3 LeagueAkari「OPGG 秒出」不能直接对标——它查玩家战绩恰恰**必须依赖客户端**

已 clone 真源码核对（commit `5109b2f`）：

| | LeagueAkari | 我们 |
|---|---|---|
| OP.GG 用来干什么 | **只拿英雄数据**（tier list / build / 符文 / 大乱斗海克斯）。`src/shared/http-api-axios-helper/opgg/index.ts:24` `BASE_URL='https://lol-api-champion.op.gg'`，整个类 102 行里**没有任何 summoner / match 方法** | 英雄数据 + 韩服玩家的赛季汇总/当前对局/历史赛段 |
| 查任意玩家（含韩服）战绩 | 走 **Riot 内部 SGP 网关**，一次请求返回整页战绩（`sgp/match-history-query.ts:26-60`）。Token 来自**本地已登录客户端**的 `entitlements` 和 `league-session`（`src/main/shards/sgp/http-client-controller.ts:120-129`）——**关客户端就废** | 走 Riot 官方 match-v5，**20 场 = 20 次独立请求**，并发 4 |
| Riot 官方 API key | 全仓库 `api.riotgames.com` / `X-Riot-Token` **零命中**，完全不用 key | 用内嵌 personal key |
| OP.GG 缓存 | 几乎**没有**（`champion-data/service-controller.ts:48-96` 每次直连），只有 ARAM 平衡数据 30 分钟后台刷新 | 有磁盘+内存缓存，JSON 侧比它好 |
| OP.GG 超时/重试 | 8s + 1 次重试 | 12s，无重试 |

**所以「Akari 秒出」说的是英雄页的出装符文秒出，不是玩家战绩秒出。它玩家战绩快是因为 SGP 一次请求给整页——代价是必须有本地客户端在线。我们选了「不依赖客户端」这条路，代价就是 20 次 match-v5。这是架构取舍，不是我们写挫了。**

真正可抄的只有两点：① 它对 OP.GG 的请求路径极短（main 进程直连，无中转）；② 它的"取消上一个请求"式去重（`champion-data/request-controller.ts:4-22`）。我们 JSON 侧的缓存比它强，**图标侧比它弱得多**（A 组）。

### 0.4 新证据：装备格是「永久空白」，不是「慢慢出」——A 组不足以解释全部症状，E-4 必须提前

用户按要求关客户端、停留 60 秒后发来一张真机截图和一份新日志。两个事实：

1. **截图里召唤师技能图标、符文图标、英雄头像全部正常显示，唯独 6 个装备格是空的**——这三类走的是**同一套**「目录 + `ddragon:` 前缀 + `/api/champion-asset` 代理」通道（`web/gameplay.js:5415-5449`），同通道三个正常一个不正常，说明问题不在 A 组诊断的「通道无缓存所以慢」上——无缓存只会让**所有**走这条路的图标一起变慢，不会挑出装备单独失效。
2. **新日志里 `ddragon_item_purchasability_shape items:868` 成功产生**（`champions.go:1281-1286`），说明装备目录本身加载成功，和 0.1 节沙箱实测一致。目录是好的，图标通道（A 组诊断的缓存问题）是真的但只影响速度，两者都不能解释"格子空白"。

**这轮复核追加了一处后端代码线索，但没有实测坐实，留给 E-4 做实测**：`riot_api.go:834-916`（`convertRiotMatchInfo`）里逐个 participant 的 `ItemIDs = itemSlots(raw.Item0...raw.Item6)`（riot_api.go:901）走的是标准 match-v5 参与者字段，正常应该有值；没有发现任何按隐私或按 `hidden` 状态清空 `ItemIDs` 的分支（已用 `grep hidden` 核对，只影响显示名，不影响装备字段）。所以后端把 ID 塞丢的可能性看起来不大，但**没有实测验证**，不能排除。

**三个候选，谁对谁错必须靠 E-4 的埋点判定，本轮不许猜着改**：
- 候选①`/api/gameplay/items` 在用户的真机环境里请求失败或超时，`ensureItems()`（`web/gameplay.js:3190-3202`）catch 后只是把 `state.items` 设为 `null`、**不重试、不上报**，前端和诊断日志两头都看不出来。
- 候选②目录加载成功但**具体这批 match 里出现的 item ID 不在 868 件的 ddragon 目录里**（`itemIconFigure` 在目录里查不到 ID 时同样走 `pendingCatalogIcon` 返回空占位，`web/gameplay.js:5446-5449`，视觉上和「目录没加载」完全一样，区分不出来）。
- 候选③响应体里 `itemIds` 字段本身是空数组或全 0（后端某个环节把值丢了，例如 SGP 与 match-v5 两套 raw 结构没对齐——这条待 E-4 验证）。

**这三个候选视觉上完全一样（都渲染成空白 `is-empty`/`is-pending` 占位），单靠日志和截图分不出来，必须先让 E-4 的三条新埋点跑一轮才能定案。** 因此 E-4 的优先级从「P2 顺手补」提到 **P0，且必须先于或随 A 组一起发布**，A 组落地后如果用户复测装备格仍然空白，直接看新埋点定位到哪一个候选，不要重复本轮的排查过程。

---

## 1. 实测基线（改之前先把这几个数记下来，改完要用同样方式复测）

### 1.1 日志里唯一一段「客户端确实没启动」的窗口

`2026-09-13T04:16:16` app_start → `lcu_discovery result=process-not-found`（log_seq 2），客户端直到 04:19 之后才连上。这段窗口里发生了两次完整的韩服玩家搜索：

**搜索 #1（04:18:28.878 → 04:18:37.484，墙钟 ≈ 8.6 秒）**

| 阶段 | 耗时 | 起止 |
|---|---|---|
| `riot_overview_cost` | **5230ms** | 04:18:28.878 → 34.108 |
| `opgg_current_game_cost` | **2664ms** | 04:18:34.154 → 36.820（**等总览返回后才开始**） |
| `opgg_season_summary_cost` | **3330ms**（page 2263 + summary 1067） | 04:18:34.15 → 37.484 |

**搜索 #2（04:20:28.33 → 04:20:33.83，墙钟 ≈ 5.5 秒）**：3586 + 1499 + 1879，形状完全一样。

13 次 `riot_overview_cost` 全量分布：1009 / 1070 / 1320 / 1321 / 1325 / 1852 / 2606 / 3079 / 3086 / 3339 / 3586 / 4009 / 5230 / 6078 ms。

### 1.2 用户机器上的上游真实延迟（从 `champion_upstream` 聚合，545 条）

| host | n | p50 | p90 | max | 失败 |
|---|---|---|---|---|---|
| ddragon.leagueoflegends.com | 91 | **433ms** | 1011ms | 1364ms | 0 |
| lol-api-champion.op.gg | 53 | 345ms | 1257ms | 1557ms | 0 |
| op.gg | 10 | 481ms | 1072ms | 1072ms | 0 |
| raw.communitydragon.org | 323 | 0（多为内存命中） | 1ms | **6997ms** | **3 次 status=0** |
| mlol.qt.qq.com | 34 | 33ms | 432ms | 656ms | 0 |

**⚠️ 我原本猜「ddragon 在国内慢，所以装备目录超时」——这条被用户自己的日志证伪了：91 次全部 200，p50 才 433ms。** 记下来，别再往这个方向修。真正不稳的是 CommunityDragon（3 次失败 + 一次 `arena_item_catalog_failed errorKind=timeout source=communitydragon`）。

### 1.3 沙箱实测：装备图标通道（这是 A 组的直接证据）

同一张图连续请求三次：

```
run1 http=200 t=0.727s
run2 http=200 t=0.179s
run3 http=200 t=0.179s
```

**三次都真打了 ddragon**（0.179s 是 TCP keepalive 省下的握手，不是缓存命中）。

100 张不同装备图标、6 并发（模拟浏览器同源并发上限）：

```
conc=6 n=100 墙钟=4.92s 失败=0 p50=277ms p90=377ms max=781ms
```

沙箱 ddragon p50 是 277ms，用户机器是 433ms，缩放系数 1.56 → **同样 100 张图在用户机器上约 7.7 秒**。一页 20 场战绩有 140 个装备格（去重后通常 50~90 个唯一 ID），**这就是「装备直接出不来」的全部真相：它不是失败，是一张一张现打、排在 6 条浏览器连接后面慢慢挤出来。**

---

## A 组（P0）装备图标通道零缓存、零单飞 —— 「装备直接出不来」的根因

### A-1 根因：`championCachePolicy` 对所有图片直接返回「不缓存」

`champion_cache.go:428-431`：

```go
func championCachePolicy(host, requestPath, accept string) (time.Duration, time.Duration, bool) {
	if accept != "application/json" && accept != "text/html,application/xhtml+xml" {
		return 0, 0, false          // ← 所有图片：ttl=0 ⇒ 完全绕过缓存层
	}
```

`champions.go:548-556` 里 `ttl <= 0` 就直接 `loader(ctx)` 真打网络，**既不查内存、不落磁盘，也不做并发合并（singleflight）**。

而**同一个进程里的另一条图标通道是有缓存的**——`/api/image`（英雄头像 / 召唤师技能 / 符文）在未连接客户端时走 `main.go:893-897 → serveCommunityDragonImage`，里面用的是 `a.loadAsset(...)`（`asset_cache.go:23-63`），**有内存缓存 + 有 singleflight**。

**这就是为什么用户看到的是「头像出来了、装备格是空的」**：

| 图标种类 | 未连客户端时的 URL | 缓存 | 单飞 |
|---|---|---|---|
| 英雄头像 / 召唤师技能 / 符文 | `/api/image` → CommunityDragon | ✅ `a.assetCache` | ✅ |
| **装备**（目录 iconPath 带 `ddragon:` 前缀，`web/gameplay.js:5417`） | `/api/champion-asset?source=ddragon` | ❌ | ❌ |
| 英雄页的出装/海克斯图标（`web/champions.js:202`） | `/api/champion-asset` | ❌ | ❌ |

连着客户端时装备 iconPath 是 LCU 路径 → 走 `/api/image` → `loadAsset` 有缓存且本地毫秒级，所以**一开客户端问题就消失**，和用户描述完全对上。

### A-2 要做的改动

1. **`handleChampionAsset`（`champions.go:810-850`）的三个 `provider.fetch(...)` 调用点全部改为经 `a.loadAsset(ctx, "champion-asset:"+source+":"+path, championImageMax, negativeTTL, loader)`**，拿到内存缓存 + singleflight。
   - 注意：`champions.go:838` 的 `loadChampionAssetFromClient` 分支本来就用 `loadAsset`，**不要动它**；要加的是它下面那条「客户端不可用 ⇒ 回源」的分支，以及 `communitydragon` 的候选循环（`champions.go:828-834`）和 `championAssetFallback` 重试（`champions.go:845`）。
   - `negativeTTL` 建议 60s：404 的图标（例如只有 `_small` 的那 60 个海克斯）不该每次重打。
2. **磁盘持久化**：`championCachePolicy` 增加一条「图片：ttl 7 天 / stale 30 天 / persistDisk=true」的分支，key 用 host+path。装备图标是按补丁号定死路径的不可变资源（`/cdn/16.18.1/img/item/3006.png`），落盘后**跨进程重启也秒出**。
   - ⚠️ 必须同时给磁盘缓存加**总量上限与淘汰**（几百张图 × 6KB 不大，但海克斯大图和皮肤图会撑大），否则会变成第二个 R24 的 159MB 事故。
3. **启动预热**：应用启动后（或首次打开任一玩家页时）后台按「当前补丁 + OP.GG 排位出场率前 ~120 件装备」批量预热图标到磁盘缓存，并发 6~8。预热必须是 best-effort、可被用户操作抢占、失败不报错。
   - 参考 R83 的教训：**预热的对象必须是「最终真正被请求的那份资源」**，否则不传递。这里就是 `/cdn/{patch}/img/item/{id}.png` 本身。
4. **前端**：`ensureItems()`（`web/gameplay.js:3190-3202`）失败时**没有任何退避与重试**（对比 `ensureSummonerSpells` 有 30s 退避重试）。补一个同款退避重试，避免一次失败后要等下一次整页重绘才补上。

### A-3 验收判据（每条都要能被变异杀掉）

- A-3-1 无客户端环境下，对**同一张**装备图标连续请求 3 次，第 2、3 次必须是缓存命中（服务端耗时 < 5ms，且 `champion_upstream` 或新埋点显示 cache=memory/disk）。**变异**：把 `championCachePolicy` 的图片分支改回 `return 0,0,false`，该测试必须转红。
- A-3-2 无客户端环境下，100 张不同装备图标、6 并发，**第二轮**（进程不重启）墙钟 < 0.5s（当前 4.92s）。
- A-3-3 进程重启后再跑第二轮，墙钟 < 1.5s（验证磁盘持久化生效）。**变异**：把 persistDisk 改成 false，该测试必须转红。
- A-3-4 同一张图标 20 个并发请求，上游只被打 1 次（singleflight）。**变异**：删掉 flight 合并，必须转红。
- A-3-5 磁盘缓存目录在连续请求 2000 张不同图后不超过设定上限。

---

## B 组（P0）韩服总览的三段串行瀑布 —— 「很慢」里可以立刻砍掉 35~40% 的那部分

### B-1 根因

`web/gameplay.js:1694-1700`：

```js
function scheduleOverviewCurrentGame(tab) {
    ...
    void loadOverviewCurrentGame(tab);
    void loadOPGGSeasonSummary(tab);
```

它在 `renderOverview` 末尾（`web/gameplay.js:1548`）被调用，也就是**必须等 `/api/gameplay/overview` 整个返回并渲染完**。而这两个请求的入参都是 `tab.data.player.playerRef`（`web/gameplay.js:807`、`web/gameplay.js:1664`），这个 ref 是**总览响应产出的**，所以结构上就无法提前。

但后端两个 handler 需要的信息其实只有 gameName / tagLine / region：`opgg_season_summary.go:277-290` 只是把 playerRef 解析回 `ref.Region == KR` 就去抓 OP.GG 页面；`overview_current_game.go` 同理。**这三件事从第一秒起就可以并行。**

### B-2 要做的改动

1. `/api/gameplay/current-game` 与 `/api/gameplay/season-summary` 的请求体，除 `playerRef` 外**增加接受 `{gameName, tagLine, region}`**（与 `/api/gameplay/overview` 的 POST 入参同形），保持 playerRef 优先、两者都缺才 400。
2. 前端在**发起韩服总览请求的同一时刻**就并行发出这两个请求（搜索框 / 职业选手页 / 英雄榜单三个入口都要覆盖），而不是等渲染完再发。总览返回后如果 playerRef 与预期不符（改名、纠错命中 OP.GG 自动补全）再按现逻辑重发一次。
3. 保持现有的 30 秒轮询与 `opggSeasonAttemptRef` 去重不变，避免重复打 OP.GG。

### B-3 预期收益（用 1.1 的实测数据算）

| | 现状墙钟 | 并行后 = max(三段) | 节省 |
|---|---|---|---|
| 搜索 #1 | 8.6s | 5.23s | **-3.4s（-39%）** |
| 搜索 #2 | 5.5s | 3.59s | **-1.9s（-35%）** |

### B-4 验收判据

- B-4-1 端到端（jsdom 或真 Chromium）：打开一个韩服页签，三个请求的**发起时间戳**差值 < 300ms；当前实现下 current-game 的发起时间比 overview 的完成时间晚 < 100ms（即等待），改后必须不再等待。**变异**：把并行发起改回渲染后发起，必须转红。
- B-4-2 后端：只传 `{gameName, tagLine, region:"kr"}`（不传 playerRef）时两个端点返回 200；两者都不传返回 400。
- B-4-3 不能引入重复请求：一次打开页签，`opgg_season_summary_cost` 与 `opgg_current_game_cost` 各只出现 1 次。

---

## C 组（P1）Riot 总览内部：并发 4、无首屏分段、match 详情只存内存

### C-1 三个可改点

1. **并发写死 4**（`riot_api.go:1072` `semaphore := make(chan struct{}, 4)`）。20 场 ⇒ 5 波。本地限速是 15/s（`riot_api.go:259-262`），**4 这个数字远低于自身预算**。
   - 改法：并发数做成可配置，personal key 下取 8（20 场 ⇒ 3 波，预计省 30~40% 的 detail 阶段耗时），升级 production key 后取 15~20（1~2 波）。
   - ⚠️ 提高并发后**必须同步确认 `p.wait` 的 15/s 短窗口不会变成新的排队点**，否则只是把等待从 semaphore 挪到 limiter，净收益为 0。这一条要用埋点验证，不能只看代码。
2. **match 详情缓存只在进程内存**（`riot_api.go:703-753`，LRU 上限 `riotMatchCacheMax`，无磁盘）。match-v5 的对局 JSON 是**永不变化**的不可变资源，理应落盘。落盘后「重开应用再查同一个韩服玩家」可以从 5 秒降到 1 秒内（只剩 ids + ranks + mastery 几个请求）。
   - 同样需要总量上限与淘汰策略。
3. **首屏不该等 20 场全到齐**。现在是全部 goroutine `wait.Wait()` 之后才 serialize 返回。可以改成：identity + ranks + mastery + 前 5 场先返回并渲染，其余 15 场用第二次请求或 SSE 补齐（仓库已有 `live_updates.go` 的 SSE 通道可复用）。
   - 这一条改动面最大，**建议排在 A/B 组之后单独一轮**，不要和 A/B 混在同一次提交里，否则出问题无法二分。

### C-2 关于 production key

去申请。理由不是「现在被限速了」（0.2 节已证明没有），而是：① C-1-1 的并发才能拉满；② 多个用户同时用同一个内嵌 key 共享配额（见 `deep-legends-riot-key-and-scraping` 记录的现状），personal key 的 100/2min 在装机量上去之后一定会成为硬墙。

### C-3 验收判据

- C-3-1 20 场对局详情的并发实际达到配置值（用埋点计数同时 in-flight 的请求数峰值），**且** `riot_overview_cost.duration_ms` 在同一台机器上相对基线下降 ≥ 25%。
- C-3-2 落盘后重启进程，再查同一玩家：`riot_overview_cost` 里新增的 `matches_from_disk` 字段 = 20，`duration_ms` < 1500ms。
- C-3-3 限速护栏不能被绕过：构造 200 次连续请求，本地 15/s、90/2min 仍然成立（这条现在**没有任何测试覆盖**，要新写）。

---

## D 组（P1）玩家页的「出装推荐」区块 —— 现状是「按设计就不存在」，不是坏了

### D-1 事实

全仓库「出装推荐」只有两个产地：

1. **英雄页**（`web/champions.js:1790`，OP.GG 数据）——**不依赖客户端，日志里在无客户端窗口正常工作**（04:18:07 `opgg_item_depths_resolved champion=jinx fourth=5 fifth=5 sixth=5`）。
2. **对局页 / 英雄选择页**（`web/gameplay.js:4917`、`gameplay.js:3632`）——数据来自 `/api/gameplay/recommendations`，唯一调用点在 `web/gameplay.js:3878` 的 `ensureLiveRecommendations`，而它只在 `loadLive()` 里被调用，`loadLive()` 开头就是 `if (... || !connected()) return;`（`web/gameplay.js:3622`）。

**也就是说：玩家总览页从来就没有过「出装推荐」区块，它只活在需要客户端的对局页里。** 用户在韩服玩家页看不到它，不是回归，是功能缺口。

### D-2 建议（需用户拍板要不要做，做多大）

在韩服玩家总览页加一块**基于该玩家常用英雄 + 常用分路的 OP.GG 出装推荐**，数据源完全复用 `champions_structured.go` 那套（`opgg_item_depths_resolved` 那条链路），零客户端依赖。

范围建议控制在：总览页玩家最近 20 场里出场最多的 1 个英雄 + 它的主分路，给一组「出门装 / 鞋子 / 核心三件 / 四五六件」，点击可跳转到英雄页看完整版。**不要**在玩家页重建整个英雄页的出装面板。

### D-3 验收判据

- D-3-1 无客户端环境下打开韩服玩家页，该区块有数据且 `source` 标注 OP.GG。
- D-3-2 该区块的任何一个请求都不得触达 `127.0.0.1` 的 LCU（用 host 白名单断言）。
- D-3-3 玩家最近 20 场为空时该区块整块不渲染，不留空壳。

---

## E 组（★P0，见 0.4 —— 已从 P2 提级，必须先于或随 A 组一起发布）埋点缺口 —— 这轮诊断被卡住的地方

按 R86 的教训「加埋点必须端到端验证能落盘」，下面每条都要有一次真实落盘样本才算完成。

- **E-1 `overview_phases_ms` 在韩服路径上是坏的。** 实际样本：
  ```json
  {"champion_names":0,"detailed_matches":0,"identity":0,"mastery":0,"queue_labels":0,
   "ranks":0,"recent_players":0,"recent_ranked":0,"season_snapshot":0,"serialize":5231,"total":5231}
  ```
  **所有阶段都是 0，5231ms 全记在 `serialize` 上。** 这套分阶段埋点只覆盖了国服/SGP 路径，韩服路径的 5 秒内部完全是黑盒。要让 `riot_api.go:965-1100` 这段也填这些字段（至少：account / summoner / matchIDs / ranks / mastery / details / opgg-historical / serialize）。
- **E-2 图标通道零埋点。** `reportChampionUpstream`（`champions.go:574-576`）第一行就是 `if ... accept != "application/json" && accept != "text/html..." { return }`，**所有图片请求都不会产生任何日志**。A 组改完后要加一条按窗口聚合的 `asset_fetch` 事件（host / 命中率 / p50 / p90 / 失败数），否则 A 组的效果无法在真机上验证。
- **E-3 本地限速排队不可见。** `riot_api.go:295-312` 只有在「等待时间 ≥ 剩余 deadline」时才发 `riot_local_rate_limited`；正常的 sleep 排队完全静默。加一个按窗口聚合的排队总时长字段到 `riot_overview_cost`。
- **E-4（0.4 节三个候选的裁判，P0，必须最先做）`/api/gameplay/items` / `perks` / `summoner-spells` 三个目录端点零埋点，且前端 `ensureItems()` 失败静默吞掉**，无法区分候选①「目录端点失败」、候选②「目录成功但查不到具体 ID」、候选③「后端 itemIds 字段本身是空的」。要补齐两侧：
  - 后端：一条 `catalog_load{endpoint, source: lcu|ddragon, items, duration_ms, outcome, error_kind}`，`handleGameplayItems`（`gameplay.go:8122-8151`）和两个 fallback（`gameplay.go:8230-8254`）成功/失败都要报。
  - 后端：`convertRiotMatchInfo`（`riot_api.go:834`）产出的**每个 participant** 的 `itemIds` 长度与非零个数计入一条按 match 聚合的诊断（例如复用或扩展 `overview_phases_ms`，加 `items_present_count` / `items_zero_count` 两个字段），直接证伪或证实候选③。
  - 前端：`ensureItems()`（`web/gameplay.js:3190-3202`）catch 分支补一条 `client_diagnostic` 上报（复用 `recordItemSetClientDiagnostic` 现成机制），把 HTTP 状态码/超时/网络错误上报出来，直接证伪或证实候选①。
  - 前端：`itemIconFigure`（`web/gameplay.js:5446-5449`）在「目录已加载但查不到该 ID」这条分支上报一次去重后的 `item_id_not_in_catalog{id}` 事件（限流，避免刷屏），直接证伪或证实候选②。

---

## F 组（P2）顺带发现，本轮不修但要记账

- **F-1 `specialist_runes_client_skip reason=no-top-players` 共 73 条**，覆盖 champion_id 6/8/18/19/24/28/29/36/41/50/57/64/96/102/105/114/120/126/134/136/141/147/150/157/234/238/246/268/498/517/523/777/804/895。绝活哥符文在**队列 1750/2400/3140** 下几乎必然拿不到高手名单。`load_top_players_shape` 只在 ranked 队列下有 `parsed:5` 的成功样本。怀疑是 OP.GG 榜单只有排位数据、而这些娱乐队列没有对应榜单——如果确认如此，应该**在 UI 上直接说明「该模式无绝活哥数据」而不是静默跳过**。需要单独一轮确认，别在这轮顺手改。
- **F-2 `pro_directory_cost stage=ladder` 实测 8456ms / 12999ms**，是职业选手页最慢的一段，且它在 `stage=supplements` 之后串行执行。B 组的并行化思路同样适用。
- **F-3 CommunityDragon 在用户网络下不稳**：323 次里 3 次 status=0、max 6997ms，另有一次 `arena_item_catalog_failed errorKind=timeout`。A 组落盘缓存做完后这个风险会显著下降，但 `/api/image` 的 CommunityDragon 回退目前也**只有内存缓存、进程重启即丢**，可以一并落盘。
- **F-4 `overview_art_source catalog_count=0 poster_available=false skin_id=76000`**：韩服玩家的生涯背景图拿不到。未确认是否是预期行为。

---

## 附：验收方式（沿用 R86 标准）

1. **不接受「测试通过」作为完成证据**，每条判据都要做一次**独立变异**：把对应修复点改坏，该测试必须转红。基线必须先全绿再开始变异（R78 的教训：忘了传 env 导致 13 条结论全作废）。
2. **图标/耗时类判据必须在真 Chromium 或真二进制上测，jsdom 不算数**。A 组的对照数字用本文件 1.3 节那套方法复测（同一台机、同一网络、进程重启前后各一轮）。
3. **Windows 专属代码（`*_windows.go`）在 Linux 上不编译**，本轮涉及的都不是，但如果实现时引入了，验收时要单独说明护栏在哪（R82 的教训）。
4. **执行顺序：E-4 必须先于或随 A 组一起交付，不能拖到最后当「顺手补」**（见 0.4）。改完后请用户**关掉客户端**、搜一次韩服玩家、**在页面停留 60 秒以上再导出诊断日志**（不要一打开就导，A/B 组的耗时改善和 E-4 的候选判定都需要这 60 秒窗口内的完整数据），同时截一张 60 秒后的战绩卡截图。重点看：`riot_overview_cost.duration_ms`、新增的 `overview_phases_ms` 各段、新增的 `asset_fetch` 命中率、`catalog_load` 的 outcome、`items_present_count`/`items_zero_count`、三个请求的发起时间差。如果装备格仍然空白，直接看这几条新埋点定位到 0.4 节的哪一个候选，不要重新走一遍本轮的排查过程。

---

## 我可能错的地方（请优先证伪）

1. ~~A 组的因果链最后一环是推断~~ **已被 0.4 节的真机截图+日志证伪为「不充分」，不是「可能错」**：A 组（图标通道无缓存）解释的是「慢」，不能单独解释截图里「装备格永久空白、同通道的技能/符文图标却正常」这个现象。**A 组仍然要做**（它本身是真实测出来的问题），但**不能把它当成「装备出不来」的完整答案**，E-4 的三条新埋点是唯一能在下一轮日志里裁定候选①②③的办法，执行方不要在 E-4 数据出来前自行选一个候选去修。
2. **D 组是我对「玩家页的出装推荐区块」的理解**：如果用户指的其实是英雄页的出装（工具›英雄），那它在无客户端时是正常工作的（日志已证），需要重新描述症状。
3. C-1-1 提高并发的收益我只做了算术估计，没有实测。实现前建议先只改并发数跑一次 A/B，确认收益是否被 `p.wait` 吃掉；且这条现在**必须等 production key 落地**才能真正拉高倍数（见 0.2 节补充）。
