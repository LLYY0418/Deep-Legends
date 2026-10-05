# 0.12.74 发布执行账本

2026-10-05（北京时间）。R223/R224/R226/R227产品修复与R222/R225流水线合并，key mode **public**。**发布未完成：当前为草稿，Latest仍为0.12.73，按R228 P6等待用户明确确认。**

## 源码与范围

候选tag `v0.12.74` 固定于 `434caa922526f483277c15e9f722db2b9d4da289`，不移动。package/lockfile/CHANGELOG均0.12.74，指纹 **a3d7c1e75735**。

R228到达前已完成版本/源码混合提交和仅推tag；新工单要求的先合R225祖先、先分支CI、版本提交只含三文件的次序不能追溯重写。接手后合并R225全部提交，分支最终代码/夹具验收SHA `3bbd7388e98aebbd440cc86a92629e7f05fff698`；相对原tag只多R69测试睡眠注入和精确延时断言，生产源码/构建指纹相同。说明修订另由 `e763345c` 仅提交CHANGELOG，未夹带源码。

原始R222 jsonl/gz/log及四个旧验证目录未入库；凭据扫描无Riot Key/GitHub token/私钥模式。Worker相对v0.12.73无改动，未新部署、不读/换Secret；15个平台公开状态200且JSON合法。7z level9保持。

## 本地预检

版本变更前完成根Go全量/race/vet、installer test/vet、格式、152个JS语法、CI筛选静态检查、renderer1280项（1276pass/4本机平台skip）、Worker15项和Chromium R100/R117；最终源码快照稳定。原backend普通131.828s、race165.863s（macOS），Node81.849s。随后0.12.74本机/Windows amd64 public后端重建、Key策略、指纹、本机self-test及4项版本生成/receipt测试通过。

R69夹具优化后再跑最终Go普通/race/vet：backend107.053/135.613s（macOS）；定向race三轮通过；public重建/自检/指纹通过。其他Node/Worker/Chromium/installer输入未改，不重复用无关测试争用CPU。起止时间、退出码和输出末5行见 [最终Go预检](history/reports/release-0.12.74/r228-go-preflight-after-fix.json)。本机4个skip为R82两个真实PowerShell用例、R86/R222两个Windows入口用例；对应Windows实际运行均pass。

## 同SHA完整质量门槛

