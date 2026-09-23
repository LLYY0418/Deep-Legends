# R116-探测 执行账本

- 工单：`docs/history/worklists/R116-探测-海克斯选择端点与局内数据形状探测-工单.md`
- 结论文档：`docs/r116-probe-findings.md`（三项判据的真机观测值**待用户回填**）
- 执行日期：2026-09-20
- 执行环境：macOS，无 Windows、无 League 客户端、无法进行真实对局
- 基线版本：0.12.7（本工单不改版本号，`desktop/package.json` 未触碰）

---

## 1. 实际做了什么

### 1.1 新建 `backend/augment_contract_probe.go`（429 行，一次性侦察代码）

| 位置 | 内容 |
|---|---|
| 1-26 行 | 文件头醒目注释：一次性侦察代码、只读边界、脱敏边界、**删除时机**（结论落盘后整体删除，不进正式版本） |
| 44 行 | `var augmentProbePath = regexp.MustCompile("(?i)cherry|augment|mayhem")` —— 工单指定的独立谓词 |
| 52 行 | `augmentProbeTextToken` —— `/help?format=Full` 的格式无关路径 token 正则（依据见 §4.2） |
| 55 行 | `augmentProbeSafeToken` —— 可写入日志的 token 形状白名单（响应码、根对象键名） |
| 57-79 行 | 常量：16 MiB 契约上限（照抄 `objective_diagnostics.go:377-380` 的限流写法）、operation 明细上限 200、参数上限 20、help token 上限 20 万、help 路径明细上限 500、单请求 10s / 整轮 45s 超时、路径字面量 512 上限 |
| 81-89 行 | `augmentProbeSources`：`/swagger/v3/openapi.json`（工单主来源）、`/swagger/v2/swagger.json`、`/help?format=Full`（兜底，理由见 §4.2） |
| 91-97 行 | `augmentProbeMatches(path)`：谓词判定，只看 `?` 之前的路径本体，>512 字节直接不匹配 |
| 99-156 行 | `augmentProbeContracts(raw)` + `augmentProbeContractStats`：对 openapi 的 `paths` 做**全量**扫描（不限定前缀），返回 operation 明细与计数（`ScannedPaths`/`MatchedPaths`/`MatchedOperations`/`Retained`/`Truncated`/`RejectedPaths`/`InvalidJSON`） |
| 158-202 行 | `augmentProbeOperation(...)`：单个 operation 的契约形状，字段与 `objective_badge_contracts` **同构**：`path`/`method`/`parameters`(`name`+`in`+`required`+`schema`)/`body`，另加 `response_codes`；schema 复用既有 `objectiveContractSchema`（只出字段名与类型，不出取值/示例/默认值） |
| 204-245 行 | `augmentProbeTextPaths(raw)` + `augmentProbeTextStats`：help 兜底扫描（token 收集 → 去重 → 谓词过滤），`count` 统计全量命中、明细受 500 条上限保护 |
| 247-298 行 | `augmentProbeRootShape(raw)`：契约文档根对象形状（`json_root`/`root_keys`/`root_key_count`/`root_keys_truncated`/`root_shape`），落实 R82 第 4.2 节第 1 条「先打形状，别再猜」 |
| 300-308 行 | `augmentProbeTraceID()`：每轮探测一个随机 trace（crypto/rand，12 字节 hex），用于把 3+2 条事件串起来 |
| **310-416 行** | **`(*app).collectAugmentContractProbe(ctx, client, source)`**：探测执行入口。逐来源 `client.getBytes(path, 16MiB, "application/json")` → 记录 `augment_contract_probe` 事件（`status`/`result`/`body_bytes`/`count`/`scanned_paths`/`matched_paths`/`operations`/`contract_read`/`negative_conclusive`/`write_executed:false`）→ 最后一条 `augment_contract_probe_summary`（`sources[]`/`contract_read_any`/`negative_conclusive_all`）。函数头注释里写明**触发命令**与删除要求 |
| 418-429 行 | `augmentProbeAllZero(summary)`：只有「所有可读来源都零命中」才允许 `negative_conclusive_all=true` |

