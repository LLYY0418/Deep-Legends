# R116 方案：海斗英雄详情页 / 对局推荐页 数据维度扩充

状态：**方案已确认，决策见第 8 节；待下令开工 R116-A**
撰写日期：2026-09-20（版本 0.12.7）
调研范围：resg.top、hexdata.com.cn、aramkit.com/zh-CN、hy.fan/CQMnyIf（虎牙海斗攻略站）
代码勘察范围：`backend/web/champions.js`、`backend/web/gameplay.js`、`backend/hexdata.go`、`backend/champions.go`、`backend/champions_structured.go`、`backend/gameplay.go`、`backend/aramkit_rating.go`、`backend/season_stats.go`

---

## 0. 三句话结论

1. **我们不是「基于 OP.GG」。** 海斗英雄详情的主源是 **Hexdata**（HTML 抓取）+ **OP.GG RSC**（只取出装顺序），海斗隐藏分来自 **ARAMKit `/rating`**，竞技场来自 **YOUR.GG + OP.GG**，ranked 才是 OP.GG + QQ101 + Riot(KR) + 腾讯 SGP。装配点：`backend/hexdata.go:1909-1990`（`Source = "Hexdata + OP.GG RSC"`）。
2. **最大机会是零新增上游、零爬取成本。** Hexdata 有 6 个 JSON 接口，我们只用了 `/api/hexdata/answer-cards` 一个。剩下的 `/api/hexdata/heroes/{id}`、`/postmatch`、`/hextech-insights`、`/augments/{id}` 已经把 ARAMKit 收费级的「表现指标 / 收益率 / 阶段 / 海克斯组合 / 对位克制 / 队友协同」全部算好了，而且是**国服 7 大区 905 万场**样本（ARAMKit 未公开分区，Hexdata 明确标注 CQ100/GZ100/HN1/HN10/NJ100/TJ100/TJ101）。
3. **我们真正的差异化不在「抄维度」，而在「局内实时」。** 四个参考站都是静态查询站，只有我们能在选人/对局中拿到我方 5 人 + 对方 5 人 + 已选海克斯 + 当前阶段 + 已出装备。把 Hexdata 的 `trios`（海克斯三件套）、`teammateSynergies`、`weakAgainst/strongAgainst` 接到这个实时上下文，能做出参考站做不到的**三选一实时排序**和**阵容协同/克制评分**。这是 P1 的核心。

---

## 1. 四个参考站的维度盘点

### 1.1 ARAMKit（aramkit.com/zh-CN）— 维度最全，是主要对标对象

站点自述：分析局数 3110.4 万，数据更新 2026-09-18，支持全分段/高分段切换、v16.16~v16.18 版本切换、低样本过滤阈值可调。

**英雄详情页六大板块（`/zh-CN/champions/yasuo`）：**

| 板块 | 具体维度 |
|---|---|
| 表现（概览 6 项） | 平均 KDA、对英雄伤害、承受伤害、参团率、伤害占比、游戏时长 —— **每项都带「较平均 ±%」** |
| 表现·详细数据（4 组 20 项） | 战斗 KDA：KDA / 击杀 / 死亡 / 助攻 / 参团率 / 多杀<br>伤害输出：对英雄伤害 / 物理 / 魔法 / 真实 / 伤害占比 / 防御塔伤害<br>生存辅助：承受伤害 / 自我减免 / 队友治疗 / 队友护盾 / 控制时长<br>经济节奏：获得金币 / 补刀 / 游戏时长 / **伤害经济转化率** |
| 海克斯（126 条） | 按**稀有度**（棱彩/黄金/白银）+ **阶段 1-4** 双维筛选；列：排名 / 海克斯 / 胜率 / 选取率 / **收益率** |
| 玩法 | 召唤师技能（5 组）、技能加点（5 组） |
| 构筑 | **核心出装流派**（暴击/战士/AD/特效/坦克，各带胜率+选取率+收益率）、出门装（5）、鞋子（5）、后续成装（20） |
| 单件装备（94 件） | 按**出装顺位**（总体/第1~6件）+ **24 个属性分类**筛选 |
| 海克斯组合（10 组） | 两两组合的胜率 / 选取率 / 收益率 |

**海克斯详情页：** 适配英雄（173，列含**英雄自身胜率**与**收益率**，可直接看出「这个海克斯把它抬了多少」）、适配装备（118，同样带装备胜率对照）、**阶段数据**（各阶段独立胜率/选取率/收益率）、海克斯组合（10）。

**地图环境页（我们完全没有的维度）：** 3 张地图（嚎哭深渊 34.0% / 莲华栈桥 32.9% / 屠夫之桥 33.1%，各带平均时长）；**红蓝方胜率对比**（蓝 51.9% vs 红 48.1%），并按 6 个职业拆分；英雄表增加 **蓝方胜率 / 红方胜率 / 阵营差异 / 相对阵营差**（相对阵营差 = 扣掉全服蓝方优势后的净差，这个口径设计很值得抄）。

**趣味榜单（14 个，全部是「反差」维度）：**
- 发现与反差：冷门高胜率、热门低胜率、**短局英雄榜**、**长局英雄榜**（列：胜率 / 基准胜率 / 相对变化 / 短局占比）、阵营胜率差异榜、地图专精英雄榜
- 战斗表现：英雄伤害榜、承伤减伤榜、治疗护盾榜、防御塔伤害榜、连杀榜
- **组合与克制：海克斯组合协同榜、最佳搭档榜、最差搭档榜、超预期克制榜**（列：胜率 / 基准胜率 / 相对变化）

**海克斯稀有度概率页（概率预测器，很硬核）：** 实战概率预测器（第 1 选概率、最可能海克斯）、4 个阶段的稀有度分布、**下一阶段概率转移矩阵**（当前阶段 × 白银/黄金/棱彩）、**四阶段组合概率枚举表**。

**隐藏分查询页：** MMR + 表现分 + 近期对局明细，并明确披露 4 条口径限制（组队会拉平分数、显示的是当前分而非当时分、结算时间为数据截止时间、未收录玩家为推算值）—— **这种「主动披露不确定性」的文案风格值得抄，和我们的数据准确性红线一致。**

**显示设置（全局）：** 低样本过滤（隐藏低样本 / 少于 200 场 / 自定义阈值，按数量或百分比）、**英雄装备统计口径**（原始数据 / 排除特定海克斯的修正胜率 / 指定特定海克斯的修正胜率）—— 即「出这件装备的胜率」可以扣掉海克斯带来的加成，解决「海克斯和装备的收益混在一起」这个归因难题。

