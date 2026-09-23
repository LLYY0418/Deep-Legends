# R121-P3-1 探测结论文档：位置偏好 / 分配位置 / 补位字段

> **本文档只提供结论回填骨架，所有表格值必须由真机运行产生。** 禁止根据常识、客户端版本或合成 fixture 推断任何字段名或结论——合成 fixture 只用于验证扫描逻辑本身，不是真机证据。

- 工单出处：`WORKLIST-R121` P3-1，以及 `WORKLIST-R121-POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE`（首次真机运行后的整改）
- 探测代码：`backend/position_contract_probe.go`、`backend/position_contract_probe_test.go`
- 探测名：`r121-position-preference-inventory`
- 安全边界：只发 GET，永不写客户端；事件里的 `write_executed` 恒为 `false`；日志只保存**形状**（端点路径、HTTP 状态、键名、值类型、键所在对象路径），不保存任何响应体取值、账号标识、召唤师名或对局数据。

---

## 0. 上一轮真机运行结果（已完成，结论为 inconclusive）

执行环境：真实 WeGame 客户端，交叉编译的独立 Windows 探测二进制。
`trace_id=e361d1b420373b6c57aaa06c`，导出文件 `r121-position-probe-output.jsonl`（5 条事件）。

| 来源 | 状态 | 结果 | 关键字段 |
|---|---|---|---|
| `/swagger/v3/openapi.json` | 404 | `unavailable` | `contract_read=false` |
| `/swagger/v2/swagger.json` | 404 | `unavailable` | `contract_read=false` |
| `/help?format=Full` | 200 | `ok` | `body_bytes=3022367`，但文本兜底只扫出 `paths_scanned=1`、`matched_path_count=0` |
| 汇总 | — | — | `contract_read_any=true`、`contract_read_structured=false`、`negative_conclusive_all=false` |

**结论：不可判定（inconclusive）。** 不是「查了、确实没有」，而是「压根没查到」。安全边界全部符合预期（三条事件 `write_executed` 均为 `false`，无任何响应取值落盘），这部分无需返工。

根因与整改见 `WORKLIST-R121-POSITION-PROBE-HELP-FALLBACK-INEFFECTIVE` §1 与本轮台账：`/help?format=Full` 输出的是驼峰式 RPC 名而非字面 REST 路径，「扫斜杠路径」的兜底策略对它天然无效。已按该工单 P1-1 删除 `/help` 文本兜底，改为**端点直探 + 响应键名扫描**。

---

## 1. 结论表（待新一轮真机运行填写）

执行时间：`待真机运行填写`　客户端版本/区服：`待真机运行填写`　`trace_id`：`待真机运行填写`　导出文件：`待真机运行填写`
探测时客户端所处阶段（看 `/lol-gameflow/v1/gameflow-phase` 那条事件）：`待真机运行填写`

| 端点 | critical | `status` / `result` | `endpoint_scanned` | `scanned_keys` | 命中键名（`matched_keys` 里的 `name`，含 `owner` 与 `type`） | 覆盖范围（己方·双方·仅实时·可事后回看） |
|---|---|---|---|---|---|---|
| `/lol-lobby/v2/lobby` | 是 | 待填 | 待填 | 待填 | 待填 | 待填 |
| `/lol-champ-select/v1/session` | 是 | 待填 | 待填 | 待填 | 待填 | 待填 |
| `/lol-gameflow/v1/session` | 是 | 待填 | 待填 | 待填 | 待填 | 待填 |
| `/lol-gameflow/v1/gameflow-phase` | 否 | 待填 | 待填 | 待填 | 待填（裸字符串响应，`scanned_keys` 应为 0） | 不适用 |
| `/lol-match-history/v1/products/lol/current-summoner/matches` | 否 | 待填 | 待填 | 待填 | 待填（预期至少含 `teamPosition`/`individualPosition`，可用来确认扫描器有效） | 待填 |
| `/lol-end-of-game/v1/eog-stats-block` | 否 | 待填 | 待填 | 待填 | 待填 | 待填 |

