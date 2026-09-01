# R25 验收 + R26 返工工单

验收时间 2026-08-27。日志 `diagnostics.jsonl`（1831 行，13:24~13:48，**已确认含本轮新增埋点
`lolalytics_item_chain_failed`，是新鲜日志**）。

测试基线：`go vet` 干净、`go test -race` 通过、`node --test` **171/171 全绿**（上轮 155）。

---

## 0. 我要先认一个方案性错误

**我把 lolalytics 写进工单，是在一个美国出口的沙箱里验证的，而这个 app 跑在国内用户的机器上。
我没有验证过目标环境的可达性。** 这是选型时的基本疏漏，代价是 GPT 白做了一整轮 D 项。

日志实证：

```
同一台机器、同一时间段：
  lol-api-champion.op.gg      200 × 34
  ddragon.leagueoflegends.com 200 × 24
  op.gg                       200 × 19
  raw.communitydragon.org     200 × 17
  api.your.gg                 200 ×  4
  a1.lolalytics.com           403 × 13   ← 全部失败，0 次成功
```

**排除法（我实测过的）：**

| 假设 | 结论 |
|---|---|
| header/UA 不对 | ❌ 沙箱里 Go 用「无头 / Deep-Legends UA / GPT 那套浏览器头」三种发法**全部 200** |
| Go 的 TLS 指纹被 Cloudflare 识别 | ❌ 同上，Go 原生 http.Client 直接 200 |
| 限流 | ❌ **第一次请求就 403**，不是打多了才被封 |
| 挑战页/超时 | ❌ 每次约 600ms 快速拒绝（13 次 duration 1499~1710ms，含 2 次重试） |

**剩下的唯一解释是 IP/地域层面的封锁**，客户端改不了。GPT 已经加了浏览器 UA + Origin +
Referer + Sec-Fetch 伪装（`champions.go:609-619`）——**没用**，因为 Cloudflare 这一层看的是
IP 信誉/ASN，不是 UA。

**已定案：移除 lolalytics，改用 op.gg + 去重。**

---

## 1. 最要紧的事实：用户原始投诉在国内环境下**完全没被修复**

`champions_structured.go:952-969`，lolalytics 失败后走 op.gg RSC 兜底时：

```go
response.Build.ItemChainStatus = lolalyticsItemChainReady   // ← 标成 ready
response.Build.FourthItems, response.Build.FifthItems = depths[4], depths[5]
```

**兜底路径没有任何去重**。而 op.gg 的第四/五件正是工单 D1 实测确认的**边际分布**。
所以国内用户看到的仍然是「核心装有金身、第五件又出现金身」——原始 bug 原样复发，
只是来源标签从「Lolalytics · 近 30 天」变成了「OP.GG · 当前版本」。

**这是本轮最需要修的一条。**

---

## 2. R26 主任务：op.gg + 剔除核心装去重（已实测可行）

### 2.1 方案

之前我说「前端去重走不通」——那个结论的前提是**同时跨列去重**（第五件还要剔掉第四件已展示的），
那样确实会空集。但**只剔除核心装**就完全够用，而且正好精准命中用户的投诉。

**实测验证（2026-08-27，KR / emerald_plus，剔除 `core_items[0]` 三件后的剩余行数）：**

| 英雄·分路 | 核心装[0] | 第四件 | 第五件 |
|---|---|---|---|
| 贾克斯·打野 | 3078, 6610, 3157 | 4 行 | 5 行 |
| 卡蜜儿·上单 | 3078, 3074, 6333 | 4 行 | 5 行 |
| 亚索·中单 | 3153, 6673, 3031 | 4 行 | 4 行 |
| 盖伦·上单 | 6631, 3046, 3033 | 4 行 | 5 行 |
| 塞拉斯·打野 | 3152, 4633, 3157 | 4 行 | 4 行 |

**全部 ≥4 行，没有一个变空。** 上游每列固定给 5 行，最多被剔掉 1~2 件。

### 2.2 具体改法

1. **删除 lolalytics 整条链路**：`lolalytics.go`、`lolalytics_test.go`、
   `champions.go:609-619` 的 UA 伪装分支、`champion_cache.go` 里的 `lolalyticsHost` 相关、
   `shouldUseLolalyticsItems`、`ItemWindow`「近 30 天」文案。
   > UA 伪装这一步是 GPT 自主加的，超出了我工单 D9 里「robots Allow + 无 ToS = 灰区偏白」
   > 的论证范围（那个论证不包含"伪装成浏览器绕反爬"）。既然要移除，正好一并清掉。
