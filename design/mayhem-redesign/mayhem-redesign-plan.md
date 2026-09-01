# 海克斯大乱斗改版 · 设计稿与实施方案

产物：

- `design/mayhem-redesign/mayhem-redesign-mockup.html` —— 可点开的四屏原型（英雄榜 / 英雄详情 / 海克斯图鉴 / 对局内推荐），带「字段来源标注」开关和深浅色切换。
- 本文件 —— 实施方案，所有结论带 `file:line` 证据。

原型里的每一个数字都不是编的：2026-08-21 从 resg.top、OP.GG、CommunityDragon 三条链路实抓，脚本生成 175KB 数据包内联进 HTML。校验方式见文末「验收记录」。

按 [[feedback-analysis-only-mode]]，本轮我只出设计稿与方案，不改仓库源码。

---

## 一、先纠正一条我自己写错的结论

`memory/deep-legends-opgg-mode-api.md` 里我写过「海克斯大乱斗（KIWI）在 OP.GG 上没有对应模式，只能降级到 `aram`」。**这句话的适用范围被我写大了。**

准确的说法是：`lol-api-champion.op.gg/api/{REGION}/champions/{mode}/{position}` 这条路径的 `mode` 枚举只有五个（`ranked` / `aram` / `arena` / `urf` / `nexus_blitz`），传 `aram_mayhem` 返回 422。但 OP.GG 另有两条完全原生的海克斯大乱斗数据面，而且**仓库里已经在用其中之一**：

- `lol-api-champion.op.gg/api/contents/tiers?type=aram_mayhem` —— 原生梯度榜，`champions.go:1417` 已经在打这个接口。
- `op.gg/{locale}/lol/modes/aram-mayhem[/{英雄}/{页签}]` 的 Next.js RSC 载荷 —— 原生逐英雄海克斯、召唤师技能、加点、出装，`champions.go:1405` 已经在抓这个页面的模式级列表。

所以用户的判断是对的：OP.GG 原生支持这个模式，它没有把海克斯大乱斗当普通极地大乱斗混合统计。**真正把两者混在一起的是我们自己**，见下一节。

---

## 二、现状体检

### P0 · 详情页的符文和出装是普通极地大乱斗的数据

`champions_structured.go:137`：

```go
"hextech-aram": {APIMode: "aram", Region: "KR", PositionMode: opggPositionLiteralNone, HasRunes: true, HasAugments: true},
```

`APIMode: "aram"` 意味着走 `/api/KR/champions/aram/{id}/none`，拿回来的是**极地大乱斗**的符文页、召唤师技能、出装。用户点开「海克斯大乱斗 → 金克丝」，看到的构建其实来自另一个模式。

`HasRunes: true` 更糟：**海克斯大乱斗根本没有符文**。我把 OP.GG 构建页的 HTML 和 RSC 载荷都翻了一遍，只有 Build / Augments / Skills / Items 四块，`perk` 字样零命中；resg 同样不提供。我们不但显示了一个不存在的东西，还用别的模式的数据把它填满了。

`champions_structured.go:453` 的 `if spec.HasRunes` 是这条分支的入口。

【未实现】原生详情链路。

### P0 · 英雄榜没有胜率，也没有场次

`champions.go:1439`：

```go
rows = append(rows, championRankingRow{ChampionID: id, Key: item.Key, Name: item.Name, Rank: item.Rank, Tier: item.Tier})
```

`contents/tiers?type=aram_mayhem` 这个接口**本身就不给胜率和选取率**，只给 `rank` 和 `tier`。所以不是解析漏了，是上游没有。这正好是 resg 的价值所在——它给真实胜率、场次、置信区间。

顺带一个隐患：`championRankingRow.Tier`（`champions.go:141`）是 `int` 且没有 `omitempty`，上游不给 tier 时零值 0 会被 `web/champions.js:84 tierBadge()` 渲染成金色 OP 徽章。这条在斗魂分支上已经被记过（[[deep-legends-arena-redesign]]），海克斯分支同样没有兜底。

【部分实现】只有梯度。

### P1 · 详情页的海克斯推荐没有任何指标

`web/champions.js:868 renderRecommendedAugments()` 渲染出来的是「首选 / 次选 / 备选」加一串「推荐」，**一个数字都没有**。数据来自 `champions.go:2205 parseRecommendedAugments()`，从 HTML DOM 里刮 `championAsset`（只有 id / name / 图标），结构上就承载不了胜率。

