# WORKLIST-R140：R138 的旗帜部分（§3 列表放大、§4 详情放大）没有执行，却被记成已关闭

执行勘误（2026-09-23）：下文第 0 节的前提与仓库中实际交付的 R138 工单不符。实际 R138 只有头像 §1/§2，且明确要求旗帜视图不动；R138 台账也记录了旗帜保持原状。旗帜改动以本 R140 工单作为新授权执行，取舍和验证见 [r140-execution-ledger.md](r140-execution-ledger.md)。保留原诊断文字以便追溯，不再把它当作 R138 遗漏的事实依据。

诊断人：Claude（独立复核 R138/R139 执行台账时发现，只读检查，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：HEAD `673bc82b`（"Complete R138 facade polish and R139 broadcast settings"）。
状态：已执行；范围按上方勘误与执行台账核对。

## 0. 证据：R138 只做了头像部分，旗帜部分被跳过且未声明

`docs/WORKLIST-R138-FACADE-ICON-SPACING-AND-DETAIL-DIALOG-SIZE.md` 原文有四节：§1 头像列表间距、§2 头像详情放大、§3 旗帜列表放大到一行 4–5 个、§4 旗帜详情放大。验收总表明确把"旗帜列表放大"和"旗帜详情放大"列为独立判据。

`docs/r138-execution-ledger.md` 只写了"头像列表与详情"一节，通篇没有一个"旗帜"字，也没有任何一句说明"为什么旗帜部分不做"或"旗帜部分留到下一轮"。`docs/WORKLIST-INDEX.md:63` 把 R138 的标题记成"头像间距与详情弹窗放大；总览战绩详情当前玩家强调"——连标题里都不提旗帜了，直接把这部分从记录里抹掉，然后状态标了"已关闭"。

代码层面逐一核对，确认旗帜部分确实一行没改：

- `backend/web/app.css:1150`：`.facade-grid.is-banners` 仍是 `grid-template-columns: repeat(auto-fill, 150px); ...gap: 12px 10px;`，跟 R138 之前完全一样，没有变成 4–5 列的宽度，间距也没有跟着头像一起加大。
- `backend/web/favorites-facade.js:281,297`：`DETAIL_ART_BANNER_MAX_HEIGHT_PX` 仍是 `320`，`applyDetailArtSize` 的旗帜分支仍是 `Math.min(DETAIL_ART_BANNER_MAX_HEIGHT_PX, height)`，没有 `×2`。
- `desktop/r138-facade-layout.cjs` 第 1 行注释直接写"real Chromium comparison of **icon** spacing and the detail image size"——这份新增的真机布局脚本从一开始就只覆盖头像，没有旗帜列数的测量或断言。
- 更值得注意的是：这一轮新增的 `backend/web/r130.test.cjs:796-813`（"R138 头像详情按原图 2 倍显示、最大 512px，旗帜详情保留 R130 尺寸"）里**明确写了一句"旗帜详情保留 R130 尺寸"**，并且断言 `size(100, 400)` 仍然等于 `['80px', '320px']`（R130 的旧值，未放大）。也就是说这不是"漏做了忘记提"，而是**执行时主动决定跳过旗帜、并且专门写了一条测试把"旗帜不变"这件事锁死**，但没有在台账里说明这是有意跳过、也没有回头改工单或另开工单说明，直接把整单标成已关闭交差。

R139 单独核对过，逻辑、样式、测试、变异全部对得上工单，是合格的；这份工单只针对 R138 被跳过的部分。

## 1. 旗帜列表：格子放大到一行 4–5 个，间距同步加大

内容与原 R138 §3 完全一致，原样搬过来，因为从未执行：

1. **不要凭猜测定一个固定像素值**，在真机 Chromium 里量出来：
   - 用 `desktop/r138-facade-layout.cjs` 已经搭好的 fixture 和量法（`measure()` 函数），在 1200px 和 960px 两个宽度下，测出收藏页内容区（去掉侧边栏和内边距后）实际可用宽度。
   - 在这个可用宽度下，选一个列宽（`.facade-grid.is-banners` 的 `grid-template-columns: repeat(auto-fill, Npx)`），让**一行正好显示 4–5 列**，两个宽度都要落在 4–5 区间（不要求两个宽度列数相同）。
   - 列宽定下来后，`.skin-art` 的高度仍然由 `aspect-ratio: var(--facade-banner-ratio)` 自动算，格子变宽后卡片会明显变高，这是预期效果；同步把 `.facade-grid.is-banners .skin-card` 的 `contain-intrinsic-size`（现在是 `auto 524px`）改成新格子对应的实际高度。
