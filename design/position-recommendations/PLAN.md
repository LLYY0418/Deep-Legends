# 分路（Position）推荐改版方案 — R24

> 目标：英雄详情页支持「按分路切换」，切换后所有推荐（召唤师技能 / 符文 / 出装 / 技能加点 / 克制）都换成该分路的真实数据；对局页保证「该英雄在当前所选位置」的推荐永远正确。
>
> 本文是给执行方的工单。所有 file:line 以 2026-08-26 工作树为准。**标注「实测」的都是本次真跑过 curl / 读码确认过的，标注「待验证」的必须在真机上确认后再写死。**

---

## 0. 一句话结论

上游 `lol-api-champion.op.gg` 的详情响应里 **`data.summary.positions[]` 本来就带全部分路 + 每路胜率 / 场次占比 / 梯度**，而且**这个数组与请求的是哪个位置无关**（camille 的 TOP / SUPPORT / MID / ADC / JUNGLE 五份响应，`positions` 字节级完全一致，实测）。

所以「有哪些分路可选」是 **0 额外请求** 就能拿到的，只是现在 Go 的 `opggStructuredDetail.Data.Summary` 只解析了 `id` 和 `average_stats`，把 `positions` 整个丢掉了（`champions_structured.go:98-101`）。

整件事的核心工作量 = 把这个字段解析出来 + 透传到前端 + 加一个切换控件 + 换掉两处「位置为空就当中单」的硬编码。

---

## 1. 上游数据实测基线

### 1.1 接口形状（现有代码已经在用）

```
GET https://lol-api-champion.op.gg/api/KR/champions/ranked/{championId}/{POSITION}?tier=emerald_plus
```

- 构造点：`champions_structured.go:170` `opggDetailRequest()`
- ranked 模式 spec：`champions_structured.go:139`，`Region: "KR"`、`PositionMode: opggPositionRequired`、`UsesTier: true`
- 位置段大小写不敏感（`/164/top` 也返回 200，实测）

### 1.2 `data.summary.positions[]` 真实结构（实测，camille=164）

```jsonc
"summary": {
  "id": 164,
  "average_stats": { ... },
  "positions": [
    {
      "name": "SUPPORT",
      "stats": {
        "play": 26318,
        "win_rate": 0.474922,
        "pick_rate": 0.0547558,
        "role_rate": 0.5699,          // ← 场次占比，分母 = summary.average_stats.play
        "ban_rate": 0.241176,
        "kda": 1.855058,
        "tier_data": { "tier": 3, "rank": 26, "rank_prev": 25, "rank_prev_patch": 4 }
      },
      "roles": [ { "name": "FIGHTER", "stats": { ... } } ]
    },
    { "name": "TOP", "stats": { "play": 18592, "win_rate": 0.506293, "role_rate": 0.402599, ... } }
  ]
}
```

**关键实测结论**

| 结论 | 证据 |
|---|---|
| `positions` 不随请求位置变 | camille 五个位置的响应，`positions` 数组字节级 distinct = 1 |
| `role_rate` 就是场次占比 | 18592 / 46180 = 0.4026 ✓ |
| 数组顺序 = 按 play 降序 | camille SUPPORT(26318) 在 TOP(18592) 前 |
| 枚举是 `TOP JUNGLE MID ADC SUPPORT`（全大写） | `middle`/`bottom`/`utility` 在 API 侧返回 **422** |
| 单分路英雄 `role_rate` 不会补到 1.0 | morgana SUPPORT = 0.8245、annie MID = 0.8098。**不能用 role_rate==1 判断单分路，必须用 `len(positions)==1`** |
| 全英雄分路数分布 | 173 个英雄：1 路=106、2 路=55、3 路=10、4 路=2 |
| 各分路数据确实完全独立 | camille TOP vs SUPPORT 逐字段 diff：`summoner_spells / core_items / boots / starter_items / last_items / rune_pages / runes / skill_masteries / skills / counters / trends / game_lengths` **全部 differ=True**，只有两个空字段相同 |

