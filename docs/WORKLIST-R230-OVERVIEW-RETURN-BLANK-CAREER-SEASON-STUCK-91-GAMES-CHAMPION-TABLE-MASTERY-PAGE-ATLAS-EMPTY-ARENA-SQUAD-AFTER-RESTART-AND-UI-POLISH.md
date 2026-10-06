# WORKLIST-R230：从详情页返回后总览左栏消失；赛季统计卡在 91 场；英雄数据表和熟练度页名称显示「英雄 804」；海克斯图鉴返回后为空；斗魂更新重启后小队被打散；总览多处样式调整

诊断人：Claude，依据：
- 用户 8 张截图；
- 日志 `lol-loot-diagnostics-1006-0109.jsonl`：0.12.73 → 0.12.74，16:14Z 在线更新，到 17:09Z 结束；
- 当前工作区源码，基线 0.12.74。

执行人：GPT。日期：2026-10-06。

构建 / 发布按用户当时的指示执行。所有截图由 GPT 在应用里实际操作后截取。

**优先级**：P1、P2 是功能问题，先做；P3、P9、P10、P11 是缺陷；其余是样式和交互调整。

---

## P1　从英雄数据表 / 熟练度页返回总览，左侧整列消失（严重，图三）

### 根因（已在源码确认）

1. `openOverviewSubpage` / `renderOverviewSubpage`（`gameplay.js` 约 2694–2722 行）打开详情页时，用 `container.innerHTML = …` 把整个总览容器替换掉了。
2. 但这个容器上的渲染缓存没有清掉，包括：
   - `container.dataset.matchTierScope`
   - `container._careerSignature`
   - `container._careerChildHTML`
   - `_stripHTML` 等
3. 点「返回总览」后，`renderOverviewBodyContent`（约 2063、2074 行）的判断出了问题：
   - `sameOverview` 仍为真，`_careerSignature` 也没变，于是 `careerSections = ""`，认为左栏不用重新生成。
   - 它想复用旧的 `.career-column`，但 `container.querySelector(".career-column")` 返回 `null`，因为旧左栏已经被详情页替换掉了。
   - 结果新 HTML 里的左栏是空的，旧左栏也不存在，左栏就是一片空白。
4. 战绩列表和顶部横幅不受影响，因为 `retainedMatchList` / `retainedStrip` 找不到时会走完整渲染。只有左栏的复用条件里同时用了签名判断，所以只有左栏出事。

### 修改

1. 详情页渲染前，清空 `activateOverviewView` 里列出的所有容器缓存字段（`_stripHTML`、`_backgroundArt`、`_careerSignature`、`_careerChildHTML`、`_careerHTML`、`_matchList*` 等），并删除 `dataset.matchTierScope`。可以抽一个 `resetOverviewRenderCache(container)`，`activateOverviewView` 切换页签时复用同一个函数。
2. 兜底：`renderOverviewBodyContent` 判断复用左栏时，签名一致但 DOM 里找不到 `.career-column`，必须当作没有缓存，重新生成 `careerSections`。横幅、战绩列表的复用分支也按同样规则检查一遍。
3. 返回后滚动位置仍恢复到进入详情页之前的位置（现有 `overviewReturnScroll` 逻辑不变）。

### 测试（Node + jsdom）

- 总览渲染完成 → 打开英雄数据表 → 返回：`.career-column` 子节点数与进入前相同，排位、英雄胜率、熟练度等卡片都在。
- 熟练度详情页、浮层玩家（`.player-overlay-scroll`）各做一遍。
- 连续进出 3 次，左栏不丢、不重复。

---

## P2　赛季统计只有 91 场，被当成「已完整」（英雄胜率卡、英雄数据表数据很少）

用户问「这个页面的数据是整个赛季的吗，感觉很少」。设计上应该是整个赛季，现在不是。

### 证据（日志）

| 时间（UTC） | 版本 | 事件 |
|---|---|---|
| 14:51:17 | 0.12.73 | `season_stats_head_refresh scanned=0 complete=false`（「已统计 0 场，正在后台补全本赛季」） |
| 14:51:27 | 0.12.73 | `season_backfill_round scanned=1 total_games=1 complete=true` |
| 16:14:04 | 0.12.74 | `season_stats_head_refresh fresh=false scanned=49 complete=true` |
| 16:18–17:07 | 0.12.74 | 之后每次都是 `scanned=90/91 complete=true`，再没有出现 `season_backfill_round` |

