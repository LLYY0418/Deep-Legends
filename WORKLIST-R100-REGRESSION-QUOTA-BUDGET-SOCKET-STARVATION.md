# WORKLIST-R100 — 修回归优先：软件卡死/韩服查不了（我上一轮工单写错造成的）+ 启动14秒卡 + 三项遗留

日志：`lol-loot-diagnostics-0916-2155.jsonl`，build_fingerprint `8944d5e33f9c`，version 0.12.1。
含两次运行：**运行A `808a9e79`（13:37:14 → 约13:41，就是崩掉的那次）**、运行B `364cbd10`（13:41:48 起，用户重启后）。

**本工单取代之前误命名的 `WORKLIST-R99-KR-QUOTA-SELF-ABORT-CHERRY-GROUPS-EXPORT.md`**
（R99 这个编号已被"旗帜/头像/职业种子账号"那轮占用，我起名重复了，那份草稿已移走）。
P3/P4/P5/P6 是从那份草稿原样搬过来的遗留项，**P0/P1/P2 是本轮新查出来的回归**。

---

## ★★★ 先说清楚责任：P0 是我上一轮工单写错造成的

用户问"为什么问题越改越多"——这次的答案很明确：**P0 这个回归是我 R98 工单里一句话写歪导致的，
不是执行方理解错。** 我当时写的是：

> "给总览路径也套上 `withRiotQueueLimit(ctx, 约5秒)`，超预算时**立刻回**'额度恢复中，X秒后重试'"

我脑子里想的是"**单次等待**不要超过5秒"，但这句话读起来是"**整轮请求**总预算5秒"，
执行方按字面实现了，而且实现得完全符合我写的字面意思。结果就是下面这个回归。
**这一条不是执行质量问题，是我的工单质量问题。**

---

## P0（★★★ 最高优先，必须第一个修）5秒预算是绝对时间点，把正在成功的加载中途枪毙

### 根因：预算在整轮开始时就换算成了墙钟时间点，越到后面越必死

`riot_api.go:1092`（`loadRiotOverview` 开头，整轮只执行一次）：
```go
ctx = withRiotQueueLimit(ctx, 5*time.Second)
```
`riot_api.go:352-357`：
```go
func withRiotQueueLimit(ctx context.Context, limit time.Duration) context.Context {
	if limit <= 0 { return ctx }
	return context.WithValue(ctx, riotQueueDeadlineKey{}, time.Now().Add(limit))  // ← 绝对时间点
}
```
`riot_api.go:324`（每个请求要等额度时都会走到）：
```go
if deadline, ok := riotQueueDeadline(ctx); ok && sleep >= time.Until(deadline) {
    ... return errThrottled + 429「韩服查询额度正在恢复，请约 X 秒后重试」
}
```
而 `riot_api.go:318-320` 规定 `sleep` **最小就是 50ms**。

⇒ 到了 `T0+5秒` 之后，`time.Until(deadline) <= 0`，**任何一个还需要排队的请求都满足
`sleep >= time.Until(deadline)`，全部立即失败**。一轮冷加载本来就要 2.0~2.3 秒
（本次日志 `limiter_queue_ms` 实测 2051/1922/1771），再叠上 P1 的职业目录 9 秒和 P2 的 perks 14 秒
一起抢同一个令牌窗口，**5 秒墙钟轻松被打穿，剩下十几场详情在同一瞬间集体阵亡**。

### 两份日志的实证完全对得上

- 上一份 `0916-1741`：26 条 `riot_local_rate_limited` 挤在**两个瞬间**，每次正好 **13 条**，
  对应 `matches_failed: 13, rate_limited_count: 13` —— **13 个 worker 同时撞上被打穿的截止点**，
  这不是"额度真的用完了"，是我们自己设的闹钟到点了。
- 本次 `0916-2155` 运行A：`limiter_queue_ms` 2051/1922/1771，紧贴 5 秒预算的边缘。

### 修法（关键是把"总预算"改成"单次等待上界"）

把 `withRiotQueueLimit` 的语义从**绝对截止时间点**改成**单次等待的上限**：
- `riot_api.go:324` 的判断改成 `if sleep > maxSingleWait` —— 只在**这一次**需要睡的时间超过上限时
  才回 429，**不看整轮已经跑了多久**。
