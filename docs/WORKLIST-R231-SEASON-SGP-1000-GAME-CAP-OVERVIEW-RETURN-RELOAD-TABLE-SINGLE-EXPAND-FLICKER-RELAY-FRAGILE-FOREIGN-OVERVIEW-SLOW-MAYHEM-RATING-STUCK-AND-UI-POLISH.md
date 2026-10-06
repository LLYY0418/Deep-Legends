# WORKLIST-R231：赛季统计被 SGP 1000 场上限截断；英雄胜率出来慢；详情页返回总览整页重绘；数据表二级表格对齐、只开一个、开合闪烁；卡片箭头；构建头像提示；五杀标签看不清；韩服战绩容易坏；外服总览慢和数据表失败；客户端关闭后总览消失慢；海斗隐藏分一直「查询中」；长战绩列表卡顿；升级耗时剩余项；构建页选中英雄收起后不复位

诊断人：Claude，依据：
- 用户 9 张截图、1 段录屏（`2026-10-06 131601.mp4`，12.8 秒，展开/收起二级表格）、海斗隐藏分截图（13:49）、构建页截图（13:54）；
- 日志 `lol-loot-diagnostics-1006-1354.jsonl`（P14 用）；
- 日志 `lol-loot-diagnostics-1006-1344.jsonl`：0.12.74 → 0.12.75 在线升级（05:11Z），0.12.75 运行到 05:44Z；期间登录过国服、日服，查过韩服玩家；
- 当前工作区源码，基线 0.12.75（提交 `b26502d8`，指纹 `09d40c535496`）。

执行人：GPT。日期：2026-10-06。

构建 / 发布按用户当时的指示执行。所有截图由 GPT 在应用里实际操作后截取。

**优先级**：P1、P2、P3、P8、P9、P11 是功能问题，先做；P4、P7、P10、P12、P14 是缺陷；P5、P6 是样式；P13 是调查项。

**界面文案约束**：不要新增口径、方法、免责声明类说明文字。

---

## P1　赛季统计被 SGP「最近 1000 场」上限截断，灵活组排 217 场对不上客户端的 313 场（图一、图二）

### 证据（日志）

本人账号 0.12.75 的后台回补：

| 时间（UTC） | 事件 |
|---|---|
| 05:13:45 | `season_stats_head_refresh scanned=83 complete=false`（R230 把缓存版本升到 13，旧缓存作废，重新扫） |
| 05:13:45–05:13:57 | `SUMMARY` 从 `start_index=50` 翻到 650，每页 5–10MB |
| 05:13:57 | `season_backfill_round round=1 scanned=600 total_games=700 resume_index=700` |
| 05:14:02 | `start_index=1000` 返回 **12 字节**（空列表） |
| 05:14:02 | `season_backfill_round round=2 scanned=300 total_games=1000 complete=true` |
| 之后 | `season_stats_head_refresh scanned=528 complete=true`（卡片显示「S26 · 已统计 528 场」） |

- 同一窗口里另外 3 个玩家（05:14:49、05:17:35）也都是 `total_games=1000` 时 `start_index=1000` 返回 12 字节。
- 只有一个玩家停在 962 场，是因为扫到了赛季开始日期之前，属于正常结束。
- **结论**：腾讯 SGP 的 `/match-history-query/v1/products/lol/player/{puuid}/SUMMARY` 只能翻到最近 1000 场，`startIndex ≥ 1000` 一律返回空。
  - 这 1000 场包含所有模式（斗魂、大乱斗、匹配等）。
  - 本人 1000 场里属于 420/440/海斗的只有 528 场，更早的灵活组排落在 1000 场之外。
  - 所以英雄数据表灵活组排 217 场（101 胜 116 负），客户端是 313 场（150 胜 163 负）。
- 代码 `season_stats.go:920`：`!more` 就把 `Complete` 置真，把「上游没有更多数据」当成了「整季已扫完」。

### 同时发现的两个扫描缺陷

1. **完整缓存的「头部扫描」会跳到旧位置去翻页，新对局反而漏统计。**
   - 证据：缓存已经 `complete=true`，05:33:08 和 05:42:58 两次头部扫描却请求了 `start_index=700/750` 和 `800/850`，每次约 21MB。
   - 原因在 `season_stats.go:920–935`：
     - 回补完成时只清了 `ResumeIndex`，没清 `PendingIndex`（第 1 轮结束时被设成 700）。
     - 下一次头部扫描从 0 开始，第一页就撞到已缓存对局，走第 930 行，`ResumeIndex = PendingIndex = 700`。
     - 再下一次头部扫描的 `start` 就是 700，不再从最新一页开始。这时新打的对局不会被统计，还要白下 20MB 旧数据；之后每刷新一次往后挪 100 场，直到 1000 才回到 0。
2. **前台头部扫描和后台回补同时跑，同一份缓存两边都在写。**
   - 证据：05:13:58–05:13:59，`start_index=700` 和 `750` 各被请求两次（一次回补，一次头部扫描），05:17:31–32 又重复一次。
   - 原因：
     - 回补每轮结束会广播 `season-progress`，前端随即重新加载总览，触发 `startSeasonStatsRefresh`。
     - 这个前台扫描读到的缓存 `ResumeIndex=700`，于是从 700 开始扫（不是从头部），扫完调用 `finishSeasonScan` 写盘。
     - 同一时刻后台回补还在用自己内存里的状态写同一个文件，后写的覆盖先写的。

