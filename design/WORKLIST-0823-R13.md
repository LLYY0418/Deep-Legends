# WORKLIST 0823-R13（加载性能根因 + W10 埋点结论闭合 + 出装布局改造）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
证据来源：用户真机导出的 `diagnostics.jsonl`（1297 行，覆盖 2026-08-23 06:38–07:22 共 44 分钟）。
所有行号为本次核对时的实测值，改前先 `grep` 确认锚点。

---

# 第一部分【P0】战绩加载不出来 / 一直转圈 的根因

## 日志实测总量

```
路由        请求数    总字节
RANKED       223 次    0.53 MB
SUMMARY      145 次  412.03 MB   ← 44 分钟内下行 412MB
SUMMONER       3 次    0.00 MB
```

单次 SUMMARY 响应 **2.6 ~ 3.6 MB**（最大 3,630,793 字节）。
最慢单次 4311ms。**55 次请求 `http_status: 0` / `error_kind: "other"`，
原因全是 `context canceled`**，集中在 06:38:26（51 次）。

峰值：**06:38:26 一秒内 118 次 SGP 请求**。

---

## A1【最高危】总览的 30 天窗口查询显式绕过了缓存

`gameplay.go:723`：

```go
infos, _, more, windowErr := a.sgp.matchHistoryOn(ctx, client, reference.ServerID, playerRef, 0, maximumSummaryMatchCount, false)
//                                                                                                                        ^^^^^ useCache = false
```

`sgp_api.go:469` / `:522` 两处都是 `if useCache` 才读/写缓存
（`sgpCacheTTL = 90 * time.Second`，`sgp_api.go:80`）——传 `false` 等于
**读也不读、写也不写，缓存对这条链路完全失效**。

配合 `gameplay.go:24 maximumSummaryMatchCount = 100` 与 `sgp_api.go:76 sgpPageSize = 20`：
**每次总览加载固定 5 页 × 约 2.8MB ≈ 14MB**，且每次都重新下载。

日志逐段核对（按 `recent_players_resolved` 分段）：

| 时间 | RANKED 请求 | SUMMARY 请求 |
|---|---|---|
| 截至 06:39:00 | 119 | 6 |
| 截至 07:05:39 | 80 | **57** ← 约 160MB |
| 截至 07:07:15 | 1 | 31 |

**要做**：

1. `gameplay.go:723` 的 `useCache` 改为 `true`。这条链路取的是"最近 30 天统计"，
   90 秒内重复读同一份数据没有任何正确性风险。
2. 该窗口查询与主战绩列表（`matches`）其实高度重叠——主列表已经取了前 20 场，
   窗口再从 `start=0` 重新拉 100 场。**应复用已取到的前 20 场，只补拉剩余部分**，
   或把两者合并成一次 `count=100` 的查询后在内存里切分。
3. `maximumSummaryMatchCount = 100` 配 `sgpPageSize = 20` 意味着必然 5 次串行往返。
   评估把 30 天窗口统计改成**按需加载**（用户滚到"活跃时段/最近一起玩"再拉），
   而不是每次进总览都无条件拉满 100 场。

**验收**：连续进出总览 5 次，SUMMARY 请求数从 ~30 降到 ≤6；单次总览下行从 ~14MB 降到 ≤3MB。

---

## A2【P0】match-tiers 逐场串行 + 每场十人，且多标签页叠加

`web/gameplay.js:1347-1362`（国服分支）是**逐场 `await` 的串行循环**，
每场 POST 一次 `/api/gameplay/match-tiers`、携带该场 10 个 `playerRef`；
后端 `rank_insights.go:229` 再以 `semaphore := make(chan struct{}, 4)` 扇出逐人查询。

20 张战绩卡 = 20 次串行请求 = **200 次 rankedStats**。
用户截图里同时开着 **3 个玩家页签**，各自渲染各自的战绩卡 → 再乘 3。
这就是 06:38:26 一秒 118 次请求、以及 51 次 `context canceled` 的来源
（前端重渲染/切页签导致 AbortController 取消，后端 `r.Context()` 随之取消，
`sgp_ranked_stats_failed` 记录的 `context canceled` 全部由此产生，
**不是腾讯网关的问题**）。

`a.rankScores` 缓存（`rank_insights.go:113`）按 `serverID|playerRef` 去重是对的，
但冷启动时全部未命中，且同一批并发请求之间没有 single-flight，
**同一个玩家会被并发重复查询**。

