# WORKLIST-R33（给 GPT 执行）

> 本轮先说一件必须先解决的事：**这次上传的日志是旧的**，见「零、日志无效」。
> A / B 两组是**读代码坐实的真根因**，可以直接改；C 组（绝活哥）**必须先拿到新日志**才能定改法。
> **D 组是 0828 用新构建的真机截图新发现的真回归，P0，务必一起改。**
> **G/H/I 组（对比 LeagueAkari 战绩链路、穷尽扫描、腾讯官方 qq101 接口）已拆分到
> `WORKLIST-R34.md`，独立执行，不在本文件范围内。**

---

## D 组（新发现，P0）：别人的「单排/双排」和「灵活组排」官方战绩卡显示了一模一样的胜负场

0828 用重新打包、指纹已核对一致的新构建实测（查看非本人玩家"梦短梦长俱是梦"）：

```
单排/双排  225胜 203负  翡翠 IV · 68 LP  胜率 53%
灵活组排   225胜 203负  翡翠 III · 4 LP  胜率 53%
```

两个队列的胜负场次**逐位数字相同**，这不是巧合——真根因在
`gameplay.go:1693 applySeasonRankWinRateFallback`：

```go
func (a *app) applySeasonRankWinRateFallback(ranks []gameplayRank, capability EndpointCapability, season gameplayAggregate) ([]gameplayRank, EndpointCapability) {
	...
	for index := range ranks {
		if ranks[index].WinRate >= 0 {
			continue
		}
		ranks[index].Wins = season.Wins       // ← season 是全队列合并总量
		ranks[index].Losses = season.Losses
		ranks[index].WinRate = season.WinRate
		filled++
	}
	...
}
```

这是给 [[deep-legends-ranked-winrate-lp]] 记录过的"SGP 不回负场"老问题做的兜底：
SGP 某个队列不返回 `losses`（`WinRate` 因此算出负数占位）时，用赛季聚合胜负数填补。
**但传进来的 `season` 参数（`response.SeasonOverall`）是单双排 + 灵活组排合并后的总量**
（`season_stats.go:312 seasonStatsOverall` 对 `seasonStatsAccumulate` 记的每个英雄条目求和，
而 `seasonStatsAccumulate` 本身就没有按队列分——420 和 440 一起累进同一个 per-champion 桶）。
只要单双排和灵活组排**同时**命中"SGP 不回负场"这个老问题（不算罕见，两个队列共用同一条
SGP 链路），这个循环就会把同一份合并总量**分别**塞进两个队列，产出这种"数字重复"的假象。

### 改法

1. 在 `season_stats.go` 里新增一个按队列分的聚合。`seasonStatsAccumulate` 已经拿到了
   `info.QueueID`（420/440），加一个 `map[int64]gameplayAggregate`（或等价结构）在同一次
   遍历里顺手按队列分别计数，别再退回去用"最近 N 场排位快照"去凑（那是有限样本，
   这里要的是整季口径，与 `SeasonOverall` 的口径必须一致，只是拆开队列）。
2. `loadSeasonChampionStats` 把这个按队列聚合一并返回；`gameplay.go:858` 附近接住。
3. `applySeasonRankWinRateFallback` 签名改成接收 `seasonByQueue map[int64]gameplayAggregate`，
   循环内部按 `ranks[index].QueueType`（`"RANKED_SOLO_5x5"` → 420，`"RANKED_FLEX_SR"` → 440）
   去查对应队列的聚合，查不到才跳过（不再拿合并总量兜底）。
4. `riot_api.go` 里如果有同名调用（韩服路径），一并检查是否有同样的合并总量误用。

**变异测试（必须做）**：构造 fixture——本赛季单双排 30 胜 20 负、灵活组排 5 胜 15 负，
两个队列的 `WinRate` 都设成 SGP 未知（-1）触发兜底；断言两个队列填回**不同**的胜负数
（30/20 与 5/15），且都不等于二者相加的合并值。把改法第 3 步改回"不分队列，统一用
`season` 兜底"，测试必须 failed。

---

## 零、日志无效（先解决，否则 C 组无法定案）

**订正**：我上一版工单在这里判断错了一个点——「导出文件名没有时间戳」不能作为
「日志是旧的」的证据，这条已经在 0827 修过了（`features.go:121 diagnosticLogExportFilename`
与 `web/app.js:2110 diagnosticExportFilename` 都已经是 `lol-loot-diagnostics-MMDD-HHmm.jsonl`，
`quality_test.go:603` 有断言）。**用户直接从日志目录里拷走的原始文件**
（`storage.go:578 filepath.Join(s.root, "logs", "diagnostics.jsonl")`）**永远叫这个固定名字，
和有没有打包成功、有没有走导出按钮完全无关**——这是我的误判，工单不该再要求"加时间戳"，那件事已经做了。

真正站得住的证据是内容本身，且是独立的两条：

```
md5  69be0f149fd63b44bf8bd271aa05af19   ← 与上一轮上传的文件逐字节相同
行数 873
最后一条 time: 2026-08-27T08:47:36Z（= 北京时间 16:47）
```

`storage.go:572 appendDiagnostic` 是**只追加**写入的（`os.O_APPEND`），2MB 才轮转一次。
如果用户在这之后重新打开过应用、重新做过操作（哪怕只是打开一次对局页），
文件必然会变大、最后一行的时间必然会往后推。**这份文件没有变大、最后一行还是同一个
时间戳**，说明自 16:47（北京时间）之后这份文件里没有写进过一条新事件——
不管是不是同一次上传，它反映的都是 16:47 之前的程序行为。

而仓库里本轮改动的文件时间戳是 **21:34 ~ 21:55（北京时间）**：
`specialist_runes.go 21:34` / `hexdata.go 21:34` / `champions_structured.go 21:43` /
`web/champions.js 21:54` / `web/gameplay.js 21:55`。**16:47 的日志不可能包含 21:34
之后新加的 `specialist_runes_handler` 埋点**，所以 C 组目前依然没有证据可用。