### 修改

1. **按队列分流扫描**。不再扫全部模式，改为带标签的历史查询（`matchHistoryFilteredOn` 已支持 `tag` 与 `tagsQueryType=OR`，`normalizeSGPMatchHistoryTags` 最多 4 个标签）：
   - 流 A：`q_420` + `q_440`（OR）；
   - 流 B：`q_2300` + `q_2400` + `q_3270`（OR）。

   每条流各自记录 `ResumeIndex`、`PendingIndex`、`Complete`，都扫到赛季开始日期之前为止。两条流共用 `GameIDs` 去重，聚合口径（`seasonStatsAccumulate` / `seasonAccumulateChampionTable` / `seasonRecordRankedMatch`）不变。
2. **先探测再定方案（写进账本）**：用本人账号分别请求 `tag=q_440&startIndex=300&count=50` 和 `tag=q_440&startIndex=0&count=50`，记录返回条数和最早一局的时间，确认带标签的查询是不是各自有独立的 1000 场窗口。
   - 如果单个队列标签也受 1000 上限，而 OR 组合不受：按每个队列一条流来扫（420、440、海斗三条）。
   - 如果单队列的 1000 场仍然到不了赛季开始：标记为「上游上限」，`Complete` 置真，新增 `capped_by_upstream=true` 写进缓存和诊断。界面不加任何说明文字。
3. 「上游没有更多数据」和「已经扫到赛季开始」在诊断里分开记：`season_backfill_round` 增加 `stop_reason`，取值 `season_start` / `upstream_end` / `upstream_cap_1000`；同时增加 `stream`（`ranked` / `mayhem`）和 `oldest_created_at`。
4. **修头部扫描**：
   - 头部扫描永远从 `startIndex=0` 开始，撞到已缓存对局就停，不改动 `ResumeIndex`。
   - 回补到达终点时同时清零 `PendingIndex`。
   - 删掉第 930–932 行把 `ResumeIndex` 改成 `PendingIndex` 的逻辑。头部扫描和续扫必须用两个不同的游标：头部扫描是无状态的，续扫才用 `ResumeIndex`。
5. **扫描单飞 + 写盘串行**：
   - 同一账号同一赛季，后台回补在跑时，`startSeasonStatsRefresh` 直接返回，由回补负责广播进度。
   - 写盘统一经过一个按账号的锁，写之前重新读盘合并 `GameIDs`（取并集），不能用旧的内存快照整份覆盖。
6. `seasonStatsCacheSchemaVersion` 13 → 14，旧缓存作废重扫（分流后口径不同）。
7. 回补每轮页数和总时限按分流后的数据量重新评估：带标签之后单页只含相关对局，`seasonBackfillTimeout=90s` 应该够用，在账本里记实测值。

### 测试（Go）

- 假 SGP：不带标签时 `startIndex≥1000` 返回空，带 `q_440` 标签时能翻到第 313 场 → 赛季灵活组排统计 313 场，`stop_reason=season_start`。
- 假 SGP：带标签同样在 1000 处截断 → `Complete=true`，`stop_reason=upstream_cap_1000`，`capped_by_upstream=true`。
- 回归 R230：完整缓存连续 3 次头部扫描，请求的 `startIndex` 只有 0（新对局 ≤ 一页时），不会出现 700/800。
- 回归：完整缓存之后在头部插入 2 场新对局，头部扫描后统计 +2。
- 竞态：回补进行中触发 `startSeasonStatsRefresh`，SGP 假服务记录的同一 `startIndex` 请求次数为 1；两个写入方并发写盘后，`GameIDs` 是两边的并集。
- `-race` 跑以上测试。

---

## P2　总览「英雄胜率」出来慢

### 日志能确认的事实

1. **查看别的国服玩家**（05:14:35、05:17:19、05:17:44 三次）：
   - 总览本身 0.7–0.9 秒就返回了，但英雄胜率卡片此时只有「正在后台读取本赛季对局」。
   - 约 1 秒后头部扫描给出 20–89 场，再过 13–15 秒回补才到 1000 场。
   - 每个玩家下载 20 页、约 120–180MB（单页 6–10MB），这是慢的主要原因。
2. **进度刷新是整份总览重载**：
   - `handleSeasonProgress`（`gameplay.js:1221`）每收到一次进度就 `loadOverview(tab, true, …)`，重新请求整个总览，然后整页重新渲染。
   - 中间进度还有 10 秒节流，所以卡片是「跳着」变的。
3. **本人冷启动**：总览首包 3.2 秒，SGP 7.9MB，其中 `detailed_matches` 697ms、`recent_ranked` 413ms，英雄胜率随总览一起出来。

### 修改

1. P1 分流后，每个玩家只下载 420/440/海斗的对局。账本里记下同一个玩家改前、改后的字节数和完成时间。目标：回补完成时间降到改前的一半以下。
2. 后台回补可以 2 页并发（同一条流里顺序不变，按 `startIndex` 合并），轮间停顿保持不变。如果 SGP 返回 429 或变慢，就退回串行，并写进诊断。
3. **轻量进度刷新**：
   - 新增 `/api/gameplay/season-summary?playerRef=…`，只读磁盘快照，返回 `seasonChampionStats`、`seasonOverall`、`seasonStatsProgress`、`rankedQueues` 的赛季部分，不访问网络。
   - `handleSeasonProgress` 改为调用这个接口，只替换「排位」卡片和「英雄胜率」卡片的 DOM，不再整份重载总览。
   - 中间进度的节流从 10 秒降到 3 秒；「完成」事件仍然立即刷新。
