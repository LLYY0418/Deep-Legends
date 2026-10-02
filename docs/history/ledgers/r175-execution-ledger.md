# R175 执行账本

日期：2026-10-01。执行范围：WORKLIST-R175；基线 0.12.39 / 4d2e848cbe8c；本轮 0.12.40 / e4427388bcc5。

## 证据与边界

工单引用的 1001-1347 真机日志与截图作为问题输入；本轮未重新读取原始日志，不能把工单里的进程断档推断升级为“安装目录被删除”。本轮源码证实：恢复前按 9 人分配位置切片，补人后按 10 人写入，会越界；缓存 owner 异常跳过 flight 收尾，后续请求会等待；后台裸 goroutine 的 panic 可结束 Go 进程。

采用仓库默认 Go 缓存；所有测试的身份、玩家名与 HTTP 数据均为合成夹具。未连接真实 LCU、Live Client、Riot 或用户账号。既有 R161/R163 恢复判定、R170 最新 10 条记录、R173/R174 装备行及界面文案保持原行为。

## P1：恢复人数与索引核对

loadGameplayLive 的 rawPlayers 在 gameflow/champ-select 合并和 recoverClassicLiveRoster 后才确定最终长度。以下行号以本轮完成源码为准：

| 变量 | 分配 / 构造 | 索引访问 | 结论 |
|---|---|---|---|
| rawPlayers | gameplay.go:7460；初始容量 10 | 7487/7493 追加；7512 合并；7623 恢复；7647/7743/7767 遍历 | 容量不是长度；补人前后均 append，不越界 |
| liveClientPositions | gameplay.go:7627 | 7747 写；7800–7802 读并覆盖 Players | 已移动到恢复之后，按最终人数分配 |
| response.Players | gameplay.go:7628 | 7717 写；7744/7785/7802 读写 | 同一最终人数；worker 的 index 参数按值传入 |
| premadeInputs | gameplay.go:7629 | 7718 写；7764 交给组队推断 | 同一最终人数；空 Matches 安全 |
| ranksFinishedAt | gameplay.go:7630 | 7680 写；7734 汇总 | 同一最终人数；匿名项保留零时间 |
| matchesFinishedAt | gameplay.go:7631 | 7688 写；7734 汇总 | 同一最终人数；匿名项保留零时间 |
| sessionPlayers | gameplay.go:7766 | 7768 写；7771/7773 使用 | 恢复后的长度；只在 Arena 分支构造 |
| names | gameplay.go:7619 | 克制/英雄名字查表等 | map 按英雄 ID 查询，不按 rawPlayers 下标 |
| positionMatchSources | gameplay.go:7735 | 7749 增计数 | map 按固定来源字符串计数，不按玩家索引 |
| proIndex | gameplay.go:7783 | 7786 身份匹配 | 身份索引；匿名无有效身份不会匹配职业账号 |

恢复前的 seed 是标量推荐目标；expectedArenaPlayers 是探测人数，均未留下“旧长度切片”。livePremadeAssignments 的 assignments/gameIDs/parent/rank 均按最终 inputs 长度分配，applyLivePremadeAssignments 另有 index>=len(players) 的保护。

### 匿名占位安全路径

- live_roster_recovery.go:162–171：匿名只在既有海斗允许条件、队伍核验和精确人数匹配下追加；保持 HIDDEN、无 PUUID，TeamID 为已核验短队。
- gameplay_summoner_cache.go:30–57 / gameplay.go:2696–2725：空身份可形成缓存 key，但实际 loadGameplaySummonerUncached 跳过无效 PUUID 与非正 SummonerID，不构造空身份请求；client 由真实加载入口提供，匿名不使 client 变 nil。
- gameplay.go:7665–7689：validRef 为 false 时不发段位或战绩请求；HistoryState=unavailable；registerGameplayReferenceDetails 拒绝空身份，PlayerRef 为空。
- arena_live_grouping.go:65–75：名字为字符串值；仅在 gameName/tagLine 均非空时拼完整 Riot ID；空名字列表不会 nil 解引用。
- gameplay.go:6278–6292：normalizeLiveClientPlayerName 对空字符串不产出匹配 key；nil map 读取安全，没有位置则返回空值。
- sessionPlayers：逐项值复制 lcuLivePlayer；无 PUUID 合法；经典/海斗匿名占位不会进入 Arena 分支。
- gameplay.go:7930–7975：组队推断遍历空 Matches 得到空 gameIDs 集；匿名没有直接组队信号，不凭空推断玩家关系；输出按 inputs 长度，回填另有边界保护。

