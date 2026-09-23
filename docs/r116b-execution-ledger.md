# R116-B 执行账本（第二轮 · 重做）

**执行时间：** 2026-09-21 10:40 ~ 12:00（本会话）
**版本：** `desktop/package.json` / `package-lock.json` 0.12.8 → **0.12.9**（`package-lock.json` 里三方依赖 `unzipper@0.12.7` 未动，已核对）
**工单：** `docs/history/worklists/R116-B-P0六项与详情页三tab重整-工单.md`
**执行契约：** `R116-B-REDO-CONTRACT.md`（八节）+ `R116-EXECUTION-BRIEF.md` + `R116-MEASURED-NOTES.md`

---

## 0. 为什么是「第二轮重做」——第一轮成果被销毁的完整记录

| 项 | 内容 |
|---|---|
| 第一轮执行 | 2026-09-20，历时 62 分钟，P0-1~P0-6 后端 + 前端 + 三-tab 重整全部落地 |
| 销毁时间点 | 2026-09-20 晚间 ~ 2026-09-21 10:22 之间，由**并行的 R117 会话整体回滚**造成 |
| 销毁确认 | 主控会话 2026-09-21 10:24~10:25 逐个 grep 复核：`champions.js` 从 2752 行退回 **2428 行**、11 处 `mayhem-detail-tab` 归零、`index.html` 的 `shared.js` 引入被删、`gameplay.js` 的 `measurementTechnique` 回到 **0 命中** |
| 损失面 | **前端全部**（`champions.js` 三-tab + P0 渲染、`index.html` 引入、`champions.css` 新增样式、`gameplay.js` 常驻页脚）；**后端全部存活**（`championMetricRow` 新字段、`applyHexdataSampleTiers`、`applyMayhemSummonerSpellPairs`、`hexdataOfficialHeroTiers`、`championPerformancePanel`、`backend/web/shared.js` 29 行） |
| 幸存的第一轮痕迹 | ① `backend/web/shared.js`（完整存活，本轮未重写）；② `gameplay.css:1870-1879` 的 `.recommendation-measurement` 样式与注释；③ `gameplay.js:5728` 的 P0-3-2 注释（条件本身未改）；④ `gameplay.test.cjs:122-142` 的 `sharedWindowStub()` 注入；⑤ `r113.test.cjs:95-98` 里为 `renderRecommendedAugments` 预留的三个桩（`state.mayhemStage` / `normalizeMayhemStage` / `mayhemAugmentStageRow` / `renderMayhemStageChips`）——**本轮沿用了这套命名**，让幸存测试重新变成有效约束 |
| 恢复来源 | 后端：直接消费（契约第一节）。前端：按工单 + 契约第二/七/八节**重新实现**，不是从旧副本还原（旧副本已不存在）；命名与桩接口对齐 `r113.test.cjs` 的幸存断言 |
| 并行会话状态 | R117 会话已于 10:22 前彻底终止并经用户确认（连续 8 分钟零文件改动）。本轮执行期间**未再发生任何外部回滚**（收尾 shasum 核对全部 OK，见第 9 节） |

---

## 1. 实际改动清单（含改动后新行号）

### 1.1 后端（本轮唯一需要动的后端项：P0-6 的 stages 下发）

| 文件 | 位置 | 改动 |
|---|---|---|
| `backend/champions.go` | **296** | `championMetricRow` 新增 `Stages []championMetricStageRow \`json:"stages,omitempty"\``，注释写明「上游缺阶段不补零行」 |
| `backend/champions.go` | **299-323** | 新增 `championMetricStageRow`（9 个字段），注释逐字写明单位约定与裁剪理由 |
| `backend/hexdata.go` | **292-311** | `applyHexdataSampleTiers` 扩为同时给 `rows[i].Stages[j]` 补 `SampleTier`（复用同一个 `snapshot.SamplePolicy.sampleTier`，阈值仍只来自 meta） |
| `backend/hexdata.go` | **2241** | `hexdataAugmentMetricRows` 填 `Stages: hexdataAugmentStageMetricRows(row.Stages)` |
| `backend/hexdata.go` | **2244-2287** | 新增 `hexdataAugmentStageMetricRows`：winRate/pickRate/stageBaselineWinRate ×100，deltaWinRate/wilsonLowerWinRate 保持上游 0..1；`stage <= 0` 整条丢弃；**不按 `hexdataMinimumSample` 过滤**（低样本阶段靠徽记披露，不是悄悄消失） |
| `backend/hexdata_r116b_test.go` | 新文件，275 行 | 4 条测试：正样例（逐字段核对 2095 掷骰狂人四阶段的单位/.games/hexLabel/sampleTier）、负样例（缺阶段 1 → 只有 3 条且不补零）、负样例（整条 stages 缺失 → JSON 里没有 `stages` 键）、序列化契约（前端要的 9 个键都在，上游的 `wins/tier/hexTier/hexScore/recommendationScore/hexTierColor` 已裁剪，装备行不带 stages） |
| `backend/static_assets_test.go` | **109-111** | 把 `shared.js` 加进嵌入式静态资源的压缩/存在性清单（它是 `index.html` 静态引入的文件，漏了 embed 就是线上 404） |

**未动的后端**（第一轮成果，本轮只消费）：`SampleTier` / `DeltaWinRate` / `WilsonLowerWinRate` / `HexTier` / `HexLabel` / `HexTierColor` / `OfficialTier` / `CoreDelta` / `AverageIndex` / `WithoutItemWinRate`（`champions.go:274-289`）、`applyLocalAugmentGrades` 官方优先（`champions.go:321-…`）、`championPerformancePanel`（`champions.go:489-530`）、`hexdataPerformanceFields` 22 项与 4 个 `Cumulative: true`（`hexdata.go:2409-2435`）、`mayhemPerformancePanel`（`hexdata.go:2440-2516`）、`applyMayhemSummonerSpellPairs` + `hexdata_spell_pairs_unavailable` 诊断（`hexdata.go:3184-3203`）、`hexdataOfficialHeroTiers` / `Stats.Tier` 为 `*int` 且 insights 不可用时留 nil（`hexdata.go:3056-3070`）。
**禁触清单全部未动**：`season_stats.go`、`features.go`、`gameplay.go`、`README.md`、`DESIGN.md`、`backend/web/r117*.test.cjs`、`readme_privacy_test.go`、`champion_cache.go`、`augment_contract_probe*.go`、`backend/web/app.css`。R117 的改动一条都没回滚。

### 1.2 前端

| 文件 | 位置 | 改动 |
|---|---|---|
| `backend/web/index.html` | **16-19** | 新增 `<script src="/shared.js" defer></script>`，位置在 `runtime.js`(15) 之后、`section-loader.js`(22) 与 `gameplay.js`(24) 之前（两条加载路径都能拿到 `window.deepLegendsShared`），附 3 行注释说明顺序约束 |
| `backend/web/champions.js` | 2428 → **2811 行** | 见下面逐项 |
| `backend/web/champions.css` | 914 → **967 行** | **917-967** 新增 R116-B 段（三-tab 工具条、页脚、较基准正负色、低样本徽记、Wilson/官方档位格、metric-count 5/6 栅格、阶段 chips、P1-6 四组卡片、本地估算小字）；**265** 删掉 `.mayhem-opening-grid > section:last-child` 的横跨规则（技能加点搬走后开局配置只剩 3 组，正好占满三列） |
| `backend/web/gameplay.js` | **5158-5165** | `renderRecommendationArea` 末尾加常驻页脚：`window.deepLegendsShared?.renderMeasurementTechnique(payload?.measurementTechnique)`，包在 `.recommendation-measurement` 里（样式第一轮已落在 `gameplay.css:1877-1879`），拼在 `${tabs.map(panel)}` **之后**、`</section>` 之前；取不到口径说明时整块不渲染。**5728-5733** 的 P0-3-2 注释是第一轮幸存的，条件 `capabilities.hasAugments ? "" :` 一字未改 |
| `backend/web/champions.test.cjs` | 13-31 / 796-800 / 841 / 844 / 873-884 / 1035-1046 / 2887-2889 / 4258-4268 | 新增 `sharedScript` + `sharedWindowStub()` + `mayhemMetricHelpers`；4 处聚焦测试补真实助手（不打桩）；P0-4 断言改成新契约；`renderMayhemItemRoutes` 的桩从 `sortedGradeRows` 换成 `mayhemRouteRows` 并加护栏断言；`renderRecommendationArea` 补 `window` 依赖 |
| `backend/web/r116b.test.cjs` | 新文件，**806 行 / 29 条测试** | 全应用 jsdom 冒烟（按 index.html 真实顺序注入 runtime → shared → champions，fetch 全量记账，夹具由 `backend/testdata/r116/hexdata-hero-157.json` 按后端单位换算逐字段搬成响应形状）+ 聚焦降级测试 + 后端契约断言 |

`champions.js` 逐项（新行号）：

- **状态与归一化**：`40` `mayhemDetailTab`（持久化，key `champion-mayhem-detail-tab`）、`43` `mayhemStage`（会话内状态，不持久化）、`139` `normalizeMayhemDetailTab`、`141` `MAYHEM_STAGE_OPTIONS`、`142` `normalizeMayhemStage`、`2507` 重置时 `mayhemStage = 0`
- **三-tab 骨架**：`1017` `renderMayhemDetailPane`（改为 `overview + staleNotice + .mayhem-detail-content > mayhemDetailWorkspace`）、`1062` `mayhemDetailWorkspace`、`1068` `mayhemDetailTabSpecs`（表现 tab 只在 `performance.metrics` 非空时出现）、`1074` `mayhemDetailActiveTab`、`1079` `mayhemDetailToolbar`（三 tab + 图鉴入口按钮）、`1091` `mayhemDetailPanelMarkup`（**只渲染当前 tab**）、`1099` `mayhemOverviewTabMarkup`、`1105` `mayhemBuildTabMarkup`、`1109` `mayhemDetailFooter`（页脚常驻，空则整块不渲染）
- **局部替换（不重挂载页脚 / 不发请求）**：`1117` `switchMayhemDetailTab`、`1125` `updateMayhemDetailTabs`、`1881` `switchMayhemStage`、`1888` `updateMayhemAugments`、`2575-2578` 两个点击分支
- **P0-5**：`1029`/`1033`/`1035` 详情页梯度徽章与「本地估算」小字、`1050` `mayhemHeroTier`（详情已到时只认 `stats.tier`，nil → 整块隐藏）、`1746` 榜单行 `tierLocallyCalculated === true` → 「本地估算」
- **P0-1 / P0-2 行内标签**：`1144` `renderMayhemItemRanking`（**去掉前端重排**，新增较基准 / 不出时 / 置信三格 + Wilson tooltip）、`1164` `mayhemDeltaLabel`、`1173` `mayhemDeltaTone`、`1179` `mayhemDeltaCell`、`1186` `mayhemWithoutItemCell`、`1195` `mayhemSampleTierLabel`、`1199` `mayhemSampleCell`、`1206` `mayhemWilsonLabel`、`1213` `mayhemWilsonTooltip`
- **P0-4**：`1218` `renderMayhemOpeningConfiguration`（`1222` 出门装/鞋子保持 `renderConfigOption(row, kind, false)`；`1233-1245` 新增 `spellStats` 逐格渲染器 + `spellGroup` 走 `renderConfigOption(row, "spell", null, spellStats)`）、`1246` `renderMayhemSkillPlan`（技能加点搬到构筑 tab）
- **P0-2 排序护栏**：`1252` `renderMayhemItemRoutes` 改调 `1262` `mayhemRouteRows`（保持后端直出顺序）；`1439` `sortedGradeRows`、`1429` `sortedArenaRows`、`1468` `sortedArenaItemRows` **函数体一字未改**
- **P0-6**：`1817` `renderRecommendedAugments`（阶段过滤 + chips）、`1848` `mayhemActiveStage`、`1854` `mayhemAugmentStageRow`、`1862` `mayhemAugmentStageItem`、`1869` `renderMayhemStageChips`、`1898` `renderMayhemRecommendedAugment`（阶段值替换 + 追加指标格）、`1933` `mayhemAugmentConfidenceMetrics`
- **P0-3**：`1954` `renderMeasurementTechnique` 改为**转发**到 `window.deepLegendsShared`（本地实现已删；保留同名函数是为了 4 处调用点、`champions.test.cjs:748` 与 `r117.test.cjs:68` 的选择器都不动）
- **P1-6**：`1964` `renderMayhemPerformancePanel`、`1978` `mayhemPerformanceGroups`、`1988` `mayhemPerformanceGroup`、`2006` `mayhemPerformanceValue`（按后端 `format` 的五个取值分支：`percent`/`int`/`count`/`decimal2`/`decimal3`，未知取值 → 该项不渲染）、`2021` `mayhemPerformanceDelta`（只看 `hasDelta`）、`2028` `mayhemPerformanceDeltaTone`

---

## 2. 逐条验证判据的真实状态

图例：**PASS**＝本环境已自动化验证；**待真机**＝需要 Windows 真机 / League 客户端 / 真 Chromium，本环境做不到，绝不写成 PASS；**未做**＝附原因。

### P0-1 收益率 `deltaWinRate`

| 判据 | 状态 | 证据 |
|---|---|---|
| 装备排行同一行同时显示 胜率 / 较基准 / 不出时胜率，三者互不相同（真实快照） | **PASS** | `r116b.test.cjs` 「P0-1 装备排行同一行给出胜率、较基准、不出时胜率三个互不相同的数字」：用 `backend/testdata/r116/hexdata-hero-157.json` 的 `items[0]`（毁坏仪式）断言 `61.63%` / `+4.9%` / `49.65%` 三段文本互不相同，且 `is-delta-up`、Wilson tooltip = `95% 置信区间下界 61.55%` |
| 海克斯卡也带「较基准」 | **PASS** | 「P0-1 海克斯卡带较基准与官方档位」：`deltaWinRate 0.111527 → +11.2%` |
| 收益率为 0 或字段缺失时不显示「较基准」（不显示 0.0%） | **PASS** | 「P0-1 收益率为 0 或字段缺失时不显示」：`0 / "0" / null / undefined / "" / NaN / "abc" / {}` 八种输入全部返回空串 |
| 后端 `DeltaWinRate` 字段 | **PASS**（第一轮已有） | `champions.go:274`；`r116b.test.cjs` J 段与 `hexdata_r116b_test.go` 都断言了单位（0..1 原值，前端 ×100） |

