# WORKLIST-R91 — 韩服额度 / 斗魂禁用误判 / 选人页闪烁 / 斗魂分组仍未出现 / 自动重连 / 四处 UI

诊断人：Claude（只读诊断，未改仓库任何代码文件；本文件是本轮唯一新增物）。执行人：GPT。

日志：`lol-loot-diagnostics-0914-2050.jsonl`（3314 行，单一 run `8b800bef`，
`build_fingerprint = 650a1dd2915b`）。

> **✅ 先确认一件事：用户跑的确实是 R90 build。**
> 在仓库根执行 `node desktop/source-fingerprint.cjs` 得到 **`650a1dd2915b`**，与日志里的
> `build_fingerprint` 逐字一致。所以下文所有结论都是针对 R90 之后的代码，不是"旧包没更新"。

> ⚠️ 行号是 2026-09-14 21:0x 快照，请按函数名/代码原文定位。

---

## 口径决定（不要自行更改）

1. **P1（韩服额度）先治根因再动提示。** 用户说"不要有这种提示"，但直接把横幅删掉等于把失败藏起来——
   必须先把"一个选手页 30 次 Riot 调用"和"磁盘缓存无法自证"这两件事解决，横幅自然不再出现。
2. **P4（斗魂禁用）只放宽"接管"判据，不改 hover→lock 的整体流程。** 日志证明锁定本来已经 armed 了，
   只是被误判取消；不要顺手重写整条禁用链路。
3. **P5（闪烁）改成 stale-while-revalidate，不是"把骨架屏删掉"。** 首次无数据时仍要有骨架。
4. **P6（斗魂分组）本轮的首要交付是"让失败可见"，其次才是扩大可用面。**
   这次日志里 R90 的分组代码**一次都没机会跑**（见 P6），而且**放弃时完全静默**——
   这才是"问题持续很长时间还定位不了"的真正原因。

---

## P1 · 总览韩服：「查询额度正在恢复」（★根因已量化）

### 这不是 Riot 返回的 429，是我们自己的本地预算

`riot_api.go` 的 `wait()`：`shortLimit = 15/秒`、**`longLimit = 90 次 / 2 分钟`**
（Riot 个人 key 是 100/2min，这里留了 10 次余量）。
当"需要等待的时间 ≥ 本次请求剩余截止时间"时，才抛出
`韩服查询额度正在恢复，请约 %d 秒后重试`（`riot_api.go:324`）并写 `riot_local_rate_limited`。

### 一个职业选手页要花多少额度（实测）

`riot_overview_cost` 28 条，**`matches_requested` 合计 290**、**`matches_from_disk` 恒为 0**。
每开一个选手页是**两轮**：

| 时间 | requested | loaded | duration | 说明 |
|---|---|---|---|---|
| 11:09:58 | 5 | 5 | 2202ms | 首屏 5 场 |
| 11:09:59 | 20 | **0** | 565ms | **整轮白跑** |
| 11:10:04 | 20 | 20 | 1956ms | 正式 20 场 |

（11:10:57 / 11:11:16 / 12:28:36 每个选手页都是同样的 5 → 20(0) → 20 三连；
12:28:23 那条白跑甚至花了 **3095ms**。）

再加上 `overview_phases_ms` 里各自独立计时的 `account / matchIDs / ranks / mastery`，
**一个选手页 ≈ 25~31 次 Riot 调用**。连开三个（11:09:58、11:10:57、11:11:16）＝ 75~90 次，
正好顶满 90/2min。于是：

- `11:11:17` → `rate_limited_count: 16, matches_failed: 15`（用户截图里的那条横幅）
- `11:11:32 / 34 / 35 / 36 / 37` → 连续 5 次重试**全部**被本地限流（`requested:0, rate_limited_count:1`）
- `11:11:58` → 靠 `limiter_queue_ms: 9399`（排队 9.4 秒）才挤过去

### 要做的

