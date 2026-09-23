# WORKLIST-R120：`TestR91AddendumWildcardHoverClearKeepsArmedLock` 在负载下随机红（新发现，非本轮改动引入）

**撰写日期：** 2026-09-22　**基线版本：** 0.12.15（工作区未提交状态，仓库 HEAD `4ca768ec`）
**触发：** 对 `WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED.md`（执行台账 `docs/r122-execution-ledger.md`）与
`WORKLIST-R120-TEST-POWER-AND-FLAKES.md`（执行台账 `docs/r120-retest-execution-ledger.md`）两张工单执行完成后的独立复测。
**本轮性质：** 纯审查，未改动仓库任何文件。

## 0. 结论

R119/R122 与 R120 复测两张工单里承诺的每一条都独立复核通过（见 §1），**但在独立全量回归里发现了一个两张工单都没提到、且与本轮改动代码完全无关的新随机红**：`TestR91AddendumWildcardHoverClearKeepsArmedLock`（`backend/r91_addendum_test.go`）在系统负载较高时会失败，机制与 R120 P1-1 修复前「1 秒墙钟预算在整包 `-race` 并行下被调度噪声吃掉」是**同一类问题**：测试用一个 120ms 的真实挂钟延迟去武装（arm）一次英雄锁定请求，然后**同步立即**去 peek 一个后台 goroutine 随时可能已经清空的 `pending` map；一旦测试自身的 goroutine 被调度器晾了 ≥120ms，后台写请求早已经跑完并把 `pending` 清掉，断言就会误判成「没有武装」。

**受影响文件与本轮改动无关**（已逐字节比对）：`backend/r91_addendum_test.go`、`backend/champselect.go`、`backend/champselect_execution.go` 与 git 已提交的 `HEAD`（`4ca768ec`，R119/R120/R122 三张工单落地之前）完全一致，`diff` 无输出。这是一个此前就存在、本轮复测时才被独立触发到的缺口，按约定沿用 R120 编号跟进。

## 执行纪律（延续 R117–R122）

- 每项验收判据都要做**对抗变异**（改回原样，对应测试必须 FAIL）。
- 变异在隔离副本里做，还原用「整文件从基线拷回 + `diff -q`」，**禁止 `git checkout -- <file>`**。
- 不改生产口径、不动阈值、不动 `features.go` 隐私文案。

---

# P1

## P1-1　`TestR91AddendumWildcardHoverClearKeepsArmedLock` 用挂钟窗口做同步断言，负载下必然偶发误判

**证据（独立复测实测，云端隔离快照，2 核容器）：**

1. **两次独立全量 `go test -race -count=1 ./backend`（无人工干预、无并行进程）：2/2 次都在这个测试上 FAIL**，与 R120 复测台账「连续 3 次全绿」和 R122 台账「`ok … 281.937s` 无 FAIL」的结论矛盾：

   | 次数 | 结果 | 耗时 | 失败子用例 |
   |---|---|---|---|
   | 1 | FAIL | 239.997s | `/0`、`/104` 均 `lock not armed` |
   | 2 | FAIL | 233.066s | `/0` `lock not armed`（`/104` 本次通过） |

2. **孤立运行同一个测试（不与其它测试并发）：100% 通过**（`-race -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock'`，单独跑、`-count=20`、`-count=50` 均 0 失败）——说明不是测试自身逻辑必然错误，而是与「同进程还有多少其它 goroutine/请求在抢 CPU」相关，即系统负载越高越容易触发。

3. **人工复现（不依赖真实测试并发，直接用 CPU 忙等 goroutine 模拟负载）**：在同一 2 核容器上起 2 个 `for { }` 忙等 goroutine 占满 CPU，然后独立跑该测试：

   | 命令 | 结果 |
   |---|---|
   | `go test -race -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock' -count=100 ./backend`（2 个忙等 goroutine 占满 2 核） | **12/100 次顶层 FAIL**（`/0` 子用例 10 次、`/104` 子用例 2 次；其中 8 次报 `clear diagnostic missing`、4 次报 `lock not armed`） |
   | 同上、去掉 `-race`、`-count=50` | **3/50 次 FAIL**（`lock not armed` / `different nonzero choice ignored`） |

   去掉 `-race` 依然能复现，说明**不是竞态检测器本身的开销制造的假象，是真实的挂钟调度问题**。