对比同一个文件里的斗魂实现 `web/champions.js:878 renderArenaAugments()`，人家有选用率和胜率。海克斯大乱斗这边是全项目最弱的一块。

【部分实现】只有排序。

### P1 · 对局内把「表现指数」当成率来显示

`web/gameplay.js:2136`：

```js
: `表现 ${rate(row.championStats.performance)} · 热度 ${rate(row.championStats.popularity)}`;
```

`rate()`（`web/gameplay.js:2436`）会给数字直接加个百分号。OP.GG 的 `performance` 是**以 100 为基准的相对指数**（低样本时封顶 170），不是百分比。金克丝的「升级：无尽之刃」performance = 101.49，界面上会显示成 **「表现 101.5%」**。这个数看起来像胜率，但它不是——真实胜率是 56.28%。

`popular` 倒是能当百分比看（同一英雄下各海克斯的 `popular` 加起来约等于 100），但它是**选取份额**不是热度，文案也不对。

【未实现】口径正确的指标。

### P1 · 对局内会给这个模式显示符文页签

`web/gameplay.js:2012`：

```js
hasRunes: typeof payload.hasRunes === "boolean" ? payload.hasRunes : augmentSource !== "arena",
```

默认值只把斗魂排除了。海克斯大乱斗（`liveAugmentRecommendationSource()` 返回 `"hextech"`，`web/gameplay.js:1838`）会走进 `recommendationTabSpecs()`（`web/gameplay.js:2018`）的 `runes` 分支，显示一个内容来自极地大乱斗的符文页。

排队识别本身是对的，不用动：`gameplay.go:1979` 认 queueId 2300/2400，`gameplay.go:1993` 认 `gameMode == "KIWI" && mapID == 12`。

【未实现】能力位。

### 小结

| 面 | 现状 | 缺什么 |
| --- | --- | --- |
| 英雄榜 | 梯度 + 名次 | 胜率、场次、档位可信度 |
| 英雄详情 · 海克斯 | 名字 + 图标 + 名次 | 胜率、选取率、样本、置信区间、组合 |
| 英雄详情 · 构建 | **普通极地大乱斗数据** | 原生模式数据 |
| 英雄详情 · 符文 | 显示了不存在的东西 | 删掉 |
| 海克斯图鉴 | 有（`champions.js:554 renderARAM()` 的右栏） | 逐英雄反查 |
| 对局内 | 指数当率显示 + 假符文页 | 口径修正 + 能力位 |

---

## 三、数据链路基线（2026-08-21 实测）

### 3.1 resg.top —— 主数据源

**它的数据从哪来。** 站点页脚写明 `Game data from CommunityDragon and Riot Games API`，备案号 `沪ICP备2025129665号-1`，作者是 YouTube 的「LPL駐韓研究員 researcher of korea」，页面挂微信/支付宝捐赠码。所以 **resg 自己是走 Riot 官方 Match-V5 接口**采集、落成静态快照，静态资源（英雄/海克斯/装备图标）用 CommunityDragon。

大区它没写死在任何字段里。两条旁证指向 Riot 运营服、大概率韩服：国服没有公开的 Riot API；作者常驻韩国；16.16 快照的采集时间与 OP.GG 韩服当前版本同步（国服通常落后一到两个补丁）。**这是强推断不是事实，UI 上只标「来源 resg.top · 16.16 · 2026-08-20 采集」，不要替它认领大区。**

样本量：16.16 快照的英雄出场合计 25,126,058 人次，除以每局 10 人 ≈ **251 万场对局**（16.15 是 220 万场）；海克斯选取合计 9230 万次。这个量级用 Riot 生产级 key 做得到。

**我们这边则是消费它的静态数据文件。** `/api/v1/**` 没有任何公开文档，也就是说——是的，等于爬它的站。缓解措施见 5.3。

**URL 形状**（base = `https://resg.top/api/v1`，注意不是 `/v/16.16`，那是前端路由）：

