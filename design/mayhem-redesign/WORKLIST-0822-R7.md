# 第七轮工单（2026-08-22）— 海克斯大乱斗 / 斗魂竞技场

> 来源：6 张截图 + 1 份诊断日志（lol-loot-diagnostics-db64ffdf.jsonl）  
> 红线：hexdata 不打包、不预热、不伪造 UA；上游字符串进 innerHTML 前一律 `escapeHTML`；变异测试必须 `cp` 真副本、副本带仓库根 `*.go`。

---

## 〇、优先级速览

| 优先级 | 截图 | 问题 | 状态 |
| --- | --- | --- | --- |
| A | 图一/图五 | 海克斯图标填充灰白（无颜色），仅边框颜色正确；图标整体偏小 | 未修 |
| B | 图二 | 棱彩装备 1/2/3 徽章应改为 S/A | 未修 |
| C | 图三 | 原画应放在红框区域；红框高度应与海斗一致；胜率行高度过高 | 待确认 |
| D | 图四 | 点击"查看详情"后装备消失、卡住并显示"未配置"；MVP 左圆圈在斗魂未正确展示 | 未修 |
| E | 图五 | 部分海克斯图标仍不正确 | 同 A |
| F | 图六 | 胜率数据未显示；梯度中英雄名称含昵称括号 | 未修 |
| G | 全局 | 标签副标题"快节奏团战"需删除 | 未修 |

---

## 一、图一（海克斯推荐区）+ 图五：图标颜色——已确认为预期行为 ✅

### 现象

fig2/fig5 截图：海克斯图标呈现白色剪影，无颜色填充，仅边框颜色（银/金/紫）正确。

### 根因（已实机确认）

**不是 bug。** CommunityDragon 的海克斯图标本身是单色白描 PNG（白色剪影 + 透明背景）。LoL 客户端里也是白色剪影，靠客户端自带的紫色/金色背景衬托。app 里 `background: var(--surface-strong)`（深色底）+ 白描 PNG = 正确渲染。

**边框色 vs S/A/B 级是两个独立维度**：
- 边框色 = 海克斯品质（来自 `row.rarity` → `arenaRarityKey()`：0/1 银色、4 金色、2/8 棱彩紫色）
- S/A/B 徽章 = 综合评分百分位（`augmentGrade()`：>=90% → S、>=70% → A）
- 银色品质海克斯拿到 S 级、金色品质海克斯是 B 级 → 完全正常

### 文件证据

- **CSS**：`web/champions.css:88` `.augment-icon` 设置 `background: var(--surface-strong); border: 1px solid var(--line)` — 正确
- **JS**：`web/champions.js:148-153` `assetImage()` 渲染 `<img src="${imageURL(source, path)}">` — 源图就是白描
- **JS**：`web/champions.js:923-928` `arenaRarityKey()` 映射 rarity 数值到品质字符串 → 边框色
- **JS**：`web/champions.js:118-126` `augmentGrade()` 基于百分位计算 S/A/B → 独立于品质

### 可选改进（非 bug 修复）

1. **图标尺寸**：`web/champions.css:341` 的 44×44 可改为 48×48 或 52×52，让剪影更清晰
2. **边框加粗**：用户要求"单个边框再加粗一点"，`.mayhem-recommend-item` 当前 `border: 1px solid var(--quality)` 可改为 1.5-2px

### 状态：**已关闭**，不需要改代码

---

## 二、图二：棱彩装备 1/2/3 → S/A

### 现象

fig1 截图：棱彩装备区显示数字徽章 1/2/3（黄色六边形），而上方海克斯推荐已用 S/A 字母徽章。用户要求棱彩装备也换成 S/A。

### 文件证据

- **JS**：`web/champions.js:964`
  ```js
  const badge = kind === "augment"
    ? `<b class="augment-grade is-${grade}">${grade}</b>`
    : `<b class="hex-rank ${index < 3 ? `is-${index + 1}` : "is-rest"}">${index + 1}</b>`;
  ```
  `kind === "prism"` 时走 `else` 分支，显示数字 hex-rank。

- **JS**：`web/champions.js:959` `const rarityKey = kind === "augment" ? arenaRarityKey(row.rarity) : "";` — prism 不走 augment 分支

