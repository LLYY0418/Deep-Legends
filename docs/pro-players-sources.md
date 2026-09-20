# 职业选手名单与账号来源

人工名单核对日期：**2026-09-08**；R97 范围恢复：**2026-09-16**；R102 账号清单核对与内置：**2026-09-17**。本文件区分人工核对名单与 OP.GG 来源报告目录，不是 Riot 官方实时注册名单，也不承诺覆盖未公开小号。

## R105 当前账号展示契约（0.12.2）

核对表已完整嵌入：**6 队 / 33 人 / 53 账号**，BLG9、IG15、T1 5、HLE8、GEN5、DK11。Wenbo 的 `14小孩幻想赢对线#4453` 已确认；`dyjkbysb#KR1` 属于 Wei，Rookie 的两个新增账号完整保留。

`pro_seed_accounts.go` 保留原始顺序与精确拼写，`pro_reviewed.go` 在每次页面响应时应用清单。旧磁盘快照、来源错误归属、重复 PUUID、后台补充及网络失败均不能删减/覆盖这53账号。仅精确同名同标签的来源记录补充动态字段；未列入清单的历史小号不进入页面。所有已核对账号默认可见，较旧标记不再将其折叠隐藏。“只看最新账号”仍是用户主动筛选。

保留 R104 活跃排序：同一次 OP.GG 目录请求中的 `revision_at` 为来源最近活跃时间，不能宣称为 Riot 精确开局时间；未命中账号才经 Riot match-v5 兜底。已知时间倒序，未知或相同时间按核对表顺序，不按 LP 选主号。无动态数据显示未知，不使用核对文档静态 LP/日期。

R104 配额限制继续有效：启动前60秒无后台 Riot 请求，每分钟最多补1个账号；共享长窗口占用达到30、短窗口达到15或前台排队时让路。OP.GG精确命中不发Riot请求；活动缓存7天、已定级24小时、未定级72小时；总览缓存3分钟不变。启动链不再全量 enrichProActivity。

诊断 `pro_players` 记录与页面一致的公开账号 DTO 和数量，用于核对最终归属；不含 PUUID、锚点、凭据或槽位数据。职业徽章仍沿用独立的严格身份冲突拒绝规则，本轮没有放宽识别边界。

## R97 范围与识别边界（徽章规则继续有效；页面账号集合由 R105 覆盖）

- 选手管理页与战绩详情、当前对局、玩家总览的职业徽章统一仅使用 `proRoster` 的 **BLG / IG / T1 / HLE / GEN / DK 六队已核对一队名单**。不展示额外战队、未核对成员或二队/学院队；历史队标仅接受逐人登记的 AllowTeams，并归入已核对的当前六队。
- `proDirectoryRoster` 的通用扩展能力保留，但不从生产管理页或徽章入口调用；二队 UI 分支保留但不再有生产数据输入。
- 仅按 **PUUID 优先、完整 gameName#tagLine 大小写不敏感精确匹配**反查。绝不按昵称包含、模糊拼写或战队前缀推断归属。冲突连通账号组全部拒绝匹配。
- 识别只读现有进程内快照，不新增目录 HTTP 请求；过期超过 24 小时或无快照则不标。仅韩服；国服无职业提示。
- 出参徽章仅含 playerName/teamCode/teamName/secondary，PUUID 不下发渲染层。maskNames 同时隐藏姓名与职业身份；从对局/战绩点击已识别玩家进入既有职业页签组。
- 既有 R95 样本中的 `Faker#구라티` 是学院队账号，R97 收回范围后不再显示职业徽章，不能误认成 T1 Faker。`Hide on bush#KR1` 的 Faker 正例仍保留；此为隔离 fixture 验证，不声称重新在线核验当前归属。
- BRO 不在六队范围内；Despair 不在当前已核对 DK 名单中，均不识别。不为补齐展示放宽匹配；徽章来源快照可变化；R105 页面账号数固定为核对表的53个。

## 已独立核对的一队名单（管理页与徽章共用白名单）

