# WORKLIST-R98 — 韩服战绩每次刷新都变 + 韩服查询慢

日志：`lol-loot-diagnostics-0916-1420.jsonl`，build_fingerprint `6b1d34ce2b52`，约6分钟、20次总览加载。
录屏：`钉钉录屏_2026-09-16 141355.mp4`，159帧/10.88秒，含3次手动刷新。

**先说结论**：用户报的三件事里，第三件（搜索栏搜到的韩服玩家要打职业选手标签）**已经被 R97 修掉了**，
根因就是 R97 那条 region 校验 bug（详见文末"P3"一节，本工单不再重复要求）。剩下两件是本工单的正事，
而且两件是**完全独立的两个根因**，不要混在一起改。

---

## P0（★★★ 用户最直观的"很奇怪"）战绩列表每次刷新都变一遍

### 实证：逐帧差分把整个过程拍下来了

对录屏做逐帧差分（159帧，阈值>3000变化像素），得到9次画面跳变，**规律性极强**：

| 周期 | 小跳变（点刷新按钮） | 大跳变①（列表被打乱） | 大跳变②（列表恢复） |
|---|---|---|---|
| 1 | 第50帧 t=3.42s | 第56帧 changed=35573 | 第61帧 changed=38878 |
| 2 | 第79帧 t=5.41s | 第86帧 changed=21173 | 第90帧 changed=24764 |
| 3 | 第104帧 t=7.12s | 第111帧 changed=20571 | 第117帧 changed=23886 |

三次刷新完全同构：**点一下 → 约0.4秒后整个列表被换成另一批对局 → 再过约0.35秒又换回来**。

抽出第55/58/63帧（周期1的"打乱前/打乱中/恢复后"）逐帧比对，内容是决定性的：

- **第55帧（正常）**：3/3/2 失败25分33秒（3天前）、4/5/9 胜利24分27秒（3天前）、1/4/1 失败15分14秒（3天前）、2/7/4 失败24分01秒（3天前）
- **第58帧（打乱）**：13/8/2 失败31分20秒 SVP（3天前）、**斗魂竞技场 18/2/19 第1名**（3天前）、4/8/6 失败30分57秒（**4天前**）、0/7/2 失败20分42秒（**4天前**）
- **第63帧（恢复）**：和第55帧**逐像素一致**，又回到3/3/2、4/5/9、1/4/1、2/7/4

★注意第58帧里 **4天前的对局排在了3天前的对局上面** —— 这不是"数据变了"，是**排序被打乱了**，
这个细节直接证明了根因不是上游返回了不同的战绩，而是我们自己把顺序搞乱了。

### 根因①（主因）：预览帧只带5场，前端却把它**插到列表最前面**

`riot_api.go:1248-1258`，20场详情用8个并发worker拉取，**只要任意5场先回来就立刻发一帧预览**，而且发完就锁死不再发第二帧：

```go
previewSent := false
for range ids {
	<-completedDetails
	loadMu.Lock(); loaded := loadedDetails; loadMu.Unlock()
	if !previewSent && loaded >= 5 {
		publishPartial()
		previewSent = true
	}
}
```

`publishPartial`（`riot_api.go:1196-1219`）把 `details` 数组**带空洞地**快照一份，跳过 `nil` 拼成 `partial.Matches`。
哪5场先回来是8个worker的竞态结果，**每次刷新都不一样**——可能是 `{#0,#3,#7,#12,#15}`，不是"最新5场"。

前端 `web/gameplay.js:768-771` 收到这帧后走的是**前插**：

```js
const matches = append
  ? [...(previous?.matches || []).filter(m => !incoming.has(String(m.gameId))), ...partial.matches]
  : [...partial.matches, ...(previous?.matches || []).filter(m => !incoming.has(String(m.gameId)))];
```

