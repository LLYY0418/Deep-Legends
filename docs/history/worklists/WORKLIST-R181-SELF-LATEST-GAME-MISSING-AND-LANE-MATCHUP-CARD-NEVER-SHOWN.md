# WORKLIST-R181：本人名单缺刚打完的一局 + 对位克制卡片整局不出现

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.45（R180 之后，指纹 97a032833e73）。

## 证据范围

- 日志 `lol-loot-diagnostics-1001-2237.jsonl`。0.12.45 的会话 run `ebfb18ec…`，14:27:27–14:37:12Z。
- 上一局 9010029782（queue 440）13:59:04 开始，**14:30:19 结束**。下一局 9010093232（queue 440）14:32:17 进选人，14:34:50 进对局。
- 用户截图：9010093232 选人阶段详情页。本人一行显示的 10 局里没有刚打完的 9010029782，其他玩家正常、无抖动、绝活哥出门装稳定。

R179 / R180 的其他项本次真机表现正常，**不要改动**。

---

## P1　本人名单缺刚打完的同模式对局

### 证据

`live_history_freshness`，14:32:20（进选人 3 秒后，上一局结束约 2 分钟）：

| 玩家 | source | lcu 当前队列最新 | sgp 当前队列最新 | missing_newer_in_queue | 最终显示最新 |
|---|---|---|---|---|---|
| 本人（team 100 slot 0） | `lcu` | 77.16 分钟前 | （未请求） | — | **77.16 分钟前** |
| 队友（team 100 slot 2，与本人组队打了上一局） | `lcu+sgp` | 77.16 分钟前 | **33.36 分钟前** | **1** | 33.36 分钟前 |

- 77.16 分钟前 ≈ 13:15，是再上一局 9009919842；33.36 分钟前 ≈ 13:59，正是刚结束的 9010029782。
- 也就是说：上一局结束 2 分钟后，**本机客户端的战绩接口（本人和队友都是）都还没有这局**，而 SGP 已经有了。
- 队友走了 R180 的 LCU+SGP 合并，补上了；**本人按 R180 的设计只读 `current-summoner`，没有 SGP 合并**，所以缺了这一局。R180 工单里"本人数据是新的"这个前提只在上一局结束较久时成立（当时的证据是 22 分钟后），在"刚打完就排进下一局"这种最常见的情况下不成立。

源码：`gameplay.go:8281` `loadLivePlayerMatches` 中 `if isCurrent { loadLiveLCUMatches(...) ; return }`，本人直接返回，不走合并。

另外，诊断只在每局每名玩家**第一次**加载时记一条（`recordLiveHistoryFreshness` 用 `scope.seen` 去重）。之后 14:32:28、14:33:28、14:33:58、14:34:58 本人的 `current-summoner/matches` 又读了 4 次，但这些读取有没有拿到上一局，日志里看不出来。

### 修复

P1-1　**本人也合并 SGP。** `loadLivePlayerMatches` 去掉 `isCurrent` 的提前返回，本人和其他玩家一样：并行读 LCU（本人仍用 `current-summoner` 路径）+ SGP SUMMARY 30 场（`useCache=false`，3 秒期限），按 `GameID` 合并去重，再交给原 `recentMatchesForPlayer`。

- 本人的 SGP 主体用本人 puuid，执行时核对 `playerRef` 在本人分支里是否就是 puuid；不是的话从当前召唤师取。
- 筛选规则不变：当前队列、胜负局、最多 10 场。
- SGP 失败、不可用：与现在一致，只用 LCU。
- 请求量：每局多 1 次 SGP SUMMARY（同样受 45 秒名单缓存和同玩家合并保护）。
- R180 账本里记的是"本人不发 SGP"（当时按用户选择），**本工单以新的真机证据取代这一条**。R180 测试 5（本人不发 SGP）改为本人也合并，账本写明原因。

P1-2　**诊断能看到后续加载有没有补上。** `live_history_freshness` 不再"每局每人只记一次"，改为：同一局同一玩家，**最终显示的最新一局变了**（内部比较 GameID，日志里不写 ID）就再记一条，并加 `load_index`（这名玩家本局第几次记录，从 1 开始）。显示没变就不记。每局每人最多 6 条。

P1-3　**本人加一个直接字段：上一局有没有显示出来。** 本次运行中观察到的上一局（gameflow 里上一个 `InProgress` 的 gameId，且它的队列与当前队列相同）：

