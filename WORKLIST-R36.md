# WORKLIST-R36（给 GPT 执行）

> 本轮基于用户 0828 22:55 的真机日志（`lol-loot-diagnostics-0828-2255.jsonl`，880 行 / 492KB，
> 时间跨度 **14:54:55 ~ 14:55:10 UTC，仅 15 秒**）+ 11 张真机截图。
> **A 组是总览性能与数据正确性的真根因，是本轮最重要的部分，请优先做完再动 UI。**
> 用户原话：「这个总览页面最重要的是快，不行参考 LeagueAkari 重构，不知道你在改什么」。

---

## A 组（P0）：总览慢 + 国服其他人没有负场/胜率 —— 日志已坐实三条独立根因

### A-0 先看这份日志说明了什么

15 秒的日志里只有 6 种事件，**全部是排位查询**：

```
  98 条  lcu_ranked_stats_shape      平均 1,971B   ← 占日志 39.3%
  97 条  sgp_ranked_stats_shape      平均 1,683B   ← 占日志 33.2%
 392 条  ranked_winrate_resolved
  98 条  ranked_data_source_decision
  97 条  sgp_request
  98 条  sgp_ranked_stats_incomplete
```

**15 秒内发起了 98 次玩家排位查询、195 次 HTTP 请求。**
日志里**没有任何** `overview_load_cost`、`season_*`、`qq101_*`、`specialist_runes_*` 事件——
不是没发生，是被上面那两个 shape 事件挤掉了（见 A-1）。

### A-1 真根因一：两个 shape 诊断事件把日志冲爆了，导致每次都拿不到真正想看的证据

`gameplay.go:2000` 的 `lcu_ranked_stats_shape` 和对应的 `sgp_ranked_stats_shape` 是**每次请求都无条件**
记录的，每条 1.7~2KB（含 `queue_keys` 全量键名 + `season_fields` 全量取值）。
**它们占了整份日志的 72.5%。** 按这个速率，`storage.go` 的 2MB 轮转阈值大约**一分钟就会触发一次**，
把绝活哥、总览耗时、qq101 探测等所有其他证据全部冲掉——
这就是为什么连续几轮都「拿到日志却看不到想看的事件」。

这两个埋点当初是为了回答「国服 LCU 到底带不带上赛季段位字段」，
**这个问题在 R33-F2 已经回答完并落地了**（`lcuRankedEntry` 六个历史字段已补全并渲染），
它们的使命已经结束。

**改法**：
1. 两个 shape 事件改成**每个进程只记一次**（用 `sync.Once` 或一个 `atomic.Bool`），
   并把 `season_fields` 整块删掉（那是最占地方的部分，问题已经回答完了）。
2. 顺便全仓库扫一遍还有没有别的「每次请求都 dump 完整结构」的诊断埋点，同样处理。
3. `storage.go` 的 `appendDiagnostic`：轮转时在新文件第一行补写一条
   `{"event":"log_rotated","previous_lines":N,"previous_bytes":M}`，
   这样以后拿到日志一眼就知道前面被截掉了，不用再靠猜。

### A-2 真根因二：`losses` 在 392 条记录里**全部**是 0，两个数据源都一样

```
losses 取值分布: {0: 392}          ← 392 条全是 0，没有例外
wins 有真实值:  27 / 30 / 101 / 134 / 530 …
complete:true 的只有 40 条，全都是 wins==0 且 tier_empty==true 的未定级玩家
```

而 `lcu_ranked_stats_shape` 的 `queue_keys` 里**明明有 `losses` 这个键**
（`["...","leaguePoints","losses","miniSeriesProgress",...]`），SGP 那份也有。
98 个不同玩家、每人 2 个队列，**胜场各不相同、负场清一色为 0** ——
这在统计上不可能是真实数据。

