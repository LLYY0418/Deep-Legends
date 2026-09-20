# WORKLIST-R101：头像 401 定因 / 旗帜解锁 / 选人页时间框 / 职业账号主号

诊断人：Claude（只读诊断，未改仓库代码文件；本轮只新增 `design-mockups/r101-facade-left-column.html`、`design-mockups/_shots/r101-facade-left-column.png`、`docs/pro-accounts-verification-2026-09-17.md`）。
执行人：GPT。
日期：2026-09-17。
输入证据：真机诊断日志 `lol-loot-diagnostics-0916-2155.jsonl`（1824 行，build `8944d5e33f9c`，run `364cbd108cf990db4c7b93a1`）+ 用户四张截图。

---

## 口径决定（不要自行更改）

1. **R99 的 P0 真机探测结果已经到手**（13 条只读 + 4 组写全部有数据，见下面「P0 结果归档」）。R99 口径 5 的门禁按下表**逐项解除**，没解除的项目仍然不许动。
2. **★ 但有两项被「零副作用」探测方式本身挡住了，仍未证实。** R99 我自己定的口径 6 要求写探测一律「把当前值原样回写」，结果 W1 和 W2/W3 **全都只验证了"写回同一个值"**，没有任何一次验证"换成另一个值"。头像恰好因为用户手动点了 7 次（非零副作用）才暴露出 401。**旗帜没有这个运气，所以「旗帜能不能换」目前仍然是未知的。** 本轮必须先补两条判别探测（W5/W6）才能写实现。
3. 头像：**未拥有的置灰不可选，弹窗默认开启「只看已拥有」**（用户已拍板）。实测拥有 478 / 目录 5099。
4. **「挑战旗帜配色」整行删除，同时把「展示清理」里的「切换上赛季旗帜」按钮一并删除**（用户已拍板）。实测 `bannerAccent` 写入返回 400，那个旧按钮走的是同一个接口、同一个字段，按这次结果看它很可能一直是无效的。
5. 选人页锁定等待时间框：**「仅亮出」「立即锁定」下保留控件、置灰、显示 0，但绝不把 0 写回设置**（用户已拍板）。
6. 生涯页：**头像卡与旗帜卡移到左列**，删除预览区那段「这是好友悬浮卡与生涯页的示意预览…」（用户已拍板）。设计稿见 `design-mockups/_shots/r101-facade-left-column.png`。
7. 职业选手账号：**本轮只修 Rookie 种子拿不到段位这个 bug**，不新增任何选手账号。33 人的账号核对清单已交用户（`docs/pro-accounts-verification-2026-09-17.md`），等他用 OP.GG 小程序核对回来再单开一轮录入。
8. 沿用 R99 口径：先探测后实现；诊断只记结构/数量/枚举/状态码，不记玩家标识。

---

## P0 结果归档（真机实测，已取得，写进 `docs/r99-probe-results.md` 替换掉「待测」）

### 只读 13 项

| 项 | 端点 | 结果 | 对实现的影响 |
|---|---|---|---|
| R1 | `summoner-icons.json` | **200，5099 条**，`title` 非空 5099，字段含 `yearReleased/isLegacy/imagePath/rarities/disabledRegions` | 与公开目录完全一致，P1 目录可放心用本机 |
| R2 | `summoner-icon-sets.json` | **200，81 条**，字段 `id/displayName/description/icons/hidden` | 系列筛选可用 |
| R3 | `regalia.json` | **200，139 条**，`isSelectable` **47**，`regaliaType` 只有 `kBanner`/`kNone`，**`id` 是字符串**，`isTencentOnly` **3** 条 | 与公开目录逐字吻合，35 面可选旗帜成立 |
| R4 | `/lol-inventory/v2/inventory/SUMMONER_ICON` | **200，508 条，`owned==true` 478 条**，`ownershipType` 去重只有 `OWNED` | **头像拥有态可用，P1 门禁解除** |
| R5 | `/lol-regalia/v3/inventory/REGALIA_BANNER` | **200，对象，37 个数字键，`isOwned==true` 4 个**，值字段 `isOwned/items/purchaseDate` | **旗帜拥有态可用**，键就是 regalia.json 的 id |
| R6 | `.../REGALIA_CREST` | 200 但**空对象**（0 键） | 牌位无数据，本轮本来也不做 |
| R7 | `.../REGALIA_BORDER` | **400** | 该 type 不支持，永久排除 |
| R8 | `/lol-inventory/v2/inventory/REGALIA_BANNER` | 200 但是**数组**、4 条、**`owned==true` 0 条** | ★对照组价值：**这条不能用来判拥有**，必须用 R5 的 `/lol-regalia/v3`。四格矩阵这次真的救了命 |
| R9 | `/lol-loadouts/v4/loadouts/scope/account` | **200，1 条，scope=ACCOUNT，槽位里确有 `REGALIA_BANNER_SLOT`**，每个槽位值字段 = `contentId` / **`data`** / `inventoryType` / `itemId` | **国服真的有这个槽位**。注意多出一个社区文档没有的 `data` 字段 |
| R10 | `/lol-regalia/v2/current-summoner/regalia` | 200，`bannerType`/`preferredBannerType` 都是 `blank`，`crestType`/`preferredCrestType` 都是 `prestige` | 证实枚举只有 `lastSeasonHighestRank`/`blank` |
| R11a | `/lol-banners/v1/current-summoner/flags` | 200 但**空数组**，`theme` 空 | — |
| R11b | `.../flags/equipped` | **404** | — |
| R11c | `.../frames/equipped` | **404** | — |

