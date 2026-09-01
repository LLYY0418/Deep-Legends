# 斗魂竞技场 第八轮改动工单（2026-08-20）

来源：用户对实机截图（图一概览+海克斯、图二一位战绩、图三 YOUR.GG 详情）提出的 14 条改动。
本文件只做定位与方案，**不含任何代码改动**。所有 file:line 均为当前仓库实测。

---

## 1. 「一位」全部改成「吃鸡」

**定位**（Go 侧无中文「一位」，全部在前端）：

| 文件:行 | 现文案 | 改后 |
|---|---|---|
| `web/champions.js:491` | `海克斯、棱彩装备、核心物品与一位战绩` | `…与吃鸡战绩` |
| `web/champions.js:504` | `海克斯 · 棱彩 · 一位战绩` | `海克斯 · 棱彩 · 吃鸡战绩` |
| `web/champions.js:585` | `查看海克斯、装备与一位战绩` | `…与吃鸡战绩` |
| `web/champions.js:604` | `arenaOverviewMetric("一位率", …)` | `"吃鸡率"` |
| `web/champions.js:631` | `["firstPlaceRate", "按一位率"]` | `"按吃鸡率"` |
| `web/champions.js:683` | 卡片 `<dt>一位率</dt>` | `<dt>吃鸡率</dt>` |
| `web/champions.js:697` | `一位 ${percent(row.firstPlaceRate)}` | `吃鸡 ${…}` |
| `web/champions.js:699` | 同上（slim 卡片） | 同上 |
| `web/champions.js:704` | 搭档协同 `一位 ${…}` | `吃鸡 ${…}` |
| `web/champions.js:733` | `aria-label="一位战绩来源"`、`data-arena-first-tab="mine"` 按钮文案 `我的一位` | `吃鸡战绩来源` / `我的吃鸡` |
| `web/champions.js:742` | `正在读取我的一位` | `正在读取我的吃鸡` |
| `web/champions.js:744` | `${name} 的全球一位率为 …` | `…全球吃鸡率为 …` |
| `web/champions.js:750` | `<h3>一位战绩</h3>` | `<h3>吃鸡战绩</h3>` |

**不要改**：`champions.js:580`、`:585` 里的「选择**一位**英雄」是量词，不是名次。同理不要动
`state.arenaFirstTab` / `firstPlaceRate` / `data-arena-first-tab` 等**标识符**，只改可见文案与
`aria-label`。

**测试同步**：`web/champions.test.cjs` 若断言了这些文案需一并更新；建议新增一条
「`champions.js` 中不出现「一位率」」的正则断言防回归。

---

## 2. 去掉「（上版本 N）」

`web/champions.js:594` `const previousRank = Number(stats.rankPrevPatch) || 0;`
`web/champions.js:600` 末尾 `${previousRank ? `（上版本 ${previousRank}）` : ""}`

删这两处即可。`stats.rankPrevPatch` 后端字段可保留（无需动 Go）。

---

## 3. 去掉「KDA —」

`web/champions.js:606` 的 `.arena-overview-secondary` 最后一个 `<span>`：
`<span>KDA <b class="metric-kda">${number(stats.kda || row.kda, 2)}</b></span>`

删除该 span。配套删 `web/champions.css:520` `.arena-overview-secondary .metric-kda { … }`
（删掉后该选择器悬空）。

> 备注：显示成 `—` 是因为 OP.GG 的斗魂聚合接口本来就不回 KDA，属预期，删掉是对的。

---

## 4. 排序 chips 并入「选用率」行，去掉排序标题与图标

**现状**：`renderArenaSortBar()`（`web/champions.js:630-633`）是一个独立的
`<section class="arena-sort-bar">`，由 `renderArenaDetailContent`（`:621`）作为第一个 section 插入；
概览条底部第二行是 `.arena-overview-secondary`（`:606`）。

**改法**：

