# R121 执行台账：「补位」标签的判定依据与护栏整改

**工单：** `docs/WORKLIST-R119-AUTOFILL-LABEL-GAPS.md`（用户下发副本标题为 `WORKLIST-R121`，本轮按 **R121** 记账）
**基线版本：** 0.12.13　**最终版本：** **0.12.14**（`desktop/package.json` + `package-lock.json` 三处已同步）
**工单性质：** 纯审查工单，5 个条目（P1-1 / P1-2 / P2-1 / P2-2 / P3-1 + P3-2 用户操作）。本轮**全部按工单的「修复方向（默认动作）」执行，没有超范围改动**。

## 0. 三句话结论

1. **接受工单的核心判断**：`teamPosition` / `individualPosition` 按 Riot 官方定义都是服务器**推算**的「最可能打的位置」，不记录大厅选位、也不记录补位，所以 R119 那条规则检测不了补位。标签总开关 `autofillLabelGate` 已改为**默认关闭**，界面上一个标签都不会出现。
2. **诊断计数照常运行**：计数改走不受开关影响的纯计算函数 `riotAutofillCandidates`，因此关闭期间仍然能收集真机数据；`autofill_swapped` 已按工单改中性名 `position_mismatch`，前端提示与后端注释里所有「由系统补位」「他自己选到的」措辞已清除，并加了真执行断言与一条 grep 型回归测试防止改回去。
3. **两条护栏补齐**：P2-1（单人 `teamPosition` 为空不得被标）有了专属测试，删掉那条 `continue` 会打红；P2-2 的整场门槛从「任意 1 人有值」收紧到「有效 `individualPosition` 覆盖率 ≥ 90%」，另加候选占比 > 30% 的第二道护栏。

---

## 1. P1-1　总开关，默认关闭

**实现**（`backend/riot_api.go`）：判定拆成两层，这是本条的关键设计——

| 函数 | 是否受开关影响 | 谁在用 |
|---|---|---|
| `riotAutofillCandidates(info) ([]bool, bool)` | **不受影响**，永远按数据形状算 | 只被 `summarizeRiotParticipants` 调用，产出诊断计数 |
| `riotAutofillFlags(info) ([]bool, bool)` | 开关关闭时一律返回全 false、`evidence=false` | 只被 `convertRiotMatchInfo` 调用，决定 JSON 里的 `autofill` |

- 开关本体：`var autofillLabelGate = false`（`backend/riot_api.go`）。**用变量而不是常量**，因为工单的验收判据要求「开关打开时 R119 原有测试全部仍然通过」——常量会让「打开」这条路径变成无法验证的死代码。测试用 `r121OpenGate(t)`（`t.Cleanup` 恢复）翻转它。
- 没有放进 `feature_gates.go`：那套 gate 会被 `DEEP_LEGENDS_FEATURE_GATES_URL` 指向的远端响应改写（`backend/main.go:394`），把一个「未经真机验证的口径」交给远端开关不合适。
- `Autofill` 字段保留 `json:"autofill,omitempty"`，开关关闭时 JSON 里连键都不出现，既有响应逐字节不变。
- 前端 `autofillChip` 未改逻辑，仍然只认布尔 `true`（工单要求「不动」）。

**验收判据对照**

| 工单判据 | 测试 |
|---|---|
| 开关关闭：端到端断言「整场没有任何 `autofill` 键」 | `TestR119ConvertRiotMatchInfoAutofillKeyFollowsGate`（写成「`autofill` 键数量必须与开关一致」的不变式）、`TestR121ConvertRiotMatchInfoTagsCandidateOnlyWhenGateOpens`（前半段） |
| 变异：把开关判断去掉 / 反向 → 必须 FAIL | 去掉 `if !autofillLabelGate` → 上面两个测试打红；把默认值改成 `true` → `TestR121AutofillLabelGateIsClosedByDefault` 打红 |
| 开关打开：R119 原有 6 个 Go 测试 + 3 个 JS 测试仍然通过 | R119 的 6 个测试全部与开关状态无关：3 个逻辑测试改打在 `riotAutofillCandidates` 上；端到端那个改成「键数量随开关」的不变式（不再硬断言开关必须关闭）；诊断计数与 `riotPositionKey` 两个本来就不受开关影响。JS 侧只认后端下发的布尔值，同样无关。`TestR121GateClosedSuppressesFlagsButKeepsCandidateMath` 与 `TestR121ConvertRiotMatchInfoTagsCandidateOnlyWhenGateOpens` 再显式覆盖「打开」路径 |
| 关闭状态下 3 个计数键仍存在 | `TestR121DiagnosticsKeepCountingWhileGateClosed`（显式断言开关为 false 时 `AutofillTagged/PositionMismatch/AutofillNoEvidence` 仍为 1/1/1） |

