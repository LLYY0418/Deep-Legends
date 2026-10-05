# R203 执行账本

工单：[R203](../worklists/WORKLIST-R203-FAVORITES-STUCK-UNDER-STARTUP-OVERLAY-RESCAN-LOOP-AND-BLOCKING-STATE-AUDIT.md)。基线 0.12.65（R201/R202 已发布），本次版本 **0.12.66**。代码实现、完整验证、public 构建和 GitHub Latest 发布已完成；真机验收待用户日志。

## 证据与判断

工单给出的 27 秒四次收藏刷新均成功、网格 12778 个 DOM 节点/1102 张图片，与源码的“收藏加载 → 启动遮罩 → app-frame inert”一致，不能据此称后端卡死。刷新来源未记录，因此原日志不能区分用户重试与自动补刷，也不能证明库存事件一定来自刷新自身。

原始 `lol-loot-diagnostics-1003-2144.jsonl` 在本次最终核验时已无法从本机 Downloads 找到；上述日志数字引用工单，本账本不额外声称已复核未提供的原始记录。新版本补齐来源、事件类别/刷新阶段/是否抑制、实际渲染保留情况及每局堆分配位置，留待下一份日志验证。

原实现 showReadingOverlay 已有“timer 存在时不重设”的检查，工单关于每次显示都重计时的猜测不完全成立。实际问题包括收藏误用全局遮罩、隐藏后可再打开、重试走全量刷新，以及初始状态请求失败时未启动兜底。

## P1 逐项落实

| 项目 | 实现与边界 | 验证 |
|---|---|---|
| 全局遮罩用途 | `backend/web/app.js:523` 只判断客户端连接与身份 readiness，收藏 loading、lastSync/lastAttempt 变化不再触发；奖池切换用现有 toast；SSE 复用同一身份判断。初始化时立即启动连接遮罩时限 | R203 Node：1183 张卡片，8 秒一次 lastSync，连续刷新不出现遮罩/inert，原卡片节点保持 |
| 收藏局部加载 | `app.js:617` 所有网络刷新均保留非空列表，内容相同不重建。尚无 snapshot 的初次收藏在网格使用现有准备占位，不留下无限 loading=true | R203 Node：首次空列表仅局部占位；非 force 刷新同样保留卡片 |
| 15 秒上限 | 首次显示创建一次 timer；重复显示、身份重试不延长。真正隐藏清除 timer；超时后的 overlaySuppressed 阻止同连接重复出现，解除 inert；现有 notice 放在各页均可见的位置显示“召唤师信息读取失败”和重试 | R203 Node：3 次再次显示、2 次重试后在 15000ms 退出；后续状态更新不重弹 |
| 身份独立重试 | `app.js:573` → POST `/api/identity/refresh?source=overlay_retry`；同一请求 single-flight，1 秒去抖；接口调用 identity-only 分支，初始身份变化也不排队收藏扫描。顶部手动重新读取仍保留原全量行为 | R203 Node/Go：没有 `/api/refresh` 或 hard-refresh；初次身份补全不触发收藏请求 |
| 自动补刷 | `app.js:2489` 前台且收藏页才允许；整个页面会话记录上次发起时间，60 秒最多一次，切换子页/视图和完成旧请求不清掉限流时间 | R203 Node：持续 dirty、后台/非收藏页均验证；desktop 集成测试模拟 60 秒前后视图切换 |
| 后端事件抑制 | `connection_manager.go:743` 刷新结束后 5 秒内只记事件，不标 dirty；**刷新期间的变化仍保留**，避免未经证实就丢掉真实库存操作。refresh 完成提交与退出均记录完成时间；原 clearCollectionDirtyThroughLocked 的“保留刷新期间的新变化”护栏不变 | R203 Go：结束后 3 秒 suppressed=true，6 秒正常；during_refresh=true 的新变化保留；原 LCU event 回归 |
| 来源与排障 | 接受请求记录固定 enum user_refresh/overlay_retry/dirty_rescan/ensure/event；直接后台扫描补记 event，begin 生命周期保留接受请求来源。身份重试单独记 identity_refresh_request/source=overlay_retry，实际不虚构收藏扫描。dirty 只记 inventory/loot/champions/other 类别，记录 during_refresh/suppressed；collection_render_client 实际保存 view/item_count/force/kept_visible | Go enum、URI 类别、初次身份与渲染日志落地；Node 诊断传输字段保留 |

