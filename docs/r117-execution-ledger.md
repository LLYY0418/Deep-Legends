# R117 执行台账：全项目优化（性能 / 加载 / 资源 / 数据准确性 / 交互 / 样式）

**工单：** `docs/WORKLIST-R117-FULL-PROJECT-OPTIMIZATION.md`（2026-09-20，基线 0.12.7）
**执行日期：** 2026-09-20
**版本：** `desktop/package.json` = **0.12.8**（R116-A 的升版，见 `docs/r116a-execution-ledger.md:3`；R117 工单全文无升版要求，本轮不动）

---

## 0. 三句话结论

1. 工单 33 条全部落地，其中本轮（续接会话）实际动手的是 6 条：P1-2 的 CI 接线、P1-8 的真 Chromium 验收、P3-5 第三条、P3-7 全部子项、P0-1 的前端披露测试、P1-9 的预算测试；其余条目在本会话之前已完成，本轮做的是**逐条对账 + 补测试缺口 + 对抗变异验收**。
2. 补齐了 5 个「有修复、无行为测试」的缺口（P0-1 前端披露、P1-7 生涯栏签名、P1-9 资源预算、P2-6 转圈复位、P2-7 弹窗兜底），并修掉一个由 P1-1 并发化引入的**真实数据竞争**。
3. 全量验证：`gofmt` 干净、`go vet ./backend` 干净、`go test -race -count=1 ./backend` ok（242s）、`node --test backend/web/*.test.cjs desktop/*.test.cjs` **805 项 / 804 通过 / 0 失败 / 1 skipped**（skip 是 Windows 专属发布门禁）、两个真 Chromium 脚本 PASS。

---

## 1. 关键澄清：R116 不是污染，不能「恢复到 HEAD」

上一段会话把 R116 的产物当成污染，计划 `git show HEAD:backend/hexdata.go > backend/hexdata.go` 之类地恢复。**这个前提是错的，本轮已停止并纠正：**

- `git log --oneline -1` = `4ca768ec refactor: Go 源码与内嵌资源统一迁入 backend/`，**HEAD 早于 R116 全部轮次**。
- `backend/hexdata.go` 相对 HEAD 有 1859 行 diff，那是 R116-A（Hexdata 数据源替换）的成果，自带台账 `docs/r116a-execution-ledger.md` 并已验收 PASS。
- `backend/web/shared.js` 被 `backend/web/gameplay.test.cjs:122` 与 `backend/web/gameplay.css:1871` 正常引用，是 R116-B 的公共模块，删掉会直接打红测试。
- 6 份 R116 工单已按项目惯例归档在 `docs/history/worklists/`。

**结论：** 恢复 HEAD 会删掉 R116 整轮已验收的工作，属于工单明令禁止的「瞎改」。R116 产物全部保留，R117 只在其之上做增量。真正该删的只有 `backend/web/shared-blocks.css`（P1-8 选择「并入 app.css」路线后它是死文件）和根目录的 `p.bin`（探测时误存的 nginx 404 页面，162 B，非任何工单产物）。

---

## 2. 逐条状态（33 条）

