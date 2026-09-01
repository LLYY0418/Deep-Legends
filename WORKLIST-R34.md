# WORKLIST-R34（给 GPT 执行，独立于 WORKLIST-R33）

> 本文件是从 WORKLIST-R33.md 里拆出来的，原本是 R33 的 G/H/I 组。拆分原因：
> R33 前面几组（0/A/B/C/D/E/F）GPT 已经开始动手改了，这三组（G/H/I）是当天
> 后续追加的研究性内容（对比 LeagueAkari 战绩链路 → 穷尽扫描它的工程模式 →
> 挖到腾讯官方 qq101 接口），继续堆进同一份正在被执行的工单容易和已经在改的
> 内容混在一起、增加范围蔓延的风险，所以单独建一份，**执行顺序、变异测试要求、
> 交付标准跟 WORKLIST-R33.md 完全一样**，只是内容拆开管理。
>
> **用户已拍板**：I-5 里 `_partner`（组合搭档）/`_segment`（段位胜率曲线）/
> `_duration`（时长胜率）这三项新功能**明确不做**，本文件保留分析过程供以后查阅，
> 但不要当成待办事项去实现。真正要做的新东西只有 I-5-1（`_newlane` 分路占比）
> 和 F 组已经拍板要做的排位数据源优先级切换（F 组仍在 WORKLIST-R33.md 里）。

---

## G 组（0828 新增）：战绩加载链路与 LeagueAkari 逐项对比 —— 差距不在数据源，在响应组合方式

用户追问"他的战绩加载也很快，仔细对比一下战绩相关的数据获取方式，看看谁的方案更好"。
已经把 LeagueAkari 战绩相关的源码全部读完（`match-history.ts` / `match-history-loader.ts` /
`source-selection.ts` / `ongoing-game/state.ts`），逐项结论如下。**先说总结论：
拉战绩用的接口跟我们一模一样，他们不快在数据源，快在响应是流式拼出来的、我们是一次性阻塞返回的。**

### G-1 完全相同、没有可借鉴空间的部分（先排除掉，别浪费时间）

| 环节 | LeagueAkari | 我们 | 结论 |
|---|---|---|---|
| 战绩列表 | SGP `/match-history-query/v1/products/lol/player/{puuid}/SUMMARY` | 同一个端点（`sgp_api.go:622`） | **完全一致** |
| 单场详情/时间线 | SGP `.../{server}_{gameId}/DETAILS`，点开才拉 | 同一个端点（`sgp_api.go:703`），`match_timeline.go:387` 独立端点按需拉 | **完全一致，都是懒加载** |
| 十人名单 | SUMMARY 一次返回，不用补请求 | 同上（`gameplay.go:1141` 一次 `matchHistoryOn` 就够） | **完全一致** |

值得注意的是：**在 LCU 数据源下他们反而比我们慢**——LCU 的战绩列表不含完整参与者，
他们必须对每一场再打一次 `getGame`（`completeLcuGame`，并发 10）才能凑齐十人。
我们走 SGP 一次就够，这一点上我们的选择是对的，不要照着 LCU 路径改。

### G-2 ★他们明显更好的地方：响应是"多条独立小流"，我们是"一坨大阻塞"

**这是唯一真正拉开体感差距的地方。**

他们的做法（`match-history.ts:406` 一带）：SUMMARY 回来 → **立刻 `page.value = {...}` 渲染出列表** →
紧接着 `loadGameDetails(games)` **不 await**，详情通过并发队列慢慢填进 `page.details`，
界面逐格补上；排位数据（`ranked-stats.ts`）是**另一个完全独立的 composable**，有自己的
loading 态，跟战绩列表互不阻塞；回放元数据又是第三条独立流。**任何一条慢/失败都不会
拖住其余部分显示。**

我们的做法（`gameplay.go:838-877`）：`/api/gameplay/overview` 是**一个巨型阻塞响应**，
内部串行执行：`loadDetailedMatches`（战绩） → `loadSeasonChampionStats`（**赛季扫描，
前台 2 页 × 每页约 2.8MB**） → `loadRanksWithFallback`（排位） → 韩服还要加
`opggHistoricalRanks`（7 秒超时）。**全部跑完才吐第一个字节，前端在此之前只有骨架屏。**

我们自己的日志就是铁证：
```
overview_load_cost duration_ms:1327 sgp_bytes:8164121 sgp_requests:4   ← 首次进入，8MB / 4 次往返
overview_load_cost duration_ms:342  sgp_bytes:2667547 sgp_cache_hits:2 ← 命中缓存后
```
**首屏要等 4 次 SGP 往返、8MB 传输**，而 LeagueAkari 只等 1 次 SUMMARY 就开始画了。
这跟"谁的接口快"没关系，是我们把三件本来可以并行的事串成了一条链。

### G-3 他们另外三个值得抄的小设计

