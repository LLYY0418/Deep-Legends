# WORKLIST-R117：全项目优化（性能 / 加载 / 资源 / 数据准确性 / 交互 / 样式）

**撰写日期：** 2026-09-20　**基线版本：** 0.12.7（`desktop/package.json`）
**触发：** 用户要求「遍历整个项目，看性能、数据加载速度、资源加载、数据准确性、交互逻辑、样式统一等方面有没有可以优化的地方，不修改代码，生成工单」。
**本轮性质：** 纯审查，**未改动任何文件**。以下每条都带 `文件:行号` 证据，可被第三方独立复核。
**已做独立复核：** 初稿完成后由分离的验收方逐条打开引用行核对，修正了 7 处（含 1 条 P0 降级、1 条 P0 收窄、1 条强断言反证），详见第 7 节。

## 0. 三句话结论

1. **最严重的不是性能，是数据准确性。** 有 4 处把「不完整 / 语义不同 / 未披露」的值渲染成与真实数据视觉完全相同的数字，其中 P0-1 的降级说明写了、但前端读它的分支在填充后**永远进不去**，是死代码。
2. **性能侧只有一条真正会伤人的路径**：`riot_history_filter.go` 的「队列数 × 页数」串行放大，切一次筛选就是 9 次串行跨国请求，代码里没有任何上界。其余多数是「负结果 / 让路结果的缓存语义没定义」这同一族缺陷（与 R104 图标毒化同根）。
3. **R115 / R107 的两个结论经独立核验不成立**：一是「共享 CSS 被其他页面使用，保留静态加载」（实测三个文件里 90%+ 的规则首屏用不到）；二是 R107 把图片并发从 2 提到 6，而 R100 专门为此写的防饥饿断言 `peakImages<=2` 还留在仓库里、且从不在 CI 跑 —— 当年「并发 6」正是被杀死的变异。

---

## 执行纪律（全轮适用）

- 每项的「验收判据」都必须做**对抗变异**：把修复点改回原样或改成同义反复，对应测试必须 FAIL。只跑现有测试 PASS 不算验收（R102/R95 的教训）。
- **后端设置了降级说明 ≠ 已披露**。凡涉及降级、标注、空态的项，必须追到前端实际渲染点，并在验收里断言用户屏幕上的结果。
- 不新增上游数据源，不引入打包器 / CSS 预处理器 / 框架，不改传输层协议（h2c 见第 6 节）。
- 本工单条目**互相独立**，可按 P 级分批执行；若某条评估后认为不该做，在台账里写明理由与证据，不要静默跳过。

---

# P0 — 数据准确性红线（用户屏幕上正在显示错误数字）

## P0-1　赛季扫描的半成品战绩覆盖上游真实胜负场，且降级说明落在永不可达的分支

**证据**

`backend/gameplay.go:2814-2816`：
```go
ranks[index].Wins = season.Wins
ranks[index].Losses = season.Losses
ranks[index].WinRate = season.WinRate
```
数据来源 `backend/season_stats.go:477-486`：无论 `cache.Complete` 真假，**都返回 `cache.QueueStats`**，`Complete` 只被写进 progress，没有拦截数据。

`backend/gameplay.go:2909-2913` 的 `verifiedRankWinRate` 是专门为「只有 wins 没有 losses 不能显示成 100%」写的，返回 `-1, false`；上面这段正好把这个 `-1` 填掉。

前端 `backend/web/gameplay.js:2254`：
```js
const recordKnown = Number(rank.winRate) >= 0;
```
填充后变 true → 渲染 `N胜 M负 · 胜率 X%`，无 tooltip、无标记。而 `gameplay.go:2825` 设置的
`capability.Detail = "上游未返回排位负场，已按赛季战绩聚合补全胜率"` 在前端唯一读取点是 `gameplay.js:2257` 的 `!recordKnown` 分支 —— **正好是填充后永远进不去的那个分支**。

**影响**：用户看到「32胜 28负 · 胜率 53%」，实际是后台扫到一半的局数；而且上游本来正确的 `Wins` 也被改小了。这是本轮最严重的一条。

**修复方向**：(a) 只在 `cache.Complete` 时套用；(b) 填充后保留 `WinRate` 已知，但另加 `winRateDerived: true` 字段，前端走带说明的样式；(c) 不完整时保持 `-1`，走现有空态。

**验收判据**：构造 `Complete:false` 的 QueueStats + `wins>0, losses==0` 的 rank，断言渲染结果不含 `win-rate-value`，或含明确降级标记。**对抗变异**：删掉 `Complete` 判定，该测试必须 FAIL；把 `capability.Detail` 改成空串，必须有测试察觉（现在没有任何测试覆盖这条说明的渲染）。

---

## P0-2　赛季英雄 K/D/A 是整季累计值，却与同一张卡里的逐场平均值并排显示

**证据**

`backend/season_stats.go:190-192` 累加 `item.Kills/Deaths/Assists`；`backend/season_stats.go:342-350` 的 finalize **只把 CS 除以 Games**：
```go
if item.Games > 0 {
    item.WinRate = int(float64(item.Wins)*100/float64(item.Games) + 0.5)
    item.KDA = round2(ratio(item.Kills+item.Assists, item.Deaths))
    item.CS = round1(float64(item.TotalCS) / float64(item.Games))
```
而同卡的「全部英雄」行走 `seasonStatsOverall`（`backend/season_stats.go:377-383`）**是除过 Games 的**：
```go
result.Kills = round1(float64(totalKills) / float64(result.Games))
```
前端 `backend/web/gameplay.js:2285-2286`，两行用**同一个模板** `${number(item.kills)} / ${number(item.deaths)} / ${number(item.assists)}`。

另一条路 `backend/gameplay.go:3210-3212`（`aggregateMatches`）也是平均值，落进同一个 UI 槽位（`gameplay.js:1715` 三元选择 `seasonChampionStats` 或 `championStats`）。

**影响**：40 场的英雄行显示 `320 / 180 / 460`，紧挨着的「全部英雄」行显示 `8.0 / 4.5 / 11.5`。同一张卡两个量纲。
（注：`kda` 比值字段不受影响 —— 累计比与平均比数学等价，只有三个分项是错的。）

**修复方向**：`seasonStatsFinalize` 里把 K/D/A 与 CS 一样除以 `Games`；或前端按 `item.games` 归一（推荐前者，与 overall 口径对齐）。

**验收判据**：构造 2 场共 10 杀的赛季统计，断言 `kills == 5`。**对抗变异**：去掉除法恢复累计，测试必须 FAIL。另加一条断言：`seasonChampionStats[i]` 与 `seasonOverall` 的 K/D/A 量纲一致（overall 应约等于各英雄按场次加权平均）。

---

## ~~P0-3~~ → 降级为 **P3-8**：`ratePercent` 靠数值大小猜量纲（复核后确认现网输出正确）

> **独立复核结论：原 P0-3 的立论不成立，已降级。** 初稿据 `backend/champions_test.go:390` 的 `pick_rate:0.53` 推断会被放大成 53%，但**那条 fixture 根本不经过 `ratePercent`** —— 它属于 OP.GG 页面 Flight 的 `teamData`，由 `backend/champions.go:1985-1986` 原样透传（同载荷 `average_stats` 的 `win_rate 48.77 / pick_rate 13.85` 证明该端点是 0..100，透传正确）。
> 而 `ratePercent` 的实际上游是 OP.GG 结构化 JSON，fixture 见 `backend/champion_network_test.go:716`：`"win_rate":0.51, "role_rate":0.70, "pick_rate":0.03` —— **该端点是 0..1 小数**，`ratePercent(0.03)→3.0` 也正确。逐个核对当前全部 `ratePercent` 调用点，**没有一个上游是 0..100 量纲**。同理 `champions.go:1770` 的无条件 `*100` 与 `:1916-1917` 的 `ratePercent` 是两个不同端点，在 0..1 上二者恒等，「不可能都对」的推理是错的。

**保留的真实风险（P3 级，非现网故障）**

`backend/champions.go:1945-1950`：
```go
if value > 0 && value <= 1 {
    return value * 100
}
```
两个量纲不同的端点写进同一个 JSON 字段 `teamCompositions`（一条透传、一条过 `ratePercent`），而量纲由**函数内的数值启发式**而非来源契约保证。一旦将来把 0..100 端点接进 `ratePercent`，`pick_rate:0.53` 就会变成 53.00%，**且没有任何测试会发现**（`grep ratePercent backend/*_test.go` 零命中）。

**修复方向**：每个 parser 在入口处固定标注量纲，删除 `ratePercent` 本身。

**验收判据**：为两条路径各加一条量纲契约测试，断言 `teamData` 的 `0.53` 输出 `0.53`、`synergies` 的 `0.03` 输出 `3.0`。**对抗变异**：把任一路径的量纲处理对调，对应测试必须 FAIL。

---

## P0-4　职业页「最近对局」显示的是 OP.GG 资料修订时间，且合并规则保证永远选错

**证据**