**已确认的口径偏差（需要在方案里明确接受）**：op.gg 网页 build 页默认 `region=global`，项目 ranked 写死 `Region: "KR"`。所以我们展示的占比数字跟用户在 op.gg 网页上直接看到的**对不上**（yasuo global 归一 60/24/17，KR 归一 52/28/20，实测）。
→ **决定：维持 KR 不改**。因为详情页页头明写「韩服 · 翡翠以上」，改成 global 反而与其余所有模块（榜单、克制、专家榜）串味。不做任何改动，只是别在测试里拿网页数字当基准。

### 1.3 已确认走不通 / 不要碰的路

| 路线 | 结果 |
|---|---|
| `_next/data/.../build/top.json` | 404，op.gg 已是 App Router |
| `/api/KR/champions/ranked?position=TOP` | 参数被忽略（字节数与不带参数完全相同，181732） |
| `/api/KR/champions/ranked/TOP` | 404，上游没有服务端分路榜单 |
| `sitemap-champion-positions.xml` | 是 5 位置 × 全英雄笛卡尔积，纯 SEO，**不能用来判断分路** |
| 抓 HTML / RSC 拿按钮 | HTML 里 `role_rate` 出现 0 次，只能正则扒 href + 归一后的百分比，信息量更少。**不要走这条** |

robots：`lol-api-champion.op.gg/robots.txt` 是 `User-agent: * / Disallow:`（全放行），`op.gg/robots.txt` 对 `*` 是 `Allow: /`。**这条链路没有 robots 障碍**，与 hexdata / YOUR.GG 情况不同。

### 1.4 已知上游缺陷（本次决定不兜）

op.gg 的 `positions` 会漏掉个别真实存在的偏门分路：annie 在 KR/tier=all 下 SUPPORT 反推样本 5745 场 ≈ 15.7%，**没有出现在 positions 里**；而 yasuo ADC 只有 16.8% 却列了。说明入选规则不是单纯的 role_rate 阈值。

**决定：直接把 `summary.positions` 当成已经筛好的按钮列表用，不自己加阈值、不做 5 位置全量探测。** 理由：0 额外请求、与 op.gg 网页按钮完全一致（camille / morgana / annie / graves / yasuo 逐个核对过）、用户不会觉得数据对不上。代价是极偏门分路查不到，接受。

---

## 2. Go 后端改动

### 2.1 解析 `positions`（`champions_structured.go`）

`champions_structured.go:96-117` 的 `opggStructuredDetail`，把 `Summary` 改成：

```go
Summary struct {
    ID           int             `json:"id"`
    AverageStats opggRankedStats `json:"average_stats"`
    Positions    []struct {
        Name  string          `json:"name"`
        Stats opggRankedStats `json:"stats"`
    } `json:"positions"`
} `json:"summary"`
```

`opggRankedStats`（`champions.go:1422`）**已经有** `Play / WinRate / PickRate / BanRate / KDA / Tier / Rank / TierData`，不需要新增字段。**唯一缺的是 `role_rate`**，加一行：

```go
RoleRate float64 `json:"role_rate"`
```

⚠️ `opggRankedStats` 是榜单和详情共用的结构体，加字段不会影响 `loadRanked`（`champions.go:1441`），但**必须确认 `champion_network_test.go:534` `TestStructuredOPGGDetailSchema` 的 fixture 里带上 `role_rate`**，否则新增的解析没有任何覆盖。

### 2.2 新响应字段（`champions.go`）

新增：

```go
type championPositionOption struct {
    Position string  `json:"position"`          // 内部小写口径：top/jungle/mid/adc/support
    WinRate  float64 `json:"winRate"`           // 已 ×100
    PickRate float64 `json:"pickRate"`          // 已 ×100
    BanRate  float64 `json:"banRate"`           // 已 ×100
    RoleRate float64 `json:"roleRate"`          // 已 ×100 —— 场次占比
    Play     int     `json:"play"`
    Tier     int     `json:"tier"`
    Rank     int     `json:"rank"`
}
```

挂到 `championDetailResponse`（`champions.go:349`）：

```go
Positions []championPositionOption `json:"positions,omitempty"`
```

**放在 `Position string`（:353）旁边**，语义是「这个英雄一共能打哪些分路」，`Position` 仍是「当前这份数据是哪一路」。

