# R116-A：Hexdata 数据源由 HTML 抓取替换为 JSON API（含全量聚合文件）

**目标：**
1. 用 `/api/hexdata/heroes/{id}` 替换现有的 `/hero/{id}-{slug}` HTML 抓取链路，并删除因此变得多余的 per-augment 描述扇出与装备名称匹配。
2. 接入 `/api/hexdata/postmatch`（89 KB 全量 22 项指标）与 `/api/hexdata/hextech-insights`（372 KB 全量 tier/topItems/topAugments）两个目录型聚合文件，按 buildID 缓存。
3. 修复 `hexdata-` 前缀文件永久豁免磁盘预算导致的静默增长（新增按 buildID 的回收逻辑，而不是移除既有保护）。

**背景：** `docs/r116-proposal-feasibility-review.md` 第 1 节实测证实：`/api/hexdata/heroes/{id}` 是现有 HTML 页面的严格超集（`augmentDescription` 126/126 满覆盖、`items[]` 自带真实 `itemId`），把它当作「新增接口」而不是「替换数据源」会让 `hexdata.go:1702-1824` 那套 per-augment 扇出继续跑，白白多付 97.6% 的请求。同一份评审第 4 节量化了 `hexdata-` 前缀绕过磁盘预算的问题（`champion_cache.go:447-450`），推算年增量 6.64 GB。本工单是 R116 系列里价值最高、风险也最集中的一张，建议排在最前，其他工单（B/D/E）都要读它产出的数据结构。

**版本：** 完成后 `desktop/package.json` 升到 0.12.8。

---

## P0：客户端基础设施——Referer、路径白名单、新 kind 校验、缓存 key

### 现状证据

- `hexdata.go:688-695`：出站请求只设置 `Accept` / `Accept-Language` / `User-Agent`，**没有 `Referer`**。实测（2026-09-20）：不带 `Referer: https://hexdata.com.cn/` 请求 `/api/hexdata/heroes/157` 返回 `403 {"error":"data_not_public","message":"此入口不提供批量数据。请停止未经许可的自动化访问。"}`；带上则 `200`。
- `hexdata.go:449-451` `allowedPath` 只放行 `hexdataAnswerPath`（`/api/hexdata/answer-cards`）、`/heroes`、`/augments`、`/augment-rarity` 和两条 HTML 详情页正则。`/api/hexdata/heroes/157` 这类路径会在请求前被 `"hexdata request path rejected"` 拒绝。
- `hexdata.go:366-426` `inspectHexdataPayload` 的 switch **有 `default` 分支**（`:418-423`），但它是把数据丢给 `parseHexdataDocument`（HTML 解析器）——对 JSON payload 一定会解析失败并报错。也就是说，**忘记给新 kind 加 case 不会静默通过，而是每次都直接失败**（比"零校验"更安全，但仍必须补上正确的 case，否则新链路永远拿不到数据）。
- `hexdata.go:453-460` `cacheKey` 用 `{buildID}|{kind}|{id}` 拼接。**JSON 响应体本身不含 `buildId` 字段**（实测 `/api/hexdata/heroes/157` 顶层只有 `items/trios/augments/weakAgainst/strongAgainst/teammateSynergies/terminalItemTrios/summonerSpellPairs` 八个数组，没有 citation/buildId）。现有的 `hexdataCitationComplete` 校验（`hexdata.go:1935-1938`）依赖 HTML 页面内嵌的 JSON-LD `Dataset`（`:1170`），JSON API 没有这个东西。**必须改用 `/api/hexdata/meta` 单独维护 buildId 快照**，实测该端点已含 `buildId`/`reportPatch`/`reportDate`/`heroCount`/`augmentCount` 等全部 citation 字段。

### 实现要求

1. **加 Referer**：在 `hexdata.go:693-695` 附近补一行 `request.Header.Set("Referer", "https://"+hexdataHost+"/")`。
2. **扩路径白名单**（`hexdata.go:449-451`），新增：
   ```go
   hexdataHeroJSONPathPattern = regexp.MustCompile(`^/api/hexdata/heroes/([0-9]+)$`)
   ```
   并在 `allowedPath` 里加 `path == "/api/hexdata/postmatch" || path == "/api/hexdata/hextech-insights" || path == "/api/hexdata/meta" || hexdataHeroJSONPathPattern.MatchString(path)`。
