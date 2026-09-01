# WORKLIST-R39（给 GPT 执行）

> 基于 0829 21:44 真机日志（`lol-loot-diagnostics-0829-2144.jsonl`，2836 行 / 3 小时 18 分钟）+ 6 张截图。
>
> **用户反馈：「对局里面几乎所有东西都用不了，问题很大」。**
>
> **本轮最重要的一条结论：我们上一轮加的诊断埋点被后端静默拒收了，所以 R38-B 组白做了一轮——
> 不是 GPT 没实现，前端代码写得完全正确，是后端的 `/api/diagnostics/client` 有一张写死的白名单，
> 把新事件直接 400 掉了，而前端是 `.catch(() => {})`，所以什么都看不见。**
> **这条必须第一个修，否则我们下一轮还是瞎的。**

---

## 零、先说 R38 验收结论（这部分已经修好了，不要动）

**A 组（左侧统计与右侧筛选解耦）真机实锤修好了。** 我在 R38 里定的两个硬指标都达成了：

```
① "source":"filtered-first-page"  → 全日志 0 次（R38 之前是元凶，现已彻底消失）
② 现在的行为：
{"event":"recent_ranked_sample_resolved","filter":"solo","queue_id":420,"source":"sgp-tag","tag":"q_420","matches":20,"time":"12:48:28.6126377Z"}
{"event":"recent_ranked_sample_resolved","filter":"flex","queue_id":440,"source":"sgp-tag","tag":"q_440","matches":20,"time":"12:48:28.6293470Z"}
```

两条请求**在同一毫秒内并发发出**、各自带 `q_420`/`q_440`、各自拿到 20 场，
之后再进入是 `source:"cache"`。完全符合 R38-A 的设计。**这块别再改了。**

R38 的第二个硬指标 `recommendation_mode_resolved` **仍然是 0 次**，B 组没修好，见下面 A 组。

---

## A 组（P0，必须第一个做）：诊断埋点被后端白名单静默拒收 —— 我们还在瞎着

### A-1 实证

R38-B 我要求加的 `live_recommendations_skip` 埋点，**前端实现得完全正确**
（`web/gameplay.js:2934-3006`，五个提前返回分支各记一条、带 reason 去重、30 秒飞行超时兜底、
空响应不写缓存，全都做到了）。

**但这个事件在 3 小时 18 分钟的日志里出现 0 次。**

根因在后端 `features.go:75-84`：

```go
type clientDiagnosticRequest struct {
	Event      string `json:"event"`
	Reason     string `json:"reason"`
	ChampionID int64  `json:"championId,omitempty"`
	QueueID    int64  `json:"queueId,omitempty"`
	Position   string `json:"position,omitempty"`
}

func (a *app) handleClientDiagnostic(w http.ResponseWriter, r *http.Request) {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()                        // ← 坑二
	if err := decoder.Decode(&request); err != nil
		|| request.Event != "specialist_runes_client_skip"   // ← 坑一：写死只收这一个事件
		|| !specialistRuneClientReasons[request.Reason] {    // ← 坑三：reason 也是写死白名单
		http.Error(w, "invalid client diagnostic", http.StatusBadRequest)
		return
	}
	...
}
```

**三重拒收**：① 事件名不等于 `specialist_runes_client_skip` 直接 400；
② 前端发的 body 里有个 `key` 字段，结构体里没有，`DisallowUnknownFields` 直接 400；
③ 前端发的 reason 里有 `has-payload`/`backoff`/`no-target` 等，不在
`specialistRuneClientReasons` 白名单里，还是 400。

而前端是 `void fetch(...).catch(() => {})` —— **400 被吞掉，一点痕迹都没有**。

★**这已经是这个项目第四次栽在"写死的白名单/枚举"上了**（queueId `-1` → `3100` → `480`
→ 现在是诊断事件名）。这个模式必须系统性解决，见 B-2。

### A-2 改法

1. **把 `clientDiagnosticRequest` 改成支持多个事件**：加一张
   `clientDiagnosticEvents = map[string]map[string]bool{}`，
   key 是事件名、value 是该事件允许的 reason 集合。
   现在至少要包含 `specialist_runes_client_skip`（沿用现有 reason 集）
   和 `live_recommendations_skip`（reason: `no-target`/`has-payload`/`cached`/`in-flight`/`backoff`）。
