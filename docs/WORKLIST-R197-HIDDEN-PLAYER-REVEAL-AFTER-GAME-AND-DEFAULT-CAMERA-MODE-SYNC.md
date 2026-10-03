# WORKLIST-R197：隐藏身份玩家在对局结束后补全身份；默认镜头模式的同步核对

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-03。基线：0.12.60（R196 之后；R196 未执行时以 0.12.59 为准，互不冲突）。

## P1　对面的塞纳：对局页一直是"隐藏玩家"，总览里却有名字

### 现象

用户反馈：最新一局（海克斯大乱斗）对面的塞纳，在「对局」页里没有玩家信息，但在「总览」的最近对局里能看到这个玩家。用户判断：这个玩家在游戏里显示的是英雄名，而不是召唤师名，所以看不到；以后凡是看不到的，都是这个原因。

### 证据

日志 `lol-loot-diagnostics-1003-1503.jsonl`（**仍是 0.12.58**），game **9012766973**，queue **2400**：

| 时间 (UTC) | 事件 |
|---|---|
| 06:51:55 | `lcu_gameflow_session_shape`：team_one 5 人，**team_two 只有 4 人** |
| 06:52:26 | `live_roster_recovery reason=playerlist-incomplete`（Live Client 还没就绪） |
| 06:52:39 | Live Client 10 人，`position_match_source_counts.riotId=9`：**有一人没有 Riot ID** |
| 06:52:39 | `live_roster_recovery reason=anonymous-placeholder appended=1 named_present=4` |
| 06:52:40 | `live_roster_rendered rendered_100=5 rendered_200=5 hidden_identity_rendered=1` |
| 07:02:19 | 对局结束（WaitingForStats → PreEndOfGame），对局页没有任何更新玩家身份的动作 |

同一份日志里更早的 9012742963（同样是 2400）也是一样：对方少一人，补成隐藏玩家。

### 结论

用户的判断是对的：这个玩家在客户端开启了**隐藏身份**。游戏里他显示的是英雄名，Live Client 不给 Riot ID，gameflow 也不给他的记录。这类"看不到"的情况都是这个原因，不是软件漏了人。软件按 R175/R183 补了一张"隐藏玩家"卡片，这一步是正确的。

**对局进行中**确实没有任何正规数据能拿到他的身份，R175 定下的规则（对局中不查战绩、段位、身份，不绕过隐藏）保持不变。

但是**对局结束后**，这局的对局详情里会包含所有参赛者（总览里能看到就是这个原因）。对局页现在结束后停在最后一次快照，不会用这份公开数据补全，所以出现了"总览有、对局页没有"。

### 修改

1. **触发**：gameflow 进入 `EndOfGame`（或 `PreEndOfGame` 之后），且对局页当前快照里有 `hidden === true` 的玩家，并且快照的 `gameId` 等于刚结束的这一局。
2. **数据来源**：用总览展开最近对局时使用的**同一个对局详情接口**（同一函数、同一缓存），按 `gameId` 读取这一局的 10 名参赛者。不新增其他接口，不使用观战类或第三方接口。
3. **匹配**：每个隐藏玩家，用**队伍 + 英雄 ID** 在对局详情里找参赛者。只有同时满足以下条件才补全：
   - 同队同英雄只有一个参赛者；
   - 该参赛者的 PUUID 不在当前名单里；
   - 该参赛者在对局详情里自己有身份（有 PUUID 和名字）。如果对局详情里他也是隐藏的，保持隐藏卡片。
4. **补全后**：这张卡片换成普通玩家卡片，显示召唤师名、段位、近期战绩，走普通玩家现有的加载流程，可以点名字打开总览。不加任何说明文字或标记。
5. **重试**：对局详情在结束后不一定马上能读到。依次在结束后 10 秒、40 秒、120 秒各尝试一次，最多 3 次；读到并完成匹配就停止。进入新的 ChampSelect、对局页切到其他对局或软件断开连接时立即停止。
6. **对局进行中不变**：`InProgress` / `Reconnect` 阶段不做以上任何事。
7. **诊断** `live_roster_post_game_reveal`：`game_id`、`queue_id`、`attempt`、`hidden_count`、`revealed`、`reason`（`ok` / `match-not-ready` / `no-candidate` / `ambiguous` / `participant-hidden` / `stopped`）。不记 PUUID 和名字。

