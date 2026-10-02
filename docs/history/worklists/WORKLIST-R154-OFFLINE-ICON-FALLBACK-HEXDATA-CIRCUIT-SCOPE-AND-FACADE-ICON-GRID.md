# WORKLIST-R154：客户端未启动时总览图标缺失、海斗/斗魂英雄详情「熔断连坐」、头像收藏页三处显示问题

诊断人：Claude（只读：用户上传的 4 张截图 + `lol-loot-diagnostics-0925-1427.jsonl` 诊断日志 + 读代码核对实现，未改仓库任何文件）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.19（`desktop/package.json`），仓库 HEAD `35788b9f`，工作区含 R144–R153 一路带下来的未提交改动（`backend/hexdata.go`、`backend/champions.go`、`backend/web/app.css`、`backend/web/favorites-facade.js` 等均在其中）。本单所有行号均以当前工作区文件（含未提交改动）为准，不是 HEAD 提交的行号，执行前请先 `grep` 确认行号未被后续改动移动。
状态：代码与本机自动验证通过，待 Windows 真机截图和新诊断日志（详见 `r154-execution-ledger.md`）。
触发：用户一次给了 4 张截图——① 韩服总览页客户端未启动时大量小图标缺失；②③ 海克斯大乱斗 / 斗魂竞技场英雄详情页出现「详情读取失败 本地请求超时，请重试」；④ 收藏页头像视图未拥有条目被整体灰阶变黑（要求改成和旗帜视图一样），并追加两个交互问题——第一次从头像切到旗帜、数据未到位前网格里残留头像卡片；切换「显示未拥有」开关时，看起来所有头像都被重新加载了一遍——同时附了一份诊断日志 `lol-loot-diagnostics-0925-1427.jsonl`（4,139 行，`run_id=ee2c19ab41d3669617cfd8b2`，覆盖 05:33:31Z–06:27:21Z）。

证据文件建议放进 `docs/r154-validation/lol-loot-diagnostics-0925-1427.jsonl`（沿用 R136/R153 的目录命名习惯），本单引用的字段全部来自这份原始日志。

## 结论摘要

| 项 | 现象 | 根因（一句话） |
|---|---|---|
| P1 | 总览页客户端未启动时很多小图标缺失 | `communitydragon` 源的图片接口没有像 `gtimg` 源那样接到失败回落，而 `raw.communitydragon.org` 在当前网络下整场 100% 超时 |
| P2 | 海克斯大乱斗 / 斗魂竞技场英雄详情「本地请求超时，请重试」 | `hexdata.com.cn` 一次英雄详情超时后，熔断器按「数据类型」而非按英雄/模式生效，牵连接下来 1–30 分钟内**任何英雄、两个模式**的详情请求全部秒失败 |
| P3 | 头像未拥有条目被整体灰阶变黑 | R130 只给旗帜的锁定态去掉了灰阶滤镜，忘了给头像补同样的规则，头像仍吃着更重的通用灰阶样式 |
| P4 | 切换到旗帜视图，数据到位前网格里还是头像卡片 | 目录接口是异步请求，请求发出后网格没有立即清空/进入加载态，旧视图的卡片一直留到新数据到达才被替换 |
| P5 | 切换「显示未拥有」看起来所有头像都重新加载了一遍 | 每次筛选变化都整体销毁重建全部卡片 DOM，包括筛选前后都可见、本该不受影响的条目，图片队列自己的注释也写明「即使 URL 最近成功过，新的 `<img>` 元素仍会重新走一遍限流准入」 |

---

## P1　总览页客户端未启动时，很多图标缺失——`communitydragon` 源没有失败回落，而它在当前网络下几乎每次都会失败

### 证据

`lol-loot-diagnostics-0925-1427.jsonl` 开局第 2 条：
```
{"event":"lcu_discovery","detail":"未检测到 LeagueClientUx 进程","result":"process-not-found", ...}
```
即整段日志期间客户端（LCU）从未连接。

