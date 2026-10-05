# WORKLIST-R212：关闭已真机验证的工单；在线升级安装再提速；中转请求失败分类

诊断人：Claude（读日志 `lol-loot-diagnostics-1004-2118.jsonl` + 只读核对源码）。执行人：GPT。日期：2026-10-04。
基线：0.12.71 工作区。R211 可以并行；本工单的 P2、P3 与 R211 合并到下一个版本发布。

日志覆盖 2026-10-04 16:57～21:18（北京时间）：前半段是 0.12.68（指纹 `a24a41472d2b`），20:48 在线升级到 0.12.71（指纹 `696da05d0ad0`），之后打了一局海斗（game `9015098843`，queue 2400），结束后回到大厅。

---

## P1　工单索引收口（只改文档）

### 用户确认关闭（第一部分，已发布版本的待验项）

用户 10-04 确认以下工单都可以关闭。`docs/WORKLIST-INDEX.md` 状态改为「已关闭：用户确认（R212）」，各账本末尾加一行同样的说明：

R153、R156、R195、R196、R197、R198、R202、R203、R204、R205。

其中三张和升级有关，本次日志里有直接证据，账本里补上：

| 工单 | 日志证据（0.12.68 → 0.12.71，北京时间 20:48） |
|---|---|
| R196 | 测速 4 个源：gh-proxy 763 KB/s、ghfast 481 KB/s、直连和 ghproxy.net 到 2.3 秒时取消；选 gh-proxy；实际下载 8.6 秒，平均 12.6 MB/s，没有换线 |
| R204 | `riot_key_migrated result=skipped_exists`；`update_install_timing result=ok`，各阶段都有记录（见 P2） |
| R205 | `update_shortcut_state result=ok`、`icon_location_stable=true`、`created_time_changed=false`、`restore_results.current=unchanged`；`update_install_detection result=installed` |

R204 里「安装 5 秒」那部分没有达成，转到本工单 P2，不再挂在 R204 上。

### 第二部分（0.12.71，R206～R210）：用户确认 + 日志核对

| 工单 | 用户反馈 | 日志核对 | 结论 |
|---|---|---|---|
| R206 | 韩服战绩能加载 | `app_start riot_key_source=relay`；`riot_relay_probe ok`（1.16 秒）；两次 `riot_overview_cost`：10/10 局读到，`matches_failed=0`，`rate_limited_count=0`，耗时 1.7 秒 / 1.5 秒。斗魂第一名对局 6 次读取都是 10/10。 | 关闭。升级提速转到 P2 |
| R207 | 海斗开局后推荐不变空 | `live_recommendation_render`：选人时请求在途，短暂为空；进入 InProgress 后 `augment_rows=106`、`has_build=true`、`exact_hit=true`；13:15 掉线重连（Reconnect）后同样是 106 行。 | 关闭 |
| R208 | — | 两个 10 分钟窗口：46 次请求 / 15 次请求，`rate_limited=0`，没有触发冷却或额度熔断。另有 8 次 / 5 次 `failures.http`，见 P3。 | 关闭。Cloudflare 每日用量由用户自行留意，不作为本工单的验收项 |
| R209 | 打完进收藏页显示正常 | 收藏 1183 件正常渲染；对局后 `dirty_rescan` 渲染为 `unchanged-suppressed`，页面保持可见。没有出现名额修正或观察器兜底事件（这次没有触发泄漏）。 | 关闭 |
| R210 | 设置卡片可以 | 21:15:30 `live_scope_reset reason=left_end_of_game`，`phase=Lobby`、`previous_game_id=9015098843`、`cleared_recommendations=5`，对局页已清空。这局结束时没有隐藏玩家，所以后端赛后补全那条路径（`live_roster_post_game_reveal`）这次没走到；前端的清空已经验证。 | 关闭 |

R206～R210 在索引中改为「已关闭：用户确认 + 日志核对（R212）」，各账本末尾补上表中对应的证据行。

---

## P2　在线升级：安装阶段再提速

### 本次实测（0.12.68 → 0.12.71）

