# R143 执行账本

版本：0.12.19。执行基线：`087ceeec`。状态：代码已实现；GitHub Actions 真正跑通及草稿 Release 验收待执行。

## P1：quality 的 ABI 测试

`installer/internal/webviewhost/abi_test.go` 原先用 `go list -m -f {{.Dir}}`，空模块缓存时目录为空。改为 `go mod download -json github.com/jchv/go-webview2`，从 JSON 的 `Dir` 读取依赖源码，并保留原有 COM 槽位断言。错误同时显示模块下载的 JSON Error 和 stderr。

本机用全新 `GOMODCACHE`、默认官方 GOPROXY 运行 `cd installer && go test ./...`：通过。GitHub Actions [`main/quality`](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107267397722) 已通过，包含原先失败的 `Test installer module`；任务耗时 10 分 48 秒。

## P2：11 项 Windows 桌面测试

| 项数 | 位置 | 修改前 | 修改后 |
|---:|---|---|---|
| 4 | `desktop/installer-shell.test.cjs` / `installer/build-shell.cjs` | `buildShell` 在函数内读进程的 `DEEP_LEGENDS_KEY_MODE`；public 构建测试期间寻找带 `-public` 的夹具文件 | `buildShell` 显式接收 `keyMode`，仅 CLI 入口读环境变量；4 个夹具显式传 `private`，新增 public 夹具验证带后缀的包 |
| 1 | `desktop/release-quality-gates.test.cjs` | Windows 门禁用 `pwsh`，启动失败时拼接 `undefined` 的输出 | 用 Windows PowerShell 5.1 的 `powershell.exe`，启动错误单独报告；门禁夹具补上新行尾处理脚本和 npm shim，继续验证真实 Go 测试失败时阻止构建 |
| 5 | `desktop/share-export.test.cjs` | 假文件系统目录用 `/tmp`、`/user-data` 等原始 POSIX 字符串，查找时使用 `path.resolve` | 目录、保存位置和断言统一由 `path.resolve` / `path.join` 生成；不改生产导出逻辑 |
| 1 | `desktop/ui-scale.test.cjs` | 假文件 Map 键写死 `/user-data/ui-scale.json` | 键与生产代码都使用 `path.join` |

`build-desktop-windows.ps1` 在所有 Go、前端和桌面测试前清除继承的 `DEEP_LEGENDS_KEY_MODE`，桌面测试完成后才设置本次 key mode，用于真正构建、打包和凭据核验。未新增平台跳过或放宽断言。

本机定向 Node 测试：48 项，47 通过，1 项原有 Windows 专属测试在 macOS 跳过。[GitHub Actions Windows job](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107271782211) 在 `Build desktop client and checksums` 步骤失败；公开页面只显示 exit code 1，完整日志要求 GitHub 登录。因此这 11 项在 Windows 上是否全部通过，当前尚无证据，不能记为验收通过。

## P3：Release 工作流

`.github/workflows/release.yml` 支持手动触发和 `v*` 标签推送，仅 release job 有 `contents: write`。在 `windows-latest` 用 `powershell.exe` 执行同一个 public 构建入口，校验标签版本，先上传恰好三个 public 文件为 artifact，再用 `latest.json.notes` 建草稿 Release。已有草稿先删后建；已发布同名 Release 拒绝覆盖。草稿链接、附件名称和哈希验收：待填。

## P5：收尾

README 发布流程改为确认 CHANGELOG → Actions 运行 Windows public release draft → 检查草稿后 Publish。本地 Windows 脚本保留为备用。删除仓库内 `tools/windows-installer-go-cache.zip` 及展开代码；Actions 不设置 goproxy.cn；删除 `dist/` 中两个 `r142-windows-*` 补丁包。

仓库为公开仓库，工单提及的私有仓库 Windows 分钟数 2 倍计费不适用。本次 `windows-build` 运行 12 分 37 秒后失败。成功单次耗时待验收时填写。

## Actions 运行记录

| 运行 | 链接 | 结果 | 耗时 |
|---|---|---|---|
| main / quality | [35886360802 / quality](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107267397722) | 通过 | 10 分 48 秒 |
| main / windows-build | [35886360802 / windows-build](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107271782211) | 失败，需登录读取 Build desktop client and checksums 日志 | 12 分 37 秒 |
| 手动 release.yml | 待填 | 待运行 | 待填 |