### 1.2 Hexdata（hexdata.com.cn）— 我们已接入，统计口径最规范

页面维度少于 ARAMKit（英雄榜 / 海克斯榜 / 装备榜 / 海克斯概率 / C位榜 / 趣味数据 / 方法 / 往期年鉴），但**统计方法论是四站里最严谨的**，且已经在 `answer-cards` 里以机器可读形式给了我们：

- 样本分档：`samplePolicy.low/medium/high`（<250 低，250-999 中，≥1000 高），代码里已解析（`backend/hexdata.go:90-110`，`hexdataMinimumSample = 250` 在 `:40`）
- 排序方法：`recommendationPolicy.ranking = "sample_tier_then_wilson_lower_bound_v1"` + `wilsonZ` + `minRecommendationGames` —— **Wilson 95% 置信下界，先按样本分层再排**，明确目的是「不让样本少的高胜率排太前」
- HexScore 定义：「这个选择比英雄平均水平高出多少，样本少就扣分」；综合评分再把跨英雄的可靠表现汇总
- **装备适配度会主动留空**并解释原因（样本不足需更大差距才能确认 / 该装备本身差距就小）—— 「一个看起来精确、实际靠不住的数值不如不给」
- C 位榜（经济排行，独一份的维度）：**「每多拿 1% 队内经济，胜率约提高 X%」** + 建议经济占比 + 参考场次 + 搭配一致性；并明确声明「描述赛后关联，不证明主动让经济会带来胜利」
- 数据溯源：`buildId`（`hexdata-2026-09-14-4984af50e055`）、`reportDate`、`reportPatch`、`snapshotId`、7 大区分区场次、`eligibleMatches` / `participantRows` / `snapshotMatches`、「只统计正常结束且超过 4 分钟的比赛」

**关键发现：Hexdata 有 6 个 JSON 接口，我们只用了 1 个。**（详见第 3 节）

### 1.3 虎牙海斗攻略站（hy.fan/CQMnyIf → hd.huya.com/gametool/lolhaidou）— 编辑向，思路可借鉴

纯静态资料库（`数据来自海斗攻略站静态资料库`），无样本量、无置信度，数据严谨性最低，但**产品思路有两个独门维度**：

1. **流派化出装 + 情境化推荐**：不按「胜率前 3」给装备，而按流派给「核心 6 件 + 备选 6 件」，且流派名直接是决策场景 —— 坦克位给出「均衡出装 / **对面 AD 多** / **对面 AP 多**」三套，每套带一句适用理由。这是把数据翻译成了玩家能直接执行的动作。
2. **陷阱/慎选（负向推荐）**：英雄详情有 `specialAugments`（专属联动）和 **`trapAugments`（陷阱联动）**，文案「这组搭配在 X 身上需要谨慎选择」「注意技能触发条件与装备冲突」，还有「需要留意的装备组合」「陷阱提醒」。**所有参考站里只有它做负向推荐** —— 而实战中「别选什么」往往比「选什么」更值钱。

其余：海克斯矩阵（关联海克斯 / 关联玩法 / 关联装备 / 海克斯直接关联装备）、搭配实验室、英雄信号（收录胜率 + 强度标签：顶级强度/高强度/强势/观察中）、改动日志与版本记录（`changes[].version`）、装备买卖金币（购买金币/出售金币）、按属性类型（Health/Armor/MagicResist…）聚合装备。

### 1.4 RESG（resg.top → B 站 Toy）— 有 4 个独家维度，价值高于预期

`resg.top` 302 → `bilibili.com/toy/resg/index.html`，真身在 iframe：`https://www.bilibilitoy.com/toy/resg/19226257645568-v15741/`（Vue 3 + Chart.js）。**它是正经的海斗数据站**，我第一轮没解压 gzip 误判了，已修正。

**架构：没有后端 API**，数据是按版本预生成的 ESM `.js` 静态文件，路由 `/v/:version`、`/v/:version/champions/:id`、`/v/:version/augments/:id`。实测版本表只有 3 个：16.18（采集于 2026-09-17）/ 16.17 / 16.16。数据自采自 **Riot Games API**，素材来自 **CommunityDragon**（页脚声明）；样本量比 Hexdata 小（亚索 58.4 万 vs 国服 97.7 万），**未明示大区**。

数据文件布局（全部实测可用）：

```
api/v1/versions.js                                 版本表（version + collectedAt）
api/v1/versions/{ver}/champions.js                 英雄榜（items/total/page/pageSize）
api/v1/versions/{ver}/champions/{id}.js            英雄详情（49 KB）
api/v1/versions/{ver}/champions/{id}/trend.js      跨版本趋势
api/v1/versions/{ver}/augments/{id}.js             海克斯详情
api/v1/items-detail.js                             装备说明
c/zh_cn/v1/items.js、c/zh_cn/champions/all.js、c/zh_cn/v1/summoner-spells.js   CommunityDragon 中文数据
```

**缓存策略值得直接抄**：数据文件按小时失效（`?v=Math.floor(Date.now()/36e5)`），图标按天失效（`/864e5`）。

**英雄详情的 8 个数据块（实测亚索 157）：**

| 键 | 内容 | 字段 |
|---|---|---|
| `champion` | 英雄概要 | tm / wm / wr / **tier（T0/T1…）** / roles / alias |
| **`bb`** | **平衡参数**（独家） | 例：`{"攻击速度增长": "+2.5%"}` |
| `b.SPELLS` | 召唤师技能组合（25 组） | rank / tm / wr / **pr（采用率）** |
| `b.BOOTS` | 鞋子（8 项） | 同上 |
| `b.SKILL_ORDER` | 技能加点（6 组，主副顺序） | 同上 |
| `si` | 出门装组合（10 组） | rank / tm / wm / wr / pr |
| **`ac`** | **海克斯 → 阶段 → 出装联动**（独家） | 按阶段分组（stage1 113 条 / stage2 887 条），每条含海克斯 `a[]`、`dl`（收益）、`ts`，以及 **`b[]`：选了这个海克斯之后最常出且胜率最高的装备组合（items[] + tm + wr）** |
| `ra` | 该英雄全部海克斯（126 条） | tm/wm/wr/pr/awr/**dl**/**cl**/**ch**/**cf** —— **cl/ch 是置信区间上下界，cf 是置信度等级（"高"）**（独家） |
| `ia` | 单件装备排行 | rank/item/tm/wm/wr/pr + **`coverage`：summarySamples / detailSamples / detailCoverage / generatedAt / sourceSnapshotSha256**（自披露覆盖率；detailCoverage=0 时诚实标 0，不假装完整） |
| **`atp`** | **该英雄各阶段的海克斯品质概率**（独家） | 亚索阶段1：棱彩 46.96% / 黄金 44.94% / 白银 8.09%；阶段 2-4 棱彩降到 ~27%。**Hexdata 与 ARAMKit 的稀有度概率都是全局口径，只有 RESG 按英雄区分** |

