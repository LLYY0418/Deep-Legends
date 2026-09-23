# R130 执行台账：皮肤列表过一会整屏「加载中」+ 头像/旗帜页面整理

版本 **0.12.18**（`desktop/package.json` 与 `desktop/package-lock.json` 两处均已递增；上一轮 R127 为 0.12.17）。
工单：`docs/WORKLIST-R130-COLLECTION-IMAGE-STALL-AND-FACADE-POLISH.md`（诊断人 Claude，执行人 GPT）。
执行范围：P1 全部 6 条修复方向 + 全部 4 条复现测试与变异、P2、P3、P4、P5（4 小条全部）、P6。**工单里没有跳过的条目。**
本轮结束时另做了一次独立的对抗式代码评审，评审提出的 1 个阻断级缺陷与 5 个应修缺陷已全部修掉并补了钉子，见第 10 节。

---

## 0. 结论

P1 的根因诊断在真机 Chromium 上被完整复现并被修复。用真实的 `image-queue.js` + 从真实 `app.js` 原样抽出的队列函数 + 真实挂起的 HTTP 连接（服务端只发响应头、永远不写 body）跑 8 张永不返回的图，三种构建各跑一遍：

| 构建 | 时刻 | 卡片名额 | 「加载中」 | 「暂无预览」 | 已出图 | stall 上报 |
|---|---|---|---|---|---|---|
| **生产**（两道防线都在） | 1.5s | 8 | 8 | 0 | 0 | 0 |
| | 60s | **0** | **0** | **8** | 0 | ≤1 |
| | 再加入 6 张正常图 +6s | 0 | 0 | 8 | **6** | ≤1 |
| **只留看门狗**（第二层不补发 error） | 60s | **0** | **0** | **8** | 0 | **≥1** |
| **变异体**（去掉合成 error + 关掉看门狗） | 45s | **8** | **8** | 0 | 0 | 0 |

变异体那一行就是用户截图的样子：名额 8/8、整屏「加载中」、再不恢复。两种修复体都把名额收回来了，之后新加入的正常图全部出图。整轮 `Runtime.exceptionThrown` 为 0。

证据：`docs/r130-validation/browser/result.json` 与 9 张截图（`p1-stall-fixed.png`、`p1-stall-watchdog-only.png`、`p1-stall-mutated.png`、`p2-banners.png`、`p3-p5-icons.png`、`p3-icons-with-unowned.png`、`p3-p5-degraded.png`、`p6-icon-detail.png`、`p6-banner-detail.png`）；脚本 `docs/r130-validation/r130-browser.cjs`（可重跑：`CHROME_BIN=… node docs/r130-validation/r130-browser.cjs`，约 3 分钟）。

---

## 1. P1　名额泄漏与两层可见区域打架

### A　第二层彻底放弃时补发合成 error（`backend/web/image-queue.js`）

`finish()` 里重试次数用尽（`count >= maxRetries`）时置 `exhausted = true`，在**移除自身 load/error 监听之后**、在 `data-queued-src` 复检之前执行 `img.dispatchEvent(new Event("error"))`，让调用方的 `onerror` 正常收尾（`app.js` 接着试下一个候选地址，`favorites-facade.js` 显示「无图」）。

补发有三道闸门，缺一不可：

1. `!cancelled`——取消走的是 `finish(false, true)`，补发会把已取消的卡片任务重新激活。
2. `wasConnected`——连接状态必须在 `prefetch.delete(img)` **之前**取好：`prefetch` 是 `deepLegendsQueueImage` 给脱离文档的图片保活用的，删掉之后 `connected(img)` 立刻变 false，补发会被自己的清理动作吞掉。
3. `img.getAttribute("data-queued-src") === url`——**只为当前这个 url 补发**。调用方（第一层看门狗）可能早就推进到下一个候选了，这时再补发会让调用方多推进一次、把本来能显示的那个地址直接跳过；下面既有的「URL 变了就重新入队」逻辑会接手新地址，不需要这个 error。这条是评审发现的阻断级缺陷，见第 10 节。

同一文件新导出 `window.deepLegendsImageSourceLabel = imageSourceLabel`，供第一层的 stall 上报复用同一份来源分类（见第 6 条）。

### A 兜底　第一层看门狗（`backend/web/app.js`）

`loadImageSources` 每设一个候选地址就挂一个 `CARD_IMAGE_STALL_MS` 的看门狗；超时即 `reportCardImageStall(...)` + `next()`（换下一个候选，全部试完显示「暂无预览」并释放名额）。

**时长偏离工单：工单写 25 秒，实现取 45 秒。** 第二层从放行到彻底放弃的最坏路径是 `10s 超时 + 10s 重试冷却 + 再 10s 超时 = 30s`（`image-queue.js` 的 `retryDelay = 10000, maxRetries = 1` 与 10s 定时器），工单里「~21s」的估算少了冷却那一段。而且这 30s 是从**第二层放行**起算，看门狗是从**第一层发名额**起算，中间还要排第二层自己的 5 个名额（第一层有 8 个）。取 25s 会让看门狗抢在第二层前面推进：既有概率跳过本来能显示的候选地址（就是第 10 节那个阻断缺陷的触发条件），也会在每张慢图上误报一条 `card_image_stalled`。45s 让它退回真正的兜底位置。

这条关系被钉成测试而不是写死常量：`backend/web/r130.test.cjs` 从 `image-queue.js` 里读出超时/冷却/重试次数算出最坏路径（断言恰好 30000ms），再断言 `CARD_IMAGE_STALL_MS > 30000`；harness 的默认时长也从 `app.js` 里解析（`PRODUCTION_STALL_MS`），不再自己写死一个副本——25s 那版之所以能溜过测试，正是因为 harness 里有一份和生产无关的默认值。

其它实现细节：

- 定时器句柄存在 `state.cardImageWatchdogs`（WeakMap，按 image 索引）。清理点共 6 处：`complete()`、`next()`、`deferCardImageSources()`、`cancelDeferredImages()`、`resetDialogImage()`、`closeSkinDialog()`。后两处是评审补的——`loadImageSources` 同时服务皮肤/炫彩详情弹窗，而弹窗节点一直留在文档里（`isConnected` 恒为 true），不清理的话看门狗会在 25/45 秒后醒来给一个已经关掉的弹窗重新写 `data-queued-src`。
- `image.onerror` 增加陈旧事件闸门：`if (index > 0 && sources[index - 1] !== image.getAttribute("data-queued-src")) return;`。
- `image.onload` 增加 `if (completed) return;`。

### B　可见区域只在第一层判一次（`backend/web/app.js`）

