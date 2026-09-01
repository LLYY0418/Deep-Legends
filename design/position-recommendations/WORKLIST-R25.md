# R25 工单：分路按钮排布 / hero 高度 / 右侧 6 条推荐 / 出装换条件链

给执行方（GPT）的工单。file:line 以 2026-08-26 工作树为准。
**标「实测」的都是本轮真跑过 curl / Go 测试 / Chromium 验证的**，标「推测」的必须先验证再落地。

---

## 0. 先更正我上一轮的一个错误结论

上一轮我说「贾克斯打野在 op.gg 上也只有一套召唤师技能，不是缺陷」。**这个结论是错的。**

- 我当时只查了 JSON 接口 `summoner_spells`，看到第二条只有 18 场，就套用我们自己的
  `structuredMetricMinimumGames=50` 门槛推理出「op.gg 也只有一套」。
- **我从来没有去看 op.gg 页面实际渲染了几行。**

本轮实测（`curl` 抓页面 + 解析 `SummonerSpells Table`）：

```
op.gg zh-cn/lol/champions/jax/build/jungle?region=kr
  行1  闪现+惩戒      99.64%   6,078 场   胜率 49.21%
  行2  幽灵疾步+惩戒   0.3%       18 场   胜率 22.22%   ← op.gg 照样显示
```

**op.gg 的规则是「无任何样本门槛、固定取前两名」。** 它连 18 场、22% 胜率的组合都会展示。
所以用户说「op.gg 有两个推荐、你漏了」是对的，我的判据（只验数据不验渲染）又犯了同一个老毛病。

---

## A. 分路按钮：3 个排一行，4 个才 2×2

### 现状

`champions.css:424` 把按钮写成 `flex: 1 1 calc(50% - 3px)`，**flex-basis 50% 强制一行最多两个**，
所以图一的塞拉斯（3 分路）被拆成 2+1 两行，白白多撑高一行。

### 改法

`.champion-detail-positions` 从 flex 改成 **grid，按分路数显式给列数**（JS 侧加 `data-count`）：

```css
.champion-detail-positions { display: grid; gap: 6px; width: 100%; }
.champion-detail-positions[data-count="1"] { grid-template-columns: repeat(1,minmax(150px,1fr)); }
.champion-detail-positions[data-count="2"] { grid-template-columns: repeat(2,minmax(150px,1fr)); }
.champion-detail-positions[data-count="3"] { grid-template-columns: repeat(3,minmax(150px,1fr)); }
.champion-detail-positions[data-count="4"] { grid-template-columns: repeat(2,minmax(150px,1fr)); }
```

`champions.js:1367` 渲染时加上 `data-count="${detailPositions.length}"`。

**为什么 150px 是下限**：按钮内容 = padding 12 + 图标 25 + gap 8 + 文字块 + padding 12。
文字块最宽的是第二行 `50.42% · 占40.64%`，上一轮 Chromium 实测需要 **96px**。
12+25+8+96+12 = **153px**。所以 `minmax(150px,1fr)` 刚好，再窄就会重新出现省略号。

**连带**：`.champion-detail-side`（`champions.css` 第三列容器）会被 3×150+2×6 = **462px** 撑宽，
标题列 `minmax(220px,1fr)` 会相应变窄。1440 宽度下总需求
104+18+220+18+462 = 822px，余量充足（实测可用约 1180px）。

**窄屏兜底**：900px 断点里第三列被写死 `316px`（`champions.css` 的
`@media (max-width: 900px)` 那条 `grid-template-columns: 104px minmax(0,1fr) 316px`），
316px 装不下 3×150。所以要加一条：

```css
@media (max-width: 1080px) {
  .champion-detail-positions[data-count="3"] { grid-template-columns: repeat(2,minmax(0,1fr)); }
}
```

即 **≥1080 时 3 个一行；<1080 退回 2+1**。四分路英雄全宽度区间都是 2×2。

> 四分路英雄全服只有 2 个（上一轮实测 173 个英雄里 1路=106 / 2路=55 / 3路=10 / 4路=2），
> 所以「3 个一行」这条覆盖了绝大多数多分路场景。

---

## B. hero 再降高度

### 现状（上一轮 Chromium 实测）

| 分路数 | hero 高度 |
|---|---|
| 1（不显示按钮） | 176px |
| 2（一行） | 176px |
| 3 / 4（两行） | 218px |

A 项做完后，3 分路会回到「一行」，也就是回到 176px。用户仍嫌高。

### 高度是被什么决定的