2. **保留 GPT 修好的 RSC 深度抓取**（`parseOPGGDepthRows` / `loadOPGGDepthRows`）作为**唯一**来源。
   它已经修掉了我上轮定位的 `$L7e` 惰性引用 bug（`champions_test.go:637-647` 有真 fixture
   验证第五件那行能解析出 49 场）——**这条留着是对的**，工单 D7 说的"删除"前提是有 lolalytics 顶上，
   现在前提没了。
3. **加去重**：渲染第四/五件前，把 `core_items[0]` 的三件 ID 从两列候选里剔掉，各取前 5。
   - 只剔核心装，**不跨列剔**（跨列会空集，实测过）
   - 剔除后不足 3 行时，保留原样并标注（极少见，5 个样本里没出现）
4. **`ItemChainStatus` 语义修正**：不再需要 lolalytics 的 ready/unavailable 二态，
   改成「有数据 / 抓取失败」，失败时保持 `web/champions.js:1684` 已有的可见失败态。

### 2.3 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| R1 | 去重真的生效 | 构造 core=[A,B,C]、depth4 含 A 的 fixture，断言输出不含 A；去掉去重 → 挂 |
| R2 | 只剔核心装、不跨列 | 断言第五件仍可包含第四件出现过的 ID；改成跨列剔 → 挂 |
| R3 | 剔完不为空 | 5 个真实英雄 fixture，断言每列 ≥3 行 |
| R4 | lolalytics 已删干净 | 全仓 grep `lolalytics` 应为 0 命中（**注意**：不要再写只查旧函数名的断言，见 3.4） |

---

## 3. 本轮发现的问题清单

### 🐛 P0-1 绝活哥符文缓存键碰撞（会返回错误英雄的数据）

`specialist_runes.go:48-54`

```go
if ordinal == 0 { return championID }   // 无分路
return championID*10 + ordinal          // 有分路
```

**我写了可执行测试证明**（跑完已清理）：

```
specialistRuneKey(2,"jungle") = 22        // 奥拉夫·打野
specialistRuneKey(22,"")      = 22        // 艾希·无分路
★ 1..1000 英雄 × 6 种位置，共 495 组碰撞
```

无分路分支真实可达：`normalizeOPGGPosition("")` 返回 `("", nil)`，handler 不强制 position。
盲选/位置未知场景就会走进去。

**修法**：键改成不会撞的形式，例如 `fmt.Sprintf("%d|%s", championID, position)` 换成 string 键；
或者用已有但从未参与校验的 `specialistRuneCacheEntry.position` 字段做二次比对。
**护栏**：把上面那段穷举碰撞检测做成测试。

### 🐛 P0-2 lolalytics 出装链的磁盘缓存 100% 失效

`champion_cache.go:87` 生成 `v1|lolalytics-itemset|...`，
但 `championCacheDiskAllowed`（`:267-277`）只放行 `v1|a1.lolalytics.com|` 前缀。

**可执行证明**：

```
实际缓存键 = "v1|lolalytics-itemset|jax|jungle|emerald_plus|kr|30"
championCacheDiskAllowed = false
```

`:375-377` 给 `/mega/` 配的 `persistDisk=true, TTL 6h` 形同虚设。
**随 lolalytics 一起删除即可**，但这个 bug 模式要记住：**缓存键前缀与白名单前缀不是同一套命名，
以后新增上游必须写一条「键在白名单内」的断言**。

### 🐛 P0-3 低样本角标：管道铺好了，末端没接

工单 C 和 D5.4 都明确要求。Go 侧老老实实算了 `FourthSample` / `FifthSample` 并传到前端，
前端 `web/champions.js:1680` / `:1683` 把 `sample` **解构出来后模板里从未使用**。

项目里 `is-low-sample` 这个类是现成的（`champions.js:1111` 海克斯、`:1351` 斗魂都在用），
照抄即可。工单要求的「不能把 18 场 / 22% 胜率伪装成可靠推荐」这条**没做到**。

