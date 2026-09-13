# R83 执行预热时机补充单验收记录（已撤回，历史记录）

日期：2026-09-12。依据 `WORKLIST-R83-ADDENDUM-EXECUTE-TIMING.md`。

**补充单 2 已撤回本报告对应的暂存执行方案，相关代码、后台任务和专用测试已删除。** 最新补充单 3 将最终目录执行预热默认关闭，仅在 `DEEP_LEGENDS_STARTUP_EXECUTE_WARM=1` 时开启实验路径，额外收益尚未证实；读预热继续默认开启并无条件读取两个最终 EXE，不保留暂存方案开关。下文仅保留当时的布局核验、实现和测试历史，不能作为当前实现或验收要求；现行规范见 [执行预热默认关闭验收报告](ACCEPTANCE-REPORT-R83-READ-ONLY-DEFAULT.md)。原实施轮没有打包或运行 Windows 安装器；默认压缩级别仍为 9。

## 证据与真实包布局

原补充单提供的指纹 `00d37a7f7892` 日志显示两个目标都是 `reason=still_running`、`execute_valid=0`、读回退 32ms，`application launch elapsed_ms=1879`、handoff 3801ms。后续补充单 2 提供同一次运行的 `spawn_to_ready=444`；当时将“未完成”直接判为“没有作用”的解释已撤回，不能仅凭 `execute_valid=0` 否定效果。

本机已有同一指纹的真实 `dist/desktop/Deep Legends Setup 0.12.1.exe`，SHA-256 为 `d3b9bfcdd2aa99f1be371ca37aec489360e1e508d7c694871d1bdbabe6c66583`。直接从现有 Go 外壳的嵌入数据提取 NSIS payload，再用已缓存的 7-Zip 24.09 解出 `$PLUGINSDIR/app-64.7z`，最后真实解压到 `/tmp/r83-staging-layout/7z-out`。没有重新构建、执行 EXE 或改动原产物。

- NSIS payload：99,534,600 字节；应用 7z：97,050,540 字节。
- 7-Zip 返回 `Everything is Ok`；应用归档包含 4 个目录、25 个文件，解压总计 335,877,900 字节；格式为 LZMA2 + BCJ。
- 主程序确实为 `Deep Legends.exe`，225,565,184 字节，SHA-256 `b77db6e1540e823bfa41d510398ef96111135ff86d62387977d67f12f76bf01e`。
- 后端确实为 `resources/app.asar.unpacked/backend/loot-service.exe`，18,981,888 字节，SHA-256 `8f1fd79f1932f110f9591afcfdc5053d89fe44dfeda1dee81732761eb7a6b37c`。

因此使用这两个相对路径有真实包依据，并非只从 `package.json` 推测。这里确认的是归档内容及解压布局，未声称运行过 Windows 上实际的 `Nsis7z::Extract`。

## 六项实现核对

| 工单要求 | 落实情况 |
|---|---|
| 1. 复用暂存目录定位 | `extraction_paths.go` 抽出共用定位，进度统计和预热均查本次安装独有 TEMP 内的 `ns*.tmp/7z-out`；Windows 保留真正的 `CreationTime >= installationStarted` 检查。找不到、多个候选或符号链接不猜路径，执行不启动，最后读取最终目录。 |
| 2. 从暂存目录执行/读取 | Windows adapter 同时传最终目录和私有 TEMP；执行目标及执行前后校验均指向 `7z-out`，不再等待最终目录出现。第一次确定源目录后不切换到其他候选。主程序仍只对该子进程设置 RunAsNode，后端仍使用无副作用早退 flag。 |
| 3. 保留 400ms 定稳 | 非空普通文件、只读打开成功，大小、mtime、文件身份持续至少 400ms 一致才调度；一边增长一边解压的文件不执行。 |
| 4. 拷贝/删除风险 | 校验读句柄开放 Windows 读、写、删除共享，不主动阻止 NSIS CopyFiles 或清理。源文件消失会取消预热；若执行期间变化，预热作废。运行中的映像仍可能被 Windows/杀毒持有；子进程退出后增加私有 TEMP 清理重试，避免首次删除失败后遗留目录。真实 AV 干扰尚未排除，详见下一节。 |
| 5. 最终复核 | 执行前后分别校验暂存文件的 SHA-256 与身份/元数据；最终目录在后台独立观察并计算 SHA-256。收尾要求执行成功、源未被观察到改写、最终 SHA 与执行前的源 SHA 一致、最终身份/元数据仍与校验时一致。复制产生不同文件 ID 是正常的，不能用源/目标 `os.SameFile` 代替内容校验。 |
| 6. 保留读取兜底 | 未发现源、进程失败/未完成/超时、源变化、最终文件缺失/不同、后台校验来不及完成都回退到 **最终目录** 的原 `runStartupPrewarm`。该函数、读预算及失败语义未改。 |

安装完成策略原本先删除 TEMP 再接力，本轮保留 `completion.go` 和原 AST 守卫结构。正常清理后源不存在不等于作废：只有已有完整执行证明且最终副本匹配时才有效；不能因目录删除而丢掉所有成功预热，也不能仅凭“源没了”跳过最终核验。

