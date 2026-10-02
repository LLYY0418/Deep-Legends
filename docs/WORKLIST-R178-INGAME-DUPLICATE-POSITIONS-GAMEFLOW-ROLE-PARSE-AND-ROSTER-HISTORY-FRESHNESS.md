# WORKLIST-R178：对局中多名玩家显示相同位置（根因已定位）+ 战绩列表"不是最新"的取证

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.42（R177 之后）。

## 先说明证据范围

- 用户这次上传的 `lol-loot-diagnostics-1001-1656.jsonl` 与上一轮的同名文件**逐字节相同**（15059 行，最后一条 08:56:21Z，最后的 build 是 0.12.41 / `c06daae1113a`）。里面**没有** 0.12.42 的会话，所以 R177 这一轮**无法验证**，截图对应的那一局也不在日志里。
- 下面 P1 的根因来自日志里 0.12.41 的第 2 场灵活排位（game 9009400838），并用源码逐条复现，结论确定。
- P2 只有截图，没有对应日志，所以 P2 先补诊断；只有一处行为改动，而且证据充分。

## P1　对局中出现多名同位置玩家（例如 3 个打野、只有 1 个下路）

### 证据

game 9009400838（queue 440），`live_position_shape`：

- 选人阶段（08:22:20）：己方 `assigned_position_counts` 为 BOTTOM/JUNGLE/MIDDLE/TOP/UTILITY 各 1，正确。
- 进入对局（08:25:55）：`normalized_position_counts = {bottom:1, jungle:3, middle:2, top:2, utility:2}`，`snapshot_applied_count=5`。
- 同一时刻，gameflow 的 `selected_position_counts` 是 BOTTOM/JUNGLE/MIDDLE/TOP/UTILITY **各 2 个，完全正确**；`selected_role_counts` 是 `BOTTOM.PRIMARY.BOTTOM.TOP ×2`、`JUNGLE.PRIMARY.JUNGLE.TOP.FILL ×2`、`MIDDLE.PRIMARY.MIDDLE.JUNGLE.FILL`、`MIDDLE.PRIMARY.MIDDLE.TOP.FILL`、`TOP.PRIMARY.TOP.BOTTOM`、`TOP.PRIMARY.TOP.JUNGLE.FILL`、`UTILITY.PRIMARY.UTILITY.BOTTOM ×2`。
- 08:25:46 的 Live Client 探测是 `unavailable`（游戏进程还没起来），之后这场对局**再也没有重新加载**名单（下一条 `live_refresh_client` 已经是 08:48 的下一局）。所以整场的位置都来自兜底来源。

### 根因（源码已核对，并按日志数据复算完全吻合）

`backend/gameplay.go:10469` 的 `normalizePosition(lane, role)` 把两个参数拼接成一个字符串，再按 **JUNGLE → MIDDLE/MID → TOP → UTILITY/SUPPORT → BOTTOM/BOT/CARRY** 的顺序做**子串匹配**。

新版客户端 gameflow 的 `selectedRole` 是复合串，格式为"分配位置.PRIMARY.第一志愿.第二志愿[.FILL]"，里面带着玩家排队时填的**其他志愿**。`gameplay.go:7590`（`seed.Position`）和 `:7717`（`Position: normalizePosition(raw.player.SelectedPosition, raw.player.SelectedRole)`）都把它整串传进去，于是：

| selectedPosition | selectedRole | 现在得到 | 正确值 |
|---|---|---|---|
| BOTTOM | BOTTOM.PRIMARY.BOTTOM.TOP | **top** | bottom |
| MIDDLE | MIDDLE.PRIMARY.MIDDLE.JUNGLE.FILL | **jungle** | middle |
| TOP | TOP.PRIMARY.TOP.JUNGLE.FILL | **jungle** | top |
| JUNGLE / UTILITY / 其余 | … | 正确 | — |

