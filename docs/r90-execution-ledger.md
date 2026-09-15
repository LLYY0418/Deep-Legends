# R90 执行与验收账本

2026-09-14。执行工单：`WORKLIST-R90-ARENA-LIVE-GROUPING-REFRESH.md`。实现和自动化验收已完成；四项 Windows 国服真机验收仍待实际对局。没有生成安装包或发布版本。

工作区原有 R87/R88/R89 等大量未提交修改均保留。主要生产代码相对本轮开始的增量见 [implementation.diff](r90/implementation.diff)，新增分组模块为 `arena_live_grouping.go`；最终相关文件哈希见 [source-sha256.json](r90/source-sha256.json)。工单中的历史日志统计作为诊断输入，本轮没有重新采集那些日志。

## 实现逐项核对

| 工单 | 完成行为 | 验证抓手 |
|---|---|---|
| P0-1 / 验收 1 | 重写 `r62ArenaFixturePlayers`：playerlist 恰好 19 个真实键，无 subteam 字段；gameflow 仅七个非空字段、名字空、party 分布与工单一致。mock summoner 端点负责补齐真实读取链路里的身份。 | `TestR90FixtureMatchesObservedFields`；真实 fixture 在修复前返回 `grouped=false`，见 [baseline-red.txt](r90/baseline-red.txt)；修复后六组三人。 |
| P0-2.1 | 队伍人数登记在 `queue_groups.go` 的 SquadSize：1700/1710=2，1750=3。顺序切块要求 6–18 人且整除 squad size。 | `TestR90QueueSquadSizesAndSessionFallback`；错误切为每两人的变异失败。 |
| P0-2.2 | 自身和记住的小队逐个同块核对；跨块输出明确不可分组并写 `arena_group_order_rejected`。中途启动无队友记忆允许分组，来源 `live-client-order-unverified`；已知完整小队尚有身份无法匹配则维持未完成，不冒充核验通过。 | `TestR90AlliesVerifyAndRejectScrambledPlayerlist` 的正例及乱序反例；删除佐证的变异失败。 |
| P0-2.3 | 分组移到 summoner 解析之后；分组与 position 共用 `livePlayerIdentityValues`。优先完整 Riot ID，再使用无歧义名字；顺序索引丢弃跨块重复的名称键。 | 空 gameflow 名字 fixture；改回 raw 名字的变异失败；原 R69 position 来源测试通过。 |
| P0-2.4 | playerlist 暂不可用时使用 session-order，按相同 squad size 切块并执行同一小队佐证。删除 party ID 佐证和固定 18 人限制。 | 正常 fallback、跨块队友拒绝、2 人制和 12 人三人制用例；旧 party-split 测试改为明确验证 party 不参与分组。 |
| P0-2.5 | gameflow 缺人时保持完整 playerlist 边界；包括缺少己方成员的情形，必要时只补该已知队友身份用于核验，不伪造名单成员。 | 缺第 2、8、18 名玩家的用例均得到 `5×3 + 1×2`，逐人检查组号；改回 session 切块的变异失败。 |
| P0-2.6 | 足够人数的成功 playerlist 响应即缓存成功，保留数组身份顺序；Grouped 仅表示真实显式 subteam 字段。手动刷新可重新读取。 | 两次 live HTTP 加载不重复探针；新 gameId 重新探针，Reconnect 旧回归保留。 |
| P0-3 | 所有顺序推断 `ArenaMascotMapping=false`，只显示“小队 N”；真实显式 subteam 字段的兼容分支仍可授权映射。 | 猜测 mascot=true 的变异失败；旧显式字段 fixture 保留，仅用于未来接口兼容，不再当作真实在线形状。 |
| P0-4 | 保存最多五局内存推断，按客户端、服务器、gameId 和 queue 隔离；普通 SGP 详情和结束阶段的有界补查都比较真值、去重。分区正确与编号恒等分别记录，缺真值时等待后续详情，不输出假阴性。 | 真实 mock SGP HTTP 到日志端到端测试；恒等、置换、错队、17 人未完整真值、重复/跨客户端/跨服及隐私断言；强制 partition=true 变异失败。 |
| P0-4 生命周期 | 结束阶段补查每阶段最多 4 次、35 秒；阶段变化取消旧任务，在新的结束阶段继续确保回查。已核验对局不再请求。清理账号引用时清理推断、队友和探针。 | `TestR90EndPhaseReadsSGPTruthOnce`；`TestR90EndPhaseTransitionRestartsCanceledTruthRead` 实际取消 WaitingForStats 请求后，EndOfGame 第二次读取成功，race 通过。 |
| P1.1 | 前后端独立完整性函数均要求 available、非空 players、无 pending history，斗魂还要求已分组或明确不能分组。完整 InProgress/Reconnect 不设 timer；不完整统一每 20 秒重试最多 8 次。采用工单允许的 20 秒方案，没有加可选的前 60 秒 5 秒加速。 | 假计时器调用实际调度函数；完整函数恒 false 与取消次数上限的变异失败。 |
| P1.2 | 页面顶部停止状态及“刷新”按钮；手动按钮请求 `?refresh=1` 使快照及 playerlist 探针失效。加载期间手动点击保留一轮后续刷新。 | 渲染状态、绑定实际点击 handler、实际 loadLive URL、加载中手动排队测试。 |
| P1.3–4 | 保留阶段事件及 1 秒 / 12 秒 phase 心跳；隐藏期间阶段变化保留待办，恢复后执行。非 live 页同阶段不装配；游戏中同阶段事件不绕过有界重试定时器。切回前台先核对 phase。 | R87 隐藏/SSE/resync、独立 phase poll，以及新前端回归。ChampSelect 仍为 3 秒，原有读取失败 60 秒退避保留。 |
| P1.5 / 验收 8 | 完整快照整局缓存；未完成快照也缓存 20 秒。每次缓存命中核对 gameId；phase、客户端、账号和主动 invalidate 失效。 | grouped / ungrouped 均两次 HTTP 只装配一次；人为老化缓存后完整复用、不完整重建；撤销两层 TTL 的变异均失败。 |
| 验收 9 | 原 R87/R88 AST 守卫扩展扫描 `arena_live*.go` 与 `gameplay_refresh.go`；新函数随文件扫描。新代码无 queue 整型比较，队列分类只取登记表。 | `TestR90QueueGuardScansNewGroupingModule`；隔离插入 `queueID == 1750` 必须失败。 |

