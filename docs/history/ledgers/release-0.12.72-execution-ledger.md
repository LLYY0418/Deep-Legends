# 0.12.72 合并发布账本

2026-10-05 用户要求“发布新版本”，恢复构建、打包、生产中转部署及正式发布授权。合并 R211～R221 已完成的产品改动；评分为 v2.1，v3 研究未上线。key mode **public**，不读取或嵌入个人 Key；生产 Worker 复用现有 Secret。

## 发布前核对

- 现有 Latest 为 0.12.71，新版本 0.12.72 不复用或移动旧标签。发布前 Release 与附件、标签元数据保留在 [报告目录](../reports/release-0.12.72/)。
- package.json、package-lock.json 与 CHANGELOG 同步 0.12.72。R221（原重号 R220 外服）最终源码 Go 全量、race、vet、前端 938 项、中转 15 项已通过；发布仍使用相同新提交的完整 CI 和正式 Windows public 构建。
- 保留 R211 v3 否决证据，采用用户已接受的 v2.1 与六关键词结果；不重跑一次性评估，不把研究样本或 mock 结果宣称为真实客户端验收。
- 旧 R116/R121/R166/R174 未跟踪验证材料不纳入本次提交；当前工单、必要测试夹具、冻结证据及研究工具归档，不进入用户安装包。

## 构建及发布进展

生产 Worker 部署成功，版本 `a56f37a5-95bf-4997-949f-4c1336a7ce4c`；15/15 平台公开 status 接口返回 200 JSON，SEA Account 与未知路径返回 404。首轮 Python urllib 非 JSON 403，改用匿名 curl 重复公开 URL 全部通过，原记录保留。未读取/替换现有 Secret，未请求玩家身份接口。见 [部署日志](../reports/release-0.12.72/worker-deploy.log)、[生产核验](../reports/release-0.12.72/worker-production-verification.json)。

待补发布源码提交/指纹、正式 public 构建、完整质量/Windows 实际升级、三附件 SHA256、正式 Latest 匿名清单验证与旧版本保留证明。

## 真机验收边界

Windows runner 的安装升级验证与真实 Riot 游戏客户端验收分开记录。用户机器的日服总览、LCU 自动恢复、练习工具、外服实时对局、切回国服、熟练度字段和图标、构建玩家切换、赛后刷新仍待真实客户端验证。相关工单保持待验收状态，不因发布自动关闭。
