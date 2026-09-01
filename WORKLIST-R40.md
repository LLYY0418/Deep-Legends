# WORKLIST-R40（给 GPT 执行）

> 基于 0830 00:49 真机日志（`lol-loot-diagnostics-0830-0049.jsonl`，1992 行 / 28 分钟）+ 3 张截图。
>
> **本轮找到了对局页几乎所有功能失效的单一根因，而且是一行代码。**
> R39 加的观测埋点全部生效，这次终于能直接看到真相，没有再靠猜。

---

## 零、R39 验收结论（观测层已经修好，这是本轮能定位的前提）

R39 加的四个埋点**全部在真机日志里正常出现**，白名单拒收问题彻底解决：

```
live_recommendations_request   9 条   ← R39 之前是 0
live_recommendations_skip     17 条   ← R39 之前是 0（被后端 400 掉）
live_roster_shape              5 条
perk_apply_attempt             1 条
```

**顺带确认符文应用其实是好的**：唯一那条 `perk_apply_attempt` 是
`outcome:"success"`、`phase:"ChampSelect"`、`perk_ids` 恰好 9 个、
LCU 返回 `isValid:true` 且 `selectedPerkIds:[8230,8226,8210,8236,8473,8451,5005,5008,5001]` 完整写入。
**说明 R39-E 组怀疑的"部分写入"在这次没有复现**，上次大概率是当时确实不在英雄选择阶段。
这条先不动，等下次真复现了再看。

---

## A 组（P0，单一根因，一行代码）：`gameId` 被数值上限卡死，导致推荐接口 100% 返回 400

### A-1 决定性证据

日志里这一段把因果链完整串起来了：

```
16:24:43.247  live_recommendations_request  {champion_id:104, queue_id:3100, raw_position:"top", status:"received"}
16:24:44.411  live_recommendations_skip     {champion_id:104, reason:"backoff", key:"104:top:CLASSIC:11:4-14"}
```

请求**到达了后端**（`live_recommendations_request` 是 handler 第一行记的），
**1.16 秒后前端就记了 `backoff`**（说明请求已经失败返回）。
而这两条之间**没有 `recommendation_mode_resolved`**。

`recommendation_mode_resolved`（`gameplay.go:3198`）距离那条新埋点（`gameplay.go:3173`）
中间**只隔了四个参数解析**。所以 handler 一定是在这四个解析里的某一个返回了 400。

### A-2 根因：`parseOptionalRecommendationInt` 的上限是 100000，而 gameId 是百亿级

`gameplay.go:3363`：

```go
func parseOptionalRecommendationInt(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, nil
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed < 0 || parsed > 100000 {   // ← 就是这里
		return 0, errors.New("invalid recommendation number")
	}
	return parsed, nil
}
```

这个函数被 `queueId` / `mapId` / **`gameId`** 三个参数共用。
`queueId`（3100）和 `mapId`（11）都远小于 100000 没问题，
**但 `gameId` 是十位数**——本份日志 `live_roster_shape` 实测：

```
game_id: 8956354574
game_id: 8956348789
game_id: 8956354731
game_id: 8956350775
game_id: 8956367141
```

**89 亿 >> 100000**，所以 `gameplay.go:3192-3196` 必然返回
`http.Error(w, "推荐对局无效", http.StatusBadRequest)`。

而前端只要 `target.gameId > 0` 就会带上这个参数
（`web/gameplay.js`：`if (target.gameId > 0) query.set("gameId", String(target.gameId));`），
**也就是说只要进了对局（有 gameId），这个接口就 100% 挂**。

### A-3 这一行解释了用户报的几乎全部症状

| 用户反馈 | 解释 |
|---|---|
| 图三「所有模式的出装推荐都没有了」 | 接口 400，`build` 永远拿不到 |
| 图一 胜率/选取率/禁用率 全是「—」 | 同上，`hero` 指标拿不到 |
| 图一 优劣势对抗「该位置暂无对线样本」 | 同上，`counters` 拿不到 |
| 图一/图三 OPGG 符文变成「客户端内置（备用）」 | 同上，走了 `clientRecommendation` 兜底 |
| 海克斯/斗魂「推荐暂不可用」 | 同上（另有 B 组的独立问题叠加） |

