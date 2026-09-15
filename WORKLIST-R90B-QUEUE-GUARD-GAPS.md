# WORKLIST-R90B — queueID 静态守卫再补六个盲区（R90 验收追加）

诊断日期：2026-09-14
来源：`WORKLIST-R90-QUEUE-GUARD-GAPS.md` 已执行完毕（4 种指定写法 + 5 种自主追加方向共 9 处，独立验收逐一手动关闭对应检测代码复测，**确证全部是真实护栏，账本可信**）。但独立验收时又构造了 16 种全新绕过手法，**其中 6 种再次成功绕过，一个没拦住**。

这是本项目**第七次**在"AST 静态守卫只认窄模式"上翻车（此前六次：直接比较、变量中转、switch-case、字符串化、R89 验收发现的 4 种、R90 自主追加的 5 种，均已修过）。

不需要重跑 R90 已验收的 9 处，那部分结论成立。本工单只处理这轮新发现的 6 处。

---

## 背景

守卫要求：champselect 相关文件里禁止 queueID 与整数字面量产生等价比较，必须走 `queue_groups.go` 的登记表。历史教训：真实的"斗魂自动禁用失效"bug 根因就是硬编码 queueID 漏了 1750。

R90 已经把守卫从"能认字面量比较"扩展到能认 map 查表、range 遍历、interface 装箱断言、指针解引用、闭包捕获、类型 switch、iota 常量、`slices.Contains` 等 9 种模式。**独立验收再构造了 16 种全新写法去试探，6 种成功绕过：**

1. **channel 传值后 select 接收再比较**：
   ```go
   ch := make(chan int64, 1)
   ch <- session.QueueID
   select {
   case v := <-ch:
       return v == 1750
   }
   ```
2. **channel 传值后普通 receive 再比较**（不经过 select）：
   ```go
   ch := make(chan int64, 1)
   ch <- session.QueueID
   v := <-ch
   return v == 1750
   ```
3. **struct 字段间接持有 queueID 再比较**：
   ```go
   type wrap struct{ q int64 }
   w := wrap{q: session.QueueID}
   return w.q == 1750
   ```
   ⚠️ 和手法 1 类似，这种写法平时代码里很自然会出现（把 queueID 塞进一个业务结构体字段），最容易被无意中写出来，不一定是故意绕过。
4. **queueID 序列化成字符串再比较**：
   ```go
   b, _ := json.Marshal(session.QueueID)
   return string(b) == "1750"
   ```
5. **queueID 编码进二进制再比较**：
   ```go
   buf := make([]byte, 8)
   binary.BigEndian.PutUint64(buf, uint64(session.QueueID))
   target := make([]byte, 8)
   binary.BigEndian.PutUint64(target, 1750)
   return bytes.Equal(buf, target)
   ```
6. **包级泛型函数包一层比较逻辑**：
   ```go
   func genericEq[T comparable](a, b T) bool { return a == b }
   // 调用点：
   return genericEq(session.QueueID, int64(1750))
   ```

## 根因定位（独立验收已经查好，直接用）

- **手法 1、2（channel）**：`bind()` 不处理 `*ast.SendStmt`，channel 的发送端到接收端这条链路完全断掉，taint 标记传不过去。
- **手法 3（struct 字段）**：`isQueue()` 没有 `*ast.CompositeLit` 分支——完全没建模"把一个 queueID 相关值塞进结构体字段初始化"这件事。
- **手法 4（json.Marshal）**：`bindValues` 在函数调用返回 2 个值、且不是 comma-ok 赋值、也不是 `TypeAssertExpr`/`IndexExpr` 的情况下，会直接放弃绑定，不再往下传播来源标记。
- **手法 5（binary 编码写入）**：`PutUint64` 这类通过函数参数副作用写入切片的模式，对静态分析不可见——没有返回值可以追踪，来源标记从"写入"这个动作上完全丢失。
- **手法 6（泛型包装）**：调用点 `genericEq(session.QueueID, int64(1750))` 本身不是 `*ast.BinaryExpr`，守卫只认比较表达式节点本身，不会把"调用一个通用比较函数、且实参之一是 queueID"这种模式识别为等价的比较行为。

这 5 类根因目前已经在 Limits 免责声明里被抽象地点到了大部分（比如"channel/range-function sources are not modeled"覆盖手法 1、2；"nested collection/struct field aliases...not modeled"大致覆盖手法 3；"function-returned collections...not modeled"勉强覆盖手法 4；"Dynamic population (index writes, append/copy)...not modeled"覆盖手法 5；"not constitute...interprocedural analysis"覆盖手法 6）。**但手法 4 的具体机制（`bindValues` 对非 comma-ok 的多返回值调用直接放弃绑定）没有被精确写出来**，这条要补上。

