# R83 启动第二阶段（执行预热）验收报告

对应：`WORKLIST-R83-STARTUP-PHASE2.md`。验收人：Claude（独立复核，不复用 GPT 自己的
`scripts/r83-startup-mutations.py`，全部变异在 `/tmp` 隔离副本中亲手插入）。
日期：2026-09-12。

## 结论

**A、B、C 三节均真实落地。** 在 Linux 沙箱里对 Go 代码做了 7 处独立变异（未参考
GPT 自己的测试脚本，自己选的注入点），**7/7 全部被现有测试杀死**；Windows 交叉编译
（`go build` / `go test -c` / `go vet`，全部 `GOOS=windows GOARCH=amd64 CGO_ENABLED=0`）
干净；根模块 `go test -race ./...`、安装器模块 `go test -race ./...`、相关 Node 回归
（`r82-startup-ab-report.test.cjs` 6/6、`startup-sequence.test.cjs` + `startup-visibility.test.cjs`
8/8）全部独立复跑通过。`.gitignore` 白名单用 `git check-ignore -v` 和 `git status`
两种方式核对，三个归档文件确实不再被忽略。

**一个关键决策被亲手验证为正确**：`--startup-warmup` 与 `--self-test` 在同一台
沙箱机器上分别实测——前者 21ms 退出、不产生任何文件；后者会真实创建
`session-token`、`account-salt` 和一整棵用户数据目录树。GPT 没有偷懒复用
`--self-test`，这个判断是对的。

**唯一遗留缺口**：R82 的 `WORKLIST-R82-ADDENDUM-3-NODE-COMPAT.md`（Node `.at()`
兼容性 + PowerShell "组测满"预检查）仍未执行——`scripts/r82-startup-ab-report.cjs`
第 65-66 行 `.at(-1)` 依然在，`scripts/r82-startup-ab.ps1` 也依然没有"组已测满 3
次"的启动前拦截。这条不影响本轮结论，因为 R83 的新验证方式（C 节）根本不再用这两个
脚本；只是记录在案，避免以后又被人捡回来当"已经修过"。

**本轮无法验证、也不该现在验证的事**：执行预热能否真的把 Windows 真机上那 3 秒
CreateProcessW 打下来，只有真机数据能回答。以下所有验证都是"代码行为和失败边界正确"，
不等于"真的达标"。

---

## 独立复核方法

在 `/tmp` 建立两份隔离副本（`cp -a --parents`，带 `testdata`），不触碰真实仓库：

- `installer/` 整个模块（自包含 `go.mod`），验证 A/B 节。
- 根模块（`*.go` + `go.mod`/`go.sum` + `data/` `web/` `testdata/` `prestige_chromas.json`），
  验证 `--startup-warmup` 的早退行为。

Go 工具链复用沙箱里已缓存的 `go1.24.0 linux/arm64`（`/tmp/go`），`GOCACHE` 单独指向
`/tmp/gocache-r83`，不碰仓库自带的、GPT 遗留的 `.gocache`/`.gomodcache`。

## 独立变异结果（7/7 KILLED）

| # | 变异位置 | 变异内容 | 结果 |
|---|---|---|---|
| 1 | `application_launch.go` `applicationEnvironment` | 去掉 `ELECTRON_RUN_AS_NODE` 过滤，让真实启动可能继承它 | `TestExecuteWarmEnvironmentCannotLeakIntoRealApplication` FAIL |
| 2 | `prewarm.go` `startupPrewarmEnabled` | 改回旧逻辑 `value == "1"`（默认关闭） | `TestPrewarmDefaultsOnWithExplicitControl` FAIL |
| 3 | `execute_prewarm.go` `Finish()` | 把非阻塞 `select{...default:}` 改成阻塞接收，让收尾等待卡死的执行进程 | `TestExecuteWarmupFailuresAndBlockedProcessNeverHoldInstallationHandoff/blocked` FAIL |
| 4 | `execute_prewarm.go` `Finish()` | 去掉收尾阶段对文件是否被改写的复核（只留执行前判定） | `TestExecuteWarmupRequiresStableFinalFileAndVoidsRewrittenExecutable`、`TestExecuteWarmupVoidsSameSizeMtimeChangeAndReplacedFile/mtime` 两个测试同时 FAIL |
| 5 | `handoff_windows.go` `application handoff` 日志行 | 改字段名（`visible`→`ok`，`elapsed_ms`→`took_ms`），不同步改采集端 | AST 耦合断言 FAIL **且** `TestStartupLogAndRealCollectorStayCoupledForSuccessAndLaunchFailure`（真实 Node 子进程解析真实日志）同时 FAIL——双重独立命中 |
| 6 | `main.go` | 把 `--startup-warmup` 的提前返回从 `flag.Parse()` 后第一分支挪到 `--self-test` 校验之后（模拟"先做一点持久化初始化再检查这个 flag"） | AST 结构断言 **和** 真实子进程行为断言（真的创建了 `user-data/` 目录）同时独立 FAIL |
| 7 | `execute_command.go` `executeWarmEnvironment` | 去掉 `NODE_OPTIONS`/`NODE_V8_COVERAGE` 等危险环境变量过滤 | 同 1 号测试的另一部分断言 FAIL |