| 时间（北京） | 阶段 | 耗时 |
|---|---|---|
| 20:48:26.6 | 用户点「立即升级」（`riot_key_migrated`） | — |
| ≈20:48:29.7 | 外层安装器启动 | 约 3.1 秒（旧程序退出、启动安装器） |
| ≈20:48:31.0 | `parent_exited` | 1.2 秒 |
| ≈20:48:38.0 | `uninstall_old_start` | **7.0 秒，目前没有任何细分** |
| ≈20:48:44.4 | `uninstall_old_done` | **6.5 秒** |
| ≈20:48:51.8 | `extract_done` | 7.2 秒 |
| ≈20:48:52.3 | 重新打开 | 0.5 秒 |
| 20:49:00.8 | 窗口就绪（`desktop_startup total=4464`） | 4.5 秒 |

从点击到能用约 34 秒，其中安装器本身 22.5 秒（`total_ms=22545`）。下载在后台完成（8.6 秒），不算在等待时间里。

### 原因分析（源码已核对）

1. **`parent_exited → uninstall_old_start` 的 7 秒**：这段在 `installer/install_windows.go` `install()` 里是**串行**执行的：
   - `validateUpgradeDestination` / `prepareDestination`（写入探测、读磁盘剩余空间）；
   - `releasePayload()`：把内嵌的约 100 MB NSIS 安装程序**完整写到 TEMP**。这一步要等旧程序退出之后才开始，其实不必等；
   - `newWindowsShortcutUpdate`：读快捷方式状态；
   - 启动 NSIS。新写出的 100 MB exe 第一次运行，Windows 会先扫描它；
   - NSIS 的 `customCheckAppRunning`（`desktop/nsis/installer.nsh` 71–94 行）**第一行无条件执行 `IS_POWERSHELL_AVAILABLE`**，会启动一次 PowerShell。这个结果只有 `FIND_PROCESS` / `_CHECK_APP_RUNNING` 才用得到，而升级时带 `--parent-exited` 根本不会走到它们。
   
   目前没有细分计时，无法确定每一步占多少，所以先补计时，再改。
2. **`uninstall_old` 的 6.5 秒**：跑的是旧版本自带的卸载程序。electron-builder 调旧卸载程序时只传 `--updated`，**不传 `--parent-exited`**；而卸载程序里也会执行同一个 `customCheckAppRunning`，于是走了 `IS_POWERSHELL_AVAILABLE` + `_CHECK_APP_RUNNING`（用 PowerShell 列进程），再删文件。这一段跑的是**旧版本**的代码，所以改动要从「新版本作为旧版本被升级」那一次才生效，也就是要再隔一个版本才能看到效果。
3. **解压 7.2 秒**：LZMA 解压约 250 MB 的程序文件。改成不压缩，安装包会大到 300 MB 左右，下载多花的时间比省下来的多，**本工单不改**。

### 修改

1. **补细分计时**（先做，并保证这次升级就有数据）：在 `update_install_timing.stages_ms` 里加：
   - `parent_exited_to_payload_released`
   - `payload_released_to_nsis_start`（`cmd.Start()` 之前）
   - `nsis_start_to_oninit`（NSIS `.onInit` 第一行写一个标记，复用现有 `DLUpdateTiming` 宏）
   - `oninit_to_check_done`（`customCheckAppRunning` 结束时）
   - 旧卸载程序内部没法加（旧代码），保持现有的 `uninstall_old` 一段。
   
   时间戳的写入方式沿用现有阶段，不新开文件。
2. **安装包提前释放**：`releasePayload()` 改到「等待旧程序退出」的同时并行进行（`waitUpgradeParent` 开始时就启动一个 goroutine 释放，等退出后再 join）。释放失败的处理和现在一样（报「无法释放安装包…」）。临时目录清理不变。
3. **不需要时不启动 PowerShell**：`customCheckAppRunning` 里把 `IS_POWERSHELL_AVAILABLE` 挪进真正要用它的两个分支（`--portable-upgrade` 分支，以及会执行 `_CHECK_APP_RUNNING` 的分支）。带 `--updated --parent-exited` 时一次 PowerShell 都不启动。
4. **卸载程序里同样跳过**：卸载程序上下文（`BUILD_UNINSTALLER`）里，`${isUpdated}` 为真时跳过 `_CHECK_APP_RUNNING` 和 `IS_POWERSHELL_AVAILABLE`。理由：只有新版安装器才会用 `--updated` 调起旧卸载程序，而它此时已经确认旧程序退出。用户手动从「应用和功能」卸载时，行为不变。
   - 在账本里写清楚：这一条要等用户**从 0.12.72（或包含本改动的版本）再升级到下一版时**才会体现在 `uninstall_old_ms` 上。
