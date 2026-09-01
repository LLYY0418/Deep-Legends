# 海克斯大乱斗改版 · 第三轮验收报告

> 2026-08-21。验收对象：GPT 针对 `ACCEPTANCE-REPORT-0821b.md` §四 工单的修复结果。
> 方法：逐条复核 7 条工单（P1×2 / P2×3 / 装饰性×2）+ 重跑全量测试 + 变异复核（`cp` 真副本，含 `.go` 同级文件）。
> 测试基线：`web/*.cjs` **75/75 全绿**（champions **57** + remaining-sort 18），较上轮 +2。Go 测试本沙箱无工具链，仅静态核验。

---

## 结论

**7 条工单全部闭环，包括两条我标为「可不做」的装饰性项。** 上轮存活的三个变异体（E/F/G）现已全部被杀，我另加的转义红线变异（H）也被杀。本轮未发现新缺陷。

**跨三轮追踪的 `v2|opgg-detail` 白名单缺陷已关闭**，且修复方式带了防复发护栏，不只是补一个字符串。

---

## 一、工单逐条复验

### P1

**1. `fallbackGameplayAugments` 错模式兜底 —— 【已实现】**

`gameplay.go:3697-3704` 去掉了写死的 `filterGameplayAugmentsForMode(catalog, "arena")`，直接返回全量 catalog，并留了注释说明理由（「backs ID-to-icon lookups, so it must retain both Kiwi and Cherry namespaces just like the LCU catalog path」）。语义与 `loadGameplayAugmentsFromClient`（`:3689-3695`）对齐。

护栏 `TestFallbackGameplayAugmentsRetainsKiwiAndCherryIcons`（`gameplay_test.go:716-738`）：用 RoundTripper 喂一条 Kiwi + 一条 Cherry，断言兜底目录 `len==2` 且两个命名空间各能过滤出 1 条。**这个断言写法是对的**——它不只查总数（总数相等可能是过滤成了另一半），而是正反两向都验，把「过滤成 arena」和「过滤成 mayhem」两种退化同时挡住了。

**2. `v2|opgg-detail` 磁盘白名单 —— 【已实现】**

`champion_cache.go:264` 已补 `strings.HasPrefix(key, "v2|opgg-detail|")`。

护栏 `TestChampionCachePersistentPoliciesUseDiskWhitelistedKeys`（`champion_network_test.go:417-438`）正是我建议的形态：把五类声明 `persistDisk=true` 的 key 逐个跑一遍，同时断言 `championCachePolicy` 仍声明持久化 **且** `championCacheDiskAllowed` 放行。**这是防复发护栏，不是补丁测试**——以后再加缓存键，只要策略与白名单脱节就会挂。第五个 case（`opggDetailCacheKey("ranked","KR",103,"MID","emerald_plus")`）直接调真实 key 生成函数而非硬编码字符串，key 格式变了测试也跟得上。

### P2（护栏补齐）

**3. P0 占位图测试 —— 【已实现】**（`champions.test.cjs:174-189`）

用 `compileFunctions` 真正编译并执行 `renderMayhemRecommendedAugment` 与 `renderMayhemItemRanking`，注入 spy 版 `assetImage` 记录调用。断言分两段：无图时 `calls.length === 0` 且输出含 `mayhem-item-name-only`；有图时 `calls.length === 2`。

**这比断言字符串强得多**——它验证的是「无图时根本没调用 `assetImage`」这个行为，而不是「源码里有个三元表达式」。上轮那种匹字面量的写法正是假护栏的来源。

顺带发现：装备排行 `:740` 的条件已从 `asset.path` 加强为 `asset.source && asset.path`，与推荐海克斯 `:1202` 完全对齐（`imageURL` 需要两者齐全才不返回占位图，只查 `path` 会漏掉 source 缺失的情况）。

**4. 对局内 citation 测试 —— 【已实现】**（`champions.test.cjs:191-211`）

同样真执行 `renderRecommendationCitation`，注入 spy 版 `escapeHTML`。四个字段全部塞了 `<script>` 载荷，断言输出里它们都被包裹成 `[...]`（spy 的标记），最后用 `assert.deepEqual(escaped, [patch, reportDate, buildId, canonicalUrl])` **锁死了转义的字段集合与顺序**——少转一个、多转一个、顺序变了都会挂。

实现侧还超出了工单要求：守卫条件从 `!citation?.source` 加强为 `source/patch/reportDate/buildId/canonicalUrl` 五要素缺一即不渲染（`gameplay.js:2014`），测试 `:210` 也锁了这条。**方向是对的**——§4.8 要求三要素齐全才算合规署名，渲染半行残缺署名比不渲染更糟。

