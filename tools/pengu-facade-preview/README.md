# 未拥有头像/旗帜「仅本地可见」实验脚本 v0.5.0 —— **已放弃，结论见 CLOSURE.md**

这是一个**运行在 LoL 客户端进程内的 Pengu Loader 脚本**，不是 Deep Legends 的一部分，
也不通过 LCU 写入任何数据。它只做一件事：拦下客户端读到的响应，把里面的头像/旗帜字段
换成你指定的目标值，于是**你自己屏幕上**的生涯页、藏品、选人页、好友栏、hovercard 都会
显示成目标外观。

## 已验证 / 未验证（重要）

| 项目 | 状态 |
|---|---|
| 改写引擎逻辑（39 个测试块、149 项断言） | ✅ 本地桩件全绿，见「本地验证」 |
| Pengu Loader 生命周期契约 | ✅ 已对照源码 `plugins/src/preload/loader.ts`、`rcp/socket.ts`、`types.d.ts` |
| XHR 改写手法 | ⚠️ **与 sona 不同**：sona 用 `addEventListener` + `defineProperty` 写死值，本脚本用惰性 own getter。理由见脚本「通道 1」注释——`xhr.onload=` 先于 `send()` 赋值时，sona 那条路线会读到未改写的原值。**真机未验证** |
| 服务端真值 | ✅ 完全不写，只改客户端读到的副本 |
| **国服真机上各端点的实际字段名与形状** | ❌ **未验证**，这正是第一次运行要产出的东西 |
| 生涯页/藏品/选人页是否真按假设渲染 | ❌ 未验证 |

所以第一次运行的目标不是「成功换头像」，而是**把 Console 里的 `[diag]` 结构日志带回来**。
拿到结构才能确定字段名对不对——按项目纪律，契约不足时不猜、不用推断值代替。

## 它做不到什么

- **别人看不到**。改的只是你本地客户端读到的数据，服务端真值没变。
- **不持久**。重启客户端后需要脚本重新施加；目标 ID 写在 `CONFIG` 里所以会自动恢复。
- **不能让别人也看到未拥有外观**。那需要服务端接受写入，已确认被所有权校验拒绝
  （`docs/history/ledgers/r106-execution-ledger.md`、`docs/r107-execution-ledger.md`；外部项目
  league-profile-tool 同样只做到聊天作用域）。
- **不改游戏文件、不联网、不上传任何数据、不写持久化存储**。

## 已知副作用

- **同一客户端里的其它 Pengu 插件**若读这些端点，也会读到改写后的值（无法只针对客户端
  自身排除）。介意就把 `CONFIG.rewriteWebSocket` 设 `false`，代价是客户端推送会把界面
  冲回真实值。
- **拥有态被改写后，客户端选择器里未拥有项不再置灰**——这是刻意的，不是 bug。
- `stats().seen` 里的结构摘要是**改写前**采集的服务端真值，可放心用于定契约；
  但 `socket.observe` 观测通道拿到的值**可能已被改写**，只能用来确认「有没有推送」，
  不能用来判断真值（脚本注释里也写明了这一点）。

## 使用

