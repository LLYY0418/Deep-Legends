# R27 工单：优劣势对抗根因修复 / 队列切换解耦 / 赛段探测

日期 2026-08-27。file:line 以当前工作树为准。
**标「实测」的都是本轮真跑过 curl / 读码 / 日志核对的**，标「待验证」的必须先验证再落地。

---

## 0. 先说三件关于「时间线」的事（很重要）

### 0.1 这次上传的日志是旧的，证明不了 lolalytics 修没修

```
上传的 diagnostics.jsonl  sha256 = 3d1dacdae13d180a...  1831 行
时间范围 2026-08-26 13:24:55 ~ 13:48:57
```

**与上一轮上传的是同一个文件（sha256 逐字节相同）。** 而 GPT 的代码改动时间是：

```
lolalytics.go        2026-08-27 00:28
specialist_runes.go  2026-08-27 01:32
champions.go         2026-08-27 01:53
web/gameplay.js      2026-08-27 02:19
```

**日志比代码改动早了约 11 小时**，它记录的是改动之前的运行状态。
所以「lolalytics 是不是打通了」这个问题，**这份日志给不出答案**。

### 0.2 代码层面 lolalytics 没有任何机制改动

`champions.go:31` 主机仍是 `a1.lolalytics.com`，`:609-619` 仍是同一套
浏览器 UA + Origin + Referer + Sec-Fetch 伪装。`lolalytics.go` 里没有新增代理、
备用主机或任何绕过手段（全文只有 `:210` 和 `:242` 两处 `fetchDirect`，都指向同一个 host）。

**没有任何改动能解释 403 变成 200。** 如果你在界面上确实看到出装数据出来了，
更可能的解释是网络环境变了（换了网络/开了代理），而不是代码修好了。

### 0.3 我的 R26 工单还没执行

`ACCEPTANCE-R25-WORKLIST-R26.md` 的写入时间是 **2026-08-27 09:16**，
在 GPT 那批改动（00:28~02:19）**之后**。所以上一轮定的「移除 lolalytics、改用 op.gg + 去重」
以及三个 P0 都还没做。**我核对过 `specialistRuneKey` 仍是 `championID*10+ordinal`，确实没改。**

**请重新导出一份最新日志**（用一下英雄详情页和对局页再导），我才能判断 lolalytics 的真实状态。

---

## A. 【P0】优劣势对抗数据变少 —— 根因已定位，用错了数组

### A.1 现象与你的判断

你说「总感觉比之前少了」——**属实，而且我找到了确切原因**。
图一里永恩上路只显示 1 个优势 + 2 个劣势，一共 3 条。

日志实证（`counters_shape` 的 `in` 字段 = 上游给了多少条）：

```
in=1  in=2  in=26  in=1  in=1  in=2  in=0  in=2  in=0  in=2  in=0  in=0  in=15
```

一半以上只有 0~2 条。

### A.2 根因（实测）

`champions_structured.go` 里这段：

```go
counterValues := payload.Data.Counters
if spec.PositionMode == opggPositionRequired {
    requestPosition := spec.requestPosition(position)
    for _, raw := range payload.Data.Summary.Positions {
        if strings.EqualFold(strings.TrimSpace(raw.Name), requestPosition) && len(raw.Counters) > 0 {
            counterValues = raw.Counters   // ← 用 3 条的摘要覆盖掉 30 条的完整列表
            break
        }
    }
}
```

**上游有两个 counters 数组，我们用错了那个。** 实测（KR / emerald_plus）：

| 请求 | `data.counters`（顶层） | `summary.positions[X].counters` |
|---|---|---|
| 永恩 TOP | **30 条** | **3 条** |
| 永恩 MID | **27 条** | **3 条** |
| 杰斯 TOP | 26 条 | 3 条 |
| 墨菲特 TOP | 19 条 | 1 条 |

**而且顶层数组本身就已经是按请求位置返回的**，根本不需要再用 positions 覆盖：