**→ R11 整条备选链路确认已死，永久关闭，文档写明不用再查。**

### 写 4 组

| 项 | 动作 | 结果 |
|---|---|---|
| W1 | 把**当前正在用的**头像原样 PUT 回去 | **201 ok** |
| W2 | 把**当前那面旗**的完整槽位原样 PATCH 回去 | **200 ok**，回包槽位字段 = `contentId/data/inventoryType/itemId` |
| W3-pair | 只传 `{inventoryType,itemId}`（仍是当前那面旗） | **200 ok**，恢复 200 |
| W3-content | 只传 `{contentId}`（仍是当前那面旗） | **200 ok**，恢复 200 |
| W3-all | 三个字段全传（仍是当前那面旗） | **200 ok**，恢复 200 |
| W4 | `bannerAccent` 试写相邻值 | **400 失败**，`changed=false`，恢复请求 204 |

**→ `contentId` 不是必填，三种字段组合都被接受。但见口径 2：这四条写的全是同一个值。**

---

## P1 — 头像 401 定因（**本轮最高优先级，不做完不许改头像实现**）

### 已经查实的事实链

1. 用户手动点「设为头像」共 **7 次，全部 401 `RPC_ERROR`**：`facade_apply` 里 7 条 `action=icon result=failed status_code=401`（13:42:15 / 13:42:19 / 13:42:43 / 13:42:58 / 13:43:55 / 13:44:09 / 13:46:00）。
2. `lcu_request` 聚合埋点对 `PUT /lol-summoner/v1/current-summoner/icon` 共发布 6 条事件、**样本合计 7 次，状态全是 401，零成功**。
3. W1 探测（13:46:24）自报 **201 ok**。
4. **3 和 2 不矛盾**，别误判成"探测上报造假"：`recordRequestDiagnostic`（`lcu.go:193-227`）是**惰性 flush** —— 某个 (method,path) 桶只有在**下一次同 key 请求落到新窗口时**才会被发布（`lcu.go:210-213`）。按 10 秒窗口把 6 条事件的 `window_start` 与 7 次点击的时间戳对齐，可以逐条还原出：最后那条事件（`log_seq 463`，`window_start 13:46:00`）记的是**第 7 次用户点击**，而它之所以被 flush，正是因为 **W1 探测这第 8 次请求到来**。W1 自己那个桶（窗口 13:46:20）**从没被 flush，所以日志里根本没有它**。
   → 结论：**没有证据说 W1 上报有误；W1 的 201 目前可信。**
5. 同一台机器、同一次会话里，**别的写操作都成功**：`PUT /lol-regalia/v2/current-summoner/regalia` 201×2（段位旗，3 次 `facade_apply` 全 ok）、`POST /lol-summoner/v1/current-summoner/summoner-profile`（生涯背景，`facade_apply` ok 且 `background_confirmed:true`）、`PATCH /lol-loadouts/v4/loadouts/{id}`（W2/W3 全 200）。**只有 icon 这一个端点 401。**
6. 用户拥有 **478 / 5099** 个头像；因为 R99 口径 5 把拥有态挡住了，弹窗当时把 5099 个**全部**设为可选，用户随手点中未拥有图标的概率约 90.6%。

### 两个仍然并存的解释（**不许直接挑一个去写实现**）

