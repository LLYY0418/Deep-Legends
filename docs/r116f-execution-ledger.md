# R116-F 执行账本：海克斯概率预测器（英雄专属）+ 负向推荐 + 平衡参数探测

执行日期：2026-09-21 ｜ 版本：工单标注完成后升到 **0.12.12**，**本轮未改 `desktop/package.json`**（R116-D 并行执行，两边同时改版本号会冲突，由主控统一收口；执行期间仓库里的值是 **0.12.9**）
工单原文：`docs/history/worklists/R116-F-概率预测器负向推荐与平衡参数探测-工单.md`
P3 探测结论：`docs/r116f-balance-param-findings.md`
真实数据：`$PI_SCRATCH_DIR/hexdata/heroes_157.json`（英雄 157 完整响应，126 条 augments）；新增仓库夹具 `backend/testdata/r116/hexdata-hero-157-stage-rarity.json`

> 本账本记录「实际做了什么、验证结果如何、留下哪些未解决问题」，行号一律是**改动后**的新行号。

---

## 0. 工单结束标志逐条状态

| 工单结束标志 | 状态 | 证据 |
|---|---|---|
| P1/P2 验证判据 PASS | **PASS（全部自动化闭合，无一条待真机）** | §3、§4；`go test ./backend/... -run Mayhem` 24 PASS / 0 FAIL；`backend/web` 588 tests / 588 pass |
| P3 探测结论明确（找到/放弃二选一，不留「待定」） | **明确放弃** | `docs/r116f-balance-param-findings.md` §1（三条实测理由）+ §3（穷举已排除清单）+ §5（另两条候选路径的处置）+ §6（重开门槛） |

通用验证要求（工单唯一一条）：`go test ./backend/... -run Mayhem` → **ok，24 PASS / 0 FAIL**（§6）。

---

## 1. 实际改动清单

### 1.1 `backend/hexdata.go`（3677 → **3924** 行）

| 位置（新行号） | 内容 |
|---|---|
| 2285-2293 | 分节注释「R116-F P1：该英雄专属的『阶段 × 稀有度』概率分布」 |
| **2295-2302** | 新增 `type hexdataHeroStageRarityRow struct`：`Stage` / `Silver` / `Gold` / `Prismatic` / `Augments` / `Total`。单位与全服口径的 `hexdataRarityStage`（`hexdata.go:691`）**逐字一致**（百分数），前端两处都能直接 `percent()` |
| **2322-2344** | 新增 `func computeHeroStageRarityDistribution(augments []hexdataAugmentRowV2) map[int]map[string]float64`——**签名与工单 P1 实现要求第 1 条逐字一致** |
| **2346-2385** | 新增 `func hexdataHeroStageRarityRows(augments []hexdataAugmentRowV2) []hexdataHeroStageRarityRow`：换算成百分数、阶段号升序、带每阶段条数与三组之和；三组全 0 的阶段整块不下发 |
| **2395-2410** | 新增 `func (p *championProvider) reportMayhemStageRarity(championID int, rows []hexdataHeroStageRarityRow)`：诊断事件 `mayhem_stage_rarity`，带 `deviatingStages`（三组之和偏离 1.0 超过 ±0.01 的阶段数，正常恒为 0） |
| 2410-2416 | 分节注释「R116-F P2：负向推荐（慎选）判据」 |
| **2417-2431** | 新增 `func mayhemPickRateMedianByRarity(rows []championMetricRow) map[string]float64`：**先按 `Rarity` 分组**再取组内中位数；`Rarity` 为空的行不进池子 |
| **2435-2446** | 新增 `func mayhemMedian(values []float64) float64`：标准定义（奇数取中间、偶数取中间两数平均），不修改入参 |
| **2463-2483** | 新增 `func markMayhemNegativeRecommendations(rows []championMetricRow) int`：三条判据，幂等，返回命中条数 |
| **2488-2508** | 新增 `func (p *championProvider) reportMayhemCautionFlags(championID int, rows []championMetricRow, flagged int, samplePolicyUsable bool)`：诊断事件 `mayhem_caution`，带 `rarityPools` / `lowSampleRows` / `negativeDeltaRows`，排障时看得出是哪一条判据把行过滤光了 |
| **3348-3354** | `loadMayhemDetail`：`response.HeroStageRarity = hexdataHeroStageRarityRows(primary.Augments)` + `p.reportMayhemStageRarity(...)`。**用 `primary.Augments` 全量（126 条）而不是 `response.RecommendedAugments`**：后者按 `hexdataMinimumSample` 过滤过、还可能被 RSC 合并改写，缺行会让三组之和小于 100% |
| **3387-3401** | `loadMayhemDetail`：`samplePolicyUsable := snapshot.SamplePolicy.usable()` → 可用时才 `markMayhemNegativeRecommendations(response.RecommendedAugments)`；随后 `reportMayhemCautionFlags`；再把 P1 的口径说明 `appendMeasurementTechnique` 进常驻页脚 |

**放在 3387 而不是更早的理由**：判据①的中位数池子必须与用户实际看到的那一份行同源（hexdata 行 + RSC 合并行 + 档位回填之后），所以它在 `mergeMayhemAugmentRows`（3361）与 `applyLocalAugmentGrades`（3378）之后。

### 1.2 `backend/champions.go`（3018 → **3030** 行，**仅新增透传字段**）

| 位置（新行号） | 内容 |
|---|---|
| **297-303** | `championMetricRow` 新增 `Caution bool \`json:"caution,omitempty"\`` 与 `CautionMedianPickRate float64 \`json:"cautionMedianPickRate,omitempty"\``（百分数，与 `PickRate` 同单位） |
| **532-536** | `championDetailResponse` 新增 `HeroStageRarity []hexdataHeroStageRarityRow \`json:"heroStageRarity,omitempty"\`` |

**没有动 R116-B 刚加的东西**：`Stages`（296）、`SampleTier`（289）、`championMetricStageRow`（320-330）、`championPerformancePanel`（562-570）全部原样，一个字段都没改。

### 1.3 `backend/web/champions.js`（2811 → **2885** 行）