**海克斯详情**：`augment`（tm/wm/wr/tier/quality/description/tooltip）+ `champions[161]`（每英雄 tm/wm/wr/**dl**/pr）。

**趋势（`trend.js`）**：`points[]` 跨版本折线 —— 亚索 16.15→16.18 的 wr（0.5673→0.5624→0.5687→0.5697）、pr（0.1027→0.1109）、matches（22.6 万→58.4 万），前端 Chart.js 画曲线，可在「稳定方案 / 曲线」间切换。

**值得借鉴的 5 点（按价值排序）：**

1. **`ac`：海克斯 → 阶段 → 出装联动。** 这是对局推荐页最该有的形态 —— 局中已知「你选了什么海克斯 + 当前阶段」，直接给「选了这个之后该出什么」，而不是给一份与海克斯无关的通用出装路线。**Hexdata 的 5 个接口里没有这个维度**（它的 `items[]` 是无条件的），但我们可以自算，见 P2-6。
2. **`atp`：英雄专属的各阶段品质概率。** 能回答「我这个英雄下一阶段有多大概率刷出棱彩」，直接影响「要不要为了棱彩改变当前选择」。Hexdata 的 `/heroes/{id}` 里 `augments[].stages[]` 带 rarity + pickRate，**按阶段把 pickRate 按稀有度聚合就能算出等价结果，不需要新数据源**。
3. **`ra.cl/ch/cf`：英雄 × 海克斯粒度的置信区间 + 置信度等级。** 比 Hexdata 只给 Wilson 下界、ARAMKit 只给低样本过滤更进一步。Hexdata 的 `wilsonLowerWinRate` 已够做「下界」，上界需自算（Wilson 区间两侧都能算，属纯计算）。
4. **`bb`：平衡参数。** ARAMKit 也有。**但获取路径比预想难**：实测 CommunityDragon 的 `champions/157.json`、`champion-summary.json`、`game/data/characters/yasuo/yasuo.bin.json` 三个端点**都没有 ARAM balance 字段**，`champions-summary.json` 与 `aram-champion-balances.json` 直接 404。RESG 证明数据存在（它大概率来自游戏内 bin 文件或版本公告），需要一次独立的可行性探测，见第 5 节。
5. **跨版本趋势 + 按版本路由。** 前提是**自己留存历史版本快照**。Hexdata 明确不做跨来源趋势；我们若只接 Hexdata 的单版本快照就画不出曲线 —— 但只要按 `buildId` 每次把取到的数据存一份，攒够 2-3 个版本后自然能做（纯本地积累，无额外请求）。

**不建议直接接 RESG 的数据**：它是 B 站 Toy 沙箱里的静态站（iframe 带 `sandbox` 属性），页脚只有 Riot 商标声明和 ICP 备案号，**没有 Hexdata 那样明确的引用许可条款**；且版本目录带构建号（`19226257645568-v15741`）会随发版变化，路径不稳定。定位与 ARAMKit 一致：**维度参考，不做数据源**。

---

## 2. 我们当前有什么（事实清单）

### 2.1 海斗英雄详情页

入口：英雄页 → `aram-mayhem` → 右侧详情面板。渲染主体 `renderMayhemDetailPane()`（`backend/web/champions.js:998-1019`，各 section 拼接顺序在 `:1018`），后端装配 `loadMayhemDetail()`（`backend/hexdata.go:1909-1990`），响应结构 `championDetailResponse`（`backend/champions.go:397-427`）。

| 现有维度 | 字段来源 | 代码位置 |
|---|---|---|
| 原画/头像/中英文名/称号 | catalog | `champions.js:203`、`champions.go:150-158` |
| 梯度徽章 + 「总榜第 N 位」 | Hexdata heroes 表 `tier`/`rank` | `champions.go:181-182` |
| 胜率、样本 | `detail.stats.winRate` / `row.play` | `champions.go:381`、`:186`（`hexdata.go:1281`） |
| 推荐海克斯（每品质前 3，共 ≤9） | `recommendedAugments[]`：胜率 / 样本 / 综合评分 / 本地 grade 徽章 / 稀有度 | `champions.js:1595-1641`、`champions.go:254-301,420` |
| 开局配置：出门装 top2 / 鞋子 top2 / 召唤师技能 top2 | `build.starterItems/boots/summonerSpells`，**只显示选用率，无胜率** | `champions.js:1031-1038,1964-1968`、`champions.go:333-336` |
| 装备路线（核心装 + 第四/五/六件） | OP.GG RSC 顺序 | `champions.js:1040-1043` |
| 装备排行 | Hexdata | `champions.js:1021-1029` |
| 技能加点 | OP.GG | `champions.js:1800-1818` |
| 统计口径说明 | `measurementTechnique` + `citation` | `champions.js:1643-1648` |
| 海克斯图鉴 + 稀有度品质分布 | `/augments`、`/augment-rarity` | `champions.js:1045-1131` |

### 2.2 对局推荐页

根渲染 `renderRecommendationArea()`（`backend/web/gameplay.js:5069-5122`），tab 定义 `recommendationTabSpecs()`（`:5036-5042`），后端 bundle `gameplay.go:3597-3764`，detail→bundle 映射 `gameplay.go:5363-5452`。

海斗局当前只有 2 个 tab：**「海克斯与出装」+「详情」**（经典模式才有「符文」tab）。

