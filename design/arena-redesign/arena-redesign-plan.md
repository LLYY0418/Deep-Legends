# 斗魂竞技场页面改版 · 实现方案

> 配套设计稿：`arena-redesign-mockup.html`（用浏览器打开，右上角「字段来源标注」开关会在每个数值下方显示它对应的上游字段）
>
> 参考站：[YOUR.GG Arena Champion Tier List](https://your.gg/en/kr/arena/champions)
> 实测时间：2026-08-20 · 游戏版本 16.16
>
> 本文档只描述改动，**不含已完成的代码**。执行者请先读一遍「零、先看这里」再动手。

---

## 零、先看这里：三条会影响设计的硬约束

**第一，斗魂竞技场没有段位维度。** OP.GG 的 `?tier=` 参数对 arena 形式上通过校验（只接受 `all/bronze/silver/gold/gold_plus`），但除 `all` 外全部返回 `average_stats: null` + `is_rip: true`，详情页则返回空壳（`core_items: []`、三组 augments 全空）。`diamond_plus` 等直接 422。**所以列表页不要做段位筛选器**，参考站也没有做。

**第二，Riot 官方 API 拿不到任何一个概览指标——不是"没优先用"，是它根本没有这类接口。** 这一条被问过，所以展开说清楚，见下面的能力对照表。

### Riot API vs OP.GG：到底谁能给什么

Riot 的公开 API 只有两类东西：**某个玩家的档案**（account / summoner / league / mastery）和**某一场对局的明细**（match-v5 / timeline）。它**没有任何"英雄级聚合统计"接口**——没有胜率榜、没有梯度、没有物品胜率、没有海克斯胜率，Data Dragon 和 CommunityDragon 也只有静态资源（名称/图标/数值），不含对局统计。这不是配额或权限问题，是接口清单里就没有这个东西。

| 需要的数据 | Riot API | OP.GG | 结论 |
|---|---|---|---|
| 单场对局明细（含斗魂 `placement` / `playerSubteamId` / `playerAugment1..6` / 伤害 / 承伤） | ✅ 已在用 | — | **走 Riot**，项目韩服链路本来就是这么拿的 |
| 我自己用某英雄拿一位的对局 | ✅ 可做（`by-puuid/ids` 拉回来自己按 `queueId===1700` + `placement===1` 筛） | — | **走 Riot / LCU / SGP**，即 ⑥ 的「我的一位」页签 |
| 全服某英雄最近的一位对局（不限玩家） | ❌ 只能按 puuid 查，没有"按英雄检索对局"的索引 | ❌ 不提供对局列表 | 只能靠第三方自建对局库 → YOUR.GG |
| 英雄梯度 / 胜率 / 平均名次 / 一位率 | ❌ 无聚合接口 | ✅ | **只能走 OP.GG** |
| 海克斯 / 棱彩 / 核心物品 / 鞋子 / 加点 的胜率与场次 | ❌ 无聚合接口 | ✅ | **只能走 OP.GG** |
| 海克斯的中文名 / 图标 / 描述 | ❌ | ❌（OP.GG 也不给） | 走 CommunityDragon `cherry-augments.json` |

理论上我们可以自己拉一大堆 match-v5 对局再本地聚合，但那需要几十万场样本才能得出和 OP.GG 同量级的置信度，配额（`riot_api.go:164` 的 `wait()` 限流）和本地存储都撑不住，而且项目里从来没做过这种聚合——`riotProvider` 的全部方法都是单玩家读取，唯一看着像统计的 `loadSpecialistRunes()`（`specialist_runes.go:116`）其实只是从 OP.GG 拿到的专家名单里抽一局符文当样本，不做任何计数。所以**不要为了"优先用 Riot"去做本地聚合**。

一句话总结：**能从 Riot 拿的（对局明细），我们已经在拿；拿不到的（英雄级统计），Riot 压根没有。**

**第三，YOUR.GG 的「一位战绩」是它自建对局库的产物，我们没有同类数据。** 但它的公开 API 无鉴权、可服务端直连，可以代理。代价是：只有韩服一份数据、`participants[]` 字段极简、以及多了一个会随对方发版失效的外部依赖。详见第四节。

---

## 一、页面结构总览

沿用现有 `.aram-workspace` 两栏骨架（左 `minmax(270px,320px)` + 右 `minmax(0,1fr)`），左栏 `position: sticky` 常驻。区块顺序按需求调整为：

```
左栏（sticky）                     右栏（详情）
┌────────────────┐   ┌──────────────────────────────────────────────┐
│ 英雄梯度        │   │ ① 概览条：头像 · 名称 · 梯度徽章 · 总榜排名     │
│ [搜索框]        │   │            + 四指标卡 + 次要指标行             │
│ # 英雄 梯度      │   ├──────────────────────────────────────────────┤
│   胜率 均名次    │   │ ② 排序切换（作用于 ③④⑤）                     │
│                │   ├──────────────────────────────────────────────┤
│ 1 安蓓萨 [1]    │   │ ③ 海克斯推荐（品质筛选 · 卡片网格 · 展开更多）   │  ← 提到最上面
│ 2 瑟提  [1]    │   ├──────────────────────────────────────────────┤
│ 3 加里奥 [2]    │   │ ④ 棱彩装备（单件 · 卡片网格）                  │
│ …             │   ├──────────────────────────────────────────────┤
│               │   │ ⑤ 核心物品（3–4 件组合路线）                    │
│               │   │    └ 附属细条：鞋子 / 技能加点                  │
│               │   ├──────────────────────────────────────────────┤
│               │   │ ⑥ 一位战绩（高手对局 / 我的一位 双页签）        │
└────────────────┘   └──────────────────────────────────────────────┘
```

**被移走的旧区块：** 现在详情页第一块是「推荐三人队伍」（`renderArenaDetailTeams`）。它不在需求列的四块里，但数据（`synergies`）是现成的，建议保留并降级——收进 ⑤ 和 ⑥ 之间，或做成概览条右侧的一个小抽屉。这是个可翻的决定，如果要彻底删掉，把 `renderArenaDetailTeams` 和 `.arena-detail-team-grid` 一起清掉即可。列表页右栏的「三人队伍协同」面板（`renderArenaTeamPodium` / `renderArenaTeamList`）在新布局里没有位置了，因为右栏整块变成了详情——需要决定是删除还是并入详情页。

**参考站里值得抄的两个细节：** 一是详情面板 sticky、列表可滚动，整页几乎不用滚就能对比英雄；二是海克斯 / 棱彩 / 核心物品用**同一套卡片语言**（图标 + 名称 + 主指标 + 次指标行），只靠标题区分，视觉统一度很高。设计稿里都实现了。

---

## 二、顶部四个指标：能不能拿到

**结论：四个全部可得，而且不需要新增任何抓取链路。** 上游 `GET https://lol-api-champion.op.gg/api/global/champions/arena/{championID}` 的 `data.summary.average_stats` 就是全部原料。

| 指标 | 计算方式 | 上游字段 | 项目当前状态 |
|---|---|---|---|
| 梯度 | 直接用 | `average_stats.tier`（**0 = OP**，1–5 为数字档） | 列表 `rows[].tier` 有；**详情 `arenaStats` 缺，需补** |
| 胜率 | `win / play` | `win`、`play` | 已有 `arenaStats.winRate` |
| 平均名次 | `total_place / play` | `total_place`、`play` | 已有 `arenaStats.averagePlacement` |
| 一位率 | `first_place / play` | `first_place`、`play` | 已有 `arenaStats.firstPlaceRate` |

### 梯度里的 OP 档：项目已经有，但斗魂分支有个坑

**好消息：OP 档不用新做。** 前端 `tierBadge(value)`（`champions.js:64-71`）的语义已经是 `0 → "op"`、`1..5 → 数字`、越界 → `.tier-badge-fallback` 文本「—」，图标是 6 个现成的静态 SVG：`web/tier-icons/{op,1,2,3,4,5}.svg`。`op.svg` 是唯一带金色渐变（`#F5CE7E → #E0AC48 → #B07E28`）+ 内描边高光的一档，字号 11（数字档是 13）。**实现时照抄现成 SVG，设计稿里那几条 `.tier-badge.top/.t1…t5` 只是配色占位，不要搬。**

**坏消息：现在的斗魂分支会把"没有梯度"误显示成 OP。** 链路是：

```go
// champions.go:139 —— 没有 omitempty，零值一定会序列化出来
Tier int `json:"tier"`

// champions.go:1326-1329 loadArenaRankings —— 直接透传，没有任何兜底
rows = append(rows, championRankingRow{ChampionID: item.ID, Rank: stats.Rank, Tier: stats.Tier, ...})
```

OP.GG 不返回 `tier` 键时，Go 零值 `0` 照样输出成 `"tier":0`，前端把它当 OP 渲染成金色徽章——**所有缺数据的英雄都会显示成最强档**。对比排位模式，`loadRanked()` 在 `champions.go:1174-1177` 是有兜底的：

```go
tierValue, rankValue := stats.TierData.Tier, stats.TierData.Rank
if tierValue == 0 && stats.Tier > 0 { tierValue = stats.Tier }
```

斗魂分支没有这一段。**修法（二选一）**：后端把 `Tier` 改成 `*int` + `omitempty`，缺数据时不输出；或者约定「无数据传 `-1`」，让前端落到已有的 `.tier-badge-fallback` 文本兜底（`tierBadge` 对越界值本来就有这条路径，前端零改动）。**推荐后者**，改动最小。这条属于顺手修的既有缺陷，不做的话 OP 档一上线就是错的。

另外注意大乱斗分支（`loadARAMRankings`，`champions.go:1285`）也是直接透传 `item.Tier`，同一个坑，可以一起处理。

次要指标行（选用率 `pick_rate` / 禁用率 `ban_rate` / 样本 `play` / KDA `(kills+assists)/deaths`）也全部可得，`kills/assists/deaths` 是现成字段但当前没往前端传。

另外 `average_stats.tier_data` 里有 `rank`、`rank_prev`、`rank_prev_patch`，可以做「总榜第 9 位（上版本 12 ↑）」这种升降提示，成本很低，设计稿里用上了。

**注意一个陷阱：整个 OP.GG arena 响应里没有任何 `win_rate` 字段**（唯一例外是 augment），所有胜率都得自己 `win/play`。不要照抄排位模式的字段名假设。

---

## 三、③④⑤ 三个区块的数据来源

三块全部来自同一个详情响应 `GET /api/global/champions/arena/{championID}`，一次请求拿全，不需要额外调用。

### ③ 海克斯推荐

上游 `data.augment_group[]`，恰好 3 组 × 15 条 = 45 条：

```
augment_group: [
  { rarity: 1, augments: [ {id, win, play, total_place, first_place, pick_rate, win_rate} × 15 ] },  // 银色
  { rarity: 4, augments: [...] },   // 黄金
  { rarity: 8, augments: [...] },   // 棱彩
]
```

`rarity` 是**数字不是字符串**：`1=银 / 4=黄金 / 8=棱彩`，已与 CommunityDragon 的 `kSilver/kGold/kPrismatic` 交叉验证一致。

后端当前把这些拍平成 `arenaAugments[]`（结构化路径上限 15 条，HTML 兜底路径上限 12 条，两条链路口径不一致）丢给前端，前端再只渲染 10 条（`slice(0,3)` 领奖台 + `slice(3,10)` 表格），品质信息在拍平过程中丢了。**建议改成分组结构**：

```go
// champions.go，新增
type arenaAugmentGroup struct {
    Rarity int                 `json:"rarity"` // 1 / 4 / 8
    Rows   []championMetricRow `json:"rows"`
}
// championDetailResponse 新增字段
ArenaAugmentGroups []arenaAugmentGroup `json:"arenaAugmentGroups,omitempty"`
```

保留旧的 `arenaAugments` 一版做过渡，前端切完再删。

**名称 / 图标 / 描述 OP.GG 完全不提供**（实测：无目录接口、页面无数据、静态站 augment 图 403）。当前代码是从 ARAM 大乱斗页 `loadAugments()` 借的目录，口径不对（大乱斗海克斯 ≠ 斗魂海克斯）。正确做法是新增一个斗魂专用目录加载器：

- 主字典：`https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json`（141KB，655 条，中文名）。字段 `id / nameTRA / augmentSmallIconPath / rarity`。**实测抽样 10 个英雄共 134 个 OP.GG augment id，134/134 全部命中，零缺失。**
- 描述补充：`https://raw.communitydragon.org/latest/cdragon/arena/zh_cn.json`（`desc` / `tooltip`）。**这个文件只有 225 条，实测缺 14 个新海克斯**，只能当补充不能当主表，缺失时不显示描述即可。
- 图标：小图用 `.../global/default/` + `augmentSmallIconPath` 去掉 `/lol-game-data/assets/` 前缀并**整体转小写**；大图必须走 `https://raw.communitydragon.org/latest/game/assets/ux/cherry/augments/icons/{slug}_large.png`（同路径挂在 `plugins/...` 下是 404，别用错根）。图标一律经 `/api/champion-asset` 代理，沿用现有回退链。

顺带：OP.GG 有个更省流量的子接口 `GET /api/global/champions/arena/{id}/augments`（5.4KB，只回 `augment_group`），如果想让海克斯区块独立刷新可以用它。

### ④ 棱彩装备

上游 `data.prism_items[]`，8 条。元素字段固定 6 个：`ids`（**恒为单件数组**）、`win`、`play`、`total_place`、`first_place`、`pick_rate`。**没有 `order`，没有 `builds`，没有 `win_rate`**（现有代码注释里提到的 `order/builds` 是排位模式的，arena 没有）。

一个容易踩的坑：`prism_items` 和 `last_items` 的 `pick_rate` 是长小数（如 `0.23862548…`，看着像「棱彩装内部占比」），普通物品是四舍五入到 4 位（`0.0848`），**两套 pick_rate 分母不同，不要放在一起横向比较**。设计稿里棱彩卡片没有展示选用率，就是为了回避这个歧义。

### ⑤ 核心物品

上游 `data.core_items[]`，15 条，`ids` 是**有序的 3–4 件组合**（这一点和 YOUR.GG 不同，它那边是单件）。组合口径信息量更大，建议保留，设计稿按「图标 › 图标 › 图标」的路线卡片渲染。

现有 `normalizeArenaBuild()` 的两段逻辑要保留：剔除 id 为 `220007`、**或**名为「棱彩装备」、**或**名为「棱镜装备」的占位道具（是三个独立的判断条件，不是某一个道具的全名）；把单件鞋子行从 `coreItems` 挪进 `boots`（`isArenaBoot` 按「鞋 / 靴 / 胫甲 / 钢盖」四个关键词判断）。

附属细条：`boots`（7 条）和 `skill_masteries`（5 条，`ids` 形如 `["Q","E","W"]`）放在本区块底部，不单开区块。`starter_items` 和 `skill_evolves` 是**空数组**，`summoner_spells` 这个 key 在 arena 详情里**根本不存在**——这三块在斗魂下要整块隐藏，不要渲染空壳。

### 三块共用的字段扩展

`championMetricRow` 目前只有 `pickRate / winRate / games`，三个区块要展示平均名次和一位率，需要补两个字段：

```go
// champions.go: championMetricRow 新增
AveragePlacement float64 `json:"averagePlacement,omitempty"` // total_place / play
FirstPlaceRate   float64 `json:"firstPlaceRate,omitempty"`   // first_place / play * 100
```

原料在 `prism_items` / `core_items` / `boots` / `last_items` / `augment_group` / `synergies` **所有区块里都有**（都带 `total_place` 和 `first_place`），一处改完全部区块受益。

### ② 排序

纯前端排序，不触发请求，同时作用于 ③④⑤。参考站是「按 tier / 按场次」两档，但我们没有物品级 tier 字段（OP.GG 不提供），所以换成四档：**按胜率（默认）/ 按一位率 / 按平均名次（升序）/ 按场次**。同值再按 `play` 降序。状态存 `state.arenaSort`，切换英雄时重置。

每块默认只露 6 个（3 列 × 2 行）+「展开全部」一次性展开，不做分页——这是参考站的做法，成本低且够用。

---

## 四、⑥ 一位战绩列表：数据怎么来

### 先回答「参考站是怎么拿的」

是的，就是最新的该英雄第一名对局。YOUR.GG 走自己的接口 `GET https://api.your.gg/{region}/api/arena/champions/{championId}/top-builds`，默认返回 5 条，**实测 `?limit=10` 可以拉到 10 条（上限就是 10，`count`/`size`/`page` 参数无效）**。这些对局天然全部满足 `me.subteamPlacement === 1` 且 `me.championKey === 查询英雄`——10 条拉满逐条验过，100% 命中，所以接口语义确实就是「该英雄的吃鸡对局」。数据来自它自建的对局库，我们没有同类爬取能力。

### 推荐方案：后端代理 + 本地降级双页签

**页签一「高手对局」——新增后端代理路由**

```
GET /api/champions/arena-first-places?championId=799&limit=10
  → 上游 https://api.your.gg/kr/api/arena/champions/{championId}/top-builds?limit=10
```

实测要点（2026-08-20）：

- **无鉴权**：不带 UA、不带 Referer 均返回 200。
- **必须服务端代理**：CORS 是白名单，只放行 `your.gg` / `www.your.gg` 两个 Origin，其余（含 `localhost`）返回**真 403 状态码**，浏览器直连一定挂。代理时**不要转发 Origin 头**。
- **必须自己缓存**：上游响应头是 `no-cache, no-store, must-revalidate`，但数据其实很静态。建议 top-builds 缓存 10–30 分钟，metadata 缓存数天。
- **没有任何限流信号**：无 `X-RateLimit-*`、无 `Retry-After`，连续 15 次全 200。自己限速，建议 ≤1 QPS。
- **英雄无数据时返回 200 + `builds: []`**，不是 404，前端要自己判空。

**隐私红线处理（必做）**：上游每个玩家都带 `summonerId`（Riot 加密稳定 ID，同区服内长期稳定）。项目红线是「不泄露稳定账号标识」，所以后端**必须剥除 `summonerId`**，只透传 `riotIdGameName` + `riotIdTagline` 用于展示。`me` 会在 `participants[]` 里重复出现一次（participants 里没有 `isMe` 字段），**去重要在后端用 summonerId 做完再剥除**，别把这个活留给前端。

**响应形状**：直接映射成项目现有的 `gameplayMatch` / `gameplayParticipant`，**前端调用总览页那个 `renderMatch()`（`gameplay.js:1127`），不写简化版战绩卡**。映射表：

| 项目字段 | 上游字段 | 备注 |
|---|---|---|
| `gameId` | `match.matchId` | |
| `createdAt` | `match.matchDate` | **毫秒 epoch** |
| `duration` | `match.gameTime` | **秒** |
| `queueId` / `queueLabel` / `modeGroup` | 固定 `1700` / `"斗魂竞技场"` / `"arena"` | 上游 `matchCategory: "Arena"` |
| `result` | 固定 `"win"` | 天然全是第一名 |
| `participants[].championId` | `championKey` | **上游给的是英文 key 不是数值 id**，需查现有 catalog 反查 |
| `participants[].displayName` | `riotIdGameName` + `#` + `riotIdTagline` | |
| `participants[].subteamId` / `placement` | `subteamId` / `subteamPlacement` | **必须透传 `subteamId`**：前端 `matchPlayerGroups()` 只有在存在 `subteamId > 0` 时才按小队分组，否则会掉进「按 `participants.length % 3 === 0 ? 3 : 2` 顺序切块」的兜底分支，分组结果是错的 |
| `participants[].itemIds` | `me.items[].id` | **长度不固定**，实测有只出 2 件的对局，别假设 6 |
| `participants[].augmentIds` | `me.augments[].id` | 固定 6 槽，未选中的是 `{id:0, image:""}` |
| `kills/deaths/assists/damage/damageTaken/gold/championLevel` | 同名字段（`damage`/`damageTaken`/`gold`/`level`） | |
| `cs` / `visionScore` / `wards*` | 上游恒为 0 | 斗魂本来就没有 |
| `primaryStyleId` / `perkIds` | 上游 `runes` **全是空壳** | 斗魂无符文，不要渲染 |

**能力边界（会直接影响 UI）**：`participants[]` 每个元素**只有 11 个轻字段**——`championKey / championImage / riotIdGameName / riotIdTagline / summonerId / subteamId / subteamPlacement / subteam / isBlue / isBot / lane`。**没有队友的 KDA、伤害、装备、海克斯。**

**这不影响"战绩卡和总览一样"这件事**，因为总览页的斗魂卡片主体本来就只用得到这些：`renderMatchPlayers()` 的斗魂分支画的是「第 N 名 + 3 个头像 + 名字」（`.match-players.is-arena` / `.arena-team-row-compact` / `.arena-team-roster`，`gameplay.css:383-388`），字段刚好够。受影响的只有**展开详情**——`renderArenaMatchOverview()`（`gameplay.js:1342-1358`）每人一行要 KDA / 伤害 / 承伤，队友这几个值全是 0。

所以处理办法是：**「高手对局」页签的卡片禁用展开按钮**（`.match-expand` 加 `disabled` + tooltip 说明"YOUR.GG 不提供队友明细"），主体渲染完全不变；**「我的一位」页签的卡片是本地完整数据，展开一切正常**。不要为此另写一套卡片。

**队伍结构**：实测当前是 **6 队 × 3 人 = 18 人**，`subteamId` 和 `subteamPlacement` 取值都是 1–6。项目记忆里的「21 人 / 7 队」是旧版本口径。**不要硬编码队伍数或每队人数**，按 `subteamId` 分组、按 `subteamPlacement` 排序即可——现有 `matchPlayerGroups()` 的主分支已经是这么写的，直接复用（但注意上一段提到的兜底分支陷阱）。

**地区问题（必须在 UI 上标注）**：YOUR.GG 的斗魂数据**只有韩服一份**。`na`/`euw` 等地区前缀返回的 `totalMatches` 和 5 个 matchId 完全一致，全是韩服玩家；`cn` 和 `global` 会 301 到一个 404 路径。所以区块标题旁要常驻「YOUR.GG · 韩服样本」标注，避免国服用户误以为是自己服的对局。设计稿里放了一条 `.notice.is-warning`。

**页签二「我的一位」——本地数据，零外部依赖**

从 `/api/gameplay/overview` 已有的结果里筛：

```js
matches.filter(m => m.modeGroup === "arena")
       .filter(m => { const s = matchSubject(m, playerRef);
                      return Number(s.championId) === championId && Number(s.placement) === 1; })
```

样本受 `pagination.count`（默认 20）限制，多数情况下会是 0 条，所以**空态必须做好**：设计稿里的写法是「还没有用安蓓萨拿过第一名 · 该英雄全球一位率 22.08%」，把空态变成有信息量的一句话。

如果想让这个页签真的有货，需要后端支持多页拉取：国服走 LCU/SGP 分页（**注意 SGP 分支的分页契约**——`matchHistory` 返回 `(games, consumed, more)`，`Pagination.Count` 用服务器侧实际消费条目数，别退回 `len(matches)==count` 的假设；LCU 兜底分支目前仍是老写法 `Count: len(rawGames)`，那是有意为之，不要顺手"统一"掉）；韩服可以用 Riot `match-v5` 的 `?queue=1700` 先拿 arena 对局 id 列表再逐个取详情，但要算好配额、务必限流。这属于可选增强，不是本次必需。

### 备选方案（如果不接受外部依赖）

只做「我的一位」单页签，不接 YOUR.GG。代价是这个区块大部分时候是空的，需求里「跟总览一样的战绩列表」的效果基本达不到。**不推荐**，但如果对新增第三方依赖有顾虑，这是唯一的替代。

---

## 四·补　斗魂战绩卡的中部数据列：把它填上，而不是清空

这一节回答「参考站截图里那三个数是什么、我们能不能加」。

**三个数分别是**：`3 / 6 / 7 (1.67)` = 击杀 / 死亡 / 助攻 + 括号里的 KDA 比值；`Damage 29.0k` = 对英雄造成的伤害；`DT 23.9k` = **D**amage **T**aken，承受伤害。

**三个我们全都有，而且零后端改动。** 对应关系与证据：

| 截图字段 | 我们的字段 | 上游 | 三条链路的填充点 |
|---|---|---|---|
| KDA `3/6/7 (1.67)` | `subject.kills/deaths/assists/kda` | 同名 | 已在用，**已经画在 `.match-kda` 里了**（`gameplay.js:1200`），数据列里不要重复 |
| Damage | `subject.damage` | `totalDamageDealtToChampions` | 韩服 Riot `riot_api.go:363 → 621`；国服 SGP 复用同一转换（`gameplay.go:930 → riot_api.go:621`）；国服 LCU 兜底 `gameplay.go:363 → 1630` |
| DT | `subject.damageTaken` | `totalDamageTaken` | 同上（`riot_api.go:364`、`gameplay.go:364`） |

转换代码对 queue 没有任何分支，斗魂（1700）不会被置零；最直接的活证据是**斗魂详情展开里那一列伤害/承伤早就在显示**（`gameplay.js:1353`，小队汇总在 `1348`/`1355`）。

**那为什么战绩卡上看不到？** 因为 `renderMatch` 主动把整列清掉了：

```js
// gameplay.js:1179-1188
let statRows;
if (arenaPlacement) {
  statRows = "";                     // ← 就是这里
} else if (match.modeGroup === "aram" || ...) { ... }
// gameplay.js:1201 —— 空串时连容器都不输出
${statRows ? `<div class="match-stats">${statRows}</div>` : ""}
```

当初清空是因为 CS / 视野在斗魂下恒为 0，但连带把伤害也一起牺牲了。

**改法**：把 `statRows = ""` 换成三行，全部复用现成变量，**不新增任何 CSS**（`.match-stats` / `.match-stat` / `.match-stat-damage` 在 `gameplay.css:332-343` 已就位，窄屏覆盖 `@container 1080px`（`gameplay.css:415-418`）和 `@container 640px`（`433-437`）里也**已经写了 `.match-main.is-arena` 的分支**，说明这一列本来就是留着的位置——`.match-main.is-arena` 是 3 列（`gameplay.css:321`），现在只坐了 `.match-champion` 和 `.match-kda`，第 3 列是空的，`.match-stats` 填进去刚好）：

```js
if (arenaPlacement) {
  statRows = `${participationRow}${damageRow}${goldRow}`;
}
```

- `participationRow`（`gameplay.js:1175`）——**斗魂下口径本来就是对的**：`groupKills` 用 `participantGroupKey(item)` 分组（`gameplay.js:1133-1135`），斗魂走的是小队键，算出来是小队参团率。
- `damageRow`（`gameplay.js:1177`）——原样复用，一个字都不用改。
- `goldRow`——需要新写一行，`subject.gold` 三条链路也都已填充（`riot_api.go:621` `Gold: raw.GoldEarned`、`gameplay.go:1630`）。这一行是可选的，只是为了让斗魂和其它模式一样有 3 行、视觉不塌。

**明确不要放**：平均段位（`tierRow`，斗魂没有段位维度，放上去只会一直是「…」）、CS / 视野 / 插眼（`cs`/`visionScore`/`wardsPlaced`/`wardsKilled` 上游恒 0）。

**数值格式**：沿用 `number()`（`gameplay.js:86`），输出 `98,209`。参考站的 `29.0k` 我们**没有对应函数**——现有 `compactNumber()`（`gameplay.js:88`）是中文「万」制（`9.8万`）。要么接受千分位，要么接受"万"，**不建议为此新写一个 k 制函数**，会和全项目其它数字口径打架。

**顺带**：`gameplayParticipant`（`gameplay.go:212-254`）里**没有**治疗 / 护盾 / 自我减伤 / 控制时长这些字段，全项目从未解析过 `totalHealsOnTeammates` / `totalDamageShieldedOnTeammates` / `damageSelfMitigated`。如果以后想加，要同时改三处结构体（`riot_api.go:336-389`、`gameplay.go` 的 `lcuParticipant.Stats`、`gameplayParticipant`）+ 两处赋值（`riot_api.go:621`、`gameplay.go:1630`）。**本次不做。**

---

| # | 文件 | 改动 | 类型 |
|---|---|---|---|
| B1 | `champions.go` | `championRankingRow` 新增 `averagePlacement`、`firstPlaceRate` | 加字段 |
| B2 | `champions.go` | arena 排行填充处补算这两个值（`total_place/play`、`first_place/play`） | 改逻辑 |
| B3 | `champions.go` | `championMetricRow` 新增 `averagePlacement`、`firstPlaceRate` | 加字段 |
| B4 | `champions_structured.go` | 物品/海克斯/羁绊映射处补算上面两个值 | 改逻辑 |
| B5 | `champions.go` | `arenaStats` 新增 `tier`、`rank`、`rankPrevPatch`、`games`、`kda` | 加字段 |
| **B5b** | `champions.go:1326-1329` | **区分「真 OP（tier=0）」与「无数据」**：`championRankingRow.Tier`（`champions.go:139`）没有 `omitempty`，缺数据时零值 0 会被前端渲染成金色 OP 徽章。无数据改传 `-1`（前端 `tierBadge` 已有越界文本兜底）或改指针 + `omitempty`。大乱斗分支 `champions.go:1285` 同坑 | 必做 |
| B6 | `champions.go` | 新增 `arenaAugmentGroup`，详情响应加 `arenaAugmentGroups`，保留旧 `arenaAugments` 过渡 | 加字段 |
| B7 | 新文件 `arena_augments.go` | 斗魂海克斯目录：CommunityDragon `cherry-augments.json`(zh_cn) 主表 + `cdragon/arena/zh_cn.json` 补描述，带磁盘/内存缓存 | 新增 |
| B8 | 新文件 `yourgg_arena.go` | 代理 `top-builds`，剥 `summonerId`、去重 `me`、映射成 `gameplayMatch`，10–30 分钟缓存，≤1 QPS 限速，失败静默降级 | 新增 |
| B9 | `main.go` | 注册 `GET /api/champions/arena-first-places` | 加路由 |
| B10 | `champions.go` | arena 分支不再渲染 `starter_items` / `skill_evolves` / `summoner_spells`（上游为空或不存在） | 清理 |
| **B11** | `champions.go` `allowedChampionHost()` | **把 `api.your.gg` 和 CommunityDragon 域名加进白名单** | 必做 |
| **B12** | `champion_cache.go` `championCachePolicy()` | **为新 host 显式加 TTL 分支**，否则不生效 | 必做 |

字段一律用 `omitempty` 加，保证老前端不炸。

### B11 / B12 是最容易漏的两步

`fetchDirect()` 第一行就会用 `allowedChampionHost()` 校验域名，不加白名单会直接返回 `champion provider request rejected`。而 `championCachePolicy()` 对未知 host 返回 `0, 0`，`fetch()` 看到 `ttl <= 0` 会**整个绕过缓存层**——所以「top-builds 缓存 10–30 分钟」这句必须落到 `championCachePolicy` 里的一个显式分支才算数，光加白名单是没有缓存的。

### 可以直接照抄的现有封装

| 用途 | 名称 | 位置 |
|---|---|---|
| 带缓存的抓取入口（首选） | `(*championProvider).fetch(ctx, host, path, query, maxBytes, accept)` | `champions.go:372` |
| 不走缓存的裸抓取 | `(*championProvider).fetchDirect(...)` | `champions.go:383` |
| 内存+磁盘两级缓存 + singleflight + stale-while-revalidate | `(*championDataCache).load(ctx, key, ttl, staleFor, loader)` | `champion_cache.go:64` |
| 缓存 key 拼装 | `championCacheKey(host, path, query, accept)` | `champion_cache.go:58` |
| TTL 策略表 | `championCachePolicy(host, path, accept)` | `champion_cache.go:252` |
| 域名白名单 | `allowedChampionHost(host)` | `champions.go:423` |
| Client 构造（TLS≥1.2、12s 超时、同域重定向≤3） | `newChampionHTTPClient(proxy)` | `champions.go:321` |

路由注册照抄 `main.go:262-267` 的写法，`a.authorized(...)` 包装不能省：

```go
mux.HandleFunc("GET /api/champions/detail", a.authorized(a.handleChampionDetail))
mux.HandleFunc("GET /api/champions/arena-first-places", a.authorized(a.handleArenaFirstPlaces)) // 新增
```

---

## 六、前端改动清单

| # | 文件 | 改动 |
|---|---|---|
| F1 | `champions.js` `renderArena()` | 右栏从「三人队伍协同面板」改为「详情面板」；`renderChampionTable(rows, true)` 打开指标列，并加平均名次列 |
| F2 | `champions.js` `renderChampionTable` | 支持 arena 专用列集（# / 英雄 / 梯度 / 胜率 / 均名次），`play < 500` 时整行 `opacity:.45` |
| F3 | `champions.js` `renderDetail` | 概览条改为 4 张指标卡 + 次要指标行；`.champion-detail-metrics.is-arena` 从 5 列改 4 列。**注意 `champions.css:458-459` 有一条 `@media (max-width:700px)` 覆盖规则把它压成 2 列并让 `> div:last-child` 横跨整行（`grid-column: 1/-1`）——改成偶数 4 列后这条会让第 4 张卡片错误地占满一行，必须一起改** |
| F4 | `champions.js` `renderDetailContent` | arena 分支区块顺序改为 ②③④⑤（协同⑦）⑥ |
| F5 | `champions.js` 新增 `renderArenaSortBar()` | 排序 chips，写 `state.arenaSort` |
| F6 | `champions.js` 重写 `renderArenaAugments()` | 按品质分组 + 筛选 chips + 每组前 6 + 展开更多 |
| F7 | `champions.js` 新增 `renderArenaItemGrid()` | 棱彩 / 核心 / 鞋子共用的卡片网格渲染器 |
| F8 | `champions.js` `renderBuildBoard` | arena 下不再走通用出装板，改用 F7；保留鞋子和技能加点细条 |
| F9 | `champions.js` 新增 `renderArenaFirstPlaces()` | 双页签 + 拉 `/api/champions/arena-first-places` + 空态 + 静默降级；卡片渲染**调用 `renderMatch`，不自己拼 DOM** |
| F10 | `gameplay.js` | **把 `renderMatch` 导出给英雄页复用**。当前整个文件是 IIFE，`renderMatch` 既没挂 `window` 也没 `module.exports`，且强依赖 `tab.openMatches` / `tab.data.capabilities` / `riotTab(tab)` / `state.matchTiers` 等外部状态 |
| **F10b** | `gameplay.js:1179-1188` | **斗魂不再清空中部数据列**：`statRows = ""` 改为 `participationRow + damageRow + goldRow`（详见「四·补」）。这一条对总览页的斗魂战绩同样生效，属于顺带修好的老问题 |
| **F10c** | `champions.js:64-71` / 后端 | **梯度徽章支持 OP 档**：前端语义已有（`0 → op.svg`），只需保证后端不把"无数据"传成 0（详见第二节）。图标用现成的 `web/tier-icons/op.svg` |
| F11 | `champions.css` | 新增 `.option-grid` / `.option-card` / `.chips` / `.slim-row` 等（设计稿里已成型，可直接搬）；删除或收起旧的 `.arena-synergy-panel` 相关块。**⑥ 战绩区块一行 CSS 都不用加**，直接吃 `gameplay.css` 的 `.match-*` |
| F12 | `demo-data.js` | **补 `/api/champions/*` 的演示数据**（当前完全没有，macOS 上无法自测本页） |

### F10：必须复用总览的 `renderMatch`，不做简化卡

用户明确要求「战绩要和总览里面的一样，不要搞特殊化」，所以**不写独立的简化战绩卡**。可选的两条路：

1. **抽公共模块**（推荐，最干净）：把 `renderMatch` 及其依赖（`renderItemSlots` / `augmentIconFigure` / `iconFigure` / `spellIconFigure` / `perkIconFigure` / `computeMatchScores` / `matchPlayerGroups` / `renderMatchPlayers` / `participantGroupKey` / `number` / `kda` / `escapeHTML` / `relativeTime` / `formatDuration`）抽到新文件 `match-card.js`，两个页面共用。改动面大，但一劳永逸，且能顺手把 `web/*.test.cjs` 的可测性提上去。
2. **导出 + 伪 tab**（最快）：`window.deepLegends.renderMatch = renderMatch`，英雄页构造一个最小 `tab` 对象传进去：

```js
const fakeTab = { openMatches: new Set(), data: { capabilities: [] }, damageSorts: new Map() };
```

   走这条要给几个函数补空值保护：`riotTab(tab)` 会读 `tab.data` 的区服信息；`matchTierCacheKey(tab, gameId)` 同理；`state.matchTiers` 是全局的可以直接用，但**斗魂根本不该请求平均段位**——按 F10b 改完之后 `tierRow` 不再出现在斗魂分支里，这个依赖自然就断了，正好省事。

**两条路都要注意**：事件绑定（`data-toggle-match` / `data-replay`）目前挂在总览页的容器上（`gameplay.js:1710-1714` 一带），英雄页要么自己绑一套，要么按上一节的结论**在「高手对局」页签直接 `disabled` 掉展开按钮**（YOUR.GG 没有队友明细，展开本来也没内容）。「我的一位」页签的数据是本地完整的，展开可以正常工作。

---

## 七、降级与容错

上游任何一环失败都不能让页面白屏，全部静默降级：

- OP.GG 结构化 JSON 失败 → 现有 HTML flight 兜底链路已存在，保留。注意 HTML 路径解析出的字段更少（没有 `total_place`），此时平均名次/一位率显示 `—` 而不是显示 0。
- CommunityDragon 海克斯目录失败 → 海克斯卡片退化为「id + 统计数值 + 字母占位图标」。注意现有的图标闪烁修复约定：目录未加载时渲染 `is-pending` 占位，**不要发注定 404 的猜测 URL**。
- YOUR.GG 代理失败 / 返回空 → 整个「高手对局」页签隐藏或显示明确的降级文案，自动切到「我的一位」，不要显示假数据。
- 低样本弱化：`play < 500` 的英雄行、`play < 100` 的物品/海克斯卡片，整体 `opacity` 降低。参考站的做法是 `matches < 30` 时把胜率文字设为 `opacity:.5`，无文案提示。

---

## 八、验收清单

- [ ] 概览条四个指标都有值，且与 OP.GG 网页上同英雄的数字一致（允许因缓存时间差有小幅偏离）
- [ ] **梯度徽章用的是现成的 `web/tier-icons/*.svg`**；`tier=0` 显示金色 OP 档，而**没有梯度数据的英雄显示「—」而不是 OP**（拿一个 OP.GG 不返回 tier 的冷门英雄验，或临时把响应里的 tier 键删掉验）
- [ ] **斗魂战绩卡的中部数据列有内容**：击杀参与率 / 伤害·承伤 / 经济三行都在，且**总览页的斗魂战绩同步生效**（这不是英雄页专属改动）
- [ ] 伤害和承伤的数字与展开详情里那一列（`gameplay.js:1353`）对得上；三条链路各验一次（国服 SGP、国服 LCU 兜底、韩服 Riot）
- [ ] 斗魂战绩卡里**没有**平均段位行、没有 CS 行（放上去会永远是「…」或 0）
- [ ] **⑥ 的战绩卡与总览页是同一个 `renderMatch` 渲染的**，不是另写的简化卡：改一处 `.match-*` 样式，两个页面同时变化
- [ ] 「高手对局」页签的展开按钮已禁用并有说明；「我的一位」页签的卡片可以正常展开
- [ ] 平均名次是 `total_place/play` 而不是名次的中位数或众数；胜率是自算的 `win/play`，没有误用不存在的 `win_rate` 字段
- [ ] 海克斯按 1/4/8 正确分成银/黄金/棱彩三组，中文名全部命中（抽查 3 个英雄，应 0 缺失）
- [ ] 棱彩卡片的选用率没有和核心物品的选用率并排展示（分母不同）
- [ ] 核心物品是 3–4 件有序组合，占位道具 220007 已被剔除，单件鞋子已归入 boots
- [ ] 排序 chips 切换后三个区块同时重排，切换英雄后重置为默认
- [ ] 一位战绩卡片里 10 条全是「第 1 名」；队伍分组按 `subteamId` 动态计算，不是硬编码 6 队或 7 队
- [ ] 后端返回的战绩里**不含 `summonerId` 或任何 puuid**（`grep` 一遍响应体）
- [ ] `me` 没有在队友列表里重复出现
- [ ] 断网状态下打开斗魂页：不白屏，各区块显示明确降级态而非空壳
- [ ] `?demo` 模式下本页可完整渲染（需要先补 F12 的演示数据）
- [ ] 窄屏检查 **700–780px 区间**：`champions.css` 用的是 `@container 780px` / `@media 700px` / `@media 760px`，`gameplay.css` 用的是 `@container 680px` / `@container 720px` / `@media 700px`，两套断点互相错位，新加的卡片网格要在这几个宽度逐一量过
- [ ] `go test -race ./...` / `go vet ./...` / `node --test web/*.test.cjs` 全绿

---

## 附录：本次实测确认过的接口清单

**OP.GG（现有链路，无需新增依赖）**

```
GET https://lol-api-champion.op.gg/api/global/champions/arena          # 173 条榜单，meta.version = 补丁号
GET https://lol-api-champion.op.gg/api/global/champions/arena/{id}     # 详情，11 个一级 key
GET https://lol-api-champion.op.gg/api/global/champions/arena/{id}/augments   # 仅海克斯，5.4KB
GET https://lol-api-champion.op.gg/api/global/champions/arena/{id}/synergies  # 仅羁绊
# 地区段可换 kr/na/euw（数据不同），cn 返回 422 Region was invalid
# ?tier= 除 all 外均无数据
```

**YOUR.GG（新增依赖，需服务端代理）**

```
GET https://api.your.gg/kr/api/arena/champions/{id}/top-builds?limit=10   # 一位对局，上限 10
GET https://api.your.gg/kr/api/metadata/arena/augments?locale=zh_CN       # 556 条（注意是 ?locale= 不是 ?lang=）
GET https://api.your.gg/kr/api/metadata/items?locale=zh_CN                # 1027 条
```

metadata 接口有两个坑：一是 `?lang=zh_CN` **无效**（返回英文），正确参数是 `?locale=zh_CN` 或 `?lang=zh` 或 `Accept-Language` 头；二是它的中文是**繁体台服文本**，且**繁中版 augments 的 description 全是空字符串**（556 条 0 条有描述，英/韩文有 225 条）。所以海克斯名称和描述都建议直接用 CommunityDragon 的 `zh_cn`，不要用 YOUR.GG 的 metadata。

**CommunityDragon（新增依赖，纯静态资源）**

```
https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json   # 655 条，主表
https://raw.communitydragon.org/latest/cdragon/arena/zh_cn.json                                            # 225 条，补描述
https://raw.communitydragon.org/latest/game/assets/ux/cherry/augments/icons/{slug}_large.png               # 大图标
```
