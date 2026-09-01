# WORKLIST-R46（给 GPT 执行）

> **本轮有三条"等了很久终于拿到答案"的发现，先看这三条：**
>
> 1. **斗魂身份之谜解开了**（A 组）——但**gameflow 的 `teamParticipantId` 这条路被日志证伪了**，
>    别走。真正可能的字段是 champ-select 的 `team`，但还差最后一次探测确认取值。
> 2. **"越用越慢"找到三处无界累积**（B 组），其中 `overviewQueryCache` 是主犯：TTL 只有 2 秒
>    但**从不删除过期条目**。
> 3. **侧边栏那个"对局提示灯"我们之前修错对象了**（F 组）——用户说的不是连接状态条，
>    是"对局"导航项上的小圆点，它被一条通配 CSS 选择器误伤了。

---

## A 组（P0）：斗魂队伍分组 —— 真机数据已到，但还差一次探测

### A-1 已经确定的事实（不用再验证）

`lcu_champ_select_session_shape`（CHERRY / queue 1750 / ChampSelect）：
```json
"my_team_length": 18,
"my_team_nonzero_counts": {"cellId":17, "gameName":3, "puuid":3, "summonerId":3,
                           "tagLine":3, "team":18, "nameVisibilityType":18, "wardSkinId":18},
"their_team_length": 0
```

- **18 个人全在 `myTeam`，`theirTeam` 是空数组。**
- **只有 3 个人有 puuid/gameName/summonerId** —— 就是自己的小队（斗魂 3 人一队）。
  其余 15 人客户端在英雄选择阶段**故意不公开身份，这是 Riot 的设计不是我们的 bug**，
  所以图三里那些"隐藏玩家"有 15 个是正常的，不要试图去"修好"它们。
- **R44-ADDENDUM 拆 5 人上限已生效**：`live_roster_shape` 现在是
  `players=18 raw=0 appended=18 emptyref=15`（之前是 5）。
- **进入游戏后身份就全了**：`phase=Reconnect players=18 raw=18 appended=0 emptyref=0`，
  gameflow 给了全部 18 人且都有身份 —— 这就是图四能看到 Uzi#49096 等真名的原因。
- **但 `team_counts` 恒为 `{"100": 18}`**：18 人全被标成 team 100，前端只能全塞进"我方"栏。
  **这就是图四的根因。**

### A-2 ★重要订正：`teamParticipantId` 这条路已被日志证伪，不要走

我原本以为 gameflow 的 `teamParticipantId` 是小队标识。**日志证明不是**：
斗魂 Reconnect 的 `team_one_team_participant_id_counts` 实际分布是
```
{"1":5, "2":1, "3":1, "4":1, "5":1, "6":1, "7":2, "8":1, "9":1, "10":1, "11":2, "12":1}
```
合计 18 人没错，但这是 **12 个不同取值、分布是 5/1/1/1/1/1/2/1/1/1/2/1**，
完全不是"6 组各 3 人"。**所以 gameflow 侧没有可用的小队字段，别在这上面浪费时间。**

### A-2b ★★★第二次订正（LeagueAkari 源码实证）：`team` 字段也不能用来分小队

我原本写的 A-3 让你去探测 `team` 字段是不是 1~6。**现在有更硬的证据表明这条路大概率也是错的**，
所以 A-3 的目标要改（探测还是要做，但不是为了"确认它是小队号"，而是为了"确认它不是"）。

我拉了 LeagueAkari（HEAD `0b7daf4b`）的源码逐文件读，实证如下：

1. **它的类型定义 `src/shared/types/league-client/champ-select.ts:100-124` 的 `ChampSelectTeam`
   与我们真机抓到的 23 个键逐字一致（含 `team: number`），没有任何 subteam 字段**
   —— 说明上游 session 本身就不下发小队信息，不是谁漏解析了。
