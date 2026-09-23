# R122 执行台账：补位诊断计数「进不进事件」的护栏 + 命名收尾

**工单：** `docs/WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED.md`（用户下发副本同名）
**基线版本：** 0.12.14　**最终版本：** **0.12.15**（`desktop/package.json` + `package-lock.json` 三处已同步）
**轮次编号说明：** 工单自称 `WORKLIST-R119`，但仓库里 R119 这个名字已被上一张工单（`WORKLIST-R119-AUTOFILL-LABEL-GAPS.md`）占用、且那一轮的执行台账是 `docs/r121-execution-ledger.md`（工单正文也是这么引用的：「对 R121 台账…的独立复测」）。为避免两个 R119 混淆，**本轮按 R122 记账**，测试文件与台账都用 r122 前缀。

## 0. 三句话结论

1. **接受工单判断**：三个补位计数「从汇总结构写进诊断事件」这一步此前确实零护栏——删掉事件里任意一行赋值，全部测试仍通过；而 P1-1 的整个设计（开关关闭期间靠诊断收真机数据）就建立在这一步上。
2. **修法按工单第 1 条**：三行赋值抽成 `autofillDiagnosticFields(summary)`，`sgp_match_history_*` 与 `sgp_summary_history_*` 两个事件共用同一来源，键名与取值只有一处定义；再补三层测试（单元 / 详情页端到端 / 窗口端到端）。
3. **P3-1 命名收尾**：事件键 `autofill_tagged` → `autofill_candidates`、结构体字段 `AutofillTagged` → `AutofillCandidates`（开关关闭时一个标签都没打，「已打标签」是假陈述）；`TestR121NoUnprovenWordingInBusinessCode` 的禁用词表加入旧键名。

**生产口径零改动**：判定规则、90% 覆盖率门槛、30% 占比门槛、队列门禁、`autofillLabelGate` 默认关闭、前端渲染，全部一字未动（工单执行纪律第 3 条）。

**验证已全部闭合**（§5.1 / §1.4）：`gofmt` 干净、`go vet` 通过、定向子集 18 个测试全绿、`go test -race ./backend` 全量 `ok 281.937s` 无竞争、前端 `node --test` 625/625、**对抗变异 8/8 打红无一存活**。仍需真机的只剩 P3-2 对照（§3）与 P3-1 的 LCU 契约探测。

---

## 1. P2-1　三行赋值抽成纯函数 + 三层测试

### 1.1 重构（`backend/gameplay.go`）

```go
func autofillDiagnosticFields(summary participantCompletenessSummary) map[string]any {
	return map[string]any{
		"autofill_candidates":  summary.AutofillCandidates,
		"position_mismatch":    summary.PositionMismatch,
		"autofill_no_evidence": summary.AutofillNoEvidence,
	}
}
```

两个事件都改成合并这个 map，不再各写三行：

- `backend/gameplay.go:1462-1465`（`windowDiagnostic`，`sgp_summary_history_succeeded / _partial`）
- `backend/gameplay.go:2294-2299`（`diagnostic`，`sgp_match_history_succeeded / _partial`）

`gameplay.go` 除「抽函数 + 两处调用 + 改名 + 注释」外**没有任何逻辑改动**（工单验收判据第 3 条）。

### 1.2 三层测试（`backend/r122_autofill_diagnostic_test.go`，344 行）

| 层 | 测试 | 覆盖什么 |
|---|---|---|
| 单元 | `TestR122AutofillDiagnosticFieldsCarryExactlyThreeNeutralKeys` | 键集合**恰好**三个中性名（多一个、少一个、改一个都 FAIL）；取值逐项对应用 `1/2/3` 三个互不相同的值（工单要求，避免「全 0 也相等」）；另断言**零值汇总也产出全部三个键**——真机上一场候选都没有时，「键缺失」与「计数为 0」是两件事，诊断导出不能少字段 |
| 端到端（详情页） | `TestR122SGPMatchHistoryEventCarriesAutofillCounts` | 真跑 `loadDetailedMatches` 走 SGP 战绩分页，从诊断日志里取出 `sgp_match_history_succeeded`，断言三个键的**具体数值** = `2 / 3 / 1`；同时断言开关关闭、响应里无 `autofill` 键 |
| 端到端（窗口） | `TestR122SGPSummaryHistoryEventCarriesAutofillCounts` | 真跑 `loadGameplayOverview`，断言 `sgp_summary_history_succeeded` 的三个键**同样是 2 / 3 / 1**，并断言同一页数据的详情页事件给出同一组数 |

