# R209 执行账本

日期：2026-10-04。工单：[R209](WORKLIST-R209-COLLECTION-SKIN-CARD-ART-STUCK-LOADING-SLOT-LEAK.md)。基于 R206/R207/R208 之后的当前工作区，desktop/package.json 与 lockfile 沿用 **0.12.71**，不单独递增。按用户暂停指示和工单收尾要求，本轮不构建、不打包、不发布桌面程序。

## 已确认的问题与修复

- 第一层旧队列没有 `queued` 去重标记，同一个等待任务可由观察器重复加入队列；旧 pump 也不排除已经 active 的任务，因此同一个 job 可能重复占用名额，而完成时只释放一次。本轮同时增加入队去重与 pump 的 active 排除。行为测试重复入队五次仍只保留一个任务，移除去重保护的变异会断言失败。
- 旧任务只保存在 WeakMap 中，无法枚举实际活动任务，计数因此不能自行核对。新增未完成任务 Set，pump 按 `job.active && !job.done && job.image.isConnected` 重算总名额和远程臻彩名额；脱离 DOM 的活动任务取消、清看门狗和图片回调、释放名额。前后计数有变化才记录 `card_image_slot_reconciled`。保留全局 8 个、远程臻彩 2 个的并发限制。
- 容器批量取消时先完成全部取消，再统一 pump，避免逐张取消期间启动同一容器内即将被取消的任务。重新给同一个图片创建任务前也收尾旧任务；没有连接到文档的未完成任务两秒后清理，完成后从 Set 删除，任务清空时停止健康检查定时器。
- 新增两秒健康检查：可见且仍 pending 的卡片直接入队，记录 `card_image_observer_fallback` 的数量。保留观察器原有 620px 预取范围和出范围撤回；兜底只使用真实可见区域，并裁剪炫彩横向卡片容器，隐藏、屏幕外、脱离 DOM 的卡片不会由兜底启动。
- appScroll 内的卡片保持既有 root；外部卡片使用最近的 overflow auto/scroll 祖先，无该祖先则用 viewport。任务先创建后挂载，或移动到外部容器时，健康检查更新 observer，并忽略旧 observer 的迟到回调。独立复核提出改为所有卡片都查最近滚动祖先；本轮按工单“卡片不在 appScroll 内时”要求处理，未扩大到改写内部炫彩观察策略，内部横向卡片由可见性裁剪兜底覆盖。

工单中的旧日志缺少第一层计数，不能据此确定原用户现象究竟命中了名额泄漏还是观察器未回调。本轮修复了可复现的重复入队缺陷，并对两类卡死都提供恢复和后续诊断；未把源码复现等同于原日志的唯一根因。

## 容器替换检查表

以下位置为本轮最终 app.js 行号。所有列出的重建入口已经有取消调用，没有发现需要另补取消的分支；保留现有统一入口，避免重复取消。

| 路径与位置 | 是否已取消 | 处理方式 |
|---|---|---|
| `renderItems`，1087–1089；el.grid 骨架、收藏准备中、读取失败、空态与普通卡片重建 | 是 | 入口先取消 render frames 和 `cancelDeferredImages(el.grid)`，之后才清空/替换内容 |
| `renderChromaCatalog`，1218；炫彩分组/横向卡片容器重建 | 是 | 由 renderItems 统一取消后进入炫彩渲染；applyChromaVisibility（3579）只改可见性，不替换 DOM |
| `renderPoolCatalog`，2330–2332；奖池骨架、失败、空态和卡片重建 | 是 | 入口先取消 pool render frames 和 `cancelDeferredImages(el.poolSkinGrid)` |
| `activateSection`，2498；离开收藏时 2514、2518 | 是 | 取消主网格和奖池网格，回到收藏时由渲染入口重新建任务 |
| `activateFavoritesPage`，2622；子页切换 2631、2633 | 是 | 切换前取消两个网格任务 |
| `loadSkins`，634；R203/R206 保持可见刷新 | 已区分保留/替换 | keepVisible 与 unchanged-suppressed 保留既有 DOM/任务，真实变化交给 renderItems 统一取消后重建 |
| `loadPoolCatalog`，2272；保持可见奖池刷新 | 已区分保留/替换 | 签名不变不重建，变化交给 renderPoolCatalog 统一取消后重建 |
| 筛选、搜索、排序 | 是 | 走 renderItems/renderPoolCatalog 的同一重建入口 |

## 诊断与隐私边界

- 收藏页可见且有 pending/queued 超过五秒时，`collection_card_image_state` 每三十秒最多一条。落盘字段完整：`active_count`、`active_jobs`、`queued`、`pending_observed`、`visible_pending`、`observer_root_ok`、`oldest_active_age_ms`；observer_root_ok 按工单表示卡片是否位于 appScroll 内。
- `card_image_slot_reconciled` 记录总名额与远程臻彩名额修正前后值；`card_image_observer_fallback` 记录实际兜底入队数量。
- runtime.js 放行三个事件及数值/布尔字段，features.go 注册事件与 reason，并做落盘边界检查。请求仍拒绝未知字段；不新增 URL、图片路径、PUUID 等资源或身份字段。诊断不添加界面说明文案。

## 自动验证

- 新增完整 jsdom 回放执行真实 app.js、runtime.js 和 image-queue.js：7/7 通过，见 [node-r209.log](history/reports/r209/node-r209.log)。覆盖八张活动卡片直接移除后恢复新屏加载且仅记一次修正；保持可见刷新、全部/已拥有/炫彩切换、打开关闭炫彩、离开返回，每步计数等于实际活动任务；正常生命周期没有修正诊断；观察器无回调时 1999ms 不启动、2000ms 启动；重复入队；等待诊断限频；外部 root 与隐藏/屏幕外卡片；真实诊断传输字段与隐私过滤。
- Go 新增三个事件实际 handler 落盘与截断、未知隐私字段和 reason 拒绝测试；连同 R130 与现有诊断定向回归通过，见 [go-targeted.log](history/reports/r209/go-targeted.log)。
- 三项变异都触发行为断言失败，非语法/引用错误：去掉名额自愈 → 泄漏回放 FAIL；去掉观察器兜底 → 无回调回放 FAIL；去掉 queued 去重 → 重复入队回放 FAIL。只在临时源码副本变异，见 [mutations.json](history/reports/r209/mutations.json) 及同目录日志。
- `go test ./backend -count=1`：通过，257.552 秒，见 [go-full.log](history/reports/r209/go-full.log)。
- `go vet ./backend`：通过，见 [go-vet.log](history/reports/r209/go-vet.log)。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`（tap 报告格式）：1170 项，1169 通过、1 项平台门禁跳过、0 失败，286.278 秒，见 [node-full.log](history/reports/r209/node-full.log)。包含当前 R207/R208 改动的回归。
- 最终 `git diff --check` 通过；命令退出码及版本核对见 [checks.json](history/reports/r209/checks.json)。全量验证后仅整理代码注释与文档，无行为改动。

## 构建、发布与真机边界

实现完成，保留工单进行中。桌面构建、打包与发布等待用户恢复后与 R206/R207/R208 合并，届时按 R199 核对发布证据，本轮没有新增构建产物或 Release，也没有将工单标为已发布/已关闭。

Windows 真机仍待用户验收：打完几局后多次进出收藏、切换已拥有/全部皮肤确认原画加载；导出日志核对是否触发名额修正或观察器兜底及其计数。自动回放不替代真机运行证据。
