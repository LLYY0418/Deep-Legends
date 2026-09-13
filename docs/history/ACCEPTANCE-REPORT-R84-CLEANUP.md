# R84 收尾验收报告

日期：2026-09-12。依据 `WORKLIST-R84-CLEANUP.md`。A、B、C 已完成代码与自动化验证；D 为工单明确允许不做的可选项，本轮未增加 `desktop.log` 路径显示界面。未打包、未执行 Windows 安装，默认压缩级别仍为 **9**。

## A：旧 Node 的采集兼容性

`scripts/r82-startup-ab-report.cjs` 用 `values[values.length - 1]` 替代两处 `.at(-1)`，保留同一 PID 最后一条 `startup prewarm` 与 `application handoff` 的语义；没有匹配时仍返回 `undefined`。内置模块引用改为 `require("fs")` / `require("path")`，避免早期 Node 14 不支持 CommonJS 的 `node:` 前缀。生产路径其余语法/API 已核对，未发现需要保留的 Node 16.6+ 专属特性，因此没有新增版本拦截。

先新增端到端 CLI 测试，再修改生产代码。在当前 Node 24.14.1 子进程中通过 preload 执行 `delete Array.prototype.at`，修复前实际复现 `[(...lines.matchAll(...))].at is not a function`；修复后成功写出结果 JSON，与正常新版 Node 的结果完全一致。这是禁用 API 的兼容性测试，没有冒充真实 Node 14 执行。

测试同时构造同一 PID 的多条预热和接力记录、其他 PID 的前后干扰记录，断言最终采到 700ms / 2100ms。取首元素的变异被实际 CLI 断言捕获；生产源码静态断言禁止重新出现 `.at(` 调用。旧的日志轮转、指纹/run_id 关联、重复运行拒绝、预算和字段校验继续通过。

## B：安装前拦截当前组已满

`scripts/r82-startup-ab.ps1` 在原指纹去重检查旁新增当前 `$Group` 的计数检查，达到 3 条就直接抛出中文错误，说明当前组及样本数量，提示另一组的 `-Group prewarm` 或 `-Group control` 参数。检查发生在设置预热环境变量与 `Start-Process` 之前。只统计当前组，不把两组相加；保留已有指纹去重与构建收据哈希校验。

使用本机缓存的 PowerShell 7.6.6 运行真实脚本测试，未跳过。测试只在临时副本移除 OS 限制，继续执行真实收据读取、哈希、预检查、Node 采集与结果写入；`Start-Process` 和进程查询由既有 mock 接管，不会运行任何真实安装器。

- 原 8 项场景全部保留，扩充为 **16 项场景**：两组分别覆盖 2 条放行、3/4 条拦截，以及一组满 3 条时另一组为空仍放行。
- 拦截场景检查安装器启动次数为 0、原结果字节不变、环境变量不变、中文错误包含正确的另一组参数。
- 放行场景不仅检查 mock 启动一次，也让真实 Node 采集器完成结果保存。
- 统计两组总数、提前在 2 条拦截、拖到 4 条才拦截、把检查移到 `Start-Process` 之后，这四种新变异均被真实脚本测试捕获。原文件名、哈希、指纹去重的三种变异继续有效。

此项只修复已有历史采集工具，不恢复六次 A/B 协议，也没有运行新的真实采样。

## C：未显示主窗口也有可导出的外壳证据

新增受原本地 token 认证保护的 `POST /api/diagnostics/startup-stage`，只接受 `{stage, elapsedMs}`。阶段名仅允许四个固定枚举，拒绝未知字段、任意文本阶段、错误类型、尾随 JSON 与超过 1KiB 的请求。`elapsedMs` 表示阶段相对外壳首条 JS 的耗时，沿用 0–600000ms 的限制。

后端写入固定事件 `desktop_startup_stage`，含 `stage`、`elapsed_ms`、既有的 `run_id` / `log_seq` / `time`。只有实际落盘成功才返回 204；日志目录不存在或写入失败返回 503，避免“HTTP 成功但日志为空”。没有接收用户路径、凭据或自由文本日志。