这也是为什么 `live_build_shape` / `champion_data_resolved` / `recommendation_mode_resolved`
连续几轮都是 0——**它们全都在那个 400 的后面**。

### A-4 改法

1. **`gameId` 不能和 `queueId`/`mapId` 共用同一个校验函数。**
   新增 `parseOptionalGameID(value string) (int64, error)`，上限用 `int64` 的合理范围
   （建议 `parsed < 0 || parsed > 1<<62` 即可，gameId 本来就是 Riot 的 64 位自增 ID，
   **不要再拍一个"看起来够大"的魔法数字**，否则几年后又会踩同一个坑）。
2. `queueId` / `mapId` 保留现在的小上限没问题，但**把 100000 这个魔法数字提成具名常量**
   并写清楚它只适用于队列/地图这类小整数。
3. **`gameId` 解析失败时不应该整个请求 400。** gameId 只是用来做诊断去重的辅助参数，
   拿不到不影响推荐本身。改成解析失败就当 0 处理并记一条诊断，**不要让辅助参数搞挂主功能**。
4. **★把失败原因暴露出来**：现在 `http.Error` 的文案（"推荐对局无效"）根本没进日志，
   前端也只记了个 `backoff`，所以连续几轮都不知道是哪一步挂的。
   改成在每个 `http.Error` 返回前记一条
   `{"event":"live_recommendations_rejected","stage":"gameId|championId|queueId|mapId|position|source","status":400}`，
   **不做去重，至少前 20 次全记**。

**变异测试**：
① 断言 `gameId=8956354574`（用日志里的真实值）时接口**不返回 400**、
且 `recommendation_mode_resolved` 会被记录；把上限改回 100000，测试必须 FAIL。
② 断言 gameId 传一个非法字符串时，接口**仍然正常返回推荐**（降级成 0），只记诊断。
③ 端到端断言一次完整的推荐响应里 `build`/`hero`/`counters` 都非空
（**这条是本轮最重要的护栏**——之前几轮就是因为没有端到端测试，
这个 400 才能连续存活好几轮没人发现）。

---

## B 组（P0）：图三 海克斯大乱斗 —— 熔断器一次失败就锁死 8 小时 + 队列 3270 没登记

### B-1 熔断器实证

```
16:28:50.217  hexdata_shape_invalid  {kind:"augments", rows:0, fields:0, buildId:""}
16:28:50.218  hexdata_circuit_trip   {kind:"augments", failures:1, reason:"shape validation failed",
                                      until:"2026-08-30T00:29:50+08:00"}
16:28:50.238  hexdata_circuit_open   {module:"augments", probeAt:"2026-08-30T00:29:50+08:00"}
16:28:50.238  hexdata_fallback       {module:"augments", reason:"hexdata circuit is open"}
```

**`failures:1` —— 上游只失败了一次，熔断器立刻跳闸，而且一锁就锁到 8 小时后。**
（对比同一时刻其他 kind 都是正常的：`hexdata_shape kind:"answer" rows:172`、
`kind:"heroes" rows:172`、`hexdata_item_match matched:8`——**只有 augments 这一个模块拿到了空载荷**。）

这个"一次失败锁 8 小时"的问题在项目历史上已经出现过（见 hexdata 熔断吸收态那次），
当时的结论是删 state 文件也救不回来。**这次要从策略上修掉，不是再手工重置一次。**

### B-2 改法

1. **失败阈值不能是 1。** 改成连续失败 N 次（建议 3 次）才跳闸。
   单次空载荷更可能是上游发布中途的瞬时状态，不是真的坏了。
2. **锁定时长不能是 8 小时。** 改成指数退避：首次 1 分钟、然后 5 分钟、15 分钟，
   **上限 30 分钟**。用户开着客户端打一晚上游戏，8 小时等于这个功能当晚彻底没了。
3. **半开状态要真的探测**：熔断打开期间，到了 `probeAt` 要**主动放一个请求过去试探**，
   成功就立刻关闭熔断，而不是等下一次用户操作。
