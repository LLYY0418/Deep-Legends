# R235 执行账本

日期：2026-10-06（北京时间）。工单：[R235](WORKLIST-R235-SEASON-WINDOW-CAP-CHAMPION-STATS-SLOW-TABLE-SCROLL-JUMP-RELAY-PEAK-HOUR-MAYHEM-TIMEOUT-JP-STARTUP-OVERLAY-FILTER-SWITCH-AND-AKARI-TAGS.md)。

## 范围与交付边界

本地实现覆盖 P1–P11；Worker `/health` 已按工单中已有的部署授权部署并实测。真实客户端数据、日服/国服 Windows 时间线及连续三个晚高峰仍待采集，工单保持进行中。不能把合成夹具当成真实 313 场回补、真实 ≤2秒总览或真实晚高峰 ≤6秒证明。

执行起点 HEAD：`b37357b2a6cf29397ac0f16255d37fa1566e87ac`。实际 `desktop/package.json`/lockfile 版本为 **0.12.75**；工单文字里的 0.12.76 与工作区实际版本不同。本单不占用发布版本号、不推 tag、不发布 Latest。验证构建使用当前实际版本，并明确标为 **public**。

工作区进入本单时已叠加安装器、授权、分享导出和 R232/R233/R234 等改动。初始差异和目标文件副本保存在 `/tmp/r235-baseline/`。共享文件仅叠加 R235 内容；未回退已有改动，未提交或打包这些混合改动。按初始脏工作区重建并逐文件比较：58个原有跟踪文件保持不变；[归属报告](history/reports/r235/scope.json)与[R235增量diff](history/reports/r235/r235-only.diff)提供独立审阅入口。R232–R234 的账本、发布目录、授权协议和版本配置不属于本单交付。

## P1：赛季时间窗与本人补充来源

- 磁盘缓存 schema 1–14 均保留历史 GameID、逐英雄累计及队列聚合；旧 schema 无流状态时撤销旧完成标记，重新建立流游标，保留原始累计数据。
- 上游返回结束但最早逐场时间仍晚于赛季开始：内部 `stop_reason=upstream_window`、`capped_by_upstream=true`；不再只按 offset 1000 判断，也不添加 UI 方法论说明。
- 本人国服被截断时后台探测：`/lol-career-stats/v1/summoner-games/{本人PUUID}` 的无参数/年份/赛季变体，以及 LCU 历史 `begIndex=1000&endIndex=1019`。只接收有 GameID、时间、支持的队列和本人身份的真实逐场数据，按 GameID 并集去重；只有 career 聚合不能补造场次。
- 每次响应记录 `season_self_source_probe` 的来源类别、变体、字节数、逐场数、新增数、最早时间及失败原因，不记账号或完整路径。身份改变后放弃应用；网络失败或 panic 释放单飞标记，完成的探测每个客户端会话只做一次。
- **实测结果：本机没有正在登录的 League 客户端，因此两个真实来源尚未探测。** 这是未采集，不是已证明“源不可用”。不能声称本人已从 217 补到 313，也不能以夹具 257 场替代真实数据。后续需取得本人 `season_self_source_probe` 与赛季快照后再判断是否能补齐，无法补齐时由用户决定是否接受。
- 夹具覆盖时间窗结束、补充 1 场与重复 ID 去重、schema 全版本保留，以及首快照落盘后第二页仍可累计。修复了首页落盘消费 `newInfos` 后下一页写入空 map 的边界。

## P2 / P7：首卡和本人总览时间线

| 场景 | 改前证据（工单提供） | 改后自动验证 | 真实机边界 |
| --- | --- | --- | --- |
| 国服陌生玩家赛季卡 | >3秒；先等召唤师与串行扫描 | 到达请求即后台头扫；ranked/mayhem 并发；首个 100ms 夹具页在300ms内推送；两流启动差<50ms；下一页400ms不会拖住首推 | 真实SGP ≤2秒待验 |
| 日服本人、有磁盘快照 | identity 3.35秒 + history 6.38秒，总览9.77秒 | 完整总览夹具 **6.41ms** 返回快照；历史6秒后 SSE；中转调用 **0** | Windows连接成功→首卡≤2秒待验 |
| 日服本人、无快照 | 同上 | 完整总览夹具 **3.002秒** 先返回资料；历史6秒后 SSE；只有1个冷历史请求 | 网络/客户端启动真实值待验 |

