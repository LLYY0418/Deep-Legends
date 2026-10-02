# WORKLIST-R150：斗魂左侧榜单与右侧详情数据对不上；全项目梯度/评级统一成左侧的 OP-S-A-B-C-D-F；海克斯卡片「综合评分」无数据源；侧边栏变窄（R149 P3 遗漏，移入本单）

诊断人：Claude（用户截图 + `lol-loot-diagnostics-0924-1745.jsonl` + 源码走读）。**执行人 GPT 负责实现与真机验证。** Claude 没有改代码，没有请求过任何站点（沙箱访问不了 YOUR.GG / OP.GG / hexdata），所以凡是「站点返回了什么字段」的地方都标了「待探测」，实现前先做 §5 的探测。
日期：2026-09-24。基线：0.12.19 + R149 已执行改动。
状态：代码与自动验证通过，待 Windows 真机验收；按用户要求暂不改版本号或打包。前置：无。执行证据见 `docs/r150-execution-ledger.md` 与 `docs/r150-probe-findings.md`。

用户提出的四件事，一份工单四节：

1. **P1** 斗魂：左侧榜单的梯度 / 胜率 / 平均名次，与右侧详情头部对不上。
2. **P2** 梯度排序与显示规则不一致（左侧 OP-S-A-B，右侧 OP-1-2-3），项目里所有带评级的地方统一成左侧那一套。
3. **P3** 海克斯卡片的「综合评分」一直是「—」：查数据源，没有就删。
4. **P4** 侧边栏变窄、标题左右留白一致。这一项本来是 R149 的 P3，**R149 已执行但这一项完全没做**（`app.css` 里 `--sidebar-width` 仍是 226px，账本里也没有这一节；从账本看 GPT 拿到的是加 P3 之前的 R149 副本）。按项目惯例不往已关闭的 R149 里追加，本单把它完整带过来。**R149 请不要再重新复制执行，以本单为准。**

---

## 1. 证据

### 1.1 截图（同一位英雄「正义巨像」，斗魂榜，版本 16.19）

| 位置 | 梯度 | 胜率 | 平均名次 | 排名 | 样本 |
|---|---|---|---|---|---|
| 左侧榜单第 1 行（选中态） | **OP**（金色六边形） | **55.96%** | **3.26** | 第 1 | 榜单没有样本列 |
| 右侧详情头部 | **2（数字六边形）「2 档」** | **53.27%** | **3.36** | 总榜第 **22** 名 | 505 场 |

同一个英雄，四项全不一样。头部另外还带「选用率 14.38%」「禁用率 10.45%」。海克斯卡片上的徽章又是第三种样式：紫色方块的 S、黄色方块的 A，「综合评分」一栏全是「—」。

### 1.2 根因（源码，已逐行确认）

**斗魂页面同时用了两个不同的站点当数据源，左右两边各用各的：**

- 左侧榜单 = **YOUR.GG** 的斗魂英雄榜。`backend/yourgg_arena_rankings.go` `parseYourGGArenaRankings`：字段 `tier`（字母 OP/S/A/B/C/D/F）、`score`、`winRate`、`averagePlacement`、`firstPlacementRate`、`banRate`、`matches`。排名 `Rank = i + 1`（按上游给的顺序即按 score 降序）。行里另存了 `Grade = 上游字母`；数字 `Tier` 用的是内部映射 `OP→0, S→0, A→1, B→2, C→3, D→4, F→5`（**OP 和 S 都是 0**，所以任何只看数字 `tier` 的地方 S 会显示成 OP）。
- 右侧详情头部 = **OP.GG** 的斗魂详情。`backend/champions_structured.go` 约 1356 行 `response.ArenaStats = arenaChampionStats{Tier: stats.Tier, Rank: …, Games: stats.Play, AveragePlacement: TotalPlace/Play, FirstPlaceRate: FirstPlace/Play, WinRate: Win/Play, PickRate, BanRate}`。数字 `Tier`（OP.GG 的 1–5 档）、OP.GG 自己的排名、OP.GG 自己的样本量。
- 前端头部 `backend/web/champions.js` `renderArenaDetailPane`（约 1516–1552 行）写的是 `stats.winRate || row.winRate`、`stats.tier ?? row.tier`、`stats.rank || row.rank`……**详情（OP.GG）优先，榜单（YOUR.GG）只是兜底**。详情一加载完，头部就整体换成了另一个站点的数字。这就是「对不上」，不是某个字段算错。
- 头部梯度用 `tierBadge(tier, …)` 且**不传第三个参数 `sourceGrade`**，所以只会画数字图标（`/tier-icons/1.svg` …）；左侧表格（`champions.js` 约 1894 行）对斗魂传了 `row.grade`，画的是字母图标（`/tier-icons/yourgg-s.svg` …）。这是 OP-S-A-B 与 OP-1-2-3 两套长相的直接原因。
- 顺带发现：`champions.js` 约 1081 行 `tierBadge(row.tier, "topcard-tier")` 同样没传 `sourceGrade`。如果这张「顶卡」会渲染斗魂行，S 会被画成 OP。落实 P2 时一并查。

