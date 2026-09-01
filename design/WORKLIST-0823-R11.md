# WORKLIST 0823-R11（真机回归后的 13 项）

诊断人：Claude（只读调查，未改任何仓库文件）。执行人：GPT。
所有结论均带 `file:line` 证据；标注 **【已实测】** 的是我实际发了请求/跑了脚本验证过的。

行号基线：`web/champions.js` 1921 行、`web/gameplay.js` 3050+ 行、`web/champions.css` 902 行、`web/gameplay.css` 1008 行。改前先 `grep` 确认锚点，别盲信行号。

---

## 优先级总览

| # | 问题 | 性质 | 优先级 |
|---|---|---|---|
| A1 | CDragon 兜底路径拼错，图标全 404 | **一行 Go 的硬 bug** | **P0** |
| A2 | 图标失败时用"品质渐变 + mask"伪造 | 设计问题 | **P0** |
| A3 | 海斗战绩卡不显示海克斯 | 判据错 | **P0** |
| C1 | 打包漏注入 Riot Key | 构建流程 | **P0** |
| B1 | 排位卡无数据也渲染 `— LP` | 一行 | P1 |
| B2 | 排位卡召唤师技能空白 | 需埋点二选一 | P1 |
| A4 | 重进页面不重新加载 | 两道缓存闸 | P1 |
| E1~E5 | 对局内实时性 / 绝活哥 / 海克斯推荐 | 中等 | P1 |
| F | 符文页图标尺寸与副系对齐 | UI | P1 |
| G | 出装页重设计 + 四五六件装备 | **最大工作量** | P1 |
| H | 英雄详情页符文两套 / 核心装 / 梯度列表 | UI + 交互 | P1 |
| D2 | 英雄胜率改赛季口径 | **需要新做后端聚合** | P2 |
| D1 | 国服他人胜率 | **不是代码问题，先读日志** | P2 |

---

# A. 战绩卡（图一）

## A1【P0，先改这条】CommunityDragon 兜底路径多拼了一层 `assets`

**这是上一轮"LCU 404 回落 CDragon"没生效的真正原因。**

`main.go:646-655`：

```go
trimmed, ok := strings.CutPrefix(strings.ToLower(assetPath), "/lol-game-data/assets")
return []string{
    "/latest/plugins/rcp-be-lol-game-data/global/default" + trimmed,
    "/latest/game/assets" + trimmed,
}
```

海克斯图标的实际路径是 `/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Deft_large.png`——注意 **`assets` 出现了两次**。`CutPrefix` 去掉第一层后 `trimmed = "/assets/ux/cherry/..."`，第二个候选就拼成了 `/latest/game/assets/assets/ux/...`。

**【已实测】四个 URL 的真实返回：**

```
404  /latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/deft_large.png
404  /latest/game/assets/assets/ux/cherry/augments/icons/deft_large.png   ← 当前代码生成的，双 assets
200  /latest/game/assets/ux/cherry/augments/icons/deft_large.png          ← 正确形态
200  /latest/plugins/rcp-be-lol-game-data/global/default/assets/ux/cherry/augments/icons/deft_small.png
```

→ **`_large` 大图在两个候选上都 404**，前端必然走 mask 伪造分支（见 A2）。

**改法**：`communityDragonImagePaths` 复用已经写对了的 `communityDragonGameAssetPath`（`champions_structured.go:1025-1036`）的判据——去掉 `/lol-game-data/assets/`（**带尾斜杠**）后，若剩余部分以 `assets/` 开头就走 `/latest/game/` + 剩余；否则走 plugin 前缀。两个候选都要按这个规则生成，顺序建议 **plugin 优先、game 兜底**（小图在 plugin 上、大图在 game 上）。

**验收**：加一个 Go 单测，喂 `/lol-game-data/assets/ASSETS/UX/Cherry/Augments/Icons/Deft_large.png`，断言候选里含 `/latest/game/assets/ux/cherry/augments/icons/deft_large.png` 且**不含**任何 `assets/assets`。

## A2【P0】图标加载失败时不要伪造样式

用户原话："是自己伪造的样式"。确实是：

`gameplay.css:419-420`
```css
.arena-augment-icon > .game-icon.has-augment-mask::after {
  content: ""; position: absolute; inset: 4px; z-index: 2;
  background: var(--augment-fill, linear-gradient(180deg,#DCE4EE,#9AA7B8));
  -webkit-mask: var(--augment-mask) center/contain no-repeat; mask: var(--augment-mask) center/contain no-repeat;
}
.arena-augment-icon > .game-icon.has-augment-mask > img { visibility: hidden; }
```

`::after` 铺的是**品质渐变**，靠 `mask` 抠形；`:420` 把真图强制隐藏。只要 `--augment-mask` 没写进去、或 mask 资源 404、或小图不是纯 alpha 剪影，露出来的就是一整块金色/棱彩实心方块。触发点唯一：`gameplay.js:2721-2733` 的 `failed()` 回调。

同一套重复实现还在 `champions.css:92-96`（`--quality-fill`），由 `champions.js:1658` 触发，**要一起改**。

**改法（按此顺序）**：
1. 先修 A1，让 `_large` 真的能加载 —— 大部分图标会直接正常显示，压根走不到降级。
2. 降级链改成**只换图不换渲染方式**：`_large` 失败 → 直接把 `img.src` 换成 `_small`（普通 `<img>`，不加 `has-augment-mask`、不隐藏 img）。`_small` 是 Riot 官方白描图，直接显示即可，不要再拿品质渐变去染。
3. 两个 `_` 都失败 → 走既有的字母占位（`pendingCatalogIcon` 风格），**不要**画任何伪造图形。
4. 删除 `gameplay.css:417-420` 与 `champions.css:92-96` 的整套 mask/fill 规则，以及 `gameplay.js:2726-2727`、`applyRenderedMetricStyles` 里 `[data-augment-mask]` 那段（`gameplay.js:2763-2765`）。
5. `web/champions.test.cjs` 里现有的 mask 相关断言要同步删/改。