所有整文件哈希在后台完成，与两个执行任务共享从首个文件定稳起算的 8 秒上下文。最终目录观察间隔为 50ms，独立于原 250ms 安装进度轮询；只为赶上 CopyFiles 到 NSIS 退出之间的短窗口。`Finish()` 仍立即取消、只做非阻塞接收和元数据复核，不等待 CreateProcess、进程退出或哈希。未拿到最终校验结果也回退。原读取兜底保留自己原有的 8 秒上限。

## 拷贝和删除风险：已排查什么、还缺什么

本地 `extractAppPackage.nsh` 的 92–109 行先整体解到 `7z-out` 再 `CopyFiles`；111–135 行在复制失败时重试，最终可能 `RMDir /r` 后直接解压。源码中的 `CopyFiles` 调用 Windows `SHFileOperation`。[NSIS 原生实现](https://github.com/kichik/nsis/blob/master/Source/exehead/exec.c#L1118)

1. 原 Go `os.Open` 在 Windows 只共享读/写，本轮校验读取使用 `CreateFileW(GENERIC_READ, FILE_SHARE_READ | FILE_SHARE_WRITE | FILE_SHARE_DELETE)`，允许其他句柄读取、写入和删除。该保证只覆盖我们自己打开的校验句柄，不能控制映像加载器或杀毒软件的句柄。[Microsoft CreateFileW 共享规则](https://learn.microsoft.com/en-us/windows/win32/api/fileapi/nf-fileapi-createfilew)
2. 新增真实文件句柄测试：在只读句柄未关闭时复制并删除源，仍可读到原字节。当前 macOS 实际通过；Windows 适配层共享标志由 AST 检查和 Windows 编译验证，Windows 运行结果未取得。
3. 新增真实子进程并发测试：复制 Go 测试程序到临时目录、从该副本启动并保持运行、同时拷贝其 EXE，随后取消/回收子进程再清理，核对复制内容 SHA-256。当前 macOS 测试通过，一次记录拷贝 3ms；**这不是 Windows、不是 225MB Electron、也不是杀毒性能数据，不能据此排除 Windows 干扰。** Windows 测试适配器使用与 NSIS 相同的 `SHFileOperationW`，已编译但没有在 Windows 运行。
4. 测试还覆盖源在运行时被删的取消、源执行期间原大小/mtime 被保留但内容变化、成功后源变化、最终副本内容/身份/mtime 变化、最终校验读取卡住，以及迟到子进程退出后重试清理。

**尚未排除的风险：**执行暂存 Electron 时的杀毒扫描可能与 NSIS 源读取竞争磁盘或安全检查；活跃映像也可能让 NSIS 的删除重试暂时失败。共享句柄和非阻塞回退不等于这两种系统行为已经实测无影响。当前可用环境为 macOS，没有可执行的 Windows 测试环境，本轮不能给出“已排除”的结论。下一次普通安装日志用于确认是否提前完成以及是否出现明显安装等待；若仍出现拷贝重试或卡顿，需要依据该次现场继续定位，不能伪造一个提速数字。

## 自动验证

- 安装器全部 Go 测试及 `go test -race -count=1 -timeout=60s ./...` 通过，包括真实子进程并发拷贝、取消及异步校验回退。
- Windows/amd64 `go vet ./...` 和测试程序交叉编译、链接检查通过，包含 `SHFileOperationW` 原生测试适配器。生成的是 `/tmp` 中的测试程序，不是 Setup；没有把交叉编译当作 Windows 执行成功。
- Node 启动顺序、splash、外壳、日志采集及 Setup-only 构建回归 **28 PASS / 0 FAIL / 0 SKIP**。模拟构建确认仍是默认 `-mx=9 -mf=BCJ`，未运行真实打包。
- 扩展后的 `scripts/r83-startup-mutations.py` **33/33 变异被实际测试断言捕获**；涵盖既有 24 项及暂存路径接线、最终内容/身份核对、最终目录兜底、定位范围、Windows 创建时间、迟到清理、源内容变化、读句柄删除共享等新增错误。
- 变异检查暴露并修正了旧测试的一个异步盲点：400ms 之前现在直接断言尚未调度，而非仅检查后台进程回调是否已经进入，避免后台校验导致错误时机的变异漏检。
- `git diff --check` 通过。`completion.go`、原完成策略 AST 守卫结构、读预热函数、8 秒预算、应用窗口接力参数、splash 和后端正常 spawn 时机均保留。

## 验收方式已由补充单 2 更新

1. 像平时那样双击安装包，正常装一次，等软件自己打开。（不用跑任何脚本，不用带 `-Group`，不用管指纹，不用先关掉软件。）
2. 找到 `%TEMP%\DeepLegendsSetup-startup.log` 和 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`。
3. 把这两个文件发回来。

旧的 `execute_valid=2` / `final_verified=true` 要求已撤回；暂存日志字段随代码删除。当前应有两条执行记录、`execute_valid` / `fallback_files` 诊断和两个最终文件的读取计数，`ok` 与 `still_running` 均可接受。

旧的“仅按 launch 阈值判定”已撤回。当前主判据为 `spawn_to_ready` 是否明显低于 1363ms，launch 仅供参考；不恢复六次 A/B，不要求先卸载，不要求保持软件关闭。当前实现及完整判据见最新报告。
