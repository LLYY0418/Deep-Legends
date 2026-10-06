# R231 执行账本

日期：2026-10-06。基线版本 0.12.75，未升级版本、未创建发布提交或 tag。状态：本地实现、验证与重建已完成；真实 SGP / Windows 验收及 Worker 部署仍有待完成项。

## 边界与证据

- 按 R231 P1～P14 执行，未修改职业账号归属。已亲自完整读取会话汇总、职业账号归属文档、R231 及相关基础账本。
- 本机没有可用的本人 LCU/SGP 会话，未执行真实 `q_440` 的 offset 0 / 300 探测。因此 ranked OR / mayhem OR 两流仍是待上游核实的实现；不能把假 SGP 的 313 场当作本人真实核对结果。若真实探测要求改为逐队列扫描，还需调整流划分。
- 本机为 macOS；Chrome 检查运行生产页面和合成 API 夹具，不代表 Windows 或日服真实账号。
- Worker `/health` 已实现且自动测试通过，尚未部署。R231 P8 明确写“部署前先问用户”，已提交部署确认问题，未获回答前不部署。
- 工作期间另有 `queue_groups.go` / `queue_groups_test.go` 的斗魂四标签改动出现在共享工作区，予以保留；R100 旧断言改为核查全部 AllowedQueues 和四个服务端标签，未扩大该变更。

## 各项实现与验收

| 项 | 本地结果 | 仍需真实验收 |
|---|---|---|
| P1 | schema 14；ranked `q_420/q_440` OR、mayhem `q_2300/q_2400/q_3270` OR；独立游标/complete；头部每次 offset 0；头部/回补共享单飞；写盘锁与 GameID 并集合并；区分 season_start / upstream_end / upstream_cap_1000 | 本人 q_440 offset 0 / 300 返回条数和最早时间；最终流划分；真实灵活 313 场核对 |
| P2 | 回补预取并发 2，慢页 >3 秒或错误退回串行；新增无网络 GET season-summary；前端按 3 秒节流，只替换 ranks/champions，完成时立即更新 | 同一真实玩家的改后流量与完成时间、目标是否减半 |
| P3 | 详情入口保存 DocumentFragment 和渲染缓存；返回恢复原节点；主页面及浮层重复进入/返回，无 overview / match-tiers 新请求 | 用户真实页面体验 |
| P4 | 同时只展开一行；排序/开合复用主行及头像；显示更多仅追加对位；首列留空，按钮置英雄列 | 用户实际头像、不同队列视觉验收 |
| P5 | 共用 16px SVG，22px 按钮、gap 2；熟练度计数为截取前完整数量，不再显示最高分 | 真实熟练度完整数量 |
| P6 | 构建头像父按钮提示玩家名，保留名字遮蔽；内层图标禁用 tooltip；核查相关父子提示点 | 无额外真实数据断言 |
| P7 | 每次 SVG 渐变独立 ID；双/三/四/五杀文字颜色按工单；四、五杀 11px / 800 与阴影 | 用户显示缩放体验 |
| P8 | 每 origin 8 秒；2xx 只认响应头；旧 Worker health 404 回退；15/30/60/120 退避；连续 2 个网络失败才退避；force 共用在途探测；恢复 SSE；诊断阶段/连接/首字节/实际读取字节/退避 | Worker 部署批准与部署；真实跨境延迟 |
| P9 | 日服本人首屏用 LCU 历史/排位/熟练度；Riot 6 秒后台补既有 MatchID 并 SSE 原位更新；队列内置立即返回、LCU 后台 1.5 秒；明确零场空表可用，缺字段仍失败；失败态保留 3 页签；零单排有灵活时默认灵活 | 日服本人真实首屏 ≤2 秒；实际 OP.GG 页面 |
| P10 | 10 秒内 URI 去重最多 30 条；仅保留已知固定信号/安全命名空间；三类前兆立刻 client-exiting，status.connected=false；15 秒误判恢复 | Windows 关闭前后各 3 次；真实最早信号。现有进程扫描只取 LeagueClientUx/LeagueClient，拿不到 Render 消失时间 |
| P11 | softReset 将 loading 置 idle 并递增 token；取消请求也置 idle；临时后端失败 TTL 30 秒、未收录 5 分钟；前端临时失败 60 秒可重试；生涯缓存签名包含查询状态 | 国服↔外服真实切换后分数 |
| P12 | 保留 R230 已有 content-visibility:auto / auto 180px；200 张生产卡片静置 60 秒达标，因此未追加窗口化 | Windows 长列表实际滚动；合成夹具不能排除用户环境问题 |
| P13 | 同源码快照 public 基线 / CRC-off 两包；仅临时 NSIS include 加 CRCCheck off；下载 SHA-256 校验已存在 | 同一 Windows 主机基线 2 次、CRC-off 2 次升级；未确认杀毒/系统原因，未采用默认关闭 CRC |
| P14 | 普通收起、筛选切换、外部视图收起均清 buildPlayers；详情内切 tab 保留；选择诊断 GameID 后端哈希化 | 用户构建切换体验 |

