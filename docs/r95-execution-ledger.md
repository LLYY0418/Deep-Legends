# R95 执行账本

执行日期：2026-09-16。范围：`WORKLIST-R95-ARENA-TRUTH-REFRESH-WIPE-FLICKER-PRO.md`。

## 交付边界

- 本地代码、fixture、真实 Chromium 与变异验证；**不冒充真实 LOL 对局验证**。
- 未提交、未推送。保留用户已有 `desktop/package.json` 的 NSIS toolsets 配置与工单文件。
- 不恢复任何生产名单顺序切块；不把 `teamParticipantId` 当小队；不改轮询间隔、限流配额或 forceHover 两段守卫。
- 真实客户端验收仍需用户打一局，清单见文末。

## 逐项执行

### P0 我的小队、真值与样本

| 条目 | 实现与证据 |
|---|---|
| 18 cell / 15 hidden / 3 unhidden | 真实 `loadGameplayLive` fixture，只有 3 名可见成员，全部 `MySquad=true`；未隐藏 15 人不会被伪造身份。`TestR95SquadSurvivesHiddenChampSelectAndScrambledLive` |
| 跨 ChampSelect / GameStart / InProgress / Reconnect | 上述测试依次走四阶段；打乱 playerlist 让队友跨块，仍保持 3 人标记，其他成员没有推断编号。|
| gameflow 缺本人/队友 | 只用已记住身份与完整 playerlist 精确对应补回已知队友；不补造未知敌队。保留原 session 17 人观察顺序；补回者没有假装已加载历史。`TestR95MissingKnownSquadMemberAndTruth` |
| 全体未知归属说明 | 下发 `arenaMySquadNotice`，顶部说明未知归属；成员用“我的小队”，不以块号/吉祥物冒充真实 subteam。|
| 国服 SGP 真值 | 共用 `arenaGroupTruthInput`；已有 SGP 详情/赛后链喂入 source=sgp；真 SGP HTTP 形状的正、反例均验证事件、18 个 subteam index、17 人 session 不构成顺序证据。|
| 国际服真值 | 复用已取得的 KR match-v5 detail，source=riot-match-v5；不新增 HTTP；GameID/QueueID/server/client 约束，避免同号跨服比较。`TestR95TruthNoneAndRiotSource` |
| 真值验实际标记 | 从最终 response 中 `MySquad` 成员取验证集合，不拿记忆名单自证正确；错分 subteam 必须 false。|
| none / partial | provider 缺失、取消/未发布也记 none；partial 不置 checked，完整样本到达后可再次验证。R90 partial 回归保留。|
| cellId 样本 | `my_team_unhidden_cell_ids`、`my_team_cell_id_sequence`，保留 cellId=0；R95 集成测试验证 3/18 数组。|
| allgamedata | 每 game 一次，2 秒预算，4 MiB 上限；父 phase 取消不丢采样；只记录协议键/类型/长度分布，未知动态键折叠，嵌套数组保留。`TestR95AllGameDataOncePrivateAndCanceledParent` |
| 17 vs 18 缺员 | `session_missing_from_playerlist` 仅含 index / is_current 和计数；在补员前采样。|
| order-check 证据口径 | `< squadSize*2` 为 inconclusive；测试覆盖 0..5 false、6 true（3 人小队）。|
| 状态隔离 | 非零旧 game 对新 game/未知 game 不标；账号 reference 清理同时清小队/真值记录。`TestR95SquadGameAndAccountIsolation`。|

**工单矛盾处理**：一处要求彻底停用顺序推断，验收又要求生产继续得到 `allies-cross-blocks`。本次以“禁止生产推断”为准，生产记录 `order-inference-disabled`；旧 `arenaAlliesCorroborate` 否决逻辑不改，并用 `TestR95LegacyAlliesCrossBlocksGuardRemainsIndependent` 独立验证跨块拒绝。没有为了某条旧断言重新开启猜测。

**诊断原文的校正**：原有 SGP 就会解码为 `riotMatchInfo`，不能仅凭 Go 类型认定它是国际服专用。本次明确来源、完善 none/逐人结果与实际标记验证，而不是凭类型声称国服原来绝不可能调用。

### P1 刷新不清生涯

