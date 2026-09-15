# WORKLIST-R94 — 重开局等快速对局场景下 gameId 跳变但 phase 不变，导致清空逻辑失效

诊断日期：2026-09-14
来源：用户上传3份诊断日志（0913-1646/1951/2329），追问"重开局是否会导致上一局数据没清空、下一局推荐没更新"，并具体问软件怎么定义重开局、有没有正确判断重开局结束。

---

## 背景（先说清楚 R91 已经做了什么，避免这次和 R91 重复）

`WORKLIST-R91-LIVE-STALE-DATA.md` 已经执行完（`docs/r91-execution-ledger.md` 存在，`web/gameplay.js` 已有 `invalidateLiveForNewGame`、`shouldResetLiveGameScopedState` 等函数），解决的是"新一局开始时 `state.live` 不清空、页面继续渲染上一局推荐"这个问题。**这部分已验证有效，不需要重做。**

但 R91 的清空触发条件，目前**完全锚定在 `phase !== state.beacon.phase` 这一个字符串比较上**（`web/gameplay.js:6161 handleGameplayPhase` 附近）——只要收到的 phase 文本和上一次记录的相同，就不会触发清空，不管背后其实是不是已经换了一局。

这次日志分析发现了一种 R91 没覆盖到的场景：**gameId 已经变了，但 phase 前后读到的都是同一个字符串（比如一直是 "InProgress"），导致清空逻辑完全不会被触发**。

---

## 已证实：日志里三次 gameId 跳变，phase 字符串全程未变

三份日志（同一次 app 运行，`run_id=64360d199e1649ee53e05b35`）里，共有 3 处 `live_roster_rendered` 事件呈现"前后两条记录 phase 字段都是 `InProgress`，但 `game_id` 换了，且间隔只有个位数到十几秒"：

- `12:09:00.472` game=8980574903(InProgress, 队列1750斗魂) → `12:09:10.103` game=8980593274(InProgress)，间隔 **9.6秒**
- `13:36:07.757` game=8980777424(InProgress, 队列2400) → `13:36:09.980` game=8980855243(InProgress)，间隔 **2.2秒**
- `14:00:44.579` game=8980858422(InProgress, 队列2400) → `14:00:49.142` game=8980931145(InProgress)，间隔 **4.6秒**

正常一局从排队到结束不可能在几秒钟内完成，这三次高度符合"重开局/异常快速结束的对局"特征——真正发生的过程应该是 `InProgress → EndOfGame/Reconnect → Lobby/ChampSelect → InProgress`，但轮询或 SSE 采样间隔没能捕捉到中间那段极短的相位窗口，两次相邻的渲染记录看到的 phase 文本"恰好"都是 InProgress，`phaseChanged` 判定为 false，`invalidateLiveForNewGame` 完全不会被调用。

**另有一条独立证据（不是重开局，是正常对局，但同一类 staleness 机制的普通实例）**：日志3中一局大乱斗（KIWI, 队列2400）ChampSelect 于 `12:49:32` 开始，`12:49:34` 转 InProgress，但直到 `12:50:11` 才渲染新数据，且这次渲染记录的 `phase` 字段仍标记为 `"ChampSelect"`——即 InProgress 已经开始37秒后，页面还在渲染带着旧相位标签的数据。这条说明即便是 R91 覆盖的普通场景，清空/过渡态的生效也不是瞬时的，有一定延迟窗口，可以顺带核实是否在预期范围内。

## 已证实：软件目前对"重开局"的定义，和实时对局页完全是两套互不相干的逻辑

- `gameplay.go:2966 isRemakeGame(explicit, surrendered, duration, ...)` 是全仓库唯一判断"这局是不是重开局"的逻辑，依据 Riot 返回的 `GameEndedInEarlySurrender` 字段 + 5分钟时长兜底，**但它只用于赛后战绩统计**（胜率/赛季统计要不要把这局算进去，`season_stats.go:174/211` 调用），跟实时对局页完全无关。
- 实时对局页（`invalidateLiveForNewGame`/`handleGameplayPhase`）**没有任何"这是不是重开局"的概念**，只是原样转发 LCU 推送的 `gameflow-phase` 字符串，靠"phase 文本变了没有"判断新一局是否开始。

**这意味着**：不是软件"重开局判断错了"，而是实时页根本不判断重开局这件事，它依赖的信号（phase 文本变化）在重开局这种异常快速收尾的场景下，有较高概率被轮询间隔"跨过去"而完全捕捉不到，这是本工单要补的缺口。

---

## 要做的改动

### 1. 清空/刷新触发条件补上 gameId 比较，不能只看 phase 文本

`web/gameplay.js` 的 `handleGameplayPhase`（约6161行）和 `invalidateLiveForNewGame`（约3749行）目前只在 phase 字符串变化时触发清空逻辑。请改为：**phase 文本变化 OR 新数据的 gameId 与 `state.live` 当前记录的 gameId 不一致，两者任一满足就触发**。`shouldResetLiveGameScopedState`（约3805行）里已经有 gameId 比较逻辑，但目前只用来清推荐缓存，**没有联动去清空 `state.live` 本身、也没有触发 R91 新增的过渡态UI**，这次要把这条判断和 R91 的清空/过渡态逻辑打通。

### 2. 排查轮询/SSE 是否存在"错过短暂相位"的采样盲区

需要确认 `/lol-gameflow/v1/gameflow-phase` 的事件转发（`connection_manager.go:226` 附近）在极短相位窗口（重开局典型的几秒到十几秒 EndOfGame/Lobby/ChampSelect 过渡）内，是否存在事件被合并、节流丢弃、或轮询间隔本身就大于这个窗口导致跳过的情况。如果确认存在，评估是否需要在检测到 gameId 跳变时，不管 phase 转发是否完整，都能兜底触发一次完整刷新。

### 3. 补充诊断埋点

在 `handleGameplayPhase` 和 `invalidateLiveForNewGame` 内新增诊断日志，记录：每次收到的 phase 原始值、来源（SSE 推送还是轮询）、以及本次 gameId 与 `state.live` 记录的 gameId 的比较结果（一致/不一致/首次无历史值）。这样下一轮如果真机复现重开局场景，能直接从日志确认是否命中这条修复路径，而不用再靠"渲染记录时间戳反推"这种间接方式。

### 4. 明确边界，别改错地方

`isRemakeGame`（`gameplay.go:2966`）和实时对局页判定是两套完全独立的逻辑，**这次不需要改 `isRemakeGame`**，也不需要让实时页去调用它或参考它的判定结果——实时页要解决的是"gameId 变了但没感知到"，跟"这局算不算重开局"这个统计口径问题无关。请在改动说明里明确写一句避免执行方混淆方向。

---

## 验收判据

- 构造测试：模拟连续两次 `loadLive` 返回结果 phase 文本相同（都是 InProgress）但 gameId 不同，验证清空/过渡态逻辑被触发，不能因为 phase 没变就放过。
- 构造测试：phase 文本和 gameId 都不变的正常轮询刷新，不应触发清空/过渡态（防止这次改动过度触发，把普通同局刷新也搞成频繁闪烁）。
- 如果第2条排查确认轮询/SSE存在采样盲区且有可行的兜底方案，需要有对应测试验证兜底生效；如果排查后认为不可行或不必要，需要在执行说明里写清楚原因，不能跳过不提。
- 新增诊断埋点：构造一次 gameId 变化的场景，确认日志里能看到本工单要求记录的字段（phase原始值/来源/gameId比较结果）。
- 回归：确认这次改动没有影响 R91 已验收的"phase 文本变化触发清空"这条原有路径。
