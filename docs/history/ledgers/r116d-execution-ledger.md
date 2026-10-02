# R116-D 执行账本：推荐页克制/协同提示 + 局内出装解析 + 队伍画像

- 工单原文：`docs/history/worklists/R116-D-推荐页克制协同与出装联动-工单.md`
- 执行契约（主控拟定）：`$PI_SCRATCH_DIR/R116-D-CONTRACT.md`
- 背景依据：`docs/r116-proposal-feasibility-review.md` §3.1 / §4.3 / §4.4、`docs/r116-probe-findings.md`
- 执行环境：macOS，无 Windows、无 League 客户端、无法进行真实对局
- 执行时间：2026-09-21

> **一句话结论**：P1-2/P1-3/P1-5 与 P1-4 阶段一全部落地并有测试；P1-4 阶段二按工单
> 「对抗变异」最后一条**只交付纯计算、不接线**（探测三项判据的真机观测值至今全部留空）；
> 真机验证三项一律标注 `待真机验证`，没有伪造任何 PASS，也没有编造任何命中率观测值。

---

## 0. 改了什么（改动后的新行号）

### `backend/gameplay.go`（8790 → 9719 行）

| 新行号 | 内容 |
|---|---|
| 3669-3672 | `gameplayRecommendationBundle` 新增 `MatchupNotices` / `SynergyNotices` / `TeamPortrait` 三个字段 |
| 3692-3770 | 新类型：`gameplayMatchupNotice`(3719) / `gameplaySynergyNotice`(3730) / `gameplayTeamPortraitLabel`(3740) / `gameplayTeamPortrait`(3749) / `gameplayNextItemSuggestion`(3764) |
| 3952-3999 | R116-D 常量与白名单：`gameplayRosterNoticeLimit=5`(3957) / `gameplayRosterSideLimit=5`(3960) / `gameplayTeamPortraitMinResolvedHeroes=4`(3963) / `gameplayTeamPortraitThresholdRatio=1.0`(3983) / `gameplayRosterMatchupPhases={"InProgress","Reconnect"}`(3999) |
| 4003-4011 | `gameplayTeamPortraitFields`：P1-5 用到的 5 项 postmatch 指标表 |
| 4017 / 4044 | `gameplayRosterChampionIDs`（严格解析查询串）/ `gameplayRosterWithout`（剔除本人） |
| 4066 / 4114 | `gameplayMatchupNotices`（P1-3 交集纯函数）/ `gameplaySynergyNotices`（P1-2 交集纯函数） |
| 4152 / 4165 | `errGameplayHexdataPageNotCached` 哨兵 / `gameplayPeekCachedHexdataPage`（**只读缓存，结构上不可能回源**） |
| 4182 / 4198 | `gameplayHeroHexdataTables`（hero-json）/ `gameplayPostmatchTable`（postmatch） |
| 4213 / 4223 | `gameplayTeamPortraitAverages` / `gameplayTeamPortraitAveragesFrom`（与 R116-B P1-6 同口径） |
| 4263 | `gameplayTeamPortraitLabels`（**P1-5 的标签生成纯函数**） |
| 4306 | `gameplayTeamPortraitFrom`（覆盖率门槛 + 组装） |
| 4335 | `(*app).gameplayApplyRosterInsights`（P1-2/P1-3/P1-5 的唯一接线点，含 ChampSelect 阶段护栏与 `gameplay_roster_insights` 诊断） |
| 4575 | `handleGameplayRecommendations` 里调用上面这个函数（`diagnosticStage = "roster-insights"`） |
| 4940 | `liveClientItem`（P1-4 阶段一） |
| 4948-4975 | `liveClientSnapshot` 新增 `ItemsByIdentity` / `ItemElementKeys` / `ItemElementsSeen` / `ItemElementsSkipped` |
| 5140 / 5196 / 5229 | `parseLiveClientItems`（宽松容错解析）/ `liveClientItemInt` / `liveClientItemBool` |
| 5248-5330 | `parseLiveClientPlayerList` 里新增 items 解析（**必须放在 position 的 `continue` 之前**，海斗 position 恒为 `OTHER` 会被归一成空串） |
| 5423 | `(*app).recordLiveClientItemsParsed`（新诊断事件 `live_client_items_parsed`） |
| 5472-5491 | `cloneLiveClientSnapshot` **深拷贝** `ItemsByIdentity` 的每个 slice + `ItemElementKeys` + 两个计数器 |
| 5656 | `liveClientItemsForIdentities`（与 `liveClientPositionForIdentities` 同一套身份优先级） |
| 5680-5706 | P1-4 阶段二的「已实现未接线」说明块 |
| 5712 / 5744 | `gameplayOwnedTerminalItemIDs` / `gameplayNextItemSuggestionFromTrios`（阶段二纯计算，**零生产调用方**） |
| 6991-7014 | 实时页 roster 循环里收集本人 items 并调用 `recordLiveClientItemsParsed` |
| 7035 | `aramMode && !arenaMode` 分支 —— **R116-探测 的埋点，本轮一字未改**（已 shasum 复核） |

### `backend/web/gameplay.js`（7029 → 7208 行）

| 新行号 | 内容 |
|---|---|
| 64-66 | `state.liveRecommendationRosters`（阵容签名旁表） |
| 4404 | 软重置时一起 clear 这张旁表 |
| 4474-4492 | Anti-scope 第 1/4 条的注释 + `R116D_MATCHUP_PHASES` / `R116D_ROSTER_SIDE_LIMIT` |
| 4494-4524 | `liveRecommendationRoster(data)`：从客户端已有的 10 人里取英雄 ID |
| 4628-4638 | `ensureLiveRecommendations`：算阵容签名、缓存命中时再比一次签名 |
| 4657-4663 | 新增三个查询参数 `phase` / `allyChampionIds` / `enemyChampionIds` |
| 4672-4676 | 请求成功后记下这份缓存对应的阵容签名 |
| 5211-5213 | `content.insight = renderLiveInsights(data, payload)`（多传一个 payload） |
| 5405-5407 | `renderLiveInsights(data, payload)` 新签名 |
| 5425-5512 | 三个嵌套 helper：`rosterChampionName` / `deltaPoints` / `confidenceText` / `evidenceSuffix` / `noticeBar` / `renderRosterNoticeBars` / `renderTeamPortraitTags` |
| 5513-5523 | `supplement` 拼装（含复用 `window.deepLegendsShared.renderMeasurementTechnique`） |
| 5524-5527 | `team(teamID, label, tone, source, extra)` 多了 `extra` 参数，画像标签只挂「我方」头部 |
| 5541-5542 | 两个非斗魂返回分支前面拼 `${supplement}` |
| 5873 | `renderBuildRecommendation` —— **P1-4 阶段二将来的接线点**（工单写的 5650/5694 已漂移到 5873） |

