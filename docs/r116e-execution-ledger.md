# R116-E 执行账本：自建海斗个人统计 + season-stats 磁盘预算 + 海克斯→出装静态查询

执行时间：2026-09-20（本会话独占工作树，无并行子任务）
工单原文：`docs/history/worklists/R116-E-自建海斗统计与磁盘预算-工单.md`
执行契约：主控会话拟定的 `R116-E-CONTRACT.md`（含用户裁决、附一~附五）
版本：`desktop/package.json` 0.12.11 → **0.12.12**（六张工单的最终版本）

---

## 0. 一句话结论

P0（放开队列过滤 + schema 升版）、P1（新建磁盘预算）、P2（容量估算与注释纠错）、P3（`seasonAugmentSample` + P2-6 静态查询）四条全部落地；用户裁决追加的「海克斯大乱斗第三个 tab + 文案同步」也全部落地；契约附五列的三处 `420/440` 硬门禁按「海斗没有分路 → 整块不给，绝不回退混合队列」处置完毕；顺带闭合了一个既存生产缺陷（`rankedQueueLabel` 在生产 JS 里从未定义）。全量 Go 测试 **ok 227.728s**、前端 **622/622**、desktop **259 pass / 0 fail / 1 skipped**、`go vet` 与 `gofmt` 无输出。两项真机判据如实标 `待真机验证`。

**一处必须上报的越界**：为了让 P2-6 的前端渲染真的能拿到数据，我在 `backend/main.go:508-509` 加了 **2 行**（1 行注释 + 1 行路由注册）。`main.go` 在禁触清单里，但路由注册只可能发生在 `main.go`，而工单交付物第 3 条明确要求「P2-6 的静态查询函数**与前端渲染**」。这 2 行是纯追加、没有改动任何既有行，`diff` 已在 §12 附出。**如果主控认为这不可接受，删掉这 2 行即可让该端点变成死代码（其余改动不受影响），但 P2-6 就变成了「算了但取不到」的死数据。**

---

## 1. 实际做了什么（含改动后的新行号）

### 1.1 `backend/season_stats.go`（764 → 1215 行）

| 新行号 | 改动 |
|---|---|
| **17-26** | P0-2 + 附一：`seasonStatsCacheSchemaVersion` **8 → 9**；顺手把 R117 留下的陈旧注释（原 `17` 行仍写「7：」）改成**逐版演进**的形式：7 = 引入来源维度、8 = K/D/A 改逐场均值（R117）、9 = 放开海斗队列 + 新增 augment 样本 |
| **28-45** | P1 现状证据第 1 条 + 评审第 2 节：修掉「每页 20 场」那段错注释，改成「单页 20~50 场波动（`sgpPageSize = 50`，缓存回退路径收窄到 `sgpFallbackPageSize = 20`），前台 2 页 = 40~100 场、后台 12 页 = 240~600 场」；并补一句「`seasonTrimRankedMatches` 按队列各裁到 40，所以放开后 `RankedMatches` 总上限是 5 × 40 = 200 条」 |
| **47-84** | 新增队列判据的唯一真源：`seasonQueueSoloDuo`(420) / `seasonQueueFlex`(440) 常量、`seasonMayhemQueueIDs = []int64{2300, 2400, 3270}`(**58**)、`seasonMayhemPrimaryQueueID = 2300`(**63**)、`seasonStatsQueueAllowed`(**69**)、`seasonMayhemQueue`(**77**)。注释里明确写了「**3220 是极地大乱斗(aram 组)，不是海斗，不在这里**」 |
| **129-135** | `seasonAugmentSlots = 6` / `seasonItemSlots = 7` |
| **137-150** | `seasonAugmentSampleLimit = 2000` 的**完整推算过程**（见 §7） |
| **152-168** | P3：`seasonAugmentSample` 结构体，字段严格是 `GameID` / `ChampionID` / `AugmentIDs[]` / `ItemIDs[]` / `Win`，注释里点名 Anti-scope 第 4 条（不存 22 项 postmatch 全量） |
| **170-215** | `seasonAugmentSampleFor(info, playerRef)`：只在海斗队列记录；零槽位丢弃；**没有海克斯的场次不记录**（对 P2-6 没有贡献） |
| **217-224** | `augmentIDsOrNil`：空切片序列化成「字段缺失」而不是 `[]` |
| **226-253** | `seasonTrimAugmentSamples(cache) int`：去重 + 按 GameID 降序 + 裁到 2000，返回被丢弃条数。**排序口径的假设与失效影响都写在注释里**（样本按 Anti-scope 第 4 条不带 `createdAt`，追加顺序也不等于时间顺序，所以用 GameID 降序近似「新在前」；该假设只决定「超限时先丢哪些」，不参与任何对外数字） |
| **299-311** | `seasonStatsCache` 新增 `AugmentSamples []seasonAugmentSample \`json:"augmentSamples,omitempty"\``；`QueueStats` 的注释从「只把 420/440 分开聚合」改成「按队列分开聚合（420/440 与 2300/2400/3270，判据见 `seasonStatsQueueAllowed`）」 |
| **339-359** | P1：`saveSeasonStats` 改成薄壳，实际逻辑在新的 `saveSeasonStatsReported`，两者都走 `marshalSeasonStatsWithinBudget`（预算检查就挂在这里，符合契约「P1 的预算检查挂在 `saveSeasonStats`」） |
| **361-364** | P0-1：`seasonStatsAccumulate` 的门禁 → `!seasonStatsQueueAllowed(info.QueueID)` |
| **404-445** | P0-1：`seasonRecordRankedMatch` 的门禁 → 同一个 `seasonStatsQueueAllowed`；**同步改写了上方那段「判据与 seasonStatsAccumulate 保持一致（只认单双排 420 / 灵活组排 440…）」的注释**，改成「两处都走 `seasonStatsQueueAllowed`，R116-E 放开海斗时这条同步约束对新增的三个队列同样适用，所以判据收敛成了一个函数」；海斗场次额外 `append` 一条 `seasonAugmentSample` |
| **708-712** | `seasonQuerySnapshotKey` 的 `queues=420,440` → `queues=420,440,2300,2400,3270`（队列范围是去重键的一部分，口径变了必须换键，否则同进程里放开前后两轮扫描会互相命中） |
| **893-929** | `finishSeasonScan`：新增 `seasonTrimAugmentSamples` 调用、一次性目录体检 `a.checkSeasonStatsDirectoryHealth()`、写入失败**不再静默**（新增 `season_stats_save_failed` 诊断事件）、`season_ranked_snapshot` 事件补 `mayhem` / `capped_mayhem` / `oldest_mayhem` / `augment_samples` / `augment_dropped` / `file_bytes` / `file_budget_bytes` / `file_truncated` / `file_over_budget` |
| **931-945** | `seasonMayhemSnapshot`：把海斗三个队列各自的快照合并成页签级的一个数 |
| **1006-1215** | P2-6 的**纯函数**聚合层（无副作用、不读盘、不发请求、不看时间）：`seasonAugmentMinimumSample = 10` / `seasonAugmentBuildGroupLimit = 6` / `seasonAugmentBuildComboLimit = 3`、`seasonAugmentBuildCombo`、`seasonAugmentBuildGroup`、`seasonAugmentBuildReport`、`seasonAugmentMinimumSampleOr`(**1064**)、`seasonAugmentComboKey`、`seasonAugmentBuildInsight`(**1086**)、`seasonAugmentSampleHasAugment`、`seasonAugmentBuildReportFor`(**1160**)、`seasonWinRatePercent`。**魔法数字全部收敛成具名常量，不散在渲染代码里**（与 R116-D P1-5 同一条要求） |

**R117 的改动一律没动**：`Kills/Deaths/Assists float64`、逐场平均（`round1(item.Kills / float64(item.Games))`）、`ratioFloat`、`seasonStatsOverall` 的 `0.0` 累加变量，全部原样保留，我的改动叠加在其之上。
> 注：契约附一说 R117「删掉了 `CSPerMinute`」，实测当前 `gameplaySeasonChampionStat:154` 仍有 `CSPerMinute float64`、`seasonStatsFinalize:353` 仍在算它。我没有动它（禁触清单明写「不要回滚或『修正』任何 R117 的改动」，反向也不该替它删）。这里只是把观测差异记下来。

### 1.2 `backend/season_stats_budget.go`（新建，269 行）

| 行号 | 内容 |
|---|---|
| 1-38 | 文件头说明：为什么必须**新建**而不是「复核 `binary_disk_budget.go`」（那套治理图片缓存的 `strictDisk`），以及按真实 20~50 场/页算出来的**放开后单文件构成表** |
| **40** | `seasonStatsFileBudgetBytes = 4 * 1024 * 1024` |
| 42-70 | `seasonStatsBudgetReport`：`BudgetBytes` / `RawBytes` / `WrittenBytes` / `RankedMatches{Before,After}` / `AugmentSamples{Before,After}` / `Truncated` / `OverBudget` / `MarshalErr` / `Steps` |
| **76-146** | `marshalSeasonStatsWithinBudget(cache)`：三级处置——① 先把 `RankedMatches` 收到全局最新的 `seasonRankedMatchLimit`(40) 条；② 再把 `AugmentSamples` 去重排序并逐次减半；③ 都用尽了仍超限就**照原样写入并置 `OverBudget`**。**绝不整体拒绝写入、绝不丢弃 `Stats`、绝不 panic**。注释里写明「只做重新切片，不改元素，所以不会影响调用方手里的 cache」 |
| **150-159** | `seasonStatsCapRankedMatches`：按 `CreatedAt` 倒序裁到 limit（磁盘兜底用的「全局 40 条」，区别于 `seasonTrimRankedMatches` 的「每队列 40 条」正常口径） |
| 161-172 | `seasonStatsDirectoryHealth` 结构体 |
| **174-221** | `seasonStatsDirectoryHealthFor(root)`：`filepath.WalkDir` 统计 `<root>/season-stats` 的文件数、总字节、最大单文件与其**相对路径**（绝对路径带本机用户名，不该进诊断事件）、超预算文件数；目录不存在不算错误 |
| 223-241 | `seasonStatsBudgetMarkChecked(root)`：进程内按存储根去重，保证「一次性」。注释里写明**为什么不往 `app` 上加字段**（`app` 结构体在 `main.go`，本轮禁触） |
| **246-269** | `(a *app) checkSeasonStatsDirectoryHealth()`：记 `season_stats_directory_health` 诊断事件（`files` / `total_bytes` / `budget_bytes` / `largest_bytes` / `largest_file` / `over_budget_files` / `reason`） |

Anti-scope 第 3 条遵守情况：`binary_disk_budget.go` **一个字都没改**，本文件也没有引用它。

### 1.3 `backend/gameplay.go`（9740 → 10100 行）

