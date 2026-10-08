# 0.12.77 候选后执行台账（R252，进行中）

日期：2026-10-08，Asia/Shanghai。基线0505fbb038bdb78c58352addc3e8bcd523ecaa8b；新分支codex/post-0.12.77-fixes；版本0.12.77，不发版。已重读R238全文的P7与小建议、发布账本及AGENTS。发布分支仍6a2911c5802e46116cc3ebeab64f788da4ac2f40；v0.12.77仍指向0505，tag object6544b99f40893f1cf1fb960cdf24b5541f48b17b；Draft405971661保持未发布。新分支直接从0505创建，不继承发布后的仅文档提交。所有新修改仅在本分支。

## P0 边界及复现证据

[protected-before.json](../reports/r252/protected-before.json) 已逐项核验原160个文件的size/SHA一致；[draft-before.json](../reports/r252/draft-before.json) 保存Draft405971661只读响应。tag、发布分支、Draft均不修改；只推新的验收分支。无私钥/.dev.vars/Secrets/manage读取，无真实注册码，无隐藏识别代码合入。`.claude-tmp/`原有归档不展开、不提交。

默认包内容隔离发现实质缺口：0505 的 `backend/main.go` 全量 `web/*.html`/`web/*.js` embed 会包含注册码/授权到期文案，即使运行时隐藏。`default-assets-baseline` 对实际 embedded FS 扫描断言失败。新增 `embedded_assets.go` 与正/反 license 标签的 frontend 包装；原激活 HTML/JS保持原样，仅 license 编译，默认HTML移除激活/到期区块、默认桥接无网络调用。逻辑路由仍是 `/index.html`、`/license-ui.js`。`default-assets-fixed`、desktop默认护栏通过，变异回嵌原 HTML/JS和删除严格扫描均断言失败；实际public包审计待最终构建。

## P1：R238 P7第3–8项（逐项重读原文）

| 项目及原文关键事实 | 复现证据/缺少数据 | 状态与处理 |
|---|---|---|
| A3 `mayhem_rating_lookup` 3.2/10.7/25.5秒、TTFB1.5–3.7秒、143KB；最近三个晚高峰/P90 | 工单只有旧三笔数字；现有报告没有三个北京20–22点同口径真实运行样本。需三晚完整时间线、请求数、失败率、TTFB与总耗时分布才能算P90 | **无证据，保持现状**。不猜缓存/压缩/预取方案；不能把三笔旧值写作三晚分布 |
| A4 `facade_load_cost`约9.1秒（目录5.7/挑战2.6/聊天0.4），真机确认不拖首卡 | `backend/profile_facade.go:130–138`仅记录facade分项成本；没有同运行连接、facade、首卡的相对时序。旧SSE/首卡改动不能证明现有竞争 | **无证据，保持现状**。缺Windows同运行LCU连接/首卡/后台调用时间线，未在 Windows 实跑 |
| A5 `match_ids` timeout同秒6次，确认回退不空页 | `backend/specialist_runes.go:314–326`独立步骤deadline；正确命名的`TestSpecialistRiotStepsCarryIndependentDeadlines`实跑通过。新`backend/web/r252.test.cjs`合成 upstream-timeout，实际三个渲染函数保留OPGG/职业选手tabs，绝活区域显示超时与重试；OPGG无position且无runes才提供客户端备用。6条缺player/request关联证据，无法确认同一请求重复 | **无证据，保持现状**（生产逻辑）。补回归验证失败区域不清空其他来源；不承诺其它上游必然可用或自动切tab，不硬去重 |
| A6 隐藏玩家固定掩码失效也不修 | 原文明确“不做”，用户本轮明确排除R249–251 | **无证据，保持现状（范围排除）**。无合入、评审、反向修改或推快照 |
| A7 R246已停止，窗口/15s/连接提示半成品只留标签代码 | 原文明确不继续生产改动；C3授权只修两测试夹具 | **无证据，保持现状（已搁置）**。不改协议/生产安全规则/共享向量 |
| A8 设备缺Go导致release-quality/filter/setup-only失败 | 本机Go可用，`default-node-guards`四文件实跑PASS17.485s，2个既有Windows条件SKIP，最终Windows CI执行对应平台项 | 本地护栏已验证；Windows最终SHA结果待补 |

