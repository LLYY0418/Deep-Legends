# 国服阵容：本人／当前查询对象对照诊断

> R82 更新：缺失的路径 × 令牌 × 目标对照已提供独立的手动矩阵入口，**无需为了采集再开一局**。当前代码、实测缺口与操作方法见 [R82 执行结果](r82-current-game-probe-matrix-results.md)。下文是历史双目标诊断说明：R82 已移除普通卡片刷新附带的旧对照请求，相关 helper 仅保留用于历史数据回归；不要再按下文开局或反复点卡片采集相同格子。

本次补齐的是诊断能力，不是已经修复国服好友阵容。采用对照验证：只改变查询对象，保持 GSM 路径、
网关、GET 方法和捕获的 league-session 凭据一致。没有认证绕过、观战启动或其他客户端写操作。

## 如何采集

使用包含本次改动的新构建，打开目标玩家，在“好友正在游戏中”卡片点击 **重试阵容查询**。
如果已显示完整阵容，卡片右上角刷新按钮也走同一入口。等待请求完成，再导出诊断日志。

最有价值的对照是：**自己确实正在一场对局中，再查询另一个正在游戏的玩家**。
本人未在局也允许采集，但 404／无对局会明确标记，不能当成“本人阵容查询成功”。
查询本人时仅发一次 GSM GET；即使正常路径已由本地 LCU 返回，也不会漏掉这次手动诊断。
查看自己再看好友的两次手动诊断之间至少间隔 30 秒；同一次好友诊断内部已包含本人读取。

## 触发与资源边界

- 自动加载／刷新不额外发对照请求，记录 not-manual-refresh。
- 每份对照最多两个 GET；本人就是当前对象时只有一个。固定一份令牌，不刷新、不重试、不跟随重定向。
- 每个进程内 provider 最多一份对照执行中，同连接／账号／服务器 30 秒冷却；跳过会关联原 comparison_id。
- 整组最多 4 秒，使用原请求的取消信号，并为正常响应保留 1 秒。剩余预算不足则明确跳过。
- 本人请求和对象请求分配各自时间，防止第一个超时后第二个完全没有预算。
- 每个响应最多 512 KiB；不进行战绩/段位补全，不写入页面、阵容缓存或历史缓存。
- 每次发请求前及结束后检查账号、连接和服务器；前后读取本机阶段。账号切换丢弃旧账号页面结果。

## 日志覆盖

| 事件／字段 | 作用 |
| --- | --- |
| current_game_access_start | comparison_id、父 trace_id、方法、固定路径、预算、请求上限 |
| current_game_access_context | 前后阶段读取、token-start、缓存是否可用、token-pinned；中途中断可定位最后阶段 |
| current_game_access_http_start | self／target、同一个 credential_id、固定凭据标志；请求开始 |
| current_game_access_read | 每个对象的 HTTP 状态、耗时、响应大小、头部分类、鉴权分类、读取/解码/网络/超时/取消错误、字段形状、阵容计数与有效性 |
| current_game_access_comparison | 必有终态：skipped/inconclusive/completed，最后阶段、跳过原因、实际请求数、self/target 结果、阶段稳定性与连接一致性 |
| roster_scope_comparison_ready | 只有本人返回经过验证的在局阵容、阶段稳定且对象收到 HTTP 响应时才允许进一步比较；不是服务端权限结论 |
| game_payload_observed / game_payload_scope_comparison_ready | 分别记录已读到包含目标的在局游戏数据、以及同一对照内能否比较该读取与目标 HTTP 响应；不要求阵容完整，不代表成员身份全部有效，更不代表权限已获确认 |
| roster_shape.validation_failures | 固定分类说明空队、无效/重复身份、目标缺失、选人不在阵容或英雄冲突；不把无效身份推断成人机，不记录原始身份 |
| roster_comparison_blockers | 明确列出缺少的对照条件，如本人未在局、本人阵容未验证、阶段未确认／变化、目标未比较／未收到 HTTP 响应；跳过或中断记 comparison-not-completed |
| current_game_access_export | 导出前最后一份对照编号、是否仍在运行、冷却剩余时间；不触发请求、不阻塞导出 |
| auth_unknown_field_shapes / size_buckets | 未知错误字段仅记录类型数量和长度档位，不记录未知字段名和值 |
| token_identity_sub / puuid | JWT 固定字段的存在性、类型、与本人/对象的字面相等布尔值；sub 未必是 PUUID，不相等不能证明账号错误 |

