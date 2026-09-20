# WORKLIST-R95 — 斗魂小队真相 / 刷新清空生涯 / 对局页抖动 / 锁定语义 / 职业选手识别

诊断人：Claude（只读诊断 + 四路子代理并行核实，**未改仓库任何业务代码**）。执行人：GPT。
日志：`lol-loot-diagnostics-0915-2244.jsonl`（2576 行）。
**`build_fingerprint = b0f152559d9b`，`node desktop/source-fingerprint.cjs` 输出完全一致——
这份日志就是当前源码（含 R91 主工单 + ADDENDUM + ADDENDUM-2 全部改动）跑出来的，不存在旧构建干扰。**

对局：`game_id 8984267096`，`queue_id 1750`（斗魂 3x6，18 人），国服 HN10（`hn10-k8s-sgp.lol.qq.com`）。
用户完整打完了一局。

---

# P0 ★★★ 斗魂小队分组 —— 六轮以来第一次拿到确定性结论

## 结论先行：前几轮的方向全错了，而且错得有据可查

**不是"代码没走到"、不是"probe 失败"、不是"时机不对"。是"客户端根本没给这个数据，而我们连它给没给都没查过"。**

下面四条全部是日志硬证据，不是推测。

---

## P0-1 「加载页面（GameStart）取数据」这条路已被证伪，**GameStart 只存在了 308 毫秒**

R91-ADDENDUM 当时写的是"真实对局里 `GameStart` 通常有 10~30 秒"。**这句话是错的。**

```
gameflow_phase_client  phase=GameStart   phase_changed=true  previous=ChampSelect  received_at=1789482169587
gameflow_phase_client  phase=InProgress  phase_changed=true  previous=GameStart    received_at=1789482169895
                                                                                    ↑ 相差 308 ms
14:22:49.897  live_prewarm {"phase":"GameStart","duration_ms":308,"error_kind":"canceled","players":0}
```

`live_prewarm` 的 `duration_ms=308` + `error_kind="canceled"` 与两次相位事件的时间差**精确相等**：
GameStart 预热刚发起 308 毫秒就被 InProgress 的相位切换取消，`players: 0`，一个玩家都没来得及取。

**旁证（更硬）**：`gameplay.go:5757` 是无条件的——只要 `phase == "GameStart"` 且 session 读到了就记一条
`lcu_gameflow_session_shape`。而整份日志里 `lcu_gameflow_session_shape` 只有 3 条：
`ChampSelect`(14:21:24)、`InProgress`(14:22:50)、`Reconnect`(14:25:45)，**没有任何一条 GameStart**。
这反证 `loadGameplayLive` 从来没有以 `phase="GameStart"` 真正跑完过。
`arena_group_order_rejected {phase:"GameStart", reason:"phase-not-attempted"}` 两次，与此完全自洽。

### 所以要纠正一个认知错误

**用户看到的"加载页面"（英雄头像 + 进度条那几十秒）根本不是 LCU 的 `GameStart` 相位。**
LCU 的 `GameStart` 是 ChampSelect 和 InProgress 之间一个 0.3 秒的瞬时状态；
用户眼里的加载页，从后端视角看**已经是 `InProgress` 了**（游戏进程在起，LCU 已经报 InProgress）。

`gameplay.go:6025` 的 `phase == "GameStart" || ...` 这个扩容**留着不用删**（没有害处，别的客户端版本可能不同），
但**必须停止把它当成解法**。真正的窗口一直都在 `InProgress`。

---

## P0-2 `session-order` 这条路永远走不通：**gameflow 名单只有 17 人，不是 18**

```
14:22:50.013  lcu_gameflow_session_shape  phase=InProgress  queue_id=1750
  "team_one_length": 17,                       ← 18 人的模式，只给 17
  "team_two_length": 0,
  "team_one_nonzero_counts": {"championId":17,"profileIconId":17,"puuid":17,
                              "selectedPosition":17,"selectedRole":17,"summonerId":17,
                              "teamParticipantId":17},
  "team_one_team_participant_id_counts":
      {"1":1,"2":1,"3":2,"4":2,"5":1,"6":1,"7":1,"8":1,"9":1,"10":2,"11":1,"12":1,"13":1,"14":1}
```

两件事同时被这一条日志钉死：

1. **17 ≠ 18，且 17 % 3 ≠ 0** → 直接触发 `arena_group_order_rejected {reason:"roster-not-divisible", source:"session-order"}`。
   这不是偶发：R90 那份日志也是 17/18，**两轮不同对局同一个数字，是系统性的，不是掉线**。
2. **`teamParticipantId` 是组队房间号，第三次被证实，可以彻底结案了。**
   14 个不同取值分摊 17 人（11 个 1 人 + 3 个 2 人 = 11 + 6 = 17），完美对应"11 个单排 + 3 个双排"。
   如果它是小队号，应该是 6 个值各 3 人。**以后任何人再提"用 teamParticipantId 分组"，直接引用这一行否掉。**

`Reconnect` 那条（14:25:45）分布一模一样，说明整局稳定。

---

## P0-3 `live-client-order` 这条路也已被证伪：**2999 名单顺序不是小队顺序，护栏是对的**

2999 接口最终是**拿到了完整 18 人**的（第 2 次 probe，耗时 18.4 秒）：

```
14:23:08.425  live_client_probe_timing {"attempt":2,"elapsed_ms":18395,"player_count":18,"success":true}
14:23:08.464  live_client_playerlist_shape
  "http_status": 200, "player_count": 18,
  "element_keys": [championName,isBot,isDead,items,level,position,rawChampionName,rawSkinName,
                   respawnTimer,riotId,riotIdGameName,riotIdTagLine,runes,scores,skinID,skinName,
                   summonerName,summonerSpells,team],   ← 19 个键，没有任何 subteam 相关键
  "subteam_field_values": null,       ← 小队字段：不存在
  "team_values": {"ORDER": 18},       ← 18 人全在 ORDER，没有阵营区分
  "position_values": {"OTHER": 18},
  "grouped": false, "group_field": "", "result": "ungrouped",
  "position_matched_count": 17, "position_match_source_counts": {"riotId":17,"summonerName":0}
```

拿到完整 18 人之后，按顺序切 3 人一块的推断**立刻被自己的护栏拦下**：

```
14:23:08.465  arena_group_order_rejected {reason:"allies-cross-blocks", source:"live-client-order", player_count:17}
14:25:46.206  arena_group_order_rejected {reason:"allies-cross-blocks", source:"live-client-order"}   ← Reconnect
14:29:40.895  arena_group_order_rejected {reason:"allies-cross-blocks", source:"live-client-order"}   ← 再次 InProgress
```

`allies-cross-blocks` 的含义（`arena_live_grouping.go:125 arenaAlliesCorroborate`）：
**我们已知的自己那 3 个队友，被这个顺序切进了不同的块**。三个不同时间点、三次独立计算，结论一致。

**这是 R90 核心假设（"名单数组顺序 = 小队顺序"）的直接实证否定。**
护栏运转正常、拦得对；错的是算法前提。**别再在"顺序推断"上加分支了。**

### 附带澄清一个会误导人的埋点

