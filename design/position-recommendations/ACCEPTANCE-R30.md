# R30 验收：A~H 全部落地且护栏为真，但发现一个未被测试覆盖的真回归

日期 2026-08-27。范围：`WORKLIST-R30-CONSOLIDATED.md` A~H 全部条目。

## 环境与基线

```
go build ./...      OK
gofmt -l .           干净
go test ./...        411 pass / 0 fail
node --test（5个）    164 pass / 0 fail
```

## 逐条核实结果（file:line 为本次读码所见）

| 项 | 状态 | 证据 |
|---|---|---|
| A 优劣势对抗数组错误 | ✅ 两处调用点都已删掉 `Summary.Positions[X].Counters` 覆盖逻辑，直接用 `payload.Data.Counters` | `champions_structured.go:308`、`:973` |
| B.1 删除 lolalytics | ✅ 全仓 `grep -i lolalytics` 零命中，`lolalytics.go`/`lolalytics_test.go` 已删除 | — |
| B.2 第六件装备 | ✅ `parseOPGGDepthRows` 循环范围改成 `{4,5,6}`，Go/JS 都补了 `SixthItems`/`第六件` | `champions_structured.go:402/477/947`，`web/champions.js:1671` |
| B.3 去重只剔核心装、不跨列 | ✅ `filterOPGGDepthCoreItems` 只用 `coreItems[0]` 的 ID 集合过滤，逐列独立处理，未跨列传播 | `champions_structured.go:1021-1046` |
| C 绝活哥缓存键碰撞 | ✅ 键改成 `fmt.Sprintf("%d\|%s", ...)` 字符串，且用 `cached.position == position` 做二次校验 | `specialist_runes.go:50-52,104` |
| D 低样本角标 | ✅ `renderConfigOption` 按每行 `games<100` 加 `is-low-sample`（比我原要求的按列汇总更细粒度，达到目的） | `web/champions.js:1689-1690` |
| E 海克斯图标降级 | ✅ `communityDragonImagePaths` 对 `_large.png` 路径新增两个 `_small.png` 候选 | `main.go:751-758` |
| F.1 三处队列联动解耦 | ✅ 拆成 `rankedQueueRecent/Ability/Position` 三个独立状态，按 `data-ranked-queue-scope` 分发 | `web/gameplay.js:15,769,2486-2487` |
| F.2 文字提示 + 正则注入 | ✅ `addRankedQueueTools` 已删除，`queueSwitcher` 形参真正被使用，可见的"排位模式"文字只剩 `aria-label`（无障碍属性，不可见） | `web/gameplay.js` 全文搜索确认 |
| F.3 灵活组排空数据 + 静默回退 | ✅ 能力表现与位置偏好统一用 `allMatches`，`queueID==420` 的静默回退 440 已删除 | `gameplay.go:1053,1055` |
| G 绝活哥诊断埋点 | ✅ `specialist_runes_start/step_failed/done` 三类埋点齐全，`specialistErrorKind` 区分 timeout/429/403/other | `specialist_runes.go:156-233` |
| H S1-S3 一次性探测 | ✅ `ranked_split_probe.go` 一次性 `sync.Once`，5 个候选端点，只记 path/状态/字节数/顶层 key，不记 PUUID/body | `ranked_split_probe.go` 全文，调用点 `gameplay.go:790` |
| 附带修复 | ✅ `specialistOpponent` 不再静默返回"第一个遇到的对手"，改成无匹配位置时返回空 | `specialist_runes.go:247-274` |
| 清理 | ✅ `zz_proof_test.go` 已删除 | — |

## 变异测试（确认护栏是真的，不是摆设）

对三处最关键的改动做了真实的"改回旧 bug → 测试必须挂"验证：

