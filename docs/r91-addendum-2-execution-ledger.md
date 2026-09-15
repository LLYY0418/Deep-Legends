# R91 ADDENDUM-2 独立验收缺口执行账本

2026-09-15。本轮只处理 [补充工单 2](../WORKLIST-R91-ADDENDUM-2-INDEPENDENT-VERIFICATION-GAPS.md) 指出的 P4 清零豁免范围和 R87 样式保护断言两个缺口。

## 实现与验证

| 工单项 | 修复 | 行为证据 |
|---|---|---|
| P4 限定清零豁免 | `champSelectManualActionChanged` 读取已提交记录的 `Decision.ForceHover`。只有实际 wildcard 禁用确认路径可以保留已清零的确认 hover。非 wildcard 恢复原判定：已确认、未完成、正英雄 ID 的 hover 清零应让位；队友已选或已禁用则继续允许候选顺延。`hover-cleared-by-client` 诊断也受相同信号限定。 | ranked / normal / arena 三种模式的普通 pick，分别覆盖手动清零、队友选走、已禁用。真实 ban PATCH 在 ranked / arena 两种模式下分别经 polling 与无中间轮询的 delayed preflight 验证清零后不再锁定，且以后轮询不能重启该操作。 |
| wildcard 正例保留 | 沿用已有实际决策信号，不根据 queue/group 名字判定。原 wildcard 清零后继续锁定、改成其他非零英雄必须让位两条用例保留。 | `TestR91AddendumWildcardHoverClearKeepsArmedLock` 的 0 / 104 两个子用例持续通过。 |
| R87 基线 | 仅把保护字符串更新成现有 `.match-main` 的 `align-self: start;`，接受上轮 P2 的合法样式调整。 | 修复前旧保护断言真实失败；更新后完整非 race / race 均覆盖并通过。业务 CSS 本轮未改。 |
| ChampSelect 阴性用例 | 保留独立验收已正式加入的 `TestR91AddendumSaveDuringChampSelectArmsNoUnrelatedRule`。 | 定向与两轮全量持续通过；`r91_addendum_test.go` 与本轮开始时副本完全相同，未删减该用例。空 `adhoc_watchrules_champselect_test.go` 保持原样。 |

信号链已核实：`champselect.go` evaluation 只在 `side == "ban" && strings.HasPrefix(banSource, "wildcard-")` 时设置 `ForceHover`；实际提交记录保存整个 `Decision`；polling 和 delayed preflight 均调用修复后的判定函数。因此 arena 的普通 pick / 非 wildcard ban 清零仍会让位，不能仅因模式是 arena 就豁免。

## 可复跑证据

运行环境：`go version go1.24.5 darwin/arm64`，缓存目录 `/tmp/deep-legends-go-cache`。全量命令均含 `-count=1` 禁用测试结果缓存，`-v` 保存每个测试及子用例输出。下面的链接都是完整日志，不是截取成功结论。

| 检查 | 结果 | 完整日志 |
|---|---|---|
| 修复前复现：新增普通 hover 清零用例 + R87 基线 | 如预期失败，观察到普通 ban 仍发出完成 PATCH、pick pending 未取消和旧 CSS 基线失败 | [修复前完整输出](r91-addendum-2/before-fix.txt) |
| 定向：全部 TestTakeover、ADDENDUM-2、原 wildcard、ChampSelect 阴性与 R87 | 13 个顶层用例、含子用例 36 个通过，0 失败，12.038 秒 | [定向完整输出](r91-addendum-2/targeted.txt) |
| `GOCACHE=/tmp/deep-legends-go-cache go test -count=1 -v .` | 1137 个顶层用例通过，含子用例 1766 个通过；0 失败，17 项跳过，87.829 秒 | [非 race 全量完整输出](r91-addendum-2/go-full.txt) |
| `GOCACHE=/tmp/deep-legends-go-cache go test -race -count=1 -v ./...` | 1137 个顶层用例通过，含子用例 1766 个通过；0 失败、无数据竞争，17 项跳过，110.849 秒 | [race 全量完整输出](r91-addendum-2/go-race-full.txt) |
| `go vet ./...` / `git diff --check` | 通过 | [vet 输出](r91-addendum-2/go-vet.txt)、[diff 检查输出](r91-addendum-2/diff-check.txt) |