## A3【P0】海斗（ARAM Mayhem）战绩卡完全不渲染海克斯

`gameplay.js:1186` 把"是不是斗魂"和"有没有海克斯"耦合成同一个变量：

```js
const arenaPlacement = Number(subject.placement) > 0 && (match.modeGroup === "arena" || Number(subject.subteamId) > 0) ? Number(subject.placement) : 0;
```

`gameplay.js:1226-1228` 的 loadout 三元、`gameplay.js:1236` 的 `is-augments` class 全挂在它上面。海斗（queueId 2300/2400、gameMode `KIWI`、`modeGroup === "hextech-aram"`，见 `gameplay.go:1979-1988` / `gameplay.go:4017-4018`）**没有 placement/subteamId**，所以 `arenaPlacement` 恒为 0，`augmentIDs`（`gameplay.js:1218`）拿到了数据也被丢弃。

后端数据完全正常：LCU 路径 `gameplay.go:1632/1639`、SGP/Riot 路径 `riot_api.go:606-611/629`，JSON 字段 `gameplay.go:229` `augmentIds`。展开详情里能显示，因为那边用的是另一套判据（`gameplay.js:1519` `hasAugments = augmentIDs.length > 0`）。

**改法**：新增独立变量 `hasAugments = augmentIDs.length > 0`，loadout 分支与 `is-augments` 改用它；`arenaPlacement` 只保留"名次徽章"用途。

**附带（同批修）**：`MORE_MODE_OPTIONS`（`gameplay.js:851-861`）里 `hextech-classic` 只覆盖 `[2400]`，`filteredMatches` 的 `"aram"` 分支只匹配 `modeGroup === "aram"`（`gameplay.js:892`）、"特殊模式"只匹配 `"other"`（`gameplay.js:896`）→ **queueId 2300 的海斗对局在任何筛选下都查不到，只有"全部"能看见**。

## A4【P1】离开页面重进要重新加载

现在是**命中内存缓存**，两道闸：
- `gameplay.js:182` `if (tabReady(tab) && !tab.data && !tab.loading) loadOverview(tab); else renderOverview();`
- `gameplay.js:366` `if (tab.data && !force) { rerenderTab(tab); return; }`

`tab.data` 挂在模块级 `state.tabs`（`gameplay.js:24`），而 `app.js:1605-1610` 切页只改 `panel.hidden`、从不卸载脚本 → `tab.data` 永久存活。总览页也没有任何 `visibilitychange` / 定时兜底（`gameplay.js:3012` 与 `:2522` 都只覆盖 live 页）。

**改法**：`activateSection`（`gameplay.js:166-188`）进入 `overview` 时改为 `loadOverview(tab, true)`。建议加一个**最小重载间隔**（如 20s 内不重复拉，用 `tab.loadedAt` 判定），避免用户来回切页把 SGP 打爆。

---

# B. 排位卡（图一）

## B1【P1】没有 LP 数据时不要渲染占位

`gameplay.js:1180-1184`，当前条件只判队列是不是 420/440，无数据就渲染 `— LP`：

```js
: ranked
  ? '<b class="lp-delta is-unavailable" data-tooltip="...">— LP</b>'
  : ""
```

而 `lpDelta` 天然稀疏：`gameplay.go:686-689` 只对 `isCurrent` 玩家标注，`lp_tracker.go:275-283` 只在助手运行且捕获到结算事件时才写历史 → 历史对局、他人页签、韩服页签**100% 落进占位分支**。

**改法**：把 `:1182-1183` 那条 `ranked ?` 分支整个删掉，改成 `: ""`。`gameplay.css:323` 的 `.lp-delta.is-unavailable` 一并删除。

## B2【P1】排位卡召唤师技能不显示 —— 先埋点，两条路二选一

渲染分支本身是对的（`gameplay.js:1220-1225`：2 个召唤师技能 + 主系基石 + 副系，共 4 格）。"看不见"只有两种可能，界面上极像（都是灰块）：

- **路径 A（目录失败）**：`/api/gameplay/summoner-spells` 挂了 → `state.summonerSpells === null`（`gameplay.js:1598-1600`）→ `spellIconFigure` 在 `gameplay.js:2693` 返回 `pendingCatalogIcon` 灰块。而 `gameplay.js:1603` 的 `if (state.summonerSpells) rerenderCatalogViews();` 守卫意味着**失败后永远不重试、不重绘**。
- **路径 B（数据缺失）**：`subject.spell1Id` 没下发 → `gameplay.js:2692` 返回 `""` → 兜成 `emptyLoadoutSlot`。后端 `gameplay.go:224-225` 带 `omitempty`，SGP 侧字段名假设是 `summoner1Id`（`riot_api.go:346-347`），**仓库里没有任何测试 fixture 覆盖过这两个字段**，`riot_api.go:410-411` 的"结构一致"只是注释断言。

**改法**：
1. 先加埋点：`/api/gameplay/overview` 返回时记录一条 `overview_loadout_shape`，含 `spell1_present` / `spell2_present` 的计数；前端 `ensureSummonerSpells` 失败时记 `catalog_load_failed{catalog:"summoner-spells"}`。
2. 顺手修两个确定的缺陷（与二选一无关，都该修）：
   - `gameplay.js:222-225` 重连分支只调了 `ensurePerks(true)`，没配套 `ensureItems()` / `ensureSummonerSpells()`（对照断线分支 `:203-209` 是三个都调的）。
   - `gameplay.js:1600-1603` 目录加载失败后加退避重试。

---

# C. Riot API Key（图三）

## C1【P0】打包链路绕过了注入 key 的那一步

**【已实测】**用 `riot_api.go:56-60` 的固定派生密钥对二进制做穷举试解密（方法已用"自己加密再扫"自测通过）：

| 文件 | 结果 |
|---|---|
| `desktop/backend/loot-service.exe` | 20 万次候选，**无任何可解密密文** |
| `dist/desktop/win-unpacked/.../backend/loot-service.exe` | 同上（与前者字节数完全一致） |
| `dist/Deep Legends.exe` | 同上 |