### 端到端测试

backend/r175_test.go 从 cachedGameplayLive 调用真实 loadGameplayLive，使用假 LCU transport 与假 playerlist：

1. A：queue 2400/KIWI，我方 4 人、对方 5 人，完整 playerlist 缺席者无 Riot ID。返回 10 人、我方 5 人、补人同当前玩家 TeamID、Hidden=true、无 PlayerRef、summoner 与战绩均恰好 9 次；live_roster_shape players=10，team_counts=5+5。
2. B：queue 420，9→10，腾讯 alias 精确查询返回合成 PUUID；补人 reference 为该 PUUID，战绩请求恰好一次，HistoryState=empty（夹具为成功空战绩）。
3. C：queue 420，8→9，补入 Player4，Player3 查询失败仍缺席；reason=partial，总人数和战绩请求均 9，无伪造 Player3。
4. backend/testdata/r175-anonymous-live.json 由 A 的实际返回体生成；后端同时核对夹具匿名项形状。backend/web/r175.test.cjs 复用实际 renderLiveInsights/renderLivePlayer：我方/对方各 5 卡，匿名项在我方、现有隐藏身份 chip、名称按钮禁用、无可查询引用。未新增 UI 文案。

## P2：panic 隔离与可恢复的 flight

- cachedGameplayLive owner 使用 defer 完成发布与 close，无论正常、取消或 panic。只在 generation 与 flight 都仍相同时清理当前 flight/写缓存；旧代际只能关闭自己的 done，不能清理新 flight。panic 返回仅 Phase，异常结果不入缓存。诊断 site=cachedGameplayLive；预热加载异常 site=live-prewarm。
- a.goSafe / a.recoverPanic 对 app 事件使用 a.recordDiagnostic。通用 worker 使用启动时的 atomic localStore 指针；尚无存储时仅输出安全 JSON。helper 不序列化 panic 值：payload 精确为 event/site/panic_kind/frames；JSONL 存储统一增加 time/run_id/log_seq/build_fingerprint 等既有封套。
- panic_kind 只从 runtime.Error 固定前缀得到 6 类或 other。frames 用 runtime.Callers/CallersFrames，只留 main. 函数名，最多 8 项，不含参数、地址或文件路径。测试编译路径归一化为 main.，生产不需要归一化。
- gameplay_refresh 的预热与 finishArenaGroupTruth 均 goSafe。HTTP authorized 使用 ServeMux 的已注册 r.Pattern 模板，不用 URL/query，异常先记日志再返回 500 JSON。HTTP 已经发送的头部无法改写状态码；本轮测试覆盖 handler 在写响应前 panic 的正常异常路径。
- 后端全部 117 个原始生产 goroutine（103 个匿名、14 个具名调用）逐个保护，新增 helper 自身也有 defer；请求子协程分别 recover。单结果通道的 producer 增加 defer close；原 WaitGroup/Semaphore 清理继续执行。共享 asset results 在 defer 发布空失败结果；并发池 proParallelLimit/proHitParallel/runeStarterParallel/enrichProProfiles 按 job 另设恢复，避免所有 worker 退场后 producer 卡住。
- 并发 3 请求测试：注入必 panic loader，全部在 1 秒内得到空 Phase 响应，日志明确记录；恢复 loader 后下一请求成功。预热使用专门测试钩子制造真实越界，测试进程继续且 site/kind 正确。HTTP panic 后第二请求 200。100 个全部 panic 的池任务仍完成、100 条 backend_panic 都保留；隐私夹具秘密不进入事件。

### 全仓 goroutine 清单

使用 Go parser/AST 枚举生产 .go 的 GoStmt，补上 grep go func() 会漏的具名调用；排除 *_test.go、node_modules 与 .git。基线共 131 处：backend 117、installer 9、tools/window-focus-probe 5。下表“原行 → 完成行”保留基线锚点与本轮固定 site 所在行。helper 新增的两处 go func 均以 defer recoverPanic 包住，并由 TestR175EveryBackendGoroutineHasItsOwnRecovery 扫描全部生产后端 GoStmt。

