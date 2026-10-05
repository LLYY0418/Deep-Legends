# WORKLIST-R215：R213 复核——整块重建后行快照记录的是挪入图片后的 HTML，下一次任何变化都会替换全部玩家行

复核：Claude（只读核对 R213 改动 + 在本机 Node 下跑定向用例）。执行：GPT。日期：2026-10-05。
基线：0.12.71 工作区（含未提交的 R211～R214 改动）。不构建、不发布，和 R211～R214 合并进下一个版本。

---

## R213 复核结论

- P1、P2、P3 都按工单实现：同局后台刷新时忽略进度快照（方案 a）；整块重建先 `preserveLiveImages` 再 `replaceChildren`；`full_reasons`、`sources.progress`、`live_progress_apply` 前后端都已登记。
- `node --test backend/web/*.test.cjs` 本机重跑：**915/915 通过**。R213/R204/R129 定向 25/25 通过。
- Go：本机没有 Go 环境，没能重跑；看了 `features.go` 的差异和 `r213_test.go`，白名单与限幅符合工单。账本里的 Go 全量日志为 PASS。
- 发现 1 个由 R213 P2 引入的问题，见下文 P1。真机录屏验收仍待构建后进行。

---

## P1　整块重建后，行快照包含已加载图片的属性，导致下一次变化替换全部行

### 复现（本机 Node + JSDOM，用 r213.test.cjs 的同一套夹具）

| 场景 | 操作 | `rows_replaced` |
|---|---|---|
| 对照 | 4 名玩家渲染 → 图片加载完成（img 写入 `src`、`data-image-ready`）→ 只改第 1 名玩家的段位 | **1** |
| R213 后 | 同上，但中间发生一次整块重建（多出一个页签）→ 只改第 1 名玩家的段位 | **4** |

`imagesRecreated` 两种情况都是 0，所以头像不会闪；但整块重建之后，下一次任何阵容变化都会把所有玩家行整行替换一遍（10 人就是 10 行）。组队标签、段位、悬停提示等节点都会重建：鼠标正悬停在组队标签上时，提示会断掉。`rows_replaced` 也会虚高，影响 R213 真机验收时对日志的判读。

### 原因

`renderLive` 整块重建路径（`backend/web/gameplay.js` 约 5851～5858）：

1. `preserveLiveImages(nodes.liveContent, fullHolder, stats)` 把旧的 img 挪进新行。这些 img 已经被图片队列改过属性（`src`、`data-image-ready` 等）。
2. 然后才 `stampLiveRows(nodes.liveContent)`，`row._liveMarkup = row.outerHTML` 记下的是含已加载图片属性的 HTML。
3. 下一次 `patchLiveRosterPanel` 用 `old._liveMarkup !== fresh.outerHTML` 判断是否替换。新 markup 里的 img 只有 `data-queued-src`，所以每一行都判定为变化。

R213 之前，整块重建是先 `innerHTML` 再立刻 stamp，记录的是干净 markup，没有这个问题。`patchLiveRosterPanel` 的逐行路径是先给 fresh 行记 `_liveMarkup` 再挪图片，本来就是对的。

`updateLivePanels` 里非 insight 面板（符文 `.specialist-game-row[data-rune-choice]`）和对线卡槽，也是先 `preserveLiveImages` 再 `stampLiveRows`，属于同一个模式（R213 之前就存在）：新符文行的 `_liveMarkup` 带着已加载图片属性，下一次复用判断永远不成立。一起修。

### 修复要求

1. 整块重建：`fullHolder.innerHTML = markup` 之后、`preserveLiveImages` 之前，先调用 `stampLiveRows(fullHolder)`，让每一行记下干净的 markup；挪图片和 `replaceChildren` 之后不要再覆盖行的 `_liveMarkup`（`stampLiveRows` 只在未记录时写入，保持即可）。
2. `updateLivePanels` 非 insight 分支和对线卡槽分支：同样在 `preserveLiveImages` 之前对 `next` / `nextSlot` 调用 `stampLiveRows`。
3. 不改变 R213 已有行为：同 src 图片仍复用同一个节点，`imagesRecreated` 仍为 0。

---

## 测试

在 `backend/web/r213.test.cjs`（或新建 `r215.test.cjs`）里加：

1. **整块重建后单行变化**：渲染 → 把所有 img 设为已加载（写 `src` 和 `data-image-ready`）→ 触发整块重建（多出页签）→ 再把新挪入的 img 设为已加载 → 只改 1 名玩家的段位：`rowsReplaced` 必须是 1，其余行节点 `===` 重建后的同一节点。
2. **对照**：不经过整块重建，同样的单行变化 `rowsReplaced=1`（现在已经成立，防回退）。
3. **符文行复用**：符文面板渲染 → 图片已加载 → 符文面板 markup 变化但某一行内容不变：这一行仍是同一节点。
4. 现有测试的夹具里 `prepareImages` 是空函数，图片属性从不变化，所以抓不到这类问题。新用例必须手动模拟图片加载后的属性变化。
5. 全量 `node --test backend/web/*.test.cjs` 与 `go test ./backend` 结果写进账本。

---

## 真机验收（与 R213 合并进行）

构建后与 R213 一起验收：选人阶段本人换一次英雄（会触发整块重建），之后另一名队友读完战绩或换英雄时，`live_render_rebuild` 里这一分钟的 `rows_replaced` 应只等于真正变化的行数，而不是全部 10 行。