- **(A) 401 = 这个图标你没有。** 服务端做了拥有校验，未拥有直接拒。W1 写的是自己当前的图标（必然拥有）所以 201。
- **(B) 401 = 这个端点在国服不接受"改成新值"。** W1 之所以 201，是因为写的值跟当前值相同、被当成空操作放行了。

两者对现有全部证据的解释力**完全一样**。这正是 R82 那次「六格矩阵只测了对角线」的同一个坑。

### W5 判别探测（手动触发，一次定生死）

用 R4 已经拿到的拥有列表，挑一个**你确实拥有、但不是当前正在用的**图标：

1. `GET /lol-summoner/v1/current-summoner` 取当前 `profileIconId`
2. `GET /lol-inventory/v2/inventory/SUMMONER_ICON` 取 `owned==true` 的集合
3. 从中选一个 `itemId != 当前值` 的，`PUT /lol-summoner/v1/current-summoner/icon`
4. **不论成功失败，立刻把原值 PUT 回去**（独立超时上下文，照抄 `r99_probe.go:318-324` 的 `defer` 恢复写法）

判定：

- **201 → 成立 (A)**：按口径 3 做「未拥有置灰不可选 + 默认只看已拥有」，头像功能即可正常上线。
- **401 → 成立 (B)**：头像不能走这个端点。**按下面的备选顺序继续探，不许猜**：
  - **W5-b（最有希望）**：`POST /lol-summoner/v1/current-summoner/summoner-profile`，body `{"key":"profileIconId","value":<已拥有的另一个图标>}`。理由：**这个端点在同一台机器上已经被证明可写**（生涯背景就是走它，`profile_facade.go:730` 的现成写法，本次日志里 `background_confirmed:true`）。
  - **W5-c**：`PUT .../icon` 带上 LCU schema 里那个可选的 `inventoryToken` 字段。先探 token 来源：`GET /lol-inventory/v2/signedInventory`、`/lol-inventory/v1/signedInventory`（只记 status + 顶层键名，**不记 token 值**）。
  - 三条都失败 → 头像功能**本轮明确放弃**，前端把卡片改成只读展示当前头像 + 一行「国服客户端不允许第三方修改头像」，结论写进文档。**不许留一个点了必然报错的按钮。**

**诊断记录要求：** `r99_write_probe` 新增 `probe: "W5"` / `"W5-restore"` / `"W5-b"` / `"W5-c"`，只记 status、`error_code`、`ok`、`restored`，**不记任何 profileIconId 的值**（它虽然不是账号标识，但按本项目口径写探测一律不落具体内容 ID）。

**验收判据：**
- `TestR101IconProbePicksOwnedNonCurrentIcon`：假 LCU 给 `{当前=71, owned=[71,72,99]}`，断言试写的是 72 或 99（**不是 71**），且最后一次请求是把 71 写回去。
- `TestR101IconProbeRestoresOnFailure`：试写返回 401 时，仍然发出恢复请求。
- `TestR101IconProbeNeverLogsIconID`：诊断事件里不含试写用的那个 id 的字面值。

**变异判据（必须 FAIL）：**
1. 把"挑一个非当前的已拥有图标"改回"用当前图标" → 第一条断言 FAIL。
2. 把失败分支的恢复去掉 → 第二条 FAIL。
3. 把 `profileIconId` 记进诊断 → 第三条 FAIL。

---

## P2 — 旗帜：同一个陷阱，先补 W6 再动实现

**事实：** W2 / W3-pair / W3-content / W3-all 四次全部 200，**但四次写的 `itemId`/`contentId` 都来自当前已装备的那面旗**（`r99_probe.go:276` 的 `variants` 全部取自 `slot`）。所以目前只证明了「原样回写会被接受」，**没有证明「能换成另一面旗」**。

**W6 判别探测（手动触发）：** 用户拥有 4 面旗（R5 实测），所以一定存在"另一面已拥有的旗"。

1. `GET /lol-loadouts/v4/loadouts/scope/account` 取当前 `REGALIA_BANNER_SLOT`
2. `GET /lol-regalia/v3/inventory/REGALIA_BANNER` 取 `isOwned==true` 的键集合
3. 选一个 **id 不等于当前 itemId** 的已拥有旗帜，**复制当前 slot 对象，只替换 `itemId` 与 `contentId`，原样保留 `data` 字段**（R9 实测每个槽位都有这个社区文档没写的字段，不要丢），PATCH 上去
4. 回读 `scope/account` 确认 `REGALIA_BANNER_SLOT.itemId` 真的变了（**这一步是关键**：200 不等于生效，必须回读证实）
5. 不论结果，把原 slot 完整写回

