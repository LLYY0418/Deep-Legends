# WORKLIST-R201：升级时主窗口没关、安装偏慢；设置文件只读时镜头模式写不进去；海斗开局卡片临时测试选人

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-03。基线：0.12.63（`c21482dba3c7`）。

日志 `lol-loot-diagnostics-1003-2002.jsonl`：08:04–10:54 为 0.12.60（`530db182f4c1`），10:55 起为 0.12.63。

## 已在真机确认正常（不改）

- **R195 安装识别**：0.12.63 启动 `update_install_detection result=installed`（`root_is_default_dir=true`）。
- **R196 下载**：测速 4 条线路，选 `gh-proxy.com`，106 MB **10.7 秒**下完（平均约 10 MB/s），无换线。
- **R200 卡片接口**：真机实际可用的是 **`GET /lol-lobby-team-builder/champ-select/v1/subset-champion-list`**，199 次全部 200、整数数组、每次 3 个 ID。`/help` 里找不到 subset 路径（`subset_help_matches paths=[]`），靠预设候选命中。三局卡片分别是 `[107,141,75]`、`[14,33,58]`、`[245,246,77]`，都不在选用序列 `[22,136,48,57,13]` 里，所以软件按设计没有选。写进 R200 账本，把这个接口标为"真机已核实"。
- **R197**：`live_roster_post_game_reveal reason=ok revealed=1`，功能本身生效。用户反馈结束后页面会清空，赛后补全意义不大，**用户决定保持现状，不再改**。

---

## P1　升级：开始安装时主窗口要关掉；安装再快一点

### 现象与证据

用户截图：安装程序窗口显示"正在升级 18% · 正在校验安装包…"，后面的 Deep Legends 主窗口（还停在更新弹窗）**仍然开着**。

| 时间 (UTC) | 事件 |
|---|---|
| 10:54:03 | 点立即升级，开始测速 |
| 10:54:04～07 | 4 条线路测速完成，最慢的直连用了 4.9 秒；**等全部测完才开始下载** |
| 10:54:13 | 下载完成（10.7 秒，含测速约 4 秒） |
| 10:54:15 | `update_data_migration stage=pre_install`，启动安装程序 |
| 10:55:28 | 0.12.63 启动 |

从启动安装程序到新版本打开约 **73 秒**，其中各阶段各占多少日志里看不出来（安装程序没有回传计时）。

安装程序（`installer/`）在还没开始解压时显示"正在校验安装包…"（`progress.go progressStage(0,0)`）。这段时间里包含：等旧程序退出、NSIS 自身对 110 MB 安装包做 CRC 校验、然后才开始解压。

### 修改

1. **开始安装时立刻关掉主窗口。** 后端确认安装程序已启动后，desktop 主进程立即隐藏所有窗口（主窗口、启动画面、弹窗），然后正常退出（停后端、释放文件）。屏幕上只留安装程序窗口。安装失败需要回到软件时，由安装程序按现有逻辑重新打开软件（不变）。
2. **安装程序等旧程序退出，而不是撞上占用的文件。** 安装版和便携版两条路径都把父进程 PID 传给安装程序（便携版已有 `portableUpdateCommandLine(..., parent)`，安装版的 `updateCommandLine` 也要加），安装程序等这个进程退出（最多 15 秒，超时按现有处理）后再开始写文件。
3. **跳过重复校验。** 软件已经用 SHA-256 校验过安装包，由软件启动的升级给 NSIS 加 `/NCRC`，跳过它自己的 CRC 校验。用户手动双击安装时不加，行为不变。
4. **测速不等最慢的线路。** 任一线路测完 1 MB 后，最多再等 1 秒；到时已完成的线路里选最快的开始下载，没测完的取消。其他规则（慢速换线、记住线路）不变。
5. **安装计时诊断。** 安装程序在每个阶段写一个时间点到数据目录下的 `update-install-timing.json`：`installer_start`、`parent_exited`、`extract_start`、`extract_done`、`copy_done`、`relaunch`。新版本启动后读一次，记 `update_install_timing`（各阶段耗时毫秒，不记路径），然后删除文件。下一份日志据此判断剩余时间花在哪，再决定要不要改压缩方式。

### 测试

1. 安装程序启动成功 → desktop 收到通知后所有窗口 `hide()` 并调用退出（Node 测试 mock BrowserWindow）。
2. 安装版和便携版的命令行都带父进程 PID；软件发起的升级带 `/NCRC`，`setupCommandLine` 用于手动安装时不带。
3. 测速：一条 0.5 秒完成、其余 5 秒 → 1.5 秒内选定线路开始下载；两条在 1 秒窗口内完成 → 选更快的。
4. 计时文件存在 → 记一条 `update_install_timing` 并删除；文件损坏 → 记 `result=invalid` 并删除，不报错。

---

## P2　镜头模式：选了自由镜头，进游戏还是锁定

### 证据

| 时间 (UTC) | 事件 |
|---|---|
| 10:55:46 | 用户在维护里选「自由镜头」：`game_camera_mode_apply stage=settings_changed target=free target_value=0`，`lcu_result=unavailable` |
| **10:55:48** | **`rig_maintenance action=settings-lock result=ok settings_locked=true`**：用户点了「设为只读」，`PersistedSettings.json` 变成只读 |
| 10:59:04 起每局 | `stage=champselect`：`PersistedSettings.json` **仍是 2**，`file_result=read_only`（跳过）；`game.cfg` 从 2 改成 0；`lcu_result=unavailable` |
| 12:00:36 | `in_game_60s`：`camera_mode_matches_target=false`，`PersistedSettings.json` 的 `Game.cfg.General.CameraMode=2` |

