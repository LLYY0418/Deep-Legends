# WORKLIST-R92 R89验收缺口收尾 + D组出装推荐区块（已改为撤销）+ F组两项直接修复

诊断日期 2026-09-14。背景：WORKLIST-R89 执行后做了四路独立子代理验收（每路真编译真跑真变异，结论见 `deep-legends-r89-acceptance` 记忆），A/B/E 组全 PASS，C 组留了两处真实缺口。D 组（玩家页出装推荐区块）和 F 组当时因为"需要用户拍板"/"记账不修"没有进入执行范围，但流程上**没有真的去问用户就直接跳过了**——这是个流程漏洞，本轮工单把这几点都明确摊开，F-1/F-4 仍然维持"先诊断再定修法"，不跳过。

**2026-09-14 更新（H 组反转）**：H 组最初版本要求实现出装推荐区块，用户看过实际效果（截图）后明确表态"总览页不需要这个"。**H 组已改写为撤销指令**，如果执行方已经按旧版本做出了 `player_build.go`/`GET /api/gameplay/player-build` 路由/`web/gameplay.js` 里的 `playerBuildTarget` 等代码（诊断时确认这些文件已存在于仓库），请按新版 H 组清单整体删除，不要只砍前端显示。G 组和 I 组不受这次反转影响，按原计划执行。

---

## G 组（P0）收尾 R89 C 组两处真实缺口

### G-1 C-3-1：duration_ms 相对基线下降 ≥25% 从未真机验证

四路验收之一（C组验收子代理）明确指出：这条判据只做过算术估计和 fixture 网络下的功能验证，从未在真实网络+真实 Riot API key 下量出过实际耗时改善。这条离不开真实环境，本轮请：

1. 在真机（有真实 production 或 personal Riot key、真实网络）上，关闭 LOL 客户端，搜索同一个韩服玩家两次：一次设置 `DEEP_LEGENDS_RIOT_MATCH_CONCURRENCY=4`（模拟旧基线），一次用默认值 8（或如果 production key 已到位，设 `DEEP_LEGENDS_RIOT_KEY_TIER=production` 让并发拉到 20）。
2. 从诊断日志里取 `riot_overview_cost.duration_ms`，每组至少采样 5 次取中位数，对比降幅是否达到 25%。
3. 把这组真机 A/B 数据写进验收记录（不要只写"预计能提升"，要写实测数字）。
4. 如果达不到 25%，检查是不是被 `p.wait` 的 15次/秒限速器吃掉了收益（C 组验收子代理做过一次极端变异：把并发强改到 50，实测有效并发被压到 11——这说明限速器本身会限制并发提升的收益上限，如果真机测出来提升不明显，先看这个，不要立刻怀疑并发逻辑本身有问题）。

### G-2 C-3-2：重启后 `matches_from_disk=20`、`duration_ms<1500ms` 这个组合场景从未测过

现有测试只验证了"单条 match 磁盘命中"这个最小场景（`TestR89RiotMatchDiskAndImmutableData`），没有"20 场全部命中磁盘 + 校验具体耗时门槛"这个完整组合。请新增一条端到端测试（或真机验证）：

1. 先对一个真实韩服玩家做一次完整总览查询（20 场全部走网络，写入磁盘缓存）。
2. 重启进程（新建 `riotProvider` 或真实杀进程重启，两种都行，重点是磁盘缓存要跨这个边界存活）。
3. 再查同一个玩家，断言 `riot_overview_cost` 里 20 场 match detail 全部来自磁盘（如果现在没有 `matches_from_disk` 这个字段，先补上，按工单 C-3-2 原文要求），且 `duration_ms < 1500ms`。

### G-3 riot-matches 磁盘缓存的 600 条 / 128MB 参数没有测试锁定

**这是四路验收里发现的最需要重视的一处真实缺口**：C 组验收子代理做变异测试时，把 `newRiotMatchDiskCache` 的上限参数从 `(600, 128<<20)` 偷偷改成 `(6000000, 128<<30)`（放宽一万倍），重跑全量 `go test .`（1099 个测试）**全部通过，没有一个测试发现**。共享的淘汰机制本身（`binary_disk_budget.go`）是有护栏的，但"riot-matches 这个具体实例套用的两个数字"没人测。

请新增一条测试，直接断言 `newRiotMatchDiskCache` 调用处传入的常量值就是 600 和 128MB（或者更好：新增一条端到端测试，往 riot-matches 缓存里写入超过 600 条不同 matchID 的数据，断言磁盘条目数不超过 600、总字节数不超过 128MB）。**验收标准：把这两个参数改大后，这条新测试必须转红**——用刚才验收子代理发现问题的那个变异手法（放宽一万倍）去验证护栏是否真的堵上了。

