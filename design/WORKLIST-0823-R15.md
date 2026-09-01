# WORKLIST 0823-R15（补护栏：性能改动裸奔 + 两个假护栏 + CSS 重复漂移）

诊断人：Claude（只读验收，未改仓库文件）。执行人：GPT。
本单**不改任何业务逻辑**，只补测试和消除重复代码——R13 的功能已经全部实现并验收通过，
这里要堵的是"下次有人（或 AI）不小心改坏了却没人报错"的缺口。

前置：`design/WORKLIST-0823-R14.md`（go:embed 泄漏测试文件）是另一张独立工单，两者互不依赖，顺序不限。

---

## 一、【P0】R13 性能改动零护栏 —— 变异测试 8 杀 8 活里存活的那 8 个

真副本 + 变异测试实测：下面 8 种改法，现有的 `go test ./...` 与
`node --test web/champions.test.cjs` **全部保持绿色**，等于这些改动没有任何东西在守着。
其中前四条最危险——回退后果分别是本轮刚修好的 412MB 加载量爆炸、
和用户这次报的"核心装只有一条"这两个真实故障。

### N1【最高危】总览窗口查询的 `useCache` 参数没有护栏

`gameplay.go:733`：

```go
infos, _, more, windowErr := a.sgp.matchHistoryOn(ctx, client, reference.ServerID, playerRef, 0, maximumSummaryMatchCount, true)
```

把最后一个参数 `true` 改回 `false`，`go test ./...` 依然全绿——这正是本轮
412MB 加载量爆炸的病根（见 `[[deep-legends-r13-log-findings]]`），现在完全没人拦。

**要补**：在 `gameplay_test.go` 里用 `sgpRoundTripFunc`（`sgp_api_test.go:16` 已有的假
transport 模式，直接复用）起一个假 SGP server，对同一个 `playerRef` 连续调用两次
`handleGameplayOverview`（间隔小于 `sgpCacheTTL=90s`），断言 **第二次请求数为 0**
（或断言 `overview_load_cost` 埋点里 `sgp_cache_hits > 0`）。

### N2 复用已取战绩的短路条件没有护栏

`gameplay.go:732`：

```go
if reference.ServerID != "" && validPlayerReference(playerRef) && len(matches) == 0 {
```

删掉 `&& len(matches) == 0` 这个条件，测试依然全绿——意味着"30 天窗口复用主列表已取的
前 20 场战绩、不重复请求"这个优化本身没有断言。

**要补**：同一个假 transport 里数请求次数，断言当 `matches` 已经非空时（比如
`begIndex=0` 的主战绩请求已经拿到数据），不会再触发窗口查询对 `matchHistoryOn` 的
第二次调用（可以在假 transport 里给 `/leagues-ledge` 之外的 SUMMARY 路由计数）。

### N3 single-flight 没有护栏

`rank_insights.go:118-133`（`beginFlight`/`finishFlight`）配 `:143-152`
（`playerRankScore` 里 leader/follower 分流）。把 `:147` 的
`return flight.entry` 改成 `return rankScoreEntry{}`（follower 不复用 leader 结果，
直接返回空值），测试依然全绿——说明**没有任何测试验证过并发去重真的生效**。

**要补**：起多个 goroutine 并发调用 `playerRankScore` 传同一个 `cacheKey`，
配合假 transport 里加一个原子计数器统计实际发出的 HTTP 请求数，
断言并发 N 次调用只产生 1 次真实请求，且所有 goroutine 拿到的 `entry` 相同。

### N4【高危】`CoreOptions` 上限没有护栏 —— 正是用户这次报的故障

`gameplay.go:2105`：

```go
bundle.Build.CoreOptions = bundle.Build.CoreOptions[:5]
```

改成 `[:1]`，测试依然全绿。这是用户这次截图报的"核心装怎么只有一条"的对应代码，
**现在改坏它没有任何测试会失败**。