1. `pumpCardImageQueue` 发名额时执行 `job.image.loading = "eager"`，第二层的懒加载闸门因此不再重复拦截。
2. 第一层观察器的 `isIntersecting === false` 分支新增撤回：任务已排队但还没拿到名额（`!job.active && !job.done && !job.cancelled`）时 `withdrawCardImageJob(job)`（从 `state.cardImageQueue` 里摘掉、`dataset.cardImage` 回到 `pending`），然后**重新 observe**。撤回不是取消，滚回来还能继续加载。

**工单没写、但不补就无法生效的一处**：原来第一层在 `isIntersecting` 分支里**立刻 `unobserve`**，于是「已排队未拿到名额」的卡片根本不在观察列表里，卡片滚出 620px 时观察器不会回调，撤回永远不会触发。现在改成**只有真的拿到名额（`active`/`done`/`cancelled`）才 unobserve**，并在 `finishCardImageJob` 里补一次 `state.cardImageObserver?.unobserve(job.image)` 收尾。`backend/web/r130.test.cjs` 的撤回用例就是先测出这个缺口（撤回后 `dataset.cardImage` 仍是 `loading`）才补的。

### C　悬停视频释放连接（`backend/web/app.js`）

新增 `state.hoverVideo` 与 `stopHoverVideo()`：

- `prepareSkinVideo` 补 `pointerleave` 与 `blur`，两者都调 `stopHoverVideo()`。
- `stopHoverVideo()` 调既有的 `resetVideo(video)`（移除 `src` + `load()`，这才会真正中断传输；单纯 `pause()` 在 Chromium 里仍保持连接）。
- 展示状态**按当前真实情况恢复**，不用播放前的快照：`const loaded = image.classList.contains("is-loaded"); image.hidden = false; fallback.hidden = loaded;`。快照方案是评审否掉的——视频播放期间图片可能已经加载完成，快照会把「加载中」的占位重新盖到已经出图的卡片上。三种状态都对：出过图只显示图；还在加载显示「加载中」；候选全部失败显示「暂无预览」（此时图片没有 `is-loaded`，CSS 里就是 `opacity: 0`，露出来也看不见）。
- 全局同时最多 1 个：`start()` 里先 `stopHoverVideo()` 再 `loadSkinVideo(...)`；同一个 video 重复 `pointerenter`/`focus` 有 `state.hoverVideo?.video === video` 的短路。
- 去掉了原来的 `{ once: true }`，否则停掉之后再也无法重新播放；测试有一条 `doesNotMatch(/\{ once: true/)` 钉住。
- 停视频的调用点共 6 处：`renderItems()`、`renderPoolCatalog()`、`activateSection()`、`activateFavoritesPage()` 四个列表拆除点，加上 `openSkinDetails()` 与 `openChromaDetails()`。后两处是评审补的——弹窗进入顶层之后卡片就收不到可靠的 `pointerleave` 了，`/api/media` 会一直挂着连接，和弹窗自己的原画、背景抢浏览器那 6 条连接，等于把 P1-C 的场景搬进了弹窗。

### D　离开奖池页时取消奖池网格（`backend/web/app.js`）

`activateFavoritesPage` 与 `activateSection` 在既有的 `cancelDeferredImages(el.grid)` 之后补 `cancelDeferredImages(el.poolSkinGrid)`。

`deferCardImageSources` 调用点全量核对（工单 §2-5 要求）：全文件共 4 处匹配 = 1 处定义 + 3 处调用（`makeChromaCard` → `el.grid`、`makeChromaStack` → `el.grid`、`makeSkinCard` → `el.grid` 或 `el.poolSkinGrid`）。两个容器的取消点：

| 容器 | 被替换时 | 被隐藏时 |
|---|---|---|
| `el.grid` | `renderItems()` | `activateSection()`、`activateFavoritesPage()` |
| `el.poolSkinGrid` | `renderPoolCatalog()`（本来就有） | `activateSection()`、`activateFavoritesPage()`（本轮补） |

`backend/web/r130.test.cjs` 用 `assert.equal(callSites, 4, …)` 钉住调用点数量，新增调用点时必须重新核对取消逻辑。

`closeSkinDialog()` 另补了三行（评审）：`el.skinDialogImage.onload/onerror = null` 与 `removeAttribute("data-queued-src")`。弹窗图不占第一层名额，但节点一直在文档里，第二层放弃时照样补发 error；不摘掉的话一个已经关闭的弹窗会继续试剩下的候选地址、白占连接，还会把「暂无预览」写进下一次打开要用的占位里。

### 6　诊断 `card_image_stalled`

| 层 | 改动 |
|---|---|
| `backend/web/app.js` | `reportCardImageStall(sourceIndex, url)`：字段 `activeCardImages`、`queued`、`reason: "watchdog"`、`sourceIndex`、`imageSource`（来源类别，不带路径）。限速 `CARD_IMAGE_STALL_REPORT_INTERVAL_MS = 10_000`，状态存在 `state.lastCardImageStallReportAt`。只有登记在册的卡片任务才上报（`state.cardImageJobs.has(image)`），弹窗图不算 |
| `backend/web/image-queue.js` | 导出 `window.deepLegendsImageSourceLabel`，来源分类只有这一份实现 |
| `backend/web/runtime.js` | 事件白名单加 `card_image_stalled`（不加就会在浏览器里被直接丢掉，永远到不了后端）；新增字段透传分支，`imageSource` 仍按 `lcu/communitydragon/ddragon/gtimg` 枚举放行 |
| `backend/features.go` | `clientDiagnosticEvents` 加 `"card_image_stalled": {"watchdog": true}`；`clientDiagnosticRequest` 加 `ActiveCardImages`/`Queued`/`SourceIndex`（`DisallowUnknownFields` 下缺一个就整条 400）；新增落盘分支，写 `active_card_images`（≤1000）、`queued`（≤100000）、`source_index`（≤100）、`image_source`（枚举内才写） |

**限速位置的取舍（工单未指定）**：放在 `app.js`，**没有**复用 `runtime.js` 的 `sampled` 通道。那条通道的 `sampledPending` 是所有 sampled 事件共用的一个 in-flight 集合，`local_request_client` 在途时会把 stall 一起压掉；而 stall 恰恰最容易和图片失败同时发生。两处都写了注释说明。

**`imageSource` 不做兜底**：原来写的是 `window.deepLegendsImageSourceLabel?.(url) || "lcu"`，评审指出 `imageSourceLabel` 对枚举外的 `source=` 是**故意**返回 `""` 的（不把没归类的 URL 算成本机客户端），`|| "lcu"` 正好把这个判断反过来，属于 AGENTS.md 禁止的「用推断值代替证据」。已改成直接透传 `?.(url)`，分类不出来就不带这个字段（`runtime.js` 与 `features.go` 两侧本来就会丢掉非枚举值）。

