# R116-A 执行账本：Hexdata 数据源由 HTML 抓取替换为 JSON API（含全量聚合文件）

执行日期：2026-09-20 ｜ 版本：`desktop/package.json` 0.12.7 → **0.12.8**（`desktop/package-lock.json` 的两处顶层 version 同步）
工单原文：`docs/history/worklists/R116-A-Hexdata数据源替换与聚合文件接入-工单.md`
实测依据：`docs/r116-proposal-feasibility-review.md` 第 1 / 1.1 / 1.2 / 4.5 / 4.6 节
真实上游响应夹具：`backend/testdata/r116/`（本工单未改动这四份文件）

> 本账本记录「实际做了什么、验证结果如何、留下哪些未解决问题」，行号一律是**改动后**的新行号。

---

## 1. 实际改动清单

### 1.1 `backend/hexdata.go`（2418 → 3188 行）

| 位置（新行号） | 内容 |
|---|---|
| 41-60 | 新增常量：`hexdataMetaPath`、`hexdataPostmatchPath`、`hexdataHextechInsightsPath`、`hexdataHeroJSONPathPrefix`、`hexdataAggregateID="all"`、`hexdataAggregateCanonical="/heroes"`、`hexdataTrioKeepLimit=50`、`hexdataAggregateHeroMin/Max=150/200`、`hexdataAggregateAugmentMin/Max=190/230`；删除 `mayhemAugmentIDFloor`、`mayhemAugmentCopyConcurrency`、`mayhemAugmentSlugFailureTTL`、`mayhemAugmentCopyTTL` |
| 69-71 | 新增 `hexdataHeroJSONPathPattern = ^/api/hexdata/heroes/([0-9]+)$`；删除已成孤儿的 `hexdataTierPattern`（只被删掉的 `parseHexdataHeroDetail` 用过） |
| 190-195 | `hexdataClient` 新增 `pruneMu` / `prunedBuildID` / `pruneWait`（P3 异步清理的去重与测试等待点） |
| 217-570 | 新增全部 JSON API 数据结构（见第 3 节原文） |
| 777-830 | `inspectHexdataPayload` 新增 4 个 case：`"meta"`、`"hero-json"`、`"postmatch"`、`"hextech-insights"`，全部做真实结构校验并用 `firstError` 包错误 |
| 869-877 | `allowedPath` 放行 4 个 JSON 端点；**不放行** `/api/hexdata/heroes`（无 ID 列表接口）；两条 HTML 正则保留（理由见 5.2） |
| 961 / 1036 | `load()` 与 `promote()` 的 TTL 判断改为 `kind == "answer" \|\| kind == "meta"`（meta 走 12h 软 TTL） |
| 1122-1125 | 出站请求补 `request.Header.Set("Referer", "https://"+hexdataHost+"/")` |
| 1192 | `recordSuccess` 的 `BuildChecked` 更新条件加上 `hexdataMetaPath`（meta 也是一次 build 检查） |
| 1290-1312 | `adoptCitation` 增加「buildID 变化 → 异步清理」触发点 |
| 1314-1335 | 新增 `adoptMeta(hexdataMetaSnapshot) bool`（JSON kind 的 citation/buildID 落地点） |
| 1337-1364 | 新增 `pruneStaleBuildsAsync(buildID)`：goroutine 内调 `cache.pruneStaleHexdataBuildsCount`，每个 (进程, buildID) 只扫一次盘，结果写 `hexdata_build_prune` 诊断事件 |
| 1413-1425 | `reportHexdataHeroShape` 改签名收 `hexdataHeroDetailV2 + citation`，kind 由 `"hero"` 改为 `"hero-json"`；新增 `reportHexdataHeroJSONStats`（丢弃/裁剪计数诊断） |
| 1794-2320 | 新增 JSON API 实现段：`hexdataParseID`/`hexdataParseIDs`、6 个 `convert()`、`hexdataTrimTrios`、`parseHexdataMeta`、`hexdataJSONCitation`、`loadHexdataMeta`、`parseHexdataHeroJSON`、`parseHexdataHeroJSONWithStats`、`hexdataRarityLabel`、`hexdataAugmentMetricRows`、`hexdataItemMetricRows`、`decorateHexdataItemAssets`、`parseHexdataPostmatch`、`loadHexdataPostmatch`、`parseHexdataHextechInsights`、`loadHexdataHextechInsights` |
| 2582-2625 | `decorateHexdataAugments` **保留**，仅删掉原第 1679 行那句 `p.hydrateMayhemAugmentCopy(ctx, rows)` |
| 2659-2762 | `loadMayhemDetail` 重写：先 `loadHexdataMeta`（拿 citation + 落 buildID，否则 cache key 退回 `bootstrap\|`），再 `load(ctx,"hero-json",id,"/api/hexdata/heroes/{id}","application/json",false)`，成功后 `promote("hero-json",…)` |

`loadMayhemDetail` 关键行号：2694（hero-json 抓取）、2713（promote）、2715（recordSuccess）、2735-2738（`hexdataAugmentMetricRows` / `hexdataItemMetricRows` / `decorateHexdataAugments` / `decorateHexdataItemAssets`）。

### 1.2 `backend/champion_cache.go`（528 → 603 行）

| 位置 | 内容 |
|---|---|
| 472-475 | `hexdataStaleBuildGrace = 8 * 24 * time.Hour`（2 个 buildID 周期） |
| 483-486 | `func (c *championDataCache) pruneStaleHexdataBuilds(currentBuildID string) error`（工单要求的签名） |
| 488-546 | `pruneStaleHexdataBuildsCount(currentBuildID) (int, error)`：扫 `hexdata-*.json` → `readCacheFile` → `strings.SplitN(entry.Key,"|",2)[0]` 取 buildID → 不等于 current 且 `cacheNow().Sub(FetchedAt) >= 宽限期` 才删；时间通过既有的 `c.now`/`cacheNow()` 注入 |

**未修改** `pruneDiskLocked` 里的 `protected` 判断（Anti-scope 第 4 条），并有测试 `TestPruneDiskStillProtectsHexdataFilesFromGenericLRU` 用源码断言把它钉住。

### 1.3 `backend/champions.go`（仅新增结构体字段，未动 `applyLocalAugmentGrades`）

`championMetricRow`（254-283）在 `SkillOrder` 之后新增 9 个官方口径字段：
`DeltaWinRate`(`deltaWinRate`)、`WilsonLowerWinRate`(`wilsonLowerWinRate`)、`HexTier`(`hexTier`)、`HexLabel`(`hexLabel`)、`HexTierColor`(`hexTierColor`)、`OfficialTier`(`officialTier`)、`CoreDelta`(`coreDelta`)、`AverageIndex`(`averageIndex`)、`WithoutItemWinRate`(`withoutItemWinRate`)。
字段名/JSON tag 与 R116-B 工单第 22、38、89 行的要求逐字对齐（B 不必再改结构体）。

