# WORKLIST-R119：补位诊断计数「进不进事件」没有测试

**撰写日期：** 2026-09-22　**基线版本：** 0.12.14
**触发：** 对 R121 台账（补位标签修复，工单出处 `WORKLIST-R119-AUTOFILL-LABEL-GAPS`）的独立复测。
**本轮性质：** 纯审查，未改动仓库任何文件。上一张 R119 工单的 P1-1 / P1-2 / P2-1 / P2-2 全部落地正确，下面只剩 1 个测试缺口和 1 个命名收尾。

## 0. 结论

补位标签现在默认关闭（`autofillLabelGate = false`），关闭期间靠三个诊断计数收集真机数据——这是上一张工单 P1-1 的核心设计。**但这三个计数「从汇总结构写进诊断事件」这一步没有任何测试**：把事件里的键删掉，全部测试仍然通过。真机数据收不上来的话，P3-2 的真机对照就没有依据，而且不会有人发现。

## 执行纪律（延续 R117–R121）

- 每项验收判据都要做**对抗变异**（改回原样，对应测试必须 FAIL）。
- 变异在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- 不改生产口径、不动阈值、不动 `features.go` 隐私文案。

## 建议执行顺序

P2-1 → P3-1。

---

# P2

## P2-1　三个补位诊断计数没有任何测试覆盖它们真的进了事件

**证据（变异实测）：** 在隔离副本里分别做下面四个变异，`go test -run 'R119|R121|Autofill|Riot|Summar|Gameplay' ./backend` 全部通过：

| 变异 | 位置 | 结果 |
|---|---|---|
| 删掉 `diagnostic["position_mismatch"] = participantSummary.PositionMismatch` | `backend/gameplay.go` 约 2279（`sgp_match_history_succeeded / _partial` 事件） | 存活 |
| 删掉 `diagnostic["autofill_no_evidence"] = participantSummary.AutofillNoEvidence` | `backend/gameplay.go` 约 2280（同一事件） | 存活 |
| 删掉 `windowDiagnostic["position_mismatch"] = participantSummary.PositionMismatch` | `backend/gameplay.go` 约 1462（`sgp_summary_history_succeeded / _partial` 事件） | 存活 |
| 删掉 `windowDiagnostic["autofill_tagged"] = participantSummary.AutofillTagged` | `backend/gameplay.go` 约 1461（同上） | 存活 |

再在全部 `*_test.go` 与 `*.cjs` 里搜这三个键名（`autofill_tagged` / `position_mismatch` / `autofill_no_evidence`）：**没有任何断言引用它们**，只有一处注释和一条「不得再出现 `autofill_swapped`」的反向检查。现有测试只覆盖到 `summarizeRiotParticipants` 返回的结构体字段（`AutofillTagged` 等），到事件为止就断了。

**影响：** 上一张工单 P1-1 要求「开关关闭期间诊断计数照常产出，否则永远拿不到真机数据」。汇总函数是对的，但事件拼装这一步现在没有护栏，任何一次重构丢键都不会被发现，真机核验会拿到一份缺字段的诊断导出。

**修复方向（只补测试与一处小重构，不改口径）：**
1. 把三行赋值抽成一个纯函数，例如 `autofillDiagnosticFields(summary participantCompletenessSummary) map[string]any`，两个事件都调用它，避免两处各写一遍再各自漂移。
2. 加测试：
   - 直接测该函数：键集合恰好是 `autofill_tagged / position_mismatch / autofill_no_evidence`，值与汇总结构逐项一致（用一个三个数都不同的 fixture，例如 1 / 2 / 3，避免「全 0 也相等」）。
   - 端到端：沿用 `gameplay_test.go:1647` 附近读取诊断日志的既有做法，让一场含「1 个候选 + 1 个位置不一致 + 整场降级」的对局走完 SGP 战绩路径，在诊断日志里断言 `sgp_match_history_succeeded`（或 `_partial`）以及 `sgp_summary_history_succeeded` 事件都带这三个键，并且**总开关保持关闭**。若其中某个事件的构造路径太重，至少用第一条覆盖，并在台账里写明哪个事件只有单元级覆盖。

