# WORKLIST-R49（给 GPT 执行）

> 本轮基于真机日志 `lol-loot-diagnostics-0831-2115.jsonl`（3132 行）+ 九张截图 + 全量代码走查。
> 所有结论带 file:line。**三条最值得先看的发现：**
>
> 1. **绝活哥符文的"没有该位置样本"是假的** —— 8 次里 6 次是撞满 25 秒超时返回 0 条，
>    然后被一律归因成"没样本"。真根因是**串行 30 场详情 + 预算(96) 大于全局限速配额(90)**，
>    两者互相踩，形成"一轮抽干配额 → 下一轮 account 全部排队饿死"的循环。（A 组）
> 2. **位置占比数据早就在前端手里了** —— `positions[].roleRate` 已随对局页响应下发，
>    英雄详情页甚至已经在渲染它。缺的只是"按钮上没显示"和"全链路从没排过序"。（C 组）
> 3. **hexdata augments 的 rows=0 有了新根因** —— R46 修的正则不是这次的原因。
>    `fields=627 / rows=0` + 627=3×209 强烈指向**上游表从 2 列变成了 3 列**，
>    但这是推断不是实证，**必须先探测列形状，不许猜着改解析器**。（F 组）

---

# A 组（P0）：绝活哥符文 —— 出不来 / 等几十秒 / 错误文案骗人

## A-0 先看这组数字（不用再验证，直接用）

`specialist_runes_done` 共 8 次：

```
dur=  7640  budget_used= 19  runes=9  pos=top      ← 成功
dur= 25001  budget_used= 93  runes=0  pos=support  ← 撞满超时
dur= 25001  budget_used=  3  runes=0  pos=mid      ← 撞满超时
dur= 25001  budget_used=  3  runes=0  pos=top      ← 撞满超时
dur= 16728  budget_used= 66  runes=6  pos=mid      ← 成功
dur= 25001  budget_used= 92  runes=0  pos=mid      ← 撞满超时
dur= 25001  budget_used=  3  runes=0  pos=adc      ← 撞满超时
dur= 25004  budget_used=  3  runes=0  pos=mid      ← 撞满超时
```

`step_failed`：`account/timeout/budget_remaining=93` **12 次**（4 轮 × 3 玩家）；
`match_detail/timeout/budget_remaining=3或4` 3 次。
`handler`：`accepted` 8 次，`empty + no-position-sample` **6 次**。

**用两组数能把耗时精确复原**：`budget_used=66 → dur=16728`（66/3=22 步 × 760ms），
`budget_used=93 → dur=25001`（93/3=31 步 × 806ms）。四轮独立算出的单请求 RTT 都落在 760~810ms。
**所以耗时 = 最长串行链 × 780ms，没有别的解释。**

> ⚠️ 更正我自己的一个预设：**单个 HTTP 请求吃不掉 25 秒**。
> `champions.go:474` 给 http.Client 设了 `Timeout: 12s`。能占满 25 秒并报 `timeout` 的，
> 只可能是阻塞在 `ctx.Done()` 上 —— 全链路只有两处这样的阻塞点，其中限速器排队与日志逐位吻合。

## A-1（主犯之一）内层 30 场详情是串行的，R43 的坑换了个位置复发

`specialist_runes.go:230-245`：

```go
for _, matchID := range matchIDs {
	if len(matches) >= specialistRunePerPlayerMax || !take() { break }
	match, err := p.matchByID(ctx, matchID)   // ← 串行，无并发
```

外层三个玩家已经是并发（`:199-271` 每人一个 goroutine，R43 的教训吸取了），
**但每个玩家内部这 30 次循环一次都没拆**。最长串行链 = `1(account) + 1(match_ids) + 30(详情)` = 32 步
× 780ms = **24.96 秒**，恰好等于 25 秒预算。

**换句话说：满深度扫描在设计上就跑不完。** 只要某个绝活哥最近 30 场里该英雄局数不够 3 场，
代码就会把 30 场全扫一遍，必然 0 结果、必然烧光预算。

**改法**：内层详情改并发（每玩家 3~4 并发），把最长串行链从 32 压到 ~10。

## A-2（主犯之二）★请求预算(96) 大于全局限速配额(90)，一轮能抽干整个应用

这是结构性矛盾，**请重点看**：

```go
specialistRuneRequestBudget = 3 * (2 + 30) = 96      // specialist_runes.go:21
longLimit                   = 90 / 2 分钟             // riot_api.go:216
```

`riot_api.go:211-248` 的 `wait()` 在长窗口满时会 sleep 到窗口滑出（**最长 120 秒**），
sleep 里 `select { case <-ctx.Done(): return ctx.Err() ... }` —— 这就是那 12 次
`account/timeout/budget_remaining=93` 的真身：**不是上游慢，是排在我们自己的限速队列里被 ctx 掐死**。

对上日志序列逐条吻合：

```
1) used=19  ok    → 窗口 +19
2) used=93  超时   → 窗口累计 >90，配额抽干
3) used=3   account超时  ← 被 (2) 饿死
4) used=3   account超时  ← 被 (2) 饿死
5) used=66  ok           ← 2 分钟窗口滑出，恢复
6) used=92  超时   → 再次抽干
7) used=3   account超时  ← 被 (6) 饿死
8) used=3   account超时  ← 被 (6) 饿死
```

**改法（三选一或组合，请说明选择理由）**：
- 把 `specialistRuneMatchScanMax` 从 30 降到 10 → budget = 3×12 = 36，远低于 90
- 给绝活哥单独划一个限速子池（不超过 40），不让它抽干全局
- `wait()` 加最大排队时长：算出的 sleep 超过阈值（比如 3s）直接返回可识别的 `errThrottled`，
  让上层立刻降级并把真实原因报给用户，而不是傻等到 ctx 死

> 注意：还有一处**不占 budget 但占限速窗口**的消耗 —— `specialist_runes.go:275-284`
> 扫描结束后还要查最多 9 个对手段位，走 `a.playerRankScore` → KR 分支 → 又是 Riot 请求。
> 所以真实配额消耗是 `budget_used + 最多 9`，日志里看不到这部分。改预算时要把它算进去。

## A-3 每一步都没有自己的超时，只有一个 25 秒全局预算

`specialist_runes.go:104` 是整条链路**唯一**的超时：

```go
ctx, cancel := context.WithTimeout(r.Context(), specialistRuneRequestTimeout)  // 25s
```

`account` / `match_ids` / `match_detail` 三步全部透传这同一个 ctx。
后果就是 A-2 里那四轮：3 个 account 卡住 → 整整 25 秒全废 → 后面的步骤一个都没跑。

**改法**：每步套一层自己的 deadline —— account 5s、match_ids 5s、单场 detail 4s。
这样单点慢/排队不会拖垮整轮，其余玩家仍能产出结果。

**注意 A-1/A-2/A-3 必须一起做才闭环**：只加单步超时救不了 `used=92/93` 那两轮
（串行 30 场仍会撞 25s）；只改并发不改预算，仍会抽干限速器。

## A-4 ★错误文案在骗用户

`specialist_runes.go:112-117`：

```go
runes := a.riot.specialistRunes(ctx, championID, metadata.Slug, metadata.NameZH, position)
if len(runes) == 0 {
	recordHandler("empty", "no-position-sample")
	respondJSON(w, map[string]any{"reason": "no-position-sample", "runes": runes})
	return
}
```

**只看 `len(runes)==0`，不看 `ctx.Err()`，不看有没有 step 失败过。**
"查失败了"和"确实没样本"被合并成同一个分支。

更糟的是 `specialistRunes`（`:121-163`）**函数签名根本没有 error 返回值**，
几乎所有失败路径都返回空切片，错误信息彻底丢掉。诊断日志里明明记了 `errorKind:"timeout"`（`:213`），
**但没有任何路径把它带回 HTTP 响应**。

前端把它渲染成（`web/gameplay.js:3638` 和 `:3641`）：
- 「该绝活哥最近 30 局没有打过下路」
- 「最近 30 局没有该位置样本」

**用户图二看到的就是这句 —— 而真实原因是超时。**

**次生副作用（也要修）**：`web/gameplay.js:3177` 把 `[]` 写进 `state.specialistRunes`，
导致 `:3151` 的 `has()` 命中，**这个英雄+位置在整个 game generation 内永不重试**
（上报为 `cached-empty`）。同时 `:3616` 的 `specialistFailed` 因为 `has()` 为 true 恒为 false，
"读取失败"分支永远走不到。**一次超时会把这个位置永久锁死成"没有样本"。**