**要做**：

1. **跨场合并**：把当前容器内所有待解析战绩卡的 `playerRef` 先做一次全局去重，
   合并成**一次**（或按 `rank_insights.go:65 matchTiersMaxRefs = 24` 分批的少数几次）
   请求，而不是每场一次。后端已有 `matchTiersMaxMatches = 50`，说明批量契约本就存在。
2. **补 single-flight**：`playerRankScore`（`rank_insights.go:112`）对同一 `cacheKey`
   的并发调用要合并，参考 `riot_api.go:149 specialistFlights` 的现成写法。
3. **限制并发面**：只为**当前可见页签**解析平均段位；后台页签的战绩卡不发请求
   （`web/gameplay.js:1288` 处已有 `scope`/`container` 概念，加一个"是否活动页签"判断即可）。

---

## A3【P1】重进节流窗口偏短，放大了 A1/A2

R11 的 W6 把重进重载做成了 `shouldReloadOverview`（`web/gameplay.js:157`，阈值 20 秒）。
在 A1 未修之前，这等于**每 20 秒就可能重新下载 14MB**。
日志里 07:05–07:08 三分钟内出现了 6 次完整总览加载即为此。

**要做**：A1 修好（缓存生效）后，20 秒阈值可以保留；
但**必须确认缓存命中时不再产生 SUMMARY 请求**——用日志验证，不要只看代码。

---

## A4【P1】诊断埋点要能直接看出"这次加载花了多少钱"

现在要靠我把 1297 行日志聚合才能算出 412MB。建议新增一条
`overview_load_cost` 埋点，记录单次总览加载的：SGP 请求数、下行总字节、
缓存命中数、耗时。这样下次真机问题可以一眼定位，不用再做全量统计。

---

# 第二部分【P0/P1】W10 四条待日志确认项 —— 结论已出

## B1【P0】排位召唤师技能不显示：**字段确实没返回，不是目录问题**

`overview_loadout_shape` 共 7 条，**7 条全部是**：

```json
{"matches": 20, "spell1_present": 0, "spell2_present": 0}
```

20 场里 `Spell1ID > 0` 的**一场都没有**（判定逻辑在 `gameplay.go:850` 附近）。
如果是英雄目录加载失败，`spell1_present` 会是 20 而只是渲染不出图标；
现在是 0，说明 **SGP SUMMARY 响应里根本不含召唤师技能字段**。

**要做**：

1. 先确认 SGP SUMMARY 的参与者结构里是否有 `spell1Id`/`spell2Id`
   （字段名可能是 `summoner1Id`/`summoner2Id`，Riot Match-V5 用的是后者，
   `riotMatchInfo` 的 tag 目前是 `json:"spell1Id"`，很可能**只是 tag 名对不上**）。
   这是最可能的低成本修复点，**先查这个再谈换数据源**。
2. 若确认 SUMMARY 不含该字段，则在**详情页**链路（已有 match 详情请求）补取，
   或对当前登录玩家走 LCU `/lol-match-history` 补齐。
3. 补一条埋点记录**参与者对象的实际 key 列表**（只记 key 名，不记值），
   一次真机即可定论。

## B2【已闭合，无需改动】优劣势对抗正常

`counters_shape` 5 条：`in == out`，`dropped_no_meta = 0`，`dropped_no_play = 0`
（59→59、35→35、29→29，另有一条 0→0 属于该英雄本就无数据）。
用户截图里优劣势对抗也确实有数据。**此项从待办移除。**

## B3【P1】"场次最多的玩家"只有 3 个：上游解析就只有 3 行

`load_top_players_shape` 5 条，覆盖 nasus / kaisa / ryze / varus / nasus，
**`parsed` 全部等于 3**。

前后端上限本来就是 5（`web/champions.js:1580`、`champions_structured.go:322/337`），
**不要去改这两个常量**（我在 R11 就说过这是伪需求，日志现在证实了）。
瓶颈在 `champions_structured.go:342` 之前的解析：OP.GG 排行榜页面只解析出 3 行。

**要做**：抓一份真实的 OP.GG 英雄页面（用 R11 里验证过的 RSC flight 解析路径），
确认页面上到底有几行；若上游确实给了 5 行而正则只吃到 3 行，修正则；
若上游就只给 3 行，则把 UI 文案从"前 5 名"改成按实际条数展示，不要留空位。