**验收判据：**
- 上表四个变异各自使新增测试 FAIL；
- 再加两个变异也必须 FAIL：① 事件里把 `position_mismatch` 换回 `autofill_swapped`；② 事件里 `autofill_tagged` 的值改成 `participantSummary.AutofillNoEvidence`；
- 台账写明四个变异的命令与结果，`gameplay.go` 除抽函数外不得有其它逻辑改动。

---

# P3

## P3-1　`autofill_tagged` 在开关关闭时统计的是「候选」，名字仍然说「已打标签」

**证据：** `backend/gameplay.go` 结构体注释（约 199–206 行）自己写明 `AutofillTagged` 是**候选**人数、「未经真机验证，不等于真实补位」，而且开关关闭时一个标签都不会打。事件键 `autofill_tagged` 与字段名 `AutofillTagged` 仍叫「已打标签」。上一张工单 P1-2 要的是把「带有未经证实含义的命名」改成中性名字，`autofill_swapped → position_mismatch` 做了，同类的这个漏了。
**修复方向：** 改成中性名字，例如事件键 `autofill_candidates`、字段 `AutofillCandidates`；同步改 P2-1 新增的测试和台账里的引用。这三个键从 0.12.13 才出现，还没有任何真机导出在用，现在改成本最低。
**验收判据：**
- `grep -rn 'autofill_tagged\|AutofillTagged' backend/ desktop/ docs/`（除历史台账）无命中；
- `TestR121NoUnprovenWordingInBusinessCode` 的禁用词表加入 `autofill_tagged`，改回去必须 FAIL。

---

# P3（需要你操作，沿用上一张工单）

## P3-2　真机对照仍未做

补位判定依据 `individualPosition`，口径至今没有真实对局数据佐证。开关保持关闭，打几局单双排 / 灵活组排（最好有你明确知道谁被补位的局）后，导出诊断，看三个计数的整体比例是否合理，再决定是否打开 `autofillLabelGate`。位置探测 `backend/position_contract_probe.go` 同样在等真机运行，结论表 `docs/r121-position-probe-findings.md` 全部是「待真机运行填写」，填完后按该文档 §4 立即删除探测文件。

---

## 已复核无需返工（不要重复排查）

- 总开关默认关闭；关闭时 JSON 无 `autofill` 键，前端一个标签都不渲染；诊断计数走 `riotAutofillCandidates`，不依赖开关。
- Go 变异 22 个：16 个被打红（覆盖率门槛取消 / `<=` / 50% / 反向、占比门槛置空、开关默认改 true、`riotAutofillFlags` 忽略开关、诊断改走带开关的函数、去掉 `teamPosition` 为空的跳过、去掉队列门禁、`mismatch` 判断取反、`teamKnown==0` 护栏、`total<=0` 分支、标记下标错位、队列只认 420 等）；2 个是等价 / 不可达变异（占比护栏调用点被 ≥90% 覆盖率门槛遮住，台账已如实说明；去掉 tagged 之后的 `continue` 不改变结果）；其余 4 个即本工单 P2-1。
- 前端变异 4 个全部被打红：提示措辞改回「由系统补位」、`autofill` 用真值判断、去掉推算位置、标签文字改成「补」。
- 全仓库禁用词检查：`riot_api.go` / `gameplay.go` / `web/gameplay.js` 里已无「由系统补位」「他自己选到的」「autofill_swapped」。
- 位置探测代码：只发 GET，`write_executed` 恒为 `false`，只记录契约形状不记录响应体取值，默认 Skip，仓库内无任何生产调用方；结论文档如实保持「待真机运行填写」，没有预填。