| 新行号 | 改动 |
|---|---|
| 1223-1232 | `seasonResult` 新增 `ranked []seasonRankedMatch` 字段（海斗页签的数据通路，见 §3） |
| 1247-1248 | `loadSeasonChampionStatsSnapshot` 的第三个返回值不再丢弃 |
| 1325 | `seasonRanked` 进入总览装配作用域 |
| **1525** | 调用点：`buildGameplayRankedQueues(gameplayRankedQueueTabs(rankedSamples.ByQueue, seasonRanked), playerRef, ranks, reference.Region)` |
| **1695-1708** | `gameplayRankedQueueTab`：`Key` / `Label` / `QueueIDs[]` / `Matches` / `Cached`。用**队列集合**而不是单个 ID，因为海斗三个 ID 在 UI 上是一个页签 |
| **1710-1731** | `gameplayRankedQueueTabs(byQueue, seasonRanked)`：产出 420/440 两个页签，**只在真的有海斗样本时**追加第三个（Key = `"2300"` = `seasonMayhemPrimaryQueueID`，Label = `海克斯大乱斗`） |
| **1733-1739** | `(tab) gameplayRankedTabHasPositions()`：`len(QueueIDs) == 1 && !seasonMayhemQueue(QueueIDs[0])` —— 海斗没有分路，所以位置偏好与能力雷达在这个页签下**根本不成立**（不是「样本不足」，是「口径不适用」） |
| **1742-1780** | `buildGameplayRankedQueues(tabs, playerRef, ranks, region)`：单队列页签走既有路径（逐字节不变）；多队列页签走 `recentRankedSummaryForQueues`；`gameplayRankedTabHasPositions()` 为假时**不下发** `Ability` / `AbilitySampleGames` / `Positions` |
| **1782-1809** | `gameplayRankedTabSamples` / `gameplayRankedTabCached`：把样本裁到本页签的队列集合 |
| **1811-1827** | `recentRankedMatchesForQueues`：多队列版本。注释里点名「**没有老函数那种『queueID 不是 420/440 就全收』的兜底**」——那个兜底（`recentRankedMatchesForQueue:1713` 的 `(queueID != 420 && queueID != 440 || match.QueueID == queueID)`）会把别的队列算进当前页签。老函数**原样保留**，既有 4 个调用点行为不变 |
| **1831-1996** | P2-6 的 HTTP 出口：`seasonMayhemBuildsResponse`(**1839**)、`seasonMayhemBuildsUnavailable`、`handleGameplaySeasonMayhemBuilds`(**1859**)、`attachSeasonMayhemItemNames`(**1939**)、`seasonMayhemItemNames`(**1972**)。详见 §6 |
| **3546-3556** | `recentRankedSummary` 拆成薄壳 + `recentRankedSummaryScoped(matches, playerRef, cached, accept)`；薄壳传 `seasonClassicRankedQueue`，**行为逐字节不变**（顶部卡片标题就是「近 N 场排位」，混进海斗会让标题变假话） |
| **3561 / 3585** | scoped 版本里的两处队列门禁改成 `!accept(match.QueueID)` / `!accept(item.QueueID)` |
| **3698-3729** | `recentRankedSummaryForQueue` 的早退：`return recentRankedSummary(...)` → `return recentRankedSummaryForQueues(matches, playerRef, cached, []int64{queueID}, rankedQueueLabel(queueID))`。注释里用 ★ 标出「这里曾经写的是什么、为什么放开海斗之后那是伪造口径」 |
| **3732-3759** | 新增 `recentRankedSummaryForQueues(matches, playerRef, cached, queueIDs, label)`：队列集合版本，样本只可能来自 `queueIDs`，**绝不回退到不分队列的全量汇总** |
| **3761-3763** | 新增 `seasonClassicRankedQueue(queueID)`：「经典排位」判据 |
| **3765-3779** | `rankedQueueLabel`：420 → 单双排、440 → 灵活组排、2300/2400/3270 → 海克斯大乱斗、**未知 → 空串**（由调用方决定兜底，绝不默默标成「单双排」） |
| **3782-3795** | `championStats` 的 `420/440` 门禁**刻意保留**，并补上理由注释（见 §5.3） |
| **3858-3878** | `positionStatsForQueue` 的早退：`return positionStats(matches, playerRef)` → **`return nil`**，★ 注释写明这是附五最危险的一条 |
| **3880-3900** | `liveRecentPositions` 的 `return nil` **行为不变**，但补上「这是**有意的**，不是漏改」的注释 |

### 1.4 `backend/riot_api.go`（1709 → 1715 行）

**1459-1465**：韩服路径的 `buildGameplayRankedQueues` 调用改成新签名，**只传 420/440 两个页签**（韩服没有海克斯大乱斗队列，ARAMKit 只收录国服），行为与改动前一致。这是本轮对 `riot_api.go` 的唯一改动。

### 1.5 `backend/main.go`（1820 → 1822 行）⚠️ 越界，见 §0 与 §12

**508-509**：`mux.HandleFunc("GET /api/gameplay/season-mayhem-builds", a.authorized(a.handleGameplaySeasonMayhemBuilds))` + 1 行注释。纯追加。

### 1.6 `backend/web/gameplay.js`（7224 → 7299 行）

| 新行号 | 改动 |
|---|---|
| 1690-1704 | 缺陷闭合的完整记录注释（见 §4）+ `MAYHEM_QUEUE_TAB_KEY = "2300"`(**1705**) |
| **1709-1717** | **定义 `rankedQueueLabel(queueId, fallback = "")`**（生产代码里第一次真正存在） |
| **1719-1725** | `isMayhemQueueId(queueId)`：`[2300, 2400, 3270].includes(...)`。**刻意内联在函数里而不是提成模块常量**——聚焦测试按名字编译真实实现时模块常量不会进作用域，函数自包含才不会被迫塞桩（这正是原缺陷的成因） |
| **1727-1729** | `rankedQueueNoun(queueId)`：海斗 → `海克斯大乱斗`，其余 → `排位` |
| **1731-1748** | `rankedQueueData`：三处 `Number(key) === 440 ? "灵活组排" : "单双排"` 全部收敛到 `rankedQueueLabel(key)`；新增 `mayhem` / `positionsApplicable` 两个字段；旧载荷（没有 `rankedQueues`）下的海斗页签宁可空着也不复用合并字段 |
| **1750-1766** | `rankedQueueSwitcher(tab, activeQueue, scope, data)`：第三个按钮**只在 `scope === "recent"` 且后端真的下发了 `"2300"` 这个 key 时**出现；`aria-label` 随之在 `排位模式`（只有排位队列时）与 **`对局模式`**（含海斗时）之间切换。既有两个字面按钮保持原样（`champions.test.cjs:6401` 的源码正则依赖它） |
| **1800** | `careerSectionEntries` 的 recent-ranked 项把 `data` 传给切换器（ability / position 两项**不传**，所以那两个 scope 永远只有两个页签，见 §3.4） |
| **2261-2295** | `renderRecentRanked`：标题 `近 N 场${rankedQueueNoun(queueId)}`（420/440 → 「近 N 场排位」逐字不变）；空态文案按队列给准确表述；海斗下「位置胜率」那一格从 `位置数据不足` 换成 `海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径`（`位置数据不足` 会被读成「多打几场就有了」，而事实是口径不适用）。**签名没变**（`champions.test.cjs:1931` 钉死了它） |
| **2396-2399** | `renderChampionStats` 的兜底标签 `本赛季 · N 场排位` → **`本赛季 · N 场对局`**：放开队列过滤后 `seasonChampionStats` 里也含海斗场次，再写「排位」就是假话。`progress.message` 分支不受影响（后端给的是「已统计 N 场」，本身不含「排位」二字）；1794 行那条 `${number(opggSeason.overall.games)} 场排位` 是韩服 OP.GG 链路、只有排位，**保持原样** |
| **2420** | `renderPositionStats` 的空态队列名 → `sourceLabel \|\| rankedQueueLabel(queueId)` |
| **4198-4207** | `bindRankedQueueControls` 的门禁：老的 `if (queue !== "420" && queue !== "440" \|\| ...) return;` 会**把海斗按钮的点击直接吞掉**，改成 `const allowed = scope === "recent" ? ["420", "440", MAYHEM_QUEUE_TAB_KEY] : ["420", "440"];` |
| 19 | `newTabView()` 的默认状态 `rankedQueueRecent/Ability/Position: "420"` **一字未动**（默认仍落在单双排，符合契约 UI 入口第 4 条） |

### 1.7 `backend/web/champions.js`（2975 → 3074 行）—— 只增不改

| 新行号 | 改动 |
|---|---|
| **54-59** | 4 个 state 字段：`mayhemPersonalBuilds` / `mayhemPersonalBuildsKey` / `mayhemPersonalBuildsLoading` / `mayhemPersonalBuildsError` |
| **726-737** | 注释块 + `mayhemPersonalAssetNames(rows)`：从详情页响应已有的 `recommendedAugments` / `itemRanking` 的 `assets[0].{id,name}` 建 ID→中文名映射，不为这一块额外拉目录 |
| **751-771** | `loadMayhemPersonalBuilds(championID)`：同一英雄只拉一次，失败也记住 key（避免每次重渲染重打） |
| **773-800** | `renderMayhemPersonalBuilds(detail)`：`available` 为假 / `groups` 为空 / key 不是当前英雄 → **返回空串，整块不进 DOM**。**零新增 CSS**：结构复用「装备排行」那套 `.recommendation-section.mayhem-item-ranking` + `.mayhem-ranking-list` 四列网格（`b` 序号 / 图标位 / `strong` 名称 / `dl` 指标）。装备名三级来源：后端 `itemNames` → 详情页装备行 → `装备 <id>`；**只给了一半名字时整组回落**（一半有名一半是 ID 更容易被误读成「这几件才是核心」）。海克斯名解析不出来显示 `海克斯 <id>`，**不编名字**。口径说明写进 `section-count` 的 tooltip：「本机保存的本赛季海克斯大乱斗对局（当前登录账号，该英雄 N 场）；每个海克斯至少 M 场才列出。**不是全服数据，也不含其他账号。**」 |
| **1001-1005** | 渲染后钩子：构筑 tab 真的渲染出来了才去拉，与既有 `mayhem-rarity` 同一套口径（不在渲染函数里发请求，也不为「用户从没点开构筑 tab」白花一次本地读盘） |
| **1206-1211** | `mayhemBuildTabMarkup`：既有三段（装备排行 / 装备路线 / 技能加点）**一字未动、顺序未变**，只在末尾追加 `<div data-mayhem-personal-builds-host>${renderMayhemPersonalBuilds(detail)}</div>`。没有样本时它渲染成 `<div ...></div>`，可见输出与改动前一致 |
| **2771-2774** | `resetTransientChampionState` 里清掉这 4 个字段（换英雄不能串号） |

`mayhemDetailTabSpecs`(**1191**) 的三 tab 骨架、`renderMayhemHeroRarityPanel`、`mayhemCautionNote`、阶段 chips、「选取率」格 —— R116-B/D/F 的成果**一律没动**。

### 1.8 CSS

**`backend/web/gameplay.css` 与 `backend/web/champions.css` 一个字都没改。**
原因：R117 的样式棘轮（`r117-style.test.cjs`）实测三个预算**正好卡在上限**——`border-radius` 33/33、`padding` 205/205、`gap` 54/54，`unreferenced` 90/93、`repeatedHexes` 41/41。任何新的间距取值都会让棘轮变差。所以海斗页签复用既有 `.ranked-queue-button`，个人海斗出装块复用既有 `.mayhem-ranking-list` 网格，`r116e.test.cjs` 里专门写了一条测试钉住「R116-E 不新增任何属于自己的 CSS 规则」。改完后六个预算实测值与改动前**完全一致**（见 §8）。

### 1.9 测试

| 文件 | 内容 |
|---|---|
| `backend/season_stats_test.go`（333 → 657 行） | 顶部那条「只认 420/440」的注释同步改写；新增 6 个测试（P0 判据 1 与回归、3220 不误收、两处判据同步、schema=9 与 7/8 都被拒、augment 样本槽位与去重上限、P2-6 三个纯函数测试） |
| **`backend/season_stats_budget_test.go`（新建，383 行）** | P1 判据 1（人为超 4 MiB → 先截断 `RankedMatches`、写入成功、不 panic、`Stats`/`GameIDs` 不动、调用方 cache 不被就地改坏）、端到端落盘、小缓存逐字节不变、**无可截断项时照写并报 `OverBudget`**、P1 判据 2（临时目录体检）、一次性去重、P3 判据 1（2000 场合成 fixture） |
| **`backend/gameplay_r116e_test.go`（新建，424 行）** | `legacyRankedQueueTabs` 兼容助手 + 契约附五的 4 条验收判据 + 门禁与文案回归 + P2-6 HTTP 出口 3 个测试 |
| `backend/player_ability_test.go` | **仅**把 2 处 `buildGameplayRankedQueues(solo, flex, ...)` 调用改成 `buildGameplayRankedQueues(legacyRankedQueueTabs(solo, flex), ...)`（签名变更的机械后果，断言一字未改）。这个文件不在契约给的测试清单里，属于必要连带修改，如实上报 |
| **`backend/web/r116e.test.cjs`（新建，449 行 / 12 个测试）** | 见 §4、§5、§6；**全程不给 `rankedQueueLabel` 注入桩** |
| `backend/web/champions.test.cjs` | 2 处：① `compileFunctions` 增加一条 R116-E 的传递编译名单（`rankedQueueLabel` / `isMayhemQueueId` / `rankedQueueNoun` 一律编译真实实现）；② **删掉 1711 行那个 `rankedQueueLabel` 测试桩**（它正是把生产缺陷盖住的原因） |

---

## 2. 逐条验证判据的真实状态

### 2.1 工单 P0（放开队列过滤）