R116-B 的成果全部保留、未被覆盖：`gameplay.js:5229-5236` 的常驻统计口径页脚与
`5907` 附近的 P0-3-2 注释都在，`r116b.test.cjs` 全绿。

### `backend/web/gameplay.css`（1879 → 1899 行）

1881-1899 新增 11 条规则：`.live-roster-supplement` / `.roster-notice-strip`(+`small`) /
`.roster-notice`(+`b`) / `.is-weak` / `.is-strong` / `.is-synergy` / `.is-unverified` /
`.team-portrait-tags` / `.team-portrait-tag`。
**零新 hex**（全部走 `var(--bg|--ink|--muted|--faint|--line|--danger|--success|--accent|--warning)`），
`border-radius`(7px/999px)、`padding`(3px 5px / 1px 5px)、`gap`(4px/6px) 全部复用既有取值。

### 新增文件

- `backend/gameplay_r116d_test.go`（1377 行，30 个 `Test*` 函数）
- `backend/web/r116d.test.cjs`（472 行，12 个测试）

### 没有碰的文件

`hexdata.go` / `champions.go` / `champions.js` / `champions.css` / `shared.js` / `index.html` /
`season_stats.go` / `features.go` / `champion_cache.go` / `augment_contract_probe*.go` /
`README.md` / `DESIGN.md` / `desktop/package.json` / `package-lock.json` / `main.go` /
`r116b.test.cjs` / `r117*.test.cjs` / `champions.test.cjs` —— **全部一字未改**。

---

## 1. 版本号

**本轮未改 `desktop/package.json`（仍是 0.12.9）。**
按主控指示：R116-F 正在并行执行，两边同时升版本会冲突，版本由主控统一收口。
**建议升到 0.12.10，由主控在 D 与 F 都落地后按实际执行顺序统一升。**

---

## 2. 逐条验证判据的真实状态

### P1-2 + P1-3（克制/协同，低调形态）

| # | 判据 | 状态 | 证据 |
|---|---|---|---|
| 1 | 「weakAgainst 里有一条命中当前对局某个敌方英雄」→ 生成且仅生成 1 条 `MatchupNotice` | **PASS** | `TestGameplayMatchupNoticesSingleHitProducesExactlyOneNotice`（真实夹具 `hexdata-hero-157.json`，`weakAgainst[0].opponentChampionId=200`，敌方 `[200,7,8,9,10]` → 恰好 1 条，`counterDelta`/`evidence`/`confidenceLow`/`confidenceHigh` 与上游逐字相同） |
| 2 | 7 条 weakAgainst 均未命中 → `MatchupNotices` 为空**且前端不渲染任何相关 DOM** | **PASS** | 后端 `TestGameplayMatchupNoticesMissProducesNothing`（返回 nil）；前端 `R116-D renders inline bars for hits and nothing at all for misses` 的空 payload 分支：对 `roster-notice`/`live-roster-supplement`/`team-portrait`/`暂无数据`/`undefined`/`NaN` 六个 needle 全部 `doesNotMatch` |
| 3 | 一次刷新，hexdata 请求数不因为加了这两项而增加（集成测试断言） | **PASS** | `TestGameplayRosterInsightsAddZeroHexdataRequests`：同夹具同工厂跑「不带 roster 参数」与「带满 roster 参数」两次，**上游路径多重集合逐条相等**；`/api/hexdata/heroes/157` 恰好 1 次；任何 `/api/hexdata/heroes/<非157>` 出现即 FAIL |
| 4 | 各自最多 5 条、按绝对值排序 | **PASS** | `TestGameplayRosterNoticesSortByAbsoluteDeltaAndCapAtFive`（7 条全命中 → 留 5 条、`|delta|` 严格降序、top2 = 106/102） |
| 5 | 必须透传 `evidence`/`confidenceLow`/`confidenceHigh` | **PASS** | `TestGameplayRosterNoticeJSONTagsEmitBothConfidenceBounds`（直接 marshal 后逐键核对）+ `TestGameplayRosterNoticesPassThroughWeakEvidence`（`evidence:"underpowered"` 与 `evidence:""` 都原样透传，后端不过滤不改写） |
| 6 | ChampSelect 阶段不展示「对面 5 人」相关的克制 | **PASS** | 后端 `TestGameplayRosterInsightsSuppressEnemiesDuringChampSelect` + `TestGameplayRosterMatchupPhaseWhitelist`；前端 `R116-D roster sends both sides in game and only allies during champion select`。**双侧都有护栏**（前端不下发 `enemyChampionIds`，后端阶段白名单再拦一次并记 `enemy_count_before_phase_guard=5` / `enemy_count=0`） |
| 7 | InProgress/Reconnect 阶段克制正常展示 | **PASS** | `TestGameplayRosterInsightsAllowEnemiesInProgress`（`matchup_phase_allowed=true`，2 条 weak notice） |
| 8 | 不做「阵容协同 +X%」总分（Anti-scope 2） | **PASS** | bundle 里没有任何总分类字段；前端 `assert.doesNotMatch(markup, /阵容协同\s*\+/)` |
| 9 | 不为 10 个英雄各拉一次 hero-json（Anti-scope 1） | **PASS** | 判据 3 + `TestGameplayPeekCachedHexdataPageNeverHitsUpstream`（缓存空时取数入口返回 false 且**上游请求数为 0**）+ 前端 `assert.doesNotMatch(code, /\/api\/hexdata\/heroes\//)` |
| 10 | 真机：克制/协同提示出现频率与 34.6%/18.8% 量级吻合 | **待真机验证** | 见 §5 |

### P1-4（局内已出装备 → 下一步出装）

