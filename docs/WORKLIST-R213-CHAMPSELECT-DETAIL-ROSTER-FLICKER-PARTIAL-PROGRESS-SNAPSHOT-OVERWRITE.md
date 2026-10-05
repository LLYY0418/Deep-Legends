# WORKLIST-R213：英雄选择阶段「详情」战绩与标签反复闪烁（后台刷新的逐人进度快照覆盖了完整数据）

诊断：Claude（逐帧看录屏 `屏幕录制 2026-10-04 214901.mp4`，对照日志 `lol-loot-diagnostics-1004-2151.jsonl`，并只读核对源码）。执行：GPT。日期：2026-10-04。
基线：0.12.71（指纹 `696da05d0ad0`）。可以和 R211、R212 并行，合并进下一个版本。

场景：灵活组排（queue 440，game `9015224945`），英雄选择阶段（北京时间 21:47:27～21:50:58），对局页「详情」面板。

---

## 现象（录屏 30fps，共 11.9 秒，逐帧比对）

整段录屏一共 357 帧，只有两处画面有变化，其余帧完全静止：

| 视频时间 | 持续 | 画面 |
|---|---|---|
| 3.53 秒（第 106～107 帧） | 约 70ms | 「我听见风的颜色」「无聊玩玩tua」的「组队 ×2」标签消失；「好暗啊0」（隐藏战绩）的 10 个近期战绩英雄头像全部变成文字占位（疾、刀、复、圣、荣……）；随后恢复 |
| 10.6 秒（第 318～329 帧） | 约 370ms | 「组队 ×2」再次消失；「我听见风的颜色」「梦终ZO现」「无聊玩玩tua」的段位从 黄金 IV / 白银 IV / 白银 III 变成「未定级」；「好暗啊0」的战绩头像又变成文字，再逐个回填；之后段位和组队标签恢复 |

注：10.6 秒时第 5 位「冈崎真一」的头像换了英雄，这是真实的选人变化，不属于本缺陷。

---

## 日志证据

1. **两次闪烁正好对应后端两次完整重读阵容**（录屏从 21:49:01 开始，误差约 1 秒）：
   - 21:49:04.23 → 04.60：`live_load_cost matches_ms=354 total_ms=364`；`local_request_client endpoint=live duration_ms=421`
   - 21:49:10.49 → 10.84：`live_load_cost matches_ms=338 total_ms=348`
   其余每 3 秒一次的刷新都命中缓存（`matches_ms=0`、`total_ms≈10`），这些时候画面没有闪烁。
2. **整个选人阶段一共读取了 55 次，其中 13 次是完整重读**：21:47:31、48:13、48:16、48:20、49:00、49:04、49:10、49:46、49:50、49:57、50:34、50:38、50:43。每次完整重读都会闪一次。原因是选人阶段缓存只保留 3 秒（`backend/gameplay_refresh.go:156-160`）。
3. **数据没有变化，详情面板却一直在重绘**：`live_render_rebuild` 在 21:48:42～21:49:42 这一分钟里，`insight`（详情）面板被替换了 107 次，`rows_replaced=185`，`full=0`。
4. **结构变化时会整块重建，所有图片都重新创建**：21:49:46～21:50:46 这一分钟 `full=5`、`images_recreated=1473`。期间本人从 266 换成 36，对线卡出现了几次。每次重建大约重新创建 300 张图片，详情面板里的所有战绩头像会一起变成文字（见 P2）。
5. **现有日志分不出重绘是谁触发的**：进度快照触发的重绘和最终响应触发的重绘，都记在这一轮加载的来源（interval / sse）下面。

---

## 根因（源码已核对）

### 1. 后台刷新时推送的进度快照数据不全，前端却直接整份替换了已有的完整数据

后端 `loadGameplayLive`（`backend/gameplay.go`）：

- 7824～7827：开始时先推送一份骨架快照。每个玩家只有身份和选人字段，`HistoryState:"pending"`；没有 Rank、PrivateHistory、Autofill、RecentPositions，也没有组队信息和 ProPlayer。
- 7930～7934：每读完一个玩家的段位和战绩，就再推送一次。这时这个玩家有了 Rank 和战绩，但仍然没有组队和职业身份。
- 7985 `applyLivePremades` 和 8007 `matchProIdentity` 要等所有玩家读完才执行，结果只写进最终响应，从来不会出现在进度快照里。

前端 `applyLivePlayerProgress`（`backend/web/gameplay.js:4898-4924`）：

- 只要 `state.liveLoading` 为真、requestId 对得上，就执行 `state.live = next` 整份替换，然后 `renderLive()`。
- 同一局里只保留 `recentGames / modeStats / recentRankedRecord / recentPositions / historyState` 这五个字段，而且要求 `hidden / privateHistory / identityUnresolved` 三个标志前后一致才保留。

所以：

- **组队标签**：进度快照里都没有 `premadeGroup`。每次完整重读时，所有「组队 ×N」都会消失，直到最终响应到达。
- **段位**：骨架快照的 Rank 是空的，界面显示「未定级」（`gameplay.js:6042`），要等这个玩家读完才恢复。
- **隐藏战绩玩家的头像变文字**：骨架快照里 `privateHistory=false`，旧数据里是 `true`，不满足保留条件。于是这一行先变成 pending（没有图片），等玩家读完再重新生成 10 个 `<img>`，图片加载完之前显示英雄名首字。所以只有「好暗啊0」那一行的头像会闪成文字。其他玩家的行也被整行替换了，但相同地址的 img 被 `preserveLiveImages` 挪了过去，所以只能看到标签在跳。
- 同样会丢失的还有：补位（autofill）、职业选手标识、positionSource 等。