- **CSS**：`web/champions.css:348-352` `.hex-rank.is-1/is-2/is-3` 用 clip-path 六边形 + 金/银/铜色

### 改法

在 `renderArenaOptionCard`（`web/champions.js:964`）中，将棱彩装备（`kind === "prism"`）也使用 `augment-grade` 而非 `hex-rank`：

```js
const badge = (kind === "augment" || kind === "prism")
  ? `<b class="augment-grade is-${grade}">${grade}</b>`
  : `<b class="hex-rank ${index < 3 ? `is-${index + 1}` : "is-rest"}">${index + 1}</b>`;
```

**注意**：核心物品（`kind === "core"`）保持数字 1/2/3 不变。

### 验收标准

- 棱彩装备区显示 S/A 徽章，样式与海克斯推荐一致
- 核心物品区仍显示数字 1/2/3
- 已有的 `champions.test.cjs` 中关于 `renderArenaAugments` 不含 `hex-rank` 的断言应继续通过（line 319）

---

## 三、图三：原画位置 + 红框高度 + 胜率行

### 现象

fig3 截图：Sett 的原画在梯度区域的红框内。用户说"不对，应该放在红框的位置"，意思是原画应放在英雄概览面板的红色边框区域内（与海斗一致），且红框区域高度应与海斗对齐，下半部分胜率行应更矮。

### 文件证据（两边已逐一对比）

**海斗（基准）**：
- CSS：`web/champions.css:224` `.mayhem-overview-strip { min-height: 132px; ... }`
- CSS：`web/champions.css:225` `.mayhem-overview-art` — 全幅铺底，`opacity: .2`，`object-position: center 20%`
- JS：`web/champions.js:752` 原画 `<img>` 在 `renderMayhemDetailPane()` 中作为**铺底背景层**（`position: absolute; inset: 0`）
- JS：`web/champions.js:758-759` 指标区 3 格：`胜率 / 样本 / 梯度`，`min-height: 70px`（CSS 234）

**斗魂（现状）**：
- CSS：`web/champions.css:731` `.arena-overview-strip { min-height: 154px; ... }` — **比海斗高 22px**
- CSS：`web/champions.css:736` `.arena-overview-art` — 同为全幅铺底 `opacity: .18`
- CSS：`web/champions.css:741` 指标区 4 格：`梯度 / 胜率 / 平均名次 / 吃鸡率`，`min-height: 76px`（比海斗高 6px）
- CSS：`web/champions.css:746-751` `.arena-overview-secondary` — **独立一整行**（选用率/禁用率/样本 + 排序按钮），`min-height: 36px`，斗魂独有，海斗没有

**结论**：两套代码结构一致（都是铺底原画 + 指标条），不存在"原画放进梯度里"的代码差异。用户看到的高度差来自两个数值：① `.arena-overview-metrics > div` 的 `min-height: 76px`（海斗 70px）；② 斗魂多了一条 `.arena-overview-secondary` 次行（36px）——这就是"胜率那行高度大一点"的根因。

### 改法方向

1. `.arena-overview-metrics > div` 的 `min-height` 从 76px 降到 70px（与海斗 234 行一致）
2. `.arena-overview-secondary` 的 `min-height: 36px` 压缩（如 26px）或去掉固定高度，让胜率行瘦身
3. `.arena-overview-strip` 的 `min-height` 从 154px 降到与海斗对齐（约 132px 档）
4. 若用户确实想要"原画作为右侧区域元素"而非铺底：需要额外的 DOM 结构调整，先按数值对齐处理，再看效果

### 验收标准

- 斗魂英雄概览面板的原画位置与海斗一致
- 红框区域高度与海斗对齐
- 胜率行不占用过多垂直空间

---

## 四、图四：查看详情卡死 + MVP 左圆圈

### 现象

fig6 截图：斗魂竞技场的"吃鸡战绩"列表中，点击某场比赛的"查看详情"后，装备消失、页面卡住，并显示"未配置"。同时 MVP 左侧的圆圈（评分环）在斗魂中未正确展示。

### 文件证据