| 端点 | 内容 | 实测规模 |
| --- | --- | --- |
| `/versions.js` | 版本索引 | 只有 16.15 / 16.16 两个快照 |
| `/versions/16.16/champions.js` | 英雄榜 | 172 个 |
| `/versions/16.16/champions/{championId}.js` | 英雄详情 | 金克丝 144 条海克斯 / 格雷福斯 155 条 |
| `/versions/16.16/augments.js` | 海克斯图鉴 | 208 条 |
| `/versions/16.16/augments/{augmentId}.js` | 逐海克斯反查 | 171 个英雄逐个给数 |

**传输特征**：`Content-Type` 是 JS 模块不是 JSON（`export default {...}`，要剥 `export default ` 前缀）；`Cache-Control: public, max-age=31536000, immutable`；**无 CORS 头**（浏览器直连必然失败，只能后端代理）；无限流头；无 robots.txt（SPA fallback）。

**英雄详情的字段**：

- `balance` —— `{承受伤害: "105%", 造成伤害: "90%"}`，大乱斗独有的平衡数值。
- `builds.SPELLS` / `builds.BOOTS` / `builds.SKILL_ORDER` —— 每条带 `value[]` / `winRate` / `pickRate` / `totalMatches`。
- `startingItems` / `itemAnalysis.items` —— 同结构。
- `recommendedAugments[]` —— 每条 `{id, quality, winRate, pickRate, totalMatches, confidenceLow, confidenceHigh, confidence, adjustedWinRate, delta}`。`confidence` 是中文的 高/中/低；`delta` = 本海克斯胜率 − 本英雄胜率（实测：金克丝 1356 号 winRate 0.5628，delta 0.0153，0.5628 − 0.0153 = 0.5475 = 金克丝本体胜率，对得上）。
- `augmentCombos` —— 按件数 1/2/3/4 分组，每条 `{augments[], winRate, totalMatches, builds[]}`，`builds[]` 是这套组合最常打出来的有序出装。**这是 OP.GG 完全没有的东西。**
- `avoid` —— 负向清单。

**两条 resg 自身的数据瑕疵**（实现时要兜底，别原样透传）：

1. `pickRate` 会给 0 而 `totalMatches` 不为 0 —— 1187「闪光弹」和 1348「闪闪现现」都是 `pickRate: 0` 但 1763 / 1739 场。**上游自相矛盾，UI 要显示「—」不要显示「0.0%」**，后者读起来是「没人拿」，和 1700 场直接打架。
2. `pickRate` 跨四个数量级 —— 逐英雄反查里 819 场的条目四舍五入到一位就是 `0.0%`。原型里用 `pctLo()` 统一处理：`null` 或 `<= 0` 显示「—」，小于 0.05% 显示「<0.1%」，其余正常。这个函数落到仓库里建议放在 `web/champions.js` 的格式化工具区，和 `percent()` 并列。

`icon` 字段是 CommunityDragon 相对路径，补前缀即可直连，不用自己拼（原型里 `cdAsset()` 就是干这个的，实测召唤师技能/装备/技能图标全部 200）。

### 3.2 OP.GG —— 补位与交叉校验

**梯度榜**：`lol-api-champion.op.gg/api/contents/tiers?type=aram_mayhem`，返回 `data[]{championId, key, name, rank, tier}` + `meta.version`。**没有胜率**。仓库已在用（`champions.go:1417`）。

**逐英雄海克斯**：`op.gg/zh-cn/lol/modes/aram-mayhem/{英雄}/augments`，带请求头 `RSC: 1` 时返回 263KB 的 flight 载荷（不带是 793KB HTML），用现成的 `decodeNextFlight()`（`champions.go:1853`）+ `extractBestArray()` 就能解。条目形如 `{id, tier, performance, popular, rarity}`。

**两个坑**：

1. `performance` **不是胜率**，是以 100 为基准的相对指数，低样本封顶 170。和 resg 真实胜率的相关系数 r = 0.939，**当交叉校验很好，当指标显示是错的**。
2. `rarity` 会缺失。逐英雄载荷 193 条里有 25 条只有 `{id, tier, performance, popular}`；模式级 267 条里有 38 条缺。**品质不能依赖 OP.GG，要以 CommunityDragon 为准。**

**模式级海克斯**：267 条，比 resg 的 208 条多 59 条——多出来的是新上线还没攒够样本的。这就是「OP.GG 补位」的具体含义：resg 没有的那些，用 OP.GG 的 tier 顶上并标注「样本不足」。