**日志能确认的部分**（`…0924-1745.jsonl`，构建 `2aa483b320b3`，0.12.19）：
- 斗魂这一路没有任何错误、熔断或形状告警：`arena_augments_built` 每次 `groupsIn=3, augmentsIn=45, rowsOut=45, missingMeta=0`；`arena_first_places` 正常；没有 `arena_rankings_parse_failed`、没有 `arena_aggregate_fallback`、没有 `arena_augment_catalog_failed`。也就是两个站点都成功返回了，**不是取数失败，而是两个数据源本来就不一致，前端把两者混着用**。
- 日志里**没有任何事件能看出左右两边各自拿到了什么数字**（榜单解析成功时不记事件，详情头部数值也不记）。所以本单要求补一个只记元数据的对账事件（见 §2.4），以后再有类似问题可以直接从日志看。

### 1.3 「综合评分」的数据源结论

- 斗魂海克斯行由 `champions_structured.go` `arenaAugmentGroups` 构造，数据来自 **OP.GG 的 `augment_group`**，结构 `opggAugmentMetric{id, play, win, total_place, first_place, pick_rate, win_rate}`：**没有 score，也没有任何官方档位字段**。构造 `championMetricRow` 时根本没给 `Score` 赋值。
- 随后 `applyLocalArenaAugmentGrades` → `applyLocalAugmentGrades` 会在一份**临时拷贝**上用 `localAugmentScore`（胜率的 Wilson 下界）算本地分，再按分位定 S/A/B；回填时**只把 `Grade` 拷回，`Score` 没拷回**。所以前端拿到的 `score` 恒为 0 → `number(0)` … 卡片上永远是「—」。
- 也就是说：**斗魂海克斯没有官方综合评分，也没有官方档位**；卡片上的 S/A/B 是本地按分位估的，只有 S（前 10%）、A（前 30%）、B（其余）三档。
- 对比：斗魂的**核心装备 / 棱彩装备**来自 YOUR.GG 聚合（`mapYourGGArenaAggregateItems`），`Tier`、`Score` 都是上游官方值，这部分的综合评分是真数据，**保留**。YOUR.GG 聚合失败时会回退 OP.GG 装备并用本地分，这时的分也是本地估的。
- 海斗（Mayhem）的海克斯用 hexdata 的 `hexScore`（官方），保留。

---

## 2. P1 斗魂头部与榜单对齐（以左侧榜单为准）

用户明确要求以左侧为准。**头部的梯度、胜率、平均名次、吃鸡率、排名、样本、禁用率全部取榜单行（YOUR.GG）**，不再用 OP.GG 详情里的同名字段。

### 2.1 前端 `renderArenaDetailPane`（`champions.js` 约 1516–1552）