每处变异都单独插入、单独跑测试、单独回滚，回滚后复跑基线确认真的恢复绿。
5 号和 6 号两处都是"同一个变异被两条独立机制分别命中"（AST 静态检查 + 真实子进程/真实
Node 行为），这是比单一护栏更强的信号。

## 关于 R82 补充单遗留的 AST 守卫盲区（本轮的差异说明）

之前在 R82-ADDENDUM 验收里，我亲手证明过 `completion_wiring_test.go` 的 AST 守卫可以被绕过——
因为当时 `--update` 和普通安装两条分支**同时存在**，绕过手法是新增一个旁路函数、
在分支判断里插队调用它。

这一轮我特意检查了同类风险是否也存在：`grep -rn "exec.Command\|exec.CommandContext" installer/*.go`
（排除测试文件）只有三处调用点——真实应用启动（`application_launch.go`，唯一一处）、
执行预热（`execute_command.go`）、拉起 NSIS 本体（`install_windows.go`）——**真实应用启动
只有这一条路径，没有第二个分支可以插队**。R82 那次能绕过是因为存在两条并行分支，这一轮
在拓扑上就不具备同样的绕过条件。这不代表"以后加代码也不会有事"，只是如实记录：
本轮没有找到、也没能人为构造出类似的绕过。

## 关键行为核实（非变异，直接实测）

`--startup-warmup` 与 `--self-test` 在同一台沙箱机器上对照：

| | `--startup-warmup` | `--self-test` |
|---|---|---|
| 耗时 | 21ms | 需要先构建奖池等状态 |
| 产生的文件/目录 | 无 | `session-token`、`account-salt`、`logs/diagnostics.jsonl`、`snapshots/` 等一整棵目录 |
| 退出码 | 0 | 0 |

证实了 GPT 报告里"不适合复用 `--self-test` 做预热"这个判断是对的——如果当初真的复用了它，
每次安装都会真实生成/轮换账号盐和会话令牌。

## Windows 交叉编译

```
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...      # installer/，0 输出
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build . 	   # installer/，exit 0
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c .      # installer/，exit 0，仅生成测试二进制，未执行
```

## Node/Go 回归

```
go test -race ./...                                        # installer/：3 个包全绿
go test -race .                                             # 根模块：TestStartupWarmup* 4 项全绿
node --test scripts/r82-startup-ab-report.test.cjs           # 6/6
node --test desktop/startup-sequence.test.cjs desktop/startup-visibility.test.cjs   # 8/8（R77/R82 保护的时序未受影响）
```

## .gitignore 白名单核实

```
git check-ignore -v output/r82-ab.json output/r83-startup/r82-ab.json output/r83-startup/DeepLegendsSetup-startup.log
# 三行都命中 .gitignore 里的 `!` 反向规则（未被忽略）
git status --short output/r82-ab.json output/r83-startup/
# ?? ??  → 未跟踪但未被忽略，可正常 git add
```

## 未验证 / 下一步

**必须由真机完成，本轮任何沙箱测试都替代不了**：GPT 构建一份新安装包（新指纹），
用户按 R83 工单 C 节的三步——正常装一次、找到 `%TEMP%\DeepLegendsSetup-startup.log`、
发回来。判定标准：`application launch ... elapsed_ms=` 从当前的 3000~3900 掉到
1000 以下算成功；1000~2500 之间算部分改善；仍高于 2500 说明这条路径没解决主要问题，
需要回到工单 0.4 节的线索重新想。

**遗留、不阻塞本轮结论的旧账**：`WORKLIST-R82-ADDENDUM-3-NODE-COMPAT.md` 仍未执行
（`.at()` 兼容性、PowerShell 组测满预检查），因为该脚本已经不在新验证流程的路径上，
优先级由用户决定要不要继续追。