3. **`/api/hexdata/meta` 作为新 kind `"meta"` 接入**，缓存策略与 `"answer"` 一致（12h 软 TTL，`hexdataBuildSoftTTL`），产出一个 `hexdataMetaSnapshot{BuildID, ReportPatch, ReportDate, HeroCount, AugmentCount}`。所有走 JSON API 的响应（hero-json / postmatch / hextech-insights）的 citation 都从这个快照取，**不要**尝试从各自的 payload 里现掰 citation。`CanonicalURL` 仍指向对用户可读的 HTML 页面（`https://hexdata.com.cn/hero/{id}-{slug}` 或 `https://hexdata.com.cn/hero-analysis` 之类），不要直接链到 JSON 端点——这是第 6.2 条硬性要求（外链给用户看的页面，不是给爬虫看的接口）。
4. **`inspectHexdataPayload` 新增三个 case**：`"hero-json"`、`"postmatch"`、`"hextech-insights"`（分别对应 P1/P2）。每个 case 要做真实的结构校验（不是 `return shape, nil` 空转）：
   - `"hero-json"`：`json.Unmarshal` 后检查 `items`/`augments`/`trios` 三个数组均非空（英雄冷门到没有出装数据是可能的，但 `augments` 为空基本等于上游改了字段名，应报错）；`shape.Rows` 记 `len(items)`。
   - `"postmatch"`：顶层是 `map[string]object`（key 是英雄 ID 字符串），检查条目数在 150~200 之间（173 英雄±正常波动），任取一条校验含 `kda`/`avgDamage` 字段。
   - `"hextech-insights"`：检查 `heroes` 数组长度在 150~200 之间、`augments` 数组长度在 190~230 之间（211 附近）。
   - 三个 case 都要在校验失败时用 `firstError` 包出明确错误信息，方案的" buildId/schema 变了要能被发现"这条红线靠这三处校验兑现。
5. **新增 `promote()` 调用**：仿照 `hexdata.go:1941` 的写法，三个新 kind 的成功路径都要调用 `p.hexdata.promote(kind, id, buildID, data, fetchedAt)`，否则会重复第 4.6 节指出的"忘记 promote 就永远不落盘、每 12 小时回源一次"的坑。

### 验证判据

1. 单测覆盖：不带 Referer 的 mock 服务器返回 403 时，`load()` 报错且**不计入**熔断失败计数升级到下一档（因为带 Referer 后应该 200，用真实 Referer 请求 mock 通过——用两条测试分别验证「有 Referer 通」「无 Referer 拒」，不是验证熔断行为本身）。
2. `allowedPath("/api/hexdata/heroes/157")`、`allowedPath("/api/hexdata/postmatch")`、`allowedPath("/api/hexdata/hextech-insights")`、`allowedPath("/api/hexdata/meta")` 全部返回 `true`；`allowedPath("/api/hexdata/heroes")`（无 ID，即被上游 403 拦截的列表接口）返回 `false`——**不放行列表接口**，这是第 6.1 条硬性要求。
3. 故意构造一份缺失 `augments` 字段的假 `hero-json` payload，`inspectHexdataPayload("hero-json", ...)` 必须返回非 nil error（证明第 4 条的校验不是空转）。
4. `go test ./backend -run Hexdata` 全绿，且新增测试数 ≥ 8（三个新 kind 各至少 2 条正/负样例 + Referer 测试 2 条）。

### 对抗变异

- 故意删掉 `"hero-json"` case 的 `augments` 空值检查，构造一份 `augments: []` 的 payload，确认测试会失败（证明护栏不是同义反复）。
- 故意让新 kind 忘记调用 `promote()`，确认对应的落盘测试能检测到「12 小时后强制回源」的回归。

---

## P1：替换英雄详情页数据源 + 删除多余链路

### 现状证据