**改法**：`specialistRunes` 改成返回 `([]rune, specialistOutcome)`，至少四态：

| reason | 含义 | 前端文案 |
|---|---|---|
| `no-position-sample` | 扫描**完整跑完**，确实没有该位置的该英雄局 | 维持现文案 |
| `upstream-timeout` | `ctx.Err() != nil` 或有 step 报 timeout | 「韩服接口响应超时，请稍后重试」+ 重试按钮 |
| `upstream-throttled` | `wait()` 排队超阈值 / 命中 429 | 「请求过于频繁，约 1 分钟后可重试」 |
| `upstream-error` | 404 / 5xx / parse | 「上游数据异常」 |

判据是现成的：`specialist_runes.go:369-398` 的 `specialistErrorKind` 已经能分辨
timeout / rate-limit / 404 / 5xx，只要在 `loadSpecialistRunes` 里聚合各 goroutine 的
errorKind 往上抛即可，**不用新写解析逻辑**。

前端相应地：只有 `no-position-sample` 才写 `state.specialistRunes.set(key, [])`（真没样本，锁死是对的）；
`upstream-*` 走 `specialistRuneFailures` + 60s 冷却（`:3158` 已有），并显示可重试。

## A-5 account 结果完全没有缓存，失败结果也不缓存

`riotProvider`（`riot_api.go:137-152`）里有 `matchCache`（600 条）和 `specialistCache`，
**唯独没有 RiotID → PUUID 的缓存**。同一个绝活哥，每一轮、每个位置、每次重试都要重打一次 account-v1。

`specialist_runes.go:165-179` 的 `finishSpecialistRuneFlight` 里 `if cache && len(result) > 0`
才写缓存 —— **空结果一律不写**，下次请求立刻重新发起完整 96 请求的扫描。
这就是日志里 champion 777/804 反复重试多轮、每轮都重新超时的原因。

另外部分成功的 TTL 只有 2 分钟（`specialistRunePartialCacheTTL`，`:23` + `:170-172`）：
`len(result) < 9` 就降到 2 分钟。第 5 轮返回 6 条 → 2 分钟后又要重扫。

**改法**：加 RiotID→PUUID 缓存（TTL 给 24h，PUUID 几乎不变）；
**缓存失败态**（负缓存 30~60s + 指数退避），避免同一位置反复重扫。

## A-6 位置切换为什么每次都要重等（用户明确抱怨了这条）

**先说清楚一个不能改的事实**：OP.GG 每个位置返回的是**不同的 3 个玩家**
（`champions_structured.go:972-978`，`query.Set("position", position)`）。
换位置 = 换玩家 = 换 PUUID = 换 matchIDs，`matchCache` 一条都命不中。
**跨位置零复用是数据源结构决定的，不是缓存写漏了。**

**也不要走"先查位置分布再定向拉取"这条路**：Riot match-v5 的 `by-puuid/ids`
只支持 `queue` 和 `type` 过滤（`riot_api.go:530-541`），拿不到英雄/位置 ——
位置信息只存在于 match detail 里面，必须先下载。

**真正的浪费在第一级过滤**（`specialist_runes.go:247-258`）：
绝活哥最近 30 场里该英雄可能只占 5~10 场，剩下 20+ 场详情（每场 150KB~500KB JSON）纯属白下，
但一样占 budget、占限速窗口、占 780ms。

**改法**：建一个 **key = PUUID 的「玩家近 30 局摘要」缓存**，
value = `[{matchID, championID, position, gameCreation}]`，TTL 30 分钟。
首次为某玩家扫描时顺手填满；之后**同一玩家在任何位置的查询都能先在摘要里筛出候选 matchID，
再只下载那几场的详情**。首次仍要全扫，但同一位置的重试、以及 2 分钟 TTL 到期后的重扫，
成本从 32 请求降到 3~5 请求 —— 对日志里 mid 反复出现 4 次的场景直接见效。

**前端顺带**：`:3641` 的加载文案只有一句「正在读取韩服绝活哥符文」，
用户不知道要等多久。改完之后如果仍需 10 秒以上，加个预期时长提示。

## A-7 顺带修的三处

1. **`specialistSlots` 只有 3 个槽**（`riot_api.go:187`），`specialist_runes.go:153-158`
   抢不到槽就直接返回空、**且不打任何诊断事件**。用户快速连切 4 个位置时，
   第 4 个会静默返回空 → 又渲染成"没有该位置样本"。**这是第三条通向错误文案的路径。**
2. **`specialistErrorKind` 的 5xx 判定用暴力字符串扫描**（`:389-392`，
   `for status := 500; status <= 599`），任何错误消息里恰好含 500~599 的数字
   （matchID、LP 值都可能）会被误判成 5xx，污染诊断。
3. **`enrichSpecialistOpponentRanks`（`:293-302`）疑似死代码** —— 全仓无调用方，
   实际用的是 `:276` 内联的 batch。请确认是历史残留还是漏接，**如果是残留就删掉**
   （对照 R43-J「死代码制造推进假象」的教训，仓库里不该留这种东西）。

## A-8 测试护栏也要改

`specialist_runes_test.go:462-463` 硬锁 `specialistRuneRequestTimeout != 25*time.Second`。
这是**只锁常量值、不锁行为**的护栏 —— 它保证不了"25 秒够用"，改常量时会误报失败。
（同 R46-D-4「测试锁死了错的值」。）

改完之后护栏应该断言的是**行为**，比如：
- 单步超时确实生效（模拟一个 6 秒的 account 响应，断言整轮在 ~5s 就返回而不是 25s）
- 超时时 reason 是 `upstream-timeout` 而不是 `no-position-sample`
- 预算总额 ≤ 限速器长窗口配额

**变异测试必做**：A-3（去掉单步超时应 FAIL）、A-4（把 timeout 归因回 no-position-sample 应 FAIL）。

---

# B 组（P0）：装备方案 —— 去重、排序、名字对不上

## B-1 核心装现在取了 15 条路线不是 3 条

`web/gameplay.js:3907-3929` 是唯一的组装函数：

```js
3910: const coreOptions  = build?.coreOptions || [];      // 上游全部 15 条路线，没有 slice
3911: const depthOptions = [...(build?.fourthOptions||[]), ...(build?.fifthOptions||[]), ...(build?.sixthOptions||[])];
3913: const items = (ids) => [...new Set((ids||[]).map(Number).filter(...))].slice(0, 20).map((id)=>({id,count:1}));
3914: const mergedItems = (options) => items((options||[]).flatMap((o)=>o.ids||[]));
3920: const core  = mergedItems(coreOptions);   if (core.length)  blocks.push({type:"核心装",   items: core});
3922: const depth = mergedItems(depthOptions);  if (depth.length) blocks.push({type:"后续装备", items: depth});
```

`coreOptions` 是后端 `championCoreRecommendationLimit = 15`（`champions.go:39`）条**完整路线**，
每条 `ids` 是 `[itemA,itemB,itemC]`，前端 flatMap 全部拍平后去重。**没有"只取前三条"。**

（`LIVE_CORE_OPTION_LIMIT=15` / 预览 6 条在 `web/gameplay.js:137-138`，那是 UI 展示层，与 payload 无关。）

**改法**：`coreOptions` 加 `.slice(0, 3)`。

## B-2 ★两组之间完全没有交叉去重（用户抱怨的重复就是这个）

组内用 `new Set` 去重了，**跨组一次都没去重**。
后端 `validateGameplayItemSetRequest`（`gameplay.go:5133-5139`）也只校验组内不重复，跨组重复是合法的。

**实证（用户图九）**：后续装备 8 件里 3500/3300/3000/2800 这 4 件在核心装里已经出现过，
其中 3300 甚至在两组里都被高亮。

**改法**：`depth` 在 `mergedItems` 之后，用**最终**核心装的 id 集合做差集：
```js
const coreIDs = new Set(core.map(({id}) => id));
const depth = mergedItems(depthOptions).filter(({id}) => !coreIDs.has(id));
```
差集必须对 `slice(0,3)` 且经过 `.slice(0,20)` 截断之后的那一份做，否则会漏。

**三个坑，请照做**：
- **后续装备可能变空**：`:3923` 已有 `if (depth.length)` 守卫，会整组消失。这是期望行为
  （用户要的就是"没有一件重复"），不用额外处理，但要在测试里覆盖。
