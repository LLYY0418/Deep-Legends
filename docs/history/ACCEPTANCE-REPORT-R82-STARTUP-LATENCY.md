# R82 安装完成后的启动交接与加载窗口

日期：2026-09-12。对应 `WORKLIST-R82-STARTUP-LATENCY.md`。

**A、C 已完成实现与本机自动化验证；B 已准备受控实验，但未完成 Windows A/B 实测，默认关闭；D 已提供方案，未购买或接入。整单的 Windows 真机验收尚未完成。**

本轮遵循用户“不要打包，我自己打包”的要求：没有运行发布构建、生成安装包、变更版本号、签名、上传或发布。仅生成了 `/tmp/r82-installer-windows.test.exe` 测试程序用于检查 Windows 编译。保留现有安装页面设计和已放大的升级图标；不改 NSIS、主窗口显示方式、后端启动顺序、asarUnpack、开机启动或杀毒设置。

## A：安装器窗口交接

- 普通安装与 `--update` 的 NSIS 成功路径统一调用 `handoffApplication`。删除原来的固定延时、隐藏外壳、ShellExecute 后关闭的链路。
- 外壳保持可见，进度 100%，文案“正在启动 Deep Legends…”。原有进度条光带动画继续运行，关闭按钮仍受安装忙碌状态保护。
- 在工作 goroutine 中通过 `exec.Command.Start()` 创建应用并取得 PID，避免 Windows 创建进程耗时阻塞安装器 UI 或超时计时器。
- 每 100ms 调用 `EnumWindows`，使用 `GetWindowThreadProcessId`、`IsWindowVisible`、`GetWindowRect` 验证同一 PID、可见、至少 160×100 的顶层窗。460×292 splash 和主窗口均可完成交接。
- 接力阶段最多 20 秒；exe 不存在、启动失败、创建进程阻塞或始终没有合格窗口，均走期限退出。仅向安装器自身发送 `WM_CLOSE`，不会关闭或终止新应用。
- 应用启动失败时记录错误并显示“程序未能启动，请稍后从开始菜单重试”；仍按期限关闭外壳。
- 原先无人调用的旧 NSIS fallback 没有重新接入，避免再次切回用户拒绝的旧界面。

主要实现：`installer/handoff.go`、`installer/handoff_windows.go`、`installer/install_windows.go`、`installer/ui/installer.html`、`installer/update.go`。

自动化证据：

| 工单变异 | 实际执行结果 |
|---|---|
| 去掉 20 秒期限退出 | KILLED，等待测试失败 |
| 不比对 PID | KILLED，其他进程窗口被拒绝的测试失败 |
| 先关闭安装器再启动应用 | KILLED，启动前可见性/关闭顺序测试失败 |
| 不检查可见性 | KILLED，隐藏窗口测试失败 |

`python3 scripts/r82-mutations.py` 在临时目录中复制实际生产 handoff 源码及测试，先确认原始版本通过，再逐个确认变异触发测试断言失败；不改工作区源文件。另有真实 Win32 窗口测试 `installer/handoff_windows_test.go`，覆盖 EnumWindows 回调、隐藏窗口、PID、尺寸和缺失 exe；**已交叉编译，尚未在 Windows 执行**。

## B：预热实验与保留门槛

**目前没有 Windows 实测数字，不能认定预热有效。** 不以工单中的 Defender 假设替代测量，也不默认增加用户等待时间。

- 仅安装器进程环境变量 `DEEP_LEGENDS_STARTUP_PREWARM=1` 启用实验；未设置、`0`、`true` 均不启用。
- NSIS 成功后、启动应用前，顺序整读 `<dest>\Deep Legends.exe` 和 `<dest>\resources\app.asar.unpacked\backend\loot-service.exe`，丢弃数据，不修改文件。
- 页面进度 97 → 98 → 100%，显示“正在优化首次启动…”。普通安装和升级共用实现。
- 总预热预算 8 秒，打开/读取失败继续处理，超时直接进入 A。阻塞的系统读取不能阻塞主等待流程；如果某次系统调用尚未返回，读取 worker 可能稍后才结束，但取消后不会继续读第二个文件。
- **8 秒预热和随后的 20 秒窗口交接是两个独立阶段**；仅显式启用实验时，两个阶段最坏合计约 28 秒。默认无预热阶段。
- 日志记录预热开关、已处理文件数、失败数、超时和耗时，以及启动 PID、交接是否成功、交接耗时。

新增采集脚本：`scripts/r82-startup-ab.ps1`；汇总脚本：`scripts/r82-startup-ab-report.cjs`。

### Windows 实测操作

要求：Windows 交互桌面、Node.js（支持 `node:test` 的当前项目版本）、PowerShell、用户自行完整构建的 `dist/desktop` 目录。安装包旁必须保留同次构建的 `release-build.json`；脚本按安装包 SHA-256 匹配记录取得指纹，允许改名。脚本本身不会构建、打包或读取/哈希已安装 exe。

