# R184 执行账本

日期：2026-10-02。工单：WORKLIST-R184-CAMERA-LOCK-RESET-EVERY-GAME-SETTINGS-WATCH-DIAGNOSTICS.md。基线 R183 / 0.12.48，本轮版本 **0.12.49**；package.json 与 package-lock.json 两处版本同步。源码指纹 **e55eeb092260**。

## 范围与结论

本轮只增加诊断。没有写入视角、游戏设置或按键，没有改变既有锁定/解除锁定的 chmod、校验和界面逻辑，没有新增界面文字。**镜头每局恢复锁定的问题尚未修复**，需要下一份 Windows 两局日志判断原因，修复另开工单。此前工作区修改均保留。

## P1：阶段快照

- 新增 game_settings_watch：app_start、champselect、game_start、in_game_60s、game_end、lobby_after_10s、lobby_after_60s、lock_action 八个时机。连接会话启动即启用，首次阶段 prime 也进入同一观察入口；GameStart/InProgress 共用首次开局，三个结算阶段共用首次结束。同局重复阶段去重，每次成功的显式锁定操作分别记录。
- 观察入口只排队，文件和 LCU 读取由后台串行 goroutine 执行。60 秒对局计时从 InProgress 开始；结束后的 10/60 秒从首次结束开始，回 Lobby 不取消。断连取消会话，新局使旧局延迟任务失效。锁定操作的临时观察器不能覆盖活跃连接观察器，后续连接可以接管并取消临时观察器。
- 扫描定位目标及 Config、../Game/Config、Game/Config 下的 PersistedSettings.json、game.cfg、input.ini，按路径去重；保留不存在候选的 exists=false。国服 sibling Game 在既有定位器给出的安装允许根目录内，普通安装根目录外的候选拒绝读取。
- 仅记录相对 path_kind、exists、is_located_target、read_only、size、mtime_utc、content_hash8。Windows 用真实 FILE_ATTRIBUTE_READONLY，其他平台用权限位。hash 为完整文件 SHA-256 前八位。
- Persisted JSON（含 files/sections/settings 结构）、game.cfg 和 LCU 只保留 Camera/Lock 名称设置，值截到 32 个字符；input.ini 及 Persisted 中嵌入的 input 设置仅记录 Camera 项名称和 present，不记录键位。
- 只读 GET /lol-game-settings/v1/game-settings；失败记录 unavailable，HTTP 错误保留实际状态，无 HTTP 响应时为 0。成功接口没有提供实际状态码，因此不伪造 200。
- 沿用安全文件和 256 KiB 限长读取；补查允许根目录内所有路径组件，拒绝符号链接。读取后核验文件身份、字节数、mtime、权限及链接状态，变动中的文件不发布 hash/设置值。过滤账号类名称、路径类值，并脱敏当前账号标识及安装目录用户名；不记录绝对路径、账号、非视角值或按键绑定。

## P2：变更与疑似写回受阻

- 同一文件与上次快照比较 hash 和 Camera/Lock 项，记录 changed_since_prev 与 camera_changed_keys 的 before/after。
- hash 改变或软件自己的锁定权限改变产生 game_settings_changed；选人/启动前变化归为 client_before_launch，对局中和结束后归为 game，软件权限变化且内容未变归为 app_lock_action。writer_guess 只表达阶段推测，不能证明实际写入进程。
- 锁定前利用既有安全 stat 捕获实际权限，解决首次快照尚未完成时仍能归因本次 chmod 的情况；不会把内容变化归给软件。
- 保留 game_end 基准，跟踪结束至 60 秒内任何已观察到的 hash 变化。只读且没有观察到变化时产生 game_settings_write_blocked_suspected；中途改变再还原也不会误报。快照只能观察采样时点，不能证明两次采样间从未有瞬时写入。

## P3：锁定结果诊断

- rig_status_read 与 rig_maintenance action=settings-lock 各分支增加 settings_locked 和相对 path_kind。
- 锁定/解除锁定成功后立即排入 lock_action 快照；原 chmod 0444/0644 和 readRigStatus 成功校验保留。