| 现有维度 | 代码位置 |
|---|---|
| 英雄概要卡：胜率 / 选取率（海斗**无禁用率**，本来也不 ban） | `gameplay.js:5327-5384`，stats 数组在 `:5377-5381` |
| 分路 chips + 占比 + 冷门分路披露 | `gameplay.js:5348-5370` |
| 优势对抗 ×3 / 劣势对抗 ×3 | `gameplay.js` `.champion-matchups` —— **海斗不出现**：`hextech-aram` 的 spec 无 `HasCounters`（`champions_structured.go:165-172`） |
| 海克斯推荐（银/金/棱彩三列，grade 徽章 + 胜率 + 场次） | `gameplay.js:5169-5218` |
| 装备推荐：出门装 / 召唤师技能 / 鞋子 / 技能加点 | `gameplay.js:5650-5710`、`renderSkillPlan :5759-5776` |
| 棱彩装备条 + 核心装路线 + 装备排行（胜率/场次） | `gameplay.js:5693-5721` |
| 「应用装备方案」写入客户端 | `gameplay.js:5723-5757`（**注意 `:5689`：`hasAugments=true` 时按钮被置空，海斗局看不到**） |
| 详情 tab：我方/对方 10 人、位置、段位、常用位置、近 N 局胜负/胜率/KDA、近 8 场评分 | `gameplay.js:5288-5315,4965-4987,5227-5248` |
| **推荐页完全没有统计口径说明** | 后端已在 `gameplay.go:5378` 给出 `measurementTechnique`/`citation`，前端未渲染 |

另有 `/api/gameplay/mayhem-rating`（海斗隐藏分，ARAMKit）渲染在总览页排位卡 popover（`gameplay.js:2204-2242`），**不在这两个页面内**。

### 2.3 我们已具备但未使用的自建能力

- **国服 SGP `SUMMARY` 已返回海斗单局 10 人完整数据**：`playerAugment1..6`、`totalDamageDealtToChampions`、`totalDamageTaken`、`goldEarned`、`gameDuration`、`item0-6`、`perk0-5`（`backend/sgp_api.go:893-897,1016`）。
- **但海斗一场都不入库**：`season_stats.go:169-171` 硬过滤 `if info.QueueID != 420 && info.QueueID != 440 { return }`。海斗队列是 **2300/2400/3270**（`queue_groups.go:120-124`）。
- ARAMKit `/rating` 的 `matches[].participants[]`（10 人 championId + rating + killParticipation + teamId）**已解析进内存后主动丢弃**（`aramkit_rating.go:61,64-76` vs `:353-359`），出于隐私边界。
- 韩服 Riot match-v5 支持按 `queue` 过滤拉取（`riot_api.go:804-818`），落盘 365 天（`riot_match_cache.go:51-61`，上限 600 项 / 128MiB）。
- 后端已有 Wilson 下界与样本分层口径（Hexdata `answer-cards.methodology`），**前端一处都没展示**。

---

## 3. 关键发现：Hexdata 未使用的 5 个 JSON 接口

从 `https://hexdata.com.cn/assets/main-D1lsCJ42.js` 提取，实测结果（2026-09-20）：

| 接口 | 实测 | 体量 | 内容 |
|---|---|---|---|
| `/api/hexdata/heroes/{id}` | **200**（需 `Referer: https://hexdata.com.cn/`） | **1.2 MB / 英雄** | 见下方逐项 |
| `/api/hexdata/postmatch` | **200** | 89 KB（全 173 英雄） | 每英雄 22 项表现指标 |
| `/api/hexdata/hextech-insights` | **200** | 372 KB | 173 英雄 topItems/topAugments + 211 海克斯全局 |
| `/api/hexdata/augments/{id}` | **200** | 300 KB | 海克斯 → 173 英雄适配，每英雄带 stages |
| `/api/hexdata/meta` | 200 | 7.8 KB | buildId / reportPatch / tierBands / sampleScope（7 大区 905 万场） |
| `/api/hexdata/ops` | 200 | 11 KB | 健康状态、最新爬取时间 |
| `/api/hexdata/heroes`（列表） | **403** | — | `{"error":"data_not_public","message":"此入口不提供批量数据。请停止未经许可的自动化访问。"}` |

### 3.1 `/api/hexdata/heroes/{id}` 顶层 8 个数组

以亚索（157）实测：

```
items[116]              单件装备
trios[~1000]            海克斯三件套组合
augments[...]           该英雄可用海克斯（每个带 stages[1..4]）
weakAgainst[...]        劣势对抗（被谁克）
strongAgainst[...]      优势对抗（克谁）
teammateSynergies[...]  队友协同
terminalItemTrios[...]  成装三件套
summonerSpellPairs[...] 召唤师技能组合
```

**逐项字段清单：**

| 数组 | 字段 | 能支撑的新维度 |
|---|---|---|
| `items` | winRate, pickRate, games, wins, hexScore, **hexTier/hexLabel/hexTierColor**（夯/顶级/…）, tier, **deltaWinRate**（相对英雄基准收益）, **coreDelta**, **averageIndex**（平均出装顺位）, recommendationScore, **wilsonLowerWinRate**, **withoutItemGames / withoutItemWins / withoutItemWinRate** | 单件装备「**出 vs 不出**」对照胜率、收益归因、出装顺位、Wilson 下界、官方档位标签（替代我们现在本地算的 grade） |
| `augments[].stages[1..4]` | stage, winRate, pickRate, **deltaWinRate**, **stageBaselineWinRate**, hexScore, hexTier/hexLabel, wilsonLowerWinRate, recommendationScore, games/wins, rarity | **阶段 × 英雄 × 海克斯**三维收益率（对标 ARAMKit「阶段数据」） |
| `trios` | trioKey, augmentIds[3], augmentNames, augmentIconUrls, winRate, pickRate, **deltaWinRate**, **heroWinRate**（基准）, winRateTier, pickRateTier, games/wins | **海克斯三件套组合榜**（比 ARAMKit 的两两组合更贴近实战：一局正好选 3-4 个） |
| `weakAgainst` / `strongAgainst` | opponentName/ChampionId/ImageUrl/Href, games/wins, **observedWinRate**, **expectedWinRate**, **adjustedWinRate**, **counterDelta**, **targetWinRateDrop**, **confidenceLow/High**, **qValue**, **evidence:"supported"**, targetBaselineWinRate, counterBaselineWinRate | **海斗对位克制**（我们现在海斗完全没有），且自带置信区间与多重比较校正 q 值 |
| `teammateSynergies` | teammateName/ChampionId/ImageUrl, games/wins, **synergyDelta**, observedWinRate, expectedWinRate, adjustedWinRate, **pValue/qValue**, confidenceLow/High, firstBaselineWinRate, secondBaselineWinRate | **海斗队友协同**（现在只有 Arena 有 synergy） |
| `terminalItemTrios` | itemIds[3], itemNames, itemImageUrls, winRate, pickRate, deltaWinRate, heroWinRate, wilsonLowerWinRate, tier, games/wins | **成装三件套胜率**（现在的「装备路线」只有顺序，没有组合胜率） |
| `summonerSpellPairs` | spellIds[2], spellNames, winRate, **pickRate**, **deltaWinRate**, wilsonLowerWinRate, tier | **召唤师技能组合胜率**（现在只有选用率，没有胜率） |

