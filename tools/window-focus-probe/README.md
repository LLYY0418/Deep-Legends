# 独立窗口防弹出实验 v1.0.1（源码维护版）

这是一个**只操作自己创建的窗口**的 Windows x64 测试程序，不是 LOL 修复程序。
没有游戏进程查找、LCU 连接、网络通信、注入、注册表写入或客户端文件读写功能。

## 已收到的实测与当前状态

2026-09-10 的 v1.0.0 Windows 报告已确认：外部遮蔽返回 `0x80070005`，未生效；自身遮蔽对照仍获得前台并临时置顶。[完整分析](../../docs/window-focus-probe-results-0910.md)

v1.0.1 **只修正模拟窗口重置、关闭和中止时先解除遮蔽再隐藏的顺序**，改为先隐藏再解除遮蔽，避免测试工具自身产生额外可显示区间。基线和恢复阶段仍故意显示。此改动没有解决外部权限拒绝，也没有实现 LOL 防弹出。

不需要重测已失败的同一候选，不需要为此排真实对局。未发布新的用户测试包；原 v1.0.0 ZIP 保留不动。

## 使用

1. 把发布 ZIP 完整解压到 Windows 的一个可写文件夹。
2. 普通双击 `DeepLegends-WindowProbe.exe`，不需要管理员权限，也不需要启动 LOL。
3. 点击“开始测试”，约 10 秒内暂时不操作鼠标、键盘、切换桌面或窗口。
4. **基线和解除恢复阶段的模拟窗口会故意弹出，这是正常对照。** 其他阶段是否弹出就是被测结果；不一定能成功抑制。
5. 完成后点击“打开报告文件夹”，发送里面的 **`WindowProbe-Report.zip`**。如亲眼看见闪烁，也请一并说明。

可用 Esc、关闭任一测试窗口、或“停止并恢复”中止。需要重跑时退出后重新双击。
请勿在正在输入密码、全屏游戏、远程操作的重要步骤中运行：基线对照会故意改变焦点。
程序未签名；如果被安全软件拦截，请保留提示反馈，不需要关闭安全软件。

报告优先写到 EXE 旁的 `WindowProbe-日期-时间-随机后缀` 文件夹，目录不可写时使用系统临时目录，界面显示实际路径。
每次使用独立目录，不覆盖已有报告。报告 ZIP 只包含 `report.json`、`trace.jsonl`、`结果说明.txt`。
若在资源管理器里直接从压缩包打开 EXE，输出位置可能处在临时解压目录，因此请先完整解压。

## 实验设计

控制器是用户当前前台锚点；它通过匿名 stdin/stdout 管道启动同一个 EXE 的内部子进程。
子进程创建一个独立的顶层模拟窗口；这不是同进程两个窗口的替代测试。
控制器保留子进程内核句柄并校验窗口 PID，不接受外部 PID/HWND 参数。

四个阶段：

| 阶段 | 用途 |
| --- | --- |
| `baseline` | 不遮蔽，模拟显示/激活/置顶；必须实际观察到可显示与模拟进程获得焦点，才是有效对照 |
| `external_cloak` | 控制器跨进程设置 `DWMWA_CLOAK`，读回 `DWMWA_CLOAKED` 的 APP 位，再重复三次显示/激活路径 |
| `self_cloak` | 子进程在自己内部设置相同属性，仅用于区分“机制可用”和“跨进程权限受限”；绝不是悄悄启用的替代实现 |
| `release_show` | 子进程解除遮蔽并重新显示，模拟真正进入选人阶段 |

外部设置失败、读取失败或 APP 位未生效时，明确记录 HRESULT/跳过原因，不把“原本隐藏的窗口没出来”算成功。
不尝试隐藏第三方窗口，不在弹出后反复最小化来冒充提前阻止。

模拟子进程在 **自身窗口**上执行从上传样本验证出的原生序列：

```text
ShowWindow(SW_SHOWNOACTIVATE)
ShowWindow(SW_SHOW)
SetActiveWindow
SetForegroundWindow
SetWindowPos(HWND_TOPMOST, SWP_NOSIZE | SWP_NOMOVE)
SetWindowPos(HWND_NOTOPMOST, SWP_NOSIZE | SWP_NOMOVE)
```

每次 API 后立即记录快照，控制器另以 20ms 间隔采样。原始返回值、含义、DWM HRESULT、可见样式、最小化、遮蔽位、临时置顶、相对锚点层级、准确锚点是否为前台、最后输入时间戳都留在报告中。
每轮明确尝试允许**自己的模拟子进程**获得前台，以避免系统恰好挡住焦点请求造成弱对照。
最后输入时间戳只用于判定干扰，不采集按键、鼠标坐标或输入内容。

## 边界与退出

- 外部程序只被分类为 `other`，不保存其他窗口标题、进程名称、画面、命令行或账号。
- `ShowWindow` 返回的是之前的可见状态，不是成功 BOOL；`SetActiveWindow` 的旧句柄不作为成功判断。
- 自身遮蔽成功不能证明外部遮蔽可用；隐藏画面也不保证焦点和 Z-order 不变。
- 即时 API 读回会改变时序；20ms 采样不是逐显示帧录像。结论至多为 `candidate_only`，不保证零闪烁，更不能等同真实 LOL 验证通过。
- 正常结束、中止时请求子进程解除遮蔽并销毁窗口。管道断开也触发子进程退出；控制器有仅针对自己子进程句柄的强制终止兜底。
- 测试阶段限时 30 秒；子进程另有 60 秒独立看门狗（随后 3 秒退出兜底），父进程异常退出时也不会留下永久隐藏的测试窗口。

## 构建和验证

独立 Go 模块，仅标准库，不更改主项目依赖。Go 1.24+：

```sh
cd /Users/ly/personal/personal-work/deep-legends/tools/window-focus-probe
go test -race -cover ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath \
  -ldflags='-H=windowsgui -s -w' -o DeepLegends-WindowProbe.exe .
```

本次在 macOS 完成的是跨平台判定/报告单元测试、race 检查及 Windows x64 交叉编译/静态检查。
本地没有 Windows 实机。v1.0.0 的用户实测结果见上；v1.0.1 的顺序回归测试仅验证源码调用顺序，**没有执行修改后的 Win32 GUI 或逐帧显示验证**。Windows 专用 ABI 测试编入测试二进制，但不能冒充在 macOS 已执行。

参考：
- [DWM 遮蔽属性](https://learn.microsoft.com/en-us/windows/win32/api/dwmapi/ne-dwmapi-dwmwindowattribute)
- [ShowWindow 返回值及显示参数](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-showwindow)
- [SetWindowPos 层级及 flags](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setwindowpos)
- [SetForegroundWindow 及限制](https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-setforegroundwindow)

无需把上传的客户端二进制复制到本工具目录，也不要替换任何客户端 EXE/DLL/WAD。
