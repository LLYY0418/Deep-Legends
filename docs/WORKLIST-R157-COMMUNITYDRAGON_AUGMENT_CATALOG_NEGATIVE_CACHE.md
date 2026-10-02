# WORKLIST-R157：海斗 / 斗魂英雄详情仍出不来——`loadCommunityDragonAugments` 无失败退避，`raw.communitydragon.org` 一抖动就拖垮整条详情链路

诊断人：Claude（只读：用户提供的诊断日志 `372f2c9a-lol-loot-diagnostics-0925-1724.jsonl` + 读代码核对实现，未改仓库任何文件）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.21（`desktop/package.json`），仓库 HEAD `35788b9f`，工作区含 R144–R156 一路带下来的未提交改动。本单所有行号均以当前工作区文件（含未提交改动）为准，执行前请先 `grep` 确认行号未被后续改动移动。
状态：代码与本机自动验证完成，待真实网络及 Windows 真机复核；执行详情见 `r157-execution-ledger.md`。
触发：用户在 R154（已验证：熔断连坐问题已修复）之后追加反馈——「海斗和斗魂的英雄详情还是出不来，根据日志仔细分析」，并附了一份新诊断日志 `372f2c9a-lol-loot-diagnostics-0925-1724.jsonl`（2,153 行，`run_id=96ce4d31ebc6ebdb2535a777`，覆盖 09:16:30Z–09:24:09Z，`app_start.version:"0.12.21"`、`build_fingerprint:"70a0bcd47e6e"`，确认是 R156 之后的构建）。

建议把该诊断日志放进 `docs/r157-validation/372f2c9a-lol-loot-diagnostics-0925-1724.jsonl`（沿用既有目录命名习惯），本单引用的字段全部来自这份原始日志。

## 先排除的可能性

R154 P2 修的是 `hexdata.com.cn` 熔断器「按数据类型而非按英雄/模式生效」导致的连坐失败。这份新日志里：
- `hexdata_circuit_trip` / `hexdata_circuit_open` 事件：**0 条**；
- 全部 11 条 `hexdata_hero_json` 事件、以及 `hexdata_request`（`kind:"hero-json"`）事件，对英雄 80/102/45/157/887/4 全部 `status:200`。

即 R154 P2 的修复本身没有回归、熔断器没有再次误伤——这次是一个新的、独立的根因。

## 结论摘要

| 项 | 现象 | 根因（一句话） |
|---|---|---|
| P1 | 海斗（HexBrawl）/ 斗魂（Arena）英雄详情经常打不开或要等很久 | 装饰增益元数据用的 `loadCommunityDragonAugments`（打 `raw.communitydragon.org`）失败后**不写任何负缓存/退避**，只要该域名不稳定，之后**每一次**英雄详情请求都要重新完整地等一遍网络超时 |
| P2 | 同一次英雄详情请求里，这个失败还要再等一遍 | 海斗的 `loadMayhemDetail` 和斗魂的 `loadDetailOnce`，在各自的失败兜底分支里都**各自独立地把 `loadCommunityDragonAugments` 调用了两次**，失败等待时间直接翻倍 |
| P3 | `mayhem_detail_phases_ms.total` 大量卡在 15000–15013ms 这个非常精确的数字上 | `/api/gameplay/recommendations`（直播推荐）给整条 `loadDetail` 调用链套了 15 秒硬超时，而海斗详情里正好卡在这条慢路径上，一撞就是被 context 取消，不是巧合 |

---

## P1　`loadCommunityDragonAugments` 失败后没有负缓存/退避，`raw.communitydragon.org` 一抖，之后每次详情请求都要重新等一遍超时

### 证据

新日志里 `champion_upstream` 事件按 host 统计（175 条）：

```
raw.communitydragon.org  status:200  ×34
raw.communitydragon.org  status:0    ×27   ← 失败
lol-api-champion.op.gg   status:200  ×48
op.gg                    status:200  ×39
ddragon.leagueoflegends.com status:200 ×13
ddragon.leagueoflegends.com status:0  ×11
api.your.gg              status:200  ×3
```

`raw.communitydragon.org` 的失败样本（`duration_ms`）：`10456`、`10462`、`0`、`12000`、`2904`、`14710`——耗时集中在 10–15 秒量级，正好卡在 `backend/champions.go:688` 那条 HTTP client 的 `Timeout: 12 * time.Second` 附近（`14710ms` 略高于单次超时，与「先等一次超时、紧接着又重试一次」的模式吻合，见 P2）。

同一份日志里 `mayhem_detail_phases_ms`（海斗详情耗时分解，共 16 条）大量样本的 `decorate` 阶段占了几乎全部耗时：