把这场 10 人代入复算：兜底结果是 top 3 / jungle 4 / middle 1 / utility 2 / bottom 0；己方 5 人随后被选人快照（正确）覆盖，敌方 5 人保留错误值，最终正好是日志里的 **bottom 1 / jungle 3 / middle 2 / top 2 / utility 2**。第 1 场（07:57）的兜底值同样会算出 jungle 4，只是那一场 Live Client 一开始就可用，正确位置把它覆盖掉了，所以没暴露。

结论：只要进对局时 Live Client 还没准备好（常见），敌方位置就会按"志愿里出现过的位置"被错标，出现重复；之后又没有重新加载，错误会一直挂到对局结束。

### 修复

P1-1　**新增 `normalizeGameflowPosition(selectedPosition, selectedRole string) string`**，只用于 gameflow 来源：

- 先对 `selectedPosition` 做**整词精确匹配**（TOP / JUNGLE / MIDDLE / BOTTOM / UTILITY，大小写、空白容错），不做子串匹配。
- `selectedPosition` 为空或无法识别时，只取 `selectedRole` 按 `.` 分割后的**第一段**，同样整词匹配。
- 其余段（PRIMARY、FILL、志愿位置）一律忽略；`NONE`、`UNSELECTED`、`FILL` 返回 `""`。海斗 / 大乱斗等现在得到 `other` 的场景，行为必须保持不变（执行时核对现有 `other` 的来源，不能回归）。
- `gameplay.go:7590` 和 `:7717` 改用它。**不要改** `normalizePosition` 本身：`gameplay.go:3561` 的比赛时间线 `Lane/Role`（如 `BOTTOM` + `DUO_SUPPORT`）依赖现有的子串规则。

P1-2　**经典模式对局中，Live Client 首次不可用时补探测。**现在只有海斗有 `startMayhemSampler` 会继续重试，经典模式第一次失败后整场都不会再取位置。

- 后端：`InProgress / Reconnect` 时，如果 Live Client 位置没有拿到，就在响应里给出能力标记（例如在 `Capabilities` 里加一条 `live-client-positions`，状态 `pending`）。
- 前端：看到 `pending` 时，每 10 秒重新加载一次名单，最多 6 次。拿到位置、对局 ID 变化或阶段离开对局，都立即停止。必须和 R90"对局中停止无意义自动刷新"的约束共存：只在 `pending` 时才刷新，拿到之后恢复静默。
- 不新增界面文字。

P1-3　**诊断**：`live_position_shape` 加两组字段：

- `position_source_counts`：最终位置分别来自 `liveclient / snapshot / gameflow / none` 的人数。
- `team_duplicate_positions`：每队重复位置的人数（正常为 0）。

不记录玩家身份。

### 测试

Go：

1. 用日志里的 7 种 `selectedRole` 原样建表：`normalizeGameflowPosition` 全部得到与 `selectedPosition` 一致的位置；`UTILITY.FILL_PRIMARY.FILL.UNSELECTED.FILL` → utility；`NONE` + `NONE.NONE.NONE.UNSELECTED` → 与现状一致（海斗场景）。
2. 复现 game 9009400838：Live Client 不可用、己方快照 5 人、敌方只有 gameflow。最终两队各 5 个不同位置，`team_duplicate_positions` 为 0。
3. `normalizePosition("BOTTOM", "DUO_SUPPORT")` 仍为 utility（锁定时间线路径不受影响）。
4. Live Client 不可用时响应带 `live-client-positions: pending`；可用时不带。

Node：

5. 收到 `pending` 后按 10 秒节奏最多重载 6 次；拿到位置、对局 ID 变化或离开对局时停止；没有 `pending` 时对局中不自动刷新（R90 行为不变）。

变异（overlay，不改仓库源码，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 7717 行改回 `normalizePosition(SelectedPosition, SelectedRole)` | 测试 1 / 2 FAIL |
| `normalizeGameflowPosition` 改成对整串做子串匹配 | 测试 1 FAIL |
| 前端去掉 `pending` 重载 | 测试 5 FAIL |

## P2　战绩列表里部分玩家"不是最新"（中路、下路）

### 现状（源码已核对）

对局 / 选人名单每个玩家的战绩由 `loadLivePlayerMatches`（`gameplay.go:8189`）读取：

