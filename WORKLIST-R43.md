# WORKLIST-R43（给 GPT 执行）

> 这轮问题很多，但**有两条主线根因**，先说清楚，避免逐条打补丁：
>
> **主线一：绝活哥的四个症状（只出1人 / 每人只1条 / 位置给错 / 对手段位又没了）是同一个根因** ——
> 30 局详情串行扫描把 10 秒预算烧光，后面的玩家和对手段位查询全被超时吃掉。
> 单点解法是给 `matchIDs` 加队列过滤 + 三个玩家并发，一改解四个症状。
>
> **主线二：非峡谷模式（斗魂/海斗）的区块显隐，后端其实早就有能力位，只是没下发** ——
> `champions_structured.go:150-157` 的 `opggModeSpecs` 里 `HasCounters` 只有 ranked/nexus-blitz 为 true，
> 但 `gameplayRecommendationBundle` 根本没把这个字段传给前端，所以前端只能无条件渲染。
> **绝对不要在前端写 `source === "arena"` 这种模式白名单硬编码——这个套路在本项目已经翻车四次。**
>
> 另外日志这轮又被两个事件刷爆了（97%），见 H 组，这条不修下轮还是拿不到有用日志。

---

## A 组（P0）：绝活哥四个症状的单一根因

### A-1 量化事实（先看这个，再看改法）

`specialist_runes.go:14-23` 的预算：
```go
specialistRunePlayerLimit    = 3   // 榜单取几个玩家（3 名绝活哥），跟场次无关
specialistRuneMatchScanMax   = 30  // 每个玩家最多扫描几场对局去找位置匹配的那一局——这个数不能改小
specialistRuneRequestBudget  = 3 * (2 + 30)   // = 96，这里的 "3" 是 3 个玩家，不是 3 场
specialistRuneRequestTimeout = 10 * time.Second
```

> ⚠️ **术语澄清，避免实现时读串**：本组下文出现的三个数字含义不同——
> `specialistRunePlayerLimit=3` 是"取几个绝活哥玩家"；`specialistRuneMatchScanMax=30`
> 是"每个玩家最多扫描几场对局找位置匹配"，**这个 30 是安全上限，A-2 不要求也不允许改小**；
> A-3 说的"每个绝活哥展示 2~3 条"是"扫描到匹配对局之后，最多挑几条给符文页展示"，
> 是**扫描完之后的展示层筛选**，和扫描上限是两件不同的事，不要混为一谈。

- `loadSpecialistRunes`（`specialist_runes.go:187-263`）是**单 goroutine 全串行**：外层逐个玩家，内层逐场 `matchByID`，零并发。
- 每个玩家最多 32 次 Riot HTTP（1 account + 1 matchIDs + 30 details），三人共 **96 次串行往返**。
- Riot 限速器 `riot_api.go:211-248`：`shortLimit=15/s`、`longLimit=90/2min`。**96 > 90**，单次绝活哥请求本身就会撞 2 分钟长窗口硬阻塞。
- 10 秒超时只够跑完约 25~33 次串行往返 = **恰好一个玩家**。

所以 R40 真机实测的 26~40 秒、以及这次"只有一个绝活哥"，是同一件事。

### A-2 改法（这一组是主要工作量，请按顺序做）

1. **给 `matchIDs` 加队列过滤**（最高性价比，一改解四个症状）。
   `riot_api.go:526-531` 目前只传 `start`/`count`。Riot match-v5 支持 `queue={queueId}` 和 `type=ranked`。
   绝活哥榜单本来就是排位榜，加上队列过滤后最近 30 场里绝大多数直接是排位单双，
   **`specialistRuneMatchScanMax=30` 这个安全上限本身不动**，但因为过滤掉了大乱斗/匹配这些杂质，
   位置命中率大幅提升，实际往往扫到 8~10 场就能提前收集够 A-3 要求的 2~3 条匹配位置的对局并 break，
   不用真的扫满 30 场，平均请求量因此从 96 降到 ~30（**这是"平均提前退出"，不是把 30 改小**）。
   **顺带解决 A-4 的位置错乱**（见下）。
2. **三个玩家改成并发**：每人一个 goroutine，共享现有限速器，串行 96 次 → 并发 3 路。
3. **超时从 10s 放宽到 25s**，同时**前端 `web/gameplay.js:3065` 的 12000ms abort 要同步放宽**，
   否则服务端做完了客户端已经断开（这是个容易漏的联动点）。
4. **超时的部分结果也要缓存**（短 TTL 即可）。现在 `specialist_runes.go:153`
   `finishSpecialistRuneFlight(..., ctx.Err() == nil)` 在超时时 `cache=false`（`:160` 不入缓存），
   下次请求全部重跑，继续烧 90/2min 配额，**越用越慢**。

### A-3 每个绝活哥要返回 2~3 条符文（现在硬性只有 1 条）

`specialist_runes.go:253-257` 找到第一条匹配就 `break`，**设计上每人上限就是 1 条**；
`result` 容量也是按 `len(players)` 分配的（`:175`）。

- 改成计数器：每个玩家收集到 2~3 条**匹配该位置**的对局后再 break。
- `result` 容量改 `len(players) * N`，`specialistRuneRequestBudget` 相应放宽。
- **前端零改动**：`renderSpecialistPlayers`（`web/gameplay.js:3486-3532`）本来就按 `playerName` 分组、
  `games.map(...)` 渲染该玩家全部对局行，现有测试 `web/champions.test.cjs:3466` 甚至断言过一个玩家 4 行。
- `Key` 用的是 `len(result)`（`:239` 传 index，`:481` `"specialist-"+index`），多条时天然不重复，无需改。

### A-4 位置给错（选辅助推上单）—— 删掉"硬凑"兜底

**位置词表本身是对的**，我逐个核对过没翻车：`normalizeOPGGPosition`（`gameplay.go:3450-3465`）、
`canonicalSpecialistPosition`（`specialist_runes.go:66-77`）、`riotPositionKey`（`riot_api.go:583-601`）
三者词表一致，`:253` 的比较是真的在跑。**问题纯粹出在找不到时不肯放弃**：

```go
// 249-252：不管位置对不对，先把最新一场记下来
if latestRune == nil { candidate := rune; latestRune = &candidate }
// 253-257：位置对得上才 append
// 260-262：30 场扫完位置都没对上 → 把位置不匹配的那条塞进去
if latestRune != nil { appendRune(*latestRune) }
```
同样的兜底还在 `:213-216`（预算耗尽）和 `:222-225`（ctx 超时）两处。