1. **绝活哥缓存键**：还原成 `championID*10+ordinal` 的旧公式 → `TestSpecialistRuneKeyIsCollisionFreeAndCanonical` 挂
2. **优劣势对抗**：在两个调用点都还原 `Summary.Positions[X].Counters` 覆盖逻辑 → `TestLoadStructuredCountersUsesTopLevelCounters`、`TestLoadStructuredDetailUsesTopLevelCounters` 都挂
3. **出装去重**：把"只剔核心装"改成跨列去重（用已选中的 ID 累积过滤后续列）→ `TestOPGGDepthDedupRemovesOnlyFirstCoreRouteAndNotAcrossColumns` 挂

三处改回后测试都真实失败，恢复原状后测试全部转绿，护栏可信。

## 发现一个真回归：depth 6 强制要求导致部分英雄整个出装区块失效

`champions_structured.go:474`：

```go
if len(depths[4]) == 0 || len(depths[5]) == 0 || len(depths[6]) == 0 {
    return nil, time.Time{}, errors.New("OP.GG item-depth response changed")
}
```

现有测试全部用合成 fixture（三层数据总是一起造出来的），**没有一个测试覆盖"上游真的只有
depth 4/5、没有 depth 6"这种情况**。我直接用项目自己的请求方式（同样的 Accept/RSC 请求头）
实测了几个真实英雄分路的 op.gg 页面：

```
贾克斯·打野   depth4=有 depth5=有 depth6=有   → 正常
亚索·中单     depth4=有 depth5=有 depth6=有   → 正常
卢兰/Lulu·辅助 depth4=有 depth5=有 depth6=无   → 触发上面这行报错！
慎/Thresh·辅助 depth4=有 depth5=无 depth6=无   → 触发（这个即便没有depth6要求也会挂，非新增）
```

**Lulu·辅助是一个新增回归**：depth 4、5 本来都有真实数据、之前能正常展示，
现在因为 depth 6 上游确实没有这个数据（辅助位很少能撑到第六件），
`len(depths[6])==0` 直接让**整个第四到第六件的出装区块**报"暂不可用"——
本来能看到的 4/5 件推荐也一起消失了。辅助位英雄大概率普遍受影响。

**建议修法**：这行判断改成只要求 depth 4、5 非空（这是解析成功与否的真实信号，
一直是这个逻辑）；depth 6 单独判断，为空时只让"第六件"这一列显示
"该阶段暂无可用样本"（前端 `web/champions.js:1677` 已经有这个空态文案，直接复用），
不要拖累整个区块。

**护栏建议**：补一个 fixture——depth4/5 各 5 行、depth6 为 0 行——断言函数不报错，
且返回的 `FourthItems`/`FifthItems` 正常有数据、`SixthItems` 为空数组。

## 结论

A~H 全部条目真实落地，三处最关键改动的护栏都用变异测试验证过是真的。
但新增的 depth 6 强制校验有一个未被任何测试覆盖的真实回归，会让辅助类英雄的整个
出装区块（不只是第六件）意外消失，**建议在合并前修复这一处**。

## 复验（08-27 晚复核，depth 6 回归已修复）

`champions_structured.go:471` 判断已改成只要求 `depths[4]`/`depths[5]` 非空，
depth 6 单独处理为空数组即可。新测试 `TestLoadOPGGDepthRowsAllowsMissingSixthItemDepth`
**直接用了 Lulu·辅助这个我实测过的真实回归场景做 fixture**（depth4/5 各一行、depth6 缺失），
断言不报错且第四/五件数据完整保留；配套的 `TestLoadOPGGDepthRowsRejectsMissingFifthItemDepth`
确认 depth 4/5 缺失时仍然正确报错（没有把校验放得太松）。

变异测试：把判断改回"depth6 也必须非空" → `TestLoadOPGGDepthRowsAllowsMissingSixthItemDepth`
真实失败；复原后两个测试都转绿。

```
go build ./...     OK
gofmt -l .          干净
go test ./...       413 pass / 0 fail
node --test（5个）   164 pass / 0 fail
```

**R30 到此全部闭环。**
