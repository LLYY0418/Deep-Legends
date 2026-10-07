# R244　对局详情战绩被 30 天窗口截断、选人阶段首载慢、冷启动识别客户端比 LeagueAkari 慢

来源：用户 2026-10-07 15:05 两张截图（LeagueAkari 选人卡片 vs 我们「对局 · 详情」），日志 `ba0483a1-lol-loot-diagnostics-1007-1505.jsonl`；对照 LeagueAkari 源码（GitHub `LeagueAkari/LeagueAkari`，提交 `5109b2f`，2026-09-09）。

**执行位置**：主工作树当前源码（里面已有未提交的 R235/R241/授权改动，不要回退、不要混提交）。**不单独发版**，默认随 R238 的 0.12.77 一起发；账本写 `docs/r244-execution-ledger.md`，报告放 `docs/history/reports/r244/`。

---

## 结论

| 现象 | 根因（日志 + 源码） | LeagueAkari 怎么做 |
|---|---|---|
| 自己只显示「近 5 局」，Akari 显示 10 局（回溯到 08-22） | 选人详情把战绩**再按 30 天截断**（`live_history_freshness.go` 的 `recentLiveMatchesForPlayer` 与 `liveHistoryPageStop`）。服务端已经按 `q_420` 返回 10 局，超过 30 天的被丢掉 | `match-history-loader.ts`：SGP `tag=q_<队列>`、`startIndex 0`、`count=设置值`，**没有时间窗口** |
| 某位队友 0 条战绩，但标题却写「近 10 局 5胜5负」 | 同上：该玩家本队列最近一局在约 56 天前（`sgp_newest_queue_age_min=80534`），条目被 30 天窗口清空；标题统计走的是另一份列表，**两边口径不一致** | 同一份列表同时出标题和条目 |
| 选人阶段第一次加载要 5.2 秒 | 07:00:53 `live_load_cost`：`lcu_ms=4691`、`sgp_ms=399`、`total_ms=5242`。SGP 已可用，仍等本地客户端战绩接口 | SGP 令牌就绪就只用 SGP，LCU 只做后备 |
| 客户端出现后很久才进总览 | 见 P3 时间线：无进程时轮询退避到 8 秒；启动期每轮扫 249 个 lockfile；探测用 8 秒通用超时；总览 3 次 SGP 有一段串行；0.12.75 连上后遮罩没有收起，用户点了重试又重启 | 进程轮询固定 2 秒；端口拒绝时 2 秒后重试；连上即开事件流；总览只拉一页 20 局 |

**隐藏身份玩家（截图里我们显示「隐藏玩家」、Akari 显示出了名字）不在本工单范围。** Akari 用的是闭源原生模块 `resources/magic/magic.*.node` 里的 `magic()` 去解 `obfuscatedPuuid`，还挂在远程开关 `ongoing-game.deobfuscation` 后面。本工单**不碰** `deobfuscateHiddenPlayerReference` / `hiddenPlayerUUIDMask` 及相关逻辑，也不移植、不逆向 Akari 的二进制。

---

## P1　选人/对局详情的「近 N 局」去掉 30 天窗口，标题与条目同源

**证据**（cs-session-19，07:00:48–07:04:20 选人）：

- slot 0（自己，`is_current=true`）：`queue_filtered=true`、`shown_count=5`、`stop_reason="window"`。截图里 5 个条目 9/10/7、4/6/2、15/6/20、1/4/3、10/9/9，与 Akari 的 09-07 那几局一一对应（Akari 多出 0/0/0 重开局，以及 08-22 的 4 局）。
- slot 2：`shown_count=0`、`sgp_newest_queue_age_min=80534`、`stop_reason="window"`。界面却显示「近 10 局 5胜5负 50% KDA 2.27」，没有任何条目。

**要做**：

1. 选人/对局详情的战绩条目**不再按 30 天过滤**：`recentLiveMatchesForPlayer` 去掉 cutoff，只保留「本队列（`queueID`）+ 跳过重开局 + 最多 10 局」。
2. `liveHistoryPageStop` 去掉 `"window"` 分支。停止条件只剩：够 10 局（`enough`）、没有更多（`exhausted`）、达到页数上限（`page_limit`）。服务端已按队列过滤（第 0 页 `q_<队列>` 10 局）时，一页就够，不再翻页。
3. 服务端不支持队列过滤时，沿用现有「全部」翻页（每页 30，最多 4 页），够 10 局本队列就停。
4. **标题「近 N 局 / 胜率 / KDA」和条目必须来自同一份列表**：N = 条目数。条目为 0 时标题显示「本模式暂无战绩」，不得出现「近 10 局」配 0 条的情况。
5. 诊断：`live_history_freshness` 去掉写死的 `window_days: 30`，加 `oldest_shown_age_days`（展示的最旧一局距今天数）和 `header_games`（标题用的局数，必须等于 `shown_count`）。

**测试**（Go）：

- 10 局 `q_420` 全部早于 30 天 → 展示 10 局，标题 = 10。
- 本队列只有 3 局（都在 60 天前）、其余为大乱斗 → 展示 3 局，标题 = 3。
- 重开局不计入、不占名额。
- 服务端不支持过滤：第 1 页 30 局里只有 4 局本队列，第 2 页再给 6 局 → 2 页停，展示 10 局。

