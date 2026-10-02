# WORKLIST-R153：R116 探测真机回填——判据一卡在节点扫描上限、判据二/三可以定论

诊断人：Claude（只读：用户上传的三份真机文件 + 读代码核对实现，未改仓库代码）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.19，`backend/mayhem_sampler.go`、`backend/augment_contract_probe.go` 均落在提交 `1b5aa72a`（R136 P2 实现），仓库 HEAD `35788b9f`（R152 之后）。
状态：R153 代码与本机验收通过；判据一第二轮已由 R155 收尾并清理临时探针，`skinName` 修复仍待 Windows 真机复核。执行细节见 `docs/r153-execution-ledger.md`、`docs/history/ledgers/r155-execution-ledger.md`。
触发：用户打完两局真实 5v5 海克斯大乱斗（KIWI，queue_id 2400）并导出诊断日志，另跑了一次 R116 判据一探测（`r116-augment-probe.exe`），把三份文件发回来回填 `docs/r116-probe-findings.md`。

证据文件（用户上传，本单引用的都是其中的原始字段）：
- `lol-loot-diagnostics-0925-1427.jsonl`：0.12.19 真机诊断日志，4,139 条，`run_id=ee2c19ab41d3669617cfd8b2`，05:33:31Z–06:27:21Z，含两局完整海斗对局（`game_id=8999110339`、`game_id=8999150286`）。
- `augment-probe.jsonl`：R116 判据一探测输出，`run_id=90412ac90be0d926f03432c5`，`trace_id=a0b512f620d71407490f3416`，2026-09-25 06:14:53Z。
- `r116-run-20260925-140820.log`：判据一探测的一次失败调用记录（人工笔误），供归档，不改代码。

请把三份文件放进 `docs/r116-validation/`（按 §6 已有的建议命名，判据一探测重命名为 `augment-contract-probe.jsonl`，海斗对局日志重命名为 `mayhem-live-shapes.jsonl`），账本引用这个路径。

## 执行纪律（延续 R117–R152）

- 每项验收判据都要做**对抗变异**：改回原样或换一种等效写法，对应测试必须 FAIL。
- 变异在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- 本单不涉及界面文案，不触碰 CLAUDE.md 红线。
- **`backend/augment_contract_probe.go` 与 `augment_contract_probe_test.go` 本轮不删除**——判据一仍未达成 `negative_conclusive`，删除条件（判据一结论回填 §2.4 后）没有满足。`backend/mayhem_sampler.go` 不属于这次侦察代码，是 R136 P2 的正式实现，本单不涉及删除。

---

## P1　判据一：探测代码本身按 R136 P2 的规格正确重写了，但真机这一轮撞上了递归节点扫描上限，仍不能定论

### 证据

`augment-probe.jsonl` 第 4 条事件（`contract_kind=help-full`，`status=200`，`body_bytes=3032902`）：

- `events_scanned=749=events_length`、`functions_scanned=1468=functions_length`、`types_scanned=3578=types_length`——三个数组**顶层元素**都数满了；
- 但 `scan_limit_reached=true`，因此 `contract_read=false`、`negative_conclusive=false`；
- `matched_items` 命中 **21 条**（4 条 `functions`、17 条 `types`），`matches_truncated=false`、`static_catalog_count=0`。

逐条看这 21 条命中（全部列在下面），**没有一条是"按局/按玩家的候选或已选端点"**：

| 命中 | 性质 |
|---|---|
| `DeleteLolCosmeticsV1SelectionTftAugmentPillar` / `PutLolCosmeticsV1SelectionTftAugmentPillar` / `GetLolCosmeticsV1InventoriesBySetNameAugmentPillars` | TFT「小小英雄」海克斯基座**外观**自定义端点，与 League 斗魂/海斗的游戏内海克斯机制无关 |
| `GetLolInventoryV1CherryInventory` | Cherry 模式的战利品/库存清单端点（GET 自己已拥有的物品），不是三选一候选 |
| 其余 17 条 `types`（`LolChampionsCollectionsChampionSkinAugment(s)`、`LolCollectionsGameDataChampionSkinAugment(s)`、`LolCosmeticsCosmeticsTFTAugmentPillar*`、`LolCosmeticsTFTAugmentPillar*ViewModel`、`LolCollectionsCollectionsSummonerBackdropAugments`、`LolEndOfGameTFTEndOfGameCustomAugmentContainerViewModel`…） | 全部是「皮肤扩展外观（skin augment）」「个人资料背景」「TFT 海克斯基座」相关的**数据类型**，同样与本方案要找的「实时海克斯三选一」无关 |