| 条目 | 状态 | 证据 |
|---|---|---|
| P0-1 赛季半成品战绩覆盖 + 降级说明 | **PASS** | `TestSeasonRankWinRateFallbackDoesNotFillIncompleteScan`、`TestSeasonRankFallbackAppearsAfterRefreshSnapshotCompletes`、`TestGameplayOverviewAppliesSeasonFallbackToIncompleteSGPRanks`；**本轮补**前端披露测试 `R117 rank win-rate degradation is disclosed on screen…`（`backend/web/gameplay.test.cjs`） |
| P0-2 赛季 K/D/A 逐场平均 + 加权 overall | **PASS** | `TestSeasonStatsChampionKDAUsesPerGameAndWeightedOverall` |
| P0-4 目录 `revision_at` 不再当最近对局 | **PASS** | `TestR117ProfileUsesJSONLDStartOverDirectoryRevision` |
| P0-5 斗魂位置回退等级披露 | **PASS** | `r117.test.cjs` 断言 `renderArenaDetailContent` 内调用 `renderMeasurementTechnique` |
| P0-6 README 隐私陈述一致性 | **PASS** | `backend/readme_privacy_test.go` |
| P1-1 分模式战绩请求上界 | **PASS（本轮修竞争）** | `TestR117MultiQueueHistoryHasBoundedConcurrentIDRequests`；见第 5 节 |
| P1-2 图片队列并发 + 护栏接进流程 | **PASS（本轮接线）** | `desktop/r100-browser.cjs` 实测 `peakImages=5`；**本轮**加进 `.github/workflows/ci.yml` 的 `Real Chromium image-queue and lazy-CSS guards` 步骤 + `R117 real Chromium guards are wired into CI…` 守卫 |
| P1-3 成功 URL 缓存不得绕过闸门 | **PASS** | `R117 image queue exports a five-slot cap and keeps successful URL remounts behind admission` |
| P1-4 负缓存四态语义族 | **PASS** | `TestR117ProfileThrottleIsPendingWithoutShortTTL`、`TestR117OPGGHistoricalRanksCachesConfirmedEmptyButNotFailure`、`TestR117FacadeChallengeCatalogRetriesAfterFailureAndSharesFlight`、`TestSpecialistRunesTimeoutNegativeCacheAvoidsSecondUpstreamAttempt`；规则写进 `DESIGN.md` |
| P1-5 客户端未运行时不再空转重建 | **PASS** | `R117 app installation lookup is singleflight and hidden SSE slices flush once on resume` |
| P1-6 SSE 检查 `document.hidden` | **PASS** | 同上（隐藏期 0 次 fetch、slice 保留、恢复可见后恰好 1 次） |
| P1-7 生涯栏签名复用 | **PASS（本轮补测试）** | `R117 career column is rebuilt only when its own inputs change`（`desktop/overview-render.test.cjs`） |
| P1-8 首屏 CSS 8→5 + 懒页样式先于模块 | **PASS（本轮真机验收）** | `r115.test.cjs:14`、`R117 section loader waits for page styles…`、**本轮新增** `desktop/r117-browser.cjs` |
| P1-9 PNG 降采样 | **PASS（本轮补测试）** | `TestR117EmbeddedRasterBudgetsStayDownsampled`；实测见第 4 节 |
| P2-1 好友过期数据可见可重试 | **PASS** | `r117.test.cjs` 断言 `friends-stale-notice` + `friends-retry` |
| P2-2 相位轮询失败埋点 | **PASS** | `r117.test.cjs` 断言 `poll-failed` |
| P2-3 应用装备方案的静默路径 | **PASS** | `r117.test.cjs` 断言 `item_set_apply_request", "skipped` |
| P2-4 并发保护不得静默丢弃 | **PASS（本轮补可见性）** | `r117.test.cjs` 断言 `上一个生涯写入尚未完成，请稍候`；**本轮**修掉「toast 被确认卡完全遮住」，见 P3-7 |
| P2-5 三处请求层错误语义 | **PASS** | `api()` 的 `timeoutHint` 形参、`ResponseFormatError`、`RequestCancelled` |
| P2-6 `finally` 卡死转圈 | **PASS（本轮补测试）** | `R117 mayhem augment spinner resets even when the view changed mid-flight` |
| P2-7 弹窗兜底二次崩溃 | **PASS（本轮补测试）** | `R117 facade picker error fallback still renders when the loading node is gone` |
| P3-1 CSS 变量校验脚本 | **PASS** | `backend/web/r117-style.test.cjs`，已单独接进 CI（`Validate frontend styles and assets`） |
| P3-2 设计 token 单一来源 | **PASS** | `R117 token colors have one source definition and no bypass drift` |
| P3-3 死类名清零 | **PASS** | `R117 confirmed dead classes are absent…` |
| P3-4 预算棘轮 | **PASS（本轮下调）** | `border-radius ≤ 33`、`padding 206 → **205**`、`gap ≤ 54`、`duplicateGroups 64 → **45**` |
| P3-5 传输层三缺口 | **PASS（本轮补第三条）** | gzip `DefaultCompression`、`/api/image` ETag/304（`TestR117ImageETagRevalidation`）、**本轮**扇出负缓存（`TestR117CommunityImageFanoutNegativeCacheIsPerAssetPathAndMinuteLevel`） |
| P3-6 组合路径缓存头断言 | **PASS** | `TestR117StaticAssetSecurityHeadersPreserveRevalidation`（走 `securityHeaders(newStaticAssetHandler(...))`） |
| P3-7 zindex / 文案 / 焦点陷阱 | **PASS（本轮完成）** | 见第 3 节 |
| P3-8 `ratePercent` 猜量纲 | **PASS** | `fractionToPercent` + `TestFractionToPercentContracts` |

---

## 3. 本轮新增/完成的修复

### 3.1 P1-8 的两个阻塞点（本轮第一件事）
- `backend/web/r117.test.cjs:67` 仍在读已删除的 `shared-blocks.css` → 改为读 `app.css`。这是当时全量前端套件里**唯一**的失败项。
- `backend/web/app.css` 的共享片被**合并了两次**：一份在 `.local-status-recovery` 之后（含 suite 静态外壳与媒体查询），一份在文件尾（`champions.css:244-256` 的逐字副本）。第二份独有两条规则（`article > .recommend-icon`、`dl > div`），所以先把它们并进规范块、再删重复块，并补上从 `champions.css:391` 漏移的移动端 `.recommend-icon { width: 42px }` 覆盖。`app.css` 95,655 → 94,585 B，`container-name: suite-page` 从 2 处回到 1 处。

### 3.2 P3-5 第三条：`_large.png` 扇出负缓存
`serveCommunityDragonImage` 对 `_large.png` 会串行试 6 个上游候选，而候选级负缓存只有 5 秒 —— MEMORY 记载 657 个海克斯里有 60 个只有 `_small`，等于**每个资源每 5 秒重放 4 次上游 404**。
- `backend/community_images.go`：新增 `communityImageCandidateNegativeTTL = 60s`、`communityImageResolveNegativeTTL = 2m`、`errCommunityImageCandidatesExhausted`。
- `backend/main.go`：整组扇出包进 `a.loadAsset(ctx, "cdragon-resolved:"+assetPath, 0, 2m, …)`，**键用 assetPath**；`maxEntrySize=0` 表示成功结果仍只由 `cdragon:<remotePath>` 那层持有，不重复占用缓存预算。
- 该键随 `main.go` 的 `assetCacheGeneration++` 一起被清空，数据版本变更不会读到陈旧负结果。