事件清单（全部只读、全部带 `temporary_reconnaissance_code: true`）：
`augment_contract_probe_start`、`augment_contract_probe`（每来源一条，共 3 条）、`augment_contract_probe_summary`。

**没有修改** `objectiveDiagnosticPath`（Anti-scope 第 2 条），也没有修改 `objective_diagnostics.go` 任何一行；
只复用了它的 `objectiveContractSchema` 与 `objectiveFieldName`。

### 1.2 新建 `backend/augment_contract_probe_test.go`（652 行）

| 位置 | 测试 | 覆盖 |
|---|---|---|
| 36-93 行 | `TestR116AugmentProbePathPredicate` | 谓词匹配 10 例（含大小写混合 `/lol-CHERRY/v1/x`、`/lol-mayhem-foo`、`/lol-augment-bar`、`/lol-game-data/assets/v1/cherry-augments.json`）+ 不匹配 12 例（`/lol-missions/v1/x` 等三个任务徽章前缀必须不匹配、空串、>512 字节）+ query 不驱动谓词 + **`objectiveDiagnosticPath` 未被本次探测改变**（两条回归断言） |
| 95-213 行 | fixture 与辅助：`r116OpenAPIFixture(withCherry)`、`r116ProbeServer`（非 GET 请求直接判失败）、`r116ProbeApp`、`r116ProbeEvents`、`r116ProbeBySource` | — |
| 215-292 行 | `TestR116AugmentContractProbeMutation` | **工单 P0 对抗变异**：fixture 里放 `/lol-cherry-augments/v1/select`（GET+POST）与 `/lol-mayhem/v1/offer` → `count==3`、path 被记录、`parameters.stage`、`body.augmentId/reroll` 均在；删掉它们 → `count==0`、`negative_conclusive==true`、`scanned_paths` 仍是 6（证明全量扫描没被短路）。附带断言：任务徽章命名空间与 `private-token` 都不出现在事件里、`write_executed` 恒 false、无任何非 GET 请求 |
| 294-339 行 | `TestR116AugmentContractProbeUnreadableContractIsNotConclusive` | 复现 R82 真机场景（openapi v3/v2 双 404、`/help?format=Full` 200）：404 来源 `contract_read=false`、`negative_conclusive=false`、**不发布 `count`**；help 兜底命中 1 条 cherry 路径、根对象形状被记录；汇总 `contract_read_any=true`、`negative_conclusive_all=false` |
| 341-359 行 | `TestR116AugmentContractProbeZeroEverywhereIsConclusive` | 可读来源全零命中时 `negative_conclusive_all=true`（「确证」这条判据的正向样例） |
| 361-385 行 | `TestR116AugmentProbeTextScanIsBounded` | help 扫描的硬上限：540 条命中时 `count==540`（全量）而明细只留 500 条且 `paths_truncated=true`；空 body 扫 0 |
| 387-488 行 | `r116LiveFixturePlayers` / `r116LiveFixture` / `r116WaitForSampling` | 假 LCU（gameflow/champ-select/lobby）+ `liveClientPlayerList`、`liveClientAllGameData` 两个钩子换成计数器；等待异步 `go sampleArenaAllGameData` 落地（正向以「日志里出现该 gameID 的 `live_client_allgamedata_shape`」为准，反向在 750ms 有界窗口内断言钩子与去重表都没被写） |
| 490-549 行 | `TestR116AramModeTriggersArenaAllGameDataSampling` | **工单 P1 对抗变异的行为侧**：7 个子测试——ARAM_MAYHEM×InProgress、KIWI×GameStart、ARAM_MAYHEM×Reconnect、ARAM×InProgress 都触发且事件 `game_id`/`success`/`items`/`itemID` 正确；ARAM_MAYHEM×ChampSelect 不触发；CLASSIC 不触发；CHERRY 仍触发且 `ArenaMySquadNotice` 非空（证明走的是**既有 arena 分支**，因为新分支要求 `!arenaMode`） |
| 551-587 行 | `TestR116AramBranchSourceGuardForArenaSampling` | 源码侧护栏：`gameplay.go` 必须含那 6 行的原文；出现 `if !aramMode && !arenaMode` 或 `if aramMode && arenaMode` 即判失败；既有 arena 分支文本必须恰好出现 1 次 |
| 589-652 行 | `TestR116AugmentContractProbeLiveClient` | **工单 P0 第 3 条的「仅本机可触发的临时入口」**：默认 `t.Skip`，只有 `R116_AUGMENT_PROBE=1` 才跑；用生产同一套 `discoverLCUDetailed()`（失败可用 `R116_AUGMENT_PROBE_LOCKFILE` 兜底）连本机客户端 → 跑探测 → 逐条打印事件 JSON → `R116_AUGMENT_PROBE_OUTPUT` 指定时把完整诊断日志导出成文件；若三个来源一个都没读到则**测试失败**并提示「本轮不构成结论」 |