**共用 fixture 的三个数是怎么凑出来的**（`r122AutofillGames`，三场对局、每场 10 人）：

| 场 | 队列 | 形状 | 贡献 |
|---|---|---|---|
| 1 | 420 | 下标 8 `teamPosition=MIDDLE` / `individualPosition=BOTTOM`；下标 9 `UTILITY` / 空 | 候选 1、不一致 1（有效个人位置 9/10 = 90%，恰好过门槛） |
| 2 | 440 | 下标 5、6 两个不一致；下标 7 `JUNGLE` / `Invalid` | 候选 1、不一致 2 |
| 3 | 420 | 全场 `individualPosition` 为空 | 整场降级 → `no_evidence` 1 |

合计 **候选 2 / 不一致 3 / 降级 1**，三个数互不相同且都非零，因此「取值错配」与「传了零值汇总」两类变异都会被抓到。

**窗口事件怎么做到既能触发、又能断言真实数值**（本轮唯一有点绕的地方。独立复核第一版在这里只断言了「键存在、值为 0」，被审查打回——那样「把窗口事件的实参换成零值汇总」这个变异会存活）：

窗口分支的进入条件是 `shouldLoadOverviewHistory` 里的 `len(matches) == 0`（`backend/gameplay.go:6606`）——**详情页必须返回 0 场可见对局**。而「可见性」与「计数」用的是两个不同字段：

- `customGameplayMatchReasons`（`backend/gameplay.go:9966-9988`）看 `gameType`，`CUSTOM_GAME` 即过滤；
- `summarizeRiotParticipants`（`backend/gameplay.go:2085`）**只看 `QueueID`**，420/440 就计数，不看 `gameType`。

所以 fixture 用 **`queueId=420/440` + `gameType=CUSTOM_GAME`**：界面上不可见（`matches` 为空 → 窗口分支进入），计数里却非零（→ 窗口事件可以断言 2/3/1）。测试还额外断言 `len(overview.Matches) == 0`，把「窗口分支的前置条件确实成立」钉死，避免前置条件失效时靠别的路径蒙混通过。

**`_partial` 事件的覆盖范围（如实收窄）**：本轮只断言 `sgp_match_history_succeeded` 与 `sgp_summary_history_succeeded`，**没有**构造 `_partial` 的失败分页 fixture。理由是这两个名字与 `_partial` 共用同一段无条件代码：事件名只是 map 字面量里的一个取值（`backend/gameplay.go:2259-2264`、`1449-1452`），而 `autofillDiagnosticFields` 的合并循环在名字确定之后、`recordDiagnostic` 之前**无条件**执行（`2297-2299`、`1463-1465`），`partialErr` 只影响额外补一个 `reason` 键。要让 `_partial` 独独漏掉这三个键，必须新增分支，那已经不是「重构丢键」这类退化形状。

### 1.3 验收判据对照

工单要求「上表四个变异各自使新增测试 FAIL」+「再加两个变异也必须 FAIL」。重构后变异的落点变了，逐条对应：