5. **不改**：解压方式、压缩等级、快捷方式保护逻辑（R205）、Key 迁移（R204）、`/NCRC`（已有）。

### 测试

1.（Go，installer）`releasePayload` 在等待旧程序退出期间就开始执行：用可控的 `waitParent` 桩阻塞 2 秒，断言释放完成时间早于 `parent_exited`；释放失败时仍然报原来的错误。
2.（Go）`update_install_timing` 输出包含 4 个新阶段，并且单调递增；缺失的阶段不输出 0。
3.（Node，`installer-nsh.test.cjs`）生成的 `installer.nsh` 里，`IS_POWERSHELL_AVAILABLE` 只出现在 `--portable-upgrade` 分支和 `_CHECK_APP_RUNNING` 分支之前；`--updated --parent-exited` 路径上没有。
4.（Node）卸载程序上下文：`${isUpdated}` 时跳过检查；非升级卸载仍会检查。
5. R186（便携版升级）、R201、R204、R205、R206 的安装器测试全部通过。

变异（必须 FAIL）：释放安装包放回等待之后 → 测试 1；`IS_POWERSHELL_AVAILABLE` 放回第一行 → 测试 3。

### Windows 实测（GPT 用 Windows runner 或本机虚拟机，用户再真机复核）

- 在 Windows runner 上实际执行一次「旧版 → 新版」静默在线升级（沿用 R206 采集真实安装阶段文件的方法），把新的 `stages_ms` 记进账本，和本工单上面那张表对比。
- 目标：`parent_exited → uninstall_old_start` 从 7.0 秒降到 2 秒以内。达不到就按细分计时说明剩下的时间花在哪一步，不要硬改其他阶段。

---

## P3　中转请求的「失败」计数把正常的 404 也算进去了

### 现状

`riot_relay_request_summary` 两个窗口分别是 46 次请求里 `failures.http=8`、15 次里 `failures.http=5`。同一时间两次战绩加载都是 10/10 成功、`matches_failed=0`，用户也确认韩服战绩正常。

`riot_api.go` 约 525–540 行：除了 401/403/429/5xx，**其他所有 4xx 都记成 `http`**。韩服的「当前对局」接口在玩家不在游戏中时本来就回 404，账号或召唤师查不到也回 404，这些都是正常结果。现在日志里看起来像有三分之一的请求失败，以后排查时会误导人。

### 修改

1. 404 单独记成 `not_found`，不算进 `failures`；摘要里另加 `not_found` 计数。
2. 其他 4xx 继续记为 `http`，同时在摘要里加 `http_statuses`（状态码 → 次数）和按接口类别（`account` / `summoner` / `match` / `league` / `spectator` / `mastery` / 其他）的计数，不记 PUUID 和名字。
3. 只改分类和日志，不改重试、冷却、熔断逻辑。R208 的冷却与额度测试保持通过。

### 测试（Go）

6. 观战接口返回 404：`failures` 为空，`not_found=1`。
7. 返回 400：`failures.http=1`，`http_statuses["400"]=1`。
8. R208 冷却、日额度熔断测试通过。

---

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go test ./installer -count=1`、`go vet ./backend ./installer`、`git diff --check` 全绿。
- `docs/WORKLIST-INDEX.md` 加 R212；按 P1 更新 R153～R210 各行状态。账本 `docs/r212-execution-ledger.md`。
- 版本与 R211 合并，按 R199 规则发布。

## 真机验收（用户）

1. 下一次在线升级后导出日志：`update_install_timing` 里有 4 个新阶段，`parent_exited → uninstall_old_start` 明显变短。
2. 再下一次升级（从包含本改动的版本升级）时，`uninstall_old_ms` 变短。
3. 日志里 `riot_relay_request_summary` 不再把 404 记成失败。
