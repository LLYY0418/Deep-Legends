# WORKLIST-R99：生涯头像 / 旗帜写入 + 职业选手种子账号（按 PUUID 追踪）

诊断人：Claude（只读诊断，未改仓库任何代码文件；本轮只新增 `design-mockups/r99*.html`、`design-mockups/r99.css`、`design-mockups/_shots/*.png` 三类设计稿产物）。
执行人：GPT。
日期：2026-09-16。
状态：**已过一轮独立对抗复核**，复核发现的 7 条阻塞问题已在本文中修正（修正点在正文里标了「★复核修正」）。

设计稿（真 Chromium 截图，1180/1240/1120 三档窗口，devicePixelRatio=2）：

- `design-mockups/_shots/r99-facade-cards.png` —— 生涯页新增「头像」「旗帜」两张卡
- `design-mockups/_shots/r99-icon-picker.png` —— 头像选择弹窗（5099 个图标怎么展示）
- `design-mockups/_shots/r99-banner-picker.png` —— 生涯旗帜选择弹窗

---

## 口径决定（不要自行更改）

1. **「旗帜」在客户端是三份互不相通的数据，本轮把三份都做，但必须分三行呈现，不许合并成一个控件。**
   - A「生涯旗帜」＝ `REGALIA_BANNER`，名人堂、赛事冠军、活动限定那些付费/活动旗帜都在这里，清单 `regalia.json`，写入走 loadout 槽位。**这是用户真正要的那一个。**
   - B「段位旗」＝ `/lol-regalia/v2/current-summoner/regalia` 的 `preferredBannerType`，**客户端枚举只有 `lastSeasonHighestRank` 和 `blank` 两个值**，表达不了名人堂旗帜，不要拿它去实现 A。
   - C「挑战旗帜配色」＝ `bannerAccent`，跟头衔、勋章同一个接口，现有「切换上赛季旗帜」按钮写死成 `"2"`（`profile_facade.go:790-791`）。
2. **头像范围＝全部 5099 个可选 + 「只看已拥有」开关 + 名称/拼音搜索 + 系列筛选**，口径与现有背景选择器一致（背景那条提示原文在 `web/suite.js:1378` 末尾：客户端接口不校验拥有，未拥有的可能下次登录被服务端还原）。用户已拍板要弹窗形态。
3. **Rookie 账号以源码内置的人工核对种子落地**（用户已拍板）。种子里**只写 Riot ID，绝对不许写 PUUID**：dpm.lol 与 op.gg 对同一账号给出的「PUUID」互相矛盾（两串都是 78 字符但完全不同），必有一方是站点自己混淆过的 ID。**唯一可信的 PUUID 来源是我们自己用内嵌 Riot Key 调 account-v1 by-riot-id 解析出来的那一个。**
4. **PUUID 锚点持久化：用户已拍板存进现有 `riot-identities` 磁盘缓存目录，TTL 30 天。**
   **★复核修正：准确口径是——复用同一个存储目录与同一套封装，新开两个独立的 key；现有 `account:` 键的 15 分钟 TTL 一个字都不许改。**
   - 新 key 一：`proseed:v1:<teamCode>/<lower(player)>` → `{"puuid":"..."}`，TTL 30 天。
   - 新 key 二：`account-by-puuid:<puuid>` → `riotAccount`，TTL 6 小时。
   - 该目录今天就已经在明文存搜索过的韩服玩家 PUUID（`riot_identity_cache.go:22-37` + `champion_cache.go:326-328` 白名单首条 `riot-identity-v1|`），本轮不是新开一类存储。
   - **★复核修正（阻塞级）：`riot-identities` 是 256 条 / 4MiB 的 LRU，`pruneDiskLocked` 按 mtime 淘汰且当前只保护 `hexdata-` 前缀。锚点写一次就不再刷新 mtime，注定是最老的那个、第一个被删。必须二选一并在代码里写死：①把 `proseed:` 前缀加进 protected 名单；②每次命中锚点后重写一遍刷新 mtime。** 不做这一步，「改名也能跟上」就是一句空话。
5. **先探测后实现。** P0 探测矩阵不跑完、结果不落到 `docs/r99-probe-results.md`，**不许开始写：P1 的拥有态部分（★复核修正：原来漏挡了，`owned` 这个字段名本身就是 R4 要探的东西）、P3（生涯旗帜写入）、P5-C（挑战配色）**。理由见 R82 教训（六格矩阵只测了对角线，改完还不行却不知道哪格坏）和 R95 教训（推断类功能先建真值回路再谈算法）。
6. **探测分两类：只读探测可以随连接自动跑一次；任何写探测必须用户手动点按钮触发，且一律用"把当前值原样回写"做零副作用验证。**
   **★复核修正：这条口径原来被我自己在 W4 里违反了（逐个试 1~12 且不恢复）。现在 W4 已改成合规写法，见 P0。任何写探测都不许留下用户没同意的状态变更。**
7. 新增的两条写操作必须同步进 `features.go` 的隐私声明，并让 `quality_test.go` 的断言真的能抓住漏写（见 P6）。

---

## P0 — 探测矩阵（只读 13 条请求 / 写 4 组，一次打完）

**证据：** 现成的一次性形状探测范式已经有了——`claimFacadeIdentityShape` 调用点 `profile_facade.go:142`、函数 `profile_facade.go:419-430`；`recordFacadeIdentityShape` 只记键名与状态码、不记值。`lcu_api.go:692` 证明 `/lol-inventory/v2/inventory/{type}` 与 `/lol-regalia/v3/inventory/{type}` 这两条路径在本仓库已经被真实调用过（SKIN_BORDER）。`clearFacadeEmotes`（`profile_facade.go:1014-1038`）证明 `GET /lol-loadouts/v4/loadouts/scope/account` + `PATCH /lol-loadouts/v4/loadouts/{id}` 这条链路在本仓库已经真实跑通过（表情轮盘）。

**要做什么：** 新增 `r99_probe.go`，函数 `recordR99SurfaceShape(ctx, client)`，挂在与 `claimFacadeIdentityShape` 相同的"每连接一次"闸门上。只读项自动跑；写入项由手动入口触发。

**★复核修正（循环依赖）：探测按钮不要挂在 P4 才做的新卡上。** 挂在**已经存在**的「展示清理」卡（`web/suite.js:1380-1387` 那一组动作的卡片）右上角，文案「检测客户端写入支持」，`[data-facade-probe]`。这样 P0 不依赖 P4。

