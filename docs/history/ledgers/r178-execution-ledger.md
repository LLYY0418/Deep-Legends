# R178 执行账本

后续更正（R180）：P2 的“两人抽样 + SGP 页缓存 + 全模式最新时间”不足以证明名单没有遗漏当前队列最新一局；R179 引用的“不陈旧”结论已撤销。P2 的 preferSGP 后续切源实现由 R180 的实时两来源合并替代。原执行过程保留，下方记录不作为当前实现或新鲜度验收结论；详见 [R180 账本](r180-execution-ledger.md)。

日期：2026-10-01。工单：WORKLIST-R178-INGAME-DUPLICATE-POSITIONS-GAMEFLOW-ROLE-PARSE-AND-ROSTER-HISTORY-FRESHNESS.md。基线 0.12.42，本轮版本 0.12.43；package.json 与 package-lock.json 两处版本已同步。源码指纹：d5961921e9b6。

## 证据边界

**R177 尚未真机验证（用户上传的日志与上一轮相同，不含 0.12.42 会话）。** 工单说明同名日志逐字节相同，共 15059 行，最后时间 08:56:21Z，最后构建为 0.12.41 / c06daae1113a。该摘要作为工单提供的证据；本轮未把自动回归当成 .42 或 .43 的真机验证。

P1 复现使用 game 9009400838、queue 440 的复合角色串。P2 截图缺少对应日志，不能确定是队列筛选还是 LCU 陈旧；因此增加诊断，只有抽样证明 LCU 缺更新战绩时才改变该玩家的后续数据来源。

## P1：位置解析、补探测与来源诊断

- 新增 normalizeGameflowPosition：selectedPosition 整词优先，大小写及空白容错；无法识别时只看 selectedRole 的第一个点分段，忽略 PRIMARY/FILL 及其余志愿。gameflow 的推荐预热 seed 与名单构造两处使用新函数。
- NONE/UNSELECTED/FILL 不推断分路，helper 返回空。显式 OTHER 保留；ARAM/海斗/斗魂等非分路模式中，原 generic 解析得到 other 的场景在 seed 和名单保留 other。NONE 表格的语义冲突按“helper 空、非分路对外保留 other”处理，并有 KIWI 实际加载回归。
- normalizePosition 未改，比赛时间线 BOTTOM + DUO_SUPPORT 仍为 utility。
- 最终位置优先级仍为 Live Client > 选人快照 > gameflow；从 Live Client 恢复的漏人位置也标记为 liveclient。positionSource 是非导出内部字段，不新增 UI 响应字段。
- 经典模式 InProgress/Reconnect 尚未拿到全部已有玩家的 Live Client 标准分路时，响应带 live-client-positions: pending。完整 playerlist 但缺位置也清除成功探测标志，允许重新探测。
- 带 pending 的名单短缓存 3 秒，避免旧完整名单整局缓存阻止 10 秒补探测；位置齐全后恢复原整局缓存。Live Client 原 3 秒探测间隔保留。
- 前端 pending 使用独立 gameID/generation 预算，10 秒一次、最多 6 次；InProgress/Reconnect 转换不重置次数。回调核对位置状态、游戏 ID、generation、阶段、连接、设置与销毁状态。位置恢复后回到 R90 静默刷新；没有 pending 的原刷新规则保留。
- live_position_shape 新增 position_source_counts（liveclient/snapshot/gameflow/none）和 team_duplicate_positions。后者统计每队每个标准位置超出第一人的人数之和，忽略 other/空值。只有队号和计数，无身份。
- 没有新增说明文字、tooltip 或布局改动。

## P2：战绩新鲜度与证据驱动切源

- livePlayerMatchesResult、flight 与 45 秒缓存增加内部 Source，记录实际返回窗口来自 lcu/sgp。原 LCU 30 场混合窗口、同当前队列最多展示 10 场、45 秒 TTL 均保留。
- freshness scope 按 LCU client / gameID / queueID 隔离；离开对局后清理。并发锁保护每玩家一次诊断、最多 2 名非本人抽样和已证实陈旧的玩家集合。
- live_history_freshness 只记录 team、is_current、source、window_games、newest_any_age_min、newest_queue_age_min、queue_games。年龄取窗口最大有效 CreatedAt，毫秒换算分钟、保留两位；没有有效时间时 null，不造值。无身份及 matchID，也不写本局 gameID。
- 仅对实际 LCU 窗口且 SGP 可用的非本人玩家额外抽样，预算在发请求前预占，多次刷新或并发不重复。复用现有 SGP SUMMARY30、原 page cache，3 秒抽样 context，无详情读取。失败只附加 sample_failed，返回原始名单数据。
- 成功抽样增加 sgp_newest_any_age_min、lcu_missing_newer；后者去重计数 SGP 中比 LCU 窗口最新时间更新、LCU 中没有的游戏。空或未标时间的 LCU 窗口不当作陈旧证据。
- 缺更新场数 > 0 时只设置该局该非本人玩家的后续 SGP 优先标记；本次结果仍是 LCU。后续使用独立同局缓存键与原 45 秒缓存，SGP 不可用、失败或空结果回退既有 LCU 路径。本人继续 current-summoner，不参与额外抽样。原 SGP 自身 page cache 不改。
- 未改变当前队列筛选规则，也没有将近期详情列表扩到 30 条。