（这已经是本项目第二次在"日志是不是最新的"上翻车，见
`deep-legends-verification-blindspot` 与 `deep-legends-r33-season-snapshot-and-stale-log`：
先验日志真伪，而且验证手段要经得起复核——这次我自己上一版的验证手段就不够硬，已订正。）

### 0-1 补一个真正缺的东西：构建指纹要在日志里能查、而不是只在设置页

设置页已经有 `buildFingerprint`（`main.go:32`，构建时通过 `-X` 注入，默认值 `"dev"`），
但诊断日志里没有任何一条事件带这个信息，导致每次拿到日志都要额外去问用户"设置页写的是什么"。

**改法**：应用启动时追加一条
`{"event":"app_start","version":"<version>","build_fingerprint":"<buildFingerprint>","riot_key":<riotKeyConfigured()>,"time":...}`
到 diagnostics.jsonl。这样任何一份日志的**第一行**就能自证是哪个构建产出的，
不再需要靠时间戳去猜。

**验证**：重启应用，日志第一行是 `app_start`，`build_fingerprint` 与设置页
「版本 X · 构建 Y」一致；`riot_key` 与实际是否配置了 key 一致。

### 0-2 用户现在就能做、不需要等这次改动的自查

麻烦在 Deep Legends 里打开「设置」页，把顶部那行「版本 X · 构建 Y」截图或原样发我——
如果 `构建` 后面写的是 `dev`，说明这个安装包在打包时没有正确注入构建指纹，
大概率就是 `deep-legends-portable-cache-trap` 里记录过的那个问题：便携版缓存键没变，
新 exe 被静默换成了旧程序。这一步能几秒钟内确认是不是"改动根本没跑起来"，
比对着日志猜快得多。

**已用这一步实锤确认**：用户设置页显示 `构建 d21fdfb6f4e8`；对当前仓库源码跑
`node desktop/source-fingerprint.cjs` 算出来是 `da000bcd0bfb`——**两者不一致**。
`source-fingerprint.cjs` 对 `web/**` 全部文件与仓库根目录全部 `*.go` 文件内容做哈希，
任何一处源码改动都会让这个值变化，所以这是比日志时间戳更硬的证据：
**用户手上的 exe 打包时对应的源码树，不是当前这一份**，这轮改动（含 GPT 加的
`specialist_runes_handler`）一定没有被打进用户正在测试的这个包里。

**这不是代码逻辑的问题，是交付链路的问题**：要么新包还没打出来给用户，要么打出来了
但安装时被 `deep-legends-portable-cache-trap` 那个老问题吞掉了（新 exe 静默复用了
`%LOCALAPPDATA%` 里的旧程序）。C 组（绝活哥）在此之前**没有任何一份日志能反映当前代码的真实行为**，
不管日志文件名或时间戳看起来多新，只要构建指纹对不上 `da000bcd0bfb`（或打包后
`node desktop/source-fingerprint.cjs` 重新算出的最新值），这份日志就依然无效。

**打包/交付要求（P0，必须在继续排查 C 组前完成）**：
1. 打包完成后，构建流程里加一步自检：把 `desktop/apply-portable-template.cjs` /
   `build-desktop.sh` 里已经在用的 `node desktop/source-fingerprint.cjs` 结果
   与注入到二进制里的 `main.buildFingerprint` 断言相等（该有的代码已经在，
   只是从来没人拿它去跟"用户设置页显示的值"做过闭环核对）。
2. 把新打的包（不是覆盖安装，是全新安装或先卸载旧版）交给用户，用户安装后
   **先看设置页构建指纹是否变化**，确认变化了再重新测试、导出日志。

### 0-3 ★★已直接修复（不用 GPT 再做，见下方"已完成"）：应用内「导出诊断日志」按钮是死 UI，用户压根点不到

上面 0-2 验证过构建指纹已经对上了（`da000bcd0bfb`），但用户后续第三次上传的
`diagnostics.jsonl` 仍然与 8 月 27 日 16:50 那份**逐字节相同**（md5 `69be0f14...`，
1 天多过去了文件大小和最后一行时间戳纹丝不动）。这次不是构建的问题——真根因找到了，
是这个应用从一开始就没法从界面里导出一份新日志，用户没有做错任何事。

`web/index.html:287` 确实有 `<a id="export-diagnostics" href="/api/diagnostics/log" download>导出诊断日志</a>`，
但它所在的 `<section id="diagnostics-panel" hidden aria-hidden="true">` 是一个**独立的顶层
`<section>`**，不属于任何 `.settings-subpanel`（对照 `web/app.js:117`
`el.settingsPanels = [...document.querySelectorAll(".settings-subpanel")]`），
**全部前端代码里没有任何一处引用 `diagnostics-panel` 这个 id**，没有任何 tab 的
`aria-controls` 指向它，也没有任何按钮把它的 `hidden` 属性摘掉。这个面板永远不会显形，
用户从「设置」里根本找不到导出入口——这条结论 [[deep-legends-diagnostics-log]] 六天前
就记录过，这次是被今天的反复拉锯重新实锤。

雪上加霜：`desktop/main.cjs:262` 里 `devTools: !app.isPackaged`——**打包后的正式版
DevTools 是关闭的**，用户连开发者工具走 `location.href='/api/diagnostics/log'` 这条路
也走不通。用户唯一能做的只有去 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl`
手动翻文件，这个体验太反直觉，大概率翻到旧的缓存副本或者干脆翻错文件夹——这才是
"用户坚持日志是新的，我们看到的却总是同一份"这场拉锯的真正根源。

**改法（P0，独立于 A/B/C/D 组，优先级最高——不修这个，以后每一轮排查都要
靠人肉猜日志新旧）**：
1. 在「设置 → 隐私与能力」页（`web/index.html:249 settings-privacy-panel`）里加一张
   `setting-card`：标题「诊断与日志」，内容复用现成的 `#copy-diagnostics`/`#export-diagnostics`/
   `#diagnostic-log-meta` 这几个元素（原样搬进这张卡片，id 不变，JS 逻辑已经在
   `loadDiagnostics()`/`copyDiagnostics` 事件里写好了，只是从来没被渲染出来过），
   不需要重新写渲染逻辑。
