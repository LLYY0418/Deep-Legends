# R82 启动补充加固完成报告

日期：2026-09-12。对应 `WORKLIST-R82-STARTUP-HARDENING-ADDENDUM.md`。

A、B、C、D 已完成；E 在限定时间内完成复现尝试，100 轮有效重复未复现，按工单暂不修改。没有打包、发布或变更版本号；页面设计、升级图标、20 秒接力、后端启动顺序和预热默认关闭的行为保持不变。

## A：升级成功必须走窗口接力——已完成

`installer/completion.go:20` 的 `completeInstallation` 接收真实 `installerOptions`、NSIS 退出结果以及操作 hooks。安装失败只报错；普通安装及 `--update` 成功后统一清理临时文件、进入 Handoff。

`installer/install_windows.go` 的 NSIS 完成分支直接调用上述函数，不再在 Windows 文件中决定成功后是否接力。

- `TestInstallationSuccessAlwaysRunsHandoffIncludingUpdate` 从参数解析开始，分别覆盖普通/升级 × 窗口可见/等待超时四种情况，实际执行 `completeInstallation → runApplicationHandoff`，断言成功后进入接力、传递启动 PID、在窗口出现/超时前保持安装器打开。
- `TestInstallationFailureNeverLaunchesApplication` 覆盖退出码非零及 Wait 错误，避免抽取时改变失败行为。
- `TestWindowsInstallCompletionCannotBypassPortableUpdateHandoff` 在 Unix 上直接解析 Windows 源码中的完成分支，锁定它调用平台无关函数，并绑定实际的 `a.handoffApplication` 回调。插入 Windows 专有的提前返回也会触发测试失败。

## B：退出只关闭自身，不终止应用——已完成

`installer/completion.go:42` 的 `finishInstallerHandoff` 统一负责记录结果、调度到 UI 线程、解除忙碌状态、关闭自有 HWND。它只持有安装器窗口及这些操作 hooks，不接收新应用的进程句柄，也没有终止 PID 的操作能力。成功与超时使用同一个退出流程。

- `TestInstallerExitOnlyClosesItsWindowAndKeepsChildAlive` 启动真实的测试子进程；分别执行成功和超时退出流程，断言关闭目标仅为自有 HWND、日志先于调度、关闭发生在 UI 回调中，并通过 `ping → alive` 验证子进程仍能响应。
- 测试结束时只清理它自行创建的 helper 进程，不操作真实 Deep Legends。
- `TestWindowsHandoffExitAdapterCannotTerminateApplication` 直接解析 Windows 的 `CloseInstaller` 回调，限制该适配器只接入上述退出流程及自身 `WM_CLOSE`。真实注入 `taskkill /F /IM "Deep Legends.exe"` 的 Windows 源码变异在宿主测试中被抓到。

两层证据的边界明确：行为测试执行真实的平台无关代码；AST 守卫检查 Windows 适配关系，不声称在 macOS 执行了 Win32 API。合法修改这两个适配边界时需要同步复核守卫。

## C：零外部调用的旧代码——已删除

删除前按函数名重新执行全仓库检索：

```text
rg -n 'runFallback|runInstallerFallback|shellOpen' --glob '!package-lock.json' --glob '!go.sum' --glob '!AGENTS.md'

installer/install_windows.go:227: func runFallback() error
installer/install_windows.go:237: func runInstallerFallback(options installerOptions) error
installer/install_windows.go:239: return runFallback()
installer/install_windows.go:268: shellOpen(...)
installer/dialog_windows.go:133: func shellOpen(exe, directory string) error
```

除此之外只有本补充工单和 Claude 验收文档中的诊断描述，没有脚本调用、专属测试或隐藏入口。三者形成封闭的死调用链；并非“每个函数单独都无引用”，而是没有任何来自该链之外的调用。

已删除三个函数及专用的 `unsafe` import。删除后在 `installer/`、`scripts/` 的 Go/JS/PowerShell/Python 源码检索三个名称，零命中（`rg` 退出码 1）。全仓库剩余命中是诊断/完成报告文字。

README 曾错误描述“缺少 WebView2 自动回退旧安装界面”，已同步修正为当前既有错误提示行为，并校正安装后等待应用窗口的说明；没有重新接入任何旧流程。根模块和 installer 全量 Go 测试通过。

## D：模板与升级文案替换耦合——已完成

`installer/update_test.go:35` 新增 `TestUpdateCompletionWordingStaysCoupledToTemplate`：

1. 读取实际嵌入的 `ui/installer.html`，断言原始目标文案恰好出现一次。
2. 执行真实的普通安装渲染，确认仍有安装文案、没有升级文案。
3. 执行真实 `renderInstallerUI(..., installerOptions{Update:true})`，确认原始文案消失、升级文案存在。

没有改页面措辞或替换行为。本条两个指定变异均实际触发新测试失败。

## A、B、D 变异执行记录

