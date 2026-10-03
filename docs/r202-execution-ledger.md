# R202 执行账本

日期：2026-10-03。基线 0.12.64，本次版本 0.12.65。R201 已发布，本工单单独递增。

## 逐项实现

- P1-1/2：卡片模式只在 FINALIZATION 安排备战席交换；其他阶段记 `bench-gate reason=waiting-finalization`，带备战席 ID、当前英雄、目标。延迟请求写前重新读会话，阶段退回 BAN_PICK 时取消。保留已有备战席停留等待设置，BAN_PICK 观察到的停留时间连续计算。
- P1-3/4：只给 BAN_PICK 发出且 postflight 未生效的请求退款，进入 FINALIZATION 恢复每目标两次预算；成功交换、目标消失和 FINALIZATION 的失败仍按原规则处理。使用写前实际读到的阶段，避免延迟跨阶段的请求错算。普通大乱斗保持既有发送时机，加入同样退款。
- P2：卡片首次手动/软件选择不触发接管；卡片模式的备战席观察只有已经持有序列英雄后再主动换走才停止本轮自动操作。重随经过英雄 ID 归零后保留上一位持有英雄的证据，再出现其他英雄时接管；自身交换的回读不接管。R201 的临时首卡开关继续保留，不增加设置。
- P3：关闭优先开关且当前英雄已在序列时返回无目标；开启时只取更靠前的英雄。当前不在序列时，无论开关都取备战席中序列最靠前者。
- P4：`camera_mode_matches_target` 仅比较安全定位且成功读取的 PersistedSettings.json 内 `Game.cfg.General.CameraMode` 与目标数值，缺失/未知不填造 true。LCU 缺少 CameraMode 或 game.cfg 不同不会掩盖该值。
- P5-1/2：显示当前队列近 30 天内最新 10 局；保留现有精确队列口径（如单双排只算 420）。复用 `loadSGPMatchHistoryPage` 与已验证队列映射，只使用当前队列的单一 tag（如 q_2400），首次请求 10 局。tag 能力按具体队列记忆，不污染同组其他队列。
- P5-3：筛选不可用时直接退回每页 30 局，按需读取；凑够 10、页内到达 30 天外、耗尽、四页上限即停止。fallback 请求使用实际 consumed 推进，不重复读取第一页 10 局；最多读取四个混合数据页（120 局），tag 探测不算混合数据页。
- P5-4/5：LCU 保持原始 30 局读取，与 fresh SGP 按正数 gameId 合并去重、按完整度选优；最终展示、统计和 freshness 的 `prev_game_shown` 共用 30 天/10 局筛选。本人 current-summoner 端点、R180/R181 最新局与六次诊断上限保留。
- P5-6/7：普通缓存和已有同局缓存均含队列，45 秒 TTL 不变；fresh SUMMARY 不读写共享五分钟 SGP 缓存。六玩家并发、单玩家 SGP 三秒/整体六秒超时不变，分页共用原上下文。
- P5-8：`live_history_freshness` 增加 `queue_filtered`、`pages_read`、`window_days=30`、`shown_count`、`stop_reason`（enough/window/page_limit/exhausted），基于实际读取和最终合并后的展示结果，不新增 UI 说明文字。

## 日志证据

`lol-loot-diagnostics-1003-2044.jsonl` 的 57 交换链显示 BAN_PICK 发出后未生效，进入 FINALIZATION 已耗尽旧预算；见 [脱敏记录](history/reports/r202/source-log-summary.json)。镜头日志中部分 located PersistedSettings 的值仍为 2、game.cfg 为 0，旧 false 对这些快照并非错误；源码另有 LCU 无 CameraMode 导致 false 的真实问题，本次以 Persisted 单一比较来源修正，并用“LCU 字段缺失、Persisted=0、game.cfg=2”测试锁定。

## 验证

相关 Go 回归通过（7.924 秒）：R202、R200/R201、普通大乱斗/手动接管、镜头、R178/R180/R181、赛后补全。旧夹具增加实际近期开始时间、遵守队列筛选和新缓存键；原去重、最新局与读取次数护栏保留。Node 全量 1127 项，1126 通过、1 跳过、0 失败（270.802 秒）。R202/R180/R181 race 通过（2.503 秒），go vet 通过。三项 overlay 变异均为断言 FAIL：BAN_PICK 发交换、失败计入最终预算、关闭开关仍横向换人。最终 `go test ./backend -count=1` 全量通过（248.905 秒）；完整 public 构建进行中。

## 构建与发布

key mode：public。版本 0.12.65；构建/发布未完成。正式发布后按 R199 补 release id、isLatest=true 和匿名 Latest 清单版本，以及附件 digest/源码指纹。

## Windows 真机验收

仍需用户海斗一局确认 waiting-finalization 后实际换入并 postflight applied、开关两种持有规则、当前模式近 30 天 10 局，以及 in_game_60s 镜头匹配。模拟 LCU/SGP 与交叉构建不能替代实际客户端验收。