2. 确保 `loadDiagnostics()` 在设置页首次进入「隐私与能力」标签时被调用一次
   （现在只在 `copyDiagnostics` 点击时才懒加载）；`diagnostic-log-meta` 那行文字
   要能在不点复制按钮的情况下也显示日志大小、最后写入时间，用户扫一眼就能自己
   判断这份日志新不新，不用发来回问我。
3. 保留 `%LOCALAPPDATA%` 手动路径作为文档记录的备用方案（面板里加一行小字提示
   这个路径），但主路径必须是"点一下按钮就下载"。

**验证**：设置页「隐私与能力」标签下能看到「诊断与日志」卡片；点「导出诊断日志」
触发浏览器下载，文件名带时间戳（`lol-loot-diagnostics-MMDD-HHmm.jsonl`）；`diagnostic-log-meta`
显示的日志大小与直接读 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl` 的文件大小一致。

**★0828 已直接实现（GPT 不用再做这条，打包时会自动包含）**：
1. `web/index.html` 删掉了独立的 `#diagnostics-panel` 顶层 section，把 `#copy-diagnostics`/
   `#export-diagnostics`/`#diagnostic-log-meta`/`#diagnostics-content`（id 全部不变）原样
   搬进了 `settings-privacy-panel` 里新增的 `.settings-diagnostics` 区块，紧跟在「隐私与安全」
   卡片下方，并补了一行 `%LOCALAPPDATA%`/macOS 路径的小字说明作为备用方案。
2. `web/app.js` 的 `activateSection` 里 `if (name === "settings")` 分支现在会跟 `loadPrivacy()`
   一起调用 `loadDiagnostics()`，进设置页就自动加载，不用先点复制按钮才懒加载；
   `loadDiagnostics()` 里本来就有的 `el.exportDiagnostics.setAttribute("download", diagnosticExportFilename())`
   （`app.js:2107` 生成 `lol-loot-diagnostics-MMDD-HHmm.jsonl`）和服务端
   `features.go:112` 的 `Content-Disposition` 时间戳文件名两层保护都还在，这次只是让
   按钮本身变得可达，没有动导出逻辑。
3. `web/app.css` 补了 `.settings-diagnostics` 的分隔线样式（跟 `.settings-privacy` 一致）。
4. 验证：`node --test web/*.test.cjs`（149/149）、`node --test desktop/*.test.cjs`（29/29，
   含 `overview-render.test.cjs` 12/12 jsdom 真渲染冒烟测试）全绿，未新增改动 Go 代码。
   全局搜索确认 `diagnostics-panel` 这个 id 在 `web/` 下已 0 处引用。

---

## A 组：排位胜率/场次「刷新几次才稳定」（真根因已坐实）

### A-1 事实链

- 首屏赛季扫描只翻 **2 页**：`season_stats.go:23 seasonScanForegroundPages = 2`，
  `sgp_api.go:76 sgpPageSize = 20` → **首屏只看最近 40 场对局**。
- 这名玩家最近的对局里排位占比很低（旧日志 `sgp_queue_filter_probe` 实测
  `distinct queues = [420, 1750, 2400]`），所以 40 场里可能只捞到个位数的排位。
- 剩下的靠后台回补：`seasonScanBackgroundPages = 12`（每轮 240 场）。旧日志里
  `season_backfill_round` 的 `resume_index` 是 **280 → 560 → 840，`complete: false`**，
  说明回补要跑好几轮。
- 每刷新一次，快照里的排位场次就多一些 → 「近 20 场排位」的胜率/场次跟着变，
  直到攒够 20 场或整季扫完才稳定。

**结论：数字会变是设计使然，不是 bug；但「不告诉用户它还在变」是 bug。**

### A-2 改法：`games > 0` 但赛季没扫完时也要给出「统计中」的态

现在 `web/gameplay.js` 里 `collecting` **只在 `games === 0` 分支被消费**
（`renderRecentRanked` / `renderAbility` / `renderPositionStats` 三处都是这个结构）。
一旦有 1 场数据就直接渲染正常卡片，用户看到的就是一个每次刷新都不一样的胜率。

要求：

1. `renderRecentRanked` 在 `games > 0 && collecting === true` 时，在卡片头部
   （`career-section-tools` 里、队列页签左边）加一个**统计中角标**，文案：
   `统计中 · 已统计 N 场`，其中 N 取 `data.seasonStatsProgress.scanned`。
   鼠标悬停提示：`正在后台向前读取本赛季对局，样本补全前胜率与场次还会变化`。
2. `renderAbility`、`renderPositionStats` 同样处理（同一个角标组件，别复制三份 HTML）。
3. `seasonComplete === true` 时角标必须消失——这是这条改动最容易做错的地方，
   必须有测试覆盖。

### A-3 改法：回补完成后自动重绘，不要让用户靠手动刷新去发现

后台回补是 `startSeasonBackfill` 起的独立任务，完成时前端毫不知情。

- 后台每完成一轮（现有 `season_backfill_round` 埋点的同一位置）通过既有的
  live-updates 通道推一条 `{"type":"season-progress","season":"S26","scanned":N,"complete":bool}`。
- 前端收到后：若当前在总览页且 `complete` 变为 true，或 `scanned` 增量 ≥ 100，
  就静默重新拉一次 `/api/gameplay/overview` 并重绘（**不要**弹刷新提示、
  **不要**重置用户选中的队列页签与滚动位置）。
- 节流：同一账号 10 秒内最多重绘一次。

**验证**：构造一个 `complete:false` 的赛季缓存，首屏渲染出角标；模拟推送
`complete:true` 后角标消失且数字停止变化。