installer 与 tools 是独立 Go 模块/进程，不在 Deep Legends 助手的后台 goroutine 中运行；本工单的 panic 隔离接入对象是助手 backend（工单 P2 的问题与 HTTP/诊断 store 均在该进程）。窗口侦察工具的 5 个 goroutine 也全部换成独立模块的 goSafe（同一 panic 白名单/字段，仅写安全 stderr），包含 stdin 读取、管道读取与 60 秒看门狗；其单元测试和 Windows 交叉编译通过。安装/卸载的 9 个都是独立有限任务，不属于助手的长期守护循环，保持原边界。

| 原位置 → 完成行 | 所属函数 / 序号 | 分类 | 处理 |
|---|---|---|---|
| backend/accept_focus_trace.go:52 → 52 | startAcceptFocusTrace #1 | 长期后台循环 | goSafe；固定 site=accept_focus_trace.startAcceptFocusTrace.1 |
| backend/accept_focus_trace.go:57 → 57 | startAcceptFocusTrace #2 | 长期后台循环 | goSafe；固定 site=accept_focus_trace.startAcceptFocusTrace.2 |
| backend/catalog.go:192 → 193 | loadIdentitySnapshot #1 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadIdentitySnapshot.1 |
| backend/catalog.go:196 → 199 | loadIdentitySnapshot #2 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadIdentitySnapshot.2 |
| backend/catalog.go:270 → 275 | loadCollectionSnapshotWithProvider #1 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.1 |
| backend/catalog.go:274 → 281 | loadCollectionSnapshotWithProvider #2 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.2 |
| backend/catalog.go:280 → 289 | loadCollectionSnapshotWithProvider #3 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.3 |
| backend/catalog.go:286 → 297 | loadCollectionSnapshotWithProvider #4 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.4 |
| backend/catalog.go:290 → 303 | loadCollectionSnapshotWithProvider #5 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.5 |
| backend/catalog.go:298 → 313 | loadCollectionSnapshotWithProvider #6 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.6 |
| backend/catalog.go:302 → 319 | loadCollectionSnapshotWithProvider #7 | 请求内并发 | defer recoverPanic；固定 site=catalog.loadCollectionSnapshotWithProvider.7 |
| backend/catalog.go:421 → 440 | runParallelLoaders #1 | 请求内并发 | defer recoverPanic；固定 site=catalog.runParallelLoaders.1 |
| backend/catalog.go:422 → 445 | runParallelLoaders #2 | 请求内并发 | defer recoverPanic；固定 site=catalog.runParallelLoaders.2 |
| backend/champion_cache.go:328 → 329 | scheduleDiskPrune #1 | 一次性后台任务 | defer recoverPanic；固定 site=champion_cache.scheduleDiskPrune.1 |
| backend/champion_images.go:232 → 233 | warmRankedItemIcons #1 | 请求内并发 | defer recoverPanic；固定 site=champion_images.warmRankedItemIcons.1 |
| backend/champion_images.go:269 → 272 | warmRankedItemIcons #2 | 请求内并发 | defer recoverPanic；固定 site=champion_images.warmRankedItemIcons.2 |
| backend/champions.go:933 → 934 | handleChampionRankings #1 | 一次性后台任务 | defer recoverPanic；固定 site=champions.handleChampionRankings.1 |
| backend/champions.go:938 → 941 | handleChampionRankings #2 | 一次性后台任务 | defer recoverPanic；固定 site=champions.handleChampionRankings.2 |
| backend/champions.go:1038 → 1043 | handleMayhemRSCPrefetch #1 | 一次性后台任务 | defer recoverPanic；固定 site=champions.handleMayhemRSCPrefetch.1 |
| backend/champions.go:1152 → 1159 | handleChampionAsset #1 | 请求内并发 | defer recoverPanic；固定 site=champions.handleChampionAsset.1 |
| backend/champions_structured.go:1174 → 1175 | loadStructuredDetail #1 | 请求内并发 | defer recoverPanic；固定 site=champions_structured.loadStructuredDetail.1 |
| backend/champions_structured.go:1215 → 1219 | loadStructuredDetail #2 | 请求内并发 | defer recoverPanic；固定 site=champions_structured.loadStructuredDetail.2 |
| backend/champions_structured.go:1219 → 1226 | loadStructuredDetail #3 | 请求内并发 | defer recoverPanic；固定 site=champions_structured.loadStructuredDetail.3 |
| backend/champselect.go:708 → 708 | handleChampSelectAutomation #1 | 长期后台循环 | goSafe；固定 site=champselect.handleChampSelectAutomation.1 |
| backend/champselect.go:1187 → 1188 | scheduleChampSelectRequest #1 | 一次性后台任务 | defer recoverPanic；固定 site=champselect.scheduleChampSelectRequest.1 |
| backend/champselect.go:1526 → 1529 | handleChampSelectTrade #1 | 一次性后台任务 | defer recoverPanic；固定 site=champselect.handleChampSelectTrade.1 |
| backend/claim_center.go:260 → 261 | scanEventClaims #1 | 请求内并发 | defer recoverPanic；固定 site=claim_center.scanEventClaims.1 |
| backend/connection_manager.go:200 → 200 | runConnectedSession #1 | 长期后台循环 | goSafe；固定 site=connection_manager.runConnectedSession.1 |
| backend/connection_manager.go:214 → 214 | runConnectedSession #2 | 一次性后台任务 | goSafe；固定 site=connection_manager.runConnectedSession.2 |
| backend/connection_manager.go:216 → 216 | runConnectedSession #3 | 长期后台循环 | goSafe；固定 site=connection_manager.runConnectedSession.3 |
| backend/connection_manager.go:223 → 223 | runConnectedSession #4 | 一次性后台任务 | goSafe；固定 site=connection_manager.runConnectedSession.4 |
| backend/connection_manager.go:224 → 224 | runConnectedSession #5 | 一次性后台任务 | goSafe；固定 site=connection_manager.runConnectedSession.5 |
| backend/connection_manager.go:235 → 235 | runConnectedSession #6 | 一次性后台任务 | goSafe；固定 site=connection_manager.runConnectedSession.6 |
| backend/facade_view_cache.go:54 → 55 | cachedFacadeState #1 | 请求内并发 | defer recoverPanic；固定 site=facade_view_cache.cachedFacadeState.1 |
| backend/gameplay.go:1273 → 1274 | loadGameplayOverview #1 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.1 |
| backend/gameplay.go:1278 → 1282 | loadGameplayOverview #2 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.2 |
| backend/gameplay.go:1283 → 1290 | loadGameplayOverview #3 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.3 |
| backend/gameplay.go:1301 → 1311 | loadGameplayOverview #4 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.4 |
| backend/gameplay.go:1307 → 1320 | loadGameplayOverview #5 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.5 |
| backend/gameplay.go:1334 → 1350 | loadGameplayOverview #6 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayOverview.6 |
| backend/gameplay.go:1637 → 1656 | loadRecentRankedSamples #1 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadRecentRankedSamples.1 |
| backend/gameplay.go:3395 → 3416 | loadGameplayHistoryContext #1 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayHistoryContext.1 |
| backend/gameplay.go:7619 → 7642 | loadGameplayLive #1 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayLive.1 |
| backend/gameplay.go:7648 → 7673 | loadGameplayLive #2 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayLive.2 |
| backend/gameplay.go:7656 → 7683 | loadGameplayLive #3 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayLive.3 |
| backend/gameplay.go:7746 → 7774 | loadGameplayLive #4 | 请求内并发 | goSafe；固定 site=gameplay.loadGameplayLive.4 |
| backend/gameplay.go:7789 → 7818 | loadGameplayLive #5 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayLive.5 |
| backend/gameplay.go:7793 → 7824 | loadGameplayLive #6 | 请求内并发 | defer recoverPanic；固定 site=gameplay.loadGameplayLive.6 |
| backend/gameplay_refresh.go:66 → 66 | observeGameplayPhase #1 | 一次性后台任务 | goSafe；固定 site=finishArenaGroupTruth |
| backend/gameplay_refresh.go:81 → 81 | observeGameplayPhase #2 | 一次性后台任务 | goSafe；固定 site=live-prewarm |
| backend/hexdata.go:1709 → 1710 | pruneStaleBuildsAsync #1 | 一次性后台任务 | defer recoverPanic；固定 site=hexdata.pruneStaleBuildsAsync.1 |
| backend/hexdata.go:2043 → 2046 | loadHexdataRankings #1 | 一次性后台任务 | defer recoverPanic；固定 site=hexdata.loadHexdataRankings.1 |
| backend/hexdata.go:2109 → 2114 | prefetchMayhemCatalogs #1 | 一次性后台任务 | defer recoverPanic；固定 site=hexdata.prefetchMayhemCatalogs.1 |
| backend/hexdata.go:2115 → 2122 | prefetchMayhemCatalogs #2 | 一次性后台任务 | defer recoverPanic；固定 site=hexdata.prefetchMayhemCatalogs.2 |
| backend/hexdata.go:2139 → 2148 | prefetchMayhemPostmatch #1 | 一次性后台任务 | defer recoverPanic；固定 site=hexdata.prefetchMayhemPostmatch.1 |
| backend/hexdata.go:3616 → 3627 | loadMayhemDetail #1 | 请求内并发 | defer recoverPanic；固定 site=hexdata.loadMayhemDetail.1 |
| backend/hexdata.go:3644 → 3658 | loadMayhemDetail #2 | 请求内并发 | defer recoverPanic；固定 site=hexdata.loadMayhemDetail.2 |
| backend/hexdata.go:3651 → 3668 | loadMayhemDetail #3 | 请求内并发 | defer recoverPanic；固定 site=hexdata.loadMayhemDetail.3 |
| backend/hexdata.go:3943 → 3963 | loadMayhemRSCWithPhases #1 | 请求内并发 | defer recoverPanic；固定 site=hexdata.loadMayhemRSCWithPhases.1 |
| backend/hexdata_augment_icons.go:120 → 120 | observeHexdataAugments #1 | 一次性后台任务 | goSafe；固定 site=hexdata_augment_icons.observeHexdataAugments.1 |
| backend/lcu_events.go:90 → 90 | ListenEvents #1 | 长期后台循环 | goSafe；固定 site=lcu_events.ListenEvents.1 |
| backend/live_prewarm.go:89 → 90 | warm #1 | 一次性后台任务 | defer recoverPanic；固定 site=live_prewarm.warm.1 |
| backend/loot_metadata.go:41 → 42 | loadLootMetadata #1 | 请求内并发 | defer recoverPanic；固定 site=loot_metadata.loadLootMetadata.1 |
| backend/lp_tracker.go:282 → 282 | handlePhase #1 | 一次性后台任务 | goSafe；固定 site=lp_tracker.handlePhase.1 |
| backend/main.go:412 → 416 | main #1 | 一次性后台任务 | defer recoverPanic；固定 site=main.main.1 |
| backend/main.go:630 → 635 | main #2 | 长期后台循环 | goSafe；固定 site=main.main.2 |
| backend/main.go:634 → 640 | main #3 | 一次性后台任务 | defer recoverPanic；固定 site=main.main.3 |
| backend/main.go:889 → 905 | handleSkinDetails #1 | 请求内并发 | defer recoverPanic；固定 site=main.handleSkinDetails.1 |
| backend/main.go:893 → 912 | handleSkinDetails #2 | 请求内并发 | defer recoverPanic；固定 site=main.handleSkinDetails.2 |
| backend/main.go:1189 → 1211 | refreshIdentityWithClient #1 | 一次性后台任务 | defer recoverPanic；固定 site=main.refreshIdentityWithClient.1 |
| backend/mayhem_sampler.go:67 → 67 | startMayhemSampler #1 | 长期后台循环 | goSafe；固定 site=mayhem_sampler.startMayhemSampler.1 |
| backend/objective_diagnostics.go:36 → 36 | objectiveEventRecorder #1 | 长期后台循环 | goSafe；固定 site=objective_diagnostics.objectiveEventRecorder.1 |
| backend/opgg_insights.go:280 → 281 | startOPGGHistoricalRanks #1 | 一次性后台任务 | defer recoverPanic；固定 site=opgg_insights.startOPGGHistoricalRanks.1 |
| backend/opgg_insights.go:671 → 674 | startOPGGGameTiers #1 | 一次性后台任务 | defer recoverPanic；固定 site=opgg_insights.startOPGGGameTiers.1 |
| backend/perk_catalog_async.go:34 → 35 | enrichPerkAugments #1 | 一次性后台任务 | defer recoverPanic；固定 site=perk_catalog_async.enrichPerkAugments.1 |
| backend/prestige.go:137 → 138 | handlePrestigeImage #1 | 一次性后台任务 | defer recoverPanic；固定 site=prestige.handlePrestigeImage.1 |
| backend/pro_players.go:246 → 247 | loadProPlayers #1 | 一次性后台任务 | defer recoverPanic；固定 site=pro_players.loadProPlayers.1 |
| backend/pro_players.go:328 → 331 | loadProPlayers #2 | 一次性后台任务 | defer recoverPanic；固定 site=pro_players.loadProPlayers.2 |
| backend/pro_players_ladder.go:80 → 80 | newProLadderPipeline #1 | 请求内并发 | goSafe；固定 site=pro_players_ladder.newProLadderPipeline.1 |
| backend/pro_players_supplement.go:97 → 98 | loadProSupplements #1 | 请求内并发 | defer recoverPanic；固定 site=pro_players_supplement.loadProSupplements.1 |
| backend/pro_players_supplement.go:135 → 138 | loadProSupplements #2 | 请求内并发 | defer recoverPanic；固定 site=pro_players_supplement.loadProSupplements.2 |
| backend/pro_profiles.go:169 → 170 | enrichProProfiles #1 | 请求内并发 | defer recoverPanic；固定 site=pro_profiles.enrichProProfiles.1 |
| backend/pro_refresh.go:138 → 138 | startProSeedRefresh #1 | 长期后台循环 | goSafe；固定 site=pro_refresh.startProSeedRefresh.1 |
| backend/pro_runes.go:335 → 336 | proParallelLimit #1 | 请求内并发 | defer recoverPanic；固定 site=pro_runes.proParallelLimit.1 |
| backend/pro_runes.go:368 → 371 | kick #1 | 一次性后台任务 | defer recoverPanic；固定 site=pro_runes.kick.1 |
| backend/pro_runes_recommendations.go:533 → 534 | proHitParallel #1 | 请求内并发 | defer recoverPanic；固定 site=pro_runes_recommendations.proHitParallel.1 |
| backend/profile_facade.go:150 → 151 | loadFacadeStateTriggered #1 | 一次性后台任务 | defer recoverPanic；固定 site=profile_facade.loadFacadeStateTriggered.1 |
| backend/profile_facade.go:1141 → 1143 | scheduleFacadeLoginReset #1 | 一次性后台任务 | goSafe；固定 site=profile_facade.scheduleFacadeLoginReset.1 |
| backend/qq101.go:461 → 462 | startQQ101Probe #1 | 一次性后台任务 | defer recoverPanic；固定 site=qq101.startQQ101Probe.1 |
| backend/qq101.go:520 → 523 | qq101ProbeWithPatch #1 | 请求内并发 | defer recoverPanic；固定 site=qq101.qq101ProbeWithPatch.1 |
| backend/r99_probe.go:199 → 200 | recordR99SurfaceShape #1 | 请求内并发 | defer recoverPanic；固定 site=r99_probe.recordR99SurfaceShape.1 |
| backend/rank_insights.go:437 → 438 | handleGameplayMatchTiers #1 | 请求内并发 | defer recoverPanic；固定 site=rank_insights.handleGameplayMatchTiers.1 |
| backend/ranked_split_probe.go:27 → 27 | startRankedSplitProbe #1 | 一次性后台任务 | goSafe；固定 site=ranked_split_probe.startRankedSplitProbe.1 |
| backend/riot_api.go:1380 → 1381 | loadRiotOverview #1 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.1 |
| backend/riot_api.go:1387 → 1390 | loadRiotOverview #2 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.2 |
| backend/riot_api.go:1394 → 1399 | loadRiotOverview #3 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.3 |
| backend/riot_api.go:1409 → 1416 | loadRiotOverview #4 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.4 |
| backend/riot_api.go:1416 → 1425 | loadRiotOverview #5 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.5 |
| backend/riot_api.go:1500 → 1511 | loadRiotOverview #6 | 请求内并发 | defer recoverPanic；固定 site=riot_api.loadRiotOverview.6 |
| backend/riot_history_filter.go:69 → 70 | matchIDsForOverview #1 | 请求内并发 | defer recoverPanic；固定 site=riot_history_filter.matchIDsForOverview.1 |
| backend/rune_starter_items.go:215 → 216 | runeStarterParallel #1 | 请求内并发 | defer recoverPanic；固定 site=rune_starter_items.runeStarterParallel.1 |
| backend/season_stats.go:756 → 757 | startSeasonStatsRefresh #1 | 一次性后台任务 | defer recoverPanic；固定 site=season_stats.startSeasonStatsRefresh.1 |
| backend/season_stats.go:970 → 972 | startSeasonBackfill #1 | 长期后台循环 | goSafe；固定 site=season_stats.startSeasonBackfill.1 |
| backend/specialist_runes.go:265 → 266 | loadSpecialistRunes #1 | 请求内并发 | defer recoverPanic；固定 site=specialist_runes.loadSpecialistRunes.1 |
| backend/specialist_runes.go:323 → 326 | loadSpecialistRunes #2 | 请求内并发 | defer recoverPanic；固定 site=specialist_runes.loadSpecialistRunes.2 |
| backend/specialist_runes.go:523 → 528 | add #1 | 请求内并发 | defer recoverPanic；固定 site=specialist_runes.add.1 |
| backend/update.go:262 → 262 | Start #1 | 长期后台循环 | goSafe；固定 site=update.Start.1 |
| backend/update.go:376 → 377 | Check #1 | 一次性后台任务 | defer recoverPanic；固定 site=update.Check.1 |
| backend/update_download.go:116 → 117 | Download #1 | 一次性后台任务 | defer recoverPanic；固定 site=update_download.Download.1 |
| backend/update_download.go:267 → 269 | downloadSource #1 | 长期后台循环 | goSafe；固定 site=update_download.downloadSource.1 |
| backend/update_http.go:86 → 87 | scheduleQuit #1 | 一次性后台任务 | defer recoverPanic；固定 site=update_http.scheduleQuit.1 |
| backend/watch_rules.go:495 → 495 | handleEvent #1 | 一次性后台任务 | goSafe；固定 site=watch_rules.handleEvent.1 |
| backend/watch_rules.go:504 → 504 | handleEvent #2 | 一次性后台任务 | goSafe；固定 site=watch_rules.handleEvent.2 |
| backend/watch_rules.go:506 → 506 | handleEvent #3 | 一次性后台任务 | goSafe；固定 site=watch_rules.handleEvent.3 |
| backend/watch_rules.go:548 → 549 | handleChampSelect #1 | 一次性后台任务 | defer recoverPanic；固定 site=watch_rules.handleChampSelect.1 |
| backend/watch_rules.go:658 → 661 | scheduleMarked #1 | 一次性后台任务 | defer recoverPanic；固定 site=watch_rules.scheduleMarked.1 |
| backend/watch_rules.go:926 → 931 | scheduleAutoMatchmaking #1 | 一次性后台任务 | defer recoverPanic；固定 site=watch_rules.scheduleAutoMatchmaking.1 |
| installer/application_launch.go:37 → 独立模块原位 | startApplication #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/execute_prewarm.go:185 → 独立模块原位 | Poll #1 | 一次性后台任务（有限预热/启动） | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/handoff.go:45 → 独立模块原位 | runApplicationHandoff #1 | 一次性后台任务（有限预热/启动） | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/install_windows.go:149 → 独立模块原位 | install #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/prewarm.go:55 → 独立模块原位 | runStartupPrewarm #1 | 一次性后台任务（有限预热/启动） | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/uninstall/webview_windows.go:147 → 独立模块原位 | onMessage #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/uninstall/webview_windows.go:164 → 独立模块原位 | uninstall #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/webview_windows.go:110 → 独立模块原位 | validatePath #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| installer/webview_windows.go:191 → 独立模块原位 | onMessage #1 | 一次性后台任务 | 独立安装/卸载进程的有限任务；不在助手后台中运行，保持原执行边界 |
| tools/window-focus-probe/actor_windows.go:65 → 65 | runActor #1 | 长期后台循环 | goSafe；固定 site=window-probe.actor_windows.runActor.1；独立 stderr 安全 JSON |
| tools/window-focus-probe/actor_windows.go:83 → 83 | runActor #2 | 长期后台循环 | goSafe；固定 site=window-probe.actor_windows.runActor.2；独立 stderr 安全 JSON |
| tools/window-focus-probe/controller_windows.go:75 → 75 | startActor #1 | 长期后台循环 | goSafe；固定 site=window-probe.controller_windows.startActor.1；独立 stderr 安全 JSON |
| tools/window-focus-probe/controller_windows.go:76 → 76 | startActor #2 | 长期后台循环 | goSafe；固定 site=window-probe.controller_windows.startActor.2；独立 stderr 安全 JSON |
| tools/window-focus-probe/main_windows.go:69 → 69 | runController #1 | 一次性后台任务 | goSafe；固定 site=window-probe.main_windows.runController.1；独立 stderr 安全 JSON |

