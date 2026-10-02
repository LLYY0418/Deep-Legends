# WORKLIST-R167：选人阶段「对位克制建议」卡片

诊断人：Claude（只读：用户描述的功能诉求 + 用户上传的真实排位对局诊断日志
`lol-loot-diagnostics-0926-0105.jsonl`（queue 440，含 `champselect_source` 事件）+
读仓库代码，未改仓库代码）。
执行人：GPT。
日期：2026-09-26。
基线：源码 0.12.30。

## 背景：先纠正一个此前的误判

之前讨论这个需求时，Claude 一开始以为「选人阶段看不到对方选了什么英雄」，
这是错的——那个结论来自 R153 对**海克斯大乱斗（KIWI 模式）**选人流程的真机
探测，不能套用到排位/灵活组排的标准交替选人。

用户提供的这份真实排位（queue 440）诊断日志里，`champselect_source` 事件已经
逐条记下了选人过程：

- 13:17:46，某个敌方位置的 pick 动作 `champion_id=875` 出现，`completed=false,
  in_progress=true`；
- 13:17:47，同一动作变成 `completed=true`——此时敌方阵容（`team_champions`，来自
  `session.TheirTeam[].championId`）里 `champion_id=875` 已经写进日志，且当时
  `phase` 仍是 `BAN_PICK`，不是选人结束后的过渡画面；
- 之后每次敌方某个位置锁定，日志里立刻多一条敌方英雄记录，直到 5 个敌方位置
  全部锁定为止。

也就是说：**排位/灵活组排的标准交替选人里，敌方每一次「完成锁定」都会实时
暴露给客户端**，不用等到 `InProgress`。

进一步读代码确认了两件事，都是现成的、已经在跑的能力，不需要新探测：

1. **敌方分路字段已经在用**：`mergeChampSelectPlayers`（`gameplay.go` 约 8242
   行）对 `session.TheirTeam` 调用 `merge(session.TheirTeam, 200, false)`，把
   `AssignedPosition` 原样写进 `SelectedPosition` → 最终 `gameplayLivePlayer.Position`
   字段。这条数据链路今天就在跑：`/api/gameplay/live` 返回的 `players[]`
   里，敌我双方每个人的 `position` 字段前端已经在渲染（`gameplay.js`
   `renderLivePlayer` 里 `positionLabel(player.position)`），选人阶段的对局
   列表本来就显示双方分路。不需要新加诊断去验证「敌方分路是否可信」——
   它已经是产品里正在使用的字段。
2. **克制数据本人这边已经在下发**：排位模式的推荐接口（`handleGameplayRecommendations`
   → `gameplayRecommendationHero`，`gameplay.go` 约 6977 行）在 `spec.HasCounters`
   时，早就把「本人英雄」的 `StrongAgainst`/`WeakAgainst`（各自最多 5 条，
   `ChampionID/ChampionName/WinRate/Games`）塞进了 `Hero` 字段，选人阶段显示的
   推荐面板早就在拉这份数据，只是前端目前没有拿它和敌方阵容做交集。

这意味着这个功能**大部分不需要新后端接口**，主要是前端交叉两份已有数据、
加一张卡片。

---

## 功能范围

新卡片名字就叫「**对位克制建议**」，出现在英雄选择阶段现有的推荐面板里
（`renderRecommendationArea`，`gameplay.js` 约 5512 行；就是"仅英雄选择阶段可
应用"装备方案那个面板），作为新增的一个页签或者嵌在「详情」页签顶部（两种都
行，GPT 按现有页签视觉风格挑一个，不要另开浮窗/悬浮窗——这个 app 是单窗口，
选人阶段这个面板本来切换到选人就自动出现）。

**触发条件（缺一个就整块不渲染，不出"暂无数据"占位）**：
- `data.phase === "ChampSelect"`；
- 本人分路已知（`self.position` 非空——蓝色 Blind Pick 30/无排位约束的模式
  可能没有分路，这种情况整块不显示，不要猜）；
- 敌方阵容里存在一个 `!isAlly && position === self.position && championLocked`
  的玩家（用现有 `data.players[]` 现成字段过滤，不需要新字段）。