### 1.3 `backend/gameplay.go`：只新增一个分支（改动后 6103-6108 行）

```go
6103	// R116-探测（一次性侦察，工单 P1 第 1 条）：海斗（KIWI/ARAM_MAYHEM）此前不满足上面的
6104	// arenaMode 条件，live_client_allgamedata_shape 在海斗下从未被观测过。这里只把既有诊断
6105	// 埋点扩大到海斗模式，不新增任何用户可见功能；探测结论落盘后按工单 P1 第 4 条评估保留/删除。
6106	if aramMode && !arenaMode && (phase == "GameStart" || phase == "InProgress" || phase == "Reconnect") {
6107		go a.sampleArenaAllGameData(ctx, response.GameID)
6108	}
```

- 位置：紧接既有 `if arenaMode && (phase == ...)` 分支（改动后 6092-6102 行）之后，**既有 arena 分支一行未改**。
- 复用了同一函数里已声明的 `aramMode := isARAMFamilyGameMode(response.GameMode)`（当前 5949 行），未新增任何变量。
- 行号说明：本工单插入点是 HEAD 的 6098-6103；因并行子任务在同一文件的 1341/1376/2794 附近改动，
  当前工作区里漂移到 6103-6108。**相对 HEAD 只有这 6 行新增，无其他改动**（`git diff` 已核对）。

### 1.4 新建两份文档

- `docs/r116-probe-findings.md`：三项判据 + 交叉项的「待真机验证」结构（每项写清看哪个事件的哪个字段、
  判定阈值、各种可能结论分别导向什么后续动作并引用工单原文）、可填写的空表格、用户操作步骤清单、
  建议存档路径、删除清单；第 1 节是**已核实的先验证据**（纯本地）。
- `docs/r116probe-execution-ledger.md`：本文件。

### 1.5 明确没有碰的文件

`backend/hexdata.go`、`backend/champion_cache.go`、`backend/champions.go`、`backend/season_stats.go`、
`backend/web/**`、`desktop/package.json`、`backend/objective_diagnostics.go`、`backend/arena_truth_diagnostics.go`、
`backend/main.go` —— 全部零改动（`git status` 里这些文件的 `M` 标记来自并行子任务，不是本工单）。

---

## 2. 验证结果

### 2.1 已自动验证 PASS

验证载体说明：执行期间并行子任务正在改 `backend/hexdata.go` 与 `backend/season_stats.go`，
工作区**整包无法编译**（见 §5.1），因此本工单的自动验证在一棵隔离树上完成：
`git archive HEAD` + **只有本工单的三处改动**（`gameplay.go` 的 6 行、两个新文件）。
两份新文件与工作区**逐字节相同**（shasum 核对），`gameplay.go` 的插入块与工作区**逐字节相同**（仅行号因并行改动而漂移）。

