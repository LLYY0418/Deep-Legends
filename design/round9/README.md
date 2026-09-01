# 第九轮工单：战绩卡 / 海克斯 / 符文块 / 交互 / 模式化推荐

> 定稿 2026-08-21。本文只做定位与方案，代码改动由 GPT 执行。
> 每条结论都带 `文件:行号` 证据；凡未经证据支持的推测都显式标注「待核实」。

---

## 0. 前置说明（先读，能省掉一轮返工）

**0.1 日志时效性。** 本轮三份 `diagnostics.jsonl` 里只有 `sgp_ranked_stats_incomplete`，**一条 `sgp_ranked_stats_shape` 都没有**，且最后一条记录停在 `2026-08-20T05:30Z` —— 比第九轮截图早约 10 小时。也就是说这些日志出自**没有包含第八轮 A/B/C 改动的旧构建**，新埋点尚未产出任何数据。G 组的结论不依赖这批日志，依据的是更早那份 `ranked_winrate_resolved` 分布统计。**要验证 A/B/C 是否生效，必须重新构建后再跑一次真机。**

**0.2 沙箱限制。** 本机（分析环境）**没有 Go 工具链**，`go vet / go test / go build` 一律跑不了。本轮改动横跨 Go 与前端，**全部 Go 侧验证必须由你在开发机执行**。前端 `node --test` 可以跑。

**0.3 本轮 OP.GG 事实都是实测的。** F 组里所有接口形状、字段、版本号都是 2026-08-21 在沙箱内用 `curl` 直接打 `lol-api-champion.op.gg` 与 `op.gg` 拿到的真实响应，不是从代码里推断的。若实现时发现与本文不符，以现场响应为准并回来更正本文。

**0.4 两条红线，改动时不得破坏。**
- **XSS：** 提示框内容含远端玩家名与 OP.GG 文案。现有实现安全的唯一原因是 `web/app.js:2490-2500` 对每个子节点单独 `textContent`。任何标题/正文拆分都必须继续走 `textContent`，**绝对不许引入 `innerHTML`**。
- **隐私：** 埋点只允许记键名、布尔、计数与 `privacy` 枚举。`puuid` / 昵称 / `accountHash` / 段位与场次数值 / 载荷片段一律不得落盘（`lp_tracker.go:11`）。

---

## A 组 · 战绩卡与玩家列表（图四、图五、图六）

### A1 评分与徽章拆成两列 —— 评分单独一列，MVP/SVP 放右侧

现状：`web/gameplay.js:1371` 的 `<span class="match-score-cell">${scoreBadgeChip(record)}${scoreChip(record)}</span>` 把徽章和评分塞进同一个格子，且**徽章在前、评分在后**，导致有徽章的行评分被顶偏、没徽章的行评分靠左，整列对不齐。第 8 轮备忘里记的 `:1333` / `:1371` 「badge-before-score」就是这里。

改法：`match-score-cell` 拆成固定两栏 —— 左栏只放 `scoreChip`（定宽、右对齐、`font-variant-numeric: tabular-nums`），右栏只放 `scoreBadgeChip`（定宽，无徽章时留空占位而不是塌缩）。**所有模式共用同一个渲染函数**，普通模式、极地、海克斯大乱斗、斗魂都要走这条路径，不允许各自拼装。

### A2 评分低于 5 分标红

现状：`web/gameplay.js:1014`

```js
const tone = record.score >= 8 ? " is-gold" : record.score >= 6.5 ? " is-good" : record.score < 4 ? " is-poor" : "";
```

阈值是 `< 4`，用户要的是 `< 5`。同一处还有第二个副本 `web/gameplay.js:2090`（`value >= 4 ? "is-gold" : value >= 3 ? "is-good" : value < 1.5 ? "is-poor"`），那是另一套量纲（0–5 分制）不要一起改，但**必须确认它渲染的是哪块 UI**，避免用户在别处又看到不标红的低分。

改法：`< 5` 改为 `is-poor`。注意 `>= 6.5` 与 `< 5` 之间留出的 5–6.5 区间是「无 tone」，配色上不要留成和 `is-poor` 相近的色。

### A3 斗魂玩家列表：补排名、补名称、别只显示 4 队

现状 `web/gameplay.js:1093-1096`：

```js
const rows = grouping.groups.slice(0, 4).map((group) => {
  return `<div class="arena-team-row-compact${group.placement === 1 ? " is-first" : ""}" data-tooltip="${escapeHTML(arenaGroupLabel(group))}" ...><span class="arena-team-roster">${group.players.slice(0, 3).map(...)}</span></div>`;
});
```

三个问题叠在一起：

1. **排名只存在于 tooltip 里**（`arenaGroupLabel` → `第 N 名 · 魄罗`），行内没有任何名次数字。用户要的是「每行前面加上排名」。
2. **`slice(0, 4)` 砍掉了后 3 队**。斗魂 21 人 7 队，卡片上只看得到前四名，第 5–7 名整队消失。
3. 名称其实**已经**渲染了（`playerButton` 里的 `.match-player-name`），但 CSS 在窄容器下把它压没了 —— 这属于 A4。

改法：行首加一个名次芯片，复用 `scoreRankChip` 已有的 `is-rank-1/2/3` 金银铜配色思路（`web/gameplay.js:1104-1107`），但**文案改中文**（见 A6）。7 队全渲染，靠容器查询控制窄容器下折行而不是截断队伍。名称加 `text-overflow: ellipsis`，超长省略号 —— 注意 `.match-player-name` 上已经挂了 `data-tooltip-overflow=".match-player-name"`（`:1091`），截断后 tooltip 会**自动**只在真截断时才出现，这套机制本来就对，不要拆。

### A4 玩家列表宽度跨模式统一

现状：卡片右侧名单宽度由 `.matches-column` 的容器查询驱动（`container-type: inline-size`），普通模式走 `.match-players`、斗魂走 `.match-players.is-arena`（`:1097` vs `:1100`），两条分支的宽度是各自算的，窄窗口下就分叉了。

改法：把名单宽度提成一个 CSS 变量（例如 `--match-roster-width`），在 `.matches-column` 层按容器宽度**统一**定义一次，`.match-players` 与 `.match-players.is-arena` 都只消费这个变量，不再各自写死。容器查询断点也要合并成同一组。

### A5 极地 / 海克斯大乱斗的数据列去补刀、伤害承伤分两行

现状 `web/gameplay.js:1183-1184`：CS 行与「伤害 · 承伤」行是并排的三段式，海克斯大乱斗下被挤压。

```js
const csRow = `<span class="match-stat" ...><em>CS</em>...`;
const damageRow = `<span class="match-stat match-stat-damage" ...><em>伤害 · 承伤</em><b><span class="match-damage-value">…</span><span class="match-stat-separator"> · </span><span class="match-taken-value">…</span></b></span>`;
```

