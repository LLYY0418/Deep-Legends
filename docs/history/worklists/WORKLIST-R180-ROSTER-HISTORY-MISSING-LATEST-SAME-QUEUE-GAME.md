# WORKLIST-R180：选人 / 对局名单里部分玩家"当前模式最新一局"没加载出来

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.43（R178 之后；R179 若已合入，以合入后的源码为准，两者不冲突）。

## 用户明确的口径

- **保持"当前模式最近 10 局"不变**，不改成所有模式，不改筛选规则。
- 问题是：有些玩家**当前模式的最新一局没有出现**在名单里。目标是这 10 局必须是真正最新的 10 局。

R178 P2 的结论（"客户端数据不陈旧，是队列筛选导致"）**作废**：它的取证方法本身不足以得出这个结论，见下文。

## 现状（源码已核对）

名单每个玩家的战绩：

1. `loadLivePlayerMatches`（`gameplay.go` 约 8189 行）先读本机客户端 `/lol-match-history/v1/products/lol/{puuid}/matches`，取 30 场。**只有客户端返回失败或空列表时**才用 SGP。
2. `recentMatchesForPlayer`（约 7185 行）按 `CreatedAt` 倒序，跳过非胜负局（重开局）、跳过非当前队列，再用 `recentMatchSubject` 找到这名玩家本人，最多取 10 场。
3. 本地缓存 45 秒（`livePlayerMatchesCacheTTL`）。

所以只要本机客户端对"别人的战绩"返回的是旧列表（缺最新一局），名单就会缺这一局，而且不会去 SGP 补。

## 为什么 R178 的取证不能证明"不陈旧"

`live_history_freshness` 里 `lcu_missing_newer` 全是 0，但：

- 每局只抽 2 名非本人玩家（`scope.samples < 2`）。用户说的"中路、下路"很可能根本没被抽到。
- 抽样用的是 `a.sgp.matchHistory(…, useCache=true)`，命中 SGP **5 分钟**页缓存（`sgp_api.go:82 sgpCacheTTL`）。拿可能同样陈旧的缓存去比，比不出差别。
- `liveMissingNewerGames` 只比"所有模式里的最新时间"，没有专门比**当前队列**。

## P1　取证：每名可查询玩家都比对，SGP 不走缓存

`live_history_freshness` 改为：**每局每个有身份的玩家（含本人）都记一条**，不再限 2 人。

- 比对用的 SGP 请求**绕过页缓存**（`useCache=false`），同一局每个玩家只比一次。
- 新增字段（仍不记身份、matchID、gameID）：
  - `slot`：该玩家在本队的序号（0–4），配合 `team` 用来对应截图里的哪一行。
  - `lcu_newest_queue_age_min` / `sgp_newest_queue_age_min`：两边当前队列最新一局距今分钟数。
  - `missing_newer_in_queue`：SGP 当前队列里比 LCU 当前队列最新一局更新、而 LCU 没有的场数。
  - `missing_newer_any`：同上，不限队列（即现有 `lcu_missing_newer` 的口径）。
  - `subject_unmatched`：LCU 窗口里当前队列的对局，因为 `recentMatchSubject` 找不到本人而被跳过的场数。
  - `remake_skipped`：当前队列里因非胜负被跳过的场数。
  - `shown_newest_age_min`：最终显示的 10 局里最新一局距今分钟数。
- SGP 失败只记 `sample_failed`，不影响名单。

## P2　修复：非本人玩家合并客户端与 SGP 两份战绩

不再等"证明陈旧"才切换，直接让两份数据互补，保证最新一局不会因为任一来源滞后而缺失。

- 非本人、有身份的玩家：并行读取 LCU 30 场和 SGP SUMMARY 30 场（**SGP 不走 5 分钟页缓存**，避免缓存本身成为陈旧来源；仍受本地 45 秒名单缓存和同一玩家请求合并的保护）。
- 按 `GameID` 合并去重，同一 `GameID` 优先保留字段更全的一份（能找到本人、带胜负、带队列）。合并后按 `CreatedAt` 倒序，再交给现有 `recentMatchesForPlayer`。筛选规则（当前队列、胜负局、最多 10 场）**完全不变**。
- 执行时核对两个来源的 `GameID` 是同一套数值（LCU `gameId` 与 SGP 转换后的 `GameID`）。如果有前缀或区服差异，统一后再去重，并为此加测试。
- SGP 不可用或失败：只用 LCU，与现在一致。LCU 失败而 SGP 成功：用 SGP（现有兜底保留）。
- 本人仍用 `current-summoner`（日志显示本人数据是新的：第 2 场选人时，本人当前队列最新一局就是 22 分钟前刚打的那局），不做合并。
- 移除 R178 的 `preferSGP` 切换逻辑（被本条取代），但保留它的测试意图：缺最新一局时，最终显示里必须有这一局。
- 请求量：每局最多 9 次 SGP SUMMARY，选人开始时一次、进入对局时一次（敌方此时才有身份），之后按 45 秒缓存。不得在每 3 秒轮询时都发。

## P3　显示：缺失时不要静默

不新增任何说明文字（红线），但"近 N 局"必须如实反映合并后的结果。合并后某玩家当前队列最新一局时间变了（例如进入对局后再次加载拿到了新的一局），按 R179 P4 的行级局部更新，只更新这一行。

## 测试

Go：

1. LCU 窗口缺当前队列最新 1 局、SGP 有：最终 10 局包含这一局且排在第一位，`missing_newer_in_queue=1`。
2. 两边都有同一局：去重后只出现一次，场数、胜负、KDA 不重复计算。
3. SGP 失败：结果与只用 LCU 完全一致，`sample_failed=true`。
4. 抽样 / 合并的 SGP 请求不读、不写 5 分钟页缓存（或使用独立的短缓存），用缓存陈旧的夹具证明结果仍是最新。
5. 本人：不发 SGP 合并请求，结果不变。
6. 选人阶段多次轮询：每个玩家 45 秒内只发一次 SGP 请求。
7. `subject_unmatched`、`remake_skipped` 计数正确。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 合并时不去重 | 测试 2 FAIL |
| SGP 读取改回 `useCache=true` 且使用陈旧夹具 | 测试 4 FAIL |
| 去掉合并（恢复只用 LCU） | 测试 1 FAIL |

`node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R180；R178 那行备注"P2 结论由 R180 取代"；账本 `docs/history/ledgers/r180-execution-ledger.md`。
- 不改队列筛选、不改"近 10 局"上限、不新增界面文字。

## 真机验收（用户）

1. 排位选人时，挑一个你确定刚打完同模式对局的队友（或用小号打一把后再和它组同一模式），名单里这名玩家的第一张战绩卡就是那一局。
2. 打完导出日志：每个有身份的玩家都有一条 `live_history_freshness`。如果哪一行仍缺最新一局，记下是哪一队第几个，我用 `team + slot` 对照。
