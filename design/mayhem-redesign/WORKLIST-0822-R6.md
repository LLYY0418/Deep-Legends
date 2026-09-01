# 海克斯大乱斗 / 斗魂竞技场 · 第六轮工单（2026-08-22）

> 来源：8 张截图 + 你写的 19 条修改要求。
> 目标：修展示逻辑、修文案、让海斗/斗魂的卡面和目录页风格统一，同时继续排查 hexdata 数据仍空的问题。
> 仍在生效的红线：hexdata 不打包、不预热、不伪造 UA；上游字符串进 innerHTML 前一律 `escapeHTML`；变异测试必须 `cp` 真副本、副本带仓库根 `*.go`。

---

## 〇、最紧急：hexdata 为什么还是空

图一「装备排行」仍然是 0 件、海克斯推荐也不像是 hexdata 的来源——先把这条根因落实，再改 UI。

### 0.1 真根因（2026-08-22 线上实证）：hero 详情页指标正则分隔符不匹配（`·` vs 全角逗号），每次请求确定性解析失败

**用户的删文件测试 = 关键实验**：删 `hexdata-state.json` + 重启后装备排行仍空。这说明熔断状态文件**不是**当前症状的原因 —— 熔断是**每次请求当场新拉**的，删了下次立刻又拉。真因在解析器本身。

#### 证据链

1. **线上页面是好的**（`web_fetch` 实测 `https://hexdata.com.cn/hero/67-vayne` HTTP 200，build `hexdata-2026-08-19-2a6a2393a083`，Patch 16.16，报告日 2026-08-19）：
   - `data-primary-content` 下正好 **2 张表**：第一张 8 行海克斯（全带 `/augment/NNNN-slug` 链接、4 格），第二张 8 行装备 → 与解析器 `len(tables)==2`、8+8 假设**完全吻合**，之前「16 行 vs 8」的说法作废，`rows=16` 就是 8+8。
   - 指标文案统一是 **全角逗号**：`胜率 57.8%，样本 1,720,638 场，层级 T1`（meta description、正文 `<p>`、FAQ、JSON-LD 全部如此，`57.8%` 与 `，` 之间无空格）。页面里唯一的 `·` 在 `<title>`/`og:title` 的「· Patch 16.16」，后面跟的是 Patch 不是 样本。
2. **正则只认中圆点**：`hexdata.go:47` `hexdataHeroMetricPattern = regexp.MustCompile(`胜率\s*([0-9]+(?:\.[0-9]+)?)%\s*·\s*样本\s*([0-9,]+)`)` —— `%` 后必须紧跟 U+00B7 `·`。页面用 U+FF0C `，` → **永不匹配** → `WinRate=0, Games=0`。
3. **闸门确定性失败**：`hexdata.go:912` `if len(Augments) != 8 || len(Items) != 8 || result.WinRate <= 0 || result.Games <= 0` → 8+8 结构过了，但 WinRate/Games 双双 0 → `recordShapeFailure("hero", 16, 0, ...)`（`hexdata.go:1180`，fields 传字面量 0）→ 熔断当场重开（1m 退避）。
4. 因此 `response.ItemRanking = primary.Items`（`hexdata.go:1194`）永远拿不到 → 截图「装备排行 0 件」。

**为什么删 state 文件救不回来（机制解释）**：`hexdata-state.json` 只挡 HTTP 层（`circuitUntil`）。解析失败发生在代码层、确定性发生：删了文件 → 下次打开英雄详情 → 解析又失败 → `recordShapeFailure` 当场重开熔断 → 排行还是空。除非修解析器，否则删多少次都一样。

#### 改法（必改，否则永远拿不到 hexdata 装备排行）

1. `hexdataHeroMetricPattern`（`hexdata.go:47`）接受 `·`/`，`/`,`：改成 `胜率\s*([0-9]+(?:\.[0-9]+)?)%\s*[·，,]\s*样本\s*([0-9,]+)`（页面 `57.8%，样本` 无空格，`\s*` 正好兼容）。或更稳：WinRate/Games 直接由已解析的 ranking 行推导（表里每行都有 `63.6%` 和 `23,765`），不依赖页面散文。
2. 结构闸门放松：`len(tables) != 2` → `>= 2`；`!= 8` → `>= 8`（或 `> 0` 交给 `hexdataMinimumSample` 过滤）。
3. `recordShapeFailure` 的 `fields`（`hexdata.go:1180` 字面量 0）改传实际字段数（如 `len(primary.Augments)`），否则下次出问题日志又是 0。
4. 熔断吸收态修复（R5 §A，放大器，必须一起上）：`recordSuccess` 清 `CircuitUntil` + `CircuitProbeAt`（`hexdata.go:512-521`，当前只清 `Failures`）+ 半开探测。

