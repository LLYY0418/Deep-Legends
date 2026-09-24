# R143 执行账本

版本：0.12.19。执行基线：`087ceeec`。状态：已完成；`main` 的 quality 与 windows-build 已通过，public 草稿 Release 已创建并验收，尚未发布。

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

本机定向 Node 测试：48 项，47 通过，1 项原有 Windows 专属测试在 macOS 跳过。[GitHub Actions Windows job](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107271782211) 完整日志确认：前端 692/692 通过，Windows 桌面测试 263 项中 261 通过、0 失败、2 项原有跳过；工单列出的 11 项均未再失败。

同一次运行在测试之后到达真正的 electron-builder NSIS 打包，随后报 `File: ".../desktop/nsis/../assets/hexcore-icon.ico" -> no files found`。仓库中该图标存在，electron-builder 自身也已使用它；问题是自定义 NSIS 脚本通过 `${__FILEDIR__}/../` 拼出的路径。改为编译器已传入的绝对 `${MUI_ICON}`；同文件卸载外壳路径改用 `${PROJECT_DIR}`。`desktop/installer-nsh.test.cjs` 断言这两个输入变量；本机相关 37 项测试全通过。[修复后的 Windows job](https://github.com/LLYY0418/Deep-Legends/actions/runs/35943171324/job/107457601534) 在真实 `windows-latest` 上完整构建、SHA-256 校验和 artifact 上传均通过，耗时 13 分 39 秒。

## P3：Release 工作流

`.github/workflows/release.yml` 支持手动触发和 `v*` 标签推送，仅 release job 有 `contents: write`。在 `windows-latest` 用 `powershell.exe` 执行同一个 public 构建入口，校验标签版本，先上传恰好三个 public 文件为 artifact，再用 `latest.json.notes` 建草稿 Release。已有草稿先删后建；已发布同名 Release 拒绝覆盖。

[手动 Release 工作流](https://github.com/LLYY0418/Deep-Legends/actions/runs/35944934254) 在提交 `c8de1343` 上成功，release job 耗时 13 分 56 秒。[v0.12.19 草稿 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/untagged-bafa096a0347f5a09fb3) 的 `isDraft=true`，正文与 `latest.json.notes` 一致，只有以下三个附件：

| 附件 | 字节 | 下载后 SHA-256 |
|---|---:|---|
| `Deep-Legends-Setup-0.12.19-public.exe` | 101961728 | `f90174abe3f8ec58fe09a5787b74c77a5c9987e4b40969844e2639101041224d` |
| `latest.json` | 2431 | `6488bbdb2c2afe741fc06dbf3c840aaba28dce4538fb56c50f831684377a174c` |
| `SHA256SUMS-public.txt` | 182 | `5f9bf5e888d60f97143f85e6ba09f5a2636109e1970055175565409578cd99a5` |

下载三个远端附件后，逐文件计算 SHA-256；EXE 和 `latest.json` 均与 `SHA256SUMS-public.txt` 完全一致，GitHub 返回的三个 asset digest 也一致。`latest.json` 中版本为 `0.12.19`，EXE 的名称、大小与 SHA-256 均与实际文件一致。三个已验收附件已放在本机 `dist/release/`，该目录没有其他文件。public 构建和 `verifyPublicReleaseBuild` 均由成功的 Actions 工作流执行；草稿保持未发布。

## P5：收尾

README 发布流程改为确认 CHANGELOG → Actions 运行 Windows public release draft → 检查草稿后 Publish。本地 Windows 脚本保留为备用。删除仓库内 `tools/windows-installer-go-cache.zip` 及展开代码；Actions 不设置 goproxy.cn；删除 `dist/` 中两个 `r142-windows-*` 补丁包。

仓库为公开仓库，工单提及的私有仓库 Windows 分钟数 2 倍计费不适用。失败的 `windows-build` 运行 12 分 37 秒；成功的 `windows-build` 单次耗时 13 分 39 秒。

## Actions 运行记录

| 运行 | 链接 | 结果 | 耗时 |
|---|---|---|---|
| main / quality | [35886360802 / quality](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107267397722) | 通过 | 10 分 48 秒 |
| main / windows-build | [35886360802 / windows-build](https://github.com/LLYY0418/Deep-Legends/actions/runs/35886360802/job/107271782211) | 测试全通过，NSIS 自定义图标相对路径失败 | 12 分 37 秒 |
| main / quality（修复后） | [35943171324 / quality](https://github.com/LLYY0418/Deep-Legends/actions/runs/35943171324/job/107455338296) | 通过 | 10 分 07 秒 |
| main / windows-build（修复后） | [35943171324 / windows-build](https://github.com/LLYY0418/Deep-Legends/actions/runs/35943171324/job/107457601534) | 通过，包含 SHA-256 与 artifact 上传 | 13 分 39 秒 |
| 手动 release.yml | [35944934254](https://github.com/LLYY0418/Deep-Legends/actions/runs/35944934254) | 通过，生成并验收未发布草稿 | 13 分 56 秒 |
