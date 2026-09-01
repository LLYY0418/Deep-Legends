# 海克斯大乱斗改版 · 最终完整方案

> 2026-08-21 定稿。**本文取代同目录 `hexdata-fetch-policy.md`**（该文的取数分层已被本文 §3 优化掉，红线部分仍然有效但以本文为准）。
> `mayhem-redesign-plan.md` 中 `:276` 那条「gameplay.js 里已有对局内海克斯的解析」是错的，依赖它的 `:221` / `:325` 一并作废，以本文 §6 为准。
> 本文是设计文档，不是已实现状态。落地由 GPT 执行。

---

## 0. 这份文档要解决的四件事

一、更正 OP.GG 模式枚举的结论（`aram_mayhem_classic` 不是海克斯大乱斗）。
二、把数据主次改成「hexdata 为主、OP.GG 兜底」，并说清哪些字段 hexdata 真的给不了。
三、把 hexdata 取数策略重做一遍，请求数降到常数级，并把过度保守的限制放宽。
四、把海克斯页面改成和斗魂竞技场一样的左右分栏，窄屏隐藏左栏。

---

## 1. 事实更正一：OP.GG 有两个不同的海克斯模式，我上一版认错了

用户的判断是对的，而且 OP.GG 自己的 i18n 词典里写得一清二楚。从 `op.gg/lol/modes/aram-mayhem/jinx/build?region=kr`（`RSC: 1`）的载荷里原样摘出：

```
"aram_mayhem":"ARAM: Mayhem",
"aram_mayhem_classic":"ARAM: Mayhem Classic-ish",
```

两个**不同的模式**。我上一版把 `aram_mayhem_classic` 当成海克斯大乱斗、还据此测出「KR 9.4K 场 / global 64.1K 场 / version 16.15」——那批数字属于经典模式，**对本次改版没有参考价值，全部丢掉**。

真正的 `aram_mayhem` 在 OP.GG 上有两个数据面，`lol-api-champion.op.gg/api/{region}/champions/{mode}/...` 这条路径**不在其中**（传 `aram_mayhem` 返回 422 `Mode was invalid`）：

| 数据面 | URL | 实测结果 | 仓库现状 |
|---|---|---|---|
| 梯度榜 | `lol-api-champion.op.gg/api/contents/tiers?type=aram_mayhem` | 200，173 条，**只有 `{champion_id, id, tier, rank}`，没有胜率没有场次** | `champions.go:1417` 已在用 |
| 逐英雄构筑 | `op.gg/lol/modes/aram-mayhem/{slug}/build`（请求头 `RSC: 1`，会先 307 到无 locale 前缀，要跟随重定向） | 200，192KB，含 `starter_items` / `boots` / `core_items` / 技能顺序 / 召唤师技能 / augments | 未接入 |

逐英雄构筑面的三条实测细节，直接影响实现：

**物品 ID 是普通 ID，不是模式变体。** 实测取到 `starter_items` = 1038 / 1042，`boots` = 3006 / 3008，`core_items` 第一条链是 3032 → 3085 → 3031。没有出现经典模式那套 `base + 770000`。所以不需要为 ID 偏移做任何特殊处理。

**图标域名已经在白名单里。** 载荷里的图标是 `https://opgg-static.akamaized.net/meta/images/lol/16.16.1/item/{id}.png`，`remoteAsset()`（`champions.go:2228-2247`）本来就只放行这一个 host，不用改。

**版本是 16.16.1，当前补丁。** 比经典模式那条链路（16.15）新，不需要 `opggPatchIsStale`（`champions_structured.go:203-213`）额外兜底。

**代价要说清楚**：这是 Next.js 的 RSC flight 载荷，数据嵌在 JSX props 里（`metaId:3032`、`"core_items_0"` 这种行键），不是干净的 JSON 数组，而且部分行是 `$L81`/`$L82` 这类延迟块、要回到流里按 `81:` `82:` 行号解引用。仓库里已有 `decodeNextFlight()`（`champions.go:1853`）可复用，但解析器天生比 JSON 脆。**所以它只能当兜底，不能当主源**——这一点恰好和用户「所有数据以 hexdata 为主」的要求一致。

---

## 2. 事实更正二：hexdata 那五个字段，页面上有、但我们够不着

用户说「hexdata 也有出装顺序 / 起始装备 / 鞋子 / 技能加点 / 召唤师技能，都在这个页面里面」——**用户在浏览器里看到的是真的**，我上一版说 hexdata 没有这些字段，说错了。但结论不能直接翻过来，中间隔着一层。