#### 验收

修完后删 `hexdata-state.json` 重启 → 打开任一英雄详情 → 装备排行应有值；诊断日志应出现 `hexdata_shape kind=hero` 且 rows/fields 为实际值，不再有 `hexdata_shape_invalid kind=hero`。

### 0.2 待用户补的确认（可选）

重启后给我一份新鲜诊断日志（浏览器直开 `http://127.0.0.1:8793/api/diagnostics/log` 下载），确认 `hexdata_shape_invalid kind=hero rows=16 fields=0` 是否随每次英雄详情请求重现 —— 以此闭环「解析器确定性失败」这个判断。

---

## 一、六条 UI / 文案（只改展示，不动取数）

### A. 海克斯推荐：把数字排名换成档位徽章，一行三个，共九个

**你的要求**：不要 1/2/3 这种数字；换成 S / A / B 这种档位标签，放在海克斯名称的左侧；每行 3 个，只展示 9 个，海斗和斗魂都一样。

**现状**：`renderMayhemAugmentPodium` 里还没有档位字段，当前只是数组 index+1；斗魂 `renderArenaOptionCard` 已经在用 `index+1`，也还没有档位。

**改法**：

1. 在后端 `recommendedAugments`（海斗）和 `arenaAugments`（斗魂）里，必须下发 `tier` / `grade` 字段。若上游（OP.GG / your.gg / hexdata）没有现成的 S-A-B 分级，就在前端按 HexScore 分位数临时映射（90%+ = S，70-89% = A，其他 = B）——同时在 `measurementTechnique` 里写明「当前档位为本地分位计算，非官方等级」，保证不会误导。
2. 前端把 `renderMayhemAugmentPodium` 与 `renderArenaOptionCard` 统一：徽章改为 `<b class="augment-grade is-S/A/B">${grade}</b>`，放在海克斯名称左侧；样式不要照搬 external site 的红底，保持项目已有的 `is-prismatic / is-gold / is-silver` 边框与文字颜色系统，只改左上角 badge 的内容。
3. 布局从 5 列变 3 列：海斗 `.mayhem-podium-grid` 改 `grid-template-columns: repeat(3, minmax(0,1fr))`；斗魂 `.arena-option-grid` 同样在推荐海克斯 section 里固定三列，只展示前 9 条（`.slice(0,9)`），同时保留展开按钮让用户看全部。
4. 海克斯名称与图标都保留，但去掉原来的排名数字 `<b>${index+1}</b>`。样式要在深色背景、金色/紫色/灰框下可读，字号不小于 12px，badge 宽高约 20–22px，用 `border-radius: 4px`。

### B. 出门装 / 召唤师技能 / 技能加点：补胜率，修召唤师技能图标

**图一显示**：四块区域右侧大面积空白；召唤师技能图标是占位 `1`；技能加点没有胜率数字。

**代码位置**：

- 开局配置：`renderMayhemOpeningConfiguration`（`champions.js:771`）
- 单个配置项：`renderConfigOption`（`:1413`）——目前只渲染图标，没有数值
- 召唤师技能数据来源：`build.summonerSpells`；图标字段来自 `row.assets[0]`

**改法**：

1. `renderConfigOption(row, kind)` 增加第二个参数 `showWinRate`：在按钮右侧加一个 `<span class="option-win-rate">${percent(row.winRate)}</span>`；海斗的出门装、鞋子、召唤师技能三个 group 都传 `true`。
2. 召唤师技能图标为空时的兜底：若 `asset.path` 或 `asset.source` 缺失，回退到 `imageURL("ddragon", "img/spell/" + asset.name + ".png")`（ddragon 的技能图标路径固定），再不行才显示技能名首字。现在直接渲染成 `1` 是因为 `assetImage` 拿到空 path 后把 `name`（可能是数字 id）当 alt 渲染了，用户看到的是 `alt` 文本。
3. 技能加点区域：`renderChampionSkillPlan` 里技能介绍那一行右边加 `<span class="skill-win-rate">${percent(skills.winRate)}</span>`。如果没有 `winRate` 字段，后端补上（从 hexdata 或 OP.GG 的 `skills` section 取）。
4. 召唤师技能不出现在技能加点区域——图一里它单独成列是对的，不要和技能加点混。