## P2 全项目阻塞状态审计

扫描范围为 `backend/web/*.js` 的全部 13 个 JS 模块（其中 demo-data.js 仅演示环境加载）及 Go HTTP/background 等待链。下表包含改动与保留理由。阅读、编辑、确认和用户主动进入的全屏允许超过 15 秒；它们没有自动任务独占整页、可由用户结束，强制关闭会中断阅读或丢掉草稿。自动启动遮罩严格限制 15 秒。

### 阻塞界面

| 文件/位置 | 场景、触发原因 | 最长显示/退出 | 处理、诊断、测试 |
|---|---|---|---|
| `app.js:523/541/560`、`index.html:26` | 未连接或身份未就绪的全窗口遮罩/inert | 15 秒；到时释放界面，可在状态条重试 | 修复用途、起始兜底、抑制重弹；blocking_state_client startup show/hide/timeout；R203 三项遮罩测试 |
| `app.js:1487/1851/3480` | 用户打开皮肤/炫彩详情 | 用户阅读时长；关闭按钮/Esc，原画全屏先 Esc 退出，再 Esc 关窗；媒体看门狗 12 秒、generation 取消旧结果 | 保留阅读模式；runtime 原生 dialog show/close（包含移出 DOM 后关闭）与 artwork_fullscreen 诊断；R203 dialog 测试及既有媒体/全屏回归 |
| `favorites-facade.js:467/512` | 用户打开头像/旗帜详情 | 用户阅读时长；关闭按钮、背景、Esc；图片读取 10 秒 | 保留；统一原生 dialog 分类日志，既有 facade 测试 |
| `champions.js:1763/1806` | 用户打开英雄梯度对照弹窗 | 用户阅读时长；关闭按钮/Esc、恢复焦点；数据请求独立有超时 | 保留；统一日志分类 champions；既有 champion dialog 回归 |
| `suite.js:738/781/797/1084` | 用户编辑征召序列 | 用户编辑时长；保存/取消/关闭/Esc；事件刷新保持现有草稿与节点 | 保留，不能 15 秒强关草稿；统一 champselect 分类日志及原有 revision 日志；R87 对话框/事件回归 |
| `gameplay.js:2681/2727` | 用户打开生涯详情/设置 | 用户阅读时长；关闭/Esc | 保留；统一 career 分类日志；既有 career 回归 |
| `app.js:3833/3924` | 用户打开版本更新对话框、选择下载/安装 | 用户决策/下载可长于 15 秒，允许关闭/后台下载；安装启动由已验证 R201 桌面流程隐藏窗口并限时等待进程 | 保留既有更新流程；统一 update 分类日志，原下载/安装日志；desktop updater/NSIS 回归 |
| `app.js:setupFloatingTooltips` | 原生 dialog 关闭归还焦点触发 data-tooltip | 只允许 pointer hover 或 :focus-visible；普通恢复焦点不显示 | R204 P7 全局修正，Node 验证恢复焦点隐藏、鼠标/键盘显示 |
| `app.js:3108`、`gameplay.js:4623` | 用户退出/启动回放时的原生 confirm | 用户选择时长；确定/取消，原生窗口可关闭 | 保留确认语义，不记录提示文本；统一 confirmation show/hide；R203 取消返回值及隐私测试 |
| `app.js:2839` | 已确认服务退出/授权失效的失败页 | 错误提示持续到用户重启/重新连接；可退出软件，没有新增 inert | 不改：失败提示不是进行中的加载遮罩；现有 backend lifecycle、fatal reconnect 回归 |
| `gameplay.js` 局部 panel、`app.js` grid、其余模块 | 面板/网格骨架、单按钮 disabled、aria-busy | 各请求预算及 finally/generation；导航仍可用 | 不提升为全窗口阻塞；见下一表，既有局部加载测试保留 |

