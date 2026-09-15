# R89 · R88 验收遗留三项收尾

2026-09-14。执行工单：`WORKLIST-R89-R88-FOLLOWUP.md`。Q1、Q2、Q3 已完成实现及针对性验收。工作区含此前 R87/R88、征召及其他功能的未提交修改，本轮没有回退这些修改。R88 账本中的源指纹及历史测试结果仍指 R88 当时的验收，不代表当前工作区全量回归结果。

## Q1 · 第四条死容器规则

- 仅删除 `web/gameplay.css` 中四行 `@container arena-first (max-width: 640px)` 详情滚动块，保留有效的 `matches-column (max-width: 720px)` 规则。相对本轮开始的精确差异见 [gameplay-css.diff](r89-r88-followup/gameplay-css.diff)。由于工作区已有 R88 CSS 修改，整个 `git diff` 还包含那些历史修改；没有为满足单轮差异要求而撤销它们。
- `web/champions.css` 整个文件逐字未动，前后 SHA-256 均为 `b82a148ddf1ce7ae1d621f861124516123409049f7730e0d34f6658daa8f5dbc`。当前文件有 `.arena-first-section` 容器声明，没有工单所述的同名 640px 规则。这是执行时原文与工单的出入，未凭描述新增或删除英雄页规则。哈希见 [删除前](r89-r88-followup/css-before-sha256.json)、[删除后](r89-r88-followup/css-after-sha256.json)。
- `desktop/r89-arena-detail-layout.cjs` 在真实 Chromium 加载 `/?demo`，通过鼠标点击真实对局卡片展开和收起详情。在 1920、1280、820、620px 四档验证 21 人/7 队、ARIA 关联、其他卡片节点稳定及窄屏实际横向滚动。详情祖先容器只有 `matches-column` / `gameplay-page`，没有 `arena-first`。
- 删除前后的四档测量完全一致。1920、1280、620px 截图逐字节相同；820px 的 664×1044 截图仅 1 个像素不同，位于 `(206,988)`，未把该图描述成像素完全一致。测量及截图使用固定时间的演示数据，不是实际客户端或图片资源完整性验收。结果见 [comparison.json](r89-r88-followup/comparison.json)、[删除前测量](r89-r88-followup/before/measurements.json)、[删除后测量](r89-r88-followup/after/measurements.json)。
- 删除前后均运行 `node --test web/r88.test.cjs`，各 9 项通过，包含所有有效 `.match-main` / `.match-stats` 原文红线。已纠正 R88 账本中错误的“英雄详情可使用”保留理由。
- 收尾时发现工作区又在 CSS 文件末尾追加了独立的总览出装卡样式；当时保留，随后用户在 R92 中要求撤销，该样式与专用差异附件现已删除。前后哈希描述的是删除规则当时的快照；最终哈希另见 [css-final-sha256.json](r89-r88-followup/css-final-sha256.json)。随后对最新工作区重跑四档 Chromium，全部通过，测量与删除后的验收快照完全一致，见 [recheck/measurements.json](r89-r88-followup/recheck/measurements.json)；最终红线 9 项仍通过，见 [redlines-final.txt](r89-r88-followup/redlines-final.txt)。

截图：[1920px 前](r89-r88-followup/before/arena-detail-1920.png) / [后](r89-r88-followup/after/arena-detail-1920.png)，[1280px 前](r89-r88-followup/before/arena-detail-1280.png) / [后](r89-r88-followup/after/arena-detail-1280.png)，[820px 前](r89-r88-followup/before/arena-detail-820.png) / [后](r89-r88-followup/after/arena-detail-820.png)，[620px 前](r89-r88-followup/before/arena-detail-620.png) / [后](r89-r88-followup/after/arena-detail-620.png)。

## Q2 · 扩展队列硬编码护栏

后续说明：本节记录 R89 当时的覆盖与边界。R90 已追加本地字面量集合索引/range、类型断言、指针、常量组和切片成员判断，完整的额外探测与变异证据见 [R90 队列护栏账本](r90-queue-guard-ledger.md)。

实现位于 `r88_queue_guard_test.go`，原 `TestR87ArenaLiteralBoundary` 继续扫描全部非测试 `champselect*.go`。使用 AST 绑定区分别名作用域，追踪队列来源与数值常量，不依赖源文本中出现特定队列数字。

覆盖：

- 原始整数比较、六种比较运算、变量/常量中转和 switch-case。
- 可解析为整数的字符串，`strconv.FormatInt` / `FormatUint` / `Itoa`、`fmt.Sprintf` / `Sprint`，包括导入别名与函数别名。
- 整数/浮点转换、整数值浮点字面量及指数表示、常量算术/位移、队列算术、函数参数/方法接收者包装。
- `reflect.DeepEqual`、`strings.EqualFold` / `Compare`、`bytes.Equal` / `Compare`、`cmp.Compare`，包含函数别名。
- 数值字符串重新赋值后再经变量中转，避免只记住首次非数值赋值导致漏判。

`r89_queue_guard_test.go` 包含 32 个等价写法用例和 1 个导入别名用例；11 个反向用例覆盖登记表查找、查表结果的数值字段、合法模式字符串、队列身份比较、无关变量及局部别名遮蔽。原 R88 三类回归保持。