非append（普通刷新）走后半句：`[...partial.matches, ...旧列表去重]`。于是屏幕变成
`[#0,#3,#7,#12,#15] + [#1,#2,#4,#5,#6,#8,...]` —— **就是第58帧那个"4天前排在3天前上面"的乱序**。
列表渲染不做二次排序（`web/gameplay.js:1629-1630` 的 `filteredMatches` 保持数组顺序），数组顺序=屏幕顺序。
等全部20场到齐、完整帧覆盖下来，顺序才恢复正常（第63帧）。

**修法（二选一，倾向前者）**：
- **(推荐)** 把预览帧的合并从"前插"改成"**按 gameId 就地更新/补齐，不改变既有顺序**"：预览帧带来的
  match 如果旧列表里已有同 gameId，原地替换；没有的，**按后端给的 `pagination.begIndex` + 数组下标插到正确位置**，
  而不是无脑放到最前面。
- 或者：预览帧**不参与列表渲染顺序**，只用来驱动"已加载 N/20"的进度提示，列表等完整帧再渲染。
  （这个更简单但会让首屏空窗期变长，和 P1 的诉求冲突，所以更推荐上面那条。）

**★绝对不要**用"给列表加一次全量排序"来掩盖这个问题——后端给的顺序本来就是对的（`riot_api.go:1192`
`details := make([]*riotMatch, len(ids))` 按下标写入、`:1243` `details[index] = detail`、`:1272` 按下标读出，
全程零 map 迭代、零完成顺序 append，**后端顺序是可信的**），前端要做的是别把它打乱，不是打乱后再排回来。

### 根因②（次因，解释"有时候刷新完了还是变的"）：出错时不回滚

`riot_api.go:1260-1262`：加载出错或被取消时，**又调一次 `publishPartial()`**，然后才 return error。
前端 `web/gameplay.js:855-897` 的 catch 分支只设 `tab.initialPageError`，**从来不把 `tab.data.matches` 回滚**。
于是那个乱序的预览快照就永久留在屏幕上了——这就是用户说的"图一图二是有时候刷新完了变"。

**修法**：catch 分支里，如果本次是非append刷新且曾经收到过预览帧，必须把 `tab.data.matches`
**还原成本次刷新开始前的快照**（进入 try 之前先存一份），不能把半成品留在屏幕上。

### 旁证：日志里的 `matches_loaded: 0`

`riot_overview_cost` 有三条是 `{matches_requested:20, matches_from_network:8, matches_loaded:0}`、
`{from_network:13, loaded:0}`、`{from_memory:3, from_network:12, loaded:0}`。
`matchesLoaded = loadedDetails` 在 `riot_api.go:1270`，位置在 `:1264-1269` 的 `if ctx.Err() != nil { return }` **之后**，
所以 loaded=0 意味着**这次加载在拿到8/13/12场之后被取消了**——对应用户连续点刷新，前端
`web/gameplay.js:189` `state.controllers.get(key)?.abort()` 中止了上一次请求。这正是根因②的触发场景。

### 不要做的事

- 前端已有 `overviewRequestToken` + controller 身份双重防护（`web/gameplay.js:762/838`），**晚到的旧响应
  不会覆盖新结果**，这一层是对的，不要动，也不要以为问题出在这儿。
- 不要改后端的 `details[index]` 写入方式，那是对的。

---

## P1（★★★ 用户说"问题很严重"）韩服查询慢 / 首屏只出5条

四个独立成因，按影响从大到小排，**建议全做，但如果只能做一半，做 #1 和 #2**。

### #1 本地限流器预算只够 1.8 次搜索/2分钟，而且卡住时没有任何提示

`riot_api.go:274-344` 的 `wait()`：
```go
shortLimit = 15; shortPeriod = time.Second
longLimit  = 90; longPeriod  = 2 * time.Minute
```
一个 `riotProvider` 全局共用一个窗口，总览页/绝活哥符文页/职业选手页全在抢。
**一次全新的20场总览要烧掉约25个令牌**（account + summoner + matchIDs + 20场details + ranks + mastery），
90/2分钟 ⇒ **每2分钟只够1.8次全新搜索**。日志里6分钟做了20次加载，超预算约5倍。