统一原生 dialog 日志在 `runtime.js:341`，先执行原生 API，只有实际成功打开才记 show；close 方法和 native close 事件共享 WeakMap 去重，DOM 移除后的 close 也记 hide。不记录动态标题/ID/确认文本；遥测保持原有 30 秒重复抑制与容量边界。

### 自动重请求与加载复位

| 文件/位置 | 场景、并发/频率/来源 | 加载复位与处理 | 测试/保留理由 |
|---|---|---|---|
| `app.js:431/604/617/2489` | status 8 秒预算；一次 status token；收藏 ensure/dirty single-flight；dirty 60 秒/前台；同请求 key 的旧皮肤请求被 abort | finally/token 或断连复位；无 snapshot 初次显示局部占位，无永久 loading；所有旧卡片保留 | R203、R71/refresh-orchestration；源头全屏错误与补刷限流已修 |
| `app.js:1956/2052/2085/2119` | account/history/pools/catalog 当前页触发，缓存及请求序号；HTTP API 10–15 秒 | 对应 requestSeq、pool generation 与 finally；切换/取消不提交旧结果 | 不改：局部读取且有旧结果护栏，无自身结果→重新扫描的通路 |
| `suite.js:287/1683/1759/1815` | facade 单飞/controller、15 秒 API；事件 800ms 合并；挑战补读 3 次、3 秒；trigger=manual/sse/poll 已传播 | controller/token finally 复位；config loading finally；领奖按项超时、取消、有限队列和 claiming finally | 不改原调度，保留 R87/R185；新增统一 dialog 诊断 |
| `friends.js:123/405` | loading 合并为一次 pending；外部事件 300ms 合并；API 10 秒；source direct/event/manual | generation、abort、finally；断连/销毁释放 controller/tick/timer | 补来源日志；不由响应 publishFriendPresence 回发 friends-updated；friends/R203 全量回归 |
| `pro-players.js:106` | 单飞；成功缓存 5 分钟，失败 30 秒；updating 前台 1.5 秒 poll，API 60 秒 | finally，关闭/销毁 timer；长请求仅职业局部面板 | 补 direct/manual/poll 来源日志；保留请求/本地旧资料回退 |
| `champions.js:407/451` | 工作区/用户筛选由 token 与 api 请求 key abort 旧请求；缓存 freshness；不是持续状态轮询 | finally 按 token+mode+tier+position，15 秒默认 API，局部加载 | 补 workspace/direct 来源；原筛选/旧结果回退测试，不串行阻塞用户新筛选 |
| `champions.js:316/522/575/635/657/693/725/766/843/933/995` | Arena、海斗详情/图鉴/品质/强化/个人出装读取；各响应 cache/flight 与 request token | detail、atlas、augment、rarity、personal、Arena local/first loading 均 finally/token；更换目标取消/忽略旧响应 | 不改：局部 loading，不锁全页；完整 champions 测试覆盖取消、过期结果与重试 |
| `gameplay.js:845/4667/7331/8152/8208` | 总览 token/controller，25 秒普通/190 秒大查询预算；live 单飞+queued、阶段/gen/gameID；常规 20 秒、未完整/选人 5 秒，位置探测最多 6 次/10 秒；phase API 8 秒；dirty 总览 7 秒延迟、最多 3 次、8 秒间隔 | tab finally、关闭 tab abort；live finally/generation；dirty 前台及参与者/活动 tab 门控，新局结果停止 retry | 保留 R87/R89/R180/R181；补 dirty_rescan 来源；190 秒只用于明确的大区历史查询，用户可切页/关页，不全局遮罩 |
| `gameplay.js:4106/4123/4148/4175/4291` | 强化/符文/物品/技能目录共享 flight；战绩追加 token/filter、自动最多 3 页及 nextAutoAppendAt | 各 loading finally；切页/断连丢弃旧 token，关闭 observer | 不改：有请求预算与局部状态；既有目录/分页回归 |
| `favorites-facade.js:405` | 主动切换头像/旗帜目录，按 view/token/controller | 10 秒图片预算、finally/AbortController；不锁 app-frame | 保留既有 controller 取消测试 |
| `image-queue.js`、`augment-artwork.js`、`overview-art.js` | 图片队列并发分 lane、超时、最多一次 retry；图案解析 cache/flight，背景按身份更新 | timeout/abort/DOM 清理、WeakMap；不产生收藏扫描 | 不改：网络受限且有退出路径；R127/R130/R185/R188/R199 相关回归 |
| `runtime.js`、`section-loader.js`、`demo-data.js` | 全局请求测量/遥测有限队列和有限重试；section 单飞加载；demo 仅显式演示环境 | 请求 timeout、finally，模块载入失败允许重试；无收藏自动反馈 | 只补固定类别状态日志；生产不加载 demo；runtime/section-loader 回归 |