1. 头部所有数字改为只读 `row`（榜单行）：`row.grade`/`row.tier`、`row.winRate`、`row.averagePlacement`、`row.firstPlaceRate`、`row.rank`、`row.play`、`row.banRate`。**去掉 `stats.* ||` / `stats.* ??` 的详情优先分支**。榜单行必然存在（`selected` 本来就要求 `selectedRow`），所以不需要再兜底到详情。
2. 「总榜第 N 名」= 榜单的 `row.rank`（YOUR.GG 按 score 排的名次），与左侧第一列一致。
3. **选用率**：YOUR.GG 榜单当前解析没有选用率。见 §5 探测第 2 条：若上游榜单行里有选用率字段，就读进榜单行并用它；**若没有，头部的「选用率」删除**（不再拿 OP.GG 的数字凑，混来源就是这次的问题）。删除时同步检查 `arenaStats.pickRate` 有没有别处依赖。
4. 详情正文里其余用 OP.GG 的部分（海克斯、装备、搭档、技能等）**不动**，它们没有 YOUR.GG 对应源。
5. `champions.js` 约 1944 行另一处斗魂详情指标（`arenaStats.averagePlacement`、`firstPlaceRate`…）同样改读榜单行；查一下这一段是否还在用（如果是无人调用的旧渲染路径，直接删）。

### 2.2 后端

- `championDetailResponse.ArenaStats`（`champions.go` 546 行，`champions_structured.go` 1356 行）里的 `Tier/Rank/WinRate/AveragePlacement/FirstPlaceRate/Games/BanRate` 不再被斗魂头部使用。**先 grep 全部消费者**：`gameplay.go` 约 6769 行（实时推荐 `heroStats = championDetailStats{Tier: detail.ArenaStats.Tier, WinRate…, PickRate…, BanRate…}`）、`web/gameplay.js` 的 `liveChampionTierBadge` 与推荐头部。
- **实时页（对局中的斗魂推荐）也必须与榜单一致**：把 `gameplay.go` 这一处改成取 YOUR.GG 榜单行（`loadArenaRankings` 已有缓存，按 `championId` 查行），梯度用字母。榜单里找不到该英雄时整块梯度隐藏，不回退到 OP.GG 数字。
- `ArenaStats` 里确实没人用的字段一并删除（`json:"arenaStats"` 有前后端契约测试，同步改）。仍有用的（例如 OP.GG 的 `RankPrevPatch` 若被别处用）保留，删除前逐个 grep。
- 不改 YOUR.GG 与 OP.GG 的请求量与缓存策略。

### 2.3 榜单本身

`parseYourGGArenaRankings` 保持：`Rank = i + 1`、`Grade = 上游字母`。**不要**再靠数字 `Tier` 表达档位（OP 与 S 同为 0 的内部映射只用于排序兜底，见 P2）。

### 2.4 对账诊断（只记元数据）

新增事件 `arena_header_source{championId, rank, grade, listGames, hasListRow}`，在斗魂详情头部成功渲染所依据的榜单行时由后端（或现有的前端诊断通道）记一条；榜单缺该英雄时记 `hasListRow=false`。**不记任何 cookie / 令牌 / 请求体。** 用途：以后用户再报「对不上」，日志里能直接看到头部用的是哪一行。

---

## 3. P2 全项目梯度统一成 OP / S / A / B / C / D / F

**规则以左侧榜单为准**：字母 `OP > S > A > B > C > D > F`，图标用左侧那套六边形（`/tier-icons/op.svg`、`/tier-icons/yourgg-{s,a,b,c,d,f}.svg`，7 个都已存在）。字母是唯一对外的评级表达。

### 3.1 现状清单（GPT 先自己 grep 一遍确认，下面是我读到的）

