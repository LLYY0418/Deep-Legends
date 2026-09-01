# WORKLIST-R48（给 GPT 执行）—— 「重新读取」语义调转 + 卡死软复位 + 库存脏标记

诊断人：Claude（只读诊断，未改仓库文件）。执行人：GPT。
本单不基于日志，基于对刷新链路的全量代码走查，所有结论均带 file:line。

---

## 需求原文（用户，2026-08-31）

> 项目中重新读取这个逻辑得改一下，读取的重点应该是总览和对局的信息，比如总览统计数据和战绩是否变化了，
> 对局开始了但是对局页面没有反应，对局推荐数据出不来，对局进行到下个阶段了之类，更严重的是页面卡死了
> 也可以通过重新读取再次正常运行；其次才是更新皮肤、炫彩之类，因为这些不是经常变化，
> 监听到库存有变化了再去更新皮肤、炫彩之类。

> **口径决定（已与用户确认，不要自行更改）**
> 1. 手动「重新读取」时皮肤/炫彩/库存 = **后台异步补刷**，不阻塞、不弹遮罩。
> 2. 库存变化事件 = **只置脏标记，进页面再扫**（不立即全量重扫）。
> 3. 卡死自愈 = **软复位**（不做自动 `location.reload()` 硬刷）。
> 4. 刷新范围 = **总览 + 战绩 + 对局一律全刷**，不按当前页面做智能裁剪。

---

## 现状认定（改之前先理解这几条，否则会改错地方）

**顶部「重新读取」目前是一条只管客户端快照的链路，总览/战绩/对局一条都不碰。**

- 按钮 DOM：`web/index.html:96`，处理器 `web/app.js:2241-2251`。
- 处理器全部内容只有三件事：弹强制遮罩（`app.js:2244`）→ `POST /api/refresh`（`:2245`）→ 450ms 后 `refreshStatus(true)`。
- 后端 `POST /api/refresh` → `main.go:398` → `handleRefresh`（`main.go:704-707`）只投递信号 → `connection_manager.go:170` → `refreshWithClient`（`main.go:875`）→ `catalog.go:138-290` **八路并行重扫皮肤/炫彩/库存**。
- 前端拿到结果后只重取 `/api/status` 和当前视图的 `/api/skins`|`/api/chromas`（`app.js:262-291`）。
- 遮罩文案写死「正在更新皮肤、炫彩与账户物品…」（`app.js:2244`），**这句话准确描述了它当前的真实行为**。
- 总览/战绩走 `loadOverview`（`gameplay.js:477-478`），入口只有「刷新战绩」按钮（`gameplay.js:4629`）、切页且超 20 秒（`gameplay.js:192-194`、`:227`）、断开→连接翻转（`gameplay.js:278`）。
- 对局走 `loadLive`（`gameplay.js:2894`），入口只有「刷新对局」按钮（`gameplay.js:4630`）、自动轮询（`gameplay.js:4155`）、页面重新可见（`gameplay.js:4712`）、gameflow SSE（`gameplay.js:4751`）。
- **顶部「重新读取」只广播 `deep-legends:status`（`app.js:491`）；`updateStatus`（`gameplay.js:238-283`）在「已连接→仍连接」时什么都不做**（`:267` 的分支要求 `!wasConnected`）。这就是"按了重新读取，对局页毫无反应"的直接原因。

### ★ 关键发现：`tab.loading` 卡住时，强制刷新是空操作

`web/gameplay.js:457-459`：

```js
} else {
  if (tab.loading) return false;              // ← 这一行在 force 检查之前
  if (tab.data && !force) { rerenderTab(tab); return false; }
  tab.loading = true;
```

`if (tab.loading) return false;` **排在 `force` 判断前面**。只要上一次请求因为任何原因没走到 `finally` 把 `tab.loading` 复位（超时、异常路径、页签在请求中途被 `resetTencentTabsAfterDisconnect` 重建等），
之后**无论点多少次「刷新战绩」或「重新读取」都直接 return，界面永远停在旧数据上**。