---

## 2. P1-2　文案与命名不再把推断当事实

| 位置 | 改前 | 改后 |
|---|---|---|
| `backend/web/gameplay.js` `autofillChip` 提示 | 「客户端没有给这名玩家分配个人位置，最终分路 X **由系统补位**」 | 「对局数据里没有这名玩家的个人位置（推算位置：X）」 |
| `backend/riot_api.go` 判定函数注释 | `individualPosition` = 「这名玩家被单独分配到的分路（**他自己选到的**那一路）」 | 明确写成 Riot 官方口径：两个字段都是服务器**推算**的「最可能打的位置」，`individualPosition` 是孤立推算、`teamPosition` 加了每队各一路的约束；并写明「这条规则**并不能检测补位**」与它更可能标出的人群 |
| `backend/gameplay.go` `Autofill` 字段注释 | 「由 teamPosition / individualPosition 的一致性推得」 | 补上「候选」定性、官方定义、以及总开关默认关闭 |
| 诊断键 | `autofill_swapped`（断言「换位」） | `position_mismatch`（中性，只说两个推算值不一致） |
| 结构体字段 | `AutofillSwapped` | `PositionMismatch` |
| `autofill_tagged` | 「判定为补位的人数」 | 保留键名，注释改为「**候选**人数，未经真机验证」 |

**标签文字「补位」本身**：按工单「最终措辞留给用户看到真机结果后再定，本工单期间开关关闭，用户看不到」，本轮**保留原字样**，未改。若要改成更保守的措辞（例如「位置未记录」），只需改 `autofillChip` 一处 + `r119.test.cjs` 的两条断言。

**验收判据对照**

- grep：`由系统补位` / `他自己选到的` / `autofill_swapped` 在 `backend/*.go` 与 `backend/web/gameplay.js` 里**已全部清零**（只剩测试文件里作为「禁用词表」出现）。
- 真执行断言（不是源码正则）：`backend/web/r119.test.cjs` 跑真实 `autofillChip`，断言提示含「个人位置（推算位置：辅助）」且 `doesNotMatch(/由系统补位|他自己选到的/)`。把旧文案改回去 → 该用例打红。
- 额外加了一条 Go 侧护栏 `TestR121NoUnprovenWordingInBusinessCode`：直接读 `riot_api.go` / `gameplay.go` / `web/gameplay.js`，命中禁用词或旧诊断键名即 FAIL。

---

## 3. P2-1　「`teamPosition` 为空则跳过」的护栏已有测试

R119 的对抗变异里这是**唯一存活**的一条（工单 R6）。现在：

- 测试：`TestR121ParticipantWithoutTeamPositionIsNeverTagged`，两个子用例（`individualPosition="Invalid"` 与 `""`），fixture 是 10 人局、其余 9 人证据齐全（覆盖率恰好 90% 达标，因此**不会**被整场降级挡掉——唯一挡住误标的就是那条 `continue`）。
- 断言：`evidence` 为 true、候选数为 0、端到端 `match.Participants[i].Autofill` 全 false、其余 9 人 `Position` 不受影响。
- 对抗变异：删掉 `riotAutofillCandidates` 里的 `if riotLaneKeyValue(raw.TeamPosition) == "" { continue }` → 下标 9 会被标为候选，`r119CountFlags` 变 1 → **本测试 FAIL**。

---

## 4. P2-2　整场证据门槛收紧

工单实测：只有 1 人带 `individualPosition` 时，`evidence=true` 且其余 9 人全被误标。原因是旧护栏只要求「任意 1 人有值」。

**新口径**（阈值写在 `backend/riot_api.go` 的具名常量里，不进魔法数字）：

1. `autofillMinIndividualCoveragePercent = 90`：有效 `individualPosition` 覆盖率 < 90% → 整场降级。10 人局即「最多允许 1 人缺失」。
2. `autofillMaxFlaggedPercent = 30`：候选占比 > 30% → 整场降级。
   **诚实说明**：在当前 90% 覆盖率护栏下，候选最多占 10%，这条护栏在 `riotAutofillCandidates` 的正常路径上**轮不到触发**。保留它是防止将来放宽覆盖率时立刻出现大面积误标；因此它由 `TestR121CoverageAndRatioThresholds` 直接单测，而不是伪装成端到端覆盖。
3. 原有的「整场没有 `teamPosition` 就降级」保留（`teamKnown == 0`）。

