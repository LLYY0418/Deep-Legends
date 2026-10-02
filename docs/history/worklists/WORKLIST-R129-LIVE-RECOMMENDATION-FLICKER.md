# WORKLIST-R129：对局页海斗推荐每隔几秒整块重绘（图标闪成文字、页面跳动）

诊断人：Claude（只读：屏幕录像 `20260922-1212-23.mp4` 逐帧 + 日志 `lol-loot-diagnostics-0922-2016.jsonl` + 代码，未改仓库代码）。
执行人：GPT。
日期：2026-09-22。
状态：待执行。根因已由录像、日志、代码三方对上；另附其他模式的影响范围。
关联：R128 §2.3 要删的对局页统计口径页脚也在 `renderRecommendationArea` 里，两单一起做时注意合并。

---

## 0. 结论

英雄选择阶段对局页每 **3 秒**自动刷新一次（`liveRefreshDelayMs` 对 ChampSelect 写死 3000ms）。每次刷新 `loadLive` 会调用两次 `renderLive`：开始时 `state.liveLoading = true` 一次，结束时 `false` 一次。`renderLive` 本来有一道"标记没变就不重绘"的保护（比较 `liveRecommendationMarkup` 生成的整段 HTML 字符串），但这段 HTML 里包含隐藏的「详情」tab，而详情 tab 里的玩家战绩区会**根据 `state.liveLoading` 输出骨架屏或"暂无样本"**。判断条件里把 `historyState` 当成 `ready/empty/unavailable/failed` 四种，可后端实际下发的是 `ok/empty/failed/unavailable`——**"ok" 不在列表里**。于是只要有一位玩家历史状态是 `ok`、但当前模式没有近期对局和模式战绩（海斗英雄选择阶段本人就是这种情况，日志 `with_stats: 0`），每次刷新开始时 HTML 变成骨架屏版本，结束时又变回来：**每 3 秒整块 `innerHTML` 重建两次**。

重建后所有海克斯图标（走懒加载队列 `data-queued-src`）先显示名称首字占位，再重新加载——就是录像里图标闪成"物""重""海"文字的现象；整块内容高度瞬间变化，滚动位置随之跳动。推荐数据本身没有重新请求（日志 `live_recommendations_skip reason: cached`），纯前端重绘问题。

---

## 1. 证据

### 1.1 录像（8.4 秒，按 4 帧/秒抽帧比对）

- 录像文件名时间 12:12:23（UTC，对应本地 20:12），画面是河流之王（英雄 223）的海斗英雄选择阶段对局页。
- 帧间差异：0.50–1.00s、1.75–2.25s、5.00–5.50s 三次大面积变化，其余时间静止；其中 0.5s 和 5.0s 两次都是**连续两帧**变化（一次刷新里两次重绘）。
- 第 8、21 帧：海克斯推荐九张卡的图标全部变成名称首字（`remoteStaticIcon` 的占位 `<span>`），下一帧恢复；同时顶部英雄头像没闪（浏览器内存缓存命中、立即 complete），说明是整块 DOM 被替换而不是图标单独失败。
- 第 1→4 帧：页面整体上移约 200px，"海克斯与出装/详情"页签和英雄条重新露出——重建导致的滚动跳动。

### 1.2 日志（12:12:07–12:12:38 这一局英雄选择）

- `live_load_cost`（后端收到的 `/api/gameplay/live` 请求）：12:12:19.57、22.65、25.72、28.80、32.07、35.19……间隔约 3.1s，正是 ChampSelect 的 3 秒刷新。
- 每次刷新对应一条 `gameflow_phase_client reason: invalidate source: interval invalidated: false`——没有触发换局清空，排除"误判新对局"。
- `live_recommendations_skip reason: cached`（12:12:19.63）：推荐数据走缓存，没有重新请求，排除"数据变了"。
- `live_roster_shape`：`players: 1, with_stats: 0, raw_count: 0`——本人一位、没有模式战绩，满足下面代码里的触发条件。
- 本日志里只有海斗（queue 3270）的英雄选择，其他模式无样本，其他模式的结论见 §1.4 的代码分析。