这解释了日志里最刺眼的那条：
```
duration_ms: 36748, limiter_queue_ms: 283718, rate_limited_count: 0
mastery 跨度 [[374,36587]]、details 跨度 [[374,36742]] —— 两条并行卡了36秒
```
**`rate_limited_count: 0` 是关键**：Riot 一次都没返回 429，这36秒**全是我们自己的限流器把自己锁死的**。
`limiter_queue_ms: 283718` 是8个goroutine排队时长的**求和**（不是墙钟），8×36秒≈283秒，对得上。

更糟的是 `riot_api.go:304-309` 算休眠时间是"等最老的那条记录过期"，于是**所有被挡住的goroutine
一起睡同一个36秒**（惊群），而 `riot_api.go:318` 那个"有deadline就提前放弃"的逃生口
**在总览路径上永远不会触发**：`withRiotQueueLimit` 只在 `specialist_runes.go:399` 用了，
server 没设 WriteTimeout（`main.go:597` 只设了 IdleTimeout），前端给了190秒超时（`web/gameplay.js:762`）。
**结果就是用户干等36秒，连一句"额度恢复中"都看不到。**

**修法（三条一起做）**：
- (a) 给总览路径也套上 `withRiotQueueLimit(ctx, 约5秒)`，超预算时**立刻回"额度恢复中，X秒后重试"**，
  而不是闷头等36秒。前端已有 `error.errorKind === "rate-limited"` 的自动重试逻辑
  （`web/gameplay.js:857-870`），接上就能用。
- (b) 把惊群式的"睡到最老记录过期"改成**按等待者分配槽位的FIFO预约**（第i个等待者算
  `longWindow[i].Add(longPeriod)`），或者改成短睡+重检，不要一次睡36秒。
- (c) `longLimit` 可以往真实 Personal Key 上限（100/2分钟）靠，**但必须先做完 #2 把需求降下来再调**，
  否则只是把撞墙时间往后推。

### #2 matchIDs / ranks / mastery 三个接口**完全没有缓存**，每次刷新都重新拉

`riot_api.go:717-745` 这三个直接调 `p.get`，**没有走** `accountByRiotID`/`summonerByPUUID` 用的
`cachedPublicIdentity`（`riot_identity_cache.go:22`）。`overview_cache.go:13` 的
`overviewQuerySnapshotTTL = 2 * time.Second` 太短，起不到作用。

日志实证：一次**20/20全内存命中**的重复刷新仍然要650ms，拆开看是
`matchIDs: 323ms + ranks: ~305ms + mastery: ~320ms`，而 `details` 只有 **2ms**。
**也就是说：20场对局明明全在内存里，我们还是每次都去网络上重新问了3次。**
这3个请求同时也在持续消耗 #1 那个宝贵的令牌预算。

**修法**：三个都包进 `cachedPublicIdentity`，按 `puuid|begIndex|count` 做键，TTL 建议
matchIDs 约60秒、ranks 约2-5分钟、mastery 约30分钟。**这一条做完，重复刷新会从650ms降到接近0，
每次加载的令牌消耗从25降到22，冷加载也顺带受益。**

### #3 `summoner` 白白挡住了战绩详情约450-750ms

`riot_api.go:1121-1139`：`summoner`、`champion_names`、`matchIDs` 三个并行跑，但
`identityReads.Wait()` **要等 summoner 和 matchIDs 都完成**才往下走。
日志实测 matchIDs 在314ms就回来了，summoner 要757ms ⇒ **详情要等到1557ms才开始拉，本可以1114ms就开始**。

