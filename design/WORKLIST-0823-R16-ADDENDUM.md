# WORKLIST 0823-R16-ADDENDUM（补护栏：R16 九处改动目前零测试保护）

诊断人：Claude（只读校验，未改仓库文件）。执行人：GPT。

## 结论先行

**R16 的功能代码全部正确落地**（A~G 七组逐条读码确认，`go build`/`go test ./...`/
`node --test web/champions.test.cjs`/`web/remaining-sort.test.cjs` 全绿，
`gofmt -l` 只剩既有的 `yourgg_arena.go`）。

但用真副本做了 9 组变异测试后发现：**把 R16 改对的地方精确改回旧的错误状态，
现有测试套件（92+18 项）全部依然通过**——也就是说这 9 处改动目前完全没有护栏，
下一轮任何一次改动都可能在测试全绿的情况下悄悄把它们改回去。
其中 **E（符文图标）、B1（核心装换行）、A4/B3（选用率显示 0%/—）** 正是用户
已经反复提过、上一轮才刚订正过判据的三个问题，尤其需要补上。

---

## 存活变异体清单（逐条给出精确断言）

### 1. E — 符文图标尺寸上界（用户提过两轮的老问题，风险最高）

把 `web/gameplay.css:991` 的 `--rune-icon-size: 32px` 改回 `36px`，92 项测试依然全绿。

**要做**：在 `web/champions.test.cjs` 加一条

```js
assert.doesNotMatch(gameplayStyles, /--rune-icon-size:\s*(3[3-9]|[4-9]\d)px/);
```

这条就是工单 R16 里已经明确要求的那条，之前没有被加上。

### 2. B1 — 对局内核心装路线换行

把 `web/gameplay.css:858` 的 `.config-option.is-route .config-icons { flex-wrap: nowrap; ...}`
改回 `flex-wrap: wrap`，92 项测试依然全绿。

**要做**：

```js
assert.match(gameplayStyles, /\.config-option\.is-route \.config-icons\s*\{[^}]*flex-wrap:\s*nowrap/);
```

### 3. A4/B3 — 四五六件选用率字段（详情页 + 对局内，两处都没测）

- `web/champions.js:1504` 把 `renderConfigOption(row, "item", null, renderDepthStats)`
  改回 `renderConfigOption(row, "item")`（丢弃 `renderDepthStats`），92 项依然全绿。
- `web/gameplay.js:2420` 把 `optionList(options, "item", "暂无数据", renderDepthStats)`
  改回 `optionList(options, "item", "暂无数据")`，92 项依然全绿。

**要做**：直接测函数输出内容，而不是只测存在与否。用 `compileFunctions` 分别把
`champions.js` 的 `renderDepthStats` 和 `gameplay.js` 的 `renderDepthStats` 抽出来单测：

```js
// 两处 renderDepthStats 都要覆盖：只输出「胜率」「场次」两列，不含「选用率/选取率」
const out = renderDepthStats({ winRate: 55.38, games: 6233 });
assert.doesNotMatch(out, /选用率|选取率/);
assert.match(out, /<dt>胜率<\/dt>/);
assert.match(out, /<dt>场次<\/dt>/);
```

再各加一条源码断言，确认两处调用点真的传了 `renderDepthStats`（防止有人把调用参数删掉）：

```js
assert.match(championsScript, /renderConfigOption\(row, "item", null, renderDepthStats\)/);
assert.match(gameplayScript, /optionList\(options, "item", "暂无数据", renderDepthStats\)/);
```

### 4. C — 技能加点统计颜色与顺序

把 `web/gameplay.js` 的 `renderRecommendationStats` 改回旧版
（`选取率→场次→胜率`、类名 `recommendation-stats`、无 `is-pick/is-win/is-games`），
92 项依然全绿。

**要做**：

```js
const { renderRecommendationStats } = compileFunctions(gameplayScript, ["renderRecommendationStats", "renderOptionStats", "rate", "compactNumber"]);
const out = renderRecommendationStats({ pickRate: 12.3, winRate: 55.1, games: 88 });
assert.match(out, /class="option-stats"/);
assert.doesNotMatch(out, /class="recommendation-stats"/);
// 顺序：选取率 → 胜率 → 场次
const order = [...out.matchAll(/<dt>([^<]+)<\/dt>/g)].map((m) => m[1]);
assert.deepEqual(order, ["选取率", "胜率", "场次"]);
assert.match(out, /class="is-pick"/);
assert.match(out, /class="is-win"/);
assert.match(out, /class="is-games"/);
```

### 5. B4 — 四五六件上限 5

把 `web/gameplay.js:2403-2405` 的三处 `5` 改回 `3`，92 项依然全绿。