1. **合并"首屏 5 场 + 20 场"两轮**。`loaded=0` 的那一轮是纯浪费（最长 3095ms），
   应当只发一次 20 场的请求，首屏用已到达的前 5 条先渲染。
2. **让磁盘缓存可自证**。`matchByIDWithCache`（`riot_api.go:743`）有三态 `hit / disk / miss`，
   但 `riotOverviewCostTracker` **只统计了 `disk`**（`riot_api.go:1178`）。
   所以这次 `matches_from_disk: 0` **既可能是"缓存坏了"也可能是"全是内存命中/首次查询"，我无法区分**。
   → 三态全部计数（`matches_from_memory` / `matches_from_disk` / `matches_from_network`），
   否则下一轮还是查不出来。
   （旁证：12:28:34 那条 `overview_phases_ms.details = 0` 且 `details_inflight_peak: 0`，
   说明内存缓存确实在工作；磁盘缓存则毫无证据。）
3. **match-v5 详情是永久不可变数据**，`persistRiotMatch` 已经写了 365 天 TTL（`riot_match_cache.go:59`）。
   确认它真的落盘、真的跨进程复用（验收判据见下）。
4. **额度耗尽不要整页报错**：已经有排队能力（`limiter_queue_ms` 证明排队可行），
   把"超时即 429"改成"排队等待 + 已加载部分先渲染"，只有排队超过一个很长的上限才提示。
5. **提示本身**：当页面上已经有可用数据时（`已加载的战绩仍可查看` 这句话本身就承认了），
   不要用顶部红/黄横幅，降级为角落的小字状态 + 自动重试，不需要用户点"重试补齐"。

---

## P2 · 斗魂战绩条：伤害/承伤没对齐、用了"万"、颜色没区分

三处都在 `web/gameplay.js` / `web/gameplay.css`，证据明确：

1. **没对齐**：`web/gameplay.css:649-650`
   ```css
   .match-stats          { align-content: start;  grid-template-rows: repeat(3,18px); min-height: 54px; }
   .match-stats.is-arena { align-content: center; grid-template-rows: repeat(2,18px); min-height: 41px; }
   ```
   普通模式三行顶对齐、斗魂两行**垂直居中** ⇒ 斗魂的"伤害"永远和别的卡片的"击杀参与率"错位。
   → 斗魂也用 `align-content: start` + `repeat(3,18px)` + `min-height: 54px`，第三行留空
   （移动端断点 `:768-770` 同改）。
2. **"5.6万"**：`web/gameplay.js:2697-2698` 用的是 `compactNumber()`（`:156`，≥10000 转万）。
   → 改用 `plainInteger()`（`:154`）——**斗魂详情表里本来就是用它**（`web/gameplay.js:2972`），
   两处保持一致即可，不要新写格式化函数。
3. **颜色**：`damageRow` / `takenRow` 共用 `class="match-stat-damage"`，CSS 里只有
   `web/gameplay.css:659` 一条布局规则，**没有任何配色**。
   → 与斗魂详情的 `.match-damage-value` / `.match-taken-value`（`web/gameplay.js:2972` 里已在用、
   但 CSS 里同样没定义）统一：伤害走暖色、承伤走冷/弱色，取值从
   `web/app.css` 的 `--danger` / `--muted` 体系里选，**不要引入新色号**。
   改完两处（战绩条 + 斗魂详情）截图比对。

---

## P3 · 生涯页「undefined undefined · 单双排」

`web/suite.js:1411 rankLabel(draft)`：

```js
const tier = {...}[draft.tier] || draft.tier;          // draft.tier 为 undefined 时 → undefined
return `${tier}${[...].includes(draft.tier) ? "" : ` ${draft.division}`} · ${queue}`;  // division 同理
```

`draft.tier` / `draft.division` 未初始化（客户端未定级或接口没给）时两个 `undefined` 直接进模板。
截图里右侧"展示段位"下拉已经是"单双排 / 未定级 / I"，说明**下拉有默认值但 draft 没有被种上同样的值**。