- `loadMayhemDetail`（`hexdata.go:1910-1990`）当前流程：`p.hexdata.load(ctx, "hero", id, "/hero/{id}-{slug}", "text/html...", false)` → `parseHexdataHeroDetail`（正则抓取 WinRate/Games/Tier/Augments/Items，**没有 item ID，只有中文名**）→ `p.decorateHexdataItems`（`:1619-1648`，按中文名反查 CommunityDragon 目录，埋了 `hexdata_item_match` 诊断数漏配率）→ `p.decorateHexdataAugments`（`:1650-1678`，按 augment ID 从 CommunityDragon 目录补图标/rarity/name，**这部分要保留**，与描述来源无关）→ `p.hydrateMayhemAugmentCopy`（`:1702-1740`）→ `p.mayhemAugmentCopy`（`:1740-1807`，24h memo）→ `p.fetchMayhemAugmentCopy`（`:1807-1824`，per-augment 请求 `/augment/{id}-{slug}`，并发 3 但被全局节流串行化）。
- 实测：`augments[]` 数组 126/126 条都带 `augmentDescription` 字段（满覆盖），`items[]` 数组每条带真实 `itemId`。**这意味着上面链路里"反查中文名"和"逐条拉描述"两步完全可以删除**。
- 梯度徽章/总榜位次现状是本地算的（`hexdata.go:1280` `Tier: index*5/len(rows)+1`，带 `TierLocallyCalculated: true`），JSON 里 `items[].tier` / `hexTier` 才是官方口径——这一条同时是 R116-B 的 P0-5，但解析层要在这里就把官方字段留出来，不要等 R116-B 再回来改数据结构。

### 实现要求

1. 新增 `parseHexdataHeroJSON(data []byte) (hexdataHeroDetailV2, error)`，解析出：
   ```go
   type hexdataHeroDetailV2 struct {
       Items                []hexdataItemRow           // itemId/itemName/winRate/pickRate/games/tier/hexTier/hexLabel/hexScore/deltaWinRate/coreDelta/averageIndex/wilsonLowerWinRate/withoutItemWinRate/withoutItemGames
       Augments             []hexdataAugmentRowV2       // augmentId/augmentDescription/rarity/tier/score/stages[1..4]
       Trios                []hexdataTrioRow            // 裁剪到 top-N，见下
       WeakAgainst          []hexdataMatchupRow         // opponentChampionId/counterDelta/evidence/confidenceLow/High/qValue
       StrongAgainst        []hexdataMatchupRow
       TeammateSynergies    []hexdataSynergyRow         // teammateChampionId/synergyDelta/evidence/confidenceLow/High
       TerminalItemTrios    []hexdataTrioRow            // 裁剪到 top-N
       SummonerSpellPairs   []hexdataSpellPairRow
   }
   ```
2. **裁剪**：`trios`、`terminalItemTrios` 实测各达 1000 条（`meta.heroTrioLimitPerHero = 1000` 是上游硬上限，不是随机采样）。解析时只保留按 `games` 降序前 50 条（覆盖率足够——`terminalItemTrios` 最小 `games` 实测已有 1248，样本本身就很扎实，前 50 条基本覆盖所有会被展示的组合）。这一步直接把单英雄解析后的内存占用从「trios+terminalItemTrios 各 1000 条」压到「各 50 条」，对应评审第 4.6 节算式里的内存膨胀问题。
3. **替换 `loadMayhemDetail` 的抓取调用**：
   ```go
   page, pageErr := p.hexdata.load(ctx, "hero-json", strconv.Itoa(id), "/api/hexdata/heroes/"+strconv.Itoa(id), "application/json", false)
   ```
   citation 从 P0 的 `hexdataMetaSnapshot` 取，`CanonicalURL` 仍用现有 `canonical := "/hero/" + id + "-" + slug` 拼出的 HTML 页面地址。
4. **删除**：`decorateHexdataItems`（`:1619-1648`）、`hydrateMayhemAugmentCopy`（`:1702-1740`）、`mayhemAugmentCopy`（`:1740-1807`）、`fetchMayhemAugmentCopy`（`:1807-1824`）、`mayhemAugmentSlugs`（`:1829` 起，若仅被上述函数调用则一并删除）。这些函数存在的唯一理由（HTML 页面没有描述、装备没有 ID）已经被新数据源解决。
5. **保留**：`decorateHexdataAugments`（`:1650-1678`，按 ID 补图标/rarity，与新数据源无冲突，且新数据源不提供 CommunityDragon 风格的图标路径，不要改用 `augmentIconUrl`/`itemImageUrl` 这两个 hexdata 自己 CDN 的字段——避免新增图片域名，且与站内其他图标风格不一致）。
6. **老函数清理**：`parseHexdataHeroDetail`（HTML 正则解析）如果只被 `loadMayhemDetail` 调用，替换后即为死代码，一并删除；若被其他路径（如榜单页）复用，保留但要在 PR 描述里写清楚为什么。
7. `hexdataHeroPathPattern`（HTML 详情页正则）如果因为第 6 步的删除而不再被任何地方引用，从 `allowedPath` 白名单里移除；`/augment/{id}-{slug}` 对应的 `hexdataAugmentPathPattern` 同理评估。