### 3.3 P3-7 全部子项
- **toast 被确认卡完全遮住**（`.toast` 与 `.suite-confirm-card` 锚点都是 `fixed; right/bottom 18px`）：这条不只是样式洁癖 —— P2-4 的「上一个生涯写入尚未完成，请稍候」正好在确认流程里弹出，被遮住等于**验收判据在用户屏幕上不成立**。修法不是抬 z-index（那会让 toast 反过来盖住确认/取消按钮），而是按卡片实测高度把 toast 抬到卡片上方：`app.css` 新增 `--suite-confirm-clearance: 0px` 与 `body[data-suite-confirm-open] .toast { bottom: calc(18px + var(--suite-confirm-clearance)) }`，`suite.js` 在 `requestAnimationFrame` 里写入实测高度、关闭时清除。
- **焦点陷阱**：`requestConfirmation` 是 `<section role="dialog">` 而非原生 `<dialog>`，没有 UA 焦点收容，Tab 会跑进背景。现在 Tab / Shift+Tab 在取消与确认之间循环并 `preventDefault()`。
- **`aria-modal` 保持 `"false"`**：这是**有意**的 —— `desktop/overview-render.test.cjs` 有一条既存契约断言「确认卡不应阻塞页面其它内容」（不得出现 `[inert]`），鼠标仍能操作背景，声明成 `true` 会让读屏软件隐藏仍然可操作的内容。已在代码里写明理由。
- **`light-dark()` 清零**：全仓 0 处（本轮核查确认）。
- **z-index 魔法数**：`z-index: 10000` 已不存在，全部走 `--z-friends: 45 / --z-toast: 70 / --z-dialog: 80 / --z-recovery: 90`；全仓无 ≥3 位数字的 z-index 字面量。
- **文案收敛**：工单点名的四处超时文案（`app.js` / `gameplay.js` / `champions.js` / `suite.js`）此前已统一为「本地请求超时，请重试」；**本轮**把 `gameplay.js` 的「更多战绩**加载**失败」改为「更多战绩**读取**失败」。剩余 5 处「加载失败」全部是**脚本/样式/演示资源**的加载失败（`runtime.js`、`section-loader.js`），与「读取数据」语义不同，刻意保留，不做无差别替换。

### 3.4 P1-2：护栏接进 CI
`.github/workflows/ci.yml` 的 `quality` job 新增 `Real Chromium image-queue and lazy-CSS guards` 步骤，跑 `desktop/r100-browser.cjs` 与 `desktop/r117-browser.cjs`。**找不到浏览器时直接失败，不允许静默跳过** —— P1-2 的根因正是「脚本存在但从不在任何流程里执行，护栏失效了半年没人发现」。接线本身也有 Node 守卫（`R117 real Chromium guards are wired into CI and cannot silently skip`），摘掉接线或把「失败」改成「跳过」都会打红。

### 3.5 P1-9：预算测试
此前只有实测数据、没有测试钉住，下次换图会静默把体积带回来。新增 `TestR117EmbeddedRasterBudgetsStayDownsampled`（`backend/main_test.go`）：rank-crests 每边 ≤192、loot-icons 每边 ≤256、两者合计 <700,000 B、`app-icon.png` <85,000 B 且恰为 256×256，并断言文件数下限防止漏扫。

---

## 4. P1-9 实测数据

| 项 | HEAD（改前） | 现在 | 判据 |
|---|---|---|---|
| rank-crests + loot-icons（16 个 PNG） | 2,424,835 B | **641,630 B** | < 700 KB ✓（省 1,783,205 B ≈ 1.70 MB，工单预估 1.67 MB） |
| 单边最大像素 | 500 px（crests）/ 414 px（loot） | **192 px / 256 px** | ≤256 ✓，与工单建议的 `rank-crests@192、loot-icons@256` 一致 |
| `app-icon.png` | 95,908 B / 256×256 | **82,431 B / 256×256** | <85 KB ✓ |

- **像素一致性**：`app-icon.png` 与 HEAD 版本各自 `sips -s format bmp` 解码后逐字节比对，**262,282 B 全等、0 字节差异**，即工单要求的 `ImageChops.difference` 全零。
- **2x DPR 无退化（分析口径）**：`.rank-crest-icon` 显示 78 px，2x DPR 需要 156 px，192 px 源图有 1.23 倍余量；loot-icons 显示尺寸更小、源图 256 px。**未做截图 diff**，理由是无法在本轮复现「改前」的渲染环境，改用「源图分辨率 ≥ 2x 显示需求」这一充分条件，并在此明确标注为分析结论而非实测。

---

## 5. 由 P1-1 并发化引入的真实数据竞争（本轮修复）

全量 `go test -count=1 ./backend` 出现一次 `TestR108MultiQueuePagesMergeAndReusePrefixes: repeated queue-prefix queries: 2`（期望 3），单独跑 5 次全过 —— 典型的竞争丢自增。

