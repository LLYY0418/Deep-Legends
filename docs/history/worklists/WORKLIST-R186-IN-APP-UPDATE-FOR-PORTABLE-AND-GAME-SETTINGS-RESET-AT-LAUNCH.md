# WORKLIST-R186：便携版也能在软件内直接升级 + 每局开局游戏设置被还原（R184 诊断结论与修复）

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.49（指纹 7436ba4ac006，R184 已合入）。如果 R185 已合入，以合入后的源码为准，三者不冲突。

---

## P1　便携版也走软件内升级（下载 → 校验 → 重启升级）

### 用户诉求

更新弹窗显示"便携版请从发布页下载新版程序"，只有"打开发布页"按钮。用户要求：直接给"立即升级"按钮，软件内下载、显示进度、重启完成升级，不要再手动去下载。

### 现状（源码已核对）

- 软件内升级流程已经存在：下载安装包 → SHA-256 校验 → "立即重启升级" → 启动 NSIS 安装程序覆盖安装 → 自动重新打开（`update_download.go` `Download` / `Apply`，`update_platform_windows.go` `launchUpdateInstaller`）。
- 但只对"已安装版"开放：`update.go:226` `u.status.Portable = u.installDir == ""`。`installedUpdateDirectory`（`update_platform_windows.go:23`）在以下情况返回空，被判定为"便携版"：
  1. 环境变量 `PORTABLE_EXECUTABLE_FILE` / `PORTABLE_EXECUTABLE_DIR` 存在（便携版 exe）；
  2. 程序目录下没有 `Uninstall Deep Legends.exe`；
  3. 注册表 `HKCU\...\Uninstall` 里找不到 `DisplayName = "Deep Legends"` 且 `InstallLocation` 与程序目录一致的项。
- 被判为便携版后，`Apply()` 直接拒绝（`update_download.go:364`），前端只显示发布页链接（`app.js:3849`）。
- 日志里**没有记录是哪一条原因**判成了便携版（`update_check_succeeded` 只有版本号和状态）。
- 发布页只提供安装包 `Deep-Legends-Setup-<版本>[-public].exe`，`desktop/package.json` 的 Windows 目标只有 `nsis`。

### 修复

P1-1　**诊断**：启动时记一条 `update_install_detection`：`result`（`installed` / `portable_env` / `no_uninstaller` / `registry_missing` / `registry_location_mismatch`）、`registry_display_found`（布尔）、`location_matches`（布尔）。不记任何路径。

P1-2　**非安装版也能下载**：`Download()` 对便携版、旧版（`ManualOnly`）同样可用，沿用现有下载、断点、镜像、SHA-256 校验和磁盘空间检查。

P1-3　**非安装版的"重启升级"**：校验通过后：
1. 启动安装包，用静默安装参数装到**默认的当前用户安装位置**（NSIS `perMachine=false` 的默认目录），安装完成后自动启动新安装的程序（沿用现有安装版升级用的"装完自动重新打开"机制；执行时核对 electron-builder NSIS 支持的参数，例如 `/S` 和安装后运行的方式，写进账本）。
2. 当前程序正常退出（沿用现有 `LOOT_QUIT update` 退出流程）。
3. **数据延续**：执行时核对本地数据目录（后端 `localStore.root`、Electron `userData`）在便携版和安装版下是不是同一个位置。如果便携版的数据放在程序旁边，新安装版启动时要能读到原来的数据：首次启动检测到"由便携版升级而来"（安装时传入的标记或升级前写入的迁移标记），把旧数据目录复制到新位置，然后再读取。复制失败不删除旧数据，并在日志里记 `update_data_migration` 的结果。设置、收藏记录、LP 记录、选人配置等都不能丢。
4. 安装失败或用户在 UAC / 安装界面取消：当前程序不退出，弹窗显示失败，保留"打开发布页"作为备用。

P1-4　**弹窗**：
- 非安装版不再显示"便携版请从发布页下载新版程序"。与安装版一样：`立即升级` → 下载进度 → `立即重启升级`。
- "打开发布页"只在下载或安装失败时作为次要入口出现。
- 不新增说明文字；沿用安装版现有的进度与就绪文案。

P1-5　**旧便携版文件**：升级后旧的便携版 exe 保持原样，不删除、不改动。新版本使用安装后生成的桌面快捷方式。

