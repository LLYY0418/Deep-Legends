# 海克斯大乱斗 / 斗魂竞技场 · 第五轮工单

> 2026-08-22。来源：你在 `http://127.0.0.1:8793/` 上的 2 张整页截图 + 5 张局部图与 14 条问题。
> 方法：源码逐行定位 + 直接打上游复现契约（hexdata.com.cn / CommunityDragon / OP.GG RSC 三方实测）。
> 沙箱仍然打不通你本机 `127.0.0.1:8793`，所以下面全部结论来自**上游真实字节 + 源码**，不是看页面猜的。
>
> **2026-08-22 二稿**：收到你发的 `lol-loot-diagnostics.jsonl`（6075 行，`2026-08-21T15:41:13.819Z → 17:28:37.253Z`）。**0.1 与 A 节被日志推翻并整节重写**，D4 已定案。改动的地方都标了 ⚠️。
>
> **你现在就能自救的一件事**（不用等 GPT 改代码）：关掉应用，删掉 `~/Library/Caches/LOLLootAssistant/champion-data/hexdata-state.json`（Windows 上是 `%LOCALAPPDATA%\LOLLootAssistant\champion-data\hexdata-state.json`），再打开。熔断状态就在这个文件的 `circuitUntil` 字段里，删掉即复位，hexdata 数据应当立刻回来。**这只是绕过，A 节还是要改**——否则下一次结构抖动又会锁死 24 小时。


---

## 零、先回答你最关心的几件事

### 0.1「严重的问题还是 hexdata 数据拿不到」——**熔断器还开着，一个请求都没发出去**

> ⚠️ **2026-08-22 用日志改写。本节原文说根因是 304 陷阱，被你发来的 `lol-loot-diagnostics.jsonl` 直接证伪。原文保留在 A 节末尾作为 P1 硬化项。**

日志覆盖 `2026-08-21T15:41:13.819Z → 17:28:37.253Z`，共 6075 行。三个数字就定案了：

```
"event":"hexdata_fallback"      27 条 —— reason 字段 27/27 全是 "hexdata circuit is open"
                                          出现 "304" 的：0 条
"event":"hexdata_circuit_open"  84 条 —— until 字段 84/84 是同一个常量
                                          "2026-08-22T22:19:08.092846+08:00"
"event":"champion_upstream"    256 条 —— host 里出现 hexdata.com.cn 的：0 条
```

逐条读：

1. **降级理由不是 304，是熔断。** 我在 0.2 让你去找的那句 `champion provider returned HTTP 304` 一次都没出现。
2. **熔断是在日志窗口之前就跳的。** `until` 减 24h = `2026-08-21T14:19:08Z`，比日志第一行还早 **1 小时 22 分**。而且 84 条共用同一个 `until`——说明这 1 小时 47 分里它**一次都没有被重新触发过**，是进程启动时从磁盘 `hexdata-state.json` 的 `circuitUntil` 读回来的旧状态。
3. **整整 1 小时 47 分，我们没有向 hexdata 发过一个请求。** 256 条 `champion_upstream` 的 host 分布是 `raw.communitydragon.org 108 / lol-api-champion.op.gg 80 / op.gg 28 / api.your.gg 26 / ddragon 14`，hexdata **一次都没上桌**。

所以这是 **R4 那条根因原封不动地还活着**：`recordSuccess` 只清 `Failures`、**不清 `CircuitUntil`**。熔断在 HTTP 层**之上**短路，请求根本发不出去 → 没有任何成功路径可以执行 → `CircuitUntil` 永远不会被清 → 死锁。而且没有半开探测，所以它只能等自然到期。上一轮改的是「不要那么容易跳闸」，**没有改「跳了怎么恢复」**。

**这也意味着 304 陷阱到底存不存在，目前是未验证的**——请求都没发出去，那条路径一次都没走到。它在代码上仍然是个真 bug，但它**不是**你现在看到的症状的原因，降级为 P1 硬化项。

**顺带一条诊断覆盖的缺口（这轮必须补）：** 84 次熔断命中只产出 27 条 `hexdata_fallback`，而且 `module` 字段清一色 `"rankings"` ——**另外 57 次命中所在的代码路径完全没有 diag**。更要命的是**熔断跳闸那一刻本身没有任何日志**：日志里找不到「为什么跳」，只有「跳着呢」。这正是 R4 记的「熔断分支零 diag」，一个字都没改。

### 0.2「你是需要日志吗」——**要，但只要一行**

**日志在哪（2026-08-22 补，三条路都能拿）：**

| 平台 | 路径 | 依据 |
|---|---|---|
| Windows | `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl` | `storage.go:91` `os.UserCacheDir()` + `:20` `storageDirectory = "LOLLootAssistant"` |
| **macOS** | **`~/Library/Caches/LOLLootAssistant/logs/diagnostics.jsonl`** | 同上，`os.UserCacheDir()` 在 macOS 上是 `~/Library/Caches` |
| 任意平台（覆盖上面两条） | `$LOL_LOOT_DATA_DIR/logs/diagnostics.jsonl` | `storage.go:83` 环境变量优先 |

轮转备份是同目录下的 `diagnostics.1.jsonl`（`storage.go:540`），必要时一起看。

**更省事的办法：应用开着的时候直接在同一个浏览器里打开 `http://127.0.0.1:8793/api/diagnostics/log`**，会直接下载整份日志（`main.go:283` 路由 → `features.go:91` `handleDiagnosticLog`，`Content-Disposition: attachment`）。之所以不用额外做认证，是因为 `a.authorized`（`main.go:336`）除了 `X-Local-Token` 头之外也认 `lol_loot_token` cookie，而这个 cookie 在你第一次打开面板时就由 `handleBootstrap`（`:350`）种下了。**UI 里确实没有下载入口，但接口一直在。**（顺带修正我之前说的「前端没有任何入口」——UI 没有，HTTP 有。）

