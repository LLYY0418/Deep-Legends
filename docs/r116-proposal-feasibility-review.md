# R116 方案可行性独立评审

评审对象：`docs/r116-mayhem-data-dimensions-proposal.md`
评审日期：2026-09-20（版本 0.12.7）
评审方式：三路子代理独立核对代码 + 本人联网实测 Hexdata 接口，代码为唯一判据
结论摘要：**方案的数据源层（R116-A）不仅可行，而且价值被低估了；P0 六项基本可行；但 P1 的核心前提被证伪，资源账算错了一个数量级。不需要新页面，需要一次页面内的信息架构重整。**

---

## 0. 五句话结论

1. **R116-A 定位错了。** `/api/hexdata/heroes/{id}` 的字段是我们现在抓的 HTML 页面 `/hero/{id}-{slug}` 的**严格超集**，而且带 `augmentDescription`（实测 126/126 满覆盖）。它不是「新增一个接口」，是「**替换**整条 HTML 抓取链路并顺手删掉一套 per-augment 扇出」。这一项能把单个英雄详情页的冷请求数从 `2+N`（N≈40）砍到 1，节省约 **97.6% 请求 / 85.7% 字节**。这是全篇价值最高的一条，方案里只字未提。

2. **P1 的论证前提（方案第 14 行）三项里有三项不成立。** 「只有我们能在选人/对局中拿到我方 5 人 + 对方 5 人 + **已选海克斯 + 当前阶段 + 已出装备**」——已选海克斯只在赛后、当前阶段没有任何通道、已出装备是「字段在响应里但代码从未解析」。**P1-1（海克斯三选一实时排序）的三个输入全部缺失，不可行**，不是「没实现」而是「没通道」。

3. **对位与协同的数据密度远低于预期。** 实测 `weakAgainst` / `strongAgainst` / `teammateSynergies` **每个英雄各只有 7 条**（且都是 `evidence:"supported"`、qValue=0 的显著项，不是截断，拉不出更多）。按 172 个英雄算，P1-3 克制提示在单局里至少命中一条的概率是 **34.6%**，P1-2 协同分是 **18.8%**。也就是说这两个卡片在 **65% / 81% 的对局里是空的**——按项目「拿不到就整块隐藏」的约定，用户多数时候根本看不到它们。

4. **资源账算错一个数量级，而且熔断保护对这条新链路失效。** 对局页按 10 个英雄各拉 1.2 MB，在现有节流下最坏排队 **11.0 秒**（ChampSelect 轮询周期只有 3 秒，前端推荐接口硬超时 20 秒），且超时走的是取消语义——`hexdata.go:783-785` 对取消直接 return，**熔断计数永远是 0**。结果是每 80 秒稳定烧 12 MB 上游带宽、永远拿不到数据、也永远不报警。正确形态是 `/hextech-insights` + `/postmatch` 两个全量文件（合计 0.46 MB、2 请求、按 buildId 缓存），**字节省 96.2%**。

5. **不需要新页面。** 全部可行的增量都落在两个既有容器里；唯一够格独立成页的 P1-1 恰恰是不可行的那一个。但英雄详情页会从 6 段涨到 9–10 段，**必须改成分 tab，否则是 R73「选手页死区」的重演**。

---

## 1. 联网实测结果（2026-09-20，本人执行）

全部带 `Referer: https://hexdata.com.cn/`：

| 接口 | 实测 | 体量 | 与方案对比 |
|---|---|---|---|
| `/api/hexdata/meta` | 200 | 8,043 B | 属实（方案写 7.8 KB） |
| `/api/hexdata/heroes/157` | 200 | 1,232,266 B | 属实（方案写 1.2 MB，精确） |
| `/api/hexdata/postmatch` | 200 | 90,058 B | 属实（方案写 89 KB） |
| `/api/hexdata/hextech-insights` | 200 | 373,350 B | 属实（方案写 372 KB） |
| `/api/hexdata/augments/1058` | 200 | 285,564 B | 属实（方案写 300 KB） |
| `/api/hexdata/heroes/157`（**不带 Referer**） | **403** `{"error":"data_not_public","message":"此入口不提供批量数据。请停止未经许可的自动化访问。"}` | 115 B | 属实，Referer 是硬要求 |

