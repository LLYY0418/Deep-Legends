# R90 · queueID 静态守卫四个盲区

2026-09-14。工单：`WORKLIST-R90-QUEUE-GUARD-GAPS.md`。本轮只修改队列静态守卫、相关测试和验收材料；R89 的 Q1/CSS、Q3/诊断标记保持原验收结果。工作区另有同编号的斗魂实时分组工单，其实现及账本未纳入本轮修改。

## 工单四种写法

| 写法 | 修改前 | 修改后及回退验证 |
|---|---|---|
| `map[int64]bool{1750:true,1760:true}[session.QueueID]` | 漏报 | 新增 `IndexExpr` 检测；关闭索引检测，该独立用例失败。 |
| `handlers := map[int64]func()bool{1750:...}` 后按 queueID 调用 | 漏报 | 跟踪本地集合变量及别名的数值 key；关闭集合别名传播，该独立用例失败。仅声明未使用的普通函数表不被当作队列判断。 |
| `range []int64{1700,1710}` 后与循环变量比较 | 漏报 | 将集合 key/value 的来源分别绑定到 `RangeStmt.Key/Value`；关闭 range 绑定，该独立用例失败。 |
| interface 装箱、`v.(int64)` 后比较 | 漏报 | `TypeAssertExpr` 保留来源；关闭断言传播，该独立用例失败。包含普通断言、逗号 ok 声明/赋值及类型 switch。 |

真实修改前失败输出见 [before.txt](r90-queue-guard/before.txt)，修改后见 [after.txt](r90-queue-guard/after.txt)。实现仍位于 `r88_queue_guard_test.go`；相对本轮开始的完整差异见 [guard.diff](r90-queue-guard/guard.diff)。

集合来源以 key/value 的数值与队列标记分别跟踪，经作用域绑定的别名作固定点传播。不把未知登记表的 queueID 索引当成其返回值来源，避免将合法的 `registeredQueueModeGroups[session.QueueID] == "arena"` 或登记表属性比较误报。支持本地具名 map/array/slice 类型、数值字符串 key、稀疏数组、集合元素取值、range 已声明变量、map key/value 遍历及局部遮蔽。

## 自主追加的五类探测

| 方向 | 可编译用例的核心写法 | 修改前实测 | 最终结果 |
|---|---|---|---|
| 指针解引用 | `p := &session.QueueID; return *p == 3110` | 漏报；`*p` 是 `StarExpr`，不是已有的 `UnaryExpr` 分支 | 补齐传播；专属回退变异被拦截。 |
| 闭包捕获 | `q := session.QueueID; check := func()bool{return q==1750}; return check()` | 已拦截 | 保留原覆盖；故意跳过闭包体扫描后，该独立用例失败。 |
| 类型 switch | `switch n := boxed.(type) {case int64: return n==1750}` | 漏报 | 断言及已有赋值扫描共同传播；专属回退变异被拦截。 |
| iota / 隐式常量继承 | `const(base=iota+1700; next); return session.QueueID==next` | 漏报 | 用 `go/types` 求值本文件可解析的常量，保留语法回退；关闭常量求值后该用例失败。 |
| 标准切片成员判断 | `slices.Contains([]int64{1700,1710,1750}, session.QueueID)` | 漏报 | 增加集合比较 helper 检测；覆盖 Contains/Index/BinarySearch、导入别名及显式泛型函数别名；专属回退变异被拦截。 |

组合核验另外发现“循环数值再强转/字符串化/做算术”会丢失数值来源。失败实录见 [composition-before.txt](r90-queue-guard/composition-before.txt)。已统一转换调用的值参数提取，并继续传播数值来源，忽略进制 10、格式串等格式化元数据。三种组合分别有回退变异验证。装箱切片 range、队列值经过集合元素取回等组合也通过。

## 测试不是仅解析语法

`r90_queue_guard_test.go` 共 49 个子用例：4 个指定写法、5 个额外方向、27 个组合、13 个合法反向用例。每个用例先由 Go 解析器及 `go/types.Config.Check` 检查整个独立程序，包含真实标准库类型检查；编译失败会直接失败，不能冒充守卫检出。

登记表 fixture 位于单独的 `queue_groups.go`，与被扫描的 `champselect_fixture.go` 共同编译；守卫只扫描后者，忠实模拟统一登记表位于扫描边界之外。反向用例包括登记表直接/别名/属性/逗号 ok 查询、无关变量、普通 map/函数表/range、类型断言/指针以及局部遮蔽。

当前 `TestR87ArenaLiteralBoundary` 对 `champselect*.go`、`arena_live*.go` 和 `gameplay_refresh.go` 的生产文件扫描也通过。扫描范围沿用已有工作区，本轮未修改该扫描器。原 R88/R89 队列测试保持通过。

## 变异与最终验证

- [mutations.json](r90-queue-guard/mutations.json)：22 项全部被对应护栏断言拦截，包含四个指定写法、五个额外方向、六个组合环节及七个旧护栏回退。每项保存了替换前后源码和目标测试，具体输出位于同目录。
- `scripts/r90-queue-guard-mutations.py` 先运行绿色基线，再逐项通过临时 Go overlay 关闭检测，最后重新运行未变异版本。要求目标测试精确失败、进程非零退出且出现护栏漏报断言；不接受编译错误、环境错误或兄弟用例代替失败。报告中的 `baseline_passed`、`restored_passed`、`source_unchanged` 均为 true。绿色实录：[之前](r90-queue-guard/mutation-green-before.txt)、[恢复后](r90-queue-guard/mutation-green-after.txt)。
- 仓库内 R87/R88/R89/R90 队列专项回归通过；[race.txt](r90-queue-guard/race.txt) 为相同范围竞态测试通过记录。
- `go vet ./...`、Go 格式检查、`git diff --check` 通过。命令及代码哈希见 [verification.json](r90-queue-guard/verification.json)。没有把本轮专项结果表述为全仓所有功能测试通过。

## 明确保留的边界

守卫源码的 `Limits` 已直接点名这次三个 AST 盲区及现有支持条件：

- map/slice `IndexExpr` 与 `RangeStmt` 要能追踪到本文件字面量或本地标识符别名；动态下标写入、append/copy 填充、嵌套集合/结构体字段、函数返回的集合、channel/range-function 来源不建模。
- `TypeAssertExpr` 只能延续已知来源，不能恢复此前经字段、指针写入、函数参数/返回值或反射丢失的来源。直接取地址/解引用和闭包捕获，不等于完整堆分析或跨函数分析。
- 常量求值只处理当前文件可解析的部分，不加载其他包的常量；反射构造调用、编码后的集合、自定义成员判断不作完整语义证明。标准 helper 的支持限定于源码列出的调用形式及别名，并不覆盖任意动态调用组合。
- 字段或裸变量名 `QueueID` / `queueID` 仍保守作为队列来源，可能误报同名无关变量。赋值分析不区分先后分支，混合来源可能保守误报，需要人工核对；别名遮蔽按 AST 绑定区分。

这些是已知边界，不代表护栏能识别所有语义等价程序。统一登记表与代码审查仍是约束的一部分。

## 复跑

```sh
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go test -run 'TestR87ArenaLiteralBoundary|TestR88QueueGuard|TestR89QueueGuard|TestR90QueueGuard' -count=1 .
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go test -race -run 'TestR87ArenaLiteralBoundary|TestR88QueueGuard|TestR89QueueGuard|TestR90QueueGuard' -count=1 .
python3 scripts/r90-queue-guard-mutations.py
```