### 只读探测（自动，一次/连接）—— 13 条请求

| 编号 | 请求 | 记录什么（只记这些） |
|---|---|---|
| R1 | `GET /lol-game-data/assets/v1/summoner-icons.json` | status、条目数、首条键名列表、`title` 非空条数 |
| R2 | `GET /lol-game-data/assets/v1/summoner-icon-sets.json` | status、条目数、首条键名列表 |
| R3 | `GET /lol-game-data/assets/v1/regalia.json` | status、条目数、`regaliaType` 去重枚举、`isSelectable==true` 条数、`isTencentOnly==true` 条数、**`id` 字段的 JSON 类型**（预期 string，见 P3 提醒） |
| R4 | `GET /lol-inventory/v2/inventory/SUMMONER_ICON` | status、数组长度、首元素**键名列表**、`ownershipType` 去重枚举、`owned==true` 条数 |
| R5 | `GET /lol-regalia/v3/inventory/REGALIA_BANNER` | status、顶层 key 数量与 **key 的形状**（纯数字串 / uuid / 其它，只记形状不记值）、value 键名列表、`isOwned==true` 条数 |
| R6 | `GET /lol-regalia/v3/inventory/REGALIA_CREST` | 同 R5 |
| R7 | `GET /lol-regalia/v3/inventory/REGALIA_BORDER` | 同 R5 |
| R8 | `GET /lol-inventory/v2/inventory/REGALIA_BANNER` | 同 R5（对照组：两个插件谁才是权威） |
| R9 | `GET /lol-loadouts/v4/loadouts/scope/account` | status、数组长度、`scope` 去重枚举、**`loadout` 对象的全部 key 名**（重点确认国服是否真有 `REGALIA_BANNER_SLOT`）、每个 slot value 的键名列表 |
| R10 | `GET /lol-regalia/v2/current-summoner/regalia` | 全部键名 + `bannerType` / `preferredBannerType` / `crestType` / `preferredCrestType` 的**字符串取值**（枚举名不是玩家标识，可以记） |
| R11a | `GET /lol-banners/v1/current-summoner/flags` | status + 数组长度 + 元素键名 + `theme` 去重枚举 |
| R11b | `GET /lol-banners/v1/current-summoner/flags/equipped` | status + 键名列表 |
| R11c | `GET /lol-banners/v1/current-summoner/frames/equipped` | status + 键名列表 |

R11a/b/c **全是 404 或全空就当场关闭这条备选链路**，结论写进文档。
**R5~R8 四格必须一次打完，不许只打其中一格。**（R82 教训：只测对角线导致"改完还不行"却定位不到哪格。）

### 写入探测（手动触发，零副作用）

| 编号 | 动作 | 记录什么 |
|---|---|---|
| W1 | **先 `GET /lol-summoner/v1/current-summoner` 现读 `profileIconId`**，再把这个值原样 `PUT /lol-summoner/v1/current-summoner/icon` | status（**预期 201，不是 200**）。`lcu.go:769` 判的是 `<200 \|\| >=300`，201 不会被当错误，但要有测试钉死。**★复核修正：绝对不许用 `a.summoner` 里的缓存值去回写——那份快照可能已过期，会真的把用户头像改回旧的。** |
| W2 | `PATCH /lol-loadouts/v4/loadouts/{id}`，`REGALIA_BANNER_SLOT` 用**当前已装备的同一个值**回写 | status、错误码、响应里该 slot 的键名 |
| W3 | 同 W2，但分别只传 `{inventoryType,itemId}` / 只传 `{contentId}` / 三个全传 | 三次各自的 status，判定 `contentId` 是否必填 |
| W4 | **★复核修正（阻塞级，原写法会破坏用户数据）：** ①`GET /lol-chat/v1/me` 读 `lol.bannerIdSelected` 记下原值；②**必须调用现成的 `writeChallengePreferences`（`profile_facade.go:799-874`）**、不许自己拼 body——它会 GET 现状把 `challengeIds` 和 `title` 一起带上，自己拼 body 会像 R66 那样把用户的头衔和挑战勋章一起清掉；③只试**一个**相邻取值，回读确认后**立刻写回原值**；④原值读不到就整个 W4 跳过 | 原值是否读到、试写 status、回读是否变化、是否成功恢复。**`/lol-chat/v1/me` 的响应只许取 `lol.bannerIdSelected` 一个字段，其余一律不落日志**（里面有玩家标识） |

**验收判据：**
- 一条测试用假 LCU 断言 R1~R11c 的**请求路径集合**与上表逐字一致（共 13 条），且断言探测产生的诊断事件里**不含** `puuid`/`summonerId`/`accountId`/`uuid`/`contentId`/`itemId`/`purchaseDate`/`gameName` 任一子串（照抄 `r66_test.go:193`、`r58_test.go:91`）。
- **★复核修正：挑战偏好的路径必须写成带尾斜杠的 `/lol-challenges/v1/update-player-preferences/`**——`lcu.go` 的路径规范化会加尾斜杠，仓库内 6 处调用与 5 个测试全用带斜杠版本，写错会让"路径逐字一致"断言直接失败。
- 写探测在**没有用户手动触发**时一次都不发：断言"仅连接、不点按钮"的场景下 PUT/PATCH/POST 计数为 0。
- W4 有一条专门断言：试写之后**最后一次请求是把原值写回去**。
- 结果归档到 `docs/r99-probe-results.md`。

**变异判据（必须 FAIL）：**
1. 把 R5~R8 里任意一条从探测清单删掉 → 路径集合断言 FAIL。
2. 把写探测的手动闸门去掉（连接即跑） → "未触发时零写请求"断言 FAIL。
3. 把 `/lol-chat/v1/me` 的整个响应体塞进诊断 → 隐私子串断言 FAIL。
4. 把 W4 的恢复写回删掉 → "最后一次请求是恢复"断言 FAIL。
5. 把 W1 的现读改成读 `a.summoner` 缓存 → 断言"W1 的第一条请求是 GET current-summoner" FAIL。

---

## P1 — 后端：头像目录（**拥有态部分要等 P0-R4 出结果**）

**证据：** 现成范式是 `loadFacadeSkins`（`profile_facade.go:1094-1125`：内存优先 → LCU 目录回退，成功缓存 15 分钟、空/失败 5 秒、client 变化即失效），出参裁剪的隐私护栏在 `r65_test.go:14`。大 JSON 必须用 `GetBytes`（`lcu.go:816-818`，上限 16MiB）——**不能用 `RequestJSON`，它只有 4MiB 上限**，而 `summoner-icons.json` 实测 1,949,389 字节。

