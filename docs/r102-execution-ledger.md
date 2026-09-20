# R102 执行账本：最近对局排序与全量账号内置

日期：2026-09-17。范围：[R102 工单](../WORKLIST-R102-PRO-ACCOUNT-RECENCY-AND-SEED-EMBED.md)。

代码与本地自动化验收已完成；真实 Riot 调用、53 账号实际刷新耗时/配额及 Windows 客户端实测仍未完成。工作区原有 R95–R101 修改保留，本轮没有提交、推送或发布。

账号唯一录入依据是用户定稿的[33 人账号表](pro-accounts-verification-2026-09-17.md)，没有重新查询第三方归属资料。共 33 人、53 个账号：BLG 7/9、IG 6/15、T1 5/5、HLE 5/8、GEN 5/5、DK 5/11（人数/账号数）。`dyjkbysb#KR1` 仅归 Wei；Canyon 的 `JUGKlNG#kr` 中小写 l、Ruler 名称和标签中的空格均保留。名单审核日期仍为 2026-09-08，账号核对日期独立记录为 2026-09-17。

## P1–P9 落地与验收

| 条目 | 最终行为 | 验收证据 |
| --- | --- | --- |
| P1 多账号种子 | `proSeedAccount.Accounts` 支持多个账号；姓名、位置、队伍 ID 从现有 roster 取；每个账号使用带索引的独立 30 天锚点 | `TestR102SeedAccountSupportsMultipleAccountsPerPlayer`、`TestR102SeedRealNameAndPositionComeFromRoster`、`TestR102SeedAnchorKeysAreUniquePerAccount`；变异 01/02 |
| P2 最近对局 | 不过滤 queue/type，先取最近一个 match ID，再读 `gameStartTimestamp`，以毫秒 UTC 保存；独立 6 小时缓存；空记录/缺字段为未知，错误透传且不缓存 | `TestR102LastMatchStartParsesGameStartTimestamp`、`TestR102LastMatchStartHandlesNoMatches`、`TestR102LastMatchStartCachedSixHours`、`TestR102LastMatchErrorsRemainUnknownAndRetry`；变异 03/04/05/25 |
| P3 预算与回退 | 选手预算为账号数 × 4 秒，各账号另有独立 4 秒子预算；失败保留该账号旧行，后续账号继续；冷启动失败也保留已核对 Riot ID 行 | `TestR102SeedBudgetScalesWithAccountCount`（验证 4s/20s 及比例）、`TestR102SeedOneAccountFailureDoesNotBlockOthers`（真实等待第二账号子超时，首尾账号成功）、`TestR102SeedKeepsPerAccountLastSnapshotOnQuotaExhausted`、`TestR102ColdIdentityFailurePreservesReviewedAccounts`；变异 06/07/08 |
| P4 排序与公开数据 | 删除 `Primary` 字段及赋值；公开 `lastMatchAt`/`lastMatchAtKnown`；已知时间优先且降序，未知沿用 Dormant/Tier/Division/LPKnown/LP/RankStatus/名称兜底；私有持久化保留活动时间 | `TestR102SortsByLastMatchTimeDescending`、`TestR102KnownAlwaysBeforeUnknown`、`TestR102UnknownLastMatchFallsBackToRankOrder`、`TestR102PrimaryFieldRemoved`、`TestR102UpstreamActivityAndSnapshotRemainPrivate`；变异 09/10/11/12/24 |
| P5 前端 | 删除主号徽章；按钮 ID 保持 `pro-primary-only`，文字为“只看最新账号”；折叠取过滤后数组第一条，高亮根据原账号数组第 0 条 | 四个 `TestR102*` DOM 测试，包含过期 `primary=true` 和首条 dormant 的反例；变异 13/14/15/16；真实 Chromium 四宽度布局检查 |
| P6 全量内置 | 全部 33 人、53 号入库；按文档逐个核对归属和顺序，保持 Unicode/大小写，拒绝重复 | `TestR102AllThirtyThreePlayersHaveSeeds`、`TestR102DyjkbysbBelongsToWei`、`TestR102SeedAccountsMatchVerificationDoc`；最后一项直接读取定稿表，与源码逐选手比较；变异 17/18/19/26 |
| P7 回归迁移 | 保留旧缓存、隐私、队列、首屏与身份测试；只迁移被本轮需求明确替代的字段/数量/展示断言 | 下节逐项说明；全量 Go、race、Node 通过；R101 段位变异 22–25 重跑 |
| P8 文档 | 更新来源说明、Wenbo 已入库状态、排序和配额估算；核对历史文档链接 | [来源说明](pro-players-sources.md)、[DESIGN](../DESIGN.md)、[链接检查](r102-validation/document-links.json) |
| P9 未定级退避 | 成功但无 solo 条目缓存 72h；已定级 6h；失败不进入退避；活动仍独立 6h；总览提前观察到真实段位立即结束退避并从当时计 6h | `TestR102UnrankedAccountBacksOffRankQuery`（0/1/70/73h，含重启）、`TestR102FailedQueryDoesNotTriggerBackoff`、`TestR102UnrankedBackoffDoesNotAffectLastMatchQuery`、`TestR102NewlyRankedAccountResetsToNormalTTL`（含磁盘重启和不滑动续期）；变异 20/21/22/23 |