`backend/pro_players.go:434-436` 把目录里的 `revision_at`（资料修订时间）`setProLastMatch(..., true)`。
`backend/pro_profiles.go:97` 用 `next.LastMatchAt > old.LastMatchAt` 取大值合并，而 `next` 才是 R107 引入的 JSON-LD 真实开局时间。`backend/pro_profile_activity.go:20-21` 的注释明写两者语义不同：`Its startTime is a game start, unlike initUpdatedAt (the page's last profile refresh)`（注意上游字段名在该处是 `initUpdatedAt`，与目录字段 `revision_at` 同义但不同名）。因为 `revision_at` 恒晚于开局时间，**取大值永远选中错的那个**。`pro_profiles.go:153` 的 worker 传的正是带 `revision_at` 的目录行，所以 `old.LastMatchAtKnown` 恒为 true。

**缺陷路径唯一（复核修正）**：只有在 `next`（JSON-LD）成功返回真实开局时间时，取大值才让 `revision_at` 胜出。若 JSON-LD 缺失，`pro_profiles.go:105` 的守卫 `!next.LastMatchAtKnown` 会触发 Riot 兜底，而 `setProLastMatch`（`pro_activity.go:39-44`）是**无条件覆写**，反而是对的。**修复时不要把这条唯一正确的路一起改掉**（初稿「挡住了 Riot 兜底」的说法已核实为错误，此处更正）。

量级实证（同一账号 `Kimman#zxfkk`）：`backend/r104_test.go:86` → `18:56:23Z`（revision_at）vs `backend/r107_test.go:26` → `18:19:58Z`（真实开局），差 **36 分 25 秒**。

**口径违约**：`docs/history/ledgers/r104-execution-ledger.md:24` 明确只授权 `revision_at` 作排序键、「不宣称是 Riot 精确对局开始时间」、且当时「不展示精确时刻」。R107 改成 `backend/web/pro-players.js:85` 显示 `最近对局 ${stamp(...)}` 精确到分，前提被推翻而后端没跟改。

**影响**：职业选手页每个账号的「最近对局」时刻系统性偏晚，最大观测偏差 36 分钟，且这是 R104 台账明文禁止的用法。

**修复方向**：`revision_at` 单独存 `directoryRevisionAt`，只参与排序；`lastMatchAt` 只接受真实开局来源（JSON-LD / Riot match-v5）；合并按**来源优先级**而不是取大值。

**验收判据**：目录 `revision_at` 晚于 JSON-LD `startTime` 时，断言 `lastMatchAt` 取 `startTime`。当前**无任何测试**覆盖 `old` 带 revision_at 的 `readProProfile` 合并路径。**对抗变异**：把合并改回取大值，测试必须 FAIL。

---

## P0-5　斗魂核心装的「位置回退等级」在斗魂详情页从不披露（复核后已收窄为披露缺口）

> **独立复核修正：下标推导本身是有意设计，不是无人看管的缺陷。** `backend/hexdata_test.go:465 TestLocalAugmentGradesAndMeasurementAreExplicit` 逐字钉死了它：
> ```go
> fallback := make([]championMetricRow, 9)
> applyLocalAugmentGrades(fallback)
> if fallback[0].Grade != "S" || fallback[1].Grade != "A" || fallback[3].Grade != "B" { ... }
> ```
> 同一测试还钉住配套文案「缺少统计指标时按推荐顺序回退」；`backend/web/champions.test.cjs:427/1201` 把 `applyLocalAugmentGrades` 的调用本身钉成不变量（:1201 是反向变异断言）。**因此「无统计量时返回空等级走 `—`」这个方向会直接打红三条现存测试，不要选它。**

**真正站得住的缺陷：说明写了但那条路径从不渲染**

`backend/champions.go:271-285`，`scores` 为空时按数组下标推导分位：
```go
percentile := float64(len(rows)-index) / float64(max(1, len(rows)))
```
调用点 `backend/champions_structured.go:1379-1381`（YOUR.GG 聚合失败时）附加了 `measurementTechnique` 说明。但 `renderMeasurementTechnique`（定义在 `backend/web/champions.js:1643`）只有 1018 / 1096 / 1129 三个调用点，**逐个确认全是 Mayhem/hexdata 路径，斗魂详情页一个都没有**。对比 `backend/hexdata.go:1984` 用同一函数、说明真的渲染出来了。

**影响**：位置推导出来的 S/A 徽章与真实上游等级视觉完全相同，而后端写好的披露文案在斗魂页**永远不会显示**。与 P0-1 是同一类缺陷（后端降级说明未追到渲染点）。

**修复方向**：把 `renderMeasurementTechnique` 接到斗魂详情页。**不要**改 `applyLocalAugmentGrades` 的回退行为。

**验收判据**：构造 YOUR.GG 聚合失败的斗魂详情响应，断言页面上出现 `measurementTechnique` 文本。**对抗变异**：摘掉新增的渲染调用，测试必须 FAIL；同时 `hexdata_test.go:465` 与 `champions.test.cjs:427/1201` 必须保持绿色（证明没有顺手改回退逻辑）。

---

## P0-6　README 关于会话令牌的两处陈述与代码相反（隐私声明一致性）

**证据**

`README.md:15`：
> 启动握手使用**每次随机生成**的会话令牌；令牌**只保存在进程内存中，不写入磁盘**，也不会上传服务器。

`backend/main.go:1657-1681`（本人已直接复核）：
```go
// sessionTokenFileName 保存本地界面的会话令牌。跨重启复用同一令牌，
// 浏览器里已授权的页面在服务重启后依然有效，不会整页退回 401。
const sessionTokenFileName = "session-token"
...
if err := atomicWriteFile(path, []byte(token+"\n"), 0o600); err != nil {
```
常量注释自己写着「跨重启复用同一令牌」，并明文落盘到 `session-token`。**README 的两句话都是事实错误**。

缓解事实（应一并写入）：文件权限 0600、数据目录 0700、仅回环监听、令牌不上传。`features.go` 的机读 `neverStores` 未覆盖此项（第 2 条只针对 LCU 令牌），所以是 README 错而非机读声明错。

**影响**：隐私承诺与实际写操作不一致 —— 这是项目明文红线（AGENTS.md「隐私声明必须与实际写操作一致」）。

**修复方向**：改 README 描述实际行为（含 0600/0700/回环/不上传的缓解说明）。**不建议**反过来改代码为纯内存态 —— 那会让重启后页面整页 401，是有意的设计权衡。

**验收判据**：加一条测试锁住二者一致（例如断言 README 不含「不写入磁盘」且含 `session-token` 文件名）。**对抗变异**：把 README 改回原文，测试必须 FAIL。

---

# P1 — 性能、配额与加载

## P1-1　分模式战绩筛选把一次翻页放大成「队列数 × 页数」次串行 Riot 请求，代码内无上界

**证据** `backend/riot_history_filter.go:46-60`：
```go
for _, queue := range queues {
    for offset := 0; offset < start+count; offset += 100 {
        page, err := p.matchIDsFiltered(ctx, puuid, offset, 100, queue, "")
        if err != nil { return nil, err }
        for _, id := range page { ids[id] = struct{}{} }
        if len(page) < 100 { break }        // :56 实际上界所在
    }
}
```
队列族大小见 `backend/queue_groups.go:87-131`（已逐个核对）：`arena` 9 个、`more:bots` 8 个、`more:match` 4 个、`more:aram` 3 个、`hextech-aram` 3 个、`ranked` 2 个（420/440 在 :35 特判）。

放大算式（实体数 × 每实体请求数）：
- **下界（代码严格支持，无需假设）**：切到「斗魂」筛选第 1 页 = **9 次串行 ID 请求**（"all" 只要 1 次）。仅此一条即足以支撑 P1 定级。
- **理论上界 909**：`begIndex` 钳到 10000（`gameplay.go:31,8653`）、`count` 钳到 50（`gameplay.go:29`）→ `9 × ceil(10050/100)`。实务上受 `:56` 的 `len(page)<100` 提前 break 约束（arena 9 个队列里 1701/1731/1732 是单人训练，多数玩家第一页就 break），**不要把 909 当成现网预期值**。
- 前端可滚到 `begIndex=500`（`backend/web/gameplay.js:164 MAX_BROWSE_MATCHES=1000`），此时 `offset < 520` → 每队列最多 6 页 × 9 = 54 次（不计 break 与缓存）；滚动是自动追加的（`gameplay.js:661-676`，间隔仅 `AUTO_PAGE_DELAY_MS=400`）。**实际值取决于该玩家在各队列的历史长度，未实测。**

本地限流 90 次/2 分钟（`riot_api.go:290-293`）。三点加重：①完全串行，无 errgroup/信号量，多次跨国往返串在首屏关键路径（调用在 `riot_api.go:1219`，goroutine 为 :1216-1221）；②`matchIDsFiltered` 缓存 TTL 只有 1 分钟（`riot_api.go:814`）；③中途 throttle 时 `return nil, err` 让整页作废，但配额已经烧完。

**影响**：韩服玩家页切「斗魂/人机/匹配」筛选后首屏多等 2~4 秒；往下滚 2~3 屏触发额度提示。该 key 全员共享，一个用户能把所有人锁 2 分钟。

**修复方向**：多队列的 offset 分页改并发 + 给 `queues × pages` 设硬上限（建议总 ID 请求 ≤ 12）；合并后的 ID 列表按 `puuid+filter` 缓存，而不是按 offset 分片缓存。