| # | 工单原始变异 | 重构后的等价变异 | 打红的测试 |
|---|---|---|---|
| 1 | 删 `diagnostic["position_mismatch"] = …` | 删 `autofillDiagnosticFields` 里的 `"position_mismatch"` 行，或删详情页的合并循环 | 第 1 层（键数量 3→2）；若是删循环则第 2 层 |
| 2 | 删 `diagnostic["autofill_no_evidence"] = …` | 同上，换一行 | 同上 |
| 3 | 删 `windowDiagnostic["position_mismatch"] = …` | 删窗口事件（`gameplay.go:1463-1465`）的合并循环 | 第 3 层 |
| 4 | 删 `windowDiagnostic["autofill_tagged"] = …` | 同上 | 第 3 层 |
| ① | 事件里把 `position_mismatch` 换回 `autofill_swapped` | 改 `autofillDiagnosticFields` 的键名 | 第 1 层 + `TestR121NoUnprovenWordingInBusinessCode` |
| ② | 事件里 `autofill_tagged` 的值改成 `AutofillNoEvidence` | 把 `"autofill_candidates"` 的取值换成 `summary.AutofillNoEvidence` | 第 1 层（1 vs 3）+ 第 2 层（2 vs 1） |
| ③ | （审查新增）把窗口事件的实参换成零值汇总 `participantCompletenessSummary{}` | 同左 | 第 3 层——窗口事件现在断言真实数值 2/3/1，零值会给 0/0/0 |
| ④ | （审查新增）把详情页事件的实参换成零值汇总 | 同左 | 第 2 层 |

**改名回 `autofill_tagged`** 由 `TestR121NoUnprovenWordingInBusinessCode` 直接打红（禁用词表本轮新增该键名）。

### 1.4 实跑结果（用户本机 macOS，`r122-mutations.sh`）

按工单纪律在 `/tmp` 隔离副本里逐条实跑：每轮「还原基线 → 施加变异 → `diff -q` 确认变异真的生效 → `go test -count=1 -run 'R119|R121|R122|Autofill|SummarizeRiot' ./backend` → 还原 → `diff -q` 校验」。**全程未碰真实工作目录，未使用 `git checkout -- <file>`**；副本结束时与基线逐字节一致。基线先跑一次确认 PASS（`ok … 1.388s`），否则「变异打红」无意义。

实际执行的命令与逐条结果：

```bash
# 第 1 轮：全量 8 条（M1–M7 打红；M8 因脚本正则缺陷产生 syntax error，被判「未能判定」）
bash r122-mutations.sh                     # 在仓库根 /Users/ly/personal/personal-work/deep-legends 下执行
# 第 2 轮：只重跑 M8（脚本正则修正后）
ONLY=8 bash r122-mutations.sh              # → ✔ 被打红，基线 ok … 1.280s，副本还原校验一致
```

脚本本体是会话 scratch 里的 `r122-mutations.sh`（一次性工具，不进仓库）；每条变异的 perl 改写规则等价于下表描述，均只作用于副本里的 `backend/gameplay.go` 单文件。

| # | 变异 | 结果 | 实际打红的测试 |
|---|---|---|---|
| M1 | 删共享函数里 `"position_mismatch"` 键 | **✔ 打红** | `R122AutofillDiagnosticFields` + 两个端到端 |
| M2 | 删共享函数里 `"autofill_no_evidence"` 键 | **✔ 打红** | 同上 |
| M3 | 删窗口事件的合并循环 | **✔ 打红** | `R122SGPSummaryHistoryEventCarriesAutofillCounts` |
| M4 | 删详情页事件的合并循环 | **✔ 打红** | `R122SGPMatchHistoryEventCarriesAutofillCounts`、`R122SGPSummaryHistoryEventCarriesAutofillCounts` |
| M5 | 键名改回 `autofill_swapped` | **✔ 打红** | `R121NoUnprovenWordingInBusinessCode` + `R122AutofillDiagnosticFields` + 两个端到端 |
| M6 | `autofill_candidates` 取值换成 `AutofillNoEvidence` | **✔ 打红** | `R122AutofillDiagnosticFields` + 两个端到端 |
| M7（=③） | 窗口事件实参换成零值汇总 | **✔ 打红** | `R122SGPSummaryHistoryEventCarriesAutofillCounts` |
| M8（=④） | 详情页事件实参换成零值汇总 | **✔ 打红**（重跑） | `R122SGPMatchHistoryEventCarriesAutofillCounts`、`R122SGPSummaryHistoryEventCarriesAutofillCounts` |

三点如实记录：