**两种情况分别展示（可以同时出现，各自独立判断，命中才显示对应部分）**：

### A. 我已经选定/正在预览的英雄 vs 敌方同位置英雄（零新增上游请求）

推荐面板本身已经在为「本人英雄」拉 `Hero.StrongAgainst` / `Hero.WeakAgainst`
（见上文第 2 点）。前端只需要：

1. 找到敌方同位置英雄的 `championId`；
2. 在 `Hero.WeakAgainst` 里找有没有这个 `championId`（对面克我）→ 显示
   「这局对线偏劣势，胜率约 X%」+ 敌方英雄头像；
3. 在 `Hero.StrongAgainst` 里找有没有这个 `championId`（我克对面）→ 显示
   「这局对线偏优势，胜率约 X%」+ 敌方英雄头像；
4. 两边都没命中（覆盖率只有 5 条，命中率不是 100%）→ 不显示这一条，不写
   「暂无数据」。

这部分零新增后端代码、零新增上游请求——数据已经在推荐面板的现有响应里。

### B. 我还没选定英雄：给出打这个敌方英雄的候选克制英雄（新增一次前端触发的请求，复用现成接口）

敌方同位置英雄锁定、但本人还没锁定/没有明确预览英雄时，改为展示"针对
对方这个英雄，谁打他有优势"：

1. 前端对**敌方那个英雄**（不是本人英雄）发一次 `/api/champions/detail?
   mode=ranked&champion=<敌方英雄>&position=<本人分路>&tier=<本人当前段位>`——
   这个接口今天就存在（英雄库页面用的同一个），不用新写后端代码。
2. 取响应里 `counters.weakAgainst`（这批是"打这个敌方英雄处于劣势"的英雄，
   即：这些英雄是`敌方英雄`打不过的，因此对我方是好选择）——挑前 3～5 个，
   按英雄图标 + 胜率差展示。
3. 这次请求要节流/去重：敌方该位置英雄一旦确定就只请求一次，缓存到本局
   选人结束；敌方位置发生 `positionSwaps`（选人途中换位）导致映射变化时，
   按新映射重新请求一次，不要在每次 `/api/gameplay/live` 轮询（选人阶段轮询
   间隔本来就短）时都重新发。
4. 本人一旦锁定/预览了具体英雄，这部分让位给情况 A；不要两个同时展示同一
   个敌方英雄的信息造成重复。

**已知的边界情况（先记录，不是本单必须解决）**：
- 敌方选人途中换位（`positionSwaps`）：极端情况下可能出现"敌方这个位置的
  英雄"短暂对不上，展示可能滞后一次轮询周期；不强求瞬时正确，能在下一次
  `/api/gameplay/live` 轮询后纠正即可。
- 双方都没有分路信息的队列（如非排位的其它模式）：整块不显示，前面已经
  写了这条判定条件，GPT 按此实现即可，不必额外适配。

---

## 测试

- Node（`gameplay.test.cjs` 或同类文件）：
  - 敌方同位置英雄命中 `Hero.WeakAgainst` 里的一条 → 卡片渲染"对线劣势"文案
    与该敌方英雄头像，且只渲染这一条；
  - 命中 `Hero.StrongAgainst` → 渲染"对线优势"；
  - 两边都不命中 → 卡片整块不渲染（DOM 里不出现这张卡片的容器）；
  - 本人未选定英雄、敌方同位置英雄已知 → 触发一次对敌方英雄的 `detail` 请求，
    断言请求参数里 `champion` 是敌方英雄 ID、`position` 是本人分路；
  - 敌方同位置英雄不变时，多次轮询不重复发情况 B 的请求（断言 fetch 调用
    次数）；
  - `data.phase !== "ChampSelect"` 或本人分路为空 → 整块不渲染。
- 变异（都要让对应测试 FAIL 后整文件还原）：把"敌方同位置"的过滤条件去掉
  （改成随便一个敌方）；去掉请求去重导致每次轮询都发。

## 收尾

- `docs/WORKLIST-INDEX.md` 加 R167 一行，账本 `docs/history/ledgers/r167-execution-ledger.md`。
- 界面文案不加统计口径/方法论说明（CLAUDE.md 红线），只展示胜率差和英雄头像。