**验收判据**：用假 `p.get` 计数器断言 `matchIDsForOverview(ctx, puuid, 500, 20, "arena")` 发出的上游请求数 ≤ 上限常量。**对抗变异**：调大上限常量或改回串行，测试必须 FAIL。只断言「返回了正确的 ID 列表」抓不住。

---

## P1-2　图片队列并发 6 正好吃满浏览器 HTTP/1.1 单源上限，而 R100 专门为此写的防饥饿断言仍是 `peak<=2`、且从不在 CI 跑

**证据（本人已直接复核全部四处）**

生产值 `backend/web/image-queue.js:4`：
```js
const limit = 6, pending = new Map(), active = new Map(), failed = new Map();
```
`docs/r107-execution-ledger.md:10` 明确记载这是 R107 从 2 提到 6 的：「现在并发 6、前端 10 秒…」。

而 R100 的防回归脚本 `desktop/r100-browser.cjs:67` 至今写着：
```js
assert.ok(peakImages<=2,'browser image connection cap exceeded: '+peakImages);
assert.ok(elapsed<8000,'status starved behind images: '+elapsed);
```
更关键：`docs/r100-execution-ledger.md:101` 记载「变异明细：…**P1图片并发6**…均由实际行为断言失败杀死」—— 也就是**当年把 limit 设成 6 就是被杀死的变异，现在它成了生产值**。

同时 `backend/web/r100.test.cjs:44` 已被改成 `assert.equal(imgs.filter(i=>i.hasAttribute('src')).length,6)`，把断言方向调转到 6。

CI 只跑 `node --test backend/web/*.test.cjs desktop/*.test.cjs`（`.github/workflows/ci.yml:40`），`*-browser.cjs` 是手动脚本（`desktop/r100-browser.cjs:2` 的注释即为手动用法），**从不在 CI 执行**，所以这条自相矛盾没有被任何流程发现。

R107 的验证目录 `docs/r107-validation/` 有 `browser-images.json`、`live-images.log`，**没有 r100-browser 的复跑产物**，也没有任何「6 路图片在途时 /api/status 耗时」记录。

**影响**：`/api/image` 集体变慢时（冷 CDN、离线、代理超时），6 个在途图片重新占满 Chromium 对 `127.0.0.1` 的 6 个 HTTP/1.1 连接，`/api/status` 排不进去 → R100 记载的「顶部一直显示重试条」可复现。阻塞上界 8 秒（服务端 `champion_images.go:15 publicImageTimeout`）/ 10 秒（前端 `image-queue.js:105`）。这是**未验证是否已复发**，但护栏确实已失效。

**修复方向**（三选一，必须在台账里写清选了哪个及理由）：
1. `limit` 降到 4~5，给 API 永久留槽（最低风险，代价是图片吞吐下降）；
2. 在 `pump()` 里对 API 请求保留 1 个空位；
3. 保持 6，但**必须**给出 6 路在途时 `/api/status` 端到端耗时的真机证据，并同步修正 `r100-browser.cjs:67` 的断言值 —— 不能让仓库里留着一条与生产相反的护栏。

**验收判据**：`desktop/r100-browser.cjs` 的 12 个挂起图标 fixture 在当前 `limit` 下复跑通过，且 `peakImages` 断言值与生产 `limit` 一致（可直接从 `image-queue.js` 解析，避免再次漂移）。把该脚本接进 CI（或至少接进发布门禁）。**对抗变异**：把保留槽逻辑删掉，浏览器用例必须 FAIL —— 现有 jsdom 用例断言的是「6 张同时有 src」，删掉保留槽它照样绿。

---

## P1-3　图片「10 分钟成功 URL 缓存」命中时完全绕过并发闸门

**证据** `backend/web/image-queue.js:24-42`：
```js
if (loadedURLs.has(url) && Date.now() - loadedURLs.get(url) < 600000) {
  ...
  img.loading = "eager";
  img.src = url;
  return;                 // 没有 pending.set / 没有 active.set / 没有 pump()
}
pending.set(img, url); pump();
```
`loadedURLs` 上限 2048（`image-queue.js:91`）。命中只证明「本进程内 10 分钟前成功过」，不证明浏览器磁盘/内存缓存还在。

**影响**：重新进入访问过的页面（重开英雄详情、重进收藏格子）时，几十到上百张图同帧 `img.src = url` 全部绕过队列；浏览器缓存未命中即是一次不受限的连接风暴，直接放大 P1-2。

**修复方向**：命中分支也走 `pending.set(img, url); pump()`，只是在 `pump()` 里给已知成功 URL 更宽松的配额或跳过 10 秒超时，而不是完全不占名额。

**验收判据**：先让 30 张图成功填充 `loadedURLs`，移除后重新挂载同样 30 张，断言任一时刻已 set src 且未 ready 的图片数 ≤ limit。**对抗变异**：恢复「命中直接 set src」，断言必须 FAIL。

---

## P1-4　「负结果 / 让路结果」缓存语义缺失族（四处同根，建议一并处理并沉淀为规则）

这四条与 R104 图标毒化是**同一个根因类别**：把「主动让路」「确认为空」「瞬时失败」「被取消」混为一谈，导致要么永久毒化、要么永不缓存。

**(a) 后台配额让路被当成失败，把职业选手档案 TTL 从 15 分钟塌成 30 秒**
`backend/pro_profiles.go:25-33`：
```go
if row.CheckFailed || row.ActivityFailed { return 30 * time.Second }
```
`backend/pro_profiles.go:105-116` 把 `activityErr != nil` 直接写成 `ActivityFailed`，而 `backend/pro_refresh.go:42-48` 的 `errThrottled` 是**主动让路**：
```go
if len(p.limitQueue) > 0 || len(p.shortWindow) >= 15 || len(p.longWindow) >= 30 {
    return fmt.Errorf("%w: background yields to foreground quota", errThrottled)
}
```
注意 `len(p.limitQueue) > 0` 意味着**只要有一个前台韩服查询在排队，53 个账号的活跃度探测全部被拒**。`ActivityFailed` 带 json tag（`pro_players.go:61`）会持久化，`pro_profiles.go:130/136` 的磁盘写入用的同样是 `proProfileTTL(old)`，内存与磁盘双双 30 秒过期（`StaleUntil` 仍是 7 天，所以不是完全失效，是降级到 stale 路径）。
→ 修复：`errors.Is(err, errThrottled)` 时保留上次 TTL，只标记活跃度待补；活跃度 TTL 与档案 TTL 拆开存。

**(b) OP.GG 历史段位「确认为空」不入缓存，每次总览首屏都重下一次玩家页**
`backend/opgg_insights.go:197-221` 只有 `if len(ranks) > 0` 才写缓存；`opggHistoryCacheTTL = 30min` 只对成功结果生效，唯一护栏是并发去重（`opgg_insights.go:248-252`），没有时间维度的负缓存。触发点 `riot_api.go:1257-1258`（`begIndex == 0` 时每次总览都调 `startOPGGHistoricalRanks`）。
→ 修复：`len(ranks) == 0` 也写一条短 TTL（约 10 分钟）空条目，并区分「确认无历史」与「抓取失败」。

**(c) 挑战目录一次瞬时失败就永久毒化，且持锁跨 LCU 请求**
`backend/profile_facade.go:259-278`，`Attempted` 在请求**之前**置 true，失败后只有客户端切换（`connection_manager.go:781/801`）才复位；`facadeChallengeCatalogMu` 覆盖整个 LCU 请求（超时 8 秒，`lcu.go:686`）。
→ 修复：成功后才置 `Attempted`，失败走带 TTL 的短退避；请求移出锁，用项目已有的 flight 范式（`asset_cache.go` / `champion_cache.go`）。

**(d) 绝活哥符文 throttled/timeout 不写负缓存，形成「配额越紧→越不缓存→请求越多」正反馈**
`backend/specialist_runes.go:218-228` 只在 `len(result) > 0 || outcome == specialistOutcomeNoPositionSample` 时写缓存；单次预算 36 次**前台** Riot 请求（`specialist_runes.go:16-21`，未加 `withRiotBackground`）。现存缓解是渲染层 60 秒冷却（`gameplay.js:705-708`）+ 3 个 flight 槽，但「重试」按钮（`gameplay.js:5983`）和换局都能绕过。
→ 修复：throttled/timeout 也写 60~120 秒负缓存，把冷却变成服务端事实。

**验收判据（四条共用范式）**：注入「恒返回让路/空/失败」的 loader，连续调用两次，断言第二次上游请求数为 0（或 TTL 未塌缩）。**对抗变异**：删掉新增的判据分支，测试必须 FAIL。

**沉淀要求**：在 `DESIGN.md` 里补一节「负结果缓存语义」，明确四种结果（成功 / 确认为空 / 主动让路 / 失败）各自的缓存与重试策略，后续新增上游链路必须对号入座。

---

## P1-5　客户端未运行时，总览每 3~8 秒重建一次 + 一次多余 HTTP + 一次入口卡闪烁

**证据（三处叠加）**