第三列 `.champion-detail-side` 的高度 = 四格指标 62 + row-gap 6 + 按钮 48 = **116px**；
头像 104px；hero padding 上下各 22 = 44。
所以内容高 = max(104, 116) = 116 → **116 + 44 = 160px**，但 `min-height: 176px` 把它顶到了 176。

### 改法（三处一起调，目标一行按钮时 ≈ 150px）

1. `.champion-detail-hero` 的 `min-height: 176px` → **`156px`**，`padding: 22px` → **`18px`**
2. `.champion-detail-metrics > div` 的 `min-height: 62px` → **`56px`**（`champions.css:420`）
3. `.champion-detail-positions button` 的 `min-height: 48px` → **`44px`**

核算：56 + 6 + 44 = 106；max(104, 106) = 106；106 + 36 = **142px**，由 `min-height:156` 兜到 156。
四分路两行：56+6+44+6+44 = 156 → 156+36 = **192px**（现在是 218）。

**验收标准（必须 Chromium 真量，不许估算）**：
一行按钮 ≤ **158px**，两行按钮 ≤ **196px**，且 1/2/3/4 分路 × 1440/1260/1080/900 四档
**截断审计 0 处**。头像 104px 不要改小——它是这块视觉的锚点。

---

## C. 右侧三张卡恒定 6 条推荐

### 现状

`champions.js:1496`：`loadout-side` = `renderSpellsCard` + `metricSideCard("出门装")` + `metricSideCard("鞋子")`，
每张卡 `.slice(0, 2)`（`renderSpellsCard:1494` 和 `metricSideCard` 内部各一处）。
理论 2+2+2 = 6，但召唤师技能只出 1 条时总数变成 5，右下角空一块。

### 改法（两条逻辑都要，按用户原话）

**第一步：先让召唤师技能尽量凑够 2 条。**

`champions_structured.go:633` 现在是
`structuredMetricsForKind(payload.Data.SummonerSpells, "spell", "spell", 5)`，
走的是通用门槛 `structuredSampleAllowed`（`champions_structured.go:834`）：
`play >= 50 && play/leading >= 0.01`。

贾克斯打野 KR 第二条 18 场 / 6078 = 0.3%，**两道门槛都不过**，所以被滤成 1 条。

给召唤师技能**单独放宽**（不要动全局门槛，核心装/鞋子那些必须保持现状）：

- 新增常量 `structuredSpellMinimumGames = 15`、`structuredSpellHeadRatio = 0`（即取消比例门槛）
- `structuredMetricsForKind` 增加一个门槛参数，或给 spell 走一个专门的分支
- **理由**：召唤师技能的组合空间只有个位数（实测每个英雄每个位置固定返回 5 行），
  不像装备那样有长尾噪声；而且这是「二选一」的决策，第二名再冷门也有参考价值。

**同时必须标注样本**——不能把 18 场 / 22% 胜率伪装成可靠推荐。
在该行加一个 `样本少` 角标（复用项目已有的 tooltip 链路），触发条件建议
`games < structuredMetricMinimumGames`（50），tooltip 写清「仅 N 场，仅供参考」。

**第二步：仍不足 2 条时，从出门装/鞋子补足到 6 条。**

在 `renderRuneWorkspace`（`champions.js:1480`）里把三张卡的条数改成**共享预算**：

```
预算 = 6
召唤师技能 = min(2, 可用数)
剩余 = 6 - 召唤师技能
出门装 = min(剩余 - min(2, 鞋子可用数), 出门装可用数)   // 优先保证鞋子至少 2 条
鞋子   = 剩余 - 出门装
```

即：召唤师技能 1 条时 → 出门装 3 + 鞋子 2（或 出门装 2 + 鞋子 3，取决于哪边样本更足）。
上游给的条数够不够：`structuredMetricsForKind` 对 starter/boots 的 limit 都是 5
（`champions_structured.go:634-635`），**够补**。

**注意**：补足是「多展示一条已有的真实数据」，**不是造数据**。
如果某一类上游本来就不足，允许总数 < 6，不要用占位符凑数。

---

## D. 四五六件重复推荐 → 核心装+四五件全部改用 lolalytics 条件链（本轮最大改动）

> **★2026-08-26 已定案：核心装也一并从 op.gg 切到 lolalytics**（不是只切四五件）。
> 原因见 D0——如果核心装继续用 op.gg、只有四五件用 lolalytics，两边不同源，
> 链条本身就对不齐，「不重复」这个结构性保证会失效。必须同一条链从头到尾都用同一个数据源。