| # | 判据 | 状态 | 证据 |
|---|---|---|---|
| 阶段一-1 | `parseLiveClientPlayerList` 读 `items` → `liveClientSnapshot.ItemsByIdentity`，身份匹配用 `liveClientEntryIdentityKeys` | **PASS** | `TestParseLiveClientPlayerListReadsItems`（3 件装备逐字段核对、顺序原样保留、`items:[]` 与「无 items 键」可区分、`ItemElementKeys` = `canUse,consumable,count,itemID,slot`） |
| 阶段一-2 | 解析容错：缺失 / 非数组 / 元素非对象 / `itemID` 非数字，安全跳过并计入诊断，绝不 panic | **PASS** | `TestParseLiveClientItemsToleratesMalformedShapes`（11 个子用例 + 整份垃圾 playerlist 不 panic；键名大小写宽松 `itemid`/`SLOT` 都认；数字字符串 `"3153"` 也认） |
| 阶段一-3 | 新诊断事件 `live_client_items_parsed`，记录本人 items 长度与 itemID 集合，**生产 UI 不展示任何内容** | **PASS** | `TestRecordLiveClientItemsParsedEmitsBoundedDiagnostic`（事件含 `ui_visible:false`、`self_item_count`、`self_item_ids` 升序、`item_element_keys`、`item_elements_seen/skipped`、`identities_with_items`、`player_count`；同一套装备去重、出装变化再记一条；本人没匹配上也记，用于区分「键在但匹配不上」与「键根本没有」） |
| 阶段一-4 | 克隆点同步（易漏点） | **PASS** | `TestCloneLiveClientSnapshotDeepCopiesItems`（改克隆不影响原件 + 8 goroutine 并发克隆，`-race` 下通过） |
| 阶段一-5 | 真机核对 `live_client_items_parsed` 出现且 itemID 集合非空 | **待真机验证** | 见 §5 |
| 阶段二-1 | 前缀匹配推第 3 件，带 `winRate`/`games` | **已实现纯函数 + 完整单测，未接线** | `TestGameplayNextItemSuggestionMatchesTrioPrefix`（真实夹具 `terminalItemTrios[0]=3153:6333:123430`，已出 3153+6333 → 推 123430，`winRate=0.60355`/`games=445607` 与上游一致；且尊重上游 games 降序，已出 3031+3153 命中第 2 条） |
| 阶段二-2 | 降级规则：解析失败 / 结构不符 / 无匹配 → 整行不渲染 | **纯函数侧 PASS**（渲染侧不存在，因为没接线） | `TestGameplayNextItemSuggestionDegradesToNothing`（8 个子用例全部 `ok=false` 且返回零值） |
| 阶段二-3 | 20 秒轮询周期足够（纯本地比对，不发新请求） | **PASS（结构性）** | 阶段二是纯函数，输入是已在内存里的 `ItemsByIdentity` 与 `TerminalItemTrios`，零 I/O |
| 阶段二-4 | 真机验证下一步出装至少命中一次 | **不适用**（阶段二未接线） | 见 §4 |

### P1-5（队伍画像缺口提示）

| # | 判据 | 状态 | 证据 |
|---|---|---|---|
| 1 | 标签生成逻辑是可测试的纯函数，魔法数字不散落在渲染代码里 | **PASS** | `gameplayTeamPortraitLabels(rows, averages)`（`gameplay.go:4263`）；唯一的数字是 `gameplayTeamPortraitThresholdRatio = 1.0`（`gameplay.go:3983`），带 20 行注释说明依据。前端 `renderTeamPortraitTags` 只做 HTML 拼装，**一个数字都没有** |
| 2 | 「5 人全是脆皮 AD 刺客」→ 生成「缺前排」 | **PASS** | `TestGameplayTeamPortraitFlagsGlassCannonComp`（真实 postmatch 数据里的 238 劫 / 91 刀锋 / 121 螳螂 / 55 卡特 / 84 阿卡丽 → 出「缺前排」+「缺控制」，不出「缺持续输出」）+ `TestGameplayTeamPortraitEndToEnd`（走完整 HTTP 路径，`teamPortrait.labels` 出现在响应里） |
| 3 | 「阵容均衡」→ **不生成任何标签**（不是生成一个空标签） | **PASS** | `TestGameplayTeamPortraitBalancedCompProducesNoLabels`（58 鳄鱼 / 64 盲僧 / 84 阿卡丽 / 222 金克丝 / 412 锤石 → `labels == nil`，连「非 nil 空切片」都不接受）；前端 `R116-D renders team portrait tags in our own header only` 的均衡分支断言 `doesNotMatch(/team-portrait/)` |
| 4 | 渲染在详情 tab 我方队伍头部 | **PASS** | 同上测试：标签只出现在 `<h3>我方</h3>` 与 `<h3>对方</h3>` 之间，对方那一段 `doesNotMatch(/team-portrait-tags/)` |
| 5 | 阈值基于 hexdata postmatch 的全英雄分布自己定，不接虎牙数据 | **PASS** | `TestGameplayTeamPortraitAveragesMatchDocumentedMeans`：均值与执行简报实测值吻合（`avgDamageTaken` 46151.55 / `avgCcTime` 34.473 / `avgDmgMitigated` 48924.14 / `avgHealShield` 1595.04 / `damageShare` 0.19214），并**独立复算一遍算术均值**防止实现偷偷换口径；仓库里没有任何虎牙域名/接口 |
| 6 | 证据不足降级 | **PASS** | `TestGameplayTeamPortraitLabelsRequireUsableAverages`（四项均值任一缺失或为 0 → 一个标签都不生成）、`TestGameplayTeamPortraitFromRequiresEnoughResolvedHeroes`（5 人里只解析到 3 个 → 整块不下发；4 个仍出结论） |

### 通用验证要求

| 判据 | 状态 |
|---|---|
| `go test ./backend/... -run Gameplay` 全绿 | **PASS**（见 §3） |
| 真机：一局海斗从 ChampSelect 到结束，观察提示出现频率是否与 34.6%/18.8% 量级吻合 | **待真机验证**（见 §5） |

### 工单结束标志

| 标志 | 状态 |
|---|---|
| P1-2/P1-3/P1-5 验证判据 PASS | **PASS**（真机那两条除外，见 §5） |
| P1-4 阶段一交付 | **PASS** |
| P1-4 阶段二依探测结论决定交付或明确记录为「暂不可行」 | **明确记录**：探测结论**尚未产出**（三项判据观测值全部「待填」），按工单「对抗变异」最后一条只交付阶段一；阶段二纯计算已备好、未接线（见 §4） |

---

## 3. 验证实跑输出

```
$ go build -o "$PI_SCRATCH_DIR/dl-build" ./backend
BUILD_OK

$ go vet ./backend && gofmt -l backend/*.go
VET_OK
GOFMT_CLEAN            # 两条命令都无输出

$ go test ./backend/... -run Gameplay -count=1
ok  lol-loot-assistant/backend  9.116s

$ go test ./backend -race -run "LiveClient|Matchup|Synergy|TeamPortrait|LiveItems" -count=1
ok  lol-loot-assistant/backend  3.733s

$ go test ./backend -run "R116D|GameplayMatchup|GameplaySynergy|GameplayRoster|GameplayTeamPortrait|\
GameplayPeek|GameplayOwnedTerminal|GameplayNextItem|ParseLiveClientItems|\
ParseLiveClientPlayerListReadsItems|CloneLiveClientSnapshot|LiveClientItemsForIdentities|\
RecordLiveClientItemsParsed" -count=1
ok  lol-loot-assistant/backend  3.450s

$ go test ./backend/... -count=1                # 全量，基线 ok 217.789s
ok  lol-loot-assistant/backend  219.050s

$ cd backend/web && node --test
tests 600 / pass 600 / fail 0                   # 基线 577，其中 588 是 R116-F 并行新增，
                                                # 600 = 588 + 本工单的 12 条
$ cd backend/web && node --test r116d.test.cjs
tests 12 / pass 12 / fail 0
```