根因：P1-1 把「队列数 × 页数」的串行 Riot 请求改成了**有界并发**，而这两个 R108 老测试的 transport 回调里是裸 `calls++`，并且直接在回调里调 `t.Fatal`（此时已跑在非测试 goroutine 上，`Goexit` 打在了错误的 goroutine）。CI 跑的是 `go test -race ./...`，这会被判为竞争并直接失败。

修复（`backend/r108_test.go`，**只改测试**，生产代码的并发是 P1-1 的预期行为）：
- `TestR108ModeHistoryQueriesOnlyMatchingQueuesAndCachesEmpty` 与 `TestR108MultiQueuePagesMergeAndReusePrefixes` 的计数器加 `sync.Mutex`，读取时也加锁。
- transport 回调里的 `t.Fatal*` 全部改为 `t.Errorf` + 返回空结果，避免在错误的 goroutine 上 `Goexit`（既停不住测试也可能挂住）。
- 本轮新增的 `TestR117MultiQueueHistoryHasBoundedConcurrentIDRequests` 一开始就是加锁的（它要测 `peak` 并发度）。

验证：`go test -race -count=3 -run 'TestR108ModeHistoryQueries|TestR108MultiQueuePagesMerge|TestR117MultiQueue' ./backend` ok；`go test -race -count=1 ./backend` ok（242s，全量无竞争）。

---

## 6. 对抗变异记录（工单硬门槛：改回原样必须 FAIL）

全部变异均在验证后**逐字节还原**，并用 `diff` 与哈希复核。

| 目标 | 变异 | 结果 |
|---|---|---|
| P3-5 扇出负缓存 | 摘掉 `cdragon-resolved:` 外层，恢复裸扇出循环 | **FAIL**：`all-candidate failure was not negative-cached under the assetPath key` |
| P3-5 候选 TTL | `60s` → `5s` | **FAIL**：`candidate negative TTL = 5s, want >= 1m` |
| P3-5 候选负缓存 | `negativeTTL` → `0`（只留 assetPath 级） | **FAIL**：`repeat 404 candidates = 8, want 4` |
| P3-7 焦点陷阱 | 删掉 `if (event.key === "Tab")` 整块 | **FAIL**：`确认卡打开时 Tab 必须被焦点陷阱接管` |
| P3-7 toast 避让 | 打开时不写入避让高度 | **FAIL**：`确认卡打开时必须标记 body 以抬起 toast` |
| P3-7 toast 避让 | 关闭时不清除 | **FAIL**：`确认卡关闭后必须撤销 toast 避让` |
| P1-2 CI 接线 | 从 `ci.yml` 删掉 `r117-browser.cjs` 那行 | **FAIL**：`R117 real Chromium guards are wired into CI…` |
| P1-2 CI 接线 | 把「找不到浏览器就失败」改成静默跳过 | **FAIL**：同上 |
| P1-8 共享片 | 删掉 `app.css` 里全部 `.mayhem-ranking-list` 规则 | **FAIL**（真 Chromium）：`shared .mayhem-ranking-list must already be a grid before any lazy CSS loads` |
| P1-8 共享片 | 删掉 `.suite-panel` 的容器声明 | **FAIL**（真 Chromium）：`suite container query anchor is not in the first-screen sheet` |
| P1-8 首屏集合 | 把 `champions.css` 放回 `index.html` | **FAIL**（真 Chromium）：`first-screen stylesheet set drifted` |
| P1-8 样式先行 | `section-loader` 不再 `await loadStyle` | **FAIL**（真 Chromium，3s 内）：`section loader never injected the champions stylesheet before running its module` |
| P1-7 生涯栏签名 | 签名写死成永远命中 | **FAIL**：`ranks 变了必须重建生涯栏`（工单点名的这条变异） |
| P1-7 生涯栏签名 | 完全不做签名比较（每帧全量重建） | **FAIL**：`只改 matches 不得重建生涯栏，实际渲染 6 次` |
| P2-6 转圈复位 | `finally` 恢复成 `if (!current()) return; loading = false` | **FAIL**：`陈旧响应不得把 mayhemAugmentLoading 永久留在 true` |
| P2-7 弹窗兜底 | 兜底改回 `querySelector(".facade-picker-loading").textContent = …` | **FAIL**：复现修复前的 `TypeError: Cannot set properties of null` |
| P0-1 前端披露 | `recordKnown` 恒为 true（半成品也渲染胜率） | **FAIL** |
| P0-1 前端披露 | 丢掉 `rankedCapability?.detail`，只留兜底文案 | **FAIL**：`capability.detail 必须落到屏幕上` |
| P0-1 前端披露 | `detail` 为空时不再兜底（tooltip 留空） | **FAIL**（工单点名的「把 Detail 改成空串必须有测试察觉」） |
| P1-9 资源预算 | rank-crests / loot-icons 换回 HEAD 的 500px / 414px 原图 | **FAIL**：`web/rank-crests/bronze.png dimensions = 500x500, want at most 192px per edge` |
| P1-9 资源预算 | `app-icon.png` 换回 HEAD 版本 | **FAIL**：`app-icon.png = 95908 bytes, want < 85000` |

