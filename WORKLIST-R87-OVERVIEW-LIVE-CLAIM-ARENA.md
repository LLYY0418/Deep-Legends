# WORKLIST-R87 — 总览名单宽度 / 对局识别 / 总览自动更新 / 收藏页 / 领取卡住 / 斗魂征召

诊断日期：2026-09-13
证据来源：`lol-loot-diagnostics-0913-1646.jsonl`（12663 行，2026-09-07 ~ 09-13T16:46Z）、用户截图两张、当前仓库源码（所有 file:line 均为本轮亲自复核，非抄旧文档）

**本工单只描述"改什么、改成什么、怎么验收"，不含代码 diff。执行方请先读完"红线"一节。**

---

## 红线（违反即判定整轮失败）

1. **P1 的改动只允许落在名单块自身的轨道/列宽上。** `.match-main` 的四条轨道、`.match-stats { justify-self:start }` 是设计基准，**一个字都不许动**。上一轮为了"填缝"去重排这块，用户当场发火。
2. **P2/P3 不允许把"自动刷新"做成无节制轮询。** 内嵌 Riot key 是全员共享配额，SGP 也有限流。任何自动刷新都必须带触发条件 + 节流 + 失败降级。
3. **P6 不允许用"再加一个 queueId 常量"了事。** 根因就是同一语义在两处用了不同口径，修法必须是**收敛到同一个判定函数**。
4. 每条都要求有可复现的护栏测试；本项目历史上多次出现"改了但那条路径压根没跑过"，**改完请附上一条会因为你的修改而由红转绿、且把你的修改回滚就变红的测试**。

---

## P1 · 总览战绩条"玩家名单"宽度不一致（UI）

### 症状
用户截图：每张战绩卡的玩家名单块宽度随该局最长名字变化，上下相邻卡片左右边缘参差不齐。
用户要求：**卡片之间宽度统一对齐**；名字短时两列之间的间隙可以变大；**要有最大宽度上限**。

### 已证实的机制
- `web/gameplay.css:620` `.match-summary { grid-template-columns: 108px minmax(0,1fr) minmax(0,max-content) 40px; ... }` — 名单轨道吃 `max-content`，宽度跟着本局最长名字走。
- `web/gameplay.css:697` `.match-players { width:auto; max-width: var(--match-roster-width,...); grid-template-columns: repeat(2,minmax(0,max-content)); justify-content:end; justify-self:end; gap: 2px 14px; }` — `--match-roster-width` 只是**上限**不是固定值；短名字时整块收缩并贴右，左边界随之左右漂移。
- `web/gameplay.js:2439-2440` `renderMatchPlayers` 不做任何宽度计算，问题 100% 在 CSS。
- `web/gameplay.css:616-619` / `693-696` 的注释写明这是上一轮**刻意**改成 max-content 的（当时诉求是"名字短不要留空隙"）。**本次需求与之正好相反，属于需求反转，不是回退旧版本。**
- `--match-roster-width` 断点：`gameplay.css:605` 基准 `clamp(150px,18cqi,190px)`；`:797` `clamp(220px,25cqi,260px)`；`:804` `clamp(300px,36cqi,420px)`。容器 ≥1080px（窗口约 1920 起）时**已恒定顶到 420px 上限**。

### 实测（真 Chromium，`localStorage['lol-loot-ui-scale']='1'` 钉死缩放，8 张名字长度 4/6/8/12/16/20 字的卡片）

| 名字长度 | 现状名单块宽 | 方案A 名单块宽 | 现状列间距 | 方案A 列间距 |
|---|---|---|---|---|
| 4 字 | 125.3px | **420px** | 14px | 308.7px |
| 6 字 | 167.3px | **420px** | 14px | 266.7px |
| 8 字 | 209.3px | **420px** | 14px | 224.7px |
| 12 字 | 293.3px | **420px** | 14px | 140.7px |
| 16 字 | 377.3px | **420px** | 14px | 56.7px |
| 20 字 | 420px（截断） | 420px（截断） | 14px | 14px |

1920 / 2560 / 3840 三档下数值完全一致；卡片高度恒 116px 不变；除 20 字档外 `truncated` 均为 0。
截图：`outputs/roster-baseline-1920.png`（左边缘参差） vs `outputs/roster-fixA-1920.png`（左边缘完全对齐）。

### 要做的改动（方案 A，仅 2 行 CSS，零 JS）
1. `web/gameplay.css:620` 第三条轨道 `minmax(0,max-content)` → `var(--match-roster-width,clamp(150px,18cqi,190px))`（写法与同文件 `:621` 的 `.match-summary.is-arena` 保持一致）。
2. `web/gameplay.css:697` 的 `.match-players`：`width:auto` → `width:100%`；**删掉** `max-width: var(--match-roster-width,...)`；`justify-content:end` → `justify-content:space-between`。`justify-self:end`、`gap: 2px 14px`、两列 `minmax(0,max-content)` 保持不变。

**最大宽度上限沿用现有 `--match-roster-width` 的 420px 顶（`gameplay.css:804`），不要改数值** —— 实测 1920/2K/4K 都已顶满 420px，加大只会挤压左侧统计区（触碰红线 1）。若用户后续仍嫌短，只改 `:804` 那一个 clamp 上限值即可，其余不动。

### 被否决的方案（写进工单是为了避免重复讨论）
- **按字符数分档量化（4ch 台阶）**：只压抑抖动，同页仍存在 2~3 种宽度，卡片之间照样对不齐 —— 不满足核心诉求。
- **JS 统计当页最长名字写页面级 CSS 变量**：观感更精致（短名字页不会被迫拉到 308px 间隙），但要在渲染入口新增 DOM 测量 + 变量注入，并接入"整页重建"那条本来就有性能问题的链路。**列为二期**：若上线后用户反馈"间隙太大"再做。

### 必须同步改判据的护栏（不改会拿旧判据把本次修改判成失败）
- `web/champions.test.cjs:1393` 断言 `.match-summary{... minmax(0,max-content) 40px}` → 改为断言新的 `var(--match-roster-width...)` 写法。
- `web/champions.test.cjs:1394` 断言 `.match-players{... max-width:var(--match-roster-width...)}` → 该属性被删除，判据改为断言 `width:100%` + `justify-content:space-between`。
- `desktop/ui-scale-layout.cjs:43 WIDTH_BAND`（按名字长度分档校验宽度）、`:65 roster-hugs-content`、`:70 column-gap`（断言 `columnGap===14` 恒定）—— **这三条是专门验证旧行为的，与新需求正面冲突，必须重写**为：①同一页内所有卡片 `playersWidth` 严格相等；②`columnGap ≥ 14` 且允许随名字长度变化；③20 字名字仍不溢出卡片。
- `desktop/ui-scale-layout.cjs:29 players-1fr` 变异分支（防止退回 1fr 等分）与方案 A 不冲突，**保留**。

### 验收判据
- 同一页 8 张不同名字长度的卡片，名单块宽度像素级相等（容差 0）。
- 16 字名字零截断，卡片高度恒 116px。
- 1920 / 2560 / 3840 三档行为一致。
- `.match-main` 四条轨道与 `.match-stats` 的 CSS 文本**逐字未变**（请在报告里贴改动前后的 diff 证明这两处为空 diff）。

---

## P2 · 对局进行中"推荐不出来"（★★★本轮最高价值，且我推翻了第一版结论）