这正是用户说的"页面卡死了"的一个真实成因。**B 组的软复位必须先把 `tab.loading` 清掉，否则 A 组的强制刷新会被这一行吃掉，整轮改动等于没做。**

对照：`loadLive` 的 `state.liveLoading && !force`（`gameplay.js:2888`）**尊重 force**，对局侧没有这个坑。只有总览侧有。

---

# A 组【P0】把「重新读取」的语义调转过来

## A-1 前端处理器重写（`web/app.js:2241-2251`）

新的执行顺序（严格按此顺序）：

1. 按钮进 loading 态。
2. **广播软复位 + 硬刷新事件**（见 A-2），并 `await` 总览与对局的结果。
3. 皮肤/炫彩/库存：`POST /api/refresh` **发出去就不管**（不 `await`、不进 `Promise.all`）。
4. 主链路结束 → 解除按钮 loading + toast。

**必须删掉的东西**：`app.js:2244` 那句 `showReadingOverlay(..., true)`。
理由：`showReadingOverlay` 会给 `el.appFrame` 加 `inert`（`app.js:359`）把整个界面锁死。
用户在对局页点刷新，结果整个界面被一层"正在更新皮肤、炫彩"的遮罩锁住——这是当前体验最反直觉的一点。
改成：**不弹强制遮罩**，只用按钮 loading 态 + toast 反馈。

> ⚠️ 不要顺手把 `showReadingOverlay` 删掉。首次连接、快照未就绪时的遮罩由
> `updateReadingOverlay`（`app.js:316-346`）独立管理，那条路径要原样保留。
> 本条只是不再由「手动点刷新」强制触发它。

**按钮状态**：当前成功路径不复位 `disabled`，靠 `renderStatus()` 的 `el.refresh.disabled = data.syncing`（`app.js:451-452`）解除。
改造后主链路不再等待后端快照，`data.syncing` 会在主链路结束后才变 true，会出现**按钮先亮起来、随后又被自己置灰**的闪烁。
要求：加一个 `state.manualRefreshing` 标志，`renderStatus` 里改成
`el.refresh.disabled = data.syncing || state.manualRefreshing;`，由 A-1 的主链路负责置位/复位。

**toast 文案**：改成「正在重新读取对局与总览」之类，**不要再说"皮肤、炫彩"**。

## A-2 新增 `deep-legends:hard-refresh` 事件，gameplay.js 接收

app.js 侧发事件时用 `waitFor` 数组收集下游 Promise（CustomEvent 的监听器是**同步**执行的，
所以 `dispatchEvent` 返回后 `waitFor` 里一定已经装好了 Promise，这个模式是可靠的）：

```js
const waitFor = [];
window.dispatchEvent(new CustomEvent("deep-legends:hard-refresh", { detail: { waitFor, reason: "manual" } }));
await Promise.allSettled(waitFor);
```

`web/gameplay.js` 在 `:4631` 那一片的事件注册里加：

```js
window.addEventListener("deep-legends:hard-refresh", (event) => {
  const detail = event.detail || {};
  const task = handleHardRefresh(detail);
  if (Array.isArray(detail.waitFor)) detail.waitFor.push(task);
});
```

`handleHardRefresh` 要做的（**顺序不能颠倒**）：

1. 先做 B 组软复位（`softResetGameplayState()`）。
2. 再并行发起：
   - `loadOverview(activeTab(), true)` —— force=true 会走到 `gameplay.js:477` 的 `force=1`，
     后端 `gameplay.go:537` 据此绕过 `overview_cache.go` 的 2 秒去重缓存。
   - `loadLive(true)`
   - `scheduleBeaconPoll(0)` —— 让 gameflow 提示灯立刻重探一次 `/api/gameplay/phase`。
3. 返回 `Promise.allSettled([...])`。

> **注意**：总览和战绩是**同一个请求**（`/api/gameplay/overview` 的 payload 里就带 `matches`）。
> 不要为"战绩"另外造一个接口调用。

**只刷当前活动页签**，不要遍历 `state.tabs` 把所有已打开的玩家页签全刷一遍——
那会瞬间打出 N 个 25 秒超时的重请求，是 R43 "串行 96 请求撞超时" 那类事故的翻版。