1. **详情有明确预算，不是"有多少拉多少"**：`ongoing-game/state.ts:52`
   `gameDetailsLoadCount: 20` —— 战绩加载 50 场，但只对**前 20 场**拉时间线。
   我们目前是点开哪场拉哪场（也没问题），但如果以后做"批量预加载详情"，
   要照这个思路设一个显式上限，不要按列表长度线性放大。
2. **并发数按任务类型分开、而且可调**：他们对不同任务用独立队列
   （详情 5 / 回放元数据 5 / LCU 补全 10），对局内玩家查询更是保守到 `concurrency: 2`
   并**暴露成用户设置项**。我们目前是 `rank_insights.go:274` 写死 `semaphore = 4`，
   一个信号量管所有平均段位查询。**建议至少把这个数提成常量并加注释说明取值依据**，
   现在这个 4 是个没有来源的魔数。
3. **数据源决策是一个"可测试的值"，不是散落的 if/else**：`source-selection.ts` 的
   `toLoadStatus()` 返回结构化决策（`load`/`wait`/`unavailable` + `source` + `reason`），
   调用方 `logDataSourceDecision(decision)` 打日志。**我们的等价逻辑散在
   `loadRanksWithFallback`/`loadDetailedMatches` 里各写一遍 if/else，既不能单测也不好排查。**
   F 组要动数据源优先级，正好顺手把这块抽成一个 `resolveDataSource()` 纯函数，
   返回带 reason 的结构体，既方便写变异测试，也让"为什么这次走了 SGP"能直接读日志。

### G-4 我们比他们好的地方（别改坏了）

- **赛季聚合统计（能力表现/近 20 场/英雄统计）他们压根没有这个功能**——他们"快"有一部分
  是因为做得少。这个功能不能砍，正确解法是 G-2 说的"别阻塞首屏"，而不是向他们看齐删功能。
- **赛季缓存落盘**（`storage.go` 的 `season-stats/`）比他们纯内存 LRU 强，重启不丢。
- **平均段位查询已有 single-flight + 10 分钟 TTL 缓存**（`rank_insights.go:118-135`），
  这层他们没有对应实现（他们对局内只查 10 个人，用不上）。

### G-5 改法（按性价比排序，第 1 条最值钱）

1. **★把 `/api/gameplay/overview` 拆成"主体 + 增量"两段**（性价比最高，直接解决体感）。
   最小改动方案：**先把 `loadSeasonChampionStats` 从主响应里摘出去**——
   主响应只返回 战绩列表 + 排位 + 玩家信息（这两样已经很快），赛季统计相关字段
   （`SeasonChampionStats`/`SeasonOverall`/`SeasonStatsProgress`/能力表现/近 20 场）
   全部改成初始为空 + `collecting: true`，由**已经做好的 A-3 那条 `season-progress`
   live-updates 推送**在后台扫完后推给前端重绘。**A 组的角标和推送链路这轮已经落地了，
   这一步等于把它从"回补进度提示"升级成"首屏就走这条路"，增量很小。**
   预期效果：首屏从"等 4 次 SGP 往返 + 8MB"降到"等 1~2 次往返 + 2.6MB"。
2. **韩服 `opggHistoricalRanks` 也从主响应摘出去**（`gameplay.go:876`，现在挂着 7 秒超时
   串在链路里）——同理改成独立端点或走推送，别让一个第三方站点的超时拖垮整个总览。
3. **把数据源决策抽成纯函数**（见 G-3 第 3 点），跟 F 组一起做。
4. `rank_insights.go:274` 的并发数 4 提成有名字的常量并写明依据。

**变异测试要求**：G-5 第 1 条改完后，补一个测试断言"赛季统计不可用/未扫完时，
overview 主响应仍然正常返回战绩与排位且不为空"（把赛季扫描改成必然超时的桩，主响应必须照常返回，测试才算真护栏）；再补一个断言"主响应里 SeasonStatsProgress
在未扫完时 collecting 为 true 且 SeasonChampionStats 为空"，防止有人为了让测试变绿
又把同步扫描塞回去。

---

## H 组（0828 新增）：LeagueAkari 工程模式穷尽扫描 —— 只挑对我们有真实收益的

G 组只对比了战绩/排位两条链路。这次把 LeagueAkari 跟"查数据"相关的部分**全部**扫了一遍
（`sgp/` shard 的令牌与服务器管理、`ongoing-game/` 两个 loader、缓存体系、错误与限流、
请求取消与竞态、`data-adapter/`、并发抽象），逐条对照我们自己踩过的坑筛选。

**筛选原则**：只写"能对上我们某个具体历史故障或当前缺陷"的，其余一概不写。
下面每条都标了它对应我们哪个已知问题。**分三档，P0 那几条建议本轮就做，P2 可以放到以后。**

### H-P0-1 缓存键与任务 ID 必须包含数据源前缀 `${source}:${id}`

LeagueAkari 全项目 10 处一致：`_gameDetailsLruMap.get(\`sgp:${gameId}\`)` /
`\`lcu:${gameId}\``，任务 ID 也是 `sgp-game-details:${gameId}` / `lcu-game-details:${gameId}`
（`match-history-loader.ts:191,220,232,260,296,327,339,370,495,518`）。理由很硬：**同一个
gameId，SGP 和 LCU 返回的是结构完全不同的两份数据**，不分键就会串。

