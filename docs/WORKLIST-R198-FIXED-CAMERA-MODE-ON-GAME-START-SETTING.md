# WORKLIST-R198：维护里加「进入游戏时的镜头模式」设置，每局开局固定成用户选的镜头

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-03。基线：R197 之后（R196/R197 未执行时以 0.12.59 为准，互不冲突）。

## 用户需求

游戏选项「镜头 → 镜头模式」有三种：**自由镜头 / 动态镜头 / 锁定镜头**（用户截图，当前选中锁定镜头）。用户每局进游戏都是锁定镜头，要手动改。希望在软件「工具 → 维护」里加一个设置，选好之后，每局进游戏都自动是这个镜头模式，不用再手动改。

本工单取代 R197 P2 的"只核对、不改设置"。R197 P2 里的测试和 `camera_mode_before/after` 诊断照做，本工单在其基础上加功能。

## 已知事实（日志 `lol-loot-diagnostics-1003-1503.jsonl` + R186 结论）

1. 镜头模式存在 `Game.cfg.General.CameraMode`（`PersistedSettings.json` 和 `game.cfg` 两处都有）。用户当前为锁定镜头，值是 **`2`**；6 局里这个值从未变过。
2. 选项顺序是 自由 / 动态 / 锁定，锁定 = 2，按顺序推断 **自由 = 0、动态 = 1**。这是推断，执行时必须核实（见"取值核实"）。
3. `Game.cfg.General.CameraModeWASD`（当前 1）是 WASD 操作方式下的镜头设置，本工单**不动**。
4. R186 已证实：每局进游戏约 10 秒，两个设置文件会被改写成客户端保存的那份设置（`/lol-game-settings/v1/game-settings`）。所以只改本地文件不可靠，**主要改客户端保存的那份**。
5. 软件已有：`game_settings_sync.go` 用 `PATCH /lol-game-settings/v1/game-settings` 只写改动项 + GET 核对 + `POST /lol-game-settings/v1/save`；`game_settings_watch.go` 在各阶段读文件快照。

## P1　设置项

位置：`工具 → 维护 → 安装与连接` 卡片，「保留对局内设置改动」开关下面一行。

- 名称：**`进入游戏时的镜头模式`**
- 下拉选项：**`不修改`**（默认）、**`自由镜头`**、**`动态镜头`**、**`锁定镜头`**
- 选择后立即保存到软件设置（和其他维护设置同一处存储），不加说明文字。
- 设置文件被设为只读（「设为只读」）时，下拉照常可选；应用时按 P2-4 处理。

## P2　应用逻辑

选项不是"不修改"时：

1. **时机**：进入 **ChampSelect** 时应用一次；进入 **GameStart** 时再核对一次（防止选人期间被别的东西改回去）。另外，在大厅里用户改了这个下拉，也立即应用一次。`InProgress` / `Reconnect` 阶段不做任何写入。
2. **写客户端保存的那份设置**：`GET /lol-game-settings/v1/game-settings`，读 `General.CameraMode`。和目标值不同时，`PATCH` **只含 `General.CameraMode` 这一项**；再 GET 核对；有 `/lol-game-settings/v1/save` 就调用（沿用 `game_settings_sync.go` 的现有写法和接口探测）。相同则不写。
3. **同时核对本地文件**：读 `PersistedSettings.json` 和 `game.cfg` 的 `General.CameraMode`。和目标值不同、且文件不是只读时，**只改这一项的值**，其他内容按原样保留（原文件的格式、顺序、其他设置不变，写前先写临时文件再替换；沿用现有的路径校验，不跟随符号链接，必须在安装目录内）。这一步只在 ChampSelect / GameStart 做，游戏进程运行中不写文件。
4. 文件是只读：不改文件，只做第 2 步，记 `file_skipped=read_only`。
5. **和 R186「保留对局内设置改动」的关系**：如果用户在对局里手动改了镜头模式，结算后 R186 会把它同步回客户端；但这个下拉不是"不修改"时，下一局 ChampSelect 仍然以下拉为准改回去。也就是说，下拉的优先级高于对局内的改动。不加说明文字。
6. 任何一步失败都不重试、不报错弹窗，只记日志，下一局再试。
7. 选"不修改"：完全不读写，行为与现在一致。

## P3　取值核实（执行时必须做）

