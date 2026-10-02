# R158 执行账本

日期：2026-09-25。执行时产品基线为 0.12.22（工单诊断时写的 0.12.21 已被 R157 递增）；本单递增至 0.12.23。工作区先前的 R144–R157 未提交改动保留。未打安装包。

## 逐项结果

| 项 | 实施 | 本机证据 |
|---|---|---|
| P1 | 总览内容容器为每个玩家标签保存其 DOM 和已有渲染签名；切换时移出旧标签节点，返回时先恢复，再运行原有按签名保留/按数据变化重绘的逻辑。只保留最近 4 个非活动标签；已关闭的标签在下一次总览渲染时清理。`shouldReloadOverview` 的 2 分钟网络刷新规则未改。覆盖层沿用同一机制。 | 前端测试验证同玩家重渲染与 A→B→A 后头像、背景图、生涯统计和战绩图标均是原节点；接入真实 `image-queue.js` 后，A 的图加载完成再切 B→A，返回时没有新 `src` 赋值或队列准入。A 数据变化后头像、背景、生涯和战绩均换成新内容；第 5 个非活动标签挤出最旧缓存后正常重绘；关闭非活动标签后缓存被清理。现有缓存总览回访测试确认不重新请求战绩。 |
| P2 | `recent_players` 改为两个独立的本地聚合计时区间，分别覆盖基础聚合和近期排位结果到达后的本地聚合；不再把并行上游等待计进去。完成装配时推进顺序计时指针，避免等待时间又被误记为 `serialize`。响应数据及加载顺序不变。 | 并行上游人为延迟 150ms 的回归测试中，`recent_players` 有两个实测 span，`recent_players` 和 `serialize` 均小于 100ms，总耗时至少 150ms；原有总览返回与阶段重叠测试通过。 |

## 诊断依据与边界

- 工单引用的 `e926fe78-lol-loot-diagnostics-0925-1746.jsonl` 未在工作区、Downloads 或本机个人工作目录找到，未生成或归档伪造原始日志；工单所列 3,627 行及 `recent_players:1877` 等数字仅作为工单摘要引用，本轮无法独立重算。
- `overview_phases_ms` 的并行阶段采用独立 span，阶段数字本来可能重叠，不能要求简单求和严格等于总耗时。修正后 `recent_players` 仅表示本地聚合工作；真实环境的新分布仍需新日志确认。
- DOM/图片队列行为已用 JSDOM 和实际队列实现验证。Windows 客户端的视觉连续性与真实图片网络请求，需要真机复核。

## 验证记录

- `node --test backend/web/r158.test.cjs`：5/5 通过；同玩家、A→B→A、真实图片队列、数据变化、缓存容量及关闭清理均覆盖。
- `node --test backend/web/r158.test.cjs backend/web/r110.test.cjs backend/web/gameplay.test.cjs`：最终改动后 43/43 通过。
- `go test ./backend -run 'TestGameplayOverviewRecordsCompletePhaseTiming|TestGameplayOverviewOverlapsIndependentUpstreams' -count=1`：通过。
- `go test ./... -count=1`：完整通过，backend 204.299 秒；此后仅补了前端关闭标签缓存清理，已由最终 Node 全量测试和重建自检覆盖。
- `go test -race ./backend -run 'TestGameplayOverviewRecordsCompletePhaseTiming|TestGameplayOverviewOverlapsIndependentUpstreams' -count=1`：通过。
- `node --test backend/web/*.test.cjs desktop/*.test.cjs`：最终改动后完整复跑 976 项，975 通过、1 跳过、0 失败。
- `go vet ./...` 与 `git diff --check`：通过。
- `go build -o /private/tmp/deep-legends-r158 -ldflags '-X main.version=0.12.23' ./backend`：通过；`-self-test` 输出“Deep Legends 0.12.23 自检通过：奖池 554 条”。未打安装包。

## 待真机复核

1. 打开韩服玩家 A、B，总览图片加载完成后在 2 分钟内 A→B→A；确认 A 的头像、背景、生涯图标、战绩图标无需重新等待，且没有新的图片请求。再使 A 的资料或战绩确实变化，确认回到 A 时显示新内容。
2. 用 0.12.23 重新导出诊断，核对 `overview_phases_ms.recent_players` 不再跟随 `total` 一起接近上游等待时间，并依据修正后的各阶段定位实际慢点；本单不根据旧字段推断性能瓶颈。
