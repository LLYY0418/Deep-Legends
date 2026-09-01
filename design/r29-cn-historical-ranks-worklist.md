# R29 工单：国服「以往赛段」——把已经下载却被丢掉的字段用起来

> 出工单人：Claude ｜ 日期：2026-08-26
> 依据：LeagueAkari 源码审计 + 真机日志 `diagnostics.jsonl`（2026-08-25 15:52~15:57，2736 行）
> 执行人：GPT

---

## 零、先纠正这轮的前提（重要，别跳过）

**「LeagueAkari 可以拿到国服以往赛季的段位数据」这个前提不成立。它也拿不到。**

对 LeagueAkari 仓库（`4e118a7`，v1.5.2-beta）做了完整的接口枚举，结论如下：

1. 它确实有一个 SGP 段位接口封装，在
   `src/shared/http-api-axios-helper/sgp/leagues-ledge.ts`：
   ```ts
   getRankedStatsByPuuid(puuid, options) {
     return this._http.get<SgpRankedStats>(`/leagues-ledge/v2/rankedStats/puuid/${puuid}`, {
       headers: { [AKARI_HEADER_TOKEN_TYPE]: 'league-session' }, ...
     })
   }
   ```
   **这就是我们 `sgp_api.go:741` 已经在调的那个接口，连令牌类型（league-session）都一样。**

2. 它的返回类型 `src/shared/types/sgp/ranked.ts` 里，`seasons` 字段**不是历史**，
   只是每个队列类型的赛季边界调度信息（`currentSeasonId` / `currentSeasonEnd` /
   `nextSeasonStart`）。没有任何按赛季分条的数组，没有 seasonId→段位的记录。
   历史相关的只有 `previousSeason*`（上一个赛季）和 `highestPreviousSeason*`（历史最高）。

3. **这个封装在它自己仓库里是死代码**——`getRankedStatsByPuuid` 零调用方。
   它 UI 实际用的是 LCU `/lol-ranked/v1/ranked-stats/{puuid}`，
   渲染组件 `RankedTable.vue` 的列就是：队列/段位/LP/胜/负/`previousSeasonEndTier`/
   `previousSeasonHighestTier`/`highestTier`。全仓库 grep `以往赛季|历史赛季|历年`
   **零命中**——它根本没做这个功能。

⇒ 所以别再花时间找「别人是怎么拿到多赛季历史的」了。**答案是没人拿得到**，
这个数据 Riot/腾讯就没有对外提供。[[deep-legends-cn-historical-ranks-research]]
那条旧结论（"最多只能拿到上赛季一格"）**被本轮独立证实，可以定案了**。

---

## 一、但真机日志给了一个好消息：我们其实已经把数据下载下来了，只是全丢了

日志里 `sgp_ranked_stats_shape` 有 **531 条**，是 R19 埋的形状探针。逐条汇总后：

### 顶层字段（531 条里的分布）
| 形状 | 条数 | 顶层键 |
|---|---|---|
| 有历史字段 | **405** | `earnedRegaliaRewardIds` / `highestPreviousSeasonAchievedRank` / `highestPreviousSeasonAchievedTier` / `highestPreviousSeasonEndRank` / `highestPreviousSeasonEndTier` / `queues` / `seasons` / `splitsProgress` |
| 无历史字段 | 126 | 同上但少了四个 `highestPreviousSeason*`（应为从没打过排位的号） |

### 每个队列条目（`queues[]`）都带这些键（531 条完全一致）
```
climbingIndicatorActive, cumulativeLp, currentSeasonWinsForRewards,
highestRank, highestTier, leaguePoints, losses, premadeMmrRestricted,
previousSeasonAchievedRank, previousSeasonAchievedTier,
previousSeasonEndRank,      previousSeasonEndTier,
previousSeasonHighestRank,  previousSeasonHighestTier,
previousSeasonWinsForRewards, provisionalGameThreshold,
provisionalGamesRemaining, queueType, rank, ratedRating, tier, wins
```