`https://hexdata.com.cn/hero/67-vayne` 的服务端 HTML 只有 **13,037 字节**。逐个 grep「出装顺序」「起始装备」「出门装」「鞋子」「技能加点」「召唤师技能」「精选装备」，**命中数全部为 0**。这 13KB 里的 `data-seo-fallback` 只有三样东西：

1. `<meta name="description">`：`胜率 57.8%，样本 1,720,638 场，层级 T1`
2. 「推荐海克斯」表 8 行：名称 + `/augment/{id}-{slug}` 链接 + HexScore + 搭配胜率 + 样本
3. 「装备」表 8 行：**只有中文名**（暴食胫甲 / 无穷饥渴 / 金铲铲……）+ HexScore + 胜率 + 样本，**没有 item id、没有链接**

其余全部由 `/assets/index-gOYOkMuS.js` 在客户端渲染，数据取自 `/api/` 或 `/data/`——而 hexdata 的 robots.txt 是 `Disallow: /api/`、`Disallow: /data/`，只放行 `/data/ai-summary.json` 和 `/api/hexdata/answer-cards` 两个文件。

**所以「出装顺序 / 起始装备 / 鞋子 / 技能加点 / 召唤师技能」这五项，在不违反对方 robots 的前提下拿不到。** 这是本方案里唯一需要用户拍板的地方，放在 §8。本文其余部分按「不越 robots」写。

顺带记两条从这一页拿到的、必须体现在 UI 上的口径：

- `measurementTechnique`：**艾欧尼亚 Queue 2400 活跃玩家关系网冻结样本；只统计正常结束且超过 4 分钟的比赛，非段位认证、非随机抽样**。这句要能被用户看到（折叠可以，但不能没有）。
- 它自己的 SEO 表没做样本分层：薇恩页推荐海克斯第二三名是「喂呜喂呜 100.0% 样本 **1**」「至高天诺言 100.0% 样本 **1**」。**我们必须过滤，否则会出一模一样的笑话。**

---

## 3. 数据分工总表（hexdata 为主，OP.GG 兜底）

| 模块 | 主源 | 字段 | 兜底 |
|---|---|---|---|
| 左栏英雄梯度 + **胜率** | hexdata `/heroes` | id / slug / 中文名 / 胜率 / 样本 | OP.GG `contents/tiers?type=aram_mayhem`（只有 tier/rank） |
| 层级 T1–T5 | **本地按 hexdata 公布口径计算**（见下） | — | OP.GG tier |
| 推荐海克斯（详情第一块） | hexdata `/hero/{id}-{slug}` | 8 条：augmentId / 名称 / HexScore / 搭配胜率 / 样本 | OP.GG RSC `augment_group`（银/金/棱彩三组） |
| 装备排行 | hexdata `/hero/{id}-{slug}` | 8 条：中文名 / HexScore / 胜率 / 样本 | OP.GG RSC `core_items` |
| **出装顺序 / 起始装备 / 鞋子** | — | hexdata 够不着 | **OP.GG RSC**（`starter_items` / `boots` / `core_items` 链） |
| **技能加点 / 召唤师技能** | — | hexdata 够不着 | **OP.GG RSC**（`skill_0..3` / `spell_0..1`） |
| 海克斯图鉴总榜 | hexdata `/augments` | id / slug / 名称 / globalHexScore / 胜率 | OP.GG `augment_group` |
| 海克斯详情「适配英雄」 | hexdata `/augment/{id}-{slug}` | 12 条 | 无 |
| 稀有度阶段概率 | hexdata `/augment-rarity` | 四阶段银/金/棱彩占比 | 无 |
| 符文 | **不存在** | 这个模式没有符文 | — |

### 层级为什么要本地算

`/heroes` 和 `/aram-mayhem-tier-list` 两个页面的 SEO 兜底表结构完全相同，都是 `名称 | 指标 | 入口` 三列，指标列是 `胜率 57.8% · 样本 1,720,638`，**没有层级列**。层级只出现在每个英雄详情页的 meta description 里（「层级 T1」）和 `answer-cards` 的 `topHeroes`（只有 12 条）。

为了左栏一次请求就能出全 172 位英雄的层级，按 hexdata 自己 `methodology.recommendationPolicy` 公布的口径本地计算，参数直接从响应里读、不要硬编码：

```
ranking = "sample_tier_then_wilson_lower_bound_v1"
wilsonZ = 1.96
samplePolicy: low [1, 250)  medium [250, 1000)  high [1000, ∞)
```

先按 sample tier 分档，档内按 Wilson 95% 下界排序，再按下界切 T1–T5。UI 上这个徽章要标明是**本地按 Hexdata 公布口径计算**，不要伪装成 hexdata 官方层级。

### 符文页签

