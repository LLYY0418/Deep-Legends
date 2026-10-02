# R177 执行账本

日期：2026-10-01。工单：WORKLIST-R177-RUNE-STARTER-QUOTA-STARVATION-AND-R167-ENEMY-LANE-INFERENCE.md。基线 0.12.41，本轮版本 0.12.42，package.json 与 package-lock.json 两处版本同步。源码指纹：8ad20599d929。

工单提供的 1001-1656 日志摘要作为本轮问题证据；本轮没有把假上游回归当成该日志的真机验证。

## P1：出门装准入、冷却、有限重试及诊断

- handleGameplayRuneStarters 与 specialistMatchStarters 的 Riot 请求使用 withRiotQueueLimit(ctx, 6*time.Second)，去掉 withRiotBackground。admitRiotBackground、limit/3、specialistRuneRequestBudget=36 不变；出门装独立最多 9 条，并发 2（specialist/pro 的可见开局装备批次都限定 2）。
- errThrottled 不写 starterFailures；上游 429 类型先归一为 errThrottled，保留 retryAfter，避免长期 Retry-After 的 429 被误算为对局失败。404/500、网络和上游 JSON 解析等其他错误仍沿用 90 秒冷却。
- 用请求 context 的独立批次 tracker 聚合限流行数及最大的 retryAfter（5–60 秒）；有行限流时顶层响应增加 retryAfterSeconds，否则字段不存在。
- 每个 rune-starters 请求收尾记录 rune_starter_batch：source、rows_requested、success、rate_limit、other_failed、retry_after_s、queue_wait_ms。队列等待复用现有 riotOverviewCostTracker，含 FIFO 等待和实际额度等待。没有 matchID、行 key 或玩家身份；原 rune_starter_items 单条事件保留。
- ensureRuneStarterItems 对仍可见且缺出门装的行，按 5–45 秒提示安排最多 2 次定时重试；定时重试绕过 90 秒完成保护，但保留 pending 单 flight。网络/超时或无 retryAfterSeconds 不安排重试。
- 请求条目保存原行集合；部分成功造成 missing-row key 变化时仍遵循同一批次的等待和次数，避免轮询制造新预算。完成补齐后不再安排重试。
- resetLiveGameScopedState、来源页签切换与主页面离开清除定时器；目标键变化在 ensure 中清除旧 timer。回调和响应均核对 generation/source/target/phase/page，旧代际或已取消请求不会回填当前行。请求中的 scope 变化同时 abort 现有 controller。
- 没有新增 UI 文案、加载提示或 tooltip。实际 patchRuneStarterEquipment 继续调用原 R173/R174 装备渲染，召唤师技能仍在最右。

## P2：分路证据与保守推断

- 新增授权只读 GET /api/gameplay/champion-lanes?champions=...&tier=...。最多 5 个不同正整数 ID（<=10000）；缺失、空项、非法、重复、超量与非法 tier 返回 400，不访问 provider。默认 tier=all；返回 {"103":[{"position":"mid","rate":0.7},...]}，rate 为 0..1。
- 数据复用现有英雄角色占比和缓存。QQ101 支持的精确 tier 使用 loadQQ101Positions，将百分比 RoleRate / 100。QQ101 未提供 iron/bronze/silver/gold/platinum/emerald/diamond 单独 tier；这些值使用现有 OP.GG detail summary.positions、version 与 opggDetailCacheKey，保持请求的原 tier，不映射到 *_plus，也不冒充 all。只读取该 detail 的 summary，不启动其他完整详情任务；没有新增上游域名或请求类型。
- OP.GG role_rate 缺失时按现有 displayRoleRate 同源的 play / average_stats.play 回退。provider 失败或空数据返回该英雄空数组，前端不报错、不显示卡片。
- 前端分路缓存按 championId:tier，pending 先占位、同英雄不重复请求，敌方换英雄只请求新 ID；只请求 locked 且原 position 为空的敌方。已知本人同路敌方优先，不发分路请求，也不覆盖任何 player.position。
- 未知敌方任一占比尚缺失时不把它当零。占比 >=0.25 的唯一英雄可判定；多个候选必须最高 >=0.5 且 >= 第二名两倍，否则记录 lane-ambiguous 并不显示卡片。
- 仅替换 laneMatchupContext 的同路敌方选择。后续 counters、候选缓存 key、去重和本人悬停即停止推荐候选规则保持原实现；R173–R176 视觉 markup/CSS 未回退。
- lane_matchup_candidate_fetch 新增 self_position、enemy_locked_count、enemy_position_known_count、ally_position_known_count；客户端字段/枚举经过后端白名单裁剪。context-unavailable 去重包含形状计数，变化重新记录，每个 generation 最多 20 条。没有玩家身份。