填充点在 `champions_structured.go:568` `loadStructuredDetail` 里，紧挨着 `Stats` 的赋值。规则：

- 只有 `spec.PositionMode == opggPositionRequired`（即 ranked）才填；其余模式留空数组 → JSON omitempty 直接不出现，前端据此不渲染切换条。
- `Position` 字段 = `strings.ToLower(raw.Name)`，且必须过一遍 `championPositionNames`（`champions.go:73`）白名单校验，**上游出现未知枚举就跳过该项**（fail-closed，别让 `TOP2` 之类的脏值渗到前端）。
- 百分比统一 ×100，与 `championRankingRow`（`champions.go:1483`）的既有口径一致。**这是本项目历史上翻车最多的地方（见 R9 `rate()` 双重放大）——前端拿到的必须已经是 50.24 而不是 0.5024。**
- 顺序：**保留上游顺序（按 play 降序）**，不要自己排。这样 `Positions[0]` 天然就是主流分路，第 4 节的兜底逻辑直接取 `[0]` 即可。

`Tier`/`Rank` 取值沿用 `loadRanked` 的写法（`champions.go:1476-1482`）：`TierData.Tier` 为 0 时回退 `*Tier`，`TierData.Rank` 为 0 时回退 `Rank`。**这段逻辑应该抽成一个共用函数**（比如 `opggTierRank(stats opggRankedStats) (int, int)`），否则两处会漂移——`build-item-row` 已经在两个文件里漂移过一次（见 R15 工单）。

### 2.3 段位回退时 positions 怎么办

`loadDetail`（`champions_structured.go:226`）在 tier ≠ `emerald_plus` 且失败/缺 counters 时会用 `emerald_plus` 再取一次，并置 `SampleTier`/`CountersTier`。

**要求**：`Positions` 必须跟随实际返回那一份数据（回退了就是回退段位的占比），**不要**把两次的 positions 混起来。前端已经在页头显示「该段位样本不足，展示 X 数据」（`champions.js:1302`），占比数字跟着变是自洽的。

### 2.4 handler 层不需要改

`champions.go:685` `handleChampionDetail` 对 ranked 已经强制 `position != "all"` 且必须命中 `championPositionNames`（:694-706），这个校验保留不动。

### 2.5 缓存不需要改

`opggDetailCacheKey`（`champion_cache.go:82`）已经把 position 拼进键：`v2|opgg-detail|ranked|KR|99|MID|emerald_plus`。切换分路 = 换 key = 独立 6h TTL 缓存条目。已有测试 `champion_network_test.go:292` 覆盖。**不要动。**

---

## 3. 英雄详情页前端（`web/champions.js` + `web/champions.css`）

### 3.1 布局：hero 内新增第二行（已定案）

现状（`champions.css:408`）：

```css
.champion-detail-hero { display: grid; min-height: 176px;
  grid-template-columns: 104px minmax(220px,1fr) auto;
  align-items: center; gap: 18px; padding: 22px; }
```

改成两行网格：

```
┌──────────────────────────────────────────────────────────────────┐
│ ┌────┐  韩服 · 翡翠以上                  ┌────┬────┬────┬────┐    │
│ │头像│  贾克斯                          │梯度│胜率│选用│禁用│    │
│ │    │  武器大师 · 版本 16.16           │ 2  │50.2│6.68│11.3│    │
│ └────┘ ┌──────────────┐┌──────────────┐└────┴────┴────┴────┘    │
│        │⚔ 上单        ││🌳 打野        │                        │
│        │50.24% · 占87%││48.10% · 占12%│   ← 选中态高亮           │
│        └──────────────┘└──────────────┘                        │
└──────────────────────────────────────────────────────────────────┘
```

CSS 要点：

```css
.champion-detail-hero {
  grid-template-columns: 104px minmax(220px,1fr) auto;
  grid-template-rows: auto auto;
  row-gap: 12px;
}
.champion-detail-portrait { grid-row: 1 / -1; grid-column: 1; align-self: center; }
.champion-detail-title    { grid-row: 1; grid-column: 2; }
.champion-detail-metrics  { grid-row: 1; grid-column: 3; }
.champion-detail-positions{ grid-row: 2; grid-column: 2 / -1; }  /* 跨标题列 + 指标列 */
```