同一份日志里 `asset_fetch` 按 host 聚合的窗口统计，`raw.communitydragon.org` 在日志覆盖到的每一个窗口里都**接近或等于 100% 失败**，例如：
```
{"host":"raw.communitydragon.org","requests":135,"failures":135,"cache":{"error":11,"host-backoff":1,"negative":123}}
{"host":"raw.communitydragon.org","requests":131,"failures":130,"cache":{"error":5,"miss":1,"negative":125}}
{"host":"raw.communitydragon.org","requests":159,"failures":159,"cache":{"error":11,"host-backoff":1,"negative":147}}
```
同期 `ddragon.leagueoflegends.com`、`game.gtimg.cn` 两个源全部成功（`failures:0`），说明不是整体断网，是这一个域名在当前网络环境下打不通/长期超时。`asset_host_backoff` 事件（`consecutive_timeouts:3`、`backoff_seconds:60`）在整段日志里对 `raw.communitydragon.org` 反复触发了 20 次以上——`backend/asset_cache.go:17`（`assetHostTimeoutThreshold = 3`）、`:127`（`assetHostBlocked`）、`:137`（`observeAssetHostResult`）这套退避机制本身工作正常，它只是如实反映了「这个域名一直连不上」的事实，退避逻辑不是问题所在。

### 根因

`backend/champions.go:1018` 的 `handleChampionAsset`：当 `source == "communitydragon"`（`:1031` 起）时，函数先探测本机客户端（`clientAssetStatus`，客户端未启动时必然失败），再对候选路径直接发起 `raw.communitydragon.org` 远程请求，**失败后在 `:1078` 直接 404 返回**，不会走到函数末尾（`:1088`）「远程失败后尝试 `championAssetFallback` 换一个主机」那段代码——那段代码对这个分支而言**是死代码，永远不会被执行到**，因为 `source=="communitydragon"` 分支自己在 `:1078` 就 `return` 了。

即使执行流程能走到 `championAssetFallback`（`:1289`），它内部第一步 `parseGtimgAssetRef(source, requestPath)`（`:1261`）也会因为 `if source != "gtimg" { return "", 0, false }`（`:1262`）直接拒绝——这个回落函数从设计上就只服务 `gtimg` 源失败后转 Data Dragon 的场景（`:1286-1288` 原注释：「maps a failed Tencent CDN request onto the matching Data Dragon asset」），从未覆盖 `communitydragon` 源失败的场景。

结果：客户端未启动 + `raw.communitydragon.org` 在当前网络下打不通，这两个条件一叠加，所有走 `communitydragon` 源的图标（截图里参与者头像格、部分英雄方块图标）就没有任何退路，只能显示前端的文字兜底（首字符占位，即截图里能看到的单字圆点）。这和 R127/R145 已经给别的图片路径（`gtimg` 失败转 ddragon、`/fe/` 战利品图标失败转 CommunityDragon）做过的是同一类修复，唯独 `communitydragon` 源本身失败时没有对应的下一跳。

### 修复方向

1. 给 `handleChampionAsset` 的 `communitydragon` 分支补一条失败回落：候选路径全部失败后（现在 `:1078` 直接 404 之前），尝试用请求路径推导出等价的 Data Dragon 或 `gtimg` 路径再取一次一一具体映射关系可以扩展 `championAssetFallback`（`:1289`）已有的「按 `skinID`/`championKey` 换算 Data Dragon 路径」思路，而不是照抄 `parseGtimgAssetRef` 的 `gtimg` 专属解析。`communityDragonChampionAssetCandidates`（`:1097`）已经知道怎么从请求路径解析出英雄/皮肤信息，可以在同一处补上「这些信息也够换算出一个 Data Dragon 等价路径」。
2. 回落命中前先查一下 `raw.communitydragon.org` 是否已经处于 `assetHostBlocked` 退避窗口内（`asset_cache.go:127`）——如果是，直接跳过它走回落，不要每次都老老实实先等一遍它的失败，减少不必要的等待。
3. 现有的 `augment_icon_fetch` 诊断（`champions.go:1117`）已经在记录 `fell_back`/`lcu_status`，回落成功后同样要能在诊断里看出「这张图最终是从哪条路径拿到的」，方便下一次真机验证确认改动生效（扩展现有字段即可，不需要新造一套事件）。

