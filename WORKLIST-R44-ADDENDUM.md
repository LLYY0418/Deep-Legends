# WORKLIST-R44-ADDENDUM（给 GPT 执行，P0 插队）

> **斗魂英雄选择阶段完全不可用**：我方栏 5 个空壳"隐藏玩家"、对方栏空、
> 出装页永远停在"请先选定英雄"。**这两个现象是同一个根因链，而且第一环是我在 R43-I1 引入的回归。**
>
> 请优先于 R44 主工单处理这一组。

---

## 根因链（日志已完整锁定，不用再猜）

斗魂那局（game_id 8958100781, queue 1750, ChampSelect）的 `live_roster_shape`：

```json
{"phase":"ChampSelect","players":5,"raw_count":0,"merge_appended":5,
 "empty_ref_count":5,"duplicate_player_refs":0,"team_counts":{"100":5},"with_stats":0}
```
配对的 `live_roster_rendered`：`{players_received:5, rendered_100:5, rendered_200:0}`
同一秒：`live_recommendations_skip {queue_id:1750, reason:"no-target"}`

**对照组**：同一份日志里峡谷/海斗的 ChampSelect 全都是
`players=10 raw=10 appended=0 emptyref=0`，完全正常。所以这是斗魂特有的。

链条：

1. **`raw_count: 0`** —— 斗魂英雄选择阶段 gameflow session (`/lol-gameflow/v1/session`)
   **一个玩家都不给**（峡谷/海斗在同阶段是给满 10 个的）。
2. **`merge_appended: 5` 恰好等于 5 人上限** —— 名单全部来自 champ-select session，
   走 `mergeChampSelectPlayers` 的 `index<0` append 分支，**被 `teamCount >= 5` 闸门截断**。