`champions_structured.go:137` 现在是 `"hextech-aram": {APIMode: "aram", ..., HasRunes: true}` —— 两处都错。`APIMode: "aram"` 意味着**这个页面现在展示的是韩服普通极地大乱斗的数据，整页每一个数字都是错模式的**；`HasRunes: true` 会让前端白挂一个空符文页签（`gameplay.js:2013` 目前在前端硬打补丁绕过，改完后那个补丁要撤掉）。

---

## 4. 取数方案（优化版）

用户的意见是「没有更好的策略了吗，并发不要太高不要触发异常流量就行」。上一版确实过度保守，主要问题是它从 `ai-summary.json`（只有 top12）起步，导致左栏要么不全、要么得逐英雄拉。**换个入口，请求数直接掉到常数级。**

### 4.1 三个入口，全部实测过

| 入口 | 大小 | 一次给到 |
|---|---|---|
| `/api/hexdata/answer-cards` | ~40KB | **robots 明确放行**。top12 英雄（含 tier）、top12 海克斯（globalHexScore / coverageHeroCount）、**172 个英雄 id→中文名+别名**、C 位榜 top10、完整 methodology（样本门槛、wilsonZ、citationPolicy）、buildId / reportPatch / reportDate |
| `/heroes` | 44KB | **全部 172 位英雄**：`/hero/{id}-{slug}` 链接 + 中文名 + 胜率 + 样本 |
| `/augments` | 51KB | 全部海克斯：`/augment/{id}-{slug}` + 名称 + globalHexScore + 胜率 |

`/api/hexdata/answer-cards` 比 `/data/ai-summary.json` 更全（多了 `answerCards` 和完整 `economyRanking`），两个都在 robots 白名单里，用前者。它对 query 参数不敏感（传 `?hero=67` 返回的仍是站点级摘要），**不要试图用它做逐英雄查询**。

### 4.2 请求预算

```
进入海克斯大乱斗榜单页（冷启动）  = 2 次   （answer-cards + /heroes）
点开一个英雄详情                  = 1 次   （/hero/{id}-{slug}）
进入海克斯图鉴页                  = 1 次   （/augments）
点开一个海克斯详情                = 1 次   （/augment/{id}-{slug}）
对局内（海克斯大乱斗）            = 1 次   （只拉当前英雄那一页）
```

一个重度用户一天典型 **6–15 次**。对比全量遍历的 381 次，差两个数量级；对比上一版方案，榜单页从「1 次但只有 top12」变成「2 次拿到全量」。

`/heroes` 同时解决了两个问题：左栏的胜率缺口，以及 `id → slug` 映射表（`canonicalOnly: true` 要求引用必须链回 canonical URL，而 canonical 形式是 `/hero/{id}-{slug}`）。**不要用 `/hero/{id}?astage=1` 这种形式**——实测那是个带 `<meta http-equiv="refresh">` 的跳转桩，白白多一跳。

### 4.3 请求纪律（相对上一版放宽）

| 项 | 上一版 | **本版** | 理由 |
|---|---|---|---|
| 全局并发 | 1 | **3** | 请求总数已经是常数级，3 并发的瞬时压力远低于原来串行拉一堆页面 |
| 最小间隔 | ≥1s | **≥300ms** | 同上；300ms 仍然低于任何自动化扫描的节奏 |
| 随机抖动 | 0–5s | **0–800ms** | 抖动是为了打散「N 个客户端同时刻打同一批 URL」，而我们已经取消预热，尖峰本来就不会出现 |
| 触发方式 | 用户操作触发 | **不变** | 见 §4.5 |
| singleflight | 按 cacheKey 合并 | 不变 | |
| 超时 | 连接 3s / 总 8s | 不变 | |
| 重试 | 最多 1 次、间隔 2s、4xx 不重试 | 不变 | |
| 熔断 | 连续 3 次失败或 429/403 → 停用 24h | 不变 | |
| Accept-Encoding | gzip | 不变 | 44KB 的 `/heroes` gzip 后大约 8KB |
| 条件请求 | `If-None-Match` / `If-Modified-Since` | 不变 | |

**User-Agent 必须诚实**，带产品名、版本和一个能找到我们的地址：

```
DeepLegends/<version> (+https://<项目地址>; LoL 本地助手; 用户触发查询)
```

伪装成浏览器反而是异常特征——对方风控更容易命中「UA 是 Chrome 但零静态资源请求、零 Cookie、访问序列过于规整」这种组合。诚实 UA 同时给了对方一条「有问题先联系」的路，而不是直接封。**绝不使用** `GPTBot` / `ClaudeBot` / `CCBot`，这三个在 hexdata robots 里是全站 `Disallow: /`。

### 4.4 缓存