改动前基线（主控 11:57 复核）：build OK / vet 无输出 / gofmt 无输出 / `go test ./backend/...`
ok 217.789s / `node --test` 577 pass / 版本 0.12.9。**没有引入任何新失败。**

---

## 4. P1-4 阶段二：「已实现未接线」的确切位置与接线条件

### 已实现的东西

| 对象 | 位置 |
|---|---|
| `gameplayOwnedTerminalItemIDs(items []liveClientItem) []int` | `backend/gameplay.go:5712` |
| `gameplayNextItemSuggestionFromTrios(ownedItemIDs []int, trios []hexdataTrioRow) (gameplayNextItemSuggestion, bool)` | `backend/gameplay.go:5744` |
| `gameplayNextItemSuggestion` 结构体 | `backend/gameplay.go:3764` |
| 单测 | `backend/gameplay_r116d_test.go` 的 `TestGameplayOwnedTerminalItemIDsFiltersConsumablesAndJunk` / `TestGameplayNextItemSuggestionMatchesTrioPrefix` / `TestGameplayNextItemSuggestionDegradesToNothing` |
| **「未接线」这条事实本身的回归钉** | `TestR116DStageTwoStaysUnwired`：剥掉 `//` 注释后统计两个函数名在 `gameplay.go` 里的出现次数必须**恰好 1**（只有定义、没有调用方）；同时断言 `gameplayRecommendationBundle` 没有任何含 `nextitem` 的字段、`gameplay.js` 里没有 `nextItem` 字样 |

### 接线条件（两个判据都要回填）

1. `docs/r116-probe-findings.md` **§3.4** 的 `live_client_playerlist_shape.element_keys`
   回填为「**含 `items`**」；
2. 同文档 **§5.3** 的 `$.allPlayers[].items` 的 `element_keys` 回填为「**与斗魂基线一致**」
   （`itemID`/`slot`/`count`/`canUse`/`consumable`，见 §1.5 的 19 键基线）。

工单 §3.3 的原文判据是「若含 `items` 且元素结构与斗魂一致 → P1-4 判定为可行，转入 R116-D
正式实现」；「若不含 `items` 或结构不同 → 判定为不可行，从 R116-D 移除」。**两个都还没回填。**

### 接线点（结论一到就能接，各一行量级）

1. **后端**：`backend/gameplay.go:4335` 的 `gameplayApplyRosterInsights` 里，把本人
   `liveClientSnapshot.ItemsByIdentity` 的装备喂给 `gameplayOwnedTerminalItemIDs` +
   `gameplayNextItemSuggestionFromTrios(detail.TerminalItemTrios)`，结果挂到
   `gameplayRecommendationBundle` 的新字段上（`TestR116DStageTwoStaysUnwired` 会立刻红，
   提醒同步删掉/更新那条钉）。注意 `gameplayApplyRosterInsights` 目前拿不到 live client
   快照（它在推荐 HTTP 处理器里，快照在实时页处理器里），接线时要么把本人 items 通过
   查询参数传进来（与 roster 同一套做法），要么在 `app` 上加一个快照读取口——
   **后者要改 `backend/main.go`，不在本工单允许范围内，接线时需要主控批准。**
2. **前端**：`backend/web/gameplay.js:5873` 的 `renderBuildRecommendation` 里加一行
   「下一件推荐」补充行，**不替换**现有的静态核心装路线；items 解析失败 / 结构不符 /
   无 trio 前缀匹配时整行不渲染（工单阶段二第 5 条）。
   （工单写的 `gameplay.js:5650-5710` 已漂移到 5873。）

### 为什么这不算「没做完」

工单 P1-4「对抗变异」最后一条原文：*「若阶段二判定为不可行（探测结果否定），本工单只交付
阶段一的诊断解析，**不强行实现阶段二**——验收时以 R116-探测的结论文档为准，**不接受
『猜测字段结构强行实现』的交付**。」* Anti-scope 第 3 条同义：*「P1-4 不在探测结论落地前
抢先实现渲染层。」*

探测结论文档 `docs/r116-probe-findings.md` 第 3-6 行自己写着：*「本文档由代码侧准备完成，
三项判据的真机观测值待用户执行后回填……没有任何一条真机观测值」*。所以本轮的正确姿势是
**能力备好、接线留空、判定依据写清**，而不是把渲染层接上去向用户展示基于猜测的数据。

---

## 5. 待真机验证（本环境无法执行，不伪造 PASS）

### 5.1 操作步骤

1. **重新构建并打包**（用户平时的方式；构建命令 `go build -o <out> ./backend`，
   本轮版本号未动，仍是 0.12.9，收口时由主控升到 0.12.10）。
2. 客户端里开一局**海斗**（KIWI / ARAM_MAYHEM 任一子模式），完整走完
   **ChampSelect → GameStart/InProgress → 正常结束**。
   - 选人阶段：让应用停在实时页并**至少刷新一次**（协同提示 + 队伍画像应在此阶段就出现）；
   - 进游戏后：停在实时页，等 2999 接口探测成功。
3. 对局结束后导出诊断日志：**设置 → 诊断与日志 → 「导出诊断日志」**
   （`backend/web/index.html` 的 `#export-diagnostics`，走 `GET /api/diagnostics/log`）。
   建议归档到 `docs/r116-validation/mayhem-live-shapes.jsonl`（探测工单 §7 已约定路径）。
4. 至少重复 3 局。

### 5.2 要看哪些事件、期望量级

| # | 真机判据 | 看哪个事件的哪个字段 | 期望 |
|---|---|---|---|
| 一 | 克制/协同提示的实际出现频率（工单「真机验证清单」第 1 条 + 「通用验证要求」真机一项） | 应用内：详情 tab 顶部有没有 `roster-notice-strip`；日志侧：`gameplay_roster_insights` 的 `matchup_notices` / `synergy_notices` / `ally_count` / `enemy_count` / `matchup_phase_allowed` / `hero_json_cached` / `postmatch_cached` / `upstream_requests_added` | ≥3 局里，克制提示约 **1/3 的局**至少出 1 条、协同提示约 **1/6~1/5 的局**至少出 1 条（理论推算见 §5.3，**不是实测**）。`upstream_requests_added` 必须恒为 **0**；`hero_json_cached` 必须为 **true**（false 说明缓存键没对上，是 bug）；ChampSelect 阶段 `matchup_phase_allowed=false` 且 `enemy_count=0`、`enemy_count_before_phase_guard` 可以是 5 |
| 二 | P1-4 阶段一解析是否正确（工单 P1-4 阶段一验证判据 1） | `live_client_items_parsed` 的 `self_matched` / `self_item_count` / `self_item_ids` / `item_element_keys` / `item_elements_seen` / `item_elements_skipped` / `identities_with_items` / `player_count` | 事件必须出现；开局后 `self_item_count > 0` 且 `self_item_ids` 非空；`item_element_keys` 若是 `canUse,consumable,count,itemID,slot` 则与斗魂一致 → **判据二可以回填为「可行」**；`player_count` 应为 10；`item_elements_skipped` 应为 0（非 0 说明海斗给了新形状，把 `item_element_keys` 逐项抄进 `docs/r116-probe-findings.md` §3.4）；`self_matched=false` 而 `identities_with_items>0` 说明身份匹配没对上，是 bug |
| 三 | 队伍画像标签在真实阵容上的观感（工单 P1-5 验证判据的真机延伸） | 应用内：详情 tab「我方」标题右侧的 `team-portrait-tags`；tooltip 里的 `本队 N 位英雄进入统计` | N 应为 5（少于 4 时整块不出现）；均衡阵容不该出标签。**这一条工单没列进真机清单，是我加的观感核对，不是验收判据** |
| 四 | 下一步出装至少命中一次（工单「真机验证清单」第 2 条） | —— | **不适用**：阶段二未接线，UI 上不会出现「下一件推荐」。这一条要等 §4 的两个判据回填、接线完成后再验 |

