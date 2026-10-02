# R186 执行账本

日期：2026-10-02。基线：已发布的 0.12.50（R185），本轮版本：0.12.51。用户授权执行 R186；本轮没有额外的发布指令。

## P1：便携版软件内升级

| 项目 | 实现与核对 |
|---|---|
| P1-1 | Windows 启动检测记录 `update_install_detection`，覆盖 installed、portable_env、no_uninstaller、registry_missing、registry_location_mismatch；同时记录 registry_display_found / location_matches，不序列化路径。 |
| P1-2 | Portable / ManualOnly 使用原有下载管线，保留镜像、断点、空间与 SHA-256 校验；应用前再次校验安装包。 |
| P1-3 安装与退出 | 非安装版调用外层 Go 安装器 `--update --fresh-install --parent-pid <Electron PID> --dest <默认目录>`。内层 NSIS 使用 `/S /currentuser --portable-upgrade --updated /D=<目录>`，`/D=` 必须最后且不加引号，沿用模板的含空格路径解析。默认目录为 `%LOCALAPPDATA%\Programs\Deep Legends`，与 perMachine=false / productName 的默认目录一致。 |
| P1-3 握手 | 外层等待 NSIS 子进程成功退出后通过继承的 stdout 发送固定 INSTALLED 标记；后端完成数据延续后才触发既有 `LOOT_QUIT update`。安装器持有真实 Electron 父进程句柄，等待正常退出，随后沿用应用 handoff 自动启动。父进程不退出时不启动第二份应用；60 秒后报告失败。 |
| P1-3 失败 | 后端异步等待安装完成；启动失败、NSIS 非零退出（含 1223 取消）或失败标记令状态 failed，不触发 quit。前端显示失败及发布页入口。便携迁移的 NSIS 进程检测分支遇到目标正在运行只中止，不调用 kill；安装版原有升级分支保留。 |
| P1-3 数据 | 后端默认 `os.UserCacheDir()/LOLLootAssistant`，Windows 即用户 Local AppData；便携与安装版均相同。Electron 沿用相同 appId/name/productName 和默认 `app.getPath('userData')`，未设置旁置目录，因此窗口配置、桌面日志、重启标记也共享原目录。 |
| P1-3 自定义旧目录 | 若设置了 `LOL_LOOT_DATA_DIR`，安装前先做完整私有暂存复制检查（不发布），避免已知复制错误后仍启动安装器。NSIS 完成后重新复制最新源目录、flush 诊断缓冲，成功后才退出；安装器等待父进程，首次新进程读取前目标已就绪。无需首次启动迁移标记。目标已有非空数据时保留、不覆盖；复制失败删除暂存、保留源数据并记 update_data_migration stage/result，当前程序继续运行。安装后复制失败也不会启动新进程，用户可修复后重试。复制拒绝符号链接、非普通文件及源目录内嵌目标。 |
| P1-4 | 两种状态均显示“立即升级”→原下载/校验进度→“立即重启升级”；移除原手动下载文案。发布页入口只在 failed 时显示。 |
| P1-5 | 旧便携 exe 不删除、不改写；新程序使用 NSIS 生成的桌面/开始菜单快捷方式。子进程清理 portable 环境变量和自定义后端数据目录变量，避免再次按便携版运行。 |

### 安装参数证据

实际读取当前 electron-builder 模板 `templates/nsis/installer.nsi`、`multiUser.nsh`、`include/allowOnlyOneInstallerInstance.nsh` 与 `getProcessInfo.nsh`。`/S` 控制内层静默安装；`/currentuser` 强制用户级 scope；`/D=` 最后传入目标，包含空格也由 NSIS 整段处理。外层 Go 壳负责等待退出码与自动启动，不能把 `/S` 当成外层参数。FIND_PROCESS 在发现目标进程时返回 0；便携分支以此中止安装。PowerShell 不可用时按进程名回退，可能保守中止，仍不会杀当前便携进程。所有这些路径的真实 Windows 行为尚待用户验收。

## P2：设置诊断与结算同步

