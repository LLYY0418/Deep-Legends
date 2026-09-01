# 海克斯大乱斗改版 · FINAL-PLAN 验收报告

> 2026-08-21。验收对象：GPT 对 `design/mayhem-redesign/FINAL-PLAN.md` 的执行结果。
> 方法：五路并行核验（取数客户端 / 前端布局 / 测试+变异 / 对局内 / 后端数据正确性），关键指控全部由我手工复跑复核，变异测试用 `cp` 真副本执行。
> 测试基线：`web/*.cjs` 73/73 全绿（champions 55 + remaining-sort 18）。Go 测试本沙箱无工具链，仅静态核验。

---

## 结论

**整体判定：【已实现】为主，方案主体落地且红线干净。** 取数纪律、缓存体系、布局改造、对局内链路四条主线全部闭环，§9 取数清单十项全过。残余问题集中在一处 P0（装备/海克斯无图时的占位图违背方案明文）、若干假护栏（变异测试存活，测试保护不了真实现），以及两条对局内引用缺口。

---

## 一、取数（§9 十项全过，附 P1）

| §9 项 | 判定 | 证据 |
|---|---|---|
| 冷启动恰好 2 次请求 | 【已实现】 | `hexdata_test.go` 冷启动预算测试断言 requests==2（answer-cards + /heroes）；budget 逻辑见 `hexdata.go` `shouldFetchHeroes`/bootstrap 分支 |
| 连点同一英雄 10 次 = 1 次 | 【已实现】 | singleflight flights map（`champion_cache.go:102-134`）；测试断言 10 并发合并为 1 次请求 |
| 重启后 = 0 次 | 【已实现】 | `loadState`/`saveStateLocked`（`hexdata.go:201-222`），atomicWriteFile 落盘；测试验证重启后请求数归 0 |
| buildId 变更：旧数据先行、下次操作更新 | 【已实现】 | cacheKey=`buildId\|kind\|id`（`hexdata.go:234-241`）；bootstrap 键单独；12h 软 TTL 挂在 `/hero/67-vayne` 页面刷新上，有测试 |
| 403/断网全模块降级 OP.GG，无白屏 | 【已实现】 | 静默降级路径：`loadARAMRankings` hexdata 失败回落 `contents/tiers`（`champions.go:1448-1477`）；形状不符 → 熔断 24h |
| 429 熔断 24h 且跨重启持久化 | 【已实现】 | `recordFailure` immediate 熔断（`hexdata.go:445-453`），Failures>=3 或 403/429 → CircuitUntil=+24h；测试覆盖 429 路径 |
| 篡改 SEO 结构 → 判定失效降级 | 【已实现】 | 形状探测：/heroes ≥150 行（:661-668）、/hero 8+8（:791-793）、/augment 12（:860-862）；不符即熔断 |
| 无构建期抓取 / 无 EXE 快照 | 【已实现】 | 全仓 grep 无 hexdata 快照文件、无构建脚本 |
| 不碰 `/api/` `/data/`（除 answer-cards） | 【已实现】 | `allowedPath` 白名单（`hexdata.go:230-232`），唯一 /api/ 项即 `hexdataAnswerPath` |
| 请求不含 PUUID/summonerId/召唤师名 | 【已实现】 | hexdata 客户端无任何用户标识入参，与 §5.3 红线一致 |

**P1：**
1. **连接超时 5s，方案写的是 3s**（`hexdata.go` 超时常量，§4.3 表格要求连接 3s / 总 8s）。总超时 8s 正确，连接档没对齐。
2. **UA 无测试保护**——诚实 UA 是 §4.3 的许可条件之一，但没有任何回归测试锁住它。
3. **抖动是伪随机**：`now.UnixNano() % int64(maxJitter+1)`，同一纳秒重复请求抖动相同。实际影响可忽略，但与方案「随机抖动」字面不符。
4. **Wilson 参数回退是硬编码**：`wilsonZ<=0 → 1.96`、`highMin/mediumMin → 1000/250`（`hexdata.go:676-712`）。方案要求「参数直接从响应里读、不要硬编码」，回退值本身与 answer-cards 公布值一致，但回退逻辑是写死的兜底。
5. **`measurementTechnique` 是 Go 常量**（`hexdata.go:35`），非从响应解析。文案与公布口径逐字一致，但没有跟随上游。
6. **`TestNormalizeAugmentRarityIsShared` 只锁了新尺度**（0/1/2/4），没有锁旧的 1/4/8 映射，无法证明两套映射已收敛。
7. 遗留：`v2|opgg-detail` 磁盘白名单漏配（`championCacheDiskAllowed` 扩展了 `hexdata-`/`bootstrap|`/`v2|opgg-rsc|`，但 `v2|opgg-detail|` 仍永不落盘）——上轮遗留，非本次引入，未加重。

