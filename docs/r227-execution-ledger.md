# R227 执行账本（2026-10-05）

状态：**修复及工单指定本地验证完成；尚未提交、发布或确认关闭。**

工单：[WORKLIST-R227](WORKLIST-R227-R224-VERIFICATION-GATE-BLOCKED-MARKED-AS-STOPPED-STICKY-AFTER-RESUME.md)。基线版本 **0.12.73**，保留工作区 R222～R224 的既有未提交改动；本单未提交、未发布、未生成安装包。

## 修复

- `scheduleChampSelectRequest` 的 `gate-blocked` 分支不再调用 `champSelectStopped`，保留原有诊断、取消和返回。暂停、手动接管、关闭设置、离开英雄选择不会耗尽写入机会，也不会产生永久 `stopped:<actionID>` 标记。
- 真正耗尽机会的 `attempt-limit`、所有候选无剩余预算的停止处理，以及对应的快照读取保留。没有调整 R224 的步骤预算、不同候选上限或勇敢举动逻辑，没有新增界面文案。
- 独立只读核验确认 gate 分支之外的停止调用仍用于预算耗尽。暂停或接管释放后可以继续调度时，回归断言没有停止记录、诊断、标记或英雄 `stopped` 状态。

## 回归证据

新增 `backend/r227_test.go`：

- `TestR227GateChangesDuringSessionReadDoNotStopPick`：用 channel 冻结实际 session GET，在初始门禁检查之后分别暂停、接管、离开 ChampSelect、关闭设置。请求没有写入，没有停止记录或标记；恢复后下一次评估实际发送英雄 5 的锁定 PATCH。
- 会话读取期间接管会先被现有 `candidate-gate/manual-takeover` 拦截，因此另加 `TestR227TakeoverAfterCandidateEvaluationDoesNotStopPick`，在候选评估之后注入接管，确定命中 `schedule/gate-blocked`；同样验证恢复正常调度、无停止状态。
- 修改生产代码前，新测试确定复现暂停、离开阶段、关闭设置及晚到接管误报停止，见 [regression-before.log](history/reports/r227/regression-before.log)。修复后 R224/R227 全部定向回归通过。
- R224 指定四项均通过：`TestR224BraverySkipsPlanningThenExecutesStrategies`、`TestR224IntentFailureDoesNotConsumeLockBudget`、`TestR224SingleCandidateFailureExplainsStopAndResetsOnDodge`、`TestR224BraveryRandomChampionConfirmsCompletedOnly`。R224 真实预算耗尽仍显示 stopped 且按动作去重，新会话重置仍有效。
- 针对 R224/R227 的 race 检查通过，包括新加的会话读取期间状态变化夹具。

## 最终验证

最后一次 Go 源码/测试修改之后运行以下检查。时间均为北京时间；原始输出与含起止时间、耗时、退出码、最后 5 行的 JSON 位于 [r227 证据目录](history/reports/r227/)。Go 使用系统默认构建缓存。

| 检查 | 开始→结束 | 耗时 | 结果 |
|---|---|---:|---|
| `go test -count=1 ./backend -run '^TestR22(4\|7)'` | 21:43:51→21:43:58 | 7.723s | 通过 |
| `go test -count=1 ./backend` | 21:45:31→21:48:08 | 156.898s | 通过 |
| `go vet ./backend` | 21:45:35→21:45:36 | 1.015s | 通过，输出为空 |
| `go test -race -count=1 ./backend -run '^TestR22(4\|7)'` | 21:45:40→21:46:15 | 35.245s | 通过 |
| public 后端构建、指纹/Key 策略及本机自检 | 21:45:55→21:46:18 | 22.554s | 通过 |

Go 全量最后 5 行（实际输出只有 1 行）：

```text
ok  	lol-loot-assistant/backend	155.762s
```

Go vet 输出为空，退出码 0。定向普通/race 输出均只有一行，分别为：

```text
ok  	lol-loot-assistant/backend	0.688s
ok  	lol-loot-assistant/backend	2.070s
```

## 构建与验收边界

已重建本机和 Windows amd64 public 后端。版本 **0.12.73**，key mode **public**，源码指纹 **d7d3268b6b23**；构建前后指纹相同、两份产物指纹及无内嵌 Riot Key 策略通过，本机 self-test 通过。

- `dist/r227/loot-service-0.12.73-public`：SHA256 `9a4a077ea13747adfc8734161dd8c8be0ec9b4eea50a01d00df026894bdfd661`。
- `dist/r227/loot-service-0.12.73-public.exe`：SHA256 `553e9165076a15c6fad107d7cb7c52c09567ef162ac85ea7316e2702ddb2ba13`。

Windows exe 仅交叉编译、未执行。本单没有前端修改；R226 的 R70 隐藏页异常由 R226 处理。R224 的 Windows 真实客户端请求、各模式重绘/录屏和间距验收仍未完成，R224 保持进行中，不能用本单的本地测试代替。

## R228草稿合并与后续验证（2026-10-05）

以上未提交/未发布描述为本单实施当时记录。修复现已纳入0.12.74 public草稿（tag434caa92，指纹a3d7c1e75735），tag完整质量37329122051与最终分支37334082296均success。R228最终预算115.797/219.765/67.949s达标；草稿附件验证通过。仍未正式发布Latest，等待R228 P6确认；Windows真实游戏客户端验收边界不变。详见 [R228账本](r228-execution-ledger.md)。

2026-10-06发布补记：用户确认后，0.12.74于00:02:04正式发布Latest（id403843482、isLatest=true、匿名清单0.12.74及三附件验证通过）。以上草稿/待确认记录为10-05历史状态，当前发布完成；真实Windows游戏客户端验收边界不变。见 [正式发布账本](history/ledgers/release-0.12.74-execution-ledger.md)。