`riot_key.local.txt` 本身有效（42 字节，`RGAPI-` 开头），所以不是 key 文件的问题。

交叉验证证明"用了 ldflags 但漏了 `-X`"：PE Subsystem = 2 (GUI) → `-H=windowsgui` 传了；buildinfo 里有 `-trimpath=true` 且 `vcs` 缺失 → `-buildvcs=false` 也传了。

**断点**：注入只发生在 `build-desktop-windows.ps1:32/67-73`。开发机是 macOS（仓库里就有 `local-verify/deep-legends-mac-arm64`），`.ps1` 跑不了。产物特征与 `README.md:127` 那条手敲交叉编译命令完全吻合：

```
GOOS=windows ... -ldflags="-s -w -H=windowsgui -X main.riotAPIKeyCipher=你的密文" ...
```

**这条命令里 `你的密文` 是占位符，粘过去不替换/删掉，key 就没了，而且没有任何报错。**随后单独跑 `npm run pack:win`，electron-builder（`desktop/package.json:11-12/24/32`）**不参与 Go 编译**，只是把磁盘上现成的 exe 原样打进 asar.unpacked。时间戳吻合：backend exe 01:18 → win-unpacked 01:24。

**改法（三选一或全做，建议至少做 1+2）**：
1. **写一个跨平台构建脚本**（`build-desktop.sh` 或 npm script），复刻 `.ps1` 的逻辑：读 `riot_key.local.txt` → `go run . -encrypt-riot-key` → 拼进 ldflags → `go build` → `npm run pack:win`。**macOS 上必须用它，不许手敲 go build。**
2. **加硬校验**：`desktop/package.json` 增加 `beforePack` 钩子，运行 `desktop/backend/loot-service.exe -self-check-riot-key`（新增一个 flag，无 key 时 exit 1），无 key 直接让打包失败。现在 `build-desktop-windows.ps1:70-72` 只 `Write-Warning` 就放行，是静默失败面。
3. `README.md:127` 那条命令里的 `你的密文` 改成显式的 `$(go run . -encrypt-riot-key "$(cat riot_key.local.txt)")`，消灭占位符陷阱。

**顺带**：`.github/workflows/ci.yml` 用 `-Version "0.11.1"` 而 `desktop/package.json:3` 是 `0.11.2` → 会命中 `build-desktop-windows.ps1:84-86` 的 `throw`；且该 workflow 被 `.gitignore` 忽略、压根没提交（git log `f35d2b1`）。CI 目前是死的，要么修要么删。

**注意**：Riot Personal Key **24 小时过期**。即使注入成功，第二天也会 403。绝活哥符文（E4）、韩服玩家查询、韩服高手对局详情三处都吊在这个 key 上。这是产品级约束，建议在设置页显式展示"Riot Key 状态 + 过期时间"，而不是让三个功能各自静默消失。

---

# D. 玩家查询（图二）

## D1【P2】国服他人"胜率暂不可用" —— 不是代码问题，下一步是读日志

链路无缺陷，**不存在 self/other 的代码分叉**：`gameplay.go:1389` 无论 `isCurrent` 真假都调同一个 `rankedStatsOn`、同一个 endpoint（`sgp_api.go:604`）、同一个 token；`isCurrent` 只透传成埋点字段 `isSelf`（`sgp_api.go:617`）。解析结构体 `sgpRankedQueue`（`sgp_api.go:577-585`）**有** `Losses` 字段。

抑制点是 `gameplay.go:1482-1493` `verifiedRankWinRate`：`wins > 0 && losses == 0` 时返回 -1，前端 `gameplay.js:768` 据此显示"胜率暂不可用"。这是**刻意设计**（测试硬约束：`gameplay_test.go:333/336/360/380`）。

`design/ranked-winrate-lp/README.md:19-72` 已有真机日志定量分析：2418 条 `wins>0 && losses==0`、42 条带完整负场，且带完整负场的里面**包含非本人玩家**（472/429、410/372）→ "SGP 对非本人一律不返回负场"这个假说已被日志推翻。`body_bytes` 差 200–290 字节 → 缺的是**一整块结构**而非一个键。

`sgp_ranked_stats_shape` 埋点（`sgp_api.go:615-620`，含 `is_self` / `privacy` / `body_bytes` / `queue_keys`）**已经落地**。

**行动**：先让用户跑一次真机、把 `sgp_ranked_stats_shape` 事件（特别是 `queue_keys` 是否含 `losses`、`privacy` 取值）导出来。**在拿到这份日志之前不要改任何代码。**

唯一现在就能做的：`gameplay.js:770` 的"胜率暂不可用"是死字符串，后端算好的 `capability.Detail = "SGP 未返回排位负场，胜率暂不展示"`（`gameplay.go:1417-1420`）被丢弃了（`gameplay.js:760` 只在 `state === "unsupported"` 时展示）。改成把 Detail 挂成 tooltip。

## D2【P2】英雄胜率：最近 20 场 → 本赛季全部排位

**这是新功能，不是修 bug。** 当前状态：

- 样本 = 一页战绩，默认 20、UI 最大 50（`gameplay.go:22-23`、`gameplay.js:79`）。
- `championStats`（`gameplay.go:1738-1775`）**不筛队列**——ARAM、匹配、斗魂全算进"英雄胜率"，而卡片标题（`gameplay.js:778` "最近 N 场"）却和上方"排位 · 当前赛季"（`gameplay.js:772`）并排，读起来像赛季数据。
- 翻页永远不更新它：`gameplay.go:701-704` `if begIndex > 0 { return }` 在算 `ChampionStats` 之前就 return；`gameplay.js:399` 前端也只合并 `matches`。
- **代码里完全没有"赛季"概念**：全仓 `*.go` 搜 `season|splitId|seasonId|S25|S26` 只命中一个测试名。`web/gameplay.js:772` 的"当前赛季"是死文案。