改法：按 `match.modeGroup` 分流 —— `aram` 与 `hextech-aram` 两组**不渲染 `csRow`**（这两个模式补刀本来就没有比较意义），并把 `damageRow` 拆成「伤害」「承伤」两个独立 `match-stat`，纵向排列。`match-stat-separator` 随之移除。斗魂（`arena`）已经有自己的 `arena-detail-damage`（`:1371`），一并按同样规则处理。

**注意**：`modeGroup` 目前由 `gameplay.go:3919-3935 queueModeGroup` 按 queueId 白名单产出，海克斯大乱斗海选赛之类的新队列会掉进 `"other"`。见 F2，那里给了不依赖白名单的判据。

### A6 `ordinalLabel` 输出英文

`web/gameplay.js:1027-1032` 返回 `1st / 2nd / 3rd / 4th`。项目其他地方全中文，这里应改成「第 1 名」或纯数字 + 名次徽章样式。A3 的名次芯片直接复用改造后的结果。

---

## B 组 · 海克斯图标、品质色与说明（图五、图六）

这一组是本轮**根因最明确**的一块，四个问题互相独立。

### B1 【根因确认】海克斯大乱斗的海克斯图标整体消失

`web/gameplay.js:2053-2063`：

```js
function renderLiveAugmentIcon(row, source) {
  if (source !== "arena") return augmentIconFigure(row.id, "large");   // ← 海克斯大乱斗分支
  const asset = row.assets?.[0] || {};
  ...
  const icon = asset.source && asset.path
    ? remoteStaticIcon(asset.source, asset.path, name, "large", false)  // ← 斗魂分支：优先用 OP.GG 图
    : augmentIconFigure(asset.id, "large");
  ...
}
```

两条分支**不对称**：斗魂优先用 OP.GG 自带的图片资源，海克斯大乱斗则直接把 `row.id` 丢给 `augmentIconFigure`。而 `row.id` 的来源是 `champions.go:1710-1714`：

```go
for _, item := range raw {
    asset, ok := remoteAsset(item.LargeIcon, item.Name)
    if !ok || item.ID == 0 || item.Name == "" { continue }
```

`item.ID` 是 **OP.GG 自己的海克斯 id**，与 Riot `cherry-augments.json` 的 id 空间**不是同一套**。于是 `augmentIconFigure`（`web/gameplay.js:2606`）在 `state.perks.augments` 里查不到条目，落到 `iconFigure("augment", id, …)` 兜底，请求一个不存在的资源 → 全部空白。

更讽刺的是：后端**已经把可用的 OP.GG 图存下来了** —— `championAugment.ImageSource` / `ImagePath`（`champions.go:314-315`，由 `:1711 remoteAsset(item.LargeIcon, …)` 填充），前端却一个字都没用。

**改法（一行级）：** 海克斯大乱斗分支与斗魂分支对齐 ——

```js
if (source !== "arena") {
  return row.imageSource && row.imagePath
    ? wrap(remoteStaticIcon(row.imageSource, row.imagePath, row.name, "large", false))
    : wrap(augmentIconFigure(row.id, "large"));
}
```

（`wrap` 指 `:2062` 那个带 `data-tooltip` / `aria-label` 的 `<span class="arena-augment-icon">`，两条分支要共用，别复制两份。）

**回归护栏：** 写一个 `web` 侧测试，断言 `renderLiveAugmentIcon` 在 `source !== "arena"` 且 `row.imagePath` 存在时，输出里包含该 `imagePath`。这条断言能同时杀掉「又改回只用 id」和「两条分支再次分叉」两种变异。

### B2 【根因确认】海克斯品质色全都一样

`web/gameplay.js:2612`：

```js
return `<span class="arena-augment-icon" tabindex="0" data-tooltip="…" aria-label="…">${icon}</span>`;
```

`arena-augment-icon` 上**没有任何品质修饰类**。战绩卡里所有海克斯（`:1207`、`:1371`、`:1499` 三处都调 `augmentIconFigure`）因此长得一模一样。对局页的 `.live-augment-option is-${row.rarity}`（`:2081`）倒是有品质类，但那是另一套元素、另一套取值（OP.GG 的小写 `silver/gold/prismatic`），两套 UI 品质表现不一致。

**品质枚举实测（`cherry-augments.json`，655 条，2026-08-21）：**

| rarity | 条数 | 现有映射 |
|---|---|---|
| `kGold` | 229 | ✅ 黄金 |
| `kSilver` | 194 | ✅ 白银 |
| `kPrismatic` | 193 | ✅ 棱彩 |
| `kEventChoice` | 25 | ❌ 漏 |
| `kBronze` | 14 | ❌ 漏 |

`web/gameplay.js:2608` 只映射了三个，`kBronze` / `kEventChoice` 会原样吐出内部枚举名到用户界面。

**改法：**
1. `augmentIconFigure` 输出 `class="arena-augment-icon is-${normalizedRarity}"`，`normalizedRarity` 由 `kGold→gold` 这种规范化函数产出，**与 `.live-augment-option.is-*` 共用同一套类名与色板**，两处 UI 从此一致。
2. 补全 5 个枚举：`kBronze→青铜`、`kEventChoice→活动`。未知值统一落到 `is-unknown` 并显示「海克斯」，不要泄漏 `kXxx`。
3. 色板参考第七轮 v4 提示框那套做法：**近黑底 + 品质色描边 + 品质色标题**，正文保持暖白。绝对不要把品质色掺进底色（第七轮已经翻过一次车，出来是浑浊的橄榄色）。八套主题都要过一遍对比度。

### B3 【根因确认】斗魂每队顶部的队伍图标 404

`web/gameplay.js:1038-1062`：

```js
const ARENA_TEAM_MASCOTS = [ null, { name: "魄罗", file: "teamporos.png" }, … ];
…
? { ...mascot, iconPath: `/lol-game-data/assets/UX/Cherry/TeamIcons/${mascot.file}` }
```

**实测：这个目录在 Riot 资源树里根本不存在。** CommunityDragon 下 `assets/ux/cherry/` 只有一个子目录 `augments`。真实的斗魂队伍图标在**另一个插件**里：

```
plugins/rcp-fe-lol-postgame/global/default/subteams/
  → poro.svg  minion.svg  scuttle.svg  krug.svg  raptor.svg  wolf.svg  gromp.svg  sentinel.svg
```

注意三处差异：**插件不同**（`rcp-fe-lol-postgame` 而非 `rcp-be-lol-game-data`）、**文件名是单数**（`poro` 不是 `teamporos`）、**格式是 svg 不是 png**。

**⚠️ 代理白名单会挡住它。** `champions.go:691`：

```go
return communityDragonHost, strings.HasPrefix(requestPath, "/latest/game/") || strings.HasPrefix(requestPath, "/latest/plugins/rcp-be-lol-game-data/global/default/")
```

只放行 `rcp-be-lol-game-data`。所以**光改前端路径没用**，必须同时在这里放行 `/latest/plugins/rcp-fe-lol-postgame/global/default/subteams/`（建议**只放行这一个子目录**，不要整个插件开口）。同样地 `champions.go:756-757` 的正/反向路径映射也要相应补一条。