### 3.2 `/api/hexdata/postmatch` — 一次请求拿全 173 英雄的 22 项表现指标

```
kda, avgKills, avgDeaths, avgAssists, killParticipation, multiKillRate,
doubleKills, tripleKills, quadraKills, pentaKills,
avgDamage, damageShare, avgTurretDmg, avgDamageTaken, avgDmgMitigated,
avgHealShield, avgCcTime, survivability, utilityScore,
avgGold, goldEfficiency, avgCs
```

这一项**直接补齐 ARAMKit「表现」板块的全部 4 组指标**（战斗 KDA / 伤害输出 / 生存辅助 / 经济节奏），而且是**国服数据**，89 KB 一次拿全，可按 `buildId` 缓存整版本。

### 3.3 `/api/hexdata/hextech-insights` — 推荐页的轻量弹药

- `heroes[173]`：id, name, tier, games, winRate, pickRate, **winRateChange**, confidence, detailUrl, **topItems[5]**（含 confidence）, **topAugments[5]**（含 `rarity`、**`reason`（如"同英雄收益更高"）**、`pairWinRate`、`deltaWinRate`、confidence、detailUrl）
- `augments[211]`：winRate, pickRate, rarity, tier, games, **avgDeltaWinRate**, **coverageHeroCount**, confidence
- `dataContract`：`rawSourceHash` / `corePayloadHash` / `schemaVersion` —— **可用于校验缓存是否同一份数据**，天然契合我们的数据准确性红线

372 KB 比 `/heroes/{id}` 的 1.2 MB 轻得多，且带 `reason` 文案，适合做推荐页的「为什么推荐它」。

---

## 4. 方案（P0 / P1 / P2）

### P0：把已到手的字段渲染出来（成本低、风险低、见效快）

> 原则：优先「后端已经拿到但前端没展示」和「同一上游多一个接口」，不引入新域名。

| # | 新增维度 | 数据来源 | 前端落点 | 说明 |
|---|---|---|---|---|
| P0-1 | **收益率 deltaWinRate**（相对英雄基准 +X%） | `/heroes/{id}` 各数组均已带 | 海克斯卡 `champions.js:1613-1641`、装备排行 `:1021-1029`；推荐页 `gameplay.js:5169-5218`、`:5712-5721` | 现在只有裸胜率。亚索「秘术冲拳 63.7%」看着高，其实是英雄基准本来就 56.7%；**收益率才是真正的决策依据**。参考站全部以此为核心排序键 |
| P0-2 | **样本置信度可视化**（低/中/高样本分档 + Wilson 下界） | 后端已有 `methodology.samplePolicy`（`hexdata.go:90-110`）+ 各数组 `wilsonLowerWinRate` | 复用 `renderMeasurementTechnique`（`champions.js:1643`）扩展；行内加低样本标记 | 对齐 ARAMKit「低样本过滤」与 Hexdata「样本少的高胜率会被压下来」。**当前前端 0 处展示置信度**，这是数据准确性红线上最该补的一块 |
| P0-3 | **推荐页补统计口径说明** | `gameplay.go:5378` 已返回 `measurementTechnique`/`citation` | `renderRecommendationArea` 的 `content.build` 拼接处（`gameplay.js:5117-5119`） | 后端给了前端没渲染，纯遗漏。同时修 `:5689` 海斗局「应用装备方案」按钮被置空的问题（确认是有意还是 bug） |
| P0-4 | **召唤师技能组合带胜率** | `summonerSpellPairs` | `champions.js:1031-1038`（把 `showWinRate=false` 改掉，`renderOptionPickStats :1964`） | 现在出门装/鞋子/召唤师技能**只有选用率**，一个胜率数字都没有 |
| P0-5 | **海克斯官方档位标签**（夯/顶级/…，含配色） | `hexTier` / `hexLabel` / `hexTierColor` | 替换/并列现有本地 grade（`champions.go:271-301` `applyLocalAugmentGrades`） | 现在的 S/A/B 是我们**本地按 score 分位自算**的，和上游口径不一致。有官方标签就该用官方的，本地的降级为补充 |
| P0-6 | **海克斯阶段（stage 1-4）× 英雄收益率** | `augments[].stages[]` | 海克斯推荐区加阶段筛选 chips（可抄 `renderArenaSortBar`，`champions.js:1152-1186`） | 同一海克斯在阶段 1 和阶段 2 的收益可以差很多（亚索的秘术冲拳：阶段1 +13.8%，阶段2 +5.2%）。**这是三选一最实用的维度** |

**P0 成本估算**：后端新增 1 个 Hexdata 端点解析 + 结构体扩展（`champions.go:397-427`、`hexdata.go:449-451` 白名单、`champion_cache.go:472-514` TTL、`:337-350` 落盘 key 白名单），前端 6 处局部改动。不新增域名、不新增上游依赖方。

### P1：对局推荐页的实时化（我们的独门差异化）

> 参考站都是静态查询，**只有我们知道「这一局现在是什么状态」**。P1 全部围绕这个信息差。

