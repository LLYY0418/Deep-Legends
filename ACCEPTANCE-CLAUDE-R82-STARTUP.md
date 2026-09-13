# R82 启动延迟 · 独立验收报告

验收人：Claude（只读验收，**未修改仓库任何代码**；全部变异在 `/tmp` 副本中进行）。
日期：2026-09-12。对象：`WORKLIST-R82-STARTUP-LATENCY.md` A~D。

---

## 一、总评

**A、B、C 三节真实落地**，我自己设计的 **14 处独立变异全部被现有测试杀掉**
（没有复用 GPT 自带的 `scripts/r82-mutations.py`）。

GPT 这份报告的可信度比较高，两个加分项：
- **B 节拒绝拿我的假设当结论**：我在工单里假设"读文件能把 Defender 扫描前移"，
  GPT 没有照单全收，而是把预热做成**默认关闭的实验**、要求真机 A/B 达到 300ms
  门槛才保留，实测表老老实实留着 `0/3`「待测」，没有拿合成数据冒充真机数字。
- **D 节直接驳回了我写错的结论**（见第四节），而且是查了微软官方文档来驳的。

### 基线（全绿，先确认再做变异）

| 套件 | 结果 |
|---|---|
| 根模块 `go test ./...` | ok，65.7s |
| `installer` 模块 `go test ./...` | ok |
| `cd desktop && node --test` | **198 pass / 0 fail**（R82 前是 187） |
| `node --test web/*.test.cjs` | 359 pass |
| `node --test scripts/*.test.cjs` | 8 pass |

---

## 二、★ 最重要的发现：`*_windows.go` 在 Linux 根本不编译，变异是隐形的

Go 的隐式 build constraint 让 `*_windows.go` 文件在 Linux 上**完全不参与编译**。
所以 `go test ./...` 打出的 `ok`，对这些文件而言是"压根没测到"，不是"测过了"。

实测证据（两处独立变异）：

| 注入的破坏 | Linux `go test ./...` | `GOOS=windows go build/vet/test -c` |
|---|---|---|
| `--update` 分支绕过 handoff，直接发 `WM_CLOSE` 退出 | 依旧 **ok** | **全部干净通过，零告警** |
| `CloseInstaller` 里顺手 `TerminateProcess` 杀掉刚启动的应用 | 依旧 **ok** | **全部干净通过，零告警** |

也就是说，这两处真被人改坏，**整条 CI（含 Windows 交叉编译的 vet/build）都不会报警**，
只能靠 Windows 真机跑一次升级、或者盯着任务管理器才能发现。

### 由此产生的三条缺口（代码现状都是对的，但零护栏）

1. **【P1】`--update` 升级路径与 handoff 的绑定只是"结构性巧合"。**
   `install_windows.go:161` 是全仓库唯一的 `handoffApplication` 调用点，升级路径靠
   `webview_windows.go:65-66` 自动触发 `install` 消息绕进来。谁以后给 update 分支
   单独抄一份旧的 ShellExecute 逻辑，测试不会红。
2. **【P1】"外壳退出只关自己、绝不杀应用"没有任何测试盯着**（`handoff_windows.go:89-94`）。
   将来有人以"确保干净退出"为由加一行 kill，护栏拦不住。
3. **【中】B 节最关键的承诺"预热失败/超时绝不挡住启动"无法验证。**
   集成点在 `handoff_windows.go:70-75`（预热结果只写日志，随后**无条件**调用
   `runApplicationHandoff`）——设计是对的，但这条在任何环境都做不了变异测试。
   工单里我要求的那条变异"预热失败后不继续启动应用"，实际上没做成。

**建议**：把这三处逻辑照 `handoff.go` 的样子抽成**可注入 hooks 的平台无关纯函数**
再测。`handoff.go` 本身就是正面例子——它没有 `_windows` 后缀，所以 6 处变异全部命中。

---

## 三、变异测试明细（14 处，全部被杀）

### A 节 · 安装器窗口接力（6 处）

| 变异 | 结果 | 平台限制 |
|---|---|---|
| 去掉 20 秒期限退出（永远等下去） | FAIL ✅ | 无，Linux 可测 |
| 等待窗口不比对 PID | FAIL ✅ | 无 |
| 退回旧顺序（先关外壳再启动） | FAIL ✅ | 无 |
| 不检查 `IsWindowVisible` | FAIL ✅ | 无 |
| 去掉 160×100 尺寸下限 | FAIL ✅ | 无 |
| （另两处 `_windows.go` 变异） | **测不到** ❌ | 见第二节 |

核实通过：旧的"隐藏外壳→ShellExecute→退出"链路确实删了
（`runFallback`/`runInstallerFallback` 已成零调用点的死代码）；
`Start()` 确实放在 `go func` 里，不阻塞 UI 线程。

### C 节 · splash 白屏（4 处）

