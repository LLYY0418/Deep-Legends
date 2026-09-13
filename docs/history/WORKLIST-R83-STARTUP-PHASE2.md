# WORKLIST-R83 · 安装后 5.5 秒的真凶已锁定：CreateProcessW 独占 3~3.9 秒

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。
数据来源：用户真机 6 次冷装实测 `output/r82-ab.json` + 真机
`%TEMP%\DeepLegendsSetup-startup.log`（六次运行全在同一个文件里）。日期：2026-09-12。

> **本单相对上一版的最大变化：取消后续所有 A/B 实验。**
> 用户明确反馈"不要再让我去测试了，或者测试流程简单一点，太麻烦等待时间太长"。
> 下面 C 节给出的新验证方式是：**普通装一次软件，把一个日志文件发回来。**
> 不再需要 `r82-startup-ab.ps1`、不再需要 `-Group`、不再需要连续六份全新构建。
> 理由见 C 节：这一轮要打的是 2000ms 量级的效果，而实测run-to-run 噪声只有 24~30ms，
> 一次安装就足以判定，不需要统计。

---

## 零、结论先行

### 0.1 预热 A/B：通过（378ms），建议默认开启

`node scripts/r82-startup-ab-report.cjs output/r82-ab.json` → `total_gain_ms: 378`，
过 300ms 门槛，三次 `failures=0 timed_out=false`，预热自身仅 46ms。
中位数：`total` 3849→3471，`installed_to_visible` 5401→4948。

两组虽然在时间上没有交替（control 全在 16:29–16:41，prewarm 全在 16:57–17:14），
但时间漂移这个混杂因素可以排除：预热只读两个文件，而收益**恰好只落在机制指向的
两处**（`spawn_to_ready` −507 对应 `loot-service.exe`；`splash_window_shown` −530 对应
`Deep Legends.exe`，因为 Electron 的 GPU/渲染子进程是用同一个 exe 再次 CreateProcess
拉起的），其余阶段（`process_to_js` 1410→1450、`js_to_ready`、`ready_to_splash`、
`ready_to_window`）反而全都略微变差。漂移会让所有阶段一起变快，解释不了这个分布。
两组 total 取值范围完全不重叠（[3755,3912] vs [3456,3498]）。

**所以不需要复测。**

### 0.2 真凶：`command.Start()` 一个调用吃掉 3.0~3.9 秒

`installer/handoff_windows.go:49` 的 `startApplication()` 里就是
`exec.Command(exe).Start()` → `CreateProcessW`。

真机日志只有秒级时间戳（`installer/internal/webviewhost/startup.go:134` 只调了
`log.SetOutput`，没设 `log.Lmicroseconds`），但配合 `handoff` 行自带的 `elapsed_ms`
可以做严格的区间约束反推。设 `started`（`handoff_windows.go:75`）真实时刻为 t，
`prewarm` 行紧邻其前写出，故 `floor(t)` = prewarm 行的秒数；
`floor(t + elapsed_handoff)` = handoff 行的秒数；`floor(t + elapsed_launch)` =
`application launch` 行的秒数。六次的解集：

| 运行 | `Start()` 耗时约束 |
|---|---|
| control 974ec9e7284f | [2.401, 4.000) 秒 |
| control 9e980bfbd6c7 | [3.000, 4.401) 秒 |
| control 796786e3e13b | [2.300, 4.000) 秒 |
| prewarm 022674c1afe5 | [2.000, 3.901) 秒 |
| prewarm 7f7ad7aa95e8 | [2.000, 3.902) 秒 |
| prewarm 606ed957dc9c | [3.000, 4.901) 秒 |
| **六次交集** | **[3.000, 3.901) 秒** |

和上一版用另一套方法（`handoff_ms − splash_window_shown`）推出的
"应用进程在 started 之后约 2.03~2.08 秒才被 OS 创建"完全自洽：
进程在 ~2.06 秒被创建，`Start()` 在 ~3.0~3.9 秒才返回。

**两条独立的推算路径指向同一件事：整个等待的最大单块，是拉起应用这一个系统调用本身。**

### 0.3 用户完整等待分解（prewarm 组中位数，合计约 5.5 秒）