### 1.3 代码

- `gameplay.js:125-128` `liveRefreshDelayMs`：ChampSelect 固定 3000ms。
- `gameplay.js:4370-4446` `loadLive`：`:4388` 开始时 `renderLive()`（此时 `state.liveLoading = true`），`:4440` finally 里 `state.liveLoading = false` 后再 `renderLive()`。
- `gameplay.js:5150-5168` `renderLive`：`nodes.liveContent._recommendationMarkup === markup` 时只更新状态条，否则整块 `nodes.liveContent.innerHTML = …` 并 `prepareImages`。
- `gameplay.js:5345-5407` `renderRecommendationArea`：所有页签面板都拼进 markup（非当前页签只是 `hidden`），所以隐藏的「详情」tab 内容也参与比较。
- 触发点两处，条件完全相同：
  - `gameplay.js:5253` `renderLivePlayer`：`historyPending = state.liveLoading === true && !player.recentGames?.length && !Number(stats.games) && !["ready","empty","unavailable","failed"].includes(player.historyState)`
  - `gameplay.js:5513` `renderInsightMatches`：同一判断，输出 `is-history-pending` 骨架行。
- 后端取值：`gameplay.go:7825-7835` `liveHistoryState` 只返回 `unavailable / ok / empty / failed`；`arena_live_grouping.go:250` 固定 `unavailable`。全后端没有地方产出 `"ready"`，也没有产出 `"pending"`（`gameplay_refresh.go:284` 只是读取判断）。所以前端列表里的 `"ready"` 永远匹配不上，`"ok"` 永远被当成"还在读取中"。

### 1.4 其他模式会不会闪

同一段代码对所有模式生效，触发条件是"刷新进行中 + 至少一位玩家 `historyState` 为 `ok` 且当前模式没有近期对局和模式战绩"：

| 场景 | 刷新间隔 | 是否会闪 |
|---|---|---|
| 海斗 / 极地大乱斗 英雄选择 | 3s | **会**，本人和队友经常没有该模式近期战绩，最容易触发（本次录像） |
| 斗魂竞技场 英雄选择 | 3s | **会**，同上 |
| 排位 / 匹配 英雄选择 | 3s | 有任意一名队友当前模式无近期对局（新号、久未玩该模式）就会闪 |
| 游戏内（InProgress） | 20s，快照完整后停止；斗魂分组探测期 5s | 概率低，但斗魂开局 60 秒内每 5 秒一次，条件满足时同样会闪 |
| 大厅 / 无对局 | 不自动刷新 | 不会 |

另外，即使不满足上面的条件，任何一次真实的数据变化（队友锁英雄、战绩读完）也会触发**整块**重建，图标同样会闪一下——这是 P2 要处理的"放大器"。

---

## 2. 修复方向

### P1　去掉 markup 里对 `state.liveLoading` 的依赖，并对齐历史状态词表（根因）

1. `gameplay.js:5253` 和 `:5513`：骨架屏只由后端给出的状态决定，不看 `state.liveLoading`。后端现在不会下发 `pending`，所以这两处的骨架分支实际上等于"整场快照还没回来时"才需要——那种情况 `renderLive` 在 `:5137-5140` 已经整块显示骨架了，这里可以直接删掉 `historyPending` 分支；如果想保留，就改成 `player.historyState === "pending"`（与 `gameplay_refresh.go:284` 同一个词）。
2. 前端所有历史状态判断统一用后端词表 `ok / empty / failed / unavailable`（`"ready"` 全部替换为 `"ok"`），抽一个 `liveHistorySettled(player)` 小函数给两处共用，避免以后再各写一份。
3. 规则写进 `renderLive` 上方注释：参与 `_recommendationMarkup` 比较的 HTML 只能由快照数据和用户选择决定，不能包含 `liveLoading`、计时器、在途请求数这类瞬时状态；加载反馈只放在 `data-live-status` 状态条里。