判定：

- **回读确认变了 → 旗帜功能可以做**，解除 P4 的实现门禁。
- **200 但回读没变 / 4xx → 旗帜写入不成立**，前端旗帜卡保持只读 + 「客户端不支持」，结论写文档，不再猜别的端点。

**contentId 的取值来源：** 目标旗帜的 `contentId` 从 `regalia.json` 里同 id 的记录取（R3 的字段列表里有 `contentId`）。若 R5 返回的 `items` 数组里也带 contentId，优先用客户端自己给的那个。

**验收判据：**
- `TestR101BannerProbeSwitchesToDifferentOwnedBanner`：断言试写的 itemId ≠ 当前 itemId，且属于 `isOwned` 集合。
- `TestR101BannerProbePreservesDataField`：假 slot 里带 `data: {...}`，断言 PATCH body 的槽位对象里 `data` 原样存在。
- `TestR101BannerProbeReadsBackBeforeClaimingSuccess`：PATCH 返回 200 但回读 itemId 未变时，结果记为 `changed:false`、`ok:false`。
- `TestR101BannerProbeAlwaysRestores`：任何分支最后都把原 slot 写回。

**变异判据（必须 FAIL）：**
1. 目标 id 改成"当前那面旗" → 第一条 FAIL。
2. 构造新 slot 时丢掉 `data` → 第二条 FAIL。
3. 把 `changed` 直接写成 `status==200` 不回读 → 第三条 FAIL。
4. 去掉恢复 → 第四条 FAIL。

---

## P3 — 解除头像拥有态门禁（R4 已证实可用）

**要做什么：**
1. `facade_icons.go` 的 `iconOwnershipUnavailable` **不再恒为 true**：读 `/lol-inventory/v2/inventory/SUMMONER_ICON`，用 `itemId` 对目录 `id`，`owned==true` 记为已拥有。读失败/为空才回退 `true`（保留现有降级路径）。
2. 出参每条加 `owned bool`。仍然**不许下发** `uuid`/`purchaseDate`/`ownershipType`/`contentId`（R99-P1 的隐私断言继续生效）。
3. 拥有态缓存失效键必须带 `client`（R99-P1 已有要求，沿用）。
4. **前端（按口径 3）**：「只看已拥有」开关**默认开启**；关掉后未拥有的图标**置灰 + 加锁角标 + 不可点击**（`disabled`，不是只改样式）；计数 chip 显示「已拥有 478 / 共 5099」。

**验收判据：**
- `TestR101IconOwnershipParsedFromInventory`：fixture 给 508 条其中 478 条 `owned:true`，断言出参里 `owned==true` 的正好 478 条、`iconOwnershipUnavailable==false`。
- `TestR101IconOwnershipStillFallsBackOpen`：inventory 500 → `iconOwnershipUnavailable==true` 且目录完整。
- jsdom：默认渲染时「只看已拥有」是 on；关掉后未拥有的格子带 `disabled`。
- 真 Chromium（`desktop/r99-facade-picker-layout.cjs` 扩展）：断言未拥有格子 `pointer-events` 不可点 / `disabled` 属性存在。

**变异判据（必须 FAIL）：**
1. `owned` 恒 true → 478 条断言 FAIL。
2. 未拥有格子只加 class 不加 `disabled` → 前端断言 FAIL。
3. 默认开关改成 off → jsdom 断言 FAIL。

---

## P4 — 解除旗帜目录与拥有态门禁（**写入部分等 P2 的 W6 结论**）

**可以立刻做（只读部分）：**
1. 旗帜目录：`regalia.json` 取 `regaliaType=="kBanner" && isSelectable`（实测 47），剔除 `id=="1"` 与 `id=="2"` 的 11 个段位变体，剩 **35** 面。**`id`/`idSecondary` 是字符串，别定义成 int64。**
2. 拥有态：**只用 `/lol-regalia/v3/inventory/REGALIA_BANNER` 的 `isOwned`**。
   **★ 明确禁止用 `/lol-inventory/v2/inventory/REGALIA_BANNER`** —— R8 实测它返回数组且 `owned` 全 false，用了会把 4 面已拥有的旗全判成未拥有。在代码里写一行注释说明这是实测结论。