### 症状
用户原话：「游戏结束后很多次都没有识别出来，下次对局都进入游戏了，对局的推荐都没出来，好像是因为一次重开导致的」。

### 日志实测：14 次 InProgress 的逐局表，这是全部证据

phase 序列从 `accept_focus_event_history` 的 entries 还原（去重 142 条），`live_load_cost` 为对局数据实际加载的埋点：

| InProgress 起点 | 该局结束 | 首次 live 加载(players≥8) | 延迟 |
|---|---|---|---|
| 04:31:28 | 04:33:39 | ❌ 全程无 | — |
| 04:34:46 | 04:51:36 | 04:34:50 | 4s |
| 04:54:02 | 05:16:09 | 04:54:07 | 4s |
| 05:17:11 | 05:26:45 | 05:17:15 | 4s |
| 05:28:14 | 05:51:56 | 05:28:43 | **29s** |
| 05:53:22 | 06:09:31 | 05:53:27 | 5s |
| 06:11:28 | 06:25:11 | 06:11:33 | 5s |
| **06:26:31** | **06:51:30** | **❌ 全程 25 分钟无** | — |
| 06:53:10 | 07:17:33 | 06:53:49 | **39s** |
| 07:18:37 | 07:34:01 | 07:18:39 | 2s |
| 07:35:12 | 07:38:22 | 07:35:21 | 9s |
| 07:39:30 | 07:59:21 | 07:39:40 | 10s |
| 08:00:23 | 08:09:04 | 08:00:25 | 2s |
| 08:10:08 | 08:32:19 | 08:13:16 | **188s** |

**★请注意两条需要澄清的事实，不要照抄用户的猜测：**
- 04:31:28 那局是 **queue 3140 / PRACTICETOOL（训练模式）**（证据：`live_position_shape` @04:31:13 `queue_id:3140, game_mode:PRACTICETOOL, game_id:8979723724`；`watch_custom_classification` @04:31:10 `custom:true`）。它无加载**属于预期**，不是 bug。
- **本份日志里没有找到"重开(remake)"的直接证据。** 用户归因到重开这一点**未被证实**。真正稳定复现的失败是 06:26:31 那局（queue 2400 海克斯大乱斗，25 分钟完整局）。

### 已证实的结构性根因：对局数据加载 100% 由前端驱动，且三条触发路径全部要求窗口可见

1. 后端**没有任何 InProgress 预取**：`live_load_cost` 只在前端发起 `/api/gameplay/live` 时产生；`gameplay.go:5719` 的 handler 是纯被动的（`phase` 不在 ChampSelect/InProgress/GameStart/Reconnect 时清空快照并返回）。
2. SSE 主路径 `web/gameplay.js:5910-5921`：`if (phaseChanged && !document.hidden && connected()) loadLive(true)` —— **玩家在游戏里时 `document.hidden` 必然为 true**，这条永远走不到。
3. 退路 `queueLiveEventRefresh()` `web/gameplay.js:5886-5895`：`document.hidden` 时只置 `state.liveRefreshQueued = true` 就 return，**不建定时器**。
4. 唯一兜底是 `web/gameplay.js:5837-5849` 的 `visibilitychange`，只在窗口重新可见时按 `resyncPending → liveRefreshQueued → section==="live"` 三选一执行。
5. `web/gameplay.js:5233-5238 scheduleLiveRefresh()` 的周期兜底 `if (!state.settings.liveRefresh || state.section !== "live" || document.hidden) return;` —— 不在"对局"页时完全停摆。
6. `web/gameplay.js:5941-5949 pollGameflowPhase()` 的 1s/12s 轮询**只调 `updateBeacon()`，从不调 `loadLive()`**。

⇒ 结论：**推荐出不出来，取决于玩家什么时候 Alt-Tab 回到应用。** 表里 29s / 39s / 188s 三次延迟正是 Alt-Tab 的时刻；用户体感就是"进游戏了推荐还没出来"。

### 高度怀疑（需埋点坐实）：`refreshAfterResync` 会无条件吞掉待刷新标志
`web/gameplay.js:5955-5967`：
```
if (state.destroyed || document.hidden) return;
state.resyncPending = false;
state.liveRefreshQueued = false;          // ← 无条件清空
if (state.section === "overview") { ...loadOverview(activeTab, true) }
if (state.section === "live") void loadLive(true);
```
且 `visibilitychange` 的 `5844` 行 `else if (state.resyncPending) refreshAfterResync();` **排在 `5846` 的 `liveRefreshQueued` 分支之前**。
⇒ 若用户 Alt-Tab 回来时正好有 resync 挂起、且当时停在"总览"页，`liveRefreshQueued` 被清空而 `loadLive` 从不调用 → **这一局剩余全程再也不会自动加载**（整局不会再有 phase 变化来重新触发）。这是唯一一条能解释 06:26:31 那局"25 分钟零加载"的路径。

### 要做的改动
1. **【主修】后端在进入 InProgress 时主动预取并缓存对局数据**，不再依赖前端窗口可见。挂在 `connection_manager.go:225-231` 已有的 gameflow 监听里（`broadcastEvent("gameflow:"+phase)` 旁边），phase 进入 `ChampSelect` / `InProgress` 时异步预热 `/api/gameplay/live` 的数据源。收益：Alt-Tab 回来是**立即**有数据，而不是像 06:53:49 那次现拉 7648ms。
2. **【必修】`refreshAfterResync` 不得吞掉 `liveRefreshQueued`**：只有在确实执行了 `loadLive` 的分支里才清标志；未执行就保留，交给后续 `visibilitychange` / 页签切换消费。
3. **【必修】`pollGameflowPhase` 检测到 phase 实际变化（不只是刷提示灯）时也触发一次 `loadLive()`**，作为 SSE 丢失时的独立兜底。
4. **【建议】** 非"对局"页时保留一个低频（30~60s）后台兜底刷新，仅在 phase ∈ {ChampSelect, InProgress, Reconnect} 时运行。

### 必须补的埋点（否则下一份日志还是判不出来）
在 `loadLive()` / `queueLiveEventRefresh()` / SSE 监听器三处各打一条诊断，记录：调用来源、当时的 `document.hidden`、`state.section`、`phaseChanged` 判定结果、`liveRefreshQueued` 值。目前这些全是前端内存态，日志里一个字都没有。

### 验收判据
- 构造"phase 在 `document.hidden=true` 时变为 InProgress，随后窗口可见"的前端测试：必须最终发生一次 live 加载。
- 构造"resyncPending 与 liveRefreshQueued 同时为真、且 section 为 overview"的测试：`liveRefreshQueued` 不得被清空。
- 后端测试：gameflow 进入 InProgress 后，`/api/gameplay/live` 的首次响应耗时显著低于冷路径（对照组用当前实现）。

---

## P3 · 总览有新战绩不自动更新（且要覆盖所有标签页玩家）

### 症状
用户原话：「总览我自己有新战绩了没有自动更新，还得靠我手动刷新；总览有标签页的玩家都要自动更新其战绩」。

### 已证实的根因：对局结束事件与总览缓存之间**物理上不存在调用链**
- 总览查询缓存 TTL 只有 2 秒：`overview_cache.go:13 overviewQuerySnapshotTTL = 2 * time.Second`。**TTL 短根本不解决问题**，因为没有任何东西去"重新请求"。
- `clearOverviewQuerySnapshots()` 共 6 处调用（`overview_cache.go:195` 定义）：`connection_manager.go:775`、`:794`（断线）、`opgg_insights.go:263`（历史段位回填完成）、`season_stats.go:545`、`:749`（赛季回填完成）。
  **★更正**：并非"仅两处"。但**这 6 处没有一处挂在 gameflow 阶段变化上** —— 结论不变，证据需按此表述。
