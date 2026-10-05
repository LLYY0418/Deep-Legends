# WORKLIST-R227：R224 复核——暂停、手动接管或离开英雄选择时被拦下的请求，也被当成「自动选用已停止，请手动选择」，而且恢复后一直显示已停止

诊断：Claude（复核 R224 执行结果：读改动和账本，重跑前端测试）。执行：GPT。日期：2026-10-05。基线：0.12.73，工作区含 R222～R224 未提交改动。

## 复核结论

R224 的 P1～P5 我逐项核对过源码，实现与工单一致：

- P1：失败计数按「动作 + 步骤 + 英雄」分开；勇敢举动在规划阶段不预选；锁定确认接受随机英雄；`advancing` 按真实候选判断；停止时有提示。
- P2：斗魂只读 teamOne，`stale_team_two_dropped`；名单两次去重；playerlist 补人；前端重复 key 过滤。
- P3：提示条、页签原地更新，名单按 key 增量更新，记录真实触发来源和 `shell_node`。
- P4：统一 `--identity-chip-gap: 6px`，标签的左边距已去掉。
- P5：斗魂顺序为自己 → 小队 → 其他人。

前端 `node --test backend/web/r224.test.cjs` 我这边重跑 5/5 通过。Go 测试我这边没有重跑，以账本记录为准（21:25:59 全量通过）。

`desktop/refresh-orchestration.test.cjs` 的 R70 项仍然失败（`liveRenderTriggerLabel is not defined`），**这个问题已经在 R226 里，本单不重复**。

R224 账本里列出的 Windows 真机验收（勇敢举动两种策略、5v5 接斗魂、各模式三分钟重绘记录和录像、真机标签间距）**仍未完成，R224 不能关闭**，继续按 R224 的清单执行。

**新发现的问题只有下面这一个。**

---

## 问题：`gate-blocked` 也调用 `champSelectStopped`

### 位置

`backend/champselect.go` `scheduleChampSelectRequest`（约 1249～1255 行）：

```go
if !r.settings.ChampSelect.Enabled || !strings.EqualFold(r.champSelect.phase, "ChampSelect") || r.champSelect.sessionPaused || r.champSelect.takeover[...] {
    r.mu.Unlock()
    r.champDiagnostic("schedule", "gate-blocked", decision, nil)
    r.champSelectStopped(decision, "gate-blocked")   // R224 新增
    ...
}
```

### 为什么不对

`gate-blocked` 表示请求在安排的那一刻被门禁拦下，原因可能是：

| 原因 | 实际含义 | 现在的表现 |
|---|---|---|
| 用户点了「本局暂停」 | 用户主动暂停 | 本局记录写一条 **fail「自动选用已停止，请手动选择」** |
| 用户手动换了英雄（takeover） | 用户已经自己选了 | 同上，提示让用户「手动选择」，和事实矛盾 |
| 英雄选择刚结束或有人秒退（phase 不再是 ChampSelect） | 回合结束 | 同上 |
| 设置里关掉了自动选用 | 用户主动关闭 | 同上 |

这些都不是「自动选用失败」。`evaluateChampSelect` 在开头检查了暂停和阶段（约 750 行），之后要先读一次 `/lol-champ-select/v1/session`（最多 8 秒），这期间用户暂停或阶段变化，就会走到这里。

**状态会一直卡在「已停止」：** `champSelectStopped` 把 `exhausted["stopped:<actionID>"]` 设为 true，这个标记只在新的英雄选择会话里才清空。`champSelectSnapshot`（约 1905～1909 行）只要看到这个标记，就把选用序列里的所有英雄都显示成「自动选用已停止」。用户恢复暂停以后，`evaluateChampSelect` 会继续正常安排请求（它不看这个标记），**界面却一直显示已停止**，和实际行为不一致。

### 修复要求

1. `gate-blocked` 不再调用 `champSelectStopped`，只保留现有的诊断。「已停止」只用于真正把这个动作的机会用完的情况：`attempt-limit`，以及 `evaluateChampSelect` 里所有候选都没有剩余机会（约 988～993 行）。
2. 被暂停、手动接管、关闭设置拦下的请求，按现有的暂停/接管提示显示，不新增文案。
3. 如果在 R224 之后 `exhausted["stopped:<actionID>"]` 还被别处用于真正的停止，保留；但快照里显示「已停止」必须和执行逻辑一致：执行逻辑还会继续安排请求时，快照不能显示已停止。
4. 不改 R224 已有的预算和勇敢举动逻辑。

### 测试

- Go：`evaluateChampSelect` 读会话期间把 `sessionPaused` 改为 true → 请求被拦下，本局记录**没有**「自动选用已停止」，快照里该英雄不是 `stopped`；恢复后下一次评估正常安排请求。
- Go：同样的场景换成 takeover 和 phase 离开 ChampSelect，各一个用例。
- Go：R224 原有四个测试全部通过（`TestR224BraverySkipsPlanningThenExecutesStrategies`、`TestR224IntentFailureDoesNotConsumeLockBudget`、`TestR224SingleCandidateFailureExplainsStopAndResetsOnDodge`、`TestR224BraveryRandomChampionConfirmsCompletedOnly`）。

## 记录

- 最后一次 Go 改动之后跑 `go test -count=1 ./backend` 和 `go vet ./backend`，按 R218 约定记录运行时间、耗时和输出最后 5 行。
- 新建 `docs/r227-execution-ledger.md`，在 `docs/WORKLIST-INDEX.md` 登记。
- 可以和 R226 一起做，两者不冲突。
