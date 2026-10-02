# WORKLIST-R168：征召选用列表选人结束未复位（真实前端缓存 bug）+ R167 真机日志复核 + R119 真机补位证据

诊断人：Claude（只读：用户提供的 3 张真实排位截图 + 新诊断日志
`lol-loot-diagnostics-0926-2043.jsonl`（queue 440，含 `champselect_source`/
`champselect_ban_probe`/`live_position_shape`/`counters_shape` 等事件）+ 读仓库
代码，未改仓库代码）。
执行人：GPT。
日期：2026-09-26。
基线：源码 0.12.32（R167 落地后）。

用户在一条消息里提出三件事：① 用两张选人前后截图核对 R167 卡片的真机行为；
② 用同一份日志回填 R119 遗留的真机补位判据；③ 截图三发现"对局已结束、征召
选用列表仍显示队友已选中"的新 bug。按惯例一条消息里的多个问题写进同一份
工单的不同小节。

---

## P1：`suite.js` 里 `champSelectRuntime` 前端缓存在选人结束后不会自动刷新（新 bug，已定位到根因，可直接修）

### 现象（截图三 `28d0b7f0-image.png`）

对局已经结束（用户描述："对局已经结束了，这个选用列表还是显示队友已选中"），
但"征召"面板的"选用序列"列表里，寒冰射手（Ashe）仍标着"队友已经选择"
（`suite.js:629` 的 `"teammate-picked": "队友已经选择"`）。这个残留锁定状态
本该在选人阶段结束时就清空。

### 根因：后端状态本身是对的，问题在前端缓存

**后端**（`backend/champselect.go`）完全正确：

- `handleChampSelectPhase(phase)`（约 636 行）在检测到 `ChampSelect` 阶段
  `entering`/`leaving` 任一转换时都会调用 `resetChampSelectRuntimeLocked()`；
- `resetChampSelectRuntimeLocked()`（约 664 行）把 `r.champSelect`整个替换成
  `newChampSelectRuntimeStore()`，`TeammatePicked` 等状态会被彻底清空。

也就是说，选人阶段一结束，后端的运行时状态**立刻**就是干净的。问题出在前端
没有重新去问后端要这份干净数据。

**前端**（`backend/web/suite.js`）：

1. `state.active` 只在用户正处于"征召"这个顶层区块时才为 `true`
   （`handleLazySection`，约 2071-2083 行：`state.active = event.detail?.name
   === "suite"`）。
2. `deep-legends:gameflow` 阶段变化监听器（约 2104-2110 行）只有在
   `state.active && state.connected` 都成立时，才会清空
   `state.champSelectRuntime = null` 并重新 `loadChampSelect(false)`：
   ```js
   if ((event.detail?.changed || event.detail?.phase) && state.active && state.connected) {
     state.champSelectRuntime = null;
     void loadChampSelect(false);
   }
   ```
   ——**如果选人结束的那一刻用户正在看别的区块（总览/对局详情/收藏等），
   这次重置根本不会触发**，`state.champSelectRuntime` 里选人阶段结束前的
   最后一次快照就这样留在内存里，没人去清。
3. 用户之后再切回"征召"区块时，`handleLazySection` 把 `state.active` 设成
   `true` 并调用 `loadAll()` → `loadActiveTab(false)` → `loadChampSelect(false)`
   （`force=false`）。而 `loadChampSelect` 的提前返回判断（约 888-935 行）是：
   ```js
   if (!force && state.watch && state.champSelectGroups && state.champSelectCatalog && state.champSelectRuntime) {
     renderChampSelect();
     return;
   }
   ```
   只要 `state.champSelectRuntime` 不是 `null`（哪怕内容是选人结束前的陈旧
   快照），这个判断就成立——**直接拿着过期数据重新渲染，根本不会发请求去问
   `/api/champselect/state`**，即使后端那边早就干净了。

