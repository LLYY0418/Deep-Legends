# WORKLIST-R155：R116 判据一二轮真机探测回填——functions/events 已扫干净，types 组预算仍不足以形式确证

诊断人：Claude（只读：用户上传的第二轮判据一探测文件，未改仓库代码）。
执行人：GPT。
日期：2026-09-25。
基线：R153 已落地的 `backend/augment_contract_probe.go`（按组独立预算 + 跳过 `values`），仓库 HEAD `35788b9f`。
状态：已关闭。第二轮数据已回填，判据一已按实践结论收尾，临时侦察代码已按 P2 删除；验证详见 `docs/history/ledgers/r155-execution-ledger.md`。
触发：用户在 Windows 真机上重跑了 R153 修复后的 `r116-augment-probe.exe`（判据一新探针），发回 `augment-contract-probe.jsonl`。

证据文件：
- `augment-contract-probe.jsonl`：`run_id=ab3ccdfafb1c41a65e88b2eb`，`trace_id=430c7f117d542ba85c652080`，2026-09-25T08:14:38Z。

请把这份文件放进 `docs/r116-validation/`，因为该目录下已有一份同名文件（R153 那一轮的，`trace_id=a0b512f620d71407490f3416`）——**不要覆盖**，按时间顺序改名，例如把旧的保留为 `augment-contract-probe-round1-0925.jsonl`，新的存成 `augment-contract-probe-round2-0925.jsonl`（具体命名 GPT 可自行斟酌，但账本里要写清楚哪个 trace_id 对应哪个文件）。

## 执行纪律

- P1 只回填文档，不新增探测逻辑，也不做对抗变异；P2 在判据收尾后删除临时侦察代码。
- 不把 `negative_conclusive_all=false` 写成形式确证；按 P2 整体删除 `augment_contract_probe.go` 与测试文件。

---

## P1　判据一二轮回填：functions / events 两个真正代表"可调用接口"的分类这次完整扫描且未触顶；types（数据结构定义，非接口）仍触顶

### 数据对比

| 指标 | R153 那一轮（`trace_id=a0b512...`） | 本轮（`trace_id=430c7f1...`） |
|---|---|---|
| events_scanned / length | 749/749 | 718/718 |
| events_limit_reached | 未单列（旧代码是全局共享预算） | **false** |
| events_nodes_visited | 未单列 | 5,752 |
| functions_scanned / length | 1468/1468 | 1468/1468 |
| functions_limit_reached | 未单列 | **false** |
| functions_nodes_visited | 未单列 | 28,724 |
| types_scanned / length | 3578/3578 | 3578/3578 |
| types_limit_reached | 未单列（全局耗尽） | **true**（单独耗尽，卡在预算上限 100,000） |
| count（命中数） | 21 | 21（与上一轮完全相同的 21 条） |
| contract_read | false | false |
| negative_conclusive | false | false |
| scan_limit_reached | true | true（原因从"全局共享预算耗尽"变成"仅 types 一组耗尽"） |

R153 的修复（按组独立预算 + 跳过 `values` 枚举数组）确认生效：`functions`（1468 个真实可调用接口声明）和 `events`（718 个 WebSocket 事件声明）这两组本轮都被**完整扫描**，节点用量远低于 10 万上限（分别只用了 2.9 万、0.6 万），说明这两类目录里如果真的存在海克斯选择接口，这一轮探测**一定能扫到**；实际命中的 21 条和上一轮完全一样，都是已知的 TFT 外观相关项，**没有新增、没有遗漏**。

`types` 组仍然卡在 10 万节点上限——这一组是 LCU 自描述的**数据结构定义**（3578 个类型的字段树），不是可调用接口本身；一个真实的"海克斯选择"接口必然会出现在 `functions` 里（哪怕它的入参/出参类型定义在 `types` 里没扫完，也不影响"有没有这个接口"这件事）。

### 结论（判据一收尾判断）

- **实践结论**：目前没有证据表明 LCU 暴露任何实时海克斯三选一选择/候选接口——`functions`、`events` 两个接口目录已完整扫描，21 条命中均为已知、与本功能无关的 TFT 外观项。P1-1（实时海克斯推荐功能）按此结论应判**不可行**。
- **形式确证**：工单口径要求的 `negative_conclusive_all=true` 本轮仍未达成，因为 `types` 组预算耗尽（`scan_limit_reached=true`）。
- **是否继续抬高预算重跑**：不建议。理由：(1) `types` 只是数据结构声明，不影响"有没有接口"这个结论；(2) `functions_nodes_visited=28,724` 远小于上限，说明真正决定判据一的那组数据早已跑满且有富余；(3) Windows 端重新编译、拷贝、反复试错命令行的成本已经不小。**判据一到此收尾，不再安排新一轮探测**；如果以后 Riot 更新了 LCU 版本，这个结论需要重新核实，但那是未来的事，不在本单范围内。