### P1 / P2 数据边界

假 SGP 专项验证：带标签取到 313 场并遇赛季边界；1000 场截断标记 Complete 和 capped_by_upstream；连续头部扫描只请求 offset 0；并发写盘不丢 GameID / 聚合；ranked 与三个 mayhem 队列同时存在；没有未筛选请求或重复后台 offset；snapshot 禁止网络仍正常返回。

工单旧真实日志：每玩家 120–180MB、回补 13–15 秒。改后同账号数据 **未测**，不填模拟数字，也不宣布“已减半”。前端进度现在只读取落盘快照，避免进度触发整页网络重读。

### P5 量值与 P12 对比

[chromium.json](history/reports/r231/chromium.json) 为生产 Chromium + 合成数据结果。深浅主题分别在 1280 / 780 宽打开可见生涯统计面板，英雄胜率和英雄熟练度的箭头中心与标题中心偏差均 **0px**（8 次测量）。

| 检查 | 改前 HEAD | 改后 |
|---|---:|---:|
| 对局卡片 | 200 | 200 |
| 静置时间 | 60003ms | 60004ms |
| dom_nodes | 31294 | 31763 |
| images | 3887 | 3913 |
| 长任务数量 / 合计 | 0 / 0ms | 0 / 0ms |
| content-visibility | auto | auto |
| contain-intrinsic-size | auto 180px | auto 180px |

改后多出的节点/图片包含本轮交互产生的生涯面板内容，不能把这组数当作 DOM 减少。两份夹具均已达静置门槛，未声称 Windows 实际性能改善。改前结果见 [chromium-before.json](history/reports/r231/chromium-before.json)。

### P6 父子 tooltip 核查

构建玩家按钮、战绩摘要玩家按钮、经典详情参与者、斗魂详情参与者、队伍分析玩家标签、对位推荐玩家标签的内层英雄图标均禁用 tooltip。已核对当前对局近期头像、分路选项、近期洞察图标原本已有同类禁用。构建提示使用父按钮的玩家名并遵循 maskNames；截图悬停点为头像中心。

### P9 文案替换

| 文件 / 用户提示位置 | 结果 |
|---|---|
| champion_table.go：外服不支持的模式 | 使用实际大区标签（如日服） |
| champion_table.go：英雄数据读取失败 | 使用实际大区标签 |
| riot_api.go：外服服务额度耗尽 | 使用请求平台标签 |
| riot_api.go：HTTP 超时错误体 | 可传 region，日服测试不含韩服 |
| overview_current_game.go：实时对局失败 | 使用实际大区标签 |
| overview_current_game.go：不支持的平台 | 使用外服通用提示 |

韩服专用的职业选手、绝活哥提示保留。OP.GG 解析失败诊断覆盖 props_missing / season_mismatch / total_mismatch / duplicate_champion / opponent_identity，错误不会伪装成 available:true 零场。

### P10 实测表

| Windows 关闭测量 | 改前 3 次 | 改后 3 次 |
|---|---|---|
| 点关闭→本人总览消失 | 未具备 Windows 主机，未测 | 未测 |
| 退出前实际 URI / 最早信号 | 未测 | 未测 |
| LeagueClientUxRender 消失 | 现有扫描不可得 | 未新增未经验证的进程推断 |