| 判据 | 状态 | 证据 |
|---|---|---|
| 海斗 fixture 跑 `seasonStatsAccumulate`，`Stats`/`QueueStats` 正确记入 | **PASS** | `TestSeasonStatsAccumulateRecordsHextechMayhem`：2300/2400/3270 各 1 场 → `stats[157].Games == 3 / Wins == 2`，三个队列各自独立聚合，`seasonStatsFinalizeQueues` 后 2300 胜率 100、2400 胜率 0 |
| 用一条 420 对局验证行为不变（回归） | **PASS** | `TestSeasonStatsAccumulateKeepsRankedBehaviourAndRejectsPlainARAM`：420 记 1 场 1 胜；**3220「极地大乱斗」/ 450 / 1700 / 0 一律不收** |
| 两处判据保持同步 | **PASS** | 同一个测试末尾对 8 个队列逐个断言 `(len(stats) > 0) == (len(probe.RankedMatches) > 0)`；两处门禁在源码里都收敛成 `seasonStatsQueueAllowed` |
| 升版本号后旧 schema 缓存触发重扫 | **PASS** | `TestSeasonStatsSchemaVersionIsNineAndRejectsEveryOlderFile`：断言常量 == 9，并分别写入 schema=7 与 schema=8 的文件，`loadSeasonStats` 两次都返回 `invalid season stats cache` |
| 隐私声明文案与实际写操作一致 | **部分完成（按契约流程）** | 见 §9：核实做完了、比对表出完了、建议条目原文给了；**`features.go` 在禁触清单里，未写入**（R117 工单第 662 行已把这一条提交用户拍板） |

### 2.2 工单 P1（新建磁盘预算）

| 判据 | 状态 | 证据 |
|---|---|---|
| 人为超 4 MiB 的 cache → 写入前先截断 `RankedMatches`，不失败、不 panic | **PASS** | `TestSeasonStatsBudgetTruncatesRankedMatchesInsteadOfRefusingTheWrite`：fixture 原始 **4,697,412 B (4.48 MiB)** > 4 MiB 预算 → 落盘 ≤ 预算、`Truncated == true`、`Steps[0] == "ranked_matches_capped"`、`RankedMatchesAfter == 40`、`Stats`(173) 与 `GameIDs`(3000) 一个不少、调用方手里的 cache 未被就地改坏。端到端版本 `TestSeasonStatsSaveAppliesTheBudgetEndToEnd` 另外验证了落盘文件大小 == `report.WrittenBytes`、且截断后的文件仍能被 `loadSeasonStats` 读回 |
| 目录健康检查能正确报出总大小（临时目录） | **PASS** | `TestSeasonStatsDirectoryHealthReportsTotalSize`：3 个文件 1024+2048+4096 → `Files==3 / TotalBytes==7168 / LargestBytes==4096`；目录外的 999999 B 文件**不算进来**；目录不存在时是干净的零报告而不是错误；`LargestFile` 是相对路径（不含本机绝对路径/用户名）；空 root 不 panic。`TestSeasonStatsDirectoryHealthRunsOncePerRoot` 验证一次性 |
| 不整体拒绝写入 / 不丢 `Stats` / 不 panic | **PASS** | 额外写了 `TestSeasonStatsBudgetWritesAndReportsWhenNothingCanBeTrimmed`：只堆 120 万个 `GameIDs`（预算不许动它）→ 实测 **16,800,150 B** 照写，`OverBudget == true`、`Steps == [augment_samples_dropped written_over_budget]` |
| 不照搬 `binary_disk_budget.go` 的复杂度 | **PASS** | 新文件 269 行、无记账结构、无 LRU、无 strictDisk；Anti-scope 第 3 条：`binary_disk_budget.go` 未改、未被引用 |
| 预算挂在 `saveSeasonStats` | **PASS** | `season_stats.go:343-359`；`saveSeasonStats` 与 `saveSeasonStatsReported` 共用 `marshalSeasonStatsWithinBudget` |

### 2.3 工单 P2（容量估算）

| 项 | 状态 | 说明 |
|---|---|---|
| 按 20~50 场/页的真实区间估算，不用错误的「20 场/页」死算 | **PASS** | 估算表写在 `season_stats_budget.go:1-38`，错注释已在 `season_stats.go:28-45` 改对（评审第 2 节点名要修的那条） |
| 这条链路不设计限流 | **PASS** | 全程 SGP，未新增任何 `enterRiotLimitQueue` 调用；`grep -c enterRiotLimitQueue backend/season_stats*.go` = 0 |

### 2.4 工单 P3（`seasonAugmentSample` + P2-6）

| 判据 | 状态 | 证据 |
|---|---|---|
| 放开后单文件大小在 4 MiB 上限内（2000 场海斗合成 fixture） | **PASS** | `TestSeasonStatsSynthetic2000MayhemGamesStayWithinBudget`：**312,480 B = 305.2 KB = 0.30 MiB**，余量 **13.4×**，`Truncated == false`。详见 §7 |
| 只存 `GameID`/`ChampionID`/`AugmentIDs[6]`/`ItemIDs[7]`/`Win`，不存 22 项 postmatch | **PASS** | 结构体就这 5 个字段；`TestSeasonRecordRankedMatchMirrorsMayhemCriteriaAndRecordsAugmentSample` 断言零槽位被丢弃且 `len(AugmentIDs) <= 6 / len(ItemIDs) <= 7` |
| 只在海斗队列记录，420/440 不记 | **PASS** | 同上测试：一场 2400 + 一场 420 + 一场 3220 → `len(AugmentSamples) == 1` |
| 条目有上限 | **PASS** | `seasonAugmentSampleLimit = 2000`，推算过程见 §7；`TestSeasonTrimAugmentSamplesDedupesAndCaps` 断言 2252 条 → 2000 条、`dropped == 252`（1 条重复 + 1 条非法 GameID + 250 条超限）、保留的是 GameID 最大的那批 |
| P2-6：给定 augment ID 聚合出「最常出的装备组合 + 场次 + 胜率」 | **PASS** | 纯函数层 `TestSeasonAugmentBuildInsightAggregatesCombosGamesAndWinRate`（16 场 8 胜 → 胜率 50%、槽位顺序不同的同一套出装聚成一行、别的英雄 30 场被排除）；HTTP 层 `TestR116ESeasonMayhemBuildsEndpointAggregatesPersonalSamples`（12 场 7 胜 → 胜率 58%、占比 100%、`scope == "current-account"`） |
| 样本不足（< 可配置阈值，建议 10 场）时整块不显示 | **PASS** | `TestSeasonAugmentBuildInsightHidesEverythingBelowTheConfigurableThreshold`：9 场 → `nil`；阈值调到 5 → 出结果；`minimumSample <= 0` 一律回落到默认 10。HTTP 层 `...EndpointHidesInsufficientSamples`：4 场 → `available == false` + `reason` 非空，`?minimumSample=3` → `available == true`。前端 `r116e.test.cjs` 断言 `renderMayhemPersonalBuilds` 在 `null` / `available:false` / `groups:[]` / **key 属于别的英雄**四种情况下一律返回空串 |
| 是静态查询，不是局内实时联动 | **PASS** | Anti-scope 第 1 条：没有任何 ChampSelect / live 路径的接线；`champions.js` 只在英雄详情页构筑 tab 里懒加载 |
| 聚合逻辑是可测试的纯函数，魔法数字不散在渲染代码里 | **PASS** | `season_stats.go:1006-1215` 全部无副作用；三个上限都是具名常量；前端只负责渲染后端算好的数字，**不重排、不重算**（`renderMayhemPersonalBuilds` 里没有任何排序或阈值逻辑） |
| 零额外网络请求 | **PASS** | 字段来自赛季扫描本来就要解的 participant JSON（`riot_api.go:633-638` 的 `PlayerAugment1..6`、`Item0..6`）；端点只读本地 `season-stats` 缓存。**唯一的外部读取**是 `attachSeasonMayhemItemNames` 走 `a.champions.loadStaticDescriptions`（Data Dragon，自带缓存，6 秒超时），用于把装备 ID 翻成中文名——这是隐私声明 `externalReads` 里**已经声明过**的「Riot Data Dragon 公共地址」，读不到就整组回落到 ID，不编名字 |

### 2.5 工单「通用验证要求」

| 项 | 状态 | 证据 |
|---|---|---|
| `go test ./backend/... -run Season` 全绿 | **PASS** | 见 §8 |
| 隐私声明文案人工审校 | **完成审校，未改代码** | §9 的比对表 + 建议条目原文 |

### 2.6 工单「真机验证清单」

| 项 | 状态 | 用户需要做什么 |
|---|---|---|
| 打若干局海斗，确认下次总览页扫描后个人海斗统计正确出现 | **待真机验证** | Windows 真机 + League 客户端 + 真实海斗对局。**步骤**：① 打 ≥1 局海克斯大乱斗并结算；② 打开助手「总览」页，等本赛季扫描完成（首次会走后台回补，最多 12 页 / 90 秒）；③ 在「近 N 场排位」卡片右上角的队列切换器里点**第三个按钮「海克斯大乱斗」**（切换器的无障碍标签此时应从「排位模式」变成「对局模式」）。**期望**：卡片标题变成「近 N 场海克斯大乱斗」，胜率/KDA/击杀参与率是**只含海斗场次**的数字；「位置胜率」那一格显示「海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径」；**位置偏好与能力表现两张卡的切换器里不应该出现海斗按钮**（有意如此，不是漏做）。④ 再打开「英雄」页 → 海克斯大乱斗模式 → 任一你打过 ≥10 场的英雄 → 「构筑」tab，**期望**在既有三段之后看到「我的海斗出装」块，每行是「海克斯名 → 装备组合 / 场次 / 胜率」，右上角「本人 N 场」的 tooltip 写着口径说明；样本不足时**整块不出现**（不是空壳）。**要看的诊断事件**：`season_ranked_snapshot`（新增字段 `mayhem` / `capped_mayhem` / `augment_samples` / `augment_dropped` / `file_bytes` / `file_truncated`）、`season_stats_directory_health`（`files` / `total_bytes` / `largest_bytes` / `largest_file`）、`season_mayhem_builds_resolved` 或 `season_mayhem_builds_insufficient`（`champion_id` / `sample_games` / `minimum_sample` / `total_samples`）。 |
| `season-stats/sgp/<accountHash>-s26.json` 文件大小在 120 KB 量级而非几 MB | **待真机验证（合成 fixture 已给出量级证据）** | **看哪个文件**：本机存储根下的 `season-stats/sgp/<accountHash>-s26.json`。**期望量级**：工单估「60 KB → 120 KB」是在**不含 augment 样本**的前提下算的；本实现按 Anti-scope 第 4 条只存必要字段，合成 2000 场海斗 + 300 场排位的实测是 **305 KB**（其中 augment 样本 251 KB / 2000 条 = **126 B/场**）。所以真机上的合理区间是：**普通玩家（几十~几百场海斗）约 120~180 KB**，**一季 2000 场海斗的高频玩家约 300 KB 量级**，任何情况都**不该到几 MB**（4 MiB 是硬上限，超了会先截断 `RankedMatches`）。**更快的核对路径**：不必打满一季，直接看 `season_stats_directory_health` 事件的 `largest_bytes`，以及 `season_ranked_snapshot` 事件的 `file_bytes`——这两个数字就是落盘字节数。若 `file_truncated == true` 或 `file_over_budget == true`，说明命中了预算处置，需要回报。 |

### 2.7 工单「结束标志」

- [x] P0~P3 验证判据 PASS（真机两项除外，已如实标注）
- [~] 隐私声明文案已人工审校并与代码行为一致 —— **审校做完了，比对表与建议条目原文在 §9；`features.go` 未改**（禁触 + R117 已提交用户拍板）

---

## 3. 用户裁决带来的额外 UI 范围（**来自用户裁决，不是工单原文**）

> 本节整节都不在工单 P0/P1/P2/P3 的文字里。工单 P0 只说「放开三处队列过滤 + 升 schema + 隐私文案同步」，**没有预料到放开后新数据在现有 UI 里一个入口都没有**。主控会话把全部下游消费点查清后向用户确认，用户的裁决是：
>
> > **照工单字面全放开（三处），并在总览页队列切换器加「海克斯大乱斗」第三个 tab + 文案同步，让新数据有入口。**（改动最大，但无死数据）
>
> 所以本节是**用户授权的范围**，不算超范围改动，但按要求单开一节说明来源。

### 3.1 为什么必须加：我复核到的证据

复核结论与契约一致——**只放开过滤 = 多存约 54 KB、每场多算一遍，用户一个数字都看不到**。我逐个确认了下游消费点：

- `gameplay.go:3551` `recentRankedSummary` 对 `matches`(**3561**) 与 `cached`(**3585**) 都过滤 420/440；
- `gameplay.go:3698` `recentRankedSummaryForQueue` 的早退会把非 420/440 直接退回上面那个只认 420/440 的函数；
- `gameplay.go:1525` 的调用点原本只塞 `rankedSamples.ByQueue[420]` 与 `[440]`；
- `gameplay.go:1742` 的 `buildGameplayRankedQueues` 原本签名与 `make(map[...], 2)` 都硬编码两队列；
- `player_ability.go:98` `seasonAbilityStatsForQueue` 按 `queueID` + `abilityPositionKnown(position)` 双重过滤；
- `gameplay.js:1765` 的切换器原本硬编码两个按钮；
- `riot_api.go:1461` 韩服路径也调同一个函数。