### P0-2 样本置信度 + 排序护栏

| 判据 | 状态 | 证据 |
|---|---|---|
| 「10 场 100%」带「样本极少」徽记且排不到「5000 场 55%」前面 | **PASS** | 「P0-2 低样本行带「样本极少」徽记，且排不到高样本行前面」：夹具里对照行是 `games:10 / winRate:100 / score:999 / sampleTier:"low"`，真实行是 `games:1406649 / winRate:61.63% / sampleTier:"high"`；断言 DOM 第一条是真实行、第二条带「样本极少」+ `is-low-confidence`，且低样本行**没有** Wilson tooltip |
| 对抗变异：把比较器改回 score 降序，该测试必须 FAIL | **PASS（已实跑）** | 见第 4 节的实际输出 |
| `sampleTier` / `wilsonLowerWinRate` 缺失时对应元素不渲染，不出现 `undefined` / `NaN%` | **PASS** | 「P0-2 sampleTier / wilsonLowerWinRate 缺失时对应元素不渲染」：`{}` / `sampleTier:""` / `"medium"` / `"high"`（无 wilson）/ `null` / `undefined` 全部返回空串；`sampleTier` 缺失时即使有 wilson 值也不渲染（分档是它的前提） |
| 低样本徽记复用既有 `is-low-confidence`，不新造一套 | **PASS** | 类名沿用；CSS 只加 `color: var(--warning)`（`champions.css:944-946`），没有复制既有的琥珀色字面值 |
| 高样本行 Wilson 下界作 tooltip、不单独占一行 | **PASS** | 装备排行走 `data-tooltip`（`mayhemWilsonTooltip`）；海克斯卡因卡片外壳不支持逐格 tooltip，改成同一 `dl` 里的一格「95%下界」（不是新增一行），并与「样本极少」互斥 |
| 后端 `SampleTier` 由 meta 阈值算好直出 | **PASS**（第一轮已有 + 本轮扩到阶段行） | `hexdata.go:298-311`；`hexdata_r116b_test.go` 断言阶段 1（59256 场）= high、阶段 3（373 场）= medium、阶段 4（250 场）= medium |

### P0-3 推荐页统计口径 + 关闭遗留确认项

| 判据 | 状态 | 证据 |
|---|---|---|
| 推荐页任意一次渲染，DOM 里出现「统计口径」且内容与后端 `measurementTechnique` 一致 | **PASS** | 「P0-3 海斗局 hasAugments=true 时写入按钮为空，经典局仍显示按钮」：编译真实 `renderRecommendationArea` + 真实 `shared.js`，断言输出含「统计口径」与「上游公布的统计口径」，且页脚位置在 `recommendation-panel-build` 之后；`payload.measurementTechnique` 为空时整块不渲染 |
| 抽成共享函数、两处调用、不复制粘贴 | **PASS** | 「P0-3 the measurement technique has exactly one implementation」：`champions.js` 里 `class="mayhem-measurement"` 命中 **0** 次、`shared.js` 里命中 **1** 次；`champions.js:1954` 只转发 |
| `gameplay.js:5684`（现 **5734**）行为不变：海斗局按钮为空、经典局仍显示 | **PASS** | 同上测试的后半段（经典局 `hasAugments=false` → `item-set-action` 出现）；「P0-3 推荐页页脚渲染后端 measurementTechnique，且 hasAugments 分支行为不变」另断言 `const action = capabilities.hasAugments ? "" :` 全文件只有 1 处、注释在位、条件未改 |
| 英雄资料页页脚常驻（跨 tab 不重挂载） | **PASS** | 「统计口径页脚跨 tab 常驻」：切 build → performance → overview 三次，断言 `[data-mayhem-detail-footer]` **节点身份不变**（`===` 原节点）且 `isConnected`、文本仍含「统计口径」 |

### P0-4 召唤师技能组合带胜率

| 判据 | 状态 | 证据 |
|---|---|---|
| 召唤师技能区块任意一行同时显示胜率与选用率 | **PASS** | 「P0-4 召唤师技能同时显示胜率与选用率」：真实夹具 `summonerSpellPairs[0]`（闪现+标记）→ `56.94%` 与 `90.78%` 同时出现；`dt` 序列是 `["选用率","胜率"]` |
| 出门装 / 鞋子行为不变（只显示选用率） | **PASS** | 同一测试断言两组的 `dt` 序列都是 `["选用率"]`；`champions.test.cjs:1041` 断言 `renderConfigOption(row, kind, false)` 仍在 |
| `summonerSpellPairs` 为空时回退 pickRate-only、不整块报错 | **PASS** | 「P0-4 上游回退到 pickRate-only 数据时只显示选用率」：编译真实 `renderMayhemOpeningConfiguration` + `renderConfigOption`，`winRate:0` 的行只出选用率、**不出「胜率 0.00%」**；两个速率都为 0 时统计块整块不渲染（图标与名称照常）。后端回退与 `hexdata_spell_pairs_unavailable` 诊断事件由第一轮的 `applyMayhemSummonerSpellPairs` 负责（`hexdata.go:3184-3203`），本轮未改 |
| 数据源切换在后端完成 | **PASS**（第一轮已有） | 本轮只改前端渲染，`renderConfigOption` 的三态开关函数体一字未改（有断言钉住） |

> ⚠️ **与契约的一处偏离（有意，已核实）**：契约第二节/第一节要求把 spells 那一处改成 `renderConfigOption(row, "spell", true)`。实测 `renderConfigOption` 的三态里 `true` 只渲染 `<span class="option-win-rate">胜率</span>`，**会把选用率整格丢掉**，直接违反工单 P0-4 的验证判据「同一行同时显示胜率与选用率」。因此改成传第四个参数 `statsRenderer`（`spellStats`，与既有 `renderDepthStats` 同一套用法），三态开关本身未改。顺带修掉一个真实缺陷：既有的 `renderOptionStats` 只用 `hasPick/hasWin` 决定「整块要不要渲染」，之后两格无条件输出，所以传 `null` 在上游回退（`winRate=0`）时会渲染出「胜率 0.00%」——那是显示不存在的数据。

### P0-5 官方档位替代本地自算

| 判据 | 状态 | 证据 |
|---|---|---|
| 展示官方档位用 `hexLabel`（中文），不用 `hexTier`（内部枚举名） | **PASS** | 「P0-5 官方档位展示中文 hexLabel」：卡片文本含「官方档位夯」，且整个详情面板不出现 `hang` / `top` / `insufficient`；后端下发的阶段行也只带 `hexLabel`（`hexdata_r116b_test.go` 断言 `"hexTier"` 不在 stage 负载里） |
| 有官方 `hexTier` 的行不再触发本地分位计算 | **PASS**（第一轮已有） | `champions.go:326-343` `applyLocalAugmentGrades` 官方优先；`hexdataRowHasOfficialGrade` 判定；回退行数由 `countLocallyGradedRows` 披露进口径文案 |
| `hextech-insights` 缺失时榜单退回本地计算且 `tierLocallyCalculated: true` 被渲染成「本地估算」小字 | **PASS**（源码级）+ **待真机**（熔断态实机复现） | `champions.js:1746` 榜单行、`champions.js:1029` 详情面板标题下都加了 `<small class="mayhem-tier-local">本地估算</small>`；`r116b.test.cjs` 断言该分支与 `mayhemHeroTier` 的取值逻辑。**实机复现需要让 insights 熔断**（本环境无法稳定制造上游熔断），故标待真机 |
| 详情页 `Stats.Tier` 为 nil 时整块隐藏梯度徽章 | **PASS** | 「P0-5 stats.tier 为 nil 时梯度徽章整块隐藏」：`stats:{winRate:…}`（无 tier）→ `.mayhem-overview-metrics` 不含「梯度」、`.tier-badge` 与 `.tier-badge-fallback` 都为 null、不出现 `T—`/`undefined`。后端红线由 `TestLoadMayhemDetailHeroTierStaysNilWithoutInsights` 钉住（本轮未改，全量测试通过） |
| `tierBands` 官方中文标签 | **未做（有意）** | 后端 `championRankingResponse.TierBands`（`champions.go:161`）已透传，但工单 P0-5 的验证判据只要求 `hexLabel` 与「本地估算」两项；详情响应里没有 `tierBands`，要在详情页用它就得再扩后端字段（超出「仅 P0-6 的 stages 字段」授权）。当前梯度展示为 `T{n}` + 官方徽章图，文案没有自造解释。留给后续工单 |

### P0-6 海克斯阶段筛选

| 判据 | 状态 | 证据 |
|---|---|---|
| 切 chip 后卡片数字变化且与 fixture 对应 stage 一致 | **PASS** | 「P0-6 切阶段只改数字不发请求」：汇总 = `67.90%`（`pairWinRate 0.679036`）→ 阶段 1 = `70.54%` + `+13.8%`（`0.705363` / `0.137788`）→ 阶段 3 = `61.66%` 且官方档位从「夯」变「顶级」（`0.616622`，373 场） |
| 切回「汇总」恢复原数字 | **PASS** | 同一测试：`assert.equal(firstCard().textContent, summary)` 全字符串相等 |
| 不产生新网络请求（mock 断言 fetch 次数不变） | **PASS** | 同一测试对 `fetch` 全量记账，三次点击（阶段 1 / 阶段 3 / 汇总）后 `calls.length` 不变 |
| 默认视图展示英雄级汇总、不分阶段 | **PASS** | `mayhemStage` 初值 0；`mayhemAugmentStageRow(item, 0) === null` → 用父行数值 |
| 阶段缺失时不渲染、不填 0 | **PASS** | 「P0-6 缺某个阶段的 augment 在该阶段视图下整条不渲染」：削掉第一条的阶段 1 后，阶段 1 视图只剩 2 张卡且不含该 augment，文本无 `0.00%`/`undefined`/`NaN`；全部行都缺阶段 2 时**阶段 2 的 chip 整块消失**（不给一个点了没反应的控件） |
| 后端把 `stages[1..4]` 下发 | **PASS** | `hexdata_r116b_test.go` 4 条（正/负/负/序列化）+ `r116b.test.cjs` J 段的前端侧契约断言 |
| 阶段行也算 `sampleTier` | **PASS** | `hexdata.go:303-305`；Go 测试断言 373 场 → medium、250 场 → medium |
| 局中自动定位当前阶段 | **未做（工单明确不做）** | 评审 3.1 判定无通道，工单 P0-6 只做手动 chips |

### P1-6 22 项表现指标面板

| 判据 | 状态 | 证据 |
|---|---|---|
| 面板显示指标，每项带「较平均」修饰 | **PASS（18/22 项）** | 「P1-6 表现面板按后端 format 渲染」：`KDA 3.41 / 较平均 +12.4%`、`击杀参与率 76.62% / 较平均 -3.2%`、`平均伤害 39,885`、`生存评分 0.938`（decimal3）、`双杀累计次数 4,904,195`（count 千分位） |
| 4 个累计项去掉 ±% 并标注「累计次数（受出场场次影响，不可跨英雄直接比较）」 | **PASS** | 同一测试：双杀/五杀两项 `doesNotMatch(/较平均/)` 且 `match(/累计次数（受出场场次影响，不可跨英雄直接比较）/)`，父元素带 `is-cumulative`；后端 `Cumulative: true` 恰好 4 处（`r116b.test.cjs` 断言） |
| 工单写「22 项每项都有较平均」，实际 18 项 | **已如实记录** | 见第 7 节「量纲偏差」。`R116-MEASURED-NOTES.md` 第 1 节给的二选一里，本轮选**「保留但去掉 ±% + 明确标注」**而不是整块不渲染：累计次数本身是真实数据，隐藏会让用户以为上游没有；标注 + 压暗配色（`champions.css:963-965`）既不误导也不丢信息 |
| 缺任意一组时该组整体隐藏（不显示 0 或 N/A） | **PASS** | 「P1-6 某组指标全缺时该组整体隐藏」：后端整组不下发 → 3 组；`groups` 里声明了但 metrics 全缺 → 同样不渲染空组；`panel` 为 `null/undefined/{}/{metrics:[]}/未知 format` 五种情况全部返回空串（→ 表现 tab 整个不出现） |
| `hasDelta=false` 不渲染「较平均」 | **PASS** | `团队伤害占比`（`hasDelta:false`）无「较平均」；`生存评分`（`hasDelta:true` 但 `deltaPercent:0`）也不显示「较平均 0.0%」 |
| 均值不在前端算 | **PASS** | 面板相关函数不含 `reduce(` / `/ metrics.length`；`champions.js` 不出现 `/api/hexdata/postmatch` |
| 四组卡片形态参考 `.arena-overview-secondary` | **PASS** | `.mayhem-performance-group dl` 用与 `.arena-overview-secondary` 同类的紧凑 `dt/dd` 指标列（`champions.css:955-963`） |

### 页面结构：三-tab 重整