**兜底：** 若不想动代理白名单，第二方案是把这 8 个 svg 直接内置进 `web/`（体积很小，项目已有 `web/rune-styles/`、`web/tier-icons/` 等先例），完全离线。**我倾向这个方案** —— 队伍图标是固定 8 个、多年不变的中立生物，没有跟版本更新的必要，内置比开代理口子更稳且更快。

### B4 海克斯详细说明：数据源有缺口，必须分模式对待

`web/gameplay.js:2609-2610` 已经在拼 `description` 了：

```js
const description = plainText(augment.description || "");
const tooltip = [augment.name || `海克斯 ${id}`, rarity ? `${rarity}级` : "", description].filter(Boolean).join("\n");
```

`gameplay.go:3378-3379` 也已经解析了 `description` / `tooltip` 两个字段。**问题在数据源本身。**

实测 CommunityDragon 的 `cherry-augments.json`（`gameplay.go:3631` 用的就是它）字段只有：

```
['id', 'augmentNameId', 'nameTRA', 'simpleNameTRA', 'augmentSmallIconPath', 'rarity']
```

**没有 `description`，也没有 `tooltip`。** 所以走 CommunityDragon 兜底时说明必然为空 —— 这就是「现在只有名称」。

补救路径有三条，覆盖面各不相同（实测数字）：

| 来源 | 覆盖 | 说明 |
|---|---|---|
| LCU `/lol-game-data/assets/v1/cherry-augments.json`（`gameplay.go:3622`） | 待核实 | 客户端本地那份**可能**带 `desc`，代码已经能吃。开发机连上客户端打一次就知道。**优先验证这条**，成本最低 |
| CommunityDragon `/latest/cdragon/arena/zh_cn.json` | **225/485 非-ARAM 条目**，`ARAM_*` 条目 **0/170** | 字段齐全（`desc` / `tooltip` / `iconLarge` / `iconSmall` / `rarity`）。只覆盖**当前在用的斗魂海克斯**，退役的和全部海克斯大乱斗条目都没有 |
| OP.GG aram-mayhem 页 | 海克斯大乱斗全覆盖 | `championAugment.Description` / `.Tooltip`（`champions.go:312-313`）**已经在抓了**，前端在 `:2054` 那条分支上没用（同 B1） |

**结论与改法：**
- **斗魂**：在 `fallbackGameplayAugments` 之后追加一次 `/latest/cdragon/arena/zh_cn.json`，按 `id` 左连接补 `desc`。**先做 LCU 那条的验证**，如果客户端那份就有 `desc`，则只在未连客户端时才需要这次额外请求。
- **海克斯大乱斗**：不要走 `cherry-augments.json` 这条路，直接用 OP.GG 已抓到的 `row.description` / `row.tooltip`（和 B1 的图一起，同一次修复）。
- 说明文本含 `<spellName>` `<br>` `@MaxStacks@` 之类的富文本标记，`gameplay.go` 的 `cleanMarkup` 已经在处理，**海克斯大乱斗那条新链路也要过同一个 `cleanMarkup`**，别绕过去。
- 前端渲染继续走 `web/app.js:2487-2500` 的标题/正文 `textContent` 拆分，**不许 `innerHTML`**（红线 0.4）。

### B5 海克斯图标尺寸加大 + 全模式统一

用户说「海克斯的宽度有点小，加大点，如果其他模式也有这个问题一样修改」。目前尺寸由 `augmentIconFigure(id, size)` 的 `size` 参数决定，调用点分别传 `"small"`（`:1207`、`:1371`）与 `"large"`（`:1499`、`:2054`、`:2061`）。

改法：把这两档尺寸提成 CSS 变量在一处定义，`large` 档整体上调，并检查窄容器查询下是否需要降档。**不要在调用点上零散地改 size 字符串**，否则下次又会分叉。

---

## C 组 · 符文块（图二、图十三）

### C1 统一走 `renderUnifiedRuneBoard`

用户对图二的要求逐条对应到代码：

| 要求 | 目标实现 |
|---|---|
| 去掉「主宰 / 巫术 / 属性碎片」这些文字 | `renderUnifiedRuneBoard`（`web/gameplay.js:2162-2176`）本来就不渲染分路名 |
| 主副系图标放中间 | `<header aria-hidden="true">${renderRuneStyleIcon(style)}</header>` |
| 副系顶部那行去掉 | `slots.slice(1)`，剥掉副系基石行 |
| 属性碎片展示九个 | `state.perks.statModSlots` 三行 × 三个 = 9（后端默认值见 `gameplay.go:3601-3619`） |
| 和主副系下面三行对齐 | `is-spacer` 占位行 |

也就是说**目标实现已经存在**，问题是对局详情/战绩详情那块用的是**另一个** `renderRuneBoard`（`web/gameplay.js:1531` 一带）。改法是让两处收敛到 `renderUnifiedRuneBoard` 一个函数，删掉旧的那份，而不是把旧的照着改一遍。

**注意坑：** 后端 `defaultGameplayStatModSlots`（`gameplay.go:3601-3619`）三行分别是 `{5005, 5008, 5007}` / `{5008, 5010, 5001}` / `{5011, 5013, 5001}` —— `5008`（适应之力）和 `5001`（成长生命值）各出现两次，这是**正确的**，Riot 本来就是这样。第七轮已经因为「碎片重复」误判过一次，别再把它当 bug 去重。`champions_structured.go:566-586 runeShardSlots` 用的是同一组固定三行，按位置匹配 `stat_mod_ids`，两边必须保持一致。

### C2 主副系符文图标偏大

相关 CSS：

```
web/gameplay.css:680   .game-icon.is-rune-style { flex-basis: 32px; width: 32px; height: 32px }
web/gameplay.css:695   .rune-option-button      { width: 32px; height: 32px }
web/gameplay.css:696   .unified-rune-board .game-icon.is-rune { width: 30px }
web/gameplay.css:914   @container (…宽) { .rune-option-button { width: 38px … } }   ← is-rune-style 不跟着变
```

两点：

1. **盒子尺寸本身是 32 vs 30，差距很小**，但截图里视觉比例约 1.25:1。差额来自 `is-rune-style` 上的 `overflow: visible` —— 系图标 SVG 的画面填满整个 viewBox 并溢出盒子，而符文圆图自带留白。**去掉 `overflow: visible`** 是最直接的一刀。
2. **宽容器下比例会翻转**：`:914` 把 `.rune-option-button` 放大到 38px，但 `is-rune-style` 还停在 32px，于是系图标反而变小。这说明尺寸关系是**容器宽度相关的**，必须统一。

改法：把两者绑到同一个变量（如 `--rune-icon-size`），在容器查询里**一起**改，系图标取 `calc(var(--rune-icon-size) * 1.0)` 或略小。改完要在窄 / 中 / 宽三档容器下各看一次。

---