- `season_ranked_snapshot`：`solo=0 flex=33 mayhem=40`。
- 截图一的排位卡显示单双排 41 胜 39 负、灵活组排 150 胜 163 负，本赛季排位至少 393 场。
- 英雄数据表「排位」页只有 33 场，正好等于 flex=33。

### 根因

1. 0.12.73 的 SGP 解析错误（R223 P1）每页只解析出 1 场，上游 `more=false`。
2. `seasonScanPagesWithHistoryCache`（`season_stats.go` 约 908–912 行）据此把 `cache.Complete` 置为 true，写进了磁盘。
3. 0.12.74 修好了解析，但没有升级赛季缓存版本（`seasonStatsCacheSchemaVersion` 仍为 12）。
4. 头部增量扫描撞到缓存里那 1 场就停（`headScan && cachedHit`），`Complete` 沿用旧值 true，于是 `startSeasonBackfill` 永远不会再触发，本赛季更早的对局永远补不上。
5. 所有在 0.12.73 下打开过总览的国服账号都受影响。

### 修改

1. `seasonStatsCacheSchemaVersion` 12 → 13，同步修改 `season_stats_test.go` 第 499 行的断言。升级后所有旧赛季缓存失效，后台重新做一次整季回补。
   - 回补代价：代码注释记录过单账号约 51 页、159MB、24 秒，在后台执行。
   - 回补期间英雄胜率卡显示「已统计 N 场，正在后台补全本赛季」（现有文案），不阻塞总览。
2. 防止再次出现「上游数据异常 → 永久标记完整」：
   - 回补时，只有满足以下任一条件才置 `Complete=true`：扫到早于赛季开始（`seasonStartS26` = 2026-01-08）的对局；或者上游明确没有更多数据，并且最后一页解析出的对局数与上游返回条数一致。
   - 如果上游返回了 N 条但解析出的少于 N 条（解析丢弃），记 `season_scan_parse_dropped`（`returned`、`parsed`、`resume_index`），保持 `Complete=false`，下次继续。
3. 回补完成后，前端要立刻刷新英雄胜率卡。
   - 现在 `handleSeasonProgress` 只在 `complete` 从 false 变 true，或 `scanned` 增加 ≥100 时才刷新。
   - 缓存失效后第一次加载是 `complete=false`，回补完成时会触发刷新，这条路径要有测试覆盖。

### 测试

- **Go**：构造一个旧版（schema 12）`Complete=true`、只有 1 场的缓存，加载时必须被丢弃并触发回补。
- **Go**：上游返回 50 条、解析出 1 条时，`Complete` 保持 false，并记录 `season_scan_parse_dropped`。
- **Go**：回补扫到 2026-01-08 之前的对局才置完整。
- **真机**（GPT 打包后用户执行）：升级后打开自己的总览，等后台补全结束，英雄胜率卡标题的场数应接近排位卡单双排加灵活组排的总场数，再加上海斗场数。

---

## P3　英雄数据表、熟练度详情页的英雄名都显示「英雄 804」；熟练度页「专精英雄 172 / 0」（图四、图七）

### 根因

- `handleGameplayChampionTable`（`champion_table.go` 372 行）和熟练度详情（`mastery_details.go` 120 行）都用 `a.championNames()`（`gameplay.go` 1159 行）取英雄名。
- 这个函数只从 `a.allSkins`（收藏页皮肤数据）里取英雄名。日志里收藏数据加载一直失败：16:24:42、16:50:31、16:51:45、16:51:53 四次 `collection_data_retry_exhausted attempts=3`。皮肤表为空时英雄名就全部为空，`championName()` 退回成「英雄 %d」。
- 熟练度页分母 `totalChampions` 也按同一个空表计数，所以显示「172 / 0」。
- 总览本身用的是 `overviewChampionNames(ctx)`（`gameplay.go` 2229 行），不依赖收藏数据，所以总览里英雄名正常。

### 修改

