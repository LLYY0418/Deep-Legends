# WORKLIST-R90 — queueID 静态守卫补四个盲区（R89-Q2 验收追加）

诊断日期：2026-09-14
来源：`WORKLIST-R89-R88-FOLLOWUP.md` Q2 已执行完毕（32+1 个等价用例、11 个反向用例、8 项变异测试全部独立复现为真实护栏，账本可信），**但六路独立复核时又想出了新的绕过手法，全部成功**。这是本项目**第五次**在"AST 静态守卫只认窄模式"上翻车（此前四次：直接比较、变量中转、switch-case、字符串化，均已在 R87/R88/R89 修过）。

不需要重跑 Q1/Q3，那两项已验收通过。本工单只处理 Q2 的后续。

---

## 背景

Deep Legends 里 champselect 相关文件禁止 queueID 与整数字面量直接比较（要求统一走 `queue_groups.go` 的登记表判断，例如斗魂 arena 模式要用 `groupID=="arena"` 或 `isArenaQueue()`，不能写 `QueueID==1700`）。这条规则背后有真实教训：R87/R88 修的"斗魂自动禁用失效"两个 bug，根因都是某处代码写死了 queueID 数字比较、漏了斗魂实际用的 1750。

`r88_queue_guard_test.go` 里的 AST 静态守卫就是防止同类问题再犯的护栏。R89 刚把它从"只认三个 Arena 队列号的字面量比较"扩展到能识别变量中转、switch-case、字符串化转换（`strconv.FormatInt`/`Itoa`/`fmt.Sprintf`）等手法。

**独立验收时又想出 4 种新手法，全部成功绕过，一个没拦住：**

1. **字面量 map 当查表用**：
   ```go
   isTarget := map[int64]bool{1750: true, 1760: true}[session.QueueID]
   ```
   ⚠️ **最危险的一种**。这个写法在代码审查时和"真正查 `queue_groups.go` 登记表"长得几乎一样，最容易被放过、也最容易被将来的开发者无意中写出来（而不是故意绕过守卫）。
2. **map 查函数表**：
   ```go
   handlers := map[int64]func() bool{1750: func() bool { return true }}
   ```
3. **range 遍历字面量 slice 比较循环变量**：
   ```go
   for _, v := range []int64{1700, 1710} {
       if session.QueueID == v { ... }
   }
   ```
4. **interface{} 装箱后类型断言再比较**：
   ```go
   var v interface{} = session.QueueID
   n := v.(int64)
   if n == 3110 { ... }
   ```

## 根因定位（已经帮你查好了，直接用）

守卫实现在 `r88_queue_guard_test.go`（核心函数 `arenaLiteralComparisons`、`isQueue`、`constantValue`、`bind` 等，具体行号请在当前代码里重新核实，R89 改动后行号可能已漂移）。

三个具体盲区：
- `found` 收集逻辑只认 `*ast.BinaryExpr` / `*ast.SwitchStmt` / 特定 `*ast.CallExpr` 三类节点，**完全不处理 `*ast.IndexExpr`**（map/slice 下标访问）——这是手法 1、2 绕过的原因。
- `bind()` 只遍历 `*ast.AssignStmt` / `*ast.ValueSpec` 做变量绑定追踪，**不遍历 `*ast.RangeStmt.Value` 的绑定**——这是手法 3 绕过的原因。
- `isQueue` 的别名传播链遇到 `*ast.TypeAssertExpr` 会直接断掉，不继续往下传播"这是个 queueID"的标记——这是手法 4 绕过的原因。

这三点是真实存在的盲区，但**没有被写进守卫代码里的"Limits"免责声明段落**——现有免责声明只提了"no interprocedural body analysis / imported constant evaluation / heap alias tracking"这类更笼统的话，反而让人以为覆盖面比实际更大。

---

## 要做的改动

### 1. 修复三个盲区

- **`found` 收集逻辑加入对 `IndexExpr` 的检测**：当索引表达式的对象是字面量 `map`/`slice`（`*ast.CompositeLit`），且其 key 或 element 里含有目标队列号（1700/1710/1750，或者更通用地——任何被判定为"看起来像队列号"的整数字面量），同时下标表达式命中 `isQueue()` 判定的变量，就应该报警。
- **`bind()` 加入 `RangeStmt.Value` 的绑定收集**：`for _, v := range []int64{...}` 里的循环变量 `v` 如果后续被拿去和 `isQueue()` 判定的变量做比较，要能追踪到这层绑定。
- **`isQueue` 传播链加入 `TypeAssertExpr` case**：`v.(int64)` 这种类型断言不应该打断别名传播——如果 `v` 是从一个 queueID 相关变量装箱来的（哪怕只能做保守的同名/同作用域判断，也比完全断链好）。

### 2. 更新 Limits 免责声明

不管上面三点补没补完，**已知盲区都要明确写进守卫代码的"Limits"注释段落**，不要让盲区只存在于验收报告里、下一轮又被动发现一遍。这条是本工单里优先级最高的一条——哪怕代码暂时改不完，至少文档要诚实。

### 3. 别再头痛医头——主动再想几种

这已经是第五次了。请在修完上面四种手法后，**再花时间主动想 3~5 种你自己能想到的新绕过方式**，不要止步于本工单列的四种。可以从"AST 遍历还漏了哪类节点"这个角度系统性想一遍：比如 `*ast.SelectorExpr` 链式调用返回值再比较、`*ast.StarExpr` 解引用指针变量、闭包捕获后在另一个函数字面量里比较、`const` 声明的队列号常量组（`iota` 或具名常量）等。**想到几种测几种，别等下一轮验收又来揭一次。**

### 4. 补变异测试

对上面 1~4 种手法（以及你自己额外想到的），每种都要有一条变异测试：先证明当前代码能拦住，再故意破坏检测逻辑证明测试真的会失败（不是摆设），修好后再确认变绿。

---

## 验收判据

- 本工单列出的 4 种绕过手法，逐一构造独立可编译的 Go 代码片段，跑守卫测试，**全部必须被拦截**。
- 反向验证：至少 2 种合法写法（通过 `queue_groups.go` 登记表查询、与队列无关的普通变量比较）**不能被误报**。
- 守卫代码的 Limits 免责声明段落里，必须能看到这次新发现的盲区（不管修没修完，至少要写明）。
- 提交时附一份"这次我自己额外想了哪几种绕过手法、结果如何"的清单，哪怕全部被拦住了也要写出来——这是证明"真的认真想过"而不是走过场的方式。
