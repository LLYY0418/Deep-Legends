# R116-B：P0 六项 + 英雄详情页三-tab 重整（含 P1-6 表现指标面板）

**目标：**
1. P0-1~P0-6：把 R116-A 已经拿到但还没渲染的字段（收益率/置信度/官方档位/召唤师技能胜率/阶段筛选）接到前端。
2. 把英雄详情页从 6 段线性堆叠改成 **概览 / 构筑 / 表现** 三个 tab，容纳 P0 新增内容与 P1-6（22 项表现指标）而不重演 R73「选手页密度/死区」问题。
3. 推荐页补上后端早就返回但前端零渲染的统计口径说明（`gameplay.go:5378` 的 `measurementTechnique`/`citation`）。

**背景：** `docs/r116-proposal-feasibility-review.md` 第 6 节判定：可行的增量 100% 落在既有容器里，但 P0 全六项加上表现指标面板会让详情页从 6 段涨到 9-10 段，是 R73 死区问题的复现路径。用户已裁决本轮就做 tab 化（2026-09-20）。

**依赖：** R116-A（`hero-json`/`postmatch`/`hextech-insights` 三条数据源必须先落地，本工单只做前端消费 + 少量后端字段透传）。

**版本：** 完成后升到 0.12.9。

---

## P0-1：收益率 `deltaWinRate` 全面展示

### 现状证据
`champions.js` 的海克斯卡（`renderRecommendedAugments`，约 1594-1611 行）与装备排行（`renderMayhemItemRanking`，约 1021-1029 行）目前只渲染 `row.winRate` 与 `row.games`，不渲染收益率。R116-A 的 `parseHexdataHeroJSON` 会把 `deltaWinRate` 挂到 `championMetricRow` 新字段上（详见 R116-A P1）。

### 实现要求
1. `championMetricRow` 新增 `DeltaWinRate float64 \`json:"deltaWinRate,omitempty"\`` 字段（若 R116-A 未加则本工单补上）。
2. 海克斯卡与装备排行行内新增一个「较基准 +X.X%」标签，正数用现有的胜率高亮色，负数用现有的低置信/警示色（复用样式，不新造配色）。
3. 装备排行额外展示 `withoutItemWinRate`（「不出这件时胜率 X%」）作为 tooltip 或副行——这是「出 vs 不出」对照，实测字段已在 `items[]` 里。

### 验证判据
- 装备排行任意一行同时显示 `胜率`、`较基准`、`不出时胜率` 三个数字，三者互不相同（用真实响应快照做 jsdom 冒烟测试断言 DOM 文本包含三段）。
- 收益率为 0 或字段缺失时不显示「较基准」标签（不显示 0.0%，避免误导）。

---

## P0-2：样本置信度可视化（低/中/高样本 + Wilson 下界）

### 现状证据
后端已有 `hexdataMethodology.samplePolicy`（低 <250、中 250-999、高 ≥1000，`hexdata.go` 校验用的门槛常量 `hexdataMinimumSample=250`）与每行的 `wilsonLowerWinRate`（R116-A 的新字段）。前端**除 `champions.js:1955-1959` 一处 OP.GG depth 路径的 `lowConfidence` 标记外，0 处展示 Wilson 下界或样本分档**。

### 实现要求
1. `championMetricRow` 新增 `SampleTier string \`json:"sampleTier,omitempty"\``（"low"/"medium"/"high"，由后端按 `Games` 与 `samplePolicy` 阈值计算好，前端不重复算阈值逻辑）与 `WilsonLowerWinRate float64`。
2. 低样本行（`sampleTier == "low"`）加一个「样本极少」徽记，复用 `champions.js:1955-1959` 已有的 `is-low-confidence` 样式类，不新造一套。
3. 高样本行（`sampleTier == "high"`）胜率数字旁可选展示 Wilson 下界作为 tooltip（「95% 置信区间下界 X%」），不需要单独一行。
4. **排序护栏**：任何按胜率排序的列表，低样本行不得排到高样本行前面——如果后端排序已经用 `sample_tier_then_wilson_lower_bound_v1`（Hexdata 原生排序）直出，前端不要再按裸 `winRate` 重新排序（检查 `sortedGradeRows` 等函数有没有偷偷重排，这是过去出过的问题，见 memory 里 R63「出装排序自己重排」）。