### 5.3 理论覆盖率复算（**理论推算，非实测**）

超几何模型：从 N 个英雄里随机抽 n 个组成一侧阵容，本人英雄的显著名单有 K 条，
「至少命中 1 条」的概率 = `1 - C(N-K, n) / C(N, n)`。
用真实夹具 `backend/testdata/r116/hexdata-hero-157.json` 核对过：`weakAgainst` 7 条、
`strongAgainst` 7 条、**两者零重叠**（并集恰好 14 条），`teammateSynergies` 7 条。

| 项 | N | K | n | P(至少命中 1 条) | 评审给的数 |
|---|---|---|---|---|---|
| 克制（P1-3） | 173 | 14 | 5（敌方 5 人） | **34.8%** | 34.6% ✓ 量级一致 |
| 协同（P1-2） | 173 | 7 | **4**（4 个队友，本人不能是自己的队友） | **15.4%** | 18.8% ✗ 偏高 |
| 协同（按评审的 n=5 复算） | 173 | 7 | 5 | 18.9% | 18.8% —— **差异来源找到了** |

**发现（记给 R116-E 与总账）**：评审 §3.1 的 18.8% 是按 **n=5** 算的，也就是把本人也算进了
「抽 5 个」里。但 `teammateSynergies` 是「我与队友」的表，本人永远不可能命中自己
（后端 `gameplayRosterWithout` 显式剔掉了 self，前端 `allyChampionIds` 含 self 但后端会减掉），
所以真实的抽样数是 **n=4**，诚实的数字是 **≈15.4%**，不是 18.8%。
克制那一条 n=5（敌方 5 人）是对的，34.8% 与评审的 34.6% 属同一量级（差异来自英雄池取
172 还是 173）。

这不影响任何实现决策（两种口径都远低于「能撑一个总分」的门槛，Anti-scope 第 2 条照旧成立），
但**前端文案与后续工单引用覆盖率时应该用 15.4%**。本轮 UI 上没有印任何具体百分比，
只印了「只列出上游标记为显著的对位与配合，没有提示不代表阵容没有问题」，所以不存在
需要改的既有文案。

---

## 6. 三处对抗变异的实跑输出

### 变异一（工单 P1-2/P1-3 §对抗变异）：把交集判断写反

把 `backend/gameplay.go:4358` 的

```go
bundle.MatchupNotices = gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, enemyIDs)
```

改成用**队友** roster 去比 `opponentChampionId`（等价于工单说的「比较 teammateID 而不是
championId」）：

```go
bundle.MatchupNotices = gameplayMatchupNotices(detail.WeakAgainst, detail.StrongAgainst, teammateIDs)
```

实跑输出：

```
$ go test ./backend -run "TestGameplayRosterInsightsAddZeroHexdataRequests|\
TestGameplayRosterInsightsAllowEnemiesInProgress|\
TestGameplayMatchupNoticesSingleHitProducesExactlyOneNotice" -count=1
--- FAIL: TestGameplayRosterInsightsAddZeroHexdataRequests (0.62s)
    gameplay_r116d_test.go:451: matchup notices = []main.gameplayMatchupNotice{
      main.gameplayMatchupNotice{Direction:"weak", OpponentID:12, CounterDelta:0.030611,
      Evidence:"supported", ConfidenceLow:0.476615, ConfidenceHigh:0.484097}},
      want 2 (200 + 90 from weakAgainst)
--- FAIL: TestGameplayRosterInsightsAllowEnemiesInProgress (0.30s)
    gameplay_r116d_test.go:502: weak notices = []main.gameplayMatchupNotice{
      main.gameplayMatchupNotice{Direction:"weak", OpponentID:12, ...}}, want 2
FAIL
FAIL	lol-loot-assistant/backend	2.037s
```

→ **检测到了**。注意这里变异体不是「0 条」而是「1 条错的」（12 牛头恰好也在 weakAgainst 里），
比全零更隐蔽——正好说明只测「有没有输出」是不够的。为此另有一条**构造出真正全零匹配**的
常驻测试 `TestGameplayMatchupNoticesNeverMatchDegenerationIsDetected`：它把队友 roster 换成
与夹具 14 个对手 ID **完全不相交**的 `[157,4,64,222,412]`，断言变异体 = 0 条、生产实现 = 2 条，
并显式断言「两者数量不同」（否则两边都空就是假绿）。协同侧同一套变异（拿敌方 roster 去比
`teammateChampionId`）也断言了 0 条 vs 1 条。

**已还原**：`cp` 备份回写后 `shasum` 逐字核对一致（`2ab9f958c0f055fd4cec6b52595f408bc8755267`）。

### 变异二（工单 P1-4 §对抗变异）：抢先接线阶段二

在 `gameplayApplyRosterInsights` 末尾插一行 `_ = gameplayNextItemSuggestionFromTrios`
模拟「提前接线」：

```
$ go test ./backend -run "TestR116DStageTwoStaysUnwired" -count=1
--- FAIL: TestR116DStageTwoStaysUnwired (0.00s)
    gameplay_r116d_test.go:1028: gameplayNextItemSuggestionFromTrios appears 2 times in
    gameplay.go code, want exactly 1 (the definition, no caller). Wiring P1-4 stage two
    requires docs/r116-probe-findings.md §3.4/§5.3 to be filled in first.
FAIL
FAIL	lol-loot-assistant/backend	0.533s
```

→ **检测到了**。这条变异对应的工单要求是「若阶段二判定为不可行，只交付阶段一，不强行实现
阶段二」；本轮的处置就是**不接渲染层**，并用这条测试把「没接线」这个状态本身钉住，
防止后来人在探测结论回填之前偷偷接上。