**要补**：一条最简单的单元测试——喂 6 条以上的 `CoreItems` 进 `structuredMetrics`
链路（或直接对包含该裁剪逻辑的函数传入超过 5 条的假数据），断言最终产出恰好 5 条、不多不少。

### N5 "只查活动页签"没有护栏

`web/gameplay.js:1299`：

```js
if (!tab.overlay && tab.key !== state.activeTab) return;
```

删掉这一行，`champions.test.cjs` 依然全绿——多页签同时发请求这个回归点无人拦。

**要补**：源码级断言 `hydrateMatchTiers` 函数体里含这一行判断
（参考本项目已有惯例：`shouldReloadOverview` 一类用 `assert.match(gameplayScript, /.../)` 的写法）。
如果想做真行为测试，需要先把 `hydrateMatchTiers` 里的活动页签判断抽成独立纯函数
（如 `shouldHydrateMatchTiers(tab, activeTabKey)`），比源码正则更可靠，优先这样做。

### N6 批量去重的分批大小没有护栏

`web/gameplay.js:1380`：

```js
for (let offset = 0; offset < uniqueRefs.length; offset += 24) {
```

把 `24` 改成 `1`（退回逐个查询），测试依然全绿。

**要补**：源码级断言这一行的批大小常量存在且等于后端上限
（`rank_insights.go:65 matchTiersMaxRefs = 24`，两处应该保持一致，
最好前端也定义成一个具名常量而不是魔法数字 `24`，并在测试里同时断言两边数值相等，
防止以后一边改了另一边没跟上）。

### N7 `overview_load_cost` 埋点名没有护栏

`gameplay.go:397` 附近的 `"event": "overview_load_cost"`，改成任意其它字符串，测试依然全绿。

**要补**：一条测试直接调用 `handleGameplayOverview`，断言 `recordDiagnostic` 收到过
`event == "overview_load_cost"` 且字段 `sgp_requests`/`sgp_bytes`/`sgp_cache_hits`/`duration_ms`
都存在（可以复用 `a.diag` 之类现有的诊断回调 mock 方式，参考
`TestRankedWinRateDiagnosticsRecordSGPAndLCUSources` 的写法）。

### N8 对局内出装顶部三列布局没有列数护栏

`web/gameplay.css:833`：

```css
.build-summary-bar { display: grid; grid-template-columns: repeat(3,minmax(220px,1fr)); ...
```

把 `repeat(3,...)` 改成 `repeat(2,...)`，测试依然全绿——C4 现有的测试
（`champions.test.cjs` 里那条 `live build recommendations...`）只断言了三块的顺序，
没断言列数。

**要补**：在现有测试里加一行 `assert.match(gameplayStyles, /\.build-summary-bar\s*\{[^}]*grid-template-columns:\s*repeat\(3,/s);`。

---

## 二、【P1】R12 遗留：设置页版本指纹本身没有护栏

这个功能存在的**唯一目的**就是让用户能在设置页肉眼确认"我现在跑的是不是这个构建"
（`[[deep-legends-portable-cache-trap]]` 那次事故就是因为没有这东西，排查绕了好几圈）。
现在它自己却没有任何测试盯着。

### N9 后端不返回指纹没有护栏

`main.go:435`：

```go
Version: version, BuildFingerprint: buildFingerprint, Connected: a.connected, ...
```

把 `BuildFingerprint: buildFingerprint` 删掉（字段留空），`go test ./...` 依然全绿。

**要补**：一条测试直接调用 `handleStatus`，断言 JSON 响应里 `buildFingerprint` 字段非空。

### N10 前端不渲染指纹没有护栏

`web/app.js:344`：

```js
el.settingsBuildIdentity.textContent = `版本 ${data.version || "未知"} · 构建 ${fingerprint}`;
```

把这一行删掉或清空，88+ 项 JS 测试依然全绿——`champions.test.cjs:9` 虽然读了
`app.js` 全文，但没有任何断言碰这一段。

**要补**：源码级断言 `app.js` 里同时出现 `data.buildFingerprint` 与
`settingsBuildIdentity.textContent` 的赋值语句；再断言 `web/index.html:220`
的 `#settings-build-identity` 元素存在（防止 HTML 那端也被误删）。

