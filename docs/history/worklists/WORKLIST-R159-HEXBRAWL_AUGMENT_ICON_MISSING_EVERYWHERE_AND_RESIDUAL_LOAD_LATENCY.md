# WORKLIST-R159：海克斯（HexBrawl）增益图标在软件所有位置都不显示——三处渲染路径共用同一个已确认不可达的 `loadCommunityDragonAugments`；`hexdata.com.cn` 已经返回可用图标却被后端整段丢弃

诊断人：Claude（只读：用户提供的诊断日志 `c958cedf-lol-loot-diagnostics-0925-1919.jsonl` + 两张截图 + 读代码核对实现，未改仓库任何文件）。
执行人：GPT。
日期：2026-09-25。
基线：0.12.23（`desktop/package.json`），仓库 HEAD `35788b9f`，工作区含 R144–R158 一路带下来的未提交改动（`git status --short` 111 处改动）。本单所有行号均以当前工作区文件（含未提交改动）为准，执行前请先 `grep` 确认行号未被后续改动移动。
状态：代码与本机自动验证完成，待新诊断日志及 Windows 真机复核；执行详情见 `r159-execution-ledger.md`。
触发：用户在验证完 R157/R158 之后追加两条反馈——
1.（附截图 1：海斗英雄详情「海克斯推荐」页，疾风剑豪 9 张增益卡，胜率/分数/稀有度徽标都正常，但**每一张卡的图标都是同一个通用六边形占位图**）「海斗英雄详情虽然出来了，但是很慢，并且海克斯图标出不来，为什么这么慢，结合日志仔细分析；韩服玩家的图标加载也很慢，结合日志继续深度优化，加载速度没什么变化」；
2.（附截图 2：多场海克斯大乱斗的战绩列表，符文/增益槽位同样是通用六边形占位图）「战绩这边海克斯图标也出不来了，仔细深度分析，软件里面所有地方的海克斯图标都没有了，严重问题，继续上面回答」。

附带日志：`c958cedf-lol-loot-diagnostics-0925-1919.jsonl`（3,355 行，`run_id:"2caee43965f5e76ce62dd8a8"`，覆盖 11:13:32Z–11:17:08Z+，`build_fingerprint:"36f912d80689"`，已确认是 R157/R158 落地之后的构建，与本单基线一致）。建议放进 `docs/r159-validation/c958cedf-lol-loot-diagnostics-0925-1919.jsonl`。

## 先排除的可能性

- **不是 R157 的回归。** 新日志里 `community_dragon_augments_backoff` 事件序列完全正常：`{"result":"started","reason":"timeout","stage":"cherry", ...}`（`log_seq:658`）→ 9 条 `"result":"skipped"`（`skipped_requests` 从 1 递增到 9）→ `{"result":"ended","reason":"retry-window-ended","skipped_requests":9}`（`log_seq:3084`，约 56 秒后，与 45 秒退避窗口 + 下一次请求到达的时间点吻合）。退避机制按设计工作，没有重复超时。
- **不是 R158 的回归。** 本单没有发现任何与「切换玩家 tab 后图标重新加载」相关的新现象；用户这次反馈的是图标**从一开始就没有过**，不是「加载过又消失」。
- **`hexdata.com.cn` 本身完全健康**，不是又一次熔断误伤（R154 P2 的场景）：全部 10 条 `hexdata_hero_json` / `hexdata_request` 事件都是 `status:200`，没有一条 `hexdata_circuit_trip`/`hexdata_circuit_open`。
- 真正在失败的只有一个域名：`raw.communitydragon.org`。`champion_upstream` 里该 host 的 5 条样本全部 `status:0`（无响应），`duration_ms` 为 `3000/4000/4000/12001/4000`——与 R157 引入的 4 秒探测预算（`communityDragonAugmentRequestBudget`）吻合，说明这不是瞬时抖动，而是**持续、稳定的不可达**（与 R154 最初的发现一致）。
- `lcu_discovery` 记录 `{"result":"process-not-found"}`——本机没有运行英雄联盟客户端，本地已认证目录（`p.gameplayAugments`）这条分支这次也完全没有数据。

