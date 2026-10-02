# WORKLIST-R182：总览里输掉的排位没有胜点变化（每场负局都被 games_jumped 跳过）

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.45（如 R181 已合入，以合入后为准，两者不冲突）。

## 用户描述

总览战绩里，赢的排位显示胜点变化，输的排位没有。之前 R162 处理过同一个问题，只加了诊断，一直没修好。

## 证据

`lp_capture_*` 事件，来自 1001 当天的四份日志（1507、2027、2237、2302），不同运行和版本（0.12.41 / 0.12.43 / 0.12.45）：

| 结算时间（UTC） | 结果 | games_gap |
|---|---|---|
| 06:41 | recorded | — |
| 07:07 | recorded | — |
| 07:51 | recorded | — |
| 08:21 | recorded | — |
| 08:48 | **skipped: games_jumped** | **2** |
| 12:14 | recorded | — |
| 12:49 | recorded | — |
| 13:09 | **skipped: games_jumped** | **2** |
| 13:53 | recorded | — |
| 14:30 | recorded | — |
| 15:01 | **skipped: games_jumped** | **2** |

- 每次跳过都是 `has_baseline=true`、第 1 次轮询就拿到 `capability_state=available`，不是接口失败、也不是超时。
- **每一次跳过的场次差都正好是 2**，没有一次是 3 或更多。R162 账本（09-25）里同样是"胜局记录、负局跳过"交替出现，当时还没有 `games_gap`。
- 结合用户描述：**被跳过的就是负局**。也就是说，负局结算后，排位接口的"胜场 + 负场"相对基线多了 2，而不是 1。

### 根因（源码已核对）

`backend/lp_tracker.go` `capture()`：

```go
case snapshot.games() > baseline.games()+1:
    skipReason = "games_jumped"
```

只要场次差大于 1 就直接跳过，不计算 LP，而且这一局永远不会再补记。

为什么负局会多出 2 场，现有日志**无法确定**。原因是诊断只记了差值，没记胜场和负场各自变了多少，也没记基线是谁、什么时候写的。可能的情况有两种，修复方案需要两种都能正确处理：

- (a) 基线写入后被别的数据源覆盖了。基线有两个写入口：`capture()` 结算后写入；以及 `observe()`，每次总览加载排位数据后都会写入（`gameplay.go:1445`）。`observe()` 用的排位数据先经过 `applySeasonRankWinRateFallback`：上游负场为 0 时，用本地赛季统计的胜负场**替换**上游数值。这时基线的胜负场就和结算时读到的上游胜负场不是同一套计数。
- (b) 上游在开局或结算时对负场的计数方式特殊，例如开局时先计一场负场，结算后才修正。

两者都说明同一件事：**靠"胜负场正好 +1"来判断是不是这一局，在负局上不可靠。**

## P1　诊断：记下胜负场和 LP 各自的变化量（只记差值）

1. `lp_capture_recorded` 和 `lp_capture_skipped` 都加：`wins_delta`、`losses_delta`、`score_delta`（绝对分差，即这一局的 LP 变化），`baseline_source`（`capture` / `observe` / `observe_season_fallback` / `game_start`），`baseline_age_s`（基线写入距今秒数），`snapshot_source`（`lcu` / `sgp`）。
2. 新增 `lp_baseline_written`：每次基线被写入时记一条：`writer`（`capture` / `observe` / `game_start`），`season_fallback`（布尔），以及相对上一个基线的 `wins_delta` / `losses_delta` / `score_delta`。同一队列 60 秒内多次写入、而且值没变，只记一次。
3. 拿到第一个与基线不同的快照后，**再轮询 2 次**（间隔 5 秒），用 `lp_capture_settle` 记录每次相对基线的三个差值。用来确认上游的数会不会在几秒内再变。
4. 隐私：仍然不记玩家标识、段位、小段、绝对 LP、绝对胜负场。差值是小整数，不能反推出段位。更新 `lp_tracker_test.go:66` 的允许字段列表，同时保留"不得出现绝对值"的断言。

## P2　修复：每局开局时单独拍一次基线，用分差计算 LP

P2-1　**开局基线。** `connection_manager.go:245` 目前只在 `WaitingForStats / PreEndOfGame / EndOfGame` 时调用 `lpTracker.handlePhase`。增加 `InProgress`（首次进入；`GameStart` 也算）：

- 读 `/lol-gameflow/v1/session`，只处理队列 420 / 440。
- 用**与结算捕获完全相同**的读取函数（同一个 `loadRanks` 回调）读一次本人排位，存为本局的开局基线：`{gameId, queueType, snapshot, source, takenAt}`，并写入 `lp-history.json`（新字段，`schemaVersion` 递增并做兼容读取；老文件里没有这个字段时正常加载，不清空已有记录）。
- 开局基线写入后，到本局捕获结束前，`observe()` 不得覆盖它（覆盖队列级基线可以，但捕获优先用开局基线）。
- 读取失败：重试最多 3 次（间隔 5 秒）。仍失败就不写开局基线，捕获走原逻辑。

