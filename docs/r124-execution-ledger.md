# R124 执行台账：位置契约探测改为「端点直探 + 响应键名扫描」

**工单：** `docs/WORKLIST-R121-POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE.md`
**基线版本：** 0.12.15　**最终版本：** **0.12.16**（`desktop/package.json` + `package-lock.json` 三处已同步）
**轮次编号说明：** 工单标题自称 `WORKLIST-R121`，但 `docs/r121-execution-ledger.md` 已被「补位标签整改」那轮占用，`r122`（诊断计数护栏）与 `r123`（生涯头像/旗帜迁移）也已被占用，因此**本轮按 R124 记账**。工单文件本身不改名，保持原名以便追溯。

## 0. 三句话结论

1. **接受工单判断并已整改**：删除 `/help?format=Full` 的格式无关文本兜底（真机实测 3 MB 响应里只扫出 1 个路径 token、命中 0，说明该端点输出驼峰式 RPC 名而非字面 REST 路径，「扫斜杠路径」的策略对它天然无效），改为**直接 GET 六个已知端点 + 扫响应 JSON 的键名**，这条路径不依赖任何契约文档，能独立给出可判定结论。
2. **结论判据从「读到没读到」改成三态 `verdict`**：`found` / `not-found` / `inconclusive`。上一轮真机的问题正是 `contract_read_any=true`（/help 返回了 200 和 3 MB）却什么都没查到，日志容易被读成「查了、确实没有」；现在「读到东西」与「能下结论」彻底分开，且 `inconclusive` 会让真机测试**失败**而不是静默通过。
3. **按工单要求保留探测代码**（`docs/r121-position-probe-findings.md` §5 的删除前提是「结论已写入」，而上一轮结论是 inconclusive，删了就再也补不上这个窟窿）；结论文档已重写为按端点逐行回填的表格，并把上一轮真机结果如实归档在 §0。

**生产代码零改动**：本轮只动 `backend/position_contract_probe.go`、`backend/position_contract_probe_test.go`（均是一次性侦察代码，无生产调用方）与两份文档。补位判定的口径、门槛、`autofillLabelGate`、前端渲染全部未动（工单执行纪律第 1 条）。

---

## 1. P1-1　端点直探 + 响应键名扫描

### 1.1 删除的东西

| 删除项 | 原因 |
|---|---|
| `positionProbeTextPath` 正则 | 「扫字面斜杠路径」的假设对当前客户端不成立（工单 §1 根因） |
| `positionProbeTextPaths` 函数 + `positionProbeTextStats` 类型 | 同上，唯一调用方是 `/help` 兜底分支 |
| `positionProbeTextTokenLimit` / `positionProbeTextPathLimit` 常量 | 随函数一并退场 |
| `positionProbeSources` 里的 `{"/help?format=Full", "help-full"}` | 工单 P1-1 明确「放弃对 /help 做格式不可知的文本扫描」 |
| collect 里的 `help-full` 分支与 `contract_read_any` 汇总键 | 该分支已无来源；`contract_read_any` 的语义正是上一轮误导的根源，改由 `verdict` 取代 |

保留 openapi v3/v2 两个契约来源，但**降级为 best-effort 补充**：404 时完全不影响 `verdict`，200 时 `$ref` 展开能给出比响应样本更完整的 schema。工单说的是「不再**依赖**契约清单」，端点直探已独立满足；顺带删掉 openapi 会一并废掉约 250 行已通过测试的 `$ref` 展开能力和它的两个测试，没有必要。

### 1.2 新增的东西

