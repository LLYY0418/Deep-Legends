# 0.12.58 GitHub 发布账本

日期：2026-10-03（Asia/Shanghai）。用户明确要求“在github上面发布一下最新版”，授权正式发布 **0.12.58**。发布前 API 确认公开 Latest 为 v0.12.19，v0.12.49 / v0.12.50 为草稿；本轮只发布 v0.12.58。

## 源码与本地准备

- 发布分支 `codex/release-0.12.58`，构建源码提交 `33e4e4a882a3bac7fba99b40325a625bda65f13a`。保留 main，未改动其他版本草稿。
- 排除本机原始日志/截图目录 `docs/r116-validation/`、`r121-validation/`、`r166-validation/`、`r174-validation/`；个人密钥和构建产物没有进入源码提交。
- 从该提交 `git archive` 单独还原后，源码指纹为 **5230b7850e83**，与本地 public 安装包、后端和构建收据一致。
- 本地完整构建和全量回归见 [R194 执行账本](r194-execution-ledger.md)。发布前清单/收据测试 **4/4 通过**；更新专项 `go test ./backend -run '^TestUpdate' -count=1` 通过（1.395 秒）。暂存差异检查与独立只读发布复核通过。
- 本地交叉构建安装包 SHA256 `387fcfdd2d46666da3ed3fef707ba791027d7640cd85379ccad706260b169963`。正式附件采用 GitHub Windows runner 的 public 构建，附件哈希以该构建产物为准。

## GitHub 构建

- 手动触发 [Windows public release draft](https://github.com/LLYY0418/Deep-Legends/actions/runs/37039189632)，目标为上述发布分支和提交。等待工作流创建草稿并全部完成后才正式发布，避免草稿创建与发布竞态。
- 分支 push 同时触发 [quality-and-windows-release](https://github.com/LLYY0418/Deep-Legends/actions/runs/37039185391)。
- 首次 CI（[37038176285](https://github.com/LLYY0418/Deep-Legends/actions/runs/37038176285)）全量 race 检查发现两处旧测试采集器的共享 map / slice 无锁写入：`TestStructuredArenaDetailPreservesAllAugmentRows` 与 `TestR151StructuredArenaUsesYourGGAugmentsAndFallsBackToOPGG`。仅在测试中补 mutex 和锁内快照，断言及生产并发请求保持；两项 `go test -race ... -count=20` 通过（2.506 秒）。源码指纹未改变。旧提交的 [发布构建](https://github.com/LLYY0418/Deep-Legends/actions/runs/37038201667) 已取消，未正式发布。
- Windows public 发布工作流已 **success**：Go 全量通过（187.043 秒），installer 全量与 vet 通过；Web **835/835**，desktop **268 项、266 通过、2 项非 Windows 测试跳过、0 失败**。Windows 专用 PowerShell 门禁实际通过；跳过仅为 bash 与非 Windows 格式门禁。清单/收据专项 **4/4**，更新专项通过（0.642 秒）；打包运行时、指纹、public 密钥策略与收据全部校验通过。日志 `/private/tmp/release-0.12.58-windows-public.log`。
- 质量流程的 quality job 已 **success**，包含格式、vet、全量 race、前端和 desktop 回归、真实 Chromium 两项门禁及 installer test/vet。windows-build job 也已 **success**：Windows Go 全量（180.849 秒）、Web/desktop 测试、public 打包、校验表与 artifact 上传全部通过。整条 CI 为 success；日志 `/private/tmp/release-0.12.58-ci-success.log`。
- 独立只读最终复核确认三附件的真实字节、API digest、大小、版本、源码指纹一致；发布说明与 CHANGELOG 一致。草稿正文已由单条版本说明补成完整累计更新说明并回读核对。

## 正式发布三件套

从 GitHub 草稿下载回来的正式 Windows 附件均重新计算 SHA256，并与 API digest、size、uploaded 状态、更新清单和校验表逐项核对：

| 文件 | 大小 / SHA256 |
|---|---|
| Deep-Legends-Setup-0.12.58-public.exe | 111328256 字节；`ed2f6113d949e9c40e4e4f343433bd60f78cd2f69a89ba7c71a1c9e8b822b3c0` |
| latest.json | 630 字节；`a0ae6ba78528a3c1736cbae9c6a78f73b5ef7ce872696b3e9eff609a207547f2` |
| SHA256SUMS-public.txt | 182 字节；`6f65d868fabfae189f03a67f70f3069b3a3eb92e305d8c79347e2102b1845001` |

清单、校验表和核验元数据归档 [history/reports/release-0.12.58/](history/reports/release-0.12.58/)，正式三件套本地副本在 `dist/github-release-0.12.58-public/`（不入 Git）。

- 已正式发布并设为 **Latest**：[v0.12.58](https://github.com/LLYY0418/Deep-Legends/releases/tag/v0.12.58)。release id `402019897`，发布时间 `2026-10-02T17:38:56Z`；draft=false、prerelease=false。
- GitHub `/releases/latest` 与 Release 列表均返回 v0.12.58 / isLatest=true；v0.12.49 和 v0.12.50 仍为原草稿。
- `refs/tags/v0.12.58` 指向 `33e4e4a882a3bac7fba99b40325a625bda65f13a`，与实际 Windows 构建源码一致。
- 匿名访问 `https://github.com/LLYY0418/Deep-Legends/releases/latest/download/latest.json` 成功；下载字节与已核验清单完全一致，version=0.12.58、fingerprint=5230b7850e83。正式发布后再次核对三个附件的 digest/size 和发布说明，均保持一致。
- 发布动作创建标签后，GitHub 自动触发同提交的重复任务。重复草稿构建 [37042140922](https://github.com/LLYY0418/Deep-Legends/actions/runs/37042140922) 已申请取消：该提交的 Windows public 发布构建此前已 success，且已发布版本禁止重建替换。标签另触发 [37042140790](https://github.com/LLYY0418/Deep-Legends/actions/runs/37042140790) CI，保留其自动执行；发布验收采用同一提交已全部通过的 37039185391。
- 后续只提交发布账本和公共核验元数据，不移动已发布版本标签。

## 发布说明与验收边界

发布说明归档于 [release-0.12.58-notes.md](history/reports/release-0.12.58-notes.md)，包含 0.12.19 后累计更新与本次 R190–R194 重点。

GitHub v0.12.19 的 `backend/update.go:253-261` 只接受无后缀或指纹后缀安装包，不接受 `-public.exe`；该版本需从发布页手动下载安装。当前版本已接受 public 后缀。公开包 key mode 为 public，不包含个人 Riot API Key。

Windows runner 构建和自动测试不等于连接真实游戏客户端的验收。本轮没有声称 R194 隐藏玩家补人、软件内下载重启升级或游戏设置保留已完成 Windows 真机验收。