| 判据 | 状态 | 证据 |
|---|---|---|
| 切三个 tab，每个 tab 只渲染对应内容，不重复渲染其他 tab 的 DOM 节点 | **PASS** | 「详情页三 tab 只渲染当前页内容」：任意时刻 `.mayhem-detail-panel` **只有 1 个**；概览态下 `.mayhem-item-ranking` / `.mayhem-performance` 为 null，构筑态下 `.mayhem-augment-ranking` / `.mayhem-opening-config` / `.mayhem-performance` 为 null，表现态下前两者为 null |
| 避免隐藏 tab 仍占布局高度（R33 那类问题） | **PASS** | 同一测试断言 `.mayhem-detail-content [hidden]` 数量为 **0**——非当前 tab 一个节点都不进 DOM，不存在「隐藏元素占位」 |
| 页脚统计口径跨 tab 保持渲染、不重挂载/闪烁 | **PASS** | 见 P0-3 那一行（节点身份 `===` 断言）。实现上切 tab 走 `updateMayhemDetailTabs()` 的**局部替换**（只换 `[data-mayhem-detail-panels]` 的 innerHTML + 页签的 `aria-selected`/`class`/`tabindex`），不调 `render()`，所以页脚节点根本不会被重建 |
| 图鉴 + 稀有度分布从详情面板搬出，做成顶部独立入口按钮，不新建路由 | **PASS** | `mayhemDetailToolbar`（`champions.js:1079`）里的 `.mayhem-atlas-entry` 按钮带 `data-mayhem-view="atlas"` + `aria-label="离开当前英雄，…"`，复用既有 `mayhemView` 状态机与既有点击分支（`champions.js:2579` 起），CSS 用虚线边框与三个 tab 视觉区分；断言 `renderMayhemDetailPane` / 两个 tab 函数都不含 `renderMayhemAtlas|renderMayhemRarityPanel`，且全文件没有 `location.hash/assign/pathname =`。**补充说明**：本轮开工前实测，图鉴与稀有度分布**已经**不在详情面板里（它们是 `mayhemView === "atlas"` 的平行视图，`champions.js:983`），工单「6 段线性堆叠含海克斯图鉴」的描述对应更早的版本；本轮补的是工单真正缺的那一半——详情面板顶部的独立入口按钮 |
| 桌面视口下详情面板首屏高度不超过一屏（真 Chromium 截图核对） | **待真机** | 见第 8 节。三-tab + 只渲染当前面板在结构上已经把 9~10 段压到「1 段头 + 1 个面板 + 1 个页脚」，但**实测像素高度必须真 Chromium** |
| 构筑 tab 的「成装三件套」（`terminalItemTrios`） | **未做（留作后续）** | 后端解析层已有 `hexdataHeroDetailV2.TerminalItemTrios`（`hexdata.go:316`，裁剪到 top-50），但 `championDetailResponse` **没有对应字段**，即数据没有下发。本轮后端授权范围是「仅 P0-6 的 stages 字段」，扩这个字段属超范围。契约第二节明写「若本轮未消费则留作后续并在账本里写明」——**即此条**。建议随 R116-C′ 一起做 |
| 概览 tab = 梯度/胜率/样本 + 推荐海克斯 + 开局配置 | **PASS** | 梯度/胜率/样本是 `.mayhem-overview-strip`（tab 之上常驻，切 tab 不消失）；概览面板 = 推荐海克斯 + 开局配置 |
| 构筑 tab = 装备排行 + 装备路线 + 技能加点 | **PASS** | `mayhemBuildTabMarkup`；技能加点从开局配置搬来（`renderMayhemSkillPlan`，`champions.js:1246`），`champions.css:265` 的横跨规则同步删除 |

### 通用验证要求

| 项 | 状态 |
|---|---|
| 前端 jsdom 冒烟测试全绿 | **PASS**：`cd backend/web && node --test` → **577 tests / 577 pass / 0 fail**（基线 548 + 本轮新增 29） |
| `go test ./backend/...` 全绿 | **PASS**：`ok lol-loot-assistant/backend 220.209s`（基线 218.103s） |
| `go build -o "$PI_SCRATCH_DIR/dl-build" ./backend` | **PASS** |
| `go vet ./backend` / `gofmt -l backend/*.go` | **PASS**：均无输出 |
| 真 Chromium 截图核对 tab 切换与页脚常驻 | **待真机**（见第 8 节） |

---

## 3. 全绿门槛复核（本会话实跑）

```
go build -o "$PI_SCRATCH_DIR/dl-build" ./backend   → OK
go vet ./backend                                   → 无输出
gofmt -l backend/*.go                              → 无输出
go test ./backend/... -count=1                     → ok  lol-loot-assistant/backend  220.209s
cd backend/web && node --test                      → tests 577 / pass 577 / fail 0
go test ./backend -run 'Stages|Stage' -count=1     → 本轮 4 条新测试全 PASS
```
R117 的三条样式棘轮测试全部保持绿（见第 6 节）。

---

## 4. P0-2 排序护栏：核对结论与对抗变异输出

### 核对结论（契约第八节的两处，都已修）

| 位置 | 第一轮（改动前）的问题 | 本轮处置 |
|---|---|---|
| `renderMayhemItemRanking`（原 `champions.js:1029`，现 **1151**） | `objectRows(rows).slice().sort((l,r) => (r.score||0)-(l.score||0) \|\| (r.games||0)-(l.games||0))`——按 `score` 降序再按 `games` 降序**重排**，完全覆盖后端直出的官方顺序；`score` 与样本量无关，所以「10 场 100%」只要 score 高就能插到「5000 场 55%」前面 | **删掉重排**，改成 `objectRows(rows).slice(0, 8)`，原样渲染后端顺序（＝`meta.recommendationPolicy.ranking = sample_tier_then_wilson_lower_bound_v1`，`rankingSignals: sample_tier → wilson_lower_bound → pick_rate → games`） |
| `renderMayhemItemRoutes`（原 `champions.js:1048` 调 `sortedGradeRows`，现 **1254** 调 `mayhemRouteRows`） | `sortedGradeRows` 按 grade → score → games 重排，同样忽略样本分档 | 海斗调用点换成新函数 `mayhemRouteRows`（`champions.js:1262`，`return objectRows(rows);`，**不排序**），与对局推荐页 `renderBuildRecommendation` 的 `inUpstreamOrder` 口径一致 |

### ⛔ 三处边界守住了（逐条核对，函数体一字未改）

| 边界 | 归属 | 核对结果 |
|---|---|---|
| `sortedGradeRows`（现 `champions.js:1439`） | 被 `renderArenaAugmentSection`（现 **1459**：`const sorted = sortedGradeRows(filtered);`）调用，属**斗魂** | **函数体未改**，只改了海斗的那个调用点；`r116b.test.cjs` 断言 `gradeRank(augmentGrade(left.tier \|\| left.grade, left.score, scores))` 仍在、`const sorted = sortedGradeRows(filtered);` 仍在 |
| `sortedArenaRows`（现 `champions.js:1429`） | 读 `state.arenaSort`，是**用户显式选择的排序控件**，不是隐式重排 | **未改**；断言 `const metric = state.arenaSort;` 仍在 |
| `sortedArenaItemRows`（现 `champions.js:1468`） | 注释明写 `// YOUR.GG's default equipment order is tier, then sample count (not score). Keep this separate from augment/mayhem scoring` | **未改**；断言该注释原文仍在 |

`renderRecommendedAugments`（海克斯推荐）本来就**不重排**（只按稀有度各取前 3、保持后端顺序），本轮保持原样并加了断言 `doesNotMatch(/\.sort\(/)` + `match(/return count < 3;/)`，防止以后被改坏。

### 对抗变异实跑输出

两个变异体都在 `$PI_SCRATCH_DIR` 里生成，通过 `R116B_CHAMPIONS_SOURCE` 环境变量喂给测试（`r116b.test.cjs:29` 支持覆盖源文件，沿用仓库既有的 `R115_LOADER_SOURCE` / `R94_RUNTIME_SOURCE` 惯例），原始文件未被修改：

```
=== 变异 1：把 renderMayhemItemRanking 的比较器改回 score 降序 → games 降序 ===
✖ failing tests:
✖ R116-B P0-2 低样本行带「样本极少」徽记，且排不到高样本行前面
  AssertionError: The input did not match the regular expression /毁坏仪式/. Input:
  '1低低样本对照装备胜率100.00%场次10置信样本极少'
✖ R116-B P0-2 海斗路径不再有任何前端重排（对抗变异必须打红这条）
  AssertionError: 装备排行不得重排后端直出顺序
→ pass 27 / fail 2   （10 场 100% 的对照行确实跳到了第 1 位，正是工单要禁止的）

=== 变异 2：把 renderMayhemItemRoutes 的 mayhemRouteRows 改回 sortedGradeRows ===
✖ failing tests:
✖ R116-B P0-2 海斗路径不再有任何前端重排（对抗变异必须打红这条）
  AssertionError: The input was expected to not match /sortedGradeRows|\.sort\(/
→ pass 28 / fail 1
```
完整输出存于 `$PI_SCRATCH_DIR/r116b2-mutation-output.txt`。

---

## 5. P0-6 stages 下发的体积取舍（实测数字）

用真实响应 `$PI_SCRATCH_DIR/hexdata/heroes_157.json`（英雄 157，1,232,266 B）逐条核算，126 条 augment 全部 `games ≥ 250`（即全部会进 `RecommendedAugments`），共 500 条 stage 行（121 条有阶段 1、126 条有阶段 2/3/4）：

| 方案 | stage 负载 | 相对父行（57,016 B）的增量 | 取舍 |
|---|---|---|---|
| 上游原样 14 字段 | 140,138 B | +140 KB（3.5×） | 否 |
| **本轮采用：裁剪到 9 字段** | **102,135 B**（204.3 B/stage） | **+102 KB（父行 2.8×，详情页响应约 1.8~2.0×）** | ✅ |
| 再砍 wilson + baseline（7 字段） | 67,780 B | +68 KB | 否：阶段视图会丢掉 P0-2 的 Wilson 与「较基准」的基准值 |
| 只给「实际会渲染的行」（3 个品质 × 各前 3，5 个视图取并集 = **10 行**） | **7,860 B** | +7.9 KB | 否，见下 |

**裁剪掉的 5 个上游字段**：`wins`、`tier`、`hexTier`、`hexScore`、`recommendationScore`、`hexTierColor`（`hexTier` 是内部枚举名 `"hang"`，下发只会诱导前端展示；`hexTierColor` 前端用 CSS 变量着色，不吃上游色值）。**保留的 9 个**：`stage` / `winRate` / `pickRate` / `deltaWinRate` / `wilsonLowerWinRate` / `stageBaselineWinRate` / `games` / `hexLabel` / `sampleTier`。`hexdata_r116b_test.go` 的序列化测试同时钉住「该有的都在、该裁的都不在」。

**为什么不选 7.9 KB 那个方案（虽然省 13 倍）**：它要求后端复刻前端的选取规则（先按阶段过滤、再按品质各取前 3），两边任何一方改了规则，另一方就会静默少数据；而前端拿不到 stages 的行在阶段视图里是**整条隐藏**，用户看到的是「这个阶段没有样本」，而上游其实有——属于自己制造的降级。102 KB 换来的是零耦合与「上游有的就一定下发」。这是**明确的取舍，不是没算过**：如果后续实测发现详情页响应体积成为问题（前端 `state.detailCache` 最多 64 个英雄 × 约 220 KB ≈ 14 MB 常驻），可以退到「只给推荐行附 stages」，但必须同时把前端的品质上限（3）写成后端可读的常量并加契约测试。

**响应体积影响的实测边界**：以上是**上游 JSON → 站内响应**的静态核算，不是端到端 HTTP 实测（本环境跑不起带 LCU 的完整服务）。详情页响应里 `RecommendedAugments` 段从约 57 KB 涨到约 159 KB；`ItemRanking`（118 行装备）不带 stages，体积不变（Go 测试断言装备行 `len(Stages) == 0`、JSON 里没有 `stages` 键）。

> **本节已按独立评审整改更新（2026-09-21，逐条见第 13 节）**：①字段集从「9 个含 `pickRate`」改成「9 个含 `grade`、不含 `pickRate`」（整改 B4 / B6），阶段负载在同一套静态核算下 102,008 B → **97,033 B**（砍 `pickRate` 省 10,915 B，加 `grade` 花 5,940 B）；②**本节原先漏算的对局内链路已记账**（整改 B5）：局内推荐 bundle 不再携带 stages，Go 测试实测海克斯段 **11,964 B → 4,403 B**（夹具 10 条 augment，−63.2%；真实英雄 126 条 augment 时省下的就是那 97 KB/英雄），详情页仍然完整下发。

---

## 6. 请求预算与 R117 样式棘轮

**本轮新增请求：0 条。**
- P0-6 的四个阶段随既有的 `/api/hexdata/heroes/{id}` 一次下发，切 chip 是纯本地重渲染（jsdom 测试对 `fetch` 全量记账，三次点击后计数不变）。
- 三-tab 切换、页脚常驻都是局部 DOM 替换，不触发任何 `api()`。
- 图鉴入口按钮复用既有 `data-mayhem-view` 分支，`/api/champions/augments` 与 `/api/champions/augment-rarity` 的请求条件一字未改（仍只在进入 atlas 视图、且缓存为空时各 1 条）。

**R116-A 已记账的预算变更（本轮未改，仅复核）**：详情页冷启动 2 → 4 条（meta / hero-json / postmatch / hextech-insights），由 `TestLoadMayhemDetailColdHeroCostsFourHexdataRequestsWithWarmSiteMetadata` 与 `TestLoadMayhemDetailRequestsHeroJSONAndMetaOnly`（暖站点元数据时 meta+hero-json 共 2 条数据请求 + 1 条 answer-cards）显式钉住，本轮全量跑通。**榜单冷启动 2 → 4** 这条：我在 `backend/*_test.go` 里**没有找到专门给海斗榜单记请求数的测试**（只有上面两条详情页的），所以这条只能标「R116-A 的行为，本轮未复核到测试级证据」，不谎称已记账。

**R117 样式棘轮全部保持绿**（`r117-style.test.cjs`，禁触文件，本轮未改）：

| 棘轮 | 阈值 | 改前 | 改后 |
|---|---|---|---|
| `border-radius` 取值种类 | ≤ 33 | 33 | **33** |
| `padding` 取值种类 | ≤ 205 | 205 | **205** |
| `gap` 取值种类 | ≤ 54 | 54 | **54** |
| 同一 hex 多处定义 | ≤ 35 | 35 | **35** |
| CSS 里定义但源码未引用的类名 | ≤ 93 | 93 | **92** |
| 重复声明组 | ≤ 45 | 45 | **45** |
| 23 个 token hex 各自只出现 1 次 | = 1 | 1 | **1** |

做法：新增 CSS **零 hex 字面值**（正收益率 `var(--success)`、负收益率与低样本 `var(--warning)`、官方档位 `var(--primary-strong)`），`padding`/`gap`/`border-radius` 全部复用文件里已有的取值字符串（`12px` / `8px` / `3px` / `0 10px` / `10px` / `1px` / `6px` / `9px`），每个新类名都在 `champions.js` 里真实使用。中途踩到一次坑并已修：注释里写了既有的那个琥珀色字面值，被 hex 计数扫到（36 > 35），改成文字描述后回到 35。

---

## 7. 真实数据核对（工单结束标志第 2 条）——已联网实抓 3 个英雄

