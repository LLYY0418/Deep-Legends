# R90B queueID 守卫执行记录

2026-09-14。依据 `WORKLIST-R90B-QUEUE-GUARD-GAPS.md`。六项均修复；只改测试侧守卫、回归用例及验证脚本，没有修改业务队列登记表或自动选禁逻辑。已有工作区修改保留。

## 六项结果

| 手法 | 处理结果 | 独立变异证据 |
| --- | --- | --- |
| 1. channel → select receive | 已修复。SendStmt 将发送值来源绑定到同一 channel 标识符，select 接收通过 UnaryExpr/赋值继承。 | 关闭发送端 queue 标记后，`Required/channel-select` 精确失败：[日志](r90b-queue-guard/channel-select.txt)。 |
| 2. channel → 普通 receive | 已修复。同一传播链支持普通 receive；另覆盖 comma-ok 与从 channel 接收数值目标。 | 单独关闭相同发送检测，`Required/channel-receive` 精确失败：[日志](r90b-queue-guard/channel-receive.txt)。两个接收用例各自验证，未用兄弟用例代替失败。 |
| 3. struct 字段 | 已修复。CompositeLit 记录本地可解析类型的字段来源，SelectorExpr 按 go/types 字段对象读取。键名、位置初始化和指针访问有覆盖，其他字段与类型不混淆。 | 关闭字段 queue 绑定，`Required/struct-field` 精确失败：[日志](r90b-queue-guard/struct-field.txt)。 |
| 4. json.Marshal 多返回值 | 已修复。非 comma-ok CallExpr 多返回值传播第一结果的参数来源，不传播到 error/status 或后续结果；覆盖函数别名和三个返回值的第一槽。 | 关闭二返回值调用的首结果绑定，`Required/json-marshal` 精确失败：[日志](r90b-queue-guard/json-marshal.txt)。 |
| 5. binary 参数写入 buffer | 已修复。识别 encoding/binary 的 BigEndian / LittleEndian / NativeEndian.PutUint16/32/64，将第二参数的 queue/数值来源写入首参数的标识符；bytes.Equal/Compare 沿用来源比较。 | 只关闭写入的 queue 传播，`Required/binary-buffer` 精确失败：[日志](r90b-queue-guard/binary-buffer.txt)。LittleEndian、导入/方法别名和 Compare 也有同类回归。 |
| 6. 包级泛型比较 | 已修复。同文件包级 `func[T comparable](T,T) bool` 按签名登记，调用含 queue 来源实参即保守标记；显式实例化和函数别名继续有效。 | 关闭专用调用检测，`Required/generic-comparator` 精确失败：[日志](r90b-queue-guard/generic-comparator.txt)。 |

每个 fixture 都先通过独立程序的 `go/parser` 与 `go/types.Config.Check`（含真实标准库导入）才运行守卫。合法登记表位于独立 fixture 文件，模拟生产中的扫描边界。新增 `r90b_queue_guard_test.go`：6 个指定用例、11 个合法反例、8 个同类流程变体，没有扩张寻找新的绕过类别。

## 正反验证与误报修复

- 修改前六个指定用例全部漏报，见 [before.txt](r90b-queue-guard/before.txt)。不是仅凭推测补测试。
- 新来源传播曾让 `champselect.go` 的英雄池空判断和候选英雄为零判断产生三处误报，见 [首次生产扫描](r90b-queue-guard/repository-scan.txt)。原因是 `groupID → definition → champions` 中，返回的业务结构体被当成原始数值 queue 来源。
- 补入可编译的 `registry-metadata-pipeline` 合法反例，先实测失败：[metadata-before.txt](r90b-queue-guard/metadata-before.txt)。修复为：能在本地解析到的结构体返回值不整体继承参数的数值队列标记，字段依赖独立来源模型。六个指定样例均不受影响。[最终生产扫描](r90b-queue-guard/repository-scan-fixed.txt) 通过。
- 其余合法反例覆盖：channel 中无关 ID、channel 到登记表、其他结构体字段/类型、结构体字段查登记表、无关 JSON/binary 编码、仅使用 JSON error、无关泛型比较和泛型登记表结果比较。
- 旧 R88/R89/R90 队列规则执行普通回归，未重做 R90 已验收的 9 种破坏实验。旧变异脚本仅将 stringify-call 的锚点适配到新的 CallExpr 结构，全部旧锚点仍唯一；没有更改原来九项的检测逻辑或断言。

## 变异与最终检查