**结论**：这是一个**纯前端**的缓存失效 bug，和"选用序列"这个具体视图无关，
而是整个 `champSelectRuntime` 驱动的征召 UI 通病——所有模式分组（排位/
普通/大乱斗/云顶/斗魂）、禁用和选用两种状态，只要"选人阶段结束时用户没有
停留在征召区块"这一条件成立，再次打开征召区块看到的都会是选人结束前的
残影，直到下一局选人重新触发完整刷新为止（因为下一局进入 `ChampSelect`
阶段本身会走同一段 `if (state.active …)` 逻辑，但这次 `state.active` 多半
也不成立，问题会持续复现）。

### 建议方案

`loadChampSelect` 的"信任缓存"条件不该只看 `state.champSelectRuntime`
是否非空，还要知道这份缓存是不是"选人阶段结束之后就该作废"的陈旧数据。
两种改法任选其一，GPT 按现有代码风格挑：

- **方案 A（推荐，改动最小）**：把"阶段变化时清缓存"这一步从
  `state.active` 的门槛里挪出来——不管用户当前在不在征召区块，
  `deep-legends:gameflow` 监听器只要看到阶段离开 `ChampSelect`（或
  `changed` 为真），就无条件执行 `state.champSelectRuntime = null`；
  只在决定要不要**立刻重新拉取**（`void loadChampSelect(false)`）这一步
  才继续用 `state.active && state.connected` 做门槛（没必要在用户看不到的
  区块也发请求）。这样缓存对象本身不会带着过期状态活到下次进入征召区块，
  `loadChampSelect` 的提前返回判断自然会因为 `state.champSelectRuntime`
  是 `null` 而失效，从而触发真正的重新拉取。
- **方案 B**：给 `state` 增加一个独立的脏标记（例如
  `state.champSelectRuntimeStale`），阶段变化时不论 `state.active` 都置位；
  `loadChampSelect` 的提前返回条件里额外要求 `!state.champSelectRuntimeStale`；
  `loadChampSelect` 成功拉取后清掉这个标记。

两种方案效果等价，A 改动更小、代码更少，除非 GPT 判断 A 会和其他读
`state.champSelectRuntime` 是否为 `null` 作为"是否已初始化"判断的地方冲突
（目前搜了一遍现有代码没看到这种冲突），否则优先选 A。

### 测试要求

- Node 单测（`backend/web/suite.test.cjs`，如果还没有这个文件就新建）：
  构造一次"用户在其他区块时收到阶段从 `ChampSelect` 变为其他阶段的
  `deep-legends:gameflow` 事件"，断言 `state.champSelectRuntime` 被清空；
  再构造"之后切回征召区块触发 `loadChampSelect(false)`"，断言这次**确实**
  发出了对 `/api/champselect/state` 的请求（不是命中提前返回直接
  `renderChampSelect()`）。
- 覆盖一下反向场景：用户全程都在征召区块时，行为必须和改动前完全一致
  （不能因为这次改动导致征召区块内多余的重复请求）。
- `go test ./backend`、`node --test backend/web/*.test.cjs` 全绿。

---

## P2：R167「对位克制建议」真机日志复核结果

### 用截图 + 日志对上了具体是哪一局

两张截图（选人中 `f061d6b4-image.png`、准备阶段 `2f435f24-image.png`）和
日志里 `champselect_source`（`queue_id=440`）的 `team_champions` 快照能
逐帧对上：我方 `[圣枪游侠236, 法外狂徒104, 青钢影164, 离群之刺84, 猩红收割者8]`，
敌方 `[齐天大圣62, 刀锋舞者39, 深海泰坦111, 麦林炮手18, 迅捷斥候17]`，和
两张截图里的头像、称号逐一对应，确认这就是同一局。完整时间线（UTC）：