**这条日志的准确语义**（读下一份真机日志时按这个理解）：`card_image_stalled` = 「第一层的一个卡片名额被占满了 45 秒，期间没有任何 load/error」。它有两种成因，都意味着用户已经盯着「加载中」看了 45 秒，都该报：
- 第二层彻底放弃却没能通知第一层（本轮修的那个缺陷；`nosynthetic` 构建实测 ≥1 条）；
- 第二层严重积压——第二层只有 5 个名额（远程道 2 个）、第一层有 8 个，排在第二层队尾的卡片可能等满 45s 才轮到（生产构建真机实测就是这种，8 张卡报 1 条，被 10 秒限速合并）。

区分办法：对照同一时间窗的 `local_request_client / failed(endpoint=image)`。有大量图片失败 → 是第二种；几乎没有失败却出现 stall → 是第一种，说明第二层又没能通知上来。评审建议过「只在 `image.hasAttribute("src")` 时上报」来排除第二种，被否掉了：第二层放弃时会先移除 `src`，那样正好会把第一种（真正要抓的）也一起过滤掉。

### 测试（`backend/web/r130.test.cjs`，28 条，P1 部分全部带对抗变异）

跑的是真实 `image-queue.js` + 从真实 `app.js` 原样抽出的 14 个函数（`functionSource` 花括号配对，与 r123/r129 同一套写法），只有「滚动容器 + 两层可见区域判据」按工单 §1-B 的实测结论建模：第一层观察器带 `root=appScroll` 与 620px 外扩，第二层观察器 root 是视口、被 `appScroll` 裁掉所以有效外扩按 0 算。

| 工单要求 | 用例 | 结果 |
|---|---|---|
| 前提：移除 src 不触发事件 | `P1 前提：移除 src 不触发任何 load/error 事件` | PASS |
| 复现 A：8 张永不返回 → 名额归零 → 后续正常图全部加载 | `P1-A 8 张永不返回的图放弃后名额归零，后续正常图全部加载` | PASS |
| A 的「后面的候选地址也永远不会去试」 | `P1-A 挂起的第一个候选放弃后必须继续试下一个候选地址`（断言 `is-loaded` 与占位隐藏，**不是**第二层写的 `imageReady`） | PASS |
| 变异：去掉 `dispatchEvent` **且**关掉看门狗 → 必须 FAIL | `P1-A 变异：两道防线都关掉时名额永久泄漏` | PASS（名额仍 8、8 张停在「加载中」、新图 0 张加载） |
| 只关掉其中一个仍要 PASS | `只留看门狗（第二层不补发 error）`、`只留合成 error（关掉看门狗）` | 两条都 PASS，两道防线各自独立有效 |
| A 的交错（评审补） | `P1-A 变异：看门狗先推进时，第二层迟到的合成 error 不得再推进一次`——故意把看门狗调回 25s 逼出交错：带闸门 4 张全部出图，去掉闸门 4 张全部落到「暂无预览」 | PASS |
| A 的时长关系（评审补） | `P1-A 看门狗必须晚于第二层的最坏放弃时间`——从 `image-queue.js` 读出 10s/10s/1 次算出 30s，断言生产常量 > 30s | PASS |
| A 的生命周期（评审补） | `P1-A 取消/重建之后看门狗不得复活已取消的卡片`、`P1-A 看门狗只统计卡片任务，详情弹窗不得污染 stall 上报` | PASS |
| 复现 B：40 张图、容器 300px、跳到底部 → 2 秒内开始加载 | `P1-B 跳到列表底部后屏幕上的卡片必须在 2 秒内开始加载` | PASS |
| 变异：去掉 eager 设置 → 必须 FAIL | `P1-B 变异：去掉 eager 设置后屏幕上的卡片永远排不到` | PASS（>2000ms 且屏幕上一张都没出图） |
| B 的撤回 | `P1-B 已排队但未拿到名额的任务离开预取范围会被撤回` | PASS |
| C：`pointerleave` 后 src 被移除；连续悬停 3 张只有 1 个在播 | `P1-C pointerleave 之后视频 src 被移除，且全局只有 1 个视频在播放` + 一条源码钉子（`blur`、`resetVideo`、不得 `{ once: true }`） | PASS |
| D：打开奖池页 → 切回皮肤页 → `activeCardImages` 只统计皮肤列表 | `P1-D 取消奖池网格后 activeCardImages 只统计皮肤列表的任务`（5 个不可见奖池任务占着名额 → 取消后归 0）+ `P1-D 切换收藏子页与一级页签时都取消奖池网格` | PASS |
| 6：stall 上报 | `P1-6 看门狗上报 card_image_stalled，每 10 秒最多一条且不带路径`（用 `nosynthetic` 构建让看门狗成为唯一推进者；8 张同时超时只报 1 条，下一个窗口才允许第 2 条）+ `P1-6 card_image_stalled 打通前端白名单与后端白名单` | PASS |
| 评审补漏汇总 | `R130 评审补漏：弹窗、悬停视频、来源分类与旗帜取样的边界`（12 条断言） | PASS |

后端侧 `backend/r130_test.go`（5 条，复用 `r127_test.go` 的 helper）：落日志字段、白名单与未知 reason → 400 + `client_diagnostic_rejected`、上下界钳制与非枚举 `imageSource` 被丢弃、`imageSource` 塞原始路径时序列化后不含 `profile-icons`/`4379`/`/api/image`/`lol-game-data`、字段不串到 `local_request_client`。

---

## 2. P2　旗帜竖长卡

`backend/web/app.css`：

```css
.facade-grid.is-banners { --facade-banner-ratio: 0.3; grid-template-columns: repeat(auto-fill, 150px); justify-content: start; gap: 12px 10px; }
.facade-grid.is-banners .skin-card { contain-intrinsic-size: auto 524px; }
.facade-grid.is-banners .skin-art { aspect-ratio: var(--facade-banner-ratio); border-radius: 8px; }
.facade-grid.is-banners .skin-art img { object-fit: contain; }
```

`--card-min: 168px` + `aspect-ratio: 1/1` 已删除。`justify-content: start` 保证宽窗口下格子尺寸不变（实测 1400px 窗口：`grid-template-columns` 全是 `150px`，不拉伸）。

`backend/web/favorites-facade.js` 新增 `sampleBannerRatio(image, isBanner)`：第一张加载成功的旗帜把 `naturalWidth / naturalHeight` 写进网格的 `--facade-banner-ratio`（`bannerRatioSampled` 闩住，只取一次，避免每张卡各写一次导致整网格反复重排）；取样前 CSS 里的 `0.3` 兜底；`naturalWidth/naturalHeight` 为 0（还没解码）时不写也不上闩，允许下一张继续取样。

两个评审补的边界：