**加重因素**：`matchIDs` 没有队列过滤，最近 30 场混着大乱斗/斗魂/匹配。
ARAM 的 `individualPosition` 是 `"Invalid"`，`riotPositionKey` 落到 default 返回 `""`，
永远不等于 `"support"` → 这局成为 `latestRune` 候选。
**所以用户看到的"上单符文"很可能压根不是上单局，而是一局没有位置信息的大乱斗被兜底顶上来了**
（UI 里 `position` 为空所以不显示位置标签，`web/gameplay.js:3512-3513`）。

改法（用户明确要求"找不到就提示，不要硬凑"）：
1. **删掉 `latestRune` 整套兜底**（`:210, 213-215, 222-224, 249-252, 260-262`），位置不匹配就不返回。
2. 后端返回可区分的原因（例如 `reason:"no-position-sample"`），
   前端 `web/gameplay.js:3473` 的 `unavailableReason.specialist` 文案改成
   **"该绝活哥最近 30 局没有打过{位置}"**，而不是现在笼统的"最近对局中没有找到完整且可核验的该英雄符文"。
3. 配合 A-2-1 的队列过滤，从源头剔掉无位置的模式。

### A-5 对手段位胜率又没了（R41 刚做完，Go 测试全绿但真机为空）

**先排除两个你可能会怀疑的点，都不是问题**：
- `main.go:312` 传 `nil` client：`rank_insights.go:171-184` 的 `useRiot = serverID=="KR" && a.riot != nil && validPlayerReference(playerRef)` 成立，
  **根本走不到需要 client 的分支**，nil 从头到尾没被解引用。
- `serverID` 写死 `"KR"`：绝活哥榜单本来就是韩服（`champions_structured.go:972` `region=kr`），
  对手也是韩服玩家，写死 KR **恰好把 `useRiot` 打开**，是唯一正确路径。缓存键也隔离干净。

**真根因 A-5-a（主）：排名查询和主扫描共用同一个 10 秒 ctx，且排在最后才启动。**
`specialist_runes.go:181-186`：`opponentRanks.add(...)` 只在 `appendRune` 时才发起，
而 `appendRune` 只在第一个玩家扫到匹配对局时才被调用 —— 真机上大约 t≈9~10s。
此时 `b.ctx` 已所剩无几，`add` 的 goroutine 走 `case <-b.ctx.Done(): return`（`:310-311`），
entries 里什么都没写 → `apply`（`:320-339`）`!ok` 直接 continue →
字段保持零值 → `omitempty`（`gameplay.go:3062-3064`）让字段从 JSON 里消失 → 前端不渲染。

**测试为什么发现不了**：`specialist_runes_test.go:142,184,240,292,489` 全部把 `opponentRankScore`
换成**内存里立即返回**的桩，并且用 `context.Background()`（无超时）。
没有任何测试让排名查询去和限速器抢令牌、或在 deadline 附近执行。
**这是典型的"测了契约没测时序"，请在这轮补上。**

**真根因 A-5-b（次）：`opponentPUUID` 为空。**
`specialist_runes.go:242-248` 只有 `opponent.ChampionID > 0` 才写 `opponentPUUID`；
而 `specialistOpponent`（`:390-417`）在 `positionKey == ""`（ARAM/斗魂/老对局）时
`if position != "" && ...` 永远不成立 → 返回零值 → 对位英雄、对手名、puuid 全空。
按 A-4 删掉无位置局之后这条自动消失。

**改法**：把对手排名查询**从主扫描的 ctx 里解耦**——
用 `context.WithoutCancel(ctx)` 再套自己的 5s deadline，
否则主扫描一超时排名必然全丢。（更稳的方案是做成第二个 HTTP 端点让前端异步补，
就像已有的 `data-match-tier` 那套机制 `web/gameplay.js:1988`；如果工作量允许优先选这个。）

### A-6 符文卡要整块可点（现在只有顶部 48px 能点）

当前 DOM（`web/gameplay.js:3530`）：`<article>` 里只有 `.specialist-game-head` 是 `<button>`
（`web/gameplay.css:1102` `min-height:48px`），`.specialist-game-content`（`:1113` `display:block`）完全在按钮外面。

**障碍**：符文板里 `renderRuneOption`（`:3580`）每个符文图标是一个 `<button class="rune-option-button">`（约 20 个），
`renderItemIcon`/`renderSummonerSpellIcon`（`:3773,3782`）也是 button。
HTML 禁止 button 嵌套 button，**所以不能简单把整个 row 包成 button**。

**关键发现：这些嵌套 button 全都没有任何 click 行为。**
全量 grep 确认 `rune-option-button` / `item-option-button` 在 `gameplay.js` 里没有任何 `addEventListener`，
它们存在的唯一目的是承载 `data-tooltip` + `aria-label`。
而且本仓库已有先例证明 tooltip 不依赖 button 标签：`.arena-augment-icon`（`:3342`）
和 `.item-tooltip`（`:4103`）都是 `<span tabindex="0" data-tooltip=...>`。

**请按这个方案做**：
1. `renderRuneOption` / `renderItemIcon` / `renderSummonerSpellIcon` 把 `<button>` 降级为
   `<span role="img" tabindex="-1" data-tooltip=... aria-label=...>`。它们本来就不可点，**零功能损失**，
   且和 `.arena-augment-icon` 保持一致。CSS `web/gameplay.css:1184` `.rune-option-button` 已是 `display:grid`，span 照样生效。
2. `.specialist-game-row` 从 `<article>` 改成
   `<div class="specialist-game-row" role="radio" aria-checked tabindex="0" data-rune-choice="...">`，
   `.specialist-game-head` 从 `<button>` 降级为 `<div>`（role/aria-checked/data-rune-choice 全部上移到 row）。
   此时 row 内部再无 interactive 元素，DOM 合法。
3. `bindRuneChoiceButtons`（`:3869-3870`）已经是按 `[data-rune-choice]` 绑 click，**自动就能工作**；
   但 div 不会自动响应键盘，**需要补 `keydown` 处理 Enter/Space**，以及 radiogroup 方向键漫游焦点
   （可以照抄 `:3856-3863` 那套 `.recommendation-tabs` 的 keydown 实现）。
4. **要同步改的测试**：`web/champions.test.cjs:3550-3571`（直接断言了
   `'<button class="specialist-game-head'` 和 `'</button><div class="specialist-game-content">'`）、
   `:2711`（点名了 `.rune-option-button::after` / `.item-option-button::after`）。
   新断言应该是"`data-rune-choice` 挂在 `.specialist-game-row` 上，且 row 内不存在 `<button`"。