```
{"decorate":14140,"hero_json":447,"insights":744,"meta":121,"postmatch":243,"rsc":2283,"total":15007}
{"decorate":14144,...,"total":15007}
{"decorate":14924,"hero_json":67,...,"total":15001}
{"decorate":14734,"hero_json":269,...,"total":15011}
```

`hero_json`（打 `hexdata.com.cn` 拿英雄数据本体）只花几百毫秒，真正拖慢整个请求的是 `decorate` 这一步——这正是装饰增益/道具元数据的阶段。

代码路径：
- `backend/hexdata.go:3706`（第一次 decorate，RSC 结果到达前）与 `:3760`（第二次 decorate，RSC 合并后）里都调用 `p.decorateHexdataAugments(ctx, response.RecommendedAugments)`（海斗）；
- `decorateHexdataAugments`（`backend/hexdata.go:3485-3512`）先 `catalog := p.loadAugmentMetadataCatalog(ctx)` 再逐行 map 查找——真正的网络调用只在 `loadAugmentMetadataCatalog` 里；
- `loadAugmentMetadataCatalog`（`backend/champions.go:2624-2635`）：本地 `p.gameplayAugments(ctx)` 之外，`if remote, err := p.loadCommunityDragonAugments(ctx); err == nil { catalog = mergeGameplayAugmentMetadata(...) }`——这就是打 `raw.communitydragon.org` 的那一跳；
- `loadCommunityDragonAugments`（`backend/champions_structured.go:1846-1897`）依次发两个请求：`/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/cherry-augments.json`，成功后再发 `/latest/cdragon/arena/zh_cn.json`；第一个请求失败就直接返回 error，不会再发第二个。

关键问题在缓存层。`p.fetch(ctx, communityDragonHost, ...)` 内部经 `championDataCache.loadWithResultTTL`（`backend/champion_cache.go:108-211`）读写缓存，`championCachePolicy`（`champion_cache.go:547-579`）给 `communityDragonHost` 配的是 `24h TTL / 7×24h staleFor`——这套 TTL **只在拿到成功响应时才会写入**：

```go
data, err := loader(ctx)
if err == nil && len(data) > 0 {
    ...
    c.storeMemoryLocked(key, entry)   // 只有成功分支会写缓存
    ...
    return ...
}
upstreamErr := err
...
if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) &&
    len(stale.Data) > 0 && now.Before(stale.StaleUntil) {
    // 只有「之前成功过、还在 staleFor 窗口内」才会退回旧数据
    ...
}
c.mu.Lock()
flight.data, flight.fetchedAt, flight.state, flight.upstreamErr, flight.err = ...
delete(c.flights, key)   // 失败也直接从 flight 表里删掉，什么都没留下
close(flight.done)
c.mu.Unlock()
return championCacheLoadResult{...}, err
```

也就是说：**失败分支既不写内存负缓存，也没有主机级退避**（不同于 R154 P1 已经给图片资源装好的 `assetHostBlocked` / `observeAssetHostResult`，`backend/asset_cache.go:127-160`——那一套只管 `/lol-game-data/assets/...` 这类图片抓取，不覆盖 `championDataCache` 这条 JSON 数据缓存）。只要这个进程还没有对 `raw.communitydragon.org` 成功过一次（例如刚启动、或该域名当前正好抽风），**接下来每一次**海斗/斗魂英雄详情请求，都会重新完整地等一遍 10–24 秒的网络超时，而不是像镜像图片那样第一次失败后就快速退避。

### 根因

`loadCommunityDragonAugments` 依赖的两跳网络请求走的是普通的「成功才缓存」策略，缺少失败退避/负缓存，导致上游一旦抖动就没有任何缓冲——每次详情请求都要重新交一次「10–15 秒超时税」，用户看到的就是海斗/斗魂详情页长时间转圈、超时报错或者压根加载不出来。

### 修复方向

1. 给 `championDataCache.loadWithResultTTL` 的失败分支补一条短 TTL 的负缓存（例如 30–90 秒可调），或者比照 `assetHostBlocked` 的连续失败计数 + 退避窗口思路，在 `championProvider` 层给 `communityDragonHost` 这类主机加一个「最近确认失败，短期内快速返回错误，不再实际发请求」的机制——注意退避判据要和 R154 P1 一致：只有超时/无响应才计入退避，明确的 4xx/5xx 或成功都应立刻清零退避计数，不能把「这次没有增益数据」和「主机连不上」混为一谈。
2. 退避期间 `loadCommunityDragonAugments` 应该快速返回错误（或直接退回本地 `gameplayAugments` 基础目录），而不是每次都重新走一遍两跳网络请求，这样海斗的 `decorateHexdataAugments` 和斗魂的两处调用点都能在毫秒级完成失败判定，不再拖累 `decorate` 阶段耗时。
3. 补一条诊断事件（如 `community_dragon_augments_backoff`），记录退避开始/结束时间和期间被短路的请求次数，方便后续用诊断日志验证退避是否生效，避免重蹈 R154 P1 图片熔断那种「有没有生效全靠肉眼猜」的覆辙。