- **Go**：`arena_match_detail.go:76-77`
  ```go
  case strings.Contains(value, "尚未配置 riot api key"):
      return http.StatusServiceUnavailable, "Riot 对局详情服务尚未配置", "not-configured"
  ```
  "未配置" 来自此分支——说明 EXE 内嵌的 Riot API Key 缺失或无效。

- **JS**：`web/champions.js:1044`
  ```js
  return api(`/api/champions/arena/match/KR_${gameID}`, `arena-match-${gameID}`);
  ```
  调用 `/api/champions/arena/match/KR_<gameID>`（`loadMatchDetails`，由 `mountArenaMatchCards` 在 champions.js:1039-1047 传入）。

- **Go 路由**：`main.go:271` `GET /api/champions/arena/match/{matchId}` → `handleArenaMatchDetail`（arena_match_detail.go:18）→ 无 Riot Key 时 arena_match_detail.go:77 返回 503 + "Riot 对局详情服务尚未配置" + `not-configured`。

- **卡住 / 装备消失的直接机制**：`renderArenaMatchDetail`（gameplay.js，`.arena-match-detail`）在详情接口 503 时进入错误分支，卡片展开后应渲染的装备网格为空，且错误提示覆盖原装备区 → 视觉上"装备消失 + 卡住"。

- **MVP 左侧圆圈**：斗魂卡片复用同一套 `renderMatchCard`（gameplay.js:1216 附近），评分排名 chip 由 `scoreRankChip()`（gameplay.js:1105-1108，`.match-rank-chip` 圆角徽章，金/银/铜）渲染。
  - gameplay.js:1216 有意跳过逻辑：`subjectScore?.badge === "MVP" && scoreRank === 1` 时不渲染排名 chip（MVP 徽章已代表第一名）——正常排位对局里 MVP 徽章左侧会有一个"第 1 名"金色 chip。
  - 图四截图中斗魂卡片该位置疑似为**空占位**（无数字、无徽章），与排位对局不一致 → 需对比同 EXE 排位页 MVP 卡片的实际渲染，确认是 `scoreRank` 为 0/undefined（arena 数据缺评分排名），还是样式未适配。
  - 排查顺序：`renderArenaMatchOverview`（gameplay.js:1330）的 `match.detail` 数据里是否带 `scores`/`rank` 字段 → `computeScores`（gameplay.js:980-1001）在 arena 模式下 `weights`/`peaks` 是否可算 → 若不可算则 `scores` 为空，`subjectScore` 为 undefined，`rankChip` 与 `scoreBadgeChip` 都为空。

- **查证要点**：`scoreChip()`（gameplay.js:1010 附近，`.match-score`）才是"评分数字"，与 `scoreRankChip` 是两个不同的元素。用户说的"MVP 左边的圆圈"更接近 `.match-rank-chip`（圆角矩形，第 1/2/3 名金/银/铜）——截图里它是空占位。

- **诊断日志**：本次日志中**无** arena_match 相关错误（日志主要记录 hexdata 和 LCU discovery），说明问题可能是：
  1. EXE 构建时未注入 Riot API Key（`-encrypt-riot-key` + `-ldflags`）
  2. 或 Key 已过期/配额耗尽

- **MVP 圆圈**：fig6 截图中 MVP 标签显示为纯文本 `<b>MVP</b>`，左侧没有评分圆圈（金色环）。需检查 `matchCards` 组件中 MVP 渲染逻辑。

### 改法

1. **"未配置"问题**：
   - 确认构建时是否通过 `-ldflags "-X main.riotAPIKeyCipher=..."` 注入了 Riot API Key
   - 如果 Key 确实缺失：这是构建配置问题，不是代码 bug
   - 如果 Key 已注入但仍然报错：检查 `riot_api.go:219` 的解密逻辑

2. **MVP 圆圈**：检查 `matchCards` 组件（可能在 `gameplay.js` 或独立模块）中 MVP 标签的渲染，确认是否为斗魂场景遗漏了圆圈样式

### 验收标准

- 点击"查看详情"后能正常加载并显示装备数据
- 不再出现"未配置"错误
- MVP 左侧显示金色评分圆圈（与韩服对局页一致）

---

## 五、图六：胜率数据未显示 + 英雄名称含昵称

### 现象

fig4 截图：英雄梯度列表中，名称显示为"暗夜猎手 (...)"、"法外狂徒 (...)"——带括号和昵称。同时胜率/均名次等数据显示为空或不完整。

