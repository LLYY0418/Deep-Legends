# WORKLIST-R170：对局详情「近 N 局」汇总统计没有跟着封顶到 10（R168 P4-2 验收缺口）

诊断人：Claude（只读代码核对 R168/R169 的实际改动，未做实机操作）。
执行人：GPT。
日期：2026-09-26。
来源：验证 R168、R169 的落地代码时发现；用户明确要求"详情的战绩保持只展示最新的 10 条，30 条太多了"。

按约定，这是对**已执行**工单（R168 P4-2）验收时发现的新缺口，单独开号，不追加进 R168。

---

## 结论先行

R168 P4-2 的主体修复是对的：`loadLivePlayerMatches` 把原始抓取窗口从 10 场（不分队列）放宽到了 30 场（`backend/gameplay.go` 新增 `const liveHistoryWindow = 30`），这解决了"混队列玩家近期战绩被漏抓、显示局数偏少"的问题。**真正展示的逐局记录 `RecentGames` 也确实还是老老实实封顶在 10 场**（`recentGamesFromMatches(matches, playerRef, 10, response.QueueID)` 调用处没有改成 30，符合预期）。

但同一份改动里，用来生成"近 N 局"汇总数字（胜率/场次/KDA 那一行，`modeStats.Games`，对应截图里"近 1 局"/"近 6 局"这种文案）的 `aggregateMatches(matches, playerRef, ...)`，用的是**没有再截断的完整 30 场窗口**——之前窗口本来就只抓 10 场，所以这个统计天然不会超过 10；现在窗口放宽到 30 场之后，`aggregateMatches` 没有跟着加一道"最多数最近 10 场"的限制，导致队友如果 30 场里同队列的局数够多，这行汇总就会真的显示成"近 30 局"，和用户想要的"最多 10 局"不一致，也和旁边 `RecentGames` 列表实际展示的局数（最多 10 条）对不上——这是两处口径没对齐。

---

## 代码证据

`backend/gameplay.go`（`loadGameplayLive` 内，约 7647-7661 行）：

```go
modeStats := aggregateMatches(matches, playerRef, func(match gameplayMatch) bool { return response.QueueID == 0 || match.QueueID == response.QueueID })
...
recentGames := recentGamesFromMatches(matches, playerRef, 10, response.QueueID)
```

`matches` 是 `loadLivePlayerMatches` 返回的最多 30 场原始记录（不分队列）。`recentGamesFromMatches` 内部会先按 `CreatedAt` 倒序排序、按 `queueID` 过滤、再取前 10 场——所以它的输出天然是"最近 10 场同队列对局"。而 `aggregateMatches` 只做了 `queueID` 过滤（通过 `include` 回调），**没有排序、也没有数量上限**，会把全部 30 场原始记录里符合队列的都算进胜率/场次统计。

也就是说：一个玩家如果这 30 场原始记录里有 18 场落在当前队列，逐局图标只会显示 10 张（`RecentGames`），但旁边"近 N 局"的文案会显示"近 18 局"——数字比实际展示的图标数还多，用户看到的就是不一致、"局数太多"的观感。

`backend/web/gameplay.js:5432`：
```js
: `<dl><div><dt>${recordGames ? `近 ${number(recordGames)} 局` : "当前模式"}</dt> ...
```
`recordGames` 直接来自后端的 `modeStats.games`，前端没有再做任何封顶，是纯透传。

---

## 修复方向

在 `aggregateMatches` 之前，对参与统计的 `matches` 做和 `recentGamesFromMatches` 一致的处理：按 `CreatedAt` 倒序排序、按队列过滤后，只取最近 10 场，再喂给 `aggregateMatches`。两种可行写法（任选其一，哪种改动小用哪种）：

- **方案 A**：给 `aggregateMatches` 增加一个"最多统计场数"的参数（或者新写一个小helper，复用 `recentGamesFromMatches` 里"排序+按队列过滤+取前 N"这段逻辑，返回过滤后的 `[]gameplayMatch` 而不是 `[]gameplayRecentGame`），调用处传 10，和 `recentGamesFromMatches(matches, playerRef, 10, response.QueueID)` 用同一个 10。
- **方案 B**：更直接——既然 `recentGamesFromMatches` 已经算出了"最近 10 场同队列"的结果集，`modeStats` 干脆基于 `recentGames`（而不是原始 `matches`）重新聚合胜负/KDA，保证两者口径完全一致、不会出现"文案局数 > 图标局数"的情况。这样以后 `recentGames` 的取数逻辑改了，`modeStats` 会自动跟着一致，不用两处分别维护"最近 10 场"的定义。

推荐方案 B：从数据源头保证"文案局数"和"实际展示的逐局图标数"永远是同一批数据算出来的，避免以后再出现类似口径不一致。

**注意**：`liveHistoryWindow = 30` 这个抓取窗口本身不用动——它是用来在"混队列玩家"场景下抓够同队列的候选局数的，缩回 10 会重新引入 R168 P4-2 本来要修的"局数漏抓"问题。这次只需要把"展示口径"（`modeStats` 的统计范围）跟 `RecentGames` 对齐到同一个"最近 10 场"，不要改抓取窗口。

---

## 测试要求

- Node/Go 测试：构造一个玩家的 `matches`，让某个队列下的局数超过 10（例如 15 场同队列历史局），断言修复后 `modeStats.Games <= 10` 且等于 `len(recentGames)`（用方案 A 或 B 都要满足这条恒等关系）。
- 补一条回归断言：`matches` 少于 10 场时（比如只有 3 场同队列），`modeStats.Games` 应该等于实际场次数（3），不要被误伤成固定 10 或 0——只是"上限 10"，不是"强制 10"。
- 如果采用方案 B（modeStats 基于 recentGames 重新聚合），确认涉及胜率/KDA/CS 计算的字段（`Kills`/`Deaths`/`Assists`/`CS`/`CSPerMinute` 等）在 `gameplayRecentGame` 结构体里是否有 `aggregateMatches` 需要的全部字段（比如 `Duration`、`CS`）；如果 `gameplayRecentGame` 缺字段，要么给它补上，要么改用方案 A。

---

## 验收标准

1. 对局详情里任意队友/敌方玩家的"近 N 局"文案，N 不超过 10，且与该玩家旁边实际展示的逐局战绩图标数量一致。
2. 局数不足 10 场时如实显示实际场次，不做任何凑数或误伤。
3. R168 P4-2 原本要修的"局数漏抓"效果不能回退——用一个混队列玩家（比如最近打了几把匹配、几把排位）的场景验证：同队列局数仍然能抓够（不再卡在"抓 10 场里凑不出几场同队列"的老问题），但展示的局数和文案封顶都是 10。

---

*本工单由 Claude 基于代码只读核对完成，未做任何实机操作，未修改任何源码。*