## 实现选择与边界

- 工单 P4 建议在 `buildProPlayers` 中发活动 HTTP。本实现把查询放在后台补齐阶段，让 `buildProPlayers` 继续只读取快照，避免页面渲染、身份徽章查询或首次目录响应触发最多数百次外部请求。seed 已在自己的预算中查询活动；带 PUUID 的已审核上游账号也补齐活动，按 PUUID 去重。结果通过同一排序函数处理。
- 目录、补充来源和天梯保留原 50 秒后台阶段；种子另有 53 × 4 秒预算，上游活动另按账号数 × 4 秒预算。首次发布立即带已核对账号和旧快照，之后渐进发布补齐结果；R100 的单次 100ms 队列准入限制继续生效。没有提高 Riot 并发。
- 单账号失败不清空其他账号。已有快照时保留上次真实活动时间；成功返回空列表时清除旧时间；无旧记录的失败行明确为未知，不用静态日期、来源 UpdatedAt 或 `gameCreation` 替代。
- 身份磁盘缓存由 256 条扩为 1024 条，字节上限仍为 4 MiB。53 个账号分别需要锚点、正反查、段位、活动、最近对局 ID 等键；旧容量会在一轮刷新中挤出未到 TTL 的结果。普通项继续 LRU，受保护锚点仍计入预算。R99 的实际 LRU 压力测试随容量扩至 1030 个普通项。
- `championCache.loadWithStatus` 仍保留原接口；新增成功结果 TTL 回调用于区分已定级和未定级。总览 `ranks:` 的 3 分钟缓存不变，只有成功观测真实 solo 段位且种子缓存仍为未定级时，更新其 6 小时 TTL，避免每次总览刷新都把已定级种子滑动延期。
- 稳定 PUUID、seed key 和活动补齐内部字段仍走私有快照；上游 JSON 不能注入本地活动字段。改名诊断只记事件与变化状态，不写 Riot ID/PUUID。种子天梯名次仍为未知。
- 活动查询上界估算为 `53 × 2 = 106 次/6h`，即最多 `424 次/天`。这只计算种子活动查询，假设每轮详情均未缓存；空列表、永久详情缓存和重复 PUUID 会降低实际数量。身份、段位及额外上游账号另计。这不是实测配额报告。

## 旧测试逐项迁移

以下只描述 R102 相关调整，不能把工作区全部未提交差异视为本轮修改。

