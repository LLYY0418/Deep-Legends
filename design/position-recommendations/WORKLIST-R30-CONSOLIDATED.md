# WORKLIST-R30：R26 + R27 + R28 合并复核（08-27 二次确认，全部仍未执行）

这份文档合并原来的三份工单（`ACCEPTANCE-R25-WORKLIST-R26.md` 的第 2/3 节、
`WORKLIST-R27.md`、`WORKLIST-R28.md`），去重后按优先级重排。**合并前逐条重新读码
核实过当前状态**（08-27 下午），结论：**三份工单里能核实的条目一条都没有被执行**——
这段时间 GPT 实际只做了 R29（收藏页库存仲裁死循环，已闭环，见 `ACCEPTANCE-R29.md`）。

以下 file:line 均为本次重新核实的结果，不是照抄旧工单。

---

## A.【P0】优劣势对抗数据变少 —— 根因两处都没改

`champions_structured.go:308-318` 和 `:995-1007` 两处**依然**用
`payload.Data.Summary.Positions[X].Counters`（3 条摘要）覆盖
`payload.Data.Counters`（27~30 条完整列表）：

```go
counterValues := payload.Data.Counters
if spec.PositionMode == opggPositionRequired {
    for _, raw := range payload.Data.Summary.Positions {
        if strings.EqualFold(...) && len(raw.Counters) > 0 {
            counterValues = raw.Counters   // ← 两处都还在
        }
    }
}
```

改法：两处都删掉这段覆盖逻辑，直接用 `payload.Data.Counters`；顶层为空时走空态，
不回退到 positions 摘要（摘要与请求位置无关）。

**护栏**：fixture 顶层 30 条 / positions[TOP] 3 条，断言两处调用点的输出都基于 30 条；
强弱划分（`weakCount`/`strongCount`）用 30 条重新验证 5 强 5 弱。

---

## B.【P0】出装：删 lolalytics + 回归 op.gg + 去重 + 加回第六件

### B.1 lolalytics 依然完整存在，一行没删

`champions.go:31` host 仍是 `a1.lolalytics.com`；`:609-619` 浏览器 UA/Origin/Referer
伪装分支仍在；`lolalytics.go`/`lolalytics_test.go` 整个文件都在；
`champion_cache.go` 仍有 4 处 `lolalytics` 相关代码（含 R26 记录过的死配置：
缓存键 `v1|lolalytics-itemset|...` 与白名单 `v1|a1.lolalytics.com|` 前缀不匹配，
磁盘缓存本来就不生效）；`champions_structured.go:950` `shouldUseLolalyticsItems`
分支仍是第四/五件的默认优先路径。

**已定案（用户拍板过）：国内环境 lolalytics 100% 403，删除整条链路，回归 op.gg。**

### B.2 第六件装备：**depth 6 解析器根本不存在，不是"透传"这么简单**

之前工单以为"depth 4/5/6 已经在解析、只需要把 6 透传出去"——**这个前提现在不成立**。
实际读码 `champions_structured.go:405-452` `parseOPGGDepthRows`：

```go
for _, depth := range []int{4, 5} {   // ← 只有 4、5，从未有过 6
```

全仓库搜不到任何 `SixthItems`/`第六件`字段（Go 和 `web/champions.js` 都没有）。
**这是一项新增解析工作，不是"加回"**：需要确认 op.gg RSC 页面里 `depth_6_item_*`
这个字段是否存在（先用 curl/真实页面验证，不要假设），存在的话把循环范围改成
`[]int{4, 5, 6}`，Go 端补 `SixthItems`/`SixthSample` 字段，前端
`web/champions.js:1678-1680` 的 `groups` 数组补第三行。

### B.3 去重：只剔核心装，不跨列（R26 实测过的方案，直接照做）

只剔除 `core_items[0]` 三件，不跨列去重（第五件仍可含第四件出现过的 ID）。
实测（KR/emerald_plus）剔除后每列至少剩 3~5 行，不会出现空集。

