# R119 执行台账：对局详情给队友/对手打「补位」标签

**需求来源：** 用户口头需求（本轮没有 WORKLIST 文件）。原话：「我想在对局中召唤师峡谷模式的单双排和灵活组排的详情中，队友和对手加一个是否是补位的标签，客户端应该有这个数据」；补充规则：「如果没有是否是补位的字段，可以这样计算，玩家的最终位置不在他选择了两个位置中，并且没有选择任意位置就是补位」。
**基线版本：** 0.12.12　**最终版本：** **0.12.13**（`desktop/package.json` + `package-lock.json` 三处已同步）
**范围：** 只做「对局」页（`data-section="live"`，`backend/web/index.html:46`）里**历史对局详情 → 概览 tab** 的队友/对手行；只在召唤师峡谷**单双排（420）与灵活组排（440）**生效。职业页、账号归属、其它模式、实时对局视图**一律未动**。

> **⚠️ 本文件是 R119 的历史台账。R121（`docs/WORKLIST-R119-AUTOFILL-LABEL-GAPS.md`）已推翻 §2 的判定口径、把标签总开关 `autofillLabelGate` 改为默认关闭、并把诊断键 `autofill_swapped` 改名为 `position_mismatch`。当前状态以 `docs/r121-execution-ledger.md` 为准；§5 的观测栏是 P3-2 真机结果的回填处。**

---

## 1. 事实核查：客户端到底有没有「补位」字段

**结论：没有。用户设想的「两个位置偏好」在历史对局数据里也拿不到。** 以下都是仓库内可复核的证据，不是推测：

| 数据源 | 位置字段 | 证据 | 能否判定补位 |
|---|---|---|---|
| Riot/SGP match-v5（战绩主路径） | `teamPosition` + `individualPosition` | `backend/riot_api.go:610-611`；主路径入口 `backend/gameplay.go:2134 loadSGPMatchHistoryPage` | **能**（每人一个「个人位置」，见 §2） |
| LCU `/lol-match-history/v1/products/lol/{player}/matches`（回退路径） | 只有 `timeline.lane` / `timeline.role` | `backend/gameplay.go:579-634`（`lcuParticipant`），映射点 `backend/gameplay.go:3449` | **不能** → 降级不打标签 |
| LCU `/lol-lobby/v2/lobby` | 只解析了 `gameConfig`，**没有 members** | `backend/gameplay.go:7030-7036`（`lcuLobby`） | 不能。全仓库搜 `firstPreference`/`secondPreference`/`positionPreference`/`autofill`/`补位` 在业务代码里**零命中** |
| LCU `/lol-gameflow/v1/session`（实时） | `selectedPosition` / `selectedRole`（每人**一个**位置，非偏好列表） | `backend/gameplay.go:6972-6992`（`lcuLivePlayer`） | 不能区分「他选的位置」与「最终位置」，因为只有一个值 |
| LCU `/lol-champ-select/v1/session`（实时） | `assignedPosition`，且**只有本人队伍** | `backend/gameplay.go:7045`（`lcuChampSelectPlayer`） | 不能覆盖对手 |

另有一条前轮已验证的事实可复用：海克斯大乱斗场次**没有** `teamPosition`/`individualPosition`（`docs/r116e-execution-ledger.md:318`，§5.4 实测结论），说明这两个字段是「召唤师峡谷有分路的模式」才下发的 —— 正好是本轮需要的范围。

---

## 2. 判定口径（**R121 已推翻，此节仅作历史记录**）

> **⚠️ 作废原因**：Riot 官方对 `teamPosition` / `individualPosition` 的定义是「游戏服务器按对局中的实际表现**推算**出来的『这名玩家最可能打的位置』」——`individualPosition` 是孤立地看这名玩家时的最佳猜测，`teamPosition` 是再加上「每队各有一个上单、一个打野、一个中单……」约束后的最佳猜测。**两者都不记录大厅选位，也不记录是否被补位**，所以下面这条规则检测不了补位；它标出来的更可能是「行为上认不出位置」的人（提前退出、挂机、极端打法）或撞上 Riot 已知数据缺失（developer-relations issue #554）。现行实现只把它当**候选**，且总开关默认关闭。详见 `docs/r121-execution-ledger.md`。

R119 当时的口径（**已作废**）：用户给的规则需要**两个偏好位置** + **是否选了任意位置**；接口每人只给一个个人位置，也没有任何「选了任意位置」的标记，因此当时收敛为：