| 命令 | 结果 |
|---|---|
| `go build -o "$PI_SCRATCH_DIR/dl-iso2" ./backend` | **PASS**（BUILD_OK） |
| `go vet ./backend` | **PASS**（无输出，无新增告警） |
| `gofmt -l backend/augment_contract_probe.go backend/augment_contract_probe_test.go` | **PASS**（无输出） |
| `go test ./backend -run "R116" -count=1 -v` | **PASS**：5 个探测测试 + 7 个子测试全绿，1 个 live 入口 SKIP（4.066s） |
| `go test ./backend -run "Objective" -count=1` | **PASS**（0.427s）——工单 P0 验证判据第 2 条：既有任务徽章诊断测试不受影响 |
| `go test ./backend -run "Objective\|Augment\|Arena" -count=1 -v` | **PASS**（全部 `--- PASS`，仅 live 入口 `--- SKIP`） |
| `go test ./backend/... -count=1`（全量） | **PASS：`ok lol-loot-assistant/backend 205.563s`**（基线 203.331s，+2.2s 来自新增测试） |

工单判据逐条对照：

| 工单条目 | 状态 | 证据 |
|---|---|---|
| P0 实现要求 1（不改 `objectiveDiagnosticPath`） | **PASS** | 该文件零改动；`TestR116AugmentProbePathPredicate` 里两条回归断言 |
| P0 实现要求 2（新文件 + 独立谓词 + 全量扫描 + 同构事件 + `count`） | **PASS** | §1.1；`TestR116AugmentContractProbeMutation` 断言 `count`/`path`/`method`/`parameters`/`body` |
| P0 实现要求 3（仅本机可触发的临时入口 + 醒目注释 + 用完整体删除） | **PASS（形式有偏差，见 §4.1）** | §1.2 最后一行；文件头与函数头注释 |
| P0 实现要求 4（打开客户端触发探测、导出日志） | **待真机验证** | 需 Windows + 已登录客户端，见 findings §6.A |
| P0 验证判据 1（`count` 读数与结论） | **待真机验证** | findings §2.2-§2.4（含 404 场景的判据修正） |
| P0 验证判据 2（编译通过 + 谓词单测 + 不影响既有 Objective 测试） | **PASS** | 上表 build/vet/`-run Objective` |
| P0 对抗变异（fixture 加/删 cherry 路径） | **PASS** | `cherry=true` → `count==3`；`cherry=false` → `count==0` |
| P1 实现要求 1（唯一代码改动：新增海斗分支） | **PASS** | §1.3 |
| P1 实现要求 2-3（打一局海斗、读三个事件） | **待真机验证** | findings §6.B |
| P1 验证判据 1-3 | **待真机验证** | findings §3/§4/§5 |
| P1 实现要求 4（探测后评估分支保留/删除） | **待真机验证后决策** | findings §8 |
| P1 对抗变异（`!aramMode` 时测试必须失败） | **PASS** | §2.2 真实输出 |
| Anti-scope 1（不实现任何产品功能） | **PASS** | 无 UI/接口/产品逻辑改动，只有诊断事件 |
| Anti-scope 2（不改 `objectiveDiagnosticPath`） | **PASS** | 同上 |
| Anti-scope 3（不在本工单决定 P1-2/3/4 实现细节） | **PASS** | findings 只给判据与后续动作，不写实现 |
| 交付物：探测文件 + 单测 | **PASS** | §1.1、§1.2 |
| 交付物：`gameplay.go` 一行分支 | **PASS** | §1.3 |
| 交付物：`docs/r116-probe-findings.md` | **代码侧完成，观测值待回填** | 文档顶部已声明 |
| 交付物：一份真机诊断日志导出文件 | **待真机验证** | 建议路径已写入 findings §7 |

### 2.2 对抗变异（工单 P1）真实执行记录

在隔离树里把新增分支的判断改成 `!aramMode`（其余不变），跑行为测试与源码护栏。
下面是**最终交付版本文件**上的真实输出（`t.Fatalf` 因 `t.Helper()` 归因到调用点 520 行）：