### 3.3 CommunityDragon —— 元数据唯一真源

`cherry-augments.json`（zh_cn，655 条）同时装着斗魂和海克斯大乱斗两套海克斯，靠资源命名空间区分：**海克斯大乱斗是 `Kiwi`，斗魂是 `Cherry`**。仓库已有 `loadCommunityDragonAugments()`（`champions_structured.go:739` 一带）。

**图标要用 `augmentSmallIconPath` 原样**（小写、去掉 `/lol-game-data/assets` 前缀）。我试过按斗魂的老办法把 `_Small` 换成 `_large`，`tank_engine_large.png` 直接 404。改用小图后随机抽 12 条全部 200，原型里 182 条海克斯图标 URL 全绿。

**品质真源排序：CommunityDragon = OP.GG > resg。** resg 在 2078、2140、2143 三条上和另外两家不一致。

### 3.4 三家的能力矩阵

| | resg | OP.GG | CDragon |
| --- | --- | --- | --- |
| 英雄胜率 / 场次 | ✅ | ❌ | ❌ |
| 英雄梯度 | ✅ 自算 T0–T4 | ✅ 原生 rank/tier | ❌ |
| 海克斯胜率 / 选取率 / 样本 | ✅ | ❌（只有 performance 指数） | ❌ |
| 海克斯置信区间 | ✅ | ❌ | ❌ |
| 海克斯组合 | ✅ 1–4 件 | ❌ | ❌ |
| 逐海克斯的英雄反查 | ✅ 171 英雄 | ⚠️ 有但只有指数 | ❌ |
| 海克斯覆盖条数 | 208 | 267 | 655（含斗魂） |
| 名称 / 说明 / 图标 / 品质 | ⚠️ 品质有 3 条错 | ⚠️ 品质会缺 | ✅ |
| 符文 | ❌ 这个模式没有 | ❌ 同上 | — |

---

## 四、设计

原型四屏，全部按 `web/app.css` 的令牌配色，深浅色都验过。

### 签名元素：置信区间条

这是整套设计里唯一花力气的地方，也是三个竞品站都没有的东西。

海克斯大乱斗的海克斯样本量差了四个数量级——「坦克引擎」有 204 万场，冷门海克斯不到 100 场。只显示一个胜率数字，会让 3000 场的 56.9% 和 200 万场的 51.3% 看起来一样可信。

所以每条海克斯的胜率下面画一条区间条：圆点是胜率，色带是 resg 给的 95% 置信区间，**竖线是这个英雄自身的平均胜率**。圆点落在竖线右边，才说明这件海克斯真的把这个英雄抬起来了；色带越长越别当真。

竖线的选择是有讲究的：拿模式平均当基线没意义（每个英雄的水位不同），拿本英雄胜率当基线，读出来的正好就是 resg 的 `delta`。一个视觉元素同时表达了「绝对水平」「不确定度」「相对提升」三件事。

### 品质三色不绑主题令牌

原型 CSS 里写死：

```css
--rar-silver: #9AA7B8;  --rar-gold: #E3B341;  --rar-prism: #C77DFF/#5AA9FF/#FF8AC7;
```

和 `web/champions.css:534-536`、`:545-547` 现有取值一致。**这是刻意的**——[[deep-legends-round8-worklist]] 记过一次「品质色绑主题令牌串色」，白银/黄金/棱彩必须跨 8 套主题恒定。

### 四屏内容

**屏一 · 英雄榜**：resg 胜率 + 场次 + 自算档位，并排放 OP.GG 韩服名次做对照。两榜口径不同（resg 是纯胜率，OP.GG 是胜率+选用率的综合分），名次相差 25 位以上打「两榜分歧」标记——实测 24 行里出现 4 个，是真实存在的差异不是噪声。

**屏二 · 英雄详情**：海克斯推荐**置顶**，按白银/黄金/棱彩三栏铺开——这不是为了排版好看，游戏里一次给你的三个选项就是按品质档给的，三栏就是玩家的决策界面。排序可切胜率/选取率/相对提升/样本量，默认隐藏 2000 场以下。

往下是海克斯组合，按已选件数 1/2/3/4 分页，每条展开是这套组合最常打出来的出装。再往下才是召唤师技能/鞋子/加点/起始装/核心装备。**没有符文页签。**