- `isBanner` 由 `createCard` 在**建卡时**定死，不在取样函数里读实时 `state.view`。卡片图片的 `onload` 是异步的，等它回来时视图可能已经切走，读实时值会让一张方形头像把 `1.0000` 写进整个旗帜网格（5099 个格子一起变成正方形）。
- 新增 `resetBannerRatioSample()`，在 `load()` 成功拿到 banners 目录后调用（清闩 + 摘掉网格上的旧值）。重试按钮、断连重连都会重新读目录，比例不能永久锁死在第一张旗帜上。

名称等文字移到图片下方：`.facade-grid .skin-copy { position: static; margin: 6px 0 0; padding: 0; background: none; }`、`.facade-grid .skin-hero, .facade-grid .skin-meta { display: none; }`，并给旗帜卡片补 `card.title = fields.title`。这三条原来是 `.facade-grid.is-icons` 专属，本轮提升到 `.facade-grid`，头像与旗帜共用一份（同时少 3 条重复声明组，对 r117-style 的棘轮是净减少）。

**验收实测**（真机 Chromium，旗帜合成图 102×400）：`--facade-banner-ratio` = `0.2550`（= 102/400）；格子列宽 150px；图片区域 148 × 580.39px（148 = 150 − 左右各 1px 边框；580.39 = 148 / 0.255）；`object-fit: contain`；**上下留白 0.00%**（≤5% 达标）；不裁剪；名称块的 `top`(762.4) ≥ 图片块的 `bottom`(756.4)，即文字在图片下方。截图 `docs/r130-validation/browser/p2-banners.png`。

## 3. P3　去掉右上角「已拥有 / 未拥有」标签

| 位置 | 改动 |
|---|---|
| `favorites-facade.js` `createCard()` | 不再写 `.skin-state`，改成 `badge.textContent = ""; badge.hidden = true;` |
| `app.js` `makeChromaCard()` | `status.textContent = ""; status.hidden = true;`（原为 `chroma.owned ? "已拥有" : "未获取"`） |
| `app.css` | 新增 `.facade-grid .skin-state { display: none; }`；删除已成死规则的 `.facade-grid.is-icons .skin-state { top: 4px; … }` |

保留项（工单明确要求，已逐条钉住）：

- `makeSkinCard` 的「三合一剩余」标签**保留**（`status.textContent = showPoolState ? "三合一剩余" : ""`）——它表示奖池状态，不是拥有状态。
- 详情弹窗的「拥有状态」一行**保留**（头像 + 旗帜各一处，测试断言恰好 2 处）。
- `facadeCollectionIconFields` / `facadeCollectionBannerFields` 仍然返回 `state` 字段（弹窗要用），R123 对返回值的三条断言不用改。
- 未拥有仍然靠置灰 + 锁图标：`card.classList.toggle("is-locked", fields.locked)` 不动，`.facade-grid .skin-lock` 从 `.is-icons` 专属提升到整个 facade 网格（旗帜格子也要能上锁）。
- **R124 降级不回退**：拥有状态未知时 `fields.locked` 仍是 `false`（不上锁），标签也不显示，摘要行有「拥有状态未读取」。真机实测：降级态 12 张卡、`locks: 0`、`stateBadges: 0`。

真机实测：头像默认视图 8 张卡 `stateBadges: 0`、`stateVisible: 0`；打开「显示未拥有」后 12 张卡 `locks > 0` 且 `stateBadges` 仍为 0；旗帜 8 张卡 `stateBadges: 0`。截图 `p3-p5-icons.png`、`p3-icons-with-unowned.png`、`p3-p5-degraded.png`。

## 4. P4　去掉三格统计条

- `index.html`：删除整个 `<dl class="metrics facade-metrics">`（含 `#facade-total`、`#facade-owned`、`#facade-visible`）。
- `favorites-facade.js`：`el` 里删掉 `total`/`owned`/`visible`/`metricsBar` 四个引用；`renderDisconnected()` 删掉 `el.metricsBar.hidden = true`；`render()` 删掉 `el.metricsBar.hidden = false` 与三处 `textContent` 赋值，只留 `renderFacadeMeta(...)` 一行摘要。
- `app.css`：删掉 `.facade-metrics`、`.facade-metrics div`、`.facade-metrics dd` 三条。

`renderDisconnected` 的收起逻辑同步调整完毕，不再引用任何已删除的元素（测试用 `doesNotMatch(/metricsBar|el\.total|el\.owned|el\.visible/)` 钉住）；`el.toolbar.hidden` / `el.listRow.hidden` 两条 R123 断言继续通过。别处的 `.metrics` 统计条（账户页）未受影响，测试也钉了一条 `assert.match(html, /class="metrics/)`。全仓扫描确认没有悬挂引用（`suite.js` / `demo-data.js` / `desktop/*.cjs` 里的 `data-facade-owned` 是生涯页的另一个控件，属性名不是 id）。

真机实测：`metricsBar: false`、`totalNode: false`。

## 5. P5　摘要行与默认筛选

1. **去掉「已显示」**：`facadeCollectionMetaText(kind, total, ownedCount, ownershipUnavailable)` 少一个 `visible` 参数，输出 `共 5,099 款头像 · 已拥有 478`；拥有状态不可用时输出 `共 5,099 款头像 · 拥有状态未读取`。
2. **数字用主题色**：新增 `.list-meta-number { color: var(--primary); font-weight: 650; font-variant-numeric: tabular-nums; }`。
   - 头像/旗帜摘要：`facadeCollectionMetaParts()` 返回 `[文本, 是否数字]`，`renderFacadeMeta()` 用 `document.createTextNode` / `createElement("b")` + `textContent` 拼，**全程不碰 `innerHTML`**。同时把纯文本版写进 `el.meta` 的 `aria-label`——摘要被拆成多个节点后读屏软件会一段一段念，`role="status"` 的容器需要一份完整文本。
   - **`aria-label` 必须在断连/出错时一起清掉**（评审补）：`aria-label` 在可访问名计算里优先级高于子树内容，`renderDisconnected()` 把 `textContent` 清空之后，读屏软件念的会是上一份目录的「共 N 款 · 已拥有 M」。`renderDisconnected()` 与 `renderError()` 现在都执行 `el.meta.removeAttribute("aria-label")`。
   - 皮肤摘要（`app.js` `renderItems`）与炫彩摘要（`applyChromaVisibility`）改走新的 `renderListMeta(...segments)`：数字直接传 `number`（内部 `formatNumber` + `createElement("b")`），字符串走 `createTextNode`，数组按顺序展开，空值跳过；同样是 `replaceChildren`，**不用 `innerHTML`**。
   - 只改摘要行：`makeChromaStack` 与 `chromaSummaryAttributes`（英雄分组小标题）测试断言 `doesNotMatch(/list-meta-number/)`，未被动到。
   - 转义钉子：`renderFacadeMeta('<img src=x onerror=alert(1)>', …)` 之后 `el.meta.querySelectorAll('img').length === 0`，尖括号原样保留为文本。
   - 真机实测数字样式：`color: rgb(217, 164, 65)`（= `--primary`）、`font-weight: 650`、`font-variant-numeric: tabular-nums`。
