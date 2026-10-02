# WORKLIST-R149：海斗首次打开的剩余耗时（节流等待占大头）与启动时的职业选手数据；生涯页去掉「头像」「旗帜」两张卡

诊断人：Claude（读了 `lol-loot-diagnostics-0924-1616.jsonl`、用户截图、`backend/hexdata.go` / `web/suite.js` / `web/index.html`）。
**本单只出工单，Claude 没有改代码**，由 GPT 实现、验证、发版。
日期：2026-09-24。基线：0.12.19，含 R144–R148 的改动。
状态：代码与全量验证通过，待真机；版本与打包按用户要求暂缓。执行记录见 [r149-execution-ledger.md](r149-execution-ledger.md)。

用户的问题：
1. 海斗详情现在快一点了，还有什么可以优化的（结合日志）。
2. 生涯页里「头像」和「旗帜」两张卡去掉，不要了（截图：头像卡有「生涯头像 / 在收藏页浏览头像与旗帜」，旗帜卡有「生涯旗帜 / 段位旗（上赛季段位｜空白）」）。
3. （提问）从页面离开再回来，回到顶部好还是保持原位置好——答复见文末，**不改代码，不进工单范围**。

---

## 0. R148 已生效的证据（不需要再做）

- `hexdata_shape kind=heroes rows=173 fields=865`：榜单不再走 `/heroes` HTML，解析失败消失。
- `hexdata_hero_tier_source officialTierRows=173 localTierRows=0`：官方档位齐全。
- `hexdata_request` 与 `mayhem_detail_phases_ms` 两个诊断事件已经落地，下面的分析全部用它们。

## P1 海斗首次打开：剩下的时间去哪了

### 数据（同一次运行，打开海斗到第一个英雄详情完整，约 5.6 s；`answer` 在 644.72，详情完成在 650.33）

| 时间 | 请求 | `pace_wait_ms` | `wire_ms` | 字节 |
|---|---|---|---|---|
| 644.72 | answer | 0 | 157 | 38 KB |
| 645.29 | meta | **442** | 100 | 8 KB |
| 646.18 | 令牌页（`path_kind=token_page`） | **858** | 21 | 17 KB |
| 647.23 | hextech-insights | **847** | 188 | 373 KB |
| 647.87 | postmatch | **1605** | 76 | 90 KB |
| 648.97 | hero-json | **1011** | 683 | 1.2 MB |

- 这 5 个请求的**网络耗时合计约 1.2 s，节流等待合计约 4.8 s**（各请求的等待部分重叠，实际墙钟上从 644.72 到 648.97 一共 4.25 s）。结论：首次打开剩下的最大一块是节流，不是网络，也不是解析。
- 详情阶段（`mayhem_detail_phases_ms`，首个英雄）：`total=3072`，`hero_json=1757`（含 1011 的节流）、`decorate=1305`、`rsc=1036`、`postmatch=617`（在等预取的结果）、`insights=16`、`meta=9`。`decorate` 里包含两份 CDragon 目录（117 KB 约 625 ms、523 KB 约 387 ms），它们在 hero-json 解析之后才开始（`champion_upstream` 在 649.66 / 650.05），**是串行的**。
- 之后每个新英雄：`total=689 / 670 ms`（`hero_json=270 / 357`，节流为 0，`rsc≈410`，`decorate≈265`），已经接近下限；已经打开过的英雄是内存命中，毫秒级。

### 要做（按收益从大到小；每做一项用新日志对比）