### 根因

`augment_contract_probe.go:264-320` 的 help-full 扫描是**双层预算**：外层 `augmentProbeHelpElementLimit=10000`（按数组元素个数计）本轮没触发（749+1468+3578=5795＜10000，所以三个 `*_scanned` 都等于 `*_length`）；但内层 `augmentProbeHelpNodeLimit=100000`（对每个元素递归遍历其全部子字段，含 `types[].fields[]`、`types[].values[]` 这类嵌套数组，跨三个数组共用同一个全局计数器）在扫到 `types`（数组最大、且很多类型带 `fields`/`values` 子列表）时提前打满。`visit()`（270-272 行）一旦 `totalNodes > augmentProbeHelpNodeLimit` 就立即返回、**连该元素的 `name` 字段都不再检查**，只是外层按元素个数计的 `Scanned[group]` 计数器（253-256 行，与 `totalNodes` 是两个独立计数器）仍然照常 +1，所以事件里 `types_scanned==types_length` 这个「看起来扫完了」的信号具有误导性——真实情况是相当一部分 `types` 元素在节点预算打满之后**连名字都没被看过**。`contractRead = stats.Complete`（320-326 行）已经正确地把 `LimitReached` 计入「未完成」，所以 `contract_read=false` 是对的、没有谎报确证；但这也意味着**这一轮拿不到判据一想要的完整负证据**，因为不能排除预算耗尽之后的 types 元素里恰好藏着一个真正的候选端点。

### 修复方向（只改探测代码，属于允许改动的侦察代码范围）

1. **不要把 `values` 数组一起深度递归**：海克斯/端点的 `url`/`path`/`uri` 字段只可能出现在 `fields`（结构体字段描述）里，`values`（多是枚举取值表，例如英雄 ID、天赋 ID 列表）体量大但从不含路径字段。`visit()` 递归到某个类型对象时，跳过键名为 `values` 的子节点（仍然计入 `name`/`fields` 的检查），预计能把 `types` 消耗的节点数砍掉一大截。
2. 在此基础上，给三个数组（`functions`/`events`/`types`）**各自独立**一份节点预算（而不是全局共享一个 `augmentProbeHelpNodeLimit`），避免前两个数组把预算耗尽导致 `types`（本来就最大、最该关心）陪着一起打折扣。
3. 事件里新增 `<group>_nodes_visited` 与 `<group>_limit_reached`（分组粒度），让下一轮真机读数时能直接看出"是哪个数组、消耗到什么程度"，而不是只有一个笼统的布尔值。
4. 上述改动后，用本轮真实响应体的结构（`root_shape={events:array:749,functions:array:1468,types:array:3578}`，`types_element_keys` 含 `fields`/`values`）做一个体量相近的夹具，跑一遍确认 `LimitReached` 不再触发、`Complete=true`（如果仍然触发，说明预算本身要继续调大，而不是继续假装「扫完了」）。

### 验收

- 新增/修改单测：用一份构造夹具（`types` 里若干元素带大 `values` 数组、个别元素带 `fields` 里的 `url`/`path` 字段），断言：① 命中的 `url`/`path` 字段无论出现在预算消耗顺序的前段还是后段都不漏判；② `values` 子数组的元素不计入节点预算；③ 三个数组独立计数，一个数组打满不影响另外两个数组的 `Scanned`/`LimitReached`。
- 变异：恢复"全局共享节点预算＋递归 `values`"的旧写法，上面的断言必须 FAIL。
- `TestR116AugmentContractProbeMutation` 等既有用例保持通过。
- 交叉编译新的 `dist/probes/r116-augment-probe.exe`，请用户在客户端**已登录、不需要进对局**的情况下重新跑一次判据一（步骤同 R136 §7/§6.A），产出新的 `augment-contract-probe.jsonl`。如果这一次 `contract_read=true` 且 `count==0`（且静态目录之外没有命中），才允许在 `docs/r116-probe-findings.md` §2.4 写「确证无端点」；如果 `count>0`，把新的命中列表发回来，由 Claude 逐条人工判读（多半还是本单列出的 TFT/外观类，但不能不看就假设）。
- 在这轮真正拿到 `contract_read=true` 之前，**不要删除 `augment_contract_probe.go`**。