---

## 7. 真 Chromium 验收（P1-8 判据 2/3/4）

新增 `desktop/r117-browser.cjs`（无 npm 依赖，CDP + 本地 fixture server，`CHROME_BIN` 可覆盖）。采样器同时用 `requestAnimationFrame` 与 `MutationObserver`：**只用 rAF 会漏掉极短的 loading 窗口**（suite 只补一个模块时可能一帧内走完），MutationObserver 在 notice 插入的同一个微任务里补帧，采样不再依赖帧率。装采样器与点击 tab 放在**同一个任务**里，否则切换前的帧会被算进本次切换。

实测（`Emulation` 1500×1050，dark）：
- 首屏样式表恰好 5 张：`/app.css /build-item-row.css /gameplay.css /friends.css /metrics.css`，且 `link[data-section-style]` 为 null。
- **反证**：切换前 `#champions-panel .champions-skeleton` 的计算 `display` 不是 `grid`，证明 champions.css 确实还没生效，后面的共享片断言才有意义。
- 共享片在对局页首屏即生效：注入 `.mayhem-ranking-list` 探针，`display: grid`、两列并排（`581.5px 581.5px`）、行 `min-height: 70px`。
- `#suite-panel` 首屏 `container-type: inline-size`（判据 3）。
- 英雄页切换：38 帧，`link[data-section-style="champions"]` 的 href 为 `/champions.css`，切换后 `.champions-skeleton` 计算 `display: grid` / `.champion-table` 计算 `table-layout: fixed`（判据 2）。
- 工具页切换：37 帧，其中确有 `loading === "suite"` 的帧，且**每一帧** `container-type` 都是 `inline-size`（判据 3 的「期间」）。
- 无样式帧 = 0（判据 4）。口径说明：**加载态期间**允许静态骨架屏未上样式，因为 `section-loader` 的顺序是「先插 notice，再 await 样式与模块」，那段时间面板显示的本来就是「正在加载页面…」；断言收敛为「加载态之外不得出现无样式帧」。
- 稳定性：连续 4 次运行全 PASS，帧数稳定（38~39 / 34~37）。

`desktop/r100-browser.cjs` 复跑：`R100 Chromium PASS {"elapsed":2,"peakImages":5}` —— `peakImages` 与生产 `IMAGE_QUEUE_LIMIT` 一致，且该值是从 `backend/web/image-queue.js` 解析出来的，不会再漂移。

---

## 8. 未做 / 降级项（明确标注，不以推断值代替证据）

1. **P1-9 的截图 diff 未做**，改为「源图分辨率 ≥ 2x 显示需求」的分析口径（见第 4 节）。`app-icon.png` 的像素一致性是**实测**的（0 字节差异）。
2. **P3-7 的文案未做无差别收敛**。剩余 5 处「加载失败」都是脚本/样式/演示资源的加载失败，与「读取数据」语义不同；工单该条为「建议」且 P3-7 无验收判据，强行替换会让措辞变差。
3. **`.suite-confirm-card` 仍不是原生 `<dialog>`**，只加了键盘焦点陷阱，没有做 `inert` 背景。原因：`desktop/overview-render.test.cjs` 有既存契约「确认卡不应阻塞页面其它内容」，改模态会打红它，且超出 P3-7（最低优先级、无验收判据）的范围。
4. **未改 `AGENTS.md` 的「当前版本：0.12.7」**。R116-A 台账已记录该文件不在其可改清单内；R117 工单也未列入。属于文档滞后，此处标注。
5. **未把浏览器脚本接进 `build-desktop.sh` / `build-desktop-windows.ps1`**。工单允许「接进 CI **或** 至少接进发布门禁」，已选 CI；再改发布脚本会动到用户自己跑的打包流程，属超范围。

---

## 9. 执行事故与恢复（如实记录）

验证 P1-8 变异 C 时，我用 `git checkout -- backend/web/index.html` 还原，**这会把该文件整体回退到 HEAD**，抹掉 P1-8 对 `index.html` 的全部改动。

恢复方式：本会话早前执行过 `go build -o "$PI_SCRATCH_DIR/dl-backend" ./backend`，Go 的 `embed.FS` 里存着改动后的 `index.html`；从二进制中定位 `<!DOCTYPE html>` 抽出 56,777 B 写回。核对结果：与 HEAD 的差异**恰好是被 P1-8 移除的 3 行懒页样式表**（`champions.css` / `pro-players.css` / `suite.css`），无其他损失；`r115.test.cjs`、`r117.test.cjs`、`r117-style.test.cjs` 共 17 项与真 Chromium 脚本全部复跑通过。

**教训（建议写进纪律）：还原变异只能用「反向的定点编辑」，绝不用 `git checkout -- <file>`** —— 在一个有大量未提交成果的工作区里，它等价于删档。本轮其余全部变异均改用备份 + `diff` 复核，并用 SHA-256 校验了 16 个 PNG 与 `app-icon.png` 的逐字节还原。

---

## 10. 复现命令