- `prev_game_in_lcu` / `prev_game_in_sgp` / `prev_game_shown`：三个布尔值。
- 本次运行没看到上一局，或上一局队列不同时，这三个字段不写。
- 只记布尔值，不记 gameId。

### 测试

Go：

1. 复现本局：本人 LCU 缺上一局，SGP 有 → 最终 10 局第一张是上一局；`source=lcu+sgp`、`missing_newer_in_queue=1`、`prev_game_shown=true`。
2. 本人 SGP 失败 → 结果与只用 LCU 一致，`sample_failed=true`。
3. 两边都有同一局 → 只出现一次，胜负 / KDA 不重复计数（本人分支）。
4. 同局第一次加载缺上一局、45 秒后再次加载补上 → 共两条 `live_history_freshness`，`load_index` 为 1 和 2；第三次加载显示不变 → 不再记录。
5. 每局每人最多 6 条。
6. 上一局与当前局队列不同 → 不写 `prev_game_*` 字段。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 恢复 `isCurrent` 提前返回 | 测试 1 FAIL |
| 恢复"每局每人只记一次" | 测试 4 FAIL |

---

## P2　对位克制卡片整局不出现

### 证据

本局所有 `lane_matchup_candidate_fetch`（本人上路，己方位置 5 个都已知）：

| 时间 | 敌方已锁定 | 敌方位置已知 | 对位英雄 | reason |
|---|---|---|---|---|
| 14:32:20 | 0 | 0 | — | context-unavailable |
| 14:33:10 | 2 | 0 | — | context-unavailable |
| 14:33:39 | 3 | 0 | — | context-unavailable |
| 14:33:44 | 4 | 0 | — | context-unavailable |
| 14:34:17 | 5 | 0 | **266（推断为上路）** | **own-champion-selected** |
| 14:34:58 | 5 | 5 | — | 对局开始，卡片按设计消失 |

- 本人英雄 799 在 14:34:02 前后已确定（推荐请求 `799:top`）。规划阶段就有预选（`champselect_evaluation` 14:32:16 `pick_intent=true`）。
- 敌方上路是**最后一个锁定**的，14:34:17 才推断出 266。这时本人早已有英雄，所以卡片只能走 R167 的**情况 A**（我的英雄 vs 对面）。

### 根因

1. **情况 A 只在对面英雄刚好落在"前 5 克制 / 前 5 被克"里时才显示。** `champions_structured.go:1731–1738` `structuredCountersForChampion` 从全部对位里只留胜率最低的 5 个（`WeakAgainst`）和最高的 5 个（`StrongAgainst`）。本局本人英雄的对位有 **61 条**（`counters_shape in=61 out=61`），前端 `renderLaneMatchupCard` 只在这 10 条里找 266，找不到就 `return ""`。大多数对局对面都不在这 10 条里，所以卡片几乎总是空的。
2. **情况 B 实际上几乎不会出现。** `laneMatchupOwnChampionId` 把规划阶段的预选（`championPickIntent`）和悬停也当成"已选英雄"。排位里绝大多数玩家在规划阶段就会预选，所以从选人一开始就跳过了 B（R167 写的是"锁定 / 预览后让位给 A"，A 又因为第 1 点经常是空的，结果两边都没有）。
3. **诊断不足以区分原因。** `context-unavailable` 同时代表"敌方分路数据还在请求""请求失败""敌方上路还没锁定"三种情况；进入 A 之后也没有任何记录说明卡片是显示了还是因为没数据被隐藏。14:33:44 的 4 人已锁定、位置已知 0，现有日志无法判断是哪一种。

### 修复

P2-1　**情况 A 用完整对位数据。** 新增 `GET /api/gameplay/lane-matchup?champion=<本人英雄>&enemy=<对位英雄>&position=<本人分路>&tier=<档位>`：