`championRankingRow` **未新增字段**：工单第 59 行提到的「总榜位次」官方口径来自 `hextech-insights.heroes[].tier`，接线是 R116-B P0-5-2 的事，本工单的解析层没有可填的真实值，加空字段只会是死重量。

### 1.4 测试与文档

- 新增 `backend/hexdata_r116a_test.go`（21 个测试）
- 新增 `backend/champion_cache_r116a_test.go`（9 个测试）
- 修改 `backend/hexdata_test.go`：删除 6 个只覆盖已删函数的测试（见 4.2），`TestHexdataHeroShapeReportsActualRecommendationAndItemCounts` 改为新签名 + kind `hero-json`，移除不再使用的 `strconv` import
- 新增 `docs/r116a-execution-ledger.md`（本文件）
- `desktop/package.json` + `desktop/package-lock.json` → 0.12.8

---

## 2. 逐条验证判据的真实状态

图例：**PASS** = 已用自动化测试验证并通过；**待真机** = 需要 Windows + League 客户端，本环境无法执行；**未做** = 明确没做并给出原因。

### P0（客户端基础设施）

| 判据 | 状态 | 证据 |
|---|---|---|
| 1. 有 Referer 通 / 无 Referer 被 403 拒（两条测试） | **PASS** | `TestHexdataRequestCarriesRefererAndUpstreamAccepts`、`TestHexdataLoadFailsWhenRefererIsMissing`（后者用一层 transport 摘掉 Referer 模拟变异，断言 `championUpstreamHTTPStatus(err)==403`） |
| 2. 4 个 JSON 路径放行、`/api/hexdata/heroes` 拒绝 | **PASS** | `TestHexdataAllowedPathCoversJSONAPIAndRejectsHeroList`（17 条表驱动，含 `/heroes`、`/heroes/`、`/heroes/abc`、`/heroes/157/trios`、`/heroes/-1`、`/api/hexdata/augments*` 全部 false） |
| 3. 缺 `augments` 的假 hero-json 必须报错 | **PASS** | `TestInspectHexdataPayloadRejectsHeroJSONWithoutAugments`，并断言错误文本含 `augments=0` |
| 4. `go test ./backend -run Hexdata` 全绿、新增测试 ≥ 8 | **PASS** | 新增 **30** 个测试（要求 ≥8）；`-run Hexdata` ok 7.47s |
| meta 作为 kind `"meta"` 接入、12h 软 TTL、citation 全部来自快照 | **PASS** | `hexdata.go:961/1036`（TTL）、`1963-1987`（`loadHexdataMeta`）、`1952-1961`（`hexdataJSONCitation`）；`TestHexdataJSONCitationComesFromMetaSnapshotAndPointsAtHTMLPage` 断言 CanonicalURL 不含 `/api/` |
| 三个新 kind 成功路径都 promote | **PASS** | `hexdata.go:1980`（meta）、`2226`（postmatch）、`2301`（hextech-insights）、`2713`（hero-json）；`TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` 断言 4 个 key 都能 `readDisk` 出来 |
| postmatch/hextech-insights 结构校验阈值 | **PASS** | `TestInspectHexdataPayloadValidatesPostmatch`（173 条通过；1 条拒绝；逐条缺 `kda`/`avgDamage` 都被抓到）、`TestInspectHexdataPayloadValidatesHextechInsights`（173/211 通过；8/8、173/100、201/211 全拒） |

**对抗变异（P0）**
- 「删掉 `augments` 空值检查」→ 同一测试里既断言 `augments:[]` 报错、又断言补一条就通过，证明护栏不是同义反复：**PASS**
- 「新 kind 忘记 promote → 12 小时后强制回源」→ `TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` 把 `cache.now` 与 `hexdata.now` 一起推到 +13h 并让上游整体 500，断言 postmatch / hextech-insights / mayhem 详情仍能拿数据（走 promote 落盘的 stale 兜底），且确实发出过 revalidate 请求。删掉任一 promote 调用该测试立刻失败：**PASS**

### P1（英雄详情页数据源替换）