- [mutations.json](r90b-queue-guard/mutations.json)：六项 `killed=true`，`baseline_passed=true`、`restored_passed=true`、`source_unchanged=true`。每项要求精确子测试失败并包含漏报断言，不接受编译错误、工具链错误、兄弟用例失败或仅进程非零。
- `scripts/r90b-queue-guard-mutations.py` 在一次 shell 调用中完成绿色基线、六次临时 Go overlay 变异及最终绿色检查。实际工作区守卫从未被破坏，所以不需要抢占共享文件后再回滚。
- [最终完整队列专项](r90b-queue-guard/mutation-green-after.txt)：118 个叶子测试通过（含父级共有 127 个 pass 事件），零失败。此处“完整”仅指 R88/R89/R90/R90B 队列专项，不是全仓所有功能测试。
- 生产扫描 `TestR87ArenaLiteralBoundary` 连同上述专项通过；`go vet ./...`、gofmt 检查和 `git diff --check` 通过。最终命令与哈希见 [verification.json](r90b-queue-guard/verification.json)，起始版本差异见 [guard.diff](r90b-queue-guard/guard.diff)。
- 独立只读复核检查六项覆盖、变异有效性、Limits 和结构体返回值误报修复。主线程验证使用仓库内 `.gocache/.gopath`，不把独立会话默认工具链/缓存环境失败当成业务测试通过或失败。

复跑：

```sh
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go test -count=1 -run 'TestR87ArenaLiteralBoundary|TestR88QueueGuard|TestR89QueueGuard|TestR90QueueGuard|TestR90BQueueGuard' .
python3 scripts/r90b-queue-guard-mutations.py
GOCACHE="$PWD/.gocache" GOPATH="$PWD/.gopath" go vet ./...
```

## Limits 的精确边界

守卫源码顶部已重写 Limits，区分“这次修好了什么”和“仍然不建模什么”：

- 精确写明原 bindValues 丢弃非 comma-ok 多返回值调用的机制。现在只跟踪第一槽，后续槽、无 queue 实参但函数内部返回 queue 的情况仍不可见。局部可解析的结构体值返回不作为整体数值别名，字段需要可见来源；指向结构体的返回指针或无法解析的返回类型仍走保守参数传播，可能误报，不能据此宣称完整返回对象分析。
- binary 的支持限定于列出的标准方法、具名 buffer 和已知参数来源。自定义编码器、通过 slice 表达式/字段写入、AppendUint/Write、任意副作用和解码不建模；没有证明两种字节编码语义相等。
- channel 只沿同一标识符及其向前别名传播，不分析 goroutine 调度；经别名写回原绑定、跨函数 channel 或 channel range 不在覆盖内。
- struct 来源按本地类型/字段合并，区分其他字段与其他类型，但不区分同类型的实例；混合实例可能误报，也可能因两边均变为 queue 来源而漏掉比较。动态字段/指针/索引写入、任意堆别名仍不建模。
- 泛型只识别同文件、单个显式 comparable 类型参数、两个该类型参数和一个 bool 结果的包级签名。它不读函数体，即使另一个参数不是数字也可能保守标记。其他文件/包中的函数、约束别名和方法形式仍需下一阶段处理。
- 原来的集合字面量来源限制、导入常量/缺失声明、反射调用、flow-insensitive 合并及同名 queueID 的保守根规则继续保留。

六个指定手法均已处理，但守卫仍不是所有语义等价程序的证明器。

## 是否应换工具

**已到应当引入 SSA 数据流分析的阶段。** 连续七轮需要补充语法节点、赋值方式和标准库副作用，再加上本轮真实业务结构体误报，说明继续逐语法扩充的收益下降、交互成本上升。建议下一轮用 `go/analysis` + `go/ssa` 对这个明确扫描边界做原型，逐步统一跨基本块、字段读写和调用参数/返回值的来源传播，并将现有正反 fixture 作为迁移验收集。SSA 也需要明确调用图精度、别名策略和 json/binary 等标准库效果摘要，不能声称换工具即可完整证明所有语义等价写法。

独立 QueueID 类型适合长期缩小业务层可做的操作，但 **`type QueueID int64` 本身并不能禁止 `q == 1750`**：Go 允许与可表示的未定类型常量比较。若采用类型方案，应考虑在独立包中隐藏数值表示、只向业务暴露登记表分类/属性操作的对象接口，限制裸整数出口，并评估 LCU/JSON 边界适配成本。本轮只作决策评估，不扩大为 SSA 或领域类型迁移。