## 结论摘要

| 项 | 现象 | 根因（一句话） |
|---|---|---|
| P1 | 海斗英雄详情、战绩列表、海克斯图鉴——**软件里所有渲染海克斯增益图标的地方**都显示同一个占位图 | 三处渲染路径背后共用同一个函数 `loadCommunityDragonAugments`（只打 `raw.communitydragon.org`），这个域名对该用户网络持续不可达、且本机客户端未连接，导致这唯一的图标来源整体失效；而 `hexdata.com.cn`（确认健康）的英雄详情响应里其实**已经自带每个增益的图标 URL**（`augmentIconUrl`），后端却从未解析、也没有把它接进图片服务白名单，被整段丢弃 |
| P2 | 即使图标问题不算，海斗英雄详情「还是很慢」 | R157 的退避是「有限度地慢」而不是「不再慢」：只要 `raw.communitydragon.org` 持续不可达，每个约 45–60 秒窗口里**总会有一次**详情请求撞上完整的 4 秒探测预算（本次日志里两条样本 `decorate` 耗时 4266ms/4008ms，对应 `total` 5496ms/4081ms）——这是 R157 设计内的正常代价，但用户感知就是「偶尔明显卡一下」 |
| P3 | 「韩服玩家的图标加载也很慢，加载速度没什么变化」 | 本次日志里**没有找到**属于代码缺陷的证据：`riot_overview_cost`（1.6–2.6 秒）、`opgg_match_tiers_cost`（首次 2.0–2.2 秒、命中缓存后 0–158ms）都落在 Riot/OP.GG 上游请求本身的合理区间，`limiter_queue_ms`（0/1453/2531ms）随耗时一起升高，与「多个概览请求共用同一份 Riot App 级限流预算排队」的模式吻合，不是哪一步在空等或重复请求。这个结论和 R156–R158 都没有关系，需要更明确的复现场景才能判断是否值得继续投入 |

---

## P1　海克斯增益图标在英雄详情 / 战绩 / 图鉴三处全部消失——共用同一个已确认不可达的函数，且 `hexdata.com.cn` 现成的图标 URL 从未被解析

### 证据：三处渲染路径，同一个根因

**现象 1：英雄详情页。** 截图 1 里 9 张增益卡的名称、稀有度、胜率、分数全部正常渲染，只有图标是同一张占位图。日志里 `hexdata_augment_metadata_missing` 事件出现 **2506 次**（全日志里出现次数最多的事件），覆盖 184 个不同的增益 ID，每个 ID 平均出现约 13.6 次（与 `decorateHexdataAugmentsWithCatalog` 每次英雄详情调用 2 次 × 10 次详情加载、且几乎每个增益都查不到目录的规模吻合）。

代码路径：`backend/hexdata.go:3489` `decorateHexdataAugmentsWithCatalog(rows, byID)` 对每一行 `byID[asset.ID]` 查表，查不到就发 `hexdata_augment_metadata_missing` 诊断、`asset.Source`/`asset.Path` 保持空。`byID` 来自 `backend/champions.go:2625` `loadAugmentMetadataCatalog(ctx)`：

```go
func (p *championProvider) loadAugmentMetadataCatalog(ctx context.Context) []gameplayAugment {
    var catalog []gameplayAugment
    if p.gameplayAugments != nil {
        if local, err := p.gameplayAugments(ctx); err == nil {
            catalog = append(catalog, local...)   // 需要本机客户端在线，这次没有
        }
    }
    if remote, err := p.loadCommunityDragonAugments(ctx); err == nil {
        catalog = mergeGameplayAugmentMetadata(catalog, remote)   // 打 raw.communitydragon.org，这次持续失败
    }
    return catalog
}
```