1. 准备同一 R82 行为基线的 **6 个真正新指纹构建**，每个指纹此前没有在测试机启动过。不能把同一个安装包改名、复制后当成新构建，也不要在计时前手动打开已安装应用。尽量保持硬件、磁盘、Windows/Defender 版本、网络和后台负载一致。
2. 两组各 3 次，建议交错顺序：control、prewarm、prewarm、control、control、prewarm。每次完全退出应用；采集脚本会拒绝仍在运行的应用/后端，且不会强制杀进程。
3. 在仓库根目录执行以下命令，路径替换为这一轮新安装包的实际路径。`control` 为关闭预热；下一组改成 `prewarm`。正常完成安装后不要立即重新打开应用，等采集结果输出。

```powershell
.\scripts\r82-startup-ab.ps1 -Installer 'D:\安装包\Deep Legends Setup 0.12.1.exe' -Group control
```

4. 每次采集追加到 `output/r82-ab.json`。不足 3+3 次时只报告 pending，不作收益判断。六次完成后运行：

```powershell
node .\scripts\r82-startup-ab-report.cjs .\output\r82-ab.json
```

5. 保留 `output/r82-ab.json`、测试构建对应关系、启动日志和每次画面观察。窗口超时、预热失败/超时、埋点缺失不算有效性能样本；保留失败证据，另用新指纹补测。

采集口径：