```
永恩 TOP 顶层前5个对手: [24 贾克斯, 75 内瑟斯, 58 雷克顿, 266 剑魔, 126 杰斯]   ← 全是上单
永恩 MID 顶层前5个对手: [112 维克托, 84 阿卡丽, 4 卡牌, 517 塞拉斯, 134 辛德拉]  ← 全是中单
```

而 `positions[TOP].counters` 的 3 条是 `[164 卡蜜儿, 23 泰达米尔, 150 纳尔]`
——**正是你图一里显示的那三个**。铁证。

### A.3 连带效应：优势/劣势的划分也因此失真

`champions_structured.go:1259-1261`：

```go
weakCount := min(5, (len(rows)+1)/2)
strongCount := min(5, len(rows)/2)
```

只有 3 条时 → 2 弱 + 1 强。但这 3 条**全都是劣势对局**（胜率 37%~38%），
所以被标成「优势对抗」的那一条其实是 38% 胜率——**只是三个里面最不差的那个**。
这就是图一里「优势对抗 · 38% 胜率」看起来很奇怪的原因。

修好数据源后，30 条排序取头尾各 5，强弱才是真的。

### A.4 改法

1. **删掉那段 positions 覆盖逻辑**，直接用 `payload.Data.Counters`。
2. 保留现有的 `structuredCountersForChampion` 过滤（`dropped_no_meta` / `dropped_no_play` 等）。
3. 顶层为空时（实测杰斯 MID 顶层就是 0 条）走现有空态，**不要**再回退到 3 条的摘要数组
   ——那个数组和请求位置无关，会给出错误分路的对手。

### A.5 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| A1 | 用顶层数组不用 positions | fixture 里顶层 30 条 / positions[TOP] 3 条，断言输出基于 30 条；把覆盖逻辑加回来 → 挂 |
| A2 | 强弱划分 | 30 条 fixture 断言 5 强 5 弱，且「优势」组的胜率都 > 50% |
| A3 | 顶层为空时的空态 | 顶层 0 条 + positions 有 3 条，断言结果为空而不是那 3 条 |

---

## B. 【P0】对局页绝活哥符文不显示

### B.1 已排除的原因

`loadTopPlayersForPosition` **工作正常**。日志里 13 条 `load_top_players_shape`
全部 `parsed: 5, source: "html-table"`——每次都成功解析出 5 个高手玩家。

而且这个函数**只被 `loadSpecialistRunes` 调用**，说明绝活哥链路确实跑了 13 次，
且 Riot Key 是配置好的（否则 handler 在 `specialist_runes.go:68` 就返回
`riot-key-missing` 了，根本走不到这里）。

**所以问题出在拿到 5 个玩家之后的 Riot API 调用环节。**

### B.2 真正的问题：整条链路零埋点，无法诊断

`specialist_runes.go` 全文**没有任何 `diag` / `recordDiagnostic` 调用**。
而 `loadSpecialistRunes` 里每一步失败都是静默 `continue`：

```go
account, err := p.accountByRiotID(...)      // err → continue（静默）
matchIDs, err := p.matchIDs(...)           // err → continue（静默）
match, err := p.matchByID(ctx, matchID)    // err → continue（静默）
if !budget.take() { return result }        // 预算耗尽 → 直接返回（静默）
```

预算 `specialistRuneRequestBudget = 36`，每个玩家要花：
1 次 account + 1 次 matchIDs + 最多 8 次 matchByID。**3 个玩家最坏 30 次**，很容易打满。

**最可能的根因（待新日志验证）**：内嵌的 Riot personal key 是**全体用户共享配额**的
（见 memory `deep-legends-riot-key-and-scraping`），极易触发 429 限流，
而 429 在这里和其它错误一样被静默吞掉。

### B.3 改法

1. **先补埋点**（这是必须的第一步，否则还是瞎猜）：
   - `specialist_runes_start`：championID / position / players_parsed
   - `specialist_runes_step_failed`：step（account / matchIDs / matchByID）+ `errorKind`
     （**必须能区分 429 / 403 / 超时 / 其它**）+ budget_remaining
   - `specialist_runes_done`：runes_returned / budget_used / duration_ms
