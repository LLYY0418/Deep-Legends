# WORKLIST-R125：收藏页「头像」视图卡片过大——按客户端原始尺寸展示，不放大

诊断人：Claude（只读：看截图 + 读 `app.css` / `favorites-facade.js`，未改仓库代码）。
执行人：GPT。
日期：2026-09-22。
基线：R123/R124 之后的 `backend/web/favorites-facade.js`、`backend/web/app.css`（仍未提交）。
状态：待执行。只动头像视图；旗帜视图、皮肤/炫彩卡片不动。

---

## 0. 结论

头像视图直接套了皮肤卡的尺寸：`.skin-grid` 用 `--card-min: 288px`（`app.css:58`），`.skin-art` 是 `aspect-ratio: 16/9` + `object-fit: cover`（`app.css:555-557`）。R123 只给旗帜视图加了覆盖（`.facade-grid.is-banners`），头像视图没有。结果是一张 128px 左右的原图被拉到 600px 宽，还被 16:9 裁掉上下两截，截图一里每屏只能看到 6 个头像。

客户端（截图二）的做法是正方形小格子，约 100px，原样显示不放大，一屏能看到二十多个。按这个改。

## 1. 要改的地方

### 1.1 CSS（`app.css` R123 那段后面追加，只作用于头像视图）

`render()` 里给网格加一个 `is-icons` 类（和现有的 `is-banners` 对应，二者互斥），然后：

```css
.facade-grid.is-icons { grid-template-columns: repeat(auto-fill, 104px); justify-content: start; gap: 14px 12px; }
.facade-grid.is-icons .skin-card { contain-intrinsic-size: auto 136px; }
.facade-grid.is-icons .skin-art { width: 104px; aspect-ratio: 1/1; border-radius: 8px; }
.facade-grid.is-icons .skin-art img { object-fit: contain; }          /* 不裁剪 */
.facade-grid.is-icons .skin-lock { width: 30px; height: 30px; }
.facade-grid.is-icons .skin-lock svg { width: 15px; height: 15px; }
.facade-grid.is-icons .skin-state { font-size: 10.5px; padding: 1px 6px; top: 4px; right: 4px; }
.facade-grid.is-icons .skin-copy { position: static; padding: 6px 0 0; background: none; }
.facade-grid.is-icons .skin-copy strong { font-size: 12px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.facade-grid.is-icons .skin-hero, .facade-grid.is-icons .skin-meta { display: none; }
```

要点（执行时以这些意图为准，具体数值可以微调）：

1. **格子固定 104px，不随窗口拉伸**：用 `repeat(auto-fill, 104px)`，不要用 `minmax(..., 1fr)`，否则宽窗口下又会被撑大。104px 在 2 倍屏上对应约 208 物理像素，和客户端观感一致，也不会超过原图太多。
2. **正方形 + `object-fit: contain`**：头像不能被裁。
3. **文字移到图下方、只留名称一行**：原来叠在图上的「名称 / 系列·年份 / ID」三行在 104px 格子里放不下。系列、年份、ID 已经在点开后的只读详情里有，格子上只保留名称（省略号截断），`title` 属性放完整名称，悬停可见。
4. **锁、未拥有角标等比缩小**：48px 的锁在 104px 格子里太大。
5. 暗化/灰度（`.is-locked`）逻辑不变。

### 1.2 JS（`favorites-facade.js`）

- `render()`：`el.grid.classList.toggle("is-icons", state.view === "icons")`，和 `is-banners` 一起切换。
- `createCard()`：头像视图给卡片 `title` 设完整名称（`card.title = fields.title`）。
- 5,099 张卡一次性渲染的成本：卡片变小后同屏数量会从 6 个涨到 40~60 个，图片排队加载会更明显（排队机制见 R127）。现有 `.skin-grid > .skin-card { content-visibility: auto }` 继续生效即可，不需要额外做分页。

## 2. 验收

- 1240px 宽窗口下，头像视图一行不少于 9 个，格子是正方形，图片没有被裁（边框、角落的装饰完整）。
- 把窗口拉到 1900px，格子尺寸不变，只是每行变多。
- 旗帜视图、皮肤与炫彩页的卡片尺寸与改动前逐像素一致（截图对比即可）。
- 未拥有头像仍然是灰度 + 小锁 + 「未拥有」角标；拥有状态不可用时不显示锁（R124 的降级不回退）。
- `node --test backend/web/r123.test.cjs` 仍然全绿；补一条断言：`app.css` 里存在 `.facade-grid.is-icons` 且其 `.skin-art` 为 `aspect-ratio: 1/1`，`favorites-facade.js` 在 `render()` 里切换 `is-icons`。

## 3. 不做的事

- 不改旗帜视图（用户没有提意见，现在 168px 正方形 + contain 是对的）。
- 不引入客户端那种按年份分组（「已获取 2026」）的布局；现有排序 + 「近三年新增」快捷分类已经够用。需要的话另开工单。