2. **间距同步加大**：把 `.facade-grid.is-banners` 的 `gap` 从 `12px 10px` 改成不小于 `20px 16px`（跟头像那次改的下限一致），具体数值和列宽一起在真机截图里调。
3. 头像视图（`.facade-grid.is-icons`）这次不动，已经在 R138 里改完。

### 验收

- 扩展 `desktop/r138-facade-layout.cjs`（或新建一份专测旗帜的），在 1200px 和 960px 两个宽度下，量出旗帜网格实际渲染的列数，断言都在 4–5 之间（含 4 和 5）。截图存到 `docs/r140-validation/`。
- CSS 断言：`.facade-grid.is-banners` 的 `grid-template-columns` 数值比 `150px` 明显更大；`gap` 两个分量都 ≥ 18px/14px。
- 变异：把列宽改回 `150px`，列数断言必须 FAIL（列数会变回 6 列以上）。
- 用真实旗帜图片过一遍（非 0.3 兜底比例），确认没有裁剪、没有变形；未拥有旗帜的置灰+锁图标在新尺寸下比例正常。

## 2. 旗帜详情弹窗：放大（同头像一样，允许模糊）

内容与原 R138 §4 完全一致：

1. 先量旗帜原图真实的 `naturalWidth/naturalHeight`（真机或真实旗帜图片样本），写进本轮台账——旗帜原图分辨率跟头像的 128px 不一样，未知，不能照抄头像那份直接乘 2 而不核实。
2. `backend/web/favorites-facade.js:281,297` 加一个 `DETAIL_ART_BANNER_SCALE = 2`，把 `Math.min(DETAIL_ART_BANNER_MAX_HEIGHT_PX, height)` 改成 `Math.min(DETAIL_ART_BANNER_MAX_HEIGHT_PX * 2, height * 2)`，`DETAIL_ART_BANNER_MAX_HEIGHT_PX` 从 `320` 提到 `640`；宽度仍按 `scaled * (width / height)` 跟着比例走。
3. 弹窗外壳 `.facade-detail-dialog` 已经是 `fit-content` 自适应，不需要额外改。

### 验收

- **改掉 `backend/web/r130.test.cjs:812-813` 那两条"旗帜详情保留 R130 尺寸"的断言**——这两条断言现在测的正是要改掉的旧行为：`size(100, 400)` 应该变成 `['160px', '640px']`（原来 `['80px', '320px']` 乘 2），`size(100, 200)` 应该变成 `['200px', '400px']`（原来 `['100px', '200px']` 乘 2，且没超过新上限 640 所以不封顶）。测试名字和第 796 行的注释"旗帜详情保留 R130 尺寸"也要跟着改，不能留着一句和新行为矛盾的话。
- 新增边界用例：给一张高度远超过 320px（旧上限）但没超过新上限 640px 的旗帜图，确认没有被旧上限误伤；给一张远超 640px 的，确认封顶在新上限。
- 真 Chromium 截图：打开任意已拥有旗帜的详情，图片区域比修改前明显更大。
- 变异：把 `DETAIL_ART_BANNER_SCALE` 改回 `1`（或删掉），上面的尺寸断言必须 FAIL。

## 3. 台账要求（这次必须写清楚，不能再悄悄跳过）

本轮台账里必须包含一句明确说明：R138 当时为什么只做了头像、跳过了旗帜（如果是时间/优先级原因就直说，不用editorialize），以及 `docs/WORKLIST-INDEX.md` 里 R138 那一行要不要连带更正标题（把"旗帜"部分的完成状态如实反映到 R140 这一行，R138 本身已关闭不再改）。

## 验收总表

| 项 | 判据 |
|---|---|
| 旗帜列表放大 | 1200px/960px 两个宽度下列数都落在 4–5 列；间距不小于 18px/14px；变异（列宽改回 150px）FAIL |
| 旗帜详情放大 | 台账记录真实原图分辨率；弹窗高度按 2 倍显示，上限 640px；`r130.test.cjs:812-813` 更新为新数值；变异（倍数改回 1）FAIL |
| 全量 | `node --test backend/web/*.test.cjs desktop/*.test.cjs` 全绿；`go vet ./...`、`go test ./...` 不受影响（本单不碰 Go 代码）；r100/r117 Chromium 护栏通过 |

本单不涉及产品版本号递增或重新打包安装包，可以和其它未打包的前端改动一起等下次合并发版。