**要查什么（不要急着改，先定位）**：
- 现有的 shape 埋点只记录了**键名**，没记录**取值**，所以无法区分
  「上游真的回 0」和「我们解析成了 0」。**A-1 改造那条埋点时，顺便加上
  `sample_queue_values`：把第一个 `RANKED_SOLO_5x5` 条目的
  `wins`/`losses`/`tier`/`division` 四个字段的原始值原样记下来**（只记一次，不涉隐私）。
  这一条就能一锤定音。
- 同时检查 `rankedQueueEntries(raw)` 取的是不是 `queues` 数组：
  日志里 `top_level_keys` 显示顶层有 `queueMap`、`queues`、`highestRankedEntry`、
  `highestRankedEntrySR` 四个可能装队列的地方。**如果国服把真实胜负场放在 `queueMap` 里、
  而 `queues` 里是脱敏后的副本，就完全解释得通「键在、值恒为 0」**。
  这是目前最可疑的方向，请优先验证。

### A-3 真根因三：F-3 改成 LCU 优先后，**每次查询都变成打两次**，而且最终还是拿不到胜率

98 次决策，**100% 是同一个结果**：

```
reason: "local-client-connected"      → 首选 LCU
attempts: [ {lcu, success, "胜负场未完整验证"}, {sgp, success} ]
fallback_reason: "lcu-unverified-win-loss"
selected: "lcu"
```

也就是说：LCU 拿到了数据但 `losses=0` 判为未验证 → 转去打 SGP → SGP 同样 `losses=0`
→ `loadRanksWithFallback` 里 `if lcuRanks != nil && ranksHaveUnverifiedWinRate(ranks) { continue }`
→ 最终回落用 LCU 那份。**两次请求，零收益。**

F-3 的设计本身没错（"LCU 不完整时用 SGP 补全"），错在**现实中 SGP 也永远补不全**，
于是这条补全路径变成了 100% 触发的纯开销。

**改法**：
1. **加一个"本次会话内 SGP 补全成功率"的自适应闸门**：记录最近 N 次
   （建议 N=20）SGP 补全尝试里有几次真的把 `losses` 补上了；
   连续失败超过阈值（建议 10 次）就**在本次会话内停止再为"LCU 未验证"这个理由去打 SGP**
   （LCU 请求失败、跨服等其他理由仍然要走 SGP，不要一刀切关掉）。
   闸门状态要能从诊断日志看到：`{"event":"sgp_completion_gate","state":"open|closed","recent_success":X,"recent_attempts":Y}`。
2. 这条能立刻把总览的排位请求量**减半**（195 → 98）。

### A-4 ★「其他人没有胜率」的直接原因：G-5 把赛季扫描改成异步后，胜率兜底的数据源变空了

`gameplay.go:883` + `:896`：

```go
seasonStats, seasonProgress, seasonRankedSamples, seasonByQueue := a.loadSeasonChampionStatsSnapshot(...)
...
ranks, rankCapability = a.applySeasonRankWinRateFallback(ranks, rankCapability, seasonByQueue)
```

`loadSeasonChampionStatsSnapshot` 是 **network-free、只读本地磁盘缓存** 的
（R34 G-5 特意改成这样，是对的）。但是：**第一次查看别的玩家时，磁盘上根本没有他的赛季缓存**，
所以 `seasonByQueue` 是空的 → `applySeasonRankWinRateFallback` 无从填充 →
胜率永远停在「负场未提供 / 胜率暂不可用」（图二）。

**这就是用户说的「之前不是好的吗」的准确答案**：R33-D 组之前用的是全队列合并总量兜底
（数字是错的，两个队列显示一样，正是用户当初投诉的那个 bug），
R33-D 改成按队列取、R34-G5 又把数据源改成异步——**两个改动各自都对，合在一起
就变成「兜底源在需要它的那一刻恒为空」**。