- `connection_manager.go:231` 的 `broadcastEvent("gameflow:"+phase)` 后面接的是 `watch.handlePhase` 和 `lpTracker.handlePhase`，**没有任何总览失效动作**。
- 前端 `web/app.js:3238-3243` 注释原文写明"对局阶段事件只转发给对局模块（'对局'页签的新对局提示灯），**不触发全量刷新**"，只 dispatch `deep-legends:gameflow`。
- 该事件唯一消费者 `web/gameplay.js:5909-5921` 只调 `updateBeacon()` 和 `loadLive()`，**从不调 `loadOverview`**。
- 总览刷新只有三个入口：手动刷新按钮 `web/gameplay.js:5681-5690`；切到某 tab 且 `tab.data` 为空时 `web/gameplay.js:668-676`（有 data 就直接复用）；`refreshAfterResync` `web/gameplay.js:5958-5965`（**只刷 `activeTab()`，不遍历全部 tab**）。
- 多标签架构本身没问题：`state.tabs` 是数组，每个 tab 自带 `{data, loadedAt, playerRef}`（`web/gameplay.js:453,1086`），支持逐 tab 独立刷新，只是没人驱动。
- 唯一已有的自动轮询是"当前对局"卡片 `scheduleOverviewCurrentGame`（`web/gameplay.js:1645-1655`，30s），**也只对 `activeTab()` 生效**。

### 要做的改动
1. `connection_manager.go:231` 的 phase 分支中，phase 进入 `EndOfGame` **或** `WaitingForStats`（见下方 ★）时，按 playerRef 精准失效总览缓存（把 `clearOverviewQuerySnapshots` 扩展出一个按 playerRef 的版本，不要沿用断线时的全量清）。
2. `web/gameplay.js:5909` 的 gameflow 监听器新增分支：对局结束时**遍历 `state.tabs`（不只 activeTab）**——
   - 自己那个 tab：立即 `force` 刷新；
   - 其他玩家 tab：只打 `dirty` 标记，等用户切过去或该 tab 可见时再 `force` 刷新。
   **这是红线 2 的落点**：不要对所有 tab 同时打上游。
3. 节流：战绩入库有延迟，**延迟 6~8 秒再拉**，若新对局仍未出现最多再补 2 次重试。刷新期间复用现有 `overview-refreshing` 提示条（`web/gameplay.js:1530`），不要整页闪烁。
4. 失败时复用现有 `tab.error` / `pagination.moreError` 的静默重试逻辑，不要弹错误。

### ★ 一个必须在实现前确认的事实（会直接决定触发条件写哪个 phase）
本份日志 14 局里，**只有 6 局真的出现了 `EndOfGame`**（05:16:11、05:26:46、05:51:57、06:25:13、07:34:03、07:38:22），
其余 8 局是 `WaitingForStats → PreEndOfGame → Lobby`，**直接跳过 EndOfGame**；还有一局（04:33:39）走的是 `WaitingForStats → None`。
⇒ **只监听 `EndOfGame` 会漏掉一半以上的对局。** 触发条件必须覆盖 `WaitingForStats` / `PreEndOfGame` / `EndOfGame` 三者（并做本局去重，一局只触发一次）。
参考已有正确写法：`lp_tracker.go:279` 就是 `phase != "EndOfGame" && phase != "PreEndOfGame" && phase != "WaitingForStats"` 三者并列；`watch_rules.go:432` 的 `case "WaitingForStats", "PreEndOfGame", "EndOfGame"` 同理。**新代码请与这两处保持同一口径。**

### 上游配额现状（供节流设计参考）
`sgp_request` 179 条中 **73 条（41%，全部集中在 09-13）`error_kind=canceled`**。做自动刷新前先查清这个取消率的来源（软预算超时 or tab 切换 abort），否则自动刷新会把它放大。

### 存疑（上线前请自行确认）
`force=true` 是否真能绕过 `sgp_api.go:82 sgpCacheTTL = 5 * time.Minute` 的 SGP 战绩页缓存尚未逐行确认（`gameplay.go:6439` 附近的 `matchHistory(..., useCache=true)` 疑似另一条路径）。**若不能绕过，即使加了触发，5 分钟内仍会读到旧缓存，等于白做。**

### 验收判据
- Go 测试：注入一次 `WaitingForStats` 事件后，对应 playerRef 的总览缓存被失效（回滚修改则测试变红）。
- 前端测试：`deep-legends:gameflow` 且 phase 为结束态时，**非 active 的其他玩家 tab 被打上 dirty 标记**，且**没有**立即发起网络请求。
- 手工：打完一局回到总览，无需点刷新即可看到新战绩。

---

## P4 · 收藏页"识别不到" + 重新获取很慢

### 症状
用户原话：「收藏页面也出现识别不到的错误，我点击重新获取好长时间才刷新成功，好像是因为客户端有物品更新的问题」。

### 已证实
**「慢」是结构性的，不是客户端更新导致的偶发。** 09-07~09-13 五次 `refresh_succeeded` 耗时全部落在 **3991~5483ms**，逐日复现。用户猜的"客户端正在更新物品"只是巧合触发点。
- 实测两次：R2 `08:33:26.02 → 08:33:31.69` = **5483ms**；R3 `08:33:44.86 → 08:33:49.96` = **4957ms**。
- 耗时分解（R2）：`skin_acquisition_dates` 1853ms + `chroma_acquisition_dates` 1744ms + `skin_ownership` 1671ms + `chroma_ownership` 1832ms ≈ 7100ms，靠 `catalog.go:324 runParallelLoaders` 的组间并行才压到 5.4s。**每组内部全程串行**。
- `catalog.go:958` `for _, source := range sources` —— 对 4 个端点顺序同步请求且**不提前 break**（`skins-minimal`、`champions`、`/lol-inventory/v2/inventory/CHAMPION_SKIN`、`/lol-inventory/v1/inventory?...`）。
- `lcu_api.go:561-574 SkinAcquisitionDates` **重复请求同一批端点**（`skins-minimal`、`champions`、`CHAMPION_SKIN`），与 `loadOwnedSkinInventory` 完全重叠 —— **同一份数据抓了两遍**。
- 单端点最慢样本：`/lol-champions/v1/inventories/{id}/champions` 1297~1316ms（log_seq 9428、9459）。
- 并发闸门是 `main.go:98,1081-1085` 的布尔量 `a.syncing`（**已在跑就直接丢弃，不排队**），与 `update.go:311` 的 `busyLocked` 无关（那是应用更新器专用）。

**「识别不到」是两类问题被同一个文案盖住了：**
- `loot_name_fallback` 共 273 条，其中 **270 条集中在 09-13 04:21~08:33 一次会话**（09-07 全程只有 3 条）。
- 但这 273 条里 **243 条（SKIN/CHAMPION）只是"客户端 LocalizedName 字段为空"**，仍能靠 loot_metadata / 皮肤名回填出可读名 —— **不等于用户看到"未知"**（判定逻辑 `lcu_api.go:235-259`，`has_localized_name` 只看字段是否为空，`:252-258` 回退后优先展示原始 LootID）。
- **真正无法命名并被整体丢弃的只有 30 条**（`loot_id_prefix=OTHER`, `raw_key_empty=true`）。
- 前端"查看未识别条目"（文案在 `app.js:718`）的数据源是 `poolIssues`（`main.go:686,1200`），那是**皮肤池匹配失败列表**，与单件战利品命名回退是两回事，目前被同一句话概括。
- `/lol-inventory/v1/inventory` 返回 400 时：`catalog.go:964-982` 非 404 一律标 `state:"failed"` 静默跳过、不上抛；`lcu.go:314-354` 连续 3 次失败或 1 次 404 触发进程内熔断（不落盘，重连即失效）。