4. 卡片在只有头部扫描结果（例如 20 场）时，就先把这 20 场渲染出来。不要等回补，也不要显示空的「正在读取」。

### 测试

- Node：收到 `season-progress` 后，`fetch` 只请求了 `season-summary`，没有 `overview`；`.match-list` 的 DOM 节点是同一个对象（没被重建）。
- Go：`season-summary` 只读快照，不发 SGP 请求（用 SGP 计数断言）。
- 记录 `overview_card_ready` 里 champions 的改前 / 改后时间。

---

## P3　从英雄数据表 / 熟练度页返回总览，整页重新加载（用户：「之前都加载过了，不应该出现加载」）

### 根因（源码确认）

- R230 P1 的修法是「进详情页时清空渲染缓存」（`renderOverviewSubpage` → `resetOverviewRenderCache`，`gameplay.js:2763`）。
- 详情页还会用 `container.innerHTML = …` 把整个总览 DOM 换掉。
- 点「返回总览」后，`rerenderTab` 只能从头生成：横幅、左栏、战绩列表全部重建。
- 所有图片重新进图片队列，要等 IntersectionObserver 放行后才给 `src`，所以先是空白再出现。平均段位、标签也重新请求（`match_tiers_overview_batch`）。
- 用户看到的就是「又加载了一遍」。

### 修改

1. 进入详情页时**保留总览 DOM**，不再替换。做法参考 `activateOverviewView`：
   - 把总览容器的子节点移进一个 DocumentFragment，挂在 `container._overviewSubpageStash`；
   - 同时保存 `overviewRenderCacheFields()` 的值和 `dataset.matchTierScope`。

   或者：详情页渲染进同级的 `.overview-subpage-host`，总览本体只设 `hidden`。选更简单、测试更容易写的那种。
2. 返回时把子节点和缓存字段原样放回，然后走正常的增量渲染。如果在详情页期间 `tab.data` 变了（例如赛季进度更新），由现有签名比较只更新变化的卡片。
3. 撤掉 R230 加在 `renderOverviewSubpage` 里的 `resetOverviewRenderCache(container)`。R230 P1 的兜底保留：签名相同但 DOM 里找不到 `.career-column` 时，当作没有缓存。
4. 返回时不触发总览网络请求。只有数据确实过期了（超过现有自动刷新间隔），才后台刷新并增量替换。

### 测试（Node + jsdom）

- 进出英雄数据表、熟练度页各 3 次：
  - 返回后 `.career-column`、`.match-list`、横幅节点都是进入前的同一个对象（`===`）；
  - 第一张战绩卡头像的 `img.src` 不变；
  - `fetch` 没有 `overview` 和 `match-tiers` 请求。
- 浮层玩家（`.player-overlay-scroll`）同样测一遍。
- R230 P1 的回归测试保留，并且通过。

---

## P4　英雄数据表二级表格：竖线和按钮对齐、只保持一个展开、开合闪烁（图五、录屏）

### 根因（源码确认 + 录屏逐帧）

1. **竖线不齐**：
   - 对位行的主题色竖线画在第二列「英雄」单元格左边（`.is-opponent .champion-table-hero { border-left }`，`gameplay.css:2104`）。
   - 「显示更多对位」那一行是一个从第一列 `#` 开始的 `td[colspan]`，竖线画在整张表最左边（`gameplay.css:2107`），所以和上面的竖线错开一列。
2. **按钮不齐**：按钮只有 `margin-left:24px`，没有按「↳ + 头像」的缩进来排。
3. **可以同时展开多个**：`tab.championTableExpanded` 是 Set，点一个加一个（`gameplay.js:2728`）。
4. **闪烁**：
   - 每次展开/收起都调用 `renderOverviewSubpage`，整个详情页 `innerHTML` 重建（`gameplay.js:2725`、`2766`）。
   - 新的 `<img>` 要重新排队才拿到 `src`（`prepareImages`，`gameplay.js:8062`）。
   - 录屏 30fps 逐帧可见：每次开合都有 1–2 帧所有英雄头像变空，竖线也消失一下。

### 修改

1. 「显示更多对位」这一行改成和对位行一样的结构：
   - 第一格是空 `<td>`；
   - 第二格是 `<th class="champion-table-hero">`，带同样的 `border-left` 和 `padding-left`；按钮放在这一格，左边缘和对位行的头像左边缘对齐（跳过「↳」的宽度，32px + 18px）；
   - 后面一个 `td colspan` 留空。

   去掉 `.is-opponent td[colspan]` 的 `border-left`。
2. 展开状态改为单个值 `tab.championTableExpandedId`：
   - 点另一行时，先收起旧的再展开新的；
   - 点已展开的行就收起；
   - 默认展开第一行（保持现状）；
   - 切换队列页签时重置。
