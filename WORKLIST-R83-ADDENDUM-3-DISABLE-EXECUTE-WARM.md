# WORKLIST-R83-ADDENDUM-3 · 关掉执行预热，只保留已证实安全的读预热

诊断人：Claude（只读诊断，**未改仓库任何代码**）。执行人：GPT。
数据来源：用户真机 `DeepLegendsSetup-startup-927eccce.log` +
`diagnostics-0d59fad4.jsonl`（指纹 `d2212835fcb1`，2026-09-12 20:22 那次安装）。

---

## 零、先记两件事

### 0.1 上一单（ADDENDUM-2 撤回暂存区）**成功了**

`d2212835fcb1` 这次安装器日志已经完全干净：

```
startup warm mode=execute file=主程序 ready_ms=11594 exit=-1 elapsed_ms=298 reason=still_running
startup warm mode=execute file=后端  ready_ms=11844 exit=-1 elapsed_ms=49  reason=still_running
startup prewarm enabled=true files=2 failures=0 timed_out=false elapsed_ms=55 execute_valid=0 fallback_files=2
application launch pid=28536 error=<nil> elapsed_ms=1928
application handoff visible=true elapsed_ms=3800
```

没有 `source=staging`；`fallback_files=2` 说明读预热恢复成无条件对两个文件执行（B 节回归已修）；
`launch=1928` / `handoff=3800` 与历史最好的一次（1879 / 3801）持平。**这一单的目标全部达成。**

### 0.2 但这次安装后，应用第一次启动卡死了

用户报告：装完之后软件一直停在加载中，强制关闭、重新打开才正常。

诊断日志坐实了这一点。失败那次运行 `run_id=d5b2582ee07da8c5bea8fba7`：

- `app_start` 20:22:37.320，后端一切正常：LCU 连上、身份刷新成功、
  `overview_load_cost duration_ms=717` 数据全部加载完成、之后每 10 秒一次
  `lcu_request` 心跳持续到 20:23:20（用户强杀）。
- **全程没有 `desktop_startup_phases_ms` 事件。** 该事件由
  `desktop/main.cjs:437` 的 `mainWindow.once("ready-to-show")` 触发
  （`show()` → `closeSplashWindow()` → `reportStartupPhases()`）。
  它没出现 = **主窗口的 `ready-to-show` 从未触发 = 主窗口从未显示**。
- splash 是 `alwaysOnTop: true`（`desktop/main.cjs:128`），主窗口不显示、
  splash 就不会被关闭，于是永远停在"正在启动本地数据服务"——与用户描述完全一致。

**这条运行的 `log_seq` 是 1→58 连续完整，没有被日志轮转截断，所以"缺少该事件"是真实的，不是采样问题。**

---

## 一、关于归因：我不能证明是 R83 造成的，也不能排除

必须如实说明，不要在验收报告里把这段写成已有定论：

- 我最初在历史日志里找到两次疑似相同故障（09-11 10:14 和 09-12 16:41），
  **但复核后发现两次都是假阳性**：它们的 `log_seq` 分别从 788 和 31 开始，
  说明该运行的前半段（含 `desktop_startup_phases_ms`）在已被轮转走的旧文件里。
  其中 09-11 那次实际运行了 4169 秒，显然一切正常。
  **所以 20:22 这次是目前唯一一次真实发生的"主窗口从未出现"。**
- 样本量：R83 执行预热上线后共 3 次安装（18:18 成功、19:45 成功、20:22 失败）；
  之前约 10 次安装无此故障。1/3 对 0/10，有提示性但**远不足以定论**，
  完全可能是巧合或与本轮改动无关的既有偶发问题。