## D 组 · 交互杂项（图七、图八、图九，以及跨页滚动）

### D1 【根因确认】点击后提示框残留（图七）

`web/app.js:2511-2524` 注册的隐藏时机只有四个：`pointerout`、`focusout`、`Escape`、以及 `position()` 里发现锚点已脱离文档。**没有 `click` / `pointerdown`。**

再点一次好友按钮关闭抽屉时：指针从未离开按钮 → `pointerout` 不触发；按钮仍在文档里 → `position()` 不清理。于是提示框留在原地。

**还有第二层，更致命** —— `web/app.js:2517`：

```js
if (!current || current !== anchor || (…) || document.activeElement === current) return;
```

按钮点击后**保持焦点**，`document.activeElement === current` 恒成立，于是**即使把鼠标移开也不会隐藏**。这解释了为什么提示框会一直挂着。

第三层：`web/friends.js:263` 每次 `updateBadge()` 都会重写 `el.toggle.dataset.tooltip`，但提示框内容只在 `show()` 时读一次，已显示的提示框内容会**过期**。

**改法：**
1. 加 `document.addEventListener("pointerdown", …)`：命中当前锚点时 `hide()`，并置一个「本次交互内不再自动重显」的抑制位。
2. **抑制位是必需的** —— 因为可聚焦元素在点击后会触发 `focusin`（`:2520`），不加抑制会立刻 `show()` 回来。抑制位在下一次真正的 `pointerout`（或指针移出锚点）时清除。
3. `pointerout` 那条的 `document.activeElement === current` 守卫过宽。它本意是「键盘聚焦时别因鼠标移开而消失」，但对鼠标点击产生的焦点同样生效。改成只在**键盘触发的焦点**下保留（例如配合 `:focus-visible` 判定，或记录焦点来源）。
4. 锚点的 `data-tooltip` 变化时同步刷新已显示内容（`MutationObserver` 太重，简单做法是在 `updateBadge` 里判断 `el.toggle.getAttribute("aria-describedby")` 存在时主动 `hide()`）。

### D2 【根因确认】好友游戏时长不增长（图八）

两个独立原因：

**(a) 显示精度到秒，刷新周期 30 秒。** `web/friends.js:49-55 formatDuration` 输出 `m:ss`（带秒），而 `web/friends.js:272`：

```js
function startTick() { stopTick(); state.tickTimer = setInterval(updateDurations, 30_000); }
```

秒位每 30 秒才跳一次，看上去就是「卡住不动」。**改法：要么把 tick 降到 1 秒（只在抽屉打开时跑，`stopTick` 已在关闭时调用，开销可忽略），要么把显示精度降到分钟。** 建议前者 —— 用户明确在意的是「随时间正确增加」。

**(b) 打开抽屉时不刷新。** `web/friends.js:281-287`：

```js
if (open) {
  el.dock.classList.add("is-open");
  render();
  if (state.stale || state.error) void loadFriends();   // ← 只在 stale 时才拉
```

`state.stale` 只由 `deep-legends:friends-updated` 事件置位（`:362-365`），而该事件源自 `connection_manager.go:188` 的 LCU 聊天事件推送，**没有周期性兜底**。若期间客户端没推事件（或推送链路断了），抽屉会拿着一份任意旧的快照渲染 —— 中途开始游戏的好友根本不会出现。

**改法：`setOpen(true)` 时无条件 `void loadFriends()`**，正如用户说的「之后在每次打开时刷新」。`render()` 先跑一次给即时反馈、`loadFriends()` 回来再重绘的现有顺序保持不变，不会闪。

### D3 冗余提示框清理（图九）

项目里**已经有**正确的机制：`web/app.js:2431-2442` 的 `data-tooltip-overflow` —— 只有当目标节点真的被截断（`scrollWidth > clientWidth`）时才显示提示框。

```js
const targetOf = (node) => {
  const next = rawTargetOf(node);
  if (!next?.dataset.tooltipOverflow) return next;
  const target = overflowTarget(next);
  return target && (target.scrollWidth > target.clientWidth + 1 || …) ? next : null;
};
```

所以「旁边已经有文字说明就别弹提示框」的正确改法不是删提示框，而是**给这些锚点补上 `data-tooltip-overflow`**。

**需要逐个排查的清单**（当前**没有** `data-tooltip-overflow`、但旁边已有可见文字的锚点）：

| 位置 | 现状 | 判断 |
|---|---|---|
| `web/gameplay.js:1091` 战绩卡玩家按钮 | 已有 `data-tooltip-overflow=".match-player-name"` | ✅ 正确，别动 |
| `web/gameplay.js:1371` 斗魂详情玩家 | 已有 `.participant-name` | ✅ |
| `web/gameplay.js:795` 常遇玩家 | 已有 `.recent-player-name` | ✅ |
| `web/friends.js:208` 好友行 | 已有 `.friend-game-name` | ✅ |
| `web/champions.js:118 iconFigure(…, withTooltip = true)` | **默认开启**，英雄名旁边通常已有文字 | ❌ 图九主体。默认值应改为 `false`，由调用点显式开启 |
| 熟练度模块 | 图九点名 | ❌ 待定位具体行，同上处理 |
| `web/champions.js:524` 位置标签页 | `data-tooltip="${item.label}"`，按钮内已有 `<span>${item.label}</span>` | ❌ 纯重复，去掉或改 overflow |
| `web/gameplay.js:1094` 斗魂队伍行 | tooltip 是「第 N 名 · 魄罗」 | ⚠️ A3 落地后行内就有名次了，届时改为 overflow 或去掉 |

`iconFigure` 的默认值从 `true` 改成 `false` 是**影响面最大**的一刀，会波及所有调用点，必须全量走查一遍再改，并补一条测试锁住默认值。

### D4 【根因确认】跨页切换保留上一页的滚动位置

`web/app.js:1593-1625 activateSection` 做了这些事：设置 `state.section`、换标题、切 `panel.hidden`、可选包在 `document.startViewTransition` 里、重置收藏/设置子标签页 —— **唯独没碰 `el.appScroll.scrollTop`**。

五个面板共用**同一个滚动容器** `el.appScroll`（见 `:2408` / `:2415` 的回到顶部按钮），所以切页后滚动位置原样保留在新页面上。

**现成的正确写法就在项目里** —— `web/champions.js:348 / 359 / 468`：

```js
state.listScroll = appScroll?.scrollTop || 0;
…
appScroll?.scrollTo({ top: state.listScroll, behavior: "instant" });
```

**改法：** 在 `activateSection` 里，离开当前 section 前把 `el.appScroll.scrollTop` 存进 `state.sectionScroll[oldSection]`，切换后恢复 `state.sectionScroll[newSection] ?? 0`。

两个细节：
- 恢复必须用 `behavior: "instant"`，否则会和 `startViewTransition` 打架产生滑动残影。
- 必须在 `panel.hidden` 切换**之后**再恢复 —— 隐藏中的面板高度为 0，容器 `scrollHeight` 还没长出来，提前 `scrollTo` 会被夹到 0。这条是最容易踩的坑，建议在实现时写注释标出来。