3. **不再整页重绘**：
   - `bindChampionTable` 改成在 `tbody` 上做事件委托；
   - 展开/收起只删除旧的对位行、插入新的对位行，新行用模板生成，只对新插入的行调用 `prepareImages`；
   - 「显示更多对位」只追加新的 10 行；
   - 排序只重排 `tbody` 里现有的 `tr` 节点（按 id 复用），表头、页签不动；
   - 切换队列页签仍然走请求和完整渲染（数据换了）。
4. 头像图片在同一张表的生命周期内复用节点，不重新排队。

### 测试（Node + jsdom）

- 展开 A 再展开 B：A 的对位行全部移除，B 的对位行出现，`aria-expanded` 正确。
- 展开/收起前后，「所有英雄」行和第 1–7 行头像的 `img` 节点是同一个对象，`src` 不变。
- 「显示更多对位」行的结构是：第一格空 `td`，第二格是 `th.champion-table-hero`，按钮在 `th` 里。
- 排序后行节点数量不变，并且是原来的节点对象（只是顺序变了）。
- GPT 用 Playwright 在 1280 宽录一段展开/收起，逐帧确认头像没有空帧，存到 `docs/history/reports/r231/`。

---

## P5　「英雄胜率」「英雄熟练度」标题的箭头没有垂直居中；熟练度标题的「最高分」（图三、图四）

### 根因

箭头是文字字符「›」（`gameplay.js:2577`、`2779`）。字形本身偏在行框下半部，而且 `font-size:24px` 和左边 11px 的文字基线不一样，所以 `place-items:center` 也居中不了。另外按钮的 `margin-left:6px` 加上父级 `gap:6px`，左边空了 12px（`gameplay.css:2045`、`2047`）。

### 修改

1. 把「›」换成内联 SVG 箭头：
   - `viewBox="0 0 16 16"`，路径 `M6 3.5 10.5 8 6 12.5`，`fill:none; stroke:currentColor; stroke-width:2.2; stroke-linecap:round; stroke-linejoin:round`；
   - 图标尺寸 16px（比现在看起来大一点），按钮 22×22，`display:inline-flex; align-items:center; justify-content:center; line-height:0`；
   - 两处共用一个函数生成。
2. 去掉按钮的 `margin-left`。父级 `span` 的 `gap` 改为 2px，`align-items:center`。
3. **熟练度「最高分」**：这只是排序方式的名字，不代表缺数据。去掉它，换成真实数据「N 个英雄」。
   - N 是有熟练度记录的英雄总数；
   - 后端在 `normalizeMasteries(…, 6)` 截断之前取总数，新增字段 `masteryChampionCount`；
   - 拿不到时右侧只显示箭头。
4. 小屏规则 `gameplay.css:450` 的 `align-items:flex-start` 不能让这两个标题的箭头错位。如果会，就单独给这两个标题设 `align-items:center`。

### 测试

- Node：两个标题都输出 SVG 箭头，不再有「›」字符；熟练度标题不再含「最高分」，`masteryChampionCount=37` 时显示「37 个英雄」。
- GPT 截图：深色、浅色主题；1280、780 两种宽度。用开发者工具量箭头中心和左侧文字中心的纵向差，要求 ≤1px，量出的数值写进账本。

---

## P6　构建页英雄切换栏：鼠标放到头像上显示的是英雄名，不是玩家名（图六）

### 根因（源码确认）

- `renderBuildPlayers`（`gameplay.js:4288`）的按钮上 `data-tooltip` 是「玩家名#编号 + 英雄 · 等级」。
- 按钮里面的头像用的是 `iconFigure("champion", …)`，这个函数默认 `withTooltip=true`，会给头像再挂一个只有英雄名的提示。
- 鼠标在头像上时命中的是里层的提示，所以显示「时间刺客」；只有指在按钮边缘的几像素上才会显示玩家名。

### 修改

1. 头像改成 `iconFigure("champion", id, name, "", false)`，只保留按钮上的提示。
2. 提示内容：
   - 第一行「玩家名#编号」；
   - 第二行「英雄名 · N 级」；
   - 隐藏玩家显示「隐藏玩家」；
   - 开了「遮罩名字」设置时，第一行按现有遮罩规则显示。
3. 同一类问题全仓排查：外层按钮自己有 `data-tooltip`、里面又放了带提示的 `iconFigure` 的地方都要改，在账本里列出改了哪些。

### 测试（Node）

- 构建切换栏里所有 `.build-player` 的子孙节点都没有 `data-tooltip`。
- 遮罩开启时提示里没有真实名字。

---

## P7　战绩卡「五杀」标签看不清；三/四/五杀的字用不同颜色（图八）

### 根因（源码确认）

1. 渐变 id 是 `mk-${level}-${gameID}`（`gameplay.js:3418`）。同一局在页面里出现两次时就会重复，例如总览列表和浮层，或者 R231 P3 之后保留在页面里的旧 DOM。
2. `fill="url(#…)"` 指向第一个同名元素；第一个在隐藏的子树里时，Chrome 不画这个渐变。
3. 截图里五杀底板就是透明的，只剩翅膀，而五杀文字是深棕色 `#492601`，落在深色卡片背景上就看不清了。

### 修改

1. 渐变 id 加一个模块级自增计数：`mk-${level}-${gameID}-${++multiKillGradientSeq}`。翅膀火焰渐变（`-flame`）用同一个后缀。
2. 文字颜色分级：
   - 双杀 `#FFFFFF`；
   - 三杀 `#FFE4E6`；
   - 四杀 `#FAE8FF`；
   - 五杀 `#3B1A00`、`font-weight:800`、`text-shadow:0 0 2px rgba(255,247,214,.9)`。

   四杀、五杀字号 11px。浅色主题用同一套。