- 存在一个**具体且可信的机制**，值得直接消除而不是继续赌：
  执行预热会启动 `Deep Legends.exe` 然后在几百毫秒后**强杀**它
  （本次 `elapsed_ms=298`）。设计上它带 `ELECTRON_RUN_AS_NODE=1`，
  应当只跑纯 Node、不碰窗口与 GPU。但只要这个环境变量在任何一条路径上没生效，
  被强杀的就是一个**正在初始化中的完整 Electron 实例**，
  它可能在 `userData` 里留下半写入的 GPUCache / Code Cache / 锁文件，
  而这正是"下一次启动时渲染进程永远不触发 `ready-to-show`"的典型成因。

## 二、要做什么

### A 节（P0）· 执行预热改为默认关闭

把执行预热整体置于一个**默认关闭**的开关之后（例如
`DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 才启用），默认路径上
**不启动任何子进程、不强杀任何进程**。

保留 R83 已经写好并通过验收的那套代码（稳定判定、`voided` 复核、非阻塞 `Finish()`、
预算、回退），只是默认不走它。不要删代码——它可能仍是对的，只是目前收益未经证实、
而风险已经真实出现一次。

**读预热保持默认开启、无条件对两个文件执行**（ADDENDUM-2 B 节刚修好的行为不要动）。
这是唯一经过 3+3 对照实验证明的收益：`spawn_to_ready` 1870→1363、
`splash_window_shown` −530ms，代价 32~55ms，且**全程只读文件、不创建任何进程**，
不具备上面那条风险机制。

日志在执行预热关闭时应明确输出一行可识别的状态（例如
`startup warm mode=execute enabled=false`），便于以后从日志一眼确认走的是哪条路。

### B 节（P0）· 补一条护栏：`ELECTRON_RUN_AS_NODE` 必须在真实启动路径上被剔除

这条已有测试（`TestExecuteWarmEnvironmentCannotLeakIntoRealApplication`，我在上一轮
独立变异验收里确认过它有效）。本节只要求补充：**即使执行预热默认关闭，这条测试也必须
继续存在并有效**，不要因为默认关闭就顺手删掉它。

### C 节（P1）· 把这次故障记进代码注释和文档

在 `installer/execute_prewarm.go` 顶部注释里写清楚：
2026-09-12 20:22（指纹 `d2212835fcb1`）安装后首次启动出现主窗口永不显示、
splash 永久停留的故障；无法证明由执行预热导致，但因执行预热会强杀一个刚启动的
`Deep Legends.exe`，在收益未经证实前默认关闭。

同时在 `ACCEPTANCE-REPORT-R83-*.md` 或 README 的相应位置更新现状，
**不要**把执行预热写成"已验证有效"。

## 三、验收判据

1. 默认配置（不设任何环境变量）下安装一次：安装器日志里**不得出现**
   任何 `mode=execute ... attempted=true` 的记录，必须能看到
   `enabled=false` 之类的明确状态行；`startup prewarm ... files=2` 仍然存在。
2. 显式设置 `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 时，执行预热行为与现在完全一致。
3. `spawn_to_ready` 应回到读预热基线区间（约 1350~1400），而不是 1800+。

## 四、变异判据

| 变异 | 测试必须能发现 |
|---|---|
| 执行预热默认开启（即 A 节没真正生效） | ✅ |
| 显式开启时执行预热不工作 | ✅ |
| 读预热被执行预热的结果影响（ADDENDUM-2 的回归复活） | ✅（无论执行预热开或关、成功或失败，读预热始终对 2 个文件执行） |
| `ELECTRON_RUN_AS_NODE` 过滤被删除 | ✅（B 节） |

## 五、不要做的事

- 不要在归因未明的情况下，把执行预热删干净。默认关闭即可，保留代码与测试。
- 不要为了"挽回"执行预热的收益而去尝试"不强杀、等它自己退出"——
  那会把安装流程和一个 3 秒的进程绑在一起，风险更大。
- 不要改 splash 的 `alwaysOnTop`/`show:false`、后端 spawn 时机、窗口接力参数。
  主窗口不显示的根因尚未查明，现在改这些只会制造新的变量。
- 不要恢复六次 A/B 协议。