> ~~在 420/440 里，`teamPosition`（队伍最终分路）能规范化成有效分路，而 `individualPosition`（这名玩家被单独分配到的分路）拿不到（空串或 `Invalid`）→ 这一路不是他自己选到的，是系统补的 → **补位**。~~

现行（R121）实现分两层，都在 `backend/riot_api.go`：

- `riotAutofillCandidates`：纯计算，**不受总开关影响**，只供诊断计数使用（开关关闭期间也要收数据）；
- `riotAutofillFlags`：对外口径，`autofillLabelGate` 关闭时一律返回全 false、`evidence=false`，JSON 里不出现 `autofill` 键。

不出标签的情况（R121 仍然成立，第 2 条已收紧）：

1. **两个推算位置都有值但不一致** → 计入诊断 `position_mismatch`，不打标签。R119 曾把它断言成「英雄选择阶段换过位置」，该断言同样没有依据，已改为中性名。
2. **整场证据不足** → 覆盖率护栏：有效 `individualPosition` 必须 ≥ 90%（10 人局最多缺 1 人），否则整场降级、计入 `autofill_no_evidence`。R119 原来只要求「任意 1 人有值」，实测会导致一场里 9 人被误标（R121 P2-2）。另有候选占比 > 30% 的第二道护栏。
3. **整场没有任何 `teamPosition`**（海克斯大乱斗的形状）、**单个玩家 `teamPosition` 为空**（issue #554 的形状，R121 P2-1 补了测试）、非 420/440 队列、LCU 回退路径 → 一律 false。

---

## 3. 改动清单

| 文件 | 改动 |
|---|---|
| `backend/riot_api.go` | 拆出 `riotLaneKeyValue`（单值规范化，无效→空串）；`riotPositionKey` 改为基于它重写（行为等价，见测试 `TestR119RiotPositionKeyBehaviourIsUnchangedByLaneHelper`）；新增 `riotAutofillFlags`；`convertRiotMatchInfo` 循环改带下标并写入 `Autofill` |
| `backend/gameplay.go` | `gameplayParticipant` 新增 `Autofill bool json:"autofill,omitempty"`；`participantCompletenessSummary` 新增 3 个证据计数；`summarizeRiotParticipants` 增加只统计 420/440 的计数；`sgp_match_history_*` 与 `sgp_summary_history_*` 两个诊断事件各加 3 个键；LCU 路径补降级注释 |
| `backend/web/gameplay.js` | 新增 `autofillChip(player)`（只认布尔 `true`）；`matchTableRows` 玩家单元格在 `participant-name` 之后、`</button>` 之前插入标签 |
| `backend/web/gameplay.css` | 新增 `.match-autofill-chip`，复用 `.match-rank-chip` 的尺寸族，颜色走 `var(--warning)` + `color-mix`，不写死十六进制 |
| `backend/r119_autofill_test.go` | 新增 5 个测试函数（判定、整场护栏、队列门禁、端到端 + JSON `omitempty`、诊断计数、`riotPositionKey` 等价性） |
| `backend/web/r119.test.cjs` | 新增 3 个用例，**真执行** `autofillChip` 与 `matchTableRows`（vm + 桩），不做源码正则护栏（R118 的教训） |
| `desktop/package.json`、`desktop/package-lock.json` | 0.12.12 → 0.12.13（3 处） |

**刻意没做**（避免超范围）：实时对局视图不加标签（§1 表里已说明证据不足）；`demo-data.js` 不造假补位样本；队伍分析 tab、列表页的 10 人小头像不加标签（需求说的是「详情中的队友和对手」）。

**兼容性**：`autofill` 带 `omitempty`，未补位的参与者 JSON 里**不出现该键**，既有响应逐字节不变（测试已断言 `"autofill":false` 不得出现）。

---

## 4. 验证状态（本会话的硬边界，不能含糊）

- **本会话 Bash 工具被拒（`TOOL_DENIED`），因此 `go build ./backend`、`go vet ./...`、`go test -race ./backend`、`node --test backend/web/*.test.cjs` 一次都没有跑过。** 代码是按静态审查写的，未经编译器验证。
- 已做的替代保障：新增/修改的结构体字段与字面量对齐按 gofmt 的分组规则手工核对（注释行会断开对齐组，因此不会引发大面积重排）；另派了一个只读审查子代理专门复核编译性与测试可执行性，结论见 §6。
- **交付前用户需要执行**（也是本项目的既有纪律「改动后必须重新构建并确认版本号最新」）：

