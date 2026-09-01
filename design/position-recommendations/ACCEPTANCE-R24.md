# R24 验收报告 + 返工工单

验收时间 2026-08-26。方案原文 `design/position-recommendations/PLAN.md`。
所有结论要么是读码确认，要么是真机（Chromium / go test）实测，逐条注明证据。

---

## 一、通过的部分

**测试**：`go vet ./...` 干净；`go test -race -count=1 ./...` 绿；
`node --test web/*.test.cjs desktop/*.test.cjs` **154/154 绿**。

**Go 侧规格逐条落地**（PLAN 第 2 节）：`Summary.Positions` 解析（`champions_structured.go:98-105`）、
`role_rate` 字段（`champions.go:1442`）、`championPositionOption`（`champions.go:349-358`）、
`ratePercent` 统一 ×100 且幂等无双重放大、只在 `opggPositionRequired` 时填充（无 `mode == "ranked"` 硬编码）、
白名单 fail-closed、保留上游顺序、`opggTierRank` 抽成共用函数（防漂移要求满足）、
段位回退时 positions 跟随实际数据。**缓存键 `opggDetailCacheKey` 一字未动。**
新增字段无 PUUID 泄漏路径，无新增 map / 全局状态，无并发问题。

**对局页 Go 侧**：`normalizeOPGGPosition` 的 `case "", "other"` 已从 `"mid"` 改为返回空串；
`loadClientRecommendation` 的 `"middle"` 硬编码已改为可见失败态；
`positionSource` 三值（client / smite / role-rate）各分支标注正确；
position 允许传空走推断；MID 探测无死循环、推断出 mid 时复用探测结果不重复请求。

**真机实测：切换分路确实生效**（这是最重要的一条）。卡蜜儿 上单 → 辅助：

| | 副标题 | 胜率 | 选用率 | 禁用率 | 选中 chip |
|---|---|---|---|---|---|
| 切换前 | 韩服·翡翠以上·上单 | 50.24% | 6.68% | 11.35% | 上单 50.24% · 占87.42% |
| 切换后 | 韩服·翡翠以上·辅助 | 47.49% | 5.48% | 24.12% | 辅助 47.49% · 占56.99% |

hero 保留、不整页重绘、四格跟着分路走（老 bug「四格读榜单行」已修）。

---

## 二、P0（三条，必须修）

### P0-1 分路按钮图标永远是灰的，选中态看不出来

`champions.css:64` 是**全局**规则：

```css
.position-icon img { ... opacity: .68; filter: grayscale(.7); }
```

复原它的只有三处：`.champion-position-tabs button:hover`（:65）、
`.champion-position-tabs button.is-active`（:66）、`.position-pill`（:106）。
**`.champion-detail-positions` 一条都没有**（:427 只改了尺寸）。

真机 computed style 实测（4 个视口 × 3 个多分路英雄，全部一致）：

```
activeIconStyle   : "0.68 / grayscale(0.7)"
inactiveIconStyle : "0.68 / grayscale(0.7)"     ← 完全相同
```

截图 `hero-jax-1440.png` 里能直接看到选中的「上单」图标和其它三个一样灰。

**改法**：补两条，与 `.champion-position-tabs` 对齐

```css
.champion-detail-positions button:hover .position-icon img { opacity: .88; filter: grayscale(.25); }
.champion-detail-positions button.is-active .position-icon img { opacity: 1; filter: none; }
```

**护栏**：不要只断言「规则存在」——那正是符文令牌那次翻车的写法。
要断言最终结果：active 的 `filter` 计算值 !== inactive 的 `filter` 计算值。

---

### P0-2 从克制关系点进不在榜单里的英雄 → 详情读取失败（本轮引入的回归）

真机复现，点击克制列表里的诺手（catalog 里有、当前榜单里没有）发出的请求是：

```
/api/champions/detail?mode=ranked&champion=darius&position=&tier=emerald_plus
                                                    ^^^^^^^^^ 空
```

后端 `champions.go:708`：`championPositionNames[""]` 取到零值 `""` → **400**。
界面结果：正文「详情读取失败」，副标题「韩服 · 翡翠以上 · **位置未知**」。