### D0. 换源决策：核心装样本量交叉验证（已实测，不是主观判断）

在拍板「核心装也换源」之前，先验证了 lolalytics 会不会让核心装变得比 op.gg 更不可靠——
结论是**不会，样本量总体更大，且两个独立站点的胜率高度吻合**。

**同一个精确三件套组合，两个源分别给出的场次**（KR / emerald_plus，实测）：

| 英雄·位置 | 组合 | op.gg play | lolalytics play | 胜率 op.gg vs lolalytics |
|---|---|---|---|---|
| 贾克斯·打野 | 三相之力→破舰者→中娅沙漏 | 1366 | **2396** | 56.4% vs 57.6% |
| 卡蜜儿·上单 | 三相之力→鞋子→黑切 | 3003 | **2992** | 57.3% vs 58.2% |
| 亚索·中单 | 多兰剑→分裂之刃→无尽之刃 | 5007 | **8146** | 55.8% vs 59.0% |
| 盖伦·上单 | 兰顿之兆→水银弯刀→黑切 | 2633 | **3120** | 55.5% vs 55.5% |

**三点结论：**

1. **样本量**：4 个英雄里 3 个 lolalytics 明显更大（1.1~1.6 倍），卡蜜儿那个基本持平
   （2992 vs 3003）。**没有出现"换源后样本变薄"的情况。**
2. **胜率交叉验证**：两个完全独立的第三方统计站，各自跑各自的数据管道，
   同一组合的胜率**逐个都在 1~3 个百分点以内**——这是两边都在统计真实对局数据的强证据，
   不是凭空生成的。
3. **op.gg 核心装本身有结构性盲区**：它的 `core_items` 接口只给 3 件套（对应第 1~3 件），
   **完全没有"给定核心装之后第 4 件推荐什么"这个条件视图**——第 4/5 件是另一套毫不相关的
   边际统计。所以哪怕不考虑样本量，只用 op.gg 也做不到"核心装 + 四五件不重复"，
   这个能力缺口是 op.gg 自己没有的，必须换源才能补上。

**如实说明局限**：lolalytics 是 LoL 圈子里常用的第三方统计站（与 op.gg / u.gg 同类），
但它没有公开的方法论说明页，无法审计它具体怎么筛对局、怎么处理代练/外挂账号。
上面的交叉验证（两站胜率吻合）是能拿到的最强旁证，但不是官方审计。
`patch=30`（天窗口）的口径也和 op.gg 的窗口不完全对得上，界面上要按 D2 的要求标注
「近 30 天」，别让用户以为是当前版本从 0 累计的数据。

### D1. 问题根因（实测确认，op.gg 自己也有）

我们的第四/五/六件来自抓 op.gg 网页 RSC（`champions_structured.go:762` 的
`opggDepthItemKeyPattern` 那套）。**op.gg 这四列是各自独立的边际分布**
——「第 N 件最常见是什么」，**不以核心装为条件**。所以：

```
贾克斯打野 KR 实测
核心装[0] = 3078 三相之力, 6610 破舰者, 3157 中娅沙漏
第四件候选 = [6333, 3157, 3053, 3091, 3026]   ← 3157 又出现
第五件候选 = [3026, 3157, 6333, 3091, 3053]   ← 3157 又出现
```

用户截图里「核心装有了金身（中娅 3157），第五件又出现金身」**完全属实**，
而且 **op.gg 网页上是一模一样的**（我逐列比对过）。

**「前端去重」这条路走不通**（已实测）：四列抽自同一个很小的装备池，
剔掉核心装 3 件 + 第四件已展示的之后，**第五件候选会变成空集**。

### D2. 数据源：lolalytics `ep=build-itemset`（已实测可用）

```bash
curl --compressed \
 'https://a1.lolalytics.com/mega/?ep=build-itemset&v=1&patch=30&c=jax&lane=jungle&tier=emerald_plus&queue=420&region=kr'
```

返回 `itemSets.itemSet1..itemSet5`（剔鞋）与 `itemBootSet1..itemBootSet6`（含鞋），
每条 = `["id_id_id", 场次, 胜场]`。

**这是真正的「顺序联合分布」，不是边际分布** —— 决定性证据（KR 实测）：

```
3078_6610_3157  →  2396 场
3078_3157_6610  →     9 场     ← 同样三件、顺序不同，计数完全不同
3078_6610_6333  →   372 场
3078_6333_6610  →     5 场
```

按前缀聚合就能得到条件分布（KR / emerald_plus / patch=30 实测）：