2. **它解析 `team` 只是为了二分蓝红**，`src/main/shards/ongoing-game/champ-select-members.ts:27` 原文：
   ```ts
   const teamIdentifier = member.team === 100 || member.team === 1 ? 'TEAM-100' : 'TEAM-200'
   ```
   非 100/1 的一律归 TEAM-200。**全仓库没有任何地方在英雄选择阶段用 `team` 做小队分组。**
3. **它在斗魂下是显式拍平成一列的**，`src/main/shards/ongoing-game/computed-state.ts:277-281`：
   ```ts
   if (queryStage.gameInfo.queueType === 'CHERRY') {
     return { 'TEAM-ALL': members.map((member) => member.puuid) }
   }
   ```
   i18n 里 `TEAM-ALL: 所有`，队伍词条只有 TEAM-ALL/TEAM-100/TEAM-200/LOBBY 四个，**没有任何小队词条**。
   同文件 `:306` 显示**进了游戏也仍是 TEAM-ALL**。
4. **`/lol-champ-select/v1/subteam-data` 这个端点在 LeagueAkari 里不存在**（全仓零命中）。
   `/lol-lobby/v2/lobby` 的 `LocalMember` 倒是有 `subteamIndex`（注释写"竞技场中属于哪个小队，1到4"），
   但**全仓 grep 只有这一处类型声明、零使用**，而且那是房间/组排阶段的自建小队，跟对局的 6 队不是一回事。

**所以结论是：英雄选择阶段拿不到斗魂的小队分组，业界标杆项目也拿不到。**
我们不需要再为这件事投入了。**A 组的目标要下调**（见 A-3 改写）。

### A-3（改写）探测目标改成"证否 + 摸清空身份成因"，不要再抱做成 6 队的预期

探测本身还是做（很便宜，两行），但目的变了：

1. `"my_team_team_counts": diagnosticInt64FieldCounts(myTeam, "team")`
   —— 预期会看到**全是 100 或全是 200 之类的二值**，而不是 1~6。
   拿到结果就把这条彻底结案，写进注释别让以后的人再试一遍。
2. **新增**：把那 15 个空身份成员的 `nameVisibilityType` 和 `obfuscatedPuuid` 的**实际取值分布**打进日志。
   LeagueAkari 的处理方式是：`nameVisibilityType === 'HIDDEN' && obfuscatedPuuid` 非空时走一个
   **闭源 native 模块反混淆**（`resources/magic/magic.win32-x64.node`，源码不在仓库里，我们复现不了），
   否则**直接丢弃、不占位**（`champ-select-members.ts:45-54`）。
   所以要先确认我们这 15 个人到底是"HIDDEN + 有 obfuscatedPuuid"还是"三者全空"：
   - 如果是后者 → 客户端根本不下发，任何前端手段都拿不到，**彻底结案**
   - 如果是前者 → 至少知道理论上存在反混淆路径（但我们不做，那需要闭源模块）
3. 顺带确认 `benchChampions` 的元素形状（见 A-5）。

### A-4（改写）本轮实际要做的：进游戏后分组 + 英雄选择阶段体面降级

既然英雄选择阶段做不到，本轮就做两件务实的事：

**(a) 英雄选择阶段：体面降级，不要再显示 15 个"隐藏玩家"占位行。**
参考 LeagueAkari 的做法——身份为空的成员**直接不进列表**。
我们现在把 18 个人全渲染出来、其中 15 个是空壳，观感很差（用户图三）。
改成：只渲染有身份的那 3 个（自己小队），下面加一行说明
**"斗魂英雄选择阶段客户端只公开己方小队，其余玩家进入对局后显示"**，
让用户知道这是模式限制不是软件坏了。

**(b) 进入游戏后（InProgress/Reconnect）：这时候 18 人身份全有了**
（真机实证 `raw=18 emptyref=0`），**至少要把"我方/对方"这个错误的二分去掉**。
斗魂没有敌我概念，现在把 18 人全塞进"我方"栏、"对方"栏空着，是明确的错误展示。
最低要求：斗魂模式下改成单栏平铺（和 LeagueAkari 一致），标题不要叫"我方"。
如果后续能从 SGP 或别处拿到可靠的小队号再升级成 6 栏。

