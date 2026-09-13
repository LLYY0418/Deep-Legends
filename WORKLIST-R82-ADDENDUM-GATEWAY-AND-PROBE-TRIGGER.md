# WORKLIST-R82-ADDENDUM 探测从未被触发 + 网关判定逻辑自相矛盾

分析文件：`lol-loot-diagnostics-0912-1149.jsonl`，共 16723 行、0 行解析失败。最新运行 `run_id=e537e259ab5327d648a0d2eb`、版本 `0.12.1`、构建指纹 `3db0f3e1a604`，日志行 13785–16723，`log_seq` 1–2939 连续，导出完整。

结论先说：**这份日志没有回答 R82 的任何一个问题，因为探测矩阵一次都没跑。** 代码写好了，实验没做。

---

## 1. 硬证据：矩阵、端点清单、契约探测全部零次

| 应有事件 | 本次出现次数 |
|---|---|
| `current_game_matrix_start` / `_cell` / `_summary` | **0** |
| `current_game_matrix_offline_control` | **0** |
| `current_game_endpoint_inventory` / `_candidate` | **0** |
| `current_game_contract_probe` | **0** |
| 任何 `diagnostic_schema: 82` 的事件 | **0** |
| `current_game_access_export` 的 `last_comparison_id` | `""`（从未有过对照） |

而 CURRENT_GAME 这一格的结果与上一轮**逐字相同**：

```
7x  CURRENT_GAME | GET | session | 401 | /gsm/v1/ledge/region/{server_id}/puuid/{puuid}
```

7 次的 `sgp-scope` 全部是 `target_is_self=false`、`server_matches=true`、`target_private=false`、`provider_present=true`；`current_game_retry` 7 次全是 `credential-unchanged / skipped`；`current_game_payload` 7 次全是 `http-401`、`game_present=false`。其中 4 次 `manual_refresh=true`，说明你确实点了重试按钮。

同一次运行、同一份 session 凭据：`RANKED` 10×200、`SUMMONER` 2×200（entitlements 的 `SUMMARY` 28×200）。**再次确认令牌本身能查别人，被拒的只有 GSM 这一个服务。**

### 为什么这轮连"顺带的探测"都没有了

R82 主动把自动契约探测从重试路径删掉了（`overview_current_game_sgp.go:40-43`，注释写明"comparisons are explicit matrix operations"）。这个改动本身合理，但副作用必须讲清楚：

> **从这个版本起，正常使用 + 导出日志永远不会再产生任何新证据。** 每一份日志都会长得和这份一模一样。所有新信息只能来自手动执行的探测矩阵。

---

## 2. 探测矩阵怎么触发（它是手动入口，不是自动跑的）

后端是 `POST /api/diagnostics/current-game-matrix`（`main.go:519`），前端在设置页的一个**默认折叠**的 `<details>` 里（`web/index.html:324`，`web/current-game-probe.js`）。操作步骤：

1. 先在**总览页加载两名国服玩家标签**。候选来源是 `window.deepLegendsCurrentGameProbeCandidates`（`web/gameplay.js:5985`），它只收 **非本人、非 Riot 服、已加载出 `player.playerRef` 的标签**。只加载一个不够，会一直提示"先加载两名同区玩家"。
2. 打开 **设置 → 隐私与安全** 页签，拉到最下面，展开 **「国服当前对局：只读对照探测（R82）」**。
3. 点 **更新玩家列表**，两个下拉框才会出现候选。
4. **查询目标**选一个正在对局中的玩家；**离线对照**选另一个**确定不在对局中**的玩家。两者不能相同、不能是本人。
5. 勾上「我已确认离线对照…当前不在对局中」——不勾，执行按钮是禁用的，后端也会直接 400。
6. 点 **执行一次只读探测**，等最多 35 秒跑完，**然后再导出日志**。

后端还有两道会让它静默失败的闸门，事先知道能省一轮：

- 离线对照若在好友 presence 里显示 `inGame`，直接判 `offline-control-in-game` 不执行（`current_game_probe_matrix.go` 的 `current_game_matrix_offline_control`）。
- 目标或对照的 `ServerID` 与网关解析出的平台不一致，直接判 `invalid-gateway-or-server-scope`。

> 建议顺手改一件事：把这个入口从设置页深处挪到"好友正在游戏中"卡片上"重试阵容查询"旁边，或者在阵容查询连续失败 N 次后主动提示一次。现在这个入口的发现成本太高了——这一轮就是这么丢掉的。

---

## 3. P0-7 确实上线了，但结论字段结构性失效

这是本轮**唯一**自动执行的 R82 项，6 次记录完全一致：

```json
{"event":"sgp_gateway_resolution","source":"builtin-fallback","server_id":"HN10",
 "endpoint_status":200,"endpoint_valid":true,
 "platform_status":404,"platform_valid":false,
 "builtin_known":true,"builtin_matches":false,"builtin_comparison_available":false,
 "upstream_host":"hn10-k8s-sgp.lol.qq.com","upstream_port":"21019"}
```

拆开看，有一条新事实和两个缺陷。

### 3.1 新事实：`platformId` 那个接口在这台客户端上是 404

`/lol-platform-config/v1/namespaces/LoginDataPacket/platformId` → **404**。这是 Ezhana 那个 2024 年仓库里用的路径，现在国服客户端上已经不存在了（或换了命名空间）。

而 `/lol-platform-config/v1/namespaces/PlayerPreferences/ServiceEndpoint` → **200 且 `endpoint_valid=true`**，也就是说**客户端把权威网关地址给我们了，而且通过了官方域名校验**。

