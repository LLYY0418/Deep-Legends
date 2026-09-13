# R83 补充单 3：执行预热默认关闭

日期：2026-09-12。依据 `WORKLIST-R83-ADDENDUM-3-DISABLE-EXECUTE-WARM.md`，完成 A、B、C 的代码、测试和文档调整。未打包、未执行 Windows 安装；默认压缩级别保持 **9**，仍只构建 Setup。

## 故障依据与归因边界

本单提供的真机诊断记录：2026-09-12 20:22、指纹 `d2212835fcb1` 安装后第一次启动一直停在 splash，强制关闭后重开恢复。失败运行 `run_id=d5b2582ee07da8c5bea8fba7` 的 `log_seq` 为连续的 1–58；后端已有连接、概览及心跳记录，但未出现 `desktop_startup_phases_ms`，与用户看到主窗口没有出现一致。该轮执行预热两个进程均为 `still_running`，主程序执行约 298ms；读预热 `files=2 failures=0 timed_out=false elapsed_ms=55`，实际启动记录 `launch=1928ms`、`handoff=3800ms`。

以上为工单提供的原始诊断摘要，本轮没有新增真机测量。之前两个疑似历史故障已被工单复核为日志轮转导致的假阳性；目前只有这一次确认故障。执行预热后的 1/3 与此前约 0/10 的样本不足以证明因果关系。执行预热可能强杀一个刚启动的 `Deep Legends.exe`；若 Node 环境模式未生效，可能影响 Electron 初始化状态，但这只是待验证的机制假设，不能据此声称已找到或修好主窗口不出现的根因。

执行预热的额外收益仍未证实，因此关闭默认执行。保留已有 3+3 对照实验支持的读预热：历史 `spawn_to_ready` 1870→1363ms，`splash_window_shown` 减少约 530ms；归档数据与历史报告继续保留。

## A：独立的显式执行开关

Windows 安装入口调用 `newConfiguredExecutionWarmup`，由实际环境配置决定是否启用原执行管理器。只有精确字符串 `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 能开启执行预热；未设置、空值、`0`、`true`、`yes`、带空格或 `01` 均不启用。原 `DEEP_LEGENDS_STARTUP_PREWARM=0` 仍优先关闭全部预热。

| 安装器环境 | 执行预热 | 安装后的读预热 |
|---|---|---|
| 不设置预热变量 | 关闭 | 两个最终 EXE 各读一次 |
| `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` | 保留原实验行为 | 成功、失败、超时、未结束均各读一次 |
| `DEEP_LEGENDS_STARTUP_PREWARM=0` | 关闭，即使另一个开关为 1 | 关闭，保留原总开关 |

默认执行管理器不检查或调度可执行文件，不创建执行预算上下文、执行结果通道或进程取消回调；预热路径不创建或强杀子进程。安装使用的 NSIS 和最终真实应用仍正常启动。

关闭状态的 `Finish()` 明确输出一次 `startup warm mode=execute enabled=false`，重复收尾不会重复记录；诊断值为 `execute_valid=0 fallback_files=2`。`completeStartupWarmup` 与 Windows 读预热调用保持原样，始终使用两个固定最终路径，诊断值不决定读取范围。正常默认日志格式应包含如下内容（格式说明，不是新增实测）：

```text
startup warm mode=execute enabled=false
startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=<实际值> execute_valid=0 fallback_files=2
```

显式开启后仍执行最终目录的主程序与后端，保留旧文件基线、400ms 稳定判断、大小/mtime/文件身份复核、`voided`、首个稳定文件起算的共享 8 秒预算、后端 `--startup-warmup` 早退、非阻塞 `Finish()` 和失败不阻断安装。没有添加等待执行完成、额外补偿或暂存目录备用执行。

## B、C：保留真实启动隔离，更新现状说明

`applicationEnvironment` 的大小写不敏感过滤不变。`TestExecuteWarmEnvironmentCannotLeakIntoRealApplication` 保留完整测试，并显式清除预热开关，确保默认关闭时仍运行：实际辅助子进程验证执行命令的 Node 环境不会污染父进程，真实应用命令剔除外部继承的 `ELECTRON_RUN_AS_NODE`。

`execute_prewarm.go` 顶部注明故障时间、指纹、主窗口未出现与 splash 长期停留、归因未明及默认关闭原因。README 和三份历史 R83 报告均链接本报告并标明当前状态，历史单次执行样本不作为已证实收益。

未改 splash 的 `alwaysOnTop` / `show:false`、后端正常 spawn 时机、窗口接力参数、安装页面或升级图标；没有恢复六次 A/B 协议。

## 自动验证

- 安装器独立模块 `go test -count=1 -timeout=60s ./...` 和 `go test -race -count=1 -timeout=60s ./...` 均通过。
- Windows/amd64 `go vet ./...` 和安装器测试程序交叉编译、链接通过。仅在 `/tmp/r83-execute-off-link-check.test.exe` 生成测试程序，没有生成 Setup，也没有把交叉编译当作 Windows 实测。
- 启动顺序、splash 显示策略、安装外壳、日志采集与 Setup-only 构建的 Node 回归 **28 PASS / 0 FAIL / 0 SKIP**。默认压缩级别 9 的断言仍通过。
- `scripts/r83-startup-mutations.py` **38/38 变异均被实际测试断言捕获**，不是因为编译失败而通过。保留原 33 项，增加默认开启、显式 `1` 失效、生产配置绕过执行开关、关闭状态日志丢失、读预热依赖执行开关这 5 项。执行成功后跳过读取和真实启动删除 Node 环境过滤的既有变异仍能被捕获。
- 新配置行为测试调用生产配置工厂，覆盖环境变量未设置与非精确值、显式开启后的成功/失败/未结束，以及默认状态下无检查、无执行调度、无进程取消上下文、只记录一次关闭日志、两个最终文件各实际读取一次。已有执行稳定性、文件作废、预算、非阻塞完成、真实子进程隔离和其他读取结果测试继续执行。
- Windows 安装入口与接力入口的 AST 守卫继续锁定配置工厂、最终目录、总开关和无条件双文件读取，避免只测试公共函数却未接入生产路径。
- 独立只读复核未发现实质性遗漏；`git diff --check` 通过。变异过程只改临时副本/Go overlay，没有改写生产源码。

## Windows 验收：一次正常安装

按现有方式自行打包，在没有设置预热环境变量的默认配置下正常安装一次，确认主窗口出现且 splash 关闭。保留同一次运行的 `%TEMP%\DeepLegendsSetup-startup.log` 和 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`，按指纹、时间与 `run_id` 对齐。

- 安装器日志必须有执行关闭状态，不得出现 `mode=execute ... attempted=true`；读预热正常仍为 `files=2`。
- `spawn_to_ready` 目标回到只读预热基线约 **1350–1400ms**，1800ms 以上仍需排查；这是待真机验证的目标，本轮未声称已达到。
- `application launch elapsed_ms` 仅作参考。一次成功不足以证明偶发主窗口故障彻底消失；若再发生，继续根据完整诊断日志定位，不能直接归咎于执行预热。

显式 `=1` 的兼容行为由自动化测试覆盖，日常验收使用默认配置，不要求重新做六次 A/B 或多指纹构建。