| 赛区 | 一队 | 已核对名单 |
| --- | --- | --- |
| LPL | BLG | Bin、Wenbo、Flandre、Xun、knight、Viper、ON |
| LPL | IG | TheShy、Wei、Rookie、Assum、JiaQi、Meiko |
| LCK | T1 | Doran、Oner、Faker、Peyz、Keria |
| LCK | HLE | Zeus、Kanavi、Zeka、Gumayusi、Delight |
| LCK | GEN | Kiin、Canyon、Chovy、Ruler、Duro |
| LCK | DK | Siwoo、Lucid、ShowMaker、Smash、Career |

一队确证替补仍保留，不把同一位置的第二位选手当作二队。二队识别使用来源队伍 ID、Academy / Challengers / Junior 等明确标识及已核对身份白名单，**不使用名字长度**。OP.GG 中的历史队员、教练以及旧队标不能直接决定名单。

### 名单依据

- BLG 第三赛段公告（2026-07-21）：[官方微博](https://weibo.com/5926660141/5323167086416657)，[公告转述](https://m.zhibo8.com/news/web/game/2026-07-21/6a5f3932525a7native.htm)。该阶段包含 Bin、Wenbo、Xun、knight、Viper、ON；Wenbo 是杨文波，不是 JackeyLove（喻文波）。微博公开抓取不稳定，正文交叉参考媒体转述。
- Flandre 加入 BLG（2026-08-09）：[PP体育公告转述](https://www.ppsports.com/360news/news/2591536.html)、[同期阵容汇总](https://m.zhibo8.com/news/web/other/2026-08-09/6a783fbb02d94native.htm)。加入公告补充在第三赛段原名单之上，不能只取旧表遗漏 Flandre。
- IG 第三赛段公告（2026-07-22）：[公告转述](https://news.zhibo8.com/game/2026-07-22/6a60504343e65native.htm)。明确列出 TheShy、Wei、Rookie、Assum、JiaQi、Meiko，且与教练人员分列；Assum 与 JiaQi 都保留。
- LCK：[2026 年度奖项公告](https://lolesports.com/ko-KR/news/2026-08-lck-awards)用于核对近期一队身份，**不是完整名单**；[Gen.G 官方阵容](https://geng.gg/pages/league-of-legends)单独区分 LCK / Challengers，并确认 Duro 为 Joo Min-kyu；[T1 官方队页](https://www.t1.gg/teams/leagueoflegends)、[HLE 官方队页](https://hle.kr/en)与[2026 赛季阶段阵容](https://draftcoach.gg/wiki/LCK/2026_Season/)交叉参考。部分官方页面姓名使用图片，不能将文本未出现解释为离队。

### 不确定项及排除边界

- **Wenbo**：2026-09-17 核对材料中 Leaguepedia 与 dpm.lol 独立把 `14小孩幻想赢对线#4453` 关联到杨文波 / BLG / 上单 / 2007-04-01，已具备交叉旁证。用户已通过 OP.GG 小程序最终确认，R102 已内置该账号；见下方最终清单。
- **Yxl**：外部百科/旧队员标签与近期 BLG 第三赛段名单不一致，尚无足够证据确认是当前一队替补，未纳入人工核对表；生产目录与徽章均不纳入，也不据此武断认定他必然是二队。
- **Painter**：非官方阶段表出现过 T1 轮换记录，但尚未确认其当前一队注册状态与二队归属变化。一场历史临时征召不能自动证明当前长期一队身份，现暂不纳入人工核对表；生产目录与徽章均不纳入。检索到的具体比赛页未能在直接复核时正常返回比赛内容，不能作为确证。
- **Bluffing、Sharvel**：旧阶段轮换/青训记录不足以证明当前属于 HLE、DK 一队，未纳入人工核对表；生产目录与徽章均不纳入。

以上待核验项意味着不能声称“官方注册成员绝无遗漏”。后续应以新的官方注册表/俱乐部公告更新白名单，而不是扩大模糊匹配。

## 账号与段位

### 主要来源：OP.GG

固定读取 [韩服职业选手目录](https://op.gg/zh-cn/lol/spectate/list/pro-gamer?region=kr)。`region=kr` 必须显式指定，裸页面可能默认到其他服务器。

解析完整 Next Flight 记录，而不是首屏或前 N 条可见 DOM。遍历整个 KR 目录。人工核对表中的成员仍按选手名称（忽略大小写）**与真实姓名同时匹配**，跨旧队标仅允许逐人显式登记 AllowTeams；其它来源成员不进入生产目录或徽章索引。账号级 `region` 为 null、缺失、空字符串时，沿用外层明确 KR 目录的来源证明；只有明确标注非 KR 的账号才拒绝。该规则由真实 Flight 结构的空 region 回归和强制非空变异测试保护。`knight` / `Knight` 合并；不能把账号昵称 `BLG 온` 误认成 ON。

PUUID 与完整 Riot ID 在后端用于联合去重和跨归属冲突排除；R100 将公开来源目录快照（含这些公开标识）保存在本机私有 pro-directory 缓存中，最长24小时，重启恢复后仍只在后端建索引，渲染页不会收到 PUUID。段位取来源的当前单双排字段，不取历史赛季、近期比赛平均段位。空字段显示“段位暂不可用”，只有明确 `UNRANKED` 才显示“未定级”。

### 补充来源：TrackingThePros

OP.GG 缺少的三位已核对选手，动态读取以下公开实名身份页：

- [TheShy](https://www.trackingthepros.com/player/TheShy)：Kang Seung-lok / 강승록。
- [Assum](https://www.trackingthepros.com/player/Assum)：Zou Wei / 邹维。来源仍可能显示旧俱乐部，以白名单决定当前分组。
- [JiaQi](https://www.trackingthepros.com/player/JiaQi)：Zi Jia-Qi / 资嘉琪。

只允许以上三个已审查 URL。解析姓名与选手昵称双重校验；包含 HTML 中默认隐藏的“不活跃”账号，排除非 KR 行。界面标记 `TTP` 和“不活跃”。动态段位从来源读取，**不硬编码胜点**；R105 页面账号身份以人工核对表为准。来源没给 LP / 更新时间时展示“—” / “更新时间未知”，不编造 0 LP 或把抓取时间当账号更新时间。

2026-09-08 本次真实接口验收：OP.GG 76 个账号 + 补充来源 14 个（TheShy 9 / Assum 1 / JiaQi 4），共 **33 位选手、90 个公开账号，1 位缺少账号（Wenbo）**。这是 2026-09-08 的历史快照，不是当前缺号结论；Wenbo 已由用户核对并在 R102 入库。R105 页面只展示核对表53个账号，不将历史90个与种子数相加。

## 网络、缓存和失败

- 复用项目设置的公网传输/代理，不携带 Cookie、Riot Key、本地客户端令牌。只访问固定 HTTPS 来源，禁止跨域、换端口和越界路径重定向；不扩展通用资源白名单。
- R100：进程内5分钟TTL、30秒刷新/失败退避及共享singleflight保留；发布的目录/补充状态/ladder排名另有有界24小时磁盘快照，重启可直接恢复，读取不延长原始抓取时间。ladder复用provider缓存层24小时策略并保留无cookie/固定URL重定向校验；主目录12秒、补充页各7秒，后台补充总上下文50秒。
- 主目录失败最多保留 24 小时缓存，明确显示过期提示；超过时仍显示人工核对账号，动态字段显示未知。
- 某补充页失败不清掉其有效旧账号：最多保留该身份最后成功快照 24 小时，逐账号标记“缓存”，不更新其成功时间，顶层 `partial` 与页面“部分来源未更新”明确区分主目录新鲜度，并提示来源待核验。超过时保留人工核对账号，失效动态字段回到未知。
- 名单核对超过 30 天会提示需要重新核对。页面不是实时段位榜；未知小号、来源未更新及身份冲突均不能默默补造。
- 点击账号以完整 Riot ID、固定 `kr` 进入既有 Riot 官方总览管线；不会将来源 PUUID 注册为永久会话引用。列表无需连接国服客户端，实际总览仍受既有 Riot 数据源配置/额度限制。

## 维护与验证

修改 `pro_roster.go` 的人名、别名或队伍时，应先记录官方/可核对来源及日期，并同步核验二队/替补身份。添加补充身份页必须审查 URL、实名与页面结构，不接受任意 URL 参数。

```bash
go test -run '^TestPro' .
node --test desktop/pro-players.test.cjs
go test ./...
node --test desktop/*.test.cjs
```

可选本地真实 HTML 回归：`PRO_PLAYERS_CAPTURE` 指向 KR 目录 HTML，`PRO_PLAYERS_SUPPLEMENT_CAPTURE` 指向包含 `pro-ttp-theshy.html`、`pro-ttp-assum.html`、`pro-ttp-jiaqi.html` 的本地目录。原始响应和稳定账号 ID 不提交仓库。使用 `PRO_PLAYERS_PUBLIC_OUTPUT` 时仅输出脱敏公开 DTO。


## R73 离线名单分歧复核（2026-09-08）

- **DK**：Riot 官方 [2026 LCK Awards（2026-08-27）](https://lolesports.com/ko-KR/news/2026-08-lck-awards)分别列出 DK Smash、Lucid、Siwoo、ShowMaker、Career 及实名，支持现有五人。没有当前一队官方依据支持以 Wayne/BeryL 替换，保留现有名单；这份奖项公告不是完整注册名单。
- **Duro**：[Gen.G 官方阵容](https://geng.gg/pages/league-of-legends)的 LCK 部分明确列出 Duro / Joo Min-kyu / SUP，与 Challengers 分离。离线对照表缺少他不能作为删人依据。
- **Wenbo（已核对入库）**：保留原公告支持的一队行；R101 核对材料已提供完整韩服账号双源身份旁证，见 [2026-09-17 核对清单](pro-accounts-verification-2026-09-17.md)。当前持续注册情况仍不以第三方账号站替代官方资料；用户已通过 OP.GG 小程序确认账号，R102 已内置 `14小孩幻想赢对线#4453`。
- **Assum（未核实项）**：保留既有第三赛段名单依据，本轮没有读取到新的官方实名注册表，不能宣称已重新官方确证，更不能静默删除。
- **Dandy（未核实项）**：没有本轮可读取的官方一队选手证明，离线材料中的未知位置代码不足以认定选手，继续不纳入，不推测该代码含义。

账号时效：来源记录超过120天或明确 inactive 可标记较旧，但 R105 已核对账号默认完整显示；缺失/非法时间不推断不活跃。当前页面排序和显示契约见 R105 节。职业徽章身份图仍排除跨归属冲突。

点击职业账号时才携带列表段位 expectedTier，使用既有韩服总览请求对比真实单双排段位；相差至少三个大段或账号 404 时在本次页面会话标记待核验。缺失段位不等于不一致。比对结果不写共享总览缓存、不存本地存储、不增加批量 Riot 请求。含空格或连字符的名字不查询公开天梯，避免 HTML 行标识歧义；未知排名保留破折号。

已审查的旧队标例外（2026-09-08）：IG Rookie 的 OP.GG 实名记录仍在 NIP（858），IG Meiko 仍在 TES（415）；以既有 IG 第三赛段名单公告为当前归属依据，仅这两人的 AllowTeams 分别登记上述 ID。昵称、实名和来源 member.team_id 自洽检查仍然全部执行，不向其他选手或旧队开放。

2026-09-08 后续纠正：TTP TheShy 页面账号表提供段位和部分 inactive_account 标记，但没有逐账号更新时间。旧实现把所有无时间的 TTP 账号视为 dormant 是错误的，已修复；不会把目录抓取时间、页面版权年份或空时间当作最后对局时间。页面取消来源提示条，失败/缓存过期仍由标题状态和对应账号元数据表达。

## R75 对局页职业符文（2026-09-09）

本节只说明公开比赛数据，独立于上文韩服账号目录。名字白名单仍来自 `pro_roster.go` 的已审查一队名单，不采用 getTeams.players（含测试占位账号）。比赛方按 livestats 的 esportsTeamId；初次将去队伍前缀的 summonerName 与名单忽略大小写完全匹配，再持久化 esportsPlayerId ↔ 队伍/名单选手。任何队伍变化仍需通过当前名单，不通过昵称包含关系扩充。

现场 getLeagues/getTeams 核对后的 leagueId：LPL 98767991314006698；LCK 98767991310872058；First Stand 113464388705111224；MSI 98767991325878492；Worlds 98767975604431411。
队伍 ID：BLG 99566404853854212；IG 99566404848691211；T1 98767991853197861；HLE 100205573496804586；GEN 100205573495116443；DK 100725845018863243。DK Challengers 105550026570060790 和 LCK Challengers 98767991335774713 均拒绝，code 只用于显示。

公开只读链路：getSchedule → getEventDetails → window → 对齐终局 window → details。赛程不提供 league/team ID，所以先读详情再按 ID 过滤，禁止用缩写抢先过滤。对局开始时间 UTC 近 30 天；终局出装仅为最终背包。8 项去重 perks 不补默认碎片。胜负不唯一时中性展示且不进分母；少于 3 场不展示百分比。

复跑脚本 `scripts/r75-live-probe.py`、正式 provider 的 opt-in 集成 `pro_runes_live_test.go` 与真实改坏还原脚本 `scripts/r75-mutations.py` 可追溯；浓缩官方 fixture 在 `testdata/r75/official-series.json`，完整原始响应仅保留在临时取证目录。该公开缓存不会复用本机账号令牌、Cookie、Riot Key、PUUID，也不改变上文账号查询的隐私边界。

## R99 人工核对种子账号（2026-09-16，历史记录）

R99 当时仅对 **IG/Rookie** 增加 `모든일은같이#KR1`，不向其他队伍或选手开放。以下是工单已完成的人工核实记录，本轮不重新宣称完成在线归属核实：

- [Leaguepedia Rookie](https://lol.fandom.com/wiki/Rookie) 的 Soloqueue IDs 包含该账号；局限是缓存较旧、无法确认最后编辑日期。
- [dpm.lol Rookie](https://dpm.lol/pro/Rookie) 将账号归于 IG、中单、PRO，工单记录最近对局为 2026-09-16。OP.GG 段位/胜负可交叉核对同一账号，但 OP.GG 召唤师页没有 Rookie 或职业战队标签，现有目录无法自行发现它。
- 两站给出的稳定标识相互矛盾，因此一律不抄写，源码只登记 Riot ID；通过自身 Riot 官方 by-riot-id 解析，后续 by-puuid 反查当前名字。归属置信度为**较可信，不是确证**，没有选手或战队一手声明。

复用 `riot-identities` 目录与缓存封装：`proseed:v1:IG/rookie` 的锚点 30 天，`account-by-puuid:` 的反查结果 6 小时，现有 `account:` 15 分钟不变。两层哈希后的锚点文件带非标识性 `proseed-` 前缀，并在实际 strict LRU 与通用淘汰中受保护；仍计入磁盘总预算，其余身份项正常淘汰。文件为 0600，正文不是加密文件。该段为 R99 当时设计；R100 已增加有界私有目录快照，R102 的锚点键和预算见下节。

种子解析只在目录取数流水线发生，携带 100ms 队列预算与 3 秒请求预算；额度不足当轮沿用上次种子快照，不从身份徽章识别路径发请求。三个发布阶段注入同一份结果，避免加载中途消失。重命名诊断仅记选手名和 changed=true。来源显示“人工核对”；有官方解析 PUUID 的种子不适用既有“无 PUUID 为低置信度”规则，该技术去重置信度不提升上述归属事实的证据等级。


## R101（2026-09-17）段位与候选账号（历史记录）

R101 当时唯一内置的 Rookie 种子在完成官方身份解析后，按 PUUID 向 Riot league-v4 读取当前单双排；精准选择 `RANKED_SOLO_5x5`，罗马小段转换为数字，大师及以上小段为 1。种子段位使用独立 6 小时缓存，总览的 3 分钟缓存不变。无单双排记录保留 unavailable，额度/网络失败保留上次种子快照；不把旁证网站的 LP 占位值写成实际段位。天梯名次继续显示未知。

R101 当时只更新候选材料，没有扩大种子。[33 人核对清单](pro-accounts-verification-2026-09-17.md)已在 R102 前由用户逐项核对定稿，旧的九项待裁决状态不再适用。


## R102 最近对局排序与全量内置（2026-09-17，历史设计；预算及 TTL 已由 R104/R105 覆盖）

以用户最终确认的[账号清单](pro-accounts-verification-2026-09-17.md)为唯一录入依据，内置 33 位选手、53 个账号：BLG 7 人/9 号、IG 6/15、T1 5/5、HLE 5/8、GEN 5/5、DK 5/11。姓名、位置和队伍 ID 复用 `pro_roster.go`；`dyjkbysb#KR1` 只归 Wei。名单审核日期与账号核对日期分别维护，不用账号核对日期伪造新的队伍注册核验。

最近活动来自 Riot match-v5：先取 `start=0,count=1` 的对局 ID，不加 queue/type，再读取 `gameStartTimestamp` 毫秒 UTC 时间。`gameCreation`、来源 UpdatedAt 和清单中的静态日期/LP 都不参与活动排序。空列表或没有开始时间为未知；失败保留该账号上次成功的活动时间，冷启动失败保留人工账号行且不制造时间。公开 JSON 只增加 `lastMatchAt`、`lastMatchAtKnown`；PUUID 和锚点继续留在后端私有缓存。

目录首先发布已核对账号和旧快照，随后在后台刷新官方身份、段位与活动。每个选手预算为账号数 × 4 秒，每个账号有独立 4 秒子预算；R100 的 100ms 单次队列准入限制继续生效，不增加并发，不阻塞首次目录响应。某账号失败只回退该账号，不清空其他账号。带 PUUID 的已审核上游账号也在后台补活动，渲染和身份徽章读取不会触发网络。

缓存彼此独立：索引锚点 `proseed:v1:<team>/<player>/<index>` 保留 30 天，录入顺序不得因改名调整；官方名字反查 6 小时；`proseed-lastmatch:v1:<puuid>` 活动时间 6 小时；`proseed-rank:v1:<puuid>` 已定级段位 6 小时，成功但无单双排记录退避 72 小时，失败不写入退避缓存。总览 `ranks:` 仍为 3 分钟，其成功响应若观察到真实单双排段位，会立即结束未定级退避并按 6 小时重新计时。确认未定级只改变查询频率，公开无段位仍沿用 unavailable；活动查询保持 6 小时。种子天梯名次继续未知。

身份磁盘缓存上限由 256 扩到 1024 条，字节上限仍为 4 MiB，普通项继续 LRU，锚点仍受保护并计入预算。53 个账号的锚点、名字正反查、段位、活动与最新对局 ID 会占多个独立键，扩容用于避免一次刷新就挤出仍在 TTL 内的结果。重命名诊断只记 event/changed，不写选手名、Riot ID 或 PUUID。

**R102 历史活动查询估算（不再代表当前 R104/R105 调度）**：33 人 × 平均 53/33 个账号 × 每 6 小时 2 次请求 = **106 次/6 小时，最多 424 次/天**。这是 53 个种子均有对局且每轮详情都未缓存时的估算；空列表只需一次、永久详情命中和重复 PUUID 会减少调用。身份解析、段位查询及额外上游账号不包含在这 424 次中。配额不足会延后实际查询，本轮未用真实 Key 测量耗时或配额。
