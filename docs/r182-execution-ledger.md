# R182 执行账本

日期：2026-10-01 至 2026-10-02。工单：WORKLIST-R182-LOSS-LP-DELTA-NEVER-RECORDED-GAMES-GAP-TWO.md。基线 R181 / 0.12.46，本轮版本 **0.12.47**；package.json 与 package-lock.json 两处同步。源码指纹 **a2e718b631f9**。

## 范围与证据

- 以工单提供的四份真机日志摘要、11 次捕获与 3 次 games_gap=2 跳过为依据。本轮没有原始 jsonl 文件，也没有 Windows 真机排位。
- 修复结算仅认场次 +1 的限制，隔离 observe 的赛季统计补全值；不据摘要断言上游多计负场的确切原因。新增差值与基线诊断供真机确认。
- 历史上已经被跳过的负局**无法补记**，因为当时没有可用的开局基线。
- 前端 renderMatch 已支持负值，本轮不改界面、不增加说明文字。R181 与此前工作区修改均保留。

## P1：差值诊断

- lp_capture_recorded / lp_capture_skipped 增加 wins_delta、losses_delta、score_delta、baseline_source、baseline_age_s、snapshot_source；未知差值为 null。跨来源 score_delta 为 null，不把跨源分差当作 LP。
- lp_baseline_written 记录 writer、season_fallback，以及相对上一队列基线的三个差值。按账号和队列、值及跳过原因在 60 秒内去重；正常写入、game_start 与 season_fallback 跳过均有记录。新增开局轮询和失败原因，保持无身份诊断。
- 首个不同快照之后每隔 5 秒记录 lp_capture_settle。兼顾 P1 的两次补采样和 P2 的稳定性判断：至少取两次后续同源可信快照；分数仍变化时取第三次，连续两次绝对分一致才记录。第三次仍不稳定记 score_unstable；等待阶段最多 18 次，临近上限首次变化时允许完成这三次采样。
- 无开局基线时，后续两次采样只用于诊断和刷新队列基线，仍以首个满足原场次条件的快照决定记录/跳过；补采样失败不改变原决定。
- 允许字段测试保留原禁止绝对值断言，构造 CHALLENGER、98765 LP、123456 胜与 654321 负，确认所有 lp_* 事件不含这些值、账号、队列或 gameId。基线时效覆盖 120 秒。

## P2：独立开局基线与结算

- 实际连接管理器的 gameflow 事件调用统一 handleLPGameflowPhase，GameStart / InProgress 和结算三种阶段共用同一个 loadRanksWithFallback 回调。
- 首次开局读取 session，仅队列 420 / 440、正数 gameId 生效。排位读取最多三次、间隔 5 秒；可信且可计算绝对分、来源明确才写入。重复阶段按账号与 gameId 去重；捕获等待已经在途的开局读取结束。
- gameStarts 保存 gameId / queueType / snapshot / source / takenAt，独立于队列 baselines。observe 可更新队列级基线，不能替换开局快照；结算按匹配 gameId 优先使用开局快照。
- schemaVersion 从 1 升到 2，兼容原 schema 1 的脱敏键与已有 games；新字段缺失正常读取，往返保持一致。原含明文 PUUID 的旧格式仍按既有隐私规则清空，不迁移稳定标识。
- 有开局快照时，任意胜负场或绝对分变化即进入稳定性采样；games_gap≥2 可以记录，胜局 W+1/L−1、总场次不变也可记录。分差始终为同源稳定绝对分减开局绝对分。
- 来源不同继续等待；整轮没有同源可信快照时 source_mismatch 跳过。曾短暂跨源但同源数据一直未变时仍记 timeout。全部 18 次不变时不记录 LP。
- 无开局快照继续使用原恰好 +1 记录、多于 1 跳过规则；读取失败不会制造开局基线。软件启动时的状态预读没有增加开局拍照调用。
- applySeasonRankWinRateFallback 设置内部 seasonFallback 标记，经总览传入 observe；补全队列记 season_fallback 跳过，原样可信队列照常写入。标记不进入 JSON/UI。
- 捕获结束后以最后可信上游快照刷新队列基线，删除匹配的本局开局快照并持久化；跨源/超时也执行清理。