- **顺序**：`new Set` 保插入序、`filter` 也保序，上游推荐顺序不会被破坏。
  但和 B-3 的价格排序有先后：**先去重、后排序**。
- **20 件上限**：只取 3 条路线后核心装最多 9 件，`:3913` 的 `.slice(0,20)` 从此形同虚设，
  **但不要删掉它** —— 它是防 `gameplay.go:5135` 返回 400 的最后一道闸。

## B-3 每组按装备价格升序排序（项目里目前一处价格数据都没有）

**现状**：完全没有排序，就是上游 flatten 顺序。用户图八实证：出门装显示 `400 / 50 / 450 / 450`，
生命药水（50 金）排在第二 —— 用户要求它排第一，也就是**升序**。

**价格数据现在一份都没有。** 三条装备目录链路的结构体全都没解析价格：

| 来源 | 位置 | 现在解析了什么 |
|---|---|---|
| LCU `/lol-game-data/assets/v1/items.json` | `gameplay.go:5856-5864` | id/name/displayName/description/iconPath/imagePath |
| ddragon `item.json` 兜底 | `champions.go:981-995` `ddragonAssetList` | name/description/tooltip/**cost（这是技能蓝耗不是金币，别搞混）**/into/image |
| CommunityDragon（斗魂） | `yourgg_arena.go:63-72` | id/name/displayName/nameTRA/description/iconPath |

对外的 `gameplayItem`（`gameplay.go:5506-5511`）也只有 id/name/description/iconPath。

**补数据成本极低，已实测确认**：CommunityDragon 的 `zh_cn/v1/items.json`（868 条）字段现成：
```
2003 生命药水    price=50   priceTotal=50
1086 多兰之弓    price=400  priceTotal=400
3006 狂战士胫甲  price=300  priceTotal=1100   ← 商店显示 1100
3008 暴食胫甲              priceTotal=1000   ← 与用户截图一致
3153 破败王者之刃          priceTotal=3200   ← 与用户截图一致
```

**必须用 `priceTotal`（总价）而不是 `price`（合成差价）** —— `price` 会让狂战士胫甲变成 300，
与游戏内显示对不上。

LCU 的 `/lol-game-data/assets/v1/items.json` 是同一份 schema（CDragon 就是它的镜像），
所以 `gameplay.go:5856` 那个匿名结构体**只要加一个 `PriceTotal int64 \`json:"priceTotal"\`` 就有了，
不需要新增任何上游请求**。ddragon 兜底那条要补的是 `gold.total`。

**改法（在 Go 侧排序，不要在前端排）**：

理由：前端 `state.items` 是异步加载的（`web/gameplay.js:2559-2571`，失败或未完成时为 null），
用户在目录到位前点"应用"，排序会**静默不生效** —— 这正是本项目反复踩过的"看起来做了其实没做"。

- `gameplayItem` 加 `Price`，三处目录填充同步
- 在 `newLCUItemSet`（`gameplay.go:5211-5223`）**之前**，用带缓存的 `id→priceTotal` map
  对每个 block 做**稳定升序排序**（价格相同保持上游推荐顺序）
- **拿不到价格目录时保持原顺序**，不要把没有价格的装备排到 0 金位置
- **必须排在 PUT 之前**：`sameLCUItemSet`（`gameplay.go:5317+`）是逐件按下标比对的，
  排序若发生在写入之后会导致回读校验失败、整个应用报错

> ⚠️ **对局页 UI 不要跟着改成价格序**。`web/gameplay.js:3835-3840` 有一大段注释解释
> "绝不本地重排"的历史教训，那里的顺序是上游推荐度。**价格排序只作用于写进客户端的 item set。**

## B-4 ★方案标题/UID 用的位置 ≠ 里面装的数据（用户图八图九能直接看出来）

`buildItemSetPayload` 的标题和 UID 取的是 `self?.position`（`web/gameplay.js:3927-3929`），
而 `build` 来自 `liveRecommendationTarget` 的 `position`，后者会被用户的位置切换覆盖，
**后端还会二次改判**。

**实证**：8/31 那局客户端给的位置是 middle，用户切到「下路」、后端也解析成 adc
（日志 `position_source: "opgg-primary"`），结果游戏里方案叫 **`DL · 芸阿娜 · 中路`，
里面装的却是下路的数据**，UID 也是 `deep-legends-v1-804-middle`。

**连带影响**：同一把里切位置反复应用会**互相覆盖同一个 UID**，存不下两套。

**改法**：标题/UID 改用后端回传的 `build.position`
（`gameplay.go:3175` 的 `gameplayRecommendationBuild.Position` 已经带回来了，前端目前完全没用）。

## B-5 「先显示错的、过很久才变对」—— 不是我们的 bug，但这条链路零可观测性

我把用户两张截图逐件比对了（20:48:38 "错的" / 20:50:32 "对的"）：

| 分组 | 对的那张 | 错的那张 |
|---|---|---|
| 出门装 | 400 / 50 / 450 / 450 | 450 / 450 |
| 鞋子选择 | 1100 / 1000 / 1200 | 1200 |
| 核心装 | 3000 2650 **3500 2650 2800 3000 3000 3300 3000 2650** | 3500 2650 2800 3000 3000 3300 3000 2650 |
| 后续装备 | 3500 3300 **3000 2800 3000 3200 3200 3400** | 3000 2800 3000 3200 3200 3400 |

**错的那张 = 对的那张每组各砍掉最前面两件，且四个组标题全部为空。**

这个形状我们的代码造不出来，三条排除理由：
1. 不是两次写入 —— 全仓 `grep "lol-item-sets"` 只命中 `gameplay.go:5175`，
   写入点唯一、一次 PUT、写完立刻回读校验（`gameplay.go:5174-5209`）；
   前端 `web/gameplay.js:4174-4190` 也没有任何 retry。
2. 不是组标题写空 —— `validateGameplayItemSetRequest`（`gameplay.go:5117-5141`）
   明确拒收 `Type` 为空的分组，这种 payload 根本发不出去、更过不了回读校验。
3. 不是旧方案残留 —— `upsertLCUItemSet`（`gameplay.go:5248-5288`）是**同 UID 原地替换**，
   不是"先删后建"；出现两条同 UID 会直接报错。

**判定为游戏内商店的渲染/同步中间态**（写入发生在开局前几十秒，与游戏加载抢时间）。

**但这条链路目前完全不可观测** —— 3132 行日志里 `grep -i item_set` **零命中**，
我们连"用户点了几次、每次发了几组几件"都不知道。

**要做的**：在 `handleGameplayItemSetApply` 里补埋点：
```
{event:"item_set_apply", champion_id, uid, position, self_position,
 blocks:[{type,count}], phase, duration_ms, verified}
```
⚠️ **R39 的教训：加埋点必须端到端验证能落盘**（后端有事件白名单），
不要加完就当做完了，要在真机或测试里确认这条事件真的写进了 jsonl。

顺带：`applyItemSet` 成功后的 toast 里带上「N 组 / M 件」，用户下次能自证第一屏对不对。

## B-6 测试护栏会被打到

`web/champions.test.cjs:3484-3510` 硬钉了 `payload.blocks[2].items.length === 20`
和 `blocks[3] = [3071,3153,6333,6692]`。这条**同时也是唯一**覆盖 `buildItemSetPayload` 的护栏。
改完必须同步改断言，**不要删护栏，改成断言新的期望值**，并补：
- 核心装与后续装备的 id 集合交集为空
- 每组内价格非递减
- 只取 3 条路线

**变异测试必做**：B-2（去掉跨组去重应 FAIL）、B-3（去掉排序应 FAIL）。

> 顺带确认一件事：`:3924-3925` 的棱彩装备组疑似死代码 —— `prismOptions` 只在斗魂/海斗出现，
> 而那两个模式按 R44-E-3 已经不显示"应用装备方案"按钮（`:3869` 的 `capabilities.hasAugments` 闸门）。
> 改的时候顺手确认，**不要为它浪费去重逻辑**。

---

# C 组（P1）：位置徽章与位置占比

## C-1 顶部「当前位置」被位置按钮耦合了（用户判断正确）

耦合点在 `web/gameplay.js:3015-3018`：

```js
const positionOverride = state.livePositionOverride.get(baseKey) || "";
const position = positionOverride || clientPosition;   // ← 用户点击的覆盖值吃掉了客户端位置
```

注意 `:3007` 的 `const clientPosition = self?.position || "other";` ——
**客户端真实位置本来就单独存着，字段名就叫 `clientPosition`，只是徽章没用它。**

