# R134 执行台账：stall 日志跨启动污染

基线版本：0.12.18（R133 之后）。工单：`docs/WORKLIST-R134-STALL-LOG-REPORT-CROSS-RUN-CONTAMINATION.md`。

## 改动

- `scripts/r133-stall-log-report.cjs`：图片失败按 `run_id` 分组。每条 stall 只计同一 `run_id`、同一时间窗的失败；stall 没有 `run_id` 时沿用原有全局窗口。全文件图片失败总数仍保持原口径。
- 同一文件出现多个 `run_id` 时，文本摘要提示「本文件跨 N 次启动」。JSON 摘要里的每条 stall 保留其 `runId`，方便复核。
- `scripts/r133-stall-log-report.test.cjs`：新增跨启动污染、同启动真实积压及缺失 `run_id` 的回归用例。

## 验证

| 验证 | 结果 |
|---|---|
| 修复前运行 `node --test --test-name-pattern='R134' scripts/r133-stall-log-report.test.cjs` | 跨启动用例按预期失败：runA 的窗口错误计入 runB 的 4 条失败（预期 0，实际 4）。移除分组逻辑会重新触发该失败。 |
| 修复后运行 `node --test scripts/r133-stall-log-report.test.cjs` | 16/16 通过；原有 14 条全部保留。 |
| 临时变异体：把窗口内的 `failures` 改回全局 `datedFailures`，对同一跨启动夹具判读 | 正式实现判 `notify-gap`；变异体判 `backlog-only`，确认分组逻辑是防止误判的必要条件。变异体仅写在临时目录，已删除。 |
| `node scripts/r133-stall-log-report.cjs docs/r89-kr-player/diagnostic-samples.jsonl` | 仍判 `clean-unverified`，退出码 1。该旧日志含 5 个 `run_id`，所以摘要新增跨 5 次启动提示。 |
| `node --check scripts/r133-stall-log-report.cjs` | 通过。 |
| `go build -o /tmp/deep-legends-r134-backend ./backend` | 通过。 |
| `desktop/package.json` 版本 | 0.12.18。 |

本轮只改 `scripts/` 下的只读诊断工具及测试，不改变安装包内的生产代码；版本号保持 0.12.18。R133 P2 的 Windows 国服客户端 5 分钟真机验证仍未执行，本轮合成日志测试不充当那项验证。
