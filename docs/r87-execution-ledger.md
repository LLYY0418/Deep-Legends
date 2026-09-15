# R87 执行与验收账本

2026-09-13。范围：完整工单 P1–P9、A1–A3（含追加 P7–P9）。所有代码修改及本地自动化验收已完成；下表明确标出的真实客户端验收仍待执行，不能用模拟数据代替。当前基线为 `2a5a0a7c9109e46683dfff32ae7b2b6ddfcd346d`，保留 R86 设置锁及指纹缓存排除修复；原 P1–P6 证据采集基线为 `a26c41ffc2d373836c15c73bee45178610c14e4e`。

| 工单 | 实现与可复现护栏 | 状态 / 边界 |
|---|---|---|
| P1 名单宽度 | 仅调整 `.match-summary` 第三轨道、`.match-players` 宽度与两列间分配。保留 max-content 两列、14px 最小间隙、420px 上限。真实 Chromium 同页 8 卡、4/6/8/12/16/20/6/16 字，覆盖 1920/2560/3840。每张名单严格 420px、左右对齐、高度 116px，16 字及以下零截断，20 字不溢出。 | 已通过。`web/champions.test.cjs` 和 `desktop/ui-scale-layout.cjs` 同步新判据；保留 `players-1fr` 变异。 |
| P2 后台识别 / 推荐数据 | `connection_manager.go` 阶段事件进入 `ChampSelect` / `InProgress` 时预热；首次连接已在局内也预热。`gameplay_refresh.go` 共享 flight，绑定客户端、阶段及取消生命周期；InProgress 的预热结果保留至首次读取，读取前核对 gameId，阶段改变取消旧预热。普通前台读取继续探测最新 Live Client 数据。前端 resync 保留队列，SSE / phase poll 都经 `handleGameplayPhase`；poll 仅在真实阶段变化时发起 roster 请求。 | 已通过隐藏窗口、总览 resync 同时挂起、poll 独立兜底、长时间隐藏、串局和过期阶段测试。非对局页仅在可见、已连接、自动刷新开启且处于活动阶段时 45 秒兜底；失败降为 60 秒。后台预热只按阶段触发，不常驻轮询。 |
| P3 全部玩家标签战绩刷新 | 后端 `invalidateOverviewPlayer` 精确删除当前玩家缓存并隔离旧 flight。三个结束态都触发，按局去重。前端所有标签标 dirty，自身及当前可见玩家延迟 7 秒 force，其余玩家切入时再拉；每标签最多 3 次，后两次间隔 8 秒。沿用刷新提示条和静默错误状态。force 物理清除该玩家 SGP 历史页，并用 generation 阻止旧请求晚到后写回。 | 已通过 WaitingForStats 精准失效、非活动标签无即时请求、7+8+8 秒上限、SGP 强刷绕缓存及其他玩家缓存保留测试。真实新战绩入库后自动出现尚待游戏客户端验收。 |
| P4 收藏识别与耗时 | `collection_reads.go` 提供单次快照共享响应体，皮肤 / 炫彩 ownership 与 acquisition 共用；每刷新独立，空结果不跨刷新缓存。获得完整显式所有权覆盖后提前退出；不完整、仅 presence 的结果继续回退。保留旧独立审计调用的多源核验。空身份战利品保留“客户端数据暂未同步，可稍后重试”；皮肤池问题单独显示“查看皮肤池匹配失败条目”。pending 自动重试 5/15/30 秒，最多三次，并绑定当前客户端。刷新入口、合并、失败及取消都有生命周期事件。 | 请求复用、空数据后恢复、占位文案和三次重试上限通过。同一机器模拟响应、前后各 3 次：中位数 139.08ms → 9.45ms（93.2%，race 模式，固定 8ms 模拟端点）。**不是实际 LCU 的 ≥35% 验收结果**，真实客户端各三次对照仍待执行。完整显式覆盖要求比旧 80% 门槛更严格，避免把不完整库存当权威全量。 |
| P5 领取停滞 | suite API 15 秒超时覆盖 fetch 与响应体，AbortController 中止请求。后端 12 秒总预算包含等待领取锁、全量校验和写入。保留逐项执行及失败后继续，执行前仍做权威校验；执行后使用结算记录过滤，仅任务奖励刷新 mission 来源以发现下一链节点。批次结束再全量扫描一次。每项 begin/end 带 trace、index、耗时、是否超时，等待锁或扫描前超时也成对；前端每 5 秒上报 claiming 心跳，finally 清理状态。 | hang / body hang 在 15 秒结束；单项超时后继续；9 个逐项请求只有 9 次全量扫描，加最终 UI 扫描共 10 次（原 18 次）。锁等待预算、handler 自带预算、早期超时诊断都通过。未改批量提交：逐项继续和任务链发现保留，已满足 ≤10 次要求。 |
| P6 斗魂预检与禁用诊断 | 下发预检使用已锁定的 `r.champSelect.groupID == "arena"`，与主评估相同；gameplay 三处收敛到队列表辅助函数。真实请求夹具对 1700/1710/1750 均发出 championId=-3 的 PATCH。AST 扫描全部 champselect 生产文件和任意函数，拦截队列字面量比较。`champselect_ban_probe` 分开记录 has_ban_action、requested、真实 HTTP status_code、raw_count；source 保留原始 queue_id。 | 已通过三队列真实 PATCH、任意新文件字面量变异，以及未请求/200/204/503 探测。自动禁用的 A/B 两个假设仍需实局日志互斥验证，不能声称已证实客户端使用另一端点。 |
| P7 自动开始匹配 | 四态守卫记录 `skipped_state` 和 custom / started / inflight / exhausted；三个结束态均取消旧任务并清 started / exhausted，取消与开关、自定义状态变更同时清 inflight。旧 worker 只可修改自己仍持有的任务状态，防止结束后晚到的 POST 污染下一局；直接退回 None 也取消。`watch_settings_client` 增加 `auto_matchmaking_enabled`，非房主、自定义跳过有可见提示；移除无调用的旧通用分支。 | 三个结束态、四个 reason、五种取消后重新启用、旧 POST 晚到及真实诊断导出通过；回退结束态复位和守卫诊断均变红。历史 9761 行日志仅有一次实际成功，没有真正失败样本；开关未开/非房主/队列未选/自定义/exhausted 的历史归因仍未证实。 |
| P8 日志导出完成状态 | 主进程独立使用下载目录作为原生 Save dialog 的默认位置，不再读取分享图目录。用 `setSaveDialogOptions` 同步配置 Electron 的原生保存对话框，提前注册 done，完成时用 `getSavePath` 获取最终路径并通知前端。保存被取消、中断或窗口销毁时不通知；分享目录为空、失效均不妨碍完成。前端跨设置子页及跨页面保留完成状态，打开文件夹后仍按原交互恢复导出。 | 首次导出、无效目录、实际最终路径、跨页完成、取消、销毁、浏览器降级通过；恢复目录为空时提前返回会变红。真实 Go HTTP 导出接 Electron 主进程钩子的启动夹具通过。preload / IPC 接口保持兼容；实际 Windows 原生保存与文件管理器定位仍待实机验收。 |
| P9 编辑序列弹窗 | 面板主体独立容器，外部刷新保留 dialog、搜索 input 及其子树；仅明确编辑/筛选结果更新列表片段。搜索不被外部值重写，showModal 仅首次打开调用。草稿绑定打开时的分组和分路；关闭/销毁中止位置请求，晚到结果不能写回。新增 open / rerender-while-open / close 诊断，附序号与服务端时间。 | 200px 滚动、四字符加六次外部事件、焦点、选区、输入法 composition、单次 showModal、关闭、销毁后晚到响应均有行为回归。两个 DOM 护栏回退均变红。480×700 真实 Chromium：滚动 198→198、输入 ahri、焦点保留；回退变为 198→0、仅 a、焦点丢失。见 [实测](r87/addendum/dialog-baseline.json)、[回退实测](r87/addendum/dialog-mutant.json)。 |
| A1 位置播报 | 所有失败出口附 reason：session-read-failed、gameflow-read-failed、chat-read-failed、conversation-unavailable、message-write-failed、team-unavailable；跳过及成功分支也有原因。 | 六类失败实调出口测试通过。原 279 次失败没有 reason，无法反推其具体根因，未伪称播报成功率已恢复。 |
| A2 结束态动作 | 复核快速下一把已覆盖 WaitingForStats / PreEndOfGame / EndOfGame；保留 R86 提前排定与去重行为。点赞由 honor ballot 事件独立触发，不依赖收到 EndOfGame。新增 endgame_trigger 记录入口，工具 UI 将三个结束态统一映射至结算显示。 | 已有真实 play-again / honor 写入回归通过，三个显示阶段新增行为测试及回退变异通过。实际触发率需新实局日志统计。 |
| A3 假重连 | 复核配置本就把重连等待限制为至少 3 秒；保留离开 Reconnect 时取消。新增等待结束后读取真实 gameflow phase 的下发预检，防止 WaitingForStats 事件丢失后仍发重连。 | 短暂 Reconnect 后取消、持续 Reconnect 真 POST、结束事件丢失时不 POST 均通过。回退新预检会变红。 |

