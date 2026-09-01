# 战绩卡高度回归 + 名次徽章重设计（第十轮，2026-08-20）

来源：用户实机截图（英雄页「吃鸡战绩」卡片被撑到 ~220px；720px 窄屏下玩家列表掉到卡片下方又撑高一截）。

**先说明责任**：第 13 条工单里我写了「别再截断队伍：`slice(0, 4)` → 全量」，这条建议是错的。
`slice(0, 4)` 不是随手写的截断，而是**战绩卡 118px 固定高度的锚点**。下面第 1 条给出量化推导。

---

## 1. 恢复「只显示 4 支队伍」——高度是硬约束

### 现状

```js
web/gameplay.js:1092
const rows = grouping.groups.map((group) => { … })   // ← 第八轮改成了全量
```

### 高度推导（这就是必须回到 4 行的原因）

卡片的固定高度在两处声明：

```css
web/gameplay.css:303  .match-entry   { min-height: 118px; contain-intrinsic-size: 118px; … }
web/gameplay.css:306  .match-summary { … min-height: 118px; padding: 10px 10px 10px 14px; }
```

单行队伍条的实际高度（`web/gameplay.css:392`）：

| 组成 | 尺寸 |
|---|---|
| 内容（`.game-icon.is-tiny`，`gameplay.css:608`） | 18px |
| `padding: 3px 5px` 上下 | +6px |
| `border: 1px solid` 上下 | +2px |
| **单行合计** | **26px** |

行间 `gap: 3px`（`.match-players.is-arena`，`:390`）：

| 行数 | 名单列总高 | 对 118px 卡片 |
|---|---|---|
| **4 行** | 4×26 + 3×3 = **113px** | ✅ 塞得进（118 − 上下 padding 20 = 98px 内容区…见下） |
| 7 行 | 7×26 + 6×3 = **200px** | ❌ 卡片被撑到 200 + 20 padding ≈ **220px** |

截图里第一张卡实测约 220px，和推导吻合。

> 注：严格说 118px 减去 `padding: 10px` 上下后内容区是 98px，4 行的 113px 也略微超出，
> 卡片会长到约 133px。如果要**严丝合缝**对齐其他模式的 118px，可以把行内 padding 从 `3px` 收到 `2px`
> （单行 24px）并把 gap 从 3px 收到 2px → 4×24 + 3×2 = 102px，接近 98px。
> 这个微调由你定，但**行数必须回到 4**，否则怎么调都对不齐。

### 改法

`web/gameplay.js:1092` 恢复 `.slice(0, 4)`：

```js
const rows = grouping.groups.slice(0, 4).map((group) => { … })
```

**信息不会丢**：完整的 6~7 支队伍在展开详情里本来就全量呈现
（`renderArenaMatchDetail`，`web/gameplay.js:1384` 起，按小队分组展示全部玩家 + 海克斯 + 装备）。
卡片正面只是预览，点开就能看全。

### 追加硬护栏（建议一并做）

光靠 `slice(0,4)` 还是「约定」，将来谁改了又会破。建议给名单列加一道 CSS 兜底：

```css
.match-players.is-arena { max-height: 113px; overflow: hidden; }
```

这样即便 JS 层再回归，卡片高度也不会失控。

---

## 2. 窄屏（≈720px 及以下）直接隐藏玩家列表

### 现状

窄屏下不是隐藏，而是**把名单挪到卡片下方另起一行**，等于又加了一截高度——这正是截图二那种观感：

```css
web/gameplay.css:447-455   @container matches-column (max-width: 680px)
	.match-players:not(.is-arena) { display: none; }          ← 普通模式隐藏了
	.match-summary.is-arena .match-players.is-arena { grid-column: 1 / 3; grid-row: 2; }   ← 斗魂反而挪到第二行

web/gameplay.css:477-484   @container arena-first (max-width: 640px)
	.match-summary.is-arena .match-players.is-arena { grid-column: 1 / 3; grid-row: 2; }   ← 同样

web/gameplay.css:465-476   @container matches-column (max-width: 420px)
	.match-summary.is-arena .match-players.is-arena { grid-column: 1 / -1; grid-row: 3; }

web/gameplay.css:486-497   @container arena-first (max-width: 520px)
	.match-summary.is-arena .match-players.is-arena { grid-column: 1 / -1; grid-row: 3; }
```

