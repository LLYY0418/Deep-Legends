# 0.12.75 发布执行账本

日期：2026-10-06（北京时间）。用户授权“发布新版本”，并要求按最新 R230 已完成内容更新；包含去除构建切换头像数字、二级表格箭头贴近头像两项后续调整。key mode **public**。**已发布为 Latest：北京时间 2026-10-06 12:29:39；release id 404311108，匿名清单与三个附件全部验证通过。**

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

## 发布源码与 tag

源码提交 `26b42ee1`，版本提交只含 package/lockfile/CHANGELOG 三文件；发布 SHA `0e11afd6b9e057af9fdae6c479b56f1b4eb000a1`，版本 **0.12.75**，指纹 **09d40c535496**。public 本地 Windows/macOS 后端重建、内嵌 Key 策略/指纹核对与 macOS 隔离 self-test 均通过。版本相关测试亦通过；详见 build-validation.json / version-checks.json。

| 事件 | 北京时间 |
|---|---|
| 开始 | 2026-10-06 12:11:09 |
| 本地全部预检通过 | 2026-10-06 12:19:10 |
| 版本提交 | 2026-10-06 12:20:13 |
| 只推 tag 开始/结束 | 2026-10-06 12:20:22 / 2026-10-06 12:20:27 |

## 同一 SHA 完整质量与 public 草稿

| 门槛 | 作业/工作流 | 结论 |
|---|---|---|
| Linux quality：race/vet、renderer、Worker、Chromium、installer | [quality](https://github.com/LLYY0418/Deep-Legends/actions/runs/37413242057/job/112106134231) | success；race 118.44s≤120s；Node 220.747s≤240s、最慢 68.181s≤90s |
| Windows 全量、发布入口/PowerShell、public 构建、校验、实际升级 | [windows-build](https://github.com/LLYY0418/Deep-Legends/actions/runs/37413242057/job/112106134086) | success |
| 完整 quality-and-windows-release | [37413242057](https://github.com/LLYY0418/Deep-Legends/actions/runs/37413242057) | **success，同 SHA 0e11afd6** |
| public 草稿 | [37413242037](https://github.com/LLYY0418/Deep-Legends/actions/runs/37413242037) | success；三个附件，public receipt/Key/runtime/指纹门禁通过 |

Linux Node 1292 PASS、2 平台 SKIP、0 FAIL；仅 Windows 发布入口两项在 Linux 跳过，已由 Windows 实跑。Windows backend 1896 项，1870 PASS、26 个既有缺样本/opt-in SKIP，82 个关键项全部 PASS；installer 99/99，Windows 发布门禁 2/2、setup-only 1/1、PowerShell 2/2，均 0 SKIP。原始关键输出见 quality-race-evidence.txt / windows-test-inventory-evidence.txt，完整原始 quality-final.log 仅本地保留（gitignore）；详见 renderer-timings-linux.json 和 ci-budget-summary.json。

## 时间线与实际升级

| 事件 | 北京时间起止 | 实测 |
|---|---|---|
| windows-build | 2026-10-06 12:20:32 → 2026-10-06 12:28:35 | 8分03秒 |
| quality | 2026-10-06 12:20:30 → 2026-10-06 12:27:47 | 7分17秒 |
| public 草稿作业 | 2026-10-06 12:20:31 → 2026-10-06 12:25:47 | 5分16秒 |
| 同 SHA 完整质量成功且附件核对完，可发布 Latest | 2026-10-06 12:29:23 | tag→可发布 9分01秒，≤15分钟 |
| 正式 Latest | 2026-10-06 12:29:39 | tag→Latest 9分17秒 |
| 全流程开始→正式 Latest | 2026-10-06 12:11:09 → 2026-10-06 12:29:39 | 18分30秒，≤30分钟 |

Windows runner 实际升级 0.12.65→0.12.68→0.12.75，8 阶段共 11.937s；卸旧 2.180s、解压 7.560s、copy 1ms；快捷方式创建时间与图标位置保持。关键计时 fingerprint 09d40c535496，result=ok，见 windows-upgrade-summary.json / windows-update-install-timing.json。用户真实游戏客户端另待验。

## 正式发布、匿名附件与保留证明

[正式 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.75)，id **404311108**，draft=false、prerelease=false，Latest API 指向同一 id/tag，isLatest=true。匿名请求禁用 curl 用户配置、清空 Authorization、no-cache，并使用唯一查询参数；Latest 清单与已验证 manifest 字节完全一致。

| 附件 | 字节 | SHA-256 |
|---|---:|---|
| Deep-Legends-Setup-0.12.75-public.exe | 111824384 | `6c4978997b5cedc2a7595931fede5166111b747c797bd7203a700178722de93c` |
| latest.json | 1720 | `e9705a9f7affacbdfe62a45392209d963188bda24f5e9b9a2904102f7a83a80e` |
| SHA256SUMS-public.txt | 182 | `6355f877344a9ed67e6f724598356cfe9b89e637c9c1040aa0781bbc91a805c9` |

manifest version/fingerprint/URL/size/hash、两行 SHA256SUMS、Release body 与 CHANGELOG 顶节均一致。草稿 API browser_download_url 使用 untagged 临时地址，核对时按最终 tag URL 验证 manifest；发布后再确认附件均为规范 v0.12.75 URL，不改 manifest 或安装包。

原 17 个 Release 元数据/附件 id/name/size/digest 与 18 个 tag SHA 全部保持；当前只增加一个 Release 与 tag。独立子代理重新核验三份匿名文件哈希、Latest 字节、同 SHA 工作流、旧 Release/tag 均 PASS；对未找到的被忽略原始日志，主线程直接确认该 1.4MB 文件存在并提取完整 race 步骤为可入库文本，未将独立日志缺口宣称为独立通过。

[发布证明](../reports/release-0.12.75/publication-proof.json)、[匿名 Latest](../reports/release-0.12.75/anonymous-latest.json)、[保留证明](../reports/release-0.12.75/preservation-proof.json)、[独立核验](../reports/release-0.12.75/independent-publication-verification.json)。源码和 tag 不再更改；发布记录以仅文档提交本地保存。完整 tag 工作流成功并正式发布后，额外发布分支推送被自动审批拒绝（理由：本次只授权推 tag）；没有执行分支推送，不影响已验证的 Latest。