```
cacheKey = buildId + "|" + kind + "|" + id
```

**落盘**（用户本地缓存目录），不是内存缓存。同一用户第二天开机看同一个英雄，请求数是 0 而不是 1。

**只有 buildId 变化才整体失效**，不要用固定 TTL 去猜——站点按日重建，`<meta name="hexdata-build-id">` 是它自己给的权威失效信号。buildId 自身用 12 小时软 TTL，过期后在**下一次真实用户操作时顺带**从那次要拉的页面 meta 里校验，**不单独发探测请求**。

**陈旧优先于空白**：buildId 变了但新数据没拉到时，先渲染旧缓存并标注旧的 `reportDate`，后台不抢跑。

### 4.5 三条不变的红线

1. **不打进 EXE。** 构建期抓全量塞进安装包 = 明确的「批量转载数据集」，`/methodology#data-license` 要求另行许可。（这套做法当初是对 resg 提的，对 hexdata 不成立。）
2. **不碰 `/api/` 和 `/data/`**，只走 `/api/hexdata/answer-cards` 这一个白名单文件和普通页面。
3. **不做启动预热、不做后台定时同步。** 这条最反直觉也最关键：我们没有中心服务端，N 个 EXE = N 个分散 IP，天然最不像爬虫；**一旦做预热，就变成 N 个 IP 在开机高峰同时打同一批 URL，比单机爬虫更异常，而且我们在本地一台机器上完全观测不到**。所有请求必须由一次明确的用户操作直接触发。

「全量遍历」这条在本版里已经用不上了——`/heroes` 一次给全，本来就没有遍历的必要。

### 4.6 解析健壮性

`data-seo-fallback` 是对方的 SEO 兜底，随时可能改结构且不会通知我们。

- 做**形状探测**而不是宽松容错：`/heroes` 应解析出 ≥150 行且每行都有 id+slug+胜率+样本；`/hero/{id}-{slug}` 应解析出 8 条海克斯 + 8 条装备；`/augment/{id}-{slug}` 应解析出 12 条英雄。**条数或必需字段不符 → 判定链路失效 → 熔断 24h 并降级到 OP.GG。宁可少显示，不可显示错的。**
- 解析失败**静默降级**，不抛错、不弹窗、不重试。
- 解析出的文本**一律走 DOM 节点构造，不进 `innerHTML`**（本项目既有红线）。
- 埋点记录**解析出的结构**（行数、字段命中率、buildId），不要只记响应大小——「只记大小不记结构」的教训在国服负场那个工单上已经吃过一次。

### 4.7 装备名 → item id 的匹配

hexdata 装备表**只给中文名，不给 id**。要显示图标就得反查。`champions.go:990` 已经在拉 ddragon `zh_CN/item.json`（`:1070-1092` 有名称回填逻辑），复用它建一张 `中文名 → id` 表：

- **精确匹配**，不要做模糊匹配或去空格近似——匹配错一个装备比不显示图标严重得多。
- 匹配不上就**只显示名字、不显示图标**，不要显示 `/image-unavailable.svg`。
- 埋点记录匹配率。匹配率低于 80% 说明 ddragon 语言包或 hexdata 命名变了，要能看出来。

### 4.8 署名不是装饰，是许可条件

`citationPolicy` 要求 `buildId` + `reportPatch` + `reportDate` 三个字段齐全，且 `canonicalOnly: true`。每个用到 hexdata 的模块**底部固定一行**：

```
数据来源 Hexdata · Patch 16.16 · 2026-08-19 · build hexdata-2026-08-19-2a6a2393a083   [查看原页面]
```

`[查看原页面]` 链到该英雄/海克斯的 canonical URL。**这一行不允许被折叠、不允许只在 tooltip 里出现**——它是我们能免谈授权使用这些数据的直接依据。三个字段全部来自响应本身，不要硬编码，**不要用 `time.Now()` 顶替 `reportDate`**（仓库里已经有 6 处硬编码 `time.Now()` 的历史包袱，别再加一处）。

OP.GG 兜底的模块单独标 OP.GG + 它自己的 `16.16.1`，**不要和 hexdata 的补丁号混在一起写**。页面必然是混源，每个模块独立标来源、独立标补丁、独立标日期。

---

## 5. 后端改造

### 5.1 `champions_structured.go`

```go
// :134-141 opggModeSpecs —— 改这一行
"hextech-aram": {APIMode: "aram", PositionMode: opggPositionLiteralNone, HasRunes: true, ...}
```

改成：**这条 spec 不再走 `/api/{region}/champions/{mode}` 这条路径**，因为 `aram_mayhem` 在那条路径上是 422。新增一条独立分支，数据来源是 hexdata（主）+ OP.GG RSC（兜底）。`HasRunes` 一律 `false`，`HasAugments` 一律 `true`。