3. `isTencentOnly`（3 面）在界面上标「国服专属」，不要过滤掉。
4. 旗帜弹窗：分组降级为「全部 / 已拥有 / 国服专属」三档 + 按 `localizedName` 排序（R99-P4 已定的口径，因为 regalia.json 没有分类字段）。未拥有同样置灰不可选。

**写入部分：** W6 判定通过后才做，实现照 R99-P3 的既有要求（只碰 `REGALIA_BANNER_SLOT` 一个键、`safeLCUIdentifier` 守卫、保留 `data` 字段）。

**验收判据：**
- `TestR101BannerCatalogIs35AndIDsAreStrings`（沿用 R99 的 fixture 形状，`id` 用字符串）。
- `TestR101BannerOwnershipUsesRegaliaV3Only`：同时喂 `/lol-regalia/v3`（4 个 isOwned）和 `/lol-inventory/v2`（owned 全 false）两份数据，断言结果是 **4 面已拥有**，且断言**根本没有请求** `/lol-inventory/v2/inventory/REGALIA_BANNER`。

**变异判据（必须 FAIL）：** 把拥有态来源换成 `/lol-inventory/v2` → 上面第二条 FAIL（这条变异正是 R8 对照组要防的事）。

---

## P5 — 删除挑战旗帜配色与「切换上赛季旗帜」（口径 4）

1. 前端旗帜卡删掉「挑战旗帜配色」整行。
2. 「展示清理」卡删掉 `previous-banner` 按钮（`web/suite.js:1384` 那组定义里的一条）。
3. 后端 `applyFacadeActionResultDetails`（`profile_facade.go:736-813`）删掉 `case "previous-banner"`，并从 `facadeDiagnosticAction` 的枚举（`profile_facade.go:702-709`）里移除。
4. **`writeChallengePreferences` 本体不许动** —— `clear-challenges` 和 `clear-title` 还在用它，且它的头衔/勋章保全逻辑是 R66 的血泪教训，动了会回归。
5. 隐私声明 `features.go:748` 的 explicitWrites：把提到「上赛季旗帜」的那条删掉或改写；`quality_test.go:417` 的条数（当前 11）同步改，并确认 `:430` 的关键词列表里「旗帜」仍能被别的条目命中（新的旗帜卡文案里有）。

**验收判据：**
- `TestR101PreviousBannerActionRemoved`：`POST /api/facade/apply` 带 `action:"previous-banner"` 返回 400，且零 LCU 请求。
- `TestR101ClearChallengesStillPreservesTitle`：`clear-challenges` 仍然保留 `title`（R66 回归护栏，确认没被误删）。
- `TestPrivacyListsEveryClientWrite` 全绿。

**变异判据（必须 FAIL）：**
1. 顺手把 `writeChallengePreferences` 里的头衔保全删掉 → 第二条 FAIL。
2. 只删前端按钮、后端 case 留着 → 第一条 FAIL。

---

## P6 — 选人页锁定等待时间框（口径 5）

**证据：** 条件渲染在 `web/suite.js:616-621`（`const lockWait = strategy === "show-then-lock"`，非该策略时 `timingControls = ""`）。时间输入 `champSelectTimeInputHTML(kind, ms, disabled)` 在 `web/suite.js:635-640`，**第三个参数已经支持输出 `disabled`**。禁用/选用共用 `renderChampSelectSideCard`（`web/suite.js:610-633`），调用处 `web/suite.js:852`。

**要做什么：**
1. `timingControls` **无条件渲染**。`lockWait == false` 时：`−`/`＋` 按钮和输入框全部 `disabled`，输入框显示 **0**。
2. **★ 绝不把 0 写回设置。** 显示值走一个纯展示分支（`lockWait ? sideConfig.lockDelayMs ?? 10000 : 0`），`sideConfig.lockDelayMs` **一个字节都不改**。切回「亮出后锁定」必须还是原来的 10 秒。
   - 现有的两条写入路径都要确认不会被触发：步进按钮（`web/suite.js:922-927`）已 `disabled` 不会触发 click；输入框 change 监听（`web/suite.js:797-814`）**已有 `input.matches(":disabled")` 守卫**（`:798`），确认这条守卫仍然生效即可。
   - ★ 特别注意 `clampInt(side.DelayMS, 0, 10000, 0)`（`champselect.go:306`）在 `minimum==0` 时**会把 0 原样存下来**，所以只要前端不发 0，后端就不会被污染；反过来只要前端发了 0，就真的存成 0 了。