### B.4 护栏

| # | 测试 | 必须能杀死的变异 |
|---|---|---|
| B-1 | lolalytics 已删干净 | 全仓 `grep -r lolalytics` 为 0（不要只查函数名，见 R26 P1-4 教训） |
| B-2 | depth 6 真的被解析 | fixture 断言 `SixthItems` 非空；把循环范围改回 `{4,5}` → 挂 |
| B-3 | 去重只剔核心装 | 断言第五件可含第四件已出现的 ID；改成跨列剔 → 挂 |
| B-4 | 三列都非空 | 5 个真实英雄 fixture，每列 ≥3 行 |

---

## C.【P0】绝活哥符文缓存键碰撞 —— 会返回错误英雄的数据

`specialist_runes.go:48-53` 仍是：

```go
func specialistRuneKey(championID int64, position string) int64 {
    ...
    if ordinal == 0 { return championID }
    return championID*10 + ordinal
}
```

穷举 1..1000 英雄 × 6 位置，共 **495 组碰撞**（例：奥拉夫·打野 == 艾希·无分路 == 键 22）。
改成字符串键 `fmt.Sprintf("%d|%s", championID, position)`，并用现有但从未参与校验的
`specialistRuneCacheEntry.position` 字段做二次比对。

**护栏**：把穷举碰撞检测写成可执行测试（1..1000×6 组合，断言键两两不同或严格绑定 championID+position）。

---

## D.【P0】低样本角标：管道通了，末端没接

`web/champions.js:1678-1679` 解构出 `fourthSample`/`fifthSample`，但模板里从未拿来加
`is-low-sample` 类（对比 `:1111` 海克斯、`:1351` 斗魂已经在用这个类，抄一下就行）。
不接的后果：18 场/22% 胜率的推荐和上万场的推荐视觉上没有任何区别。

**护栏**：构造样本数 <100 的 fixture，断言渲染结果 class 含 `is-low-sample`；去掉判断 → 挂。

---

## E.【P1】海克斯图鉴部分图标缺失

`main.go:732-750` `communityDragonImagePaths` 仍然只返回 2 个候选路径，
没有加 `_large.png → _small.png` 的降级候选。穷举实测 657 个海克斯里 60 个
上游只有 `_small` 没有 `_large`，服务端加两个候选路径即可一次修好
（细节见 memory `deep-legends-augment-icon-missing-large`）。

**护栏**：断言 `communityDragonImagePaths(".../xxx_large.png")` 返回 4 条且含 `_small.png`
降级、large 优先于 small；用清单里的 `Drop_Bear`/`Mercy` 做真实 fixture。

---

## F.【P1】三处排位队列切换联动 + 文字提示 + 灵活组排空数据

### F.1 联动仍未解耦

`web/gameplay.js:15` `rankedQueue: "420"` 仍是三个切换器共用的单一状态，
`:2486-2487` 点任一处仍改同一个 `tab.rankedQueue`。改法：拆成
`rankedQueueRecent`/`rankedQueueAbility`/`rankedQueuePosition` 三个独立字段，
`rankedQueueData(data, tab, which)`/`rankedQueueSwitcher(tab, activeQueue, which)`
按 `which` 取值，事件处理按 `data-ranked-queue-scope` 只更新对应的一个。

### F.2 队列切换器前的文字提示、正则注入 HTML 的隐患

`addRankedQueueTools`（约 `:788-790`）仍用正则
`(<header><h3>[^<]*</h3>)(<span>[\s\S]*?</span>)(</header>)` 往 header 里塞切换器，
失配时静默返回原串。`renderAbility`/`renderRecentRanked`/`renderPositionStats`
都已声明 `queueSwitcher` 形参但从未使用——把切换器作为参数直接传入模板渲染，
删掉这个正则函数，顺带去掉文字提示。

### F.3 灵活组排切换后经常没数据