| 新增 | 说明 |
|---|---|
| `positionProbeEndpoints`（6 项，字段 `path`/`note`/`critical`） | 端点清单，`note` 逐个写明仓库内出处（见 §1.3） |
| `positionProbeResponseKeys(raw) positionProbeResponseScan` | 核心纯函数：扫响应 JSON 的**键名**，返回命中清单 + 扫描键数 + 截断标记 |
| `positionProbeWalkResponse` | 递归 walker：对象按键排序遍历，数组只穿透前 N 个元素且不计深度 |
| `positionProbeKeyHit` / `positionProbeResponseScan` | 结果类型，只含 `name`/`type`/`owner` 与计数，**没有任何取值字段** |
| `positionProbeJSONType` | 只回答「值是什么类型」，不读取也不返回取值 |
| `positionProbeGet` | 抽出重复的只读 GET + 错误分类（`LCUHTTPError` → status，`errResponseLimitExceeded` → `too-large`），两个探测循环共用 |
| `positionProbeVerdict` + 三个 verdict 常量 | 三态结论收敛（见 §2） |
| 常量 `positionProbeResponseDepth=2`、`ResponseKeyLimit=4000`、`ResponseArrayLimit=2` | 深度与上限，防止战绩响应把一次性侦察变成分钟级 CPU 占用 |
| `positionProbeTotalTimeout` 45s → 90s | 请求数从 3 增加到 8（6 端点 + 2 契约源） |

### 1.3 端点清单的出处（工单 P1-1 验收判据 3）

逐个在仓库里核对过，全部是**生产代码**里的真实调用点，不是测试桩：

| 端点 | critical | 仓库出处 |
|---|---|---|
| `/lol-lobby/v2/lobby` | 是 | `backend/gameplay.go:7244`（`client.GetJSON`）、`7254`、`7256`；另见 `backend/watch_rules.go` 多处 |
| `/lol-champ-select/v1/session` | 是 | `backend/gameplay.go:7226`（`GetBytesContext`）、`8752`（`RequestJSON`） |
| `/lol-gameflow/v1/session` | 是 | `backend/gameplay.go:7163`（`GetBytesContext`）、`7177` |
| `/lol-gameflow/v1/gameflow-phase` | 否 | `backend/gameplay.go:7138`、`8254`、`8744` |
| `/lol-match-history/v1/products/lol/current-summoner/matches` | 否 | `backend/gameplay.go:3296-3297` |
| `/lol-end-of-game/v1/eog-stats-block` | 否 | **只有命名空间级出处**：`backend/flow_diagnostics.go:162` 的 `{"/lol-end-of-game/", "end-of-game"}` 前缀分类条目。该**具体路径在本仓库无任何调用点**，由工单指定 |


> 如实说明最后一条：我没有为 `eog-stats-block` 编造出处。它在 `note` 字段里被明确标注为「仅命名空间有出处……由工单指定，局外才有效，404 属预期且不构成证据」，因此它 404 时既不算失败、也不参与任何判据。
>
> 上表与代码 `note` 字段里的行号均以 **0.12.16 的 `backend/gameplay.go`** 为基准；该文件改动频繁（R123 那轮就让 `lcuLivePlayer.SelectedPosition` 从 6937 漂到 7009），行号若对不上请以函数名 / 端点字符串为准重新 grep。探测代码是一次性侦察、结论回填后即删，因此不为它做行号自动校验。

### 1.4 三处刻意偏离工单字面口径的地方（都写进了代码注释）