也就是说普通模式在 680px 就干脆隐藏了，唯独斗魂被特殊对待、换行保留——**这是第八轮我建议
「斗魂分支改为只保留名次+头像，而不是整块消失」造成的，同样是我的判断失误**。用户的诉求更简单也更对：
窄屏就不显示，别动高度。

### 改法

上面四个断点里的 `.match-summary.is-arena .match-players.is-arena { grid-column…; grid-row… }`
全部替换成隐藏，并让斗魂跟普通模式共用同一条规则：

```css
/* matches-column ≤680 与 arena-first ≤640 两处 */
.match-players, .match-players.is-arena { display: none; }
.match-summary, .match-summary.is-arena { grid-template-columns: 108px minmax(0,1fr) 40px; }
```

同时把 `align-items: start` 改回 `center`（`:451`、`:478` 里为了两行布局改成了 start，
隐藏名单后应该回到垂直居中），并删掉 `matches-column ≤420` / `arena-first ≤520` 里
那两条把名单放到 `grid-row: 3` 的规则（名单已经不存在了，规则悬空）。

### 断点取值

用户说的是「720px 左右」。目前两条链路的第一档分别是 `matches-column ≤680` 和 `arena-first ≤640`。
考虑到英雄页的 pane 宽度天然比总览页窄，建议：

- `@container matches-column (max-width: 680px)` —— 保持不变，普通模式已经在这里隐藏
- `@container arena-first (max-width: 640px)` —— 保持不变

也就是**不需要动阈值，只需要把「换行保留」改成「隐藏」**。如果你实测下来 720px 时还是觉得挤，
再把 `arena-first` 那档从 640 提到 720 即可（改一个数字）。

---

## 3. 前三名徽章重新设计

### 现状（太素）

```css
web/gameplay.css:394-396
.arena-rank-chip { display: grid; width: 18px; height: 18px; place-items: center;
  color: var(--muted); background: transparent; border: 1px solid var(--line);
  border-radius: 5px; font-size: 9.5px; font-weight: 800; font-variant-numeric: tabular-nums; }
.arena-rank-chip.is-rank-1 { color: var(--on-primary); background: var(--primary); border-color: transparent; }
.arena-rank-chip.is-rank-2, .arena-rank-chip.is-rank-3 { color: var(--ink); background: var(--surface-strong); }
```

问题：2、3 名共用一条规则，视觉上完全一样；1 名只是填了主题色，没有「冠军感」；
4~7 名的灰边框和 2/3 名的灰底也不够拉开层次。

### 建议设计（金 / 银 / 铜三档 + 中性档）

沿用第 6 条海克斯品质色的经验教训：**名次是游戏语义，用固定色，不要绑 `--primary` 这类主题令牌**
（否则换紫色主题第一名就变成紫色，跟"冠军=金"的直觉冲突）。

```css
.arena-rank-chip {
  display: grid; width: 18px; height: 18px; place-items: center;
  color: var(--muted); background: transparent;
  border: 1px solid var(--line); border-radius: 5px;
  font-size: 9.5px; font-weight: 800; font-variant-numeric: tabular-nums;
  --rank-tone: transparent;
}
.arena-rank-chip.is-rank-1 { color: #2A1D05; background: linear-gradient(160deg,#F5D372,#D9A441); border-color: #E8C468; box-shadow: 0 0 0 1px rgb(217 164 65 / .28), 0 1px 3px rgb(217 164 65 / .35); }
.arena-rank-chip.is-rank-2 { color: #1C2027; background: linear-gradient(160deg,#DDE4EC,#AFBAC8); border-color: #C7D0DC; }
.arena-rank-chip.is-rank-3 { color: #2A1A0E; background: linear-gradient(160deg,#DCA070,#B87333); border-color: #C8834A; }
```

