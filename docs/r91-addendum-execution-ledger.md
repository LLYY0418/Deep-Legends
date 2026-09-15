# R91 主工单与 Loading / 静默额度补充执行账本

2026-09-14。覆盖 `WORKLIST-R91-KR-QUOTA-ARENA-BAN-FLICKER-GROUPING.md` 的 P1–P9，以及 `WORKLIST-R91-ADDENDUM-LOADING-GROUPING-SILENT-QUOTA.md`。额度提示以补充工单为准：仅 429 完全静默，500/503 保留可见失败。以下为代码与本地模拟验收；真实 Windows 客户端的验收单独列在末尾。

## 2026-09-15 更正

独立验收发现 P4 清零豁免范围过大，以及最后的 CSS 对齐调整未同步到 R87 Go 基线。这两项已按第二份补充工单修复；当前结论与完整逐项测试日志以 [ADDENDUM-2 执行账本](r91-addendum-2-execution-ledger.md) 为准。下方保留上轮历史证据，不再将最后一次 CSS 修改前的 race 结果视为最终源码全量通过。

## 逐项交付

| 项目 | 最终实现 | 验证 |
|---|---|---|
| P1 合并查询 | 默认 20 场只走一次 ID / 详情流水线，同一 NDJSON 响应先发布 5 场、再发布完整汇总。分页预览不提前推进游标。关闭或替换请求取消流读取。 | Go 真实 HTTP 流测试在收到 5 场前锁住后续详情，最终断言 1 次 IDs / 20 次详情；JS 断言 1 次 count=20 请求、5→20 和 20→25→40。 |
| P1 缓存 | 成功详情分别计入 `matches_from_memory`、`matches_from_disk`、`matches_from_network`，沿用不可变详情持久化。 | 同一 ID 网络→内存；重建 provider 后磁盘命中，累计仅 1 次网络请求；删除持久化或计数的变异失败。 |
| P1 补充静默 429 | JSON/HTTP 头与已开始流的 error 帧携带类型、Retry-After。首屏和翻页保留数据，不写 warning/moreError、不 toast、不 autoPaused；按期限后台重试。页签关闭、请求替换、重置、筛选/场数变化使旧重试失效。连续上游 429 保留最后一次 Retry-After。 | fake timer 验证 4999ms 无请求、5000ms 自动重试；500/503 首屏和翻页继续显示错误；截断流仍可见；删除短路 return 必须失败。 |
| P2 伤害/承伤 | 普通和斗魂主区从相同顶部开始，斗魂复用三行统计轨道；统一窄列行距。卡片及详情使用整数，伤害为 danger、承伤为 muted。 | Chromium 三种宽度首行 top 完全一致；四种宽度的斗魂详情展开/收起与滚动通过；单位、颜色和布局断言通过。 |
| P3 生涯默认值 | draft、rankLabel 和未保存判断统一 UNRANKED / I；缺字段显示“未定级 · 单双排”。 | jsdom + Chromium 下拉值、预览与未编辑状态一致，无 undefined。 |
| P4 亮出被清零 | 仅已确认 hover 的提交决策 `ForceHover=true`（wildcard 禁用确认路径）被清零时保留 armed 锁定并记录一次 `hover-cleared-by-client`；普通路径清零恢复手动让位及队友已选/已禁用豁免，另一非零英雄仍立即让位。执行前保留回合、会话、设置与候选可用性校验。 | 模拟 wildcard ban 真实 PATCH，0 时继续锁定、104 时让位；恢复旧判据或旧 preflight 都变红。 |
| P5 刷新闪烁 | 同局同队列 pending 结果保留已读战绩，pending 仍可后续补齐。内容未变时只更新刷新状态，名单 DOM 节点不替换。首次无数据继续显示骨架。 | jsdom 文本与节点身份断言；Chromium 刷新前后战绩一致；恢复骨架覆盖/无条件重绘均失败。 |
| P6 分组与取证 | GameStart 走纯 session-order，不探测 2999；InProgress / Reconnect 保留游戏进程优先。每次 Loading 的首份 session 独立强制 shape 采样；无 gameId 时新阶段仍可采样。所有实际分组拒绝带 reason/unavailable，可重试情况单独标记；未尝试的已知斗魂阶段结束记录 `phase-not-attempted`。 | Loading 18 人分组成功且 0 次 2999；同局丢失中间一人仍保留原边界，新 gameId 不借用。强制采样、漏尝试与失败原因均有行为测试及变异。 |
| P6 首分钟与沿用 | 仅沿用同客户端/账号/区服/队列/有效 gameId 的 Loading 成功分组；17 人没有完整边界时不硬切。各游戏阶段前 60 秒未分组每 5 秒探测，之后保持 20 秒 × 8 次，完整结果停止。后端短缓存避免吞掉早期探测。选人按 CellID 保存完整英雄顺序，与 session/playerlist 比对，仅记录统计。 | 前端模拟 12 次短探测不消耗后续 8 次预算；后端 4 秒后能够再次探测；原 17 人 gameflow + 18 人 playerlist 边界测试继续通过。 |
| P7 斗魂玩家段位 | Arena 玩家行不输出段位，普通模式保留。 | jsdom + Chromium，恢复段位变异失败。 |
| P8 征召说明 | 删除 arena 分组下“斗魂禁用环节需真机确认”的 warning。 | Chromium arena 页面无该 warning，恢复变异失败。 |
| P9 保存后重评估 | 保存 watch 设置后读取当前 phase 并调用既有阶段调度，支持重连、接受、再来一局。UI 区分尚未到阶段和已到阶段但未开规则。 | Reconnect / ReadyCheck / EndOfGame 保存后立即出现对应 pending；删除重评估变异失败。 |

