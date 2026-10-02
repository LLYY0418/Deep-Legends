# WORKLIST-R158：来回切换总览页玩家标签后图标重新加载；顺带看一眼总览加载耗时诊断字段是否失真

诊断人：Claude（只读：用户提供的诊断日志 `e926fe78-lol-loot-diagnostics-0925-1746.jsonl` + 读代码核对实现，未改仓库任何文件）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.21（`desktop/package.json`），仓库 HEAD `35788b9f`，工作区含 R144–R157 一路带下来的未提交改动。本单所有行号均以当前工作区文件（含未提交改动）为准，执行前请先 `grep` 确认行号未被后续改动移动。
状态：代码与本机自动验证完成，待新诊断日志及 Windows 真机复核；执行详情见 `r158-execution-ledger.md`。
触发：用户反馈——「韩服玩家总览现在在没有打开客户端的情况下图标都出现了（R154 P1 已生效），但是离开一个玩家的总览页后再回去，图标还要重新加载，之前都加载过了」，并追加「加载速度方面看看能不能再优化一下」，附诊断日志 `e926fe78-lol-loot-diagnostics-0925-1746.jsonl`（3,627 行，`run_id=96ce4d31ebc6ebdb2535a777`，覆盖 09:16:30Z–09:44:36Z+，与 R157 引用的日志同一次 `run_id`，是同一场会话更完整的导出）。

建议把该诊断日志放进 `docs/r158-validation/e926fe78-lol-loot-diagnostics-0925-1746.jsonl`（沿用既有目录命名习惯）。

## 结论摘要

| 项 | 现象 | 根因（一句话） |
|---|---|---|
| P1 | 离开一个玩家的总览页再回去，头像/背景图/生涯统计/战绩列表里的图标都要重新加载一遍，即使这个玩家的数据完全没有变化、也没有重新发网络请求 | 总览页在同一个玩家分组里只有**一个共享容器**，「保留旧 DOM 节点，避免图片重新入队」这套判断只跟「上一次渲染进这个容器的是不是同一个玩家」比较——只要中途切换到过别的玩家标签，这个判据就必然为假，保留逻辑整体失效，哪怕又切回原来那个玩家、数据分毫未变 |
| P2（观察项，非确定性结论） | 诊断日志里 `recent_players` 这个耗时字段经常和 `total` 几乎相等，看起来像总览页最大的耗时来源 | 这个字段用的是「距上一个检查点过了多久」的计时方式，而它是全流程里**最后一个**检查点，中间那些已经各自单独计时的并行阶段（`champion_names`/`detailed_matches`/`season_snapshot`/`ranks`/`mastery`/`recent_ranked`）全部被它重复计入，字段名和实际含义对不上，会误导后续找真正的瓶颈 |

---

## P1　总览页玩家标签之间来回切换，图标重新走一遍限流准入队列——保留判据只认「上一次」，不认「这个玩家」

### 证据

**共享容器**：`backend/web/gameplay.js:632`
```js
function overviewContainer(tab) {
  if (tab?.overlay) return nodes.playerOverlayContent;
  const group = tabGroup(tab);
  return group === overviewGroupForSection() && tab?.key === state.activeTabs[group] ? overviewWorkspace(group).content : null;
}
```
同一个玩家分组（`players`，涵盖「当前召唤师」、搜索出的国服玩家、韩服玩家等所有非职业选手标签）共用 `overviewWorkspace(group).content` 这**一个** DOM 容器；切换到哪个玩家标签，这个容器的内容就整体替换成谁的。

**「是否是同一次总览」的判据只看容器上一次渲染的是谁**：`backend/web/gameplay.js:1886-1892`
```js
function renderOverviewBodyContent(container, tab) {
  const tierScope = matchTierScope(tab);
  ...
  const sameOverview = container.dataset.matchTierScope === tierScope;
  container.dataset.matchTierScope = tierScope;
```
`tierScope` = `region:serverID:playerRef`（`matchTierScope`，约 1266 行），即「这一份数据是谁的」。`sameOverview` 的意思其实是「上一次往这个共享容器里塞东西的，跟这一次是不是同一个玩家」——而不是「这个玩家的这份数据，之前是不是已经完整渲染过」。