### 要做的改动
1. **消除重复抓取**：`loadOwnedSkinInventory`（`catalog.go:935-990`）与 `SkinAcquisitionDates`（`lcu_api.go:561-574`）合并成一次抓取，acquisition 直接复用 ownership 阶段已拿到的响应体（chroma 同理）。预计砍掉约 40% 耗时。
2. **提前退出**：每阶段内 3~4 个候选端点改成"拿到第一个成功且 authoritative 的结果就 break"，不要无条件全跑完。
3. **区分"暂时未就绪"与"真的没有"**：那 30 条真正无法识别的条目，前端应显示"客户端数据暂未同步，可稍后重试"而非笼统"未识别"，并做后台指数退避自动重试（5s / 15s / 30s 三次），减少用户手动点"重新读取"。
4. **拆开两类文案**：`poolIssues`（皮肤池匹配失败）与 loot 命名回退在 UI 上分开呈现，不要共用"识别不到"。

### 必须补的埋点
`requestCollectionRefresh()`（`main.go:1061`）目前**只有成功路径有事件**（`main.go:1207`）。本轮 3 次 `collection_refresh_request` 里，R1（04:21:39）找不到对应的 `refresh_succeeded`，也没有任何失败事件，**无法还原它卡在哪**。请在入口、失败分支、取消分支各补一条。

### 存疑
`poolIssues` 的 reason 文本本轮未展开读取，无法确认是否与那 30 条 `loot_name_fallback` 真实重叠。下次请抓一份带 poolIssues 详情的日志。

### 验收判据
- 同机对照：改动前后各跑 3 次"重新读取"，耗时中位数下降 ≥35%。
- 重复端点请求次数在日志里可数，改动后每个端点每次刷新最多出现一次。
- 构造"LCU 返回空数组"的测试：不得被当成"永久无数据"缓存下来，且前端文案为"暂未同步"。

---

## P5 · 领取功能领取到一半卡住

### 症状
用户原话：「领取这个功能领取到一半会卡住，日志应该有反应」。