**要做什么：**
1. 新增 `facade_icons.go`：`loadIconCatalog(ctx, client)`，读 `summoner-icons.json` + `summoner-icon-sets.json`，产出 `{ID int64, Title string, Year int, Legacy bool, ImagePath string, Disabled bool, Sets []string, SearchTerms []string}`。
   - **★复核修正：`summoner-icons.json` 里没有 `isTencentOnly` 字段**（那是 regalia.json 才有的）。要么别做头像的国服标记，要么明写推导式（`rarities` 含 TENCENT 且不含 riot）并在文档里注明是启发式。
   - **★复核修正：`rarities[].region` 大小写不统一**，实测 `TENCENT` 257 条、`tencent` 3 条。判定一律用 `strings.EqualFold`，fixture 里必须塞一条小写的。
   - `disabledRegions` 非空的（实测正好 501/1669/3504 三条）标 `Disabled`，界面置灰不可选。
   - `imagePath` 为空的（实测 86 条）直接跳过，不要产出空格子。
2. **拥有态（等 P0-R4）**：`GET /lol-inventory/v2/inventory/SUMMONER_ICON`，字段名以 R4 探测结果为准。拿不到拥有态时不要报错，走 `iconOwnershipUnavailable=true`，前端强制关掉"只看已拥有"开关（照抄背景那边 `profile_facade.go:170` + `web/suite.js:1356` 的 `skinOwnershipUnavailable` 处理）。**P0 未出结果前，这一段先恒定走 `iconOwnershipUnavailable=true`。**
3. **★复核修正（性能，别重蹈 R24/R25 的 24s/159MB）：5099 条目录不许塞进 `/api/facade/state`。** 单开 `GET /api/facade/icons`，前端打开弹窗时才拉一次并在前端内存里留着。`SearchTerms` 用 `championPinyinTerms`（`champions.go:1595-1608`）生成，**必须和目录一起进那份 15 分钟缓存，只算一次**，不许每次请求现算 5099 遍拼音。
4. **出参只许下发 `{id,title,year,legacy,disabled,owned,sets,searchTerms}`**，不许下发 `uuid`、`purchaseDate`、`ownershipType`、`contentId`。
5. **★复核修正：拥有态缓存的失效键必须包含 `client`**（照抄 `loadFacadeSkins` 里 `facadeSkinCatalogClient != client` 即失效的写法），否则切换账号后会显示上一个号的拥有状态。

**验收判据：**
- `TestR99IconCatalogProjectionDropsPrivateFields`：喂含 `uuid/purchaseDate/ownershipType/contentId` 的假 inventory，断言响应不含这四个子串。
- `TestR99IconOwnershipUnavailableFallsBackOpen`：inventory 返回 500 时，目录仍完整且 `iconOwnershipUnavailable==true`。
- `TestR99IconCatalogRarityRegionIsCaseInsensitive`：fixture 里放小写 `tencent`，断言被正确识别。
- `TestR99IconCatalogCachedAcrossRequests`：连续两次请求只触发一次 `championPinyinTerms` 计算（用计数器）。
- `TestR99IconOwnershipInvalidatedOnClientSwitch`：换 client 后拥有态重新拉取。

**变异判据（必须 FAIL）：**
1. 把 `ownershipType` 加回出参 → 隐私断言 FAIL。
2. 把 `EqualFold` 改成 `==` → 大小写断言 FAIL。
3. 把 inventory 失败改成整个接口失败 → fallback 断言 FAIL。
4. 把拼音计算从缓存里挪出来现算 → 计数器断言 FAIL。
5. 把失效键里的 `client` 去掉 → 切号断言 FAIL。

---

## P2 — 后端：写入头像 + 让客户端侧的改动回流

**证据：** 动作分发 `profile_facade.go:722-797`，白名单枚举 `profile_facade.go:691`，合法性校验范式 `knownFacadeSkin`（`profile_facade.go:969-982`），错误映射 `profile_facade.go:660-665`。请求结构体 `facadeApplyRequest` 在 `profile_facade.go:74-80`。

**要做什么：**
1. **★复核修正：先给 `facadeApplyRequest` 加字段**（`IconID int64`、`BannerID string`、`RankBanner string`、`BannerAccent string`），工单原稿漏了这一步。
2. 新动作 `"icon"`：`client.RequestJSON(ctx, http.MethodPut, "/lol-summoner/v1/current-summoner/icon", map[string]any{"profileIconId": request.IconID}, nil)`。
3. 校验：`request.IconID > 0 && knownFacadeIcon(ctx, client, request.IconID)`，否则 `errFacadeInvalid`。**不校验拥有**（口径 2）。
4. 把 `"icon"` 加进 `facadeDiagnosticAction` 的枚举（`profile_facade.go:691`）。
5. **★复核修正（阻塞级，原方案会造成真回归）：不要把 `/lol-summoner/v1/current-summoner` 加进 `isFacadeLCUEvent`。**
   `connection_manager.go:249` 是 `if a.handleFacadeLCUEvent(event) { return }` —— **命中即 early-return**，而召唤师身份刷新在 `:294` 之后的 scope switch 里（`case "summoner"` 在 `:299`、`case "summoner-profile"` 在 `:302`）。加了这个前缀会把召唤师事件整个吞掉，`a.summoner` 永不更新，连带把 R50 的生涯背景确认也废掉。
   **正确改法：在 `handleSummonerIdentityEvent` / `summoner-profile` 那两个 case 里额外调一次生涯变更广播**（即让它同时喂两条通道），或把 facade 判定挪到 scope switch 之后且不 `return`。二选一，别动 `isFacadeLCUEvent` 的前缀表。
6. **★复核修正（阻塞级）：写完头像的回读在当前架构下永远确认不了。**
   `loadFacadeStateTriggered` 里 `state.Summoner = projectFacadeSummoner(current)`，`current` 来自 `gameplayClient()` → `a.summoner`，那是**进程内缓存快照不是现读**。PUT 完立刻回读必然还是旧 `profileIconId`，界面必然常驻"客户端尚未确认"。
   **正确改法二选一：①`"icon"` 动作写完之后现读一次 `/lol-summoner/v1/current-summoner` 并更新 `a.summoner`；②把 5 的事件通道接通后，前端等一次 SSE 再判定。** 两条至少做一条，做完要有测试。