```bash
gofmt -l backend/*.go                                  # 空
go vet ./backend                                       # 空
go test -count=1 ./backend                             # ok  ~213s
go test -race -count=1 ./backend                       # ok  ~242s（CI 口径）
node --test backend/web/*.test.cjs desktop/*.test.cjs  # 805 / 804 pass / 0 fail / 1 skipped
node desktop/r100-browser.cjs                          # R100 Chromium PASS peakImages=5
node desktop/r117-browser.cjs                          # R117 Chromium PASS unstyledFrames=0
```

工作区卫生：`p.bin` 与 `backend/web/shared-blocks.css` 已删除；`shared-blocks` 仅作为 `r115.test.cjs:14` 的**反向**断言存在（断言它不得被 link）。

---

## 11. 独立验收整改（2026-09-21，追加）

独立验收会话复跑全量测试确认数字吻合、生产代码修复真实落地，但用四路对抗变异揪出 **7 处「测试 PASS 但断言写法拦不住回归」的假护栏**（另追加 P3-2/P3-3 两条同类低优先级项）。9 条全部整改完毕，每条都用对抗变异重新证明。

| # | 条目 | 假护栏的成因 | 整改 | 变异验证 |
|---|---|---|---|---|
| 1 | P0-4 | fixture 里 `old.LastMatchAt` 与 directory revision 是**同一时刻**，`readProProfile` 第一步的 `proRealLastMatchAt` 直接把 `LastMatchAtKnown` 重置为 false，合并逻辑根本没被测到 | `old.LastMatchAt` 改成 `2026-09-16T23:30:00Z`：既不等于 revision，又**晚于** JSON-LD 的 `18:19:58Z`，于是「取大值」与「按来源优先」结果不同 | 合并逻辑改回取大值 → **FAIL**（`JSON-LD startTime must win over a later directory/old timestamp`） |
| 2 | P1-1 | 只断言 `peak>=2` 和 `calls<=12`，从未断言并发上界；且信号量容量是字面量 4 | 提为具名常量 `riotOverviewMaxConcurrentIDRequests`；测试里**硬编码审计值 4**（不能引用生产常量，否则改常量等于移动球门），并分别断言常量未漂移、实测 peak 未越界 | 容量改 100000 → **FAIL**（bound drifted）；绕过常量把信号量写成 8 → **FAIL**（`peak=8 > 4`） |
| 3 | P1-2 | CI 守卫只是 4 条正则子串匹配，加一行 `continue-on-error: true` 照样 PASS | 改用 `js-yaml` **结构化解析** `ci.yml`：按名字定位步骤，断言无 `continue-on-error`、无 `if:`、无 `set +e`，两个脚本各自那一行不得被 `\|\| true` 软化，「找不到浏览器」必须是 `exit 1` 硬失败；并遍历整个 job 禁止任何步骤吞失败 | 给该步骤加 `continue-on-error: true` → **FAIL** |
| 4 | P1-8 | 独立验收连跑 8 次全 PASS：`<link>` 创建是同步的，去掉 `await` 只是不等 onload，DOM 存在性检查测不出时序回归 | fixture server 把 `/champions.css` **拖慢 350ms**，再用 Resource Timing 断言 `champions.js` 的 `startTime >= champions.css` 的 `responseEnd`；并先断言 CSS 确实被拖慢 ≥200ms，防止断言变成空转 | 去掉 `await Promise.all(...loadStyle)` → 连跑 3 次**全部 FAIL**（`cssEnd=1460.9 jsStart=1107.4`） |
| 5 | P2-3 | `gameplay.js` 两处都打相同的 `"item_set_apply_request","skipped"`，子串正则命中任意一处就过 | 用 `functionSource(gameplay,'applyItemSet')` 限定作用域，断言 skipped 埋点**恰好 2 处**，并分别钉住 `reason: "missing-self-or-build"` 与 `blockCount/reason` 那一处，外加 submitted 埋点 | 删掉工单真正要保护的那处（原 `:6124`）→ **FAIL**（`必须保留两处 skipped 埋点，实际 1`） |
| 6 | P2-4 | 两处相同文案，且**没有任何测试真的构造重入场景** | 源码侧断言重入守卫恰好 2 处且分别落在 `applyFacade` / `applyFacadeIdentity`；**新增行为测试**：`facadeApplying=true` 时两个入口各自返回 false、各自弹一次 toast、0 次写请求、不推进 token、不动在途锁 | 删掉 `applyFacadeIdentity` 的重入提示 → **FAIL 2 项**（源码计数 + 行为测试） |
| 7 | P2-5c | `/const IMAGE_QUEUE_LIMIT\|RequestCancelled\|本地请求超时，请重试/` 是**无分组顶级或**，删掉 `RequestCancelled` 本体也能靠另一分支蒙过 | 拆成独立断言（哨兵命名、放行顺序、超时文案、不得再出现「联网读取超时」），并断言放行分支**排在**超时分支之前；**新增行为测试**：同 key 连续两次 `api()`，第一个 promise 必须以 `RequestCancelled` 收场、埋点记 `errorKind=canceled` 而非 `timeout`、第二个调用正常返回 | 把 AbortError 抹平成「联网读取超时」（修复前的真实形状）→ **FAIL 2 项**（源码断言 + 行为测试） |
| 8 | P3-2 | 23 个 hex 的固定白名单，白名单之外的新重复测不出来 | 补**全量扫描棘轮**：扫遍所有 CSS 统计每个 hex 出现次数，断言「出现 >1 次的 hex 种类数 ≤ 35」；白名单本身仍逐条断言恰好 1 次 | 新增一个白名单外的重复 hex → **FAIL**（`increased: 37`） |
| 9 | P3-3 | 20 个类名的固定白名单，同上 | 补**全量扫描棘轮**：CSS 里定义的全部 1384 个类名逐个回查 JS/HTML 标识符，断言「未被引用数 ≤ 93」；20 个已确认死类名仍逐条断言不存在 | 新增 3 个白名单外死类名 → **FAIL** |