根因链：`openCounterDetail`（`champions.js:712`）改成了
`position: firstPositionOf(ranked)`，而 `ranked = rankingRows().find(...) || {}`（:711）。
英雄不在当前榜单（榜单被 `state.position` 过滤过，或本来就不在返回集合里）时 `ranked = {}`，
`firstPositionOf({})` → `""`。往下 `openDetail:403` 的整条兜底链
`state.detailPosition || row.position || state.position(非all) || firstPositionOf(row)`
全落空，最终发空位置。

改之前用的是 `state.selected?.position`，恒非空——**所以这是本轮新引入的失败路径**。

**改法**：`firstPositionOf` 全空时再兜一档。建议顺序
`firstPositionOf(ranked) || state.selected?.position || "mid"`，
并且在 `openDetail` 里加一道**前端 fail-closed**：位置为空就不发请求，直接走「请先选择分路」的可见态，
而不是让后端回 400 再显示一个看不懂的「详情读取失败」。

**护栏**：变异测试实测这处**零护栏**——把 `firstPositionOf(ranked)` 改回
`state.selected?.position`，154 个测试没有一个挂。必须补。

---

### P0-3 窄窗口分路按钮文字截断

Playwright `scrollWidth > clientWidth` 审计（4 视口 × 4 英雄）：

| 视口 | 1 分路 | 2 分路 | 3 分路 | 4 分路 |
|---|---|---|---|---|
| 1440 | ✅ | ✅ | ✅ | ✅ |
| 1260 | ✅ | ✅ | ✅ | ✅ |
| 1080 | ✅ | ✅ | ✅ | ❌ 87/96 `占87.…` |
| 900 | ✅ | ✅ | ❌ 81/96 | ❌ **45/96** `50.24…` |

900px 下 4 分路只剩 45px 可用、需要 96px，**占比数字整个消失**（见 `hero-jax-900.png`）。
这就是之前 KDA 被挤成 `2.1…` 的同一类问题。

**改法**（任选，建议第一个）：窄断点下把 `小` 那行从「胜率 · 占比」降级为只留胜率，
或者 `.champion-detail-positions` 改 `overflow-x: auto`（对局页的 `.live-position-switch`
`gameplay.css:909` 就是这么做的，更稳），再或者 ≤1080 时隐藏中文名只留图标 + tooltip。
**不要靠缩字号硬塞**。

顺带说明：900px 下四格丢「禁用率」一整格是 hero `overflow:hidden` 裁掉的，
**预先存在、不是本轮回归**（单分路的莫甘娜同样丢），可另开工单。

---

## 三、P1 护栏缺口（变异测试实证）

Go 变异 **6 杀 7 活**，JS 变异 **6 杀 2 活**。存活的都是假护栏：

### 3.1 `handleGameplayRecommendations` 完全没有 HTTP 层测试

grep 全仓，这个 handler 只在 `main.go:289` 的注册处出现过，没有任何测试构造请求打它。
以下变异体**全部存活**：

| 变异 | 后果 |
|---|---|
| `needsPositionInference = position == ""` → `= false` | 盲选局直接退回 400 |
| `bundle.PositionSource = positionSource` 整行删掉 | 退回硬编码 `"client"`，**「自动」角标静默消失** |
| `probePosition := "mid"` → `"top"` | 探测位改了没人知道 |
| handler 里惩戒那层 `if` 短路 | 惩戒判打野失效 |

第二条正是 PLAN 4.4 专门点名要防的 fail-visible 翻车模式。
纯函数 `resolveGameplayRecommendationPosition` 是被测的（惩戒判定、smite/role-rate 标注三个变异体都被杀），
**但 handler 这一层是裸的**。补一个 `httptest` 级测试即可全部覆盖。

### 3.2 `TestStructuredOPGGDetailSchema` 把生产逻辑抄进了测试

`champion_network_test.go:550-561` 没有调 `loadStructuredDetail`，而是在测试里
**原样重写了一遍** `champions_structured.go:606-626` 的过滤 + `ratePercent` 循环，然后断言自己的副本。
自己测自己。

后果实测：把生产代码里 `WinRate: ratePercent(raw.Stats.WinRate)` 改成
`WinRate: raw.Stats.WinRate`（分路胜率变成 0.5024），**没有任何测试挂**。
分路胜率恰恰是用户在按钮上直接读的数字。