- 建议 `maxSingleWait` 取 5 秒（保持原意图：不让用户为一次等待干等超过5秒）。
- ★**`specialist_runes.go:399` 和 `pro_seed_accounts.go:30` 这两处现有调用的语义不能被改坏**：
  它们传的是 `specialistRiotQueueTimeout` 和 `100*time.Millisecond`，其中 pro_seed 那个 100ms
  明显是"基本不许等"的意思。如果统一改成"单次等待上界"，pro_seed 的 100ms 语义**恰好不变**
  （单次等待超过100ms就放弃），specialist 那个要**逐个确认原意**再决定。
  **如果两种语义确实都需要，就加两个不同的函数名，不要用一个函数承担两种含义。**

### 必须同时修：预算被打穿时不要丢掉已经付过费的数据

现在一旦打穿，那十几场**已经发出去、已经扣过配额**的详情结果被整轮丢弃。
和 P3 的"取消也要保住已拿到的详情"合并处理：**任何情况下已成功返回的详情都要进 `p.matchCache`**。

---

## P1（★★★）"无法连接本地助手 / 本地请求超时"崩溃的真凶：图标请求把浏览器的连接池占死了

### 这个弹窗不是后端挂了，是前端连不上后端

- 文案来源：`web/app.js:2530`（"无法连接本地助手"）、`web/app.js:390`（"本地请求超时，请重试"）。
- 触发条件：`web/app.js:462` `if (state.statusFailures >= 3) showFatal(error.message)`
  —— **只看 `/api/status` 这一个接口，连续失败 3 次**才弹。
- `/api/status` 的前端超时是 **8000ms**（`web/app.js:421`），而 "本地请求超时" 这句话是
  **前端自己的 setTimeout 打出来的**（`app.js:390`），**不代表后端返回了任何东西**。
- 后端 `handleStatus`（`main.go:681-720`）只取 `a.mu.RLock()`、零 I/O，自己不可能跑 8 秒；
  而且日志显示 13:40:52 后端还在正常写 `overview_load_cost`。**Go 服务没有挂。**

### 真凶：HTTP/1.1 的 6 连接上限被图标请求占满

1. `main.go:565` 用的是裸 `net.Listen`，**没有 h2c/HTTP2**（全仓 grep 无 `h2c`/`http2`）
   ⇒ 浏览器走 HTTP/1.1 ⇒ **同源最多 6 个并发连接**。
2. 而 `<img src="/api/champion-asset?...">`（`web/gameplay.js:1907`、`:5815`、`web/champions.js:202`）
   是**同源**请求，和 `api()` 的 fetch **抢同一个 6 连接池**。
3. 单个图标请求最坏能占住一个连接 **72 秒**：
   `main.go:975-983` 对 `_large.png` 返回 **6 个候选路径**，
   每个候选用的 HTTP 客户端超时是 `champions.go:494` 的 **12 秒**，6×12=72。
4. 本次日志坐实了这个 12 秒确实在发生：
   `champion_upstream {host:"raw.communitydragon.org", status:0, bytes:0, cache:"error", duration_ms:12000}`。

⇒ communitydragon 不通时，几个图标请求就能把 6 个连接全占住，
`/api/status` **根本排不上队发不出去**，3×8 秒后弹崩溃页。

### 为什么"重启软件 + 打开国服客户端"就好了

客户端连上以后，图标走 LCU 本地解析（日志里 `catalog_load {source:"lcu", duration_ms:22/32/164}`），
**毫秒级返回、不再长期占用连接**，连接池立刻松开。这条正好解释了用户的现象，是很强的印证。

### 修法（四条，建议全做，1和2是关键）

1. **图标请求不能长时间占用浏览器连接**：给 `/api/champion-asset` 加一个**整体**处理超时
   （建议 2~3 秒，不是每候选12秒），超时直接返回占位图/404，让连接立刻释放。
   ★**不要把 6 个候选串行试完**——改成"本地/LCU 优先 + 最多 1 次远端尝试"，
   或者远端候选并发竞速取第一个成功的。