顶部 hero 放平衡数值（承受伤害 105% / 造成伤害 90%）作为小芯片——这是大乱斗最有辨识度的一条信息。这个模块用户没点名要，原型里标了「可选模块」，砍掉不影响其他部分。

**屏三 · 海克斯图鉴**：左边全量列表可按品质筛，右边是选中海克斯的**强势英雄反查**——「这件海克斯在谁身上最好用」。这是 resg 独有数据，也是图鉴页存在的理由（否则它只是个静态说明书）。

**屏四 · 对局内推荐**：符文页签置灰。三列对应游戏里一次给的三个品质档。已选海克斯单独一条，然后从组合数据倒推「下一件拿什么和已选成套」，成套的打标并置顶。实测已选 1336 + 1356 时，6 条候选带上了成套胜率。

---

## 五、实施方案

### 5.1 后端

**新建 `resg.go`** —— 与 `yourgg_arena.go` 平级的独立数据源文件。

```
resgHost = "resg.top"                      // 常量，与 opggChampionHost 同层级
loadResgVersions(ctx)                       → 版本索引，取最新
loadResgChampions(ctx, version)             → []resgChampionRow
loadResgChampionDetail(ctx, version, id)    → resgChampionDetail
loadResgAugments(ctx, version)              → []resgAugmentRow
loadResgAugmentDetail(ctx, version, id)     → resgAugmentDetail
```

响应体是 `export default {...}` 的 JS 模块，解析前先剥前缀。`p.fetch()` 的现成签名可以直接用（`champions.go` 里到处在用），Accept 传 `application/javascript`。

**改 `champions_structured.go:137`**：

```go
"hextech-aram": {APIMode: "aram_mayhem", Region: "KR", PositionMode: opggPositionOmitted, HasAugments: true},
```

`HasRunes` 删掉（这个模式没有符文），`APIMode` 换成原生值。注意 `opggDetailRequest()`（`champions_structured.go:166`）走的是 `/api/{region}/champions/{mode}/...` 这条 422 的老路，**这个模式必须换成 RSC 页面链路**，所以更稳的做法是在 `loadStructuredDetail()`（`champions_structured.go:406`）里给 `hextech-aram` 单开一条分支，复用 `loadARAMPage()` 的抓取方式加上 `RSC: 1` 头，而不是硬塞进表驱动。

**改 `champions.go:1416 loadARAMRankings()`**：先打 resg 拿胜率/场次/档位，OP.GG 的 `contents/tiers` 降为交叉列。resg 挂了就退回现在的行为（只有梯度），响应里带上 `Source` 标明实际用了哪条。

`championRankingRow` 需要加 `WinRate` / `TotalMatches` / `ConfidenceLow` / `ConfidenceHigh` 字段（`champions.go:139` 一带），全部 `omitempty`。**顺手把 `Tier`（`champions.go:141`）改成 `*int` 或让无数据时传 -1**，避免零值被渲染成金色 OP 徽章。

**改 `champions.go:1752 loadAugments()`**：现在从 HTML 刮模式级海克斯。改成 resg `augments.js` 为主体（208 条带真实胜率），OP.GG 的 267 条做并集补位，缺的标「样本不足」。品质统一走 CommunityDragon。

**新增 `handleChampionAugmentDetail`** —— 图鉴页反查用，代理 resg 的 `/augments/{id}.js`。路由挂在 `main.go` 现有 champion 路由旁边。

**`FetchedAt` 用真实抓取时间。** `champions.go:1442` 和 `:1811` 都是 `time.Now()`，[[deep-legends-round9-cleanup]] 记过这类硬编码还剩 6 处。resg 的载荷里自带 `collectedAt`（16.16 是 `2026-08-20T04:41:44.106Z`），透传它，别用当前时间——不然 UI 上会显示「刚刚更新」，而数据其实是一天前的快照。

### 5.2 前端

**`web/champions.js`**