→ 两件事都要做：(a) `rankLabel` 兜底（`tier` 缺失 → "未定级"，`division` 为空则不拼接）；
(b) 初始化 `state.facadeDraft` 时用与下拉框一致的默认值种进去，别让 UI 和 draft 各说各话。

---

## P4 · 斗魂禁用：亮出后又消失，被误判成「玩家主动切换」（★根因锁定）

### 日志逐帧（第二局，12:35:14~16）

| 时间 | 事件 | 关键字段 |
|---|---|---|
| 12:35:14.843 | `postflight started` | 发出 ban hover PATCH（555 血港鬼影） |
| 12:35:14.907 | `http-success` → `awaiting-confirmation` | duration 63ms |
| 12:35:14.918 | `postflight` | **`observed_champion_id: 555`, `business_confirmed: true`** ⇒ 客户端确实亮出来了 |
| 12:35:14.931 | `armed` | `completed: true, delay_ms: 2000` ⇒ 2 秒后锁定已排定 |
| 12:35:16.082 | `postflight` **`awaiting-state`** | **`observed_champion_id: 0`**（客户端把 hover 清回 0） |
| 12:35:16.082 | `delay` **`canceled`** | 锁定被取消 |
| 12:35:16.093 | `candidate-gate` **`manual-takeover`** | ⇒ 界面上那句"玩家主动切换，本轮已停止自动禁用" |
| 12:35:16.916 | `postflight` `not-applied-after-2s` | 收尾 |

**用户全程没碰过鼠标**，是**客户端自己**在 1.16 秒后把我们已确认的 hover 清成了 0。

### 代码根因

`champselect_takeover.go:110 champSelectManualActionChanged`：

```go
if action.ChampionID != 0 { return !submitted || action.ChampionID != last.ChampionID }
if !submitted || !last.Confirmed || last.Completed || last.ChampionID <= 0 { return false }
// Clearing a confirmed hover also yields, unless the client had to clear
// it because another player locked/banned that champion.
return !champion.TeammatePicked && !champion.SelectionStatus.IsBanned
```

"**已确认的 hover 被清成 0**"被当作玩家接管，只豁免"队友选走"和"已被禁用"两种情况。
斗魂 1750 走的是 `availability_source: "wildcard-session-evidence-hover"`
（R88 记录过：斗魂 `bannable-champion-ids` 返回哨兵 `[-1]`，只能靠 hover 反证可禁性），
**而这条路径下客户端本来就会把未提交的 hover 清掉** —— 判据和模式行为直接冲突。

### 要做的

1. **`action.ChampionID == 0` 一律不再判定为玩家接管**，改记一条新诊断
   `hover-cleared-by-client`（带 side / championID / 距上次确认的毫秒数）。
   只有 `action.ChampionID != 0 且 != 我们提交的英雄` 才算玩家换人（第一条分支保持不变）。
2. hover 被清后**不要取消已 armed 的锁定**：日志显示 12:35:14.931 的 `completed:true` 锁定
   本来就排好了，只要不误判就会在 2 秒后发出去。
3. 若担心放宽后出现"玩家手动清了 hover 却被我们强行锁定"，把豁免限定在
   `requires_hover_confirmation == true` 的路径（即斗魂 wildcard）即可，不要全局放开。

---

## P5 · 斗魂英雄选择：对局页每隔几秒闪一下 + 自己小队战绩"一直加载不出来"（同一个根因）

### 根因（一行代码）

`web/gameplay.js:4701 renderInsightMatches()` 第一行：

```js
if (state.liveLoading === true) return '<div class="insight-match-row is-history-pending">…骨架…</div>';
```

而 `loadLive()` 开头就是 `state.liveLoading = true; renderLive();` —— **每次轮询一开始，
所有玩家的战绩行会先被整体替换成骨架屏**，请求回来后再换回数据。

ChampSelect 的轮询间隔是**硬编码 3 秒**（`web/gameplay.js:117 liveRefreshDelayMs`，
R90 只收了 InProgress，没动 ChampSelect）。