证据：[本人总览夹具](history/reports/r235/self-overview-fixture.log)。≤300ms 的快照路径、6秒慢历史/SSE、旧请求代次隔离及公开引用的 Participants 深拷贝均有测试。首屏迟到 SSE 保留已翻页尾部，旧代次不能覆盖新请求或落盘；新败场到达时原地更新头像旁连胜徽章，不重挂头像/名称元数据。

本人身份复用已识别的 LCU current-summoner；不等待中转。空或 pending 的本人历史不会再触发第二个串行 30天 LCU 历史请求。

连接事件流方面：`runConnectedSession` 已并发启动快照预热和目标检查；settings watch 也改为异步入口。该入口内部本就会启动任务，不能据此声称原来的3秒一定来自它。真实“连接成功→事件流连接成功”改后时间尚未采集，R235不承诺缩短客户端登录64秒或客户端自身初始化23.9秒。

## P3 / P4：滚动与返回按钮

真实 macOS Chromium，加载生产前端及明确标记的合成 API 夹具：

| 主题 | 点底部行前/后 top | 更多对位前/后 top | 1280/780 返回箭头中心差 |
| --- | --- | --- | --- |
| 深色 | 805 / 805 px | 544 / 544 px | 两详情页均0px |
| 浅色 | 805 / 805 px | 544 / 544 px | 两详情页均0px |

被点击行或“更多对位”按钮展开前后按 top 差补偿滚动；数据表容器与 `.match-list` 均 `overflow-anchor:none`。返回箭头为16px左向SVG。子页返回直接放回总览暂存，保留 career/list/strip 节点身份。

证据：[Chromium测量](history/reports/r235/chromium.json)；[深色录屏](history/reports/r235/dark-table-anchor.webm)、[浅色录屏](history/reports/r235/light-table-anchor.webm)；对应 `*-return-{masteries,champion-table}-{1280,780}.png`。

## P5：Worker 部署、中转与三个晚高峰

部署：2026-10-06，北京21:32；Cloudflare已有OAuth登录，无需新密钥。`wrangler deploy` 版本：`87c17775-fafe-48bf-930a-8c575852d8b1`。Worker测试16/16通过。

实测同一部署：

| 路径 | 北京时间 | 结果 |
| --- | --- | --- |
| `/health` | 21:32:33 | 204，`Cache-Control: no-store` |
| `/r/xx/foo` | 21:32:34 | 404 |
| `/r/kr/lol/status/v4/platform-data` | 21:32:35 | 200；`X-App-Rate-Limit: 100:120,20:1` |

[部署回执](history/reports/r235/worker-deployment.json)。部署curl之外，还调用**生产探测函数真实联网**：`riot_relay_probe result=ok/http_status=204/ttfb_ms=646/backoff_s=0`，请求数1，没有回退；[实测日志](history/reports/r235/live-worker-probe.log)。该临时验证文件已移除，未改生产代码。当前执行环境的646ms不是用户国内晚高峰指标，也不是Windows真机日志；Windows软件链路仍待验。

实现与夹具：业务请求不等待探测；10分钟内成功直接发业务；首次业务和探测并发；health/fallback各8秒；单次15秒；网络错误重试1次、间隔500ms；连续3次网络错误才15秒退避。账号磁盘缓存24小时，命中即复用并后台刷新。失败的 auth/network 按 account/summoner/league/mastery/match 类别分开记录。