4. **根因定位（debug dump，隔离副本，未进真实工作目录）**：在 `pending == nil` 分支临时加了一段调试输出（用完即弃，未保留在任何交付物里），捕到一次典型失败：

   ```
   DEBUG decision={... Action:champselect-ban ActionID:0 ChampionID:141 Completed:true ...
                    ForceHover:true AvailabilitySource:wildcard-grid-hover ...}
   ```

   `r.champSelect.decision["champselect-ban"]` 此时已经是**第二次（lock-now，`Completed:true`）**的决策，而且诊断事件流里能看到这次武装完整走完了 `schedule→armed→preflight→write→http-success`——也就是说**武装本身成功了，甚至写请求都已经完成**，只是测试自己的 goroutine 恢复执行、去读 `pending` 的时候，武装该请求的后台 goroutine（`backend/champselect.go:1187` 的 `go func(){...}`）已经跑完 120ms 定时器、发完 PATCH、并在其 `defer` 里把 `r.pending[decision.Action]` 删掉了（`backend/champselect.go:1188-1195`）。

   `scheduleChampSelectRequest`（`backend/champselect.go:1142`）把 `r.pending[decision.Action] = pending` 和 `r.champSelect.decision[decision.Action] = decision` 放在同一个临界区里同步写入，这两者不会不一致；测试读到「`decision` 已更新为第二次请求、但 `pending` 已经是 nil」，只能说明**从武装到清理的整个 120ms 生命周期已经在测试自己的 goroutine 被调度器晾着的这段时间里跑完了**——与 R120 P1-1 修复前那句原话完全对应：「1 秒墙钟预算在整包 `-race` 并行下会被调度噪声吃掉，变成随机红」，只是这里的窗口更短（120ms），所以更容易被吃掉。

**影响：** GitHub Actions 的 `ubuntu-latest` 跑者是 2 核共享机型，负载特征与本轮复测用的 2 核容器更接近，而不是 R120/R122 两张台账实测所用的 8 核 Mac——这个随机红很可能在真实 CI 上比本地更容易复现，会污染与本轮 R119/R120/R122 改动完全无关的 PR 的 CI 结果。

**修复方向（只改测试同步方式，不碰生产代码）：**
1. 不要在武装之后同步 peek 一个后台 goroutine 随时可能已经清空的 `pending` map；`scheduleChampSelectRequest` 在同一临界区里已经同步产生了一条 `schedule`/`armed` 诊断事件（`backend/champselect.go:1180` 之后），改成断言 `f.events` 里出现了这条「本次决策」的 `armed` 事件（可用 `decision.Completed == true` 与 `trace_id` 区分是不是第二次请求），这条断言不依赖后台 goroutine 有没有跑完。
2. 如果确实需要断言「写请求还没发生」（而不只是「已经武装」），比照 R120 P1-1 的思路，改成对**完成计数**（例如复用/新增一个「该 decision 是否已经进入过 `write-result` 阶段」的确定性信号）做断言，而不是对一个会被后台清理的 map 做时间窗口内的 peek。
3. 两个子用例（`newID==0` 与 `newID==104`）都要覆盖，`/104` 分支本轮复测中失败频率更低，但根因相同，不要只修 `/0`。