2. **结构体补上 `Key string \`json:"key,omitempty"\`` 字段**，
   并且**落盘前做长度截断（比如 128 字符）**，避免把过长内容写进日志。
   `key` 里只有 championId/position/gameMode/mapId/spellKey，没有隐私信息，可以记。
3. **`DisallowUnknownFields` 保留**（这是好的安全实践），但**前端发什么字段、
   后端结构体就要有什么字段**——这次就是两边没对齐。
4. **★关键：拒收时不要静默。** 现在 400 之后前端 `.catch` 吞掉、后端也不记录，
   等于两边都看不见。改成**后端在拒收时自己记一条诊断**：
   `{"event":"client_diagnostic_rejected","reason":"unknown-event|unknown-reason|decode-error","raw_event":"..."}`
   （`raw_event` 要截断+转义）。这样下次再出现两边不对齐，日志里一眼就能看到。
5. **前端 `recordLiveRecommendationSkip` 的 `.catch(() => {})` 改成**：
   非 2xx 响应时在浏览器控制台 `console.warn` 一次（同样按 reason 去重，不要刷屏）。
   静默吞掉错误是这次白瞎一轮的直接原因。

**变异测试**：
① 断言 `live_recommendations_skip` + 各个合法 reason 都能被接受并落盘；
② 断言带 `key` 字段不会被 `DisallowUnknownFields` 拒掉；
③ 断言未知事件名会被拒收**且记录 `client_diagnostic_rejected`**；
④ 把 A-2 第 1 点改回只收 `specialist_runes_client_skip`，测试必须 FAIL。

---

## B 组（P0）：图一 斗魂/海克斯没有海克斯推荐 —— 又是硬编码队列 ID 漏了一个

### B-1 真根因（代码实证）

`web/gameplay.js:109-110`：

```js
const HEXTECH_AUGMENT_QUEUE_IDS = new Set([2300, 2400]);
const ARENA_AUGMENT_QUEUE_IDS   = new Set([1700, 1710]);   // ← 缺 1750
```

用户截图一是「**斗魂竞技场 3x6**」，这个模式的 queueId 是 **1750**。
上一份日志里有铁证：
`{"event":"match_mode_classified","game_mode":"CHERRY","kind":"arena","map_id":30,"queue_id":1750,"queue_label":"斗魂竞技场 3x6"}`

**1750 不在 `ARENA_AUGMENT_QUEUE_IDS` 里**，所以 `liveAugmentRecommendationSource` 走不到 `"arena"` 分支。
（下面虽然有个 `gameMode === "CHERRY" && mapId === 30` 的兜底，但那依赖 live 载荷里
`gameMode`/`mapId` 两个字段都齐全且正确，游戏进行中未必；**不能靠兜底，要把 1750 补进去**。）

### B-2 ★结构性问题：后端有权威队列表，前端还在各自硬编码

R37 我们建了 `queue_groups.go` 那张权威队列对照表，**但前端完全没用上**，
它自己还留着至少三处独立的硬编码队列 ID 集合：
`HEXTECH_AUGMENT_QUEUE_IDS`、`ARENA_AUGMENT_QUEUE_IDS`、以及 `matchModeKind` 里的那些数组。

**这就是为什么后端 480 修好了、前端这边 1750 又漏了——两边各维护一份，必然漂移。**

**改法（这是本条的重点，不要只补一个 1750 了事）**：

1. **后端把权威队列表下发给前端**：在现有的某个启动期接口
   （比如 `/api/features` 或 catalog 类接口，选一个前端本来就会调的，不要新增一次请求）
   里带上 `queueGroups` 数组，每项包含 `{id, name, modeGroup, filter, recommendationMode, augmentSource}`。
   `augmentSource` 是新增字段，取值 `""`/`"arena"`/`"hextech"`，**由后端根据 modeGroup 推导**。
2. **前端删掉那三处硬编码集合**，改成从下发的表里查。
   拿不到表时的兜底：保留现在的 `gameMode`+`mapId` 语义判断（CHERRY+30→arena、KIWI+12→hextech），
   **但不要再保留硬编码的 queueId 数字集合**。
3. **`queue_groups.go` 里补上 1750 的 `augmentSource`**，并顺手核对
   1700/1710/1750/2300/2400 五个队列的 augmentSource 都对。

