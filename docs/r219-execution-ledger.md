# R219 执行账本

2026-10-05；基线 0.12.71。范围：[R219 工单](WORKLIST-R219-CLIENT-CLOSED-LAUNCHPAD-ENTRIES-MISSING-AND-STARTUP-OVERLAY-15S-WRONG-STARTING-COPY.md)。继续沿用用户“不构建、不发布”的要求；不打包、不增版本，保留并行 R211～R218 改动。

## P1：恢复登录入口并记录渲染异常

- 初始 state 补回 `installations: []`；`renderLaunchpad` 先处理尚未完成的扫描，再从 `(state.installations || [])` 过滤入口。没有字段的夹具在未加载/已加载空列表两种情况下均可渲染。
- 未连接时安装扫描在 `renderStatus` 前启动，沿用原有 Promise 复用、扫描 TTL、失败/空列表与手动重新检查流程；扫描自身的各次入口渲染也单独捕获异常，防止“把扫描移到前面，但又被扫描内部的首次渲染阻断”。即使入口渲染抛错，`/api/client-installations` 仍发送并完成。
- `refreshStatus` 成功收到状态后发生异常时，新增 `status_render_failed/failed`。只发送固定 errorType 和 functionName；从栈中匹配有限的函数名，栈、消息、路径和自定义错误名称均不发送。runtime/Go 同步固定词表，未知错误类型降为 Error，未知函数名降为 other；保留原请求取消、旧 token 和状态接口失败的处理。

## P2：按真实进程检测结果处理启动遮罩

`/api/status.clientDiscovery` 在原 `a.mu.RLock` 内读取 discovery.Result，只输出有限结果枚举；searching/未知值/未检测归空，已连接强制为 connected。不带 discovery.Detail 或路径。不修改连接管理器的发现/重试流程；connecting 重试期间保留上一次发现结果。

- 未连接且 process-not-found/process-query-failed：立即关闭遮罩，取消 15 秒定时器、恢复正常状态轮询，清除 suppression 以允许稍后检测到客户端时重新显示；不因短暂 connectionState=connecting 再次显示。
- 未连接且结果为空：文案“正在检测英雄联盟客户端。”。
- 未连接且 credentials-unreadable/probe-failed：显示原“检测到客户端正在启动，请稍候。”及 900ms 轮询。
- 已连接身份未就绪保持原身份读取提示，身份就绪立即关闭；15 秒兜底仍保留。
- hide 增加固定 hide_reason：identity-ready/no-client-process/timeout/suppressed，runtime 与 Go 同步过滤。界面只修正工单指定的状态文案，没有新增说明。

## 自动验证

- 前端定向 R219/official-login/R203 **23/23 通过**，见 [node-targeted-final.log](history/reports/r219/node-targeted-final.log)。覆盖缺 installations 的真实 renderStatus、两个入口、失败/空列表重查、无进程及短暂 connecting、晚启动客户端、身份读取、timeout/suppressed、入口渲染异常仍发送扫描请求、诊断匿名性。
- Go R219/R203 诊断及全事件登记定向通过（0.517 秒），见 [go-targeted-final.log](history/reports/r219/go-targeted-final.log)。覆盖全部发现枚举、connected 覆盖、Detail 不出现在状态响应、渲染错误词表、未知敏感字段值丢弃、hide_reason 与时长限幅。
- 两组独立只读复核未发现确定缺陷；前端复核运行 R219/official-login 15/15，后端分别运行 R219/status 定向检查通过。
- 两项临时源码变异均触发行为断言 FAIL：扫描移回 renderStatus 后，异常路径请求数从 1 变成 0；忽略无进程结果，遮罩未隐藏。见 [mutation-scan-after-render.log](history/reports/r219/mutation-scan-after-render.log)、[mutation-no-process.log](history/reports/r219/mutation-no-process.log)。正式源文件未写入变异。
- 第一次前端全量 929 项、928 通过、1 失败（78.145 秒），唯一失败为旧 R206 图片队列在并发调度下超过 100ms（该用例 142ms），见 [node-full.log](history/reports/r219/node-full.log)。图片队列文件及测试未修改；单独复核 4/4 通过，见 [node-image-recheck.log](history/reports/r219/node-image-recheck.log)。保留原 100ms 断言，使用 `node --test --test-concurrency=2 backend/web/*.test.cjs` 复跑相同全量集合，最终 **929/929 通过、0 失败、0 跳过**（39.550 秒），见 [node-final.log](history/reports/r219/node-final.log)。
- 首次 Go 定向编译遇到并行 R216 测试短暂未使用的 strings 导入；该文件由并行工作自行修正，本任务未修改，原日志见 [go-targeted.log](history/reports/r219/go-targeted.log)。之后定向通过；最终 `go test ./backend -count=1` **全量通过（272.758 秒）**，见 [go-full.log](history/reports/r219/go-full.log)，`go vet ./backend` 通过见 [go-vet.log](history/reports/r219/go-vet.log)。最终 `git diff --check` 通过；验证汇总与源码摘要见 [verification.json](history/reports/r219/verification.json)。

## 验收边界

未构建、未发布，版本仍为 0.12.71。构建后由用户确认：关闭客户端启动软件，遮罩不误报正在启动且快速关闭、入口出现；软件开着时启动客户端，接口就绪前正常提示、大厅连接后收起入口；客户端已开时身份读取保持原行为。自动 DOM/HTTP 测试不替代这些 Windows 真机日志与时间验收。