### 是真数据不是占位符
`previousSeasonEndTier` 的取值分布（531 条）：
`GOLD 108 / PLATINUM 106 / SILVER 82 / EMERALD 53 / BRONZE 24 / DIAMOND 20 / MASTER 8 / IRON 4 / 空 126`
`highestPreviousSeasonEndTier`：`PLATINUM 137 / EMERALD 103 / GOLD 74 / DIAMOND 38 / SILVER 34 / MASTER 13 / BRONZE 5 / IRON 1 / 空 126`

分布合理、跨玩家有差异 ⇒ **确认是真实数据，不是常量或占位符。**

### 而我们的解析器把它们全扔了
`sgp_api.go:714` 的 `sgpRankedQueue` 只保留 7 个字段：
`QueueType / Tier / Rank / LeaguePoints / Wins / Losses / ProvisionalGamesRemaining`。
上面那一堆 `previousSeason*` / `highestTier` **解析时直接丢弃**。

### 而且前端那张「以往赛段」表在国服根本不会有数据
`gameplay.go:837`：
```go
if reference.Region == riotRegionKR && !strings.EqualFold(..., "PRIVATE") {
    response.HistoricalRanks = a.opggHistoricalRanks(...)
}
```
**`HistoricalRanks` 只有韩服才填**（走 OP.GG 抓取），国服恒空。

⚠️ **注意**：你之前在生涯统计弹窗截图里看到的那张 9 行赛段表
（S2026 S1 翡翠III 63LP 54% / S2025 S3 钻石IV 18LP 52% ...）
**是演示数据**，来自 `web/demo-data.js:282-291`，逐行逐值对得上。真实国服玩家看到的是空的。

---

## 二、结论与方向

拿不到多赛季历史，但我们**已经免费拿到了两格真实数据**（零额外请求，同一个响应里）：

- **上赛季**：`previousSeasonEndTier/Rank`（赛季结束时的段位）、
  `previousSeasonHighestTier/Rank`（上赛季最高）、
  `previousSeasonAchievedTier/Rank`（上赛季达成）——**每个队列各一份**
- **历史最高**：`highestPreviousSeasonEndTier/Rank`、
  `highestPreviousSeasonAchievedTier/Rank`（顶层，跨所有历史赛季）
- **本赛季最高**：`highestTier/highestRank`（每队列）

方向就是：**别再假装能做多赛季历史表，改成如实展示这三格。**

---

## A. 后端：把丢掉的字段接出来

### A1. 扩展 `sgpRankedQueue`（`sgp_api.go:714`）
新增字段（全部用指针或空串判空，**上游会整段省略，不能假定存在**）：
```go
type sgpRankedQueue struct {
    QueueType                 string `json:"queueType"`
    Tier                      string `json:"tier"`
    Rank                      string `json:"rank"`
    LeaguePoints              int    `json:"leaguePoints"`
    Wins                      int    `json:"wins"`
    Losses                    int    `json:"losses"`
    ProvisionalGamesRemaining int    `json:"provisionalGamesRemaining"`
    // 新增：本赛季最高
    HighestTier               string `json:"highestTier"`
    HighestRank               string `json:"highestRank"`
    // 新增：上赛季（三种口径，上游都给了，先全接出来再决定展示哪个）
    PreviousSeasonEndTier     string `json:"previousSeasonEndTier"`
    PreviousSeasonEndRank     string `json:"previousSeasonEndRank"`
    PreviousSeasonHighestTier string `json:"previousSeasonHighestTier"`
    PreviousSeasonHighestRank string `json:"previousSeasonHighestRank"`
}
```

### A2. 接出顶层「历史最高」
`rankedStatsOn` 目前只解 `queues`。改成同时解顶层：
```go
var payload struct {
    Queues                            []json.RawMessage `json:"queues"`
    HighestPreviousSeasonEndTier      string            `json:"highestPreviousSeasonEndTier"`
    HighestPreviousSeasonEndRank      string            `json:"highestPreviousSeasonEndRank"`
    HighestPreviousSeasonAchievedTier string            `json:"highestPreviousSeasonAchievedTier"`
    HighestPreviousSeasonAchievedRank string            `json:"highestPreviousSeasonAchievedRank"`
}
```
函数签名要跟着改（现在返回 `([]sgpRankedQueue, error)`）。建议引入一个
`sgpRankedStats` 结构体把 queues + 顶层字段一起返回，别再加第三个返回值——
调用点在 `gameplay.go` 的 `loadRanksWithFallback` 链路上，改签名要一路跟到底。