| 时间 | 事件 |
|---|---|
| 12:10:35 | 敌方刀锋舞者(39)锁定 |
| 12:10:38 | 敌方齐天大圣(62)锁定 |
| 12:10:57 | 敌方深海泰坦(111)、我方青钢影(164)相继锁定 |
| 12:11:25 | 敌方深海泰坦(111)+麦林炮手(18)已锁；本地玩家自己的 pick 回合开始（`in_progress=true, champion_id=0`，还没悬停任何英雄） |
| 12:11:44 | 本地玩家悬停英雄 68（诺手？先悬停了一个，未锁定） |
| 12:11:49 | 本地玩家改悬停 8（猩红收割者/弗拉基米尔） |
| 12:11:50 | 本地玩家锁定 8 |
| 12:12:10 | 敌方迅捷斥候(17)锁定，进入 `FINALIZATION`（对应截图二"准备好你的赛前配置") |

### 结论一（确认无误）：标准排位/灵活组排选人阶段，敌方锁定确实实时可见

齐天大圣（敌方，很可能是对方上单）在 `12:10:38` 锁定完成，此时 `phase`
仍是 `BAN_PICK`，距离 `FINALIZATION`（`12:12:10`）还有将近一分半——和
R167 工单里基于上一份日志得出的结论完全一致，这次是另一份独立日志的
二次印证，之前的"选人阶段看不到对方英雄"是我的误判，这次进一步坐实了
纠正后的结论没有问题。

### 结论二（找到一个无法从日志确认、需要 GPT 补一个诊断点的空白）

R167 的候选请求（`ensureLaneMatchupCandidates`，`gameplay.js` 约 5543 行）
只在**当前玩家自己还没有锁定/悬停任何英雄**（`laneMatchupOwnChampionId`
返回假值）时才会去请求 `/api/champions/detail?champion=<敌方英雄>`。这局
里满足这个条件的窗口是 `12:11:25`（自己回合开始，`champion_id=0`）到
`12:11:44`（悬停 68 号英雄）之间，大约 19 秒，此时敌方上单齐天大圣已经
锁定了 47 秒。

但翻遍这段时间的诊断事件，**没有找到任何能确认这次候选请求真的发出过**
的证据：
- `local_request_client` 里唯一归类到 `champions` 端点的 3 条记录全在这局
  比赛的时间窗口之外；
- 唯一能对应"拉取某个英雄的克制数据"的 `counters_shape`/`champion_upstream`
  事件簇，时间是 `12:11:47~12:11:50`——正好卡在玩家**自己**悬停/锁定
  猩红收割者前后，更像是给"自己已锁定英雄"填充推荐面板 `StrongAgainst/
  WeakAgainst`（工单里说的场景 A，这是选人卡片早就有的既有功能）触发的，
  不是给敌方齐天大圣拉候选数据（场景 B，R167 新加的部分）。

这不代表功能一定没生效——`ensureLaneMatchupCandidates` 只在渲染
`renderRecommendationArea` 的"insight"页签时才会被调用到（`gameplay.js`
约 4540 行 `void ensureLaneMatchupCandidates(state.live);` 挂在每次
`/api/gameplay/live` 轮询之后，轮询本身在选人阶段确实每隔几秒都在跑，
日志里 `live_load_cost` 在 `12:11:19` 之后到 `12:11:44` 之间应该有若干次
轮询），所以更可能的情况是：用户当时没有停留在"对位克制建议"所在的页签
上，或者这次悬停/决策速度太快（从自己回合开始到悬停只有 19 秒），根本没给
这张卡片喘息的时间就已经悬停了别的英雄。但也不能排除请求逻辑本身有条件
没对上（比如 `laneMatchupContext` 依赖的 `self.position`/敌方 `position`
字段在这局里没能及时解析出 `top`）。**现有诊断埋点不够细，没法在事后
把这两种可能分开。**

### 建议方案

1. 给 `ensureLaneMatchupCandidates` 加一个专属诊断事件（例如
   `p.diag({"event": "lane_matchup_candidate_fetch", "enemy_champion_id":
   …, "position": …, "tier": …, "rows": …})`），在请求成功/失败/命中缓存
   三种分支都打一条（用不同的 `reason` 字段区分），这样以后一份日志就能
   直接确认这张卡片有没有真的发起过候选请求，不用再像这次一样靠旁证拼凑。