1. 英雄数据表、熟练度详情改用和总览相同的 `overviewChampionNames(ctx)`。
2. `totalChampions` 取英雄目录里的英雄总数，不依赖皮肤表。
3. 全局搜索其他调用 `a.championNames()` 的接口，凡是展示给用户的英雄名都换成同一个名称来源。
4. 名称来源为空时，记一次 `champion_names_empty`（`endpoint`、`source`）。

### 测试

- **Go**：`allSkins` 为空时，`/api/gameplay/champion-table` 和 `/api/gameplay/masteries` 返回的 `championName` 都是中文英雄名，`totalChampions > 0`。

---

## P4　英雄数据表交互与样式（图四）

用户要求，逐条执行：

1. **去掉「排位」页签**，只保留「单排/双排」「灵活组排」「海克斯大乱斗」。
   - 默认打开单排/双排；本赛季单排为 0 场而灵活组排有场次时，默认打开灵活组排。
   - 后端 `queue=ranked` 分支保留兼容，前端不再请求它。
2. **排序**：现在已经按场次从多到少排了（表头「场次 ↓」），但场次相同时按英雄 ID 排，看起来像乱序。
   - 改为：场次相同按胜率从高到低，再按 KDA 从高到低。
3. **点击英雄所在行的任意位置都能展开 / 收起二级表格（对位）**，鼠标悬停时变成手型。
   - 删掉最右侧的展开按钮和那一列，`colspan` 同步减一。
   - 键盘可达：行设 `tabindex="0"` 和 `aria-expanded`，Enter 或空格切换。
   - 没有对位数据的行不显示手型，点击不响应。
4. **二级表格向右缩进**：
   - 对位行的英雄列左侧留出约 24px 缩进，左边加一条 2px 的主题色竖线，和主行区分开。
   - 「显示更多对位」按钮同样缩进。
5. **场次列不再用红色**：
   - 胜率 ≥60% 的强调色改为绿色（`.is-hot`，现在是 `#d44747`）。
   - 进度条改为：胜场部分绿色，负场部分中性灰。现在底色是 `#bb4c5c` 红、填充是蓝。
   - 浅色和深色主题下对比度都 ≥3:1。
6. **KDA 之后的每一列，前三名用主题色显示**：
   - 范围：评分、伤害、承伤、守卫、补刀、金币、双杀、三杀、四杀、五杀。
   - 只比较每格的第一行数值，越大越好。
   - 只在英雄主行之间比较，「所有英雄」行和对位行不参与。
   - 空值和 0 不参与。并列时一起高亮，最多高亮前三个不同的数值。
   - 用主题色加粗，不另加图标或说明文字。
7. **「‹ 返回总览」按钮加大**：图标至少 18px、字号 14px、点击区域至少 32px 高。熟练度页同样修改。
8. 英雄名显示依赖 P3。

**测试（Node）**：
- 页签只有三个。
- 场次相同按胜率排序。
- 点击行内非按钮区域切换展开；最右侧按钮列不存在。
- 每列前三名带高亮 class，「所有英雄」行不带。
- 进度条和胜率文字不含红色。

---

## P5　熟练度详情页（图七、图八）

用户问「两个点是 S 评分的意思吗」。是的：这几个点是距离下一个赛季里程碑还需要的 S 级评价数量（`masteryMarks` 取 `nextSeasonMilestone.requireGradeCounts`），实心表示已获得。

修改：

1. **头像放大**：
   - 现在 `--mastery-portrait-size: 56px`，改为 72px，卡片和网格列宽同步放大。
   - 等级数字移到徽章下方，或者放在徽章底边以外，不能盖住英雄头像和徽章本体。
   - 不再出现图七那种等级数字压在徽章中间的情况。
2. **把点换成标记图标**，样式参考图八 OP.GG 的标记：
   - 未获得：灰色描边。
   - 已获得：主题色实心。
   - 不要直接使用 OP.GG 的图片资源。优先用客户端自带的里程碑标记图（CommunityDragon `rcp-fe-lol-collections` 下与 mastery mark / token 相关的图），找不到就用内联 SVG 的菱形或星形标记。
   - 图标大小 12–14px。
