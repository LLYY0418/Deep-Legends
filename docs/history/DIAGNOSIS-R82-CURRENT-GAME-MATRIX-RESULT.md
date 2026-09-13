# R82 探测矩阵实测结论：GSM 对任意玩家的路已经走到头

数据来源：`lol-loot-diagnostics-0912-1158.jsonl`，`trace_id=cgm-11`，`state=completed`，`cell_count=11`，日志行 16945–16963。矩阵完整执行，离线对照通过 presence 反证（`in_game_contradiction=false`），两种令牌均成功固定（`credential_pinned=true`）。

**这一轮拿到了决定性结果。不需要再采集同类日志。**

---

## 1. 十格 GET 的完整结果

| 格 | 路径族 | 区服码 | 令牌 | 目标 | 状态码 | 响应体 | 错误信封 |
|---|---|---|---|---|---|---|---|
| A-self | region | 大写 | session | 本人 | **404** | 119B | `message/errorCode/httpStatus` |
| A-offline | region | 大写 | session | 离线真人 | **401** | 127B | 同上，`UNAUTHORIZED` |
| A-synthetic | region | 大写 | session | **不存在的 PUUID** | **401** | 127B | 同上，`UNAUTHORIZED` |
| B-self | region | 大写 | entitlements | 本人 | **404** | 119B | 同上 |
| B-target | region | 大写 | entitlements | 在局目标 | **401** | 127B | 同上 |
| C-self | spectator | 大写 | session | 本人 | **404** | 119B | 同上 |
| C-target | spectator | 大写 | session | 在局目标 | **405** | 61B | **`status.status_code`（另一种信封）** |
| D-self | spectator | 大写 | entitlements | 本人 | **404** | 119B | 同上 |
| F-A-target | region | 小写 | session | 在局目标 | **401** | 127B | 同上 |
| F-C-target | spectator | 小写 | session | 在局目标 | **401** | 127B | 同上 |
| OPTIONS-target | spectator | 大写 | session | 在局目标 | **405** | 0B | 无 `Allow` 头 |

---

## 2. 唯一的自变量是「路径里的 PUUID 是不是令牌自己的 sub」

把十格 GET 按 `token_identity_sub.equals_target` 分组：

```
sub == 路径 PUUID  →  404, 404, 404, 404     （4/4）
sub != 路径 PUUID  →  401, 401, 401, 405, 401, 401   （6/6，无一例 200 或 404）
```

**零例外。** 路径族（region / spectator）、令牌种类（league-session / entitlements，两者 `token_issuer_kind` 分别是 `other` 和 `tencent-lol`，确实是两份不同的令牌）、区服码大小写——这三个变量全部换过，**一个都不影响结果**。

### 2.1 最关键的一格：不存在的 PUUID 也返回 401，不是 404

`A-synthetic` 用的是随机生成、假定不存在的 PUUID，返回 **401 UNAUTHORIZED**，响应体与真实离线玩家的 401 **完全同构、同长度（127 字节）**。

这意味着：**服务端根本没有去查这个玩家存不存在、在不在对局中。** 鉴权在业务查询之前就拒绝了。

于是此前所有"目标隐藏战绩"、"目标是否正在对局"、"是不是只有排位能查"的猜测，全部被排除——这些都是关于**目标**的属性，而服务端压根没看目标。

### 2.2 405 不是方法问题，是路由层的副产物

`C-target` 的 405 响应体是 `{"status":{"status_code":405,"message":…}}`，只有 61 字节，**和所有 401/404 的 `{message,errorCode,httpStatus}` 完全不是一个信封**。前者来自网关/路由层，后者来自 ledge 业务服务。

而同一条 spectator 路径，只把区服码换成小写（`F-C-target`），就变回了业务层格式的 **401**。

所以：405 从来不是"这条路由只收别的方法"。`OPTIONS-target` 也返回 405 且**没有 `Allow` 头**（`allow_header_present=false`，`allowed_methods={}`）。

> 这里要认账：我在上一版工单的 P0-1 里判断"405 的 `Allow` 头里已经藏着答案，先去读那两行"。**这个判断是错的。** 字段确实早就采集了，但值一直是空的——服务端从来没发过 `Allow`。这条假设现在正式关闭。

### 2.3 spectator 路径是真实存在的，但不是绕过口

`C-self` / `D-self` 对**本人**返回 404，响应体与 region 路径的 404 **逐字节同构（119B）**。说明 `/gsm/v1/ledge/spectator/region/{ID}/puuid/{p}` 是一条真实可用的 GET 路由，行为与 region 路径一致——**同样绑定 sub，同样不接受别人的 PUUID**。

Ezhana 那个 2024 年仓库里 `GetGamingInfoByPuuid` 接受任意 puuid 的函数签名，只是签名而已，不代表服务端接受。

---

## 3. 结论

**GSM 这条链对"任意国服玩家"已经确定不可用。** 不是配置问题、不是令牌问题、不是路径写错、不是目标隐私设置，而是服务端按调用者身份绑定：**只能查自己**。