**验收判据对照**（`TestR121WholeGameDegradesWhenCoverageIsLow`，5 个子用例）

| 输入（10 人局） | `evidence` | 候选数 |
|---|---|---|
| 只有 1 人有个人位置、9 人缺失 | false | 0 |
| 5 人缺失、5 人正常 | false | 0 |
| 2 人缺失（80%） | false | 0 |
| 1 人缺失（90%，恰好达标） | true | 1 |
| 全员齐全 | true | 0 |

对抗变异：把覆盖率判断改回「任意 1 人有值」（`autofillCoverageInsufficient` 恒返回 false）→ 前三行用例全部 FAIL。

---

## 5. P3-1　LCU 契约探测（只读，不进生产路径）

交付了**只读探测代码**，但**结论表按工单要求留空**——本会话连不上 League 客户端、也没有真机，任何字段名都不能凭空写进结论。

- `backend/position_contract_probe.go`：独立路径谓词 `(?i)lobby|champ-select|gameflow|end-of-game|match-history`（不复用、不修改 `augmentProbePath` / `objectiveDiagnosticPath`）；三级契约来源 `/swagger/v3/openapi.json` → `/swagger/v2/swagger.json` → `/help?format=Full`（兜底依据是 R82 真机观测：swagger 两个来源双双 404、`/help?format=Full` 返回 200/3 MB）。
- **关键新增能力是 `$ref` 展开**（augment 探针没有）：要回答「`/lol-lobby/v2/lobby` 的成员对象里有没有偏好字段」，必须把响应里的 `$ref` 解析到 `components.schemas`（v2 是 `definitions`）并列出属性名与类型。实现带 schema 名去重（循环引用 A→B→A 自然收敛）、递归深度 6、属性总数 400、operation 明细 200 的硬上限。
- 属性关键词 `(?i)position|preference|pref|fill|autofill|role|lane|assigned`，命中才记录**名字与类型**；示例值、默认值、响应体取值一律不进日志，事件恒带 `write_executed=false` 与 `temporary_reconnaissance_code=true`。
- **判据护栏（本轮额外收紧的一处）**：`contract_read_any` / `contract_read_structured` / `negative_conclusive` / `negative_conclusive_all`。只有真的解析到 openapi **结构化契约**时，零命中才算否定证据；`/help` 的纯文本路径扫描只能证明「有哪些端点」，所以那一支的 `negative_conclusive` 恒为 false，`negative_conclusive_all` 也改为要求 `contract_read_structured`。这是为了防止把「404 / 只读到 help」误写成「客户端确实没有补位字段」——正是 R119 犯过的那类错。
- `backend/position_contract_probe_test.go`：两个**不依赖环境变量**的纯函数单测（合成 openapi v3 fixture：断言命中 2 个端点、`LobbyMemberDto` 恰好命中 `firstPreference`/`secondPreference`、`ChampSelectMemberDto` 命中 `assignedPosition`、无关端点不出现、fixture 里的示例值绝不外泄；以及 `$ref` 循环引用不炸），加一个 `R121_POSITION_PROBE=1` 门控的真机入口（含 lockfile 兜底与 `R121_POSITION_PROBE_OUTPUT` 导出）。**fixture 里的字段名是合成示例，不是真机证据**，文件头已注明。
- `docs/r121-position-probe-findings.md`：结论表骨架（5 个接口 × 5 列，全部「待真机运行填写」）、Git Bash / PowerShell 两套运行命令、四条判读规则、回填步骤、以及「结论回填后整体删除探测代码」的处置约定。
- **不新增任何生产调用方**：`collectPositionContractProbe` 只被那个默认 Skip 的测试调用；`gameplay.go` / `main.go` / `watch_rules.go` 一行未改。
- 工单验收判据「结论表写入台账，每一行注明证据来源」→ **本轮无法闭合**，它需要真机。运行方式见 `docs/r121-position-probe-findings.md` §2。若探测确认存在真实字段，按工单另起「对局进行时记录己方补位信息」工单，届时涉及 `features.go` 的 `stores` 隐私声明，**文案由用户拍板**。

---

## 6. P3-2　真机对照（需要用户操作）

步骤与回填表格已写进 `docs/r119-execution-ledger.md` §5「观测栏」。**在回填之前 `autofillLabelGate` 保持 `false`**（`TestR121AutofillLabelGateIsClosedByDefault` 会守住这一点）。

---

## 7. 改动清单