### 测试

Go：
1. 五种检测结果分别产生对应的 `update_install_detection.result`，日志里没有路径。
2. 便携版状态下 `Download()` 成功，`Apply()` 调用注入的启动函数，参数为默认安装位置的静默安装命令（不再返回"便携版请从发布页下载"错误）。
3. 安装版的原有升级路径不变（原测试保留）。
4. SHA-256 不匹配：便携版同样拒绝启动安装包。
5. 启动安装包失败：状态变为 `failed`，当前进程不退出。

Node（前端）：
6. 便携版状态下弹窗的主按钮是"立即升级"，不出现"便携版请从发布页下载新版程序"；下载中显示进度；就绪后主按钮为"立即重启升级"。
7. 失败状态下出现"打开发布页"。

数据迁移（Go 或 desktop Node 测试，按实际实现位置）：
8. 旧位置有数据、新位置为空 → 复制完成，内容一致；新位置已有数据 → 不覆盖；复制失败 → 旧数据保留，记录失败结果。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 恢复 `Apply()` 对便携版的拒绝 | 测试 2 FAIL |
| 前端便携版仍显示发布页文案 | 测试 6 FAIL |
| 迁移时覆盖已有数据 | 测试 8 FAIL |

---

## P2　每局开局，游戏设置被还原成同一个旧版本（R184 诊断结论）

### 证据

日志 `lol-loot-diagnostics-1002-1729.jsonl`，0.12.49（7436ba4ac006），08:34–09:29Z，三局。用户开局前已点"解除锁定"。

1. **文件不是只读**：所有快照 `read_only=false`，没有 `game_settings_write_blocked_suspected`。软件的锁定功能与本问题无关。
2. **只有一份设置文件**：游戏用的就是 `../Game/Config/` 下的 `PersistedSettings.json` 和 `game.cfg`，也是软件定位的那份；其他候选路径都不存在。
3. **每一局都在来回切换两个固定版本**：

| 时间 | 阶段 | PersistedSettings.json | game.cfg |
|---|---|---|---|
| 08:34:31 | 开局（软件刚启动） | **A** `19b0ab77`（56109 字节） | **A** `c0486464` |
| 08:45:51 | 第 1 局结束 | **B** `75624da7`（57570 字节） | **B** `2aaa04a2` |
| 08:47:05 | 第 2 局开局前 | B | B |
| 08:48:05 | 第 2 局开局 60 秒 | **A**（08:47:17 写入） | **A**（08:47:16 写入） |
| 09:04:51 | 第 2 局结束 | **B** | **B** |
| 09:08:20 | 第 3 局开局前 | B | B |
| 09:09:20 | 第 3 局开局 60 秒 | **A**（09:08:30 写入） | **A**（09:08:27 写入） |
| 09:28:43 | 第 3 局结束 | **B** | **B** |

   - 每局进游戏约 10 秒时，两个文件都被改写回**完全相同的旧版本 A**（哈希一模一样）；每局结束时，游戏又写成**完全相同的版本 B**。
   - B 是用户在对局里改过之后的设置；A 是开局时被还原出来的旧版本。也就是说，**用户在对局里的改动，下一局开局时就被还原掉了**，这就是"每次进去都要重新设置"。
4. **已记录的视角相关项在 A、B 里完全一样**：`HUD.CameraLockMode=0`、`DynamicCameraLockMode=0`、`General.CameraMode=2`、`SnapCameraOnRespawn=0` 等，客户端 `/lol-game-settings/v1/game-settings` 里也是同样的值。所以 A 与 B 的差别在**名字里不含 Camera / Lock 的项**上，现有诊断没记到具体是哪一项。
5. 最可能的解释（**推断，P2-1 用 `lcu_matches_file` 证实**）：开局时写回 A 的，是游戏启动阶段按客户端保存的那份设置（客户端会把设置同步到服务器，开局时下发给游戏）重新生成的配置。结算后游戏写出的 B 没有被同步回客户端保存的那份，所以下一局又用旧的 A。可以确定的是：软件本身不写这两个文件，也不调用任何设置写入接口。

### P2-1　诊断补全：找出 A 与 B 到底差在哪一项