命令：`python3 scripts/r82-hardening-mutations.py`。脚本复制 installer 到临时目录，每个变异后恢复；移除 `GOOS/GOARCH` 覆盖，在宿主执行 Go 测试。要求原始版本通过、变异退出码非零且存在真实 `--- FAIL:` 断言，编译错误不算杀死。

| 变异 | 退出码 | 实际失败测试 |
|---|---:|---|
| A：平台无关成功路径在 `options.Update` 时直接返回 | 1 | `TestInstallationSuccessAlwaysRunsHandoffIncludingUpdate` 的升级成功/超时两项 |
| A：Windows 的 NSIS 完成分支在升级时提前返回 | 1 | `TestWindowsInstallCompletionCannotBypassPortableUpdateHandoff` |
| B：退出函数把关闭目标换成另一个 HWND | 1 | `TestInstallerExitOnlyClosesItsWindowAndKeepsChildAlive` |
| B：Windows `CloseInstaller` 插入 taskkill 终止应用 | 1 | `TestWindowsHandoffExitAdapterCannotTerminateApplication` |
| D：HTML 原文添加“了”，与替换目标失联 | 1 | `TestUpdateCompletionWordingStaysCoupledToTemplate` |
| D：升级渲染直接返回未替换的原文 | 1 | `TestUpdateCompletionWordingStaysCoupledToTemplate` |

原始版本 PASS，6/6 变异被杀死；独立复核再次执行也得到相同结果。[完整记录](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/mutations.log)

本机实际执行平台为 **macOS darwin/arm64**，未使用 `GOOS=windows` 运行这些断言。测试不含 Windows build constraint，Linux amd64 测试程序也已成功交叉编译；当前没有可运行的 Linux 宿主（Docker daemon 未启动），因此**不把本次结果写成“Linux 真机已跑”**。在 Linux 上可直接运行同一脚本，无需 Windows 或 Win32 API。

## E：WebView host 偶发失败——限时复现未成功，暂不处理

初次探索时遇到依赖下载地址 `goproxy.cn` DNS 失败，定位于 `TestCOMSlotsMatchCompleteDependencyInterfaces` 调用 `go list -m` 获取 pinned dependency 路径。该环境错误稳定遮蔽测试，不能当成运行时 flake 复现，也没有据此改动 WebView 代码。

主线程随后明确使用仓库已有依赖缓存，设置 `GOPROXY=off`，在 30 分钟限制内完成：

| 条件 | 重复次数 | 包执行耗时 | 结果 |
|---|---:|---:|---|
| 普通负载 `go test -count=50 -timeout=120s ./internal/webviewhost/...` | 50 | 3.143s | PASS，退出码 0 |
| 自建 4 个 CPU 压力进程，`GOMAXPROCS=2`，`go test -race -count=50 -timeout=120s ./internal/webviewhost/...` | 50 | 6.614s | PASS，退出码 0 |

全部压力进程在 finally 中清理。**已尝试有效重复 100 次未复现，记为已知 flake，暂不处理。** 此结论仅说明本次无法复现，不证明问题不存在，也不将之前的 DNS 失败计入这 100 次通过记录。

[普通重复日志](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/webviewhost-baseline.log)、[压力与 race 日志](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/webviewhost-pressure.log)

## 全量回归结果

| 检查 | 结果 |
|---|---|
| 根模块 `go test -count=1 ./...` | PASS，72.072s |
| installer `go test -race -count=1 ./...` | 全部 PASS |
| `node --test desktop/*.test.cjs web/*.test.cjs scripts/*.test.cjs` | **565 PASS / 0 FAIL / 0 SKIP** |
| installer Windows amd64 `go vet ./...` | PASS |
| installer Windows amd64 / Linux amd64 `go test -c` | PASS；仅测试程序交叉编译 |
| 原 R82 `scripts/r82-mutations.py` | 原始 PASS，4/4 变异被杀死 |
| 本补充单 `scripts/r82-hardening-mutations.py` | 原始 PASS，6/6 变异被杀死 |
| 改动文件 `gofmt`、`git diff --check` | PASS |

根模块首次受沙箱禁止本机回环端口监听而在 `httptest.NewServer` 处退出，之后经工具许可在沙箱外重跑全部测试通过；没有调整或跳过测试来获得通过。

[根模块日志](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/root-go.log)、[installer 日志](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/installer-go.log)、[全部 Node 日志](/Users/ly/personal/personal-work/deep-legends/output/r82-hardening/node.log)

原 R82 的 Windows 画面连续性、首次预热 A/B 数据与 Win32 交互桌面实测仍属于原单真机待办；本补充单没有冒充完成这些项目，预热继续默认关闭。

## 独立验收后的补修：安装入口旁路守卫

日期：2026-09-12。对应 `ACCEPTANCE-CLAUDE-R82-ADDENDUM.md` 的 A/B 遗漏。

独立验收指出的范围缺口已复现并补齐：原先只检查 `install` 和 `handoffApplication` 内部，无法发现调用者新增一条完全不进入这两个函数的路径。当前生产代码本身没有该旁路，本轮只修改测试、变异脚本和本报告，没有改安装页面、升级图标、安装/接力运行逻辑，也没有打包或发布。