后端每轮广播 **2 次** `connection-state`，退避上限只有 8 秒：
`backend/connection_manager.go:25-26` `minimumDiscoveryBackoff=3s / maximumDiscoveryBackoff=8s`；
`backend/connection_manager.go:147-152` 先 `setConnectionPhase("connecting")` 再 `markDisconnected()`，`broadcastEvent`（`live_updates.go:91`）无去重。

前端 A：`backend/web/app.js:448` `if (!state.status.connected) await loadClientInstallations();`，而 `app.js:769-774` 每次都先 `state.installationsLoaded = false; renderLaunchpad(...)`，`app.js:823-826` 随即把列表塌成 `正在检查 TCLS 与 Riot 客户端…`。

前端 B：`backend/web/gameplay.js:451-482` 的 `updateStatus`，注释自己写着「周期性的未连接状态事件不能清空已加载的 Data Dragon 兜底」，但 `wasConnected` 守卫**只加在目录上**，`resetTencentTabsAfterDisconnect()` 和 `renderPlayerTabs/renderOverview/renderLive/renderOverlay` 四个全量渲染每次都跑。

**影响**：没开游戏停在总览时，启动入口列表每 3~8 秒塌成一行灰字再变回按钮；同时总览/对局/覆盖层 DOM 被整体重建。如果用户开着韩服页签，重建的是完整战绩页而非空态。

**修复方向**：(a) `loadClientInstallations` 加「上次结果 + TTL/指纹」短路，或只在 `connected` 发生跃迁时调用；(b) `updateStatus` 的 `!status.connected` 分支整体加 `if (!wasConnected) return;`；(c) 可选：后端同一相位不重复广播。

**验收判据**：jsdom 连投 5 次内容完全相同的 `connection-state`（假计时器），断言 `/api/client-installations` 只 fetch 1 次、`renderOverview` 只被调用 1 次、`正在检查 TCLS` 从未出现第二次；再投 `connected:true` 后再投 `false`，计数必须变成 2。**对抗变异**：把守卫写成恒 true（永不执行），「首次就断开」用例必须 FAIL。

---

## P1-6　SSE 路径不检查 `document.hidden`，窗口隐藏时仍在轮询

**证据**：定时器路径有守卫（`backend/web/app.js:408-412`）、`visibilitychange` 也只管定时器（`app.js:3256`），但 SSE 绕过它：`app.js:3377` → `queueLiveUpdateSlices` → `flushLiveUpdateSlices`（`app.js:3262`）→ `refreshStatus`，而 `refreshStatus`（`app.js:414-416`）只检查 `state.destroyed || state.backendExited`。

**影响**：窗口最小化 / 切后台且客户端未运行时，仍每 3~8 秒发 `/api/status` + `/api/client-installations`（叠加 P1-5），Electron 后台窗口持续唤醒。

**修复方向**：`flushLiveUpdateSlices` 在 `document.hidden` 时把 slices 留在集合里不消费（`state.liveUpdateSlices` 是 Set，天然可积累），恢复可见时统一 flush。

**验收判据**：设 `document.hidden=true` 投 3 次事件，断言 fetch 0 次且 slice 仍在集合里；触发 `visibilitychange` 后恰好 1 次 fetch。**对抗变异**：改成「隐藏时直接丢弃 slices」，恢复可见后必须有用例 FAIL。

---

## P1-7　韩服总览每个进度帧都全量重建并重新序列化整个生涯栏，只为发现「没变」

**证据**：后端每完成 2 场详情推一帧（`backend/riot_api.go:1345-1355`），20 场约 9~10 帧。前端每帧 `rerenderTab` → `renderOverviewBodyContent`。战绩列表做了保留（`gameplay.js:1778-1782` 的 `preserveMatchList`），**生涯栏没有**：
```js
// gameplay.js:1810
const careerSections = renderCareerSections(data, tab);       // 无条件构串
// gameplay.js:1825-1833
container.innerHTML = `... ${careerSections} ...`;             // 无条件解析回 DOM
// gameplay.js:1842-1843
const careerChildHTML = newCareerChildren.map(child => child.outerHTML);  // 再序列化回字符串
// gameplay.js:1862-1864
if (container._careerChildHTML?.[index] === careerChildHTML[index]) newCareerChildren[index]?.replaceWith(child);  // 发现相同，丢弃
```
构串 → 解析 → 再序列化 → 发现相同 → 换回旧节点，三倍无用功 × 约 10 帧。

**影响**：查询韩服玩家时主线程被反复占用，生涯栏（段位/英雄统计/活跃时段图）在弱机上可见卡顿。这是历史「韩服慢」里未被拆掉的一个成分。

**修复方向**：给生涯栏引入独立的 `careerRevision` 数据签名（由 ranks/masteries/championStats/activityHours 的引用变化决定），未变时直接走保留路径，不构串不解析不序列化。

**验收判据**：monkey-patch `renderCareerSections` 计数，用同一份 data（只改 `matches`）连投 5 帧，断言计数 = 1，且 `.career-column` 节点是同一对象。**对抗变异**：把签名固定为常量（永远命中），「ranks 真的变了」用例必须 FAIL。

---

## P1-8　首屏仍静态加载三个懒加载页专属 CSS —— R115 的「共享 CSS 被其他页面使用」经两路独立核验不成立

**证据**

`backend/web/index.html:13-16` 仍有：
```html
<link rel="stylesheet" href="/champions.css">     <!-- 86,821 B -->
<link rel="stylesheet" href="/pro-players.css">   <!-- 10,958 B -->
<link rel="stylesheet" href="/suite.css">         <!-- 54,038 B -->
```
而对应 JS 已由 `backend/web/section-loader.js:4/17` 按需注入（注意 `index.html:13-16` 这四行里第 15 行是 `friends.css`，三个目标文件并不连续；`friends.css` 必须留在首屏，因为 `friends.js` 在 `index.html:25` 是急加载的）。

两路独立审查 + 一轮独立复核，逐规则核对首屏 JS（app/gameplay/friends/overview-art/augment-artwork）与 `index.html` 常驻标记的实际类名使用：

| 文件 | 规则总数 | 首屏真正需要 | 可下沉 |
|---|---|---|---|
| champions.css | 783 条 / 86,821 B | **约 44 条 / 约 4.6 KB** | ~95% |
| pro-players.css | 124 条 / 10,958 B | **约 10 条 / 约 1.0 KB** | ~91% |
| suite.css | 497 条 / 54,038 B | **37 条**（复核更正，初稿误写 0 条） | ~93% |

champions/pro-players 侧首屏真实依赖只有三小簇：`.mayhem-ranking-list` / `.recommendation-section > header`（被 `gameplay.js:5720` 的对局页装备排行复用，基础定义只在 champions.css）、`.tier-badge*`（`gameplay.js:5323-5324`）、`.region-menu-pro`（`index.html:88`）与 `.pro-verification-warning` / `.pro-player-name`（`gameplay.js:1306/5473`）。

**suite.css 的 37 条必须保留在首屏（复核更正）**：`index.html:255-281` 有**完整的 suite 面板静态外壳** —— `.panel.suite-panel`（:255）、`.suite-subpanel`×5、`.suite-loading`×5、`.suite-tab`/`.suite-tab-icon`（:258-261）、`.suite-head`、`.suite-metrics`、`.suite-offline`/`.suite-offline-mark`。其中 **`.suite-panel` 的 `container-name: suite-page; container-type: inline-size` 是整个 suite.css 的容器查询锚点**，下沉走会让 7 条 `@container suite-page` 全部失效，且在「点工具页签 → `hidden` 摘掉 → section-loader 注入 CSS 完成」之间出现一帧无样式外壳（这个外壳不是 suite.js 渲染的，是静态 HTML）。

初稿另一句「496 条选择器全在 `#suite-panel` 内」**是错的**：`grep -c "#suite-panel" suite.css` = 2，且都不是选择器作用域；全部规则都是裸类选择器，无 ID 作用域 / 无 CSS nesting / 无 `@scope`。成立的只有后半句——**无全局元素级规则**（所有选择器片段以 `.` 或 `button.` 开头），所以下沉后不会影响其他页面。

**注意**：`backend/web/r115.test.cjs:13` 目前**反向锁死**了这条：`assert.match(html,/<link rel="stylesheet" href="\/champions.css"/)`。执行本项时必须同步改这条断言，否则测试会红。

**影响**：8 个渲染阻塞样式表共 420,199 B / 3,675 条规则，其中 1,440 条（39%）属于用户可能整场不打开的三个页面，却参与每次 innerHTML 重建后的样式匹配。诚实评估：**回环下载耗时可忽略（gzip 后约 37 KB），真实成本是 151,817 B CSS 的解析与样式计算**，验收口径应为「首次绘制前主线程占用」，**不要以「首屏快几秒」验收，一定会得出没效果的结论**。

**修复方向**：三个文件各拆成「共享小片」（合计约 6.6 KB，并入 app.css 或新建 shared-blocks.css）+「页面片」；页面片由 `section-loader.js:12 load()` 在注入 `/${name}.js` 时一并注入 `<link>`，并 `await` 其 load 事件后再回调，避免无样式闪烁。