---

## 三、【P1】C2 出装列 CSS 在两个文件里各写了一份，且已经开始漂移

`web/champions.css:480-489`（详情页）与 `web/gameplay.css:852-861`（对局内）
是**同一套 `.build-item-row` / `.build-core-column` 规则的两份拷贝**，class 名完全相同。
现在拿去 diff 已经能看到两处不一致：

```diff
- champions.css:485  .build-core-column, .build-depth-column
+ gameplay.css:857    .build-core-column, .item-core-column, .build-depth-column, .item-depth-columns > section
```
```diff
- champions.css:488  ...border-left: 1px solid var(--line); border-top: 0; }
+ gameplay.css:860    ...border-left: 1px solid var(--line); }              ← 少了 border-top: 0
```

第二处差异会导致对局内出装区域比详情页多一条分隔线——这轮验收暂未观察到明显视觉问题，
但两份规则会持续独立漂移，工单要求的"抽成同一套"没有做到。

**要做**：把这套 `.build-item-row` 家族规则（`build-item-row` 本体 + 四个
`[data-depth-count="N"]` 变体 + `.build-core-column`/`.build-depth-column`/
`.item-depth-columns` 相关规则）抽成一份共享文件，两处都引用它。

具体做法两选一，GPT 按项目现有构建方式选一个：

1. 如果 `web/` 目前是纯静态文件、没有构建步骤（据观察确实如此，`index.html`
   直接引用 `champions.css`/`gameplay.css`），新建 `web/build-item-row.css`，
   在两个页面的 `<link>` 里都引入它，并把重复规则从两个文件里删掉；
2. 或者接受"两份拷贝"的现状，但改成**其中一份是权威、另一份从权威生成**
   （不推荐，纯静态项目没必要引入构建步骤）。

**优先选方案 1**。

**要补的护栏**：一条测试读取 `web/champions.css`、`web/gameplay.css`（或新的共享文件），
断言 `.build-item-row` 家族规则**只在一个地方定义**（比如断言旧的两个文件里都不再包含
`.build-item-row {` 这个选择器，只有共享文件里有），防止以后又被复制粘贴回去。

---

## 验收清单

- [ ] N1~N8：性能类改动全部补上行为测试或源码级断言，且每条都用变异体自验
      （改回旧逻辑，新测试必须真的 FAIL）
- [ ] N9~N10：设置页版本指纹的后端字段与前端渲染都有护栏
- [ ] 出装列 CSS 合并成一份共享文件，`.build-item-row` 选择器不再在两处重复定义
- [ ] `go test ./...` 全绿，`node --test web/champions.test.cjs` 全绿，`gofmt -l` 只剩既有的 `yourgg_arena.go`
- [ ] 详情页与对局内出装区域视觉验证一次，确认合并 CSS 后两处显示效果与合并前一致
      （截图或本地打开对比即可，不需要额外自动化）

**测试纪律**（本项目反复踩过，务必遵守）：

1. 跑测试用 `node --test web/champions.test.cjs`，**不能用 `node --test web/`**。
2. 每条新断言都要用变异体自验——把对应的代码改坏，确认新测试真的会 FAIL；
   再改回来，确认变绿。**不要只加断言不做这一步**，本项目已经因为这个反复出现"看似有效实则零覆盖"的教训。
3. 变异测试拷副本时必须拷真副本（不能软链接源码文件），且要带 `desktop/`、
   `prestige_chromas.json`（`prestige.go:20` 有 `go:embed`）、`data/`
   （`main.go:33` 有 `go:embed data/...`）；**跑变异体前先跑一次未变异的对照组，
   确认测试本身是绿的**，否则所有"杀死"的结论都是假的。
4. Go 里把条件改成 `if false` 会触发 "declared and not used" 编译错误 → 假杀，
   要换成仍然引用原变量的变异形式（如 `if created <= 0 {` 而不是 `if false {`）。