1. **扫描深度用 3 而不是「顶层 + 一层子对象」，且到达上限时如实标记不完整。** 工单原话是「只扫响应 JSON 顶层与一层子对象的键名」，但真实形状装不下：`/lol-gameflow/v1/session` 把队伍放在 `gameData.teamOne[]` 里，`selectedPosition` 已在第三层，按「顶层 + 一层」永远扫不到——而那正是本仓库 `lcuLivePlayer.SelectedPosition`（`backend/gameplay.go:7009`）天天在读的字段；再留一层是给可能存在的同类包装。数组不计入深度（只穿透），所以 `lobby.members[].firstPreference` 仍在第 1 层。**关键（第二轮复核修正）**：到达深度上限时，若被跳过的值下面还有非空结构（`positionProbeHasChildren`），必须置 `Truncated`，否则「零命中」会被当成完整扫描后的否定证据；若被跳过的只是标量则不算截断，否则任何真实响应都会被判不完整、关键端点永远无法下否定结论。`TestR121PositionResponseKeysReachNestedArraysAndRejectBareStrings` 锁「深度够深能扫到」，`TestR121PositionProbeIncompleteScansNeverBecomeNegativeEvidence` 锁「漏扫必须标记」。
2. **引入 `critical` 概念。** 工单要求「只要至少一个端点返回 200 且完成了键名扫描，就能重新定义 negative_conclusive」。我把它收紧为「至少一个 **critical** 端点」：如果 lobby 与 champ-select 全部 404（客户端不在大厅也不在选人阶段，这是很常见的情况），那么即使 match-history 扫了几千个键零命中，也**不能**得出「客户端没有位置偏好字段」——最可能藏这个字段的地方压根没看到。这正是上一轮「读到 3 MB 却什么都没查到」的同型错误，不能在换个形式后重犯。
3. **截断时不下否定结论。** 战绩响应（几十场 × 十人 × 几十个字段）在真机上**必然**超过 4000 键上限，因此 `keys_truncated=true` 时该端点的 `negative_conclusive` 恒为 `false`。不修这条，真机日志里会出现 `keys_truncated: true` 与 `negative_conclusive: true` 并存的自相矛盾组合，回填结论的人会被误导。

另外补了一条工单没提但必要的护栏：**裸字符串响应不算扫描完成**。`/lol-gameflow/v1/gameflow-phase` 返回的是 `"ChampSelect"` 这种裸 JSON 字符串，`json.Unmarshal` 成功、`ValidJSON=true`，但一个键都没有；若只判 `ValidJSON`，它会被当成「扫过了、零命中」的否定证据。因此扫描完成的判据是 `ValidJSON && Scanned > 0`。

### 1.5 新增测试（工单 P1-1 验收判据 1、2）

| 测试 | 覆盖 |
|---|---|
| `TestR121PositionResponseKeysScanSyntheticLobbyBody` | 合成 `/lol-lobby/v2/lobby` 响应：`firstPreference` 命中 2 次、`secondPreference` 1 次，类型均为 `string`、`owner` 均为 `members[]`；9 个不该命中的键（`lobbyId`/`members`/`puuid`/`slot`/`isLeader`/`gameConfig`/`queueId`/`mapId`/`invitationCount`）逐个断言不出现；序列化结果不含任何 `R121_SYNTHETIC_` 取值 |
| `TestR121PositionResponseKeysReachNestedArraysAndRejectBareStrings` | gameflow 三层嵌套（`gameData.teamOne[].selectedPosition/selectedRole`，owner 为 `gameData.teamOne[]`）；12 元素数组只穿透前 10 个且置 `Truncated`；3 元素数组全扫且不置 `Truncated`；裸字符串 → `ValidJSON=true / Scanned=0`；非法 JSON → `ValidJSON=false` |
| `TestR121PositionProbeVerdictSeparatesReadFromConclusive` | 表驱动 **8** 例，覆盖三态全部分支，含「上一轮真机形状」→ inconclusive 且 reason 非空，以及第 8 例「契约只有端点命中、属性零命中且不完整 → 仍 inconclusive」（§6 缺陷 #1 的回归锁） |
| `TestR121PositionProbeIncompleteScansNeverBecomeNegativeEvidence` | 5 例锁住「不完整扫描不得成为否定证据」：深度超限且下面还有结构 → `Truncated`；深度超限但下面是标量 → 不 `Truncated`；owner 路径脱敏（含空格祖先键 → `?`，原始键名不外泄）；契约缺 response schema → `MissingSchemas=1`；`$ref` 指向不存在节点 → `UnresolvedRefs=1`（§6 缺陷 #3/#4/#5 的回归锁） |
| 真机入口 `TestR121PositionContractProbeLiveClient`（改造） | 外层 ctx 90s → 150s；结尾改为统计 `position_endpoint_probe`/`position_contract_probe` 事件数 + switch `verdict`，`inconclusive` → `t.Error`（刻意让「没查到」失败，而不是静默通过） |