### A3. 新的响应字段
在 `gameplayOverview` 里加（`gameplay.go:74` 附近，`HistoricalRanks` 旁边）：
```go
// 国服 SGP 能给的历史段位只有「上赛季」和「历史最高」两格，
// 不是 HistoricalRanks 那种多赛季表（那个只有韩服走 OP.GG 才有）。
RankMilestones *gameplayRankMilestones `json:"rankMilestones,omitempty"`
```
```go
type gameplayRankMilestones struct {
    // 历史最高（跨所有赛季，顶层字段）
    PeakTier     string `json:"peakTier,omitempty"`
    PeakDivision string `json:"peakDivision,omitempty"`
    // 上赛季（按队列分，只取单双排；灵活组排另开一项或先不做）
    PreviousSeason []gameplayPreviousSeasonRank `json:"previousSeason,omitempty"`
}
type gameplayPreviousSeasonRank struct {
    QueueType   string `json:"queueType"`
    Tier        string `json:"tier"`
    Division    string `json:"division,omitempty"`
    HighestTier string `json:"highestTier,omitempty"`
    HighestDiv  string `json:"highestDivision,omitempty"`
}
```

### A4. 只在国服填，且要判空
- 只有 `reference.Region == ""`（国服）才填 `RankMilestones`；
  韩服继续走现有 `HistoricalRanks`（OP.GG），两条路互不干扰。
- **126/531 的响应里根本没有 `highestPreviousSeason*`**，
  `previousSeasonEndTier` 也可能是空串。**任何一格空就不要输出那一格**，
  整体都空就让 `RankMilestones` 保持 nil（`omitempty` 会整个省掉）。
  别输出 "UNRANKED"/"NONE" 之类的伪值——前端要靠字段缺失来决定不渲染。
- tier 字符串上游是全大写（`GOLD`/`EMERALD`），我们前端现有的
  `rankTitle`/`rankCrestIcon` 吃的是小写（看 demo-data 是 `"emerald"`），
  **落库前统一 `strings.ToLower`**，别让前端两套大小写各自判断。

---

## B. 前端：如实展示，别硬凑成表格

`web/gameplay.js` 的 `renderRanks(ranks, capabilities, historicalRanks)` 现在
末尾调 `renderHistoricalRanks(historicalRanks)`（`:1005`）。

改成：韩服有 `historicalRanks` 就还是渲染那张多行表；国服改渲染一个
**紧凑的两格摘要**（不要做成只有一两行的表格，那样表头比内容还高，很难看）：

```
历史最高  💎 钻石 IV        上赛季  🟢 翡翠 I（最高 翡翠 I）
```

要点：
- 复用现有的 `rankCrestIcon(tier)` 和 `rankTitle({tier, division})`，别新写一套。
- 两格都空 → 整块不渲染（不要显示"暂无数据"占位，国服大部分新号都会是空，
  满屏"暂无"很难看）。只有一格有 → 只渲染那一格。
- 上赛季那格如果 `highestTier` 和 `tier` 相同，**别重复写**"（最高 XX）"。
- 文案别写成"以往赛季段位"，会让人以为有多赛季。建议就叫
  **「历史最高」**和**「上赛季」**两个标签，说清楚边界。
- 放在 `.rank-list` 下面、原来 `renderHistoricalRanks` 的位置。

---

## C. 补最后一个未知数：`seasons` 那 667 字节到底是什么