### 本轮先回填的内容（`docs/r116-probe-findings.md` §2.4，如实写「未完成」，不要拔高成确证）

按本单证据部分的表格填入 `contract_kind=help-full` 那一行的 `status=200`、`result=ok`、`body_bytes=3032902`、`count=21`、`contract_read=false`、`negative_conclusive=false`；openapi-v3/v2 两行按 `augment-probe.jsonl` 前两条事件填（均 404、`contract_read=false`）。命中列表按上面证据表抄入，「是否已知无关命中」全部填「是（外观/TFT/库存类）」。**判据一结论**填「D 的变体——三个来源里 openapi 两个不可读、help-full 可读但递归节点预算打满未完成，暂不出结论；P1-1 保持未判定」，不要按现有 §2.3 的 A/B/C 硬套，本单 P1 的修复完成并重新真机验证之前不关闭这一项。

---

## P2　判据二：确认可行，同时顺手核实了 allgamedata 里出现的 `<unknown-key>`——大概率是 `skinName` 漏收进了安全字段表，不是海克斯相关

### 证据（判据二本体）

两局海斗（`game_id=8999110339`/`8999150286`）的 `live_client_playerlist_shape` 均 `http_status=200`、`player_count=10`、`result=ungrouped`，`element_keys` 与 §1.5 记录的斗魂 19 键基线**逐字一致**（含 `items`），没有 `<unknown-key>`：

```
championName, isBot, isDead, items, level, position, rawChampionName, rawSkinName,
respawnTimer, riotId, riotIdGameName, riotIdTagLine, runes, scores, skinID, skinName,
summonerName, summonerSpells, team
```

`live_client_allgamedata_shape.arrays["$.allPlayers[].items"]`（两局一致）的元素结构是
`canUse/consumable/count/displayName/itemID/price/rawDescription/rawDisplayName/slot`，与判据二判定阈值（§3.2）要求的 `itemID`/`slot`/`count` 一套完全吻合。

**判据二结论：可行**——海斗 `/liveclientdata/playerlist` 与斗魂一样带 `items`，且元素结构一致。P1-4「下一步出装」保留在 R116-D，不需要新增判定分支。

### 证据（顺手核实的疑点）

同一条 `live_client_allgamedata_shape` 里，`$.allPlayers` 的 `element_keys` 却是（两局一致，只是元素顺序不同）：

```
<unknown-key>, championName, isBot, isDead, items, level, position, rawChampionName,
rawSkinName, respawnTimer, riotId, riotIdGameName, riotIdTagLine, runes, scores, skinID,
summonerName, summonerSpells, team
```

比 playerlist 的 19 键**少了 `skinName`、多了 `<unknown-key>`**，其余 18 个完全对上。按 §3.2/§3.3 的判据，出现 `<unknown-key>` 本该升级为独立调研项、人工判读是否与海克斯相关——但在动手立项之前，先核对了产生 `<unknown-key>` 的代码：

`backend/arena_truth_diagnostics.go:249-256` 的 `arenaShapeKey` 是一张写死的安全字段允许表（白名单，共 ~100 个），不在表里的键一律收拢成 `<unknown-key>`，避免把可能带身份信息的动态键名抄进日志。逐字核对这张表，**`skinName` 确实不在里面**（`rawSkinName`、`skinID` 在，`skinName` 漏了）。而 `<unknown-key>` 那一项在两局里的 `field_shapes` 全部是**变长非空字符串**（6～32 字符），没有一个是数组/对象——如果这是海克斯相关字段（比如选中的海克斯 ID 列表），形态应该是数组而不是单个字符串；反而与"皮肤的展示名"这种短字符串字段的形态高度吻合，且长度分布明显短于同时出现的 `rawSkinName`（30+ 字符，长格式内部标识符），符合"短显示名 vs 长内部名"的常见搭配。

