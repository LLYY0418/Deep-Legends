# WORKLIST 0823-R16（出装布局 + 统计列口径 + 样本门槛 + 韩服总览超时 + 他人胜率兜底）

诊断人：Claude（只读诊断，未改仓库文件）。执行人：GPT。
本单基于两份日志复核：第一份是旧构建产生的（已排除，不作为任何结论依据）；
第二份 `diagnostics-0d59fad4.jsonl`（1577 行，06:38~11:47）确认是当前构建产生的新鲜日志，
下面所有结论均以第二份为准。每条都给了实测根因与文件行号，请按顺序逐条落实。

> **口径决定（不要自行更改）**
> 1. 第四/五/六件的统计列 = **胜率 + 场次**（去掉选用率——上游 OP.GG 本身没有这个字段，
>    见 A4 实测，不是我们的渲染 bug）；核心装路线仍保留 **选取率 + 胜率 + 场次**。
> 2. 四五六件每类最多显示数量：3 → **5**（B4，需求变更，不是回归）。

---

# A【P0】英雄详情页出装 —— 行高过大 / 统计换行 / 选用率无数据

## A1+A3 行高过大与"胜率换行"是同一个根因

`web/champions.css:502`：

```css
.champion-build-board .config-option { min-width: 0; flex-wrap: wrap; }
```

`flex-wrap: wrap` 让 `.option-stats` 在宽度不够时**折到图标下面另起一行**，
于是每一行被撑成两行高——这正是第四/五/六件每格特别高、且「选用率/胜率」
跑到图标下方的原因。

**要做**：

1. 把 `:502` 改成 `flex-wrap: nowrap;`，让图标与统计**始终同一行**。
2. 统计列宽收紧，避免 nowrap 之后挤压图标：`:509` 的
   `.champion-build-board .option-stats` 由 `repeat(2,minmax(62px,74px))`
   改为**两列**（A4 之后是 胜率+场次）`repeat(2,minmax(52px,64px))`，并加 `flex: 0 0 auto;`。
3. 给 `.champion-build-board .config-option` 一个与左侧路线行一致的固定行高（见 A2）。

## A2 两侧行高必须对齐，且核心装单条路线要略高一点

左侧核心装路线行是 `web/champions.css:481`：

```css
.route-row { display: flex; align-items: center; gap: 12px; padding: 8px 16px; }
```

右侧 depth 行是 `.config-option`（无固定高度，由内容撑开）。两边高度因此对不齐。

**要做**：引入一个共享行高变量，放进 `web/build-item-row.css`（两边都已引用该文件）：

```css
/* 出装区两侧共用行高：左侧核心装单条路线与右侧四五六件每格必须等高 */
.build-item-row { --build-row-height: 60px; }
```

然后：

- `web/champions.css:481` `.route-row` 增加 `min-height: var(--build-row-height, 60px);`
  （核心装单条路线的高度再高一点，60px 比当前 `padding:8px` 撑出来的约 52px 更宽松）；
- `.champion-build-board .config-option` 同样设 `min-height: var(--build-row-height, 60px);`。

**验收判据**：详情页出装区，左侧一条核心装路线的高度 = 右侧第四件任意一格的高度。

## A4【已实测定案】选用率永远没有数据 —— 上游根本不提供

抓了真实 OP.GG 页面（`https://op.gg/zh-cn/lol/champions/twistedfate/build?patch=16.16&region=kr&tier=emerald_plus&type=ranked`）
并按 `decodeNextFlight` 的方式解码，取出 `depth_4_item_0` 整块：

```
该行内出现的百分比数值： ['55.38']      ← 只有胜率
该行内出现的场次：       ['6,233']
该行内是否含 pick/选用/选取 字样： False False False
```

**结论：OP.GG 的第四/五/六件数据里只有「胜率」和「场次」两项，没有任何选用率字段。**
所以 `champions_structured.go` 的 `parseOPGGItemDepths` 构造 `championMetricRow` 时
只写了 `WinRate` 和 `Games`、**没写 `PickRate`**，这是**正确的**——
错的是前端在渲染一个上游不存在的字段：详情页渲染成「—」，对局内渲染成「0%」。

**要做**：

`web/champions.js` 的 `renderBuildDepthGroups` 目前调用通用的
`renderConfigOption(row, "item")`，进而走到 `renderOptionStats`（选用率+胜率）。
新增一个 depth 专用的统计渲染，只输出两列：

