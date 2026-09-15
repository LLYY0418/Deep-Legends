# R94 执行记录

日期：2026-09-14。对应 `WORKLIST-R94-LIVE-GAMEID-JUMP-MISSED.md`。在工作区已有 R87–R93 等改动上增量执行，沿用 R91 的新局过渡态及局部取消规则。

## 逐项交付

| 工单要求 | 实现与验收证据 |
| --- | --- |
| 同阶段 gameId 跳变也失效旧局 | `handleGameplayPhase` 独立比较 ID；`loadLive` 直接取得新 ID 时同样调用 `invalidateLiveForNewGame`，清 live、摘要及推荐缓存、渲染过渡态，再发布新数据。`web/r94.test.cjs` 连续两次 InProgress / 不同 ID 测试直接断言中间 null + awaiting 渲染和缓存 generation 变化。 |
| 同局正常刷新不闪烁 | 同阶段、同 ID 不失效；未知、零值和非法 ID 不伪造换局。R94 测试检查内容、generation 和渲染记录；R91 的正常阶段推进断言保留。 |
| 排查并弥补短暂阶段窗口 | 已确认轮询及事件队列盲区，详见下一节。阶段 API 附带有效 session gameId，SSE 透传 session 身份，发现跳变后强制完整刷新。R94 测试先确认 R90 完整名单已停刷，再只返回相同 phase / 新 ID，验证立即清空、发出 `?refresh=1`，新局完整后恢复停止策略。 |
| 补诊断 | `received` / `invalidate` / `stale-response` 记录 phase 原始值、来源、缓存 ID、收到的 ID、same / different / first / unavailable、是否清空及客户端接收时间。前端实际 runtime 测试验证重复观察不被采样丢弃；Go HTTP handler 测试验证 89 亿级 gameId 原样落盘、比较结果重算及来源字段。 |
| R91 回归及统计口径边界 | R91 13 项测试、6 个隔离变异通过。本轮未修改 `isRemakeGame`，未将它接入实时页。函数与执行前快照逐字节比对，见 `r94/remake-unchanged.json`。 |

## 采样盲区与补偿机制

- `connection_manager.go` 原 phase 事件回调直接转发，未发现该分支自身节流。`live_updates.go` 的每个订阅者队列容量 32；慢消费者溢出会清队列并发 `resync-required`，因此连续阶段可能被合并。
- `web/gameplay.js` 在最近 45 秒收到事件帧时每 12 秒轮询一次阶段，否则每秒轮询；隐藏页面暂停轮询。短于轮询间隔的 EndOfGame / Lobby / ChampSelect 窗口可以完全跨过。R90 完整名单停止定时刷新，不能只依赖下一次名单请求自行发现 ID 变化。
- 现在阶段接口从原 phase 读取加一次 session 身份读取（仅 GameStart / InProgress / Reconnect；子请求预算 1.5 秒，整体遵循 6 秒请求上下文）。不读取玩家名单或推荐。session 暂不可用仍返回阶段；ChampSelect 不采用可能残留上一局的 gameData。
- 新增 SSE session 身份消息复用已收到的事件载荷，不在事件线程再发 HTTP 请求。真实 TLS/WebSocket fixture 经 `runConnectedSession` 回调验证两条同阶段不同 ID 消息及原 phase-only 消息都到达订阅者。
- ID 跳变立即失效旧请求；事件刷新队列在既有 180–1000ms 合并窗口内触发完整强制读取。隐藏页面先失效，回到前台再加载；测试执行实际 SSE listener 和 visibility listener 验证此路径。
- 已飞行的旧 phase 轮询通过请求开始时的 phase / generation / expectedId 检查，不能覆盖较新 SSE 或 live 响应；新局读取返回旧 ID 时保持过渡态并走原有有界重试或手动刷新。

## 延迟窗口核实

同局 ChampSelect → InProgress 原本允许保留推荐，且 `/api/gameplay/live` 请求预算为 45 秒，因此日志中 37 秒窗口不能单凭渲染时间判为重开局。现在阶段推进立即显示“对局阶段已变化，正在同步当前对局…”，拒绝迟到的同局选人快照回退 beacon，并让旧阶段的完整快照继续同步重试。新 ID 的正常选人响应则仍能开启新局，避免把所有选人响应误当旧数据。

若新快照已经到达，清空与替换会在同一 JS 执行段完成；测试验证过渡态渲染调用，不为制造闪屏而强加等待。若先收到 SSE / poll 身份，网络等待期间会实际保留新局过渡态。

