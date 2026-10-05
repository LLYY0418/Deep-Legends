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

`lol-loot-diagnostics-1003-2044.jsonl` 的 57 交换链显示 BAN_PICK 发出后未生效，进入 FINALIZATION 已耗尽旧预算；见 [脱敏记录](../reports/r202/source-log-summary.json)。镜头日志中部分 located PersistedSettings 的值仍为 2、game.cfg 为 0，旧 false 对这些快照并非错误；源码另有 LCU 无 CameraMode 导致 false 的真实问题，本次以 Persisted 单一比较来源修正，并用“LCU 字段缺失、Persisted=0、game.cfg=2”测试锁定。

## 验证

相关 Go 回归通过（7.924 秒）：R202、R200/R201、普通大乱斗/手动接管、镜头、R178/R180/R181、赛后补全。旧夹具增加实际近期开始时间、遵守队列筛选和新缓存键；原去重、最新局与读取次数护栏保留。Node 全量 1127 项，1126 通过、1 跳过、0 失败（270.802 秒）。R202/R180/R181 race 通过（2.503 秒），go vet 通过。三项 overlay 变异均为断言 FAIL：BAN_PICK 发交换、失败计入最终预算、关闭开关仍横向换人。最终 `go test ./backend -count=1` 全量通过（248.905 秒）；完整 public 构建通过，含全部 Go 分片、backend/installer vet、installer 测试及打包校验。

## 构建与发布

key mode：public。本地完整构建通过，版本 0.12.65、指纹 `7a0b897f0881`，源提交/标签 `be6294969a38ccab6c0ff1c6bf9727fadf501991`。本地 Setup SHA-256：`a41720bb92898694bc23ee2a9520fdc21ed245442bc0b5ab9e785d278d3ed3ea`，见 [本地构建记录](../reports/r202/local-release-build.json)。

已正式发布 [v0.12.65](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.65)。Release id **402540854**，发布时间 **2026-10-03T14:14:13Z**（北京时间 22:14:13），`draft=false`、`prerelease=false`、`isLatest=true`；匿名下载 `releases/latest/download/latest.json` 返回 **0.12.65**，字节与已经下载核验的正式清单相同。见 [发布核验](../reports/r202/publication-verification.json)。

Windows public 工作流 [37127823415](https://github.com/LLYY0418/Deep-Legends/actions/runs/37127823415) 已成功，source SHA 与标签一致。标签质量工作流 [37127823435](https://github.com/LLYY0418/Deep-Legends/actions/runs/37127823435) 的 Linux quality 已成功（全量 race、Node、真实 Chromium、installer）；其后独立的 Windows 重复构建在收尾取证时仍运行中。正式 Windows 构建已完成全部必需检查，独立重复任务不记为已通过，具体快照见 [质量状态](../reports/r202/github-quality-run.json)。

正式发布恰有三个 public 附件：`Deep-Legends-Setup-0.12.65-public.exe`、`latest.json`、`SHA256SUMS-public.txt`。逐个下载核对大小和 GitHub digest，再核对清单版本/指纹/URL及 SHA256SUMS；全部吻合。正式 Windows Setup SHA-256：`bf97221a0c8465b78d57bf32f15c746eeec72deb7f3ec4923323b095c16c70ac`，大小 111408128 字节。不同构建主机的 Setup SHA 不同，两份构建源码指纹均为 `7a0b897f0881`。见 [附件核验](../reports/r202/release-asset-verification.json)。

本次仅新增该版本发布；之前九个 Release 的身份、正文、目标、发布时间及附件 id/名称/大小/digest 均未修改，0.12.60 草稿 **402378684** 保持原样。

## Windows 真机验收

仍需用户海斗一局确认 waiting-finalization 后实际换入并 postflight applied、开关两种持有规则、当前模式近 30 天 10 局，以及 in_game_60s 镜头匹配。模拟 LCU/SGP 与交叉构建不能替代实际客户端验收。

## R204 追加真机核对（2026-10-04）

R204 工单真机摘录确认正式 2400 FINALIZATION 瑞兹 13 换入成功，以及用户将 22 换成 112 后 gate-blocked、不再换回，以上范围真机验证通过。queue_filtered=true/pages_read=1/stop_reason=enough 的 10 局读取也有摘录。临时测试选人由 R204 P9 删除。原 P6（Riot Key）因复制的是旧稿未执行，移到 R204。开关两种持有规则与完整近期战绩逐项真机验收仍缺记录，R202 保留进行中；不扩大证据范围。

## R206 补充的 10-04 用户日志核验

海斗卡片选人及备战席应用在已提供范围内通过；无序列英雄时不提交选人。证据来自 `lol-loot-diagnostics-1004-1451.jsonl`，摘要见 [R206 日志证据](../reports/r206/user-log-evidence.json)。

## R212 收口（2026-10-04）

已关闭：用户确认（R212）。

依 R212 P1 用户确认关闭；不以自动测试替代历史真机验收事实。

来源：[R212 工单](../../WORKLIST-R212-CLOSE-VERIFIED-WORKLISTS-UPGRADE-INSTALL-GAP-AND-RELAY-FAILURE-LABELS.md)；[匿名日志证据](../reports/r212/user-log-evidence.json)（原日志行号及 SHA256）。以上为本次收口结论，早先的“待验收”记录保留其当时语境。
