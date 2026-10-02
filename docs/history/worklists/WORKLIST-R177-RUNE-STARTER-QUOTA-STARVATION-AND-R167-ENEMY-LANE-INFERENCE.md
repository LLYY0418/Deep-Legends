# WORKLIST-R177：绝活哥出门装被后台额度挡住（根因已定位）+ R167 对线卡片拿不到敌方分路

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.41（R175 / R176 之后）。

## 背景与证据

用户提供 `lol-loot-diagnostics-1001-1656.jsonl`。其中最后一段（run `c26948c0…`，build `c06daae1113a`，version 0.12.41，15:56–16:56）是装了 R175 / R176 之后的真机会话：两把灵活排位（queue 440，game 9009347791 / 9009400838）加一把海克斯大乱斗（queue 2400，game 9009458591）。

这段日志里已经确认的：

- 没有 `backend_panic`、`desktop_backend_exit`、`desktop_relaunch_*`；三把 `live_roster_shape` 的 `players=10`、`team_counts 100:5 / 200:5`，`live_roster_rendered` 的 `rendered_100 / rendered_200` 都是 5。
- 但 `live_roster_recovery` 在这段里 **一次都没出现**（该事件上一轮 0.12.39 日志里有 17 条）。也就是说这几把没有触发"漏人补位"分支，R175 P1 的修复只能算"没复发"，不算真机验证。本工单不处理，验收时看 P3。
- 符文一键应用正常：`perk_apply_attempt` outcome=success，`spell_requested=true`、`spell_applied=true`（08:25:18，Yone 绝活哥）。
- 斗魂 / 海斗详情冷加载：`mayhem_detail_phases_ms` 第一次 total 1712 ms，其中 `rsc_wire` 约 1041 ms；之后热加载 80–130 ms。属于正常的首次拉取，不再单独开工单。

本工单处理剩下两个问题，都有明确证据。

## P1　符文卡片「出门装」被后台额度挡住，用户基本等不到

### 现象与时间线

- 第 1 场（Darius，07:57）：`specialist_runes_done` 在 07:57:12，`budget_used=36`。出门装请求直到 **08:07:40** 才发出（用户 10 分钟后才点开），6 个 `rune_starter_items` 全部 `outcome=success`。
- 第 2 场（Yone，08:24:54）：`specialist_runes_start` 08:24:54，`specialist_runes_done` 08:24:59，`budget_used=36`。**15 秒后**（08:25:14.880–.881）3 个 `rune_starter_items` 在同一毫秒内全部 `outcome=failed reason=rate-limit`。同一毫秒说明是本地限流器直接拒绝，没有真正发出 HTTP。
- 上一份 1507 日志里第 2 场 9/9 全部同样失败，同一模式。

### 根因（已核对源码，不是猜测）

1. `backend/rune_starter_items.go:105` 与 `:158`：出门装的 `matchTimeline` 请求都带 `withRiotBackground(ctx)`，走后台准入。
2. `backend/pro_refresh.go:23-50` `admitRiotBackground` → `backend/riot_rate_headers.go:65-100` `scopedRiotDelay(…, background=true)`：对 ≥1 分钟的窗口，后台只允许 `limit/3`。日志里的策略是 app `20/1s、100/120s`，所以后台在 120 秒窗口里只有 **33** 个名额。
3. 绝活哥的符文批量请求（`specialist_runes.go`，预算 `specialistRuneRequestBudget = 36`）**不是后台请求**，它走前台，和后台共用同一个 app 窗口计数。36 > 33，所以批量请求一结束，窗口占用就已经超过后台上限；之后 120 秒窗口内任何后台请求都被拒绝（`riot_rate_headers.go` 注释写明窗口从首次观察起整段等待，不猜测重置时刻）。
4. 失败处理雪上加霜：`rune_starter_items.go` 约 177–186 行，任何错误（包括限流）都会写入 `starterFailures[matchID]`，该场 90 秒内直接返回 `starter retry cooldown`；前端 `ensureRuneStarterItems`（`gameplay.js` 约 5139–5178）对同一请求键 90 秒内不重试，请求失败被 `catch (_)` 吞掉，也不会按时间重试。选人阶段只有约 40 秒，之后进入对局，用户再也等不到。

一句话：出门装不是"偶尔限流"，而是**每次紧跟在绝活哥批量请求之后几乎必然失败**，只有隔很久才打开才成功。

### 修复

P1-1　**出门装改走前台准入，但带短排队上限**，不放宽后台上限。

