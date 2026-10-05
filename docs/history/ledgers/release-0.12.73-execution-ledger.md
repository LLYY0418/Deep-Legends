# 0.12.73 合并发布账本

2026-10-05 用户要求“发布新版本”，恢复构建、打包、生产中转部署与正式发布授权。合并 R211～R221 已完成的产品改动；评分为 v2.1，开放六种关键词，v3 研究未上线。key mode **public**；不读取或嵌入个人 Key，Worker 复用现有 Secret。

## 发布准备与候选处理

当前正式 Latest 是 0.12.71；0.12.72 候选因旧桌面/构建测试夹具缺依赖及 KR-only 菜单断言而停止，没有创建 Release 或发布 Latest。修正只涉及夹具和新的菜单遍历，定向 14+1 项通过；不移动旧标签。完整记录见 [0.12.72 停止账本](release-0.12.72-execution-ledger.md)。

package.json、lockfile 与 CHANGELOG 同步 0.12.73。发布所需源码、必要 golden、冻结证据和当前账本入仓库；旧 R116/R121/R166/R174 验证材料排除，原始研究采集数据不入库或安装包。没有重跑一次性评估。

## 生产 Riot 中转

部署版本 `a56f37a5-95bf-4997-949f-4c1336a7ce4c`，15/15 平台公开状态接口均为 200 JSON，SEA Account 和不支持路径均为 404。没有读取或替换既有 Secret，没有请求真实玩家 API。首轮 Python urllib 收到非 JSON 403，匿名 curl 重复同一 URL 全部通过，记录保留。见 [部署日志](../reports/release-0.12.73/worker-deploy.log)、[生产核验](../reports/release-0.12.73/worker-production-verification.json)。这不代表已验证用户中国网络或真实游戏客户端。

## 构建与正式发布

发布源码为 `245a579b7356ffb436b71a528e00da119f230f18`，标签 `v0.12.73` 固定到同一提交，指纹 **7c34391c96f6**。本地桌面/构建/Worker 预检 334 项：331 通过、3 项因 Windows/PowerShell 环境跳过、0 失败，351.030 秒；见 [预检日志](../reports/release-0.12.73/preflight-final.log)。

正式 Windows public 工作流 **37298993273** 成功，草稿 id **403626344** 恰好三个附件。下载实文件与 API size/digest、两条 checksum、manifest 版本/指纹/URL、Release body 和 CHANGELOG 均匹配；独立只读复核无阻塞。public receipt 与内嵌 Key 门禁成功，安装包不含个人 Key。见 [工作流](../reports/release-0.12.73/release-workflow.json)、[构建日志](../reports/release-0.12.73/windows-release.log)、[草稿附件验证](../reports/release-0.12.73/draft-assets-verified.json)。

| 附件 | 字节数 | SHA256 |
| --- | ---: | --- |
| Deep-Legends-Setup-0.12.73-public.exe | 111787520 | `091421bc80714eb636f1fea7c644f824f938f179c4b71a3ae62f359de5cc1628` |
| latest.json | 1606 | `bd11ce293189eda6f23f4c4d6ae9baf5c2fad3c2d420b772b0171fc6b5fc12e6` |
| SHA256SUMS-public.txt | 182 | `a439852357f07d7f5e4be8bd29a508f9576fe3a177882a38d15d500fef8e792e` |

完整质量流水线 **37298993269** 的 Linux quality 作业成功：Go 全量 race 235.246 秒、vet、前端/桌面 1218 项（1217 通过、1 项需 Windows 跳过、0 失败）、Worker 15/15、真实 Chromium 图片队列与延迟 CSS 护栏、installer 模块、内嵌皮肤池检查全部通过。见 [质量日志](../reports/release-0.12.73/quality-linux.log)。

完整质量与 Windows 工作流 **37298993269** 最终 `conclusion=success`，源码 SHA 与正式构建一致。Windows runner 实际安装 **0.12.65 → 0.12.68 → 0.12.73**；八阶段完整、耗时 **12530ms**，卸载旧版 **1828ms**、copy **262ms**，独立图标位置正确，桌面快捷方式创建时间未变。见 [最终工作流](../reports/release-0.12.73/quality-workflow.json)、[完整日志](../reports/release-0.12.73/quality-full.log)、[实际升级摘要](../reports/release-0.12.73/real-upgrade/real-upgrade-summary.json) 及同目录原始阶段与诊断。

[0.12.73 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.73) 于 **2026-10-05 19:25:58 +08:00** 正式发布（UTC 11:25:58），release id **403626344**，`draft=false`、`prerelease=false`、`isLatest=true`。Latest 匿名清单为 **0.12.73**，与已验证 `latest.json` 字节一致。

正式 tag 的三个附件另经匿名 curl 下载，禁用 curl 配置、清空 Authorization、no-cache 与唯一查询参数；全部 size/SHA256 与草稿及 API 一致。发布前的 **15** 个旧 Release/附件、**16** 个既有标签均保持不变（含未发布的 v0.12.72 候选），比较仅忽略 updated_at/下载计数等会变化字段。见 [发布证明](../reports/release-0.12.73/publication-proof.json)、[匿名 Latest](../reports/release-0.12.73/anonymous-latest.json)、[正式元数据](../reports/release-0.12.73/published-release.json)、[旧版本保留证明](../reports/release-0.12.73/old-releases-preserved.json)。

public 构建、生产中转部署、正式发布和验证记录收尾完成。此后只提交文档证据，不移动发布标签；真实游戏客户端验收边界继续保留。

## 真机边界

用户机器日服总览、LCU 自动恢复、练习工具、外服对局、切回国服、熟练度字段与徽章、构建玩家选择、剪贴板及赛后缓存仍待真实客户端验收。Windows runner 安装升级只验证安装器，不替代这些游戏客户端场景；对应工单不因发布自动关闭。
