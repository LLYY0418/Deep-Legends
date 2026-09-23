# R118 执行台账：R117 独立验收遗留缺口

**工单：** `docs/WORKLIST-R118-INDEPENDENT-VERIFICATION-GAPS.md`（基线版本 0.12.12）
**执行日期：** 2026-09-21
**范围：** P1-2 job 级 CI 护栏、P2-8 race 测试闭合；P3-9 按工单的可选建议不扩扫描范围。

## 执行结果

### P1-2：job 级 CI 护栏

仅修改 `backend/web/r117.test.cjs`。YAML 结构化解析现在读取 `workflow.jobs.quality`，分别断言 job 级 `continue-on-error` 与 `if` 均未设置；原有 step 级、浏览器命令白名单及软化检查保持不变。

在 `$PI_SCRATCH_DIR/r118-ci-mutations` 隔离副本中完成变异，未改工作区 CI YAML。未变异控制用例 PASS；以下变异各自使 `R117 real Chromium guards are wired into CI and cannot silently skip` FAIL：

| 变异 | 结果 |
|---|---|
| `jobs.quality.continue-on-error: true` | **FAIL**，job 属性断言命中 |
| `jobs.quality.if: github.event_name == 'workflow_dispatch'` | **FAIL**，job 属性断言命中 |
| 浏览器命令后缀 `|| echo "skipped"` | **FAIL**，整行白名单命中 |
| 浏览器命令后缀 `|| :` | **FAIL**，整行白名单命中 |
| 浏览器命令后缀 `; true` | **FAIL**，整行白名单命中 |
| run 块前置 `set +e` | **FAIL**，run 块白名单命中 |

每次变异都在 scratch 副本反向定点还原；每次还原后与 `ci.baseline.yml` 的 `diff -u` 均为空。变异控制路径确认测试读取的是同一 scratch 根下的工作流文件。

### P2-8：诊断回调 race

仅修改 `backend/hexdata_r116f_test.go`：给 `provider.diag` 的 `events` 切片追加加 `sync.Mutex` 保护；`loadMayhemDetail` 返回后等待 `provider.hexdata.pruneWait`，再读取诊断事件，确保异步剪枝 goroutine 已汇合。生产代码未改。

| 验证 | 结果 |
|---|---|
| 定向 `go test -race -count=1 -run '^TestLoadMayhemDetailCarriesHeroStageRarityAndCautionFlags$' ./backend` 连跑 5 次 | **5/5 PASS**，无 `DATA RACE` |
| 全量 `go test -race -count=1 ./backend`，不切块 | **PASS**，267.426 秒 |
| `go vet ./backend` | **PASS** |
| 最终 `var eventsMu sync.Mutex` 版本定向 race 复跑 | **PASS**，2.756 秒 |

在 `$PI_SCRATCH_DIR/r118-go-mutation` 隔离副本中移除回调锁、保留 `pruneWait.Wait()`，定向 race 测试重新 **FAIL** 并报告 shape 诊断与异步剪枝诊断之间的 `DATA RACE`。变异 diff 只包含锁移除，等待行保持不变；反向定点恢复后与 `hexdata_r116f_test.baseline.v2.go` 的 `diff -u` 为空，恢复后的隔离控制测试 PASS。

## 其它检查

- `gofmt -l backend/*.go`：无输出。
- `go build -o "$PI_SCRATCH_DIR/r118-backend" ./backend`：**PASS**，产物仅写入 scratch。
- `node --test --test-name-pattern="R117 real Chromium guards are wired into CI" backend/web/r117.test.cjs`：**PASS**。
- 完整 Node 套件：845 项，843 通过、1 失败、1 skipped。唯一失败被报告为 `desktop/overview-render.test.cjs` 文件级 worker failure，输出没有具体断言；该文件不在 R118 改动范围。R59 UUID 渲染目标用例单独运行 **PASS**；整文件单独复跑超过 4 分钟仍未结束，已停止，未改该文件。
- P3-9：按工单明确的低优先级可选说明，当前 `desktop/*.js` 没有对应 hex 字面量，且工单不建议为此单独扩大扫描；本轮保持未实施。
- `desktop/package.json` 当前版本为 **0.12.12**。

## 最终状态

P1-2 与 P2-8 的修复及对抗变异验收完成。Go race、vet、构建、格式和 R117 CI 聚焦测试通过。完整 Node 套件仍有一项 `desktop/overview-render.test.cjs` 文件级失败，原因未能从运行器输出定位；此项作为剩余验证限制记录，不扩范围修改。