~~请把 `diagnostics.jsonl` 里 `"event":"hexdata_fallback"` 的记录发我。按我的判断，它的 `reason` 字段会**原样写着** `champion provider returned HTTP 304`。……另外顺手看一眼有没有 `"event":"hexdata_circuit_open"`——**按我的判断这次不会有**。~~

> ⚠️ **2026-08-22：日志已收到，上面这段预测两句全错。** `reason` 27/27 是 `hexdata circuit is open`，`304` 零命中；`hexdata_circuit_open` 有 **84 条**。见 0.1。

### 0.3 我有五处判断是错的，先认下来（第 3 条是**这份工单自己写错的**）

1. **我说 24h 熔断器是「整页全 OP.GG」的唯一总开关。** 那条根因本身是真的，但我说「不是唯一」也说错了——**日志证明它到现在还是唯一的那个**，见 0.1。R4 那条修复只降低了跳闸频率，没解决「跳了怎么恢复」。
2. **R4 我说「kiwi 154 个 id 与 arena 225 个 id 交集为 0，所以海斗拿不到说明」。** 这句话的**前半句和结论都是对的**，我在本工单初稿里把它「推翻」了——**那次推翻本身是错的**，见下一条。
3. **本工单初稿说「海斗推荐榜里 6 条是 Cherry 命名空间、它们在 arena JSON 里有描述」——这是错的，我已自查推翻。** 我把**图标路径**当成了命名空间判据。实测（2026-08-22 交叉核对）：

   ```
   薇恩 10 条海斗推荐，在 arena JSON 里命中的：0 / 10
   arena JSON 225 个 id 里，带 ARAM_ 前缀的：0
   arena JSON 225 个 id 里，id >= 1000 的：0
   ```

   我当时认作 Cherry 的那 6 条是 `ARAM_DualWield / ARAM_ScopierWeapons / ARAM_FanTheHammer / ARAM_Deft / ARAM_ScopiestWeapons / ARAM_Typhoon`——**全是货真价实的海斗海克斯**，只是图标文件恰好放在 `/UX/Cherry/Augments/Icons/` 下面。

   **正确的模型：同一个海克斯在两个模式里是两条独立记录。** 斗魂那条 `augmentNameId` 不带前缀且 `id < 1000`，海斗那条带 `ARAM_` 前缀且 `id >= 1000`，`nameTRA` 相同、图标相同、**数值不同**、id 零碰撞。例：`116 Flashy` / `1116 ARAM_Flashy`，`19 Dashing` / `1019 ARAM_Dashing`。

   **对工单的影响：B 节那个 bug 依然成立**（而且证据比原来更硬，见下），但 B 节不能再声称「顺便就能从 arena JSON 给海斗补上说明」——**海斗的说明只有 hexdata 一个源**（cdragon 侧我今天逐个探过：`latest/cdragon/{aram,kiwi,mayhem}/zh_cn.json` 全 404，`rcp-be-lol-game-data` 下的 `{kiwi,aram,}augments.json` 也全 404，只有 `arena/zh_cn.json` 是 200；而 `cherry-augments.json` 两个语区 655 条 0 条带 description）。

4. **我预测「这次不会有 `hexdata_circuit_open`」——日志里有 84 条。** 见 0.2 的删除线。这是本轮最重的一次误判：我在没有任何运行时证据的情况下，仅凭「上一轮改过了」就断言它已经不是问题，还据此把整个 A 节建在了 304 上。**教训：「上一轮修过」不构成「现在是好的」，必须要日志。**

5. **我记着「hexdata 也有斗魂（arena-hero / arena-augment）数据」——没有。** 2026-08-22 实测反证：

   ```
   GET /arena-augment/116-flashy               17744 字节
   GET /arena-augment/99999-totally-fake-slug  17744 字节
   GET /                                       17744 字节
   三者 diff：完全无差异
   ```

   任意 slug 都返回同一份字节 → 是 SPA 的 catch-all，正文就是海斗首页（「Hexdata 海克斯大乱斗数据站 · ARAM Mayhem」）。另外 `/api/hexdata/answer-cards` 里 `arena` / `斗魂` / `cherry` 三个词命中 **0** 次，站点自报的 `canonicalPages` 是 home / items / patch / heroes / hexbox / archive / augments / tierList / methodology / bestAugments / augmentRarity / economyRanking——**没有任何 arena 路由**。

   **结论：hexdata 是纯海斗站，斗魂的东西一点都没有。** 你说「斗魂的海克斯数据从那个网站找」，这条路是断的；斗魂说明只能继续走 CommunityDragon 模板 + 本节 D 的渲染修复。OP.GG 侧我也顺手确认过：`lol-api-champion.op.gg/api/global/champions/arena/67`（20979 字节）里 `description` / `desc` / `tooltip` 命中全 0，**它只有 id 和统计，没有文本**。

---

### 0.4 其余上游这段时间是健康的（排除法，省得下轮再查）

256 条 `champion_upstream` 里非 200 的只有 **1** 条：`api.your.gg`，`status: 0`、`cache: "error"`、`duration_ms: 5006`（超时），对应一条 `arena_first_places {"errorKind":"unavailable","outcome":"error","source":"your.gg"}`。其余 25 条 `arena_first_places` 全部干净（`accepted:10, returned:10, rowsOut:10`）。

**一个新信号（并入 B 节的证据）：** 26 条 `arena_augments_built` 有 25 条完全一致——`{augmentsIn:45, groupsIn:3, groupsOut:3, missingMeta:2, rowsOut:45}`（另 1 条 `missingMeta:3`）。**斗魂 45 个海克斯里稳定有 2 个查不到目录元数据**，这是个小而确定的缺口，值得单独定位（大概率就是那 105 条 `/Strawberry/` 路径被两边同时丢弃的残留）。