**5. 稀有度双尺度约束 —— 【已实现】**（`champions_test.go:271`）

`augmentRarity(1)=="silver" && augmentRarity(4)=="gold" && augmentRarity(8)=="prismatic"` 与 `hexdata_test.go:419` 两处各自锁住旧尺度入口。加上 `champions.go:1884-1895` 的翻译层，「旧值必经 `augmentRarity`」这条约束现在两侧都有断言。

### 装饰性（我标为可不做，也修了）

**6. 署名行位置 —— 【已实现】**：`gameplay.js:2061` 现在把 `renderRecommendationCitation(payload)` 放在所有 panel 之后、`</section>` 之前，即模块底部，与 §4.8「底部固定一行」字面一致。原来在 tabs 之前。

**7. 抖动伪随机 —— 【已实现】**：`hexdata.go:189-191` 改为 `rand.Int64N(int64(maximum)+1)`，不再是 `UnixNano() % N`。同一纳秒并发请求现在会拿到不同抖动，与 §4.3「随机抖动」字面一致。函数以字段形式注入（`:128` `jitter func(time.Duration) time.Duration`），测试可替换为确定值，可测性也保住了。

## 二、变异复核（本轮亲手执行）

上轮存活的三个变异体全部重放，另加一个我自拟的转义红线变异：

| 变异体 | 上轮 | **本轮** |
|---|---|---|
| E. 推荐海克斯 `:1202` 回退为无条件 `assetImage` | 存活 55/0 | **被杀** 56/1 |
| F. 装备排行 `:740` 回退为无条件 `assetImage` | 存活 55/0 | **被杀** 56/1 |
| G. 对局内 citation 守卫改为 `if (true) return ""`（整行禁用） | 存活 55/0 | **被杀** 56/1 |
| H. `canonicalUrl` 去掉 `escapeHTML`（innerHTML 红线） | 未测 | **被杀** 56/1 |

H 是我额外加的：`renderRecommendationCitation` 的 href 是唯一一处把上游 URL 拼进 HTML 属性的地方，如果只锁「有没有 citation」而不锁「转不转义」，红线就是没护栏的。结果 `assert.deepEqual(escaped, [...])` 那条把它一并挡住了。

**变异环境修正**：`champions.test.cjs:16` 现在会读同级 `../gameplay.go`，副本必须把 `*.go` 一起 `cp` 到 `web/` 的父目录，否则 ENOENT 会伪装成「测试挂了」，把存活的变异误判成被杀。本轮踩到一次并已修正——这是变异测试的经典假阳性，**软链接不行、少拷文件也不行**。

## 三、遗留与观察

**无 P0/P1 遗留。** 三轮追踪的问题已全部关闭。

两条纯记录、不构成工单：

- **hero 形状埋点仍写死** —— `hexdata.go:1066` `p.reportHexdataShape("hero", 16, 16, ...)` 两个维度都是常量，而同函数的 augment 分支（`:941`）用的是 `len(detail.Rows)`。这意味着 hero 链路的 `hexdata_shape` 事件永远上报 16/16，**上游真实缩到 8 条时诊断日志看不出来**——校验逻辑本身（`:523` 强制三值 > 0）是好的，只是埋点这一路失去了观测价值。属既有瑕疵、本轮未加重，但记一笔：埋点要记真实结构，这条和「不只记大小」是同一个教训。
- 本沙箱无 Go 工具链（`go: command not found`），Go 侧四条新测试（cache 白名单、fallback 目录、稀有度、UA）均为**静态核验**：读断言逻辑 + 反查被测函数实现，未实际执行。逻辑上成立，但**没有跑过**——建议在有 Go 环境的机器上跑一次 `go test ./...` 确认，特别是 `TestChampionCachePersistentPoliciesUseDiskWhitelistedKeys` 里 `championCachePolicy(opggChampionHost, "/api/KR/champions/ranked/103/MID", ...)` 这条依赖 `containsChampionDetailPath` 的路径段计数，静态看是通的，实跑更稳妥。

## 附：红线复核（三轮均干净）

- hexdata 请求链路无 PUUID / summonerId / 召唤师名。
- `/api/` 只碰 `answer-cards` 一个白名单文件；无 `/data/` 请求。
- 无构建期抓取脚本、无 EXE 内 hexdata 快照、无启动预热与定时同步。
- innerHTML 红线：本轮新增渲染全部走模板字符串 + `escapeHTML`，且现已有变异护栏（H）。
- 测试 75/75，无 skipped、无 todo。