| 位置（新行号） | 内容 |
|---|---|
| **1097-1103** | `mayhemOverviewTabMarkup`：概览 tab 变成「推荐海克斯 → **该英雄专属品质概率** → 开局配置」。**没有动三-tab 骨架**（`renderMayhemDetailPane` 1017 / `mayhemDetailWorkspace` 1062 / `mayhemDetailTabSpecs` 1068 / `mayhemDetailToolbar` 1079 / `mayhemDetailPanelMarkup` 1091 / `mayhemDetailFooter` 1111 / `switchMayhemDetailTab` 1119 / `updateMayhemDetailTabs` 1127 全部原样），新内容随 `mayhemDetailPanelMarkup` 走同一条局部替换路径，页脚节点不重建 |
| **1356-1365** | 分节注释：说明英雄口径与全服口径并存，并**如实记下实测差异很小**（英雄 157 阶段 1 棱彩 46.98% vs 全服 45.0%，差 ≤2 个百分点，见 §3.4）——不宣称两者有量级差别 |
| **1366-1372** | 新增 `renderMayhemHeroRarityPanel(rows)`：`<section class="recommendation-section mayhem-hero-rarity">`，取不到就返回空串（整块隐藏） |
| **1374-1379** | 新增 `mayhemHeroRarityTotal(row)`：三组之和（前端自己的护栏） |
| **1382-1390** | 新增 `mayhemHeroRarityStage(row)`：单阶段一列，**复用全服口径那套 `.mayhem-rarity-stages` 标记结构**（`article > b + span.is-silver/.is-gold/.is-prismatic + small`），小字披露「N 项海克斯 · 三项合计 X%」 |
| **1880-1883** | `renderRecommendedAugments` 的 return 末尾追加 `${mayhemCautionNote(items)}`——**在 `<section>` 之内、卡片栅格之后**，即工单要求的「海克斯推荐卡的次要信息位」，没有新开 section。读的是**未裁剪的 `items`**，所以没进「每品质前三」的命中行也会被列出 |
| **1990-1999** | 分节注释 + `const MAYHEM_CAUTION_LIMIT = 5` |
| **2002-2010** | 新增 `mayhemCautionNote(items)`：只认后端下发的 `caution === true`，**不重算判据、不重排**（`.filter` + `.slice`，没有任何 `.sort(`），超过 5 条时写「按官方推荐顺序列出前 5 项」并给出总数 |
| **2014-2021** | 新增 `mayhemCautionItem(row)`：单项客观陈述「名称（品质 · 选取率 X% · 同品质中位数 Y% · 较基准 −Z%）」；名称取不到就整条不列，`deltaWinRate` 为 0/缺失时不拼「较基准」，中位数缺失时不显示 NaN |

**禁改项复核**（行号为改动后的新行号）：`sortedGradeRows`（1439 → **1476**）函数体一字未动；`renderMayhemItemRanking`（1144 → **1146**）仍然是 `objectRows(rows).slice(0, 8)` 不重排；`renderMeasurementTechnique`（1954 → **2028**）仍然只转发 `window.deepLegendsShared`，我没有另写口径渲染；`renderMayhemRarityPanel`（1344 → **1346**）函数体一字未动，仍由 `renderMayhemAtlas`（1268）在 1286 行调用；`mayhemWithoutItemCell`（1188）未动。

### 1.4 `backend/web/champions.css`（968 → **982** 行，只加 3 条规则）

| 位置 | 内容 |
|---|---|
| 970-978 | 分节注释（写明与 B 新增段同一条纪律） |
| **980-982** | `.mayhem-caution-note` / `.mayhem-caution-note b` / `.mayhem-caution-note span` |

**样式棘轮自查（R117 的 `r117-style.test.cjs` 全部通过，见 §6）**：新增段 **零 hex 字面值**；`padding: 12px 14px` 复用 `.mayhem-rarity-stages article`（335 行）已有取值；**没有引入任何新的 `border-radius` / `gap` 取值**；颜色只走 `var(--muted)` / `var(--warning)` / `var(--line)`；`.mayhem-caution-note` 在 JS 里被引用（未引用类名棘轮）；三条规则的 selector+body 组合唯一（重复声明组棘轮）。P1 的英雄面板**一行新 CSS 都没加**——复用 `.recommendation-section` 外壳 + `.mayhem-rarity-stages` 栅格。

### 1.5 新增测试与夹具

| 文件 | 行数 | 内容 |
|---|---|---|
| `backend/hexdata_r116f_test.go`（新建） | 493 | 9 个测试，函数名全部含 `Mayhem`（工单通用验证要求 `-run Mayhem` 能覆盖到） |
| `backend/web/r116f.test.cjs`（新建） | 405 | 11 个测试：jsdom 全应用冒烟（真实数值驱动）+ 降级红线 + 契约钉子 |
| `backend/testdata/r116/hexdata-hero-157-stage-rarity.json`（新建） | 144 行 / 56,065 B | 英雄 157 真实响应里 **augments[] 全量 126 条**的字段投影（`augmentId`/`augmentName`/`rarity`/`games`/`pickRate`/`deltaWinRate` + `stages[]{stage,pickRate,games,deltaWinRate}`），另含真实的前 3 件 items 与前 2 组 trios 原样字段（`loadMayhemDetail` 的 hero-json 形状校验要求 items/augments/trios 三个数组都非空）。**数值全部是上游原值，未做任何修改、裁剪或归一化**，ID 保持上游的字符串形状，走生产解析路径 `parseHexdataHeroJSONWithStats` |

**为什么必须新增夹具**：既有的 `hexdata-hero-157.json` 只保留了前 10 条 augments，用它算「三组之和 = 1.0 ± 0.01」必然失败（10 条的和约 0.08）。工单 P1 的验证判据要求「用一个英雄的 `hero-json` fixture 验证聚合结果」，只有全量 126 条才验证得了。完整原始响应 1.23 MB，没有塞进仓库；投影后 56 KB，与同目录既有夹具（58 KB）同量级。

### 1.6 改到的既有测试（1 处，属必要的 harness 修复）

`backend/web/r113.test.cjs` 第 95-99 行：`renderRecommendedAugments` 的隔离编译依赖表里补注入
`mayhemCautionNote:()=>''`，并把 R116-B 留下的那条注释扩写一句说明。**这是 R116-B 已经开过的先例**
（同一行注释里写着「R116-B P0-6：…把三个新依赖按默认『汇总』注入即可」）。不改这一处，
基线的 577 会掉到 576（`ReferenceError: mayhemCautionNote is not defined`）。
断言语义一字未改，只补依赖注入。

> **越界说明**：这个文件不在主控给的「可改」清单里，但也不在**禁触**清单里
> （禁触的前端测试只有 `r116b.test.cjs` 与 `r117*.test.cjs`）。基线「577/577 不许引入新失败」
> 是硬要求，两者只能满足一个时我选择了改 harness 并在账本与报告里显式披露。改动是 1 个注入项 + 注释。