1. `r99_test.go` 的 `TestR99SeedRenameSurvivesRestartAndTTL`：通过 `r102RookieSeed()` 固定原 Rookie 单账号，解析调用新增索引 0；保留 PUUID 稳定、重启改名、TTL 断言。多账号和索引独立由 R102 P1 测试新增覆盖。
2. `TestR99SeedAnchorSurvivesLRUAndHasOnlyStableField`：同样隔离原账号；260 次压力写入/256 条上限改为 1030/1024；锚点不可被挤出、持久化只含稳定字段的断言保留。
3. `TestR99SeedAppearsMergesAndNeverLeaks`：仅加载该原账号 fixture，保留同账号合并与 PUUID 不泄漏断言；不让新增 52 个账号改变原场景。
4. `TestR99SeedRespectsQuotaBudget`：使用独立单账号加载入口；额度耗尽零 HTTP、保留完整旧行等断言保留。R102 另覆盖按账号回退和部分成功。
5. `TestR99SeedQuotaDiagnosticsNeverContainIdentity`：解析签名适配账号索引，GameName 引用改为 `Accounts[0].GameName`；429 状态和诊断脱敏断言保留。
6. `TestR99SeedPresentInEveryPipelinePublication`：该项有意使用生产全目录，Rookie 从 1 个变为 4 个已核对账号，每个渐进发布阶段均断言 4 个、最终退出 partial。mock 按请求 Riot ID 返回独立稳定 ID，避免把全部账号伪造成同一人；活动列表 fixture 返回空数组。
7. `r100_test.go` 的 `TestR100QueueCallersKeepDistinctSemantics`：仅把 seed 调用适配为原 Rookie 账号与索引 0；忙配额、250ms 上界、不同 caller 等待语义断言保留。
8. `r101_test.go` 的段位 fixture helper 与 `TestR101SeedRankRespectsQuotaBudget`：隔离原 Rookie 账号并适配索引。`TestR101SeedResolvesSoloQueueRank`、`TestR101SeedRankMapsRomanDivision`、`TestR101SeedWithoutSoloQueueStaysUnavailable`、`TestR101SeedRankSixHourCacheSeparateFromOverview` 仍分别验证 solo 筛选、小段映射、无 solo 不伪造段位、种子/总览 TTL 独立。
9. 原 R101 种子主号选择断言迁移为 `TestR101SeedRecentActivityBeforeOlderUpstream`：明确给 seed 和 upstream 不同官方活动时间，断言较新的 seed 在前并保留真实段位。`Primary` 删除由 `TestR102PrimaryFieldRemoved` 和 DOM 反例测试承担；不再保留已被工单废止的主号布尔断言。
10. `r73_test.go` 的旧 dormant/主号场景更名为 `TestR73DormantUnknownActivityOrder`：只去除 Primary 断言，旧日期 dormant、未知活动时 fresh 在前、DormantCount、无日期补充仍可见等断言全部保留。R102 已知时间优先测试单独覆盖新排序。
11. `pro_players_test.go` 的 `TestProCacheSingleflightRefreshBackoffAndStale` 两处、`TestProDirectoryPublishesBeforeSlowSupplements` 一处：内部原始记录数量由 9 改为 42（原 fixture 的 6 条目录 + 3 条补充 + 33 条选手种子记录；不是 42 支球队或 42 个账号）。并发 singleflight、刷新冷却、失败旧数据、过期上限和首屏返回速度检查保留。
12. `desktop/pro-players.test.cjs`：旧 R73 主号场景改为最近账号、历史折叠、最新筛选；保留账号点击核验和跨页会话断言。删除 production fixture 的 `primary`，仅在 R102 反例中注入过期属性。去除段位列降序箭头/`aria-sort` 旧断言，因为当前账号按活动而非段位排序；空选手 tbody 不参与账号索引比较。
13. `desktop/pro-players-layout.cjs`：移除旧 primary fixture，原溢出、行高、缩进、主题和故意破坏 CSS 的检查继续运行；追加真实 Chromium 最新账号折叠检查，检测徽章空占位及四宽度溢出。
14. `scripts/r101-mutation-check.py` 的第 25 项：段位 TTL 现在由成功结果选择器决定，变异改为把该选择器的 6h 改成 3m，仍由原 `TestR101SeedRankSixHourCacheSeparateFromOverview` 检出。22–25 四项已重新运行，未把旧版 25 项历史结果当作本轮重跑。

## 自动化验收结果

命令在仓库根目录执行，Go 使用 `GOCACHE=/tmp/deep-legends-go-cache`。完整 Go/竞态测试需要 loopback `httptest`，已在获准的沙箱外执行；未开启真实 Riot 测试开关。