| 范围 | SHA/运行 | 结果 |
| --- | --- | --- |
| tag完整quality-and-windows-release | 434caa92 / [37329122051](https://github.com/LLYY0418/Deep-Legends/actions/runs/37329122051) | success；Linux race115.133s、Node157.798s、最大49.155s；Windows完整测试/构建/校验/真实升级成功 |
| tag public草稿 | 434caa92 / [37329122050](https://github.com/LLYY0418/Deep-Legends/actions/runs/37329122050) | success；public receipt、无内嵌Key、runtime/指纹/SHA256门禁保留 |
| R228首轮分支 | 0d6b760d / [37330990603](https://github.com/LLYY0418/Deep-Legends/actions/runs/37330990603) | 软件CI success，但race139.721s超过120s，**R228预算未通过**；Node222.657s/最大70.790s通过 |
| R228最终完整分支 | 3bbd7388 / [37334082296](https://github.com/LLYY0418/Deep-Legends/actions/runs/37334082296) | success；race115.797s≤120、Node219.765s≤240、最大67.949s≤90 |

最终Windows backend1880顶层项，1854pass/26既有缺样本或opt-in skip，关键82项均pass；installer99/99；Node筛选2/1/2均0skip。R69未知大区HTTP500原真实等待21s，改用既有historyRetrySleep记录并断言3/6/12s；状态/缓存断言保留，Windows该项0.01s，生产默认逻辑不变。第二轮同push/SHA/path生成重复运行37334082894，已取消副本，仅保留37334082296；重复触发原因未确认。

## 草稿与附件

草稿id **403843482**，draft=true、prerelease=false。P4说明只写用户可感知变化，不含流水线条目，不声称真实账号已修复。草稿body和manifest notes与当前CHANGELOG一致；manifest版本/指纹/URL、两行checksum、三个附件实文件size/SHA256/API digest逐项一致。安装包保持原tag CI产物，仅修订草稿说明/manifest/checksum。前后证明均保留： [初始附件](history/reports/release-0.12.74/draft-assets-initial-verified.json)、[最终附件](history/reports/release-0.12.74/draft-assets-verified.json)。

| 附件 | 字节 | SHA256 |
| --- | ---: | --- |
| Deep-Legends-Setup-0.12.74-public.exe | 111817728 | `3bd0153b91b1bfe176b9baa631cd968b9218faacfddfed279e8943c6903f0065` |
| latest.json | 1280 | `20448b4e2ff0673fd8e8de2c1b3244db88a3b74621561d00e16687ddfb1b3f1b` |
| SHA256SUMS-public.txt | 182 | `616cfe462192ddb8179905a9d1c87f32558a852f17bb7b5556fe78a6284b2ffd` |

## 时间线与超标（北京时间）

| 事件 | 时间 | 实测 |
| --- | --- | --- |
| 开始（含公开中转核验与预检） | 22:44:59 | — |
| 原本地预检全部完成 | 22:56:22 | 版本号尚未改 |
| 原版本/源码提交 | 22:58:53 | 434caa92；R228到达前已混合提交 |
| 仅推tag/工作流创建 | 22:59:00 | 未同时推发布分支 |
| tag Linux质量开始→结束 | 22:59:03→23:04:49 | 5分46秒 |
| tag Windows开始→结束 | 22:59:03→23:08:10 | 9分07秒，和Linux并行 |
| 草稿作业开始→结束 | 22:59:04→23:03:23 | 4分19秒 |
| 原tag自动门槛及初始附件完成 | 23:08:10 | tag后9分10秒；不冒充R228最终准备时刻 |
| R228首轮补分支CI | 23:12:55→23:24:44 | 11分49秒；race预算失败 |
| 定位R69、修复与最终Go预检 | 23:24:44→23:34:46 | 定向、普通/race/vet、重建自检 |
| R228最终分支CI创建 | 23:35:39 | 3bbd7388 |
| 最终Linux开始→结束 | 23:35:42→23:42:43 | 7分01秒 |
| 最终Windows开始→结束 | 23:35:42→23:43:28 | 7分46秒，和Linux并行 |
| 用户说明与草稿metadata修订、附件和保留核验 | 最终23:48:02 | 技术准备完成，P6仍需确认 |
| 正式发布Latest | **未执行** | 用户确认后补真实时刻和匿名证明 |

按R228最终准备口径，tag→准备完成 **49分02秒**，超过15分钟；开始→准备完成 **63分03秒**，已超过30分钟，正式完整发布仍未完成，不能写通过。额外环节单列：工单在tag后到达导致补祖先/分支CI；首轮race超标后定位与全量复验；第二轮分支CI；说明与草稿校验修订；P6待用户确认。这些可作为后续工单输入，未移动tag、未用分支CI数值替代tag流水线。

## 升级、保留与真机边界

原tag Windows实际0.12.65→0.12.68→0.12.74，8阶段9.882s，卸旧1.538s、解压6.232s、copy1ms；图标位置/快捷方式创建时间保持。最终分支再次实际升级8阶段 **12.819s**、卸旧2.070s、copy1ms，同样保持；完整阶段毫秒见 [tag阶段摘要](history/reports/release-0.12.74/update-install-timing-summary.json)、[分支阶段摘要](history/reports/release-0.12.74/r228-update-install-timing-summary.json)。用户机器0.12.73在线更新未验。

发布前旧 **16** 个Release/附件与 **17** 个既有tag均保持，当前v0.12.74 tag仍434caa92，Latest仍0.12.73；见 [P6前证明](history/reports/release-0.12.74/r228-prepublication-proof.json)。未执行正式发布、匿名Latest0.12.74与匿名三个附件验证，这三项等待P6确认后继续；因此仍写“发布未完成”。

R223国服真实20场/赛季/详情、日服与入口；R224勇敢举动/真实各模式重绘录像/标签间距等仍待用户Windows证据。事故具体SGP字段无真实证据，不宣称已查明。按 [可照做的真机清单](r228-windows-client-checklist.md) 独立记录，失败新开工单，不回改R223。R223/R224保持进行中。