- `rune_starter_items.go:105` 和 `:158` 去掉 `withRiotBackground`，改为 `withRiotQueueLimit(ctx, 6*time.Second)`（沿用 `riot_api.go` 已有的 `withRiotQueueLimit`）。理由：出门装是用户打开卡片后才触发的、条数很小（前端每次最多 3 条，pro 来源 5 条）、结果缓存一年的请求，不是批量后台任务。
- **不要改** `admitRiotBackground` 和 `limit/3` 的规则：职业选手种子 / 刷新等真正的后台任务依赖它。
- 并发保持 2，预算保持独立的最多 9 条（`runeStarterRowLimit`），不要和 `specialistRuneRequestBudget` 合并。
- 前台排队 6 秒仍拿不到额度时，照旧返回 `errThrottled`（带 `retryAfter` 秒数），由 P1-2 / P1-3 处理，不要改成无限等。

P1-2　**限流不是对局失败，不写 90 秒冷却。**

- `specialistMatchStarters` 的错误分支：`errors.Is(err, errThrottled)` 时不要写 `starterFailures`；其他错误（404、解析失败、网络错误）保持原有 90 秒冷却。
- 响应 JSON 在有行因限流未取到时，增加顶层字段 `retryAfterSeconds`（取这批错误里最大的 `riotStatusError.retryAfter`，夹在 5–60 之间）。字段不存在时前端行为不变。

P1-3　**前端在选人 / 对局页面仍然可见时按提示重试，最多 2 次。**

- `ensureRuneStarterItems` 收到 `retryAfterSeconds` 且仍有可见行缺 `starterItemIds` 时，用 `setTimeout` 在 `clamp(retryAfterSeconds, 5, 45)` 秒后重新触发同一目标；同一请求键最多 2 次定时重试，定时重试绕过现有"90 秒内不重复"的保护，但仍受"请求进行中不并发"的限制。
- 对局代数（`state.liveGameGeneration`）变化、符文来源页签切换、目标键变化时清除定时器（接入现有 4680 行附近的取消逻辑），不允许对已离开的对局继续请求。
- 请求失败（网络 / 超时）不重试，保持现状。
- 不新增任何界面文字、加载提示或 tooltip（红线）。

P1-4　**诊断。**新增 `rune_starter_batch` 事件，每个 `/api/gameplay/rune-starters` 请求结束记一条：`source`、`rows_requested`、`success`、`rate_limit`、`other_failed`、`retry_after_s`、`queue_wait_ms`。不记录 matchID、玩家身份。`rune_starter_items` 单条事件保持不变。

### 测试

Go（用 `limitNow` 假时钟 + `httptest` 假上游，不碰真实 Riot）：

1. 把 app `100/120s` 窗口的 `used` 预置为 40（模拟批量请求之后）：后台上下文的请求被拒绝（锁定现有后台行为，防止误改 `admitRiotBackground`）；`specialistStarterRows` 在同样状态下 **成功** 取到出门装。
2. 前台额度也满时：`specialistStarterRows` 在 6 秒排队上限内返回限流，响应带 `retryAfterSeconds`，且 `starterFailures` 里 **没有** 该 matchID。
3. 上游返回 404 / 500：`starterFailures` 写入，90 秒内再次请求返回冷却，响应不带 `retryAfterSeconds`。
4. `rune_starter_batch` 的计数与实际成功 / 限流 / 其他失败数一致。

Node（`gameplay.test.cjs`，假定时器）：

5. 响应带 `retryAfterSeconds: 8`、仍缺出门装：8 秒后再次请求；第 2 次仍失败则再重试 1 次；第 3 次不再发。
6. 重试等待期间对局代数变化 / 切到别的来源页签：不再发请求。
7. 没有 `retryAfterSeconds` 的响应：不安排重试，行为与现在一致。
8. 重试成功后 `patchRuneStarterEquipment` 把出门装补到行里，召唤师技能（R174）仍在最右。

变异（用 overlay，不改仓库源码；各自必须断言 FAIL，不是编译错误）：

| 变异 | 期望 |
|---|---|
| 把 `withRiotBackground` 放回 `rune_starter_items.go:105/158` | 测试 1 FAIL |
| 限流错误也写入 `starterFailures` | 测试 2 FAIL |
| 删掉前端定时重试 | 测试 5 FAIL |
| 去掉对局代数检查 | 测试 6 FAIL |