真护栏在 `TestStructuredDetailFallbackKeepsFallbackPositions`(:600-619)——它走真 `loadDetail`，
断言了保序 / RoleRate ×100 / 白名单 / role_rate tag（G1/G3/G4 都被它杀掉）。
**建议：删掉 :550-561 那段抄来的循环，把 WinRate/PickRate/BanRate 的断言并进 :600 那个真路径测试。**

### 3.3 其余存活变异体

- 基础 `min-height: 176px` 改成 300px 无人拦（只断言了 `.has-positions` 的 208）
- `openCounterDetail` 零护栏（即 P0-2）
- 非 ranked 模式的 positions 只覆盖了 `opggPositionOmitted`（arena / 海斗），
  `opggPositionLiteralNone`（aram / urf / 极限闪击）没覆盖

---

## 四、P1 功能缺口（读码确认，未真机验）

1. **`positions.length === 1` 时对局页切换条整个不渲染**（`gameplay.js:2809`），
   连「自动」角标一起消失。位置是推断出来的时候用户完全看不出来——fail-visible 被 `length > 1` 吃掉了。
2. **`state.livePositionOverride` 从不清理**。PLAN 3.4 要求「当前这一局内锁定」，
   现在下一局同英雄同模式同地图仍沿用上一局的手动选择，且界面无提示。
   建议 gameId 变化或 phase 迁出 ChampSelect 时 clear。
3. **`selectLivePosition`（`gameplay.js:2398-2399`）删的是旧 key**。
   `target` 在 set override 之前算的，所以删的是旧 position 的缓存和失败记录。
   后果：切走再切回白丢缓存；新位置若 60s 内失败过，退避仍生效、点了没反应。
4. **前端缓存键不含 spell**（`gameplay.js:2380`）。盲选下 position 恒为 `"other"`，
   所以选人阶段中途换上惩戒不会重算，惩戒判打野只在第一次生效。
5. `gameplay.js:2798` 的对线样本空态文案仍用 `self?.position`，没换成 `resolvedPosition`，
   位置是推断出来的时候 tooltip 会说错位置。

---

## 五、P2 清理项

- `gameplay.css:914` `.live-position-chip .position-icon img` 是死规则——
  gameplay.js 的 `positionIcon`（:3281）返回的是**裸 `<img class="position-icon">`**，没有嵌套 img。
  连带 `object-fit: contain` 丢失（对比 `gameplay.css:463` 同一元素是有的）。
- `champions_structured.go:611` 的 `!strings.EqualFold(...)` 是恒真死条件
  （`championPositionNames` 的 key 恒等于 value 的小写）。
- `champions.css:641-646` 的 1260px 断点里三条规则与基础规则完全相同，纯冗余。
- `gameplay.go:2455` 对 ranked 是死赋值（被 :2464 立刻覆盖）；
  `:2600` 的 `PositionSource` 赋值被 `:2518` 覆盖；惩戒判断在 `:2486` 和 `:2573` 重复实现两份。
- `champions.js:400/404/408` 三处 mode 条件恒真/恒假（arena / 海斗在 :378-385 已提前 return）。
- `state.detailCache` 从不清理，会随会话增长（键含 tier/position，不会串味，只是内存）。
- `loadRanked` 的 `Positions`（`champions.go:1477`）没过白名单。字段是旧的，
  但本轮前端才开始把它当默认位置用（`firstPositionOf`），fail-closed 缺口变成可达路径。
- `resolveGameplayRecommendationPosition:2576` 取的是 `positions[0]` 而非遍历比最大 `RoleRate`，
  但标签叫 `role-rate`。符合 PLAN 2.2 的定案（依赖上游 play 降序），
  只是上游哪天改排序会静默选错且无断言拦得住。建议补一个「role_rate 高的排在数组第二位」的用例。

---

## 六、高度核算的更正

PLAN 第 206 行算的 208px 前提是头像在第 1 行，但实现把头像改成了 `grid-row: 1/-1` 跨两行，
算式不再成立。真机实测：

