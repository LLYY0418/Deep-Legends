# R83 安装启动第二阶段验收报告

日期：2026-09-12。依据 `WORKLIST-R83-STARTUP-PHASE2.md` 执行 A、B、C；D 为工单允许不做的 P2，本轮不新增绝对时钟字段。没有运行真实 Setup 打包或 Windows 安装，没有修改安装页面和升级图标。

**本报告保留原阶段的历史实现和测试记录。** 补充单 2 已撤回暂存执行方案；最新补充单 3 将最终目录执行预热默认关闭，仅在 `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 时开启实验路径。读预热继续默认开启，无论执行开关及结果如何，都读取两个最终 EXE。执行预热的额外收益与首次启动故障的归因均未证实。当前实现、两份日志的交付方式及以 `spawn_to_ready` 为主的判据统一见 [执行预热默认关闭验收报告](ACCEPTANCE-REPORT-R83-READ-ONLY-DEFAULT.md)。

## 压缩级别：按用户要求恢复为 9

R83 交付后，用户明确优先控制安装包体积，默认压缩级别已从 3 恢复为 **9**。仅构建 Setup 和 R83 启动优化继续保留；本次调整未打包。以下说明保留对原问题的回答。

7z/LZMA 的这些级别都是无损压缩。调整的是压缩耗时与体积，不降低图片、字体或程序质量，也不改变文件清单。解压后的字节应与原始文件一致。当前仍保留 NSIS 的 7z 格式、安装期兼容 BCJ 过滤器、运行时文件检查、后端指纹及哈希验证。新包可能增大；本轮未打包，因此没有新包体积或真实安装的测量值。

## A：默认开启与可追溯证据

- `startupPrewarmEnabled` 改为仅精确值 `"0"` 关闭；未设置、空值、`"1"`、`"true"`、`"yes"`、空格等全部开启。
- 注释记录六次真机数据：total 中位数 3849→3471ms、installed_to_visible 5401→4948ms、读预热自身 46ms、三次 failures=0/timed_out=false，spawn_to_ready −507ms、splash_window_shown −530ms。
- 已实际运行原报告脚本读取用户补齐的 `output/r82-ab.json`，结果 `total_gain_ms: 378`。
- 原始 JSON 与 `output/r83-startup/r82-ab.json` 的 SHA-256 均为 `a9c13803bce9acde03f7dc2aea23da62b5c091ddfbe72340868795dd611eac76`。
- `output/r83-startup/DeepLegendsSetup-startup.log` 保留用户提供的 196 行日志；原始文件未改写。
- `.gitignore` 按既有白名单模式放行上述三个文件，包括原本被全局 `*.log` 忽略的日志。文件可被 Git 纳入版本控制，本轮未创建提交。
- 旧 control 分支仍明确设置 `'0'`，未删除。后续交付验证不要求用户运行该实验协议。

工单从秒级时间戳估算的数秒启动等待，是本轮优化和新增直接计时的依据；本轮不把执行预热的收益当成已经实测的事实。

## B：执行预热实现

| 项目 | 实现与边界 |
|---|---|
| 主程序 | `Deep Legends.exe -e "process.exit(0)"`，仅该子进程设置 `ELECTRON_RUN_AS_NODE=1`，禁止控制台窗口 |
| 后端 | 使用新 `loot-service.exe --startup-warmup`，解析参数后立即退出，位于存储、网络和服务初始化之前 |
| 不复用 self-test | 核查发现原 `--self-test` 会先创建数据目录、盐、会话 token、诊断记录，且可能触发配置的 feature-gates 请求；不适合执行预热 |
| 环境隔离 | 为子进程分配独立环境切片；真实应用启动按大小写不敏感规则删除 `ELECTRON_RUN_AS_NODE`；预热额外清理 Node preload/inspector/coverage 与日志输出相关环境变量 |
| 触发时机 | 接入现有 250ms NSIS 进度轮询，在完成信号尚未到达时检查并触发两个独立 goroutine |
| 文件判定 | 非空普通文件、只读打开成功、大小/修改时间/文件身份一致且稳定至少 400ms；变化即重置稳定计时 |
| 覆盖安装 | 初始存在且尚未变化的旧 EXE 不触发执行，避免把旧版本误认为新版本 |
| 改写与替换 | 记录执行时的大小、修改时间和文件身份；运行期间发现改写即取消该预热，NSIS 退出后再次复核，作废者 `voided=true` 并回退读取 |
| 执行预算 | 第一份文件就绪后开始共享 8 秒预算，避免 NSIS 尚未产出文件时提前耗尽预算 |
| 失败与未完成 | 启动失败、非零退出、超时、缺失、作废或 NSIS 完成时仍在运行的目标均回退读取；不向安装完成策略返回错误，不弹窗，不等待预热进程结束 |
| 进程清理 | `CommandContext` 只取消预热子进程；真实应用启动不用该取消上下文，仅异步回收进程句柄 |
| 完成策略 | 保留 `completion.go` 及现有 AST 守卫结构，普通安装和升级仍走同一个完成/窗口接力入口 |

当前 NSIS 会先在临时 `7z-out` 解压，再复制到最终目录；因此不能承诺能利用全部解压时段。执行预热只针对最终安装目录里的就绪文件。若文件到达太晚，日志会显示 `attempted=false reason=not_ready`，随后使用读预热。实际重叠时间和收益要由一次普通安装日志判断。

`Finish` 会立即取消未完成的执行，不等待可能卡在 `CreateProcess` 的调用；迟到的成功也按未完成处理，优先保证真实启动继续。回退读取沿用原有独立 8 秒上限，成功执行的目标无需再读。进程创建的系统调用本身若不可及时取消，后台回收会在调用返回后继续，但不会被安装完成线程等待。

## 日志与耦合验证

新增每个目标的执行结果：

```text
startup warm mode=execute file=主程序 ready_ms=... exit=... elapsed_ms=... voided=false attempted=true reason=ok error="<nil>"
startup warm mode=execute file=后端 ready_ms=... exit=... elapsed_ms=... voided=false attempted=true reason=ok error="<nil>"
startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=... execute_valid=... fallback_files=...
application launch pid=... error=... elapsed_ms=...
application handoff visible=true elapsed_ms=...
```

- `ready_ms` 从本次安装处理开始计算；执行行的 `elapsed_ms` 是该次执行耗时，未完成时为观察到的已运行时间，退出码未知为 -1。
- `application launch` 的 `elapsed_ms` 从真实接力的 `started` 起算，成功和失败都记录，包含进程创建及很小的调度开销。
- `StartupLog` 使用 `LstdFlags | Lmicroseconds`，明确不设置 `Lmsgprefix`；关闭后恢复原 logger flags。
- 保留 `application handoff visible=... elapsed_ms=...` 的相邻字段，原采集器仍兼容历史秒级日志。新增可复用解析函数读取成功/失败 launch 计时。
- 耦合测试用真实 `StartupLog()` 写文件，使用实际 Windows adapter 的日志格式和真实启动计时函数，再调用真实 Node parser/collector。PID 过滤、微秒、handoff 数值及两条 launch 成功/失败记录都由该测试检查，不是两份仿写正则。

## 自动验证

- 安装器全部 Go 测试通过；`go test -race ./...` 通过。
- Windows/amd64 `go vet ./...` 与安装器测试程序交叉编译、链接检查通过；只生成临时测试程序，未生成 Setup 安装包，也未在本机执行 Windows 程序。
- 根模块 `go vet .` 及 4 项 `TestStartupWarmup*` 测试通过；真实测试子进程进入真实 `main()`，使用不可能成功监听的地址，确认暖启动立即无输出退出、未创建隔离用户目录。早退位置和新增 init 的审查护栏同时验证。
- Node 的既有启动顺序、splash、外壳、A/B collector 回归共 25 项通过。
- `scripts/r83-startup-mutations.py` 在临时副本和 Go overlay 中执行，首轮 **24/24 变异被明确测试断言捕获**；包括四种默认值错误、Node 模式泄漏、失去与 NSIS 的重叠、未稳定就执行、文件改写/替换被忽略、失败被当成功、缺失无回退、等待卡死进程、取消失效、误用 self-test、Windows 轮询未接入、微秒丢失、PID 位置变化、生产日志/消费正则单侧漂移、失败启动不记计时及后端执行到持久初始化。

这些验证确认代码行为与失败边界，不代替 Windows 上执行预热能否把启动耗时降到 1 秒以内的实测。

## C：验证方式已由补充单 2 更新

1. 像平时那样双击安装包，正常装一次，等软件自己打开。（不用跑任何脚本，不用带 `-Group`，不用管指纹，不用先关掉软件。）
2. 找到 `%TEMP%\DeepLegendsSetup-startup.log` 和 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`。
3. 把这两个文件发回来。

无需重新做六次实验，无需先卸载，也无需保持软件关闭。旧的“仅按 launch 判断”和“必须执行完成”的要求已撤回；当前以 `spawn_to_ready` 为主、launch 为参考，`ok` 和 `still_running` 均可接受，详见最新报告。

## D 与保留的约束

本轮未增加 `process.getCreationTime()` 的绝对时钟上报，避免把 epoch 写入有 600000ms 上限的阶段字段。未调整 Electron 后端 spawn 时机、splash 的 show/ready-to-show、100ms/20s 窗口接力参数；没有修改代码签名策略，也不要求杀毒软件白名单。

实现核对资料：[Electron 的 RunAsNode 环境变量](https://www.electronjs.org/docs/latest/api/environment-variables#electron_run_as_node)、[Windows 子进程继承规则](https://learn.microsoft.com/en-us/windows/win32/procthread/inheritance)。Windows 无窗口启动配置保留 Go runtime 已设置的错误模式，不启用 `CREATE_DEFAULT_ERROR_MODE`；本机 Go `runtime/signal_windows.go` 的 `preventErrorDialogs()` 已设置关键错误/崩溃/打开文件错误的静默标志。
