# R181 执行账本

日期：2026-10-01。工单：WORKLIST-R181-SELF-LATEST-GAME-MISSING-AND-LANE-MATCHUP-CARD-NEVER-SHOWN.md。基线 R180 / 0.12.45，本轮版本 **0.12.46**；package.json 和 package-lock.json 两处版本同步。源码指纹 **bea94c083830**。

## 范围与证据

- 以工单提供的 0.12.45 真机日志摘要和两局时间线为定位依据；本轮没有重新读取原始 jsonl，也没有 Windows 真机对局。
- 刚结束即进入下一局时，本人 current-summoner 也可能遗漏最新同模式对局。R180 的“本人不合并 SGP”用户选择由本次明确授权执行的 R181 取代；R180 账本与索引已加替代说明。
- R179/R180 的名单行更新、组队、占位、出门装保持原实现；没有新增界面说明、tooltip 或统计口径文字。

## P1：本人历史补齐与诊断

- 去掉 loadLivePlayerMatches 的本人提前返回。本人 LCU 仍使用 current-summoner/matches；与 SGP SUMMARY 30 场并行，SGP useCache=false、3 秒期限。沿用 45 秒名单缓存、同玩家 flight 与按正数 GameID 合并去重；显示和胜负/KDA 共用当前队列胜负局的最多 10 场。
- 实际 loadGameplayLive 本人分支先用本地 cell ID 或身份判定本人，再以当前召唤师 PUUID 覆盖历史请求引用，确保 SGP 主体和缓存键不是名单里的别名。集成测试故意让名单引用与 current.PUUID 不同，验证 LCU 路径、SGP 路径及最终第一张战绩卡。
- SGP 不可用或失败仍回退 LCU；失败保留 sample_failed，不写共享五分钟 SGP 页缓存。R180 原本人不请求 SGP 测试已改为本人也合并；真实选人名单累计 5 次、十人对局累计 10 次 SUMMARY，缓存内轮询不重复。
- live_history_freshness 首次加载记录，此后只在最终展示的最新 GameID 变化时增加 load_index，同局同人最多 6 条；GameID 仅用于内部比较。
- 当前客户端运行中观察到 InProgress 的 gameflow 游戏与队列，跨 End/Lobby 保留；当前局创建 scope 时仅取同队列上一局。本人事件附 prev_game_in_lcu / prev_game_in_sgp / prev_game_shown 布尔值；未知或跨队列省略，日志不含上一局 ID。
- 覆盖：本人 LCU 缺上一局而 SGP 有；SGP 失败；重复局不重复胜负/KDA；过期补齐产生 load_index=1/2，第三次同展示不记录；6 条上限；未知/不同队列省略；当前游戏进入 InProgress 后仍取上一局；新客户端不继承旧客户端观察；真实名单 canonical PUUID 与上一局观察链路。

## P2：完整对位与 A+B

- 新增经过授权中间件的 GET /api/gameplay/lane-matchup。复用推荐详情的 OP.GG request/version/cacheKey/fetchWithMetadataCacheKey；从现有 dropped_* 过滤后的完整行查找敌方，返回 championId / enemyChampionId / winRate / games。缺失返回 HTTP 200 空对象。
- structuredCounterRowsForChampion 保留完整行；其他展示仍通过 championCounterSectionsFromRows 截取最低、最高各 5 条，不删 Hero.WeakAgainst/StrongAgainst。
- 对位、候选和分路请求统一使用 liveRecommendationTier()，来源为 champion-tier 设置、默认 emerald_plus；补齐 champion-lanes 与诊断允许的 *_plus 档位。
- A 按本人:敌方:分路:tier 单次请求并缓存； pending/完成/失败均不随 3 秒轮询重发。换本人、敌方或档位产生新键。离开选人或新局清理 pair 缓存、卡片诊断并阻止旧响应回写。
- 严格以 championLocked === true 判断锁定。预选/悬停显示 A+B，锁定仅显示 A，共用一个标题和敌方头像；对位未确定仍隐藏。大于/小于 50 使用优势/劣势文案；等于 50 使用“这局对线五五开，胜率约 50%”。
- candidate 诊断拆成 lanes-pending / lanes-failed / lane-not-locked / lane-ambiguous；context-unavailable 留给本人分路未知、非选人等情况。候选请求成功、失败、请求中、命中缓存分别记录，并保留去重。
- lane_matchup_card 每局每种结果一次：mode、shown、hidden_reason、own_locked、enemy_champion_id、tier。直接发送器、runtime 和后端都支持新字段；后端仅写白名单字段，身份类未知字段直接拒绝，允许接收的 gameId 也不写入此事件。
- Node：完整对位不在旧前 5 时 A 可见；缺行/失败/等待的隐藏原因；预选与悬停 A+B、锁定仅 A、一个敌方头像；61 次并发轮询仅一个 A 请求，换本人/敌方/档位新请求；分路三种原因及歧义；50% 和 0% 文案；真实发送器/runtime 诊断传输。
- Go：61 条完整对位中的 266 位于中间、胜率 53%、1000 场；原两组各 5 条均不含 266。先调用实际推荐 handler，再请求对位，上游详情次数始终为 1；不存在的敌方仍为 200 空对象。真实前端诊断夹具经 handleClientDiagnostic 落盘验证。
- 两路 default 子代理完成独立只读核验。主代理补齐本人 canonical PUUID 集成测试；分路占比成功/失败缓存沿用 R177 的按英雄+档位复用范围，不扩大本轮修改范围。