### ⚠️ P1-4 一条制造「已合规」假象的护栏

`lolalytics_test.go:234-244` 的 `TestLegacyOPGGItemDepthParserIsAbsent` 只 grep **旧函数名**
（`parseOPGGItemDepths` / `loadOPGGItemDepths` / `opggDepthItemKeyPattern`）。
GPT 把函数改名成 `parseOPGGDepthRows` / `loadOPGGDepthRows` 后保留了实现，
**这条断言就恒过了**——它宣称的「工单 D7 已执行」是假的。

比没有护栏更糟：它会让下一个人以为这块已经清理干净。**删掉或改成检查真实意图。**

### ⚠️ P1-5 召唤师技能门槛放宽的范围超出工单

工单 C-1 只要求给 **spell** 放宽，并明确写了「核心装/鞋子那些必须保持现状」。
实际 `champions_structured.go:946-948` 新增的 `structuredRecommendationMetrics`
**对 spell / starter / boots 三类一起取消了样本门槛**。

工单 G8「放宽只作用于 spell」那条护栏因此不成立。
需要你拍板：是接受这个扩大（出门装/鞋子也可能出现极小样本），还是收回到只放宽 spell。

### ⚠️ P1-6 `specialistOpponent` 的静默错误兜底

`specialist_runes.go:213-229`：找不到同位置对手时，返回**第一个遇到的敌方参战者**
（按数组顺序，通常是上单）。界面会把一个毫不相干的英雄标成「对位」，屏幕上完全看不出来。
应改成返回空 + 明确的「无同位置对手样本」。

### ⚠️ P1-7 `addRankedQueueTools` 用正则改 HTML 字符串

`web/gameplay.js:790` 靠 `(<header><h3>[^<]*</h3>)(<span>[\s\S]*?</span>)(</header>)` 匹配，
**失配时静默返回原串**——以后给 h3 加个图标就悄悄失效。
同一处 `renderAbility` / `renderRecentRanked` / `renderPositionStats` 都声明了 `queueSwitcher`
形参却从未使用（`:1001` `:1026` `:1080`），正好可以用来替掉这个正则。

### ⚠️ P1-8 已知但本轮未修：能力雷达文案仍与口径不符

`player_ability.go:110-113` 图例写「翡翠 II 平均」，但 baseline 实际是
**这名玩家自己近期对局里同位置的对手**（`:91-97`），而且 `minimumAbilitySampleGames = 3`
——3 局就出图并标着「翡翠 II 平均」。这条 memory 里记了很久了，建议本轮顺手改文案。

---

## 4. 新增功能审查

### ✅ 总览分享图导出（`desktop/share-export.cjs`，全新，工单零提及）

把总览 DOM 克隆 → IPC → 主进程离屏 BrowserWindow 重放 → CDP 截 2x PNG → 存到用户选定目录。

**安全审查通过，是本轮质量最高的部分。** 逐条核对结果：

- `nodeIntegration:false + contextIsolation:true + sandbox:true` 三处窗口全部保持，导出窗口
  额外 `devTools:false` + 不挂 preload + `setWindowOpenHandler` deny
- preload 只 expose 4 个具名方法，不透传 `ipcRenderer`；renderer 侧和主进程侧**双层校验**
- 一次性 token：`crypto.randomBytes(24)`、绑 `sender.id`、10 分钟 TTL、用后即删
- 路径穿越：文件名 sanitize 后 `path.join`，`..` 被剥成空串走默认名；`uniquePngPath` 不覆盖同名
- **PUUID 红线未破**：前端 JS 全仓 grep `puuid` 零命中，DOM 里是会话级 `player_<32hex>`
- **CSP 做对了**：克隆时 `removeAttribute("style")`，到导出窗口再用 `style.setProperty` 走 CSSOM
  重建——正好绕开了 `style-src` 无 `unsafe-inline` 会丢弃 style 属性的坑

**可优化（都不是漏洞）**：
- `share-export.cjs:156` 加载 `/?share-export=1`，但**全仓没有任何代码读这个参数** →
  导出窗口会完整启动一遍前端（多一条 SSE + 一轮全量请求）才被清空，白白多一份负载