2. 这个诊断点补上之后，麻烦用户下次真机验证时**额外截一张选人阶段"对位
   克制建议"卡片本身内容的截图**（不只是选人前后的敌方头像列表），这样
   才能直接肉眼确认卡片是否渲染、渲染的候选英雄对不对，不用再靠日志间接
   推断。
3. 不建议改动 `laneMatchupOwnChampionId` 的现有"自己悬停即停止推荐候选"
   逻辑——这是合理的产品设计（已经在考虑一个英雄了就没必要再推候选），
   只是这次真机验证顺带发现它把候选窗口压得比较短，属于预期内的正常
   行为，不是 bug。

---

## P3：R119 遗留的真机补位判据——这份日志给出了一条更直接的真机证据，但不是工单原本要的那种

> **2026-09-26 追加**：用户随后又提供了一份打开过总览页的新日志
> （`lol-loot-diagnostics-0926-2139.jsonl`），P3-2 原本要的历史战绩口径
> **已经有真实数据填进 `r119-execution-ledger.md` §5 了**，下面这一节的
> "先说清楚……填不了"仅作为诊断过程记录保留，**结论以账本 §5 表格正下方
> 的"判读"段落为准**：已知的补位（青钢影/Camille）落进了 `position_mismatch`
> 桶，没有落进 `autofill_candidates` 桶，首次真机证实了 §6 风险项 2 早就
> 预判的"两值不一致会被保守地判成换位、不打标签"确实会发生；同时这份日志里
> 全部 23 条快照 `autofill_candidates` 无一例外是 0，`autofill_no_evidence`
> 也全部是 0。样本量仍然只有 1 个真实补位案例，落在"注定漏标"的那一桶里，
> 还没有覆盖到"整场 `individualPosition` 缺失"的那一桶，`autofillLabelGate`
> 暂不具备打开的依据，继续保持 `false`。GPT 不需要再等用户补这份数据了，
> P3-2 本身可以视为"已有第一条真实样本、仍需更多样本"而不是"完全空白"。

### 先说清楚：工单 P3-2 要的历史战绩口径这次日志里没有

`docs/r119-execution-ledger.md` §5 要的是打开自己的**总览页**（或任意国服
玩家的生涯页）之后，`sgp_match_history_succeeded`（或
`sgp_summary_history_*`）事件里的 `autofill_candidates`/`position_mismatch`/
`autofill_no_evidence` 三个计数——这是基于 match-v5 历史对局的
`individualPosition`/`teamPosition` 做的**事后推断**口径。这份新日志里
完全没有 `sgp_match_history_succeeded`/`sgp_summary_history_*` 这两个事件
（说明这次会话没有打开过总览/生涯页），所以 P3-2 表格**目前仍然填不了**，
这不是本轮能替用户做的事——需要用户下次真机验证时，除了打游戏，也记得
打开一次自己的总览页（哪怕只是切一下 tab），再导出日志。

### 但日志里意外抓到了一条更直接、更权威的真机补位证据

`live_position_shape` 事件（这局对局开始后的 `12:12:49` 和重连时的
`12:43:07`，同一局 `game_id=9001498310`）里，`selected_role_counts`
这个字段（来自 `backend/gameplay.go:5480` 附近的
`livePositionShapeDiagnostic`，直接读 LCU `/lol-gameflow/v1/session` 里
`GameData.TeamOne/TeamTwo` 每个玩家的 `SelectedRole` 原始字段）里有一条：

```
"UTILITY.AUTOFILL.JUNGLE.MIDDLE.FILL": 1
```

这是 **Riot 客户端自己**吐出来的字符串，字面直接带着 `AUTOFILL`——意思是
这一局 10 个人里有且仅有 1 个人，报名时主选 `JUNGLE`、次选 `MIDDLE`，
最终被分到了 `UTILITY`（辅助位），且客户端自己标注这是一次补位。这不是
`individualPosition` 那种事后统计推断，是**选人当下、Riot 官方 API 直接
给出的补位事实**，可信度比 R119 整个工单原本依赖的历史推断口径更高。