1. **节流改成「突发额度 + 间隔」**（收益最大，预计省 2–3 s）。现在每个请求都是「300 ms 最小间隔 + 0–800 ms 随机抖动」，并且请求排队叠加（`postmatch` 等了 1605 ms）。改为：距离上一个 hexdata 请求已空闲超过窗口（建议 2 s，GPT 用日志调整）时，允许最多 4 个请求几乎无等待地连发，之后回到 300 ms 最小间隔；随机抖动只在突发额度用完、连续请求时才加，并把上限降到 200–300 ms。确认 R148 第 4 条（等待不持全局锁）是否真的落地：`pace_wait_ms` 叠加的形态说明可能没有，**请 GPT 先核实并在账本里写结论**。请求总数不变（一次打开海斗页最多 answer / meta / 令牌页 / insights / postmatch / 一个 hero-json，均按 buildId 缓存），仍在 R116 第 6 节红线和 R147 的许可范围内；`hexdataGlobalGate` 容量不要加。
2. **令牌页请求不要排在 meta 后面**（并入第 1 条的收益，单独写明，防止漏做）：令牌页不依赖 meta，进入海斗视图时与 answer 同时发出；令牌拿到之前依赖它的 insights / postmatch / hero-json 才等待。
3. **两份 CDragon 目录（装备 117 KB、海克斯 523 KB）提前取，并落盘**：进入海斗视图时后台并发预热，不要等到第一个英雄的 hero-json 解析后才开始；另外核对它们是否有磁盘缓存（日志里每次进程首次使用都是 `cache: miss`，而 ddragon 的同类文件是 `cache: disk`），没有就按补丁版本落盘，重启后不再重取。预计再省约 1 s（首个英雄的 `decorate` 从 1305 ms 降到约 300 ms）。
4. **`decorate` 里与 OP.GG RSC 无关的部分不要等 RSC**（收益小，约 0.25 s / 每个新英雄，放最后做）：装备资产、召唤师技能配对等在 hero-json 到手后立刻做，只有「合并 RSC 行后的海克斯装饰」等 RSC。
5. **不做的事**：新英雄的 hero-json（约 1.2 MB，`wire_ms` 215–683）与 RSC（约 410 ms）已接近下限；想再快只能为每个英雄预取，那是 R116 明确禁止的；R148 第 7 条的两段式响应也暂不启动，先看 1–3 的效果。

### 验收目标（以新日志里的 `hexdata_request` / `mayhem_detail_phases_ms` 为准，是目标值不是承诺）

- 首次打开海斗到首个英雄完整：从约 5.6 s 降到约 2–3 s；`pace_wait_ms` 合计降到 1 s 以内。
- 每个新英雄：保持 ≤ 0.7 s。
- hexdata 请求总数不变（冷启动最多 answer + meta + 令牌页 + insights + postmatch，外加每个被点开英雄一个 hero-json）。

### 测试

- Go：节流新语义（空闲后突发额度内 `pace_wait` 为 0；额度用完后连续请求仍 ≥ 300 ms；等待期间不持锁，`-race`）；令牌页与 answer 并发；CDragon 目录预热与落盘命中；`decorate` 拆分后行为与顺序不变的对照测试。
- 变异（都要让对应测试 FAIL，整文件还原）：去掉突发额度；抖动改回无条件；预热改回串行；把预取范围扩大到 per-hero（必须 FAIL）。

### 观察到但本单先不改：应用启动时的职业选手数据

- 日志前 15 s：约 40 个 op.gg 主站页面（每个约 1.1 MB，合计约 45 MB）加约 25 个 `lol-api-champion.op.gg` 请求；对应事件 `pro_directory_cost stage=supplements duration_ms=12183`、`stage=ladder stage_duration_ms=12646`、`pro_profile_batch accounts_checked=53 riot_requests=15 duration_ms=10372`，`pro_directory_cost stage=directory duration_ms=1810`。这就是 R148 里「没分析的那 45 MB」的来源：**启动时的职业选手目录补全**。
- 两份日志（1453、1616）里这些请求全是 `cache: miss`，看起来每次启动都重新下载。它不影响这次海斗的测量（用户在启动约 10 分钟后才打开海斗），但如果用户启动后马上进海斗，会与这些下载抢带宽。
- 请 GPT **先查后改**：① 这些页面有没有磁盘缓存和 TTL（`champion_cache.go` 里 op.gg 主机的缓存策略，尤其 `proLadderPath`）；② 为什么每次启动都 miss；③ 能否延后到应用空闲后（例如启动 20 s 后）或按 6–12 h TTL 落盘。**先在账本里写结论和建议，再决定是否另开工单**，不要在本单里直接改职业选手功能。

## P2 生涯页去掉「头像」「旗帜」两张卡

### 定位

- 都在 `web/suite.js` 的生涯（facade）渲染里：
  - 「头像」卡：`<section class="suite-card facade-icon-card">`，含 `<h3>头像</h3>`、`icon-current`（当前头像缩略图、「生涯头像」、聊天头像不一致时的提示、`data-facade-browse="icons"` 按钮）。
  - 「旗帜」卡：`<section class="suite-card facade-banner-card">`，含「生涯旗帜」（`data-facade-browse="banners"` 按钮）和「段位旗」（`data-facade-rank-banner`，`lastSeasonHighestRank` / `blank`，点击会执行 `applyFacade({action:"rank-banner"})`，后端写 `/lol-regalia/v2/current-summoner/regalia`）。