> ⚠️ 注意：`teamParticipantId` 在 SGP 的 `/gsm/v1/ledge/...` 里也有，
> LeagueAkari 也是只在 in-game 阶段才查它（`additional-info-controller.ts:152-158`
> 明写非 in-game 就直接返回）。但我们真机那份 `teamParticipantId` 分布是 12 个值不是 6 组，
> **所以进游戏后能不能分出 6 队仍然存疑**，先做 (b) 的单栏平铺，别把 6 栏当成必达目标。

### A-5 拿到确认后的改动范围（结构体部分仍然有效）

- `lcuChampSelectPlayer`（`gameplay.go:4003-4017`）加 `Team int64`（现在没有这个字段）
- `lcuLivePlayer`（`:3968-3986`）加 `SubteamID`（现在完全没有承载小队的字段）
- `mergeChampSelectPlayers` 新建那行（`:4422`）和已匹配分支（`:4425-4468`）各补一次赋值
- `gameplayLivePlayer`（`:3837-3853`）加 `SubteamID int64 \`json:"subteamId,omitempty"\``，
  组装点 `:4214`
- **建议保留 `TeamID` 不动**（100/200 对普通模式是正确语义），只新增小队维度，别连累 5v5
- 前端 `renderLiveInsights`（`web/gameplay.js:3450-3460`）从硬编码两栏改成按小队分组；
  `orderLivePlayers`（`:3234-3246`）第一排序键要改；
  `recordLiveRosterRendered`（`:3137-3149`）签名是 `rendered100/rendered200`，小队模式下会失真要一起改；
  CSS `.live-teams` 是固定两列 grid（`web/gameplay.css:967`），6 队要加 `is-arena` 分支
- **可复用的参照**：战绩详情页 `matchPlayerGroups` 主分支（`web/gameplay.js:1819-1836`）
  已经做对了"按 subteamId 分组 + 自己小队排第一"，以及 `arenaGroupLabel`（`:1847-1850`）。
  ⚠️ **不要复用它的兜底分支**（`:1838-1843` 按 `length%3` 切块，那个分组是假的）。
- 队伍名映射 `ARENA_TEAM_MASCOTS`（`web/gameplay.js:1795-1805`）有效键 1-8，
  `web/arena-team-icons/` 确有 8 个 svg 且已 embed，越界会退化成"小队 N"，兜底是安全的。
- **这条链路目前零测试覆盖**（`web/champions.test.cjs` 里搜不到 `renderLiveInsights`/`live-team`），
  改完必须自己补护栏 + 变异测试。

### A-5（P1）斗魂随机英雄（用户说"很关键"）

`top_level_keys` 里有 `benchChampions`、`rerollsRemaining`、`allowRerolling`，
但 `lcuChampSelectSession`（`gameplay.go:3988-3993`）**只解析了 4 个顶层字段**
（gameId/localPlayerCellId/myTeam/theirTeam），这三个全被丢弃；
前端全目录 grep `bench|reroll` **零命中**。
（注：`data/reroll_pool_14_5.json` 是皮肤重铸池，跟这个无关，别混淆。）

**未知点**：`benchChampions` 元素是 `{"championId":N}` 还是裸 int，现有探测只记了顶层键名。
**请在 A-3 那次探测里一并加上元素形状**，确认后再写解析。

改动四层：结构体加字段 → `gameplayLiveResponse`（`:3002-3017`）加出参 →
handler 赋值（`:4106-4125` 的 `champSelectErr == nil` 分支内，英雄名走已有的
`a.overviewChampionNames`/`championName(names, id)`，零额外请求）→ 前端加区块。

---

## B 组（P0）：★"对局开的越多识别越慢"—— 三处无界累积

### B-1（主犯）`overviewQueryCache` 完全没有淘汰

