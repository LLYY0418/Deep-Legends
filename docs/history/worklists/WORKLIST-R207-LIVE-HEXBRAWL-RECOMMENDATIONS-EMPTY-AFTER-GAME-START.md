# WORKLIST-R207：进入游戏后，对局页「海克斯与出装」变成"暂无该英雄的海克斯样本 / 等待推荐出装与技能数据"

诊断人：Claude（看截图 + 读日志 + 只读核对源码）。执行人：GPT。日期：2026-10-04。基线：0.12.68（`1ba24a67`）。可与 R206 合并发布。

## 现象

用户截图：海克斯大乱斗（queue 2400，嚎哭深渊），「游戏进行中」，英雄海兽祭司（420）。「海克斯与出装」页签：

- 英雄头部只有名字，位置"其他"，没有胜率等数据；
- 海克斯区："暂无该英雄的海克斯样本 / 当前数据源还没有足够的英雄关联样本"；
- 出装区："等待推荐出装与技能数据"。

用户说**每次**选人结束、进入游戏都会这样；选人阶段是正常的。

## 证据（日志 `lol-loot-diagnostics-1004-1532.jsonl`，0.12.68）

两局都是同一个过程。以第二局（`game_id=9014523366`，英雄 420）为例：

| 时间 (UTC) | 事件 |
|---|---|
| 07:30:37～52 | 选人阶段，推荐缓存键 `420:other:KIWI:12:emerald_plus:4-32`（末段是召唤师技能 4=闪现、32=雪球）。后端 4 次 `live_recommendations_response status=success`，带完整 `build_options`；前端 `received` + `rendered` |
| 07:31:11.9 | ChampSelect → GameStart → InProgress |
| 07:31:12～13 | 对局数据换成 gameflow 的 InProgress 名单；live client 接口还是 404（20 秒后才可用） |
| 07:31:14.79 | 新的推荐请求，键变成 **`420:other:KIWI:12:emerald_plus:none`**：InProgress 名单里没有自己的召唤师技能 ID |
| 07:31:14.94 | 后端 `success`，`live_build_shape core_in=5 core_out=5`，海克斯数据也读到了（`hexdata_hero_json`、`mayhem_caution rows=126`） |
| 07:31:14.95～.98 | 前端 `live_recommendations_client reason=received`，随后 `reason=rendered` |
| 07:31:15 之后约 15 分钟 | **没有任何推荐请求，也没有任何 `live_recommendations_skip`**；`live_render_rebuild` 显示对局页还在重绘（1 分钟内 build 区 17 次） |

第一局（英雄 157，07:17～07:30）一样：选人键 `…:4-32`，进游戏后变成 `…:none`，请求成功、前端 `received`、`rendered`。

也就是说，**后端每次都返回了完整数据，前端也收到了**，但进游戏之后的渲染拿不到这份数据，显示成空状态。

## 源码核对

- `backend/web/gameplay.js` `renderRecommendationArea`（约 6153 行）：`payload = liveRecommendationsFor(data) || {}`。海克斯区 `renderLiveAugmentRecommendations` 取 `payload.augments`，出装区取 `payload.build`。两者都为空时，正好是截图里的两段文字。头部只有名字，也说明 `payload.hero` 为空。所以**渲染时 `liveRecommendationsFor(state.live)` 返回的是空**。
- `liveRecommendationsFor`（约 5068 行）：
  1. `data.recommendations` 存在就直接用它；
  2. 否则按 `liveRecommendationTarget(data).key` 去 `state.liveRecommendations` 里精确查找。
- 缓存键（约 5045 行）：`${championId}:${position}:${gameMode}:${mapId}:${tier}:${spellKey}`，`spellKey` 来自当前快照里自己的 `spell1Id/spell2Id`，取不到时是 `none`。进入游戏后，自己的技能 ID 来源会变：
  - 选人阶段：有技能 ID（`4-32`）；
  - InProgress 刚开始：gameflow 名单里没有技能 ID（`none`）；
  - live client 可用后：技能可能按名称映射回 ID，也可能映射不到。

  只要渲染时的键和缓存里的键差一点，就查不到，页面就是空的。
- 能清空 `state.liveRecommendations` 的 `resetLiveGameScopedState` 有 5 个调用点：
  - `shouldResetLiveGameScopedState`：gameId 变化，或进入选人；
  - 断开连接；
  - 重新等待新对局；
  - 实时连接中断；
  - 手动硬刷新（`softResetGameplayState`）。

  清空后，只有拿到完整 `/api/live` 快照时（约 4728 行）才会再次调用 `ensureLiveRecommendations`。对局中的轮询大多返回"未变化"的小响应，不会触发重新请求。
- 07:31:14 之后完全没有 skip 记录，说明之后 `ensureLiveRecommendations` 再也没有用一个有效的键执行过。这与"缓存被清空或键变了，之后又没人重新请求"一致。

日志里缺少渲染那一刻的信息（渲染用的是哪个键、缓存里有哪些键、缓存是否刚被清过），所以无法断定是"键不一致"还是"缓存被清空后没有重新请求"。下面的修改两种情况都覆盖，并加诊断确认。

## 修改

### P1　找不到精确缓存时不显示空状态

