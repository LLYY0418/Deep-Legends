# WORKLIST-R43-ADDENDUM-2（给 GPT 执行）

> 上一份 ADDENDUM 的三组里，**J 组基本没有推进，且新增了一段无效的死代码**，
> 是本轮最需要重视的问题。K 组完成度高（六项里五项半是真的）。L 组只完成了一半。
> 这份补充工单只列还没解决的部分，K 组已经做对的五项半不用再动。

---

## J 组（P0，仍未解决）：斗魂对局页 subteam —— 请先老实等真机日志，不要再加脚手架代码

**上一轮验收明确结果：J 组没有做实际修复。** 探测埋点 `lcu_gameflow_session_shape`
（`gameplay.go:3872-3884`）原样保留、没有任何改动，全仓库搜不到任何"根据真机日志确认
字段名是 XX"的痕迹。`lcuLivePlayer`/`lcuChampSelectPlayer`/`gameplayLivePlayer` 都还
**没有 subteam 字段**，`gameplay.go:3921/3927` 依旧写死 `team:100`/`team:200`，
前端 `renderLiveInsights`（`web/gameplay.js:3441-3452`）依旧硬编码"我方"/"对方"。
**用户报的"斗魂 17 人全在 team 100"的原始症状完全没修。**

**这轮新增的代码需要先处理一下**：前端加了 `recordArenaTeamMappingProbe`
（`web/gameplay.js:3146-3159`）和调用点（`:3445`），试图读取 `data.players[].subteamId`
并上报探测事件核对 `ARENA_TEAM_MASCOTS` 队伍名映射。**但因为后端响应里根本不存在
`subteamId` 这个字段**（第2点还没做），`Number(player?.subteamId)` 恒为 `NaN→0`，
`.filter(id => id > 0)` 直接把结果清空，函数在 `if (!groups.length) return;` 处
直接返回——**这段代码永远不会真正上报任何数据，是死代码**。看起来像是在推进任务，
实际上只是加了一层依赖尚未解析字段的探测脚手架，没有真正往前走一步。

**这轮请这样做，顺序不能变**：

1. **先确认后端 `lcu_gameflow_session_shape` 埋点有没有真机日志回传**。
   如果还没有，请明确告诉用户"需要一份打过斗魂对局的诊断日志"，
   **在拿到日志、确认字段名之前，不要再写任何依赖这个字段的代码**（前端后端都不要）。
2. 拿到真机日志、确认字段名后，按顺序补：
   `lcuLivePlayer`/`lcuChampSelectPlayer` 加字段 → `gameplayLivePlayer` 加 `SubteamId`
   → `gameplay.go:3921/3927` 改成按 subteam 分组而不是写死 100/200
   → 前端 `renderLiveInsights` 改成按小队分组渲染（参考已经做对的战绩详情页
   `matchPlayerGroups`，`web/gameplay.js:1827-1833`，当前玩家所在队伍排第一）。
3. 这一步做完之后，`recordArenaTeamMappingProbe` 才有意义去核对
   `ARENA_TEAM_MASCOTS`（`web/gameplay.js:1790-1800`）那份队伍名映射；
   在第2步完成之前，这段探测代码建议删掉或者加个"字段不存在直接跳过不上报"的判断，
   **不要让它看起来"已经在工作"但实际什么都不做**——这种代码比不写还容易让人误判进度。

---

## M 组（P2，K 组唯一未完成的一项）：G-1 的 15 条截断护栏仍是假的

K 组六项里五项半已验证为真护栏，**唯一没做对的是"15 条截断改 0"这一半**。
现有新增测试 `TestGameplayRecommendationsPreserveAllArenaAugments`
（`gameplay_test.go:2458`）直接把 16 条 `ArenaAugments` 注入
`championDetailResponse.ArenaAugments`，**绕过了真正做截断的那个调用点**
（`champions_structured.go:1288` 的 `flattenArenaAugmentGroups(response.ArenaAugmentGroups, 0)`）。
变异测试证实：把这一行的 `0` 改回 `15`（真实回归场景），现有测试依旧 PASS，抓不到。

请补一条**真正覆盖到 `champions_structured.go:1288` 这个调用点**的测试
（直接调用 `flattenArenaAugmentGroups` 或者构造一个走到这行代码的完整请求路径），
断言 limit 参数确实是 0、不会把分档数据截断到 15 条以内。

---

## L 组（P1，仍未完成的两点）

**L-1（根因排查）：完全没做。**
全仓库搜索不到任何针对"`ranked_winrate_resolved` 每秒 7 次、恒为空数据却标
`complete:true`"这个问题的排查痕迹——没有新增注释、没有诊断字段、没有测试。
`rank_insights.go:325-330` 的 match-tiers 请求确实会对 `PlayerRefs` 去重后
逐个不同玩家查一次，这个结构本身**支持**"多个不同玩家各查一次、载荷恰好长得一样"
这个假设，但这是老代码的既有结构，不是这轮排查得出的结论，也没人把结论写下来、
没有给诊断 payload 加区分玩家的字段（如 playerRef 的哈希）来验证这个假设。

请这轮真正做一次排查，并且**把结论落到代码里**（哪怕只是给
`ranked_winrate_resolved` 的诊断 payload 加一个玩家标识哈希字段，
用来验证"是不同玩家还是同一玩家反复查"），不要停留在猜测。

**L-2（30%占比护栏测试）：覆盖面不够，需要补一种场景。**
现有测试 `TestDiagnosticEventShareStaysBelowThirtyPercent`（`gameplay_test.go:2772`）
变异测试证实是真的——把去重机制 `isNoisyDiagnosticEvent` 改成恒 false，测试会 FAIL。
**但这条测试的 100 次调用用的是完全相同的 payload**，天然会命中已有的"逐字节相同
payload 去重"（H-1），所以它验证的是"H-1 机制没被删掉"，**不是**工单真正想防的
"新的刷屏事件，各次 payload 不完全相同（比如带了不同玩家的字段），累计占比超标"
这种更贴近现实的场景。

请追加一个测试用例：同一事件名，200 次调用每次 payload 里带一个不同的字段
（比如 `player_index` 递增），验证：即便 payload 逐次不同、无法被 H-1 的
字节级去重拦住，仍然有别的机制能把这类事件的占比控制住，或者至少测试能
**明确报警**"这种场景下占比超标了"——不需要现在就实现新的限流机制，
但至少要让这类场景在测试里可见，不能是盲区。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **J 组最优先，且必须先拿到真机日志再动代码**——这条已经连续两轮工单都在原地打转，
  下一轮如果还是没有真机数据支撑就继续写代码，请明确告知用户"卡在等日志"，
  不要再加不会生效的脚手架代码。
- M/L 组都是补测试性质，不需要改动已经验证过的功能代码。