---

## B 组（P1）：战绩卡跨模式对齐 + 斗魂伤害/承伤

（注：战绩卡渲染主体在 `web/gameplay.js` + `web/gameplay.css`，不在 `app.js`。）

### B-1 击杀参与率跨模式垂直不对齐

渲染在 `web/gameplay.js:1986`（participationRow）/`:1987`（csRow）/`:1988`（tierRow），
`:1991` 拼成 `statRows`，`:2000` 输出 `<div class="match-stats">`。
**solo/flex/aram/hextech-aram 四种模式的 statRows 完全相同（恒 3 行）**，所以差异不在这一列自身。

**根因**：`web/gameplay.css:640` `.match-stats { display: grid; align-content: center; }`
没有固定高度也没有固定行高，整块在 `.match-main` 第一行里**垂直居中**。
而第一行高度由左邻居 `.match-champion`（`:628`）决定：
- 标准/单双/灵活/大乱斗：`.match-loadout-mini`（`:631`）= 2行×26px + 3px = **55px** → 行高 58px
- 海克斯大乱斗：`renderMatchLoadout` 返回 `layout="mayhem"`（`gameplay.js:1888`）→
  `.match-loadout-mini.is-augments.is-mayhem`（`css:705`）= 3列×2行×30px + 3px = **63px** → 行高 63px

**海克斯卡片第一行比其他模式高 5px，居中的 `.match-stats` 整块下移约 2.5px。这是海克斯错位的确定根因。**

**灵活组排**：DOM 与单双排逐字节相同，偏移来自第三行"平均段位"的高度变化——
`css:647` `.match-tier-value { min-height: 18px }`，占位符 `…`/`—` 时 18px，
异步回填出段位徽章 `.match-tier-crest`（`css:654`，19px）后变 19px。
块高一变 `align-content:center` 立刻重新居中，上面两行整体上移 0.5px。
灵活组排恰恰是最容易停在 `—`/`…` 的队列，所以看起来和单双排不齐。

**一句话：这一列的 Y 坐标是被"卡片里别的东西有多高"反算出来的。只调 padding 无效，三条都要做**：
1. 让四种模式第一行等高——给 `.match-champion`/`.match-loadout-mini` 统一 `min-height: 63px`，
   或把 `is-mayhem` 的 30px 改回 26px；
2. `.match-stats` 改 `align-content: start` + `grid-template-rows: repeat(3, 18px)`（或整块 `min-height`），
   **让这一列的几何与模式彻底解耦**；
3. `.match-tier-value` 的 `min-height` 从 18px 提到 19px（等于 crest 高度），消除异步回填抖动。

### B-2 斗魂伤害/承伤两行数据要回来

**根因**：`web/gameplay.js:1991`
```js
const statRows = modeKind === "arena" ? "" : `${participationRow}${csRow}${tierRow}`;
```
斗魂被硬编码跳过整个数据列，`:2000` 于是连 `<div class="match-stats">` 容器都不输出。
**是被条件跳过，不是数据取不到**——`subject.damage`/`subject.damageTaken` 在斗魂链路有值
（`gameplay.js:1694` 的 `computeMatchScores` 用了 damage，展开详情 `:2223` 正常渲染）。

CSS 里还留着死代码证明这行曾经存在：`gameplay.css:649` `.match-stat-damage > b`、
`:650/652` `.match-damage-value`、`:651/653` `.match-taken-value`（全仓库只有 CSS 没有 JS 产出）。

改法：
1. `gameplay.js:1991` 把 arena 分支改成输出两行
   `<span class="match-stat"><em>伤害</em><b>…</b></span>` / `<em>承伤</em>`，复用 `.match-stat` 结构。
2. **颜色统一**：其他模式该列 `<b>` 用 `var(--muted)`（`css:643`），只有击杀参与率特意用 `var(--success)`（`:645`）。
   要和其他模式对齐就是**删掉 `css:650/651/652/653` 这四条覆盖**让它继承 `var(--muted)`。
   注意 **652/653 是 650/651 的高特异性重复，两处都要删，只删一处不生效**。
3. **有测试上锁**：`web/champions.test.cjs:1185`
   `assert.doesNotMatch(functionSource(gameplayScript,"renderMatch"), /damageRow|takenRow/)`，
   恢复功能必须同步改这条断言。
4. 复核 `gameplay.css:795` `.match-summary.is-arena .match-stats { display:none }`（≤520px 窄场景），
   恢复出来的列别在窄屏被二次隐藏。
5. **和 B-1 联动**：斗魂只给 2 行而其他模式 3 行，`align-content:center` 又会错位，
   所以务必配合 B-1 的"固定行高/固定块高"方案。

---

## C 组（P1）：对局页头部布局（图二）

### C-1 位置标签去掉"自动"两个字
`web/gameplay.js:3443`（`positionSwitch`）：`${automatic && active ? "<em>自动</em>" : ""}`。
**只删这个 `<em>` 分支，保留 `automatic` 变量和它控制的 tooltip**（tooltip 解释"为什么帮你选了这个位置"，有用）。
`web/gameplay.css:1013` 的 `.live-position-chip em` 随之变成死规则可一并删。
护栏提示：`web/champions.test.cjs:343/393` 断言的是 `const automatic = ...` 这行表达式文本，删 `<em>` 不会破坏它。

### C-2 优劣势对抗右侧留白 + C-3 上下两行卡片宽度不一致

**这两条要一起解，因为单独解会互相打架。**

留白根因在内层不在外层：外层 grid `.recommendation-champion-summary`（`gameplay.css:994`）
第二列 `minmax(0,1.18fr)` 本身已经顶到右内边距。真因是
`.champion-matchups > div { display: flex }`（`css:1015`），
子项是 58px 固定标签 + N 个 `.recommendation-matchup`（`css:1130`，`inline-flex; min-width:104px`，
**没有任何 flex-grow**），内容多宽就多宽、全部靠左堆积，上游只给 2~3 条时右边就空着。