**变异测试**：
① 断言 queueId=1750 时 `liveAugmentRecommendationSource` 返回 `"arena"`；
② 断言前端源码里**不再出现** `new Set([1700, 1710])` 这类硬编码队列集合
（用正则断言，防止有人图省事又加回来）；
③ 把 1750 从后端表里删掉，测试必须 FAIL。

---

## C 组（P0）：图二/图四 上一局的数据没清理 —— 推荐缓存从来没被清空过

### C-1 真根因（代码实证）

用户反馈两条，是同一个根因：
- 图二：「我还没选英雄就有英雄推荐了，还是上局的英雄残留」
- 图四：「对局的详情里面上把和这把的玩家都放在一起了，上局残留数据没有清理干净」

全仓库 grep `state.liveRecommendations`，**只有 `.set()` / `.get()` / `.has()`，
一次 `.clear()` 或 `.delete()` 都没有**。

唯一的清理点是 `web/gameplay.js:4338` 的 `deep-legends:live-disconnected` 事件：

```js
window.addEventListener("deep-legends:live-disconnected", () => {
  state.live = null;
  state.liveError = "";
  state.liveRecommendationFlights.clear();   // 只清了飞行标记
  state.specialistRuneFlights.clear();       // 只清了飞行标记
  // ← state.liveRecommendations 没清
  // ← state.specialistRunes 没清
  // ← state.liveRecommendationFailures 没清
  // ← state.livePositionOverride 没清
});
```

**而且这个事件只在"客户端断开"时触发，换一局游戏是不会触发的。**
所以从头到尾，上一局的推荐缓存、绝活哥符文缓存、位置覆盖都原封不动带到了下一局。

### C-2 改法

1. **新增一个 `resetLiveGameScopedState()` 函数**，统一清理所有"只对当前这一局有意义"的状态：
   `liveRecommendations`、`specialistRunes`、`liveRecommendationFailures`、
   `liveRecommendationFlights`、`specialistRuneFlights`、`livePositionOverride`、
   `liveRecommendationSkipDiagnostics`、以及推荐页签的选中状态。
2. **在"换局"时调用它**。判据用 **gameId 变化**（`data.gameId`），
   以及**从非 ChampSelect 进入 ChampSelect**这两个时机
   （项目里 `resetLivePositionOverrides` 已经有 `gameChanged || leftChampionSelect` 这个现成判据，
   直接复用同一个判据，不要另发明）。
3. **`live-disconnected` 事件里也调用它**（替换掉现在那两行只清飞行标记的代码）。
4. **图四那个"我方/对方名单混了上一局的人"要单独查**：
   名单来自 `/api/gameplay/live` 的载荷，如果后端返回的就是混的，那前端清理再干净也没用。
   **请先确认后端 live 载荷里的 participants 是不是干净的**（加一条按 gameId 去重的诊断埋点
   `{"event":"live_roster_shape","game_id":N,"phase":"...","players":N,"with_stats":N}`），
   **确认是后端问题再改后端，不要凭猜**。

**变异测试**：
① 构造「gameId 从 A 变成 B」，断言 `liveRecommendations` / `specialistRunes` 等全部被清空；
把清理调用去掉，测试必须 FAIL。
② 断言"同一局内的普通轮询"**不会**误清缓存（防止过度清理导致每次轮询都重新请求）。

---

## D 组（P0）：图五/图六 OPGG 符文变成客户端内置、装备出不来

### D-1 现状与判断

- 图五：瑞兹，OPGG 页签下只有「客户端内置（备用）」，没有真正的 OPGG 符文
- 图六：嘉文四世，出装与技能页显示「等待推荐出装与技能数据」

`recommendation_mode_resolved` / `live_build_shape` / `champion_data_resolved`
**三个埋点全是 0 次**，说明 `/api/gameplay/recommendations` 这个接口**仍然一次都没被请求成功过**。
「客户端内置（备用）」正是拿不到 OPGG 数据时的兜底（走 `state.live.clientRecommendation`）。

**这两个症状是同一个根因的两个表现，根子就是推荐接口没被调用。**

### D-2 为什么我这轮还是不能直接告诉你根因

因为 A 组那个白名单——`live_recommendations_skip` 全被 400 掉了，
**我拿不到"到底是五个提前返回分支里的哪一个"这个关键信息**。

上游数据本身是好的，可以排除数据源问题：
`qq101_probe ok:true`、`counters_shape in:2/out:2`、`structured_recommendations` 8 条、
`arena_first_places accepted:10`、`load_top_players_shape parsed:5`。