**高度核算（必须验）**：第 1 行内容高 = max(头像 104, 标题 ~70, 四格 64) = 104；第 2 行 48；row-gap 12；padding 44 → 208px。所以 `min-height` 从 176 改成 **208**，并且要保证「容器高度 = 内容高度」这条硬等式（R18~R23 的教训：容器写死高度但内容超出，就会出现「还是贴着标签」那种视觉 bug）。

分路块本体：

```css
.champion-detail-positions { display: flex; flex-wrap: nowrap; gap: 6px; min-width: 0; }
.champion-detail-positions button {
  display: flex; flex: 0 1 auto; min-width: 0; min-height: 48px;
  align-items: center; gap: 8px; padding: 6px 12px;
  background: rgb(5 5 6 / .62); border: 1px solid rgb(255 255 255 / .18);
  border-radius: 10px; backdrop-filter: blur(10px);
}
.champion-detail-positions button.is-active {
  border-color: color-mix(in oklab, var(--primary) 62%, white);
  background: color-mix(in oklab, var(--primary) 26%, rgb(5 5 6 / .72));
}
```

每个按钮内部两行：上行 `position-icon` + 中文名，下行 `胜率 · 占比`。复用已有的 `.position-icon`（`champions.css:63`）和 `positionIcon()`（`champions.js:100-103`）——注意那张映射表是 `{all, top, jungle, mid→middle, adc→bottom, support→utility}`，图标文件名跟内部 position 值**不同名**，别写成直接拼接。

**只有一个分路时不渲染整个第二行**，并把 `min-height` 退回 176（用一个 `.has-positions` 修饰类切换，不要用 JS 量高度）。

### 3.2 状态与交互

`state` 里新增：

```js
detailPosition: null,   // 当前详情页正在看的分路（内部小写口径）
```

`openDetail(row)`（`champions.js:371-411`）现在是 `const position = row.position || state.position;`（:395）。改法：

```js
const position = state.detailPosition || row.position || (state.position !== "all" ? state.position : "") || firstPositionOf(row);
```

其中 `firstPositionOf(row)` 取 `row.positions?.[0]`（`championRankingRow.Positions` 已存在，`champions.go:159`）。**这是必需的**：从克制关系点进详情时（`openCounterDetail`，`champions.js:645`）现在复用的是主角的位置（比如给下路雷克顿查上单数据），本轮顺手修掉——改成用被点击英雄自己的 `positions[0]`。

点击分路按钮的处理：

1. `state.detailPosition = 新位置`
2. **不要整页重绘**。保留 hero，只把内容区换成骨架屏（复用 `renderDetailSkeleton()`），再 `api('/api/champions/detail?...')`。理由：整页重绘会让原画闪一下，而 hero 里的四格指标本来就要跟着变，两者节奏不同。
3. 请求成功后同时更新四格指标和内容区。

**记忆化**：`state.detailCache` 用 `${championId}:${position}:${tier}` 做键，同一次会话内来回切分路不重复发请求。上游本来就有 6h 缓存，这层只是免掉 HTTP 往返。

**持久化**：不要把 `detailPosition` 写进 localStorage。榜单筛选那个已经持久化了（`readSetting("champion-position")`，`champions.js:30` / `:84` / 写入点 `:1814` `:1984`，实际键 `lol-loot-champion-position`），它的语义是「我想看哪一路的榜单」，跟「这个英雄我想看哪一路」是两回事，串在一起会让用户从榜单点进任何英雄都被迫回到上次那一路。**返回榜单时 `detailPosition` 必须清空**（`champions.js:1786` 的 `restorePosition` 分支旁边）。

### 3.3 四格指标改成跟随分路（重要，现在是错的）

`champions.js:1303` 的四格读的是 `row.winRate / row.pickRate / row.banRate / row.tier`，即**榜单行**的数据，不是 `detail.stats`。这有两个后果：

- 切换分路后四格纹丝不动 —— 与本次需求直接冲突；
- 从克制关系点进一个不在当前榜单里的英雄，四格全是 `—`。