| 变异 | 结果 | 断言类型 |
|---|---|---|
| splash `show` 改回 `true` | FAIL ✅ | **真执行** |
| 删掉 `ready-to-show` 里的 `show()` | FAIL ✅ | 真执行 |
| 删掉 `did-finish-load` 里的 `startBackendOnce()` | FAIL ✅ | 真执行 + R77 正则护栏双红 |
| 删掉 400ms 兜底定时器 | FAIL ✅ | 真执行 + R77 护栏双红 |

- `desktop/main.cjs:123` 已是 `show:false`，`once("ready-to-show")` 里才 `show()`
  并记录 `splashWindowShown` 时间戳。**白屏根因已修掉。**
- **R77 的成果没有被破坏**：`did-finish-load → startBackendOnce` + 400ms 兜底原样保留。
- `startup-visibility.test.cjs` 是**真执行**：用 `vm.runInContext` 跑真的 `main.cjs`，
  配 fake Electron/定时器，靠 `EventEmitter` 真实触发事件后断言状态，
  **不是对源码做正则匹配**（这点我特意让子代理区分过）。
- ★ 新埋点 `splash_window_shown` 的落盘测试**显式 `MkdirAll(logs)`**，
  没有重蹈 R76 那个"logs 目录不存在 → `appendDiagnostic` 静默失败 → 断言全空的假绿"的坑。

### B 节 · 预热（4 处）

| 变异 | 结果 |
|---|---|
| 预热改成默认开启 | FAIL ✅ |
| `"true"` 也被当成启用 | FAIL ✅ |
| 去掉 8 秒预算（可无限读） | 测试**直接挂死**被强杀 ✅（反证兜底真实存在） |
| 300ms 门槛改 0 / 判断取反 | FAIL ✅ |

- `startupPrewarmEnabled(v) { return v == "1" }` —— 严格匹配，`""`/`"0"`/`"true"` 都不启用，
  **默认确实是关的**。
- 8 秒预算用 `context.WithTimeout` 实现，只读不写（有测试断言读后文件大小/内容不变）。

---

## 四、★ 我在工单里写错了两处，GPT 驳得对（已更正工单 D 节）

1. **"签名后能压掉那 3.2 秒"——超出证据范围。**
   埋点只能证明耗时落在"OS 创建/加载进程"这一段；把它**全部**归因于 Defender
   并给出一个具体提速幅度，是我的推断，不是实测。签名后到底快多少必须同机测了才算。

2. **"EV 证书 SmartScreen 信誉即时生效"——已经不成立。**
   微软 2024 年 8 月通过 Trusted Root Program 更新（3.D.3）把 EV Code Signing OID
   从根证书移除，**所有代码签名证书一视同仁**，EV 不再有即时信誉旁路，
   新版本照样要重新积累信誉。DigiCert 也确认了这点。
   ⇒ **不要为了"免 SmartScreen 警告"去买 EV**，那是过时的营销话术。

   补充（GPT 查到、我原文漏了的）：Azure Artifact Signing 的准入有地区限制——
   个人开发者目前限美国/加拿大，组织名单不含中国大陆主体，且需要**付费** Azure 订阅
   （不支持免费/试用）。**先按实际主体核实资格，再谈要不要买。**

工单 D 节已加更正块。这条给我自己的教训：**写工单时涉及"外部平台策略"这类会过期的事实，
要么当场查证要么标注时效，不能凭记忆断言。**

---

## 五、其它观察（低严重度）

- `installer/internal/webviewhost` 在刚装完 Go 的**第一次** `go test ./...` 偶发 FAIL 一次，
  之后连跑 11 次（含 4 路 CPU 加压）全绿，无法复现 ⇒ 记为 flake，不是缺陷。
  但它对机器负载敏感这点值得留意，CI 上可能偶发红。
- **死代码**：`install_windows.go:227-272` 的 fallback、`dialog_windows.go:133-142`
  的 `shellOpen`，全仓库零调用点。不影响功能，但留着容易让后续验收误判"两条链路并存"。
- `update.go` 里升级完成文案是靠对 `installer.html` 做**字符串替换**实现的
  （"安装已完成"→"升级已完成"）。哪天改了 HTML 里的原始措辞而没同步改替换目标串，
  替换会**静默失效**，且没有测试覆盖这个耦合点。

---

## 六、仍然只能真机验收的（GPT 报告已如实列出，我确认属实）

1. 全新安装 → 录屏确认**没有任何一帧空桌面**；升级路径也走一次。
2. splash 首次可见时就是深色带 logo，**不先露白窗**。
3. 采一次新指纹的完整 `desktop_startup_phases_ms`，和基线
   （proc2js 1618 / splash_paint 151 / spawn2ready 1580 / total 3623）对比，
   另看新字段 `splash_window_shown`。
4. **B 节两组各 3 次全新指纹 A/B 实测**，按 300ms 门槛决定留还是删。
   ⚠ 必须用真正的新指纹构建，不能拿同一个包改名——否则第二次本来就是热的，会得出假结论。
5. 应用启动失败时外壳 20 秒兜底退出、不残留、不误杀其它进程。
6. 在交互式 Windows 桌面跑 `installer` 目录下那两个真 Win32 窗口测试
   （目前只交叉编译过，从未真跑）。
