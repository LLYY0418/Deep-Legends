# 第八轮工单（2026-08-22 R8）

> 来源：6 张截图 + `lol-loot-diagnostics.jsonl`（5655 lcu / 84 circuit_open / 0 shape_invalid）
> **本轮含两条我上轮的判断失误（A、B），已在文内标注更正。**

---

## 〇、优先级

| 级 | 项 | 问题 | 上轮状态 |
| --- | --- | --- | --- |
| P0 | A | 海克斯图标无颜色（全灰白） | ❌ 我误判为"非 bug"并关闭 |
| P0 | B | S/A 徽章未移到图标与名称之间 | ❌ 我误读需求，未进工单 |
| P0 | C | 熔断器锁死 24h → 海克斯说明全空 | 新发现（日志实证） |
| P1 | D | 图标尺寸需放大（三处：斗魂/海斗/图鉴） | 部分未做 |
| P1 | E | 核心物品改用 YOUR.GG 数据 + S/A 评分 | 新需求 |
| P1 | F | 搭档协同：序号宽度/间隙/名称截断/列对齐 | 新需求 |
| P1 | G | 斗魂原画位置 + 红框上下高度分配 | ❌ 反复未闭环 |
| P2 | H | 海斗图标与标题未对齐；装备/路线去掉胜率 | 新需求 |
| P2 | I | 评分/胜率/场次数字配色（灰色太普通） | 新需求 |

---

## A（P0）海克斯图标上色 —— 我上轮判断失误，此处更正

### 上轮我说错了什么

我抓了 4 张图看到 `gray+alpha`，就下结论"上游素材如此、不是 bug、已关闭"。**这个结论是错的**——素材形态判断对了，但"因此无解"的推论错了。

### 实测证据（全量扫描 200 张）

```
grayscale: 183   colored: 17
```
17 张彩色的全部是 `arena_2026_s2*` 新赛季文件和 `mercy.png` 等特例。

**决定性证据**——逐像素分析标准 `_small.png`：
```
heavyhitter_small.png      | 像素 766  | 亮度 255-255 | 分布 {7: 766}
bladewaltz_small.png       | 像素 1349 | 亮度 255-255 | 分布 {7: 1349}
parasiticmutation_small.png| 像素 1127 | 亮度 255-255 | 分布 {7: 1127}
```

**所有不透明像素亮度恒为 255（纯白），连灰阶层次都没有。** 这不是"灰度图"，是**纯白剪影 + alpha 蒙版**——Riot 客户端拿 alpha 当 mask、底下铺品质色渐变来渲染。我们只贴原图当然只有白色。

### 改法：CSS mask 上色

`web/champions.css:88-91` 现在是 `<img>` 直接显示。改为用 mask：

```css
/* .augment-icon 内的图标改为 mask 上色 */
.augment-icon img[data-champion-image] { visibility: hidden; }   /* 保留 img 做加载/无障碍 */
.augment-icon::after {
  content: ""; position: absolute; inset: 4px; z-index: 2;
  -webkit-mask: var(--augment-mask) center/contain no-repeat;
          mask: var(--augment-mask) center/contain no-repeat;
  background: var(--quality-fill, linear-gradient(180deg,#DCE4EE,#9AA7B8));
}
.is-silver    { --quality-fill: linear-gradient(180deg,#DCE4EE,#9AA7B8); }
.is-gold      { --quality-fill: linear-gradient(180deg,#FFE18C,#E3B341); }
.is-prismatic { --quality-fill: linear-gradient(150deg,#C77DFF,#5AA9FF 55%,#FF8AC7); }
```

JS 侧在 `assetImage()`（champions.js:148-153）把 URL 同时写进 CSS 变量：
```js
style="--augment-mask:url('${imageURL(asset?.source, asset?.path)}')"
```

**已在沙箱验证可行**（用 PIL 模拟 mask + 三档渐变，输出正常）。

### 注意
- 17 张本身彩色的图（`arena_2026_s2*`）套 mask 会丢失原色 → 建议按文件名后缀判断，含 `arena_2026` 的走原图直显
- `imageURL()` 拼出的地址要注意 CSS `url()` 里的引号转义

### 验收
- 银/金/棱彩三档图标呈现对应色，不再是白灰剪影
- 斗魂推荐区、海斗推荐区、海克斯图鉴三处一致
- 彩色原图（2026_s2 系列）不被 mask 洗掉