### 3.2 缺陷一：因为 platformId 404，客户端给的权威地址被整份丢弃

`sgp_gateway.go:124`：

```go
if valid && sgpPlatformCode.MatchString(dynamicPlatform) {
    result.Platform, result.Base, result.Source = dynamicPlatform, base, "client"
}
```

要求**两个读取都成功**才采信客户端。platformId 恒 404 ⇒ 这个分支**永远进不去** ⇒ `source` 永远是 `builtin-fallback` ⇒ 仍然用写死的表。P0-7 想消除的那个变量，一点没消除。

修法（二选一，推荐第一个）：

- **从已验证的 endpoint 主机名反解平台码**：`^([a-z0-9]+)(?:-k8s)?-sgp\.lol\.qq\.com$` 取第一组大写，`hn10-k8s-sgp.lol.qq.com` → `HN10`。这样完全不依赖那个 404 的接口，而且平台码和网关地址天然自洽，不会出现"平台码和 host 对不上"。
- 或改读整个命名空间对象 `/lol-platform-config/v1/namespaces/LoginDataPacket` 再取 `platformId` 键；但这条仍要赌接口存在，不如上面那条。

### 3.3 缺陷二：`builtin_matches` 在唯一需要它的场景下恒为 false

`sgp_gateway.go:127-131` 的顺序是：

```go
builtin, known := p.serverBase(result.Platform)
result.BuiltinKnown, result.BuiltinMatches = known, known && builtin == result.Base
if result.Base == "" && known {
    result.Base, result.Source = builtin, "builtin-fallback"
}
```

比较发生在赋值**之前**，此时 `result.Base` 还是空串（因为 3.2 的分支没进），所以 `builtin_matches` 必然 false。它只有在 `source=="client"` 时才可能为 true——而那正是**不需要比较**的情况。

也就是说：**这个字段永远无法回答"内置表和客户端给的地址是不是同一个"**，而这恰恰是加它的唯一目的。这和 R81 那次"改记录文件的字段去骗校验"是同一类问题——判据要独立于被判对象重算，不能顺着赋值流程捡。

修法：把客户端解析出的 base 单独存一个变量，无论最终采用哪个来源都做比较；并且**把客户端返回的 host 记进日志**。host 不是凭据，没有脱敏必要，现在只记了内置表那个 host，等于比较的两边只看得见一边。

> 顺带：`sgp_gateway.go:142` 的 `if u, err := url.Parse(result.Base); err == nil && valid` 用的是**客户端 endpoint 的** `valid`，却去解析**内置表的** `result.Base`。当前巧合下能输出，但语义是错的，改比较逻辑时一并修掉。

### 3.4 这条还不能解释 401

同一个 base 上 `RANKED`、`SUMMONER` 都是 200，所以 `hn10-k8s-sgp.lol.qq.com:21019` 这个网关对本账号是对的。3.2/3.3 要修，是为了**把这个变量彻底钉死**，不是把它当成 401 的原因。

---

## 4. 已落地但本轮无法验证的部分

`gsmRosterSession(raw, target, partial)`（`overview_current_game_sgp.go:199-263`）已经做出了 `verified` / `displayable` 两档，主链路走 `displayableGSMRoster`，空敌方队伍和无效身份不再让整份数据作废。ADDENDUM 第 3 节要求的分档改造**确实做了**。

但本轮 7 次请求全是 401，`game_present` 恒 false，**一次 200 都没有**，所以这个改动有没有真的让自定义/人机局显示出来，这份日志给不出证据。需要你自己在局中（或开一把自定义）时看一眼自己的那张卡片。

---

## 5. 一个采集口径的小漏洞

`lcu_spectator_read_probe` 记录了 `presence_fields: ["championId","gameId","gameMode","gameQueueType","gameStatus","mapId","queueId","timeStamp"]`，`in_game_friend_count: 1`。

但 `social.go:19` 的白名单 `spectatorPresenceFieldNames` **本来就没有 `spectatorKey`**。所以这份日志**既不能证明也不能否认**好友 presence 里有没有 `spectatorKey`——而 `overview_spectate_test.go` 里的观战链路正是靠这个字段。白名单里加上它（只记存在与否，不记值）。

---

## 6. 下一轮要做什么

**不需要写新代码就能做的（优先）：**

1. 按第 2 节的六步跑一次探测矩阵，然后导出日志。这一步不需要你开游戏，只需要目标玩家正在对局中、对照玩家不在对局中。
2. 把 `0911-2326.jsonl` 第 16695、16977 行的 `allowed_methods` / `allow_header_present` / `content_type` 读出来（R82 工单 P0-1，至今没人做过；那两行是旧构建留下的 spectator 405，`Allow` 头当时就已经落盘了）。

**需要改代码的：**

3. 按 3.2 从 endpoint 主机名反解平台码，去掉对 404 接口的依赖。
4. 按 3.3 重算 `builtin_matches`，并记录客户端返回的 host；顺手修 `sgp_gateway.go:142` 的 `valid` 用错对象。
5. 把探测入口从设置页深处提到阵容查询失败的那张卡片上。
6. `spectatorPresenceFieldNames` 补 `spectatorKey`。

**验收判据：**

- 新日志里必须出现 `current_game_matrix_start` 且 `current_game_matrix_summary.cell_count >= 11`，否则本轮同样不算数。
- `sgp_gateway_resolution` 必须同时出现客户端 host 和内置 host 两个值，且 `builtin_matches` 在 `source=builtin-fallback` 时也有意义。
- 3.3 的比较逻辑要有变异护栏：把比较改回"赋值后再比"时必须有测试变红。
