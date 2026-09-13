# WORKLIST-R82 国服「正在进行的对局」：把猜测收敛成探测矩阵

范围：**任意搜索到的国服玩家**（不限好友）。方案重量级：**只做轻量只读接口探测**，不启动观战客户端，不接第三方登录态接口。

本工单不是"再加一轮日志"。前面十几轮已经把日志做得很细了，问题不在日志密度，在于**关键对照组一直没跑**，以及**已经采到的字段没被读过**。下面每一条都写明：为什么它是未知的、怎么测、测出什么算什么结论。

---

## 0. 先纠正三条被当成结论的推论

这三条都可以在仓库里直接核对，不需要新日志。

### 0.1 「spectator 路径 405」这条证据缺对照组，不能当成路径不可用

`probeCurrentGameContract`（`current_game_diagnostics.go:57`）只在一个地方被调用：`overview_current_game_sgp.go:41-42`，条件是**主请求对目标返回 401/403 且是手动刷新**。也就是说这条 spectator 路径**从来没有对"本人"跑过**。

而 region 路径之所以能立住结论，靠的正是"本人 200 / 目标 401"这组同凭据对照。spectator 路径缺同款对照，于是 405 目前只能说明"spectator 路径 + entitlements 令牌 + GET + 这个目标"这一格不通。它既不能证明路由不存在，也不能证明它对本人不通，更不能证明换令牌不通。

报告里写的「历史 spectator GET 对照本次仍为 405」把一格当成了一整行。

### 0.2 405 的 `Allow` 头已经采集了，但两份报告都没读

`sgpResponseDiagnostic`（`sgp_auth_diagnostics.go:295-318`）已经把 `allow_header_present` 和 `allowed_methods` 写进事件；`probeCurrentGameContract` 在非 200 分支会先 merge 它再返回（`current_game_diagnostics.go` 的 `http-error` 分支）。

所以 `lol-loot-diagnostics-0911-2326.jsonl` 的 **16695、16977 两行里，大概率已经写着这条路由到底允许哪些方法**。而报告写的是「405 不说明换成 POST 就是安全的阵容读取契约」——在手上就有服务器自己给出的答案的情况下，用谨慎话术替代了读取。

> 这和记忆里 R79 的 `ItemShop*` 是同一个失效模式：白名单里采了字段，十几轮没人读那一段，却一直在讨论"要不要再加日志"。

**第一步不是发请求，是把这两行读出来。零成本。**

### 0.3 「令牌没有查询他人的权限」这个说法已经被自己的日志否掉了

同一份 league-session 令牌，在同一次运行里：

- `RANKED`（`leagues-ledge/v2/rankedStats/puuid/{X}`）：11 次 200
- `SUMMONER`（`summoner-ledge/v1/regions/{id}/summoners/puuids`）：3 次 200

这些查的就是**别人**的 PUUID。所以 ledge 家族不是整体自绑定，被拒的只有 GSM 这一个服务。这把问题从"国服不让查别人"缩小到"GSM by-puuid 这条路由的授权口径"，范围小得多，也说明还远没到该放弃的地步。

---

## 1. 覆盖矩阵：6 格里只测了 2 格，而且这 2 格互相不可比

| # | 路径 | 令牌 | 方法 | 本人 | 他人 |
|---|---|---|---|---|---|
| A | `/gsm/v1/ledge/region/{ID}/puuid/{p}` | league-session | GET | **200 已测** | **401 已测** |
| B | `/gsm/v1/ledge/region/{ID}/puuid/{p}` | entitlements | GET | 未测 | 未测 |
| C | `/gsm/v1/ledge/spectator/region/{ID}/puuid/{p}` | league-session | GET | 未测 | 未测 |
| D | `/gsm/v1/ledge/spectator/region/{ID}/puuid/{p}` | entitlements | GET | **未测** | **405 已测** |
| E | `/gsm/v1/ledge/region/{ID}/gameId/{g}` | league-session | GET | 未测 | 未测 |
| F | 以上任意路径，区服码**小写** | 任一 | GET | 未测 | 未测 |