**上游能力**：
- Riot Match-V5 `/lol/match/v5/matches/by-puuid/{puuid}/ids` 官方支持 `queue` / `type` / `startTime` / `endTime` —— **本仓库一个都没用**（`riot_api.go:467` 只带 `start` + `count`）。
- SGP `/match-history-query/.../SUMMARY` 只有 `startIndex` + `count`（`sgp_api.go:488-491`），没有 queue/season 参数，**只能靠翻页 + 本地过滤**。

**方案（分两步，第一步就能交付）**：
1. **先把口径说对**：`championStats` 加 queueId 过滤（420/440），卡片标题从"最近 N 场"改成"最近 N 场排位"。这一步零新增请求。
2. **再做赛季聚合**：新增 `seasonStartAt` 常量表（S26 起始时间戳，按年递增，明年 S27），后端新增一个**按赛季分页扫描 + 落盘缓存**的聚合器：
   - 首次进入时后台翻页拉到赛季起点为止（SGP 每页 20，`sgp_api.go:76`），结果按 `accountHash` 落盘（复用 `storage.go` 的加盐哈希，别存 PUUID —— 见 `deep-legends-lp-history-puuid` 那条历史教训）。
   - 后续只增量拉最新的、直到撞上已缓存的 gameId。
   - 输出 `{championId, games, wins, kills, deaths, assists}`，前端展示 胜率 / KDA / 场次。
   - **必须做进度反馈**（"已统计 N 场"），否则首次要拉几百场、用户会以为卡死。

---

# E. 对局内（图九）

## E1【P1】15s → 事件驱动（管道已通 3/4，只差最后一跳）

**现有管道**：LCU WebSocket（`lcu_events.go:38/47`，订阅 `OnJsonApiEvent`）→ `connection_manager.go:99-113` 识别 `/lol-gameflow/v1/gameflow-phase` → `broadcastEvent("gameflow:" + phase)` → SSE `/api/events`（`live_updates.go:39-73`，`main.go:248`）→ 浏览器 `EventSource`（`app.js:2372-2376`）→ `deep-legends:gameflow` CustomEvent（`app.js:2381`）。

**缺的最后一跳**：`gameplay.js:3044` 收到事件后**只调 `updateBeacon` 点灯，不调 `loadLive()`**。

**粒度也不够**：`lcuEventRefreshScope`（`lcu_events.go:92-110`）的白名单里**没有 `/lol-champ-select/v1/session`**，所以"预选/锁定英雄"这类同 phase 内的变化零事件，只能靠轮询发现。

**改法**：
1. `lcu_events.go:92-110` 增加 champ-select session 分支，`connection_manager.go` 广播 `champselect:changed`（**必须去抖**，该事件极高频；已有 `eventDebounceInterval = 900ms` 常量在 `connection_manager.go:13`，建议对 champ-select 用 300ms）。
2. `gameplay.js:3044` 收到 `gameflow:*` / `champselect:changed` 时调 `loadLive(true)`。
3. `live_updates.go:80-82` 的订阅者 channel 缓冲只有 4 且**满则丢**，接高频事件前要放大到 32 或改成"合并最新一条"。
4. 轮询保留为兜底，`normalizeLiveInterval`（`gameplay.js:81`）白名单 `[10,15,30,60]` 加 `3`、`5`，HTML 的 option 同步加。**默认值建议保持 15**——事件驱动上线后轮询只是保险丝。
5. 注意 `gameplay.js:1806-1808` 是在请求 finally 里排下一次，真实周期 = 间隔 + 请求耗时；改小间隔前要先确认 `/api/gameplay/live` 的 P95 耗时（当前超时预算 45s，`gameplay.js:1795`）。

## E2【P1】右侧推荐面板迟迟不出现

要"齐"需要 **3 段串行网络往返**，且第一段还被轮询节拍卡着：

1. `/api/gameplay/live`（`gameplay.js:1795`）拿 `available` + `championId`
2. 在第 1 步的 finally 之前才发 `/api/gameplay/recommendations`（`gameplay.js:1801/1870`，20s 超时，后端 15s ctx + `gameplay.go:2049` 抓 OP.GG/Hexdata）
3. 独立的 `ensurePerks/Items/SummonerSpells`

**最致命的一环**：`gameplay.go:2380-2399` —— 国服在英雄选择早期 `/lol-gameflow/v1/session` 会失败（代码注释 `gameplay.go:2382-2388` 已经承认这点），此时 `queueId=0 / mapId=0 / gameMode=""`。而 `gameplay.go:2423-2431` 的 champ-select 补救**只补 `rawPlayers`，不补 queueId/mapId/gameMode**。

连锁反应：
- `liveAugmentRecommendationSource`（`gameplay.js:1848-1857`）返回 `""` → `gameplay.js:1868` 不带 `source=mayhem` → 后端 `resolveGameplayRecommendationMode(0,"",0)` 落到 `default: "ranked", IsFallback = true`（`gameplay.go:2007-2009`）→ **返回召唤师峡谷的符文和出装**。
- `target.key` 含 queueId（`gameplay.js:1832`），queueId 从 0→2300 时 key 变了 → 已缓存的推荐作废 → **再来一次 15-20 秒往返**。

**改法**：
1. `gameplay.go:2423-2431` 用 champ-select session 补齐 queueId/mapId/gameMode（champ-select 响应里有队列信息；实在没有就查 `/lol-lobby/v2/lobby`）。
2. `state.section !== "live"` 时 `scheduleLiveRefresh` 直接 return（`gameplay.js:2522`）→ **没有后台预热**。收到 `gameflow:ChampSelect` 事件时即使不在对局页也预取一次。
3. `gameplay.js:1832` 的 `target.key` 去掉 queueId，或在 queueId 从 0 变为真值时做缓存迁移而非丢弃。

## E3【P1】绝活哥符文消失