**红线证明与自动化证据**

`.match-main`、`.match-stats` 包含断点的全部 6+6 个规则逐字比较一致。分别对改前 / 改后规则文本执行统一 diff，输出均为 **0 字节（空 diff）**：

```text
$ diff -u before.match-main.css after.match-main.css
(exit 0; stdout 0 bytes)
$ diff -u before.match-stats.css after.match-stats.css
(exit 0; stdout 0 bytes)
```

原文、全部规则及哈希见 [protected-css.json](r87/protected-css.json)。真实尺寸见 [roster-layout.json](r87/roster-layout.json)。名单截图来自生产页面真实 Chromium，测量不使用 jsdom。

| 最终检查 | 结果 / 可复现命令 |
|---|---|
| 完整 Go | `go test ./...` 通过；[最终日志](r87/addendum/go-test.log)。 |
| 完整前端与桌面 | `node --test --test-concurrency=4 web/*.test.cjs desktop/*.test.cjs`：605 项，604 通过，0 失败，1 跳过（Windows / PowerShell 发布门禁，当前 macOS）；[最终日志](r87/addendum/js-test.log)。 |
| 新增前端行为 | `node --test web/r87.test.cjs`：6/6；[日志](r87/js-r87-test.log)。 |
| 并发检查 | `go test -race -run 'TestR87\|TestClaim' -count=1 .` 通过，包含 P7 新状态测试；[最终日志](r87/addendum/race-test.log)。早一轮另包含 R86 play-again 与 honor；[行为与延迟证据](r87/behavior-evidence.log)。 |
| Windows 编译 | `GOOS=windows GOARCH=amd64 go build -o /tmp/r87-addendum-loot-service.exe .` 通过；仅交叉编译，不代表在 Windows 运行验收。 |
| 名单真实浏览器 | `node desktop/ui-scale-layout.cjs` 通过；4 个 CSS 变异分别被宽度、截断或列间距护栏拦截。 |
| 关键修复回退 | `python3 scripts/r87-mutation-check.py`：17/17 被断言拦截；另 4 个 Chromium CSS 变异 + 1 个新增生产文件 AST 变异 + P8 首次导出变异 + P9 滚动/输入两个独立断言，总计 **25/25**。[代码/前端矩阵](r87/addendum/mutations/results.json)、[CSS 矩阵](r87/mutations/css-results.json)、[AST](r87/mutations/ast-result.json)。P8/P9 变异内置于对应测试；编译错误不计有效变异。 |
| 弹窗真实浏览器 | `node desktop/champselect-dialog-layout.cjs` 通过；加 `R87_DIALOG_MUTANT=1` 回退时断言失败，滚动与输入实测均退化。[修复](r87/addendum/dialog-baseline.json)、[回退](r87/addendum/dialog-mutant.json)。 |
| 合回校验 | 46 个修改/新增代码及测试文件与完整检查使用的隔离目录逐字节一致；[SHA-256](r87/addendum/source-sha256.json)。共享目录 6 项关键行为回归全部通过；[日志](r87/addendum/integration-js.log)。 |

