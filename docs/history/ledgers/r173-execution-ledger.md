# R173 执行账本

日期：2026-09-30。基线 0.12.37；本轮源码版本 **0.12.38**。

## P1：符文对局记录的装备行

- 绝活哥：内部保留 matchId / participantId 以定位时间线，这些字段不进入前端 JSON。复用抽出的 participantTimelineRecords，开局 90 秒内净购买抵消撤销和出售、排除饰品、保留重复药水，出门装独立最多 6 格。现有按分钟装备路线继续使用同一解析器。
- 符文列表仍先返回，前端渲染当前玩家的对局行后才 POST /api/gameplay/rune-starters；只传当前可见行的 key 和 playedAt，不接受任意时间线地址或账号标识。绝活哥当前玩家最多 3 场，切换玩家后补该玩家，上限为 3 玩家 × 3 场的独立 9 场预算；不占原 36 次符文列表预算。2 个时间线 worker 走 Riot 共享限流器后台优先级。
- 绝活哥缓存按 matchId 保存派生的参赛者槽位 → 出门装物品列表，不保存完整时间线；rune-starters 有界磁盘缓存 600 条 / 128 MiB、TTL 一年、失败冷却 90 秒。重启后同场对局命中磁盘。实际内容已登记隐私存储声明和目录覆盖表。
- 职业选手：proRuneIndexGame.Start 已由 window.Frames[0].Timestamp 创建（pro_runes.go 的 indexGame），不是赛事日程时间。本轮暂用首帧 + 60 秒并沿用接口的 10 秒对齐，取返回帧中该参赛者首个非饰品非空库存，独立最多 6 格。60 秒仍是待用户日志核对的暂定值。
- 职业开局缓存为独立 details-start-<gameId>，复用原职业赛事磁盘缓存目录；帧非空、时间不早于首帧、每帧参赛者属于本局且不重复。终局 details-<gameId> 缓存仍使用原终局时间校验与裁剪，互不污染。
- 职业来源只补当前选手实际展示的最多 5 场。后台索引、终局验证和开局请求共同通过 3 槽后台门，再通过原 4 槽总门，保留一个前台名额，避免两类后台 worker 合计占满总门。
- 有出门装时原行变为「装备」：出门装 → R172 同款 3px route-divider → 原终局最多 7 格。无出门装时输出与原「最终装备」行逐字一致。只替换装备子节点，保留对局行、顺序、选中状态；再次刷新职业符文列表时保留同 key / playedAt 的已完成补充，避免退回旧行。
- 失败、空事件与缺失数据只保留现有终局装备；不增加占位图标、说明文字或 tooltip。OPGG 符文来源不接入。R172 的核心装路线前缀未改。

## P2：职业原始字段形状诊断

终局与开局 details 的网络响应均在原始 JSON 上、常规类型解析前记录 pro_details_shape，每个 gameId / kind 进程内去重；只输出 kind、帧数、首末帧相对 window 首帧的秒数、participants 的安全键名集合。候选技能键名保留，其他键名合并为 <unknown-key>；不输出参赛者 ID、对局 ID、队伍 ID、物品 ID、技能值或原始响应。开局另外输出请求偏移、选中帧偏移和非饰品物品个数；物品数量按完整库存计数，显示仍限 6 格。

已有解析磁盘缓存不具备原始字段证据，缓存命中不会伪造“没有技能字段”的诊断。本轮首次请求新增开局缓存时可取得原始 start 形状；end 形状在真实终局缓存未命中时取得。职业技能继续留空；必须等待用户原始形状日志后，才能确认字段有无并决定后续解析工单。

## P3：启动仅读缓存

main 启动入口改为 warmProPlayersCaches：读取职业目录快照和 pro-profiles 的 7 天 StaleUntil 缓存，保留原 CheckedAt / fetchedAt，不伪造新鲜度，也不启动 enrichProProfiles 或 Riot 种子刷新。无磁盘目录时使用人工审核名单，53 个账号原大小写、顺序和归属不变。

依赖核对：对局职业身份识别 proIdentitySnapshot 读取 proPlayers.teams，未自行发起网络；因此预热发布缓存目录或人工名单即可满足离线身份识别，不需要整体刷新 53 个资料页。账号稳定锚点网络刷新仍由实际打开职业页后的原加载路径触发。本轮没有改职业页 15 分钟资料 TTL、未定级 / 失败回退规则或 refresh=1 语义。新增 pro_startup_cache 只记恢复条数和 network_requests=0。