4. **空载荷（rows:0 且 fields:0）要和"解析失败"区分开**：
   前者更像上游正在发布，应该走**短退避 + 保留上一次成功的缓存**，
   而不是和"数据格式错误"一样对待。

### B-3 队列 3270 没有登记（独立问题，同样影响图三）

`live_recommendations_request` 里出现了 **`queue_id: 3270`**（champion 432/516，
`raw_position:"other"`，前端 key 显示 `KIWI:12`）：

```
{"champion_id":432,"queue_id":3270,"raw_position":"other","event":"live_recommendations_request"}
{"champion_id":516,"queue_id":3270,"raw_position":"other","event":"live_recommendations_request"}
```

**`grep 3270 queue_groups.go` 零命中**——这是海克斯大乱斗的又一个队列 ID，我们没登记过。
（我们表里只有 2300/2400。）

**这已经是第五次栽在队列 ID 上了**（`-1` → `3100` → `480` → `1750` → 现在 `3270`）。

改法：
1. `queue_groups.go` 补上 3270（`hextech-aram` 组、`augmentSource:"hextech"`），
   并顺带核对还有没有别的海克斯/斗魂变体队列。
2. **更重要的是加一条"未知队列"告警**：`resolveGameplayRecommendationMode` 里
   如果 queueID > 0 但不在权威表中，记一条
   `{"event":"unknown_queue_observed","queue_id":N,"game_mode":"...","map_id":N}`（按 queueID 去重）。
   **这样下次再出现新队列，日志里会主动告诉我们，而不是等用户截图报障。**
   这条是本轮的防复发措施，比补一个 3270 重要得多。

**变异测试**：断言 queueId=3270 时能正确解析成 hextech-aram 且 augmentSource 是 hextech；
断言遇到一个表里没有的 queueID（比如 9999）会记 `unknown_queue_observed`。

---

## C 组（P1）：图一 位置选择策略改成"跟随 OPGG 常用位置"（用户的方案是对的）

### C-1 用户的判断完全正确，日志实证

用户原话：
> 「你去找该英雄在当前对局中选择的位置的符文推荐，我觉得这样不对，
> 这些数据在 opgg 里面都是该英雄的常用位置上才有数据……
> 你就像英雄详情里面展示几个常用位置就行，然后如果几个常用位置里面包含
> 当前对局中选择的位置，那就直接选择该位置，如果没有，就选择第一个（胜率高）位置」

日志实证用户是对的——所有请求的 `raw_position` **全是 `"top"`**：

```
{champion_id:104(格雷福斯), raw_position:"top"}
{champion_id:13 (瑞兹),     raw_position:"top"}
{champion_id:22 (艾希),     raw_position:"top"}
```

格雷福斯是打野、瑞兹是中单、艾希是下路——**没有一个是上单英雄**。
用户在自定义房间里站在上路位置，我们就硬拿 `top` 去查 OPGG，
而 OPGG 上这三个英雄的 top 位置根本没有统计样本。

**注意**：A 组那个 400 修好之后，如果不改这条，
这三个英雄依然会因为"OPGG 没有 top 的数据"而显示一片空——
**A 组和 C 组必须一起修，只修 A 组用户看到的还是空的。**

### C-2 改法（严格按用户描述的规则）

1. **先取该英雄在 OPGG 上的常用位置列表**（`positions` 数组，
   英雄详情页已经在用了，`qq101_positions_resolved` 也有这个能力，**复用现成的，不要新造**）。
2. **选位规则**：
   - 如果当前对局的位置**在**常用位置列表里 → 用当前对局的位置
   - 如果**不在**（或当前位置是 `other`/空）→ **用列表里的第一个（选用率/胜率最高的那个）**
3. **把最终选中的位置和候选列表都返回给前端**，前端在英雄名下面显示实际使用的位置，
   并且**像英雄详情页一样把几个常用位置做成可点的按钮**让用户能手动切。
4. **这条改完之后，OPGG 那一页就不该再出现"客户端内置（备用）"这个备选方案了**
   （用户明确提到这一点）——只有在 OPGG 完全没有该英雄任何位置的数据时才降级。
5. **加诊断**：`{"event":"position_resolved","champion_id":N,"requested":"top","resolved":"jungle","source":"opgg-primary|requested|fallback","candidates":["jungle","mid"]}`。

