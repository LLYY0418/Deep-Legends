# WORKLIST-R104 — 冷启动烧光额度（我 R102 设计错的）+ 图标被"重渲染"误判成失败永久变白 + 额度耗尽时页面纯空白

日志：`lol-loot-diagnostics-0917-1526.jsonl`，build_fingerprint `81d76436f8cc`，version 0.12.1。
两次运行：**运行A `b953a7fd`（07:16:39 启动，就是用户截图那次）**、运行B `a4e1dc90`（07:23:06，重启后）。

---

## ★★★ 先说清楚责任：P0 又是我工单写错造成的

用户问"现在韩服玩家我查询了才 2-3 个就出现额度不够，额度这么低吗"——
**答案是：额度没变，是我在 R102 里设计的功能把额度在你搜索之前就烧光了。**

我在 WORKLIST-R102 里写的原话大意是：
> "现有种子解析的 3 秒 context 超时是为单账号 2 次 Riot 调用设计的，33 人多账号用固定 3 秒
> 必然系统性截断后面的账号……按账号数线性放大预算。"

我当时只盯着"别把后面的账号截断"，**把那个固定 3 秒超时当成 bug 给拆了。
但那 3 秒恰恰是当时唯一在限制这个后台任务能烧多少配额的熔断器。**
我把熔断器拆掉、改成"按账号数线性放大"，等于给了它近 10 分钟的许可去打满配额。

**更糟的是：我在 R100 工单里刚刚亲手算过这笔账**（"一次 20 场搜索精确消耗 25 个请求
⇒ 每 2 分钟只能搜 4 个玩家"），一轮之后设计新功能时**完全没有把这个数字套上去**。
这不是执行方的问题，执行方严格按我写的实现了。

---

## P0（★★★ 最高优先）每次冷启动向 Riot 发约 390 个请求，把用户的额度全部吃光

### 账怎么算的（代码实证）

`pro_players.go:269-278` 后台发布流程末尾：

```go
seedCtx, seedCancel := context.WithTimeout(context.Background(),
    time.Duration(proSeedTotalAccounts())*proSeedPerAccountBudget)   // 53 × 4s = 212 秒
seeds = a.loadProSeeds(seedCtx, previous)
seedCancel()
...
activityCtx, activityCancel := context.WithTimeout(context.Background(),
    time.Duration(proActivityAccountCount(completed))*proSeedPerAccountBudget)  // ~90 × 4s = 360 秒
a.enrichProActivity(activityCtx, completed, previous)
activityCancel()
```

**种子账号侧**（`pro_seed_accounts.go:129-186`），`proSeedTotalAccounts()` = **53 个账号**（33 名选手）。
每个账号冷路径要发：

| 调用 | 位置 | 缓存 TTL | 冷启动请求数 |
|---|---|---|---|
| `resolveProSeed` → `accountByRiotID` / `fetchAccountByPUUID` | `pro_seed_accounts.go:96-118`、`riot_api.go:738` | 锚点 30 天；by-puuid **6 小时** | 1 |
| `proSeedRank` → `/lol/league/v4/entries/by-puuid/` | `pro_seed_accounts.go:200-213` | **6 小时**（未定级 72 小时） | 1 |
| `lastMatchStart` → `matchIDsFiltered` + `matchByIDWithCache` | `pro_activity.go:10-34` | **6 小时** | **2** |

⇒ **53 × 4 = 212 个请求**。

**非种子账号侧**（`pro_activity.go:44-94` `enrichProActivity`）：对目录里**每一个**已核对、
带 PUUID、`Source != "seed"` 的账号再做一次 `lastMatchStart`（2 个请求）。
本次日志 `pro_identity_match {"candidates": 143}` ⇒ 非种子约 90 个 ⇒ **再约 180 个请求**。

### ⇒ 合计约 390 个请求，而本地预算是 90 次 / 2 分钟