| | min-height | 实际 hero 高 | 实际内容高 |
|---|---|---|---|
| 有分路条 | 208px | 208 | 201 |
| 无分路条 | 176px | 176 | 162 |

差 7px / 14px，没到「撑出大片空白」的程度，不影响使用，
但严格说不满足 PLAN 强调的「容器高度 = 内容高度硬等式」。低优先级，可与 P0-3 的窄屏改动一并调。

---

## 七、返工建议顺序

1. P0-1（一条 CSS + 一个结果型断言）—— 5 分钟的事
2. P0-2（兜底链 + 前端 fail-closed + 补护栏）
3. P0-3（窄屏截断，建议直接改 `overflow-x: auto` 与对局页对齐）
4. 3.1 补 `httptest` 级 handler 测试（一个测试覆盖四个存活变异体）
5. 3.2 删掉抄来的测试循环，断言并进真路径测试
6. 四、五两节按优先级排

**每补一条护栏都要先植入变异体确认它真的挂**，
这轮已经证明「测试全绿」和「护栏是真的」是两回事。

---

## 八、追加工单（2026-08-26 用户复核实机截图后提出）

用户看了真机截图后反馈两点布局问题，我又追加核实了一条数据问题。

### D. 分路按钮位置改错了，且现在太高

用户反馈：不要把分路按钮放在英雄名称下面（现状），改到右侧「梯度/胜率/选用率/禁用率」
四格下面，这样 hero 高度基本不用增加，或者只增加一点。

现状确实是当初 AskUserQuestion 里 A 选项（hero 内新增第二行，跨标题列+指标列）的实现，
用户当时选的是这个，现在真机看过截图后改主意，选回了 B 选项的方向（右侧四格下方）。
**这是合理的：** 我当初否决 B 选项的理由是"四个分路时每格只剩 78px 装不下文字会截断"——
这个顾虑成立的前提是按钮被强制排成与四格同款的 4 列网格。**只要允许按钮换行
（`flex-wrap: wrap`），这个顾虑就不成立了**：4 个分路可以排成 2×2，不需要挤在一行。

**改法**：

1. 把 `.champion-detail-hero` 改回**单行网格**（不需要 `grid-template-rows: auto auto`
   那个额外行）：`grid-template-columns: 104px minmax(220px,1fr) auto`，头像/标题各占一列。
2. 第三列（原来只放四格指标）改成一个纵向容器：**四格指标在上，分路按钮组在下**，
   两者垂直堆叠、共享同一列宽度、右对齐。按钮组 `display: flex; flex-wrap: wrap;
   justify-content: flex-end; gap: 6px`，允许换行成 2 行。
3. **删除 `.champion-detail-hero.has-positions { min-height: 208px }` 这条**——
   不再需要靠增加 hero 整体高度来装按钮，第三列自己变高、其它两列靠 `align-items` 居中即可。
4. 验收标准（真机量，不是估算）：4 分路（贾克斯类）情况下 hero 总高度相对**当前无分路条的
   基线（176px）增量不超过 ~50px**（即按钮两行 + 间距的实际高度，而不是现在的整行 32px
   固定溢出）；1 分路（不显示按钮）时 hero 高度应与基线一致，不应该因为改了网格结构而跑偏。
5. 沿用现有的 `hasPositions` 判断（`champions.js:1360`）来决定要不要渲染这个按钮组，
   逻辑不用改，只改 CSS 放置位置和 JS 里外层容器的 class/结构。

**护栏**：截图审计要覆盖 1/2/3/4 分路四种英雄在 1440/1080/900 三档视口，
断言"分路按钮不再出现在标题下方独占一行"（比如断言 `.champion-detail-positions` 的
`getBoundingClientRect().left` 落在第三列范围内，而不是横跨标题列）。

### E. 按钮里的胜率/占比颜色要复用全局配色，不要用灰色

现状 `champions.css:431` `.champion-detail-positions button small { color: var(--muted); }`，
把胜率和占比（选用占比）都渲染成同一种灰色。而项目其它地方（榜单表格、符文卡、技能卡）
都遵循同一套约定：胜率用 `--success`（绿）、选用率/占比类用 `--accent`（蓝），
见 `champions.css:100-102` 那三行共享规则（`.champion-table td.metric-win` /
`.metric-pick` / `.metric-ban`，被榜单表格、符文页卡片、召唤师技能卡、技能加点卡多处复用）。

