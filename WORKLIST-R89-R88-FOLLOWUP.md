# WORKLIST-R89 — R88 验收遗留三项收尾

诊断日期：2026-09-14
来源：R88 验收（六路独立复核）挖出的三个真实缺口，均**不影响 R88 本身已验收的结论**，属于遗留清理项。三条都很小，建议一次性做完。

---

## Q1 · gameplay.css 里还有第 4 条同类死代码，账本给的保留理由是错的

### 背景
R88 已经删掉了三条从未命中的 `@container arena-first (max-width:720px/520px/420px)` 死代码（`arena-first` 这个容器名从未在总览页任何祖先节点声明过）。但 `web/gameplay.css` 里还留着**第 4 条同类规则**：
```css
@container arena-first (max-width:640px) { .arena-match-detail ... }
```
R88 执行账本（`docs/r88-execution-ledger.md` 第31行）给的保留理由是「英雄详情可使用的独立 arena-first 640px 详情滚动规则保留」——**这条理由被验证是错的**。

### 已证实
`.arena-match-detail` / `.arena-detail-columns` / `.arena-detail-team` 这几个类名只在 **`web/gameplay.js:2907`** 附近被渲染（总览页对局卡片展开详情用的），跟声明 `container: arena-first` 的 `web/champions.css:877 .arena-first-section`（英雄详情页"我的吃鸡战绩"区块）**完全不在同一棵 DOM 祖先链上**。也就是说这条规则和被删的三条性质完全相同：**永远不会命中**，是死代码。

`web/champions.css:847` 附近另有一条**真正被使用**的 `@container arena-first (max-width:640px)` 规则（详情页滚动用的）——**这条要留着，别删错**。两条规则容器名相同、断点数值也相同（640px），很容易在清理时删错文件，请特别小心核对：**删 `web/gameplay.css` 里的，留 `web/champions.css` 里的**。

### 要做的改动
1. 定位并删除 `web/gameplay.css` 里这条 `@container arena-first (max-width:640px) { .arena-match-detail ... }` 死代码块。
2. 删除前后各截一次图 / 跑一次现有的红线护栏测试，确认 `.arena-match-detail` 相关的详情展开功能在真实交互下没有受影响（这条规则从未命中过，删掉理论上零视觉变化，用来验证"确实无影响"这个判断）。
3. 同步更新 `docs/r88-execution-ledger.md` 或本轮的新账本，把那条错误的"保留理由"改正。

### 验收判据
- `git diff web/gameplay.css` 只有这一条规则被删除，其余内容不变。
- `web/champions.css:847` 附近的同名规则逐字未动。
- 对局详情展开交互（真 Chromium 测试或护栏脚本）行为不变。

---

## Q2 · AST 守卫漏了"queueID 转字符串后比较字符串字面量"这种绕过手法

### 背景
R88 把处理斗魂自动禁用的 queueID 硬编码问题修好了，并加固了 AST 静态守卫（禁止 champselect 相关文件里出现 queueID 与整数字面量的直接比较）。验收时验证了三种已知绕过手法（直接比较、变量中转、switch-case）确实都被拦住了。

**但验收时又想出了第四种手法，实测完全没被拦住**：
```go
strconv.FormatInt(session.QueueID, 10) == "3110"
```
把 queueID 转成字符串后，去和一个字符串字面量比较，语义上和硬编码整数比较完全等价，但因为守卫的 `isInteger()` 只认 `token.INT` 类型的 `*ast.BasicLit`，字符串字面量是 `token.STRING`，直接漏过。

这是本项目历史上第四次在同一类"静态守卫只认窄模式"上翻车（此前已有直接比较、变量中转、switch-case 三次教训），说明这类守卫需要一次性做宽，而不是每发现一个漏洞补一个分支。

### 要做的改动
1. 找到 AST 守卫的实现（`r87_test.go` 里的 `arenaLiteralComparisons` / `isInteger` 等函数，R88 应该已经改名或扩展过，找当前的实际位置）。
2. 扩展检测范围，覆盖"queueID 经过字符串化转换后再与字符串字面量比较"这种模式。具体识别：`==`/`!=` 的一侧是对某个被判定为 queueID 的表达式调用 `strconv.FormatInt` / `strconv.Itoa` / `fmt.Sprintf("%d", ...)` 之类做字符串化，另一侧是 `token.STRING` 的 `*ast.BasicLit` 且内容能解析成整数。
3. **不要只补这一种手法就停手**——请再想一轮还有没有其他等价绕过方式（比如：把 queueID 强转成 `float64` 再比较、包一层无意义的函数调用再比较、用 `reflect.DeepEqual` 之类），能覆盖的一并覆盖，或者至少在守卫的注释里列出"已知覆盖的手法清单"和"已知无法覆盖的手法清单"，让下一个人心里有数。
4. 补三条变异测试：①字符串化直接比较；②字符串化+变量中转；③保留原有三种手法的回归测试不能失效。

