# R93 执行与验收记录

日期：2026-09-14。对应 `WORKLIST-R93-SPECIALIST-RUNES-GATE-DISK-TEST-GAP.md`。

按用户本轮明确要求，J 组先调研，功能开放、提示文案和埋点变更均等用户确认；工单 J-2 的自动处理分支本轮不执行。K 组已完成。本轮没有变更娱乐模式门禁，也没有恢复已删除的总览推荐区块。

## 逐项状态

| 条目 | 状态 | 结果 |
|---|---|---|
| J-0 | 沿用既有结论 | 原 skip 是本地门禁；没有把它重新解释成上游返回空榜 |
| J-1 / J-3-1 | 调研完成 | 三队列分别核验，URL、时间、响应结构与限制见下方和 `r93/research/` |
| J-2 / J-3-2 / J-3-3 / J-3-4 | 等用户确认 | 尚未改前后端能力、来源映射、UI 或 reason，故不冒称功能验收或门禁变异已通过 |
| K-1-1 / K-2-1 | PASS | 常规 Go 测试：20 场冷请求落盘，重建两个 provider 后 20 场 `disk`、0 次详情网络调用 |
| K-1-2 | 已调整验收口径 | 不保留延迟性能硬判据；耗时继续记录，删除两个真实采样/验收脚本中的 1500ms 判定 |
| K-1-3 | 已保留 | R92 真机 Python 采样和磁盘变异脚本继续保留，历史观测不改写 |
| K-2-2 | PASS | Go overlay 禁用磁盘读取 → `disk_hits=0 network_calls=20` 转红；恢复后转绿 |
| G-1 / I-1 / I-4 | 未重新安排 | 遵守 R93 范围，不重新跑并发/流水线优化或声称完成 Windows 真机核验 |

## J：结论与边界

建议目前三个队列均不开放绝活哥符文。是否保持现有表现，或添加“该模式暂不支持绝活哥符文推荐”的提示，由用户确认后再做。下面的“无可用来源”指满足“模式 + 英雄 → 真实高手账号 → 可读取对应对局”的整条链路，不是英雄强度/装备/增幅统计。

### 1750 — Arena 3x6