`specialist_runes.go:52-55`：**无 Riot Key 时返回 HTTP 200 + 空数组 `[]`**，不是错误。前端 `gameplay.js:1896-1898` 把 `[]` 当成功缓存，于是显示"暂无可核验的韩服绝活哥符文"（`gameplay.js:2206`）——用户看到的就是"消失了"。

第二个消失点：`gameplay.js:2192` + `gameplay.js:2000-2002`，section 本身被 `queueId === 420 || 440` 门禁。而 queueId 在 ChampSelect 早期常为 0（见 E2）→ **排位对局前几秒绝活哥 section 先消失、后出现**。

数据面也天然稀疏：2 名玩家 × 最近 8 场 × 必须命中同英雄 × 符文必须完整（`specialist_runes.go:118-154/198-200`），常态就是 0-2 条。并发信号量只有 1（`riot_api.go:156`）。

**改法**：
1. `specialist_runes.go:52-55` 改成返回 `{"reason":"riot-key-missing","runes":[]}` 之类的结构，前端据此显示"未配置 Riot Key"而不是"暂无样本"。**别再让"没配置"和"没样本"长得一样。**
2. E2 修完后 queueId 的抖动会一并消失。

## E4【P1】海斗对局内"先推符文、过一会才变海克斯"

不是切换，是**页签集合被重建**。两个信号，谁晚谁决定：

| 信号 | 决定什么 | 证据 |
|---|---|---|
| `data.queueId/gameMode/mapId` | `augmentSource`、`target.key`、`source=mayhem` 参数 | `gameplay.js:1848/1832/1868` ← `gameplay.go:2380-2399` |
| `payload.hasAugments` | 海克斯页签是否存在、符文页签是否消失 | `gameplay.js:2015-2016` ← `gameplay.go:2102` ← `champions_structured.go:140` |

`recommendationCapabilities`（`gameplay.js:2004-2010`）在 payload 为空时默认 `hasRunes=true / hasAugments=false` → 首帧必然是"符文"。`gameplay.js:2050-2051` 还会把 `"runes"` **写回 `state.recommendationTab`**，所以海克斯页签出现后也不会自动跳过去。

另外 `gameplay.js:2073` 的 fallback 是 `augmentSource || "arena"` → 若 `hasAugments=true` 但 `augmentSource` 还是 `""`，会**按斗魂样式渲染海斗数据**（单列 + "OP.GG 斗魂竞技场选用率排序"字样）。

**改法**：
1. `recommendationCapabilities` 用 `liveAugmentRecommendationSource(data)`（同一函数里 `gameplay.js:2046` 已经算好了）兜底 `hasAugments`，首帧就能判出是海斗。
2. 页签集合变化时，若 `state.recommendationTab` 不在新集合的"推荐首选"位置且用户没手动切过，自动跳到 `tabKeys[0]`。
3. `gameplay.js:2073` 的 `|| "arena"` 兜底删掉，`augmentSource` 为空时不渲染而不是错渲染。
4. `gameplay.js:2023` 的 fallback 提示只覆盖 `resolvedMode === "aram"`；`resolvedMode === "ranked"`（queueId=0 的早期态）时 `isFallback` 也是 true 却没有任何提示，**用户会看到一份召唤师峡谷出装而不自知**——补上。

## E5【P1】对局内海克斯推荐对齐英雄详情页

用户要求：**样式与英雄详情页一致、推荐 12 个、按 S/A 排序、带海克斯说明提示、装备与技能也用海斗英雄详情页的数据**。

现状差异（两套完全独立的渲染）：

| 维度 | 对局内 `renderLiveAugmentRecommendations`（`gameplay.js:2122-2151`） | 详情页 `renderRecommendedAugments`（`champions.js:1269-1309`） |
|---|---|---|
| 条数 | arena `slice(0,12)`；hextech 3 品质 × `slice(0,4)` | 固定 `slice(0,9)` |
| **S/A 档徽章** | **无**（`:2132-2138` 从不读 grade/tier） | **有**（`champions.js:1014-1016` + `:117-126` 分位回退） |
| 排序 | 无二次排序 | 无二次排序 |
| 说明 tooltip | 有（`wrapAugmentIcon`，`gameplay.js:2095-2102`） | 有（`renderAssetButton` → `assetTooltip`） |
| 目录二次富化 | **无** | **有**（`augmentMetaForAsset`，`champions.js:1273/1318-1328`） |
| 低样本标记 | 无 | 有（`games < 100` → `.is-low-sample`） |

**底层数据是同源的**：两条路都走 `provider.loadDetail(mode, slug, ...)`，同一个 `championMetricRow`。差异纯在上层裁剪与渲染。

**改法**：把 `renderLiveAugmentRecommendations` 换成复用 `champions.js` 那套卡片（上一轮已经把海斗/斗魂统一到 `renderArenaOptionCard` 了，这次是第三处复用）。具体：
- 条数改 12（`slice(0,12)`）。
- 排序：按 `augmentGrade` 档位 → `score` → `games` 排。
- 说明：后端 `hydrateMayhemAugmentCopy`（`hexdata.go`，上一轮新增）已经会补齐说明，`/api/gameplay/recommendations` 这条路要确认也走到了 `decorateHexdataAugments`。
- **抽公共模块**：`renderArenaOptionCard` 目前在 `champions.js`，对局内在 `gameplay.js`，两个文件独立加载。建议抽到 `web/augment-card.js` 之类的共享文件，用 `window.deepLegendsAugmentCard` 暴露（参考现有 `window.deepLegendsMatchCards` 的做法，`champions.js:1064`）。
- 装备与技能：确认 `gameplay.go:2049` 的 `loadDetail` 用的 mode 与详情页一致（E2 修完 queueId 后自然一致）。

---

# F. 符文页 UI（图四）

## F1 优劣势对抗是"—"

不是渲染 bug，是 `hero.strongAgainst/weakAgainst` 为空数组（兜底在 `gameplay.js:2181` 行末的 `|| '<span class="muted">—</span>'`）。