要点：

- **金/银/铜**是通用心智，不需要额外图例
- 三档都用**深色前景 + 亮底**，和 4~7 名的「灰字 + 透明底 + 细边框」形成明确的实心/空心对比
- 第 1 名额外给一圈极淡的 `box-shadow` 光晕，在一堆小徽章里能一眼定位；2/3 名不给，避免喧宾夺主
- **尺寸严格保持 18×18**，否则会破坏第 1 条刚算好的高度账（`.arena-team-row-compact` 的
  `grid-template-columns: 18px minmax(0,1fr)` 也是按这个宽度写的，`gameplay.css:392`）
- 不要用 emoji（🥇🥈🥉）：字体渲染不可控、宽度不稳定，会撑破 18px 格子

`.arena-team-row-compact.is-first`（`:393`）现在用 `var(--primary)` 画左侧内阴影，
建议同步改成固定金色 `#D9A441`，和徽章呼应；或者干脆去掉，让金色徽章独自承担「第一名」的标记，
避免同一行出现两套强调。这条看你偏好。

---

## 4. 两条链路都要覆盖

用户明确要求「英雄里面的斗魂战绩和总览里面斗魂的战绩都要按上面的要求改」。

**好消息**：第 1 条和第 3 条是**共用代码**，改一次两边同时生效——
`renderMatchPlayers()`（`web/gameplay.js:1091`）被 `renderMatch` 在 `:1229` 唯一调用，
英雄页通过 `mountExternalMatchCards` 复用的也是同一个 `renderMatch`。

**需要注意的只有第 2 条**：窄屏隐藏规则分散在**两个具名容器**里，必须两边都改：

| 容器 | 声明处 | 服务的页面 |
|---|---|---|
| `matches-column` | `web/gameplay.css:301` | 总览页战绩栏 |
| `arena-first` | `web/champions.css:589` | 英雄页「吃鸡战绩」区块 |

漏改任何一个，就会出现「总览页对了、英雄页还是丑」或反之。

---

## 5. 测试同步

现有测试里有几条会被这轮改动影响，需要一并更新：

```js
web/champions.test.cjs:225  assert.match(gameplayScript, /grouping\.groups\.map\(\(group\) => \{[\s\S]+arena-team-row-compact/);
```
恢复 `slice(0, 4)` 后这条正则不再匹配，要改成
`/grouping\.groups\.slice\(0, 4\)\.map\(/`——**顺便这条改完就成了防回归护栏**，
锁死「卡片正面最多 4 队」这个高度约定。

```js
web/champions.test.cjs:281  assert.match(gameplayStyles, /\.arena-rank-chip\s*\{[^}]*width:\s*18px[^}]*height:\s*18px/);
```
这条要保留（它正好锁住第 3 条里「尺寸必须保持 18px」的约束）。

**建议新增两条**：

1. 断言 `.match-players.is-arena` 存在 `max-height`（第 1 条的 CSS 兜底）
2. 断言 `matches-column` 和 `arena-first` 两个容器的窄屏档里都有
   `.match-players.is-arena` 的 `display: none`——这条能防住「只改了一边」

按 [[deep-legends-mutation-testing]] 的规矩，正则型断言提交前先做变异验证
（故意把 `slice(0, 4)` 改成 `slice(0, 5)`、把某一档的 `display:none` 删掉，确认测试真的会红）。

---

## 附：本轮不涉及但确认无恙的

- 搭档协同板块（截图一）用户确认「还行」，`.arena-team-faces { min-width: 96px }`
  （`web/champions.css:151`）的挤压风险在实机上没有暴露，**暂不处理**。