**验收判据**：(1) `index.html` 的 stylesheet link 从 8 降到 5；(2) 真 Chromium 导航到英雄页，断言切换前 `link[href="/champions.css"]` 为 null、切换后存在，且 `[data-section-loading]` 移除后 `.champion-table` 的关键计算属性已生效（证明 CSS 真到位而非靠继承蒙混）；(3) 点击「工具」页签，断言 `#suite-panel` 在 `[data-section-loading]` **期间** `container-type` 已是 `inline-size`（即共享片已在首屏生效，而不是等 suite.css 注入后才有）；(4) 逐帧检查切换瞬间无无样式帧。**对抗变异**：删掉共享片里的 `.mayhem-ranking-list` 规则，对局页装备排行的几何断言必须 FAIL；删掉 `.suite-panel` 的容器声明，(3) 必须 FAIL —— 否则说明共享片划分只是摆设。

---

## P1-9　rank-crests / loot-icons 的 PNG 是显示尺寸的 6~7 倍，内嵌资源可省约 1.67 MB

**证据（实测）**

```
rank-crests/  10 个文件  1,479,127 B   challenger.png  500x500  211,869 B
loot-icons/    6 个文件    943,290 B   hextech-chest-transparent.png 512x512 265,261 B
```
全项目最大显示尺寸：`backend/web/gameplay.css:223` `.summoner-highlight-rank .rank-crest-icon { width:78px; height:78px; }`；其余为 44px（gameplay.css:345）、28px（pro-players.css:56）、19px（gameplay.css:681）、18px（champions.css:614）。loot 侧 `app.css:672 .loot-art{70px}` × `app.css:676 --loot-icon-scale:1.65` → 有效最大约 116px。

即 500px 的图渲染成 78px（2x DPR 也只需 156px）。降采样实测（Pillow LANCZOS + optimize）：
```
保守（rank-crests@192 / loot-icons@256，留 2.2~2.4x DPR 余量）   省 1,755,081 B (1.67 MB)
激进（rank-crests@160 / loot-icons@192，精确 2x DPR）            省 1,985,200 B (1.89 MB)
```
这两个目录占全部内嵌资源 4,278,568 B 的 **57%**。PNG 不走 gzip（`static_assets.go:52-53` 只压 js/css/html/svg，这是正确的），所以磁盘体积 = 传输体积，省的是双份。

附带：`backend/web/app-icon.png` 256x256 / 95,908 B，无损 `optimize=True` 重存 → 81,796 B，**省 14,112 B**，像素零变化。

**修复方向**：离线批量降采样重新落盘（一次性替换文件，不引入构建期依赖）。建议 rank-crests@192、loot-icons@256。

**验收判据**：`rank-crests` + `loot-icons` 总和 < 700 KB，每张 PNG 宽高 ≤ 256；2x DPR 下 `.summoner-highlight-rank .rank-crest-icon`（78px）截图与改前 diff 无肉眼可见退化。app-icon 断言 < 85 KB 且 `ImageChops.difference` 全零。

---

# P2 — 交互可见性与错误可操作性

## P2-1　好友面板刷新失败时静默显示过期数据，`state.stale` 是只写不读的死标志

**证据** `backend/web/friends.js`：`:29` 声明 `stale: true`，`:141` 成功时置 false，`:380`/`:401` 失败前置 true —— 但 `render()`（`:250`）、`updateSummary()`、`updateBadge()` **全都不读它**。同时 `:301` 有 `setInterval(updateDurations, 1_000)` 在持续走表。`render()` 唯一的错误分支要求 `!state.friends.length`。

**影响**：断流或接口报错时，只要本地还有上一次列表，面板不显示错误、不显示过期、没有重试入口，而且好友的「对局中 03:41」继续每秒 +1。对照 `backend/web/pro-players.js:124` 同场景写了「刷新失败，当前显示上次读取的数据，不代表最新账号、段位和排名。」—— 项目已有约定，friends.js 是唯一违反的。

**修复方向**：`render()` 读 `state.stale`/`state.error`，列表顶部挂过期提示条（含重试）；失败时 `stopTick()` 或把时长改为静态。

**验收判据**：mock 第二次请求返回 500，面板必须出现过期提示且时长停止跳动。**对抗变异**：删掉 render 里的 stale 判据，测试必须 FAIL。

---

## P2-2　埋点缺口：相位轮询失败零埋点，四个前端文件整体零埋点

**证据**

(a) `backend/web/gameplay.js:6836-6853` 的 gameflow 相位轮询：
```js
.catch(() => {})                                  // 6852
.finally(() => scheduleBeaconPoll(...));
```
`recordLiveObservation` 有 `received`/`stale-response`/`invalidate` 三个态（`gameplay.js:4253/4277/6800`），**唯独失败没有态**。这是 R94 那类「相位没跟上」问题的主探针，8 秒超时后静默重排，用户导出的诊断日志里看不到任何轮询失败记录。

(b) `grep -c "reportFlowDiagnostic" backend/web/*.js`：app.js 2、gameplay.js 8、suite.js 3、runtime.js 1，而 **champions.js 0、pro-players.js 0、friends.js 0、image-queue.js 0、section-loader.js 0**。且 `pro-players.js:124` 的 `catch (_)` 把 error 对象整个丢掉，网络错误 / HTTP 500 / schema 不合法三种原因坍缩成同一句话。

**影响**：英雄页（2420 行，全依赖 OP.GG/hexdata 上游）和职业选手页出问题时诊断日志里完全没有痕迹；R104 的图标全白正是 image-queue.js 域内的问题，该文件至今零埋点。直接违反项目原则「排障场景把能想到的地方都补上日志」。

**修复方向**：`.catch((error) => recordLiveObservation("poll-failed", ...))` 带 errorKind/httpStatus；四个文件各自在 `api()` 的 catch 里补 `local_request_client` 级别 channel；pro-players 保留 `error.message`。

**验收判据**：`/api/gameplay/phase` 返回 503 时必须收到一条 `poll-failed`；断网后进入英雄页，导出诊断日志必须含至少一条 champions 相关失败记录。**对抗变异**：删掉埋点，测试必须 FAIL。

---

## P2-3　「应用装备方案」存在点了什么都不发生且无埋点的路径，与同屏「应用符文」行为不一致

**证据** `backend/web/gameplay.js:6067-6076`：
```js
if (!self || !build) return;                                  // 静默
const payload = buildItemSetPayload(build, self, state.live);
if (!payload.championId || !payload.blocks.length) return;    // 6074 静默
...
recordItemSetClientDiagnostic("item_set_apply_request", "submitted", ...);   // 埋点在 guard 之后
```
按钮 enable 条件（`gameplay.js:5683`）只覆盖两个条件里的一个：
```js
const canApply = itemSet.blocks.length > 0 && data?.phase === "ChampSelect";
```
`championId` 来自 `liveRecommendationChampionId(self) || Number(data?.currentChampionId) || 0`（`gameplay.js:5748/4328`），在英雄交换/取消锁定瞬间可以为 0，而 `blocks` 来自上一份缓存的 build 仍非空 → 按钮可点、点了无反应、无 toast、无埋点。

对照 `gameplay.js:6046-6052` 的 `applyRunes`，同类 guard 全部配了 toast。这是不一致，不是有意设计。

**修复方向**：guard 里加 `showToast` + `recordItemSetClientDiagnostic(..., "skipped", {reason})`；或把 `canApply` 与 handler 判据对齐（同时检查 championId）。

**验收判据**：构造 `championId=0` 且 build 有 blocks 的 live 状态，点击按钮必须出现提示**或**按钮为 disabled；两者都不满足则 FAIL。

---

## P2-4　二次确认通过后，并发保护把销毁性操作静默丢弃

**证据** `backend/web/suite.js:1731-1735` 的确认流程调用 `applyFacade(...)` 但忽略返回值，而 `suite.js:1765`：
```js
async function applyFacade(body, success = "生涯设置已应用") {
  if (state.facadeApplying) return;      // 返回 undefined，无 toast
```
背景应用、头像应用、登录重设保存共用这把锁。

**影响**：用户点「卸下全部勋章」→ 读确认文案 → 点确认，如果此刻另一个 facade 写入在途，整个操作被丢弃且屏幕上没有任何变化，用户会认为已经执行。同一把锁还让 `suite.js:1389` 的头像 apply 在重入时走进「客户端尚未确认更改」这个误导性分支（`ok` 是 undefined 而非 false）。

**修复方向**：重入分支 `toast("上一个生涯写入尚未完成，请稍候")` 并返回可区分的值；确认按钮在 `facadeApplying` 时渲染为 disabled。

**验收判据**：置 `state.facadeApplying = true` 后走完确认流程，必须出现提示 toast。

---

## P2-5　三处请求层的错误语义缺陷

**(a) suite.js 的超时文案是「未领取奖励」专用，换页语义错误**
`backend/web/suite.js:162` 硬编码：`"请求超时，可重新扫描后重试；本项失败不影响其它条目。"`，而调用方包括 `api("/api/watch/rules")`（:506）、`api("/api/rig/status")`（:1291）、`api("/api/facade/apply")`（:1765）、`api("/api/champselect/groups")`（:871），失败后 `toast(error.message)` 原样弹出。保存自动规则超时的用户会看到「可重新扫描后重试；本项失败不影响其它条目」——「扫描」「条目」在那个页面不存在。
→ 修复：`api()` 加 `hint` 形参，默认文案中性化，奖励扫描处传专用后缀。