3. **悬停提示框**：把「本赛季最高评价」和点状标记改成 OP.GG 那种字母方块。需要几个 S 就显示几个带「S」的小方块，已获得的高亮，右侧写「里程碑 N」。保留下面这些内容：
   - 进度条；
   - `已获得 / 需要 点数`；
   - 最近游玩日期。
4. 「专精英雄 X / Y」的分母问题见 P3。

**截图**（GPT 执行）：熟练度页 1280 宽一张、悬停提示一张，深色和浅色主题各一套。

---

## P6　总览卡片标题栏、卡片顺序（图六）

1. 「英雄胜率」「英雄熟练度」标题栏右侧的文字（如「S26 · 已统计 91 场」「最高分」）和箭头按钮，与左侧标题垂直居中对齐。
   - 现在 `header span` 是行内元素，28px 高的按钮把基线撑偏了，改成 `display:inline-flex; align-items:center; gap:6px`。
2. 箭头按钮 `.overview-detail-arrow`（`gameplay.css` 2041 行）：
   - 去掉边框和背景；
   - 箭头加大到约 24px、加粗（`font-weight:700`，或改用 2px 描边的 SVG 箭头）；
   - 悬停时只改颜色；
   - 点击区域保持至少 28px。
3. **英雄熟练度和位置偏好互换位置**：`careerSectionEntries`（约 1953–1955 行）的顺序改为 `champions → masteries → positions`。生涯统计弹窗 `renderCareerDialogSections` 的 `secondaryKeys` 同步调整顺序。

---

## P7　战绩卡的名次 / 多杀 / 关键词标签放回装备栏右侧（图一）

- 现状：`.match-build .match-badges { flex:0 0 100% }`（`gameplay.css` 2140 行）强制标签单独占一行。
- 修改：
  - 标签和装备格同一行，紧跟在装备格右侧（`flex:0 1 auto`）。
  - 只有剩余宽度放不下时才整体换行，换行后左对齐。
  - 装备格不能被挤压变小。
- 验收截图（GPT 执行）：1280、1000、780 三种宽度，含海斗卡（标签较多）、5v5 卡、斗魂卡。

---

## P8　构建页英雄切换栏的间距和分割线（图二）

- 现状：`.build-player-selector { margin:0 0 20px }`，上方紧贴「概览 / 队伍分析 / 构建」页签，下方是 20px 外边距加 7px 内边距，所以上下间距不一致。
- 修改：
  1. 切换栏上下间距相等，建议各 12px。
  2. 顶部加一条 1px `var(--line)` 分割线，把切换栏和上方页签分开。
  3. 头像左下角的等级数字（`.build-player small`）要完整显示，不能被下方的蓝 / 红队伍线遮住。
- 斗魂 16 人的布局（`.is-arena`）同步检查。

---

## P9　海克斯图鉴：去别的页面再回来，显示「0 个海克斯 / 没有匹配的海克斯」（图五）

### 根因（源码确认）

- `state.mayhemView`（「英雄榜 / 海克斯图鉴」）保存在设置里，回来时仍然是 `atlas`。
- 但 `state.augments` 会被清空：`switchChampionMode` 第 145 行切换模式时置为 null；应用重启后它本来也是 null。
- 整个 `champions.js` 只有两处会调用 `loadMayhemAtlas`：点击「海克斯图鉴」页签（2827 行）和点击重试（2889 行）。所以以图鉴视图重新进入时不会发起请求。
- `renderMayhemAtlas`（1391 行）在「不在加载、没有错误、`augments` 为空」时，直接把空列表渲染成「0 个海克斯，没有匹配的海克斯」。

### 修改

1. `render()` 渲染图鉴视图时，如果 `mode === "aram-mayhem"`、`mayhemView === "atlas"`、没有 `augments`、不在加载中，就调用 `loadMayhemAtlas()`，期间显示骨架屏。
2. `renderMayhemAtlas` 遇到 `augments == null` 时一律显示骨架屏，不显示「没有匹配」。「没有匹配」只在 `augments.rows` 非空、但被筛选条件过滤成 0 条时出现。
3. 回到图鉴时保留用户的搜索词、品质筛选和选中的海克斯；右侧详情按选中项重新加载。

### 测试（Node）