**注意冲突**：`flex: 1 1 0` 能填满右边，但优势 3 张、劣势 2 张时**上下行仍然不等宽**。
要同时满足"贴右边"和"上下等宽"，只能**固定列数**：
```css
.champion-matchups > div { display: grid; grid-template-columns: 58px repeat(3, minmax(0,1fr)); }
```
不足的格子渲染空占位（或用 `visibility:hidden` 的占位卡）。卡片内 `b`/`small` 要配 `min-width:0` + 省略号。
护栏提示：`web/champions.test.cjs:3407` 断言了 `matchups()` 的 `.slice(0, 5)`（`gameplay.js:3432-3436`），
如果改渲染条数需同步改测试。

---

## D 组（P1）：非峡谷模式的区块显隐（图三/图四）

### D-1 先建立正确的机制（**这条是前提，不要跳过**）

后端**早就有能力位**：`champions_structured.go:150-157` 的 `opggModeSpecs`，
`HasCounters` 只有 `ranked` 和 `nexus-blitz` 为 true，aram/hextech-aram/arena 都是 false；
组装处在 `gameplay.go:3538-3543`。

**但 `gameplayRecommendationBundle`（`gameplay.go:2994-3014`）根本没下发这个字段**——
现有的只有 `hasRunes/hasAugments/hasTopPlayers`，前端没有任何模式信号可用，
所以 `web/gameplay.js:3447` 只能无条件渲染 `champion-matchups`，
落到 `emptyMatchups`（`:3431`）显示"该位置暂无对线样本"。

改法：
1. `gameplay.go:3522-3531` 的 bundle 里补 `HasCounters: spec.HasCounters`（json `hasCounters`）。
2. 前端 `recommendationCapabilities`（`web/gameplay.js:3213`）透传。
3. `renderChampionRecommendationHeader` 里 `hasCounters` 为 false 时**整块不渲染** `champion-matchups`，
   同时外层 grid 的 `grid-template-areas` 要退化成单列，否则右侧会留一个空列。
4. **禁用率同理**：新增一个能力位（建议 `hasBanRate`），false 时不渲染 `is-ban` 那一格。

> ⚠️ **不要在前端写 `source === "arena"` 这种硬编码模式白名单。**
> 队列 ID 白名单在本项目已经翻车四次（-1→3100→480→1750），
> 一律走后端 `queue_groups.go` / `opggModeSpecs` 下发的能力位。

### D-2 选用率/禁用率上游到底有没有（我已经 curl 实测过真实接口）

| 模式 | pick_rate | ban_rate | 说明 |
|---|---|---|---|
| 极地大乱斗 `GET lol-api-champion.op.gg/api/KR/champions/aram/64/none` | `0.069309` ✅ | `null` ❌ | 大乱斗没有 BP，上游就是 null |
| 斗魂 `GET .../api/global/champions/arena/64` | `0.100909` ✅ | `0.0158024` ✅ | 斗魂确实有禁用，走 `gameplay.go:3515-3516` 的 `ArenaStats` 分支 |
| 海克斯大乱斗 | ❌ 恒 0 | ❌ 恒 0 | 不走 op.gg detail（`champions_structured.go:188,745-753` 转 `loadMayhemDetail`），`hexdata.go:1826` **只写了 `WinRate`** |

结论：
- **选用率在极地大乱斗和斗魂都拿得到**（字段 `pickRate`，前端文案叫"选取率"），已在链路里。
  海克斯大乱斗要选用率得另找源（Hexdata 页面或 op.gg RSC，现未解析）——**本轮先不做，标为已知缺口**。
- **禁用率**：大乱斗/海克斯没有这个概念，应**按模式隐藏而不是显示"—"**；斗魂有真值可以保留。
- 护栏提示：`web/champions.test.cjs:3410` 与 `:3423` 断言了 `class="is-ban"` 存在及其颜色，必须同步改。

### D-3 删除所有"数据来源"署名（共四处，都在 `web/gameplay.js`）

1. `3244-3254` `renderRecommendationCitation()` → `<footer class="recommendation-source-line">数据来源 OP.GG/Hexdata…查看原页面</a>`，在 `:3318` 拼到推荐区末尾。样式 `web/gameplay.css:1064-1066`。
2. `3384` 斗魂海克斯区块 header 里的 `<span>OP.GG 斗魂竞技场选用率排序</span>`。
3. `3393` 海克斯大乱斗区块 header 里的 `<span>Hexdata · 已过滤低于 250 场的样本</span>`。
4. `3664-3665` 出装区的 `<p class="item-chain-source">${build.itemSource} · ${build.itemWindow}</p>`。样式 `web/gameplay.css:1302`。

删渲染 + 对应三处样式规则。**必须同步删/改 `web/champions.test.cjs:658-678`**
（该测试直接编译并调用 `renderRecommendationCitation`，断言 `数据来源 Hexdata` 和转义行为），否则 JS 测试必挂。

### D-4 主升技能"置灰"

**整个 `gameplay.css` 里没有任何针对技能图标的 `grayscale`/`opacity` 规则**
（R31 已删除低样本压暗，`web/champions.test.cjs:2711` 还在断言 `.skill-icon-button::after` 不存在）。
所以"灰"只可能是这两条之一，**请先确认是哪条再动手，不要用 CSS 掩盖资源问题**：

- **(a) 芯片本身就长这样**：`gameplay.css:1291` `.skill-icon-button { color: var(--muted); background: var(--surface-strong); border: 0 }`
  —— 四个技能芯片长得一模一样、文字是 muted 灰，**"主升"没有任何强调**
  （对比 `champions.css:484` 给大招留了 `.is-ultimate` 高亮，对局页从来没用过这个类）。视觉上就像被禁用。
  → 如果是这条：给主升芯片加高亮类（参考 `.is-ultimate` 的写法），而不是继续全 muted。
- **(b) 图标真的没加载出来**：`ability.iconPath` 为空时走 `<span class="skill-letter">`（`gameplay.js:3699`），
  或图片两次 404 后 `prepareImages`（`:4130-4155`）把 img 置 hidden，露出 `.game-icon` 的 `--surface-strong` 灰底 + 首字母占位。
  → 如果是这条：是资源链路问题（图标来自 `/lol-game-data/assets/v1/champions/{id}.json`，
  经 `/api/image?path=` 代理，`gameplay.go:4032-4087`），**看诊断里 champion-abilities 这个 capability 的 state
  以及那张图的 `/api/image?path=` 是否 200**。

---

## E 组（P2）：收起侧边栏后连接状态灯没有提示（图八）

`web/index.html:45` 的 `#connection` 是
`<div id="connection" class="connection"><img …><span class="status-dot"></span><span class="connection-label">正在检查客户端</span></div>`。