两个公开实现恰好分处对角线，这也是为什么它们不能互相印证：

- LeagueAkari `src/shared/http-api-axios-helper/sgp/gsm.ts`：region 路径 + **league-session**，调用点 `ongoing-game/additional-info-controller.ts:156,161` 传的是 `summoner.me.puuid`，且只在本机 `phase === 'in-game'` 时触发 → **A 行，只有本人**。
- Ezhana/lcu `sgp.go` 的 `GetGamingInfoByPuuid`：spectator 路径 + **entitlements**（`lcu.go` 的 `GetSgpToken` 读 `/entitlements/v1/token`），函数签名接受任意 puuid → **D 行**。

F 行不是凑数：本仓库 `summoner-ledge` 用的是 `strings.ToLower(serverID)`（`sgp_api.go:1199`），而 GSM 两条路径都用 `strings.ToUpper`（`overview_current_game_sgp.go:32`、`current_game_diagnostics.go:84`）。同一个网关上两种大小写并存，说明大小写不是随便写的。**路由没匹配上时网关返回 405 而不是 404，是完全可能的。**

---

## 2. P0 探测清单

全部是只读 GET/OPTIONS，无写操作，无观战启动，无第三方域名。除 P0-2 外都不需要任何人正在对局中。

### P0-1　先读已有日志，不发任何请求

从 `lol-loot-diagnostics-0911-2326.jsonl` 第 16695、16977 行取出 `current_game_contract_probe` 的：

`allow_header_present` / `allowed_methods` / `content_type` / `auth_challenge_present` / `auth_challenge_classes` / `location_present` / `body_bytes` / `upstream_host` / `upstream_port`

判定：

- `allowed_methods` 非空 → **服务器已经直接告诉你这条路由收什么方法**，按它走 P0-5，不要再猜。
- `content_type` 是 `text/html` 且 `body_bytes` 很小 → 大概率打到了网关的兜底页而不是业务路由，优先怀疑路径/大小写（F 行），而不是方法。
- `allow_header_present=false` 且返回 JSON → 才需要 OPTIONS 探路。

### P0-2　离线目标对照（决定性，随时可跑，一条就能判生死）

对 **A 行** 依次打三个目标，同一份 league-session 令牌、同一次运行内连续发出：

1. 本人 PUUID（对照基线）
2. 一个**确定不在对局中**的真实国服玩家 PUUID（非好友即可，从搜索页拿）
3. 一个格式合法但**不存在**的 PUUID

判定真值表：

| 目标 2（离线真人） | 目标 3（不存在） | 结论 |
|---|---|---|
| 404 | 404 | 网关**确实解析了目标**，只是"没有对局"。那么之前的 401 就和"目标正在对局"强相关——属于条件性拒绝（隐私 / 对局可见性 / 观战权限），**这条路没死**，进 P0-3 继续缩小 |
| 404 | 401 或 400 | 同上，且顺带确认了"查不到"和"不让查"是两种响应，后续所有 401 都必须按"条件性拒绝"解释 |
| 401 | 401 | JWT `sub` 硬绑定，GSM by-puuid 就是本人专用。**A 行对任意玩家彻底断**，直接跳到 P0-3/P0-4 看有没有别的路由；若也全负，进第 4 节降级 |

这条之所以重要：它把一直卡着的"要等你正好在对局中、好友正好也在对局中"的采集门槛**完全去掉了**。前面好几轮都在等这个条件，而这个对照根本不需要它。

### P0-3　补齐令牌 × 路径的 2×2（B、C、D-self 三格）

同一次运行、同一个目标、固定四格顺序，每格一次 GET，不重试：

- B：region + entitlements，本人与他人各一次
- C：spectator + league-session，本人与他人各一次
- D-self：spectator + entitlements，**本人**（补上 0.1 缺的对照组）