本机客户端未连接（`lcu_discovery: process-not-found`）+ `raw.communitydragon.org` 持续不可达 ⇒ 两条分支都拿不到数据 ⇒ `catalog` 实质为空 ⇒ **每一个**增益的 `Path` 都设不上。

**现象 2：战绩列表。** 截图 2 的符文/增益槽位也是同一个占位图。这条路径完全不经过 `decorateHexdataAugmentsWithCatalog`，走的是 `backend/web/gameplay.js:6815` 的 `augmentIconFigure(id, size)`：

```js
function augmentIconFigure(id, size = "") {
  const augment = (state.perks?.augments || []).find((candidate) => Number(candidate.id) === Number(id));
  ...
  const icon = augment?.iconPath ? assetIcon(augment.iconPath, ...) : state.perks ? iconFigure("augment", id, ...) : pendingCatalogIcon(...);
```

`state.perks.augments` 来自全局目录接口 `GET /api/gameplay/perks`（`backend/gameplay.go:9369` `handleGameplayPerks`）。本机客户端离线时 `cacheKey = "ddragon"`，`client = nil`，随后 `enrichPerkAugments`（`backend/perk_catalog_async.go:11`）异步填充：

```go
if client != nil {
    ... // LCU 分支，这次不适用
}
if client == nil || err != nil {
    augments, err = a.fallbackGameplayAugments(ctx)   // ← 唯一兜底
}
```

而 `fallbackGameplayAugments`（`backend/gameplay.go:9624`）：

```go
func (a *app) fallbackGameplayAugments(ctx context.Context) ([]gameplayAugment, error) {
    loadCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
    defer cancel()
    catalog, err := a.championDataProvider().loadCommunityDragonAugments(loadCtx)
    return catalog, err
}
```

**和英雄详情页打的是同一个 `loadCommunityDragonAugments`。** 该函数失败 ⇒ `enrichPerkAugments` 里 `if err == nil { entry.payload.Augments = augments }` 这一句不会执行 ⇒ `payload.Augments` 永远是空数组 ⇒ 前端 `state.perks.augments` 为空 ⇒ `augmentIconFigure` 对任何 ID 都找不到条目 ⇒ 落到 `iconFigure("augment", id, ...)` 通用占位渲染——这正是截图 2 里所有海克斯大乱斗战绩行看到的六边形占位图。

**现象 3（用户没有截图，但会受同样影响）：海克斯图鉴（`/api/champions/augments`）。** `backend/champions.go:2479` `loadAugments`：

```go
catalog := p.loadAugmentMetadataCatalog(ctx)   // 还是它
byID := gameplayAugmentIndexAll(catalog)
for index := range rows {
    if meta, ok := byID[rows[index].ID]; ok {
        ...
        if path := communityDragonGameAssetPath(meta.IconPath); path != "" {
            rows[index].ImageSource, rows[index].ImagePath = "communitydragon", path
        }
    }
}
```

**同样依赖 `loadAugmentMetadataCatalog`。** 也就是说，海斗英雄详情、全局符文/增益目录（战绩、构筑等所有引用 `state.perks.augments` 的地方）、海克斯图鉴，三条完全独立开发的渲染路径，最终都汇聚到同一个从未做过容灾的共享函数上——这不是三个各自独立的 bug，是一个共享依赖的单点故障，可以完整解释用户「软件里所有地方的海克斯图标都没有了」这句话。

### 关键新发现：`hexdata.com.cn` 的响应里已经带着能用的图标 URL，后端却从来没有解析过

用户附带的截图证明海斗英雄详情页本身能正常打开（`hexdata_hero_json` 全部 `status:200`）。读取仓库现有测试夹具 `backend/testdata/r116/hexdata-hero-157.json`，其中每一条 `augments[]` 记录本来就有：