### P2　真实变化时也不要整块重建（减少闪烁面）

`renderLive` 目前是"整段字符串一变就全部替换"。改成按面板比较：给每个 `recommendation-panel`（build / insight / runes）和状态条各自记一份上次的 markup，只替换变了的那个面板的 `innerHTML`；替换前记录 `appScroll`/`.app-main` 的 `scrollTop`，替换后恢复。这样队友战绩读完只会重绘隐藏的「详情」面板，用户正在看的海克斯与出装面板里图标不动。

### P3　诊断

新增 `live_render_rebuild` 诊断：每 60 秒聚合一次，记录各面板（status/build/insight/runes/整块）被替换的次数与触发来源（interval/sse/manual/catalog）。修复后同一局英雄选择里，数据没变时这些计数应为 0。

---

## 3. 验收

- **jsdom 回归测试**（新增到 `gameplay.test.cjs` 或 `r129.test.cjs`）：构造海斗英雄选择快照（1 名本人玩家，`historyState: "ok"`，`recentGames: []`，`modeStats: {}`，推荐数据含 9 条海克斯），先 `state.liveLoading = true` 调 `renderLive()`，再 `false` 调一次；断言 `nodes.liveContent.innerHTML` 的 setter 只在第一次被调用、海克斯 `<img>` 节点前后是同一个对象。**对抗变异**：把 P1 的改动撤回，这条测试必须 FAIL。
- 同一测试再跑 `historyState` 分别为 `empty`/`failed`/`unavailable` 的情形，全部不重建。
- P2 测试：只改变 insight 面板的数据（队友 `recentGames` 从空变成有），断言 build 面板里的 `<img>` 节点对象不变。
- 真机：海斗、斗魂、排位各进一次英雄选择，录 30 秒屏，海克斯/装备图标不闪、页面不跳；导出日志里 `live_render_rebuild` 在数据不变期间为 0。

---

## 4. 验收核对（2026-09-22 执行后复核）：旧版对局页测试因新依赖抛 ReferenceError，需要各自补依赖

结论：P1/P2/P3 三项修复本身完全正确——本工单新增的 `r129.test.cjs` 11 条用例（含 §3 要求的对抗变异测试）全绿；但 `renderLive`/`loadLive`/`bindLiveContent` 新增的 `liveRenderTriggerLabel`、`recordLiveRenderRebuild`、`updateLivePanels`、`bindLivePanelScope` 这几个依赖，没有同步补进 R87/R90/R91/R91-addendum/R94/R95/R113 这几份旧对局页回归测试各自独立维护的编译沙箱（这几份文件不共享 `compileFunctions` helper，是各自用 `vm.runInNewContext`/`Function(...)` 手写一份函数抽取 + 桩依赖），导致 `node --test backend/web/*.test.cjs` 40 处失败里有 36 处是同一类 `ReferenceError`，不是逻辑倒退。

### 4.1 现象（2026-09-22 复核，`node --test backend/web/*.test.cjs`）

| 文件 | 失败/总数 | 报错 |
|---|---|---|
| r91.test.cjs | 13/13 | `liveRenderTriggerLabel is not defined` |
| r94.test.cjs | 9/14 | 同上 |
| r95.test.cjs | 7/19 | 同上 |
| r87.test.cjs | 2/6 | 同上 |
| r90.test.cjs | 2/6 | 1 处 `liveRenderTriggerLabel is not defined`（loadLive 相关 compile 调用），1 处 `bindLivePanelScope is not defined`（`r90.test.cjs:78` 单独编译 `bindLiveContent` 那处） |
| r91-addendum.test.cjs | 2/15 | 1 处 `liveRenderTriggerLabel is not defined`，1 处 `liveHistorySettled is not defined` |
| r113.test.cjs | 1/8 | `liveHistoryStateOf is not defined`（`r113.test.cjs:78` 单独编译 `renderLivePlayer` 那处） |