记录：`http_status`、`allowed_methods`、`content_type`、`auth_error_class`、`body_bytes`、`token_identity_sub` 与路径 puuid 的字面相等。

### P0-4　区服码大小写（F 行）

对 A、C 两条路径各再发一次，区服码改小写（`hn10`）。若 405 变成 401/404/200，说明之前一直在打错路由——这会一次性推翻前面好几轮的"授权"结论。

### P0-5　OPTIONS 探路（仅在 P0-1 没拿到 `Allow` 时执行）

对 spectator 路径发 `OPTIONS`，只读响应头的 `Allow`，不读 body、不改产线方法。OPTIONS 是安全方法，不产生副作用，比直接试 POST 稳妥得多。

拿到 `Allow` 之后再决定要不要动方法，**不要跳过这一步直接 POST**。

### P0-6　拉本机 LCU 的完整端点清单（项目至今没做过）

运行中的客户端自己会生成 OpenAPI 文档，按顺序试：

```
GET https://127.0.0.1:{port}/swagger/v3/openapi.json
GET https://127.0.0.1:{port}/swagger/v2/swagger.json
GET https://127.0.0.1:{port}/help?format=Full
```

拿到后在里面搜：`spectate` / `spectator` / `gsm` / `active-game` / `activeGame` / `featured` / `ongoing` / `live`。

这是**唯一能一次性穷尽"国服客户端本地到底有没有查别人在局的能力"**的办法。目前已知的只有 `POST /lol-spectator/v1/spectate/launch`（需要 presence 里的 `spectatorKey`，非好友没有），但那是从两个第三方项目里反推的，不是穷举。这份文档是客户端自己吐的，是权威清单。

产出：把搜到的候选端点列成表（路径、方法、参数、是否接受 puuid），作为 R83 的输入。

### P0-7　SGP 网关地址改为向客户端要，消除混淆变量

`sgp_api.go:35-44` 写死了 10 个大区的网关地址。平台 ID 不在表里，`available()`（`sgp_api.go:250-269`）直接返回 false，整个功能**静默消失**——用户看到的现象就是"一直出不来"，而日志里连请求都不会有。

客户端本身就提供权威值：

```
GET /lol-platform-config/v1/namespaces/PlayerPreferences/ServiceEndpoint   → "https://hn10-k8s-sgp.lol.qq.com:21019"
GET /lol-platform-config/v1/namespaces/LoginDataPacket/platformId          → "HN10"
```

改成优先读客户端、读不到再回退到内置表，并把两者是否一致记一条日志。

这不一定是本次 401 的原因，但它是必须先消掉的变量：否则每份日志都要先自证"host 和区服码是对的"，而现在这一点从来没被验证过。

---

## 3. 一个独立的真实缺陷（顺带修，别混进上面的结论）

2326 日志里本人 GSM 返回 200，`IN_PROGRESS`、地图 11、队列 3110，但**己方 5 条、敌方 0 条，其中 4 条身份无效、3 条重复**。

这是自定义 / 人机局的**正常形态**，不是数据损坏。现在的阵容校验要求"双方非空且每个成员身份有效"，于是把一份已经拿到手的合法数据**整份丢弃**了。结果是：自己开自定义或人机局时，即使接口完全正常，页面也永远显示不出来。

修法：把校验从"全有或全无"改成分档——

- 双方齐全且身份有效 → 完整阵容
- 只有一方 / 存在无效身份 → 展示能展示的，无效位标为"未知玩家"，并在卡片上标明这是不完整阵容
- 无有效游戏 ID 或不在进行中 → 才算失败

**这条与 401 无关，不要拿它去解释好友查询失败，也不要用放宽校验来"凑"出十人。**

---

## 4. 如果 P0 全部为负：产品侧必须降级，而不是继续修

先说一条必须面对的事实：**非好友玩家连"他是否正在对局中"这个状态本身都没有来源。** `spectatorKey`、`gameId`、`gameStatus` 全部来自 `/lol-chat/v1/friends` 的 presence，只覆盖好友。所以如果 GSM 对任意 puuid 确认不可用，"任意搜索到的国服玩家的正在进行的对局"这个功能在轻量接口范围内就是**没有数据源**，不是没做好。