### 验收

- 新增/扩展单测：模拟客户端未连接 + `raw.communitydragon.org` 返回超时/负缓存命中，断言 `handleChampionAsset` 对 `communitydragon` 源的请求最终能从回落主机拿到图返回 200，而不是 404。
- 对抗变异：把新加的回落调用去掉，上面的断言必须 FAIL。
- 现有 `champions_test.go`（含 R127 相关用例）保持全绿；`TestCommunityDragonImagePathsCoverPluginAndGameAssets` 一类既有断言不得因为改动而回退。
- 真机验收：客户端保持未启动，联网但不要求能连到 `raw.communitydragon.org`（当前网络环境本来就连不上，不用特意模拟断网），刷新总览页，之前缺图的格子应该显示出图（哪怕是换了个源取到的图），导出新一份诊断日志确认 `raw.communitydragon.org` 的失败没有再变成前端的裸 404。

---

## P2　海克斯大乱斗 / 斗魂竞技场英雄详情「本地请求超时，请重试」——一次超时熔断全局，两个模式一起遭殃

### 证据

`hexdata_fallback`（06:10:24Z 起）：
```
{"module":"hero-detail","reason":"hexdata unavailable: Get \"https://hexdata.com.cn/api/hexdata/heroes/89\": context deadline exceeded"}
{"module":"hero-detail","reason":"context deadline exceeded"}
```
`hexdata_request` 同期出现多条 `kind=hero-json`、`status=0`、`wire_ms=8000`（撞满 `hexdata.go:1333`/`:1454` 的 `context.WithTimeout(ctx, 8*time.Second)` 超时预算）。

3 次连续失败后触发熔断（`hexdata_circuit_trip`），failures 从 3 一路到 5，`until` 时间逐次拉长：
```
failures:3 until:14:11:41  （距触发+1分钟）
failures:4 until:14:15:41  （距触发+5分钟）
failures:5 until:14:25:49  （距触发+15分钟）
```
随后出现的 `hexdata_circuit_open` + `hexdata_fallback{reason:"hexdata circuit is open"}` 说明熔断打开期间，请求**根本没有发出去**，直接秒失败。熔断在 06:27:21Z（约首次触发后 17 分钟）才因半开探测成功而恢复（`hexdata_request kind=hero-json status=200`）。

### 根因

`backend/hexdata.go:1014` 的 `load(ctx, kind, id, requestPath, ...)`：熔断状态存在 `state.Circuits[kind]`（`:1039` 前后），**键是数据类型字符串（例如 `"hero-json"`），不含英雄 ID，也不含游戏模式**——`id` 参数只用于组装磁盘缓存键（`cacheKey`，`:1005`），不参与熔断判定。`recordCircuitFailure(kind, ...)`（`:1585`）同理，只按 `kind` 计数失败次数。而「海克斯强化」是海克斯大乱斗与斗魂竞技场共用的同一套游戏机制，两个模式的英雄详情页在需要「该英雄的海克斯强化专属胜率」时，走的是同一个 `kind="hero-json"` 请求。

所以链路是：**某一个英雄**在**某一个模式**下的一次 hero-json 请求连续超时 3 次 → 熔断器按 `kind="hero-json"` 整体打开 → 接下来 1–30 分钟内（`hexdataCircuitBackoff`，`:1622`，按失败次数取 `[1分钟,5分钟,15分钟,30分钟]`）**任何英雄、任意一个模式**的详情请求全部直接返回「熔断已打开」，不会真的去问上游——这正是用户截图里两个模式同时打不开详情的原因，不是各自独立坏掉两次，是一次超时连坐了全部。

补充：熔断状态会持久化跨进程重启（`loadState`/`saveState`，`:786`/`:827`，重启只清理「剩余时间超过 6 小时」的陈旧熔断），用户如果在熔断打开期间重启客户端，大概率**不会**恢复——这个副作用现在没有任何提示，容易被误判成「重启也没用，问题更严重了」。

这次不是 R146/R147 处理过的上游拒绝（403/`page_token_required`，已由 R147 的令牌机制解决），而是纯网络超时；熔断器本身的设计意图（保护一个明显不可用的上游、避免把请求打爆）是合理的，问题在**粒度**和**用户可见性**两处。