- 以 `champion-mayhem-view=atlas` 初始化，或从单双排切回海克斯大乱斗时，会请求一次 `/api/champions/augments`，渲染出列表。
- `augments == null` 时不出现「没有匹配的海克斯」。

---

## P10　斗魂：进入游戏后自己小队的队友被打散，没有排在前三

### 结论（回答用户）

- **其他小队**：之前已验证做不到。对局中客户端的 playerlist 和 session 都不提供小队归属：日志 16:32:38 `arena_group_truth_check` 显示对局中所有 `subteam_id` 都是 0，只有赛后 SGP 才有。
- **自己的小队**：可以做到。英雄选择阶段客户端会给出本小队成员，R224 P5 已经实现「自己 → 两名队友 → 其他人」的排序，在 0.12.74 里。

### 这次被打散的原因

- 这一局（gameId 9017164496）在 16:03Z 开始，用的是 0.12.73。16:14Z 应用在对局中在线更新并重启。
- 英雄选择阶段记下的小队成员（`a.arenaAllyPlayers` / `a.arenaAllyGameID`，`gameplay.go` 6727–6741、7833 行）**只在内存里，重启就丢了**。
- 重启后 `markRememberedArenaSquad` 找不到队友：日志里 16:14:06 是 `order-inference-disabled`，16:14:42 渲染 18 人，没有小队标记。
- 另外，英雄选择发生在 0.12.73 下，当时还没有 R224 的排序逻辑。

### 修改

1. 英雄选择阶段确定小队成员后，把「gameId + 成员身份（PUUID、summonerId、名字#tag）+ 记录时间」写到磁盘（应用数据目录，小文件）。
2. 应用启动、或对局页首次加载时，如果当前对局的 gameId 与磁盘记录一致，就恢复到内存，再执行 `markRememberedArenaSquad`。
3. gameId 不一致，或记录超过 2 小时，就删除这条记录。
4. 恢复时记 `arena_squad_restored`（`game_id`、`members`、`source=disk`）。
5. 排序沿用 R224 P5 的规则：自己 → 两名队友 → 其他人，组内按用户选择的排序方式。

### 测试

- **Go**：写入小队记录 → 模拟重启（新建 app 实例）→ 同一 gameId 的对局响应里，3 人带 `mySquad`。
- **Go**：gameId 不同时不恢复，记录被清除。
- **真机**：斗魂选完英雄进入对局后，退出并重新打开应用，对局页前三行仍是自己的小队。

---

## P11　总览点击召唤师名字没有复制成功；复制后要有提示

### 现状

- `bindSummonerCopy`（`gameplay.js` 8691 行）直接调用 `navigator.clipboard.writeText`，失败时 `.catch(() => {})` 静默吞掉错误，也没有任何成功提示。
- 日志里没有相关记录，无法判断失败原因。

### 修改

1. 复制成功后，调用页面已有的右下角提示 `showToast`（`#toast`，`app.css` 889 行），文案「召唤师名称和编号已复制」，**3 秒后自动消失**。给 `showToast` 加一个时长参数，其他调用保持原来的时长。
2. `navigator.clipboard.writeText` 失败时：
   - 改用 Electron 主进程剪贴板：preload 暴露一个 `copyText` IPC，主进程调用 `clipboard.writeText`。
   - 再失败就用 `document.execCommand("copy")`。
   - 全部失败时提示「复制失败」。
3. 记 `summoner_copy`（`ok`、`method`、`error_name`），不记录名字本身。
4. 浮层玩家页和对局页里，凡是带 `data-copy-summoner` 的地方行为一致。
5. **GPT 在真机上排查**：点击后剪贴板里没有内容的原因（窗口焦点、提示层拦截点击、权限），写进账本。

### 测试（Node）

- 模拟 `clipboard.writeText` 成功：出现提示，3 秒后隐藏。
- 模拟失败：走下一级复制方式。

---

## P12　总览「英雄胜率」出来慢

### 日志能确认的事实

1. **国服本人总览**：
   - 15 次 `overview_load_cost`（非流式）大多在 0.77–2.6 秒之间，有一次 6.1 秒。
   - 英雄胜率的数据来自磁盘快照（`season_snapshot` 阶段 1–3ms），和总览在同一个响应里返回，后端没有单独等待。