```bash
go build ./backend && go vet ./backend && go test -race ./backend
node --test backend/web/r119.test.cjs
node --test backend/web/*.test.cjs
```

版本号已确认为 **0.12.13**（`desktop/package.json:3`）。打包仍由用户自己做。

---

## 5. 真机验证步骤与观测栏（P3-2 的结果回填到这里）

> **R121 变更**：诊断键 `autofill_swapped` 已改名为 `position_mismatch`（中性口径，不再断言成因）；标签总开关 `autofillLabelGate`（`backend/riot_api.go`）**默认关闭**，因此界面上看不到标签，但下面三个计数照常产出——这正是收集真机数据的手段。

### 5.0 操作细节（R122 补：日志在哪、怎么导、怎么开开关）

**诊断日志文件**：`<用户缓存目录>/LOLLootAssistant/logs/diagnostics.jsonl`，滚动归档还有 `diagnostics.1.jsonl` … `diagnostics.4.jsonl`（`backend/storage.go:660`、`872`）。目录可用环境变量 `LOL_LOOT_DATA_DIR` 覆盖（`backend/storage.go:109`）。

- Windows（真机）：`%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`
- macOS：`~/Library/Caches/LOLLootAssistant/logs/diagnostics.jsonl`

**不用找文件也能导**：应用内 **设置 → 隐私 → 「诊断与日志」→「导出诊断日志」**（`backend/web/index.html:321`），导出的文件带时间戳、不会互相覆盖，内容是最近 5 个归档合并后的全量（`readDiagnosticLogForExport`）。同一面板还有「复制诊断摘要」。

**只想看这三个计数**，导出后在 PowerShell 里：

```powershell
Select-String -Path "$env:LOCALAPPDATA\LOLLootAssistant\logs\diagnostics*.jsonl" -Pattern 'autofill_candidates'
```

**触发事件的动作**：打开自己的**总览**页（或任意国服玩家的生涯页）即会加载战绩页，从而写出 `sgp_match_history_succeeded`；当详情页可见对局为 0 场时还会写 `sgp_summary_history_succeeded`。每次翻页/刷新都会再写一条，所以看**最新那条**即可。

**要看到标签本身**（可选，第二步再做）：把 `backend/riot_api.go` 的 `var autofillLabelGate = false` 改成 `true`，重新构建并打包。注意 `TestR121AutofillLabelGateIsClosedByDefault` 会因此打红，**这是预期**，对照完必须改回 `false` 再提交。

1. 用真实客户端打若干局**单双排或灵活组排**，尽量包含你明确知道谁被补位的对局（自己被补位，或队友在语音里说了）。
2. 看诊断日志里 `sgp_match_history_succeeded`（或 `sgp_summary_history_*`）事件的三个键：
   - `autofill_candidates`：**候选**人数（未经真机验证，不等于真实补位。R122 之前叫 `autofill_tagged`，因开关关闭时一个标签都没打、名字是假陈述而改中性名）；
   - `position_mismatch`：两个推算位置都有值但不一致的人数；
   - `autofill_no_evidence`：整场缺少可用位置证据、因而整场降级的场次数。
3. 判读：
   - `autofill_no_evidence` 等于场次数 → 该地区/版本的 match-v5 根本不下发 `individualPosition`，本功能无法在历史对局成立，转 P3-1 的 LCU 契约探测结论（`docs/r121-position-probe-findings.md`）；
   - `autofill_candidates` 相对总人数明显偏离常识（例如 10 人里 3 个以上）→ 口径不成立，开关保持关闭；
   - 想直接看标签：把 `backend/riot_api.go` 的 `autofillLabelGate` 临时改成 `true` 重新构建（`TestR121AutofillLabelGateIsClosedByDefault` 会打红，属预期），对照完**必须改回 `false`**。
4. 实时对局侧的形状证据本来就有：`live_position_shape` 事件里的 `selected_position_counts` / `assigned_position_counts`（`backend/gameplay.go:5244` 起）。若里面出现 `INVALID` 计数，说明实时侧也能用同一口径判定，可作为下一步。

### 观测栏（P3-2 回填；未回填之前 `autofillLabelGate` 保持 false）

| 日期 | 队列 | 局数 | `autofill_candidates` | `position_mismatch` | `autofill_no_evidence` | 已知真实补位的人 | 标签是否落在正确的人身上 | 结论 |
|---|---|---|---|---|---|---|---|---|
| _待填_ | | | | | | | | |