| # | 新增维度 | 数据来源 | 落点 | 说明 |
|---|---|---|---|---|
| P1-1 | **海克斯三选一实时排序** | `trios`（三件套组合）+ `augments[].stages[]`（当前阶段收益率） | 新卡片插在 `gameplay.js:5117-5119`，`augmentContent` 与 `renderBuildRecommendation` 之间 | 已知「我已选的 2 个海克斯 + 当前阶段 + 系统给的 3 个候选」→ 用 `trios` 匹配前缀算出**每个候选与已有海克斯的组合胜率**，再叠加该阶段 `deltaWinRate`，给出带理由的排序。**这是四个参考站都做不到的**（它们不知道你已经选了什么） |
| P1-2 | **阵容协同分** | `teammateSynergies`（synergyDelta + CI） | 详情 tab `.live-team` 头部（`gameplay.js:5288-5315`） | 我方 5 人两两取 synergyDelta，聚合成「阵容协同 +X%」，并列出最强/最弱的一对。可直接复用 Arena 已有的 `renderArenaSynergySection`（`champions.js:1309`）交互形态 |
| P1-3 | **克制风险提示** | `weakAgainst` / `strongAgainst`（counterDelta + confidence + evidence） | 英雄概要卡 `.champion-matchups`（海斗当前不出，因 `HasCounters=false`，`champions_structured.go:165-172`） | 局中实时算：对面 5 人里有几个克我、累计压多少胜率；我方 5 人反过来克对面多少。**注意必须透传 `evidence`/`confidenceLow-High`，证据不足要明确降级**（项目红线） |
| P1-4 | **下一步出装建议** | `terminalItemTrios`（itemIds[3]）+ 局内已出装备 | `renderBuildRecommendation`（`gameplay.js:5650-5710`） | 已知我出了哪 2 件 → 匹配 trio 前缀 → 推荐第 3 件并给出该组合的胜率/样本。比现在静态「核心装路线」实用得多 |
| P1-5 | **队伍画像缺口提示** | `postmatch`（avgDamageTaken / avgCcTime / avgDmgMitigated / avgHealShield / damageShare） | 详情 tab 我方队伍头部 | 用 5 人的表现指标画像算出「这队缺前排 / 缺控制 / 缺持续输出」，给出补位建议。虎牙站的「流派」思路（承伤与开团 / 治疗与增益 / 暴击与攻速）可以借来做标签体系 |
| P1-6 | **22 项表现指标面板（英雄详情页）** | `postmatch`（89 KB 全量，按 buildId 缓存） | 新 section，插在 `champions.js:1018` 的拼接序列中；形态抄 `.arena-overview-secondary`（`:1152-1186`） | 分 4 组呈现（战斗 / 伤害 / 生存辅助 / 经济节奏），**每项都要带「较全英雄平均 ±%」**，否则绝对值没有意义（ARAMKit 就是这么做的） |

**P1 成本估算**：P1-1/P1-4 依赖 `/heroes/{id}` 的 1.2 MB 响应，需在解析层裁剪（`trios`/`terminalItemTrios` 只留 top-N，按 pickRate 或 deltaWinRate），否则内存与磁盘预算（`backend/binary_disk_budget.go`）会吃紧。P1-2/P1-3 需要对局页触发时按「我方/对方各英雄」并发拉 `/heroes/{id}`，受 Hexdata 限速与熔断约束（`hexdata.go:37-39,77,655-684`）—— **必须走单飞合并 + 显式退避，且失败时整块隐藏而不是留空壳**（`champions.js` 现有约定：「读取失败时整块隐藏」「用户要求：拿不到就别展示」）。

### P2：自建国服海斗统计（长期护城河，需先过隐私评审）

| # | 新增维度 | 做法 | 说明 |
|---|---|---|---|
| P2-1 | **「我的海斗表现」个人维度** | 放开 `season_stats.go:169-171` 的队列过滤，允许 2300/2400/3270 入库，新增 augment 维度字段 | **零新增网络请求**（SGP SUMMARY 已经返回了全部所需字段）。可产出：我的海斗胜率、常用英雄、我拿某海克斯的胜率、我的经济占比 vs 胜率、我的平均伤害/承伤相对全服分位 |
| P2-2 | 经济占比 → 胜率弹性（对标 Hexdata C 位榜） | 基于 P2-1 的个人样本自算 | Hexdata 的 C 位榜是全服口径；我们能给**个人口径**，「你自己每多拿 1% 经济时胜率怎么变」比全服数字更有说服力。样本不足必须明确降级 |
| P2-3 | 短局/长局分段胜率 | 同上，按 `gameDuration` 分桶 | 对标 ARAMKit 短局/长局英雄榜 |
| P2-4 | 海克斯稀有度概率预测器 + **英雄专属品质概率** | 现有 `/augment-rarity` 阶段分布（`hexdataRarityStage`，`hexdata.go:235-239`）+ `/heroes/{id}` 的 `augments[].stages[]`（带 rarity 与 pickRate）本地聚合 | ARAMKit 的「下一阶段概率转移矩阵」「四阶段组合概率」是纯计算。**RESG 的 `atp`（按英雄区分的各阶段棱彩/黄金/白银概率）也能自算**：把该英雄某阶段所有海克斯的 pickRate 按 rarity 求和即可 —— Hexdata 的稀有度概率是全局口径，我们能做到英雄口径，这是比参考站更强的一档。落点：海克斯图鉴面板（`champions.js:1123` `renderMayhemRarityPanel`） |
| P2-5 | 负向推荐（慎选/陷阱） | 自算：`items.withoutItemWinRate` 反例 + `augments` 低 deltaWinRate 高 pickRate 项 | 借虎牙站的 `trapAugments` 思路，但**必须有统计依据才给**，不能编辑拍脑袋。「选的人多但收益为负」是可靠信号 |
| **P2-6** | **海克斯 → 阶段 → 出装联动**（RESG `ac` 的等价物，**推荐页价值最高的一项**） | 自建：SGP SUMMARY 同时返回 `playerAugment1..6` 与 `item0-6`（`sgp_api.go:893-897`），按「海克斯 × 阶段 × 成装」聚合即可 | **Hexdata 的 5 个接口都给不了这个维度**（它的 `items[]` 是无条件的），只能自算。产出形态：局中已知你选了哪些海克斯 + 当前阶段 → 直接推「选了这个之后胜率最高的装备组合」，每条带场次与胜率。这是 P2 里唯一能做出「参考站没有、只有我们有」的维度，优先级建议高于 P2-2/P2-3 |
| P2-7 | 跨版本趋势曲线 | 按 `buildId` 留存每次取到的 Hexdata 快照（本地 JSON），攒够 2-3 个版本后用 Chart 或纯 SVG 折线呈现 wr/pickRate/场次 | RESG 的做法（`trend.js` + `points[]`）。**纯本地积累，无额外网络请求**；Hexdata 明确不做跨来源趋势，所以我们自己存历史反而是差异化。落点：英雄详情页概览条下方 |