---

## 一、P0

### A. 熔断器一旦跳闸就永远回不来 → hexdata 永久失联（**P0，真根因**）

> 本节 2026-08-22 依据 `lol-loot-diagnostics.jsonl` 重写。原来的「304 陷阱」内容移到本节末尾 A-bis，降为 P1。

**证据（全部来自日志，不是推断）：**

| 观测 | 值 | 含义 |
|---|---|---|
| `hexdata_fallback.reason` | 27/27 = `hexdata circuit is open` | 降级原因唯一，且不是 304 |
| `hexdata_circuit_open.until` | 84/84 = `2026-08-22T22:19:08.092846+08:00` | 单一常量 → 1h47m 内零次重新跳闸 |
| 跳闸时刻（until − 24h） | `2026-08-21T14:19:08Z` | 早于日志首行 `15:41:13.819Z` 82 分钟 → 从磁盘恢复的旧状态 |
| `champion_upstream` 中 hexdata.com.cn | **0 / 256** | 整个窗口零请求 |

**根因链（两段，第二段是新的）：**

1. `recordShapeFailure` → `recordFailure(0, true)` **一次**就写 24h 熔断（9 处调用点）。跳闸门槛过低这条 R4 已提，**这次的重点不是它**。
2. `recordSuccess` 只清 `Failures`、**不清 `CircuitUntil`**；而熔断判断在发请求**之前**短路。→ **没有任何成功路径能执行 → 没有任何代码能清掉它 → 落盘后重装 EXE 也带着。** 这是一个真正的吸收态：进去了就出不来，只能等 24 小时自然到期，而在到期前的任意一次 shape 失败又会把它续满 24 小时。

**改法（四条，1 和 2 是必须的）：**

1. **半开探测**：`CircuitUntil` 到期前也允许**放行一个**探测请求（例如每 5 分钟一次、单并发）。成功即立刻清空 `CircuitUntil`，失败则不延长。没有半开，熔断器就只是个定时炸弹。
2. **`recordSuccess` 必须清 `CircuitUntil`**，不只是 `Failures`。这是 R4 就该做完的一行。
3. **单次 shape 失败不许直接跳 24h。** 改成计数阈值（如连续 3 次）+ 指数退避（1min → 5min → 30min，上限 24h）。单次结构变化就锁死一天，风险收益完全不对称。
4. **提供一条人工复位**：`hexdata-state.json` 里 `circuitUntil` 早于当前 buildId 的记录一律作废；或在设置面板给一个「重置数据源状态」按钮。用户现在唯一的自救办法是手删文件，而他不知道文件在哪。

**诊断补齐（这轮一并做，否则下次还得猜）：**

- **跳闸点必须写 diag**：`hexdata_circuit_trip`，带 `reason`（哪一处 `recordShapeFailure`）、`kind`、`id`、`failures`、`until`。现在跳闸本身完全无声。
- **熔断命中要全路径记账**：84 次命中只有 27 条 fallback，缺口 57 次。要么让所有短路点都发 `hexdata_fallback`，要么补一个计数型 diag。
- `hexdata_fallback.module` 现在只有 `"rankings"` 一个值，说明其余模块的降级根本没被观测到，一并补上 `module` 取值。

**验收护栏：**

- 构造「`CircuitUntil` 在未来 + 一次成功响应」，断言 `CircuitUntil` 被清空。
- 构造「`CircuitUntil` 在未来且距上次探测 > 探测间隔」，断言**确实发出了**一个 upstream 请求（半开）。
- 构造「单次 shape 失败」，断言 `CircuitUntil` **不是** 24h 而是首档退避值。
- 断言跳闸时产出了 `hexdata_circuit_trip` 且 `reason` 非空。

> 变异要求：把 `recordSuccess` 里清 `CircuitUntil` 那行删掉、把半开条件反转、把退避表改成常量 24h——三条测试要各自变红。**上一轮 `recordSuccess` 的测试就是因为只断言 `Failures` 而漏过了这个 bug，这次断言必须逐字段。**

---

#### A-bis（P1 硬化，不是当前症状的原因）：304 条件请求陷阱

> ⚠️ 这条在日志里**未被验证**——请求都没发出去，这条路径一次都没走到。代码上它仍然是真 bug，修熔断之后它很可能就是下一个坑，所以同轮一起修，但**不要再把它当成「数据没变化」的解释**。

**机制：** 校验器（ETag / Last-Modified）按「请求路径」存在 `hexdata-state.json`；正文按「buildId|kind|id」存在 champion-data 缓存目录。`pruneDisk` 按 **mtime** 淘汰，hexdata 正文写一次就不再更新、mtime 永远最老、每次清理都第一批被删；`hexdata-state.json` 一直在写、从不被删。于是下一次请求：正文没了、校验器还在 → 发条件请求 → 上游正确地回 **304**（空 body）→ `len(stale) == 0` → 硬错误 `champion provider returned HTTP 304`。

**改法：**

1. **`len(stale) == 0` 时不许带校验器。** 没有可复用的正文，就不该发条件请求。
2. **收到 304 但 stale 为空时，丢弃已存的校验器并无条件重试一次。**
3. **304 永远不计入失败**，不喂 `recordFailure` / `recordShapeFailure`，不进熔断计数。
4. **`pruneDisk` 不按 mtime 淘汰 `hexdata-` 正文**，`hexdata-state.json` 本身应在淘汰白名单外。