验证：`node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## P2　R167 对线克制卡片：这一轮仍然一次都没触发

### 证据

- `lane_matchup_candidate_fetch` 在两场灵活排位里只有 `reason=context-unavailable`，没有任何 `own-champion-selected / requested / cached`。
- `champ_select_autofill_shape`：`enemy_assigned_position_count=0`（08:22:18、08:48:43）；`live_position_shape` 选人阶段 `normalized_position_counts` 里敌方 5 人位置为 `""`。对应 `laneMatchupContext`（`gameplay.js` 约 5581–5591）要求 `livePositionValue(player.position) === position`，敌方位置永远为空，所以永远返回 `null`。
- R168 已记录过"queue 440 选人阶段敌方英雄锁定可实时看见"，所以问题在敌方 **分路**，不在英雄。
- 但是：本日志里 `lcu_champ_select_session_shape` 只在 08:22:18 / 08:23:15 采样了两次（都还在禁用阶段），**没有敌方锁定之后的样本**，没法在日志里直接确认这两场敌方英雄是否在选人阶段可见。所以第一步先补诊断，第二步再做推断。

### P2-1　诊断（先做，成本很小）

`ensureLaneMatchupCandidates` 的 `context-unavailable` 记录里补这些字段（都是计数 / 枚举，不含玩家身份）：`self_position`、`enemy_locked_count`（`!isAlly && teamId 不同 && championLocked && championId>0`）、`enemy_position_known_count`、`ally_position_known_count`。去重键加入这三个计数，计数变化时再记一条；每个对局最多 20 条，防止刷屏。后端白名单同步放行这些字段。

### P2-2　敌方分路推断

规则必须保守：**不确定就不显示卡片**，宁可不出现也不能把非同路英雄当对位。

1. 数据来源：只用项目里已有的"英雄 → 分路占比"数据（和推荐面板分路切换芯片同源，`displayRoleRate`，后端 `qq101.go: loadQQ101Positions` 一类已缓存路径）。新增一个只读接口 `GET /api/gameplay/champion-lanes?champions=1,2,3,4,5`，返回 `{ "<championId>": [{ "position": "top", "rate": 0.62 }, …] }`；最多 5 个 ID，非法 / 超量返回 400；只读缓存或走现有 provider 路径，**不新增任何上游域名或请求类型**。段位使用与 `laneMatchupTier` 相同的取值（未知用 `all`）。
2. 前端：对每个 `championLocked`、`position` 为空的敌方英雄取分路占比（按英雄 ID + 段位缓存，同一英雄不重复请求）。位置已知的敌方照旧使用已知位置，**已知位置优先**，不被推断覆盖。
3. 判定"本路对位"：在本人分路上，满足下面任一条才算：
   - 所有已锁定敌方里，只有一个英雄在本人分路的占比 ≥ 0.25；或
   - 有多个 ≥ 0.25，但最高者 ≥ 0.5 且至少是第二高者的 2 倍。
   否则视为"无法确定"，不显示卡片，诊断记 `lane-ambiguous`。
4. 推断出的对位只用来替换 `laneMatchupContext` 里"同位置敌方"这一步，后面的克制数据、候选请求、去重逻辑、"本人悬停即停止候选"规则一律不变。
5. 界面不新增任何文字（不写"推断""可能"之类的说明），卡片外观与 R167 一致。

### 测试

Go：`champion-lanes` 返回缓存数据；非法 ID、超过 5 个返回 400；不发出任何非既有的上游请求。

Node（`champions.test.cjs` / `gameplay.test.cjs`）：

1. 敌方中单位置为空、占比 mid 0.7，本人中单：出现对线卡片。
2. 两个已锁定敌方在本人分路占比都 ≥ 0.25 且没有 2 倍优势：不出现卡片，诊断 `lane-ambiguous`。
3. 敌方有明确 `position`：行为与现在完全一致（不请求 `champion-lanes`）。
4. 同一敌方英雄只请求一次分路占比；敌方换英雄后只为新英雄再请求一次。
5. 占比数据缺失 / 请求失败：不出现卡片，不报错。

变异：删掉 2 倍优势判断 → 测试 2 FAIL；已知位置不再优先 → 测试 3 FAIL。

## P3　只有真机能验证的（不是代码改动，写进账本）

账本 `docs/history/ledgers/r177-execution-ledger.md` 里必须逐条写明"未验证"，不能写成已验证：

- R175 P1 漏人补位分支：这轮日志没有触发。需要出现 `live_roster_recovery` 事件的真机对局。
- R176、R174、R173 的视觉效果：日志不记录布局，只能用户目测。
- R168 P1 / P4 前端项、自动补位样本：这轮没有对应事件。

## 收尾

- 版本按仓库惯例递增（0.12.42），构建验证同 R175 / R176；`docs/WORKLIST-INDEX.md` 加 R177；账本 `docs/history/ledgers/r177-execution-ledger.md`。
- 界面红线不变：不新增统计口径 / 说明文字 / tooltip。
- 不重做、不回退 R173 – R176 的任何内容。

## 真机验收（用户，Windows）

1. 进一局灵活排位选人，选完符文页的"绝活哥"后立刻点开对局记录：出门装应在几秒内出现（最迟一次重试后），不再像上次那样全部缺失；导出日志里 `rune_starter_batch` 的 `rate_limit` 应为 0 或随后重试成功。
2. 选人阶段敌方中路锁定后，如果你是中单：出现"对线克制建议"卡片；敌方两个英雄都可能打你这一路时不出现卡片。导出日志里 `lane_matchup_candidate_fetch` 能看到 `enemy_locked_count` 等字段。
3. 顺带照旧观察：左右两侧人数、刷新是否报错。