A3/A4/A5未拿到因果证据，故无生产修复及对应回退变异，不用合成数据替代真实日志。

## P2：B1–B3复现、修复与变异

- B1：`b-go-baseline`已先出现队数断言失败，随后HTTP夹具因sandbox不能监听而panic，环境失败不算业务变异。真实监听`b-go-real-listener`通过。Go与JS均取max(实际队数,已知队列固定队数)，覆盖1750三队子集、1700前半名次、实际十队不截断、未知队列/缺名次。JS旧逻辑影响`renderArenaMatchOverview`小队色调及未知result连胜回退；`js-arena-baseline`复现3队/第3名误判。已同步修复；`b-a5-js-fixed`通过。
- B2：`b-tags-baseline` map0断言失败；仅map0/缺失按solo/flex/match→峡谷、aram/hextech-aram/classic/qualifier→大乱斗、arena→斗魂回退；明确map优先。classic/qualifier依据Go分类矩阵map12，clash因跨地图保持其他模式。`b-tags-fixed`及更新后`b-a5-js-fixed`通过。未知模式仍可显示共通标签，不伪造地图。
- B3：`startSeasonStatsRefresh`持久marker和query snapshot两处recent都仅挡自动刷新；manual fresh保留flight去重/最新一局探测。合成HTTP新局215在60秒内更新，无新局仅1次请求且no_new_game。旧R214/R241测试只更新被用户明确撤销的“fresh也要recent”预期，其他断言/预算不放宽。

[mutations-01/results.json](../reports/r252/mutations-01/results.json) 10项全为断言失败、源文件前后SHA一致；增加JS队数两项并最终源码再次验证，结果待补。第一次A5客户端备用测试错误地断言原fixture title，生产函数按既有规则覆盖title为“客户端内置（备用）”；修正夹具期望，失败原日志保留，不改生产规则。深/浅Chromium合成截图待补；未在 Windows 实跑。

## P3：稳定性

C1证据：旧Linux164文件239134ms，最慢79858ms。实际本机CPU profile31.481秒采样里，`hydrateVisibleMatchTags`占16.820秒（以保留profile按production函数名寻找最近祖先的可复算分析为准）；每个卡片对container querySelector整树扫描导致重复遍历。改为一次querySelectorAll建gameId→首节点索引，原可见性/队列/结果/工作线程/超时不变。195个合格条目+5排除+一个缺DOM条目回归保持集合，整树读取<=1；回退到逐卡195次扫描必然断言失败。200卡原全部断言独立实跑通过14.895s（原profile运行31.696s含采样开销，不能当同条件提速百分比）。不改既有90秒文件/240秒集合预算，CI另加192000ms硬门槛。最终Linux≥20%余量待CI实测。

C2：本地30个独立进程 **0/30失败**，每次PASS及时间线齐全，19.133s；[r104-local-30/summary.json](../reports/r252/r104-local-30/summary.json)。只新增阶段单调时间线与失败时goroutine堆栈，不改业务/5秒断言。已排除本地确定性FIFO顺序失败、固定夹具必然卡死、已停后台worker依旧持有quota等稳定复现；没有排除Windows磁盘/fsync调度抖动或旧tagCI运行环境影响。**未解决：旧Windows偶发根因未复现，不能声称已修复**。Windows新SHA再跑30次并保留每轮日志；不retry掩盖。

C3：两个旧夹具在`license-two-baseline`语义断言失败；R233原10+5分钟隐含15分钟合同，改为elapsed10分钟+licenseLifetime余量，持久counter/不能延长缓存期限/最终过期断言不变。R242原issued+900晚于旧15分钟lease但早于当前2小时lease，不能作为合法授权到期；合法signed field取lease.ExpiresAt，所有非法字段断言不变。`license-two-fixed`PASS13.422s，旧夹具回退各断言FAIL；`license-compile-real`全包compile PASS17.849s。只改测试前置，无生产license/签名/字段/错误码/向量变化。