`arena_champ_select_order_check {champ_select_count:18, comparable_count:2, matched_count:0, agreement:0}`
看起来像"顺序完全不一致"的铁证，**其实它几乎是空转的**：
`comparable_count` 只有 2，因为英雄选择阶段只有 3 个人的 championId 可见（见 P0-4），
`rememberArenaChampOrder`（`arena_live_grouping.go:518`）拿到的 18 个 championId 里 15 个是 0。
**n=2 的比对不能证明任何事。这个埋点当前的口径需要一并修（见 P0-6 第 4 条），别拿它当证据用。**

---

## P0-4 ★真正的新发现：**Riot 在英雄选择阶段主动把另外 15 人匿名了，而我们自己那 3 人是 100% 可靠的**

```
14:21:24.321 / 14:22:21.278 / 14:22:39.142   lcu_champ_select_session_shape  queue_id=1750  game_mode=CHERRY
  "my_team_length": 18,                                    ← 18 个人全在
  "my_team_name_visibility_type_counts": {"HIDDEN": 15, "UNHIDDEN": 3},    ← ★
  "my_team_obfuscated_puuid_shapes": {"string_empty": 18},  ← 18 个 obfuscatedPuuid 全是空串
  "my_team_nonzero_counts": {
      "cellId":17, "team":18, "nameVisibilityType":18, "wardSkinId":18,     ← 这几个 18 人都有
      "gameName":3, "puuid":3, "tagLine":3, "summonerId":3,                 ← ★ 只有 3 个人有身份
      "spell1Id":3, "spell2Id":3, "championId":3 },
  "my_team_team_counts": {"1": 18}
```

解读（这是本轮最重要的一段）：

- 18 个 cell 全在，`cellId` 覆盖 0~17（非零计数 17 = 除了 cellId 0 那个人）。
- **只有 3 个人有 `gameName`/`puuid`/`tagLine`/`summonerId`/`championId`——就是你自己和你的两个队友。**
- 另外 15 人 `nameVisibilityType = HIDDEN`，**连 `obfuscatedPuuid` 都是空字符串**（不是脱敏串，是压根没有）。
- 这解释了 `live_roster_shape` 在 ChampSelect 阶段恒为 `players: 3`，
  也解释了用户上一轮反馈的"英雄选择阶段自己小队的人的战绩加载不出来"——
  **本来就只有 3 个人可查，不是加载不出来。**

**结论：Riot 是故意不让你在选人阶段看到另外 5 个小队的。这是产品设计，不是接口缺陷，不会有"换个接口就能拿到"的解法。**

### 而这 3 个人，代码其实早就抓到了，却只拿来当否决条件

`gameplay.go:5856 rememberArenaAllies(rawPlayers)` 在 ChampSelect 阶段把这 3 人存进 `arenaAllyKeys/arenaAllyPlayers`。
然后 `arena_live_grouping.go:243` 只把它传给 `arenaAlliesCorroborate` **当校验器用**——
整个推断失败时，这 3 个人的确定信息**一起被丢掉，界面上什么都不显示**。

**这就是本轮要立刻改的地方：把这份 100% 可靠的信息从"否决条件"提升为"正向分组"。**

---

## P0-5 ★为什么六轮都没收敛：**赛后真值校验在国服永远不会触发，我们从来没验证过任何一次猜测**

`checkArenaGroupTruth`（`arena_live_grouping.go:342`）的签名是：

```go
func (a *app) checkArenaGroupTruth(client *LCUClient, serverID string, info *riotMatchInfo) {
	if info == nil || !isArenaQueue(info.QueueID, info.GameMode) { return }
```

**它只接受 `*riotMatchInfo`，也就是 Riot match-v5 接口的数据——那是韩服/国际服的接口，国服的对局根本不在里面。**

本局是国服 HN10（`sgp_gateway_resolution server_id="HN10"`，5 次，全程一致）。
所以：用户打完了一整局（`endgame_trigger phase=PreEndOfGame` 14:32:32），
**整份日志里 `arena_group_truth_check` 事件数 = 0。**

**而真值其实一直都在手边：**

```
14:28:13 / 14:32:40  sgp_match_history_succeeded  "max_participants": 18, "participant_counts": {"18": 3, ...}
```

国服 SGP 战绩**确实返回 18 人的斗魂对局**，而且代码**早就在解析小队号了**：

- `gameplay.go:600` `PlayerSubteamID int64 \`json:"playerSubteamId"\`` ← SGP/LCU 战绩详情结构体
- `gameplay.go:3090` `SubteamID: raw.Stats.PlayerSubteamID, Placement: raw.Stats.SubteamPlacement`
- `gameplay.go:480` `SubteamID int64 \`json:"subteamId,omitempty"\`` ← 已下发前端

**也就是说：每打完一局斗魂，我们手上都有那一局 18 个人的真实小队号，只是从来没拿它跟自己的猜测比对过。**

**这才是六轮不收敛的真正原因——不是猜得不够聪明，是猜完从来不对答案。**

---

## P0-6 本轮要做的（用户已拍板：先上"我的小队" + 接真值验证）

### 1. ★把"我的小队"作为确定性结果直接展示（本轮必须可见的交付）

- 数据来源：ChampSelect 阶段 `myTeam` 里 `nameVisibilityType == "UNHIDDEN"`（等价判据：`puuid` 非空）的那批人，
  即现有 `rememberArenaAllies` 已经存下的 `arenaAllyPlayers`。
- **展示规则**：这 3 人打上「我的小队」标记；其余 15 人**不分组、不编号、不猜**，
  维持现在的平铺，只在区块顶部给一句说明。
- **阶段覆盖**：ChampSelect（此时只有 3 人可见，就是全部）、`GameStart`、`InProgress`、`Reconnect` 全程保持。
  进游戏后 2999 给到 18 人时，用已存的 puuid/riotId 把这 3 人**重新定位**到新名单里（`arenaAlliesCorroborate`
  里已有的身份匹配逻辑可直接复用，不要另写一套）。
- **绝对不要**因为"整体推断失败"就把这 3 人的标记一起丢掉——这正是现在的行为。
- **文案**：用「我的小队」，**不要用「小队 1」这种编号**（我们不知道真实编号是几），
  更不要用暗影狼这类吉祥物名（`arena_live_grouping.go:271` 那条注释的约束继续有效：
  推断出的块号不是客户端小队号）。其余 15 人处给一句静态说明，例如
  「英雄选择阶段客户端只提供本小队信息，其余小队进入对局后仍不提供小队归属」。

### 2. ★★接上赛后真值校验（本轮最关键的长期投资）

- **让 `checkArenaGroupTruth` 也能吃国服 SGP 的战绩详情。**
  现在它只认 `*riotMatchInfo`；要抽出一个与数据源无关的入参（至少包含
  `GameID / QueueID / GameMode / []{PUUID, PlayerSubteamID}`），
  让 Riot match-v5 和国服 SGP 两条链都能喂进去。`gameplay.go:600/3090` 已经解析好了，不用新抓数据。
- **触发点**：`gameplay_refresh.go:66 finishArenaGroupTruth` 在 `PreEndOfGame/WaitingForStats/EndOfGame`
  已经会触发，但国服路径拿不到 `riotMatchInfo` 就直接空转。改成走 SGP 取当局详情。