`overview_cache.go:61/:83`：TTL 只有 2 秒（`:11`），但**从不删除过期条目**。
整个文件唯一的 `delete` 是 `:63` 删 flights。全表清空只发生在
`season_stats.go:540/692`、`opgg_insights.go:261`。

每个 value 是完整 `gameplayOverview`（20 场比赛 + 全部参战者）。
每局新增约 10 个陌生 puuid 的键，几十局后堆里全是 TTL 早过期、永不复用的死对象
→ GC 每轮标记变长 → `handleGameplayLive` 这种重分配请求（10 个 goroutine 并发拉战绩+排位）
直接背 GC assist。**这就是"局数越多越慢"最主要的来源。**

**改法**：加过期清理 + 容量上限（参考 `rankScoreCache` 已经做好的 `container/list` LRU，
`rank_insights.go:127-149`）。

### B-2 `gameplayRefs` / `gameplayRefDetails` 无界且在全局写锁下

`main.go:103-104` 两个 map 无界，写操作在**全局 `a.mu` 写锁**下（`gameplay.go:1833-1842`），
仅在召唤师更换时清（`main.go:964-967`、`:1050-1053`，注释写明"刻意保留"）。
每看一页战绩排约 200 次写锁（`gameplay.go:1871-1890`），每次 live 刷新 10 次（`:4218-4221`）。
而 `handleGameplayLive` 开头要 `a.mu.RLock()`（`:4034`）——
**Go 的 RWMutex 挂起写者会阻塞后续所有读者**，champselect 事件到达时 live 请求要排在这串写锁后面，
表越大 rehash/GC 扫描成本越高。

**改法**：独立 mutex（别和全局 `a.mu` 共用）+ 容量上限。

### B-3 `diagnosticDedupCounts` 无界 + 日志写入持锁做重活

`main.go:144` 无界，键 = 事件名 + **整条 JSON payload**（`:1110-1116`），每次都要 `json.Marshal`。
`resetDiagnosticDeduplication`（`:1158`）**只在日志轮转时被调**。
另外 `appendDiagnostic`（`storage.go:574-636`）全程持 `s.mu` 做 2 次 Lstat + 开关文件；
轮转那次要在锁内 `os.ReadFile` 整个 2MB（`:587`），同一把锁还被
`saveSnapshot`→`deduplicateSnapshotsLocked`（`storage.go:398-419`）和 `lp_tracker.go:172-176` 争用。

放大源是 `recordLiveRosterShape`（`gameplay.go:3409-3432`）——
**指纹里含 `championPickIntent`，英雄选择阶段预选一变指纹就变，等于每次刷新写一条日志**。

**改法**：给 dedup map 加上限；`appendDiagnostic` 的重活移出锁；
`recordLiveRosterShape` 的指纹考虑剔除 `championPickIntent`（或单独降频）。

### B-4 已排除的方向（别再查）

`matchModeDiagnosticKeys`（键是 QueueID，上限 128）、`recommendationModeDiagnosticKeys`（64）、
`championDataDiagnosticKeys`（128）、`liveRosterDiagnosticKeys`（512）**均有界**；
`rankScoreCache` 已是 O(1) LRU；SSE 注册/注销成对无泄漏（`live_updates.go:49-56`）；
`startEvents` 的 retry 不叠加 goroutine；定时器均 Stop 后 Reset。
**英雄选择识别链路本身是事件驱动不是轮询**
（`lcu_events.go` → `connection_manager.go:120-126` → 300ms 去抖 → SSE），链路无累积。

### B-5（P2）`sgp_cache_hits` 恒为 0 的两个原因

不是缓存键写错（读写同键同变量，R13/R35 那两个老坑都没复发）：
1. **口径不对齐**：`sgp_requests`/`sgp_bytes` 累加在 HTTP 分页层（每页一次），
   `sgp_cache_hits` 只加在逻辑调用层（整段命中才 +1）。真正挡住请求的三层缓存
   （`overview_cache.go:113`、`gameplay.go:1143`、`season_stats.go:452`）在 SGP 之前就短路了，
   永远不会 +1。**这个指标本身有误导性，建议改口径。**