**`verdict`（取自 `position_contract_probe_summary`）**：`待真机运行填写`（`found` / `not-found` / `inconclusive` 三选一，不得预填）
**`inconclusive_reason`**：`待真机运行填写`

若 `verdict=found`，再补一张「字段 → 语义 → 是否等于补位」的判断表；**注意命中谓词只代表键名含 position/preference/fill/role/lane/assigned 等词，不等于该字段就是「补位」标记**，语义必须另行论证。

## 2. 证据判读（新口径）

1. **先看 `verdict`，不要先看 `status`。** 三态含义：
   - `found`：任一端点响应里扫到命中谓词的键（`endpoint_matched_keys > 0`），或契约 schema 里有属性命中（`contract_matched_properties > 0`）→ 肯定结论，把 `matched_keys` / `matched_properties` 抄进结论表。
   - `not-found`：没有任何肯定证据，且至少一路否定依据成立——**要么**至少一个 critical 端点（lobby / champ-select / gameflow session）证据完整（200 + 合法 JSON + `scanned_keys > 0` + `keys_truncated=false`），**要么**契约来源被完整读取（`contract_complete=true`）且属性零命中。此时可以写「这些端点的响应里没有位置偏好/补位字段」，但**范围受 §2 第 7 条的两条边界限定**。
   - `inconclusive`：既没命中，也没有任何能支撑否定结论的完整扫描 → 本轮不构成结论，必须换阶段重跑（见 §3）。`inconclusive_reason` 会说明原因。
2. **三个标志要分清**：`endpoint_scanned` 只表示「返回了合法 JSON 且至少扫到一个键」；`negative_conclusive` 才表示「它的零命中可作为否定证据」，额外要求 `keys_truncated=false`；汇总层的 `endpoints_critical_conclusive` 只统计 critical 端点里 `negative_conclusive` 或命中的那些。`endpoint_scanned=false` 的三种情况都不构成否定证据：HTTP 非 200、响应不是合法 JSON、响应是合法 JSON 但一个键都没有（例如 `gameflow-phase` 返回裸字符串 `"ChampSelect"`）。
3. **critical 端点全部 404 时，即使 match-history 扫了几千个键零命中，`verdict` 也必须是 `inconclusive`。** 位置偏好最可能落在大厅 / 选人对象里，那两个没看到就不能下否定结论——这是上一轮「读到 3MB 却什么都没查到」教训的直接固化。
4. `keys_truncated=true` 有三种成因：响应键数超过 4000 上限、某个数组的元素数超过 10 个上限、或到达深度上限时被跳过的值下面还有非空结构。任一情况下该端点的 `negative_conclusive` 恒为 `false`（漏扫的部分里可能就有目标键）。战绩端点在真机上大概率触发前两种，属预期；它是非 critical 端点，不影响 `verdict`，但结论表里要注明。
5. **契约来源的判据是「属性命中数」而不是「端点命中数」。** `matched_endpoints` 只说明「契约里存在这个路径」，与「这个路径的 schema 里有没有位置字段」毫无关系；只有 `matched_properties` / `contract_matched_properties` 才是肯定证据。`contract_complete=true` 要求五个条件同时成立：契约被读到、schema 展开未触上限（深度 6 / 属性 400）、operation 明细未触上限（200 条）、`missing_response_schemas=0`（每个命中端点都有可解析的响应 schema）、`unresolved_refs=0`（所有 `$ref` 都解析成功）。任一条不满足，「零属性命中」就只是**没展开出来**而不是确实没有，契约那一路无法支撑 `not-found`，`verdict` 只能由端点直探决定。
6. 只抄日志里实际出现的键名与类型，不把 fixture、推断或记忆中的字段名写进结论。
7. **`not-found` 的范围边界（回填结论时必须一并写明）**：扫描覆盖「深度 ≤ 3 的键」与「每个数组的前 10 个元素」。到达深度上限时，若被跳过的值下面**还有非空结构**，会置 `keys_truncated=true`，该端点随即失去否定证据资格（`verdict` 降级为 `inconclusive`）；若被跳过的只是标量则不算截断——否则任何真实响应都会被判不完整，关键端点永远无法下否定结论。因此 `not-found` 的严格表述是：**在深度 ≤3、每数组前 10 个元素的范围内，这些端点的响应里没有命中谓词的键，且没有任何因超限而漏扫的结构**。