### 后端等待与锁

| 文件/位置 | 原风险/预算 | 处理及对应测试 |
|---|---|---|
| `summoner_identity.go:30` | 同客户端 flight 直接 `<-done`，请求取消不能退出 | producer 和 waiter 共用调用 context 的 3 秒预算；defer 清理 flight/关闭 done，identity-only retry；R203 cancelled waiter、原 single-flight 回归 |
| `main.go:909`、`lcu_api.go:800/815` | 两个详情结果 channel 未 select context，连续 border 候选只靠每次 8 秒预算 | 请求 context 总预算 8 秒，底层 GetBytesContext 传播；两路 select；panic 关闭 channel 返回失败，避免假 200；R203 取消/异常测试 |
| `game_settings_watch.go:554/688` | s.mu 内 recordDiagnostic 可进入日志磁盘 IO | 状态锁内只比较与收集事件，解锁后写全部诊断和堆样本；LCU 5 秒 context；R198/R202 与 R203 采样测试 |
| `lp_tracker.go:217/407/467/749/805`、`connection_manager.go:860` | 开局 flight 直接等待；部分 baseline/rejected 诊断在 t.mu 内；跨账号小表无边界 | flight select context；开局任务 45 秒、结算 5 分钟总预算（18 次轮询和稳定观测需要大于 15 秒，后台不阻塞界面）；生产 rank callback 8 秒；waitContext 支持取消；diagnostic 收集后解锁写；R203 LP 回调 TryLock、cancelled flight、全量 LP 回归 |
| `gameplay.go:8304` | live history loader panic 后 flight 留存 | defer 故障清理并唤醒 waiter，按失败返回给 waiter；保留原缓存/队列隔离；R203 panic 后下一请求成功 |
| `facade_view_cache.go:29` | 异步 loader panic 会跳过 close(done)，后续等待到超时且无法再发起 | defer 无条件发布/释放自己的 flight，异常快照只短缓存 2 秒；共享读 12 秒，不由页面取消破坏共用数据；R203 panic 后强制重试、R106 导航/旧账号测试 |
| `main.go:1329`、`connection_manager.go` | 收藏 single-flight，snapshot/account 读取默认 LCU client 每请求 8 秒；状态 mutex 内无网络 | 保留短 mutex；诊断均锁外；完成窗口时间、来源补齐；不把短内存 mutex 机械替换为 context 锁 |
| `lcu.go:764/863` | LCU JSON/bytes、状态/媒体 | RequestJSON/GetBytesContext 带 context，http.Client 默认 8 秒；未改无缺陷路径；LCU/ownership/media 回归 |
| `overview_cache.go:45`、`asset_cache.go:30`、`riot_api.go`、`sgp_api.go`、`pro_runes_recommendations.go:16` | HTTP cache flight、队列许可、详情并发 | 已 select ctx.Done；网络在 cache mutex 外，Riot foreground/background 各有许可/预算；短写入/淘汰留锁内；保留 timeout/queue/singleflight 测试 |
| `gameplay_refresh.go:21/72/89`、`arena_live_grouping.go:359`、`live_post_game_reveal.go:162/184` | 组队/真实队伍、预热、赛后身份补全 | 阶段取消 context；8/35/40 秒预热或探测，补全最多 3 次，各次 20 秒；网络均锁外，状态按 generation 提交；不改有界后台任务 |
| `champselect*.go`、`watch_rules.go`、`pro_refresh.go` | 征召/便捷操作/职业后台批次 | 已有阶段、generation、single-flight/有限回合和各端点 context；请求在动作状态锁外，诊断来源已有 stage/reason；保留 R200/R201/R202 全套测试 |

