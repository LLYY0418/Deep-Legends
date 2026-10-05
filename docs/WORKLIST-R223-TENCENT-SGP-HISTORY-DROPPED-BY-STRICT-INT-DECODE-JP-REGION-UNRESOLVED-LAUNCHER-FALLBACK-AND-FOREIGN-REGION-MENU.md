# WORKLIST-R223：国服战绩全空（新加的击杀/控制等字段解析失败，整场被丢）；日服大区识别失败导致总览战绩读不出；登录入口点「否」仍去直接启动客户端、启动后入口被收起；外服搜索菜单和分组调整

诊断：Claude（4 张截图 + 日志 `lol-loot-diagnostics-1005-1956.jsonl` + 只读核对源码）。执行：GPT。日期：2026-10-05。
基线：**0.12.73（已公开发布）**，指纹 `7c34391c96f6`，`run_id d71f956ff473eb8f9278df82`（北京时间 19:39～19:56）。对比基线：同一份日志里 0.12.71 的运行 `f29b9fbb64a46c9d2c0e49d3`。

> **P1 是线上事故：0.12.73 下所有国服用户的总览战绩、赛季统计、对局玩家战绩都会是空的。** 建议 P1 修完单独出补丁版本，不等其他 P。是否发布由用户决定。

---

## 现象（对应截图）

1. 图一：日服账号登录后，自己的页签被归到「国服」分组，头像旁显示「国服」，右侧「战绩暂时无法读取」，左侧「近 0 场排位」。
2. 图二：点「Riot 客户端」入口后弹出系统窗口「无法直接启动游戏：我们无法直接启动游戏。请通过恰当的启动器来开始游戏。」，入口按钮被收起，只剩「重新选择入口」。
3. 点「国服纯净入口」，Windows 弹出「是否允许运行」，用户点了「否」，软件却显示「正在启动」。
4. 图三：搜索日服玩家能正常显示，但页签归在「韩服」分组下。
5. 图四：切回国服账号后，战绩区「当前已加载的战绩中没有该模式」，「近 0 场排位」「英雄胜率 已统计 0 场」，和外服失败时看起来一样。
6. 用户确认正常：关着客户端启动软件，遮罩很快消失；关闭客户端后入口重新出现（R219 生效）。

---

## P1　国服 SGP 战绩：新加字段按整数解析，国服返回的数据解析失败，整场对局被直接丢掉（最严重）

### 日志证据

| 版本 | `sgp_match_history_succeeded` |
|---|---|
| 0.12.71（`f29b9…`，多次） | `consumed=20`，`returned=20`，`participant_counts={"10":18~20,"1":0~2}` |
| 0.12.73（19:54:27 切回国服后，多次） | `consumed=73`，**`returned=3`**，`participant_counts={"1":3}`，`single_participant=3`，`filtered_custom=3`，**`visible=0`** |

- 同一账号、同一个 SGP 网关：服务器给了 73 场，程序只留下 3 场，而且这 3 场全是只有 1 名参与者的自定义/练习模式对局（随后又被当成自定义局过滤掉），所以可展示的对局是 0。
- 下一页 `beg_index=73` 也是 `returned=1`。`season_backfill_round scanned=20 ranked_samples=0`，赛季统计也是 0。
- 一次总览请求 `sgp_bytes` 约 24～30 MB（0.12.71 约 1/3），因为留下的对局太少，循环一直往后翻页。

### 根因（源码已核对）