## A-3 断开状态下的行为

`handleHardRefresh` 开头要判断 `connected()`（`gameplay.js:172`）。未连接时：
只做软复位 + `renderOverview()` / `renderLive()`，**不要发 `/api/gameplay/*` 请求**（必然失败并弹错）。
`POST /api/refresh` 仍照发——那条路径在 `connection_manager.go:85` 的 `waitForDiscovery` 里会被消费，
起到"立刻重新找一次客户端"的作用，这对"客户端刚启动"的场景是有用的。

---

# B 组【P0】卡死软复位

新增 `softResetGameplayState()`（`web/gameplay.js`）与 `softResetShellState()`（`web/app.js`）。
两个模块是各自独立的 IIFE，`state.controllers` 是**两个不同的 Map**（`app.js:38` 与 `gameplay.js:141` 各一份），
必须各自复位，不能只做一边。

## B-1 gameplay.js 侧 `softResetGameplayState()`

必须覆盖以下每一项：

1. **abort 本模块所有在飞请求**：遍历 `state.controllers` 全部 `.abort()` 后 `.clear()`。
2. **★ 清 `tab.loading` / `tab.loadingMore`**（对所有 `state.tabs`）——见前面「关键发现」，
   这条是整个 B 组的核心，漏了它 A 组就是空转。
3. **清分页卡死态**：`tab.paginationStalls = 0`、`tab.paginationBackoffMs = 0`、`tab.nextAutoAppendAt = 0`，
   并把 `tab.data.pagination` 里的 `autoPaused` 置 false、`pauseReason`/`moreError` 清空
   （对应 `gameplay.js:490-493` 和 `:544` 设进去的暂停态）。
4. `state.liveLoading = false`。
5. **调用既有的 `resetLiveGameScopedState()`（`gameplay.js:2941-2951`）**——
   它已经把推荐/专家符文的在飞请求 abort、把 `liveRecommendationFailures` /
   `specialistRuneFailures` 等失败缓存清空了。**这正是"对局推荐数据出不来"的修复点**：
   推荐一旦进了 failures map，后续不会自动重试，只有清掉才会重算。不要另写一个。
6. `state.liveError = ""`、`tab.error = ""`。

> **一个取舍要注意**：`resetLiveGameScopedState()` 会把 `recommendationTab` 重置回 `"runes"`、
> `runeSourceTab` 重置回 `"opgg"`（`:2946-2950`）。用户如果正停在"出装"页签，点一下重新读取会被弹回"符文"。
> 建议：给它加一个 `{ preserveTabs }` 参数，手动刷新且 `state.recommendationTabTouched === true` 时
> 保留 `recommendationTab` / `runeSourceTab`。若实现成本高，保持现状也可接受，但要在工单回复里说明。

## B-2 app.js 侧 `softResetShellState()`

1. abort 本模块 `state.controllers` 全部请求并清空。
2. **解除遮罩死锁**：`clearTimeout(state.startupFallbackTimer); state.startupFallbackTimer = 0;`
   `state.overlayForced = false; state.overlaySuppressed = false; state.overlayBaselineAttempt = "";`
   然后**调 `updateReadingOverlay()` 让它按当前 status 重新判定**，
   而不是无脑 `hideReadingOverlay()`（快照真没就绪时该显示还得显示）。
   > 背景：`app.js:360-367` 的 25 秒看门狗是目前唯一的遮罩兜底。如果遮罩因为
   > `overlayForced` + `lastAttempt` 不变而卡在 `app.js:325-328` 的 450ms 自旋分支里，
   > 用户看到的就是"界面被锁住且一直转圈"。软复位必须能打破这个状态。
3. **重建 SSE**：直接调 `setupLiveUpdates()`（`app.js:2620`）。
   它开头就有 `state.eventSource?.close()`（`:2622`），可安全重入。
   > 背景：`source.onerror`（`app.js:2669-2679`）**只标记 `eventStream:false`，不重建 EventSource**，
   > 靠浏览器自身重连。浏览器放弃重连后，SSE 就永久哑了——所有 `snapshot-updated` /
   > `account-updated` / `gameflow:*` 推送全部失联，界面表现就是"什么都不更新"。
   > 手动重新读取必须能把这条流拉回来。