### 日志现场
两次完整批次：**05:27:07~05:27:19**（9 项全完成）、**05:52:10~05:52:33**（7 项全完成）。两批最终都归零，**不是永久死锁**。
第二批内部有一次明显停滞：`claim_scan_shape` 在 **05:52:22.080（log_seq 4355）之后停了约 11 秒**，直到 **05:52:33.24（log_seq 4400）** 才领完最后一项 —— 正常节奏是每项 <300ms。
同窗口的旁证：log_seq 4407 `accept_focus_inspection` 记录到真实超时（`error_class:"timeout", ok:false, duration_ms:1201`）；log_seq 4401 `/lol-rewards/v1/grants` max **2284ms**；log_seq 4416 `/lol-missions/v1/missions` max **3047ms` —— 领取扫描与"自动接受"轮询在抢同一条 LCU 连接。

### 已证实的根因（后端为主，前端零超时把它放大成"永久卡住"）
1. `web/suite.js:1760-1789 executeClaims()`：前端**逐项串行**，每项一个 `fetch`，请求体是 `items: [{key, selections}]` —— **一项一个请求**。单项失败走 try/catch **continue**，不会掐灭整批（这点是好的）。
2. `web/suite.js:141-157 api()`：**完全没有超时 / AbortController**（只在 catch 里认 `AbortError`，自己从不 abort）。后端多慢前端就干等多久，进度条冻结在当前 done 数，**无任何提示**。
   ★对照：`web/gameplay.js` 里的 `api()` 是**带 timeout 参数**的（如 `api("/api/gameplay/phase", {}, "gameflow-phase", 8000)`）。两个模块的 `api()` 不是同一个，suite.js 这个是缺的。
3. `claim_center.go:622` `client.claimExecutionMu.Lock(); defer Unlock()` **持锁覆盖整个 handler**。
4. `claim_center.go:624` 执行前一次全量 `scanClaimsObserved`，`:669` 执行后**又一次全量 `scanClaimsObserved`**。
   ⇒ 结合第 1 点：**N 个条目 = N 个请求 = 2N 次全量账本重扫**。9 项就是 18 次。
5. `claim_center.go:227-352 scanEventClaims`：并发上限仅 **4**（`:251 sem := make(chan struct{}, 4)`），每个活动发 2 个 LCU 请求（`:273-289`），**无独立超时**，只吃 `lcu.go:682 http.Client{Timeout: 8 * time.Second}` 的全局兜底，**handler 级别没有总时限**。
   ⇒ 单请求最坏 = 多次 × 8s 叠加；前端又不设超时 → 用户看到的就是"领取到一半不动了"。

### 要做的改动
1. `web/suite.js:141-157 api()` 加 `AbortController` + 超时（建议 15s）与失败提示/重试，别让 fetch 无限等。
2. `claim_center.go handleClaimExecute` 加整体预算 context（建议 10~15s），超时立即返回明确错误，而不是让调用方干等。
3. **合并两次全量重扫**：执行后只需增量确认该 entry 的状态，不必整账本重扫一遍。这是 2N 降到 N 的直接收益。
4. 考虑让前端一次提交多个条目（后端本来就支持 `items` 数组），把 2N 次扫描降到 2 次。

### 必须补的埋点
本轮**没能捕获到"前端一直转圈、后端彻底不响应"的确证案例**，现有证据是"11 秒停滞后自行恢复"的近似案例。
请补：**每项 claim 的 begin/end 配对事件（含耗时、是否超时中断）** + 前端 `claiming` 状态的心跳/看门狗日志。有了这两个才能直接区分"后端真 hang" vs "前端收不到完成信号"。

### 存疑
06:09:44.709（log_seq 5004）扫到 `actionable=2` 后，全日志再无一条 `claim_action`，直到 08:33:24（log_seq 9414）才 `actionable=0`（间隔 2h23m）。但同时刻 log_seq 4990 已触发 play-again 自动匹配、用户随即进入 Matchmaking → ReadyCheck → ChampSelect，**更像"没回来点领取"而非 UI 卡死**，不能当作本症状的证据。

### 验收判据
- 构造后端 hang（慢响应）的测试：前端必须在 15s 内给出超时提示且进度条不冻结。
- 领取 9 项时，日志中 `scanClaimsObserved` 的调用次数从 18 降到 ≤10（或改成批量提交后降到 2）。
- 单项超时后，其余条目仍能继续（不掐灭整批）。

---

## P6 · 斗魂：自动选用"勇敢举动"永不执行 + 自动禁用不生效（★★★根因已锁定）

### 症状（用户截图逐字转录，应用本地时间）
```
17:25:07 ~ 17:25:20  ● 已排定：0.5 秒后亮出选用勇敢举动     ×18 条，几乎每秒一条，全部相同
17:24:58             ● 检测到手动禁用，已让位
17:24:38             ● 客户端禁用列表没有提供任何英雄 ID，并非队友已选择全部候选；已记录原始 ID 与会话状态，等待客户端更新
17:24:38             ● 跳过血港鬼影：客户端可用列表未包含
17:24:38             ● 跳过暗夜猎手：客户端可用列表未包含
17:24:31 ~ 17:24:37  ● 已排定：0.5 秒后提前预选勇敢举动     ×7 条
```
特征：**"已排定"每秒重复，永不出现"已执行/已亮出"的后续条目。**

### 文案 → 代码映射（全部本轮亲自复核）
| 文案 | 位置 |
|---|---|
| `已排定：%.1f 秒后%s英雄 %d` | `champselect.go:1145`（`scheduleChampSelectRequest`） |
| "提前预选" / "亮出" 等动词 | `champselect_execution.go:26-38`（`champSelectDecisionVerb`） |
| `检测到手动%s，已让位` | `champselect.go:918`（`evaluateChampSelect`） |
| `客户端禁用列表没有提供任何英雄 ID…` | `champselect.go:944-946` |
| `跳过英雄 %d：%s`，标签表含"客户端可用列表未包含" | `champselect.go:1068,1071` |
| "勇敢举动" 前端文案 | `web/suite.js:548/613/639/646`，对应后端哨兵值 **championID = -3** |

### 已证实的根因：同一个"是否斗魂"的语义，在两处用了**不同口径**，其中一处漏掉了当前实际在用的 queueId

- **主评估路径**用 groupID：`champselect.go:929,932` `chooseChampSelectCandidate(..., groupID == "arena")`。`groupID` 由 `champselect.go:355 champSelectGroupID()` 算出，`queue_groups.go:109-111` 把 **1700 / 1710 / 1750 三个都登记为 arena**。⇒ 这一步正确，所以"已排定"能正常打出。
- **下发前的二次预检**却用硬编码：**`champselect_execution.go:212`**
  ```
  candidate, reasons := chooseChampSelectCandidate(side, []int64{d.ChampionID}, available, grid,
      config.AvoidTeammateIntent, session.QueueID == 1700 || session.QueueID == 1710)
  ```
  **漏掉 1750，且没有 `GameMode == "CHERRY"` 兜底。**（对照：`gameplay.go:1711 / 2937 / 2956` 三处同类判断都带了 CHERRY 兜底，唯独这里没有。）

- **★决定性证据：用户的斗魂就是 1750。** 本份日志 `match_mode_classified` 三条：
  ```
  {"queue_id":1750,"game_mode":"CHERRY","kind":"arena","map_id":30,"mode_group":"arena",
   "queue_label":"斗魂竞技场 3x6","time":"2026-09-13T04:22:07Z"}   （另两条 05:16:32、09-07 11:54:31 同样是 1750）
  ```

### 失败链（逐跳可复核）
1. `champselect_execution.go:212` 传入 `arena=false`；
2. `champselect.go:429` 的直通规则 `if side == "pick" && arena && championID == -3 { return championID }` **不生效**；
3. 退回按 `grid[-3]` 查表 —— 而 grid 建表时过滤了 `id > 0`（`champselect_execution.go:182-187`），**-3 永远不在 grid 里**；
4. `champselect.go:433-435` 命中 `reasons[-3] = "unknown"`，预检返回该 reason；
5. `champselect.go:1193-1202` 的 `if isBanPick && !champSelectRequestStillCurrent(...)` 分支命中 → **取消本次写入、`delete(r.champSelect.decision, decision.Action)`**，记 `watch_action result=canceled reason=champselect-state-changed`，**从不发出 PATCH**；
6. 下一轮询（~1s）`decision` 已被删除 → `champselect.go:1124` 的 `same-decision` 短路失效 → 重新 schedule → **再打一条"已排定"**。
   ⇒ 完全对应截图里"每秒一条、永不执行"。

### 要做的改动（红线 3：不要只补一个 1750 常量）
1. **`champselect_execution.go:212` 的 arena 判定收敛到与 `champselect.go:929/932` 同一口径** —— 用 `r.champSelect.groupID == "arena"`（该字段存在：`champselect.go:141`，赋值于 `:775`、`:823`），**不要再写 queueId 字面量**。
2. 顺带全仓库排查同类硬编码：`gameplay.go:1711 / 2937 / 2956` 虽有 CHERRY 兜底但口径仍不统一，建议一并收敛到 `queue_groups.go` 的登记表。
3. 加一条**静态守卫测试**：禁止在 champselect 相关文件里出现 `QueueID == 1700` 这类字面量比较（守边界，不要守函数名白名单 —— 上一轮 AST 守卫就是因为只认函数名被绕过的）。

### 斗魂自动禁用不生效（中置信度，两个假设未能互斥排除）
- arena 组定义**是有 ban 的**：`champselect.go:65` `{GroupID:"arena", ..., BanLimit:3, HasBan:true}`。
- `champselect.go:809` `if definition.HasBan && champSelectSessionHasActionType(session, "ban")` 才会去请求 `/bannable-champion-ids`；否则 `bannableIDs` 停留在 `:808` 预置的空切片。
- 空集合最终走到 `champselect.go:944-946`，打出截图里那句"客户端禁用列表没有提供任何英雄 ID"。
- 两个不能互斥排除的成因：
  **(A)** `champSelectSessionHasActionType(session,"ban")`（定义在 `champselect.go:580-589`，逐个比对 `action.Type`）在斗魂三人赛制下判定为 false，**端点根本没被调用**；
  **(B)** 端点调用了但客户端在斗魂下就是返回空（禁用走另一套接口）。
  下游 `champSelectBanAvailability`（`champselect_execution.go:67-106`）对 `queueID != 3110` 直接透传 raw，**两种成因在代码侧现象完全一样**。

### 必须补的埋点（这是能一次钉死 (A)/(B) 的唯一办法）
在 `champselect.go:809` 处加一条独立埋点，把 **`champSelectSessionHasActionType(...)` 的布尔结果** 与 **`/bannable-champion-ids` 的 HTTP 状态码 / 返回长度** 分别打点。目前两者混在同一个空数组里，测不出差异。
另请把 `session.QueueID` 原始值直接打进 `champselect_source`。

### 证据边界（请如实写进报告，不要越界）
- 用户截图时间 17:24~17:25 本地 = **09:24~09:25 UTC**，而日志导出于本地 16:46 = 08:46 UTC（已用日志最后一条 `champselect_trace` @08:46:39Z 核实本地 = UTC+8）。**该次事件不在这份日志里。**
- 全量 12663 行中，`champselect_*` 系列出现过的 `queue_id` **只有 2400（海克斯大乱斗）**，1700/1710/1750 一次都没有 —— **本份日志里没有任何一次斗魂英雄选择**。
- ⇒ "勇敢举动"那条链是**代码级静态锁定 + queueId 实证（1750 确已在用）**，非日志直接观测；"斗魂禁用"只能到两个假设。
- 代码里已有可定位此 bug 的现成埋点：`champDiagnostic("preflight", why, ...)` 会写出 `champselect_trace`，届时应能看到 `stage:"preflight", reason:"unknown", champion_id:-3, group_id:"arena"`。**请让用户在斗魂对局中操作后立刻导出一份日志做最终坐实。**

### 验收判据
- 单元测试：构造 `QueueID=1750, GameMode=CHERRY` 的 session + `ChampionID=-3` 的 pick decision，预检必须放行并真的发出 PATCH。**把 `champselect_execution.go:212` 改回硬编码，该测试必须变红。**
- 同样构造 1700 / 1710，行为不得回归。
- 静态守卫测试：在 champselect 文件里新增一处 `QueueID == 1700` 字面量比较，守卫必须报错。

---

## 附加发现（不在用户清单里，但日志里是 100% 失败，建议本轮一并处理）

### A1 · `position-broadcast`（大乱斗阵营位置播报）成功率 0%
```
watch_action action=position-broadcast result=armed   279 次
watch_action action=position-broadcast result=failed  279 次
```
**279 次 armed / 279 次 failed，零成功。** 代码在 `watch_rules.go:1259 broadcastPosition`，失败出口至少有 4 个（`:1272 / :1280 / :1316 / :1322 / :1334`），而 `:1264` 记录 result 时**不带 reason 字段**（实测该事件只有 `action/event/log_seq/result/run_id/time` 六个键）。
⇒ **第一步只能是补埋点**：把这 5 个失败出口各带一个不同的 reason 值落进 `watch_action`，否则无法区分是哪一条。这个功能 R68 修过，现在又是全红。

### A2 · 一半以上的对局收不到 `EndOfGame`
见 P3 的 ★ 节。14 局里只有 6 局出现 `EndOfGame`。**任何挂在 `EndOfGame` 上的功能都会漏掉一半以上的对局** —— 包括"快速下一把"、"结算自动点赞"（`web/suite.js:240-242` 的 `phaseKey: "EndOfGame"`）。建议一并复查这几个自动动作的实际触发率。

### A3 · `InProgress` 之后普遍出现一次假 `Reconnect`
phase 序列里 14 局中有 10 局在结束前出现 `InProgress → Reconnect → WaitingForStats`。日志里 `watch_action reconnect` **armed 10 次 / canceled 9 次** —— 自动重连功能几乎每局都被这个假 Reconnect 触发一次再撤销。虽然目前没造成可见故障，但属于误触发，建议在 `watch_rules.go:419/445` 的 Reconnect 分支上加一个"持续 N 秒仍为 Reconnect 才动作"的条件。

---

## 执行顺序建议

1. **P6 + P9**（★同一条事件链上的两个 bug，互相放大，必须一起做 —— 详见 P9 的"与 P6 的联动"一节）
2. **P2 + P3**（同属 gameflow 事件链，改动点相邻，建议同批做，避免两次动同一段代码）
3. **P8**（改动最小、纯 Electron 侧，可随时插入）
4. **P5**（前端超时 + 后端预算，各自独立）
5. **P7**（先补埋点，下一轮才可能定位）
6. **P1**（纯 CSS 两行 + 三组护栏判据重写）
7. **P4**（收益明确但改动面最大）
8. **A1/A2/A3**（先补埋点，下一轮再定修法）

---
---

# R87-ADDENDUM（追加，2026-09-13 晚）

新证据：`lol-loot-diagnostics-0913-1951.jsonl`（9761 行，导出于本地 19:51 = UTC 11:51，本地 = UTC+8，覆盖 UTC 06:52~11:51，含 5 次 `app_start`）。
以下三条的 file:line **全部经我本人二次复核**；★特别注意：子代理最初给出的 `suite.js:1850-1857`、`suite.js:648-669` 两处行号**经复核不成立**，下面是重新定位后的真实位置。

---

## P7 · 自动开始匹配一直不成功

### 症状
用户原话：「自动开始匹配一直成功不了，日志应该有记录」。

### 日志实况（先说结论：**这份日志里没有一次失败样本，但暴露了一个更大的诊断盲区**）
- 动作名已在代码里核实为 **`auto-matchmaking`**（`web/suite.js:246` `key:"autoMatchmaking"`；实现 `watch_rules.go:855 scheduleAutoMatchmaking`）。
- 新日志里 `auto-matchmaking` **全天只出现 1 次**：`armed`(11:00:48.507) → `fired`(11:00:49.993, attempts:1)。**这一次是成功的** —— 之后 11:02:09 起 `champselect_trace` 密集连发、11:03:48 进入 `live_load_cost`，说明确实排进了英雄选择并进了对局。
- 旧日志（12663 行）里 `auto-matchmaking` **一次都没出现**，连 `armed` 都没有。
- **★关键异常**：全天 536 次 `/lol-lobby/v2/lobby` GET 中 500 次返回 200（93%），但 **`lcu_lobby_shape` 事件全天 0 次**。而 `watch_rules.go:757 matchmakingPreflight` 每次成功 GET 后必调 `recordLobbyShape`（首次必落盘，去重表初始为空）。
  ⇒ **`scheduleAutoMatchmaking` 这条链路当天绝大多数时间根本没跑起来。**
- 同时 `watch_custom_classification` 显示全天有十几次「在房间但没选队列」（`lobby=true, queue_id=0`，06:53 / 07:18 / 07:35 / 08:00 / 08:10 / 09:04 / 09:25 / 10:35 …）。如果开关当时是开的，这些时刻理应产生 `armed` →（2/4/8s 退避重试 3 次）→ `gave_up`，**但一次都没有**。

### 已证实的头号问题：一个完全不落日志的四态守卫
`watch_rules.go:861`：
```
if r.customSession || r.autoMatchStarted || r.autoMatchInFlight || r.autoMatchExhausted {
    r.mu.Unlock(); cancel(); return false     // ← 静默 return，不产生任何 watch_action 事件
}
```
命中这四个状态中的任何一个时，**日志里连一个字都不会留下**。这就是为什么"用户说一直失败"而"日志里什么都看不到"——**目前这两件事在证据上无法区分"开关没开"和"被守卫吞掉"。**

### 高度怀疑（结构性，且与本工单 A2 直接联动）
`autoMatchExhausted` 的复位路径**只有一条**：`watch_rules.go:389-390`
```
if phase == "None" { ... if previousPhase != "None" { r.autoMatchExhausted = false } ... }
```
而 `watch_rules.go:437-442` 的 `EndOfGame` 分支**只清 `autoMatchStarted`，不清 `autoMatchExhausted`**。
⇒ 结合 A2 的实测（上一份日志 14 局里 phase 几乎从不走到 `None`，只有一次 04:33:39 的 `WaitingForStats → None`）：
**`autoMatchExhausted` 一旦因为"队列没选中重试耗尽"（`watch_rules.go:924/943`）被置上，在整个进程生命周期内几乎永远不会被复位** —— 之后无论进多少次房间，`auto-matchmaking` 都在 `:861` 被静默吞掉，用户看到的就是"一直成功不了"。
这与用户描述高度吻合，但**尚未被日志直接证实**（因为守卫不落日志）。

### 其它已证实的静默失败出口（这些至少还有事件，但 UI 可能无提示）
- `watch_rules.go:766` `skipped_not_leader` —— **不是房主就直接放弃**。
- `watch_rules.go:761` `skipped_custom` —— 自定义房间跳过。
- `watch_rules.go:770-772` / `:931-941` `gave_up` —— 队列未选中、重试耗尽。
- **死代码**：`watch_rules.go:653` 的 `skipNonLeaderMatchmaking` 分支在 `schedule()` 里只被引用一次，但没有任何调用点会传入 `"auto-matchmaking"`（真实入口只有 `scheduleAutoMatchmaking`），**该分支从未被走到**，建议一并清理。

### 要做的改动
1. **【必做·第一优先】`watch_rules.go:861` 的四态守卫补一条诊断**：
   `r.record({"event":"watch_action","action":"auto-matchmaking","result":"skipped_state","reason": <custom|started|inflight|exhausted>})`。
   **不补这条，下一轮还是查不出来。**
2. **【必做】`autoMatchExhausted` 增加复位路径**：在 `watch_rules.go:437-442` 的 `WaitingForStats/PreEndOfGame/EndOfGame` 分支里一并清空（与 `autoMatchStarted` 同处），不要只依赖 `phase == "None"`。
   ★与 A2 同一个病根：**不要再把任何状态复位挂在一个实测极少出现的 phase 上。**
3. **【必做】把 `rules.AutoMatchmaking.Enabled` 的开关状态写进 `watch_settings_client` 事件**（现有该事件已含 `champselect_enabled` / `master_enabled` 等字段，照着加）。否则永远无法从日志判断"当时这个功能到底开没开"，本轮就是卡在这一点上。
4. **【建议】** `skipped_not_leader` / `skipped_custom` 发生时前端给出可见提示（例如"你不是房主，自动开始匹配已跳过"），不要静默。

### 存疑（请如实写进报告）
- 本份日志**没有采集到一次真正的失败样本**，无法证实用户命中的是"非房主" / "队列未选" / "自定义房间" / "exhausted 卡死"中的哪一种。
- 请让用户在下次"点了没反应"时**立刻**导出日志，并同时记下：当时是否房主、房间几人、是否已选队列、本次启动后是否取消过一次匹配。
- `run_id` 在同一份日志里对应 5 次 `app_start`，语义偏向安装级而非进程级，因此**无法用本日志验证"卡死状态是否跨进程重启持续"**。

### 验收判据
- 单元测试：把 `autoMatchExhausted` 置为 true，注入一次 `EndOfGame`，该标志必须被清空（回滚修改则测试变红）。
- 单元测试：四态守卫命中时必须产生一条带 reason 的 `watch_action` 事件，四个 reason 值各覆盖一次。
- 顺带回归：`position-broadcast` 在新日志里仍是 **armed 153 / failed 152（≈99.3%）**，与旧日志 279/279 一致，**A1 依旧成立且未改善**。

---

## P8 · 导出日志后没有变成"打开日志所在目录"按钮

### 症状
用户原话：「导出日志没有变成打开日志所在目录按钮」。

### 结论先行：**功能四层代码全都实现了，接线也正确，断点在"目录来源"这一跳**
链路（全部复核过）：
| 跳 | 位置 | 状态 |
|---|---|---|
| 按钮 | `web/index.html:321` `#export-diagnostics`（`<a href="/api/diagnostics/log" download>`） | ✅ |
| 点击 | `web/app.js:3038-3062`：未就绪时 preventDefault → flush → 手动建 `<a>` 下载；就绪时调 `window.desktopDiagnostics.openFolder()` | ✅ |
| 文案切换 | `web/app.js:3031-3035` `window.desktopDiagnostics?.onCompleted(...)` 里置 `diagnosticsExportReady=true`、文案改为"打开日志文件夹" | ✅ |
| preload 暴露 | `desktop/preload.cjs:56-62` `contextBridge.exposeInMainWorld("desktopDiagnostics", {onCompleted, openFolder})` | ✅ |
| 主进程 handler | `desktop/main.cjs:534-539` `ipcMain.handle("desktop-diagnostics-open-folder")` → `shell.showItemInFolder` | ✅ |
| 下载钩子 | `desktop/main.cjs:521-532` `attachDiagnosticsExport(...)` → `desktop/diagnostics-export.cjs:52` `session.on("will-download")` | ✅ |
| **目录来源** | `desktop/main.cjs:525` `getDirectory: () => windowShareExportController.getSaveDirectory({...}).directory` | ❌ **就是这里** |