`normalizeInternalChampionMode`（`:143-149`）保留 `aram-mayhem → hextech-aram` 的映射不变。

**解锁 augment 消费点。** `:441` / `:459` / `:466` 三处 `spec.APIMode == "arena"` 的门禁要改成 `spec.HasAugments`，`gameplay.go:2100` 的 `detail.Mode == "arena"` 同理。`opggArenaAugmentGroup`（`:77-80`）结构体本身可以原样复用。

**统一稀有度映射。** 仓库里有两套互相矛盾的映射：`augmentRarity()`（`champions.go:1827-1838`，1→silver/4→gold/8→prismatic）和 `arenaCDragonRarity()`（`champions_structured.go:785-787`，0→kSilver/1→kGold/2→kPrismatic/4→kEventChoice）。这次要收敛成一处，并加测试锁死。

**完整性检查要改。** `:472-474` 现在只看 `CoreItems / Boots / Skills`，而海克斯大乱斗的第一块是推荐海克斯——检查项要按模式区分，否则 hexdata 链路成功、OP.GG 链路失败时会被判成整体失败。

### 5.2 `champions.go`

`loadARAMRankings`（`:1416-1417`）现在打 `contents/tiers?type=aram_mayhem`，`:1439` 填行时不带胜率：

```go
rows = append(rows, championRankingRow{ChampionID: id, Key: item.Key, Name: item.Name, Rank: item.Rank, Tier: item.Tier})
```

改成：**优先走 hexdata `/heroes`**，填满 `WinRate` / `Play`（`championRankingRow` 在 `:134-151`，这几个字段都是 `omitempty`，本来就支持）；hexdata 不可用时回落到现有的 `contents/tiers`，此时 `WinRate` 留空、前端显示「—」而不是 0%。

新增 hexdata 客户端（建议单独文件 `hexdata.go`），承载 §4 的全部纪律：并发闸、间隔、抖动、singleflight、落盘缓存、buildId 主键、熔断、形状探测、埋点。

新增 OP.GG RSC 兜底解析：复用 `decodeNextFlight()`（`:1853`），从 `"starter_items_N"` / `"boots_N"` / `"core_items_N"` / `"skill_N"` / `"spell_N"` 行键里抓 `metaId`，注意 `$L81` 这类延迟引用要回到流里按行号解。

`remoteAsset()`（`:2228-2247`）**不用改**，RSC 里的图标 host 就是已放行的 `opgg-static.akamaized.net`。

`parseRecommendedAugments`（`:2205`）目前没有调用点，`championDetail.RecommendedAugments`（`:290`）从来没被赋值过——这次要么接上，要么删掉，不要留着。

### 5.3 隐私

**hexdata 链路不涉及任何用户标识**，不要为了「个性化推荐」往请求里塞 PUUID / summonerId / 召唤师名，一个字节都不要。这是我们能主张「用户触发的个人查询」的前提。

顺带提醒一条既有欠账（不属于本次工单，但别在这次改动里加重）：`lp-history` 目前存了 PUUID，和 `neverStores` 隐私声明冲突，建议复用 `storage.go` 的 `accountHash` 加盐。

---

## 6. 前端布局改造

### 6.1 目标结构（照抄斗魂竞技场）

现在 `renderARAM()`（`champions.js:561-572`）返回的是 `.mayhem-shell` + 视图页签，CSS 里 `.mayhem-shell` 是 `display: grid` 单列（`champions.css:190`）——这就是「上下分开、很割裂」的直接原因。

改成和 `renderArena()`（`:640-652`）同构：

```
.mayhem-workspace                       ← 两列 grid
├── section.champion-list-card.mayhem-champions   ← 左栏：英雄梯度 + 胜率
│   ├── .aram-champion-head  <h3>英雄梯度</h3>
│   ├── label.champion-search.compact
│   └── renderChampionTable(rows, "mayhem")
└── .mayhem-detail-pane                 ← 右栏：该英雄详情
    └── renderMayhemDetailPane(selected)
```

`renderChampionTable`（`:859-866`）现在第二个参数是三态（`"arena"` / `rankedMetrics` / 无指标列），要加 `"mayhem"` 分支。指标列头（`:880`）：

```js
const metricHeaders = mode === "mayhem"
  ? '<th class="metric-win">胜率</th><th class="metric-play">样本</th>'
  : ...
```

层级徽章沿用第十轮那套金/银/铜名次徽章的视觉语言，但显示的是 T1–T5。

**详情区块顺序（用户明确要求推荐海克斯放第一块）：**