1. 删掉 `renderArenaSortBar()` 里 `<div>` 包裹的 `arena-section-icon` + `<strong>排序</strong>` +
   `<small>同步作用于…</small>`，只保留 `<div class="arena-chips" role="group" aria-label="竞技场数据排序">…</div>`。
2. 把这个 chips 组渲染进 `.arena-overview-secondary`（`:606`）末尾，与选用率/禁用率/样本同一行。
3. `renderArenaDetailContent`（`:620-625`）的 `sections` 数组里删掉 `renderArenaSortBar()`。
4. CSS：
   - `web/champions.css:523-527` 中 `.arena-sort-bar` 相关规则删除（`:523` 里 `.arena-data-section` 要保留）。
   - `:516` `.arena-overview-secondary` 加 `justify-content: space-between;`，
     并给 chips 组加 `margin-left: auto;` 让它靠右。
   - `:619` 的 `@container` 里 `.arena-sort-bar` 选择器一并清掉。
   - 窄屏（`:618` 那档）让 chips 换行到第二行并 `overflow-x: auto`，复用 `:620` 已有的
     `.arena-chips { max-width: 100%; overflow-x: auto; }`。

**注意**：`.arena-overview-secondary` 现在是 `flex: 0 0 100%`（`:516`），在 flex+wrap 的概览条里独占一行，
chips 塞进去不会破坏第一行的头像/指标并排布局。

---

## 5. 区块标题黄色方块内的图标放大

`web/champions.css:528`：

```
.arena-section-icon { display: grid; width: 24px; height: 24px; flex: 0 0 24px;
  place-items: center; color: #17120A; background: var(--primary);
  border-radius: 8px; font-size: 12px; font-weight: 900; }
```

字号 12px 装在 24px 方块里，视觉占比偏小。建议方块 24→26px、`font-size` 12→15px、
`border-radius` 8→9px。

**顺手修**：`color: #17120A` 是硬编码近黑色，在浅色主题 / 其他主题下 `--primary` 会变（蓝 `#58B7E8`、
绿 `#3ECF8E`、紫 `#A78BFA`、红 `#D94A5A`，见 `web/app.css:95/107/119/131`），近黑前景不一定合适。
改成 `color: color-mix(in oklab, var(--ink) 88%, var(--primary));` 或复用项目已有的
"on-primary" 令牌（若无则新增一个）。

---

## 6. 海克斯品质颜色不区分 —— 根因已定位

### 排查结论

**枚举映射是对的，不用改**：OP.GG 的 `augment_group[].rarity` 用 `1/4/8`
（实测 fixture `champion_network_test.go:396` 里 `"rarity":8`），
`augmentRarity()`（`champions.go:1746-1756`）映射 1→silver / 4→gold / 8→prismatic，
前端 `arenaRarityKey()`（`web/champions.js:645-647`）同表且对字符串幂等。图一里「棱彩」筛选能出 6 条、
徽章也正确，说明这条链路通。

**真正的根因：品质色绑了主题令牌，换主题就串色。**

```
web/champions.css:548  .arena-option-card.is-silver    { --arena-quality: color-mix(in oklab,var(--muted) 72%,var(--ink)); }
web/champions.css:549  .arena-option-card.is-gold      { --arena-quality: var(--primary-strong); }   ← 问题在这
web/champions.css:550  .arena-option-card.is-prismatic { --arena-quality: linear-gradient(180deg,oklch(.62 .18 310),oklch(.64 .13 245),oklch(.68 .16 350)); }
```

`--primary-strong` 是**随主题变的**（`web/app.css`）：

| 主题 | `--primary-strong` | 与棱彩渐变的关系 |
|---|---|---|
| 默认暗色 `:52-53` | `#F0BE5C` 金 | 可区分 |
| 蓝 `:95-96` | `#8AD2F5` 浅蓝 | **与棱彩中段 `oklch(.64 .13 245)` 几乎同色** |
| 绿 `:107-108` | `#74E2B2` 薄荷 | 勉强可分 |
| 紫 `:119-120` | `#C4B0FF` 淡紫 | **与棱彩首段 `oklch(.62 .18 310)` 几乎同色** |
| 红 `:131-132` | `#F07A85` 粉红 | **与棱彩尾段 `oklch(.68 .16 350)` 几乎同色** |

