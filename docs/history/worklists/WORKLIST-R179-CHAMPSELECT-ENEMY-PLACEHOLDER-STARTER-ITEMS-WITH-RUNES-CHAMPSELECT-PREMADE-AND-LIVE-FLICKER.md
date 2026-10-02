# WORKLIST-R179：选人阶段敌方卡片文案 / 绝活哥出门装慢且会消失 / 选人阶段组队情况 / 对局页抖动闪烁

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-01。基线：源码 0.12.43（R178 之后）。

## 证据范围

用户日志 `lol-loot-diagnostics-1001-2027.jsonl`，0.12.43 会话：run `d984a5ec…`，build `d5961921e9b6`，时间 11:47–12:27Z。包含两场灵活排位：

- game 9009758600：选人 11:52:47，本人中路，发条魔灵 61。
- game 9009800745：选人 12:17:29，本人下路，英雄 145。

用户提供两张截图（选人阶段「详情」页、符文「绝活哥」页）。

这份日志顺带验证了下面几项，账本里记为"真机已验证"：

- R177 P1：`rune_starter_batch` 共 16 条，`rate_limit` 全部为 0，不再被后台额度挡住。
- R177 P2：两场都出现 `lane_matchup_candidate_fetch reason=requested → succeeded rows=5`（敌方 101 中路、81 下路），卡片已经能触发。
- R178 P1：game 9009800745 进入对局时 `team_duplicate_positions` 两队都是 0；12:20:28 的位置来自 snapshot 5 人加 gameflow 5 人，各路 2 人；12:20:39 补探测后变成 liveclient 10 人。
- R178 P2：所有抽样的 `lcu_missing_newer = 0`，**客户端战绩并不陈旧**。"不是最新"来自按队列筛选，例如 `newest_any_age_min 26.73` 对 `newest_queue_age_min 34459.88`：玩家 27 分钟前刚打过其他模式，但名单只显示 24 天前的灵活排位。是否改成"所有模式"由用户决定，本工单不改。

## P1　选人阶段敌方卡片显示"隐藏玩家 / 隐藏身份 / 客户端未公开该玩家"，应改为"暂无玩家信息"

### 现状

截图 1 中，选人阶段对方 5 张卡片都显示：名字"隐藏玩家"、标签"隐藏身份"、副标题"位置未知 · 未定级"、模式列"客户端未公开该玩家"、底部"客户端未公开该玩家"。

选人阶段客户端本来就不下发敌方身份（`their_team_nonzero_counts` 只有 cellId / nameVisibilityType / team / wardSkinId），这不是玩家主动隐藏身份。现有文案把"还没到能查的时候"说成了"玩家隐藏了身份"。

相关渲染：`gameplay.js` 约 5538 行 `hiddenChip`（"隐藏身份"），约 5541 行 `emptySummary`（"客户端未公开该玩家"），约 5967 行战绩行占位，约 7072–7075 行 `playerLabel / maskedPlayerName` 等（"隐藏玩家"）。

### 要求

- 仅当 `phase === "ChampSelect"` **且**是敌方（按现有 R167 的 teamId 规则判断：不是 `isAlly`、不是本人、`teamId` 与本人不同）时，整张卡只显示一行"暂无玩家信息"：
  - 不显示"隐藏身份"标签、"位置未知 · 未定级"、模式 / 胜率 / KDA 三列、底部"客户端未公开该玩家"。
  - 头像保留现在的占位图；该敌方已锁定英雄时，照旧显示英雄头像。
  - 卡片高度与己方卡片的最小高度一致，不要因为内容变少让两列对不齐。
- 进入对局（InProgress / Reconnect）后，真正开启了隐藏身份的玩家仍按现有逻辑显示"隐藏玩家 / 隐藏身份"，不受影响。
- 己方玩家文案不变。不新增任何解释性文字或 tooltip（红线）。

### 测试

Node：

1. 选人阶段敌方 `hidden=true`：只有"暂无玩家信息"，没有 `player-tab-hidden`、"客户端未公开该玩家"、"位置未知"。
2. 同样数据、阶段为 InProgress：输出与现在逐字一致。
3. 选人阶段己方 `hidden=true`（极少见）：仍是现有文案。

变异：去掉阶段判断 → 测试 2 FAIL。

## P2　绝活哥出门装要等一会才出来，过一会又消失

### 时间线

game 9009758600（本人中路发条，截图 2 就是这一场）：