### 需要 GPT 做的事

1. 把本轮上传的 `augment-contract-probe.jsonl`（`trace_id=430c7f117d542ba85c652080`）归档进 `docs/r116-validation/`，按上面"不要覆盖旧文件"的方式命名；`docs/r116-validation/README.md` 里补一行说明两份文件分别对应哪一轮、哪个 trace_id。
2. 在 `docs/r116-probe-findings.md` §2.4 的回填表下面新增"第二轮回填"小节，把上表的数字和结论原样写进去，尤其是 `functions_limit_reached=false`、`events_limit_reached=false`、`types_limit_reached=true` 这三个新增的分组字段（上一轮文档里没有，因为那时用的是旧版探测代码，是全局共享预算）。
3. 把 §2.4 顶部判据一那一行（`docs/r116-probe-findings.md:18`）的结论文字，从"未判定：help-full 节点预算耗尽，待新探针重跑"改成"未获形式确证，但 functions/events 已扫满、无命中——按不可行处理，不再安排新一轮探测"。
4. 顺手检查 `docs/WORKLIST-INDEX.md` 补一行 R155；`r155-execution-ledger.md` 按惯例记录改了哪些文件、每处引用的原始 trace_id/run_id。
5. P1 回填本身只改 docs；P2 另按下文删除临时探测代码并完成构建与测试。用 `git diff --stat` 核对 R155 的实际改动范围，同时区分工作树中既存的其他改动。

---

## P2　三项判据全部有结论，满足删除条件——清理侦察代码

`backend/augment_contract_probe.go` 文件开头写明："这不是产品功能，必须整体删除...在真机探测执行完毕、结论写入 `docs/r116-probe-findings.md` 之后立即删除"。R153 执行时判据一还没结论，所以当时留着；现在 P1 把判据一收尾，加上早先已经收尾的判据二（可行）、判据三（看不到对方英雄），**三项判据全部有了结论**，删除条件已经满足。

需要 GPT 做的事：

1. 删除 `backend/augment_contract_probe.go` 与 `backend/augment_contract_probe_test.go`（含 R153 里新增的两个对抗变异测试，一并删除，不留半份）。
2. 确认 `backend/gameplay.go` 里 R153 改写过的那段注释（引用判据三结论、`docs/r116-probe-findings.md §4.2` 的那处）不依赖即将删除的探测文件的任何代码符号——预期不需要改动（注释只引用文档路径，不引用代码），但要跑一次 `go build ./backend` 确认没有编译依赖断裂。
3. `dist/probes/` 下的 `r116-augment-probe.exe`、`run-r116.ps1`，以及 `README.txt` 里 R116 相关的段落可以一并清理（三项判据都收尾了，不会再用这个探针）；`r121-position-probe.exe`、`run-r121.ps1` 是另一张工单（R121）的，不要动。
4. `docs/history/worklists/R116-探测-...-工单.md`（原始工单）和 `docs/r116-probe-findings.md`（结论落盘处）保留不删，作为历史记录。
5. `go vet ./...`、`go test ./...`、`node --test backend/web/*.test.cjs` 全绿；把删除清单和跑测试的结果写进 `r155-execution-ledger.md`。

---

## 验收总表

| 项 | 验收标准 |
|---|---|
| P1 | §2.4 新增第二轮数据表格且数字与 `augment-contract-probe.jsonl`（`trace_id=430c7f1...`）逐项一致；顶部判据一结论文字已按第 3 点改写；`r116-validation/` 下两轮探测文件都在且不互相覆盖；`WORKLIST-INDEX.md` 新增 R155 一行 |
| P2 | `augment_contract_probe.go`/`_test.go` 已删除；`dist/probes/` 下 R116 相关探针文件已清理；`go build`/`go vet`/`go test`/Node 测试全绿；`docs/history/worklists/` 与 `docs/r116-probe-findings.md` 未被误删；`git diff --stat` 显示的改动只有"删除探测代码 + docs 回填"这两类，没有碰到其他功能代码 |