三个可能，按可能性排序：
1. **模式不支持**：`HasCounters` 只在 `ranked` 与 `nexus-blitz` 为真（`champions_structured.go:138/143`），ARAM / 斗魂 / 海斗 / 无限火力**全部没有 counters**（`:139-142`）→ 这些模式下必然是"—"，是设计如此。
2. **英雄目录未就绪**：`structuredCounters`（`champions_structured.go:621-631`）里 `meta.ID == 0` 就丢弃整行，catalog 未 load 时会静默变空数组。
3. 样本门槛 `value.Play <= 0`（`:628`）。

图四是**排位英雄（瑞兹·下路）**，属于 ranked → 应该有 counters。**先加埋点**：`structuredCounters` 里记一条 `counters_shape{in, out, dropped_no_meta, dropped_no_play}`，跑一次真机再定。

**另一个确定的缺陷**：`gameplayRecommendationMatchup`（`gameplay.go:1912-1915`）**只带 championId + championName，丢掉了上游已有的 `WinRate` 和 `Games`**（`champions.go:328-336` 有这两个字段，`champions_structured.go:631` 也算好了）。所以对局内的对抗图标无法显示胜率/场次，而英雄详情页的 `renderCounters` 能显示。→ 补上这两个字段。

## F2 符文图标尺寸统一 + 副系顶部下移一行

**当前实测的尺寸**（`gameplay.css`）：

| 元素 | 生效行 | 尺寸 |
|---|---|---|
| 主系基石 / 小符文 / 属性碎片 | `:771` + `:772` + `:776` | 全都是 `var(--rune-icon-size)` = **32px** |
| 主系/副系**系别图标** | `:731` | `var(--rune-style-icon-size)` = **30px** |

**注意**：CSS 里根本不存在"基石大于小符文"的规则，`renderRuneOption`（`gameplay.js:2246-2254`）对基石和小符文输出的是**完全一样的 button**，没有任何区分标记。用户感觉系别图标"大"，是因为它 `object-fit: contain`、无边框无底色（`:731-733`），而符文是带底色的圆形，视觉重量不同。

**改法**：
1. `gameplay.css:681` 把 `--rune-style-icon-size` 设成与 `--rune-icon-size` 相同（含 `:994`、`:1002` 两处响应式覆盖），并给系别图标加与符文一致的圆形底（或反过来，两者都去掉底色）——目标是"每个符文图标一样大小"。
2. **副系顶部图标下移一行**：现在副系列的 DOM 顺序是 `<header>`（第 1 行 36px）→ `.unified-rune-row.is-spacer`（第 2 行）→ 3 个 slot 行。把 `gameplay.js:2239` 里 header 与 spacer 的**顺序对调**，让 header 落到第 2 行（与主系基石行对齐）。
3. **注意三处响应式会把 header/spacer 设成 `display:none`**：`gameplay.css:923-924`、`:966-967`、`:1005-1006` —— 对调顺序后要同步核对，别把系别图标一起藏了。
4. `.unified-rune-column` 的 `grid-template-rows: 36px repeat(4,40px)`（`:728`）在对调后要确认第 1 行是否还需要 36px。

---

# G. 出装与技能页重设计（图五 → 图六）

**这是本轮最大的一块，建议单独开一个分支做。**

## G1 现状问题

- **前端零 slice，条数完全由后端限额决定**（`gameplay.js:2308-2310` 的 `optionList` 是全量 `map`）：召唤师技能 5、出门装 5、鞋子 5（斗魂 7）、核心装 **15**、后期备选 **30**、棱彩 10（`champions_structured.go:451-460`）。核心装 15 条 route 铺在单列 grid（`gameplay.css:864`）里，这就是版式失控的直接原因。
- `.item-option-groups` 固定三等分（`gameplay.css:855`），三列内容量差异极大（5 条 vs 15 条），列高严重不齐。
- build 页签**没有英雄信息条**（`gameplay.js:2074` 直接返回 `renderBuildRecommendation`），与符文页不一致。

## G2 新版布局（上半区 2×2）

```
┌──────────────────────────┬────────────────────────────────────────┐
│ 召唤师技能（胜率前 2）     │ 鞋子（胜率前 2）                        │
├──────────────────────────┼────────────────────────────────────────┤
│ 出门装（胜率前 2）         │ 技能加点（QWER 四按钮 + 15 级阶梯）      │
└──────────────────────────┴────────────────────────────────────────┘
   左列窄 minmax(0,1fr)          右列宽 minmax(0,1.6fr)
```

- `grid-template-columns: minmax(0,1fr) minmax(0,1.6fr)`，两行。右列更宽是为了让技能加点的 15 级阶梯（`gameplay.css:849` `repeat(auto-fit,minmax(28px,1fr))`）不被挤压。
- **"胜率前 2"是按胜率降序取前 2，不是按选取率**。现在的排序是上游给的（选取率序），需要前端 `sort((a,b) => b.stats.winRate - a.stats.winRate).slice(0,2)`。
- ⚠️ **这里我做了一处推断**：用户说"这三个和技能加点放在上面的区域，左右两个区域，上下两行 2x2 那种，但是技能加点放右边应该要宽点"，没有明说四个格子各放什么。我按"技能加点在右下、右列更宽"来排。**GPT 实现时把四个格子的内容做成一个易改的常量数组，方便用户看到后一句话调整。**

## G3 核心装备区（下半区，对齐 OP.GG）

```
┌────────────────────────────────┬──────────┬──────────┬──────────┐
│ 核心装备（最多 5 条出装路线）    │ 第四件    │ 第五件    │ 第六件    │
│  ○ › ○ › ○   80.76% 8,677场 54.32%│  最多3    │  最多3    │  最多3    │
│  ○ › ○ › ○    7.48%   804场 51.99%│           │           │           │
│  ...                            │           │           │           │
└────────────────────────────────┴──────────┴──────────┴──────────┘
   左侧 flex: 1                      右侧三列竖向并列，整体等高、自适应
```