## 诊断传输边界

- 独立 gameflow 批量队列保留重复观察，不受原 `live_refresh_client` 一秒采样 / 30 秒去重影响。每批最多 8 条，每次发送完成后至少间隔一秒再发下一批，只有一个该类请求在飞行，队列上限 64；请求失败不重试，避免挤占业务请求。
- 达到容量时优先丢普通未变化观察；无可丢普通观察时淘汰最旧记录。`transport_dropped` 可由队列丢弃或失败批次产生（批次失败按一次传输计数，不等同于丢失观察数）；`transport_failed`、客户端 `received_at` 与服务端写入时间帮助判断缺口和延迟。离线及异常高频情况下不承诺无损。
- 导出会立即尝试发送排队 batch，在原有一秒窗口内等待，并记录尚未发送数量。通用 pending 上限同步为 97（最多 32 个既有请求 + 1 个 gameflow 请求 + 64 条排队观察），旧边界断言更新为新容量；不宣称导出总能排空全部队列。
- 后端保持 4 KiB 总请求限制和未知字段拒绝，接受最多 8 条合法观察；最大允许字段长度的前端 batch 实测小于 4 KiB。仅保留阶段及枚举、ID、时间、布尔值，不记录玩家名称、令牌或任意文本。

## 验证记录

- 首批 6 个 R94 场景在旧实现上均失败，见 [before-js.txt](r94/before-js.txt)。执行前相关测试基线通过，见 [baseline-js.txt](r94/baseline-js.txt)。
- 最终针对性 JS：60 项全部通过，包含 R94 14 项及 R91 / R90 / R88 / R87 / runtime 回归，见 [targeted-js.txt](r94/targeted-js.txt)。
- 完整前端与桌面回归：660 项，659 通过、0 失败、1 跳过（Windows 门禁），见 [full-js.txt](r94/full-js.txt)。此后只补充了隐藏页面及诊断失败测试，已纳入最终针对性运行；生产 JS 与该完整运行一致。
- Go 针对性、完整回归、R94 race 及 `go vet ./...` 结果分别见 [targeted-go.txt](r94/targeted-go.txt)、[full-go.txt](r94/full-go.txt)、[race-go.txt](r94/race-go.txt)、[vet.txt](r94/vet.txt)。首轮完整 Go 唯一失败为导出 pending 上限的旧 32 断言，保留于 [full-go-initial.txt](r94/full-go-initial.txt)。
- 11 个 R94 隔离变异全部被语义断言拦截，见 [mutations.json](r94/mutations.json)：删除 ID 清空、phase-only 触发、漏传 poll / SSE ID、漏强制刷新、接受旧局响应 / 旧轮询、去重丢观察、缺身份探针 / 后端 SSE 接线 / 日志 ID。变异不修改生产工作区；编译失败或超时不算拦截。
- 适配 R90 / R91 变异脚本中随本次实现改变的锚点。R91 六个变异复跑通过，见 [r91-mutations-rerun.txt](r94/r91-mutations-rerun.txt)。首次扩大清空变异会使无关的合并请求测试等待，记录在 `r91-mutations-initial.txt`；复跑改为每个变异只执行对应语义测试，并要求 AssertionError，未将超时算作成功。
- 两份独立只读复核分别检查前端竞态与后端/诊断链路，无确认缺陷；建议的隐藏页面、传输失败、真实回调接线测试已补齐。
- `node --check` 三个生产 JS 文件及 `git diff --check` 通过。起始快照差异与最终文件哈希见 `r94/scoped-changes.patch`、`r94/source-sha256.json`。

复跑：`node --test web/r94.test.cjs web/r91.test.cjs web/r90.test.cjs web/r88.test.cjs web/r87.test.cjs web/runtime.test.cjs`；`go test ./...`；`go test -race -run TestR94 .`；`python3 scripts/r94-mutation-check.py`。

## 真机验证边界

尚未在 Windows 国服客户端实际重开或连续快速结束对局验证。本轮证明同阶段 ID 跳变的检测、清空及刷新路径与异常恢复，不证明用户三段日志中的对局均为重开，也不宣称真实客户端延迟已经消失。实际 SSE 若缺 session 身份事件，仍由轻量轮询兜底；需要下一轮真机诊断核对 `source`、`game_id_comparison=different`、`invalidated=true` 与后续新局渲染。
