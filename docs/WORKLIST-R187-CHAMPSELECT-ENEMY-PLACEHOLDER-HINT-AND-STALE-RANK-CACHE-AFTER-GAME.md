# WORKLIST-R187：选人阶段敌方占位补一句提示 + 赛后排位数据 10 分钟内仍是赛前旧值

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.50（R185 已合入）。R186 如果先合入，以合入后为准，两者不冲突。

## 证据范围

日志 `lol-loot-diagnostics-1002-1815.jsonl`，0.12.49 运行 `0807172d…`（08:34–10:15Z）。打了 4 局海克斯大乱斗 (2400)、1 局灵活组排 (440，**胜**)，最后一局灵活组排在选人阶段就导出了。

真机已确认正常（不用改）：

- R183 匿名占位：两局海斗 `live_roster_recovery reason=anonymous-placeholder appended=1`，渲染 5/5。
- R182 胜局：`lp_capture_recorded baseline_source=game_start games_gap=1 wins_delta=1 losses_delta=0 score_delta=17`。**负局还没有真机样本。**
- R181 对位克制卡片：两局排位都显示（`lane_matchup_card shown=true`，预选时 b、锁定后 a）。
- R185 P1：生涯页在本次运行只加载 2 次（上一版本 959 次），`facade_event_source` 忽略了大部分聊天事件。后端内存 43–143 MB，goroutine 14–38，正常。

## P1　选人阶段敌方卡片：「暂无玩家信息」后面加一句提示

用户要求。`backend/web/gameplay.js` `renderLivePlayer` 里 `champSelectEnemyPlaceholder` 分支（约 5602 行）：

- 文案改为 **`暂无玩家信息，进入游戏后显示`**，一行显示，不加图标、tooltip 或其他说明。
- 只改选人阶段的敌方占位。对局中的「隐藏玩家」卡片不变。
- 窄宽度时允许换行，不能截断成省略号。

Node 测试：ChampSelect 敌方占位渲染出完整文案；InProgress 阶段不出现该文案。

## P2　赛后排位数据 10 分钟内仍是赛前旧值，而且会把 LP 基线写回旧值

### 证据（本局 440 胜局）

| 时间 (UTC) | 事件 |
|---|---|
| 10:11:10 | `lp_capture_recorded` +17；`lp_baseline_written writer=capture wins_delta=+1 score_delta=+17` |
| 10:11:33 | 总览加载：`overview_phases_ms ranks=0 ms`，命中缓存 |
| 10:11:33 | `lp_baseline_written writer=observe wins_delta=-1 score_delta=-17` |

结算捕获刚写入赛后基线，23 秒后总览用缓存里的**赛前**排位数据调用 `observe()`，把基线退回到赛前。

### 根因（源码已核对）

1. 总览的排位数据来自 `rank_insights.go` `playerRankScoreWithCacheStatus` 的 `rankScores` 缓存。TTL 是 `rankScoreCacheTTL = 10 * time.Minute`（第 63 行）。结算后没有任何地方让当前玩家的缓存失效，`force` 刷新也只清 SGP 战绩（`gameplay.go` 约 1165 行），不清排位缓存。
2. 所以赛后 10 分钟内，总览头部的段位、胜点、胜负场都是**赛前旧值**（本局就是少了 17 分、少 1 胜）。
3. `gameplay.go` 约 1446 行用这份旧数据调用 `lpTracker.observe()`。`observe()` 只在 `pending`（捕获进行中）时跳过，捕获结束后不再拦，于是 `writeQueueBaselineLocked` 把队列基线写回赛前值。

影响：

- 下一局有开局基线时，`capture()` 用开局基线，**不受影响**（开局快照走 `loadRanksWithFallback`，不经过这个缓存）。
- 下一局**没有**开局基线时（对局中才打开软件、或开局读取三次失败），队列基线是退回去的旧值，场次差会变成 2，又落回 `games_jumped` 跳过。这正是 R182 之前负局丢失的那种形态。
- 总览头部胜点显示滞后最多 10 分钟。

### 修复

P2-1　**结算后让当前玩家的排位缓存失效。** 给 `rankScoreCache` 加 `invalidatePlayer(playerRef)`，删除该玩家所有来源和作用域（含 tier-only）的条目，并结束进行中的 flight（等待者不能拿到旧值）。调用时机：

- `lpTracker.capture()` 结束时（记录、跳过、超时都算）；
- gameflow 进入 `EndOfGame` 时，所有队列都调用（非排位局结束后排位数据不会变，但清一次代价很小，可以省掉判断）。

P2-2　**`observe()` 拒绝倒退的快照。** 写队列基线前，和当前队列基线比较：新快照的 `wins + losses` **小于**现有基线，就不写入，记 `lp_snapshot_rejected stage=observe reason=stale_regression`（60 秒去重）。场次相同但分数不同的照常写入（例如赛季重置、手动调整），场次增加的照常写入。

P2-3　**总览手动刷新也不用排位缓存。** `force=true` 时，当前玩家的排位读取跳过 `rankScores` 缓存，读到后写回缓存。

### 测试

Go：

1. 缓存失效：`put` 当前玩家（LCU 和 SGP 两种来源、普通和 tier-only 两个作用域）→ `invalidatePlayer` → 四个键都 `get` 不到；其他玩家的条目保留。
2. 复现本局：开局基线 W10 L10 S50 → 结算快照 W11 L10 S67 → 记录 +17 → 用缓存里的 W10 L10 S50 调 `observe()` → 队列基线仍是 W11 L10 S67，事件 `lp_snapshot_rejected reason=stale_regression`。
3. `observe()` 收到场次更多的快照 → 正常写入；场次相同、分数不同 → 正常写入。
4. 端到端：模拟 InProgress → EndOfGame → 总览加载。总览拿到的 ranks 来自新读取（假的上游计数 +1），不是缓存。
5. `force=true` 的总览：排位读取计数 +1，结果写回缓存。
6. 无开局基线的下一局：前一局结束后 observe 拿到旧值也不退基线，下一局场次差 = 1，正常记录。

Node：

7. P1 文案测试（见上）。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 不调用 `invalidatePlayer` | 测试 4 FAIL |
| `observe()` 不比较场次 | 测试 2、6 FAIL |
| `invalidatePlayer` 只删 LCU 来源的键 | 测试 1 FAIL |
| 敌方占位文案恢复为「暂无玩家信息」 | 测试 7 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R187，R182 那行备注"赛后旧缓存回写基线由 R187 修复"；账本 `docs/r187-execution-ledger.md`。
- 账本写明：R182 负局记录仍未有真机样本；R185 P2 的 `renderer_perf` / `desktop_process_metrics` 在 0.12.49 日志里没有出现，需要 0.12.50 以后的日志确认。
- 除 P1 那一句外，不加任何界面文字。

## 真机验收（用户）

1. 选人阶段，敌方卡片显示「暂无玩家信息，进入游戏后显示」。
2. 软件开着打一把排位，结束回到大厅后立刻看总览头部：段位胜点已经是赛后的值，不用等 10 分钟。
3. 导出日志：结算后不再出现 `lp_baseline_written writer=observe` 且 `wins_delta`/`losses_delta` 为负数的情况。
