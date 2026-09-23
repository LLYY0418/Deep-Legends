# R116-探测 结论文档：海克斯选择端点穷举 + 海斗局内数据形状

> **本文档由代码侧准备完成，三项判据的真机观测值待用户执行后回填。**
> 编写环境是 macOS，没有 Windows、没有 League 客户端、无法进行真实对局，因此本文档里
> **没有任何一条真机观测值**；所有 `待填` 单元格必须由真机导出的诊断日志逐字抄写。
> 项目红线：证据不足明确降级，绝不用推断值代替真实值——包括「看起来应该是 0」这种推断。

- 工单原文：`docs/history/worklists/R116-探测-海克斯选择端点与局内数据形状探测-工单.md`
- 执行账本（做了什么、验证到哪一步）：`docs/r116probe-execution-ledger.md`
- 背景依据：`docs/r116-proposal-feasibility-review.md` 第 3 节（P1 逐项证伪）与 3.2 节（这条探测）
- 探测代码：`backend/augment_contract_probe.go`、`backend/augment_contract_probe_test.go`、`backend/gameplay.go` 的 `aramMode && !arenaMode` 分支

---

## 0. 现状一览

| # | 判据 | 读哪个诊断事件的哪个字段 | 状态 | 回填位置 |
|---|---|---|---|---|
| 一 | LCU 客户端契约里是否存在海克斯三选一候选端点（决定 P1-1 去留） | `augment_contract_probe.count` / `.operations[].path`；`augment_contract_probe_summary.negative_conclusive_all` | **待真机验证** | §2.4 |
| 二 | 海斗 `/liveclientdata/playerlist` 是否带装备字段（决定 P1-4 去留） | `live_client_playerlist_shape.element_keys`（+ `live_client_allgamedata_shape.arrays` 看 items 元素结构） | **待真机验证** | §3.4 |
| 三 | 海斗选人阶段能否看到对方阵容（决定 P1-2/P1-3 覆盖率） | `lcu_champ_select_session_shape.their_team_length` / `.their_team_nonzero_counts` | **待真机验证** | §4.4 |
| 交叉 | 海斗 allgamedata 顶层键（不阻塞判定，只交叉核实） | `live_client_allgamedata_shape.top_level_keys` | **待真机验证**（依赖本次新增分支） | §5.3 |

代码侧已完成：探测函数 + 单测（含工单要求的两条对抗变异）全部就位，`go build` / `go vet` / `go test` 全绿，详见账本。
真机侧未完成：P0 第 4 条（打开客户端触发探测）、P1 第 2~3 条（打一局海斗并导出日志）。

---

## 1. 探测前的先验证据（纯本地可验证，2026-09-20 已核实）

这一节是**不需要启动 League 客户端**就能做完的部分，已全部做掉，用来给三项判据定基线、
并提前排除两类会污染读数的东西（已知无关命中、以及「404 被误读成确证」）。

### 1.1 `/lol-cherry*`、`/lol-mayhem*`、`/lol-augment*` 三个命名空间在仓库里确实一次都没出现

核实方式：对整仓检索 `lol-cherry|lol-mayhem|lol-augment`（排除 `.git`）。

命中 **4 处，全部在两份 R116 文档里**，代码 **0 处**：

| 文件 | 行 | 性质 |
|---|---|---|
| `docs/history/worklists/R116-探测-...-工单.md` | 19 / 36 / 41 | 工单自述与判据 |
| `docs/r116-proposal-feasibility-review.md` | 94 | 评审的证伪表格 |

→ 与工单第 19 行的自述一致：**我们从未请求过这三个命名空间**。「仓库里没有」不代表「客户端没有」，
这正是判据一要用 openapi 契约一次性回答的事。

### 1.2 但客户端确实暴露一个海克斯**静态目录**，它一定会被谓词命中，读数时必须先排除

| 位置 | 路径 | 用途 |
|---|---|---|
| `backend/gameplay.go` `loadGameplayAugmentsFromClient`（改动后约 8154 行） | `GET /lol-game-data/assets/v1/cherry-augments.json` | 海克斯全量静态目录（ID/名称/图标/稀有度） |
| `backend/perk_catalog_async.go:42` | 同上（带磁盘缓存，3 秒超时） | 符文页的海克斯列表 |
| `backend/champions_structured.go:1790` | CommunityDragon 镜像 `/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json` | 客户端目录不可用时的回退 |