新的10分钟 `riot_relay_request_summary` 增加**业务请求** `ttfb_samples`、`ttfb_median_ms`、`ttfb_p90_ms`，不含身份、URL或凭据；偶数样本中位数取中间两值平均。测试覆盖4个合成样本100/300/200/400ms：中位数250ms、P90 400ms，以及路径分类。**这不是实际晚高峰汇总。**

三个连续北京20–22点的改后数据：尚未取得（单次21:32的curl不能作为一个晚高峰统计）。需要每晚保留该时段所有10分钟汇总，记录业务网络失败数/请求数及各窗口 TTFB 中位数/P90；不从窗口中位数推造整晚精确中位数。

第二节点不是必须；目前已处理health误判和业务阻断。它可能改善国内到Cloudflare的链路，提升比例必须实测，不能预先承诺；不会增加personal key额度。三个晚高峰中失败率>5%或P90>4秒时将原始窗口数据交给用户决定是否租节点。未租机器、未新增第二节点；production key申请不属于本单依赖。

## P6：海斗超时与重试

响应头10秒、收到头后的响应体25秒，前端40秒；未收录缓存5分钟，网络/超时失败不缓存，失败可立即重试。主总览和生涯弹窗共用绑定；更新弹层内容保留触发按钮和焦点。

慢夹具：先失败一次，再8秒收到头、15秒收到体（总23秒）成功，分数2350；诊断校验 `ttfb_ms>=7900`、`bytes>0`。**真实 ARAMKit ttfb/bytes 尚未采集。** 深浅主题均保存失败/重试成功截图：`*-mayhem-failure-retry.png`、`*-mayhem-retry-ready.png`。

## P8：启动遮罩与自动分组

遮罩等本人首个总览卡ready才消失；找不到进程持续5秒才失败；硬超时120秒；启动30秒不提前消失。诊断hide_reason包括 self-tab-ready/connect-failed/hard-timeout。客户端identityReady后跟随本人分组和标签；近10秒手动切换优先。

Node覆盖30秒、120秒及连续5秒无进程；真实Chromium启动夹具保存 `*-startup-overlay.png`。真实日服登录→识别→首卡→遮罩与分组时间线待Windows客户端验收。

## P9：数据标签与可读性

自行实现工单给出的 LeagueAkari 规则，没有复制其组件源码。后端新增 optional challenges、治疗/拆塔字段，SGP/Riot/LCU转换保留缺失为空。新数据标签遵守全场/队内最大值、优先级、地图和时长阈值；保留MVP/SVP、关键词、多杀、零阵亡，同义标签去重。

一行按实际宽度收进 +N；悬停包含全部标签及数据句；不截断数组。ResizeObserver只在宽度变化时重算，跳过离屏 content-visibility:auto 卡片，避免切回200场强制布局。

深浅Chromium截图包含★与队内标签及+N悬停；测得最低文字对比度 **5.9177**，达到≥4.5。证据：`*-tags-star-team.png`、`*-tags-overflow-tooltip.png` 及 chromium.json。

## P10：筛选暂存

每筛选DocumentFragment保存同一列表、图片、展开/detail/build状态和滚动；TTL5分钟，最多4个LRU视图；超过30秒后台只读头1页，只有新GameID才插入。Node31秒用例证实1次请求、begIndex0/count20、旧卡节点仍相同。

最终Chromium200场“全部→灵活→全部”：深色 **17.6ms**，浅色 **17.7ms**；同一批200节点、切回窗口超过50ms的长任务均 **0**。首帧耗时及长任务用Performance API采集。Mac合成夹具不替代Windows真机，但验证生产代码路径。

## P11：3x6胜负与连胜

以后端同一场实际 subteam 数的 `ceil(队数/2)` 为准：6队前三，8队前四。缺少实际队数时1750按6，1700/1710按8，未知队列不猜。归一化result供卡片、聚合和连胜使用，详情高亮与demo规则一致。

6队名次1/2/4/3/4/5不显示连胜；连续前三3场仍可显示3连胜，5场可强样式；8队边界4胜/5负。重开、未知和特殊模式沿用既有规则。