也就是说：**只要用户不是默认主题，「黄金」就不再是金色，会和「棱彩」撞成一个色**。
同一问题还出现在徽章 `web/champions.css:559 .arena-option-rarity.is-gold { color: var(--primary-strong); … }`
和 chip 圆点 `:537 .arena-rarity-chips i.is-gold { background: var(--primary-strong); }`。

**另有两处放大症状**：

- 区分只靠 `::before` 的 **3px 左边条**（`:547`），在 102px 高的卡上信息量极低。
- `.arena-option-rarity` **没有 `.is-silver` 规则**（只有 `:558` prismatic、`:559` gold），
  银色徽章退化成 base 的 `var(--muted)` 灰底灰字，与「未分类」不可区分。
- `arenaRarityKey()` 兜底返回 `"unknown"` → `class="… is-unknown"` 无对应规则 →
  落到 `:546` 的默认 `--arena-quality: var(--accent)`，而 `--accent` 在红色主题下是
  `#E0A45C`（`app.css:132`）——**和金色撞**。

### 建议改法

海克斯品质是**游戏语义**，不是主题装饰，应当脱离主题令牌，用固定色：

```
.arena-option-card            { --arena-quality: var(--muted); }        /* unknown 兜底改成中性灰，别用 --accent */
.arena-option-card.is-silver  { --arena-quality: #9AA7B8; }
.arena-option-card.is-gold    { --arena-quality: #E3B341; }
.arena-option-card.is-prismatic { --arena-quality: linear-gradient(180deg,#C77DFF,#5AA9FF,#FF8AC7); }
```

同步固定 `.arena-rarity-chips i.is-*`（`:536-538`）与 `.arena-option-rarity.is-*`（`:557-559`），
并**补上 `.arena-option-rarity.is-silver`**。

同时把区分度从 3px 边条提升：左边条 3px→4px，并给卡片加
`border-color: color-mix(in oklab, var(--arena-quality) 38%, var(--line));`
（棱彩是渐变不能进 `color-mix`，给 `.is-prismatic` 单独写一条固定 border-color）。

### 验收方式

切到**紫色主题**打开斗魂页 → 海克斯筛选选「全部」→ 黄金卡与棱彩卡的左边条必须肉眼可分。
这是最短的复现路径。

---

## 7. 去掉 OP.GG 字样

三处 `<span class="arena-source-tag">OP.GG</span>`：

- `web/champions.js:665`（`renderArenaItemSection`，棱彩装备）
- `web/champions.js:672`（`renderArenaCoreSection`，核心物品）
- 另有文案 `web/champions.js:622` `"单件 · OP.GG prism_items"` → 改为 `"单件"`

删除后 `web/champions.css:544 .arena-source-tag { … }` 悬空，一并删。

> 注意：图二里的 `YOUR.GG · 韩服样本` 提示条是**另一条**（见第 11 条），不要混。

---

## 8. 鞋子：去掉标题文字，推荐并排一行

`web/champions.js:672` `renderArenaCoreSection` 末尾：

```js
<div class="arena-slim-rows">${renderArenaSlimRow("鞋子", sortedArenaRows(build.boots || []).slice(0, 3), "boots")}${renderArenaSlimRow("技能加点", (build.skills || []).slice(0, 1), "skills")}</div>
```

**改法**：`renderArenaSlimRow`（`:695-700`）里删掉 `<strong>${label}</strong>`，
CSS `web/champions.css:575` `.arena-slim-row { grid-template-columns: 72px minmax(0,1fr); }`
改成 `grid-template-columns: minmax(0,1fr);`，`:577 .arena-slim-row > strong` 删除，
`:622` 窄屏那档的 `.arena-slim-row { grid-template-columns: 1fr; }` 也就冗余了。