- `backend/sgp_api.go` `matchHistoryFilteredOn`（约 912 行）：`json.Unmarshal(game.JSON, &info) != nil` 时直接 `continue`，**整场丢弃，不记任何日志**。只有解析成功且 `GameID > 0` 的对局才会被保留。
- 0.12.71 → 0.12.73 之间，`riotParticipant`（`riot_api.go` 约 707 行）新增了一批字段，类型都是 `*int`：`damageSelfMitigated`、`totalHealsOnTeammates`、`totalDamageShieldedOnTeammates`、`timeCCingOthers`、`damageDealtToBuildings`、`turretTakedowns`、`doubleKills`～`pentaKills`，以及 `challenges.dragonTakedowns / baronTakedowns / riftHeraldTakedowns`；`totalDamageTaken` 也从 `int` 改成了 `*int`。这些是 R216/R217 的战绩关键词和每分钟曲线加的。
- Go 的 `encoding/json` 遇到带小数的数字（如 `2.0`、`12.5`）写进整数字段会直接报错，整个 `Unmarshal` 失败。拳头官方 Match-V5（日服走的路径）返回的是纯整数，所以日服正常；**国服 SGP 的 JSON 至少有一个新字段不是纯整数**（最可疑的是 `challenges` 里的字段和 `timeCCingOthers`）。练习/自定义局没有这些数据，所以能活下来。这和「只剩 1 人局」的现象完全吻合。
- `gameplay.go` 约 701 行 LCU 战绩结构也加了同一批 `*int` 字段，可能有同样的问题（LCU 回退路径）。
- 测试没发现：R216/R217 的夹具都是手写的整数 JSON，没有一份真实的国服 SGP 原始响应。

### 修复要求

1. **用户执行补充（2026-10-05）：真实样本改为非阻塞项。** 不创建 `backend/testdata/r223/sgp-summary-tencent.json`；宽容类型与单场核心兜底直接实施，以合成数据验收。每个新增可选字段覆盖整数、小数、数字字符串、null、对象、数组；`challenges` 整体为 null、数组、字符串时按缺失处理。真正触发线上事故的字段留待 Windows 修复版日志确认，不能用合成样本冒充真实响应。
2. **解析要宽容**：这些「只用来算标签/曲线」的可选统计字段，改成可以接受整数、带小数的数字、数字字符串和 `null` 的类型（例如自定义一个 `lenientInt`，小数四舍五入或截断都行，但要统一）。`riotParticipant` 和 LCU 那份结构都要改。
3. **单个可选字段坏了，不能丢整场**：如果整场解析失败，再用只含核心字段（gameId、queueId、参与者基础数据等）的结构重新解析一次；还是失败才跳过。跳过时记一条新诊断 `sgp_game_decode_failed`，只记 `count` 和出错的**字段路径**（`json.UnmarshalTypeError.Field`，如 `participants.challenges.dragonTakedowns`）、值的类型（number/string），**不记值本身和任何身份信息**。runtime 和 `features.go` 的诊断白名单同步。
4. `sgp_match_history_succeeded` 增加 `decode_failed` 计数，以后看日志就能直接看出是不是这个问题。
5. 翻页兜底：一页里解析失败的对局计入 `consumed`，但不要因为可用对局少就一直翻页拉几十 MB。保持和 0.12.71 一样的请求量（一页 20 场就停）。
6. 赛季统计（`season_stats*.go`）、对局玩家战绩、历史同队玩家，凡是走 SGP 或 LCU 战绩解析的地方都确认用的是修好后的同一套解析，并用上面的真实样本跑一遍。

### 验收

- 按用户补充用合成国服结构样本：20 场十人局全部保留，`returned=20`；赛季统计、同队记录、关键词和每分钟曲线正常计算。真实国服具体坏字段留待真机确认，不阻塞 P1。
- 新增测试：把样本里 `challenges` 和 `timeCCingOthers` 等字段改成小数、字符串、`null`，对局都不被丢；构造一个核心字段损坏的对局，只有这一场被跳过并记录 `sgp_game_decode_failed`。
- 拳头 Match-V5（日服）和 LCU 路径的现有测试全部通过。

---

## P2　日服客户端的大区没识别出来：自己的页签被当成国服，Riot API 没用上，LCU 8 秒超时后就放弃

### 日志证据