```json
{
  "augmentId": "2095",
  "augmentName": "掷骰狂人",
  "augmentIconUrl": "/assets/augments/icons/16.18/highroller_small.png",
  "rarity": "棱彩", "hexTier": "hang", ...
}
```

`augmentIconUrl` 是一个 `hexdata.com.cn` 侧的相对路径——和已确认持续不可达的 `raw.communitydragon.org` 完全无关。但：

```
$ grep -rn "augmentIconUrl\|AugmentIconURL" backend/*.go
（零匹配，_test.go 也没有）
```

对应的 Go 结构体 `hexdataAugmentRowV2`（`backend/hexdata.go:392-410`）根本没有这个字段：

```go
type hexdataAugmentRowV2 struct {
    AugmentID           int                      `json:"augmentId"`
    AugmentName         string                   `json:"augmentName"`
    AugmentDescription  string                   `json:"augmentDescription"`
    Rarity              string                   `json:"rarity"`
    ...
    // 没有任何字段映射 augmentIconUrl
}
```

`encoding/json` 反序列化时会静默丢弃 JSON 里存在、结构体里没有字段的键——`augmentIconUrl` 从来没有机会被后端读到，更谈不上用它。

同一个文件里另外两处同样标了 `Source: "hexdata"` 的资产构造，处理方式并不一样：

- **装备**（`hexdataItemMetricRows`，`backend/hexdata.go:2513-2531`）构造时不带 `Path`，但随后 `decorateHexdataItemAssets`（`backend/hexdata.go:2841-2875`）会补一个真正可用的路径：`asset.Path = p.ddragonAssetPath("item", asset.ID)`——**基于 Data Dragon，和 CommunityDragon 完全无关**，装备图标始终工作正常，用户也从未反馈过装备图标缺失。
- **召唤师技能**（`hexdataSpellPairMetricRows`，`backend/hexdata.go:3822-3857`）同样不带 `Path`，靠前端 `spellIconFigure`（`backend/web/gameplay.js:6799`）按 ID 反查 `state.summonerSpells`——这份目录同样来自 Data Dragon 兜底路径，同样不依赖 CommunityDragon。
- **只有增益**（`hexdataAugmentMetricRows`，`backend/hexdata.go:2515-2535`）从始至终没有任何独立于 CommunityDragon 的图标来源——这解释了为什么用户只反馈了海克斯图标缺失，装备/召唤师技能图标一切正常。

### 还需要注意的第二个缺口：就算解析了 `augmentIconUrl`，图片代理路由现在也会直接拒绝它

前端所有资产图片最终都走 `imageURL(source, path)`（`backend/web/champions.js:201`）：

```js
function imageURL(source, path) {
  return source && path ? `/api/champion-asset?source=${...}&path=${...}` : "/image-unavailable.svg";
}
```

`GET /api/champion-asset` 由 `handleChampionAsset`（`backend/champions.go:1020`）处理，路径合法性校验在 `validateChampionAssetPath(source, requestPath)`（`backend/champions.go:1249-1270`）：

```go
switch source {
case "opgg": ...
case "ddragon": ...
case "communitydragon": ...
case "gtimg": ...
default:
    return "", false   // ← "hexdata" 落在这里，直接判定非法
}
```

**这个校验函数目前完全没有 `"hexdata"` 分支。** 注意不要和 `allowedChampionHost`（`backend/champions.go:874-876`，已经包含 `hexdataHost`）搞混——那个白名单是给后端自己去抓 `hexdata.com.cn` 的 API（英雄详情 JSON 等）用的通用出站请求校验，和这条**面向浏览器的图片代理路由**是两套独立的校验逻辑。也就是说：如果只做「解析 `augmentIconUrl` 并把它塞进 `championAsset.Path`、`Source` 设成 `"hexdata"`」，前端拿到的 `<img src>` 会是 `/api/champion-asset?source=hexdata&path=...`，打到后端会被 `validateChampionAssetPath` 直接拒绝（`400 invalid champion asset`），**图标依然出不来**，只是报错原因从「查不到目录」变成了「路由拒绝」。这是执行时容易漏掉的一步，必须一并修。