改成：优先用 `detail.positions` 里匹配 `detail.position` 的那一项（`winRate/pickRate/banRate/tier`），拿不到再回退 `row.*`。这样切换分路四格自动跟着变，且顺带修掉「—」那个老 bug。

### 3.4 页头文案

`champions.js:1302` 的 `韩服 · ${tierLabel(state.tier)} · ${positionLabel(row.position)}` 里的 `row.position` 要换成 `detail?.position || row.position`，否则切了分路文案还停在旧值。

---

## 4. 对局页：位置感知（`web/gameplay.js` + `gameplay.go`）

### 4.1 现状盘点（读码确认）

**已经做对的**（不要重做）：

- `liveRecommendationTarget()`（`gameplay.js:2344`）已经把 position 纳入缓存键：`${championId}:${position}:${gameMode}:${mapId}`（:2355）
- 请求已经带 position（:2392）
- ChampSelect 事件链路已经通：`lcu_events.go:92-96` 识别 `/lol-champ-select/v1/session` → `connection_manager.go:187-202` 300ms 去抖 → `broadcastEvent("champselect:changed")` → `app.js:2387` → `gameplay.js:3660` `loadLive(true)`
- 位置来源：`lcuChampSelectPlayer.AssignedPosition`（`gameplay.go:2729`）→ `:3068 / :3091-3093` merge → `:2907` `normalizePosition()` → `top/jungle/middle/bottom/utility/other`
- 装备方案 UID 已含位置：`deep-legends-v1-{championId}-{position}`（`gameplay.go:3572`）

**所以「监听位置变化就换推荐」这件事已经实现了**，用户提的这条需求现状是满足的。真正的缺口是下面这个。

### 4.2 核心缺陷：位置未知时静默当中单

```go
// gameplay.go:2454
func normalizeOPGGPosition(value string) (string, error) {
    switch strings.ToLower(strings.TrimSpace(value)) {
    case "", "other", "middle", "mid":
        return "mid", nil      // ← 空 / other 被吞进 mid
    ...
```

同样的问题在 `gameplay.go:3131-3134` `loadClientRecommendation` 里也有一处（空/other → `"middle"`）。

后果：**盲选 / 匹配模式（普通征召以外的队列）`assignedPosition` 全程为空**，此时打上单的贾克斯会拿到中单的符文和出装，而且界面上没有任何提示。这是本次真正要修的东西。

### 4.3 定案方案：三层兜底

**第一层 — 惩戒判打野**

在 `lcuChampSelectPlayer`（`gameplay.go:2728`）加两个字段：

```go
Spell1ID int64 `json:"spell1Id"`
Spell2ID int64 `json:"spell2Id"`
```

> ⚠️ **待验证**：`/lol-champ-select/v1/session` 的 `myTeam[]` 是否确实回 `spell1Id`/`spell2Id`。按 LCU 文档应该有，但本项目是 macOS 开发、无法本地联调，**必须先在 Windows 真机 dump 一次 session JSON 确认**，再决定写不写。拿不到就直接跳过第一层，只保留第二、三层——功能不受致命影响。
>
> 这个项目在 spell 字段上已经栽过一次：R13 工单（`design/WORKLIST-0823-R13.md:131-133`）就是因为 `spell1Id` 的 JSON tag 与上游实际字段名对不上、导致召唤师技能整块缺失，直到 R16 才用双字段兼容修掉。**这次不要重蹈覆辙：先看真日志的原始 key 列表，再写 tag。**

规则：`Spell1ID == 11 || Spell2ID == 11`（惩戒）→ 位置直接判定为 `jungle`，置信度标 `smite`。这条比任何统计推断都可靠。

**第二层 — 该英雄场次占比最高的分路**

`Positions[0]`（第 2.2 节已保证是按 play 降序）就是答案。实现上：`handleGameplayRecommendations`（`gameplay.go:2380`）在 position 解析为空时，**先用该英雄任意一个合法位置发一次详情请求**（因为 `positions` 与请求位置无关，实测），从 `Positions[0].Position` 取到主流分路，再用它作为最终 position 重新走一次 `loadDetail`。

代价是最多多一次上游请求，且第一次请求的结果 6h 缓存里也存着、不浪费。**别为了省这一次请求去猜一个"探测位"，就用 `MID` 当探测位即可**（任何位置返回的 positions 都一样）。