- 19:41:38 `lcu_discovery result=connected`；19:41:39 **`client_platform_resolved platform="" region="" source=command-line-query`**。
- 19:41:47 `match_history_data_source_decision client_region="" reason=sgp-unavailable`，`attempts: sgp disabled（当前查询没有可用的 SGP 路由）, lcu failed（读取失败）`；`overview_phases_ms detailed_matches=8000`（正好 8 秒超时）。整次运行**没有一条 `lcu_history_retry`**，Riot API 也没被调用。
- 同一时间 `lcu_summary_history_succeeded returned=20`（只含本人的摘要能读到），排位 `PLATINUM I 17胜14负`，和图三日服数据一致，说明连上的确实是日服账号。
- 19:41:49 `mayhem_rating_lookup source=aramkit`：大区没识别出来，日服账号也被拿去查了国服专用的 ARAMKit。
- 之后用户手动搜日服玩家，走 `jp1.api.riotgames.com` / `asia.api.riotgames.com`，10 场全部读到（`riot_overview_cost matches_loaded=10`）。**Riot API 本身是好的，只是本人总览没有走到它。**

### 根因（源码已核对）

- `lcu.go` `applyPlatformArgs` 只认 `--region=` 和 `--rso_platform_id=`，并且**两个都有**才算识别成功。国服客户端两个都有（`--region=TENCENT --rso_platform_id=HN10`，日志里 `source=startup-args` 正常）。
- 拳头客户端的 LeagueClientUx 启动参数里有 `--region=JP`，但**没有 `--rso_platform_id`**（来源是 `command-line-query` 而不是 `startup-args`，说明进程命令行和 `/riotclient/command-line-args` 都没凑齐两个参数）。
- 于是 `region="JP"`、`platform=""`。`clientRiotPlatform` 再拿 `"JP"` 去查平台表，但表里只有 `"jp1"`，查不到，结果是空。`clientRegionInfo` 返回空，后端和前端都把它当成「既不是外服也不是国服」，而前端默认显示「国服」。
- `platformInfo` 第一次查询后就把 `platformProbe` 置为 true，**以后再也不查**，即使结果是空的。
- `clientPlatformDiagnostic` 在 `platform` 为空时把 `region` 也清空了，所以日志里看不到 `JP`。
- R221 的测试夹具都带着 `--rso_platform_id=JP1`，所以没测出来。

### 修复要求

1. **按客户端的大区名映射平台**：`JP→jp1`、`KR→kr`、`NA/NA1→na1`、`EUW/EUW1→euw1`、`EUNE/EUN1→eun1`、`BR/BR1→br1`、`LAN/LA1→la1`、`LAS/LA2→la2`、`OCE/OC1→oc1`、`RU→ru`、`TR/TR1→tr1`、`ME/ME1→me1`、`SG/SG2→sg2`、`TW/TW2→tw2`、`VN/VN2→vn2`、`TENCENT→国服`。PH2/TH2 等已并入 SG2 的旧名也要映射到 `sg2`。以拳头官方客户端实际使用的名字为准，未知值不能当成国服。
2. **再加一个权威来源**：连上客户端后读取 `/lol-platform-config/v1/namespaces/LoginDataPacket/platformId`（返回如 `"JP1"`）；读不到时再用 `/riotclient/region-locale`。优先级：启动参数平台 ID > LoginDataPacket > 大区名映射。
3. **识别结果为空时不要锁死**：没识别出来就在 `identityReady` 变为 true 时和之后每次状态刷新时重新识别（加退避，例如 2/4/8 秒，60 秒后只在刷新时重试），识别出来以后再锁定。从「未知」变成具体大区时，按 R221 的逻辑重新初始化本人页签和总览。
4. **未知大区不能当国服**：
   - `/api/status` 的 `clientRegion` 在未知时保持空，前端**不显示「国服」**，页签暂不归组（或只显示加载状态），直到识别出来。
   - 未知时不调用 ARAMKit、腾讯 SGP 等国服专用功能。
   - 未知时 LCU 战绩失败也要按 R221 的 3/6/12 秒最多 4 次自动重试，不能一次超时就停在「战绩暂时无法读取」。
5. 诊断：`client_platform_resolved` 在平台为空时也保留大区名的**枚举值**（不在白名单里就记 `other`），并加 `has_region_arg`、`has_platform_arg`、`source`（startup-args / command-line-query / login-data-packet / region-locale）、`attempt`。不记任何其他命令行内容。

### 验收