---

## E 组 · 推荐数据正确性与排版（图十、图十一、图十二、图十三）

图十（「出门装买不起」）和图十二（「有些胜率 100%，有些又没有」）是**同一条因果链上的四个缺陷叠加**，必须一起修，单修任何一个都还是错的。

### E1 【根因确认·首要】`rate()` 把百分比又乘了一次 100

`web/gameplay.js:2357-2361`：

```js
function rate(value) {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return "—";
  return `${(parsed <= 1 ? parsed * 100 : parsed).toFixed(1).replace(/\.0$/, "")}%`;
}
```

后端**已经是百分比量纲**了 —— `champions.go:1489-1494`：

```go
func ratePercent(value float64) float64 { if value > 0 && value <= 1 { return value * 100 }; return value }
func percentOf(value, total int) float64 { if value <= 0 || total <= 0 { return 0 }; return float64(value)/float64(total)*100 }
```

于是「0.42%」（一个真实的长尾出门装选取率）在前端被当成小数 0.42，渲染成 **42%**。这就是图十里「上面两行正确（96.9% / 1.1% 都 > 1，不触发放大），下面几行全是几十个百分点」的确切原因 —— 不是数据源选错了英雄，是**任何小于 1% 的真实值都会被放大 100 倍**。

`web/champions.js:72` 的 `percent()` 量纲是对的：

```js
function percent(value) { const parsed = Number(value); return Number.isFinite(parsed) && parsed !== 0 ? `${parsed.toFixed(2)}%` : "—"; }
```

**改法：删掉 `rate()` 里的 `parsed <= 1 ? parsed * 100` 分支**，与 `percent()` 对齐。全项目搜一遍 `rate(` 的调用点（至少 `:2080` 海克斯表现/热度、`:2112-2116` 胜率/选取率/禁用率、`:2304-2306` `renderOptionStats` 三处），确认它们的入参都来自后端百分比量纲。

**这条改动会同时修好图十的出门装、图十二的出装胜率、图十三头部的三个比率。**

**必须配变异测试**：把 `rate()` 改回带放大分支，测试必须失败。第七轮的教训是正则型/字面量型断言是假护栏。

### E2 【根因确认】`omitempty` 把真实的 0% 抹成了「—」

`gameplay.go:1928-1932`：

```go
type gameplayRecommendationStats struct {
    PickRate float64 `json:"pickRate,omitempty"`
    WinRate  float64 `json:"winRate,omitempty"`
    Games    int     `json:"games,omitempty"`
}
```

Go 的 `omitempty` 对 `float64(0)` 生效 → 一个**真实的 0% 胜率**（小样本组合里很常见）会被整个字段丢掉 → 前端 `Number(undefined)` 非有限 → `rate()` 返回 `"—"`。图十二里「有些又没有」就是这个。

**改法：去掉这三个字段的 `omitempty`**，让 0 如实序列化。前端要能区分「0%」和「没有数据」——建议后端改用指针（`*float64`）或补一个 `hasStats bool`，前端 `null` 才显示「—」。**只删 `omitempty` 是不够的**，因为那样「无数据」也会变成 0%，反而制造新的错数据。

`web/champions.js:72` 的 `percent()` 也有同类问题：`parsed !== 0` 让真实 0% 显示成「—」，一并修。

### E3 【根因确认】没有最小样本闸门

`champions_structured.go:384-409 structuredMetrics`：

```go
if len(value.IDs) == 0 || value.Play <= 0 { continue }
```

只要有 1 场就会被采纳。OP.GG 的 `starter_items` 是一条干净的降序分布（实测 Lux/ranked：`0.9773, 0.0038, 0.0032, 0.0030, 0.0016, …`），**长尾本来就是垃圾**。E1 修好之后这些行会正确显示成「0.4%」，但仍然没有推荐价值。

**改法：在 `structuredMetrics` 加一道闸门** —— 同时满足「`Play >= N`」与「占该分布头名的比例 >= M%」才保留（建议 N=50、M=1%，可调）。**建议做成常量并写进注释说明取值理由**，避免下次有人当魔数删掉。

### E4 【根因确认】合成的「出装路线」挂着别人的统计

`gameplay.go:2066-2121 completeRecommendationItemRoutes` 把 `cores[:3]` 和 FourthItems/FifthItems/SixthItems 里各取一件按 `index` 轮转拼出「路线」，然后在 `:2117` 把 `core.PickRate / WinRate / Games` **整个搬过来**当这条路线的统计。

这条路线在 OP.GG 上**从来不存在**，它的「选取率 20%」其实是核心装的选取率。图十二「为什么有些出装胜率是 100%」有一部分出自这里。

**改法（二选一，倾向 A）：**
- **A：不再合成。** 直接用 OP.GG 的真实分布：`core_items`（15 条）作为核心装、`last_items`（30 条）作为后期备选，各自带各自的真实统计，分成两块展示。用户要的是「能看的推荐」，不是「一条完整六件套」。
- **B：保留合成，但不显示统计。** 合成路线上把选取率/胜率去掉，只标「按热门核心装 + 常见后期装组合」，避免用错误数字误导。

顺带：`gameplayADCItemRouteLimit = 7` / `gameplayDefaultItemRouteLimit = 6` 两个上限在方案 A 下失去意义，一并清理。

### E5 出门装把两个分布拼在了一起（HTML 链路）

`champions.go:1983-2018 parseChampionBuild` 的 key 是 `caption(小写) + " " + heading(未小写)`，而实测 OP.GG 页面上**存在两个 `SummonerSpells Table`**；`champions.go:2128-2150 appendUniqueMetricRows` 只按资源路径签名去重，**不区分来源表**，于是两个不同分布的行被静默串成一段。

这条在改用结构化 API（F 组）之后会自然消失 —— 但**在 F 组落地之前**，HTML 链路仍是海克斯大乱斗的唯一来源，值得先加一道防护：`appendUniqueMetricRows` 增加来源表标识，同 key 但不同来源表时**报错或只取第一个**，不要静默拼接。

### E6 图十三 头部的英文与数字

`web/gameplay.js:1941`：

```js
`<span>${escapeHTML(data.gameMode || "游戏模式未知")} · 地图 ${number(data.mapId)}${data.gameId ? ` · 对局 ${number(data.gameId)}` : ""} · ${escapeHTML(note)}</span>`
```

`data.gameMode` 是内部枚举（截图里的 `KIWI` 就是海克斯大乱斗），`mapId` 是裸数字（`12` = 嚎哭深渊），`gameId` 是对用户毫无意义的内部编号。

**改法：** 建 `gameMode → 中文` 与 `mapId → 中文` 两张映射表，`gameId` 直接不展示（或收进提示框）。参考已有的 `gameplay.go:3907-3908 queueLabel`。

已知取值（其余待补，未命中时**回落到 `queueLabel` 而不是显示原枚举**）：