```js
function renderDepthStats(row) {
  const hasWin = Number.isFinite(Number(row?.winRate)) && Number(row.winRate) > 0;
  const games = Number(row?.games);
  if (!hasWin && !(games > 0)) return "";
  return `<dl class="option-stats"><div class="is-win"><dt>胜率</dt><dd>${percent(row?.winRate)}</dd></div><div class="is-games"><dt>场次</dt><dd>${compactNumber(games)}</dd></div></dl>`;
}
```

并让 `renderBuildDepthGroups` 里的每一行使用它（`renderConfigOption` 加一个可选参数，
或在 depth 分支单独拼行，**不要动核心装路线那条链路**——那边上游确实有选用率）。

> `compactNumber` 若 `champions.js` 里没有，就用 `formatNumber`/本地实现，
> 与对局内 `gameplay.js` 的显示格式保持一致（如 `6,233`）。

---

# B【P0】对局内出装 —— 核心装换行 / 高度不齐 / 统计列 / 四五六件只有 3 个

## B1 核心装路线换行

`web/gameplay.css:861`：

```css
.config-icons { display: flex; min-width: 0; flex: 1 1 auto; flex-wrap: wrap; align-items: center; gap: 6px; overflow: visible; }
```

核心装路线 4 件装备排成「3 个 + 换行 1 个」，就是这条 `flex-wrap: wrap` 造成的。

**要做**：核心装路线（`kind === "route"`）必须单行。给路线行加一个修饰类
（例如 `.config-option.is-route .config-icons { flex-wrap: nowrap; overflow-x: auto; }`），
或直接把 `.config-icons` 改成 `nowrap` 并允许横向滚动。
**不要**用缩小图标的方式硬塞——图标尺寸要与其它模块一致。

## B2 高度与详情页对齐

`web/gameplay.css:860` 目前是 `min-height: 66px`，与详情页的 `.route-row` 不一致。

**要做**：改用 A2 定义的同一个变量 `min-height: var(--build-row-height, 60px);`，
让详情页与对局内两处**用同一个数值**。

## B3 统计列去掉选用率（同 A4 结论）

`web/gameplay.js` 的 `renderOptionStats`（约 `:2380` 附近）现在固定输出三列
选取率/胜率/场次。四五六件的 `pickRate` 上游不存在，渲染成 `0%` 是错的。

**要做**：与 A4 同样处理——四五六件用只含 胜率+场次 的两列渲染；
**核心装路线保持三列不变**。

## B4 四五六件上限 3 → 5

`web/gameplay.js:2402-2404`：

```js
const fourthOptions = byWinRate(build.fourthOptions || [], 3);
const fifthOptions  = byWinRate(build.fifthOptions  || [], 3);
const sixthOptions  = byWinRate(build.sixthOptions  || [], 3);
```

三处 `3` 改成 `5`（详情页本来就是按上游给多少显示多少，最多 5）。

---

# C【P0】对局内技能加点的统计顺序与颜色不对

两个渲染函数不一致，这是唯一根因：

`web/gameplay.js` `renderRecommendationStats`（技能加点用）：

```js
`<dl class="recommendation-stats"><div><dt>选取率</dt>...<div><dt>场次</dt>...<div><dt>胜率</dt>...`
```

顺序是 **选取率 → 场次 → 胜率**，且外层类是 `.recommendation-stats`、
内层 `div` **没有 `is-pick`/`is-win`/`is-games` 类**。

而颜色规则只挂在 `.option-stats` 上（`web/gameplay.css:871-873`）：

```css
.option-stats .is-pick  dd { color: var(--accent); }
.option-stats .is-win   dd { color: var(--success); }
.option-stats .is-games dd { color: var(--muted); }
```

`.recommendation-stats`（`:717-720`）**只定义了字号，没有任何颜色**——
所以技能加点的数字是灰白的，鞋子/出门装的是彩色的。

**要做**：让 `renderRecommendationStats` 输出与 `renderOptionStats`
**完全相同的结构**：类名用 `option-stats`，顺序 **选取率 → 胜率 → 场次**，
三个 `div` 分别带 `is-pick`/`is-win`/`is-games`。
改完后 `.recommendation-stats` 若无其它使用者，一并从 CSS 删掉。

---

# D【P0】法外狂徒等英雄没有核心装与鞋子推荐

## 根因：绝对样本门槛 50 场把整组数据滤空了

`champions_structured.go`：

```go
structuredMetricMinimumGames = 50
structuredMetricHeadRatio    = 0.01

func structuredSampleAllowed(play, leadingSample int) bool {
	return play >= structuredMetricMinimumGames && leadingSample > 0 && float64(play)/float64(leadingSample) >= structuredMetricHeadRatio
}
```