2. **`/api/status` 不能和图标抢连接**。两种做法任选其一（倾向后者，改动小）：
   - 给服务端开 h2c（HTTP/2 多路复用，连接数限制消失）；
   - 或者**前端把图标改成不占 fetch 连接池的方式**：图标失败时不重试、加
     `loading="lazy"`、并限制同时在途的图标数量（例如自建一个并发≤2的图标队列）。
   ★还有一条兜底一定要做：**`/api/status` 失败不应该直接弹全页崩溃**——
   它只是个状态轮询，失败时应该降级成顶部小提示条 + 自动重试，
   **只有在真正确认后端进程消失时才弹全页**。现在"3次8秒超时"就判死刑太激进了。
3. `main.go:600` 已有 `ReadHeaderTimeout: 5s`、`IdleTimeout: 45s`，但**没有 `WriteTimeout`**。
   加上一个合理的 `WriteTimeout`，避免慢响应长期占住连接。
4. `champions.go:494` 那个 12 秒客户端超时对"取一张图标"来说太长了，建议降到 3 秒。

---

## P2（★★★）启动时 perks 目录卡 14 秒，而且卡在一把互斥锁上

### 根因：为了一份**拿不到就算了**的数据，同步干等了 12 秒

`gameplay.go:7891-7912`（`loadGameplayPerkCatalog` 的 `client==nil` 分支，即客户端没开时）：
先从 Data Dragon 取符文（`fallbackGameplayPerks`，`:8321`，约 2.1 秒），
然后 `:7911` 调 `a.fallbackGameplayAugments(ctx)` 去 **raw.communitydragon.org** 取斗魂海克斯
（`champions_structured.go:1789-1790`）——就是这一下烧掉 12 秒
（`gameplay.go:8087` `context.WithTimeout(ctx, 12*time.Second)`）。

★**最离谱的是它的错误被直接丢弃**：`:7911` 和 `:7974` 都写成 `augments, _ = ...`。
**也就是说我们干等 12 秒，等来一个错误，然后把错误扔掉继续走。** 实测 `catalog_load {perks, 14149ms}`。

### 为什么这条会让用户"一打开就用不了"

- `/api/gameplay/perks` 在前端模块初始化时就会调（`web/gameplay.js:6545`），
  而且**每次渲染战绩列表都会调**（`:1720`）。
- 前端给它的超时是 **15000ms**（`web/gameplay.js:3350`）。
  实测 14149ms ⇒ **只剩 851ms 余量**，网络稍慢一点符文图标就彻底不出来。
- ★**`cachedGameplayPerkCatalog` 在整个 14 秒里一直持有 `a.perkCatalogMu`**
  （`gameplay.go:7874-7880`）⇒ 所有并发的 perks 请求全部堵在这把锁上。

### 修法

1. **把海克斯从 perks 响应里摘出去**：ddragon 的符文一到就立刻返回，
   `Augments` 交给后台 goroutine 慢慢补进缓存条目。**不要让主响应等一个可选字段。**
2. 把 communitydragon 那个 12 秒超时降到 **3 秒**。
3. 把归一化后的 perks 目录**落盘缓存**，重启不用再付这个钱
   （现在 `gameplay.go:7836` `gameplayPerkCatalogTTL = 30分钟` 是纯内存的，
   而失败的 fetch 永远不会写盘 —— `champion_cache.go:472-474` 的策略对失败无效，
   所以网络不通的用户**每次启动都重新付 12 秒**）。
4. ★**不要把 cdragon 和 ddragon 改成竞速取第一个**——它们返回的是**不同的内容**
   （ddragon 给符文、cdragon 给海克斯），不是互相替代的镜像。这是个容易踩的坑。

---

## P3（★★）职业选手目录启动时占 9 秒，且跨进程不缓存

`pro_players.go:160-253`，三阶段实测：`directory` 3.0s + `supplements` 5.9s +
`ladder` 5.8s（含 `wait_after_supplements_ms: 3.0s`）= **8.9 秒**，
运行B 里又重新付了 **9.2 秒**。它恰好和用户的第一次韩服搜索**并发抢同一个令牌窗口**。

- `a.proPlayers` 是**纯内存**（TTL 5分钟 `:19`、maxStale 24小时 `:21`），**没有落盘快照**
  ⇒ 每次启动重付 9 秒。
