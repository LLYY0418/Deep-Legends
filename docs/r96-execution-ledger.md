# R96 执行账本

执行日期：2026-09-16。范围：`WORKLIST-R96-ARENA-GUARDRAIL-RECONNECT.md`。本轮只修复护栏调用链、对应验证脚本和 partial 真值日志；保留已有 R95 工作及用户 NSIS 配置，未提交、未推送。

## 逐项结果

1. **生产护栏接回**：`applyArenaLiveGrouping` 取得真实字段候选、通过结构校验后，正式设置任何 `ArenaGroup` 前调用 `arenaAlliesCorroborate`。发现冲突，或者已有完整小队却不能完成交叉验证，立即用 `allies-cross-blocks` 硬拒绝，并关闭该原因的重试。没有完整小队记忆时维持原有真实字段行为，任何已知矛盾仍必须拒绝。
2. **不改算法、不软化护栏**：护栏函数体与本轮开始前备份逐字节一致；没有重新接通 session/playerlist 顺序分组。没有真实字段时仍为 flat / `order-inference-disabled`。“我的小队”标记在拒绝后仍保留。
3. **真实生产路径对抗测试**：`TestR96ExplicitSubteamGuard` 为 `subteamId`、`playerSubteamId` 分别构造真实 JSON，经 `parseLiveClientPlayerList`、`loadGameplayLive` 和 `applyArenaLiveGrouping`。交换两名成员的字段值，使六组各三人的结构仍合法、但已知三人被拆开，断言硬拒绝。每种字段还覆盖一致分组放行；正例使用不同于顺序块号的真实 ID，防止用名单位置冒充字段值。未手工构造 grouping 类型，未以直接调用 guard 的单测代替集成验证。
4. **partial 日志清理**：记录同局已经观察到实际真值的状态，跨后续 observation 保留；部分 SGP 数据仍记录为 `sgp`、`conclusive=false`，不置 `checked`，重试结束不再追加误导性的 `none`。`TestR96PartialTruthDoesNotEndAsNone` 实际耗尽四次 SGP HTTP 重试，再走 Reconnect / 无 provider，最后补齐真值，验证仍可得到完整结论。完全没有数据的 none 行为由原 R95 回归继续覆盖。

R95 账本中“禁用顺序推断与保留生产护栏存在矛盾”的解释不成立：真实字段路径也必须接受队友交叉验证。R96 在此明确纠正该遗漏，不通过恢复顺序推断来绕开问题。

## 变异脚本修复与实跑

`scripts/r90-mutation-check.py` 保留 12 项原有检测意图，并新增 partial→none 变异，共 **13/13 KILLED**。每个唯一锚点均实际命中；每组测试先跑未变异基线；编译失败、超时不计 kill。Go 使用临时 overlay，JS 使用源码环境变量，不替换工作区生产文件。

- `removed-ally-check`：当前真实生产调用改为恒定验证通过，冲突 fixture 断言失败。
- `raw-identity-only`：改用显式字段正例，验证真实身份补全链。
- `wrong-block-size`：原顺序 helper 已不属于生产路径，改为把真实候选结构校验的 squadSize 变成 2。
- `session-instead-of-playerlist`：旧分支已删除，改为在真实候选赋值处注入名单位置块号；正例真实 ID 断言捕获。不恢复生产顺序推断。
- `truth-partition-forced`：锚点迁至 `arena_truth_diagnostics.go` 的实际事件字段。
- 其余 mascot / cache / retry / queue 静态防线保留。其中 mascot 的 order 参数负例仍是独立防御性 helper 测试，不宣称有生产顺序分组。
- `partial-truth-reported-none`：撤掉已观察真值的 none 抑制，真实重试测试失败。

实跑命令：

```sh
R90_MUTATION_OUTPUT=docs/r96-validation/mutations \
GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp \
python3 scripts/r90-mutation-check.py
```

## 验证证据

三组全量回归均实际执行，退出码均为 0：

| 命令 | 结果 | 完整日志 |
|---|---|---|
| `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go test -count=1 -v .` | PASS，89.360s；1149 个顶层测试通过，17 个既有 opt-in 网络/本地采集 fixture 测试按条件跳过 | [go-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/go-test.txt) |
| `GOCACHE=/tmp/deep-legends-go-cache GOTMPDIR=/tmp/deep-legends-go-tmp go test -race ./...` | PASS，110.479s，无 DATA RACE | [go-race.txt](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/go-race.txt) |
| `node --test web/*.test.cjs desktop/*.test.cjs` | 696 tests，695 pass，0 fail，1 skip，232.232s | [node-test.txt](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/node-test.txt) |

Node 唯一跳过项为已有的 Windows/PowerShell release 测试（当前 macOS）；没有新增 skip 或削弱测试以使回归通过。所有 R96 新 fixture 均执行通过。`git diff --check` 通过。

- [定向生产 fixture 输出](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/go-focused.txt)
- [变异脚本完整输出](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/mutation-run.txt)
- [13 项变异矩阵](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/mutations/mutations.json)（同目录每项均有完整断言失败日志及对应基线日志）
- [函数体与用户配置不变核验](/Users/ly/personal/personal-work/deep-legends/docs/r96-validation/invariants.txt)

按工单，本轮不要求真实 LOL 客户端验证；上述是实际执行的协议 fixture，不冒充真实对局。
