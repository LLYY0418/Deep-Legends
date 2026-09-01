# 海克斯大乱斗改版 · 第二轮验收报告（复验上一轮工单）

> 2026-08-21。验收对象：GPT 针对 `ACCEPTANCE-REPORT-0821.md` §六 工单的修复结果。
> 方法：逐条复核 9 条工单 + 重跑全量测试 + 亲手变异复核（`cp` 真副本，非软链接）。
> 测试基线：`web/*.cjs` **73/73 全绿**（champions 55 + remaining-sort 18）。Go 测试本沙箱无工具链，仅静态核验。

---

## 结论

**上一轮 9 条工单，8 条已闭环，1 条未动。** P0 已修复，3 处假护栏全部转为真护栏（变异逐个验证），死契约已清除，对局内两条缺口（Kiwi 过滤 / citation）已端到端打通，hexdata 六项 P1/P2 全部落实。

**唯一未实现**：工单外遗留项 `v2|opgg-detail` 磁盘白名单（本就标注为「上轮遗留、非本次引入」，仍未修）。

**新发现两条护栏缺口**：本轮新增的两处实现（P0 修复、对局内 citation）都没有测试保护，变异存活。另发现一条真实缺陷：`fallbackGameplayAugments` 对 Mayhem 模式返回 Cherry 目录。

---

## 一、上一轮工单逐条复验

| # | 工单 | 判定 | 证据 |
|---|---|---|---|
| 1 | **P0** 推荐海克斯无图时只显名字 | 【已实现】 | `champions.js:1202` `const icon = asset.source && asset.path ? assetImage(...) : '<span class="mayhem-item-name-only" aria-hidden="true"></span>'`；新增 `.mayhem-item-name-only { width: 0; height: 46px }`（`champions.css:253`）保持行高。与装备排行（`:740`）做法已对齐 |
| 2 | 修复 3 处假护栏 | 【已实现】 | 见下节变异复核，3 个变异体全部被杀 |
| 3 | 删除 `.aram-workspace` 死契约 | 【已实现】 | `grep aram-workspace web/` 零命中，CSS 三条死规则与测试断言一并清除 |
| 4 | 对局内 Kiwi 命名空间过滤 | 【已实现】 | 新增 `filterGameplayAugmentsForMode`（`champions_structured.go:795-808`）按 IconPath 中 `/kiwi/` `/cherry/` 分流 + `gameplayAugmentIndexForMode`（`:810-817`）。四个消费点全部显式传模式：`hexdata.go:991`（对局内 mayhem）、`champions.go:1791`（图鉴）、`champions_structured.go:718`（arena 组）、`gameplay.go:3701`。有 Go 测试 `TestCommunityDragonAugmentNamespacesStayModeScoped`（`champion_network_test.go:589-604`），三向断言（mayhem 只留 Kiwi / arena 只留 Cherry / 索引同样隔离），**是真护栏** |
| 5 | 对局内 citation 三要素 + canonical | 【已实现】 | 后端：`gameplayRecommendationBundle.Citation`（`gameplay.go:1896`）← `detail.Citation` 透传（`:2103`）← `hexdata.go:1061` `response.Citation = &primary.Citation` ← `hexdataCitation` 解析四要素（`:653-662`）。前端：`renderRecommendationCitation`（`gameplay.js:2012-2021`）渲染 `<footer class="recommendation-source-line">`，静态不可折叠，patch/date/buildId/href **四个字段全部过 `escapeHTML`**（`:2016-2020`），innerHTML 红线干净 |
| 6 | 连接超时 3s / UA 回归测试 | 【已实现】 | `champions.go:359` `net.Dialer{Timeout: 3 * time.Second}` + `:362` `DialContext`，5s→3s 已对齐；hexdata 总超时 8s 双保险（`hexdata.go:393` ctx + `:415` client.Timeout）。UA 测试 `TestHexdataRequestUsesHonestUserAgent`（`hexdata_test.go:442-458`）捕获真实请求头，正向断言三段式 UA，反向断言不含 GPTBot/ClaudeBot/CCBot |
| 7 | 稀有度旧尺度补锁 | 【已实现】 | `hexdata_test.go:419` 新增 `augmentRarity` 的 `{1,silver} {4,gold} {8,prismatic}` 断言。**消歧机制**：1/4 在两套尺度里语义冲突，解法是不让旧尺度直接进共享函数——`augmentRarity`（`champions.go:1884-1895`）先把 1/4/8 翻译成 0/1/2 再调 `normalizeAugmentRarity`，边界处分流。`champions_structured.go:891` 同构。设计意图清晰，但**这个「必须经 augmentRarity 进入」的约束本身没有测试或类型保护**，后续若有人直接对 `normalizeAugmentRarity` 传裸 1/4 会静默出错 |
| 8 | Wilson 参数 / measurementTechnique 改为响应解析 | 【已实现】 | `applyHexdataLocalTiers`（`hexdata.go:704-713`）三个参数全取自 `methodology`，**硬编码回退已彻底删除**——参数缺失时直接 `return` 不计算层级，而非退回 1.96/1000/250；`hexdata.go:523` 形状校验强制三者 > 0，拿不到就整体降级。`measurementTechnique` 改为 `hexdataMeasurementTechnique(answer)`（`:38-40`）从响应读，原 `:35` 常量已不存在。测试 `hexdata_test.go:426-440` 用 2.58/321/1234 非默认值验证参数确实来自响应——**这个测试选值很好，用默认值就测不出来** |
| 9 | 删除 `parseRecommendedAugments` 死代码 | 【已实现】 | 全仓 Go 源码零命中（仅剩 design 文档与构建产物）。且 `RecommendedAugments` 从「永不赋值」变为真正赋值：`hexdata.go:1062`，含 RSC 兜底（`:1075-1076`）与全空拒绝（`:1084-1086`），消费点 `gameplay.go:2109` |
| — | `v2\|opgg-detail` 磁盘白名单（遗留项） | **【未实现】** | `championCacheDiskAllowed`（`champion_cache.go:263-273`）仍只放行 `hexdata-` / `bootstrap\|` / `v2\|opgg-rsc\|` + 4 个 v1 host。而单英雄详情 key 是 `v2\|opgg-detail\|...`（`champion_cache.go:82-84`，使用点 `champions_structured.go:426`），其缓存策略明确要求落盘（`:357-358` 返回 `persistDisk=true`），却被 `:115` 读盘与 `:163-165` 写盘双重拦截 → **策略声明与白名单自相矛盾，最热的请求重启即失**。无任何测试覆盖 |