### 已证实的断点
`desktop/diagnostics-export.cjs:14-17`：
```
let directory;
try { directory = getDirectory(); } catch (_) { return; }
if (!directory || !path.isAbsolute(directory)) return;     // ← 直接 return
```
目录为空时**直接 return**，于是 `item.setSavePath()`（`:50`）不会调用、`item.once("done", ...)`（`:28`）不会注册 → **`onCompleted` 永不触发 → IPC 永不发送 → 按钮永远停在"导出诊断日志"**。文件本身仍会被 Electron 的原生"另存为"对话框存下来（`:26` 注释原文 "Fall back to Electron's normal Save dialog"），所以**用户感觉"导出成功了，但按钮没变"** —— 完全吻合。

而这个目录来自**分享图的保存目录**：`desktop/share-export.cjs:190` `let saveDirectory = "";`，只有用户曾经生成过分享图并在弹窗里选过位置（`persistDirectory`，`:212-220`）才会非空；`:234-238 getSaveDirectory` 还会在目录不可用时把它重置为 `""`。
`web/index.html:299` 也印证了这个设计：「导出位置 / **分享图与诊断日志共用** / 首次生成时选择」，默认文案就是"首次生成时选择"。
⇒ **只要用户从没手动选过导出位置（首次导出日志的用户几乎都是），这个 bug 必现。**