**验收判据：**
- 新断言方式在人工 CPU 压力下（2 个忙等 goroutine 占满 2 核）`go test -race -run 'TestR91AddendumWildcardHoverClearKeepsArmedLock' -count=100 ./backend` 必须 100% 通过（对照组：改动前的旧断言在同样压力下应能复现本工单 §P1-1 第 3 条的失败率，作为「压力确实起作用」的反证）；
- 补一个变异用例验证新断言仍然「真的在测东西」而不是变成空转：把 `backend/champselect.go:985` 的 `forceHover := side == "ban" && strings.HasPrefix(banSource, "wildcard-")` 改成 `forceHover := false`（本轮已验证这是 R120 P2-1 `Test2351BanSentinelHoversOnceWithoutLocking` 用的同一个变异点），新断言必须 FAIL；
- 台账写明：压力复现的具体命令、压力前后的失败次数对比、根因（挂钟窗口 vs 调度延迟）与 R120 P1-1 的关系。

---

## 已复核无需返工（不要重复排查）

- **R119/R122（`docs/WORKLIST-R119-AUTOFILL-DIAGNOSTIC-EVENTS-UNTESTED.md`，执行台账 `docs/r122-execution-ledger.md`）**：P2-1 三层测试与 P3-1 命名收尾**全部独立复核通过**。`autofillDiagnosticFields` 重构逻辑正确、两个事件（`sgp_match_history_*`、`sgp_summary_history_*`）都正确合并；独立重跑了台账列出的全部 8 组变异（M1/M2 删键、M3/M4 删合并循环、M5 改回 `autofill_swapped`、M6 取值错配、**M7/M8 实参换成零值汇总**），**8/8 按预期 FAIL**，其中 M7/M8 是台账里记录的「独立对抗复核」发现并修复的项，本轮复测确认修复真实落地、不是只改了台账文字。`grep` 确认业务代码零命中 `autofill_tagged`/`AutofillTagged`。唯一的瑕疵是**文档级**的：`backend/r122_autofill_diagnostic_test.go` 文件头注释（17-26 行）仍描述修复前「第 3 层只断言键存在、值恒为 0」的旧方案，与实际代码（已改成断言真实数值 2/3/1）不符，是纯注释滞后，不影响测试有效性，顺手改一下注释即可，不需要单独开工单。
- **R120 复测 P1-1（`TestProDirectoryPublishesBeforeSlowSupplements`）**：M1（工单原始变异）复现结果与台账一致——FAIL，6 次补充源请求完成后才返回（约 24s）。`-race -count=100` 默认并行与 `GOMAXPROCS=1` 均独立跑通。
- **R120 复测 P2-1（`Test2351BanSentinelHoversOnceWithoutLocking`）**：M-A（`forceHover` 强制 `false`）、M-B（通配候选条件恒假）两个变异复现结果与台账一致——均 FAIL，超时消息一致（`no PATCH within 5s`）。`-race -count=200` 默认并行与 `GOMAXPROCS=1` 均独立跑通。
- **R120 复测 P2-2（哨兵 `ZQ7XK` + 进程内桩）**：`-count=20000` 独立跑通两个目标测试，`TestGameplayOverviewTencentLookupHTTPErrorIsExplainedAndRecorded`（15.1s，0 失败）与 `TestLootNamingPreservesLocalNamesAndReportsUnresolvedRawIDs`（5.1s，0 失败），与台账「消除 macOS 端口耗尽型随机红」的结论一致。
- **R120 复测 P3-1（CI 与隐私钉死加固）**：`TestPrivacyStoreDirectoryCreationCallSitesArePinned`、`TestPrivacyStoresDeclareEveryLocalDirectory` 独立跑通；`node --test backend/web/r117.test.cjs` 7/7 通过；独立复现了 B6 变异（给 `quality` job 的 `checkout` 步骤后插入一步往 `$GITHUB_ENV` 写 `R100_APP_SOURCE` 的步骤）——按预期 FAIL，报错文案与台账一致。
- **全量回归（独立复测，云端隔离快照）**：`gofmt -l backend/*.go` 无输出；`go vet ./backend` 通过；`go build` 产物正常；**`node --test backend/web/*.test.cjs desktop/*.test.cjs` 全量 885 个测试、884 通过、0 失败、1 跳过**，与两张台账记录的基线数字逐项一致。