命令（不带 Referer 返 403）：
```
curl -s -H "Referer: https://hexdata.com.cn/" -H "Accept: application/json" \
  -H "User-Agent: DeepLegends/probe" "https://hexdata.com.cn/api/hexdata/heroes/<id>"
```
2026-09-21 实抓英雄 **1 / 222 / 64**（1,064,806 B / 1,200,507 B / 1,201,817 B，全部 200），`meta` 同批抓取：`buildId=hexdata-2026-09-18-167d464cf859`、`reportPatch=16.18`、`reportDate=2026-09-18`、`heroCount=173`、`samplePolicy={low:1..249, medium:250..999, high:≥1000}`、`recommendationPolicy.ranking=sample_tier_then_wilson_lower_bound_v1`——与评审实测**完全一致**。原始响应存于 `$PI_SCRATCH_DIR/hexdata3/`，核算输出存于 `$PI_SCRATCH_DIR/r116b2-realdata-check.txt`。

| 字段非空比例 | 英雄 1（121 items / 108 augments / 28 spellPairs） | 英雄 222（122 / 107 / 28） | 英雄 64（118 / 120 / 27） | 与评审实测 |
|---|---|---|---|---|
| `items[].deltaWinRate ≠ 0` | 100.0% (121/121) | 100.0% (122/122) | 100.0% (118/118) | 一致 |
| `augments[].deltaWinRate ≠ 0` | 100.0% (108/108) | 100.0% (107/107) | 100.0% (120/120) | 一致 |
| `items[].hexTier` / `hexLabel` 非空 | 99.2% (120/121) | 99.2% (121/122) | 99.2% (117/118) | 一致（每英雄恰有 1 行为空 → 前端该行「官方档位」整格隐藏，降级路径真实可用） |
| `augments[].hexTier` / `hexLabel` 非空 | 100.0% | 100.0% | 100.0% | 一致 |
| `augments[].augmentDescription` 非空 | 100.0% | 100.0% | 100.0% | 一致（评审实测 126/126） |
| `wilsonLowerWinRate ≠ 0` | items 98.3% / augments 100% | items 99.2% / augments 100% | items 100% / augments 100% | 一致 |
| `withoutItemWinRate ≠ 0`（items） | 100.0% | 100.0% | 100.0% | 一致 |
| `summonerSpellPairs[].deltaWinRate ≠ 0` | 100.0% (28/28) | 100.0% | 100.0% (27/27) | 一致 |
| `summonerSpellPairs[].winRate ≠ 0` | 92.9% (26/28) | 96.4% (27/28) | 88.9% (24/27) | **新发现**：有 1~3 条胜率恰为 0 → 正是 P0-4 那个 `spellStats` 逐格判断要挡的情况（否则会显示「胜率 0.00%」） |
| `sampleTier`（按 meta 阈值算，items+augments 合计） | high 148 / medium 22 / **low 59** | high 168 / medium 21 / **low 40** | high 198 / medium 23 / **low 17** | 三档都真实存在 → 「样本极少」徽记不是死代码 |
| `stages` 覆盖（阶段 1 / 2 / 3 / 4） | 103 / 108 / 108 / 108（4 阶段齐全 95.4%） | 104 / 107 / 107 / 107（97.2%） | 113 / 120 / 120 / 120（94.2%） | **与评审实测一致**：阶段 1 有缺失（评审 121/126 = 96.0%），阶段 2/3/4 满覆盖 → 「缺阶段不渲染、不填 0」这条路径在真实数据里必然被走到 |
| `hexLabel` 取值分布（items+augments） | 拉完了 83 / NPC 72 / 顶级 31 / 样本过少 21 / 人上人 17 / 夯 4 / 空 1 | 拉完了 131 / NPC 59 / 人上人 15 / 顶级 13 / 样本过少 7 / 夯 3 / 空 1 | NPC 102 / 拉完了 68 / 顶级 31 / 人上人 31 / 夯 4 / 样本过少 1 / 空 1 | 六个枚举全部出现，含 `insufficient → 样本过少`；前端原样展示上游中文标签，不自己造文案（后端 `hexdataOfficialGrade` 对 `insufficient` 故意不给字母档位，两边口径一致） |

**量纲偏差（必须如实写）**：工单 P1-6 写「22 项指标，每项都有较平均修饰」，实际带 ±% 的是 **18 项**。原因是 `doubleKills`/`tripleKills`/`quadraKills`/`pentaKills` 是「该英雄全部对局的累计次数」，`pearson(总场次, doubleKills)=0.8117`，对它们做 ±% 量到的是人气不是强度。按 `R116-MEASURED-NOTES.md` 第 1 节给的二选一，本轮选**「保留渲染 + 去掉 ±% + 明写『累计次数（受出场场次影响，不可跨英雄直接比较）』」**，没有为了凑 22 项给计数字段编百分比。`multiKillRate` 原样展示数值 + ±%，不加任何解释性文案（其口径无法从公开字段反推）。

---

## 8. 真机验证清单：**待真机验证**（本环境无 Windows、无 League 客户端、无真 Chromium，一律不写 PASS）

| # | 操作步骤 | 期望值 | 状态 |
|---|---|---|---|
| 1 | 启动应用 → 英雄页 → 切到「海克斯大乱斗」→ 依次点开 **≥5 个不同英雄**（建议含 157 疾风剑豪、222 金克丝、64 李青、1 安妮、21 赏金猎人）→ 每个英雄依次点「概览 / 构筑 / 表现」三个 tab | ① 三个 tab 都能点，内容不串（概览只有推荐海克斯+开局配置；构筑只有装备排行+装备路线+技能加点；表现只有四组指标卡）；② 切 tab 时**页脚「统计口径」不闪、不消失**；③ 面板下方不出现空白占位区；④ 切英雄后若该英雄无 postmatch 数据，「表现」tab 整个消失（不是空页签） | **待真机验证** |
| 2 | 在任一英雄概览 tab 的「海克斯推荐」标题行，依次点阶段 chips「汇总 → 阶段 1 → 阶段 2 → 阶段 3 → 阶段 4 → 汇总」，同时开 DevTools Network 面板过滤 `/api/` | ① 卡片上的胜率/较基准/样本数字随阶段变化（例：157 的掷骰狂人 汇总 67.90% → 阶段 1 70.54%、较基准 +13.8% → 阶段 3 61.66% 且官方档位由「夯」变「顶级」）；② **Network 里 0 条新请求**；③ 切回「汇总」数字完全复原；④ 某阶段样本小的行显示「样本极少」 | **待真机验证**（jsdom 已覆盖 ①②③，见第 10 节） |
| 3 | 概览 tab → 开局配置 → 召唤师技能那一组 | 每行同时显示「选用率」与「胜率」两个数字（例：闪现+标记 选用率 90.78% / 胜率 56.94%）；出门装与鞋子两组**只有**选用率；若上游波动导致回退，召唤师技能自动退回只显示选用率，**不出现「胜率 0.00%」**。排障看诊断事件 `hexdata_spell_pairs_unavailable`（字段 `rawRows` / `opggFallbackRows`）与 `hexdata_spell_pairs`（字段 `rows`） | **待真机验证**（需要 League 客户端在英雄选择/对局中触发真实上游；jsdom 已覆盖渲染分支） |
| 4 | 进入一局对局 → 对局推荐页（任意 tab）→ 滚到推荐区最底部 | 所有 tab 面板**之后**出现常驻页脚「统计口径 …」，文案与后端 `measurementTechnique` 一致；切「海克斯与出装 / 详情 / 符文」页签时页脚始终可见；海斗/斗魂局里**没有**「游戏装备方案」写入按钮，经典局里**有** | **待真机验证**（jsdom 已覆盖 markup 与分支） |
| 5 | 真 Chromium 截图核对（工单「通用验证要求」）：桌面视口（建议 1440×900 与 1920×1080 各一张）打开海斗英雄详情，截三个 tab 各一张 | ① 详情面板首屏高度**不超过一屏**（不出现需要滚动才能看到页脚的情况）；② 统计口径页脚在三张截图里位置一致、始终可见；③ 三个 tab 的内容不重叠、不留大片空白；④ 阶段 chips 与图鉴入口按钮不换行错位 | **待真机验证**（本环境无法跑真 Chromium；结构上已把 9~10 段压成「头 + 1 个面板 + 页脚」） |
| 6 | insights 熔断态核对（P0-5-2 的降级） | 让 `hextech-insights` 不可用（断网/上游 5xx）后打开海斗英雄榜与详情页：榜单梯度列出现「本地估算」小字，详情页**梯度徽章与「梯度」指标整块消失**（不是 `T—`、不是行级 `items[].tier`） | **待真机验证**（本环境无法稳定制造上游熔断；源码级断言已 PASS） |

---

## 9. 防再毁：备份与 shasum 逐个核对

每写完一个文件立刻 `cp` 到 `$PI_SCRATCH_DIR/r116b2-backup/`。收尾核对（2026-09-21 11:52，live vs 备份）：

```
OK   f482f49590daf9d0bc0134bea28bfec7036b790d  backend/champions.go
OK   65da30d0db76987384a6a306b7e7f657c223437f  backend/hexdata.go
OK   3310533cb941940748645fc8bd39deed9fa989c4  backend/hexdata_r116b_test.go
OK   bb688e9498d17564f76a527abf6ec49ed3751ee0  backend/static_assets_test.go
OK   eb2f69e9929dbd2a1c92e28a0ffd5c6d77a093b8  backend/web/index.html
OK   39dfc25169fe89868847c3b7e47c0b3f58cb0f89  backend/web/champions.js
OK   90250761e3a653081b98e3e0169c15de737d7df7  backend/web/champions.css
OK   62871650a8bd4c2d730e4b121050bd76d61ff357  backend/web/gameplay.js
OK   b0c73f626aaad6dbabab2b2f773a7f0a6625fe77  backend/web/champions.test.cjs
OK   092ee09c01e6803058b4f56fa53d2d092a8fab79  backend/web/r116b.test.cjs
OK   57ecbe132b596849832be3592f8abb851e75958c  desktop/package.json
OK   dd0188aa7f47770f20cda957ccf15f7a712d7b30  desktop/package-lock.json
```
12/12 一致，**本轮执行期间没有发生任何外部还原**。（注：上面两个变异体是写到 `$PI_SCRATCH_DIR` 的副本，live 的 `champions.js` 未被改过，shasum 与备份一致即为证。）

---

## 10. jsdom 替代了什么、没替代什么

**替代掉（已自动化，29 条测试）**：
- tab 切换内容不串、每个 tab 只渲染自己的节点（`.mayhem-detail-panel` 恒为 1 个）；
- 隐藏 tab 不占布局高度（`.mayhem-detail-content [hidden]` 恒为 0，非当前 tab 零节点）；
- 页脚常驻且**节点身份不变**（切三次 tab 后 `===` 原节点，证明没有重挂载）；
- 阶段 chip 点击**不产生新请求**（对 `window.fetch` 全量记账，三次点击计数不变）+ 数字与真实夹具的对应 stage 一致 + 切回汇总全字符串相等；
- P0-1 的三个数字互不相同（真实快照）、P0-2 的低样本徽记与排序护栏、P0-4 的胜率/选用率组合与回退、P0-5 的 hexLabel/nil 隐藏/本地估算分支、P1-6 的五种 format 与累计项标注与整组隐藏；
- 全应用冒烟走的是**真实加载链路**：`index.html` → `runtime.js` → `shared.js` → `champions.js`，`fetch` 打桩，点击走真实的 `root.addEventListener("click")` 委托与真实的局部替换函数（不是直接调渲染函数）。

**没替代（必须真机，见第 8 节）**：真实像素高度与首屏是否超一屏、真 Chromium 的 CSS 容器查询/栅格实际换行、League 客户端在英雄选择/对局中触发的真实上游数据与回退路径、insights 熔断态、tab 与 chips 的键盘可达性/焦点环实际表现、Electron 里 `zoom: var(--ui-zoom)` 与新工具条的相互作用。

---

## 11. 留下的未解决问题与建议

1. **成装三件套（`terminalItemTrios`）未消费**：后端解析层已有（裁剪到 top-50），但 `championDetailResponse` 没有对应字段。需要一次「仅加一个响应字段」的后端授权，建议随 R116-C′ 一起做，落在构筑 tab。
2. **`tierBands` 官方中文标签未接前端**：`championRankingResponse.TierBands` 已透传（实测 `1=前15% … 5=后15%`），但详情响应里没有；要在详情页把 `T2` 显示成「15%-35%」需要扩后端字段（超出本轮授权）。当前没有自造文案。
3. **既有缺陷（本轮发现，未修，属超范围）**：`champions.js` 的 `renderOptionStats`（整改后 **`champions.js:2475-2480`**）只用 `hasPick/hasWin` 决定「整块是否渲染」，之后**两格无条件输出**。~~所以任何走默认三态（`showWinRate=null`）且缺胜率的行为会渲染出「胜率 0.00%」~~ ← **独立评审 B8 指出这句症状描述不准确，整改时已实跑重新核实（完整证据见第 13.8 节）**：`championMetricRow.WinRate` / `PickRate` 都带 `omitempty`，Go 的 0 不进 JSON → 前端拿到 `undefined` → `percent()` 走「—」分支，所以**真实症状是「胜率 —」/「选用率 —」这种带标签的破折号占位格**（占掉布局、暗示「这项有数据但取不到」），而**不是**「胜率 0.00%」；「0.00%」只在调用方递进来一个**显式 0**（JS 侧构造的行、测试夹具）时才出现，走 Go API 的行不可能出现。影响面（grep 核实）：所有走默认 `statsRenderer` 的调用点——`renderSpellsCard`（**`champions.js:2263`**）里的 `renderConfigOption(row, "spell")`（**`2319`**）、出门装（**`2321`**）、鞋子（**`2322`**）、斗魂棱彩装备（**`2323`**）；另外 `backend/web/gameplay.js` 有一份**同形的独立实现**（`renderOptionStats` @ **`gameplay.js:6058`**，被 `renderConfigOption` @ `6044` 调用），三格（选取率/胜率/场次）同样无条件输出，实测「只有 pickRate」的行会渲染成「选取率 41.2% 胜率 — 场次 —」两个破折号格；它还有一个**反向**问题：`hasPick/hasWin` 都为假时整块不渲染，所以「只有真实场次、没有两个速率」的行连场次都看不到（把真数据藏了）。本轮只在海斗召唤师技能这一处用 `spellStats` 绕开（P0-4 判据 2 要求），**没有去改这两个共用函数**（工单明写不要动 `renderConfigOption`，而 `renderOptionStats` 被多处共用，改它属超范围）。建议单开一条小工单把两处 `renderOptionStats` 都改成逐格判断，并补「winRate 缺失不渲染胜率格」「只有 games 时也要渲染场次格」两条测试。
4. **详情页响应体积**：stages 让 `RecommendedAugments` 段从约 57 KB 涨到约 159 KB（第 5 节）。如果后续要压，退路是「只给推荐行附 stages」，但必须同时把前端的品质上限写成后端可读的常量并加契约测试，否则会静默少数据。
5. **榜单冷启动 2→4 的请求预算缺测试级证据**：只有详情页那两条测试在记账（第 6 节）。建议 R116-A 的收尾里补一条海斗榜单的请求数断言。
6. **键盘可达性**：三个 tab 用了 `role="tablist"/tab/tabpanel` + `aria-selected` + `tabindex`，但**没有实现左右方向键切换**（既有的 `recommendationTabSpecs` / `arena-first-tabs` 也没实现，保持前后端两处 tab 交互一致）。若要补，应三处一起补，属独立工单。
7. **本轮踩到的一个真实 bug（已修，记录以免重犯）**：`normalizeMayhemDetailTab` 最初依赖模块级 `const MAYHEM_DETAIL_TABS`，而它在 `state` 初始化（第 40 行）时就被调用，`const` 还没到声明位置 → **TDZ ReferenceError，整个英雄页白屏**。已改成函数内字面量，并在源码里写了注释说明原因（`champions.js:135-139`）。这条只有全应用 jsdom 冒烟才抓得到，聚焦测试抓不到——是本轮坚持写全链路冒烟的直接收益。