| 位置 | 现在显示 | 现在的来源 |
|---|---|---|
| 斗魂榜单（左） | 六边形字母 | YOUR.GG 字母 |
| 斗魂详情头部 | 六边形数字 1–5 | OP.GG 数字（P1 之后改读榜单） |
| 斗魂海克斯卡片 | 紫/黄/灰**方块**字母 `.augment-grade` | 本地分位 S/A/B（`champions.go` `applyLocalAugmentGrades`） |
| 斗魂核心/棱彩装备卡片 | 方块字母 | YOUR.GG 官方字母 |
| 海斗榜单与详情（`champions.js` 1137、1894） | 六边形数字 1–5 + 「本地估算」小字 | hexdata 官方数字档位，缺失时本地分档 |
| 海斗海克斯卡片 | 方块字母 | hexdata `hexTier` 映射：`hang→S, top→A, elite→B, npc→C, trap→F`（`champions.go` `hexdataOfficialGrade`）；另有一个文字指标「官方档位：夯/顶级/…」（`champions.js` 约 2146–2149） |
| 排位 / 大乱斗榜单与详情（`champions.js` 1081、1944） | 六边形数字 | OP.GG 数字 0–5（0 = OP） |
| 实时页（对局中）`gameplay.js` `liveChampionTierBadge`（5813 行）、`.augment-grade`（5580 行） | 数字图标 / 方块字母 | OP.GG 数字 / 各源字母 |

### 3.2 要做

1. **一处定义、全处使用。**
   - 后端：新增唯一的映射函数（放 `champions.go` 里，紧挨 `hexdataOfficialGrade`），把各上游的**原始档位**转换成字母，**API 里每一行/每个头部都直接带 `grade`（字母）**；前端不再自己把数字翻译成字母，也不再用数字 `tier` 渲染徽章。
   - 映射表（**每一条都要写进函数注释，并各有一个表驱动测试**）：
     - YOUR.GG：字母原样（OP/S/A/B/C/D/F）。
     - OP.GG 数字档位（排位 / 大乱斗 / 其余用到的地方）：`0→OP, 1→S, 2→A, 3→B, 4→C, 5→D`。（依据：OP.GG 的「OP」+「1–5 档，1 最强」。**待探测确认 5 档确实是最差档**，见 §5 第 4 条，不符就按探测结果改并记账本。）
     - hexdata 官方：`hang→S, top→A, elite→B, npc→C, trap→F`，`insufficient`/缺失 → **没有档位**（保持现状，不硬套）。hexdata 的**英雄级数字档位** 1–5 用与 `hexTier` 对应的同一张表（`1→S, 2→A, 3→B, 4→C, 5→F`），不要另起一套；具体对应关系以 §5 第 5 条探测为准。
     - 本地估算（斗魂海克斯、YOUR.GG 失败时的装备回退、hexdata 缺档位的英雄）：沿用现有分位（S ≥ 90%、A ≥ 70%、其余 B），输出仍是 `S/A/B` 字母，**不引入新等级**。
   - 数字 `tier` 字段保留仅用于内部排序/缓存兼容，前端不得再用它画徽章、也不得在界面上出现「N 档」字样（头部的「2 档」文字改为字母，例如「S 档」，或直接去掉这行小字，二选一，取与左侧最一致的那个，账本里写明）。
2. **一个前端徽章组件。** 把 `tierBadge`（六边形图标）与 `.augment-grade`（方块）合并成一个 `gradeBadge(grade, extra)`：所有评级位置都画左侧那套六边形字母图标。
   - 尺寸按位置沿用既有 CSS（榜单 20–26px、头部 24px、卡片略小），**保持各处现有尺寸，只换图标样式**；`.augment-grade` 的紫/黄/灰配色规则整体退役（`champions.css` 349–355、`gameplay.css` 1264、1732 附近同步清理，别留死样式）。
   - `tier-icons/1.svg … 5.svg` 退役；`op.svg` 保留。`liveChampionTierBadge` 删除，实时页改用同一个组件。
   - 缺失档位时的表现统一：整块不画，不画「—」占位徽章（现有的 `tier-badge-fallback` 文本占位一并去掉）。
3. **一个前端排序函数。** `gradeRank` / `sortedGradeRows` / `sortedArenaItemRows` 里现在各有一份 `{OP:0,S:1,A:2,B:3,C:4,D:5,F:6}`（`champions.js` 约 1582、1613 行）；合并成同一个常量。排序规则统一为：**字母档位 → 综合评分（有官方分时）→ 样本数**。
   - 注意 `parseYourGGArenaRankings` 里 OP 与 S 的数字都是 0，排序**不得**再依赖数字 `tier`，要用字母。榜单本身的顺序仍按上游给的 score 顺序（不要用字母重排，否则同档内顺序会变）。
