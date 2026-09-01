# WORKLIST-R38（给 GPT 执行）

> 基于 0829 13:26 真机日志（`lol-loot-diagnostics-0829-1326.jsonl`，2770 行 / 25 分钟）+ 9 张截图。
>
> **先说好消息：R37 的服务端 tag 筛选在真机上被完整验证了，而且日志第一次抓到了探测结果。**
> 坏消息是本轮暴露的两个 P0 里，**A 组（左侧统计跟着右侧筛选变）的根因是我在 R37 里定的设计本身就错了**，
> 不是 GPT 写错。下面每条都有日志实证，不要凭感觉改。

---

## 零、R37 的真机验收结论（这部分不用改，是给你的背景）

**① `tag` 参数被真机实锤支持**，`sgp_queue_filter_probe` 事件终于被日志抓到了（这是这个探测
写下来之后第一次拿到结果）：

```
baseline_distinct_queues: [420, 1750, 2400, 3100]   ← 不带参数：混合模式
candidate "queueId"  → [420,1750,2400,3100]  matchesBaselineExactly=true   ← 被忽略
candidate "queue"    → [420,1750,2400,3100]  matchesBaselineExactly=true   ← 被忽略
candidate "tag"      → [420]                 looksFilteredToRanked=true    ← ★真的生效
candidate "gameType" → [420,1750,2400,3100]  matchesBaselineExactly=true   ← 被忽略
```

**四个候选参数里只有 `tag` 生效**，和 LeagueAkari 源码完全一致。
`sgp_match_filter_resolved` 14 次全部 `state:"supported"`，覆盖了
`ranked` / `q_440` / `q_450,q_930` / `q_400,q_430,q_480,q_490` / `q_2300,q_2400` 五组 tag，
**一次降级都没有发生**。

**② 队列 480 的修复也被实锤**：
`{"event":"match_mode_classified","game_mode":"SWIFTPLAY","queue_id":480,"mode_group":"match","kind":"more:match"}`
——快速模式已经正确归到匹配模式组，不再混进极地大乱斗。

**③ 绝活哥符文修好了**：`specialist_runes_done` 两次，`runes_returned:9`，
`specialist_runes_client_skip` 里已经没有 `no-top-players` 了（全是正常的 `cached`/`in-flight`）。
截图图四也确认绝活哥符文正常出来了。

**④ 探测代码可以删了**：`sgp_queue_filter_probe.go` 的使命已经完成（结论：参数名是 `tag`）。
但 `queueFilterCapability` 那套自适应降级机制**要保留**（那是运行时保护，不是探测）。
删的时候注意别把 `setQueueFilterCapability`/`queueFilterCapability` 一起删掉。

---

## A 组（P0，最严重）：左侧统计数据跟着右侧战绩筛选变 —— 我在 R37 定的设计错了

### A-1 用户的原话和现象（图五~图八）

> 「搜索的玩家一开始左侧是有单双排的统计数据的，灵活没有，然后右边战绩切换到灵活组排时
> 才有灵活的统计数据，这时候我再把左边切回单双排时居然数据没有了，这是什么操作，
> **必须要依赖右侧有战绩才有统计数据吗，那这统计数据有什么意义呢，左右是分开的**，
> 左侧统计数据只需要近20场，**必须在一开始就要统计好**」

用户完全说对了。四张截图连起来是一条完整的证据链：

- 图五：右侧「全部」→ 左侧「单双排」有数据（近 20 场，60% 胜率）
- 图六：右侧「全部」→ 左侧切「灵活组排」→ **「近 0 场排位 / 当前样本未发现灵活组排对局」**
- 图七：右侧切到「灵活组排」→ 左侧「灵活组排」**才有了**数据（近 19 场，26% 胜率）
- 图八：此时左侧切回「单双排」→ **「近 0 场排位 / 当前样本未发现单双排对局」**（原本有的数据没了）

### A-2 日志实证与真根因（这条是我的设计错误，不是 GPT 写错）

日志里 `recent_ranked_sample_resolved` 的 `source` 字段直接把根因写在脸上：

```
{"filter":"all",  "source":"sgp-tag","tag":"ranked","matches":20}      ← 图五/图六时
{"filter":"flex", "source":"filtered-first-page","matches":20}          ← 图七/图八时
```

`gameplay.go:1105 loadRecentRankedSamples` 现在的逻辑是：

```go
if pagination.ServerFiltered && (matchFilter == "solo" || matchFilter == "flex") {
    return matches   // ← 直接复用右侧战绩列表那一页
}
```

**这就是 R37 D 组我写的"已筛选到具体队列时直接复用首屏数据，零额外请求"。
当时我还把它当成优点验收通过了，实际上它把左右两块彻底耦合死了：**