**棘轮阈值的诚实说明**：35 与 93 都不是 0。`#000`/`#FFF` 这类基础色在多个文件里合法重复；93 个未引用类名里大部分是 `is-0`/`is-S`/`category-*`/`loot-*` 这类字符串拼接出来的动态类名。这两条棘轮**只保证不再变差**，不等于证明了「零重复 / 零死类名」，已按此口径写进测试注释。

**方法论已落进代码注释**：`assert.match(source, /A|B|C/)` 这种无分组顶级或、以及「同一文案在文件里出现两次以上时的子串断言」，都在对应测试里写了为什么不能那么写。断言上界值必须硬编码在测试里，不能引用被测的生产常量。

### 整改后全量验证
```
gofmt -l backend/*.go                                  # 空
go vet ./backend                                       # 空
go test -count=1 ./backend                             # ok  215s
node --test backend/web/*.test.cjs desktop/*.test.cjs  # 808 / 807 pass / 0 fail / 1 skipped
node desktop/r100-browser.cjs                          # PASS peakImages=5
node desktop/r117-browser.cjs                          # PASS unstyledFrames=0 styleBeforeModule{cssEnd:1515,jsStart:1516}
```
测试数从 805 增至 808（新增 P2-4 重入、P2-5c 取消哨兵、P3-2/P3-3 全量棘轮共 3 条；P1-2 的 CI 守卫与 P1-8 的时序断言是在既有用例内加强的）。

---

## 12. 第二轮独立验收的 5 处残余缺口（待执行，含精确修复方案）

第二轮用**不同角度**的新变异复测第 11 节的 9 处整改：P0-4、P1-1、P1-8、P3-3 经得住（真修复）；以下 5 处仍有残余缺口。**结论性教训：「改回原样必须 FAIL」只证明了没被这一种退化绕过，不等于堵住了这一类退化。**

| # | 缺口 | 精确修复方案 | 必须补的新变异 |
|---|---|---|---|
| 3 | **P1-2**：`r117.test.cjs` 的 `/\|\|\s*true\|\bif\b\|;\s*true$/` 只认字面 `\|\| true`，`\|\| echo "skipped"`、`\|\| :` 等 shell 级软化测不出，而在 GitHub Actions 的 bash 默认行为下确实会静默吞掉 Chromium 护栏失败 | 改成**白名单式结构断言**：把 `guard.run` 按行拆开，取出含 `node desktop/r1xx-browser.cjs` 的行，断言该行**整行严格等于** `CHROME_BIN="$chrome" R1xx_BROWSER_OUTPUT="$RUNNER_TEMP/r1xx-browser" node desktop/r1xx-browser.cjs`（去掉首尾空白后），不允许任何后缀。再补一条：整个 `run` 块里不得出现 `||`、`&&`、`if`、`:` 空命令、`set +e`、`2>/dev/null` 中任何一个 | (a) `\|\| echo "skipped"`；(b) `\|\| :`；(c) 行尾追加 `; true`；(d) `set +e` 前置 |
| 5 | **P2-3**：作用域只限定到 `applyItemSet` 函数体，没限定到分支。把两处埋点位置互换/错配（分支 A 变零埋点）时 243 项全过 | 断言**分支内**的埋点：用 `functionSource` 取出函数体后，按 `if (!self || !build) {` 与 `if (!payload.championId || !payload.blocks.length) {` 切成两段，分别断言第一段含 `reason: "missing-self-or-build"`、第二段含 `blockCount: payload.blocks.length, reason`，且**两段各自的 skipped 计数都恰好为 1** | 把两个分支的埋点互换；把第一个分支的埋点删掉只留第二个 |
| 6 | **P2-4**：`applyFacadeIdentity` 改成**委托**调用 `applyFacade` 时，行为测试照样 PASS（文案仍对），只剩旧源码正则在兜底。若以后"清理"掉旧正则，这类回归就无人看守 | 让行为测试能区分「独立守卫」与「委托」：断言重入时 `applyFacade` 的**副作用计数为 0**——委托会让 `applyFacade` 的 `state.facadeRequestToken`/`api` 调用等被触达。具体做法：给 `applyFacadeIdentity` 注入一个可计数的 `applyFacade` 桩，断言重入路径下该桩调用次数为 0，并断言两个函数各自独立持有 `if (state.facadeApplying)` 守卫（源码侧保留计数=2 的断言，并在注释里写明它是委托类回归的唯一护栏，不得删） | 委托改写；以及「只删源码正则」这条元变异（应让委托变异从 FAIL 变 PASS，用来证明源码正则不可删） |
| 7 | **P2-5c**：`RequestCancelled` 有**两个产生点**（① AbortError 分支；② fetch 成功但 `state.requests.get(key) !== controller` 的取代分支），行为测试的 mock 只触发了 ① | 行为测试补第二个场景：让 fetch 正常 resolve，但在 resolve 之前用另一个 controller 覆盖 `state.requests` 里的同 key 条目，断言第一个 promise 仍以 `RequestCancelled` 收场且埋点 `errorKind=canceled`。两条路径各自独立断言 | 分别破坏 ① 和 ②，各自必须 FAIL |
| 8 | **P3-2**：棘轮只扫 `.css`，在 **JS 里**硬编码一份已在白名单里的 hex 测不出，「单一来源」的承诺范围小于实际检查范围（低优先级） | 把 hex 扫描范围从 `cssText` 扩到 `cssText + sourceText`（`.js`/`.html`），预算阈值按实测重新审计后写入；`tokenHexes` 那 23 个仍断言全仓恰好 1 次 | 在某个 `.js` 里硬编码 `#E3B341` |