**验收判据：**
- `TestR99IconWriteSendsOnlyProfileIconID`：只发 1 次请求、方法 PUT、路径逐字相等、body 只有 `profileIconId` 一个键（照抄 `r50_suite_test.go:405`）。
- `TestR99IconWriteAccepts201`：假 LCU 返回 201 + 空 body，断言 `handleFacadeApply` 返回 200 而不是 502。
- `TestR99IconWriteRejectsUnknownID`：目录里没有的 id → 400，且零请求。
- `TestR99IconWriteRefreshesSummonerSnapshot`：写完之后 `a.summoner.ProfileIconID` 等于新值（对应第 6 条）。
- **★复核修正：`TestR99SummonerEventStillUpdatesIdentity`** —— 喂一条 `/lol-summoner/v1/current-summoner` 的 UPDATE 事件，断言 `a.summoner` **确实被更新了**（这是防止有人图省事去改 `isFacadeLCUEvent` 前缀表的反向护栏）。注意 `isFacadeLCUEvent` 的签名是 `func(event LCUEvent) bool` 不是 `func(string) bool`，测试要构造 `LCUEvent{URI: ...}`。

**变异判据（必须 FAIL）：**
1. body 改成 `{"profileIconId":x,"inventoryToken":""}` → "只有一个键"断言 FAIL。
2. 把 201 当失败（`status != 200` 即报错） → 201 断言 FAIL。
3. 把 `knownFacadeIcon` 校验去掉 → 未知 id 断言 FAIL。
4. **把 `/lol-summoner/v1/current-summoner` 加进 `isFacadeLCUEvent` 前缀表 → `TestR99SummonerEventStillUpdatesIdentity` FAIL**（这一条是本轮最重要的反向护栏）。
5. 把写完后的快照刷新去掉 → 快照断言 FAIL。

---

## P3 — 后端：生涯旗帜（**P0 的 R5/R9/W2/W3 出结果之后才动手**）

**证据：** `clearFacadeEmotes`（`profile_facade.go:1014-1038`）是本仓库已经跑通的 loadout 读改写范式：GET scope/account → `anyString(loadouts[0],"id")` → `safeLCUIdentifier(id)`（`watch_rules.go:1460`）→ `PATCH /lol-loadouts/v4/loadouts/{id}`，body 只带要改的槽位。

**要做什么：**
1. 目录：从 `regalia.json` 取 `regaliaType=="kBanner" && isSelectable==true` 的条目（实测 47 条）。剔掉 `id=="1"`（空白）和 `id=="2"` 的 11 个段位变体（交给 P5-B），**剩下正好 35 面**进旗帜选择器。`isTencentOnly`（实测 3 条）标注出来，不要过滤掉。
   **★复核修正：`regalia.json` 里 `id` 和 `idSecondary` 都是字符串**（`"id": "3"`），不是整数。结构体里定义成 `int64` 会反序列化直接炸。
2. 拥有态：`GET /lol-regalia/v3/inventory/REGALIA_BANNER` 的 `isOwned`（字段名以 R5 结果为准）；拿不到就 `bannerOwnershipUnavailable=true`，同 P1 的降级口径，失效键同样带 `client`。
3. 新动作 `"banner"`：照抄 `clearFacadeEmotes` 的形状，**PATCH 的 `loadout` 对象里只许有 `REGALIA_BANNER_SLOT` 一个键**，值的字段组合按 W3 的探测结论定。
4. 若 P0 的 R9 发现国服 loadout 根本没有 `REGALIA_BANNER_SLOT`，或 W2 回写被 4xx 拒绝：**当轮不实现写入**，前端旗帜卡只读展示当前旗帜 + 一行"你的客户端不支持从第三方设置这面旗帜"，结论写进 `docs/r99-probe-results.md`。**不许猜一个别的端点硬试**（R95 教训）。

**验收判据：**
- `TestR99BannerPatchTouchesOnlyBannerSlot`：假 loadout 里同时放 `EMOTES_WHEEL_CENTER`、`WARD_SKIN_SLOT`、`REGALIA_CREST_SLOT`、`REGALIA_BANNER_SLOT`，断言 PATCH body 的 `loadout` **只有 `REGALIA_BANNER_SLOT` 一个键**。
- `TestR99BannerRejectsUnsafeLoadoutID`：loadout id 含 `/`、`..`、空格时不发请求。
- `TestR99BannerCatalogExcludesBlankAndRankVariants`：喂完整 `regalia.json` fixture（**`id` 用字符串**，含 `"1"`、`"2"` 的 11 个 `idSecondary`、若干 `kNone`），断言出参正好 35 条。
- `TestR99BannerUnsupportedClientDegradesReadOnly`：loadout 里没有该槽位时 `bannerWritable=false`，apply 返回明确 4xx 而不是 502。

**变异判据（必须 FAIL）：**
1. PATCH body 改成整份 `loadout` 回写 → "只有一个键"断言 FAIL。
2. 去掉 `safeLCUIdentifier` → 不安全 id 断言 FAIL。
3. 段位变体过滤条件从 `id!="2"` 改成 `idSecondary==""` → 35 条断言 FAIL。
4. `bannerWritable` 恒为 true → 降级断言 FAIL。
5. `id` 字段类型改成 `int64` → 反序列化测试 FAIL（fixture 里是字符串）。

---

## P4 — 前端：两张新卡 + 两个弹窗

