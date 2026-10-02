# WORKLIST-R173：出门装应加在符文卡片「最终装备」行（R172 改错了位置）；职业选手来源的出门装与召唤师技能先补原始字段诊断；0930 日志其余发现

诊断人：Claude（只读：用户两张截图 + 诊断日志 `lol-loot-diagnostics-0930-1912.jsonl`（0.12.37，构建 `1ff21959b693`，run `b6f4d1a47f63b6e0bd1e510b`）+ 读仓库代码与 R171/R172 账本，未改仓库代码）。
执行人：GPT。
日期：2026-09-30。
基线：源码 0.12.37（R170–R172 之后）。

用户本轮反馈：
- R166 雷达图：差距已经看得出来了。
- R167 / R168 / R170：还没打实际对局确认。
- R169：检查更新已提示最新版本。
- R171：召唤师技能有了，应用后客户端也改了（截图一，绝活哥来源）。
- R172：截图里没有出门装，也没有竖线分隔。

---

## P1　出门装要加在符文推荐卡片的「最终装备」行（绝活哥 / 职业选手来源），不是出装卡片的核心装路线

### 为什么 R172 做完了用户却看不到

R172 工单是 Claude 写的，把用户说的「完整装备列表」理解成了出装推荐卡片里「核心装」那条带箭头的路线（`renderBuildRecommendation` → `renderConfigOption(kind="route")`）。GPT 按工单实现了，代码是对的（`backend/web/gameplay.js:6483` 起的 `leadingIds`，`gameplay.css:1512` 的 `.route-divider`），但**位置错了**：用户指的是截图里**符文推荐卡片每条对局记录底部的「最终装备」行**——这一行才是一场对局从头到尾的完整装备。所以这是工单理解错误，不是执行错误。

`renderRuneSourceSection`（`backend/web/gameplay.js` 约 6179–6182 行）：

```js
const itemIDs = (config.itemIds || config.itemIDs || []).map(Number).filter(...).slice(0, 7);
const items = itemIDs.length
  ? `<div class="specialist-game-items"><span>最终装备</span><div>${itemIDs.map((id) => renderItemIcon(id)).join("")}</div></div>`
  : "";
```

这一行只有 `config.itemIds`（终局 7 格），两个来源都没有出门装数据：

- **绝活哥**：`backend/specialist_runes.go:687`，`itemSlots(participant.Item0 … Item6)`，来自 Riot match-v5 对局详情的终局装备格，不含购买过程。
- **职业选手**：`backend/pro_runes_recommendations.go` 的 `supplement()` 只请求 `details/{gameId}?startingTime=<最后一帧>`，并且 `result.Frames = result.Frames[len(result.Frames)-1:]` 只保留最后一帧——拿到的也是终局装备。

R172 已落地的出装卡片前缀（每条核心装路线前面接出门装 + 竖线）**本单不动**，是否保留请用户看过真机后再说；本单只做符文卡片这一行。

### P1-1　绝活哥来源：用对局时间线取出门装

仓库里已经有现成的零件，不需要新写解析：

- `riotProvider.matchTimeline(ctx, matchID)`（`backend/riot_api.go:939`）读 `/lol/match/v5/matches/{matchId}/timeline`，已有 `riotTimelineResponseMax = 16 MiB` 的响应上限；
- `extractParticipantTimeline(frames, participantID)`（`backend/match_timeline.go:105`）已经处理 `ITEM_PURCHASED` / `ITEM_SOLD` / `ITEM_UNDO` 的抵消。它按分钟分组，出门装判定需要原始时间戳，建议把内部的 `records`（`timelineItemRecord{at, itemID, sold}`）抽成一个可复用的小函数，出门装和现有的「按分钟分组」都从它派生，不要再写一遍事件解析。

**出门装的定义**（写进代码注释，不进界面）：开局 90 秒内（`at <= 90_000`）该玩家的净购买（已被 `ITEM_UNDO` 抵消的不算；90 秒内又卖掉的不算），**排除饰品**（3340 / 3363 / 3364 这类饰品格物品），同一物品买多个就重复显示（与出装卡片 `starterOptions` 里 `[1055, 2003]`、`[1086, 2003, 2003]` 的表示一致）。如果 90 秒内没有任何净购买（比如时间线缺事件），出门装为空，行为和现在一样。

**请求预算与时机**（这是本单最需要小心的地方）：