---

## 12. 工单/契约里我认为有误或需要修正的陈述

1. **契约第二节 P0-4「改成 `renderConfigOption(row, "spell", true)`」是错的**：`true` 只渲染胜率、丢掉选用率，直接违反工单 P0-4 的验证判据「同一行同时显示胜率与选用率」。已改为传第四个参数 `statsRenderer`（`spellStats`），三态开关函数体未改。详见第 2 节 P0-4 的偏离说明。
2. **工单「现状证据」里「详情页 6 段线性堆叠含海克斯图鉴」与当前代码不符**：本轮开工前实测，`renderMayhemDetailPane` 只拼 5 段（推荐海克斯 → 开局配置 → 装备路线 → 装备排行 → 统计口径），图鉴与稀有度分布是 `mayhemView === "atlas"` 的**平行视图**，不在详情面板里。工单结构要求第 3 条真正缺的是「详情面板顶部的独立入口按钮」，本轮补的就是这个。
3. **工单 P0-2 现状证据里的行号（`champions.js:1955-1959`）已漂移**：`lowConfidence` / `is-low-confidence` / 「样本极少」在 `renderDepthStats` 里。~~本轮改动后约 `champions.js:2178-2182`~~ ← **独立评审 B7 指出：本条给出的替代行号本身也是错的**，按它跳转会落进 `renderRankedBuild`。整改时重新 grep 核实的真实位置（2026-09-21 整改后行号）：`renderDepthStats` 在 **`champions.js:2482`**，`lowConfidence` 判定与「样本极少」/`is-low-confidence` 输出在 **`champions.js:2494-2498`**；R116-B 自己那一份低样本徽记是 `mayhemSampleTierLabel`（**`champions.js:1218`**）+ `mayhemSampleCell`（**`champions.js:1222-1224`**）+ 海克斯卡的「置信」格（**`champions.js:2061`**）。契约第七节的 10:25 锚点表是对的，本轮一律以它为准；**行号一律以 grep 结果为准，本台账里的任何行号都只是当次快照**。
4. **工单 P1-6「22 项每项带较平均」在数据上不成立**：只有 18 项可比（第 7 节）。这不是实现缺陷，是工单写作时还没做量纲核算。

---

## 13. 独立评审整改（2026-09-21 13:30 ~ 15:20）

**整改单：** `R116-B-REVIEW-FINDINGS.md`（与执行方分离的 code-reviewer 子任务产出：A 3 条必修 / B 5 条建议 / C「已核实不要误伤」清单 / D 注意事项）
**追加范围：** R116-D 账本 §11.7 记下的两笔技术债（当初是为避开与 R116-F 的并行冲突而做的妥协，F 已收尾）
**处置结果：** A1 / A2 / A3 **全修**；B4 / B5 / B6 **修**；B7 / B8 按要求**重新核实并修正台账原有描述**（既存缺陷本身仍属超范围，没有顺手修）；D 的两笔技术债**已清**。整改单 C 节点名的禁改边界逐条复核，一条都没碰（见 13.10）。
**版本号：** 本轮**没有改** `desktop/package.json`（保持 0.12.11），按主控要求由 E 完成后统一收口。

### 13.0 改动清单（含改动后新行号）

| 文件 | 位置 | 改动 | 条目 |
|---|---|---|---|
| `backend/web/champions.js` | **579** | `primeMayhemDetail` 里加 `state.mayhemStage = 0;`（换英雄回到汇总） | A1 |
| `backend/web/champions.js` | **1885-1896** | `renderRecommendedAugments` 顶部加兜底钳制：记住的阶段号在当前 items 上一条阶段行都匹配不到 → 退回 `stage = 0` 渲染汇总并写回状态 | A1 |
| `backend/web/champions.js` | **1921-1927** | 空态文案分岔：阶段视图保留原文案，汇总视图（含斗魂）改成「暂无可展示的海克斯推荐样本。」——钳制之后「这个阶段…」只可能在汇总视图出现，那是错的口径 | A1 |
| `backend/web/champions.js` | **1179-1186** | `mayhemDeltaLabel` 的 0 判据挪到 `toFixed(1)` 之后 | A2 |
| `backend/web/champions.js` | **2177-2183** | `mayhemPerformanceDelta` 同形修正 | A2 |
| `backend/web/champions.js` | **1955-1962** | `mayhemAugmentStageItem` 改成按键覆盖：阶段维度拥有的 9 个键，阶段行没带就置 `undefined` | A3 |
| `backend/champions.go` | **337-347** | `championMetricStageRow` 去掉 `PickRate`、加上 `Grade`（注释写明取舍与实测体积） | B4 / B6 |
| `backend/hexdata.go` | **2265-2286** | `hexdataAugmentStageMetricRows` 填 `Grade: hexdataOfficialGrade(row.HexTier)`，不再填 `PickRate` | B4 / B6 |
| `backend/web/champions.js` | **1560-1567** | `renderArenaOptionCard` 新增可选 `options.hideGradeBadge`（不传时行为逐字不变） | B4 |
| `backend/web/champions.js` | **2010-2035** | `renderMayhemRecommendedAugment`：阶段视图徽章取阶段行 `grade`、没有就整块隐藏；「综合评分」在阶段视图标注「（英雄级）」 | B4 |
| `backend/gameplay.go` | **6283-6303** / **6325-6328** | 新增 `gameplayAugmentRowsWithoutStages`，局内 bundle 拷海克斯行时清掉 `Stages` | B5 |
| `backend/web/champions.js` | **1196-1205** | `mayhemDeltaCell` 加口径 tooltip（含 `tabindex`，与 `mayhemWilsonTooltip` 同一套写法） | B6 |
| `backend/web/champions.js` | **2043-2055** | `mayhemAugmentConfidenceMetrics` 新增可见的「阶段基准」一格（消费 `stageBaselineWinRate`） | B6 |
| `backend/web/champions.css` | **951-953** | 新增 `[data-metric-count="7"]` 的三列栅格（零 hex、零新 padding/gap/border-radius 取值） | B6 |
| `backend/web/gameplay.js` | **5404-5514** | `renderLiveInsights` 的 7 个渲染 helper 提到模块作用域 | D-债 1 |
| `backend/web/gameplay.js` | **5531-5532** | 调用点改成 `renderLiveRosterNoticeBars(payload, players)` / `renderLiveTeamPortraitTags(payload)` | D-债 1 |
| `backend/web/gameplay.js` | **4628-4634** | 删掉 `typeof liveRecommendationRoster === "function"` 护栏 | D-债 2 |
| `backend/gameplay_r116b_test.go` | 新文件 **88 行 / 1 条测试** | B5 的真实链路测试（用 `loadMayhemDetail` 跑真夹具） | B5 |
| `backend/hexdata_r116b_test.go` | **8-11 / 140-190 / 267-291** | 阶段行 `Grade` 的正样例断言（阶段 1 = S、阶段 3/4 = A、父行 = S）、`pickRate` 从序列化契约里移出并加反向断言 | B4 / B6 |
| `backend/web/r116b.test.cjs` | **29 / 57-83 / 165-186 / 363-392 / 816-846 / 848-891 / 894-1204** | 夹具阶段行改成新形状（去 `pickRate`、按 `omitempty` 语义加 `grade`）、harness 支持多英雄、A2 用例加强、后端契约断言更新（含 B5）、A3 断言加强、**新增 K/L 两节 8 条测试** | 全部 |
| `backend/web/champions.test.cjs` | **206-221 / 777-779** | `compileFunctions` 增加 7 个 live helper 的传递编译 + `liveRecommendationRoster` 默认注入；「综合评分」断言改成阶段视图带口径限定词的新形态 | D-债 / B4 |
| `backend/web/gameplay.test.cjs` | **134-137** | `compile` 的自动追加列表补上 7 个 live helper | D-债 |
| `backend/web/r116d.test.cjs` | **254-259 / 267-270 / 293-300 / 435-444** | 护栏测试改成显式注入 `liveRecommendationRoster: () => null`（行为断言一字未改）、`insightDeps` 编译列表补 7 个 helper、变异锚点缩进随提作用域更新 | D-债 |
| `docs/r116b-execution-ledger.md` | **§5 末尾 / §11.3 / §12.3** | §5 体积记账更新（B5/B6）、§11.3 症状描述按实跑重写（B8）、§12.3 替代行号修正（B7） | B5 / B7 / B8 |

### 13.1 A1 阶段筛选不随英雄切换重置（必修，已修）

**问题**：`state.mayhemStage` 的重置只发生在 `resetTransientChampionState`（换模式 / section 重置），换英雄走的 `selectMayhemChampion → primeMayhemDetail` 两者都不重置。最现实的触发路径不需要上游缺阶段：主源熔断时后端仍用 RSC 的 augments 兜底，那些行一律没有 `stages` → 过滤后 0 条 → 卡片区变空态，而 `renderMayhemStageChips` 因 `available.length <= 1` 整块不渲染 → **用户没有任何控件能点回「汇总」**，只能换模式或重载。违反评审 6.1，也与 `champions.js:41-43` 自己写的意图相反。

**修法（两处，与整改单一致）**：
1. `primeMayhemDetail`（**`champions.js:579`**）加 `state.mayhemStage = 0;`——从源头上不让阶段号跨英雄存活。
2. `renderRecommendedAugments`（**`champions.js:1893-1895`**）加兜底钳制：
   ```js
   const wanted = state.mode === "aram-mayhem" ? normalizeMayhemStage(state.mayhemStage) : 0;
   const stage = wanted && !items.some((item) => mayhemAugmentStageRow(item, wanted)) ? 0 : wanted;
   if (stage !== wanted) state.mayhemStage = stage;
   ```
   写回状态是必要的：`renderMayhemStageChips` 用 `state.mayhemStage` 决定 `is-active`/`aria-pressed`，只在局部变量里钳制会渲染出「显示汇总数据、却没有任何 chip 处于选中态」的工具条。钳制只用 `state` / `normalizeMayhemStage` / `mayhemAugmentStageRow` 三个既有自由变量，`r113.test.cjs:99` 那份固定注入列表因此不需要改（实测未改，测试全绿）。
3. 顺带修掉一个由钳制暴露出来的文案错误（**`champions.js:1921-1927`**）：钳制之后「这个阶段没有可展示的海克斯样本。」只可能在**汇总视图**出现（含斗魂路径，它根本没有阶段维度），对着一片空说「这个阶段」是错的口径，所以按 `stage` 分岔成两句。没有任何测试钉住过这句文案（grep 确认 0 命中）。

**测试**：
- `r116b.test.cjs:898`「A1 记住的阶段号在新英雄上没有阶段行时退回汇总，不锁死成空态」（聚焦）：RSC 兜底形状（两条行都没有 `stages`）+ `state.mayhemStage = 3` → 断言不出现空态文案、两条都渲染、`2 个`、`state.mayhemStage` 被写回 0；再断言钳制**不吃掉 P0-6 的正常路径**（有阶段 3 时照常过滤、部分行缺阶段 3 时缺的那条整条不渲染、斗魂路径不改海斗状态）。
- `r116b.test.cjs:943`「A1 换英雄后阶段筛选回到汇总（真实点击链路）」（jsdom，三英雄）：157（四阶段齐全）点「阶段 3」→ 换到 222（同样有阶段数据，男爵之手 汇总 61.41% / 阶段 3 80.00%）→ 断言 `is-active` 的 chip 是 `0`、卡片是 61.41% 而不是 80.00%、没有「阶段基准」格；在 222 上再点阶段 3 断言 80.00% + 阶段基准 57.05%（证明重置不等于砍掉 P0-6）→ 再换到 64（一条阶段行都没有）→ 断言 chips 整块消失、2 张卡照渲染、无空态文案、无残留数值。

**对抗变异（两条，实跑输出见 13.9）**：把钳制改回 `const stage = wanted;` → 「A1 记住的阶段号…」FAIL；把 `primeMayhemDetail` 里的 `state.mayhemStage = 0;` 删掉 → 「A1 换英雄后…」FAIL。
> ⚠️ 如实记录：第一次跑「删掉 primeMayhemDetail 重置」这个变异体时是 **pass 37 / fail 0（没被抓到）**——因为钳制兜住了空态，jsdom 测试当时只断言了「不出现空态」。这说明原测试只覆盖了必修的表象、没覆盖「换英雄必须回到汇总」这条意图，于是把 jsdom 测试改成三英雄版本（中间那个英雄**有**阶段数据，只有重置能救），补跑后该变异体 FAIL（pass 36 / fail 1）。

### 13.2 A2 `+0.0%` 舍入漏洞（必修，已修）