### 验证判据

1. 冷启动打开一个海斗英雄详情页的 hexdata 请求数：改动前 `2+N`（N≈40，实测样例请求数 42），改动后应为 **2**（`/api/hexdata/heroes/{id}` + `/api/hexdata/meta`，后者 12h 内命中缓存后降为 1）。用集成测试断言 mock server 收到的请求路径列表长度。
2. `response.RecommendedAugments` 里每条的 `Description` 字段非空比例应 ≥ 95%（对照实测 126/126 满覆盖，留 5% 容差给上游数据缺口）。
3. `response.ItemRanking` 里每条的资产 ID 应等于 JSON 的 `itemId`，不再经过名称反查——用一个"中文名故意重复/含特殊字符"的 fixture 验证不会走错装备。
4. `go test ./backend/... -run Mayhem` 全绿；额外跑一次全量 `go test ./backend/...`，确认删除的函数没有残留的孤儿调用点（编译期就会报错，但要显式在验收记录里写"编译通过"）。

### 对抗变异

- 故意在测试里把 `augmentDescription` 设为空字符串，确认前端渲染路径会走"整块隐藏"或占位而不是显示空描述（呼应第 6.5 条降级规则）。
- 故意保留一个对 `fetchMayhemAugmentCopy` 的死引用（不删干净），确认 `go vet`/编译能捕获（这一条主要是提醒执行方仔细删代码，不是要求新增测试）。

---

## P2：接入两个全量聚合文件（`/postmatch` + `/hextech-insights`）

### 现状证据

- 实测：`/api/hexdata/postmatch` 200，90,058 字节；`/api/hexdata/hextech-insights` 200，373,350 字节。两者都是**一次请求覆盖全部 173 英雄**，与 per-entity 的 `/api/hexdata/heroes/{id}` 性质完全不同——它们更接近现有的 `/heroes`（榜单）、`/augments`（图鉴）这两个全量端点，应该按同样的方式对待：**每个 buildID 只取一次，长期缓存**。
- `hextech-insights.heroes[]` 每条含 `topItems[5]`（带 `confidence`）和 `topAugments[5]`（带 `rarity`/`reason`/`pairWinRate`/`deltaWinRate`/`confidence`），这正好是对局推荐页需要的"10 个英雄概览 + 推荐"的全部字段——**不需要对每个英雄单独调 `/heroes/{id}`**，R116-D 的 P1-2/P1-3/P1-5 都应该优先从这个文件取数据，只有 P1-2/P1-3 需要的 `weakAgainst`/`teammateSynergies` 才落回单英雄的 `/heroes/{id}`（且只拉本人英雄一次，见 R116-D）。

### 实现要求

1. 新增 `loadHexdataPostmatch(ctx)` / `loadHexdataHextechInsights(ctx)`，走 `kind="postmatch"` / `kind="hextech-insights"`，`load()` 参数里 `id` 传固定值如 `"all"`（与现有 `/heroes` 用 `id="all"` 的惯例一致，见 `hexdata.go:1260`）。
2. 两者的缓存 TTL 参考 `hexdataCacheTTL`（10 年，配合 buildID 变化触发重新拉取的既有机制，不需要单独设计过期），落盘走既有的 `hexdata-` 前缀通道（`champion_cache.go:341`）。
3. 解析后的结构体在内存里应该做字段级裁剪：`postmatch` 每条只留 22 项指标本身（无需裁剪，89 KB 已经很小）；`hextech-insights` 的 `heroes[]`/`augments[]` 无需裁剪（172/211 条，每条字段数有限，总量可控）。

### 验证判据

1. 用集成测试断言：连续两次调用 `loadHexdataPostmatch`/`loadHexdataHextechInsights`（模拟对局页两次打开），实际发出的 HTTP 请求数为 **1 次**（第二次命中缓存），不是 2 次。
2. `loadHexdataHextechInsights` 返回的 `heroes` 长度在 150~200 之间（对照实测 172，注意上游可能因下架英雄有小幅波动）。

---

## P3：修复 `hexdata-` 前缀文件豁免磁盘预算

### 现状证据

- `champion_cache.go:445-450`：
  ```go
  // Hexdata正文 and its validator state are user-facing recovery data.
  // They must not be selected as generic LRU victims based on mtime.
  protected := strings.HasPrefix(item.Name(), "hexdata-") || strings.HasPrefix(item.Name(), "proseed-")
  if protected { continue }
  ```
  这段保护是**故意设计**（注释写明意图：hexdata 正文是用户可见的兜底恢复数据，不该被普通 LRU 按 mtime 误杀）。真正的缺口不是"要不要保护"，是"**没有任何机制在 buildID 轮换后回收旧版本文件**"。