- 时间线每场 1–3 MB。本日志 `specialist_runes_done.budget_used = 36`，已经用满 `specialistRuneRequestBudget = 3 × (2 + 10) = 36`，**不能把时间线请求塞进这 36 次里**，也不能让符文列表等时间线：本日志里绝活哥符文列表本身已要 5.9–6.8 秒（`specialist_runes_done.duration_ms`）。
- 做法：符文列表照原样先返回；出门装作为第二段异步补上。只请求**当前实际要展示的对局**（绝活哥每位玩家最多 `specialistRunePerPlayerMax = 3` 场、最多 3 位玩家，即最多 9 场），给它单独一个小预算（上限 9 次），走 Riot 共享限流器的后台优先级，不得挤占前台请求。
- 按 `matchId` 缓存到磁盘（已结束对局的时间线不会变，可长期缓存，大小受现有公共缓存上限约束）；同一场对局、同一个英雄/位置重复进入选人不重复请求。
- 前端：出门装到达前这一行保持现状（只有最终装备），到达后原位补上，不闪、不重排对局顺序。
- 失败（429 / 超时 / 时间线缺失）只影响这一行的出门装部分，不报错、不出占位图标。

### P1-2　职业选手来源：从开局那一段帧里取出门装（先做 P2 的字段确认）

LoL Esports livestats 的 `details/{gameId}` 返回的是给定 `startingTime` 之后约 10 秒的逐帧数据，每帧带每个参赛者当时的 `items`。出门装 = 开局后、第一次回城前某一帧的装备：

- 用 `window/{gameId}` 已拿到的首帧时间戳（`proRuneIndexGame.Start`，即 `w.Frames[0].Timestamp`）加一个偏移量（建议从 60 秒起），请求一次 `details/{gameId}?startingTime=…`，在返回的帧里取**第一帧里该参赛者有非饰品物品**的那一帧，去掉饰品后作为出门装。
- 这个偏移量对不对（首帧时间戳是不是游戏内 0:00、60 秒时出门装是否已买完），GPT 的环境访问不到 livestats（R171 账本已记录 DNS 不通），**不能在仓库里凭空定死**。所以先按 P2 加诊断，用户下一份日志里就能看到这一帧的物品数量与偏移量是否合理，再定最终数值；在拿到日志之前可以先用 60 秒上线，但诊断必须同时上。
- 与现有终局 `details` 一样写磁盘缓存（键区分，比如 `details-start-<gameId>`），不影响现有 `details-<gameId>` 终局缓存的校验逻辑（`valid()` 检查的是「最后一帧不早于终局」，开局那份不能走这个校验，要单独写校验：帧非空、参赛者 ID 都在本局元数据里、不重复）。
- 预算：职业选手每位选手最多展示 5 场（`gameplay.js` 里 `.slice(0, 5)`），同样只请求当前展示的场次，沿用 `proRuneProvider` 现有的并发门（`gate` 4 个名额，注释说要留一个给选中符文详情），不得占满。

### P1-3　前端渲染

在 `renderRuneSourceSection` 那一行里：

- 有出门装时：`<span>装备</span>` + 出门装图标 + `route-divider`（直接复用 R172 已有的 `.route-divider` 样式，3px 主题色竖条，和 R172 保持一致）+ 终局装备图标。行标签由「最终装备」改为「装备」，因为这一行不再只是终局装备；不加任何说明文字或 tooltip。
- 没有出门装时：保持现在的「最终装备」+ 终局图标，输出与改动前逐字一致。
- 出门装图标数量不计入现在的 `.slice(0, 7)`（那是终局 7 格的上限），出门装单独最多 6 个。
- OPGG 来源的符文卡片没有「最终装备」行，不受影响。

### 测试

- Go：
  - 出门装提取：构造时间线，包含 0:05 买多兰剑+药、0:20 撤销再买、0:40 买饰品、1:10 卖掉一瓶药、3:00 回城买装备——断言结果只有 90 秒内的净购买且不含饰品，3:00 的购买不进来。变异：阈值改成全局所有购买、去掉 `ITEM_UNDO` 抵消、不排除饰品——各自 FAIL。
  - 预算隔离：时间线请求不消耗 `specialistRuneRequestBudget`；符文列表在时间线请求挂起时照常返回（用阻塞的假 Riot 服务断言返回时间不受时间线影响）。变异：把时间线请求挪进同步路径 → FAIL。
  - 职业开局帧：夹具里前两帧参赛者装备为空、第三帧有 `[1055, 2003, 3340]`，断言出门装为 `[1055, 2003]`；终局缓存校验不被开局请求污染。