---

## 2. 交付物对照（工单「交付物」四条）

| 工单要求 | 落地 |
|---|---|
| `computeHeroStageRarityDistribution` + 渲染 | `hexdata.go:2322`（签名逐字一致）+ `hexdata.go:2346` 下发换算 + `champions.js:1363` 渲染 |
| 负向推荐判据函数 + 渲染 | `hexdata.go:2463`（+ 2417 分组中位数、2435 中位数）+ `champions.js:1999` 渲染 |
| `docs/r116f-balance-param-findings.md` | 已产出，结论「明确放弃」，含穷举的已排除清单 |
| `docs/r116f-execution-ledger.md` | 本文件 |

---

## 3. P1 验证判据的实测复核结果

### 3.1 四阶段求和表（**我自己重算的，不是抄主控的核算**）

我用 `$PI_SCRATCH_DIR/hexdata/heroes_157.json`（英雄 157 完整真实响应）**独立写脚本重算**了一遍
（脚本 `$PI_SCRATCH_DIR/recompute.mjs`），结果与主控契约里的表格**逐位一致**：

| 阶段 | 有 stage 行的 augment 数 | `SUM(stages[].pickRate)` | 棱彩 | 黄金 | 白银 |
|---|---|---|---|---|---|
| 1 | **121（不是 126）** | **1.000003** | 0.469768 (n=50) | 0.449009 (n=38) | 0.081226 (n=33) |
| 2 | 126 | **1.000003** | 0.275884 (n=51) | 0.437865 (n=39) | 0.286254 (n=36) |
| 3 | 126 | **1.000001** | 0.271517 (n=51) | 0.439486 (n=39) | 0.288998 (n=36) |
| 4 | 126 | **1.000001** | 0.272221 (n=51) | 0.437424 (n=39) | 0.290356 (n=36) |

→ **工单 P1 的验证判据「三个概率之和应为 1.0（±0.01）」成立**，直接实现，**没有改口径、没有做归一化**。
四个阶段的偏差分别是 +3e-6 / +3e-6 / +1e-6 / +1e-6，远小于 ±0.01 的容差。

我另外核到的两条支撑事实：
- 126 条 augment 的 `rarity` 取值**只有** `棱彩` / `黄金` / `白银` 三种，没有第四种、没有空值
  （`unknown rarity values: []`），所以「三组」这个前提是上游数据本身就成立的。
- 阶段覆盖模式只有两种：`"1,2,3,4"` × 121 条、`"2,3,4"` × 5 条。**没有** `"1,2,3"`、`"1,2"` 之类的形状。

### 3.2 陷阱①：阶段 1 只有 121/126 条 stage 行 → 缺失的行跳过，不补 0

缺阶段 1 的 5 条我逐条打出来了（`$PI_SCRATCH_DIR/recompute.mjs` 输出，写进了测试断言）：

| augmentId | 名称 | 稀有度 | games |
|---|---|---|---|
| 1389 | 男爵之手 | 棱彩 | 7,641 |
| 1404 | 属性！ | 白银 | 9,493 |
| 1187 | 闪光弹 | 白银 | 1,051 |
| 1348 | 闪闪现现 | 白银 | 987 |
| 1150 | 面包和果酱 | 黄金 | 49,233 |

注意这 5 条**都不是小样本行**（最小的 987 场也超过 `hexdataMinimumSample` 的 250），
所以「阶段 1 少 5 条」不是样本过滤造成的，是上游 `stages[]` 本身就没有这一条。
实现里 `if stage.Stage <= 0 { continue }` + 只对真实存在的 stage 行累加，缺失的行**既不补
`pickRate=0`、也不进 `Augments` 计数**；`Augments` 字段（121/126/126/126）随响应下发并在 UI 小字里
披露给用户，让分母可见。

测试钉子：`TestMayhemHeroStageRaritySkipsMissingStageRowsInsteadOfZeroFilling`
（真实数据断言 121 + 那 5 个名字；再用合成用例断言「补 0 的实现会让阶段 1 多出一个 `gold` 键」）。

### 3.3 陷阱②：绝对不能用 augment 顶层的 `pickRate`

我在同一份真实数据上把「用错字段」的结果也算了出来（写进测试当反例钉子）：

| 口径 | 求和 | 棱彩 | 黄金 | 白银 |
|---|---|---|---|---|
| **`stages[].pickRate`（正确，阶段内归一化）** | 1.000003 / 1.000003 / 1.000001 / 1.000001（四阶段） | — | — | — |
| **augment 顶层 `pickRate`（错误，跨阶段选取率）** | **3.791613** | 1.235358 | 1.671008 | 0.885247 |

与主控实测的 3.791613 / 1.235358 / 1.671008 / 0.885247 **逐位一致**。
测试钉子：`TestMayhemHeroStageRarityMustNotUseTopLevelPickRate` 同时断言
「用错字段得到 3.791613（且不落在 1.0±0.01 内）」与「生产函数用 `stages[].pickRate`，四阶段全部满足判据」；
前端侧 `r116f.test.cjs` 另有一条断言 `renderMayhemHeroRarityPanel` / `mayhemHeroRarityStage` 的函数体里
**不出现 `pickRate` 字样**（前端只消费后端算好的百分数，不自己聚合）。

### 3.4 英雄口径 vs 全服口径：**实测差异比工单预期小得多**（一条必须纠正的陈述）

工单 P1 实现要求第 2 条给的理由是「两者信息量不同：全服口径回答『整体分布』，英雄口径回答
『这个英雄』」，主控契约里进一步写「阶段 1 的棱彩占比 47.0% 明显高于阶段 2-4 的 ~27%……
这说明『英雄专属口径』确实比全服口径多出信息量」。**前半句是跨阶段比较（成立），但它推不出
后半句**：本轮我把全服口径的真实数值也抓了下来（`https://hexdata.com.cn/augment-rarity`，
2026-09-21，HTTP 200 / 9,757 B，页面标题 `海克斯大乱斗海克斯稀有度概率 · Patch 16.18`、
`dateModified: 2026-09-18`，与英雄数据同 patch 同报告日，可直接对比）：

| 阶段 | 全服：白银 / 黄金 / 棱彩 | 全服选择次数 |
|---|---|---|
| 1 | **8.4% / 46.6% / 45.0%** | 221,750,338 |
| 2 | 28.6% / 43.8% / 27.6% | 220,965,328 |
| 3 | 28.8% / 44.0% / 27.1% | 212,721,276 |
| 4 | 29.0% / 43.9% / 27.1% | 184,313,835 |

