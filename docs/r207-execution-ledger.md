# R207 执行账本

日期：2026-10-04。工单：[R207](WORKLIST-R207-LIVE-HEXBRAWL-RECOMMENDATIONS-EMPTY-AFTER-GAME-START.md)。实际执行基线 `c0e3b6a7`、版本 0.12.70（已包含 R206）；本轮递增为 **0.12.71**。实现及全部要求的自动验证完成；真机验收与构建/发布未完成。

## P1 推荐键与同局回退

- `liveRecommendationScopes` 记录成功响应的 gameId、championId、gameMode、mapId、tier 与接收时间。精确键缺失时取同局同英雄最近一次成功数据，忽略 position/spellKey；渲染保留数据并补发精确键请求。跨局、跨英雄、模式、地图与档位均不借用回退数据。
- KIWI、ARAM、CHERRY 及既有特殊队列保留选人阶段最后完整有效的技能 pair；进入游戏后不使用 gameflow 缺失 ID 或 live client 名称重建技能段。特殊模式仅 ChampSelect 将技能 ID 发送到出装接口。普通 CLASSIC 仍按现有技能变化创建新键、发送技能 ID，R171/R174 行为保留。
- 对局范围重置时清除技能与成功响应元数据。初始未知 gameId 在同一 generation 内补齐时，给现存成功响应补上已知范围；短暂 gameId=0 沿用最近已知范围，真正不同的正 ID 仍重置。此处仅保留推荐；没有撤销 R204 对未知 gameId 玩家战绩的隔离规则。

## P2-2 实际代码原因与重置来源

`shouldResetLiveGameScopedState` 原先直接比较 `String(previous.gameId || "") !== String(next.gameId || "")`。因此有效 ID → 0/缺失会被当作换局，清空推荐及 flights；而 `invalidateLiveForNewGame` 原先已要求前后两个正 ID 不同。两个入口的判定不一致是本次确认的代码缺陷。现在只有前后最近已知正 ID 不同或进入 ChampSelect 才重置；保留已知 ID 记忆以识别 0 → 另一局。

全部五个调用入口记录 `live_scope_reset`：完整快照的换局/进入选人、invalidate 的换局/进入选人/await_game、客户端断开 disconnect、实时断开 resync、手动硬刷新 hard_refresh。记录来源、前后 gameId/phase、清除推荐条数；先捕获旧快照再置空，防止诊断丢失旧 ID。ChampSelect → GameStart → InProgress 本身不触发重置。

工单提供的原日志没有渲染键和 reset 证据，不能唯一认定那两局究竟发生了键漂移还是缓存清空。此账本确认的是源码中可重放的两类缺口，未编造原日志里没有的信息。新诊断用于下次真机区分。

渲染缺少精确缓存时补发推荐，复用现有 single-flight、30 秒 flight 过期与60秒失败退避；成功后重绘。即使后续轮询无完整快照，也可从重绘恢复。

## P3 渲染诊断

`live_recommendation_render` 在阶段变化首次可识别目标的渲染或推荐空状态记录。前端按同局同键最多5条，硬刷新也不重置同键计数；计数表最多128个键。同英雄 cached_keys 最多5个。后端新增事件/原因白名单与完整字段落盘，仍拒绝未知字段；文本128字符、数值与数组有界。

字段完整保留 phase、game_id、champion_id、target_key、exact_hit、fallback_hit、cached_keys、has_payload_field、augment_rows、has_build、last_reset_reason、ms_since_reset。没有增加 UI 方法论或免责声明。

## P4 实际原因与副作用

原 `watchRunner.handleEvent` 对所有 `/lol-honor-v2/v1/ballot` URI 都触发荣誉逻辑，没有 gameflow 阶段检查；因而 GameStart 时的事件也能记录 `endgame_trigger source=honor-ballot`。当前日志摘录不足以证明该事件是上一局遗留，还是客户端事件时序问题。

荣誉路径会执行荣誉 POST、取消并可能恢复 play-again；不直接触发前端推荐清空，也不调用 gameplay/overview/rank 的缓存失效。后者由独立的真实 phase 变化路径触发。

现在只有 EndOfGame / PreEndOfGame 处理 ballot，其余阶段记录 `honor_ballot_ignored`。异步流程入口、读取完成后、ballot POST 前以及 deferred play-again 的恢复前再次检查阶段，防止读数据期间已进入 GameStart 仍执行旧荣誉流程。阻塞 GET 后切换 GameStart 的测试验证无 POST。

## 自动验证

- Node/jsdom 8项定向回放通过：选人→GameStart→无技能 gameflow→按名称 live client，键漂移先显示并仅发一个精确请求，清空缓存后渲染恢复，多次重绘 single-flight/退避，gameId=0抖动，普通排位新技能键，完整诊断与5条限额，跨局隔离、初始未知 ID 补齐及部分技能 ID 不覆盖有效 pair。
- 后端定向诊断/荣誉测试通过，包括正常 EndOfGame 与 PreEndOfGame、四种非结束阶段不触发，以及处理期间阶段切换无写入。旧荣誉测试夹具补真实 EndOfGame 前提。
- 三项实际源码副本变异全部由目标断言杀死（非编译错误）：删除 fallback、删除渲染补发、shouldReset 将 gameId=0视为换局。见 [变异结果](history/reports/r207/mutations.json) 与同目录原始日志。
- 现有孤立渲染测试为新诊断/补发副作用补依赖桩，R207 回放编译真实实现及海克斯/出装 renderer；技能键 const→let 与新增缓存元数据导致的源码形状断言同步调整，原行为断言保留。
- `go test ./backend -count=1`：通过，250.874秒，见 [后端全量日志](history/reports/r207/go-full.log)。`go vet ./backend` 与 `git diff --check` 通过。Node首轮全量发现旧测试提取器将新增的对象默认参数当成函数体，改用null默认值后76项相关回归通过。
- 最终 `node --test backend/web/*.test.cjs desktop/*.test.cjs`（仅增加tap报告格式）：1161项、1160通过、1项平台门禁跳过、0失败，252.575秒；见 [最终Node日志](history/reports/r207/node-final.log)。含R171/R174既有回归。
- `go test -race ./backend -run 'TestR207|TestWatchHonorEvent|TestR68Honor|TestWatchReadFailures' -count=1`：通过，3.258秒，见 [异步race日志](history/reports/r207/go-race.log)。最终 diff check 通过，package与lockfile版本均为0.12.71。

## 构建、发布与用户验收

[R206账本](r206-execution-ledger.md) 当前记录用户2026-10-04“暂停构建和发布”，与本工单收尾要求冲突；已向用户确认是否恢复，当前未执行构建/打包/Release 写操作。版本号已递增，**没有声称 0.12.71 已构建或已发布**。若恢复验证性打包，key mode 使用 public，仅保留 `-public` 可识别文件名，发布需完整 R199 三项证据。

用户真机仍需打一局海克斯大乱斗，验证选人及进入游戏后的同一页签持续显示海克斯/出装，导出诊断检查 exact_hit 或 fallback_hit，以及出现时的 live_scope_reset 原因。Node模拟不替代 Windows 客户端实测，工单保持进行中。
