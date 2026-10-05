# WORKLIST-R209：收藏「皮肤与炫彩」卡片原画一直显示"加载中"

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-04。基线：0.12.71 工作区（R206/R207/R208 之后）。与 R206/R207/R208 合并发布。

## 为什么单独开这一份

这一项原本作为 R207 P5 追加，但执行时用的 R207 是追加之前的版本（仓库里的 `docs/WORKLIST-R207-…` 没有 P5，账本也没有提到），所以没有执行。按规则单独放在 R209。

## 现象


用户截图：「皮肤与炫彩 → 已拥有」，列表文字（名称、英雄、品质、ID）都在，所有卡片的原画区域一直是"加载中"。

## 证据（日志 `lol-loot-diagnostics-1004-1719.jsonl`，0.12.68）

| 时间 (UTC) | 事件 |
|---|---|
| 09:17:17.7 | 进入收藏，`collection_refresh_request source=dirty_rescan`；`/api/skins` 157 ms 返回 |
| 09:17:17.9 / 09:17:20.4 | `collection_render_client reason=unchanged-suppressed kept_visible=true item_count=1183` |
| 09:17:34.4 | 切到「全部皮肤」，`updated`，1952 项 |
| 09:17:35.9 | 切回「已拥有」，`updated`，1183 项 |
| 09:17:35 → 09:19:11（导出日志） | 约 95 秒里**没有任何图片相关记录**：没有 `local_request_client endpoint=image`（失败才记），没有 `image_queue_slow`（慢图成功才记），也没有 `card_image_stalled` |

源码（0.12.68 `backend/web/app.js`）：卡片原画分两层排队：

- 第一层：卡片任务，全局 8 个名额；`IntersectionObserver` 发现卡片接近可见区域才入队；拿到名额后有 45 秒看门狗，超时一定记 `card_image_stalled`。
- 第二层：`image-queue.js`，10 秒超时，失败一定记 `local_request_client`。

95 秒内两层都没有任何记录，说明**卡片任务根本没有拿到第一层名额**（拿到了就会在 45 秒内出图、失败或报卡死）。只有两种可能：

1. 第一层的 `state.activeCardImages` 计数没有回落到 8 以下，即名额泄漏。之后所有卡片都排在队里不动，也就不会有任何看门狗。
2. `IntersectionObserver` 没有为这些卡片触发（`root: el.appScroll` 与卡片的实际滚动容器不一致，或观察被提前取消），卡片停在 `pending`，从未入队。

日志里没有第一层的计数和队列状态，无法确定是哪一种，两种都要处理。这次出现在长时间运行（多局对局、多次进出收藏）之后，名额泄漏的可能性更大。

## 修改

1. **诊断** `collection_card_image_state`：收藏页可见、并且有卡片停在 `pending` / `queued` 超过 5 秒时记一条（每 30 秒最多一条）。字段：
   - `active_count`（计数值）、`active_jobs`（实际 `job.active=true` 且图片仍在文档中的任务数）；
   - `queued`、`pending_observed`、`visible_pending`（在可见区域内但未入队的数量）；
   - `observer_root_ok`（卡片是否在 `el.appScroll` 内）；
   - `oldest_active_age_ms`。
2. **名额自愈**：泡队列时（`pumpCardImageQueue`）如果计数与实际活动任务数不一致，以实际数为准，并记 `card_image_slot_reconciled`（修正前后的值）。实际活动任务只统计 `job.active && !job.done && job.image.isConnected` 的任务；已经脱离文档的活动任务直接按取消处理并释放名额。
3. **找出泄漏点并修复**：逐一检查会替换或清空卡片容器的地方，确认在替换前都调用了 `cancelDeferredImages`：
   - `el.grid` 的骨架屏、"收藏信息还在准备中"、读取失败分支；
   - 炫彩卡片（1329 / 1363 行附近）所在的容器；
   - 奖池皮肤网格；
   - 页面切换；
   - R203 / R206 新加的保持可见刷新。

   没有调用的地方补上。账本列出检查表（位置、是否已取消、处理方式）。
4. **观察器兜底**：卡片渲染完成 2 秒后，如果可见区域内仍有卡片停在 `pending`，直接入队，不再等待观察器回调，并记 `card_image_observer_fallback`（数量）。卡片不在 `el.appScroll` 内时，观察器改用卡片实际所在的滚动容器。
5. 基于当前工作区实现（R206/R207/R208 之后的代码，0.12.71）。

## 测试（Node，jsdom）

1. **泄漏复现**：8 个活动任务的卡片被直接从 DOM 移除（不经过 `cancelDeferredImages`），再渲染新的一屏卡片 → 新卡片能拿到名额，并开始加载；`card_image_slot_reconciled` 记一次。
2. 依次执行以下操作，每一步之后计数都与实际活动任务数一致，最终全部卡片完成或进入"暂无预览"：
   - 进入收藏；
   - 保持可见刷新（`unchanged-suppressed`）；
   - 切换「全部皮肤」再切回「已拥有」；
   - 打开 / 关闭炫彩；
   - 离开再回到收藏。
3. **观察器不触发**：模拟 `IntersectionObserver` 不回调 → 2 秒后可见区域内的卡片直接入队，并记 `card_image_observer_fallback`。
4. 计数正常时不记 `card_image_slot_reconciled`。

变异（必须 FAIL）：去掉名额自愈 → 测试 1；去掉观察器兜底 → 测试 3。

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿；变异：去掉名额自愈 → 测试 1 FAIL，去掉观察器兜底 → 测试 3 FAIL。
- 账本 `docs/history/ledgers/r209-execution-ledger.md`，写明查到的泄漏点检查表；`docs/WORKLIST-INDEX.md` 加 R209。
- 版本沿用合并版本（不单独递增，除非已经发布）；构建和发布等用户恢复后与 R206/R207/R208 一起，按 R199 规则核对。

## 真机验收（用户）

1. 打完几局后进入收藏「皮肤与炫彩」，多切几次「已拥有 / 全部皮肤」：原画都能出来，不再一直"加载中"。
2. 导出日志：若出现 `card_image_slot_reconciled` 或 `card_image_observer_fallback`，说明命中了对应问题，日志里有修正前后的数值。