| 时间 | 事件 |
|---|---|
| 11:54:07.95 | 第一次请求绝活哥符文（推荐键 `…:3-4`） |
| 11:54:15.00 | `specialist_runes_done`，36 次请求，用时 7.1 秒，终局装备此时已经可以显示 |
| 11:54:21.30 / 21.31 / 22.88 | 3 条出门装 `cache=miss` 成功，比装备晚 6–8 秒 |
| 11:54:50.89 | 用户点"应用符文"并同时应用召唤师技能（`spell_applied=true`） |
| 11:54:51.57、11:54:51.66 | 推荐键变成 `…:4-6`，**又请求了两次绝活哥符文**，接着两批出门装（来自内存缓存） |
| 11:54:58.95、11:54:59.03 | 推荐键 `…:4-12`，同样又请求两次 |
| 11:55:36.80、11:55:36.97 | 进入对局，推荐键 `…:none`，同样又请求两次 |

game 9009800745 一样：键依次为 `4-12 → 4-21 → 3-4 → none`。12:19:00 符文完成后，出门装到 12:19:13.9 才出来（用户先看的是职业选手页签）。

### 根因（源码已核对）

1. **出门装和装备不是一次拿到的。**终局装备来自 36 次请求里的对局详情；出门装需要每场再拉一次时间线（`rune_starter_items.go`）。而且出门装只在前端把卡片画出来**之后**，才对"当前可见的 3 行"发起第二阶段请求（`ensureRuneStarterItems`，`gameplay.js` 约 5173 行）。所以出门装天生比装备晚一轮。
2. **换召唤师技能就把整个绝活哥数据作废。**`specialistRequestTarget`（约 4876–4880 行）的键是 `championId:position:gameMode:mapId:tier:spellKey`，`spellKey` 是本人当前的召唤师技能。但 `/api/gameplay/specialist-runes` 只用 `championId` 和 `position` 两个参数，结果和技能、段位都无关。用户一应用技能（R171 的一键应用本身就会改技能），键就变了：
   - `state.specialistRunes` 里找不到新键，重新请求，卡片先进入加载状态；
   - 新返回的记录不带 `starterItemIds`，于是出门装消失、行变回"最终装备"；
   - 再发一轮出门装请求，补回来。
   这就是"过一会就没有了"和整块符文区闪一下的原因。进入对局时技能键变成 `none`，又来一次。
3. **绝活哥路径没有保留出门装。**`retainRuneStarterItems`（约 5147 行）只在职业选手路径调用（约 5077 行）。绝活哥重新拿到数据时，从不继承已拿到的出门装。
4. **每次键变化都请求两遍。**同一毫秒级出现两次 `specialist_runes_handler accepted`，随后两批出门装。执行时查清第二次请求从哪里来，并去掉重复。

### 修复

P2-1　**绝活哥 / 职业选手的请求键去掉与结果无关的部分。**`specialistRequestTarget` 的 `key` 改为只含真正影响结果的字段（`championId:position`，若后端结果确实受 gameMode / mapId 影响则保留这两个；`tier`、`spellKey` 必须去掉）。`proRequestTarget` 同样核对：结果不依赖技能就去掉 `spellKey`。依赖这些键的地方（`specialistPlayerTabs`、`runeStarterRequests.targetKey`、`specialistRuneFlights / Failures`）同步。验收：同一英雄同一路，应用技能、换技能、进入对局，都**不再**请求 `/specialist-runes`，卡片不进加载态。

P2-2　**绝活哥路径也保留出门装。**绝活哥数据写入 `state.specialistRunes` 前调用 `retainRuneStarterItems(runes, 旧数据)`；旧数据按"英雄 + 分路"找，不按旧的完整键找。

P2-3　**出门装随符文一起返回（后端预取）。**

- `specialistRunes` 算完后，立刻为**第一位绝活哥**的前 3 场预取时间线：走 R177 的前台 6 秒排队上限，2 个 worker。
- `/api/gameplay/specialist-runes` 返回前，把 starter 缓存里已经有的 `starterItemIds` 直接带上（只带缓存命中的，不为此阻塞等待）。
- 前端第二阶段请求保留，用来补没命中的行和切换到其他绝活哥页签的行。它与后端预取在同一个 match 上要合并成一次请求（复用 `cache.loadWithStatus` 的进行中合并，或加一个按 matchID 的 singleflight），不能重复打时间线。
- 额度：36 + 3 = 39，在前台 90 / 120 秒内。只预取 3 场，不预取其他玩家页签。