**(b) suite.js 把无法解析的 200 响应当成合法空配置**
`backend/web/suite.js:150-158`：`try { payload = text ? JSON.parse(text) : {}; } catch (_) {}` → `return payload ?? {}`。`loadWatch`（:477）随后 `state.watch = response; renderWatch();`。后端返回 200 但 body 损坏时，自动规则页渲染成空配置，用户会认为规则被清空，而这时任何一次 toggle 都会把空配置 POST 回去。
→ 修复：解析失败且 `response.ok` 时抛显式错误（「响应格式异常」），不要落到 `?? {}`。

**(c) champions.js 把主动取消抹平成「联网读取超时」**
`backend/web/champions.js:238-256`：`:247` 刚造出 `AbortError` 取消哨兵，`:252` 就 `throw new Error("联网读取超时，请重试")` 把它抹平。其余三个模块都有独立的 `RequestCancelled` 名字并逐处检查（`app.js:385`、`gameplay.js:256`、`suite.js:168`）。而且请求打的是 localhost，「联网读取超时」会把用户引到「是不是我网断了」的错误方向。
→ 修复：取消哨兵改用 `RequestCancelled` 名字并在 :252 放行；超时文案改「本地请求超时，请重试」。

**验收判据**：(a) 在 watch 保存路径注入超时，toast 不得含「扫描」或「条目」；(b) mock 200 + 非 JSON body，`loadWatch` 必须进 errorCard 分支；(c) 连续两次调用同 key 的 `api()`，第一个 promise 的 `error.name` 必须是 `RequestCancelled`。

---

## P2-6　`finally { if (!current()) return; loading = false }` 留下可卡死的转圈

**证据** `backend/web/champions.js:420-427`、`:449-452`、`:673-677`：
```js
} finally {
  if (!current()) return;          // 陈旧请求直接跳过，state.loading 永不复位
  state.loading = false;
```
`loadMayhemAugmentDetail`（`:673`）的 `current()` 含 `state.mayhemView === "atlas"`（`:663`）。用户点开某个海克斯后立刻切走图鉴视图，响应回来时 `current()` 为 false → `mayhemAugmentLoading` 永久停在 true。

**是否存在真正永久骨架屏的返回路径未验证**（依赖 `mayhemAugmentDetailCache` 命中与否），但这个形状本身就是「总览永久骨架屏」的同一类。

**修复方向**：`finally` 里**无条件**复位自己这一轮的 loading，用 token 比对决定是否 `render()`，而不是决定是否复位。

**验收判据**：先请求 augment A，再改 `state.mayhemView`，响应返回后 `state.mayhemAugmentLoading` 必须为 false。

---

## P2-7　两个选择器弹窗的错误兜底会在渲染阶段抛错时二次崩溃

**证据** `backend/web/suite.js:1399` 与 `:1448`：
```js
} catch (error) {
  if (dialog.open) dialog.querySelector(".facade-picker-loading").textContent = `${error.message}。请关闭后重试。`;
}
```
该节点在 `suite.js:1330` 已被 `.remove()`。目录接口失败（节点还在）→ 正常提示；目录成功但 `render()`/`preview()`/`search.focus()`（:1396）抛错（节点已删）→ `querySelector` 返回 null → catch 内再抛 TypeError → unhandledrejection，弹窗留在半空白状态。

**修复方向**：加可选链，并在节点缺失时插入新的提示节点。
**验收判据**：在 `render()` 注入 throw，弹窗必须显示错误文案而不是空白。

---

# P3 — 工程卫生与防退化（低风险、可批量）

## P3-1　七个 CSS 变量被引用但从未定义，其中三处静默失效

**证据**（对 8 个 CSS 文件求 `var(--x)` 与 `--x:` 差集，并排除 JS `setProperty` 的 11 个动态变量）：
未定义者为 `--border`、`--panel`、`--text`、`--text-muted`、`--gold`、`--hit-size`、`--warning-ink`。

**三处无 fallback、直接 invalid-at-computed-value-time**：
- `backend/web/app.css:1064` `.local-status-recovery { border: 1px solid var(--border); }` → `border-style` 回落 `none`，**边框根本不渲染**。该元素由 `backend/web/image-queue.js:137-138` 创建，正是后端挂掉时唯一的反馈条。
- `backend/web/suite.css:634` `.facade-banner-option:hover { border-color: var(--gold); }` → 回落 `currentColor`，hover 高亮是错的。
- `backend/web/app.css:808` `.setting-proxy-url input { min-height: var(--hit-size); }` → `auto`。这是**拼写漂移**：真实 token 是 `--control-hit-size: 42px`（`app.css:37`，在 `:378/383/566/831` 正确使用），这个输入框丢了 42px 触控高度。

另**五处**有硬编码 fallback，恒取兜底值、永远脱离主题：`app.css:1064` 的 `var(--panel,#19222e)` / `var(--text,#e0e6ed)`、`gameplay.css:1875/1877` 的 `var(--text-muted,#a0a6ad)`、`gameplay.css:319` 的 `var(--warning-ink,var(--ink))`。

**修复方向**：改名对齐现有 token（`--hit-size`→`--control-hit-size`、`--text-muted`→`--muted`、`--text`→`--ink`、`--panel`→`--surface`、`--border`→`--line`、`--gold`→`--primary`），删掉冗余 fallback。

**验收判据**：新增一个约 30 行的 Node 校验脚本（放 `backend/web/`，接入现有 `node --test`）：解析所有 `*.css` 收集 `var(--x)` 与 `--x:`，叠加从 `*.js` 提取的 `setProperty` 白名单，任何未解析名即失败。脚本需明确策略：**带 fallback 的未定义变量也要报**（否则会漏掉上面那五处），只是可分为 error / warn 两级。**这是本轮性价比最高的一条** —— 同一个脚本还能覆盖 P3-3 的死类名扫描。目前项目**没有任何 CSS lint / stylelint / CSS 断言**（`.github/workflows/ci.yml` 确认），这类缺陷 100% 不可检测。

## P3-2　设计 token 漂移三族

- **稀有度配色硬编码 41 处**（`grep -c '7E8896\|9AA7B8\|E3B341\|C77DFF\|5AA9FF\|FF8AC7'` → champions.css 24 / gameplay.css 17），同一个四档色阶有三套实现（`champions.css:296-298`、`champions.css:811-813`、`gameplay.css:1275-1277`），已确认两处漂移：iron `#7E8896`（champions.css:586/821）vs `#6F7885`（gameplay.css:1274）；棱彩渐变方向 135deg vs 180deg。`app.js` 还把同样的 hex 作为 `--arena-augment-quality` 内联输出，是第三份副本。
- **名次奖牌两套六个漂移值**：`gameplay.css:697-700`（`.match-rank-chip`）vs `:724-727`（`.arena-rank-chip`），相隔 27 行，金银铜三档的前景/背景/圆角逐个被挪动（银底 `#DFE7EE→#9FB2C1` vs `#DDE4EC→#AFBAC8`，铜底 `#DFA678→#B4794C` vs `#DCA070→#B87333`）。
- **`--on-primary` 被绕过 12 次**：token 定义在 `app.css:10`，却在 `app.css:317/318/804/850/967`、`gameplay.css:698/703/707/1308` 直接写 `#14100A`；另有漂移变体 `#17130A`（`champions.css:358`、`gameplay.css:992/994`）。

**验收判据**：grep 守卫 —— 这几个 hex 字符串在全仓库各自只允许出现 1 次（在 `:root`）。

## P3-3　21 个确认的死类名，其中一条被逐字复制到两个文件后双双未用

方法：提取全部 1,398 个类名，对 502 个 `.js/.cjs/.html/.go/.mjs`（6.9 MB）逐个 grep；79 个未命中中剔除 52 个模板生成（`is-${grade}`、`is-color-${colorIndex}` 等，已逐个核实）与 2 个 desktop 脚本在用。剩余 0 命中者：
`augment-toolbar`(champions.css:50)、`brand-mark`(app.css:298)、`build-depth-sample-badge`(**champions.css:534 + gameplay.css:1469，两处文本完全相同且都死**)、`build-empty`、`build-side-group`、`champion-search-row`、`current-game-unavailable`、`facade-icon-badge`、`final-build`、`icon-label-button`、`live-augment-grid`、`live-orbit`、`live-waiting`、`loot-table`、`mayhem-champion-board`、`mayhem-skill-grid`、`panel-actions`、`pool-actions`、`privacy-list`、`recommended-augment-list`、`sync-copy`。
（`loot-table` 建议手工复核 —— `loot-${tokenClass}` 理论上可能生成它，虽然当前没有任何 token 产出 `table`。）

**另有 64 组逐字节相同的声明块**，最高频的四个即事实上的未命名原语：截断三连（12 处）、卡片外壳（5 处，已正确用 token，只差命名）、hover 态（5 处，跨 friends/gameplay/suite）、42px 头像（4 处）。

**验收判据**：把「已用/未用类名扫描」与「重复声明块检测」并入 P3-1 的同一个脚本，阈值可棘轮下调。

## P3-4　组件族重复实现（chip 31 个类族 / 空态 24 个 / 骨架屏 8 个 / 表格 7 个）

