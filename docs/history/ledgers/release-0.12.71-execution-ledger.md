# 0.12.71 合并发布账本

2026-10-04 用户要求「发布新版本」，恢复先前暂停的桌面构建、打包与正式发布。合并 R206～R210 及 Riot API Key 说明图标追加修改，版本 **0.12.71**，key mode **public**，不读取个人 Key。

## 发布前核对

- 当前 Latest 为 0.12.68，0.12.69 为未发布草稿；0.12.71 尚不存在。既有 Release、附件和标签基线保存到 [发布前元数据](../reports/release-0.12.71/release-metadata-before.json)、[标签基线](../reports/release-0.12.71/tags-before.json)，旧版本保持不变。
- package.json 与 lockfile 均为 0.12.71。R206～R210 原有自动验证结果保留；追加设置说明调整已通过定向检查及宽窄窗浏览器回放。
- 发布预检覆盖发布文件生成、public receipt/Key 门禁、样式预算、R204/R210 设置与实时对局、Worker，原始日志见 [preflight-node.log](../reports/release-0.12.71/preflight-node.log)。
- 不纳入未跟踪的旧 R116/R121/R166/R174 验证材料。正式 Windows 构建使用 release.yml；完整质量及实际旧版升级使用 ci.yml。待所有必需流水线通过后才发布 Latest，并按 R199 核对匿名清单。

## 构建进展与真机边界

发布源码提交 `ea6d64c99a3e4ab86f9d9055a7a3fc4b300de4e6`，新标签 `v0.12.71` 固定到同一提交，源码指纹 `696da05d0ad0`。预检 33/33 通过。源码及文档空白检查通过；原始控制台日志保留输出空白，不改写测试证据。正式 public 构建成功后，草稿 id `403010042` 的三个附件经下载及独立复核通过，见 [草稿附件核对](../reports/release-0.12.71/draft-assets-verified.json)。完整质量与实际升级通过后才执行正式发布。重复分支流水线 `37201453422` 已取消，保留同一源码的标签质量流水线。用户真机待办继续保留。


## 正式发布与 R199 证据

[0.12.71 Release](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.71) 已正式发布：release id **403010042**，发布时间 **2026-10-04T12:42:23Z**，`draft=false`、`prerelease=false`、`isLatest=true`。源码与标签固定为 `ea6d64c99a3e4ab86f9d9055a7a3fc4b300de4e6`，指纹 **696da05d0ad0**；收尾账本提交不移动标签。

匿名 curl 禁用配置、明确空 Authorization、使用 no-cache 和唯一查询参数，取得 Latest 清单版本 **0.12.71**，与正式 `latest.json` 附件逐字节一致。证据见 [publication-proof.json](../reports/release-0.12.71/publication-proof.json)、[Latest 查询](../reports/release-0.12.71/latest-after.json)、[匿名清单](../reports/release-0.12.71/anonymous-latest.json)、[正式发布元数据](../reports/release-0.12.71/published-release.json)。

| 正式附件 | 字节数 | SHA256 |
|---|---:|---|
| Deep-Legends-Setup-0.12.71-public.exe | 111698432 | `bbdd9999a4006423ad6484dd78e679cb8c46afc4742160e0d5c8d4510787c0f3` |
| latest.json | 1500 | `8ff7415ad7a15c092559ddb9be7ebdf828f76cf0033f1bb0ad07e4dee9a5715a` |
| SHA256SUMS-public.txt | 182 | `2420bf4123556cc9e26e9312ad808f3a39c4ce89ce524e0a6d7e0c4d5185b165` |

正式 public 工作流 **37201404424**、完整质量与 Windows 构建/升级工作流 **37201404448** 均为 `conclusion=success`，headSha 与发布源码一致。检查覆盖全量 Go/race/vet、Node、Worker、真实 Chromium 图片队列/延迟 CSS 护栏、installer、内嵌运行时/Key 门禁、receipt、指纹及 checksum。见 [正式 Windows 构建日志](../reports/release-0.12.71/windows-release.log)、[完整质量日志](../reports/release-0.12.71/quality-full.log)、[正式构建结果](../reports/release-0.12.71/release-workflow.json)、[质量结果](../reports/release-0.12.71/quality-workflow.json)。

Windows runner 实际安装 0.12.65、升级到 0.12.68，再升级到 0.12.71：八阶段完整且单调，原始 NSIS 包含 uninstall_old_done，诊断导入成功；升级耗时 **13562ms**、卸载旧版 **2197ms**，快捷方式创建时间未变、目标正确且使用独立 app.ico。见 [实际升级摘要](../reports/release-0.12.71/real-upgrade/real-upgrade-summary.json) 与同目录阶段原文/诊断。此耗时仅代表 Windows runner；用户电脑界面、战绩、配额、推荐、收藏、赛后清理、图标和实际升级耗时仍待真机验收。

主线程与独立只读复核确认草稿三附件的实际字节、size/digest、notes、manifest、两条 checksum 和构建源码一致；正式发布后另核对三个正式 tag 下载 URL。安装包只使用 `-public` 名称，没有个人 Key。见 [正式附件验证](../reports/release-0.12.71/published-assets-verified.json)。

发布前后全部 **14** 个既有 Release 及附件元数据保持，旧 **14** 个标签未改动，包括 0.12.60/0.12.69 未发布草稿。比较忽略 updated_at 与下载计数，其他记录字段见 [保留证明](../reports/release-0.12.71/old-releases-preserved.json)、[发布前](../reports/release-0.12.71/release-metadata-before.json)/[发布后](../reports/release-0.12.71/release-metadata-after.json)、[标签](../reports/release-0.12.71/tag-proof.json)。构建、打包、正式发布及证据收尾完成；R206～R210 工单保留真机待验收状态。