**方案第 3 节的接口清单与体量断言全部属实。** 未复验 `/api/hexdata/heroes` 列表接口——上游已明文警告「请停止未经许可的自动化访问」，不应再去触碰，方案把它列入「明确不做」是对的。

### 1.1 `/heroes/157` 顶层结构实测

```
items               118 条
trios              1000 条   （meta.heroTrioLimitPerHero = 1000，上游硬上限，不是采样）
augments            126 条   （126/126 带 stages[1..4]，126/126 带 augmentDescription）
weakAgainst           7 条   ← 方案未给数量
strongAgainst         7 条   ← 方案未给数量
teammateSynergies     7 条   ← 方案未给数量
terminalItemTrios  1000 条
summonerSpellPairs   28 条
```

### 1.2 三个方案没写、但会改变结论的实测发现

**（a）`augmentDescription` 100% 覆盖。** 样例：`"附近敌人在阵亡时有几率掉落【属性锻造器】。"`

这一条直接推翻了 `hexdata.go:1696-1701` 那段注释的前提。注释原话是「Mayhem 海克斯在 Riot 的数据里任何地方都没有描述，Hexdata 的海克斯页面是唯一存在渲染后中文文案的地方，所以用户正在看的那些行按需从它那里补水」——现在 JSON 接口直接给了，整套 `hydrateMayhemAugmentCopy` / `mayhemAugmentCopy` / `fetchMayhemAugmentCopy`（`hexdata.go:1702-1824`）可以删掉。

**（b）`items[]` 自带真实 `itemId`。** 现在 HTML 链路只能拿到中文装备名，靠 `decorateHexdataItems`（`hexdata.go:1619-1646`）做字符串反查，代码自己埋了 `hexdata_item_match` 诊断来数漏掉几条。换 JSON 后这套有损匹配可以整段删除。

**（c）对位/协同只有 7 条，且是显著性筛选后的结果。** 7 条全部 `evidence:"supported"`、`qValue: 0`，带 `confidenceLow/High`。这说明上游是**只发布通过多重比较校正的显著项**，不是「前 7 名」。我们拿不到更多，也不应该自己去放宽阈值。

---

## 2. 代码侧核对：方案里被证伪的条目

方案引用了约 60 处行号。绝大多数属实或差 1–6 行（代码在动，可接受）。以下是**实质性错误**，会影响工单设计：