`riot_api.go:285-288` `shortLimit=15/秒`、`longLimit=90/2分钟`。
**390 ÷ 90 × 2 分钟 ≈ 8.7 分钟的配额被一个后台装饰性任务 100% 占满。**
这段时间里用户的任何搜索都只能排队或直接被拒。

### 日志逐条对得上

- `07:16:39.662` `app_start`
- `07:17:24.204` — **同一瞬间 126 条 `riot_local_rate_limited`**，`retry_after_seconds: 88`
  ⇒ 88 秒后最老令牌才过期 ⇒ 最老令牌是在 `07:18:52 − 120s = 07:16:52` 消耗的
  ⇒ **启动后第 13 秒开始，到第 45 秒，90 个长窗口令牌全部烧完**，此时用户还什么都没做。
- `07:17:24.355` `pro_directory_cost {stage:"ladder", duration_ms:40862, stage_duration_ms:38426,
  wait_after_supplements_ms:36502}` —— 这一段 R100 时实测是 5.8 秒，**现在是 40.8 秒**。
- 之后用户自己的查询全程挨饿：
  - `07:17:25` `matches_failed:15, rate_limited_count:15`（只出来 5 条）
  - `07:19:01` `limiter_queue_ms:49142`
  - `07:19:03` `limiter_queue_ms:80067, matches_failed:7`
- 全会话 `riot_local_rate_limited` 共 **168 条**。

### 为什么"重启一下好像好点"不能当成没事

运行B（`a4e1dc90`，07:23:06 启动）**零限流、连 `pro_directory_cost` 都没有**——
因为它在几分钟内重启，R100-P3 的 24 小时快照还新鲜、而且 6 小时的 rank/lastmatch 缓存全热。
**这是时间上的巧合，不是防护。** 由于 rank 和 lastmatch 的 TTL 都是 6 小时，
**距离上次启动超过 6 小时的每一次启动（也就是每天第一次打开软件）都会完整重演这 390 个请求。**

### 修法（四条，1 和 2 是必须的）

**1）限流器必须区分前台/后台优先级——这是结构性缺口。**
R98 引入的 FIFO 队列让前台和后台**严格公平**，而这里恰恰要求**不公平**：
装饰性后台任务永远不能把用户正在等的搜索挤掉。
- 给 `wait()` / `enterRiotLimitQueue` 增加优先级类别：**foreground（用户可见的总览/搜索）**
  与 **background（职业目录、ladder、种子、活跃度）**。
- **background 只能使用长窗口的剩余配额，并且要给 foreground 留固定保留额**：
  建议 background 在 `len(longWindow) >= 30`（即长窗口已用掉 1/3）时就一律让路/放弃，
  **而不是等到 90 打满才失败**。foreground 仍可用到满额。
- ★**措辞消歧**（吸取 R98/R100 教训）：上面这个 30 是**"长窗口里已消耗的令牌数"这一个瞬时计数**，
  不是"background 每 2 分钟最多 30 个请求"的累计配额，也不是时间预算。
  按另一种读法（给 background 单独记 30/2 分钟的桶）实现的话，
  它仍然会在用户搜索的同时稳定吃掉 1/3 配额，**达不到"用户搜索时后台完全让路"的目标**。
  如果你认为累计桶更好实现，请先在账本里写明再做。

**2）把职业活跃度改成"细水长流"，不要开机一次性拉满。**
- **不要在启动流程里同步跑 `loadProSeeds` 的 Riot 部分和 `enrichProActivity`。**
  启动时只发布快照（`pro_players.go:221` 那个 `new(app).loadProSeeds` 本来就不带 Riot，保持原样）。
- 活跃度改成独立的后台涓流任务：**每分钟最多 N 个账号**（建议 N=3，即 6 个请求/分钟），
  按"最久没刷新"排序轮询，390 个请求摊到约 1 小时慢慢补齐。
- **启动后前 60 秒完全不发 background 请求**，把最容易被用户撞上的窗口留干净。
- ★这里的"每分钟最多 3 个账号"是**累计速率**（与第 1 条那个瞬时计数不同），请注意区分。