2. **缓存被赛季回补冲干净**：`sgpCacheTTL=90s`、`sgpCacheMax=64`（`sgp_api.go:80-81`）且是 FIFO 非 LRU，
   而 `season_stats.go:565-615` 的回补每页一个新键，一季几十页远超 64 上限，
   一轮回补就把总览那几条挤光。另 `:709-711` 出现 `partialErr` 时直接 return、跳过缓存写入。

---

## C 组（P1）：hexdata 熔断与"一直转圈"

### C-1 熔断真根因：一个正则漏改

`hexdata_shape_invalid {"failure_kind":"empty","fields":0,"rows":0,"kind":"augments"}` 13 次，
两个 buildId 都空。**但 buildId 非空说明不是上游返回空，是解析器把每一行都淘汰了**
（buildId 在 `hexdata.go:1334` 才赋值，此前的 meta 提取和 `len(tables)!=1` 都已通过，
唯一还能 rows=0 的是 `:1318-1333` 每行都被 `:1325` 的
`len(path)!=3 || len(metrics)!=3` continue 掉）。

**最可能的一行**：`hexdata.go:57` 的 `hexdataGlobalMetric` 正则分隔符写死了间隔号 `·`：
```go
regexp.MustCompile(`globalHexScore\s*([0-9.]+)\s*·\s*胜率\s*([0-9.]+)%`)
```
而**同文件 `:52` 的 `hexdataHeroMetricPattern` 已经改成容错的 `[·，,]`** ——
正是历史上 hero 详情页修过的那个坑，**augments 页漏改了**。
`parseHexdataRarity`（`:1431`）的 `·` 同样写死。

次要嫌疑：`hexdataAugmentPathPattern`（`:51`）`^/augment/(\d+)-([a-z0-9-]+)$` 首尾锚定，
href 变绝对 URL 或带 query 就全灭。

**"两个 buildId 都空"的解释**：不是上游连发两次空数据，而是 `checkedPage`（`:517-524`）
回退旧 buildId 磁盘缓存时要重过同一个 `inspectHexdataPayload`，同一条正则把新旧两份一起判死。

**顺带修**：`hexdata.go:1734/1744` 写死 `recordShapeFailure("augments", 0, 0, "")`，
无论真实原因都伪造一条 `empty`，污染诊断。

### C-2 `empty` 这个标签本身有歧义

`hexdata.go:864` `empty := shape.Rows==0 && shape.Fields==0`，
但 `:474-475` 里 `Fields = len(rows)*5`，所以 **fields 恒等于 rows，两个不是独立信号**。
上游真空响应（`errHexdataEmptyPayload`）也标成 empty ——
**两种完全不同的故障共用一个标签**，建议拆开，并让 Fields 记真实字段数。

### C-3 转圈不是熔断造成的（澄清一个误判）

对局页海克斯**根本不用 augments 这个 kind**，它走 `loadMayhemDetail`（`hexdata.go:1788-1851`），
kind 是 `"hero"` + OP.GG RSC，两路在 `:1841` 合并；熔断按 kind 分离，augments 不拖累 hero。

图一转圈的真判据在前端 `web/gameplay.js:3297-3304`：
```js
if (key === "build") return pending && ((!payload?.build) || (payload?.hasAugments && !payload?.augments));
```
`hextech-aram` 的 `HasAugments:true`，而 `gameplay.go:3045` 的 `Augments` 带 `omitempty`
——**空切片会让整个字段消失**，于是 pending 期间一直遮罩。
`pending` 有两道兜底（finally 必删 flight、30s TTL），所以不会真的永远转，
用户看到的是这段最长 20~30 秒的窗口。

**改法**：`gameplay.go:3045` 去掉 `Augments` 的 `omitempty`（让空数组也序列化成 `[]`），
这样前端能区分"还没到"和"到了但是空"。