## B4【P0】国服他人胜率缺失：**埋点记错了维度，需要补记数值**

`sgp_ranked_stats_shape` 169 条，`queue_keys` **169/169 都含 `losses` 与 `wins`**，
`queue_count` 恒为 7，`is_self` 为 false 的 151 条，`privacy` 多为 `UNKNOWN`（152 条）。

而 `ranked_winrate_resolved` 的分布是：

```
sgp  complete=false suppressed=true   229
sgp  complete=true  suppressed=false  111
lcu  complete=false suppressed=true    86
lcu  complete=true  suppressed=false   38
```

抑制逻辑在 `gameplay.go:1501 verifiedRankWinRate`：`wins > 0 && losses == 0` 就返回 `-1, false`。

**所以：字段名在，但值大概率是 0。** 这与既有认知（SGP 只对少数玩家回负场）一致，
但**当前埋点只记了键名没记数值，无法直接证实**——这正是我上一轮自己写下的教训又踩了一次。

**要做（按顺序，不要跳步）**：

1. `gameplay.go:1416` 的 `ranked_winrate_resolved` 补记 `wins` 与 `losses` 的**实际数值**
   （纯计数，无隐私风险），以及 `tier` 是否为空。
2. 拿一次真机日志确认 `losses` 是否恒为 0。
3. **确认后再改口径**，候选方案（不要提前实现）：
   - 若 `losses` 恒为 0 而 `wins` 有值：SGP 这条路对他人不可用，
     改为从该玩家的 SUMMARY 战绩里按赛季聚合胜负（W8 的 `season_stats.go` 已有现成聚合器）；
   - 若部分玩家有值：保持抑制，但 UI 上把"—"换成可解释的提示，而不是空白。

---

# 第三部分【P1】出装与推荐区域布局改造（用户本轮四项要求）

> 用户原话已逐条拆解如下。四项都属于布局，**不要顺手改数据口径**。

## C1 英雄详情页顶部区块重排（对应用户图六）

**现状**：左列"推荐符文"（大块），右列上"召唤师技能"、右列下"技能加点"；
"出装"是下面独立一块，其内部左侧才是"出门装 / 鞋子"。

**要改成**：

- **右列**（召唤师技能所在列）自上而下：召唤师技能 → **出门装** → **鞋子**；
- **左列**（符文所在列）自上而下：推荐符文 → **技能加点**；
- 顶部整块**高度相应加大**，让右列三块与左列两块视觉平衡；
- "出装"块内部因此**只剩出装路线与四五六件**，宽度全部让给 C2 的并列布局。

实现锚点：`web/champions.js` 的 `renderRuneWorkspace`（约 `:1353`）与
`renderRankedBuild`（约 `:1422`）；CSS 在 `web/champions.css` 的
`.rune-board-panel`（`:431`）、`.build-split` / `.build-side` / `.build-routes`（`:476` 附近）。

## C2【重点】英雄详情页四五六件必须并列成列，不是换行（对应用户图二 / 图三）

**现状问题**（用户原话："弄的什么玩意啊"）：右侧区域大片空白，
装备行被迫换行堆叠，整块下方还独占一整行。

**要改成**（图三红框示意）：出装区域是**一整行的多列网格**：

```
┌─────────────────┬──────────┬──────────┬──────────┐
│  核心装（路线）   │  第四件   │  第五件   │  第六件   │
│  3 件 → 列更宽    │  等宽     │  等宽     │  等宽     │
└─────────────────┴──────────┴──────────┴──────────┘
```

硬性要求：

1. 核心装那一列**更宽**（因为一条路线含 3 件装备），四/五/六件三列**彼此等宽**；
2. 若上游只给了第四、第五件（部分英雄如此），则只渲染 2 列且**两列自适应变宽**，
   不允许留空列；
3. **不允许换行**——每一列内部纵向堆叠自己的候选项，列与列之间横向并排；
4. 列高互相对齐，短的列不被拉伸变形。

参考 CSS 方向（`web/champions.css` 的 `.champion-item-depth-columns`，约 `:506`）：

```css
.build-item-row {
  display: grid;
  min-width: 0;
  /* 核心装列固定更宽，depth 列按实际数量等分 */
  grid-template-columns: minmax(0, 1.6fr) repeat(var(--depth-count, 3), minmax(0, 1fr));
  align-items: start;
}
```