P2-4　**诊断**：`rune_starter_batch` 增加 `duration_ms`、`prefetched`（命中预取的行数）；`specialist_runes_done` 增加 `prefetch_started`。不记身份。

### 测试

Node：

4. 同一英雄同一路，`spell1Id / spell2Id` 变化、phase 由 ChampSelect 变为 InProgress：不发 `/specialist-runes` 请求，已显示的出门装节点保持不变（DOM 节点同一引用）。
5. 绝活哥数据刷新（同 key / playedAt）：`starterItemIds` 被继承，第二阶段不再请求这些行。
6. `/specialist-runes` 响应自带 `starterItemIds` 的行：首次渲染就是"装备 + 出门装 + 竖线"，不发第二阶段请求；没带的行照旧发。

Go：

7. `specialistRunes` 完成后对前 3 场发起预取；第二阶段同时请求同一 match 时只有一次上游时间线请求。
8. 响应只携带缓存命中的 `starterItemIds`，未命中不等待。

变异：键里放回 `spellKey` → 测试 4 FAIL；去掉 `retainRuneStarterItems` 调用 → 测试 5 FAIL；去掉 singleflight → 测试 7 FAIL。

## P3　组队情况只能在进入游戏后才看到

### 现状（源码 + 日志已核对）

- `applyLivePremadeAssignments`（`gameplay.go` 约 7966 行）显式只在 `InProgress / Reconnect` 计算组队。
- 直接信号 `teamParticipantId` 来自 gameflow 的 `teamOne / teamTwo`。日志显示选人阶段这两个数组**长度都是 0**（11:52:41、11:53:41），进入对局后才有（11:55:27，`team_one_team_participant_id_counts {1:3, 2:1, 3:1}`）。选人会话 `myTeam` 里也没有组队字段。

所以选人阶段拿不到客户端的组队信号。但其中两部分在选人阶段已经可以得到：

1. **本人所在的组队**：大厅 `/lol-lobby/v2/lobby` 的 `members`（日志中选人阶段该接口持续 200）是确定信号。
2. **其他己方玩家之间的组队**：现有的"共同对局推断"（`livePremadeAssignments` 的历史推断部分）用的是战绩，选人阶段己方 5 人战绩已经加载（`with_stats=5`）。

敌方在选人阶段没有任何身份，**无法显示**，这一点不变。

### 要求

- 选人阶段只对**己方**计算组队：
  - 本人的大厅成员按 puuid 映射到己方玩家，作为确定信号（`PremadeSource="lobby"`，`PremadeSessionSignal=true`）。
  - 其余己方玩家用现有历史推断，阈值、覆盖率门槛不变。
- 敌方在选人阶段不分配组队。
- 进入对局后照旧用 `teamParticipantId`。同一组人的组标识尽量保持不变（同一组成员集合沿用同一个分组标签 / 颜色），避免进对局时分组标记跳变。
- 大厅读取失败时只用历史推断，不报错、不新增文字。斗魂 / 竞技场（`arenaMode`）维持现状。
- 展示样式与对局中完全相同，不新增说明文字。

### 测试

Go：

9. 选人阶段：大厅成员 2 人（含本人）在己方：这 2 人同组，来源 lobby；敌方没有分组。
10. 选人阶段大厅只有本人，另两名己方玩家共同对局数达到阈值：这两人按推断同组。
11. 大厅接口失败：只有推断结果，不 panic。
12. 选人阶段到对局：同一成员集合的分组标签不变。

变异：去掉"选人阶段只算己方" → 测试 9 FAIL（敌方被分组）。

## P4　对局页一直抖动、闪烁、像在重新加载

### 证据（日志）

- game 9009800745 选人阶段约 3 分钟（12:17:29–12:20:18）里，后端名单加载约 40 次（每 3–5 秒一次，来源 interval / sse / event），名单指纹（`live_roster_shape.fingerprint`）变化 10 次以上（队友预选 / 锁定英雄都会变）。
- 推荐键在同一阶段变了 4 次（见 P2）。每次都会让符文面板重新请求、重新渲染。
- 前端唯一能说明"哪块 DOM 被重建了多少次"的诊断 `live_render_rebuild`（`gameplay.js` 约 7724–7745 行）**从来没有进过日志**：`runtime.js:182` 的 `reportFlowDiagnostic` 白名单里没有它，直接被丢掉；后端白名单也没有。所以目前无法从日志精确量化抖动，只能结合代码判断。

### 根因判断（代码已核对）