这个问题是 R204「逐人战绩」引入的。进度快照原本只是为了第一次进入时先显示已经读完的玩家，但现在已有完整数据的同一局后台刷新也会用它。

### 2. 结构变化时整块重建，没有复用图片

`renderLive`（`gameplay.js:5829-5840`）里，外壳（chrome）不一致时直接用 `nodes.liveContent.innerHTML = ...` 整块替换，只统计了 `imagesRecreated`，没有调用 `preserveLiveImages`。选人阶段本人换英雄、对线卡出现或消失、推荐页签出现，都会走这条路径。

---

## 修复要求

### P1　同一局的后台刷新不再使用数据不全的进度快照（必做）

前端 `applyLivePlayerProgress`：

1. 判定条件：`state.live` 已经有同一 gameId、同一 queueId 的可用阵容（`available` 为真、`players` 非空、`liveAwaitingGame` 为假）。满足时，进度快照**不得**整份替换 `state.live`。两种做法任选一种，推荐 a：
   - **a.** 直接忽略（`return false`），等最终响应一次性替换。最终响应只比进度快照晚 300～400ms，选人字段的更新几乎不受影响。
   - **b.** 逐人合并：进度快照里仍是 pending 的玩家，整条保留旧对象；已经读完的玩家只更新新读到的字段。`rank / premadeGroup / premadeSize / premadeSource / proPlayer / autofill / privateHistory / recentPositions` 在进度快照里没有时，一律沿用旧值。选人字段（championId、championPickIntent、championLocked、championPickPending、championName、position、spell1Id/2Id）仍按进度快照更新。
2. 第一次进入（`state.live` 为空、换局、awaiting 状态）保持 R204 现在的行为：先显示已经读完的玩家。
3. 旧 requestId、换局、阶段落后时拒绝进度快照，这些规则不变。

后端（可选的第二道保险）：如果 `a.liveSnapshots` 里已经有同一 gameId 的完整响应，就不再推送 7827 的骨架快照。逐人快照以旧响应为底，只替换已经读完的玩家，并保留旧响应里这个玩家的组队和职业身份字段。

### P2　整块重建时复用图片

`renderLive` 的完整重建路径：

1. 替换之前先调用 `preserveLiveImages(nodes.liveContent, fullHolder, stats)`，再用 `replaceChildren` 把 `fullHolder` 里的节点移进 `<div data-live-body>`。不要把 HTML 字符串再交给 `innerHTML` 解析一遍，否则挪过来的 img 又会丢失。
2. 修改后，地址相同的图片 `imagesRecreated` 应为 0。
3. 在 `live_render_rebuild` 里给 full 计数加上 `full_reasons`，标出外壳哪一部分不一样（`tab-row` / `banner` / `panel-count` / `lane-slot` / `other`），方便以后判断是否应该改成增量替换。

### P3　诊断

1. `live_render_rebuild` 的 sources 增加 `progress` 来源：由 `applyLivePlayerProgress` 触发的 `renderLive` 单独计数。
2. 新增 `live_progress_apply` 聚合事件（每 60 秒一条），字段：`applied`、`ignored_same_game`、`ignored_stale`。需要在 `runtime.js` 的白名单和 `features.go` 里一起登记。
3. 不在界面上加任何说明文字（CLAUDE.md 界面文案红线）。

---

## 测试

- **修改 `backend/web/r204.test.cjs`**：
  - 「R204 same-game pending incremental rows retain known history during background refresh」和「R204 pending progress does not reuse history across unknown games or changed identity scope」两条用例默认同一局后台刷新会使用进度快照，这个前提变了，按 P1 选定的方案改写。
  - 「首次进入先显示已读完的玩家」「旧 requestId / 换局 / 换阶段时拒绝」两条保持不变，必须继续通过。
- **新增 `backend/web/r213.test.cjs`**：
  1. 同一局已有完整阵容：两名玩家 `premadeGroup` 相同、`premadeSize=2`，三人有 rank，一名玩家 `privateHistory=true` 且有 10 局战绩。依次投递骨架快照和单人读完的快照后，`state.live` 里的组队、段位、隐藏战绩玩家的 `recentGames` 都没有变；`liveRecommendationMarkup(state.live)` 的输出和投递前完全一样。
  2. 第一次进入（`state.live` 为空）时，骨架快照仍然能渲染出来。
  3. 整块重建：构造一次外壳变化（多出一个页签），重建后地址相同的 img 是同一个对象（`===`），`imagesRecreated=0`。
  4. 如果做了后端可选项，补一条 Go 测试：已有同局缓存时不推送骨架快照。
- 全量运行 `node --test backend/web/*.test.cjs` 和 `go test ./backend`，结果写进账本。

---

## 真机验证（GPT 打包，用户在真机上操作）

验证性打包按 CLAUDE.md 在账本里写明 key mode；public 产物文件名带 `-public` 后缀。

请用户进入一局单双排或灵活组排，在选人阶段的「详情」页停留 2 分钟以上（期间换一次英雄），录屏并导出日志。验收标准：

1. 录屏里组队标签、段位、补位和隐藏战绩标签、战绩英雄头像在选人期间都不再闪烁（真实的选人变化除外）。
2. 日志：
   - 同一局完整重读期间，`live_render_rebuild.sources.progress` 为 0（方案 a），或者 `insight` 计数只在阵容真的变化时增加（方案 b）。
   - `live_progress_apply.ignored_same_game` 与 `live_load_cost` 里 `matches_ms>0` 的完整重读次数对得上。
   - 本人换英雄触发的 `full` 重建，`images_recreated` 接近 0。