### 验收判据
- 新增的字符串化绕过手法必须被守卫拦截。
- 原有三种手法（直接比较、变量中转、switch-case）继续被拦截，不能因为改动引入回归。
- 合法写法（比如用 `queue_groups.go` 的登记表 `map[int64]...` 查表）不能被误伤，要有一条"正确写法不报警"的反向测试。

---

## Q3 · 两类诊断事件没有带 build_fingerprint，账本"所有事件都带"的说法不准确

### 背景
R88 加了 build_fingerprint 标识，目的是让以后统计任何诊断事件时，能分清是哪个 build 产生的（R87/R88 验收时都因为一份日志里混了新旧两个 build 而踩过坑，且两个 build 的 `version` 号还相同）。

账本原话「所有后端诊断事件都附加当前 build fingerprint，包含聚合与去重事件」——**验收时发现这句话不准确**。

### 已证实
标准路径是通过 `a.appendDiagnosticEvent`（`main.go:1495` 附近）写入的事件才会被自动打上 `build_fingerprint`。但至少有两类事件绕开了这条标准路径，直接调用了更底层的写入函数：
1. **`arena_rankings_parse_failed`**（R88 本轮刚加的新诊断）：走 `main.go:366` 附近的 `championProvider.diag = func(event){ _ = store.appendDiagnostic(event) }`，**直接调 `store.appendDiagnostic`，绕过了 `a.appendDiagnosticEvent`**，所以不带 build_fingerprint。
2. **`desktop_startup_stage`**：在类似的独立文件里（`desktop_startup_stage.go:39` 附近）也是同样的绕过模式。

这意味着：下次如果要排查"某个 arena 榜单解析失败是不是新 build 才有的问题"，或者"某次启动阶段异常是不是新 build 引入的"，**日志里现在还是看不出来**——这恰好是 R87/R88 两轮都吃过亏的那个坑，这次自己新加的诊断又踩了一遍。

### 要做的改动
1. 把 `arena_rankings_parse_failed` 和 `desktop_startup_stage` 这两处的写入路径改为经过 `a.appendDiagnosticEvent`（或者给 `store.appendDiagnostic` 这类底层函数本身补上 build_fingerprint 注入，让所有写入路径都统一经过同一处打标签的逻辑，而不是在每个调用点各自记得加——后者更容易再漏）。
2. **顺手全仓库搜一遍**，还有没有其它诊断事件是直接调用 `store.appendDiagnostic` 或类似底层函数、绕过了 `a.appendDiagnosticEvent` 的。R88 这次只抓到了 2 个，不代表只有 2 个。
3. 把账本/文档里的表述改成准确的版本，比如「经过 `appendDiagnosticEvent` 统一入口的诊断事件都带 build_fingerprint」，而不是笼统的"所有"。

### 建议的更稳妥做法
与其在每个诊断事件的调用点各自记得"要带上 build_fingerprint"，不如把这个字段的注入点**下沉到唯一的落盘函数**（无论最终是哪个函数真正写文件），这样以后新增任何诊断事件都不需要额外操心这件事，从根上杜绝"又漏了一个"的问题。请评估这个方向是否可行，如果不可行（比如底层函数需要保持精简、不感知 build 信息），至少要在代码里加一条静态检查或运行时断言，防止未来再有诊断事件绕过标准入口。

### 验收判据
- 单元测试：触发 `arena_rankings_parse_failed` 和 `desktop_startup_stage`，验证落盘的事件带有正确的 `build_fingerprint`。
- 全仓库扫描确认没有其它诊断事件绕过标准入口（或者列出一份完整清单，标注哪些是有意为之的例外、为什么）。
- 回归：原本就带 build_fingerprint 的事件（如 `live_load_cost`、`champselect_trace`）继续正常。

---

## 执行顺序建议

三条互不依赖，可以任意顺序或并行做。Q1 改动最小（删几行 CSS），Q3 收益最高（避免以后每一轮都要靠人工按 build 拆分日志才能下结论），Q2 是护栏加固、不影响现有功能。建议顺序：**Q3 → Q2 → Q1**。

---

**验收状态（2026-09-14）**：Q1/Q2/Q3 均已执行，六路独立复核 Q1 属实、Q3 基本属实（两处措辞细节待订正，见执行账本）。**Q2 复核时发现新的绕过手法，已拆分为独立工单 `WORKLIST-R90-QUEUE-GUARD-GAPS.md`，请执行那份文件。**