| 方案断言 | 真实情况 | 影响 |
|---|---|---|
| 梯度徽章/总榜第 N 位来自 Hexdata heroes 表的 `tier`/`rank` 字段（champions.go:181-182） | **是本地算的**。`hexdata.go:1280`：`Rank: index+1, Tier: index*5/len(rows)+1`，而且自带 `TierLocallyCalculated: true` 标记 | P0-5 的「用官方档位替代本地自算」范围比方案想的更大——**tier 本身也是本地的**，JSON 的 `items[].tier` / `hexTier` 才是上游口径 |
| 「当前前端 0 处展示置信度」 | 不成立。`champions.js:1955-1959` 有 `lowConfidence` 分支会渲染「样本极少」徽记（限 OP.GG depth 路径）。Wilson / samplePolicy / tierLocallyCalculated 确实 0 展示 | P0-2 措辞要改成「0 处展示 Wilson 下界与样本分档」，否则验收时会因为找到那一处而判定「已存在」 |
| 开局配置「只显示选用率，无胜率」，暗示要改渲染逻辑 | 现象属实，但 `renderConfigOption(row, kind, showWinRate = null, ...)`（`champions.js:1900`）**已经是三态开关**，海斗只是传了 `false` | P0-4 成本从「中」降到「改一个实参」 |
| SGP 字段在 `sgp_api.go:893-897,1016` | 字段声明在 `gameplay.go:538/575/578-584/588-593/599-604/607-608` 与 `riot_api.go:633-638`；`sgp_api.go` 那两处只是端点 URL 拼装 | P2-1 的改动点定位错了 |
| `season_stats.go:169-171` 硬过滤 | 在 **167** 行 | 小 |
| 「前台 2 页 / 后台 12 页」，注释说每页 20 场 | `sgpPageSize = 50`（`sgp_api.go:77`）。**前台 2 页 = 100 场，后台 12 页 = 600 场** | P2 的扫描上限评估要按真实值重算；顺带该修那段错注释 |
| 决策 3 要求「磁盘预算复核（`binary_disk_budget.go`）」 | **指错文件**。`binary_disk_budget.go` 治理的是 `championDataCache.strictDisk`，而 `strictDisk` 只被图片缓存打开（`champion_images.go:21`）。season-stats 走 `storage.go:161-170` 的 `writeLocalStoreFile`，**无大小上限、无条目上限、无 LRU、无任何 prune** | 不是「复核」，是「**新建**一套 season-stats 预算」。这是 P2 的真实前置工作 |
| 遗留待确认项：`gameplay.js:5689` 应用装备方案按钮被置空是有意还是遗漏 | **在 5684 行，确认是有意**。`const action = capabilities.hasAugments ? "" : <footer…>` 是显式三元分支；git 历史从引入（`ff3cf27c`）到 `7eb6393c`（2026-09-13）条件从未动过，只改过文案；且该按钮本来就只在 ChampSelect 可用（文案「仅英雄选择阶段可应用」）。**缺的是一句解释性注释，不是修复** | 方案第 339 节的唯一待确认项可以关闭 |

另有一条方案自身的表述问题：第 62 行说「Hexdata 有 6 个 JSON 接口，我们只用了 `/api/hexdata/answer-cards` 一个」。**JSON 接口确实只用了一个，但我们同时还在抓 5 条 HTML 路径**（`hexdata.go:449-451` 白名单：`/heroes`、`/augments`、`/augment-rarity`、`/hero/{id}-{slug}`、`/augment/{id}-{slug}`）。这个表述掩盖了「JSON 可以替换 HTML」这个最大机会。

---

## 3. P1 的核心前提：逐项证伪

方案第 14 行是整个 P1 的论证基础。逐个输入核查：

| 输入 | 方案假设 | 实际 | 证据 |
|---|---|---|---|
| 我方 5 人英雄 | 可得 | **可得** | ChampSelect：`gameplay.go:6660-6724` `merge(session.MyTeam, 100)`；InProgress：`gameplay.go:5833-5847` |
| 对方 5 人英雄 | 可得 | **InProgress 可得；ChampSelect 未证实** | 仓库里 `theirTeam` 的 fixture **无一例外是 `[]`**（gameplay_test.go 6 处、r68_test.go 2 处），docs 里零真实观测 |
| **已选海克斯** | 可得 | **只在赛后** | `PlayerAugment1..6` 仅出现在赛后战绩解析（`gameplay.go:599-604`、`riot_api.go:633-638`）。实时结构体 `gameplayLivePlayer` / `lcuLivePlayer` / `lcuChampSelectPlayer` 里**完全没有 augment 字段** |
| **当前阶段 1-4** | 可得 | **无任何通道** | backend 里 `stage` 的非测试出现全是诊断阶段标签。唯一的海克斯阶段概念在统计侧（`/augment-rarity` 全服静态分布）。时间推断也不成立：`/liveclientdata/allgamedata` 只在斗魂调一次且只记形状（`arena_truth_diagnostics.go:307-335`） |
| **已出装备** | 可得 | **字段在，代码没读** | `/liveclientdata/playerlist` 的 19 键实测含 `items`（R90 的 63 次 200 采样恒定），但 `parseLiveClientPlayerList`（`gameplay.go:4557-4666`）只读 position / 身份键 / subteam 三样 |
| **系统给的 3 个候选** | 可得 | **完全不存在** | `/lol-cherry*`、`/lol-mayhem*`、`/lol-augment*` 命名空间在仓库里一次都没出现；`/lol-champ-select/v1/*` 只有 session/current-champion/pickable/bench/swap 等，无 augment；WebSocket 虽订阅全量 `OnJsonApiEvent`，但路由白名单只有 6 个 service（`lcu_events.go:160-220`），augment 事件从未被消费也从未被观测；playerlist 19 键里没有 augment，连 `<unknown-key>` 都没有 |