| gameMode | mapId | 中文 |
|---|---|---|
| `CLASSIC` | 11 | 召唤师峡谷 |
| `ARAM` | 12 | 极地大乱斗 |
| `KIWI` | 12 | 海克斯大乱斗 |
| `CHERRY` | 30 | 斗魂竞技场 |
| `URF` | 11 | 无限火力 |
| `NEXUSBLITZ` | 21 | 极限闪击 |

### E7 图十一 文案

`web/gameplay.js:2107`：

```js
<span>${players.filter(...).length} 名玩家 · 当前模式最近战绩，点击名字查看总览</span>
```

改成只保留「当前模式最新战绩」。

### E8 图十 排版重做

`web/gameplay.js:2232-2242 renderBuildRecommendation` 现在是：`<div class="build-essentials">`（召唤师技能 + 技能加点）+ `<div class="item-option-groups">`（出门装 / 鞋子 / 出装路线）+ client item-set 页脚。技能加点下方的空白就出在 `build-essentials` 这个两列容器里，右列内容比左列短。

**排版建议**（结合 E3/E4 之后内容量会变）：

```
┌─────────────────────────────────────────────┐
│  召唤师技能 [D][F]      技能加点 Q→W→E  主升序 │   ← 单行横条，不再是两列方块
├─────────────────────────────────────────────┤
│  出门装        │  鞋子        │  核心装       │   ← 三等分，每格纵向列表
│  ◻ 96.9%      │  ◻ 41.2%    │  ◻◻◻ 22.1%  │
│  ◻  1.1%      │  ◻ 30.8%    │  ◻◻◻ 18.4%  │
├─────────────────────────────────────────────┤
│  后期备选  ◻ ◻ ◻ ◻ ◻ ◻                      │   ← 横向一排，只给图标+胜率
└─────────────────────────────────────────────┘
```

要点：把「召唤师技能 + 技能加点」压成一条横向摘要条（它们信息量小、不该占半屏），腾出的纵向空间给真正有比较价值的装备分布；三个装备分组等宽，行内统一「图标 · 选取率 · 胜率 · 场次」四段式并用 `tabular-nums` 对齐。斗魂没有召唤师技能与符文（见 F1），横条要能优雅退化成只剩技能加点。

---

## F 组 · 模式化推荐方案 ★（本轮主功能）

> 目标：**对局页按当前模式，从 OP.GG 对应模式取推荐**；有符文的模式推符文，没符文的（海克斯大乱斗、斗魂）推海克斯；绝活哥 / 职业选手**只在单双排与灵活排**出现。

### F1 OP.GG 接口实测基线（2026-08-21，Lux id=99）

**这是整个方案的事实地基，实现时以此为准。**

**模式枚举**：`lol-api-champion.op.gg` 只接受 `ranked` / `aram` / `arena` / `urf` / `nexus_blitz`。其他一律 HTTP 422 `{"message":"Mode was invalid"}`（已逐个验证过 `brawl` / `swarm` / `ultbook` / `one_for_all` / `doombots` / `aram_clash` / `hexgates` / `arena_2v2v2v2` / `aram_mayhem` / `aram-mayhem`）。

**URL 形状**：`/api/{REGION}/champions/{mode}/{championId}/{position}`

| 模式 | 区域 | position 段 | 实测 |
|---|---|---|---|
| `ranked` | `KR` | 必须是 `TOP/JUNGLE/MID/ADC/SUPPORT` 之一 | 传 `none` → 422 |
| `aram` | `KR` | **必须字面量 `none`** | 传 `mid` → 422「The position must be NONE」 |
| `urf` | `KR` | 同上 `none` | 200 |
| `nexus_blitz` | `KR` | 同上 `none` | 200 |
| `arena` | **`global`** | **完全不带 position 段** | 加 `/none` → 422；用 `KR` → 422 |

`tier` 查询参数在 `aram` 上返回 200（被忽略或被接受，无害）；`ranked` 需要有效 tier。

**各模式载荷字段（有=✅）**

| 字段 | ranked | aram | urf | nexus_blitz | arena |
|---|---|---|---|---|---|
| `summary` | ✅ | ✅ | ✅ | ✅ | ✅ |
| `summoner_spells` | 5 | 5 | 5 | 5 | **✗** |
| `starter_items` | 15 | ✅ | ✅ | ✅ | **0** |
| `boots` | 5 | ✅ | ✅ | ✅ | 7 |
| `core_items` | 15 | ✅ | ✅ | ✅ | 15 |
| `last_items` | 30 | ✅ | ✅ | ✅ | 30 |
| `prism_items` | ✗ | ✗ | ✗ | ✗ | **10** |
| `mythic_items` | 0 | 0 | 0 | **5** | ✗ |
| **`rune_pages`** | **5** | **5** | **5** | **5** | **✗** |
| `runes` | 5 | 5 | 5 | 5 | ✗ |
| `skill_masteries` | 4 | 5 | ✅ | ✅ | 5 |
| `skills` | 5 | 5 | 5 | 5 | 5 |
| `counters` | 35 | **0** | **0** | ✅ | **✗** |
| `augment_group` | ✗ | ✗ | ✗ | ✗ | **3** |
| `synergies` | ✗ | ✗ | ✗ | ✗ | **40** |
| `meta.version` | **16.16** | **16.16** | **16.04** | **13.23** | 16.16 |

**`rune_pages` 结构在四个有符文的模式里完全一致**（实测逐字段比对）：

```
rune_pages[i] = { id, primary_page_id, secondary_page_id, play, win, pick_rate, builds[] }
builds[j]     = { id, primary_page_id, primary_rune_ids, secondary_page_id,
                  secondary_rune_ids, stat_mod_ids, play, win, pick_rate }
```

→ **`champions_structured.go:498-544 structuredRunes` 与 `:566-586 runeShardSlots` 可以原封不动复用，无需按模式分支。** 这是本方案能低成本落地的关键。

**两个必须处理的坑：**
1. **`urf` 的 `meta.version` 是 16.04，`nexus_blitz` 是 13.23** —— 后者落后当前版本（16.16）**几十个补丁**，数据已经完全失效。必须做新鲜度闸门（F5）。
2. **海克斯大乱斗（KIWI）在 OP.GG 上没有对应模式。** `aram_mayhem` / `hexgates` 全部 422。最接近的是 `aram`（同地图、同英雄池、同随机机制），但海克斯本身完全不同。方案见 F2 与 F6。

### F2 模式路由表

判定输入优先级：`queueId` → `gameMode` + `mapId` → 兜底。

**不要只靠 queueId 白名单**（`gameplay.go:3919-3935 queueModeGroup` 现在就是这么做的，所以海选赛之类的新队列全掉进 `"other"`）。`gameMode` + `mapId` 组合是稳定判据 —— 例如 `KIWI` + `12` 唯一确定海克斯大乱斗，无论 Riot 又开了多少个变体队列。