**验收护栏：** 构造「有校验器、无正文」断言请求**不带** `If-None-Match` / `If-Modified-Since`；构造「304 + stale 空」断言不返错、清校验器、触发一次无条件重试；断言 304 之后 `Failures` 与 `CircuitUntil` 无变化；`pruneDisk` 测试放入 mtime 最老的 `hexdata-` 正文，触发淘汰后断言它还在。

### B. 海克斯名字/图标/品质色三个症状是同一个根因：命名空间过滤按「图标路径」放行

**先纠正判据：`augmentSmallIconPath` 里的 `/Kiwi/`、`/Cherry/` 是资产目录，不是命名空间。** 真正区分模式的是 `augmentNameId` 的 `ARAM_` 前缀与 id 区间。catalog 655 条的实测分布：

```
ARAM_ 前缀 170 条 ｜ 路径含 /kiwi/ 154 条 ｜ 两者只重合 36 条
ARAM_ 但路径不是 kiwi：119 条   非 ARAM_ 但路径是 kiwi：103 条
```

**最硬的证据——拿 hexdata 自己的海斗海克斯全集来量。**`https://hexdata.com.cn/augments` 列出 208 个 id（`min 1001 / max 12317`），208 条**全部**能在 catalog 里查到。把当前过滤器套上去：

| 判据 | 208 条里漏掉 | 放行了名单外的 |
|---|---|---|
| **当前：路径含 `/kiwi/`** | **105 条（50.5%）** | 51 条 |
| 仅 `ARAM_` 前缀 | 6 条 | 35 条 |
| `ARAM_` 或 kiwi 路径 | 6 条 | 71 条 |
| **不过滤，按 id 直查** | **0 条** | — |

**当前判据漏掉了一半的海斗海克斯。**（薇恩那 10 条推荐里，`1225 ARAM_DualWield`、`1071 ARAM_ScopierWeapons`、`1220 ARAM_FanTheHammer`、`1022 ARAM_Deft`、`1115 ARAM_ScopiestWeapons`、`1087 ARAM_Typhoon` 这 6 条正是被路径判据滤掉的，它们的图标在 `/Cherry/` 目录下。）注意 `1356 CriticalMissile` / `1328 CriticalRhythm` 是**不带 `ARAM_` 前缀却出现在海斗推荐里**的，所以换成前缀判据同样不安全——**唯一没有漏报的做法是不过滤**。

`hexdata.go:1019`：

```go
byID := gameplayAugmentIndexForMode(catalog, "mayhem")   // 路径 kiwi-only，154 条
...
meta, ok := byID[asset.ID]
if !ok { continue }        // ← 一半的海斗海克斯在这里掉出去
asset.Description = meta.Description
if meta.Name != "" { asset.Name = meta.Name }
rows[rowIndex].Rarity = normalizeAugmentRarity(meta.Rarity)   // 只在 ok 分支里写
```

`continue` 之后：**没名字**（保留 `hexdata.go:1312` 的占位名「海克斯 1225」）、**没图标**（`Source`/`Path` 不写）、**连 `Rarity` 都不写**（默认空串 → 前端 `augmentRarityKey` 归为 `unknown` → `champions.css` 里根本没有 `.is-unknown` 规则 → 渲染成默认灰）。

**一个判据修复同时解决名字、图标、颜色三件事。**

**改法：**

1. `decorateHexdataAugments` 查目录时**不要按模式过滤**——推荐榜里出现哪个 id 就查哪个 id。模式过滤是给「全量图鉴列举」用的判据，不该用在「按 id 补元数据」上。上表最后一行就是理由：只有不过滤才 0 漏报。
   > **图鉴列举侧顺带一并修**：如果哪里要「列出全部海斗海克斯」，判据也该换成 `ARAM_ 前缀 || id >= 1000`，而不是图标路径。但这跟本条 P0 是两件事，别混在一个 commit 里。
2. **品质必须有 base，目录只做 refine。** 这就是你说的「参考一下海克斯图鉴的颜色是怎么拿的」——图鉴（`champions.go:1805` `loadAugments`）的写法是对的：排行数据源先给一个 base rarity（OP.GG 分支走 `augmentRarity(item.Rarity)`，hexdata 分支走 hexdata 行），目录命中时才覆盖。海斗这条链路**从头到尾没有 base**，`parseMayhemRSC`（`hexdata.go:1290-1348`）压根不写 `Rarity`。
3. **RSC 里其实带了品质、名字和图标，我们全丢了。** 实测 OP.GG 的推荐海克斯行里有：图标 URL `https://opgg-static.akamaized.net/meta/images/lol/latest/aram-augment/DoubleTap_large.png`、英文名 `Double Tap`、以及一个用渐变色编码品质的 SVG（`#FF9BD2 → #6B42DC → #54ACEE` = 棱彩）。**图标文件名与 `augmentNameId` 一一对应**（`DoubleTap` ↔ `ARAM_DoubleTap`），是个可用的 join key。建议 `parseMayhemRSC` 至少把图标 URL 和英文名捞出来做二级兜底，别只留一个 id。
4. **前端那个 100% 空转的调用要修**：`web/champions.js:1198`
   ```js
   const catalogMeta = augmentMetaForAsset(item);   // item 是 championMetricRow
   ```
   `championMetricRow`（`champions.go:212`）**没有 `id` / `name` / `path` 字段**，而 `augmentMetaForAsset`（`:1238`）读的正是这三个 → **永远返回 null**。本该由图鉴兜底补上的名字/图标/品质，一次都没生效过。应传 `item.assets?.[0]`。