**对应我们的问题**：这正是 [[deep-legends-r33-season-snapshot-and-stale-log]] 第 8 条
"传输链路按文件名缓存、重命名即可绕开"的同类错误——**缓存键没有包含所有影响结果的维度**。
F 组马上要把排位改成 LCU 优先、SGP 兜底，**两个源的数据会同时存在，这条必须先做，
否则 F 组一上线就会出现"缓存里存的是 SGP 的数据、当成 LCU 的用"这类脏读**。

**改法**：全仓库审一遍缓存键——`sgp_api.go` 的 `historyCache` 键、`rank_insights.go:139`
的 `cacheKey := serverID + "|" + playerRef`（**这个就缺 source 维度**）、
`season-stats` 落盘文件名。凡是"同一个 key 可能被不同数据源写入"的，都补上源前缀。

### H-P0-2 区分「请求被取消」和「请求失败」，取消不置错误态

LeagueAkari 两个 loader 里出现 9 次、完全一致（`match-history-loader.ts:460-468`）：
```ts
} catch (error) {
  if (isAbortError(error)) { logger.info('...aborted', puuid); return }   // 静默返回
  logger.warn('Error loading...', error, puuid)
  state.setMatchHistoryLoadingState(puuid, 'error')                       // 只有真失败才 error
}
```
更细的一层在 `champion-data/service-controller.ts:60,79`：`try` 之前和 `catch` 之后**各调
一次 `signal.throwIfAborted()`**——取消要盖过错误分类，否则用户切个页面会看到一个假的
"请求失败"。

**对应我们的问题**：[[deep-legends-r31-live-build-sort-and-lowsample]] 那轮反复出现的
"切筛选后满屏错误态"、以及我们自己 `web/gameplay.js` 里已经有的
`if (error.name !== "RequestCancelled")` 判断——**前端做了，Go 后端没有对应概念**。
`context.Canceled` 目前在多处会被当成普通错误记进 capability 和诊断日志，污染排查。

**改法**：Go 侧加一个 `isCancellation(err) bool`（判 `errors.Is(err, context.Canceled)` 与
`context.DeadlineExceeded` 要分开——超时是真失败，取消不是），在所有
`recordDiagnostic` + `capabilityFailed` 的位置前面过一道，取消就静默返回不记错误。

### H-P0-3 「启动全量 + 事件增量 + 断连清空」三件套，字段级同步

`league-client/lc-state/index.ts:113-136` 每个状态片都是同一套五步模板：
声明要同步的**具体字段** → 全量拉取函数 → 注册到启动任务表 → **断连时显式清空** →
WebSocket 订阅增量更新。

**对应我们的问题**：E-5 那条「收藏与奖池已更新」横幅语义错乱——根因就是我们的 SSE
**任意事件都触发全量 `refreshStatus()` + 无条件 `renderNotice()`**，既没有字段粒度，
也没有"哪个事件该更新哪块状态"的映射。这轮 E-5 只改了文案（治标）。

**改法**：给 live-updates 的事件定义明确的 `type` → 影响哪些状态片的映射表，
前端按 type 决定重绘哪一块，而不是收到任何事件就整页刷。**断连时要显式清空对应状态**
（我们现在断连后界面还留着上次的数据，容易让人以为是实时的）。

### H-P1-1 三层去重闸门（查询条件比对 → 缓存 → 队列内 single-flight）

`match-history-loader.ts:400-424`（**查询参数逐字段比对，且把 `source` 本身纳入判据**）
→ `:191-198`（LRU 命中直接返回）→ `:200-203`（`queueKeeper.hasTask(id)` 已在队列里就跳过）。
三道缺一不可。

**对应我们的问题**：[[deep-legends-r25-overview-perf-and-ranked-samples]] 的"首次总览
24s/159MB"、R33 的"胜率刷新几次才稳定"，本质都是重复/无效请求。我们目前有
single-flight（`rankScoreCache.beginFlight`）和 TTL 缓存，**但缺第一道"查询条件没变就不查"**。

**改法**：给 overview / 赛季扫描加一层"上次查询参数快照"，参数逐字段相同就直接复用
上次结果不发请求（注意要把数据源、队列筛选、begIndex/count 全部纳入比对）。

### H-P1-2 明确拒绝跨数据源自动降级，并返回完整的 attempts 决策留痕

`champion-data/service-controller.ts:57-59` 的注释信息量最大，原文翻译：
> 换个数据源 = 换个数据集，不是重试目标。地区范围、甚至同名模式的统计口径都可能含义不同，
> 所以重试只应发生在 source loader / HTTP 客户端层内部。跨源替换需要一条明确的兼容性规则。

返回值带 `attempts: [{source, outcome, message}]`，`outcome` 四态
（`success`/`failed`/`disabled`/`mode-unsupported`），外加 `fallbackReason` 三态。
前端因此能准确告诉用户"为什么没数据"，而不是笼统的"加载失败"。