- **`arena_group_truth_check` 埋点要扩充**，除现有字段外必须记录：
  - `truth_source`: `"riot-match-v5"` / `"sgp"` / `"none"`（`none` 也要记，否则又变成静默 return）
  - `subteam_id_by_index`: 按我们当时名单顺序排列的真实小队号数组（18 个数字）
  - `playerlist_order_is_squad_order`: 布尔，直接回答 P0-3 那个假设
  - `session_order_is_squad_order`: 同上，针对 gameflow 17 人顺序
  - `my_squad_correct`: 我们标的「我的小队」3 人，与真值里自己所在小队是否完全一致（**这是第 1 项的正确性验收**）

### 3. ★补齐三个从来没采过的关键样本（下一份日志就能给出答案）

这三条都是"只加埋点、不改行为"，风险为零，但能一次性终结"还有没有别的数据源"这个问题：

- **a) `cellId` 与可见性的对应关系（最有价值）。**
  在 `lcu_champ_select_session_shape` 里加 `my_team_unhidden_cell_ids`（我们自己 3 人的 cellId 数组，
  数字不涉及隐私）和 `my_team_cell_id_sequence`（18 个 cellId 的排列）。
  **要验证的假设：小队是不是按 cellId 连续分块（0-2 / 3-5 / 6-8 / 9-11 / 12-14 / 15-17）。**
  如果我们 3 人的 cellId 恰好是 `{3k, 3k+1, 3k+2}`，这个假设就成立，
  再配合第 2 项的真值，就能确定"cellId 块 = 小队"。这是目前最有希望的一条线索，
  而它**从来没被记录过**——六轮里没人查过。
- **b) `/liveclientdata/allgamedata` 从来没试过。**
  `gameplay.go:39` 全仓只用了 `/liveclientdata/playerlist` 一个 2999 端点。
  每局斗魂对 `allgamedata` 采样一次，记录其顶层键与各数组的键集合（**只记键名和值的分布，不记玩家身份**，
  沿用现有 `liveClientPlayerListShape` 的脱敏做法）。
  游戏内 HUD 能显示 6 个小队名，说明客户端某处有这份数据；
  在断言"拿不到"之前，至少要把这个端点看一眼。
- **c) gameflow 17 vs 18：到底少了谁。**
  记录 `session_missing_from_playerlist`（2999 的 18 人里，哪 1 个不在 gameflow 的 17 人里——
  记**索引位置和是否是本人**，不记身份）。
  两轮日志都是 17，这个缺口是系统性的，搞清楚它可能直接打通 session-order 那条路。

### 4. 修掉 `arena_champ_select_order_check` 的空转口径

现在 `comparable_count` 恒为 2（因为选人阶段只有 3 人 championId 可见），
`matched_count: 0` 会被误读成"顺序不一致的证据"。要么在 `comparable_count < squadSize*2` 时
不记这条事件、要么加一个 `conclusive: false` 字段明示"样本不足，不构成结论"。
**不要留一个看起来像证据、其实什么都证明不了的埋点——这正是上一轮把人带偏的原因之一。**

### 5. 明确不做的事（写死，防止下一轮又绕回来）

- **不再做任何"按名单顺序切块"的推断**，无论数据源是 2999 还是 gameflow。已被 P0-3 实证否定。
- **不再把 `teamParticipantId` 当小队号**。已被 P0-2 第三次证伪。
- **不再期待 `GameStart` 能做任何事**。已被 P0-1 证明只有 308 毫秒。
- **不显示吉祥物小队名**，也不显示推断出来的小队编号。只有拿到真实 `playerSubteamId` 才谈得上编号。
- 不改 `arenaAlliesCorroborate` 的否决逻辑（它是对的，继续当护栏用），只是**额外**把它的输入拿来正向展示。

### 6. 验收判据（变异测试）

1. 构造 ChampSelect fixture：18 人 `myTeam`，15 人 `nameVisibilityType=HIDDEN` + 空 `obfuscatedPuuid`，
   3 人完整身份。断言这 3 人被标为「我的小队」，其余 15 人无分组标记。
   **把"提升为正向分组"那段删掉 → 必须变红。**
2. 承接测试：同一 fixture 进入 `InProgress`，2999 返回 18 人完整名单（无 subteam 字段、顺序被打乱到
   使 3 个队友跨块）。断言：(a) 整体推断仍然被 `allies-cross-blocks` 拒绝；
   (b) **但这 3 人的「我的小队」标记依然在**。
   **把"推断失败就清空标记"的行为改回来 → 必须变红。**（这条正是当前的 bug。）
3. 真值校验：喂一份国服 SGP 形状的赛后详情（18 人带 `playerSubteamId`），
   断言 `arena_group_truth_check` 事件被记录且 `truth_source == "sgp"`、`my_squad_correct == true`。
   **把国服分支去掉（退回只认 riotMatchInfo）→ 必须变红。**
4. 反向测试：喂一份"我们标错了"的真值（自己的 3 人分属不同 subteam），
   断言 `my_squad_correct == false` 且事件照常记录。
   防止把校验写成恒真。
5. 埋点完整性：`truth_source == "none"` 的情况也必须落一条事件。
   **把这个分支改成静默 return → 必须变红。**（记忆里的教训：静默 return 是排障杀手。）

### 7. 真机验证清单（这些只能靠用户再打一局）

- ChampSelect 阶段是否真的看到「我的小队」3 人标记。
- 进游戏后（用户眼里的加载页 + 对局中）这 3 人标记是否保持不丢。
- 赛后日志里是否出现 `arena_group_truth_check`，`truth_source` 是不是 `sgp`，`my_squad_correct` 是不是 true。
- 新增的 `my_team_unhidden_cell_ids` 是不是连续的 3 个数、是否 3 对齐。
- `allgamedata` 采样里有没有任何小队相关字段。

---

# P1 ★★ 刷新战绩后「英雄熟练度 / 最近一起玩 / 当前段位」一起消失

## 根因（一句话）

**韩服流式响应的中途 `progress` 帧只算了 `player/matches/pagination` 三个字段，其余字段被 Go 序列化成 `null`；
前端 `web/gameplay.js:769` 用 `{...previous, ...partial}` 无差别整体展开，把已经拿到的数据覆盖成 `null`。
三样数据是同一个根因，不是三个。**

## 代码证据

```go
// riot_api.go:1204  —— partial 帧只填了 3 个字段
partial := gameplayOverview{Player: gameplayPlayer{...}, Pagination: gameplayPagination{...}}
```

```go
// gameplay.go:200-220  —— 有没有 omitempty 决定了一个字段会不会变成 null
Ranks           []gameplayRank           `json:"ranks"`                     // 无 omitempty → null
HistoricalRanks []gameplayHistoricalRank `json:"historicalRanks,omitempty"` // 有 → 整个字段省略
Masteries       []gameplayMastery        `json:"masteries"`                 // 无 → null
RecentPlayers   []gameplayRecentPlayer   `json:"recentPlayers"`             // 无 → null
RankedQueues    map[string]...           `json:"rankedQueues,omitempty"`    // 有 → 省略
```

```js
// web/gameplay.js:769  —— 整体展开，只保护了 matches 和 pagination
tab.data = { ...previous, ...partial, matches, pagination: previous?.pagination || partial.pagination };
```

## 决定性证据：`omitempty` 的分界线与截图完全吻合

这条最硬，因为它预测了一个**纯由 JSON tag 决定、毫无业务含义**的分界，而截图三恰好落在线上：