### 验证判据
- 构造一个「样本 10 场胜率 100%」和「样本 5000 场胜率 55%」的对照 fixture，验证前者带「样本极少」徽记且不会排在后者前面。
- `docs/r116-proposal-feasibility-review.md` 第 6.1 条要求统一：任何新维度取不到就整块隐藏，不留空态——sampleTier/wilsonLowerWinRate 缺失时对应展示元素不渲染，不显示 "undefined" 或 "NaN%"。

---

## P0-3：推荐页补统计口径说明 + 关闭遗留确认项

> **本节已被 2026-09-22 用户指示撤销（R128 §2.3/§2.5）。** 界面上不再添加「统计口径」、方法论与免责类说明：`backend/web/shared.js`、`renderMeasurementTechnique`、详情页页脚与推荐页常驻页脚均已删除，后端 `measurementTechnique` 字段前端一律不消费。下面第 1 条实现要求与其验证判据不要再执行；第 2 条（`hasAugments ? "" : <footer>` 的注释与条件）仍然有效。

### 现状证据
`gameplay.go:5378` 已经返回 `Citation`/`MeasurementTechnique`，但全文件 `grep -n "measurementTechnique" backend/web/gameplay.js` **零命中**——纯遗漏，是本方案里最便宜的一项。
另：`gameplay.js:5684`（不是原方案写的 5689）的 `hasAugments ? "" : <footer>...</footer>` 已核实为**有意设计**（git 历史从 `ff3cf27c` 到 `7eb6393c` 条件从未变动，按钮本来就只在 ChampSelect 可用）。

### 实现要求
1. 在 `renderRecommendationArea`（`gameplay.js:5069-5122`）拼接序列末尾加一个常驻页脚，复用 `champions.js:1643-1648` 的 `renderMeasurementTechnique` 渲染逻辑（抽成共享函数放到 `web/shared.js` 或类似的公共模块，两处调用，不要复制粘贴一份新实现）。
2. `gameplay.js:5684` 附近补一行注释说明该分支是有意设计（「海斗/斗魂的海克斯局不支持写入客户端装备方案，写入接口只认经典出装」），关闭方案里的遗留待确认项，**不改动条件本身**。

### 验证判据
- 推荐页任意一次渲染，DOM 里出现「统计口径」文案，内容与后端 `measurementTechnique` 字段一致。
- `gameplay.js:5684` 行为不变（回归测试：海斗局 `hasAugments=true` 时按钮仍为空，经典局仍显示按钮）。

---

## P0-4：召唤师技能组合带胜率

### 现状证据
`renderMayhemOpeningConfiguration`（`champions.js:1031-1038`）里 `group()` 对 starters/boots/spells 统一调用 `renderConfigOption(row, kind, false)`（`:1036`），`false` 走 `renderOptionPickStats`（`:1964`起），**只渲染选用率**。`renderConfigOption` 本身已经是三态开关（`showWinRate: true/false/null`，`:1900`），不需要改这个函数。
根因不是渲染逻辑，是**数据源**：`build.summonerSpells` 现在来自 OP.GG RSC（`response.Build = rsc.Build`，`hexdata.go:1972`），OP.GG 这份数据只有 `pickRate`。R116-A 新接入的 `summonerSpellPairs`（Hexdata `hero-json`）**同时带 `winRate` 与 `pickRate`**（实测样例：`spellIds:["4","32"], winRate:0.569432, pickRate:0.907758, deltaWinRate:0.001923`）。

### 实现要求
1. `loadMayhemDetail` 里把 `response.Build.SummonerSpells` 的数据源从 `rsc.Build.SummonerSpells`（OP.GG）**切换为** `primary.SummonerSpellPairs`（Hexdata `hero-json`），按 `spellIds[]` 映射到现有 `championAsset{ID, Kind:"spell"}` 结构（复用前端 `summonerSpellAsset()` 已有的 ID→图标解析，不需要新写映射表）。
2. 只改 `group("召唤师技能", spells, "spell")` 这一处调用为 `renderConfigOption(row, "spell", true)`；出门装/鞋子（OP.GG 数据，仍无胜率）**保持 `false` 不变**——不要因为改了一处就把另外两处也强行加胜率显示不存在的数据。