---

## D 组（P1）：海克斯与出装的展示调整（图二）

1. **每个品质放一行，放不下换行**（现在是三列并排）。
2. **当前品质没有数据就不展示这一档**（图二里"黄金 1个"那种只有一条的也照常展示，
   但完全空的档不要占位）。
3. **选用率没有就换成梯度（档位）**——图二里有些条目显示"HexScore — · 胜率 — · — 场"，
   这种没有统计的应该退化成只显示档位徽章（S/A/B），不要画一排破折号。
4. **图标和英雄详情页对齐**（尺寸/圆角/边框统一）。

参照：详情页 `renderRecommendedAugments`（`web/champions.js:1470-1471`）。
注意 R45 已经把"取前 9 条 + 顺序"统一成详情页口径了，**这轮改的是分栏布局这一层**。

---

## E 组（P1）：斗魂核心装响应式（用户说现在太保守）

- **宽屏改成 3 个一行**
- **窄屏（图六那种尺寸）现在是 1 个一行，太保守，应该 2 个一行**

相关 CSS 在 `web/build-item-row.css`（R43 抽出来的共享文件）和 `web/gameplay.css` 的
`.item-core-column`/`.item-depth-columns`。注意这套是共享的，改之前确认不会连累英雄详情页。

---

## F 组（P1）：★侧边栏"对局提示灯"—— 我们之前修错对象了

**用户说的不是连接状态条，是"对局"导航项上的小圆点。**

这个东西**确实存在**：`renderBeacon()`（`web/gameplay.js:4651-4665`）向 `#section-live`
append 一个 `<span class="live-beacon">`，带 `data-tooltip="检测到新的对局，点击查看"`，
显示条件是 `beacon.active && !beacon.acked && section!=="live"`，
`beaconPhases = ["ChampSelect","GameStart","InProgress","Reconnect"]`（`:4635`）。
CSS 在 `web/gameplay.css:1268`（绝对定位 + 呼吸动画），**没有任何收起态覆盖规则**。

**根因**：`web/app.css:269`
```css
:root[data-sidebar="collapsed"] .section-tab span { display: none; }
```
本意是隐藏导航文字 `<span>对局</span>`，但它是**通配 span 选择器**，
把同为 `#section-live` 子 span 的 `.live-beacon` 一起 `display:none` 了。
特异性 (0,3,1) vs (0,1,0)，无论加载顺序都赢。

**改法**：改成 `.section-tab > span:not(.live-beacon)`（或给文字标签加专用 class）；
复核 `app.css:415` 的 `.section-tab{overflow:hidden}` 会不会裁掉圆点；
收起态侧栏只有 70px 宽，`gameplay.css:1268` 的 `right:9px` 需要重新定位。

**另外一个独立缺陷**（R43-E 那轮的遗留）：`web/app.js:476` 在收起态仍然写
`dataset.tooltipOverflow=".connection-label"`，而 `app.css:269` 恰好把
`.connection-label` 设成了 `display:none` → `scrollWidth===clientWidth===0` →
溢出判据不成立 → tooltip 被自己挡死。**所以 R43-E 那轮实际没生效**，一并修掉。

---

## G 组（P1）：刷新提示与频率（图五）

### G-1 只显示"自动刷新已开启"
`web/gameplay.js:3180`：
```js
const note = state.liveLoading ? "正在刷新" : state.settings.liveRefresh ? "自动刷新已开启" : "手动刷新";
```
`state.liveLoading` 在 `:2842` 置 true、`:2863` finally 置 false，而 `loadLive` 在
`:2844` 和 `:2864` 各重绘一次 —— **所以每轮轮询这行必闪一次**。
**去掉"正在刷新"这个分支即可。**

### G-2 按阶段分档刷新频率
`state.settings.liveInterval` 默认 **3 秒**，调度是自链式 setTimeout（`:4077-4081`），
**完全没有按 phase 区分** —— ChampSelect / InProgress / Lobby / EndOfGame 一视同仁 3 秒。