- 这两张卡里的「在收藏页浏览头像与旗帜 →」只是入口；**收藏页本身的「头像与旗帜」子页保留不动**，它在收藏页导航里有独立入口（`index.html` 的 `favorites-tab-facade`，`data-favorites-page="facade-collection"`），删卡不会让它不可达。

### 要做

1. 删除这两张卡的渲染（`suite.js` 约 1459–1460 行），以及只服务于它们的事件绑定（约 1611 行 `data-facade-browse`、1613 行 `data-facade-rank-banner`）。生涯页其余内容（预览卡、背景、聊天身份、展示清理、「这一页会改什么」）不变。
2. **段位旗（`rank-banner`）这个写入功能随卡一起退场**（用户截图明确要求去掉整张「旗帜」卡，段位旗在里面）。GPT 清理因此变成死代码的部分：前端 `applyFacade` 的 `rank-banner` 分支、`suite.js` 里 `rankBanner` 相关状态（约 1364、1706 行）、`demo-data.js` 里的 `rank-banner` 处理与 `rankBanner` 字段、`suite.css` 里 `.facade-banner-card` 样式、`window.deepLegendsOpenFacadeCollection`（`app.js` 约 2471 行）若已无调用点；后端 `facade_icons.go` 约 168 行的设置段位旗写入函数及其路由/动作分发、`profile_facade.go` 的 `RankBanner` 字段（约 63、81、160 行）——**逐项确认是否仍被别的功能用到再删**：`profile_facade.go` 约 866 行同一个 `PUT regalia` 属于「卸下头像框」（`clear-border`），**必须保留**，只能改它需要的字段；`r99_probe.go` 只读探针保留。删除清单和保留理由写进账本。
3. 「这一页会改什么」卡里的文案（「头像框、挑战勋章、赛季旗帜、表情轮盘」）：卸下头像框、卸下勋章的操作仍在，文字保持不变；只检查有没有提到已删除功能，有就删对应词，**不新增任何说明文字**。
4. 客户端上原有的段位旗偏好不动（我们只是不再提供修改入口）。

### 测试

- `web/r123.test.cjs` 里断言过「在收藏页浏览头像与旗帜」这段文案存在（约第 55 行）：按新行为改成「生涯页不再出现头像卡、旗帜卡、`data-facade-browse`、`data-facade-rank-banner`；收藏页仍有『头像与旗帜』导航入口」。
- 新增：生涯页渲染不含 `facade-icon-card` / `facade-banner-card`；「展示清理」与背景、聊天身份卡仍在；`applyFacade` 不再接受 `rank-banner`。
- Go：被删的动作不再出现在动作分发里，`clear-border` 的测试仍通过；删除后的死代码扫描（`go vet`、未使用符号）。
- 对抗变异：把头像卡加回渲染 → 新测试 FAIL；把 `rank-banner` 分支加回 → FAIL；误删 `clear-border` 依赖的字段 → 现有测试 FAIL（确认这道保护有效）。

## GPT 的其他工作

真实依赖下 `go build` / `go vet ./...` / `go test -race ./...` 与 R148 之后的基线对比，R135 记录的既有失败不得增加；`node --test backend/web/*.test.cjs desktop/*.test.cjs`。发版（版本号递增，`private` 模式，账本记 key mode、指纹、SHA256），写 `docs/r149-execution-ledger.md`，更新 `docs/WORKLIST-INDEX.md`。真机请用户导出日志，Claude 对照 `hexdata_request` 的 `pace_wait_ms` 和 `mayhem_detail_phases_ms` 做前后对比。

## 关于「离开页面再回来：回到顶部还是保持原位置」（仅答复，不在本单实现范围）

现状已经是「按页面记住并还原滚动位置」（`app.js` 约 2280–2325 行的 `sectionScroll` 还原，收藏/英雄页有各自的还原逻辑），我的建议是**保持现状**：这是带侧边栏/标签页的桌面应用，主流做法是各标签页各自保留状态（iOS/Android 的标签栏、Slack、Discord、VS Code 的标签、浏览器的后退都是这样）；「回到顶部」是网页里点新链接的习惯，放到应用里会让人丢掉刚看到的位置。补充两条规则，现有代码基本已满足：① 内容身份变了（换英雄、换玩家、换筛选）就回顶部，R148 的 tab 重置是同一个思路；② 数据刷新后位置已经没有意义的页面才回顶部。再点一次当前导航项回到顶部可作为以后的小增强，现在不做。