3. 禁用序列和选用序列两侧行为一致（共用函数，自然一致）。

**验收判据：**
- `TestR101LockWaitStaysVisibleAndDisabled`（jsdom / suite.test.cjs）：三种策略各渲染一次，断言 `[data-cs-time="lock"]` **三次都存在**；`show-only` 与 `lock-now` 时 `disabled` 为真且 `value === "0"`；`show-then-lock` 时可编辑且 `value === "10"`。
- `TestR101StrategySwitchNeverWritesZero`：模拟「亮出后锁定(10s) → 仅亮出 → 亮出后锁定」，断言全程 `saveChampSelect` 要么没被调用、要么 body 里 `lockDelayMs` 始终是 10000，**从未出现 0**。
- 既有 `web/suite.test.cjs:246-272` 必须仍然绿。

**变异判据（必须 FAIL）：**
1. 把置灰分支改成同时写 `sideConfig.lockDelayMs = 0` → 第二条 FAIL。
2. 把 `timingControls` 改回条件渲染 → 第一条 FAIL。
3. 去掉 `input.matches(":disabled")` 守卫 → 第二条 FAIL。

---

## P7 — 征召托管默认打开（**这条有三处连锁影响，别只改一个默认值**）

**证据：** 默认值在 `champselect.go:240-251` 的 `defaultChampSelectSettings()`，`Enabled` 是零值 `false`；挂载点 `watch_rules.go:147`。

**要做什么：**
1. `defaultChampSelectSettings()` 里 `Enabled: true`。
2. **★ 连锁一：不影响已保存用户。** `loadWatchSettings`（`watch_rules.go:192-196`）对**已经存在 `champSelect` 对象**的存档会直接 unmarshal 覆盖，所以老用户保持原样；只有全新安装和 schemaVersion ≤2 的老档走默认值。这是对的，**不要"顺手"给老档也刷成 true**。
3. **★ 连锁二：隐私声明会变成假话。** `features.go:749` 的 `automaticWrites` **10 条全部以「默认关闭；仅在工具自动页逐条开启后…」开头**，其中一条覆盖自动禁用/选用；`quality_test.go:424` 硬性要求 `automaticWrites` 里含子串「默认关闭」。
   → 必须把**自动禁用/选用那一条**改写成准确表述，例如：「**默认开启，但禁用/选用序列为空时不发送任何请求**；只有你自己往序列里填了英雄才会真正生效」。同时确认「默认关闭」这个子串仍能被**其余 9 条**命中，`quality_test.go:424` 才不会挂。如果不能，就要同步改测试关键词——**但必须是有意识地改，并在账本里说明为什么**。
4. **★ 连锁三：连接管理器的总闸。** `connection_manager.go:634` 是 `if !settings.MasterEnabled && !settings.ChampSelect.Enabled { return }`。默认打开后，这条链路会对以前"什么都没开"的新用户开始运行。**必须确认序列为空时确实一次写请求都不发**（现有行为是"全部不可用时不发送请求，并在本局记录说明原因"，见截图里的文案）。

**验收判据：**
- `TestR101ChampSelectDefaultsOn`：全新 `defaultChampSelectSettings()` 的 `Enabled == true`。
- `TestR101ExistingProfileKeepsItsOwnSetting`：存档里 `champSelect.enabled=false` 时，加载后仍然是 false。
- `TestR101EnabledWithEmptySequencesSendsNothing`：`Enabled=true` 但 ban/pick 序列为空，跑完一轮选人流程断言**零** PATCH/POST 到客户端。
- 既有 `diagnostics_1110_test.go:42-45` 和 `r78_test.go:151-153` 会因为默认值变化而失败，**必须逐条判断是"测试断言的是旧默认值、应当更新"还是"这条测试其实在守别的东西"**，在账本里写清楚每一条的处理理由。**不许无脑把 false 改成 true 了事。**

**变异判据（必须 FAIL）：**
1. 让 `loadWatchSettings` 把老档也刷成 true → 第二条 FAIL。
2. 序列为空时仍然发请求 → 第三条 FAIL。

---

## P8 — 生涯页左列重排 + 删除说明文字（口径 6）

**设计稿：** `design-mockups/r101-facade-left-column.html` / `_shots/r101-facade-left-column.png`。