5. **前端品质尺度自相矛盾，顺手对齐。** `web/champions.js:923` `arenaRarityKey` 把 `1→silver, 4→gold`；后端 `normalizeAugmentRarity` 是 `1→gold, 4→event`。今天不炸，只因为后端已经把字符串算好了再下发；哪天下发数字就整片错。两张表要合并成一处。
6. **`arenaAugmentGroups`（`champions_structured.go:967`）的静默失败要堵。**
   ```go
   Rarity: normalizeAugmentRarity(map[int]int{1: 0, 4: 1, 8: 2}[group.Rarity])
   ```
   OP.GG 哪天换掉 1/4/8 这套旧尺度，map 查不到返回零值 0 → **全部变白银，没有任何报错**。今天实测（`lol-api-champion.op.gg/api/global/champions/arena/67`，三组 rarity=1/4/8 各 15 条，40 个可解析 id 与 CommunityDragon 逐条比对 **0 处不一致**）是对的，但这是运气不是设计。查不到时要发 diag。

### C. 「装备排行」就是「装备路线」的第一列——难怪重复、难怪数据全是「—」

`hexdata.go:1096-1112`，降级到 RSC 时：

```go
if len(response.ItemRanking) == 0 {
    response.ItemRanking = append([]championMetricRow(nil), rsc.Build.CoreItems...)
}
```

**装备排行直接等于装备路线。**而 `metricRowsFromIDs`（`hexdata.go:1250`）造出来的行**只有 id，没有 Score / WinRate / Games**：

```go
championAsset{ID: id, Kind: "item", Name: strconv.Itoa(id), Source: "ddragon"}
```

所以「装备排行」那块的 HexScore / 胜率 / 样本**必然全是「—」**，而且每行显示的名字就是路线的第一件装备——你截图里四行同名，就是这么来的。

**上游到底重复到什么程度（实测薇恩 RSC，在 Node 里按 `rscRows` 的逻辑原样复刻）：**

```
starter_items: [1042,3006] / [1042,2003,3006]
boots:         [3006] / [3111]
core_items:    [3153,3124,2010]
               [3153,3124,3091]
               [3153,3124,3302]
               [3153,3124,6672]
               [6672,3153,3124]
```

**5 条路线里 4 条前两件完全相同。**所以你说的「只展示前三个」是对的——上游本身就没有 5 条不同的信息量。

**改法：**

1. **降级路径不要伪造装备排行。** hexdata 的 SEO 兜底层里第二张表就是真的装备排行（8 行，带 HexScore / 胜率 / 样本），A 节修好之后它自然回来。拿不到时应当**整块隐藏或显式标注「当前来源无装备排行样本」**，不要拿路线冒充。
2. `web/champions.js:781` `.slice(0, 5)` → `.slice(0, 3)`。
3. 顺带记一笔：`rscRows`（`hexdata.go:1216`）有个 **16000 字符窗口截断**，实测薇恩第一条路线正好撞上了这个上限，它的 id 列表是五条里最不可信的。窗口该按下一个 `["$","tr"` 边界收，而不是按字节数硬切。

---

## 二、P1

### D. 斗魂海克斯说明「数字部分不对」——四类缺陷，实测计数

先说你那句「海斗里面的海克斯说明是正确的借鉴一下」：**海斗那份是 hexdata 上人工写的中文正文**，数字是写死在句子里的，所以永远对。**斗魂借不到这个源**（hexdata 只覆盖海克斯大乱斗）。斗魂唯一的说明源是 CommunityDragon `/latest/cdragon/arena/zh_cn.json`，那是**带占位符的模板**，必须我们自己渲染。

顺带把上一轮的一个误判也纠正掉：`cherry-augments.json` 的 `zh_cn` 和 `default` 两个语区我今天都拉了，**655 条、6 个字段、0 条带 description**——所以「换个语区就有说明」这条路是死的。

**实测 225 条 arena 海克斯，`renderArenaAugmentDescription`（`champions_structured.go:826`）有三类问题：**

| # | 缺陷 | 实测量 | 表现 |
|---|---|---|---|
| D1 | 算术型占位符 `@Name*K@` 完全不匹配 | **69 处、涉及 55 条说明** | 原样漏进 UI |
| D2 | `dataValues` 里查不到 key → 静默替换成空串 | **24 / 134 处** | 句子里的数字凭空消失 |
| D3 | 跨技能引用 `@spell.Augment_X:Field@` | 3 处 | 原样漏进 UI |
| D4 | 数组按「第一个非零」取值（≈ index 0）而非 index 1 | **19 处、涉及 15 条说明** | 数字系统性偏小/偏大 |

**D1 详情。**正则 `@([A-Za-z][A-Za-z0-9_]*)@`（`:805`）匹配不到带乘数的形式。实测乘数只有三种：`*100`（67 处）、`*1000`（1 处）、`*10000`（1 处），而且 **69 处的 key 在 `dataValues` 里 100% 都存在**——纯粹是正则没写全。样例：

```
id=129 神射法师   你的攻击造成相当于你@APRatio*100@%法术强度的额外物理伤害。
id=305 练腿日     获得@MovementSpeed@移动速度和@SlowResist*100@%减速抗性。
id=26  唯快不破   ……则对其多造成@BonusDamagePerMSDifference*1000@%额外伤害。
```

改法：正则扩成 `@([A-Za-z][A-Za-z0-9_]*)(?:\s*\*\s*([0-9.]+))?@`，取值后乘上乘数再格式化。

**D2 详情。**`arenaDataValueText`（`:835`）查不到就 `return ""`，于是「使目标最大生命值降低@MaxHealthReduction@%」渲染成「使目标最大生命值降低%」。样例与真实 key 名：

```
id=369 渴望权力  要 MaxHealthReduction，实际有 HealthReductionPercent
id=322 强化之能量 要 DamageAmp，       实际有 BaseDamageAmp
id=236 诡术恶魔  要 TotalDamage，      实际有 BonusDamage / APRatio
id=103/150/151   要 spell1KeyBind / spell2KeyBind / spell3KeyBind（这三个是按键名，不是数值）
```