### 日志对照

- 12:33:22 / 26 / 30 / 33 / 37 / 40 / 44 / 47 / 51 / 54 / 58 …… `live_load_cost` 每 3~4 秒一条，
  `players=3`（斗魂选人只展示自己小队），`total_ms` 只有 6~13ms。
  ⇒ **数据其实是秒回的，用户看到的"闪"纯粹是这次无谓的骨架切换。**
- 冷启动那两次是真的慢：`live_prewarm duration_ms: 2161` / `2401`（12:33:16、12:35:05），
  这 2 秒里战绩行是空的 ⇒ 用户说的"一直加载不出来"就是这两秒 + 每 3 秒重来一次的叠加观感。

### 要做的

1. **stale-while-revalidate**：`renderInsightMatches` 只在"这一格从未有过数据"时才显示骨架；
   已经渲染过的玩家在刷新期间保留上一帧内容。建议按 `player.playerRef` 记住上一次的
   `recentGames`，而不是依赖全局 `state.liveLoading`。
2. ChampSelect 的 3 秒轮询可以保留（选人阶段确实在变），但**刷新不得导致整块 DOM 重建**；
   至少保证"数据没变化时不重写 innerHTML"（可用 R90 已经在用的 `fingerprint` 思路做短路）。
3. 顺带核对：`live_roster_shape` 里 `with_stats: 3`，说明后端三个人的数据都齐了——
   这条能证明"加载不出来"不是后端缺数据，纯前端渲染问题。

---

## P6 · 斗魂小队分组仍然没出现（★这次日志里 R90 的分组代码一次都没跑到）

### 事实链（本次日志唯一一局斗魂，game 8982434568）

| 时间 | 事件 |
|---|---|
| 12:36:35.746 | `lcu_gameflow_session_shape` phase=InProgress，**`team_one_nonzero_counts.puuid = 17`**（只有 17 人） |
| 12:36:35.746 | `live_client_playerlist_shape` **`result: "unavailable", http_status: 0, player_count: 0`** |
| 12:36:47.15 | `live_roster_shape` InProgress **players=17**，`arena_grouped: false`，`arena_group_source: ""` |
| 12:37:09~11 | 相位 → **Reconnect**（用户掉线，全程没进过游戏，见截图七的"重新连接"） |
| 全程 | **`arena_group_order_rejected` 0 条、`arena_group_truth_check` 0 条** |

### 为什么 R90 的代码没跑

`arena_live_grouping.go` 的 `applyArenaLiveGrouping`：

- playerlist 不可用 ⇒ `snapshot.OrderedIdentities` 为空 ⇒ 走 `else` 分支调 `arenaSessionOrderGroups`；
- `arenaOrderGroups(17, 3)`：`count % squadSize != 0` ⇒ `return nil, false`；
- 于是 `applyArenaLiveGrouping` **直接 `return`，不写任何诊断、也不置 `ArenaGroupingUnavailable`**。

⇒ 用户看到平铺名单，日志里**一点痕迹都没有**。这正是"问题持续很长时间"却每轮都定位不了的原因。

**必须说清楚的一点：这次日志不能证明 R90 的分组在正常斗魂局里也是坏的**——
它根本没拿到任何一个可用的顺序来源（游戏进程没起来 + LCU 名单缺 1 人）。
0913 的日志里 playerlist 在真正开局后是稳定返回 18 人 / HTTP 200 的，那种情况下 R90 的主路应当成立。
**所以本轮第一优先级是让失败可见，而不是再猜一版算法。**

### 要做的

1. **所有放弃分支都必须写 `arena_group_order_rejected`**，reason 至少覆盖：
   `playerlist-unavailable` / `roster-not-divisible`（带 `player_count`、`squad_size`）/
   `allies-cross-blocks` / `allies-unresolved` / `invalid-blocks`。
   同时把 `ArenaGroupingUnavailable = true` 置上，让前端能说人话
   （现在这条路径连这个字段都不设，前端只能一直显示"自动刷新已开启"）。
