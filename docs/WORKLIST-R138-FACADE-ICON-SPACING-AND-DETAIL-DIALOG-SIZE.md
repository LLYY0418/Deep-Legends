# WORKLIST-R138：收藏页「头像」列表太挤 / 详情弹窗太小

诊断人：Claude（用户反馈截图 + 读 `app.css` / `favorites-facade.js`，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：R137 之后，HEAD `cd44bcd9`。
状态：已完成（见 `r138-execution-ledger.md`）。只动「头像」视图（`state.view === "icons"`）的网格间距和详情弹窗图片尺寸；「旗帜」视图、皮肤/炫彩卡片、其它页面不动。

## 0. 背景与用户决定

用户截图反馈：头像列表格子挨得太挤；点开单个头像的详情弹窗（示例：「鹰啸！图标」）太小。

诊断时发现详情弹窗的图片尺寸是 R130 P6 特意做的设计：`favorites-facade.js:275-296` 按图片自身 `naturalWidth/naturalHeight` 显示、**绝不放大**，因为放大会发糊（R130 之前弹窗固定 560px、把约 128px 的原图拉伸 4 倍多，发糊问题就是那次修的）。就此向用户说明了这个取舍，用户明确决定：

- **详情弹窗**：接受发糊，明确要求放大（不是"能放多大放多大"，是"就算糊也要放大"）。
- **头像列表**：不追求把格子拉大导致画质问题，而是**把格子间距加大**，解决拥挤的观感。

本工单按这个决定拆成两部分，互不影响，可分别验收。

## 1. 头像列表：格子间距加大

### 现状

`backend/web/app.css:1161`：

```css
.facade-grid.is-icons { grid-template-columns: repeat(auto-fill, 92px); justify-content: start; gap: 12px 10px; }
```

格子固定 92px 不变（这是 R125 定的、不放大避免模糊的尺寸，继续保留），但行列间距只有 12px/10px，视觉上格子紧挨在一起。

### 修复

把 `gap` 从 `12px 10px` 改成 `20px 16px`（行间距 20px、列间距 16px；具体数值可按 GPT 实际渲染截图微调，但不得小于 18px/14px，否则和现在比不出明显差别）。**只改这一条规则的 `gap`**，`grid-template-columns` 的 `92px` 不动，`.skin-card`／`.skin-art` 的尺寸不动。

### 验收

- `backend/web/r123.test.cjs:151` 的 `92px` 断言必须继续通过（格子尺寸没变）。
- 新增/更新一条 CSS 断言：`.facade-grid.is-icons` 的 `gap` 不再是 `12px 10px`，且新值的两个分量都 ≥ 现有值（不能改小）。
- 真 Chromium 截图（1200px 和 960px 各一张，复用 `desktop/r136-live-gap-layout.cjs` 或 `current-game-layout.cjs` 的写法即可）：相邻两个头像格子之间的可见间隙比修改前明显变大，肉眼可辨。
- 变异：把 `gap` 改回 `12px 10px`，上面新增的断言必须 FAIL。
- 旗帜视图 `.facade-grid.is-banners`（`app.css:1150`）的 `gap: 12px 10px` **不动**——用户只反馈了头像列表，不要顺手改旗帜。

## 2. 详情弹窗：头像图片放大一倍（允许模糊）

### 现状

`backend/web/favorites-facade.js:279`：

```js
const DETAIL_ART_ICON_MAX_PX = 256;
```

`favorites-facade.js:285-294`（`applyDetailArtSize`）：头像视图下 `size = Math.min(DETAIL_ART_ICON_MAX_PX, Math.max(width, height))`——原图多大就显示多大，256px 只是给异常大图片兜底的上限，对约 128px 的常规头像图完全不生效（`min(256,128)=128`），所以现在看到的就是原图原尺寸。

### 修复

头像视图的尺寸计算改成**在原图基础上乘以 2**，仍然设一个上限防止极端情况把弹窗撑爆：

```js
const DETAIL_ART_ICON_SCALE = 2;
const DETAIL_ART_ICON_MAX_PX = 512; // 原 256 上限翻倍，跟着放大倍数走
...
if (state.view === "icons") {
  const nativeSize = Math.max(width, height);
  const size = Math.min(DETAIL_ART_ICON_MAX_PX, nativeSize * DETAIL_ART_ICON_SCALE);
  setDetailArtSize(size, size);
  return;
}
```

`img` 元素本身不用换更高分辨率的图源（客户端接口 `/lol-game-data/assets/v1/profile-icons/{id}.jpg` 就这一种），`object-fit: contain` 配合 CSS 尺寸放大即可，浏览器会做插值，允许发糊，不需要额外处理。

**旗帜视图不动**：`DETAIL_ART_BANNER_MAX_HEIGHT_PX`（`favorites-facade.js:280`）和它对应的分支（`favorites-facade.js:296-298`）保持原样，用户没有反馈旗帜详情图的问题。

加载前占位值 `128px`（`app.css:1158` 的 `var(--facade-art-width, 128px)`，以及 `favorites-facade.js` 里图片 `onload` 之前的初始状态）可以保留不动——只是加载完成前的兜底占位，不影响加载完成后的最终尺寸。

### 验收

- 单元测试：给 `applyDetailArtSize` 一张 `naturalWidth=naturalHeight=128` 的头像图，断言 `--facade-art-width` / `--facade-art-height` 变成 `256px`（不是 128px）。
- 边界：给一张 `naturalWidth=naturalHeight=300` 的头像图（罕见的大图），断言尺寸被封顶在 `512px`，不是 `600px`。
- `backend/web/r130.test.cjs:819` 原本断言"128×128 原图 → 弹窗 128px"，这条要改成断言"→ 256px"，并在测试里写清楚原因（引用本工单，说明这是用户明确接受发糊后的决定，不是回退 R130 的反发糊设计）。
- 真 Chromium 截图：打开任意头像详情，图片区域比修改前明显更大。
- 变异：把 `DETAIL_ART_ICON_SCALE` 改回 `1`（或删除这个乘数），上面两条尺寸断言必须 FAIL。

## 3. 不做的事

- 不改旗帜视图的网格间距、格子尺寸或详情图尺寸。
- 不改皮肤/炫彩卡片（`.skin-grid`）的任何样式。
- 不引入新的图片源或更高分辨率资源——客户端本身只提供这一种头像图片，放大后的模糊是用户已知并接受的结果，不算缺陷，测试里不要写"图片清晰"之类的断言。
- 不改 `.facade-grid.is-icons .skin-art` 的 92px 格子宽度（用户这次要的是间距，不是格子本身变大）。

## 验收总表

| 项 | 判据 |
|---|---|
| 头像列表间距 | `gap` 两个分量都变大且不小于 18px/14px；92px 格子宽度不变；Chromium 截图对比间隙变大；变异（改回原 gap）FAIL |
| 详情弹窗放大 | 头像详情图按原图 2 倍显示（128→256px 一类），有上限（512px）；旗帜详情不受影响；`r130.test.cjs:819` 更新并说明原因；变异（把倍数改回 1）FAIL |
| 全量 | `node --test backend/web/*.test.cjs desktop/*.test.cjs` 全绿（含本单新增/修改的用例）；`go vet ./...`、`go test ./...` 不受影响（本单不碰 Go 代码）；r100/r117 Chromium 护栏通过 |

本单不涉及产品版本号递增或重新打包安装包（纯前端 CSS/JS 改动，下次和其它工单一起打包发版即可，不需要单独出一版）。
