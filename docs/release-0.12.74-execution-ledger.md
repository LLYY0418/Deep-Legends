# 0.12.74 发布执行账本

2026-10-05 用户要求“发布新版本”。合并 R222～R227 已完成修复，key mode **public**。R223/R224 Windows 真实游戏客户端验收仍待执行；本次发布不自动关闭这些工单。

## 发布前门槛

开始：2026-10-05 22:44:59 +08:00。本地预检全部完成：22:56:22。最终源码快照无变动，格式、152 个 JS 文件语法及凭据扫描通过。根模块 Go 全量/race/vet、installer test/vet、CI 筛选静态检查、完整 renderer、Worker 15项和真实 Chromium R100/R117 均通过；详见 [预检摘要](history/reports/release-0.12.74/preflight-summary.json)。Go 后端全量 131.828s、race 165.863s。renderer 集合 81.849s，所有文件维持90s门槛、集合240s门槛；平台跳过项交由 Windows CI 验证。

Worker 源码相对 v0.12.73 无变化，沿用已部署版本；15个平台公开状态均200/合法JSON。只查询公开状态，未读取 Secret 或玩家数据。见 [生产中转核验](history/reports/release-0.12.74/worker-production-verification.json)。

预检完成后同步 package/lockfile/CHANGELOG 到0.12.74。只推 tag，不同步推发布分支。原始日志与旧捕获材料保留本地，未纳入发布提交。

## 发布状态

发布未完成。等待同一 SHA 的完整质量、Windows全量测试/public构建/校验/实际升级成功及草稿三附件验证，再发布 Latest。正式 SHA、指纹、工作流、附件、时间线及匿名证明由发布后续记录补齐。

本机及Windows amd64 public后端重建/Key策略/指纹核验通过，本机self-test通过；版本0.12.74、指纹 **a3d7c1e75735**。版本生成/receipt/指纹专项4/4通过。Windows安装包和实际执行交由同SHA CI。

## 时间线（北京时间，进行中）

| 事件 | 时刻 | 证据 |
| --- | --- | --- |
| 开始 | 22:44:59 | 含独立核验、预检与公开中转状态 |
| 本地预检全部通过 | 22:56:22 | 10组检查、源码快照稳定 |
| 提交发布版本 | 22:58:53 | `434caa922526f483277c15e9f722db2b9d4da289` |
| 仅推tag | 22:59:00 | `v0.12.74`，两个工作流同时创建 |

质量：[37329122051](https://github.com/LLYY0418/Deep-Legends/actions/runs/37329122051)；public草稿：[37329122050](https://github.com/LLYY0418/Deep-Legends/actions/runs/37329122050)。二者 headSha 都为发布提交。1280项renderer：1276通过、0失败、4项需Windows/PowerShell而跳过；Windows专属门槛必须在本轮实际运行通过。

## R228 接手边界

R228 于23:06到达时，0.12.74版本提交和候选tag已完成，Latest尚未发布。已按P6停止正式发布；不移动现有tag。先补齐R225全部提交的分支祖先、分支完整CI及P7真机清单。原步骤的先后次序保留为事实，不改写成R228到达前已按其P1/P3/P4执行。草稿说明将按P4移除流水线条目；代码和public指纹不变。
