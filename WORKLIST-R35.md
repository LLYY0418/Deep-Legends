# WORKLIST-R35（给 GPT 执行）：R34 验收发现的两个连带缺陷

> 本轮 R33-F 组 + R34（G/H/I 组）**全部条目都正确落地了**，`go build`/`go vet`/`gofmt` 干净，
> Go 579 / web 156 / desktop 32 测试全绿，我另跑的 6 个变异体全部被现有测试杀死、0 存活。
> **下面两条不是"工单没做"，是"工单做对了、但引入了工单没预料到的副作用"**，
> 都由我在验收时实测发现，两条都是 P1。

---

## A 组（P1）：`playerRankScore` 缓存读写用了不同的键，10 分钟 TTL 缓存完全失效

### A-1 真根因（实测，不是推理）

`rank_insights.go`：

```go
preferredSource := "unknown"
if len(decision.Sources) > 0 {
    preferredSource = decision.Sources[0]          // ← LCU 优先后这里恒为 "lcu"
}
cacheKey := rankScoreCacheKey(preferredSource, serverID, playerRef)
if entry, ok := a.rankScores.get(cacheKey); ok {   // ← 用「首选源」读
    return entry
}
...
a.rankScores.put(rankScoreCacheKey(capabilitySource(capability), serverID, playerRef), entry)
                                   // ↑ 用「实际服务的源」写
```

F-3 之前，"首选源 == 实际服务源"几乎恒成立，所以这个不对称一直没暴露。
**F-3 恰恰把「LCU 优先、但 LCU 拿不到完整负场时由 SGP 实际服务」变成了常态**，
此后条目写进 `sgp:HN1|xxx`，而每次读都去查 `lcu:HN1|xxx`，**永远 miss**。

**实测证据**：我在仓库副本里写了个测试直接跑真实 `playerRankScore`
（复用你们自己 `gameplay_test.go:956 TestRankedStatsFallBackToSGPWhenLCUFails` 的
LCU 503 + SGP 正常那套 httptest fixture），同一个玩家连查两次：

```
lcuCalls=2 sgpCalls=2      ← 缓存生效的话第二次应该还是 1/1
CACHE MISS: second lookup re-queried SGP (1 -> 2)
```

**影响**：一张 10 人战绩卡每次重绘都会把 10 个人的 LCU + SGP 请求**全部重打一遍**，
`rankScoreCacheTTL = 10 * time.Minute` 这层缓存等于不存在。方向是安全的
（不会串读到别的源的数据），但性能上等于把 H-P1-1 那轮去重的收益又还回去了一部分。

### A-2 改法

两种都可以，选一种即可，但要写清注释说明为什么：

- **方案 1（推荐）**：写入时**两个键都写**——既写 `capabilitySource(capability)` 那个键
  （保持"不同源结果绝不串读"这条不变式），也写 `preferredSource` 那个键。
  后者可以存一个带 `source` 字段的 entry，读到时如果 `entry.source != preferredSource`
  也仍然可用（因为它记录的正是"以 preferredSource 为首选时最终得到的结果"）。
- **方案 2**：统一用 `preferredSource` 做缓存键，把「实际来源」放进 `rankScoreEntry`
  的一个新字段里而不是放进键里。这样键的语义变成"这次查询的决策入口"，
  而不是"数据来自哪"，读写自然对称。**注意**：选这个方案时必须确认
  `decision.Sources[0]` 对同一玩家在同一时刻是稳定的（它只依赖
  `LCUConnected`/`RemoteServer`/`SGPAvailable`，是稳定的），并把这个前提写进注释。

**不要**简单地把写入键改成 `preferredSource` 了事而不做任何记录——
`rank_insights.go:189-190` 那条"LCU 与 SGP 结果绝不能互相串读"的注释是对的，
它保护的不变式必须继续成立。

### A-3 变异测试（必须做，这是本条唯一有意义的护栏）

现有的 `data_sources_test.go:77-80` 只测了"两个源的键不碰撞"，**没测读写对称性**，
所以这个缺陷才能通过全部 579 个测试。要补的测试：

用 LCU 返回 503、SGP 正常返回的 fixture（直接抄 `TestRankedStatsFallBackToSGPWhenLCUFails`
那套 httptest server），对同一 `playerRef` 调用两次 `playerRankScore`，
**断言第二次调用没有产生任何新的上游请求**（`lcuCalls`/`sgpCalls` 计数在两次调用间不变）。
把 A-2 的修复改回现在这样（读写键不同），这个测试必须 failed。

---

## B 组（P1）：qq101 出装数据没有场次字段，低样本 100% 胜率在界面上裸奔

### B-1 事实