**这和 R95 斗魂小队分组是同一个坑。** 那次猜了六轮才发现客户端根本不下发字段，教训写在 memory 里是「推断类功能先建真值回路再谈算法」。P1-1 现在的处境更糟：不是「字段值猜不准」，是「连字段都没有」。

### 3.1 P1 逐项处置建议

| 项 | 判定 | 处置 |
|---|---|---|
| **P1-1 三选一实时排序** | **不可行** | 从 R116-C 移除。若一定要做，只剩「用户手动点选这一手的 3 个候选 + 当前阶段」的形态——要玩家在选择窗口内切窗口点三下，产品价值存疑 |
| **P1-2 阵容协同分** | **可行但覆盖率 18.8%** | `teammateSynergies` 每英雄 7 条。建议改成「有就显示、没有就整块隐藏」的低调形态，不要做成固定卡片，也不要算「阵容协同 +X%」这种听起来很全面的总分——**7/172 的覆盖率撑不起一个总分** |
| **P1-3 克制风险提示** | **可行但覆盖率 34.6%** | 同上。另外**只在 InProgress 出**，ChampSelect 的对方阵容零真实观测，想在选人阶段出必须先实测 |
| **P1-4 下一步出装** | **需先实测** | 改动很小（`parseLiveClientPlayerList` 多读一个 `items`），但海斗 KIWI 的 playerlist 形状**零观测**（19 键是在斗魂 CHERRY 下测的）。20 秒轮询对「推荐第 3 件」是够的（出装节奏以分钟计） |
| **P1-5 队伍画像缺口** | **可行** | 只依赖 championId + postmatch 静态指标，不碰任何缺失上下文 |
| **P1-6 表现指标面板** | **可行** | 纯英雄详情页静态数据，90 KB 一次全量，与局内上下文无关 |
| **P2-6 海克斯→阶段→出装联动** | **数据可行、形态不可行** | SGP SUMMARY 赛后确实同时给 `playerAugment1..6` + `item0-6`，自建统计库没问题；但方案第 277 行写的交付形态是「**局中**已知你选了哪些海克斯 + 当前阶段 → 直接推」——这个形态死在 Q2+Q3 上。只能做成英雄详情页的静态查询（「选了 X 之后通常出什么」） |

### 3.2 一条成本极低、能一次性收敛的探测

`objective_diagnostics.go:373-403` **已经实现了**拉 LCU `/swagger/v3/openapi.json`（16 MiB 上限）并提取 operations 的能力，现在只过滤 missions/notifications 前缀。把过滤器临时改成匹配 `augment|cherry|mayhem`，就能一次性穷举客户端到底有没有海克斯选择端点。这比再猜六轮便宜得多，也是回答「Q1 是否存在我们没发现的端点」的唯一权威手段。

同时建议打一局海斗导出日志，看三个**已有**诊断事件：`live_client_playerlist_shape.element_keys`（一次回答 P1-4 的 items 形状 + 海斗有无 augment 字段）、`lcu_champ_select_session_shape.their_team_length`（回答选人阶段对方阵容）、`live_client_allgamedata_shape.top_level_keys`（需先放开 `gameplay.go:6096` 的 `if arenaMode` 条件）。

---

## 4. 资源账：方案最危险的一项

按 R104 的算式方法论（`实体数 × 每实体请求数 ÷ 配额`）重算。

### 4.1 现有节流的真实吞吐