**已还原**：`shasum` 核对一致，`grep -c MUTATION backend/gameplay.go` = 0。

### 变异三（工单 P1-5 §验证判据）：「不生成标签」退化成「生成一个空标签」

在 `gameplayTeamPortraitLabels` 的 `return labels` 前插入：

```go
if len(labels) == 0 { labels = append(labels, gameplayTeamPortraitLabel{Key: "frontline", Label: ""}) }
```

实跑输出：

```
$ go test ./backend -run "TestGameplayTeamPortraitBalancedCompProducesNoLabels|\
TestGameplayTeamPortraitFromRequiresEnoughResolvedHeroes|TestGameplayTeamPortraitEndToEnd" -count=1
--- FAIL: TestGameplayTeamPortraitBalancedCompProducesNoLabels (0.01s)
    gameplay_r116d_test.go:1088: a balanced comp produced labels:
      []main.gameplayTeamPortraitLabel{main.gameplayTeamPortraitLabel{Key:"frontline", Label:""}}
--- FAIL: TestGameplayTeamPortraitFromRequiresEnoughResolvedHeroes (0.01s)
    gameplay_r116d_test.go:1239: a balanced comp produced a portrait:
      &main.gameplayTeamPortrait{Labels:[...{Key:"frontline", Label:""}], RosterSize:5,
      ResolvedHeroes:5, HeroPoolSize:173, MeasurementTechnique:"队伍画像门槛是 173 位英雄..."}
--- FAIL: TestGameplayTeamPortraitEndToEnd (1.29s)
    gameplay_r116d_test.go:1269: a balanced comp produced a teamPortrait: &main.gameplayTeamPortrait{...}
FAIL
FAIL	lol-loot-assistant/backend	2.382s
```

→ **三条同时检测到**（纯函数层、组装层、HTTP 端到端层各一条）。前端侧还有一条常驻的
源码替换变异 `R116-D placeholder mutation is detected by the empty-payload assertions`：
把 `renderRosterNoticeBars` 的 `if (!bars.length) return "";` 换成返回一个「暂无克制数据」
占位小条，断言变异体确实产出了 `roster-notice` 节点、而生产实现在同一场景下一个节点都没有。

> 说明：工单里显式标题为「对抗变异」的只有两处（P1-2/P1-3 与 P1-4）。主控要求跑三处，
> 第三处按 P1-5 验证判据的「**不是生成一个空标签**」这半句执行——它本身就是一条
> 反退化要求，上面的变异三就是它的实跑。

**已还原**：`shasum` 核对一致。

---

## 7. 工单里的一处笔误（已按正确写法实现）

工单 P1-2/P1-3 的结构体示例写了：

```go
ConfidenceLow, ConfidenceHigh float64 `json:"confidenceLow,confidenceHigh"`
```

**Go 的 struct tag 不支持一个 tag 两个名字。** `encoding/json` 会把整个
`confidenceLow,confidenceHigh` 当成字段名（逗号在 tag 语法里是「名字,选项」的分隔符，
所以实际解析出的名字是 `confidenceLow`、选项是 `confidenceHigh`——两个字段会共用同一个
JSON 键 `confidenceLow`，其中一个被静默丢弃，`confidenceHigh` 永远不会出现在响应里）。

正确写法是两个独立字段各带自己的 tag，已按此实现（`backend/gameplay.go:3719-3736`）：

```go
type gameplayMatchupNotice struct {
	Direction      string  `json:"direction"` // "weak"（我被克）| "strong"（我克对面）
	OpponentID     int     `json:"opponentChampionId"`
	CounterDelta   float64 `json:"counterDelta"`
	Evidence       string  `json:"evidence"`
	ConfidenceLow  float64 `json:"confidenceLow"`
	ConfidenceHigh float64 `json:"confidenceHigh"`
}

type gameplaySynergyNotice struct {
	TeammateID     int     `json:"teammateChampionId"`
	SynergyDelta   float64 `json:"synergyDelta"`
	Evidence       string  `json:"evidence"`
	ConfidenceLow  float64 `json:"confidenceLow"`
	ConfidenceHigh float64 `json:"confidenceHigh"`
}
```

回归钉：`TestGameplayRosterNoticeJSONTagsEmitBothConfidenceBounds` 直接 marshal 后逐键核对
`confidenceLow` 与 `confidenceHigh` 都存在且取值正确，并断言响应里不出现
`confidenceLow,confidenceHigh` 这个字面串。

**顺带发现的一个上游语义坑（记给 R116-E）**：`confidenceLow`/`confidenceHigh` 括住的是
上游的 `adjustedWinRate`，而 `adjustedWinRate` 在 `weakAgainst` 里属于**对手**（克我方的一方）、
在 `strongAgainst` 里属于**我方**——`counter*` 恒为赢家侧、`target*` 恒为输家侧
（实测 `weakAgainst[0]`：`adjustedWinRate` 0.529662 + `targetAdjustedWinRate` 0.470338 = 1.0，
`expectedTargetWinRate` 0.506727 − `targetAdjustedWinRate` 0.470338 = `targetWinRateDrop` 0.03639
= `counterDelta`）。所以前端**没有**把这个区间标成「我的胜率区间」，只中性地写成
「上游置信区间 52.5%–53.5%」放在 tooltip 里，避免把对手的区间说成自己的。
工单只要求透传这三个字段，没要求解释语义，这个处理不超出工单范围。

---

## 8. P1-5 阈值口径声明（**本地启发式，非上游官方口径**）

`gameplayTeamPortraitThresholdRatio = 1.0`（`backend/gameplay.go:3983`）+ 三条判据
（`gameplay.go:4279-4287`）：

| 角色 | 判据 | 用的字段 |
|---|---|---|
| 前排（承伤） | `avgDamageTaken ≥ 全英雄均值 × 1.0` **且** `avgDmgMitigated ≥ 全英雄均值 × 1.0` | 2 项 |
| 控制（开团） | `avgCcTime ≥ 全英雄均值 × 1.0` | 1 项 |
| 持续输出 | `damageShare ≥ 全英雄均值 × 1.0` | 1 项 |

我方 5 人里「一个都没有」→ 产出对应的「缺 X」标签；三类都有人 → `nil`（一个标签都不产出）。

**声明（三点，都要转述给 R116-E 与总账）**：

1. **这套阈值是本地约定的启发式，不是 Hexdata 官方口径，也不是虎牙站或任何第三方站点的
   流派划分。** 工单要求「阈值定义参考虎牙站的流派划分思路（承伤与开团 / 治疗与增益 /
   暴击与攻速）」——按方案第 294 行「只借产品思路，不接数据」，代码里**没有一个外部数字**：
   比例 1.0 就是「算术均值」本身，均值由 `gameplayTeamPortraitAveragesFrom` 从 hexdata
   `/api/hexdata/postmatch` 的 173 英雄全量现算，口径与 R116-B P1-6 的
   `mayhemPerformancePanel` 完全一致（字段没出现过的英雄不进池，绝不拿 0 顶替缺失值）。