这条路径**同时含 `cherry` 与 `augment`**，`augmentProbePath = (?i)cherry|augment|mayhem` 必然命中它。
它是**全量静态目录**，不是按对局/按玩家的端点，**不含任何「这一手系统给的 3 个候选」信息**。

→ **判据一读数规则**：`/lol-game-data/assets/v1/cherry-augments.json`（及其 `perks.json` 同类）属于
**已知无关命中**，回填时要单列并从「是否存在选择端点」的判断中排除。真正的阳性证据必须形如
「按对局/按玩家、能返回候选列表或已选海克斯」的端点。

### 1.3 R82 真机观测：本机 openapi.json 是 **404** —— 「count == 0」本身不能当确证

来源：`docs/history/DIAGNOSIS-R82-CURRENT-GAME-MATRIX-RESULT.md` 第 4 节
（原始数据 `lol-loot-diagnostics-0912-1158.jsonl`，2026-09-12 真机）：

| 契约来源 | 状态 | 当时的观测 |
|---|---|---|
| `/swagger/v3/openapi.json` | **404** | 「客户端没有这个文档」 |
| `/swagger/v2/swagger.json` | **404** | 同上 |
| `/help?format=Full` | **200** | `body_bytes=3,021,918`、`truncated=false`，但 `parsed=false`、`error_kind=unrecognized-contract-format`、`paths_scanned=0` |

工单把「`count == 0` → 确证客户端没有海克斯端点」写成判据，**前提是 openapi.json 读得到**。
在这台机器上它 404 过，所以 404 场景下的 `count == 0` 是**零信息**，把它写成确证就是伪造结论。
为此探测代码做了两件事（全部在新文件内，未改任何既有逻辑）：

1. **每条事件自带判据护栏字段**：`status` / `result` / `contract_read` / `negative_conclusive`。
   `contract_read` 只有在「HTTP 200 且真的解析到契约内容（openapi 的 `paths` 非空 / help 扫到路径 token）」时才为 true；
   **只有 `contract_read=true` 时 `count == 0` 才构成否定证据**。
2. **补第三个来源 `/help?format=Full` 的格式无关兜底扫描**：正则收集全部 `/段/段…` 路径 token → 去重 → 谓词过滤，
   同时记录根对象形状（`json_root` / `root_keys` / `root_shape` / `root_key_count`），
   落实 R82 第 4.2 节第 1 条「先把形状打出来，别再猜」与第 2 条「加一条与格式无关的兜底」。
   汇总事件 `augment_contract_probe_summary.negative_conclusive_all` **只在所有可读来源都零命中时才为 true**。

→ **判据一的阈值因此修正为**：先看 `contract_read_any`，为 false 时本轮探测**不出结论**（要换来源或换客户端版本重跑）；
为 true 时才用 `count` 判定。这一条修正已写进代码与单测（`TestR116AugmentContractProbeUnreadableContractIsNotConclusive`）。

### 1.4 三个局内形状事件在仓库存档里**没有任何真实观测**

检索 `docs/` 与 `backend/testdata/`：`live_client_playerlist_shape`、`live_client_allgamedata_shape`、
`lcu_champ_select_session_shape` 只出现在**源码快照**（`docs/r100-validation/before/*.txt`、
`docs/r96-validation/before-arena_truth_diagnostics.go.txt`）与工单/评审文本里。
仓库内唯一的 `.jsonl` 真实日志存档是 `docs/r89-kr-player/diagnostic-samples.jsonl` 与 `catalog-failure.jsonl`，
与这三个事件无关。

→ 与工单第 49-51 行「海斗（KIWI）下这个事件从未被观测过」「没有一条真实海斗/大乱斗对局的观测」一致。
判据二、三、交叉项都是**从零开始的第一次观测**，没有可以先抄的历史值。

### 1.5 斗魂（CHERRY）下的 19 键基线 —— 判据二的对照物

`docs/history/worklists/WORKLIST-R90-ARENA-LIVE-GROUPING-REFRESH.md:34`：三份日志共 **63 条**
`live_client_playerlist_shape`，凡 HTTP 200 的 `element_keys` **恒为同一组 19 个键**：