**要做什么：**
1. 把 `.facade-icon-card`（`web/suite.js:1513`）和 `.facade-banner-card`（`web/suite.js:1514`）两个完整 `<section>` 从 `.facade-controls` 移到 `.facade-left`，插在 `.facade-preview` 之后、`.facade-write-card` 之前。两张卡是独立 section，整段搬即可。
2. 删除 `web/suite.js:1511` 里 `.facade-slots` 之后那个 `<p>这是好友悬浮卡与生涯页的示意预览…</p>`。**顺手把它对应的死规则 `web/suite.css:266`（`.facade-preview-body > p`）一起删掉**，别留孤儿 CSS。注意 `.facade-commit` 里那句「改动只在左侧预览，确认后才写入客户端。」（`web/suite.js:1515`）是另一处文案，**不要误删**。
3. **★ 392px 窄列的两个必要改动**（调研实测：段位旗那行按现在的左标签右控件排法只剩约 60px 余量）：
   - 旗帜卡的行改成**标签在上、控件在下**（设计稿里的 `.facade-stack-row`），不再用 `.facade-row` 的左右两栏。
   - 「上赛季最高段位」按钮文案缩短为「**上赛季段位**」，并给 `.suite-segment button` 补 `white-space: nowrap`（现在只有 `.cs-segment button` 有，`web/suite.css:448`）。不加这条，窄列里按钮文字会换行把控件撑高。
4. 头像卡补一个当前头像缩略图（设计稿里的 `.icon-current`），不然搬到左边只剩一个孤零零的按钮。
5. 事件绑定不用改：`bindFacadeControls()`（`web/suite.js:1657-1678`）全部用 `roots.facade.querySelector("[data-*]")`，不依赖列归属。

**验收判据：**
- 真 Chromium（扩展 `desktop/r99-facade-picker-layout.cjs` 或新开一个）：在 1500 / 1180 / 980 三档视口断言 ①头像卡与旗帜卡的 `offsetLeft` 落在左列范围内 ②段位旗分段控件 `scrollWidth <= clientWidth`（不溢出）③分段控件高度 ≤ 34px（证明没被换行撑高）④三档视口下都不出现横向滚动。
- jsdom：断言那段说明文字**不在** DOM 里，且 `.facade-commit` 那句**还在**。

**变异判据（必须 FAIL）：**
1. 去掉 `white-space: nowrap` → 分段控件高度断言 FAIL。
2. 把两张卡留在右列 → `offsetLeft` 断言 FAIL。
3. 把 `.facade-commit` 那句也删了 → jsdom 第二条 FAIL。

---

## P9 — Rookie 种子拿不到段位（图三：新账号排最后、不是主号）

### 真因（已定位，不用再查）

种子账号走 `resolveProSeed` 只解析了 Riot ID/PUUID，**造出来的 `opggProAccount` 的 `Rank` 字段是空的**。于是：

- `normalizeProAccount`（`pro_players.go:468-476`）`json.Unmarshal(raw.Rank, &rank)` 失败 → 直接 `return account, true`，`RankStatus` 停在初始值 `"unavailable"` → 界面显示「段位暂不可用」。
- 主号选取（`pro_players.go:639-642`）要求 `!Dormant && RankStatus == "ranked"`，种子永远不满足 → **当不上主号**。
- 排序 `proAccountLess`（`pro_players.go:492-512`）把无段位的排在有段位的后面 → **排最后**。

截图完全吻合：`dyjkbysb#KR1` 钻石 II 拿到「主号」徽章，而真正的主号 `모든일은같이#KR1`（外部源显示王者 2490LP）显示「段位暂不可用」并沉底。

### 要做什么

1. 种子解析完 PUUID 后，**补一次 `leagueEntries(ctx, puuid)`**（`riot_api.go:753-759` 现成封装，打 `/lol/league/v4/entries/by-puuid/{puuid}`）。
2. 从返回里取 `queueType == "RANKED_SOLO_5x5"` 那条，组装成 `normalizeProAccount` 认识的形状：`{"tier":"CHALLENGER","division":1,"lp":2490}`。
   - **`rank` 字段是罗马数字**（`riotLeagueEntry.Rank` = `"I"/"II"/"III"/"IV"`），要映射成 int：I=1 / II=2 / III=3 / IV=4。
   - **大师及以上 `division` 必须填 1**（`pro_players.go:488-493`：`proTierOrder[tier] < proTierOrder["MASTER"]` 时才校验 1–4，否则 division 固定 1）。
   - 查不到单双排条目时**不要瞎填**：留空走现有的 `unavailable` 路径（宁可显示"暂不可用"也不要显示错误段位）。