**全服口径自己就有同样的阶段 1 偏斜**（棱彩 45.0% → 阶段 2-4 约 27%），所以那个偏斜是
**模式级现象，不是英雄专属信息**。我另外抓了 3 个英雄（223 河流之王 / 17 迅捷斥候 / 875 腕豪，
均 200、约 1.2 MB）做同样的聚合，与全服逐阶段作差：

| 英雄 | augments | 阶段 1 有 stage 行 | 阶段1 Δ(白银/黄金/棱彩) | 阶段 2-4 最大 \|Δ\| | 四阶段三组之和 |
|---|---|---|---|---|---|
| 157 疾风剑豪 | 126 | **121** | −0.28 / −1.70 / **+1.98** | 0.16 pp | 100.0003 / 100.0003 / 100.0001 / 100.0001 |
| 223 河流之王 | 128 | **124** | −0.34 / −1.21 / **+1.55** | 0.38 pp | 99.9999 / 99.9998 / 99.9998 / 100.0000 |
| 17 迅捷斥候 | 120 | **117** | −0.33 / −1.54 / **+1.86** | 0.28 pp | 99.9995 / 100.0001 / 100.0001 / 100.0002 |
| 875 腕豪 | 132 | **126** | −0.25 / −1.41 / **+1.66** | 0.26 pp | 100.0001 / 100.0000 / 99.9996 / 99.9998 |

三条结论，都写进了代码注释与 UI 文案的取舍里：

1. **P1 的验证判据在 4 个英雄 × 4 个阶段 = 16 组上全部成立**（99.9995 ~ 100.0003，偏差 ≤5e-4，
   远小于 ±0.01 容差）。判据不是英雄 157 的巧合。（仓库夹具只放了 157 的全量投影，
   另 3 个英雄的原始响应各 1.2 MB，没有塞进仓库；核算脚本与原始文件在
   `$PI_SCRATCH_DIR/r116f-probe/heroes_{223,17,875}.json`。）
2. **「缺失 stage 行」不是英雄 157 独有的**：121/126、124/128、117/120、126/132——四个英雄的阶段 1
   都少 3~5 条。陷阱①的处理（跳过、不补 0）是**通用必需**，不是个例兜底。
3. **英雄口径与全服口径的数值差异很小**：阶段 1 最大差 +1.98 pp（棱彩），阶段 2-4 最大差 0.38 pp。
   → 我**没有**在 UI 文案里宣称「英雄口径与全服口径有量级差别」，面板副标题只写
   「该英雄样本里各阶段选取到白银 / 黄金 / 棱彩的概率，全服整体分布见『海克斯图鉴 · 海克斯品质分布』」；
   `champions.js:1358-1365` 的注释里也把上面这张对比表的结论写了进去（原来那句
   「全服阶段 1 棱彩只有 30.9%」是错的——那个数字来自 `hexdata_test.go:204` 的**测试夹具**，
   不是真实上游值，已按实测值改掉）。
   → **工单要求两者并存，我照做了**（并存本身没有成本，且「这个英雄」与「整体」确实是两个问题），
   但工单给的那条理由（信息量差异大）**在实测数据上站不住**，如实记录在此，供主控判断要不要
   在下一轮把英雄面板做成「与全服逐阶段作差」的形式——那才是真正能体现英雄差异的呈现方式，
   本工单没有做（超出实现要求第 2 条的范围）。
UI 文案里**只陈述数值，没有对任何差异做因果解释**（面板小字只有「N 项海克斯 · 三项合计 X%」）。

### 3.5 P1 的落点决策（与主控锚点提示的一处偏差，需要主控确认）

主控的锚点提示是「P1 的『该英雄专属』对比视图挂在 `renderMayhemRarityPanel`(1344) 附近」。
实测发现 **R116-B 已经把 `renderMayhemRarityPanel` 搬进了全局图鉴视图**（`renderMayhemAtlas`，
`champions.js:1286` 调用；`mayhemView === "atlas"` 才渲染），而英雄专属分布是**单个英雄**的数据，
放进图鉴视图没有英雄语境。所以我按「**代码位置靠近 + 语义落在英雄详情**」来实现：

- 新函数 `renderMayhemHeroRarityPanel` 就定义在 `renderMayhemRarityPanel`（1346）**正下方**（1366），
  复用同一套 `.mayhem-rarity-stages` 标记与样式，两个视图形态一致；
- **渲染落点是英雄详情页的「概览」tab**（`mayhemOverviewTabMarkup`，1102），紧跟海克斯推荐卡；
- **全服口径面板一行未动、一个调用点未删**（`r116f.test.cjs` 有断言钉住它仍在图鉴里）；
- 英雄面板的副标题明确写出「全服整体分布见『海克斯图鉴 · 海克斯品质分布』」，两个视图互相指路；
- 英雄面板**刻意不用** `.mayhem-rarity-panel` 这个类名：`champions.js:906` 按这个类名触发
  `loadMayhemRarity()`，挂上它就会在详情页多打一条 `/api/champions/augment-rarity`
  （P1 是纯本地聚合，`r116f.test.cjs` 有请求预算断言钉住 0 条）。

**代价**：全服口径与英雄口径**不是像素级并排**（分别在图鉴视图与英雄详情里），需要点一次
「海克斯图鉴」入口才能看到全服那份。如果主控要求真正并排，可以在英雄面板里再渲染一份
`state.mayhemRarityData`（已缓存时零请求），但那样会出现「有时并排、有时只有一列」的不一致 UI，
我判断当前方案更稳，故未做。**这是本工单唯一一处需要主控点头的设计取舍。**

### 3.6 Anti-scope 第 1 条

**没有做**概率转移矩阵、四阶段组合枚举。`r116f.test.cjs` 有一条 `doesNotMatch(championsScript, /转移矩阵|组合枚举|transitionMatrix/i)` 钉住。

---

## 4. P2 负向推荐：判据、真实数据命中情况与文案

### 4.1 判据实现（三条全部满足才生成）