## 测试与变异证据

新增 8 个 Go 专项测试覆盖开局边界、净购买 / 撤销出售 / 重复物品 / 6 格上限、独立预算、时间线挂起时符文列表不阻塞、同场重启磁盘复用、第三帧取装备、两种职业缓存隔离、参赛者校验、原始字段隐私 / 去重、后台门保留前台、启动网络 0 次而打开页面后刷新。6 个 Node 专项测试覆盖严格 DOM 顺序、逐字兼容、原位补充、可见行请求上限、失败 / 过期响应以及职业列表刷新保留补充。

变异使用 /private/tmp/r173-mutations 下 Go overlay，未改仓库源码。以下 6 项全部得到测试断言 FAIL，均非编译错误：

| 变异 | 捕获证据 |
|---|---|
| 90 秒阈值放开为全部购买 | 出门装混入 1001 / 3006，净购买断言失败 |
| 去掉 ITEM_UNDO 抵消 | 多出一次 1055，净购买断言失败 |
| 不排除饰品 | 多出 3340，净购买断言失败 |
| 时间线挪进同步符文路径 | rune list waited for opening timeline |
| 原始取值写入形状诊断 | raw value leaked: 1055 |
| 启动预热恢复 force=true 网络加载 | startup downloaded profiles 1 |

全量 Go 初次运行仅发现新缓存目录缺隐私登记；已补实际声明与覆盖表，再跑 go test ./backend -count=1 **通过，193.960 秒**。go vet ./backend 通过。前端全量 node --test backend/web/*.test.cjs **745 / 745 通过**。git diff --check 通过。

## 构建验证

**key mode: public**：main.riotAPIKey 和 main.riotAPIKeyCipher 两个嵌入字段均置空。构建版本 0.12.38，最终源码指纹 c07a579e9c89。

- macOS arm64：/private/tmp/Deep-Legends-backend-0.12.38-public
- Windows amd64：/private/tmp/Deep-Legends-backend-0.12.38-public.exe

两平台 go build 均通过；两份产物的指纹均校验为 c07a579e9c89，与收尾重新计算的源码指纹一致。macOS 在 LOL_LOOT_DATA_DIR=/private/tmp/r173-selftest、空 RIOT_API_KEY 下执行 -self-test，输出 Deep Legends 0.12.38 自检通过（奖池 554 条）。仅后端验证构建，未制作或发布安装包，用户后续按原流程打包。

## 0930 工单中的其余观察

以下是本工单提供的真机日志判读和用户反馈，**并非本轮重新读取原始 jsonl 或运行 Windows 得到的证据**：

- R171：两次 perk_apply_attempt 成功、spell_requested / spell_applied 均为 true，两次客户端技能 PATCH 204；OPGG / 绝活哥部分真机通过。职业技能仍待 P2 原始字段确认。
- R169：直连清单超时后镜像成功两次，真实公开清单校验通过。公开最新 0.12.19 低于用户本机 0.12.37，提示已最新正常；让其他用户升级仍需发布新版 public，本轮不发布。
- R168 P2：5 条 lane_matchup_candidate_fetch 均 context-unavailable，实际为队列 3110 练习模式、敌方电脑无分路；卡片缺席符合现有逻辑。R167 / R168 / R170 的排位真机验证仍待。
- R166：用户确认雷达差距可见；日志未打开斗魂 / 海斗详情，首次加载速度尚未真机覆盖。
- 生涯 / 总览背景图本地 splash 6 次 400，gtimg 回退均成功：低优先级，本单不改。
- pro_identity_match 高调用频率已有去重，正常，未改。
- sgp_ranked_stats_incomplete 72 次以及请求 30 / 返回 20 的降级属于已有现象，未改。

## 用户真机验收待办

1. 选人后打开绝活哥、职业选手符文来源，检查每条对局底部出门装 → 明显竖线 → 终局装备；切换选手后补充仍不改变选中记录和顺序。
2. 冷启动停在总览 2 分钟后导出日志，确认不再出现整批 1 MB 级职业资料页请求；打开职业页后按原规则刷新。
3. 再导出含职业来源的日志，依据 pro_details_shape 核对开局偏移 / 物品数量及召唤师技能候选键；本轮夹具不作为上游字段证据。
4. 其余 R167 / R168 / R170 排位验证、R166 斗魂 / 海斗详情首载验证继续保持待真机状态。