**保留旧 DOM（从而避免图片重新排队）的每一处判断都以 `sameOverview` 为前提**：`gameplay.js:1908-1912`（战绩列表）、`:1955`（生涯统计区块）、`:1963-1966`（头像+背景图所在的 `.summoner-strip`、单独的 `.summoner-strip-art`、`.career-column`）：
```js
const retainedMatchList = container.querySelector(".match-list");
const preserveMatchList = Boolean(retainedMatchList && sameOverview && container._matchListViewRevision === Number(tab.matchViewRevision || 0));
...
const careerSections = sameOverview && container._careerSignature === careerSignature ? "" : renderCareerSections(data, tab);
...
const retainedStrip = sameOverview && container._stripHTML === stripHTML ? container.querySelector(".summoner-strip") : null;
const retainedArt = !retainedStrip && sameOverview && container._backgroundArt === backgroundArt ? container.querySelector(".summoner-strip-art") : null;
const retainedCareer = sameOverview && container._careerSignature === careerSignature ? container.querySelector(".career-column") : null;
```
随后 `:1970` 起用新拼好的 `container.innerHTML` 整体重写容器，再在 `:1998-2000` 把保留下来的旧节点换回去——这一整套「先留旧节点、重写完再换回来」的机制本身是好的，能避免已经加载完成的 `<img>` 元素被销毁重建。**但它只在 `sameOverview === true`（即「上一次渲染的就是同一个玩家」）时才生效**。用户描述的操作——离开玩家 A 的总览页（切到玩家 B，或点开某个队友的资料），再回到玩家 A——第二次回到 A 时，容器上一次记的 `dataset.matchTierScope` 是 B 的 `tierScope`（切到 B 时被覆盖成 B 了），跟 A 的 `tierScope` 不相等，`sameOverview` 必然是 `false`。于是 `retainedMatchList`/`careerSections` 复用/`retainedStrip`/`retainedArt`/`retainedCareer` **全部落空**，即使 `tab.data`（A 的数据）是同一个对象、完全没有变化（没有触发 `shouldReloadOverview`，见下），也会整体重新生成 `stripHTML`（含头像背景图 `<img data-queued-src>`）、整个生涯统计区块、整份战绩列表——里面的 `<img>` 全部是新建的 DOM 节点。

**新建的 `<img>` 元素即使 URL 之前成功过，也要重新走一遍限流准入**：`backend/web/image-queue.js:74-77`
```js
// A successful URL is still admitted through the same queue. The cache only
// records that it succeeded recently; it is not evidence that the browser
// cache can satisfy a new element without opening a connection.
if (loadedURLs.has(url) && Date.now() - loadedURLs.get(url) >= 600000) loadedURLs.delete(url);
```
队列同时只放行 `IMAGE_QUEUE_LIMIT = 5` 张图（`image-queue.js:4`，远程源另外单独限额 `REMOTE_LANE_LIMIT = 2`，`:11`），新建的这些 `<img>` 元素要重新排队等名额，用户看到的就是「明明都加载过了，怎么又要等」——这正是 R154 P5 在收藏页发现的同一种架构模式（「即使 URL 最近成功过，新的 `<img>` 元素仍会重新走一遍限流准入」），这次出现在总览页的玩家标签切换上。

而 `shouldReloadOverview`（`gameplay.js:422-424`）：
```js
function shouldReloadOverview(tab, now = Date.now()) {
  return Boolean(tab?.dirty) || !tab?.data || now - Number(tab.loadedAt || 0) >= 120_000;
}
```
只要没有标脏、数据存在、且距上次加载不到 2 分钟，回到这个玩家标签时并**不会**重新发起网络请求（这点没问题，日志里 27 分钟间隔的几次 `overview_load_cost` 正是超过这个阈值后的正常重新拉取，不是本单要修的问题）——问题完全出在拿到（未变的）数据之后，**渲染层**把已经加载好的图标又扔进了重新排队的流程。

### 根因