- 夹具：命令行只有 `--region=JP`、没有 `--rso_platform_id` → `clientRegion=jp1`，`clientRegionLabel=日服`；本人总览走 Riot API。
- 夹具：命令行什么都没有，LoginDataPacket 返回 `"JP1"` → 识别为 `jp1`。
- 夹具：第一次识别为空、第二次才成功 → 第二次后页签和总览切换到日服，并只重新加载一次。
- 夹具：未知大区 → 前端不显示「国服」，不请求 ARAMKit。
- 每个外服平台各至少一个大区名映射用例；`TENCENT` + `HN10` 行为不变。

---

## P3　登录入口：点「否」后仍去直接启动 LeagueClient.exe；Riot 入口报「无法直接启动游戏」；启动后入口被收起

### 日志证据（北京时间）

| 时间 | 事件 |
|---|---|
| 19:39:51 | `client_launch riot requested → started source=riot` |
| 19:40:00 | 再点一次 Riot：`requested → started source=riot`（截图二就在这之后、19:40:13 之前） |
| 19:40:13 | `official_login_launch tcls requested` |
| 19:40:18 | `attempt_failed source=launcher stage=shell-execute error_code=1223`；`attempt_failed source=tcls error_code=1223` |
| 19:40:18 | **`official_login_launch result=started source=league-client`** |
| 19:40:18 | `lcu_discovery credentials-unreadable process_count=1` → 遮罩「检测到客户端正在启动」，15 秒后超时关闭 |

- Windows 错误码 1223 是 `ERROR_CANCELLED`，就是用户在「是否允许运行」窗口点了「否」。

### 根因（源码已核对）

1. `client_launcher.go` `launchClientCandidates` 把每个候选依次试一遍，**任何失败都继续试下一个**，包括 1223（用户主动取消）。
2. TCLS 入口的候选里有 `{root}\LeagueClient\LeagueClient.exe` 和 `{root}\LeagueClient.exe`（`buildDetectedClientInstallations` 79～80 行）。这个程序不能直接启动，启动后会弹出「我们无法直接启动游戏。请通过恰当的启动器来开始游戏。」。所以用户点了「否」之后，程序绕过了他的拒绝，直接把 LeagueClient.exe 拉起来，进程一出现，遮罩就显示「正在启动」。
3. Riot 入口执行的是 `RiotClientServices.exe --launch-product=league_of_legends --launch-patchline=live`，参数本身是对的；但截图二的报错出现在第二次点击 Riot 之后。现有日志不足以判断是哪个原因：
   - 选中的 `RiotClientServices.exe` 不是拳头官方安装的那个（候选来自 `RiotClientInstalls.json` 里的**所有**路径和 C～F 盘的猜测路径）；
   - Riot 客户端里英雄联盟产品的安装路径指向了腾讯目录；
   - 或者是 6 秒冷却后再次 `--launch-product` 触发的。
4. 前端 `renderLaunchpad`（`app.js` 约 900 行）：只要 `state.clientLaunched` 有值，就 `launcherList.hidden = true`，直到连上客户端或点「重新选择入口」。`ShellExecute` 成功不代表客户端真的起来了；客户端报错退出后，入口仍然被收起。

### 修复要求