结合截图二能看到的我方阵容——辅助位（`辅助`一栏）选的是**青钢影
（Camille）**，一个几乎不出现在辅助位的英雄——这条 `UTILITY.AUTOFILL`
记录大概率就是这位辅助玩家：他大概率是本来想打野或中路，被补位到了辅助，
只能选一个自己能凑合出装的英雄。但 `selected_role_counts` 是对全部 10 人
做的聚合计数，日志里没有把这条记录和具体某个 `puuid`/召唤师名绑在一起，
所以"就是青钢影那个辅助"是**合理推断，不是能从日志字段直接坐实的定论**。

### 建议方案

给 GPT 两条平行的后续动作，不互相排斥：

1. **短期**：把这条真实证据原样记进 `r119-execution-ledger.md` §5 的
   观测栏，标注清楚"证据来源是 `live_position_shape`/`selected_role_counts`
   的 `AUTOFILL` 标记，不是 P3-2 原定的历史 `autofill_candidates` 口径，
   且未能定位到具体是哪个玩家"，保留 P3-2 原表格继续等用户下次带上总览页
   访问记录的日志来填。
2. **中期（建议新开一个可选方向，不是本工单强制要求）**：`live_position_
   shape` 里这条 LCU 原生 `SelectedRole` 字段本身就是**当前对局**的直接
   补位事实，不需要 R119 整套"从历史战绩推断"的迂回逻辑，而且 §6 风险项 4
   "实时对局视图未覆盖"里已经预留了这个方向。值得让 GPT 评估一下：能不能
   直接拿 `SelectedRole` 里的 `AUTOFILL` 标记（配合 `PUUID` 精确绑定到具体
   某个 `gameplayLivePlayer`，而不是像现在这样只在 `livePositionShapeDiagnostic`
   里做匿名聚合计数）作为实时对局里"这个人被补位了"标签的数据源，作为对
   历史推断口径的补充或替代。这比等一堆真实对局去校准历史推断的准确率
   要直接得多，但目前 `SelectedRole` 是否稳定可靠（比如非标准排位模式下
   这个字段是否存在、`AUTOFILL` 标记是否只在特定客户端版本才有）还没验证
   过，需要 GPT 另外找几局（含非排位模式）日志交叉确认字段稳定性，再决定
   要不要正式立项。

### 测试要求（仅针对 1，把新证据记进账本的部分）

不涉及代码改动，只是文档更新，不需要新测试；如果 GPT 采纳"中期"方向并
落地成代码，再按新工单的要求另行补测试。

---

## P4：对局详情里两处问题（截图追加，2026-09-26）——P4-1 是文案混淆，P4-2 是真的漏抓数据

用户看的是"对局详情"页里的双栏玩家列表（`renderLivePlayer`，`gameplay.js`
约 5408 行起）截图，指出两处文案：

### P4-1：敌方"隐藏玩家"/"隐藏身份"文案把"选人阶段还看不到"和"玩家真的设了隐私"混为一谈

截图里对方 5 人全部显示"隐藏玩家"+"隐藏身份"徽章+"客户端未公开该玩家"，
但这只是因为选人阶段敌方身份字段本来就还没解析出来（`gameName`/`displayName`
为空——见 R167/R153 之前的结论：排位选人阶段敌方的召唤师身份要等到
`InProgress` 才会被完整解析，选人阶段能看到的只有 `championId`/`position`
这些字段），不是这几位玩家真的开了"隐藏姓名"隐私设置。

**根因**（`backend/gameplay.go:7632`）：

```go
hidden := strings.EqualFold(raw.player.NameVisibilityType, "HIDDEN") ||
  (strings.TrimSpace(summoner.GameName) == "" && strings.TrimSpace(summoner.DisplayName) == "")
```

