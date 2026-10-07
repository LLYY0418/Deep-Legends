# R244 执行账本

状态：**本地实现完成 / 真机待验 / 随 R238 发布**。在主工作树未提交的 R235/R241/授权改动上执行；不修改 R238、不占用发布版本、不提交或推送其他工单改动。`desktop/package.json` 当前版本仍为 **0.12.75**。

## P1：近十局与标题

- `backend/live_history_freshness.go`：取消展示与分页停止的 30 天窗口；保留本队列、有效玩家主体、胜负结果和最多十局。服务端已过滤队列时只读一页；不支持过滤时每页三十、最多四页，够十局停止。
- `backend/web/gameplay.js`：标题局数、胜负、胜率、KDA 都从实际展示的 `recentGames`（最多十局）计算，零局显示“本模式暂无战绩”；不再受另一份 `recentRankedRecord` 覆盖。
- `live_history_freshness` 删除 `window_days`，增加 `oldest_shown_age_days`、`header_games`。后者通过实际行统计函数计算，必须与 `shown_count` 相同；没有时间戳时最旧年龄为 null。
- Go：`TestR244OldLiveHistoryAndHeaderShareRows` 覆盖六十天前十局、三局混合其他队列、零局与重开不占名额；`TestR244UnfilteredHistoryStopsAfterTwoPages` 覆盖四局加六局、两页停止。
- Node：`R244 live header counts and KDA come from displayed rows even if stale stats disagree` 验证旧汇总冲突时仍与条目同源。
- Chromium：生产渲染器加本地合成夹具，自己十局、六十天前玩家三局、空战绩标题与条目一致，运行时异常零条。截图：[暗色](history/reports/r244/champselect-history-dark.png)、[亮色](history/reports/r244/champselect-history-light.png)。这是合成数据，不能代替真实客户端验收。

## P2：SGP 首载

- `backend/gameplay.go` / `backend/live_history_freshness.go`：SGP 成功（包括短页和空页）直接采用。只有失败或不可用才回退 LCU；自己有已知同队列上一局且成功 SGP 页确实缺该局时，最多补一行 LCU。
- 每行历史返回后立即推送，不等排位信息；自己补 LCU 时其他玩家继续推送。保留现有单飞、45 秒缓存及阶段/客户端隔离。
- `live_load_cost` 增加 `first_player_ms` 和 `lcu_players`；前者是首个有条目的玩家首次推送耗时，后者按加载证据统计。
- Go：`TestR244SGPAuthoritativeShortEmptyAndSelfCatchUp`（SGP 正常 LCU=0，缺自己上一局 LCU=1，上一局已在 SGP 时 LCU=0）；`TestR244SGPFailureFallsBackLCU`；`TestR244SelfCatchUpPublishesOtherPlayersBeforeLCU` 用阻塞 LCU 验证另外四名已公开队友先出。
- 不曾请求 LCU 时 `prev_game_in_lcu` 为 null，不把未读取当成确认不存在。SGP 失败的来源诊断与隐私检查保留。

## P3：冷启动与首卡

- `backend/process_windows.go` / `process_other.go` / `cold_client_discovery.go`：每秒轻量快照只枚举 LeagueClientUx/LeagueClient 名字，无命令行、lockfile 或 PowerShell。发现进程后完整查询；命令行可读时只查 install-directory 和 LeagueClient 映像同目录 lockfile；命令行不可读/查询失败时全盘扫描每次启动最多一次，后续只重用已发现路径；进程消失后重置。
- `backend/lcu.go`：发现探测独立 1.5 秒 deadline；正式 HTTP 客户端与正式会话 probe 保持 8 秒。日志增加 `duration_ms`、`sweep`、`probe_ms`、`probe_error_kind`。拒绝连接后 500 ms 重试，其余一秒。HTTP 404 或尚未就绪的响应正文也证明端口开放。
- `backend/connection_manager.go` / `main.go`：端口通后立即建立并订阅事件流；召唤师事件到达即推进连接，探测作为一秒后备在独立 goroutine 中执行。已获得召唤师直接建立身份，资料/熟练度后台补齐。等待成功后更新最终 discovery 状态；连接与身份各自计时。
- `backend/gameplay.go` / `overview_cache.go`：自己国服总览单双/灵活队列样本与第一页详细战绩并行；LCU/Riot 原有路径保持复用。客户端查询支持 NDJSON 首页推送与单飞进度重放；公开引用前复制参与者数组，避免进度污染最终统计。
- `backend/web/gameplay.js` / `app.js` / `runtime.js` / `features.go`：增加 `matches` 首卡诊断，自己首卡到达立即收遮罩；`connection-state` 直接启动 `/api/status` 请求；连上十五秒没有首卡时隐藏遮罩，并在总览显示已有的重试错误流程。保留启动无客户端/客户端退出逻辑与其他页的后台静默刷新。
- `backend/performance_diagnostics.go`：`backend_runtime_metrics` 增加 `process_snapshot_p50_ms`、`process_snapshot_p95_ms`。
- `client_cold_launch_timeline` 每次进程出现最终只记录一条，包括工单全部里程碑、尝试/扫描次数、应用初次检查时客户端是否运行。未观察到的里程碑为 -1；失败退出也保留一条不完整时间线。
- Go：`TestR244LightDiscoveryAndOneSweepPerLaunch`、`TestR244ProbeReasonsAndTimeout`、`TestR244ColdConnectionSequenceAndSummonerEvent`、`TestR244OverviewParallelAndEarlyMatches`、`TestR244OverviewProgressDoesNotMutateAggregation`、`TestR244OverviewFlightReplaysEarlyCard`。
- Node：一百毫秒内 status、首卡后三百毫秒内隐藏、十五秒无首卡错误与重试、matches 首卡只通知一次。
- 延迟夹具：三次 SGP 各五百毫秒，总览并行约 **0.53 秒**，小于八百毫秒；召唤师 loopback WebSocket 事件后连接断言小于一百毫秒。轻量快照 p95<20 ms 只在注入夹具中通过，真实 Windows 分位数和工单速度目标待验。