按用户要求改成：**ChampSelect 保持 3 秒**（这个阶段信息最重要），
**InProgress 拉长到 15~30 秒**。phase 取 `state.live?.phase`，
枚举参考 `phaseLabel`（`:4192`）。

---

## H 组（P1）：云顶之弈明确提示不支持

**现状**：TFT 队列（1090/1100/1130/1160 等）在 `supportedQueueDefinitions`
（`queue_groups.go:86-125`）里**完全没登记** → `modeGroup="other"` →
`resolveGameplayRecommendationMode`（`gameplay.go:3156-3189`）的 `gameMode="TFT"`
不匹配任何 case → `InternalMode="unsupported"`, `IsFallback=true`。
不会 400 也不会崩，但会**静默错误展示**：8 人大概率全在 teamOne → 全部 teamId=100 →
界面变成"我方 8 人 / 对方空"，地图显示"地图未知"，三个推荐页签全是空态。

**改法**：判据用 **`gameMode == "TFT"`**（比 queueId 白名单稳，
队列白名单在本项目已翻车四次）。
给 `gameplayLiveResponse` 加 `Unsupported bool` + `UnsupportedReason string`，
前端在 `renderLive`（`web/gameplay.js:3187-3218`）走 `emptyState(...)` 分支渲染
**"暂时不支持此模式，敬请期待"+ 图标**。

---

## I 组（P1）：加载态没有居中（图一）

**居中的 CSS 声明本身是有的**（`web/gameplay.css:1029-1041` 有 `justify-items:center` +
`text-align:center`），**偏左是几何问题不是对齐问题**：
`.panel-loading` 是 `position:absolute; inset:0`，包含块是 `.recommendation-panel`，
而该面板 `gameplay.css:1025` **没有 `min-width:0`** —— 作为 flex/grid 后代时被内部宽表格撑宽，
遮罩跟着撑宽 → 与可视区错位。同时 `align-content:center` 在很高的 build 面板里
把转圈推到整块面板的纵向中点。

**改法**：`gameplay.css:1025` 加 `min-width:0`；`:1029-1041` 改 sticky 或限高居中。
**加 `justify-content` 无效，别试。**

---

## J 组（P2）：客户端窗口

1. **窗口自适应太保守，可以再放大一点**——请找到根据屏幕分辨率算初始窗口大小的逻辑
   （`desktop/main.cjs`），把比例调大。
2. **顶部空白区域按住拖动有时没反应**——这是 Electron 的 `-webkit-app-region: drag` 覆盖不全。
   请检查 `web/app.css` 里 `.window-drag` 的范围，确认顶栏空白处都可拖动，
   同时保证按钮/输入框上是 `no-drag`（否则按钮点不了）。

---

## K 组（P2）：标签页与召唤师横条（图七）

1. **标签页上去掉大区名称**（"黑色玫瑰"那个角标）。
2. **召唤师横条上的大区**挪到"召唤师等级"右边、"隐藏玩家"标签左边。
3. **如果查看的玩家正在对局中**，就在现在大区那个位置放对局提示。
4. **对局提示做成标签形式**：内容是"模式 + 英雄 + 已进行时间"，
   参照好友列表里"对局进行中"那个提示的样式，但**比现在大一点，底部对齐**。

---

## L 组（P2）：好友观战 + 韩服回放（研究性）

1. **好友观战按钮**：好友在游戏中时，现在悬停只有"战绩"，
   要求在战绩左侧加一个**观战按钮**，点击后在客户端里看实时对战。
   LCU 有现成端点可以拉起观战（`/lol-spectator/v1/spectate/launch`），请确认可用性。
   好友的对局状态判断参考 `web/friends.js`。
2. **韩服回放能否在国服客户端播放**（用户说"研究一下尽量实现"）：
   请调研 LCU 的 `/lol-replays/v1/rofls/{gameId}/download` 与 `/watch` 系列端点，
   确认跨服回放的可行性和限制（大概率受版本号和分区限制）。
   **如果做不到，请明确说明技术原因，不要硬做。**