本次未恢复 R186“保留局内设置改动”，没有修改身份归属或 R201 首卡测试策略。

## P3 内存与缓存清单

`heap_profile.go` 每局结束 60 秒复用 lobby_after_60s，一代只采一次，设置文件不可定位也会采。runtime.MemProfile 按 InUseBytes 取前 15 个分配位置，仅输出函数名与 MiB，去掉包路径前缀；无数据、文件名、源码行号或栈路径。不强制 GC，因此采样可能滞后两个 GC 周期；不能直接把样本 MB 总和当成精确 heap_alloc。

| 缓存/状态 | 容量、过期与实际清理 | 本次处理 |
|---|---|---|
| overviewQueryCache `overview_cache.go:13/104` | 256 条、2 秒普通 TTL；写入 sweep、LRU 淘汰 | 已有边界，保留 |
| live 单值/赛后单值 `gameplay_refresh.go:104`、`live_post_game_reveal.go:10` | live 20 秒或未完整 3 秒；invalidate 覆盖单值；赛后只存一局，离开/停止清零 | 不跨局累积，保留 |
| livePlayerMatchCache `gameplay.go:8304` | 256 条、45 秒、LRU；普通和同局键均有队列维度 | 保留 R202 语义，补 panic 释放 |
| liveRosterRecovery、freshness、Arena allies、premade labels | roster/freshness 按 client/game/queue 替换；Arena 当前 roster；premade 按 gameID 替换 | premade 局内 labels 加 64 上限，其余单局状态保留 |
| SGP history `sgp_api.go:801` | 256 条、24 MiB、5 分钟；失败退避 45 秒；写入 sweep+LRU，玩家失效可清除 | 已有三重边界，保留 |
| Riot match details `riot_api.go:1930` | 600 条 FIFO，独立 normalized 磁盘缓存 | 已有容量；提取同一写入 helper 供五局模拟覆盖，不改变读取语义 |
| match timeline `match_timeline.go:381` | 120 条 FIFO；缓存的是已提取的最多 60 组出装与技能序列，不保留 16 MiB 原始 timeline | 已有有效边界，保留 |
| Riot account `riot_api.go:730` | 新增 512 条；成功 24 小时、not-found 30/60 秒；读/写删除所有过期 key，按 expiry 淘汰 | 修复只增不清；保留 not-found 连续失败的原退避计数 |
| specialist rune/recent `specialist_runes.go:184/474` | 各新增 256 条；完整符文 6 小时、部分/空 30 分钟、负结果 90 秒，近期摘要 30 分钟；读写 sweep+expiry 淘汰 | 修复只在同 key 访问时过期的留存 |
| asset 正向/负向/host `asset_cache.go:30/97` | 正向 1200 条/256 MiB；负向新增 1200 条且读写 sweep；host 仅固定远程域名，60 秒退避 | 修复 negative map 无上限；不清掉仍可用的收藏图片 |
| champion/public data disk `champion_cache.go:21` | 内存 128 条/32 MiB，磁盘默认 256 条/64 MiB，单条 8 MiB；各来源 TTL/stale 策略 | 已有容量和落盘策略；新鲜失败不污染旧数据，保留 |
| proProfiles `pro_profiles.go:331`、startup cache | 新增 128 条；更新频率继续按原 15 分钟/72 小时/失败 30 秒；内存回退最多 7 天，读写和启动 sweep | 保留 R173 7 天 stale 资料，不能用 fresh TTL 提前删掉有用回退；启动入口也执行上限 |
| proPlayers/proRefresh `pro_players.go:27`、`pro_refresh.go:168` | 当前目录单值；5 分钟 TTL/失败30秒；refreshAttempted 仅固定人工 seed 账号集合 | 有有限维度与单值，保留人工归属 |
| OP.GG `opgg_insights.go:32`、ARAMKit `aramkit_rating.go:23`、rank score `rank_insights.go:62` | OP.GG 两表各32，tier90/失败30秒、history30/空10分钟；ARAMKit128、成功10分钟/失败5分钟；rank600、10分钟/negative60秒 | 已有容量，保留 |
| LP history/maps `lp_tracker.go:749` | games400；新增三张账号基线表共64账号，按 TakenAt 保留最新；baseline/stale diagnostics各256；startSeen400；pending/flights按结束/取消 defer 清理 | 每次 persist 即清理，包括无 store 情况；保留用户已有 400 局记录 |
| facadeView/catalogs `facade_view_cache.go`、`facade_icons.go` | 一账户单值30秒/失败2秒，切换清除；各目录单值/当前版本 | 无 per-game 累积，补异常释放 |
| facade fingerprints `performance_diagnostics.go:81` | 新增256，连接重置清除；sources固定类别且每分钟 flush，pending 每轮清除 | 修复动态 URI key 长期留存 |
| diagnostic sample/dedup | watch payload256/30秒；通用dedup512；recommendation64、match/champion128、roster512、其它shape128 | 已有边界；perk counts新增256、unknown queue128，轮转仅小诊断键，不改游戏数据 |
| queue filter capabilities `queue_groups.go:21` | 新增256，固定server/filter事实超限保守轮转 | 修复潜在任意 filter key 增长 |
| gameplay refs/recent ranked/season snapshots | refs4096 LRU；recent ranked128/10分钟；season snapshots512 LRU/10秒 dedup；backfill/flight结束删除 | 已有界；断连清理 refs/snapshots，保留 |
| perk async/jobs/current catalog | async attempts 只有 lcu/ddragon 两个固定 key，1分钟退避；jobs 3秒context结束delete；catalog 按当前版本/source 单值有限表 | 固定维度，不机械增加无用清理 |
| frontend response/image/diagnostic caches | runtime ResponseCache容量+TTL；目录最多当前版本/当前视图；遥测128历史/32 flight；图片按队列、DOM/WeakMap清理 | 不是本工单 backend heap 单局留存来源；保留既有边界 |