- Node：有出门装时 DOM 顺序为 标签 → 出门装图标 → `route-divider` → 终局图标（按顺序断言，不只断言「包含」）；无出门装时输出与改动前一致；出门装稍后到达时对局行顺序与选中状态不变。
- 全量 `go test ./backend`、`go vet ./backend`、`node --test backend/web/*.test.cjs` 全绿。

### 真机验收（用户）

进一局选人（练习模式即可），打开符文推荐里的「绝活哥」「职业选手」：每条对局记录底部那一行，最前面是出门装，接一条明显的竖线，后面是最终装备。

---

## P2　职业选手来源：给 livestats `details` 原始响应加字段形状诊断（回答「有没有召唤师技能」「开局帧对不对」两个问题）

用户截图二（职业选手来源）确认：符文行没有召唤师技能，这是 R171 执行时的既定处理（`docs/history/ledgers/r171-execution-ledger.md`：GPT 环境访问 livestats DNS 不通，无法确认原始响应里有没有技能字段，所以留空，不借别的来源）。这个结论到现在还是「待确认」，而用户的机器是能访问 livestats 的（截图里职业选手数据正常出来了），所以办法是让应用自己在用户机器上把字段形状记下来：

1. 在 `supplement()` 拿到 `details` 响应、以及 P1-2 新增的开局请求拿到响应时，**先对原始 JSON** 记一条 `pro_details_shape` 诊断（每个 `gameId` 每类请求只记一次，进程内去重）：
   - `kind`（`end` / `start`）、`frames_length`、第一帧与最后一帧的 `rfc460Timestamp` 相对 `window` 首帧的秒数偏移；
   - `participants[]` 元素的**键名集合**（只记键名，不记取值，沿用 `arena_truth_diagnostics.go` 的白名单/`<unknown-key>` 收拢做法；白名单至少收录 `participantId`、`items`、`perkMetadata`，以及可能出现的 `summonerSpells` / `spells` / `summoner1Id` 这类候选名，便于直接看出有没有技能字段）；
   - 开局请求额外记：选中那一帧的偏移秒数、该参赛者非饰品物品个数（只记数量）。
2. 不记录选手名、队伍名以外的任何标识；不记物品 ID 以外的取值（物品只记个数）。
3. 下一份日志回来后由 Claude 判读：
   - 键名里出现技能字段 → 另开工单补 R171 的职业选手技能解析；
   - 没有 → R171 职业选手技能保持留空，在 R171 账本里把「待确认」改成结论。
   - 开局帧偏移与物品个数若不合理（比如 60 秒那帧物品还是 0 个，或已经出现大件），据此调整 P1-2 的偏移量。

测试：用夹具断言 `pro_details_shape` 只含键名、不含物品 ID 与任何取值；同一 `gameId` 重复读取只记一次。变异：把取值写进诊断 → FAIL。

---

## P3　启动时后台刷新职业选手资料，每次约 49 MB OP.GG 页面

### 证据

本日志 11:02:47–11:03:04（应用刚启动、用户还在总览页）出现 45 次 `champion_upstream{host: op.gg}`，每次 1.07–1.14 MB、1.5–3.3 秒，合计约 49 MB；同时间段 `pro_profile_batch{accounts_checked: 53, duration_ms: 15045}`、`pro_directory_cost{stage: supplements, duration_ms: 17538}`。

来源：`backend/main.go:630` 启动即 `go a.loadProPlayers(runtimeContext, true)`（`force=true`），走 `enrichProProfiles` 以 6 并发逐个读 53 个职业账号的 OP.GG 页面；`proProfileTTL` 对有段位账号只有 15 分钟（`backend/pro_profiles.go:25-33`），所以**距上次启动超过 15 分钟，每次打开软件都会重新下这约 50 MB**，不管用户这次会不会打开职业选手页。

### 要做