**判据 2（对抗变异）的落点**：把 `positionProbeProperty` 换成必然扫不到的假谓词（如 `regexp.MustCompile("zzz_no_such_key")`），第一个测试的「`firstPreference` 必须被扫出」立即 FAIL；把 `positionProbeResponseDepth` 从 3 改成 1，第二个测试的 gameflow 用例 FAIL；把 `positionProbeResponseArrayLimit` 调大，数组穿透断言 FAIL；删掉 walker 深度分支的 `scan.Truncated = true`、删掉 `positionProbeOwnerPath` 的白名单校验、删掉 `stats.MissingSchemas++` / `stats.UnresolvedRefs += walker.badRefs`，第四个测试的对应用例各自 FAIL。

---

## 2. 三态结论口径

```
found        —— 端点响应里命中谓词的键（endpoint_matched_keys > 0），
                或契约 schema 里有属性命中（contract_matched_properties > 0）
not-found    —— 无任何肯定证据，且至少一路否定依据成立：
                · 某个 critical 端点证据完整（200 + 合法 JSON + scanned_keys>0
                  + keys_truncated=false），或
                · 某个契约来源被完整读取（contract_complete=true）且属性零命中
inconclusive —— 既没命中，也没有任何能支撑否定结论的完整扫描
```

`negative_conclusive_all` 字段名保留（便于与上一轮真机日志对照），但取值改为 `verdict == not-found`，口径已收紧。

**范围限定（回填结论时必须一并写明）**：即使 `not-found`，结论也只覆盖「被完整扫到的那几个端点、在深度 ≤3 且每数组前 10 个元素的范围内」的响应形状，不能推广成「客户端任何地方都没有该字段」。详见 `docs/r121-position-probe-findings.md` §2 第 1、7 条。

---

## 3. 结论文档重写（`docs/r121-position-probe-findings.md`）

- **§0 新增**：上一轮真机运行结果如实归档（`trace_id=e361d1b420373b6c57aaa06c`，三条来源的 status/result/关键字段，结论 inconclusive，安全边界全部符合预期）。
- **§1 结论表**：从「5 个模糊接口分类」改成**按 6 个实际端点逐行**，列为 `critical` / `status`+`result` / `endpoint_scanned` / `scanned_keys` / 命中键名（含 `owner` 与 `type`）/ 覆盖范围；全部保持「待填」，**没有预填任何字段名**。
- **§2 判读规则**：7 条，改成先看 `verdict`；明确 `endpoint_scanned` / `negative_conclusive` / `endpoints_critical_conclusive` 三个标志的区别；明确 critical 全 404 时必须 inconclusive；明确 `keys_truncated` 的三种成因；明确契约判据是「属性命中数」而非「端点命中数」以及 `contract_complete` 的五个条件；最后一条给出 `not-found` 的**范围边界**（深度 ≤3、每数组前 10 个元素、无因超限漏扫的结构），回填时不得省略。
- **§3 真机运行**：**新增「探测结果强依赖客户端所处阶段」的提示**，建议分两次跑（大厅已排队 / 英雄选择阶段）——上一轮 inconclusive 很可能就是因为跑探测时客户端不在这些阶段。命令的 `-run` 过滤从 `'R121|PositionContract'` 改为 `'R121Position'`（新增的四个测试都以 `R121Position` 开头，一并覆盖）。
- **§4 回填步骤**：改为读 `verdict` 等新字段。
- **§5 处置约定**：明确 `verdict=inconclusive` 时**不得删除**探测代码。

---

## 4. 改动清单