[全局阈值检索](history/reports/r235/arena-threshold-search.txt)：剩余 `<=4` 仅是段位小段（champions_structured/opgg_insights/overview_current_game）、存储轮转代数、技能槽、拖动距离及海斗stage，不是斗魂胜负阈值。`arena_result.go`、JS helper和demo均无固定4名胜负判断。

深浅 `*-arena-3x6-no-false-streak.png` 使用用户提供的显示名“甜甜酱QvQ#34839”和合成3x6逐场夹具；明确不是真实账号战绩截图。用户真实3x6与斗魂重启小队不散仍待真机。

## 最终自动验证与构建

最终生产源码指纹 **e7a91facbf4b**，所有下列结果对应该源码；未增加版本号、未发布。旧失败尝试保留在 `history/reports/r235/attempts/`，不作为通过证据。

| 检查 | 最终结果 | 证据 |
| --- | --- | --- |
| `go test -count=1 ./backend` | PASS，164.616秒 | [Go全量](history/reports/r235/go-full-final.log) |
| `go vet ./backend` | PASS | [vet](history/reports/r235/go-vet-final.log) |
| R235/R231赛季、探测及R206/R208相关 `-race -count=1` | PASS，59.251秒 | [race](history/reports/r235/go-race-final.log) |
| web/desktop/scripts 全部文件，`node scripts/test-renderers.cjs all` | PASS；1322项，1318通过、4个条件跳过；164文件；171.983秒；最大单文件46.336秒 | [完整日志](history/reports/r235/renderers-all-final.log)、[汇总](history/reports/r235/renderer-summary.json) |
| Worker | 16/16 PASS | [Worker](history/reports/r235/worker-tests-final.log) |
| installer test/vet | PASS（安装器源未改，Go测试缓存命中） | [test](history/reports/r235/installer-tests.log)、[vet](history/reports/r235/installer-vet.log) |
| JS syntax | 148文件 PASS | [syntax](history/reports/r235/js-syntax.log) |
| 真实Chromium | 深浅主题、1280/780、滚动/标签/重试/遮罩/3x6/200场切换 PASS，异常0 | [Chromium](history/reports/r235/chromium.json) |
| `git diff --check` | PASS | 最终执行检查 |

Renderer全量使用3个文件worker以控制同机CPU争用（`NODE_OPTIONS=--require=/tmp/r235-renderer-workers.cjs`，仅覆写 `os.availableParallelism()` 为3）；没有过滤用例或放宽断言。4个条件跳过不作为Windows验证。全部文件仍满足每文件≤90秒、总量≤240秒。

全量核验期间修正了两类后台清理夹具（临时目录销毁必须等赛季任务完成），以及旧R86取消夹具：原断言按4并发上界，默认却是8；现在明确4并发，假Transport遵守取消，保留原5–8请求/配额≤12断言，连续20次通过后重跑Go全量。没有改动该旧业务功能。

验证构建：**0.12.75 / public / e7a91facbf4b**。macOS arm64和Windows amd64后端已重建；指纹、无内嵌Riot key校验通过；macOS在独立临时数据目录自检通过。仅验证后端，**不是安装包**，包含共享工作区已有其它源改动，Windows执行/升级未验证。产物均带 `-public` 文件名，未留下本单private同名产物。

[构建回执（命令、路径、大小、SHA256）](history/reports/r235/public-build.json)、[构建/自检日志](history/reports/r235/public-build.log)。产物在 `dist/r235-validation/`：

- `loot-service-0.12.75-r235-darwin-arm64-public`
- `loot-service-0.12.75-r235-public.exe`

六类变异已经逐项修改真实生产源码、执行断言并恢复原始字节；六项均FAIL，非编译错误：[mutations.json](history/reports/r235/mutations.json)。涵盖迁移清缓存、斗魂固定4、两流串行、探测共享预算、遮罩15秒和标签slice(0,4)。