| 字段 | tag | partial 帧 | 截图三（JUGKlNG）实际 |
|---|---|---|---|
| `ranks` | 无 omitempty | `null` → 被覆盖 | 单排/灵活 **Unranked「尚未定级或客户端未提供」** ✗ |
| `historicalRanks` | **有 omitempty** | 字段不存在 → 保留 | **S2025 大师 320LP / S2024 王者 2084LP 全在** ✓ |
| `rankedQueues` | **有 omitempty** | 字段不存在 → 保留 | 左下「近 11 场排位」仍在 ✓ |
| `masteries` | 无 omitempty | `null` → 被覆盖 | 截图二「客户端未提供熟练度」✗ |
| `recentPlayers` | 无 omitempty | `null` → 被覆盖 | 截图二「暂无重复同场玩家」✗ |

**"当前赛季段位空了但历史赛段还在"这个看起来最诡异的现象，就是一个 `omitempty` 的差别。**
这也直接排除了"段位接口挂了"之类的解释。

## 日志证据

```
14:11:42  overview_phases_ms  spans={account,champion_names,details,mastery,matchIDs,opgg-historical,ranks,summoner}
          mastery:346  ranks:346                 ← begIndex==0 的完整响应，三样齐全
--- 截图1（22:12:24）数据完好 ---
--- 截图2（22:12:37）两块同时变空 ---
14:12:52  overview_load_cost duration_ms:58338
          overview_phases_ms spans={account,champion_names,details,matchIDs,summoner}    ← 没有 mastery/ranks span
14:12:53  overview_load_cost duration_ms:27227   ← 同上
```

`riot_api.go:1164` 是 `markSpan("ranks"/"mastery")` 的**唯一**调用点，整体包在 `if begIndex == 0 {` 里。
所以"没有 mastery/ranks span" ⇒ 这两次都是 `begIndex > 0`（翻页/加载更多）。
用户看到变空的时刻正好夹在两次请求中间，屏幕上的空态**只能来自流式 progress 帧**。

## 为什么是永久消失而不是一闪而过

`begIndex > 0` 两端都不会补回来：
- 后端 `riot_api.go:1313-1319`：`if begIndex > 0 { return response }`，三个字段全是零值
- 前端 append 分支 `web/gameplay.js:804`：只取 `matches` 和 `pagination`，不重新展开 payload
- 收尾 `web/gameplay.js:893` 走 `appendOverviewMatches()`，只往 DOM 追加战绩行，连重绘生涯栏的机会都没有

`begIndex==0` 的强制刷新（`web/gameplay.js:824`）本来能自愈，但**如果该请求在 progress 帧之后失败**
（本次日志里 `limiter_queue_ms:435027`、单请求 58 秒，429 完全可能），
`web/gameplay.js:884` 只写 `tab.initialPageError`，`tab.data` 就停在被 null 掉的状态。

## 要做的

**唯一改动点：`web/gameplay.js:769`，从"黑名单保护"改成"白名单更新"。**

```js
// progress 帧在语义上是增量，它对 ranks/masteries/recentPlayers 没有任何意见，就不该表达意见。
tab.data = previous
  ? { ...previous, player: { ...previous.player, ...partial.player }, matches,
      pagination: previous.pagination || partial.pagination }
  : { ...partial, matches };
```

**★ 明确不要这样改**：不要给 `Ranks/Masteries/RecentPlayers` 加 `omitempty`。
那会让"该玩家确实没有熟练度"和"这次没算"变得不可区分，前端会保留陈旧数据——
把"误清空"换成"误保留"，更难发现。

**可选加固（建议一并做）**：`riot_api.go:1204` 的 progress 帧改用一个独立的
`{player, matches, pagination}` 专用结构体，让"partial 帧携带完整 overview 结构"
在类型层面就不可能发生，防住以后 `gameplayOverview` 新增字段又忘了 omitempty。

**顺手补埋点**：`overview_load_cost` / `overview_phases_ms` 加 `beg_index` 和 `stream` 两个字段。
这次判定 `begIndex>0` 是靠"唯一调用点"反推的，下次应该一眼可判。

## 验收判据（变异测试）

现有测试为什么抓不到：`web/r91-addendum.test.cjs:8` 的 fixture 是
`({player,matches,pagination})`，**根本不含 `masteries/ranks/recentPlayers`**，所以跑绿证明不了任何事。

1. **append 场景**：`tab.data` 先放完整数据（含 ranks/masteries/recentPlayers/historicalRanks），
   再喂一个 `{player, matches, pagination, ranks:null, masteries:null, recentPlayers:null}` 的 progress 帧。
   断言三样都还在。**把 `:769` 改回 `{...previous, ...partial}` → 必须变红。**
2. **刷新中途 429**：progress 帧 → `{type:'error',status:429}`，断言三样仍是刷新前的值。
3. **首屏反向断言**：`tab.data` 为空时收到 progress 帧，断言 `matches.length===5` 且不抛错。
4. ★**防假修复**：fixture 里必须**显式写 `ranks: null`**（不能只是"没有这个 key"），
   并在测试里加注释锁住原因——写成"没有该 key"会让测试假绿。
5. ★**防另一种假修复**：加一条断言"complete 帧里 `masteries:[]`（空数组）必须能把旧值清成空"。
   这条能把"给字段加 omitempty"这个错误修法钉死为红。

**置信度：高**（代码链路 + 日志 span 形状 + 截图 omitempty 分界三方互证）。

---

# P2 ★★ 斗魂对局页每 3 秒抖一下（视频实证）

## 根因（一句话）

**为一个平均只要 10 毫秒的缓存命中请求，渲染了一个高 40px 的"正在刷新…"提示条，
每 3 秒插入 DOM 又立刻移除，导致整页刚性上下跳动 40px。**

## 视频实证（不是推测，是逐帧差分）

视频 2202×1132 @30fps，8.427 秒 251 帧。全帧率灰度差分（阈值 25）只找到三处变化：

```
f031  t=1.033s   f122  t=4.067s   f213  t=7.100s
间隔 3.033s / 3.033s，每次只持续 1~2 帧（33~67ms）
其余 244 帧与首帧像素完全一致（差分 0.00%）
```

把闪烁帧按 dy 平移后匹配正常帧：**最优解 dy = 40px 整**（meanabs 2.53；dy=39 是 5.20，dy=41 是 4.20，dy=0 是 10.49）。
即**整页刚性下移 40px，内容零变化**。

**关键否定证据**：三名玩家的 KDA 小方块、头像、胜率在闪烁帧里**全部完好**。
所以这轮不是内容丢失，是纯布局位移。

## 与日志精确对齐（误差 < 15ms）

```
14:22:39.143  live_load_cost  total_ms=13  players=3  matches_ms=0   gap=3.305s
14:22:42.168  live_load_cost  total_ms=10  players=3  matches_ms=0   gap=3.025s  ← 视频 t=4.067s
14:22:45.195  live_load_cost  total_ms=10  players=3  matches_ms=0   gap=3.027s  ← 视频 t=7.100s
14:22:48.217  live_load_cost  total_ms=10  players=3  matches_ms=0   gap=3.022s
14:22:26.784  live_refresh_client  reason=load  source=interval  phase_changed=false
```