`.arena-slim-grid` 目前已是三列并排（检查 `champions.css` 中 `.arena-slim-grid` 定义，
若为 `repeat(auto-fit,…)` 需固定成 `repeat(3,minmax(0,1fr))`，并在窄屏保持一行、允许横向滚动）。

---

## 9. 去掉技能加点

同 `web/champions.js:672`：删掉 `${renderArenaSlimRow("技能加点", (build.skills || []).slice(0, 1), "skills")}`。

连带清理：

- `renderArenaSlimRow` 里 `if (kind === "skills") { … }` 整个分支（`:696-698`）变成死代码，删。
- `:671` 的空判断 `if (!sorted.length && !(build.boots || []).length && !(build.skills || []).length) return "";`
  去掉 `build.skills` 那一项。
- 后端 `championBuildSections.Skills`（`champions.go:229` 附近）如果只服务斗魂页可以留着不动
  （召唤师峡谷页仍在用），**不要删 Go 字段**。

---

## 10. 搭档协同：当前英雄排第一

**排查结论：两条数据链路，只有一条保证了顺序。**

- 结构化链路 `structuredSynergies()`（`champions_structured.go:713-732`）**已经**是
  `[]arenaTeamChampion{subject, mate}`（`:727`），当前英雄天然在第一位。✅
- HTML 兜底链路 `parseArenaTeamCompositions()`（`champions.go:1503-1545`，
  调用点 `:1474` 与 `:1916`）**原样保留 OP.GG 页面里的英雄顺序**，当前英雄可能排第二/第三。❌
- 而 `champions_structured.go:369-370` 在 HTML 结果更丰富时会**优先采用 HTML 版**，
  所以线上大概率走的是没保证顺序的那条。

**改法（二选一，推荐 A）**：

- **A（后端，推荐）**：给 `parseArenaTeamCompositions` 增加一个 `subjectID int` 参数，
  组装完 `team.Champions` 后做一次稳定前移：把 `ID == subjectID` 的元素移到 index 0，
  其余保持原序。两个调用点（`champions.go:1474`、`:1916`）传入当前英雄 ID。
  这样前端零改动，且 `arenaTeamComposition` 的语义变成「首位恒为主视角英雄」，可写进注释。
- **B（前端）**：`renderArenaTeamFaces`（`web/champions.js:772-774`）渲染前重排。
  缺点是 `teamName()`（`:776-779`）拼名字也要同步重排，两处容易漏。

**加测试**：在 `champions_test.go` 里对 `parseArenaTeamCompositions` 喂一个主视角排第三的
fixture，断言输出首位是主视角。

---

## 11. 去掉「YOUR.GG · 韩服样本」黄色提示条

`web/champions.js:739`：

```js
body = `<div class="arena-source-notice"><b>!</b><span>YOUR.GG · 韩服样本。该数据不代表当前客户端所在服务器。</span></div><div class="match-list arena-match-list" data-arena-match-list="pros"></div>`;
```

删掉 `<div class="arena-source-notice">…</div>`，只留 `match-list`。
CSS `web/champions.css:596-597` `.arena-source-notice` 一并删。

**保留** `web/champions.js:746-748` 的 `.arena-first-degraded`（失败/降级提示），
那个是必要的容错反馈，不是常驻横幅。

> 合规提醒：来源标注全删干净后，「高手对局」的数据出处在界面上就没有任何说明了。
> 若要保留一丝可追溯性，建议把来源收进「高手对局」tab 按钮的 `data-tooltip` 里，
> 而不是完全抹掉。这条由你定。

---

## 12. 一位战绩详情打不开 —— 根因已定位

### 为什么打不开

**不是 bug，是当前代码主动禁用的。**

`web/champions.js:759-764`：

