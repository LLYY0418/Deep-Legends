# 职业选手名单与账号来源

核对日期：**2026-09-08**。本文件是身份白名单的维护依据，不是 Riot 官方实时注册名单，也不承诺覆盖未公开小号。

## 当前收录范围

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

- **Wenbo**：保留选手行，目前没有可交叉确认归属的韩服账号。类似名字或可查到的召唤师页面不等于归属证明，不加入推测账号。
- **Yxl**：外部百科/旧队员标签与近期 BLG 第三赛段名单不一致，尚无足够证据确认是当前一队替补，未纳入；也不据此武断认定他必然是二队。
- **Painter**：非官方阶段表出现过 T1 轮换记录，但尚未确认其当前一队注册状态与二队归属变化。一场历史临时征召不能自动证明当前长期一队身份，现暂不纳入。检索到的具体比赛页未能在直接复核时正常返回比赛内容，不能作为确证。
- **Bluffing、Sharvel**：旧阶段轮换/青训记录不足以证明当前属于 HLE、DK 一队，未纳入。

以上待核验项意味着不能声称“官方注册成员绝无遗漏”。后续应以新的官方注册表/俱乐部公告更新白名单，而不是扩大模糊匹配。

## 账号与段位

### 主要来源：OP.GG

固定读取 [韩服职业选手目录](https://op.gg/zh-cn/lol/spectate/list/pro-gamer?region=kr)。`region=kr` 必须显式指定，裸页面可能默认到其他服务器。

解析完整 Next Flight 记录，而不是首屏或前 N 条可见 DOM。遍历整个 KR 目录，按选手 ID 名称（忽略大小写）**与真实姓名同时匹配**白名单，上游队伍 ID 必须等于当前名单的 OPGGID，跨旧队标仅允许逐人显式登记 AllowTeams；未登记的别队同名实名记录拒收并标 partial。`knight` / `Knight` 合并；不能把账号昵称 `BLG 온` 误认成 ON。

PUUID 与完整 Riot ID 仅在后端内存中用于联合去重和跨归属冲突排除；渲染页不会收到 PUUID。段位取来源的当前单双排字段，不取历史赛季、近期比赛平均段位。空字段显示“段位暂不可用”，只有明确 `UNRANKED` 才显示“未定级”。

### 补充来源：TrackingThePros

OP.GG 缺少的三位已核对选手，动态读取以下公开实名身份页：

- [TheShy](https://www.trackingthepros.com/player/TheShy)：Kang Seung-lok / 강승록。
- [Assum](https://www.trackingthepros.com/player/Assum)：Zou Wei / 邹维。来源仍可能显示旧俱乐部，以白名单决定当前分组。
- [JiaQi](https://www.trackingthepros.com/player/JiaQi)：Zi Jia-Qi / 资嘉琪。

只允许以上三个已审查 URL。解析姓名与选手昵称双重校验；包含 HTML 中默认隐藏的“不活跃”账号，排除非 KR 行。界面标记 `TTP` 和“不活跃”。账号及段位每次从来源读取，**没有硬编码真实账号和胜点**。来源没给 LP / 更新时间时展示“—” / “更新时间未知”，不编造 0 LP 或把抓取时间当账号更新时间。

2026-09-08 本次真实接口验收：OP.GG 76 个账号 + 补充来源 14 个（TheShy 9 / Assum 1 / JiaQi 4），共 **33 位选手、90 个公开账号，1 位缺少账号（Wenbo）**。这是当次公开来源快照，不是固定验收常量或永久数量。

## 网络、缓存和失败

- 复用项目设置的公网传输/代理，不携带 Cookie、Riot Key、本地客户端令牌。只访问固定 HTTPS 来源，禁止跨域、换端口和越界路径重定向；不扩展通用资源白名单。
- 仅进程内缓存，5 分钟 TTL、30 秒刷新/失败退避、共享 singleflight；整次读取超时 18 秒，补充页各 7 秒。
- 主目录失败最多保留 24 小时缓存，明确显示过期提示；超过时保留名单空态，不假装没有选手。
- 某补充页失败不清掉其有效旧账号：最多保留该身份最后成功快照 24 小时，逐账号标记“缓存”，不更新其成功时间，顶层 `partial` 与页面“部分来源未更新”明确区分主目录新鲜度，并提示来源待核验。超过时回到缺失状态。
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
- **Wenbo（未核实项）**：保留原公告支持的一队行；当前持续注册情况、韩服账号归属仍待新的官方资料复核，不用同名推断账号。
- **Assum（未核实项）**：保留既有第三赛段名单依据，本轮没有读取到新的官方实名注册表，不能宣称已重新官方确证，更不能静默删除。
- **Dandy（未核实项）**：没有本轮可读取的官方一队选手证明，离线材料中的未知位置代码不足以认定选手，继续不纳入，不推测该代码含义。

账号时效：仅来源记录时间明确超过 120 天，或来源明确标记 inactive 时折叠为较旧/来源标记不活跃账号；缺失或非法时间保持未知并直接展示。来源更新时间不是最后游戏时间，不能推断账号 120 天没有玩。排序为非历史优先，然后段位、小段、胜点及名称；第一条非历史且有明确段位的账号标为主号（算法展示语义，不是选手官方认证）。无 PUUID 的来源记录为低置信度；身份图以 PUUID 为主锚点，完整 Riot ID 为次键，仍排除跨归属冲突。

点击职业账号时才携带列表段位 expectedTier，使用既有韩服总览请求对比真实单双排段位；相差至少三个大段或账号 404 时在本次页面会话标记待核验。缺失段位不等于不一致。比对结果不写共享总览缓存、不存本地存储、不增加批量 Riot 请求。含空格或连字符的名字不查询公开天梯，避免 HTML 行标识歧义；未知排名保留破折号。

已审查的旧队标例外（2026-09-08）：IG Rookie 的 OP.GG 实名记录仍在 NIP（858），IG Meiko 仍在 TES（415）；以既有 IG 第三赛段名单公告为当前归属依据，仅这两人的 AllowTeams 分别登记上述 ID。昵称、实名和来源 member.team_id 自洽检查仍然全部执行，不向其他选手或旧队开放。

2026-09-08 后续纠正：TTP TheShy 页面账号表提供段位和部分 inactive_account 标记，但没有逐账号更新时间。旧实现把所有无时间的 TTP 账号视为 dormant 是错误的，已修复；不会把目录抓取时间、页面版权年份或空时间当作最后对局时间。页面取消来源提示条，失败/缓存过期仍由标题状态和对应账号元数据表达。

## R75 对局页职业符文（2026-09-09）

本节只说明公开比赛数据，独立于上文韩服账号目录。名字白名单仍来自 `pro_roster.go` 的已审查一队名单，不采用 getTeams.players（含测试占位账号）。比赛方按 livestats 的 esportsTeamId；初次将去队伍前缀的 summonerName 与名单忽略大小写完全匹配，再持久化 esportsPlayerId ↔ 队伍/名单选手。任何队伍变化仍需通过当前名单，不通过昵称包含关系扩充。

现场 getLeagues/getTeams 核对后的 leagueId：LPL 98767991314006698；LCK 98767991310872058；First Stand 113464388705111224；MSI 98767991325878492；Worlds 98767975604431411。
队伍 ID：BLG 99566404853854212；IG 99566404848691211；T1 98767991853197861；HLE 100205573496804586；GEN 100205573495116443；DK 100725845018863243。DK Challengers 105550026570060790 和 LCK Challengers 98767991335774713 均拒绝，code 只用于显示。

公开只读链路：getSchedule → getEventDetails → window → 对齐终局 window → details。赛程不提供 league/team ID，所以先读详情再按 ID 过滤，禁止用缩写抢先过滤。对局开始时间 UTC 近 30 天；终局出装仅为最终背包。8 项去重 perks 不补默认碎片。胜负不唯一时中性展示且不进分母；少于 3 场不展示百分比。

复跑脚本 `scripts/r75-live-probe.py`、正式 provider 的 opt-in 集成 `pro_runes_live_test.go` 与真实改坏还原脚本 `scripts/r75-mutations.py` 可追溯；浓缩官方 fixture 在 `testdata/r75/official-series.json`，完整原始响应仅保留在临时取证目录。该公开缓存不会复用本机账号令牌、Cookie、Riot Key、PUUID，也不改变上文账号查询的隐私边界。