以 `14:22:39.143` 对齐视频 t=1.033s 反推起点 14:22:38.110：
t=4.067s → 预测 14:22:42.177，实测 14:22:42.168（差 9ms）；
t=7.100s → 预测 14:22:45.210，实测 14:22:45.195（差 15ms）。**一一对应。**
`total_ms=8~13` 也直接解释了为什么只闪 1~2 帧。

## 代码链路

```js
web/gameplay.js:118-122   if (normalized === "ChampSelect") return 3_000;      // 3 秒轮询
web/gameplay.js:3774-3778 state.liveLoading = true; renderLive();              // 同步插入提示条
web/gameplay.js:139-145   renderLiveRefreshStatus → <div class="live-refresh-status">正在刷新…</div>
web/gameplay.css:1831-1840 .live-refresh-status { display:flex; gap:8px; margin-bottom:12px; ... }  // ≈40px
web/gameplay.js:4460-4471 status.innerHTML = statusMarkup;                     // 0px→40px，下方整体下移
web/gameplay.js:3823-3828 finally { state.liveLoading=false; renderLive(); scheduleLiveRefresh(); }  // 弹回
```

`grep data-live-status web/*.css` **零命中**——这个槽位没有任何 `min-height` 占位。

**为什么只在选人页闪**（三重叠加，都只在 ChampSelect 成立）：
1. `:120` ChampSelect 固定 3 秒，对局中是 20 秒；
2. `:133` `if (!["InProgress","Reconnect"].includes(data?.phase)) return "";`
   → 选人页空闲态文案恒为空串，所以是 **0px↔40px 全有全无**；对局中文案常驻，只换字不变高；
3. 选人页只有 3 人且全缓存命中，请求 10ms 返回，指示器一闪而过。

## 与 R91-P5 那次修复的关系（明确回答）

**是同一段代码块，但完全是另一条故障路径。P5 真的修好了，而且正因为修好了，
这条抖动才从"被更大的内容闪烁掩盖"变成唯一可见的残留伪影。**

P5 生效的硬证据：视频里 KDA 方块完好（`historyPending` 收窄生效，没回落骨架）；
日志里 `live_roster_rendered`（只在全量重建时埋点）在 14:22:26~48 **一次都没有**，
而 `live_load_cost` 有 8 次 → 走的是快路径，roster DOM 完整保留。

**P5 从设计上就没考虑这条**：它把 status 和 body 拆成两个**在流兄弟块**，
目标是"状态变化不动 roster DOM"。DOM 节点确实没动，**但它们的屏幕位置动了**。
修复者验证的是"节点身份有没有被替换"，不是"用户屏幕上有没有跳"。

对应测试 `web/r91-addendum.test.cjs:96-101` 正好只断言节点身份：
```js
state.liveLoading = true;  h.renderLive(); assert.equal(h.nodes.liveContent.querySelector('article'), row);
```
jsdom 不做布局，`article` 永远是同一个对象 → 测试永远绿，而用户屏幕上它在跳。
**这是记忆里"只验工单是否被执行、不验用户屏幕结果"那个盲区的第四个案例。**

## 要做的（1+2 一起上，3 作兜底）

1. **后台轮询不渲染加载提示，只有手动刷新才渲染。**
   `loadLive(force, source)` 已经带 `source`（`interval`/`sse`/`event`/`manual`/`direct`）。
   新增 `state.liveLoadingVisible`，仅当 `force === true || source === "manual"` 时置真；
   `:142` 和 `:4424` 改读这个标志。
   **注意：`web/r91.test.cjs:104-106` 明确要求手动刷新要有可见反馈（R91-P4 的诉求），这条必须继续绿——
   不能简单删掉提示条。**
2. **加延迟显示阈值（反抖动）**：即使手动刷新也 `setTimeout(200~300ms)` 后仍未结束才显示，
   先结束就 `clearTimeout`。10ms 返回的请求永远不会闪，真正慢的（对局中 13.4 秒）照常显示。
3. **兜底**：`[data-live-status] { min-height: 40px; }` 或把提示条改成浮层脱离文档流。
   单独用第 3 条会在选人页常驻 40px 空白，只适合配合 1、2 做保险。

**顺带**：`:120` 硬编码的 3 秒值得复核——8 次请求 `matches_ms` 全是 0，绝大多数是纯浪费。
但**这不是根因，别把它当修复**。

## 验收判据（变异测试）

| # | 变异 | 应该让谁变红 |
|---|---|---|
| M1 | 把 `force \|\| source==="manual"` 改成 `true` | 新增：`loadLive(false,"interval")` 期间 `.live-refresh-status` 必须为 null |
| M2 | 改成 `false` | `web/r91.test.cjs:104-106` 手动反馈断言必须红（保证没把 P4 改回去） |
| M3 | debounce 阈值改 0 | 新增：假 timer 下 10ms 内 resolve 的请求全程 `.live-refresh-status` 计数为 0 |
| M4 ★ | 删掉 `[data-live-status]{min-height}` | **必须走真 Chromium**：`PerformanceObserver({type:'layout-shift'})` 或对 `[data-live-body]` 连续采 `getBoundingClientRect().top`，断言 3 秒周期内方差为 0 |
| M5 | `:133` 的 `return ""` 改成 `return "x"` | M4 的 CLS 断言应**保持绿**（证明护栏防的是"高度变化"而非"恰好为空串"这个巧合） |

**M4 是本次核心判据。** 只加 jsdom 断言的话 M4 会活下来，等于又修了一遍假护栏。
项目已有 Playwright 真 Chromium 工具链（见 `deep-legends-visual-harness-playwright.md`），直接用。

---

# P3 ★ 本局记录 + 锁定策略时间语义

## A. 本局记录：8 条全部正确，但它们如实暴露了两个真缺陷

**先纠正一个前提：这一局是斗魂（`champselect_trace` 全程 `queue_id=1750, group_id=arena`），不是排位。**
配置（14:19:56.837 `champselect_settings_saved`）：
`ban: {strategy:"lock-now", delayMs:2000}`、`pick: {strategy:"show-then-lock", delayMs:500, lockDelayMs:10000}`。

日志还原的真实动作序列与截图 8 条记录**逐条对上，时间戳误差 < 30ms**，顺序没颠倒、没重复、没缺失
（禁用的 hover 三条和「已排定：2.0 秒后禁用并锁定」在截图裁切线以下，日志里都在）。

**所以记录本身没写错。** 但它暴露了：

### A-1（真缺陷）「立即锁定」的禁用一共等了 4.265 秒

```
14:21:34.257  champselect_evaluation  action_type=ban  queue_id=1750
14:21:34.271  availability  reason=wildcard-session-evidence-hover  requires_hover_confirmation=True
14:21:34.272  schedule armed  write_step=hover  delay_ms=2000          ← 等 2 秒
14:21:36.299  write → 36.391 write-result  http-success                 ← 实际等了 2.027s
14:21:36.427  schedule armed  write_step=lock   delay_ms=2000          ← 又等 2 秒
14:21:38.448  write → 38.519 write-result  http-success                 ← 又等了 2.021s
14:21:38.522  confirmation applied  observed_completed=True
```

`champselect.go:987` `delay := champSelectDelay(config.DelayMS, remaining)` 对**所有** strategy 无条件生效，
`lock-now` 完全不豁免。斗魂 `forceHover` 把一次写拆成两次写，于是 **`DelayMS` 被收了两遍**。