- progress 只更新 player / matches / pagination；ranks/masteries/recentPlayers/historicalRanks 保留。
- append 与强刷后 429：显式 null、缺 key、空数组三种噪声都覆盖。fixture 注释明确 **null 不能被省略**。
- 无旧数据首屏 progress=5 条；有旧数据刷新保留旧 matches，避免现有 20 条被中间帧缩成 5 条。
- complete 的显式空数组权威清空；Go JSON 保留三字段，不加 omitempty。
- overview_load_cost / overview_phases_ms 增加 beg_index / stream。
- 验证：`web/r95.test.cjs` P1、`TestR95OverviewEmptyArraysAreExplicit`。
- 可选独立 progress 结构体未引入：本次遵循工单指定的前端白名单修复，保留完整响应 JSON 契约；不把“没算”与“确实为空”的语义混成 omitempty。

### P2 去抖动

- 仅 force/manual 才有延迟加载提示；240ms 内完成永不显示，interval/sse/event 不显示。
- 请求结束、替换、重置清 timer；保留手动刷新按钮反馈。
- `[data-live-status] { min-height:40px }` 固定流中占位；未修改 3 秒轮询周期。
- 真实浏览器执行生产 `loadLive`/`renderLive`，ChampSelect 慢 manual 出现/消失完整周期连续采样 body top。
- 原版 body top span=0；删占位 M4 span=46px，真实断言失败；空闲串改 x 的 M5 保持 span=0。
- JSDOM 负责 force/source 与 239+1ms/10ms timer 逻辑；**布局结果不采用 JSDOM**。
- 浏览器 fixture 的 phase/live/SSE 一致且连通；详情页签实际可见，行尺寸非零，避免隐藏 DOM 的假绿。

### P3 时间语义

- lock-now 全步骤 0 延迟；show-only/hover 0 延迟；show-then-lock 才读 LockDelayMS 并受剩余时间夹取；ban/pick 同语义。
- intent 仍按原节奏；forceHover hover→回显→lock 守卫与 hover-cleared-by-client 豁免不改。
- 两侧仅 show-then-lock 显示“锁定等待”；步进与手输都写本侧 lockDelayMs。新增测试额外发现并修复了 ban 手输误写 pick 的遗漏。
- “已发送，等待确认”正常路径记 ok，非 warn。
- `TestR95LockStrategiesUseOnlyVisibleWait` 实际执行 wildcard ban，断两条 schedule 延时、两次 PATCH、show-only 不锁、50ms 剩余夹取与记录 kind。
- 工单 A-5 的 Arena PLANNING intent 跳过是**可选建议，未实施**：与本次明确“intent 沿用、不动”保持一致；需下一局证明接口不接受后再单独调整策略，不凭一份日志改变其它队列预选流程。

### P4 职业识别

- 现有内存快照全部合法一队 + 学院/二队，不再仅六队；教练/不自洽归属/冲突账号排除；二队灰色并有后缀。
- PUUID 精确优先；完整 Riot ID 大小写不敏感精确匹配；PUUID 冲突 tombstone 不得被 RiotID fallback 绕过；禁止模糊/前缀猜归属。
- 只 KR；空/超过24小时快照不识别，不发新目录 HTTP。
- 对局、战绩详情、当前对局、玩家总览从后端已有身份出 badge；公开徽章仅四字段；maskNames 同时隐藏职业身份。
- 玩家按钮传职业 context，非目录点击也进入既有职业组；后端 overview 命中后也迁移组。
- `pro_identity_match` 命中与 none 都记 surface/region/candidates/matched_by/secondary，不记身份。
- fixture：Faker#구라티 → Guti/T1 Academy，Hide on bush#KR1 → Faker/T1，Faker#different 不命中，CN 不标，PUUID 优先、空缓存0请求、公开JSON不含 PUUID、目录与badge一致、冲突拒绝。
- 900px 双列下账号名沿用既有省略号；徽章单独换行完整显示。实测移除/加入职业徽章，名字宽度同为115px（其它五断点同为146px），证明徽章未额外挤压；tooltip 仍可查看完整账号名。
- 前端 mask/secondary/context/click 回归与六断点真实截图；极端行同时显示“自己 + 预组×3 + 我的小队 + HLE Gumayusi”，不改 live grid 模板。
- **BRO IDEA、DK Despair 在工单提供目录样本中缺失，不能识别。不放宽匹配来掩盖缺口。**
- `docs/pro-players-sources.md` 已区分人工核对名单与完整来源目录；本轮未声称实时重新核验外部账号归属。