2. **playerlist 探测要覆盖整个加载屏**。游戏进程通常在 InProgress 后 30~60 秒才开 2999 端口，
   本次第一次探测就在 `GameStart` 那一刻（12:36:35）必然失败。
   `liveClientRetryDelay = 3s`（`gameplay.go:40`）本身没问题，问题是前端 R90 改成
   "最多重试 8 次就停"——要确认这 8 次覆盖得到加载屏窗口（8×20s = 160s，勉强够，
   但前 60 秒应该用更密的节奏）。把探测次数/首次成功耗时记进诊断。
3. **17 人（有人没进来）时的兜底**：
   - playerlist 有 18 人时 R90 已能处理（以 playerlist 为准），这条保持；
   - playerlist 完全拿不到时**不要硬切**，按第 1 条明确报"无法分组"。
4. **取证项（为下一轮准备，不要这轮就改算法）**：0913 日志证实斗魂
   `lcu_champ_select_session_shape.my_team_length = 18`（cellId 0~17，只有自己小队 3 个有 puuid）。
   **选人阶段就能拿到完整的 18 人 cell 顺序**，这是比 InProgress 更早、更完整的顺序来源。
   请加一条诊断，在 InProgress 时比对"ChampSelect 的 cell 顺序"与"gameflow/playerlist 顺序"
   是否一致（按 championId 对齐即可，记录一致率），为下一轮换源提供依据。

---

## P7 · 斗魂详情的玩家战绩里不要显示段位

`web/gameplay.js:4451-4452`：

```js
const rankCopy = rank?.tier ? rankTitle(rank) : "未定级";
const contextCopy = arenaMode ? rankCopy : `${positionLabel(player.position)} · ${rankCopy}`;
```

斗魂下副标题**就是段位本身**（截图里每个名字底下的"未定级/白银 I/青铜 III"）。
→ `arenaMode` 时 `contextCopy` 置空（或换成小队标识，等 P6 分组出来后再定），并检查
`.live-player-*` 的栅格在少一行文字后不塌。

---

## P8 · 「斗魂禁用环节需真机确认」提示删掉

`web/suite.js:822` 末尾：

```js
${definition.groupId === "arena" ? '<div class="suite-note is-warning">…斗魂禁用环节需真机确认…</div>' : ""}
```

整块删除。（P4 修好之后这句话本身也不再成立。）

---

## P9 · 自动重连「已开启但没触发」（★不是规则坏了，是开关开晚了 + 规则只认相变）

### 决定性证据

`watch_settings_saved` 全表：

| 时间 | autoReconnect |
|---|---|
| 12:30:52 / 12:31:22 / 12:32:00 / 12:32:03 / 12:33:02 | **`{enabled: false, delayMs: 10000}`** |
| **12:37:31** | `{enabled: true, delayMs: 10000}` |
| 12:37:33 | `{enabled: true, delayMs: 3000}` |

而 **Reconnect 相位在 12:37:11 就开始了**——开关是掉线 **20 秒之后**才打开的。

`watch_rules.go:454`：

```go
case "Reconnect":
    if settings.Rules.AutoReconnect.Enabled { r.schedule(client, "reconnect", …) }
```

定时器**只在进入 Reconnect 的那一刻**挂载。开关打开时相位早就停在 Reconnect 了，
没有新的相变 ⇒ 永远不会 armed ⇒ 界面显示"未触发过"。全日志 `watch_action` 里
**没有任何 action=reconnect 的记录**，与此完全吻合。

### 要做的

1. **保存设置后立即用当前 phase 重新评估一次规则**（不只是 reconnect——accept、play-again
   都有同样的"开关开在相位中间就失效"问题）。做法：设置保存成功后，拿一次
   `/lol-gameflow/v1/gameflow-phase` 并走一遍 `observePhase` 的相同分支。
2. 规则卡片上的"未触发过"文案要能区分**"没到触发时机"**和**"到了但当时没开"**，
   否则用户永远分不清是 bug 还是自己开晚了。