徽章渲染在 `:3247-3256`，`:3253` 读的是 `payload.resolvedPosition || target?.position`，
这两个都是"用户选择后"的值。

客户端真实位置的来源确认无误：`gameplay.go:4433` 的 `normalizePosition(SelectedPosition, SelectedRole)`
← `gameplay.go:4648/4678` 的 `SelectedPosition: selected.AssignedPosition`（LCU 的 `assignedPosition`），
进游戏后固定不变，**完全符合用户要的语义**。

**改法**（一行）：
```js
// gameplay.js:3253
const resolved = livePositionDisplay(target?.clientPosition || self?.position);
```
白名单（`:3254`）已经能挡掉 `other`/`""`，斗魂/大乱斗不会误显示，不用额外处理。

**护栏要同步改**：`web/champions.test.cjs:3223` 现在传的是 `players: []` +
`recommendations.resolvedPosition`，改后没有 `isCurrent` 玩家就不会出徽章了。
**改成"传入 `players:[{isCurrent:true, position:"top"}]` 且 `resolvedPosition:"mid"` 时徽章仍显示上路"** ——
这样才真能杀住"又被耦合回去"的回归。

## C-2 ★位置按钮加占比 + 按占比排序 —— 数据早就有了，零额外请求

**先回答用户的前置问题：OPGG 能不能拿到占比？能，而且已经在前端手里了。**

字段链路（逐层实证）：

```
OP.GG 原始           data.summary.positions[].stats.role_rate
  ↓ champions_structured.go:106-113 + champions.go:1465-1481（RoleRate float64）
解析落库             champions_structured.go:1139-1162
  ↓ ratePercent（champions.go:1770-1775）：0~1 自动放大到 0~100
结构体 JSON tag      champions.go:371-381 → `json:"roleRate"` + `json:"play"`
  ↓
对局页响应           gameplay.go:3101 `Positions []championPositionOption json:"positions,omitempty"`
                     赋值在 gameplay.go:3900
  ↓
前端已经在读了       web/gameplay.js:3596 用了 positions[0].roleRate
英雄详情页已经在渲染 web/champions.js:1475 `占${percent(item.roleRate)}`
演示数据也齐了       web/demo-data.js:521 `roleRate: 76.4 / 23.6`
```

**下发的一定是百分比（0~100），不用再乘 100。**

**腾讯 QQ101 是权威覆盖不是备胎**：`qq101.go:253-259` 从 `lane_details` 第 6 个字段解析 roleRate；
合并逻辑 `qq101.go:289-303` 里 **roleRate 是无条件覆盖的**（其它字段都是"为 0 才补"，唯独它直接盖）。
为什么需要它：OP.GG 的 `role_rate` 实测**可能为 0**（fixture `qq101_test.go:342` 就是这种情况，
靠 QQ101 合并后才凑出 100%）。对局页同样走这条链路（`gameplay.go:3364/3372` → `loadStructuredDetail`）。

### ★两个必须知道的坑

**坑 1：`positions` 数组从来没排过序，`positions[0]` 不是占比最高的那个。**
- Go 侧：`champions_structured.go:1145` 按上游数组顺序 append，无排序；
  `mergeQQ101PositionShares` 还会把 QQ101 独有的位置**追加到末尾**（`qq101.go:317-321`）
- JS 侧：全仓 16 处 `.sort(`，**没有一处排 positions**
- 反证：`champion_network_test.go:675,683` 的 fixture 里 TOP(55%) 排在 SUPPORT(91%) 前面，
  断言明确要求保持这个顺序 —— 改排序时这条断言要一并处理

**顺带修一个现存 bug**：`web/gameplay.js:3596` 的文案
`已按${rate(positions[0].roleRate)} 场次占比选择` 用的是 `positions[0]`，
而后端 `resolveGameplayRecommendationPosition`（`gameplay.go:3844-3866`）是**遍历取 max**。
两者不一致 → **这行提示语在多分路英雄上显示的是一个错误的百分比**。做排序会顺手修掉它。

**坑 2：`positions` 只在 ranked 模式存在。** `champions_structured.go:152-157`：
只有 `"ranked"` 是 `opggPositionRequired`，aram/arena/hextech-aram/urf/nexus-blitz 都是
`LiteralNone` 或 `Omitted`。位置按钮条本身就只在召唤师峡谷出现，符合预期。

### 改法

按钮模板在 **`web/gameplay.js:3600`**（单行超长）。三处改动：

1. **排序**：map 之前插
   ```js
   const ordered = [...positions].sort((l, r) =>
     (Number(r.roleRate)||0) - (Number(l.roleRate)||0) || (Number(r.play)||0) - (Number(l.play)||0));
   ```
2. **渲染占比**：`<span>${positionLabel(display)}</span>` 后追加
   ```js
   ${item.roleRate > 0 ? `<b class="live-position-rate">${rate(item.roleRate)}</b>` : ""}
   ```
   （`rate()` 是现成的百分比格式化函数）
3. **roleRate 为 0 的兜底**（OP.GG 无 role_rate 且 QQ101 未命中）：用 `play` 自算
   `share = play / sum(play) * 100`。`play` 字段后端已下发（`champions.go:377`）。

CSS 加在 `web/gameplay.css:1023` 附近。
⚠️ `.live-position-switch`（`gameplay.css:1018`）是 `flex-wrap: nowrap; overflow-x: auto`，
加百分比后 5 个按钮更容易触发横向滚动 —— 把 chip 的 padding 从 `4px 8px` 收到 `4px 6px`，
或在**已有的** `@container recommendation-area (max-width: 700px)`（`gameplay.css:1497`）里隐藏百分比。
**不要新增 `@media`。**

---

# D 组（P1）：五处 UI 缺陷

## D-1 三个 tab 切换时宽度跳动 —— 真因不是字重也不是 border

我先排除三个常见猜测：`font-weight: 700` 写在基态（`gameplay.css:1074`），
`.is-active`（`:1076`）**没改字重**；基态 `border: 0`，选中态用的是不参与布局的 `box-shadow: inset`；
`padding: 0 12px` 只声明一次。**这三条本来就写对了。**

**真因**：`web/gameplay.js:3622` 只有 `specialist` 一个 section 带 `note`，`:3628` 是条件渲染：

```js
sections.push({ key: "specialist", title: "绝活哥", note: "当前分路 · 最近 30 局", ... });
const sourceNote = activeSection?.note ? `<small class="rune-source-note">...</small>` : "";
```

这个 `<small>`（`white-space: nowrap; flex: 0 0 auto`，约 100~110px + gap 10px）
**只在选中"绝活哥"时存在**。它一出现，`.rune-source-tab-buttons`（`flex: 1 1 auto`）
被压缩约 110~120px，而每个 tab 是 `flex-grow: 1`，三个平分损失 → **每个跳变 ±37~40px**。

**改法 A（推荐，只动 CSS 三行）**：
```css
/* gameplay.css:1073 */ .rune-source-tab-buttons { flex: 0 1 auto; }   /* 1 1 auto → 0 1 auto */
/* gameplay.css:1074 */ .rune-source-tab { flex: 0 0 108px; }          /* 宽度锁死 */
/* gameplay.css:1078 */ .rune-source-note { flex: 0 1 auto; min-width: 0; margin-left: auto;
                                            overflow: hidden; text-overflow: ellipsis; }
```

**改法 B（保留 tab 撑满整行的观感）**：`.rune-source-tabs` 加
`position: relative; padding-right: 130px`（恒定预留），note 改绝对定位。

**请二选一并说明理由。** 窄容器规则加进已有的 `@container recommendation-area (max-width: 700px)`，
**不要用 `@media`**。注意：若选"窄屏隐藏 note"，必须先做 A 或 B，否则 `display:none`
反而在断点附近制造新的跳变。

## D-2 装备名太长导致图标错位 —— 省略号其实已经有了，是阈值定错

⚠️ **先更正我自己的两个预设**：`min-width: 0` **已经全都写了**
（`.config-option`/`.config-icons`/`.config-item`/`.route-step` 四处），
而且因为它们是 `flex: 0 0 auto`（shrink=0），这些 `min-width` 其实是**死代码**；
`text-overflow: ellipsis` **也已经有了**（`gameplay.css:1384`，还带 `white-space: nowrap`）。

**真因两段**：