```
championName, isBot, isDead, items, level, position, rawChampionName, rawSkinName,
respawnTimer, riotId, riotIdGameName, riotIdTagLine, runes, scores, skinID, skinName,
summonerName, summonerSpells, team
```

`docs/history/worklists/WORKLIST-R95-ARENA-TRUTH-REFRESH-WIPE-FLICKER-PRO.md:85-93` 有一条完整原始记录：
`http_status:200, player_count:18, subteam_field_values:null, team_values:{"ORDER":18},
position_values:{"OTHER":18}, grouped:false, result:"ungrouped", position_matched_count:17,
position_match_source_counts:{"riotId":17,"summonerName":0}`。

→ 判据二要回答的就是：**海斗（10 人、无小队）下这 19 键是否同样包含 `items`**，以及是否出现 `<unknown-key>`。
注意 `element_keys` 只记录玩家对象的**顶层键**（`gameplay.go:4587` `diagnosticKeyUnion`），
**不记录 `items[]` 的元素结构**；要判「结构是否与斗魂一致（`itemID`/`slot`/`count`）」必须再看
`live_client_allgamedata_shape.arrays` 里 `$.allPlayers[].items` 的 `element_keys`——
这正是本次新增 `aramMode` 分支才会产出的事件（见 §5）。**两个事件配合才能完整回答判据二。**

### 1.6 诊断存档里出现的 cherry/augment 字样都来自外部静态资源，不是 LCU 端点

| 位置 | 内容 | 性质 |
|---|---|---|
| `backend/testdata/diagnostics-2024/README.md:5-7` | `hexdata.com.cn/augment-rarity`、`raw.communitydragon.org/.../ux/kiwi/augments/icons/bonk_large.png`、`.../ux/cherry/augments/icons/drop_bear.png` | 外部静态资源 |
| `backend/testdata/diagnostics-2135/README.md:4` | `.../ux/kiwi/augments/icons/criticalmissile_large.png` | 外部静态资源 |
| `docs/diagnostics-0911-2055/2153/2245/2326.md` | cherry / mayhem / augment 命中数 **0** | 无关 |

→ 存档里没有一条 LCU 侧的海克斯端点线索，先验证据到此为止；剩下的只能靠真机探测。

---

## 2. 判据一：LCU 客户端是否存在海克斯选择端点（工单 P0）

### 2.1 看哪个事件的哪个字段

事件 `augment_contract_probe`（每个来源一条，共 3 条）+ `augment_contract_probe_summary`（一条汇总）+
`augment_contract_probe_start`（一条开始标记，含 `path_predicate` 与 `contract_limit_bytes`）。

| 字段 | 含义 |
|---|---|
| `contract_kind` | `openapi-v3` / `openapi-v2` / `help-full` |
| `source_path` | `/swagger/v3/openapi.json`、`/swagger/v2/swagger.json`、`/help?format=Full` |
| `status` / `result` | HTTP 状态码；`ok` / `unavailable` / `too-large` |
| `body_bytes` | 契约文档实际字节数（16 MiB 上限） |
| **`count`** | **本来源命中谓词的条目数**：openapi = 命中的 operation 数（get/put/patch/post/delete/head/options 全算）；help = 命中的去重路径数 |
| `scanned_paths` / `matched_paths` | openapi：`paths` 总数 / 命中谓词的路径数（**全量扫描，不限定前缀**） |
| `paths_scanned` / `unique_paths` | help：扫到的路径 token 数 / 去重后数量 |
| `operations[]` | 与 `objective_badge_contracts` **同构**：`path` / `method` / `parameters`（`name`+`in`+`required`+`schema`）/ `body` / `response_codes` |
| `matched_paths[]`（help） | 命中的路径字面量列表（最多 500 条，超出时 `paths_truncated=true`，但 `count` 仍是全量） |
| `operations_truncated` | 明细超过 200 条被截断（`count` 不受影响） |
| `json_root` / `root_keys` / `root_shape` | help 来源的根对象形状（R82 4.2-1 要求） |
| **`contract_read`** | **是否真的读到契约内容**（判据护栏） |
| **`negative_conclusive`** | `contract_read && count == 0`：本来源是否构成否定证据 |
| `write_executed` | 恒为 `false`（只读探测） |
| 汇总：`contract_read_any` / `negative_conclusive_all` / `sources[]` | 是否有任一来源可读 / 所有可读来源是否都零命中 / 每来源一行的摘要 |