2. **不用绝对值门槛**：这 5 项里 `avgHealShield` 极差巨大（min 0.63 / mean 1595 /
   max 27543），任何写死的绝对值都会在上游改口径后立刻失效。「相对全英雄均值的比例」
   是唯一不需要魔法数字的写法。取均值而非中位数会让判据偏严（右偏分布下高于均值的英雄
   少于半数：实测 `avgDamageTaken` 67/173、`avgCcTime` 75/173、`damageShare` 107/173），
   因此更容易报「缺」——这是有意的，漏报会让用户以为阵容没有缺口。
   实测随机 5 人组合（173 英雄池，2 万次采样）：**85.2% 不产出任何标签**，
   9.8% 报「缺前排」、5.5% 报「缺控制」、0.6% 报「缺持续输出」。噪声水平可接受。
3. **工单列的 5 项指标里，`avgHealShield`（治疗与增益）本轮不参与任何判定。**
   三个标签里没有一个能给它一个可辩护的判据位置，硬塞进去就是编造口径（数据准确性红线）。
   它照常算进 `gameplayTeamPortraitAverages`、照常进 `gameplay_team_portrait` 相关的
   诊断路径，等真要加「缺治疗/护盾」标签时直接用。这一条是我对工单的**收窄**，
   在这里显式记录，不静默处理。
4. 这句声明对用户是**可见的**，不只写在代码注释里：`gameplayTeamPortrait.MeasurementTechnique`
   的文案明写「这是本地约定的启发式判据，不是上游官方口径」，前端通过
   `window.deepLegendsShared.renderMeasurementTechnique`（R116-B 的公共实现，不另写一份）
   渲染成 `aside.mayhem-measurement`，只在确实有标签时出现。
   `TestGameplayTeamPortraitFromRequiresEnoughResolvedHeroes` 断言了这句文案必须含
   「本地约定的启发式」与「不是上游官方口径」两个片段。

---

## 9. Anti-scope 第 4 条：ChampSelect 阶段放开克制的具体位置

本轮采取的是**基于「探测未证实」的保守分支**：`docs/r116-probe-findings.md` §4.4 的
`lcu_champ_select_session_shape.their_team_length` 观测值仍是「待填」，仓库里 `theirTeam`
的 fixture 无一例外是 `[]`（`gameplay_test.go` 6 处、`r68_test.go` 2 处），docs 里零真实观测。
所以 ChampSelect 阶段一律不展示「对面 5 人」相关的克制；协同（P1-2）本身只需要我方，不受影响。

真机证实 `their_team_length > 0` 且 `their_team_nonzero_counts.championId > 0` 之后，
**放开要改两处（缺一不可）**：

| # | 位置 | 改法 |
|---|---|---|
| 1 | `backend/gameplay.go:3999` `var gameplayRosterMatchupPhases = []string{"InProgress", "Reconnect"}` | 加上 `"ChampSelect"` |
| 2 | `backend/web/gameplay.js:4491` `const R116D_MATCHUP_PHASES = ["inprogress", "reconnect"];` | 加上 `"champselect"`（小写，前端比较用 `phase.toLowerCase()`） |

同时要改的测试（它们钉住的就是这条保守分支，放开时必须同步）：
- `backend/gameplay_r116d_test.go` 的 `TestGameplayRosterMatchupPhaseWhitelist`
  （断言 `champselect` **不在**白名单里）、`TestGameplayRosterInsightsSuppressEnemiesDuringChampSelect`
  （断言 ChampSelect 下 `matchup_notices == 0` 且 `enemy_count == 0`）；
- `backend/web/r116d.test.cjs` 的 `R116-D roster sends both sides in game and only allies
  during champion select`、`R116-D recommendation request carries the roster and refetches
  when it changes`（断言 ChampSelect 下不发 `enemyChampionIds`）。

`backend/gameplay.go:6618-6623` 那段 R95 沉淀的注释（「Arena squads therefore cannot be
recovered during champion select; … Do not retry.」）说的是**斗魂小队**，与海斗的 `theirTeam`
不是一回事，本轮**一字未改、也没有绕过**：P1-2/P1-3 在 ChampSelect 阶段只用我方 5 人，
与它的结论一致。

---

## 10. 并行会话冲突防护（实际做了什么）

1. 开工前把 `backend/gameplay.go` / `backend/web/gameplay.js` / `backend/web/gameplay.css`
   `cp` 到 `$PI_SCRATCH_DIR/r116d-backup/`；此后每写完一个文件立刻增量备份
   （`gameplay_r116d_test.go` / `r116d.test.cjs` 也在写完后立即备份）。
2. 三处对抗变异全部走「改 → 跑 → 从备份 `cp` 还原 → `shasum` 逐字核对」的流程，
   没有用 `git checkout`（工作树里有 R116-F 的未提交改动，`git checkout` 会毁掉它）。
3. 每次改动后 `grep` 自查关键符号：`aramMode && !arenaMode`（R116-探测的埋点，
   现在在 `gameplay.go:7035`）、`recommendation-measurement`（R116-B 的常驻页脚，
   现在在 `gameplay.js:5236`）——两者都还在。
4. **没有回滚或"修正"任何不属于 R116-D 的改动。** 本轮实际观测到 R116-F 在并行推进
   （前端测试基线从我开工时的 577 涨到 588），我没有碰它改的任何文件。
5. **收尾 `shasum` 逐个核对（live vs 备份，全部一致）**：

```
MATCH  backend/gameplay.go               2ab9f958c0f055fd4cec6b52595f408bc8755267
MATCH  backend/gameplay_r116d_test.go    9dcdc81c04c5a7734441e2d4c1588a6906643456
MATCH  backend/web/gameplay.js           7076cabf73874e1001d5f037e6cef7b138f71c39
MATCH  backend/web/gameplay.css          00bf26f46d43c69e95741f2cd5247f4006f2fc2a
MATCH  backend/web/r116d.test.cjs        cdb7a54372254682c6894780f26bdd51ed18466d
```

（中途 `backend/web/gameplay.js` 曾出现 backup ≠ live，`diff` 核对后确认是我自己后两次编辑
比备份新——把 `is-${tone}` 改成类名字面量、把画像标签的 `is-<key>` 改成 `data-portrait-gap`
属性，都是为了不抬高 R117 样式棘轮的「未引用类名」预算。已重新备份并复核一致。）

### R117 样式棘轮的实际读数（改前 → 改后）

`r117-style.test.cjs` 的六个预算改前**全部顶格**，所以新样式必须零增量：