另外，客户端设置 `/lol-game-settings/v1/game-settings`（每次都是 200）里跟镜头有关的只有 `General.SnapCameraOnRespawn`、`HUD.CameraLockMode` 两项，**根本没有 `CameraMode`**。所以 `lcu_result=unavailable` 不是接口坏了，而是客户端那份设置里本来就不存镜头模式。

### 结论

1. 镜头模式只存在本地文件里，游戏开局读的是 `PersistedSettings.json`（R186 已证实开局时 `game.cfg` 会按它重写）。
2. R198 只改成功了 `game.cfg`。`PersistedSettings.json` 因为用户刚好设成了只读，被 R198 P2-4 的规则跳过，所以开局仍是锁定。

### 修改

1. 「进入游戏时的镜头模式」不是"不修改"，且 `PersistedSettings.json` 是只读时：在 ChampSelect / GameStart（游戏进程启动前）**临时去掉只读 → 只改 `CameraMode` 这一项 → 立即恢复只读**，再读一次核对值和只读状态。沿用 R198 的路径校验和"只改这一项、其他字节不变"的写法。
2. 恢复只读失败时重试一次，仍失败就记 `relock_failed`，并在维护页现有的只读状态显示里如实显示"可写入"（不新增文字）。
3. `lcu_result`：客户端设置里没有 `CameraMode` 时记 **`not_present`**，不再记 `unavailable`；这种情况不算失败。
4. `game_camera_mode_apply` 增加 `relocked`（布尔）。`file_result` 增加 `ok_relocked`。
5. 只读锁的其他行为不变：对局内的改动仍写不回文件，R186 的结算同步仍是 `skipped_locked`。

### 测试

1. 只读 `PersistedSettings.json`（CameraMode=2）+ 目标自由 → 文件变成 0，写后仍是只读，除这一项外字节不变，`file_result=ok_relocked relocked=true`。
2. 恢复只读第一次失败、第二次成功 → `relocked=true`；两次都失败 → `relock_failed`。
3. 客户端设置里没有 `CameraMode` → `lcu_result=not_present`，不发 PATCH。
4. 对局中（InProgress）→ 不去只读、不写（R198 原规则）。
5. 选"不修改" → 不碰文件和只读属性。

---

## P3　海斗开局卡片：临时测试选人（之后删除）

### 背景

R200 的读卡片已在真机确认可用（见文首），但三局的卡片都没有选用序列里的英雄，"按卡片锁定"这一步还没在真机上跑过。用户要求先临时加一个测试：卡片里没有序列英雄时，也随便选一张，确认锁定能成功；以后删掉。

### 修改

1. 只在卡片模式（`allowSubsetChampionPicks=true`）且**卡片里没有选用序列英雄**时生效：选卡片列表里的**第一张**，按用户设置的策略（亮出 / 锁定、锁定等待）执行。「避让队友预选」照常：第一张被队友预选了就取下一张。
2. **界面上不显示**：「本局记录」不加这条，也不显示 R200 的"开局卡片中没有选用序列里的英雄"。只记诊断 `champselect_trace stage=subset reason=test-first-card champion_id=…`，加上现有的 postflight 结果。
3. 用一个常量开关实现（例如 `champSelectSubsetTestFirstCard = true`），代码旁注明"R201 临时测试，真机验证后删除"。不加设置项。
4. 卡片里有序列英雄时，完全按 R200 的逻辑走，这个开关不影响。
5. 账本写明这是临时逻辑，删除另开工单。

### 测试

1. 卡片 `[107,141,75]`、序列里没有 → 选 107，`reason=test-first-card`；「本局记录」里没有新增条目。
2. 107 被队友预选 → 选 141。
3. 卡片 `[107,136,75]` → 选 136（R200 逻辑），不出现 `test-first-card`。
4. 开关设为 false → 卡片里没有序列英雄时不选（R200 原行为）。

---

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go test ./installer/...`（能跑的部分）、`go vet ./backend`、`git diff --check` 全绿；变异：P1-1 不隐藏窗口、P2-1 跳过只读文件、P3 在 UI 里显示测试选人 → 对应测试 FAIL。
- 版本递增；`docs/WORKLIST-INDEX.md` 加 R201，R200 那行备注"子集接口真机已核实；临时测试选人见 R201 P3"，R197 那行备注"用户决定保持现状"。账本 `docs/r201-execution-ledger.md`。
- 按 R199 的规则发布到 GitHub Latest（release id、`isLatest=true`、匿名 `latest.json` 版本号）。

## 真机验收（用户）

1. 从 0.12.63 在线升级：点立即升级后，主窗口马上消失，只剩安装窗口；装完自动打开新版本。
2. 「设为只读」保持开着，「进入游戏时的镜头模式」选「自由镜头」，打一局：开局就是自由镜头。
3. 打一局海斗：开局卡片里没有你序列里的英雄时，软件会自动选第一张（测试用）；有序列英雄时选序列英雄。
4. 导出日志：`update_install_timing` 有各阶段耗时；`game_camera_mode_apply file_result=ok_relocked`，`in_game_60s camera_mode_matches_target=true`；`stage=subset reason=test-first-card` 后有 postflight 生效记录。