**对应我们的问题**：[[deep-legends-acceptance-0825-r25-r26]] 记录过的
"lolalytics 失败回退 op.gg 导致口径不一致、用户原始投诉在兜底路径原样复发"——
**我们当时就是无声跨源降级，结果两个源的去重规则不同，bug 换个路径又出来了**。
另外 C 组绝活哥"一直修不好"的体感，也是因为空态文案分不清五种原因。

**改法**：F 组要改排位数据源优先级，正好一起做——`resolveDataSource()` 返回结构化决策
（G-5 第 3 点已提），失败时返回 attempts 数组而不是 nil，前端据此显示具体原因。
**并且把这条注释的精神写进代码注释**：跨源替换必须是显式的兼容性决策，不是默认兜底。

### H-P1-3 Feature gate：带默认值的远程开关

`feature-gating/index.ts:20-28`，`isEnabled(key, defaultValue)` **每次调用都带默认值**——
远程配置服务挂了就用默认值，不影响功能。维度含 platform / version / sgpServerId。
用法（`additional-info-controller.ts:169-171`）：某个上游接口出问题时，**远程一关就行，
不用发新版**。

**对应我们的问题**：我们是"EXE 发出去就收不回"的桌面应用，而且**已经被上游打脸过至少四次**：
lolalytics 国内 403 全面失效、hexdata 结构抖动触发熔断锁死 24 小时、op.gg RSC 解析器
因上游改版失效、CDragon 图标路径变更。**每一次都只能等用户重新下载新包**。

**改法**：这条实现成本不低（要有配置分发通道），但价值也最大。**建议先只做最小版本**：
一个内置默认值的开关表 + 一个可选的远程 JSON（拉不到就全用默认值），先把
lolalytics/hexdata/op.gg 这几个已知易碎的上游各挂一个开关。分发通道可以先用 GitHub raw
或任何静态托管，不用像 LeagueAkari 那样借 npm registry。**这条如果本轮做不完，
单独开一个工单，不要硬塞。**

### H-P2-1 并发调度抽象（QueueKeeper 的 Go 等价物）

`src/shared/utils/queue-keeper.ts`（169 行）用三个 Map 同时解决了 single-flight、优先级、
按标签批量取消、防泄漏四件事：`_queues`（多队列）/ `_tasks`（taskId → controller+tags）/
`_tags`（tag → Set<taskId> 倒排索引）。取消支持 AND/OR 语义
（`cancelByTags([puuid, 'match-history'], 'and')` 只杀这个玩家的战绩任务，不误伤他的段位任务）。
`add()` 强制 taskId 唯一，`finally` 里必清理。

**对应我们的问题**：我们的并发/取消散在各处——`rank_insights.go:274` 一个裸信号量、
`sgp_api.go` 的 historyCache 单飞、`season_stats.go` 的 backfill goroutine 各写各的，
**没有统一的"取消这个玩家的所有在途请求"能力**（切玩家时旧请求还在跑）。

**改法**：Go 移植成本不高（`map[string]context.CancelFunc` + 倒排索引 + 优先级 worker pool），
但这是重构，**建议等 F/G 组落地稳定后单独开工单**，本轮不做。

### H-P2-2 优先级按「数据种类」分，不按「玩家」分

`ongoing-game/context.ts:15-25` 是一张全局数值表：拿队伍名单 50 > 召唤师基本信息 30 >
战绩列表 25 > 本地标记 20 > 段位 15 > 熟练度 10 > 普通时间线 5 > 补充召唤师 -1（负数=最后）。
**没有"先查自己人"这种按玩家的优先级**，10 个玩家一视同仁；急需时走 `priority: Infinity` 旁路。
排序依据是"UI 上先出现的先查"。

**改法**：跟 H-P2-1 一起做，本轮不做。

### H-P2-3 两个互相约束的设置项用 transform + sideEffect 保证不变式

`ongoing-game/index.ts:74-105`：`gameDetailsLoadCount <= matchHistoryLoadCount` 这条不变式，
通过 `transform`（钳制自己）+ `sideEffect`（钳制对方）双向保证，不管用户从哪一边改都成立，
比在 UI 上加 max 属性可靠。非法值**静默回退到旧值**而不是抛错。

**对应我们的问题**：我们设置页那堆 select 目前没有互相约束的场景，但如果以后加
"战绩展示数量 / 详情预加载数量"这类联动项，照这个模式做。**本轮不做，记录备查。**

### H-★ 明确不要学 LeagueAkari 的地方

调研里发现它有几处**比我们差**，别照抄：

1. **限流与熔断**：全仓库**零** 429/503/Retry-After 处理，没有熔断器，没有令牌桶，
   SGP 客户端连 timeout 都没设，全靠 `concurrency: 2` 的队列上限当限流。
   **我们的 hexdata 熔断器（含 24h 锁死那次教训后的修复）比它完善得多**，保持现状。
