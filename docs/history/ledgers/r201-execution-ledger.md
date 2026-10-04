# R201 执行账本

日期：2026-10-03。基线 0.12.63（`c21482dba3c7`），本次版本 0.12.64。

## 实现与逐项核对

- P1-1：安装页与 payload 校验就绪后才回报 `DEEP_LEGENDS_UPDATE_STARTED`。后端随后发 `LOOT_UPDATE_STARTED`，desktop 隐藏全部 BrowserWindow（主窗口、启动画面、弹窗），阻止 activate/second-instance 再显示；后端按原流程停运行任务、关闭 LCU、刷新诊断，再发 `LOOT_QUIT update` 退出。安装失败时尝试打开原启动文件；便携版使用原始 portable exe，不误用尚未安装完成的目标文件。就绪超时停止本次安装子进程。
- P1-2：新版安装版/便携版命令均带 desktop PID 和 `--parent-first`。安装器最多等待 15 秒，退出后才做目录写入探测、释放 NSIS 和安装。便携数据迁移在启动安装器前完成，避免迁移占用退出等待窗口。
- 0.12.63 兼容：旧安装版未带 PID，安装器按**目标 exe 的完整路径及实际窗口**定位旧进程，先隐藏并通过 WM_CLOSE 请求正常退出。旧便携版的后端只有在 `INSTALLED` 后才迁移数据，不能提前杀掉：隐藏窗口后保留其既有“安装到独立新目录 → INSTALLED → 数据迁移/退出 → 等父进程 → 启动”握手；新版便携调用则全部先等退出再写。此旧便携兼容路径没有完整的线性六阶段计时，不填造时间点；新版与旧安装版的计时完整。兼容路径的实际 Windows 行为仍需用户验证。
- P1-3：仅升级 NSIS 命令加 `/NCRC`；手动安装的 `setupCommandLine` 保持 CRC。后端仍先核验 SHA-256。
- P1-4：首个有效 1 MB 测速完成后最多再比较一秒，选已完成线路的最快者并取消余下探测。取消的线路保持为未测量备选，下载失败/慢速换线仍能使用；记忆线路和续传保留，新增 `probe_measured` 区分未测速值。
- P1-5：`installer_start`/`parent_exited`/`relaunch` 由 Go 实际流程写入；`extract_start`/`extract_done`/`copy_done` 在 electron-builder 的真实 Nsis7z/CopyFiles 边界定点插桩，覆盖正常与 fallback 解压，保留寄存器与错误标志。安装器导入 child TEMP 的原始时间点并写数据目录 `update-install-timing.json`；新后端启动消费一次，只记阶段毫秒并删除；损坏/缺项记 invalid 后删除，无路径。模板结构变化会使构建失败，避免静默失去插桩。System 的 `.r9` 对应 `$9`，核对来源：[NSIS 官方文档](https://nsis.sourceforge.io/Docs/AppendixD.html)。
- P2-1：仅 ChampSelect/GameStart 且游戏进程未运行，临时解除 PersistedSettings.json 的只读。沿用安全路径、文件身份/字节核对与 CameraMode 单字段替换；Windows 保存/恢复完整文件属性，其他平台恢复原权限。无论写入成功或中途失败，立即恢复锁并核对；恢复后再读字节与只读状态。game.cfg 的只读规则不变。
- P2-2/4：恢复失败重试一次，两次失败记录 `relock_failed`；维护状态以实际 Windows READONLY 属性为准，显示既有“可写入”。诊断增加 `relocked`、`ok_relocked`，不增加 UI 文案。
- P2-3：LCU 200 且没有 CameraMode 记 `not_present`，不 PATCH；未知/失败仍明确区分。
- P2-5：只读锁其余行为不变。R186 结算同步已被用户在 0.12.62 明确删除，本工单不恢复已撤销的功能；原工单提及的旧 `skipped_locked` 实现不再适用。
- P3-1/4：常量 `champSelectSubsetTestFirstCard=true`，代码标注“R201 临时测试，真机验证后删除”。只有 subset 模式且 offered 与配置序列无交集才用卡片原顺序；第一张被队友预选时按既有避让取下一张。写前重新读子集、网格与队伍状态；有序列英雄但被挡住时不转测试选人。亮出、亮后锁定及等待、立即锁定均保留。
- P3-2/3/5：测试决策传播静默标记，抑制候选跳过、卡片提示、排定/发送/确认/失败的 UI 记录与 watch 提示；保留 `stage=subset reason=test-first-card champion_id` 和既有 postflight/confirmation 诊断。不加设置项。**这是临时逻辑，真机测试后删除，删除另开工单。**

## 真机日志与不改项

`lol-loot-diagnostics-1003-2002.jsonl` 逐行核对：子集预设接口 `/lol-lobby-team-builder/champ-select/v1/subset-champion-list` 199 次均 200、整数数组、3 ID；三次 /help 匹配为空。R200 账本与索引已改为“真机已核实”；实际卡片锁定/换人仍待用户日志。R197 赛后补全 `reason=ok revealed=1`，按用户决定保持现状，索引注明。R195 安装识别及 R196 已确认的下载能力不另改。见 [脱敏统计](../reports/r201/source-log-summary.json)。

## 验证

- Node 全量：1126 项，1125 通过、1 跳过、0 失败（300.035 秒）。
- Go 全量 `go test ./backend -count=1`：通过（262.544 秒）；R201/R196 定向通过（6.500 秒）；R201 race 通过（5.660 秒）。最终 public 构建已完整执行全部 1743 个 Go 分片测试、后端/installer vet 与 installer 测试。
- installer 独立模块 `go test ./...` 通过，另已交叉编译 Windows 安装器；根目录不是 installer 的 Go 模块，按实际模块目录执行。
- 三项变异均触发断言 FAIL，没有以编译失败充数：不 hide、跳过只读文件、测试选人写 UI；实际将常量改 false 的 overlay 下 R200 原无交集不选人测试 PASS。
- 测试还覆盖首个 0.49 秒完成/其他 5 秒的测速提前返回、窗口内两条完成选最快及取消后的下载备选；只读恢复首次失败/两次失败、局内与 none 不触属性；三种卡片策略、真实确认及 postflight；损坏计时删除、重复启动不重复记事件、原便携启动文件恢复。
- 证据在 [R201 核查目录](../reports/r201/verification.json)。本地最终构建与 GitHub Windows public 发布构建通过。

## 构建与发布

本地完整 public 构建通过，版本 0.12.64、指纹 `88054c16893c`，Setup SHA-256 `7e3718e03581c2ccef113db107ec4e4d766420c3618fef767cb6a8db42a1fc84`（[本地记录](../reports/r201/local-release-build.json)）。源提交及标签 `37bb3c12ea98260ea2b2806f88bea6f0a7d627cb`。

已正式发布 [v0.12.64](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.64)。Release id **402520305**，发布时间 **2026-10-03T13:15:14Z**（北京时间 21:15:14），`draft=false`、`prerelease=false`、`isLatest=true`；匿名下载 `releases/latest/download/latest.json` 返回 **0.12.64**，字节与核验后的发布清单相同。三项证据见 [发布核验](../reports/r201/publication-verification.json)。

Windows public 工作流 [37124778247](https://github.com/LLYY0418/Deep-Legends/actions/runs/37124778247) 已成功，source SHA 与标签一致。tag 质量工作流 [37124778245](https://github.com/LLYY0418/Deep-Legends/actions/runs/37124778245) 已全部成功：Linux quality（含全量 race、Node、真实 Chromium）及后续 Windows 构建/校验和均通过，状态见核查记录。

正式发布恰有三个 public 附件：`Deep-Legends-Setup-0.12.64-public.exe`、`latest.json`、`SHA256SUMS-public.txt`。逐个下载比对大小、GitHub digest、SHA256SUMS 与清单；版本和指纹均吻合。Windows 正式 Setup SHA-256：`f59d0db66d5ba9ce10309aef3e352b4ae73b7a807d59535a0938b855969049ea`，大小 111415808 字节。跨构建主机的 Setup SHA 不同，两个构建的源码指纹均为 `88054c16893c`。见 [附件核验](../reports/r201/release-asset-verification.json)。

只新增本版本发布；对比之前八个 Release 的身份、正文、目标、发布时间和附件 id/名称/大小/digest 均未改变，0.12.60 草稿 **402378684** 保持原样。

## 用户 Windows 验收

仍需从 0.12.63 在线升级，确认窗口隐藏、自动启动与计时；只读开启/自由镜头打一局，核对 `ok_relocked` 和 `in_game_60s camera_mode_matches_target=true`；海斗无序列英雄的首卡选择实际锁定成功。自动测试与交叉编译不替代用户游戏客户端验收。

## R204 追加真机核对（2026-10-04）

R204 工单提供的真机证据确认只读设置写自由镜头后恢复只读，game_start 仍 CameraMode=0，用户确认生效；卡片实际锁定全部 applied，临时首卡测试已由 R204 P9 删除。安装总耗时约 50 秒，但旧计时不完整，进度与提速数据追踪由 R204 P8 接手。原实现、已确认部分及后续接手构成关闭依据，R201 关闭。证据引用工单摘录；未将模拟或 Windows runner 当作用户验收。