**P2 红线（必须先确认）**：
1. `season_stats.go` 目前只统计**当前登录账号本人**；新增海斗入库会写入更多对局级数据到 `season-stats/{source}/{accountHash}-{season}.json`（schema v7）。**磁盘预算、schema 版本号、隐私声明文案三者必须同步改**，声明要和实际写操作一致（项目红线）。
2. **不得**恢复 ARAMKit `/rating` 中被主动丢弃的 `participants[]`（`aramkit_rating.go:353-359`）用于建库 —— 那是第三方估算的他人身份数据，出口匿名化是刻意设计。
3. 扫描上限（前台 2 页 / 后台 12 页，`season_stats.go:23-28`）需要重新评估，避免海斗高频对局把配额吃光。

---

## 5. 明确不做 / 暂缓

| 项 | 理由 |
|---|---|
| 抓取 ARAMKit 英雄/海克斯页面 | `api.aramkit.com` 只公开 `/rating`，其余数据是 Nuxt SSR 内嵌（实测 `/champions`、`/augments` 均 404）。要拿只能爬 HTML，**脆弱 + ToS 风险 + 与我们已接入的 Hexdata 高度重复**。只把 ARAMKit 当维度清单的对标参考，不当数据源 |
| 红蓝方 / 地图维度（ARAMKit 地图环境页） | Hexdata 的 5 个接口里**没有**阵营与地图字段；要做得自建（P2 路线，SGP SUMMARY 有 teamId 但 mapId 单一）。收益不确定，暂缓 |
| 平衡参数（伤害加成/承伤修正） | ARAMKit 与 RESG（`bb` 字段）都有，证明可获取。但**实测 CommunityDragon 常规 JSON 端点没有**：`champions/157.json`（200，无 aram/balance 键）、`champion-summary.json`（200，无）、`game/data/characters/yasuo/yasuo.bin.json`（200，无）、`champions-summary.json` 与 `aram-champion-balances.json`（404）。需要一次独立可行性探测（候选：游戏内 bin 文件、LCU 端点、版本公告解析），**拆成独立小工单，与本次维度扩充解耦，探测不通过就明确放弃** |
| 虎牙站静态资料库 | 无样本量、无置信度，数据严谨性不达标；只借「流派化 / 情境化 / 负向推荐」的**产品思路**，不接数据 |
| 直接接 RESG（bilibilitoy）数据 | 无明确引用许可条款（页脚只有 Riot 商标 + ICP 备案），且版本目录带构建号 `19226257645568-v15741` 会随发版变化，路径不稳定。**只作维度参考** |
| 批量爬取 Hexdata 建离线数据集 | `/api/hexdata/heroes`（列表）明确返回 403「此入口不提供批量数据。请停止未经许可的自动化访问」。见第 6 节 |

---

## 6. 合规与降级红线（必须写进工单）

1. **按需单资源请求，禁止批量遍历。** Hexdata 已对列表接口做了 403 拦截并明确警告。我们的实现必须保持「用户打开某个英雄详情 → 请求该英雄」的形态，不得为 173 个英雄预拉取。
2. **`postmatch` / `hextech-insights` 是全量聚合文件（89 KB / 372 KB）**，性质上比单英雄请求更接近「转载数据集」。Hexdata 方法论页写明：*「生成的统计汇总与页面结论个人查询、引用都可以；引用时注明 Hexdata 并链接到对应页面就行……成批转载数据集或者拿去商用，需要另外取得许可。」*
   → **已决策（见第 8 节）：全部直接接，不额外走授权流程。** 因此以下对冲措施从「建议」升级为**硬性要求**：(a) 严格按 `buildId` 缓存、每版本只取一次，不做二次分发；(b) 所有新维度沿用现有 `citation` + `measurementTechnique` + `detailUrl` 展示与外链；(c) 保持按需单资源请求，**不得**为 173 个英雄批量遍历（列表接口已被上游 403 拦截并明确警告）；(d) 面板标注数据来源与 `reportDate`，不与 OP.GG（韩服）口径混排。
3. **`Referer` 头是硬要求**：`/heroes/{id}` 不带 Referer 返回 403。新增请求要在 `hexdata.go` 客户端统一加，并纳入 `allowedChampionHost` 白名单校验（`champions.go:679-681`）与路径白名单（`hexdata.go:449-451`）。
4. **1.2 MB 响应的资源治理**：解析层裁剪（`trios` ~1000 条、`terminalItemTrios` ~1000 条只留 top-N）、缓存 TTL 与落盘 key 白名单（`champion_cache.go:337-350,472-514`）、磁盘预算（`binary_disk_budget.go`）三处都要过一遍。
5. **降级规则统一**：任何新维度取不到就**整块隐藏**，不留空态占位（沿用 `champions.js` 现有约定），并在推荐页 `renderRecommendationDataNotices`（`gameplay.js:5050-5056`）给出降级提示。`evidence != "supported"` 或 `games < hexdataMinimumSample(250)` 的行必须打低样本标记，不得参与排序前列。
6. **口径不混算**：Hexdata 是国服 queue 2400 的 7 大区快照；OP.GG 是韩服；ARAMKit 未公开分区。**同一面板内不得混排不同来源的胜率**（Hexdata 自己也强调「不同来源的数据不会混在一起算版本变化」）。新增维度需明确标注来源与 `reportDate`。

---

## 7. 建议的工单拆分

| 工单 | 内容 | 依赖 | 预估 |
|---|---|---|---|
| **R116-A** | Hexdata JSON 客户端扩展：`/heroes/{id}`、`/postmatch`、`/hextech-insights` 解析 + 白名单 + 缓存策略 + 熔断/限速 + 单测（含 403/无 Referer/超大响应裁剪） | 第 6.2 条授权确认 | 中 |
| **R116-B** | P0-1 ~ P0-6：英雄详情页与推荐页的收益率、样本置信度、统计口径、召唤师技能胜率、官方档位、阶段筛选 | R116-A | 中 |
| **R116-C** | P1-6 + P1-1：表现指标面板 + 海克斯三选一实时排序 | R116-A/B | 中大 |
| **R116-D** | P1-2 ~ P1-5：阵容协同分、克制风险提示、下一步出装、队伍画像缺口 | R116-A/B | 大 |
| **R116-E** | P2-1 ~ P2-3：自建海斗个人统计（含 schema 升版、隐私声明同步、磁盘预算复核） | 隐私评审 | 大 |
| **R116-F** | P2-4 概率预测器 + P2-5 负向推荐 + 平衡参数抓取（可独立） | — | 小中 |

每张工单都要按项目约定产出 `docs/r116x-execution-ledger.md` 与 `docs/r116x-validation/`，改动后重新构建并确认版本号。