3. **头像默认不显示未拥有**：`state.filters.icons.showUnowned` 改 `false`（`banners` 仍是 `true`），`index.html` 的 `#facade-show-unowned` 去掉 `checked`，初始状态一律由 `syncControls` 从 filters 写回。真机实测：头像默认 8 张（= 已拥有数）、开关未勾选；旗帜默认 8 张（全部）、开关勾选。
4. **与 R124 的配合**：删掉 `syncControls` 里的 `if (unavailable) filters.showUnowned = true;`。实际生效值本来就由 `facadeCollectionIconRows` / `facadeCollectionBannerRows` 里的 `if (!ownershipUnavailable && !filters.showUnowned && !icon.owned) return false;` 决定，等价于工单要求的 `unavailable || filters.showUnowned`，所以**过滤逻辑一行没动**，只是不再改写用户保存的值。
   - R124 原有用例（状态不可用时网格不能为空）继续通过：`r123.test.cjs` 里那 4 条断言一个字没改。
   - 新增用例：`backend/web/r130.test.cjs` 的 `P5-4 拥有状态从不可用恢复后，头像开关回到用户原来的设置`——不可用期间开关隐藏但 `checked` 仍是 `false`、`filters.icons.showUnowned` 仍是 `false`、网格 3 条不被滤空；恢复可用后开关重新可见且**回到 `false`**，此时过滤才真的生效（只剩 1 条已拥有）。带对抗变异：把 `if (unavailable) filters.showUnowned = true;` 塞回去，该用例必须抛错。
   - 真机 Chromium 也跑了同一条链路（真实 `favorites-facade.js` + 真实断连/重连事件）：降级态 12 张卡 / 开关隐藏 / `checked: false`；恢复后 8 张卡 / 开关可见 / `checked: false`。

## 6. P6　头像详情弹窗按原尺寸显示

`backend/web/favorites-facade.js`：`setDetailArtSize(w, h)` 把尺寸写进 `--facade-art-width` / `--facade-art-height`；`applyDetailArtSize()` 在 `onload` 里按 `naturalWidth × naturalHeight` 计算——头像取 `min(256, max(nw, nh))` 的正方形（≤256px 时就是 CSS 像素 1:1，不放大），旗帜按自身比例缩放、高度封顶 `DETAIL_ART_BANNER_MAX_HEIGHT_PX = 320`；`naturalWidth/Height` 为 0 时不改尺寸。`openDetail()` 加载前先按 `DETAIL_ART_PLACEHOLDER_PX = 128` 占位，避免弹窗尺寸跳动。

`openDetail()` 在赋 `src` 之前先 `removeAttribute("src")`（评审补）：重复打开同一条目时 URL 没变，浏览器可能直接复用当前请求、不派发 `load`，`applyDetailArtSize()` 就永远不会跑，弹窗会卡在 128×128 的占位上而图片已经显示出来了——正好是 P6 要消灭的尺寸跳动，方向反过来。`app.js` 的 `resetDialogImage` 本来就是这么防御的。

`backend/web/app.css`：`.facade-detail-dialog` 由 `width: min(560px, …)` 改成 `width: fit-content` + `min-width: min(320px, …)` + `max-width: calc((100dvw - 28px) / var(--ui-zoom, 1))`；`.dialog-art` 由 `aspect-ratio: 1/1`（跟着 560px 弹窗变成 560×560）改成 `width: var(--facade-art-width, 128px); max-width: 100%; height: var(--facade-art-height, 128px); aspect-ratio: auto; margin: 12px auto 0`。布局选**图标在上、信息在下**（原标记顺序不变）。

**工单没写、但为达标所必需的一处结构改动**：关闭按钮 `<div class="dialog-toolbar">` 从 `.dialog-art` 里挪到 `.dialog-copy` 的开头。`.dialog-art` 带 `contain: layout paint`，图片区域缩到 128px 之后那个悬浮工具条（约 42px 的深色胶囊）会盖住头像三分之一，留在里面也逃不出裁剪。挪到 `.dialog-copy`（`position: static`）之后它的包含块变成弹窗本身，仍然落在右上角，`.dialog-toolbar` 的既有样式一条没改；`#skin-dialog` 的工具条位置没动。评审确认：backdrop 点击（`event.target === el.dialog`）与 `cancel` 处理都不受影响，工具条仍然是弹窗里第一个可聚焦元素。真机实测 `closeInsideArt: false`。

**验收实测**（真机 Chromium）：

| 项 | 结果 |
|---|---|
| 头像 128×128 | 图片区域 `128 × 128`，`--facade-art-*` = `128px/128px`，`object-fit: contain` → CSS 像素 1:1，不放大 |
| 头像弹窗宽度 | `320px`（原 560px） |
| 详情行 | `名称 / 系列 / 拥有状态 / ID`，「拥有状态」保留 |
| 旗帜 102×400 | 图片区域 `82 × 320`，比例 0.25625 vs 图片 0.255（差 0.00125，来自 `Math.round`），高度 ≤ 320 上限，`contain` 不裁剪 |
| 皮肤/炫彩弹窗 | `.skin-dialog`（900px）、`.dialog-art`（16/9）、`.skin-dialog.is-chroma-dialog` 三条规则一字未改，测试逐条钉住；`--facade-art-*` 不会渗到皮肤弹窗 |

截图 `p6-icon-detail.png`、`p6-banner-detail.png`。

---

## 7. R123 / R124 / R125 旧用例的逐条改动（工单验收总表要求列出）

先说结论：**R123/R125 里没有任何一条用例断言「统计条存在」或「状态标签存在」**，所以 P3/P4 没有需要删除的旧断言。实际改动集中在 P5，共 4 处，全部在 `backend/web/r123.test.cjs`：