**要做**：

```js
assert.match(gameplayScript, /byWinRate\(build\.fourthOptions \|\| \[\], 5\)/);
assert.match(gameplayScript, /byWinRate\(build\.fifthOptions \|\| \[\], 5\)/);
assert.match(gameplayScript, /byWinRate\(build\.sixthOptions \|\| \[\], 5\)/);
```

### 6. A2/B2 — 两侧共用行高变量的实际取值

把 `web/build-item-row.css:3` 的 `--build-row-height: 60px` 改成 `40px`，92 项依然全绿
（说明现在只是"两边引用了同一个变量"，没人断言这个变量本身该等于多少、
也没人断言等高契约真的生效）。

**要做**：

```js
assert.match(sharedBuildStyles, /--build-row-height:\s*60px/);
assert.match(championsStyles, /\.route-row\s*\{[^}]*min-height:\s*var\(--build-row-height,\s*60px\)/);
assert.match(championsStyles, /\.champion-build-board \.config-option\s*\{[^}]*min-height:\s*var\(--build-row-height,\s*60px\)/);
assert.match(gameplayStyles, /\.config-option\s*\{[^}]*min-height:\s*var\(--build-row-height,\s*60px\)/);
```

### 7. F2 — 韩服总览并发数（Go 侧，`go test ./...` 目前不会发现回退）

`riot_api.go:836` 把 `semaphore := make(chan struct{}, 4)` 改回 `8`，
`go test ./...` 依然全绿——完全没有测试盯着这个具体数值。

**要做**：在 `riot_api_test.go`（或新建一个针对 `loadRiotOverview` 并发行为的测试）里，
用一个带并发计数器的假 Riot HTTP server，断言"同一时刻在途请求数 ≤ 4"，
而不是只做源码正则断言（源码断言也可以作为兜底，但主断言应该是行为级的）：

```go
// 伪代码示意，具体按仓库里已有的假 server 测试风格实现
var inFlight, maxInFlight atomic.Int32
// 每次请求进入时 inFlight++，若超过历史最大值则更新 maxInFlight；请求结束 inFlight--
// 断言 maxInFlight <= 4
```

### 8. G — 赛季聚合兜底的接线（Go 侧，只测了裸函数，没测真实调用链）

把 `gameplay.go:723` 的
`ranks, rankCapability = a.applySeasonRankWinRateFallback(ranks, rankCapability, response.SeasonOverall)`
整行删掉（只保留 `_ = response.SeasonOverall` 占位），`go test ./...` 依然全绿。
现有的 `TestSeasonRankWinRateFallbackFillsOnlyIncompleteSGPRanks` 只单测了裸函数，
没有任何测试确认这个函数真的在 `loadGameplayOverview`/`handleGameplayOverview`
的调用链里被接上。

**要做**：写一条端到端测试——构造一个 SGP 返回"有 wins 无 losses"的假响应
（复用现有假 SGP server 测试基础设施），命中 `sgp_ranked_stats_incomplete` 分支后，
断言最终 `handleGameplayOverview` 返回的 `Ranks[].WinRate`/`Losses` 确实被赛季聚合数据填充了，
而不是仍然是「不完整」的哨兵值。

### 9. F3 — 韩服 tab 前端超时 25 秒

把 `web/gameplay.js:390` 的 `riotTab(tab) ? 25_000 : 10_000` 改成
`riotTab(tab) ? 10_000 : 10_000`，92 项依然全绿。

**要做**：

```js
assert.match(gameplayScript, /const timeout = riotTab\(tab\) \? 25_000 : 10_000;/);
```

---

## 验收清单

- [ ] 1~9 每条新断言先确认变异体自验：把对应代码改回本文档描述的错误状态，
      新断言必须让测试 FAIL；改回来，测试必须变绿
- [ ] `node --test web/champions.test.cjs`、`node --test web/remaining-sort.test.cjs`
      改完后仍然全绿（不能用 `node --test web/`）
- [ ] `go test ./...` 改完后仍然全绿；`gofmt -l` 只剩既有 `yourgg_arena.go`
- [ ] 变异测试拷副本时必须拷真副本（不能软链接源码），并带上 `desktop/`、
      `prestige_chromas.json`、`data/`；**跑变异体前先确认对照组是绿的**
- [ ] Go 里把条件改成 `if false` 会触发 "declared and not used" 编译错误 → 假杀，
      请改成逻辑等价但能编译通过的变体

**不需要改任何功能代码**——本单 9 条全部是"只补测试，不动实现"，
因为逐条读码 + `go build`/`go test`/JS 测试已确认 R16 的功能实现本身是对的。