- 启动时不再对职业资料做强制刷新：启动预热只读磁盘缓存（`pro-profiles` 公共缓存本来就有 7 天 `StaleUntil`），让职业页首屏有数据可显示；真正的 OP.GG 刷新推迟到用户打开职业选手页（`/api/pro-players` 请求）时再按现有 TTL 进行。
- 职业页打开时的新鲜度规则（15 分钟 TTL、`refresh=1` 强制刷新）保持不变，这是用户已经认可的行为，本单不改。
- 如果 GPT 核实后发现有别的功能依赖「启动即刷新」（比如对局页的职业选手身份识别 `pro_identity_match` 需要最新目录），在账本里写清楚依赖点，改为只刷新那个依赖需要的最小部分（例如只刷新目录，不刷新 53 个账号的资料页），不要整体保留。
- 验收：冷启动后停留在总览 2 分钟，新日志里 OP.GG `champion_upstream` 不再出现这一串 1 MB 级请求；打开职业选手页后，资料按原规则刷新，页面数据与改动前一致。
- 测试：启动流程不触发 `enrichProProfiles` 的网络读取（假 provider 计数为 0）；打开职业页后才触发。变异：恢复 `force=true` 启动刷新 → FAIL。

---

## 本日志其余检查结论（不需要改代码，记账本即可）

- **R171**：两次 `perk_apply_attempt` 均 `outcome=success`、`spell_requested=true`、`spell_applied=true`，对应两次 `PATCH /lol-champ-select/v1/session/my-selection` 返回 204——与用户截图一致，R171 的 OPGG/绝活哥部分真机通过。
- **R169**：启动时直连清单 `update_check_failed{stage: fetch, error_kind: timeout, mirror_prefix: direct}`，随后两次 `update_check_succeeded`（`latest_version=0.12.19`，当前 0.12.37）。说明清单校验在真实发布清单上已经通过、直连超时后镜像回退正常。另记一笔：公开发布渠道目前最新还是 0.12.19，用户本机 0.12.37 比它新，所以显示「已是最新」；这不是缺陷，但如果以后要让别的用户收到更新，需要发布新的 public 版本。
- **R168 P2 诊断埋点**：`lane_matchup_candidate_fetch` 已经在打点，本日志 5 条全部是 `reason=context-unavailable`——这两次选人都是队列 3110 的练习模式（`champselect_evaluation.group_id=practice`，己方 1 人、对面 1 个无分路的电脑），本来就没有同位置对手，卡片不出现是对的。R167/R168/R170 仍需一局单双排或灵活组排验证。
- **R166 P3**：本日志没有打开斗魂/海斗英雄详情（没有 `mayhem_detail_phases_ms`、`arena_header_source`），首次加载速度这一项还没被真机覆盖；下次请打开一个斗魂英雄和一个海斗英雄详情后再导出日志。
- **生涯/总览背景图**：`/lol-game-data/assets/v1/champion-splashes/{id}/{id}000.jpg` 这条本地路径 6 次全部返回 400（1 ms），随后都由 `game.gtimg.cn` 正常取到，界面无影响，属于每张图多一次无效尝试；低优先级，本单不做，若以后整理图片来源可顺带核对本地客户端 `skins.json` 里的 `splashPath` 实际格式。
- `diagnostic_dedup` 里 `pro_identity_match` 在打开几个玩家总览时累计上千次（已被去重，没刷屏），是逐行身份匹配的正常调用频率，暂不处理。
- `sgp_ranked_stats_incomplete`（72 次）、`sgp_page_size_downgraded`（请求 30 返回 20）：已知正常现象，与 R169 结论一致。

---

## 验收总表

| 项 | 标准 |
|---|---|
| P1 | 绝活哥、职业选手两个来源的符文对局记录底部：出门装 → 竖线 → 终局装备；无出门装时与现在一致；时间线请求不占符文预算、不拖慢符文列表；各项变异 FAIL |
| P2 | `pro_details_shape` 只记键名、帧偏移与物品个数，同一场只记一次；下一份用户日志能回答「有无技能字段」「开局帧偏移是否合理」 |
| P3 | 冷启动停在总览不再下载职业资料页；打开职业页后按原规则刷新 |
| 全量 | `go build`、`go vet ./backend`、`go test ./backend`、`node --test backend/web/*.test.cjs` 全绿；R172 已有的出装卡片前缀保持不变 |

## 收尾

- `docs/WORKLIST-INDEX.md` 加 R173 一行；R172 那一行的状态补一句「符文卡片「最终装备」行的出门装见 R173」。账本 `docs/history/ledgers/r173-execution-ledger.md`。
- 界面文案红线：不加统计口径、说明性文字或 tooltip。