---

## B 组：明明打过灵活，却显示「本赛季未参加灵活组排」（真根因已坐实，P0）

### B-1 真根因：排位快照的 40 条上限是**两个队列共享**的

`season_stats.go`：

```go
seasonRankedMatchLimit = 40           // 第 28 行

func seasonTrimRankedMatches(cache *seasonStatsCache) {   // 第 270 行
    ...
    sort.SliceStable(merged, func(i, j int) bool { return merged[i].CreatedAt > merged[j].CreatedAt })
    if len(merged) > seasonRankedMatchLimit {
        merged = merged[:seasonRankedMatchLimit]          // ← 两个队列混在一起裁
    }
    cache.RankedMatches = merged
}
```

单双排和灵活组排**混在同一个数组里**按时间倒序裁到 40 条。
只要玩家最近 40 场排位都是单双排，**灵活组排会被整队裁光**，于是：

- `recentRankedSummaryForQueue(..., 440)` 过滤后拿到空数组 → 「近 20 场排位」灵活页签空；
- `buildGameplayAbilityProfileForQueueWithSnapshot(..., 440)` 快照侧也空 → 能力表现空；
- 等整季扫完 `seasonComplete === true`，前端就打出
  **「该玩家本赛季未参加灵活组排 / 已完成本赛季对局统计。」**——一句与事实相反的断言。

更糟的是 `seasonTrimRankedMatches` 在 `finishSeasonScan` 里**每次扫描都会跑**
（前台增量扫描也跑），所以后台好不容易翻到的灵活对局，会在下一次前台扫描时
被更新的单双排挤掉并**永久写回磁盘**。灵活样本永远攒不起来。

旧日志里 `season_backfill_round` 的 `ranked_samples` **每一轮都恰好是 40**，
说明上限一直是顶满的、裁剪一直在生效。

### B-2 改法：按队列分别保留

1. `seasonTrimRankedMatches` 改成**按 `QueueID` 分组**，420 与 440 **各自**保留最新
   `seasonRankedMatchLimit` 条，再合并写回（合并后仍按时间倒序，方便下游取前 20）。
2. `seasonRankedMatchLimit` 保持 40 不变（每队列 40，够「近 20 场」用两倍余量）。
3. 加一条埋点，放在 `finishSeasonScan` 里：
   `{"event":"season_ranked_snapshot","season":"S26","solo":N,"flex":M,"capped_solo":bool,"capped_flex":bool,"oldest_solo":ts,"oldest_flex":ts}`
   下一份日志就能直接读出灵活到底有没有被裁掉。

**变异测试（必须做）**：把分组裁剪改回「混在一起裁 40」，用「41 场单双排 + 3 场更早的灵活」
这组 fixture，测试必须 failed。这是本条唯一有意义的护栏。

**0828 新构建实测追加确认**：这个 bug 不止影响自己，查看别的玩家（"梦短梦长俱是梦"）时，
右侧完整战绩列表切到「灵活组排」明明能看到真实对局，左侧「近 20 场排位」「能力表现」
两张小卡片却都显示「该玩家本赛季未参加灵活组排」。这条路径（`gameplay.go:858` 附近，
`isCurrent=false` 也会调用同一个 `loadSeasonChampionStats`）走的是同一套按 `accountHash`
持久化的赛季快照逻辑，`accountHash` 只依赖传入的 `player.PUUID`，本身跟是不是"自己"无关——
**B-2/B-3 修完后必须补一个非本人 `playerRef` 的测试用例**，不能只测自己这个账号，
否则复现不了这次真机看到的现象。

### B-3 顺带修掉一个会造成同样假结论的早返回

`season_stats.go:343`：

```go
if serverID == "" || a.sgp == nil || !validPlayerReference(playerRef) {
    return nil, seasonStatsProgress{Season: season, Complete: true, Message: "当前样本已按排位队列统计"}, nil
}
```

**`Complete: true` + 零条排位快照**，前端就会显示「该玩家本赛季未参加XX / 已完成本赛季对局统计。」
但这条分支的真实含义是「**根本没做赛季统计**」（韩服玩家、SGP 不可用、玩家引用无效都会走到这里）。

**改法**：这里必须返回 `Complete: false` 且给一个能区分的 `Message`（例如
`"当前数据源不提供赛季统计"`），并在 `seasonStatsProgress` 上加一个显式字段
`Unavailable bool`。前端 `rankedQueueData` 里：

- `unavailable === true` → 既不是 collecting 也不是 seasonComplete，文案走
  「当前数据源未提供本赛季排位统计」，**绝不能说「该玩家本赛季未参加」**。

**规则（写进注释）**：「没查到」和「查过了确实没有」是两件事，只有后者才允许对用户下断言。

### B-4 「该玩家本赛季未参加XX」的措辞收紧

即便 B-2/B-3 都修好，`seasonComplete && !playedQueue` 仍然依赖「整季扫描真的完整」。
把文案改成不那么绝对：主标题 `本赛季暂未发现${queueLabel}对局`，
副标题 `已完成本赛季对局统计（共 N 场）。`。

---

## C 组：对局页绝活哥符文仍然不出现（**先拿新日志，别盲改**）

### C-1 现状：这次已经有能定位的埋点了

GPT 本轮在 `specialist_runes.go:78` 加了 `specialist_runes_handler`
（`outcome` = accepted / skipped / failed，`reason` = riot-key-missing / champion-metadata），
补上了上一轮**唯一缺的那一环**——在这之前，如果 Riot Key 没配好，接口会
静默返回 `{"reason":"riot-key-missing"}`，日志里一条记录都不留，所以才会
「一直修不好」：**我们从来没有证据能区分下面四种情况**。

### C-2 拿到新日志后，按这张表一次定案