## 自动门禁

- 定向 Go（R181/R180/R178 freshness/R95 补人）：通过，2.209 秒；/private/tmp/r181-go-target.log。
- 全量 Node：**783/783 通过**，10.565 秒；/private/tmp/r181-web-full.log。
- 全量 Go：通过，197.639 秒；/private/tmp/r181-go-full.log。
- go vet ./backend：通过；/private/tmp/r181-vet.log。
- git diff --check：代码、索引、账本文档收尾后均通过。
- R63 静态契约同步到拆分后的完整行 loader 和前 5 分组 helper，保留完整 payload、胜率方向和原变异检出意图。

## 6 项变异

Go 使用 -overlay，Node 通过 R181_GAMEPLAY_SOURCE 读取临时变异源；未覆盖仓库生产文件。六项均为实际断言 FAIL，无编译/语法失败冒充检出。

| 变异 | 检出 |
|---|---|
| 恢复本人提前返回 | TestR181SelfLatestMergeAndPreviousGame：显示最新为 111，期望上一局 112。 |
| 恢复每人每局只记一次 | TestR181SelfRefreshRecordsOnlyChangedLatest：仅 1 条，期望 load_index 1/2。 |
| A 改回旧前 5 | Node 完整行复现：未显示 53% 胜率。 |
| 预选也视作已锁定 | Node A+B 场景：候选消失，断言失败。 |
| 取消 pair 请求去重 | Node 61 次轮询：请求数 61，期望 1。 |
| 对位接口直接请求上游 | TestR181LaneMatchupFullRowsAndRecommendationCache：详情次数从 1 增至 2。 |

源、overlay 和日志：/private/tmp/r181-mutants/{self-early-return,once-per-player,old-top-five,intent-locked,no-dedup,cache-bypass}。

## 构建与真机边界

- key mode=**public**；CGO_ENABLED=0，两平台显式 main.version=0.12.46、main.buildFingerprint=bea94c083830、main.riotAPIKey=、main.riotAPIKeyCipher=。
- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.46-public；Windows amd64：/private/tmp/Deep-Legends-backend-0.12.46-public.exe。构建通过；两份均通过 verifyBuildFingerprint 与 verifyRiotKeyPolicy(..., "public")。产物均保留 -public 文件名，不生成或发布安装包。
- macOS 自检使用空 RIOT_API_KEY 与隔离 /private/tmp/r181-selftest；通过：0.12.46、奖池 554、哈希 dee5e21f5234；/private/tmp/r181-selftest.log。最终源码再次计算指纹与两份构建一致。

Windows 真机三项留给用户：

1. 同模式刚结束马上进入下一局，确认本人第一张战绩卡为刚结束那局、总数最多 10。
2. 预选后敌方同路锁定，确认 A+B；锁定本人后 B 消失、A 保留。
3. 软件保持运行再导出日志，确认 0.12.46 / bea94c083830、本人的 prev_game_shown 与 lane_matchup_card。