改法：`spellNKeyBind` 直接映射成 Q/W/E；其余查不到时**不要留空**，退回 `tooltip` 重渲染，两边都拿不到就**整句丢弃**。注意现在 `:790` 的 tooltip 兜底**只在整段渲染结果为空时才触发**，说明「渲染出来了但里面缺数字」这种半成品**永远走不到兜底**——这条判据要改成「渲染后仍含未解析占位符」也触发。

**D3 + 收尾。**加一条最终清扫：渲染完如果字符串里还能匹配 `/(%i:|@[^@\s]{1,60}@)/`，就判定该条渲染失败，走 tooltip 或整句丢弃。**并把它做成全量断言**：225 条渲染后一个都不许匹配。

**D4（2026-08-22 已定案：取 index 1，不需要你再去游戏里确认）。**

`arenaDataValueText` 用「数组里第一个非零值」取数，等价于绝大多数情况下取 index 0。**正确的是 index 1。** 三条独立证据：

**证据一：index 0 是重复槽位。** 225 条里所有长度 ≥2 的数组共 1024 个，其中 `v[0] == v[1]` 的有 **977 个（95.4%）**。这就是 LoL 数据里「index 0 是 index 1 的副本」那条老惯例。

**证据二：数组的平台期落在 index == 等级。** 对每个数组，检查它在哪个下标之后变成常量（等级用完就不再变化）：

```
平台期落在 index == MaxLevel      （即 index = 等级）    ：113 个数组
平台期落在 index == MaxLevel - 1  （即 index = 等级-1）  ：  0 个数组
两种都成立（不可分辨）                                  ：  3 个
两种都不成立（数组 7 格全程递增，不提供信息）            ：115 个
```

**113 : 0**，没有反例。样例：

```
QuantumComputing BaseDamage  MaxLevel=3  [150, 150, 225, 300, 300, 300, 300]
                                           ↑idx0  ↑Lv1  ↑Lv2  ↑Lv3 之后恒定
WitchfulThinking AP          MaxLevel=3  [ 60,  60, 120, 180, 180, 180, 180]
BannerofCommand  BuffDuration MaxLevel=2 [ 10,  10,  15,  15,  15,  15,  15]
LightWarden  ShieldsToThrow  MaxLevel=2  [  1,   1,   2,   2,   2,   2,   2]
```

**证据三：按 index 0 渲染出来的句子是自证荒谬的。** 闪现向前 idx0=1 → 「你的闪现有 **1** 层充能」（闪现本来就 1 层，等于什么都没给）；超强大脑 idx0=1 → 「获得 **1** 点护盾值」；练腿日 idx0=10 → 「获得 **10** 移动速度」。取 index 1 后分别是 2 层 / 3 点 / 40 移速，全部合理。

**⚠️ 顺手纠正本工单自己的一个错误：** 我在上一稿写「`MaxLevel[1] == 2` 而 `MaxLevel[0]` 与之不同，所以按 index 0 连最大等级都读错」——**这句是错的**。实测 Flashy 与 Dashing 的 `MaxLevel` 都是 `[2,2,2,2,2,2,2]` 全常量。那条旁证作废，结论靠上面三条站住。

**实际受影响的范围（desc 里被引用、且 idx0 ≠ idx1 的占位符）：21 处 / 17 条海克斯。** 其中 2 条因为 idx0 == 0、被「第一个非零」这个启发式**碰巧蒙对**（IceCold、HiveMind），**其余 19 处 / 15 条现在是错的**：

| 海克斯 | 占位符 | 现在渲染 | 应为 |
|---|---|---|---|
| 旋转至胜 SpinToWin | `@SpinHaste@` / `@SpinDamageAmp*100@` | 15 / 15% | **25 / 25%** |
| 超强大脑 BigBrain | `@ShieldBaseTooltip@` | 1 | **3** |
| 残暴之力 TheBrutalizer | `@AbilityHaste@` / `@Lethality@` | 10 / 10 | **15 / 15** |
| 智械互补 ADAPt | `@APAmp@` | 15% | **10%** |
| 双持 DualWield | `@AttackSpeed@` | 15% | **20%** |
| 混合 Hybrid | `@AbilityBoost@` / `@AttackBoost@` | 10 / 10 | **20 / 20** |
| 逃逸 escAPADe | `@ADAmp@` | 15% | **10%** |
| 闪现向前 Flashy | `@MaxAmmo@` | 1 层 | **2 层** |
| 踢踏舞 TapDancer | `@MSPerHit@` | 6 | **8** |
| 小兵法师 Minionmancer | `@MinionAmp@` | 35% | **40%** |
| 恐慌房间 PanicRoom | `@HealthLock@` | 20% | **15%** |
| 练腿日 LegDay | `@MovementSpeed@` / `@SlowResist*100@` | 10 / 15% | **40 / 35%** |
| 全凭身法 Dashing | `@Haste@` | 75 | **200** |
| 瞄准头部 AimForTheHead | `@CritChanceToDamageRatio@` | 40% | **50%** |
| 缩小射线 ShrinkRay | `@DamageReduction@` | 10% | **15%** |

注意 ADAPt / escAPADe / PanicRoom 三条是**变小**（15→10、20→15）——这反证了 index 0 不是「1 级 = 最小值」的语义，它就是个陈旧的重复槽位。

**改法：** `arenaDataValueText` 把「第一个非零」换成 `values[1]`（长度 < 2 时退回 `values[0]`）。**不要**保留非零启发式，那会让 IceCold / HiveMind 这两条恰好蒙对、掩盖真正的语义。

**验收护栏：** 上面 15 条逐条断言渲染值（表格直接抄成测试表）。变异要求：把 `values[1]` 改回 `values[0]`、改成「第一个非零」，两种变异都必须让测试变红。

