# R168 执行账本

日期：2026-09-26。基线源码版本 0.12.32；本轮源码版本 0.12.33。

## P1 征召运行时缓存

`deep-legends:gameflow` 到来时，无论是否停留在征召区块，都使 `champSelectRuntime` 失效；只有区块正在显示且已连接时才立即读取。离开征召页期间选人结束后，重进会重新请求 `/api/champselect/state`，不会沿用旧队友选择。Node 用例覆盖离开期间阶段变化、返回后的真实请求，以及一直停留在征召页的单次刷新。

## P2 R167 真机复核与候选诊断

按 R168 工单所载截图和 `0926-2043` 日志，440 灵活组排的选人阶段敌方锁定可实时看到；上单齐天大圣在 `FINALIZATION` 前约 92 秒锁定。已有日志不能证明本人尚未悬停时 R167 的候选请求是否触发，也不能证明卡片实际渲染。详细时间线与证据边界见 [r167-execution-ledger.md](r167-execution-ledger.md)。本轮工作区没有原始截图或日志文件，因此沿用工单诊断记录，不称作本轮重新验出的真机事实。

`ensureLaneMatchupCandidates` 新增专用 `lane_matchup_candidate_fetch` 事件；前端发出状态为上下文不足、本人已选、请求、飞行中、缓存、成功、失败，重复轮询去重。后端白名单接收并仅记录敌方英雄 ID、队列、场次、位置、段位、返回行数、HTTP 状态和错误类别。Go 端测试检查事件能入日志且不记录夹带的玩家标识；Node 测试覆盖请求去重、成功、缓存、失败与跳过。本人悬停后停止候选的 R167 行为未改。

## P3 R119 真机补位证据

`0926-2043` 日志的匿名 `selected_role_counts` 包含一条 `UTILITY.AUTOFILL.JUNGLE.MIDDLE.FILL`。用户已确认该局青钢影辅助确实补位，但匿名聚合值本身未绑定 PUUID。随后 `0926-2139` 日志已给 P3-2 历史战绩口径填入首个真实样本：该补位落入 `position_mismatch`，`autofill_candidates=0`；23 条快照候选数均为 0。两种来源及其局限已并记于 [r119-execution-ledger.md](r119-execution-ledger.md) §5，修正了旧段落仍称“待填”的矛盾。

`SelectedRole` 若要成为实时个人标签，必须按 PUUID 精确绑定，并先在 420 与非排位模式验证字段稳定性。本轮不将匿名聚合值归到个人，不打开 `autofillLabelGate`，不改变历史推断判据。

## P4 实时身份与当前模式战绩

后端 `Hidden` 只代表 LCU 明确给出 `NameVisibilityType=HIDDEN`；姓名暂缺时另给 `identityUnresolved`。前端实时玩家名、玩家列表名、详情行、身份徽章和空战绩文案分别显示“身份待公开”及“身份尚未公开”，真实隐私继续显示“隐藏身份”。历史对局自身的 `hidden` 语义未改。Go/Node 测试覆盖两种成因和对应 JSON、界面结果。

实时每位玩家的 LCU/SGP 战绩窗口由最近 10 场扩大到 30 场，再按当前队列筛选，展示上限仍是最近 8 场。LCU 仍是每人一次列表请求、`details=false`，没有新增逐场详情请求；SGP 回退窗口同步调整。既有 `live_load_cost` 记录 `matches_ms`/`total_ms`，但工单给出的原始诊断文件不在当前工作区，不能从它计算基线耗时。30 场是用单次请求换取混合队列命中率的上限选择；真机仍需观察耗时和队友局数。Go 用例构造前 10 场只有 2 场 440、更早还有 4 场 440 的形状，断言计为 6；另测确实只有 2 场时保持 2，且两例均只请求一次 `endIndex=29`。

## 验证与交付边界

- 定向 Go `TestR168`：通过；前端全套 `node --test backend/web/*.test.cjs`：733/733 通过。
- `go test ./backend -count=1` 最终全套通过（197.829 秒）；诊断原因白名单调整后的 R168 定向 Go 测试也通过。`git diff --check` 通过。
- 公开模式（**key mode: public**，`main.riotAPIKey` / `main.riotAPIKeyCipher` 均为空）构建 macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.33-public` 与 Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.33-public.exe`。两者均通过 `verify-build-fingerprint.cjs` 的 `badca4155be6` 校验；macOS `-self-test` 输出版本 0.12.33 并通过。仅保留带 `-public` 后缀的验证产物，不制作或发布安装包。
- R167 卡片显示、P4 真机局数与延迟仍需 Windows 真机复核。

## 后续调整：详情战绩条数（2026-09-26）

用户要求对局详情最多展示最新 10 条当前模式战绩。后端摘要与前端展示上限统一调整为 10，并按 `CreatedAt` 降序稳定排序后筛选；时间相同的场次保留来源顺序。LCU/SGP 单次读取 30 条混合队列的窗口保持不变，以便从中找到足够的当前模式场次；30 条不会直接展示。

源码版本 0.12.35。Go 定向测试覆盖混合队列及来源乱序时取最新 10 场；前端测试覆盖详情最多渲染 10 条。`go test ./backend -count=1` 全套通过（193.117 秒），前端全套 `node --test backend/web/*.test.cjs` 734/734 通过，`git diff --check` 通过。公开模式（**key mode: public**，`main.riotAPIKey` / `main.riotAPIKeyCipher` 均为空）构建 macOS arm64 `/private/tmp/Deep-Legends-backend-0.12.35-public` 与 Windows amd64 `/private/tmp/Deep-Legends-backend-0.12.35-public.exe`；两者均通过源码指纹 `ac3548b78320` 校验，macOS `-self-test` 输出 0.12.35 并通过。仅保留带 `-public` 后缀的验证产物，未制作或发布安装包。