```js
matchCards.mount(container, {
  key: `arena-pros-${state.selected?.championId}`,
  matches: state.arenaFirstPlaces?.matches || [], region: "kr",
  disableExpand: true, disableReplay: true,
  expandDisabledReason: "YOUR.GG 不提供完整队友明细",
  …
```

→ `web/gameplay.js:2677` 存进 `tab.matchCardOptions` → `:1150 const canExpand = !matchCardOptions.disableExpand;`
→ `:1222` 展开按钮渲染成 `disabled`。

**为什么当初这么写**：`top-builds` 接口对**队友只回身份字段**，没有战斗数据。
`yourgg_arena.go:53-62`：

```go
type yourGGArenaParticipant struct {
    ChampionKey, RiotIDGameName, RiotIDTagline, SummonerID string
    SubteamID int64; SubteamPlacement int; IsBot bool
}
```

KDA / 伤害 / 装备 / 海克斯 **只有 `Me` 有**（`yourgg_arena.go:64-81` 的 `yourGGArenaSubject`）。
所以展开后详情表格会是 20 行空白 —— 这也是 `convertYourGGArenaMatch`（`:247` 起）
只给 `Me` 填了统计、其余参与者走 `yourGGGameplayParticipant` 空壳的原因。

**图三能显示完整数据，说明 YOUR.GG 前端调的是另一个「对局详情」接口，不是 `top-builds`。**
`top-builds` 的裁剪是上游行为，本地怎么解析都变不出数据来。

### 建议方案 A（推荐）：展开时用 Riot 官方 API 补全

项目里这条路**已经通了大半**：

- `riotProvider.matchByID()`（`riot_api.go:474-497`）已实现，打
  `asia.api.riotgames.com/lol/match/v5/matches/{matchID}`，自带 LRU 进程缓存（`riotMatchCacheMax`）。
- 韩服 matchID 格式 `KR_<gameId>`，而 `convertYourGGArenaMatch` 已经把 `raw.MatchID` 存进
  `gameplayMatch.GameID`（`yourgg_arena.go:252`）——拼一下就是 `"KR_" + strconv.FormatInt(match.GameID, 10)`。
- **match-v5 对斗魂是完整支持的**：`riot_api.go:371-380` 已经声明了
  `playerSubteamId` / `subteamPlacement` / `playerAugment1..6`，
  `:602-624` 的转换器已经在填 `SubteamID` / `Placement` / `AugmentIDs`。
  也就是说**转换逻辑现成，不用新写**。
- API Key 是构建期注入（`riot_api.go:39-53`），与用户所在服无关，国服用户同样可用。

**落地步骤**：

1. 新增 `GET /api/champions/arena/match/{matchId}`（`main.go:278` 附近注册，走 `a.authorized`）。
2. handler 校验 `matchId` 必须匹配 `^KR_\d+$`（**白名单正则，防 SSRF / 路径注入**），
   调 `matchByID` → 复用现有 Riot→`gameplayMatch` 转换器 → 返回单个 `gameplayMatch`。
3. 前端：`mountExternalMatchCards`（`gameplay.js:2662`）的展开回调里，
   若 `match.participants` 少于 2 条有效统计，则先 `fetch` 补全、替换 `matches[i]`、再 `view.render()`；
   把 `disableExpand: true` 从 `champions.js:762` 去掉。
4. 展开后的排版**不用另写**——`renderMatch` 的展开分支对斗魂已有分组逻辑
   （`gameplay.js:1397` 按 `subteamId` 分组），天然与总览一致，满足「和总览里面的详情展示一样」。

**必须守住的红线**：

- match-v5 会回 `puuid` 与 `riotIdGameName#tag`。**puuid 一律不得出前端**，
  必须走既有的会话内 `player_<hex>` 映射（与总览同一套）。
- 该响应**不要落盘**。参照 `championCachePolicy` 对 `yourGGArenaHost` 的处理
  （`champion_cache.go:335-337` 返回 `(20min, 0, false)`），Riot 对局详情走
  `riotProvider` 自己的**内存** LRU 即可，不要接进 `championDataCache` 的磁盘层。