P2-2　**捕获优先用开局基线。** `capture()` 中，如果存在 `gameId` 与本局一致的开局基线：

- 等待条件：快照与开局基线**任一项不同**（胜负场或绝对分）。不再要求"场次增加"，避免情况 (b) 下胜局场次总数不变而永远等不到。
- 拿到第一个不同的快照后，再取 1 次（间隔 5 秒）。两次的绝对分一致才记录；不一致就以最后一次为准，继续最多再取 2 次，直到连续两次一致。
- 记录：`delta = 结算后绝对分 − 开局基线绝对分`。开局基线保证中间不可能夹着别的对局，所以 `games_gap ≥ 2` **不再跳过**；只把 `games_gap` 写进诊断。
- 场次和绝对分在 18 次轮询内都没变：记 `lp_capture_timeout`，不记录（负局有掉分保护、LP 不变时可能出现这种情况，宁可不显示也不显示错的值）。
- 开局基线和结算快照的 `source` 不一致（一个 LCU、一个 SGP）：不计算，继续轮询，直到拿到同一来源的快照；整轮都拿不到同源快照就跳过，原因记 `source_mismatch`。

P2-3　**没有开局基线时（例如对局进行中才启动软件）**：保留现有逻辑（场次正好 +1 才记录，否则跳过），行为不变。

P2-4　**`observe()` 不再用赛季统计补出来的胜负场写基线。** `gameplay.go:1440` 一带：把"经过 `applySeasonRankWinRateFallback` 补全"的标记带进 `observe()`；被补全的队列不写基线（记 `lp_baseline_written` 的跳过原因 `season_fallback`，同样 60 秒去重）。上游原样返回、可信的数据照常写。

P2-5　结算基线更新：捕获结束后，队列级基线仍更新为最后一次快照（与现在一致），并删除本局的开局基线。

## 测试

Go（`lp_tracker_test.go`）：

1. **负局场次差 2**：开局基线 W=10 L=10，结算快照 W=10 L=12、分数 −18 → 记录 −18，`games_gap=2`，`baseline_source=game_start`。
2. **开局预计负场**：开局基线已是 L+1；胜局结算后 W+1、L−1，场次总数不变、分数 +20 → 记录 +20，不超时。
3. **负局分数立即变、场次稍后变**：第一次不同的快照只有分数变化，第二次分数一致 → 记录；分数先变后又变 → 以连续两次一致的值为准。
4. **掉分保护**：分数和场次在全部轮询中都不变 → `lp_capture_timeout`，不写记录。
5. **来源不一致**：开局基线是 LCU，前两次结算快照是 SGP，第三次是 LCU → 只用 LCU 那次计算；全程都是 SGP → `source_mismatch`，不记录。
6. **无开局基线**：现有"恰好 +1 记录 / 多于 1 跳过"的行为与 0.12.45 完全一致（原测试保留）。
7. **observe 不覆盖**：开局基线写入后、捕获前，总览加载触发 `observe()` → 开局基线不变。
8. **赛季补全不写基线**：`observe()` 收到被补全的队列 → 队列级基线不变，`lp_baseline_written` 记跳过原因 `season_fallback`。
9. **`lp-history.json` 兼容**：旧格式（没有开局基线字段）正常读取，已有的 `games` 记录不丢；新格式写入后再读取，往返一致。
10. **隐私**：所有 `lp_*` 事件只含允许字段；构造绝对 LP / 胜负场 / 段位字符串，断言不出现在任何事件里。
11. **端到端**：模拟 `InProgress` → `WaitingForStats` 的 gameflow 事件，确认开局基线由 `InProgress` 触发、捕获使用它。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 有开局基线时仍对 `games_gap ≥ 2` 跳过 | 测试 1 FAIL |
| 等待条件改回"场次必须增加" | 测试 2 FAIL |
| `observe()` 可以覆盖开局基线 | 测试 7 FAIL |
| `observe()` 写入赛季补全值 | 测试 8 FAIL |
| `InProgress` 不触发开局基线 | 测试 11 FAIL |

`node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R182，R162 那行备注"负局 LP 缺失由 R182 修复"；账本 `docs/r182-execution-ledger.md`。
- 账本写明：历史上已经被跳过的负局**无法补记**，因为当时没有可用的开局基线。
- 前端 `renderMatch` 已经能显示负值，不改界面，不加说明文字。

## 真机验收（用户）

1. 软件**在开局之前就打开着**，打一把排位，输了之后打开总览：这一局显示负的胜点变化（例如 −18）。赢的局照常显示。
2. 导出日志（导出前不要关软件）：负局那条是 `lp_capture_recorded`，`baseline_source=game_start`；还能看到 `wins_delta` / `losses_delta`，我用它确认负局多出的那 1 场是哪来的。
3. 对局进行中才打开软件的那一局，没有胜点变化属于预期（没有开局基线）。