1. **横向**：`.config-item` 是 `inline-grid` 单列，列宽 = `max(42px, min(名字宽, 70px))`。
   短名（"无尽之刃"≈32px）→ 42px；长名（"斯特拉克的挑战护手"）→ 顶到 **70px**。
   核心装一行 3 件，最坏差 84px。而核心列轨道是 `minmax(0,1.6fr)`（**下限 0，永不为内容变宽**）
   + `.config-icons` 不收缩 + `.config-option` 是 `flex-wrap: nowrap`
   → 直接溢出核心列，被 `.build-recommendation { overflow: hidden }`（`gameplay.css:1294`）裁掉。
   同时每行第 2/3 个图标的 x 坐标随各自名字长度浮动 → **竖列排不齐**（这就是用户看到的错位）。
2. **纵向**：`.config-item` 内容高 = 42(图标) + 3(gap) + 12(8px×1.5) = **57px**；
   `.config-option` 是 `min-height: 60px` + `padding: 1px` → 内容盒仅 **58px**，**只剩 1px 余量**。
   任意一行超高，该列以下全部下移且**累积**，越往下越歪。

**改法**：
```css
/* gameplay.css:1379 */
.config-item { display: inline-grid; width: var(--option-icon-size, 42px); justify-items: start; gap: 3px; }
/* gameplay.css:1384 */
.item-option-name { display: block; width: 100%; max-width: 100%; overflow: hidden;
                    font-size: 8px; line-height: 12px; text-overflow: ellipsis; white-space: nowrap; }
```
关键是**改成固定 width，不能再用 max-width 让内容浮动**。
（若 42px 只放得下 3 个汉字太窄，折中用 `width: 52px` 并把 `.config-icons` 的 gap 从 6px 收到 4px。）
顺手把 `build-item-row.css:4` 的 `--build-row-height` 从 60px 提到 64px，给那 1px 的脆弱预算留余量。

**tooltip 必须复用现成机制**（`web/app.js:2744-2890` 的 `setupFloatingTooltips`，
body 单例 + 全局事件委托 `closest("[data-tooltip]")`，动态 innerHTML 自动生效）：
```js
// gameplay.js:3977
<small class="item-option-name" data-tooltip="${escapeHTML(name)}"
       data-tooltip-overflow="self" data-tooltip-size="compact">
```
`data-tooltip-overflow="self"`（`app.js:2762-2777`）让**没截断时不弹**，
避免和图标自身的装备说明 tooltip 连环打架。**不要用原生 `title`**，全站统一走浮层。

⚠️ `build-item-row.css:15/24/30` 用的是**匿名** `@container`，绑的是最近祖先
`.build-recommendation`（`gameplay.css:1294`），**不是** `recommendation-area`。改这个文件要注意区分。

## D-3 海克斯卡片：高度调小 + tooltip 命中区扩大

⚠️ **更正一个预设：说明文字的数据是有的，不是上游缺口。**
`gameplay.go:3106` → `champions.go:239 Assets` → `champions.go:216 Description`；
CHERRY 填的是真文案（`champions_structured.go:2301`，来源 `loadCommunityDragonAugments`）；
测试 fixture 可证（`champions_test.go:355-365` 断言 `Description == "获得总攻击速度。"`）；
前端 `gameplay.js:3462-3487` 的 `wrapAugmentIcon` **已经生成了 data-tooltip**。

**真正的体感问题**：hover 命中区只有左侧 62px 的 `span.arena-augment-icon`，
`article.live-augment-option` 本身没有 `data-tooltip` —— **鼠标停在名称/档位/胜率那一大片什么都不弹**。

**高度**：`min-height` 和图标尺寸互相咬死，**必须同改两处**，只改一个另一个会顶住：
- `gameplay.css:1164`：`min-height: 82px` → **64px**，`grid-template-columns: 62px …` → **48px …**，
  `padding: 9px` → **7px**
- `gameplay.css:1171`：图标 `62px` → **48px**
- 算式：48 + 7×2 + 1×2 = 64，恰好咬合，且与详情页 `champions.css:882` 的 48px 对齐

**tooltip**：`gameplay.js:3514` 的 `<article>` 上补 `tabindex="0"` + `data-tooltip`，
文案复用 `wrapAugmentIcon`（`:3462-3468`）那套三段式（名称\n品质\n说明）；
**建议抽成 `augmentTooltipText(augment, fallbackID)` 供两处共用**。
`app.js` 的 `closest()` 委托天然兼容嵌套（图标命中内层，卡片其它区域命中外层，不打架），
**不需要动 app.js**。

⚠️ **会打到护栏**：`web/champions.test.cjs:4015` 断言 62px；`:4009` 断言
`.live-augment-column > div` 的 `gap: 8px` / `padding: 10px 11px 12px`。改 CSS 必须同步改断言。

> 唯一的真数据残缺（**不在本轮范围，别顺手改**）：KIWI 模式下 ID ≥ 1000 的海斗专属海克斯，
> Hexdata 抓取失败时 description 会是占位串「海克斯图鉴中可读取说明」
> （`champions.go:2128-2145` 有结案注释）。这条归 F 组。

## D-4 装备排行放到核心装右侧，一行两件

**DOM 现状**（`web/gameplay.js:3893`）：`.build-recommendation` 的直接子节点顺序是
header → `.build-summary-bar` → `${lateBands}`(棱彩,条件) → **`${itemBuildLayout}`(核心装)**
→ **`${itemRankingContent}`(装备排行,条件)** → `.build-apply-bar`。**两者是紧邻兄弟，核心装在前。**

**父容器是普通 block**（`gameplay.css:1294` 的 `.build-recommendation` 没有 display 声明），
"上下堆叠"纯粹是块级流默认行为，没有任何 grid/flex 在管。

**"一行只有一件"的直接原因**：`gameplay.css:1381` 的
`.live-item-ranking .mayhem-ranking-list { grid-template-columns: 1fr; }`
覆盖了基础规则 `champions.css:247` 的 `repeat(2,minmax(0,1fr))`（英雄详情页本来就是两列）。
而且配套的 `champions.css:249-250` 的 `nth-child(odd){border-right}` 都是按两列写的，
现在一列时视觉上是错的 —— **改回两列反而顺手修好**。

**改法（动一处 JS 模板加 wrapper）**。纯 CSS 方案因为 `lateBands`/`itemRankingContent`
都是条件渲染、`nth-child` 必然出错，不推荐。

`gameplay.js:3893` 把 `${lateBands}${itemBuildLayout}${itemRankingContent}` 改成：
```js
${lateBands}<div class="build-core-ranking-row">${itemBuildLayout}${itemRankingContent}</div>
```

CSS（写在 `gameplay.css:1344 / 1380` 附近）：
```css
.build-core-ranking-row { display: grid; min-width: 0;
  grid-template-columns: minmax(0,1.5fr) minmax(0,1fr); align-items: start;
  border-bottom: 1px solid var(--line); }
.build-core-ranking-row > .item-build-layout { border-bottom: 0; }
.build-core-ranking-row > .live-item-ranking { margin-top: 0; border: 0;
  border-left: 1px solid var(--line); border-radius: 0; }

/* 并排后右栏只有约 40% 宽，原卡片最小宽 418px 放不下，必须同时压缩内部网格 */
.live-item-ranking .mayhem-ranking-list { grid-template-columns: repeat(2,minmax(0,1fr)); }
.live-item-ranking .mayhem-ranking-list article { grid-template-columns: 20px 42px minmax(0,1fr); }
.live-item-ranking .mayhem-ranking-list article dl { grid-column: 3;
  grid-template-columns: repeat(3,minmax(0,1fr)); }
```
（`.live-ranking-icon` 的 46px 硬写在 `gameplay.css:1382-1383`，要一起降到 42px。
8 条 ÷ 2 列 = 4 行，与左侧核心装预览条数高度可比。）

**窄容器退化**（挂已有断点，**不要新增 `@media`**）：
```css
@container recommendation-area (max-width: 1080px) {
  .build-core-ranking-row { grid-template-columns: minmax(0,1fr); }
  .build-core-ranking-row > .item-build-layout { border-bottom: 1px solid var(--line); }
  .build-core-ranking-row > .live-item-ranking { border-left: 0; }
}
@container recommendation-area (max-width: 900px) {
  .live-item-ranking .mayhem-ranking-list { grid-template-columns: minmax(0,1fr); }
}
```

**动 JS 必须连带修的两处**：
1. `gameplay.css:1313-1315` 的 `.live-arena-build-section + .item-build-layout`
   依赖"棱彩块紧邻核心装"，包 wrapper 后失效 → 补 `.live-arena-build-section + .build-core-ranking-row`