## P3：桌面证据及存储声明

新增 desktop/backend-evidence.cjs，随 package.json 的 build.files 打包，自动纳入 source-fingerprint。main 的 stderr handler 同步保留末尾 64 KiB；onBackendClosed 在原退出 UI 逻辑前记录 desktop_backend_exit。只导出 code/signal/uptime_ms/panic_kind/frames；frames 用限定函数名语法提取，最多 8，丢弃参数和路径。信号与类型均白名单。原始 stderr 仍只在本地 desktop.log，导出照旧丢弃。

重启 IPC 在原 app.relaunch(); quitting=true; app.quit() 前写 desktop_relaunch_requested 和 userData/desktop-relaunch.json；标记只含整数 requested_at，0600 权限，不含账号/令牌。下次持有单实例锁的新 shell 启动时读到即删、记录 desktop_relaunch_completed.interval_ms。缺失、损坏、未来时间或过大标记不影响启动。没有改 relaunch 时序。该小文件属于已有“不含令牌和账号名的诊断事件”本地存储声明；本轮未新增 UI 说明。

desktop/r175.test.cjs 通过实际 main.cjs 的 VM harness 执行 spawn stderr、close、restart IPC 与新进程完成 helper。真实 Go 格式堆栈含假玩家名/PUUID/路径：原本地日志可含夹具，导出只含固定字段；另验证标记间隔、删除、缺失/损坏、64 KiB 截断与伪造额外字段拒绝。该验证不是 Windows 真机或真实 Electron 启动测量。