| 文件 | 改动 |
|---|---|
| `backend/position_contract_probe.go`（原 528 → **834** 行） | 删 `/help` 文本扫描全套（正则 + 函数 + 类型 + 2 个常量 + 来源条目 + collect 分支）；新增端点清单（6 项带 `critical`）、响应键名扫描器（`positionProbeResponseKeys` / `positionProbeWalkResponse` / `positionProbeJSONType` / `positionProbeHasChildren` / `positionProbeOwnerPath` + 2 个类型）、`positionProbeGet`、三态 `positionProbeVerdict` + 3 个常量；`positionProbeContractStats` 加 `MissingSchemas` / `UnresolvedRefs`；重写 `collectPositionContractProbe` 为「端点直探 + 契约补充」两段；含 §6 的 6 处复核修复 |
| `backend/position_contract_probe_test.go`（原 196 → **480** 行） | 新增 4 个测试（合成 lobby 响应 / 嵌套数组与裸字符串 / 三态 verdict 表驱动 8 例 / 不完整扫描不得成为否定证据 5 例）+ `r121HitIndex` 辅助；改造真机入口的判据（改读 `verdict`）与超时（90s → 150s） |
| `docs/r121-position-probe-findings.md`（原 64 → **102** 行） | 按 §3 重写，含上一轮真机结果归档、按端点逐行的结论表、7 条判读规则（含 `not-found` 的范围边界） |
| `desktop/package.json`、`desktop/package-lock.json` | 0.12.15 → **0.12.16**（3 处） |

**升版本的理由**：本轮只改一次性侦察代码，理论上不影响产品行为；但用户是**交叉编译独立二进制**在真机跑探测的，版本号是唯一能确认「装的是新探测还是旧探测」的手段，因此仍然升版。

**未改动**：`backend/riot_api.go`、`backend/gameplay.go`、`backend/web/*`、`backend/feature_gates.go`、`features.go` 隐私文案、`backend/augment_contract_probe*.go`（另一个探针，有自己的 `augmentProbe*` 前缀，不受影响）、历史 WORKLIST 与历史台账。

---

## 5. 验证状态

### 5.1 执行方实跑结果（用户本机 macOS）

| 命令 | 结果 |
|---|---|
| `gofmt -l backend/*.go` → 复跑 `gofmt -d backend/position_contract_probe.go` | **已清零**：`gofmt -d backend/position_contract_probe.go`（只打印差异、不改文件）**无任何输出**。**但首次 `gofmt -l` 报出它的原因始终没有查明**——我先前归因的「`final` map 字面量里注释 + 单键行 + 多键行混排」经复核**证伪**：`augment_contract_probe.go:326-331` 正是同一形状且长期干净。已把该文件改成与 `augment_contract_probe.go:404-411` 逐字符同构，并逐项核算了全文其余对齐敏感构造（见 §5.2）。**旧归因不得沿用** |
| `go vet ./backend` | **通过**（vet 静默即 PASS）→ 整包可编译，含本轮大改的探测文件与全部新测试 |
| `go test -race -run 'R121Position' ./backend -v` | **PASS，`ok lol-loot-assistant/backend 2.165s`，无 DATA RACE**。7 个测试全绿：`ContractsExpandSyntheticV3`、`ProbeRefCycleBounded`（既有 `$ref` 展开，证明本轮改动没破坏它们）、`ResponseKeysScanSyntheticLobbyBody`、`ReachNestedArraysAndRejectBareStrings`、`ProbeVerdictSeparatesReadFromConclusive`（8 个子用例全过，含「契约只有端点命中 → 仍 inconclusive」这条 §6 缺陷 #1 的回归锁）、`ProbeIncompleteScansNeverBecomeNegativeEvidence`（5 例，锁 §6 缺陷 #3/#4/#5）；`TestR121PositionContractProbeLiveClient` 按设计 SKIP |
| `go test -race ./backend`（全量） | **首次实跑 FAIL，但与 R124 无关**：三条报错全在 `backend/match_tier_cache.go:31-34`（`a.matchTierCacheOnce` / `a.matchTierCache` undefined）。该文件属**并行工单 R127**（文件头自述「R127 P1-c.3」），不是本轮改动，且它在本轮 `go vet ./backend` 通过之后才出现。复查发现 `backend/main.go:135-136` **现已声明**这两个字段且对齐与邻近字段一致，即报错是那一轮「先落 `match_tier_cache.go`、后补 `app` 字段」的中间态。**需再跑一次确认**；若仍失败，失败点属 R127，不记作 R124 回归 |
| `node --test backend/web/*.test.cjs desktop/*.test.cjs` | 本轮未改任何前端文件，预期与 R122 的 625/625（R123 之后为 623 项）一致；未回传 |

