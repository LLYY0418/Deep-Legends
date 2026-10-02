# R180 执行账本

日期：2026-10-01。工单：WORKLIST-R180-ROSTER-HISTORY-MISSING-LATEST-SAME-QUEUE-GAME.md。基线 R179 / 0.12.44，本轮版本 **0.12.45**，package.json 与 package-lock.json 两处版本同步。最终源码指纹 **97a032833e73**。

> R181 更新（2026-10-01）：刚结束即进入下一局的真机证据显示本人 LCU 也会滞后。本账本“本人不发 SGP / 不合并”及每局每人只记一次的边界由 R181 取代；0.12.46 起本人同样并行合并 SGP，诊断在最新展示局变化时记录、最多 6 条。以下保留 R180 当时的实施与验证记录。

## 结论更正与用户确认

- R178 P2 的“不陈旧 / 仅当前队列筛选”结论撤销。只抽两名他人、读取 SGP 五分钟页缓存、仅比较全模式最新时间，不能排除某名玩家遗漏当前队列最新对局。
- R178 的 preferSGP 证据驱动后续切源逻辑由本轮直接合并替代；R178 索引已备注“P2 结论由 R180 取代”，R178/R179 账本已增加更正。
- 工单 P1“含本人 SGP 比对”与 P2“本人 current-summoner、每次最多九名非本人 SGP”存在冲突。用户明确选择：**本人只记 LCU/展示诊断；其他有身份玩家比对并合并**。本人不发 SGP 请求，诊断不造 SGP 比较值。
- 当前模式筛选、胜负局筛选、最近 **10 场**上限均保留；没有新增界面文字。

## P1：每名有身份玩家的新鲜度诊断

- liveHistoryFreshnessScope 按 LCU client / gameID / queueID 隔离；每名有身份玩家含本人记录一次 live_history_freshness，删除两人抽样上限。
- 将 LCU/SGP 原窗口及请求结果作为内部只读 evidence，与 45 秒缓存和 flight 一起保留。合并与诊断共用同一次 SGP 请求，不另发抽样请求。
- 保留 team、is_current、source、window_games、queue_games 和原 LCU 窗口年龄字段；新增 slot、lcu_newest_queue_age_min、sgp_newest_queue_age_min、missing_newer_in_queue、missing_newer_any、subject_unmatched、remake_skipped、shown_newest_age_min。
- shown_newest_age_min 基于实际传给名单展示的 recentMatchesForPlayer(..., 10, queueID) 结果；两个 missing 字段分别按当前队列与全模式的 LCU 最新时间比较、以正数 GameID 去重。无有效日期的 LCU 基线时为 null，避免把证据缺失写成“缺零场”。
- subject_unmatched 与 remake_skipped 分别统计 LCU 当前队列中找不到主体、非 win/loss 的记录；两者是独立计数，可以重叠（主体缺失也可能导致 unknown result）。
- SGP 请求失败只增加 sample_failed，不写成功比较字段；名单返回可用 LCU。SGP 不可用时不发请求。所有诊断不含身份、matchID、gameID。
- 默认排位详情页 slot 在位置经过选人快照 / Live Client 纠正后计算：同队 top、jungle、middle、bottom、utility、未知，未知或同位置本人优先，序号从 0 开始。对应默认详情页行序；用户自定义排序时应结合当时排序设置核对。
- 诊断用真实内部玩家引用关联原始战绩窗口，避免使用对外匿名引用寻找主体。斗魂可能在 enrich 后补入队友，对这种未读取战绩的补入行记录 source=none、空窗口，不造数据，不访问旧数组索引。

## P2：实时两来源合并

- 非本人：并行读取 LCU 30 场与 SGP SUMMARY 30 场；SGP 使用 matchHistory(..., false)，既不读取、也不写入共享五分钟页缓存。只读取 SUMMARY，不增加详情请求。
- SGP 独立请求期限三秒，失败不阻止 LCU 返回。保留已有 SGP 不可用、失败、空结果回退 LCU，以及 LCU 失败而 SGP 成功时使用 SGP 的路径。
- 按正数 GameID 合并去重；重复局优先保留能找到主体、带胜负、带队列与时间等更完整的一份，同等完整时保留 LCU。合并后按 CreatedAt 稳定倒序，再交给原 recentMatchesForPlayer。
- 核对 LCU normalizeGameplayMatch 的 game.GameID 与 SGP convertRiotMatchInfo 的 info.GameID 都直接来自数值 gameId；没有区服前缀。SGP metadata.matchId 的前缀不参与去重，适配器及 9009800745 的同 ID 去重均有测试。
- 当前玩家直接使用 current-summoner，结果不合并；用户确认的本人无 SGP 边界已测。
- 名单缓存键包含客户端、游戏、队列、玩家引用与本人标志，保留 45 秒 TTL、同玩家 flight 和 256 项 LRU 上限。同局过期才重读；新局不会复用旧局缓存。
- 真实链路夹具：选人仅本人加四名队友有身份，四次 SGP；进入对局五名敌方身份公开，累计九次。45 秒内多次选人与对局轮询均不重复发 SUMMARY；过期后单名玩家刷新可取得新最新一局。