`structuredMetrics` 对每一行都要过这道闸，**不满足就整行丢弃**；
若该英雄该位置所有核心装/鞋子行的 `Play` 都 < 50，就会整组为空 → 前端显示「暂无核心装备样本」。
而 OP.GG 官网没有这道闸，所以用户在 OP.GG 上看得到数据。

**要做**（建议方案 2）：

1. 直接下调 `structuredMetricMinimumGames`（如 10）——简单但会让长尾噪声回来；
2. **改成"保底 + 相对门槛"**（推荐）：先按 `structuredMetricHeadRatio` 过滤，
   若过滤后整组为空，则**至少保留场次最高的 N 行**（N 取该组 limit 与实际行数的较小值），
   保证"上游有数据就一定看得见"，同时长尾仍被排序压到后面；
3. 让门槛随 `leadingSample` 自适应（如 `min(50, leadingSample/5)`）。

**必须同时加埋点**：`structured_metrics_gate`，记录
`kind`（core/boots/starter/spell）、`in`（上游行数）、`out`（通过门槛行数）、
`leading`（最大场次）、`dropped_below_floor`（因绝对门槛丢弃的行数）。
下次真机日志就能直接确认这条闸有没有再误杀。

---

# E【P1】对局内主副系符文图标过大、互相遮挡

`web/gameplay.css` 的窄容器规则把尺寸**调大**了：

```css
@container (max-width: 980px) {
  .recommendation-panel { --rune-icon-size: 36px; }   /* ← 默认是 675 行的 32px */
  .unified-rune-row { gap: 10px; min-height: 48px; }
  .unified-rune-column { grid-template-rows: 40px repeat(4,48px); ... }
}
```

默认 `.recommendation-area { --rune-icon-size: 32px }`（`:675`），
但窄屏时反而升到 **36px**，塞进 48px 的行里就会与相邻元素挤压/重叠。

**要做**：把 `@container (max-width: 980px)` 里的 `--rune-icon-size: 36px` 改成 **不超过 32px**
（直接删掉这一行让它继承 32px 最简单）。改完确认主系基石、副系图标与下方小符文**三者视觉等大**。

**护栏**：断言 CSS 中**不存在任何把 `--rune-icon-size` 设为大于 32px 的规则**，例如

```js
assert.doesNotMatch(gameplayStyles, /--rune-icon-size:\s*(3[3-9]|[4-9]\d)px/);
```

这条比"只断言复用同一令牌"强得多——上一轮就是只断言了复用同一令牌，
漏掉了"令牌本身被容器查询改大"，导致用户重复提了两轮同一个问题。

---

# F【P0】韩服总览经常超时卡住

新日志实测证据链（`diagnostics-0d59fad4.jsonl`）：

1. `web/app.js:127` 的 `api()` 默认 `timeout = 10000`（10 秒），`loadOverview`
   调用 `/api/gameplay/overview` 时没有传自定义超时，国服韩服共用这个 10 秒值。
2. 韩服总览走 `riot_api.go:708 loadRiotOverview`，这条链路**完全没有诊断埋点**
   （对比国服路径有一整套 `sgp_request`/`sgp_ranked_stats_shape`）。新日志里
   9 条 `overview_load_cost` 有 3 条是 `sgp_requests:0, sgp_bytes:0`，但
   `duration_ms` 高达 2340/3119/3701——这三条只可能来自韩服路径（国服每一条
   都有非零 `sgp_requests`），说明韩服总览常年在 2~4 秒量级，且完全没有可观测性。
3. `riot_api.go:227-289 getLimited`：命中 HTTP 429 时最多重试 3 次，每次按
   `Retry-After` 等待（最长 30 秒）——**单次接口调用最坏情况下可以卡 90 秒**。
   `loadRiotOverview` 对局详情是 **20 场、8 并发**（`gameplay.go:22 defaultMatchCount=20`），
   一旦触发限流，8 个并发 goroutine 同时排队，总耗时很容易超过前端 10 秒的超时。
4. 关键背景：这个 Riot API Key 是内嵌在 exe 里、**全体用户共享同一份配额**的个人 key——
   不需要这个用户本人操作频繁，只要同一时刻有其他人也在查韩服，配额就可能被打满。

超时后前端目前是能正确显示"战绩读取失败"+ 重试按钮的（已核对
`web/gameplay.js:713-722` 与 `loadOverview` 的 catch/finally，逻辑没问题，
**这部分不需要改**）。但用户反复点重试、每次都可能撞上同一限流窗口，
体感上等同于"一直加载"。国服总览在新日志里全部在 1.7 秒内完成，
**没有证据表明国服本身有问题，不要动国服的超时/并发参数**。

**要做**：