```
1. 推荐海克斯        ← hexdata，三列 .mayhem-recommend-columns
2. 装备排行          ← hexdata
3. 出装顺序 / 出门装 / 鞋子   ← OP.GG RSC
4. 技能加点 / 召唤师技能      ← OP.GG RSC
（无符文页签）
```

`renderDetailContent`（`:922-935`）里 `renderRecommendedAugments`（`:924`）本来就在第一位，这个顺序是对的，保留。`renderBuildBoard`（`:1068-1074`）内部顺序是「召唤师技能/技能加点 → 出门装 → 鞋子 → 出装路线」，按上面调成 3、4 两块。

`renderDetail()`（`:901-919`）现在是**整页替换 + 返回按钮**，海克斯模式下不再走这条路——和 arena 一样，改成 `openDetail`（`:348-351`）在 `state.mode === "hextech-aram"` 时分流到 `selectMayhemChampion`，只重绘右栏。`primeArenaDetail` / `prepareArenaSelection`（`:181` / `:194`）那套请求令牌防竞态的写法要一并复用，否则快速点几个英雄会出现串号。

### 6.2 窄屏隐藏左栏（不是斗魂那种上下堆叠）

用户特意点明：要像**总览页**那样隐藏，不要像**斗魂**那样堆叠。这两个是不同实现，别抄错。

斗魂现在的写法（`champions.css:719-724`）是堆叠，而且 `champions.test.cjs:199-225` 锁死了这个契约——**不要改斗魂**，海克斯要另起一套。

要抄的是总览页（`gameplay.css:30` + `:159-176`）：

```css
.gameplay-content { container: gameplay-page / inline-size; }

.overview-layout { grid-template-columns: minmax(300px, 340px) minmax(0, 1fr); }
.overview-career-entry { display: none; }

@container gameplay-page (max-width: 1020px) {
  .overview-layout { grid-template-columns: 1fr; }
  .overview-layout > .career-column { display: none; }
  .overview-career-entry { display: flex; }
}
```

海克斯对应写成：

```css
.mayhem-workspace {
  display: grid;
  grid-template-columns: minmax(280px, 320px) minmax(0, 1fr);
  align-items: start;
  gap: 12px;
  container: mayhem-page / inline-size;      /* 具名，见下 */
}
.mayhem-detail-pane { display: grid; min-width: 0; gap: 12px; container: mayhem-pane / inline-size; }
.mayhem-tier-entry { display: none; }        /* 窄屏时的入口按钮 */

@container mayhem-page (max-width: 1020px) {
  .mayhem-workspace { grid-template-columns: 1fr; }
  .mayhem-workspace > .mayhem-champions { display: none; }
  .mayhem-tier-entry { display: flex; }
}
```

左栏被隐藏后，`.mayhem-tier-entry` 是一个「英雄梯度」按钮，点开一个和 `career-dialog` 同构的浮层（`gameplay.js:820-844` 那套 `ResizeObserver` 宽度同步的写法可直接复用），浮层里放同一份 `renderChampionTable`。**列表只渲染一份数据、两个宿主**，不要复制两套 DOM 结构出来各自维护。

### 6.3 ⚠️ 具名 container 必须先处理

`.champion-list-card` 现在是**匿名** `@container`（`champions.css:123` / `:126`），而它同时是斗魂左栏和海克斯榜的宿主。这个项目已经被匿名 container 串扰坑过一次（第八轮那个把玩家名藏掉的 bug）。

**动手第一步：先给 `.champion-list-card` 一个具名 container，把现有 `@container` 查询挂到具名上，跑一遍测试确认斗魂没回归，再开始加海克斯的样式。** 顺带把同样还是匿名的 `.rune-board-panel` 和 `.champion-build-board` 也具名了。

### 6.4 卡片高度

第十轮踩过：名单 `slice(0, 4)` 是 118px 高度锚点，去掉后卡片撑到 220px。海克斯详情里如果有类似的列表，沿用同样的截断策略，不要指望 CSS `max-height` 兜住。

---

## 7. 对局内（海克斯大乱斗模式）

`gameplay.js:1836-1845` 已经能识别海克斯大乱斗（queueId 2300/2400 或 augment 命名空间是 `KIWI` + map 12），这部分是对的。

要改的：

- `:2013` 硬编码 `hasRunes: false` 的前端补丁，在后端 spec 改成 `HasRunes: false` 之后**撤掉**，让它由后端决定。
- `:2021` 页签推入顺序调整为「推荐海克斯」在第一位。
- `assetPath()`（`:2638-2666`）**没有 `augment` 分支**，导致海克斯图标全部落到 `/image-unavailable.svg`。要补上。图标来源沿用 `:3678-3718` 的 LCU 优先 → CDragon 兜底链路。
- 推荐数据来源：当前英雄的 hexdata `/hero/{id}-{slug}`，**只拉一次、只拉当前英雄**，命中缓存就不发请求。带 `source=mayhem` 且**不带 tier**（沿用斗魂那次的做法）。