而 summoner 只提供 `ProfileIconID` / `SummonerLevel` 两个**纯装饰性的头像/等级字段**。
（注：`account` → `summoner` 确实必须串行，summoner-v4 要用 account 返回的 PUUID，这个没法优化，
不要去合并这两个接口，Riot 没有合并端点。）

**修法**：把 `summoner` 挪进 `profileWait`（和 ranks/mastery 一组），只用 `idsErr` 来gate详情拉取；
头像和等级晚一帧再补进去即可。**预计每次冷加载省400-700ms。**

### #4 "只出5条然后要等很久"——预览帧一辈子只发一次

就是 P0 里那段代码：`previewSent` 是个**一次性闩锁**，硬编码在 `loaded >= 5` 触发，
发完第6到第20场**再也不会流式推送**，UI 只能从5条直接跳到20条（或者出错时跳）。

**修法**：去掉 `previewSent` 闩锁，改成**每完成一场就推**（或每2场/每约300ms合并推一次，避免推太密）。
`publishPartial` 本身是幂等的、每次都会重新转换所有已就绪的详情，**改动很小**。
★注意这一条必须和 P0 的前插修复**一起做**：如果只去掉闩锁不修前插，列表会从"刷新时抖一下"
变成"刷新时连续抖十几下"，那就更糟了。

### #5（可选，优先级最低）`pro_identity_match` 日志刷屏

本次日志6112行里有 **5142行是 `pro_identity_match`（84%）**，其中5102条是 `surface:"match"`。
`pro_identity.go:221` 对每个参与者无条件记一条，经 `gameplay.go:2444` 的
`publicizeMatchReferencesWithProIndex` 调用 ⇒ 20场×10人=200条/次加载，预览帧+完整帧×2≈250条。
它**不在** `isNoisyDiagnosticEvent` 名单里（`main.go:1527-1534`），绕过了1/100采样；
也没用 `claimBoundedDiagnosticKey`。

**性能上不是瓶颈**（写入有 `bufio.NewWriterSize(file, 64<<10)` 缓冲 + 200ms定时flush，
`storage.go:758`，总成本个位数毫秒，相比36秒不值一提）——**但它把日志按84:16的比例淹掉了，
会严重妨碍以后排查真问题**。

**修法**：把 `"pro_identity_match"` 加进 `isNoisyDiagnosticEvent`，或者改成**只在 `matchedBy != "none"`
时才记**（命中才记，没命中不记）。顺带：`a.proIdentitySnapshot()`（`pro_identity.go:133`）在
`publicizeMatchReferences`（`gameplay.go:2431`）里每次都重建整个并查集索引，可以按
`proPlayers.fetchedAt` 做记忆化。

---

## P3 职业选手标签——**已由 R97 修复，本工单不需要再做，但要验证**

用户第三个诉求是"搜索栏搜到的韩服玩家如果是职业选手，要打标签+进职业分组，比如图四 허거덩#0303 是 GEN Chovy"。
诊断结论：**这不是缺功能，是 R97 那个 region 校验 bug 的又一个表现面**，链路本身早就是完整的。

三条硬证据：
1. 日志里 **5142 条 `pro_identity_match` 全部是 `"candidates": 14`** ——候选账号池塌到只剩14个。
   `docs/pro-players-sources.md:63` 明确记录"补充来源 **14** 个（TheShy 9 / Assum 1 / JiaQi 4）"，
   数字精确吻合：OP.GG 来的账号被 region 校验**全部**拒掉，只剩 `pro_players_supplement.go:322`
   里硬编码 `Region:"kr"` 的 TrackingThePros 补充账号。旁证：`matched_by:"puuid"` 是 **0** 次
   （补充账号不带PUUID），`matched_by:"riotid"` 只有152次。
2. 实测抓了一次真实上游（`op.gg/zh-cn/lol/spectate/list/pro-gamer?region=kr`，HTTP 200，3.7MB）：
   账号级 `"region":null` 出现 **910 次**，`"region":"kr"` 的 120 次全都在**队伍外层**——
   和 `pro_players.go:302-304` 注释说的完全一致。
