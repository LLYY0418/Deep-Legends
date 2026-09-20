# WORKLIST-R90 — 斗魂对局中小队分组永不生效 / 对局中停止无意义自动刷新

诊断人：Claude（只读诊断，未改仓库任何代码文件；本文件是本轮唯一新增物）。执行人：GPT。

日志样本：`lol-loot-diagnostics-0913-1646.jsonl`（12663 行）、`-1951.jsonl`（9761 行）、`-2329.jsonl`（9934 行），
覆盖 0913 全天，含 **33 次斗魂 InProgress 名单快照**（1646 日志 3 次 / 1951 日志 17 次 / 2329 日志 13 次）。

> ⚠️ **行号有保质期。** 诊断期间仓库正在被并行会话改动（`gameplay.go` mtime 09-14 11:41、
> `web/gameplay.js` mtime 09-14 11:59，本文件写于 12:0x）。下文行号是 **09-14 12:00 快照**，
> 请一律**按函数名/代码原文定位**，不要照抄行号。

---

## 口径决定（不要自行更改）

1. **小队名称本轮只显示「小队 1~6」，不显示吉祥物名（暗影狼/魄罗…）。** 已跟用户确认。
   理由见 P0-3：游戏内任何接口都不给小队编号，顺序推断出来的「第 6 块」不等于客户端的「小队 6」，
   直接套 `ARENA_TEAM_MASCOTS` 会张冠李戴。等 P0-4 的赛后真值回查跑过几局、确认块序号恒等于
   `playerSubteamId` 之后，下一轮再把名字打开。
2. **英雄选择阶段（ChampSelect）一律不分组，维持现状**（只展示自己小队 3 人）。
   客户端在选人阶段本来就不向玩家展示别队构成，提前算出来会越过 Riot「不得提供玩家本不应知道的对局内信息」这条线。
   分组只在 `InProgress` / `Reconnect` 输出。
3. **对局中（InProgress/Reconnect）数据齐了就彻底停止定时刷新**，只保留：手动刷新按钮、
   客户端阶段变化（gameflow phase 事件）、切回前台时的阶段核对。已跟用户确认，不要留 5 分钟兜底。
4. **不许再出现 queueID 整型字面量比较**（R88 的 AST 守卫），新代码一律走 `queue_groups.go` 登记表。

---

## P0-1 · 诊断：进入游戏后为什么还是不分组（三条独立证据，全部来自本次日志）

### 证据 A：游戏进程的 playerlist 根本没有小队字段（不是"进游戏就能拿到"）

`gameplay.go:39` 唯一的游戏进程数据源是 `https://127.0.0.1:2999/liveclientdata/playerlist`。
三份日志共 **63 条 `live_client_playerlist_shape`**，凡是 HTTP 200 的，`element_keys` **恒为同一组 19 个键**：

```
championName, isBot, isDead, items, level, position, rawChampionName, rawSkinName,
respawnTimer, riotId, riotIdGameName, riotIdTagLine, runes, scores, skinID, skinName,
summonerName, summonerSpells, team
```

- `subteam_field_values` **恒为 `null`**（`parseLiveClientPlayerList` 会扫描任何含 `subteam` 的键，一次都没扫到）。
- `team_values` 在斗魂里恒为 `{"ORDER": 18}`（18 人全在 ORDER，不是两队五人）。
- 因此 `result` 恒为 `"ungrouped"`、`grouped` 恒为 `false`。

⇒ `parseLiveClientPlayerList`（`gameplay.go:4491`）里挑 `groupField` 的那段（只认 `subteamId` / `playerSubteamId`）
**在真实数据下永远选不出字段**，直接 `return snapshot, shape, nil`。这条路是死的，跟"进没进游戏"无关。

### 证据 B：LCU 的 gameflow 名单也没有小队字段，`teamParticipantId` 不是小队且不稳定

`lcu_gameflow_session_shape`（queue 1750 / CHERRY / InProgress）里 `team_one_nonzero_counts` 只有七个键有值：
`championId, profileIconId, puuid, selectedPosition, selectedRole, summonerId, teamParticipantId`。
`diagnosticNonZeroKeyCounts` 是遍历所有键的，所以 **`teamOwner`、`summonerName`、`summonerInternalName`、
`lastSelectedSkinIndex` 是被量过、确认全空/false 的**（这点很重要，见 P0-2 第 3 条）。