这一行把两种完全不同的成因**合并成同一个 `Hidden` 布尔值**：
- 真正的原因一：LCU 明确报告 `NameVisibilityType == "HIDDEN"`（玩家自己设置
  了隐私姓名）；
- 原因二：这一刻压根还没拿到这个人的 `GameName`/`DisplayName`（选人阶段的
  正常阶段性缺失，不是玩家的选择）。

前端（`gameplay.js:5425` `hiddenChip`、`:5428` `emptySummary`、`:1958`/`:2124`/
`:2682`/`:6805`-`:6807` 等多处 `player.hidden` 判断）全都只认这一个布尔值，
文案统一写"隐藏玩家"/"隐藏身份"/"客户端未公开该玩家"，没法区分这两种成因。

**建议方案**：给 `gameplayLivePlayer`（或紧贴着 `Hidden` 字段）加一个新字段
区分成因，例如 `IdentityUnresolved bool`（选人阶段身份还没解析出来）和
`Hidden bool` 只保留给 `NameVisibilityType == "HIDDEN"` 这一种真实原因；
前端两种状态给不同文案——比如身份未解析显示"选人中"/"身份待公开"之类
的措辞，真正隐私隐藏才继续用"隐藏身份"。具体措辞 GPT 定，只要求做到
"选人阶段暂时看不到"和"玩家真的设了隐私"这两种情况在界面上说法不一样，
不能再共用一套文案。这个区分同样适用于所有读 `player.hidden` 的地方
（详情弹窗、対局列表、推荐面板等），不要只改截图里这一处，漏掉其他复用
同一个 `hidden` 语义的位置。

### P4-2：我方战绩每行的"近 N 局"局数太少——不是文案问题，是真的漏抓了数据

> **更正**：一开始误判成文案缺失，用户纠正为"局数太少（漏抓）"——队友
> 明明最近打了不少局，面板却只显示 1/2/6 局，根因在数据抓取的时间窗口
> 太窄，不是措辞问题。

**根因**：`backend/gameplay.go:7628` 起，选人/对局详情里每个玩家的战绩
是这样算出来的：

1. `loadLivePlayerMatches`（`backend/gameplay.go:8117`）调用
   `loadGameplayHistoryContext(ctx, client, playerRef, isCurrent, 0, 10, false)`——
   **写死只拉最近 10 场**，这 10 场是这个玩家不分队列的全部最近对局
   （`/lol-match-history/v1/products/lol/{player}/matches?begIndex=0&endIndex=9`），
   不是"最近 10 场当前队列的对局"。
2. 这 10 场原始数据到手后，才在 `recentGamesFromMatches`
   （`backend/gameplay.go:7133`）里按 `queueID` 过滤——`if queueID > 0 &&
   match.QueueID != queueID { continue }`——并且上限还砍到 `limit=8`
   （`recentGamesFromMatches(matches, playerRef, 8, response.QueueID)`，
   `backend/gameplay.go:7635` 附近）。
3. `recentRankedRecord(recentGames)`（`backend/gameplay.go:7118`）最后
   算出的 `games` 数量，就是"这 10 场里恰好匹配当前队列的那几场"。

**问题就在这里**：如果一个玩家最近打得比较杂（灵活组排、单双排、匹配、
大乱斗混着打），"最近 10 场不分队列"这个窗口里能落进"当前这局的队列"
的场次自然就很少——不是因为这个人打得少，是因为窗口本身太窄、又不是
按队列去抓的，先天就会漏。截图里"近 1 局"/"近 2 局"的队友完全可能是
本身很活跃、只是最近这 10 场混杂了别的队列。

**建议方案**（任选其一或组合，GPT 按现有代码结构和性能预算权衡）：