- 复用 `loadStructuredCounters` 已缓存的 OP.GG 详情数据（`fetchWithMetadataCacheKey` 同一个 cacheKey），**不新增上游请求**。从完整对位行里（经过现有 `dropped_*` 过滤后的全部 rows，不截前 5）找 `enemy`，返回 `{championId, enemyChampionId, winRate, games}`；找不到返回空结果（200 + 空对象），不当作错误。
- `tier` 用本人推荐请求同一个档位（本局推荐用的是 `emerald_plus`，而现在对位候选用的是本人段位 `gold`，两者不一致）。执行时核对推荐接口实际用的 tier 来源，统一成同一个。
- 前端：本人英雄（预选、悬停或锁定）和对位英雄都确定时，按 `本人英雄:对位英雄:分路:tier` 请求一次并缓存到本局选人结束；本人换英雄或对位变化时按新键再请求。不得在每次 3 秒轮询都发。
- 显示：`winRate > 50` 显示"这局对线偏优势，胜率约 X%"；`< 50` 显示"偏劣势"；等于 50 显示"这局对线五五开，胜率约 50%"。沿用现有卡片样式和文案格式，不新增说明文字。
- 原 `Hero.StrongAgainst / WeakAgainst` 前 5 的用法保留给其他已有展示，不删字段。

P2-2　**本人还没锁定时，同时显示候选克制英雄（情况 B）。** "本人已选"的判断分两级：

- 本人**已锁定**（`championLocked === true`）：只显示情况 A。
- 本人只是**预选 / 悬停**：显示情况 A（预选英雄 vs 对面）**和**情况 B（打对面有优势的候选英雄），共用一个对位标题，不重复显示对面头像。
- 对面上路还没确定：整块不显示（不变）。

P2-3　**诊断。** `lane_matchup_candidate_fetch` 的 reason 拆细：

- `lanes-pending`：敌方分路数据还在请求。
- `lanes-failed`：分路数据请求失败。
- `lane-not-locked`：分路数据都有了，但没有敌方英雄够得上本人分路的门槛（对面这路还没锁定）。
- `lane-ambiguous`：保留。
- `context-unavailable`：只用于本人分路未知、非选人阶段等其余情况。

新增 `lane_matchup_card` 事件，每局每种结果只记一次：`mode`（`a` / `b` / `a+b`）、`shown`（true / false）、`hidden_reason`（`pair-no-data` / `pair-pending` / `pair-failed` / `candidates-empty`），以及 `own_locked`、`enemy_champion_id`、`tier`。不记身份。

### 测试

Node：

1. 本局复现：本人 799 已锁定，敌方 266 最后锁定且不在前 5 克制 / 被克里，完整对位数据里有 266 → 卡片显示 A，`lane_matchup_card mode=a shown=true`。
2. 完整对位数据里也没有 266 → 卡片不显示，`hidden_reason=pair-no-data`。
3. 本人只有预选、对位已知 → 同时显示 A 和 B，对面头像只出现一次；锁定后 B 消失、A 保留。
4. 多次轮询、对位与本人英雄不变 → `lane-matchup` 只请求一次；本人换英雄 → 按新键再请求一次。
5. 分路数据请求中 / 失败 / 都有但没人够门槛 → reason 分别是 `lanes-pending` / `lanes-failed` / `lane-not-locked`。

Go：

6. `lane-matchup` 从完整 61 行里找到不在前 5 的对位并返回正确胜率和场数；不存在的对位返回空结果，HTTP 200。
7. 先请求过同英雄同分路同档位的推荐后，`lane-matchup` 不触发新的上游请求（断言上游计数不变）。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| A 改回只在前 5 里找 | 测试 1 FAIL |
| 预选也视为已锁定（隐藏 B） | 测试 3 FAIL |
| 去掉请求去重 | 测试 4 FAIL |
| `lane-matchup` 绕过缓存直接请求上游 | 测试 7 FAIL |

---

## 收尾

- 版本递增（0.12.46）；`docs/WORKLIST-INDEX.md` 加 R181；R180 那行备注"本人不合并 SGP 由 R181 取代"；账本 `docs/history/ledgers/r181-execution-ledger.md`。
- `node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 界面红线不变：不加说明文字 / tooltip / 口径说明。

## 真机验收（用户）

1. 打完一局后**马上**排同一模式的下一局：选人阶段本人第一张战绩卡就是刚打完的那一局。
2. 选人时预选英雄、对面同路锁定后：详情页顶部出现"对位克制建议"，显示"这局对线偏优势/劣势，胜率约 X%"；还没锁定自己英雄时，同时出现几个候选克制英雄。锁定后候选消失，胜率那一条保留。
3. 导出新日志（确认版本 0.12.46，导出前不要关软件）：本人的 `live_history_freshness` 带 `prev_game_shown`；有 `lane_matchup_card` 事件。