## 二、数据正确性（§9 八项）

| §9 项 | 判定 | 证据 |
|---|---|---|
| `hextech-aram` 不再映射 `APIMode: "aram"` | 【已实现】 | `opggModeSpecs["hextech-aram"] = {APIMode: "aram_mayhem", HasRunes: false, HasAugments: true}`（`champions_structured.go:137`）；模式字符串在前端归一到 `"hextech-aram"` |
| `HasRunes: false`，空符文页签消失，前端补丁已撤 | 【已实现】 | `gameplay.go:2100/2105-2108` 透传 spec 标志；`gameplay.js` 无 hasRunes 硬编码残留（仅 `:1991` 布尔兜底）；前端 tab 顺序 augments 第一 |
| 左栏 172 英雄全有胜率样本 + 「本地计算」标注 | 【已实现】 | `/heroes` 一次拉全；`renderChampionTable` "mayhem" 分支列头 胜率/样本；「胜率与样本 · T1-T5 本地计算」可见文本（`champions.js:702/713`） |
| 样本 <250 过滤 | 【已实现】 | `hexdataMinimumSample = 250`（`hexdata.go:34`），分层前过滤（:794-800）；薇恩页「样本 1 / 100%」那种条目已被挡 |
| 三字段署名齐全、canonical 可点、不可折叠 | 【已实现】 | `Citation`/`BuildCitation` 独立字段（`champions.go:288-289`）；`renderSourceCitation`（`champions.js:1205`，canonical 链接转义）；每模块固定底部一行 |
| OP.GG 兜底单独标 OP.GG + 16.16.1 | 【已实现】 | `renderOPGGCitation` 独立渲染；不混写 hexdata 补丁号 |
| `measurementTechnique` 可见 | 【已实现】 | `renderMeasurementTechnique`（`champions.js:1219-1221`） |
| 装备名→id 匹配率埋点、不匹配只显名字 | 【部分实现】 | 精确 byName map + `hexdata_item_match` 遥测（`hexdata.go:934-963`）；**装备排行正确地只显示名称**（`champions.js:740`）——但 **P0**：推荐海克斯走 `renderMayhemRecommendedAugment`（:1197-1202）无条件 `assetImage`，空 source/path 会落到 `/image-unavailable.svg`，违背「只显示名字、不显示占位图」。实测 `imageURL`（:121）空参必回占位图。 |

**P0（唯一）：** 推荐海克斯在 `decorateHexdataAugments` 补不上 CDragon path 时（`hexdata.go:982-984` 只在 path 非空才填 source/path），前端渲染 `/image-unavailable.svg` 占位图。同一页装备排行的处理是对的（只显名字），推荐海克斯没对齐。

## 三、布局（§9 七项，全过）

| §9 项 | 判定 | 证据 |
|---|---|---|
| 三个容器全部具名 | 【已实现】 | `.champion-list-card { container: champion-list / inline-size }`（`champions.css:123`）+ 具名 `@container` 查询；`.rune-board-panel`/`.champion-build-board` 同步具名（`champions.css:421/485`） |
| 斗魂无回归 | 【已实现】（测试有漏洞，见下） | 1180px 堆叠规则仍存在（`champions.css:833-835`） |
| 宽屏左右分栏、只重绘右栏、无返回按钮 | 【已实现】 | `openDetail` 分流 `selectMayhemChampion`（`champions.js:344-349`）；`.mayhem-workspace` 两列 grid |
| 窄屏 ≤1020px 隐藏左栏 + 入口按钮 + 浮层 | 【已实现】 | `@container mayhem-page (max-width: 1020px)`（`champions.css:360-363`）；浮层单实例双宿主（:1025-1066，同一 DOM 节点两处宿主，无复制 DOM） |
| 快速连点无串号 | 【已实现】 | 请求令牌 `state.mayhemRequestToken + 1` + `current()` 检查（`champions.js:443-448`） |
| 推荐海克斯第一块 | 【已实现】 | 详情块顺序（`champions.js:734`）推荐海克斯居首 |
| 战绩卡 118px 高度未回归 | 【已实现】 | `slice(0, 4)` 锚点完好（`gameplay.js:1092`） |

## 四、测试与变异（§9 四项）

- `champions.test.cjs:102` 的「暂不展示」断言：**已随改动删除** ✓
- `:112` 的 legacy `.aram-workspace` 死契约：**仍在**（【未实现】）。我亲手变异验证：把 CSS 里的 `.aram-workspace` 三条规则全部删掉，测试依旧 55/55 全绿——这是死契约，锁住了一个没有任何 JS/HTML 生成的 class，删规则不挂、留规则也毫无保护作用。
- 新增契约的变异测试：**部分是真护栏**（删除整个 `@container mayhem-page` 块 → 2 条测试挂），**部分存活**，共 4 个存活变异体：

