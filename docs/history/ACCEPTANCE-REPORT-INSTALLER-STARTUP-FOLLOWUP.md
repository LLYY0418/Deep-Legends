# 安装器启动问题二次修复

日期：2026-09-11。按用户要求，仅修改源码和执行检查，未打包、未上传、未修改版本号。原有 `dist` 文件仍是此前构建，不能用于验证本次修复。

## 20:11 实机日志后的根因更正

用户构建 `5a6a6d81a476` 的日志显示运行时 `152.0.4191.66` 和控制器创建成功，随后报 `configure WebView: get_Settings: WebView COM interface is unavailable`。

**这条错误由本项目 COM 适配层的方法序号写错直接解释。** 先前核验错误地认可了手写序号，下面“已确认的问题”及“验证”章节保留为上一轮历史记录，不能作为其方法序号正确的证据。

| 操作 | 错误序号 | 该序号实际调用 | 正确序号 |
| --- | ---: | --- | ---: |
| 获取 CoreWebView2 | 23 | NotifyParentWindowPositionChanged | 25 |
| 关闭控制器 | 22 | PutParentWindow | 24 |
| 设置背景色 | 25 | GetCoreWebView2 | 27 |

序号为从零开始，包含 IUnknown 的三个方法。依据是固定版本依赖中 Controller / Controller2 的完整 vtable 声明，已使用程序展开字段逐一核对，不再凭手工计数。

错误调用的链路为：控制器创建成功 → 实际调用位置变化通知并得到成功结果 → `core` 输出指针没有被填写，仍为 nil → 下一步 `get_Settings` 在进入 COM 之前即被项目的空指针检查拒绝。该结果与用户日志一致。微软文档对这两个方法的作用有明确区分，参见 [ICoreWebView2Controller](https://learn.microsoft.com/en-us/microsoft-edge/webview2/reference/win32/icorewebview2controller)。背景色调用原本还会把颜色整数错误地当成接口输出指针，必须一起修正。

本轮源码指纹：`2bafb51fb3cc`。修改限定于共享 COM 适配层及其测试：

- 修正三个序号，所有 COM 序号集中到 `installer/internal/webviewhost/abi.go`；安装与卸载共用修复。
- `coreFromController` 统一获取接口，并在成功返回但指针为空时立即给出准确错误；GetSettings 同样检查输出指针。
- 新增跨平台 `abi_test.go`，读取真实依赖的完整接口声明、递归展开 IUnknown，并核对全部 20 个使用中的方法序号。
- 修复前运行新测试，准确报出 Close 22/24、GetCoreWebView2 23/25、PutDefaultBackgroundColor 25/27 三处失败；修复后全部通过。
- Windows COM 假对象改用反射读取真实依赖的字段偏移，不再复制生产代码的数字。原假对象把“获取接口”也写在 23 上，因此未能发现实现错误。
- `go test -race -count=1 ./...`、Windows amd64 `go vet ./...` 通过；安装器、卸载器及共享模块的 Windows 测试程序交叉编译通过。

本轮没有修改页面样式、业务流程或发布版本，没有打包。未执行 Windows 原生安装器，因此以上结论是明确的代码根因和接口布局验证，不代表完整 Windows 安装流程已经实机验收。

## 已确认的问题

1. 空日志能够通过测试复现。此前 `StartupLog` 使用 `io.MultiWriter(previous, file)`，先向控制台写入；无控制台的 GUI 进程写入失败后，MultiWriter 不再写文件。新增测试用不可用控制台 writer 模拟该条件，修复前日志内容确实为空，测试失败。修复后直接写入文件并追加历史记录、标注 PID，该测试通过。
2. WebView2 依赖的 `GetSettings` 和各个设置方法错误地根据 `GetLastError` 判断 COM 成败，忽略 HRESULT。Windows 残留错误码可能让成功调用被当成失败，项目此前随即回退旧 NSIS 界面。这是已确认的代码缺陷；由于用户的日志为空，尚不能确认它就是该机器本次回退的唯一触发原因。COM 返回值规则见 [微软文档](https://learn.microsoft.com/en-us/windows/win32/com/error-handling-in-com)。
3. 自动回退旧界面掩盖了自定义页面启动失败，违反用户本次要求。安装、卸载入口已取消该路径，改为显示错误及日志路径。

## 改动

- `installer/internal/webviewhost/settings_windows.go` 通过已有 HRESULT 适配器完成 GetCore、GetSettings 和 8 项设置操作；明确启用脚本和 WebMessage，保留原来的界面限制；释放取得的 COM 引用。
- 安装器与卸载器都接入上述实现。背景色使用同样的 HRESULT 调用规则，保持原有颜色。
- 运行时检查改用创建 WebView 时使用的实际 loader，记录运行时版本，删除旧的注册表单独判断。
- 页面启动失败会保留接口名称、HRESULT、脚本错误位置；超时记录导航和页面初始化各自是否完成。
- 错误弹窗前先结束旧窗口并消费 WM_QUIT，避免弹窗被残留退出消息影响。
- 启动日志直接写文件，使用追加模式，不再依赖 GUI 进程的 stderr，也不覆盖上次记录。日志位置仍为 `%TEMP%\DeepLegendsSetup-startup.log` 和 `%TEMP%\DeepLegendsUninstall-startup.log`。
- 原安装页 HTML/CSS、升级图标及箭头未改变。未修改 NSIS 核心安装逻辑。

## 验证

- 安装器 Go 模块：`go test -race -count=1 ./...` 通过，包含安装器、共享模块和卸载器。
- 安装器相关 Node 测试：29/29 通过，涵盖真实页面交互、启动握手、NSIS 配置和构建流程的测试替身。
- 日志回归：覆盖控制台不可用、重复启动保留历史记录、恢复原日志 writer；修复前失败、修复后通过。
- 页面错误诊断测试：确认脚本位置保留、换行被规范化、超时记录缺失的就绪信号。
- Windows amd64 `go vet ./...` 通过。
- 安装器、卸载器、共享模块的 Windows 测试程序交叉编译通过，临时测试文件写入 `/tmp`，未生成发布安装包。
- 新 Windows COM 测试覆盖 GetCore → GetSettings → 8 个 Put 方法：成功 HRESULT 配合非零 last-error，失败 HRESULT 配合零 last-error，错误操作名及引用释放次数。该测试仅完成交叉编译，未在 Windows 执行。
- 两次独立只读核验确认设置槽位、BOOL、颜色参数、引用释放及错误窗口退出顺序；没有发现新的明确缺陷。

## 验证边界

本机不能运行 Windows 原生安装器，自定义页面在原故障机器正常启动仍需用户重新构建后验证。此记录不把编译通过视作真机验收通过。

WebView2 依赖内部同步创建环境／控制器的阶段仍不受页面 20 秒超时约束。启动日志在系统临时目录本身不可写时也可能无法生成；错误弹窗会直接显示本次启动错误，便于继续定位。