- 右侧筛「灵活」→ 左侧拿到的 20 场**全是灵活** → 左侧切「单双排」自然是 0 场（图八）
- 右侧筛「全部」→ 走 `tag=ranked` 拿最近 20 场排位，**这 20 场恰好全是单双排**
  （用户最近在打单排，灵活是 5 天前的）→ 左侧切「灵活」自然是 0 场（图六）

**注意第二条：即使右侧是「全部」也一样出错**，所以这不只是"耦合"的问题，
`tag=ranked` 混着取 20 场这个做法本身就满足不了"两个队列各自都要有近 20 场"的需求。

### A-3 正确的设计（请严格按这个改）

**左侧三个模块（近 20 场排位 / 能力表现 / 位置偏好）必须有自己独立的数据源，
与右侧战绩列表的筛选状态完全无关。**

1. **改成按队列各取一次，两条独立请求**：
   - `tag=q_420` + `count=20` + `startIndex=0` → 单双排的近 20 场
   - `tag=q_440` + `count=20` + `startIndex=0` → 灵活组排的近 20 场
   - **这两条请求在总览首屏加载时就并发发出**，不等用户点任何东西
     （用户原话："必须在一开始就要统计好"）。
2. **`loadRecentRankedSamples` 里那段 `ServerFiltered && (solo||flex) → return matches`
   的复用逻辑整个删掉**。它是本条的直接根因。右侧筛选状态不允许影响左侧任何一个模块。
3. **`buildGameplayRankedQueues`（`gameplay.go:1151`）改成吃两份独立样本**：
   现在它对 420/440 都从同一个 `matches` 池里 `recentRankedMatchesForQueue` 过滤，
   池子里没有的队列就恒为 0。改成 `queues["420"]` 用 q_420 那份、`queues["440"]` 用 q_440 那份。
4. **缓存按 `serverID|playerRef|queueID` 分开存**，TTL 沿用现在的 10 分钟。
   两条请求各自独立缓存，不要合成一个 key。
5. **必须对搜索的其他玩家同样生效**（用户原话："只有当前玩家这些数据是统计好的，
   国服其他玩家都不行"）。这条路径走的是同一个 `loadRecentRankedSamples`，
   按上面改完应该自动覆盖，但**请专门验证一次搜索别人的场景**。
6. **「近 N 场」标题继续跟着实际样本数走**（现在已经是动态的，别改回写死 20）。

**成本评估**：从 1 次请求变成 2 次，每次 20 场。相比 R37 之前动辄翻 7 页，这个开销完全可以接受，
而且有 10 分钟缓存兜底。不要因为"多了一次请求"就退回复用方案——**功能正确性优先**。

### A-4 变异测试要求

1. 构造「右侧筛选 = flex」的场景，断言左侧单双排模块**仍然拿得到 420 的样本**
   （不是 0 场）；把 A-3 第 2 点的删除操作还原回去（即恢复 `return matches`），测试必须 FAIL。
2. 断言首屏加载时**恰好发起两次**带 `tag=q_420` 和 `tag=q_440` 的请求，
   且两次的 `startIndex` 都是 0、`count` 都是 20。
3. 断言第二次进入同一玩家时命中缓存、**零请求**。
4. 断言搜索其他玩家（非当前登录用户）时这两条请求同样会发出。

---

## B 组（P0）：对局页数据全没了 —— 推荐接口根本没被调用过

### B-1 日志实证（这条证据非常干净）

用户在图二/图三反映：对局里出装没了、胜率/选取率/禁用率全是「—」、
优劣势对抗显示「该位置暂无对线样本」、OPGG 符文变成了「客户端内置（备用）」。

**日志里 `recommendation_mode_resolved` 出现 0 次。**

这个埋点在 `handleGameplayRecommendations`（`gameplay.go:3131`）的**第 3153 行**，
位置在所有 `http.Error` 提前返回**之前**——只要这个 handler 被调用过一次，它就必然会记一条。
同理 `live_build_shape`（3229 行）和 `champion_data_resolved`（3220 行）也都是 **0 次**。

**结论：整整 25 分钟的对局页使用过程中，`/api/gameplay/recommendations` 这个接口一次都没被请求过。**
不是后端返回了空数据，是前端压根没发这个请求。

**交叉验证（排除"是不是没进对局"这种可能）**：同一时间段里绝活哥符文的接口是正常工作的——
`specialist_runes_handler` / `specialist_runes_start` / `specialist_runes_done` 各 2 次，
`runes_returned:9`。绝活哥走的是**另一个接口**，但它同样依赖前端的
`liveRecommendationTarget(data)` 能算出有效目标。**所以 `target` 是有效的**，
问题一定出在 `ensureLiveRecommendations` 内部的提前返回上。