| 预算 | 上限 | 改前 | 改后 |
|---|---|---|---|
| 未引用类名 | 93 | 92 | **90**（`is-weak`/`is-strong` 现在在 gameplay.js 里以字面量出现，反而降了 2） |
| `border-radius` 去重取值 | 33 | 33 | **33**（只用既有的 7px / 999px） |
| `padding` 去重取值 | 205 | 205 | **205**（只用既有的 `3px 5px` / `1px 5px`） |
| `gap` 去重取值 | 54 | 54 | **54**（只用既有的 4px / 6px） |
| 重复定义的 hex | 35 | 35 | **35**（新样式零 hex） |
| 重复声明组 | 45 | 45 | **45** |

`r117-style.test.cjs` 与 `r117.test.cjs` 的既有断言**一字未改**，全绿。

---

## 11. 留下的未解决问题

1. **P1-4 阶段二未接线**，阻塞在真机探测（§4）。这是本轮唯一「工单要求但没交付用户可见
   功能」的项，依据是工单自己的「对抗变异」条款，不是遗漏。
2. **「未成型的空槽」过滤做不到。** 工单 P1-4 阶段二第 3 条要求「滤掉消耗品/未成型的空槽」。
   消耗品能滤（`consumable=true`），空槽能滤（`itemID<=0`），但**合成组件滤不掉**：
   `/liveclientdata/playerlist` 的 `items[]` 元素里没有任何字段能区分「成品件」与「合成组件」
   （`canUse` 是「能不能主动使用」，与是否成型无关）。要真滤组件需要一份成品件 ID 目录，
   而那不在本轮允许改动的文件里（`champions.go` / `hexdata.go` 都是禁触）。
   **后果**：如果组件也进 `items[]`，前缀匹配会假命中（例如已出「吸血权杖」+「十字镐」
   被当成某 trio 的前两件）。接线前必须用真机日志核对 `self_item_ids` 里到底有没有组件 ID。
   已在 `gameplayOwnedTerminalItemIDs` 的注释里写明（`gameplay.go:5704-5711`）。
3. **阶段二接线可能要改 `backend/main.go`。** `gameplayApplyRosterInsights` 跑在推荐 HTTP
   处理器里，拿不到 live client 快照（快照在实时页处理器里）。接线时要么把本人 items 通过
   查询参数传进来（与 roster 同一套做法，不用改 main.go），要么在 `app` 上加一个快照读取口
   （要改 main.go）。本轮没做这个决定，留给接线那一轮 + 主控批准。
4. **`live_client_items_parsed` 复用了既有的诊断去重 map。** 去重键前缀是 `items:`
   （`recordLiveClientPlayerListShape` 用的是 `game:`），共用
   `a.liveClientPlayerListDiagnosticMu` / `a.liveClientPlayerListDiagnosticKeys`
   （上限 `lcuSessionShapeDiagnosticLimit=128`，超出整体重置）。
   **原因**：给 `app` 结构体加新字段要改 `backend/main.go`，不在本工单允许范围内。
   **代价**：两个事件共享一个 128 键的预算，一局长对局里出装变化多时可能互相挤掉。
   真机验证时如果发现 `live_client_playerlist_shape` 少了，就是这个原因；
   届时应该给 items 单独开一个 map（需要主控批准改 main.go）。
5. **协同覆盖率的诚实数字是 ≈15.4%，不是评审写的 18.8%**（§5.3）。差异来源已定位
   （评审按 n=5 算，把本人也算进了抽样）。后续工单引用时应该用 15.4%。
6. **阵容变化会触发推荐接口重取。** 选人阶段英雄逐个锁定时，阵容签名变化会让
   `ensureLiveRecommendations` 重新请求一次（一局大约多 3~5 次）。每次请求的 hexdata 部分
   都是缓存命中（判据 3 已钉住「上游请求数不变」），多出来的只是本机 HTTP 往返，
   且既有的 in-flight 去重与 60 秒退避仍然生效。**没有实测这 3~5 次在真机上的观感**
   （会不会让详情 tab 闪一下），留给真机验证时一并观察。
   之所以必须重取：推荐缓存 key 的格式被 `champions.test.cjs:2470/2478` 钉死
   （`championId:position:gameMode:mapId:tier:spellKey`，不含阵容），而那份文件正被
   R116-F 并行修改，本轮不去碰它，所以用 `state.liveRecommendationRosters` 旁表实现失效。
7. **两个 `typeof` / 嵌套 helper 的妥协。** `renderLiveInsights` 的三个渲染 helper 定义在
   函数体内（而不是模块作用域）、`ensureLiveRecommendations` 里用
   `typeof liveRecommendationRoster === "function"` 护栏——都是因为
   `champions.test.cjs` 用 `compileFunctions` 单独编译这两个函数、注入的依赖列表是固定的，
   加新的自由变量会让它抛 `ReferenceError`，而那份文件正被 R116-F 并行修改。
   **技术债**：R116-F 收尾后，应该把这两个 helper 提到模块作用域，并在
   `champions.test.cjs` 的 `compileFunctions` 自动注入列表里补上（仓库已有这个模式，
   见它对 `insightTeamLayout` 的处理）。本轮不做，避免与并行会话互相覆盖。
8. **`avgHealShield` 本轮不参与判定**（§8 第 3 点），是对工单「取 5 项」的收窄，已显式记录。

---

## 12. 新增的诊断事件（排障用）

| 事件 | 位置 | 关键字段 |
|---|---|---|
| `gameplay_roster_insights` | `backend/gameplay.go:4376-4391`（每次推荐请求一条，不去重） | `trace_id` / `recommendation_key` / `champion_id` / `internal_mode` / `phase` / `ally_count` / `teammate_count` / `enemy_count` / `enemy_count_before_phase_guard` / `matchup_phase_allowed` / `hero_json_cached` / `postmatch_cached` / `weak_against_rows` / `strong_against_rows` / `synergy_rows` / `matchup_notices` / `synergy_notices` / `team_portrait_labels` / `team_portrait_hero_pool` / `upstream_requests_added`（恒 0） |
| `live_client_items_parsed` | `backend/gameplay.go:5423-5459`（按 `items:<gameID>:<items 指纹>` 去重，指纹 = 件数 + 升序 itemID 串） | `game_id` / `phase` / `ui_visible`（恒 false）/ `self_matched` / `identity_source` / `self_item_count` / `self_item_ids`（升序）/ `item_element_keys` / `item_elements_seen` / `item_elements_skipped` / `identities_with_items` / `player_count` |

两个事件都不在 `isNoisyDiagnosticEvent`（`backend/main.go:1573`）的名单里，会正常落盘。
`live_client_playerlist_shape` 的字段**一字未改**——`docs/r116-probe-findings.md` §3.1
描述的就是它，改了会让那份文档的判读表失效；items 的元素键名放在新事件里而不是塞进
`element_keys`（后者按探测文档的定义只记录玩家对象的**顶层**键）。