`hexdata.go:653-684` 的节流锁是**跨 sleep 持有**的，所以并发闸门 3（`hexdataGlobalGate`）只作用于 HTTP 在途阶段，进入 HTTP 之前所有请求必须串行穿过 `hexdataGlobalPaceMu`，每个强制付出 `300ms + U(0,800ms)`。

```
有效吞吐 = 1 / (0.3 + U(0,0.8)) = 0.91 ~ 3.33 req/s，均值 1.43 req/s
```

### 4.2 方案写法 vs 正确写法

```
方案写法（P1-2/P1-3 按 10 个英雄各拉 /heroes/{id}）：
  请求 10 × 1 = 10 次（单飞 key 是 {buildID}|{kind}|{id}，10 个不同 key，救不了）
  字节 10 × 1.2 MB = 12.0 MB
  排队 最好 3.0 s ／ 均值 7.0 s ／ 最坏 11.0 s ／ 含 1 次重试 21.0 s

  对 ChampSelect 3 秒轮询：       11.0 / 3.0  = 3.67×  超
  对前端推荐接口 20 秒硬超时：     21.0 / 20.0 = 1.05×  超
```

```
正确写法（/hextech-insights + /postmatch 两个全量文件，按 buildId 缓存）：
  请求 2 次（首次），0 次（同 buildId 内）
  字节 373,350 + 90,058 = 463,408 B = 0.46 MB
  排队 2 × 1.1 = 2.2 s（首次），0 s（后续）

  请求节省 (10-2)/10 = 80%（首次）、100%（后续）
  字节节省 (12.0-0.46)/12.0 = 96.2%
  2.2 s 装得进 3 秒的 ChampSelect 周期；11.0 s 装不进
```

`/hextech-insights` 的 `heroes[173]` 每条带 id / name / tier / games / winRate / pickRate / winRateChange / confidence / topItems[5] / topAugments[5]（含 rarity、reason、pairWinRate、deltaWinRate）——**对局页要展示的「10 个英雄的概览 + 推荐」，这一个文件全覆盖**。只有 P1-2/P1-3 需要的 `teammateSynergies` / `weakAgainst` 它给不了。

### 4.3 P1-2/P1-3 也不必拉 10 次

`weakAgainst[]` 本来就是「对手全表」（虽然只有 7 条）。**只拉本人英雄 1 次**，就能把「对面 5 人里谁克我」全部算出来；想要「我方谁克对面」才需要拉队友。建议第一版只拉本人英雄（1 请求），把覆盖率的话讲清楚，不要为了 34.6% 的命中率付 10 倍请求。

### 4.4 熔断保护对这条链路失效

这是比排队时长更危险的一点。超过 20 秒会被前端 `AbortController` 掐断（`web/app.js:359-364`），而 `recordLoadFailure`（`hexdata.go:783-785`）对取消直接 return：

```go
if err == nil || isCancellation(err) { return }
```

于是熔断计数永远是 0，链路进入「20 s 拉 12 MB → 掐断 → 60 s 退避 → 再来一遍」的循环：**每 80 秒稳定烧 12 MB 上游带宽，永远拿不到数据，也永远不报警。**

R102 的教训是「熔断被人当 bug 拆了」。这次是「熔断还在，但新链路从它下面绕过去了」。**工单必须明确：新增的大响应链路，取消也要计入失败预算，或者干脆别让它有机会超时。**

### 4.5 磁盘：`hexdata-` 前缀豁免预算

`pruneDiskLocked`（`champion_cache.go:424-470`）里：

```go
protected := strings.HasPrefix(item.Name(), "hexdata-") || strings.HasPrefix(item.Name(), "proseed-")
if protected { continue }        // 在 items = append(...) 和 total += info.Size() 之前
```

而 hexdata 的 cache key 是 `{buildID}|{kind}|{id}`，buildID 本身就以 `hexdata-` 开头，所以**每一个 hexdata 缓存文件都自动带这个前缀，既不参与 LRU、也不计入 total**。`championCacheMaxBytes = 64 MiB` 对它们完全不生效。