**3）把活跃度结果的 TTL 从 6 小时拉长到 7 天。**
`pro_activity.go:15` 和 `pro_seed_accounts.go:203` 现在都是 6 小时。
"哪个小号最近在打"这件事变化很慢，**6 小时的刷新频率和它的价值完全不匹配**，
却正好卡在"每天第一次开软件必然全量重拉"的节奏上。
- `proseed-lastmatch:v1:` → **7 天**
- `proseed-rank:v1:` → 建议 **24 小时**（未定级仍走现有 72 小时退避）
- ★`proSeedRankTTL`（`pro_seed_accounts.go:196-201`）是现有函数，改它会同时影响
  `cachedPublicIdentityTTL` 的调用点，只有 `proSeedRank` 一处（已全仓核对），可以放心改。

**4）把 `proSeedPerAccountBudget` 的线性放大拆掉，改回有界总预算。**
`pro_seed_accounts.go:119` `const proSeedPerAccountBudget = 4 * time.Second`，
现存调用点**三处，必须逐个处理，不能只改一处**：
- `pro_seed_accounts.go:122` `proSeedContext(ctx, count)` —— `count × 4s`
- `pro_seed_accounts.go:158` 单账号 `context.WithTimeout(seedCtx, proSeedPerAccountBudget)` —— 这个保留
- `pro_players.go:269` `proSeedTotalAccounts() × 4s = 212s` —— **必须改**
- `pro_players.go:276` `proActivityAccountCount() × 4s ≈ 360s` —— **必须改**
- `pro_activity.go:81` 单账号 4 秒 —— 这个保留

单账号 4 秒保留是对的（那是"一个账号最多等多久"）；
**要改的是整轮的总时长许可**，改成涓流任务后它天然由"每分钟 N 个"控制，
`pro_players.go:269/276` 这两个整轮 context 应该直接删掉或收到很小的值。

### 本项验收要求

- 必须有一条测试断言：**模拟冷启动（所有缓存为空），统计整个后台职业流程在头 2 分钟内
  实际发出的 Riot 请求数 ≤ 某个明确上限（建议 ≤ 12），且 foreground 的一次 20 场搜索
  在后台任务运行期间仍能拿到 20/20 场、零 `errThrottled`。**
- ★这条测试在"把 background 优先级判据删掉"和"把涓流速率改回一次性全量"两种变异下都必须真实失败。
- 补埋点：`enrichProActivity` 和种子流程现在**完全没有请求计数埋点**，
  请各补一条含 `accounts_total` / `accounts_refreshed` / `riot_requests` / `skipped_by_budget` 的事件，
  否则下次再出这种问题还是只能靠推算。

---

## P1（★★★）国服/韩服英雄头像、装备图标大面积变白：图标队列把"页面重渲染"误记成"加载失败"

### 这不是后端问题

日志 `asset_fetch {"host":"ddragon.leagueoflegends.com","requests":102,"failures":0}`
—— 服务端取图 **102 个请求零失败**。问题全部在前端队列里。

### 根因：取消 = 记失败 = 毒化 URL 60 秒 = 永不重试

`web/image-queue.js`：

```js
// scan()：元素不在文档里了就取消
for (const [img, cancel] of active) if (!connected(img)) cancel();
// 而 cancel 就是"当成失败"
active.set(img, () => { img.removeAttribute("src"); finish(true); });
// finish(true) 把这个 URL 拉黑 60 秒
if (error) { failed.set(url, Date.now() + 60000); ... }
```

⇒ 一张图**正在加载途中，仅仅因为列表重渲染把它从 DOM 摘掉**，
就被记成一次真实的网络失败，**该 URL 被拉黑 60 秒**。

然后 `pump()` 里对被拉黑的 URL：