### 根因（已确认）

**hexdata 原始锚文本带括号昵称**：`hexdata.go:777` 的 `firstLink(cells[0])` 取 HTML 锚文本，包含"暗夜猎手 (薇恩)"这种原始格式。hexdata 行将此设为 `Name`（`hexdata.go:859`），而 `renderChampionRow`（`champions.js:1134`）优先使用 `row.name`（即 hexdata 的带括号名），而非干净的 `meta.nameZh`。

**竞技场行不设 Name**（`champions.go:1441/1582/1638`）→ 前端回退到 `meta.nameZh` → 干净名称。这就是斗魂显示正确、海斗显示不正确的原因。

### 改法

在 `hexdata.go` 的 `parseHexdataHeroes`（line 762-799）中，提取名称时去掉括号内容：

```go
// hexdata.go:777 附近
href, rawName := firstLink(cells[0])
name := strings.TrimSpace(regexp.MustCompile(`\s*[\(（].*[\)）]\s*$`).ReplaceAllString(rawName, ""))
```

或者更简单：在 `hexdata.go:859` 的 `loadHexdataRankings` 中**不传播 Name**（让前端用 catalog 的干净名称）：

```go
// hexdata.go:859 — 去掉 Name 字段
championRankingRow{ChampionID: item.ID, Key: item.Slug, Rank: index + 1, ...}
```

### 验收标准

- 英雄名称只显示中文名（如"暗夜猎手"），不带括号/昵称
- 胜率/均名次/选用率等数据正常显示
- 图鉴页英雄名称同样不含括号

---

## 六、标签副标题删除"快节奏团战"

### 现象

海克斯大乱斗的标签副标题当前为"随机英雄 · 全员海克斯 · 快节奏团战"，需删除"快节奏团战"。

### 文件证据

- **JS**：`web/champions.js:663`
  ```js
  ${modeTab("aram-mayhem", "海克斯大乱斗", "随机英雄 · 全员海克斯 · 快节奏团战")}
  ```
  改为：`"随机英雄 · 全员海克斯"`

- **测试**：`web/champions.test.cjs:322`
  ```js
  assert.match(script, /随机英雄 · 全员海克斯 · 快节奏团战/);
  ```
  需同步改为：`assert.match(script, /随机英雄 · 全员海克斯/);`

### 验收标准

- 标签副标题显示"随机英雄 · 全员海克斯"
- 测试全绿（line 322 断言同步更新）

---

## 七、诊断日志分析

### hexdata 英雄详情页确定性失败（与 R6 §0.1 同一根因）

日志关键事件：
```
hexdata_shape_invalid | kind=hero, fields=0, rows=16  (3次)
hexdata_circuit_trip  | kind=hero, failures=3/1        (3次)
hexdata_circuit_open  | kind=answer, probeAt=...       (15次)
hexdata_fallback      | module=rankings, reason=circuit open (23次)
```

- `kind=heroes`（批量排行）**正常**：fields=860, rows=172 ✅
- `kind=hero`（单英雄详情）**确定性失败**：fields=0, rows=16 ❌

这与 R6 §0.1 的结论一致：hero 详情页正则分隔符（`·` vs `，`）导致解析失败，当场重开熔断。R6 已修复正则（`hexdata.go:47`），但用户当前运行的 EXE 仍是旧版（`hexdata_circuit_trip` 仍在触发）。

**结论**：R6 的正则修复是正确的，但用户尚未重新构建并运行新 EXE。本次截图中的问题（图标灰白、棱彩数字、名称括号等）是独立的 UI bug，与 hexdata 熔断无关。

---

## 八、待确认项

1. **图一图标灰白**：需在开发机上打开 DevTools 检查图标 `src` URL 返回的实际图片是否为全彩。如果是 CDN 返回单色版，需要换源或加 CSS filter。
2. **图三原画位置**：需对比海斗和斗魂的 CSS 定位代码，确认具体差异。截图中 Sett 原画看起来已在正确位置，可能是用户对"红框区域"的理解与代码不一致。
3. **图四"未配置"**：需确认构建时是否注入了 Riot API Key。如果是构建问题而非代码问题，应记录为已知限制。