- **按需拉取**，只在用户点击展开时请求单场；不要在渲染 5 张卡时预拉 5 场，
  否则很容易撞 Riot 个人 key 的 100 req/2min 限额。
- 失败时保持 fail-closed：展开按钮回到 disabled + tooltip 说明，不要渲染半张空表。

**埋点**：新增 `arena_match_detail` 事件，字段限
`{event, source:"riot", status, duration_ms, cache:"hit|miss", outcome, errorKind}`，
并照 `champion_network_test.go:223` 的写法补一条**字段白名单测试**。

### 方案 B（不推荐）：继续爬 YOUR.GG 的详情接口

需要逆向出 `top-builds` 之外的详情端点，无契约、随时会变，且会把
`summonerId` 这类稳定标识再引进来一次（上一轮刚花了三重措施清理，见
`champion_cache.go:72/253-260/335-337`）。除非方案 A 被 Riot 配额堵死，否则不要走。

---

## 13. 玩家列表：加名次 + 加玩家名 + 小窗口宽度对齐

### 现状

`renderMatchPlayers()`（`web/gameplay.js:1089-1098`）：

```js
const rows = grouping.groups.slice(0, 4).map((group) =>
  `<div class="arena-team-row-compact${group.placement === 1 ? " is-first" : ""}" data-tooltip="…">
     <span class="arena-team-roster">${group.players.slice(0, 3).map(playerButton).join("")}</span>
   </div>`).join("");
```

三个问题：

1. **没有名次前缀** —— 名次只存在于 `data-tooltip`（`arenaGroupLabel`，`:1082-1085`），要悬停才看得到。
2. **只显示前 4 队** —— `slice(0, 4)`，斗魂 6~7 队会被截断（图二正好只有 4 行）。
3. **玩家名被 CSS 隐藏** —— 名字其实**有渲染**（`playerButton` 里的 `.match-player-name`，`:1091`），
   但被 `web/gameplay.css:426` `.match-summary.is-arena .arena-team-roster button span:last-child { display: none; }` 藏了。

### 「小窗口宽度不一样」的根因

断点用的是**匿名 container query**，会命中**最近的**容器祖先：

- 总览页：最近容器是 `.matches-column`（`gameplay.css:301 container-type: inline-size`），宽度≈整个战绩栏。
- 斗魂页：最近容器是 `.arena-first-section`（`champions.css:595 container-type: inline-size`），
  宽度 = 右侧 pane 宽度，两栏布局下约 **537–900px**。

于是同一批规则在两个页面落在完全不同的档位：

| 容器宽 | `gameplay.css` 规则 | 斗魂页实际 |
|---|---|---|
| ≤1080 (`:420`) | 名字列 150–190px → 118–152px；**玩家名 `display:none`**(`:426`) | **恒命中** |
| ≤900 (`:428`) | 名字列 → 96–118px | 两栏布局下基本恒命中 |
| ≤680 (`:434`) | `.match-players` 整块 `display:none`(`:436`) | 单列窄窗时命中 → 玩家列表整个消失 |

这就是「小窗口时和上面不一样宽」以及「玩家名称看不到」的统一根因。

### 建议改法

1. **给容器命名，切断串扰**。`gameplay.css:301` 改成
   `.matches-column { container: matches-column / inline-size; }`，
   所有战绩卡断点改成 `@container matches-column (max-width: …)`。
   同时给 `.arena-first-section`（`champions.css:595`）命名为 `arena-first`，
   并**为斗魂页单独写一组阈值**（建议 640 / 520 / 420，对应 pane 的真实取值域），
   避免拿峡谷战绩栏的 1080/900/680 去量一个最宽才 900 的 pane。