---

## H 组（P0，已改为撤销）玩家页出装推荐区块 —— 用户拍板：不要，请整体移除

**⚠️ 这条和本文件最初版本相反：最初版本写的是"要做"，用户看到实际效果后明确决定不要，总览页不需要这块内容。如果你是刚拿到这份工单开始执行、还没做过 H 组，请直接跳过，不用实现。如果你已经按旧版本做完了（仓库里已经能看到 `player_build.go`、main.go 里 `GET /api/gameplay/player-build` 路由、`r92_test.go` 里的相关测试、`web/gameplay.js` 里 `playerBuildTarget`/`player-build-card` 等代码——诊断时确认这些已经存在），请按下面清单整体撤销，不要只砍前端显示、留着后端和数据链路。**

### H-1 撤销清单（已确认存在的产物，按下面逐项删除/回滚）

1. **整个文件删除**：`player_build.go`（后端 `handlePlayerBuild` 及支撑逻辑都在这个文件里，如果还有其他文件引用了它导出的类型/函数，一并清理引用）。
2. **main.go**：删掉 `GET /api/gameplay/player-build` 这条路由注册（`main.go:511`）。
3. **web/gameplay.js**：删掉 `playerBuildTarget` 函数（约第 843 行起）、发起请求与渲染的那段逻辑（约第 863-891 行，包含 `tab.playerBuild`/`tab.playerBuildPending`/`tab.playerBuildAttemptKey`/`tab.playerBuildAttemptAt` 这几个 tab 状态字段的读写）、`state.controllers.get(`player-build:...`)` 相关的 abort 调用（约第 1364/1374/1455 行）、以及总览页 DOM 里占位的 `<div data-player-build hidden></div>`（约第 1649 行，只删这一段 div，同一行的 `data-current-game` 占位不动，那是其他功能用的）。
4. **web/gameplay.css**：删掉 `.player-build-card`、`.player-build-card header`、`.player-build-card header > span`、`.player-build-card header > button`、`.player-build-groups`、`.player-build-group > span`、`.player-build-group > div` 这几条规则（约第 1832-1838 行）。
5. **r92_test.go / r89_test.go 里如果有针对 player-build 的测试**：一并删除，不要留着测一个已经不存在的功能。已确认存在的两处：`r89_test.go:294 TestR89PlayerBuildOPGGOnlySixSections`、`r92_test.go:138 TestR92PlayerBuildSharesStructuredCache`；连带 `playerBuildResponse` 这个类型（定义在 `player_build.go` 里，随整个文件删除即可，但如果别的测试文件里还有单独引用这个类型名的地方，搜一遍确认没有遗漏）。
6. 删完后跑一遍全量 `go build ./...` 和 `go test ./...`、以及相关的 `node --test web/*.test.cjs`，确认没有残留引用导致编译或测试失败。

### H-2 验收判据

- H-2-1 全仓库搜索 `player-build`/`playerBuild`/`常用英雄出装推荐` 应该零命中（demo-data.js 如果有独立的演示数据不算，那是展示用，和真实功能无关，可以保留或一并清理，不强制）。
- H-2-2 无客户端环境下打开韩服玩家总览页，页面结构和 R89 交付时（撤销 H 组之前）完全一致，没有多出来的空白区块或占位符。
- H-2-3 `go build ./...`、`go test ./...` 全绿，没有因为删除代码产生的悬空引用。

---

## I 组（P1，两项可直接修，两项仍需先诊断）F 组四条逐一处理

### I-1（可直接修）F-2：职业选手页 `stage=ladder` 串行瀑布

`pro_players.go:190-238` 这段 goroutine 里，`stage="directory"` → `stage="supplements"`（`enrichProLadderRanks` 之前）→ `stage="ladder"`（`enrichProLadderRanks` 之后）是严格串行的，R89 日志实测 `ladder` 单独耗时 8456ms/12999ms，是整条链路里最慢的一段。

**改法**：参考 R89 B 组的并行化思路——`enrichProLadderRanks`（`pro_players.go:231` 之后调用）需要的是每个账号的 ladder 排名，这个查询理论上可以和 `loadProSupplements`（`pro_players.go:218-222`）**并行**发起，而不是等 supplements 完全跑完再开始 ladder。请评估 `enrichProLadderRanks` 的输入是否真的依赖 supplements 的完整结果（如果只依赖账号列表本身，账号列表在 directory 阶段就已确定，两段就能并行）；如果存在真实数据依赖（比如 ladder 查询需要 supplements 补全后的账号 ID），则至少做到"按账号级别流水线"——每个账号 supplements 一完成就立刻发起该账号的 ladder 查询，不用等全部账号的 supplements 都跑完。