`game_settings_changed` 增加 `changed_keys`：**所有**变化的设置项（不只 Camera / Lock），记 `文件.节.名称: 旧值 → 新值`（值截到 32 字符）。`Input.ini` 只记按键项名称，不记键位值。最多 60 项，超出记 `changed_keys_truncated`。

`game_settings_watch` 增加：
- `lcu_settings_hash8`：`GET /lol-game-settings/v1/game-settings` 完整返回体的哈希；
- `lcu_matches_file`：`A` / `B` / `neither`。按"客户端保存的那份设置的所有项"和当前文件对应项逐一比较，说明客户端那份对应 A 还是 B；
- 同样只读 `GET /lol-game-settings/v1/input-settings`，记 `lcu_input_hash8`。

### P2-2　修复：结算后把游戏写出的设置同步回客户端（"保留对局内设置改动"）

开关放在"安装与连接"卡片里现有的设置文件锁定区，名称"保留对局内设置改动"，**默认开启**；不加说明文字。

开启时，每局结算后（`game_end` 快照后 5 秒，文件已稳定）：
1. 读取 `PersistedSettings.json` 里 `Game.cfg` 部分的设置（B），与开局时的快照（A）比较，得到**本局在游戏里被改过的项**。
2. 读取客户端 `GET /lol-game-settings/v1/game-settings`。只有当这些改动项在客户端那份里仍是旧值时，才用 `PATCH /lol-game-settings/v1/game-settings` 写入这些项的新值（**只写改动项，不整份覆盖**）。
3. 写入后再 `GET` 一次核对；执行时核对客户端是否有把设置保存到服务器的接口（例如 `POST /lol-game-settings/v1/save`，先用只读方式确认它存在），有就调用，并在账本写明实际使用的接口。
4. 记录 `game_settings_sync`：`changed_count`、`patched_keys`（只记名称）、`result`（`ok` / `verify_failed` / `lcu_unavailable` / `skipped_no_change` / `skipped_disabled`）。
5. 任何一步失败都不重试写入、不改本地文件，只记日志。
6. 文件被设为只读时（用户用锁定功能），不做同步（`skipped_locked`）。
7. `Input.ini`（按键）不在本次同步范围内。

如果 P2-1 证明 A、B 的差别根本不在 `Game.cfg` 里，P2-2 会记 `skipped_no_change`，不会误写。

### 测试

Go：
1. 构造 A、B 两份 `PersistedSettings.json`，差别在 `Game.cfg.General.SomeSetting` 和 `Game.cfg.HUD.Other`：`changed_keys` 正好是这两项，带旧值 → 新值；`Input.ini` 的变化只出现名称。
2. LCU 那份与 A 相同 → `lcu_matches_file=A`；与 B 相同 → `B`。
3. 同步：LCU 那份是旧值 → 只 PATCH 两个改动项，PATCH 请求体里没有其他项；核对 GET 返回新值 → `result=ok`。
4. LCU 那份已经是新值 → 不发 PATCH，`skipped_no_change`。
5. 开关关闭 → 不读、不写，`skipped_disabled`；文件只读 → `skipped_locked`。
6. PATCH 后核对不一致 → `verify_failed`，不重试。
7. 全程不写 `PersistedSettings.json`、`game.cfg`、`input.ini`（测试里比对文件哈希）。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| PATCH 整份设置而不是只写改动项 | 测试 3 FAIL |
| LCU 已是新值仍然写入 | 测试 4 FAIL |
| 开关关闭仍同步 | 测试 5 FAIL |

---

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R186；R184 那行备注"诊断结论与修复见 R186"；账本 `docs/history/ledgers/r186-execution-ledger.md`。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 真机验收（用户）

升级：
1. 下次出现更新提示时，点"立即升级"，看到下载进度；完成后点"立即重启升级"，软件自动关闭、安装、重新打开，版本号变成新的。
2. 升级后原来的设置、选人配置、LP 记录都还在。之后用桌面上的新快捷方式打开。

游戏设置：
3. 确认"保留对局内设置改动"是开着的。打一局，在游戏里把视角改成你要的；结束后不关软件，再打一局，看开局视角是否保持。
4. 两局结束后导出日志发我：我看 `changed_keys` 确认被还原的是哪一项，看 `game_settings_sync` 确认同步是否成功。
