# WORKLIST-R252：0.12.77 候选后的遗留修复与稳定性验证

日期：2026-10-08（Asia/Shanghai）。执行：GPT；来源：用户本轮指令与R238 P7第3–8项、三条小建议。基线0505fbb038bdb78c58352addc3e8bcd523ecaa8b；工作分支codex/post-0.12.77-fixes；版本保持0.12.77。

## P0 边界与取证

- 不改动/删除/重建/发布v0.12.77、Draft405971661、codex/release-0.12.77；不发布Latest、不bump、不打tag、不建Release。
- 所有改动仅在新分支；不读私钥/.dev.vars/Secrets/manage、不用真实注册码。R249–R251不合入、不评审其逻辑、不推快照。
- 先保存并核验原160个STAGING/R248审计文件size/SHA，结束再次逐项核验；新审计产物使用独立public命名目录。
- 注册码仍只在license构建标签，默认构建不包含授权域名/kid/公钥/隐私定稿句，无激活入口/网络/license事件；保留搁置代码。原始激活 HTML/JS 仅由 license 标签嵌入；默认 HTML/桥接单独嵌入，并对实际二进制扫描授权文案。
- 复现、失败和CI每次单独目录/时间戳，原始日志不可覆盖；不放宽断言/预算、不skip/重试掩盖。

## P1 R238 P7第3–8项逐项证据与处理

每项先在账本列原文、日志/测试/代码file:line，再判定修复。没有可验证证据时明确“无证据，保持现状”，写出缺少的数据；不是凭旧记录推断实际根因。

3. mayhem_rating_lookup：寻找最近三个北京20–22点真实样本，分布/P90>5秒才制定修复；没有样本不硬改缓存/压缩/预取。
4. facade_load_cost：核对后台目录/挑战/聊天与首卡共享连接的时序；有可复现阻塞证据才延后到首卡渲染之后。
5. specialist_runes_step_failed：复现match_ids超时的影响、重复诊断与现有回退来源；确认不会清空对局页，有缺陷再修。
6. 隐藏玩家：用户明确排除R249–R251，不合入或修改现有识别逻辑，记录边界。
7. R246停止：生产逻辑保持搁置，仅P3允许修两个测试夹具/前置条件；不改租约安全规则。
8. 环境性失败：在有Go/网络环境重跑release-quality-gates、verify-ci-test-filters、setup-only-build，拿实跑结果，不放宽护栏。

每个实际修复补测试及去掉修复必然断言失败的变异；变异不得以语法/依赖失败算通过。

## P2 三条小建议

B1. arenaPlacementResult队数=max(实际不同队伍数,已知队列固定队伍数)，覆盖部分参赛者以及实际数超过固定数，未知队列保持实际数。
B2. matchDataTags仅mapId=0/缺失时按modeGroup回退；已有明确地图优先，不能将未知模式误判；覆盖峡谷/大乱斗/斗魂及原地图。
B3. 同账号60秒闸门仅限制自动刷新，fresh手动刷新绕过recent；保留无新对局小请求探测和既有inflight去重。

三项均先复现失败、修复、补测试、各至少一项回退变异。标签及刷新按钮深/浅真实Chromium截图，明确合成数据与非Windows真机。

## P3 稳定性

C1. 分析Linux164文件最慢项及239134ms墙钟，分析后优先消除已实证的重复整树查询；必要时合理拆分/并行分组减少耗时；保持全部集合、断言、90秒单文件/240秒总预算，最终Linux目标≤192000ms（至少20%余量），不足目标写未解决。
C2. R104冷启动测试在本地或CI循环至少30次，逐轮保存失败率、时间线；找到可验证根因才修并做回退变异；找不到列已排除原因并标未解决，不以30次通过宣称修复旧CI偶发问题。
C3. 只修TestR233FailedCounterSaveDoesNotRefreshCachedLifetime与TestR242LicenseExpiryStrictSignedField的夹具/前置条件，符合当前2小时合同；生产license逻辑、字段、签名、错误码、共享向量不改。tags license全量编译，这两个测试通过或明确“随R246搁置”并说明。

## P4 最终验证与交付

- 最终源码go test -count=1 ./...、相关race、go vet ./...、gofmt全量；installer test/vet；node scripts/test-renderers.cjs all、desktop集合、Worker16、JS语法/diff、三个环境护栏；tags license全量编译及两夹具实跑。
- 最终SHA分支完整quality-and-windows-release success（Windows+Linux），不引用0505旧结果；只推codex/post-0.12.77-fixes，不触碰发布ref/草稿，不推隐藏快照。
- 默认public包重新构建并严格审计（授权缺失、Key策略、指纹/digest/fuses/ASAR/隔离数据）；独立目录，不覆盖160个受保护文件。
- 台账docs/history/ledgers/post-0.12.77-execution-ledger.md记录每项复现/修复/变异/失败/CI及源码SHA；WORKLIST-INDEX加R252，R238发布状态保持原样。
- 最终只回复分支/SHA、每项已修复/无证据/未解决、CI链接、失败原因、dist/STAGING是否改动；保留用户决定后续版本归属，不发布Latest。

## P4 依据首轮CI补充的验证修复（2026-10-08中断后）

- 首轮CI37717392380原样留档：Linux整体202984.504ms未达到192000ms目标。将原串行other组和固定3个大DOM worker改为共享原上限4个worker的最长文件优先队列；请求类长文件拆两份，测试body逐字节相同、集合不漏、不增加skip，90/240秒不变。
- Windows实际安装版本/8阶段/精确fingerprint/total_ms=9017及result=ok存在，但CI匹配失败。候选根因是ConvertFrom-Json返回DateTime后隐式string Parse丢失毫秒；仅修验证时间转换，不改更新生产逻辑/30秒预算/精确本次安装匹配。新增Windows原生分数时间、旧事件拒绝与原Parse(object)断言变异；以实际CI回归证据判断，不靠重跑掩盖。
- 共享工作区已被另一会话切到R253；R252后续改动仅在/private/tmp/r252-post-0.12.77的原post分支，不合入R253/R254/隐藏识别。
- 全160保护项的中期核验一致，结束检查现8文件缺失（原因未确认；R253初始核验亦记录同8缺失）；152个仍一致，等待原SHA备份来源，不重建、不覆盖，不宣称160全部未变。

## 第二轮CI后的独立取证

- Windows原生时间转换及旧Parse变异通过，Windows作业整体成功；Linux旧R206并行耗时测试3.062762007秒超过既有3秒，相关测试/生产链相对0505fbb无差异。保存失败，隔离race30次只作为定位证据，不宣称修复。
- 待范围授权前，不改R206测试调度/断言/预算/业务。独立renderer/Worker/Chromium/installer在前序失败后继续收集结果；失败仍使quality job及整体工作流失败，无continue-on-error。