## P3：展示与行级更新

- 胜负、KDA、“近 N 局”和最近战绩卡共用合并后按当前队列筛选的同一批最多十场数据，重复 GameID 不会重复计数。
- 无需修改 R179 前端生产代码：实际 renderLivePlayer/renderInsightMatches 与 updateLivePanels/patchLiveRosterPanel 的 DOM 回归已验证，九场补成十场后只有对应卡行和战绩行被替换，另外九名玩家行根节点与图片保持原引用；新增场在最左侧，rows_replaced=1、images_recreated=0。原英雄头像与已有九场图片也保留。

## 自动验证

- R180 Go 七项顶层场景：最新同队列补齐与去重统计；陈旧 SGP 页缓存绕过且不写回；SGP 失败 / LCU 失败 / SGP 不可用 / 本人分支；十名玩家并发轮询与 45 秒过期 / 新局；两来源实际并发；字段完整度 / 跳过计数 / slot；真实选人 → 对局链路及十条 team+slot 诊断。
- 最终 R180/R178 freshness/R95 斗魂补人/R175 定向回归通过，2.318 秒；/private/tmp/r180-go-target-final.log。
- R69 history / R66 TTL 原回归通过，保留失败不缓存、空窗口缓存与缓存结果切片隔离意图。
- R180 Node DOM 一项通过；/private/tmp/r180-node-target-final.log。
- 全量 Node：**777/777 通过**，17.895 秒；/private/tmp/r180-web-full.log。
- 最终 go test ./backend -count=1：通过，198.640 秒；/private/tmp/r180-go-full-final.log。
- 最终 go vet ./backend：通过；/private/tmp/r180-vet-final.log。
- git diff --check：代码、账本与索引最终收尾均通过。

首轮全量 Go 在 TestR95MissingKnownSquadMemberAndTruth 检出 enrich 后斗魂补人导致诊断数组越界。已将诊断与战绩关联改为真实玩家引用，不以最终名单索引读取原数组；原斗魂补人测试和 R175 定向回归通过，重跑全量。此修复只影响诊断关联，不改变斗魂补人或战绩加载策略。

R178 测试同步更新为即时合并与每名玩家一次诊断，保留本人、失败/空回退、同局缓存、新局重置、年龄及隐私断言；删除已被替代的两样本预算与 preferSGP 假设。新测试最初的 rank/lobby 响应与 DOM 旧节点取值夹具问题已修正，没有修改生产渲染来迎合测试。

## 变异验证

Go -overlay 位于 /private/tmp/r180-mutants，未覆盖仓库生产文件；三项全部为实际断言 FAIL。

| 变异 | 测试与检出结果 |
|---|---|
| 不去重 | TestR180MergedLatestSameQueueAndDedup：重复局占满十场，最后一场不再为正确的 GameID=103。 |
| SGP 恢复 useCache=true | TestR180BypassStaleSGPCacheWithoutWriting：最新仍为陈旧的 111，期望实时的 112，SUMMARY 请求次数也不符。 |
| 去掉合并、只用 LCU | TestR180MergedLatestSameQueueAndDedup：最新为 111，期望补入的 112。 |

日志分别为 no-dedup.log、sgp-cache.log、no-merge.log。没有把编译、依赖或语法错误计作变异通过。

两路 default 子代理探索、两路独立只读核验均完成；主代理负责实现、完整链路补测及最终门禁。

## 构建与验证边界

- key mode=**public**；CGO_ENABLED=0、main.version=0.12.45、main.buildFingerprint=97a032833e73、main.riotAPIKey=、main.riotAPIKeyCipher=，两平台显式设置。
- 最终 macOS arm64：/private/tmp/Deep-Legends-backend-0.12.45-public；Windows amd64：/private/tmp/Deep-Legends-backend-0.12.45-public.exe。两份构建均通过 verifyBuildFingerprint(..., 97a032833e73) 与 verifyRiotKeyPolicy(..., "public")；最终源码再次计算指纹一致。
- public 名称保留 -public，不发布安装包；初次构建产物在修复后以同一 public 名称重建，不保留旧指纹产物。
- macOS 自检使用空 RIOT_API_KEY 与隔离 /private/tmp/r180-selftest；最终产物自检通过：0.12.45、奖池 554、哈希 dee5e21f5234；/private/tmp/r180-selftest-final.log。

本轮没有 Windows 真机对局，自动夹具不等同于线上“最新一局”验收。用户仍需：

1. 找刚打完同模式对局的队友，确认名单第一张战绩卡就是该局，最多十场。
2. 对局后导出新日志，确认 0.12.45 / 97a032833e73；每名有身份玩家一条 live_history_freshness。若仍缺局，记录队伍、行号与排序设置，用 team + slot 和当前队列比较字段定位。
