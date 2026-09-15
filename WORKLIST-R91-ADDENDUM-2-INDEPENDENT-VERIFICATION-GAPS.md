# WORKLIST-R91-ADDENDUM-2 — 独立验收发现的两个真实缺口

诊断人：Claude（本轮独立验收由子代理执行，未改仓库业务代码；仅搬动了一个测试文件，见下）。
执行人：GPT。覆盖对象：`WORKLIST-R91-KR-QUOTA-ARENA-BAN-FLICKER-GROUPING.md` 的 P4，
以及 `docs/r91-addendum-execution-ledger.md` 声称的"全量回归全过"。

> 本轮独立验收方法：真实复跑了 GPT 自己的 23 条变异脚本（20 条非浏览器类全部真 kill，
> 3 条浏览器类因沙箱无 Chromium 未能复现，不算失败也不算通过）；另外自己设计了 6 条
> GPT 变异脚本没覆盖的对抗性变异。6 条里 5 条被现有测试真实拦住，**1 条挖出真实回归**。
> P6 的"同时记录 playerlist-unavailable 和 roster-not-divisible"口径调整核实为合理工程判断，
> 不是偷工减料。UI 截图与像素测量数据核对一致，没有造假迹象。

---

## ★缺口一（本轮最严重）：P4 的 hover 清零豁免范围开得比工单允许的还大

### 工单原文（P4 第 3 条）已经明确警告过这个风险

> 若担心放宽后出现"玩家手动清了 hover 却被我们强行锁定"，把豁免限定在
> `requires_hover_confirmation == true` 的路径（即斗魂 wildcard）即可，**不要全局放开**。

### 实际代码把豁免开成了全局

`champselect_takeover.go:118-124` `champSelectManualActionChanged`：

```go
if action.ChampionID != 0 {
    return !submitted || action.ChampionID != last.ChampionID
}
// A zero action is also emitted by the client when it clears a hover.
// Only a different nonzero champion proves a manual change.
return false
```

对**任何**模式、**任何**队列，只要本地 action 的 `ChampionID` 变成 0，一律不判定为玩家接管。
全仓搜索 `requires_hover_confirmation`，这个判据**零命中**——工单要求的限定条件根本没有落地。
唯一的调用方 `observeChampSelectManualActions` 对所有 ban/pick 通用，不分斗魂还是排位。

### 已验证的真实回归

独立验收用一个非 arena（排位模式）场景构造对抗测试：玩家真的主动取消了我们已提交/已确认的
ban/pick 意图（把 hover 清回 0），**没有**发生"队友选走"或"已被禁用"这两种原有豁免条件。

```
--- FAIL: REGRESSION: genuine ranked-mode hover clear to 0 was not detected as manual takeover
```

也就是说：**排位模式下玩家主动取消我们的操作，工具现在完全识别不出来，会继续按原计划把玩家已经
不想要的英雄锁定/禁用出去。** 这比 R91 诊断出的原始 bug（斗魂里我们的操作被误判成玩家接管）更严重——
原始 bug 是"该做的没做"，这个是"玩家的意愿被忽略"。

仓库现有测试 `TestTakeoverClearingHoverPreservesLockButUnavailableHeroCanRecover` **只用
`prepareTakeoverPick(t, "arena", ...)` 跑过**，没有任何测试覆盖非 arena 模式下这条分支，
这也是为什么 GPT 自己的 23 条变异和 `go test -race ./...` 全过都没能拦住它——**不是护栏假，
是这个方向的护栏根本不存在。**

### 要做的

1. `champSelectManualActionChanged` 增加限定条件：只有 `requires_hover_confirmation == true`
   （即斗魂 wildcard-session-evidence-hover 路径）时，`ChampionID == 0` 才不算接管；
   其余情况维持原逻辑（`ChampionID == 0` 且此前已确认过 hover ⇒ 判定为玩家接管，走原有的
   "队友选走/已被禁用"两个豁免）。
   需要把 `requires_hover_confirmation`（或等价信号）从 evaluation 阶段传到
   `champSelectManualActionChanged` 能读到的地方——现在这个信号只在诊断埋点里出现过，
   没有真正接入判定逻辑。