```
MUTATION APPLIED
--- FAIL: TestR116AramModeTriggersArenaAllGameDataSampling (22.26s)
    --- FAIL: .../海斗_ARAM_MAYHEM_InProgress (5.01s)
        augment_contract_probe_test.go:520: sampleArenaAllGameData never produced a shape event for gameID 91601 (calls=0 claimed=false)
    --- FAIL: .../海斗_KIWI_GameStart (5.01s)        ... gameID 91602 (calls=0 claimed=false)
    --- FAIL: .../海斗_ARAM_MAYHEM_Reconnect (5.01s) ... gameID 91603 (calls=0 claimed=false)
    --- FAIL: .../普通大乱斗_ARAM_也扩大覆盖 (5.01s)   ... gameID 91604 (calls=0 claimed=false)
    --- FAIL: .../经典模式不触发 (0.00s)
        augment_contract_probe_test.go:520: gameID 91606 was sampled but must not be (calls=1)
--- FAIL: TestR116AramBranchSourceGuardForArenaSampling (0.00s)
    augment_contract_probe_test.go:559: gameplay.go lost the R116 aramMode sampling branch (or it was mutated)
    augment_contract_probe_test.go:563: mutation survived in gameplay.go: if !aramMode && !arenaMode
FAIL	lol-loot-assistant/backend	23.293s
```

→ 变异被两类断言同时捕获：海斗/大乱斗四个场景「不触发」，经典模式「反而触发」，源码护栏也报错。
变异随后已还原（`grep -n "if aramMode && !arenaMode"` 回到 6101 行），还原后重跑：
`go test ./backend -run "Objective|Augment|Arena" -count=1` → **`ok lol-loot-assistant/backend 9.061s`**；
§2.1 的全量 205.563s 同样是未变异版本的结果。

P0 对抗变异（fixture 加/删 `/lol-cherry-augments/v1/select`）由
`TestR116AugmentContractProbeMutation` 的两个子测试常态覆盖，不需要手工变异源码。

---

## 3. 留下哪些未解决问题

1. **三项判据的真机观测值全部为空**（P0 第 4 条、P1 第 2-3 条）。本环境做不到，已在 findings 里
   写成可照做的步骤清单 + 空表格，等用户在 Windows 真机执行后回填。**工单的「结束标志」第 1 条
   （三项判据均有明确结论）目前未达成**，这是事实，不做任何推断填充。
2. **工单 P0 判据 1 存在一个未覆盖的分支**：它把「`count == 0`」直接等同于「确证没有端点」，
   但 `docs/history/DIAGNOSIS-R82-CURRENT-GAME-MATRIX-RESULT.md` 第 4 节记录了本机
   `/swagger/v3/openapi.json` 与 `/swagger/v2/swagger.json` **双双 404**（2026-09-12 真机）。
   404 时 `count` 也是 0，直接套用判据会产出**假阴性结论**。处理方式见 §4.2。
3. **`/help?format=Full` 的 3 MB 契约至今没人读懂**（R82 记录 `parsed=false`、`paths_scanned=0`，
   当时的 `current_game_endpoint_inventory.go` 已从仓库删除）。本次用格式无关的正则兜底绕开了
   「必须理解 JSON 结构」这个前提，但如果客户端的 `/help` 里路径不是以 `/段/段` 形式出现，
   兜底也会漏；因此事件里额外记录了 `root_keys`/`root_shape`，真机跑完可据此判断是否需要再补一轮。
4. **工作区整包当前编译不过**，原因不在本工单（见 §5.1）。等并行子任务收敛后必须在工作区重跑一次
   `go build -o ... ./backend` + `go vet ./backend` + `go test ./backend/...`，把这轮的输出补进本账本。
5. **`gameplay.go` 是并行子任务与本工单的共同改动文件**：执行期间本工单插入的 6 行曾被并行改动整段覆盖，
   已重新插入并核对（§5.2）。合并/打包前需要再确认这 6 行还在。

---

