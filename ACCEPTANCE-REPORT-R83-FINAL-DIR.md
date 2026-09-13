# R83 补充单 2：最终目录执行与完整读预热

**现状更新：补充单 3 已将执行预热默认关闭，只有 `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 才启用；读预热继续默认开启并读取两个最终 EXE。** 以下为补充单 2 的历史交付记录，其中执行代码保留为实验路径，额外收益尚未证实。当前配置、故障记录和用户验收方式以 [执行预热默认关闭验收报告](ACCEPTANCE-REPORT-R83-READ-ONLY-DEFAULT.md) 为准。

日期：2026-09-12。依据 `WORKLIST-R83-ADDENDUM-2-REVERT-TO-FINAL-DIR.md`。本轮完成 A、B、C；D 为工单允许不做的项目，没有增加主程序启动优化。未打包、未运行 Windows 安装，默认压缩级别保持 **9**。

## A：恢复最终目录，删除暂存执行方案

- `newExecutionWarmup(dest, ...)` 只从 `startupPrewarmPaths(dest)` 得到主程序及后端路径，实际命令的 EXE 与工作目录均指向最终安装位置。
- 恢复构造时的旧文件基线：覆盖安装中尚未变化的旧 EXE 不触发执行；变化后重新观察，至少稳定 400ms 才开始。
- 保留大小、mtime、文件身份及只读打开检查；执行期间和收尾都复核最终文件，改写、替换或消失仍记 `voided=true`。
- 保留首个文件定稳后起算的共享 8 秒预算、并行执行、子进程环境隔离、后端无副作用早退、失败/超时不影响安装，以及不等待执行进程的 `Finish()`。
- 删除暂存发现、跨目录 SHA-256、50ms 最终副本后台校验、暂存删除重试及其专用文件/测试；无备用分支、无开关。删除 `source=staging`、`discovered_ms`、`final_verified`、`source_removed` 日志字段。
- NSIS 本身仍需暂存目录解压。原 `extractedBytes()` 留在 Windows 安装适配器中，仅供进度统计，不连接执行预热。

## B：执行结果不能替代两个文件的读取

新增小型公共函数 `completeStartupWarmup`，供 Windows 接力入口和行为测试共同调用：先收集执行结果，再固定调用 `runStartupPrewarm(startupPrewarmPaths(directory), ...)`。不使用执行结果中的路径列表决定读取范围。

| 执行状态 | 最终 EXE 读取范围 | 诊断字段 |
|---|---|---|
| 两个执行均完成且有效 | 主程序、后端都读 | `execute_valid=2 fallback_files=0` |
| 一个完成有效、另一个未完成或失败 | 主程序、后端都读 | `execute_valid=1 fallback_files=1` |
| 两个都未完成、失败、超时或未开始 | 主程序、后端都读 | `execute_valid=0 fallback_files=2` |

上述为代码/自动化场景语义，不是本轮伪造的真机日志。`files` 只统计实际读预热处理的文件数，不再加执行成功数；正常应为 2，不会变成 4。`failures` / `timed_out` / `elapsed_ms` 保留原读预热含义；缺失或读取报错仍计失败并继续下一文件，保留原读取预算。`fallback_files` 现在只是未完成有效执行的数量，不能解释成“实际只读了这些文件”。

默认开启行为保留；显式 `DEEP_LEGENDS_STARTUP_PREWARM=0` 仍关闭全部预热。这里的“无条件读两个”是指启用预热时不受执行结果影响，不删除 control 开关。

## C：still_running 是状态，不是补偿条件

`reason=still_running`、`attempted`、`ready_ms`、`exit`、执行耗时和 `voided` 继续如实记录；该分支只设置状态，不新增读取、重试或等待。所有执行状态都走同一份双文件读取。

`execute_prewarm.go` 顶部记录工单提供的 18:18 样本：最终目录后端执行仅持续 12ms，`spawn_to_ready` 仍为 444ms，对比读预热基线 1363ms。注释说明工单的机制解释——对最终文件发起执行即可触发扫描，进程是否跑完不作为收益前提；同时区分这份观察与尚未直接测量的 Windows 缓存内部机制。

本轮不把一个 `still_running`、一个 `execute_valid=0` 或一个 launch 数值单独判为实现失败/成功。工单所列暂存运行 `spawn_to_ready=2118` 与“成功执行后跳过后端读取”的代码路径，是这次撤回和修复读取条件的依据；本轮没有新增真机测量值。

## 自动验证

- 安装器全部 Go 测试通过，`go test -race -count=1 -timeout=60s ./...` 通过。
- Windows/amd64 `go vet ./...` 及安装器测试程序交叉编译、链接检查通过。只在 `/tmp` 生成测试程序，未生成 Setup，也未把交叉编译当作 Windows 实测。
- Node 的启动顺序、splash、外壳、日志采集、Setup-only 构建回归 **28 PASS / 0 FAIL / 0 SKIP**，默认压缩参数仍是 `-mx=9 -mf=BCJ`。
- 更新后的 `scripts/r83-startup-mutations.py` **33/33 变异被实际测试断言捕获**。覆盖“成功执行后跳过读取”“整体删除读取”“生产入口绕过读取”“目标重定向到临时目录”“旧 EXE 被当成新文件”“削弱改写/替换复核”“Finish 等待未结束进程”“still_running 触发额外补偿”和日志计数错误等。
- 检索确认生产 Go 代码不含暂存执行分支及 `source=staging` / `discovered_ms` / `final_verified` / `source_removed`；这些字符串只在防回归测试和已标记撤回的历史文档中保留。`git diff --check` 通过。

覆盖的回归场景包括：执行全部成功、部分成功、失败、未完成、超时、文件作废、缺失、未调度、没有执行管理器，以及首次读取报错。每种场景都要求两个最终路径各读一次；执行成功不能减少读取，失败不能多读或等待执行。Windows 适配器由 AST 契约锁定真实调用入口与日志参数，防止只测公共函数却没接入生产代码。

旧的 `completion.go` 及完成策略 AST 守卫结构未改；保留 splash、后端正常 spawn 时机、窗口接力 100ms/20s 参数和签名策略。历史验收报告已标记撤回并链接本报告，不再提供“必须执行完成”或“仅按 launch 判断”的旧操作要求。

## 原补充单 2 用户验证（历史判据，已被补充单 3 替代）

本节保留当时的执行预热验证目标，不是当前默认配置的操作要求；当前只读预热基线目标约为 1350–1400ms，默认应出现执行关闭日志。请按上方最新报告验收。

1. 像平时那样双击安装包，正常装一次，等软件自己打开。不用运行脚本、不用 `-Group`、不用另做多指纹构建、不用先卸载或保持软件关闭。
2. 文件资源管理器地址栏输入 `%TEMP%`，找到 `DeepLegendsSetup-startup.log`；再输入 `%LOCALAPPDATA%\LOLLootAssistant\logs`，找到 `diagnostics.jsonl`。
3. 把这两个文件发回来。

按同一次运行的指纹、时间和应用 `run_id` 对齐两份日志，两个指标一起看：

- **主判据 `spawn_to_ready`**：应明显低于读预热基线 1363ms；18:18 样本 444ms 作为已有观察参考，不承诺复现该数值。1300–1400ms 表示未显示出额外执行预热收益；1800ms 以上按工单视作需要排查的回归信号。
- **安装器记录**：应有两条 `mode=execute` 和 `execute_valid` / `fallback_files`；正常读取计数为 `files=2`，不再出现 `source=staging`。`ok` 与 `still_running` 都可以接受，不能把未结束自动当成需要修复的问题。
- **参考指标 `application launch elapsed_ms`**：跨运行波动较大，单次不能独立下结论。本轮不以 1000/2500ms 阈值单独判定，也不为此延长执行窗口。

当前为代码和自动化验证交付，Windows 真机效果待上述两份日志确认，不恢复六次 A/B。
