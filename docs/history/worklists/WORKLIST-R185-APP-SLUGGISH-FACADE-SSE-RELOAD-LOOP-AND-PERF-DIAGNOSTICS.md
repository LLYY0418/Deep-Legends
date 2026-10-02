# WORKLIST-R185：软件所有页面变卡变慢（生涯页每 2 秒整包重载 + 缺少性能诊断）

诊断人：Claude（读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-02。基线：源码 0.12.48（指纹 14c6cc553556）。如果 R184 已合入，以合入后的源码为准，两者不冲突。

## 证据范围

日志 `lol-loot-diagnostics-1002-1606.jsonl`。0.12.48 只有一次运行（run `8803f499…`），从 06:28 连续运行到 08:06Z（约 1 小时 40 分钟），期间打了 5 局。

**现有日志里完全没有前端卡顿、内存、CPU 方面的指标**（仓库里没有 longtask、`performance.memory`、`getAppMetrics`、`runtime.ReadMemStats` 之类的采集）。所以"所有页面都卡"的完整原因，现有日志不能直接证明。下面 P1 是日志里能确定的一个大问题；P2 补上性能诊断，下一份日志可以直接定位。

## P1　生涯页每 2 秒整包重载一次，持续 32 分钟

### 证据

- `facade_load_cost` 共 967 条，其中 **958 条 `trigger=sse`**。从 07:13:09 到 07:48:14，**每分钟 30 条**，也就是每 2 秒一次，没有间断。以前的日志（1001 当天五份、1002-0119）每份只有 2–7 条，都是 `poll`。这是新出现的情况。
- 同一时段（07:20–07:45）后端每 2 秒对客户端发 5 个请求：`summoner-profile`、`/lol-chat/v1/me`、`regalia`、`/lol-challenges/v1/summary-player-data/local-player`（最慢 1108 ms）、`/lol-collections/.../backdrop`。25 分钟内每个约 750 次。
- 每次加载 `catalog_ms=0`（皮肤目录有缓存），但 `/api/facade/state` 的返回体每次都带上**全部 2125 个皮肤**（`facade_skin_state skin_count=2125`）。
- 这段时间是大厅里，用户没有操作生涯页（只有 07:16 的两次 `facade_apply`），说明是客户端事件驱动的。

### 根因（源码已核对）

1. 后端 `connection_manager.go:514` `isFacadeLCUEvent`：只要客户端推送 `/lol-chat/v1/me`、`/lol-challenges/v1/summary-player-data`、`/lol-regalia/...`、`/lol-collections/.../backdrop` 任一事件，就 `broadcastEvent("facade:changed")`。唯一的限制是 2 秒节流（`facadeEventThrottleInterval`）。客户端在大厅里会频繁推送 `/lol-chat/v1/me`（在线状态里的时间戳等字段一直在变），所以节流后正好每 2 秒广播一次。**后端没有判断生涯页关心的字段到底有没有变。**
2. 前端 `suite.js:1793` `refreshFacadeFromEvent` → `loadFacade(true, …, "sse")`：每次都强制请求 `/api/facade/state`，拿回 2125 个皮肤，然后 `suite.js:1690` `facadeRenderSignature` 对新旧两份各自 map 一遍 2125 个皮肤，再分别 `JSON.stringify` 比较。数据没变时不重绘，但**下载、解析、两次 map、两次大字符串序列化**每 2 秒都在主线程上发生一次。
3. 后端 `facade_view_cache.go:29` 在 `force=true` 时不走 30 秒缓存，每次都重新读那 5 个接口（挑战数据最慢超过 1 秒）。

### 修复

P1-1　**后端只在生涯页关心的字段变了才广播。** `handleFacadeLCUEvent` 按 URI 分别算指纹，只取 `facadeRenderSignature` 用到的字段：
- `/lol-chat/v1/me`：`icon`、`availability`、`statusMessage`、`lol.rankedLeagueQueue/Tier/Division`；
- 挑战：称号（`title`）和展示中的挑战 ID 列表；
- regalia、backdrop：事件体整体哈希。

指纹与上次相同就不广播（不计入节流）。事件体解析失败时按"已变化"处理（保守）。

P1-2　**事件触发的刷新不带皮肤列表。** `/api/facade/state` 增加 `skins=0` 参数：只返回身份、资料、聊天、挑战等，`skins` 字段省略。前端 `sse` 触发时带 `skins=0`，合并时保留已有皮肤列表（现有"空列表时沿用旧列表"的逻辑改成"没带皮肤时沿用旧列表"）。`manual`、`poll`（含 30 秒过期）仍然取完整数据。

P1-3　**比较不再每次序列化皮肤。** `facadeRenderSignature` 的皮肤部分改成用数量 + 每个皮肤关键字段的轻量哈希，并缓存在 state 上（皮肤列表没换就不重算）；`sse` 触发、没带皮肤时直接跳过皮肤比较。

