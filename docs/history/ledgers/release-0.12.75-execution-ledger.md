# 0.12.75 发布执行账本

日期：2026-10-06（北京时间）。用户授权“发布新版本”，并要求按最新 R230 已完成内容更新；包含去除构建切换头像数字、二级表格箭头贴近头像两项后续调整。key mode **public**。

## 范围与源码

R230 与此前已完成的 R229 共用源码一起验证、发布；R230 是本次发布说明重点。版本基线 0.12.74；本地预检全部通过前不更新版本、不推 tag。旧 R222 原始日志和四个旧验证目录不纳入提交。未改 Worker，不重新部署、不读取或更换 Secret；15 平台公开状态均 200 且 JSON 合法。

本地分支 `codex/release-0.12.75`；**只推 tag，不同时推同一提交到发布分支**。发布前完整 Go 全量/race/vet、installer、格式/JS syntax、CI filters、renderer、Worker、真实 Chromium 门槛见 [本地证据](../reports/release-0.12.75/)。

## 预检与发布状态

本地预检全部通过：2026-10-06 12:19:10。开始时刻 2026-10-06 12:11:09；预检期间版本仍 0.12.74，源指纹 59cb5070cc47。详细起止/耗时/退出码/最后五行见 preflight-summary.json。

| 检查 | 耗时/结果 |
|---|---|
| go-full | 116.378s，退出 0 |
| go-race | 171.793s，退出 0 |
| go-vet | 1.364s，退出 0 |
| installer-test | 3.697s，退出 0 |
| installer-vet | 0.305s，退出 0 |
| static | 8.934s，退出 0 |
| ci-filters | 0.416s，退出 0 |
| renderers | 86.263s，退出 0 |
| worker | 0.316s，退出 0 |
| chromium-r100 | 1.594s，退出 0 |
| chromium-r117 | 3.29s，退出 0 |

Node 1290 PASS、0 FAIL、4 本机平台 SKIP；最慢文件 46.821s，集合 ≤240s、每文件 ≤90s。四项 SKIP 为 R82 两个真实 PowerShell 与 R86/R222 两个 Windows 发布入口；由同 SHA Windows CI 实跑。macOS backend race 158.519s，不冒充 Linux ≤120s 证据。格式 522 个 Go 文件、syntax 261 个 JS/CJS/MJS、Worker 15 项、真实 Chromium R100/R117 全部通过。

同一发布 SHA 的完整 `quality-and-windows-release` conclusion=success，且 public 草稿三个附件的实文件/manifest/Release body 验证通过，才能发布 Latest。本次用户已明确授权正式发布，无需沿用上一版 R228 的草稿等待。

## 真机边界

Windows runner 的完整 Windows 测试、public 构建、真实安装升级由本次同 SHA CI 验证。用户 Windows/LCU 真实赛季回补、斗魂小队重启、剪贴板和各页性能仍独立待验，不用 CI 或夹具替代；R229/R230 不因发版自动关闭。