### C. 装备路线：把「召唤师技能 / 符文饼干」从路线里清出去，补胜率

**图三**显示：装备路线里出现了召唤师技能图标和符文里的饼干——这是最明显的数据错位。

**根因**：`renderMayhemItemRoutes` 里 `routes` 取自 `build.coreItems`，而 `coreItems` 的填充（`hexdata.go:1443`）是 `metricRowsFromIDs(rscRows(expanded, "core_items", 5), "item")`。问题大概率是 RSC 解析时 `core_items` 字段边界被误切（R5 §C 已提过 16000 字符窗口硬切），把后面 `summonerSpells` 或 `runes` 的 id 塞进了 core_items 数组。

**改法**：

1. 在 `hexdata.go` 的 `rscRows(expanded, "core_items", 5)` 返回后，追加一道过滤：只保留 `kind == "item"` 且 id 落在 `items.json`（ddragon）有效范围（1000–9999）内的行，遇到非法 id 记 `diag("rsc_core_item_rejected", id, reason)`。
2. `renderMayhemItemRoutes` 里把 `build.coreItems` 改成 `objectRows(build.coreItems).filter(row => row.assets?.[0]?.kind === "item").slice(0,3)`，双保险。
3. 装备路线右侧空着 → 每条路线的 `route-row` 末尾加 `<span class="route-win-rate">${percent(row.winRate)}</span>`。
4. 如果 `hexdata.go` 的 RSC 16000 截断是真因（上一轮已定位），这一轮把截断窗口改成按 `"$","tr"` 边界收尾（R5 §C），否则即使修了过滤也只是掩盖问题。

### D. 所有 section 副标题：去掉「按 OP.GG / 按 hexdata」

**你的要求**：「每个标题下面的介绍不要写成什么按 opgg、按 hexdata」。

**现状**：`renderMayhemOpeningConfiguration` 的副标题写死 `当前补丁的 OP.GG 海克斯大乱斗构筑`；`renderMayhemItemRanking` 写 `按 HexScore 查看当前英雄的高表现装备`；`renderMayhemAtlasDetail` 的适配英雄写 `按 HexScore、胜率与样本展示`。

**改法**：统一用描述性文案而不是暴露来源：

| section | 新副标题 |
|---|---|
| 开局配置 | 当前版本推荐出门装、鞋子与召唤师技能 |
| 装备路线 | 核心装备路线与后续成装顺序 |
| 装备排行 | 按综合评分排序的高表现装备 |
| 海克斯推荐（海斗） | 按综合评分、胜率与样本展示前九项 |
| 海克斯推荐（斗魂） | 按综合评分、胜率与样本展示前九项 |
| 适配英雄 | 按综合评分、胜率与样本展示 |

不要在任何一处出现 OP.GG / hexdata / your.gg 等来源名（F 表署名全删已拍板）。

---

## 二、五条展示细节

### E. 版本号

**问题**：海斗「英雄梯度」副标题是 `胜率与海克斯 · 当前版本`（`champions.js:728`），应该是具体的 patch 字符串。

**改法**：两处（海斗列表 `:728` + 弹窗 `:738`，斗魂 `:855`）一律用 `state.rankings?.patch || state.detail?.citation?.patch`，拿不到时才回退到 `当前版本`。日志里 `arena_augments_built` 已带 patch 字段，说明上游有，只要前端把它存进 `state.rankings` 就行。

### F. 斗魂 / 海斗英雄条高度对齐 + 斗魂加上英雄原画

**你的要求**：英雄条高度要一致；斗魂英雄条包括下面选用率/禁用率/胜率那行，那行高度可以小一点，让上面内容大一点；斗魂也把英雄原画加在上面。

**现状**：斗魂 `renderArenaDetailPane` 已有 `arena-overview-art`（原画 `<img>`），但英雄列表条 `renderChampionTable` 没有原画；海斗列表条有缩略图但没有原画背景。

**改法**：