## 全部过程与失败记录（不可覆盖）

每次 `run-check.py` 写微秒时间戳 .log与.json，JSON含完整命令、开始结束、耗时、exit与最后5行，全部留在 [r252报告目录](../reports/r252)。包括以下真实失败，不用后来PASS覆盖：

- license-two-baseline：两个15分钟旧合同夹具FAIL。
- b-go-baseline/b-go-first-fix：业务断言复现及sandbox httptest监听Permission denied；后者不能算修复失败或变异证明。
- b-tags-baseline：map0回退缺失断言FAIL。
- hydration-baseline：195次整树读取断言FAIL。
- hydration-fixed第一次：测试提取器不认识注释中的单引号，Unbalanced function；仅去掉注释单引号，保留失败，不计变异。
- default-assets-baseline：默认embed含授权文案断言FAIL。
- license-compile第一次：sandbox系统Go缓存Permission denied；真实环境编译PASS保留另档。
- a5-specialist-regression第一次过滤器用了不存在的旧符号，只验证其他两测试，不能据此声称独立deadline覆盖；`a5-specialist-correct-filter`正确符号PASS8.684s。
- js-arena-baseline：旧JS实际队数优先断言FAIL，同时发现A5夹具期望title不符；后者按既有title修夹具，失败日志原样保留。
- mutations-01：10个预期业务/护栏/夹具断言FAIL；驱动PASS，不含语法/依赖失败。

## P4 最终验证与交付（进行中）

最终默认Go/race/vet/installer/gofmt、全量renderer/Worker/JS、标签编译/两夹具、深浅截图、public包审计、最终SHA完整Windows/Linux CI、160文件结束核验待补。当前版本仍0.12.77，Latest仍0.12.76；不操作tag/Draft/发布分支。

### 独立复核后的限定修正（10:06）

前端仅1700/1710/1750的固定队数映射漏掉后端已登记的1701/1704/1720/1731/1732/1740；新增已知队列矩阵后`affected-review-fixes`断言FAIL（第一次文本替换未命中目标行，源码当时仍旧），随后精确替换并单独实跑。未扩展到未知队列，固定下限与既有Go分支一致。默认R248首帧测试从旧license变体移到实际default资产，hidden断言改为更强的元素完全不存在；Windows真实Electron probe显式选择default资产，历史R248 mutation probe仍保留原模板默认行为。Mutation驱动加无效替换检测及失败test名称限定，避免无关断言记作kill；先前12项记录保留，最终新输入再次运行。

独立探子称mutation调用base(new disabled file)导致失败，根线程沿scripts/r252-mutations.py:38–40核对实际代码并无该调用，已有12项完整results也反证，未按错误线索修改。B2已有map回退测试亦已核对，不按遗漏报告删除断言。

### 第二轮renderer失败与限定接法修复（10:09）

`renderer-all-final-02` 104627.852ms **FAIL**，R225两个静态护栏无法解析新加的`go test -tags license -run ... ./...`，默认滤镜一直只支持三组Windows Node过滤器。未改护栏解析器或3组计数断言。新增独立`scripts/r252-license-check.cjs`：结构化Go参数先编译所有tagged包、精确列出两fixture名字，再读取go test -json并用既有verifyGoEvents证明两fixture全部pass/zero skip；显式搁置测试不混入默认集合。CI改调用该入口，失败原始log/JSON保留；最终全量再次执行，不用重跑覆盖失败。

### 最终限定修正验证（10:12）

`mutations-final-03`80.877s PASS，12项均为指定test的断言失败，source SHA前后一致：[results.json](../reports/r252/mutations-03/results.json)。默认资产两项变异、两个fixture回退、两个manual gate、两个Go/两个JS队数、map fallback、一次DOM索引均有精确归因，无语法/环境失败。