2. **重试**：只有第三方数据源配了指数退避，自家接口都是裸 `{retries: 2}`。
3. **令牌失效自动重试**：不存在。设计上假定 LCU 会一直推有效令牌。
4. **请求版本号/竞态防护**：**全文搜索零命中**。前台靠 `isLoading` 再入锁 + `queue.clear()`
   + **Vue 组件卸载时 effect scope 自动销毁**兜底（迟到的响应写进没人读的 ref）。
   **我们是原生 JS，没有这层保护，版本号是必需的，不能照抄它的缺省**——
   我们 `web/gameplay.js` 现有的 `overviewRequestToken` 机制反而是对的，要保留。
5. **`data-adapter/analysis/`** 那 33 个文件（IQR 离群检测、变异系数、泛型聚合 DSL）
   是它独有的评分功能，跟数据获取无关，且那个 DSL 是 TS 映射类型魔法，Go 学不了也不该学。

**交付建议**：H 组 P0 三条（缓存键带源前缀 / 取消不算失败 / SSE 字段级同步）建议本轮做完，
其中 **H-P0-1 必须排在 F 组之前或同批**（F 组会让两个数据源并存）。
P1 三条按情况，H-P1-3（feature gate）如果做不完就单独开工单。P2 三条本轮不做。

**变异测试要求**：H-P0-1 补一个 fixture——同一个 gameId/playerRef 先用 SGP 写入缓存、
再用 LCU 读取，断言**不会读到 SGP 那份**；把源前缀去掉这个变异体必须让测试 failed。
H-P0-2 补一个"context 被取消"的用例，断言 capability 不是 failed 态且诊断日志里没有错误事件；
把取消判断删掉，测试必须 failed。

---

## I 组（0828 新增，★本轮最有价值的发现）：腾讯官方 101 接口 —— 出装/符文/召唤师技能的国服原生 JSON 数据源

用户问"LeagueAkari 拿 OPGG 的数据有没有可以借鉴的，我们拿装备数据和绝活哥数据一直有问题"。
查下来 **op.gg 那条链路他们跟我们打的是同一个 JSON 接口（`lol-api-champion.op.gg`），没有魔法**；
但**他们有第二个数据源是我们完全没注意到的**，而且它恰好直击我们两个老大难。

### I-1 ★ `mlol.qt.qq.com` —— 腾讯掌上英雄联盟官方接口，已沙箱实测可用

LeagueAkari 的 `src/shared/http-api-axios-helper/qq101/index.ts` 里有一整套：
```
BASE = https://mlol.qt.qq.com
版本列表    /go/database/versionlist?zone=lol&from=h5
出装        /go/battle_info/odp_proxy/lol_101strategy_build
符文        /go/battle_info/odp_proxy/lol_101strategy_runeinfo
召唤师技能  /go/battle_info/odp_proxy/lol_101strategy_skill
技能加点    /go/battle_info/odp_proxy/lol_101strategy_skill_point
对位/搭档   /go/battle_info/odp_proxy/lol_101strategy_confront / _partner
分路分布    /go/battle_info/odp_proxy/lol_101strategy_newlane
梯度榜      /go/battle_info/odp_proxy/lol_101strategy
```
公共参数：`itier`（段位）、`version_id`（**是版本名如 `16.17`，不是数字 id——传 id 会返回空**）、
`lane`（`TOP/JUNGLE/MIDDLE/BOTTOM/SUPPORT/ALL`）、`championid`。
段位映射（`qq101-protocol.ts:576`）：`all=255 / gold_plus=24 / platinum_plus=25 /
emerald_plus=26 / diamond_plus=27 / master=8 / master_plus=28 / grandmaster=9 / challenger=10`。

**沙箱实测（2026-08-28，非国内 IP 都能通，HTTP 200）**，瑞兹中单 emerald_plus / 16.17：

```
GET /go/battle_info/odp_proxy/lol_101strategy_build?itier=26&version_id=16.17&lane=MIDDLE&championid=13

starting_details "1056,2003,2003_1_67.31_44.72#2003,2003,3070_2_28.53_48.59#..."
shoes_details    "3111_1_43.28_44.32#3020_2_29.01_50.81#..."          （5 条）
core_details     "6657,3003,2522_1_59.17_55.52#..."                   （有序三件组合，5 条）
forth_details    "3157_1_25.89_61.58#3089_2_21.93_63.35#..."          ← ★第四件，5 条
fifth_details    "3089_1_26.7_65.45#3157_2_19.42_67.5#..."            ← ★第五件，5 条
sixth_details    "3157_1_31.43_63.64#3135_2_25.71_66.67#..."          ← ★第六件，5 条
dtstatdate       "20260827"                                            （数据日期，当天）
```
记录格式：`装备ID[,装备ID...]_名次_选取率_胜率`，`#` 分隔多条。响应外层是
`{"code":0,"data":{"_fieldValues":{"R18087":"<内嵌JSON字符串>"}},"result":"<同一份>"}`，
需要二次 `JSON.parse`（解析器见 `qq101-protocol.ts:84 parseInner`，字段号 build=18087 /
runes=18119 / spells=18029）。