4. **「官方档位」文字指标**（海斗海克斯卡片 `champions.js` 2146–2149，显示「夯/顶级/人上人…」）：卡片徽章已经是同一等级的字母，这个中文文字指标与徽章重复且是另一套叫法，**删除**。`hexLabel` 若别处（图例、弹窗、测试）也在用，同步处理；后端 `HexLabel` 字段仍要保留一个用途：`hexdataRowHasOfficialGrade` 依赖它判断「有官方档位」，字段本身不动。
5. **不改语义，只统一表达**：官方数据缺失时仍走原来的「隐藏 / 本地估算」逻辑，R116/R148 立下的规则（不用本地值冒充官方值、海斗榜单的「本地估算」标注）保持不变；「本地估算」小字位置与文案不动。

### 3.3 斗魂海克斯卡片上的本地 S/A/B

斗魂海克斯**没有官方档位**（§1.3），卡片上的字母是本地分位估的。本单**保留**它（用户没有要求删，且卡片排序依赖它），但与其他位置一致地用六边形字母图标、同一个排序函数。**在报告里请把这一点写清楚**（「斗魂海克斯的 S/A/B 是本地估算，不是官方档位」），是否也要整块隐藏由用户在真机看过效果后决定；**不要**自行在界面上加说明文字（用户明确不要统计口径类说明）。

---

## 4. P3 海克斯卡片「综合评分」：有官方值才显示，没有就整项删除

### 4.1 结论

- 斗魂海克斯：**无数据源**（§1.3）。卡片上的「综合评分」删除。
- 斗魂核心/棱彩装备：YOUR.GG 官方 `score`，保留。
- 海斗海克斯 / 适配英雄：hexdata `hexScore`，保留。
- 任何位置**本地算出来的分不得作为「综合评分」显示**。

### 4.2 后端

- `applyLocalAugmentGrades` 里的 `rows[index].Score = localAugmentScore(rows[index])` 会把本地估算写进 `Score`（装备回退路径 `applyLocalAugmentGrades(response.Build.CoreItems)` 会真的写进去并下发给前端）。改成**本地分只用于算分位，不写回 `Score`**（用局部切片存）；`Score` 只承载官方值（YOUR.GG `score`、hexdata `hexScore`）。
- `applyLocalArenaAugmentGrades` 的回填已经是「只回填 Grade」，保持，并加注释说明为什么不回填 Score。
- `mayhem` 系的 `Score`（hexScore）不受影响。

### 4.3 前端

1. `renderArenaOptionCard`（`champions.js` 约 1656–1663）：「综合评分」这一项**只有 `score > 0` 才加入指标列表**，没有就不出现（不要再渲染「—」）。其他没有值的指标（例如没有吃鸡率）沿用现有行为，不在本单动。
2. 检查 `data-metric-count` 与 CSS：斗魂海克斯卡片从 5 项变 4 项后网格排布要好看（现在是 3 列 × 2 行，最后一行 2 个；4 项应是 3+1 还是 2×2，让 GPT 看真机截图后选更整齐的，账本写明）。装备卡片仍是 5 项不变。
3. 文案：`champions.js` 约 1607 行的副标题「按综合评分、胜率与样本展示前九项」对斗魂海克斯已经不成立（实际排序是字母档位→样本），改为「按档位与样本展示前九项 · N 条」；**只改现有文字，不新增说明**。装备区的副标题不变。斗魂适配英雄等其他处的「综合评分」仍有官方来源的不动。

---

## 5. 第 0 步：探测（GPT 在用户真机，全部单资源、非批量、不记 cookie）