上游数据也全是好的，可以排除数据源问题：
```
qq101_probe          champion:sylas  ok:true  forth_n:5 fifth_n:5 sixth_n:5  rune_pages_n:12
counters_shape       in:45  out:45  dropped_*: 全 0
load_top_players_shape  parsed:5
structured_recommendations  spell/starter/boots/core 全部 returned>0
```

### B-2 定位到具体这一行

`web/gameplay.js:2924`：

```js
async function ensureLiveRecommendations(data) {
    const target = liveRecommendationTarget(data);
    if (!target || data?.recommendations || state.liveRecommendations.has(target.key)
        || state.liveRecommendationFlights.has(target.key)) return;          // ← 就是这一行
    if (Date.now() - Number(state.liveRecommendationFailures.get(target.key) || 0) < 60_000) return;
    ...
}
```

`!target` 已经被绝活哥的正常工作排除了，所以是后面四个条件之一：

| 可能原因 | 说明 |
|---|---|
| `data?.recommendations` 为真 | 但后端 live 载荷里**没有** `recommendations` 字段（全仓库只有 `gameplayRecommendationsResponse` 有），基本可排除，但要确认前端有没有在别处给 `state.live` 塞过这个字段 |
| `state.liveRecommendations.has(key)` | **最可疑**：缓存里被塞进了一个空的/降级的条目，导致此后永远不再请求。要查是谁写进去的 |
| `state.liveRecommendationFlights.has(key)` | 飞行标记泄漏（`finally` 里虽然会删，但如果 key 在中途变了就删不掉——注意 `key` 里含 `spellKey`，**召唤师技能在英雄选择阶段会变**，`target.key` 因此会变，这是个真实的泄漏路径） |
| 失败退避 60 秒 | 只能解释 60 秒，解释不了 25 分钟 |

**★我个人最怀疑第三条**：`target.key` 的组成是
`${championId}:${position}:${gameMode}:${mapId}:${spellKey}`，
而 `spellKey` 来自 `self.spell1Id/spell2Id`。在英雄选择阶段召唤师技能是会变的，
`gameMode`/`mapId` 在早期快照里也可能是空的。如果请求发出时 key 是 A、
`finally` 执行时重新算出来的 key 已经是 B，那 **A 这个飞行标记就永远留在 `Set` 里了**，
之后凡是算出 key = A 的请求全部被挡掉。

### B-3 改法

1. **先加诊断埋点确认到底是哪一条**（不要跳过这步直接改，我们已经因为"猜根因"来回好几轮了）：
   在 `ensureLiveRecommendations` 的提前返回处记一条前端诊断，
   字段：`{"event":"live_recommendations_skip","reason":"has-payload|cached|in-flight|backoff|no-target","key":"...","championId":N}`，
   **按 reason+key 去重**（参考项目里已有的去重写法，别再刷屏）。
2. **不管埋点结果如何，飞行标记的泄漏都要修**：
   `ensureLiveRecommendations` 开头把 `target.key` 存进一个局部常量，
   `finally` 里**用这个局部常量**删除飞行标记，而不是重新调用 `liveRecommendationTarget()` 再算一次。
   （现在 `finally` 里是 `state.liveRecommendationFlights.delete(target.key)`，`target` 是闭包变量所以
   其实是对的——**但请顺着 `renderLive()` 和 `liveRecommendationTarget(state.live)?.key === target.key`
   这两处再核一遍**，那两处是重新算的。）
3. **给飞行标记加超时兜底**：任何 key 在 `Set` 里超过 30 秒自动清除，
   防止将来再出现类似的永久卡死。这是防复发措施，必须做。
4. **`state.liveRecommendations` 缓存不允许写入空值**：
   只有 `response.recommendations` 存在且至少有一项有效数据时才写缓存；
   否则走失败退避路径。防止"缓存了一个空对象导致永远不再重试"。

### B-4 变异测试要求

1. 构造「飞行标记残留」场景（手动往 `state.liveRecommendationFlights` 塞一个 key），
   断言 30 秒兜底生效后请求能正常发出；把兜底去掉，测试必须 FAIL。
2. 断言接口返回一个空的 `recommendations` 时**不写缓存**、且下次仍会重试。
3. 断言正常路径下 `/api/gameplay/recommendations` **确实被请求了一次**
   （现在这条链路是零测试覆盖，这也是为什么这个回归能溜过 R37 验收——请补上）。

---

## C 组（P1）：图一 空态区域出现了两个一模一样的加载中

### C-1 真根因（代码实证，两处同时渲染了同一段文案）

切到没有战绩的模式时，截图里出现了**两行一模一样的**
「正在查找更早的极地大乱斗对局…第 2 页」。原因是这段文案被渲染了两次：