### 修复方向

1. 评估是否需要按「模式」（海克斯大乱斗 / 斗魂竞技场）拆开熔断状态键，而不是两个模式共用一把「hero-json」锁——如果两个模式请求的确实是同一份数据（很可能是，因为海克斯强化是共享机制），拆分可能没有实际意义，那就至少在代码注释里写清楚「两个模式共用熔断是有意为之」，避免以后被当成遗漏改掉。
2. 无论是否拆分粒度，前端在「熔断已打开」这个失败原因下，不应该展示和普通网络超时一样的「本地请求超时，请重试」——用户点重试只会立刻再失败一次。按 `CLAUDE.md` 的界面文案红线（只允许错误/失败与降级提示，不允许统计口径类说明），这里合法的做法是把「上游暂时不可用，请稍后再试」和「网络超时，请重试」区分成两种错误文案，不需要暴露熔断的实现细节或倒计时，只要不再诱导用户做一个必然无效的重试。
3. 核对「3 次失败触发熔断」（`hexdataCircuitFailureLimit = 3`，`:36`）配合 8 秒/次的超时预算，是否对正常使用场景（用户短时间内连续点开几个不同英雄）过于敏感——本次日志看是不同英雄（`heroes/89`、`heroes/105`）连续超时触发，不是同一个英雄反复重试触发，所以现有阈值本身未必是问题，这一条只作为可选项记录，不要求本轮一定要改，且改之前要有真实日志支撑。

### 验收

- 新增测试：模拟连续 3 次 `context deadline exceeded`，断言熔断打开后**同一 kind 下不同英雄 ID** 的请求都立即返回「熔断已打开」错误而不发起网络请求（钉死现有的「按 kind 而非按英雄」行为，防止以后无意中改成按英雄粒度却没人注意到跨模式还是共用）。
- 前端：新增一条测试断言「熔断已打开」类错误与普通超时错误对应不同的提示文案（具体文案由 GPT 按红线拟定，不写统计口径类说明）。
- 对抗变异：把错误文案分支去掉、恢复成两种失败共用一条文案，新测试必须 FAIL。
- 真机验收：GPT 复现一次连续 3 次超时（可用本地网络限速或临时指向不可达地址模拟，具体手段由 GPT 决定），确认熔断打开期间两个模式的英雄详情都显示新的「上游暂时不可用」文案而不是「本地请求超时」；熔断窗口过后（或应用重启后，视 1 的结论而定）恢复正常。

---

## P3　收藏页头像「未拥有」被整体灰阶变黑，应该和旗帜一样只压透明度

### 证据

`backend/web/app.css:568-569`（通用规则，头像视图目前吃的就是这一条）：
```css
.skin-card.is-locked { color: color-mix(in oklab, var(--ink) 68%, var(--muted)); border-color: color-mix(in oklab, var(--line) 78%, transparent); }
.skin-card.is-locked .skin-art img.is-loaded, .skin-card.is-locked .skin-art video { opacity: .38; filter: grayscale(1) saturate(.25); }
```
`app.css:1156`（R130 已经给旗帜视图单独覆盖过）：
```css
.facade-grid.is-banners .skin-card.is-locked .skin-art img.is-loaded { opacity: .68; filter: none; }
```
`.facade-grid.is-icons`（头像视图，`:1167` 起）没有任何针对 `.is-locked` 的覆盖规则。

### 根因

R130 把旗帜未拥有的样式从「全灰阶」改成了「只降透明度、保留原色」（R130 工单原文：「未拥有的旗帜只压透明度、不做全灰去色……能看清旗帜本来的配色」），但只加了 `.facade-grid.is-banners` 这一条覆盖，没有给 `.facade-grid.is-icons` 补同一条——头像视图因此一直吃着更早、更重的通用 `.skin-card.is-locked` 规则（`opacity:.38` + `grayscale(1)` + `saturate(.25)` 三个效果叠加），就是截图里那种接近纯黑的效果。这是 R130 的遗漏，不是本来的设计意图。

### 修复方向