### 2.2 判定阈值

```
第一步：augment_contract_probe_summary.contract_read_any
  false → 本轮探测不出任何结论（三个来源都没读到）。
          动作：抄下每条事件的 status/result/body_bytes，按 §1.3 换来源重跑，不许写"确证无端点"。
  true  → 进第二步。

第二步：所有 contract_read=true 的来源的 count
  全部 == 0（即 negative_conclusive_all == true）
        → 确证：客户端自陈契约里没有任何 cherry/augment/mayhem 端点（§1.2 的静态目录除外，
          若它也 0 命中，说明契约文档根本不列 game-data 资源路径，这一点要一并记下）。
  任一 > 0 → 逐条读 operations[].path / matched_paths[]，按 §2.3 分流。
```

### 2.3 三种可能结论 → 后续动作（引用工单原文判据）

| 观测 | 工单原文判据（P0 验证判据第 1 条） | 后续动作 |
|---|---|---|
| **A. `count == 0` 且 `contract_read == true`** | 「**确证**客户端 openapi 契约里没有任何海克斯/斗魂相关端点。这是比"猜六轮"更权威的否定证据（openapi.json 是客户端自陈的完整契约，不依赖某次网络包被恰好捕获）」 | P1-1「海克斯三选一实时排序」判定为**不可行**，从 R116-C/R116-D 移除；把本表与导出文件路径写进工单结束标志，然后按 §8 删除探测代码 |
| **B. `count > 0`，且命中里存在按对局/按玩家的候选或已选端点** | 「逐条检查 `path`：是否存在类似 `/lol-cherry/*` 或包含 "augment"/"pick" 的 GET 端点。如果有，记录到 `docs/r116-probe-findings.md` 并**新开工单**评估其可用性（不在本工单范围内实现）」 | 把每条 `path`/`method`/`parameters`/`body` 抄进 §2.4，**新开一张工单**评估可用性；本工单不实现任何功能（Anti-scope 第 1 条） |
| **C. `count > 0`，但命中全是 §1.2 那类静态目录** | 工单未单列这种情况（先验证据 §1.2 补的） | 视同 A 处理：静态目录不解决「这一手的 3 个候选」，P1-1 仍判不可行；但在结论里写明「命中的是静态目录，不是选择端点」，避免后人误读 |
| **D. 三个来源都读不到（`contract_read_any == false`）** | 工单未覆盖（先验证据 §1.3 补的） | **不出结论**。记录 status/result，改约一次能读到契约的客户端版本或换来源重跑；P1-1 保持「未判定」，不许默认删除也不许默认实现 |

### 2.4 回填表（待真机观测，禁止填推断值）

执行时间：`待填`　客户端版本/区服：`待填`　是否在对局中：`待填`（工单要求：登录即可，不需要在对局中）
`trace_id`：`待填`　导出文件：`待填`（建议路径见 §7）

| `contract_kind` | `source_path` | `status` | `result` | `body_bytes` | `scanned_paths` / `paths_scanned` | `count` | `contract_read` | `negative_conclusive` |
|---|---|---|---|---|---|---|---|---|
| openapi-v3 | `/swagger/v3/openapi.json` | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |
| openapi-v2 | `/swagger/v2/swagger.json` | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |
| help-full | `/help?format=Full` | 待填 | 待填 | 待填 | 待填（另记 `unique_paths`=待填） | 待填 | 待填 | 待填 |

汇总：`contract_read_any` = `待填`　`negative_conclusive_all` = `待填`

命中的路径逐条列出（排除 §1.2 已知无关项后还剩几条）：

| # | `path` | `method` | `parameters`（name/in/required） | `body` 关键字段 | 是否「按对局/按玩家的候选或已选端点」 | 是否已知无关命中 |
|---|---|---|---|---|---|---|
| 1 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |
| 2 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |

help 来源的根对象形状（若 openapi 全 404，这一栏是唯一能判读的东西）：
`json_root` = `待填`　`root_key_count` = `待填`　`root_keys` = `待填`　`root_shape` = `待填`

**判据一结论（A/B/C/D 之一）**：`待填`　→ 对 P1-1 的处置：`待填`

---

## 3. 判据二：海斗 playerlist 是否带装备字段（工单 P1 验证判据第 1 条 → P1-4）

### 3.1 看哪个事件的哪个字段

`live_client_playerlist_shape`（既有诊断，`gameplay.go` `recordLiveClientPlayerListShape`，
去重键是 `game:<gameID>:<result>`，**无条件对所有模式记录，本次未改代码**）：

| 字段 | 含义 |
|---|---|
| `phase` / `attempt` | 触发阶段与第几次探测 |
| `http_status` / `player_count` | 2999 接口状态码与返回人数（海斗应为 10） |
| **`element_keys`** | 玩家对象的顶层键集合（斗魂基线是 §1.5 的 19 键） |
| `team_values` / `subteam_field_values` / `position_values` | 阵营、小队、位置分布 |
| `grouped` / `group_field` / `result` | 分组结论（海斗预期 `ungrouped`） |
| `position_matched_count` / `position_match_source_counts` | 位置匹配来源 |

`items[]` 的**元素结构**要看 `live_client_allgamedata_shape.arrays`（见 §5）。

### 3.2 判定阈值

```
element_keys 含 "items"？
  否 → P1-4 不可行
  是 → 再看 allgamedata 的 $.allPlayers[].items 的 element_keys 是否为 itemID/slot/count 那一套
        一致 → P1-4 可行
        不同 → P1-4 不可行（结构不同即视为不可用）
element_keys 出现 "<unknown-key>"？（arenaShapeKey 的兜底分支）
  是 → 海斗有斗魂没有的字段，必须人工判读该字段是否与本方案相关（可能是 augment 相关，属重大发现）
```

### 3.3 三种可能结论 → 后续动作（工单原文）

| 观测 | 工单原文判据 | 后续动作 |
|---|---|---|
| 含 `items` 且结构与斗魂一致 | 「若含 `items` 且元素结构与斗魂一致（`itemID`/`slot`/`count` 等）→ P1-4「下一步出装」判定为**可行**，转入 R116-D 正式实现」 | P1-4 保留在 R116-D；实现是 `parseLiveClientPlayerList` 多读一个 `items`（评审 3.1 表） |
| 不含 `items` 或结构不同 | 「若不含 `items` 或结构不同 → P1-4 判定为**不可行**，从 R116-D 移除，记录到 `docs/r116-probe-findings.md`」 | 从 R116-D 移除 P1-4，本表即为记录 |
| 出现 `<unknown-key>` | 「说明海斗有斗魂没有的字段，需要人工判读该字段是否与本方案相关」 | 把 `element_keys` 全量抄进 §3.4，逐个人工判读；若疑似 augment 相关，升级为独立调研项（同 §5 判据 3） |

### 3.4 回填表（待真机观测）

对局信息：模式（KIWI / ARAM_MAYHEM / ARAM_MAYHEM_CLASSIC）=`待填`　`game_id`=`待填`　日期时间=`待填`

| `phase` | `http_status` | `player_count` | `result` | `element_keys`（逐项抄，不要省略） | 含 `items`？ | 含 `<unknown-key>`？ |
|---|---|---|---|---|---|---|
| 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |
| 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |

与斗魂 19 键基线（§1.5）的差异：多出的键 = `待填`　缺失的键 = `待填`

`items[]` 元素结构（取自 §5 的 `live_client_allgamedata_shape.arrays`）：路径 = `待填`　`element_keys` = `待填`

**判据二结论**：`待填`（可行 / 不可行 / 需人工判读）　→ 对 P1-4 的处置：`待填`

---

## 4. 判据三：海斗选人阶段能否看到对方阵容（工单 P1 验证判据第 2 条 → P1-2/P1-3）

### 4.1 看哪个事件的哪个字段