**第三层 — 用户可手动改**

在 `renderChampionRecommendationHeader()`（`gameplay.js:2743`）里加一排位置切换 chip，数据来自新返回的 `positions`。

- 客户端给了 `assignedPosition` → 默认选中它，chip 仍然可点（用户在选人阶段改位置、或客户端判错时能救）
- 客户端没给 → 默认选中自动判定的那个，并在 chip 上标一个小角标「自动」，tooltip 写清楚判定依据（「客户端未提供位置，按惩戒判定为打野」/「客户端未提供位置，按 87% 场次占比推定为上单」）
- 用户手动选过之后，**在当前这一局内锁定用户选择**，后续 `champselect:changed` 刷新不得覆盖。存法：`state.livePositionOverride = new Map()`，键用 `${championId}:${gameMode}:${mapId}`（**不含 position**，否则一改就换键、立刻失效）。
- 英雄换了（`championId` 变）→ 该键自然失效，回到自动判定。这符合直觉。

### 4.4 API 契约变化

`GET /api/gameplay/recommendations` 的响应（`gameplayRecommendationBundle`，`gameplay.go:2308` 的 `gameplayRecommendationBuild` 已有 `Position`）需要新增：

```go
Positions        []championPositionOption `json:"positions,omitempty"`   // 可选分路
ResolvedPosition string                   `json:"resolvedPosition"`      // 最终用的位置
PositionSource   string                   `json:"positionSource"`        // client | smite | role-rate | user
```

前端据 `positionSource` 决定要不要显示「自动」角标和什么文案。**`positionSource` 是 fail-visible 的关键**——历史上这个项目最常见的翻车模式就是"静默兜底成某个值，用户屏幕上看不出来"（见 memory 的验收方法论盲区）。

`position` query 参数保持现状（`main.go:286` 注册，`gameplay.go:2402-2407` 解析），但**要允许显式传空**，让后端走推断分支；现在 `opggPositionRequired` 且解析不出位置直接 400（`champions_structured.go:180-182`），这条对 handler 层要放开。

### 4.5 顺带记录，本轮不做

`specialist_runes.go:48` 只校验 position 合法性，`specialistRunes()`（:66）和 `loadSpecialistRunes()`（:115）签名里根本没有 position，缓存键是 `p.specialistCache[championID]`（:104）——**绝活哥符文完全不分位置**。前端 `ensureSpecialistRunes` 传了 position（`gameplay.js:2422`）但后端丢弃。

这是个真实的口径不一致（上单贾克斯会看到打野绝活哥的符文），但改动面在 Riot Match-V5 侧、与本次不同链路，**单独开工单**。本轮至少要做的：在 `specialist` 分区标题旁加一句说明，讲清「绝活哥符文按英雄统计，不区分分路」，别让用户以为它跟着上面的分路切换走了。

---

## 5. 非目标（明确不做）

- **ARAM / 海克斯大乱斗 / 斗魂竞技场 / 无限火力 / 极限闪击不加分路**。`opggModeSpecs`（`champions_structured.go:138-145`）里这些模式是 `LiteralNone` 或 `Omitted`，上游根本没有分路维度。前端靠 `positions` 为空自动不渲染，**不要写 `if mode === "ranked"` 这种硬编码分支**，否则以后加模式又要改一处。
- **不改 region**（见 1.2 末尾）。
- **不做 5 位置全量探测**（见 1.4）。
- **不给榜单页加服务端分路筛选**——上游没有这个接口，`loadRanked` 的本地过滤（`champions.go:1450-1475`）是唯一正确做法，保持不动。

---

## 6. 测试与护栏要求

> 本项目历史上反复出现「工单执行了、但护栏是假的」——正则型测试写完必须先做变异测试再判定（见 memory `deep-legends-mutation-testing`）。以下每一条都要求**先改代码让它失败，确认测试真的挂了，再改回来**。