**变异测试**：
① 断言"瑞兹 + 请求位置 top"最终解析成 OPGG 常用位置里的第一个（中单），而不是 top；
② 断言"瑞兹 + 请求位置 mid"（在常用列表里）就用 mid，不被改写；
③ 把规则改回"永远用请求位置"，测试必须 FAIL。

---

## D 组（P1）：绝活哥符文加载太慢（实测 26~40 秒，比用户说的 10 秒还糟）

### D-1 实测耗时

```
champion 104: duration_ms 26503  budget_used 15  runes_returned 9
champion 13:  duration_ms 34340  budget_used 17  runes_returned 5
champion 22:  duration_ms 40006  budget_used 21  runes_returned 3
```

**26.5 秒 / 34.3 秒 / 40 秒**，而且**越慢的返回的符文越少**（9 → 5 → 3）。

日志里有 12 条 `specialist_runes_step_failed`，绝大多数是 `step:"match_detail"`，
`errorKind:"other"`，而且是**连续快速失败**（16:26:20.543 → .544 → .545 → .546 → .547，
每毫秒一条），说明在**批量拉某个绝活哥的历史对局详情时整批失败**，
然后继续消耗预算（`budget_remaining` 从 24 一路降到 19）去试下一个。

### D-2 改法（按用户要求）

用户原话：
> 「可以去找这个绝活哥是否有在当前对局中选择的位置的战绩，
> **最多只找该英雄最新的 30 局战绩**，没有就展示最新一局的符文，太多加载太慢了」

1. **硬性上限：每个绝活哥最多扫最近 30 局**，扫到符合位置的就停，
   扫完 30 局还没有就**直接用最新一局的符文**，不要再往前翻。
2. **`match_detail` 整批失败时要立刻中止这个绝活哥**，不要继续消耗预算——
   现在的行为是失败了还接着试，白白拖时间。
   连续失败 3 次就跳过当前绝活哥、换下一个。
3. **加超时上限**：整个绝活哥符文请求**最多 10 秒**，超时就把已经拿到的先返回
   （现在是 40 秒还在硬扛，用户体验上等于卡死）。
4. **`step_failed` 的 `errorKind` 现在全是 `"other"`，等于没说**。
   请把真实的错误类型分出来（超时 / 404 / 限流 / 解析失败 / 上游 5xx），
   否则下次还是不知道为什么失败。

**变异测试**：断言扫描在第 30 局处停止（构造一个 50 局的场景）；
断言连续 3 次 `match_detail` 失败后会跳过该绝活哥；断言 10 秒超时会返回已有结果而不是空。

---

## E 组（P2）：图二 绝活哥符文卡片的信息补全与排版

用户要求（这一组是纯 UI/数据展示，**但要先确认数据拿不拿得到**）：

1. **右侧那局的对线英雄旁边，补上对线玩家的召唤师名称、段位、胜率。**
   ★**上一轮我问过这个，当时的结论是 `load_top_players_shape` 只 `parsed:5`（只有绝活哥本人），
   没有对手信息。这轮请先确认：我们已经在拉那一局的 match detail 了
   （`step:"match_detail"` 就是在干这个），那一局的完整参与者名单里应该是有对手的
   puuid / 召唤师名 / 英雄的。**
   如果确实能拿到，就补上；**如果拿不到，如实回报，不要用占位数据糊**。
   段位和胜率如果需要额外查询，**要走已有的批量查询和缓存，不要每个对手单独发一次请求**
   （否则又会变成 D 组那种几十秒的问题）。

   **★★0830 验收追加：召唤师名称已经做上了，但段位胜率完全没做，验收时误判成"上游拿不到"，
   实际是判断错了——项目里已经有现成的机制能查任意召唤师的段位和胜率：`rank_insights.go` 的
   `playerRankScore` / `/api/gameplay/match-tiers`（就是 R37/R38 那几轮给战绩列表算"平均段位"
   用的那套），输入 playerRef（召唤师名#编号或puuid）就能批量拿到 `Tier`/`Division`/`WinRate`/
   `Wins`/`Losses`，自带 `rankScoreCache` 缓存和并发限制。**
   **这条不要重新设计，直接复用这套现成机制**：把对手的 playerRef 传给同一个批量查询入口
   （可以直接调用 `handleGameplayMatchTiers` 背后的批量逻辑，或者给绝活哥符文接口的响应
   顺带带上对手的 playerRef，前端复用现有的 `hydrateMatchTiers` 那条请求路径去查），
   **不允许每个对手单独发一次请求，也不允许说"上游没有"就跳过**——这个能力我们自己就有。