| 检查 | 结果 | 记录 |
| --- | --- | --- |
| `go test -count=1 -v ./...` | 通过，117.899s；1,232 个顶层测试通过，662 个子测试通过，17 项跳过；R102 的 23 个顶层测试均通过 | [Go 日志](r102-validation/go-test.log) |
| `go test -race -count=1 -v ./...` | 通过，144.030s；未报告数据竞争 | [race 日志](r102-validation/go-race.log) |
| `node --test web/*.test.cjs desktop/*.test.cjs` | 724 通过，0 失败，1 跳过（需要 Windows/PowerShell 的 R86 release 检查） | [Node 日志](r102-validation/node-test.log) |
| `node --test desktop/pro-players.test.cjs` | 26/26 通过；在修正变异文件加载路径后重新执行 | [职业页 DOM 日志](r102-validation/pro-players-node-test.log) |
| `go build -o /tmp/deep-legends-r102 .`、`go vet ./...` | 通过 | [build/vet 状态](r102-validation/build-vet.json) |
| `python3 scripts/r102-mutation-check.py`（含断点续跑） | 26/26 killed；每项 baseline 通过，故意改坏后由对应断言失败；编译/语法错误、panic、超时均不计 killed | [变异矩阵](r102-validation/mutations/matrix.json) |
| `R101_MUTATION_FROM=22-first-entry python3 scripts/r101-mutation-check.py` | 4/4 killed | [R101 迁移变异日志](r102-validation/r101-mutation-migration.log) |
| `node desktop/pro-players-layout.cjs` | 本机真实无头 Chrome，生产页面及全部样式；820/1100/1440/1920 px 均无折叠横向溢出、无主号徽章、无徽章空占位、第一账号高亮正确 | [Chromium 日志](r102-validation/chromium-layout.log) |
| P8 文档链接 | 历史两份文档当前没有该链接；来源文档中的 3 处相对链接均指向已存在的定稿表 | [链接结果](r102-validation/document-links.json) |

Go 跳过项是现有真实网络开关或本地录制样本未满足的测试，不计为通过；完整名称保留在日志和 [validation summary](r102-validation/summary.json)。Chrome 原布局检查仍报告 R73 已有的“46% 账号列使短名称右侧空隙超过 140px”说明；这与已删除徽章的空占位不同，本轮未改列宽设计，R102 的名称左缩进和单子元素检查全部通过。没有用 fixture 浏览器结果替代 Windows/League 客户端验收。

### 修正过程留痕

首次全量 Go 发现三项失败：两项旧 pipeline 测试仍固定 9 条内部记录，以及新增快照测试把 JSON `null` 与 Go `nil` 当作持久化语义差异。数量按上节迁移，快照测试改为比较完整私有 DTO 的 JSON，继续覆盖 SeedKey/活动字段；定向与完整回归随后通过。原失败日志保留为 [go-test-initial.log](r102-validation/go-test-initial.log)。

首轮变异第 13 项存活，原因是 DOM harness 仍只读取 R97 文件覆盖变量，忽略 R102 的被变异文件。修正为优先 R102、兼容 R97，重新跑全部职业页 DOM 测试及第 13–26 项变异；最终矩阵全部 killed。第 26 项故意在 TheShy/Wei 之间交换普通账号，保持人数、各队账号数、总数和唯一性不变，验证新增逐账号归属断言不会只靠计数通过。原过程日志保留为 [mutation-run.log](r102-validation/mutation-run.log)，续跑见 [mutation-resume.log](r102-validation/mutation-resume.log)。

## 真实网络与客户端待验收

[凭据可用性记录](r102-validation/credential-availability.json)只记录布尔值：当前执行环境未配置 `RIOT_API_KEY` 或构建用 `RIOT_API_KEY_CIPHER`，本地默认源码 Key 变量为空。未输出或读取任何真实 Key 内容，未对 53 个账号发起官方 API 请求。

1. 用真实测试账号确认 match-v5 响应中的 `gameStartTimestamp` 存在且单位为毫秒。本轮只有协议 fixture 验证，没有真实响应证据或可用的官方网页核验结果。
2. 测量 53 个账号的一轮真实刷新耗时和配额消耗，确认每账号 4 秒预算在实际网络下的成功率；当前 106/6h、424/天为理论活动成本上界。
3. Windows/League 客户端最终验收。最新筛选的生产 HTML/CSS 已在本机 Chrome 通过；这不代表已在 Windows 客户端实际操作。

上述环境限制明确保留，不阻塞本轮代码和可执行本地验证交付，也不声称真实网络或客户端验收已通过。