### A-2（真缺陷）「0.5 秒后亮出选用」的 0.5 秒是 UI 永远不显示、用户改不了的隐藏默认值

`web/suite.js:591-592`：
```js
const lockWait = !isBan && (sideConfig.strategy || "show-then-lock") === "show-then-lock";
const delayKey = lockWait ? "lockDelayMs" : "delayMs";
```
选用侧在 `show-then-lock` 下步进器只绑 `lockDelayMs`，**`pick.delayMs` 没有任何入口**，
值来自 `champselect.go:246` 的出厂默认 `DelayMS: 500`，却每局在记录里露一条。
截图印证：选用序列只有「锁定等待 10」一个框。

### A-3（真，次要）正常成功路径被记成黄色 warn

`champselect.go:1279` 把「已发送…等待客户端确认」记成 `"warn"`，
但这是 happy path 的正常中间态（`disposition == "awaiting-confirmation"`）。
截图里这两条确实是琥珀色圆点、其余是绿色。改成 `"ok"`。

### A-4（真，潜在 bug，本局未触发）锁定等待不受回合剩余时间约束

`champSelectHoverLockDelay`（`champselect_takeover.go:25-33`）**不接收也不使用 `remaining`**。
hover 那步走的 `champSelectDelay(config.DelayMS, remaining)` 有剩余时间上限，lock 这步没有。
若轮到自己时只剩 3 秒而锁定等待设了 10 秒，会排定一个必然错过回合的锁定。
本局剩余 43 秒所以没暴露。**要给 lock 这步也套上 `remaining` 夹取。**

### A-5（附带观察，建议顺手处理）PLANNING 阶段的预选每局都会留两条黄字

```
14:21:24.486  schedule armed  write_step=intent  delay_ms=500  champion_id=-3
14:21:27.474  postflight not-applied-after-2s → confirmation retry-after-fresh-state (attempt 1)
14:21:30.475  confirmation attempt-limit (attempt 2)
```
**斗魂客户端根本不接受 PLANNING 期 pick-intent**（`observed_champion_id` 始终 0），
每局都会在记录里留 2 条刺眼的黄字。建议斗魂组跳过 intent 写入，或把 attempt-limit 降级为普通提示。

## B. 锁定策略与时间框（用户已拍板：去掉人为等待，保留两段写入）

目标语义：

| 策略 | 时间框 | 行为 |
|---|---|---|
| 仅亮出 | **不显示** | 只亮出，不锁定，不等待 |
| 亮出后锁定 | **显示**（锁定等待 N 秒） | 先亮出，N 秒后锁定 |
| 立即锁定 | **不显示** | 立刻锁定，不受任何延时影响 |

### 后端（`champselect.go:987-990` 单点改写）

```go
delay := 0
switch {
case intent:
	delay = champSelectDelay(config.DelayMS, remaining)   // 预选沿用现有节奏，不动
case config.Strategy == "lock-now":
	delay = 0                                             // 立即锁定：任何一步都不等
case !completed:
	delay = 0                                             // 亮出这一步不等
default:                                                  // show-then-lock 的锁定这一步
	delay = champSelectDelay(champSelectHoverLockDelay(config, lastSubmission, time.Now()), remaining)
}
```

`default` 分支去掉了 `side == "pick" &&` 限制，让禁用侧的「亮出后锁定」也走 `lockDelayMs`，两侧语义对齐；
同时要把 `champselect.go:311` 的 `side.LockDelayMS = nil` 改成和 pick 一样赋值。
`default` 外层的 `champSelectDelay(..., remaining)` 顺带修掉 A-4。

### 前端（`web/suite.js:591-599`）

```js
const strategy = sideConfig.strategy || "show-then-lock";
const showTime = strategy === "show-then-lock";     // 仅亮出 / 立即锁定 都不渲染
```
把标签 + 步进器整段包进 `${showTime ? ... : ""}`，标签固定「锁定等待」，`kind` 固定传 `"lock"`。

### ★ ForceHover 必须共存，绝对不能绕（这是 R91-ADDENDUM-2 刚修好的）

`forceHover` 与延时是两条**正交**的链路，上面的改法完全不碰它：
- `forceHover` 只影响 `completed` 取值（`champselect.go:980-983`）和写前守卫（`champselect.go:1375`）。
- 改后斗魂 wildcard ban 在「立即锁定」下**仍然是 hover 写 → 客户端回显 → lock 写两段**，
  只是两段之间不再人为空等。**日志实测回显只要 113 毫秒**
  （14:21:36.299 发出 → 36.412 确认），所以总耗时约 0.2 秒，符合"立刻锁上"的体感。
- **绝对不能做**：不要因为"lock-now 就该一次到位"而给 `lock-now` 加 `forceHover` 豁免。
  那会让 `completed=true` 的单次写绕过 `:1375` 守卫，直接踩回 R91-ADDENDUM-2 修掉的兼容性问题。
- 同样**不要动** `champselect_takeover.go:101` 的 `hover-cleared-by-client` 豁免条件
  （它依赖 `last.Decision.ForceHover`），否则 R91 的"斗魂禁用被误判玩家主动切换"会复发。

## 验收判据

- `lock-now + DelayMS=2000` → 断言两条 `schedule armed` 的 `delay_ms` 都是 0。
  **改回 `champSelectDelay(config.DelayMS, remaining)` → 变红。**
  ⚠️ `diagnostics_2351_test.go:128`（`g.Pick.DelayMS, g.Pick.Strategy = 20, "lock-now"`）
  和 `diagnostics_2143_test.go:36` 目前断言的正是旧行为，**会确证性失败，需同步更新基线**。
- `show-only` 亮出必须零延时。
- 前端 `showTime` 恒 true → 新增 jsdom 断言「`show-only`/`lock-now` 下 `[data-cs-time]` 为 null」变红；
  同时 `web/suite.test.cjs:246-259` 必须**仍然绿**（保证 `show-then-lock` 的框没被误删）。
- `champselect.go:311` 的 `LockDelayMS = nil` 改回 → 「ban 侧 show-then-lock 用 lockDelayMs」变红。
  ⚠️ `champselect_takeover_test.go:354`、`r78_test.go:35` 会确证性失败，属预期，同步更新。
- `champselect.go:1279` 的 `"warn"` 改回 → 「已发送记录 kind 必须为 ok」变红。

**真机验证**：斗魂禁用设成「立即锁定」跑一局，确认两条 `schedule armed` 的 `delay_ms` 都是 0，
且 `preflight` 不出现 `compatibility-hover-unconfirmed` 的反复重排。

---

# P4 ★ 职业选手识别与分组（用户已拍板：放开到全部一队 + 二队单独标 + 复用玩家页签"职业"组）

## 关键事实：数据早就在手，只是被丢了

项目每 5 分钟把 OP.GG 韩服职业目录**全量**拉下来并完整解析
（`pro_players.go:304-400`，沙箱实测 HTTP 200、3.77MB、**118 队 / 375 人 / 593 账号，每个账号 100% 带 PUUID**），
然后在 `buildProPlayers`（`pro_players.go:512-568`）按 `proRoster` 白名单把 112 队全部丢掉，
只留 BLG/IG/T1/HLE/GEN/DK 六队 33 人。

