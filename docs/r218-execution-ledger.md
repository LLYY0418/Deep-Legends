# R218 执行账本

工单：[WORKLIST-R218](WORKLIST-R218-R216-VERIFICATION-TWO-FAILING-GO-TESTS-LIGHT-MORE-BUTTON-AND-R217-CONTINUE-WITH-REVISED-GATE.md)。2026-10-05。基线 0.12.71 + R211～R217 工作区。

## P1 平台探测与历史窗口

- 生产 `loadLiveLCUMatches` / `clientTencentServerID` / `platformInfo` 未修改。
- R168 桩按 endpoint 区分：`/riotclient/command-line-args` 返回 `[]`，平台探测与历史请求使用独立原子计数器。
- 历史路径仍检查 `begIndex=0`、`endIndex=29`；两个子用例的原场次数和队列检查保持。
- 同一 client 连续加载两次，每次只增加一个历史请求；平台探测总计一次，证明缓存生效。

## P2 强刷总览请求与后台扫描

- 生产后台赛季扫描逻辑未修改。
- 测试传输层用已有 `overviewFreshHistoryKey` context 标记统计前台 SUMMARY；后台扫描使用独立的 background context，计数互不干扰。
- 保留 `expectGameId` 不比普通强刷多拉一页的检查；增加前台 baseline 非零断言，避免空计数造成假通过。
- 保留四次强刷诊断的 `sgp_history_cache_hits == 0` 断言。
- 第一轮强刷等待后台请求 context 的 `Done()`：生产在完成扫描、缓存写入、诊断和进度广播后执行 deferred cancel。断言后台 fresh=true、use_history_cache=false、history_calls=1、cache_hits=0。
- cleanup 使用同一完成信号与锁保护的 in-flight 状态，无 sleep，不把后台请求混进前台计数。
- 两个测试联合 `-count=100`、联合 `-race -count=20` 通过。最终命令及结果见收尾。

## P3 测试记录纠正与新约定

R218 复核确认 R216 的“Go 全量通过”不能证明其最终工作区：R168 探测混入计数必失败，R211 后台请求竞态可能失败。旧声明不作为最终验收依据；本单修复后以最后一次 Go 改动后的无缓存全量记录为准。不能用“可能是结果缓存”代替这次记录问题的纠正。

`WORKLIST-INDEX.md` 顶部已写入：最后一次 Go/测试改动后必须执行 `go test -count=1 ./backend`，记录运行时间、耗时、最后 5 行；未重跑只能写“未重跑”。输出不足 5 行按实际记录，不补造行数。

最终无缓存全量于 **2026-10-05T15:49:40.069586+08:00** 开始，耗时 **255.184 秒**，退出码0。见 `docs/history/reports/r218/go-full-run.json`。这是前台计数非零断言补齐后的最终Go工作区，不沿用15:41的上一轮结果。

输出最后5行（本次完整输出只有1行）：

```text
ok  	lol-loot-assistant/backend	253.383s
```

## P4 显示更多对位按钮

- 为 `[data-table-more]` 增加表格专属样式：正文色 `--ink`、对位行底色 `--surface-strong`、既有 `--line` 边框。
- 截图工具增加该按钮的文字/背景色及 WCAG 对比度测量，并等待有限的 CSS 主题过渡结束后测量，避免把切换中的中间色误记作浅色结果。
- 浅色 780/1280：**15.11:1**；深色 780/1280：**13.39:1**，全部 ≥ 4.5:1。
- `docs/history/reports/r216/table-light-780.png`、`table-light-1280.png` 已重截；`layout.json` 的 `moreButtonContrast` 含实际颜色和比值。其余浅/深色标签、英雄表截图同步保持八场景布局检查。
- 人工查看浅色 780px 截图，按钮文字清楚；这是 Chromium 夹具渲染，不是 Windows 客户端真机验收。

## P5 与边界

R217 后续按 R218 修订的门槛继续，冻结 P1 研究形状，不再搜索；详情写入 [R217 账本](r217-execution-ledger.md) 的后续小节。结算评分仍为 v2.1、详情缓存 v4/赛季缓存 12 不改；标签模型按 R217 要求升级到 schema 3。十个关闭关键词仍关闭，四个原保留关键词仍可见，待 Claude 新留出集评估再决定。

不构建、不打包、不发布；应用版本保持 0.12.71。浅色按钮 Windows 真机验收待用户。

## 收尾

- 最后 Go 改动后的 `go test -count=1 ./backend`：通过（15:49:40开始，255.184秒）。
- `go vet ./backend`：通过（无输出）。
- `go test -count=100 ./backend -run '^(TestR168LiveHistoryWindowFindsOlderSameQueueGames|TestR211SGPForceSummaryFreshAndAllExpectedReusesFirstPage)$'`：通过，7.715秒。
- 同样两项 `go test -race -count=20`：通过，4.110秒。
- R217 曲线/标签持久化/同局时间线复用相关 `-race`：通过。
- `node --test backend/web/*.test.cjs`：920/920 通过。
- `node --test desktop/overview-render.test.cjs`：55/55 通过。
- `node desktop/r216-layout.cjs`：八场景通过，按钮对比度全部达标。
- P3 新留出集200局已冻结；评估脚本只进行语法检查，未执行。没有评估结果或started标记。
- 最终收尾：`git diff --check` 通过；R216 三份冻结文件、R217 规则/候选/账号/清单及五份源码哈希未变，采集清单 590 份文件的字节数和 SHA-256 全部复核通过。最后 Go 修改时间早于上述无缓存全量启动时间。

两个独立只读探子核验最终候选、16账号和200局元数据、源码冻结hash及评估脚本；评估输出增加重复/额外participant key拒绝检查。合法的不可计算曲线按缺失预测报告，不伪造分数或从分母删除。

## R217 评估后的显示接入

Claude 已完成一次性评估，用户指示按 openKeywords 接入六关键词显示，八项继续关闭；完整评估三组数字与逐关键词 precision 见 [R217 账本](r217-execution-ledger.md) 末尾。GPT 未重跑评估。本次不改模型、阈值或缓存版本，不重算已有缓存；本次最后 Go/测试改动后的无缓存 Go 全量/vet、Node 929/929（单独串行全量重跑）、overview-render 55/55 通过；首轮 Node 图片启动限时失败记录保留，原断言未放宽。最终验证时间/耗时/输出以 R217 本次收尾为准。R218 原 P1～P4 验证仍有效，浅色按钮 Windows 真机验收待用户。不构建、不发布。