日志里代码自己给出的结论是 `both-other-controls-auth-rejected;sub-binding-not-proven`。"not proven" 这个保留是对的——我们没有验证 JWT 签名，也不能排除"服务端用了别的等价 ACL 而恰好与 sub 一致"。但要真正"证明"，需要一份 sub 不等于本人 PUUID 的合法令牌，而那种令牌拿不到。

**对产品决策而言，10 格零例外的相关性已经足够。不要为了追求这个"证明"再开一轮。**

已经穷尽的轴：路径族 ×2、令牌 ×2、区服码大小写 ×2、方法（GET/OPTIONS）、目标类别 ×4（本人 / 在局他人 / 离线真人 / 不存在）。

---

## 4. 范围内还剩最后一件事：那份 3MB 的客户端契约文档

端点清单三个来源：

| 来源 | 状态 | 结果 |
|---|---|---|
| `/swagger/v3/openapi.json` | **404** | 客户端没有这个文档 |
| `/swagger/v2/swagger.json` | **404** | 同上 |
| `/help?format=Full` | **200** | `body_bytes=3,021,918`，`truncated=false`，但 `parsed=false`、`error_kind=unrecognized-contract-format`、`paths_scanned=0` |

**客户端把 2.9MB 的完整接口契约交出来了，我们没读懂。** 这是范围内唯一还没穷尽的证据来源。

### 4.1 为什么没解析出来

`current_game_endpoint_inventory.go:64-101` 的回退分支要求 `root["functions"]` 是 `map[string]any`。而 `truncated=false` 说明循环体一次都没进过——如果 `functions` 存在且里面有条目解析失败，`out.Truncated` 会被置 true。所以根对象的形状和代码假设不一样（`functions` 可能是数组，或者顶层键名不同）。

### 4.2 修法（建议两条都做）

1. **先把形状打出来，别再猜**：记录根对象的顶层键名列表、每个值的 JSON 类型和元素个数。这是一份**公开的、与账号无关的接口契约**，同一版本客户端人人相同，不含任何凭据或玩家数据，没有脱敏必要。

2. **加一条与格式无关的兜底**：对原始文本直接跑正则收集所有形如 `/lol-[a-z0-9-]+/v\d+/…` 的唯一路径，再用现有的 `rosterEndpointKeyword`（`spectat|gsm|active|featured|ongoing|live`）过滤。这条不依赖任何 JSON 结构，客户端换格式也打不垮它。

搜出来的结果决定最终结论：

- 如果存在某个接受他人 PUUID 的实时对局端点 → 新链路，继续做。
- 如果没有 → **"任意国服玩家的正在进行的对局"在约定范围内确定没有数据源**，进第 5 节。

---

## 5. 如果 `/help` 也没有：产品必须收敛（请提前想好）

这不是"做不好"，是没有数据源，需要如实反映在界面上。

| 目标 | 能拿到什么 | 展示 |
|---|---|---|
| 自己 | LCU gameflow / champ-select 全量；GSM 自查 200 | 完整对局详情。分档校验已落地（`gsmRosterSession` 的 `partial` 档），但还没在真实 200 上验证过 |
| 好友 | `/lol-chat/v1/friends` presence：`championId`、`gameId`、`gameMode`、`queueId`、`mapId`、`timeStamp`、`gameStatus` | "对局中"卡片：英雄 + 队列 + 已进行时长。不伪造十人 |
| 非好友 | **什么都没有**——连"他是否在对局中"都没有来源 | **整个模块不渲染**。把"当前对局无法确认"那行去掉 |

第三行是你从 R68 起就反复提的那个诉求，现在有了明确依据：不是暂时查不到，是这个位置永远不会有内容。

---

## 6. 下一轮工单

**P0（唯一必做）**：按 4.2 修 `/help?format=Full` 的解析 —— 先打形状，再加正则兜底。跑完把命中 `spectat|gsm|active|featured|ongoing|live` 的端点全部列出来（路径 + 方法 + 是否有 puuid 参数）。

**P1**：`sgp_gateway.go` 两处（沿用上一版追加工单）——从 endpoint 主机名反解平台码以摆脱已 404 的 `platformId` 接口；`builtin_matches` 改为对客户端 base 独立重算，并记录客户端返回的 host。**这两条与 401 无关，只是消除残余变量。**

**P1**：`social.go:19` 的 `spectatorPresenceFieldNames` 补 `spectatorKey`（只记存在与否）。

**P2**：按第 5 节收敛 UI（建议现在就开始做设计稿，不要等 P0 结果——三种目标的展示形态无论如何都要分开）。

**明确不再做**：不再对 GSM 任意组合发请求；不再采集同类 401 日志；不再尝试用 POST 或改方法访问 spectator 路径（405 已证明与方法无关）。

**验收判据**：新日志里 `current_game_endpoint_inventory` 对 `/help?format=Full` 必须 `parsed=true` 且 `paths_scanned > 0`；若仍为 false，必须附带根对象形状记录，不允许再出现一次"200 但读不懂"。
