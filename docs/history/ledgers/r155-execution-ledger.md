# R155 执行账本

日期：2026-09-25。基线：R153 已实现的按组独立预算探针；当前 `desktop/package.json` 为 0.12.20。执行范围：第二轮真机证据回填、判据一实践收尾、临时 R116 探针整体清理。工作树开始时已有 R144–R154 等未提交改动，本单未重置或改写它们。

## P1：第二轮原件与判据一

- 首轮原件从 `docs/r116-validation/augment-contract-probe.jsonl` 改名为 `augment-contract-probe-round1-0925.jsonl`，按字节保持 SHA-256 `302f3a70193736629ea7ef51122c50bfacd6fa39f027d60a053afcc732554d10`；`run_id=90412ac90be0d926f03432c5`、`trace_id=a0b512f620d71407490f3416`。
- 第二轮原件从用户文件 `augment-contract-probe.jsonl` 按字节复制为 `docs/r116-validation/augment-contract-probe-round2-0925.jsonl`，SHA-256 `d3583a2cb85c904f0ab701db6b82ef1502aca5689da3400c2e55f6644c4c227f`；`run_id=ab3ccdfafb1c41a65e88b2eb`、`trace_id=430c7f117d542ba85c652080`（第 1–5 行）。两轮映射已写入目录 README。
- 第二轮第 2–3 行：openapi-v3/v2 均 404、`result=unavailable`。第 4 行：help-full 200、`body_bytes=3027989`、`count=21`；`events=718/718`、`nodes_visited=5752`、`limit_reached=false`；`functions=1468/1468`、`nodes_visited=28724`、`limit_reached=false`；`types=3578/3578`、`nodes_visited=100000`、`limit_reached=true`。21 条 `matched_items` 与首轮逐项、逐序相同，4 条 functions 为 TFT 外观/Cherry 库存，17 条 types 为外观/TFT 类型，无相关候选或已选端点。第 5 行 `contract_read_any=false`、`negative_conclusive_all=false`。
- 已在 `docs/r116-probe-findings.md` §2.4 回填来源、分组预算和结论；§0 同步状态。**实践判断：P1-1 按不可行处理，不安排第三轮。形式确证未达成**，因为 types 仍触顶；没有把 `negative_conclusive_all` 改写为 true。判据二仍为可行，判据三仍为选人阶段看不到对方英雄；`skinName` 真机复核仍是独立待办。

## P2：临时侦察代码清理

- 已整体删除 `backend/augment_contract_probe.go`、`backend/augment_contract_probe_test.go`，包括一次性 LCU 探测入口及 R153 的测试；未改 `backend/gameplay.go`。该文件的 R153 阶段注释只引用 `docs/r116-probe-findings.md §4.2`，不依赖已删除的 Go 符号。`backend/gameplay.go` 的海斗正式采样分支仍保留。
- 已删除 `dist/probes/r116-augment-probe.exe`、`dist/probes/run-r116.ps1`，并从 `dist/probes/README.txt` 移除 R116 探针步骤。R121 的 exe、`run-r121.ps1` 与说明保留；R153 的 `skinName` 真机复核提示保留。
- R116 原始工单 `docs/history/worklists/R116-探测-海克斯选择端点与局内数据形状探测-工单.md` 和结论文档 `docs/r116-probe-findings.md` 均保留。后者 §6.A 已标为历史步骤、入口不可再运行；§7/§8 已更新为两轮归档及已删除状态。

## 验证与边界

- R155 没有新增实现逻辑，按更新版工单不做对抗变异。删除后 `go build -o /private/tmp/deep-legends-r155 ./backend` 通过，产物 SHA-256 `f13441b428387f9841a3b4056b735efba231b3794b345aeae0da0950fc197582`；`go vet ./...` 通过；`go test ./... -count=1` 通过（backend 195.142 秒）；`node --test backend/web/*.test.cjs` 707/707 通过。`git diff --check` 通过。
- 两轮文件的 SHA、trace_id、21 条命中逐项相等、第二轮各组计数与状态均由原始 JSONL 程序化核对通过；已检查 R116 Go 源码/exe/脚本均不存在，R121 exe/脚本与 R116 原始工单仍存在。
- 已运行 `git diff --stat`。全局输出含开始本单前已有的 R144–R154 等未提交功能改动，不能把它当作 R155 自身的改动清单；本单实际只删除两份临时 Go 文件与 `dist/probes` 的 R116 文件，修改其 README 和上述 docs。`dist/` 由 `.gitignore` 忽略，清理另用文件清单核验；未动 `backend/gameplay.go` 或其他功能代码。