**执行注意**：#3 与 #5 是纯测试改动；#6/#7 需要新增行为场景（#6 要给 `applyFacadeIdentity` 注入 `applyFacade` 桩）；#8 扩范围后阈值会变，必须先实测再定值，不要沿用 35。全部修完后按第 11 节的口径逐条做对抗变异，并复跑 `go test -race`、`node --test`、两个真 Chromium 脚本。

**第二轮另披露**：一路验收子代理曾手误用 `Edit` 改到真实工作目录的 `section-loader.js`，当场改回；验收方已逐字核对该文件与本轮开始前一致。本会话亦已用 `diff` 复核 `backend/web/section-loader.js` 与备份一致。

### 本轮执行记录（2026-09-21）

**测试改动：**仅修改第 12 节指定的四个测试文件，并在本节追加结果。P2-3 除按真实 if 块统计埋点、校验 reason 与 return 顺序外，增加了两条拒绝分支的行为测试，实际执行生产函数体并断言各自上报 skipped、零写请求。P2-4 注入可计数的 `applyFacade` 桩，重入时要求调用数为 0；源码侧继续保留两个独立守卫计数。

**对抗变异：**

| 条目 | 变异结果 |
|---|---|
| P1-2 | `|| echo "skipped"`、`|| :`、`; true`、前置 `set +e` 四种变异均 **FAIL**；CI run 块逐行白名单命中。 |
| P2-3 | 两分支埋点互换、删除第一埋点、埋点移到 `return` 后均 **FAIL**；把任一分支埋点包入 `if (false)` 时静态契约通过、对应行为测试 **FAIL**。 |
| P2-4 | 委托改写且源码计数保留时静态契约 **FAIL**（守卫数 1）；元变异删除该计数后 R117 静态用例 **PASS**，但注入桩行为测试仍 **FAIL**（委托调用数 1）。因此表内“委托变异从 FAIL 变 PASS”仅成立于静态用例，不代表完整测试放行；两层护栏均保留。 |
| P2-5c | 破坏 AbortError 转换时仅路径①用例 **FAIL**；移除请求 Map 身份检查时仅成功 fetch 被取代路径②用例 **FAIL**。 |
| P3-2 | JS 临时加入 `#E3B341` 后，token 计数为 2 且重复 hex 种类升至 42（上限 41），相关测试 **FAIL**。当前实测范围为 8 CSS + 14 JS/HTML，基线仍为 41。 |

所有临时变异均用反向定点编辑恢复；`.github/workflows/ci.yml`、`gameplay.js`、`suite.js`、`champions.js`、`image-queue.js` 及 P2-4 元变异测试文件分别与当时快照执行 `diff -u`，输出均为空；未使用 `git checkout`。执行期间 `gameplay.js` 与 `champions.js` 出现并发 R116-E 增量，均完整保留；对应变异恢复以增量出现后的 live 快照为基准复核。

**最终验证：**

- `gofmt -l backend/*.go`：空。
- `go build -o "$PI_SCRATCH_DIR/r117-backend" ./backend`：PASS，产物写入 scratch。
- `go vet ./backend`：未通过，Go 测试编译引用未定义的 `legacyRankedQueueTabs`（`backend/gameplay_test.go:1017,1036`、`backend/player_ability_test.go:212,237`）。
- `go test -race -count=1 ./backend`：同一未定义符号导致编译失败，Go 测试未执行；未改动范围外 Go 测试文件。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：**870 项 / 869 通过 / 0 失败 / 1 skipped**（Windows/PowerShell 专属项）。
- `node desktop/r100-browser.cjs`：**PASS**，`peakImages=5`。
- `node desktop/r117-browser.cjs`：**PASS**，`unstyledFrames=0`，CSS 先于模块加载。

构建检查时 `desktop/package.json` 版本为 **0.12.11**；本台账开头记录的 0.12.8 未在本轮改动，以遵守测试与第 12 节范围限制。