1. 先读本机客户端 `/lol-match-history/v1/products/lol/{puuid}/matches`，窗口为最近 **30 场**（`liveHistoryWindow`）。只有客户端返回失败或空列表时，才用 SGP 兜底。
2. 再由 `recentMatchesForPlayer`（`gameplay.go:7181`）筛选：**只保留与当前对局同一队列的胜负局**（例如灵活排位只留 440），最多 10 场。
3. 后端缓存 45 秒（`livePlayerMatchesCacheTTL`），不会造成长时间陈旧。

所以"不是最新"有两种可能，截图本身无法区分：

- (a) **按队列筛选**：玩家最近打的是单双排、大乱斗等其他模式，这些不会出现在名单里，看到的是 30 场窗口里较早的同队列对局。小卡片没有日期，看起来就像"没更新"。
- (b) **客户端对他人战绩的接口返回旧数据**：本机客户端查别人的战绩可能有自己的缓存。总览页查人走的是 SGP（日志里的 `sgp_match_history_succeeded`），两边可能对不上。

### P2-1　诊断（必做）：`live_history_freshness`

每局每个玩家记一次，只记计数和时间差，不记身份 / matchID：

- `team`、`is_current`、`source`（lcu / sgp）、`window_games`（返回场数）
- `newest_any_age_min`：窗口内最新一场（不限队列）距今多少分钟
- `newest_queue_age_min`：当前队列最新一场距今多少分钟
- `queue_games`：当前队列场数
- 抽样比对：每局最多抽 2 名非本人玩家，**额外**请求一次 SGP SUMMARY（走现有 `a.sgp.matchHistory`，同样 30 场）。记录 `sgp_newest_any_age_min`，以及 `lcu_missing_newer`：SGP 里比 LCU 最新一场更新、而 LCU 列表里没有的场数。抽样请求失败只记 `sample_failed`，不影响名单。

拿到下一份日志后：如果 (b) 成立，`lcu_missing_newer > 0`；如果是 (a)，`newest_any_age_min` 会远小于 `newest_queue_age_min`。

P2-2　**行为改动（只做这一条，有充分证据才改）**：非本人玩家，如果客户端返回的最新一场比 SGP 抽样旧（即 `lcu_missing_newer > 0`），这一局后面再加载该玩家时直接走 SGP（45 秒缓存不变）。本人仍用 `current-summoner`。没有 SGP 或 SGP 失败时保持现状。

队列筛选规则（a）本工单**不改**。它是产品口径问题，等用户确认想要"当前模式最近 10 场"还是"所有模式最近 10 场"后另开工单。

### 测试

Go：

6. 构造 LCU 列表最新一场早于 SGP 两场：`lcu_missing_newer=2`，同局再次加载该玩家时走 SGP；本人不受影响。
7. SGP 抽样失败：名单数据与现状一致，只记 `sample_failed`。
8. 每局抽样不超过 2 人（多次刷新不重复抽样）。
9. `newest_any_age_min / newest_queue_age_min / queue_games` 计算正确（混合队列夹具）。

变异：去掉"每局最多 2 人"的限制 → 测试 8 FAIL。

## 收尾

- 版本递增（0.12.43）；`docs/WORKLIST-INDEX.md` 加 R178；账本 `docs/r178-execution-ledger.md`。
- 账本必须写明"R177 尚未真机验证（用户上传的日志与上一轮相同，不含 0.12.42 会话）"。
- `node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 界面红线不变：不加说明文字 / tooltip / 口径说明。

## 真机验收（用户，Windows）

1. 打一把排位进入对局后，看名单两边：每队 5 个位置互不重复；敌方位置和实际分路一致（开局几十秒内可能先显示兜底位置，之后会自动纠正）。
2. 选人阶段截图里"不是最新"的玩家，打开他的总览对比最近几场。记下他最近打的是不是别的模式，导出日志时告诉我是哪一路。
3. 导出日志前不要关闭软件。这次请确认导出的是**新文件**（时间在本局之后）。