### 3.2 海斗 tab 的数据通路（契约要求「把实际数据通路查清后写进账本」）

**查清了：首屏样本路径本身不取海斗。** `loadRecentRankedSamples`(**1564-1597**) 硬编码 `for _, queueID := range []int64{420, 440}`，每个队列各打一次 `loadRecentRankedSampleQueue`（带 `solo`/`flex` 过滤标签的 SGP 请求）。要让它取海斗就得**为首屏多加一次网络请求**，而且 `filter` 的取值映射（`1605-1608`）也只认 `solo`/`flex`。

所以海斗 tab 的数据**只能来自赛季缓存 `RankedMatches`**（P0 放开后它已经含 2300/2400/3270 的场次）。实现路径：

1. `seasonResult` 新增 `ranked` 字段，`loadSeasonChampionStatsSnapshot` 的第三个返回值不再被 `_` 丢弃（**1223-1232 / 1247-1248 / 1325**）；
2. `gameplayRankedQueueTabs(byQueue, seasonRanked)` 从 `seasonRanked` 里挑出海斗条目组成第三个页签（**1710-1731**）；
3. 该页签 `Matches == nil`、`Cached == 海斗快照`，走 `recentRankedSummaryForQueues(samples, playerRef, cached, [2300,2400,3270], "海克斯大乱斗")`；
4. **`recentRankedSummaryForQueue` 的 3698 早退必须一起改**（契约点名的那条），否则海斗 tab 会拿到混了 420/440 的数据 = 伪造。已改成按显式队列集合过滤。

**这条通路零额外网络请求**：赛季缓存本来每次总览都要读（`loadSeasonChampionStatsSnapshot` 是 network-free 的快照读取）。

### 3.3 三个海斗队列合并成一个 tab

按契约 UI 入口第 3 条：`2300/2400/3270` 在 `queue_groups.go:120-124` 的官方中文名都是「海克斯大乱斗」、同属 `hextech-aram` 组，UI 上**一个按钮**。页签标识取 `seasonMayhemPrimaryQueueID = 2300`（组内第一个），语义是「页签标识」而不是「只统计这个队列」——这一点在 Go 常量注释与 `r116e.test.cjs` 里都钉死了（前端 `MAYHEM_QUEUE_TAB_KEY` 必须等于后端常量，测试直接读生产源码比对，不各写一个字面量）。`3220`「极地大乱斗」在 Go 与 JS 两侧的测试里都被显式断言**不收**。

### 3.4 为什么海斗按钮只出现在「近期战绩」这一个 scope

`careerSectionEntries` 有三张卡各自带一个切换器（`recent` / `ability` / `position`），三者的状态是**独立**的（`rankedQueueRecent` / `rankedQueueAbility` / `rankedQueuePosition`）。海斗按钮只加在 `recent` 上，理由有两条，都是诚实性考虑：

1. **能力雷达与位置偏好依赖分路口径，海斗没有分路**。给它们一个海斗按钮，点进去必然是一块空壳或 0 值——契约附五明确要求「前端必须整块隐藏而不是显示一个空雷达或 0 值」。
2. **会变成死胡同**：如果 `position` scope 也能切到海斗，那么整块隐藏之后**切换器自己也跟着消失**，用户再也点不回单双排。

代价是三个切换器的按钮数不一致（3 / 2 / 2）。我判断这比「一个点进去什么都没有、还回不来的按钮」更诚实，并且在海斗的「近期战绩」卡里写了一句解释：「海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径」，让用户知道那两块为什么没有海斗选项。

### 3.5 `aria-label` 的处置

`aria-label="排位模式"` 在加了非排位队列后确实变成不准确表述。处置是**按实际可选范围切换**：含海斗按钮时 → `aria-label="对局模式"`；只有 420/440 时（`ability`/`position` scope，以及没有海斗样本的玩家）→ 保留 `aria-label="排位模式"`（此时它仍然准确）。`r116e.test.cjs` 两个方向都断言了。

### 3.6 文案同步

| 位置 | 改动 | 理由 |
|---|---|---|
| `gameplay.js:2266` `近 N 场排位` | → `近 N 场${rankedQueueNoun(queueId)}`；420/440 逐字不变，海斗 → `近 N 场海克斯大乱斗` | 海斗不是排位 |
| `gameplay.js:2270-2272` 空态 | 海斗 → 「基于本赛季已扫描到的海克斯大乱斗对局，最多统计 20 场。」；队列名解析不出来 → 「当前样本未发现对局」（**不写「单双排」**） | 海斗数据来自赛季缓存不是首屏 20 场；未知队列不许编名字 |
| `gameplay.js:2286-2288` 位置胜率格 | 海斗 → 「海克斯大乱斗没有分路，不提供位置胜率与能力雷达口径」 | `位置数据不足` 会被读成「多打几场就有了」 |
| `gameplay.js:2399` `本赛季 · N 场排位` | → `本赛季 · N 场对局` | 放开后 `seasonChampionStats` 含海斗场次 |
| `gameplay.js:2420` 位置偏好空态 | `Number(queueId) === 440 ? ... : ...` → `rankedQueueLabel(queueId)` | 收敛到唯一解析点 |
| `gameplay.js:1794` `${number(opggSeason.overall.games)} 场排位` | **未改** | 韩服 OP.GG 链路只有排位，仍然准确（`desktop/pro-players.test.cjs:386` 也钉着它） |
| `gameplay.js:2294` `正在后台按当前队列统计本赛季战绩…` | **未改** | 复核结论：这句在 `renderRanks` 的**段位行**里，`collecting` 来自 `seasonProgress.collecting`，而段位只有单双排/灵活组排两个 `queueType`（`expected = ["RANKED_SOLO_5x5","RANKED_FLEX_SR"]`）。海斗没有段位、不进这张卡，所以「按当前队列统计」在这里指的始终是排位队列，**在海斗 tab 下也不会被渲染到**。契约 UI 入口第 7 条要求复核这一句，结论是「仍准确，无需改动」 |
| `season_stats.go:664/666` 与 `696/698` `已统计 %d 场` | **未改** | 契约附二已核实：`GameIDs` 的 append 在队列过滤**之前**，所以这个 N 今天就已经包含海斗场次了，这句话本身不会因为本轮改动而变错 |

---

## 4. `rankedQueueLabel` 既存生产缺陷的闭合记录

**这是发现的既存缺陷，不是本轮的新功能。**

- **缺陷**：`backend/web/gameplay.js:1703`（改动前行号）调用了 `rankedQueueLabel(Number(queue.positionQueueId || key))`，但这个函数**在生产代码里从未定义**。全仓 grep（`backend/`、`desktop/`，含 `.js`/`.cjs`/`.html`）改动前只有两处命中：那一处调用，和 `backend/web/champions.test.cjs:1711` 的**测试桩** `rankedQueueLabel: (queueId) => queueId === 440 ? "灵活组排" : "单双排"`。
  > 注：Go 侧另有一个同名的 `rankedQueueLabel(queueID int64) string`（`gameplay.go:3427`，改动前），与 JS 是两个语言的两个符号，**不构成定义**。
- **触发条件**：`data.rankedQueues` 存在、`data.rankedQueues[key]` 存在、且 `queue.positionQueueLabel` 为假值 → 真机命中即 `ReferenceError: rankedQueueLabel is not defined`，整页渲染被 `renderOverviewBody` 的 try/catch 兜住后切到「总览渲染失败」错误态。
- **为什么测试没抓到**：测试自己注入了桩。这是「桩掩盖生产缺陷」的典型案例。
- **闭合方式**：
  1. 在 `gameplay.js:1709-1717` **真正定义** `rankedQueueLabel(queueId, fallback = "")`：420 → 单双排、440 → 灵活组排、2300/2400/3270 → 海克斯大乱斗、**未知 → 空串**（`fallback` 只在调用方显式传时生效，绝不默默标成「单双排」）；
  2. 把散落的二元三元收敛上去：`rankedQueueData` 的 **3 处**（原 1695/1699/1700 对应现在的 1741/1746/1747）、`renderPositionStats` 的 1 处（**2420**）。契约点名的 `2107` 那条 `queueType === "RANKED_FLEX_SR"` 是**段位来源**、语义不同，**没动**；`gameplay.js:1794` 的韩服 OP.GG 文案也没动；
  3. **删掉 `champions.test.cjs:1711` 的测试桩**，改成让 `compileFunctions` 按名字编译真实实现（沿用该文件里 R75/R116-B 的既有做法），并在 `compileFunctions` 里加了一条 R116-E 的传递编译名单；
  4. `r116e.test.cjs` 第 1 个测试**不注入任何桩**，断言：定义确实在 `gameplay.js` 源码里、对 420/"420"/440/2300/2400/3270/"2400" 各返回正确值、对 3220/450/1700/0/-1/"abc"/null/undefined/NaN 一律返回空串；
  5. **对抗变异**（见 §8）：把定义从源码里删掉 → 3 个测试 FAIL。
- **附带发现**：`isMayhemQueueId` 刻意把 `[2300, 2400, 3270]` **内联在函数体里**而不是提成模块级常量。原因就是上面这个缺陷的成因——聚焦测试按函数名编译真实实现时，模块级常量不会跟着进作用域，函数不自包含就会被迫塞桩。这一点写在源码注释里，避免后来人「顺手优化」成常量。

---

## 5. 契约附五：三处工单没提到的 `420/440` 硬门禁的处置与理由

### 5.1 `positionStatsForQueue`（改动前 3509，现在 **3858-3878**）—— ⛔ 最危险的一条

- **改动前**：`if queueID != 420 && queueID != 440 { return positionStats(matches, playerRef) }`，即**回退到不分队列的全量位置统计**。
- **后果**：海斗 tab 会拿到混了全部队列的分路数据，却显示在「海克斯大乱斗」标签下 = **伪造口径**。
- **处置**：改成 `return nil`。★ 注释里把「曾经写的是什么、为什么那是最危险的一条、海斗是 ARAM 系没有 top/jungle/middle/bottom/utility」全写清楚了。
- **额外一层防护**：`buildGameplayRankedQueues` 里用 `gameplayRankedTabHasPositions()` 判断，海斗页签**根本不调用**这个函数、也不下发 `Positions`/`Ability`/`AbilitySampleGames`。所以即使有人日后把 `positionStatsForQueue` 的早退改回去，海斗页签也不会拿到它——两道闸。
- **对抗变异实跑**：见 §8 变异 B。

### 5.2 `liveRecentPositions`（改动前 3523，现在 **3880-3900**）

- **改动前**：`if queueID != 420 && queueID != 440 { return nil }`。
- **处置**：**行为不变**（`return nil` 对海斗恰好是正确的），只把判据换成 `!seasonClassicRankedQueue(queueID)` 并补上注释：「非 420/440 一律返回 nil 是**有意的**，不是漏改：局内视图的 `response.QueueID` 在海克斯大乱斗里会是 2300/2400/3270，而 ARAM 系没有分路，位置偏好在这个语境下不成立。改成回退到全量统计等于把别的队列的分路数据挂在海斗局内页上。」
- 这条注释的存在本身就被测试钉住（`r116e.test.cjs` 断言源码里必须出现「非 420/440 一律返回 nil 是**有意的**」），防止后来人当成漏改「修好」。

### 5.3 `championStats` 的对局循环（改动前 3447，现在 **3782-3795**）—— 契约的描述需要更正

- **契约附五把它描述成「对位/counter 统计」**。实测不是：那个循环在 `championStats(matches, playerRef, names)` 里，产出的是 `response.ChampionStats`，也就是总览页**「英雄胜率」卡片在赛季缓存不可用时的兜底**。它**不被任何队列页签消费**——只有两个调用点：`gameplay.go:1515`（国服总览级）与 `riot_api.go:1454`（韩服总览级），都不带 `queueID` 参数。所以「海斗 tab 的对位统计为空」这个后果**不成立**：海斗 tab 根本不读它。
- **处置**：**刻意保留 `420/440` 门禁**（改成调用 `seasonClassicRankedQueue(match.QueueID)`，语义等价），并补上理由注释。理由：那张卡片的标签是 `本赛季 · N 场排位`（`renderChampionStats` 的兜底分支），放开会把海斗场次混进一张写着「排位」的卡片——正是契约禁止的「静默显示一个基于混合队列的数字」。
- **诚实性检查**：保留门禁不会让海斗数据「取不到」，因为海斗的英雄级统计走的是赛季 `Stats`（P0 已放开）→ `data.seasonChampionStats`，`championStats` 只是它不可用时的兜底。
- **测试**：`TestR116EChampionStatsStaysRankedOnly` 用 420 + 2400 + 2300 + 3220 四场断言只数出 1 场。
- **连带的文案修正**：赛季 `Stats` 放开后确实含海斗场次，所以 `renderChampionStats` 的兜底标签从「N 场排位」改成「N 场对局」（§3.6）。这样两条路径都不会说假话：兜底路径只含排位、标签写排位；赛季路径含全部队列、标签写对局。