- **方案 A**：把 `loadLivePlayerMatches` 传给 `loadGameplayHistoryContext`
  的 `count` 从 10 调大（比如 20-30），换取同队列命中率提升，代价是
  这是选人/对局详情页对 10 个玩家并发请求的路径，调大窗口会增加每个
  请求的响应体积和耗时——GPT 需要评估这条路径当前的耗时预算（可以参考
  `live_load_cost`/`overview_load_cost` 这类既有诊断事件先摸一下现状再
  决定调多大），不是越大越好。
- **方案 B**：请求阶段就按队列过滤（如果 LCU 或 SGP 接口支持按 queueId
  查询战绩），直接按"最近 N 场当前队列的对局"去拉，而不是"最近 10 场
  不分队列再筛"，这样命中率不受混队列影响，但要先确认这两个数据源
  （LCU `match-history` 和 SGP `matchHistory` 回退）本身是否支持队列
  维度的服务端过滤，不支持就退回方案 A。
- 不管选哪个方案，都不要改变"当前模式最新战绩"这个措辞本身的语义——
  只是让这句话对应的数据真的把这个队列里能抓到的最近对局都抓全，而不是
  被一个和队列无关的固定窗口砍掉。

### 测试要求

- P4-1：Node 单测覆盖两种场景——① `NameVisibilityType == "HIDDEN"`（真实
  隐私）应继续显示"隐藏身份"一类文案；② `GameName`/`DisplayName` 为空但
  `NameVisibilityType` 不是 `HIDDEN`（选人阶段身份未解析）应显示新的、
  不同的文案，两种情况渲染结果必须能被断言区分开。
- P4-2：Go 单测构造"最近 10 场里只有 2 场匹配当前队列，但更早的对局里
  还有更多同队列场次"这种数据形状，断言修复后 `recentRankedRecord`/
  `AutofillCandidates` 之类下游计数能拿到超过原来窗口能覆盖的场次（具体
  断言随 GPT 选的方案变，A 方案断言窗口变大后数量变化，B 方案断言按队列
  过滤后数量不再受窗口大小限制）；同时要有一个回归用例确保"确实只打过
  很少当前队列对局"的玩家不会被过度拉取或误判。
- `go test ./backend`、`node --test backend/web/*.test.cjs` 全绿。

---

## 验收标准

- P1：`node --test backend/web/*.test.cjs` 全绿，新增用例覆盖"选人结束时
  不在征召区块→再次进入征召区块必须重新拉取"和"全程在征召区块行为不变"
  两种场景；`go test ./backend` 全绿（P1 不涉及后端改动，这里只是回归）。
- P2：不要求 GPT 改动 `laneMatchupOwnChampionId` 的现有逻辑；只要求补上
  P2 建议方案 1 的诊断埋点，并在 `r167-execution-ledger.md` 里补一节记录
  这次真机复核的结论（结论一成立、结论二是待补诊断的已知空白）。
- P3：`r119-execution-ledger.md` §5 观测栏按建议方案 1 补一条记录；P3-2
  原表格保持"待填"，不要用这条新证据顶替；中期方向只需要在账本或新的
  探测文档里留一段评估结论，不强制立项写代码。
- P4：`Hidden`/身份未解析两种成因在界面文案上必须能区分；我方每行战绩的
  局数统计必须修掉"最近 10 场不分队列再筛队列"这个窗口漏抓问题（方案 A
  或 B 任一均可），不是改文案；两处改动都要有测试守着。

---

## 给用户的备注（不需要 GPT 处理，仅供参考）

- 下次真机验证 R167 时，麻烦：① 除了选人前后的敌方头像截图，也截一张
  "对位克制建议"卡片本身的内容；② 打一局单/双排或灵活组排，中途也顺手
  切一下"总览"页（这样才会触发 `sgp_match_history_succeeded`，R119 的
  P3-2 表格才有数据可填）。
- 这份日志显示的补位证据大概率是我方辅助位那位玩家，但不是 100% 能从
  日志字段坐实——如果你记得当时语音里队友确实提过"被塞辅助了"之类的话，
  麻烦告诉我，我可以把这条也一起记进账本，作为人工确认的佐证。