侧边栏收起时 `web/app.css:270`
`:root[data-sidebar="collapsed"] .connection span:last-child { display: none; }` 把文字标签隐藏，
**只剩一个圆点，而 `#connection` 上完全没有 `data-tooltip`**，所以鼠标悬停什么都没有。

对比：所有导航项（`index.html:37-40,46`）都带了 `data-tooltip` + `data-sidebar-tooltip`，
配合 `web/app.js:2722-2727` 的 `targetOf`——`data-sidebar-tooltip` 的元素**只在收起态或 ≤820px 时才显示 tooltip**。
这套机制现成可用。

改法：给 `#connection` 补上 `data-tooltip`（内容跟随当前连接状态动态更新，
在 `web/app.js:1974-1979` 那段设置侧边栏状态的同一处，或在更新 `connection-label` 文案的地方同步写 `dataset.tooltip`）
+ `data-sidebar-tooltip` + `data-tooltip-side="menu"` + `data-tooltip-size="compact"`，和导航项保持一致。
注意 tooltip 文案要用**实时状态**（"已连接客户端"/"正在检查客户端"/"未检测到客户端"），不能写死。

---

## F 组（P2）：斗魂详情按队伍分组（图九/图十）

### F-1 战绩展开详情：分组已有，但排序规则不对

- 渲染 `renderArenaMatchOverview`：`web/gameplay.js:2211-2228`
  （入口 `renderMatchDetail` `:2174-2177` 对 arena 直接短路，不走"概览/队伍分析/构建"三页签）。
- 分组 `matchPlayerGroups`：`web/gameplay.js:1812-1832`。主分支按 `subteamId` 分组，
  **排序键是 `placement`（名次）升序，不是当前玩家优先**（`:1823`）。
  兜底分支（`:1825-1830`）在没有 `subteamId` 时按 `length%3===0?3:2` 顺序切块，**分组是假的**。
- 队伍名来自前端硬编码常量 `ARENA_TEAM_MASCOTS`（`web/gameplay.js:1790-1800`）：
  subteamId 1-8 → 魄罗/小兵/河道蟹/石甲虫/锋喙鸟/暗影狼/魔沼蛙/哨兵，图标 `web/arena-team-icons/*.svg`。
  **⚠️ 这份映射从未与国服客户端核对过**，用户提到的队伍名不一定在表里——
  请加一次真机探测埋点把 subteamId→实际队伍名对上，不要凭猜。
- 战绩侧字段链路是通的：SGP `gameplay.go:479-480`（`playerSubteamId`/`subteamPlacement`）→ `:2518`；
  Riot `riot_api.go:433-434`→`:721`；YOUR.GG `yourgg_arena.go:203-204`→`:466-471`。

改法：`matchPlayerGroups` 排序改成"当前玩家所在队伍排第一，其余按 placement 升序"。

### F-2 对局页名单没有 subteam（这条是真缺失，就是日志里那条线索）

真机诊断 `live_roster_shape` 显示 `players:17, team_counts:{"100":17}` —— **17 个人全在 team 100**。

根因：`lcuLivePlayer` 结构体（`gameplay.go:3747-3764`）**完全没有 subteam 字段**；
名单来源 `gameplay.go:3853-3868` 把 `gameData.teamOne` 全部写死 `team=100`、`teamTwo` 写死 `200`。
斗魂时 LCU 把所有人放在 teamOne，所以全是 100。
渲染 `renderLiveInsights`（`web/gameplay.js:3410-3419`）硬编码 `team(100,"我方")`/`team(200,"对方")`，
斗魂下"对方"整块空。

改法：`lcuLivePlayer` + `lcuChampSelectPlayer` 加字段 → `gameplayLivePlayer` 加 `SubteamId`
→ `gameplay.go:3961` 赋值 → 前端 `renderLiveInsights` 改按小队分组。

> ⚠️ **LCU 在 ChampSelect/InProgress 是否真给 subteam 字段没有经过真机证实。**
> 请先加一次探测埋点（把 `/lol-gameflow/v1/session` 里 player 的**原始 key 列表**打进日志），
> 拿到真机回传再写解析——这个项目已经有过"猜了三轮字段名"的教训。

### F-3 斗魂随机英雄

现有链路是通的：`gameplay.go:3869-3889` 在 ChampSelect 阶段读 `/lol-champ-select/v1/session`，
`mergeChampSelectPlayers`（`:4089-4150`）把 `championId`（锁定）与 `championPickIntent`（预选）合进名单；
前端 `liveRecommendationChampionId`（`web/gameplay.js:2858-2863`）锁定优先、未锁定用 pick intent。
**随机英雄场景下客户端直接给 `championId`，逻辑上会立刻命中。**

事件也是现成的，不需要新建：`lcu_events.go` 订阅 `OnJsonApiEvent`
→ `connection_manager.go:120-126` 捕获 `/lol-champ-select/v1/session`
→ 300ms 去抖（`:12-14`）→ `broadcastEvent("champselect:changed")`（`:209`）
→ SSE → `web/app.js:2637-2639` → `web/gameplay.js:4503-4507` `loadLive(true)`。
兜底是 3 秒轮询。

**缺口**：`lcuChampSelectSession`（`gameplay.go:3766-3770`）只解析 `gameId/myTeam/theirTeam`，
**没有 `benchChampions`/`rerollsRemaining`/`subteams`**。所以"随机到什么"能拿到，"随机池/剩余次数"拿不到。
本轮只需确认"随机后立刻出海克斯和出装"这条通路真机可用（可以先只加埋点验证），
展示随机池是可选项。

---

## G 组（P2）：斗魂海克斯/出装页重做 + 与海斗合并（图十一/图十二）

### G-1 斗魂海克斯改成分品质三列（和海斗一致）

统一入口 `renderLiveAugmentRecommendations`：`web/gameplay.js:3363-3394`
- **斗魂**（`source !== "hextech"`）`:3383-3384`：单一扁平网格 `.live-augment-grid`，
  按 S/A/B/C 档再按 score/games 排序，`slice(0,12)`。
- **海斗**（`source === "hextech"`）`:3386-3393`：
  `definitions = [["silver","白银"],["gold","黄金"],["prismatic","棱彩"]]`，每档 filter + `slice(0,4)`
  成三列 `.live-augment-columns`（CSS `web/gameplay.css:1164`，窄屏 `:1384` 塌成一列），另有"品质待确认"兜底块。