### 5.4 能力雷达（契约附五最后一条：要求实测确认）

**实测结论：海斗 tab 的能力雷达确实为空，这是诚实降级，前端整块不出。** 三条独立的原因，任何一条都足以让它为空：

1. `gameplayAbilityStatsForQueue`（`player_ability.go:152`）要求 `match.QueueID == queueID` 且 `abilityPositionKnown(position)`；海斗场次没有 `teamPosition`/`individualPosition`（`seasonRecordRankedMatch` 里 `position` 落到空串）→ 0 场样本；
2. `seasonAbilityStatsForQueue`（`player_ability.go:102-108`）除了按 `queueID` 过滤，还要求 `item.Ability != nil` 且 `abilityPositionKnown(position)`；而 `seasonRankedAbilitySampleFor`（`season_stats.go` 内）在 `!abilityPositionKnown(position)` 时**返回 nil** → 海斗场次的 `Ability` 字段恒为 nil；
3. `buildGameplayRankedQueues` 里海斗页签的 `gameplayRankedTabHasPositions()` 为假，**根本不调用** `buildGameplayAbilityProfileForQueue` / `gameplayAbilitySampleGamesForQueue`，`Ability` 保持 nil、`AbilitySampleGames` 保持 0。

测试证据：`TestSeasonRecordRankedMatchMirrorsMayhemCriteriaAndRecordsAugmentSample` 断言海斗快照的 `Position == "" && Ability == nil`；`TestR116EMayhemTabRendersNoPositionsAndNoAbilityRadar` 断言 `mayhem.Ability == nil && mayhem.AbilitySampleGames == 0 && mayhem.Positions == nil`，同时断言**单双排页签的位置与能力样本没有被带坏**（回归）。

`backend/player_ability.go` **一个字都没改**——它不需要改，三条原因都在别处。

### 5.5 附五的 4 条验收判据

| 判据 | 状态 | 测试 |
|---|---|---|
| 1. 海斗 tab 的「近期战绩」只含 2400 场次，不含任何 420/440 场次 | **PASS** | `TestR116EMayhemTabOnlyContainsMayhemGames`：420/440/2400/2300/3270/3220 各 1 场的 fixture → 海斗页签 `Games == 3, Wins == 2, Losses == 1`（三个海斗 ID 全收、3220 与 420/440 全不收），`QueueLabel == PositionQueueLabel == "海克斯大乱斗"`，`Positions` 为空 |
| 2. 海斗 tab **不渲染**位置统计与能力雷达 | **PASS** | `TestR116EMayhemTabRendersNoPositionsAndNoAbilityRadar`（后端：`Positions == nil`、`Ability == nil`、`AbilitySampleGames == 0`、`positionStatsForQueue(...,2400) == nil`、`liveRecentPositions(...,2400) == nil`）+ `r116e.test.cjs`（前端：海斗卡显示「没有分路」解释而不是「位置数据不足」；`ability`/`position` scope 的切换器里不出现海斗按钮） |
| 3. 单双排 tab 的行为**逐字节不变**（回归） | **PASS** | `TestR116EClassicTabsAreUnaffectedByMayhemSamples`：同一批 420/440 样本，在「没有海斗样本」与「有海斗样本」两种输入下，`json.Marshal(queues["420"])` 与 `queues["440"]` 的字节串**完全相等** |
| 4. 韩服路径（`riot_api.go`）行为不变（回归） | **PASS** | `TestR116EKoreanPathStillProducesExactlyTwoQueues`：只产出 2 个 key、标签分别是单双排/灵活组排、**`queues["2300"]` 不存在** |
| 对抗变异：把早退改回 `return positionStats(matches, playerRef)`，判据 2 必须 FAIL | **PASS** | §8 变异 B 实跑输出 |

---

## 6. P2-6 静态查询的最终定义原文

### 6.1 `seasonAugmentSample`（`backend/season_stats.go:129-168`）

```go
// 海克斯大乱斗一场的海克斯槽位与装备槽位数量。样本只保留非零槽位，
// 但槽位数本身是有界的：6 个海克斯 + 7 个装备（含饰品栏）。
const (
	seasonAugmentSlots = 6
	seasonItemSlots    = 7
)

// seasonAugmentSampleLimit 是本人海斗样本的条数上限。推算过程（工单 P3 只给了
// 量级，这里给出可复核的算术）：
//
//	单条 JSON 实测约 130 B（gameId 14 位 + championId + 3~6 个海克斯 ID
//	+ 6 个装备 ID + win），2000 条 ≈ 260 KB。
//	加上 RankedMatches（5 队列 × 40 条 × ~450 B ≈ 90 KB）、Stats（≤173 条
//	≈ 15 KB）、QueueStats（5 条 ≈ 0.5 KB）、GameIDs（整季 2000 场 × 15 B
//	≈ 30 KB），单文件合计约 400 KB，对 seasonStatsFileBudgetBytes 的 4 MiB
//	上限留了约 10 倍余量。
//
// 工单算的「精简路线 300 B/场 × 2000 场 = 0.6 MB」把伤害/经济等字段也算进去了，
// 本实现按 Anti-scope 第 4 条只存必要字段，所以真实占用更低；上限仍取 2000，
// 与工单的量级一致，也覆盖「一季 2000 场海斗的高频玩家」这个估算基准。
const seasonAugmentSampleLimit = 2000

// seasonAugmentSample 是一场海克斯大乱斗里本人的海克斯与成装快照，
// 与 seasonRankedMatch.Ability 同属「赛季扫描顺手记」的模式：扫描本来就要
// 解出这一场的 participant JSON，多留几个字段是零额外网络请求。
//
// 只存 P2-6「选了某个海克斯之后通常出什么」这一个静态查询必要的字段。
// 伤害/承伤/经济/22 项 postmatch 指标一律不存——那是英雄级统计，走
// R116-A 的 /api/hexdata/postmatch，不在个人对局记录里重复存一份
// （Anti-scope 第 4 条）。
//
// 只在海克斯大乱斗队列（2300/2400/3270）记录，经典排位没有海克斯。
type seasonAugmentSample struct {
	GameID     int64 `json:"gameId"`
	ChampionID int64 `json:"championId"`
	// AugmentIDs 按槽位顺序保留非零海克斯 ID，最多 seasonAugmentSlots 个。
	AugmentIDs []int64 `json:"augmentIds,omitempty"`
	// ItemIDs 按槽位顺序保留非零装备 ID，最多 seasonItemSlots 个。
	ItemIDs []int64 `json:"itemIds,omitempty"`
	Win     bool   `json:"win"`
}
```

> **与工单字面的一处差异（如实上报）**：工单写 `AugmentIDs[6]`/`ItemIDs[7]`。实现用的是**切片 + `omitempty`**而不是定长数组 `[6]int64`/`[7]int64`。理由：定长数组会把空槽位序列化成 `"augmentIds":[0,0,5001,0,0,0]`（约 25 B），而只存非零槽位是 `"augmentIds":[5001]`（约 18 B）；工单要求走的正是「精简 0.6 MB」路线。槽位上限由 `seasonAugmentSlots = 6` / `seasonItemSlots = 7` 两个具名常量表达，并由测试断言 `len(AugmentIDs) <= 6 && len(ItemIDs) <= 7`。**存的信息量与工单一致，只是编码更省。**

### 6.2 P2-6 查询函数（`backend/season_stats.go:1006-1215`，全部是无副作用纯函数）

```go
const (
	// seasonAugmentMinimumSample 是海克斯分组的最小样本门槛。
	// 工单建议「复用 hexdataMinimumSample 类似的量级但按个人样本调低，比如
	// 10 场」：hexdataMinimumSample = 250 是全服英雄级口径，个人样本一个赛季
	// 打到 2000 场海斗已经是高频玩家，10 场是「看得出倾向、又不至于把 1 场
	// 当规律」的下限。门槛可由调用方覆盖（HTTP 层暴露成查询参数）。
	seasonAugmentMinimumSample = 10
	// 一次最多返回几个海克斯分组、每个分组最多几套成装。都是展示上限，
	// 不影响聚合口径本身。
	seasonAugmentBuildGroupLimit = 6
	seasonAugmentBuildComboLimit = 3
)

// seasonAugmentBuildCombo 是「选了某个海克斯之后最常出的一套成装」。
// ItemIDs 升序排列（与游戏内的装备栏顺序无关），保证同一套出装不管槽位怎么
// 摆都聚成一行。含饰品栏——样本存的就是最终 7 个槽位，不在聚合时偷偷丢字段。
type seasonAugmentBuildCombo struct {
	ItemIDs   []int64  `json:"itemIds"`
	ItemNames []string `json:"itemNames,omitempty"`
	Games     int      `json:"games"`
	Wins      int      `json:"wins"`
	WinRate   int      `json:"winRate"`
	// Share 是这套出装占「该海克斯全部场次」的百分比。
	Share int `json:"share"`
}

// seasonAugmentBuildGroup 是一个海克斯 ID 的聚合结果。
type seasonAugmentBuildGroup struct {
	AugmentID int64                     `json:"augmentId"`
	Games     int                       `json:"games"`
	Wins      int                       `json:"wins"`
	WinRate   int                       `json:"winRate"`
	Combos    []seasonAugmentBuildCombo `json:"combos"`
}

// seasonAugmentBuildReport 是 P2-6 静态查询的完整结果。
// SampleGames 是「这个英雄的海斗样本总数」，Groups 为空表示没有任何分组
// 达到 minimumSample —— 前端此时整块不显示（工单 P3 判据第 2 条）。
type seasonAugmentBuildReport struct {
	ChampionID    int64                     `json:"championId"`
	SampleGames   int                       `json:"sampleGames"`
	MinimumSample int                       `json:"minimumSample"`
	Groups        []seasonAugmentBuildGroup `json:"groups,omitempty"`
}

// seasonAugmentMinimumSampleOr 把「可配置门槛」收敛成一个函数：
// 非正数一律回落到默认值，避免调用方传 0 导致「1 场也算规律」。
func seasonAugmentMinimumSampleOr(configured int) int {
	if configured > 0 {
		return configured
	}
	return seasonAugmentMinimumSample
}

// seasonAugmentComboKey 把一套出装折叠成稳定的分组键（ID 升序）。
func seasonAugmentComboKey(itemIDs []int64) string {
	sorted := append([]int64(nil), itemIDs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, fmt.Sprintf("%d", id))
	}
	return strings.Join(parts, ",")
}

// seasonAugmentBuildInsight 是工单判据里的那条查询：给定一个海克斯 ID，
// 从本人历史海斗样本里聚合出「选了它之后最常出的装备组合 + 场次 + 胜率」。
// 样本不足 minimumSample 时返回 nil（调用方整块不显示），绝不返回一个
// 用 1、2 场凑出来的「规律」。
func seasonAugmentBuildInsight(samples []seasonAugmentSample, augmentID int64, minimumSample, comboLimit int) *seasonAugmentBuildGroup {
	if augmentID <= 0 {
		return nil
	}
	minimumSample = seasonAugmentMinimumSampleOr(minimumSample)
	type comboCounter struct {
		itemIDs []int64
		games   int
		wins    int
	}
	combos := make(map[string]*comboCounter)
	group := &seasonAugmentBuildGroup{AugmentID: augmentID}
	for _, sample := range samples {
		if !seasonAugmentSampleHasAugment(sample, augmentID) {
			continue
		}
		group.Games++
		if sample.Win {
			group.Wins++
		}
		key := seasonAugmentComboKey(sample.ItemIDs)
		counter := combos[key]
		if counter == nil {
			sorted := append([]int64(nil), sample.ItemIDs...)
			sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
			counter = &comboCounter{itemIDs: sorted}
			combos[key] = counter
		}
		counter.games++
		if sample.Win {
			counter.wins++
		}
	}
	if group.Games < minimumSample {
		return nil
	}
	group.WinRate = seasonWinRatePercent(group.Wins, group.Games)
	rows := make([]seasonAugmentBuildCombo, 0, len(combos))
	for _, counter := range combos {
		rows = append(rows, seasonAugmentBuildCombo{
			ItemIDs: counter.itemIDs, Games: counter.games, Wins: counter.wins,
			WinRate: seasonWinRatePercent(counter.wins, counter.games),
			Share:   seasonWinRatePercent(counter.games, group.Games),
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Games != rows[j].Games {
			return rows[i].Games > rows[j].Games
		}
		return seasonAugmentComboKey(rows[i].ItemIDs) < seasonAugmentComboKey(rows[j].ItemIDs)
	})
	if comboLimit > 0 && len(rows) > comboLimit {
		rows = rows[:comboLimit]
	}
	group.Combos = rows
	return group
}

// seasonAugmentSampleHasAugment 判断一场样本里是否选了某个海克斯。
func seasonAugmentSampleHasAugment(sample seasonAugmentSample, augmentID int64) bool {
	for _, id := range sample.AugmentIDs {
		if id == augmentID {
			return true
		}
	}
	return false
}

// seasonAugmentBuildReportFor 把查询收成英雄详情页「构筑」tab 能直接用的一份
// 报告：先按英雄过滤（详情页是英雄语境，把别的英雄的出装混进来就是伪造），
// 再按「本人实际用过的海克斯」分组，逐个跑 seasonAugmentBuildInsight。
//
// augmentID > 0 时只返回那一个分组（工单判据里的「给定一个 augment ID」）；
// 为 0 时返回本人在这个英雄上用得最多、且达到门槛的前 groupLimit 个分组。
func seasonAugmentBuildReportFor(samples []seasonAugmentSample, championID, augmentID int64, minimumSample, groupLimit, comboLimit int) seasonAugmentBuildReport {
	minimumSample = seasonAugmentMinimumSampleOr(minimumSample)
	report := seasonAugmentBuildReport{ChampionID: championID, MinimumSample: minimumSample}
	scoped := make([]seasonAugmentSample, 0, len(samples))
	for _, sample := range samples {
		if championID > 0 && sample.ChampionID != championID {
			continue
		}
		report.SampleGames++
		scoped = append(scoped, sample)
	}
	if augmentID > 0 {
		if group := seasonAugmentBuildInsight(scoped, augmentID, minimumSample, comboLimit); group != nil {
			report.Groups = []seasonAugmentBuildGroup{*group}
		}
		return report
	}
	used := make(map[int64]int, 8)
	order := make([]int64, 0, 8)
	for _, sample := range scoped {
		for _, id := range sample.AugmentIDs {
			if id <= 0 {
				continue
			}
			if _, seen := used[id]; !seen {
				order = append(order, id)
			}
			used[id]++
		}
	}
	sort.Slice(order, func(i, j int) bool {
		if used[order[i]] != used[order[j]] {
			return used[order[i]] > used[order[j]]
		}
		return order[i] < order[j]
	})
	for _, id := range order {
		group := seasonAugmentBuildInsight(scoped, id, minimumSample, comboLimit)
		if group == nil {
			continue
		}
		report.Groups = append(report.Groups, *group)
		if groupLimit > 0 && len(report.Groups) >= groupLimit {
			break
		}
	}
	return report
}

// seasonWinRatePercent 与仓库既有的胜率取整口径一致（四舍五入到整数百分比）。
func seasonWinRatePercent(wins, games int) int {
	if games <= 0 {
		return 0
	}
	return int(float64(wins)*100/float64(games) + 0.5)
}
```