1. `liveRecommendationsFor`：精确键找不到时，退一步按**同一局、同一英雄**查找（`gameId` + `championId` + `gameMode` + `mapId` + `tier`，不比较 `spellKey` 和 `position`），用最近一次成功的数据显示，同时在后台发起精确键的请求。
2. **海克斯大乱斗、大乱斗、斗魂**：召唤师技能不影响海克斯数据。`spellKey` 为 `none` 时，不能因此认为"换了一份推荐"：
   - 优先沿用同局同英雄已有的数据；
   - 技能 ID 只在**选人阶段**用来细化出装请求；
   - 进入游戏后，键里的技能段**沿用选人阶段最后一次有效的值**，不再变成 `none`。
3. 普通召唤师峡谷模式的键规则不变（R174 / R171 依赖技能段的地方保持原样）。

### P2　缓存被清空后一定重新请求

1. `renderRecommendationArea`：渲染时如果有目标英雄，但缓存里没有可用数据，并且没有请求在进行、也不在失败退避期，就调用一次 `ensureLiveRecommendations(state.live)`。这一路已有的 single-flight 和退避保证不会刷屏。
2. 检查 `resetLiveGameScopedState` 的 5 个调用点在 ChampSelect → GameStart → InProgress 这段时间是否被触发，特别是：
   - `shouldResetLiveGameScopedState` 在快照 `gameId` 短暂为 0 或缺失时，**不能**当成换了一局（只有前后都是非 0 且不同才算）；
   - 选人到进游戏这段时间如果有任何硬刷新 / 重新同步，要记下来源。
3. 每次 `resetLiveGameScopedState` 记诊断 `live_scope_reset`：调用点（`game_changed` / `enter_champselect` / `disconnect` / `await_game` / `hard_refresh` / `resync`）、前后 gameId、前后阶段、清掉的推荐条数。

### P3　渲染诊断

新增前端诊断 `live_recommendation_render`。只在以下两种情况记录，同一局同一键最多 5 条：

- 阶段变化后的第一次渲染；
- 推荐区显示成空状态。

字段：

- `phase`、`game_id`、`champion_id`；
- `target_key`；
- `exact_hit`（布尔）、`fallback_hit`（布尔）；
- `cached_keys`（同一英雄的缓存键，最多 5 个）；
- `has_payload_field`（`data.recommendations` 是否存在）；
- `augment_rows`、`has_build`；
- `last_reset_reason`、`ms_since_reset`。

### P4　顺带：开局时出现 `endgame_trigger source=honor-ballot`

同一份日志 07:31:11.909，**选人结束、刚进入 GameStart 时**记了一条 `endgame_trigger honor_enabled=true source=honor-ballot`。荣誉投票只应在对局结束后出现。执行时确认：

- 这条触发是不是上一局遗留的事件；
- 它会不会引起对局页刷新或缓存清空（与 P2-2 一起查）。

如果是误触发，限定为只在 `EndOfGame` / `PreEndOfGame` 阶段处理。

## 测试（Node，jsdom，按日志顺序回放）

1. **复现**：依次喂入以下快照，最后对局页「海克斯与出装」显示海克斯和出装，不出现"暂无该英雄的海克斯样本"或"等待推荐出装与技能数据"：
   - 选人快照（420，技能 4/32），推荐请求成功；
   - GameStart 快照；
   - InProgress 快照（gameflow 名单，没有技能 ID），推荐请求成功；
   - 再一个 InProgress 快照（live client 名单，技能按名称）。
2. **键漂移**：缓存里只有 `…:4-32`，当前快照是 `…:none` → 先显示 `…:4-32` 的数据，并只发一次 `…:none` 请求。
3. **缓存被清空**：InProgress 中 `state.liveRecommendations` 被清空，之后只收到"未变化"轮询 → 下一次渲染发起一次请求，成功后显示数据；1 秒内多次渲染只发一次。
4. **gameId 抖动**：InProgress 快照 gameId 一次为 0、下一次恢复 → 不清空缓存，`live_scope_reset` 不记录 `game_changed`。
5. 召唤师峡谷排位：技能变化后仍按原规则用新的键请求，出装推荐与以前一致（R171 / R174 现有测试保持通过）。
6. `live_recommendation_render` 在空状态时有记录，字段完整，同一键不超过 5 条。

变异（必须 FAIL）：

| 变异 | 失败的测试 |
|---|---|
| 去掉 P1 同局同英雄的退一步查找 | 测试 2 |
| 去掉 P2-1 渲染时补发请求 | 测试 3 |
| gameId 为 0 时仍算换局 | 测试 4 |

## 收尾

- `node --test backend/web/*.test.cjs desktop/*.test.cjs`、`go test ./backend -count=1`、`go vet ./backend`、`git diff --check` 全绿。
- 版本递增，可与 R206 合并发布；`docs/WORKLIST-INDEX.md` 加 R207；账本 `docs/history/ledgers/r207-execution-ledger.md`，写明 P2-2 / P4 查到的实际原因。
- 按 R199 规则发布到 GitHub Latest。

## 真机验收（用户）

1. 打一局海克斯大乱斗：选人阶段「海克斯与出装」有数据；进入游戏后，同一页签仍然显示海克斯推荐和出装，不再变空。
2. 导出日志：
   - 进入游戏后有 `live_recommendation_render`，`exact_hit` 或 `fallback_hit` 为 `true`；
   - 若出现 `live_scope_reset`，看它的 `reason` 是否合理。