### P5 动作汉化

- 自动禁用 / 自动选用 / 备战席换英雄 / 英雄交换；未知统一“自动规则”，不用英文键回退。
- 时间按 start 相对显示（含 start=0）；champselect fired 计入总次数，与最近一次一致。
- 工单“删 pick 映射，仅测无英文就应红”逻辑不足：删除后 fallback 仍是中文“自动规则”。新增测试同时检查**准确的“自动选用”**，因此该变异确实 kill。

## 测试基线变更说明

- `diagnostics_2351_test.go:128`、`diagnostics_2143_test.go:36`：保留，实际并未因为新语义失败；不为工单预测“必失败”而捏造修改。另增 R95 显式 schedule=0 断言。
- `champselect_takeover_test.go`：原默认10秒锁等待测试改验证“默认等待被剩余回合夹取”，仍真实执行并检查锁定。
- `r78_test.go:35`：保留，实际无需变更原断言；R95 独立验证 ban LockDelayMS 不丢失。
- `champselect_execution_test.go`：旧写入行为 fixture 为 ban 显式指定零 LockDelayMS，避免被新默认等待干扰；生产默认值不改。
- `r91_addendum2_test.go`：普通ban clear预检 fixture 改用 LockDelayMS=120，仍验证人工切换会取消，不动 forceHover 豁免。
- `gameplay_test.go`、`r90_test.go`、`r91_addendum_test.go`：旧“无真 subteam 仍按顺序分组”的预期改成 flat/unavailable；probe重试、缓存、身份、隐私、phase、真值正反检查继续保留。
- R90 truth partial 先 inconclusive、完整后再次验证；允许取消时先 none、稍后SGP完整事件。非推断观察的 verified_by_allies=false，不再冒充已证实块序。
- R49 文案断言更新为工单要求的“我的小队”与未知归属完整说明，未改 grid 断言。
- 前端 R87/R88/R90/R91/R94/champions/gameplay harness 加入真实新 helper；局部 fixture 明确 settings，避免缺模拟依赖的 TypeError；不降低断言。
- R91 手动刷新即刻仍无“正在刷新”，240ms后必须显示；InProgress原有空闲文案常驻，不能误断 status 元素必不存在。
- `desktop/overview-render.test.cjs`：先切 ban show-then-lock 再输入本侧 lock 框，检查保存1250ms和10000ms夹取；与新策略表一致。
- `pro_players_test.go` / `pro_players_supplement_test.go` 的合法 KR 账号 fixture 加 `Region:"kr"`；非 KR 拒绝规则不放宽。

## 验证与完整输出

最终：Go `-count=1` **PASS（72.144s）**；`-race ./...` **PASS（88.618s）**；Node **696 项：695 pass / 0 fail / 1 既有 Windows/PowerShell 专属测试 skip（macOS）**（232.260s）；两项 JS syntax check PASS；真实 Chromium PASS；21 项变异符合预期。生产源码指纹 `0a9ffb2f0c10`。

所有日志在 `docs/r95-validation/`，不是仅摘要：

| 命令 | 输出 |
|---|---|
| `node --check web/gameplay.js` | [node-check-gameplay.txt](r95-validation/node-check-gameplay.txt) |
| `node --check web/suite.js` | [node-check-suite.txt](r95-validation/node-check-suite.txt) |
| `node --test web/*.test.cjs desktop/*.test.cjs` | [node-test.txt](r95-validation/node-test.txt) |
| `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go test -count=1 .` | [go-test.txt](r95-validation/go-test.txt) |
| `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go test -race ./...` | [go-race.txt](r95-validation/go-race.txt) |
| `node desktop/r95-browser.cjs` | [browser.txt](r95-validation/browser.txt)、[布局数据](r95-validation/browser/layout.json)、[徽章边界数据](r95-validation/browser/badge-measurements.json) |
| `python3 docs/r95-validation/run-mutations.py` | [逐项结果](r95-validation/mutations.txt)、[JSON矩阵](r95-validation/mutations/results.json)；每项独立完整输出在 mutations 子目录 |