## 二、变异复核（本轮亲手执行，`cp` 真副本）

**上一轮的假护栏，现已全部转真：**

| 变异体 | 结果 | 说明 |
|---|---|---|
| A. 删除斗魂 1180px `.arena-redesign-workspace { grid-template-columns: 1fr }` | **被杀** 54/1 | 测试改用 `cssBlockAfter(css, arenaMarker)` 平衡括号提取（`champions.test.cjs:54-65`），消除了原来 `[\s\S]+` 跨 `@media` 块误匹配 `:673` 无关规则的问题 |
| B. 基线 `.mayhem-tier-entry` `display:none` → `flex` | **被杀** 53/2 | `assertMayhemCSSContract` 补锁基线规则 |
| C. 块内 `.mayhem-tier-entry` `display:flex` → `none` | **被杀** 53/2 | 块内断言经 `cssBlockAfter` 锚定，不再跨块 |
| D. 请求令牌：`current = () => token === state.mayhemRequestToken` → `() => true` | **被杀** 54/1 | `:142` 断言已从「查字面量存在」改为三段序列匹配（令牌自增 → current 定义 → `if (!current()) return`），真正锁住竞态判断路径 |

`cssBlockAfter` 这个辅助函数是本轮修复的关键——它把「正则跨块误匹配」这类假护栏从根上消除了，后续新增 CSS 契约都应该走它，不要再手写 `[\s\S]{0,N}` 猜距离。

**新增实现无护栏（本轮新发现）：**

| 变异体 | 结果 | 说明 |
|---|---|---|
| E. P0 修复回退：推荐海克斯改回无条件 `assetImage` | **存活** 55/0 | 工单 1 修好了实现，但没加测试。同一个 bug 再犯一次，测试抓不住 |
| F. 装备排行 `:740` 条件回退 | **存活** 55/0 | 同上 |
| G. 对局内 citation 整行返回空字符串 | **存活** 55/0 | `renderRecommendationCitation` 无任何断言。**署名是我们免谈授权使用 hexdata 数据的直接依据（§4.8），这一行被误删不该是静默的** |

榜单页那条 citation 有测试（`champions.test.cjs:153` 锁 `renderSourceCitation` 的三要素模板），对局内这条没有——两条渲染路径独立，护栏没跟着走。

## 三、本轮新发现的缺陷