| 项目 | 实现与核对 |
|---|---|
| P2-1 差值 | PersistedSettings / game.cfg / input.ini 收集全部普通设置项，game_settings_changed 增加排序 changed_keys，非按键值截到 32 字符并过滤路径/身份，最多 60 项、记录截断数。Input.ini 值只在内存保留摘要用于判断变化，日志仅名称；快照内部 AllValues 不序列化。保留 R184 原 camera 值与权限诊断。 |
| P2-1 LCU | 每次只读 GET game-settings / input-settings，记录完整 JSON payload 的 8 位摘要。客户端返回的所有可比较 Game.cfg 项逐一比较 A/B 对应文件项；不存在、不同或未知结构判 neither。这里按工单的“客户端保存的所有项”比较，并不要求文件额外项都在客户端返回中。 |
| P2-2 开关 | “安装与连接”原锁定区新增“保留对局内设置改动”，默认开启，状态保存在后端本地存储并通过 rig/status 返回；POST /api/rig/settings-sync 保存。没有新增说明段落；demo 同步支持。偏好文件损坏/缺字段保守关闭，读取不到文件采用默认开启。 |
| P2-2 时序 | game_start / in_game_60s 保存本局 A（后者覆盖启动阶段尚未写回的 B）；game_end 快照完成后等待 5 秒，重新读取稳定 B。连接、context、游戏 generation 与 ended 同时核验，下一局会使旧同步失效。 |
| P2-2 写入 | 仅比较 PersistedSettings 的 Game.cfg；本局有变化且 LCU 仍为旧值才 PATCH 对应节/字段，保持客户端类型。已为新值、分歧、缺失字段均不写；Input.ini 不同步。PATCH 前复核开关、阶段、目标安全路径、权限、mtime、size 与内容摘要。 |
| P2-2 核对/保存 | 一次 PATCH 后再 GET，逐项核对。随后只读 GET `/swagger/v3/openapi.json`；仅它明确公布无需必填参数/body 的 POST `/lol-game-settings/v1/save` 时调用一次。未公布不猜写接口，记录 save_result=not_advertised；必填 schema 未支持则 unsupported_schema。当前未连接真实 Windows LCU，不能宣称该 save 接口已在用户客户端存在或云端保存已验证。 |
| P2-2 失败 | 不重试写入，不改任何本地游戏配置文件。日志包括 changed_count、patched_keys（名称）、result 与阶段/save_result。关闭开关不访问 LCU；PersistedSettings 或所在 config 下 game.cfg 只读均 skipped_locked。失败 lcu_unavailable / verify_failed。 |
| 声明 | 隐私/自动写入声明补充默认启用的设置差值 PATCH；按键和本地游戏文件不写入。 |

## 验证

- Go 专项 `TestR186|TestR184|TestUpdate` 通过（1.657 秒）；installer 全量通过。
- 前端既有更新弹窗及 desktop 生命周期/NSIS 回归 37 项通过；R186 新增开关 DOM/保存/失败回退测试 3 项通过。
- 新增生产生命周期测试：真实 observe phase → game_end 快照 → 注入 5 秒时钟 → 原队列同步；核对 A/B、两种 LCU hash、日志无按键值、延迟前无 PATCH、下一局 generation 不写。
- 迁移三种分支、安装前复制失败不启动、嵌套目标拒绝；installed 原升级、portable 下载/校验/失败不退出均通过。
- 6 项变异全部实际断言 FAIL：portable Apply 拒绝、portable 弹窗手动入口、迁移覆盖已有数据、整份设置 PATCH、LCU 已是新值仍 PATCH、关闭开关仍同步。临时覆盖文件与输出位于 `/private/tmp/r186-mutants/`，生产源码没有恢复错误分支。
- `go vet ./backend` 与 `git diff --check` 通过。最终无缓存 `go test ./backend -count=1` 通过（233.381 秒）；Web/Desktop 全量 1061 项，1060 通过、1 项平台条件跳过、0 失败（237.547 秒）。R186 race 通过（2.356 秒）。
- Go 全量初轮暴露的隐私声明数量/落盘调用点清单、安装结果协程 panic 保护已修正，专项 1.613 秒通过后再跑全量。
- 初次 public NSIS 编译发现 getProcessInfo 提前加载 System 插件冲突，已把引用移至模板 customHeader（插件目录注册之后），并保留卸载器函数定义；相关 desktop 16 项通过。第二轮完整 `DEEP_LEGENDS_KEY_MODE=public ./build-desktop.sh` 成功：1654 个 Go 测试五分片、Go/installer vet 与测试、Windows 后端交叉构建、NSIS installer/uninstaller、外层安装器、包内 runtime 和 public 收据全部通过；最终源码与包内指纹 **4970535ffccd**。未嵌入 Riot Key，最终只保留 `-public` 安装包。日志 `/private/tmp/r186-public-build-final.log`。

- 安装包：`dist/desktop/Deep Legends Setup 0.12.51-public.exe`；SHA-256 `ab18ac0e97fa5fa76edd91457769ad0fcb410f8685254aba20e0225c02a87681`，与 public 收据和 SHA256SUMS-public.txt 一致。

## 真机验收边界

本机没有 Windows/真实游戏客户端。待用户验证：便携版下载与重启安装、取消安装旧程序继续运行、升级后设置/收藏/LP/选人配置延续、新快捷方式启动；开启保留设置，连续两局确认下一局没有恢复，导出新日志核对 changed_keys / game_settings_sync / save_result。