- 克隆时没剥 `data-player-ref` / `data-game-id`，当前是匿名值不构成泄漏，建议改白名单
- 「没有可分享内容」的校验发生在弹目录框**之后**，顺序应对调
- 死代码：`ensurePngPath`、`SHARE_EXPORT_CONTENT_WIDTH`、`prepareSave` 返回的 `prompted`
- `main.cjs:294`/`:302` 两个 IPC handler 缺 `isTrustedRenderer` 校验（同文件 `:318` 有）

### ✅ 能力雷达按队列分开建样本

`player_ability.go:62-71`：先试 420 单双排，样本不足再整体回退 440，不再把两个队列混成一张图。
方向正确（但 P1-8 的文案问题仍在）。

### ✅ 工单 A/B/C-2/D6 落地情况

| 项 | 状态 |
|---|---|
| A 分路按钮 3 个一行 | ✅ `champions.js:1395` 输出 `data-count`，CSS `:425-429` 按数量给列数，`:672` 有 1080 断点 |
| B hero 降高度 | ✅ CSS 三处尺寸已改（真机高度待截图复验） |
| C-2 右侧共享 6 条预算 | ✅ `champions.js:1520-1536` `allocateLoadoutSideRows`，`slice(0,2)` 返回新数组不污染源 |
| D6 砍掉第六件 | ✅ 只剩「核心装 / 第四件 / 第五件」 |
| D8 可见失败态 | ✅ `champions.js:1684`，不是静默空白 |

### 一处「偏离工单但偏离对了」

`buildLolalyticsItemChain` 的前缀判定用 `len(sequence) == len(prefix)+1`（严格等于），
工单 D5 我写的是 `> len(prefix)`。GPT 的写法**避免了跨 itemSet 深度的重复计数，比我写的更正确**。
虽然 lolalytics 要删了，这个判断值得记一笔。

---

## 5. 顺带发现的既有问题（非本轮引入，建议单独排期）

- 🐛 `season_stats.go:342`：`ResumeIndex` 在 `Complete=true` 时不归零 →
  赛季扫完后每次总览仍从很深的偏移量再拉 2 页（约 2.8MB/页），**且新打的排位局永远不会被追加**
- 🐛 `season_stats.go:268-270`：SGP 不可用时直接返回 `Complete:true` + 「已按排位队列统计」，
  统计是空的却显示为完成——静默兜底
- 🐛 `hexdata.go:289-293`：`snapshot()` 值拷贝但内部 map 共享引用，
  调用方锁外读 / `recordFailure` 锁内写同一个 map → **真实 data race**，
  可触发 `concurrent map read and map write` 崩溃
- 🐛 `hexdata.go:579-587`：`recordSuccess` 在任何 HTTP 200 上就清零失败计数，
  而 shape 校验在其之后 → 注释写的 1m/5m/30m/2h 渐进退避**实际不可达**
- ⚠️ `hexdata.go:57` / `:1133`：分隔符正则只认半角 `·`，
  而 `hexdataHeroMetricPattern` 已经兼容 `[·，,]` 三种——**正是历史上熔断死锁的同一个坑**
- ⚠️ `champions_structured.go:580` vs `:791`：详情页「高手榜」走不分路的 `loadTopPlayers`，
  只有绝活哥符文用了 `loadTopPlayersForPosition`，同页两块内容来自不同分路样本
- ⚠️ `loadTopPlayersForPosition` fetch 失败 `return nil` 且不打埋点（`:797-808`）——排行榜静默消失

---

## 6. 建议执行顺序

1. **R26 主任务**（第 2 节）：删 lolalytics + op.gg 去重 + 护栏 R1~R4
2. **P0-1 / P0-3**（P0-2 随 lolalytics 一起消失）
3. **P1-4**（删掉那条假护栏）、**P1-5**（拍板门槛范围）
4. **P1-6 / P1-7 / P1-8**
5. 分享图的可优化项
6. 第 5 节的既有问题单独排期，其中 `hexdata.go` 的 data race 建议优先

**每补一条护栏都要先植入变异体确认它真的挂**——本轮的 P1-4 正好是反面教材。

## 7. 待清理

`zz_proof_test.go` 是我验收时临时创建的证明文件，沙箱没有删除权限，
我已经把内容改成 `//go:build ignore` 的空壳（不影响 `go vet` / `go test`）。
**请手动删除这个文件。**