4. `clearTimeout(state.statusTimer)`、`clearTimeout(state.liveUpdateTimer)`，
   `state.statusDelay = STATUS_INTERVAL`，随后正常 `refreshStatus(false)` 会重新 `scheduleStatus()`。
5. `state.skinsCache?.clear()`。

## B-3 不要做的事

- **不要**加自动 `location.reload()`（用户明确选了软复位档）。
- **不要**动 `app.js:2616` 的 `visibilitychange` 逻辑和 `gameplay.js:4712` 的可见性重载。
- **不要**把 `state.destroyed` 或退出流程（`app.js:2253`）牵扯进来。

---

# C 组【P1】库存变化改为脏标记，进页面再扫

## C-1 后端：新增 `collection` scope（`lcu_events.go:92-113`）

当前 `lcuEventRefreshScope` 的分类：

```go
"/lol-champions/v1/inventories/", "/lol-champion-mastery/",
"/lol-inventory/", "/lol-summoner/v1/current-summoner"  → "full"   // 立即八路全量重扫
"/lol-loot/v1/player-loot-map", "/lol-rewards/v1/grants" → "account" // 轻量
```

问题：`/lol-champion-mastery/` **每局结束都会响**，`/lol-inventory/` 在补发库存时也频繁响，
每响一次就是 `catalog.go:138-290` 的八路并行 LCU 扫描。这是"越用越卡"的一个持续来源。

改为：

| URI 前缀 | 新 scope | 行为 |
|---|---|---|
| `/lol-champions/v1/inventories/`、`/lol-champion-mastery/`、`/lol-inventory/` | `"collection"` | **只置脏标记 + 广播，不扫** |
| `/lol-summoner/v1/current-summoner` | `"full"`（保持） | 身份变化极少且必须立刻反映 |
| `/lol-loot/v1/player-loot-map`、`/lol-rewards/v1/grants` | `"account"`（保持） | 只重读 profile/loot/rewards，开销小，宝箱钥匙数该实时 |

## C-2 后端：脏标记状态与广播

- `app` struct（`main.go:52-79`，建议加在 `snapshotFallbackAt` 之后）新增
  `collectionDirty bool` + `collectionDirtyAt time.Time`。
- `statusResponse`（`main.go:159-192`）新增 `CollectionDirty bool \`json:"collectionDirty,omitempty"\``，
  在 `handleStatus`（`main.go:543-553`）里填上。
- `connection_manager.go:226-236` 的 `case <-debounce:` 分支加一条：
  `scope == "collection"` 时**只**置脏标记 + `a.broadcastEvent("collection-dirty")`，不调 `refreshWithClient`。
- `refreshWithClient` 成功后（`main.go:998` 广播 `snapshot-updated` 附近）把 `collectionDirty` 清掉。
  **注意**：清标记要在 `a.mu` 锁内，且要跟 `snapshotReady` 的赋值在同一个临界区，避免"扫完了但标记没清/清早了"。

## C-3 前端：进页面再扫

- `app.js:5-14` 的 `LIVE_UPDATE_STATE_SLICES` 加 `"collection-dirty": ["status"]`。
  > ⚠️ 这张表是白名单，`app.js:2653` 会把不在表里的事件**静默丢弃**。
  > 漏加这一行 = 整个 C 组白做（对照 R39 埋点被后端白名单静默拒收那次事故）。
- 触发真实重扫的时机，**两处都要接**：
  1. 进入收藏页：`activateFavoritesPage`（`app.js:1863` 一带）中 `name === "collection"` 分支。
  2. 切换视图到 owned/all/remaining/chromas：`resetCollectionControls`（`app.js:1868`）之后。
- 条件：`state.status?.collectionDirty === true` 且 `state.status?.connected`。
  满足则 `POST /api/refresh`（后台，不 await 不弹遮罩），随后照常 `loadSkins(true)`。