| 判据 | 实现 | 备注 |
|---|---|---|
| ① `pickRate` 超过该英雄**同稀有度**海克斯的中位数 | `mayhemPickRateMedianByRarity`（`hexdata.go:2417`）先按 `Rarity` 分组，再 `mayhemMedian`（2435）取组内中位数；判定是**严格大于**（`row.PickRate <= median` 就跳过） | 偶数个元素按标准定义取中间两数平均（工单没另给定义，不自造口径） |
| ② `deltaWinRate < 0` | `row.DeltaWinRate >= 0` 就跳过 | `= 0` 不算「低于基准」 |
| ③ `sampleTier != "low"` | 直接读 **R116-B 已落地的** `championMetricRow.SampleTier`（`champions.go:289`，JSON tag `sampleTier`），由 `applyHexdataSampleTiers`（`hexdata.go:292-311`）按 `meta.samplePolicy` 算出 | **没有自己再算一遍分档**；`markMayhemNegativeRecommendations` 函数体里不出现 250/1000 任何阈值（`r116f.test.cjs` 有断言钉住） |

### 4.2 一处必须披露的降级设计（工单没写，我主动加的护栏）

判据③按字面是 `sampleTier != "low"`。但 `sampleTier` 在 **`meta.samplePolicy` 不可用时是空串**
（`applyHexdataSampleTiers` 直接 return，见 `hexdata.go:292-311`），空串 `!= "low"` 成立 →
**所有行都会通过判据③**，包括样本量完全未知的行。那是把「不知道样本量」当成「不是低样本」，
属于项目红线禁止的「用推断值代替真实值」。

处置：调用方（`hexdata.go:3391`）先判 `snapshot.SamplePolicy.usable()`，**不可用就整块跳过**，
一条慎选提示都不给；判据函数本身仍按工单字面实现（便于单测直接驱动）。
测试钉子：`TestLoadMayhemDetailSkipsCautionFlagsWhenSamplePolicyIsUnavailable`
（同时证明「如果不跳过，判据③会形同虚设」——空分档下那条正样例仍然命中）。

### 4.3 真实数据上的命中情况（英雄 157，126 条全量）

我用真实响应独立算了一遍（脚本 `$PI_SCRATCH_DIR/caution.mjs`），Go 实现的结果与之**完全一致**：

| 项 | 实测值 |
|---|---|
| 126 条经 `hexdataMinimumSample`(250) 过滤后 | **126 条**（最小 games = 914，一条都没被过滤掉） |
| 同稀有度选取率中位数（百分数） | 棱彩 **1.6190**（n=51）／黄金 **2.3460**（n=39）／白银 **1.42015**（n=36） |
| 三条判据全部满足 | **37 条** |
| 被判据③（`sampleTier == "low"`）排除 | **0 条**（126 条全部是 medium/high，最低 914 场） |
| 命中示例（前 5，官方顺序） | 升级：无尽之刃（黄金 · 19.77% · −0.05%）／关键暴击（黄金 · 7.47% · −0.02%）／虹吸（白银 · 8.78% · −0.82%）／战争交响乐（棱彩 · 5.72% · −1.43%）／质变：黄金阶（白银 · 3.42% · −1.34%） |

**一个必须记录的实测事实**：这 37 条命中行里，**没有一条落在 UI 实际展示的 9 张卡里**
（海克斯推荐卡按官方顺序每品质取前三，展示的 9 条 deltaWinRate 全为正：掷骰狂人 +11.15%、
秘术冲拳 +7.14%、魔法转物理 +5.22%、灵魂虹吸 +2.90%、男爵之手 +4.65%、会心防御 +3.85%、
暴击飞弹 +3.44%、狂热者 +3.86%、狂徒豪气 +2.27%）。
→ **如果 P2 只做「逐卡片内提示」，这个功能在英雄 157 的真实数据上一条都显示不出来。**
所以我把渲染做成**读未裁剪的全量 `items`** 的一段陈述（§4.4），卡片内提示没有做。

另外：判据③在英雄 157 上过滤掉 0 条，**不能**据此认为它没作用——126 条恰好都 ≥914 场。
低样本排除的行为由合成用例覆盖（工单验证判据的反样例：样本 10 场 → `sampleTier="low"` → 不生成），
`TestMayhemNegativeRecommendationOnRealHero157Augments` 里还专门有一条断言把
「真实数据里 low 行数 = 0」钉住，防止后来人误以为判据③已被真实数据验证过。

### 4.4 渲染落点与文案（Anti-scope 第 2 条）

落点：`<section class="recommendation-section mayhem-augment-ranking">` **内部**、卡片栅格之后的一段
`<p class="mayhem-caution-note">`。**没有新开 section**（`r116f.test.cjs` 断言
`note.parentElement.classList.contains("mayhem-augment-ranking")` 且概览 tab 的直接子 section 数没变）。

真实渲染输出（jsdom dump，3 条命中行的样例）：

```
数据显示：3 项海克斯的选取率高于该英雄同品质中位数，但胜率低于基准（英雄级汇总口径，
样本分档非低档）。按官方推荐顺序：升级：无尽之刃（黄金 · 选取率 19.77% · 同品质中位数 2.35% ·
较基准 -0.1%）、战争交响乐（棱彩 · 选取率 5.72% · 同品质中位数 1.62% · 较基准 -1.4%）、
虹吸（白银 · 选取率 8.78% · 同品质中位数 1.42% · 较基准 -0.8%）。仅描述赛后关联，不构成因果结论。
```

- 文案骨架就是工单给的「数据显示：选取率较高但胜率低于基准 X%」，把「较高」具体化成
  **真实数值 + 同品质中位数**，比模糊的「较高」更可核对；
- **禁用词**：`r116f.test.cjs` 逐词断言渲染文案里不含「陷阱 / 坑 / 别选 / 禁用 / 最差 / 垃圾」，
  另有一条 `doesNotMatch(championsScript, /陷阱/)` **扫全文**（为了它能成立，我把源码注释里
  引用工单原词的那句改写成了「主观评判词（Anti-scope 第 2 条点名的那类词）」）；
- 口径标注「英雄级汇总口径」是必要的：阶段视图（P0-6 的 chips）下卡片数字是阶段值，
  而判据用的是英雄级汇总值，不标明就是混口径；
- 结尾「仅描述赛后关联，不构成因果结论」沿用项目一贯措辞（参考 Hexdata C 位榜的
  「描述赛后关联，不证明主动让经济会带来胜利」）。

### 4.5 工单 P2「现状证据」里的 `items.withoutItemWinRate` 反例：**已识别，本轮未纳入判据**