**好消息：斗魂本来就有品质字段，可以直接复用海斗的三列渲染，无需改协议。**
`champions_structured.go:2304-2312` 用 `map[int]int{1:0,4:1,8:2}[group.Rarity]`
→ `normalizeAugmentRarity`（`hexdata.go:1772-1786`）产出 `silver/gold/prismatic`
→ 写进 `championMetricRow.Rarity`（`champions.go:241`）→ `gameplay.go:3536` 原样透传
→ 前端 `normalizeAugmentRarity`（`web/gameplay.js:3325-3334`）两种写法都认。

**一个坑**：`flattenArenaAugmentGroups`（`champions_structured.go:2331-2341`）
按 PickRate 全局排序后**截断到 15 条**（调用点 `:1285`），三档不保量；
而完整分档数据 `response.ArenaAugmentGroups`（`:1283`）**没有进 `gameplayRecommendationBundle`**
（结构体只有 `Augments []championMetricRow`，`gameplay.go:3012`）。
要做三列且每列有货，得改 `:1285` 的 limit 或把分组透传出去。

### G-2 斗魂出装区块重做

渲染 `renderBuildRecommendation`：`web/gameplay.js:3627-3670`。当前顺序：
1. `.build-summary-bar`（`:3645-3650`）：召唤师技能 → 出门装 → 鞋子 → 技能加点
2. `.item-build-layout`（`:3665`）：核心装（左）+ 第四/五/六件（右）
3. `.late-option-band`（`:3666-3668`）：**棱彩装备（在最下面）**

**(a) 删掉召唤师技能和出门装**：`champions_structured.go:1229-1232`，
`spec.HasAugments`（arena/hextech-aram）分支**只填 Boots/CoreItems/PrismItems，根本不填 SummonerSpells/StarterItems**
（那两行在 `:1236-1237` 的 else 分支里）。所以后端返回空数组，前端走空态文案。
删这两块只需在 `:3646-3647` **按后端能力位**（不要按 source 硬编码）排除。

**(b) 技能/装备要显示名称 + 技能显示 QWER**：
`renderConfigOption`（`web/gameplay.js:3732-3742`）只吐 `.config-icons`，名称目前只在 tooltip 里。
**前端就能拿到名字，不用改后端**——`renderItemIcon`（`:3767-3774`）和
`renderSummonerSpellIcon`（`:3776-3783`）已经从 `state.items`/`state.summonerSpells` 查到了 `name`。
QWER 字母：`renderSkillPlan`（`:3694-3709`）现在只在 `iconPath` 缺失时才用 `.skill-letter` 兜底；
战绩详情侧已有现成常量 `SKILL_SLOT_LETTERS`（`web/gameplay.js:2409`）**直接复用**。

**(c) 棱彩上移 + 核心装不要那么空**：
棱彩在 `:3666-3668` 的 `lateBands`，渲染在 `itemBuildLayout` 之后，要上移即调整 `:3669` 的拼接顺序。
核心装空的原因：斗魂 `CoreItems` 来自 YOUR.GG 聚合，
`mapYourGGArenaAggregateItems`（`yourgg_arena.go:124-155`）**每行只有一个 asset**（单件装备），
而 `optionList(coreOptions,"route",...)` 是为"三件套路线+箭头"设计的（`:3736` 的 `route-step`/`route-arrow`），
一件装备占一整行 → 视觉极空。CSS 在 `web/gameplay.css:1281-1308`。

**(d) 参照英雄详情页**（`web/champions.js`，**它已经做对了两件事**）：
- `renderBuildBoard`（`:1623-1639`）：`:1634` `if (state.mode !== "arena")` 跳过出门装；
  `:1636` `if (state.mode === "arena" && prismItems.length)` **把棱彩插在鞋子之后、出装路线之前**。
- 核心装+第四五六件横排：`renderRankedBuild`（`:1642-1651`），
  `.build-item-row` + `data-depth-count`，**与对局页 `:3665` 是同一套 class**，可以直接对齐。
- 左符文右侧栏（召唤师技能/出门装/鞋子带选用率胜率）：
  `renderRuneWorkspace` `:1540-1559`、`renderSpellsCard` `:1582-1596`、`renderSkillsCard` `:1598-1603`。

### G-3 海斗和斗魂的"海克斯 + 出装"合并成一个页面

页签定义 `recommendationTabSpecs`：`web/gameplay.js:3223-3229`
```js
if (capabilities.hasAugments) tabs.push(["augments","海克斯"]);
if (capabilities.hasRunes)    tabs.push(["runes","符文"]);
tabs.push(["insight","详情"], ["build","出装与技能"]);
```
能力位来自后端 `opggModeSpecs`（`champions_structured.go:150-156`：
arena 与 hextech-aram 都是 `HasRunes:false, HasAugments:true`），经 `gameplay.go:3525` 透传。
所以这两个模式现在的页签都是 **[海克斯][详情][出装与技能]**。

**合并改动范围不大**：`recommendationTabSpecs` 去掉独立 `augments` 项，
在 `content.build` 里把 `renderLiveAugmentRecommendations` 拼到 `renderBuildRecommendation` 前面即可。
但要同步处理这四处：
1. `recommendationPanelBusy`（`:3258-3266`，augments 与 build 两个加载判据不同，合并后要取并集）
2. `loadingCopy`（`:3285-3290`）
3. `recommendationActiveTab`（`:3231-3234`，有海克斯时默认落到第一个页签，合并后默认页签要改）
4. `resetLiveGameScopedState` 里写死的 `state.recommendationTab = "runes"`（`:2884`、`:2895`）

**两个模式的差异化设计按 `liveAugmentRecommendationSource(data)`（`:2950-2960`）的返回值分流**，
不要新建模式判断。页面设计上：海斗信息更少（只有海克斯 + 少量出装），
斗魂有棱彩/核心装/技能加点，请在同一页里用小标题分区，不要简单上下堆叠。

---

## H 组（P0，元问题）：诊断日志又被两个事件刷爆了

这份日志 8492 行、1.7MB，覆盖 06:24~06:37 共 13 分钟，其中：

| 事件 | 条数 | 占比 |
|---|---|---|
| `ranked_winrate_resolved` | 5490 | 64.6% |
| `ranked_data_source_decision` | 2745 | 32.3% |
| **两者合计** | **8235** | **97.0%** |
| 其余所有事件合计 | 257 | 3.0% |

**而且这两个事件的载荷是高度重复的**：
- `ranked_winrate_resolved` 只有两种组合，各 2745 条：
  `{source:lcu, queue:RANKED_SOLO_5x5, wins:0, losses:0, tier_empty:true, complete:true}`
  和同样的 `RANKED_FLEX_SR`。**5490 条里没有任何一条带非零 wins/losses。**
