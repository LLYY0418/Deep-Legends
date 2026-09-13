# R82 补充加固工单 · 独立验收报告

验收人：Claude（只读验收，未改仓库任何文件；全部变异在 `/tmp` 副本中进行）。
日期：2026-09-12。对象：`WORKLIST-R82-STARTUP-HARDENING-ADDENDUM.md` A~E。

---

## 一、总评

**C、D、E 三节真实落地**，没有发现造假。**A、B 两节的核心思路是对的、但留了一个
我亲手复现过的真实绕过**——之前的问题没被根治，只是绕过方式变了。

### 基线

根模块 `go test ./...` ok（69.3s）、`installer` 模块 `go test ./...` ok（含 `internal/webviewhost`）。

---

## 二、★ A/B 节：AST 守卫是本轮最大发现，有真实绕过

### 背景

上一轮我发现 `installer/*_windows.go` 在 Linux 上根本不参与编译，导致两条约束——
"升级必须走窗口接力"、"外壳退出不许杀应用"——**代码是对的，但没有任何护栏**。

这轮 GPT 做了两件事：

1. **把逻辑抽成平台无关的 `installer/completion.go`**（`completeInstallation`/
   `finishInstallerHandoff`，用 hooks 结构体解耦）——**这部分设计是扎实的**，
   `finishInstallerHandoff` 接收的 `installerExitHooks` 结构体里根本没有"终止进程"
   这个字段，类型系统层面就杜绝了误杀。
2. **新增"AST 守卫"测试**：用 Go 自带的 `go/parser` 在 Linux 上直接解析
   `install_windows.go`/`handoff_windows.go` 的**源码文本**，逐字节比对指定函数的
   代码块有没有被改动过——这是个聪明的技巧，绕开了"Windows 文件在 Linux 不编译"
   这个物理限制。

### 但我亲手验证出一个真实绕过

问题在于，这个 AST 守卫**只解析两个具名文件里两个具名函数**（`install_windows.go`
里的 `install`、`handoff_windows.go` 里的 `handoffApplication`），完全不管：

1. 同一个文件里是否存在**另一个函数**绕开了 completion.go；
2. **谁在调用** `install()`——真正的调用点在 `webview_windows.go`，AST 守卫压根不解析这个文件。

我在 `/tmp` 副本里新增了一个函数：

```go
func installFast(a *installerApp) {
    postMessage.Call(a.window.hwnd, WM_CLOSE, 0, 0)
}
```

然后把 `webview_windows.go` 里 `case "install":` 的分发改成：

```go
case "install":
    if a.options.Update {
        go installFast(a)
        return
    }
    // ...原有逻辑...
```

结果：

| 检查 | 结果 |
|---|---|
| Linux `go test ./...`（含两条 AST 守卫） | **全部 PASS** |
| `GOOS=windows GOARCH=amd64 go build ./...` | **exit 0，零输出** |
| `GOOS=windows GOARCH=amd64 go vet ./...` | **exit 0，零输出** |

这就是本工单原本要防的那个 bug（升级不走接力、外壳直接退出）**一字不差地又活了过来**，
而且不需要碰任何被检查的函数体，只需要"新开一个函数、换个调用点"。

### 怎么理解这个结果

AST 守卫本质是对"两个已知缝合点内部代码形状"的强绑定快照（类似 golden test），
**不是对"--update 必须经过 completion.go"这条不变量的证明**。上一轮的漏洞是
"整个文件不参与编译"，这一轮换成了"新开一个函数就能绕过检查名单"——问题没有
被根治，只是绕过的门槛从"完全没人管"变成了"知道这个技巧的人才能绕过"。

**行为测试本身没有问题**：`TestInstallationSuccessAlwaysRunsHandoffIncludingUpdate`
真的覆盖了普通/升级 × 可见/超时四种组合；`TestInstallerExitOnlyClosesItsWindowAndKeepsChildAlive`
真的启动了一个子进程、用 ping 验证退出流程走完后子进程还活着（子代理把里面的 fake
关闭钩子偷偷换成真 `Kill()`，测试立刻抓到，证明这个存活检查有牙齿）。**问题只出在
AST 静态检查这一层的覆盖边界**——9 处独立变异里 8 处被拦住，只有"新增旁路函数"
这 1 处放行。

### 建议

至少加一条检查：`webview_windows.go` 里 `case "install":` 分支只能有一条无条件的
`go a.install(message)` 调用（同样可以用 AST 或者简单的字符串行数比对做到）。
更根本的做法是从架构上让"install 消息只有一个可能的处理入口"，让"多态入口"这件事
本身变得不可能，而不是靠不断加检查清单去堵后门。

---

## 三、C、D、E 节：真实落地

### C · 死代码清理

`runFallback`/`runInstallerFallback`/`shellOpen` 重新 grep 全仓库源码零命中
（只在文档/报告里被提及），编译、测试、Windows 交叉编译全部干净。README 里
"缺少 WebView2 自动回退旧安装向导"的描述已修正，和 `main_windows.go` 的实际行为
（运行时缺 WebView2 就报错退出，没有任何 fallback）一致。

### D · 升级文案与模板的耦合测试

`TestUpdateCompletionWordingStaysCoupledToTemplate` 真的读取
`//go:embed` 进来的真实 `ui/installer.html`、真的调用生产环境同一个 `renderInstallerUI`
函数，不是自建假 HTML 或重新实现一遍替换逻辑。两处独立变异都命中：

| 变异 | 结果 |
|---|---|
| HTML 原文改一个字（模拟"两边失联"） | FAIL ✅ |
| 渲染函数跳过替换、直接返回原文 | FAIL ✅ |

"目标文案必须恰好出现一次"这条防护也验证是真的——在 HTML 里多塞一份重复文案，
测试确实是因为"出现次数不对"报错，不是蒙对了别的检查分支。

### E · webviewhost 偶发 flake

上一轮我遇到过一次偶发 FAIL，这轮由两名独立子代理分别重跑（合计 200 次以上，
含 `-race`、4 路 CPU 加压）**全部零复现**。"记为已知 flake、暂不处理"的结论方向合理，
但要注意这只能说明不是高频 flake，不能证明问题不存在。

---

## 四、方法论小结

这次最大的收获不是找到一个 bug，而是一条通用经验：**AST/静态解析型护栏，必须
额外测"新增一条完全独立的路径"这种变异**，不能只测"修改已知函数内部"。后者是
显眼的破坏动作，容易被写测试的人本能地想到去防；前者（加一个新函数）在日常开发里
完全不会被当成"危险操作"，是这类护栏的天然盲区，比直接改内部逻辑更容易被无意中触发。

## 五、建议的下一步

这个绕过目前只是**理论上存在**（我在沙箱里人为构造出来的），不是说现在的代码有问题——
现在的 `install_windows.go`/`handoff_windows.go` 本身没有引入 `installFast` 这类旁路。
是否值得再开一轮工单去补这最后一道检查，还是先记录在案、等真的有人不小心引入类似
旁路时再处理，这个优先级由你定。C、D、E 三节可以直接算过。
