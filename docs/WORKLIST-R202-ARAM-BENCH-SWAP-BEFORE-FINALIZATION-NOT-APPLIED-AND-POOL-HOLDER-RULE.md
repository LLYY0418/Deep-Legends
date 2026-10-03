# WORKLIST-R202：海斗开局选了卡片后，备战席出现选用序列英雄却没换过去；对局页战绩改为当前模式最新 10 局

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-03。基线：R201 之后（R201 未执行时以 0.12.63 为准；与 R201 P3 的临时测试选人配合，见 P2）。

## 用户需求

开局（卡片阶段）一定要先选一个英雄。选完后，如果备战席出现了征召选用序列里的英雄，应该换一次过去。如果开局选到的本来就是序列里的英雄，就不要再换。一切以征召里设置的序列为主。

用户截图里卡片下方显示"你主动换成别的英雄后，本轮停止自动选用和换人，下一局恢复"，用户以为是自己开局选了一次卡片，被当成了"手动换人"。

## 证据

日志 `lol-loot-diagnostics-1003-2044.jsonl`，0.12.63，12:16:56 进入选人，queue 2400，选用序列 `[22, 136, 48, 57, 13]`。

| 时间 (UTC) | 事件 |
|---|---|
| 12:16:57 | 卡片 `[43, 76, 51]`，`reason=no-pool-champion`，软件不选（R200 设计） |
| 12:17:01 | 用户自己选了 76；备战席 `[3, 35, 51, 43, 233, 57]` 出现 **57（扭曲树精，序列第 4）** → `bench-gate selected champion_id=57` |
| 12:17:02 | 换 57：HTTP 成功，`timer_phase=BAN_PICK` |
| 12:17:05 | `bench-postflight not-applied`：自己仍是 76 → 第 2 次换 57，HTTP 成功 |
| 12:17:08 | 进入 **FINALIZATION**；第 2 次 `not-applied`，57 用完 2 次机会 |
| 12:17:08～38 | 57 一直在备战席，但 `bench-gate no-preferred-target`：**软件不再尝试** |

结论：

1. **不是"手动选择"导致的。** 用户开局选 76 之后，软件仍然判定要换 57，说明手动接管规则没有生效。截图里那句是卡片下方一直显示的固定说明，不是这局触发的提示。
2. **真正的原因：其他人还在选卡的 BAN_PICK 阶段，备战席交换请求返回成功，但客户端不执行。** 两次都在 BAN_PICK 里发、都没生效，R200 P3 的"最多 2 次"在 FINALIZATION 开始时刚好用完。到了真正能换的 FINALIZATION，57 已被标记为放弃，就再也没换。

## P1　卡片模式下，备战席交换等到可以换的时候再发

1. 卡片模式（`allowSubsetChampionPicks=true`）下，`timer_phase=BAN_PICK`（还有人在选卡）时，**不发**备战席交换请求，记 `bench-gate reason=waiting-finalization`（带 `bench_ids`、`current_champion_id`、目标英雄）。
2. 进入 `FINALIZATION` 后立即按序列判断并交换，沿用 R200 P3 的 postflight 确认和每个目标 2 次的上限。
3. 在 BAN_PICK 里如果已经发过请求、没生效（例如本次日志这种情况），这些次数**不计入**上限；到 FINALIZATION 重新计数。
4. 非卡片模式（普通大乱斗的备战席）保持现状。普通大乱斗如果也出现"BAN_PICK 发出、未生效"，同样按第 3 条处理：未生效的次数不计入，进入 FINALIZATION 后再试。

## P2　卡片模式下，开局自己选的卡不算"手动换人"

1. 卡片阶段的第一次选择，无论是用户手动点的，还是软件选的（R200 按序列选，或 R201 P3 的临时测试选第一张），都**不触发**"你主动换成别的英雄后，本轮停止自动选用和换人"。
2. 只有在已经拿到序列英雄之后，用户又主动换成了别的英雄（从备战席换走，或用重随），才按现有规则停止本轮自动换人。
3. 加单测锁定这条规则（现在的日志说明它没有误触发，但没有测试保护）。