### 修复方向

1. **解析字段**：给 `hexdataAugmentRowV2`（`backend/hexdata.go:392`）新增 `AugmentIconURL string \`json:"augmentIconUrl"\`` 字段，把上游本来就在发的数据接住，不再静默丢弃。
2. **放行路由**：给 `validateChampionAssetPath`（`backend/champions.go:1249`）新增 `case "hexdata":` 分支，返回 `hexdataHost` + 一个明确的路径前缀白名单（例如 `strings.HasPrefix(requestPath, "/assets/")`，具体前缀以 `augmentIconUrl` 实测样本为准，注意仍要满足前面 `.png/.jpg/.jpeg/.webp` 后缀校验）。**执行前请先用实际网络请求确认 `augmentIconUrl` 这个相对路径确实挂在 `hexdataHost`（`hexdata.com.cn`）本身上，而不是 hexdata 站点用的另一个静态资源子域名**——本环境的出站网络策略无法帮你验证这一步，这是本单唯一没有跑通网络验证的一环，请务必先确认再接线，避免又是一条看似修好、实际仍然 404 的路径。
3. **接入渲染路径**：在 `hexdataAugmentMetricRows`（`backend/hexdata.go:2515`）构造增益 `championAsset` 时，若 `row.AugmentIconURL` 非空，直接把清洗后的路径设进 `Source: "hexdata"`、`Path: <清洗后的 augmentIconUrl>`（可以参考 `communityDragonGameAssetPath`，`backend/champions_structured.go:2437`，写一个同构的 `hexdataGameAssetPath` 做路径合法性清洗，而不是直接信任上游字符串）。这一步单独就能让**英雄详情页**的增益图标立刻可用——因为它是每次 `hexdata_hero_json` 请求自带的数据，不依赖任何额外网络往返，也完全不受 `raw.communitydragon.org` 是否可达影响。
4. **战绩 / 图鉴这两处全局目录路径**，因为它们要的是「任意增益 ID → 图标」的全局映射，而 `augmentIconUrl` 只在某个英雄的详情响应里出现（覆盖的是「对该英雄可用的增益」子集，不是全量），不能直接照搬第 3 步。建议：把 `loadAugmentMetadataCatalog`（`backend/champions.go:2625`，三条渲染路径唯一的共同交汇点）改成第三层来源——除了「本机客户端」「CommunityDragon」，再加一层「由每次真实的 hexdata 英雄详情响应顺手攒起来的、按增益 ID 去重的本地索引（可持久化到磁盘，参考 `champion_cache.go` 现有的缓存基础设施）」。这样只要用户正常使用过应用（看过几个英雄的详情），战绩和图鉴就能逐步覆盖到之前查看过的增益；虽然不能保证冷启动就 100% 覆盖，但已经是在完全不依赖 `raw.communitydragon.org` 的前提下能做到的最好效果。如果执行时能确认 hexdata 一侧存在专门的全量增益列表接口（而不是本单看到的、按英雄详情返回的子集），优先直接用那个接口做全局目录，比攒索引更彻底、更简单。

### 验收

- 断网模拟 `raw.communitydragon.org` 持续不可达（复用 R157 测试里现成的手段）+ 本机客户端未连接，验证：
  - 英雄详情页「海克斯推荐」的每一张卡都显示与上游数据一致的、区分开的图标（不再是清一色的占位图）；
  - 战绩列表里海克斯大乱斗对局的增益/符文槽位图标能正常显示（至少覆盖已经被访问过的英雄对应的增益集合，验证第 4 步的索引路径确有生效）；
  - `hexdata_augment_metadata_missing` 事件数量相对本次基线（2506 次 / 184 个 ID）明显下降。