其余 4 处失败与本工单无关，记在 `WORKLIST-R128-...md` §4（`r116b.test.cjs`/`r116d.test.cjs` 里两处旧断言残留 + 一处既存无关问题）。

### 4.2 根因

这几份文件各自手写沙箱，把 `renderLive`/`loadLive`/`bindLiveContent`/`renderLivePlayer` 等函数按名字从源码抠出来单独跑，其余被调用的东西全部以 `noop`/桩函数塞进 `context`/`deps`（例：`r94.test.cjs:24-31` 的 `context = {...bindLiveContent: noop, applyRenderedMetricStyles: noop, prepareImages: noop}`；`r90.test.cjs:78` `compile(["bindLiveContent"], {...})`；`r113.test.cjs:78` `compile('gameplay.js',['renderLivePlayer'],{state:{},...})`）。

本工单 P1 把 `renderLivePlayer`/`renderInsightMatches` 里内联的历史状态判断抽成了 `liveHistoryStateOf`/`liveHistorySettled`（`gameplay.js:143-151`）；P3 让 `loadLive` 每轮加载开始时记一次触发来源（`liveRenderTriggerLabel`，`gameplay.js:4389` 附近）、`renderLive`/`bindLiveContent` 渲染与绑定时调用 `recordLiveRenderRebuild`/`bindLivePanelScope`。这些新函数没有被任何旧文件的 `names`/`deps` 列表收录，沙箱里就是自由变量，一调用就抛 `ReferenceError`。

### 4.3 修复方向

不改这几份旧测试原本要验证的逻辑（状态机、轮询、DOM 复用），只给各自的沙箱补齐新增依赖：

1. **`liveRenderTriggerLabel`**（7 个文件都要加）：纯函数、无外部依赖，可以直接加进各文件的 `names`/extract 列表按真实实现跑（如 `r94.test.cjs:39` 的 `names` 数组末尾加一项），或者桩成 `() => "direct"`——这几份测试都不断言触发来源文案，桩值不影响判据。
2. **`recordLiveRenderRebuild`**（凡真实抽取了 `renderLive` 的文件都要加）：桩成 `() => {}`。
3. **`updateLivePanels`**（同上，凡真实抽取了 `renderLive` 的文件）：桩成 `() => false`——这样 `renderLive` 会退回本工单之前"标记没变就不动、变了就整块重建"的老路径，正好对上这批旧测试本来就在断言的"DOM 节点复用/整块替换"语义，不用把 P2 的分面板增量替换逻辑一起引入这些旧测试。
4. **`bindLivePanelScope`**（`r90.test.cjs:78` 单独抽取 `bindLiveContent` 的地方）：跟同一个 deps 对象里已有的 `bindRuneWorkspaceControls`/`bindPlayerLinks` 一样桩成 `() => {}`。
5. **`liveHistoryStateOf` / `liveHistorySettled`**（`r113.test.cjs:78`、`r91-addendum.test.cjs` 里直接编译 `renderLivePlayer`/`renderInsightMatches` 的地方）：建议按真实实现加进编译列表，不要桩掉——这两个函数正是本工单 P1 修的那个 bug 的核心判据，桩掉会让这几条旧测试以后测不出同类回归。

### 4.4 验收

- `node --test backend/web/*.test.cjs` 全绿（当前 617/657；本节改完、且 `WORKLIST-R128-...md` §4 的三处断言也删掉后应为 654/654）。
- 每个文件补完依赖后，原有断言一律不改（`liveHistoryStateOf`/`liveHistorySettled` 除外——那两处按 §4.3-5 用真实实现），不允许为了让测试通过而放宽判据。
