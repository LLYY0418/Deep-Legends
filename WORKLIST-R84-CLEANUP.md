# WORKLIST-R84 · 收尾：补齐两笔旧账 + 修掉"外壳日志谁也找不到"

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。日期：2026-09-12。

> 背景：启动速度这条线已经收尾（执行预热默认关闭、读预热保留，见
> `WORKLIST-R83-ADDENDUM-3-DISABLE-EXECUTE-WARM.md`）。本单是把剩下几件
> 明确可修的事一次做完。

## 零、先说三件已经确认的现状

1. **R82 的 AST 守卫绕过已经被补上了**（不在本单范围）。我刚刚在隔离副本里
   亲手复现了当初那次绕过手法——新增 `installFast()` 旁路函数、在
   `webview_windows.go` 的 `case "install"` 里插队调用它——
   `installer/message_wiring_test.go` 的
   `TestWindowsInstallMessageCannotBypassCompletion` **当场把它杀掉了**。
   这个守卫改成守"分派边界"（switch 之前的全部语句 + selector + install case
   必须抵达同一个 `go a.install(message)`），而不是守一份函数名白名单，
   这个思路是对的。**本单不要改动它。**
2. **`WORKLIST-R82-ADDENDUM-3-NODE-COMPAT.md` 从未被执行**：
   `scripts/r82-startup-ab-report.cjs:65-66` 的 `.at(-1)` 还在，
   `scripts/r82-startup-ab.ps1` 也仍然没有"该组已测满 3 次"的启动前预检查
   （只有第 38 行的"同一指纹不能重复测"）。见下面 A、B 节。
3. **外壳日志 `desktop.log` 用户找不到，而且这正是那次卡死查不下去的原因。** 见 C 节。

---

## A 节（P1）· `r82-startup-ab-report.cjs` 去掉 `Array.prototype.at`

`.at()` 需要 Node ≥ v16.6.0。这个脚本由 `& $node ...` 用**用户机器上任意版本**的
Node 执行，用户此前就因此报过 `[(...lines.matchAll(...))].at is not a function`
（当时是用户自己升级 Node 绕过去的，脚本本身一直没修）。

把这两处改成不依赖 `.at` 的等价写法（取数组最后一个元素），例如
`const m = [...s.matchAll(re)]; const last = m[m.length - 1];`，
或抽一个 `const last = a => a[a.length - 1]` 复用。写法不重要，只要不再出现 `.at(`。

**顺带检查**：该脚本在用户真机上会执行到的代码路径里还有没有别的 Node 16.6+
专属特性（测试文件 `*.test.cjs` 不算，那些只在开发机跑）。如果还有摘不掉的，
就在脚本开头加一次版本检查，给出人能看懂的中文报错，而不是原生 TypeError 堆栈。

### 验收判据

1. 在**不支持** `Array.prototype.at` 的环境下（例如运行前执行
   `delete Array.prototype.at` 的包装脚本，或真实 Node 14）跑一次端到端采集：
   修复前必须能复现 `.at is not a function`，修复后必须跑通，且"取最后一条匹配"
   的行为与修复前在新版 Node 上完全一致。
2. 新版 Node 上 `scripts/r82-startup-ab-report.test.cjs`、
   `scripts/r82-startup-ab-script.test.cjs`、`desktop/release-build.test.cjs` 继续全绿。

### 变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 改写后取到数组第一个而不是最后一个元素 | ✅（构造同一 pid 下多条 `startup prewarm` / `application handoff` 记录，断言取到最后一条） |
| 重新引入任何 `.at(` 调用 | ✅（对脚本源码做一次静态断言即可） |

## B 节（P1）· `r82-startup-ab.ps1` 增加"该组已测满"的启动前预检查

用户实测踩过：前 3 次都用 `-Group control` 成功采集，第 4 次忘了切换参数、
仍传 `-Group control`，脚本**跑完一整次真实安装、应用都启动了**，才在采集阶段
报 `Use exactly 3 fresh samples per group`。而按本项目既定规则，构建一旦被真正
启动过就永久作废——等于白跑一次安装、烧掉一份一次性构建。

在现有"同一指纹不能重复测"检查（`scripts/r82-startup-ab.ps1:36-39`）旁边，
增加一条**同样在 `Start-Process` 之前**执行的检查：读取 `$Results` 里属于
`$Group` 这一组的记录数，若已达 3 条则直接报错终止，不启动安装器。
文案要说清楚"这个组已经有 3 个有效样本；如果是想测另一组，请改用
`-Group prewarm`（或 `control`）"。

只看当前 `$Group` 这一组的数量，两组各自独立判定。

### 验收判据

1. 构造已有 3 条 `group=control` 的 `results.json`，再以 `-Group control` 调用：
   必须在 `Start-Process` 之前报错终止，**不得启动安装器**。
2. 同一份 `results.json` 改用 `-Group prewarm` 调用：必须正常继续到启动安装器。
3. 只有 2 条 `group=control` 时以 `-Group control` 调用：必须正常继续。