1. 安装 [Pengu Loader](https://github.com/PenguLoader/PenguLoader)，装完打开一次确认状态 ready。
2. 打开 Pengu 的插件目录：Loader 界面右上 **Plugins → 右下角「打开插件目录」**
   （即安装目录下的 `plugins/` 文件夹，例如 `D:\Download\Pengu Loader\plugins\`）。
   **不要**把文件放在安装目录根下，也不要找 `scripts/`——那个目录不存在，是我早期写错的。
3. 在 `plugins/` 里新建文件夹 `facade-preview`，把 `facade-local-preview.js`
   **改名为 `index.js`** 放进去，最终路径形如：
   `plugins\facade-preview\index.js`
   （sona 等已验证可用的插件都是「文件夹 + index.js」这个形状；Loader 不支持直接拖单个文件。）
4. 回到 Loader 点**刷新**，列表里应出现 `facade-preview`。
5. 编辑 `index.js` 顶部的 `CONFIG`：
   - `icon.id`：目标头像 ID。不知道填哪个就在客户端 Console 里跑
     `await __facadePreview.suggestIcons()`，它会列出几个「未拥有且本区域可用」的候选。
     填 `0` 表示不改头像。
   - `banner.enabled: true` + `banner.id`：目标旗帜，用 regalia 目录里的**字符串 id**
     （不是 `contentId`，也不是 `idSecondary`，见 `docs/r110-execution-ledger.md`）。
     只接受 `[A-Za-z0-9_-]`，其它输入会被拒绝（防原型污染）。
   - 第一次运行保持 `diagnostic: true`。
6. **重启 LoL 客户端**（脚本在 `init` 阶段挂 hook，必须早于客户端自己的请求）。
7. 在客户端里按 F12 打开 DevTools（或 Console 里执行 `__facadePreview.openDevTools()`），
   过滤 `[FacadePreview]`。

不想改文件也可以在 Console 里直接调：

```js
__facadePreview.suggestIcons()     // 未拥有候选；库存不可信时会**拒出**并告警（v0.3.3 起的安全网）
await __facadePreview.searchIcons('至臻')  // 按名称搜目录；ownedPerInventory 列在库存可用时可信
await __facadePreview.newestIcons(10)      // id 倒序（最新最可能未拥有）；挑 ownedPerInventory=false 的
__facadePreview.setIcon(4379)      // 换头像：**立即**伪造推送刷新已渲染界面 + 记住（重启后启动缓存界面也生效）
__facadePreview.setBanner('12')    // 换旗帜：同上，立即生效 + 记住
__facadePreview.applyNow()         // 手动重放一次「拉真值→改写→伪造推送」，界面立刻重渲染
__facadePreview.forget()           // 清除记住的选择（重启后恢复真实外观）
__facadePreview.stats()            // counts=各端点改写次数；seen=服务端真值结构摘要
copy(__facadePreview.dump())       // 一键导出排障 JSON 到剪贴板，直接粘给开发者
__facadePreview.disable()          // 只停改写，hook 仍在（仍会 parse 响应）
__facadePreview.stopAll()          // 停改写 + 停诊断；hook 无法卸载，需重载客户端才彻底干净
__facadePreview.reloadClient()     // 重载客户端
```

## 关闭

三档，按需选：

| 做法 | 效果 |
|---|---|
| `CONFIG.icon.enabled = false` + `CONFIG.banner.enabled = false`，重启客户端 | 不再改写任何值，但三个 hook 仍安装、命中端点仍会被 `JSON.parse` |
| 再把 `rewriteXhr / rewriteFetch / rewriteWebSocket` 全设 `false`，重启客户端 | 完全不拦截 |
| **删掉脚本文件** | 最干净，等于没装过 |

已安装的 hook 在运行时无法卸载，所以任何改动都要重启客户端才彻底生效。

## 需要带回来的东西

**一条命令就够**：在客户端 Console 里执行

```js
copy(__facadePreview.dump())
```

然后把剪贴板内容粘给我。这份 JSON 里已经包含定契约所需的全部信息：客户端版本
（`__llver`）、Pengu 版本、三个 hook 是否装上、每个端点的改写次数、**改写前采集的
服务端真值结构**（`seen`）、WS 预筛片段表、以及所有告警。
隐私口径：`puuid` / `summonerId` **不落值**，只报「是否已捕获」和长度。

除了 dump，再告诉我两件事：

1. **你眼睛看到的结果**：生涯页 / 藏品 / 选人页 / 好友栏，分别变了没有。
2. 如果 dump 里 `hooksInstalled` 有 `false`，或 `warnings` 非空，把 Console 里
   `[FacadePreview]` 开头的**告警行**（黄色）一并贴来。

（下面这些是 dump 里各字段对应的原始 Console 行，仅供手动核对，不需要逐条抄：
`[diag] current-summoner`、`[diag] summoner-profile`、`[diag] regalia-banner-inventory`、
`[diag] chat-me lol.regalia 原值`、`[diag] champ-select myTeam[0] keys`、
`WS 改写` / `WS 推送到达`。）

## 本地验证

桩件测试（`facade-local-preview.harness.mjs`，曾 39 块 / 149 断言）已按使用者要求删除。
本实验已放弃，全部结论见 `CLOSURE.md`；如需回归基线可按其结论重建。

## 覆盖的端点

| 端点 | 改写内容 |
|---|---|
| `/lol-summoner/v1/current-summoner` | `profileIconId`；顺带捕获自己的 puuid / summonerId |
| `/lol-inventory/v2/inventory/SUMMONER_ICON` | 目标项 `owned`/`isOwned`/`ownershipType`（仅改已存在的键）；缺失则按现有条目形状追加，`uuid` 换成唯一值 |
| `/lol-chat/v1/me` | `icon`；**仅当响应带 `lol` 对象时**改 `lol.bannerIdSelected`。`lol.regalia`/`lol.iconOverride` 只观测 |
| `/lol-summoner/v1/current-summoner/summoner-profile`、`/lol-summoner/v1/summoner-profile` | 只改确实存在的 `profileIconId`/`iconId`/`bannerId`/... 键，其余交给诊断 |
| `/lol-regalia/v3/inventory/REGALIA_BANNER` | 目标旗帜 `isOwned`/`owned` → true（支持对象与数组两种形态） |
| `/lol-challenges/v1/summary-player-data/local-player` | `bannerId`（保留 `title`/`crestId`） |
| `/lol-champ-select/v1/session` | `myTeam[]`/`theirTeam[]` 中**自己那一条**的 `profileIconId` 与 `bannerId`/`regaliaBannerId`/`banner` |
| `/lol-hovercard/v1/friend-info/{puuid}` | 仅自己的卡；头像**三个字段** `icon` / `summonerIcon` / `lol.profileIcon` 全改（真机实测不同界面读的不一样） |
| `/lol-regalia/v2|v3/.../regalia` | **自己的**才改 `profileIconId`（真机实测生涯页头像读这里）；`summoners/{id}/` 形态要与自己 summonerId 对上，否则不动。旗帜字段仍只观测 |

## 真机实测契约（2026-09-18，国服 Windows，客户端 127.0.0.1 webview）

第一轮真机日志（v0.3.1，目标头像 10005）定下来的事实，**这些是改代码的依据，不是推测**：

1. **生涯页头像不读 `current-summoner`**。该端点改写成功（`profileIconId 5917 -> 10005` +
   `XHR 改写` 日志都在），但生涯页纹丝不动；同一时刻生涯页拉的是
   `/lol-regalia/v2/summoners/{自己的id}/regalia`，里面带 `profileIconId=5917`。
   → v0.3.2 起 regalia 也改写 `profileIconId`（仅自己）。
2. **好友栏生效**，它读 hovercard 的 `icon`。但 hovercard 里头像有三个字段：
   `icon`、`summonerIcon`、`lol.profileIcon`（他人卡上观察到后者）。→ v0.3.2 三个都改。
3. `current-summoner` 的键全表：accountId, displayName, gameName, internalName,
   nameChangeFlag, percentCompleteForNextLevel, privacy, **profileIconId**, puuid,
   rerollPoints, summonerId, summonerLevel, tagLine, unnamed, xpSinceLastLevel,
   xpUntilNextLevel。**没有第二个头像驱动字段**。
4. **旗帜契约首次实证**：`lol.regalia` 是 JSON 字符串
   `{"bannerType":1,"crestType":1,"selectedPrestigeCrest":21}`；`lol.bannerIdSelected=6`
   是挑战旗帜 id；regalia v2 端点里 `bannerType=blank`、`crestType=prestige`、
   `selectedPrestigeCrest=21`，且**没有** `preferredBannerType`/`preferredCrest` 键
   （与 swagger / 国际服形状不同，别按文档猜）。
5. 本次会话**没有**出现 `icon-inventory` / `summoner-profile` / `challenges-summary`
   的 diag，也**没有** `WS 改写` / `WS 推送到达`。说明：藏品页和生涯页没拉这几个端点，
   且改写没被实时推送冲掉（好友栏保持了改写值）。藏品页到底拉什么、生涯页旗帜读什么，
   留待下一轮观测。
6. **`summoner-profile` 的真键**（第二轮实测）：`backgroundSkinAugments`、`backgroundSkinId`、
   `regalia`（JSON 字符串 `{"bannerType":1,"crestType":1,"selectedPrestigeCrest":21}`）。
   生涯页的背景与旗帜读的是这里——下一轮旗帜本地预览的改写目标。
7. **国服库存端点是可信的拥有态来源**（第三轮纠正第二轮的判断）：
   `/lol-inventory/v2/inventory/SUMMONER_ICON` 返回 508 条且 **508 条全 `owned=true`**——
   即「已拥有子集」。第二轮拥有集合为空是**登录前拉取失败的偶发**，不是字段形状问题。
   v0.3.3 的拒出防护保留作安全网（库存不可用时宁可不给候选）。
8. **「未拥有头像本地可见」已在第三轮证实**：使用者挑了一个确定未拥有的头像（4679），
   生涯页渲染成功。改写计数 `current-summoner ×2 / regalia ×13`，均命中自己。
   这是本项目六轮以来第一个真机成立的未拥有 case。
9. **好友列表上方自己的头像不随中途 setIcon 更新**：它绑定聊天 presence
   （`/lol-chat/v1/me` 的 icon），客户端**启动时读一次并缓存**；该窗口内 chat-me 读取 0 次、
   WS 推送 0 条，没有可改写的读取发生。要覆盖它：把 id 写进 `CONFIG.icon.id` 后**重启客户端**。
   （第三轮日志里出现的 hovercard 读取全是**好友的卡**，自己的 hovercard 未被拉取。）
10. 第二轮日志中段的「登录态失效 / 重新登录」与真值跳变（5917/21 → 1296/8）仍无定论；
    第三轮未再出现。验证时保持单账号。
11. **顶部栏 / 好友列表上方的头像不随中途 setIcon 更新**（第三、四轮实测）：它们在启动时读取
    并缓存。v0.3.5 的解法是**选择持久化**（Pengu DataStore，退路 localStorage）：
    `setIcon`/`setBanner` 记住选择，重启客户端后从第一个请求起就改写，全界面一次到位。
    `forget()` 清除。模块加载日志会打印「已恢复上次选择」。
12. **生涯页旗帜的读取源是 regalia v2，不是 challenge-summary**（第五轮实测）：
    生涯页窗口内 regalia v2 读取 12 次，而 `challenge-summary` 与 `summoner-profile` 均 0 次。
    v0.3.6 起改写 regalia v2 的 `bannerType`（**待实证假设**：装备时等于旗帜目录 id 字符串，
    未装备为 `blank`）；单变量——`crestType`/`selectedPrestigeCrest` 不动。
    `challenge-summary.bannerId` 与社交面 `lol.bannerIdSelected` 的改写保留（其它界面会读）。
13. **第五轮「更糟糕」不是回退**：该会话 setIcon 后**没有重启**，顶部栏/好友列表是启动缓存，
    本来就不会变；生涯页当时已开着、数据已缓存，回主页再回来才重新拉取——与第四轮「立刻成功」
    的差异只是第四轮恰好切了页。持久化的效果必须在**重启后**观察。

14. 国服 404 的端点（与项目记忆一致，永久排除）：
   `/lol-platform-config/v1/namespaces/Regalia/BannerSkinsEnabled`、
   `/lol-banners/v1/current-summoner/flags/equipped`、
   `/lol-trophies/v1/current-summoner/trophies/profile`、
   `/lol-player-behavior/v1/chat-restriction`。
15. **点击即生效，不需要重启或切页**（v0.4.0）：客户端界面是数据绑定的，已渲染视图只在收到
    WS 推送时重渲染。脚本持有全部 message 监听器引用（`wrapListener` 登记，上限 64），
    `applyNow()` 拉一次真值 → 套用改写 → 把改写后的 payload 包成 `OnJsonApiEvent_*` 帧
    （事件名规则照抄 Pengu `socket.ts` 的 buildApi）喂给客户端自己的监听器。
    纯本地、不写服务端、只改克隆体（T24 断言真值副本不被污染）。
    `setIcon`/`setBanner` 末尾自动调用；也可手动 `applyNow()`。
    **待真机证实**：国服客户端的 databinding 是否对这些合成事件照单全收。
16. **第六轮两个根因与假设更替**：
    (a) v0.4.0 的合成推送送不到客户端——客户端 databinding 用 `onmessage` **属性**而非
    `addEventListener`，而 onmessage 包装路径此前没登记 ws，`pushTargets` 只有 1 个
    （日志：「向 1 个客户端监听器伪造了 6 次推送」）。v0.4.1 在 onmessage setter 里登记 ws。
    (b) 旗帜假设 #1（regalia v2 `bannerType`）**证伪**：改写 20 次无视觉变化；
    `challenge-summary.bannerId` 改写 8 次也无视觉变化。剩下唯一被生涯页读取的旗帜载体是
    `summoner-profile.regalia` 这个 JSON 字符串（数值口径 `bannerType`）。
    v0.4.1 起改写该字符串内的 `bannerType`（假设 #2，单变量，crest 字段不动；解析失败放弃）。
17. **第七轮：推送通路确认修好，但生涯页/旗帜组件不吃推送**。`applyNow` 向 5 个监听器
    伪造 35 次推送后**顶部栏头像当场更新**；生涯页头像与旗帜仍不动——这类组件要么
    「显示时才发请求」，要么直接按自定义元素属性渲染。v0.4.2 增加 DOM 层：
    诊断扫描把「自己」那些 `lol-regalia-*-element` 的标签与全部属性打出来
    （`[diag] dom` 行，属性签名变化才重复打），并对已存在 `banner-id` / 头像类属性的
    元素直接设目标值；MutationObserver 兜底重扫。**下一轮把 `[diag] dom` 行贴回来**，
    即可拿死旗帜/头像渲染的属性词汇表，不再猜。
18. **第八轮回退的根治：让位机制（v0.4.3）**。此前预览把头像/旗帜钉死成目标值，
    每次读取都覆盖，导致**客户端原生切换被顶掉**（使用者反馈：客户端里切头像/旗帜失效、
    所有旗帜变成一样）。v0.4.3 起：我们的改写只作用于响应副本、从不改服务端真值，
    因此「服务端真值发生变化」只可能来自原生操作；`noteTruth` 观察到真值变化即
    `yieldPreview`——停用该种类预览、清除持久化、还原 DOM 补丁，客户端功能立即恢复。
    DOM 层同理：我们设过的属性被客户端改回即让位。T28 锁死该语义。
    **预览与原生不再互斥：原生操作永远优先。**
19. **第九轮「全都不行」的两个叠加 bug（v0.4.4 修）**：
    (a) 交付文件漏了 `const domPatched = []` 定义（编辑事故），`applyNow` 一走让位路径就
    `ReferenceError: domPatched is not defined`，推送全部中断 → 什么都不会变。
    日志特征：`applyNow:<uri> ReferenceError: domPatched is not defined`。
    (b) 让位真值键原先按「种类」存，但不同端点对同一种类本来报不同值
    （账号头像 5917 vs 聊天 presence 7235），启动时第二个源一读即误让位 → 预览自杀。
    v0.4.4 起真值键改为「源:字段」（`current-summoner:icon`、`chat-me:icon`、
    `hovercard:icon`、`regalia:icon`、`regalia:banner`、`challenge-summary:banner`）。
20. **第十轮：旗帜自杀的真正原因与 DOM 作用域收紧（v0.4.5）**。DOM 诊断拿到真实词汇表：
    本人作用域元素是 `lol-regalia-identity-customizer-element`（带 `puuid`/`summoner-id`，
    属性 `profile-icon` / `banner-id` / `banner-type` / `crest-type` / `prestige-crest-id`）；
    而 `lol-regalia-banner-v2-element` 是**选择器列表项**（每项一个 banner-id，无身份属性）。
    旧 `isSelfElement` 对无身份属性元素返回 true → 把每个列表项都改成目标值 →
    「所有旗帜都一样」+ 客户端重渲染列表触发误让位 → 旗帜预览自杀。
    v0.4.5 起 `isSelfElement` 要求**正向身份匹配**（member-type=current-player 或
    summoner-id/puuid 与本人一致），无身份标识的元素一律不补丁、不让位。T30 锁死。
    生涯页头像「立即性」仍受限于该页组件显示时才发请求；DOM 补丁已覆盖 identity-customizer，
    若真机仍不立即变，下一步读客户端 bundle 定位生涯头像元素的输入属性。
21. **第十一轮：服务器所有权墙从客户端内部撞上来（v0.5.0 架构转向）**。日志实证：我们的
    属性补丁让 identity-customizer 组件尝试 `POST /lol-challenges/v1/update-player-preferences`
    装备未拥有旗帜 → `400 RPC_ERROR: Player does not own REGALIA_BANNER 12` → 组件回滚属性 →
    旗帜永远显示不回去。结论：任何让客户端「装备」未拥有物品的路径都会撞服务器所有权墙
    （项目六轮卡住的同一堵墙，这次从客户端内部撞）。
    v0.5.0 起：属性补丁 `domPatch` 默认**关闭**；新增**像素层** `pixelPatch`（默认开）：
    只替换本人作用域元素 shadow DOM 里渲染出来的 img src（当前真值头像路径 → 目标路径），
    不碰输入属性、不触发保存、不撞墙。T31 锁死（本人 shadow img 被替换、非本人不动）。
    旗帜的像素替换待拿到 banner 资源 URL 映射后接入；下一轮 `[diag] dom-shadow` 行会给出
    shadow DOM 里 banner 图的 src 模式。
    新增 T29 假 DOM 测试：缺 `domPatched` 这类错误以后在桩件里就会炸，不会再到真机才暴露。

## 排查：Console 里 `__facadePreview is not defined`

这个报错的含义是**脚本在本次客户端会话里根本没被执行**（init 只要跑过一次，
这个全局就一定存在）。按顺序查：

1. **客户端是不是在放插件之前就启动了？** Pengu 只在客户端启动时读插件列表。
   完全退出 LoL 客户端**和**托盘里的 Riot Client，再重新启动。这是最常见原因。
2. 文件形状对不对：必须是 `plugins\facade-preview\index.js`（文件夹 + index.js）。
   放在安装目录根下、或文件夹里文件名不叫 `index.js`，都不会被加载。
3. 在 Console 过滤框里输入 `Pengu`，找这两类行：
   - `Loaded plugin "facade-preview/index.js".` → 加载成功
   - 红色 `Failed to load plugin ...` / `Failed to initialize plugin ...` → 把这一行贴给我
4. v0.3.1 起脚本自带信标：加载成功会弹 Toast「FacadePreview v0.3.1 已加载」，
   且 Console 第一行是 `[FacadePreview] module evaluated v0.3.1`。
   **连 module evaluated 都没有** = Pengu 没 import 本文件（回到第 1、2 步）。
5. 确认你拷的是新版：文件里搜 `suggestIcons`，搜不到就是旧版，重新从仓库拷。

DevTools 入口（客户端窗口内，不是 Loader 窗口）：v0.3.1 默认启动后 1.5 秒自动弹；
或按 **Ctrl+K** 唤出 Pengu 命令栏 → 选「打开开发者工具」；或 Console 里
`__facadePreview.openDevTools()`。在 Loader 管理窗口里按 F12 是没有反应的。

## 风险

- 这是**修改客户端行为**，属 Riot ToS 灰区，风险由使用者自行承担。Pengu Loader 与
  sona 在国内被广泛使用且不改游戏文件，但「广泛使用」不等于「被允许」。
- 端点名、字段名会随客户端版本漂移。失效时先开 `diagnostic` 打印结构再对照修，
  不要凭猜测改字段名。
- 新增规则时**必须同步维护** `installWebSocketHook` 里的 `pathFragments` 预筛表，
  否则该端点的 WS 推送不会被改写。

## 参考实现（均已核对源码，非推测）

- [WJZ-P/sona](https://github.com/WJZ-P/sona) `src/lib/xhr/core.ts`、`xhr/profile-privacy-rules.ts`
  （改写 LCU 响应骗过客户端 UI 的先例）、`features/rank-disguise.ts`（presence `lol.*` 可写且不校验）、
  `features/custom-banner.ts`（另一条路线：DOM 属性劫持，仅自己可见）、
  `lib/ember-hook.ts` + `features/chroma-unlock.ts`（Ember 组件 mixin，选人页可能要用）
- [Hanxven/LeagueAkari](https://github.com/Hanxven/LeagueAkari) `src/shared/types/league-client/chat.ts`
  （`ChatLol` 字段表：`bannerIdSelected`/`regalia`/`iconOverride`）、
  `views/toolkit/misc/SummonerProfile.vue`（`bannerAccent` 与 regalia 的实际调用）
- [VeryVeryCoolName/league-profile-tool](https://github.com/VeryVeryCoolName/league-profile-tool)
  `core/services/profile-icon/profile-icon.service.ts`（未拥有头像只能落到聊天作用域的实证）
- [PenguLoader/PenguLoader](https://github.com/PenguLoader/PenguLoader)
  `plugins/src/preload/loader.ts`、`plugins/src/preload/rcp/socket.ts`、`plugins/src/types.d.ts`