17 项跳过均为仓库已有的 opt-in 公网探测或需要本地捕获文件的用例。本轮未启用这些额外来源；它们不计为通过，具体名称和原因完整保留在日志中。这里的全量零失败指默认完整测试套件的实际结果。

race 运行前保存了所有仓库 Go 文件、go.mod/go.sum 和运行时布局 CSS 的 [输入 SHA-256](r91-addendum-2/go-input-sha256.json)，运行后逐项核对未变化。[运行元信息](r91-addendum-2/go-race-run.json) 记录命令、开始时间、环境和最终结果。

## 变异验证

只执行与本轮 P4 改动相关的 6 条，不重审其它工单项。使用 [现有隔离变异脚本](../scripts/r91-addendum-mutations.py)，为旧 wildcard 变异更新源码锚点，并补入相反方向的护栏；用 `R91_MUTATION_OUTPUT` 将新输出与上轮历史结果分开。

| 变异 | 捕获它的行为 |
|---|---|
| wildcard 清零也判定接管 | 原 wildcard 正例不能完成锁定，变红 |
| preflight 恢复对零值的一律拒绝 | 原 wildcard 延迟锁定正例变红 |
| 删除 ForceHover 限定，恢复全局豁免（ban） | 普通 ban 清零后仍发出锁定 PATCH，新增测试变红 |
| 删除 ForceHover 限定，恢复全局豁免（pick） | 普通 pick 清零后 pending 未取消，新增矩阵变红 |
| 去掉队友选走/已禁用豁免 | 正常顺延被错误判定为手动接管，新增矩阵变红 |
| 清零诊断恢复全局记录 | 普通手动清零被误记成客户端兼容清零，新增测试变红 |

6/6 均由实际断言失败拦截。编译失败、超时或启动失败不计入通过。真实源码不改写；变异文件只放临时目录并用 Go overlay 加载。

完整 [变异结果 JSON](r91-addendum-2/mutations/results.json)、[执行摘要](r91-addendum-2/mutations.txt) 和各项输出位于同目录。复跑命令：

```bash
R91_MUTATION_OUTPUT="$PWD/docs/r91-addendum-2/mutations" \
R91_MUTATION_NAMES=hover-cleared-takeover,hover-clear-lock-rejected,hover-clear-global-exemption-ban,hover-clear-global-exemption-pick,hover-clear-unavailable-exemptions,hover-clear-diagnostic-scope \
python3 scripts/r91-addendum-mutations.py
```

只读独立复核还顺着 evaluation → Decision → submit record → polling/preflight 核验了信号传播及测试路径，未发现本轮两个缺口的阻断问题。最终测试与变异结果由主代理核对。

## 上轮结论更正与范围

[上轮执行账本](r91-addendum-execution-ledger.md) 已更正：117.454 秒的 Go race 运行早于最后 CSS 主区顶部对齐修改，后续只复跑了前端布局测试，因此不能代表最终源码全量通过。该历史输出保留，本轮的 `-count=1 -v` 非 race / race 输出取代其最终验收结论。P4 原先的全局清零豁免描述也已收窄，README / DESIGN 同步说明实际条件。

没有重新审查或修改 P1/P2/P3/P5/P6/P7/P8/P9 业务逻辑，没有回退 P2 CSS，也没有改变 P6 两条独立拒绝 reason 的口径。未打包、发布或操作真实客户端。已有工作区修改均保留；[本轮独立补丁](r91-addendum-2/scoped-changes.patch) 以本轮开始副本为基线，[来源指纹](r91-addendum-2/source-sha256.json) 标记本轮最终文件。