### 5.2 写代码时的硬边界

- **本会话 Bash 工具被拒（`TOOL_DENIED`）**，代码靠静态审查 + 两轮只读复核写成。
- 已做的替代保障：
  - 手工推演了新测试的全部断言：lobby fixture 的命中集合与 `owner` 路径、gameflow 三层嵌套的 `owner` 拼接顺序、12 元素数组只穿透 10 个、裸字符串与非法 JSON 的返回值、8 个 verdict 用例逐一代入实现；
  - 逐个核对了「不该命中」清单里的 13 个键名与谓词 `(?i)position|preference|pref|fill|autofill|role|lane|assigned` 的关系，确认无意外子串匹配；
  - 确认删除文本扫描后 `sort`、`strings`、`regexp` 三个 import 仍有使用点，不会产生「imported and not used」；
  - 确认 `positionProbeMatches` / `positionProbePathLimit` / `positionProbeMethods` 仍被契约扫描使用，不会变成未使用标识符。
- **gofmt 归因更正（重要，勿沿用旧结论）**：§5.1 首次那行我写的根因是「map 字面量里注释 + 单键行 + 多键行混排」，**这条是错的**，已被证伪：`augment_contract_probe.go:326-331` 是完全相同的形状（三行多键 + 结尾一行单键 `"temporary_reconnaissance_code": true,`，中间无注释）且长期 gofmt 干净。本文件的 `final` 字面量（749-769）现已与 `augment_contract_probe.go:404-411` 逐字符同构——`"negative_conclusive_all":` 后 7 空格、`"write_executed":` 后 16 空格、`"temporary_reconnaissance_code":` 后 1 空格，全部按 tabwriter「列宽 = 最长单元 + 1」核算并用正则实测。复跑 `gofmt -d` 无输出即已干净，但**首次报出的真实原因没有查明**（同一份内容不可能 `-l` 报而 `-d` 空，倾向于读到了编辑落盘的中间状态）。**不要把「不可混排」当规则写进后续工单。**
- 本轮为定位 gofmt 做的全量静态核算（全部通过，可当后续手工自查清单）：const / var 块对齐——注释行**会**断开对齐组（实测 `positionProbeSchemaDepth` 3 空格 = 组内最长 26 − 24 + 1）、`positionProbeVerdictFound` 8 空格 = 32 − 25 + 1；4 处 struct 类型字段列；map 字面量单键行（`"schema_name":` 8 空格 = 21 − 14 + 1）；import 单组且字母序；doc comment 的 `//   - ` 列表 + 5 空格续行、紧跟「：」行的写法与 `champions.go:315-316`、`gameplay.go:5587-5588` 同形状；doc comment 代码块为单 TAB 缩进（正则 `^//\t` 实测 8 行）；无空格缩进（`^[ ]+\S` 零命中）、无行尾空白（`[ \t]+$` 零命中）、无 CR。

### 5.3 仍待跑

```bash
go test -race ./backend   # 全量，约 4-5 分钟；若失败先看报错文件是否属并行工单 R127
node --test backend/web/*.test.cjs desktop/*.test.cjs
gofmt -l backend/*.go     # 应为空。gofmt -d backend/position_contract_probe.go 已实测无输出
```

---

## 6. 两轮独立对抗复核与修复（共 4 个 P1 + 2 个 P2）

改动完成后派**只读**审查子代理复核了两轮（编译性 / gofmt / 断言成立性 / 逻辑与安全边界 / 既有回归）。两轮都确认编译性与断言推演成立（含逐个核对 13 个「不该命中」的键名与谓词的关系、8 个 verdict 用例、lobby 扫 13 键命中 3 条、gameflow 扫 10 键命中 3 条、12 元素数组只穿透 10 个、既有两个 `$ref` 展开测试不受影响），并抓到 6 个会让探测**给出错误结论**的缺陷，均已修复：

