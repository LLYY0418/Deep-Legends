# WORKLIST-R247：收藏页重扫测试只等"请求完成"不等"状态提交"，时好时坏，挡住了 R246 出包

日期：2026-10-07。诊断与工单：Claude（读 R246 账本与 `docs/history/reports/r246/dirty-analysis.md`、读测试代码）。执行：GPT（`deep-legends` 仓库，就是正在做 R246 的会话 A）。版本保持 **0.12.75**，不发布。

**与其他工单的关系**：
- 本单只修测试的等待条件，**不改 `backend/web/app.js` 的业务逻辑**，不放宽断言、不放宽等待预算、不改 60 秒限制。
- 本单完成后，**接着完成 R246 的 P4–P6**（R245 的 P1–P3 已完成；R246 共享向量已替换并核对，SHA `39377b86…cc7`），最后只构建一次 `dist/R246-staging-public/`。

## 问题

`desktop/refresh-orchestration.test.cjs` 中的 "dirty collection rescans on entry and view changes without duplicate refresh requests"：
- 本轮单独连续三次结果为 **失败/失败/通过**，失败点都是 `:271` 的 `view change after 60 seconds did not start the deferred rescan`。
- 这个测试此前在 R237、R239、R240、R244 也多次出现失败或需要复跑。R244 修改前的副本也能复现同一断言失败（`docs/history/reports/r244/baseline-collection-rescan.log:15`）。说明这是**一直存在的测试问题**，和 R245/R246 的授权改动无关：该测试的授权固定为 ACTIVE，不走激活、原生窗口或共享向量。

## 根因（GPT 在 R246 账本中已定位，Claude 核对代码同意）

- 测试在 `:236–241` 的假 `fetch` 里，**在把 `/api/status` 响应还给页面之前**就把 `completedStatusRequests` 加 1。
- `:260–264` 只等这个计数达到 2、3，就认为"clean 状态已经释放了重扫门控""第二次 dirty 已经到达"。
- 但页面拿到响应后还要做几件事，`backend/web/app.js` 中：
  - `:401–405` 解析响应体，并检查请求是否被取消。
  - `:443–449` 通过请求 token 检查后才提交状态。
  - `:456–458` 才解除重扫门控。
- 所以测试有时在门控还没释放时就把时间拨到 60 秒后并切换视图（`:268–271`），重扫不会启动，断言失败。
- 结论：测试等的是"传输完成"，而断言依赖的是"状态已提交"，两者之间存在时序缺口。

## P1　改测试的等待条件

1. 把"clean status did not release the local rescan gate"和"second dirty status did not reach the shell"这两处等待，改为等**页面已经提交了对应状态**，而不是等请求计数。具体方式由 GPT 根据现有代码决定，优先顺序：
   - (a) 用测试已能访问的页面状态（例如 app 暴露给现有测试的 state 对象，或现有测试工具 `r71RefreshHarness` 用到的同一套状态）判断 `collectionDirty` 已更新、`collectionRescanInFlight` 已为 false 等。
   - (b) 如果确实没有可观察的状态，再加**仅测试可见**的观测点：不能改变业务行为，不能在生产构建中暴露新的全局写接口；需要在账本中说明理由。
2. 第一次 dirty 的等待（"dirty status did not reach the shell"）按同样方式检查，避免同类问题。
3. **不允许**：
   - 修改该测试中任何 `assert` 断言（包括"view changes duplicated…""view changes bypassed the 60 second…""view change after 60 seconds…"和 errors 检查）。
   - 增加 `setTimeout` 等待时间或放宽 `waitFor` 超时。
   - 改 60 秒限制或 `app.js` 的门控逻辑。
   - 用 skip、重试或"失败时再跑一次"来掩盖问题。
4. 在测试中加一句注释，说明为什么必须等状态提交而不是请求完成。

## P2　证明修好了

1. 修改后，单独连续运行该测试 **20 次**（`node --test --test-name-pattern="dirty collection rescans" desktop/refresh-orchestration.test.cjs`），要求 20/20 通过，记录每次耗时。
2. 加负载再验证一次：同时跑一个 CPU 压力进程（例如另一个终端跑 `node -e "for(;;){}"`，或同时跑全量 renderer），再连续跑 10 次，要求 10/10 通过。记录方式。
3. 变异：
   - 把等待条件改回"请求计数"，并在假 fetch 中、页面提交状态前人为延迟约 50 ms，测试应**稳定失败**（至少 5 次中 5 次失败）。
   - 证明新的等待条件确实能挡住这个时序缺口。
   - 变异只在临时副本中进行，完成后删除。
4. 账本写在 `docs/r232-execution-ledger.md` 的 R246 一节之前，新增 R247 一节。写清：根因、改了哪些行、20 次和 10 次的结果、变异结果。

## P3　继续 R246

R247 通过后，按 R246 工单继续：

1. P4：`licenseLifetime` 改为 2 小时，并同步更新向量 SHA/条数的固定校验。
2. P5：完成测试，跑完所有规定检查（包括完整 renderer）。
3. P6：只构建一次 `dist/R246-staging-public/`，把新安装包 SHA-256 写进账本和回复末尾。

如果完整 renderer 里出现**其他**失败，按同样原则处理：先单独跑三次，查清是否和 R245/R246/R247 相关。相关就修；无关就停下报告。

## 验收标准

- 收藏页重扫测试单独 20/20 通过，加负载 10/10 通过，变异能稳定击中。
- 没有修改任何断言、等待预算或业务逻辑。
- R246 完成并出新 STAGING 包。