**这直接解决我们最脆的那条链路**：我们为了拿"第四/五/六件"一直在爬 op.gg 网页的 RSC 载荷
（`champions_structured.go` 的 `parseOPGGDepthRows` + 一堆正则 + 716KB HTML），
而且 [[deep-legends-lolalytics-itemset]] 记录过 op.gg 的 JSON 接口里 `last_items`
**是扁平的单件分布、不按顺位分桶**，所以只能爬网页。**腾讯这个接口直接按顺位分好了桶。**
同一份日志里 `opgg_item_depths_failed champion:zaahen` 正是那条脆弱链路又失败了一次。

同批实测通过的还有：
- 符文 `_runeinfo`：完整符文页（基石+主系4+副系2+三个属性碎片）+ 选取率/胜率/**场次**，
  例如 `1_8230_jj_8230,8226,8210,8236,8451,8473,5005,5008,5001_21.76_42.39_637`。
- 召唤师技能 `_skill`：`12_4_45.13_48.75#14_4_47.99_14.45#...`，5 组带胜率/选取率。

### I-2 这对「绝活哥符文」意味着什么（C 组的一条新出路）

**先说事实：LeagueAkari 没有任何类似「绝活哥」的功能**（全仓库搜 one-trick/specialist/
榜单符文，零命中），所以这一块没有现成实现可抄。

但值得重新想一下我们这个功能的成本结构。现在的链路是：
**内嵌共享 Riot Personal Key → 抓韩服英雄榜单 → 逐个查 account → 查 match_ids →
逐场拉 match detail → 提取符文**，四五跳，而且那把 key 是全体用户共享、会限流、会过期
（[[deep-legends-riot-key-and-scraping]]）。C 组"一直修不好"的根本原因就是这条链太长、
任何一环断了表现都是"符文不出现"。

**qq101 的 `_runeinfo` 用一个请求就能拿到国服该英雄该段位的真实符文分布，带胜率和场次，
不需要任何 API key。** 它不等于"绝活哥"（不是特定高手的配装，是分段位聚合），
但对国服用户来说信息价值未必更低——而且可以按 `itier=10`（最强王者）或 `itier=28`（大师+）
查高分段符文，语义上已经很接近"高手都在用什么符文"。

**建议（需要跟用户确认产品取舍，不要单方面决定）**：
- 方案 A：保留绝活哥，但把 qq101 高分段符文作为**并列展示**或 key 不可用时的兜底，
  空态不再是"暂无可核验的韩服绝活哥符文"而是直接给出国服高分段数据。
- 方案 B：C 组诊断埋点拿到日志后如果确认链路无法稳定，用 qq101 高分段符文**替代**这个功能，
  彻底摆脱对内嵌 Riot key 的依赖。
- **无论选哪个，C 组的客户端诊断埋点该拿的日志还是要拿**，先弄清楚现在到底断在哪一环，
  再决定是修还是换。

### I-3 接入要求（真机验证优先，别一次性替换）

1. **先做只读探测，不改现有展示**：新增一个 qq101 客户端 + 一条诊断埋点，
   在英雄详情页加载时**并行**打一次 qq101 build/runes/skill，只记录
   `{event:"qq101_probe", champion, tier, patch, ok, forth_n, fifth_n, sixth_n, rune_pages_n, spells_n, ms, bytes}`，
   **不改任何界面**。目的是拿真机数据回答三个问题：国内网络下延迟多少、覆盖率如何
   （冷门英雄/冷门分路有没有数据）、版本号跟得上不上（`version_id` 要用 `_getLatestPatch` 动态取，
   不能写死）。
2. **探测通过后，第四/五/六件优先切 qq101**，op.gg RSC 爬取降级为兜底（**不要直接删**，
   [[deep-legends-vs-leagueakari-match-history]] H-P1-2 那条"跨源替换必须是显式兼容性决策"
   在这里同样适用：两个源的段位口径、地区口径都不同，切换要有明确规则并在 UI 上标明数据来源）。
3. **数据源必须可远程关闭**：LeagueAkari 对两个源各挂了一个 feature gate
   （`CHAMPION_DATA_OPGG_FEATURE_GATE` / `CHAMPION_DATA_QQ101_FEATURE_GATE`，
   `champion-data/index.ts:145-152`），上游一出问题远程一关就行。**这条跟 H-P1-3 是同一件事，
   qq101 接入正好是它的第一个落地场景**——新加的上游更需要这个保险。
4. **HTTP 客户端参数照抄他们对第三方源的配置**（`champion-data/index.ts:129-137`）：
   `timeout: 8000` + `retries: 1` + `shouldResetTimeout` + **指数退避**。
   注意他们对自家/官方接口用的是裸 `retries:2` 不退避，**只有第三方源才配退避**——
   因为这是用户主动点击触发的，宁可快速失败也不让人干等。
5. **每个子请求独立 `Promise.allSettled` + 部分失败只记日志不中断**
   （`source-loader.ts` 的 `_logPartialFailure`，10 个子请求任何一个挂了都不影响其余展示）。
   我们现在装备数据是一荣俱荣一损俱损，这条要一起改。

### I-4 顺带确认：op.gg 那条链路他们没有魔法，但有两个小细节我们没做

1. **显式传 `version` 参数**：他们每次请求前先调 `getVersions(region, mode)` 拿版本列表、
   取 `data[0]` 作为 `version` 显式传给 `getChampion`（`source-loader.ts:_resolveOpggVersion`）。
   我们目前不传版本，拿的是上游默认版本——**版本切换当天可能拿到新旧混合数据**。
2. **`getChampion` 的 URL 按模式分形态**（`opgg/index.ts:getChampion`）：
   `arena` 不带 position，`aram` 固定 `/none`，其余才是 `/{position}`。
   这跟我们记忆里 [[deep-legends-opgg-mode-api]] 的实测一致，只是他们把它固化进代码了。

**变异测试要求**：I-3 第 1 步的探测埋点先不用变异测试（只读不改行为）。
真正切换数据源时（第 2 步）必须补：①qq101 返回空/超时的 fixture，断言自动回退到 op.gg
且界面不空白；②两个源都失败的 fixture，断言空态文案能说清是哪个源失败（对照 H-P1-2 的
attempts 决策留痕）；③`version_id` 传数字 id 而非版本名的 fixture，断言能识别出空响应
并报错而不是静默展示空数据（**这是实测踩到的真坑，传 `226` 返回 `result:""`**）。

### I-5 ★0828 追加：qq101 除出装/符文/召唤师技能外，还有 11 个接口，逐个沙箱实测

用户追问"腾讯掌上英雄联盟官方接口还有什么别的接口我们可以用的"。读完
`qq101/index.ts` 全部 17 个方法 + `qq101-protocol.ts` 全部解析器，逐个用真实英雄
（瑞兹13/中单/emerald_plus/16.17）打了一遍，**对着我们现有功能逐条比对**，
结论：其中一个是直接补上我们一个已知数据缺口的强证据，其余大多是"能做但暂无对应功能"，
两个跟现有资源库定位重叠、优先级低。

**★I-5-1 `_newlane`（分路占比）——直接命中 [[deep-legends-opgg-positions-field]] 记录的老缺口**

我们分路推荐功能一直缺 `role_rate`（这个英雄在该分路的出场占比），
只能拿 op.gg `summary.positions` 数组的存在与否粗判"是否为该英雄的分路"，
单分路英雄不能用 `role_rate==1` 判——因为这个字段我们的上游根本没给。

实测 `_newlane?itier=26&version_id=16.17&championid=13`：
```
lane_details: "MIDDLE：1.92_46.12_0.31_T4_52_78.54#TOP：0.53_51.5_0.3_T4_21_21.46"
             = 分路: 选取率_胜率_禁用率_强度分档_排名_★占比(该分路/全部分路)
```
`78.54` / `21.46` 正是我们一直没有的"占比"字段（两条相加=100%，语义精确对应
"这个英雄打中单的局里有 78.54% 是中单，其余在打上单"）。**这条建议单独立项**，
不必等 I-3 的出装/符文切换，风险低、收益直接命中已知投诉。

**★I-5-2 `_confront`（对位胜率）——跟我们已有的 op.gg counters 功能重叠，暂不替换**

实测 `_confront?...&championid=13`：返回 `high_op_details`/`low_op_details`，
每条 `名次_对面英雄ID_胜率_胜率差`，各 15 条（5 个英雄 ×3，疑似三个样本量分桶各一条，
解析器只取第 1、2 字段所以实际去重后是 5 个英雄）。跟我们 0827 修复的
[[deep-legends-counters-wrong-array]]（op.gg 完整 30 条数组）比，**op.gg 那条数组更全**
（30 vs 10），qq101 这条**不构成升级**，只有当 op.gg counters 端点又崩时才值得当兜底。

**用户追问"`_partner`/`_segment` LeagueAkari 自己做了吗、在哪展示"——读完
`champion-data-view-model.ts`（`src-opgg-window`）逐字段核对，结论要更正一处：**

- `_partner`（组合搭档 `getSynergies`）：**LeagueAkari 自己确实做了、也确实渲染了**，
  跟斗魂模式共用同一个 `OpggChampionSynergies.vue` 组件——`champion-data-view-model.ts:254`
  把 `sections.synergies`（qq101 `_partner` 结果）映射成 `champion_id/op_rank/play/win/pick_rate`，
  跟 op.gg 斗魂数据走同一套渲染代码，只是排位模式没有"均排名/吃鸡率"这两列（源数据没有
  `averagePlacement`/`firstPlaceRate`，映射时按存在性可选添加，条件不满足就跳过该字段）。
  **上面"召唤师峡谷没有最佳搭档功能"是我的错误判断，已订正**：功能有，只是字段比斗魂模式少两列。
  实测返回 `名次_搭档英雄ID_胜率_搭档局数_己方局数`（如 `1_112_65.67_67_44`）。
  同一份代码路径也证实了 `_confront`（对位胜率）**同样被渲染**——`matchupCounters`
  在 `champion-data-view-model.ts:177` 并入 `counters` 字段，跟 op.gg 的 `counterChampionIds`
  用同一个 `OpggChampionCounters.vue` 组件展示，即上面 I-5-2 说"跟 op.gg 重叠"没错，
  但要补一句：**LeagueAkari 是真把它当兜底/补充在用，不是只拉不展示**。
- `_segment`（分段胜率 `getTierStats`，实测 itier 强制走 255）：按 10 个大众段位 +
  6 个特殊分段（`24/25/26/27/28` 高分段、`88` 未知）各给一行胜率/选取率/禁用率，
  即"这个英雄在黑铁怎么样、在王者怎么样"的段位曲线图数据源。**逐文件核实：
  `source-loader.ts` 确实请求了这个接口、`qq101.ts:344` 的 `adaptQq101RankedDetails`
  也确实把它塞进了 `ChampionDataDetails.sections.tiers` 这个统一类型里，
  但把整个 `src/renderer` 目录搜了一遍 `.tiers`/`sections.tiers`，零命中——
  没有任何 Vue 组件读取这个字段。即 LeagueAkari 拉了这份数据、适配好了，却没做界面，
  是它自己代码里的死数据。** 我们目前也完全没有这个功能，做不做、值不值得做是新功能决策，
  不能拿"LeagueAkari 也做了"当参考依据，因为它其实没做到界面。
- `_duration`（时长胜率 `getDurations`）：实测 `1_34_53#2_44_51#3_47_46#4_50_35#5_54_11#6_47_34`
  = 时长分档(<20/20-25/25-30/30-35/35-40/40+分钟)_胜率_排名。**同上，同样是拉了+适配了
  但零渲染的死数据**（`src/renderer` 搜 `.durations` 零命中）。我们完全没有这个功能，
  同样是纯新功能决策，不构成"抄它做法"的依据。
- 基础梯度榜（`getTierList`，`RIFT_API_PATH` 裸端点）：实测返回按分路排序的完整强度榜，
  含 `rank/championId/strengthTier/position/winRate/pickRate/banRate/counterChampionIds/rankChange`——
  跟我们 `champions.go` 现有的"英雄排行榜"页面（op.gg 来源）功能重叠，**换源无收益，不建议动**。
- `getClassicTierList`（经典模式，即极地大乱斗/云顶除外的其余非峡谷模式榜？实测传
  `lane=all&itier=255` 打的是 `jade_hero_rank`）：返回英雄 ID 是 `60000+原ID`
  （如 `60026`→英雄26），需要减 60000 换算，**且实测发现 LeagueAkari 自己的解析器可能有
  字段名 bug**——它读 `tierscore_top_hero_list`，但我们实测原始响应里这个字段实际叫
  `winrate_top_hero_list`（两者其一，需要以我们自己实测为准，不能直接照抄它的字段名）。
  这个端点具体对应什么模式（云顶之弈？大乱斗？）还没查证，**暂不建议投入，优先级最低**。
- 3 个海克斯大乱斗接口（`fuwen_aram_hero_rank_v2`/`fuwen_aram_rune_rank_v2`/
  `fuwen_aram_hero_parttner`）：都实测有真实数据返回。但我们海克斯大乱斗模块已经用
  resg.top（[[deep-legends-mayhem-redesign]]，本身是 Riot 官方采集器的转发）跑通，
  **两个源定位重叠，替换收益不明确，不建议现在动**。

**结论与建议顺序（★用户已拍板，非"留给用户选"）**：
**只做 I-5-1**（`_newlane` 分路占比，单独立项，风险最低、直接堵已知缺口）。
**`_partner`（组合搭档）/`_segment`（段位胜率曲线）/`_duration`（时长胜率）三项
用户已明确表态不做**——上面的分析（含"LeagueAkari 自己是否渲染了"的订正）保留作为
以后如果重新考虑这几项时的参考依据，但**本轮及可预见范围内不要实现，不要当成待办事项**。
I-5-2（`_confront` 对位胜率）及基础梯度榜/`getClassicTierList` 同样不建议动
（跟现有功能重叠或收益不明确）。

**本文件（G/H/I 组）执行要求同 WORKLIST-R33.md**：`go build`/`go vet`/`gofmt` 干净、
Go 与 JS 测试全绿；每处改动按各组内已写明的"变异测试要求"逐条补（H-P0-1、H-P0-2、
G-5、I-3 各自的变异测试段落已经写在对应小节里，不重复列一遍）；I-5-1 是本文件里
唯一真正要实现的新条目，改完后同样要给出真机验证（对着一个已知单分路英雄和一个
双分路英雄各测一次，确认"占比"字段两条相加约等于 100%、且能用它替换掉原来
"role_rate 缺失"那条判断逻辑）。