## 4. 与工单的偏差（都是主动声明，不是遗漏）

### 4.1 触发入口的形式：用「环境变量门控的 Go 测试」代替「诊断面板临时按钮」

工单 P0 第 3 条给了两种建议：诊断导出面板加临时按钮，或复用「导出诊断日志」入口附带调用一次。
这两种都需要改 `backend/main.go`（注册路由，参考 `main.go:559` 的 `POST /api/diagnostics/objectives`）
和/或 `backend/web/**`（加按钮）——**这两个文件都在本工单的禁止触碰清单里，且有并行子任务正在改**。
因此改用等价的「仅本机可触发」入口：`TestR116AugmentContractProbeLiveClient`，
默认 `Skip`，只有显式 `R116_AUGMENT_PROBE=1` 才执行；这与仓库既有惯例一致
（`DEEP_LEGENDS_1945_LIVE`、`OPGG_CURRENT_GAME_PROBE`、`R107_LIVE_IMAGES`、`R112_OPGG_CAPTURE` 等）。
它满足工单的实质要求：仅本机、需人工显式触发、走生产同一套 LCU 传输与诊断写入、只读、可整体删除。

如果后续更希望要 HTTP 入口，只需在 `backend/main.go` 的路由表加一行（本工单未做）：

```go
mux.HandleFunc("POST /api/diagnostics/augment-probe", a.authorized(a.handleAugmentContractProbe))
```

并在 `augment_contract_probe.go` 里补一个 10 行的 `handleAugmentContractProbe`（照抄
`handleObjectiveDiagnostics` 的写法：取 `a.lcu`/`a.connected` → 调 `collectAugmentContractProbe` → `respondJSON`）。
这两个改动都属于「删掉即回退」的临时代码。

### 4.2 探测来源从 1 个扩到 3 个（新增 `/swagger/v2` 与 `/help?format=Full`）

工单只写了 `/swagger/v3/openapi.json`。扩到 3 个的理由是 §3.2 的先验证据：本机 v3/v2 都 404 过，
只探 v3 极可能拿到「404 → count 0」这种**无法区分「没有端点」和「没读到契约」**的结果，
那这一轮探测就白跑了（工单目标是「一次性回答」）。
`/swagger/v2/swagger.json` 沿用既有 `collectObjectiveDiagnostics` 的双版本做法；
`/help?format=Full` 用格式无关的正则扫描 + 根对象形状记录，来自 R82 第 4.2 节的两条建议。
三个来源各自出一条 `augment_contract_probe` 事件（`contract_kind` 区分），
判据字段 `contract_read` / `negative_conclusive` / `negative_conclusive_all` 保证
「只有真的读到契约时 count==0 才算否定证据」。这属于**在新文件内**为实现工单目标所必需的最小扩展，
没有改动任何既有功能，也没有新增产品行为。

### 4.3 `count` 的语义按来源分别定义（写进事件与 findings）

openapi 来源：`count` = 命中谓词的 **operation 数**（一个 path 的 GET/POST 各算一条）；
help 来源：`count` = 命中谓词的**去重路径数**。
两者的 `count` 都不受明细截断上限影响（明细截断另有 `operations_truncated` / `paths_truncated` 标记），
避免「截断导致 count 归零」污染判据。

---

## 5. 执行过程中的环境事实（如实记录）

### 5.1 工作区整包编译失败，原因全部来自并行子任务

执行期间三次尝试在工作区直接构建，均因**他人正在编辑的禁止触碰文件**失败：

```
# 第一次
backend/hexdata.go:1279:58: undefined: hexdataHeroDetail
# 第二次
backend/season_stats.go:351:28: cannot use item.Kills + item.Assists (value of type float64) as int value in argument to ratio
backend/season_stats.go:351:53: cannot use item.Deaths (variable of type float64) as int value in argument to ratio
backend/season_stats.go:385:29: cannot use totalKills + totalAssists (value of type float64) as int value in argument to ratio
backend/season_stats.go:385:54: cannot use totalDeaths (value of type float64) as int value in argument to ratio
# 第三次（本账本定稿时）
backend/main.go:50:14: undefined: embed
backend/season_stats.go:351 / 385: 同上四条
```

