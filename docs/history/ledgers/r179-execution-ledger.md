# R179 执行账本

日期：2026-10-01。基线 0.12.43，本轮版本 **0.12.44**；desktop/package.json 与 package-lock.json 的两处版本已同步。源码指纹 **300fa449afb5**。

工单：WORKLIST-R179-CHAMPSELECT-ENEMY-PLACEHOLDER-STARTER-ITEMS-WITH-RUNES-CHAMPSELECT-PREMADE-AND-LIVE-FLICKER.md。

## 既有真机证据

R180 更正：下表 R178 P2 的抽样事实仍成立，但由此推得“不陈旧”的结论作废；抽样人数、SGP 五分钟缓存及只比全模式最新时间不足以排除当前队列缺局。后续修复与诊断见 [R180 账本](r180-execution-ledger.md)。

以下结论依据工单提供的 `lol-loot-diagnostics-1001-2027.jsonl` 分析摘要：0.12.43、run d984a5ec…、build d5961921e9b6、11:47–12:27Z，game 9009758600 / 9009800745。本轮没有重新取得该日志原文件，也没有把自动测试当成真机对局。

| 项目 | 真机已验证的结论 |
|---|---|
| R177 P1 | 16 条 rune_starter_batch 的 rate_limit 全为 0，前台出门装请求不再被后台额度挡住。 |
| R177 P2 | 两场对线候选均有 requested → succeeded、rows=5，敌方分别为 101/mid、81/adc；卡片请求已能触发。 |
| R178 P1 | game 9009800745 两队 team_duplicate_positions 均为 0；12:20:28 为 snapshot 5 + gameflow 5，12:20:39 补探测后为 liveclient 10。 |
| R178 P2 | 全部抽样 lcu_missing_newer=0；newest_any_age_min=26.73 对 newest_queue_age_min=34459.88。R180 已指出该抽样不足以排除当前队列缺最新一局，原“不陈旧 / 仅队列筛选”结论撤销。 |

这是 R177/R178 账本中此前“未真机验证”状态的后续证据。R179 保留当前队列筛选、最新同队列最多 10 条的展示规则。

## P1：选人敌方占位

- gameplay.js 的 renderLivePlayer 接收实际 phase；只在 ChampSelect、非本人/己方且阵营与本人不同的敌方卡片显示单一“暂无玩家信息”。
- 保留英雄头像或原占位头像和原卡片高度；不输出隐藏身份标签、位置/段位、统计列、上下文及底部重复说明。
- InProgress 隐藏玩家、选人己方隐藏玩家仍走原渲染路径。用修改前 HTML 黄金夹具核对原输出。

## P2：符文与出门装稳定

- 绝活哥请求键统一为 championId:position；核对后端结果只依赖这两个字段。职业选手原键已符合要求，保留。页签、flight/failure 与出门装 targetKey 共用对应请求键。
- 绝活哥写缓存前调用 retainRuneStarterItems，按同英雄同分路旧数据继承同 key/playedAt 的出门装；同技能/阶段变化不触发额外符文请求或加载态。
- specialistRunes 完成扫描后，只为第一位绝活哥前 3 场异步预取 timeline；2 个 worker，使用既有前台 6 秒排队上限，独立总期限 30 秒，不等待预取才返回符文。保留扫描 36 次预算，额外最多 3 次，即 39。
- 返回前仅通过 lookupReady 读取已完成的内存/磁盘缓存，不等待进行中的 flight，不造出门装值。
- 前端第二阶段继续补未命中行及其他页签；预取与第二阶段共用 cache.loadWithStatus，按 match 合并上游请求。
- rune_starter_batch 增加 duration_ms、prefetched；specialist_runes_done 增加 prefetch_started。prefetched 只计实际由成功预取 loader 产出的命中，不计失败的预取占位，也不计前台 loader 抢先产生的缓存。worker 即使 panic 也标记结束，以免永久占位。

## P3：选人己方组队

- ChampSelect 从现有 lobby 端点读取 members.puuid（包含本人），只有包含本人的名单才作为直接组队证据；成员标记 source=lobby、sessionSignal=true。
- 对己方非 lobby 成员继续使用原历史共同对局及覆盖率门槛；选人敌方不进行组队推断，斗魂逻辑不改。lobby 失败静默保留可用历史推断。
- 同一局按稳定成员集合复用组号，进入对局或阵营顺序变化后匹配的成员组号不跳变。复用原直接组队呈现，没有新增 tooltip 文案。

## P4：局部渲染与诊断

- runtime 和后端客户端事件白名单接通 live_render_rebuild，仅透传有限 counts/sources、total、windowMs/phase、imagesRecreated/rowsReplaced；日志使用 window_ms、images_recreated、rows_replaced。未知键和身份字段丢弃，不记录 gameId。
- 名单以稳定玩家键比较每行的规范 HTML；同一名单结构只替换内容变化的行，未变的行根节点保留。变化行里来源未变的图片继续使用旧节点；人数、分组或阵营顺序等结构变化允许重建。
- images_recreated 计同来源旧图被新 img 替代的数量；rows_replaced 计替换的玩家卡行。诊断合并窗口记录真实操作计数。
- 符文面板更新时保留未变的比赛行和图片节点；装备行固定 48px，高度不因出门装补入而变化，横向溢出使用滚动，保留 R174 的技能位置。
- 只有 manual 加载显示“正在刷新…”；interval/sse/event 等后台来源保持“刷新对局”。请求被已有加载挡住时也不覆盖当前手动来源。
- 局部 DOM 复用后对事件绑定去重；连续重新绑定后点击仍只执行一次，避免重复应用/重复请求。