**证据与必须照抄的既有范式：**
- 渲染入口 `renderFacade()` `web/suite.js:1352-1389`（注意它是一句 `roots.facade.innerHTML = ...` 全量重刷），控制列 `web/suite.js:1378-1387`，绑定 `bindFacadeControls()` `web/suite.js:1521-1539`，写请求 `applyFacade()` `web/suite.js:1566-1580`（带 `state.facadeApplying` 互斥与在途 GET abort），草稿水合 `hydrateFacadeDraft()` `web/suite.js:1279-1294`。
- **弹窗必须用原生 `<dialog>` + `showModal()`，逐条照抄 `career-dialog` 的关闭链路**（`web/gameplay.js:6269-6295`）：点遮罩关、ESC 拦截关、`close` 时把焦点还给打开它的按钮、进出时调 `window.desktopTheme?.setModalOpen?.(bool)`（`desktop/preload.cjs:24`）。
- **不许复用 `requestConfirmation`**（`web/suite.js:112-128`）——它是右下角吐司、`aria-modal="false"`、无遮罩无焦点陷阱。
- 大网格性能照抄收藏页：时间切片 `runRenderTasks`（`web/app.js:1186-1200`，每帧 7ms）+ 图片懒加载 `ensureCardImageObserver`（`web/app.js:1528-1538`，`rootMargin: "620px 0px"`）。**本轮不引入虚拟滚动。**
- 搜索复用 `window.deepLegendsChampionSearch.scoreOption`（`web/champions.js:2406`，消费范例 `web/app.js:202`），防抖 150ms（`web/app.js:3157`），中文输入法 `compositionstart/end` 照抄 `web/champions.js:2283-2298`。
- 图片一律走 `/api/image?path=`（`web/suite.js:176-178`）。**白名单不用改**：`sanitizeClientImagePath`（`lcu_api.go:821-830`）已放行 `/lol-game-data/assets/`，实测头像 `imagePath` 是 `/lol-game-data/assets/v1/profile-icons/1339.jpg`、旗帜 `assetPath` 是 `/lol-game-data/assets/ASSETS/Regalia/BannerSkins/lny2023.png`，都命中；未连客户端时 `main.go:904-916` 自动回落 CommunityDragon（映射表 `main.go:955-985`）。
- **CSP 禁内联样式**（`main.go:666-672`，测试 `gameplay_test.go:5750-5751` 逐字锁定）：**设计稿里为了省事写了 `style="..."`，产品实现一律不许**，格子底图/尺寸走 `data-*` + CSSOM，范式见 `web/gameplay.js` 的 `applyRenderedMetricStyles`。

**要做什么：**
1. 控制列在「背景」卡之后插入「头像」卡和「旗帜」卡（截图 ①）。「旗帜」卡三行：生涯旗帜、段位旗（二选一分段）、挑战旗帜配色。
2. 左侧预览区加旗帜缩略图（截图 ① 左下），头像预览沿用 `.facade-avatar`（`web/suite.css:252`），不动布局。
3. **★复核修正（阻塞前提）：两个 `<dialog>` 必须 `document.body.append`，挂在 `roots.facade` 之外。** 原因有二：①`renderFacade` 是全量 innerHTML 重刷，挂在里面必然被重建，"打开时不重建子树"根本做不到；②`.app-frame` 已经缩放过一次，挂在里面再抄 `.career-dialog` 的 `zoom: var(--ui-zoom,1)` 会平方缩放（`web/suite.css:530` 的 `.cs-dialog` 注释就是在讲这个坑）。**挂 `document.body` → 抄 `.career-dialog`（`web/gameplay.css:263`）的 `zoom` 写法。**
4. 头像弹窗（截图 ②）：左侧系列侧栏（81 个 set + 全部/已拥有/近三年新增）、顶部搜索 + 系列下拉 + 排序 + "只看已拥有" + 计数 chip、圆形网格（`minmax(74px,1fr)`，拥有打勾 / 未拥有置灰加锁）、底部选中预览 + 「设为头像」。
5. 旗帜弹窗（截图 ③）：`minmax(104px,1fr)` 的 2:3 竖版网格、名称一行省略、底部预览 + 「设为旗帜」。
   **★复核修正：`regalia.json` 里没有任何分类字段**，设计稿上「名人堂 / 赛事冠军 / 活动限定 / 排位荣誉」这四个分组没有数据源，而且「排位荣誉」在剔掉 `id=="2"` 之后必然是空的。**降级为：左侧只留「全部 / 已拥有 / 国服专属（`isTencentOnly`）」三档，主区按 `localizedName` 排序**；想保留名人堂分组就明写关键词表（`名人堂`）并在代码注释里承认是启发式。
6. **★复核修正：`hydrateFacadeDraft`（`web/suite.js:1279-1294`）要扩四个字段**（`iconId` / `bannerId` / `rankBanner` / `bannerAccent`），初值分别从 `value.summoner.profileIconId`、当前 banner slot、`regalia.preferredBannerType`、`lol.bannerIdSelected` 反填。注意 `applyFacade` 在 `:1574` 会调 `hydrateFacadeDraft(true)` 强制重置草稿，新字段要一起考虑。
7. **★复核修正：补 `web/demo-data.js`**（已有 `suiteFacade` 相关数据：`:914 facadeSkins`、`:929 skins`、`:1001 /api/facade/state`、`:1069 /api/facade/apply`）。不补 icons/banners，macOS 上的 `?demo` 开发模式下两张新卡是空的、弹窗打不开——**这个项目 macOS 侧开发全靠 demo 模式，别漏。**
8. **★复核修正：未连客户端时两张新卡跟背景卡一样整体隐藏/置灰**（`facadeState.Reason` 已有"未连接英雄联盟客户端"）。**不要**去给目录 JSON 做 CommunityDragon 回落——图片有回落，目录没有，也不需要。

**验收判据：**
- 新增 `desktop/r99-facade-picker-layout.cjs`（照抄 `desktop/champselect-dialog-layout.cjs` 的 CDP + 变异开关范式）：1500 / 1180 / 980 三档视口打开两个弹窗，断言 ①弹窗不超出视口 ②网格首格 `clientWidth >= 40` ③底部预览名称与按钮无 `scrollWidth-clientWidth>1 且 overflow-x:hidden` 的截断 ④侧栏与网格各自可滚而弹窗本身不滚 ⑤`zoom` 在 `--ui-zoom=1.25` 下不出现平方缩放（用实际像素宽度断言）。
- `web/suite.test.cjs` 补：弹窗 DOM 挂在 `document.body` 而非 `roots.facade` 内；ESC / 遮罩 / close 三条路径都把焦点还给触发按钮。
- 断言渲染出的 DOM 里**没有 `style=` 属性**（CSP 回归护栏）。
- 断言首帧渲染的格子数 ≤ 上限（时间切片生效）。

**变异判据（必须 FAIL）：**
1. 把网格格子的 `aspect-ratio` 删掉 → 布局脚本 ② FAIL。
2. 把 `display:block` 从格子底图上删掉（**本次设计稿真踩过这个坑，第一版截图整片空白**） → 布局脚本 ② FAIL。
3. 用 `style="background:..."` 写内联样式 → CSP 断言 FAIL。
4. 把 `setModalOpen(false)` 从 `close` 里删掉 → 模态断言 FAIL。
5. 把 `<dialog>` 挂回 `roots.facade` 内 → "挂在 body"断言 FAIL + zoom 断言 FAIL。
6. 把时间切片改成一次性 `innerHTML` 渲染全部 → 首帧条数断言 FAIL。

