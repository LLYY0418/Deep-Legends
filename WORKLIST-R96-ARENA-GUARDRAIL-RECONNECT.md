# WORKLIST-R96 — 把 arenaAlliesCorroborate 护栏接回生产调用链

## 背景

WORKLIST-R95 P0 要求："不改 `arenaAlliesCorroborate` 的否决逻辑（它是对的，继续当护栏用），只是**额外**把它的
输入拿来正向展示。" R95 执行后独立验收发现：`arenaAlliesCorroborate`（`arena_live_grouping.go:125`）函数体本身
确实字节未改，专属测试 `TestR95LegacyAlliesCrossBlocksGuardRemainsIndependent` 也仍在且独立通过——但
`applyArenaLiveGrouping`（`arena_live_grouping.go:207-290`）已经**不再调用它**，生产调用链上零调用点。

也就是说"不改否决逻辑"这半句字面上兑现了，但"继续当护栏用"这半句实质没有兑现——函数变成了一段只被自己
的单元测试验证、生产代码路径完全不会经过的死代码。

## 为什么现在看起来无害，但仍是真实缺陷

当前没有任何已知 Riot 客户端版本会在直播数据里暴露真实 `subteamId`/`playerSubteamId` 字段（R95 已用三方独立
信源证实：项目自己的日志、社区 hasagi-types 完整 LCU schema、多个独立第三方 spectator-v5 客户端库字段枚举，
均无此字段），所以这条护栏此刻确实不会被触发，也就不会导致任何用户可见的错误。

但 `gameplay.go:4593-4643` 的字段嗅探逻辑依然在，一旦某天 Riot 真的在某个接口里加了这类字段，现有代码会
**直接信任它、只做结构合法性检查**，不会再用已经 100% 确定的 3 名真实队友身份去交叉验证——这正是 R95 工单
最初想要双重把关、明确要求保留的那一类"未经验证的推断"风险。这是一个休眠的回归风险，不是当前故障，但应该
趁着这次发现顺手接回，而不是等哪天真出问题时再排查一轮。

## 要做的事（范围很小，不要借机改动分组算法本身）

1. **把 `arenaAlliesCorroborate` 接回 `applyArenaLiveGrouping`**：在字段嗅探路径（`gameplay.go:4593-4643` 那条
   逻辑，一旦检测到真实 subteamId 类字段）产出候选分组结果之后、正式采纳之前，用已知的 3 名"我的小队"成员
   （`rememberArenaAllies` / `markRememberedArenaSquad` 已经在维护的那份记忆）去调用 `arenaAlliesCorroborate`
   做交叉验证；验证不通过就必须按现有 `allies-cross-blocks` 语义拒绝，不能采纳。
2. **不要**：不要改 `arenaAlliesCorroborate` 函数体本身的否决逻辑（工单验收过是对的）；不要把这条护栏改成
   可绕过/可降级的软校验；不要因为"现在没有真实字段所以测试很难写"就用 mock 出一个假字段类型敷衍了事——
   要真的在字段嗅探成功产出候选分组的那个分支里插入调用，哪怕目前永远不会被真实数据触发到。
3. **修复 `scripts/r90-mutation-check.py:33`**：这个变异检测脚本硬编码的源码行
   `verified, rejected := arenaAlliesCorroborate(groups, response.Players, remembered, squadSize, grouping, current.PUUID)`
   在当前 `arena_live_grouping.go` 里已经不存在了（本条目修完后，正确的调用点行文本会变化，脚本要跟着更新，
   否则这个变异检测本身就是在检测一个不存在的目标，形同虚设）。
4. **顺手清理 `finishArenaGroupTruth` 的日志噪音**（`arena_live_grouping.go:430-458`）：如果一局游戏收到过
   多条真实但不完整的 SGP 响应，其中恰好有一条无关参与者的 subteamId 永久畸形，当前逻辑仍可能在重试耗尽后
   吐出一条误导性的最终"none"事件（明明收到过部分真实数据，却报告成完全没收到）。这个只影响诊断日志的可读性，
   不影响任何生产判定结果，优先级低于第1、3条，如果时间不够可以放到下一轮。

## 验收要求

- 新增一个真实的对抗测试：构造一个"字段嗅探成功、候选分组与已知3人小队冲突"的 fixture，断言最终结果被
  `allies-cross-blocks` 正确拒绝，且这条路径**真的**经过了 `applyArenaLiveGrouping` 而不是直接调用
  `arenaAlliesCorroborate` 的单元测试（后者已经存在，不能算数——问题正是这个函数没有从生产路径被调用到）。
- `scripts/r90-mutation-check.py` 重跑一遍，确认它现在检测的是真实存在于源码里的那一行。
- 常规回归：`go test -count=1 -v .`、`go test -race ./...`、`node --test web/*.test.cjs desktop/*.test.cjs`
  全部真实跑一遍且全绿，附带真实日志文件，不要只写"通过"两个字。
- 不要求真机验证（这条护栏本来就要等 Riot 哪天真的加字段才会被触发），fixture 级别的对抗测试即可。