1. **M4 与 M8 的红都比预期多一个**：这两条变异分别破坏详情页事件的「合并循环」与「实参」，除打红 `R122SGPMatchHistoryEventCarriesAutofillCounts` 外，`R122SGPSummaryHistoryEventCarriesAutofillCounts` 也一起红了。原因是后者在断言完窗口事件后，还会断言同一页数据的详情页事件（`r122_autofill_diagnostic_test.go` 末尾两行），属于交叉覆盖，不是误报。反过来 M3 / M7（只破坏窗口事件）则**精确**只红窗口用例——这说明两个事件的护栏是各自独立生效的，没有互相顶包。
2. **M8 首轮「未能判定」是脚本缺陷、不是护栏缺陷**：我写的 perl 替换文本多插了一个 `}`，把代码改成了 `syntax error: unexpected name diagnostic`。脚本刻意把「编译错误」与「断言抓到」分开报告，正是为了不让语法破坏冒充变异被杀——这条判据按设计工作了。修正正则（捕获组完整包含 `diagnostic[key] = value` 及其闭合括号）后重跑，M8 被断言正常打红。
3. **两轮实跑的基线都是 PASS**（`ok … 1.388s` / `1.280s`），副本结束时均与基线逐字节一致，真实工作目录未被修改，全程未使用 `git checkout -- <file>`。

---

## 2. P3-1　`autofill_tagged` → `autofill_candidates`

| 项 | 改前 | 改后 |
|---|---|---|
| 诊断事件键 | `autofill_tagged` | `autofill_candidates` |
| 结构体字段 | `participantCompletenessSummary.AutofillTagged` | `.AutofillCandidates` |
| 禁用词表 | `autofill_swapped` | `autofill_swapped` + `autofill_tagged` |
| 活文档 | `docs/r119-execution-ledger.md` §5 的判读步骤与观测栏表头用旧键名 | 已改为新键名，并注明改名原因 |

**为什么现在改成本最低**（工单原话）：这三个键从 0.12.13 才出现，还没有任何真机导出在用。

**验收判据对照**

- `grep -rn 'autofill_tagged\|AutofillTagged' backend/ desktop/ docs/`：
  - `backend/riot_api.go`、`backend/gameplay.go`、`backend/web/gameplay.js`（业务代码）**零命中** ✓；
  - `desktop/` **零命中** ✓；
  - `docs/` 仅剩**历史文档**（两张 WORKLIST、`r119-independent-verification.md`、`r121-execution-ledger.md`）——工单判据本身写了「除历史台账」，这些是审查记录，按纪律不改写历史；
  - `backend/r121_autofill_test.go:257` 与 `backend/r122_autofill_diagnostic_test.go:137` 各有一处命中，**这是护栏本身**：禁用词表必须包含被禁字面量才能检查它，注释里也必须写出旧名才能说明改了什么。这不是漏改。
- `TestR121NoUnprovenWordingInBusinessCode` 的禁用词表已加入 `autofill_tagged`（连同 `autofill_swapped` 一起做成列表），改回去必须 FAIL ✓。

---

## 3. P3-2　真机对照（仍需你操作，本轮无法闭合）

没有变化：`autofillLabelGate` 保持 `false`，`TestR121AutofillLabelGateIsClosedByDefault` 守着。回填表格在 `docs/r119-execution-ledger.md` §5「观测栏」，**表头键名本轮已同步为 `autofill_candidates`**，照着填即可。

位置探测 `backend/position_contract_probe.go` 同样在等真机运行，结论表 `docs/r121-position-probe-findings.md` 仍全部是「待真机运行填写」；填完后按该文档 §4 整体删除探测文件。

---

## 4. 改动清单