### 变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 预检查统计两组总数而非当前组 | ✅（"control 3 条 + prewarm 0 条"时 `-Group prewarm` 必须放行） |
| 预检查放在 `Start-Process` 之后 | ✅（用现有测试里已有的 `Start-Process` mock，断言已测满时调用次数为 0） |
| 阈值差一（3 条时就拦、或到 4 条才拦） | ✅（边界值 2/3/4 都要覆盖） |

## C 节（P0）· 外壳侧的启动故障必须留下用户拿得到的证据

### 问题

2026-09-12 20:22 那次安装后应用卡死（splash 永久停留、主窗口从未显示，
见 `WORKLIST-R83-ADDENDUM-3-DISABLE-EXECUTE-WARM.md` 0.2 节）。
排查时发现**根本没有可用的外壳侧证据**：

- `desktop/main.cjs:242` 的 `appendDesktopLog()` 写到
  `app.getPath("userData")/logs/desktop.log`。`desktop/package.json` 顶层只有
  `"name": "deep-legends-desktop"`、**没有顶层 `productName`**
  （`Deep Legends` 只写在 `build.productName` 里，那是打包用的，不影响
  Electron 的 `app.getName()`）。所以真实路径是
  `%APPDATA%\deep-legends-desktop\logs\desktop.log`——
  和用户认知里的产品名、以及后端数据目录 `%LOCALAPPDATA%\LOLLootAssistant`
  都对不上，用户按产品名去找必然找不到。
- 更关键：**外壳侧唯一会流进后端诊断流的，是 `reportStartupPhases()`，
  而它只在主窗口成功显示之后才发。** 卡死场景恰恰是"主窗口没显示"，
  所以那条路径永远不会执行——故障越严重，留下的证据越少。
- `desktop/diagnostics-export.cjs` 的应用内"导出诊断日志"只下载后端的
  `/api/diagnostics/log`（即 `diagnostics.jsonl`），**完全不含 `desktop.log`**。

### 要做什么

目标只有一个：**下次再出现"卡在加载中"时，用户用应用内那个导出按钮导出的文件里，
就能看到外壳卡在了哪一步**，不需要去任何隐藏目录翻文件。

1. 在后端 ready 之后、主窗口显示之前的关键节点，各向后端发一条轻量诊断事件
   （复用现有 `/api/diagnostics/startup` 的同类机制，或新增一个只接受固定枚举
   阶段名的端点）。至少要能区分这几步：`backend_ready` →
   `main_window_created` → `main_window_did_finish_load` → `main_window_ready_to_show`。
   这样卡死时，日志里停在哪一步就一目了然。
2. 事件必须**即时发送**，不能攒着等成功了一起发——否则又回到"失败时没证据"。
3. 保持 `desktop_startup_phases_ms` 现有行为与字段不变
   （`scripts/r82-startup-ab-report.cjs` 依赖它，改了会连带打断采集脚本）。

**注意后端侧的既有陷阱**（`desktop_startup.go`）：
`decoder.DisallowUnknownFields()` 会让带未知字段的请求整个被拒；
所有数值字段都过 `clampDesktopStartupPhase()`（上限 600000）。
新增端点/字段必须端到端验证事件真的落盘——R76 踩过
"`logs` 目录不存在时 `appendDiagnostic` 静默失败导致测试假绿"的坑。

### 验收判据

1. 正常启动一次，导出的诊断文件里能看到上述四个阶段事件，顺序正确。
2. **人为模拟主窗口永不显示**（例如在测试里让 `ready-to-show` 永不触发），
   导出的诊断文件里必须能看到停在 `main_window_created` 或
   `main_window_did_finish_load`，而不是一片空白。这条是本节的核心价值，必须有测试。
3. `scripts/r82-startup-ab-report.cjs` 的既有解析全部不受影响，相关测试继续全绿。

### 变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 阶段事件改成攒到成功后一起发 | ✅（模拟卡死场景，断言仍能看到前面的阶段） |
| 新增字段触发 `DisallowUnknownFields` 导致整条请求被拒（埋点静默全丢） | ✅（端到端发一次真实请求，断言返回成功且事件真的落盘） |
| `desktop_startup_phases_ms` 的字段名/结构被顺手改动 | ✅（`r82-startup-ab-report.cjs` 的采集测试必须红） |

## D 节（P2，可不做）· 顺带明确 `desktop.log` 的位置

C 节做完之后，`desktop.log` 就不再是排障的必需品了。但如果顺手，
可以在应用的"设置 → 诊断"之类的位置把这个文件的完整路径显示出来
（直接显示 `app.getPath("userData")` 拼出来的真实路径，不要硬编码字符串，
否则以后改了名字又会对不上）。

**不要**为了让路径"好看"而去改 `app.getName()` 或 `userData` 目录——
那会让已有用户的窗口位置（`window-bounds.json`）和界面缩放设置
（`ui-scale.json`）全部丢失。

## 三、不要做的事

- 不要改动 `installer/message_wiring_test.go`（零节第 1 条：它已经有效）。
- 不要重新启用执行预热，不要恢复六次 A/B 协议。
- 不要改 splash 的 `alwaysOnTop`/`show:false`、后端 spawn 时机、窗口接力参数。
  那次卡死的根因仍未查明，C 节是为了下次能查，不是现在就去改。
- 不要为了 C 节去动 `app.getName()` / `userData` 路径（见 D 节说明）。