> **两张截图为什么不能用来结这个案（保留备查）。**
> 截图里是「闪现向前 黄金 ID **1116**」和「全凭身法 棱彩 ID **1019**」——**那是海斗的那对孪生记录**（`ARAM_Flashy` / `ARAM_Dashing`），不是斗魂的 `116` / `19`。
> hexdata 上的文案与截图逐字一致：`/augment/1116-flashy` 「你的闪现有3层充能」、`/augment/1019-dashing` 「获得175技能急速」。这是人工写死的中文正文，**不经过 `renderArenaAugmentDescription`**。
> 而斗魂 Dashing 的 `Haste` 数组是 `[75,200,325,450,...]`——**175 根本不在里面**，这本身就再次印证了「两个模式是两条独立调参的记录」。
加上「闪现有 1 层充能」是空操作，两条合起来指向 index 1。
> **但我不打算第四次在这个问题上过度自信。** 建议：先按 index 1 改，同时在游戏里对着**斗魂模式**的「闪现向前」（应显示 2 层充能）和「全凭身法」（应显示 200 技能急速）确认一眼——**注意要在斗魂里看，不是海斗**。对不上我按实际值收尾。

### E. 英雄列表：去掉样本列 + 高度不占满

**E1 去样本列（海斗）：**

- 表头 `web/champions.js:1119`：`'<th class="metric-win">胜率</th><th class="metric-games">样本</th>'` → 去掉后一个 `<th>`。
- 单元格 `web/champions.js:1142`：去掉 `<td class="metric-games">${compactNumber(row.play)}</td>`。
- **别忘了列宽**：`web/champions.css:216-220` 给 `.mayhem-champions .champion-table th` 写死了 `nth-child(1..5)` 五档宽度，删列之后第 5 条变成死规则，前四档也要重新配（原来 32/100/36/52/58）。
- 样本不是不重要，只是不该占列宽——建议挪进胜率单元格的 `title`/tooltip，或者只留在右侧详情页的 overview 条里（`:754` 已经有「样本」这个 metric）。

**E2 高度不占满（海斗 + 斗魂都有）：**

```css
web/champions.css:105  .aram-champions   { position: sticky; top: 0; height: calc(100vh - 218px); }
web/champions.css:211  .mayhem-champions { position: sticky; top: 0; height: calc(100vh - 278px); min-height: 460px; }
```

那个 218 / 278 建模的是**页面没滚动时**页头占掉的高度。一旦向下滚动、页头滚出视口，sticky 元素钉在 `top: 0`，但**高度还是那个缩过的值**，于是底下空出 218 / 278px——正是你截图里的现象。

改法：高度必须由 sticky 偏移推导，别写死页头高度。例如 `top: 12px; height: calc(100dvh - 24px);`（用 `dvh` 顺带解决移动端地址栏），或者继续用 `100vh` 但配 `top` 计算。两个类都要改，斗魂那条还要同步 `:846` 的窄屏 `max-height: 430px`。

### F. 文案与版式（逐条对应你的清单）

| 你的要求 | 位置 | 改法 |
|---|---|---|
| 「推荐海克斯」改成「海克斯推荐」，和斗魂对齐 | `web/champions.js:1203` `<h3>推荐海克斯</h3>`；同行 `renderSourceCitation(citation, "推荐海克斯")`；`:1210` 占位名 `推荐海克斯 ${rank}` | 三处一起改。斗魂的基准写法在 `:937` `<h3>海克斯推荐</h3>` |
| 图标加在**大标题**，别加在小标题 | 去掉：`:776` 四个 `<h4><span class="mayhem-config-icon">◌/◈/✦/⌁</span>`；加到：`:1203` `<h3>海克斯推荐</h3>`、`:777` `<h3>开局配置</h3>`、`:782` `<h3>装备路线</h3>`、`:768` `<h3>装备排行</h3>` | 直接复用斗魂的 `.arena-section-icon`（`champions.css:749`，26px 圆角方块），**不要**再用 `.mayhem-config-icon`（21px，`:273`），两套图标样式没必要并存 |
| 「有些图标还没有」 | 同上 | 四个字形里 **`⌁`（U+2301）** 最可能缺字形——它不在 Microsoft YaHei / Segoe UI 的常用子集里，要回退到 Segoe UI Symbol 才有。**新图标只从已经证明能渲染的集合里选**：斗魂在用的 `✦`(U+2726) / `◈`(U+25C8) / `◇`(U+25C7) 在你截图里都正常显示 |
| 出门装 / 鞋子 / 召唤师技能一行，技能加点单独一行 | `champions.css:269` `.mayhem-opening-grid { grid-template-columns: repeat(4,minmax(0,1fr)); }` | 改 `repeat(3,minmax(0,1fr))`，技能加点那个 `<section>`（`js:777` 的第四块）加 `grid-column: 1 / -1; border-left: 0; border-top: 1px solid var(--line);`。窄屏断点 `:377-379` / `:390-391` 要跟着重排 |
| 技能加点里「技能介绍一行、加点顺序一行」 | `js:1370` `renderChampionSkillPlan` | 结构**已经**是 `.skill-priority`（图标行）+ `.skill-order`（顺序行）两个块级元素，问题只是被塞进 1/4 宽的列里挤到换行。上一条改完就自然成立。另外海斗这里没套 `.skill-plan` 外层，`gameplay.css:860` 的 `.skill-plan .skill-order { margin-top:10px }` 吃不到，补个包裹类 |
| 装备路线只展示前三个 | `js:781` `.slice(0, 5)` | → `.slice(0, 3)`（见 C 节） |
| 删掉「左侧数据来源 / 右侧查看原页面」那行 | `js:1216` `renderSourceCitation`、`js:1224` `renderOPGGCitation`；调用点 `:729 / :768 / :777 / :782 / :801 / :832 / :846 / :1203` | **全部删除**（已由你拍板，见下方⚠️）。两个函数连同 8 个调用点一起删干净，别留空 `<div>`；`champions.css:202-206` 的 `.mayhem-source-line` 规则同步删除，避免留死样式 |
| 图三文字 | `js:92` `"海克斯大乱斗 · OP.GG 梯度回退"`，渲染在 `:753` `<span>${mayhemSourceLabel(sourceLabel)}</span>` | 删掉这个 `<span>` |
| 图四文字 | `js:878` `<span>斗魂竞技场 · 全球样本${detail?.patch ? ` · ${patch}` : ""}</span>` | 删掉这个 `<span>` |
| 图五「英雄梯度」副标题 → 「胜率与海克斯 · 版本号」 | 海斗 `js:726` `<p>胜率与样本 · T1-T5 本地计算</p>`；斗魂 `js:856` `<p>全球样本</p>` | 两处统一成 `胜率与海克斯 · ${patch}`。海斗弹窗版 `js:737` 里还有一份同样的副标题，**三处一起改**，别漏 |
| 图六 斗魂 tab 副标题重写 | `js:661` `modeTab("arena", "斗魂竞技场", "海克斯 · 棱彩 · 吃鸡战绩")` | 候选：**「一命两人，八强争胜」** / 「双人开黑 · 逐轮淘汰」/ 「组合、海克斯与最后一名的距离」。第一个最贴模式本身（2v2v2v2、8 支队伍、共享一条命），也和隔壁 tab「英雄与全部海克斯」那种平铺直叙拉开了差别 |