## 自动验证

- Go R177/R173 定向：通过，1.341 秒，日志 /private/tmp/r177-go-target.log。
- Node R177 定向：11/11 通过，含真实 DOM 开局装备回填与最右技能；日志 /private/tmp/r177-node-target.log。
- Go 用 limitNow 与 httptest 本地上游，预置 app 100/120s used=40：后台拒绝、前台出门装成功；used=100 在 6 秒上限内返回 retry=60、不写失败冷却。404/500 保持冷却，上游长 Retry-After 429 仍识别限流；短额度等待记录 queue_wait_ms；混合成功/限流/其他失败计数与隐私一致。
- champion-lanes 测试覆盖缓存命中、QQ101 / OP.GG 原 tier 和 rate 尺度、最多 5 ID、非法请求 400，无非既有域名/请求。
- 前端覆盖 8 秒重试、最多 2 次、pending 不并发、部分补齐无新预算、generation/source/target/page/phase 失效、无提示或网络失败不重试、最终 DOM 保留 R174 技能；分路唯一/歧义/临界两倍/已知优先/同 ID 缓存/换英雄增量/缺失或失败降级；不可用诊断形状变化与 20 条上限。
- node --test backend/web/*.test.cjs：764/764 通过，12.290 秒，日志 /private/tmp/r177-web-full-final.log。
- go test ./backend -count=1：通过，195.699 秒，日志 /private/tmp/r177-go-full.log。
- go vet ./backend：通过，日志 /private/tmp/r177-vet.log。
- git diff --check：代码修改后与最终收尾均通过。

## 变异验证

全部通过 /private/tmp/r177-mutants 的 Go overlay 或 Node 读取源码 overlay 注入，不覆盖仓库生产源码。每项必须发生运行时断言失败，不能把编译或 ReferenceError/SyntaxError 算成功。

| 变异 | 实际测试 | 结果 |
|---|---|---|
| 恢复两处 withRiotBackground | TestR177StarterForegroundReserve | 运行时断言 FAIL：前台 rows 为空，calls=0 |
| 限流也写 starterFailures | TestR177StarterFullQuotaHasRetryWithoutFailureCooldown | 运行时断言 FAIL：starterFailures 出现两个 match key |
| 删除定时重试 | R177 starter rate hint | AssertionError：未安排 timer |
| 去掉代际检查 | R177 starter waiting retry | AssertionError：新 generation 仍请求 |
| 删除两倍优势判断 | R177 competing lane | AssertionError：0.6/0.4 显示了卡片 |
| 去掉已知位置优先 | R177 known enemy position | AssertionError：改选 64 而非明确 mid 的 103 |

## 构建

两平台 public 后端构建通过：

- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.42-public。
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.42-public.exe。
- 两份产物均通过 desktop/verify-build-fingerprint.cjs 核验 8ad20599d929；最终源码再次生成相同指纹。
- macOS 自检使用 /private/tmp/r177-selftest 隔离目录、空 RIOT_API_KEY，确认版本 0.12.42、奖池 554 条、哈希 dee5e21f5234；日志 /private/tmp/r177-selftest.log。

key mode=public；CGO_ENABLED=0，main.version=0.12.42、main.buildFingerprint=8ad20599d929、main.riotAPIKey= 和 main.riotAPIKeyCipher= 在两个构建里均显式设置。产物名称保留 -public，没有发布安装包。

## P3：真机项逐条未验证

1. **R175 P1 漏人补位分支：未验证。** 工单给出的这轮日志没有 live_roster_recovery，三局 10 人且未复发不等于触发修复分支。仍需出现该事件的 Windows 真机对局。
2. **R176、R174、R173 视觉效果：未验证。** 日志不记录布局；DOM 自动回归不能代替用户目测。
3. **R168 P1/P4 前端项及自动补位样本：未验证。** 这轮日志没有对应事件。

本轮 R177 同样未进行 Windows 真机验收。用户需复核：选完绝活哥符文立刻打开记录，出门装及时补齐，rune_starter_batch 无限流或后续成功；选人敌方中路锁定且分路占比可唯一确定时显示原对线卡片，有歧义时不显示；lane_matchup_candidate_fetch 导出带队伍计数字段。顺带观察两侧人数与刷新错误。

首次全量前端运行因 champions/r91/r94 的旧聚焦 harness 未载入新增 clearRuneStarterRetries，出现 ReferenceError；已把真实 helper 纳入各 harness，保留既有断言后重跑。该失败没有计入变异验证。