- 目录正文和 supplements 走 `provider.fetch`，**是有磁盘缓存的**；
  但 **ladder 阶段完全绕过缓存**：`pro_players_ladder.go:160-188` 的 `fetchOPGGLadderRank`
  自己构造请求直接 `client.Do`，**零内存缓存、零磁盘缓存**，
  每个职业账号一个 HTML 页面、并发 8（`:21`），**每次启动 + 每 5 分钟都重新拉一遍**。这就是那 5.8 秒。
- `wait_after_supplements_ms = 3.0s` 是 `pro_players.go:246-247` 的 `ladder.finish()` 在排空队列，
  因为只有 supplements 才能发现的账号没法更早提交，**这段串行是真实依赖，不是写错**，不用硬改。

**修法**：(a) 把发布出去的快照 + ladder 排名**落盘缓存 24 小时**；
(b) 把 `fetchOPGGLadderRank` 改走 `provider.fetch` 以复用现有缓存层。

---

## P4（★★）后台历史赛段任务打断用户请求 + 左侧数据只在完整帧下发

这条是上一份草稿的 P0，**实证仍然成立，本轮继续要修**。

用户截图（The shy#asdf）症状精确吻合：战绩列表正常、职业徽章正常、英雄胜率正常，
但 **"召唤师等级 —"、两个排位都是 "Unranked / 尚未定级或客户端未提供"、"近 0 场排位"、
"能力表现"无样本** —— 左侧一整列全空。

**两个叠加成因**：
1. 预览帧只带 player + matches + pagination（`riot_api.go:1237` 构造、`:1247` 守卫），
   **段位/熟练度/召唤师等级只在 `complete` 帧里**。
   （R98 特意把 summoner 移出阻塞路径并明确不放进预览帧，所以等级显示成 "—" 是这一改动的直接结果。）
2. `complete` 帧经常压根不会到：后台历史赛段任务跑完调
   `clearOverviewQuerySnapshots()` + 广播（`opgg_insights.go:265`）→ 前端强制重载 → abort 掉在途请求。
   **上一份日志实测三次"加载不全"分别发生在后台任务完成后 4ms / 4ms / 24ms 内**，三次全中，不是巧合。

**修法**：
1. 后台任务完成时，**当前玩家正有总览请求在飞就不要触发强制重载**，延后到那次请求结束；
   或者改成"只把新数据合并进去，不要求前端重发整轮"。**后台补充数据不应该有权打断用户正在等的主请求。**
2. **段位/熟练度/召唤师等级要随预览帧一起下发**。
   ★**这里是本轮最容易把老 bug 改回来的地方**：前端 progress 合并白名单要同步放开这几个字段，
   但必须保持 R95-P1 的语义 —— **有值才覆盖、为 null 一律忽略**，
   绝不能退回"整体展开覆盖"，否则会把 R95 修好的"刷新清空生涯"重新引入。**必须有专门测试守住。**
3. 被取消/被预算打穿的加载，已拿到的详情要进内存缓存（和 P0 合并做）。

---

## P5（★★）战绩条显示 "CHERRY" 而不是中文模式名

`gameplay.go:8412` `queueLabel`：先查实时表 → 再查 `supportedQueueDefinition` → **查不到就 `return mode`**
（`:8419-8421`，就是这里漏出 "CHERRY"）。

**只有韩服中招**：`riot_api.go:942` 调 `convertRiotMatchInfo(..., nil, riotRegionKR, "")` —— 韩服**永远传 nil**，
拿不到实时队列表；国服/SGP 传的是 LCU 队列目录（`gameplay.go:3056`，`:1284` 的 "queue_labels" 阶段）。

**只有部分斗魂中招**：`queue_groups.go:110-112` 只登记了 **1700/1710/1750**，
而斗魂家族实际有 **1700, 1701, 1704, 1710, 1720, 1731, 1732, 1740(勇气斗魂), 1750** ——**六个没登记**。