3. 底板渐变的颜色变量不变。

### 测试

- Node：同一个 `gameID` 渲染两次，两个 SVG 里的 `linearGradient` id 不同，各自的 `fill` 指向自己的 id。
- GPT 截图：1280 宽深色、浅色两套，二/三/四/五杀各一张，放到 `r231/`。

---

## P8　韩服（外服）战绩查询「战绩服务暂时不可用」，而且一坏就是 5 分钟（图七）

### 证据（日志）

| 时间（UTC） | 事件 |
|---|---|
| 05:18:31 | `riot_relay_probe result=failed http_status=0 duration_ms=3000`（3 秒内连不上） |
| 05:18:34 / 05:18:46 / 05:36:25 | 韩服总览 7ms 内直接返回 503，`riot_overview_cost error_kind=riot_relay_unavailable`（处在 5 分钟锁定期，根本没发请求） |
| 05:23:41 | 再探测，3 秒超时，失败 |
| 05:28:44、05:33:47 | `http_status=200`，但 `result=failed`：响应头回来了，响应体没在 3 秒内读完（`riot_relay_request_summary failures.read=2`） |
| 05:38:49 | 探测成功，耗时 1941ms |

0.12.74 时段（10/05 17:14–17:38）中转站一直正常。

### 根因（源码确认，`riot_relay.go:341–411`）

1. 探测用的是 `/r/kr/lol/status/v4/platform-data`，所有中转站加起来只有 **3 秒**总时限，还要求把整个响应体读完、而且是合法 JSON。中转站在 Cloudflare 上，国内访问延迟常在 2–3 秒，很容易超时。
2. 失败一次就**锁定 5 分钟**（`nextTry = now + 5min`），锁定期内所有外服请求直接返回 `errRiotRelayUnavailable`。
3. 点「重试」或「刷新战绩」也绕不过锁定，所以用户看到的是「一直坏」。
4. 单次业务请求失败也会调用 `failed(origin)`，同样锁 5 分钟。

### 修改

1. **探测**：
   - 改为每个中转站单独 8 秒时限。
   - 优先请求中转站自己的轻量健康路径（如 `/health`）。如果 Worker 没有这个路径，就由 GPT 在 Worker 侧加上，并更新部署说明。这一步需要部署 Worker，部署前先问用户。
   - 响应头 2xx 就算可达，不再要求读完响应体。
2. **锁定改为退避**：
   - 连续失败依次等 15s → 30s → 60s → 120s，封顶 120s；成功一次就清零。
   - 单次业务请求失败不再直接锁定，连续 2 次网络错误才进入退避。
3. **用户主动重试必须真的重试**：
   - 总览「重试」、「刷新战绩」带上 `force=1`，后端忽略退避、立刻重新探测一次（仍然单飞）。
   - 探测成功后广播 `riot-relay-recovered`，前端当前处在「战绩服务暂时不可用」的外服页签自动重新加载一次。
4. **诊断**：`riot_relay_probe` 增加：
   - `failure_stage`：`dns` / `connect` / `tls` / `headers` / `body`；
   - `connect_ms`、`ttfb_ms`、`bytes`；
   - `backoff_s`。
5. 错误页文案保持「战绩服务暂时不可用」，下面的「重试」按钮要能真正触发 3 里的强制探测。

### 测试（Go）

- 假中转站：第 1 次探测超时、第 2 次成功。`force` 重试立刻成功，不用等 5 分钟。
- 连续失败的退避序列是 15/30/60/120/120 秒。
- 响应头 200、响应体慢：判定为可达。
- 单次业务请求网络错误不锁定；连续 2 次才进入退避。
- 前端（Node）：收到 `riot-relay-recovered` 后，处在错误态的外服页签重新请求一次总览。

---

## P9　外服（日服）总览出来很慢；外服英雄数据表出不来，提示写成「韩服」（图九）

### 证据（日志）

1. **日服本人总览 13.2 秒**（05:38:05 → 05:38:19）：
   - `queue_labels` **6541ms**：读客户端 `/lol-game-queues/v1/queues`，日服客户端刚连上，接口很慢；
   - 然后才开始 `detailed_matches` **5010ms**：先走 Riot 接口（中转站正处在 P8 的不可用状态），失败后才回退 LCU；
   - 第二页（05:39:46）又是 13.6 秒，`match_history_data_source_decision` 显示 Riot 接口「响应超时」后回退 LCU。
   - LCU 自己读历史只要 100–130ms（05:39:51 两次）。
2. **日服英雄数据表**：
   - `opgg_champion_table_cost http_status=200 queue=FLEXRANKED`（05:39:34、05:40:18），接口返回 80 字节，即 `available:false`；
   - `champion_table.go:355` 把所有解析失败都写成「韩服英雄数据读取失败」；
   - 失败时页面不显示队列页签，用户也没法切到别的队列。

### 修改

1. **本人外服总览以 LCU 为先**：
   - 当前客户端的本人账号（`isCurrent` 且客户端是 Riot 区）先用 LCU 历史出首屏（带上已经能拿到的字段）；
   - Riot 接口在后台补全十人详情，限时 6 秒，回来后原地替换对应战绩卡；
   - 超时或失败不影响首屏，也不弹错误。