## 自动验证

- Go R178 定向：通过，2.209 秒，日志 /private/tmp/r178-go-target.log。覆盖日志中 7 种原始 role、UTILITY.FILL_PRIMARY、精确值优先及未知 token；真实缓存/LCU/playerlist/名单链路，己方快照 5 人、敌方 gameflow 5 人，各队五个不同位置、重复数 0；首次不可用、十人列表缺位置、后来位置齐全与齐全后静默缓存；KIWI other 与时间线 support；LCU 缺 SGP 两场、首屏不变、后续切源、自己路径、失败及空列表回退、同局刷新和并发 2 人上限、新局重置、混合队列年龄计数与诊断隐私。
- Node R178：2 项，包含 10 秒六次、Reconnect 不重置、ready/gameID/phase/generation 取消、没有 pending 静默。旧 R90/R91/R94 定向 33 项通过。
- node --test backend/web/*.test.cjs：766/766 通过，10.891 秒，日志 /private/tmp/r178-web-full-final.log。
- 最终 go test ./backend -count=1：通过，202.870 秒，日志 /private/tmp/r178-go-full-final.log。
- 最终 go vet ./backend：通过，日志 /private/tmp/r178-vet-final.log。
- git diff --check：通过；构建详情见下方收尾记录。

首次全量前端有 7 条 ReferenceError，来自旧 r113/r161/r163/r91-addendum 聚焦 harness 未载入新增 liveClientPositionsPending；已纳入真实 helper，保留原断言后全量重跑通过。此前定位/编译夹具错误未算作变异结果。

## 变异验证

通过 /private/tmp/r178-mutants Go overlay 或 Node 源码读取 overlay，不覆盖仓库生产源码。四项都必须运行时断言 FAIL；编译/ReferenceError 不计。

| 变异 | 实际测试 | 结果 |
|---|---|---|
| 名单构造改回 generic normalizePosition | TestR178RealRosterPositionsAndPendingCache | FAIL：敌方仅剩 jungle/top/utility 三种位置 |
| helper 对整串子串匹配 | TestR178GameflowExactPosition | FAIL：BOTTOM 得 top、MIDDLE 得 jungle 等 |
| 删除前端 pending 补探测 | R178 两项 Node | AssertionError：20000 != 10000；旧任务未取消 |
| 删除每局 2 人抽样限制 | TestR178FreshnessConcurrentSampleLimit | FAIL：requests=8、budget=8，期望 2 |

两路 default 子代理独立只读核验 P1/P2 已完成；P2 提出的 SGP 空窗口回退边界已修复并补测。最终验证由主代理执行。

## Windows 真机待验

1. 排位进对局看两边各队五路互不重复，敌方符合实际分路；Live Client 开局延迟后应在有限补探测中纠正。
2. 对截图里“不是最新”的玩家打开总览比对，记录是哪一路、最近是否打其他模式；用新日志的 live_history_freshness 判断筛选与陈旧来源。
3. 导出日志前不关软件，导出本局结束后的新文件。必须确认版本 0.12.43 / 本账本指纹。

本轮未运行 Windows 真机对局；R177 及本轮 R178 均不宣称真机验收完成。

## 构建与最终收尾

两平台 public 后端构建通过：

- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.43-public。
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.43-public.exe。
- 两份产物均通过 desktop/verify-build-fingerprint.cjs 核验 d5961921e9b6，最终源码再次生成同一指纹。
- 两份产物均通过 verifyRiotKeyPolicy(..., "public")，没有内嵌明文或加密 Riot key。
- macOS 使用 /private/tmp/r178-selftest 隔离目录、空 RIOT_API_KEY 自检通过，确认版本 0.12.43、奖池 554 条、哈希 dee5e21f5234；日志 /private/tmp/r178-selftest.log。

key mode=public；CGO_ENABLED=0、main.version=0.12.43、main.buildFingerprint=d5961921e9b6、main.riotAPIKey= 与 main.riotAPIKeyCipher= 在两平台构建均显式设置；产物名称保留 -public，没有发布安装包。

- 第一轮 go test ./backend -count=1：通过，201.079 秒，日志 /private/tmp/r178-go-full.log。随后将 pending 短缓存条件覆盖所有经典模式队列，重新进行最终全量回归。
- 最终 go vet ./backend：通过，日志 /private/tmp/r178-vet-final.log。
- git diff --check：代码变更、构建后账本收尾均通过。

- 最终源码全量 Go 回归通过，202.870 秒，日志 /private/tmp/r178-go-full-final.log；所有工单自动验证门禁已完成。Windows 真机待验项保留。
