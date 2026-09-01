# WORKLIST-R41（给 GPT 执行）

> R40 已经全部验收通过（gameId 400 的单一根因、hexdata 熔断、位置解析策略、绝活哥限速），
> **只剩一件事没做完**：绝活哥符文卡片右侧的对手段位/胜率。
> 上一轮验收把这条误判成了"上游没有数据"，实际是判断错了——项目里已经有现成的机制，
> 只是没有接上。这份工单只有一组，专门把这条补完。

---

## A 组（P1）：绝活哥符文卡片补上对手的段位与胜率

### A-1 背景

用户截图（图二）要求：绝活哥符文推荐右侧显示的对线英雄旁边，把对线玩家的
**召唤师名称、段位、胜率**都展示上。

召唤师名称这轮已经做上了（`specialist_runes.go:226-229` 已经在写
`OpponentChampionID`/`OpponentChampionName`/`OpponentPlayerName`/`OpponentTagLine`），
**唯独段位和胜率没做**。上一轮验收时误判成"上游拿不到数据"，用户追问后我去核实了代码，
**这个结论是错的，纠正如下**。

### A-2 数据从哪来（已经全部具备，不需要新对接任何上游）

1. **对手的 puuid 我们已经有了**：`specialist_runes.go:298` 的 `specialistOpponent()` 返回的
   是 `riotParticipant`，这个结构体（`riot_api.go:390`）本身就带 `PUUID string`。
   `specialist_runes.go:225` 那行 `opponent := specialistOpponent(...)` 拿到的
   `opponent.PUUID` 就是查段位需要的 playerRef，**不需要额外请求去换取**。
2. **查段位胜率的能力项目里已经有一套现成的**：`rank_insights.go` 的
   `playerRankScore(ctx, client, playerRef, isCurrent, serverID, privacy)` 函数
   （`rank_insights.go:167`），返回 `rankScoreEntry{score, known, source, at}`；
   它背后调用的 `loadRanksWithFallback` 拿到的是 `[]gameplayRank`
   （`gameplay.go:160`，字段有 `Tier`/`Division`/`LeaguePoints`/`Wins`/`Losses`/`WinRate`），
   **这就是我们平时给战绩列表算"平均段位"用的同一条链路**
   （`/api/gameplay/match-tiers`，`rank_insights.go:267`），自带 `rankScoreCache` 缓存
   （10分钟 TTL，见 `rank_insights.go:63`）和并发限制（`matchTiersRankConcurrency=4`，
   一批最多 `matchTiersMaxRefs=24` 人）。
3. **`gameplayRecommendationRune` 结构体（`gameplay.go:3038`）本身已经有
   `Tier`/`Division`/`LeaguePoints` 三个字段**——那是给绝活哥自己的段位用的，
   现在只是没有对应的 `Opponent` 版本。

### A-3 改法

1. **结构体加字段**：`gameplay.go:3061` 那行 `OpponentTagLine` 后面加：
   ```go
   OpponentTier         string `json:"opponentTier,omitempty"`
   OpponentDivision     string `json:"opponentDivision,omitempty"`
   OpponentWinRate      *int   `json:"opponentWinRate,omitempty"`
   ```
2. **查询方式：走批量接口，不要每个对手单独查一次**。
   绝活哥符文推荐一次会返回好几条（本份日志里是 `runes_returned:9`），
   每条背后可能是不同的对手 puuid。**收集这一批推荐里所有的 `opponent.PUUID`，
   一次性调用批量查询**（可以直接复用 `handleGameplayMatchTiers` 背后的批量逻辑，
   把它抽出一个内部函数给这里也调用；或者直接循环调用 `playerRankScore` ——
   注意 `playerRankScore` 本身已经有 `rankScoreCache` 缓存和 `beginFlight`/`finishFlight`
   的单飞保护，**多次调用不会产生重复的上游请求**，所以循环调用是安全的，
   但仍然要用 `matchTiersRankConcurrency` 那种并发上限包一层，不要一次性无限并发甩出去）。
3. **组装响应时把查到的 Tier/Division/WinRate 填进每条 rune 记录的
   `OpponentTier`/`OpponentDivision`/`OpponentWinRate`**。
4. **查不到时留空（`omitempty` 已经处理了序列化），前端对应字段没有就不显示**，
   不要因为某个对手查不到段位就影响其他字段的展示。
5. **前端 `web/gameplay.js`** 补上这三个字段的渲染，放在对手召唤师名称旁边，
   段位用现有的段位徽章/文案格式（照抄战绩列表里"平均段位"那块的展示方式，不要新造一套）。

### A-4 变异测试

1. 构造一份包含 3 条绝活哥推荐、对手 puuid 有重复的场景，断言**批量查询只发生一次
   去重后的调用**（重复 puuid 不重复查）；把去重逻辑去掉，测试必须 FAIL。
2. 断言某个对手查询失败/无结果时，`OpponentTier` 等字段留空，
   但**不影响**该条推荐其他字段（符文本体、`OpponentPlayerName` 等）正常返回。
3. 断言整个流程复用的是 `rankScoreCache`（可以检查调用次数/缓存命中标记），
   而不是绕开缓存另开一条路径。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- 写至少一个真变异测试（改回旧行为 → 测试必须 FAIL → 复原变绿），变异结果贴在提交说明里。
- **这条必须复用 `playerRankScore`/`rankScoreCache` 这套现成机制**，不允许新写一套
  独立的段位查询逻辑，也不允许因为要多查一次段位就让绝活哥符文接口的耗时明显变长
  （D 组刚把耗时从 40 秒压到 10 秒以内，这条不能把它拖回去——批量查询要和现有的绝活哥
  扫描并发一起走，不要串行等它查完再返回）。
- 改完后请用户重启应用复现，从「设置 → 隐私与能力 → 导出诊断日志」导出，
  **上传前把文件重命名一下**。