- 左 5 条、右侧每列最多 3 条。
- **两边等高、且要自适应**：`align-items: stretch` + 右侧容器 `display:grid; grid-auto-flow: column; grid-auto-columns: minmax(0,1fr)`。**有的英雄 OP.GG 只给了第四、第五件**（实测瑞兹 depth 6 只有 5 条且样本只有 3-4 场），所以右侧列数必须按实际有几组动态决定，不能写死 3 列。

## G4 数据从哪来 ——【已实测】必须新增一条抓取链路

**OP.GG 的 JSON API 给不了四五六件装备。**【已实测】`GET https://lol-api-champion.op.gg/api/KR/champions/ranked/13/ADC?tier=emerald_plus` 的 `data` 只有这些 key：

```
summary, summoner_spells, core_items(15), mythic_items(0), boots(5),
starter_items(2), last_items(28), rune_pages(5), runes(5),
skill_masteries(1), skills(5), skill_evolves(0), trends, game_lengths, counters(0)
```

`last_items` 是**扁平的"最终出装里出现过的单件装备分布"**（28 条，每条 `{ids:[3157], play, win, pick_rate}`），**不是按 4/5/6 顺位分桶**。

**但 OP.GG 的网页（图六那个 URL）里有。**【已实测】抓 `https://op.gg/zh-cn/lol/champions/ryze/build?patch=16.16&region=kr&tier=emerald_plus&type=ranked`，RSC flight 里有 `"tr","depth_{4|5|6}_item_{i}"` 的行，我用正则抽出来的结果与图六**逐位一致**：

```
depth 4 (5 条): 3157 53.55% 2056场 | 3089 57.29% 1138场 | 3135 49.36% 470场 | 3110 55.15% 388场 | 2522 60.89% 248场
depth 5 (4 条): 3157 53.3% 227场 | 3135 52.24% 201场 | 3110 59.78% 92场 | 3102 46.55% 58场
depth 6 (5 条): 4629 75% 4场 | 3110 0% 3场 | 4645 0% 3场 | 3135 0% 3场 | 3089 100% 3场
```

抽取要点（仓库里已有 `decodeNextFlight` + `extractBestArray` 机制可复用，见 `champions_structured.go:309-321` 的 `loadTopPlayers`）：
- 行 key 正则：`"tr","depth_(\d)_item_(\d+)"`
- 每行紧随其后取 `"metaId":(\d+)`、`"children":\[([0-9.]+),"%"\]`、`"children":"([0-9,]+) 场"`
- **注意 HTML 里是转义过的**（`\"`），解析前要先 unescape。

**后端要做的**：
1. 新增 `loadOPGGItemDepths(ctx, championSlug, position, tier)`，产出 `map[int][]championMetricRow`（key = 4/5/6）。
2. 填进 `championBuildSections.FourthItems / FifthItems / SixthItems`（`champions.go:310-312`）—— **这三个字段现在是死字段，从未被赋值**。
3. **删掉 `splitLateItems`（`champions_structured.go:603-609`）**：它是当初为了填这三个字段写的，`parts[index%3]` 轮转分三份的做法在语义上是错的，而且全仓**零调用点**（含测试）。留着会误导。
4. **`champions.go:1052-1057` 的资产装饰列表遍历了 Fourth/Fifth/Sixth 却漏了 `LateItems`** —— 新链路接上后要确认三个字段真的被装饰，否则 tooltip 会退化成裸 item ID（`champions_structured.go:531` `Name: strconv.Itoa(id)`）。
5. 这是一个**新的 HTML 抓取源**，务必：走已有的 `p.fetch` 限流、加长缓存（装备分布日更即可）、shape 校验失败时**降级为不展示右侧三列**而不是整页报错。

## G5 英雄详情页同一块（图七下半部分）

`champions.js:1480` 正在读那三个死字段：

```js
const late = [build.fourthItems || [], build.fifthItems || [], build.sixthItems || []];
```

→ `champions.js:1492-1498` 两段"续接后期装"的循环恒空转 → 每条路线只剩 `core.assets` 的 3 件，`ADC_ITEM_ROUTE_LIMIT = 7` / `DEFAULT_ITEM_ROUTE_LIMIT = 6`（`champions.js:5-6`）形同虚设，`champions.css:478` 的 `grid-auto-rows: minmax(84px,1fr)` 也白留了高度。

G4 做完后这里自然就有数据了。用户要求："OPGG 推荐了几件装备就展示几件，这几个装备竖向并列排列" → 与 G3 同一套组件，**建议 G3/G5 用同一个渲染函数**（注意 `renderConfigOption` 在两个文件里是**同名但签名不同**的两个函数：`gameplay.js:2382` 收 `{ids}`，`champions.js:1503` 收 `{assets}`，要先统一数据形态才能复用）。

---

# H. 英雄详情页与梯度列表（图七、图八）

## H1【P1】符文推荐两套同时展示

`champions.js:1353` 无条件渲染全部面板：

```js
const boards = pages.map((page, index) => `<div class="rune-board-panel" data-rune-page-panel="${index}">${renderRuneTree(page)}</div>`).join("");
```

点击 tab 的处理（`champions.js:1737-1747`）**只切 tab 的 class，既不重绘也不改 DOM 可见性**（`return` 在 `:1746`）。`.rune-board-panel`（`champions.css:433`）没有任何 `[hidden]` / `display:none` 规则，且带 `flex: 1` → N 个面板就撑 N 份高度。`state.runePage` 对显示结果实际上是死状态。

⚠️ **这个行为被测试锁死了**，改的时候必须同步改：`web/champions.test.cjs:1328-1329`
```js
assert.doesNotMatch(script, /class="rune-board-panel"[^\n]+ hidden/);
assert.doesNotMatch(script, /panel\.hidden = Number\(panel\.dataset\.runePagePanel\)/);
```