链路健康，没有隐藏故障：本次日志两轮完整加载全部成功
（`pro_directory_cost` directory 3772ms/2157ms、supplements 5437ms/4169ms、ladder 10590ms/8998ms，
`pro_supplement_result` 9/1/4 与 `docs/pro-players-sources.md` 记载完全一致）。

## "GEN Canyon" 徽章是怎么来的：点击来源透传，不是名称匹配

```
web/pro-players.js:163-165   点击账号 → dispatch "deep-legends:open-player"
                             detail = {gameName, tagLine, region:"kr", source:"pro-players",
                                       teamCode:"GEN", playerName:"Canyon", expectedTier}
web/gameplay.js:6153-6162    source==="pro-players" → tabContext = {group:"pro", proTeam, proPlayer}
web/gameplay.js:1147-1151    summonerProChip → <span class="pro-identity-chip">GEN Canyon</span>
```

**推论**：直接在搜索框搜同一个账号**不会**出现徽章（`source` 是 `"search"`）。
徽章完全依赖"你是从职业目录点进来的"这个上下文，刷新页面就没了。

## ★「按名称判断」必须否决 —— 有当场可复现的反例

用真实目录逐个核对用户截图里的 8 个 ID：

| 用户看到 | 目录记录 | 真实归属 | 判定 |
|---|---|---|---|
| `JUGKiNG#kr` | `JUGKlNG`（**小写 L**）# `kr` | GEN / Canyon | ✅ 精确命中 |
| `HLE Gumayusi` | `HLE Gumayusi` # `0298` | HLE / Gumayusi | ✅ |
| `NS Calix#KR11` | `NS Calix` # `KR11` | NS Nongshim / Calix（当前被丢弃） | ✅ |
| `WBG Moham…` | `WBG Moham` # `lpl` | **OMG** / Moham | ⚠️ 账号名写 WBG，人已在 OMG |
| `T1 Eclipse#20…` | `T1 Eclipse` # `2009` | T1 Esports Academy / Eclipse | ⚠️ 二队 |
| **`Faker#구라티`** | `Faker` # `구라티` | **T1 二队打野 Guti（문정환）** | 🔴 **不是 Faker！** |
| `BRO IDEA#2006` | 查无此人 | — | ❌ 目录缺口 |
| `DK Despair…` | 查无此人 | — | ❌ 目录缺口 |

**Faker 本人在目录里只有一个账号：`Hide on bush#KR1`。**
任何"名字里含 Faker → 标成 Faker"的模糊匹配，**在用户这张截图上当场就把 Guti 错标成 Faker**。
这不是理论风险，是已发生、可复现的假阳性。

同类陷阱（同一份真实数据）：`Gen G Namgung#KR1` 实为 **BRION** 的 Namgung；
`C9 Thanatos#KR2` 实为 **DK Challengers**；`DWG KIA#KR1`/`MIDKING#asd` 都是 DK ShowMaker。
**战队前缀完全不可信**——既有旧前缀不更新，也有蹭别队前缀的，
所以"要求带战队前缀"这个降误判思路是**无效的**（还会漏掉 `Hide on bush`、`팽도리` 这类没前缀的真主号）。

另外 `pro_roster.go:6` 和 `docs/pro-players-sources.md:16` 有一条既有设计红线：
`Never infer membership from an account's name.` —— 这条红线是对的，要保留，
只是措辞要更新成"允许按**精确 Riot ID / PUUID** 反查，仍禁止模糊名称推断"。

## 要做的

### 必须遵守的硬约束

1. **判据只用精确 `gameName#tagLine`（大小写不敏感）+ PUUID 双键，PUUID 优先。绝不做模糊/包含/前缀匹配。**
   实测 593 个账号里跨选手 Riot ID 冲突只有 1 条（且两条记录指向同一个人），
   精确匹配的冒名顶替假阳性为 0。
2. **PUUID 绝不下发渲染层**，索引和匹配全在后端完成，出参只有 `{playerName, teamCode, teamName, secondary}`。
   `pro_players_test.go:105-111` 已有隐私断言守着，带 PUUID 会红，而且**红得对**。
3. **只读现有内存快照，不触发新的目录加载**（`pro_players.go:126-133` 的读法），命中不了就静默不标。
   整条目录链要 9~11 秒，**绝不能让对局页去触发它**，否则重演 R91 的韩服额度打满。
4. **韩服专属**：目录是 `?region=kr`，账号全是韩服 Riot ID，**国服玩家永远不可能命中**。
   国服页面上不显示任何职业标识，也不显示"未识别"之类提示——这是数据源决定的硬边界，不是 bug。
5. **尊重 `maskNames` 设置**（`web/gameplay.js:5671-5673`）：隐藏玩家名时职业徽章必须一并隐藏，
   否则等于绕过打码泄露身份。
6. **补埋点** `pro_identity_match`：`{surface, region, candidates, matched_by:"puuid|riotid|none", secondary}`。
   `none` 也要记——记忆里的教训，别再留零埋点的静默分支。

### 覆盖范围（已拍板）

放开到**全部一队**（292 人 / 493 账号），二队/学院队**识别但视觉降级**
（灰色徽章或加"二队"后缀，如 `T1 Academy Eclipse`）。
复用 `proSecondaryTeam`（`pro_players.go:424-436`）**给二队打标，而不是当过滤器**。
`web/pro-players.js:13` 的六队硬编码和 `pro_roster.go` 两处都要同步放开。

⚠️ **必须如实告知用户的缺口**：`BRO IDEA` 和 `DK Despair` 在目录里确实查无此人，
**任何方案都认不出来**。不要为了"看起来覆盖全"而放宽匹配。

### 落点

- **后端索引**：从 `a.proPlayers.teams` 快照建 `map[riotIDKey]proIdentity` 和 `map[puuid]proIdentity`。
  `pro_players.go:685-691 proAccountKeys` 已有 `"puuid:"` / `"name:"` 两类键，格式直接复用。
- **下发**：`gameplay.go:438-456 gameplayParticipant` 和 `gameplay.go:5449-5475 gameplayLivePlayer`
  各加一个 `ProPlayer *proIdentityBadge`。
- **"职业分组"（已拍板：复用玩家页签）**：`web/gameplay.js:282`
  `const PLAYER_GROUPS = { players: "国服", kr: "韩服", pro: "职业" }` 已存在。
  改 `web/gameplay.js:6153-6162` 的 `open-player` 处理：**后端已判定该玩家是职业选手时，
  即便 `source !== "pro-players"` 也走 `{group:"pro", proTeam, proPlayer}`。**
  这样"从对局里点开 HLE Gumayusi → 自动进职业组 + 自动带徽章"一次打通，徽章渲染零改动。
- **"英雄横条"展示选手名**：插入点是 `web/gameplay.js:4550` 的 `live-player-identity` 内、
  紧跟 `${premadeTag}` 之后。`.self-chip`（`gameplay.css:1007`）和 `.premade-team-tag`（`:1008`）
  都是 `flex:0 0 auto`，**已经是"在这行追加不压缩小标签"的既有范式，照抄即可**。
  ⚠️ **只往 flex 行里塞新 span，不要动 grid 模板**：
  `web/champions.test.cjs:289` 锁了 `.live-player dl` 的 `repeat(3,minmax(60px,1fr))`、
  `:272-286` 锁了 `.live-teams` 的多档 `grid-template-columns`，改列宽会红。
  （`.match-main`/`.match-stats` 那套 R88 保护规则本改动不碰，不会触发。）

