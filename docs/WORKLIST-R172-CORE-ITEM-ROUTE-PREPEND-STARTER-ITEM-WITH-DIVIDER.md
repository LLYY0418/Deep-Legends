# WORKLIST-R172：装备推荐"核心装"完整装备列表前面加出门装，用竖线隔开

诊断人：Claude（只读代码调研，未做实机操作）。
执行人：GPT。
日期：2026-09-27。
来源：用户要求——出门装也要展示出来，放在现在的完整装备列表第一个装备前面，出门装算这条装备序列的一部分，但要用一条竖线明显地分割开，跟后面几件装备用箭头连接区分开。

---

## 现状

`backend/web/gameplay.js` 的 `renderBuildRecommendation`（约 6302-6360 行）里，出门装（`starterOptions`，来自 `build.starterOptions`）和"核心装"（`coreOptions`）现在是两块完全独立的区域：

- 出门装：在顶部的 `build-summary-bar` 里单独一节，标题"出门装"（`backend/web/gameplay.js:6329`：`starterOptions.length ? \`<section class="build-summary-starter"><h3>出门装</h3>${optionList(starterOptions, "item", ...)}</section>\` : ""`）。
- 核心装：在下面的 `itemBuildLayout` 里，走的是 `optionList(coreOptions, "route", "暂无核心装备样本", renderCoreStats)`（`backend/web/gameplay.js:6358`），每一条 `coreOptions` 记录会被 `renderConfigOption`（`backend/web/gameplay.js:6457` 起）渲染成一串用"›"箭头连起来的装备图标链（`kind === "route"` 分支：`ids.length > 1` 时用 `route-step`/`route-arrow` 连接）。这就是用户说的"完整装备列表"——一条从第一件到最后一件的装备路线图。

用户想要的：出门装不再只是顶部单独一节，还要**额外**出现在这条"核心装路线"的最前面，视觉上算它的一部分（挨着，不是分开的两个卡片），但用一条竖线跟后面的箭头链区分开——竖线要比箭头更明显，让人一眼看出"这一段是出门装，后面才是正式核心装路线"。

---

## 改动方向

### 1. `renderConfigOption`（`backend/web/gameplay.js:6457`）

给这个函数加一个可选参数，比如 `leadingIds`（出门装的装备 ID 数组），只在 `kind === "route"` 时生效：

```js
function renderConfigOption(option, kind, statsRenderer = null, leadingIds = []) {
  const ids = option.ids || [];
  const leading = kind === "route" && leadingIds.length
    ? leadingIds.map((id) => `<span class="config-item">${renderItemIcon(id)}</span>`).join("")
      + `<span class="route-divider" aria-hidden="true"></span>`
    : "";
  const icons = leading + ids.map((id, index) => { ... 原逻辑不变 ... }).join("");
  ...
}
```

要点：
- `route-divider` 是新加的一个竖线分隔符（不是 `route-arrow`），CSS 上要跟现有 `.route-arrow`（`backend/web/gameplay.css:1501`：`color: var(--muted); font-size: 18px;`）明显区分开——比如用更粗、更不透明的竖线（`│` 或者一个 `background` 竖条），不要做成跟箭头差不多颜色深浅的样子，用户明确要求"明显一点"。
- 只有 `kind === "route"` 时才可能有 `leadingIds`；`kind === "item"`（出门装自己那一节）、`kind === "spell"` 不受影响，调用处不传这个参数就是原来的行为，向下兼容。

### 2. `renderBuildRecommendation`（`backend/web/gameplay.js:6302` 起）里调用核心装那一行

`backend/web/gameplay.js:6358`：
```js
: `<div class="item-build-layout"><div class="build-item-row" data-depth-count="${depthGroups.length}"><section class="item-core-column"><h3>核心装</h3>${optionList(coreOptions, "route", "暂无核心装备样本", renderCoreStats)}</section>...`
```
`optionList` 这个内部函数（约 6317 行定义）目前签名是 `(options, kind, emptyCopy, statsRenderer)`，也要加一个可选的 `leadingIds` 透传给 `renderConfigOption`。取哪一组出门装 ID：用 `starterOptions` 里排第一的那条（上游已经按胜率/使用率排好序，跟现有"出门装"小节展示的是同一条——即 `starterOptions[0]?.ids || []`），每一条 `coreOptions` 路线前面都统一接上这同一组出门装图标，不用给每条核心装路线单独配不同出门装（出门装本来就跟具体核心装路线无关，是每个人打法开局都会买的）。
- 没有出门装数据（`starterOptions` 为空）时，`leadingIds` 传空数组或不传，`renderConfigOption`/`optionList` 走原来的样子，不出现空竖线。
- 这只应用在经典模式（`isArenaBuild` 为假、走 `optionList(coreOptions, "route", ...)` 的那个分支，`backend/web/gameplay.js:6358`）。斗魂竞技场/海克斯大乱斗那条 `arenaOptionList`/`renderArenaBuildOption`（`6357` 行）分支不用改——这些模式本身出门装概念不一样（斗魂开局装备是系统给的，不是玩家买的"出门装"），不要照搬过去。

### 3. 顶部"出门装"小节保留

用户说的是"也展示出来"，不是"搬过去"——顶部 `build-summary-bar` 里独立的"出门装"小节（`backend/web/gameplay.js:6329`）继续保留，这次是在核心装路线前面**额外**再展示一次，两处都在，不用互斥。

---

## 测试要求

- 前端 Node 测试（`.test.cjs`，找现有覆盖 `renderBuildRecommendation`/`renderConfigOption` 的用例照着写）：
  - 给定 `starterOptions` 非空、`coreOptions` 非空，渲染出的核心装第一条路线 HTML 里，出门装图标排在最前面，紧跟一个 `route-divider`，之后才是原来的核心装图标序列（用 DOM 结构或字符串顺序断言，不要只断言"包含"）。
  - `starterOptions` 为空时，核心装路线渲染结果跟改动前完全一致（没有多余的空 `route-divider` 或空图标格子）——这是兼容性回归项，得单独写一条断言。
  - 顶部 `build-summary-starter` 小节在两种情况下都还在（不会因为这次改动被顶掉）。
  - 斗魂/海克斯大乱斗分支（`isArenaBuild` 为真）的渲染结果不受这次改动影响。

---

## 验收标准

1. 有出门装数据时，"核心装"那条完整装备路线最前面能看到出门装图标，紧接一条明显的竖线（跟后面装备之间的箭头能一眼区分开），再往后还是原来的箭头连接装备链。
2. 没有出门装数据时，核心装路线跟现在完全一样，不出现空竖线或空图标格。
3. 顶部原有的"出门装"小节继续存在，不受影响。
4. 斗魂竞技场/海克斯大乱斗模式的装备展示不受这次改动影响。

---

*本工单由 Claude 基于代码只读调研完成，未做任何实机操作，未修改任何源码。*