| # | 轮 | 审查发现 | 严重度 | 修复 |
|---|---|---|---|---|
| 1 | 一 | 契约段的 `contractAllZero` 依据的是 `stats.MatchedEndpoints`（命中**路径**数）而不是 `MatchedProperties`（命中**属性**数）。后果：只要 openapi 里存在 `/lol-lobby/v2/lobby` 这个路径，即使其 schema 里一个位置字段都没有，`verdict` 也会返回 `found`——「端点存在」被当成「字段存在」 | **P1** | `positionProbeVerdict` 签名改为 `(endpointHits, contractHits, criticalConclusive int, contractConclusive bool)`，把「肯定证据」与「可作否定依据」拆成两组独立入参；契约段改用 `contractProps = stats.MatchedProperties` 累加 `contractHits`。verdict 测试新增第 8 例作为回归锁：「契约只有端点命中、属性零命中且不完整」必须是 `inconclusive` 而不是 `found` |
| 2 | 一 | 数组元素超过上限时直接 `break` 而**不标记截断**。反例 `{"members":[{…},{…},{"assignedPosition":"TOP"}]}`：只有第 3 个成员带目标字段，却会零命中且 `Truncated=false` → 被当成 `not-found`（假否定） | **P1** | 数组分支跳过元素时置 `scan.Truncated = true`；`positionProbeResponseArrayLimit` 由 2 提到 **10**（lobby / gameflow / champ-select 的队伍都最多 5 人，10 能保证关键端点整支队伍被扫到、不因截断失去下否定结论的资格；真正会超限的只有战绩那种大数组，而它是非关键端点）。测试改为 12 元素断言「只穿透 10 个 + Truncated=true」，另加 3 元素用例断言「不截断」 |
| 3 | 二 | **深度上限处静默跳过子对象**，不置 `Truncated`。反例 `{"members":[{"profile":{"deep":{"deeper":{"assignedPosition":…}}}}]}` 会零命中且 `Truncated=false`，进而被 critical 端点判据当成 `not-found`——与 #2 同型的假否定，只是发生在深度维度 | **P1** | 新增 `positionProbeHasChildren`：到达深度上限时，被跳过的值若仍是**非空 map/array** 就置 `Truncated`；若是标量或空容器则不置（否则任何真实响应都会被判不完整、关键端点永远无法下否定结论）。`positionProbeResponseDepth` 由 2 提到 **3**。新增测试用例 1、2 分别锁住这两面 |
| 4 | 二 | `contractComplete` 只检查 `ContractRead` / `SchemaTruncated` / `OperationTruncated`，但 operation **没有可读 response schema** 时 walker 根本不跑、`$ref` 解析失败时也是静默 `return`，两者都不留痕迹 → 零属性命中仍被当成完整否定证据 | **P1** | `positionProbeContractStats` 新增 `MissingSchemas`（命中端点但无可解析响应 schema 的 operation 数）与 `UnresolvedRefs`（`$ref` 形状非法 / 指向不存在节点 / 目标不是对象）；契约 walker 区分「循环引用正常收敛」与「解析失败」；`contractComplete` 追加 `MissingSchemas == 0 && UnresolvedRefs == 0`；两个计数写进事件便于诊断。新增测试用例 4、5 |
| 5 | 一 | `positionProbeOwnerPath` 无条件拼接原始祖先键名，`{"private name":{"assignedPosition":…}}` 会把未过 `SafeName` 白名单的 `private name` 经 `owner` 写进日志——白名单只保护了命中键名，没保护完整路径 | P2 | 新增 `positionProbeRedactedSegment = "?"`，祖先键名不过白名单就用占位符替代该段；新增测试用例 3 断言 owner 被脱敏且原始键名不外泄 |
| 6 | 一 | ctx 中途取消时循环 `break`，但 summary 里 `endpoints_probed` 仍固定写 `len(positionProbeEndpoints)`（6），会谎报「六个端点都探过了」 | P2 | 改为 `len(endpointSummaries)`（实际产出事件数） |