P1-4　**事件刷新的最小间隔改为 5 秒。** 后端节流 `facadeEventThrottleInterval` 从 2 秒改为 5 秒。手动刷新和应用操作后的确认读取不受影响。

P1-5　诊断：后端每 60 秒汇总一条 `facade_event_source`：按 URI 前缀统计收到的事件数、指纹没变被忽略的次数、实际广播次数。只在这 60 秒内有事件时才记。

## P2　性能诊断：下一份日志能直接看出哪里卡

所有采集每 60 秒汇总一条，不记身份。

P2-1　**渲染进程（前端）`renderer_perf`**：
- 用 `PerformanceObserver({type: "longtask"})` 统计 50 ms 以上的长任务：`longtask_count`、`longtask_total_ms`、`longtask_max_ms`，并按当时所在页面（`section`、`tab`）分组；
- `heap_used_mb` / `heap_limit_mb`（`performance.memory`，Electron 可用；不可用就不写）；
- `dom_nodes`（`document.getElementsByTagName("*").length`）、`img_count`；
- 当前页面上 `setInterval` / 轮询类计时器的数量（如果能统计；不能就省略）；
- 主线程卡顿检测：每 1 秒的计时器，实际间隔超过 1.2 秒记一次，统计 `timer_lag_count` / `timer_lag_max_ms`。

只在本分钟有长任务、或每 5 分钟固定一次时记录，避免日志膨胀。

P2-2　**Electron 主进程 `desktop_process_metrics`**：`app.getAppMetrics()`，按进程类型（Browser / Renderer / GPU / Utility）记 `cpu_percent` 和 `working_set_mb`；同时记后端子进程的内存（能取到就记）。每 60 秒一条。

P2-3　**Go 后端 `backend_runtime_metrics`**：`runtime.NumGoroutine()`、`MemStats` 的 `HeapAlloc`、`HeapInuse`、`NumGC`、`PauseTotalNs` 增量；当前 SSE 连接数；进行中的本地 HTTP 请求数。每 60 秒一条。

P2-4　**本地请求耗时补全**：`local_request_client` 目前只覆盖 status / image 等少数入口。把各页面主数据请求（总览、收藏、生涯、对局、套件各页签）都纳入，记 `endpoint`、`duration_ms`、`response_bytes`（能取到就记）。

## 测试

Go：

1. `/lol-chat/v1/me` 事件只有时间戳变化、关心的字段不变 → 不广播；`statusMessage` 变化 → 广播一次。
2. 连续 30 个只有时间戳变化的事件 → 广播 0 次，`facade_event_source` 记录 `ignored=30`、`broadcast=0`。
3. 节流间隔 5 秒：5 秒内两次真实变化 → 只立即广播一次，结束时补发一次（沿用现有补发逻辑）。
4. `/api/facade/state?skins=0` 不含 `skins` 字段，其他字段与完整请求一致；不带参数时返回完整皮肤列表。
5. `backend_runtime_metrics` 字段齐全，每 60 秒一条；测试里用注入的时钟。

Node：

6. `sse` 触发的刷新请求带 `skins=0`，返回后 `state.facade.skins` 仍是原来的 2125 个；不重绘。
7. 签名比较：皮肤列表没换时不重新遍历皮肤（用计数器断言 map 调用次数）。
8. `renderer_perf`：模拟 longtask 条目，按页面分组统计正确；没有长任务时 5 分钟内只记一次。
9. 主线程卡顿检测：模拟计时器延迟 1.5 秒 → `timer_lag_count=1`。

变异（overlay，必须断言 FAIL）：

| 变异 | 期望 |
|---|---|
| 后端不比较指纹，事件就广播 | 测试 1、2 FAIL |
| `sse` 刷新仍请求完整皮肤 | 测试 6 FAIL |
| 签名每次都遍历皮肤 | 测试 7 FAIL |

`node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。

## 收尾

- 版本递增；`docs/WORKLIST-INDEX.md` 加 R185；账本 `docs/history/ledgers/r185-execution-ledger.md`。
- 账本写明：现有日志没有前端性能指标，本工单只修了日志能确认的生涯页重载问题；"所有页面都卡"需要下一份带 P2 诊断的日志再确认是否还有其他原因。
- 不新增界面文字。

## 真机验收（用户）

1. 打开软件后像平时一样使用，**连续用 1 小时以上**（包括在大厅停留、打几局、切换各个页面）。
2. 停在生涯页时，页面不再每隔一两秒闪一下或变卡。
3. 觉得卡的时候，记下大概时间和当时在哪个页面，然后不关软件，导出日志发我。我用 `renderer_perf`、`desktop_process_metrics`、`backend_runtime_metrics` 对应时间点定位。