「保留旧 DOM、避免图片重新入队」这套优化，判据挂在共享容器的“上一次渲染的是谁”上，而不是挂在“这个玩家标签自己的数据/标记有没有变”上。总览页的玩家标签本来就是多个标签共用一个容器，只要中途看过任意一个别的玩家（哪怕只是瞟一眼又切回来），这个共享的“上一次”状态就会被覆盖，保留逻辑对所有玩家标签之间的来回切换必然失效。

### 修复方向

把“要不要保留旧节点”的判断，从挂在共享容器上的单一「上一次」状态，改成挂在**每个玩家标签自己**身上的状态。具体来说：
1. 把 `_stripHTML`/`_backgroundArt`/`_careerSignature`/`_matchListViewRevision`（以及对应的 DOM 节点引用）从 `container.xxx` 挪到 `tab.xxx`（或者一个以 `tab.key`为键、有限容量的旁路缓存），离开某个玩家标签、容器要被别的玩家标签接管之前，把该标签当前的这些节点从 DOM 里摘下来存好（而不是任其被 `container.innerHTML = ...` 整体覆盖丢弃）。
2. 再次渲染某个玩家标签时，先看这个标签自己缓存的签名/HTML 字符串是否与新算出来的一致，一致就直接换回缓存里那份**已经加载完成**的节点，不一致（数据真的变了）才重新生成。
3. 缓存要设置上限（比如只保留最近打开过的几个玩家标签的节点），避免长时间开着很多标签导致内存无限增长；被淘汰的旧节点正常按现在的方式重新走一遍渲染即可，不算回归。
4. 这一条独立于「多久没打开就要不要重新发请求」的 `shouldReloadOverview` 判断，只改渲染层，不改网络请求的触发时机。

### 验收

- 打开玩家 A 的总览页，等头像、背景图、生涯统计、战绩列表里的图标都加载完成；切到玩家 B；在 2 分钟内切回玩家 A：确认 A 页面里之前已经加载完成的图标不再重新经过限流准入（不再有短暂的占位/空白），且没有发起新的图片网络请求。
- 同样的来回切换场景下，若玩家 A 的数据在离开期间确实发生了变化（比如比赛结束后总览被标脏），确认能正确地重新渲染出最新内容，不会因为缓存旧节点而显示过期数据。
- 补充/更新前端单测覆盖：`sameOverview` 判据改造后，「同一玩家反复渲染」「不同玩家来回切换后返回」两类场景都要有断言，防止本单修复后被后续改动悄悄改回「按容器上一次」的旧判据。

---

## P2（观察项）　`recent_players` 耗时字段和 `total` 几乎相等，是计时口径问题，不是它本身很慢

### 证据

日志里 `overview_phases_ms`（`log_seq 78`）：
```json
{"account":0,"champion_names":1004,"detailed_matches":527,"details":0,"identity":0,"mastery":28,"matchIDs":0,"opgg-historical":0,"queue_labels":47,"ranks":23,"recent_players":1877,"recent_ranked":345,"season_snapshot":1,"serialize":7,"total":1885}
```
`recent_players`（1877ms）几乎等于 `total`（1885ms）；而 `champion_names`（1004）+ `detailed_matches`（527）+ `queue_labels`（47）+ `ranks`（23）+ `mastery`（28）+ `recent_ranked`（345）+ `season_snapshot`（1）+ `serialize`（7）加起来已经有 1982ms——比 `total` 还多。这些字段之间明显是重叠计时，不是互斥的连续区间。

代码层面能确认这一点：`backend/gameplay.go` 里两种计时函数语义不同——
- `markSpan`（`:141-149`）记录一段**独立测量**的区间（并发 goroutine 自己算好的 `started`/`finished`），不会推进任何全局指针；
- `mark`（`:130-138`）记录**「距上一次 `mark()` 调用过了多久」**（`now.Sub(p.last)`），而且只有 `mark()` 自己会推进 `p.last`，`markSpan()` 不会。