2. **修 `specialistRuneKey` 缓存键碰撞**（R26 的 P0-1，仍未修）：
   `championID*10 + ordinal` 会让「奥拉夫(2)+打野」和「艾希(22)+无分路」撞成同一个键 `22`。
   我写过可执行测试证明：**1..1000 英雄 × 6 位置共 495 组碰撞**。
   改成字符串键 `fmt.Sprintf("%d|%s", championID, position)`，
   并把已有但从未参与校验的 `specialistRuneCacheEntry.position` 字段用起来做二次比对。
3. **失败要可见**：`respondJSON(w, a.riot.specialistRunes(...))` 直接返回切片，
   空结果和「限流失败」在前端看起来一模一样。加一个 `reason` 字段
   （`rate-limited` / `no-samples` / `budget-exhausted`），前端按 reason 显示不同文案。
4. **顺带确认队列门槛**：`web/gameplay.js:2801` 的
   `recommendationQueueHasTopPlayers` 只认 `queueId === 420 || 440`。
   如果你截图那局是匹配/大乱斗，绝活哥分区**本来就不该出现**——这是设计如此。
   **请在回传新日志时说明当时是什么队列**，好排除这个可能。

---

## C. 三处排位模式切换：解除联动 + 去掉文字 + 修空数据

### C.1 联动根因（读码确认）

三个切换器**共用同一个状态**：

- `web/gameplay.js:15` `rankedQueue: "420"`（每个 tab 一份，但三处共用这一份）
- `:2487` 点击任一按钮 → `tab.rankedQueue = queue`
- `:797` `const queue = rankedQueueData(data, tab)` —— 三处都读这一个值
- `:800-803` 三处都用 `rankedQueueSwitcher(tab, queue.queueId)` 渲染

所以切任何一个，另外两个跟着变。

### C.2 改法：拆成三个独立状态

```js
// 替换单一的 rankedQueue
rankedQueueRecent:   "420",
rankedQueueAbility:  "420",
rankedQueuePosition: "420",
```

- `rankedQueueData(data, tab, which)` 按 which 取对应的 key
- `rankedQueueSwitcher(tab, activeQueue, which)` 输出 `data-ranked-queue-scope="{which}"`
- `:2484` 的事件处理按 `scope` 只更新对应的那一个状态

**注意**：`:2486` 现在有 `tab.rankedQueue === queue` 的短路（点当前项不重绘），
拆分后要按 scope 分别判断，别写成共用。

### C.3 去掉按钮前的文字提示

`addRankedQueueTools`（`:788-790`）用正则把原来的 `<span>`（`$2`）和切换器
一起塞进 `.career-section-tools`。你要去掉的就是这个 `$2`。

**但这个正则本身就是个隐患**：

```js
markup.replace(/(<header><h3>[^<]*<\/h3>)(<span>[\s\S]*?<\/span>)(<\/header>)/, ...)
```

失配时**静默返回原串**——以后给 h3 加个图标，切换器就悄无声息地消失了。

**正好一起修掉**：`renderAbility` / `renderRecentRanked` / `renderPositionStats`
（`:1001` `:1026` `:1080`）**都已经声明了 `queueSwitcher` 形参却从未在函数体内使用**，
调用点也不传。把切换器作为参数直接传进去在模板里渲染，**删掉 `addRankedQueueTools` 这个正则函数**。

### C.4 能力表现切到灵活组排没数据

**根因（读码确认）**：`gameplay.go` 的 `buildGameplayRankedQueues` 里

```go
ability := buildGameplayAbilityProfileForQueue(windowMatches, playerRef, ranks, region, queueID)
positions := positionStatsForQueue(allMatches, playerRef, queueID)
```