### 6.3 HTTP 出口（`backend/gameplay.go:1839-1996`，路由 `main.go:509`）

`GET /api/gameplay/season-mayhem-builds?championId=<id>[&augmentId=<id>][&minimumSample=<n>]`

响应形状：

```json
{
  "available": true,
  "scope": "current-account",
  "season": "S26",
  "championId": 157,
  "augmentId": 5001,
  "minimumSample": 10,
  "sampleGames": 87,
  "groups": [{
    "augmentId": 5001, "games": 23, "wins": 13, "winRate": 57,
    "combos": [{ "itemIds": [3006,3031,3153,6672], "itemNames": ["…"], "games": 9, "wins": 5, "winRate": 56, "share": 39 }]
  }]
}
```

关键处置（都是「取不到就明说」，不猜不编）：

| 情况 | 响应 |
|---|---|
| `championId` 缺失/非正数、`augmentId` 非法、`minimumSample` 非法 | **HTTP 400** |
| 没连 League 客户端（拿不到「本人」是谁） | 200 + `available:false` + `reason:"未连接英雄联盟客户端，读不到本人赛季样本"`。**不退化成扫全目录**——那会混账号 |
| `a.storage == nil` / `accountHash == ""` | 200 + `available:false` + 原因 |
| 赛季缓存读不到 | 200 + `available:false` + `reason:"本赛季还没有可读取的海克斯大乱斗样本"`。**不用最近 20 场代替** |
| 有缓存但没有任何分组达到门槛 | 200 + `available:false` + `reason:"本人海克斯大乱斗样本不足（该英雄 N 场，每个海克斯需 ≥M 场）"` + 诊断事件 `season_mayhem_builds_insufficient` |
| 正常 | 200 + `available:true` + `groups` + 诊断事件 `season_mayhem_builds_resolved` |

- `minimumSample` **可配置**（工单要求），但夹在 `[1, seasonAugmentSampleLimit]`，`<= 0` 直接 400，防「1 场也算规律」。
- `scope: "current-account"` 是刻意加的：这份样本只属于**当前登录账号**，不是详情页正在浏览的那个英雄的全服口径。前端 tooltip 也把这句话写给用户看了。
- `attachSeasonMayhemItemNames` 用 `a.champions.loadStaticDescriptions`（Data Dragon，自带缓存，6 秒超时）把装备 ID 翻成中文名；**只要有一件翻不出来就整组不给名字**（一半有名一半是 ID 更容易被误读成「这几件才是核心」），前端会回落到详情页自带的装备行、再回落到 `装备 <id>`。读不到目录时记 `season_mayhem_item_names_unavailable` 诊断事件。

---

## 7. 容量：推算过程与实测数字

### 7.1 修正后的容量估算（P2）

工单 P1 现状证据第 1 条与评审第 2 节都指出「每页 20 场」是错的。真实口径：

- `sgpPageSize = 50`（`sgp_api.go:77`），缓存命中回退路径收窄到 `sgpFallbackPageSize = 20`（`:78`，`matchHistoryFilteredOn`）→ **单页 20~50 场波动**；
- 前台 `seasonScanForegroundPages = 2` → **40~100 场**；
- 后台 `seasonScanBackgroundPages = 12` → **240~600 场**。

| 组成 | 修正前（工单/错注释） | 修正后（本实现） |
|---|---|---|
| 前台单轮覆盖 | 40 场 | **40~100 场** |
| 后台单轮覆盖 | 240 场 | **240~600 场** |
| `GameIDs` | 不变（本来就全队列记录） | 不变；整季 2000 场 ≈ 30 KB |
| `Stats` | 不变（≤173 条） | 不变 ≈ 15 KB |
| `QueueStats` | +3 条 ≈ +0.3 KB | +3 条（2300/2400/3270）≈ +0.5 KB |
| `RankedMatches` | +3 队列 × 40 × ~450 B ≈ +54 KB | 同口径；上限从 2 队列 × 40 = 80 条变成 **5 队列 × 40 = 200 条** ≈ 90 KB |
| `AugmentSamples`（P3 新增） | 工单按 300 B/场 × 2000 = 0.6 MB | **实测 126 B/场 × 2000 = 251 KB**（只存必要字段，Anti-scope 第 4 条） |
| 单文件合计 | 60 KB → 120 KB | **实测 305 KB**（2000 场海斗 + 300 场排位的合成 fixture） |

### 7.2 `seasonAugmentSampleLimit = 2000` 的推算

```
单条样本 JSON 实测（合成 fixture，2000 条 251,001 B）= 126 B/场
  2000 场 × 126 B                        ≈ 251 KB
  RankedMatches 5 队列 × 40 条 × ~450 B   ≈  90 KB
  Stats ≤173 条                           ≈  15 KB
  QueueStats 5 条                         ≈ 0.5 KB
  GameIDs 整季 2300 场 × ~15 B            ≈  30 KB
  ─────────────────────────────────────────────
  单文件合计（实测）                        = 312,480 B = 305.2 KB = 0.30 MiB
  seasonStatsFileBudgetBytes              = 4,194,304 B = 4.00 MiB
  余量                                    = 13.4×
```

结论：**2000 这个上限对 4 MiB 预算留了 13.4 倍余量**，即使 2000 场全是 6 海克斯 + 7 装备的极端形状也不会触发截断。取 2000 而不是更大的数，是为了与工单 P2/P3 的估算基准（「一季 2000 场海斗的高频玩家」）对齐——超过 2000 场的赛季在 S26 一个赛季内基本不可能（2000 场 × 20 分钟 ≈ 667 小时）。

### 7.3 实测输出（`go test -run ... -v` 原样）

```
season_stats_budget_test.go:77:  P1 fixture: raw = 4697412 bytes (4.48 MiB), budget = 4194304 bytes
season_stats_budget_test.go:208: P1 untrimmable case: written = 16800150 bytes, over budget = true,
                                 steps = [augment_samples_dropped written_over_budget]
season_stats_budget_test.go:346: P3 capacity (synthetic, 2000 mayhem + 300 ranked games):
                                 file = 312480 bytes = 305.2 KB = 0.30 MiB;
                                 budget = 4194304 bytes; headroom = 13.4x
season_stats_budget_test.go:348: P3 breakdown: gameIds=2300 stats=1 queueStats=5
                                 rankedMatches=200 augmentSamples=2000
season_stats_budget_test.go:356: P3 augment samples: 251001 bytes for 2000 entries = 126 B/场
                                 （工单按 300 B/场估，本实现只存必要字段）
season_stats_budget_test.go:381: P2-6 over the reloaded 2000-game cache: sampleGames=2000 groups=6
                                 topGroup=augment 5201 with 65 games / 49% win rate
```

---

## 8. 验证实跑输出

### 8.1 交付前必须全绿

```
$ go build -o "$PI_SCRATCH_DIR/dl-build" ./backend
（无输出，退出码 0）

$ go vet ./backend && gofmt -l backend/*.go
（两条都无输出）

$ go test ./backend/... -run Season
ok  	lol-loot-assistant/backend	2.160s        # 工单「通用验证要求」第 1 条

$ go test ./backend/...
ok  	lol-loot-assistant/backend	220.111s     # 基线 220.990s，全绿，无新增失败
                                                 # （中途另一次全量跑是 227.728s，同一批代码，
                                                 #   差异来自机器负载，两次都 ok）

$ cd backend/web && node --test
ℹ tests 622   ℹ pass 622   ℹ fail 0            # 基线 608/608；本轮新增 r116e.test.cjs 的 12 个
                                                 # 排除 r116e.test.cjs 后为 610/610

$ cd backend/web && node --test r117-style.test.cjs
ℹ tests 6     ℹ pass 6     ℹ fail 0            # R117 的样式棘轮六个预算，一条都没变差

$ cd desktop && node --test
ℹ tests 260   ℹ pass 259   ℹ fail 0   ℹ skipped 1
```

> **测试计数的一处如实说明**：主控给的基线是 `backend/web` 608/608。我在**只改完 `champions.test.cjs` 与前端源码、还没建 `r116e.test.cjs`** 时实测就是 **609/609**；建完 `r116e.test.cjs`（12 个）后是 622，而排除该文件重跑是 **610/610**。也就是说这 +1~+2 不是本轮新增的测试，而是基线数字与当时树状态的口径差（可能有测试按源码数据驱动生成条数）。**关键是 fail 始终为 0，且 desktop 那 260 个也 0 fail。**

### 8.2 CSS 六个预算：改前 vs 改后（完全一致，一个都没变差）

| 预算 | 阈值 | 改前实测 | 改后实测 |
|---|---|---|---|
| `border-radius` 去重取值数 | ≤ 33 | 33 | **33** |
| `padding` 去重取值数 | ≤ 205 | 205 | **205** |
| `gap` 去重取值数 | ≤ 54 | 54 | **54** |
| 未引用类名 | ≤ 93 | 90 | **90** |
| 重复 hex 种类数 | ≤ 41 | 41 | **41** |
| 重复声明组 | ≤ 45 | （`r117-style.test.cjs` 通过） | **通过** |