| # | 位置 | 原内容 | 改成 | 原因 |
|---|---|---|---|---|
| 1 | 第 34 行 `names` 数组 | `[…, "facadeCollectionMetaText"]` | 增加 `"facadeCollectionMetaParts"` | `facadeCollectionMetaText` 现在是 `facadeCollectionMetaParts` 的纯文本投影，抽函数时必须一起抽，否则 `ReferenceError` 会让整个文件（含 R124/R125 用例）在加载期就炸 |
| 2 | 用例 `R123 unowned entries show by default and hide when the toggle is off` 的标题与第一条断言 | `assert.match(facadeSource, /showUnowned: true/, "显示未拥有默认必须开启")` | 标题改为 `R123 unowned entries hide when the toggle is off (R130: icons default to owned-only)`；断言改为三条：`icons: {…showUnowned: false }`、`banners: {…showUnowned: true }`、`doesNotMatch(html, /id="facade-show-unowned"[^>]*checked/)` | P5-3 把头像默认值改成 `false`。原断言只写 `/showUnowned: true/`，旗帜那一行仍会命中，属于「改坏了也测不出来」，所以拆成按视图分别钉住，并补上 HTML 不得预置 `checked` |
| 3 | 用例 `R123 ownership-unavailable payloads downgrade state badges and locks` 的最后 4 行 | `facadeCollectionMetaText("头像", 5099, 0, 240, true)` / `("头像", 5099, 128, 240, false)`，期望 `"共 5,099 款头像 · 已拥有 128 · 已显示 240"` | 去掉 `visible` 实参，期望改为 `"共 5,099 款头像 · 已拥有 128"` | P5-1 |
| 4 | 用例 `R124 ownership-unavailable catalog survives a previously closed unowned toggle` 的最后 2 行 | `// 双重保险：syncControls 在不可用时必须把记忆值复位为 true。` + `assert.match(facadeSource, /if \(unavailable\) filters\.showUnowned = true;/)` | `const syncControls = functionSource(facadeSource, "syncControls");` + `assert.doesNotMatch(syncControls, /filters\.showUnowned\s*=[^=]/, "syncControls 不得再改写用户保存的开关值")` + `assert.match(syncControls, /el\.showUnowned\.checked = filters\.showUnowned;/, "开关显示状态仍然从记忆值写回")` | P5-4。**这条断言的极性被反转**：它原来钉住的正是那条会永久改掉用户设置的语句。新写法只在 `syncControls` 函数体内匹配，不会被注释里的字面量误触发（第一版写成全文 `doesNotMatch(/filters\.showUnowned = true/)` 时被自己的注释命中，已改） |

R124 用例的另外 4 条断言（不可用时图标 3 条 / 旗帜 2 条不被滤空，可用时开关仍生效返回 0 条）**一字未改，继续通过**。
R125 用例 `R125 icon tiles render at native square size` 的 5 条断言（92px 固定列、`aspect-ratio: 1/1`、`object-fit: contain`、`is-icons` 类切换、`card.title`）**一字未改，继续通过**——P2 把 `.skin-copy`/`.skin-hero`/`.skin-meta`/`.skin-lock` 四条规则从 `.is-icons` 提升到 `.facade-grid`，没有触碰这 5 条钉的选择器；`if (state.view === "icons") card.title = fields.title;` 也原样保留，旗帜是另起一行。

工单里写的 `backend/web/r125*.cjs` **不存在**（`R124`/`R125` 的用例都在 `r123.test.cjs` 里），所以那条命令的第二个 glob 匹配为空，不是漏跑。

---

## 8. 验证

### 自动化（本机 macOS，Node v24.14.1）

| 命令 | 结果 |
|---|---|
| `go build ./backend` | OK |
| `go vet ./backend` | OK |
| `gofmt -l backend/features.go backend/r130_test.go` | 无输出（clean） |
| `go test ./backend -count=1` | `ok lol-loot-assistant/backend 204.523s`（全量，在评审整改前跑的；整改只动了前端，之后又跑了 `-run 'R130\|R127\|ClientDiagnostic'` 与 `-run 'R130\|R127\|R104\|R117\|StaticAssets\|R70Embedded'` 两轮，均 ok） |
| `go test ./backend -run 'R130' -count=1 -v` | 5 条全部 PASS（子测试 4 条也 PASS） |
| `node --check` × app.js / image-queue.js / favorites-facade.js / runtime.js / suite.js / desktop/main.cjs | 全部 OK |
| `node --test backend/web/*.test.cjs` | **685 tests / 685 pass / 0 fail**（含新增 `r130.test.cjs` 28 条） |
| `node --test backend/web/r117-style.test.cjs` | 6/6 PASS（CSS 变量全部可解析、23 个 token hex 仍单一来源、`border-radius`/`padding`/`gap`/重复 hex/未引用类名/重复声明组六条预算棘轮均未超） |
| `node --test backend/web/r123.test.cjs` | 8/8 PASS（R123 + R124 + R125） |
| `CHROME_BIN=… node desktop/r100-browser.cjs` | `R100 Chromium PASS {"elapsed":2.3,"peakImages":5}`（既有真机图片队列护栏未被 P1 改动破坏） |
| `CHROME_BIN=… node docs/r130-validation/r130-browser.cjs` | `R130 Chromium PASS`，`browserErrors: []`，三种构建各跑一遍 |

*关于「基线 81 条失败」的说明*：本轮开工时先跑了一次 `node --test backend/web/*.test.cjs` 做基线，出现 81 条失败（r87/r90/r91/r94/r95/r113/r116b/r116d，多为 `ReferenceError: liveRenderTriggerLabel is not defined` 这类 harness 编译错误）。当时有两个 explorer 子代理正在并行扫仓库，CPU 被打满；子代理结束后**同一份未改动的代码重跑，全绿**。这 81 条是负载导致的假红，不是仓库既有缺陷，也不是本轮引入。收尾时又跑了一次，685/685。

### 真机 Chromium（`docs/r130-validation/r130-browser.cjs`）

跑的是真实 `image-queue.js`、真实 `favorites-facade.js`、真实 `app.css`、从真实 `index.html` 切出来的收藏页片段与 `#skin-card-template`、从真实 `app.js` 原样抽出的队列函数；服务端 `/api/image?path=…hung…` 只发响应头永不写 body（真实挂起连接，本轮共 11 个），图标/旗帜是带精确 `naturalWidth/naturalHeight` 的合成 SVG。目录 JSON 是合成的，**不连英雄联盟客户端**。看门狗时长与第二层的超时/冷却常量都从生产源码里读，脚本内不写死副本。

三个构建（`?mode=` 切换，服务端换源码）：生产、`nosynthetic`（只关掉第二层补发）、`mutated`（两道防线都关掉）。产出 `docs/r130-validation/browser/`：`result.json` + 9 张 png。

两个真机专属的坑（已修，记在这里避免复用时再踩）：

1. 变异体要「关掉看门狗」，jsdom 里传 `Infinity` 给假时钟即可，但**真机 `setTimeout` 会按 WebIDL `long` 把 `Infinity` 转成 0**，看门狗立刻触发，变异体反而不泄漏。脚本里改用 `2000000000`（int32 上限附近，约 23 天）。
2. 变异体不能在同一个页面里重新 `eval` 一份 `image-queue.js`——原来那份的 `MutationObserver` 还在，会变成两层队列同时抢同一批 `data-queued-src`。改成整页重新导航、由服务端按 `mode` 换源码。