2. **名次前缀**：`renderMatchPlayers` 每行前面插
   `<b class="arena-rank-chip">${group.placement}</b>`。
   样式建议复用页面已有的语言而不是新造：参照
   `web/champions.css:547` 的左边条 + `web/gameplay.css:391` 的 `is-first`（`box-shadow: inset 2px 0 0 var(--primary)`）。
   一档配色建议：第 1 名 `var(--primary)` 实心 + 深色字；2/3 名 `var(--surface-strong)` + `var(--ink)`；
   4 名及以后 `transparent` + `var(--muted)` + 1px `var(--line)` 边框。
   尺寸 18×18、`border-radius: 5px`、`font-size: 9.5px`、`font-weight: 800`、`tabular-nums`。
   `.arena-team-row-compact` 的 `grid-template-columns` 由 `minmax(0,1fr)` 改为
   `18px minmax(0,1fr)`。
3. **玩家名常显**：删掉 `gameplay.css:426`。名字省略号已经有了
   （`:394 .arena-team-roster button span:last-child { overflow:hidden; text-overflow:ellipsis; white-space:nowrap; }`），
   且 `playerButton` 已带 `data-tooltip-overflow=".match-player-name"`，溢出才弹 tooltip，行为正确。
   为了给名字腾地方，建议在斗魂分支把每行从 3 列改成 `repeat(3, minmax(0,1fr))` 保持，
   但把整个玩家列列宽从 `minmax(150px,190px)`（`gameplay.css:305`）提到 `minmax(190px,240px)`。
4. **别再截断队伍**：`slice(0, 4)` → 全量（斗魂最多 7 队），
   或至少 `slice(0, 6)`。若担心卡片变高，改成前 4 队常显 + 其余折叠。
5. **≤最窄档不要整块隐藏**：`gameplay.css:436` 的 `display: none` 对斗魂页太激进
   （用户会觉得「数据丢了」）。建议斗魂分支改为只保留名次 + 头像、隐藏名字，
   而不是整块消失。

---

## 14. 玩家列表右侧数据列英文名改中文

`web/gameplay.js:1185-1186`：

```js
const arenaDamageRow = `… <em>Damage</em><b class="match-damage-value">${number(subject.damage)}</b></span>`;
const arenaTakenRow  = `… <em>DT</em><b class="match-taken-value">${number(subject.damageTaken)}</b></span>`;
```

改为 `<em>伤害</em>` / `<em>承伤</em>`（`data-tooltip` 里已经是中文「对英雄伤害」「承受伤害」，
保持不变即可）。

若第 12 条落地后展开详情里也出现 `Damage / DT / Gold` 列头（图三那种），
同步改成 `伤害 / 承伤 / 经济`。

**注意**：`<em>` 的宽度是按等宽英文估的，换成 2 个汉字后要检查
`.match-stat-damage` 的列宽是否需要从当前值放宽 ~6px，否则会挤压数字。

---

## 一并建议的回归测试

1. `web/champions.test.cjs` 新增：源码中不得再出现 `一位率`、`OP.GG`、`arena-source-notice`、
   `技能加点`。（正则型断言按 [[deep-legends-mutation-testing]] 的要求，**先做变异验证**再提交。）
2. `champions_test.go` 新增：`parseArenaTeamCompositions` 主视角前移断言（第 10 条）。
3. `yourgg_arena_test.go` / 新的 riot 详情测试：matchID 白名单正则拒绝
   `KR_1/../x`、`NA1_123`、`KR_` 等；响应不含 `puuid`。
4. 主题矩阵：对 5 套主题各断言 `.arena-option-card.is-gold` 与 `.is-prismatic` 的
   解析色不相等（第 6 条）。

## 仍未验证

- `go vet ./...` / `go test -race ./...` 沙箱无 Go 工具链，需在本机跑。
- 第 6 条的「换主题串色」需在紫色/蓝色主题下实机确认。
- 第 13 条的 container 归属是静态推导（`champions.css:595` + `gameplay.css:420` 均为匿名容器），
  建议实机用 DevTools 选中 `.match-summary` 看 `@container` 命中的是哪一档以确认。