- `ranked_data_source_decision` 只有两种载荷：
  2099 条 `{selected:lcu, reason:local-client-connected, attempts:[{lcu,success}]}`
  和 646 条同样的但带 `fallback_reason:"sgp-completion-gate-closed"`。

13 分钟 5490 条 = **每秒 7 条**。这直接导致 1MB 轮转触发得极快，
**真正需要的绝活哥/对局页/斗魂事件全被冲掉了**——这轮我只能拿到 231 条
`specialist_runes_client_skip` 和 1 条 `live_recommendations_skip`，
`specialist_runes_start`/`specialist_runes_done` 一条都没有。

**这是第三次因为埋点刷屏拿不到关键日志了**（R36 是两个 shape 事件占 72.5%）。请这轮一并修：
1. 给这两个事件加**载荷级去重**（项目已有 `resetDiagnosticRotation`/去重 map 机制，
   见 R39 加的那套，这两个事件显然没接上）——相同载荷在同一个轮转周期内只记一次 + 计数。
2. 顺带看一眼**为什么会每秒 7 次**：`ranked_winrate_resolved` 恒定 `wins:0/losses:0/tier_empty:true`
   却标 `complete:true`，这本身就可疑（老的"losses 恒为 0"问题，见 R36 记录）。
   如果是某个轮询在反复解析同一份空数据，那问题不只是日志噪音。
3. 建议加一条护栏测试：**单一事件名在一个轮转周期内的条数占比不得超过 30%**，
   超过就说明该埋点没做去重。

**另外一条真机观察**：`specialist_runes_client_skip` 231 条里
162 条 `no-top-players` + 69 条 `no-target`，全部 `queue_id:1750`（斗魂 3x6）、`champion_id:233`。
斗魂本来就没有绝活哥榜单，这个 skip 是**正确行为**，但 13 分钟打 231 条同样是刷屏，一并去重。

---

## I 组（P0，追加）：对局名单——英雄选择阶段多出重复行 + 进入游戏后对方少一人

> 这两条是用户第二批截图 + `lol-loot-diagnostics-0830-1520.jsonl` 补充的。
> **先说三条已经排除的方向，别再往这三个方向查**：
> - `resetLiveGameScopedState`（`web/gameplay.js:2879-2888`）**一个字都没碰 `state.live.players`**，
>   名单来自每次 `/api/gameplay/live` 的整包响应，`loadLive` 里 `state.live = nextLive`（`:2836`）是整体替换，
>   前端不可能"留住"上一局的人。而且 `shouldResetLiveGameScopedState`（`:2872-2877`）在 gameId 变化**或**
>   进入 ChampSelect 时都会触发，ChampSelect 的 gameId=0 已经被 `enteringChampionSelect` 兜住了。**这条是好的，别改。**
> - 前端渲染层**没有任何 Map/Set 去重**：`renderLiveInsights`（`:3410-3419`）就是
>   `orderedPlayers.filter(p => p.teamId === 100/200)`，`orderLivePlayers`（`:3186-3198`）是 `[...players]` 拷贝后 sort，不删元素。
>   CSS `.live-team`/`.live-player-list`（`web/gameplay.css:967-975, 1191-1195`）没有 max-height，不存在裁切。
>   **给定一个 5/5 的 10 人载荷，界面必然渲染 5+5。**
> - 后端 InProgress 路径没有过滤/去重：`response.Players = make(..., len(rawPlayers))`（`gameplay.go:3896`），
>   逐 index 写入（`:3961`），`wait.Wait()` 后只重写了一次 `PlayerRef`（`:3965-3968`）。

### I-1（P0）Bug 1 真相：ChampSelect 多出来的人**不是上一局残留，是同一局玩家的重复行**

**根因：`mergeChampSelectPlayers` 匹配失败就 append（`gameplay.go:4119-4125`），而匹配两侧用了不对称的身份规则。**

```go
// gameplay.go:4109 —— champ-select 侧：走了反混淆
visiblePlayerRef := visibleChampSelectPlayerReference(selected)
selectedReference := normalizeGameplayReference(gameplayReference{PlayerRef: visiblePlayerRef, ...})
// gameplay.go:4113 —— gameflow 候选侧：直接用原始 PUUID，没走 visibleLivePlayerReference 反混淆
candidateReference := gameplayReference{PlayerRef: result[candidate].player.PUUID, ...}
```

两侧口径不一致，再叠加 `normalizeGameplayReference`（`:1563-1574`）会把不合 UUID 格式的引用直接置空、
`gameplayReferencesMatch`（`:1700-1720`）在 PUUID 与 summonerId 全为空/0 时**恒返回 false**。
国服 champ-select 对隐藏名玩家给的就是 `puuid=""` + `obfuscatedPuuid` + `summonerId=0`，
于是匹配必然失败 → 走 `index < 0` 分支 **append 一条新行**。

**界面特征完全对得上**（这条是确证不是猜）：append 出来的新行 `SelectedPosition = selected.AssignedPosition`，
大乱斗里该字段为空 → `normalizePosition("","")` 返回 `""`（`gameplay.go:5785-5786`）
→ 前端 `positionLabel("")` = **"位置未知"**（`web/gameplay.js:4023`）；
而 gameflow 那批带 `selectedPosition:"NONE"` 会落到 `default: return "other"` → **"其他"**。
用户截图里"我方 7 人、多出来的两个显示『位置未知』而其他人显示『其他』"，正是这两条 append 出来的重复行。
"对方 4 真人 + 4 隐藏玩家"同理 = gameflow 给出的 4 个真身份 + champ-select `theirTeam` 里
4 条匹配不上、身份为空（`gameplayDisplayName` 兜底 `gameplay.go:1893-1901`）而被 append 的重复行。

顺带解释了另一个现象：已匹配上的行位置没被修正，因为 `gameplay.go:4152`
`if selected.AssignedPosition != ""` 才覆盖，大乱斗恒为空。

**改法（推荐做第 2 条，第 1 条是最小修复）**：
1. 最小修复：`gameplay.go:4113` 改用 `visibleLivePlayerReference(result[candidate].player)`
   并对候选也走 `normalizeGameplayReference`，让两侧口径对称。