**改法（三选一，建议第 1 种）**：
1. **异步扫描完成后，把胜率兜底重新算一遍并推给前端**。现在
   `startSeasonStatsRefresh` 完成后只推了 `season-progress` 事件，前端重新拉 overview 时
   磁盘缓存已经有了、兜底自然会生效——**请先确认这条链路是不是真的能让数字最终出现**
   （写一个测试：首次查询 → 断言胜率不可用；模拟扫描完成 → 再查一次 → 断言胜率出来了）。
   如果确实能出现，那问题就只是**首屏那几秒的空窗期没有正确的提示**，
   文案应该是「正在统计中」而不是「负场未提供」。
2. 如果扫描本身对别人不生效（`startSeasonStatsRefresh` 有 `client == nil` 等前置条件，
   请核实查别人时是否满足），那要先修这个。
3. 兜底彻底拿不到时，文案必须说清楚是「上游未提供负场，无法计算胜率」，
   不能让用户以为是我们没做。

### A-5 参考 LeagueAkari：用户问"他是怎么快速拿到国服其他人胜负场的"

**必须先说清楚一件事**：R33-F 组那轮我已经拉过 LeagueAkari 源码逐行读过，
它拿排位数据用的就是 `/lol-ranked/v1/ranked-stats/{puuid}` 这一个本机 LCU 端点，
**跟我们现在 LCU 优先那条路是同一个端点**（见 [[deep-legends-leagueakari-ranked-data-source]]）。
所以「他有个我们不知道的快接口」这个假设**已经被排除了**。

真正的差别只可能在两处，**请按顺序验证，不要跳过直接重构**：
1. **他解析的是响应里的哪个字段**——见 A-2 那条 `queueMap` vs `queues` 的怀疑。
   拉一份 LeagueAkari 的 `RankedStats` 类型定义对照我们的解析路径（源码在
   `src/shared/types/league-client/ranked.ts`），看它读的是不是同一个数组。
   **这是最可能的答案，成本也最低。**
2. **他压根不做"平均段位"这个功能**——我们 98 次查询里绝大多数是为了给战绩卡算平均段位，
   他的对局内面板只查当前这一局的 10 个人。如果 A-2/A-3 修完请求量还是下不来，
   要考虑把「战绩列表里每一场都算平均段位」改成**只在用户展开那一场时才算**（懒加载），
   这跟 G 组"流式渲染"是同一个思路。

**在 A-2 定位清楚之前，不要做任何大重构。** 用户说的"不行参考 LeagueAkari 重构"是结果导向的
表达，真正要的是"快"和"数据对"，而不是重写一遍。

---

## B 组（P0）：绝活哥符文在召唤师峡谷自定义局里仍然不出现 —— 根因已定位

图六：界面同时显示了两句自相矛盾的话——
标题「当前队列没有可用的绝活哥榜单」，副标题却是
「绝活哥榜单仅支持单双排、灵活组排**和召唤师峡谷自定义对局**」，而用户当前正是自定义局。

`gameplay.go:2844`：
```go
func recommendationModeHasTopPlayers(resolution gameplayRecommendationModeResolution) bool {
	if resolution.InternalMode != "ranked" { return false }
	return resolution.QueueID == 0 || resolution.QueueID == 420 || resolution.QueueID == 440
}
```

而 `resolveGameplayRecommendationMode`（`:2889`）里，自定义局是靠
`case mode == "CLASSIC" && mapID == 11:` 这条**兜底规则**解析成 `ranked` 的，
**这条分支根本不看 QueueID**。图六里 OP.GG 那一栏数据完全正常（优劣势对抗都出来了），
证明模式确实解析成了 `ranked`；那么唯一能让 `hasTopPlayers` 为 false 的就是
**`QueueID` 既不是 0 也不是 420/440**。

**自定义对局 LCU 上报的 `queueId` 是 `-1`，不是 0**——这是 Riot 客户端对 CUSTOM 队列的标准取值。
前端兜底 `web/gameplay.js:2934` 写的也是 `queueId === 0`，同样漏掉 `-1`。

**改法**：
1. `recommendationModeHasTopPlayers` 改成：`InternalMode == "ranked"` 且
   （`QueueID == 420 || QueueID == 440 || QueueID <= 0`）。用 `<= 0` 同时覆盖
   自定义局的 `-1` 和某些接口返回的 `0`，并写注释说明为什么。