1. `renderLive → updateLivePanels`（约 5337–5440 行）只有"整个面板 `innerHTML` 替换"这一档粒度。名单指纹一变（任何一个队友换英雄），整个「详情」面板 10 张卡片全部重建，所有头像、英雄、装备 `<img>` 都是新节点，于是一起闪。
2. 符文面板在推荐键变化时会先进入加载态再出现（见 P2），而且出门装消失又补回，行内容来回变化。
3. 刷新按钮在每次后台加载时都会在"刷新对局 / 正在刷新…"之间切换（约 5351–5353 行），文字宽度变化会带着工具栏一起抖。

### 修复

P4-1　**先把诊断打通**：

- `runtime.js` 白名单与后端客户端诊断白名单加入 `live_render_rebuild`（只有计数字段：`counts`、`sources`、`total`、`windowMs`、`phase`）。
- 同时增加 `images_recreated`（被替换的 `<img>` 中 `src` 与旧节点相同的数量）和 `rows_replaced`。

P4-2　**名单按行局部更新**：

- 「详情」面板（以及对局页名单）按 `data-player-ref`（或现有的稳定玩家键）逐行比较，只替换 HTML 真的变了的那一行。
- 行内的 `<img>` 若 `src` 不变则保留原节点（只更新属性 / 文本）。整面板重建只在面板结构变化（人数、分组、阵营顺序变化）时发生。

P4-3　**后台轮询不改刷新按钮文字**：只有用户手动点刷新时显示"正在刷新…"；interval / sse / event 来源的加载保持按钮文字不变（加载状态可以继续反映在 `aria-busy` 上）。

P4-4　**符文面板稳定**：靠 P2-1 / P2-2 解决（技能变化不再触发加载态，出门装不再丢失）。另外，出门装补入时如果是"整行替换"，要保证行高不变（`.specialist-game-items` 已有 `min-height`，执行时核对补入前后高度相同）。

### 测试

Node（用现有名单夹具，模拟选人阶段 3 分钟、每 3 秒一次加载，其中 10 次单个队友英雄变化、4 次本人技能变化、1 次进入对局）：

13. 只有变化的那一行被替换；其余 9 行的根节点和 `<img>` 节点保持同一引用；`images_recreated = 0`。
14. 技能变化 4 次：符文面板没有进入加载态，绝活哥行的出门装节点保持不变。
15. 后台加载：刷新按钮 `textContent` 始终为"刷新对局"；手动刷新时变为"正在刷新…"。
16. `live_render_rebuild` 经过 runtime 与后端白名单后能落到日志（端到端测试）。

变异：恢复整面板 `innerHTML` 替换 → 测试 13 FAIL；后台加载也切换按钮文字 → 测试 15 FAIL。

## P5　R177 对线卡片诊断字段全是 0 / 空（小修）

`lane_matchup_candidate_fetch` 在 0.12.43 日志里，`self_position` 永远是 `""`，`enemy_locked_count / enemy_position_known_count / ally_position_known_count` 永远是 0。连 `reason=requested`、`enemy_champion_id=101`、`position=mid` 的那条也是这样，自相矛盾。

前端（约 5695–5739 行）发的是 camelCase 的 `selfPosition / enemyLockedCount …`。执行时核对后端白名单的字段名映射，修正为真实值，并补一条端到端测试：前端发出非零值，日志里能看到相同的非零值。

## 收尾

- 版本递增（0.12.44）；`docs/WORKLIST-INDEX.md` 加 R179；账本 `docs/history/ledgers/r179-execution-ledger.md`。账本里写明本日志已验证的 R177 P1/P2、R178 P1/P2 结论（见"证据范围"）。
- `node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿；变异逐条记录。
- 界面红线：不新增说明文字、tooltip、口径说明。P1 的"暂无玩家信息"是用户明确指定的文案。

## 真机验收（用户，Windows）

1. 选人阶段对方 5 张卡只显示"暂无玩家信息"；进入对局后有隐藏身份的玩家照旧显示"隐藏玩家"。
2. 打开绝活哥：第一位绝活哥的前 3 场，出门装和装备基本同时出现；点"应用所选符文"（连同技能）后，卡片不闪、出门装不消失；进入对局后也不消失。
3. 和朋友组队排位：选人阶段己方就能看到组队标记，进入对局后标记不变。
4. 选人阶段停留在「详情」页 1–2 分钟：队友换英雄时只有那一行变化，其余卡片和头像不闪；刷新按钮文字不跳动。
5. 导出日志，确认里面有 `live_render_rebuild` 事件。