0 / 1 / 2 的对应关系是推断，执行时按下面的办法核实，并写进账本：

1. 查本地已有资料（`/lol-game-settings/v1/game-settings` 的返回、LCU schema、`PersistedSettings.json` 里的字段说明（如果有））。
2. 如果找不到权威对应关系，代码里把映射集中在一个常量表里（`free=0, dynamic=1, locked=2`），账本写明"按选项顺序推断，待用户真机核实"，并在 P4 诊断里记录开局后的实际值，用户第一局后就能确认。
3. 用户核实后如果映射不对，只需改这张常量表。

## P4　诊断 `game_camera_mode_apply`

每次应用记一条：`stage`（`champselect` / `game_start` / `settings_changed`）、`target`（`free` / `dynamic` / `locked`）、`target_value`、`lcu_before`、`lcu_after`、`lcu_result`（`ok` / `unchanged` / `verify_failed` / `unavailable`）、`file_before`、`file_after`、`file_result`（`ok` / `unchanged` / `read_only` / `not_found` / `write_failed`）、`save_called`（布尔）。

另外，R184 的 `in_game_60s` 快照里本来就有 `General.CameraMode`，再加一个字段 `camera_mode_matches_target`（选项不是"不修改"时才有），直接说明开局后镜头模式有没有生效。

不记录路径、账号。

## 测试

Go：

1. 目标自由（0），LCU 为 2 → PATCH 请求体只有 `General.CameraMode=0`；核对 GET 为 0 → `lcu_result=ok`；有 save 接口时被调用。
2. LCU 已是 0 → 不发 PATCH，`lcu_result=unchanged`。
3. 本地 `PersistedSettings.json` 和 `game.cfg` 为 2 → 两个文件都只改这一项；改前改后对比，除 `CameraMode` 外**字节级不变**（JSON 格式、缩进、顺序、其他值）。
4. 文件只读 → 不改，`file_result=read_only`，LCU 照常写。
5. `InProgress` / `Reconnect` 阶段触发 → 不发任何写请求、不写文件。
6. 选"不修改" → 不读不写。
7. `CameraModeWASD` 在任何情况下都不被改。
8. 路径校验：符号链接或安装目录外的路径 → 拒绝，`file_result=write_failed`。
9. R197 P2 的同步测试仍然通过；R186 结算同步把镜头改成锁定后，下一局 ChampSelect 按下拉改回自由。

Node：

10. 维护页出现「进入游戏时的镜头模式」，四个选项，默认"不修改"；切换后发保存请求，刷新页面后保持选中项。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| PATCH 整份设置 | 测试 1 FAIL |
| 改文件时重写整个 JSON（格式变化） | 测试 3 FAIL |
| 对局中也写 | 测试 5 FAIL |
| 顺手改 `CameraModeWASD` | 测试 7 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R198，R184 / R186 那行备注"开局镜头模式由 R198 固定"，R197 那行备注"P2 由 R198 取代并扩展"。账本 `docs/r198-execution-ledger.md`，附维护页截图（演示数据）。
- **R197 和 R198 合并成一个版本 0.12.61 一起发布**（R196 的 0.12.60 未发布，见 R199，不单独发）。R197 收尾里的"版本递增、发布"以这里为准，不分两次发。
- 按 R195 的方式发布 0.12.61 到 GitHub，标为 Latest，附 `latest.json`；账本必须有 release id、`isLatest=true` 查询结果、匿名下载 `releases/latest/download/latest.json` 得到 `0.12.61`（R199 规则），缺一项只能写"发布未完成"。
- 发布说明里写上 0.12.60 的改动（R196：下载测速选线、剩余时间、后台下载与完成提示、顶部按钮），因为 0.12.60 没有单独发布。

## 真机验收（用户）

1. 装新版本后，打开「工具 → 维护」，把「进入游戏时的镜头模式」选成「自由镜头」。
2. 开一局，进游戏不做任何操作，看是不是自由镜头；在游戏选项「镜头 → 镜头模式」里看到的也应是「自由镜头」。
3. 再打一局，仍然是自由镜头。
4. 导出日志：`game_camera_mode_apply` 里 `lcu_result=ok`（第一局）或 `unchanged`（之后），`in_game_60s` 快照里 `camera_mode_matches_target=true`。如果第 2 步看到的不是自由镜头，把日志发我，说明 0/1/2 的对应关系需要调整。