1. `loadRiotOverview` 补齐与国服对称的埋点：每次 `getLimited` 命中 429 时记一条
   `riot_rate_limited` 事件（`host`/`path`/`retry_after_ms`/`attempt`）；
   `loadRiotOverview` 结束时补一条 `riot_overview_cost`
   （`duration_ms`/`matches_requested`/`matches_loaded`/`rate_limited_count`），
   让日志里能直接看到韩服到底慢在限流还是别的地方。
2. `loadRiotOverview` 里对局详情的并发数从 8 **降到 3~4**，减少同一时刻打爆
   共享 Key 限流窗口的概率。
3. 前端把 `/api/gameplay/overview` 的超时**为韩服 tab 单独放宽到 20~25 秒**
   （复用已有的 `riotTab(tab)` 判定函数），国服 tab 保持 10 秒不变。

---

# G 国服他人胜率仍为「—」

新日志证据：`sgp_ranked_stats_shape.is_self=false` 共 153 条，其中仅 **4** 条出现
`losses:0` 而 `wins>0`（同时 `complete:false`、`suppressed:true`），其余 149 条都有正常
非零 `losses`。**确认是小概率上游问题**（SGP 对少数玩家不回负场），不是普遍故障。

**要做**：`sgp_ranked_stats_incomplete` 触发时（目前只是打点+压制显示为「—」），
改为**回退读 `season_stats.go` 的赛季聚合胜负数**再显示，而不是直接显示「—」。

---

# 已确认无需处理的两项（不要动）

- **排位召唤师技能**：新日志 13 条 `overview_loadout_shape` 全部 `spell1_present:20/spell2_present:20`，
  R13 的 `summoner1Id`/`spell1Id` 双字段兼容已生效，此项关闭。
- **场次最多的玩家只有 3 个**：新日志 12 条 `load_top_players_shape` 全部 `parsed:3`
  （不同英雄都一样），确认是上游 OP.GG 榜单本身只给 3 行，不是我们的解析/上限问题，
  **不要改 5 这个上限常量**。

---

# 验收清单

- [ ] A1/A3 详情页统计不再换行（`flex-wrap: nowrap`），每格高度明显下降
- [ ] A2 详情页左侧核心装单条路线与右侧四五六件每格**等高**，且路线行比现在略高
- [ ] A4 详情页四五六件显示 **胜率 + 场次**，不再出现「—」
- [ ] B1 对局内核心装路线**单行展示**，不换行
- [ ] B2 对局内行高与详情页一致（同一个 `--build-row-height`）
- [ ] B3 对局内四五六件显示 **胜率 + 场次**，不再出现「0%」；核心装路线仍是三列
- [ ] B4 对局内四五六件最多 **5** 件
- [ ] C 技能加点统计 = `option-stats` 结构，顺序 **选取率 → 胜率 → 场次**，三色与鞋子一致
- [ ] D 法外狂徒（mid）能看到核心装与鞋子；新增 `structured_metrics_gate` 埋点
- [ ] E 主副系符文图标与下方小符文等大，无遮挡；CSS 中无 >32px 的 `--rune-icon-size`
- [ ] F1 `loadRiotOverview` 补 `riot_rate_limited` 与 `riot_overview_cost` 埋点
- [ ] F2 韩服对局详情并发数从 8 降到 3~4
- [ ] F3 韩服 tab 的 `/api/gameplay/overview` 超时从 10 秒放宽到 20~25 秒（国服不动）
- [ ] G `sgp_ranked_stats_incomplete` 触发时回退读 `season_stats.go` 赛季聚合胜负数，不再直接显示「—」
- [ ] `go test ./...` + `node --test web/champions.test.cjs` 全绿；`gofmt -l` 只剩既有 `yourgg_arena.go`

**测试纪律**（本项目反复踩过，务必遵守）：

1. 跑测试用 `node --test web/champions.test.cjs`，**不能用 `node --test web/`**。
2. **每条新断言都要用变异体自验**：把对应代码改回坏的样子，确认测试真的 FAIL，再改回来确认变绿。
3. **布局类断言不要只断言"元素存在"**。本轮尤其要断言：
   `flex-wrap` 的取值、两侧共用的行高变量、统计列的**列数与顺序**、
   `--rune-icon-size` 的上界，而不是"复用了某个令牌""某元素存在"。
4. 变异测试拷副本时必须拷真副本（不能软链接源码），并带上 `desktop/`、
   `prestige_chromas.json`、`data/`；**跑变异体前先确认对照组是绿的**。
5. Go 里把条件改成 `if false` 会触发 "declared and not used" 编译错误 → 假杀，
   请改成逻辑等价但能编译通过的变体。