## P3　手上已经是序列英雄时，按「优先选用序列靠前的英雄」开关决定

用户要求"开局选到序列里的就不要换"。征召页现有开关「优先选用序列靠前的英雄」（说明："手上已有序列英雄时，只换取排序更靠前的目标"）：

- **开着**：只换成排序更靠前的英雄（现有逻辑，不变）。
- **关着**：手上已经是序列英雄时**不换**。

`champselect.go` `champSelectBenchTarget` 在开关关着、`currentRank >= 0` 时，现在会返回备战席里第一个序列英雄，可能把第 2 位换成第 4 位。改为：开关关着且手上已是序列英雄时返回 0（不换）。

手上不是序列英雄时（本次日志的 76）：不管开关，都换成备战席里排序最靠前的序列英雄。

## P4　镜头诊断字段算错

同一份日志里，12:16:56 进选人时 `game_camera_mode_apply file_result=ok`：`PersistedSettings.json` 2 → 0，`game.cfg` 已是 0。12:18:38 的 `in_game_60s` 快照里两个文件都是 0，但 `camera_mode_matches_target=false`。

查清这个字段的比较对象（猜测是拿客户端设置里不存在的值，或者拿字符串和 `free` 比），改为：只比 `PersistedSettings.json` 的 `Game.cfg.General.CameraMode` 和目标数值，相等即 true。加单测。

## P5　对局页的近期战绩：要的是"当前模式最新 10 局"，不是"最新 30 局里有几局当前模式"

### 现象

用户截图：对局页我方 5 人，标题"当前模式最新战绩"。自己「近 2 局」、者孙行「近 2 局」、无聊玩玩「近 6 局」，只有两名预组队玩家是「近 10 局」。用户要的是每个人都显示当前模式最新的 10 局。

### 根因（源码已核对）

1. `gameplay.go:8352 loadLiveLCUMatches`（注释"Keep a mixed-queue window of thirty"）和 `live_history_freshness.go loadLiveSGPMatches`（`matchHistory(..., 0, 30, ...)`）都只取**不分模式的最新 30 局**。
2. `gameplay.go:7293 liveRecentPlayerStats` 再用 `recentMatchesForPlayer(matches, playerRef, 10, queueID)` 从这 30 局里挑当前队列，最多 10 局。最近 30 局里当前模式只有 2 局的人，就只显示 2 局。
3. **现在没有"一个月内"的限制**，用的是"最近 30 局"这个场次窗口。
4. 仓库里已经有按队列让服务器筛选的读取：`loadSGPMatchHistoryPage(..., matchFilter)` → `matchHistoryFilteredOn`（SGP `tag=q_<queueId>`，有 `queueFilterCapability` 记录哪些筛选可用，`sgp_match_filter_resolved` 诊断）。排位样本 `recent_ranked_sample_resolved source=sgp-tag` 已经在用，对局页没用。

### 修改

1. **目标**：每个玩家显示当前队列（同一个 ModeGroup 内按现有口径，如单双排只算 420）最新的 **10 局**，且只算**最近 30 天内**的对局（按对局开始时间）。30 天内不足 10 局就有几局显示几局；标题"近 N 局"照实显示。
2. **优先用服务器按队列筛选**：对局页取战绩改用 `loadSGPMatchHistoryPage`，`matchFilter` 由当前队列映射（复用现有 `matchHistoryFilterFor` 的分组）。一次请求 10 局；筛选可用（`ServerFiltered=true`）时直接用，再去掉 30 天以前的。
3. **筛选不可用时的回退**（某些模式没有可验证的单一 tag，或该服务器不支持）：按现在的方式分页读取不分模式的战绩，每页 30 局，在本地挑当前队列。满足以下任一条件就停：已凑够 10 局；这一页最早的一局已经早于 30 天；已读 **4 页**（120 局）。4 页是上限，防止对局页加载过慢。
4. LCU 战绩那一路照现有逻辑合并、去重（`mergeLivePlayerMatches`），只是也按第 1 条的规则挑选。
5. 本人和其他玩家用同一套规则。R180/R181 的"自己最新一局要出现"规则不变。
6. 缓存键加上队列 ID，避免同一个玩家在不同模式的对局之间串用。
7. 性能：10 个人并发读取的现有上限、超时不变。回退分页按需进行，第一页凑够就不再读下一页。
8. 诊断 `live_history_freshness` 增加 `queue_filtered`（布尔）、`pages_read`、`window_days=30`、`shown_count`、`stop_reason`（`enough` / `window` / `page_limit` / `exhausted`）。