```
第1件 n=15301: 3078(12810场,52%)
第2件 n=11096: 6610( 8753场,55%)
第3件 n= 5622: 3157( 3791场,59%)
第4件 n= 1395: 6333(  572场,61%)
第5件 n=  104: 3026(   49场,57%)
贪心链 3078 → 6610 → 3157 → 6333 → 3026    （无重复，且是结构性保证）
```

**为什么这从根上解决问题**：条件分布里前缀已经买过的装备，
按定义不可能再出现在后续位置。不需要任何去重逻辑。

### D3. 接口规格（全部实测）

| 项 | 值 |
|---|---|
| 主机 | `a1.lolalytics.com`（要加进 `champions.go:28` 的主机白名单） |
| 路径 | `/mega/` |
| 必需参数 | `ep=build-itemset` `v=1` `patch` `c` `lane` `tier` `queue` `region` |
| `c` | **lolalytics slug**，见 D4 |
| `lane` | `top` / `jungle` / `middle` / `bottom` / `support` / `all` |
| `tier` | `emerald_plus` 可用，与我们现有口径一致 |
| `region` | `kr` 可用（**已定案沿用 KR**，与页头「韩服」一致） |
| `patch` | `30`（天数窗口）/ `16.16`（具体版本）都可用 |
| `queue` | **实测被忽略**，420/440/normal 返回 md5 完全相同；`aram` 直接 404 |
| 传输 | Cloudflare 后面但**无挑战**，不带 UA 也 200；`br` 压缩，线上约 32~55KB |
| CORS | `access-control-allow-origin: *` |

**`patch` 取值建议 `30`**（30 天窗口），理由是样本量差 2 倍以上（实测）：

```
patch=30     剔鞋总场次 15301
patch=16.16  剔鞋总场次  6858
patch=16.15  剔鞋总场次  7655
```

但这会与页头「版本 16.16」的文案产生口径差。**建议在出装卡片上标注
「近 30 天」**，不要让用户以为是当前版本的纯数据。这一条请一并落地，别留隐性口径差。

⚠️ **`queue` 被忽略意味着这份数据只有排位单双一个口径**（推测，依据是 `aram` 404 + 其余值同 md5）。
**所以只对 `mode=ranked` 使用 lolalytics；ARAM / 斗魂 / 海克斯 / 无限火力一律维持 op.gg 现状。**

### D4. 英雄 slug 映射（我已穷举验证过全部 169 个）

**结论：lolalytics 的 slug = Riot key 转小写，全 169 个英雄里只有一个例外：**

```
MonkeyKing  →  wukong
```

验证方法：拿 DDragon 的 169 个 Riot key 逐个转小写去打接口，统计返回的 `itemSets` 条数。
只有 `monkeyking` 异常（`swain` 那次 -1 是网络超时，重试正常）。

⚠️ **最危险的一个坑：slug 错了不会报错。**

```
c=monkeyking  →  HTTP 200，但 itemSets 是空对象 {}
c=wukong      →  HTTP 200，itemSets 11 个键
```

**HTTP 200 + 空 itemSets 是静默失败**。必须显式判定：
`len(itemSets) == 0` 一律当作**失败**（记诊断埋点 + 走兜底），
绝对不能当成「这个英雄没有出装数据」静默渲染空白。
这正是本项目历史上反复吃亏的模式（静默兜底、屏幕上看不出来）。

### D5. 算法规格

1. 取 `itemSets.itemSet1..itemSet5`（**剔鞋族**，忽略 `itemBootSet*`），
   拍平成 `[]struct{ seq []int; games, wins int }`。
2. 贪心条件链，最多 5 步：
   ```
   prefix = []
   for step in 1..5:
       agg = {}
       for row in rows where len(row.seq) > len(prefix) and row.seq[:len(prefix)] == prefix:
           agg[row.seq[len(prefix)]] += (row.games, row.wins)
       if agg is empty: break
       候选 = agg 按 games 降序
       记录该步的 (候选 top-K, 该步总样本 n = sum(agg.games))
       prefix = append(prefix, 候选[0].item)
   ```
3. 映射到界面：
   - **核心装** = chain 第 1~3 件（三件一组，与现在的展示形态一致）
   - **第四件** = 第 4 步的候选列表（top 5）
   - **第五件** = 第 5 步的候选列表（top 5）
   - 每一步的候选**天然不含 prefix 里的装备**，所以「不与前面推荐过的重复」是结构性保证
