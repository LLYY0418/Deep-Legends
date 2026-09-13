# R82：Setup 单产物构建与压缩耗时定位

日期：2026-09-12。用户要求只构建 Setup，实际打包由用户执行；本轮未运行真实 Go 编译、electron-builder、NSIS、7z 或安装器。

后续调整：按用户优先控制安装包体积的要求，默认压缩级别已从 3 恢复为 **9**；仅构建 Setup 的优化继续保留。下文的级别 3 验证结果保留为当时的历史记录。

## 本次日志的确定结论

输入：用户附件 `7b8be0cc-cc73-48fa-9811-a64aaebf9109/pasted-text.txt`，命令 `DEBUG=electron-builder ./build-desktop.sh`，指纹 `606ed957dc9c`，版本 `0.12.1`。

| 阶段 | 日志实际耗时 | 结论 |
|---|---:|---|
| npm ci | 10 秒 | 有开销，但不是数分钟停顿的来源 |
| 免安装 EXE | 57.5 秒 | 本次直接取消 |
| 卸载外壳 | 0.8 秒 | 保留，NSIS 安装钩子需要它 |
| NSIS 合计 | 220.2 秒 | 主要耗时阶段 |
| 其中应用载荷 7z 压缩 | **203.890 秒** | 占 NSIS 约 92.6% |
| 安装外壳封装 | 1.8 秒 | 保留当前 Go/WebView2 安装页面 |
| 独立 ZIP | 16.5 秒 | 本次直接取消 |

首个 portable 计时点至 ZIP 完成为约 4 分 58 秒，不包含之前的 Go 编译、Key 加密和 npm 安装；不能把它当成完整构建总耗时。

长停顿紧接 `7za ... -mx=9 ... -mf=BCJ`，之后日志明确输出 `nsis package, x64: 203s 890ms`。这次已有直接子阶段计时，补足了上一份诊断仅凭产物时间推测的证据。Electron、NSIS 和 7zip 工具均命中缓存；没有下载重试或 Wine/VM 回退证据。两次 EXE 资源处理各不到 1 秒。npm 的配置/引擎警告未中止构建，不能解释该 204 秒区间。

日志里的独立 ZIP 是 `win-unpacked` 的应用解压版，不是源码包。`win-unpacked` 是生成 Setup 所需的中间目录，也不是源码副本。

## 修改后的流程

`Go 后端 → 卸载外壳 → 一次 NSIS → Go 安装外壳 → 运行时/指纹/凭据校验 → 构建收据与 Setup SHA-256 → 清理 win-unpacked`

- Bash、PowerShell 和默认 Windows target 均只选择 NSIS；移除 portable 构建、额外 ZIP 和旧 `SKIP_SETUP` 分支，不生成源码包。旧免安装版、ZIP 和收据仍在下一次构建开始时按原清理范围移除。
- 取消当前构建的 portable 模板修改和模板前置校验。历史模板文件保留；`beforePack` 的凭据校验继续执行。
- `beforePack` 使用打包器支持的 `ELECTRON_BUILDER_COMPRESSION_LEVEL`，最初默认 3，现按用户要求恢复为 **9**。显式设置 0–9 可覆盖；其他值报错。两个系统入口共用同一个 hook，不修改 `node_modules`。
- 保留 7z 格式、`differentialPackage:false` 和打包器的安装期兼容 `BCJ` 过滤器，不切换解压器，不使用会跳过 `beforePack` 的 `--prepackaged`。单纯设置 `compression:normal` 无法解决问题：当前打包器对普通 7z 仍默认使用 level 9。
- 保留当前安装/卸载外壳、页面、图标、权限与启动逻辑；最终 Setup 仍通过原外壳 PE 校验再进入收据，不能把中间的旧 NSIS 向导当成最终产物。
- `release-build.json` 保持 schema 1，仅记录最终 Setup 哈希。缺失或空 Setup 会失败并清除旧收据；后端一致性和 public/private 凭据检查保留。
- `make-release.cjs` 接受 Setup 单产物，发布目录为 Setup、`latest.json`、`SHA256SUMS.txt` 三个文件；更新器消费的资产名及清单格式保持兼容。A/B 脚本仍根据 Setup 实际哈希读取收据指纹。

直接删除的两个阶段在本次日志中共 **74 秒**，已包括 portable 阶段的重复应用组装，不另行叠加估算。降低 7z 级别预计进一步缩短主要压缩阶段，但可能增大包体积；本轮没有实际打包，**不提供未经实测的新耗时、包大小或 Windows 安装结论**。新包与此前七份 level 9 样本压缩条件不同，后续做安装耗时对比时应注明。

## 验证

- 33 项定向 Node 测试通过，包括真实 Bash 脚本在临时目录中的模拟构建：只调用一次 setup target、正确传递带空格的 Electron 缓存路径、先构建卸载外壳再 NSIS、最后封装安装外壳、按最终字节生成单资产收据和哈希、清理旧免安装包/ZIP/中间目录。
- 模拟构建执行真实 `beforePack` 与已安装打包器的 `compute7zCompressArgs`，确认默认参数为 `-mx=3 -mf=BCJ`，外部 BCJ2 设置不能覆盖安装期兼容过滤器。没有执行真实压缩。
- 模拟 NSIS 失败时保留非零退出码，停止封装，不生成成功收据或哈希，并清理卸载外壳。既有测试继续覆盖外壳编译失败、无效 PE、凭据泄露、过期后端、篡改发布文件等拒绝路径。
- A/B 收据测试改为仅含 Setup 的夹具，使用真实 PowerShell 运行：2 项 Node 测试通过，内部 8 项夹具检查全部通过，3 个故意破坏查找/哈希/去重的变异均被拒绝。合计 **35 PASS / 0 FAIL / 0 SKIP**。
- `bash -n build-desktop.sh`、PowerShell AST 语法解析和 `git diff --check` 通过。测试和语法解析均未运行真实 Windows 安装器。

## 用户下一次构建

继续运行：

```bash
DEBUG=electron-builder ./build-desktop.sh
```

日志应显示 `Setup compression: 7z level 9 (default 9)`，实际压缩命令应有 `-mx=9 -mf=BCJ`，且没有 portable / release-zip 阶段。分发应用只需最终 `Deep Legends Setup 0.12.1.exe`；构建收据与哈希继续保存为 `release-build.json`、`SHA256SUMS.txt`。默认已是级别 9，无需额外设置环境变量。