### 测试（Go）

- 服务器筛选可用：返回 10 局当前队列 → 显示 10 局，`queue_filtered=true pages_read=1`。
- 筛选可用但其中 3 局早于 30 天 → 显示 7 局，`stop_reason=window`。
- 筛选不可用：第 1 页 30 局里当前队列 2 局，第 2 页 5 局，第 3 页 4 局 → 显示 10 局（2+5+3），`pages_read=3 stop_reason=enough`。
- 回退时第 2 页最早一局已超过 30 天 → 停止，`stop_reason=window`。
- 回退读满 4 页仍不足 → 停止，`stop_reason=page_limit`。
- 同一玩家先在 420、再在 2400 的对局里 → 两次结果互不复用。

Node：对局页某玩家返回 10 局当前模式战绩 → 显示「近 10 局」和 10 个战绩块。

## 测试（Go）

1. 复现本局：卡片模式，自己 76，备战席有 57，`timer_phase=BAN_PICK` → 不发交换，`reason=waiting-finalization`；切到 FINALIZATION → 发交换 57；postflight 生效 → `applied`。
2. BAN_PICK 里已有 2 次未生效 → 进入 FINALIZATION 仍会再试（最多 2 次）。
3. 卡片阶段自己选了不在序列的卡 → 不触发手动接管，备战席有序列英雄时仍会换。
4. 已经换到 57 后，用户又换走 → 本轮停止自动换人（现有规则）。
5. 开关关着、手上是 136（序列第 2）、备战席有 22（第 1）和 57（第 4）→ 不换；开关开着 → 换 22。
6. 手上 76（不在序列）、备战席有 57 和 13 → 换 57（排序更靠前）。
7. 普通大乱斗（非卡片模式）的现有备战席用例全部通过。
8. 镜头：两个文件都是 0、目标自由 → `camera_mode_matches_target=true`；`PersistedSettings.json` 是 2 → false。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| BAN_PICK 也发交换 | 测试 1 FAIL |
| BAN_PICK 的未生效次数计入上限 | 测试 2 FAIL |
| 开关关着仍横向换人 | 测试 5 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 可与 R201 合并成一个版本发布；R201 已发布则单独递增版本。按 R199 的规则发布到 GitHub Latest。
- `docs/WORKLIST-INDEX.md` 加 R202，R200 那行备注"卡片模式备战席交换时机由 R202 调整"。账本 `docs/r202-execution-ledger.md`。

## 真机验收（用户）

1. 打海斗：开局选一张卡（或软件替你选），等所有人选完、进入最后的准备阶段时，备战席里如果有你序列里的英雄，软件会换过去。
2. 开局拿到的已经是序列英雄：「优先选用序列靠前的英雄」开着时，只有备战席出现更靠前的才换；关着时不换。
3. 对局页每个人的近期战绩显示当前模式最新的 10 局（30 天内不足 10 局的按实际显示）。
4. 导出日志：选人期间有 `bench-gate reason=waiting-finalization`，进入 FINALIZATION 后 `bench-postflight reason=applied`；进游戏 60 秒 `camera_mode_matches_target=true`。