| 日志里看到什么 | 结论 | 对应改法 |
|---|---|---|
| **完全没有** `specialist_runes_handler` | 前端压根没请求 | 走 C-3 |
| `handler outcome=skipped reason=riot-key-missing` | 打包没注入 Riot Key | 走 C-4 |
| `handler outcome=accepted` 但没有 `specialist_runes_start` | 卡在缓存/单飞/并发槽 | 查 `specialistRunes()` 的 single-flight，多半是上一次失败被缓存了 6 小时（`specialistRuneCacheTTL`）——空结果不该按 6 小时缓存 |
| `specialist_runes_start players_parsed=0` | 榜单没解析出人 | 查 `loadTopPlayersForPosition`；注意旧日志里 `load_top_players_shape parsed=5`，所以这条概率低 |
| `specialist_runes_step_failed step=account/match_ids/match_detail` + `errorKind` | Riot API 侧失败 | 看 errorKind：`rate-limited` → 共享 Personal Key 配额被打满；`http-403` → key 过期 |
| `specialist_runes_done runes_returned=0` 且没有 step_failed | 扫了但没扫到该英雄的对局 | 放宽 `specialistRuneMatchScanMax`（现在 8）或改用 match-v5 的 champion 过滤 |

**在拿到这份日志之前，不要动 C 组任何代码。**

### C-2.5 ★0828 新日志已拿到：真机复现，第一行**完全没有** `specialist_runes_handler`，且已排除三个最可能的解释

0828 用重命名规避传输缓存后拿到的干净日志（`diagnostics-0828-1020.jsonl`，02:18:24~02:19:45 UTC）
证实了一次真实的对局：英雄 `zaahen`，玩家确认这局是**召唤师峡谷自定义**（`queueId` 应为 0、
`gameMode=CLASSIC`、`mapId=11`）。日志里能看到这局对局的出装/符文/海克斯推荐、
`load_top_players_shape zaahen parsed:5`（榜单页有数据）都正常返回了两轮，
但**从头到尾没有一条 `specialist_runes_handler`，也没有任何 `specialist_runes_*` 事件**——
不是失败，是请求从来没有发出去过。

**已排除的三个假设**（按代码逐条核对过，不用再查）：

1. **不是旧包**：对当前仓库跑 `node desktop/source-fingerprint.cjs` 得到 `da000bcd0bfb`，
   与用户设置页此前已核对过的构建指纹完全一致，说明这次测试用的就是当前这份源码，
   不是构建滞后的问题。
2. **不是"自定义房间没有榜单所以正确地不请求"**：C-3 第 2 点写的"训练模式/自定义房间里为 false"
   **已经过时，与当前代码不符**——`gameplay.go:2612 resolveGameplayRecommendationMode` 里
   `mode == "CLASSIC" && mapID == 11` 会显式把这类对局解析成 `InternalMode: "ranked"`；
   `gameplay.go:2567 recommendationModeHasTopPlayers` 对 `InternalMode == "ranked" && QueueID == 0`
   显式返回 `true`；前端 `web/gameplay.js:2810 recommendationQueueHasTopPlayers` 在拿不到
   `payload.hasTopPlayers` 时的兜底规则里也有 `queueId === 0 && mode === "CLASSIC" && (mapId === 0 || mapId === 11)`
   这一条。**前后端两层判断都明确把"召唤师峡谷自定义"当成有榜单的对局**，
   闸门理论上应该放行，C-3 第 2 点这条结论要删掉重写。
3. **不是 riot-key-missing 缓存**：如果是这个原因，`specialist_runes_handler` 至少会打一条
   `outcome=skipped reason=riot-key-missing`——但连这条都没有，说明请求根本没有离开浏览器，
   问题不在 Go 后端这一侧。

**结论：这是 `web/gameplay.js:2692 ensureSpecialistRunes` 内部某个提前 `return` 在拦截，
但现有埋点体系完全看不到浏览器端发生了什么**——`api()` 调用之前的所有分支
（`target` 为空 / 网关判断为 false / `embedded` 命中缓存 / `state.specialistRuneFailures` 里有
未过期的失败记录 / `state.specialistRuneFlights` 一直卡着没被清空导致后续调用永远被去重掉）
都不会留下任何后端可见的痕迹。**下一步不是猜，是先加临时诊断**：

**改法（C 组新的第一步，比 C-3/C-4 优先级更高）**：在 `ensureSpecialistRunes` 每一个提前
`return` 之前加一次 `console.debug` 或者更好——通过既有的客户端遥测通道（如果有）打一条
`specialist_runes_client_skip` 事件并带上具体原因（`no-target` / `no-top-players` /
`cached-empty` / `key-missing-permanent` / `recent-failure-cooldown` / `in-flight`），
这样下一份日志能直接看到浏览器侧在哪一步拦下来的，不用再靠代码走查猜测。
**这几个分支里最值得怀疑的是 `state.specialistRuneFlights`**：如果某次请求异常中断
（比如页面在请求进行中切换了英雄/掉线重连）导致 `finally` 块没有执行到、
`target.key` 却复用了同一个值，`flights.has(target.key)` 会永远返回 true，
后续这个英雄/位置组合就再也不会重试，直到应用重启——这跟"每次都用同一个英雄测都复现"的
体感是吻合的，值得作为第一个变异测试的对象。

### C-3 前端没发请求时的已知拦截点（⚠️ 第 2 点已被 C-2.5 更正，其余仍需自查）

`web/gameplay.js:2692 ensureSpecialistRunes`：

1. `liveRecommendationTarget(data)` 为空 → 没锁定英雄就不请求（符合预期）。
2. ~~`recommendationQueueHasTopPlayers(data)` → 训练模式/自定义房间里为 false~~
   **已被 0828 实测推翻**：召唤师峡谷自定义（`CLASSIC`+`map11`）在当前代码里前后端都会判定
   有榜单，不是这条拦截的。真正的自定义类拦截只会发生在非召唤师峡谷的自定义模式
   （比如自定义大乱斗、自定义斗魂），这些确实应该在空态里明说「当前队列没有可用的绝活哥榜单」。