2. 前端 `recommendationQueueHasTopPlayers` 的兜底同步改成 `queueId <= 0`。
3. **加诊断埋点**：`{"event":"recommendation_mode_resolved","queue_id":N,"game_mode":"...","map_id":N,"internal_mode":"...","has_top_players":bool}`，
   每局只记一次。这样下次如果还不出现，一眼就能看到 queueId 到底是几——
   **这轮的教训就是我们猜了三轮 queueId，却从来没把它记进日志**。
4. 改完后 `-1` 这个取值要有测试：`resolveGameplayRecommendationMode(-1,"CLASSIC",11)` →
   `InternalMode=="ranked"`，`recommendationModeHasTopPlayers(...)==true`。

**变异测试**：把 `QueueID <= 0` 改回 `QueueID == 0`，上面那个 `-1` 的测试必须 failed。

---

## C 组（P1）：近 20 场排位 / 能力表现 / 位置偏好 不该去扫整个赛季

图一：「近 20 场排位」上面挂着「统计中 · 已统计 2…」，
「能力表现」挂着「统计中 · 已统计 560 场」。用户原话：
「近20场排位最多统计20场不就行了吗，为什么要统计那么多，能统计多少场统计多少，最多20场就行，
下面的能力表现和位置偏好也是这样，不需要统计那么多啊，搞得太复杂了」。

**用户是对的，这三个模块的口径本来就只需要最近 N 场。** 现在它们依赖整季扫描
（`seasonScanForegroundPages=2` 前台 + `seasonScanBackgroundPages=12` 后台反复回补），
既慢又要挂"统计中"角标，复杂度全是自找的。

**改法**：
1. 「近 20 场排位」「位置偏好」：**只用首屏已经拿到的战绩列表**里该队列最近 20 场算，
   够几场算几场，不足 20 场就按实际场数算并在标题旁标明「近 N 场」。
   **不再读赛季快照、不再显示"统计中"角标。**
2. 「能力表现」：同样改成基于最近 20 场（与"近 20 场排位"共用同一批样本，
   避免两个模块口径不一致），标题保持「能力表现」，副标题标明样本区间。
3. **赛季扫描不要删**——英雄统计（`SeasonChampionStats`）那块仍然需要整季数据，
   保留后台异步扫描和它自己的进度提示即可。只是把上面三个模块跟它解耦。
4. A-4 那条胜率兜底如果确实依赖赛季聚合，**要单独保留这条依赖**，
   不要因为本条改动把兜底也一起砍掉——这两件事要分开处理。

**变异测试**：构造一个"赛季扫描完全不可用"的桩，断言近 20 场排位/位置偏好/能力表现
**照常显示**（用首屏战绩算出来的结果），且**没有**"统计中"角标。

---

## D 组（P1）：切换战绩筛选后应自动继续查找，不要"继续查找"按钮

图三：切到「灵活组排」显示"没有符合条件的对局"，底部一行小字
「当前筛选结果较少；需要更早对局时请继续查找」+ 一个「继续查找」按钮。
用户要求：**切换模式时自动去找，一直找到本赛季找完为止，不要这个按钮。**

代码在 `web/gameplay.js:2420 updateMatchFilter`：切筛选时如果可见对局数
少于 `matchCount/2` 就设 `autoPaused: true` + 那句 pauseReason。

**改法**：
1. 切换筛选后**自动继续翻页**，直到满足下面任一条件才停：
   ①该筛选下已凑够 `state.settings.matchCount` 场；②已经翻到本赛季边界/上游没有更多；
   ③连续翻页失败（保留现有的 `paginationStalls` 熔断，**这个不能删**，
   [[deep-legends-pagination-runaway]] 记录过翻页失控下载 252MB 被 503 的事故）。
2. 翻页过程中显示一个**不可点击的进度提示**（如「正在查找更早的灵活组排对局…第 N 页」），
   而不是让用户自己点按钮。