**改法**：`champions.js:1353` 只渲染 `pages[active]` 一个面板；tab 点击时 `state.runePage = index` 后走 `render()`。**用户要求"一套时高度要加大点，不然右边放不下"** → 单面板后 `champions.css:483` 的 `--rune-option-size: clamp(28px,5.8cqi,38px)` / `--rune-row-h: clamp(40px,7cqi,52px)` 可以整体上调；右列 `.loadout-side`（`champions.css:434`）被左列撑高，左列变高后右边的召唤师技能+技能加点两张卡就放得下了。

## H2【P1】核心装备接入四五六件

见 G5。

## H3【纠正前提】"场次最多的玩家"已经是 5

用户要求"增加到 5 个"，但**代码里前后端都已经是 5**：
- 前端 `champions.js:1580` `const visible = players.slice(0, 5);`
- 后端 `champions_structured.go:322` `make([]championTopPlayer, 0, 5)` + `:337-339` `if len(result) == 5 { break }`
- CSS 无额外裁剪（`champions.css:562/566`）

所以线上只看到 3 名，只可能是 **OP.GG 专家榜 flight 解析出的行数本身 < 5**（`loadTopPlayers`，`champions_structured.go:309-321`，解析失败直接 `return nil`）**或者跑的是旧构建**。

**行动**：先加一条埋点记录 `loadTopPlayers` 实际解析出几行；**不要盲目去改那两个 5**。

## H4【P1】梯度列表选中后滚回顶部

海斗 `selectMayhemChampion`（`champions.js:467`）与斗魂 `selectArenaChampion`（`champions.js:578`）都直接调 `render()` → `renderWorkspace()` → **`champions.js:669` `root.innerHTML = ...` 全量重写**，把 `.champion-table-scroll`（真正持有 scrollTop 的元素，`champions.css:115/215` 的 `overflow:auto`）销毁重建，新节点 scrollTop 天然为 0。

更糟：**每次选中会触发多次全量重绘** —— 海斗 2 次（`:467`、`:487`），斗魂 4 次（`:578`、`:601`、`:617`、`:632`，三条并发请求各自 `.finally` 一次），异步回包时还会再顶回去一次。

代码里唯一的滚动持久化 `state.listScroll`（`champions.js:59/384/642`）绑的是外层 `#app-scroll`，且被 `openDetail` 的 arena/mayhem 提前 return（`champions.js:373-380`）跳过，对列表内滚动完全无效。

**改法（按投入产出排序）**：
1. **最小改动**：在 `render()` 前后保存/恢复 `.champion-table-scroll` 的 `scrollTop`（存进 `state.listScrollInner`，`requestAnimationFrame` 里恢复）。能解决问题但每次重绘会闪一下。
2. **更好**：选中英雄时不要全量 `render()`，改成只重绘右侧详情面板 + 增量切换列表行的 `is-selected` class。工作量大但顺手解决了"多次重绘"的问题。
3. 无论选哪个，都要**给三条并发请求的 `.finally` 加上"只在数据真的变了才重绘"的判据**，别每个回包都刷一次全页。

## H5【P1】梯度列表选中样式

`is-selected` / `aria-current` **JS 侧有加**（`champions.js:1208-1211`），但：

1. **ranked 列表根本不参与**：`champions.js:1208` 的判据是 `(arena || mayhem) && ...`，ranked 传的 `metrics === true` → 恒 false。CSS 里也只有两条选中规则，都限定在 mayhem/arena 容器内。
2. **斗魂的选中底色被自家 `td` 背景吃掉**：`champions.css:716` 给 `.arena-champions .is-arena-table td` 加了 `background: color-mix(in oklab,var(--surface) 84%,transparent)`，绘制在 `tr` 之上，把 `:730` 的 `primary 14%` 稀释到约 2%，肉眼几乎看不见，只剩 `box-shadow: inset 3px 0 0` 的左侧竖条。
3. **海斗的选中态在 hover 下失效**：`.champion-table tbody tr:hover`（`champions.css:79`，特异度 0,2,2）压过 `.mayhem-champions tr.is-selected`（`:228`，特异度 0,2,1）。

**改法**：
- 选中规则改成挂在 `td` 上（或给 `td` 加 `background: transparent` 让 `tr` 透出来），特异度要高于 `:79` 的 hover。
- 给 ranked 列表也加上选中态（`champions.js:1208` 去掉模式限制）。
- 样式建议与现有风格一致：左侧 3px `var(--primary)` 竖条 + `color-mix(in oklab, var(--primary) 14%, var(--surface))` 底色 + 选中行 hover 时底色再加深一档。

---

# 验收清单

改完后逐条核对：

- [ ] A1 `communityDragonImagePaths` 单测通过，无 `assets/assets`
- [ ] A2 全仓搜不到 `has-augment-mask` / `--augment-fill` / `--quality-fill`
- [ ] A3 海斗战绩卡显示 4 个海克斯；`hextech-classic` 筛选能查到 queueId 2300
- [ ] A4 切走再切回总览页会发起新的 `/api/gameplay/overview`
- [ ] B1 无 LP 数据时排位卡上没有任何 LP 徽章
- [ ] C1 用新构建脚本产出的 exe，能扫出可解密的密文
- [ ] E1 champ-select 里改预选英雄，右侧推荐 ≤3s 跟着变
- [ ] E4 进入海斗英雄选择，**首帧**就是"海克斯"页签，没有符文页签闪现
- [ ] F2 符文页所有图标同尺寸；副系系别图标与主系基石行对齐
- [ ] G3/G5 瑞兹（ADC）能看到第四件 5 条、第五件 4 条、第六件 5 条，且两侧等高
- [ ] H1 符文区一次只显示一套
- [ ] H4 列表滚到底部选中英雄，滚动位置不动
- [ ] H5 三个模式的梯度列表都有选中态，且 hover 不会盖掉它

**测试要求**（沿用既有约定）：Go 侧 `go test ./...`，前端 `node --test web/champions.test.cjs`（**不能用 `node --test web/`**）。凡新增正则型断言，必须做变异测试确认是真护栏——上一轮实测正则型 JS 断言很容易是假护栏。