标签验证入口18.512s PASS，两fixture明确expected=2、passed=2、skipped=0；全部标签包compile PASS。受影响race19.075s PASS。真实Electron/macOS以default变体2.096s PASS，首帧正常1050×750/opacity1/可缩放，授权请求/事件0。首帧原有startup-loading会暂时将appFrame inert，旧R248记录也同样如此；这是读取客户端期间的既有startup逻辑，不是激活遮罩，本轮不修改该业务逻辑，不声称首帧所有交互已ready。R100图片队列1.779s/R117CSS4.619s真实Chromium护栏PASS。

最终B截图[chromium-02](../reports/r252/chromium-02/chromium.json)：深/浅各map0标签、manual刷新两张共四张，页面演示角标明确合成；自动freshHistory=0，按钮点击freshHistory=1，两次间隔不足1秒。未在 Windows 实跑，不能替代真实游戏样本。

## 本地预检汇总（最终输入，10:16）

| 检查 | 结果 | 秒 | 完整命令/最后5行 |
|---|---|---|---|
| go-all-final-02 | PASS | 183.779 | [JSON](../reports/r252/go-all-final-02-20261008T101010.440225.json) |
| go-race-final | PASS | 231.481 | [JSON](../reports/r252/go-race-final-20261008T100233.385629.json) |
| go-race-affected-final | PASS | 19.075 | [JSON](../reports/r252/go-race-affected-final-20261008T101206.248220.json) |
| go-vet-final | PASS | 1.598 | [JSON](../reports/r252/go-vet-final-20261008T100237.604734.json) |
| installer-test-final | PASS | 3.355 | [JSON](../reports/r252/installer-test-final-20261008T100239.128392.json) |
| installer-vet-final | PASS | 0.241 | [JSON](../reports/r252/installer-vet-final-20261008T100239.183029.json) |
| relay-worker-final | PASS | 0.286 | [JSON](../reports/r252/relay-worker-final-20261008T100127.304608.json) |
| final-license-helper | PASS | 18.512 | [JSON](../reports/r252/final-license-helper-20261008T101003.091791.json) |
| final-filters-helper | PASS | 1.845 | [JSON](../reports/r252/final-filters-helper-20261008T100959.902160.json) |
| final-static-after-ci-helper | PASS | 13.043 | [JSON](../reports/r252/final-static-after-ci-helper-20261008T101530.334377.json) |
| chromium-b-final-02 | PASS | 2.639 | [JSON](../reports/r252/chromium-b-final-02-20261008T100639.698990.json) |
| default-electron-final | PASS | 2.096 | [JSON](../reports/r252/default-electron-final-20261008T101058.358901.json) |
| mutations-final-03 | PASS | 80.877 | [JSON](../reports/r252/mutations-final-03-20261008T101006.729612.json) |

[汇总](../reports/r252/final-local-checks.json)保留所有最后5行；Go默认全量183.779秒、race全量231.481秒，后续JS映射修正后受影响race19.075秒也PASS。最终renderer第三轮待结束；第二轮失败依旧留档。默认测试实际239文件、license测试0，原skip数4未增加。工作区另出现未跟踪的R253文档，本轮未读/未改/未提交。

最终第三轮renderer **PASS118622.712ms**，165文件、1337测试/1333PASS/4既有SKIP，预算90/240秒不变；[timings-local-03](../reports/r252/renderer-timings-local-03.json)。本机macOS结果不替代Linux192000ms门槛，待同SHA完整CI。全部本地预检结束，public审计包开始在`/private/tmp/R252-0.12.77-no-license-public-20261008T1017`构建，无根dist写入。

原始Node失败日志自带空白行末空格，暂存全量diff检查报告22处raw `.log` whitespace；[原输出](../reports/r252/raw-log-whitespace-check.log)保留，未改任何日志字节。源码/文档/JSON仅排除本轮raw `.log`后git diff --cached --check PASS，未放宽源码检查，[精确命令](../reports/r252/source-diff-check.json)。