---

## P5 — 段位旗（B）与挑战旗帜配色（C）

**证据：** `profile_facade.go:777-785`（`clear-border` 已经在 GET 后 PUT regalia，并**特意沿用了原来的 `preferredBannerType`**）；`profile_facade.go:799-874`（`writeChallengePreferences` 统一处理三个动作，base body 在 `:823-826`）。

**要做什么：**
1. B：新动作 `"rank-banner"`，取值只允许 `"lastSeasonHighestRank"` / `"blank"`，其余 `errFacadeInvalid`。**必须先 GET 再合并**：`preferredCrestType` 和 `selectedPrestigeCrest` 沿用客户端当前值，不许写死——否则会和 `clear-border`（写 `prestige`/`22`）互相踩。
2. C：把 `previous-banner` 写死的 `"2"`（`profile_facade.go:790-791`）参数化成 `"banner-accent"` 动作，**必须走 `writeChallengePreferences`**，取值集合取自 P0-W4 结论；**W4 没结论就整条不做**，`previous-banner` 原样保留。

**验收判据：**
- `TestR99RankBannerPreservesCrestPreferences`：当前 regalia 是 `{preferredCrestType:"ranked", selectedPrestigeCrest:7}`，写 `blank` 后断言 PUT body 里这两个字段**仍是 ranked/7**。
- `TestR99RankBannerRejectsUnknownValue`：`"hextech"` → 400 且零请求。
- C 若实现：`TestR99BannerAccentKeepsTitleAndChallenges`：断言 `bannerAccent` 之外的 `challengeIds` / `title` 与读到的现状一致（回归点见 `r66_test.go:20-21`）。

**变异判据（必须 FAIL）：**
1. `preferredCrestType` 写死成 `"prestige"` → 保全断言 FAIL。
2. 去掉取值白名单透传任意字符串 → 未知值断言 FAIL。
3. C 实现后把 `challengeIds` 改成固定空数组 → 保全断言 FAIL（这正是 R66「卸勋章必清头衔」的同一类破坏）。

---

## P6 — 隐私声明与能力文案（**别当成文档活跳过**）

**证据：** `features.go:744-753` 的 `handlePrivacy`（`:748` 的 explicitWrites 实测正好 9 条）；`quality_test.go:403-458` 的 `TestPrivacyListsEveryClientWrite`：`:417` 断言 `len(ExplicitWrites) != 9` 即失败、`:430` 断言必含「生涯背景」「个性签名」「头像框」等关键词、`:450` 断言 `neverStores` 必含 `"PUUID"`。

**要做什么：**
1. `explicitWrites` 新增（9 → 11，C 做了就 12）：
   - 「设置生涯头像：只写 `profileIconId` 一个字段，不改其它资料」
   - 「设置生涯旗帜：只写生涯装扮里的旗帜槽位，不动表情、守卫皮肤和其它槽位」
   - （C 若实现）「设置挑战旗帜配色：与头衔、挑战勋章同一个接口，写入时保留原有头衔与勋章」
2. `externalReads` 新增：「为了让职业选手账号在改名后仍能认出来，会按已解析的稳定标识向 Riot 官方接口反查该选手当前的 Riot ID；**只针对内置名单里的职业选手，不针对你和你的好友**」。
3. `stores` 新增：「内置职业选手名单里那条人工核对账号的稳定标识锚点（本地缓存，30 天）」。
4. **★ 顺手修一条既有的不准确表述（复核已独立确认属实，不是冤枉）：** `features.go:752` 的 `neverStores` 写着「PUUID、AccountID、SummonerID 与战绩内容」绝不存储，但事实是——`fetchAccountByRiotID`（`riot_api.go:703-715`）把含 `PUUID` 的 `riotAccount`（`riot_api.go:514-518`）整个交给 `cachedPublicIdentity`，后者 `json.Marshal` 进 `riot-identities` 磁盘缓存（`riot_identity_cache.go:22-37`），磁盘白名单放行见 `champion_cache.go:326-328`。文件是 0600、文件名两次 sha256，但**内容是明文**。
   **本轮必须把这条改成准确表述**，例如：「PUUID、AccountID、SummonerID 绝不外发给任何第三方，也绝不出现在前端和诊断日志里；其中韩服查询解析到的 PUUID 会在本机缓存目录里短期保存以便复用」。
5. **★复核修正：只改 `quality_test.go:417`（ExplicitWrites 9→11/12）。`:420` 是 `AutomaticWrites != 10`，本轮一条自动写入都没加，不要动它。**
6. **新增关键词断言**：explicitWrites 必含「头像」「旗帜」「不动表情」，neverStores 必含「不外发」，stores 必含「锚点」。**只改数字不加关键词不算完成本条。**

**变异判据（必须 FAIL）：**
1. 只把 `9` 改成 `11` 而不往 `features.go` 加文案 → 新增的关键词断言 FAIL。
2. 把 neverStores 那条改回原文 → 「不外发」关键词断言 FAIL。

---

## P7 — 职业选手种子账号：IG Rookie

### 7.1 身份核实结论（已独立核对，写进文档，别再重查）

- 核对日期 2026-09-16。两个相互独立的来源把 `모든일은같이#KR1` 归给 IG 中单 Rookie（Song Eui-jin / 송의진）：
  - Leaguepedia `https://lol.fandom.com/wiki/Rookie` 的 Soloqueue IDs 栏：`KR: 모든일은같이#KR1 / dyjkbysb#KR1 / asdfzxcvasdfwqer#KR2`（弱点：取到的是较旧的缓存版本，拿不到最后编辑日期）。
  - dpm.lol `https://dpm.lol/pro/Rookie`：`gameName:"모든일은같이", tagLine:"KR1", team:"IG", lane:"MIDDLE", role:"PRO"`，最近对局 2026-09-16。
  - 交叉印证：op.gg 该召唤师页的段位战绩（大师组 2437 LP / 389 胜 310 负）与 dpm.lol 逐字段一致，证明两站指的是同一个账号。
- **关键事实：op.gg 的召唤师页整页没有 "Rookie" 字样也没有战队标签**——上游目录压根没把这个账号标成职业账号，**所以它永远不会从现有 OP.GG 链路自己冒出来，必须靠种子**。
- **两站公布的 "PUUID" 互相矛盾**（op.gg 给 `GBtYyX...`、dpm.lol 给 `KqKz5k...`，都是 78 字符），必有一方是站点自己混淆过的 ID。**一律不许抄，只认我们自己 by-riot-id 解析的结果。**
- 置信度：**较可信，不是确证**（没有选手本人或战队的第一手公布）。文档里要如实这么写。