`lcu_champ_select_session_shape`（既有诊断，`gameplay.go` `recordLCUChampSelectSessionShape` +
`lcuChampSelectSessionShapePayload`，**无条件对所有模式记录，本次未改代码**；
去重键是 `PHASE:GAMEMODE:queueID`，所以**每个阶段+模式+队列只记一条**，要在选人阶段就打开面板刷新）：

| 字段 | 含义 |
|---|---|
| `phase` / `game_mode` / `queue_id` | 必须确认这条是海斗的 `ChampSelect`（`game_mode` 为 `KIWI`/`ARAM_MAYHEM`/`ARAM_MAYHEM_CLASSIC`） |
| **`their_team_length`** | 对方阵容长度（**判据主体**） |
| **`their_team_nonzero_counts`** | 对方数组里各字段的非零值计数（有长度还要看是否真有 championId） |
| `their_team_keys` | 对方元素的键集合 |
| `my_team_length` / `my_team_nonzero_counts` / `my_team_unhidden_cell_ids` | 我方对照 |
| `top_level_keys` | 会话顶层键 |
| `bench_champions_length` / `champion_progress_bucket` | 替补与选人进度 |

### 4.2 判定阈值

```
their_team_length > 0 且 their_team_nonzero_counts 里 championId 非零 > 0 → 选人阶段能看到对方阵容
their_team_length == 0（或非零计数全 0）                                  → 选人阶段看不到
```
注意：海斗是 5v5，若 `their_team_length` 为 5 属正常；若为 0，则与评审第 3 节的保守判定一致。

### 4.3 两种可能结论 → 后续动作（工单原文）

| 观测 | 工单原文判据 | 后续动作 |
|---|---|---|
| `their_team_length > 0` | 「`> 0` → P1-2/P1-3 可以扩展到 ChampSelect 阶段」 | R116-D 的 P1-2（阵容协同）/P1-3（克制提示）可在选人阶段出现；仍受覆盖率约束（评审 3.1：协同 7/172、克制 34.6%），形态按「有就显示、没有就整块隐藏」 |
| `their_team_length == 0` | 「`== 0` → P1-2/P1-3 只在 InProgress 阶段出现（与本次评审的保守判定一致，**不算回退**）」 | R116-D 的 P1-2/P1-3 明确只在 InProgress 出；选人阶段不做克制/协同展示 |

### 4.4 回填表（待真机观测）

| `phase` | `game_mode` | `queue_id` | `their_team_length` | `their_team_nonzero_counts`（抄 championId 一项即可，其余可略） | `my_team_length` | `top_level_keys` |
|---|---|---|---|---|---|---|
| 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |
| 待填 | 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |

**判据三结论**：`待填`（>0 / ==0）　→ 对 P1-2/P1-3 的处置：`待填`

---

## 5. 交叉核实：海斗 `live_client_allgamedata_shape`（工单 P1 验证判据第 3 条）

### 5.1 为什么需要本次代码改动

`liveClientAllGameDataShape` / `sampleArenaAllGameData`（`backend/arena_truth_diagnostics.go`）本身与模式无关
（只用 `gameID` 去重，`arenaShapeKey` 白名单不含模式限定），但唯一触发点此前的条件是
`arenaMode && (phase == "GameStart" || "InProgress" || "Reconnect")`，**海斗（`aramMode`）不满足，从未被调用**。
本次在 `backend/gameplay.go` 新增一个独立分支（不改动既有 arena 分支任何一行）：

```go
if aramMode && !arenaMode && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect") {
    go a.sampleArenaAllGameData(ctx, response.GameID)
}
```

### 5.2 看哪个字段

| 字段 | 含义 |
|---|---|
| `game_id` / `http_status` / `success` | 必须确认这条属于本次海斗对局且 `success=true` |
| **`top_level_keys`** | allgamedata 顶层键（工单判据主体） |
| `arrays` | 每个数组路径的 `length` / `element_keys` / `element_shapes` / `field_shapes`；判据二的 `items` 结构看 `$.allPlayers[].items` |

### 5.3 判定与后续动作（工单原文）

工单原文：「主要用于交叉核实，**不阻塞任何功能判定**；若发现明显与海克斯相关的字段（如 `augment`/`cherry` 字样），
记录并升级为独立调研项。」