对应的 UI 收敛：

| 目标 | 能拿到什么 | 该怎么展示 |
|---|---|---|
| 自己 | LCU gameflow / champ-select 全量十人 | 完整对局详情（现在就能做，只差第 3 节的校验分档） |
| 好友 | presence：英雄、队列、开始时间、`spectatorKey` | "对局中"卡片 + 英雄 + 时长；观战按钮（链路已有） |
| 非好友 | 无 | **整个模块不渲染**，而不是显示"当前对局无法确认" |

用户在图里反复指出的"当前对局无法确认那行不要出现"，本质上说的就是这个：没有数据源时不要留一个永远失败的入口。

---

## 5. 明确不做

- 不再对**已经测过的那一格**重复发请求。
- 不再新增同类 401 日志。现有字段已经够了，缺的是没跑的对照和没读的字段。
- 不在拿到 `Allow` 之前改用 POST。
- 不放宽好友阵容的身份校验去"凑"十人（第 3 节的分档只适用于**已经 200 返回**的数据）。
- 不为了证明 401 再让用户开一局。P0-2 不需要任何人在对局中。

---

## 6. 验收判据

1. P0-1 的六个字段值被原文列出（脱敏后），并据此给出"方法问题 / 路径问题 / 授权问题"的三选一判断。
2. P0-2 的三目标真值表被完整填满，并明确落到表中的哪一行。
3. P0-3 的四格状态码全部有值，B/C/D-self 不得留空。
4. P0-4 大小写对照有结果。
5. P0-6 产出客户端端点清单，并列出所有含 `spectat|gsm|active|featured|ongoing` 的端点。
6. P0-7 改动后，日志中出现"客户端 ServiceEndpoint 与内置表是否一致"的记录。
7. 第 3 节的分档校验需要变异测试护栏：把分档逻辑改回"全有或全无"时，必须有测试变红。
8. 结论必须写成"哪一格测出什么"，禁止再出现"仍需进一步核实授权范围"这类无状态结论。

---

## 附：已核实的上游事实（2026-09-12 核对）

- LeagueAkari `main`，`src/shared/http-api-axios-helper/sgp/gsm.ts`：`getByPuuid` 用 region 路径 + league-session；`getByGameId` 用 `/gsm/v1/ledge/region/{id}/gameId/{g}`，其响应模型 `SgpGsmLedgeRegionGame` 含 `endOfGameTimestamp`、`eloChange`、`teamPlayerParticipantStats`，**确为终局统计**，不能当实时阵容用（但可作为"刚打完的对局详情"的数据源，属另一个需求）。
- LeagueAkari 全仓 `getByPuuid` 只有一处调用（`additional-info-controller.ts:232`），入参为本人 PUUID，触发条件 `phase === 'in-game'`。**它不是任意玩家查询可行的证据，也不是不可行的证据。**
- LeagueAkari `league-client/spectator.ts`：`POST /lol-spectator/v1/spectate/launch`，body 仅 `{puuid, spectatorKey}`。`spectatorKey` 在 `shared/types/league-client/chat.ts:176` 定义为好友 presence 的可选字段 → 观战链路天然只覆盖好友。
- LeagueAkari `shared/http-api-axios-helper/game-client/index.ts`：`https://127.0.0.1:2999/liveclientdata/*` 可取全量十人数据。本轮按范围约定不采用（需启动观战客户端），记录在此备查。
- Ezhana/lcu `sgp.go` `GetGamingInfoByPuuid`：`GET {endpoint}/gsm/v1/ledge/spectator/region/{CODE}/puuid/{puuid}`，`endpoint` 来自 `GetServiceEndpoint()`（即 P0-7 那个接口），令牌来自 `/entitlements/v1/token`。该仓库 2024 年提交，接口可能已变。