2. `web/champions.test.cjs:3264` 用 `markup.indexOf('<div class="item-build-layout">')` 定位
   → wrapper 必须包在**外面**、保持这个精确子串不变。`:3290` 断言 `.item-build-layout { }`
   **不得**含 `grid-template-columns: minmax(0,1fr)` → 新列定义要写在 `.build-core-ranking-row` 上。

⚠️ 另注意 `gameplay.css:1519/1524/1531/1546` 和 `build-item-row.css:15/24/30` 是**匿名** `@container`，
绑 `.build-recommendation` 而非 `recommendation-area`。例如 `build-item-row.css:15` 的 980px
（核心装四列塌成单列）是按 `.build-recommendation` 宽度触发的，改并排必须一起算，
否则核心装列会在意料外的宽度塌陷。

## D-5 技能图标置灰 —— 两条真问题并存，R43 只修了 1/4

⚠️ **先排除"取错素材"**：后端 `loadChampionAbilities`（`gameplay.go:4544-4600`）
只取 `spell.IconPath` → `ImagePath` → `AbilityIconPath`，**全是彩色原图，没有任何
`_disabled`/`_gray` 变体路径**。这条可以彻底排除。

**(A) CSS 遗留**（`gameplay.css:1330`）：
```css
.skill-icon-button { color: var(--muted); background: var(--surface-strong); border: 0; }
.skill-icon-button.is-priority { color: var(--ink); background: color-mix(...); }  /* :1331 */
```
`is-priority` 只命中 1 个（主升），**其余 3 个吃基态的灰字灰板 = 禁用观感**。

**而且 R 永远拿不到 `is-priority`**：`gameplay.js:3937` 用 `skillPriority.indexOf("R")`，
后端 `skillPriority` 只含主/副/最后三个基础技能，`indexOf` 恒为 `-1` → **大招 100% 灰**。
`champions.css:484` 有现成的 `.skill-icon-button.is-ultimate`，但 **gameplay.js 从不输出这个类名**
（全仓仅 `champions.js:1706` 用过）—— R43 工单当时就点名了这条，到现在还是没用上。

**(B) 资源缺失**：`championAbilities` 只走 LCU（`gameplay.go:4455-4470`），
**没有任何 DDragon/CDragon 兜底**（装备/符文/召唤师技能都有 `proxyAsset` 的 `ddragon:` 分支，唯独技能没有）。
失败即 `abilities=[]` → `:3936` 走 `.skill-letter`；图片 404 时 `gameplay.js:4436-4437`
把 `<img>` 置 hidden，露出 `gameplay.css:916` 的灰底方块。

**排他判据（一分钟现场定位）**：F12 选中技能图标看 Computed 的 `filter` ——
`none` 则问题在图标本身(B)；`blur(1.5px) saturate(0.6)` 则是 `gameplay.css:1036` 的加载遮罩没退。
再看 `<img data-game-image>` 有无 `hidden`、Network 里 `/api/image?path=` 是否 404。

**改法**：
- **P0-CSS**：`gameplay.js:3940` 类名从二值改成分级 `is-ultimate` / `is-priority` / `is-secondary`；
  `gameplay.css:1330` 的 `color` 从 `var(--muted)` 提到 `var(--ink)`
  （**图标本体永远不该看起来禁用**，层级差异靠底色/描边表达）；让对局页真正复用 `champions.css:484`
- **P0-数据**：`gameplay.go:4544` 在 `err != nil` 或结果为空时，用 DDragon 英雄详情
  （`/cdn/{patch}/data/zh_CN/champion/{key}.json` 的 `spells[].image.full`）补一份，
  `IconPath` 写成 `ddragon:/cdn/{patch}/img/spell/{file}` ——
  `proxyAsset`（`gameplay.js:4344`）会自动走 `/api/champion-asset?source=ddragon`，**不用动前端**
- **P1**：修 `demo-data.js:590-593` 和 `:696-700` 两处假路径
  （`/lol-game-data/assets/demo-spell/*.png` 全仓不存在；`:696` 把 `iconPath: asset.path` 直接映射，
  **丢掉了 `ddragon:` 前缀** → 演示模式必 404）
- **P1**：`gameplay.css:1342` 之前补 `.skill-order .is-q b / .is-w b / .is-e b` 三色。
  现在这三个类名**有生成、CSS 里却没有对应规则**，12 格 Q/W/E 全是同一个近黑底 +
  同一个蓝字，只有 3 个 R 格是亮的。而战绩详情页的同类组件（`gameplay.css:901-905` 的
  `is-slot-1/2/3/4`）**做对了** —— 同一个应用里两条加点条不一致本身就是缺陷

⚠️ **护栏锁死了半成品**：`web/champions.test.cjs:3930`
```js
assert.deepEqual(classes, ["skill-icon-button is-priority", "skill-icon-button",
                           "skill-icon-button", "skill-icon-button"]);
```
三个裸类名被当成"正确答案"写进了护栏（同 R47-B-1 批评过的模式）。改完必须同步改断言。

---

# E 组（P1）：斗魂详情页

## E-1 ★自己那张卡的英雄头像现在就能修好（不需要任何新上游数据）

`/lol-champ-select/v1/current-champion` **已经在调了**（`gameplay.go:4340-4349`，
ChampSelect 分支无条件调用，斗魂下也会调），返回值落在 `response.CurrentChampionID`
（`gameplay.go:3070`，JSON 字段 `currentChampionId`）。

**但它只喂了推荐系统，没喂玩家卡片**。前端 4 处消费点全是推荐相关
（`web/gameplay.js:3005 / 3576 / 3835 / 3928`），而玩家卡头像走的是另一条：

```js
// web/gameplay.js:2916-2918
function liveDisplayedChampionId(player) {
  return Number(player?.championId) || Number(player?.championPickIntent) || 0;
}
```

**改法**：`isCurrent === true` 的那张卡，头像回退到 `data.currentChampionId`。
**至少能保证"自己随机到什么"立刻显示出来。** 队友的要等 E-2 的探测结果。

## E-2 ★随机英雄：`actions` 从来没解析过，必须先探测

`lcuChampSelectSession`（`gameplay.go:4157-4162`）只声明了 4 个顶层字段：
`gameId` / `localPlayerCellId` / `myTeam` / `theirTeam`。
`top_level_keys` 里的其余 30 个键（**含 `actions`**）全部被 `encoding/json` 静默丢弃。
全仓 grep `"actions"` / `actorCellId` / `isAllyAction` 在 `*.go` 和 `web/*.js` 里**零命中**。

`actions` 是 LCU 的动作数组，通常形如
`[[{actorCellId, championId, completed, type:"pick"/"ban", isAllyAction}]]`。
**如果里面的 pick action 带 championId，那就是随机英雄的答案所在** —— 但我们从没记过它的形状，
**这是彻底的未知数，按项目铁律必须先探测再写解析。**

### ★一个方法论陷阱，请务必看（这条会推翻我之前的一个判断）

真机日志里"CHERRY 的 championId 全是 0"这条证据**不可靠**。
`recordLCUChampSelectSessionShape`（`gameplay.go:3553-3559`）用
`claimBoundedDiagnosticKey` + key = `PHASE:GAMEMODE:QUEUEID`，
**整个进程生命周期只记一次**，而调用点在 `handleGameplayLive` 的 ChampSelect 分支
（`gameplay.go:4334`）—— 也就是**用户第一次轮询到选人阶段的那一瞬间，大概率还没人锁定**。

所以那条证据**无法区分**：
- (a) 国服客户端在斗魂选人阶段根本不暴露 championId
- (b) 采样时刻太早，所有人都还没锁

连自己小队里另外 2 个 VISIBLE 队友的 championId 也是 0，这更像 (b) ——
**队友的选择在客户端 UI 上是看得见的。**

**所以 A-1 部分我之前写的"championId 全是 0 所以拿不到"要翻案，这是采样偏差。**

### 探测方案（脱敏，复用现成函数）

在 `lcuChampSelectSessionShapePayload`（`gameplay.go:3581`）里增加 ——
`actions` 是二维数组，先用 `rawArrayField`（`gameplay.go:3780`）拆外层，
再对每个 group 拆一次并 append 成扁平数组：