完整前端首轮暴露既有滚动测试的固定 180ms 等待竞态：骨架屏期间 jsdom 可被强行赋予 640px，但此时生产代码有意保留待恢复状态。测试现等待玩家内容加载及恢复帧，再执行原有 640px / 210px / 跨页面断言，未放宽断言或改动生产滚动逻辑；最终全量通过。

P2 固定模拟慢源对照：冷路径 177.36ms、预热首读 6.31ms；另有 10 分钟隐藏后首次读取仍命中、跨 gameId 拒绝旧数据测试。数字只证明该缓存路径工作，不代替真实网络耗时承诺。

**证据边界与仍待真实客户端完成的验收**

追加工单给出的 9761 行历史日志也未在工作区找到原文件，本轮没有重新统计它。工单说明其同一 run_id 对应五次 app_start，不能据此验证状态是否跨进程重启持续。位置播报的新历史统计 153 armed / 152 failed 同样不能反推失败出口；A1 已补原因诊断，需新日志验证。桌面导出断点是代码和后端日志的交叉证据，并非历史 Electron 抓包；当前实现完成时仍附加可用的 desktop.log。

本轮工作区未找到 `lol-loot-diagnostics-0913-1646.jsonl` 原文件。工单中的历史统计被视为工单提供的证据，未假装重新统计 12663 行。73/179 次 canceled 无法凭现有旧字段区分客户端 abort 与软预算；本轮沿调用链补 `cancel_scope`（client-canceled / budget-timeout / loader-canceled / request-canceled / transport），并验证诊断实际可导出。不能把新增分类当成对历史 41% 的已完成归因。