Go 假 LCU 已验证 pre-shutdown 立即置 client-exiting、14 秒不恢复 / 15 秒恢复，以及聊天离线与空列表必须在 10 秒内组合；日志不保留未知动态账号、游戏路径参数。

### P13 对照与判定

构建脚本：`scripts/r231-crc-control.cjs`。脚本冻结同一源码快照，两个包仅临时 include 的 CRCCheck off 和相应构建指纹不同；均为 key mode public。默认 `desktop/nsis/installer.nsh` 未关闭 CRC。

关键现状：`installer/update.go` 的正常升级 NSIS 命令已经带 `/NCRC`，因此“关 CRC 就能消除 2.93 秒”目前没有依据。升级下载在 `backend/update_download.go` 中校验清单 SHA-256，失败不会交付安装包。

| Windows 对照 | 第 1 次 nsis_start_to_oninit | 第 2 次 |
|---|---|---|
| 基线包 | 未测 | 未测 |
| CRC-off 包 | 未测 | 未测 |

同一 Windows 主机使用两个 public 包各升级 2 次，保存 update-install-timing / nsis stage 日志，再比较中位耗时。只有符合工单阈值才考虑采用。现在不归因于 Defender 或系统，也不关闭 R212 剩余项。构建产物与 SHA-256 见 `dist/r231-crc-control/receipts.json` / `SHA256SUMS-public.txt`。

### P14 使用点

- bindMatchEntryControls：普通玩家及职业玩家战绩共用，收起删除对应 gameId。
- updateMatchFilter：普通玩家、职业页签、浮层总览共用，切换筛选 clear 全表。
- mountExternalMatchCards.toggleMatch：外部卡片（包括消费该组件的其他视图）收起删除对应 gameId；真实生产组件 Node 测试覆盖“选他人→切概览保留→收起重开回自己”。
- buildSubject / renderBuildPlayers / bindMatchDetailControls：展开期间选择仍保留，切换概览/队伍分析/构建不清除。
- build_player_selection 的 select.is_self 描述被选玩家；reset-collapse / reset-filter.is_self 描述复位后的选择，故为 true。后端只落盘 game_id_hash，不落盘原始 gameId。

## 自动验证与构建收尾

最终 Go 批次开始调度时间：2026-10-06 14:53:31 CST（06:53:31 UTC，命令紧随工具调度启动），对应最后一次 Go 源码/测试修改之后。

| 检查 | 结果 / 耗时 | 日志 |
|---|---|---|
| go test -count=1 ./backend | 通过，112.525 秒 | go-all-final.log |
| go vet ./backend | 通过，退出码 0 | go-vet-final.log（正常无输出） |
| go test -race -count=1 ./backend -run 'Test(R231\|R206Relay\|R208\|SeasonStats\|R86)' | 通过，33.501 秒 | go-race-final.log |
| backend/web Node 全量 | 969 项通过，37368ms；文件并发 2（最终批次 14:58:27 CST 开始调度） | node-all-final.log |
| R231 主页面/浮层/外部视图追加核验 | 7 项通过，8072ms | node-r231-final.log |
| desktop/overview-render.test.cjs | 10 项通过，30508ms | desktop-overview-final.log |
| Worker | 16 项通过，1985ms | worker-final.log |
| installer test / vet | 全模块通过 / vet 退出码 0 | installer-final.log |
| JS syntax / git diff --check | 通过 | syntax-final.log |

Go 全量输出末尾（输出只有一行，按实际记录）：

```text
ok  lol-loot-assistant/backend  112.525s
```

Race 输出末尾（输出只有一行）：

```text
ok  lol-loot-assistant/backend  33.501s
```

两份 public 对照包已按最终源码重建，未发布：

| 包 | 开始 / 完成（CST） | 指纹 | SHA-256 |
|---|---|---|---|
| 0.12.75-r231-baseline-public.exe | 14:50:31 / 14:53:23 | 826f2b2e0588 | e83e67015421a56cac4b7540b4b93da11178a1c5124742585c1275516fadbcd5 |
| 0.12.75-r231-crc-off-public.exe | 14:53:23 / 14:56:00 | 561bd607db34 | d0087970b23e2bdf091133a8cc8e534dde45c49c67d569c5c714e6f5e1b82532 |