---

## 要做的改动

### 1. 修复五类根因

- **`bind()` 加入 `*ast.SendStmt` 与对应 receive 的绑定追踪**：`ch <- x` 之后，从同一个 channel 变量接收到的值（无论是 `v := <-ch` 还是 `select { case v := <-ch: }`）都应该继承 `x` 的来源标记。可以做保守的同 channel 变量、同函数作用域内的绑定，不必解跨 goroutine 的调度顺序。
- **`isQueue` 加入 `*ast.CompositeLit` 分支**：当一个结构体字面量的某个字段被赋值为 queueID 相关表达式时，记录"这个类型的这个字段是 queueID 来源"；后续对该字段的 `*ast.SelectorExpr` 取值应继承这个标记。
- **`bindValues` 补上非 comma-ok 多返回值场景的保守处理**：函数调用返回多个值时，不应该直接放弃绑定，至少对第一个返回值做来源传播（尤其是 `json.Marshal` 这种"值, error"的常见模式）。
- **`found` 收集逻辑加入对"参数副作用写入"模式的检测**（手法 5）：形如 `binary.*.Put*` 这类函数，第一个参数是切片、后续参数含 queueID 来源，之后对该切片做 `bytes.Equal`/`bytes.Compare` 比较的，应视为等价比较。这条如果实现成本过高，至少要在 Limits 里精确点名这个具体场景（而不是泛泛的"dynamic population"）。
- **`found` 收集逻辑识别包级泛型包装函数**（手法 6）：如果调用的是一个签名形如 `func[T comparable](a, b T) bool` 的包级函数，且某个实参来源被标记为 queueID，应视为等价比较。

如果时间/复杂度不允许全部修完，**优先级：手法 3（struct 字段）> 手法 6（泛型包装）> 手法 4（json.Marshal）> 手法 1/2（channel）> 手法 5（binary 编码）**——前两种在正常业务代码里最容易被无意写出来，后两种更像是刻意绕过，出现概率相对低。哪怕暂时修不完，也必须完成下面第 2 条。

### 2. 更新 Limits 免责声明

- 把手法 4（`bindValues` 对非 comma-ok 多返回值直接放弃绑定）精确写进去，不要只用"function-returned collections"这种泛泛的话带过。
- 如果手法 5（binary 编码写入）没有修完，也要精确点名"通过函数参数副作用写入 slice/buffer 的模式不可见"，而不是笼统的"dynamic population"。
- 检查这次没有列出的、Limits 里原有的措辞是否还准确，不要因为本轮改动又让某句话变得比实际覆盖面更宽。

### 3. 别再头痛医头——这次请谨慎评估是否还要主动多想

前六轮的经验是：每次"主动多想几种"之后，独立验收总能再挖出新的，这已经不是巧合而是这类语法级 AST 守卫的结构性局限。**这次修完后，除非你有把握想到的手法确实不在已知的"数据流类别"范围内，否则不必强行再凑 3~5 种新的**——请改为在改动说明里评估一句：这类"看语法长相"的静态守卫，是否已经到了该转向 SSA 数据流分析工具（如 `golang.org/x/tools/go/ssa`）或改用独立 `QueueID` 类型从源头杜绝裸整数比较的阶段。这个评估不需要落地实现，写清楚你的判断和理由即可，供下一轮决策参考。

### 4. 补变异测试

对手法 1~6（实际修复的部分）每种都要有变异测试：证明当前代码能拦住 → 故意破坏对应检测逻辑 → 证明测试真的变红 → 恢复 → 确认变绿。沙箱是多会话共享工作区，**同一个文件的"破坏→测试→恢复"必须在一次 bash 调用里原子完成**，避免被其他并行会话的改动打断导致回滚不完整。

---

## 验收判据

- 手法 1~6 中实际修复的部分，逐一构造独立可编译 Go 代码片段，跑守卫测试，必须被拦截。
- 未修复的部分，必须在 Limits 里有精确到位的说明（不能用泛泛的话掩盖具体机制）。
- 反向验证：至少 2 种合法写法不能被误报。
- R90 已验收的 9 处不能因为本次改动引入回归。
- 提交时附一份"手法 1~6 各自的处理结果（修复/未修复+原因）"清单，以及第 3 条要求的"是否该换工具"的评估结论。
