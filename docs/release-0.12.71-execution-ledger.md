# 0.12.71 合并发布账本

2026-10-04 用户要求「发布新版本」，恢复先前暂停的桌面构建、打包与正式发布。合并 R206～R210 及 Riot API Key 说明图标追加修改，版本 **0.12.71**，key mode **public**，不读取个人 Key。

## 发布前核对

- 当前 Latest 为 0.12.68，0.12.69 为未发布草稿；0.12.71 尚不存在。既有 Release、附件和标签基线保存到 [发布前元数据](history/reports/release-0.12.71/release-metadata-before.json)、[标签基线](history/reports/release-0.12.71/tags-before.json)，旧版本保持不变。
- package.json 与 lockfile 均为 0.12.71。R206～R210 原有自动验证结果保留；追加设置说明调整已通过定向检查及宽窄窗浏览器回放。
- 发布预检覆盖发布文件生成、public receipt/Key 门禁、样式预算、R204/R210 设置与实时对局、Worker，原始日志见 [preflight-node.log](history/reports/release-0.12.71/preflight-node.log)。
- 不纳入未跟踪的旧 R116/R121/R166/R174 验证材料。正式 Windows 构建使用 release.yml；完整质量及实际旧版升级使用 ci.yml。待所有必需流水线通过后才发布 Latest，并按 R199 核对匿名清单。

## 当前状态

发布准备中，尚未正式发布；构建和发布结果随后追加。用户 Windows 真机界面、战绩、配额、推荐、收藏、赛后清空、桌面图标及升级耗时的验收边界继续保留，自动化 Windows runner 不替代用户真机验收。