---

## B（P0）S/A 徽章位置 —— 我上轮误读需求

### 上轮我说错了什么

你原话是"**S、A 这些评分放到海克斯图标和海克斯名称之间**"。我读成"现在的位置是对的"，直接标 ✅ 关闭，根本没进工单。**这是我的阅读错误。**

### 现状

`web/champions.js:965` `renderArenaOptionCard`：
```js
<div class="arena-option-main">${badge}<div class="arena-option-icons">${route}</div>${name?`<strong>...`}
```
顺序是 **徽章 → 图标 → 名称**，徽章在最左。

`web/champions.js:1219` `renderMayhemRecommendedAugment`：
```js
<div class="mayhem-recommend-item is-${rarity}"><b class="augment-grade is-${grade}">${grade}</b>${icon}<span><strong>...
```
同样是 **徽章 → 图标 → 名称**。

### 要改成

**图标 → 徽章 → 名称**，即徽章夹在图标和名称中间。两处都要改，棱彩装备（`kind==="prism"`）和海斗（mayhem）也要改。

CSS 侧 `.arena-option-card`/`.mayhem-recommend-item` 的 `grid-template-columns` 需要跟着调整顺序。

### 验收
- 斗魂海克斯推荐、棱彩装备、海斗海克斯推荐三处，徽章都在图标右侧、名称左侧
- 徽章宽度与搭档协同的序号宽度一致（见 F）

---

## C（P0）熔断器 24h 锁死 → 海克斯说明全空

### 日志实证

本次日志 `hexdata_circuit_open` **84 次**，`hexdata_shape_invalid` **0 次**、`circuit_trip` **0 次**——熔断器是"开着醒来"的，本次运行一次都没试过。

```json
{"event":"hexdata_circuit_open","time":"2026-08-21T16:09:54Z","until":"2026-08-22T22:19:08+08:00"}
{"event":"hexdata_fallback","module":"rankings","reason":"hexdata circuit is open"}
```

**`until` 锁到次日 22:19，跨度 30 小时。**

### 根因

`hexdata.go:31` `hexdataCircuitDuration = 24 * time.Hour` —— HTTP 类失败（`recordFailure`，line 554-559）退避阶梯是 `[1min, 5min, 30min, 24h]`，第四次失败直接锁 24 小时。

`hexdata.go:273-291` 有半开探测（每 5min 一次），但日志里 84 次 open 期间**没有任何探测成功的记录**，说明探测也在失败且失败路径未打点。

### 改法

1. **缩短上限**：`hexdataCircuitDuration` 24h → 2h（本地工具，用户会当场重开，24h 等于永久残废）
2. **补探测埋点**：`hexdata.go:290` 附近，探测发起/成功/失败都要 `diag`，否则无法判断是探测没跑还是跑了失败
3. **启动清零**：进程启动时若 `CircuitUntil` 距今 > 6h，视为陈旧状态直接清空（用户重启本身就是"我要重试"的信号）

### 验收
- 重启后不再出现"开着醒来"的 `circuit_open`
- 日志能看到 `hexdata_circuit_probe` 系列事件
- 海克斯图鉴的说明文字（`champions.js:837` `item.description || item.tooltip`）恢复显示

---

## D（P1）图标放大

三处尺寸（`web/champions.css`）：
- `:341` `.mayhem-recommend-item .augment-icon { 44px }` → 建议 **56px**
- `:293` `.mayhem-atlas-row .augment-icon { 42px }` → 建议 **52px**
- 斗魂 `.arena-option-card` 内图标同步放大

同时 `.mayhem-recommend-item` / `.arena-option-card` 的 `grid-template-columns` 要跟着放宽。

---

## E（P1）核心物品改用 YOUR.GG —— 已实测，字段齐全，用户已拍板采用

### 决定
**用 YOUR.GG 的数据，展示形态改成和棱彩装备一样。**（2026-08-22 用户确认）

### 现状（数据来源已查实）
- 核心物品来自 **OP.GG**：`champions_structured.go:451` `response.Build.CoreItems = p.structuredMetrics(payload.Data.CoreItems, "item", 15)`，上游字段 `core_items`（`champions_structured.go:103`）
- 棱彩装备同源：`:453` `PrismItems` ← `prism_items`
- 前端 `renderArenaCoreSection`（champions.js:906）用 `kind="core"` → 数字徽章

