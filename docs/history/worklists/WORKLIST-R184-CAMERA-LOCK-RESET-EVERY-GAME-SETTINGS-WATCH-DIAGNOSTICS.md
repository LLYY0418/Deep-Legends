# WORKLIST-R184：每局进游戏都是锁定视角，改了下局又恢复（游戏设置文件监测诊断）

诊断人：Claude（只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.48（R183 之后）。

## 用户描述

现在每次进入游戏都默认锁定视角，在游戏里改成不锁定，下一局又变回锁定，每局都要重新设置。用软件"安装与连接"页的"游戏设置锁定 / 解除锁定"操作过，没有用。

## 已核对的事实

- 软件**没有任何**写视角、写游戏设置、写按键的代码。对游戏设置相关的操作只有一处写入：`game_settings_lock.go` `handleSettingsLock` 对 `PersistedSettings.json` 做 `chmod`（0444 只读 / 0644 可写）。读取的地方有：`readRigStatus`（只读 `stat`），`item_set_diagnostics.go` 的 `readPersistedDisplayDiagnostic` / game.cfg 读取（只取 HUD 缩放）。
- 游戏内实时数据接口只调用了 `liveclientdata/allgamedata`、`playerlist`，都是只读接口，不涉及镜头。
- 现有日志只有 `rig_status_read result=ok`，**没有记录文件是否只读、文件何时被谁改过、视角相关设置是什么值**，所以无法判断原因。
- 可能的原因（需要诊断区分）：
  1. 设置文件仍是只读：游戏结束时写不回去，每局都恢复成锁定那一刻的值。
  2. 客户端在开局前用服务器上保存的设置覆盖了本地文件（设置云同步），服务器上那份是锁定。
  3. 游戏实际读写的不是软件定位的那个文件（国服安装目录结构下可能有多份 `Config`），所以锁定/解锁操作作用在了另一份文件上。
  4. 游戏本身某个"开局锁定视角"类的选项被设为开启，与文件写入无关。

本工单**只加诊断，不改任何游戏设置、不改锁定逻辑**。

## P1　设置文件快照 `game_settings_watch`

在以下时机各记一条（`stage` 字段）：

| stage | 时机 |
|---|---|
| `app_start` | 连接到客户端后第一次 |
| `champselect` | 进入 ChampSelect |
| `game_start` | 进入 GameStart / InProgress（首次） |
| `in_game_60s` | InProgress 后 60 秒 |
| `game_end` | 进入 WaitingForStats / PreEndOfGame / EndOfGame 首次 |
| `lobby_after_10s` | 对局结束回到 Lobby / 结算后 10 秒 |
| `lobby_after_60s` | 对局结束后 60 秒 |
| `lock_action` | 用户点击锁定 / 解除锁定，操作完成后立即再记一次 |

每条记录内容（每个候选文件一组）：

1. **候选文件**：除了 `locateGameSettings` 定位的那份 `PersistedSettings.json`，再列出以下位置（存在才读）：
   - `installRoot/Config/PersistedSettings.json`
   - `installRoot/../Game/Config/PersistedSettings.json`
   - `installRoot/Game/Config/PersistedSettings.json`
   - 同目录下的 `game.cfg`、`input.ini`
   每个候选记：`path_kind`（用相对安装根目录的相对路径表示，例如 `Config/PersistedSettings.json`、`../Game/Config/PersistedSettings.json`；**不写绝对路径和用户名**），`exists`、`is_located_target`（是否就是锁定按钮操作的那一份）、`read_only`（Windows 只读属性 / 权限位）、`size`、`mtime_utc`、`content_hash8`（整个文件 SHA-256 前 8 位，用于判断有没有被改写）。
2. **视角相关设置**：对 `PersistedSettings.json` 和 `game.cfg`，取所有**名称里含 `Camera` 或 `Lock`（不区分大小写）**的设置项，记 `section.name = value`（值原样字符串，长度截到 32）。`input.ini` 只记含 `Camera` 的按键项名称是否存在，不记具体键位。
3. **客户端里的游戏设置**：只读 GET `/lol-game-settings/v1/game-settings`，同样只取名称含 `Camera` / `Lock` 的项；接口不存在或失败时记 `lcu_settings: "unavailable"` 和 HTTP 状态。不调用任何写入接口。
4. 与上一条同一文件的快照对比：`changed_since_prev`（hash 是否变化）、`camera_changed_keys`（哪些视角相关项变了，记旧值 → 新值）。