同一个 inline chip 原语出现 9 种圆角（3/4/5/6/8/999px）、7 种字号（8~12px）、11 种内边距；`.match-rank-chip` 与 `.match-badge-chip` 相隔 9 行、只有颜色不同。空态 17 个带独立 base 规则，字号 ∈ {10,10.5,11,11.5,12}px、内边距 12 种、min-height ∈ {66,84,210,360}px。
**间距/圆角无刻度**：33 种圆角取值（3~14px 连续，只有 17% 用 `--radius-*` token）、206 种 padding、53 种 gap。
**修复方向**：`.chip` + `.is-sm/.is-tone-*`、`.empty-state` + 尺寸修饰符；补 `--space-1..6`，只机械迁移覆盖约 70% 出现次数的 top-5 取值。属于长期债，可分批。
**验收判据**：预算型测试 —— 断言 `distinct(border-radius 字面量) <= N` 并逐轮下调 N。

## P3-5　传输层两个小缺口

- **gzip 用 `BestSpeed` 但结果被 `sync.Once` 永久缓存**（`backend/static_assets.go:55` + `:45`）：压缩每进程只算一次，BestSpeed 的前提不成立。实测首屏 17 个文件 raw 1,194,382 B → L1 359,050 B → L6 290,102 B，**多省 68,948 B（当前传输量的 19.2%），一次性多付 13 ms CPU**。L9 相比 L6 只多省 450 B，不值得。**诚实口径：回环下 69 KB 差值是亚毫秒级，不要以「首屏快多少」验收；这条的性价比在代价极低，不在收益大。**
- **`/api/image` 只有 max-age 没有 ETag**（`backend/main.go:951-953`、`:974-976`）：TTL 过期后是全量重下而非 304，且未走 `http.ServeContent`（对比 `handleMedia` 在 `:1052` 是走的）。`data` 已在手里，加 `ETag` 即可。节省量未验证。
- **`_large.png` 缺图时单请求扇出最多 6 个上游候选，负缓存只有 5 秒**（`backend/main.go:995-1003` + `:963-969` 串行 + `backend/community_images.go:22` negativeTTL=5s）。MEMORY 记载 657 个海克斯里有 60 个只有 `_small`。建议把「该 assetPath 全部候选均失败」本身作为一条负缓存（key 用 assetPath 而非 remotePath），TTL 提到分钟级。

## P3-6　缺组合路径断言：静态资源缓存头一旦被中间件覆写，全部测试仍然全绿

**证据**：`backend/main.go:692` 的 `securityHeaders` 写 `no-store`，`backend/static_assets.go:76` 随后 `Set` 成 `no-cache` 覆盖回来 —— 当前行为**正确**（配合 `static_assets.go:50` 的 sha256 内容 ETag，重启后端后浏览器命中 304 不全量重下）。但 `backend/static_assets_test.go` 全部用例都直接测裸处理器，没有一个经过 `securityHeaders(mux)` 的真实组合路径；`grep "no-cache" backend/*_test.go desktop/*.test.cjs` 零命中。

这正是 MEMORY 里反复出现的「函数体没改但组合关系断了」模式（R95 arenaAlliesCorroborate 拔线）。
**验收判据**：新增用例走完整栈请求 `/app.js`，断言 `Cache-Control == "no-cache"` 且带 `If-None-Match` 返回 304。**对抗变异**：删掉 `static_assets.go:76` 那行，用例必须 FAIL。

## P3-7　zindex 与文案一致性（最低优先级）

- `.toast`（`app.css:849`，z-40）与 `.suite-confirm-card`（`suite.css:96-100`，z-60）**锚点完全相同**（`fixed; right:18px; bottom:18px`），确认卡打开时 toast 被完全遮住；`.friends-dock`（`friends.css:21`，z-45）也压过 toast。
- `app.css:1064` 的 `z-index: 10000` 是全项目唯一超过 70 的魔法数，就在那条边框失效的 `.local-status-recovery` 上。
- `light-dark()` 只用了 2 次（`gameplay.css:311`、`:1030`），而 7 个深色主题都设 `color-scheme: dark`，**这两个值在全部深色主题里完全相同，根本无法参与主题化**。考虑到该函数在本项目已翻车两次，建议一并换成主题变量、清零。
- 文案：「读取失败」42 处 vs「加载失败」5 处；同一个本地请求超时有四种说法（`app.js:390` / `gameplay.js:264` / `champions.js:252` / `suite.js:162`）。建议收敛为「读取失败」与「本地请求超时，请重试」两套基准词。
- 自绘确认卡 `suite.js:118` 是 `aria-modal="false"` 且无 focus trap，Tab 会跑进被遮罩的背景 —— 对「卸下全部勋章」这类不可逆操作，键盘用户可能在焦点已离开卡片后按下 Enter。

---

# 4. 已排查排除（确认无问题，执行时不要再挖）

**后端并发与配额**
- R100「perks 持锁干等 12 秒」**已修**：`perk_catalog_async.go:9-12,34-36` 改异步 + 3 秒超时，注释明写 "Never hold the catalog mutex over network I/O"。
- R104 图标队列「取消记成失败并毒化」**已修**：`asset_cache.go:43-46,69-75`，前端 `image-queue.js:69-70` 取消只 `seen.delete`。
- R104 冷启动约 390 请求**已修**：`pro_refresh.go:122,132-152,199-200`、`pro_seed_accounts.go:105,117`。
- 「单次等待上界 vs 整轮总预算」**未复现**：现在是独立 context key 且有注释区分（`riot_limiter_queue.go:15-16`、`riot_api.go:283-285`）。
- 缺超时的 `http.Client`：**无**（6 个全有）。无上界重试：**无**（全部带次数上限与 ctx）。熔断器被拆：**无**（hexdata 有完整 per-kind 熔断，`hexdata.go:487-505,802-825`）。`time.Ticker` 未 Stop：**无**（8 处全 defer Stop）。无 ctx 的后台循环：**无**。
- LCU WebSocket 读循环未被同步 IO 阻塞（6 个回调逐条核对）；SSE 已 `SetWriteDeadline(time.Time{})`（`live_updates.go:41`）。

**数据准确性（这些是已正确降级的设计，属于应该照抄的范式，不要当缺陷改）**
- `player_ability.go:8,117,226-244` 的 `minimumAbilitySampleGames=3` 门禁 + `Grade:"—"`，前端 `gameplay.js:2139` 把阈值和当前场次都写给用户 —— **全项目标杆**。
- `hexdata.go:1224-1241` Wilson 下界 + 样本档位；`hexdata.go:902` 缺 SamplePolicy 直接拒收载荷。
- `qq101.go:355` + `champions.js:1954-1959`：无样本量时检测 0%/100% 极值并标注「样本极少」。
- `gameplay.go:3342-3346` + `gameplay.js:2175-2179`：参团率自带分母，不足时 `—` 并说明「不估算」。
- `data_sources.go:19-120` 的 `DataSourceAttempt`、`rank_insights.go:190-270` 的选源决策 + 诊断埋点 —— **P0-4 应该照抄这个范式**。
- 时间单位整体干净：`normalizeEpochMillis`（`gameplay.go:8674-8679`）7 个调用点全部传时间戳；`riot_api.go:945-952` 的 2021 单位切换守卫正确；前端走 `Intl.DateTimeFormat("zh-CN")` 本地时区。
- 版本/赛季缓存不会跨版本污染：`season_stats.go:137,149` 键含赛季、`champion_cache.go:95` 含 `dataVersion`、DDragon patch 嵌在请求路径里。
- **`lp-history` 存 PUUID 的历史遗留已修复**：`lp_tracker.go:53-67` 只写 `accountHash`，`storage.go:350-361` 是**每机随机 32 字节加盐** SHA-256 截 16 位，旧 schema 直接清空不迁移。README:38 / DESIGN.md:86 描述准确。**MEMORY 里那条待修项可以关闭。**

**前端**
- 骨架屏/空态清空已有内容：历史三类事故的守卫都在且正确（`gameplay.js:1763`、`:4861`、`:848-853` 进度帧逐字段过滤 null、`:1967-1971`）。
- 相位状态机**已同时看 gameId**：`gameplay.js:6794-6812` 的 `gameChanged` 与 `phaseChanged` 并联触发 `invalidateLiveForNewGame`，R94 的根因已被真实覆盖。
- 竞态保护普遍到位：`RequestCancelled` 哨兵、`liveGameGeneration`、`overviewRequestToken`、`workspaceRequestToken`、`skinLoadGeneration`、friends 的 `requestGeneration`；轮询自身带三重陈旧判据（`gameplay.js:6846`）。
- 自动规则重复提交已解决（`suite.js:506` 的 `watchSaveQueue` + `watchRevision`）；破坏性操作有二次确认（`suite.js:112/1731/1267`、`app.js:2835`）。
- `demo-data.js`(85,856 B) 生产零成本（`runtime.js:7-23` 仅 demo 模式注入，且被 `r115.test.cjs:14` 钉住）。
- `section-loader.js` 的 generation / singleflight / 失败重试 / 事件快照补交全部正确；失败有 `role="alert"` + 重新加载按钮。
- `overview-art.js` 三重去重 + IntersectionObserver + visibilitychange，无泄漏。
- 战绩列表的 `preserveMatchList` 真实生效（不重建、不重启图片、不跳滚动位置）。
- 对局链路的 `document.hidden` 覆盖完整（7 处），唯一漏网是 P1-6 的 status 链路。