- 确认 `GET /api/champion-asset?source=hexdata&path=...`（真实的 `augmentIconUrl` 路径）返回 200 和正确的图片内容类型，而不是 400。
- 装备、召唤师技能图标（现有工作路径）在改动后行为不受影响。

---

## P2　英雄详情页仍然偶发明显卡顿——R157 的退避把「必然很慢」变成了「偶尔很慢」，`raw.communitydragon.org` 持续不可达时这个「偶尔」是必然会发生的

### 证据

新日志里 10 条 `mayhem_detail_phases_ms`，`total` 分布为：`5496, 881, 719, 82, 100, 85, 805, 80, 681, 4081`（毫秒）。多数请求已经落在几十到几百毫秒——这正是 R157 生效后的预期效果（对照 R157 诊断时反复出现的 15000ms 级卡死，已经是巨大改善）。但仍有两条明显偏高：

```
{"decorate":4266, "hero_json":1224, ..., "total":5496}   log_seq 918
{"decorate":4008, "hero_json":64,   ..., "total":4081}   log_seq 3343
```

对照 `community_dragon_augments_backoff` 的时间线：`log_seq 918` 发生在 `started`（`log_seq 658`）之后、退避正式建立**之前**的窗口内；`log_seq 3343` 发生在 `ended`（`log_seq 3084`，退避窗口结束、允许再探测一次）**之后**。两次都恰好卡在 R157 引入的 `communityDragonAugmentRequestBudget = 4 * time.Second` 探测预算上——不是新 bug，是设计内「每个退避周期允许一次真实探测」的必然代价。

### 根因

只要 `raw.communitydragon.org` 对该用户网络是**持续性**（而非偶发抖动）不可达，R157 的 45 秒退避窗口就会不断循环：退避到期 → 下一次英雄详情请求撞上一次完整的 4 秒探测 → 建立新的退避 → 45 秒后再来一次。用户只要恰好在这个探测窗口内打开英雄详情页，就会体感到明显的卡顿——这与用户反馈「海斗英雄详情虽然出来了，但是很慢」完全吻合，是 R157 已知设计权衡下的真实残留成本，不是 R157 没生效。

### 修复方向

以 P1 为前提：一旦英雄详情页的增益图标改为优先使用 `hexdata.com.cn` 自带的 `augmentIconUrl`（P1 第 3 步），`decorate` 阶段就**不再需要等待** `loadCommunityDragonAugments` 的结果才能返回图标——CommunityDragon 目录此时降级为「补充稀有度/描述等次要字段」的锦上添花数据源，而不是图标能否显示的唯一依据。建议：海斗详情装配流程里，涉及增益的 `decorate` 步骤改为「先用 hexdata 自带数据渲染完整可用的响应，`loadCommunityDragonAugments` 的探测/合并放到不阻塞响应返回的路径上（尽力而为，超时或失败也不影响已经返回的结果）」。这样能从根上消掉这个周期性的 4 秒卡顿，而不只是让退避窗口更长/更短这种参数调整。

### 验收

- P1 落地后，复现「`raw.communitydragon.org` 持续不可达」场景，确认 `mayhem_detail_phases_ms.total` 不再出现随退避周期反复出现的 4000ms+ 尖峰。
- 确认这一步改动不影响 P1 里「稀有度/描述用 CommunityDragon 目录补充」这部分现有行为——只是不再让它阻塞响应。

---

## P3　「韩服玩家的图标加载也很慢，加载速度没什么变化」——本次日志未发现代码缺陷，观察到的耗时符合 Riot/OP.GG 上游请求 + 共享限流排队的正常区间

### 证据

`riot_overview_cost` 4 条样本：

```
duration_ms:1714  limiter_queue_ms:0     matches_from_network:10
duration_ms:2570  limiter_queue_ms:1453  matches_from_network:20
duration_ms:1613  limiter_queue_ms:0     matches_from_network:10
duration_ms:2610  limiter_queue_ms:2531  matches_from_network:20
```