### 未验证 / 边界（按 R107 台账口径如实列出）

1. **工单验收总表的手动 5 分钟验证没做**：「在皮肤列表来回快速拖动滚动条、悬停十几张动态原画卡、在奖池页和皮肤页之间切换，持续 5 分钟后屏幕上的卡片仍能在 2 秒内出图」需要连着国服客户端、在 Windows 真机上跑真实皮肤目录与真实动态原画视频。本轮只做到：单条机制在真机 Chromium 上复现并修复（第 0 节的表）、40 张图跳底部在 jsdom 里 2 秒内出图、悬停 3 张卡只有 1 个视频在播。**请在下一次真机运行时按这 3 个动作各跑一遍，并导出诊断日志确认有没有 `card_image_stalled`。**
   → **R133 复核后这一条仍未关闭**：R133（`docs/WORKLIST-R133-R130-VERIFICATION-FOLLOWUPS.md` P2）再次确认本机没有英雄联盟客户端（无进程、无安装路径、无 LCU lockfile、无监听端口），所以真机验证到现在还是空的。交接件已经备好，见 `docs/r133-execution-ledger.md` 第 2 节：8 步操作流程 + `node scripts/r133-stall-log-report.cjs <导出的 jsonl>`，脚本会按同一时间窗的 `local_request_client/failed(endpoint=image)` 数量把结果判成 `clean` / `backlog-only` / `notify-gap` / `inconclusive`，并且会拒绝把 0.12.18 之前的旧日志判成 `clean`。
2. **P1-C 的连接占用没做真实大体积视频实测**：工单 §1-C 的「5 个 40MB 视频占满 6 条连接」由诊断人已经实测过，本轮只验证了修复侧的可观察结果（`pointerleave`/`blur` 后 `src` 被移除、`resetVideo` 被调用、全局同时只有 1 个视频、打开详情弹窗也会停、原画/占位正确恢复），没有用真实 40MB 视频再测一次 Chromium 的连接占用。
3. **45 秒这个看门狗阈值仍然没有真机数据支撑**：它是按「必须晚于第二层最坏放弃时间 30s」推出来的下界加余量，不是从真实日志里量出来的。下一份真机日志里如果 `card_image_stalled` 大量出现、而同一时间窗的 `local_request_client/failed(endpoint=image)` 很少，说明是第二层积压而不是通知失败，那时该调的是第二层的名额而不是这个阈值（判读方法见第 1 节第 6 条）。
4. **`desktop/*.test.cjs` 有 2 条先前就红的用例，本轮未动**（不在工单范围）：
   - `desktop/overview-render.test.cjs:774` `R101 保留旗帜只读入口` —— 断言 `.facade-left [data-facade-banners]`，而 R123 已经把 `data-facade-banners` 从 `suite.js` 删掉换成 `data-facade-browse="banners"`（`grep -c data-facade-banners backend/web/suite.js` = 0）。
   - `desktop/diagnostics-2024.test.cjs:126` —— 断言 UI 里有 `不代表抽取、刷新或保底概率`，而 R128 §2.5 已经按用户指示把这类免责声明从界面上删干净（`grep -c … backend/web/gameplay.js` = 0）。
   两条都属于旧用例没跟上 R123/R128，需要单独一轮处理；本轮没有顺手改，因为工单只授权更新 R123/R125 的用例。
5. **CI 的 gofmt 门禁在本轮之前就是红的**：`gofmt -l backend/` 列出 `champions.go`、`gameplay.go`、`hexdata.go`、`overview_current_game.go` 四个文件（均为本轮之前的未提交改动）。已核实：把本轮对 `features.go` 的三处新增原样摘掉后该文件 gofmt-clean，`gofmt -w backend/features.go` 实际只改了我自己那一行的 map 对齐（`"card_image_stalled":` 后 9 个空格 → 11 个空格），没有顺手重排任何既有代码。那四个文件本轮**没有**格式化（超范围）。

---

## 9. 改动文件清单

| 文件 | 改动 |
|---|---|
| `backend/web/image-queue.js` | P1-A 合成 error（三道闸门：`!cancelled`、`wasConnected`、`data-queued-src === url`）；导出 `deepLegendsImageSourceLabel` |
| `backend/web/app.js` | P1-A 看门狗（45s）+ 陈旧事件闸门 + 只报卡片任务 + 弹窗两处清理；P1-B eager + 撤回 + 延迟 unobserve + `finishCardImageJob` 收尾 unobserve；P1-C `stopHoverVideo`/`pointerleave`/`blur`/单一播放 + 6 处调用点；P1-D 两处 `cancelDeferredImages(el.poolSkinGrid)`；P1-6 `reportCardImageStall`；P3 炫彩标签；P5 `renderListMeta` + 两处摘要行 |
| `backend/web/favorites-facade.js` | P2 `sampleBannerRatio(image, isBanner)` + `resetBannerRatioSample`；P3 `createCard` 标签；P4 删 4 个 `el` 引用与 4 处赋值；P5 `facadeCollectionMetaParts`/`renderFacadeMeta`/`facadeCollectionMetaText` 新签名/icons 默认 `false`/删 `syncControls` 的记忆值复位/断连与出错清 `aria-label`；P6 `setDetailArtSize`/`applyDetailArtSize`/`openDetail`（含先摘 `src`） |
| `backend/web/index.html` | P4 删 `.facade-metrics` 整块；P5 去掉 `checked`；P6 关闭按钮挪出 `.dialog-art` |
| `backend/web/app.css` | P2 旗帜竖长卡 4 条 + 共享 `.facade-grid` 文字/锁 4 条；P3 `.facade-grid .skin-state { display: none; }`；P4 删 `.facade-metrics` 3 条；P5 `.list-meta-number`；P6 `.facade-detail-dialog` 2 条；删死规则 1 条 |
| `backend/web/runtime.js` | P1-6 事件白名单 + 字段透传 |
| `backend/features.go` | P1-6 3 个结构体字段 + 白名单条目 + 落盘分支 |
| `desktop/package.json`、`desktop/package-lock.json` | 0.12.17 → **0.12.18** |
| `backend/web/r123.test.cjs` | 第 7 节列出的 4 处更新 |
| `backend/web/r130.test.cjs` | 新增，28 条（P1 18 条含 6 条对抗变异，P2–P6 8 条含 1 条对抗变异，评审补漏 2 条） |
| `backend/r130_test.go` | 新增，5 条后端诊断用例 |
| `docs/r130-validation/r130-browser.cjs` + `browser/` | 新增，真机 Chromium 验证脚本与证据（1 个 json + 9 张 png） |