**改法**：

1. `champions.js:1367` 生成按钮文案时，把 `<small>${percent(item.winRate)} · 占${percent(item.roleRate)}</small>`
   拆成两个带语义 class 的 `<span>`：胜率用 `class="metric-win"`，占比用 `class="metric-pick"`
   （占比在语义上等价于"这个位置有多少场次"，与选用率同一类蓝色，不要新发明第三种颜色）。
2. CSS 层**不要新写死颜色值**，而是把 `.champion-detail-positions` 加进
   `champions.css:100-102` 那条既有共享规则的选择器列表里
   （例如 `.champion-detail-positions .metric-win` / `.metric-pick`），
   这样以后主题色改动这两处会自动同步，不会再漂出第三份颜色定义。
3. 保留 `strong`（分路中文名）不上色，只有数字部分变色，这样与四格指标卡片的视觉层级一致
   （四格是"标签灰 + 数字彩色"，按钮也应该是"分路名默认色 + 数字彩色"）。

**护栏**：断言 active/inactive 按钮里 `.metric-win` 的 computed color 等于
`.champion-detail-metrics .metric-win strong` 的 computed color（同一个变量），
而不是新写一条独立断言——这样以后如果主题色变量改名，两处会一起挂，不会只改一处。

### F. 召唤师技能"只有一个"——已用真实上游数据核实：不是缺陷，是真实数据

用户反馈：有些英雄召唤师技能只推荐了一套（如截图里的打野贾克斯），看着很空，
以为是数据没取全，要求补全。

**我直接查了 op.gg 后端 JSON 接口（`lol-api-champion.op.gg`，就是我们自己在用的那个上游）
核实过了：这不是缺陷，是真实数据。**

```
GET /api/KR/champions/ranked/24/JUNGLE?tier=emerald_plus   ← 贾克斯·打野
summoner_spells: 5 行，只有 1 行 play≥50：
  [闪现,惩戒] play=6078 pick_rate=99.64%
  其余 4 行 play 分别是 18 / 2 / 1 / 1 —— 全是噪声，远低于我们代码里
  structuredMetricMinimumGames=50 的门槛（champions_structured.go:830）
```

这个门槛是 R13 那轮专门加上去、用来**防止把 1~2 场的噪声组合当成"推荐"展示给用户**的
（历史教训见 memory「验证盲区」案例三：R13 之前就因为没有这个门槛，出现过展示极小样本
数据的问题）。贾克斯打野惩戒位受限，全服 99.64% 的人都是闪现+惩戒，本来就没有第二套
主流搭配可推荐——**op.gg 网页本身对这个英雄这个位置也只会有一套数据**（我们和它用的是
同一个上游接口）。

**对照组（证明门槛没有误杀真实数据）**：

| 英雄·位置 | ≥50 场的真实组合数 |
|---|---|
| 卡蜜儿·上单 | 4 |
| 贾克斯·上单 | 5 |
| 亚索·中单 | 5 |
| 盖伦·上单 | 5 |

打野位置样本天然稀疏是因为**惩戒是硬约束**（所有召唤师技能组合必带惩戒），
组合空间本来就比不带位置限制的上单/中单窄很多。这不是我们代码的问题。

**所以不要"补全数据"**——放宽 `structuredMetricMinimumGames` 门槛会让 1~2 场的噪声重新
混进来，是倒退。真正该做的是**纯视觉打磨**：只有一套组合时，卡片右侧留白显得像是"缺了一块"。

**改法**：`web/champions.js:1493` `renderSpellsCard`，`rows.length === 1` 时，
让这一张卡片撑满整行（不再对半分两栏），或者居中显示，避免视觉上像是缺了数据。
不需要改任何 Go 侧代码或抓取逻辑。

**护栏**：补一个用例——只传 1 个 summonerSpells 条目时，断言渲染结果里
没有第二个空的 `.spell-option` 占位元素，且第一个元素的宽度/布局 class 与"两套时"不同
（证明确实走了单套分支，不是巧合看起来还行）。