| 文件 | 改动 |
|---|---|
| `backend/riot_api.go` | 判定拆成 `riotAutofillCandidates`（纯计算）+ `riotAutofillFlags`（受 `autofillLabelGate` 门控）；新增 `autofillLabelGate`、`autofillMinIndividualCoveragePercent`、`autofillMaxFlaggedPercent`、`autofillCoverageInsufficient`、`autofillFlagRatioExcessive`；注释按 Riot 官方字段定义重写 |
| `backend/gameplay.go` | `summarizeRiotParticipants` 改调 `riotAutofillCandidates`；`AutofillSwapped` → `PositionMismatch`；两处诊断键 `autofill_swapped` → `position_mismatch`；`Autofill` 字段注释补「候选 + 默认关闭」 |
| `backend/web/gameplay.js` | `autofillChip` 提示改为只陈述数据事实；注释同步 |
| `backend/r119_autofill_test.go` | fixture 换成 10 人局（`r119ClassicInfo`，默认全场证据齐全、零候选）；6 个测试改打在 `riotAutofillCandidates` 上；端到端测试改为「开关关闭 → 无 `autofill` 键」 |
| `backend/r121_autofill_test.go`（新增） | 7 个测试：默认关闭、开关双向、端到端开/关、P2-1 护栏、P2-2 覆盖率、阈值单测、诊断计数不受开关影响、禁用词回归 |
| `backend/web/r119.test.cjs` | 提示文案断言更新 + 新增禁用词 `doesNotMatch` |
| `docs/r119-execution-ledger.md` | 顶部加作废横幅；§2 口径标记推翻并写现行两层实现；§5 改名 + 新增 P3-2 观测栏 |
| `desktop/package.json`、`desktop/package-lock.json` | 0.12.13 → **0.12.14**（3 处） |
| `backend/position_contract_probe.go`（新增，528 行） | P3-1 一次性只读探测：LCU openapi 契约里到底有没有位置偏好/分配位置/补位字段。含 `$ref` 展开、循环引用去重、深度/属性/operation 三级上限，只记形状不记取值。**无生产调用方**，结论回填后整体删除 |
| `backend/position_contract_probe_test.go`（新增，196 行） | 2 个不依赖环境变量的纯函数单测（合成 openapi v3 fixture + `$ref` 循环引用）+ 1 个 `R121_POSITION_PROBE=1` 门控的真机入口（默认 Skip） |
| `docs/r121-position-probe-findings.md`（新增，64 行） | 结论表骨架（全部「待真机运行填写」）、Git Bash / PowerShell 运行命令、四条判读规则、回填步骤、探测代码处置约定 |

**未改动**：`backend/feature_gates.go`（理由见 §1）、前端标签文字、`demo-data.js`、实时对局视图、职业页与账号归属。

---

## 8. 验证状态（本会话的硬边界）

- **Bash 工具在本会话被拒（`TOOL_DENIED`），`go build` / `go vet` / `go test` / `gofmt` / `node --test` 一次都没跑过。** 所有代码靠静态审查 + 一个只读审查子代理复核，未经编译器验证。交付前必须实跑：

```bash
gofmt -l backend/*.go
go build ./backend && go vet ./backend && go test -race ./backend
node --test backend/web/r119.test.cjs
node --test backend/web/*.test.cjs
```

- **`gofmt` 遗留**：`docs/r119-independent-verification.md:25` 记录 `gofmt -l backend/*.go` 会列出 `backend/gameplay.go`（归因于 R119 的 `Autofill` 字段未对齐），工单说明该项已记入 R120 P1-1、本轮不重复。本轮已把该字段写成 `Autofill  bool`（两空格，与同文件 `backend/gameplay.go:266-268` 的 `PrivateHistory/IsCurrent/reference` 对齐先例一致），但**无法在本会话用 gofmt 证实**，请以 R120 的处置为准。
- 对抗变异**未实跑**（同样因为 Bash 被拒）。§1/§3/§4 里列的每条变异都是「预期会打红」的推演，需要执行方在隔离副本里实跑确认（工单执行纪律要求：隔离副本 + 整文件拷回 + `diff -q`，禁止 `git checkout -- <file>`）。

---

## 9. 遗留

1. **P3-2 未做**（需要用户的真机与真实对局）。开关保持关闭直到观测栏回填。
2. **标签文字「补位」仍是对玩家的事实陈述**，工单允许本轮先保守处理；真机结果出来后由用户拍板最终措辞。
3. 若 P3-1 探测发现 LCU 真有位置偏好/补位字段，需要另起工单设计「对局进行时记录己方补位信息」；那会涉及 `features.go` 的 `stores` 隐私声明，**文案必须由用户拍板，执行方不得代写**（工单执行纪律第 3 条）。