| 文件 | 改动 |
|---|---|
| `backend/gameplay.go` | 新增 `autofillDiagnosticFields`；两处事件改为合并该函数的返回值；`AutofillTagged` → `AutofillCandidates`（含结构体注释重写）；`summarizeRiotParticipants` 里的自增改名 |
| `backend/r122_autofill_diagnostic_test.go`（新增，344 行） | 3 个测试 + 8 个 fixture/断言辅助函数（`r122NormalLanes`、`r122SGPGame`、`r122EncodeGames`、`r122AutofillGames`、`r122AssertAutofillKeysPresent`、`r122AssertAutofillCounts`、`r122Number`、`r122Store`）；读诊断事件复用仓库既有的 `r116dDiagnosticEvents`（`gameplay_r116d_test.go:1327`），不另造轮子 |
| `backend/r121_autofill_test.go` | `AutofillTagged` → `AutofillCandidates`；禁用词表由单条 `autofill_swapped` 判断改成列表，加入 `autofill_tagged` |
| `backend/r119_autofill_test.go` | `AutofillTagged` → `AutofillCandidates` |
| `docs/r119-execution-ledger.md` | §5 判读步骤、观测栏表头的键名同步为 `autofill_candidates`，并注明改名原因 |
| `desktop/package.json`、`desktop/package-lock.json` | 0.12.14 → **0.12.15**（3 处） |

**未改动**：`backend/riot_api.go`（判定与门槛一字未动）、`backend/web/gameplay.js` / `gameplay.css`、`backend/position_contract_probe*.go`、`features.go` 隐私文案、历史 WORKLIST 与历史验证文档。

---

## 5. 验证状态

### 5.1 执行方实跑结果（用户本机 macOS）

| 命令 | 结果 |
|---|---|
| `gofmt -l backend/*.go` | **无输出 = 全部格式正确**。`docs/r119-independent-verification.md:25` 记录的 `backend/gameplay.go` 对齐问题已不复现（R121 把 `Autofill` 写成两空格对齐，本轮 `AutofillCandidates` 与 `AutofillNoEvidence` 等长，均按 gofmt 分组规则对齐） |
| `go build ./backend` | **命令本身失败**：`go: build output "backend" already exists and is a directory`。这是命令写法问题、不是代码问题——`go build ./backend` 要在当前目录产出名为 `backend` 的可执行文件，与 `backend/` 目录同名冲突。Windows 上产物是 `backend.exe`、不冲突，所以 `AGENTS.md` 里那条命令在 Windows 可用、在 macOS/Linux 不可用 |
| `go vet ./backend` | **通过**（vet 通过时不打印任何内容，静默即 PASS）。它连同 `_test.go` 一起做类型检查，因此**整个包可编译已被证实**——包括 R121 新增、此前从未编译过的 `backend/position_contract_probe.go`，以及本轮新增的 `backend/r122_autofill_diagnostic_test.go` |
| `go test -race -run 'R119\|R121\|R122\|Autofill\|SummarizeRiot' ./backend -v` | **PASS，`ok lol-loot-assistant/backend 2.119s`，无 DATA RACE**。18 个测试全绿：R119 6 个、R121 7 个（含 `TestR121PositionContractsExpandSyntheticV3` 与 `TestR121PositionProbeRefCycleBounded` 两个探针纯函数测试）、R122 3 个、既有 `TestSummarizeRiotParticipants*` 2 个（回归）。`TestR121PositionContractProbeLiveClient` 按设计 SKIP（未设 `R121_POSITION_PROBE=1`） |
| `go test -race ./backend`（全量） | **PASS：`ok lol-loot-assistant/backend 281.937s`**，无 FAIL、无 `WARNING: DATA RACE`。约 4 分 42 秒，相对 R116 台账记录的 222s 增长与这几轮新增的测试量相符。这条确认了本轮改动 `gameplay.go` 两处诊断事件拼装方式后，**没有任何既有测试依赖旧的键名或旧的赋值形状** |
| `node --test backend/web/*.test.cjs` | **625 / 625 通过，0 失败 0 跳过**，耗时 12.2s。其中 R119 的 3 个补位标签前端用例（提示措辞、逐参与者渲染、CSS 令牌）全部通过——这同时验证了 R121 那轮的前端改动 |

**结论：Go 与前端两侧全部验证通过，对抗变异 8/8 打红（§1.4），本轮改动的自动化验证全部闭合。** 剩下的只有需要真机的 P3-2 对照（§3）与 P3-1 的 LCU 契约探测。