## 变异验证

七个变异均被对应行为测试拦住。最终脚本 `scripts/r244-mutations.py` 使用 **Go overlay / Node 临时源码副本**，确认共享源码 SHA 不变。

| 变异 | 护栏 | 结果 |
|---|---|---|
| 恢复三十天 cutoff | 六十天前十局与标题同源 | KILLED |
| 标题恢复独立排位记录 | 三局/零局标题夹具 | KILLED |
| 恢复成功 SGP 仍拉 LCU | SGP 成功请求数为零 | KILLED |
| 恢复八秒 idle 等待 | 虚拟等待序列固定一秒 | KILLED |
| 每轮重新全盘扫描 | 每次启动最多一次扫描 | KILLED |
| recent_ranked 串行等详细战绩 | 三次五百毫秒总耗时<八百毫秒 | KILLED |
| 去掉十五秒遮罩兜底 | 遮罩定时与错误事件 | KILLED |

初次变异脚本曾短暂写共享源码，可能干扰其他工单正在编译的测试；已改为完全隔离并重跑。初次结果不作为其他工单源码失败依据。结果见 `history/reports/r244/mutations.json`。

## 验证记录

最终核对：2026-10-07 16:25（Asia/Shanghai），源码指纹 `5f96886c0375`，当前 HEAD `b37357b2a6cf` 加未提交工作树。未创建提交或 tag。

最终检查与构建证据均位于 `docs/history/reports/r244/`。旧 R178/R180/R181/R202/R204/R229 及 R129/R193/R203 测试仅更新被 R244 明确替代的行为/夹具；保留缓存、回退、重开、隐藏身份和隐私断言。

- R244 Go 全部行为通过：`history/reports/r244/r244-go-final.log`；专项 race：`r244-race-final.log`。
- 受影响 Go 回归通过：`focused-go.log`；最终受影响 Node 37 项通过：`focused-node-final.log`。
- Chromium：`chromium.json`，自己十条、老玩家三条、空玩家零条，异常零条。
- installer test/vet 通过：`installer-test.log`、`installer-vet.log`。
- 全量首轮 Go 新夹具缺选人队列字段，已补齐；旧并发夹具的延迟后来被并行 tag 请求抢先取走，已限定到不带 tag 的详细页。最终 `go test ./... -count=1` 全量通过（212.201 秒），`go test -race ./... -count=1` 全量通过（285.238秒）；Go vet 与 Windows 测试交叉编译通过。全量首轮 Node 旧标题/遮罩夹具已更新，之后重跑。
- 最终 renderer 全量：1358 项，1353 PASS、4 SKIP、1 FAIL；耗时120.646秒，每文件小于90秒。唯一失败为已在修改前复现的收藏重扫；R244相关断言均通过。
- 收藏重扫断言 `desktop/refresh-orchestration.test.cjs:271` 在 R244 修改前的 app/gameplay/runtime 副本与最终源码都复现相同失败：`baseline-collection-rescan.log` / `final-collection-rescan.log`。该既有问题未扩大到本工单修复。
- 一轮全量 race 的 R206 收藏耗时护栏受并行负载影响为3.044秒；隔离连续三次均在2.81–2.82秒通过，完整 race 重跑已通过（285.238秒），没有数据竞争报告，见 `go-race-final.log`。
- 初轮 desktop/ui-scale 的 mock 缺 `getNormalBounds`（不在 R244 修改范围），由并行 R245 工单修复；最终全量以重跑结果为准。

### public 后端重建

- 版本：0.12.75；key mode：public；license mode：production（未使用 license_staging）。
- 文件：`/tmp/Deep-Legends-R244-0.12.75-public.exe`，32,724,480字节；仅验证后端编译，不是安装包或发布候选。
- 构建指纹：`5f96886c0375`，与最终源码一致，内嵌指纹校验通过。
- SHA-256：`20100e2fa3918b9c5c6450bca441ffbb68c7029f6dc8b28dacab0461461f5be0`。
- 详情：`public-build.json` / `public-build.log`。不覆盖现有 private/STAGING 安装包，不修改版本、CHANGELOG 或 R238。

## Windows 真机清单（用户操作）

使用后续 R238 构建的同一份新源码。每轮先完全退出客户端，先开 Deep Legends，再从 WeGame 启动客户端；重复三次。导出每次的新 diagnostics 日志，保留各自 run_id，旁边记录 Akari 同机“客户端窗口出现时总览是否已就绪”的体感。

核对：进程出现到发现≤1.2秒；召唤师可读到连接≤0.6秒；连接到自己首卡≤1秒；首卡到遮罩隐藏≤0.3秒；process_snapshot_p95_ms<20；SGP正常选人 total_ms≤1200、lcu_players≤1。

| 轮次 | client_cold_launch_timeline | Akari 同机体感 | 状态 |
|---|---|---|---|
| 1 | 待真实日志 | 待用户记录 | 待验 |
| 2 | 待真实日志 | 待用户记录 | 待验 |
| 3 | 待真实日志 | 待用户记录 | 待验 |

真实选人详情另核对自己的十局、六十天以上的老队列玩家、空列表标题。隐藏身份解码、混淆 PUUID 与 Akari 二进制均不在 R244 范围，未修改相关逻辑。
