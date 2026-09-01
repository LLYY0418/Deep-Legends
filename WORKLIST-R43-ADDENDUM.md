# WORKLIST-R43-ADDENDUM（给 GPT 执行）

> R43 主体验收完成，质量整体很高——十个组里没有发现"假装做了"的情况，
> 功能代码基本都是真实现。但验收过程中用真变异测试挖出了两类问题，需要这份补充工单收尾：
>
> 1. **F-2 是唯一一个功能本身没做完的**（不是测试问题，是真的没修完），优先级最高。
> 2. **多组的测试护栏是假的**——代码写对了，但测试测不出"如果有人手滑改回旧逻辑"这种回归。
>    这类不需要重新设计，只需要把断言补上。

---

## J 组（P0）：F-2 补完 —— 斗魂对局页 subteam 字段解析 + 队伍分组渲染

上一轮 F-2 只做了"先加探测埋点"这一步（`gameplay.go:3872-3884` 的
`lcu_gameflow_session_shape` 一次性探测，把 `/lol-gameflow/v1/session` 的原始字段名打进日志）——
**这一步做得对，没有凭猜编字段名**，但工单要求的实际修复还没做：

- `lcuLivePlayer`（`gameplay.go:3788-3805`）和 `lcuChampSelectPlayer`（`:3821-3834`）
  **都还没有 subteam 字段**；
- `gameplay.go:3921/3927` 依旧把 teamOne 写死 100、teamTwo 写死 200；
- `gameplayLivePlayer` 结构体（`:3657-3673`）没有 `SubteamId`；
- 前端 `renderLiveInsights`（`web/gameplay.js:3441-3451`）依旧硬编码
  `team(100,"我方")`/`team(200,"对方")`。

**也就是说用户报的"斗魂 17 人全在 team 100"这个原始症状目前完全没修，
如果现在发版，斗魂对局页对方栏依旧是空的。**

请这样收尾：
1. 先看探测埋点 `lcu_gameflow_session_shape` 有没有真机日志回传（找用户要一份斗魂对局的诊断日志），
   确认 `/lol-gameflow/v1/session` 里 player 对象实际带的 subteam 相关字段名
   （候选：`playerSubteamId`、`subteamId`，具体以真机日志为准，不要凭猜）。
2. 确认字段名后，给 `lcuLivePlayer`/`lcuChampSelectPlayer`/`gameplayLivePlayer` 补上字段，
   `gameplay.go:3921/3927` 改成按 subteam 分组而不是写死 100/200。
3. 前端 `renderLiveInsights` 改成按小队分组渲染（可以参考已经做对的战绩详情页
   `matchPlayerGroups`，`web/gameplay.js:1827-1833`，当前玩家所在队伍排第一）。
4. `ARENA_TEAM_MASCOTS`（`web/gameplay.js:1790-1800`）那份 subteamId→队伍名的映射
   同样需要用真机数据核对，不要沿用未经验证的猜测表。

---

## K 组（P1）：给"功能真实现但护栏是假的"六处补测试

以下六处**功能代码本身是对的，不需要重做**，只是变异测试证实"如果有人以后手滑改回旧逻辑，
现有测试完全抓不到"。请逐条补一条能锁住具体实现的断言（属性值/结构/调用参数），
不要只加"函数存在"这种弱断言。

1. **B-1 战绩卡对齐**（`web/gameplay.css`）：`.match-stats` 的
   `align-content:start` + `grid-template-rows:repeat(3,18px)`、`.match-tier-value`
   的 `min-height:19px`、`.match-champion`/`.match-loadout-mini` 的 `min-height:63px`
   三处都改回旧值后现有测试全绿。请补 CSS 结构性断言锁住这三个具体属性值。

2. **C-2/C-3 优劣势对抗等宽**（`web/gameplay.css:1011` 附近）：`.champion-matchups > div`
   的 `grid-template-columns: 58px repeat(3,minmax(0,1fr))` 改回 `display:flex` 后
   现有测试全绿。请补一条断言这条 grid 规则存在（正则或结构解析均可）。

3. **D-1 能力位运行时行为**：`champions.test.cjs` 现有测试只对
   `renderChampionRecommendationHeader` 的函数源码做正则匹配，从未真正传入
   `hasCounters:false`/`hasBanRate:false` 的数据跑一遍断言渲染结果。
   请补至少一条真正调用函数、传入 false 值、断言对应 DOM 片段确实不出现的测试。

4. **D-4 主升技能高亮**：`.is-priority` 类和对应 CSS 高亮目前零测试覆盖
   （`renderSkillPlan` 相关逻辑，`web/gameplay.js:3745` 附近）。补一条断言主升技能芯片
   带 `.is-priority`、非主升不带的测试。

5. **G-1 斗魂海克斯三列 + 15 条截断改 0**：把斗魂渲染改回单列 `slice(0,12)`，
   或把 `flattenArenaAugmentGroups` 的 limit 改回 15，现有测试全绿。
   请分别补断言：斗魂必须走三列结构（同海斗）；`ArenaAugments` 透传时不应被截断到 15 条以内。

6. **I-1 对局名单去重的人数闸门**：`gameplay.go:4185-4198` 新加的
   "该队人数已达 5 就不再追加"闸门，是目前看来真正能拦住重复行问题的机制，
   但**零测试覆盖**。请补一条测试：构造已有 5 名队友 + 1 条无法匹配身份的
   champ-select 记录，断言合并后人数仍是 5（不会变 6）。

   **另外说明一点，不算缺陷但要知会**：工单当初点名要改的"候选侧对称化"那一行
   （`visibleLivePlayerReference(result[candidate].player)`）经变异测试证实是**死代码**——
   `NameVisibilityType` 字段全代码库只在 champ-select 侧被赋值，gameflow 原始构建路径
   从不写这个字段，所以候选侧的反混淆函数永远退化成裸 `.PUUID`，改不改这一行结果都一样。
   这不是本轮需要修的问题，只是希望 GPT 知道"重复行真正被拦住"靠的是人数闸门，
   不是最初设想的对称化那一行，避免以后有人以为对称化在生效而依赖它。

---

## L 组（P1）：H 组收尾 —— 根因排查 + 占比护栏测试

H-1（载荷级去重）已经是真实现，变异测试证实有效，这条不用动。但还有两条工单要求的事没做：

1. **排查"为什么 `ranked_winrate_resolved` 每秒 7 次、且恒定
   `wins:0/losses:0/tier_empty:true` 却标 `complete:true`"**。目前代码里没有任何调查痕迹。
   请顺着调用链 `loadRanksWithFallback` → `playerRankScore`（`rank_insights.go:171`）确认：
   这是"很多个不同玩家各自查一次、载荷恰好长得一样"（属正常，只是巧合），
   还是真的有什么轮询在反复重新解析同一份空数据（属真 bug）。
2. **补一条"单一事件名在一个轮转周期内占比不得超过 30%"的护栏测试**——
   全仓库目前搜不到任何按事件名统计占比的测试。这条测试的意义是防止以后又出现新的
   刷屏事件而没人发现（这已经是第三次同类问题了）。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- J 组是本轮唯一的功能缺口，**必须等真机日志回传字段名后再写解析，不要在没有真机数据的情况下继续猜**。
- K 组六处都是"补断言"性质的工作，不需要改动现有功能代码（I-1 那条例外说明除外）。
- 改完后请用户重启应用，重点针对 J 组（斗魂对局页）和 L 组（导出日志确认占比是否正常）复现验证。