而 `teamParticipantId` 是**组队小队（premade party）编号，不是斗魂小队**：

| 日志时间 | 阶段 | `team_one_team_participant_id_counts` | 致命点 |
|---|---|---|---|
| 2329 / 11:52:55（局 8980574903） | InProgress | `{1:2, 2:2, 3:1, 4:2, 5:6, 6:1, 7:1, 8:1, 9:1, 10:1}` | **一个 id 下挂 6 个人** |
| 2329 / 12:37:39（局 8980672961） | InProgress | `{1:2, 2:1, 3:3, 4:1, 5:1, 6:2, 7:3, 8:4, 9:1}` | 一个 id 下挂 4 个人 |
| 2329 / 11:51:27（**同一局** 8980574903） | ChampSelect | `{1:2, 2:1, 3:1, 4:2, 5:2, 6:3, 7:3, 8:1, 9:2, 10:1}` | 与 60 秒后的 InProgress **分布不一致** |

3 人一队的模式里，6 人 / 4 人一组不可能是一个小队；同一局同一批人在两个阶段分布还会变。
另有佐证：`gameplay.go:6254` 附近 `applyLivePremadeAssignments` 自己就是拿 `TeamParticipantID` 当**组队信号**用的——
同一个字段不可能既是组队又是小队。

### 证据 C：所以代码里唯一的兜底 `arenaSessionOrderGroups` 永远走不通

`gameplay.go:4886 arenaSessionOrderGroups`，三道门：

```go
if queueID != 1750 || len(players) != 18 { return nil, false }   // 门1
if player.TeamParticipantID <= 0 { return nil, false }            // 门2
if previous := partyGroup[player.TeamParticipantID]; previous != "" && previous != group { return nil, false } // 门3
```

- **门 3 是致命的**：它要求"同一个 `teamParticipantId` 的人不能跨 `index/3` 块"。证据 B 里 6 人/4 人一组**必然跨块** ⇒ 直接失败。
- **门 1 还会额外杀掉 17 人局**：1951 日志里 17 人的 InProgress 名单出现 11 次（局 8980214295、8980420794、8980482581）。

**实测闭环**：33 次斗魂 InProgress 的 `live_roster_shape` 里，`arena_grouped` **全部为 false**、
`arena_group_source` **全部为空串**、`arena_group_counts` **全部为 `{}`**。零例外。
`gameplay.go:6031` 的 `response.ArenaGrouped` 从未为真 ⇒ 前端 `arenaLivePlayerGroups`
（`web/gameplay.js:4623`，第一行就 `data?.arenaGrouped !== true` 直接 return []）⇒ 铺平成一条大名单。

### 证据 D：为什么一直没人发现——两条路都被**假数据**养绿了

- `gameplay_test.go:2396`（`r62ArenaFixturePlayers`）给 playerlist 塞了
  `"playerSubteamId": index/3 + 1` —— **真实接口 63 次采样一次都没返回过这个键**（证据 A）。
- `gameplay_test.go:2756`（`TestArena1750UsesCorroboratedGameflowRosterOrderWhenPlayerListHasNoSubteams`）
  构造的 `teamParticipantId` 是 `index+1`（每块内不跨界）—— 真实客户端从不长这样（证据 B）。

两条测试都是绿的，功能一次都没工作过。**这是本轮最需要一并修掉的东西：fixture 的形状必须改成日志里的真实形状。**

### 证据 E（附带发现，影响修法）：LCU 名单会少人，游戏进程名单不会

1951 日志 10:36~11:04 那局：`live_roster_shape` 一直是 **17** 人（来自 gameflow teamOne），
而同窗口三条 `live_client_playerlist_shape`（10:36:52 / 10:40:10 / 11:04:09）**`player_count` 稳定为 18**。
⇒ **完整名单只能以 2999 端口的 playerlist 为准**，gameflow teamOne 会缺人。

---

## P0-2 · 改法：分组主路改成「游戏进程名单顺序 + 自身小队佐证」

上游既然没有小队字段（这不是我们的 bug，是 Riot 的 Live Client Data 在非标准模式下复用了旧 JSON），
业界通行做法就是**按 allPlayers 的数组顺序切块**。我们比别人多一张牌：**我们知道自己的小队是谁**
（`gameplay.go:5839` 的 `rememberArenaAllies` 在 ChampSelect 阶段已经把自己 + 队友记下来了，
`filterArenaChampSelectPlayers` 只留有真 PUUID 的条目，日志里恒为 3 人），可以用它把顺序假设**当场证伪**。