工单「现状证据」提到「可靠信号：`items.withoutItemWinRate` 反例（出了反而更差）」，
但「实现要求」第 1 条**只给了 augments 的三条判据、没有给 items 的任何阈值**。
处置：**以 augments 的三条判据为准实现**，`withoutItemWinRate` 反例只在这里记录，不做进判据。

- **理由**：工单没给阈值（「低于多少算反例」？「要不要也按某种中位数分组」？「样本门槛是多少」？），
  自行设定等于自造口径，直接撞项目红线「证据不足明确降级，不用推断值代替」。
- **数据通路已经齐了，将来加是低成本的事**：R116-A 已解析 `WithoutItemWinRate` / `WithoutItemGames`
  （`hexdata.go` 的 `hexdataItemRow`），R116-B 的 P0-1-3 已渲染「不出这件时胜率 X%」
  （`champions.js` 的 `mayhemWithoutItemCell`，`hexdataItemMetricRows` 里 ×100 下发）。
  只差一条判据函数与阈值来源，**建议留给下一轮工单，并让工单先给出阈值口径**。

---

## 5. P3 平衡参数探测：结论与已排除清单

**结论：探测不通过，明确放弃。** 完整文档见 `docs/r116f-balance-param-findings.md`，此处只摘要。

- 三条判定理由：① CommunityDragon 的 ARAM 维度是**逐技能 `SpellDataValue` 覆盖 + 起始装推荐覆盖**，
  不是英雄级平衡乘数；② **口径不匹配**（全球经典 ARAM mapID 12 vs 国服七区 `queueId 2400` 海斗），
  即使找到也不能当国服海斗的数值展示；③ **hexdata 侧没有平衡字段**（`meta` 78 个顶层键，
  balance/multiplier/damageDealt/damageTaken/modifier 命中全 0，`dataHealthWarnings` 为空）。
- **本轮把探测从「点状试 404」升级成「目录穷举」**：`raw.communitydragon.org` 允许目录枚举，
  我列出了 `plugins/.../v1/`（94 个 `.json` 全部列出）与 `game/data/` 全树
  （11 个子目录 + `characters/` 全部英雄目录），逐个排除。**这是结论能从「没找到」升级成
  「已穷举，确实没有」的关键。**
- 本轮新增的 4 条关键实测（都是真实状态码）：
  - `plugins/.../v1/kiwi-hub.json` → **200，2 字节，内容是 `[]`**。`KIWI` 就是海斗的内部模式名，
    文件存在但**全球客户端数据里一条都没有**；
  - `plugins/.../v1/queues.json` → 200，354,880 B，420 条队列，**确实含 `ARAM: Mayhem`**
    （2400/2401/2403/2405/2410/2450/3240/3270）与 `Brawl`(2300)，但 14 个字段名里
    balance/damage/multiplier/modifier 类字段 **0 个**；
  - `plugins/.../v1/maps.json` → 200，1,414 B，**地图表里没有海斗**（只有 0/11/12/22/30/453）；
  - `plugins/.../v1/augment-lists.json` → 200，22,353 B，`modeName` = `CHERRY`(44) /
    **`KIWI`(223)** / **`KIWI_JADE`(188)**，字段只有 `augmentList`(资产路径) + `modeName`，
    **知道海斗的海克斯池有哪些，但不知道任何数值**。
- 工单原文「`yasuo.bin.json`（200，无）」已**精确化**为「200，含 ARAM 逐技能覆盖但不含英雄级平衡乘数」，
  两处 `"ARAM"` 命中的原文 JSON 片段抄在 findings §4；提莫对照样本同样只有 2 处，
  `DamageDealt` 命中 0、`Modifier` 命中 0；亚索/提莫的 `Multiplier` 命中（4 处 / 7 处）逐处核对过，
  全是技能公式内部件、tooltip 展示倍率、通用 `critDamageMultiplier: 2.0`、对小兵野怪的 `monstermod`。
- **Anti-scope 第 3 条执行到位**：本轮**没有向 RESG / ARAMKit 发出任何请求**，连域名都没解析。
  findings §3.6 里只写「未发出任何请求 + 排除理由」，不列它们的路径。
- 工单列的另两条候选路径：**LCU 本地缓存 → 即使可行也不采纳**（需逆向未公开格式、口径无法与
  hexdata 国服七区交叉验证、合规链比抓公开页面更弱；且网络侧证据已显示全球包里海斗的数值维度是空的）；
  **版本公告解析 → 排除**（自然语言散文、无结构化字段、不覆盖国服海斗）。
- findings §6 写了**重开门槛**（三条任一成立才值得重开）与「不构成重开理由」的清单，
  避免以后重复调研。**全文没有任何「待进一步调研」的表述。**
- 一处如实标注的**未探测项**：Riot Games API 本轮没探测（需要开发者 API key，本仓库没有），
  已在 findings §6 明确写成「未探测」，**不作为放弃的理由**。

---

## 6. 验证记录（全部真实执行，2026-09-21）

### 6.1 工单「通用验证要求」

```
$ go test ./backend/... -run Mayhem -count=1
ok  lol-loot-assistant/backend  3.966s        # 24 PASS / 0 FAIL（含本工单新增 9 条）
```

### 6.2 构建 / 静态检查

```
$ go build -o "$PI_SCRATCH_DIR/dl-build-r116f" ./backend     # OK（必须带 -o）
$ go vet ./backend                                            # 无输出
$ gofmt -l backend/*.go                                       # 收尾时只列出 backend/gameplay.go
$ gofmt -l backend/hexdata.go backend/champions.go backend/hexdata_r116f_test.go   # 无输出
# ↑ backend/gameplay.go 是 R116-D 正在并行编辑的文件（禁触），不是本工单的产物，我没有碰它。
#   本工单改到的三个 Go 文件单独跑 gofmt 一律无输出。
$ node --check backend/web/champions.js                       # OK
```

> 中途 `go vet` 抓到我一条测试里的 `%` 转义错误（`超出 100% ± 1` → 应为 `100%%`），
> 已修（`hexdata_r116f_test.go:423`）。另有一处 gofmt 把 ①②③ 列表注释重排成缩进块，已 `gofmt -w`。

### 6.3 全量后端测试（基线 ok 217.789s）

```
$ go test ./backend/... -count=1
ok  lol-loot-assistant/backend  220.370s      # 收尾复跑全绿（改完当时那次是 217.676s），与基线同量级
```

### 6.4 前端测试（基线 577 tests / 577 pass / 0 fail）

