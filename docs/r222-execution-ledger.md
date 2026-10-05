# R222 执行账本

日期：2026-10-05（北京时间）。基线：0.12.73，HEAD `4f27d413`；不改用户功能、7z level 9 和在线升级流程。

状态：实现与本地普通全量预检通过，最终 race 及同 SHA Linux/Windows CI 验收进行中；正式发布耗时留待下次发版记录。本次不推 tag、不发布 Latest。

## P1：测试去重与 Windows 覆盖

- 两个 Windows 构建入口新增 `-SkipTestsInCI`；只有严格 `GITHUB_ACTIONS=true` 才接受，先检查再处理源码/依赖/旧产物。本地不带开关仍运行全部原有 test/vet；R86 保留真实失败 Go 测试及独立提前 build 变异，另加两个入口非 CI 拒绝的执行与变异检查。
- 跳过 root/installer test/vet、web/desktop Node 与 public wrapper 的重复 release/Update 测试。格式、node syntax、源码指纹、后端自检、NSIS、wrapper、verify-packaged-runtime、verify-build-fingerprint、release-build receipt 和 SHA256 全部保留。
- `quality` 与 `windows-build` 无 needs，直接并行。release 工作流仅生成草稿，Latest 的完整同 SHA 门槛见发布模板。
- Linux 全量增加此前漏跑的 `scripts/*.test.cjs`，仍全量跑 web/desktop/Worker、Go race/vet、installer 和 Chromium。移除单独重复的 r117-style 和 embedded-pool 调用，它们仍在完整 Node/Go 集合中执行。

0.12.73 Linux 质量日志仅有一个 Node Windows SKIP：

1. `R86 Windows release stops on a real failing Go test and rejects an independent early build`（desktop/release-quality-gates.test.cjs）。现在 Windows 单独执行，另含 R222 两个 release 入口的本地拒绝测试。

此前 scripts 未进入 quality；补入后还必须在 Windows 跑 R82 PowerShell 两项：

2. `R82 A/B receipt lookup and deduplication execute the real PowerShell script`。
3. `R82 A/B filename, hash mismatch and duplicate mutations reach failing assertions`。

`setup-only-build` 三个 Bash 实际执行用例仍在 Linux 全量中，Windows 新增结构护栏 `setup-only Windows CI skip leaves NSIS, wrapper and receipt gates unconditional` 单独执行。

Windows 编译约束及平台分支不能被 Linux 替代，windows-build 还单独跑：

| 模块 | Windows 用例 |
| --- | --- |
| backend | TestSplitRegistryPathSupportsNativeTencentKeys；TestR204KeySaveRejectsUnauthorizedAndEncrypts；TestR204KeyRuntime401AndPrivacy |
| installer | TestWindowsApplicationWindowDetection；TestWindowsMissingApplicationDoesNotYieldAPID；TestR205WindowsShortcutRoundTrip；TestR205WindowsShortcutTargetAlias；TestR205WindowsKeepShortcutsStringValue；TestR205WindowsCOMAlreadyInitialized；TestR86WindowsDestinationPreflightSmoke；TestR86WindowsDialogDirectoryAndDiskHelpers；TestR206IconOnlyChangesIconAndPreservesCreation |
| installer/internal/webviewhost | TestSettingsUseHRESULTAcrossEntireConfiguration；TestCOMUsesHRESULTInsteadOfThreadLastError；TestFailedNavigationEventIsNotReadiness |

这些用例不重跑跨平台完整集合。常规 Windows build 后继续原有真实安装升级。

## P2：按文件并行与耗时护栏

- 原 overview 55 项变成 57 项（external 正常路径、四 mutant 集合另成两项）；原名称/断言全部保留。5 个 200 场组分别拆文件，controls 正常/external/mutants 又分别运行，普通剩余用例按工具/生涯/请求/事件拆开。
- controls 四变异体、filter 和 late timeline 变异体用 30 场；正常路径与 external 都为 200。200 场 `<500` 创建预算不变，四个 full-render mutant 必须因 DOM 护栏 AssertionError reject。image-listener mutant 保持 200，因为 `<1000` 预算依赖数量。
- 同一窗口内的 controls/timeline/metric/damage/keyboard/link 检查仍共用真实 app；共享 harness 统一 teardown。诊断 HTTP 替身仅补 `/api/diagnostics/client`，同时修复 R192 harness 的同类噪声，不屏蔽异常或断言。
- `scripts/test-renderers.cjs` 使用 Node 原生文件 workers（并行度至少 2，避免两核 runner 文件串行）枚举完整测试集合，逐文件实际墙钟 >90s、集合 >240s、失败/缺 summary/缺文件计时都失败；artifact 保存平台、并行度、用例数、每文件耗时。
- CI `measure_windows_renderers` 为手动验收选项，常规发版 false，避免再次在 Windows 常驻重跑全量。

## P3：发布顺序

AGENTS.md 已明确本地预检全部通过→提交版本号→只推 tag。采用“只推 tag，必要分支在 tag 工作流结束后推”的去重方案。创建 `docs/release-ledger-template.md`，同 SHA 完整 quality-and-windows-release success 是 Latest 前门槛，并有开始/预检/版本提交/tag/各作业/可发布 Latest/正式发布的逐阶段时间表。