1. **错误码 1223 立即停止**：不再尝试其他候选，返回「已取消」（新结果 `cancelled`，HTTP 可用 409 或 200 加 `cancelled:true`）。前端回到初始入口状态，不弹错误提示，不显示「正在启动」，也不重新显示遮罩。诊断 `official_login_launch/client_launch result=cancelled`。
2. **删除直接启动 `LeagueClient.exe` 的候选**（两个 `league-client` 路径）。国服只用 `Launcher\Client.exe`、`TCLS\Client.exe` 和快捷方式；都不可用时返回失败，不要退回到 LeagueClient.exe。旧测试里依赖这条回退的，按新行为修改。
3. **Riot 入口**：
   - 只使用拳头官方 Riot Client：优先读取 `RiotClientInstalls.json` 里的 `rc_default` / `rc_live`；路径在腾讯游戏目录下的跳过；猜测路径只作为最后兜底。
   - 启动前读取 `C:\ProgramData\Riot Games\Metadata\league_of_legends.live\league_of_legends.live.product_settings.yaml` 里的 `product_install_full_path`，如果它指向腾讯目录或不存在，不要启动，返回「Riot 客户端里没有可用的英雄联盟安装」。
   - Riot Client 进程已在运行时，再点 Riot 入口不重复传 `--launch-product`，改为把已有的 Riot Client 窗口带到前台（做不到就提示「Riot 客户端已经打开」）。
   - 在用户机器上复现截图二，诊断里记录：选中的是哪一类路径（`rc_default` / `rc_live` / `installs-json-other` / `drive-guess` / `shortcut`）、是否位于腾讯目录、`product_settings` 是否存在、所在安装目录是哪一类，只记枚举不记路径。在账本里写明真正的原因。
4. **入口不收起**：
   - 点击后入口一直显示，正在启动的那一张卡片显示「正在启动…」状态并暂时禁用，其他入口仍可点。
   - 「已启动 / 正在登录」提示放在入口列表上方，不替换列表。
   - 启动后 60 秒内没检测到客户端进程（`process-not-found`），或客户端进程出现后又消失，提示文字恢复为初始状态，卡片恢复可点。
   - 连上客户端后入口卡按原逻辑收起。「重新选择入口」按钮不再需要，删掉。
   - 不新增解释性文字。

### 验收

- 模拟 ShellExecute 返回 1223 → 只尝试一个候选，结果 `cancelled`，不启动任何进程，前端回到初始状态、没有遮罩。
- `buildDetectedClientInstallations` 的输出里不再出现 LeagueClient.exe。
- 前端：点击后列表 `hidden=false`；启动成功后、未连接前，列表仍可见；超时或失败后，卡片恢复可点。
- 真机：点「否」不启动任何东西；Riot 入口能正常拉起登录窗口，不再出现「无法直接启动游戏」。

---

## P4　搜索服务器菜单：外服折叠成一项，只保留玩家多的服务器，默认韩服

现状（图一）：菜单顶部是「国服 ›」（点开才有子服务器），下面是一个「韩服」标题，再下面平铺了全部 15 个外服。

要求：

1. 和「国服」一样，增加一个可折叠的「**外服 ›**」项（同样的样式和交互），点开后显示外服服务器列表。国服和外服互斥展开。
2. 列表只保留玩家较多的 8 个：**韩服、日服、美服、欧西、欧北东、台服、越南、东南亚**。其余平台（巴西、拉北、拉南、中东、大洋洲、俄服、土耳其）不在菜单里显示，但后端和页签继续支持（例如从对局里点开、客户端本身就是这些区）。
3. 外服列表第一项是「跟随客户端」，和国服一致：当前客户端是外服时显示括号里的区名并可选；否则禁用。如果当前客户端在上面 8 个之外（如巴西），「跟随客户端」仍然能用。
4. **默认选中韩服**：菜单切到外服但没选具体服务器时，以及首次使用时，外服默认是韩服。当前客户端是外服时，搜索默认「外服 · 跟随客户端」；客户端是国服时，默认「国服 · 跟随客户端」（保持现有逻辑）。用户手动选择过的，记住手动选择。
5. 按钮文字：选外服时显示具体区名（如「日服」）；选「跟随客户端」时显示「外服 · 日服」。
6. 删除菜单里那个单独的「韩服」标题行。

验收：前端测试覆盖菜单结构（只有国服/外服两个折叠项、外服 8 项加跟随客户端）、默认韩服、跟随客户端可用/禁用、手动选择被记住。

---

## P5　玩家分组改名「外服」，并在总览玩家名旁显示具体服务器