---

## 8. 决策记录（2026-09-20 用户确认）

| # | 决策项 | 结论 |
|---|---|---|
| 1 | Hexdata 授权路径 | **全部直接接**，含 `/postmatch`、`/hextech-insights` 两个全量聚合接口，不再单独走授权流程。第 6.2 条的对冲措施（来源标注 + 外链 + buildId 缓存 + 不二次分发 + 禁止批量遍历）由「建议」升级为**硬性要求** |
| 2 | 第一批范围（R116-A / R116-B） | **P0 全 6 项**：① 收益率 deltaWinRate ② 样本置信度 / Wilson 下界 ③ 推荐页统计口径说明 ④ 召唤师技能组合胜率 ⑤ 海克斯官方档位标签 ⑥ 海克斯阶段（stage 1-4）筛选。P1-6 表现指标面板顺延至 R116-C |
| 3 | P2 自建统计 | **纳入本轮规划**：放开 `season_stats.go:169-171` 的队列过滤，允许 2300 / 2400 / 3270 入库（仍只统计当前登录账号本人）。**必须同步四项**：schema 版本号升级、磁盘预算复核（`binary_disk_budget.go`）、隐私声明文案与实际写操作保持一致、扫描上限（前台 2 页 / 后台 12 页）重新评估 |

**建议执行顺序**：R116-A（后端接口扩展 + 缓存 + 熔断 + 单测）→ R116-B（P0 六项前端）→ R116-E（自建海斗统计，含 schema 与隐私声明同步）→ R116-C（表现指标面板 + 三选一实时排序）→ R116-D（协同分 / 克制提示 / 下一步出装 / 队伍画像）→ R116-F（概率预测器 / 负向推荐 / 平衡参数）。

**决策后补记（RESG 调研结论）**：上述 3 项决策是在误判 resg.top「非数据站」的前提下做的，补做 RESG 调研后结论如下 —— 决策 1、2 **不变**；决策 3（P2 自建统计）**价值上调**，因为 RESG 的 `ac`（海克斯 → 阶段 → 出装联动）是对局推荐页最有价值的形态，而**Hexdata 的 5 个接口都给不了这个维度，只能自算**，已登记为 P2-6 并建议其优先级高于 P2-2 / P2-3。R116-E 的工单范围需相应纳入 P2-6。

**仍待确认 1 点**：`gameplay.js:5689` —— 海斗局 `hasAugments=true` 时「应用装备方案」按钮被置空，是有意设计还是遗漏？R116-B 动推荐页前需要这个答案，否则容易踩「超范围改动」红线。

---

## 附录 A：参考站可达性与形态（2026-09-20 实测）

| 站点 | 结果 | 形态 |
|---|---|---|
| `resg.top` | 302 → `bilibili.com/toy/resg/index.html` → iframe `bilibilitoy.com/toy/resg/19226257645568-v15741/` | **是海斗数据站**。Vue 3 + Chart.js 纯静态，无后端 API，数据为按版本预生成的 ESM `.js`（`api/v1/versions/{ver}/…`）。自采 Riot API + CommunityDragon，保留 3 个版本历史（16.16-16.18） |
| `hexdata.com.cn` | 200 | SSR + Vite，JSON API 6 个（5 个可用） |
| `aramkit.com/zh-CN` | 200 | Nuxt SSR，API 仅 `/rating` 公开 |
| `hy.fan/CQMnyIf` | 301 → `hd.huya.com/gametool/lolhaidou/haidou/index.html` | React SPA + 内嵌静态资料库（`index-CddVFiMF.js`，157 KB） |

## 附录 B：Hexdata 数据版本快照（本次调研取值）

```
buildId       hexdata-2026-09-14-4984af50e055
reportPatch   16.18        reportDate  2026-09-14
heroCount     173          augmentCount 211
sampleScope   queueId 2400, 7 大区（CQ100/GZ100/HN1/HN10/NJ100/TJ100/TJ101）
              eligibleMatches 9,055,202   participantRows 90,552,020   snapshotMatches 9,429,943
              windowUtc 2026-09-10 06:55 ~ 2026-09-14 04:32
tierBands     T1 前15% / T2 15-35% / T3 35-65% / T4 65-85% / T5 后15%   tierMethod data_rank_v2
过滤口径      只统计正常结束且超过 4 分钟的比赛
```

## 附录 C：RESG 实测数据样本（2026-09-20 取值）

```
版本表        16.18（collectedAt 2026-09-17T18:45:14Z）/ 16.17 / 16.16
亚索(157)     tm 584,078   wm 332,763   wr 0.5697   tier T0   roles [fighter, assassin]
bb            {"攻击速度增长": "+2.5%"}
b.SPELLS      25 组，rank1 = 闪现+标记  tm 540,339  wr 0.5710  pr 0.9250
b.BOOTS        8 项，rank1 = 狂战士胫甲  tm 290,328  wr 0.5787  pr 0.6692
b.SKILL_ORDER  6 组，rank1 = Q→E        tm 430,421  wr 0.5665  pr 0.7394
si            10 组出门装，rank1 = 短剑+狂战士胫甲  tm 59,198  wr 0.5726  pr 0.2996
ac            stage1 113 条 / stage2 887 条；每条带 a[]（海克斯）+ dl + b[]（items+tm+wr）
ra            126 条；首条 灵魂虹吸 wr 0.5977 pr 0.0611 dl +0.0280 cl 0.5777 ch 0.6177 cf "高"
ia            rank1 = 123430  tm 380,125  wr 0.6102  pr 0.6508
ia.coverage   summarySamples 584,078 / detailSamples 0 / detailCoverage 0
              generatedAt 2026-09-17T19:44:29Z / sourceSnapshotSha256 ""（空值如实留空）
atp           stage1 棱彩 0.4696 黄金 0.4494 白银 0.0809
              stage2 棱彩 0.2774 黄金 0.4363 白银 0.2863
              stage3 棱彩 0.2716 黄金 0.4389 白银 0.2895
              stage4 棱彩 0.2733 黄金 0.4366 白银 0.2900
trend         16.15 wr 0.5673 pr 0.1027 m 225,694 → 16.18 wr 0.5697 pr 0.1109 m 584,078
秘术冲拳(1058) tm 1,490,308  wm 763,922  wr 0.5126  tier T3  quality 1  champions[161]
              其中亚索：tm 104,813  wr 0.6415  dl +0.0718  pr 0.0703
```