**第一轮我做错的一个判断，被第二轮推翻（如实记录）**：我原先认为「深度受限是刻意选择的扫描范围，不该算截断」，理由是否则关键端点永远无法下否定结论。第二轮用具体反例证明这会造成假 `not-found`。正确做法是**区分两种情况**：被跳过的是非空结构 → 算截断（如实降级为 `inconclusive`）；被跳过的只是标量 → 不算（保住下否定结论的资格）。这样既不会假否定，也不会退化成永远 `inconclusive`。同时把深度从 2 提到 3 给真实响应留余量。§1.4 第 1 条与结论文档 §2 第 4、7 条已同步改正。

**契约那一路的实际可用性（第二轮纠正了我的表述）**：我原先写「一份 3 MB 的真实 LCU openapi 很可能触发截断」，不准确——响应体上限是 16 MiB，3 MB 不会触发；契约截断只由 200 条 operation、schema 深度 6、属性 400 这三个上限触发。因此契约分支**并非必然永远不成立**：小型、完整、每个命中 operation 都有可遍历 schema 的契约可以让 `contract_complete=true`（现有合成 fixture 就满足）。对大型真实 openapi 若确实触发任一上限，让 `contractConclusive` 保持 false 是正确的保守行为，不应删除这条分支。结论文档 §2 第 5 条已改为明确列出这三个上限与两个新增计数。

---

## 7. 遗留

1. **真机复测未做**（工单 P1-1 的最后一步）：需要按 `docs/r121-position-probe-findings.md` §3 分两个阶段各跑一次。上一轮 inconclusive 的根因很可能就是客户端不在大厅/选人阶段。
2. **gofmt 已收口**：复跑 `gofmt -d backend/position_contract_probe.go`（只打印差异、不改文件）**无任何输出**，即当前文件已是规范形状。但首次 `gofmt -l` 报出它的**真实原因始终没有查明**，而我原先写进 §5.1 的归因（「单键行 + 多键行混排」）**已被证伪**——`augment_contract_probe.go:326-331` 是完全相同的形状且长期 gofmt 干净。详见 §5.2 的更正条目，**不要把那条当规则沿用**。`go vet` 与定向测试已实测通过（7 个测试全绿、`ok 2.165s`、无 DATA RACE）。
3. **`go test -race ./backend` 全量需再跑一次**：首次实跑 FAIL，但三条报错全在 `backend/match_tier_cache.go:31-34`（`a.matchTierCacheOnce` / `a.matchTierCache` undefined），该文件属**并行工单 R127**（文件头自述「R127 P1-c.3」，工单 `docs/WORKLIST-R127-OVERVIEW-MATCH-LIST-SLOW-LOAD.md`），不是本轮文件，且它在本轮 `go vet` 通过之后才出现。复查发现 `backend/main.go:135-136` **现已声明**这两个字段，说明报错发生在那一路「先落 `match_tier_cache.go`、后补 `app` 结构体字段」的中间态。R124 只改无生产调用方的侦察代码与其测试，再跑预期与 R122 的 `ok 281.937s` 同结果；**若仍失败，失败点属 R127，不得记作 R124 的回归**。
   版本号复核：`desktop/package.json` 仍为 **0.12.16**（本轮所改），未被并行工单覆盖。
4. **对抗变异未实跑**（工单说「变异测试可选」，但新增的合成 fixture 测试必须能在改回旧实现时 FAIL）。§1.5 列了六个变异落点，需要在隔离副本里实跑确认。
5. 若真机 `verdict=found`，命中谓词只代表键名含 position/preference/fill/role/lane/assigned 等词，**不等于该字段就是「补位」标记**，语义必须另行论证后才能动 `autofillLabelGate`。这一点已写进结论文档 §1 末尾。
6. 若确认存在真实字段并要落地「对局进行时记录己方补位信息」，会涉及 `features.go` 的 `stores` 隐私声明，**文案必须由用户拍板，执行方不得代写**（延续前几轮工单纪律）。