| 段 | 耗时 | 占比 |
|---|---:|---:|
| 读文件预热 | 46ms | 0.8% |
| **CreateProcessW（其中 ~2.06s 后进程才被创建）** | **~3000ms 起** | **最大单块** |
| process_to_js（Electron 自身冷启动） | 1450ms | 26% |
| js_to_ready + ready_to_splash + splash_paint | 416ms | 7.5% |
| spawn_to_ready（后端进程创建；Go 自身 init 实测仅 15ms） | 1363ms | 25% |
| ready_to_window | 257ms | 4.6% |

（前两段有重叠：`Start()` 返回之前进程就已经在跑 Electron 冷启动了，所以合计是
约 5.5 秒而不是简单相加。）

### 0.4 关键线索：读文件满足不了"首次执行"的检查

预热明明**读过** `Deep Legends.exe`，却完全没改善这段 CreateProcessW
（control 与 prewarm 两组的这一段几乎相等）。但同一份预热确实改善了
**同一个 exe 的子进程拉起**（splash 首帧 −530ms）和 `loot-service.exe` 的
spawn（−507ms）。

**结论：把文件读一遍，能满足后续子进程创建时的检查，但满足不了首次执行时的检查。**
要消掉这 3 秒，必须让这个 exe **真的被执行过一次**，而不是被读过一次。

### 0.5 有 ~20 秒的空档可以用来藏这件事

日志里"installer page ready"到"startup prewarm"之间：22 / 19 / 22 / 21 / 26 / 27 秒。
这段是用户点"安装"加上 NSIS 解压落盘的时间。六次都稳定落在 19~27 秒，说明主要是
真实解压耗时而非用户操作快慢。

**也就是说：安装过程本身有二十来秒，而我们需要藏的执行预热只要三秒左右。
只要在 NSIS 还在解压其它文件时就把已经落盘的 exe 执行一遍，这三秒就完全不会
被用户感知到。**

---

## 一、本单锁定的决定

1. 不要在 `app.whenReady()` 之前 spawn 后端（R77 已用真机数据证否，
   `desktop/main.cjs:252` 有注释，`desktop/startup-sequence.test.cjs` 有护栏）。
2. 不要把 splash 的 `show:false` + `ready-to-show` 改回 `show:true`（R82-C 修白屏的修复）。
3. 不要向用户提"加杀毒软件白名单"这类要求。
4. 不要恢复或扩展 6 次 A/B 实验协议。验证方式见 C 节。
5. 不要在这一单里评估代码签名。R82 已查实 EV 证书自 2024-08 起**不再**享有
   SmartScreen 即时信誉（见 `WORKLIST-R82-STARTUP-LATENCY.md` 的更正块），
   签名的收益不确定而成本明确，等 B 节的效果出来之后再谈。

---

## 二、任务

### A 节（P0）· 预热转正

`installer/prewarm.go:15` 现在是：

```go
// Experimental until Windows A/B medians prove >=300ms improvement. No default
// production read: the OS-scanning benefit cannot be inferred from a file read.
func startupPrewarmEnabled(value string) bool { return value == "1" }
```

改成默认开启、可显式关闭：

- 未设置 / 空值 → 开启
- 显式 `"0"` → 关闭（保留关闭能力）
- 显式 `"1"` → 开启
- 其它任意值（`"true"`、`"yes"`、`" "` 等）→ 开启（走默认，不要因为拼错就静默退化）

注释换成实测结论：2026-09-12 六次真机冷装，中位数 total 3849→3471、
installed_to_visible 5401→4948、自身耗时 46ms、三次零失败；收益来自
`spawn_to_ready`(−507) 与 `splash_window_shown`(−530)。把 `output/r82-ab.json`
归档进仓库（若尚未归档）。

`scripts/r82-startup-ab.ps1` 的 control 分支设的是显式 `'0'`，默认改变后语义仍然正确，
确认即可，不需要改。

**变异判据**

| 变异 | 测试必须能发现 |
|---|---|
| 未设置环境变量时返回 false（没真正改默认） | ✅ |
| 显式 `"0"` 时返回 true（关不掉） | ✅ |
| 未知值（`"true"`/`"yes"`/`" "`）时返回 false | ✅ |
| 显式 `"1"` 时返回 false | ✅ |

### B 节（P0）· 把"读预热"升级成"执行预热"，并塞进 NSIS 解压的空档里

这是本单的主菜，对应 0.4 和 0.5。