3. `state.specialistRuneFailures` 里存了 `"riot-key-missing"` → **这个失败态永不过期**
   （`specialistFailure === "riot-key-missing"` 直接 return，没有时间判断）。
   一旦某次拿到 key-missing，**这个会话内再也不会重试**，即使用户中途配好了 key。
   **改法**：给它也加 60 秒过期，与另一条失败态一致。

### C-4 如果确认是 Riot Key（大概率）

按 `deep-legends-portable-cache-trap` 和 `deep-legends-exe-packaging-verification-0823`：

- 便携版有缓存陷阱：`portable.nsi` 的缓存键是 `VERSION + 手工序号`，两者都没变时
  新 exe 会静默复用 `%LOCALAPPDATA%` 里的旧程序。**先确认用户跑的是新 exe**
  （0-1 的 `app_start.version` 就是为这个准备的）。
- 构建时 `-X` 注入是否生效，用 `strings` 扫二进制验证（0823 已经这么验过一次）。
- 无论如何，**空态文案要说人话**：key 缺失时显示「未配置 Riot Key，绝活哥符文不可用」
  并给一个「去设置」的入口，而不是「暂无可核验的韩服绝活哥符文」——后者会让人
  以为是数据问题，这正是「一直修不好」的体感来源。

---

## E 组（0828 新增，真机截图）：收藏页多处 UI/数据展示问题 + 全局下拉框样式不统一

用户提供了收藏页（账户与物品）的真机截图，逐条如下。**这组跟 A~D 组无关，可以独立并行改，不用等 C 组的日志诊断。**

### E-1 下拉框样式不统一：「段位」筛选跟「搜索玩家」不是一套东西

英雄页的「段位」筛选（`web/champions.js:798`/`:1437`，`<label class="champion-tier-select select-wrap"><select data-champion-tier>`）是**原生 `<select>`**，展开后的选项列表是 Chromium 系统原生弹层，应用的 CSS 完全管不到——用户截图里那个纯黑底+蓝色高亮选中项，就是系统原生外观，不是我们画的。

「搜索玩家」旁边的地区选择器走的是完全不同的实现：`web/index.html:54-62 #player-search-region-menu.region-menu`，是纯 CSS 画的自定义弹层（`role="menu"` + `role="menuitemradio"` 按钮），样式在 `web/champions.css:317-347`，选中态用 `var(--primary-strong)`/`var(--primary-soft)`。收藏页排序控件 `#sort-button.select-menu-trigger` + `#sort-menu.select-menu-popover`（`web/index.html:176`，样式 `web/champions.css:458-466`）也是同一套自定义弹层模式（背后留一个 `sr-only` 的真 `<select>` 只做无障碍语义）。

**排查结果：全项目除了「搜索玩家」地区选择器和收藏页排序，其余全部下拉框都是原生 `<select>`**，包括但不限于：`#pool-picker`/`#pool-quality`/`#pool-sort`（`web/index.html:199-204`）、设置页里 `#setting-default-page`/`#setting-match-count`/`#setting-default-match-filter`/`#setting-champion-position`/`#setting-live-order`（`web/index.html:232,238-241`）、以及这次截图里的「段位」筛选。**不是"哪几个"不统一，是"只有两个"统一了、其余都是原生系统外观**。

**改法**：把 `region-menu`/`select-menu-popover` 这套自定义弹层模式抽成一个可复用组件（选中态高亮、圆角、边框、弹出动画都跟「搜索玩家」一致），替换掉上面列出的所有原生 `<select>`。工作量较大，建议按页面拆成几个小 PR：先做英雄页「段位」+「默认位置」两个（用户这次点名的），再扩展到收藏页和设置页。**每换一处都要保留原生 `<select>` 做键盘/无障碍兜底**（参考收藏页排序控件已有的 `sr-only` 用法），不能只做视觉层。

### E-2 材料列表里 `CHEST_promotion` 显示英文原始 ID，图标也不对

截图里"材料"分组下有一项直接显示字面量 `CHEST_promotion`，图标看起来跟旁边"战利品宝箱"（`CHEST_CHAMPION_MASTERY`）一样。查证：

- 中文名映射表 `lootChineseNames`（`lcu_api.go:61-67`）**只有 `CHEST_CHAMPION_MASTERY: "战利品宝箱"` 一条，没有 `CHEST_promotion`**，兜底链 `lootDisplayName`（`lcu_api.go:639-647`）在映射缺失、LCU 自带本地化名称又是空的情况下，最终展示原始 `LootID` 字符串——这就是显示成英文 ID 的原因，不是 bug，是漏了这条映射。
- 图标同理：`lootClientIcons`（`lcu_api.go:69-75`）和前端 `lootImagePaths`（`web/app.js:2043-2054`）也都只映射了 `CHEST_CHAMPION_MASTERY`，`CHEST_promotion` 没有专属图标条目，会整个 fallthrough 到 LCU 原始响应里的 `tilePath`/`asset` 字段（`web/app.js:2053`）——**不是"代码写错复用了宝箱图标"，是根本没有为这个 ID 单独配置任何东西，视觉上撞车是巧合**。
- 素材缺口：`web/loot-icons/` 目录下只有 `hextech-chest.png`/`hextech-chest-transparent.png` 两张图，都已经挂给 `CHEST_CHAMPION_MASTERY`，**没有紫色促销宝箱的素材**，需要新找一张图（Data Dragon/CDragon 的战利品图标库应该有，用户描述这是"紫色宝箱"，按 `CHEST_promotion` 这个 loot ID 去 CDragon 的 loot 图标端点查一下）。

**改法**：①在 `lootChineseNames` 加 `CHEST_promotion: "晋级宝箱"`（或更准确的名字，需要先查证这个 loot 具体是什么——建议先查 LCU `/lol-loot/v1/player-loot` 或 CDragon loot 数据确认官方名称，不要瞎翻译）；②找到对应的紫色宝箱图标素材放进 `web/loot-icons/`，在 `lootClientIcons`（后端）和 `lootImagePaths`（前端）各加一条映射；③顺手全量核对一遍还有没有其他 loot ID 也是这种"映射表只覆盖了一个 ID"的半成品状态（`lootChineseNames`/`lootClientIcons` 目前都只有单条，覆盖率本来就很可疑，建议拉一份用户账号里出现过的全部 `LootID` 去对着映射表查缺口）。