★**结构性问题**：判断"是不是斗魂"用的是 `gameMode=="CHERRY"`+地图30（宽松，`queue_groups.go:250`
`isArenaQueue`、`gameplay.go:3795`），查中文名用的是队列ID白名单（严格，只认3个）。
两套口径不一致 ⇒ 这些对局**渲染出斗魂UI却顶着英文标签**，还会从"斗魂竞技场"筛选里消失、`arenaSquadSize()` 返回 0。

**同类隐患**（韩服路径、未登记ID→吐英文）：`1810~1840→STRAWBERRY`、
**`2600/2700→KIWI/ARAM_MAYHEM_CLASSIC`（前端有筛选项、Go表没有，最可能是下一个被看到的）**、
`1020→ONEFORALL`、`1400→ULTBOOK`、`1200→NEXUSBLITZ`、`910/920→KINGPORO`、`2000~2020→TUTORIAL`、`TFT`。
参考：`arena_match_detail.go:58-59` 已经在**详情页**打了同样的补丁，**列表页漏了**。

**修法（三层都要，缺一层以后还会犯）**：
1. **兜底层**：`queueLabel` 把"返回原始 mode"改成 `gameMode→中文` 映射表，再查不到返回 **"其他模式"**。
   ★**绝不允许把英文原始串吐到界面上**，这是硬约束。
2. **数据层**：`supportedQueueDefinitions` 补 1701/1704/1720/1731/1732/1740（顺带修复筛选器和 `arenaSquadSize`）。
3. **来源层**：`riot_api.go:942` 把 LCU 队列目录也传进韩服路径，让新队列自动汉化。

**还要补埋点**：韩服路径现在只发 `riot_match_items`（不含 queueId），
`recordMatchModeClassifications`（`gameplay.go:1300`）是国服专属。
请在韩服路径补一条含 `queue_id`/`game_mode`/`queue_label` 的诊断事件。

---

## P6（★★）三个玩家分组要常驻 + 各自空态

### 根因(A)：不是"职业分组被删了"，是**整个分组头部被 CSS 整体隐藏**

`web/gameplay.css:32`：`.gameplay-panel.is-disconnected .player-workspace-head { display: none; }`
而 `.player-workspace-head`（`web/index.html:159-169`）里**同时装着** `#player-groups` 和 `#player-tabs`。
开关在 `web/gameplay.js:1539-1540`：
```js
const panelUsable = group === "pro" ? Boolean(tab) : connected() || groupTabs.some((item) => riotTab(item));
```
★`groupTabs.some(riotTab)` **是死代码恒为 false**：`groupTabs` 是 `tabGroup()===group` 的页签，
而 `tabGroup` 只在 `!riotTab(tab)` 时才返回 `"players"`（`gameplay.js:1104`）。
⇒ 客户端没开 + 切到国服 ⇒ `panelUsable=false` ⇒ **分组胶囊、下拉、页签栏全部消失**，回不到职业分组。
下拉框本身是好的（`gameplay.js:1065` 遍历完整 `PLAYER_GROUPS`），所以截图里能看到"国服0/韩服0/职业6"
——**两条渲染路径的可见性规则不一致**才是问题。

配套要放开：`visiblePlayerGroups()`（`:302`）只留 `players`+非空分组；
`selectPlayerGroup`/`choosePlayerGroup`（`:305`/`:348`）会拒绝其它分组；`:1067` 把零页签分组渲染成 disabled。

### 根因(B)：空态位置

- **国服的启动卡** `#client-launchpad`（`index.html:134-139`）由 `app.js:767` 控制，
  数据来自 `gameplay.js:1531` 的 `current: group === "players" && ...`
  —— ★**已经是分组感知的，不用改**，用户要求"国服保持现状"正好已满足。
- **韩服 0 页签时**走的是 `gameplay.js:1532-1535`，打出
  **"尚未打开职业选手账号 / 请返回职业选手目录选择账号。"** —— 对韩服完全不对。
  `group !== "players" && !tab` 这个分支就是该做分组分流的地方。

**可复用**：`gameplay.js:5749` 的 `emptyState(title, copy, retry)`，class `.gameplay-empty`，
CSS（`gameplay.css:938/957`）**已有 58px 圆形图标位**，目前图标硬编码 `⬡`，
建议加可选第4参数 `icon`（12个调用点全是位置参数，向后兼容）。
先例：`app.js:1019` 用 `◆`、`:1758` 用 `◉`。