**问题**：`mayhemDeltaLabel` 只挡 `value === 0`，任何 `|deltaWinRate| < 0.00005` 都会被 `toFixed(1)` 抹成 `0.0`，渲染出工单 P0-1 判据明文禁止的「较基准 +0.0%」；`mayhemPerformanceDelta`（阈值 `|deltaPercent| < 0.05`）同形。触发面：单英雄 118 条装备行 + 499 条阶段行都走前者。

**修法**：判据挪到格式化之后（**`champions.js:1179-1186`** / **`2177-2183`**）：
```js
const text = Math.abs(points).toFixed(1);
if (text === "0.0") return "";
return `${points > 0 ? "+" : "-"}${text}%`;
```
不改成「按阈值砍」而是「按格式化结果砍」：阈值写法会在边界上与实际渲染不一致（`(0.05).toFixed(1)` 实测是 `"0.1"`，不是 `"0.0"`），而判格式化结果永远与用户看到的那个字符串同真同假。`mayhemDeltaTone` 保持原样——它只在 label 非空时被消费，不存在独立的失效路径。

**测试**：`r116b.test.cjs:363`（既有那条加强）补 `1e-5 / -1e-5 / 4e-5 / -4e-5 / ±0.00004999`（必须挡）与 `±0.0006 → ±0.1%`、`0.02 → +2.0%`（必须留）；`r116b.test.cjs:1147`（新）覆盖 `mayhemPerformanceDelta` 的 `0 / -0 / ±0.02 / ±0.049 / 1e-9` 与既有两条硬要求（只看 `hasDelta`、累计项永不带 ±%）。

**对抗变异**：两处各改回 `if (!Number.isFinite(value) || value === 0) return "";` + 原来的一行返回 → 分别打红「P0-1 收益率为 0 或字段缺失时不显示「较基准」标签」与「A2 表现面板的「较平均」同样挡掉被舍入成 0 的小值」。

### 13.3 A3 阶段行字段缺失时静默沿用父行（必修，已修）

**问题**：后端 `championMetricStageRow` 展示字段全带 `omitempty`，Go 零值不进 JSON → 「这个阶段没有这个值」在前端表现为「键不存在」；`{ ...item, ...staged }` 会静默保留父行（英雄级汇总）的值，却显示在「阶段 N」的 chip 下。最容易触发的是 `deltaWinRate`：阶段行缺键 → 卡片在阶段 3 下渲染出英雄级的「较基准 +11.2%」。

**选了哪种修法：前端（整改单给的第一种），后端不动。** 理由：
1. 后端方案要把 `deltaWinRate`/`hexLabel` 改成指针并去掉 `omitempty`，那会同时改掉「序列化契约」测试钉住的 JSON 形状，还要重新核对 R116-F 的阶段分布与 `applyHexdataSampleTiers` 对阶段行的写入路径——改动面比收益大。
2. 后端方案治不了同一类问题的其它来源：上游给 `0`、给 `null`、或将来新增一个阶段级字段时，前端仍然需要「键不在就不继承」这条规则。放在前端一处，覆盖全部字段与全部上游形态。
3. 前端方案与评审 6.1「取不到就整块隐藏」天然对齐：置 `undefined` 之后 `mayhemDeltaLabel` / `percent` / `mayhemSampleTierLabel` / `mayhemWilsonLabel` 各自已有的隐藏分支就生效了，不需要新写降级逻辑。
4. 同时保留「键存在但值是 0」与「键不存在」的区分（前者照抄 0，后者隐藏），不把两种语义压成一种。

**实现**（**`champions.js:1955-1962`**）：
```js
const merged = { ...item, ...staged };
const stageOwnedKeys = ["winRate", "pickRate", "deltaWinRate", "wilsonLowerWinRate", "stageBaselineWinRate", "games", "hexLabel", "grade", "sampleTier"];
for (const key of stageOwnedKeys) if (!Object.hasOwn(staged, key)) merged[key] = undefined;
return merged;
```
`score` / `rarity` / `assets` / `description` 这些**阶段维度根本没有**的字段继续沿用父行（「综合评分」的口径由 13.4 显式标注成英雄级）。键名列表写在函数体内而不是模块级 `const`：本函数被 `champions.test.cjs` 与 `r116b.test.cjs` 的 `compileFunctions` 单独编译（注入列表固定），模块级 `const` 会变成自由变量抛 `ReferenceError`（与 `normalizeMayhemDetailTab` 的 TDZ 教训同源，源码里写了注释）。`Object.hasOwn` 在仓库里已有先例（`gameplay.js:5360`），Electron 43 支持。

**测试**：`r116b.test.cjs:848`（既有那条加强）断言阶段行缺 6 个键时 `merged[key] === undefined` 且 `Object.hasOwn(merged, key) === true`（必须是「显式 undefined」，否则助手会读到父行值），并用一条**父行确实带着这些值**的夹具证明挡住的是继承而不是「本来就没有」，另断言「键存在但值是 0」时照抄 0；`r116b.test.cjs:1020`（新，jsdom）按整改单要求构造「阶段 3 的行缺 `deltaWinRate` 键」→ 断言阶段视图下**不渲染**「较基准」、不出现父行的 `+11.2%`、「阶段基准」格一起消失、无 `undefined/NaN`，且同视图里字段齐全的另外两条照常显示「较基准」（不是整块降级）。

**对抗变异**：改回 `return staged ? { ...item, ...staged } : item;` → 一次打红 3 条（「前端阶段助手按 stage 号取行…」「A3 阶段行缺 deltaWinRate…」「B4 阶段行没有官方档位时字母徽章整块隐藏」——后者依赖 `grade` 的同一套键覆盖语义）。

### 13.4 B4 阶段视图同一张卡混用两种口径（建议修，**已修**）

**问题**（评审实测复现）：徽章 `Grade` 与「综合评分」取父行值，胜率/样本/官方档位取阶段行值 → augment 2095 父行 `hang → S`、阶段 3 `top → A`（标签「顶级」），切到阶段 3 后同一张卡同时显示徽章 **S** 和「官方档位 **顶级**」；「综合评分 95.8」也是英雄级的（阶段级 `hexScore` 后端刻意裁掉了）。实测这不是边角案例：英雄 157 的 **499 条阶段行里有 115 条（23%）`hexTier` 与父行不同**。

**修法（组合，两半都做）**：
1. **徽章跟着阶段行走，映射只有一份实现**：后端 `championMetricStageRow` 加 `Grade`（**`champions.go:345`**），由 `hexdataAugmentStageMetricRows` 用**同一个** `hexdataOfficialGrade(row.HexTier)` 算好直出（**`hexdata.go:2282`**）。前端**不复制** `hexTier`/`hexLabel` → 字母的对照表（P0-3 立的「一份实现」纪律）。阶段行没有官方档位时（上游给 `insufficient`，映射返回空串 → `omitempty` → 键不存在，实测 4/499 条）字母徽章**整块隐藏**（`renderArenaOptionCard` 新增可选 `options.hideGradeBadge`，**`champions.js:1560-1567`**；不传时行为逐字不变，斗魂/棱彩/核心装备照旧）——拿英雄级的 S 去配阶段级的「样本过少」正是评审点名的口径混用，宁可隐藏。
2. **「综合评分」标注口径**：阶段视图里标签变成「综合评分（英雄级）」（**`champions.js:2035`**），因为后端不下发阶段级 `hexScore`（第 5 节的体积取舍），不能让一个英雄级数字冒充阶段数值。

**体积影响**：加 `grade` +5,940 B/英雄，与 B6 砍掉的 `pickRate`（−10,915 B）相抵后阶段负载 **102,008 B → 97,033 B**（同一套静态核算，见 §5 末尾的更新）。

**测试**：Go 侧 `hexdata_r116b_test.go:150-190` 断言阶段 1 `Grade="S"`（hang）、阶段 3/4 `Grade="A"`（top）、父行 `Grade="S"`——即评审复现的那个矛盾点在真实夹具上被钉住；`hexdata_r116b_test.go:267-291` 的序列化契约加 `"grade":"S"`。前端 `r116b.test.cjs:1045`（jsdom）断言汇总徽章 `S`+「官方档位夯」+「综合评分95.8」，切阶段 3 后徽章 `A`+「官方档位顶级」+「综合评分（英雄级）95.8」，切回汇总恢复 `S`；`r116b.test.cjs:1074`（聚焦）断言阶段行缺 `grade` 时卡片里**没有** `.augment-grade`、不出现 `>S<`，带着 `grade:"A"` 时徽章是 `is-A`，斗魂路径（stage 恒 0）不受影响。

**对抗变异（三条）**：①`grade: stage ? stageGrade : entry.grade` → `grade: entry.grade`（徽章跟着父行）→ 打红 2 条 B4 测试；②删掉 `hideGradeBadge` → 打红「阶段行没有官方档位时字母徽章整块隐藏」；③标签改回固定的 `"综合评分"` → 打红 B4 与 B6 各一条。

### 13.5 B5 对局内推荐页白背 stages（建议修，**已修**）

**问题**：`gameplayRecommendationsFromResolvedDetail` 把 `detail.RecommendedAugments` 原样拷进局内 bundle，而 `backend/web/gameplay.js` 全文对 `stages` **0 命中**（grep 复核仍为 0）。这是延迟敏感的局内 overlay 上未记账的成本。

**修法**：新增 `gameplayAugmentRowsWithoutStages`（**`gameplay.go:6291-6303`**），逐行值拷贝并把 `row.Stages = nil`，调用点在 **`gameplay.go:6327`**。逐行拷贝而不是共享底层数组：`range` 出来的 `row` 是副本，改它不会反向清空 `detail.RecommendedAugments`（那份切片还要给详情响应与缓存用）——这一点由测试显式断言。

**实测收益**（`go test -run TestGameplayRecommendationBundleDropsAugmentStages -v` 的 `t.Logf`）：
```
海克斯段 JSON：详情页 11964 B → 局内 4403 B（省 7561 B，63.2%，夹具 10 条 augment）
```
夹具只裁了 10 条 augment；真实英雄 126 条时省下的就是那 **97 KB/英雄**（13.4 的核算）。

**R116-D 的东西一条没动**（改完 grep 复核命中数）：`MatchupNotices` 7、`ItemsByIdentity` 14、`cloneLiveClientSnapshot` 4、`gameplayNextItemSuggestionFromTrios` 4、`gameplayApplyRosterInsights` 4、`parseLiveClientItems` 3；`r116d.test.cjs` 全部测试保持绿。

**测试**：`backend/gameplay_r116b_test.go`（新文件，用真实 `loadMayhemDetail` + 真夹具跑完整条转换）钉住 4 件事：bundle 每行 `len(Stages)==0`、序列化后 JSON 无 `"stages"` 键、除 `Stages` 外 9 个展示字段与 assets 逐字保留、`detail` 那份切片没被反向清空，外加体积必须变小。前端侧 `r116b.test.cjs:838-841` 加了源码级钉子：调用点必须走 `gameplayAugmentRowsWithoutStages`、helper 里必须有 `row.Stages = nil`、`gameplay.js` 对 `stages` 必须保持 0 命中（一旦局内开始消费 stages，这条剥离就要重新评估）。

**对抗变异**：把调用点改回 `append([]championMetricRow(nil), detail.RecommendedAugments...)` → `--- FAIL: TestGameplayRecommendationBundleDropsAugmentStages`，`gameplay_r116b_test.go:43: bundle.Augments[0] 仍然带着 4 条阶段行`。变异后已从安全副本还原，`shasum` 前后一致（`5176a81a…`）。

### 13.6 B6 `stageBaselineWinRate` 与阶段 `pickRate` 下发但零渲染（建议修，**已修：消费 baseline + 砍 pickRate**）