---

## M 组（P1）：★英雄详情页核心装恢复到 5 条 —— 这是我 R45 改错了，要回退

**先说清楚这是谁的锅**：R45 的 D-1 是我写的，当时的理由是"后端提到 15 条了但详情页
还停在 `slice(0,5)`，口径不一致"，于是让你把详情页也提到 15。
**但我没有确认用户是否真的想让详情页显示更多** —— 用户当初抱怨的是
"**对局页海斗**核心装怎么这么少"，跟英雄详情页的排位模式完全是两回事。
现在用户看到详情页排出十几条，明确说"最多只要 5 条就行"。**这是我过度推广口径统一的结果。**

**改法**：把 R45 D-1 在**英雄详情页**那两处改回 5：
- `web/champions.js:1706 buildItemRoutes`（排位/大乱斗详情页）
- `web/champions.js:933 renderMayhemItemRoutes`（海斗详情页）

建议保留 `CORE_RECOMMENDATION_LIMIT` 这个常量的写法（比散落字面量好），
只是把值改成 5，或者拆成两个常量区分详情页/对局页。

**对局页不要动**：`LIVE_CORE_OPTION_LIMIT=15` + `PREVIEW_LIMIT=6` + 展开按钮这套保持现状，
用户这次没有抱怨对局页。

**测试要同步改**：R45 时为这条加过护栏（`champions.test.cjs` 里断言两处都用
`CORE_RECOMMENDATION_LIMIT`、以及 `:824` 等几处），改值之后断言要跟着改，
**别把护栏删掉，改成断言新的值**。

---

## N 组（P1）：英雄详情页要连"当前打开的英雄"一起持久化

R44-G 组做了英雄页持久化（模式/两个搜索框/图鉴页签/斗魂首位页签/符文页签），
但**"当前正打开着哪个英雄的详情页"没有持久化** —— `resetTransientChampionState()`
（`web/champions.js:1958` 附近）把"详情"明确当成临时态清掉了。

用户要求：**在英雄详情页里离开英雄模块，再回来时还应该停在那个英雄的详情页**，
而不是退回列表。

**改法**：把"当前详情页的英雄 ID"也纳入持久化（复用已有的 `readSetting/writeSetting`，
不要新造存储），回来时自动重新打开该英雄的详情。
注意区分：**英雄 ID 应该持久化，但详情数据本身、加载态、错误态、请求令牌不应该持久化**
（回来时应该重新拉取，而不是复用可能已过期的旧数据）——
R44-G 组当时就是这么划分的，沿用同一条界线。

**边界情况**：如果持久化的那个英雄 ID 在当前模式下不存在（比如切了模式），
要优雅退回列表，不能白屏或报错。这条要写进测试。

---

## 交付要求补充

- M 组是回退我上一轮的改动，**注意别把 R45 加的护栏一起删掉**，改成断言新值。
- N 组的"英雄不存在时优雅退回"要有测试覆盖。
- **A 组的预期已经下调**：本轮不追求做出 6 个小队，做到"英雄选择阶段体面降级 +
  进游戏后去掉错误的敌我二分"即可。探测是为了把这条路彻底结案，不是为了继续尝试。

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **A 组先只做 A-3 的探测（加两行诊断字段），拿到真机结果再做 A-4/A-5 的解析。**
  这条纪律前几轮执行得很好，继续保持。
- **B 组是本轮性能主线**，三处累积都要修，且要补测试证明"反复调用后容量不会无限增长"。
- **变异测试**：B-1（缓存淘汰）、C-1（正则容错）、C-3（omitempty）、F（beacon 选择器）、
  G-2（按阶段分档）、H（TFT 判据）六处必做。
- D/E/I/K 组用 Playwright 在 1440/900/420 三个宽度截图作为验收证据。
- L 组是研究性任务，做不到就写清楚原因，不要硬凑。