**目标**：三个分组永远可见可点。国服保持现状；韩服给"在顶部搜索框输入召唤师名称即可查询"
+ 图标，**不出现任何"启动客户端"字样**；职业给引导去目录 + 保留"返回职业选手 →"入口。

### ★必须一起处理的 7 个风险（逐条查出来的，不要漏）

1. `desktop/diagnostics-1042.test.cjs:9` 断言 `visiblePlayerGroups()` 深等于 `['players','pro']`，
   `:11` 断言 `const disabled = key !== "players" && itemCount === 0` 这段源码字符串。
   分组常驻后这两条会失败，**属预期基线更新，要在账本里逐条写清楚为什么改**。
2. `selectPlayerGroup`（`:306-307`）目前只是 `selectPlayerTab`（`:1325`）的副作用在设 `state.activeGroup`，
   **零页签时静默无操作**。要让空分组可选，必须显式设 `state.activeGroup = group`
   + `renderPlayerTabs()` + `renderOverview(group)`。
3. `renderPlayerTabs():1047-1051` 会在 `activeGroup` 不在 `visiblePlayerGroups()` 里时
   **强制改写成 `groups[0]`** —— 放开可见性后这个自动踢回要跟着调整，
   否则空的活动分组每次渲染都被踢回国服（这也是本 bug 的触发因素之一）。
4. 职业自动晋升（`gameplay.js:857-859`）只在 `state.activeTabs[oldGroup]===tab.key` 时才切；
   `openPlayer`/`openPlayerByRiotId`（`:1358-1359`、`:1377-1378`）也会强制切分组。
   **新规则不能和这些打架。**
5. `activeTab(group)` 对空分组返回 `null` 是合法的。已确认 `:1030/:1050/:1518/:1529/:1532/:6111`
   都有守卫、`renderPlayerTabWorkspace:1098` 已有骨架分支，**无空指针风险**，可放心放开可选。
6. `gameplay.js:1089` 断连时把非 riot 页签从国服页签栏隐藏；
   `resetTencentTabsAfterDisconnect:500-521` 断连时删除所有非 riot 非 current 页签
   （职业/韩服页签因为是 `riotTab` 能活下来）。**两条都要保留。**
7. `#pro-players-return`（`:1080-1081`）只在 `group==="pro"` 显示，而它就住在被隐藏的
   `.player-workspace-head` 里 —— **职业空态下必须仍然可达。**

---

## P7（★）导出日志记住上次选的文件夹

**机制早就写好了，只差一根线没接**：`desktop/diagnostics-export.cjs:6` 的 hook
**已经接受 `getDirectory` 参数**，`:53-64` **已实现自动保存 + 防重名**
（`lol-loot-diagnostics-MMDD-HHMM[-N].jsonl`）。但 `desktop/main.cjs:534-545` 只传了
`getDefaultDirectory`（`:538`），**从没传过 `getDirectory`** ⇒ `:55-57` 每次提前返回 ⇒ 每次弹窗。

**照分享图抄**（`desktop/share-export.cjs`，`createShareExportController` 在 `:186`）：
存储 `path.join(app.getPath("userData"), "share-export.json")`（`:14`+`:189`）单键 `saveDirectory`，
`persistDirectory`（`:210-217`）落盘；校验 `normalizeSaveDirectory`（`:37-41`）+
`usableDirectory`（`:202-208`）；**目录被删的兜底** `prepareSave`（`:250-255`）重置并重新弹窗；
防重名 `uniquePngPath`（`:43-52`）；信任边界每个入口 `isTrustedRenderer`（`:234/240/247/262`）
+ 单次令牌（`:256-266`）。IPC `main.cjs:554-561`、preload `preload.cjs:32-51`。
测试基线：`share-export.test.cjs:139/172/180/188/195`。