### 实测结果（沙箱直连，2026-08-22）

**正确端点是 `/kr/api/arena/champions/{id}`**，不是现有代码用的 `/top-builds`——后者只返回单场对局明细（`builds[].match.me.items`），没有聚合统计。

```
GET https://api.your.gg/kr/api/arena/champions/122
http=200  size=44386

response.keys = [augments, coreItems, performance, prismaticItems, totalMatches, version]

coreItems       n=  49  {S:4, A:7, B:11, C:12, D:12, F:3}
prismaticItems  n=  20  {OP:1, S:2, A:4, B:4, C:8, D:1}
augments        n= 174  {OP:3, S:7, A:30, B:46, C:51, D:25, F:12}
totalMatches    116946
version         16.16
```

**单条字段**（coreItems / prismaticItems 同构）：
```json
{
 "itemId": 223071, "tier": "D", "score": 12.11,
 "winRate": 0.4959, "averagePlacement": 3.502,
 "firstPlacementRate": 0.1414, "pickRate": 0.2290, "matches": 2835
}
```
augments 用 `augmentId`，其余字段一致。

**performance**（可替代/校验现有 arenaStats）：
```json
{"championId":122,"winRate":0.4674,"averagePlacement":3.618,
 "firstPlacementRate":0.1441,"lastPlacementRate":0.1736,
 "banRate":0.0739,"bans":8645,"matches":12376}
```

### 三个关键实测结论

1. **tier 是上游原生给的**，七档 `OP/S/A/B/C/D/F`，不需要我们算百分位。已验证 tier 与 score 严格单调：
   ```
   coreItems 按 score 降序的 tier: S S S S A A A A A A A B B B B
   prismatic 按 score 降序的 tier: OP S S A A A A B B B B C C C C
   ```
   → 直接按 `score` 降序排列 + 显示上游 `tier` 即可。

2. **⚠️ winRate 是 0~1 小数，不是百分数**（`0.4959` = 49.59%）。`averagePlacement` 是绝对值（3.502 名）。
   接入时必须确认换算路径，**这是本项目历史上翻过车的地方**（见 [[deep-legends-round9-worklist]] rate() 双重放大）。

3. **itemId 能对上现有图标链路**。6 位数（223071）是斗魂改造装备，用 CDragon `items.json` 验证：
   ```
   223071 -> 黑色切割者  /lol-game-data/assets/ASSETS/Items/Icons2D/3071_Fighter_T3_BlackCleaver...
   446632 -> 神圣分离者  /lol-game-data/assets/ASSETS/Items/Icons2D/6632_Fighter_T4_DivineDevourer...
   223078 -> 三相之力
   ```
   中文名和 iconPath 都有，走现有 `communityDragonGameAssetPath()` 即可。

### 授权边界（已向用户明示并获确认）

- `api.your.gg/robots.txt` = `User-agent: * / Disallow: /`（全站禁止）
- 主站 `your.gg/robots.txt` = `Allow: /`，只禁 `/*/match/` 与 `/*/multisearch`，注释写明是拦 SEO 爬虫省 ALB 流量
- **现有代码已在用该 host**（`/top-builds`，吃鸡战绩），UA 诚实为 `Deep-Legends/{version}`（champions.go:565）
- 限流：`waitForYourGG`（yourgg_arena.go:88）全局串行 **1 秒/请求**
- 缓存：`champion_cache.go:365` → **20 分钟内存、不落盘**（第三参 false），符合"按需查询、不打快照"

**用户已知悉 robots 状况并决定采用。** 红线维持：诚实 UA、不提速、不落盘、不预热、不打包快照。

### 改法