**`fallbackGameplayAugments` 对 Mayhem 返回错模式目录**（`gameplay.go:3697-3702`）：

```go
func (a *app) fallbackGameplayAugments(ctx context.Context) ([]gameplayAugment, error) {
	catalog, err := a.champions.loadCommunityDragonAugments(loadCtx)
	return filterGameplayAugmentsForMode(catalog, "arena"), err   // ← 写死 arena
}
```

它是 `loadGameplayPerkCatalog` 在 LCU 读 `cherry-augments.json` 失败时的兜底（`:3585`，另 `:3522` ddragon 分支同样），产物进 `gameplayPerkCatalogResponse.Augments`，前端存为 `state.perks.augments`，被 `assetPath("augment", id)`（`gameplay.js:2626-2629`）用来查图标路径。

**后果**：海克斯大乱斗对局中若 LCU 目录读取失败，兜底目录里只有 396 条 Cherry、没有 154 条 Kiwi，Mayhem 强化 ID 一个都查不到 → `augment?.iconPath || ""` 全部返回空 → 推荐海克斯图标集体消失。这正是工单 4 要根治的「命名空间边界」问题，在这条兜底路径上还留了个口子。

对比：`loadGameplayAugmentsFromClient`（`:3689-3695`）直接返回 LCU 全量 655 条不过滤——这条**反而是对的**，因为它是给 `assetPath` 做 id→iconPath 查表用的，跨模式全量查表无害；有害的是兜底路径把它**收窄成了错误的那一半**。修法是让 `fallbackGameplayAugments` 返回全量（与 LCU 路径一致），而不是给它加个 mode 参数——查表用途本就不该按模式切分。

**严重度 P1**：仅在 LCU 目录读取失败时触发，非常规路径，但触发时是整块图标消失。

## 四、给 GPT 的待办工单

**P1**
1. `gameplay.go:3701`：`fallbackGameplayAugments` 去掉 `filterGameplayAugmentsForMode(..., "arena")`，返回全量 catalog，与 `loadGameplayAugmentsFromClient` 的语义对齐（这是 id→iconPath 查表源，不是推荐候选集）。加一条测试锁住「兜底目录同时包含 Kiwi 与 Cherry 条目」。
2. `champion_cache.go:264`：`championCacheDiskAllowed` 补 `v2|opgg-detail|`。当前 `:357-358` 声明 `persistDisk=true` 而白名单不放行，是自相矛盾的死配置。补一条测试：遍历所有声明 `persistDisk=true` 的 key 前缀，断言其全部通过 `championCacheDiskAllowed`——这样以后再加缓存键不会重蹈覆辙。

**P2（护栏补齐，实现本身没问题）**
3. 给 P0 修复加测试：断言 `champions.js` 中推荐海克斯与装备排行两处都有 `source && path`（或 `path`）条件分支，无图时走 `.mayhem-item-name-only` 而非 `assetImage`。变异 E/F 必须能挂。
4. 给对局内 citation 加测试：断言 `gameplay.js` 的 `renderRecommendationCitation` 输出含 buildId/patch/reportDate 三要素与 canonical 链接，且四者均过 `escapeHTML`。变异 G 必须能挂。
5. 稀有度双尺度：加一条测试或注释锁住「旧尺度必须经 `augmentRarity` 进入」的约束，避免有人直接对 `normalizeAugmentRarity` 传裸 1/4。

**装饰性（可不做）**
6. 对局内署名行位置在 `.recommendation-area` 顶部（`gameplay.js:2061`，tabs 之前），§4.8 写的是「模块底部固定一行」。要素齐全、不可折叠，仅位置与文字描述有出入。
7. 抖动仍是 `now.UnixNano() % (maxJitter+1)` 伪随机；`hexdata.go:1044` detail 行埋点硬编码 16,16。两条均为上轮已记录的轻微瑕疵，未加重。

## 附：复核过的「无问题」项

- **红线全部干净**：hexdata 请求链路无 PUUID/summonerId/召唤师名；`allowedPath` 白名单唯一 /api/ 项仍是 answer-cards；全仓无构建期抓取脚本、无 EXE 快照；无启动预热与定时同步。
- **innerHTML 红线**：本轮新增的 `renderRecommendationCitation` 与 P0 占位符渲染均走模板字符串 + `escapeHTML`，`escapeHTML` 本体（`gameplay.js:83`）用 `textContent → innerHTML` 实现，正确。
- 测试计数 73/73 与实现同步，无跳过、无 todo。