```
$ cd backend/web && node --test
ℹ tests 588   ℹ pass 588   ℹ fail 0            # 改完当时：577 基线 + 11 条 R116-F 新增
ℹ tests 600   ℹ pass 600   ℹ fail 0            # 收尾复跑：R116-D 并行期间又加了 12 条 gameplay 测试，仍 0 失败
$ node --test r116f.test.cjs
ℹ tests 11    ℹ pass 11    ℹ fail 0
```

R117 的样式棘轮（`r117-style.test.cjs`：border-radius ≤33、padding ≤205、gap ≤54、重复 hex ≤35、
未引用类名 ≤93、重复声明组 ≤45、23 个 token hex 各 1 次）**全部通过**——新增 3 条规则零 hex、
零新 padding/gap/border-radius 取值。R116-B 的 29 条测试（`r116b.test.cjs`，本轮实测 `node --test r116b.test.cjs` → 29 tests / 29 pass / 0 fail）**全部通过、文件一字未改**。

### 6.5 新增测试清单

**后端 `backend/hexdata_r116f_test.go`（9 条，全部吃真实数据或明确标注合成）**

| 测试 | 钉住什么 |
|---|---|
| `TestMayhemHeroStageRaritySumsToOnePerStageOnRealHero157` | **工单 P1 验证判据本体**：四阶段三组之和 = 1.0 ± 0.01（实测 1.000003/1.000003/1.000001/1.000001），逐组数值到 1e-6，下发结构百分数 + 升序 + 条数 121/126/126/126 |
| `TestMayhemHeroStageRaritySkipsMissingStageRowsInsteadOfZeroFilling` | **陷阱①**：阶段 1 = 121 条、缺的 5 条名字逐条对上；合成用例证明「补 0 的实现会多出一个 `gold` 键」 |
| `TestMayhemHeroStageRarityMustNotUseTopLevelPickRate` | **陷阱②**：顶层 `pickRate` 求和 = 3.791613（棱彩 1.235358 / 黄金 1.671008 / 白银 0.885247），且不落在 1.0±0.01；生产函数四阶段全部满足判据 |
| `TestMayhemHeroStageRarityDegradesOnUnknownRarityAndIllegalStage` | 未知稀有度整条跳过（表现为该阶段 Total 明显 < 100%，可见降级）、非法阶段号跳过、三组全 0 的阶段不下发、空输入返回 nil |
| `TestMayhemNegativeRecommendationRequiresAllThreeCriteria` | **工单 P2 验证判据本体**：正样例「选取率 30% / `deltaWinRate` −0.05 / 样本 5000」→ 生成；反样例「选取率高但样本低（10 场 → `sampleTier="low"`）」→ **不**生成；另覆盖 `=中位数`不生成、`delta=0`不生成、`Rarity` 空不生成、幂等、空输入 |
| `TestMayhemNegativeRecommendationMedianIsGroupedByRarity` | **判据①必须先分组**：分组后黄金中位数 2 → 选取率 3 的黄金行命中；混合池中位数 11.5 → 同一行不命中（判据失效）。另钉偶数个取中间两数平均、奇数个取中间、空池 0、不修改入参、空稀有度不进池 |
| `TestMayhemNegativeRecommendationOnRealHero157Augments` | 真实 126 条：中位数 1.619/2.346/1.42015、命中 **37** 条；逐行断言「`Caution` ⟺ 三条判据同时成立」；命中行 `SampleTier` 必须非 low 且非空；`low` 行数 = 0（防止误以为判据③已被真实数据验证） |
| `TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags` | **端到端接线**：`loadMayhemDetail` 真的下发 4 个阶段（阶段 1 = 121 条、Total ≈ 100%）、37 条 `caution`、口径说明进了 `measurementTechnique`、`mayhem_stage_rarity` 与 `mayhem_caution` 两条诊断事件都在且 `deviatingStages = 0`；**hero-json 仍然只请求 1 次**（P1 没加请求） |
| `TestLoadMayhemDetailSkipsCautionFlagsWhenSamplePolicyIsUnavailable` | §4.2 的降级护栏：策略不可用时 `SampleTier` 全空，判据③形同虚设，所以调用方必须整块跳过 |

**前端 `backend/web/r116f.test.cjs`（11 条，jsdom 全应用 + 隔离编译）**

| 测试 | 钉住什么 |
|---|---|
| P1 概览 tab 渲染四阶段品质概率 | 真实数值逐格对上（阶段 1：白银 8.12% / 黄金 44.90% / 棱彩 46.98%；阶段 4 棱彩 27.22%）、`121 项海克斯 · 三项合计 100.00%`、无 `undefined/NaN/null`、面板在概览 tab 内、页脚不属于任何 tab |
| P1 分布缺失/为空/非法时整块隐藏 | 7 种输入（`undefined`/`null`/`[]`/`stage:0`/三组全 0/`"nope"`/`[{}]`）全部不渲染面板，且推荐卡不受影响 |
| P1 只渲染合法阶段 | `stage:-1` 与三组全 `null` 的阶段被丢掉，只剩合法那一列 |
| P1 保留全服口径面板 | `renderMayhemRarityPanel` 与其在图鉴里的调用点原样存在；英雄面板用自己的类名（不复用会触发 `loadMayhemRarity` 的 `.mayhem-rarity-panel`）；**`augment-rarity` 请求 0 条**；渲染后无补充请求；面板函数体不出现 `pickRate`；Anti-scope 第 1 条关键词全文不存在 |
| P1 随 tab 局部替换进出 | 切到构筑 tab 面板消失、切回恢复，**页脚节点始终是同一个对象**（不重挂） |
| P2 挂在次要信息位、不开新 section | `note.tagName === "P"`、`closest("section")` 是 `.mayhem-augment-ranking`、`note.parentElement` 就是那张卡、概览 tab 直接子 section 数没变；**展示的 9 张卡里没有命中行，陈述照样列出来** |
| P2 文案客观 + 禁用词 | 逐词断言不含「陷阱/坑/别选/禁用/最差/垃圾」，`champions.js` 全文不含「陷阱」；含「数据显示 / 英雄级汇总口径 / 样本分档非低档 / 仅描述赛后关联，不构成因果结论」；真实数值串（`战争交响乐（棱彩 · 选取率 5.72% · 同品质中位数 1.62% · 较基准 -1.4%）`）逐字对上；超 5 条按官方顺序截断并给总数；**列出顺序 = 后端直出顺序** |
| P2 无命中/字段缺失/名称为空时整块不渲染 | 3 种 jsdom 场景 + 6 种隔离调用（`caution:false`、字段整体缺失、空数组、`null`、`"nope"`、名称为空）；`deltaWinRate` 为 0/缺失时不拼「较基准」；中位数缺失时不出现 NaN；只 1 条命中时不说「列出前 5 项」 |
| P2 只属于概览 tab，切 tab 不发请求 | 切构筑消失、切回恢复、请求数不变、页脚节点不变 |
| P2 前端不重算判据、不重排 | `mayhemCautionNote`/`mayhemCautionItem` 里没有 `.sort(`、没有中位数计算、只认 `row?.caution === true` 与 `row?.cautionMedianPickRate`；后端三条判据的源码形状、`markMayhemNegativeRecommendations` 签名、判据函数体内不含 250/1000、三个新 JSON tag、`computeHeroStageRarityDistribution` 签名与工单逐字一致、函数体只用 `stage.PickRate` 且不用 `augment.PickRate`、非法阶段号跳过 |
| 新增样式合规 | R116-F 段零 hex、`padding` 复用既有取值、无新 `border-radius`/`gap`、新类名在 JS 里有引用 |