### 修复前复现

在临时 installer 副本新增 Windows 辅助函数 `installFast`，对安装器 HWND 发送 `WM_CLOSE`；在 `onMessage` 的 `case "install"` 前置 `if a.options.Update { go installFast(a); return }`。

| 检查 | 修复前实际结果 |
|---|---|
| 原始 installer `go test -count=1 -timeout=30s ./...` | PASS |
| 新增旁路后的同一套宿主测试 | 仍然 PASS，确认旧守卫遗漏 |
| 新增旁路后的 Windows amd64 `go vet ./...` | exit 0，变异代码有效 |

[修复前复现日志](/Users/ly/personal/personal-work/deep-legends/output/r82-entry-guard/before.log)

### 新增检查及覆盖边界

`installer/message_wiring_test.go` 新增三条在宿主直接执行的 Windows 源码契约：

1. `TestWindowsInstallMessageCannotBypassCompletion`：检查 `onMessage` 的整个分发前缀、switch 选择器和完整 `install` 分支。校验失败可以返回；每个被接受的普通/升级请求必须走同一条顶层 `go a.install(message)`。因此新增辅助函数、在分支前截流、把调用改为仅普通模式执行，都会失败。
2. `TestWindowsInstallMessageCallbackUsesSharedDispatcher`：锁定 WebView 消息回调经过 UI dispatch 进入 `a.onMessage(raw)`，防止把分支内旁路挪到原始消息入口。
3. `TestWindowsInstallAutoUpdateUsesSharedDispatcher`：锁定页面 ready 回调中的自动升级仍投递同一条 install 消息，防止首次自动升级绕开共同入口。

这些检查与原有完成/退出适配器检查、真实平台无关行为测试连接起来。没有按 `installFast` 名称建立黑名单，也没有禁止新增未使用的辅助函数。它们是指定入口边界的静态契约，不是全程序不变量的形式证明；合法调整这些入口时需要同步复核契约与行为测试。

### 变异实际结果

命令：`GOPROXY=off python3 scripts/r82-hardening-mutations.py`。

原有六项 A/B/D 变异仍全部失败在测试断言；新增七项如下：

| 新增变异 | 实际失败测试 | Windows vet |
|---|---|---|
| 同文件新增 helper，并在 install 的 update 分支直接调用 | `TestWindowsInstallMessageCannotBypassCompletion` | exit 0 |
| 新文件新增 helper，并在 install 的 update 分支直接调用 | 同上 | exit 0 |
| `onMessage` 在 switch 之前截流升级 install 消息 | 同上 | exit 0 |
| WebView 原始消息回调直接调用旁路 | `TestWindowsInstallMessageCallbackUsesSharedDispatcher` | exit 0 |
| ready 回调的自动升级直接调用旁路 | `TestWindowsInstallAutoUpdateUsesSharedDispatcher` | exit 0 |
| 安装调用只在普通安装模式执行 | `TestWindowsInstallMessageCannotBypassCompletion` | exit 0 |
| 新增升级 helper 先终止应用，再关闭安装器 | 同上 | exit 0 |

**13/13 变异被拦截。** 每项宿主测试退出码为 1 且有真实 `--- FAIL:`；新增七项还要求出现指定入口测试的失败，编译失败或无关测试失败不计数。Windows vet 只检查代码有效性，不运行变异中的进程操作。

原始基线、只新增未调用 helper 的反向对照、全部变异恢复后的基线均 PASS。脚本只写临时副本，新增 helper 和调用者同为 Windows 文件；宿主不编译这两个文件，而由测试使用 `go/parser` 读取调用者源码。因此不会因宿主缺少 `installFast` 定义而制造假失败。

[完整变异记录](/Users/ly/personal/personal-work/deep-legends/output/r82-entry-guard/mutations.log)

### 本轮回归及保留事项

- installer `go test -race -count=1 ./...` 全部 PASS，涵盖原有接力、退出后真实子进程存活、升级文案和 WebView host 测试。[日志](/Users/ly/personal/personal-work/deep-legends/output/r82-entry-guard/installer-go.log)
- Windows amd64、Linux amd64 的 installer 测试程序交叉编译均 exit 0；输出只写入临时目录并已清理，没有生成发行安装包。[日志](/Users/ly/personal/personal-work/deep-legends/output/r82-entry-guard/cross-compile.log)
- 改动的 Go 测试通过 `gofmt` 检查，变异脚本语法及本轮改动文件的空白检查通过。
- 本轮仅补充 A/B 的入口覆盖；C 死代码清理、D 文案耦合沿用独立验收通过结论，D 的原有两项变异也已在本轮重跑通过。
- E 沿用已知偶发问题、此前重复未复现的记录，没有因本轮通过而声称 flake 不存在。
- 本机执行平台仍是 macOS darwin/arm64，未冒充 Linux 或 Windows 真机执行。Windows 安装交互及原 R82 预热 A/B 待办状态不变。