2. **队列名不再挡在关键路径上**：
   - `loadQueueLabelsContext` 改为先用内置队列表（`queue_groups.go` 里已有的 ID → 中文名），同时后台读 LCU，限时 1.5 秒；
   - LCU 结果回来后进入缓存，下次使用；
   - `loadDetailedMatches` 不再等队列名，可以并行。
3. **数据表区分「没数据」和「读取失败」**：
   - OP.GG 页面里找到了该玩家、但这个队列本赛季没有对局，返回 `available:true, rows:[], overall:{games:0}`，界面显示「本赛季没有该模式对局」；
   - 真正解析失败时，新增诊断 `opgg_champion_table_parse`，字段为 `platform`、`queue`、`reason`（`props_missing` / `season_mismatch` / `total_mismatch` / `duplicate_champion` / `opponent_identity`），记录具体在哪一步失败。
4. **提示语按实际服务器**：
   - 后端统一用 `riotRegionLabel(reference.Region)`（日服、美服等）生成提示；
   - `champion_table.go:348/355`、`riot_api.go:360/2031`、`overview_current_game.go:526/623` 这些面向所有外服的用户可见文案都改掉；
   - 只服务韩服的功能（绝活哥、职业选手）保持「韩服」；
   - 改了哪些，在账本里列一张表。
5. **失败或无数据时也显示队列页签**（`renderOverviewSubpage` 里只有成功才渲染页签），让用户能切换队列。
6. **外服默认页签**：
   - 单双排本赛季 0 场、灵活组排有场次时，默认打开灵活（现有 `seasonGames` 逻辑）；
   - 两者都没有时，默认单双排，并显示「本赛季没有该模式对局」。

### 测试

- Go：
  - 当前客户端是日服本人时，Riot 假接口挂起 20 秒，总览也在 1 秒内返回 LCU 首屏；
  - `queue_labels` 假接口挂起 10 秒，总览不受影响。
- Go：OP.GG 假页面「该队列 0 场」→ `available:true`、`rows` 为空。
- Go：`region=jp1` 解析失败时 `detail` 含「日服」，不含「韩服」。
- Node：数据表失败态仍然有三个队列页签。
- GPT 真机（日服账号）：账本里记总览首屏时间，目标 ≤2 秒。

---

## P10　客户端关闭后，总览页面消失得有点慢

### 日志能确认的事实

- 两次关闭（05:35:57、05:41:48）：LCU WebSocket 断开 `lcu_event_stream close-abnormal` 后，前端 0.2–0.4 秒就切掉了总览（`blocking_state_client skip client-exiting`）。应用这一段很快。
- 慢在断开之前：客户端窗口关掉以后，`LeagueClientUx` 进程还会活几秒，这期间 WebSocket 不断、接口照常返回 200。
  - 第一次关闭时，05:35:55.9 好友列表事件变成 0 人（`social_presence_resolved friend_count=0`），比断开早 1.4 秒，说明客户端已经在退出了。
- 日志里没有记录断开前收到的事件 URI，看不出最早的退出信号是什么。

### 修改

1. 新增诊断 `client_shutdown_trace`：断开时把断开前 10 秒内收到的 LCU 事件 URI（去重，最多 30 条）和各自的相对时间一起写出。同时记录 `LeagueClientUxRender` 进程消失的时间（如果现有进程扫描能拿到）。
2. 订阅并识别退出前兆，任一出现就立刻按「客户端正在退出」处理（隐藏本人总览、停止轮询，等同于现在断开后的处理），并在诊断里写明命中的是哪个信号：
   - `/riotclient/pre-shutdown/begin`；
   - `/process-control/v1/process` 的 `status` 变成 `Stopping`（如有）；
   - 聊天会话 `/lol-chat/v1/session` 变成离线并且好友列表清空。
3. 如果这些信号在 GPT 的 Windows 实测里都不出现，就按 1 的诊断结果选一个最早且稳定的信号，账本里写清楚选择依据。
4. 前兆触发后 15 秒内客户端没有真正断开（误判），就恢复显示。

### 测试

- Go：假 LCU 先推 `/riotclient/pre-shutdown/begin` 事件，`status` 立刻变为 `client-exiting`，不用等 WebSocket 断开。
- Go：前兆后 15 秒内没有断开 → 恢复连接状态。
- GPT Windows 实测：从点客户端右上角关闭到总览消失的时间，改前 / 改后各记 3 次。

---

## P11　海斗隐藏分一直显示「查询中」（用户 13:49 截图）

### 根因（源码确认）

1. `loadMayhemRating`（`gameplay.js:2548–2565`）把状态设成 `loading` 后发请求。请求被取消（`RequestCancelled`）时，`catch` 里直接 `return`，**不改状态**，状态就永远停在 `loading`。
2. 下次再调用时，第一行判断 `current.status === "loading"` 直接返回，所以再也不会重新请求，界面一直转圈。
3. 请求会被取消的地方：
   - `softResetGameplayState()`（客户端断开 / 切换账号时 abort 所有请求，`gameplay.js:5212`）；
   - `resetTencentTabsAfterDisconnect()`。

   用户 13:35–13:42 正好从国服切到日服再切回国服，和截图时间吻合。