## P4：Go 慢用例实测

基线 `go test -json ./backend` 通过，包 261.368s（本机 macOS arm64；不能当作 Linux race）。前 20 个顶层用例如下，原始 JSON 见 reports/r222/backend-before.jsonl.gz。

| 用例 | 基线秒 |
| --- | ---: |
| TestR99SeedQuotaDiagnosticsNeverContainIdentity | 60.01 |
| TestR183RankedAnonymousEnemyEndToEnd | 22.41 |
| TestR89DiskBudgetAfter2000Images | 20.63 |
| TestR96PartialTruthDoesNotEndAsNone | 17.04 |
| TestR194BotsAndUnsupportedDiagnostic | 10.96 |
| TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField | 10.49 |
| TestR100OverviewLongRoundUsesFreshSingleWaitBudget | 6.46 |
| TestR92RiotMatchConcreteDiskBudget | 6.02 |
| TestR64FacadeEventBurstIsThrottled | 5.15 |
| TestR86WebsocketDropKeepsLiveLCUConnection | 5.02 |
| TestR102SeedOneAccountFailureDoesNotBlockOthers | 4.10 |
| TestPruneStaleHexdataBuildsBoundsDiskAcrossPatchHistory | 4.08 |
| TestR194QuickplayAnonymousTopEndToEnd | 3.08 |
| TestR113BroadcastEventStormIsBoundedAndRearmsNextSession | 3.01 |
| TestR100AssetHasWholeHandlerDeadline | 3.00 |
| TestR206ArenaRetryPartialAndAllFailed | 3.00 |
| TestR206FullCollectionRefreshTranslationTimeoutUnderThreeSeconds | 2.82 |
| TestR206TranslationsTimeoutUsesPreviousCache | 2.80 |
| TestR201ProbeWindowCancelsSlowRoutes | 2.70 |
| TestR54CollectionProbeRetriesUntilReady | 2.10 |

- R99 60s 真冷却改用已有 limitNow/limitSleep，实际仍经过 429 与限流 admission，新增“恰好等待 60s”断言；专项变成 <0.01s。
- R96 真 0/2/5/10s 等待注入，生产回退为原 timer/cancellation 逻辑，新增精确序列断言；专项 17.04s→0.02s。
- R102 注入测试专用 200ms per-account timeout，仍断言生产请求预算为 4s，完整保留独立超时/其它账号不受影响的断言。
- R183/R90 synthetic LCU fixture 增加离线 champion HTTP 替身，消除意外远程技能回退；原玩家/请求计数/隐私/分组/缓存断言不变。R183/R194/R96/R99/R102 专项整体 1.319s。
- 真实磁盘 2000/605/480 文件数量及预算断言不变；独立 IO/真实 timeout 测试加 t.Parallel，重试/节流等待本身保持真实。
- R100 的 >5s 整轮年龄回归保留真实等待，因为它直接防止旧绝对预算复发；不为减少数字删掉这个护栏。

## 本地预检与构建

本机是 macOS，Windows/PowerShell 专项明确跳过。首轮 Go 沙箱限制本地 HTTP，改在授权环境重跑；首次分拆普通用例漏导入 bootLiveTab 已修复，失败记录保留，不作为通过证据。

- 全量 Node CLI：1277 项，1273 pass、4 Windows/PowerShell skip、0 fail，121.364s（含 Worker）。
- 最终预算 runner：1262 项，1258 pass、4 skip、0 fail，142.602s（web/desktop/scripts，Worker 另测）；每文件全部 ≤90s，最大 external 79.111s。见 node-timings-local.json。
- 独立 Go/流水线 review 未发现生产默认逻辑或断言减弱。Windows 专属 Go 清单由第二轮只读核验补齐。
- installer 全量 test/vet 通过；Chromium R100/R117 通过，R117 unstyledFrames=0；JS syntax、最终 Go test/vet、public 后端重建/自检见下方最终记录。

## CI 与最终结果

待写入同 SHA 的 Linux quality、Windows 专属/全量桌面计时/public 构建/实际升级结果。正式发布 tag→可发布 ≤15min、全流程 ≤30min 的验收要在下一次真实发版账本记录，不能用本次分支 CI 的预计值代替。

最终本地 `go test -count=1 ./backend` 在最后一次 Go 修改后执行，通过，耗时 **117.097s**；输出：

```text
ok  lol-loot-assistant/backend 117.097s
```

最终 root vet、installer test/vet、JS syntax、两个真实 Chromium 护栏通过。public 后端重新构建：版本 **0.12.73**（保持工单基线，未申请新发布版本），指纹 **be7741263b5b**，`--self-test` 与指纹核验通过。本地没有生成安装包，Windows public 安装包由验收 CI 构建。key mode **public**。

最终源码 `go test -count=1 -race ./...` 全量通过，backend **209.067s（macOS）**。Linux race ≤120s 门槛仍须 CI 实测，不能把本机普通测试的 117.097s 代作 race 验收。

公开验收分支只提交代码、文档和不含载荷的汇总 JSON；原始/压缩测试日志仅保存在本地 `docs/history/reports/r222/`。暂存内容已核验无 Riot UUID Key、GitHub token 或私钥模式。