3. **TTL 与配额**：这条跟 by-puuid 反查同频即可（**6 小时**，不要用 `leagueEntries` 现有的 3 分钟 TTL——那是给玩家总览用的）。仍然带 `withRiotQueueLimit` 预算，超额当轮放弃、保留上次快照。稳态新增成本：每 6 小时 1 次。
4. 天梯名次那一列（截图里的 `#78,572`）种子拿不到，显示 `—` 即可，**本轮不做**。

### 验收判据

- `TestR101SeedResolvesSoloQueueRank`：假 Riot 返回含 `RANKED_SOLO_5x5 / CHALLENGER / I / 2490` 和一条 `RANKED_FLEX_SR`，断言出参 `RankStatus=="ranked"`、`Tier=="CHALLENGER"`、`LP==2490`、`Division==1`，且**用的是单双排那条不是灵活组那条**。
- `TestR101SeedBecomesPrimaryOverLowerRankedAccount`：上游给同一选手一个「钻石 II」账号，种子是「王者 2490」，断言 **种子排第一且 `Primary==true`**，钻石那条不是主号。（这条直接对应用户截图里的问题。）
- `TestR101SeedRankMapsRomanDivision`：`"II"` → `Division==2`；`"I"` 且 tier 是 MASTER/GRANDMASTER/CHALLENGER → `Division==1`。
- `TestR101SeedWithoutSoloQueueStaysUnavailable`：只返回灵活组时 `RankStatus=="unavailable"`，**不许回退成 unranked 或瞎猜**。
- `TestR101SeedRankRespectsQuotaBudget`：限流满额时不阻塞、保留上次快照。
- R99 的 `TestR99SeedNeverLeaksPUUID` 等隐私断言必须仍然绿。

### 变异判据（必须 FAIL）

1. 取 `entries[0]` 而不是筛 `RANKED_SOLO_5x5` → 第一条 FAIL（fixture 里灵活组排在前面）。
2. 罗马数字映射写死成 1 → 第三条 FAIL。
3. 查不到单双排时填一个默认段位（比如 UNRANKED 或 IRON） → 第四条 FAIL。
4. 把 TTL 改回 3 分钟 → 配额行为测试 FAIL（断言 6 小时内第二次调用零 HTTP）。

---

## P10 — 文档更新

1. `docs/r99-probe-results.md`：把「待测」全部替换成本文「P0 结果归档」的真实数据，并写明 R11 链路已关闭、R8 不可用于判拥有。
2. `docs/pro-players-sources.md`：**三处关于 Wenbo「没有可交叉确认的韩服账号」的记录已经过期**（`:37`、`:64`、`:93`）。Leaguepedia 的 ids 栏与 dpm.lol 各自独立把 `14小孩幻想赢对线` 绑到「杨文波 / BLG / 上单 / 2007-04-01」同一组身份事实，满足该文档自己定的交叉确认标准，完整账号 `14小孩幻想赢对线#4453`。
   **但按口径 7，本轮不录入这个账号**，只更新文档措辞并注明"等 OP.GG 小程序核对后再入库"。
3. 新增一节记录本轮的真机探测结论（头像 401、bannerAccent 400、REGALIA_BANNER_SLOT 存在）。

---

## 本轮明确不做的事

1. 除 Rookie 之外的任何选手账号录入 —— 等用户的 OP.GG 核对结果。
2. 旗帜写入的正式实现 —— 等 W6 回读证实。
3. 头像写入的正式实现调整 —— 等 W5 定因。
4. 牌位 / 头像框（`REGALIA_CREST` 实测空、`REGALIA_BORDER` 实测 400）。
5. `/lol-banners` 那套三角旗 —— 已确认死亡，永久关闭。
6. 种子账号的天梯名次列。
7. 生涯背景卡、聊天身份卡的任何改动（保持在右列不动）。

---

## 只能真机验收的清单

1. **W5**（头像换一个已拥有的图标）—— 这一条决定头像功能到底能不能做。
2. **W6**（旗帜换一面已拥有的旗 + 回读确认）—— 这一条决定旗帜功能到底能不能做。
3. 若 W5 走到 W5-b/W5-c，那两条的真实返回。
4. 头像弹窗默认只看已拥有（478 个）之后的真机滚动流畅度。
5. 征召托管默认打开后，全新安装的用户在序列为空时确实什么都不做。
6. Rookie 补上段位后，是否真的顶到第一位并拿到「主号」徽章。