**B-1. 执行预热的两条命令**

- 主程序：`Deep Legends.exe`，用 `ELECTRON_RUN_AS_NODE=1` 环境变量启动并让它立刻退出
  （例如 `-e "process.exit(0)"`）。这个模式下 Electron 只跑内置 Node，
  **不创建任何窗口、不加载 app.asar、不初始化 GPU**，但操作系统仍然要完整走一遍
  "执行这个 exe"的检查——这正是我们要的。
  环境变量只能加在这个子进程上，绝不能污染安装器自身或后面真正启动应用的那次调用
  （真正启动时如果误带 `ELECTRON_RUN_AS_NODE=1`，应用会变成一个 node 进程、
  永远不出窗口，接力会一直轮询到 20 秒超时——这是本节最危险的坑，必须有测试锁死）。
- 后端：`loot-service.exe --self-test`。这个 flag 已经存在
  （`main.go:292`，"validate embedded resources and exit"）。
  执行前请**先确认** `--self-test` 路径不会启监听端口、不会写用户数据目录、
  不会产生任何持久副作用；如果会，就换一个更干净的只读退出路径，
  或者在后端新增一个明确用于此目的的 flag。

**B-2. 时机：在 NSIS 还在跑的时候就开始，而不是等它跑完**

安装器已经在轮询目标目录的字节数来算进度（R80）。在同一个轮询里判断这两个文件
是否"已经完整落盘"，一旦判定完成就**在后台并行**触发对应的执行预热，
不阻塞进度条、不阻塞 UI 线程。

"完整落盘"的判据请用可测试的规则，例如：文件存在 **且** 大小在相隔 ≥400ms 的两次
轮询中没有变化 **且** 能成功以只读方式打开。

**B-3. 必须处理"预热之后文件又被改写"的情况**

如果 NSIS 在我们预热之后又重写/替换了这个 exe（比如先写临时文件再改名），
之前那次执行预热就作废了。做法：记录预热时该文件的大小与修改时间，
NSIS 退出后重新比对，**若已变化则判定作废**，并回退到现有的读预热（46ms，很便宜）。

**B-4. 失败一律不影响安装**

执行预热的任何失败（文件还没写完、进程起不来、非零退出、超时）都只记日志，
绝不能让安装失败、绝不能弹窗、绝不能阻塞后续的真实启动。整体给一个预算
（沿用现有的 `startupPrewarmBudget = 8s` 即可），超时就放弃并回退到读预热。

**B-5. 日志要让"一次普通安装"就能判定成败**

新增/调整这些日志行：

```
startup warm mode=execute file=<主程序|后端> ready_ms=<从安装开始到判定落盘完成> exit=<退出码> elapsed_ms=<执行耗时> voided=<true|false>
application launch pid=<pid> error=<err> elapsed_ms=<相对 started 的毫秒数>
```

其中给 `application launch` 那行**追加 `elapsed_ms=`** 是本节信息量最大的一处改动：
有了它，`Start()` 到底花了多久就是日志里直接读的数，不用再像 0.2 那样做区间反推。
现有的 `pid=` / `error=` 字段保留不动。

同时把日志时间戳精度提到毫秒（`log.SetFlags(log.LstdFlags | log.Lmicroseconds)` 或等价做法）。

**⚠️ 两个必须一起改、否则会静默炸掉的耦合点**

1. `scripts/r82-startup-ab-report.cjs:79` 的正则是
   `/application handoff visible=(true|false) elapsed_ms=(\d+)/`。
   在 `visible=` 和 `elapsed_ms=` 之间插任何字段都会让它失配，
   报告脚本会"找不到 handoff"然后一路轮询到 60 秒超时报错。
2. 同文件按 `[pid=N] ` **行首前缀**过滤日志行。加时间戳 flags 之后，
   Go 到底是先写时间戳还是先写 prefix，取决于是否设了 `log.Lmsgprefix`——
   **必须实际验证，不要想当然**；如果前缀不再在行首，报告脚本会一行都读不到。

改日志格式和改报告脚本必须同一次提交完成，并且要有测试同时吃"真实
`StartupLog()` 产出的行"和"真实正则/真实过滤逻辑"，任一侧单独改动都要红。
这正是 R82 补充单 D 节"文案耦合测试"那套模式的适用场景，照抄即可。

