# WORKLIST-R143：Windows 打包改到 GitHub Actions，不再在远程机器上逐轮试

诊断人：Claude（读完 R142 全部 Windows 打包对话和 2026-09-23 最新日志，只读诊断，未改仓库代码）。
执行人：GPT。
日期：2026-09-23。
基线：HEAD `087ceeec`（"Record CRLF checkout regression verification"），版本 0.12.19。
状态：已完成。R142 §P2「在真实 Windows 上构建」已由本单的 Release 工作流完成；R142 其余内容不变。验收记录见 `docs/history/ledgers/r143-execution-ledger.md`。

## 0. 结论

不要再让用户在那台远程 Windows 上逐轮试了。打包改在 GitHub Actions 的 `windows-latest` 上跑：网络正常、自带 pwsh、有管理员权限、检出就是 LF。GPT 能自己看日志、自己迭代，用户不用再传 zip。

用户最后只需要做两件事：在 Actions 页面点 **Run workflow**，等草稿 Release 出现后自己点 **Publish**。

## 1. 为什么一直打不好（执行前先读，别再用同一种做法）

1. **Windows 链路从来没在 CI 里跑通过。** `ci.yml` 的 `windows-build` 要等 `quality` 通过才会跑，而 `quality` 一直卡在 `installer/internal/webviewhost/abi_test.go`：它用 `go list -m -f {{.Dir}}` 找 go-webview2 源码，缓存是空的时候 `.Dir` 返回空（这次对话的第一个问题就是这个，到现在还没修）。结果几个月里 Windows 专属的问题一直在攒，最后全部在用户那台机器上一个个冒出来。
2. **打包脚本把全量测试当成打包前提，遇到第一个错就停。** Go 测试约 170 秒，桌面测试约 273 秒，跑一轮 8–10 分钟只能暴露一个问题，每一轮都要用户通过 UU 传包、重跑、贴日志。
3. **在 macOS 上"模拟 Windows"验证，模拟不全。** CRLF 能模拟，但路径语义、环境变量、缺少 pwsh 都模拟不出来，所以每次说"已验证"，到了真机上又挂。
4. **失败几乎都不是打包本身的问题**：PS 5.1 编码、CRLF、symlink 权限、时钟精度、proxy.golang.org 和 git clone 的网络、没装 pwsh、测试里写死 POSIX 路径、脚本把 KEY_MODE 泄漏给测试。**真正的打包步骤（electron-builder 从 GitHub 下载 Electron、NSIS、winCodeSign 工具链）到现在一次都没跑到**，那台机器访问 GitHub 不稳定，继续在上面跑，后面大概率还会卡住。

最新日志（2026-09-23，`r142-windows-build-fix-b2c3e33d`）：Go 根模块、installer 模块、前端 692 项全部通过；桌面测试 262 项里有 **11 项失败**，全是测试问题，见 P2。

## P1　修好 CI 的 quality 任务

- `abi_test.go` 改用 `go mod download -json github.com/jchv/go-webview2` 来取 `Dir`（需要时会自动下载，并按 go.sum 校验），不要再依赖缓存里正好已经有这个模块。测试本身要能独立跑通，不要靠在 CI 里多加一步来掩盖。
- 验收：`GOMODCACHE` 为空时，`cd installer && go test ./...` 能通过；`main` 上的 `quality` 变绿，`windows-build` 真正开始执行。

## P2　修掉 11 项 Windows 桌面测试失败（都是测试或脚本的问题，不改产品行为）

| 测试 | 原因 | 修法 |
|---|---|---|
| `installer-shell.test.cjs` 4 项 | `build-desktop-windows.ps1` 第 69 行在测试**开始之前**就设了 `DEEP_LEGENDS_KEY_MODE=public`，`build-shell.cjs` 按环境变量去找 `…-public.exe`，而测试夹具写的是没有后缀的文件名 | ① `buildShell` 增加显式的 `keyMode` 参数，只在 CLI 入口读环境变量；测试显式传入，再补一条 public 用例 ② 构建脚本把设置 `DEEP_LEGENDS_KEY_MODE` 挪到所有测试跑完之后 |
| `release-quality-gates.test.cjs` 的 R86 Windows 用例 | 用例调用 `spawnSync("pwsh")`，但用户机器上只有 Windows PowerShell 5.1；启动失败后 `stdout+stderr` 是 `undefined+undefined`，得到 NaN | 改用 `powershell.exe`（和文档里给用户的入口一致，还能顺带守住 5.1 兼容）；启动出错时直接报出明确的错误，不要变成 NaN |
| `share-export.test.cjs` 5 项 | 假文件系统用 `"/tmp"`、`"/user-data"` 原样的字符串建目录，查找时却用 `path.resolve`，在 Windows 上会变成 `D:\tmp`，对不上 | 夹具里所有路径统一经过 `path.resolve` 生成，或者改用 `os.tmpdir()` 下的真实路径 |
| `ui-scale.test.cjs` 的 IPC 用例 | 假文件 Map 的键是 `"/user-data/ui-scale.json"`，生产代码用 `path.join` 得到的是 `\user-data\ui-scale.json` | 键改为用 `path.join("/user-data", "ui-scale.json")` 生成 |