| 存活变异体 | 类型 | 说明 |
|---|---|---|
| `champions.test.cjs:273` 斗魂 1180px 断言 | 假护栏 | 正则 `[\s\S]+` 跨 @media 块匹配：删掉 `.arena-redesign-workspace { grid-template-columns: 1fr }` 后测试仍过——它实际匹配到了 `:673` 处无关的 `.champion-loadout-row`。变异实测存活。 |
| `:128` 请求令牌断言 | 假护栏 | 只检查字面量 `state.mayhemRequestToken + 1` 存在于源码，不验证使用路径 |
| `assertMayhemCSSContract` 的窄屏入口基线 | 覆盖缺口 | `.mayhem-tier-entry { display: none }` 基线规则被删/改，测试不挂（契约只锁了 @container 块内 `display: flex`，没锁基线 `display: none`） |
| `:66-69` 窄屏规则（子代理名） | 部分真护栏 | 整块删除会挂；单条规则变异不挂 |

- 变异测试纪律：**用 `cp` 真副本执行**，符合 §9 要求（软链接会被 `__dirname` realpath 解析失效）。

## 五、对局内（§7）

| 项 | 判定 | 证据 |
|---|---|---|
| 前端补丁已撤、由后端决定 | 【已实现】 | `gameplay.js` 无 hasRunes 硬编码 |
| 推荐海克斯 tab 第一位 | 【已实现】 | tab 推入顺序 augments 优先（`gameplay.js:1999-2002`） |
| `assetPath()` augment 分支 | 【已实现】 | LCU→CDragon 图标链路（:2076-2088），不再落 `/image-unavailable.svg` |
| 只拉当前英雄、source=mayhem、不带 tier | 【已实现】 | `query.set("source", "mayhem")`（:1852）；`liveRecommendationTarget`（:1807-1817） |
| 模式识别 2300/2400 | 【已实现】 | `HEXTECH_AUGMENT_QUEUE_IDS`（`gameplay.js:92`） |
| 空态三分支 | 【已实现】 | `:2090-2119` |
| **Kiwi 命名空间过滤** | 【未实现】 | `loadCommunityDragonAugments` 加载全部 655 条 Kiwi+Cherry（`champions_structured.go:745-789`），没有命名空间过滤。方案 §7 明确要求 Kiwi 154 条才是海克斯大乱斗、Cherry 396 条是斗魂——对局内推荐可能混入斗魂专属强化 |
| **对局内引用行** | 【未实现】 | `gameplay.js` grep buildId/reportPatch/reportDate/canonical/citation 零命中；响应 bundle（`gameplay.go:1885-1900`）不携带 citation 三要素。§4.8 署名义务在对局内模块缺失 |

## 六、给 GPT 的待办工单（按优先级）

**P0**
1. `champions.js:1197-1202`：推荐海克斯 asset 无 source/path 时只渲染名字（对齐 :740 装备排行的做法），禁止走 `/image-unavailable.svg` 占位图。

**P1**
2. 修复假护栏：`champions.test.cjs:273` 的 1180px 断言改成锚定块内匹配（跨 @media 块的正则必须消除）；`:128` 请求令牌断言改为验证令牌参与请求竞态判断；`assertMayhemCSSContract` 补锁 `.mayhem-tier-entry` 基线 `display: none`。
3. 删除死契约：`champions.test.cjs:112` 的 `.aram-workspace` 断言连同 CSS 里三条死规则（`champions.css:104/609/632`）一并清掉。
4. 对局内 Kiwi 命名空间过滤（`champions_structured.go:745-789` 按 `KIWI`/`Cherry` 分流）。
5. 对局内引用行：响应 bundle 带 citation 三要素 + canonical，`gameplay.js` 渲染不可折叠署名行。
6. 连接超时 3s 对齐 §4.3；UA 加回归测试。
7. `TestNormalizeAugmentRarityIsShared` 补锁旧 1/4/8 尺度，证明两套映射真正收敛。

**P2**
8. `hexdata.go` 的 Wilson 参数回退与 `measurementTechnique` 常量改为从 answer-cards 响应解析。
9. `parseRecommendedAugments`（`champions.go:2265-2287`）零调用死代码：删除。

## 附：复核过的「无问题」项

- hexdata 链路不含用户标识（§5.3 红线干净，PUUID/summonerId 一个字节都没进）。
- 打点记结构不记大小：`hexdata_item_match` 记 rows/matched；detail 行埋点存在（`hexdata.go:1044`，硬编码 16,16 是轻微瑕疵）。
- `remoteAsset` 白名单未动，RSC 图标 host 本就在放行列表。
- innerHTML 红线：本次新增渲染全部走模板字符串 + escapeHTML，未发现上游字符串直入 innerHTML。