**资源与打包（这些"经典 Web 性能清单"项目都已做，且比一般水平细）**
- gzip 已实现且真实接入（`static_assets.go:45-75` + `main.go:491`），q 值解析正确、Range 时禁压、有 `Vary`。
- 内容哈希 ETag，重启后端不全量重下；`no-cache` 是**正确**选择（URL 未指纹化，用 max-age 会拿到旧文件）。
- 各 API 的 `no-store` 逐个核对后认为**全部必要**（含账号拥有态、实时排名、NDJSON 流），真实缓存在服务端，前端不缓存是正确分层。
- 无未使用却被 embed 的文件：`data/reroll_pool_14_5_source.jpg`(1.34 MB) **没有**被 embed（`main.go:49` 只写 `.txt`/`.json`）；`rank-crests/*.png` 的 0 引用是假阳性（模板字符串动态拼接，4 处）；embed 通配未泄漏测试文件（测试是 `.cjs`）。
- 字体无 FOIT（`app.css:264 font-display: swap`），中文回退链**全项目只定义一次**，是最干净的区域。
- Go 构建**已有** `-ldflags="-s -w"` + `-trimpath -buildvcs=false`（`build-desktop.sh:64-65`、`build-desktop-windows.ps1:115-116`）；`desktop/package.json:37-51` 的 `files` 是显式白名单，40 多个 `*.test.cjs` 不进包。
- `!important` 仅 24 处（4800 行），全部合法（`[hidden]`、`prefers-reduced-motion`、分享导出面、覆盖内联样式）。
- 跨主题「近似色」360 对经逐一核对**是有意设计**（各主题把中性色向自己的强调色微调，`app.css:94-96` 有注释），已全部排除，只报同角色漂移。
- 浅色主题是一等公民（`app.js:2328/2438/2465`），14 处 `color:white` 与 7 处硬编码深底经核对基本正确（多位于英雄原画/图片遮罩之上）。

---

# 5. 本轮不建议动（需要单独立项或证据不足）

- **h2c（HTTP/2 明文）**：依赖已在 `go.mod:8`，理论上能根治 P1-2 的六连接瓶颈，但改传输层回归面大，且 R100 台账明确记过「h2c 不能假设 Chromium 自动支持」。除非先做独立探测，否则不要在本轮顺手加。
- **`champions.js` 的 key 冲突是否真会误显示「超时」**（`loadWorkspace`/`loadRankings` 共用 `"catalog"`/`"rankings"`）：只做了静态推导，未跑 jsdom 复现，**未验证**。
- **`mayhemAugmentLoading` 是否存在真正永久骨架屏的返回路径**：依赖缓存命中，**未验证**。修复 P2-6 不需要先回答这个问题。
- **`pro-profiles` 磁盘缓存上限 64 vs 已内置 53 个账号**（`pro_profiles.go:58`，`pro-profile-v2|` 不在 `proseed-` 的 LRU 保护名单里）：当前 53 < 64 不抖动，是潜在容量缺陷而非现网故障。若顺手改，建议由 `proSeedTotalAccounts()`（`pro_activity.go:99-105`）推导而不是魔法数。
- **`season-stats/` 未出现在 `features.go:752` 的 `stores` 清单里**：`season_stats.go:114-132` 持久化整季 `gameIds[]`、逐英雄战绩与逐局 `rankedMatches[]`；身份是加盐哈希，无原始标识泄露，但「逐局保存整个赛季战绩」不是现有声明能推出的。**这条需要用户拍板**是补进声明清单还是认为已被现有条款覆盖 —— 按项目惯例（R92 教训），不代为决定。
- **诊断日志导出端零过滤**（`features.go:451`）：当前无泄露（脱敏在发射端 `features.go:275-330` 白名单 + `gameplay.go:2137-2157`），属健壮性隐患而非现存违规。

---

# 6. 建议的执行顺序

1. **P0-1 / P0-2 / P0-4 / P0-6**（数据准确性红线，其中 P0-1、P0-2 是用户屏幕上正在显示的错误数字）。**P0-5 已收窄为披露缺口**，可与 P0 同批，但修复方向只允许「接上 `renderMeasurementTechnique`」。
2. **P1-1**（唯一能打空全员配额且无上界的路径）→ **P1-2/P1-3**（护栏已失效，且 P1-3 会放大 P1-2）。
3. **P1-4**（四条同根，一起改一起沉淀规则最省事）→ **P1-5/P1-6/P1-7**（同属「空转重建」族）。
4. **P3-1 的校验脚本**（约 30 行，同时解决 P3-1/P3-3 两条，并把三条审查发现转成 CI 强制不变量）—— 建议与 P1 并行，互不冲突。
5. **P1-8/P1-9** 体积项，需要真 Chromium 与截图比对，单独一批。P1-8 执行前必须先改 `backend/web/r115.test.cjs:13` 的反向断言，并把 `.suite-panel` 容器声明划进共享片。
6. **P2/P3 其余**按批清理。降级后的 **P3-8（原 P0-3）** 属于防未来退化，优先级最低。

---

# 7. 独立复核记录（2026-09-20，与撰写方分离）

本工单初稿完成后做了一轮独立验收，逐条打开引用的 `file:line` 实际核对。结果：**P0-1 / P0-2 / P0-6 / P1-2 / P1-9 / P3-1 / P3-7 未发现任何偏差**（多数逐字精确）。发现并已在正文修正的问题：

| 条目 | 复核发现 | 处理 |
|---|---|---|
| P0-3 | **立论不成立** —— 引用的 fixture 属于另一个端点，不经过被指控的函数；`ratePercent` 的实际上游全是 0..1 小数，现网输出正确 | **降级为 P3-8**，改写为「量纲缺少来源契约锁定」的潜在风险 |
| P0-5 | 下标推导被 `hexdata_test.go:465` 等三条现存测试钉死，是有意设计；原修复方向 (a) 会打红它们 | **收窄为披露缺口**，删除方向 (a)，验收判据加「现存测试必须保持绿色」 |
| P1-8 | 「suite.css 首屏需要 0 条」被反证 —— `index.html:255-281` 有完整 suite 静态外壳，37 条规则必须保留，含容器查询锚点 | 表格与结论已更正，验收判据新增第 (3) 条 |
| P0-4 | 「挡住了 Riot 兜底」为误判；`pro_profile_activity.go:22` 行号应为 :20-21 | 已更正，并加注「不要把唯一正确的那条路一起改掉」 |
| P1-1 | 队列计数全对，但「909」与「约 44 次」两个数字不可复现（省略了 `:56` 的提前 break） | 改为只保留代码严格支持的下界，大数标注为理论上界 |
| P1-4(b) | 触发点行号 1247-1250 应为 1257-1258 | 已更正；并补上 (a) 的磁盘 TTL 行号与 stale 语义 |
| P3-1 | 带 fallback 的未定义变量是五处不是四处（漏了 `gameplay.css:319`） | 已更正，并给校验脚本加了明确策略 |

**本轮暴露的三条方法论（建议一并沉淀）**

1. **拿 fixture 当反证前，必须回溯它经过哪个 parser。** P0-3 是找到一个数值反例就下结论，没确认这个反例是否流经被指控的代码 —— 与 R97「fixture 全硬编码 `Region:kr` 是盲区」是同一类错误的镜像面。
2. **指控一段行为是缺陷前，先 grep 有没有测试正在钉它。** P0-5 被一条专门命名为 `...AreExplicit` 的现存测试定义为有意设计，而工单的验收判据等于要求打破这条不变量还不自知。
3. **声明了核查范围就要真的跑完。** P1-8 漏的是 `index.html` 的常驻面板外壳，而「index.html 常驻标记」恰恰是工单自己写明的核查范围 —— 与 P0-1 那条方法论（「后端设置了降级说明必须追到前端渲染点」）是同一个毛病的另一面。

## 跨轮次方法论（建议写进 AGENTS.md / CLAUDE.md）

- **「后端设置了降级说明」必须追到前端渲染点才算数。** P0-1 被一路子代理判成「教科书式正确」，因为后端只填 `WinRate < 0` 且设置了 `capability.Detail` —— 缺陷完全在于那个 Detail 在前端的唯一读取点位于填充后永远不可达的分支。这是 MEMORY 里「只验工单执行、不验用户屏幕结果」的又一次复现。
- **改一个数值常量前，先查有没有测试/脚本把旧值当护栏钉着。** P1-2 里 R107 把并发从 2 提到 6，而 R100 明确记载「并发 6」曾是被杀死的变异，其断言至今留在仓库且与生产相反 —— 因为那个脚本不在 CI，没人发现。
- **负结果的缓存语义必须显式定义**（成功 / 确认为空 / 主动让路 / 失败四态），否则就会反复出现 P1-4 这一族缺陷。
- **手动 `*-browser.cjs` 脚本不进 CI = 护栏会随时间失效。** 至少应接入发布门禁，或在 CI 里做一次「脚本里的断言常量与生产常量一致」的静态比对。