先说清楚：**`-1` 哨兵值你们处理对了**（`qq101.go:363` 的 `len(fields) != 4` 会跳过，
`itemID <= 0` 也有兜底），冷门组合的兜底也有测试覆盖，这部分没问题。
覆盖率我也补测了（本来是 I-3 第 1 步探测该拿的数据）：
9 组冷门英雄/分路/段位组合全部 HTTP 200 有数据，只有"瑞兹打野"这种基本不存在的组合返回 `-1`。
**覆盖率不是问题，数据置信度才是问题：**

真机 curl 冷门组合拿到的原始数据：

```
大发明家(74)/辅助/emerald+   sixth_details: "3102_1_100.0_100.0"   ← 选取率100% 胜率100%
瑞兹(13)/中单/最强王者(10)   forth_details: "3089_1_100.0_0.0"      ← 胜率 0%
```

这些显然是 1~2 局的样本。而 **qq101 的 `_build` 载荷里根本没有 games 字段**
（符文 `_runeinfo` 是有场次的，出装没有），所以 `parseQQ101BuildRows`
构造 `championMetricRow` 时 `Games` 恒为 0，前端 `web/champions.js:1795 renderDepthStats`
的"场次"格 `compactNumber(0)` 渲染成 `—`。

**界面最终会出现「胜率 100% / 场次 —」**，而且用户完全无从判断这是 1 局还是 1 万局。

### B-2 为什么这条必须修：这是老坑换个数据源原样复发

[[deep-legends-r31-live-build-sort-and-lowsample]] 记录过 R31 那次真根因——
"男枪没闪现"是因为对局页按胜率排序把**只有 2 场的 50%** 顶到了第一。
当时的结论是低样本必须能被识别。现在换成 qq101 之后，**连"有多少场"这个信息本身都没有了**，
比 R31 那次更糟：op.gg 那条路径是有场次的（`web/champions.test.cjs:445`
明确断言渲染出 `场次 18`），换源之后这一列静默变成 `—`。

### B-3 改法（二选一或都做）

1. **显式标注数据来源与其局限**：qq101 来源的出装行，"场次"那一格不要渲染成 `—`
   （会被误读成"没有数据"），改成明确的"腾讯官方数据不提供样本量"之类的提示
   （可以做成一个小 icon + tooltip，不要占太多空间）。**这条是底线，必须做**。
2. **对明显的低置信行做过滤或降权**：`pickRate` 或 `winRate` 恰好等于 `100.0` 或 `0.0`
   的行，几乎必然是个位数样本。可以直接不展示，或者展示但加一个"样本极少"的角标。
   **注意别做成 R31 那种整块 opacity 变灰**（那次是做错了、已经全删），要用明确的文字或角标。

### B-4 变异测试

补一个 fixture：qq101 build 返回 `"3102_1_100.0_100.0"` 这种单条 100% 的数据，
断言渲染结果里**要么这一行不出现、要么带上了低置信/无样本量的标记**，
并且断言"场次"格不再是裸的 `—`。把 B-3 第 1 点改回原样（直接渲染 `—`），测试必须 failed。

---

## C 组（说明，不用改代码）：I-3 的"先只读探测"这一步被跳过了

工单 I-3 第 1 步写的是"先做只读探测，不改现有展示"，第 2 步才是"探测通过后切换数据源"。
实际交付里两步一起做完了——`champions_structured.go` 已经把 qq101 接进生产响应
（`build.ItemSource = "QQ101"`），前端 `web/champions.js:1434` 也已经渲染分路占比。

**这不算越权**（第 2 步本来就在同一份工单里被批准了），而且兜底、feature gate、
超时重试都做到位了，所以我不要求回滚。但要说清楚代价：**跳过探测就等于跳过了
"用真机数据回答覆盖率/延迟/置信度三个问题"这一步**，上面 B 组那个缺陷正是这一步
本该提前发现的东西（我事后补做了 curl 探测才发现）。

**以后凡是工单里写了"先探测再切换"的，请按顺序做**，或者如果判断可以合并，
在交付说明里明确写一句"合并了第 1、2 步，理由是 X，探测本该回答的问题我用 Y 方式验证了"。

---

## 交付要求

- `go build` / `go vet` / `gofmt` 干净；Go 与 JS 测试全绿。
- A-3、B-4 两处各写**至少一个真变异测试**（改回旧行为 → 测试必须 failed → 复原变绿），
  变异结果贴在提交说明里。
- A 组改完后，请顺手确认另外几个用 `sourceScopedKey` 的地方
  （`opgg_insights.go`、`season_stats.go`、`overview_cache.go`、`sgp_api.go`、`match_timeline.go`）
  **没有同类的"读写键不对称"问题**——判据是：写入时用的 source 和读取时用的 source
  是不是同一个表达式算出来的。是就没问题，不是就要一并修。