1. **新增 `orderGroupsFromLiveClient`**：直接用 playerlist 的数组下标切块。
   - 每队人数**从 `queue_groups.go` 登记表取**（给 arena 条目加一列 squad size：1700/1710 → 2，1750 → 3），
     **不要在 `gameplay.go` 里写 1750 字面量**（R88 的 AST 守卫会抓）。
   - 前置校验：`count % squadSize == 0 && 6 <= count <= 18`。
2. **佐证（必须做，不许省）**：自己 + `rememberArenaAllies` 记住的队友，**必须全部落在同一块**。
   - 全在同一块 ⇒ 接受，`arenaGroupSource = "live-client-order"`。
   - 不在同一块 ⇒ **不分组**，并写一条诊断（见 P0-4），不要硬着头皮输出。
   - 队友信息缺失（用户是对局中途才开的应用）⇒ 仍然接受顺序分组，但 `arenaGroupSource = "live-client-order-unverified"`，
     诊断里标出来，便于后续统计这条路的准确率。
3. **身份匹配不能再用 raw 名字**：`gameplay.go:4764 liveClientGroupingForPlayer` 现在只吃
   `lcuLivePlayer` 的 `SummonerName / GameName / TagLine`，而**国服 gameflow 这三个字段全是空的**（证据 B）。
   必须改成用**解析后的** summoner（`gameName#tagLine` / `summonerName`），和
   `gameplay.go:6015~6023` 那段 position 匹配同源——建议抽一个共用的 identity keys 函数，两处都走它。
   （否则即使上游哪天真给了 subteam 字段，这条路照样匹配不上。）
4. **退路保留 gameflow 顺序**：playerlist 拿不到（游戏进程还没起来）时，用 gameflow teamOne 顺序做同样的切块 +
   同样的自身小队佐证，`arenaGroupSource = "session-order"`。
   **但要删掉 `teamParticipantId` 那道佐证门**（证据 B 已把它证伪），并**删掉 `len(players) != 18` 硬门**。
5. **17 人局的口径**：以 playerlist 的 18 人切块，再把 gameflow 的 17 人按身份映射进去；
   有一块只剩 2 人是正常的，不要因此整体失败。
   相应地 `validateArenaLiveGroups` / `validArenaGroupAssignments` / `liveClientArenaDistribution`
   （`gameplay.go:4857 / 4871 / 4423`）的"每块 1~3 人"约束要按 squad size 参数化，别继续写死 3。
6. `liveClientSnapshotForGame`（`gameplay.go:4648` 附近）里
   `usable = shape.Grouped && shape.PlayerCount >= expectedArenaPlayers` 这条要改：
   斗魂下 `shape.Grouped` 永远 false，导致**探针永远 `succeeded=false`，每次都重新拉一遍 playerlist**。
   新口径：拿到 `PlayerCount >= expectedArenaPlayers` 的 200 响应就算成功（顺序信息就是有效载荷），
   `Grouped` 只用来标记"上游真给了 subteam 字段"这种未来情形。

---

## P0-3 · 名称：顺序推断时 `ArenaMascotMapping` 必须是 false

`gameplay.go:5891` 现在在 session-order 成功时直接 `arenaMascotMapping = true` ——**这是没有依据的**：
顺序切块只能证明"这三个人是一队"，证明不了"这一队是客户端里的 6 号小队（暗影狼）"。
前端 `web/gameplay.js:4623 arenaLivePlayerGroups` 已经有 `小队 ${index + 1}` 兜底，
只要 `arenaMascotMapping !== true` 就会走兜底，不用改前端。

**要做的**：顺序推断路径一律 `ArenaMascotMapping = false`；只有上游真给了 `subteamId/playerSubteamId`
（`arenaLiveClientMascotMapping`，`gameplay.go:4910`）时才为 true。

---

## P0-4 · 赛后真值回查埋点（这轮必须做，否则下轮没法开吉祥物名）

对局结束后我们**本来就会**通过 SGP 取这局的详情，而 SGP 的 participant 里**是有真值的**——
三份日志里 `sgp_participant_keys` 出现 101 次，键表里稳定包含 `playerSubteamId` 和 `subteamPlacement`。