```js
pending.delete(img);
seen.set(img, url);
if ((failed.get(url) || 0) > Date.now()) continue;   // ← 直接丢弃
```

—— `pending` 已删、`seen` 已写，而**没有任何定时器在冷却结束后重新入队**；
之后 `enqueue()` 又会因为 `seen.get(img) === url` 直接 return。
**这张图从此再也不会加载。**

### 我已经在 jsdom 里用生产文件复现了

直接加载 `web/image-queue.js`，3 张图共用同一个 URL，只做普通重渲染、**零真实网络失败**：

```
frame1 rendered: imgs=3 withSrc=2
frame2 re-render: imgs=3 withSrc=0
frame3 re-render: imgs=3 withSrc=0
after settle:    imgs=3 withSrc=0
FRESH IMG src after poison window: null  => POISONED: new images get NO src, stay blank
```

**两次重渲染之后，再也没有任何一张图拿得到 `src`。**

### 为什么这一版特别严重、而且会自我维持

- 英雄头像在一页战绩里**大量共用同一个 URL**，毒化一次 = 这个英雄全页面变白 ⇒ 正好是图一/图三
  （头像退化成单字占位符、装备格全黑）。
- R98/R100 把总览改成渐进流式（5/7/9/11/13/15/17/19/20 场 ⇒ **一次加载重渲染 9 次以上**），
  重渲染频率远高于图片加载完成的速度。
- P0 的额度风暴让每次加载拉长到 10 秒以上，**在途窗口被最大化**，毒化概率进一步上升。
- ★**自我维持**：每次新渲染产生的新 img 会被下一次渲染取消 ⇒ 再次毒化同一批 URL
  ⇒ 只要页面在动，头像可以一直白下去。

### 修法

1. **区分"取消"和"失败"。** `cancel()` 必须走一条不写 `failed` 的路径
   （例如 `finish(false, {cancelled:true})`，或者直接 `active.delete(img)` 后不做失败记账）。
   ★**只有 `img` 真的触发了 `error` 事件、或真的超时，才允许写 `failed`。**
   元素被摘出 DOM 不是失败。
2. **冷却到期必须能自愈。** 命中 `failed` 冷却时不要 `pending.delete` + 丢弃，
   应保留在队列里并安排一个到期后重新 `pump()` 的定时器；
   或者至少不要 `seen.set(img, url)`，让后续 `scan()` 能重新入队。
   ★现在这两条叠在一起才导致"永久变白"，**只修其中一条不够**。
3. **同 URL 的失败冷却不应该惩罚新元素到这个程度。** 建议把 60 秒降到 10 秒，
   并且**冷却期内只跳过重试、不要把元素标记成已处理**。
4. `limit = 2` 这个并发上限在 R100 是为了防 HTTP/1.1 六连接被占死，**不要直接调大**。
   但可以考虑：**本地/已缓存命中的图标不占用这个名额**，或把上限提到 3 并配合真 Chromium 复测连接数。
   ★改这个值必须重跑 R100 的真 Chromium 连接池验证（`desktop/r100-browser.cjs`），
   断言峰值同源图片连接仍 ≤ 2~3 且 `/api/status` 仍在 8 秒内返回。

### 本项验收要求

- **必须有一条 jsdom 回归测试**：渲染 N 张共用同一 URL 的图 → 在图片仍在途时重渲染 2~3 次
  → 断言**最终仍有图片成功拿到 `src` 并触发 load**，且该 URL **没有**进入 `failed`。
  ★这条测试在"把 cancel 改回记失败"的变异下必须真实失败。
- 另加一条：人为让某个 URL 真实 error 一次 → 断言冷却期结束后**会自动重试**（不需要 DOM 变化触发）。
- 真 Chromium 侧：复跑 `desktop/r100-browser.cjs`，确认连接池护栏没被这次改动破坏。

---

## P2（★★）额度耗尽时新开的页签渲染成一片纯空白（图二）

`web/gameplay.js:1649-1651`：