| 观测 | 后续动作 |
|---|---|
| `top_level_keys` 与斗魂一致、无 augment/cherry 字样 | 仅作交叉核实归档，不改任何判定 |
| 出现 `augment` / `cherry` / `<unknown-key>` 字样 | **升级为独立调研项**（这会是判据一之外的第二条海克斯线索），单开工单，不在本工单实现 |
| 事件根本没出现 | 说明分支未生效或对局未进 InProgress：先核对构建是否包含 §5.1 那行，再重打一局 |

回填表：

| `game_id` | `http_status` | `success` | `top_level_keys`（逐项抄） | `$.allPlayers[].items` 的 `element_keys` | 是否出现 augment/cherry/`<unknown-key>` |
|---|---|---|---|---|---|
| 待填 | 待填 | 待填 | 待填 | 待填 | 待填 |

---

## 6. 用户操作步骤清单（照做即可）

### 6.A 判据一：LCU 端点穷举（**不需要打包应用，不需要进对局**，只要客户端已登录）

> 触发入口说明：工单 P0 第 3 条建议「诊断面板加临时按钮」或「复用导出诊断日志入口」。
> 这两种都要改 `backend/main.go`（注册路由）与 `backend/web/**`（加按钮），本次执行的允许改动范围不含这两个文件，
> 因此改用**等价的仅本机可触发入口**：环境变量门控的 Go 测试（与仓库既有的
> `DEEP_LEGENDS_1945_LIVE`、`OPGG_CURRENT_GAME_PROBE`、`R107_LIVE_IMAGES` 同一套惯例）。
> 它只在本机、只在你显式设置环境变量时才跑，默认 `Skip`；只发 GET，不写客户端。

1. 启动并登录 League 客户端（任意模式，不需要在海斗、不需要在对局中）。
2. 在仓库根目录（Windows，PowerShell 或 Git Bash 均可）执行：
   ```bash
   # Git Bash / WSL
   R116_AUGMENT_PROBE=1 \
   R116_AUGMENT_PROBE_OUTPUT=docs/r116-validation/augment-contract-probe.jsonl \
     go test ./backend -run TestR116AugmentContractProbeLiveClient -v -timeout 5m
   ```
   ```powershell
   # PowerShell
   $env:R116_AUGMENT_PROBE="1"
   $env:R116_AUGMENT_PROBE_OUTPUT="docs/r116-validation/augment-contract-probe.jsonl"
   go test ./backend -run TestR116AugmentContractProbeLiveClient -v -timeout 5m
   ```
3. 若报「客户端未连接」：先确认客户端已登录；仍失败时用 lockfile 兜底
   （lockfile 一般在 `<英雄联盟安装目录>\LeagueClient\lockfile`）：
   ```bash
   R116_AUGMENT_PROBE=1 R116_AUGMENT_PROBE_LOCKFILE='D:\Games\英雄联盟\LeagueClient\lockfile' \
   R116_AUGMENT_PROBE_OUTPUT=docs/r116-validation/augment-contract-probe.jsonl \
     go test ./backend -run TestR116AugmentContractProbeLiveClient -v -timeout 5m
   ```
4. 测试会把每条事件 JSON 打到终端（`-v`），并把完整诊断日志写到 `R116_AUGMENT_PROBE_OUTPUT`。
5. 搜关键字并抄表：在导出文件里搜 `augment_contract_probe`，按 §2.4 逐格填写；
   重点先看 `augment_contract_probe_summary` 的 `contract_read_any` 与 `negative_conclusive_all`。
6. 判读时**先排除 §1.2 的已知无关命中**，再按 §2.3 选 A/B/C/D。

### 6.B 判据二 / 三 / 交叉项：打一局海斗

1. **重新构建并启动应用**（必须包含 §5.1 那行 `aramMode && !arenaMode` 分支；用你平时的打包方式，
   版本号本次不变，仍是 0.12.7）。
2. 客户端里开一局**海斗**（KIWI / ARAM_MAYHEM 任一子模式），完整走完
   **ChampSelect → GameStart/InProgress → 正常结束**（工单 P1 第 2 条）。
   - 选人阶段：让应用停在实时页并**至少刷新一次**，这样 `lcu_champ_select_session_shape` 的
     `ChampSelect:KIWI:<queueID>` 去重键才会被记下（每个 阶段+模式+队列 只记一条）。
   - 进游戏后：停在实时页，等 2999 接口探测成功（`live_client_playerlist_shape` 需要 `http_status:200`）。