| 游戏内模式 | queueId | gameMode | mapId | → OP.GG mode | position | region | 推荐符文 | 推荐海克斯 | 绝活哥/职业 |
|---|---|---|---|---|---|---|---|---|---|
| 单双排 | 420 | CLASSIC | 11 | `ranked` | 实际分路 | KR | ✅ | ✗ | **✅** |
| 灵活排 | 440 | CLASSIC | 11 | `ranked` | 实际分路 | KR | ✅ | ✗ | **✅** |
| 匹配 | 400/430 | CLASSIC | 11 | `ranked` | 实际分路 | KR | ✅ | ✗ | ✗ |
| 极地大乱斗 | 450 等 | ARAM | 12 | `aram` | `none` | KR | ✅ | ✗ | ✗ |
| 海克斯大乱斗 | 2300/2400 | **KIWI** | 12 | `aram`（降级） | `none` | KR | ✅ | **✅** | ✗ |
| 斗魂竞技场 | 1700/1710 | CHERRY | 30 | `arena` | **无** | **global** | **✗** | **✅** | ✗ |
| 无限火力 | 900/1900 | URF/ARURF | 11 | `urf` | `none` | KR | ⚠️ 见 F5 | ✗ | ✗ |
| 极限闪击 | 1300 | NEXUSBLITZ | 21 | `nexus_blitz` | `none` | KR | ⚠️ 见 F5 | ✗ | ✗ |
| 其他 | — | — | — | `ranked`（降级并标注） | mid | KR | ✅ | ✗ | ✗ |

**当前代码的错误行为**（`gameplay.go:1950-1998`）：

```go
mode := "ranked"
if source == "arena" { mode = "arena" } else if source != "" { 400 }
…
loadDetail(ctx, mode, slug, position, championCounterFallbackTier)
```

加上 `normalizeOPGGPosition`（`:1985-1998`）把空分路映射成 `"mid"` —— 于是**一局海克斯大乱斗的库奇，被当成排位中单去查**。这正是 E3 那些「只有个位数场次」的小样本从哪来的。

### F3 后端改造

**F3.1 泛化请求构造器。** `champions_structured.go:307-382 loadStructuredDetail` 现在是两条硬编码分支：

```go
requestPath := "/api/KR/champions/ranked/" + strconv.Itoa(id) + "/" + position
query := url.Values{"tier": {tier}}
region := "KR"
if mode == "arena" {
    requestPath = "/api/global/champions/arena/" + strconv.Itoa(id)
    query = nil
    region = "GLOBAL"
}
```

改成表驱动：

```go
type opggModeSpec struct {
    APIMode      string // ranked / aram / urf / nexus_blitz / arena
    Region       string // KR / global
    PositionMode int    // required / literalNone / omitted
    UsesTier     bool
    HasRunes     bool
    HasAugments  bool
    HasCounters  bool
    MinVersion   string // 新鲜度闸门基准，见 F5
}
```

`loadStructuredDetail` 只读这张表，不再有 `if mode == "arena"`。**这张表就是 F1 的载荷矩阵，一一对应。**

**F3.2 放开模式白名单。** `champions.go:517-546 handleChampionRankings` 与 `:553-573 handleChampionDetail` 目前只认 `ranked` / `aram-mayhem` / `arena`，必须扩到新枚举。注意 `aram-mayhem` 是**项目内部**叫法（对应海克斯大乱斗），与 OP.GG 的 `aram`（极地大乱斗）**不是一回事**，扩展时不要合并 —— 建议内部枚举统一改成 `ranked / aram / hextech-aram / arena / urf / nexus-blitz`，并在一处集中做「内部枚举 → OP.GG mode」翻译。

**F3.3 `loadDetailOnce` 的分支同样表驱动。** `champions_structured.go:145-161`：

```go
if mode == "ranked" || mode == "arena" { loadStructuredDetail(...) } else { HTML }
```

改成「凡在 `opggModeSpec` 表里的模式都走结构化 API」。HTML 链路（`champions.go:1898-1947 loadDetailHTML`）**只保留给海克斯大乱斗的海克斯数据**（F6），其余全部退役 —— 这顺带解决 E5。

`TopPlayers`（绝活哥）目前也在 `loadDetailOnce` 里只对 ranked 加载，这个条件是对的，保留。

**F3.4 推荐路由。** `gameplay.go:1950-1983 handleGameplayRecommendations` 改为：接收 `queueId` / `gameMode` / `mapId`（而不是现在的自由 `source` 字符串），按 F2 表解析出 `opggModeSpec`，再调 `loadDetail`。`normalizeOPGGPosition`（`:1985-1998`）只在 `PositionMode == required` 时生效，其余模式直接给字面量 `none` 或不带段。

**F3.5 响应要带上「这份数据是从哪个模式取的」。** 加 `resolvedMode` / `resolvedRegion` / `dataVersion` / `isFallback` 四个字段。前端要靠它显示降级提示（F6），排查问题时也不用猜。

### F4 前端改造

**F4.1 推荐类型矩阵。** `web/gameplay.js:2038` 现在的页签是：

```js
${tab("runes", augmentSource ? "海克斯" : "符文")}${tab("insight", "详情")}${tab("build", "出装与技能")}
```

`augmentSource` 是个二元开关，不够用。改成由后端 `resolvedMode` 驱动的能力位（`hasRunes` / `hasAugments` / `hasTopPlayers`），页签按能力位增删。**海克斯大乱斗要同时有符文和海克斯两个页签**（它既有符文又有海克斯），这是当前二元开关表达不了的情况。

**F4.2 符文来源分区按队列条件化。** `web/gameplay.js:2119-2130 renderRuneRecommendations` 硬编码了三块：

```js
{ key: "opgg", title: "OPGG" }, { key: "specialist", title: "绝活哥" }, { key: "pro", title: "职业选手" }
```

改成：`opgg` 恒在；`specialist` / `pro` **仅当 queueId ∈ {420, 440}** 时渲染。不是「有数据就显示」—— 因为 `loadDetailOnce` 在非 ranked 模式下压根不会加载 `TopPlayers`，靠数据判空会得到一个空分区而不是不渲染分区。**要显式按队列判定。**

**F4.3 符文块统一。** 见 C1，`renderUnifiedRuneBoard` 是唯一实现。

### F5 数据新鲜度闸门（必做，否则会推 13.23 版本的符文）

实测 `nexus_blitz` 的 `meta.version` = **13.23**，落后当前 16.16 几十个补丁；`urf` = 16.04，落后约 12 个。

**改法：** `loadStructuredDetail` 拿到响应后比较 `meta.version` 与当前游戏版本（项目里已有版本来源，`catalog.go` 那条链路）：

- 差距 ≤ 2 个小版本：正常展示。
- 差距 > 2：正常展示，但在推荐区顶部挂一条**明确的**提示 ——「该模式数据停留在 13.23 版本，仅供参考」。**不要静默展示**，也**不要直接不展示**（用户宁可看过时数据也不想看空白）。
- `meta.version` 缺失：按「差距 > 2」处理。