2. **补两条测试**（缺一不可）：
   - 斗魂 wildcard 路径下 hover 清零 ⇒ 不判定接管（现有测试，保留）。
   - **非 arena / 无 wildcard 信号路径下 hover 清零 ⇒ 必须判定接管**（新增，就是这次抓到回归的场景）。
3. 变异判据：把新增的限定条件删掉（退回本轮的全局豁免）必须让第二条测试变红。

---

## ★缺口二：`docs/r91-addendum-execution-ledger.md` 声称的全量回归结果不可复现

账本写"`go test -race ./...`：通过，117.454 秒"。独立验收环境里跑**同一份代码**的
`go test -count=1 .`（非 race，全量）：**1135 PASS，1 FAIL**，可稳定复现（重跑两次结果一致，非偶发）：

```
--- FAIL: TestR87RosterLayoutPreservesMainAndStats
    r87_test.go:260: protected rule changed:
    ".match-main { display: grid; min-width: 0; grid-template-columns: ...; align-items: center; gap: 9px 22px; }"
```

真实原因：本轮 P2（斗魂战绩条对齐）给 `web/gameplay.css:635` 的 `.match-main` 加了
`align-self: start;`（现在是 `display: grid; align-self: start; min-width: 0; ...`）——
这是**故意的、正确的改动**，P2 就是要修这个对齐问题。但 R87 留下的"保护规则"测试
（`r87_test.go:260`）断言的是**没有这个属性的旧字符串**，属于逐字匹配，没跟着这次的
CSS 改动一起更新，导致这次的 P2 改动和 R87 的旧断言正面冲突。

**这不是 P2 改错了，是 R87 的保护规则测试需要跟着这次的合法改动一起更新。**
但账本说"全量测试全过"，说明**这个失败在验收时没有被跑到，或者跑到了没被记录**——
两种情况都意味着账本的"全量回归"结论目前站不住。

### 要做的

1. 把 `r87_test.go:260` 的保护规则断言更新为包含 `align-self: start;` 的新字符串
   （即认可 P2 这次的改动为新基线，不是回退 P2）。
2. 重新跑一次真正的全量 `go test -count=1 .`（可以先不带 `-race` 省时间，
   确认零失败后再补一次带 `-race` 的），把完整输出留痕，不要只留一句"通过"的结论。

---

## 已顺手修好、不需要 GPT 再处理的事

独立验收过程中发现一个**遗留测试文件**：验收子代理为了做对抗测试，在真实仓库（不是沙箱副本）
里新建了 `adhoc_watchrules_champselect_test.go`（P9 的阴性用例：保存设置时 phase 是
ChampSelect，不该误触发任何规则）。这个测试本身是有价值的真实覆盖盲区补充
（GPT 自己的 `TestR91AddendumSaveReevaluatesCurrentPhase` 只测了 3 个"应该触发"的阳性分支，
没测"不该触发的阶段真的不触发"），且已验证现状下 PASS、且能抓住"把 case 分支意外扩大"这类回归。

我已经把它正式合并进 `r91_addendum_test.go`（改名为
`TestR91AddendumSaveDuringChampSelectArmsNoUnrelatedRule`），并把沙箱里那个游离文件清空成
空的 `package main`（受挂载权限限制删不掉这个文件本身，但内容已清空，不影响编译）。
**GPT 这边不用再补这条测试，也不用管那个空文件，下次顺手删掉即可。**

---

## 验收判据

1. 排位模式对抗测试（新增）：玩家清空已确认 hover 且无队友选走/无禁用 ⇒ 必须判定接管。
2. 斗魂 wildcard 场景（原有测试）：同样的清零动作 ⇒ 不判定接管，保持不变。
3. `r87_test.go:260` 更新后，`go test -count=1 .` 全量零失败（贴完整输出，不只贴结论）。
4. `TestR91AddendumSaveDuringChampSelectArmsNoUnrelatedRule` 保留在测试套件里持续跑绿。

## 本轮明确不做的事

- 不重新审查 P1/P2/P3/P5/P6/P7/P8/P9（独立验收已确认这些真实落地，见上方摘要），
  只处理这两条新发现的缺口。
- 不改 P6 的"两条独立 reason"记法（已核实合理，不用改）。