两个 NSIS 包均验证实际打包后端与原编译后端相同、fingerprint 正确、key mode public；正常包 111484416 字节、对照包 111482880 字节。根工作区 `desktop/backend/loot-service.exe` 已重建为 0.12.75 public，指纹 826f2b2e0588，并通过无内置 Key 校验。默认 NSIS include 未采用 CRCCheck off。日志保存在 `docs/history/reports/r231/`；Go 测试使用系统默认构建缓存。一次 race 失败由测试夹具后台协程未等退出造成，已补清理等待并重跑；一次 R206 图片启动的 100ms 断言在并行打包高负载下失败，保留失败日志并以 Node 文件并发 2 重跑，没有放宽断言。

截图共深浅两套：table-expanded、table-frame-0～5、headers-1280/780、build-avatar-tooltip、multikills-2-3-4-5、foreign-table-empty/fail、mayhem-rating-ready/failed、200-matches。图标资源在夹具中为本地中性占位图；不冒充真实账号截图。

索引只更新指定工单的状态列。R211 / R214 / R216 / R217 / R218 / R219 / R221 / R223 / R229 依用户本次真机反馈关闭；R230 的 P2/P12 转本单、斗魂 P10 待真机；R224 / R213 / R215 待真机保持；R212 记录 1.88 秒达标与 3.19 秒转 P13。历史账本未改。

## 用户后续 UI 调整：国服合区说明入口

按用户截图，把国服标题右侧的 i 图标移到国服“跟随客户端”一行右侧，独立按钮保留原合区说明悬停提示。修改仅为 index.html / app.css 的布局。

既有搜索/合区提示测试 2 项通过；生产 Chromium 验证图标与跟随客户端同一行、位于按钮右侧、中心偏差 0px，悬停提示包含联盟一区等说明。截图：dark-cn-follow-info.png / dark-cn-follow-info-tooltip.png。

当前 0.12.75 public 后端已重新构建，最新指纹为 **9135083d2c16**，指纹和无内置 Key 校验通过。上面的 R231 全量日志与 CRC 双包仍对应其各自记录的早先源码快照，不包含这次随后提出的图标位置调整；本次补充验证为上述 2 项和 Chromium 检查。

## 遗留点复核：进度刷新节流补测

2026-10-06 15:25:15 CST 开始本轮 Node 全量调度。增加确定性虚拟时钟测试，验证 2999ms 不请求、3000ms 请求，连续进度仅保留一个定时器且采用最新进度，完成事件立即请求并取消等待中的尾部刷新。另把生产函数中的节流窗口由 3_000 突变为 0，确认新测试在“3 秒内不应刷新中间进度”断言失败；未修改生产节流逻辑。

专项 9 项通过（7555ms）；包含国服图标调整及补测的 Node 全量 **971 项通过，35056ms**，见 node-r231-throttle-final.log / node-all-throttle-final.log。本轮没有修改 Go 源码或 Go 测试，既有 Go 全量/race/vet 结论仍对应最后 Go 修改后的快照；当前 0.12.75 public 后端再次构建，运行源码指纹仍为 9135083d2c16，已校验无内置 Key。

真实 q_440 探测、313 场核对、同账号流量与时间、Windows 退出及 CRC 2+2 对比仍未完成，不追加推断数据。若真实 ranked 流 stop_reason=upstream_cap_1000，需要改为每个排位队列独立流后重新核验。

Worker 尚未获得部署授权、未部署。/health 404 可回退原状态路径，但两次请求共享每 origin 的 8 秒预算，因此不能把额外请求对慢链路成功率的影响视为已验证不存在。

本地提交范围为 R231 代码、图标调整、工单/账本与对应证据；排除独立的 queue_groups.go / queue_groups_test.go 斗魂改动以及其他既有未跟踪报告。版本保持 0.12.75。

本轮本地提交同时归档上述最终验证日志的 `.log.gz` 副本；原始 `.log` 保留在本地。未推送或发布。首次暂存遇到稳定一小时的空 Git 锁，确认宿主机无 Git 进程、仅虚拟机只读文件映射持有后清理，提交成功。