Go 需本地 httptest 绑定端口，沙箱 bind denied 不视为业务失败，最终在获准环境重跑。未更改 GOROOT；临时目录不是挂载 sessions 路径。

## 逐项变异实跑矩阵

20 kill + 1 按要求 survive；无编译失败/超时冒充 kill。源码通过临时副本/Go overlay 注入，不就地破坏工作区。

| 变异 | 结果 | 耗时 |
|---|---|---|
| [P0-M1-remove-positive-mark](r95-validation/mutations/P0-M1-remove-positive-mark.txt) | kill | 9.56s |
| [P0-M2-clear-on-rejection](r95-validation/mutations/P0-M2-clear-on-rejection.txt) | kill | 7.18s |
| [P0-M3-remove-SGP](r95-validation/mutations/P0-M3-remove-SGP.txt) | kill | 39.03s |
| [P0-M4-constant-correct](r95-validation/mutations/P0-M4-constant-correct.txt) | kill | 4.76s |
| [P0-M5-silent-none](r95-validation/mutations/P0-M5-silent-none.txt) | kill | 5.62s |
| [P1-M1-spread-partial](r95-validation/mutations/P1-M1-spread-partial.txt) | kill | 0.65s |
| [P1-M2-omitempty](r95-validation/mutations/P1-M2-omitempty.txt) | kill | 4.92s |
| [P2-M1-all-visible](r95-validation/mutations/P2-M1-all-visible.txt) | kill | 0.66s |
| [P2-M2-none-visible](r95-validation/mutations/P2-M2-none-visible.txt) | kill | 0.68s |
| [P2-M3-zero-delay](r95-validation/mutations/P2-M3-zero-delay.txt) | kill | 0.71s |
| [P2-M4-remove-reserved-height](r95-validation/mutations/P2-M4-remove-reserved-height.txt) | kill | 5.03s |
| [P2-M5-nonempty-idle](r95-validation/mutations/P2-M5-nonempty-idle.txt) | survive | 4.62s |
| [P3-M1-old-delay](r95-validation/mutations/P3-M1-old-delay.txt) | kill | 5.66s |
| [P3-M2-all-time-inputs](r95-validation/mutations/P3-M2-all-time-inputs.txt) | kill | 0.6s |
| [P3-M3-discard-ban-lock-wait](r95-validation/mutations/P3-M3-discard-ban-lock-wait.txt) | kill | 5.94s |
| [P3-M4-happy-warn](r95-validation/mutations/P3-M4-happy-warn.txt) | kill | 5.71s |
| [P3-M5-type-ban-into-pick](r95-validation/mutations/P3-M5-type-ban-into-pick.txt) | kill | 0.62s |
| [P4-M1-fuzzy-name](r95-validation/mutations/P4-M1-fuzzy-name.txt) | kill | 5.68s |
| [P5-M1-remove-pick-title](r95-validation/mutations/P5-M1-remove-pick-title.txt) | kill | 0.68s |
| [P5-M2-English-fallback](r95-validation/mutations/P5-M2-English-fallback.txt) | kill | 0.69s |
| [P5-M3-exclude-champselect-count](r95-validation/mutations/P5-M3-exclude-champselect-count.txt) | kill | 0.64s |

## 仍需真实客户端验收（不能由 fixture 替代）

- [ ] 国服斗魂 ChampSelect 可见 3 人“我的小队”。
- [ ] 加载画面 / InProgress / Reconnect 仍保持 3 人，其他人不猜组。
- [ ] 打完后有 truth_source=sgp、my_squad_correct=true 的 arena_group_truth_check。
- [ ] 查看 cellId 全序列与 unhidden cellIds；是否连续仅作为观察，不据此开启生产推断。
- [ ] 检查 allgamedata 的小队字段样本及 session 缺员 index/is_current。
- [ ] 立即禁用两条 schedule delay_ms=0，forceHover 回显→锁定无反复 compatibility-hover-unconfirmed。
- [ ] 真实 KR 战绩详情/当前对局点职业玩家，身份和职业组正确；国服不出现职业身份，maskNames 不泄露。

截图：[620px](r95-validation/browser/pro-live-620.png)、[900px双列](r95-validation/browser/pro-live-900.png)。图片为隔离 fixture 使用生产 DOM/CSS 的浏览器实测，不是游戏客户端实拍。