```js
if (!tab.data && !tab.error) {
  container.innerHTML = quotaMessage || '<div class="gameplay-skeleton">…</div>';
  return;
}
```

额度耗尽时前端把 429 转成 `tab.quotaRetry` 自动重试状态（**不是** `tab.error`），
于是对一个**全新、还没有任何数据**的页签：`tab.data` 为空、`tab.error` 为空
⇒ `quotaMessage` 为真 ⇒ **它把骨架屏整个替换掉**，页面只剩一行字和一片空白。

日志实证——三次"连身份都没拿到"的空加载：
- `07:17:57.0739` `matches_requested:0, rate_limited_count:2`
- `07:18:41.6637` `rate_limited_count:1`
- `07:18:41.6644` `rate_limited_count:2`

`spans` 里 `account/summoner/matchIDs` 全是 `[[0,0]]` ⇒ **连 `accountByRiotID` 都被本地限流拒了**，
整轮没有任何数据返回。

**修法**：把 `||` 改成"提示条 **+** 骨架屏"，即 `quotaMessage + skeleton`，
让用户看到"正在等额度恢复 + 正在加载"的完整状态，而不是一片空白。
★同时确认：这个分支只在 `!tab.data` 时走，**已有数据的页签必须继续显示旧数据**
（这是 R98/R100 反复守的语义，不要在这里退回去）。

### 本项验收要求
- 断言：`tab.data` 为空 + `quotaRetry` 存在 ⇒ 容器里**同时**包含额度提示文案和骨架屏元素。
- 断言：`tab.data` 有值 + `quotaRetry` 存在 ⇒ 旧数据仍然渲染，不被提示条替换。

---

## 关于"必须换 production key 吗"（用户又问了一次）

**这次的额度问题跟 key 的档次无关，是我们自己在启动时把额度花光了。**
个人密钥 100 次/2 分钟（本地自限 90）本来足够单人用：一次 20 场冷搜索 25 个请求
⇒ 正常情况下每 2 分钟能搜 3~4 个新玩家。P0 修好之后单人体验会恢复。

但**多人分发仍然必须换 production key**，这一条 R100 的结论不变：
EXE 内嵌的是同一个个人密钥、全体用户共享同一个桶，且 Riot 明文禁止用个人密钥做公开分发。
**拿到 production key 之前不要对外发。**

---

## 执行顺序

1. **P0**（额度）—— 这是"韩服完全没法用"的主因。
2. **P1**（图标毒化）—— 这是"图标全白"的主因，且它和 P0 互相放大。
3. **P2**（空白页）—— 一行改动，但直接决定额度恢复期间界面还能不能看。

## ★本轮明确不做的事

- 不动默认战绩条数（保持 20 条）。
- 不上调 `shortLimit=15/秒`、`longLimit=90/2分钟` 的数值——
  本轮要做的是**给后台任务降级让路**，不是抬总量。
- 不把韩服战绩改走 OP.GG。
- **不要为了省请求去掉"按最近对局时间排序"这个功能本身**——用户 R102 要的就是这个，
  要改的是它的**取数节奏**，不是把功能砍掉。
- 不要直接调大 `image-queue.js` 的 `limit`（除非重跑真 Chromium 连接池验证）。
- 不要为了让测试变绿删或放宽任何现有断言。

## ★给执行方的一句话提醒

本轮 P0 又是**我的工单写错**造成的（我把一个当时在起熔断作用的固定超时当成 bug 拆了，
还把它"线性放大"，等于给后台任务发了近 10 分钟打满配额的许可）。
**如果本工单里还有任何一句话你觉得有两种读法、或者按字面实现会导致明显不合理的资源消耗，
请先停下来在账本里写明你的疑问和你选择的读法，不要闷头按字面做完。**
特别是 P0 第 1 条和第 2 条里出现了两个都叫"预算"但含义不同的数字（瞬时计数 vs 累计速率），
我已经在原文标注，如果仍然不清楚请先问。