### E-3 收藏页「当前召唤师」信息条要跟总览页统一：加背景图、六边形换圆形头像

收藏页当前实现（`web/app.js:1401`）：`<span class="account-hex" aria-hidden="true">⬡</span>` —— **这是一个纯 Unicode 六边形字符，不是任何头像图片**，没有用到 `profileIconId` 或任何头像字段，跟截图里看到的一致。

总览页（`web/gameplay.js:858-864` `.summoner-strip`）已经是想要的效果：圆形头像走 `iconFigure("profile", player.profileIconId, ...)`（`gameplay.js:837`），CSS `.game-icon.is-summoner-avatar { border-radius:50%; }` 72×72px（`gameplay.css:920`）；生涯背景图是 `player.backgroundSource`/`backgroundPath` 驱动的满铺 `<img class="summoner-strip-art">`（`gameplay.js:852-853`，`gameplay.css:176-179`，加载完成后淡入）。

**改法**：收藏页的召唤师条直接复用总览页这一套（不是重新实现一遍）——把 `account-hex` 换成跟 `gameplay.js:837` 一样的 `iconFigure("profile", ...)` 调用，接上生涯背景图的 `backgroundSource`/`backgroundPath` 逻辑和 CSS，头像与召唤师名称之间的间距对照总览页的实际取值调小（截图里收藏页这块间距明显比总览页松）。**「主页背景」这个统计格子直接删掉**——它在 `<dl class="account-facts">` 三格网格里（`web/app.js:1401`），CSS 是写死的 `grid-template-columns: repeat(3,minmax(0,1fr))`（`web/app.css:599`），删格子的同时要把这里改成 `repeat(2,...)`，不然会留一块空白，不是删一个 `<div>` 就完事。

### E-4 「待领取奖励」卡片显示了明显的调试占位符文本

截图文案原样是 `Placeholder Name for Reward Group DO NOT TRANSLATE`——这是 LCU 直接返回的字段：`lcu_api.go:596 grant.Title = source.RewardGroup.Localizations.Title`，原样透传，没有做任何过滤。前端 `web/app.js:1396` 只有在 `grant.title` 是**空字符串**时才会退回 `"待领取奖励"` 兜底文案，但 LCU 这次返回的不是空字符串，是这段占位符原文，所以兜底没触发。

**改法**：在展示前加一层"疑似占位符"检测（可以放在 `lcu_api.go:596` 赋值时，也可以放在前端 `app.js:1396` 渲染前），命中就退回通用文案 `"待领取奖励"`。判定规则建议：包含 `DO NOT TRANSLATE`（大小写不敏感）；或者整串是全大写字母+下划线+空格的样子（典型占位符格式，比如 `PLACEHOLDER_NAME_FOR_...`）。**不要试图翻译这段占位符**，它本来就是 Riot 客户端自己的一个已知瑕疵（未配置的奖励组名称），不是我们的数据源问题。

### E-5 「收藏与奖池已更新」提示的触发条件跟"更新"没有关系

这不是一个一次性 toast，是常驻的 `<section id="notice">` 横幅（`web/index.html:123-125`，渲染逻辑 `renderNotice()` in `web/app.js:402-425`），只在收藏页的"皮肤与炫彩"子页显示。成功文案 `✓ 收藏与奖池已更新`（`app.js:423-424`）触发条件是 `data.calculationOK && !stale`——**这个字段的含义是"当前缓存的核对结果是有效的"，不是"刚刚发生了一次更新"**，命名和实际语义对不上。

而且 `refreshStatus()` 会在几乎任何一次 `/api/events` SSE 推送时被防抖调用（`app.js:2526-2528`），跟收藏/奖池数据是否真的变化完全无关，`renderNotice()` 又是每次 `refreshStatus` 都无条件执行（`app.js:177`）——所以这条横幅会在跟收藏毫无关系的事件后也重新刷新展示，给人一种"又更新了一次"的错觉，这就是用户说的"状态不对"。

**改法**：①先把文案改成不带时效性暗示的说法，比如去掉"已更新"，换成"核对结果有效"之类的静态状态描述，或者②如果产品上确实想要一个"刚刚更新"的提示，需要新增一个真正基于"这次数据跟上次不同"的比对（存一份上次核对结果的哈希/时间戳，`calculationOK` 从 false→true 或数据实际变化时才展示"已更新"，其余时间只展示常驻状态而不是重新播放"已更新"的措辞）。选哪种方案由 GPT 跟用户确认一下产品意图。

**变异测试要求**：E-1 无法写传统单测（纯视觉），用 jsdom 渲染测试断言"段位"筛选和其它列出的原生 select 换成新组件后 DOM 结构（`role="menu"`）跟搜索玩家一致；E-2 补一个 fixture（loot 列表含 `CHEST_promotion`）断言中文名和图标都不是兜底值；E-4 补大小写混合与非典型占位符两个 fixture，断言都被拦截且合法奖励名称不会被误杀；E-5 补一个"SSE 推送了无关事件、收藏数据没变"的场景，断言横幅文案没有重新念一遍"已更新"（如果采用方案②）。

---

## F 组（0828 新增，用户点名参考的开源项目）：借鉴 LeagueAkari 的排位数据链路

用户发来一张 LeagueAkari（`https://github.com/LeagueAkari/LeagueAkari`）的真机截图——单双排/灵活组排两张卡片 + 一张完整队列表（含"上赛季段位"/"上赛季最高段位"/"历史最高段位"），要求"看一下这两个排位模式的胜率和场次怎么获取的，他获取的很快也很准，借鉴一下"。

