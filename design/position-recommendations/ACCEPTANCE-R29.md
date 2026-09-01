# R29 验收：核心修法落地良好，但仲裁函数改出一个真回归 + 一个测试笔误

日期 2026-08-27。范围：`design/position-recommendations/WORKLIST-R29.md` 四项改法。

## 环境

沙箱现装 Go 1.23.4（`go build` 自动拉了 toolchain 1.24）+ Node v22.23.2。

```
go build ./...            OK
gofmt -l .                无输出（干净）
go test ./...             408 pass / 2 fail
node --test（web+desktop） 161 pass / 0 fail
```

## 做对的部分（B/C/D 三项全部落地且有真测试覆盖）

- **重试退避+上限**（`connection_manager.go:282` `nextSnapshotRetryDelay`）：5s→10s→20s→40s→60s→60s封顶，`TestSnapshotRetryDelayBacksOffAndCaps` 通过。不再是死循环 5 秒重试。
- **失败预算耗尽后回退历史快照**（`main.go:855-901`）：连续失败 10 次或超过 2 分钟（`snapshotRetryFailureBudget`/`snapshotRetryBudgetDuration`）后标记 `snapshotFallback`，从 `storage` 里取上一次成功快照展示，`/api/status`、`/api/skins` 都带 `stale`/`snapshotFallbackAt`。前端 `web/app.js:413-418` 也接上了："实时库存暂不可用，当前显示 XX 保存的历史快照"，还带"已重试 N 次，耗时 Xs"的文案（`snapshotRetryText`）。这一段测试（`TestHistoricalFallbackSkinsAreServedAsStaleWithoutSnapshotReady`、`TestStatusExposesRetryBudgetAndHistoricalFallback`）都通过。**用户不会再看到无限转圈——这是本次投诉的核心诉求，已经解决。**
- **v1 端点长期失败后降级**（`inventoryV1FailureBudget`）：`TestInventoryV1EndpointDisablesAfterRepeatedFailures`、`TestInventoryV1EndpointIsRemovedFromLiveArbitrationAfterFailures` 通过，确认死端点不再参与仲裁、也不再无谓请求。
- **ID 级诊断**（`OwnershipSourceStatus.MissingIDs`/`ExtraIDs`，`annotateOwnershipDifference`）：补上了，下次再出现"权威源恒差 N"可以直接查出具体是哪些 ID。

## 两个失败测试

### 1. `TestStalePresenceSubsetRetainsAuthoritativeNewIDs` —— 大概率是测试笔误，不是实现 bug

fixture：skins-minimal 显式拥有 1-5（5 件），champions 显式拥有 1-7（7 件），
presence 只知道 1-3（滞后子集）。断言写的是：

```go
if err != nil || len(ids) != 5 || !ids[4] || ids[5] {   // ← 最后一项 ids[5]
```

实现选中了 skins-minimal 作为基准（5 件都在），逻辑上完全正确——但因为
fixture 里 skins-minimal 明确把 5 号标成 `owned:true`，最后一个条件应该是
`!ids[5]`（要求 5 号也在），写成了 `ids[5]`（要求 5 号不在），和前一个条件
`!ids[4]` 的模式对不上，自相矛盾。**建议把 `ids[5]` 改成 `!ids[5]`，不用动实现代码。**

### 2. `TestVerifiedInventorySupersetCompletesTencentOwnedCollection` —— 真回归，需要做设计取舍

用 `git show HEAD:catalog.go`（08-19 首次提交）对比确认：**这个"用完整 CHAMPION_SKIN
库存补齐显式收藏接口遗漏"的能力是老功能**（`corroboratingPresenceSuperset`
在 08-19 就存在，且原本没有"presence 不能有权威源之外的额外 ID"这条限制）。

本轮为了满足我 R29 工单里"R29-2：佐证源多出未知 ID 时依然拒绝"这条护栏要求，
在 `corroboratingPresenceSuperset` 里新增了一段检查（`catalog.go:1041-1051`）：
presence 的 ID 集合如果超出两个权威源 ID 的并集，就整体判定不成立。

**问题是：这条新限制和老功能用的是同一个判定形状**——"presence 是权威源并集的
超集，且多出一个权威源都没提到的 ID"——老功能（Tencent 库存补全）依赖的正是
这种情况被接受，新护栏要求的正是这种情况被拒绝。**本轮新增的两个测试互相矛盾：**

| 测试 | fixture 形状 | 期望结果 |
|---|---|---|
| `TestPresenceSupersetWithExtraIDCannotCorroborateAuthorities`（`r29_test.go`，本轮新写） | presence={1,2,3,9}，权威源并集={1,2,3} | **拒绝**（9 号不该被采信） |
| `TestVerifiedInventorySupersetCompletesTencentOwnedCollection`（`catalog_test.go`，本轮新写） | presence={1001,2001,3001}，权威源并集={1001,2001} | **采信**（3001 号也算拥有） |

两个 fixture 结构完全同构（presence = 权威源并集 + 1 个多余 ID），只是数字和
故事背景不同，**没有任何可从数据里读出的信号能同时满足这两条测试**——
这不是实现没写对，是这轮加的护栏和已有功能在需求层面本来就冲突。

**建议的取舍（我的判断，供确认）**：`corroboratingPresenceSuperset` 是"presence
完整覆盖两个权威源全部内容"这种更强的信号，应该保留原来的宽松行为（撤销
`catalog.go:1041-1051` 那段新增检查，同时删掉/放宽 `r29_test.go` 里那条新测试）；
我 R29-2 真正想防的是"两个权威源打架、只有一个存在争议 ID 的弱佐证"这种场景，
那属于 `bestCorroboratedAuthoritative` 的精确匹配打分逻辑，应该只在那一层加限制，
不该动 `corroboratingPresenceSuperset` 这个更强的整体覆盖判定。

**这是我在写 R29 工单时没有讲清楚场景边界的责任**，两条护栏字面上都对，
但适用的函数应该分开。

## 结论

用户最初投诉的"一直读取中不停止"已经解决（退避上限 + 历史快照兜底，双重测试覆盖）。
但仲裁函数这次改动有一个会影响国服（Tencent）部分账号"用完整库存补全遗漏收藏"
这个老功能的真回归，加上一个测试笔误，**建议 GPT 按上面的取舍修正后再收尾**，
不要直接合并当前状态。

## 复验（08-27 13:20 改动后）

按建议的取舍复核：`corroboratingPresenceSuperset` 已恢复 08-19 原始的宽松行为
（不再检查 presence 是否超出权威源并集），矛盾的那条新测试
`TestPresenceSupersetWithExtraIDCannotCorroborateAuthorities` 已删除；
"佐证源多出未知 ID 不该被采信"这条护栏改到了 `bestCorroboratedAuthoritative`
里（`TestPresenceExtraIDCannotSupportEitherAuthoritativeSet` 覆盖，针对的是
弱信号的精确匹配打分，不影响强信号的整体覆盖判定）。测试笔误
（`ids[5]`→`!ids[5]`）也改了。

```
go build ./...     OK
gofmt -l .          干净
go test ./...       409 pass / 0 fail
node --test（5个）   161 pass / 0 fail
```

`TestVerifiedInventorySupersetCompletesTencentOwnedCollection`（国服库存补全）
和 `TestStalePresenceSubsetRetainsAuthoritativeNewIDs`（原失败测试）都单独
verify 通过。**R29 到此闭环，可以合并。**