`hexdata.go`、`season_stats.go`、`main.go` 都不在本工单的允许改动范围内（前两个明确在禁止清单里），
未做任何处理。因此本工单的自动验证改用 §2.1 描述的隔离树（HEAD + 只有本工单三处改动）。
**并行子任务收敛后，必须在真实工作区补跑一次并记录：**
`go build -o "$PI_SCRATCH_DIR/dl-build" ./backend`、`go vet ./backend`、
`go test ./backend -run "Objective|Augment|Arena"`、`go test ./backend/...`、`cd backend/web && node --test`。

### 5.2 本工单在 `gameplay.go` 的 6 行曾被并行改动覆盖

第一次插入后（当时位于 6098-6103），并行子任务对同一文件的改动把这 6 行整段覆盖掉了
（`grep "aramMode && !arenaMode"` 一度返回空）。已重新插入到当前 6103-6108 行，
并逐字节核对既有 arena 分支（6092-6102）未被改动。**合并前请再 grep 一次确认这 6 行还在。**

---

## 6. 临时代码清单与删除时机

| # | 对象 | 位置 | 性质 | 删除时机 |
|---|---|---|---|---|
| 1 | `augmentProbePath` / `augmentProbeTextToken` / `augmentProbeSafeToken` / 常量组 / `augmentProbeSources` | `backend/augment_contract_probe.go:44-89` | 一次性侦察 | 判据一结论回填后立即删（整文件） |
| 2 | `augmentProbeMatches` / `augmentProbeContracts` / `augmentProbeOperation` / `augmentProbeTextPaths` / `augmentProbeRootShape` / `augmentProbeTraceID` / `augmentProbeAllZero` | 同上 91-298、418-429 | 一次性侦察 | 同上 |
| 3 | `(*app).collectAugmentContractProbe` | 同上 310-416 | 一次性侦察入口 | 同上 |
| 4 | 全部 8 个 `TestR116*` 测试与 5 个 fixture 辅助函数 | `backend/augment_contract_probe_test.go`（整文件） | 一次性侦察的单测与真机入口 | 同上（与探测文件一起删） |
| 5 | 海斗触发分支（3 行注释 + 3 行代码） | `backend/gameplay.go:6103-6108` | 既有诊断埋点的模式扩容 | 按工单 P1 第 4 条**评估**：P1-4/P1-2/P1-3 要做 → 保留为正式埋点；不做 → 删除（回退成本一行） |
| 6 | 新增的 3 个诊断事件名 | `augment_contract_probe_start` / `augment_contract_probe` / `augment_contract_probe_summary` | 随 #1-#3 消失 | 同上 |
| 7 | `docs/r116-probe-findings.md`、`docs/r116probe-execution-ledger.md` | `docs/` | 结论与账本 | **不删除** |
| 8 | 建议的真机导出存档 | `docs/r116-validation/`（本次未创建） | 证据 | **不删除**（待用户产出） |

删除后必须重跑并记录：`go build -o <out> ./backend`、`go vet ./backend`、
`go test ./backend -run "Objective|Augment|Arena"`、`go test ./backend/...`、`cd backend/web && node --test`。

---

## 7. 本工单没做、也不该做的事（对照 Anti-scope）

1. 没有实现任何海克斯选择相关的产品功能（即使探测命中端点，也只记录 + 建议新开工单）。
2. 没有修改 `objectiveDiagnosticPath`，没有修改 `objective_diagnostics.go`。
3. 没有决定 P1-4/P1-2/P1-3 的实现细节，findings 只给「可行/不可行」的判据与后续动作。
4. 没有改版本号（`desktop/package.json` 保持 0.12.7）。
5. 没有创建 `docs/r116-validation/`（超出允许改动范围），只在 findings §7 记录建议路径。