## 测试与独立核验

backend/r184_test.go 覆盖工单 Go 1–9：国服两份文件和唯一定位目标、Camera/Lock 过滤、只读与解除锁定、结束后视角项变化、启动前归因、只读写回疑似受阻、LCU 成功/404、用户名/账号/路径/键位隐私、同局阶段去重。额外覆盖后台延迟实际触发、Lobby 延续、新局取消旧计时、连接所有权、首次锁定归因、变动还原不误报、限长、符号链接及普通安装根目录外拒读。符号链接用例有目标不存在的前置断言，实际 PASS，没有 SKIP。

四路 default 子代理分别定位阶段/文件安全入口并独立核验生命周期和隐私。核验线索由主代理点验；Windows 原 readRigStatus 权限判断依据 Go 的 DOS 只读属性映射保持不变，不因核验猜测改动锁定逻辑。

## 3 项变异

使用 Go -overlay，生产文件不被替换。最终源码的三项均为目标测试断言 FAIL，逐份核对没有编译失败冒充检出。

| 变异 | 检出 |
|---|---|
| 只读定位目标 | TestR184CandidatePrivacyAndSettingsFilter：缺少 Config/PersistedSettings.json。 |
| 记录全部设置项 | TestR184CandidatePrivacyAndSettingsFilter：input 事件含 LockFoo、evtChat 等越界项。 |
| 取消阶段去重 | TestR184StageDedupAndConnectionPrime：champselect 和 game_start 各记录 11 次。 |

源、overlay、清单与日志：/private/tmp/r184-mutants/{located-only,all-settings,no-stage-dedup}。

## 自动门禁

- 最终专项 Go：通过，2.032 秒；/private/tmp/r184-go-target.log。
- 全量前端：**784/784 通过**，16.103 秒；/private/tmp/r184-web-full.log。
- 全量 Go：最终源码通过，211.762 秒；/private/tmp/r184-go-full.log。前一轮通过，212.841 秒；/private/tmp/r184-go-full-first.log。
- go vet ./backend：通过；/private/tmp/r184-vet.log。
- R184 -race：通过，1.907 秒；/private/tmp/r184-race.log。
- 限长与符号链接专项：通过，0.541 秒；/private/tmp/r184-symlink.log。
- git diff --check：代码、版本、索引与账本文档收尾后通过。

## 构建与真机边界

- key mode=**public**；CGO_ENABLED=0，macOS arm64 与 Windows amd64；显式 main.version=0.12.49、main.buildFingerprint=e55eeb092260、main.riotAPIKey=、main.riotAPIKeyCipher=。
- 产物：/private/tmp/Deep-Legends-backend-0.12.49-public 和 /private/tmp/Deep-Legends-backend-0.12.49-public.exe。两平台构建通过，均通过 verifyBuildFingerprint 与 verifyRiotKeyPolicy(..., "public")；最终源码与两份产物指纹一致。保持 -public 文件名，不生成或发布安装包。
- macOS 自检使用空 RIOT_API_KEY 与隔离 /private/tmp/r184-selftest：通过，0.12.49、奖池 554 条、哈希 dee5e21f5234；/private/tmp/r184-selftest.log。
- Windows 真实客户端、文件属性和每局相机行为尚未验收；交叉构建及合成测试不能替代真机日志。

## 用户两局采集

1. 安装新版本后先打开软件，再开始第一局；进游戏把视角改为不锁定，正常结束。
2. 保持软件运行，在大厅等 1 分钟，再开第二局，观察视角是否恢复锁定。
3. 第二局结束后再等 1 分钟，导出日志；若期间点过锁定/解除锁定，附上大概时间。
4. 核对日志版本 0.12.49 / e55eeb092260，以及八阶段快照、候选文件状态、Camera/Lock 差异和变更/疑似写回受阻事件，再决定后续修复。