1. 列表条固定高度：`.champion-row { height: 72px }`，海斗斗魂统一；下面的 secondary 行（选用率等）用独立的 `.arena-overview-secondary`，高度 `min-height: 36px; line-height: 1.2`。
2. 斗魂列表条左侧加 hero artwork 背景（与详情条一样的 `heroArtworkURL`），用 `position: absolute; inset: 0; object-fit: cover; opacity: .15`，保证不干扰文字。
3. 宽度问题：`.aram-champions` 与 `.mayhem-champions` 最小宽度从当前值提到 `320px`（PC 端），搜索框与排行表字号同步放大到 14px。
4. 斗魂二级指标行（选用率、禁用率、样本）的字号降到 12px，颜色用 `var(--text-muted)`，让主指标（胜率、名次）更突出。

### G. 斗魂 / 海斗「一命两人，八强争胜」文案

**你确认图六是对的风格但写错了**：斗魂竞技场是 3 人组队、6 强决出冠军，应为「一命三人，六强争胜」。

**改法**：`champions.js:661` 的 `modeTab("arena", "斗魂竞技场", "海克斯 · 棱彩 · 吃鸡战绩")` 改成 `modeTab("arena", "斗魂竞技场", "一命三人，六强争胜")`。

顺便检查其他两个模式的副标题：

| 模式 | 新副标题（建议） |
|---|---|
| 斗魂竞技场 | 一命三人，六强争胜 |
| 海克斯大乱斗 | 随机英雄 · 全员海克斯 · 快节奏团战 |
| （如果还有第三个 tab）| 按实际模式规则填写，不要暴露数据来源 |

### H. 海克斯图鉴右侧面板宽度 + S/A 标签 + globalHexScore 改中文

**你的要求**：右侧宽度太大，要小很多；左右一半一半；S/A 标签要好看；87.6 旁边的 `globalHexScore` 改中文。

**现状**：

- `champions.css:279` `.mayhem-atlas` 是 grid，但两列宽度是 `minmax(410px,1.15fr) minmax(330px,.85fr)` —— 左侧最小 410px 比右侧 330px 更宽，与你截图里「右侧太大」的观感一致，且窄屏下会溢出。
- S/A 标签在 `renderMayhemAtlasRow`（`champions.js:807-813`）里用 `<b>${augmentTierLabel(item.tier)}</b>` 收尾，样式是默认加粗，没有视觉包装。
- `renderMayhemAtlasDetail` 里性能指标写死 `globalHexScore`（`champions.js:820`）。

**改法**：

1. `champions.css:279` 改 `grid-template-columns: 1fr 1fr; gap: 16px;`，两列严格等宽；`.mayhem-atlas-list` 保持 `max-height: calc(100vh - 344px)` 内部滚动。
2. S/A 标签样式：新增 `.augment-grade { display: inline-flex; align-items: center; justify-content: center; width: 22px; height: 22px; border-radius: 4px; font-size: 12px; font-weight: 700; margin-right: 6px; }`；`.is-S { background: var(--prismatic); color: #fff; }` `.is-A { background: var(--gold); color: #000; }` `.is-B { background: var(--silver); color: #000; }`。这个样式也要同步到海克斯推荐卡片（A 节）。
3. `renderMayhemAtlasDetail` 里把 `globalHexScore` 改成 `综合评分`；适配英雄列表里的 `HexScore` 也统一改成 `综合评分`（`champions.js:830`）。

---

## 三、一个数据边界问题：your.gg 能不能补斗魂海克斯

你发来的 `https://your.gg/en/kr/augments` 是斗魂数据站。日志里已有 `api.your.gg` 被用作 `arena_first_places` 数据源（26 条事件，25 条 200，1 条 5006ms 超时）。

### 3.1 已知情况

| 项目 | 状态 |
|---|---|
| your.gg 被本项目调用 | 是，`arena_first_places` 模块已在用 |
| your.gg 站点上有 augments 页面 | 是，你截图里有海克斯图鉴（229 个海克斯，按梯度与品质排列） |
| 我能直接调 your.gg | 不能——沙箱打不通 127.0.0.1，外部网络无法确认你的鉴权方式与区域参数 |

### 3.2 需要你帮做的事（任选一种）