## 变异验证

临时 Go -overlay 位于 /private/tmp/r175-mutants；没有覆盖仓库生产文件。每项只改对应故障，运行指定 TestR175 子测试；以下均为实际运行失败，不以编译错误计数。

| 变异 | 运行测试 | 捕获结果 |
|---|---|---|
| 位置分配移回恢复前 | TestR175RecoveredRosterEndToEnd/(anonymous-ally|named-ally) | loader 真实越界被缓存捕获；A/B 返回 0 人，人数断言 FAIL |
| 匿名 TeamID 改敌方 200 | TestR175RecoveredRosterEndToEnd/anonymous-ally | 我方人数/补人 TeamID 断言 FAIL |
| 匿名仍请求战绩 | TestR175RecoveredRosterEndToEnd/anonymous-ally | history requests=10，want 9；断言 FAIL |
| 删除 cachedGameplayLive owner defer | TestR175CachePanicReleasesAllWaiters | 测试外层只接住 owner panic，followers 不关闭；1 秒等待断言 FAIL |
| 删除预热 goSafe | TestR175PrewarmPanicIsContained | Go 测试子进程未捕获 index out of range，退出 2，go test FAIL；测试进程崩溃即判定捕获 |

flight 变异最初出现未使用 generation 的编译错误；已加空引用让该变异保持可编译后重跑，最终记录的是 waiter 运行时断言 FAIL，不计初次编译失败。