### 测试

Go：

1. 复现本局：快照里对方 4 人 + 1 名隐藏塞纳（team 200，championId 塞纳）；对局详情里对方 5 人，塞纳有 PUUID 和名字 → 补全 1 人，`reason=ok revealed=1`。
2. 对局详情第一次 404/未就绪，第二次成功 → `attempt=2` 成功；三次都失败 → 停止，`reason=match-not-ready`，卡片保持隐藏。
3. 同队同英雄 2 个候选（构造数据）→ 不补，`reason=ambiguous`。
4. 对局详情里该参赛者也没有身份 → 不补，`reason=participant-hidden`。
5. `InProgress` 阶段 → 不读取对局详情（断言请求次数为 0）。
6. 重试期间进入新的 ChampSelect → 停止，`reason=stopped`，不再请求。
7. 快照 `gameId` 与结束的对局不同 → 不处理。

Node：

8. 补全后的返回体渲染：原隐藏卡片变成普通卡片，显示名字和段位，不再出现「隐藏玩家」「客户端未公开该玩家」。
9. 未补全（`participant-hidden`）：卡片与 R193 的隐藏卡片一致。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 不按英雄 ID 匹配、按位置顺序填 | 测试 1 或 3 FAIL |
| 对局中也去读对局详情 | 测试 5 FAIL |
| 同队同英雄多个候选时取第一个 | 测试 3 FAIL |

## P2　进游戏一直是锁定视角

### 证据

同一份日志，0.12.58 共 6 局，每局开局和结束的 `game_settings_watch` 中，所有视角相关项**完全没变过**：

- `Game.cfg.General.CameraMode = 2`
- `Game.cfg.General.CameraModeWASD = 1`
- `Game.cfg.HUD.CameraLockMode = 0`、`DynamicCameraLockMode = 0`
- `SnapCameraOnRespawn = 0`

每局结束都是 `game_settings_sync result=skipped_no_change`，`game_settings_changed` 里唯一变化的是 `SystemMouseSpeed 8 ↔ 0`（游戏退出时自己写的，和视角无关）。

### 结论

1. 软件没有改视角，也没有把设置还原。R186 的同步每局都运行了，只是没有需要同步的改动。
2. 游戏里按 **Y** 解锁视角只对当前这一局有效，**不会写进设置文件**，所以下一局开局还是锁定。
3. 决定"开局时视角锁不锁"的是游戏选项里的**默认镜头模式**（选项：自由 / 锁定）。按日志里的值，用户这一项现在是"锁定"一侧（`General.CameraMode=2`）。只要它不改，每局开局都会锁定。

### 修改（只加核对，不改游戏设置）

1. 同步逻辑不变。补一条 Go 测试：本局开局 `General.CameraMode=2`、结束时为其他值，LCU 那份仍是 2 → 只 PATCH `General.CameraMode` 一项，`result=ok`。确认用户在游戏选项里改了默认镜头模式后，下一局不会被还原。
2. `game_settings_sync` 增加 `camera_mode_before` / `camera_mode_after`（只记这一项的值），方便下一份日志直接确认。
3. 不新增界面文字，不新增写游戏设置的功能。

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 版本递增；`docs/WORKLIST-INDEX.md` 加 R197，R183 / R193 那行备注"对局结束后补全隐藏玩家由 R197 处理"；R186 那行备注"视角问题核对见 R197 P2"。账本 `docs/r197-execution-ledger.md`，附演示截图：补全前后的对方卡片。
- 按 R195 的方式发布到 GitHub，标为 Latest，附 `latest.json`。

## 真机验收（用户）

1. 再遇到"隐藏玩家"时，打完这局不要切页面，停在对局页等 1～2 分钟：那张卡片变成正常玩家（名字、段位、战绩）。对局进行中仍然显示隐藏玩家。
2. 视角：进游戏后按 Esc 打开选项，在**游戏 → 镜头**里把**默认镜头模式**改成**自由**（不是按 Y）。打完这局，下一局开局应该不再锁定。
3. 两局后导出日志：看到 `live_roster_post_game_reveal reason=ok`；`game_settings_sync` 里 `camera_mode_before=2`、`camera_mode_after` 为新值，`result=ok`。