- 加一个**本地在途标志**（如 `state.collectionRescanInFlight`）防止在收藏页里反复切视图时
  连打多次 `/api/refresh`。后端 channel 满会丢弃（`connection_manager.go:36-39`），
  但前端不该依赖这个来兜底。

## C-4 兜底不能拆

`connectedFallbackInterval = time.Hour`（`connection_manager.go:12`）的整点全量刷新**保持不动**。
它是"事件漏了"的最终兜底：即使 `collection` 事件全部丢失，一小时内也会自愈。
A 组的后台 `POST /api/refresh` 是第二层兜底。**这两层都不要动。**

---

# D 组 护栏与验证（不做完不算交付）

> 依据 `[[deep-legends-mutation-testing]]`：正则型 JS 断言极容易是假护栏，**一律先变异再判定**；
> 软链接沙箱无效，必须 `cp` 真副本。

## D-1 必须新增的行为测试

前端测试放 `desktop/` 下的 `.test.cjs`（`node --test` 跑，见 `desktop/package.json` 的 `"test": "node --test"`），
建议新建 `desktop/refresh-orchestration.test.cjs`：

1. **`tab.loading` 卡死后仍能强制刷新**：构造 `tab.loading = true`，走一次软复位 + `loadOverview(tab, true)`，
   断言**请求真的发出去了**。变异判据：把 B-1 第 2 项（清 `tab.loading`）删掉，测试必须 FAIL。
2. **手动刷新会打到总览和对局两个端点**：断言 `/api/gameplay/overview?...force=1` 与 `/api/gameplay/live` 都被调用。
   变异判据：把 `loadLive(true)` 从 `handleHardRefresh` 里删掉，测试必须 FAIL。
3. **手动刷新不弹强制遮罩**：断言点击后 `el.appFrame` 没有 `inert` 属性。
   变异判据：把 `showReadingOverlay(..., true)` 加回去，测试必须 FAIL。
4. **推荐失败缓存被清空**：往 `liveRecommendationFailures` 塞一条，软复位后断言为空。
5. **`collection-dirty` 在白名单里**：断言该事件能走到 `refreshStatus`。
   变异判据：从 `LIVE_UPDATE_STATE_SLICES` 删掉这一行，测试必须 FAIL。

Go 侧（`lcu_events_test.go` 或并入 `main_test.go`）：

6. **scope 分类表**：逐条断言 `/lol-champion-mastery/xxx` → `"collection"`、
   `/lol-summoner/v1/current-summoner` → `"full"`、`/lol-loot/v1/player-loot-map` → `"account"`。
   **必须是逐条真值断言，不要写成"只要非空就通过"**。
7. **`collection` scope 不触发全量重扫**：断言走 `case <-debounce:` 且 scope 为 `collection` 时
   `refreshWithClient` 的调用次数为 0。
8. **`refreshWithClient` 成功后 `collectionDirty` 被清**。

## D-2 变异测试要求

对 D-1 的 1/2/3/5/6/7 六条**逐条做变异**（把被测行为改坏，确认测试真的 FAIL）。
在工单回复里贴出每条的变异内容与 FAIL 证据。**存活的变异体要列出来并说明原因，不要跳过。**

## D-3 常规收尾

`go build ./...`、`go test ./...`、`gofmt -l .`、`cd desktop && npm test` 全绿。

> `GOCACHE` **千万不要**设在仓库里。仓库根目录曾因此躺着 7.5GB 的 `.gocache/`（已于本轮清理）。
> 用 `/tmp/gocache` 或 `$HOME` 下的路径。

---

# 附：本轮明确不做的事

1. **不做**"按当前页面智能裁剪刷新范围"——用户选了一律全刷。
2. **不做**自动 `location.reload()` 硬刷。
3. **不改** `connectedFallbackInterval` 的 1 小时兜底。
4. **不动**首次连接/快照未就绪时的遮罩逻辑（`app.js:316-346`）。
5. **不要**在 `handleHardRefresh` 里遍历刷新所有玩家页签，只刷活动页签。
6. **不要**为"战绩"单独造接口——它和总览是同一个 payload。