## 测试覆盖

工单 11 项均有 Go 覆盖：负局 gap=2、预计负场后总场次不变胜局、先掉分后改场次/多次改分稳定、掉分保护、短暂与全程跨源、原无开局行为、observe 隔离、赛季补全跳过与去重、schema1 兼容和 schema2 往返、隐私、实际 WebSocket InProgress→WaitingForStats 链路。

另覆盖：开局三次重试与阶段去重；第三次后分数仍变化不记录；无开局补采样改变数据或失败时原决定保持；跨源回退后同源未变记超时。

两路 default 子代理独立核验捕获时序、schema 与隐私。核验发现一次差值 helper 中误插入的重复捕获代码，已删除，并重新通过专项回归。未扩大 timeout 事件的字段要求。

## 5 项变异

Go -overlay 读取临时变异源，不覆盖生产文件。五项都断言 FAIL，已逐份检查目标测试名；没有编译错误冒充检出。

| 变异 | 检出 |
|---|---|
| 开局快照仍按 gap≥2 跳过 | TestR182LossGapTwoUsesGameStartScore：记录为空、出现 games_jumped。 |
| 等待改回场次必须增加 | TestR182ProjectedLossWinWithUnchangedGames：18 次后超时、没有 +20。 |
| observe 覆盖开局基线 | TestR182ObserveCannotReplaceGameStart：开局快照被改成总览值。 |
| observe 写赛季补全值 | TestR182SeasonFallbackSkippedAndDiagnosticsDedup：队列基线变成赛季胜负数。 |
| 忽略 InProgress | TestR182GameflowEventsStartThenSettlement：真实 WebSocket 阶段未写开局基线。 |

源、overlay、清单与日志：/private/tmp/r182-mutants/{start-gap-guard,wait-games-increase,observe-overwrites-start,season-fallback-writes,ignore-inprogress}。

## 自动门禁

- 专项 Go（R182 与原 LP 测试）：通过，1.359 秒；/private/tmp/r182-go-target.log。
- 全量 Node：**783/783 通过**，14.825 秒；/private/tmp/r182-web-full.log。
- 全量 Go：通过，195.764s；/private/tmp/r182-go-full.log。
- go vet ./backend：通过；/private/tmp/r182-vet.log。
- 实际 WebSocket 开局与结算的 -race：通过，2.258 秒；/private/tmp/r182-race.log。
- git diff --check：代码、版本、索引与账本文档收尾后均通过。

## 构建与真机边界

- key mode=**public**；CGO_ENABLED=0，macOS arm64 与 Windows amd64。显式 main.version=0.12.47、main.buildFingerprint=a2e718b631f9、main.riotAPIKey=、main.riotAPIKeyCipher=。
- 构建通过：/private/tmp/Deep-Legends-backend-0.12.47-public 与 /private/tmp/Deep-Legends-backend-0.12.47-public.exe。两份均通过 verifyBuildFingerprint 与 verifyRiotKeyPolicy(..., "public")；源码再次计算指纹与两份产物一致。保持 -public 文件名，不生成或发布安装包。
- macOS 自检使用空 RIOT_API_KEY 与隔离 /private/tmp/r182-selftest：通过，0.12.47、奖池 554 条、哈希 dee5e21f5234；/private/tmp/r182-selftest.log。

Windows 真机验收仍由用户执行：

1. 软件开局前保持运行，排位负局后总览显示负 LP；胜局照常显示。
2. 软件仍在运行时导出日志，确认 0.12.47 / a2e718b631f9，以及负局 lp_capture_recorded 的 baseline_source=game_start、wins_delta / losses_delta / score_delta。
3. 对局中才启动软件时没有可用开局基线，继续原保守规则，不能保证显示该局变化。