1. `GET` YOUR.GG 斗魂英雄榜原始返回，记录**字段名**（不记数值）：确认每个英雄行有哪些字段（`tier/score/winRate/averagePlacement/firstPlacementRate/banRate/matches` 之外是否还有 `pickRate/pick_rate/…`、`rank`）。结论写进 `docs/r150-probe-findings.md`。
2. 若有选用率字段：读进榜单行（`championRankingRow` 增加字段，前后端契约测试同步），头部用它；没有则按 §2.1-3 删除。
3. 抽 3 个英雄（含正义巨像）：记录 YOUR.GG 榜单行的 `tier/winRate/averagePlacement/matches` 与 OP.GG 详情头部对应字段，写进探测文件，用来确认「两个站点不一致」是数据本身的差异，而不是解析错误（例如把 `winRate` 当百分比还是小数、`matches` 与 `Play` 口径）。**如果发现是解析口径错误（例如 3.36 与 3.26 是 OP.GG 里 `TotalPlace/Play` 的算法问题），单独记结论并停下来回报，不要在本单里悄悄修**。
4. OP.GG 数字档位的方向与档数：确认 1 最强、5 最弱、0 为 OP（决定 §3.2 的映射）。
5. hexdata 英雄级数字档位与 `hexTier` 枚举的对应关系（确认 `1..5` 对应 `hang..trap`），以及有无 OP 级别。

---

## 6. P4 侧边栏变窄，顶部标题左右留白一致（原 R149 P3，未执行，整体带入）

### 现状（`web/app.css`，用户截图 + 代码）

- `--sidebar-width: 226px`（第 60 行）；`.sidebar` 内边距 10px（第 279 行）；`.sidebar-brand` 内边距 `4px 4px 13px`（第 281 行），logo 32px、间距 10px；标题 `DEEP LEGENDS` 用 Beaufort 18px、字距 .045em、`white-space: nowrap`（约 303–312 行）。
- 截图换算成 CSS 像素：logo 左缘距侧边栏约 14–16px，标题「S」右缘距侧边栏右边线约 35px。**右侧留白约是左侧的两倍多**。
- 导航项图标左缘在 `10 + 11 = 21px`（`.section-tab` 内边距 11px，第 463 行），而 logo 在 14px，没有对齐（约差 7px）。

### 要做

1. 标题左右留白相等，且左侧比现在略大：logo 左缘与导航图标左缘对齐（21px，即 `.sidebar-brand` 左内边距补到 11px 或等价做法），右侧留白与左侧相同。
2. `--sidebar-width` 改为「左留白 + logo 32 + 间距 10 + 标题实际宽度 + 右留白」，按上面的数字约 **208–212px**。**标题实际宽度以真机渲染为准**（Beaufort 字体加载后量 `.sidebar-brand-copy strong` 的 `scrollWidth`），取整并给 1–2px 余量防止子像素换行。
3. 收窄后检查：底部账号条（长名字应是省略号，不撑宽不换行）、设置按钮、导航文字、连接状态。若账号名截断难看，宁可宽度取大 2–4px，取舍写进账本。
4. 折叠态（`data-sidebar="collapsed"`）仍是 70px，窄屏媒体查询（约 915 行）不变；`.sidebar-brand::after` 分隔线左右各 4px，随左右留白调整。
5. 不新增任何说明文字。

### 测试

- Node（CSS 断言）：`--sidebar-width` 在 ≤ 214px 且 < 226px；`.sidebar-brand` 左内边距与导航图标左缘一致；折叠态仍 70px。**`champions.test.cjs` 第 2085 行现在断言 `--sidebar-width: 226px`，要一并更新**。
- 对抗变异：宽度改回 226px → FAIL；logo 左内边距改回 4px → FAIL。

---

## 7. 测试要求（P1–P3）