`loadGameplayOverview` 里，`identity` 之后一路到 `recent_players` 之间的所有中间阶段——`queue_labels`（`:1311`）、`champion_names`（`:1312`）、`detailed_matches`（`:1316`）、`season_snapshot`（`:1348`）、`ranks`（`:1401`）、`mastery`（`:1417`）、`recent_ranked`（`:1542`）——用的**全部是 `markSpan`**，唯独最后 `recent_players`（`:1583`）用的是 `mark`。而这个请求周期里，上一次调用 `mark()` 是在 `handleGameplayOverview` 里、调用 `loadGameplayOverviewDeduplicated` 之前的 `phases.mark("identity")`（`:852`）。也就是说：从「identity」打点到「recent_players」打点之间，没有任何一次 `mark()` 调用推进过 `p.last`——`recent_players` 这次 `mark()` 算出来的时长，等于从 identity 一路到 recent_players 计算完成为止的**全部**挂钟耗时，天然把中间那些已经各自单独用 `markSpan` 计过时的并行阶段全部重复计入了一遍。它的名字叫「recent_players」，听起来像是「最近一起玩」这一项本地聚合计算的耗时，实际上是「除了 identity 和最后 serialize 之外的一切」的挂钟耗时总和，字段名和它测的东西对不上。

### 根因

`recent_players` 这个检查点恰好是 `identity` 之后**第一个**用 `mark()`（而不是 `markSpan()`）打点的位置，导致它把中间一串并行阶段的挂钟时间全部吞并计入，产生了「recent_players 几乎等于 total」这种看起来很吓人、但实际不代表 recentPlayers() 本身很慢的假象。这不是一个真实的性能问题，而是诊断字段的计时口径问题——但正因为它看起来最显眼，容易把后续排查性能问题的注意力引到错误的方向上。

### 修复方向

这一条本单不下确定性结论、也不建议直接改动性能相关代码，只建议先把计时口径修准，让后续（如果真要做加载速度优化）有可信的数据可看：
- 要么在 `ranks`/`mastery`/`recent_ranked` 等并行阶段各自的 `markSpan` 调用之后，各插一次 `mark()` 把检查点推进一下，让 `recent_players` 最终只反映它自己那段本地聚合计算（`aggregateMatches`/`championStats`/`positionStats`/`buildGameplayAbilityProfile`/`buildGameplayRankedQueues`/`activityHours`/`recentPlayers` 这几步）的真实耗时；
- 要么给这个字段改个更准确的名字（比如拆成「等待并行阶段」+「本地聚合计算」两段），避免继续用一个和实际含义不符的字段名。

计时口径修准之后，如果本地聚合计算确实还是偏慢，再针对性地看是否需要优化；如果修准后发现大部分时间其实花在某个具体的并行阶段（比如 SGP 历史请求）上，则应该针对那个真正慢的阶段去看是否有优化空间——这一点目前证据不足以下结论，留给下一轮如果用户还觉得慢、且诊断字段已经修准之后再判断。

### 验收

- 计时口径调整后，重新生成一份诊断日志，确认各阶段耗时字段之和与 `total` 大致吻合（允许合理的多线程重叠误差），不再出现某个字段单独接近甚至等于 `total` 的异常情况。
- 确认调整只影响诊断字段的准确性，不改变总览页任何实际的加载行为或返回数据。

---

## 结论

用户报告的「离开一个玩家总览页再回去图标要重新加载」，代码层面有明确、可确定复现的原因：总览页同一分组下所有玩家标签共用一个 DOM 容器，判断「能不能保留旧图标节点」的依据只是「容器上一次渲染的是不是同一个玩家」，而不是「这个玩家自己有没有变」，导致只要中途切换过其他玩家标签，回到原玩家时保留机制必然失效，图标要重新走一遍限流准入队列——这不是网络请求变多了（`shouldReloadOverview` 的 2 分钟节流仍然生效），纯粹是渲染层把已经加载好的图标又扔了一遍重建。顺带看了一眼加载速度，诊断日志里最显眼的 `recent_players` 耗时字段其实是计时口径问题（把好几个并行阶段的挂钟时间重复计入了），建议先修准这个字段再判断总览页是否真的还有值得优化的慢点。