### D-3 改法（严格按顺序）

1. **先做完 A 组**（让 `live_recommendations_skip` 能落盘）。
2. **同时在后端补一条"请求已到达"的埋点**，位置在
   `handleGameplayRecommendations` 的**第一行**（比现在 `recordRecommendationModeResolution`
   的位置还要靠前，且**不要做任何去重**，至少前 20 次全记）：
   `{"event":"live_recommendations_request","champion_id":N,"queue_id":N,"raw_position":"...","status":"received"}`。
   这样下轮日志就能确定性地区分**"前端没发"**还是**"发了但后端提前返回了"**——
   这两种情况的修法完全不同，现在只靠 `recommendation_mode_resolved=0` 区分不了。
3. **顺手修一个已经看出来的问题**：`recordRecommendationModeResolution` 和
   `recordChampionDataResolution` 的去重 map **在日志轮转时不会重置**。
   本份日志里有 `log_rotated`，如果这两个 key 在轮转前就记过，轮转后的日志里就永远看不到了。
   **改法**：日志轮转时把这些诊断去重 map 一起清空（`storage.go` 轮转那里发个信号），
   或者改成"每个 key 每 10 分钟可以再记一次"。
   **这条本身也可能是 `recommendation_mode_resolved=0` 的假象来源，必须排除掉。**

**注意**：这一组这轮**不要求你猜根因然后改**，要求你**把观测补齐**。
我们已经因为猜根因来回三轮了，这次先看清楚再动手。

---

## E 组（P1）：图三 符文应用失败 / 只点上了一部分

用户反馈：「OPGG 和绝活哥的符文应用都没有用，全是符文应用失败，
点击了之后客户端符文有些点上了有些没有，没点全」。
截图里的错误提示是「符文应用失败：请确认客户端仍在英雄选择或大厅阶段」。

**两个症状要分开查**：

1. **「应用失败」提示**：这个文案说明我们做了阶段校验。请确认校验条件是不是太严了——
   截图里用户明明就在英雄选择阶段（左边 LCU 就是英雄选择界面）。
   查一下我们判断"是否在英雄选择/大厅"用的是哪个字段、
   和 LCU 实际返回的 phase 值对不对得上（`ChampSelect`/`Lobby`/`Matchmaking`/`ReadyCheck`）。
   **加一条诊断**：`{"event":"perk_apply_attempt","phase":"...","outcome":"...","reason":"..."}`。
2. **「有些点上了有些没有」**：这个更严重，说明**写入是部分成功的**——
   符文页创建了但部分 perk 没写进去。可能原因：主副系搭配非法（比如副系选了主系的符文）、
   或者 perkIds 顺序/数量不符合 LCU 要求（LCU 要求恰好 9 个：主系 4 + 副系 2 + 小符文 3）。
   **请把实际发出的 perkIds 数组和 LCU 的响应体记进诊断**（perk ID 不是隐私数据，可以记），
   然后对照 Riot 的符文页格式要求逐条核对。

**这一组同样是"先补观测"**，不要凭猜改写入逻辑——符文写入是有副作用的操作，改错了会覆盖用户的符文页。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- **A、B、C 三组各写至少一个真变异测试**（改回旧行为 → 测试必须 FAIL → 复原变绿），
  变异结果贴在提交说明里。
- **A 组是本轮的前置条件，请第一个做完**——它决定了我们下一轮能不能看见 D/E 组的真相。
- **D 组和 E 组这轮只要求"补齐观测"，不要求猜根因去改**。
  我们已经在对局页这条链路上来回三轮了，每轮都在猜，这次先把日志看清楚。
- **B-2 那条"后端下发权威队列表、前端删掉硬编码集合"是本轮最重要的防复发措施**，
  我们已经因为"某处队列 ID 枚举漏了一个"错了四轮（`-1` → `3100` → `480` → `1750`），
  只补一个 1750 是不够的。
- 改完后请用户**重启应用**再复现，从「设置 → 隐私与能力 → 导出诊断日志」导出，
  **上传前把文件重命名一下**。
- 下轮验收我会重点看三个硬指标：
  ① `live_recommendations_skip` **必须出现**（出现 = A 组修好了，我们不瞎了）；
  ② `live_recommendations_request` **必须出现**（出现 = 前端真的发了请求）；
  ③ 队列 1750 下 `liveAugmentRecommendationSource` 返回 `arena`（B 组修好了）。
