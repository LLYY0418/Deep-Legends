# R225 执行账本

日期：2026-10-05（北京时间）。基线 `1c0508a6`，版本 0.12.73；使用 `/tmp/deep-legends-r225-ci` 隔离工作树，不混入并行 R223/R224。

## P1：恢复 Windows 路径测试

R222 漏掉 `TestR204KeySaveAndClear`，原名单的 `TestR204KeySaveRejectsUnauthorizedAndEncrypts` 是不存在的名字。现在删除 backend/installer 的缩减 `-run`，在独立 Windows CI 步骤运行完整 backend（非 race）和 installer 所有包。构建脚本仍带 `-SkipTestsInCI`；两个 CI job 继续并行；release.yml 只生成草稿的设计不变。

选择 **A**，而非维护 Windows 白名单：覆盖未来新增的跨平台测试及全部 Windows 编译实现，避免再次漏掉共享入口。初测本地 backend 完整 JSON runner 150.393s（与 Node 全量并行运行）、installer 3.518s；Windows 全量实测与 ≤15 分钟总时长以下方最终 CI 证据为准，不能用本地时间推断。

覆盖调查见 [Windows 逐函数映射](history/reports/r225/windows-path-map.md)：枚举全部 7 个 backend、19 个 installer（含 uninstall/internal/webviewhost）Windows 源文件的具名函数，再追共享入口到测试；不依赖 Linux skip 或测试文件后缀。R204 的保存/重载真正调用 DPAPI；更新 manager 初始化调用安装探测，但 fixture 后续 fake freeBytes/launch 不等于原生 installer 启动；部分客户端/相机测试是 fake 或源码结构检查。完整 Windows 测试恢复基线的编译及共享路径契约，不宣称所有 Win32 API 错误分支都有单测。实际安装升级仍是独立集成证据。

## P2：静态与实际运行守卫

- Linux quality 新增 `node scripts/verify-ci-test-filters.cjs`。解析 ci.yml 每个 `-run`／`--test-name-pattern` 分支，在对应目录查声明；Go 使用 AST，包含 Windows 源测试、不依赖当前 GOOS；不把 installer 的同名测试误当 backend 匹配。Node 同时校验声明数与 `--expected-tests`。关键 Windows backend 三个真实名称也必须存在。
- Windows Go 使用 `go list` 的当前平台 TestGoFiles/XTestGoFiles + AST 清单，再运行 `go test -count=1 -json`。全部已编译顶层测试必须有终态；全部 TestUpdate*/TestR204*/TestR201*/TestR198*、客户端安装启动组、accept trace/进程共享组必须 pass。Windows 上三个关键名字与所有 installer Windows 测试不能被 build tag 静默排除；installer 全部必须 pass。输出逐条 `--- PASS` 及 R225_GO_VERIFIED 无载荷汇总。
- Windows Node 用明确 TAP reporter；release gates **2**、setup-only **1**、R82 PS5.1 **2**。tests=pass=expected 且 fail/cancelled/skipped/todo 全为 0，否则失败。
- 四项回归包含：真实 ci.yml 添加一个不存在的 Node 分支失败；旧 Go 错名失败、跨目录名字失败；Go pass 缺项/skip/空事件失败；真实 Node 子进程选中测试通过、改为 skip 后失败。独立 CLI 子进程清理父 node:test 的 NODE_TEST_CONTEXT，避免测试自身嵌套时静默跑零项。

## 本地验证

初轮完整 Node 1265 项，1261 pass、4 平台 skip、0 fail，96.389s，最慢文件 49.643s；最终受影响静态/运行守卫、R86/R222 CI 结构断言复验通过。backend JSON 完整 runner 1860 顶层声明，1834 pass、26 平台 skip、81 个关键用例实际 pass；installer 87/87、0 skip。原始日志只留本地，不提交到公开仓库。

## 同 SHA CI（待实测）

记录 quality、windows-build 及 release 作业：ci.yml 实际只有前两个 job；第三项 release.yml 由 tag 触发，本次分支验收不推 tag、不创建草稿、不覆盖已发布 0.12.73，故第三项应如实记录未触发，不能编造起止时间。完整 CI 总时长必须 ≤15 分钟。

最终 public 后端重新构建并校验指纹；版本 0.12.73，key mode public，指纹 be7741263b5b。最终工具 JS syntax/Go vet 通过；四项独立审查与回归通过，无载荷汇总见 reports/r225/local-validation-summary.json。

## 首轮 CI 与 Windows 全量暴露的夹具竞态

运行 `37321360823`，SHA `d0e6fdf4`：Linux quality success；Windows full backend 中全部 18 个 TestUpdate*、TestR204KeySaveAndClear、TestR204KeyRuntime401AndPrivacy、TestSplitRegistryPathSupportsNativeTencentKeys 已逐条 PASS。但职业目录回退与赛后 honor 的两个旧夹具竞态使整包失败，正确阻止 installer/后续构建。

- TestProDirectoryFailureRetainsRosterAndAuth：loadProPlayers 先 close(done)，随后后台 profile fallback 更新 teams/fetchedAt；handler 会重新读取最新 snapshot。夹具原来依赖 Unavailable=true 时 fallback 尚未完成，Windows 调度可提前完成。现在仅在夹具 transport 暂阻塞 profile 请求，固定首次失败目录响应的观察阶段；cleanup 放行。33 人/53 账号/6 队、available、Unavailable、no-store、query 与鉴权断言原样保留；人工名单及生产逻辑不改。
- TestR207HonorBallotOnlyAtEndgame：fired 通知先于 handleHonor defer 清除 honorInProgress。夹具收到 fired 后立刻进入下一阶段，会让第二次 handleHonor 被 in-progress 拦下。现在收到 fired 后按 mutex 读取状态并等清理完成，仍共用原 1 秒 deadline，不放宽预算；原早期 0 写入/0 trigger、两个 endgame 触发与完成断言保留。
- 两项修复在本地 `-race -count=40` 共 80 次重复通过。最终本地 Node 全量 1266 项，1262 pass、4 平台 skip、0 fail，105.761s，最慢文件 62.203s。最终 CI 需再次同 SHA 全部通过。