**验收判据**：`pro_directory_cost` 里 `stage=ladder` 的 `duration_ms` 相对当前基线（8456/12999ms）有实测下降；变异（改回严格串行）测试必须能捕捉到这个回归。

### I-2（可直接修）F-3：`/api/image` 的 CommunityDragon 回退只有内存缓存，进程重启即丢

`main.go:929-955`（`serveCommunityDragonImage`）调用 `a.loadAsset(...)`（`asset_cache.go:23`），这个函数**从头到尾没有任何磁盘写入**（已读代码确认，纯内存 map），进程重启后所有 CommunityDragon 回退图标（头像/技能/符文，未连客户端时的兜底通道）全部要重新拉取。R89 A 组已经把装备图标那条通道做了磁盘持久化（`champion-images` 缓存实例，7天/30天/64MB上限），这里应该复用同一套机制。

**改法**：给 `/api/image` 的 CommunityDragon 回退分支也接一条磁盘缓存——可以扩展 `loadAsset` 支持传入磁盘持久化选项，或者直接让 `serveCommunityDragonImage` 复用 `champion-images` 那个缓存实例（`main.go:364` 已经初始化好的 `newPublicBinaryCache`），避免重新发明一套缓存逻辑。**上限单独设置，不要和装备图标共用同一个 64MB 配额**（这条通道服务的是头像/技能/符文，量级和装备图标不同，需要重新估算合理上限，建议先测一下典型使用场景下这条通道实际缓存的文件总数和总大小再定数字，不要照抄装备图标的 64MB）。

**验收判据**：仿照 R89 A-3-3——无客户端环境下拉一批头像/技能/符文图标，杀进程重启，再拉同一批，墙钟应远快于冷启动（具体阈值改动前先测一次真实基线再定，不要凭空定数字）。变异（关掉这条磁盘持久化）测试必须转红。

### I-3（先诊断，不要直接猜着改）F-1：`specialist_runes_client_skip reason=no-top-players` 在娱乐队列下几乎必然失败

R89 日志里这条覆盖了 34 个不同 champion_id，集中在队列 1750/2400/3140。R89 工单原文的判断是"疑似 OP.GG 榜单只有排位数据、娱乐队列没有对应榜单"，但**没有实测确认**。本轮请先做诊断，不要直接改 UI：

1. 用 `load_top_players_shape` 那条链路（`champions_structured.go` 里 `loadTopPlayersForPosition`）针对这三个队列对应的模式（先确认 1750/2400/3140 具体是什么模式——快速匹配/无限制格斗/其他，不要凭猜）分别打一次 OP.GG 请求，确认是不是真的没有对应榜单数据。
2. 如果确认没有数据：按原工单建议，在 UI 上直接说明"该模式无绝活哥数据"，不要静默跳过（用户目前看到的是功能像坏了，而不是"这个模式本来就没有"）。
3. 如果确认其实有数据只是我们没查对端点：那是另一条修复路径（查对的模式参数），不要去改 UI 文案。

**这条不给验收判据**——诊断结果出来之前不知道该往哪个方向验收，诊断完成后再补一版工单。

### I-4（先诊断，不要直接猜着改）F-4：韩服玩家生涯背景图拿不到

`overview_art_source catalog_count=0 poster_available=false skin_id=76000`，R89 工单原文写"未确认是否是预期行为"。本轮请先确认：

1. 这个生涯背景图功能对国服玩家是否正常工作（如果国服也拿不到，那是全局性的资源目录问题，不是韩服专属）。
2. `skin_id=76000` 这个值本身是否合理（是不是默认皮肤兜底值，还是解析出错导致的异常值）。
3. 确认根因后再决定修法，这条同样不给验收判据。

---

## 附：验收方式（沿用 R89/R86 标准）

1. 不接受"测试通过"当完成证据，每条 G/H/I-1/I-2 判据都要做真实变异（改坏对应实现代码，测试必须转红，恢复后必须转绿）。
2. G-1/G-2 涉及真机耗时数据的，必须是真实网络下量出来的数字，不接受 fixture/mock 网络下的估算当作满足判据。
3. I-3/I-4 完成的是诊断，不是修复——诊断报告写清楚"确认的根因是什么、后面应该往哪个方向改"，不要在没有诊断结论前顺手改代码。
4. 改完 G/H/I-1/I-2 后，请用户在关闭客户端的状态下实际使用一次（搜韩服玩家、看职业选手页、看总览页出装区块），确认体验上的改善，而不只是看测试绿。