### 验收

- 构造 `raw.communitydragon.org` 连续超时的场景：验证第 2 次及以后的海斗/斗魂详情请求不再等待完整的网络超时，`decorate` 阶段耗时应从秒级降到毫秒级。
- 验证主机恢复正常响应（成功或明确的 4xx/5xx）后退避立即解除，不会把一次性抖动误判成长期故障。
- 新增/更新单测覆盖：失败退避的建立、期间的快速失败、超时窗口结束后的重新探测、以及退避不应影响其他 host（如 `hexdata.com.cn`、`ddragon.leagueoflegends.com`）。

---

## P2　同一次详情请求里，海斗和斗魂都会把 `loadCommunityDragonAugments` 独立调用两次，失败等待时间直接翻倍

### 证据

**海斗（HexBrawl）**——`backend/hexdata.go` 内 `loadMayhemDetail` 单次调用中：
- 第一次 decorate（RSC 结果到达前，`hexdata.go:3691`）：`p.decorateHexdataAugments(ctx, response.RecommendedAugments)`；
- 第二次 decorate（RSC 合并后，`hexdata.go:3727`）：再次 `p.decorateHexdataAugments(ctx, response.RecommendedAugments)`。

两次调用之间没有共享同一份已加载的目录，各自独立触发 `loadAugmentMetadataCatalog` → `loadCommunityDragonAugments`。

**斗魂（Arena）**——`backend/champions_structured.go` 内 `loadDetailOnce` 的 `spec.HasAugments` 分支（约 1358-1403 行）：

```go
} else {
    augmentCatalog, err := p.loadCommunityDragonAugments(ctx)   // 第一次，1371 行
    if err != nil || len(augmentCatalog) == 0 {
        augmentFallbackReason = "catalog-failed"
    } else { ... }
}
...
if augmentFallbackReason == "" {
    ...
} else {
    response.ArenaAugmentGroups = p.structuredArenaAugmentGroups(ctx, payload.Data.AugmentGroup)  // 内部第二次，见下
    ...
}
```

`structuredArenaAugmentGroups`（`champions_structured.go:1799`）开头就是 `catalog, err := p.loadCommunityDragonAugments(ctx)`——即第一次调用失败（`err != nil` → `augmentFallbackReason = "catalog-failed"`）之后，兜底分支会**再次**调用同一个会失败的函数。

两个模式的失败兜底逻辑是对称的同一个 bug：第一次失败之后，代码没有把「这次已经确认拿不到目录」这个结果带下去，而是让兜底路径重新触发一遍同样会失败的网络调用。

### 根因

`loadMayhemDetail`（海斗）和 `loadDetailOnce`（斗魂）都没有在单次请求的作用域内缓存/复用 `loadCommunityDragonAugments` 的调用结果，导致上游失败时，同一个请求要连续等两遍超时。这也是新日志里能观察到 `14710ms` 这类超过单次 12 秒 client timeout 的样本的直接原因之一（先等一次超时，紧接着又发起第二次）。

### 修复方向

在 `loadMayhemDetail` 和 `loadDetailOnce` 各自的函数作用域内，把 `loadCommunityDragonAugments(ctx)` 的调用结果（含错误）缓存一次并在同一请求内的后续调用点直接复用，而不是重新发起网络请求。海斗可以在 `loadMayhemDetail` 顶部先加载一次目录传给两处 `decorateHexdataAugments`；斗魂可以让 `structuredArenaAugmentGroups` 接受一个可选的已加载目录参数，第一次已经失败时直接跳过重复请求、直接走本地兜底。

这一条和 P1 是互补关系：P1 解决「跨请求」的重复超时（进程级退避），P2 解决「同一请求内」的重复超时（请求级复用）；两者都做，才能把最坏情况从「4 次 12 秒超时」（海斗 2 次 + 斗魂视情况）降到「1 次退避判定，几乎零等待」。

### 验收

- 构造 `loadCommunityDragonAugments` 返回错误的场景，验证单次 `loadMayhemDetail` / `loadDetailOnce` 调用只触发一次底层网络请求（结合 P1 的退避，理想情况下退避生效后甚至是 0 次）。
- 现有的两个 decorate 阶段/两处 Arena 调用点的功能行为（合并结果、兜底措辞、诊断事件字段）保持不变，只是不再重复发起网络请求。

---