```js
// web/gameplay.js:349  —— 空列表区域
if (tab.filterPaging) {
  return `<div class="match-filter-paging" role="status">${paginationCopyFor(tab)}</div>`;
}

// web/gameplay.js:322  —— 列表底部的分页哨兵（是另一个 DOM 节点，一直都在）
sentinel.innerHTML = tab.filterPaging ? paginationCopyFor(tab) : '...正在加载下一批战绩…';
```

**空列表时这两个节点都存在、且都调用了同一个 `paginationCopyFor(tab)`**，所以显示两遍。

### C-2 改法

列表为空且正在翻页时，**只保留一处**。建议保留空态区域那一处（视觉上居中更自然），
把底部哨兵在"列表为空"时隐藏掉（`sentinel.hidden = true` 或不渲染该节点）。
列表非空时维持现状（哨兵正常显示、空态区域本来就不渲染）。

**变异测试**：断言「`filterPaging:true` + 空列表」时渲染结果里
`正在查找更早的` 这段文案**只出现一次**；改回两处都渲染，测试必须 FAIL。

---

## D 组（P1）：对局页交互与信息补全

### D-1 切换英雄时两级页签都要复位

用户原话：「我在对局中一个英雄的符文推荐切换成了绝活哥，然后换成另外一个英雄，
这时候 tab 还在绝活哥上，应该都要还原，上面的 tab 也要还原到符文」。

改法：监听 `liveRecommendationTarget(data).championId` 变化，一旦英雄变了：
- 外层页签（符文 / 详情 / 出装与技能）复位到**符文**
- 内层符文来源页签（OPGG / 绝活哥 / 职业选手）复位到 **OPGG**

注意判据用 `championId` 变化，**不要用 `target.key` 变化**——
key 里含 `spellKey` 和 `position`，换个召唤师技能或改个分路也会变，那样会误复位。

**变异测试**：断言"英雄从 A 换到 B"后两个页签状态都回到默认值；
断言"同一英雄只改了召唤师技能"时页签**不复位**（防止过度复位）。

### D-2 胜率区域的位置调整（图三）

用户要求：胜率/选取率/禁用率那一组指标**往左靠近召唤师名称**，大约间隔 10px，
**中间放一条分割线**。

现在这组指标离英雄名称太远（截图里中间有一大片空白）。
改法：把这组指标紧跟在英雄名/位置那一块后面，间距 10px，
中间加一条竖直分割线（用现有的分割线令牌/样式，不要新造颜色）。

### D-3 绝活哥符文右侧补充对手信息（图四）

现在绝活哥符文右侧只显示了英雄图标 + 「胜率 51% · 场次 824」。
用户要求**再加上对手的召唤师名称、段位和胜率**。

注意：这里的"对手"指的是**那位绝活哥玩家那一局的对手**。
请先确认 `specialist_runes` 的上游载荷里到底有没有对手的召唤师名/段位——
`load_top_players_shape` 只 `parsed:5`（5 个绝活哥玩家），**没有对手信息**。
**如果上游拿不到，不要硬编造**，先在工单里回报"上游没有这个字段"，
我们再决定是换数据源还是改需求。**不允许用占位数据糊上去。**

### D-4 英雄详情横条高度再压一点（图九）

英雄详情页顶部那条横条（英雄名 + 梯度/胜率/选用率/禁用率 + 分路占比）**高度再减少一点**。
上一轮已经压过一次，这次再压。请用 Playwright 截图对比改前改后，
**同时确认 480px 窄屏下没有出现文字截断或换行**（这个横条以前出过截断问题）。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- **A、B、C、D-1 四组各写至少一个真变异测试**（改回旧行为 → 测试必须 FAIL → 复原变绿），
  变异结果贴在提交说明里。
- **B 组请务必先加埋点确认根因再改**，不要直接猜着改。B-3 的第 2/3/4 点是无论如何都要做的加固。
- **A 组是本轮最重要的一条**，它是我在 R37 里设计错的，请严格按 A-3 的六点改，
  不要为了省一次请求又把左右两块耦合回去。
- `sgp_queue_filter_probe.go` 可以删了（结论已拿到：参数名是 `tag`），
  但 `queueFilterCapability` 那套自适应降级要保留。
- 改完后请用户**重启应用**再复现，从「设置 → 隐私与能力 → 导出诊断日志」导出，
  **上传前把文件重命名一下**。
- 下轮验收时我会重点看：`recommendation_mode_resolved` 这个事件**必须出现**
  （出现 = B 组修好了），以及 `recent_ranked_sample_resolved` 里**不能再有
  `source:"filtered-first-page"`**（消失 = A 组修好了）。这两个是硬指标。