`opgg_match_tiers_cost`：首次真实拉取 2021–2217ms（`cache_hit:false`），同一批次的后续命中缓存 0–158ms（`cache_hit:true`）——缓存本身工作正常，不是重复发起网络请求。

`riot_rate_policy` 记录该 App Key 的限流策略为 `{"app":{"1":20,"120":100}}`（每秒 20 次、每 120 秒 100 次，全进程共享，不是按玩家或按 tab 单独分配）。观察到 `limiter_queue_ms` 恰好在 `matches_from_network` 更多（20 条）的两次请求里同时升高到 1453ms/2531ms——与「多个概览请求在短时间内一起抢占同一份 App 级限流预算，需要排队」的模式吻合，而不是某一步在空转或重复发起请求。

### 结论

本次日志里没有找到能归因为代码缺陷的证据：耗时集中在 Riot/OP.GG 上游请求本身（1.6–2.6 秒是这类批量拉取比赛详情的正常量级），排队耗时的出现时机和请求量大小同步，符合限流器按设计工作的表现。R156/R157/R158 都没有承诺过要改变整体首次加载速度（R158 只解决「重复打开同一玩家 tab 时图标又重新加载一遍」这一个具体症状），这次的「加载速度没什么变化」很可能是因为用户对比的基准就是这个从未被承诺修复过的首次加载耗时。

### 修复方向

在没有更具体的复现证据之前，不建议直接调大并发数或限流预算——`detail_concurrency:8` 和 App Key 的限流策略是否已经打满，需要先确认这份 App Key 实际获批的限流等级（Riot 开发者后台）是否比当前配置的 `{"1":20,"120":100}` 更宽松；如果是，调大并发/限流预算才是有意义的优化，否则只会更快触发 429。同时建议向用户确认：本次日志记录期间是否同时打开了多个玩家概览 tab（R158 刚支持的多 tab 场景）——如果是，`limiter_queue_ms` 的升高就是多个 tab 共享同一份限流预算的正常排队，而不是单个玩家概览本身变慢了，这会显著改变后续要不要投入优化的判断。

### 验收

这一项本单不给出可直接验收的代码改动——建议先由 GPT/用户确认上面两个前提（App Key 实际限流等级、复现时是否多 tab 并发），再决定是否值得单独立一个后续工单。

---

## 结论

用户反馈「海克斯图标在软件所有地方都没有了」是真实、可复现、有明确根因的问题：海斗英雄详情、战绩列表、海克斯图鉴三条完全独立的渲染路径，最终都依赖同一个从未做过容灾的共享函数 `loadCommunityDragonAugments`（只打 `raw.communitydragon.org`），这个域名对该用户网络是持续性（不是偶发）不可达，本机客户端又没有连接，导致这唯一的图标来源整体失效。同时确认了一个此前完全没被利用的机会：`hexdata.com.cn`（全程健康）的英雄详情响应里其实已经自带每个增益的可用图标 URL（`augmentIconUrl`），但后端反序列化时直接把这个字段丢弃了；即使补上这个字段，图片代理路由 `validateChampionAssetPath` 目前也会直接拒绝 `source=hexdata` 的请求，需要一并放行——这是本单诊断出的、执行时最容易漏掉的一步。R157 的退避机制本身工作正常，但只要 `raw.communitydragon.org` 持续不可达，它「每个退避周期允许一次真实探测」的设计就会周期性地造成几秒钟的可感知卡顿，这一层的根治同样依赖于让图标显示不再等待这次探测的结果。至于「韩服玩家图标加载慢、优化没有效果」，本次日志里的耗时数字符合 Riot/OP.GG 上游请求与限流排队的正常区间，未发现代码缺陷，建议先确认 App Key 实际限流等级和是否多 tab 并发复现，再决定是否值得单独立项。