**变异判据**

| 变异 | 测试必须能发现 |
|---|---|
| `ELECTRON_RUN_AS_NODE=1` 泄漏到真正启动应用的那次调用 | ✅（断言真实启动的子进程环境里**没有**这个变量——这会导致应用永不出窗口） |
| 执行预热改成在 NSIS 退出之后才跑（丢掉与解压的重叠，等于没优化） | ✅（用可控的假 NSIS 进程，断言预热在 NSIS 仍在运行时就已触发） |
| 文件还没写完就触发执行预热，且没有回退 | ✅（构造一个大小仍在变化的文件，断言不触发；构造预热后文件被改写，断言 `voided=true` 并回退到读预热） |
| 执行预热失败/超时导致安装失败或阻塞启动 | ✅（让预热进程返回非零、卡死、文件不存在三种情况，断言安装照常完成、真实启动照常进行） |
| 日志格式改了但 `r82-startup-ab-report.cjs` 的正则/前缀过滤没跟着改 | ✅（同上，两侧必须由同一个测试同时锁住） |
| `application launch` 行只在成功路径打 `elapsed_ms`，失败路径不打 | ✅（launch 报错时也必须留下这条，否则失败案例反而没数据） |
| 后端 `--self-test` 会启端口/写用户目录（副作用泄漏） | ✅（断言执行后没有监听端口、没有新建用户数据文件） |

### C 节（P0）· 新的验证方式：一次普通安装，一个日志文件

**给用户的完整操作，只有三步：**

1. 像平时那样双击安装包，正常装一次，等软件自己打开。
   （不用跑任何脚本，不用带 `-Group`，不用管指纹，不用先关掉软件。）
2. 打开文件资源管理器，地址栏输入 `%TEMP%`，找到 `DeepLegendsSetup-startup.log`。
3. 把这个文件发回来。

**为什么一次就够（不需要再做 3+3 的对照实验）**：这一轮要打掉的是
3000ms 量级的东西，而实测的 run-to-run 噪声只有 24~30ms（六次
`handoff_ms` 分别是 5401/5401/5300 与 4901/4902/4901）。信噪比是 100 倍量级，
不存在"要靠统计才能看出来"的问题。判定标准就一条：

> `application launch ... elapsed_ms=` 这个数，
> 从现在的 **3000~3900** 掉到 **1000 以下** = 成功；
> 还在 2500 以上 = 这条路走不通，回到 0.4 的线索重新想。

日志里同时会有 `startup warm mode=execute ... exit=0 voided=false` 三行左右，
一眼就能看出执行预热到底跑没跑、有没有作废。

**GPT 这边要做的**：在交付时把上面三步照抄进验收报告，不要让用户再去翻脚本。
另外**不要**要求用户提供全新构建、不要要求用户先卸载、不要要求用户保持软件关闭——
这些约束都是旧 A/B 协议的产物，本轮一概不需要。

### D 节（P2，可以不做）· 两套时钟的精确对齐

如果 B 节做完 `application launch elapsed_ms` 之后仍然想知道"进程到底是第几毫秒
被创建的"，可以让应用把 `process.getCreationTime()` 的绝对值上报进
`desktop_startup_phases_ms`。

**但注意**：`desktop_startup.go` 的所有字段都过 `clampDesktopStartupPhase`
（上限 600000ms），epoch 毫秒是 1.7e12 量级会被直接截断；且
`decoder.DisallowUnknownFields()` 会让带新字段的请求整个被拒收
（R76 踩过"埋点静默失败导致测试假绿"的坑，`logs` 目录不存在时
`appendDiagnostic` 会静默失败）。要做就必须端到端验证事件真的落盘。

**B 节做完之后这一节大概率就不需要了**，所以列为 P2。

---

## 三、不要做的事

- 不要在 B 节里顺手改 splash、改后端 spawn 时机、改窗口接力的轮询参数
  （100ms / 20s 都不是瓶颈，X 的极差只有 24~30ms 说明轮询很稳）。
- 不要动 R82 补充单建立的 `installer/completion.go` 抽取和 AST 守卫测试结构。
- 不要因为 A 节改了预热默认值就删掉 `-Group control` 这条实验路径，以后还要用。
- 不要在验收报告里要求用户跑 `r82-startup-ab.ps1`。