| 判据 | 状态 | 证据 |
|---|---|---|
| 1. 冷启动请求数 = 2（hero-json + meta） | **PASS（有一处需要主 Agent 知情）** | `TestLoadMayhemDetailColdHeroCostsTwoHexdataRequestsWithWarmSiteMetadata` 断言路径列表恰为 `[/api/hexdata/meta, /api/hexdata/heroes/157]`；`TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly` 断言 `/augment/*` 与 `/hero/*` HTML 请求数为 0。**知情点**：真·全冷启动还有第 3 条既有请求 `/api/hexdata/answer-cards`，它是 `measurementTechnique` 的唯一来源（meta 里没有这个字段），12h 软 TTL、与榜单页共用，工单没有要求移除，删掉会让统计口径说明消失（触数据准确性红线）。见第 6.1 条 |
| 2. `RecommendedAugments` 的 `Description` 非空 ≥95% | **PASS** | 真实夹具 10/10 满覆盖，测试断言比例 ≥0.95 |
| 3. `ItemRanking` 资产 ID == JSON `itemId`，名称重复/特殊字符不走错装备 | **PASS** | `TestLoadMayhemDetailItemIDsSurviveDuplicateAndSpecialCharacterNames`：两条都叫「无尽之刃（改）」的装备（3031/3032）+ 一条含 `· 特殊/字符\` 的名称，在 Data Dragon 目录 404 的情况下 ID 与名称仍逐条对得上；主测试另用真实夹具逐行比对 itemId/itemName/tier/hexTier/hexLabel |
| 4. `-run Mayhem` 全绿 + 全量编译通过（无孤儿调用点） | **PASS** | `go test ./backend/... -run Mayhem` ok 0.86s；`go build -o …`、`go vet ./backend` 均干净 |
| `trios` / `terminalItemTrios` 裁剪到按 games 降序前 50 | **PASS** | `hexdataTrimTrios`（1909-1925）+ `TestParseHexdataHeroJSONTrimsTriosToTopFiftyByGames`（1000→50，裁掉 950，排序单调、两次解析结果一致） |
| 字符串 ID 安全转换，非法值整条丢弃 | **PASS** | `hexdataParseID`/`hexdataParseIDs`（1802-1830）+ `TestParseHexdataHeroJSONDropsRowsWithUnparseableIDs`（8 个数组共丢 10 条，保留行里没有一个 0 值 ID） |

**对抗变异（P1）**
- 「把 `augmentDescription` 设为空字符串，确认走占位而不是显示空描述」→ `TestEmptyAugmentDescriptionDegradesToOfflinePlaceholder`：空描述被 `augmentDescriptionWithOfflineGuidance` 换成既有占位文案 `海克斯图鉴中可读取说明`，真描述不被覆盖，序列化后的 payload 里不出现 `"description":""`：**PASS**（前端渲染层的「整块隐藏」属 R116-B，本工单不动 `backend/web/**`）
- 「故意留一个对 `fetchMayhemAugmentCopy` 的死引用，确认编译/vet 能捕获」→ 工单说明这条不要求新增测试；额外加了源码守卫 `TestHexdataDeadHTMLChainIsGone`，检查 6 个已删函数的**定义与调用形式**、4 个已删常量、`hexdata_item_match` 事件、`load(ctx, "hero",` 调用是否复活，同时钉住「必须保留 `decorateHexdataAugments`」「不得出现 `augmentIconUrl`/`itemImageUrl`」：**PASS**

### P2（两个全量聚合文件）

| 判据 | 状态 | 证据 |
|---|---|---|
| 1. 连续两次调用只发 1 次 HTTP 请求 | **PASS** | `TestLoadHexdataPostmatchFetchesOnceAcrossTwoCalls`、`TestLoadHexdataHextechInsightsFetchesOnceAndParsesIDs`（均断言对应路径计数 == 1） |
| 2. `heroes` 长度在 150~200 | **PASS** | 同上（173）；另 `parseHexdataHextechInsights` 对空 payload 报错 |
| `id` 传 `"all"`、TTL 参考 `hexdataCacheTTL`、落盘走 `hexdata-` 前缀通道 | **PASS** | `hexdataAggregateID="all"`；kind 非 answer/meta 时 `load()` 用 `hexdataCacheTTL`；`TestHexdataJSONKindsSurviveSoftTTLBecauseTheyArePromoted` 断言 `{build}\|postmatch\|all`、`{build}\|hextech-insights\|all` 能 `readDisk`，且目录里 `hexdata-*.json` ≥4 个 |
| 22 项指标 / topItems / topAugments 字段级保留 | **PASS** | `TestLoadHexdataPostmatchFetchesOnceAcrossTwoCalls` 校验 hero 157 的 `kda=2.597684`、`avgDamage=41778.020045`、`pentaKills=29056`、三/四杀计数非零；insights 校验 `topItems[5]`/`topAugments[5]`/`rarity`/`confidence`/`coverageHeroCount` |

### P3（`hexdata-` 前缀文件回收）

| 判据 | 状态 | 证据 |
|---|---|---|
| 1. 两个不同 buildID 的落盘文件，旧的删、新的留 | **PASS** | `TestPruneStaleHexdataBuildsRemovesExpiredBuildsOnly`（另断言 `hexdata-state.json`、`proseed-*`、`bootstrap\|` key 的文件都不动） |
| 2. 宽限期内不立即删（时间可注入，不真等 8 天） | **PASS** | `TestPruneStaleHexdataBuildsHonorsGracePeriod`：文件年龄 8 天−1 小时保留、8 天+24 小时删除，时间由 `cache.now` 注入 |
| 3. 173 英雄 × 24 patch 等比例验证磁盘不会无限增长 | **PASS** | `TestPruneStaleHexdataBuildsBoundsDiskAcrossPatchHistory`：24 build × 20 文件 = 480 个落盘文件，prune 后只剩当前 build 的 20 个，目录字节数降到 1/10 以下，且逐个校验幸存文件的 `entry.Key` 前缀 == 当前 buildID |
| 触发点异步、不阻塞请求主路径 | **PASS** | `TestAdoptingNewBuildIDPrunesStaleHexdataFilesAsynchronously`（走 `loadHexdataMeta` → `adoptMeta` 生产路径，`pruneWait.Wait()` 后断言旧文件已删、宽限期内文件保留、`hexdata_build_prune` 诊断 `removed==1`；并验证同一 buildID 不重复扫盘、`adoptCitation` 触发点同样生效）、`TestPruneStaleHexdataBuildsMakesNoRequests`（清理全程零网络请求） |
| 健壮性 | **PASS** | `TestPruneStaleHexdataBuildsIgnoresUnreadableFiles`（垃圾内容/无 Key/无 FetchedAt 的文件一律不删，无 FetchedAt 退回 mtime）、`TestPruneStaleHexdataBuildsRequiresValidBuildID`（空/`bootstrap`/缺前缀的 buildID 直接 no-op）、`TestPruneStaleHexdataBuildsReportsMigrationFailure`（migrationErr 上抛且不误删；readFile 注入失败时不删不报错） |
| Anti-scope 4：不动 `protected` | **PASS** | `TestPruneDiskStillProtectsHexdataFilesFromGenericLRU`：超预算 LRU 仍然杀不掉 `hexdata-`/`proseed-` 文件、但能杀掉未受保护文件，并源码断言 `protected := strings.HasPrefix(...)` 那行原样存在 |

### 通用验证要求

| 项 | 状态 | 说明 |
|---|---|---|
| `go build -o "$PI_SCRATCH_DIR/dl-build" ./backend` | **PASS（工作区实测）** | 二进制已产出（22.5 MB，19:28）。执行中途工作区曾被并行任务改到编译不过（`season_stats.go`、`pro_profiles.go`），当时先在隔离副本 `$PI_SCRATCH_DIR/tc2` 里验证；并行任务收敛后**已在真实工作区重跑并通过** |
| `go vet ./backend` | **PASS（工作区实测，含测试文件）** | 输出为空，exit 0 |
| `go test ./backend/... -run Hexdata` | **PASS（工作区实测）** | ok 7.273s |
| `go test ./backend/... -run Mayhem` | **PASS（工作区实测）** | ok 0.858s |
| 全量 `go test ./backend/...` | **PASS（工作区实测，全绿）** | `ok lol-loot-assistant/backend 212.843s`，exit 0，无 FAIL。基线（改动前）同样全绿，未引入新失败。隔离副本期间曾见过的 3 个失败（loot-icons PNG 尺寸、R104 pro 目录投影、specialist runes 负缓存）都来自并行任务的在途改动，它们收敛后已自行消失 |
| `go test -race`（本工单范围） | **PASS（工作区实测）** | `-race -run 'Hexdata\|Prune\|Mayhem\|Inspect\|ParseHexdata\|LoadMayhem\|LoadHexdata\|Adopting\|EmptyAugment\|Referer'` ok 11.861s。过程中 `-race` 抓到过一处**测试自身**的竞争（`writeDisk` 异步排的 `pruneDisk` 与测试改 `migrationErr`/`diskMax*` 并发），已加 `r116aDrainDiskPrune` 排空后复跑干净；生产代码无竞争 |
| 版本号同步 0.12.8 | **PASS** | `desktop/package.json:3`、`desktop/package-lock.json:3,9`（第 3912 行的 `unzipper@0.12.7` 是第三方依赖版本，未动）。`AGENTS.md` 里「当前版本：0.12.7」不在本工单可改文件清单内，未改 |
| `cd backend/web && node --test` | **541 tests / 540 pass / 1 fail** | 唯一失败项由本工单引起，需要主 Agent 决策，见第 6.2 条（未自行修改 `backend/web/**`） |

---

## 3. 新数据结构最终定义（R116-B / D / E / F 直接消费）

`backend/hexdata.go:217-570`，以下为**文件里的原文**：

```go
type hexdataMetaSnapshot struct {
	BuildID      string `json:"buildId"`
	ReportPatch  string `json:"reportPatch"`
	ReportDate   string `json:"reportDate"`
	HeroCount    int    `json:"heroCount"`
	AugmentCount int    `json:"augmentCount"`
}

type hexdataHeroDetailV2 struct {
	Items              []hexdataItemRow      `json:"items"`
	Augments           []hexdataAugmentRowV2 `json:"augments"`
	Trios              []hexdataTrioRow      `json:"trios"`
	WeakAgainst        []hexdataMatchupRow   `json:"weakAgainst"`
	StrongAgainst      []hexdataMatchupRow   `json:"strongAgainst"`
	TeammateSynergies  []hexdataSynergyRow   `json:"teammateSynergies"`
	TerminalItemTrios  []hexdataTrioRow      `json:"terminalItemTrios"`
	SummonerSpellPairs []hexdataSpellPairRow `json:"summonerSpellPairs"`
	HeroWinRate float64 `json:"heroWinRate,omitempty"`   // 官方 heroWinRate×100，见源码注释
}

type hexdataItemRow struct {
	ItemID             int     `json:"itemId"`
	ItemName           string  `json:"itemName"`
	WinRate            float64 `json:"winRate"`
	PickRate           float64 `json:"pickRate"`
	Games              int     `json:"games"`
	Tier               int     `json:"tier"`
	HexTier            string  `json:"hexTier"`
	HexLabel           string  `json:"hexLabel"`
	HexTierColor       string  `json:"hexTierColor"`
	HexScore           float64 `json:"hexScore"`
	DeltaWinRate       float64 `json:"deltaWinRate"`
	CoreDelta          float64 `json:"coreDelta"`
	AverageIndex       float64 `json:"averageIndex"`
	WilsonLowerWinRate float64 `json:"wilsonLowerWinRate"`
	WithoutItemWinRate float64 `json:"withoutItemWinRate"`
	WithoutItemGames   int     `json:"withoutItemGames"`
}

type hexdataAugmentRowV2 struct {
	AugmentID           int                      `json:"augmentId"`
	AugmentName         string                   `json:"augmentName"`
	AugmentDescription  string                   `json:"augmentDescription"`
	Rarity              string                   `json:"rarity"`   // 上游中文串：棱彩/黄金/白银
	Tier                int                      `json:"tier"`
	Score               float64                  `json:"score"`
	HexTier             string                   `json:"hexTier"`
	HexLabel            string                   `json:"hexLabel"`
	HexTierColor        string                   `json:"hexTierColor"`
	HexScore            float64                  `json:"hexScore"`
	PickRate            float64                  `json:"pickRate"`
	PairWinRate         float64                  `json:"pairWinRate"`
	HeroWinRate         float64                  `json:"heroWinRate"`
	DeltaWinRate        float64                  `json:"deltaWinRate"`
	WilsonLowerWinRate  float64                  `json:"wilsonLowerWinRate"`
	RecommendationScore float64                  `json:"recommendationScore"`
	Wins                int                      `json:"wins"`
	Games               int                      `json:"games"`
	Stages              []hexdataAugmentStageRow `json:"stages"`
}

type hexdataAugmentStageRow struct {
	Stage                int     `json:"stage"`
	Tier                 int     `json:"tier"`
	HexTier              string  `json:"hexTier"`
	HexLabel             string  `json:"hexLabel"`
	HexTierColor         string  `json:"hexTierColor"`
	HexScore             float64 `json:"hexScore"`
	WinRate              float64 `json:"winRate"`
	PickRate             float64 `json:"pickRate"`
	DeltaWinRate         float64 `json:"deltaWinRate"`
	WilsonLowerWinRate   float64 `json:"wilsonLowerWinRate"`
	RecommendationScore  float64 `json:"recommendationScore"`
	StageBaselineWinRate float64 `json:"stageBaselineWinRate"`
	Wins                 int     `json:"wins"`
	Games                int     `json:"games"`
}

type hexdataTrioRow struct {          // trios[] 与 terminalItemTrios[] 共用
	TrioKey            string   `json:"trioKey"`
	AugmentIDs         []int    `json:"augmentIds,omitempty"`
	AugmentNames       []string `json:"augmentNames,omitempty"`
	ItemIDs            []int    `json:"itemIds,omitempty"`
	ItemNames          []string `json:"itemNames,omitempty"`
	WinRate            float64  `json:"winRate"`
	PickRate           float64  `json:"pickRate"`
	DeltaWinRate       float64  `json:"deltaWinRate"`
	HeroWinRate        float64  `json:"heroWinRate"`
	WinRateTier        int      `json:"winRateTier,omitempty"`
	PickRateTier       int      `json:"pickRateTier,omitempty"`
	Tier               int      `json:"tier,omitempty"`
	WilsonLowerWinRate float64  `json:"wilsonLowerWinRate,omitempty"`
	Wins               int      `json:"wins"`
	Games              int      `json:"games"`
}

type hexdataMatchupRow struct {       // weakAgainst[] / strongAgainst[]
	OpponentChampionID     int     `json:"opponentChampionId"`
	OpponentName           string  `json:"opponentName"`
	OpponentHref           string  `json:"opponentHref"`
	CounterDelta           float64 `json:"counterDelta"`
	Evidence               string  `json:"evidence"`
	ConfidenceLow          float64 `json:"confidenceLow"`
	ConfidenceHigh         float64 `json:"confidenceHigh"`
	QValue                 float64 `json:"qValue"`
	AdjustedWinRate        float64 `json:"adjustedWinRate"`
	ExpectedWinRate        float64 `json:"expectedWinRate"`
	ObservedWinRate        float64 `json:"observedWinRate"`
	TargetWinRateDrop      float64 `json:"targetWinRateDrop"`
	ExpectedTargetWinRate  float64 `json:"expectedTargetWinRate"`
	TargetAdjustedWinRate  float64 `json:"targetAdjustedWinRate"`
	TargetBaselineWinRate  float64 `json:"targetBaselineWinRate"`
	CounterBaselineWinRate float64 `json:"counterBaselineWinRate"`
	Wins                   int     `json:"wins"`
	Games                  int     `json:"games"`
}

type hexdataSynergyRow struct {       // teammateSynergies[]
	TeammateChampionID    int     `json:"teammateChampionId"`
	TeammateName          string  `json:"teammateName"`
	TeammateHref          string  `json:"teammateHref"`
	SynergyDelta          float64 `json:"synergyDelta"`
	Evidence              string  `json:"evidence"`
	ConfidenceLow         float64 `json:"confidenceLow"`
	ConfidenceHigh        float64 `json:"confidenceHigh"`
	PValue                float64 `json:"pValue"`
	QValue                float64 `json:"qValue"`
	AdjustedWinRate       float64 `json:"adjustedWinRate"`
	ExpectedWinRate       float64 `json:"expectedWinRate"`
	ObservedWinRate       float64 `json:"observedWinRate"`
	FirstBaselineWinRate  float64 `json:"firstBaselineWinRate"`
	SecondBaselineWinRate float64 `json:"secondBaselineWinRate"`
	Wins                  int     `json:"wins"`
	Games                 int     `json:"games"`
}

type hexdataSpellPairRow struct {     // summonerSpellPairs[]
	SpellIDs           []int    `json:"spellIds"`
	SpellNames         []string `json:"spellNames"`
	SpellKey           string   `json:"spellKey"`
	WinRate            float64  `json:"winRate"`
	PickRate           float64  `json:"pickRate"`
	DeltaWinRate       float64  `json:"deltaWinRate"`
	HeroWinRate        float64  `json:"heroWinRate"`
	WilsonLowerWinRate float64  `json:"wilsonLowerWinRate"`
	Tier               int      `json:"tier"`
	Wins               int      `json:"wins"`
	Games              int      `json:"games"`
}

type hexdataPostmatchRow struct {     // 22 项，与上游一一对应
	KDA               float64 `json:"kda"`
	AvgKills          float64 `json:"avgKills"`
	AvgDeaths         float64 `json:"avgDeaths"`
	AvgAssists        float64 `json:"avgAssists"`
	AvgCs             float64 `json:"avgCs"`
	AvgGold           float64 `json:"avgGold"`
	AvgDamage         float64 `json:"avgDamage"`
	AvgDamageTaken    float64 `json:"avgDamageTaken"`
	AvgDmgMitigated   float64 `json:"avgDmgMitigated"`
	AvgHealShield     float64 `json:"avgHealShield"`
	AvgTurretDmg      float64 `json:"avgTurretDmg"`
	AvgCcTime         float64 `json:"avgCcTime"`
	DamageShare       float64 `json:"damageShare"`
	GoldEfficiency    float64 `json:"goldEfficiency"`
	KillParticipation float64 `json:"killParticipation"`
	MultiKillRate     float64 `json:"multiKillRate"`
	DoubleKills       int     `json:"doubleKills"`
	TripleKills       int     `json:"tripleKills"`
	QuadraKills       int     `json:"quadraKills"`
	PentaKills        int     `json:"pentaKills"`
	Survivability     float64 `json:"survivability"`
	UtilityScore      float64 `json:"utilityScore"`
}

type hexdataPostmatch struct {
	Citation  championSourceCitation
	FetchedAt time.Time
	Heroes    map[int]hexdataPostmatchRow   // key = 英雄 ID
}

type hexdataInsightEntry struct {      // topItems[] / topAugments[] 共用
	AssetID      int     `json:"-"`     // ID 的安全转换；非法时留 0，调用方不得当成真实资产
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Games        int     `json:"games"`
	WinRate      float64 `json:"winRate"`
	PickRate     float64 `json:"pickRate"`
	Confidence   string  `json:"confidence"`
	Rarity       string  `json:"rarity,omitempty"`
	Reason       string  `json:"reason,omitempty"`
	DetailURL    string  `json:"detailUrl,omitempty"`
	PairWinRate  float64 `json:"pairWinRate,omitempty"`
	DeltaWinRate float64 `json:"deltaWinRate,omitempty"`
}

type hexdataInsightHero struct {
	ChampionID  int                   `json:"-"`
	ID          string                `json:"id"`
	Name        string                `json:"name"`
	Tier        int                   `json:"tier"`
	Games       int                   `json:"games"`
	WinRate     float64               `json:"winRate"`
	PickRate    float64               `json:"pickRate"`
	DetailURL   string                `json:"detailUrl"`
	Confidence  string                `json:"confidence"`
	TopItems    []hexdataInsightEntry `json:"topItems"`
	TopAugments []hexdataInsightEntry `json:"topAugments"`
}

type hexdataInsightAugment struct {
	ID                string  `json:"id"`
	AugmentID         int     `json:"-"`
	Name              string  `json:"name"`
	Tier              int     `json:"tier"`
	Games             int     `json:"games"`
	Rarity            string  `json:"rarity"`
	WinRate           float64 `json:"winRate"`
	PickRate          float64 `json:"pickRate"`
	DetailURL         string  `json:"detailUrl"`
	Confidence        string  `json:"confidence"`
	AvgDeltaWinRate   float64 `json:"avgDeltaWinRate"`
	CoverageHeroCount int     `json:"coverageHeroCount"`
}

type hexdataHextechInsightsPayload struct {
	Heroes        []hexdataInsightHero    `json:"heroes"`
	Augments      []hexdataInsightAugment `json:"augments"`
	ReportPatch   string                  `json:"reportPatch"`
	ReportDate    string                  `json:"reportDate"`
	HeroCount     int                     `json:"heroCount"`
	AugmentCount  int                     `json:"augmentCount"`
	GeneratedAt   string                  `json:"generatedAt"`
	SchemaVersion string                  `json:"schemaVersion"`
}

type hexdataHextechInsights struct {   // 内嵌 payload，可直接 insights.Heroes
	hexdataHextechInsightsPayload
	Citation  championSourceCitation
	FetchedAt time.Time
}
```

### 3.1 对外函数签名（后续工单的调用入口）

```go
func parseHexdataMeta(data []byte) (hexdataMetaSnapshot, error)                                  // hexdata.go:1927
func hexdataJSONCitation(snapshot hexdataMetaSnapshot, canonical string) championSourceCitation   // hexdata.go:1952
func (p *championProvider) loadHexdataMeta(ctx) (hexdataMetaSnapshot, error)                      // hexdata.go:1963
func parseHexdataHeroJSON(data []byte) (hexdataHeroDetailV2, error)                               // hexdata.go:1989
func parseHexdataHeroJSONWithStats(data []byte) (hexdataHeroDetailV2, hexdataHeroJSONStats, error)// hexdata.go:1996
func hexdataTrimTrios(rows []hexdataTrioRow) ([]hexdataTrioRow, int)                              // hexdata.go:1909
func hexdataRarityLabel(value string) string                                                      // hexdata.go:2081
func hexdataAugmentMetricRows(rows []hexdataAugmentRowV2) []championMetricRow                      // hexdata.go:2097
func hexdataItemMetricRows(rows []hexdataItemRow) []championMetricRow                              // hexdata.go:2122
func (p *championProvider) decorateHexdataItemAssets(ctx, rows []championMetricRow)                // hexdata.go:2148
func parseHexdataPostmatch(data []byte) (map[int]hexdataPostmatchRow, int, error)                  // hexdata.go:2191
func (p *championProvider) loadHexdataPostmatch(ctx) (hexdataPostmatch, error)                     // hexdata.go:2210
func parseHexdataHextechInsights(data []byte) (hexdataHextechInsightsPayload, int, error)          // hexdata.go:2239
func (p *championProvider) loadHexdataHextechInsights(ctx) (hexdataHextechInsights, error)         // hexdata.go:2285
func (h *hexdataClient) adoptMeta(snapshot hexdataMetaSnapshot) bool                               // hexdata.go:1314
func (h *hexdataClient) pruneStaleBuildsAsync(buildID string)                                      // hexdata.go:1337
func (c *championDataCache) pruneStaleHexdataBuilds(currentBuildID string) error                   // champion_cache.go:483
func (c *championDataCache) pruneStaleHexdataBuildsCount(currentBuildID string) (int, error)       // champion_cache.go:489
```

单位约定（后续工单注意）：`hexdata*Row` 里的所有胜率/收益率都是**上游原样的 0..1 小数**；搬到 `championMetricRow` 时才乘 100（`WinRate`/`PickRate`/`WithoutItemWinRate` 是百分数，`DeltaWinRate`/`WilsonLowerWinRate` 保持 0..1 原样，因为它们是官方口径的差值/下界，R116-B 渲染时自己决定格式）。`hexdataHeroDetailV2.HeroWinRate` 已乘 100。

新增诊断事件：`hexdata_hero_json`（含 9 项 dropped/trimmed 计数）、`hexdata_item_asset`（rows/matched/missingIds）、`hexdata_item_asset_catalog_unavailable`、`hexdata_postmatch_dropped`、`hexdata_hextech_insights_dropped`、`hexdata_build_prune`（buildId/removed[/error]）。`hexdata_shape` 的 kind 新增 `meta`/`hero-json`/`postmatch`/`hextech-insights`；原来 kind=`hero` 的 shape 事件改为 `hero-json`。已消失的事件：`hexdata_item_match`、`hexdata_augment_copy`。

---

## 4. 删除的死代码清单

### 4.1 生产代码（`backend/hexdata.go`）

| 函数 / 符号 | 删除前的调用点 | 删除理由 |
|---|---|---|
| `decorateHexdataItems`（原 1619-1648） | 只被 `loadMayhemDetail`（原 1968）调用 | 它按**中文装备名**反查 Data Dragon 目录来补 ID，自己还埋了 `hexdata_item_match` 数漏配率。JSON 的 `items[].itemId` 就是真实资产 ID，名称匹配这条有损路径整段作废。替代品是 `decorateHexdataItemAssets`（2148），只做**精确 ID 查表** |
| `hydrateMayhemAugmentCopy`（原 1702-1738） | 只被 `decorateHexdataAugments`（原 1679）调用 | 它的前提（函数上方那段注释：「Riot 数据里任何地方都没有海克斯描述，Hexdata 页面是唯一来源」）已被实测推翻：`augments[].augmentDescription` 126/126 满覆盖 |
| `mayhemAugmentCopy`（原 1740-1805） | 只被 `hydrateMayhemAugmentCopy`（原 1722）调用 | 24h memo 的 per-augment 描述抓取调度器，随上游需求消失 |
| `fetchMayhemAugmentCopy`（原 1807-1825） | 只被 `mayhemAugmentCopy`（原 1782）调用 | 每条海克斯单独请求 `/augment/{id}-{slug}`，是「多付 97.6% 请求」的元凶 |
| `mayhemAugmentSlugs`（原 1829-1870） | 只被 `mayhemAugmentCopy`（原 1760）调用 | 唯一用途是给上面的扇出提供 ID→slug 表 |
| `rememberAugmentSlugFailure`（原 1872-1876） | 只被 `mayhemAugmentSlugs`（原 1842/1848/1859）调用 | `mayhemAugmentSlugs` 删除后成为孤儿（工单未点名，属同一条链） |
| `parseHexdataHeroDetail`（原 1295-1347） | 只被 `loadMayhemDetail`（原 1934）调用 | HTML 正则解析器，只有中文名、没有 item ID；已被 `parseHexdataHeroJSON` 取代 |
| `hexdataHeroDetail`（原 195-202） | `parseHexdataHeroDetail`、`reportHexdataHeroShape`、`loadMayhemDetail` | 三个使用方全部改写，结构体无生产者 |
| `hexdataTierPattern`（原 61） | 只被 `parseHexdataHeroDetail`（原 1306）调用 | 孤儿正则（页面「层级 T1」文本） |
| `mayhemAugmentIDFloor`、`mayhemAugmentCopyConcurrency`、`mayhemAugmentSlugFailureTTL`、`mayhemAugmentCopyTTL`（原 42/45/46/47） | 只被上述已删函数使用 | 孤儿常量 |
| `decorateHexdataAugments` 里的 `p.hydrateMayhemAugmentCopy(ctx, rows)`（原 1679） | — | 函数体本身**保留**，只摘掉这一句调用 |

净效果：单英雄详情页的 hexdata 请求从 `2+N`（实测样例 42）降到 2（meta + hero-json）。

### 4.2 测试代码（`backend/hexdata_test.go`，1288 → 995 行）

删除的 7 个测试全部只覆盖上面被删的函数，留着会编译不过：
`TestParseHexdataHeroDetailFiltersLowSampleRecommendations`、`TestParseHexdataHeroDetailAcceptsMetricSeparatorsAndExpandedShape`、`TestMayhemAugmentSlugsNegativeCachesListFailure`、`TestHydrateMayhemAugmentCopyFillsRecommendationRows`、`TestDecorateHexdataAugmentsHydratesMayhemCopy`（源码扫描断言 `decorateHexdataAugments` 必须调用 hydrate）、`TestHydrateMayhemAugmentCopyUsesBoundedParallelismAndCache`、`TestMayhemAugmentCopyCacheExpiresAfterTTL`（共 7 个，含并发上限那条）。
改写 1 个：`TestHexdataHeroShapeReportsActualRecommendationAndItemCounts` → 新签名 + kind `hero-json`。
低样本门槛（`hexdataMinimumSample`=250）的覆盖没有丢：过滤逻辑搬进 `hexdataAugmentMetricRows`，由 `TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly` 与真实夹具覆盖。

---

## 5. 明确保留的东西及原因

### 5.1 `decorateHexdataAugments`（`hexdata.go:2582`）
按 augment ID 从 CommunityDragon 目录补图标 / rarity / 名称，与新数据源无冲突，工单 P1-5 要求保留。**没有**改用 `augmentIconUrl`/`itemImageUrl`（Anti-scope 第 2 条），并有源码守卫测试钉住这两个字段名不得出现在 `hexdata.go` 里。

### 5.2 `hexdataHeroPathPattern` / `hexdataAugmentPathPattern` 留在白名单
工单 P1-7 的条件是「如果因为第 6 步的删除而**不再被任何地方引用**」。实测两个正则都仍被引用，条件不成立，因此保留：
- `hexdataHeroPathPattern`：`parseHexdataHeroes`（1690 行附近，榜单页解析 `/hero/{id}-{slug}` 链接）与 `parseHexdataAugmentDetail`（图鉴页解析适配英雄链接）仍在用；
- `hexdataAugmentPathPattern`：`loadHexdataAugmentDetail` 仍会抓 `/augment/{id}-{slug}` HTML 详情页（海克斯图鉴页的描述与 12 个适配英雄），另有 `hexdataTableShapeDiagnostic`、`parseHexdataAugmentsWithStats` 在用。
生产代码已经**不再请求** `/hero/{id}-{slug}`（有测试断言请求数为 0），白名单里留着这条只是不缩小既有权限，不构成新风险。`inspectHexdataPayload` 的 `case "hero"` 也一并保留：它服务的是既有 kind，删掉会让 `hexdata_test.go` 里四条缓存/重启回归测试失去入口。

### 5.3 `champions.go` 里变成死字段的 augment copy 缓存
`championProvider.augmentCopyMu / augmentCopy / augmentSlugs / augmentSlugsRetryAt`（113-116）与 `type augmentCopyCacheEntry`（128-131）在删除扇出链路后已无生产引用。本工单对 `champions.go` 的授权仅限「给 `championMetricRow`/`championRankingRow` 新增字段」，所以**没有**顺手删；Go 不会因为未使用的结构体字段/类型报错，`go vet` 也干净。建议在 R116-B（它本来就要改 `champions.go`）里一并清掉，另见 `champions.go:2303-2310` 那段「Riot 没有海克斯描述、只能从 Hexdata 页面取」的注释也已过时（`augmentOfflineDescription` 占位逻辑本身仍然有用，只有注释前提变了）。

---

## 6. 未解决问题 / 需要主 Agent 决策

### 6.1 冷启动第 3 条请求：`/api/hexdata/answer-cards`（工单判据与实际不一致）
工单 P1 判据 1 写「改动后应为 2」，但 `loadMayhemDetail` 仍然要调 `loadHexdataMeasurementTechnique` 才能填 `measurementTechnique`（前端 `champions.js:1643 renderMeasurementTechnique` 会渲染它），而 `/api/hexdata/meta` **不含** `methodology.measurementTechnique`（实测字段清单里没有）。所以：
- 真·全冷启动（本站任何缓存都没有）= **3** 条：`meta` + `hero-json` + `answer-cards`；
- `answer-cards` 命中 12h 缓存后（用户先开过榜单页，或第二次打开详情页）= **2** 条。
没有为了凑「2」而删掉这条请求：那会让统计口径说明消失，属数据准确性红线。两条测试分别覆盖了这两种情形，请主 Agent 决定是接受现状、还是把 `measurementTechnique` 的来源改到别处（需要上游先提供）。

### 6.2 前端测试 `backend/web/champions.test.cjs:4524` 失败（**需要决策，我没有改前端**）
```js
assert.match(goFunctionSource(hexdataBackend, "loadMayhemDetail"), /response\.Stats\.Tier = &tier/);
```
这条断言直接读 `backend/hexdata.go` 源码，钉住「海斗详情页必须给出英雄级梯度徽章」。而 `/api/hexdata/heroes/{id}` 的顶层只有 8 个数组，**没有英雄级 tier 字段**（`items[].tier`/`augments[].tier` 是行级档位，拿它冒充英雄档位就是伪造数据）。因此 `response.Stats.Tier` 现在明确降级为 `nil`，该断言失败：
```
✖ live recommendation capability flags hide unsupported matchup and ban-rate blocks
  AssertionError: The input did not match the regular expression /response\.Stats\.Tier = &tier/
```
- 用户可见影响：海斗详情页/对局推荐头的英雄梯度徽章暂时不显示（`renderChampionRecommendationHeader` 在 `tier` 缺失时本来就走无徽章布局，不会报错）。
- 官方口径的真实来源是 `hextech-insights.heroes[].tier`，本工单 P2 已经把它取回并按 buildID 缓存好了，接线正是 **R116-B P0-5-2**（「英雄榜单 `championRankingRow.Tier`/`Rank` 改为优先取 `hextech-insights.heroes[].tier`」）的范围。
- 三个可选处置：**(a)** 并入 R116-B（推荐：一次把详情页与榜单的官方 tier 都接上，同时改这条前端断言）；**(b)** 就地最小修复 = 改 `champions.test.cjs:4524` 的断言，但 `backend/web/**` 在本工单的禁改清单里，我没有动；**(c)** 在 `loadMayhemDetail` 里消费 `hextech-insights`，会让冷启动变成 3 条数据请求并多拉 373 KB，直接违反 P1 判据 1，不建议。
- 复跑结果：并行任务收敛后重跑 `node --test`，前端总数 **541 tests / 540 pass / 1 fail**，唯一失败项就是上面这条。执行中途一度出现的另外 4 条失败（`安装扫描失败与未安装状态分开显示并允许重试`、`R106 reopening a successfully loaded icon…`、`R111 admitted lazy thumbnails…`、`R87 suite fetch and stalled response bodies…`）来自并行任务对 `backend/web/app.js`、`backend/web/image-queue.js` 的在途改动，与本工单无关，现已自行消失。判据：全仓只有 `champions.test.cjs` 与 `position-filter.test.cjs` 会读 Go 源码，其余 `.cjs` 不可能被后端字段变更影响。

### 6.3 `meta.buildId` 与 `answer-cards.buildId` 必须同源（假设，未能实测）
两者都会写 `hexdataClient.state.BuildID`（`adoptMeta` / `adoptAnswer`），而 cache key 以它为前缀。生产上它们描述同一份数据构建、取值应当相同（本次抓到的 meta 是 `hexdata-2026-09-18-167d464cf859`），但**没有抓到 answer-cards 的真实响应**做逐字比对。若上游哪天让两者分叉，cache key 会在两个 build 之间来回跳，表现为 meta 每次访问都回源。缓解措施已有：`pruneStaleHexdataBuilds` 只认 `currentBuildID`，不会因此误删；真机验证时建议顺手看一眼 `hexdata_shape` 事件里 kind=`meta` 与 kind=`answer` 的 `buildId` 是否一致。

### 6.4 工作区在本工单执行期间被并行任务回滚过两次（重要）
- 18:48 与 19:08 两个时刻，`backend/hexdata.go` 与 `backend/hexdata_test.go` 被外部进程还原成 HEAD 内容（`git diff` 为空、mtime 同一秒、其余文件未受影响）。两次都从 Edit 工具快照 / `$PI_SCRATCH_DIR/r116a-backup/` 完整恢复，恢复后逐条重跑验证。
- 并行任务收敛后，**全部验证已在真实工作区重跑并通过**：`go build -o "$PI_SCRATCH_DIR/dl-build" ./backend`、`go vet ./backend`、`-run Hexdata`、`-run Mayhem`、`-race` 子集、全量 `go test ./backend/...`（ok 212.843s）、`cd backend/web && node --test`（540/541）。第 2 节记录的就是工作区实测结果。
- 备份位置：`$PI_SCRATCH_DIR/r116a-backup/`（6 个 Go 文件的最终版）。若再次发生回滚，直接 `cp` 回去即可；`docs/r116a-execution-ledger.md`、`desktop/package*.json` 未被回滚过。

### 6.5 磁盘预算仍未真正约束 hexdata 正文
P3 只解决「buildID 轮换后旧文件不回收」这一条增长曲线（评审 4.5 节的 6.64 GB/年）。当前 buildID 之内的 173 英雄 × 1.6 MB ≈ 277 MB 仍然豁免 `championCacheMaxBytes`，因为 `protected` 是故意设计（Anti-scope 第 4 条不许动）。要真正封顶需要 R116-E 的磁盘预算工单来定策略（例如给 hexdata 单独一个预算池），本工单没有越界处理。

---

## 7. 真机验证清单

| 项 | 状态 | 操作与期望 |
|---|---|---|
| 打开一个此前从未访问过的海斗英雄详情页，导出诊断日志，确认请求数为 2（hero-json + meta） | **待真机验证** | Windows + 国服客户端。步骤：① 关闭应用，删除（或改名）`%APPDATA%/Deep Legends/champion-data/` 整个目录，保证冷启动；② 启动后直接搜一个海斗英雄（如 疾风剑豪）打开详情页；③ 导出诊断日志，统计 `hexdata_shape` 事件。**期望**：出现 kind=`meta` 与 kind=`hero-json` 各一条、kind=`answer` 一条（见 6.1，若先开过榜单页则 answer 已在缓存里，总请求就是 2）；**不得**出现任何 kind=`augment` 的 `hexdata_shape`/`hexdata_fallback`，也不得出现 `hexdata_item_match`、`hexdata_augment_copy`（这两个事件已随死代码删除）；`hexdata_hero_json` 事件里 9 项 dropped/trimmed 计数应全为 0（trimmed 两项在有 1000 条 trios 时会是 950） |
| 连续打开 5 个不同英雄详情页，确认无熔断触发 | **待真机验证** | 紧接上一步，连续打开 5 个不同海斗英雄详情页并导出日志。**期望**：日志里**没有** `hexdata_circuit_open`、`hexdata_circuit_trip`、`hexdata_circuit_probe`；每个英雄一条 kind=`hero-json` 的 `hexdata_shape`，`meta` 只出现一次（12h 内命中缓存）。若出现 `hexdata_circuit_trip` 且 reason 含 `HTTP 403`，说明 Referer 没生效（`hexdata.go:1125`），属回归 |
| 手动触发一次假想的 buildID 变化，确认旧文件被清理 | **已自动化**（无需真机） | `TestAdoptingNewBuildIDPrunesStaleHexdataFilesAsynchronously` 用临时目录 + mock 走完整生产路径（`loadHexdataMeta` → `adoptMeta` → 异步 `pruneStaleHexdataBuildsCount`），断言：超宽限期的旧 build 文件被删、宽限期内的上一个 build 文件保留、`hexdata-state.json` 不动、`hexdata_build_prune` 诊断里 `removed` 计数正确。真机上如需复核：改小 `hexdataStaleBuildGrace` 或把某个 `hexdata-*.json` 信封里的 `fetchedAt` 手动改到 9 天前，再打开任意海斗详情页，观察该文件消失并出现 `hexdata_build_prune` 事件 |

---

## 8. 工单结束标志

- [x] P0~P3 全部**可自动化**的验证判据 PASS（逐条见第 2 节）
- [ ] 真机验证清单前两项 **待真机验证**（本环境无 Windows / 无 League 客户端，操作步骤与期望值见第 7 节，未伪造 PASS）
- [x] 独立核对：随机抽 3 处代码引用行号，确认与实际代码一致

抽查记录（用 Read 工具逐行读取原文核对，不是凭记忆）：

| # | 账本里的引用 | 实际读到的内容 | 结论 |
|---|---|---|---|
| 1 | `hexdata.go:1125` = Referer 头 | `	request.Header.Set("Referer", "https://"+hexdataHost+"/")` | 一致 |
| 2 | `hexdata.go:2694` = hero-json 抓取调用 | `		page, pageErr := p.hexdata.load(ctx, "hero-json", strconv.Itoa(id), requestPath, "application/json", false)` | 一致 |
| 3 | `champion_cache.go:483` = prune 入口签名 | `func (c *championDataCache) pruneStaleHexdataBuilds(currentBuildID string) error {` | 一致 |