2. **每次打开总览都会重新下载 SGP 战绩**：
   - `overview_load_cost` 的 `sgp_bytes` 为 7.6–7.8MB，`sgp_history_cache_hits` 全部是 0。
   - 紧接着 `season_stats_head_refresh fresh=true` 又下载一次 5.7–6.1MB。
   - 1 小时内总览下载约 101MB，头部扫描下载约 78MB，合计约 180MB。
   - 原因是 `overviewFreshHistory(ctx)` 让这次后台头部扫描绕过了历史缓存。
3. **外服（韩服）玩家**：英雄胜率等 OP.GG 赛季汇总，`opgg_season_summary_cost` 耗时 2.6–4.9 秒，在总览出来之后才显示。
4. **渲染**：0.12.74 刚启动时，`renderer_perf` 中 overview 的最长任务 461ms。
5. **更新后第一次打开**：英雄胜率是旧缓存的 49 场；赛季进度事件需要 `scanned` 增加 ≥100 才刷新，所以 49 → 90 不会自动更新，要等下一次重新加载。P2 修好后这个问题会一起消失。

### 修改

1. 加前端计时 `overview_card_ready`：从页签开始加载到每张卡片（排位、英雄胜率、熟练度、位置）首次带数据渲染的毫秒数，以及数据来源（`snapshot`、`network`、`opgg`）。用来确认用户感觉到的慢具体在哪一步，写进账本。
2. 总览自身加载完成后的后台头部扫描使用历史缓存（`useHistoryCache=true`），只有用户点「刷新战绩」时才 fresh。目标：连续两次打开同一账号的总览，第二次 `sgp_history_cache_hits > 0`，总下载量降到第一次的一半以下。
3. 外服玩家：OP.GG 赛季汇总在总览请求发出的同时就开始请求，不要等总览回来再发；有上次缓存时先显示缓存（现有 `opggSeasonStale` 逻辑），新数据回来后再原地替换。
4. 461ms 的长任务：用 Performance 面板定位是哪一段渲染，拆分或延后到空闲时执行，目标单次长任务 ≤200ms。

---

## 收尾

1. 最后一次改代码之后跑：
   - `go test -count=1 ./backend` 全量（记录开始时间、耗时、最后 5 行）；
   - `go vet ./backend`；
   - 本单涉及的 `-race` 专项；
   - `backend/web` Node 全量；
   - `desktop/overview-render.test.cjs`。
2. **截图**（GPT 执行，放到 `docs/history/reports/r230/`），深色和浅色主题各一套：
   - 总览左栏，进出英雄数据表各一次后；
   - 英雄数据表：1280 宽展开一个英雄；
   - 熟练度页和悬停提示；
   - 战绩卡：1280 / 1000 / 780 三种宽度；
   - 构建页切换栏；
   - 海克斯图鉴：从单双排切回之后；
   - 总览卡片标题栏。
3. 账本写进 `docs/r230-execution-ledger.md`；`docs/WORKLIST-INDEX.md` 加 R230 行。

## 用户真机验收

1. 打开英雄数据表或熟练度页再返回，总览左栏完整（P1）。
2. 升级后等几分钟，「英雄胜率」标题的场数接近本赛季实际场数；数据表里能看到更早的英雄（P2）。
3. 数据表和熟练度页显示中文英雄名；熟练度页分母不为 0（P3）。
4. 英雄数据表（P4）：
   - 只有三个页签；
   - 点击行展开对位；
   - 场次和进度条是绿色；
   - 每列前三名高亮。
5. 熟练度页（P5）：头像变大，等级数字不挡头像，标记是图标，悬停提示里是 S 方块。
6. 卡片标题对齐，箭头无边框；熟练度在位置偏好上面（P6）。
7. 战绩卡标签在装备右边（P7）；构建页切换栏上下间距一致，并有分割线（P8）。
8. 去别的页面再回到海克斯图鉴，列表正常（P9）。
9. 斗魂对局中重启应用，自己的小队仍排在前三（P10）。
10. 点名字后右下角出现「召唤师名称和编号已复制」，3 秒后消失，并且能粘贴出 `名字#编号`（P11）。
