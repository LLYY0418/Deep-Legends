# <版本> 发布执行账本

日期（北京时间）：<日期>。执行范围：<对应工单>。

## 发布前门槛

- 本地预检结束时刻/结论/日志：<全部通过后，才提交版本号并推 tag>。
- 最终源码 SHA：<SHA>；package/lockfile/CHANGELOG 版本：<版本>。
- 推送方式：**只推 tag，不同步推 `codex/release-*` 分支**；如需发布分支，在 tag 工作流结束后推。
- key mode：**public/private（明确填写）**；public 附件必须带 `-public` 后缀。
- 指纹：<指纹>；tag：<tag>；三个 public 文件的 size/SHA256/API digest/manifest/Release body 核对：<证据>。

## 同一 SHA 的完整质量门槛

**只有同一 SHA 的完整 `quality-and-windows-release` 工作流 `conclusion=success` 后，才能发布 Latest。**

| 门槛 | SHA | 工作流/作业链接 | 结论/证据 |
| --- | --- | --- | --- |
| Linux quality：Go race/vet、web/desktop/scripts、Worker、Chromium、installer | <SHA> | <链接> | <结论> |
| Windows 专属：backend/installer Windows/DPAPI/COM、R86/R222 release gates、R82 PowerShell | <SHA> | <链接> | <结论> |
| Windows public 构建、校验、实际升级 | <SHA> | <链接> | <结论> |
| **完整 quality-and-windows-release** | <SHA> | <链接> | **success** |
| release.yml public 草稿与附件 | <SHA> | <链接> | <结论，不能替代完整质量门槛> |

Linux 每个发布 SHA 完整跑一遍；Windows 构建仅在 `GITHUB_ACTIONS=true` 时使用 `-SkipTestsInCI`，保留格式、syntax、指纹、自检、NSIS、runtime/receipt/SHA256 校验。
首次 R222 验收手动启用 `measure_windows_renderers`；记录 `renderer-timings-linux/windows` artifact（Node 总时长 ≤240s、单文件 ≤90s）和 backend Linux race 包时长 ≤120s。常规发布不重复跑 Windows 桌面全量。

## 时间线（北京时间）

| 事件 | 时刻 | 耗时/证据 |
| --- | --- | --- |
| 开始（含 Worker 部署与预检） | <时间> | <日志> |
| 本地预检全部通过 | <时间> | <日志> |
| 提交发布版本号 | <时间> | <SHA> |
| 推 tag | <时间> | <tag SHA> |
| quality 开始/结束 | <时间/时间> | <耗时> |
| windows-build 开始/结束 | <时间/时间> | <耗时，确认并行> |
| release 草稿作业开始/结束 | <时间/时间> | <耗时> |
| 同 SHA 完整质量成功，附件验证完成（可发布 Latest） | <时间> | <tag 到此 ≤15 分钟> |
| 正式发布 Latest | <时间> | <release id> |
| 全流程 | — | <开始到正式 Latest ≤30 分钟> |

7z level 9 不变；用户端安装升级另记实测阶段，不与 CI 等待合并。

## 发布证明与边界

Release id：<id>；`isLatest=true` 查询：<证据>；匿名 Latest manifest 版本：<版本/证据>。
缺任何一项只能写“发布未完成”。旧 Release/tag/附件保持的证据：<链接>。
未达到耗时目标、被跳过用例、Windows/用户游戏客户端未验证的部分：<明确填写，不用预计值代替实测>。