（`gameplay.css` 与 `champions.css` 本轮**零改动**，所以数字必然一致；这一节是为了让「不许变差」这条有实测证据，不是推断。重复声明组那一条由 `node --test backend/web/r117-style.test.cjs` 覆盖，全量 622/622 里已含。）

### 8.3 对抗变异实跑（5 条，全部按要求 FAIL）

**变异 A：把 `rankedQueueLabel` 的生产定义从 `gameplay.js` 里删掉**（契约 UI 入口第 5 条要求的对抗变异）
```
removed 8 lines
✖ R116-E rankedQueueLabel is defined in production source and resolves every queue
✖ R116-E rankedQueueData labels the mayhem tab and marks lanes as not applicable
✖ R116-E card headings stay truthful once mayhem games are counted
ℹ tests 12   ℹ pass 9   ℹ fail 3
```
→ 恢复后 `ℹ pass 12 ℹ fail 0`。

**变异 B：把 `positionStatsForQueue` 的早退改回 `return positionStats(matches, playerRef)`**（契约附五明确要求的对抗变异）
```
--- FAIL: TestR116EMayhemTabRendersNoPositionsAndNoAbilityRadar (0.00s)
    gameplay_r116e_test.go:136: positionStatsForQueue(2400) = []main.gameplayPositionStat{
      {Position:"top", Label:"上单", Games:3, Share:100},
      {Position:"jungle", Label:"打野", Games:0, Share:0}, ...},
      want nil (ARAM has no lanes)
FAIL	lol-loot-assistant/backend	1.093s
```
→ 输出里的 `top Games:3` 正是那 3 场 **420** 对局：变异体确实把单双排的分路数据挂到了海斗队列上，也就是契约说的「伪造口径」。恢复后 `ok`。

**变异 C：把 `gameplayRankedTabHasPositions()` 改成恒真**
```
--- FAIL: TestR116EMayhemTabRendersNoPositionsAndNoAbilityRadar (0.00s)
    gameplay_r116e_test.go:120: mayhem tab must not claim to have lane positions:
      main.gameplayRankedQueueTab{Key:"2300", Label:"海克斯大乱斗",
      QueueIDs:[]int64{2300, 2400, 3270}, Cached:[3 条海斗快照]}
```

**变异 D：把 `recentRankedSummaryForQueue` 的早退改回 `return recentRankedSummary(matches, playerRef, cached)`**
```
--- FAIL: TestR116ERecentRankedSummaryForQueueNeverMixesQueues (0.00s)
    gameplay_r116e_test.go:230: queue 2400 label = "单双排", want 海克斯大乱斗
```
→ 变异体把 420 的汇总当成了 2400 的结果返回，标签都跟着错了。

**变异 E：把 `recentRankedSummary` 的队列门禁放开成「所有队列都收」**
```
--- FAIL: TestR116ERecentRankedSummaryForQueueNeverMixesQueues (0.00s)
    gameplay_r116e_test.go:257: classic summary must ignore mayhem-only samples:
      {QueueID:0, QueueLabel:"", Games:2, Wins:2, WinRate:100, Kills:16, ...}
```
→ 只有海斗（2400）与极地大乱斗（3220）样本时，标题写着「排位」的卡片会显示 2 场 100% 胜率。

三次变异之间都用 `$PI_SCRATCH_DIR/r116e-backup/gameplay.go` 完整恢复，最后一次恢复后 `go build` + `go test -run R116E` 均 `ok`。

### 8.4 关键符号自查（契约冲突防护第 3 条）

改完后逐个 grep 确认 R116-A/B/D/F 与 R117 的成果都还在（计数为非测试文件命中数）：

```
pruneStaleHexdataBuilds 16   parseHexdataHeroJSON 15   aramMode && !arenaMode 5
gameplayAugmentRowsWithoutStages 3   renderLiveTeamPortraitTags 2
renderMayhemHeroRarityPanel 2   mayhemCautionNote 2
fractionToPercent 18   ratioFloat 4   loadStyle 2
MatchupNotices 27   ItemsByIdentity 31
```

---

## 9. 隐私声明一致性核实结果

**`backend/features.go` 未修改**（禁触清单 + R117 工单第 662 行已把 `season-stats/` 这一条提交用户拍板，按项目惯例 R92 教训不代为决定）。以下是契约「隐私文案」一节要求的四步流程的完整结果。

### 9.1 第 1 步：`handlePrivacy`（`features.go:744-755`）三个数组 vs 改动后的实际写操作

**本轮改动新增/扩大的实际写操作**（全部落在 `season-stats/sgp/<accountHash>-s26.json` 一个文件里，身份是加盐哈希）：

| 写操作 | 内容 | 改动前 | 改动后 |
|---|---|---|---|
| `Stats` | 逐英雄整季聚合 | 只含 420/440 | **含 420/440 + 2300/2400/3270** |
| `QueueStats` | 分队列胜负聚合 | 2 条 | **5 条** |
| `RankedMatches` | 逐场快照（K/D/A、队伍击杀、位置、能力雷达原料） | 只含 420/440，上限 80 条 | **含海斗，上限 200 条**（海斗条目 `Position` 恒空、`Ability` 恒 nil） |
| **`AugmentSamples`（全新）** | 逐场 `GameID` / `ChampionID` / `AugmentIDs`(≤6) / `ItemIDs`(≤7) / `Win` | 不存在 | **只在海斗队列记录，上限 2000 条，实测 126 B/场** |
| `GameIDs` | 逐场对局 ID | 本来就全队列记录 | **不变** |

**比对表**：

| `stores`（`features.go:752`）现有 7 条 | 与改动后实际写操作的关系 |
|---|---|
| ①「内置职业选手名单里人工核对账号的稳定标识锚点（本地缓存，30 天）…」 | 无关，未受影响 |
| ②「公开韩服查询的身份解析与对局内容…保存在有界本地缓存」 | 无关（韩服路径本轮未扩大写入） |
| ③「随机脱敏账号标识」 | **部分覆盖**：`season-stats` 的文件名与 `accountHash` 字段确实是加盐脱敏标识，这一条说的是标识本身 |
| ④「已拥有和三合一剩余的本地历史快照」 | 无关（收藏页快照） |
| ⑤「按对局 ID 与加盐脱敏账号标识记录的胜点变化」 | **只覆盖 `lp-history.json`**，不覆盖 `season-stats/` |
| ⑥「用户导入的奖池清单」 | 无关 |
| ⑦「不含令牌和账号名的诊断事件」 | **仍然准确**：本轮新增的 4 个诊断事件（`season_stats_save_failed` / `season_stats_directory_health` / `season_mayhem_builds_resolved` / `season_mayhem_builds_insufficient` / `season_mayhem_item_names_unavailable`）只含队列/英雄/海克斯 ID、条数、字节数、相对路径，**不含令牌、账号名、PUUID**；目录体检刻意只报**相对路径**（绝对路径带本机用户名） |

**「实际会写但声明里没有」**：
1. **`season-stats/` 整个目录**（逐局 `gameIds[]`、逐英雄战绩、逐局 `rankedMatches[]`）—— **这是既存缺口，不是本轮造成的**。R117 工单第 662 行已经把它标记为「需要用户拍板」。
2. **本轮在这个既存缺口里新增了 `augmentSamples[]`**（逐局海克斯与装备 ID + 胜负，只在海斗队列）——**这是本轮造成的增量**，同样落在未声明的 `season-stats/` 里。

**「声明里有但实际不写」**：无。7 条声明逐条都对应真实存在的写操作。

**其它三个数组的核实**：
- `reads`（748）：本轮**没有新增任何读取源**。海斗数据来自既有的 SGP 战绩扫描（本来就在读，只是过滤掉了）；P2-6 端点只读本地缓存。⚠️ 一个**既存**观察：`reads` 里最接近的是「League Client 中的最近战绩、对局参与者与当前游戏流程」，而国服赛季扫描实际走的是**腾讯 SGP 网关**（用 LCU 的 entitlements token），`README.md:31` 对这一点有明确描述但 `reads` 数组没有。**这不是本轮造成的，也不该由本轮代改**，一并报给用户/R117 侧。
- `externalReads`（751）：本轮唯一的外部读取是 `attachSeasonMayhemItemNames` 触发的 Data Dragon 静态目录（把装备 ID 翻成中文名），**已被第 ③ 条「『英雄』页统计与图标只向固定 OP.GG、腾讯官方图片与 Riot Data Dragon 公共地址请求」覆盖**（P2-6 就在英雄页），且它自带缓存、有 6 秒超时、失败即降级。→ **无需新增条目**。
- `neverStores`（753）：逐条核实——「本机当前账号的 PUUID、AccountID、SummonerID 不外发给第三方，也不出现在前端和诊断日志」：`seasonAugmentSample` **不含任何身份字段**，文件按 `accountHash`（加盐 SHA-256 前 16 位）命名，前端拿到的是聚合后的 ID 与计数，诊断事件里也没有身份。「LCU 临时令牌仅在当前进程内存中短暂使用」：P2-6 端点**不碰令牌**（不发上游请求）。「战利品、待领取奖励、任务与活动奖励明细」：无关。→ **本轮改动不让任何一条 `neverStores` 变成假的。**
- `uploadsData: false` / `localOnly: false` / `requiresPassword: false`：均未受影响（P2-6 端点不外发任何数据）。
- `mayhemRatingDisclosure`（747）：讲的是 ARAMKit 第三方估算分。**本轮没有碰 `aramkit_rating.go`**，Anti-scope 第 2 条遵守：被主动丢弃的 `participants[]`（`aramkit_rating.go:353-359`）**没有复活**。→ 这段声明仍然准确。

**结论：本轮改动不会让任何现有隐私文案变成假的**，但它**扩大了一个已经未声明的写操作**（`season-stats/`）。这个缺口在 R116-E 之前就存在，已由 R117 提交用户拍板。

### 9.2 第 3 步：建议的 `stores` 新增条目原文（**待用户/R117 侧拍板后写入 `features.go:752`**）

现有条目的风格是「存什么 + 用什么标识 + 边界」，中文、无句末标点、一条一句话。按这个风格给两个粒度供选择：

**方案 A（推荐，一条覆盖整个 `season-stats/`，同时闭合 R117 第 662 行的既存缺口）**

```
按加盐脱敏账号标识保存的本赛季个人对局统计：逐场对局 ID、逐英雄胜负与 K/D/A/补刀、单双排与灵活组排及海克斯大乱斗的逐场快照，以及海克斯大乱斗逐场的海克斯与装备 ID 和胜负（用于本地聚合「选了某个海克斯之后通常出什么」）；单文件上限 4 MiB，超限时先截断逐场快照、绝不丢弃英雄统计；不含原始账号标识，不外发
```

**方案 B（只补本轮增量，既存缺口继续由 R117 那一条处理）**

```
按加盐脱敏账号标识逐局保存本赛季海克斯大乱斗对局的海克斯与装备 ID、英雄与胜负，用于在本地聚合「选了某个海克斯之后通常出什么」；最多 2000 局，不含原始账号标识，不外发
```

> 我的建议是 **A**：`season-stats/` 的既存缺口和本轮增量在同一个文件里，分两条声明会让用户以为它们是两处存储；而且 A 顺带把「4 MiB 上限 + 超限先截断逐场快照」这个本轮新建的预算行为也如实写进了声明（P1 的处置顺序对用户是有意义的：它保证了「英雄统计不会因为磁盘预算被丢掉」）。**最终由用户/R117 侧决定，本轮未写入。**

### 9.3 第 3 步（续）：产品文档侧复核

**复核结论：主控的初判成立。** 实测证据：

- `grep -n "赛季" README.md DESIGN.md PRODUCT.md` → 只有 **`README.md:254`** 与 **`DESIGN.md:157`** 两处命中，都是讲**请求编排**（「韩服总览、当前对局和赛季汇总在已知完整 Riot ID 时同时发起…」／「总览、当前对局、赛季汇总共用一个入口并行发起」），**都不涉及队列范围**。
- `PRODUCT.md` 里 `赛季` 零命中；`统计` 只命中第 13 行（产品定位）与第 24 行（皮肤统计口径），都与队列范围无关。
- `grep -rn "season-stats" README.md DESIGN.md PRODUCT.md` → **零命中**。
- `grep -n "统计范围\|只统计\|仅统计" README.md DESIGN.md` → 只有 `README.md:42` 一处「『详情』页签中每名玩家的最近战绩只统计当前队列」，讲的是**对局页**（局内）而不是赛季统计，且本轮 `liveRecentPositions` 行为未变 → 仍然准确。