**整改单给的是二选一（加 tooltip / 砍字段），本轮选了「一进一出」的组合，理由是与 B5 的取舍保持同一条原则：只下发会被渲染的字段。**
- `stageBaselineWinRate` → **消费它**。「较基准」原来对用户是个没有定义的词，而这个字段正是它的基准。实测口径精确成立：`deltaWinRate = 阶段胜率 − stageBaselineWinRate`（英雄 157 的 **499/499** 条零误差）。因为海克斯卡外壳是 `overflow:hidden`、逐格 `data-tooltip` 会被裁掉（P0-2 的 Wilson 下界当初就是因此改成单独一格），所以这里用**可见的一格「阶段基准」**（**`champions.js:2055`**），用户能自己验算 `61.66% − 57.05% ≈ +4.6%`；只在「较基准」也渲染时才出现（依附关系），拿不到就整格不渲染。七格时卡片仍按三列排（**`champions.css:953`**，零 hex、零新 padding/gap/border-radius 取值，六个样式预算实测未变差，见 13.10）。
- 装备排行的「较基准」→ 加 **tooltip 定义**（**`champions.js:1202-1205`**，整改单点名的 `mayhemDeltaCell`）：`较基准 = 这件装备的胜率 − 该英雄整体胜率`。这句是实测核实的口径而不是推测：4 个英雄（157/1/222/64）共 **479/479** 条装备行满足 `deltaWinRate = winRate − heroWinRate`，海克斯父行同样 **126/126** 满足 `deltaWinRate = pairWinRate − heroWinRate`。tooltip 里**不放算出来的基准数字**，因为英雄级基准值没有逐行下发，前端拿 `winRate − deltaWinRate` 反推等于自己造一个「看起来权威」的数（数据准确性红线）。`tabindex="0"` + `data-tooltip-size="compact"` 沿用 `mayhemWilsonTooltip` 的既有写法。
- 阶段 `pickRate` → **从下发里去掉**（**`champions.go`** 结构体 / **`hexdata.go`** 转换）。前端零渲染（阶段视图的七格是 胜率/样本/综合评分（英雄级）/较基准/阶段基准/官方档位/置信），省 **10,915 B/英雄**。R116-F 的「阶段 × 稀有度」分布读的是上游解析结构 `hexdataAugmentRowV2.Stages[].PickRate`（`hexdata.go:2329/2356`），**不走这个下发结构**，所以砍它不影响 F（`r116f.test.cjs` 全绿复核）。

  > ⚠️ **与工单 P0-6「实现要求」第 1 条的一处冲突，如实记并请主控裁决**：工单原文是「点击后前端本地对已加载的 `stages[]` 重新渲染对应阶段的 `winRate`/`deltaWinRate`/**`pickRate`**」。选「砍掉」的三条依据：①B 的第一轮实现里海克斯卡**从来没有**选取率这一格（汇总视图也没有），这条从落地那天起就只满足了 3 个字段里的 2 个，字段下发了却零渲染——那正是评审 B6 点出的浪费；②工单的**验证判据**只要求「卡片数字变化且与 fixture 里对应 stage 的数值一致 + 不产生新请求」，没有选取率；③本轮整改单明确给了「把这两个字段从下发里去掉以省体积」这个选项，主控指令也要求「选一个并说明理由，与 B5 刚要为局内链路省体积的取舍保持一致」。如果主控认为工单那半句必须字面满足，回退成本很小：`champions.go` 的结构体加回 `PickRate`、`hexdata.go` 的转换加回 `PickRate: row.PickRate * 100`、卡片加一格「选取率」（**汇总视图也要加**，否则又变成一张卡两种口径），再把 `hexdata_r116b_test.go` 与 `r116b.test.cjs` 里那两处「不许有 pickRate」的反向断言改掉。

**残留缺口（如实写）**：**英雄级汇总视图**的海克斯卡上，「较基准」仍然只有一个定义在装备排行 tooltip 里的口径，卡片本身没有基准数值——因为英雄级基准（上游 `heroWinRate`）没有下发。要在汇总视图也给一格可见基准，正确做法是在详情响应里加**一个英雄级标量**（约 30 B，不是逐行 2.5 KB），这需要一次「仅加一个响应字段」的后端授权，超出本轮范围，建议随 §11.1（`terminalItemTrios`）一起提。

**测试**：`r116b.test.cjs:1121`（jsdom）断言装备排行「较基准」格的 `data-tooltip` / `tabindex` / `data-tooltip-size`，阶段 3 视图的六格顺序与取值（`胜率 61.66% / 较基准 +4.6% / 阶段基准 57.05% / 官方档位 顶级`、`data-metric-count="6"`），以及 CSS 里 `[data-metric-count="7"]` 的三列规则在位；`r116b.test.cjs:363` 断言 tooltip 与「拿不到值时整格不渲染（tooltip 也不许留下）」；`r116b.test.cjs:816-846` 断言后端阶段结构不再有 `pickRate`、转换器不再填 `PickRate`。

**对抗变异**：删掉「阶段基准」那一格 → 一次打红 3 条（B4 两条 + B6 一条）。

### 13.7 B7 台账 §12.3 的替代行号本身是错的（**已修正**）

按要求先自己 grep 再改台账。真实位置（整改后行号）：`renderDepthStats` 在 **`champions.js:2482`**，`lowConfidence` 判定与「样本极少」/`is-low-confidence` 输出在 **`champions.js:2494-2498`**；台账原先写的 `2178-2182` 落在 `renderRankedBuild` 里（评审的判断成立），评审自己给的 `2337-2341` 是 12:4x 的快照，R116-D/F 之后也漂了。顺带记清 R116-B 自己那一份低样本徽记的位置：`mayhemSampleTierLabel`（**`1218`**）、`mayhemSampleCell`（**`1222-1224`**）、海克斯卡的「置信」格（**`2061`**）。§12.3 原文已就地改正，并加了一句「本台账里的任何行号都只是当次快照，一律以 grep 为准」。

### 13.8 B8 台账 §11.3 对既存缺陷的症状描述不准确（**已重新核实并修正，缺陷本身未修**）

评审报告在这一条被截断，按要求自己重新核实。做法：把 `renderOptionStats` 单独编译出来，喂五种行形状实跑（`percent` 用真实实现）。**实跑输出**：
```
winRate 键缺失（Go omitempty 的常态）   →  选用率 41.20% 胜率 —
pickRate 键缺失                        →  选用率 — 胜率 56.94%
显式 winRate:0（JS 构造的行/夹具才可能） →  选用率 41.20% 胜率 0.00%
两个都缺                               →  （整块不渲染）
两个都有                               →  选用率 41.20% 胜率 56.94%
```
**结论（与评审一致，并补全影响面）**：
1. **真实症状是「胜率 —」/「选用率 —」这种带标签的破折号占位格**，不是台账原来写的「胜率 0.00%」。原因正如评审所说：`championMetricRow.WinRate`/`PickRate` 都带 `omitempty`（`champions.go:263-264`），Go 的 0 不进 JSON → 前端拿到 `undefined` → `percent()`（`champions.js:160`）走「—」分支。「0.00%」只在调用方递进来一个**显式 0**（JS 侧构造的行、测试夹具）时才出现，走 Go API 的行不可能出现——所以台账那句话把「只有测试夹具才走得到的分支」写成了主症状。
2. 危害比「0.00%」轻但仍然真实：一个带标签的空值格占掉布局，并暗示「这项有数据、只是取不到」，与评审 6.1 的「取不到就整块隐藏」相反。
3. **影响面（grep 核实）**：`renderOptionStats`（`champions.js:2475-2480`）是 `renderConfigOption`（`2439`）的默认 `statsRenderer`，走它的调用点是 `renderSpellsCard`（`2263`）里的召唤师技能（`2319`）、出门装（`2321`）、鞋子（`2322`）、斗魂棱彩装备（`2323`）。台账原来只写了「`champions.js:2164` 附近的 `renderConfigOption(row, "spell")`」，行号和范围都不准。
4. **新发现（评审报告被截断处之后）**：`backend/web/gameplay.js` 有一份**同形的独立实现**（`renderOptionStats` @ `6058`，被 `renderConfigOption` @ `6044` 调用），三格（选取率/胜率/场次）同样无条件输出。实测：`{pickRate:41.2}` → 「选取率 41.2% **胜率 — 场次 —**」（两个破折号格）；`{win:5694, games:10000}` → 正常推导出胜率 56.9%。它还有一个**反向**问题：`hasPick/hasWin` 都为假时整块不渲染，所以「只有真实场次、两个速率都缺」的行连**真实的场次**都看不到（把真数据藏了）。
5. **处置：不修**（与评审一致，属超范围；工单明写不要动 `renderConfigOption`，而这两个 `renderOptionStats` 被多处共用）。§11.3 原文已按上面 1~4 重写，并把建议改成「两处一起改成逐格判断 + 补两条测试（`winRate` 缺失不渲染胜率格；只有 `games` 时也要渲染场次格）」。

### 13.9 追加：R116-D 账本 §11.7 的两笔技术债（**已清**）

评估结论：**做**，风险可控。风险点只有一个——`compileFunctions` 的注入列表是固定的，函数体里多一个自由变量就抛 `ReferenceError`；F 已收尾，三个测试文件都可以改，于是按 B/F 开过的先例把新名字补进注入列表（**传递编译真实实现，不打桩**：桩会让断言与生产各说各话）。

1. **7 个渲染 helper 提到模块作用域**（`gameplay.js`）：`liveRosterChampionName`（**5420**）、`liveDeltaPoints`（**5430**）、`liveConfidenceText`（**5434**）、`liveEvidenceSuffix`（**5441**）、`liveNoticeBar`（**5455**）、`renderLiveRosterNoticeBars(payload, players)`（**5459**）、`renderLiveTeamPortraitTags(payload)`（**5497**）。闭包变量改成显式参数（`players` / `payload`），函数体逻辑与文案逐字未改；`renderLiveInsights`（**5516**）的调用点在 **5531-5532**。收益：每次渲染不再重新构造 7 个闭包，helper 可被单独编译单独断言（`r116b.test.cjs:1170` 就是这么测的）。
2. **`typeof` 护栏删掉**（**`gameplay.js:4634`**）：`const roster = liveRecommendationRoster(data);`。护栏防的不是生产情况而是「聚焦测试漏注入依赖」，留着只会掩盖真正的漏注入。
3. **同步更新的注入列表**：`champions.test.cjs:206-221`（`compileFunctions` 里按名字传递编译 7 个 helper，顺序＝先调用方后被调用方；并给 `liveRecommendationRoster` 一个「返回 null」的默认注入，与生产降级方向一致）、`gameplay.test.cjs:134-137`（同一套自动追加列表）、`r116d.test.cjs:293-300`（`insightDeps` 的编译列表补齐 7 个名字）、`r116d.test.cjs:264-270`（护栏测试改成显式注入 `() => null`，**行为断言一字未改**：仍然断言 `phase`/`allyChampionIds`/`enemyChampionIds` 三个参数都不发）、`r116d.test.cjs:435-444`（对抗变异锚点缩进 6→4 空格，因为 early return 跟着搬进了 `renderLiveRosterNoticeBars`）。
4. `renderLiveInsights` 本体除了那两行调用点**一字未改**（`gameplay.test.cjs` 里两处按字符串替换源码的测试依赖 `const selfTeam = …` 与 `recordLiveRosterRendered(…)` 两行逐字不变，已复核未动）。

**测试**：`r116b.test.cjs:1170`（新）断言 7 个 helper 都是模块作用域的函数声明、`renderLiveInsights` 体内不再有那 7 个 `const … =`、护栏字符串 `typeof liveRecommendationRoster` 全文 0 命中，并**单独编译** 5 个纯 helper 逐个断言取值（`liveDeltaPoints(0.0346)="3.5"`、置信区间文案、`evidence` 非 `supported` 时的降级、按 `championId`/`championPickIntent` 取名、拿不到名单返回空串、没有置信区间不给空 tooltip）。D 自己的行为测试（`r116d.test.cjs` 的克制/协同/画像断言）全部保持绿——它们编译到的是提作用域后的真实实现。

**对抗变异**：把护栏放回去 + 把 `liveDeltaPoints` 塞回 `renderLiveInsights` 体内 → 「D-债 renderLiveInsights 的 helper 已提到模块作用域，typeof 护栏已删」FAIL（pass 36 / fail 1）。变异后从安全副本还原，`shasum` 前后一致（`a946047e…`）。

### 13.10 对抗变异汇总（11 个变异体，全部被对应测试打红）

变异体一律生成在 `$PI_SCRATCH_DIR` 里，champions.js 的 9 个通过 `R116B_CHAMPIONS_SOURCE` 喂给测试（`r116b.test.cjs:20` 支持覆盖源文件），live 文件一个字都没改；gameplay.go / gameplay.js 的两个是「就地改 → 跑 → 从安全副本还原 → `shasum` 核对一致」。完整输出：`$PI_SCRATCH_DIR/r116b-fix-mutation-output.txt`。

| # | 变异体（把修复点改回原样） | 被打红的测试 | pass/fail |
|---|---|---|---|
| 1 | A1 钳制 → `const stage = wanted;` | A1 记住的阶段号…退回汇总 | 36/1 |
| 2 | A1 删掉 `primeMayhemDetail` 的 `state.mayhemStage = 0;` | A1 换英雄后阶段筛选回到汇总（真实点击链路） | 36/1 |
| 3 | A2 `mayhemDeltaLabel` 判据挪回格式化之前 | P0-1 收益率为 0 或字段缺失时不显示「较基准」标签 | 36/1 |
| 4 | A2 `mayhemPerformanceDelta` 同形改回 | A2 表现面板的「较平均」同样挡掉被舍入成 0 的小值 | 36/1 |
| 5 | A3 → `return { ...item, ...staged }` | 前端阶段助手按 stage 号取行… / A3 阶段行缺 deltaWinRate… / B4 阶段行没有官方档位… | 34/3 |
| 6 | B4 徽章 → `grade: entry.grade` | B4 徽章与官方档位同口径 / B4 字母徽章整块隐藏 | 35/2 |
| 7 | B4 删掉 `hideGradeBadge` | B4 阶段行没有官方档位时字母徽章整块隐藏 | 36/1 |
| 8 | B4 标签改回固定「综合评分」 | B4 徽章与官方档位同口径 / B6 阶段基准是一格可见数字 | 35/2 |
| 9 | B6 删掉「阶段基准」格 | B4 ×2 / B6 ×1 | 34/3 |
| 10 | B5 局内 bundle 恢复原样拷 `Augments` | `TestGameplayRecommendationBundleDropsAugmentStages`（Go） | FAIL |
| 11 | D-债 护栏放回 + helper 塞回函数体内 | D-债 helper 已提到模块作用域，typeof 护栏已删 | 36/1 |

实跑原文（节选，`node --test r116b.test.cjs`）：
```
=== 变异：A1-钳制 ===
✖ R116-B 评审整改 A1 记住的阶段号在新英雄上没有阶段行时退回汇总，不锁死成空态
ℹ pass 36 / fail 1
=== 变异：A1-换英雄重置 ===
（第一次跑：ℹ pass 37 / fail 0 —— 没被抓到，测试随即加强成三英雄版本）
（补跑：✖ R116-B 评审整改 A1 换英雄后阶段筛选回到汇总（真实点击链路）  ℹ pass 36 / fail 1）
=== 变异：A3-阶段行无条件展开 ===
✖ R116-B 前端阶段助手按 stage 号取行，缺失返回 null 而不是 0 值
✖ R116-B 评审整改 A3 阶段行缺 deltaWinRate 时不渲染「较基准」，也不拿英雄级的值顶
✖ R116-B 评审整改 B4 阶段行没有官方档位时字母徽章整块隐藏
ℹ pass 34 / fail 3
=== 变异：B5（Go，就地改后还原）===
--- FAIL: TestGameplayRecommendationBundleDropsAugmentStages (0.26s)
    gameplay_r116b_test.go:43: bundle.Augments[0] 仍然带着 4 条阶段行
restore: before=5176a81adbfe48e41ddf53ec9b6a650ff896c44b after=5176a81adbfe48e41ddf53ec9b6a650ff896c44b → RESTORE-OK
```

### 13.11 整改单 C 节「不要误伤」逐条复核

| 边界 | 复核结果 |
|---|---|
| `sortedGradeRows` 函数体禁改 | **未改**；`champions.js:1517` 仍是 `const sorted = sortedGradeRows(filtered);`，`r116b.test.cjs` 的 `gradeRank(augmentGrade(left.tier \|\| left.grade, left.score, scores))` 断言仍绿 |
| `sortedArenaRows`（读 `state.arenaSort`） | **未改**；`champions.js:1488` `const metric = state.arenaSort;` 在位，断言仍绿 |
| `sortedArenaItemRows`（YOUR.GG 注释） | **未改**；注释原文在 `champions.js:1524`，断言仍绿 |
| `renderMayhemItemRanking` 不排序 | **未改**（只改了它调用的 `mayhemDeltaCell` 内部）；`doesNotMatch(/\.sort\(/)` + `objectRows(rows).slice(0, 8)` 断言仍绿 |
| `mayhemRouteRows` 保持后端顺序 | **未改**；`champions.js:1286` `return objectRows(rows);` |
| `renderMeasurementTechnique` 只转发 shared.js | **未改**；`class="mayhem-measurement"` 在 champions.js 仍 0 命中、shared.js 1 命中 |
| R116-F 的 `renderMayhemHeroRarityPanel` / `mayhemCautionNote` | **未改**；`r116f.test.cjs` 全部测试绿 |
| R116-D 的 `renderLiveInsights` | 只做了明确要求的 helper 提作用域，本体逻辑与文案逐字未改；`r116d.test.cjs` 全绿 |
| `champions.css` 零硬编码 hex + 复用既有取值 | 新增只有 1 条 `[data-metric-count="7"]` 的 `grid-template-columns`（不含 hex/padding/gap/border-radius）。**六个预算实测未变差**：未引用类名 **90**（阈值 93）、border-radius **33/33**、padding **205/205**、gap **54/54**、重复 hex **35/35**、重复声明组 **45/45** |
| 单位换算全链路 / P1-6 量纲 / 降级红线其余分支 / P0-3,4,5 / shared.js 加载链路 / TDZ / 后端 stages 测试 | 全部未改；对应的既有测试（含 `hexdata_r116b_test.go` 4 条）全部保持绿 |
| 禁触文件 | `season_stats.go`、`features.go`、`README.md`、`DESIGN.md`、`backend/web/r117*.test.cjs`、`readme_privacy_test.go`、`champion_cache.go`、`augment_contract_probe*.go`、`backend/web/app.css`、`desktop/package.json`、`desktop/package-lock.json`、`backend/main.go` —— **一个都没动** |

### 13.12 全绿门槛复核（整改后实跑）

```
go build -o "$PI_SCRATCH_DIR/dl-build" ./backend   → BUILD-OK
go vet ./backend                                   → 无输出
gofmt -l backend/*.go                              → 无输出
go test ./backend/... -count=1                     → ok  lol-loot-assistant/backend  220.675s   （基线 219.872s；整改期间跑了两次全量，220.426s / 220.675s，两次全绿）
cd backend/web && node --test                      → tests 608 / pass 608 / fail 0              （基线 600/600，本轮 +8）
```
新增/加强的测试：**新增 8 条前端测试**（A1×2、A3×1、B4×2、B6×1、A2×1、D-债×1）+ **新增 1 条 Go 测试文件**（`backend/gameplay_r116b_test.go`，B5）；**加强 5 处既有测试**（`r116b.test.cjs` 的 P0-1 delta、阶段助手、后端 stages 契约；`hexdata_r116b_test.go` 的正样例与序列化契约；`champions.test.cjs:777` 的综合评分断言）。

### 13.13 防毁：备份与 shasum 逐个核对

每写完一个文件立刻 `cp` 到 `$PI_SCRATCH_DIR/r116b-fix-backup/`。收尾核对（2026-09-21 15:2x，live vs 备份，**12 个代码/测试文件逐个一致** → 整改期间没有发生任何外部还原）：

```
OK   34d19c48cf8da278dbc8bb63d87612ace146179d  backend/champions.go
OK   298b21237ec8c81e327839a0824e9b089b2e057e  backend/hexdata.go
OK   5176a81adbfe48e41ddf53ec9b6a650ff896c44b  backend/gameplay.go
OK   3be904b3bc47de48cc153340cee728523bc105f8  backend/hexdata_r116b_test.go
OK   b7196674da5bd15352561dfe7fbe243252cd7ada  backend/gameplay_r116b_test.go
OK   a9ced7ba5ee73370a5723516ee5a0e97545f55aa  backend/web/champions.js
OK   1bd3a7570281d8b10dde27ceb18d4657ad2fcbd3  backend/web/champions.css
OK   a946047efd004d173750e0a0ef11579ad0b5a29e  backend/web/gameplay.js
OK   61b8e782847e7214abe7e4d59c9017968be36e2e  backend/web/champions.test.cjs
OK   42be382ba553b9206f05f869bbbd63b64a4e86ab  backend/web/gameplay.test.cjs
OK   3873e1e59fae36f42e866f02b81beea8b8a2865f  backend/web/r116d.test.cjs
OK   74df37a5714b9b7b5cf14091fa6a8c52e23c42b0  backend/web/r116b.test.cjs
（台账自身 `docs/r116b-execution-ledger.md` 不参与哈希核对：它是自引用的，写入哈希这个动作本身就会改变哈希。它已同样 `cp` 进备份目录，供事后比对内容。）
```

两个「就地改 → 跑 → 还原」的对抗变异也各自核对过还原后的 `shasum`：`backend/gameplay.go` = `5176a81a…`、`backend/web/gameplay.js` = `a946047e…`，与上表一致，说明变异体没有残留在 live 文件里。champions.js 的 9 个变异体全部写在 `$PI_SCRATCH_DIR/champions.mutant-*.js`，live 文件从头到尾没被改过（`a9ced7ba…` 即为证）。

### 13.14 整改过程中新发现的问题 / 仍未解决

1. **英雄级汇总视图的「较基准」没有可见基准值**（13.6 的残留缺口）：正确修法是在详情响应里加一个英雄级标量（约 30 B），需要后端授权，建议与 §11.1 的 `terminalItemTrios` 一起提一条「仅加响应字段」的小工单。
2. **两处 `renderOptionStats` 的既存缺陷仍未修**（13.8）：症状是破折号占位格，`gameplay.js` 那份还有「只有真实场次时整块不渲染」的反向问题。属超范围，建议单开工单。
3. **阶段视图卡片最多 7 格**（原来最多 6 格），首屏高度需要真机复核——挂在 §8 第 5 条（真 Chromium 截图）里一起验，本轮无法执行。
4. **`mayhemDeltaTone` 仍保留 `value === 0` 的判据**（没有跟着改成「按格式化结果判」）：它只在 label 非空时被消费，不存在独立失效路径，为最小改动没有动它。若以后有人直接调用它渲染颜色，需要同步这条判据。
5. **`champions.js` 有一条既有的源码文本纪律**（`champions.test.cjs:1050`）：`renderMayhemOpeningConfiguration` / `renderMayhemItemRoutes` / `renderMayhemItemRanking` / `renderRecommendedAugments` / `renderMayhemAtlasDetail` 五个函数体内不许出现 `OP.GG|Hexdata|hexdata|your.gg|HexScore|globalHexScore`。整改时 A1 的注释里写了「hero-json 熔断时 hexdata.go 仍然用 OP.GG RSC 兜底」，被这条打红（两个测试），已改写成「主数据源熔断时后端仍然会拿 RSC 那份 augments 兜底」。**后来人在这五个函数里写注释要注意这条**，它扫的是注释而不只是渲染文案。
6. **`state` 在渲染函数里被写回**（`renderRecommendedAugments` 的钳制）是本轮引入的一处新模式。理由与边界：`mayhemStage` 是会话内视图状态、不持久化、写回只发生在「记住的阶段号在当前数据上无意义」时，且必须写回才能让 chips 的 `aria-pressed` 与实际视图一致。如果以后有人在这里加第二个写回，应该先把这类钳制抽成 `prepare*` 阶段而不是继续塞在渲染里。

---

## 14. 主控裁定：恢复阶段行的 `pickRate`（推翻评审整改 B6 的「出」那一半）

### 14.1 裁定
评审整改 B6 采取的是「一进一出」：进 = 消费 `stageBaselineWinRate`（阶段视图新增可见的「阶段基准」一格 + 装备排行「较基准」加口径 tooltip）；**出 = 把阶段行的 `pickRate` 从下发里裁掉**，理由是「前端零渲染的字段不下发」，省约 10.9 KB/英雄。

**主控判定：「出」这一半不成立，已恢复。** 依据是工单 P0-6「实现要求」第 1 条的字面要求：

> 点击后前端本地对已加载的 `stages[]` 重新渲染对应阶段的 `winRate`/`deltaWinRate`/**`pickRate`**，不重新发请求

工单点名了三个值。裁掉 `pickRate` 会让阶段 chip 只能重渲染其中两个，**工单「实现要求」第 1 条不成立**。本轮的用户指令是「不要遗漏任何一份工单里面的内容」，而 B6 只是独立评审的**建议项**（整改单里归在「建议修但不阻塞」），不是工单要求。二者冲突时**工单字面优先**。

B6 的「进」那一半（`stageBaselineWinRate` 可见一格 + tooltip）**保留**，它不与任何工单条目冲突，且解决了「较基准」是个没有定义的词这个真问题。

### 14.2 改了什么
| 文件 | 改动 |
|---|---|
| `backend/champions.go:340` | `championMetricStageRow` 加回 `PickRate float64 \`json:"pickRate,omitempty"\``；上方单位注释同步为「WinRate / PickRate / StageBaselineWinRate 是百分数」 |
| `backend/hexdata.go:2280` | `hexdataAugmentStageMetricRows` 填 `PickRate: row.PickRate * 100`；函数头注释第 3 条改写，写明恢复理由与体积代价 |
| `backend/web/champions.js:2031-2037` | 海克斯卡在**阶段视图**新增「选取率」一格：`...(stage && Number(item.pickRate) > 0 ? [["选取率", percent(item.pickRate), ""]] : [])`。**汇总视图不含这一格** —— 工单 P0-6 第 2 条只要求默认视图展示英雄级汇总的 `winRate`，给汇总视图也加一格属超范围视觉改动。样式类用空串（与既有「样本」格一致），**不新增 CSS 颜色规则** |
| `backend/web/champions.css:951-956` | 补 `[data-metric-count="8"]` 的三列栅格规则（恢复 pickRate 后阶段视图最多 8 格），并把 `="7"` 那条的注释改成能反映 8 格上限的版本 |
| `backend/hexdata_r116b_test.go` | 142-149 加 `stage1 pickRate = 2.498` 的值断言；276-303 把「阶段 payload 不含 pickRate」的反向断言改成**正向**断言，并从 `unwanted` 清单里移除 `"pickRate"` |
| `backend/web/r116b.test.cjs` | 夹具补 `pickRate: stage.pickRate * 100`（按 `omitempty` 语义，值为 0 时键不存在）；阶段切换测试补 `选取率2.50%`；B6 测试的 `deepEqual` 标签表加「选取率」、`byLabel["选取率"] === "0.02%"`、`data-metric-count` 6→7、并补 `="8"` 的 CSS 断言；结构体 tag 断言由 `doesNotMatch(/pickRate/)` 改为 `match(/\`json:"pickRate,omitempty"\`/)`；converter 断言补 `PickRate: row.PickRate * 100`；汇总视图补 `doesNotMatch(/选取率/)` |

### 14.3 单位与真实数值（都用真实夹具核过）
`pickRate` 与 `winRate`/`stageBaselineWinRate` 同属**百分数口径**：后端 ×100 直出，前端不再换算。英雄 157 的 augment 2095（掷骰狂人）四阶段实测：

| 阶段 | 上游 `pickRate` | 下发值 | `percent()` 渲染 |
|---|---|---|---|
| 1 | 0.02498 | 2.498 | 2.50% |
| 2 | 0.013186 | 1.3186 | 1.32% |
| 3 | 0.000163 | 0.0163 | 0.02% |
| 4 | 0.000128 | 0.0128 | 0.01% |

阶段 3/4 的选取率极小但**非零**，`percent()` 两位小数渲染成 0.02% / 0.01% 是真实值，不是 A2 那类「被舍入抹成 0」的假零——A2 的护栏针对的是 `deltaWinRate` 的「较基准 +0.0%」标签（那个 0.0% 没有信息量且工单明令禁止），选取率 0.02% 是有信息量的真实观测值，两者不是同一回事。

### 14.4 对抗变异（实跑，两侧都做）
- **前端**：把 `champions.js` 的「选取率」格整条删掉 → `node --test` **608 → 606 pass / 2 fail**，打红的是 `R116-B P0-6 切阶段只改数字不发请求，切回汇总恢复原值` 与 `R116-B 评审整改 B6 阶段基准是一格可见数字…`。还原后 608/608。
- **后端**：把 `hexdata.go` 的 `PickRate: row.PickRate * 100,` 删掉 → `go test -run "TestLoadMayhemDetailEmitsAugmentStagesInParentUnits|TestMayhemDetailResponseSerializesAugmentStagesForFrontend"` **两条都 FAIL**：
  - `hexdata_r116b_test.go:145: stage1 pickRate = 0, want 2.498`
  - `hexdata_r116b_test.go:302: stage payload lost pickRate; P0-6 requires the stage chip to re-render winRate/deltaWinRate/pickRate: […]`
  还原后 `ok`。
- 变异前都用 `cp` 存了安全副本（`$PI_SCRATCH_DIR/mut-champions.js.bak`、`mut-hexdata.go.bak`），还原后 `grep` 与 `gofmt -l` 复核一致、无 `MUTATION` 残留。

### 14.5 体积账（更新第 5 节）
- 详情页 `RecommendedAugments` 段：整改后 B6 曾把它压掉 10,915 B/英雄，本次恢复即 **+10.9 KB/英雄**（126 条 augment × 4 阶段的口径）。相对第 5 节记的 stage 总负载 102 KB，约 +10.7%。
- **局内推荐链路不受影响**：评审整改 B5 已在 `gameplay.go` 的 `gameplayAugmentRowsWithoutStages` 里把整个 `Stages` 清空（实测海克斯段 11,964 B → 4,403 B，−63.2%），所以这 10.9 KB 只落在英雄详情页，不落到延迟敏感的局内 overlay。
- 净账：B5 为局内省下的 97 KB（真实 126 条口径）远大于本次为详情页加回的 10.9 KB。

### 14.6 遗留
阶段视图的卡片现在最多 **8 格**（胜率 / 选取率 / 样本 / 综合评分（英雄级）/ 较基准 / 阶段基准 / 官方档位 / 置信或 95%下界）。栅格规则已补到 `="8"`，但**首屏高度是否仍不超过一屏需要真 Chromium 截图核对**，与第 8 节原有的截图待办合并为一条，本环境做不到。