> ⚠️ **署名：你已拍板全删（2026-08-22）。**
> 我原本建议「详情页底部保留唯一一条」，理由是我们用 hexdata 的前提是**按需查询 + 署名**，署名是许可条件本身。你的决定是**全部删掉**，照办。
> 一句话记在这里备查，之后不再提：**删完之后我们不再满足 hexdata 那条署名条件**，取数边界（按需、不打包、不预热、UA 诚实、只碰白名单文件）全部照旧不变。如果哪天想恢复，把 `renderSourceCitation` 挂回详情页底部一处即可。
> 实施细则：删 `renderSourceCitation`（`js:1216`）、`renderOPGGCitation`（`js:1224`）与全部 8 个调用点（`:729 / :768 / :777 / :782 / :801 / :832 / :846 / :1203`），以及 `champions.css:202-206` 的 `.mayhem-source-line`。**注意 citation 里的 `patch` 字段在别处还有用**（F 表里「英雄梯度副标题 → 胜率与海克斯 · 版本号」要拿它），删渲染函数不要顺手把上游 `citation` 数据结构一起删了。

---

## 三、优先级汇总

| # | 项 | 级别 | 位置 |
|---|---|---|---|
| A | **熔断器跳了就回不来**：`recordSuccess` 不清 `CircuitUntil`，无半开探测 → hexdata 永久失联（日志实证） | **P0** | `hexdata.go` 熔断状态机 + `hexdata-state.json` |
| A-bis | 304 陷阱：校验器与正文生命周期不一致（**日志未验证，同轮硬化**） | P1 | `hexdata.go` 取数客户端 + `pruneDisk` |
| A-diag | 跳闸点零日志；84 次熔断命中只有 27 条 fallback | **P0**（同 A 一起做） | `hexdata.go` 全部 `recordShapeFailure` / 短路点 |
| B | 模式判据按图标路径 → 漏掉 105/208 个海斗海克斯 → 名字/图标/品质色三缺 | **P0** | `hexdata.go:1018-1039`；`champions.js:1198,923`；`champions_structured.go:967` |
| C | 装备排行 = 装备路线，且无 Score/WinRate/Games | **P0** | `hexdata.go:1096-1112,1250`；`champions.js:781` |
| D | 斗魂说明 `@Name*K@` / 缺 key / 跨技能引用 / **取值索引改 index 1（19 处已定案）** | P1 | `champions_structured.go:805,826,835,790` |
| E | 样本列 + 列表高度不占满（两个模式） | P1 | `champions.js:1119,1142`；`champions.css:105,211,216-220,846` |
| F | 文案与版式 12 条 | P1 | 见上表 |
| G | RSC 16000 字符窗口硬切 | P2 | `hexdata.go:1216-1248` |
| H | hero 形状埋点写死 16/16（跨轮遗留） | P2 | `hexdata.go:1066` |
| I | 斗魂 45 个海克斯里稳定 2 个 `missingMeta`（日志新发现） | P2 | `champions_structured.go` 目录匹配 |

---

## 附：红线复核

本工单不改变任何取数边界。实施时保持：

- hexdata 只碰 `/api/hexdata/answer-cards` 这一个白名单文件，不碰 `/api/` 其余路径与 `/data/`；
- 不打进 EXE、不做启动预热与定时同步；
- UA 保持诚实，不伪装；
- hexdata 请求链路不得出现 PUUID / summonerId / 召唤师名；
- 上游字符串（海克斯名、说明、citation URL、RSC 里新捞的图标 URL 与英文名）一律经 `escapeHTML` 才进 innerHTML——**B3 从 RSC 新捞图标 URL、D 节重写占位符渲染，都是新增的字符串链路，尤其要确认没绕过转义**；
- 变异测试必须 `cp` 真副本（软链接无效，`__dirname` 会解析 realpath），副本要带上与 `web/` 同级的仓库根 `*.go`。

**唯一一处边界变动：** 按你 2026-08-22 的决定删除全部 hexdata / OP.GG 署名（F 表最后一行）。取数行为本身不变，变的只是我们不再展示来源署名。