- 判据是 GitHub Actions 的 Windows 机器上桌面测试全绿，不是 macOS 上的模拟。
- 不许用 `skip`、按平台跳过或放宽断言来让测试通过。

## P3　新增 `.github/workflows/release.yml`

- 触发方式：`workflow_dispatch`（用户在 Actions 页面一键运行），以及推送 `v*` 标签。
- `runs-on: windows-latest`，只给这一个 job 开 `permissions: contents: write`。
- 构建命令用 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\build-public-release-windows.ps1`，和本地是同一个入口，也保证是用 5.1 跑的。`GITHUB_ACTIONS=true` 时跳过脚本里的 goproxy.cn 设置；脚本的管理员检查在 Actions 上应该能通过，如果过不了，就按 `GITHUB_ACTIONS` 判断放行，不要把本地的检查删掉。
- 校验版本：用标签触发时，标签必须等于 `v` 加上 `desktop/package.json` 里的版本号。
- 产物：先把 `dist/release/` 里的三个文件上传为 artifact，再用 `gh release create v<version> --draft --target ${{ github.sha }}`（`GH_TOKEN: ${{ github.token }}`）建一个**草稿** Release，附上这三个文件，正文取 `latest.json` 里的 `notes`。同名草稿已经存在，就先删掉旧草稿再建；同名的**已发布** Release 存在，就直接失败，不要覆盖。
- 不生成、不读取任何 Riot Key；`-KeyMode public` 和 `verifyPublicReleaseBuild` 的检查一项都不能少。

## P4　跑通并交付

1. 本机要有已经登录的 `gh`（用 `gh auth status` 确认）。没有的话就停下来，请用户在 Mac 上一次性执行 `brew install gh && gh auth login`。**不要再回到 zip + UU 的方式。**
2. 推送 P1–P3 的改动后，用 `gh run watch` / `gh run view --log-failed` 自己看日志、自己迭代，直到 `main` 上的 `quality` 和 `windows-build` 都是绿的。
3. 用 `gh workflow run release.yml` 跑一次，确认生成了草稿 Release `v0.12.19`，附件正好三个：`Deep-Legends-Setup-0.12.19-public.exe`、`latest.json`、`SHA256SUMS-public.txt`，并且哈希与 `SHA256SUMS-public.txt` 一致。
4. **不要点 Publish。** 草稿留给用户检查后自己发布。R142 的边界不变，变的只是"建草稿"这一步从用户手动改成了由工作流自动完成。

## P5　收尾

- README「发布流程」改成三步：确认 CHANGELOG → 在 Actions 运行 Windows Release → 检查草稿后点 Publish。本地 Windows 脚本作为备用方式，写一行说明即可。
- 删除 `tools/windows-installer-go-cache.zip` 以及脚本里展开它的代码（这是一个 2.2 MB 的二进制文件，已经进了仓库，而 GOPROXY 已经能兜底）；清理 `dist/` 下的 `r142-windows-*` 补丁包。
- 账本写到 `docs/r143-execution-ledger.md`，记录：每次 Actions 运行的链接和结论、11 项测试修改前后的写法、草稿 Release 的链接。
- 如果仓库是私有的，Windows 的运行分钟数按 2 倍计费。账本里写一句 `windows-build` 单次耗时多久，方便用户判断是否只在 main 和标签上跑。

## 不要做的事

- 不要再让用户在远程 Windows 上跑任何构建命令，也不要再给用户传补丁包。
- 不要在 macOS 上模拟 Windows 之后就宣称通过；判据只认 GitHub Actions 上 Windows 的运行结果。
- 不要跳过、`skip` 或放宽任何测试来让流水线变绿。
- 不要发布 Release，只建草稿；不要删除或修改任何已经发布的 Release。

## 验收总表

| 项 | 判据 |
|---|---|
| quality | 空缓存下 installer 测试通过；`main` 上 quality 为绿 |
| Windows 测试 | Actions windows-latest 上桌面测试全绿，11 项逐条修改，没有 skip |
| 环境泄漏 | 测试阶段不再存在 `DEEP_LEGENDS_KEY_MODE`；`buildShell` 显式传入 keyMode |
| Release 工作流 | 手动触发成功一次；草稿 v0.12.19 附三个文件，哈希一致 |
| 边界 | 草稿未发布；没有 Key 泄漏；没有跳过任何测试 |
| 文档 | README 发布流程改为三步；账本附 run 链接 |