`cherry-augments.json` 里 `.../UX/Kiwi/...` 命名空间 154 条就是海克斯大乱斗的强化，`.../UX/Cherry/...` 396 条是斗魂，两者共 655 条。目录加载在 `champions_structured.go:739-749`。**catalog 缺失时不能白屏**——第六轮已经因为这个出过 P0，要走空态而不是崩。

---

## 8. 关于读 hexdata `Disallow` 接口这件事（2026-08-21 补测后：不做，不再是待拍板项）

原本这里写的是「需要用户拍板」，理由是 license 措辞（`searchAndUserDirectedRetrieval: "allowed"`）和 robots 的 `Disallow` 之间存在解释空间。**补测之后这个解释空间不存在了，本节结论改为明确不做。**

### 被禁的到底是什么

```
User-agent: *
Allow: /
Allow: /data/ai-summary.json
Allow: /api/hexdata/answer-cards
Disallow: /api/
Disallow: /data/
Disallow: /v1/track
```

从 `/assets/main-DIR2UMZi.js` 里可以看到被挡住的正是我们要的东西：`/data/heroes/{id}.json`、`/data/heroes.json`、`/data/augments/{id}.json`、`/data/items.json`、`/data/augment-items/{...}`、`/api/hexdata/heroes/{id}`、`/api/hexdata/augments/{id}` 等。

### 三层互相印证的拒绝，不是笔误

**第一层**：robots 对 `*` 禁 `/api/` `/data/`，只放行两个白名单文件。

**第二层（这条推翻了我原来的论证）**：robots 里为 `OAI-SearchBot` / `ChatGPT-User` / `Claude-SearchBot` / `Claude-User` / `PerplexityBot` / `Perplexity-User` / `Google-Extended` / `Bytespider` **逐个单独写了规则块**，注释是 `# Search, user-directed answer retrieval, and domestic AI discovery`——**站长明确考虑过「用户触发的检索」这个类别，给了页面访问权，同时照样禁掉 `/api/` 和 `/data/`。** 所以拿 `crawlerPolicy.searchAndUserDirectedRetrieval: "allowed"` 去论证「我们可以读接口」是站不住的：作者用 robots 定义了「查询」允许走哪条门，license 管的是「拿到的数字能不能用」，两句话管的是不同的事，不冲突。

**第三层**：`/assets/index-gOYOkMuS.js` 这个 loader 里有一段 UA 嗅探，正则里列的正是上面那批 agent 加 `GPTBot` / `ClaudeBot` / `CCBot`，命中就**不加载 SPA 主包**：

```js
t(navigator.userAgent) || ( p(...), import(`./main-DIR2UMZi.js`) )
```

也就是说，SEO 兜底层 = 给机器看的，SPA 数据 = 给人看的，**这是被代码强制执行的两层设计**。

### 由此推出的关键后果

带诚实 UA 去请求 `/data/`，命中的是 `Disallow`；要真的读到，实际做法必然是**把 UA 伪装成浏览器**。这一步会让性质发生跳变：从「读了不该读的路径」变成「绕过站方的技术措施」。在数据抓取的司法实践里，这条线基本就是定性的分水岭（我不是律师，但这条界线在公开判例里相当清晰）。而且我们不是中立第三方——**hexdata 自己有 HexBox**（Windows、免费、本地 OCR 三选一 + 悬浮窗 + 赛后复盘），和本项目直接重合，竞品身份会显著加重《反不正当竞争法》第十二条那类认定的分量。

### 「异常流量」不是这里的风险点

`/data/heroes/{id}.json` 是**一英雄一文件**，请求数和读 `/hero/{id}-{slug}` 页面一样是 1，甚至更小（JSON 比 HTML 省）。**流量维度上读接口比读页面更"轻"。** 所以这件事拦不住我们的不是限速能力，而是上面那三层意图。反过来说也成立：**不要用「我们量很小」来给读 `Disallow` 路径背书，量小不改变性质。**

### 真实的代价清单

被封是最轻的一档，但会**全体同时发生**：我们没有中心服务端，站方最省事的处置就是封 UA 特征，一封所有用户一起黑屏；如果那时页面的英雄榜、推荐海克斯、装备排行也都挂在 hexdata 上，整页一起死，而不只是那五个字段。

比这更贵的是不可逆的部分：一个对外主张 `neverStores` 隐私优先的本地助手，内部对一个个人维护的小站伪装 UA 绕技术措施——这个反差一旦被作者写成一篇帖子，代价不在技术层面，也没法靠改代码回滚。