3. **허거덩#0303 确实在上游目录里**，`teamId:416`（`pro_roster.go:60` 的 GEN）、
   `nickname:"Chovy"`、`real_name:"Jeong Ji-hoon (정지훈)"`（和 `pro_roster.go:63` 的 Names 对得上）、
   带 PUUID、`level:760`（**和用户截图里的"召唤师等级 760"完全一致**）。
   所以修好 region 后它会**靠 PUUID 精确命中**，不是靠名字猜。

前端链路也早就通了：`web/app.js:2798` 搜索派发 → `web/gameplay.js:6215` 开页签（此时还不知道是职业选手，
进 players 组是正常的）→ 总览返回后 `web/gameplay.js:841-846` 检测到 `region==="kr" && payload.player.proPlayer`
就调 `applyPlayerTabContext(tab, {group:"pro", ...})` 并把 `state.activeGroup` 切到 `"pro"`，
徽章由 `summonerProChip`（`web/gameplay.js:1160-1163`，在 `:1651/:1654` 调用）渲染。后端在
`gameplay.go:2418-2419` 以 `surface:"overview"` 供给。**一个环节都不缺。**

**本工单要做的只有一件事**：R97 改完后，**必须真实验证 허거덩#0303 能被识别成 GEN Chovy**——
用真实 Flight 响应形状（`region` 为 null）的 fixture 跑一遍完整链路，断言
`candidates` 回到几百量级、`matched_by` 出现 `"puuid"`、且 Chovy 这条能命中。
R97 已经在 `pro_players_test.go:430` 加了"region 可为 null/缺失/空"的测试，但**要确认它覆盖的是
完整链路而不只是 `normalizeProAccount` 单函数**，并且确认 `pro_identity.go:186` 那条改成委托
`normalizeProAccount` 的路径也被同一条测试覆盖到（那里原本是第二份重复的校验，才是真正产出
`candidates:14` 的地方）。

---

## 验收要求

- **P0**：必须用**真实浏览器**（复用 `desktop/r95-browser.cjs` 那套真 Chromium + CDP 的做法）跑一次
  "先有20条 → 收到一个只含5条乱序子集的预览帧 → 再收到完整帧"的序列，断言**列表顺序在整个过程中
  始终与完整帧一致、不出现任何中间乱序**。★jsdom 断言不算数（jsdom 不做布局也不做真实渲染顺序验证，
  历史上已经在这类问题上假绿过一次）。另外要单独断言"出错时列表回滚到刷新前的快照"。
- **P1**：给出修复前后的真实耗时对比（至少覆盖"全新搜索"和"重复刷新"两种场景），
  `limiter_queue_ms` 和 `overview_phases_ms` 的实测数字都要附上。
  限流器改动必须有并发测试证明不会惊群、且超预算时能在约5秒内返回 rate-limited 而不是干等。
- 常规回归：`go build .`、`go vet .`、`go test -count=1 -v .`、`go test -race ./...`、
  `node --test web/*.test.cjs desktop/*.test.cjs` 全部真实跑、附真实日志文件。
- **变异测试**：P0 的前插修复、P1 的三处缓存、限流器逃生口，每一处都要有对应变异能被测试真实杀掉。
- 不要求真实客户端验收（P0/P1 都能用 fixture + 真浏览器复现）。

## ★特别提醒：fixture 必须覆盖真实上游的字段形态

R97 那个 bug 之所以能溜过两轮"零缺陷"验收，是因为 `pro_players_test.go` 的 `proFixtureAccount`
**所有 fixture 都硬编码了 `Region:"kr"`**，而生产环境真实数据里这个字段 910 次全是 null。
本轮新增的任何 fixture，**必须照着真实上游响应的字段形态写**（该为 null 的就写 null、该缺字段就别加），
不要写"理想化的完整数据"——否则变异测试再多也测不出这一类 bug。