### 验证判据
- 召唤师技能区块任意一行同时显示胜率与选用率；出门装/鞋子区块行为不变（只显示选用率）。
- 若 Hexdata 的 `summonerSpellPairs` 为空（上游波动），回退到 OP.GG 的 pickRate-only 展示，不整块报错。

---

## P0-5：官方档位标签替代本地自算（范围比原方案更大）

### 现状证据
本地自算分两处，**都要处理，原方案只提到其中一处**：
1. 海克斯/装备档位：`applyLocalAugmentGrades`（`champions.go:271-301`）按分位自算 S/A/B，与 Hexdata 官方的 `hexTier`/`hexLabel`/`hexTierColor`（"夯"/"顶级"等）口径不一致。
2. **梯度徽章 + 总榜第 N 位**（英雄榜单，不是单个英雄详情）：`hexdata.go:1280` `Tier: index*5/len(rows)+1`，按返回顺序本地算的名次分档，**自带 `TierLocallyCalculated: true` 标记**（`champions.go:193`，字段已存在但前端未引用）。R116-A 接入的 `hextech-insights.heroes[].tier` 是官方口径的英雄级 T0-T5 分档，可以替换这个本地计算。

### 实现要求
1. `championMetricRow` 新增 `HexTier`/`HexLabel`/`HexTierColor` 字段，`applyLocalAugmentGrades` 改为：**若官方字段存在则直接用，只有官方字段缺失时才回退到本地分位算法**（保留本地算法作为兜底，不删除）。
2. 英雄榜单（`championRankingRow.Tier`/`Rank`）改为优先取 `hextech-insights.heroes[].tier`；`TierLocallyCalculated` 字段保持准确（官方数据可用时设为 `false`，回退时设为 `true`），前端在 `TierLocallyCalculated == true` 时展示一个小字注释「本地估算」，官方数据时不显示（这个字段已经存在，只是接上真实语义）。

### 验证判据
- 有官方 `hexTier` 的行不再触发本地分位计算（用测试断言 `localAugmentScore` 不被调用，或断言输出的 Grade 与官方 HexLabel 一致的映射关系）。
- `hextech-insights` 缺失时（熔断/降级），榜单退回本地计算且 `tierLocallyCalculated: true` 被前端正确渲染成「本地估算」小字。

---

## P0-6：海克斯阶段（stage 1-4）手动筛选

### 现状证据
`augments[].stages[1..4]` 已在 R116-A 的 `hero-json` 解析里带出（每个 stage 有独立 `winRate`/`deltaWinRate`/`pickRate`/`stageBaselineWinRate`）。`docs/r116-proposal-feasibility-review.md` 第 3.1 节已判定：**局中自动定位到当前阶段不可行**（无通道），本工单只做**手动筛选**。

### 实现要求
1. 在海克斯推荐区加阶段筛选 chips（1/2/3/4，可复用 `renderArenaSortBar` 的交互形态，`champions.js:1152-1186`），点击后前端本地对已加载的 `stages[]` 重新渲染对应阶段的 `winRate`/`deltaWinRate`/`pickRate`，**不重新发请求**（数据已经全部在 `hero-json` 响应里）。
2. 默认视图（未选阶段）展示英雄级汇总（`augments[].winRate`，不分阶段）。

### 验证判据
- 切换阶段 chip 后，卡片数字变化且与 fixture 里对应 stage 的数值一致；切回「汇总」视图恢复原数字。
- 不产生新的网络请求（用 mock 断言点击 chip 后 fetch 调用次数不变）。

---

## P1-6：22 项表现指标面板（落在「表现」tab）

### 现状证据
R116-A 接入的 `/api/hexdata/postmatch` 一次性给全 173 英雄的 22 项指标（战斗 KDA / 伤害输出 / 生存辅助 / 经济节奏四组）。

### 实现要求
1. 新函数 `renderMayhemPerformancePanel(postmatchRow)`，四组卡片形态参考 `.arena-overview-secondary`（`champions.js:1152-1186`）。
2. **每项必须带「较全英雄平均 ±%」**（方案原文要求，否则绝对值没有意义）：后端在响应里额外算好 173 英雄该项的均值，前端只做展示，不在前端算均值（避免 173 英雄的完整表加载到前端）。
3. 落在下面「三-tab 重整」的「表现」tab 内，不作为详情页顶层新 section。