### 5.2 macOS / Linux 上的正确命令（`AGENTS.md` 那条构建命令在这两个平台不可用）

`go vet` 会连同测试文件一起做类型检查，因此它一条就能覆盖「能不能编译」；`go test` 再覆盖行为：

```bash
go vet ./backend && go test -race ./backend
# 只想先快速确认编译与本轮用例：
go test -race -run 'R119|R121|R122|Autofill|SummarizeRiot' ./backend -v
# 确实要产出二进制时，显式指定输出路径，别让它撞 backend/ 目录：
go build -o /tmp/deep-legends-build ./backend
```

另外：CI 跑的是 `node --test backend/web/*.test.cjs desktop/*.test.cjs`（`.github/workflows/ci.yml:42`），本次只跑了前半段。本轮未改 `desktop/` 下任何代码（只动了 `package.json` / `package-lock.json` 的版本号），但若要完全对齐 CI，可把 `desktop/*.test.cjs` 一并跑。

### 5.3 写代码时的硬边界

- 执行方（本会话）的 Bash 工具全程被拒（`TOOL_DENIED`），代码与测试**全部靠静态审查写成**。
- 已做的替代保障：
  - fixture 数值逐场手算过（覆盖率 9/10 = 90% 恰好达标、候选占比 1/10 ≤ 30%、三场合计 2/3/1），与 `riotAutofillCandidates` 的两道护栏一致；
  - 事件名、SGP 响应形状（`{"games":[{"json":{…}}]}`）、`loadDetailedMatches`（10 参）与 `loadGameplayOverview`（9 参）的调用签名、`trackTestStore` 返回 `*localStore`、`localStore.readDiagnosticLog`、`gameplayOverview.Matches`（`backend/gameplay.go:231`）逐一核对；
  - 诊断事件读取复用仓库既有的 `r116dDiagnosticEvents`（`gameplay_r116d_test.go:1327`），不新造解析逻辑；
  - 结构体与 map 字面量对齐按 gofmt 分组规则手写，并已由 §5.1 的 `gofmt -l` 实测确认干净。

---

## 6. 遗留

1. ~~`go test -race ./backend` 全量~~ **已闭合**：`ok lol-loot-assistant/backend 281.937s`，无 FAIL、无 `WARNING: DATA RACE`（§5.1）。连同 `gofmt`、`go vet`、定向子集、前端 625/625，本轮**所有自动化验证全部通过**。
2. **`AGENTS.md` 记录的构建命令在 macOS/Linux 上不可用**：文档写的是 `go build ./backend`；Windows 产物是 `backend.exe`、不冲突，macOS/Linux 产物是 `backend`、与目录同名会直接报 `build output "backend" already exists and is a directory`。建议改成 `go build -o <输出路径> ./backend`，或直接用 `go vet ./backend` 做编译检查。本轮**没有改 `AGENTS.md`**（超出工单范围，需用户点头）。
3. **P3-2 真机对照未做**（需要真机与真实对局）。开关保持关闭直到 `docs/r119-execution-ledger.md` §5 观测栏回填。
4. ~~对抗变异~~ **已闭合**：8 条全部实跑打红（§1.4），无一存活。
5. `backend/r122_autofill_diagnostic_test.go` 第 3 层测试用 30 秒超时兜底 `loadGameplayOverview`；实测该用例 **0.02s** 返回（§5.1），既有同类测试也在 750ms 内返回，因此正常不会触发。若 CI 上偶发超时，应优先怀疑该测试的 SGP/LCU 桩是否漏了某个端点，而不是放宽超时。

---

## 7. 独立对抗复核与修复（本轮抓到 3 处，其中 1 处是测试强度不足）

改动完成后派了一个**只读**审查子代理，按五项复核（编译性 / gofmt 稳定性 / 断言是否真成立 / 既有回归面 / 工单六条变异是否真被打红）。结论与处置：