**变异**：恢复 30 天 cutoff → 「10 局全部早于 30 天」必须 FAIL；标题改回旧口径 → 「header_games == shown_count」必须 FAIL。

---

## P2　选人阶段首载：SGP 可用时不再等 LCU 战绩

**证据**：07:00:53 第一次 `live_load_cost`：`concurrency=6`、`lcu_ms=4691`、`matches_ms=5230`、`sgp_ms=399`、`total_ms=5242`。之后的刷新 `lcu_ms` 降到 31（缓存）。5 个 `live_history_freshness` 里有 4 个 `source="lcu+sgp"`，说明首载对每个人都同时走了本地战绩接口。

**要做**：

1. 选人/对局详情拉玩家战绩时：SGP 可用（`a.sgp.available(client)` 为真）就**只走 SGP 队列过滤页**；只有 SGP 失败/不可用才回落 LCU。不要「两边都拉再合并」。
2. 若「自己」必须用 LCU 补最新一局（刚打完 SGP 还没入库，`prev_game_in_sgp=false`），只对自己这一行补，且不能阻塞其他 9 人的渲染：其他人先出，自己补完再局部刷新。
3. 每个玩家的条目先到先渲染（已有逐人推送的保持），`live_load_cost` 增加 `first_player_ms`（第一个有条目的玩家耗时）和 `lcu_players`（走了 LCU 的人数）。

**目标**：SGP 正常时首载 `total_ms` ≤ 1.2 秒，`lcu_players` ≤ 1。

**测试**：SGP 可用的夹具 → LCU 战绩接口调用次数 = 0（自己补最新一局的情况除外，最多 1 次）；SGP 返回错误 → 回落 LCU 并正常展示。
**变异**：恢复「两边都拉」→ 「SGP 可用时 LCU 调用 = 0」必须 FAIL。

---

## P3　冷启动识别客户端：从「客户端出现」到「总览可看」对齐 LeagueAkari

### 时间线（0.12.75，run `60a4f7b4…`，应用 05:45:34 先开、客户端后开，非本软件启动）

| 时刻（UTC） | 事件 |
|---|---|
| 05:45:34 起 | `process-not-found`，退避 3→6→8 秒，之后每 8 秒查一次 |
| 06:07:46.740 | 首次看到进程：`credentials-unreadable`，`process_count=1`、`command_line_count=1`、`credential_candidates=0`、**`lockfiles_checked=249`** |
| 06:07:46.942 | 启动遮罩出现 |
| 06:07:55.836 | `probe-failed`：已有凭据（`process_count=2`），探测 `/lol-summoner/v1/current-summoner` 失败 |
| 06:08:05.140 | `connected` |
| 06:08:05.239 | 身份就绪（99 ms） |
| 06:08:07.562 | 自己总览数据完成：1972 ms，3 次 SGP 共 8.08 MB。`detailed_matches` 374→1441 ms，**之后** `recent_ranked` 1441→1945 ms（串行） |
| 06:08:12.019 | 用户点了遮罩上的「重试」（`identity_refresh_request source=overlay_retry`），遮罩仍在 |
| 06:08:30.783 | 用户重启应用（之前没有任何 `blocking_state_client hide`）；重启后 1.2 秒收起 |

本日志里 0.12.76 没有「先开应用、后开客户端」的样本，所以 0.12.76 的冷启动表现还没有实测。

### 与 LeagueAkari 的差距（源码对照）

| 环节 | 我们 | Akari |
|---|---|---|
| 无进程时多久查一次 | `connection_manager.go`：`minimumDiscoveryBackoff=3s` 翻倍到 `maximumDiscoveryBackoff=8s`。只有本软件启动的客户端才 1 秒（`client_launch_timing.go`） | `league-client-ux/context.ts`：`CLIENT_CMD_DEFAULT_POLL_INTERVAL=2000`，连上后 60 秒 |
| 进程已出现、凭据还没有 | 每秒把 C–Z 盘 × 10 个路径共 249 个 lockfile 候选全扫一遍（`lcu.go lockfileCandidates`），没有耗时日志 | 只读 `LeagueClientUx` 命令行，不扫盘 |
| 探测 | `probe()` 用通用 HTTP 客户端 8 秒超时，必须拿到 `summonerId>0` 才算连上 | `_doConnectingLoop`：只有 `ECONNREFUSED` 才 2 秒后重试；WebSocket 一打开就算连上，召唤师信息靠事件推送 |
| 连上到总览 | 3 次 SGP，有一段串行，约 2 秒 | 一页 20 局 |
| 遮罩 | 0.12.75 连上后不收（`identity-ready` 条件）；R235 改为等「玩家页签出现」（未发布） | 无遮罩 |

### 要做