## 自动化验证

- [全量 Go](r90/full-go.txt) 通过；[本轮定向 Go](r90/targeted-go.txt)、[相关 race](r90/targeted-race.txt)、[连续结束阶段 race](r90/end-phase-race.txt) 通过。
- [前端/桌面全量](r90/full-js.txt)：636 项，635 通过、0 失败、1 跳过（Windows/PowerShell 实际发布门禁）；[定向前端 258 项](r90/targeted-js.txt) 通过。
- 11 个隔离变异全部被拦截，见 [mutations.json](r90/mutations.json) 及同目录各变异日志。脚本为 `scripts/r90-mutation-check.py`，Go 使用 overlay、JavaScript 使用临时副本，生产工作区不被变异。AST 属于运行时源码读取，另用显式测试路径读取隔离副本，不能仅靠编译 overlay。
- `go vet ./...`、`node --check web/gameplay.js`、`git diff --check` 通过；Windows amd64 后端交叉编译通过，检查产物 `/tmp/r90-loot-service.exe`。未修改安装器，未生成 Setup。
- 初次本轮回归发现 `storage.go` 的 build marker 行处于 `MUTATION-TEST-DISABLED`，已恢复并验证新的真值日志包含实际 `buildFingerprint`。一次前端全量运行还在读取 CSS 时观察到旧 `arena-first 640px` 规则被临时恢复；随后文件已恢复，本轮 CSS 相对起始快照仅新增刷新状态样式。最终重新验证，不把受临时变异影响的运行冒充通过。
- 最终哈希复核又发现 R89 总览并发补充入口从临时 `if (false && !append)` 恢复为正常 `if (!append)`，保留正常代码；针对当前文件补跑 R89/R90 共 12 项全部通过，见 [最终并行改动复核](r90/final-concurrent-recheck.txt)。最终差异和哈希已重新生成。
- 独立只读复核覆盖后端生命周期及前端排队边界。关于多结束阶段可能取消后不恢复的疑点，按实际代码确认回查位于 `isEndOfGamePhase(phase)` 而非单次 ended 门下，并补真实取消/恢复的 HTTP race 测试。未解析玩家身份与“没有选人队友记忆”分别处理：前者继续有界重试，后者允许 unverified 分组。

复跑：`GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go test ./...`；`node --test web/*.test.cjs desktop/*.test.cjs`；`python3 scripts/r90-mutation-check.py`。HTTP mock 需要回环监听权限。

## 尚需 Windows 国服真机完成的四项

1. queue 1750 进入游戏，截图确认六块三人（或五块三人、一块两人）；日志确认 `arena_grouped=true`、`arena_group_source=live-client-order` 和组人数。
2. 对局中静置三分钟再导出日志，确认没有每 20 秒整名单装配，summoner 请求不随时间线性增长。
3. 赛后查看 `arena_group_truth_check` 的 `partition_match`、`block_to_subteam_identity`。积累 3–5 局可信结果后，下一轮再决定是否启用吉祥物映射；本轮未启用。
4. 对局中关闭再打开应用，确认 `live-client-order-unverified` 可分组、停止状态及手动按钮正常。

以上四项尚未执行。本轮 mock、计时器、竞态和交叉编译不能证明真实客户端的数组顺序永远正确，也不能替代实际三分钟请求量验证。

## 补充：顺序推断命名条件的独立回归

将原命名许可表达式等价提取为 `arenaLiveMascotMappingAllowed`，新增 `TestR90OrderInferenceNeverEnablesMascotMapping`。测试有意构造同时满足 subteamId / playerSubteamId 与有效小队编号的元数据，先确认下游吉祥物检查为 true，再断言顺序推断必须关闭映射；真实客户端编号允许映射、未分组禁止映射作为对照。这样不再依赖当前 order 分支的 Field="order" 恰好让另一条件返回 false。

已用 Go overlay 隔离验证：仅删除 `!order` 后，两种字段的 inferred-blocks 子测试均因 `mascot mapping = true, want false` 失败，见 [精确条件删除日志](r90/mascot-guard/removed-order-condition.txt)。常规变异脚本新增同一删除场景，并更新原整段表达式变异的锚点。

[五项相关 Go 回归](r90/mascot-guard/targeted-go.txt) 与 [前端小队 N / 吉祥物标签回归](r90/mascot-guard/frontend-labels.txt) 通过，`git diff --check` 通过。本次没有重跑此前全量验证。