**能力表现用的是 `windowMatches`（近期窗口），而位置偏好用的是 `allMatches`（全部）。**
再加上 `player_ability.go:8` `minimumAbilitySampleGames = 3`，且要求
**玩家自己 ≥3 场 且 同位置对手样本 ≥3 场**（`:98-100`）。

所以「这赛季打过灵活组排」但**近期窗口内不足 3 场**，就会是空的。

**改法（二选一，建议都做）：**
1. 能力表现也改用 `allMatches`（或赛季缓存），与位置偏好口径一致
2. 样本不足时**显示明确原因**而不是空白：
   「近期对局中灵活组排样本不足（需 ≥3 场，当前 N 场）」

### C.5 顺带：去掉静默的跨队列回退

`buildGameplayRankedQueues` 里有三处「420 没数据就偷偷用 440」：

```go
if queueID == 420 && recent.Games == 0 { recent = fallback440 }
if queueID == 420 && ability == nil    { ability = ...440 }
if queueID == 420 && positionStatsGames(positions) == 0 { positions = fallback440 }
```

在**没有切换按钮**的时代这是合理的兜底。但现在用户可以显式选队列了，
**显式选了单双排却看到灵活组排的数据**是误导。

建议：用户显式切换后不做跨队列回退，空就空、并说明原因。

### C.6 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| C1 | 三个状态独立 | 改 recent 的队列，断言 ability/position 的 `data-ranked-queue` 仍是原值；改回共用 → 挂 |
| C2 | 切换器不再走正则注入 | 断言 `addRankedQueueTools` 已删除，且三个 render 函数真的使用了 `queueSwitcher` 形参 |
| C3 | 文字提示已移除 | 断言 header 里不再有那个 `<span>` 标签 |
| C4 | 样本不足有明确文案 | 构造 440 只有 2 场的 fixture，断言输出含「样本不足」而不是空字符串 |

---

## D. 本赛季 S1/S2/S3 赛段段位：先埋探测

### D.1 现状：两个数据源都拿不到（日志实证）

**SGP**：`splitsProgress` 字段出现 301 次，**全部是空对象 `{}`**。

**LCU**（`lcu_ranked_stats_shape`，`is_current: true`）的 `season_fields` 实际内容：

```json
{
  "currentSeasonSplitPoints": 0,
  "currentSeasonWinsForRewards": 0,
  "highestCurrentSeasonReachedTierSR": "EMERALD",   ← 本赛季最高段位（只有 tier）
  "highestPreviousSeasonEndDivision": "IV",
  "highestPreviousSeasonEndTier": "EMERALD",
  "previousSeasonEndDivision": "NA",
  "previousSeasonEndTier": "",
  "previousSeasonSplitPoints": 0
}
```

**没有任何按 S1/S2/S3 分开的段位字段。** 只有一个不分赛段的「本赛季最高」。

### D.2 已定案：先埋一次性真机探测

参照仓库里已有的 `sgp_queue_filter_probe.go` 那套模式（一次性调研代码 + 埋点 + 等回传日志）。

**探测目标（按可能性排序）：**

| 候选 | 说明 |
|---|---|
| `/lol-ranked/v1/splits-progress` | 名字最直接，SGP 里那个空字段可能对应它 |
| `/lol-ranked/v1/current-ranked-stats` 的完整原始 JSON | 我们现在只解析了部分字段，**先把完整 key 列表打出来**，可能有已被解析器丢弃的字段（memory 里记过「上赛季+历史最高已在下载被解析器丢弃」这个先例） |
| `/lol-career-stats/v1/summoner-games/{puuid}` | 生涯统计，可能带赛段维度 |
| `/lol-regalia/v2/summoners/{id}/regalia` | 徽章信息，含赛季/赛段成就 |
| `/lol-seasons/v1/*` | 赛季元信息，用来确定赛段边界日期 |

**埋点要求**：
- 每个候选记 `path` / HTTP 状态 / body 字节数 / **顶层 key 列表**（不要记 body 本身，可能含隐私）
- 命中的话额外记「含 split 字样的 key 及其值类型」
- 一次性：跑过一次就落个标记，不要每次启动都探
- **不要记 PUUID 等稳定标识**（项目红线）