### 第二个失败模式（独立存在，也要一起修）
`web/app.js:3031`：
```
window.desktopDiagnostics?.onCompleted(() => {
  if (state.section !== "settings" || state.settingsPage !== "privacy") return;   // ← 离开该子页就丢弃
  ...
});
```
再配合 `web/app.js:3036-3038` 的 `deep-legends:section` 监听会 `resetDiagnosticsExport()`。
⇒ 即使目录问题修好了，**只要用户在导出完成前切走页面（或不在"隐私"子页），完成信号也会被静默丢弃**。导出是个有延迟的异步动作，这个窗口期很容易踩中。

### 要做的改动
**不需要动 IPC / preload（已完整）**，只改目录来源：
1. `desktop/main.cjs:525` 的 `getDirectory` 不再复用分享图目录。二选一：
   - (a) 为诊断日志单独维护目录设置，首次导出时走一次 `requestDirectory` 弹窗并 persist；
   - (b) **更简单、推荐**：改 `attachDiagnosticsExport` —— 没有已知目录时主动 `dialog.showSaveDialog` 拿到用户选的路径再 `setSavePath`，这样无论有没有配过分享目录都能拿到 `destination` 并触发 `onCompleted`；再不行就退回 `app.getPath("downloads")`。
2. `web/app.js:3031` 的 `onCompleted` 回调**去掉 section/settingsPage 的前置判断**，改为无条件记下"已完成 + 文件路径"，由渲染时决定是否显示按钮。
3. 顺带：`web/index.html:299` 的文案"分享图与诊断日志共用"若按 (a) 拆开了目录，需要同步改。

### 浏览器环境的降级（现状是对的，不要动）
`window.desktopDiagnostics` 为 `undefined` 时 `?.onCompleted(...)` 静默跳过、`diagnosticsExportReady` 恒 false、按钮点击走普通 `<a download>` 浏览器下载。**这条降级路径设计正确**，不是本 bug 的涉及点。

### 证据边界
日志里检索到 3 条 `diagnostic_delivery_client`(reason=export) 对应 3 次 `/api/diagnostics/log` 请求（log_seq 9618 / 15817 / 15961），均正常 —— 证明**后端每次都成功把文件吐出去了**。但这份日志是纯 Go 后端日志，**不采集 Electron 主进程事件**（检索 `will-download` / `saveDirectory` / `showItemInFolder` 均 0 命中），因此"断在 Electron 侧"是**代码走查 + 日志交叉印证的强证据链，不是直接抓包**。
★若要最终坐实，需要 `%APPDATA%\deep-legends-desktop\logs\desktop.log`（注意：这个文件**不在应用内导出的日志里**，见旧记忆 R84 的发现）。