3. 只有在熔断触发（条件③）时才回退成现在这种「手动继续查找」的形态，
   并把原因说清楚（例如「上游连续无响应，已暂停自动查找」）。
4. **`matchCount` 上限与熔断阈值保持现状不变**，这条改的是"谁来触发翻页"，
   不是"翻多少页"。

**变异测试**：构造一个"该筛选下前 3 页都没有匹配对局、第 4 页有"的桩，
断言切换筛选后**无需任何点击**就能拿到第 4 页的对局；把自动续翻改回 `autoPaused=true`，测试必须 failed。

---

## E 组（P2）：英雄详情页分路按钮的位置与对齐

图四：分路按钮（打野/中单/辅助三个）现在**独占一整行、横跨整个卡片**。
用户要求：**放到右侧那四格胜率指标的下面，并且"胜率那行与位置按钮在垂直方向上居中"**，
不要占满一整行。

现状 `web/champions.css:426`：
```css
.champion-detail-positions { grid-column: 1/-1; grid-row: 3; }   /* ← 横跨全部列 */
```
右侧指标区是 `.champion-detail-side { grid-column: 3; grid-row: 1/3; }`。

**改法**：把 `.champion-detail-positions` 移进右侧那一列
（`grid-column: 3`），跟 `.champion-detail-metrics` 上下排布，
两者作为一组在英雄立绘区域内**垂直居中**（`.champion-detail-side` 已经是
`align-content: center`，把 positions 放进这个 side 容器里即可）。
按钮宽度按该列宽度自适应，`data-count` 那几条 `grid-template-columns` 相应调小
（三个按钮一行放不下时允许换成两行，但**不要**再回到横跨全宽）。
窄屏断点（`:681` / `:701` / `:723` 那几处）要一并调整并实测不截断。

**验证**：用项目已有的 Playwright 截图工具（[[deep-legends-visual-harness-playwright]]）
在宽屏和窄屏各截一张，确认按钮在右侧、与指标行整体垂直居中、文字不截断。

---

## F 组（P2）：总览页签增强 —— 加"添加到总览"按钮 + 支持拖拽排序

用户描述：英雄详情页里点「场次最高玩家」的名字会进入该玩家总览（图六那种浮层），
但**这个浮层退出后就没了，不能作为一个常驻页签继续查看**。

**改法**：
1. 在玩家浮层顶部那一行（现在只有左侧「← 返回」按钮，`web/gameplay.js:701` 一带的
   `player-overlay` 结构）的**最右侧**加一个按钮，例如「+ 添加到总览」。
   点击后把当前这个玩家作为一个新页签加进 `state.tabs`（复用现有的开页签逻辑），
   并给出反馈（toast 或按钮变成"已添加"）。已经在页签里的玩家，按钮应显示为禁用/已添加态。
2. **总览页签支持拖拽调整顺序**：`state.tabs` 数组重排 + 持久化到设置里，
   使得下次启动仍保持用户调整过的顺序。注意 `web/gameplay.js:266-281` 那段
   现在会把 `current`（自己）强制排到第一个，**拖拽排序要尊重用户的选择，
   但"自己"这个页签建议仍固定在首位不可拖动**（避免误关/误排），
   这一点如果与用户预期不符可以再调整。
3. 拖拽实现请用原生 HTML5 drag-and-drop 或指针事件，**不要引入新依赖**；
   键盘可达性上至少保证能用左右方向键移动焦点页签的顺序（跟现有下拉框增强层一样，
   不能只做鼠标）。

---

## G 组（P2）：三处纯装饰性文字删除 + 下拉框选项边框 + 紫色宝箱

### G-1 下拉框选项有多余边框（图七）

`web/app.css:465` 的 `.select-menu-popover button` **没有 `border: 0`**，
而同类实现 `.app-select-menu button`（`:454`）是有的——所以收藏页排序那个下拉框
每个选项都带了一圈边框。