## 全量验收

- go test ./backend -count=1：通过，194.398 秒。
- go vet ./backend：通过。
- node --test backend/web/*.test.cjs：753/753 通过，27.116 秒。
- node --test desktop/*.test.cjs：265 项通过，0 失败；1 个既有 Windows/PowerShell 专用发布测试在 macOS 跳过，266 项共 258.404 秒。
- git diff --check：通过。

首次后端全量运行仅 R55 检查失败：旧测试要求文本 go a.primeWatchState(client)，包装 goSafe 后仍正常执行。已将旧检查同步为安全包装内的同一调用，R55 实际 phase/lobby/action 行为回归通过。首次桌面全量仅 3 项 R82 启动测试失败：老 VM require 桩把新证据模块替换为空对象；已补消费标记函数桩，实际启动事件与 4 项 R82 变异重跑通过。最终全量不沿用这些失败结果。

另外，独立 tools/window-focus-probe 的 go test ./... -count=1 与 Windows AMD64 go build 通过；产物 /private/tmp/WindowFocusProbe-R175-public.exe。

## 构建与真机边界

key mode: **public**。main.riotAPIKey 与 main.riotAPIKeyCipher 都用 -X 显式清空；CGO_ENABLED=0。版本 **0.12.40**，指纹 **e4427388bcc5**。

- macOS ARM64：/private/tmp/Deep-Legends-backend-0.12.40-public
- Windows AMD64：/private/tmp/Deep-Legends-backend-0.12.40-public.exe

两份 go build、指纹校验均通过。macOS 使用独立 LOL_LOOT_DATA_DIR=/private/tmp/r175-selftest、空 RIOT_API_KEY 运行 -self-test，通过，输出 0.12.40、554 条奖池。只保留有 -public 后缀的本轮验证后端，未制作/发布安装包。

仍待用户 Windows 真机：隐藏玩家缺席的海斗/排位应返回我方 5 张卡、刷新不掉线；导出日志应无 backend_panic。有异常时检查 site/函数名；软件再次消失时用 desktop_backend_exit 与 relaunch requested/completed 判断后端退出和重启动作，不能据此直接断言安装目录被删。