| 字段 | 用什么函数 | 回答什么问题 |
|---|---|---|
| `actions_group_count` / `actions_flat_count` | `len()` | 外层组数、动作总数 |
| `actions_element_keys` | `diagnosticKeyUnion` | **到底有没有 championId** |
| `actions_nonzero_counts` | `diagnosticNonZeroKeyCounts`（`:3724`） | 每个键的非零个数，**关键指标** |
| `actions_type_counts` | `diagnosticEnumFieldCounts`（`:3707`） | type 枚举分布（pick/ban/…） |
| `actions_actor_cell_id_counts` | `diagnosticInt64FieldCounts`（`:3606`） | actorCellId 分布（cellId 非身份信息） |
| `actions_champion_id_counts` | `diagnosticInt64FieldCounts` | championId 分布（英雄 ID 是公开常量） |
| `actions_completed_counts` | `diagnosticFieldShapeCounts` | pick 是否已完成 |

**绝对不要记**：puuid / obfuscatedPuuid / gameName / tagLine / summonerId 的原值。
现有代码对这些一律只记形状（`string_empty` / `string_nonempty_length_N`），照抄。

**★采样时点必须改成多次**，否则这次探测会跟上次一样得出无法证伪的结论。
建议给 `lcuSessionShapeDiagnosticKey`（`gameplay.go:3543`）对 CHERRY 追加一个"进度桶"维度，
用不含身份信息的量做桶，例如 `myTeam` 里 championId 非零的人数（0 / 1-2 / 3 / 4+）。
一局斗魂最多多记 3~4 条，`lcuSessionShapeDiagnosticLimit = 128`（`:3525`）完全吃得下，
**能直接回答"是客户端不给，还是我们采早了"**。

**这一轮只做探测，不写解析。** 拿到真机结果再决定。

### 已经彻底结案的部分（写进注释，别再试）

- `obfuscatedPuuid` 真机 **18 个全是空字符串** → LeagueAkari 那条
  "HIDDEN + obfuscatedPuuid 非空 → 走闭源 native 反混淆"的路径**在国服连原料都没有**，
  隐藏的 15 人身份彻底拿不到
- `my_team_team_counts: {"1": 18}` → **team 字段确认是二值不是 1~6 小队号**，
  R47-A-3 的结案注释判断正确，探测目标达成
- `benchChampions` **length=0**，随机英雄不在这里
- 别的 champ-select 端点（`pickable-champions` / `bench-champions` / `my-selection` /
  `session/actions/{id}`）仓库里零引用、无真机证据，**同样不许臆断，要用先探测再实现**

## E-3 去掉"位置未知"（斗魂没有分路）

渲染点 `web/gameplay.js:3321`：
```js
<span>${escapeHTML(positionLabel(player.position))} · ${rank?.tier ? ... : "未定级"}</span>
```
文案表 `web/gameplay.js:4301`：`positionLabel` 兜底返回 `"位置未知"`。

后端侧确认它必然是空串：`gameplay.go:4433` 的 `normalizePosition` 在 lane+role 都为空时
`return ""`（`gameplay.go:6314-6315`），斗魂的 `assignedPosition` 就是空的。

**改法**：`renderLivePlayer` 目前不接收模式信息，但调用方 `web/gameplay.js:3549`
已经算好了 `const arenaMode = liveAugmentRecommendationSource(data) === "arena";`，
`:3553` 的 `.map((player, index) => renderLivePlayer(player, index))` 把它透传下去即可。
**斗魂时整段位置文案连同 ` · ` 分隔符一起去掉，只留段位。**

（`web/gameplay.js:3255` 的 `live-current-position` 有白名单守卫（`:3254`），
斗魂下本来就返回空串，**不用动**。）

## E-4 布局：当前玩家排小队第一 + 两列小队 + 提示醒目

用户要求三条：

1. **把当前玩家（自己）放在己方小队第一个** —— 现在自己排在第三个（用户图六）。
   排序在 `orderLivePlayers`（`web/gameplay.js:3285-3297`），R47-A-2 已经把
   `groupByTeam` 参数化了，斗魂下传 false。**再加一条：`isCurrent` 优先。**
2. **一行放两个小队，每个小队宽度为页面一半** —— `.live-teams.is-arena`
   （`gameplay.css:970`）现在是 `grid-template-columns: 1fr`（R47-A-2 做的单栏平铺）。
   改成两列。
   ⚠️ **但英雄选择阶段只有 1 个小队**，两列会让右侧空一大片。
   **建议：`grid-template-columns: repeat(auto-fit, minmax(320px, 1fr))`**，
   一个小队时自动占满、两个及以上时分列，同时挂已有的
   `@container recommendation-area (max-width: 1080px)` 退化成单列。
3. **提示要更醒目** —— 现在那行「斗魂英雄选择阶段客户端只公开己方小队，其余玩家进入对局后显示」
   （`web/gameplay.js:3516` 的 `.live-roster-notice`）样式太弱。
   **日志已经确认剩余小队数据拿不到**（`obfuscatedPuuid` 全空），所以这条提示是长期存在的，
   请给它加图标 + 边框 + 稍强的底色，做成正式的说明条而不是脚注。

> **进游戏后的 18 人仍然分不出 6 个小队**（`teamParticipantId` 真机 12 个取值不是 6 组，
> 已在 R47-A-3 结案注释里写死）。所以两列布局在进游戏后表现为"18 人平铺成两列"，
> 这是可接受的（比单列 18 行紧凑）。**不要为了凑 6 队去猜分组。**

---

# F 组（P1）：hexdata augments 熔断风暴

## F-1 ★rows=0 有了新根因，但**必须先探测列形状，不许猜着改解析器**

日志：`{"failure_kind":"parse","fields":627,"rows":0,"kind":"augments",
"buildId":"hexdata-2026-08-29-d4274259bdae"}` × 8 次。

**`buildId` 非空这条证据能排除掉一半可能。** `parseHexdataAugments`（`hexdata.go:1320-1352`）
三个出口里，只有最后一个（`:1347` 的 `len(rows) < 150`）会带非空 citation：
```go
document, buildID, err := parseHexdataDocument(data)
if err != nil { return nil, championSourceCitation{}, err }          // buildId=""
tables := primaryTables(document)
if len(tables) != 1 { return nil, championSourceCitation{}, errors.New(...) }  // buildId=""
...
citation := hexdataCitation(document, buildID, "/augments")           // :1346
if len(rows) < 150 { return rows, citation, errors.New(...) }         // ← 只有这个 buildId 非空
```
**所以：文档解析成功、恰好 1 个表、每一行都被 `:1337` 的 continue 淘汰了。**

```go
// hexdata.go:1331-1343
for _, cells := range tableRows(tables[0]) {
	if len(cells) < 2 { continue }
	href, name := firstLink(cells[0])
	path := hexdataAugmentPathPattern.FindStringSubmatch(hexdataLinkPath(href))
	metrics := hexdataGlobalMetric.FindStringSubmatch(hexdataNodeText(cells[1]))
	if len(path) != 3 || len(metrics) != 3 { continue }      // ← 淘汰点
```

**最可能的解释（是推断不是实证）**：只有 1 个表 + 627 个 `<td>`，
均匀列宽只有 **3 列 × 209 行**能整除（2 列→313.5、4 列→156.75、5 列→125.4 都不是整数）。
209 与 `design/mayhem-redesign/hexdata-fetch-policy.md` 里记的"208 海克斯"几乎吻合。
而 R46 的测试夹具里 augments 表是 **2 列**（`hexdata_test.go:194-197`）。
→ **`/augments` 表很可能从 2 列变成了 3 列，指标被挤出了 `cells[1]`。**

**但 path 失败（href 换成 `/zh/augment/...` 之类）同样能造成 rows=0，只是解释不了 3 列这件事。**

> ⚠️ **我没有去抓 hexdata.com.cn 验证** —— `design/mayhem-redesign/hexdata-fetch-policy.md` §0
> 记着 robots.txt 对 `GPTBot / ClaudeBot / CCBot` 是**全站 Disallow**。
> 探测必须由用户真机上的 EXE 在一次真实用户操作里完成。

### 探测方案（只记结构特征，不记页面原文）

在 `inspectHexdataPayload` 的 augments 分支失败时，额外发一条 `hexdata_table_shape`：
- `table_count`：`len(primaryTables(document))`
- `row_cell_count_counts`：`map[int]int`，每行 `<td>` 数量的分布
  （**直接回答"是不是 3 列 209 行"**）
- 逐列（cell index 0..N-1）各记：
  - `link_present_counts`：该列有没有 `<a>`
  - `link_path_head_counts`：`hexdataLinkPath(href)` 的**第一段**（`/augment`、`/hero`…），不记完整 path
  - `augment_path_match_counts`：`hexdataAugmentPathPattern` 是否命中
  - `text_class_counts`：把 `hexdataNodeText(cell)` 归类成布尔组合，**只记类名不记原文** ——
    `has_globalhexscore` / `has_胜率` / `has_percent_sign` / `pure_number` / `other`