### 7.2 代码

**证据：** 名单 `pro_roster.go:38-45`（IG），Rookie 那行 `pro_roster.go:41`（`AllowTeams:[]int{858}`）。合并流水线 `buildProPlayers` `pro_players.go:514-671`，身份并查集 `:580-598`，账号双键 `proAccountKeys` `pro_players.go:693-699`。补充源接入范式 `pro_players_supplement.go:75-83` + 调用点 `pro_players.go:207` / `:229` / `:236-247`。Riot 封装 `accountByRiotID` `riot_api.go:654`、`fetchAccountByRiotID` `riot_api.go:703-715`、身份磁盘缓存 `cachedPublicIdentity` `riot_identity_cache.go:22-37`。限流 `riot_api.go:273-350`（全局 15/秒 + 90/2 分钟，**所有端点共用一个池**）、队列预算 `withRiotQueueLimit` `riot_api.go:352-357`、FIFO `riot_limiter_queue.go:23-96`。

**要做什么：**

1. 新增 `pro_seed_accounts.go`：

```go
// 人工核对的种子账号。只登记 Riot ID，绝不登记 PUUID：
// 第三方站点公布的 PUUID 互相矛盾，唯一可信来源是我们自己的 by-riot-id 解析。
type proSeedAccount struct {
    TeamCode string // "IG"
    Player   string // "Rookie"，必须与 pro_roster.go 里的 Name 完全一致
    OPGGID   int    // 371（IG 本队），不是 858
    GameName string // "모든일은같이"
    TagLine  string // "KR1"
    Reviewed string // "2026-09-16"
}
var proSeedAccounts = []proSeedAccount{
    {TeamCode: "IG", Player: "Rookie", OPGGID: 371, GameName: "모든일은같이", TagLine: "KR1", Reviewed: "2026-09-16"},
}
```

2. 锚点与解析（"改名也不影响"的核心）：
   - 锚点：复用 `cachedPublicIdentity`，identity 串 `"proseed:v1:" + teamCode + "/" + lower(player)`，TTL 30 天，内容只有 `{"puuid":"..."}`。**并按口径 4 的 ★ 要求解决 LRU 淘汰问题（加保护前缀或命中即重写刷新 mtime）。**
   - 新增 `fetchAccountByPUUID`：`GET /riot/account/v1/accounts/by-puuid/{puuid}`（host 用 `riotClusterHost`，`riot_api.go:126`），identity 串 `"account-by-puuid:"+puuid`，TTL 6 小时。**全仓目前没有这个端点的封装**，照 `riot_api.go:703-715` 的形状新写。
   - 流程：先读锚点 → 命中就 by-puuid 拿**当前** Riot ID（改名照样拿得到）→ 未命中就用种子里的 Riot ID 走 `accountByRiotID` 拿 PUUID 并写回锚点。
   - **自愈**：by-puuid 返回的 Riot ID 与种子里写的不一致时不要报错，直接用返回值，并打一条诊断 `pro_seed_renamed`（**只记 `changed:true` 和选手名，不记新旧 Riot ID，更不记 PUUID**）。
3. 注入：做 `withProSeed(base []opggProTeam) []opggProTeam`，**★复核修正：三处调用点都要包，不是两处** —— `pro_players.go:207`（占位快照）、`:229`（`loadProSupplements` 的 partial 回调里那次 `append(cloneProTeams(base), ...)` → `:233 c.teams = snapshot`）、`:236-247`（completed）。**漏掉 `:229` 的话，加载中途种子账号会短暂消失再出现**，正是 R98 那类"列表乱一下又恢复"的观感。
4. **★复核修正：造出来的 `opggProMember` 必须设对字段**，否则要么过不了闸门、要么整页永久 partial：
   - `Authority: "PROGAMER"`（`proDirectoryMemberValid` `pro_identity.go:28` 硬性要求）
   - `TeamID: 371`、所在 `opggProTeam{ID: 371, ShortName: "IG"}`（`proReviewedMemberBadge` `pro_identity.go:141-144` 允许 371 或 858，**选 371 本队，不要用 858**，858 是 OP.GG 那边的历史队标例外，见 `docs/pro-players-sources.md:101`）
   - `Nickname: "Rookie"`、`RealName` 要能过 `proIdentityMatches`（`pro_players.go:411-424`），用 `"Song Eui-jin"` 或 `"송의진"`
   - **`Incomplete` 和 `Supplement` 都留 `false`** —— 照抄 `pendingProSupplements` 会让 `proSupplementsIncomplete` 把整页永久标成 partial。
5. 产出的 `opggProAccount`：`{PUUID: 解析值, GameName/TagLine: 反查到的当前值, Region: "kr", Source: "seed", UpdatedAt: time.Now().UTC().Format(time.RFC3339)}`（**★复核修正：`UpdatedAt` 是 string 不是 time.Time**）。`normalizeProAccount`（`pro_players.go:441-491`）里 `Source=="seed"` 时 `proAccount.Source` 显示为「人工核对」；有 PUUID 所以**不置 `Confidence="low"`**。
6. **配额纪律（R98 教训）**：种子解析必须带 `withRiotQueueLimit` 预算，超预算时**当轮放弃**（保留上次快照）而不是阻塞排队；否则会占住 FIFO 队头，抢用户正在看的韩服战绩。稳态成本：30 天 1 次 by-riot-id + 6 小时 1 次 by-puuid。
7. `docs/pro-players-sources.md` 新增一节「R99 人工核对种子账号（2026-09-16）」，按该文档既有写法（`:101` 那条 AllowTeams 例外是范式）写清：两个来源与各自局限、op.gg 未标记该账号为职业账号这一事实、两站 PUUID 矛盾因此一律自解析、置信度是"较可信"而非"确证"、**这条例外只对 IG/Rookie 生效不向其他选手开放**，以及「有 PUUID 的种子账号不按低置信度处理」这条与 `:50` 既有口径的关系。