### D.3 拿到数据后的 UI（先不做，等探测结果）

你要的形态是在图三每个模式段位下面一行行展示，只展示已经进行到的赛段：

```
单排/双排   翡翠 IV · 0 LP      215胜 192负  53%
   S1  黄金 II
   S2  翡翠 IV          ← S3 未开始就不显示
灵活组排   翡翠 III · 4 LP     215胜 192负  53%
   S1  白银 I
历史最高   翡翠 I
```

规则：**两个模式各自独立展示；没参加/没数据的赛段整行不渲染；未开始的赛段不渲染。**

### D.4 备选（如果探测发现真的拿不到）

退回展示 `highestCurrentSeasonReachedTierSR`（**这个字段现在就有，实测值 EMERALD**），
在每个模式下加一行「本赛季最高 翡翠」。信息量小一些，但是真实可得的。

---

## E. 日志里其它值得处理的问题

### E.1 海克斯大乱斗数据源熔断了 8 小时

```json
{"event":"hexdata_circuit_trip","reason":"shape validation failed","kind":"augments",
 "rows":0,"fields":0,"buildId":"hexdata-2026-08-19-2a6a2393a083",
 "until":"2026-08-26T21:27:37+08:00"}
```

`hexdata_shape_invalid` × 2（`rows: 0, fields: 0`）→ 熔断 → 之后 15 次
`hexdata_circuit_open` 全部直接短路。**buildId 是 08-19 的，已经一周没更新。**

这与上一轮审计发现的 `hexdata.go:579-587`（`recordSuccess` 在 shape 校验**之前**
清零失败计数，导致注释里写的 1m/5m/30m/2h 渐进退避实际不可达）是同一族问题。
建议一并处理，并确认 08-19 那个 buildId 是不是上游真的没更新。

### E.2 SGP 排位数据不完整 236 次

`sgp_ranked_stats_incomplete`（`queue_count: 2`）出现 236 次——这是已知的
「SGP 只对少数玩家回负场」问题（memory `deep-legends-ranked-winrate-lp`），
导致胜率无法展示。数量这么多说明影响面很广，值得单独排期。

### E.3 总览每次加载拉 2~6MB，缓存基本没命中

```
duration_ms=604   sgp_bytes=2226858  cache_hits=1
duration_ms=3310  sgp_bytes=5084979  cache_hits=0
duration_ms=1319  sgp_bytes=6109731  cache_hits=0
...9 次里 7 次 cache_hits=0
```

比历史上的 24s/159MB 已经好很多，但每次 5~6MB、缓存几乎不命中仍然偏重。
这大概率与上一轮审计发现的 `season_stats.go:342`（`ResumeIndex` 在 `Complete=true`
时不归零，导致每次都从很深的偏移量再拉 2 页）有关。

### E.4 `rsc_core_item_rejected` × 40，全是 `kind_aram-augment`

核心装解析时把 40 个大乱斗增强符文当候选拒掉了。守卫在正常工作，
但说明解析器选中的节点范围偏大，建议收紧选择器而不是靠事后过滤。

---

## F. 执行顺序

1. **A**（优劣势对抗）—— 根因明确、改动小、用户可见收益最大
2. **B 的埋点部分** —— 不修问题，只让问题可诊断；配合你回传新日志
3. **B 的缓存键碰撞** —— 会返回错误英雄数据，必修
4. **C**（三处切换解耦 + 去文字 + 空数据文案）
5. **D 的探测埋点** —— 一次性，等回传
6. **E** 按优先级排期（E.1 熔断建议优先）

**另外两件事：**
- 上一轮的 `ACCEPTANCE-R25-WORKLIST-R26.md` 还没执行（移除 lolalytics + 3 个 P0），
  两份工单有重叠，建议合并成一轮做
- 仓库里 `zz_proof_test.go` 是我上一轮验收时留下的空壳，沙箱没有删除权限，**请手动删除**