### 验证判据
- 面板显示 22 项指标，每项都有「较平均」修饰；缺任意一组时该组整体隐藏（不显示 0 或 N/A）。

---

## 页面结构：英雄详情页三-tab 重整

### 现状证据
`renderMayhemDetailPane`（`champions.js:998-1019`）目前是 6 段线性拼接（推荐海克斯 → 开局配置 → 装备路线 → 装备排行 → 统计口径 → 海克斯图鉴）。加完 P0 六项 + P1-6 会变成 9-10 段。

### 实现要求
1. 改成三个 tab，复用项目里已有的 tab 组件模式（如 `recommendationTabSpecs` 的实现方式，`gameplay.js:5036-5042`，保持前后端两处 tab 交互一致）：

   | tab | 内容 |
   |---|---|
   | 概览 | 梯度/胜率/样本 + 推荐海克斯（P0-1/2/5/6）+ 开局配置（P0-4） |
   | 构筑 | 装备排行（P0-1/2）+ 成装三件套（R116-A 新增的 `terminalItemTrios`，若本工单未消费则留作后续）+ 装备路线 + 技能加点 |
   | 表现 | P1-6 的 22 项指标面板 |

2. ~~**统计口径说明（`renderMeasurementTechnique`）不进任何 tab，改成页脚常驻**（跨 tab 切换始终可见）——它的整个意义就是让置信度信息随时可见，藏进 tab 等于 P0-2/P0-3 白做。~~ **已被 2026-09-22 用户指示撤销（R128 §2.3）：口径说明整块从 UI 删除，页脚不复存在；下面「页脚统计口径跨 tab 常驻」的验证判据同步作废。**
3. **海克斯图鉴 + 稀有度分布（`renderMayhemAtlas`/`renderMayhemRarityPanel`，`champions.js:1045-1131`）从详情面板里搬出**，理由：这是全局维度（跨英雄的 `/augments`、`/augment-rarity` 数据），混进"这个英雄"的 tab 语境会造成认知混乱。搬到详情面板顶部的一个独立入口按钮（点击后原地切换成全局图鉴视图，与三个 tab 平级但视觉上区分为"离开当前英雄"），**不新建独立页面/路由**。

### 验证判据
- jsdom 冒烟测试：切换三个 tab，每个 tab 只渲染对应内容，不重复渲染其他 tab 的 DOM 节点（避免隐藏 tab 仍占用布局高度，重演 R33「收藏页 UI」类的隐藏元素占位问题）。
- 页脚统计口径在三个 tab 之间切换时保持渲染，不随 tab 切换重新挂载/闪烁。
- 桌面视口下详情面板首屏高度不超过一屏（用 Playwright 截图核对，参考 memory 里"沙箱里跑真 Chromium 截图"的既有方法）。

---

## 明确不做（Anti-scope）

1. 不在本工单实现 P1-1~P1-5（局内实时化，留给 R116-D，且 P1-1 取决于 R116-探测的结论）。
2. 不新建独立页面/路由，海克斯图鉴维持在详情面板内的一个入口。
3. 不改动 `gameplay.js:5684` 的按钮置空逻辑本身，只补注释。

## 通用验证要求

- 前端 jsdom 冒烟测试全绿（沿用 memory 里「jsdom 渲染冒烟测试」方法论）
- 真 Chromium 截图核对 tab 切换与页脚常驻效果
- `go test ./backend/...` 全绿（后端新增字段的序列化测试）

## 交付物

- `champions.js`/`gameplay.js` 的 tab 重整与六项 P0 渲染
- `champions.go` 的 `championMetricRow`/`championRankingRow` 字段扩展
- `docs/r116b-execution-ledger.md`

## 真机验证清单

- [ ] 打开至少 5 个不同英雄的详情页，三个 tab 都能正常切换且内容不串
- [ ] 阶段筛选 chip 点击后数字正确变化且不产生新请求
- [ ] 召唤师技能区块显示胜率
- [ ] 推荐页页脚显示统计口径说明

## 工单结束标志

- [ ] P0-1~P0-6、P1-6、tab 重整全部验证判据 PASS
- [ ] 独立核对：抽查 3 个真实英雄的响应快照，确认新字段（deltaWinRate/sampleTier/hexTier）在真实数据里非空比例与评审文件实测一致