`desktop/main.cjs` 在以下位置立即提交事件，每个阶段每次运行只记一次：

| 阶段 | 触发位置 |
|---|---|
| `backend_ready` | 通过后端启动载荷校验，记录 ready 后 |
| `main_window_created` | 主窗口 `BrowserWindow` 构造完成后 |
| `main_window_did_finish_load` | 主窗口首次 `did-finish-load` |
| `main_window_ready_to_show` | 主窗口 `ready-to-show` 回调入口 |

四次轻量请求使用一个连接顺序发送，不等待主窗口成功后再批量发送；传输顺序不阻塞窗口创建或显示。每次请求保留 2 秒超时，失败只写外壳错误日志。原 `reportStartupPhases()` 的成功后发送行为、请求字段以及 `desktop_startup_phases_ms` 的结构保持不变，旧采集器仍使用原摘要计算耗时。

端到端测试使用模拟 Electron 事件驱动真实 `main.cjs`，通过真实 HTTP 请求进入 Go 认证、解码和落盘处理，再请求真实 `/api/diagnostics/log`；下载响应经过主窗口实际注册的 `attachDiagnosticsExport` 选择保存路径，测试将原响应字节写入临时文件并读取内容断言。

- 正常场景逐阶段检查导出文件，四个事件按触发顺序存在，同一 `run_id`、递增 `log_seq`，最终仍有原摘要；把导出内容送进真实 `collectRecord`，原字段集合、`spawn_to_ready=200`、`total=900` 的合成测试值全部匹配。
- 卡在创建后的场景永不触发 `ready-to-show`，导出仍有 `backend_ready` 和 `main_window_created`。
- 卡在加载完成后的场景仍不触发 `ready-to-show`，导出已有前三个阶段；这两个失败场景均没有伪造成功摘要。
- 重载或重复 ready 事件不会重复阶段记录。以同一目录重建存储、开始新 `run_id` 后再次导出，前一次未完成启动的记录仍在。
- “等主窗口显示才发”和“发送后端拒绝的未知字段”两种前端变异，均被真实 HTTP/导出断言捕获。
- `scripts/r84-cleanup-mutations.py` 使用临时 Go overlay，另核验 **3/3 后端变异被捕获**：原 `spawn_to_ready` 改名会让真实采集器报缺少阶段字段；不落盘却返回成功会失败；接受任意字段会失败。没有把编译失败当作变异验收。

以上是带真实 HTTP、磁盘和导出的自动化验证，窗口由测试替身提供，没有声称做过 Windows 真机启动，也没有将之前卡死的根因认定为已解决。若实际 Electron 事件顺序不同，日志如实保留实际到达顺序，不伪造尚未发生的阶段。

## 验证与范围边界

- A/B 指定 Node 测试与 `desktop/release-build.test.cjs`：**13 PASS / 0 FAIL / 0 SKIP**，包含 PowerShell 16 项场景与变异执行。
- 外壳启动顺序、splash、诊断下载、网页导出、窗口缩放、升级退出回归：**35 PASS / 0 FAIL / 0 SKIP**。
- `go test -count=1 -timeout=60s -run '^TestDesktopStartup' .` 通过；新增事件、原启动耗时、日志导出/轮转、启动早退的相关 Go 测试在 `-race` 下通过。
- Windows/amd64 `go vet .` 通过；独立 `installer` 模块全量 Go 测试通过，包括既有分派边界守卫和默认关闭执行预热的测试。
- 独立只读复核未发现本轮范围内的实质遗漏。
- `git diff --check` 通过。真实 HTTP 测试首次运行被沙箱禁止监听本机端口拦截；获得回环监听测试权限后重跑通过，该次拦截不计作业务测试失败。

未修改 `installer/message_wiring_test.go`，未开启执行预热，未修改 splash 的 `alwaysOnTop` / `show:false`、后端 spawn 时机或窗口接力参数。没有修改 `app.getName()` / `userData` 路径；原窗口位置和缩放设置继续使用原位置。D 的完整路径展示按可选项暂不增加，C 的关键证据通过现有导出按钮获取即可。