边界已写入守卫注释：这是单文件、保守的语法和数据流检查，不是完整语义证明。名为 `QueueID` / `queueID` 的字段和裸变量均作为来源，所以同名但无关的变量仍可能被报出；别名按绑定区分作用域。包装函数保守传播参数，赋值分析不区分先后分支。暂不覆盖跨函数函数体、外部包常量求值、堆/集合别名、动态反射调用、编码后的字面量集合及隐式 iota/常量继承。登记表索引刻意不传播来源，以免误伤 `queue_groups.go` 分类结果。复核中发现的同名变量边界已明确记录，未通过收窄原检测规则来消除警报。

8 项独立回退变异全部被拦截：原始直接比较、别名、switch，数值字符串、字符串化调用、重新赋值、比较 API，以及 Q3 的存储标记。见 [mutations.json](r89-r88-followup/mutations.json)。脚本 `scripts/r89-followup-mutation-check.py` 使用临时 Go overlay，不修改生产文件。旧 `scripts/r88-mutation-check.py` 同步了护栏/标记的源码定位点；未将 R88 历史 19 项结果冒充本轮重跑结果。

## Q3 · 统一落盘构建标记

`storage.go:marshalDiagnosticRecord` 在复制事件后注入当前 `buildFingerprint`，覆盖生产者误带的旧值，不修改调用者 map。该编码器覆盖正常写入、轮转标记及轮转后重新编码的触发事件；归档旧行只移动，不重写构建身份。`main.go:appendDiagnosticEvent` 移除重复注入，保留原有写入失败处理。启动 handler 仍直接取得存储错误，成功落盘返回 204，失败返回 503。

全仓诊断写入口清单：

| 入口 | 事件范围 | 最终路径 |
|---|---|---|
| `main.go` 初始化 `championProvider.diag` 回调 | `arena_rankings_parse_failed` 及其他早期 provider 事件 | `store.appendDiagnostic` → `appendDiagnosticLocked` → `marshalDiagnosticRecord` |
| `main.go:appendDiagnosticEvent` | 常规 app 事件，包括 `live_load_cost`、`champselect_trace` 及聚合/去重记录 | 同上 |
| `desktop_startup_stage.go:handleDesktopStartupStage` | `desktop_startup_stage` | 同上，保留存储错误返回 |
| `storage.go:appendDiagnosticLocked` 内部 | `log_rotated` 及触发轮转事件 | 直接调用同一编码器 |

全仓检索 `appendDiagnostic`、`appendDiagnosticLocked`、`marshalDiagnosticRecord` 与 `diagnostics.jsonl`，没有发现其他生产代码绕过编码器写后端诊断 JSONL。独立桌面日志、工具 `trace.jsonl` 不属于此保证；测试合成数据与历史归档也不冒充当前构建新事件。R88 账本和 `DESIGN.md` 已明确范围。

验证：真实 YOUR.GG 解析器触发无效 tier 单行诊断并读取落盘记录；真实启动 HTTP 校验标记及写入失败语义；常规事件、轮转标记、旧标记覆盖和调用方不变性回归。启动 shell/HTTP/导出夹具也检查所有导出事件的实际标记。历史 A/B 报告器只接受 12 位十六进制 release 标记，因此端到端测试在串行测试作用域模拟 release 构建并恢复原值，不再通过事件载荷伪造标记。另以 `-ldflags '-X main.buildFingerprint=r89-followup-proof'` 验证新诊断单测确实读取当前链接标记。

## 验证结果及工作区边界

- R89 队列与诊断、原 R87/R88 护栏/诊断、完整启动阶段 HTTP/shell/导出/变异测试通过；相关竞态结果见 [targeted-race.txt](r89-r88-followup/targeted-race.txt)。
- `go vet ./...`、`git diff --check` 通过；前端红线及 Chromium 结果如上。
- 全量 Go 回归结果见 [full-go-test.txt](r89-r88-followup/full-go-test.txt)。当前工作区还有三个独立失败：`TestOverviewCurrentGameReadOnlyCacheAndPrivacy` 返回 502；`TestR86RiotQueuedDetailsExitWhileActiveRequestsHoldSlots` 和 `TestRiotOverviewCapsMatchDetailConcurrencyAtFour` 仍断言并发 4，而现有另一项功能修改默认并发为 8。
- 已用临时 overlay 撤去本轮 `storage.go` / `main.go` 的诊断标记变更，三个失败仍逐一复现，见 [baseline-unrelated-tests.txt](r89-r88-followup/baseline-unrelated-tests.txt)。因此不声称当前工作区全量 Go 回归通过；也未为本轮验收撤销其他工单的并发/对局功能修改。启动摘要失败与本轮标记下沉有关，已修正夹具并单独复验通过。

复跑命令：`node --test web/r88.test.cjs`；`R89_LAYOUT_OUTPUT=docs/r89-r88-followup/recheck node desktop/r89-arena-detail-layout.cjs`；`python3 scripts/r89-followup-mutation-check.py`。Go 命令使用项目内 `.gocache` / `.gopath`，HTTP 与 Chromium 测试需要本机回环监听权限。