### 收益侧有多小

换来的只是那五个字段的数字换个来源。而 OP.GG `aram-mayhem` 的 RSC 链路**已实测能拿到全部五项**，patch 还是 16.16.1（当前），比经典模式那条链路还新。

**结论：那五项走 OP.GG RSC 兜底，hexdata 只走页面和两个白名单文件。**

---

## 9. 验收清单

**取数**

- [ ] 冷启动进入榜单页：hexdata 请求数**恰好 2**（answer-cards + /heroes），且不早于用户进入该页面
- [ ] 连点同一英雄 10 次：请求数 **1**（singleflight + 缓存）
- [ ] 重启 EXE 后重看同一英雄：请求数 **0**（落盘缓存命中）
- [ ] buildId 变更：先渲染旧数据 + 旧 reportDate，再在下一次用户操作时更新
- [ ] 断网 / 返回 403：页面完整可用，全部模块降级到 OP.GG，无白屏、无报错弹窗
- [ ] 收到 429：24h 内不再发起任何 hexdata 请求，且熔断状态跨重启持久化
- [ ] 篡改 SEO fallback 结构（少一行 / 改字段名）：判定失效并降级，而不是渲染出残缺表
- [ ] 全仓搜不到任何构建期抓取脚本 / 打包进 EXE 的 hexdata 快照
- [ ] 全仓搜不到 `/data/` 或 `/api/`（除 `answer-cards`）的 hexdata 请求
- [ ] hexdata 请求里不含任何 PUUID / summonerId / 召唤师名

**数据正确性**

- [ ] `champions_structured.go` 里 `hextech-aram` 不再映射到 `APIMode: "aram"`
- [ ] `HasRunes` 为 false，前端不再出现空符文页签，`gameplay.js:2013` 的前端补丁已撤
- [ ] 左栏 172 位英雄都有胜率和样本，层级徽章标注「本地计算」
- [ ] 样本 < 250 的条目不出现在推荐位（薇恩页那两条「样本 1、胜率 100%」必须被过滤掉）
- [ ] 每个 hexdata 模块底部三字段署名齐全、canonical 链接可点、不可折叠
- [ ] OP.GG 兜底模块单独标 OP.GG + 16.16.1，不与 hexdata 补丁号混写
- [ ] `measurementTechnique` 那句口径（艾欧尼亚 Queue 2400 / 非随机抽样）在页面上可见
- [ ] 装备名→id 匹配率有埋点，匹配不上时只显示名字、不显示占位图

**布局**

- [ ] `.champion-list-card` / `.rune-board-panel` / `.champion-build-board` 全部具名 container
- [ ] 斗魂竞技场无回归（`champions.test.cjs:199-225` 的 1180px 堆叠契约仍然成立）
- [ ] 宽屏：左右分栏，点英雄只重绘右栏、不整页替换、不出现返回按钮
- [ ] 窄屏（≤1020px）：左栏**隐藏**，「英雄梯度」入口按钮出现，浮层可开可关
- [ ] 快速连点多个英雄：无串号（请求令牌生效）
- [ ] 推荐海克斯是详情区第一块
- [ ] 战绩卡高度未回归（118px 锚点）

**测试**

- [ ] `champions.test.cjs:112` 里断言的那句「胜率、样本和组合数据暂不展示」必须随本次改动一起删除或改写，否则测试和实现对不上
- [ ] `champions.test.cjs:102` 断言的 legacy `.aram-workspace` grid 字符串，确认是留还是删，不要留成死契约
- [ ] 新增的 CSS 契约测试要做**变异测试**验证是真护栏：把断言里的数字改掉、把 `display:none` 改成 `display:flex`，测试必须挂
- [ ] 变异测试要用 `cp` 出真副本，**不能用软链接**（`__dirname` 会解析 realpath，软链接沙箱无效）

---

## 附：明确不做的三件事

**平衡改动（伤害承受/造成 %）**——用户已决定不要。
**海克斯两两搭配组合**——用户已决定不要。
**resg.top**——上面两项去掉后它就没有不可替代的字段了，整站从方案中移除。

以及两条不建议的「优化」：

**共享缓存 / 自建中转。** 会把 N 个分散 IP 收敛成 1 个高频 IP，同时把「用户触发的个人查询」变成「我们在集中转发」——法律上更接近再分发，技术上更接近爬虫，两头都变差。

**主动联系 hexdata 谈授权。** 在还没越线的前提下不必谈；而一旦要谈，谈判对象是 HexBox 的作者，也就是我们的直接竞品。真要合作，等产品成型、有明确对价再谈。