**★安全（naive 实现最容易漏的三条）**：现在日志导出**渲染层根本不提供路径**
（hook 只接受 URL 链等于 `/api/diagnostics/log` 且 `contents === sender` 的请求，`:11-15`）。
加"记住文件夹"要新增 `desktop-diagnostics-choose-directory` IPC，**这是全新的渲染层→主进程选路面**，必须：
1. 和 `main.cjs:548` 一样同时做 `isTrustedRenderer` **和** `event.sender === mainWindow.webContents`；
2. 自动保存意味着日志**静默落盘**到可能云同步的目录，`statSync` 会跟随符号链接，
   **要在写入时刻再校验一次**，不能只在保存路径时校验；
3. 桌面日志追加路径（`:30-39` 的 `O_NOFOLLOW` + inode/dev 复查）**原样保留不许动**。

**用户已拍板：各记各的** —— 日志用独立键（如 `diagnosticsSaveDirectory`，默认下载文件夹），
分享图继续用 `saveDirectory`（默认图片文件夹）。
设置页也要加一行对等入口（照 `desktop-share-get-directory`/`choose-directory` 那对抄）。

---

## P8（★ 最低优先，可以放到下一轮）生涯页 facade 每 2 秒轮询

`facade_load_cost` + `facade_skin_state` **各 30 次/分钟，连续 13 分钟**（各 309 条），
即每 2 秒一次，每次 fan-out 5 个 LCU 请求。

- 节流器：`connection_manager.go:22` `facadeEventThrottleInterval = 2 * time.Second`
  —— 它是**限速器不是防抖**，LCU 持续推送 facade URI 时就按上限 30/分钟一直发。
- 前端 `web/suite.js:2155` → `queueFacadeRefresh(800)`（`:1862`）→ `loadFacade`（`:1787`）。
  本来 `:1789` 有个 30 秒新鲜度守卫，但被 `:1803`
  `state.facadeLoadedAt = next.skinsUnavailable ? 0 : Date.now()` **打掉了**
  —— `SkinsUnavailable` 为 true 时（`profile_facade.go:177`）每个事件都强制全量重载。
  **这是守卫写歪了，不是有意设计。**
- ★**它是有门禁的**（`suite.js:1866/:1837/:2138` 要求 section=suite 且 tab=facade）
  ⇒ **用户查韩服时它不在跑，不解释本次崩溃**，所以优先级最低。
- 日志也要采样：`facade_load_cost`（`profile_facade.go:129`）+ `facade_skin_state`（`:192`）
  每轮 2 条 = 60条/分钟，本次日志 618 条。建议 `facade_skin_state` 按签名变化才记或 1/N 采样。

**修法**：修 `:1803` 那个被打掉的新鲜度守卫（`skinsUnavailable` 不应该把
`facadeLoadedAt` 清零）+ 给这两个诊断事件加采样。

---

## 关于配额的结论（用户问"必须换 product key 吗、多人怎么办"）

已核实 Riot 官方文档：

| 密钥 | 上限 |
|---|---|
| **个人/开发密钥**（现在用的） | **20次/秒 + 100次/2分钟**，官方原话 "by design very limited"，**按区域分别计算** |
| **生产密钥** | **500次/10秒 + 30000次/10分钟** |

一次全新 20 场搜索精确消耗 **25 个请求**（身份1+资料1+战绩ID1+段位1+熟练度1+详情20）
⇒ **100÷25 = 每2分钟只能搜4个新玩家**。上一份日志6.5分钟发了197个请求，
两次触顶时刻的滚动2分钟计数是 82 和 95，精确卡在 90 的自限线上。

- **单人**：把 P0/P1/P2/P3/P4 这些浪费修掉，体验会明显改善，**暂时不换也能用**。
- **多人：必须换，技术和规则两条线都堵死。**
  - 技术：EXE 内嵌的是**同一个个人密钥，所有用户共享同一个 100次/2分钟的桶**
    ⇒ 4次搜索/2分钟是**全体用户加起来**的总量，大约只能撑 **2 个活跃用户**。
  - 规则：Riot 明文写着 *"You may not run your application for public consumption using a personal key...
    public consumption includes open alpha/beta tests."* 拿个人密钥公开分发**本身违反条款**，会被吊销。
- **换成生产密钥后**按人均行为估算大约能支撑 **120 个活跃用户**。
  **所以你申请生产密钥的方向完全正确，拿到之前不要对外分发。**