1. **无进程时 1 秒一次轻量进程检查**：只做 `CreateToolhelp32Snapshot` 找 `LeagueClientUx.exe` / `LeagueClient.exe`，不读命令行、不扫 lockfile、不跑 PowerShell；发现进程后立即进入完整发现。删除「仅本软件启动时才 1 秒」的特例（统一 1 秒）。轻量检查耗时写进 `backend_runtime_metrics`（p50/p95），p95 必须 < 20 ms。
2. **启动期不扫全盘 lockfile**：命令行可读、只是还没有 `--remoting-auth-token` 时，只检查：命令行里 `--install-directory` 推出的路径，以及 `LeagueClient.exe` 映像路径（`QueryFullProcessImageNameW`）同目录的 `lockfile`。C–Z 盘全扫只在「进程查询失败」或「有进程但命令行读不到」时做，且同一次客户端启动最多扫一次（结果缓存到进程退出）。`lcu_discovery` 增加 `duration_ms`、`sweep=true/false`。
3. **探测改短、分清失败原因**：发现阶段的 `probe()` 用独立的 1.5 秒超时（正式会话仍用 8 秒）。`lcu_discovery` 增加 `probe_ms`、`probe_error_kind`（`refused` / `timeout` / `http-404` / `no-summoner` / `tls`）。`refused` 时 500 ms 后重试，其余 1 秒。
4. **端口一通就开事件流**：凭据有了、端口能返回任意 HTTP（包括 404）时，立刻建立事件 WebSocket 并订阅 `/lol-summoner/v1/current-summoner`，收到召唤师（`summonerId>0`）就立即完成连接和身份读取，不再等下一轮轮询。轮询保留作后备。
5. **总览首屏并行**：自己总览的 `recent_ranked`（单双/灵活 tag 查询）与 `detailed_matches` 并行发出。战绩列表第一页一到就渲染卡片、上报 `overview_card_ready`（`card="matches"`），不等另外两次。`overview_phases_ms` 里 `recent_ranked` 的起点必须 ≤ `detailed_matches` 起点 + 50 ms。
6. **遮罩**：在 R235 已改的 `updateReadingOverlay`（`self-tab-ready`）上补齐：
   - 后端广播 `connection-state` 后，前端在 100 ms 内拉一次 `/api/status`（不等轮询）；
   - 自己总览第一张卡片渲染后 300 ms 内收起遮罩；
   - 连上后 15 秒仍没有首卡，收起遮罩并在总览里显示可重试的错误，不能一直挡着。
7. **统一时间线诊断**：每次客户端出现发一条 `client_cold_launch_timeline`，不管是否由本软件启动，从「首次看到进程」算起：`process_ms=0`、`ux_cmdline_ms`、`credentials_ms`、`port_open_ms`、`summoner_ready_ms`、`connected_ms`、`identity_ms`、`overview_first_card_ms`、`overlay_hidden_ms`、`attempts`、`sweeps`，以及应用启动时客户端是否已在运行。

### 目标（以 Akari 为基准）

- 客户端进程出现 → 被发现：≤ 1.2 秒（Akari ≤ 2 秒）。
- 客户端召唤师可读 → 我们连上：≤ 0.6 秒（事件流 + 500 ms 重试）。
- 连上 → 自己总览首卡：≤ 1.0 秒；遮罩在首卡后 ≤ 0.3 秒收起。

### 测试

- Go：用假的进程快照/假的探测，跑完整序列 `not-found(×N) → unreadable → probe(refused) → probe(no-summoner) → summoner 事件 → connected`。断言：
  - 无进程阶段等待间隔都是 1 秒；
  - unreadable 阶段 `sweep=false`，且全盘扫最多 1 次；
  - 收到 summoner 事件后 ≤ 100 ms 进入 connected（假时钟）；
  - 时间线事件只发一条且字段齐全。
- Node：遮罩状态机。`connection-state` → 100 ms 内请求 status；首卡渲染 → 300 ms 内 hide（`hide_reason=self-tab-ready`）；15 秒无首卡 → hide 并显示错误。
- 总览并行：假 SGP 各延迟 500 ms → 总耗时 < 800 ms。

**变异**：
- 恢复 3→8 秒退避 → 「无进程间隔 1 秒」必须 FAIL；
- 启动期恢复每轮全盘扫 → 「sweep 最多 1 次」必须 FAIL；
- `recent_ranked` 改回串行 → 「总耗时 < 800 ms」必须 FAIL；
- 去掉 15 秒兜底 → 遮罩用例必须 FAIL。

### 真机（GPT 给清单，用户操作）

先开本软件，再从 WeGame 启动客户端，重复 3 次。日志里取 3 条 `client_cold_launch_timeline` 写进账本，旁边记录 Akari 同机的体感（客户端窗口出现时总览是否已就绪）。

---

## P4　账本与索引

- `docs/WORKLIST-INDEX.md` 加 R244 一行。状态写「本地实现完成 / 真机待验 / 随 R238 发布」。
- 账本要写清：每个 P 的改动文件、测试名、变异结果、Chromium 截图（选人详情：自己 10 局、老队列玩家标题与条目一致），以及 P3 真机时间线。
- 本工单不改 R238 文件；发布时 R238 的更新说明由用户决定是否加入「对局详情战绩显示更完整、选人阶段更快、客户端识别更快」。