## 3. 真机运行

**关键：探测结果强依赖客户端所处阶段。** lobby 端点只在**已进入队列/大厅**时有效，champ-select 只在**英雄选择阶段**有效。上一轮之所以 inconclusive，很可能就是跑探测时客户端不在这些阶段。因此建议分两次跑：

- 第一次：客户端停在**大厅且已选好想打的队列**（单双排/灵活组排，最好已经点了「寻找对局」但还没进选人）；
- 第二次：客户端处于**英雄选择阶段**（此时 champ-select 与 gameflow session 都有内容）。

启动并登录 League 客户端后，在仓库根目录执行。`R121_POSITION_PROBE_LOCKFILE` 可填客户端 lockfile 绝对路径；自动发现成功时可留空。

### Git Bash / WSL

```bash
R121_POSITION_PROBE=1 \
R121_POSITION_PROBE_LOCKFILE='D:/Games/League of Legends/LeagueClient/lockfile' \
R121_POSITION_PROBE_OUTPUT=docs/r121-validation/position-endpoint-probe.jsonl \
go test ./backend -run 'R121Position' -v -timeout 5m
```

### PowerShell

```powershell
$env:R121_POSITION_PROBE="1"
$env:R121_POSITION_PROBE_LOCKFILE="D:\Games\League of Legends\LeagueClient\lockfile"
$env:R121_POSITION_PROBE_OUTPUT="docs/r121-validation/position-endpoint-probe.jsonl"
go test ./backend -run 'R121Position' -v -timeout 5m
```

测试会把诊断日志写入临时本地存储，并在设置 `R121_POSITION_PROBE_OUTPUT` 时导出完整 JSONL。测试自身会在结尾打印 `verdict` 并据此判定：`inconclusive` 会让测试**失败**（这是刻意的，避免把「没查到」当成「查过了」而静默通过）。

## 4. 回填步骤

1. 打开导出的 JSONL，找到 `position_contract_probe_summary`，记录 `verdict`、`inconclusive_reason`、`endpoints_probed`、`endpoints_scanned`、`endpoints_critical_conclusive`、`endpoint_keys_scanned`、`endpoint_matched_keys`。
2. 对每条 `position_endpoint_probe` 事件，按 `endpoint_path` 逐行填 §1 的结论表：`status`/`result`、`endpoint_scanned`、`scanned_keys`、`matched_keys`（只抄 `name`/`type`/`owner`）。
3. 若 `verdict=inconclusive`：§1 的 `verdict` 行如实写 `inconclusive`，其余单元格保留「待真机运行填写」或填实际的 404 状态，**不要写「不存在」**；然后按 §3 换阶段重跑。
4. 回填完成后复核：所有事件的 `write_executed` 均为 `false`；导出内容里没有任何响应体取值、召唤师名、puuid 或对局数据。

## 5. 探测代码处置约定

结论表回填为 `found` 或 `not-found`（即**可判定**）并完成复核后，立即整体删除 `backend/position_contract_probe.go` 与 `backend/position_contract_probe_test.go`；探测代码不进入正式版本，本文档保留为证据存档。删除后按项目惯例重跑 `go vet ./backend`、`go test -race ./backend`、`node --test backend/web/*.test.cjs desktop/*.test.cjs`，并把结果记入对应执行台账。

`verdict=inconclusive` 时**不得删除**探测代码——那等于把「没查到」当成「查过了」，下一轮就再也补不上这个窟窿。