4. **样本门槛**：某一步 `n` 低于阈值（建议 30）时，该列标注「样本少」；
   `n` 为 0 时该列显示明确空态，不要静默不渲染。

### D6. 「第六件」这一列建议去掉（实测：它就是噪声）

一局最多 6 个格子，其中一个是鞋子 → **剔鞋后最多 5 件**。
这也正是 lolalytics `itemSet` 族最长只到 `itemSet5` 的原因（实测，没有 `itemSet6`）。

op.gg 的「第六件」列实测场次（四个英雄）：

```
jax      第四件 [288,230,157,127,82]  第五件 [—,27,20,20,19]   第六件 [3,2,2,1,1]
camille  第四件 [635,601,582,218,199] 第五件 [—,49,42,36,20]   第六件 [4,3,3,2,2]
yasuo    第四件 [2124,849,394,355,239] 第五件 [—,143,72,49,49] 第六件 [3,1,1,1,1]
garen    第四件 [1586,711,652,472,415] 第五件 [—,210,184,183,155] 第六件 [21,19,17,13,8]
```

**第六件全线 1~21 场**，是「卖了鞋子再补一件」的极端局，没有推荐价值。

**建议**：出装区从「核心装 / 第四件 / 第五件 / 第六件」四列改成
**「核心装 / 第四件 / 第五件」三列**，腾出的宽度分给前三列（现在每列都偏挤）。

如果一定要保留第四列，就改成**「完整出装」**：把贪心链 5 件 + 鞋子拼成一条完整路线展示
（含鞋链实测为 `3078 → 3111(鞋) → 6610 → 3157 → 6333 → 3026`，正好 6 格）。
**这个由你定，我倾向直接砍掉第六件。**

### D7. 删掉 op.gg 的 RSC 深度抓取（连带修掉「第五件缺一行」）

用户报的「每个英雄第五件推荐都缺最后一件」我复现并定位了根因：

- `parseOPGGItemDepths`（`champions_structured.go:770`）用**固定 700 字符窗口**，
  要求窗口内同时匹配到 `metaId` / 胜率 / 场次三样。
- **`depth_5_item_0` 那一行的场次单元格是 RSC 惰性引用 `"$L7e"`**，真正的数值在**另一个 flight chunk**里。
- 于是 `games` 匹配不到 → 整行被静默丢弃。

用真解析器实测（在仓库副本里加临时测试调用 `parseOPGGItemDepths(decodeNextFlight(page))`）：

```
jax      depth4 rows=5   depth5 rows=4 ✗   depth6 rows=5
camille  depth4 rows=5   depth5 rows=4 ✗   depth6 rows=5
yasuo    depth4 rows=5   depth5 rows=4 ✗   depth6 rows=5
garen    depth4 rows=5   depth5 rows=4 ✗   depth6 rows=5
```

**100% 复现，且只有第五件列中招**（因为只有它的首行是惰性引用）。

改用 lolalytics 之后，这套 RSC 抓取整体作废。**请直接删除**
`opggDepthItemKeyPattern` / `opggDepthMetaPattern` / `opggDepthPercentPattern` /
`opggDepthGamesPattern` / `parseOPGGItemDepths` / `loadOPGGItemDepths` 及其调用点和测试，
不要留着当兜底——留着就等于把这个 bug 留在代码里。

### D8. 兜底与工程约束

- **兜底链**：lolalytics 失败或 `itemSets` 为空 → 核心装退回 op.gg 的 `core_items`（JSON 接口，可靠），
  第四/五件显示**可见的失败态**（「第四/五件推荐暂不可用，稍后重试」），
  **不要静默空白**。同时打诊断埋点，记录 slug / lane / 返回体大小 / itemSets 键数。
- **缓存**：沿用现有 `champion_cache.go` 的模式，键里必须含
  `champion + lane + tier + region + patch`，TTL 与 op.gg 详情一致（6h）。
- **并发**：沿用现有 single-flight，别让切分路时并发打上游。
- **超时**：实测 TTFB 约 0.85~1.0s、总 1.2~1.5s，建议超时 8s。
- **主机白名单**：`a1.lolalytics.com` 加进 `champions.go:28` 的 `championHosts`；
  前端 `champions.test.cjs` 的「只调本地 API」防泄漏断言不受影响（这是 Go 侧出站）。
- **限流**：我连发过 169 次（穷举 slug）没有遇到 429，但**这不构成"没有限流"的证明**。
  请保留退避重试，别做批量预取。

### D9. 抓取边界（我查过，如实说明）