**改法**：给 `.select-menu-popover button` 补上 `border: 0`，与 `.app-select-menu button` 对齐。
**并且全项目扫一遍**还有没有别的弹层按钮漏了这条（搜 `popover button`/`menu button` 相关规则），
用户明确要求"再检查其他地方有没有这种问题"。

### G-2 删除三处文字（图八 / 图九 / 图十）

1. **图八**：收藏页那条常驻绿色横幅「✓ 收藏与奖池核对结果有效」——
   `web/app.js:539`。**整条横幅删掉**（不是改文案）。R33-E5 那轮只改了措辞，
   用户现在要求直接不要了。删的时候注意 `renderNotice()` 里错误/警告态的分支要保留，
   只删这个"一切正常"的成功态横幅。
2. **图九**：`web/index.html:184` 里 `账户与物品` 上方的 `<p class="eyebrow">客户端仓库</p>`
   和下方的 `<p>按类别展示客户端物品。</p>` 两行删掉，保留 `<h2>账户与物品</h2>`
   和右侧的 `#account-live-state` 状态芯片。
3. **图十**：`web/index.html:187` 里 `<p class="eyebrow">三合一奖池</p>` 删掉，
   保留 `<h2>奖池皮肤</h2>`。

删完检查 `.panel-heading` 在只剩一个 `<h2>` 时的上下间距是否还协调，必要时调 CSS。

### G-3 紫色宝箱：改名 + 去掉背景 + 尺寸对齐（图十一）

用户要求：叫「**紫色宝箱**」，**背景色去掉**（和其他物品一样只展示本体），**大小和其他物品一致**。

实测证据（我直接读了 PNG 文件头）：
```
promotion-chest.png           303 x 303   colortype 2 = RGB，★没有透明通道
hextech-chest-transparent.png 512 x 512   colortype 6 = RGBA，有透明通道
```
**根因很明确：这张图是不带 alpha 通道的 RGB 图，深色背景是烤进图片里的**，
所以它在卡片里显示成一个带底色的方块，而旁边的宝箱是透明的。尺寸也小了一圈（303 vs 512）。

**改法**：
1. `lcu_api.go:70` 的中文名从「高级宝箱」改成「**紫色宝箱**」。
2. **换一张带透明通道的图**：优先从 CDragon 的战利品图标目录重新取
   （参考 [[deep-legends-augment-icon-missing-large.md]] 里记的 CDragon 取图注意事项：
   它拒绝默认 urllib UA、不支持 HEAD）。取回来后**必须核对 colortype 是 6（RGBA）**，
   尺寸统一到 512×512 与其他宝箱一致。
   如果实在找不到官方透明图，再考虑对现有图做去背（背景是纯色时可以直接键出），
   **但要先试官方源**。
3. 换完后在真机截图里跟旁边的「战利品宝箱」并排对比，确认无底色、大小一致。

**测试**：补一个断言 `lootChineseNames["CHEST_PROMOTION"] == "紫色宝箱"`；
图片资源加一个构建期检查——`web/loot-icons/*.png` 全部必须是 RGBA（colortype 6），
防止以后再混进不透明素材。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- **A 组请按 A-1 → A-2 → A-3/A-4 的顺序做**：先让日志能用（A-1），
  再用新埋点定位 `losses` 到底断在哪（A-2），**拿到答案后再改 A-3/A-4**。
  A-2 没定位清楚之前不要改解析逻辑，也不要做大重构。
- B 组、C 组、D 组各写至少一个真变异测试（改回旧行为 → 测试必须 failed → 复原变绿），
  变异结果贴在提交说明里。
- E 组请附宽屏 + 窄屏两张截图；G-3 请附紫色宝箱与战利品宝箱并排的截图。
- 改完后请用户**重启应用**再复现一次（尤其是自定义局的绝活哥符文），
  从「设置 → 隐私与能力 → 导出诊断日志」导出，**上传前把文件重命名一下**
  （避免传输层按文件名缓存导致又拿到旧的那份）。