在 `app.css:1167`（`.facade-grid.is-icons` 规则块）附近补一条与旗帜对称的覆盖：
```css
.facade-grid.is-icons .skin-card.is-locked .skin-art img.is-loaded { opacity: .68; filter: none; }
```
透明度数值直接照抄旗帜的 `.68`，保持两个视图未拥有态的视觉语言一致（用户原话「改成和旗帜一样就行」）；锁形图标（`.skin-lock`，`:571-573`）两个视图都继续保留，不受影响。

### 验收

- 截图核对：头像视图未拥有条目应显示原始配色、仅轻微变暗，不再是接近纯黑的灰阶效果，与旗帜视图未拥有态观感一致。
- 如果仓库里已有覆盖这段 CSS 规则存在性的测试（搜索 `is-locked`/`facade-grid`），确认新增规则不破坏既有断言；没有的话不需要专门为一条 CSS 规则新增测试。

---

## P4　切换到旗帜视图，新数据到位前网格里残留着头像卡片

### 证据

`backend/web/favorites-facade.js:446` 的 `setView`：
```js
function setView(view) {
    const next = view === "banners" ? "banners" : "icons";
    const changed = state.view !== next;
    state.view = next;
    syncControls();
    if (changed && !state[next]) load(next, false);
    else render();
}
```
`syncControls()` 在发起请求**之前**就已经把网格的 CSS class 切到了 `is-banners`（函数体内的 `el.grid.classList.toggle("is-banners", ...)`）。而 `load()`（`:378`）里，请求发出后只是 `el.grid.setAttribute("aria-busy", "true")`（一个无障碍属性，不影响视觉），**没有清空或替换网格内容**；只有 `fetch` 成功返回、`state[key]` 被赋值之后才会调用 `render()`（`:341`）去 `el.grid.replaceChildren()` 重建。

### 根因

两个视图切换之间存在一个可见的过渡态：网格的**布局 class** 已经变成旗帜专用的 `is-banners`（卡片尺寸比例、网格列宽都按旗帜的竖长图调整），但网格里的**内容**还是上一个视图（头像）渲染出来的卡片，直到 `/api/facade/banners` 请求返回才会被替换掉。网络越慢，这个「旗帜网格样式 + 头像卡片内容」的错位过渡态停留时间越长，用户看到的就是「用头像占位」。

### 修复方向

`setView` 里，`changed && !state[next]` 为真（切到一个还没加载过的视图）时，在调用 `load()` 之前先清空网格并进入统一的加载态（可以直接复用 `load()` 现有的 `aria-busy` 属性，配合 CSS 让「网格为空 + `aria-busy=true`」时显示骨架屏或空白，而不是留着旧内容）。最直接的做法是把 `el.grid.replaceChildren()`（当前只在 `render()` 里调用）提到 `setView` 判断「切到未加载视图」的分支里先执行一次，或者在 `load()` 顶部、真正发起 fetch 之前加一行清空逻辑。

### 验收

- 新增测试：模拟「已加载头像数据 → 调用 `setView('banners')` → 此时（fetch 尚未 resolve）网格应为空或处于明确的加载态，不包含任何头像卡片节点」。
- 对抗变异：去掉新加的清空逻辑，断言必须 FAIL。
- 人工核对：限速网络下来回切换头像/旗帜 tab，观察不到「旗帜网格里显示头像图」的过渡态。

---

## P5　切换「显示未拥有」（或任何筛选/排序）时，看起来所有头像都被重新加载了一遍

### 证据

`backend/web/favorites-facade.js:341` 的 `render()`：
```js
el.grid.setAttribute("aria-busy", "false");
el.grid.replaceChildren();              // 销毁全部现有卡片节点
...
const fragment = document.createDocumentFragment();
while (index < rows.length && performance.now() - started < 7) {
  const item = rows[index++];
  const fields = ...;
  fragment.append(createCard(fields, item));   // 为每一条筛选后仍可见的条目重新创建全新的 <img>
}
el.grid.append(fragment);
```
`el.showUnowned.addEventListener("change", ...)`（`:472`）只是改 `filters.showUnowned` 然后调用 `render()`——不发起任何新的网络请求（目录数据 `state.icons`/`state.banners` 本来就已经在内存里），但每次都会触发上面这段「整体销毁重建」。