日志里 `season_fields.seasons` 恒为 `<667 bytes>`——我们的探针
`diagnosticSeasonFields` 对象一律记字节数，所以没展开过。
LeagueAkari 的类型声明说它只是赛季边界调度信息（`currentSeasonId` /
`currentSeasonEnd` / `nextSeasonStart`），但**那是他们手写的类型，不是实测**，
而我们已经实测到真实载荷比他们的类型多出好几个字段（`previousSeasonAchieved*`、
`climbingIndicatorActive`、`currentSeasonWinsForRewards` 他们类型里都没有）。
所以他们的类型不足以采信，得自己看一眼。

同理 `splitsProgress` 恒为 `<2 bytes>`（就是 `{}`），空的；
但**万一某些账号非空**，那里可能有分段（split）历史。

做法：在 `diagnosticSeasonFields` 里对 **`seasons` 和 `splitsProgress` 这两个
键开白名单，原样记录 JSON**（其余对象维持记字节数不变）。
理由：这两个字段只含赛季 ID 和时间戳，是全服一致的调度信息，**不含任何玩家标识**，
原样落日志没有隐私问题；667 字节也不会把日志撑大。

⚠️ **`sgp_api_test.go` 里有一条字段白名单断言 `allowedShapeKeys`**，
改形状埋点必须同步更新，否则
`TestSGPRankedAndSummonerRoutesReportSessionToken` 会红。（这条 R19 就踩过。）

这是一次性调研，拿到真机日志确认 `seasons` 确实只是调度信息之后，
把白名单撤掉、恢复成记字节数。

---

## D. 顺手清掉一个误导源

`web/demo-data.js:282-291` 那 9 行 `historicalRanks` 演示数据，
**在国服真机上永远不可能出现**（那是韩服 OP.GG 链路的形状）。
这轮我就是差点被它误导（截图里看到 9 行表，以为国服拿到了数据）。

建议：演示数据保留（韩服确实是这个形状，删了韩服预览就没东西看了），
但在 `demo-data.js` 那段前面加一行注释写明
「这是韩服 OP.GG 链路才有的形状，国服 `historicalRanks` 恒空、走 `rankMilestones`」，
免得下次又有人（包括我）拿它当国服的证据。

---

## E. 测试要求（按这个项目的既有标准）

1. **Go 侧**：给 `rankedStatsOn` 加解析测试——用真机日志里实际观测到的那两种形状
   各造一个 fixture（有 `highestPreviousSeason*` 的 / 没有的），断言：
   - 有的那份能正确解析出 peak 和 previousSeason
   - 没有的那份 `RankMilestones` 必须是 nil（不是空结构体，不是零值伪数据）
   - tier 已转小写
2. **前端**：断言国服路径渲染 `rankMilestones`、韩服路径渲染 `historicalRanks`，
   两者不会同时出现；两格都空时整块不渲染。
3. **变异测试必跑**（这个项目的硬要求）。至少覆盖：
   - 判空逻辑被去掉（空值也渲染 → 应该被杀）
   - `ToLower` 被去掉（大小写不一致 → 图标/文案挂掉，应该被杀）
   - 国服/韩服分支判据反了（应该被杀）
   - `omitempty` 被去掉
   ⚠️ 变异测试**只拷 `web/` 或必要的 `.go` 文件**，
   **不要 `copytree` 整个仓库**——`dist/` 有 702MB，会把沙箱磁盘写满
   （R27 刚踩过）。
4. 全绿标准：`gofmt` 干净、`go vet` 干净、`go test -race ./...`、
   前端 138 测试、desktop 17 测试。

---

## F. 明确不要做的事

- ❌ 不要再去找「多赛季历史段位」的第三方数据源。已经查实：
  lolso1 没这功能且 `/api/` 禁抓；wegame 上了 WAF；腾讯官方页面停在 2016 年且要登录；
  LeagueAkari 也没有。**这个数据不存在可获取的来源。**
- ❌ 不要把 `HistoricalRanks` 在国服硬塞成一行（"S2025 上赛季"这种自己编的赛季号）。
  我们不知道上赛季的准确赛季编号，编一个出来是伪造数据。
  就叫「上赛季」，不带编号。
- ❌ 不要为了这个功能新增任何网络请求。所有字段都在**已经在发的那一个响应**里。