版本比较要按数字分段比（`16.4` vs `16.04` vs `16.16`），不要字符串比较 —— 字符串比较下 `"16.4" > "16.16"`，会得到相反结论。这是个很容易写错的地方，**建议单独写单测覆盖 `16.16 / 16.04 / 16.4 / 13.23 / ""` 五个用例**。

### F6 海克斯大乱斗的降级策略（唯一没有 OP.GG 原生支持的模式）

- **符文 / 出装**：走 `aram`（极地大乱斗）。同地图、同英雄池、同随机机制，符文与出装的迁移性合理。**必须在 UI 上标明来源是极地大乱斗**（用 `isFallback` + `resolvedMode` 显示「参考极地大乱斗数据」），不要假装是原生数据。
- **海克斯**：**继续**走现有的 OP.GG aram-mayhem HTML 链路（`champions.go:1688 loadAugments`）—— 这是目前唯一的海克斯大乱斗海克斯数据源，不要在这轮一起换掉。同时按 B1 修好图标、按 B4 修好说明。
- **斗魂海克斯**：走 `arena` 的 `augment_group`（3 组），配 `cherry-augments.json` 的品质与 `cdragon/arena/zh_cn.json` 的说明。

### F7 缓存键必须带上模式与区域

现有缓存键若只含 `champion + position + tier`，加入多模式后会**串号**（同一个英雄在 aram 和 ranked 下拿到同一份缓存）。缓存键改为 `mode|region|championId|position|tier`。这条不做的话，症状会表现为「切模式后推荐没变」，而且极难排查 —— **请优先确认现有键的构成**。

### F8 分期与验收

| 期 | 内容 | 验收 |
|---|---|---|
| **P0** | E1 + E2（量纲与 0%） | 图十的出门装比例合计接近 100%；图十二不再有无来由的 100% / 「—」 |
| **P1** | F3.1 + F3.2 + F7（表驱动 + 缓存键） | 同一英雄在 ranked / aram 下拿到**不同**的 `rune_pages`；切模式推荐立即变化 |
| **P2** | F3.4 + F4.1 + F4.2（路由 + 页签 + 绝活哥条件化） | 斗魂无符文页签；海克斯大乱斗同时有符文与海克斯两个页签；只有 420/440 出现绝活哥与职业选手 |
| **P3** | F5 + F6（新鲜度 + 降级提示） | 极限闪击顶部出现 13.23 过时提示；海克斯大乱斗符文区标注「参考极地大乱斗」 |
| **P4** | E3 + E4（最小样本 + 停止合成路线） | 出门装不再出现 < 1% 的长尾行；出装路线要么消失、要么不带统计数字 |

**每一期都要能独立验收**，不要一次性合并提交。

---

## G 组 · 国服其他玩家胜率仍不可用（图一）

**结论不变，且已经不需要再猜。** 更早那份 `ranked_winrate_resolved` 分布：

| 分类 | 条数 |
|---|---|
| sgp 有胜无负（被抑制） | 1925 |
| sgp 0 胜 0 负 | 1217 |
| lcu 有胜无负（被抑制） | 493 |
| lcu 0 胜 0 负 | 213 |
| **sgp 胜负齐全** | **42（全部是本人）** |

**零条完整的 LCU 记录**，且 42 条完整记录无一例外是登录玩家自己。这排除了「解析 bug」（H2），坐实了「腾讯服务端策略」（H1）—— 国服对非本人**不下发负场数**。这不是能靠找接口解决的问题。

**方向 A7（建议）：** 不再试图从 rankedStats 拿他人胜率。改为**从战绩列表自行统计**「近 N 局 x 胜 y 负」，并且**绝对不要把它写回 `gameplayRank.WinRate`** —— 那个字段的语义是「赛季排位胜率」，塞进近 N 局数据会污染所有依赖它的判断（含 `verifiedRankWinRate`）。新开一个字段，UI 上也明确写「近 N 局」。

**本轮日志无法验证第八轮埋点** —— 见 0.1。请用包含 A/B/C 的构建重跑一次真机，重点看有没有 `sgp_ranked_stats_shape` 事件，以及其 `privacy` 分布。

**另外，第八轮遗留的 4 条改进项仍未处理**（`capture()` 静默出口、`privacy` 在高频路径恒为 UNKNOWN、`queue_keys` 只采样首条、`lp_capture_poll` 不带 game_id）—— 其中第 2 条会让 A5 决策树拿不到有效样本，建议和本轮一起做掉。

---

## 验收清单

**必须在开发机执行（沙箱跑不了 Go）：**

```
go vet ./...
go test ./...
go test -race ./...
go build ./...
node --test web/*.test.cjs desktop/*.test.cjs
```

前端改动需要 `go build` 后才生效（`web/` 是 go:embed 的）。上一轮基线是 **65 pass / 0 fail**，本轮新增测试后只许增不许减。

**必须补的测试（都要先做变异验证再判定有效）：**

1. `rate()` 不再放大 —— 变异：改回 `parsed <= 1 ? parsed * 100`，测试须失败（E1）
2. `winRate: 0` 与 `winRate: null` 渲染结果不同（E2）
3. `renderLiveAugmentIcon` 非 arena 分支使用 `imagePath`（B1）
4. `augmentIconFigure` 输出含品质类，且五个枚举全覆盖（B2）
5. `opggModeSpec` 表：五个模式的 URL 形状与 F1 表逐字段一致（F3.1）
6. 版本比较：`16.16 / 16.04 / 16.4 / 13.23 / ""` 五个用例（F5）
7. 缓存键含 mode 与 region（F7）
8. 绝活哥/职业选手分区只在 queueId 420/440 出现（F4.2）
9. `activateSection` 切页后 `scrollTop` 归位（D4）
10. 提示框在 `pointerdown` 后隐藏且不因 `focusin` 立即重现（D1）

**变异测试的正确姿势**（第七轮踩过坑）：必须 `cp -r web desktop *.go /tmp/rtN/` 做**真副本**。软链接不行 —— Node 把 `__dirname` 解析成 realpath，会读回原文件，测试看起来通过其实什么都没验。

---

## 红线复述

1. **不许 `innerHTML`。** 提示框标题/正文拆分继续走 `web/app.js:2490-2500` 的逐节点 `textContent`。
2. **埋点不许落值型标识。** 只允许键名、布尔、计数、`privacy` 枚举。`accountHash` 即使加盐也是稳定玩家标识，不得写日志（`lp_tracker.go:11`）。
3. **不许 `light-dark()`。** 全项目禁用。
4. **`color-mix` 用 `in oklab`**，近中性色/对色相操作数在 `oklch` 下会翻车。
5. **主题名以 UI 为准**：自动 / 浅色 / **海克斯黑金** / 寒冰蓝 / 翡翠绿 / 星域紫 / 血月红 / 极光青 / 纯黑 OLED。**项目里没有「深色」这个主题。**