## P3　`mayhem_detail_phases_ms.total` 精确卡在 15000–15013ms——直播推荐入口给海斗详情套了硬超时，撞车不是巧合

### 证据

`loadMayhemDetail` 会被两条不同的入口调用：
- `backend/champions.go:562`：`GET /api/champions/detail` → `handleChampionDetail`（`champions.go:952-969`）→ `a.championDataProvider().loadDetail(r.Context(), ...)`——这条路径**没有**自己的超时包装，直接用 `r.Context()`；
- `backend/main.go:524`：`GET /api/gameplay/recommendations` → `handleGameplayRecommendations`（`backend/gameplay.go:4854`）→ 第 4975 行 `ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)`，随后同样调用 `provider.loadDetail(ctx, ...)`。

前端「英雄详情」页（`backend/web/champions.js:523/623/831/928`）走的是前一条、无超时的 `/api/champions/detail`；后一条 `/api/gameplay/recommendations`（`backend/web/gameplay.js:4882`，客户端另有 20 秒 abort）是游戏内/直播场景下的自动推荐入口，两条路径共用同一套 `loadDetail → loadMayhemDetail` 底层实现。

新日志里 `mayhem_detail_phases_ms` 的 `total` 大量集中在 15000–15013ms，且横跨多个不同英雄（说明不是某个英雄特有的巧合）——这与 `handleGameplayRecommendations` 的 15 秒硬超时高度吻合：当 P1/P2 描述的 `decorate` 阶段卡住 10-15 秒时，如果这次调用恰好来自 `/api/gameplay/recommendations`，15 秒硬超时会直接 cancel 掉整条 `ctx`，`loadMayhemDetail` 的 `defer` 里 `time.Since(started)` 记录下来的自然就是「几乎精确 15000ms 出头一点点（多出来的几毫秒是取消后清理/写诊断日志的耗时）」。

而对没有硬超时保护的 `/api/champions/detail`（用户手动点开的「英雄详情」页正是走这条），P1/P2 描述的同一个慢路径不会在 15 秒被打断，而是会一直等到 12-24 秒的底层 HTTP client 超时或前端 35 秒（`backend/web/champions.js:240`）abort 才失败——这解释了用户看到的「等很久也好、直接报错也好，总之详情就是出不来」这个更宽泛的现象；日志里精确卡在 15 秒的样本更可能来自后台自动触发的直播推荐请求，而不是用户手动点开详情页的那一次。

### 根因

两条入口共享同一套慢速详情装配逻辑，其中一条（直播推荐）给这套逻辑套了 15 秒硬超时。只要 P1（跨请求负缓存/退避）落地，`decorate` 阶段就不会再稳定卡在 10-15 秒，这个 15 秒撞车现象和用户手动打开详情页遇到的更长等待/超时都会自然消失，无需单独改这个超时值本身。

### 修复方向

以 P1/P2 的修复为主；P1/P2 落地后建议用新一轮真实网络抖动场景复核一次这三个耗时分解字段（`decorate`/`hero_json`/`rsc`/`total`），确认 `decorate` 阶段回落到毫秒级、`total` 不再出现 15000ms 附近的硬性聚集。如果复核后仍有请求撞上 15 秒硬超时（例如退避机制刚建立、还没来得及生效的极短窗口），再考虑是否要让 `handleGameplayRecommendations` 在超时后返回「已有部分数据+提示增益信息不完整」而不是整体报错，这个属于锦上添花，不阻塞本单的主修复。

### 验收

- P1/P2 落地并通过验收后，用同样的「`raw.communitydragon.org` 连续超时」场景重跑一次，确认 `mayhem_detail_phases_ms.total` 不再出现 15000ms 附近的异常聚集。
- 确认 `/api/champions/detail`（用户手动打开英雄详情的入口）在同样的网络抖动场景下能在合理时间内（毫秒到个位数秒级）返回结果或明确的降级提示，而不是长时间卡住后报「本地请求超时，请重试」。

---

## 结论

用户反馈「海斗和斗魂的英雄详情还是出不来」是一个新的、独立的根因，**不是 R154 P2 熔断器问题的回归**（本次日志确认熔断器完全没有触发）。真正的问题是：给海斗/斗魂详情页装饰增益元数据用的 `loadCommunityDragonAugments`（打 `raw.communitydragon.org`）失败后没有任何负缓存或主机级退避，且海斗、斗魂各自的详情装配逻辑在失败兜底时都会把这同一个慢请求在单次调用里再打一遍——两个问题叠加，导致只要这个域名当前不稳定（新日志里 61 次请求 27 次失败），海斗和斗魂的英雄详情就会稳定地又慢又容易失败，这也解释了为什么用户会同时看到两个模式都受影响：它们背后依赖的是同一个从未被加固过的共享函数。