- `lolalytics.com/robots.txt`：`User-agent: * → Allow: /`，**没有针对 user-directed agent 的额外条款**。
  逐个 `Disallow` 的只有 Amazonbot / Applebot-Extended / Bytespider / CCBot /
  **ClaudeBot** / CloudflareBrowserRenderingCrawler / Google-Extended / **GPTBot** / meta-externalagent
  —— 全是 AI 训练类爬虫，我们不是。
- Content-Signal：`search=yes, ai-train=no, use=reference`。我们是用户触发的按需查询、
  不做模型训练、不把快照打进 EXE，符合 `use=reference`。
- **全站没有 ToS/Terms 页面**（`/terms/` `/tos/` `/about/` `/faq/` 实测全 404），
  隐私政策里没有任何关于接口调用/自动化访问/第三方客户端的禁止性表述。
- **如实说**：这是「没有明文禁止」，**不等于 hexdata 那种「按需查询+署名=明确许可」**。
  属于灰区偏白的一侧。如果你不接受这个边界，就退回「D6 只砍第六件 + 保留 op.gg 核心装、
  彻底不展示第四/五件」这个纯净方案 —— 请明确告诉我。
- **u.gg 已排除**：全站 Cloudflare Managed Challenge，连数据主机 `stats2.u.gg` 都 403，
  绕过挑战本身就越线。

---

## E. 测试与护栏要求

> 老规矩：**每条护栏都要先植入变异体确认它真的挂**，再判定通过。
> 本项目已经证明「测试全绿」和「护栏是真的」是两回事。

### Go

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| G1 | 条件链聚合正确 | 把前缀比较 `seq[:L]==prefix` 改成 `seq[0]==prefix[0]` → 挂 |
| G2 | 链上无重复 | 断言链里 5 件互不相同；人为让候选包含 prefix 装备 → 挂 |
| G3 | 顺序敏感 | 用 `3078_6610_3157=2396 / 3078_3157_6610=9` 做 fixture，把聚合改成无序集合 → 挂 |
| G4 | **空 itemSets fail-closed** | 把 `len(itemSets)==0` 的判定删掉 → 挂（这是 monkeyking 那个坑） |
| G5 | slug 映射 | `MonkeyKing → wukong`；删掉例外表 → 挂 |
| G6 | lane 映射 | 内部 `mid/adc/support` → lolalytics `middle/bottom/support`；改错 → 挂 |
| G7 | 只对 ranked 生效 | 让 aram 也走 lolalytics → 挂 |
| G8 | 召唤师技能放宽门槛只作用于 spell | 把核心装也放宽 → 挂 |
| G9 | 缓存键含 lane/tier/region/patch | 去掉任一维 → 挂 |
| G10 | RSC 深度抓取已删干净 | 全仓 grep `parseOPGGItemDepths` 应为 0 命中 |

### JS

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| J1 | `data-count` 正确输出 | 去掉属性 → 挂 |
| J2 | 3 分路走 3 列、4 分路走 2 列 | 把 `[data-count="4"]` 的列数改成 4 → 挂 |
| J3 | 右侧总条数 = 6（可用数据充足时） | 把预算改回固定 2/2/2 → 挂 |
| J4 | 召唤师技能只有 1 条时出门装/鞋子补足 | 去掉补足逻辑 → 挂 |
| J5 | 低样本角标 | 去掉 `games < 50` 判定 → 挂 |
| J6 | hero `min-height` 与三处尺寸 | 改回 176/62/48 任一 → 挂 |

### 视觉（Playwright，必须真跑）

- 1 / 2 / 3 / 4 分路 × 1440 / 1260 / 1080 / 900 四档
- 断言：**截断审计 0 处**；一行按钮 hero ≤ 158px；两行按钮 ≤ 196px
- 断言：3 分路在 ≥1080 时按钮**在同一行**（比较三个按钮的 `getBoundingClientRect().top` 相等）
- 右侧三张卡的行数合计 = 6

---

## F. 执行顺序

1. **A + B**（纯 CSS/JS，独立可验，先把用户最直观的两条关掉）
2. **C 第一步**（Go 放宽召唤师技能门槛 + 低样本角标）
3. **C 第二步**（前端 6 条预算分配）
4. **D**（lolalytics 接入 → 删 RSC 抓取 → 出装区改版），这一步最大，建议单独一轮
5. **D6 第六件列**的取舍先跟我确认再动

A/B/C 做完就能关掉用户这次提的前三条；D 是数据源级改动，跑完 E 的全套护栏再合。