- `:554 renderARAM()` —— 英雄榜加胜率列、场次列，OP.GG 名次做对照列，分歧标记。
- `:847` 详情页 metrics —— 现在 `aram-mayhem` 只有「排名 + 梯度」两格，补成「胜率 / 场次 / 梯度 / 韩服名次」。
- **`:868 renderRecommendedAugments()` 整体重写** —— 三品质分栏 + 置信区间条 + 排序切换。可以照 `:648 renderArenaAugmentSection()` / `:674 renderArenaOptionCard()` 的结构改，那两个函数的骨架能复用，主要差别是多一条区间条、指标从「选用率/胜率」换成「胜率/选取率/相对提升/样本」。
- **新增 `renderAugmentCombos()`** —— 组合区块，件数分页。
- **新增图鉴页反查面板** —— 挂在 `:554 renderARAM()` 右栏现有海克斯列表的详情位。
- `:644 arenaRarityKey()` 是斗魂命名，海克斯大乱斗要复用它就改个中性名字（`augmentRarityKey`），别在两处各写一份映射。

**`web/gameplay.js`**

- **`:2012`** —— `hasRunes` 默认值从 `augmentSource !== "arena"` 改成 `!augmentSource`（斗魂和海克斯大乱斗都没有符文，其余模式有）。
- **`:2136`** —— hextech 分支从 `表现 ${rate(performance)} · 热度 ${rate(popular)}` 换成 `胜率 ${percent(winRate)} · 选取 ${percent(pickRate)} · ${compactNumber(totalMatches)} 场`。**注意 `percent()`（`:88`）会 `Math.round`，胜率要小数位就别用它**，加一个 `winRatePercent()` 或直接用 `rate(value * 100)`。OP.GG 的 performance 保留但降级成灰色的「韩服指数 101.5」，文案里不带百分号。
- **`:2073 liveChampionAugmentRows()`** —— 排序键从 `performance` 换成 resg 的 `winRate`，OP.GG 指数留作次级排序键。
- **新增：组合感知。** 拿到本局已选海克斯（`gameplay.js` 里已有对局内海克斯的解析），从 resg 的 `augmentCombos` 倒推「与已选成套」的候选并置顶打标。原型屏四演示了这个交互。
- `:2026 renderRecommendationDataNotices()` —— 加一条 resg 快照时间与来源的提示，替代现在那句「当前模式暂无原生推荐，符文与出装参考极地大乱斗数据」（改完之后这句话对这个模式不再成立）。

**CSS**

- `web/champions.css` —— 新增区间条、三品质分栏、组合卡的样式。品质色沿用 `:534-536` / `:545-547` 的固定值，**不要新造一套，也不要绑主题令牌**。
- `web/gameplay.css` —— `.live-augment-option`（`:729-734`）加一个 `.is-combo` 态和成套徽章。`--augment-accent` 五档（`:402-407`）不用动。

**海克斯说明的富文本渲染。** 上游描述带一整套私有标签：`<br>`、`<font color='#F0C200'>`、`<rules>`、`<flavorText>`，外加 20 多种语义高亮（`<crit>` `<magicDamage>` `<scaleHealth>` …）。全量统计我跑过：208 条描述里 `br` 105 次、`font` 52 次、其余长尾。

原型里的 `gameText()` 是先整体转义、再按白名单还原的两步做法，可以照抄逻辑，**但不能照抄实现**——它返回字符串再进 `innerHTML`，这是 [[deep-legends-tooltip-redesign]] 划的红线。落到仓库里要改成建 DOM 节点。白名单：`br` → 换行，`font` → 只接受 `#RRGGBB` 形式的 color，`rules` / `flavorText` → 次要样式块，其余一律降级成统一的高亮色。

### 5.3 缓存与降级

resg 是单人维护的爱好站，没有 SLA，只保留两个版本的快照。三道闸门：

1. **强缓存。** 上游给了 `max-age=31536000, immutable`，缓存键用 `version + endpoint + id`，命中就永不回源。版本号变了才重新拉，正常情况下一个补丁周期只打它几次。
2. **降级链路。** resg 不可用时退回纯 OP.GG：英雄榜只剩梯度（即现有行为），海克斯推荐退回 OP.GG 指数排序但**指标位显示「—」而不是把指数当胜率**，组合区块和图鉴反查整块隐藏。
3. **开关。** 配置项 `resgEnabled`，默认开，出问题能一键切回。

**UI 必须固定标注来源与采集时间。** 原型每个面板底部都有一条 sourceline，实现时保留。