### 未证实的另一种可能
若用户之前确实设置过导出位置（例如存过分享图），而该目录后来被删除或不可写，`share-export.cjs:234-236` 会静默把 `saveDirectory` 重置为 `""`，**同样复现本 bug**。本轮无法证实或证伪，但上面的修法对两种情况都有效。

### 验收判据
- 全新用户资料目录（从未设置过导出位置）下点一次导出：文件存下来 **且**按钮变为"打开日志文件夹"，点击能定位到文件。
- 导出发起后立刻切到别的设置子页再切回：按钮仍应为"打开日志文件夹"。

---

## P9 · 征召"编辑序列"弹窗：滚动自动回顶 + 输入框打不了字

### 症状
用户原话：「征召里面出现了一次编辑序列弹窗里面滚动到下面自动回到顶部，输入框也输入不了」。

### 已证实的机制：弹窗和整个征召面板是**同一坨 innerHTML**，外部事件一来就整块重建
1. **`web/suite.js:757`**（`renderChampSelect` 结尾）：
   ```
   roots.champselect.innerHTML = `<section class="suite-card cs-master">…</section>…${renderChampSelectDialog()}`;
   ```
   → `<dialog class="cs-dialog">`（由 `web/suite.js:649-669 renderChampSelectDialog()` 生成）**是这坨字符串的一部分**，每次 `renderChampSelect()` 都把它连同整个面板一起销毁重建。
2. **`web/suite.js:759`**：`if (state.champSelectDialog) requestAnimationFrame(() => { … if (dialog && !dialog.open) dialog.showModal(); })` —— 重建后重新 `showModal()`。
   ⇒ **新建的 `<dialog>` 的 `scrollTop` 天然是 0，`showModal()` 的 autofocus 行为又把焦点丢给第一个可聚焦元素（关闭按钮）。** 两个症状一次齐活。
3. **输入框的值是受控的**：`web/suite.js:668` 的 `<input type="search" value="${escapeHTML(dialog.query || "")}" data-cs-dialog-search>` —— 值来自 `state.champSelectDialog.query`。外部重建时用旧 `query` 覆盖，**用户正在输入但尚未反映到 state 的字符被吞掉**。
4. **现有的焦点保护覆盖不到这个输入框**：`web/suite.js:743-744`
   ```
   const focusedTime = roots.champselect.querySelector("[data-cs-time]:focus");
   if (settings.enabled && focusedTime && …) return;
   ```
   **只护 `[data-cs-time]`，不护 `[data-cs-dialog-search]`。**
5. 反过来，**搜索框自己触发的重渲染是有保护的**：`web/suite.js:929-938` 的 `input` 监听会存 `selectionStart`、渲染后 rAF 里 `focus()` + `setSelectionRange()`。
   ⇒ **所以"自己打字"本来是好的，坏的是"外部事件插进来"那一下。** 请不要误判成输入框绑定写错了。

### 触发整块重建的三条外部事件链（全部复核过行号）
| 位置 | 触发源 |
|---|---|
| `web/suite.js:1887-1894` | `deep-legends:gameflow` —— 来自 `web/app.js:3240-3243`，把 SSE 的 `gameflow:<phase>` **和 `champselect:changed`** 转成同一个事件；命中后 `state.champSelectRuntime = null; void loadChampSelect(false)` → `renderChampSelect()` |
| `web/suite.js:1042-1045` | `watch:session:champselect-*` |
| `web/suite.js:1069-1073` | `watch:<kind>:champselect-*` —— **每一次禁用/选用动作的 armed / fired / failed 都会触发一次** |
后端侧：`champselect.go:1068 r.emit("champselect:changed")`，经 `connection_manager.go:383-396` 的 **300ms** 防抖（`connection_manager.go:21 champSelectEventDebounce = 300 * time.Millisecond`）后广播。英雄选择阶段这个广播可以 **<1s 一次**。

### 真 Chromium 实测（窄视口 480×700，外部重建用脚本注入）
- 滚动条内容高 558 / 可视 360，手动滚到 **scrollTop = 198px**；**仅 1 次外部重渲染后 scrollTop 立即归零（重置率 100%）**。
- 连续输入 `ahri` 共 4 个字符，期间每 400ms 泵一次外部重渲染（3.5 秒内 6 次）：**输入框最终值只剩 `"a"`（4 个字符丢了 3 个）**，且 `document.activeElement` 变成 `.cs-dialog-close` 按钮而非输入框。
- 截图：`outputs/cs-dialog-scroll-reset.png`、`outputs/cs-dialog-repro-final.png`。

### ★与 P6 的联动（这是"为什么偏偏这次遇到"的答案，两条必须一起修）
P6 里那个"每秒一条『已排定：0.5 秒后亮出选用勇敢举动』"的死循环，**每一拍都会 emit 一次 `watch:armed:champselect-pick`** → 命中 `web/suite.js:1069-1073` → **每秒一次整块重建**。
⇒ 用户正是在斗魂征召卡死的同时打开编辑序列弹窗，才撞上这个 1Hz 的重建。
**只修 P6 会让本问题的出现频率大幅下降但不消失**（正常英雄选择阶段仍有 <1s 的 `champselect:changed`）；**只修 P9 则 P6 的死循环仍在**。两条一起做。

### 要做的改动（按推荐顺序）
1. **【止血·先做】把 `web/suite.js:743-744` 的焦点保护扩展到弹窗**：重渲染前若弹窗内有元素持有焦点（尤其 `[data-cs-dialog-search]`），保存 `scrollTop` + `activeElement` 选择器 + `selectionStart/End`，渲染后在 rAF 里恢复 —— 直接复用 `web/suite.js:929-938` 已有的那套写法。成本最低。
2. **【治本·主修】把弹窗从 `renderChampSelect()` 的 innerHTML 里拆出来**，作为独立渲染单元：弹窗打开期间，外部事件只更新面板主体，**不触碰 `<dialog>` 子树**（或只做增量 diff）。这样 scrollTop 和焦点天然不会丢。
3. **【配套】搜索框改为非受控**：只在 `dialog.query` 确实发生变化时才写 `value`，用户正在编辑时不要用外部值覆盖。
4. **【配套】** `renderChampSelect()` 里那个无条件 `showModal()`（`web/suite.js:759`）在弹窗已 open 时不应重新触发。

### 必须补的埋点
**弹窗的开 / 关 / 打开期间被重渲染，目前零埋点** —— 日志里完全看不出用户何时打开过这个弹窗。
（注：`watch_settings_client` 只记 `renderWatch`，且它约 30~40 秒一条是 `runtime.js:142` 客户端 30 秒去重窗口的产物，**不能用它推断真实 renderChampSelect 频率**；`champselect_delivery` 只是每 100 个 LCU 事件的计数采样。）
建议新增 `champselect_dialog_client` 事件，记录 `open` / `rerender-while-open` / `close` 三态及时间戳。

### 验收判据
- 前端测试：打开弹窗 → 滚动到 200px → 注入一次外部重渲染事件 → `scrollTop` 必须仍为 200。
- 前端测试：在搜索框输入 4 个字符期间注入 6 次外部重渲染 → 输入框最终值必须是完整的 4 个字符，且 `document.activeElement` 仍是该输入框。
- 两条测试在回滚修改后都必须变红。