```
落盘体积 = 1.2 MB × 4/3（信封 json.Marshal 把 []byte 编成 base64）= 1.60 MB / 英雄
173 英雄全缓存 = 276.8 MB = 64 MiB 预算的 4.12×
× 每年约 24 个 patch，且没有任何按 buildID 回收旧文件的逻辑 = 6.64 GB / 年
```

单条 8 MiB 门槛（`championCacheMaxEntry`）挡不住 1.2 MB。**没有任何机制会报错，它会安静地涨。**

内存侧同样紧张：`championMemoryMaxBytes = 32 MiB`，而 `storeMemoryLocked` 是**先插入后驱逐**（`champion_cache.go:224-242`），插 10 个英雄（12 MB 裸体积、在途峰值约 68 MB）会把 catalog、OP.GG RSC、augment 页几乎全部挤出内存，随后触发重新拉取——缓存抖动。

### 4.6 换接口前必须先补的三个洞

| 洞 | 证据 | 不补的后果 |
|---|---|---|
| **没有 Referer 头** | `hexdata.go:693-695` 只设 Accept / Accept-Language / User-Agent；backend 非测试代码 grep `Referer` 零命中 | 实测不带 Referer 返 403；`fetch` 对 403 立即 break 不重试并计失败，**3 次就把新 kind 熔断掉，第一次上线就自爆** |
| **路径白名单会直接拒绝** | `hexdata.go:449-451` `allowedPath` 只放行 `/api/hexdata/answer-cards`、`/heroes`、`/augments`、`/augment-rarity` 和两条正则 | `/api/hexdata/heroes/157` 在**发出请求之前**就被 `"hexdata request path rejected"` 拒掉 |
| **新 kind 默认零校验** | `inspectHexdataPayload`（`hexdata.go:366-426`）的 switch 只有 6 个 case，**没有 default**，末尾直接 `return shape, nil` | 加新 kind 时忘写 case，**形状校验静默完全失效**——上游改字段不会被发现，坏数据直接进缓存并按 10 年 TTL（`hexdataCacheTTL`）落盘 |

补充：新 kind 必须记得在成功后调 `promote()`（照 `hexdata.go:1941`）。`refreshBuild` 路径用 `key+"|refresh"` 且 `persistDisk=false`（`hexdata.go:558-565`），**忘了 promote 就永远不落盘**，每 12 小时后第一次访问都回源。

还有一条隐藏放大：`state := h.snapshot()` 在并发 fetch 之前取，10 个英雄并发启动时全部快照到同一个过期的 `BuildChecked`，于是**12 小时后第一次打开对局页，所有请求全部强制回源，磁盘缓存一条都不生效**。

---

## 5. P2 自建统计：代价核算

放开队列过滤本身**代价很小**：

| 组成 | 变化 |
|---|---|
| `GameIDs` | **不变**（append 在队列过滤之前，`season_stats.go:653-655`，海斗场次早就在里面了） |
| `Stats` | **不变**（按 championID 聚合，上限 173 条） |
| `QueueStats` | +3 条 ≈ +0.3 KB |
| `RankedMatches` | +3 队列 × 40 条 × ~450 B ≈ +54 KB（前提是 `seasonRecordRankedMatch:211` 的过滤也一起放开） |
| **小计** | 约 60 KB → 120 KB，翻一倍 |

**真正的成本在决策 3 附带的那句「新增 augment 维度字段」**：

```
按对局存 playerAugment1..6 + item0-6 + 伤害/承伤/经济 ≈ 300 B/场 × 2000 场 = 600 KB
按 postmatch 那 22 项全量存                        ≈ 1 KB/场 × 2000 场 = 2.0 MB
```

而这个文件**每次前台扫描都要整文件 `json.Marshal` + 全量重写 + fsync**（`finishSeasonScan:676-690` 无条件 `saveSeasonStats`），总览页每次打开都会触发前台扫描。**2 MB 的同步全量重写挂在首屏路径上，在一个完全没有磁盘预算的目录里**——这才是 P2-1 的真实成本。