### Go（`go test -race ./...`）

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| G1 | `positions` 解析 | 把 `role_rate` 的 json tag 改掉 → 挂 |
| G2 | 百分比 ×100 | 去掉 `* 100` → 挂（防 R9 的双重放大重演） |
| G3 | 顺序保留 | 加一个 `sort.Slice` 按名字排 → 挂 |
| G4 | 未知枚举 fail-closed | 塞一条 `"name":"TOP2"` → 必须被跳过而不是透传 |
| G5 | 非 ranked 模式 `Positions` 为空 | 去掉模式判断 → 挂 |
| G6 | 段位回退时 positions 跟随实际数据 | 把回退分支的 positions 换成第一次的 → 挂 |
| G7 | 位置未知时走 role-rate 推断 | 把推断改回 `return "mid"` → 挂 |
| G8 | `positionSource` 正确标注 | 把 `smite` 分支的标注改成 `client` → 挂 |
| G9 | 惩戒判打野 | spell 11 改成 12 → 挂（若第一层落地） |
| G10 | 缓存键仍含 position | 已有 `champion_network_test.go:292`，跑通即可 |

`champion_network_test.go:534` `TestStructuredOPGGDetailSchema` 的 fixture **必须补上 `positions` 段**，否则 G1~G4 无处落脚。

### JS（`node --test web/*.test.cjs desktop/*.test.cjs`）

> 注意：跑法不能用 `node --test web/`（见 R7 验收记录），要显式列 glob。

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| J1 | 单分路不渲染切换条 | 去掉 `length > 1` 判断 → 挂 |
| J2 | 图标映射走 `positionIcon()` 而非直接拼 position | 改成 `/position-icons/${position}.svg` → 挂（mid/adc/support 文件名不同名） |
| J3 | 切分路后四格读 `detail.positions` 而非 `row.*` | 改回 `row.winRate` → 挂 |
| J4 | 返回榜单清空 `detailPosition` | 去掉清空 → 挂 |
| J5 | `detailPosition` 不写 localStorage | 加一行写入 → 挂 |
| J6 | 对局页 `livePositionOverride` 键不含 position | 把 position 拼进键 → 挂 |
| J7 | `positionSource !== "client"` 时显示「自动」角标 | 去掉条件 → 挂 |
| J8 | 只调本地认证 API | 已有 `champions.test.cjs:244`，新增请求要进白名单 |

变异测试执行方式：**必须 `cp` 真副本**，软链接无效（`__dirname` 解析 realpath）；Go 侧副本要带 `data/` 和 `desktop/`，否则对照组自己就崩了，且 `if false` 在 Go 里是假杀。

### 视觉护栏

按 R18 之后的做法，用 Playwright 真 Chromium 截图 + `scrollWidth > clientWidth` 截断审计，覆盖：

- 1 / 2 / 3 / 4 分路四种英雄（morgana / camille / yasuo，4 分路的从榜单里挑）
- 窄窗口（侧边栏展开态）下分路条不换行、不截断
- hero 实测高度 == 208（有分路条）/ 176（无分路条），**硬等式断言**

---

## 7. 执行顺序建议

1. **先做 Go 的 `positions` 解析 + 响应字段**（第 2 节），配 G1~G6。这一步独立可验，前端不动也不会坏。
2. **再做详情页切换**（第 3 节），配 J1~J5 + 视觉护栏。到这里用户需求的主体就完成了。
3. **最后做对局页**（第 4 节），配 G7~G9 + J6~J7。第一层（惩戒）**先在 Windows 真机 dump 一次 champ select session JSON 确认字段存在**再写。
4. `specialist` 分区的说明文案（4.5 末尾）随手带上。

第 3 步之前，第 1 步的 `positions` 就已经能被对局页复用，两边不会重复实现。

---

## 附：本方案依赖的、需要真机确认的两件事

1. `/lol-champ-select/v1/session` 的 `myTeam[]` 是否回 `spell1Id` / `spell2Id`。**拿不到就砍掉第一层兜底**，不要用别的方式硬猜打野。
2. 盲选队列下 `assignedPosition` 到底是空字符串还是 `"NONE"`/`"none"`。`normalizePosition`（`gameplay.go:4638`）目前的分支能不能正确落到 `other`，需要用真日志核一遍——**别只看代码就下结论**，这个项目已经有过"日志里的数字是假的"的先例。