`CHANGELOG.md` 未改：R116–R129 都没有写 CHANGELOG（最后一条是 R115 · 0.12.7），本轮沿用 `docs/rNNN-execution-ledger.md` 的口径，没有单独给 R130 补一条造成断档。

嵌入资源清单未改：`backend/main.go` 的 `//go:embed web/*.js …` 不匹配 `.cjs`，新增的两个测试文件都不会进包；本轮没有增删任何 `backend/web/` 下的生产资源，`backend/static_assets_test.go` 的清单无需同步。

---

## 10. 对抗式代码评审：发现与处置

改完之后单独跑了一轮只读评审（不复用实现时的上下文）。评审确认干净的类别：看门狗生命周期、延迟 `unobserve` 的观察集合是否有界、`card_image_stalled` 的隐私与白名单一致性、`.facade-metrics` 删除是否有悬挂引用、工具条移位对焦点/弹窗事件的影响、界面文案红线与超范围改动。以下是它查出并已修掉的：

| 级别 | 缺陷 | 处置 |
|---|---|---|
| **阻断** | 看门狗 25s 早于第二层最坏放弃时间 30s。第二层在 t=30 补发的合成 error 会通过 `onerror` 的陈旧闸门（此时 `data-queued-src` 已经等于 `sources[index-1]`），让第一层再推进一次直接跳到候选末尾——**本来能显示的图被丢掉**，卡片落到「暂无预览」，而第二层随后把那张图加载成功也只是白加载（`onload` 被 `completed` 挡掉，没有 `is-loaded`，CSS 里仍是 `opacity: 0`）。等于用修复重新造了一遍 P1 要修的那种损坏 | 两处一起修：① `image-queue.js` 的补发加 `data-queued-src === url` 闸门，只为真正失败的那个 URL 补发；② `CARD_IMAGE_STALL_MS` 25s → 45s。新增用例 `P1-A 变异：看门狗先推进时，第二层迟到的合成 error 不得再推进一次`（故意把看门狗调回 25s 逼出交错：带闸门 4 张全部出图，去掉闸门 4 张全部「暂无预览」）与 `P1-A 看门狗必须晚于第二层的最坏放弃时间`（从第二层常量算出 30s 并断言生产值更大）。**已实测确认**：只把常量改成 45s 而不去掉闸门时，原有测试仍然全绿——所以这条钉子是必须的 |
| 应修 | 原来那条「候选回退」用例断言的是 `dataset.imageReady`，而这个标记是**第二层**写的，第一层 `complete()` 之后它照样会被置上，所以它对上面的阻断缺陷完全不敏感 | 改成断言第一层自己认的 `classList.contains('is-loaded')`、`fallback.hidden`、以及「不该出现『暂无预览』」 |
| 应修 | `closeSkinDialog()` 只清了看门狗，`onload`/`onerror` 与 `data-queued-src` 还留着。弹窗节点一直在文档里，第二层放弃时照样补发 error，一个已经关闭的弹窗会继续试剩下的候选地址、白占连接，还会把「暂无预览」写进下一次打开要用的占位 | 补 `onload = onerror = null` 与 `removeAttribute("data-queued-src")`，并加源码钉子 |
| 应修 | `openSkinDetails` / `openChromaDetails` 不调 `stopHoverVideo()`。弹窗进入顶层后卡片的 `pointerleave` 不可靠，悬停视频会一直挂着 `/api/media`，和弹窗自己的原画、背景抢那 6 条连接——P1-C 的场景搬进了弹窗 | 两个函数开头各加一行 `stopHoverVideo();`，并加源码钉子 |
| 应修 | `renderFacadeMeta` 写的 `aria-label` 在断连/出错时不清，`aria-label` 优先级高于子树内容，读屏软件会念上一份目录的「共 N 款 · 已拥有 M」；断连时 `textContent` 是空串，那个陈旧 label 是唯一的可访问名 | `renderDisconnected()` 与 `renderError()` 各补 `el.meta.removeAttribute("aria-label")`，并加源码钉子 |
| 应修 | `stopHoverVideo()` 用播放前的快照恢复展示状态。视频播放期间图片加载完成的话，快照会把「加载中」重新盖到已经出图的卡片上 | 改成按当前真实状态推导（`is-loaded` 决定占位是否隐藏），快照字段整个删掉 |
| 细节 | `reportCardImageStall` 的 `imageSource` 兜底成 `"lcu"`，把 `imageSourceLabel` 故意返回 `""` 的「没归类」翻成了「本机客户端」，属于用推断值代替证据 | 去掉 `\|\| "lcu"`，分类不出来就不带字段，并加 `doesNotMatch` 钉子 |
| 细节 | `bannerRatioSampled` 一旦置上永不复位（重试按钮、断连重连都重新读目录，比例却锁死在第一张旗帜上）；`sampleBannerRatio` 读实时 `state.view`，头像卡片迟到的 `onload` 会把 `1.0000` 写进整个旗帜网格 | 新增 `resetBannerRatioSample()` 并在 `load()` 拿到 banners 目录后调用；`isBanner` 改由 `createCard` 建卡时定死并作为参数传入；`doesNotMatch(sampleBannerRatio, /state\.view/)` 钉住 |
| 细节 | `openDetail()` 直接赋 `src`。重复打开同一条目时 URL 没变，浏览器可能不派发 `load`，`applyDetailArtSize()` 就不跑，弹窗卡在 128×128 占位而图片已显示 | 赋值前先 `removeAttribute("src")`（与 `app.js` 的 `resetDialogImage` 同款防御），并加源码钉子 |
| 细节 | `prefetch.delete(img)` 在 `connected(img)` 判断之前执行，脱离文档的预取图片会让自己刚补发的合成 error 被吞掉 | 删除前先 `wasConnected = connected(img)`，闸门改用这个快照 |
| 细节 | 测试 harness 自己写死了一份 `stallMs = 25000` 默认值，和生产常量无关——25s 那版能溜过测试正是因为这个漂移 | harness 与真机脚本都改成从 `app.js` 解析 `PRODUCTION_STALL_MS`，不再留副本 |
| 未采纳 | 建议「只在 `image.hasAttribute("src")` 时上报 stall」以排除第二层积压造成的误报 | **否掉**：第二层放弃时会先移除 `src`，这样正好把「第二层没能通知第一层」（真正要抓的那种）也一起过滤掉。改成在第 1 节第 6 条把语义写清楚，并给出用 `local_request_client/failed(endpoint=image)` 区分两种成因的判读方法 |
| 未采纳 | `enqueueCardImageJob` 的 `requestAnimationFrame` 重试没有次数上限，图片一直不连接时会每帧空转 | **本轮不动**：这是 R130 之前就存在的行为，触发条件是「没有 `IntersectionObserver` 且图片永不连接」，本轮的改动没有让它更容易发生。修它属于超范围，记在这里留给下一轮 |