好消息：这条链路走 SGP（`sgp_api.go:621-739` 用 LCU 的 entitlements token 打腾讯 SGP），**全程不调用 `enterRiotLimitQueue`，不吃 Riot 的 90/2min 配额**。R104 的配额炸穿在这里不会重演。

---

## 6. 要不要新设计页面

**不需要新页面。需要一次英雄详情页的信息架构重整。**

三条理由：

**第一，可行的增量 100% 落在两个既有容器里。** 把第 3 节判定为可行的项归位：P0 六项全部落在英雄详情页右侧面板和推荐页现有 tab 内；P1-5/P1-6 是两个新 section；P1-2/P1-3/P1-4 是推荐页的条件卡片。没有一项需要独立的导航入口。

**第二，唯一够格独立成页的恰恰是不可行的那个。** 如果 P1-1 成立（局中实时三选一），它值得一个独占的、大字号的、能在 10 秒内扫完的专用视图——但它的三个输入全部没有通道。

**第三，反过来说，不重整会出事。** 英雄详情页现在是 6 段线性堆叠（推荐海克斯 → 开局配置 → 装备路线 → 装备排行 → 统计口径 → 海克斯图鉴，拼接在 `champions.js:1018`）。P0 加完收益率、置信度、阶段筛选，再叠上 P1-6 的 22 项表现指标（4 组），会变成 9–10 段。**这正是 R73「选手页密度未实质解决 / 死区」的复现路径。**

### 建议的结构

**英雄详情页：6 段线性 → 3 个 tab**

| tab | 内容 | 来源 |
|---|---|---|
| 概览 | 梯度/胜率/样本 + 推荐海克斯（带 deltaWinRate + 阶段 chips + 官方 hexTier 标签）+ 开局配置（补胜率） | `/heroes/{id}` |
| 构筑 | 装备排行（带「出 vs 不出」对照）+ 成装三件套 + 装备路线 + 技能加点 | `/heroes/{id}` + OP.GG RSC |
| 表现 | 22 项指标 4 组，每项带「较全英雄平均 ±%」 | `/postmatch` |

海克斯图鉴与稀有度分布是**全局**维度、不属于单个英雄，建议从详情页里拆出去，跟着现有 `/augments`、`/augment-rarity` 走独立入口。

统计口径说明不要做成第 4 个 tab——它应当作为**页脚常驻**，因为 P0-2 的整个意义就是让置信度信息随时可见，藏进 tab 等于白做。

> **已被 2026-09-22 用户指示撤销（R128 §2.3/§2.5）**：界面上不再添加「统计口径」、方法论与免责类说明，`shared.js`、详情页页脚与推荐页常驻页脚均已删除。本条不要再据此加回 UI；口径说明写在代码注释或 docs 里。

**对局推荐页：2 tab → 3 tab**

现状「海克斯与出装」+「详情」，建议加第三个「表现」（P1-5 队伍画像 + 双方英雄的 postmatch 指标对比）。P1-2/P1-3 的协同/克制因为覆盖率只有 18.8% / 34.6%，**不要做成固定卡片**，做成「命中才出现」的条目挂在详情 tab 里，符合现有「拿不到就整块隐藏」的约定。

P0-3 的统计口径说明同样常驻页脚（后端 `gameplay.go:5378` 已经返回，前端 grep `measurementTechnique` 零命中，纯遗漏，是整个方案里最便宜的一项）。

> **已被 2026-09-22 用户指示撤销（R128 §2.3）**：对局推荐区不再渲染常驻页脚，`measurementTechnique` 前端一律不消费（后端字段保留但标注「前端不展示」）。

---

## 7. 修正后的工单拆分建议

原方案的执行顺序是 A → B → E → C → D → F。建议改成：