### 修复方向

1. 把 `skinName` 加进 `arena_truth_diagnostics.go:250` 的 `fields` 白名单（斗魂基线 §1.5 本来就承认这是已知的、无害的字段，纯粹是这张表当初漏收）。
2. 加回归测试：用一份含 `skinName` 字段的 `allPlayers` 夹具跑 `liveClientAllGameDataShape`，断言 `element_keys` 里出现字面量 `"skinName"`、不再出现 `<unknown-key>`。变异：从白名单删掉 `skinName`，断言必须 FAIL（证明测试真的在盯这张表，不是碰巧过）。
3. 顺手确认一下这张白名单里还有没有类似的、已知无害但漏收的字段（比如 §1.5 斗魂 19 键里的另外 18 个，一一核对是否全在表里——目前核对下来 `skinName` 是唯一缺的一个）。

### 验收

- 新增测试如上，变异 FAIL、修复后 PASS。
- 交付新的探测/诊断包后，请用户**再打一局海斗**（不需要专门为这条重打，跟下一轮真机验收合并即可），确认这次 `$.allPlayers.element_keys` 里 `<unknown-key>` 消失、`skinName` 出现，两局历史样本作为旁证写进账本即可，不强制为了这一条单独安排一局。
- `docs/r116-probe-findings.md` §3.4 按上面两条证据表回填：`element_keys` 抄 playerlist 的 19 键；判据二结论填「可行」；顺手核实一栏写明 `<unknown-key>` 已定位为 `skinName` 白名单遗漏，附上修复后的确认状态（本轮先填「已定位待复核」，等下一轮真机确认消失后改「已复核，非海克斯相关」）。

---

## P3　判据三：确认结论是「选人阶段看不到对方阵容」——`their_team_length>0` 但 `championId` 仍是 0，`gameplay.go:4451` 附近的白名单注释只写了半个条件，有误导未来改动的风险

### 证据

两局的 `lcu_champ_select_session_shape`（`game_mode=KIWI`、`queue_id=2400`，真实 5v5 匹配，非自定义房间）：

| 局 | `their_team_length` | `their_team_nonzero_counts` |
|---|---|---|
| 1（05:40:45） | 5 | `{cellId:4, nameVisibilityType:5, team:5, wardSkinId:5}` |
| 2（06:10:05） | 5 | `{cellId:5, nameVisibilityType:5, team:5, wardSkinId:5}` |

两局 `their_team_nonzero_counts` 里都**没有 `championId`**（即该字段的非零计数为 0）——对方阵容在选人阶段能看到"有 5 个位置"，但看不到"选了谁"，`puuid`/`gameName`/`summonerName`/`spell1Id`/`spell2Id`/`championPickIntent`/`selectedSkinId` 等同样全是 0。

对照 `docs/r116-probe-findings.md` §4.2 的判据（**两个条件都要满足**）：

```
their_team_length > 0 且 their_team_nonzero_counts 里 championId 非零 > 0 → 能看到对方阵容
their_team_length == 0（或非零计数全 0）                                  → 看不到
```

`their_team_length=5`（满足前半句）、但 `championId` 非零计数 = 0——落在第二支「非零计数全 0」的判词里，**结论是"看不到"**，尽管字面上 `their_team_length` 并不是 0。

对照之后，游戏进入 `InProgress` 后的 `lcu_gameflow_session_shape` 显示 `team_one`/`team_two` 都是 `length=5` 且 `championId`/`puuid`/`selectedPosition`/`selectedRole`/`summonerId`/`profileIconId`/`teamParticipantId` 全部非零计数=5——**进入对局后双方阵容才完整可见**，这与 R116-D 现有的"InProgress/Reconnect 的 10 人来自 `session.GameData.TeamOne/TeamTwo`，是既有能力"完全吻合，不需要改代码。

### 为什么这条要单独写一节，而不是抄完表就完事

`backend/gameplay.go:4443-4460` 的 `gameplayRosterMatchupPhases` 注释这样写：

> "真机证实 `their_team_length > 0` 之后，放开的位置就是这个白名单：加上 'ChampSelect' 即可"