五局 Go 测试每局写入 1300 个新键，让缓存达到上限后继续五轮，验证 asset 正负、overview、timeline、SGP、live history、Riot详情、LP三表/games、Riot account和近期符文摘要的计数不再增长，并验证条目/字节限制与不再访问的过期键清理。另测小诊断表/profile 上限、每代 60 秒采样一次、排序/隐私、故障后重试。

这验证的是真实缓存写入/淘汰算法的合成循环，不是完整的五场真实对局。**真机五局大厅 heap_alloc 增量 ≤15 MiB 尚未核实**，须由用户连续 3–5 局的日志检查；现阶段不把无界小表修复或采样结果推断成已经达标。

## 验证与变异

最终 `node --test backend/web/*.test.cjs desktop/*.test.cjs`：1135 项，1134 通过、1 跳过、0 失败（263.192 秒）；`go test ./backend -count=1` 通过（254.516 秒）；`go vet ./backend`、`git diff --check` 通过。记录见 [验证摘要](../reports/r203/verification-summary.json)。R203 专项 Go 15 项、Node 8 项通过；保留并更新旧集成测试的 source query 与 60 秒新规则，不删去原请求序号、取消、single-flight、旧资料回退断言。

三项指定变异均是运行生产函数后 AssertionError FAIL，均非编译错误：恢复收藏遮罩条件、每次 show 重设计时、去掉 60 秒限流。记录见 [变异目录](../../r203-verification)。