- [Riot 2026-04-14 官方开发日志](https://www.leagueoflegends.com/en-gb/news/dev/dev-leveling-up-arena/)将 3x6 定义为 Arena 的限时活动变体：六队、每队三人，替代通常的八队二人。结合已核实的 CommunityDragon `1750 = Arena 3x6`，不能再猜为仅快照或测试服队列。
- [OP.GG Arena](https://op.gg/lol/modes/arena)和[阿狸 Arena 页](https://op.gg/lol/modes/arena/ahri/build)均实际返回 HTTP 200。前者有三英雄组合的平均名次、第一名率、胜率与选取率，后者有英雄、装备、技能与增幅统计。两页均未检出指向真实玩家个人页的链接。
- OP.GG 已有三人组合统计，说明不能说它完全没有 3x6 相关内容；但页面没有给出该统计与 queue 1750 的明确 ID 映射，更没有已验证的“1750 × 英雄高手玩家”榜单。三人统计不能证明玩家榜覆盖 1750。
- 公开榜单导航为单/双排、灵活排位、英雄、等级、熟练度。用实际韩服李青专家榜作结构对照，可识别 50 个玩家链接；其页面说明按钻二以上场次排名，不能替代 Arena 专家榜。
- 另外检查了 Arena 页面引用的模式入口脚本，HTTP 200、210 bytes，仅为入口引导，没有可识别的玩家榜路由。该检查不覆盖所有共享脚本和未公开 API，不用它证明绝对不存在隐藏接口。

结论：**未找到 OP.GG 已验证的 Arena 按英雄高手账号榜，也没有玩家榜覆盖 1750 的证据**。不把普通排位榜加一个未经验证的 mode 参数后当作 Arena 来源。

### 2400 — ARAM: Mayhem

- [模式页](https://op.gg/lol/modes/aram-mayhem)及[薇古丝增幅页](https://op.gg/lol/modes/aram-mayhem/vex/augments)均 HTTP 200。返回英雄强度/构筑/增幅内容；两页均未检出真实玩家个人页链接。模式路径和页面标题确实对应 Mayhem。
- [OP.GG 官方帮助](https://help.op.gg/hc/en-us/articles/60909599637657-How-to-Check-ARAM-Mayhem-Match-History)，更新于 2026-08-07：该模式记录依赖 OP.GG Desktop 本地保存；用户只能登录后查看自己的记录，不能查看其他用户的 Mayhem 战绩。这里引用的是 OP.GG 自己公布的服务范围。
- 官网的用户攻略入口是投稿攻略，不能作为该英雄高手真实账号及其真实对局符文的数据证明。已有 `aram_mayhem` tier 聚合也不能满足需求。

结论：**当前没有可验证的 Mayhem 英雄高手榜来源；OP.GG 公布的他人战绩访问限制也阻断了后续抓取链路**。不能用常规排位玩家的符文伪装成 Mayhem 专家符文。

### 3140 — Multiplayer Practice Tool Custom

- [OP.GG 模式入口](https://op.gg/lol/modes)实际重定向到 Mayhem，最终 HTTP 200。可见导航列出 Mayhem、Mayhem Classic-ish、Arena、ARAM、URF、Brawl、Doom Bots、Nexus Blitz，没有 Practice Tool / 3140 的模式入口。
- [OP.GG 官方自定义对局说明](https://help.op.gg/hc/en-us/articles/31089445595033-I-want-to-view-my-stats-for-custom-games)，更新于 2025-07-17，明确说明 OP.GG 不提供自定义对局统计。配合已核实的队列名称，其服务范围不支持把 3140 当作公开高手榜来源。
- 官方自定义统计说明与公开模式导航共同支持此判断；不是因为猜测一个 URL 得到 404，也不是因为抓取失败。Riot Tournament API 对带赛事代码的自定义比赛存在另外的结果链路，这不等于 OP.GG 提供训练模式公开高手榜。

结论：**未发现可用的 3140 英雄高手账号榜；OP.GG 官方明确不提供自定义对局统计**。这一判断不扩大为所有服务商永远无法保存任何自定义比赛。

## J：取证方式

`scripts/r93-public-research.py` 为手动联网调研脚本，常规 Go 套件不会运行。证据 `r93/research/responses.json` 保存每个真实请求的 UTC 时间、最终 URL、HTTP 状态、字节数、正文 SHA-256、导航和结构计数，不保存玩家账号行。可见文字检查排除了 script/style，防止共享翻译字符串污染模式判断。玩家链接数只用于结构对照，不能作为“不存在隐藏接口”的证明。

最终直接请求时间为 2026-09-14 08:14 UTC 左右，准确到每个请求的时间以 JSON 为准。Arena、Mayhem 模式页/英雄页、模式导航、普通榜单对照、Riot 官方说明和队列 JSON 均返回 200。两个帮助中心页面通过 Python 直连均为 **403**；这部分如实保留，403 不作无数据的依据。两篇帮助正文另通过 web 浏览工具成功读取，浏览工具未暴露数值 HTTP 状态，记录在 `r93/research/help-sources.json`，不将它编造成直连 200。

复查入口：`python3 scripts/r93-public-research.py`。默认证据输出 `/tmp/deep-legends-r93/research`，可以设置 `R93_RESEARCH_OUTPUT` 更改位置。公开网页会变化，本结论限定为本次已核实来源。

## K：实现与验证

`r93_test.go` 新增 `TestR93TwentyMatchDetailsFromDiskAfterRestart`，自动进入 `go test .`，没有 opt-in 环境门禁：

1. 在独立临时目录下，为 20 个不同 match ID 构造 mock HTTP 响应。
2. 第一轮经过真实 `matchByIDWithCache` 与持久化实现，断言 20 个 `miss`、20 条不同请求路径，每条只请求一次。
3. 重建 champion provider、Riot provider 及各自内存缓存，只共享磁盘目录。
4. 再读同一批 20 场，断言 20 个 `disk`、0 次详情 HTTP transport 调用，同时核对返回的 match ID。

所有 HTTP 请求均被 mock transport 截获，不需要真实 key 或可用外网。虚拟时钟仅省去本地限速等待；没有毫秒或相对耗时断言。本测试覆盖跨 provider 重建的磁盘恢复，不声称模拟了独立进程文件锁；真实终止/重启进程的补充证据仍由 R92 脚本保留。

`scripts/r93-mutation-check.py` 用 Go overlay 将磁盘读条件禁用，源文件不落地修改。先 baseline 绿，再要求变异以特定行为断言 `restart disk_hits=0 network_calls=20` 失败，排除编译失败，最后撤去 overlay 再跑绿并核对生产文件 SHA-256 未变。证据：`r93/mutation/`。

真实脚本也统一为本轮验收口径：`r92-riot-benchmark.py` 和 `r92-verify-evidence.py` 保留耗时，只以成功返回完整 20 场、无详情失败、20 场磁盘命中判定磁盘正确性。不重跑已关闭的 G-1 性能工作。历史 JSON 中记录的耗时和旧判定保持原始记录。

| 验证 | 结果 | 证据 |
|---|---|---|
| `go test . -count=1` | PASS，97.800s | `r93/go-test.log` |
| R93 baseline / 禁用磁盘 / restored | 绿 / 红（0 disk、20 network）/ 绿 | `r93/mutation/results.json` 与三份日志 |
| 新测试与 mutation 独立核验 | PASS；确认无真实网络、无耗时门槛、仅磁盘跨重建共享 | 独立探子定点核验并复跑 mutation |
| 4 个相关 Python 文件语法解析 | PASS | 新调研/变异与旧采样/验收脚本 |
| 真机证据 verifier 的 5 个合成判据样本 | PASS：1070ms、9000ms 的 20 命中均通过；19 命中、HTTP 失败、详情失败均拒绝 | `r93/real-evidence-verifier.json` |

未打安装包、提交或发布。本次功能决策仍停留在用户要求的确认点，J 的功能验收没有被 K 的测试通过替代。