4. 后端的 ARAMKit 当天也慢：05:17:29、05:17:54 两次 9 秒超时（`mayhem_rating_lookup reason=timeout`）。超时结果按失败缓存 **5 分钟**（`aramkitRatingFailureTTL`），这 5 分钟内用户只能看到「暂不可用」。
5. 这次截图在日志导出之后，日志里没有对应记录。上面是按源码推出的触发路径，需要 GPT 按「测试」复现确认。

### 修改

1. 请求被取消时，如果 `requestToken` 仍是当前值，把状态退回 `idle`，下次渲染或加载总览时自动重试。
2. `R230 openOverviewSubpage` 里同样的「取消后直接 return」也要检查。如果会让详情页一直停在「正在读取…」，按同样方式退回。
3. 后端：超时、网络错误这类临时失败的缓存时间改为 30 秒；上游明确返回「未收录」的，仍然按 5 分钟缓存。
4. 前端：`status === "error"` 或「未收录」之外的不可用结果，60 秒后重新加载总览时允许重试。

### 测试

- Node：发出海斗请求后立刻 `softResetGameplayState()`，状态回到 `idle`；再次加载总览会重新请求，最后显示分数。
- Go：ARAMKit 假服务超时一次后 31 秒，再请求会真正发出；返回「未收录」的结果在 5 分钟内命中缓存。

---

## P12　战绩列表翻到 200 场后页面卡顿（日志）

### 证据

- `renderer_perf` 05:33:42 和 05:34:41：overview 页
  - `dom_nodes` 25976 / 32017；
  - `img_count` 3613 / 4510；
  - 一分钟内长任务 59 / 74 次，合计 3.6 / 4.4 秒。
- 同时段用户在本人海斗筛选下连续翻页（`beg_index` 20→180）。
- 平时同一页只有 4–5 千个节点，长任务不到 0.5 秒。

### 修改

1. 战绩卡外层加 `content-visibility:auto; contain-intrinsic-size:auto 120px`（按实际卡片高度测出来的值），让屏幕外的卡片不参与布局和绘制。
2. 如果 1 之后仍然超标：做窗口化。保留可视区上下各 30 张卡片的 DOM，其余卡片换成等高占位块；数据仍然保留在 `tab.data`，滚回来时重新生成。
3. 屏幕外卡片的图片不进入图片队列（现有 IntersectionObserver 逻辑要和 `content-visibility` 一起验证，确保滚回来时会加载）。

### 测试

- GPT 真机或 Playwright：连续翻到 200 场后静止 1 分钟，`renderer_perf` 的长任务合计 ≤1000ms，`dom_nodes` 和改前的对比写进账本。
- Node：在 jsdom 里断言卡片带 `content-visibility` 类名或样式。如果做了窗口化，断言 200 场时实际卡片 DOM ≤ 80 个。

---

## P13　在线升级耗时：R212 剩余项（调查）

### 用户真机结果（本次日志，0.12.74 → 0.12.75）

| 阶段 | R212 改前 | 0.12.73→0.12.74（10/05） | 0.12.74→0.12.75（本次） | 目标 |
|---|---|---|---|---|
| `parent_exited → uninstall_old_start` | 7.0 秒 | 3.15 秒 | **3.19 秒** | ≤2 秒（未达标） |
| 其中 `nsis_start_to_oninit` | — | 2.87 秒 | **2.93 秒** | — |
| `uninstall_old_ms` | 6.5 秒 | 1.71 秒 | **1.88 秒** | 变短（达标） |
| `extract_ms` | — | 8.26 秒 | 6.61 秒 | — |
| 总计 `total_ms` | — | 15.5 秒 | 13.4 秒 | — |

剩下的时间几乎全在 `nsis_start_to_oninit`：从安装程序进程启动到 `.onInit` 第一行。这一段是 NSIS 自身的初始化，包括对整个安装包做 CRC 校验；Windows Defender 扫描新 exe 的时间可能也在这里面。

### 修改（调查为主）

1. 构建一个对照包：在 electron-builder 的 NSIS `include` 脚本里加 `CRCCheck off`，其余不变。在同一台 Windows 机器上，用两个包各升级 2 次，比较 `nsis_start_to_oninit`。
2. 如果 CRC 关掉后这一段降到 1 秒以内，就采用。安装包完整性仍然由下载阶段的 SHA-256 校验保证（确认现有下载流程确实校验了；如果没有，先补上校验，再关 CRC）。
3. 如果关 CRC 没有明显效果，就在账本里写明是杀毒扫描或系统因素，R212 的这一条按「已知限制」关闭，不再继续硬改。

---

## P14　构建页切换到别的英雄后，收起战绩卡再展开，构建仍停在别的英雄上（用户 13:54 截图）

### 现象

在一局的「构建」里点了对面的英雄（截图里是第 3 个红方头像），收起这张战绩卡再展开、进入「构建」，选中的仍然是那个英雄，不是自己。期望：重新展开后回到当前玩家自己的英雄。

### 根因（源码确认；日志 `lol-loot-diagnostics-1006-1354.jsonl` 只有 `match_timeline_succeeded`，没有前端选择状态的记录，佐证这是纯前端状态没清）