`--depth-count` 由 `renderBuildDepthGroups`（`web/champions.js:1500`）按实际分组数写入行内
`style` 属性——**注意本项目 CSP 禁用内联 style**（见既有教训），
必须走 `data-depth-count` 属性 + CSSOM 设置，或直接给 2/3 两种情况准备两个 class。

## C3 对局内出装：核心装路线要 5 条，且套用 C2 同款布局（对应用户图四）

**现状**：`web/gameplay.js:2355` 已经是 `byWinRate(build.coreOptions || [], 5)`，
前端限额没问题；但截图里只显示 1 条 → **后端只给了 1 条**。

后端链路：`gameplay.go:2180` `result.Build.CoreOptions = recommendationOptions(detail.Build.CoreItems)`，
而 `detail.Build.CoreItems` 来自 `champions_structured.go:455/462`
`p.structuredMetrics(payload.Data.CoreItems, "item", 15)`。

**要做**：

1. 先查清对局内取到的 `CoreItems` 实际长度（加一条 `live_build_shape` 埋点记录
   `core_in` / `core_out`），确认是上游只回 1 条、还是中途被裁剪；
2. 按 OP.GG 数据补齐，**最多 5 条**；
3. 出装区域布局与 C2 完全一致（核心装列更宽 + 四五六件等宽并列，不换行）。
   对局内那份布局在 `web/gameplay.js:2377` 的 `item-build-layout`
   与 `web/gameplay.css:849-854`，**两处布局代码应尽量抽成同一套 class**，
   避免详情页改好、对局内又走样（这轮已经发生过一次）。

## C4 对局内顶部三块并成一行（对应用户图五）

**要改成**：`召唤师技能` → `出门装` → `鞋子` **三块同一行**（出门装居中），
出装路线**单独一行**。

⚠️ **需要确认**：图五里还有"技能加点"块，用户这次没提它的位置。
我按"技能加点单独占一行，放在三块之下、路线之上"实现；
若用户另有想法，以用户为准——**GPT 不要自行改动这条之外的排布**。

---

# 验收清单

- [ ] A1 `gameplay.go:723` 改为使用缓存；连进 5 次总览后 SUMMARY 请求 ≤6 次、下行 ≤3MB（**用新日志验证，不能只看代码**）
- [ ] A2 match-tiers 跨场合并 + single-flight + 只查活动页签；一次总览的 RANKED 请求从 ~119 降到 ≤30
- [ ] A3 缓存命中时确认不再产生 SUMMARY 请求
- [ ] A4 新增 `overview_load_cost` 埋点
- [ ] B1 先查 `spell1Id` 的 JSON tag 是否与 SGP 实际字段名一致；补参与者 key 列表埋点
- [ ] B3 抓真实 OP.GG 页面确认榜单行数；**不要改 5 这个常量**
- [ ] B4 `ranked_winrate_resolved` 补记 wins/losses 数值；**拿到日志前不要改口径**
- [ ] C1 详情页顶部：右列三块、左列符文+技能加点、整块加高
- [ ] C2 详情页出装：核心装列更宽 + 四五六件等宽并列，2 列时自适应，无换行、无空列
- [ ] C3 对局内核心装最多 5 条；布局与 C2 共用同一套 class
- [ ] C4 对局内顶部三块一行（出门装居中），路线单独一行
- [ ] `go test ./...` + `node --test web/champions.test.cjs` 全绿；`gofmt -l` 只剩既有的 `yourgg_arena.go`

**测试纪律**（本项目反复踩过）：

1. 跑测试用 `node --test web/champions.test.cjs`，**不能用 `node --test web/`**。
2. 布局类断言极易写成假护栏（只断言"元素存在"）。C2/C3 必须断言**列数与顺序**
   （如 2 列时 `grid-template-columns` 的实际取值、核心装列在最左、depth 列彼此等宽），
   并用变异体自验：把并列改回换行、把核心装列宽改成与其它列相同，两个变异体都必须被杀掉。
3. 变异测试拷副本时必须拷真副本（不能软链接），带上 `desktop/` 与 `prestige_chromas.json`；
   **跑变异体前先跑未变异的对照组确认 `# fail 0`**。
4. Go 里把条件改成 `if false` 会触发 "declared and not used" 编译错误 → 假杀。