**验收判据：**
- `TestR99SeedAppearsWhenUpstreamHasNoAccount`：上游目录里 IG/Rookie 一条账号都没有时，`/api/pro-players` 里 Rookie 名下出现 1 条 `source=="人工核对"` 的账号，`status != "missing"`。
- `TestR99SeedMergesWithUpstreamByPUUID`：上游已有一条**同 PUUID 但不同 Riot ID**（模拟改名）的记录时，结果**只有一条**（并查集合并，`pro_players.go:580-598`），取 `UpdatedAt` 较新的那条。
- `TestR99SeedRenameSurvivesRestart`：锚点已存在、种子里的旧 Riot ID 用 by-riot-id 查会 404 时，仍能通过 by-puuid 拿到新 Riot ID 并正常展示。**这条是整个需求的验收核心。**
- `TestR99SeedNeverLeaksPUUID`：`/api/pro-players` 响应里不含 `puuid` 键、不含解析出的 PUUID 字面值（照抄 `r73_test.go:31`、`pro_players_test.go:107`）。
- `TestR99SeedRespectsQuotaBudget`：限流器已满时种子解析不阻塞、当轮返回上次快照、不产生额外 HTTP。
- **★复核修正（原稿写反了，这里是正确版本）：`pro_players_test.go:64` 的 `PlayerCount==33 / AccountCount==1 / MissingCount==32` 与 `:549` 的 `AccountCount==7`（以及 `:566` 的 `index.candidates==7`）** 这三处硬编码计数 **必须保持不变**。这两个测试是直接调 `buildProPlayers(rows, proRoster)` 和直接塞 `a.proPlayers.teams` 的，根本不走取数流水线。**如果它们的数字变了，说明你把种子注进了 `buildProPlayers` 内部——那是错的实现，退回去按第 3 条改。** 这条本身就是护栏，不要改数字。
- `r95_test.go:263`（空快照不得发起任何 HTTP）**必须仍然绿** —— 种子解析不许挂在 `proIdentitySnapshot` / `matchProIdentity` 路径上。

**变异判据（必须 FAIL）：**
1. **★复核修正（原稿是同义反复的假护栏）：不要写 `if proSeedAnchorTTL != 30*24*time.Hour` 这种把常量抄一遍的断言。** 改成行为断言：注入可控 clock，断言「第一次调用发 1 次 by-riot-id；TTL 内第二次调用**零** by-riot-id、只走 by-puuid」。把锚点写入去掉或把 TTL 改成 0 → 这条 FAIL。
2. 把 by-puuid 反查去掉、永远用种子里的 Riot ID → `TestR99SeedRenameSurvivesRestart` FAIL。
3. **★复核修正：** 把 `proSeedAccount.GameName` 换成一串第三方站点抄来的 PUUID 字面值 → 新增源码级断言「`pro_seed_accounts.go` 里不得出现长度 ≥ 40 的连续 base64url 串」FAIL（照抄 `quality_test.go` 的源码扫描范式）。
4. 把 `withRiotQueueLimit` 去掉 → 配额测试 FAIL。
5. 把种子注进 `buildProPlayers` 内部 → `pro_players_test.go:64` / `:549` 的既有计数 FAIL（见上条验收判据）。
6. 把 `withProSeed` 从 `:229` 那处漏掉 → 新增断言「partial 回调发布的中间快照里也含种子账号」FAIL。
7. 把 `Incomplete` 设成 true → 断言「`result.Partial == false`」FAIL。

---

## 本轮明确不做的事

1. 头像框 / 牌位（`REGALIA_CREST`、`REGALIA_BORDER`）、表情、守卫皮肤、TFT 相关的其它 loadout 槽位——探测会顺手记形状，不实现写入。
2. `/lol-banners/v1/...` 那套 Clash/赛事三角旗（`TOURNAMENT_FLAG`）——R11a/b/c 只判断它还活没活着，结论写文档，不实现。
3. 选手管理页的"手动绑定账号"输入框——用户已拍板走源码内置，本轮不做写接口。
4. 其它 5 位 IG 选手、其它 5 支队伍的种子账号——**只加 Rookie 这一条**，跑通再谈扩。
5. 收藏页/图鉴里的头像展示改造。
6. 虚拟滚动——用既有的时间切片 + 懒加载，别引新机制。

---

## 只能真机验收的清单（macOS 上测不了，必须在 Windows + 国服客户端上看）

1. P0 全部 13 条只读 + 4 组写探测的真实返回（尤其 R5 / R9 / W2 / W3，它们决定生涯旗帜功能到底能不能做）。
2. 设置了**未拥有**的头像 / 旗帜之后，重登客户端是否被服务端还原、还原耗时。
3. 国服 `regalia.json` 与 CommunityDragon 版本的差异（已知 `isTencentOnly` 至少 3 条）。
4. 5099 个图标的弹窗在真机上滚动是否掉帧（jsdom 和沙箱截图都测不出）。
5. 种子账号那条链路真实占用的韩服额度，以及它与用户正在加载的韩服总览是否互相拖慢（R98 的 90/2 分钟本来就紧）。
6. 用户在客户端里自己换头像后，我们这边多久刷新（P2-5/6 那条通道是否真接通）。
7. Rookie 若在观察期内真的改名，新名字是否自动跟上（只能等，不能造）。

---

## 附：本轮调研的一手证据来源

- LCU schema：`https://github.com/dysolix/hasagi-types/blob/main/swagger.json`（LCU `/help` 自动导出）
- `lolinventorytype.json`（Riot 自己的 inventoryType→资源清单映射表，`REGALIA_BANNER` 的 `gipJsonPath` 指向 `regalia.json`）：`https://raw.communitydragon.org/latest/plugins/rcp-be-lol-game-data/global/default/v1/lolinventorytype.json`
- 头像清单（实测 5099 条，中文 `title` 100% 覆盖，1,949,389 字节）：`.../global/zh_cn/v1/summoner-icons.json`；系列 81 条：`.../summoner-icon-sets.json`
- 旗帜清单（实测 139 条，47 条 `isSelectable` 全为 `kBanner`，剔除空白与段位变体后 35 条，`isTencentOnly` 3 条）：`.../global/zh_cn/v1/regalia.json`
- loadout 槽位名交叉印证：LeagueAkari `src/shared/types/league-client/game-data.ts`（`https://github.com/LeagueAkari/LeagueAkari`）
- 头像写入 201 的实证：`https://github.com/MManoah/lol-summoner-icon-changer`
- **注意：`REGALIA_BANNER_SLOT` 的写入没有任何公开项目实证过**，Akari 只写过 `EMOTES_*` 和云顶难度。这就是 P0 必须先跑的原因。