本轮唯一允许的限流器数值改动：**按主机拆成两个独立窗口**
（Riot 本来就按区域分别计量，`asia` 和 `kr` 是两个桶，我们错误地合并成一个，拆开白得约 12% 额度）。
★**不要上调 `shortLimit=15/秒` 或 `longLimit=90/2分钟` 的数值**，那是 Riot 硬上限下的安全余量。

---

## 执行顺序（请严格按这个顺序，不要并行乱改）

1. **P0**（5秒预算语义）—— 这是本次"韩服用不了"的主因，也是我工单写错的那条，先修这个。
2. **P1**（连接池占死 + `/api/status` 失败不该弹全页崩溃）—— 这是"崩溃"的主因。
3. **P2**（perks 14秒 + 锁）—— 启动即卡的主因。
4. **P4**（后台打断 + 左侧数据随预览帧下发）
5. **P3**（职业目录 9 秒落盘缓存）
6. **P5**（CHERRY 汉化）、**P6**（分组空态）、**P7**（导出目录）
7. **P8** 可以留到下一轮。

## 验收要求

- **P0 必须有一条测试证明**：构造"一轮加载总耗时超过5秒、但每次单独等待都不超过5秒"的场景，
  断言**所有20场详情全部成功**、没有任何 `errThrottled`。
  ★这条测试在"把语义改回绝对截止时间点"的变异下必须真实失败。
  同时保留一条"单次等待确实超过上限时正常返回429"的正向测试。
  另外必须确认 `specialist_runes.go:399` 和 `pro_seed_accounts.go:30` 两处调用语义未被改坏（逐个断言）。
- **P1 必须有真实浏览器验证**（复用 `desktop/r98-browser.cjs` 那套真 Chromium+CDP）：
  构造"多个图标请求长时间挂起"的场景，断言 `/api/status` **仍能在8秒内返回**、不弹全页崩溃。
  ★jsdom 不算数（它没有真实的 6 连接上限）。
- **P2** 断言 perks 响应**不等待** augments（模拟 cdragon 挂起12秒，断言 perks 在约2秒内返回），
  且断言 `perkCatalogMu` 不被长时间持有（并发请求不互相阻塞）。
- **P4** 必须有专门回归测试守住 R95-P1：断言 progress 帧里 ranks/masteries/summonerLevel
  **为 null 时前端不覆盖已有值**，只有非 null 才更新。**这是本轮最容易把老 bug 改回来的地方。**
- **P5** 断言 `queueLabel` 在任何输入下都不返回英文原始串（未知 queueID + 未知 gameMode → "其他模式"），
  且该测试在"把兜底改回返回原始 mode"的变异下真实失败。
- **P6** 真实 DOM 测试（jsdom 够用，不涉及布局几何）+ 上面 7 条风险逐条在账本里说明处理方式。
- **P7** 断言首次选目录后持久化、第二次不弹窗、目录被删能回退重弹、重名不覆盖、不受信任 renderer 不能选路径。
- 常规回归：`go build .`、`go vet .`、`go test -count=1 -v .`、`go test -race ./...`、
  `node --test web/*.test.cjs desktop/*.test.cjs` 全部真实跑、附真实日志。
- 变异测试：P0、P1、P2、P4、P5、P7 每处都要有变异被真实杀掉。

## ★本轮明确不做的事

- 不动默认战绩条数（保持20条）—— 用户上一轮已拍板。
- 不把韩服战绩改走 OP.GG —— 改动太大，如要做单独开一轮。
- 不上调限流器的 15/秒、90/2分钟数值（只允许按主机拆窗口）。
- 不把 cdragon 和 ddragon 改成竞速取第一个 —— 它们返回不同内容，不是互相替代。
- **不要为了"让测试变绿"去删或放宽任何现有断言**；确证性失败必须在账本里逐条解释。

## ★给执行方的一句话提醒

上一轮的 P0 回归是**工单一句话有歧义**造成的（我把"单次等待上界"写成了"5秒预算"，
执行方完全按字面实现，实现本身没错）。
**如果本工单里还有任何一句话你觉得有两种读法、或者按字面实现会导致明显不合理的行为，
请先停下来在账本里写明你的疑问和你选择的读法，不要闷头按字面做完。**