已经把这个仓库拉到本地读了源码（不是猜的），关键发现如下。

### F-1 核心发现：LeagueAkari 对排位数据完全不碰 SGP，只用两个本机 LCU 端点

`src/renderer/.../data/ranked-stats.ts` 开头注释原文：

> 加载排位赛信息 —— 此特性仅支持 lcu 数据源，且不可在跨区查询时生效

调用链路：`lc.api.ranked.getRankedStats(puuid)` → `src/shared/http-api-axios-helper/league-client/ranked.ts`：

```ts
getCurrentRankedStats() { return this._http.get('/lol-ranked/v1/current-ranked-stats') }
getRankedStats(puuid)  { return this._http.get(`/lol-ranked/v1/ranked-stats/${puuid}`) }
```

**这两个端点我们仓库里已经在用**——就是 `gameplay.go:1737 loadGameplayRanks` 现在的 SGP 失败兜底路径，一字不差。区别在于**优先级**：我们是"先打 SGP，SGP 有响应就用 SGP（哪怕 losses 是假的 0），SGP 彻底挂了才退回 LCU"；LeagueAkari 是**只用 LCU，从来不碰 SGP**，无论查自己还是查别人（`getRankedStats(puuid)` 直接接受任意 puuid）。用户截图里"很快很准"，快是因为这是纯本机 HTTP 调用没有外部网络往返，准大概率是因为**绕开了 SGP 那个"少数玩家才回负场"的老毛病**（[[deep-legends-ranked-winrate-lp]] 实测过 2460 条只有 42 条带负场）。

### F-2 顺带发现：我们自己的 LCU 结构体在丢字段，跟"上赛季/历史最高"那个老工单是同一个坑

LeagueAkari 的 `RankedEntry` 类型（`src/shared/types/league-client/ranked.ts`）对每个队列还带了
`previousSeasonEndTier`/`previousSeasonEndDivision`/`previousSeasonHighestTier`/`previousSeasonHighestDivision`/`highestTier`/`highestDivision` 六个字段，`RankedTable.vue` 截图里那三列（上赛季段位/上赛季最高段位/历史最高段位）就是直接读这几个字段渲染的，不需要额外请求。

对照我们自己 0827 那次真机埋点 `lcu_ranked_stats_shape` 打出来的 `queue_keys`，**这些字段名一个不差全都在里面**——也就是说我们的 LCU 响应里本来就带这些数据，但 `gameplay.go:373 lcuRankedEntry` 这个结构体只解析了 `QueueType/Tier/Division/LeaguePoints/Wins/Losses/IsProvisional` 七个字段，其余全部被 `json.Unmarshal` 静默丢弃。**这正是 [[deep-legends-cn-historical-ranks-research]] 里"上赛季+历史最高已在下载被解析器丢弃"那条老结论**——当时的修复是在 SGP 路径（`sgpRankedEntry`，`sgp_api.go:737`）补的，LCU 兜底路径这个结构体从来没跟着补全。

### F-3 改法（用户已拍板：直接切，不用先等一轮观察）

**下行风险很低，值得直接做**：`verifiedRankWinRate`（`gameplay.go:1782`）这道闸门本来就在——
发现 `wins>0 && losses==0` 会标成"未验证"、走赛季聚合兜底，这个保护跟数据来自 SGP 还是
LCU 无关。也就是说，就算换成 LCU 优先后某些玩家 LCU 也拿不到完整负场，最差情况也只是
回到现在这个"胜率暂不展示"的降级态，不会比现在更差；但只要 LCU 对大多数情况确实可靠
（本机直连，大概率不用绕后台隐私/共享数据校验），大部分玩家就能直接拿到准确胜负场，
是个只赚不亏的方向。

1. **先做 F-2**：补全 `lcuRankedEntry` 结构体六个历史字段，跟 `sgpRankedEntry` 对齐命名。
2. **`loadRanksWithFallback`（`gameplay.go:1596`）调整顺序**：本机 LCU 客户端已连接时，
   **LCU 优先**（`a.loadGameplayRanks`），SGP 降级成补充/兜底——用于 LCU 也没能验证完整
   （`losses` 仍是 0）时再去查一次 SGP 试着补全，或者 LCU 请求失败/客户端未连接/跨服
   时的兜底路径（这几种场景 SGP 本来就是唯一选项，逻辑不变）。**查自己和查别人都走这个
   新顺序，不要只切自己那条**——用户这次要解决的正是"查别人"负场缺失的问题。
3. **保留现有诊断埋点**（`ranked_winrate_resolved` 的 `source` 字段本来就区分 `"lcu"`/`"sgp"`），
   这样换完之后如果真的还有查不到负场的情况，日志里能立刻看出是哪个来源、哪种玩家类型，
   不需要事后再去猜。

**变异测试要求**：①F-2 结构体补全——fixture（原始 LCU JSON 带全六个历史字段）断言解析后
六个字段都不为空，字段名改错一个测试要 failed；②F-3 优先级调整——fixture 模拟"LCU 返回
完整 wins+losses、SGP 也可用"场景，断言最终结果来自 LCU 而不是 SGP（`source` 字段可查），
把顺序改回"SGP 优先"这个变异体要让测试 failed；再补一个"本机 LCU 未连接/请求失败"场景，
断言依然能正确回退到 SGP，不能两条路都不通。

**验证**：换完之后请用户重新打包，用同一个非本人玩家（比如"梦短梦长俱是梦"）复测一次，
截图对比负场是否补全，顺手确认历史段位三列是否也一起显示出来了。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 测试与 JS 测试全绿。
- A-2、B-2、B-3 三处各写**至少一个真变异测试**（把代码改回旧行为 → 测试必须 failed → 复原变绿），
  并把变异结果贴在提交说明里。B-2 的变异体见 B-2 第 3 点。
- 改完后请用户**重启应用**（不是刷新页面）再导出一次日志，确认第一行是 `app_start`、
  文件名带时分，再上传。
