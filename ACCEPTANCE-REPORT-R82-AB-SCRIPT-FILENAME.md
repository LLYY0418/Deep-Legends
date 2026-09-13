# R82 A/B 采集脚本文件名补修

对应：`WORKLIST-R82-ADDENDUM-2-AB-SCRIPT-FILENAME.md`。日期：2026-09-12。

## 修改范围

`scripts/r82-startup-ab.ps1` 不再要求安装包文件名包含指纹：

1. 从安装包同目录读取 `release-build.json`。
2. 使用 `Get-FileHash -LiteralPath ... -Algorithm SHA256` 计算安装包哈希，遍历 `assets` 的 value 匹配，不依赖 key 中的文件名。
3. 匹配成功后取顶层 `fingerprint` 并转成小写，供原有启动前去重和真实报告脚本使用。
4. 缺少记录时明确提示“把整个 dist/desktop 目录一起带过来”；记录无法读取、指纹/资产结构无效、哈希不符时均在启动安装器前终止。

`r82-startup-ab-report.cjs` 已完整检查：只校验指纹字符串格式、关联诊断日志和统计结果，没有安装包文件名假设，保持原样。去重逻辑、参数传递、预热设置和采集统计逻辑也保持原样。

含中文提示的 PowerShell 文件保存为 UTF-8 BOM，构建记录按 UTF-8 显式读取。旧操作说明 `ACCEPTANCE-REPORT-R82-STARTUP-LATENCY.md` 中的示例同步改为版本号文件名，并补充携带完整构建目录的要求。

本轮没有改构建产物命名、安装页面或接力逻辑，没有打包、发布或改变预热默认开关。

## 自动验证方法

新增 `scripts/r82-startup-ab-script.test.ps1`：执行真实采集脚本的临时副本及真实 Node 报告脚本，只在副本中去掉 Windows 平台限制、在测试进程内替换 `Get-Process`/`Start-Process`。安装包、构建记录、诊断日志和结果都使用临时夹具，安装进程返回值由测试模拟，没有启动真实 exe。

八项夹具检查包括：版本号文件名正确保存记录中的指纹、任意改名仍按哈希匹配、实际采集一次后改名并换组仍启动前拒绝、内容不匹配、缺失记录、损坏 JSON、无效指纹、无效 assets。还检查失败时不写结果、重复测量不修改已有结果、预热环境变量恢复。

新增 Node 测试入口 `scripts/r82-startup-ab-script.test.cjs` 调用 PowerShell，并对临时副本执行工单指定的三项变异：按文件名匹配、忽略哈希不符、跳过去重。每项必须触发指定夹具断言，语法错误、运行时缺失或超时不计作成功拦截。

可执行命令：

```powershell
node --test scripts/r82-startup-ab-script.test.cjs scripts/r82-startup-ab-report.test.cjs desktop/release-build.test.cjs
```

Windows 默认使用 `powershell.exe`，其他系统默认使用 `pwsh`；可通过 `R82_POWERSHELL` 指定路径。没有运行时且未显式指定时，新测试会明确标记 skip，不能把 skip 当作验收通过。

## 实际执行结果

本机为 macOS darwin/arm64。在临时目录解压官方 PowerShell 7.6.6，下载完成后先校验发行记录提供的 SHA-256；没有安装到系统目录。执行上述测试时显式设置 `R82_POWERSHELL` 指向该运行时。

| 检查 | 结果 |
|---|---|
| 真实 PowerShell 脚本的八项夹具检查 | 8/8 PASS |
| 原有报告采集/统计测试 | 6/6 PASS |
| 原有构建记录测试 | 2/2 PASS |
| Node 测试入口总计（含脚本夹具和变异两个入口） | **10 PASS / 0 FAIL / 0 SKIP** |
| 变异：改为按文件名匹配 | 被“改名内容必须匹配”断言拦截 |
| 变异：哈希不符也继续 | 被“内容不符必须报错”断言拦截 |
| 变异：关闭启动前去重 | 被“重复指纹必须拒绝”断言拦截 |

**3/3 指定变异被拦截。** 每个变异都在临时副本中实际运行 PowerShell，并确认失败来自预期夹具断言。首次执行发现测试 mock 使用 `$script:` 作用域无法从被调用脚本读取夹具状态，已修正为测试进程内的显式共享状态，随后基线及所有变异通过；没有为此修改生产脚本。

[完整测试日志](/Users/ly/personal/personal-work/deep-legends/output/r82-ab-filename/tests.log)

这些结果验证真实哈希、JSON 读取、指纹传递、去重和报告采集；安装进程为模拟对象。没有执行 Windows PowerShell 5.1 或 Windows 安装真机验证，不把合成测试记作实际安装或冷启动测量。脚本使用兼容 5.1 的语法，中文脚本的 BOM 和空白检查通过。

## 使用方式

把本次完整构建的 `dist/desktop` 目录带到 Windows，确保安装包旁边有它对应的 `release-build.json`。在仓库根目录执行：

```powershell
.\scripts\r82-startup-ab.ps1 -Installer 'D:\Download\Deep Legends Setup 0.12.1.exe' -Group control
```

安装包可以改名；同一构建改名或换组仍不能当成新指纹重复测量。本单测试使用的合成日志不属于 R82 的六次 Windows 冷启动实测，不改变原有 A/B 待办状态。