3. **`empty_ref_count: 5`** —— 这 5 条记录的 `puuid` 和 `obfuscatedPuuid **两个键都是空**
   （正则 `^[A-Za-z0-9_-]{16,128}$` 不是元凶，它允许连字符、36 位 PUUID 和混淆串都能过；
   而且 `normalizeGameplayReference:1572-1574` 还有"主 ref 空就用备用 ref"的兜底，
   两条路都没救回来 → 只能是两个键都空）。所以图一那不是隐私屏蔽，**是空对象被当成玩家渲染了**。
4. **身份全空 → `isCurrent` 恒 false** —— 后端 `gameplay.go:4013`
   `isCurrent := gameplayReferenceContains(reference, current.PUUID)` 要求精确匹配本机 PUUID。
5. **→ 前端 `liveRecommendationTarget()`（`web/gameplay.js:2912`）第一行
   `find(player => player.isCurrent)` 返回 undefined → championId=0 → 返回 null
   → `no-target` → 出装页永远停在"请先选定英雄"**。全前端没有任何兜底，`isCurrent` 是唯一途径。

**连带影响（用户还没报但必然也是坏的）**：`gameplay.go:4064-4069` 的服务端符文/技能加载
同样以 `player.IsCurrent` 为闸门；`validateGameplayItemSetContext:4722-4750` 用同一套匹配
→ **斗魂里"一键应用装备"也必定报"英雄选择中没有找到当前账号"**。

---

## A 组（P0）：拆掉 5 人上限 —— 这是我 R43-I1 写错的前提

`gameplay.go:4213-4226` 现在是：

```go
// Champ-select can expose a hidden duplicate while gameflow already
// has the full five-player team. Do not grow the roster past the
// Summoner's Rift/ARAM team bound; large arena rosters already have
// more than five gameflow entries and therefore remain untouched.
if teamCount >= 5 {
    continue
}
```

**注释里那句"large arena rosters already have more than five gameflow entries and
therefore remain untouched"就是错的前提**——`raw_count: 0` 证明斗魂英雄选择阶段
gameflow 一条都不给，`result` 从空开始计数，于是这个本来只想拦"重复行"的闸门
把斗魂的真实名单从第 6 条起全部静默丢掉了。`merge_appended` 恰好等于 5 不是巧合。

这个闸门是 R43-I1 我要求加的（当时为了修"英雄选择阶段多出重复行"），
**我写工单时假设了斗魂一定有 gameflow 名单，这个假设没有验证过，是我的错。**
但闸门本身要解决的重复行问题是真实的，所以不能简单删掉。

**改法**：把上限和"gameflow 是否已经给出名单"绑定——
只有 `raw_count > 0`（即 gameflow 已提供基准名单、champ-select 只是来补充/去重）时
才应用 5 人上限；`raw_count == 0` 时 champ-select 就是唯一数据源，不设上限
（或者设一个足够大的、按模式来的上限）。

**必须补测试**：`raw_count==0` + champ-select 给 6 条以上 → 断言全部保留；
`raw_count==5` + champ-select 给重复行 → 断言仍然拦住（R43-I1 那个场景不能回归）。
两条都要，只写一条等于没修。

---

## B 组（P0）：补 `localPlayerCellId` / `cellId`，让"识别自己"不再依赖 PUUID

**全仓库 grep `localPlayerCellId|cellId|CellID|CellId` 零命中**——我们从来没解析过这两个字段。

这是 LoL 英雄选择的**通用模型**（与队列模式无关）：session 顶层的 `localPlayerCellId`
直接告诉你哪个 cell 是自己，每个 cell 有 `cellId`。拿到之后可以在 `gameplay.go:4013`
的 `isCurrent` 判据上追加一条 `raw.player.CellID == champSelect.LocalPlayerCellID`，
**完全绕开身份系统**——即使 puuid 全空也能认出自己。

改动点：`lcuChampSelectSession`（`gameplay.go:3835-3839`，现在只有 `gameId/myTeam/theirTeam`
三个字段）加 `localPlayerCellId`；`lcuChampSelectPlayer`（`:3849-3862`）加 `cellId`；
`mergeChampSelectPlayers` 构造 `lcuLivePlayer` 时（`:4230`）透传；`:4013` 加判据。

> ⚠️ **顺序很重要**：如果 A 组的 5 人截断丢掉的正好包含我们自己那一条
> （本机玩家在 `myTeam` 里下标 ≥5），那 cellId 判据也会落空。
> **必须先做 A 组再做 B 组，两者缺一不可。**

---

## C 组（P1）：加 `/lol-champ-select/v1/current-champion` 作为最后兜底

即使一个玩家都识别不出，这个接口也能直接返回本机已锁定的英雄 ID，
足以驱动"出装与技能"页 —— **完全不依赖名单结构**，是图二那个问题最稳的兜底。

全仓库零引用。请加上，作为 `liveRecommendationChampionId` 链路在
"名单里找不到 isCurrent"时的降级路径。

---

## D 组（P0）：给 `/lol-champ-select/v1/session` 加结构探测

**明确指出：这个接口我们从来没有任何结构探测埋点。**
全仓库 `sync.Once` 型 shape 埋点只有三处（`lcuGameflowShapeOnce`、`rankedLCUShapeOnce`、
`sgp.rankedShapeOnce`），champ-select 那边只在 `gameplay.go:3978` 记了一个
`Count: len(MyTeam)+len(TheirTeam)` 的能力位，而且**这个数字没有进 `live_roster_shape`**，
导致我这次想用它反推 myTeam 真实长度都做不到。

A/B/C 三组能修掉"出装页停在请先选定英雄"（图二），但**修不掉"我方栏 5 个空壳玩家"（图一）**
——因为我们不知道那 5 条空对象到底是什么、斗魂真实的 18 人身份藏在哪个键里。这段必须有真机结构。

请仿照 `gameplay.go:3900-3912` 的写法加一个 champ-select shape 探测，打这些内容：
- `top_level_keys`（session 顶层所有键）
- `myTeam` / `theirTeam` 的**键并集**
- 两个数组的**长度**
- 每个键的**非零值计数**（用来判断是"键存在但值为空"还是"键根本不存在"）
- **`game_mode` 和 `queue_id`**（这次的教训，见下）

> ⚠️ **绝对不要用整进程 `sync.Once`。** R44 主工单 J 组已经吃过这个亏：
> `lcuGameflowShapeOnce` 是整进程一次，结果打在了第一局海斗上，
> 用户后来打了多少局斗魂都不会再记录。**按 `queueId` 或 `gameMode` 去重，并加上限轮转。**
> 这次 J 组要求的探测改造和这条是同一件事，可以一起做。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **A 组两条测试都要写**（不设上限的新场景 + R43-I1 重复行的老场景不回归）。
- **变异测试**：A 组（把上限改回无条件 `>=5`，新测试必须 FAIL）、
  B 组（去掉 cellId 判据，`isCurrent` 必须识别不出来）两处必做。
- D 组只加观测，不要基于猜测写解析。
- 改完后请用户**再开一局斗魂**复现，导出日志。这次重点看：
  ① `live_roster_shape` 的 `merge_appended` 是否还卡在 5、`empty_ref_count` 是否下降；
  ② 新增的 champ-select 探测有没有落盘（确认按模式去重生效，不是又打在别的模式上）；
  ③ 出装页是否能出数据。