- 后端默认日志 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`，兼容 `.1` 至 `.4` 归档；若有 `LOL_LOOT_DATA_DIR` 则使用该目录，也可传 `-DiagnosticsPath`。
- `desktop_startup_phases_ms` 本身没有指纹，必须通过同一 `run_id` 的 `app_start.build_fingerprint` 关联，不能只看相邻行。安装器日志按 `Start-Process` 返回的 PID 匹配。
- 拒绝结果文件内重复指纹/运行、留存日志中已经启动过的指纹、模式不匹配、多个启动、坏时间戳、缺失字段或失败交接。旧记录滚出全部归档后，脚本无法证明该指纹此前未运行，仍需测试者保证构建确实全新。
- 输出 `process_to_js`、`spawn_to_ready`、`total` 的两组中位数。`total` 保持既有含义：OS 创建应用进程到主窗 show。
- 同时输出预热耗时与 `installed_to_visible_ms = prewarm_ms + handoff_ms` 的中位数，后者近似“开始可选预热到首个应用窗口可见”的等待，包含预热及创建进程开销，**不包含 NSIS 安装/清理或主窗口余下加载时间**。
- 预热组 `total` 中位数改善 **不足 300ms：删除整个预热实验实现及入口**。达到门槛也要检查包含预热成本的实际等待和稳定性，再决定启用。脚本只出结论，不擅自改生产配置。

实测表（待填写，不能用合成测试数据填入）：

| 组 | 有效样本数 | process_to_js 中位数 | spawn_to_ready 中位数 | total 中位数 |
|---|---:|---:|---:|---:|
| control | 0/3 | 待测 | 待测 | 待测 |
| prewarm | 0/3 | 待测 | 待测 | 待测 |

`r82-startup-ab-report.test.cjs` 的 6 项测试使用明确标注的合成数据，仅验证关联、拒绝错误样本、归档读取、中位数和 300ms 门槛。PowerShell 启动流程没有在当前 macOS 环境执行；因此 B 仍是未完成的真机验收项。

## C：加载窗口显示时机

- splash 改为 `show:false`，`once("ready-to-show")` 中实际 show 后记录时间。窗口已关闭或应用正在退出时，不因迟到事件重新显示。
- 保留 `did-finish-load → startBackendOnce()` 以及精确 400ms 兜底；不提前 spawn。`startBackendOnce` 保持幂等，退出时取消兜底且阻止迟到事件启动后端。
- 单实例激活、activate 不会提前显示尚未准备好的 splash。
- 主窗口继续保持原来的 `show:false + ready-to-show`。修正 catch 收尾与 splash 赋值挤在一行的排版。
- 新埋点 `phases_ms.splash_window_shown` 是 **OS 进程创建到实际 show 的毫秒数**；既有 `splash_paint` 继续表示 splash 构造到 `did-finish-load`，并非严格意义上的屏幕首帧扫描完成时间。
- 新字段通过桌面请求、Go 严格 JSON 解码、范围限制，最终写入诊断日志；实际落盘测试覆盖该字段。

`desktop/startup-visibility.test.cjs` 执行真实 `main.cjs` 的事件处理代码，使用模拟 Electron 窗口与时钟。以下四个实际源代码变异全部触发断言失败：`show:true`、删除 show、删除 did-finish-load 的启动触发、删除 400ms 兜底。R77 既有护栏继续通过。此测试证明事件顺序，不替代 Windows 画面验收。

## D：代码签名方案（仅方案）

核对日期：2026-09-12。工单把 OS 创建/加载的整段耗时都归因于 Defender、并断言签名能消除约 3.2 秒，这些结论超出了现有阶段日志能证明的范围。日志能定位耗时区间，签名后的具体提速仍需同机测量。

微软明确说明：OV、EV 或 Artifact Signing 都不能保证新文件立即免除 SmartScreen 提示；**EV 的即时信誉旁路已经取消**。签名有助于显示经验证的发布者并积累信誉，不能承诺“不再扫描”或固定提速。[微软 SmartScreen 说明](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/smartscreen-reputation)

| 方案 | 参考成本，不是采购报价 | 条件与选择建议 |
|---|---|---|
| Azure Artifact Signing（原 Trusted Signing） | 约 US$9.99/月起 | 托管密钥，适合自动构建；先确认公开信任身份资格 |
| CA 签发的 OV 代码签名证书 | 微软比较表估计 US$150–300/年 | 向 CA 确认主体资格及含硬件/云签名的总价；不能把这个范围当成具体厂商报价 |
| EV 代码签名证书 | 微软比较表估计 US$400+/年 | 仅在业务确需更严格身份验证时考虑，不为“立即免 SmartScreen”支付溢价 |
| 暂不签名 | 无证书费用 | 保持当前发行方式，无法使用签名身份积累信誉 |

成本参考及硬件要求见[微软代码签名选项](https://learn.microsoft.com/en-us/windows/apps/package-and-deploy/code-signing-options)。OV 私钥需要合规硬件保护，但可用硬件 token 或云 HSM，并非只能插 U 盾，也并非必然无法接入 CI。

资格以服务自身的[Artifact Signing 快速入门](https://learn.microsoft.com/en-us/azure/artifact-signing/quickstart#prerequisites)为准：个人开发者当前限美国、加拿大；组织列表还包含欧盟、英国、澳大利亚、新西兰、日本、韩国、新加坡、瑞士、挪威、以色列等。中国大陆主体目前不在该公开信任列表内。微软的通用比较页组织地区列表较短，不能用它替代服务实际准入文档。还需要付费 Azure 订阅，[FAQ](https://learn.microsoft.com/en-us/azure/artifact-signing/faq)明确不支持免费、试用或赞助订阅。

建议先按实际个人/公司主体核实资格：符合 Artifact Signing 条件可优先评估托管签名；不符合则向 CA 获取 OV 与云 HSM 的完整报价。当前不做身份假设、不购买、不上传身份材料、不接入签名。

后续若用户决定接入，应与 R81 公开发布一起落实：覆盖后端 exe、Electron 应用 exe、安装/卸载程序，使用一致发布者身份及时间戳；最终签名完成后再算对外 SHA-256/更新清单，验证已签文件不被后续打包修改，再做 Windows 启动及升级回归。

## 已执行检查

| 检查 | 结果 |
|---|---|
| installer 模块 `go test -race -count=1 ./...` | 全部通过，含 handoff、prewarm、WebView host、卸载回归 |
| installer Windows amd64 `go vet ./...` | 通过 |
| installer Windows amd64 `go test -c` | 通过；仅编译测试程序，没有运行 Windows 原生测试 |
| 根模块 `go test -run '^TestDesktopStartup' -count=1 .` | 通过，新字段实际落盘及边界限制 |
| startup-sequence、startup-visibility、installer-shell | 19/19 通过，包含 C 的 4 个变异 |
| installer-startup、installer-nsh、update-lifecycle、build-fingerprint | 22/22 通过 |
| A/B 采集与汇总测试 | 6/6 通过，全部为合成测试数据 |
| A 交接变异脚本 | 原始通过，4/4 变异被杀死 |

## Windows 最终验收待办

- [ ] 全新安装：从安装完成到 splash/主窗出现，连续录屏确认没有空桌面；升级路径也确认一次。
- [ ] splash 首次可见时已有深色背景和 logo，没有先露出空白窗口。
- [ ] 保存一次新指纹的完整 `desktop_startup_phases_ms`，与工单提供基线（1618 / 151 / 1580 / 3623ms）比较，另检查 `splash_window_shown`。
- [ ] 完成 B 的两组各 3 次全新指纹对照，按 300ms 门槛决定撤销或保留；目前预热继续关闭。
- [ ] 六次冷启动采集结束后，再测同一构建第二次启动，独立记录冷热差异，不混入冷启动样本。
- [ ] 在测试目录中验证应用启动失败时外壳约 20 秒退出、自身无残留，且不终止其他应用；启用预热时明确区分其额外阶段。
- [ ] 在 Windows 交互桌面运行 `go test -run '^TestWindows(ApplicationWindowDetection|MissingApplicationDoesNotYieldAPID)$' -count=1 .`（工作目录 `installer`）。