---

## 7. 防毁纪律执行记录

- 开工前把 `backend/hexdata.go`、`backend/champions.go`、`backend/web/champions.js`、
  `backend/web/champions.css` 拷进 `$PI_SCRATCH_DIR/r116f-backup/`；**每写完一个文件立刻增量备份**
  （`hexdata_r116f_test.go`、`r116f.test.cjs`、`r113.test.cjs`、新夹具同样入库备份）。
- 收尾 `shasum` 逐个核对 live 与备份（2026-09-21）：

| 文件 | sha1（live == backup） |
|---|---|
| `backend/hexdata.go` | `b1200717307678d0197f6ce418d989df53f2a57e` |
| `backend/champions.go` | `2029ee2b614c025cc5923c8d527b8d50b2358672` |
| `backend/hexdata_r116f_test.go` | `87459f7b5055af4aa0a0d7af8d2856a14ddbad1a` |
| `backend/web/champions.js` | `f3edfe3ce47b0e5d93623b14324f0fb8f7e50bd5` |
| `backend/web/champions.css` | `42e3b03a5648390413d8efbf2d19d3d1d43f01f3` |
| `backend/web/r116f.test.cjs` | `9d900b9d97723b347ab533aaa2b8e5cad7db3e9b` |
| `backend/web/r113.test.cjs` | `b0bc5d7a8233309c73c43e35689fa623dd546be2` |
| `backend/testdata/r116/hexdata-hero-157-stage-rarity.json` | `de8e2d67b95360d698b909108a30d69eb971a836` |
| `docs/r116f-balance-param-findings.md` | `9066de51972841c3b9887929ec14e663f356f22c` |
| `docs/r116f-execution-ledger.md` | 本文件自身无法自校验 sha（每次编辑都变），收尾时以最后一次 `cp` 到备份目录的快照为准 |

- **本轮没有遇到并行会话回滚**：`hexdata.go` 从 3677 行改到 3924 行后每次复核行数/sha 都一致，
  没有发现被还原成 HEAD 的情况。R116-D 并行改 `backend/gameplay.go` / `backend/web/gameplay.js`
  期间，我的每一次 `go build` / `go test` 都一次通过，没有撞上它的编辑中间态。
- **没有回滚或"修正"任何不属于 R116-F 的改动。**
- **没有改版本号**（`desktop/package.json` 保持 0.12.9，`package-lock.json` 未触碰）。

---

## 8. 留下的未解决问题 / 需要主控决定的事

1. **版本号未收口**：工单标注 0.12.12，本轮按主控指示**没有改** `desktop/package.json`
   （当前 0.12.9）。R116-D / E / F 三张工单都落地后由主控统一升到 0.12.12（含 `package-lock.json` 两处顶层 version）。
2. **P1 的落点取舍待主控点头**（§3.5）：英雄专属面板落在英雄详情「概览」tab、全服口径面板留在
   海克斯图鉴，两者**不是像素级并排**。如果主控要求真正并排对比，需要在英雄面板里再渲染一份
   `state.mayhemRarityData`（会引入一次 `/api/champions/augment-rarity` 请求，或「已缓存才显示」的
   不一致 UI）。我判断当前方案更稳，未做。
3. **`withoutItemWinRate` 反例未纳入判据**（§4.5）：工单只给了 augments 的三条判据、没给 items 的阈值，
   自造阈值等于自造口径。数据通路已齐（R116-A 解析 + R116-B 渲染），建议下一轮工单**先给阈值口径**再实现。
4. **`augment-lists.json` 里的 KIWI(223) / KIWI_JADE(188) 海克斯池清单**（findings §3.2）：
   这是本轮探测的意外收获——全球客户端数据里有海斗的**海克斯池归属**（虽然没有任何数值）。
   本项目目前的海克斯目录来自 hexdata + CommunityDragon 图标目录，池归属暂无用途。
   **本工单没有消费它**（超范围），只记录在此，供将来「海斗专属海克斯池」相关工单参考。
5. **`kiwi-hub.json` 现在是 `[]`**：如果将来某个版本全球包里这个表被填上了，findings §6 的重开门槛
   第 2 条就需要重新评估。建议下次做 hexdata 相关工单时顺手 `curl` 一眼这个 2 字节的文件（成本几乎为 0）。
6. **判据③在真实数据上没有过滤掉任何行**（§4.3）：英雄 157 的 126 条最低 914 场，全是 medium/high。
   低样本排除只被合成用例验证过。等遇到一个有低样本 augment 的英雄（冷门英雄更可能），
   建议用真实数据再复核一次判据③的行为——这不是「待定」，是**已知覆盖边界**，代码与测试都已就位。
7. **英雄口径与全服口径的实测差异很小**（§3.4）：4 个英雄逐阶段作差，阶段 1 最大 +1.98 个百分点
   （棱彩），阶段 2-4 最大 0.38 个百分点。工单 P1 实现要求第 2 条给的理由（「两者信息量不同」）
   在数值上**站不住**——阶段 1 的棱彩偏斜是模式级现象，全服口径自己就有。两个视图我按工单要求
   都保留了，但**建议下一轮考虑把英雄面板改成「与全服逐阶段作差」的呈现**（那才是真正体现英雄
   差异的形式），或者干脆评估这块面板的信息价值。本轮没有做：改呈现形式超出实现要求第 2 条，
   删面板又违反工单，两条都不是执行方该自行决定的。