补充口径：完整 18 人且身份有效的 session-order 仍应成功。17 人且无有效完整边界时同时记录 `playerlist-unavailable` 和 `roster-not-divisible`；不能为满足主工单中人数/原因对应不一致的文字而把完整有效名单强行判失败。

## 测试与证据

- 历史运行：`GOCACHE=/tmp/deep-legends-go-cache go test -race ./...` 曾通过，117.454 秒，见 [当时的 Go race 输出](r91-addendum/go-race.txt)。该运行早于最终 CSS 顶部对齐修改，之后只重跑了前端布局测试，漏掉 `r87_test.go` 的旧字符串断言；此前将其表述为最终源码全量通过不准确。最终修复后的非 race / race 完整输出见上方 ADDENDUM-2 账本。
- `node --test web/*.test.cjs desktop/*.test.cjs`：677 项，676 通过、0 失败、1 项 Windows PowerShell 专用检查跳过。见 [全量前端输出](r91-addendum/frontend.txt)。
- 最后的 CSS 顶部对齐/窄列行距改动后，相关 `web/champions.test.cjs` + `web/r88.test.cjs` 243 项全部通过。见 [布局回归输出](r91-addendum/layout-tests.txt)。R88 样式基线只更新本工单批准修改的主区顶部、行距与 Arena 三行统计规则。
- `go vet ./...`、JS 语法检查、`git diff --check`：通过。
- 隔离源码变异：23/23 个均被行为断言拦截。运行 [变异脚本](../scripts/r91-addendum-mutations.py)，结果与每个实际断言失败输出见 [结果](r91-addendum/mutations/results.json)。不以编译错误、超时或启动失败冒充变异通过，真实源码不被改写。
- [Chromium 验收脚本](../desktop/r91-addendum-browser.cjs) 使用生产 JS/CSS 和本地模拟数据；测试接口注入仅存在于 fixture HTTP 响应，不进入生产源码。卡片图标为明确的本地占位，截图不用于证明远端资源连通性。
- [布局数值](r91-addendum/browser/measurements.json)：2400/1500px 下两种卡片首行均为 123.59375px；1280px 下均为 201.59375px。伤害 `rgb(224,92,92)`，承伤 `rgb(139,147,166)`。
- [大屏卡片](r91-addendum/browser/cards-2400.png)、[窄列卡片](r91-addendum/browser/cards-1280.png)、[生涯默认段位](r91-addendum/browser/facade-default.png)、[斗魂征召](r91-addendum/browser/arena-config.png)、[斗魂刷新战绩](r91-addendum/browser/arena-history.png)。
- 斗魂详情另用现有 Chromium runner 验证 1920/1280/820/620px、21 玩家、7 队、窄屏横向滚动与稳定兄弟卡片：[数值](r91-addendum/browser/detail/measurements.json)、[1280px 截图](r91-addendum/browser/detail/arena-detail-1280.png)。此处仅为 demo 布局样本，不代表真实斗魂人数结论。

修正过的旧测试契约：429 无法在当前预算内等完时保留实际 429/Retry-After，旧“必然 DeadlineExceeded”断言随之更新；旧预热取消测试的“非预热阶段”从现在已支持预热的 GameStart 改成 Matchmaking。未关闭 race 检查或削弱取消/缓存隔离断言。

## 真实客户端待验收

1. 正常完成一局斗魂，观察 Loading 或稍后的 InProgress 是否出现小队；导出强制 `lcu_gameflow_session_shape`、`arena_group_order_rejected`、`live_roster_shape` 与赛后顺序核验记录。模拟数据不证明当前客户端真实名单顺序稳定。
2. 连开三个韩服选手页并重启应用重查，确认没有 429 可见提示，诊断中有磁盘命中。模拟测试已证明缓存跨 provider 重建；这里还需核对打包后的真实存储路径与上游行为。
3. 选人停留 30 秒观察战绩不闪，完成一次自动禁用确认实际锁定；检查 hover 清零诊断且没有错误的手动接管。
4. 在掉线前开启自动重连，再验证真实客户端触发。当前测试覆盖保存后处于 Reconnect 的即时调度，不代替客户端操作效果。

本次未打包、发布或执行真实客户端对局操作。原工作区已有大量 R87–R94 修改，均保留；[本轮独立补丁](r91-addendum/scoped-changes.patch) 以开始本轮时的文件副本为基线，便于与已有修改区分。本轮当时的来源指纹见 [历史 SHA-256 清单](r91-addendum/source-sha256.json)。