`backend/web/image-queue.js:63-65` 自己的注释写明了后果：
```js
// A successful URL is still admitted through the same queue. The cache only
// records that it succeeded recently; it is not evidence that the browser
// cache can satisfy a new element without opening a connection.
if (loadedURLs.has(url) && Date.now() - loadedURLs.get(url) >= 600000) loadedURLs.delete(url);
```
即：哪怕同一个 URL 最近才成功加载过，一个**新的** `<img>` DOM 节点仍然会被重新塞进只有 5 个并发名额（`IMAGE_QUEUE_LIMIT = 5`，远程道 `REMOTE_LANE_LIMIT = 2`，`:5-11`）的准入队列，逐个排队触发 `img.src = url`，配合 `.facade-grid.is-icons .skin-art:has(img:not(.is-loaded))` 的骨架屏动画（`app.css:1175`），呈现出的就是「所有头像又要排队重新加载一次」。

### 根因

`render()` 不做任何新旧列表的差异比较，每次筛选/排序变化都无差别地把**当前视图下所有可见条目**的卡片连同 `<img>` 一起销毁重建，而不是只增删「因为这次筛选变化而新出现/被过滤掉」的那部分条目、保留其余条目原有的 DOM 节点（包括它们已经加载完成的 `<img>`，其 `is-loaded` class 与 `src` 都还在）。对已拥有条目而言，切换「显示未拥有」开关前后它们本该完全不受影响，却也被一起拆了重建。

### 修复方向

给 `render()` 加一层按条目 ID 的 keyed diff：维护一份「当前网格里已渲染的 id → DOM 节点」映射（例如用一个 `Map`），每次 `render()` 时：
- 对新筛选结果里已经存在对应节点的条目，直接复用旧节点（必要时更新排序位置，用 `insertBefore`/`appendChild` 调整顺序，不重新创建 `<img>`）；
- 只对新出现的条目调用 `createCard()` 创建新节点；
- 只对不再出现的条目移除节点。

`chunk()` 的分帧渲染逻辑（避免大目录一次性同步渲染卡顿）可以保留，只是改成分帧处理「需要新增的条目」，而不是分帧处理「全部条目」。

### 验收

- 新增测试：模拟「已渲染 N 张头像卡片（各自标记为已加载）→ 切换 `showUnowned` → 断言筛选前后都可见的条目对应的 DOM 节点是同一个对象引用（没有被销毁重建），只有新增/移除的条目对应节点变化」。
- 对抗变异：把 diff 逻辑去掉、恢复成 `replaceChildren()` 全量重建，新测试必须 FAIL。
- 人工核对：加载数量较多（例如切到「全部头像」）后来回切换「显示未拥有」，已拥有条目的图片不应重新出现骨架屏闪烁；仅新增/移除的条目有加载过程。

---

## 验收总表

| 项 | 判据 |
|---|---|
| P1 | `communitydragon` 源失败时能回落到其他主机取图；对抗变异（去掉回落）必须 FAIL；真机日志确认客户端未启动场景下总览图标不再空缺 |
| P2 | 熔断按 kind 连坐两个模式的行为被测试钉住（防止意外改动）；前端区分「熔断已打开」与「普通超时」两种提示文案；真机复现确认两个模式在熔断期间都提示正确文案 |
| P3 | 头像未拥有态改为 `opacity:.68; filter:none`，与旗帜一致；截图核对 |
| P4 | 切视图时未加载完成前网格不残留旧视图内容；对抗变异必须 FAIL |
| P5 | 筛选/排序变化时未受影响的条目 DOM 节点被复用、不重新加载；对抗变异必须 FAIL |
| 全量 | `go build -o <tmp> ./backend`、`go vet ./...`、`go test ./...`、`node --test backend/web/*.test.cjs desktop/*.test.cjs`，与 R153 基线一致或更好（既有失败不得增加）；改动后版本号递增（当前 0.12.19），更新 `docs/WORKLIST-INDEX.md`