| # | 审查发现 | 严重度 | 处置 |
|---|---|---|---|
| 1 | 第 3 层（窗口事件）原本只断言「三个键存在、值为 0」，因为旧 fixture 用的是 `queueId=0 + CUSTOM_GAME`。这让**「把 `windowDiagnostic` 的实参换成零值汇总 `participantCompletenessSummary{}`」这个变异存活**——键在、值也「对」（都是 0）。审查同时指出：我台账里「自定义对局不属于 420/440，所以只能全零」的说法**不受代码保证**，因为 `summarizeRiotParticipants` 只按 `QueueID` 统计、`customGameplayMatchReasons` 只看 `gameType`，两者判据不同 | **P1（护栏强度不足 + 台账论断不实）** | fixture 改为 **`queueId=420/440` + `gameType=CUSTOM_GAME`**：界面上不可见（满足窗口分支的 `len(matches)==0` 前置条件），计数里却非零。窗口事件现在断言真实数值 **2/3/1**，并额外断言 `len(overview.Matches)==0` 钉死前置条件。三场 fixture 抽成 `r122AutofillGames(t, playerRef, gameType)` 供两层测试共用，数值断言抽成 `r122AssertAutofillCounts`。变异 ③④ 已补进 §1.3 表格 |
| 2 | 工单变异表写的是 `sgp_match_history_succeeded / _partial`，而我只覆盖了 `_succeeded`，没有 `_partial` 的失败分页 fixture | P2（承诺超出覆盖） | 按工单允许的第二种做法**如实收窄台账承诺**（§1.2 末段），并给出代码层面的理由：事件名只是同一个 map 字面量里的一个取值，合并循环在名字确定后、`recordDiagnostic` 前**无条件**执行，`partialErr` 只额外补一个 `reason` 键。要让 `_partial` 独独丢键必须新增分支，不属于「重构丢键」这类退化 |
| 3 | 台账写的行数（347）与辅助函数清单（含已被我删掉的 `r122DiagnosticEvent`）与文件实际状态不符 | P3 | §1.2 标题、§4 改动清单已同步为 344 行 / 8 个辅助函数，并写明诊断事件读取**复用**仓库既有的 `r116dDiagnosticEvents`（`gameplay_r116d_test.go:1327`）而不是另造一个 |

审查确认**无问题**的项（可复核）：

- 编译性：新测试文件的 import 全部被使用且无缺失；所有被调用的既有标识符（`autofillLabelGate`、`newSGPProvider` 与 `sgpProvider` 各字段、`LCUClient` 各字段、`app` 各字段、`newLPTracker(nil)`、`trackTestStore` 返回 `*localStore`、`localStore.readDiagnosticLog`、`loadDetailedMatches` 10 参、`loadGameplayOverview` 9 参）签名逐一核对通过；`[10][2]string` 形实参一致；无 `data, err :=` 重复声明；`r122*` 与 `TestR122*` 共 11 个新标识符在 `backend/` 内无同包重名。
- gofmt：`participantCompletenessSummary` 三个计数字段、`autofillDiagnosticFields` 的三个 map 键、fixture 里相邻单键行、`app` 复合字面量的多键同行写法，均判定为 gofmt 稳定。
- 断言成立性：独立重算了三场 fixture，确认合计就是 `2 / 3 / 1`（含 `Invalid` 被规范化成空串因此算候选而非 mismatch）；确认 SGP 分页在 `consumed=3 < pageSize` 后立刻停止、不会重复返回；确认 `queueId=0`/`CUSTOM_GAME` 会被过滤、`windowAvailable` 置真后 1493 的复用分支与 1500 的 LCU 兜底都不会替代窗口事件；确认桩端点齐全、不会挂到 30 秒超时。
- 回归面：除 R122 外没有任何测试对这两个事件的**键集合**做精确断言（`gameplay_test.go:1647` 只检查不应出现 partial/failed 字符串）；`backend/web/` 全无旧键名；`web/r119.test.cjs` 只依赖参与者级 `autofill` 布尔字段，与本轮诊断键无关。

**审查同样无法执行编译器**（Bash 被拒），因此 §5 的构建与测试命令仍必须由用户实跑；这也是本轮唯一未闭合的验证项。