2. **结构性修复（更推荐）**：ChampSelect 阶段**以 champ-select 名单为权威重建**，
   gameflow 只用于补字段，而不是反过来以 gameflow 为基底 append；
   同时对 `index < 0` 的 append 加上**"该队人数已达上限就不再追加"**的闸门（大乱斗/召唤师峡谷每队 5）。
3. **必须补测试**：gameflow 给真 PUUID + champ-select 给同一人的 obfuscated 身份，断言结果长度不变。
   现有 `gameplay_test.go:1729-1810` 的用例全是 `existing=nil` 或身份能对上的，**正好绕过了这个分支**——
   这就是它能溜到线上的原因。

### I-2（P0）Bug 2：进入游戏后对方少一人 —— **代码里找不到丢人的地方，必须先补埋点**

我把整条链路走完了：后端 10 人 5/5 → JSON `teamId` 无 omitempty（`gameplay.go:3618`）→ 前端纯 filter。
**不存在任何一处会丢掉第 10 个人的代码。**
日志 `live_roster_shape` 明确显示 `{"phase":"InProgress","players":10,"team_counts":{"100":5,"200":5},"with_stats":10}`，
后端口径是完整的。

**最可能的解释（假设，未确证）：那条日志不是截图那一帧的响应。**
`recordLiveRosterShape`（`gameplay.go:3372-3389`）的去重键是 `game:<gameId>`，gameId<=0 时退化成 `phase:<PHASE>`，
而 **`liveRosterDiagnosticKeys` 这个 map 整个进程生命周期从不清空、也没有上限**
（对比 `championDataDiagnosticKeys` 有 128 上限，`:3357`）。所以：
- 每局只记录**第一次**响应；
- GameStart 阶段同样走这个 handler（`:3821` 白名单含 GameStart），此时 `GameData.GameID` 可能仍为 0
  → 键变成 `phase:GAMESTART`，一个进程只记一次；
- **也就是说"对方只有 4 人"的那一帧名单，日志里根本不会出现**，等 gameId 出现后记下的第一条已经是 10 人。

这也解释了为什么这份 14 分钟的日志里 `live_roster_shape` **只有 1 条**。

**所以这一组的第一要务不是改渲染，是把埋点改成能看见问题的样子**：
1. `recordLiveRosterShape` 的去重从"每局一次"改成"**名单指纹变化时记一次**"——
   键改成 `gameId + phase + 各玩家 (teamId, playerRef 或 hidden 标记) 的哈希`，**并加上限轮转**。
   这样 GameStart→InProgress 的每一次名单变化都会留痕，I-1 的 ChampSelect 重复行也会立刻显形。
2. 载荷补 `phase`、`raw_count`（`len(rawPlayers)`）、`merge_appended`（merge 里 `index<0` 命中次数）、`empty_ref_count`。
   **`duplicate_player_refs` 现在只在 `player.PlayerRef != ""` 时统计（`gameplay.go:3397`），
   空 PlayerRef 的重复行天然算不进去——这个指标结构上就抓不到 I-1，必须改成
   "按 (playerRef 或 obfuscated 身份) 计数，空引用单独计一列"。**
3. **前端补一条 `live_roster_rendered`**：`{phase, gameId, players_received, rendered_100, rendered_200}`。
   **只有这条能一刀切开"后端少给"与"前端少画"**，目前所有证据都停在后端口径上。
   ⚠️ 按 R39 的教训，新事件名和 reason **必须先在 `features.go` 的白名单登记**，否则会被静默拒收，白瞎一轮。
4. 顺手修一个潜在隐患：`escapeHTML`（`web/gameplay.js:99`）用 `textContent → innerHTML`，
   序列化文本节点**只转义 `& < >`，不转义 `"`**。`renderLivePlayer`（`:3183`）把玩家名塞进 `data-tooltip="..."`，
   名字含 `"` 会撕坏属性、被 HTML 解析器吞掉整行。Riot ID 一般不允许引号，所以这更像隐患而非本次根因，但值得顺手修。

**交付要求**：I-2 这一组**只加观测、不要猜着改渲染逻辑**（这条红线在 R39-D/E 组执行得很好，请照做）。
埋点上线后让用户复现一次，拿到 `live_roster_rendered` 与 `live_roster_shape` 的对照再定位。

### I-3 与 H 组的关联

这份 0830-1520 的日志 6441 行里，`ranked_winrate_resolved` 4114 条 + `ranked_data_source_decision` 2058 条
= **6172 条，占 95.8%**，和上一份 97% 是同一个病。而 `live_roster_shape` 只有 1 条、
`specialist_runes_client_skip` 又是 263 条重复（206 条 `no-top-players` + 57 条 `no-target`）。
**H 组的载荷级去重不修，I-2 加再多埋点也会被这两个事件冲掉。所以 H 组和 I-2 必须同一轮做完。**

---


- `go build` / `go vet` / `gofmt` 干净；`go test ./...` 全绿；
  `node --test web/*.test.cjs desktop/*.test.cjs` 全绿（**必须显式列 glob，不能用 `node --test web/`**）。
- **本轮要改的现有测试比较多**，逐条列在上面了（B-2 的 `champions.test.cjs:1185`、
  A-6 的 `:3550-3571`/`:2711`、D-2 的 `:3410`/`:3423`、D-3 的 `:658-678`、C-2 的 `:3407`）。
  改测试时**不要简单删断言，要换成能反映新行为的等价断言**。
- **A 组必须补"时序"测试**（A-5 说的那类）：用真实带 deadline 的 ctx，
  验证对手排名在主扫描超时后仍能返回；这是 R41 全绿却真机为空的直接原因。
- **变异测试**：A-2（队列过滤）、A-4（删兜底后位置不匹配就不返回）、
  B-1（`.match-stats` 几何解耦）、D-1（`hasCounters` 能力位）四处必须做，改回旧行为要 FAIL。
- **F-2 和 F-1 的队伍名映射要先加探测埋点再写解析**，不要凭猜，这个项目已经有过猜三轮字段名的教训。
- **I-2 只加观测、不要猜着改渲染逻辑**；I-2 与 H 组必须同一轮做完，否则新埋点会被刷屏事件冲掉。
- **I-1 必须补那条"gameflow 真 PUUID + champ-select obfuscated 身份是同一人"的测试**——
  现有测试用例正好绕过了出问题的分支，这是它能上线的直接原因。
- B/C/D/G 组用 Playwright 在 1440/900/420 三个宽度截图作为验收证据。
- 改完后请用户重启应用复现，从「设置 → 隐私与能力 → 导出诊断日志」导出并上传。