正常请求已有的 HTTP、重读令牌、缓存、好友状态、前端渲染及上报健康日志继续保留。
CURRENT_GAME 在 401/403 后仍重读一次令牌；值未变时不重发相同请求，记录
`current_game_retry result=skipped reason=credential-unchanged`，保留原始 HTTP 错误及终态。
凭据实际变化仍允许重试；不改变其他 SGP 路由，不设置跨请求的权限封禁或永久负缓存。
新事件直接走后台 JSONL 存储，带 run_id/log_seq；复用现有导出，不依赖前端新增字段白名单。
终态落盘调用后才释放执行标志，避免导出显示空闲却还未写入终态。

## 如何解释结果

- self-roster-target-auth-rejected：同一凭据，本人有阵容，对象被 401/403 拒绝；重点核查对象授权范围。
- self-game-payload-target-auth-rejected：同一凭据已读到包含本人的在局数据，但阵容校验不通过，对象被 401/403 拒绝；数据读取与阵容可展示性必须分开判断。
- self-only-game-payload-roster-unverified：只查本人，读到了在局数据但阵容不完整/身份无法验证。不是 HTTP 鉴权失败，也没有比较好友。
- both-auth-rejected：两个对象都被拒绝；不能用“只允许查询本人”作为结论。
- self-no-game-target-auth-rejected：本人 404，对象被拒绝；本人是否能取到阵容仍未验证。
- both-rosters-verified：两次都取得经过校验的阵容；仍用 roster_outcome 区分完整与部分阵容。
- mixed-results-inconclusive／phase_stable=false／connection-changed：条件不完整或已变化，不能据此归因权限。

`game_payload_scope_comparison_ready=true` 的对象 HTTP 响应可以是 401/403：这正是比较本人成功读取与对象拒绝的场景；
它不是“对象已授权/阵容可用”的标志。必须同时看 `self_result`、`target_result` 和严格阵容校验结果。

这些都是观察结果，不是权限策略声明。没有明文 token、JWT claims、PUUID、玩家姓名、未知错误原文、
观战密钥、完整响应体或 Authorization；JWT 解码和字面相等也不代表签名已验证。

## 验证范围与不可保证项

测试覆盖同凭据/缓存令牌轮换、本人本地早退、请求计数、本人/对象分配预算、空令牌/失败/超时、账号切换、
阶段变化、并发/冷却/跨账号隔离、HTTP 状态矩阵、取消/截断/超限/错误 JSON/未知阶段/部分阵容、
不跟随重定向/不重试/不写入、日志关联及导出时仍在执行的情况。

已通过：`go test ./... -count=1`；新增对照、现有阵容、2055 诊断与 FlowDiagnostics 的 `-race` 测试；
`go vet ./...`；18 项前端 API／运行时上报／诊断导出测试；`git diff --check`。

2245 实测已确认对照完成，结果见 [2245 分析](diagnostics-0911-2245.md)。后续 [2326 分析](diagnostics-0911-2326.md)
已确认本人在局数据 HTTP 200 与好友 401 使用同一凭据，但本人这局阵容不完整，不能当作完整阵容成功。
前端 request/received/failed/rendered 等事件现保留同一次请求的 forceRefresh 标记，避免手动请求后续事件被误记为自动。
本地测试不能替代国服实测。服务端未披露的权限规则、断电、强杀进程、
磁盘持续不可写等不能靠代码保证日志绝对完整。导出时 comparison_running=true 表示该份文件可能尚无终态，
应等待查询完成后重新导出。