- 构建页选中的玩家存在 `tab.buildPlayers`（`Map<gameId, participantId>`，`gameplay.js:4715–4716`），`buildSubject`（`gameplay.js:4273`）优先读它。
- 收起战绩卡时只清了 `tab.matchDetailTabs`，没清 `tab.buildPlayers`。代码里一共三处：
  - `bindMatchEntryControls` 的收起分支（`gameplay.js:4807–4811`，注释写的是「收起后清除页签记忆」）；
  - 外部视图的 `toggleMatch`（`gameplay.js:8155–8157`）；
  - 切换战绩筛选时的 `openMatches.clear()` / `matchDetailTabs.clear()`（`gameplay.js:4479–4480`）。

### 修改

1. 上面三处在清 `matchDetailTabs` 的同时，清掉 `tab.buildPlayers` 里对应的 `gameId`；筛选切换时整表 `clear()`。
2. 浮层玩家、职业选手战绩等其他用到 `buildPlayers` 的视图，也全部检查一遍，在账本里列出来。
3. 战绩卡展开期间，在「概览 / 队伍分析 / 构建」之间来回切换，选中的英雄保留（这一点不变）。只有收起或换筛选才复位。
4. 加诊断 `build_player_selection`：`reason` 取值 `select` / `reset-collapse` / `reset-filter`，带 `game_id` 的哈希和 `is_self`，方便以后从日志定位。

### 测试（Node）

- 展开战绩卡 → 构建 → 选对面第 3 个英雄 → 收起 → 展开 → 构建：选中的是自己（`.build-player.is-selected` 带自己的标记 `.build-player-self`）。
- 展开期间切到「概览」再回「构建」：仍然是刚才选的英雄。
- 切换战绩筛选后再展开：选中的是自己。
- 外部视图的 `toggleMatch` 同样测一遍。

---

## 收尾

1. 最后一次改代码之后跑：
   - `go test -count=1 ./backend` 全量（记录开始时间、耗时、最后 5 行）；
   - `go vet ./backend`；
   - 本单涉及的 `-race` 专项（P1 竞态、P8 退避）；
   - `backend/web` Node 全量；
   - `desktop/overview-render.test.cjs`。
2. **截图**（GPT 执行，放到 `docs/history/reports/r231/`），深色和浅色主题各一套：
   - 英雄数据表：展开一行，含「显示更多对位」，1280 宽；
   - 二级表格开合逐帧检查（录屏或连续截图）；
   - 「英雄胜率」「英雄熟练度」标题，1280 / 780 两种宽度；
   - 构建切换栏悬停在头像中心时的提示；
   - 二/三/四/五杀标签；
   - 外服数据表的「无数据」和「失败」两种状态；
   - 海斗隐藏分弹层，正常和失败各一张。
3. **账本**写进 `docs/r231-execution-ledger.md`，内容包括：
   - P1 的标签分页探测结果；
   - P2 改前 / 改后的字节数和时间；
   - P5 的箭头量值；
   - P9 的文案替换表；
   - P10 的退出信号实测；
   - P13 的对照结果；
   - P14 检查过的 `buildPlayers` 使用点。
4. `docs/WORKLIST-INDEX.md` 加 R231 行。
5. **按用户本次真机反馈更新索引里相关行的状态**（只改状态列，不改历史账本）：
   - R211、R214、R216、R217、R218、R219、R221、R223、R229、R230 的真机项：用户确认通过；
   - 例外一：R230 P2 赛季场数转到本单 P1；
   - 例外二：R230 P12 英雄胜率慢转到本单 P2；
   - R224、R230 P10（斗魂相关）：用户尚未验证，保持「待真机」；
   - R212：`uninstall_old_ms` 达标；`parent_exited → uninstall_old_start` 3.19 秒未达标，转到本单 P13；
   - R213 / R215（选人详情录屏）：尚未验证，保持「待真机」。

## 用户真机验收

1. 等后台统计完成后，英雄数据表的灵活组排场数等于客户端的 313 场（或者离得很近，差额是本赛季更早、上游已经拿不到的对局）（P1）。
2. 打开一个没看过的国服玩家，英雄胜率卡片几秒内就有数据，之后原地变多，整个总览不重新加载（P2）。
3. 进出英雄数据表、熟练度页，返回后总览瞬间出现，图片不闪（P3）。
4. 二级表格（P4）：
   - 竖线从对位行一直连到「显示更多对位」；
   - 按钮和头像左对齐；
   - 同时只开一个；
   - 开合不闪。
5. 两个卡片标题的箭头和文字垂直居中、稍大，离文字更近；熟练度显示「N 个英雄」（P5）。
6. 构建切换栏把鼠标放在头像中间，显示玩家名（P6）。
7. 五杀标签金底深字，看得清；三、四、五杀文字颜色各不相同（P7）。
8. 韩服玩家查询失败后点「重试」能马上恢复，不用等 5 分钟（P8）。
9. 日服本人总览 2 秒内出来；日服英雄数据表能显示，或者显示「本赛季没有该模式对局」，提示里写的是「日服」（P9）。
10. 关客户端后总览消失明显更快（P10）。
11. 国服 ↔ 外服来回切一次后，海斗隐藏分能正常显示分数（P11）。
12. 战绩列表翻到 200 场后滚动不卡（P12）。
13. 构建页选了别的英雄，收起再展开后回到自己的英雄（P14）。
14. 斗魂相关（R224 / R230 P10）仍待验证：开一局斗魂，确认自己的小队排前三；重启应用后小队不被打散。