`gameplay.go` `buildGameplayRankedQueues` 里能力表现用 `windowMatches`（近期窗口），
位置偏好用 `allMatches`（全部）——口径不一致，导致"这赛季打过灵活组排但近期窗口
不足 3 场"时能力表现是空的。改法二选一（建议都做）：能力表现也改用 `allMatches`；
样本不足时显示"近期对局中灵活组排样本不足（需 ≥3 场，当前 N 场）"而不是空白。
同时去掉 `queueID==420 且无数据时静默回退440` 的三处兜底——用户已经能显式选队列，
选了单双排却看到灵活组排数据是误导。

**护栏**：三状态独立（改一处不影响另两处）；文字提示已移除；440 只有 2 场时输出含
"样本不足"而不是空字符串。

---

## G.【P1】绝活哥符文诊断埋点（诊断问题的前提，不是修问题本身）

`specialist_runes.go` 全文仍然**零 `diag`/`recordDiagnostic` 调用**，每步失败都是
静默 `continue`。补三个埋点：`specialist_runes_start`（championID/position/players_parsed）、
`specialist_runes_step_failed`（step + errorKind，必须区分 429/403/超时/其它 + budget_remaining）、
`specialist_runes_done`（runes_returned/budget_used/duration_ms）。不补这个，
"绝活哥符文不显示"这个问题永远只能猜。

---

## H.【P2】本赛季 S1/S2/S3 赛段段位：一次性探测埋点

SGP `splitsProgress` 实测 301 次全空，LCU 无分赛段字段，只有
`highestCurrentSeasonReachedTierSR`（本赛季最高，无分赛段）可用。
仍未埋一次性探测代码（参照仓库已有的 `sgp_queue_filter_probe.go` 模式），候选端点：
`/lol-ranked/v1/splits-progress`、`current-ranked-stats` 完整原始 key 列表、
`/lol-career-stats/v1/summoner-games/{puuid}`、`/lol-regalia/v2/summoners/{id}/regalia`、
`/lol-seasons/v1/*`。埋点只记 path/状态/字节数/顶层 key 列表，不记 body、不记 PUUID。

---

## I. 其它 P1/P2（未重新核实，按原工单结论列出，执行前先复核一次现状）

- 召唤师技能门槛放宽的范围可能超出预期：工单原意只放宽 spell，`structuredRecommendationMetrics`
  疑似连 starter/boots 也一起放宽了，需要拍板是否收回。
- `specialistOpponent` 找不到同位置对手时静默兜底成"第一个遇到的敌方参战者"，
  应改成返回空 + 明确"无同位置对手样本"。
- 能力雷达图例文案"翡翠 II 平均"与实际口径（该玩家近期同位置对手的表现，非段位平均）不符。
- `hexdata.go` 存在的 data race（`snapshot()` 值拷贝但内部 map 共享引用）和
  `recordSuccess` 在 shape 校验前清零失败计数（导致退避机制不可达）——建议单独排期，
  优先级不低于本文档但性质不同（稳定性 bug 不是功能缺陷）。
- `season_stats.go:342` `ResumeIndex` 在 `Complete=true` 时不归零，导致赛季扫完后
  总览仍反复重拉深偏移量的旧数据。

---

## J. 待清理

`zz_proof_test.go` 仍然存在于仓库根目录（验收时的空壳文件，`//go:build ignore`，
不影响构建/测试），沙箱没有删除权限，**请手动删除**。

---

## 建议执行顺序

1. **A**（优劣势对抗）—— 改动最小，用户可见收益最大
2. **C**（缓存键碰撞）—— 会返回错误英雄数据，必修
3. **B**（lolalytics 删除 + 去重 + 第六件，第六件需先验证上游是否有 depth 6 字段）
4. **D**（低样本角标接上）
5. **E**（海克斯图标降级）
6. **F**（三处队列解耦 + 灵活组排口径统一）
7. **G**（绝活哥埋点，为下一轮诊断做准备）
8. **H**（S1-S3 一次性探测）
9. **I** 按优先级插空，**J** 顺手做