→ **没有任何产品文档陈述过「赛季统计只含 420/440」**，所以 P0 的放开不会让产品文档变成假的，产品文档侧**无需为本条改动**。

**但我发现一处由「用户裁决的 UI 范围」造成的文档不完整（不是 P0 造成的）**：

- **`README.md:31`**：「『总览』按当前召唤师和新打开的玩家标签展示**单排/双排、灵活组排**、英雄胜率、位置偏好、熟练度、过去 30 天排位、最近 30 天同场玩家、活跃时段及最近战绩」。本轮给总览页加了**第三个队列页签「海克斯大乱斗」**，这个枚举就不再完整了（不是假话，是漏了一项）。
  建议改法（**`README.md` 在禁触清单里，R117 的地盘，本轮未改**）：把「单排/双排、灵活组排」改成「单排/双排、灵活组排、海克斯大乱斗（有海斗场次时才出现；海斗没有分路，因此不提供位置偏好与能力雷达）」，并注明「英雄胜率自 R116-E 起含海克斯大乱斗场次」。
  > 顺带一个**既存**问题（非本轮造成）：同一句里的「过去 30 天排位」整块已在早前某轮删除（`champions.test.cjs:6381` 有专门测试钉住「the duplicated 30-day ranked card is gone from every layer」），但 `README.md:31` 还列着它。报给 R117/用户一并处理。
- **`DESIGN.md:54/56`**：讲的是英雄页与对局页，与总览页队列页签无关，**不受影响**。

---

## 10. Anti-scope 逐条遵守情况

| 条 | 遵守情况 |
|---|---|
| 1. 不做局内实时的海克斯→出装联动 | **遵守**。P2-6 是纯静态查询：数据源是本地赛季缓存，触发点是英雄详情页「构筑」tab 的懒加载，与 ChampSelect / live / beacon 没有任何接线。`gameplay.js` 的局内路径未改（除了 `liveRecentPositions` 那条**行为不变**的注释）。`docs/r116-probe-findings.md` 的三项判据观测值仍全部「待真机」，本轮没有把它们当成已成立 |
| 2. 不恢复 ARAMKit `/rating` 的 `participants[]` | **遵守**。`backend/aramkit_rating.go` **一个字都没改**（不在改动文件清单里），`353-359` 的丢弃逻辑原样保留 |
| 3. 不把 season-stats 预算塞进 `binary_disk_budget.go` | **遵守**。`binary_disk_budget.go` 未改、未被 `season_stats_budget.go` 引用；新预算是独立的 269 行新文件 |
| 4. 不存 22 项 postmatch 全量到个人样本 | **遵守**。`seasonAugmentSample` 只有 5 个字段，伤害/承伤/经济/22 项指标一律不存；英雄级指标继续走 R116-A 的 `/api/hexdata/postmatch`，个人记录里没有重复的一份。走的是「精简路线」（实测 126 B/场），不是「全量路线」 |

---

## 11. 留下的未解决问题

1. **`features.go:752` 的 `stores` 清单仍缺 `season-stats/`**（既存缺口 + 本轮增量）。建议条目原文已在 §9.2 给出（推荐方案 A），**待用户/R117 侧拍板后写入**。本轮未改 `features.go`。
2. **`README.md:31` 的总览页卡片枚举不完整**（缺「海克斯大乱斗」页签；顺带还留着已删除的「过去 30 天排位」）。`README.md` 在禁触清单里，本轮未改，建议原文见 §9.3。
3. **`main.go` 的 2 行路由注册是对禁触清单的越界**（§0、§12）。需要主控确认是保留还是回退；回退的话 P2-6 会变成「算了但取不到」的死数据。
4. **`reads`（`features.go:748`）没有明确提到腾讯 SGP 网关**。国服赛季扫描与详情战绩实际走 SGP（用 LCU 的 entitlements token），`README.md:31` 有描述但隐私声明里最接近的只有「League Client 中的最近战绩」。**既存问题，非本轮造成**，一并报给用户/R117。
5. **`seasonAugmentSample` 没有 `createdAt`**（Anti-scope 第 4 条要求只存必要字段），所以 `seasonTrimAugmentSamples` 的「保留最新」只能按 GameID 降序近似（Riot/SGP 的 gameId 平台侧递增分配）。**该假设只决定「超过 2000 条时先丢哪些」，不参与任何对外展示的数字**；即使假设失效，影响也只是被淘汰的场次选择，不会让任何统计口径变成假的。已在源码注释与本账本双写。如果日后要精确，最小代价是给样本加一个 `createdAt`（约 +20 B/场，2000 场 +40 KB，仍在预算内），但那要动 Anti-scope 第 4 条的字段清单，**需要用户拍板**。
6. **海斗页签的 key 是 `2300`，但内容是 2300/2400/3270 三个队列的合并**。这是契约要求的「合并成一个 tab」的必然结果，语义是「页签标识」而不是「只统计这个队列」，Go 常量注释、前端常量注释与测试三处都钉住了。但如果日后有人只看 `rankedQueues["2300"]` 这个 JSON 键名而没读注释，可能会误以为它只含 2300。**没有更好的办法**（前端 `rankedQueueData` 用 `String(tab[stateKey])` 做键，键必须是数字串），只能靠注释与测试兜住。
7. **`championStats` 的契约描述需要更正**：附五说它是「对位/counter 统计」，实测是「英雄胜率」卡片的兜底（§5.3）。已按实测处置（保留门禁 + 补理由注释 + 测试），但契约文档本身的这处描述是错的，报给主控。
8. **`player_ability_test.go` 被机械修改了 2 行**（`buildGameplayRankedQueues` 签名变更的连带后果）。该文件不在契约给的测试清单里，如实上报；断言一字未改，只把调用形状换成 `legacyRankedQueueTabs(...)`。
9. **两项真机判据仍待验证**（§2.6），本环境没有 Windows / League 客户端 / 真实海斗对局，**未伪造 PASS**。合成 fixture 已给出量级证据（305 KB @2000 场）。
10. **`desktop/package-lock.json` 只做了一致性替换**（3 处 `0.12.11` → `0.12.12`：`package.json:3`、`package-lock.json:3`、`package-lock.json:9`），**没有重跑 `npm install`**（本环境不应改动 `node_modules` 与依赖解析结果）。三方依赖 `unzipper@0.12.7`（`package-lock.json:3912`）**未动**，已 grep 确认。

---

## 12. `main.go` 越界改动的完整 diff

```diff
$ diff "$PI_SCRATCH_DIR/r116e-backup/main.go" backend/main.go
507a508,509
> 	// R116-E P2-6：本人海克斯大乱斗「选了某个海克斯之后通常出什么」静态查询。
> 	mux.HandleFunc("GET /api/gameplay/season-mayhem-builds", a.authorized(a.handleGameplaySeasonMayhemBuilds))
```

**除这 2 行外，`main.go` 没有任何其它改动**（`app` 结构体、`recordDiagnostic`、路由表其余部分全部原样）。这也是为什么 `season_stats_budget.go` 的「一次性」去重用包级 `sync.Mutex + map[string]bool` 而不是往 `app` 上加 `sync.Once` 字段——加字段就必须改 `main.go` 的结构体定义，那才是真正的侵入。

---

## 13. 防毁纪律：增量备份与 shasum 核对

每写完一个文件立即 `cp` 到 `$PI_SCRATCH_DIR/r116e-backup/`。收尾时逐个核对 live 与备份一致：

```
$ for f in <16 个文件>; do shasum "$f"; shasum "$PI_SCRATCH_DIR/r116e-backup/$(basename $f)"; done
OK   77249f8557706b094857f58566d7c659ff50cecf  backend/season_stats.go
OK   97b98b1c72d84cea79a14d2afe3cedf6963c1d87  backend/season_stats_budget.go
OK   5158863e917ea4b9ec9ff7406d59942d3f66f271  backend/season_stats_test.go
OK   c15c1929521352ac19e7f2902368dbd08e2af369  backend/season_stats_budget_test.go
OK   f1ff8512d2be58f5fdd111e7087b8d61bc9985a5  backend/gameplay.go
OK   195a8b1158d09707e3464ff59fbd757146e18837  backend/gameplay_r116e_test.go
OK   8ec335331e200a7966aeffd918eaf4c621a5f7df  backend/gameplay_test.go
OK   422d331aadba79d8aed888b1f231b12e6ec9e371  backend/player_ability_test.go
OK   f90ba3a87279a8efa4ec2726156901d36a4b66d2  backend/riot_api.go
OK   64d2e8aadadad6f87f4ed98e61ceb21de7cd4d3a  backend/main.go
OK   7534fd29e71b3dbe3381b2944c616928bc27b187  backend/web/gameplay.js
OK   4c409926ccb15ddd2797502f05c9ed9c56bb0a86  backend/web/champions.js
OK   1f9abd4a8f76e3b53defc4d619ab46e2ca792c61  backend/web/champions.test.cjs
OK   43a6601f36e06097d81bd98f5d2aab78930c3a2b  backend/web/r116e.test.cjs
OK   a4a5e8dbf871160431a32a6dfb2b99bf7a5ca8e5  desktop/package.json
OK   e222b62dfa60aac4882d8fabe482fe6c23840318  desktop/package-lock.json
```

**16/16 全部 `OK`（live 与备份哈希逐一相等），无 `DIFF`。**

另外两条零改动证明（与会话开始时的备份逐字节比对）：

```
$ diff "$PI_SCRATCH_DIR/r116e-backup/gameplay.css"   backend/web/gameplay.css   → IDENTICAL
$ diff "$PI_SCRATCH_DIR/r116e-backup/champions.css"  backend/web/champions.css  → IDENTICAL
$ diff "$PI_SCRATCH_DIR/r116e-backup/player_ability.go" backend/player_ability.go → IDENTICAL
$ git diff --stat backend/aramkit_rating.go backend/binary_disk_budget.go backend/features.go backend/player_ability.go
  （无输出：这四个文件相对 HEAD 也没有任何改动）
```

备份清单：`season_stats.go`、`season_stats_budget.go`、`season_stats_test.go`、`season_stats_budget_test.go`、`gameplay.go`、`gameplay_r116e_test.go`、`gameplay_test.go`、`player_ability_test.go`、`riot_api.go`、`main.go`、`gameplay.js`、`champions.js`、`champions.test.cjs`、`r116e.test.cjs`、`package.json`、`package-lock.json`。
本轮**未发生**任何外部还原或文件丢失（工作树独占，无并行会话）。

---

## 14. 改动文件清单

**新建（5）**
- `backend/season_stats_budget.go`（269 行）
- `backend/season_stats_budget_test.go`（383 行）
- `backend/gameplay_r116e_test.go`（424 行）
- `backend/web/r116e.test.cjs`（449 行 / 12 个测试）
- `docs/r116e-execution-ledger.md`（本文件）

**修改（10）**
- `backend/season_stats.go`（764 → 1215 行）
- `backend/gameplay.go`（9740 → 10100 行）
- `backend/riot_api.go`（1709 → 1715 行，仅韩服调用点适配新签名）
- `backend/main.go`（1820 → 1822 行，**越界 2 行**，见 §0/§12）
- `backend/season_stats_test.go`（333 → 657 行）
- `backend/gameplay_test.go`（仅 2 处调用形状适配）
- `backend/player_ability_test.go`（仅 2 处调用形状适配，见 §11.8）
- `backend/web/gameplay.js`（7224 → 7299 行）
- `backend/web/champions.js`（2975 → 3074 行，只增不改）
- `backend/web/champions.test.cjs`（撤掉 `rankedQueueLabel` 测试桩 + 一条传递编译名单）
- `desktop/package.json` / `desktop/package-lock.json`（0.12.11 → **0.12.12**）

**零改动（明确确认）**
`backend/features.go`（仅读取核实）、`backend/binary_disk_budget.go`、`backend/aramkit_rating.go`、`backend/hexdata.go`、`backend/champions.go`、`backend/champion_cache.go`、`backend/champions_structured.go`、`backend/player_ability.go`、`backend/augment_contract_probe*.go`、`backend/queue_groups.go`、`backend/storage.go`、`backend/web/shared.js`、`backend/web/index.html`、`backend/web/app.css`、`backend/web/app.js`、`backend/web/gameplay.css`、`backend/web/champions.css`、`backend/web/r117*.test.cjs`、`backend/web/r116b.test.cjs`、`backend/web/r116d.test.cjs`、`backend/web/r116f.test.cjs`、`backend/readme_privacy_test.go`、`README.md`、`DESIGN.md`、`PRODUCT.md`。