## 构建与发布

key mode：**public**。最终源码指纹 **42989b84146c**。本地 public 构建已完成，打包后 backend 指纹一致；本地安装包 SHA256 `6a5245e2fe4362eb78d96f2ce7c776c6ec87963f199d32ce3d89d914d26e69fd`，见 [构建收据](../reports/r203/local-public-build.json)。正式云端 Windows public 构建也已完成，版本/源码指纹相同；正式安装包 SHA256 `4a4c0b11f158ab21f900cafeeff379c1932d82babf89a0d46b81b8fb9720b922`。不同宿主的安装包 SHA 可不同，正式发布采用云端验证产物；只保留 `-public` 可识别安装包名。

源码提交/标签：`d56b126751a0ce350b442d0d3c688840957ab493` / `v0.12.66`；后续仅补文档证据，不移动源码标签。

- [正式 Windows public 构建 37135310296](https://github.com/LLYY0418/Deep-Legends/actions/runs/37135310296)：success；Node 两批共1135项，1133通过、2平台跳过、0失败，发布生成测试4项通过；backend 打包前后指纹、public 模式及附件清单检查通过。
- [云端质量检查 37135310312](https://github.com/LLYY0418/Deep-Legends/actions/runs/37135310312)：Linux quality job success，含 race、全量前端/桌面、真实 Chromium、安装器检查。该工作流额外的 windows-build 复验在发布取证时仍 in_progress，**不声称整个工作流已完成**；正式发布使用上面独立成功的 Windows public Release 任务。
- [0.12.66 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.66)：id **402589430**，发布时间 **2026-10-03T16:17:49Z**（北京时间10月4日00:17:49），draft=false、prerelease=false、isLatest=true。
- 匿名、无认证、no-cache 下载 `releases/latest/download/latest.json`：version=0.12.66、fingerprint=42989b84146c，字节与正式附件完全一致，SHA256 `b10d362d3831c63e165dac20f5907d4a62c4ab0c251103263778d00048e043b4`。
- 三附件恰为安装包、latest.json、SHA256SUMS-public.txt，下载后 size/SHA 与 GitHub digest、manifest、两行 SHA256SUMS 逐项一致。正式安装包111424000字节；清单1012字节；校验表182字节。
- 所有10个旧 Release 的 id/tag/正文/target/发布时间/附件 id/name/size/digest/URL 对比保持；v0.12.60 草稿 **402378684** 原样保留。v0.12.65 tag仍指向 `be6294969a38ccab6c0ff1c6bf9727fadf501991`。

取证见 [发布核验](../reports/r203/publication-verification.json)、[正式附件核验](../reports/r203/release-asset-verification.json)、[云端构建摘要](../reports/r203/source-log-summary.json)、[构建日志](../reports/r203/build-public.log)。

## 用户真机验收

收藏各页/视图切换与刷新不再锁窗口、卡片保持；连续3–5局导出日志，核对 collection_refresh_request 来源和 dirty_rescan 频率，collection_dirty_marked 的阶段/抑制字段，以及每局一个 heap_profile_top 和大厅空闲 heap_alloc。子集选人、备战席真实应用、镜头读取等 R200–R202 待验事项沿用各自账本，不在本工单虚报通过。

## R212 收口（2026-10-04）

已关闭：用户确认（R212）。

依 R212 P1 用户确认关闭；不以自动测试替代历史真机验收事实。

来源：[R212 工单](../../WORKLIST-R212-CLOSE-VERIFIED-WORKLISTS-UPGRADE-INSTALL-GAP-AND-RELAY-FAILURE-LABELS.md)；[匿名日志证据](../reports/r212/user-log-evidence.json)（原日志行号及 SHA256）。以上为本次收口结论，早先的“待验收”记录保留其当时语境。