1. **后端**：`yourgg_arena.go` 新增 `loadArenaChampionAggregate(ctx, championID)`，打 `/kr/api/arena/champions/{id}`，复用现有 `fetchWithMetadata` + `waitForYourGG` + 缓存策略
2. **映射**：`coreItems`/`prismaticItems` → `championMetricRow`，带上 `tier` 字段（新增，现有结构体没有原生 tier）
3. **换算**：`winRate * 100` 得百分数；`firstPlacementRate * 100`；`pickRate * 100`。**加测试锁死量纲**
4. **前端**：`renderArenaCoreSection` 的 `kind` 从 `"core"` 改走与 prism 相同的分支（`augment-grade` 字母徽章），徽章值优先用上游 `row.tier`，缺失时才 fallback 到 `augmentGrade()` 百分位
5. **展示形态**：与棱彩装备一致——图标 → 徽章 → 名称（注意 B 条要求的新顺序）+ 胜率/平均名次/吃鸡率/样本

### 验收

- 核心物品显示 OP/S/A/B/C/D/F 字母徽章，与棱彩装备形态一致
- 胜率显示为 `49.59%` 而非 `0.50%` 或 `4959%`（量纲测试必须变异验证）
- 装备中文名与图标正常（223071 → 黑色切割者）
- 诊断日志能看到新端点的 `champion_upstream` 打点，且 1 秒限流生效

---

## F（P1）搭档协同对齐

### 现状
`web/champions.css:811`：
```css
.arena-synergy-grid article { grid-template-columns: 24px auto minmax(0,1fr) 54px 54px minmax(74px,auto); }
```

### 问题与改法
1. **序号与头像间隙大**：`gap: 7px` → `4px`；序号列 `24px` → 与 S/A 徽章同宽（22px，见 `.augment-grade`）
2. **第二个英雄名看不到**：`champions.js:982` `teamName(team)` 生成 "A + B"，被 `text-overflow: ellipsis` 截断。改法：两个名字各自独立 `<span>`，各占等宽列，分别截断
3. **每行对齐**：胜率 `54px`、名次 `54px` 已固定，但 `<small>` 是 `minmax(74px,auto)` 导致右侧不齐 → 改固定宽度
4. **首个头像与上方物品对齐**：序号列宽度改为与 S/A 徽章一致后自然对齐

---

## G（P1）斗魂原画位置 + 红框高度分配 —— 反复未闭环

### 你的原话
"英雄梯度下面的英雄原画还是没有去除，要放到红框那块展示原画，然后红框那块下面胜率那一行的高度低一点，上面的高度大一点"

### 现状
- `web/champions.css:732` `.arena-overview-art { position:absolute; inset:0; opacity:.18 }` —— 全幅铺底
- `:747` `.arena-overview-secondary { min-height: 26px; padding: 4px 16px }` —— R7 已从 36px 压到 26px
- 左侧英雄梯度列表每行也有原画铺底（`champions.css` 英雄行样式）

### 我上轮的疏漏
我只对齐了数值（154→132px、76→70px、36→26px），**没有处理"左侧列表行的原画要去掉"这一条**。

### 改法
1. **左侧梯度列表行**：去掉行内原画铺底（截图里每行英雄背后的大图）
2. **右侧红框区**：原画保留并加强（`opacity: .18` → `.28`，`object-position` 调整让人物脸部落在右侧）
3. **上下高度重分配**：指标区（上半）`min-height: 70px` → **78px**；secondary（下半胜率行）`min-height: 26px` → **22px**，`padding: 4px` → `3px 16px`

---

## H（P2）海斗对齐 + 去掉多余胜率

1. **图标与标题未对齐**：`.recommendation-section > header`（css:412）与 `.arena-data-section > header` 的 `align-items` / `padding` 不一致 → 统一为 `align-items: center; padding: 10px 14px`
2. **装备/装备路线去掉胜率**：`renderMayhemItemRoutes`（js:1233）、`renderMayhemOpeningConfiguration`（js:1223）里 `renderConfigOption(row, kind, true)` 第三参控制是否显示胜率 → 传 `false`

---

## I（P2）数字配色

### 现状
评分/胜率/场次都是 `var(--muted)` 灰色（`.arena-option-card dl dd` 等）。

### 改法
与其他页面对齐，按语义上色：
- 胜率 → `var(--success)` 绿（已有）
- 平均名次 → `var(--primary-strong)` 金
- 吃鸡率 → `var(--accent)` 蓝
- 样本/场次 → 保持 `var(--muted)`，但字重提到 700
- 综合评分 → 新增 `var(--ink)` + `font-variant-numeric: tabular-nums`

参照 `.arena-overview-metrics` 已有的 `.is-win/.is-placement/.is-first` 配色体系。