不记录：账号、召唤师名、绝对路径、非视角相关的设置值。

## P2　变更归因 `game_settings_changed`

两次快照之间任一候选文件的 hash 变化时，额外记一条：

- `path_kind`、`between`（例如 `game_end → lobby_after_10s`）、`camera_changed_keys`、`read_only_before/after`；
- `writer_guess`：
  - 发生在 `game_start` 之前（ChampSelect / GameStart 之间）→ `client_before_launch`；
  - 发生在对局中或刚结束 → `game`；
  - 软件自己的锁定 / 解锁操作导致的权限变化 → `app_lock_action`（软件本身不写内容，内容 hash 变化不可能来自软件）。

同时，`game_end` 之后 60 秒内文件**没有变化**、而且文件是只读 → 记 `game_settings_write_blocked_suspected`。

## P3　锁定操作结果补全

`rig_maintenance action=settings-lock` 和 `rig_status_read` 增加 `settings_locked`（布尔）、`path_kind`。锁定 / 解锁成功后立即触发一次 `stage=lock_action` 快照。

## 约束

- 全部是只读操作（`os.Stat`、限长读取、LCU GET）。文件读取沿用 `readBoundedItemSetDiagnosticFile` 的路径校验与大小上限（256 KB），拒绝符号链接和安装目录外路径。
- 每个时机每个文件最多读一次；同一局重复进入同一阶段不重复记录。
- 读取在后台 goroutine 中进行，不阻塞阶段处理和界面。
- 不新增界面文字。

## 测试

Go：

1. 临时目录构造国服目录结构，两份 `PersistedSettings.json`（`Config/` 与 `../Game/Config/`）；快照列出两份，`is_located_target` 只对定位的那份为真；记录里只有相对路径。
2. 文件里含 `CameraLockMode`、`SnapCameraOnRespawn` 等含 Camera/Lock 的项和其他项 → 只记录含 Camera/Lock 的项；其他值不出现在事件里。
3. 只读文件 → `read_only=true`；chmod 改成可写后的 `lock_action` 快照 → `read_only=false`，`writer_guess=app_lock_action`，内容 hash 不变。
4. `game_end` 与 `lobby_after_10s` 之间修改文件中的视角项 → `game_settings_changed`，`writer_guess=game`，`camera_changed_keys` 正确显示旧值 → 新值。
5. ChampSelect 与 `game_start` 之间修改 → `writer_guess=client_before_launch`。
6. 只读文件、`game_end` 后 60 秒无变化 → `game_settings_write_blocked_suspected`。
7. LCU `/lol-game-settings/v1/game-settings` 返回含 Camera 项 → 只记录这些项；404 → `unavailable` 且不报错。
8. 隐私：构造包含用户名的绝对路径和账号信息，断言事件中不出现；`input.ini` 不出现键位值。
9. 同一局同一阶段重复触发 → 只记一次。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 只读定位的那一份文件，不扫描候选路径 | 测试 1 FAIL |
| 记录全部设置项 | 测试 2 FAIL |
| 不按阶段去重 | 测试 9 FAIL |

`node --test backend/web/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R184；账本 `docs/history/ledgers/r184-execution-ledger.md`。
- 本工单不修复问题，只为下一份日志提供证据；修复另开工单。

## 真机操作（用户）

1. 安装新版本后，**先打开软件**，再开始一局（任何模式都可以）。
2. 进游戏后把视角改成你想要的（不锁定），正常打完。
3. 结束后**不要关软件**，在大厅等 1 分钟，再开第二局；进游戏后看视角是不是又变回锁定。
4. 第二局结束后等 1 分钟，导出日志发我。期间如果点过"锁定 / 解除锁定"，也请告诉我大概什么时候点的。