| 工单 | 内容 | 与原方案的差异 |
|---|---|---|
| **R116-A′** | **替换**（不是新增）HTML 抓取链路为 `/api/hexdata/heroes/{id}`：加 Referer、扩路径白名单、给新 kind 写 `inspectHexdataPayload` case、补 `promote()`、解析层裁剪 trios/terminalItemTrios 到 top-N、**删除** `parseHexdataHeroDetail` 正则抓取 + `decorateHexdataItems` 名称匹配 + `hydrateMayhemAugmentCopy` 整套 per-augment 扇出 | 定位从「扩展」改成「替换」。**顺带修 `hexdata-` 前缀豁免磁盘预算的洞**，否则 1.6 MB/英雄会静默积累 |
| **R116-A″** | `/postmatch` + `/hextech-insights` 两个全量文件按 buildId 缓存 | 与 A′ 分开，因为它们是目录型全量文件，性质和 per-entity 完全不同，缓存策略也不同 |
| **R116-B** | P0 六项前端 | 基本不变。P0-4 成本下调（改一个实参）；P0-5 范围上调（tier 本身也是本地算的）；P0-6 明确只做手动 chips |
| **R116-C′** | P1-6 表现指标面板 + P1-5 队伍画像 + 英雄详情页 3-tab 重整 | **移除 P1-1**（不可行） |
| **R116-D′** | P1-2 / P1-3，只拉本人英雄 1 次，覆盖率话术写进文案；P1-4 需先有海斗 playerlist 实测 | 从「10 英雄并发」改成「1 英雄」 |
| **R116-E** | P2-1~P2-3 + P2-6（静态查询形态） | 前置改成「**新建** season-stats 磁盘预算」而非「复核 binary_disk_budget.go」；`sgpPageSize=50` 的真实值要写进算式；P2-6 的交付形态改成静态查询 |
| **R116-F** | P2-4 概率预测器 + P2-5 负向推荐 | 不变。平衡参数探测继续保持独立解耦 |
| **R116-探测** | 改 `objective_diagnostics.go` 过滤器穷举 LCU augment/cherry/mayhem 端点 + 打一局海斗导出三个已有诊断事件 | **新增，建议排在最前**。成本极低，能一次性收敛 P1 全系列的可行性 |

---

## 8. 需要用户拍板的三点

1. **P1-1 怎么处置**：直接删掉，还是先做 R116-探测（穷举 LCU openapi）确认「真的没有端点」再删？探测成本很低，但会推迟 R116-C。

2. **P1-2/P1-3 值不值得做**：覆盖率 18.8% / 34.6% 意味着大多数对局里这两块是空的。是按「命中才出现」的低调形态做，还是这一轮先不做？

3. **英雄详情页要不要在这一轮改成 3-tab**：不改，P0+P1-6 加完会变成 9–10 段线性堆叠；改，则 R116-B 的范围要扩大到页面结构，和「超范围改动」红线有冲突，需要显式授权。

---

## 附录：本次评审的证据来源

- 联网实测：`https://hexdata.com.cn/api/hexdata/{meta,heroes/157,postmatch,hextech-insights,augments/1058}`，2026-09-20，共 7 次请求（含 1 次无 Referer 对照），全部为单资源按需查询，未触碰已被上游 403 拦截的列表接口
- 代码核对：三路子代理独立核查，覆盖 `champions.js` / `gameplay.js` / `hexdata.go` / `champions.go` / `champions_structured.go` / `gameplay.go` / `aramkit_rating.go` / `season_stats.go` / `champion_cache.go` / `binary_disk_budget.go` / `lcu_events.go` / `live_prewarm.go` / `sgp_api.go` / `riot_api.go` / `storage.go`
- 历史交叉印证：`docs/history/worklists/WORKLIST-R90-ARENA-LIVE-GROUPING-REFRESH.md`（playerlist 19 键实测）、`WORKLIST-R95-ARENA-TRUTH-REFRESH-WIPE-FLICKER-PRO.md`（GameStart 308 ms）
- Hexdata 数据版本（本次取值）：`buildId = hexdata-2026-09-18-167d464cf859`，`reportPatch 16.18`，`reportDate 2026-09-18`，`sampleScope queueId 2400 / 7 大区`，`heroTrioLimitPerHero 1000`（方案调研时是 09-14 那一版，说明上游约每 4 天出一版）