**要做的**：在 arena 对局结束、拿到 SGP 详情时，把**在线推断的分组**与**真值**比对，写一条诊断：

```json
{"event":"arena_group_truth_check","game_id":8980574903,"queue_id":1750,
 "source":"live-client-order","player_count":18,"squad_size":3,
 "partition_match":true,                  // 切块（谁和谁一队）对不对
 "block_to_subteam":[1,2,3,4,5,6],        // 第 k 块实际对应的 playerSubteamId
 "block_to_subteam_identity":true,        // 上一行是否恒等于 [1..n]
 "self_block":6,"self_subteam_id":6,
 "verified_by_allies":true}
```

- `partition_match` 回答「顺序切块这条路到底准不准」。
- `block_to_subteam_identity` 回答「能不能直接把块序号当小队编号，从而显示暗影狼这些名字」。
- 攒够 3~5 局都为 true，下一轮把 P0-3 的 `ArenaMascotMapping` 打开即可，**不要这轮就赌**。
- 埋点写入必须走 `a.appendDiagnosticEvent`（R89 已经指出 `arena_rankings_parse_failed` /
  `desktop_startup_stage` 绕过它导致缺 `build_fingerprint`，别再踩第三次）。

---

## P1 · 对局中不要再一直刷新

### 现状与代价（实测）

| 事实 | 证据 |
|---|---|
| 前端在 InProgress/Reconnect **写死 20 秒**轮询，用户在设置里选的间隔完全无效 | `web/gameplay.js:115 liveRefreshDelayMs` 第 3 行 `return 20_000`；2329 日志 `live_refresh_client` 中 `reason=load, source=interval, section=live` 共 30 条 |
| 斗魂快照**永不落缓存**，每次请求都整轮重算 18 人 | `gameplay_refresh.go:197` `groupingReady := phase=="ChampSelect" \|\| !isArena \|\| response.ArenaGrouped`，斗魂恒 false ⇒ `c.at` 永不写入 |
| 斗魂对局中 12 分钟（11:53–12:05）打出 **778 次 LCU 请求**：`/lol-summoner/v2/summoners/puuid/{id}` **417 次**、`/lol-match-history/.../matches` **153 次** | 2329 日志 `lcu_request` 按 `count` 求和 |
| 整名单装配 17/18 人时 p90 **4995ms**、最大 **12514ms**（n=129） | 2329 日志 `live_load_cost` |
| 用户**不在"对局"页**时也在整名单重载 | 2329 日志 `live_refresh_client`：`load/sse/suite` 34 条、`load/sse/champions` 9 条 |

而对局中这些数据（段位、近期战绩、玩家身份、推荐）**在这局结束前一个字都不会变**——用户说得对。

### 要做的

1. **`liveRefreshDelayMs` 不再对 InProgress/Reconnect 返回固定 20s**：
   - 快照**完整** ⇒ 不排定时器（`scheduleLiveRefresh` 直接 return，`web/gameplay.js:5414`）。
   - 快照**不完整** ⇒ 继续按 20s 重试（建议前 60 秒 5s，之后 20s），**最多 8 次**后停，停后只剩手动。
   - **「完整」的判定口径（写成一个独立函数，测试要能直接调）**：
     `available && players.length > 0 && 没有 player.historyState 处于 pending && （非斗魂 或 arenaGrouped===true 或 已明确判定不可分组）`。
2. **页面上必须有可见出口**：定时器停掉后在名单顶部给一行状态 +「刷新」按钮
   （文案建议：「对局中数据不再变化，已停止自动刷新」）。**没有按钮就直接停，用户会以为卡住了。**
3. **阶段变化仍然刷新**：`handleGameplayPhase`（`web/gameplay.js:6104`）→ `queueLiveEventRefresh` 保留不动。
4. **不在"对局"页时不要整名单重载**：`queueLiveEventRefresh`（`web/gameplay.js:6076`）
   在 `state.section !== "live"` 且 `phaseChanged === false` 时直接 return——
   后端 `observeGameplayPhase`（`gameplay_refresh.go:70`）已经在阶段切换时做了 prewarm，覆盖了"切过去要快"的诉求。