- 落盘的 envelope 文件（`championCacheEnvelope`，JSON 格式）里的 `Key` 字段完整保留了原始 cache key（`hexdata.go` cacheKey 格式 `{buildID}|{kind}|{id}`），可以从文件内容里解析出 buildID，不需要改文件命名格式（`pathFor` 用的是 key 的 sha256 哈希，文件名本身不含 buildID）。

### 实现要求

1. 新增 `(c *championDataCache) pruneStaleHexdataBuilds(currentBuildID string) error`：扫描 `c.dir` 下所有 `hexdata-*.json` 文件，逐个用现有 `readCacheFile` 打开、解析出 `entry.Key`，取 `strings.SplitN(entry.Key, "|", 2)[0]` 作为该文件的 buildID；若不等于 `currentBuildID` 且经过一个宽限期（建议 2 个 buildID 周期，约 8 天，避免正在使用的旧缓存被立刻删除导致回退请求风暴），则删除。
2. 触发时机：在 `hexdataClient` 确认拿到新 buildID 的地方（`hexdata.go` 里 `adoptCitation` 或 `refreshBuild` 成功之后）异步调用一次，不要挂在请求主路径上阻塞响应。
3. 这个新函数**不修改**现有 `protected` 逻辑，是并行的第二条清理路径——现有保护继续防止"因为访问不频繁被普通 LRU 误杀"，新函数只处理"buildID 已经过期"这一种情况。

### 验证判据

1. 构造两个不同 buildID 的 `hexdata-*` 落盘文件，调用 `pruneStaleHexdataBuilds(newBuildID)`，验证旧 buildID 文件被删除、新 buildID 文件保留。
2. 验证宽限期内（模拟 buildID 刚变化）不会立即删除上一个 buildID 的文件（用时间戳字段或额外参数控制，避免真实等待 8 天）。
3. `du -sh` 模拟 173 英雄 × 24 个 patch 的场景（用小规模 fixture 等比例验证清理逻辑生效），确认磁盘占用不会无限增长。

---

## 明确不做（Anti-scope）

1. **不触碰 `/api/hexdata/heroes`（无 ID 的列表接口）**——上游已 403 拦截并明确警告，且实测证实我们不需要它（`/heroes` 榜单页和 `/hextech-insights` 已经覆盖全量英雄概览）。
2. **不改用 hexdata 自己 CDN 的图标字段**（`augmentIconUrl`/`itemImageUrl`）——继续用 CommunityDragon，避免新增图片域名和风格不一致。
3. **不在本工单里实现 P0-1~P0-6 的前端渲染**——本工单只做数据源层，前端消费留给 R116-B。
4. **不删除 `protected` 保护逻辑本身**——P3 是新增按 buildID 的清理路径，不是移除既有保护。

## 通用验证要求

- `go build ./backend` 通过，`go test ./backend/...` 全绿（含 race）
- 新增/修改文件跑一遍 `go vet`
- 构建打包后确认版本号同步到 0.12.8
- 真机验证：连续打开 5 个不同海斗英雄详情页，导出诊断日志，确认没有出现 `hexdata_circuit_open`（证明没有触发熔断）

## 交付物

- `hexdata.go` 的路径白名单、`inspectHexdataPayload` 新 case、`hexdataMetaSnapshot` 缓存、`loadMayhemDetail` 重写、`parseHexdataHeroJSON`/`loadHexdataPostmatch`/`loadHexdataHextechInsights` 新函数
- `champion_cache.go` 的 `pruneStaleHexdataBuilds`
- 删除的死代码清单（写进执行账本，逐个函数名+删除理由）
- `docs/r116a-execution-ledger.md`

## 真机验证清单

- [ ] 打开一个此前从未访问过的海斗英雄详情页，导出诊断日志，确认请求数为 2（hero-json + meta）
- [ ] 连续打开 5 个不同英雄详情页，确认无熔断触发
- [ ] 手动触发一次假想的 buildID 变化（改测试环境变量或用 mock），确认旧文件被清理

## 工单结束标志

- [ ] P0~P3 全部验证判据 PASS
- [ ] 独立核对：随机抽 3 处代码引用行号，确认与实际代码一致（代码会随改动移动，账本要记录改动后的新行号）