训练模式 3140 不加载属于工单已说明的预期；没有把“重开”作为已证实根因。斗魂截图晚于原日志导出时间，原日志中的 champselect 阶段也没有斗魂样本；-3 失败链是代码路径证据，1750 在用是工单提供的对局证据，不是该截图动作的日志直接观测。

仍需在实际 Windows / LCU 环境完成以下验收，代码与护栏已准备就绪：

1. 游戏隐藏窗口进入对局再切回，确认及时显示对局数据；结束一局不点刷新，确认自身新战绩出现，切换其他已打开玩家标签验证按需刷新。
2. 同一机器、同一账号、相近库存状态，对改前版本与改后版本各执行三次“重新读取”，统计 refresh_succeeded 中位数，目标下降 ≥35%；确认每刷新库存端点至多读取一次。
3. 斗魂中操作勇敢举动与自动禁用后立即导出日志，核对 championId=-3 的下发及 ban probe 的 action / requested / status / raw_count，区分“不曾调用”与“客户端返回空”。
4. 使用新 reason / endgame_trigger / cancel_scope 统计位置播报、结算动作和取消来源；不要用旧日志无字段的失败次数推断新版本实机结果。
5. 下次自动匹配“点了没反应”时立刻导出，并记下是否房主、房间人数、是否已选队列、本次启动后是否取消过匹配；按 `auto_matchmaking_enabled`、`skipped_state.reason` 与实际动作结果定位。
6. 全新 Windows 用户资料下直接导出日志，确认保存后按钮变为“打开日志文件夹”，点击能定位；导出时切走设置页再返回也应保留完成状态。
7. 实局征召打开编辑序列，在持续 SSE 更新期间滚动、输入并保存；检查 P6 勇敢举动成功下发，并使用 P9 三态诊断核对弹窗生命周期。

**工作区恢复说明**

执行期间共享目录的 R87 改动两次被其他任务暂存清走。P1–P6 在 `/tmp/r87-recovery`、P7–P9 在 `/tmp/r87-addendum` 保全并继续验证；随后共享目录恢复了原暂存内容。合回前逐文件核对恢复内容与原 stash / 当前 HEAD，仅合入后续 R87 差异与新测试/账本，保留 R86 提交，不重置分支。全部成果保持工作区改动，未发布。