有了 `text_class_counts`，"globalHexScore 在第几列、胜率在第几列、是不是被拆开了"一眼就能定，
同时能顺带验证 `hexdataHeroMetricPattern`（heroes 页）会不会是下一个受害者。

**本轮只加探测，拿到真机结果再改解析器。**

## F-2 ★熔断计数跨进程累加、永不衰减，退避阶梯事实上失效

`hexdata.go:876-900`：`circuit.Failures++`，**只有 `recordSuccess` 会清零**
（`:830-841` 的 `delete(h.state.Circuits, kind)`）。而 `h.state` 是**落盘持久化**的
（`saveStateLocked` → `atomicWriteFile`，`:320-329`），重启后除非 `Until` 距今超过 ±6 小时才丢弃。

**所以日志里的 `failures: 127` 是跨多次开关软件累加的历史总数，不是本次会话的失败次数。**
（本次会话真实失败 8 次：`hexdata_shape_invalid` × 8 == `hexdata_circuit_trip` × 8，
这些事件不在 `isNoisyDiagnosticEvent` 白名单里，不会被去重吞掉，所以 8 是可信的。）

退避（`hexdata.go:914-921`）：
```go
durations := []time.Duration{1min, 5min, 15min, 30min}
index := max(0, failures - hexdataCircuitFailureLimit)   // 127-3 = 124
return durations[min(index, len(durations)-1)]           // 夹到 3 → 恒定 30 分钟
```

**后果：用户人生第一次遇到解析失败之后，从第 6 次失败起就永远是最高档 30 分钟，
1min/5min/15min 三级阶梯事实上只在第一次生效。** 这才是"过于激进"的核心，
不是 `hexdataCircuitFailureLimit = 3` 本身。

**改法（请二选一并说明理由）**：给 `Failures` 加基于时间的衰减/上限；
或者在退避计算里用"最近一个窗口内的失败数"而不是历史总数。

## F-3 ★★探测器把用户触发型客户端变成了每 5 分钟叩门的定时爬虫（违反自家红线）

`hexdata_circuit_probe` **262 次** ≈ 131 次半开循环（一次循环打 2 条：start + success/failure），
**每次半开都是一次真实的公网请求**。

定时器路径在失败后会**无条件把自己续上**：`recordCircuitFailure` → `scheduleProbe(kind, now+5min)`
（`hexdata.go:889-895`）。所以熔断一旦打开就是**每 5 分钟一次、永不停止的后台请求，
而且熔断越久打得越久**。

**这直接违反 `design/mayhem-redesign/hexdata-fetch-policy.md` §1 红线第 4 条：
「不做启动预热、不做后台定时同步；所有请求必须由一次明确的用户操作直接触发」。**

**改法**：探测改成**惰性**的 —— 熔断期间不主动定时探测，
等下一次真实用户操作到来时，如果已过 `ProbeAt` 就借那次操作做半开试探。
这样既保留恢复能力，又不违反红线。

> 顺带记一条实现细节，值得写进注释免得后人误读：`probeTargets[kind]` 会被**每次 `load()` 覆盖**
> （`hexdata.go:547` → `rememberProbeTarget`）。对 hero/augment 这种"一个 kind 对应几百个 ID"的场景，
> 探测打的是**最后一次被请求的那个 ID 的页面**，探测成功还会 `recordSuccess(kind)` 清掉整个 kind 的熔断。
> 逻辑上说得通（形状全站统一），但这个假设应该显式写下来。

## F-4 影响面：augments 熔断会让海克斯说明全线为空

**"熔断按 kind 分离"这条现在仍然成立**（`h.state.Circuits` 是 `map[kind]`，
测试 `hexdata_test.go:688-690` 有断言），日志里 answer/heroes/hero 三个 kind 也确实还在成功。
唯一的跨 kind 耦合是全局限速（`hexdataGlobalGate` 并发 3 + 300ms 最小间隔），
失败的探测会占配额让正常请求略慢，但不会让它们失败。

**augments 熔断后受影响的两处**：

**(a) 海克斯图鉴页**（`/api/champions/augments`）—— 会降级到 OP.GG ARAM
（`champions.go:2000` 打 `hexdata_fallback`）。**页面不白屏**，但丢掉 hexdata 的
`globalHexScore` 排序和署名。

**(b) ★海克斯中文说明全线为空（更痛）** —— `mayhemAugmentSlugs`（`hexdata.go:1742-1774`）：
```go
page, err := p.hexdata.load(ctx, "augments", "all", "/augments", ..., false)
if err != nil { return nil }
```
拿不到 slug → 一个 `/augment/{id}-{slug}` 详情页都发不出去 →
**所有 ID ≥ 1000 的海斗专属海克斯没有任何中文说明**（就是 D-3 里提到的那个占位串）。
`hexdata.go:1612-1617` 的注释已写清：Riot 的 `cherry-augments.json` 没有 description，
CommunityDragon 的 `arena/zh_cn.json` 只到 ID 405，**hexdata 页面是这批文案的唯一来源**。

而且 `p.augmentSlugs` 只在成功时写入（`:1770-1772`），**失败不做负缓存**，
所以每次渲染都重试一次、每次都被熔断挡回、每次都打一对 `circuit_open` + `fallback` ——
日志里那 24 对事件就是这么来的。**顺手加个负缓存。**

## F-5 两处漏改的 href + 两处漏改的 fields

R46-C 的 `hexdataLinkPath` 只改了两个调用点（`:1288` hero 详情、`:1335` augments 榜单），
**还有两处没走**：
- `hexdata.go:1169`（`parseHexdataHeroes`）：`hexdataHeroPathPattern.FindStringSubmatch(href)`
- `hexdata.go:1377`（`parseHexdataAugmentDetail`）：同上

这两处今天还没炸（heroes 的 `rows=172` 说明 /heroes 页 href 目前是相对路径），
但只要上游给 hero 链接换成绝对 URL，`parseHexdataAugmentDetail` 的 `len(result.Rows) != 12`
硬校验（`:1385`）会立刻整页失败。**同一类隐患，统一掉。**

另外 R46-C 说要让 `Fields` 报真实字段数，但 `champions.go:1992` 和 `:1997`
**还在用 `len(rows)*4`**。这两个分支近似死代码（`:1971` 的 `pageErr == nil`
意味着 `checkedPage` 已做过形状校验），你看到的 `fields=627` 来自
`inspectHexdataPayload`（`hexdata.go:475`）。但既然目标是"Fields 报真实字段数"，
**这两处属于没改干净**。

---

# 交付要求

- `go build` / `go vet` / `go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **优先级**：A 组和 B 组是 P0（用户抱怨最强烈的两件事），先做完再动别的。
- **A-1/A-2/A-3 必须一起做才闭环**，单做任何一条都救不了。请在提交说明里写清楚你选了哪种预算方案及理由。
- **E-2 和 F-1 这两条只做探测，不写解析代码。** 这条纪律前几轮执行得很好，继续保持。
  探测要脱敏（只记形状/枚举/计数，不记身份原值），并且 **E-2 的采样必须是多时点的**，
  否则拿到的结论和上一轮一样无法证伪。
- **变异测试必做**：
  A-3（去掉单步超时）、A-4（把 timeout 归因回 no-position-sample）、
  B-2（去掉跨组去重）、B-3（去掉价格排序）、
  C-1（把徽章改回读 resolvedPosition）、C-2（去掉 roleRate 排序）、
  E-1（去掉 currentChampionId 兜底）。
- **本轮会打到至少 6 处现有护栏**（`champions.test.cjs:3223 / 3264 / 3290 / 3484-3510 / 3930 / 4009 / 4015`，
  `specialist_runes_test.go:462`）。**一处都不许删，全部改成断言新的期望值。**
  其中 `:3930`（技能图标类名）和 `specialist_runes_test.go:462`（25 秒常量）
  是典型的"锁死了错误/无意义的值"，改的时候顺便升级成行为断言。
- D 组用 Playwright 在 1440 / 900 / 420 三个宽度截图作为验收证据。
- 响应式规则**一律用 `@container`**，不要新增 `@media`（R47-ADDENDUM 刚统一过）。
  注意区分具名 `@container recommendation-area` 和匿名 `@container`（绑 `.build-recommendation`）。