3. 对局结束后导出诊断日志：**设置 → 诊断与日志 → 「导出诊断日志」**
   （`backend/web/index.html:320` 的 `#export-diagnostics`，走 `GET /api/diagnostics/log`；
   导出内容包含轮转的 `diagnostics.1..4.jsonl`，比手工拷单个文件全）。
   导出目录在 **设置 → 导出位置**（`index.html:298`）里设置，文件名自带 `MMDD-HHmm`。
   - 兜底：日志原件在 `%LocalAppData%\LOLLootAssistant\logs\diagnostics*.jsonl`
     （若设过 `LOL_LOOT_DATA_DIR` 则以它为准）。
4. 在导出文件里搜这三个关键字，按对应表格抄写：
   - `live_client_playerlist_shape` → §3.4（先看 `element_keys` 有没有 `items`、有没有 `<unknown-key>`）
   - `lcu_champ_select_session_shape` → §4.4（先确认 `phase=ChampSelect` 且 `game_mode` 是海斗，再看 `their_team_length`）
   - `live_client_allgamedata_shape` → §5.3（先确认 `game_id` 是本局、`success=true`，再抄 `top_level_keys`）
5. 把导出文件归档到 §7 的路径，并在各回填表里写上文件名。

---

## 7. 真机诊断日志导出文件（待办：路径已定，文件待产出）

沿用仓库既有惯例（`docs/r107-validation/`、`docs/r115-validation/`、`docs/r89-kr-player/*.jsonl`）：

| 用途 | 建议存档路径 | 状态 |
|---|---|---|
| 判据一：端点穷举导出 | `docs/r116-validation/augment-contract-probe.jsonl` | **待产出**（§6.A 第 2 步的 `R116_AUGMENT_PROBE_OUTPUT` 直接写这里） |
| 判据二/三/交叉：海斗对局导出 | `docs/r116-validation/mayhem-live-shapes.jsonl` | **待产出**（§6.B 第 3 步导出的文件改名后放这里） |
| 终端输出与命令记录 | `docs/r116-validation/probe-run.log` | **待产出**（可选，便于复核命令与耗时） |
| 目录说明（来源、客户端版本、时间、脱敏声明） | `docs/r116-validation/README.md` | **待产出**（参考 `docs/r107-validation/` 的写法） |

本次执行**没有创建** `docs/r116-validation/`（不在允许改动范围内），路径仅作为约定记录在此。
归档时请在 README 里写明：客户端版本/区服、对局模式与 `game_id`、导出时间、
以及「日志已脱敏，仅含接口形状与公开编号，不含账号令牌」这一条与实际写操作一致的声明。

---

## 8. 探测代码的删除清单与时机（工单 P0 第 3 条、P1 第 4 条）

| 对象 | 位置 | 删除时机 |
|---|---|---|
| 探测函数与谓词 | `backend/augment_contract_probe.go`（整个文件） | 判据一结论回填进 §2.4 后**立即整体删除**，不进正式版本 |
| 探测单测 + 真机临时入口 | `backend/augment_contract_probe_test.go`（整个文件，含 `TestR116AugmentContractProbeLiveClient`） | 同上，与探测文件一起删 |
| 海斗触发分支 | `backend/gameplay.go` 的 `if aramMode && !arenaMode && (...)`（3 行注释 + 3 行代码） | 按工单 P1 第 4 条**评估**：若 P1-4/P1-2/P1-3 判定要做 → 保留作为正式埋点（回退成本一行）；若判定不做 → 删除 |
| 本文档 | `docs/r116-probe-findings.md` | **不删除**，作为结论存档 |
| 执行账本 | `docs/r116probe-execution-ledger.md` | **不删除** |

删除后必须重跑：`go build -o <out> ./backend`、`go vet ./backend`、`go test ./backend -run "Objective|Augment|Arena"`、
全量 `go test ./backend/...`，确认删干净且无回归（`augment_contract_probe*` 两个文件删除后，
`-run` 里的 `Augment` 仍会命中既有的海克斯静态目录相关测试，属正常）。