这句注释**只复述了判据的前半句**，没有提第二个条件（`championId` 非零）。本轮真机 `their_team_length` 恰好是 5（>0），如果 GPT 只看这条代码注释、不回头核对 `docs/r116-probe-findings.md` §4.2 的完整判据，很可能会误以为条件已经满足，把 `"ChampSelect"` 加进白名单——那样上线后选人阶段的克制/协同模块会拿到 5 个"位置存在但 `championId=0`"的空壳对象，渲染出全空或错误的对位建议，而不是工单设计的"没有就整块隐藏"。

### 修复方向

把 `backend/gameplay.go:4451-4452` 的注释改成完整复述两个条件（或者直接引用 `docs/r116-probe-findings.md §4.2`），例如：

```go
// 真机证实需要同时满足两个条件才放开：their_team_length > 0 **且**
// their_team_nonzero_counts 里 championId 非零计数 > 0（见
// docs/r116-probe-findings.md §4.2）。R153 的真机数据是 their_team_length=5
// 但 championId 非零计数=0，仍不满足，ChampSelect 继续不进白名单。
```

不改 `gameplayRosterMatchupPhases` 白名单本身（`"ChampSelect"` 不加入）。

### 验收

- 这是文档/注释改动，没有可对抗变异的代码分支；验收就是 diff 里能看到新注释完整复述了两个条件，且白名单常量本身没有改动（`git diff` 里 `gameplayRosterMatchupPhases` 的值不变）。
- `docs/r116-probe-findings.md` §4.4 按上表回填两局数据；**判据三结论**填「`==0`（对方 `championId` 全 0，视同看不到）」；对 P1-2/P1-3 的处置维持"只在 InProgress 出现，选人阶段不做克制/协同展示"，不算回退，和 §4.3 第二行的既定动作一致。
- §5 交叉核实表一并回填：两局 `game_id`、`http_status=200`、`success=true`、`top_level_keys=[activePlayer, allPlayers, events, gameData]`（不含 augment/cherry 字样）、`$.allPlayers[].items` 的 `element_keys` 见 P2、「是否出现 `<unknown-key>`」填「是，见 P2（已定位为 `skinName` 白名单遗漏）」。

---

## P4　（低优先级，纯记录）判据一探测的一次失败调用是人工笔误，不是代码问题

`r116-run-20260925-140820.log` 显示 14:08:20 本地时间那次调用报 `flag provided but not defined: -test`，随后打出的是 Go 测试二进制的 `-test.*` 用法说明。对照命令行文本 `.\r116-augment-probe.exe -test.run TestR116AugmentContractProbeLiveCl...`（PowerShell 换行截断），报错的是裸 `-test`（不是 `-test.run`）——最可能是复制/手输时 `-test.run` 中间多打了一个空格变成 `-test .run`，Go 的 flag 解析器把 `-test` 当成一个不存在的独立参数。约 6 分钟后（06:14:53Z）同一进程重新正确调用，产出了本单分析的 `augment-probe.jsonl`，说明这只是一次一次性的输入笔误，**不需要改代码**。

建议（可选，不阻塞本单其它项）：下一版 README 里把判据一/二/三的命令行整理成可以整段复制的代码块（如果还不是的话），并提醒不要在 PowerShell 里手动分段敲这条命令，减少同类笔误。

---

## 验收总表

| 项 | 判据 |
|---|---|
| P1 | help-full 扫描改为分组独立节点预算、跳过 `values` 深度递归、新增 `<group>_nodes_visited`；夹具+变异验证不再漏判；重新真机跑出 `contract_read=true` 之前不删探测代码、不在 §2.4 写确证结论 |
| P2 | `skinName` 补进 `arenaShapeKey` 白名单，新增测试+变异；`docs/r116-probe-findings.md` §3.4 判据二结论「可行」 |
| P3 | `gameplay.go` 白名单注释改为完整复述两条件，白名单本身不变；§4.4/§5 回填，判据三结论「`==0`」 |
| P4 | 账本记一笔即可，无需代码改动 |
| 全量 | `go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`、`node --test backend/web/*.test.cjs desktop/*.test.cjs`，与 R152 基线一致或更好（既有失败不得增加） |