**隐私。** resg 的载荷是纯聚合统计，不含任何玩家标识——我逐字段查过，没有 `summonerId` / `puuid` / `riotId` 这类字段，不存在 [[deep-legends-arena-redesign]] 里 YOUR.GG 那种剥字段的问题。

### 5.4 护栏

**当前仓库的前端 JS 没有任何测试覆盖**——`grep -rn '"web/' --include="*_test.go"` 零命中，20 个 `*_test.go` 全是 Go 侧逻辑。所以下面这些护栏都要新建，用 `embedded.ReadFile("web/gameplay.js")` 加正则断言。

按 [[deep-legends-mutation-testing]]：**正则型测试一律先变异再判定**，而且变异要 `cp` 真副本，软链接沙箱无效（`__dirname` 会解析成 realpath）。

要加的：

- `hextech-aram` 的 `opggModeSpecs` 条目**不带 `HasRunes`**。变异：把 `HasRunes: true` 加回去，测试必须红。
- `recommendationCapabilities()` 对 `augmentSource === "hextech"` 返回 `hasRunes: false`。
- 对局内 hextech 分支**不出现 `rate(row.championStats.performance)`**。变异：改回原写法，必须红。
- resg 解析：`delta` 与 `winRate − 英雄 winRate` 一致（用真实载荷做 fixture）。
- 品质映射：2078 / 2140 / 2143 三条以 CommunityDragon 为准而不是 resg。
- 描述渲染：白名单外的标签不得出现在输出里，且输出中不得含未转义的 `<`。
- `championRankingRow.Tier` 无数据时不渲染成 OP 徽章。

---

## 六、落地顺序

分四步，每步都能独立验收：

1. **止血**（改动最小，收益最大）—— 关掉海克斯大乱斗的符文页签，修 `rate(performance)` 的口径，修 `Tier` 零值。只动 `champions_structured.go:137`、`web/gameplay.js:2012`、`:2136`、`champions.go:141`。这一步不引入任何新数据源。
2. **接 resg** —— 新建 `resg.go`，英雄榜和详情页的海克斯推荐换成真实胜率，带降级和缓存。
3. **新区块** —— 海克斯组合 + 图鉴反查。
4. **对局内组合感知** —— 已选海克斯 → 成套推荐。

---

## 七、验收记录

原型里的数字全部脚本生成，不是手写的。校验手段：

- **JS 运行时**：jsdom 加载整页，`pageerror` 零；DOM 里 `undefined` / `NaN` 零命中；图片 URL 里 `undefined|null|NaN` 零命中。
- **渲染规模**：英雄榜 24 行 / 详情页 18 张海克斯卡（三品质各 6）/ 组合 5 条含 15 条出装线 / 图鉴 208 条 / 反查 12 行 / 对局内 9 张卡含 6 个成套标记。
- **图标可达性**：229 个不同 URL 按四类（英雄 26、Kiwi 海克斯 83、Cherry 海克斯 99、装备 16、召唤师技能 5）各抽样 HTTP 实测，**全部 200**。
- **富文本**：用页面自己的 `gameText()` 跑完 208 条描述，残留转义标签 **0** 条；`<font color='#F0C200'>` 正确转成带色 span，`<rules>` 正确转成次要块。
- **数据自洽**：金克丝 144 条 / 格雷福斯 155 条海克斯推荐，海克斯 id 在图鉴映射里缺失 0 条；组合 1–4 件各 8 条，组合里引用的海克斯 id 全部可解析。
- **交叉验证**：OP.GG `performance` 与 resg `winRate` 相关系数 r = 0.939；品质在三家之间比对，resg 有 3 条与 CDragon/OP.GG 不一致（2078、2140、2143）。
- **视觉复核**：Chromium 全页截图逐屏看过，`pageerror` 零、`naturalWidth === 0` 的破图零。看图改掉三处：选取率 0.0% 的口径（见 3.1 末尾）、四个构建小面板缺表头、图鉴反查列的极小选取率。

---

## 八、需要一并更新的记忆

`memory/deep-legends-opgg-mode-api.md` 里「海克斯大乱斗在 OP.GG 上没有对应模式，只能降级到 aram」这句要改成「`/api/{region}/champions/{mode}` 这条路径不支持，但 `contents/tiers?type=aram_mayhem` 与模式页 RSC 是原生支持的」。我会在本轮结束时改掉。