5. **后端改成分层 TTL，而不是"干脆不缓存"**（`gameplay_refresh.go:128 / 197`）：
   - 快照完整 ⇒ 按 `gameId` **整局有效**（`gameId` 变化、phase 变化、`liveSnapshots.invalidate()` 时失效）。
   - 快照不完整（斗魂还没分组、playerlist 还没起来）⇒ 维持现有 20s，让重试路径活着。
   - 现在这条 `groupingReady` 的写法是"不完整就完全不缓存"，代价就是上面那 417 次 summoner 读取。

---

## 验收判据（每条都要有变异测试，见下）

1. **真实形状 fixture**：重写 `r62ArenaFixturePlayers`——
   playerlist 元素只允许出现证据 A 那 19 个键（**禁止出现 `playerSubteamId` / `subteamId`**）；
   gameflow teamOne 只给证据 B 那 7 个有值字段，`summonerName` 留空，
   `teamParticipantId` 用真实分布 `{1:2, 2:2, 3:1, 4:2, 5:6, 6:1, 7:1, 8:1, 9:1, 10:1}`。
   **改前这份 fixture 必须红（不分组），改后必须绿（6 块 × 3 人）。**
2. **变异 1**：把切块粒度 `index/squadSize` 改成 `index/2` → "自己+队友同块"佐证必须把它打回不分组 → 测试变红。
3. **变异 2**：删掉"自己+队友同块"这道佐证 → 必须有一条用**打乱顺序的 playerlist** 的测试因此变绿
   （证明这道佐证确实在挡东西，而不是摆设）。
4. **变异 3**：把身份匹配改回只吃 raw 名字 → 在"gameflow 无名字"的国服 fixture 下必须变红。
5. **变异 4**：17 人 gameflow + 18 人 playerlist 的用例，分布必须是 `5×3 + 1×2`；
   把"以 playerlist 为准"改回"以 gameflow 为准"必须变红。
6. **变异 5**：`ArenaMascotMapping` 在顺序推断路径上改回 `true` → 必须有测试变红（守住 P0-3 口径）。
7. **刷新（前端，`web/*.test.cjs`）**：用假计时器验证 InProgress 完整快照之后**不再排新的定时器**；
   把"完整"判定函数改成恒 `false` → 必须退回 20s 轮询且测试变红。
8. **刷新（后端）**：同一 `gameId` 连续两次 `/api/gameplay/live`（**斗魂未分组的情况也一样**）
   必须只触发一次完整装配 → 去掉分层 TTL 必须变红。
9. **AST 守卫**：新增的分组代码不得出现 queueID 整型字面量比较；确认新文件/新函数在守卫的扫描范围内
   （R88 的守卫是按目录/文件扫的，新函数如果落在没被扫的文件里等于没守）。

---

## 本轮明确不做的事

- 不猜、不硬编码吉祥物名映射——等 P0-4 的 `arena_group_truth_check` 出结果。
- 不改 ChampSelect 阶段的展示口径（仍只显示自己小队 3 人 + `arenaChampSelectNotice`）。
- 不动 `/api/gameplay/phase` 的 1s/12s 心跳（`BEACON_FAST_POLL_MS` / `BEACON_IDLE_POLL_MS`）——
  它便宜，且是阶段变化的唯一来源，停了它 P1 第 3 条就没了。
- 不去动 5v5 的 ChampSelect 3 秒刷新（选人阶段数据真的在变）。
- 不新增任何对 2999 端口以外的游戏进程接口的探测。

---

## 只能真机验收的清单

1. 打一局斗魂（queue 1750），**进入游戏后**截图确认 6 块 × 3 人分组；导出日志确认
   `live_roster_shape` 出现 `arena_grouped: true`、`arena_group_source: "live-client-order"`、
   `arena_group_counts` 为六个 3（或 5 个 3 + 1 个 2）。
2. 对局中**静置 3 分钟**后导出日志：`live_load_cost` 不应再每 20 秒出现一条（应只有进入时的 1~2 条）；
   `lcu_request` 里 `/lol-summoner/v2/summoners/puuid/{id}` 的累计 `count` 不应随时间线性增长。
3. 对局结束后导出日志，看 `arena_group_truth_check` 的 `partition_match` 与 `block_to_subteam_identity`
   ——这两个值决定下一轮能不能把「暗影狼」这些名字打开。
4. 中途关掉应用再打开（模拟"对局中途才开应用"）：确认走 `live-client-order-unverified` 也能分组，
   且页面上的「已停止自动刷新 + 手动刷新」状态正常。