---

## 验收判据 + 变异测试（每条都要能被改坏后变红）

1. **P1 合并请求**：构造一次总览加载，断言只发出**一轮** 20 场的详情请求；
   把合并逻辑改回"先 5 后 20"必须变红。
2. **P1 缓存三态**：同一 matchID 连续两次 `matchByIDWithCache`，第二次必须记 `memory`；
   新建 provider（模拟重启）读同一 key 必须记 `disk` 且**零网络请求**；
   把 `persistRiotMatch` 的写盘删掉，这条必须变红。
3. **P4 禁用误判**：构造"hover 已确认(555) → 下一帧 session 里本地 ban action championId 变 0，
   且该英雄未被队友选走/未被禁用"，断言**不进入 takeover**、已 armed 的锁定仍然发出 PATCH；
   把 `champSelectManualActionChanged` 的 `ChampionID == 0` 分支改回 `return true` 必须变红。
   另补反向用例：championId 变成**另一个非零英雄**时**必须**进入 takeover（防止放宽过头）。
4. **P5 闪烁**：jsdom 下先渲染一次带战绩的名单，再置 `state.liveLoading = true` 重渲染，
   断言战绩行文本**不变**（不出现骨架）；把 stale-while-revalidate 去掉必须变红。
   同时保留"从未有数据 ⇒ 显示骨架"的正向用例。
5. **P6 失败可见**：三个用例——playerlist 不可用 + 17 人 gameflow、playerlist 不可用 + 18 人、
   17 人且 playerlist 18 人——前两个必须产出 `arena_group_order_rejected`
   （reason 分别为 `playerlist-unavailable`、`roster-not-divisible`）**且** `ArenaGroupingUnavailable == true`；
   把任一分支的埋点删掉必须变红。
6. **P9 相位重评估**：在相位已经是 Reconnect 的状态下打开 autoReconnect，
   断言立刻 armed 出一条 `watch_action{action:"reconnect", result:"armed"}`；
   把"保存后重评估"删掉必须变红。
7. **P2/P3/P7/P8 的 UI 改动**：走现有 jsdom 渲染冒烟 + 真 Chromium 截图，
   至少断言：斗魂战绩条第一行与普通卡片第一行 `getBoundingClientRect().top` 相等；
   伤害文本不含"万"；`rankLabel({})` 不含 `undefined`；斗魂玩家行不含段位文案；
   suite 页 arena 分组下不存在那条 `suite-note is-warning`。

---

## 本轮明确不做的事

- 不再改斗魂分组的**算法**（P6 第 4 条只加取证埋点）。这次日志根本没跑到算法，
  再猜一版只会重复上一轮的错误。
- 不动本地限流的 `shortLimit/longLimit` 数值——预算本身是对的，要减的是浪费。
- 不动 InProgress 的刷新收敛（R90 已做，本次日志的 Reconnect 局只加载了 4 次，符合预期）。
- 不动 `accept` 的前台窗口逻辑（本次日志里 accept 全部 `armed → fired` 正常）。

---

## 只能真机验收的清单

1. 打一局**能正常进入游戏**的斗魂（不要中途掉线），确认：分组出现 6 块；
   若仍未出现，导出日志看 `arena_group_order_rejected` 的 reason —— 这次必须有，不能再是空白。
2. 连开 3 个韩服职业选手页，确认不再出现"额度正在恢复"横幅；
   **关掉应用重开**再看同一个选手，导出日志确认 `matches_from_disk`（或新的三态埋点）不为 0。
3. 斗魂选人阶段停留 30 秒，确认战绩行不再每 3 秒闪一次。
4. 斗魂选人阶段确认禁用真的锁定成功（不再出现"玩家主动切换"），
   导出日志确认有 `hover-cleared-by-client` 而**没有** `manual-takeover`。
5. 先开好"断线自动重连"再制造一次掉线，确认 `watch_action{action:"reconnect"}` 出现。