### 验收判据

1. 精确匹配：`Faker#구라티` 必须标成 **Guti / T1 Academy**，**不得**标成 Faker。
   **把匹配改成"gameName 包含"→ 必须变红。**（这条是本项的核心护栏。）
2. `Hide on bush#KR1` 必须标成 Faker / T1。
3. 普通玩家把 gameName 改成 `Faker` 但 tagLine 不同 → 必须不标。
4. 国服玩家一律不标，且不产生任何提示。
5. 出参 JSON 中不得出现 `puuid`（沿用 `pro_players_test.go:105-111` 的断言形状）。
6. `maskNames` 开启时徽章必须消失。
7. 内存快照为空时不得触发目录加载（断言 HTTP 调用次数为 0）。

### 需要真机/后续确认

- 对局内玩家的 PUUID 在到达渲染前的哪一层还持有：结构体里有
  （`gameplay.go:5598`、`gameplayReference:263-278`），但 KR 走 Riot API、国服走 LCU，**两条链需真机跟一遍**。
- 加第三个徽章后最窄断点会不会把名字挤成省略号：**必须真 Chromium 截图验**
  （`.live-player` 中间列最小只有 100px，同时有「自己」+「预组×3」+「HLE Gumayusi」是极端行）。
- ⚠️ **本次日志里 23 条 `overview_current_game` 全部 `state:"none"`、`roster_counts:[]`，
  说明这次会话根本没有活跃的韩服对局**——用户截图里那些职业 ID 来自**战绩/对局详情**，不是当前对局卡。
  实现时以战绩/对局详情为主战场。

---

# P5 「最近一次」动作名汉化

## 根因

`web/suite.js:347-356 latestWatchAction()` 只在 `watchDefinitions`（`web/suite.js:254-264`，9 条 watch 规则）
里查中文标题，**4 个 `champselect-*` 动作不在表里**，于是 `|| latest.action` 直接回落成原始键。

`champselect-*` 能进 `state.watchEvents` 是因为 `web/suite.js:1155` 的写入**早于** 1156 行的提前 return。

日志佐证：`14:22:12.983 watch_action {"action":"champselect-pick","result":"fired",...}`
正是截图六显示的那一次；此后到截图时刻没有新的 fired，所以选中它是对的，问题只在没汉化。

## 动作键全集与建议中文

已有中文（`watchDefinitions`）：`accept` 自动接受对局 / `reconnect` 断线自动重连 / `play-again` 快速下一把 /
`auto-honor` 结算自动点赞 / `skip-celebration` 跳过任务庆祝 / `position-broadcast` 阵营位置播报 /
`promote-leader` 房主自动转交 / `invitations` 房间邀请处理 / `auto-matchmaking` 自动开始匹配。

**缺的 4 个**（`champselect.go:17-20` 常量，`champselect_execution.go:162` / `champselect.go:1489,1551` 发出）：

| 键 | 建议中文 |
|---|---|
| `champselect-ban` | 自动禁用 |
| `champselect-pick` | 自动选用 |
| `champselect-bench` | 备战席换英雄 |
| `champselect-trade` | 英雄交换 |

现有可复用映射都不够用：`champselect_takeover.go:62` 那张表只有 `ban/pick/bench` 三项、键不带前缀、且在 Go 侧。

## 要做的

```js
const watchActionNames = {
  "champselect-ban": "自动禁用", "champselect-pick": "自动选用",
  "champselect-bench": "备战席换英雄", "champselect-trade": "英雄交换",
};
function watchActionTitle(action) {
  return watchDefinitions.find((d) => d.action === action)?.title || watchActionNames[action] || "自动规则";
}
```
`web/suite.js:354` 和 `:1167` 都改用它。
**兜底不能再是 `latest.action` 原始键**，否则将来新增动作又会漏英文出来。

**顺带两个建议（可选，建议一起做）**：
1. **「刚刚」是硬编码的**（`:355`），不论过去多久都写"刚刚"。
   截图六拍摄于 22:34:53，而那次 fired 是 22:22:12，**已过 12 分钟仍显示"刚刚"**。改成按 `latest.event.start` 算相对时间。
2. **计数与"最近一次"口径不一致**：`state.watchFired`（「本次启动已代劳 N 次」）因 `:1156` 提前 return
   而**不统计** champselect 动作，但「最近一次」统计。截图六就出现了"最近一次 = champselect-pick"但计数没涨的割裂。
   把 `if (kind === "fired") state.watchFired += 1;` 移到 `:1156` 之前。

## 验收判据

- 删掉 `watchActionNames["champselect-pick"]` → 新增断言
  「`latestWatchAction()` 返回值不得匹配 `/champselect|[a-z]+-[a-z]+/`」变红。
- 兜底改回 `|| latest.action` → 用虚构键 `"champselect-foo"` 驱动，同一断言变红。
- `watchFired` 自增移回 `:1156` 之后 → 「champselect fired 必须计入 watchFired」变红。
- 现状：`web/suite.test.cjs` 里 `latestWatchAction`/`最近一次` **grep 零命中**，这三条全是新增护栏。

**待确认（产品用语）**：`champselect-bench` 叫「备战席换英雄」还是「自动换人」、
`champselect-trade` 叫「英雄交换」还是「处理换英雄请求」——UI 里两种说法都出现过（`web/suite.js:620-624`），
执行时按上表落地即可，若用户另有偏好再改。

---

# 全局验收要求

1. **全量 `go test -count=1 .` 零失败**（贴完整输出，不只贴结论）。
   ⚠️ 本轮 P3 会让 `diagnostics_2351_test.go:128`、`diagnostics_2143_test.go:36`、
   `champselect_takeover_test.go:354`、`r78_test.go:35` 确证性失败，**这是预期的基线更新，不是回归**，
   但必须在账本里逐条写清楚"哪条测试因为哪个故意改动而更新了断言"。
2. **`node --test web/*.test.cjs desktop/*.test.cjs` 零失败。**
3. **P2 的 M4 必须走真 Chromium 布局断言**，jsdom 断言不算数。
4. 每个 P 项的变异判据逐条真跑，贴 kill/survive 结果。**编译失败、超时不计入 kill。**
5. **环境提醒**（本轮踩过）：`TMPDIR`/`GOTMPDIR` 绝不能指向 `/sessions/.../mnt/` 的挂载路径
   （会导致大量无关测试假失败）；`GOROOT` 不能指向仓库内的 `.gotoolchain`
   （会让 `go/importer` 把它误判成本模块子包）。

# 本轮明确不做的事

- 不再做任何斗魂"按名单顺序切块"的推断，也不显示推断出的小队编号或吉祥物名。
- 不给 `Ranks/Masteries/RecentPlayers` 加 `omitempty`（P1 里说明了为什么这是假修复）。
- 不给 `lock-now` 加 `forceHover` 豁免，不动 `hover-cleared-by-client` 的判据。
- 不改本地限流数值（15/秒、90/2 分钟）。
- 不做职业选手的模糊/前缀名称匹配。
- 不因为"选人页 3 秒轮询浪费"就顺手改轮询间隔并声称修好了闪烁（那不是根因）。