1. `gameplay.js` `PLAYER_GROUPS.kr` 的显示名「韩服」改为「**外服**」（内部键名可以不改）。分组切换菜单、`aria-label`、空状态提示里的「韩服」一起改为「外服」（如「在顶栏搜索里选外服服务器并搜索玩家」）。凡是指「明确只查韩服」的地方（职业选手目录、绝活哥、英雄高手榜）保持「韩服」不变。
2. 总览头部：把服务器标签（「日服」「国服 · 黑色玫瑰」这类）移到**玩家名 `名字#编号` 的右边**，不再放在「召唤师等级」后面。国服显示具体子服名，外服显示区名。
3. 外服页签上同样显示一个小的区名标签（如 `hate camille#PPPPP` 后面加「日服」），因为外服分组里会同时有多个区的玩家。
4. 本人页签只有在 P2 识别出大区后才归组；识别出是外服就进「外服」组，国服进「国服」组。

验收：前端测试——日服搜索结果进入「外服」组，页签和头部显示「日服」；国服显示子服名；职业目录仍写「韩服」。

---

## P6　外服限制检查

把「当前客户端是外服」时的限制和「大区未知」时的行为一起核对一遍，写进账本，逐项写明结论：

| 功能 | 外服客户端 | 大区未知 |
|---|---|---|
| 腾讯 SGP（战绩、段位补充、跨服搜索） | 不用 | 不用 |
| ARAMKit 海克斯乱斗评分 | 不调用（本次日志里日服账号调用了，P2 修好后不应再出现） | 不调用 |
| TCLS / 国服纯净入口 | 已连接时隐藏；断开后重新显示（R219 已满足） | 显示 |
| 本人总览战绩 | Riot API → LCU（带重试） | LCU（带重试），识别出来后切换 |
| 对局内玩家战绩/段位 | 按对局所在平台走 Riot API → LCU | LCU |
| 练习工具符文推荐（R221 P6） | 正常 | 正常 |
| OP.GG 当前对局（每分钟轮询） | 用对局平台 | 不轮询 |

另外：日志显示外服搜索页签每 60 秒轮询一次 OP.GG 当前对局（`overview_current_game region=jp1 state=none`，持续十几分钟）。请确认窗口不在前台或页签不在当前显示时会暂停轮询，没有的话补上。

---

## P7　删除空状态里的统计口径说明（UI 红线）

截图一和图四里出现了这类说明文字，违反「界面不加统计口径/说明」的要求，一并删掉，只保留标题：

- `gameplay.js` 2389 行「基于首屏最近对局，至少需要 3 场包含完整参与者数据的排位对局。」
- `gameplay.js` 2427 行「基于首屏已加载的最近对局，最多统计 20 场。」
- `gameplay.js` 2572 行「位置偏好基于首屏已加载的最近排位对局。」
- `season_stats.go` 634、695 行返回的「当前数据源不提供赛季统计」：显示在「英雄胜率」标题旁，改成不显示这句话（只保留赛季标签）。

同时在全部前端代码里搜一遍「基于」「统计口径」「最多统计」「至少需要」，有同类说明的一起删。

---

## 测试与记录

- Go：P1 合成数据与宽容解析测试（真实样本按用户补充非阻塞）、`sgp_game_decode_failed` 白名单、P2 大区映射和 LoginDataPacket 回退、P3 遇到 1223 立即停止、不再出现 LeagueClient.exe 候选、Riot 路径分类。跑 `go test ./backend -count=1`、`go vet ./backend`。
- 前端：P2 未知大区不显示国服、P3 入口不收起、P4 菜单、P5 分组和标签、P7 文案已删除。跑全量 `node --test backend/web/*.test.cjs`。
- 新建 `docs/r223-execution-ledger.md`，验证日志放 `docs/history/reports/r223/`；在 `docs/WORKLIST-INDEX.md` 登记。
- **真机验证（Windows，GPT 执行）：**
  1. 国服账号总览 20 场、赛季统计、对局玩家战绩都正常（P1）。
  2. 日服账号登录后，本人页签进「外服」组、显示「日服」，总览战绩能读出来（P2）。
  3. 点国服纯净入口、在「是否允许」里点「否」：什么都不启动，入口还在（P3）。
  4. Riot 入口不再出现「无法直接启动游戏」（P3）。
  5. 日服 → 国服 → 日服来回切换，分组、标签、战绩都正确。