2. **左上角对局结果旁边加上位置**，格式就按用户给的：`负（中路）`、`胜（上路）`。
3. **「本局装备」按实际出装顺序从左到右排列，且只统计成装**（不含小件/消耗品）。
   出装顺序需要从 match detail 的事件时间线里取，**确认我们拉的那份详情里有没有
   `ITEM_PURCHASED` 事件**；如果只有最终装备栏（无顺序信息），
   **如实回报"上游只有最终装备、拿不到顺序"**，不要自己编一个顺序。
   **注意：这条说的是绝活哥自己那局的装备（卡片下方"本局装备"一栏），不是对手的装备，
   不要和第 1 点混在一起。**

   **★★0830 验收追加：出装顺序确认了上游没有 ITEM_PURCHASED 事件、也没有编造顺序，这点做对了；
   但"只统计成装、不含小件消耗品"这条完全没做——现在 `Item0..Item6` 原样展示，饰品栏（Item6）
   也混在里面。请补上过滤：装备 ID 要对照装备数据里的"是否为完整装备"（能通过成装判定的字段，
   项目里出装推荐模块应该已经有类似的"核心装/成装"判定逻辑，找现成的复用，不要新写一套
   装备分类规则）来过滤，饰品栏单独处理（不算在"本局装备"这一排里，如果要展示饰品可以放在
   旁边单独一个位置，不要和成装混排）。**

**布局自己设计**，但要求：不要让卡片变得更高（现在一屏已经放不下两局了），
优先横向利用右侧那块空白区域。改完用 Playwright 截图对比，
**并确认 420px 窄屏下不截断**。

---

## F 组（P2）：图一 胜率/选取率/禁用率 离英雄名称还是太远

R38 已经调过一次，用户说**还是太远，要求间距 15px**。

现在这三个指标和左侧英雄名之间还有一大片空白。
改成紧跟在英雄名/位置那一块后面、间距 **15px**，中间那条分割线保留。
**用 Playwright 截图量一下实际像素间距确认是 15px**，不要凭感觉调。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- **A、B、C、D 四组各写至少一个真变异测试**（改回旧行为 → 测试必须 FAIL → 复原变绿），
  变异结果贴在提交说明里。
- **A 组是本轮的头号任务**，一行代码，但它是对局页几乎所有功能失效的单一根因。
  **A-4 第 3 点（辅助参数不许搞挂主功能）和变异测试第 ③ 条（端到端断言推荐响应非空）
  是防复发的关键**，请务必做——这个 400 能连续存活好几轮没被发现，
  就是因为缺一个"推荐接口端到端返回了有用数据"的测试。
- **A 组和 C 组必须一起修**，只修 A 组的话，格雷福斯/瑞兹/艾希这些英雄
  用户看到的还是一片空白。
- **B-3 那条 `unknown_queue_observed` 告警比补 3270 本身更重要**——
  我们已经因为队列 ID 错了五轮，需要一个能主动发现新队列的机制。
- **E 组的两处"先确认上游有没有数据"（对手信息、出装顺序）请如实回报，
  拿不到就说拿不到**，不要编占位数据。
- 改完后请用户**重启应用**再复现，从「设置 → 隐私与能力 → 导出诊断日志」导出，
  **上传前把文件重命名一下**。
- 下轮验收硬指标：
  ① `recommendation_mode_resolved` / `champion_data_resolved` / `live_build_shape`
  **必须出现**（出现 = A 组修好了）；
  ② `specialist_runes_done` 的 `duration_ms` **必须降到 10000 以内**（D 组）；
  ③ 日志里**不该再有** `hexdata_circuit_trip` 的 `failures:1`（B 组）。