Go：
- 榜单解析：YOUR.GG 夹具，字母 → `Grade`、`Rank = i+1`；OP 与 S 两行的排序与显示互不混淆。
- 映射函数表驱动：§3.2 的每条映射各一行，含边界（`insufficient`、空串、未知字符串、OP.GG 数字 6 或负数 → 无档位）。
- 斗魂详情接口：不再需要 `ArenaStats.Tier/Rank/…`（视 §2.2 的删除范围）；实时推荐的斗魂梯度取自榜单行；榜单缺英雄时梯度为空。
- `applyLocalAugmentGrades`：装备回退路径下 `Score` 保持 0（不写本地分），`Grade` 仍按分位给出；官方行不参与分位池的既有规则不变（既有测试保持通过）。
- 对账诊断事件：字段齐全，不含任何取值之外的内容。

Node：
- 斗魂头部：给定「榜单行 = OP / 55.96 / 3.26 / rank 1」与「详情 stats = tier 2 / 53.27 / 3.36 / rank 22」，头部显示榜单值，且**不出现「2 档」**；给榜单行 S 时显示 S 图标而不是 OP。
- `gradeBadge`：7 个字母各渲染对应图标；缺失时返回空串；页面里不再引用 `/tier-icons/1.svg…5.svg`（grep 断言）。
- 斗魂海克斯卡片：没有 `score` 时不出现「综合评分」，有 `score` 时出现（装备）；副标题文案更新。
- 排序：合并后的常量只有一份（grep 断言）；`OP` 排在 `S` 前。
- 「官方档位」文字指标已不渲染。

对抗变异（每项都要让对应测试 FAIL，并整文件还原）：头部改回 `stats.* ||` 详情优先；`gradeBadge` 对 S 输出 OP 图标；`applyLocalAugmentGrades` 恢复写 `Score`；卡片恢复无条件渲染综合评分；映射表把 `1→A`；恢复 `.augment-grade` 方块渲染；侧边栏宽度改回 226。

## 8. 真机验证（GPT 负责截图，不用用户截图）

- 斗魂：选正义巨像，头部的梯度、胜率、平均名次、排名与左侧选中行**逐项一致**；再抽 3 个英雄同样核对，截图进账本。
- 各页面抽查徽章：斗魂榜单 / 斗魂详情头部 / 斗魂海克斯卡片 / 斗魂装备卡片 / 海斗榜单与详情 / 排位 / 大乱斗 / 实时页，全部是同一套六边形字母，界面上找不到「1 档」「2 档」或数字六边形，也找不到方块字母。
- 斗魂海克斯卡片没有「综合评分」，布局整齐；装备卡片仍有综合评分且是数值。
- 海斗卡片没有「官方档位：夯…」文字指标，徽章仍在。
- 侧边栏：默认主题与至少一个其他主题下量 logo 左缘到侧边栏左缘、标题末字右缘到右边线，两者相等（±1px）；账号条 20 字符左右的长名字不溢出。
- 日志导出：`arena_header_source` 有事件且 `hasListRow=true`，无新的 error 类事件。

## 9. GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test ./...` 与 R149 之后的基线对比，既有失败不得增加（含 `TestR92ProLadderStartsBeforeSupplementsFinish` 的偶发，若再现请单独记）；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。发版（版本号递增，`private` 模式，账本记 key mode、指纹、SHA256），写 `docs/r150-execution-ledger.md` 与 `docs/r150-probe-findings.md`，更新 `docs/WORKLIST-INDEX.md`。

## 10. 已知风险

- 榜单行与 OP.GG 详情的差异**本来就存在**（不同站点、不同样本、不同时间点）。P1 只是让头部不再混用；今后左侧刷新与头部之间不会再出现分歧，但「与 OP.GG 网页上看到的数字不同」是预期内的。
- P2 改动面大（前端徽章、排序、CSS、实时页、多个测试夹具），务必先 grep 清单、一次性改干净，不要留下「部分位置仍是数字」的中间状态。`tier` 数字字段若被缓存文件的既有版本引用，缓存 schema 版本要跟着递增，避免旧缓存缺 `grade`。
- OP.GG 数字 → 字母的映射是「等价对应」，不是站点官方说法；这是统一表达的必要取舍，已在函数注释里写明，不要在界面上加说明。
- 斗魂海克斯的本地 S/A/B 仍是估算（§3.3），是否隐藏待用户在真机上看过后决定。
