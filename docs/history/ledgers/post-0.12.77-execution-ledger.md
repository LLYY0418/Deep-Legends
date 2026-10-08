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

## 首轮分支CI与构建失败（原记录保留）

修复提交`104b62838c68b87d50b5b3980a8baf2abb26a363`已推，仅post分支；[37717392380](https://github.com/LLYY0418/Deep-Legends/actions/runs/37717392380) **failure**。[原job日志/全部artifact](../reports/r252/ci-01/run.json)。Linux格式/vet/严格默认授权缺失/license compile与两fixture/race/fullrenderer1337项均通过；renderer202984.504ms未达到192000ms硬门槛（未改）。大DOM197734.694、串行other202970.589，故继续调度修复。被后续step阻断的Worker/Chromium/installer未执行，不能算这一SHA的整体验证。

Windows全量backend1951/installer100测试与PASS清单、默认首帧、四项条件项（R235具体由全量inventory证明）、30次独立R104均通过。R104 **0/30失败**，[Windows每轮时间线](../reports/r252/ci-01/r252-r104-windows/summary.json)；结合本地60轮0失败仍无法确定旧tag偶发根因，保持未解决。Windowspublic构建/校验和通过，但实际升级step失败`Eight stages not imported as result=ok`，随后最终包artifact未上传。

升级证据安装版本0.12.77.0/file0.12.77/asar0.12.77；relaunch1791426591181、installer_start1791426582164、total9017ms，diagnostic事件实际时间2026-10-08T02:29:51.5344331Z（1791426591534），fingerprint f2794a1d683c，result=ok。三个精确条件本应匹配，怀疑JSON时间已经DateTime又Parse成string丢失毫秒，原函数路径在PowerShell6+存在该类型转换（[微软说明](https://learn.microsoft.com/en-us/powershell/module/microsoft.powershell.utility/convertfrom-json?view=powershell-7.5)）。仅验证helper按类型保留分数；新增原生断言/旧Parse变异，下一轮CI确认再定修复，30s预算和指纹/total/时间界限未放宽。

第一次`gh api .../logs`工具拒绝ANSI输出，原0字节文件+失败JSON保留；第二次按gh明示allow-escape-sequences获取原始字节，未重跑CI。

本地public审计构建99.194s在预检阶段失败：README夹具缺段落（另一会话同时改README，未反向恢复）和R92 ENOSPC；尚未产出新的本地包，原.log/.json保留。首次push自动审批因远端可信性证据不足拒绝；只读gh确认当前认证用户/远端owner同为LLYY0418，公开repo和用户指定tag/Draft一致后原动作获准。未换接口绕过，未推任何保护ref。

缓存清理调用被用户中断；没有本轮derived-cache-cleanup结果，不重试删除。共享根工作区后续成为R253，独立worktree从104b创建，后续R252代码只在原post分支。不可把R253通过结果用作R252证据。

### 保护文件核验异常

10:15中期160个均一致（protected-midpoint.json）；中断恢复后8个PE缺失，原因未确认。独立探子核对R253的protected-validation-before/after也为152一致/8缺失，未找到原实体备份，不归因到R253；[实际缺失清单](../reports/r252/protected-after-other-thread.json)。已向用户询问原SHA备份来源，不能重建/替换，不写成160全部未变。本轮所有public审计使用tmp输出，没有执行root dist清理命令。

### C1第二阶段与原生验证候选（12:11）

四worker共享队列按首轮CI实测cost最长优先，发现集合166文件（新增拆分文件），所有文件最大并发仍4，预算90/240秒不变。原requests 361行按overview/live两块拆分，共用原前置导入，测试body拼接逐字节等于原文件；[拆分证明](../reports/r252/renderer-split-proof.json)。本机全量 **111924.909ms PASS**，测试1337/1333PASS/4SKIP，未减断言/集合；生产指纹仍f2794a1d683c，Linux20%目标待下一SHA。

升级helper修复候选仅把DateTime/DateTimeOffset按实际类型取Unix毫秒，string仍Parse；CI新增原生测试覆盖en-US/zh-CN、string/typed对象/真实ConvertFrom-Json、界限前旧事件拒绝。原Parse(object)必须触发分数断言失败才能通过mutation护栏；本机无pwsh，未在 Windows 实跑，下一轮CI证实。原30秒和exact fingerprint/total/time不改，不将第一次失败说成安装失败或已排除根因。

未推送候选5e9c4653误包含下载artifact中的106.69MiB Setup，GitHub100MiB限制拒绝。实际Setup保留为本地证据（size/SHA receipt），移出Git索引，不删除任何原始失败日志；重写仅该未推送post提交，不涉及保护tag/分支。R252报告目录使用-text属性保留Windows原始CRLF/BOM字节，增加exe忽略防止重现。请求类拆分边界末尾空白规范化（source diff检查要求），全部test body/断言不变。

## 第二轮CI与默认包审计（12:32）

候选`4c48777a95426293f8e1146485bbfb7bb6d708f3`，[37726779989](https://github.com/LLYY0418/Deep-Legends/actions/runs/37726779989) **整体failure；windows-build整体success、quality failure**。[原始日志与状态](../reports/r252/ci-02/run.json)。不引用其他分支通过结果。

Windows全量backend1951/installer100、默认Electron首帧、R104独立30轮、原生PowerShell平台护栏、public构建/校验和、0.12.65/0.12.68/0.12.76→0.12.77实际升级通过。原生分数时间验证成功；旧Parse(object)变异精确断言FAIL：en-US/DateTimeOffset被截成1791426591000，正确值1791426591534。修复仅验证器按类型取毫秒，实际升级本轮也通过，未改变生产更新或30秒预算。R104这轮Windows30次仍0失败，累计本地30+两轮Windows60=90次0失败，**旧tag偶发仍未解决**。

Linux格式/vet/默认授权隔离/标签编译两fixture/filter通过；全量race在旧`TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds`耗时3.062762007秒FAIL，原3秒断言未改。相关测试、catalog与metadata生产链相对0505fbb diff为空。隔离race原样30轮PASS，88.339秒test时间（外部命令101.981秒）；[分析](../reports/r252/r206-failure-analysis-02.json)与全部轮次日志保留。隔离通过不足以确认并行资源竞争根因，不将这次CI失败替换为PASS。已向用户询问限定测试调度修复范围；未获回复前保持测试和生产逻辑原样。

这次前序race使renderer及下游护栏未执行，C1的Linux≤192000ms仍待实证。为补齐独立证据，将后续JS/依赖/renderer/20%门槛/Worker/Chromium/installer步骤设为`!cancelled()`，不加continue-on-error；任何失败依旧使job及整体失败。三个相关CI静态护栏PASS2.734秒，两个既有Windows条件skip不变。

独立public审计构建02 **PASS200.962秒**，先在该snapshot实跑完整Go1951与installer预检，再构建0.12.77；路径`/private/tmp/R252-0.12.77-no-license-public-02`，不写根dist。实际Windows包审计PASS1.085秒，版本0.12.77、key mode public、指纹f2794a1d683c、授权文案/域名/kid/公钥/隐私定稿句缺失、fuses及ASAR完整性与后端摘要通过；[收据](../reports/r252/package-audit-02.json)。Windows Setup SHA-256 `a9719726622ad7e5f98da779ba41c6c18843765d0e79f4a7259289fa5c8b2dea`；Windows后端SHA-256 `10c0eafb3f6e066decaed3b9b12b735802c576b63e9ffab9491728de526e1af2`。这是本地交叉构建审计包，**未在 Windows 实跑**；上述Windows CI包与实际升级为独立实跑证据。

Mac public后端build PASS5.582秒，actual binary SHA-256 `3374f445381b56a2d2e8b0e84b9ea7cc09834f6100d6c80c584b79cf5cc42a79`；隔离全新LOL_LOOT_DATA_DIR自检PASS0.667秒。Mac二进制授权缺失/空Key/指纹及实际Windows ASAR18个脚本/HTML的UTF8、UTF16授权文案缺失追加审计PASS0.427秒；[Mac收据](../reports/r252/macos-public-audit-02.json)。一次手动CLI误传verify-embedded-riot-key.cjs参数（该CLI只读默认路径，不接收参数），报desktop/backend缺失；改用导出的verifyRiotKeyPolicy(binary,"public")验证实际Mac文件，未改验证器或放宽断言，未读取Key。

12:32再次核验152个size/SHA一致、8文件仍缺失、现存文件变更0；[逐项异常](../reports/r252/protected-followup-1234.json)。本会话root dist写入0，不宣称160全部不变。共享向量SHA仍39377b860440d8f609f9d28162afe086e4e01ac91d3d2ff39da2a45d5eebfcc7。
