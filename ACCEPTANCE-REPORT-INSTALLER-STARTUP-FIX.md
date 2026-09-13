# 安装器透明空框排查与修复记录

日期：2026-09-11。修复版本：`0.12.1`，源码指纹：`3f7274804cac`。

> 后续实机反馈：该构建仍退回旧 NSIS 界面，且启动日志为空，因此本次交付未解决用户的问题。以下保留为历史记录；后续源码修复、验证结果和限制见 `ACCEPTANCE-REPORT-INSTALLER-STARTUP-FOLLOWUP.md`。后续按用户要求未重新打包。

## 现象与判断范围

用户截图中的程序为 `Deep Legends Setup dd5512e9514e.exe`。启动后只出现有边框和阴影的透明窗口，没有安装页面，持续不消失。

检查了安装器、卸载器、页面初始化脚本以及实际使用的 WebView2 Go 封装。发现启动链路存在能够导致空白窗口长期保留的缺陷，已修复并重新打包。本机为 macOS，未在用户的 Windows 环境重现，尚不能断言截图一定由其中某一个分支单独导致。

## 排查结果与修复

1. 原实现收到 `NavigationCompleted` 就显示窗口，没有检查导航是否成功，也没有确认页面脚本已初始化。现在同时等待导航成功和页面 `shell-ready`；脚本报错、导航失败均进入兼容向导。消息处理转到 UI 消息队列，避免在 COM 回调内执行后续窗口操作。
2. 原封装的 `NavigateToString` 没有把 HRESULT 交给调用方。新增小范围 COM 适配层，检查导航、边界和可见性操作的 HRESULT，同时读取导航事件的成功标记与错误状态。
3. 原实现没有完整同步父窗口与 WebView 控制器的可见性。现在在导航前设置有效边界、显式启用控制器可见性，在显示、最小化和恢复时同步状态。微软文档说明控制器不可见时 WebView 会透明，且该属性不改变父窗口本身；这是需要单独管理的状态。参见 [WebView2 Controller 官方文档](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2controller?view=webview2-1.0.3967.48)。
4. 原窗口没有背景画刷，却直接吞掉背景擦除消息，页面没有绘制时会保留透明外框。安装与卸载窗口现在使用与页面一致的深色背景，并交由默认窗口过程擦除背景。
5. 初始化数据改为随 HTML 内联，先于应用脚本执行，消除对异步初始化脚本注册时序的依赖。保留 JSON 的 HTML 转义，并检查 `NavigateToString` 的 2 MiB 限制。当前页面远低于该限制，未发现超大 HTML 导致导航失败的证据。参见 [WebView2 本地内容文档](https://learn.microsoft.com/en-us/microsoft-edge/webview2/concepts/working-with-local-content)。
6. 增加页面启动 20 秒超时、失败后关闭原窗口并进入既有兼容流程，以及关闭后忽略迟到事件的处理。安装器和卸载器均已接入。
7. 增加本地启动诊断日志，记录初始化阶段和错误。此次新增代码不记录初始化数据、账号信息或 API 密钥。

已检查依赖包在启动线程锁定和 COM 初始化方面的现有处理，没有把“缺少 COM 初始化”当成已确认根因。没有修改 NSIS 安装逻辑。升级图标继续保持 30px，箭头保持用户此前确认的放大版本。

## 代码入口

- `installer/internal/webviewhost/startup.go`、`startup.js`：HTML 初始化、页面握手、启动状态和日志。
- `installer/internal/webviewhost/surface_windows.go`：WebView2 控制器和导航 HRESULT 检查。
- `installer/webview_windows.go`、`window_windows.go`、`main_windows.go`：安装器接入及兼容回退。
- `installer/uninstall/` 中对应三个 Windows 文件：卸载器接入及兼容回退。
- `installer/internal/webviewhost/*_test.go`、`desktop/installer-startup.test.cjs`：启动状态、真实页面初始化、失败分支及 Windows COM ABI 检查。
- `desktop/source-fingerprint.cjs`：将新增嵌入式 JavaScript 纳入安装器指纹。

## 已完成验证

| 验证 | 结果 |
| --- | --- |
| 全量 Node 回归 | 530/530 通过，无失败、跳过或取消 |
| 安装器 Go 模块 `go test -race -count=1 ./...` | 安装器、共享启动模块、卸载器通过 |
| 启动状态变异检查 | 6/6 故意破坏被测试捕获，未修改基线通过 |
| 真实安装和卸载 HTML 的启动测试 | 4/4 通过，覆盖成功握手与初始化脚本异常 |
| Windows amd64 `go vet ./...` | 通过 |
| Windows 测试程序交叉编译 | 安装器与 COM 适配层测试程序编译通过；未在 Windows 执行 |
| 完整 public 桌面构建 | 通过；打包运行时代码、源码指纹和 public 密钥策略检查通过 |
| 最终发布文件 | 恰好四个文件，哈希、清单大小、版本、构建凭据和源码指纹一致 |
| 安装包格式 | x64 Windows GUI PE，103,249,920 字节；包含新增启动逻辑 |
| 用户确认的升级图标 | 打包后的后端资源中保留 30px 图标及放大箭头 |

变异检查覆盖：忽略 DOM 就绪、忽略导航成功、跳过控制器激活、吞掉导航失败、吞掉超时、忽略关闭状态。

证据位于 `output/installer-startup-fix/`：`full-node-tests.log`、`installer-tests.log`、`dom-tests.log`、`windows-vet.log`、`mutations.json`、`build.log`、`release-verification.json`。

## 修复产物

文件均位于 `dist/release/`，本次未上传到 GitHub。`latest.json` 中的发布地址是发布清单配置，不代表远端文件已可下载。

| 文件 | 字节数 | SHA-256 |
| --- | ---: | --- |
| `Deep-Legends-Setup-0.12.1-3f7274804cac.exe` | 103249920 | `d1ba52d86b976bb9bed55dd818df45bccf681bdd20c6cde9b1edc4ca05e3b3e8` |
| `Deep-Legends-0.12.1-3f7274804cac.exe` | 141280683 | `77558584dd579f08a7f1349056818438612289120088cbf4049d8092c85dcbef` |
| `latest.json` | 787 | `585e360032b15ac83d773edfa3f8ffd38ceba22f427788023aefe54b1a7bdc82` |
| `SHA256SUMS.txt` | — | 记录上述三个文件的校验值 |

## Windows 复验与剩余边界

先在任务管理器结束旧的 `Deep Legends Setup` 进程，再启动新的 `Deep-Legends-Setup-0.12.1-3f7274804cac.exe`。安装器采用单实例互斥锁，旧进程还在时，新进程可能只会把旧透明窗口带到前台。

在原故障机器上确认安装页正常出现，最小化、恢复后仍正常，并完成一次安装及卸载。若再次卡住，请保留故障发生后的日志：

- 安装器：`%TEMP%\DeepLegendsSetup-startup.log`
- 卸载器：`%TEMP%\DeepLegendsUninstall-startup.log`

日志每次启动会重写，请在再次启动前复制留存。

20 秒超时覆盖 WebView 控制器创建成功后的页面加载和初始化阶段。依赖库内部的同步环境／控制器创建阶段尚未加入独立超时；如果日志停在该阶段，需要据此继续定位。控制器可见性检查也不等于实际像素渲染验证，机器上的 WebView2 运行时及图形环境问题仍需 Windows 实测确认。

本报告确认代码修复、自动化检查及构建产物完成，不把交叉编译或 DOM 测试视为 Windows 原生安装验收通过。