**方案 A（快）：** 在浏览器里打开 `http://127.0.0.1:8793/api/diagnostics/log`，然后在你的.gg 页面上点进任意一个海克斯（例如「闪现向前」），回到日志搜索 `your.gg` + `augment`，把最近一条 `champion_upstream` 贴给我。我需要看：host、path、status、duration_ms。

**方案 B（更完整）：** 打开浏览器开发者工具 → Network → 过滤 `your.gg`，点进任意海克斯，把请求 URL（完整 path + query）和响应 JSON 的前 30 行贴给我。

### 3.3 拿到 your.gg 的 augment API 后我能做的事

1. 确认它是否返回 `description`（中文或英文占位符文本）——如果返回的是和 cdragon 一样的 `@Key@` 模板，那你的.gg 只是转发社区源，没有额外价值。
2. 确认它是否返回 rarity / tier / icon ——如果返回了且比 cdragon 更完整（例如已经把占位符渲染好了），可以把 your.gg 加为斗魂 augment 的首选源，cdragon 降为 fallback。
3. 确认 id 体系是否与 cdragon 的 `augmentNameId` 一致——如果一致，直接用 id join；如果不一致，需要建 name → id 映射表。

**红线提醒**：your.gg 调用必须遵守已有边界——只走白名单接口、不打包进 EXE、不做预热、UA 诚实、不带 PUUID。如果 your.gg 的 augment 数据需要鉴权或有 rate limit，必须在 `hexdata-state.json` 的熔断逻辑之外单独处理，不能复用 hexdata 的熔断器。

---

## 四、优先级汇总

| # | 项 | 级别 | 前置 |
|---|---|---|---|
| 0.1–0.2 | hexdata 熔断排查 + 你补一行证据 | **P0（数据前提）** | 需要你操作 |
| A | 海克斯推荐改档位徽章 + 3列9个 | **P0** | 需后端下发 tier/grade |
| B | 出门装/召唤师技能/技能加点补胜率 + 修召唤师技能图标 | **P0** | — |
| C | 装备路线清非法 id + 补胜率 + RSC 截断 | **P0** | — |
| D | 副标题去掉来源名 | P1 | — |
| E | 版本号替换「当前版本」 | P1 | — |
| F | 英雄条高度对齐 + 斗魂原画 + 左栏加宽 | P1 | — |
| G | 模式副标题文案 | P1 | — |
| H | 海克斯图鉴左右分栏 + S/A 标签 + globalHexScore 中文 | P1 | — |
| 3 | your.gg 斗魂 augment API 探测 | **P0（数据前提）** | 需要你提供 API 响应 |

---

## 五、验收标准（每条改完必须过）

1. 海克斯推荐卡片没有 1/2/3 数字；S/A/B 徽章在名称左侧；每行 3 个；总共显示 9 个；斗魂与海斗一致。
2. 出门装 / 鞋子 / 召唤师技能右侧有胜率；召唤师技能图标正常显示（不是 `1`）；技能加点右侧有胜率。
3. 装备路线里没有召唤师技能和符文饼干；每条路线右侧有胜率；最多 3 条。
4. 所有 section 副标题不含 OP.GG / hexdata / your.gg 字样。
5. 英雄梯度副标题显示实际版本号（如 `25.S1.S3`），不是「当前版本」。
6. 斗魂 / 海斗英雄列表条高度一致；斗魂列表条有原画背景；左栏宽度 ≥ 320px。
7. 斗魂 tab 副标题为「一命三人，六强争胜」；海斗 tab 副标题不含「当前版本」。
8. 海克斯图鉴左右面板 1:1；S/A/B 标签符合新样式；详情页无 `globalHexScore` 英文。
9. `npm test` 全绿；变异测试至少覆盖 A/B/C 三项的主路径。

---

## 附：红线复核

- hexdata 白名单不变：只碰 `/api/hexdata/answer-cards` + 常规 HTML 页；不碰 `/api/` 其余、不碰 `/data/`。
- 不打包进 EXE；不做启动预热；不做定时后台同步。
- UA 保持诚实（用真实应用名 + 版本）；不伪装浏览器或爬虫。
- hexdata 请求链路不得出现 PUUID / summonerId / 召唤师名。
- 上游字符串进 innerHTML 前一律 `escapeHTML`。
- 变异测试 `cp` 真副本，副本带仓库根 `*.go`。
- your.gg 若新增为数据源，单独配置请求间隔/熔断，不复用 hexdata-state.json。