## P5：对线诊断字段

核对发现实际缺口在前端 recordItemSetClientDiagnostic 的直接请求格式化：调用方虽传了 camelCase 的四个字段，格式化输出却遗漏。补入 selfPosition、enemyLockedCount、enemyPositionKnownCount、allyPositionKnownCount，并接通 runtime 对应事件白名单；后端现有 snake_case 映射保留。

测试由真实前端 formatter/runtime 产生请求夹具，再交给真实后端 handler 与诊断存储，日志中核对 self_position 与三个非零计数完全一致。保留本人悬停后停止候选推荐的既有规则。

## 自动验证

- R179 Node：10 项通过；实际缓存请求、并发调用、继承、第二阶段、DOM 引用、按钮和诊断传输均有断言。最终定向日志 /private/tmp/r179-node-target-final.log。
- 模拟 3 分钟、61 个名单 tick、10 次单行英雄变化：每次只替换变化行，其余 9 行根节点和图片保持引用，rows_replaced=10、images_recreated=0。另以真实符文渲染模拟 4 次技能变化和 ChampSelect → InProgress，出门装节点引用不变。
- R179 Go：4 项通过；真实扫描触发首位前三场预取、阻塞 timeline 时列表仍返回、同时第二阶段合并为每场一次上游、仅附带已完成缓存、lobby 成功/仅本人/失败、历史推断、敌方禁止、阶段组号、真实诊断落盘、失败预取不冒充命中。
- R173 + R179 最终定向：通过，1.268 秒；/private/tmp/r179-go-target-recheck.log。
- 全量 Node：**776/776 通过**，15.689 秒；/private/tmp/r179-web-full.log。
- 最终 go test ./backend -count=1：通过，194.597 秒；/private/tmp/r179-go-full-final.log。
- 最终 go vet ./backend：通过；/private/tmp/r179-vet-final.log。
- git diff --check：代码与账本收尾均通过。

旧 Node 聚焦 harness 补载新增真实 helper，保留原断言；请求键、诊断 gameId 与固定装备行测试按本工单新要求更新。首轮全量 Go 揭示 R173 测试仍假定列表不会启动任何 timeline，并在加入 flight 返回后立即检查磁盘：更新为允许异步预取、列表仍必须及时返回；等待预取 worker 完成持久化再核对新 provider 缓存命中，仍断言上游只请求一次。没有为测试改动生产缓存发布顺序。

## 变异验证

使用 /private/tmp/r179-mutants 的 Node 源码 overlay 与 Go -overlay，生产文件不受变异覆盖。以下 7 项全部是运行时断言 FAIL，语法或依赖错误不计为通过。

| 变异 | 检出结果 |
|---|---|
| 删除 ChampSelect 条件 | InProgress 隐藏玩家原 HTML 与黄金夹具不一致。 |
| 请求键放回 spellKey | 换技能找不到原缓存行，节点/请求稳定性断言失败。 |
| 去掉 retainRuneStarterItems | starterItemIds 变为 undefined，期望 [1055]。 |
| 去掉进行中请求合并 | 阻塞时上游请求变为 4，期望 2；预取/前台重复。 |
| 去掉选人己方限制 | lobby-two、self-only、lobby-failed 都给敌方分组，断言失败。 |
| 恢复整面板 innerHTML 替换 | 未变玩家行根节点不再是同一引用。 |
| 后台也切换刷新文字 | 得到“正在刷新…”而非“刷新对局”。 |

对应日志：/private/tmp/r179-mutants/{no-phase,spell-key,no-retain,no-singleflight,enemy-groups,whole-panel,background-button}.log。

三路 default 子代理探索与三路只读独立核验已完成；重复绑定与预取计数边界已按核验意见修复并补测。代码修改和最终验证由主代理执行。

## 构建

- key mode=**public**；CGO_ENABLED=0，显式 main.version=0.12.44、main.buildFingerprint=300fa449afb5、main.riotAPIKey=、main.riotAPIKeyCipher=。
- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.44-public，构建通过。
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.44-public.exe，构建通过。
- 两份产物均通过 verifyBuildFingerprint 与 verifyRiotKeyPolicy(..., "public")，最终生产源码指纹仍为 300fa449afb5。仅保留带 -public 的可识别名称，没有发布安装包。
- macOS 使用空 RIOT_API_KEY、隔离 /private/tmp/r179-selftest 自检通过：0.12.44、奖池 554、哈希 dee5e21f5234；日志 /private/tmp/r179-selftest.log。

## Windows 真机待验

本轮未运行 Windows 真机对局，R179 的视觉与实时操作验收仍需用户完成；不把既有 .43 日志当成 .44 验收。

1. 选人敌方五卡只显示“暂无玩家信息”；进入对局隐藏身份玩家恢复原“隐藏玩家”。
2. 第一位绝活哥前三场出门装出现；应用符文/技能及进入对局后节点不闪、出门装不丢。
3. 和朋友组队排位，选人己方已有组队标记，进入对局标记保持。
4. 详情停留 1–2 分钟，队友换英雄仅对应一行变化，其他头像和卡片稳定；后台刷新文字不跳动。
5. 导出新日志，确认版本 0.12.44 / 300fa449afb5，live_render_rebuild 已落盘，并查看 P2/P5 新计数。