---

## 6. 已知风险与未决问题

1. **`individualPosition` 语义未经真机验证**（§2）—— 唯一实质风险，已给出一步定位方法与一行修正方案。
2. **换位与补位在「两值不一致」形状下不可区分**。当前口径把「不一致」判为换位（不打标签），偏保守：宁可漏标，不可错标。
3. **LCU 回退路径（国服客户端战绩）没有标签**。这是刻意降级，不是缺陷；该路径参与者只有 `timeline.lane/role`。
4. **实时对局视图未覆盖**，理由同 §1 表格。
5. 标签放在 `.participant-link` 按钮**内部**：该按钮在详情表里是 `width:100%` 的 flex 容器，放外面会被挤到第二行。CSS 已给 `flex: none` + `white-space: nowrap`，名字仍按既有 `min-width:0` 规则省略号截断。**未做真机视觉确认**（本会话无法启动应用与浏览器预览）。

---

## 7. 独立对抗复核与修复（本轮实际抓到 1 个 P0 回归）

改动完成后派了一个**只读**审查子代理按六项复核（Go 编译性、gofmt 稳定性、前端测试可执行性、断言成立性、既有回归面、逻辑正确性）。它抓到两处真问题、一处口径不准，均已修复：

| # | 审查发现 | 严重度 | 修复 |
|---|---|---|---|
| 1 | `riotPositionKey` 被重写成「teamPosition **规范化后**无效就回退 individualPosition」，与改动前**不等价**：改前只在 teamPosition **原始值**为空时回退。输入 `teamPosition="Invalid"` + `individualPosition="BOTTOM"`，改前得 `""`、改后得 `"bottom"`，会直接污染 `gameplayParticipant.Position`（详情表分路、能力雷达取样、位置胜率都吃这个值）。改动前的原文可在 `docs/r100-validation/before/riot_api.go.txt:894-912` 复核 | **P0（真回归）** | `backend/riot_api.go:944-953` 恢复原始回退条件（只对原始值判空），switch 部分委托给 `riotLaneKeyValue`；`backend/r119_autofill_test.go:187-206` 把该用例期望值从 `"bottom"` 改成 `""`，并补 `{"   ","MIDDLE","middle"}`、`{" middle ","","middle"}`、`{"","Invalid",""}` 三条边界 |
| 2 | `gameplayParticipant` 里 `Autofill` 与紧随其后的 `reference` 属同一 gofmt 对齐组，`Autofill bool` 少一个空格 | P2 | `backend/gameplay.go:502` 改为 `Autofill  bool`（对齐组宽度由 `reference` 的 9 字符决定，与 `backend/gameplay.go:266-268` 既有先例一致） |
| 3 | `AutofillNoEvidence` 的注释写成「没有 individualPosition」，但实现里 teamPosition 整场缺失也会计数 | P3 | `backend/gameplay.go:199-204` 注释改为「individualPosition 或 teamPosition 任一整场拿不到」 |

审查确认**无问题**的项（可复核）：

- Go 侧无未使用变量、无标识符冲突（`riotLaneKeyValue` 全仓库唯一）；`summarizeRiotParticipants`（`backend/gameplay.go:2047`）里既有的内层 `participant` 循环作用域在 2073 行前已结束，与新增块（2077 起）不冲突。
- 前端测试可执行：`extract()` 能正确切出 `autofillChip`（`backend/web/gameplay.js:3043-3046`）与 `matchTableRows`（`backend/web/gameplay.js:3437-3443`；内部四空格缩进的 `}).join("")` 不会被 `\n  }` 误判为函数结束），且 `matchTableRows` 引用的 9 个外部标识符全部有桩，不会 ReferenceError。
- 既有回归面干净：`backend/gameplay_test.go:742-751` 只比较 summary 的既有字段（未做整体相等）；`backend/gameplay_r116e_test.go:161-165` 的逐字节 JSON 比较因 `omitempty` 不受影响；`backend/web/champions.test.cjs:3357-3365` 的 `data-tooltip-overflow=".participant-name"` 与「禁止任何 `title=` 属性」两条断言均仍成立（新标签只用 `data-tooltip`）。
- 诊断 map 只新增键、没有测试对键集合做精确断言。

**审查同样无法执行编译器**（本会话 Bash 被拒），因此 §4 的构建与测试命令仍必须由用户实跑一次；这也是本轮唯一未闭合的验证项。
