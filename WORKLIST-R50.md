# WORKLIST-R50：从 LeagueAkari 迁移「随行」模块（设计 + 实现方案）

日期：2026-09-01
基线：LeagueAkari `1.5.2-beta`（`git clone --depth 1` 读源码，非猜测；引用均带 `文件:行号`）
本轮产物：本文件 + 可交互视觉稿 `design/r50-suixing-mockup.html`
本轮模式：**只出方案与工单，不改仓库代码**

---

## 0. 一句话结论

新增顶级菜单 **「随行」**（放在「收藏」下面），下挂四个页签 **值守 / 整备 / 门面 / 拾遗**。
四块功能全部可以用**纯 LCU HTTP + 现有 WebSocket 事件流**实现，**零原生插件、零管理员权限、零外网请求、零新增落盘数据**。
其中「拾遗」是本轮的重头：你要的「连斗魂之前历史活动的赠礼都能领到」——**真正的功臣不是 Akari 的代码，而是 Riot 的 `/lol-rewards/v1/grants` 端点本身**（Akari 在这条链路上一行特殊处理都没有）。详见第 5.4 节，那里也列出了 Akari 的三个实打实的缺陷，我们要修掉而不是照抄。
⚠️ 但「这个账本保留多久」这一步目前是**推断而非已验证事实**，E 组开工前必须先做一次真机抓样（工单 E-0），抓样结果决定这一页的投入规模。

---

## 1. 命名方案

### 1.1 顶级菜单：随行

现有一级菜单是「总览 / 英雄 / 对局 / 收藏」，四个都是两字功能名词，风格朴素。新菜单要挤进这一列，就不能太文艺。

**「随行」** = 这些功能的共同点是「在你打游戏的过程中一路跟着、替你把杂事处理掉」：值守是流程托管，整备是端侧维护，门面是身份展示，拾遗是奖励清扫。它们不属于「看数据」，而属于「陪着你」。

备选（换起来只是改几个字符串，随时可替）：

| 候选 | 理由 | 顾虑 |
|---|---|---|
| **随行**（推荐） | 覆盖四个异构页签，语气中性、与现有四项同调 | 略抽象 |
| 值守 | 最贴自动化，画面感强 | 覆盖不了门面/拾遗，且会和二级页签重名 |
| 备战 | 贴自动流程与设置锁 | 覆盖不了「领取」 |
| 助手 | 一看就懂 | 和整个 App 的定位撞名，等于什么都没说 |

### 1.2 四个页签：与上游全部不同名

| 上游（LeagueAkari） | 我们 | 副标题 | 为什么这么改 |
|---|---|---|---|
| 自动操作 › 自动游戏流程 | **值守** | 托管客户端流程 | 「自动化」是实现视角；「值守」是用户视角——它在替你盯着 |
| 工具集 › 客户端 / 游戏端 | **整备** | 游戏端与客户端维护 | 上游那个名字是把两个模块名拼在一起，不是功能名 |
| 工具集 › 百宝箱 | **门面** | 生涯展示与聊天身份 | 「百宝箱」是杂物抽屉式命名；这些功能其实全都只改「别人看到的你」，叫门面更准确也更诚实 |
| 工具集 › 领取工具 | **拾遗** | 未领取奖励清扫 | 上游自己的中文提示就是「找回一些遗忘的东西」——拾遗就是这个意思 |

四个页签是「值守 / 整备 / 门面 / 拾遗」，与一级菜单同为两字，成一个命名族。

---

## 2. 视觉稿

`design/r50-suixing-mockup.html` — 单文件，直接用浏览器打开，点左侧「随行」和二级页签可切换。已用真 Chromium 在 1440 宽下截图核对过四页排版。

设计稿用的是项目真实的 `data-theme="dark"` 令牌值（`web/app.css:45-68`）、真实类名（`.button` / `.state-chip` / `.notice` / `.toggle-control` / `.setting-card` 等），以及收藏页二级导航的两行结构（`web/index.html:105-107`），所以它不是"另一套设计"，是长在现有设计系统上的。

**四页各自的设计立意（这是与上游最大的差别，上游四页全是同一套 naive-ui 设置行 + 数据表格）：**

- **值守：相位轨**。顶部一条 `房间 → 匹配中 → 确认对局 → 英雄选择 → 游戏中 → 结算` 的横轨，当前阶段高亮（数据我们已经有了，SSE 的 `gameflow:<Phase>`）。每条规则卡挂在它会触发的阶段上，卡上直接写「触发于 确认对局 · ReadyCheck」。当前阶段的规则卡会加金色描边并显示倒计时（「1.2 s 后接受」）。**你一眼就知道"接下来会发生什么"，而不是看一列不知道何时生效的开关。**
- **整备：装置台**。左边一张只读的「安装与连接」信息卡（区服 / 安装根目录 / 配置目录 / 设置文件 / 端口状态），中间是本页主角——一块大的「设置锁」，两态清晰；右边一列纯手动的维护动作。底部一张「不做的能力」卡，逐条写清哪些需要管理员权限、为什么不做。
- **门面：预览 + 控制台**。左边一张按真实生涯卡比例做的预览（背景大图 / 头像 / 段位牌 / 签名条 / 勋章位），右边任何改动**先反映到预览，确认后才写客户端**。上游是一列干巴巴的设置行，你改完只能切到客户端去看效果。
- **拾遗：合流清单**。上游是**三张互相独立的表**（奖励 / 任务 / 事件中心），且它自己在 `ClaimTools.vue:7` 注释了「和 Rewards 里面重叠」。我们合成**一张表**，每行带来源芯片，重叠的条目直接标「与「事件中心」重叠」。底部粘性动作条带进度与失败计数。

---

## 3. 迁移取舍总表

风险分级口径：**低** = 只动客户端 UI 状态或社交/展示层；**中** = 代替玩家做出会写进对局的决策；**高** = 与其他玩家形成直接竞争（手速），或使用异常请求模式。

### 3.1 做（本轮范围）

| # | 功能 | 落到 | 依赖 | 风险 | 备注 |
|---|---|---|---|---|---|
| 1 | 自动接受对局 | 值守 | 纯 HTTP | 中 | **已存在**（`convenience.go:90`），本轮加延时参数 |
| 2 | 断线自动重连 | 值守 | 纯 HTTP | 低 | **已存在**（`convenience.go:98`），加可配延时 |
| 3 | 快速下一把（返回房间） | 值守 | 纯 HTTP | 低 | **已存在**（`convenience.go:94`），补 `WaitingForStats`/`PreEndOfGame` 两档 |
| 4 | 结算自动点赞 | 值守 | 纯 HTTP | 低 | 新增，**修上游的死配置**（见 5.1） |
| 5 | 跳过任务庆祝动画 | 值守 | 纯 HTTP | 低 | 新增 |
| 6 | 大乱斗阵营位置播报 | 值守 | 纯 HTTP | 低 | 新增，默认「仅自己可见」 |
| 7 | 房主自动转交 | 值守 | 纯 HTTP | 低 | 新增，**修上游的越界 bug** |
| 8 | 房间邀请自动处理 | 值守 | 纯 HTTP | 低 | 新增，默认全部「不处理」 |
| 9 | 自动开始匹配 | 值守 | 纯 HTTP | 中 | 新增，**不做「排队超时自动重排」** |
| 10 | 锁定游戏设置文件 | 整备 | 文件系统 | 低 | 新增，本组价值最高的一条 |
| 11 | 客户端维护五件套 | 整备 | 纯 HTTP | 低 | 重启 UX / 结束 UX / 启动 UX / 关闭客户端 / 断开连接 |
| 12 | 生涯背景（皮肤） | 门面 | 纯 HTTP | 低 | 新增 |
| 13 | 聊天状态 / 签名 / 展示段位 | 门面 | 纯 HTTP | 低 | 三者同一个端点 |
| 14 | 展示清理四件套 | 门面 | 纯 HTTP | 低 | 卸头像框 / 卸勋章 / 切上赛季旗帜 / 清空表情轮盘 |
| 15 | 奖励账本领取 | 拾遗 | 纯 HTTP | 低 | **核心**，历史活动赠礼靠这条 |
| 16 | 任务奖励领取 | 拾遗 | 纯 HTTP | 低 | 含任务链续扫 |
| 17 | 事件中心一键领取 | 拾遗 | 纯 HTTP | 低 | 含与奖励账本的重叠标注 |

### 3.2 不做（逐条给理由，设计稿里也写在页面上）

| 功能 | 风险 | 不做的理由 |
|---|---|---|
| 自动选 / 禁英雄 | 中 | 替你做出会写进对局的决策。上游 `lock-in-immediately` + `delaySeconds=0` 的响应速度不是人类能达到的。另外它的可选/可禁判定要同时依赖 `pickable-champion-ids` 和 `all-grid-champions` 两个正交数据源，grid 拿不到就整个英雄静默弃用——一个安静失效的自动化比没有更糟 |
| 备战席（bench）抢英雄 | **高** | 大乱斗备战席本质是玩家之间的手速竞赛。上游甚至做了「该英雄在席上停留累计 2.9 秒才动手」来规避抢夺战——这说明作者自己也知道这是在跟真人抢。这条越过公平线 |
| 英雄选择秒退 | **高** | 上游实现是 5 路并发死循环 spam `POST /lol-login/v1/session/invoke`（`ChampSelectOperations.vue:75-96`，注释 `Seana goes mad!`），请求形态异常；秒退本身还有低优先级队列惩罚 |
| 自动符文 / 召唤师技能 | 低 | 功能本身无害，但**项目里已经有了**（`POST /api/gameplay/runes/apply`、`/item-sets/apply`），且我们的实现比上游安全（上游在符文页满时会直接覆写 `pages[0]`，毁掉用户的第一个符文页）。不重复造 |
| 结束游戏端进程 / 快捷键杀进程 | — | 需要 Windows 原生插件 + 管理员权限 + 全局键盘钩子。且上游的实现有个逻辑陷阱：`isProcessForeground` 不可用时短路求值直接跳过前台判定，**会无条件杀掉所有游戏端进程**（`game-client/index.ts:126-140`） |
| 修正客户端窗口尺寸（FixLCUWindow） | — | 需要 WinAPI 调窗 + 管理员权限，收益只是修一个显示问题 |
| 游戏内键盘模拟发送消息 | 中 | 需要 native input + 管理员 + 前台判定，全套约 9600 行 ts/vue；模拟键盘输入与反作弊的关系不明朗 |
| 自定义模板 + Monaco 编辑器 | — | 上游用 `node:vm` 做沙箱，但把整个 `AkariManager` 实例注入了上下文（`custom-template-executor.ts:18-20`）——那不是安全沙箱，它自己的 UI 上都挂了红色风险提示。加上 Monaco 是 MB 级依赖，对一个无构建步骤的原生 JS 前端不划算 |
| 无尽狂潮工具 | — | 限时 PVE 模式，上游自己在 `loadouts.ts:21-23` 注释「下一次模式开放估计 API 会有很大变动」 |
| in-process（退结算 / 退房间） | 低 | 90 行的客户端急救按钮，与数据工具无关。「退出结算」的能力已经被值守的「快速下一把」覆盖 |
| 创建指定队列房间 | 低 | 国服很多队列 ID 服务端根本不开，做了也常年报错。可以放到以后 |
| **直播软件进程探测** | — | ⚠️ 上游 `client-installation/live-streaming-detector.ts` 每 20 分钟枚举一遍你的进程列表判断你是否在直播（名单含 obs64 / livehime / douyutool 等），源码自己的注释是 `try being a spyware`。**这条越过隐私红线，明确不迁移**，也请注意别在参考 `client-installation` 时顺手抄进来 |

### 3.3 值得做但排在本轮之后（记一笔，别忘了）

| 功能 | 价值 | 说明 |
|---|---|---|
| 好友工具（**只读**） | 中高 | `GET /lol-chat/v1/friends` + `GET /lol-store/v1/giftablefriends` 里的 `friendsSince`（成为好友的时间，冷门但好用）+ 每个好友的最后对局日期。**只做只读，不做批量删好友**——删好友不可逆、收益为零。注意上游是串行 for 循环逐好友请求（`FriendTools.vue:394-398`），迁移必须改成有并发上限 + 缓存，否则会重演 R43/R49 的串行超时坑 |
| 选人阶段发送评分到聊天 | 高 | in-game-send 的裁剪版：**只在选人/房间阶段用 `POST /lol-chat/v1/conversations/{id}/messages`**，零 native、零管理员。配合我们已有的战绩聚合能力，这是投入产出比最高的一条。上游的 `type:'celebration'` 消息**只有本地玩家可见**，这个技巧值得学（用来做不打扰队友的提示） |
| 战利品分解 / 开箱 | 中 | 上游 `LootTools` 的 `craft()` 是**空函数**（`LootTools.vue:118`），完成度约 15%，只值得抄它的类型定义。真要做，关键是理解 Riot 战利品靠 **recipe 驱动**：`disenchantRecipeName` 直接挂在每件战利品上（分解不需额外请求），开箱走 `GET /lol-loot/v1/recipes/initial-item/{lootId}`，右键菜单走 `GET /lol-loot/v1/player-loot/{lootId}/context-menu`（LCU 已算好 enabled / 缺什么）。**分解不可逆**，必须补上游注释掉的护栏 `NO_DISENCHANT_TAGS = ['prestige','nodisenchant']` + 二次确认 |

---

## 4. 与现有功能的重叠处置（这一节别跳过）

调研时发现两处已有实现，方案必须先把它们理顺，否则会做出两套打架的东西。

### 4.1 `convenience.go` 已经迁移过 auto-gameflow 的核心三条

`convenience.go` 文件头注释就写着「迁移自 LeagueAkari 的"便捷设置"」，已实现 `autoAccept` / `autoPlayAgain` / `autoReconnect`，落盘在 `convenience.json`，UI 在**设置页**（`web/index.html:244-246`）。

**处置：值守页接管并扩展它，设置页那三行删掉。**

- `convenienceSettings` 三个 bool 字段升级为规则表（保持向后兼容：旧的三个 bool 能读进来映射成 `{enabled:true, delayMs:默认值}`）。
- 现有的 `handlePhase` 状态机（`convenience.go:82`）、3 秒去重（`:105`）、play-again 的 1200ms 预等待（`:116`）全部保留——这些是已经在真机上跑通的，不要推倒重来。
- 设置页保留一行入口：「值守规则已移到「随行 › 值守」」+ 跳转按钮（用现有的 `deep-legends:navigate` 事件，`web/app.js:1817`）。

### 4.2 收藏页已有「账户与物品 · 战利品与奖励」

`web/index.html` 的 `#favorites-account-panel`，后端是 `RewardsAPI.PendingGrants()`（`lcu_api.go:558`）读 `GET /lol-rewards/v1/grants`，**只读**。

值得注意：它的过滤用的是 `isPendingRewardStatus`（`lcu_api.go:692`），是**黑名单**（排除 CLAIMED / FULFILLED / COMPLETED / EXPIRED / REVOKED），比 Akari 的 `status === 'PENDING_SELECTION'` **白名单覆盖更广**。这是我们的优势，保留。它还已经处理了 `DO NOT TRANSLATE` 占位符（`rewardTitle` → `rewardLocalizationPlaceholder`）。

**处置：职责切开，不合并。**

- **收藏 › 账户与物品 = 库存视图**（我拥有什么 / 有什么在等我），保持只读。在待领取奖励区加一个「去拾遗领取」跳转。
- **随行 › 拾遗 = 动作视图**（把它们领掉），负责写操作 + 任务 + 事件中心两个新来源。
- 后端 `RewardGrant` 模型复用，不要新建第二套。

---

## 5. 四个页签的实现方案

> 端点全部来自实际源码，已标注上游位置。国服真机上仍需逐条实测——按 `DESIGN.md:81` 的规定，**mock 自动测试不构成发布验收**。

### 5.1 值守

**触发源**：复用现有 LCU WebSocket → `connection_manager.go:107` 已经在广播 `gameflow:<Phase>`。

⚠️ **新规则的事件分支要加在 `connection_manager.go:104-142` 的 `ListenEvents` 回调里**（与现有的 gameflow-phase 分支并列），**不是** `lcu_events.go:92 lcuEventRefreshScope`。后者返回的是「数据刷新作用域」（`champselect`/`collection`/`full`/`account`），被 `connection_manager.go:134` 拿去触发防抖后的皮肤/账户/奖池**全量重扫**，而且它拿不到 `event.Data`——把 `/lol-honor-v2/v1/ballot` 加进去，结果是每次结算都附带一次收藏重扫，而点赞规则因为读不到 ballot 内容根本实现不了。
WS 本身订阅的是 `OnJsonApiEvent` 全量（`lcu_events.go:47`），**不用改订阅**，只需在回调里加 URI 分支。


| 规则 | 监听 | 动作端点 | 默认值 |
|---|---|---|---|
| 自动接受对局 | phase `ReadyCheck` | `POST /lol-matchmaking/v1/ready-check/accept` | 关，延时 **1.5s**（上游默认 0，我们不跟） |
| 断线自动重连 | phase `Reconnect` | `POST /lol-gameflow/v1/reconnect` | 关，延时 10s（上游硬编码，我们做成 3~30s 可配） |
| 快速下一把 | phase `WaitingForStats` / `PreEndOfGame` / `EndOfGame` | `POST /lol-lobby/v2/play-again` | 关；三档等待沿用上游常量：WaitingForStats **10000ms**、PreEndOfGame **3250ms**、EndOfGame **1575ms**（映射见 `lobby-flow-controller.ts:87-107`；注意 `context.ts:11/12/13` 的源码顺序是 3250/10000/1575，别按行号对号入座）。⚠️ 这三个常量原名是 `WAIT_FOR_BALLOT`/`WAIT_FOR_STATS`/`BUFFER`，语义是「等结算就绪 / 等点赞票出现 / 缓冲」，**不是给用户调的延时**，不要做成滑块暴露出去 |
| 跳过任务庆祝 | 事件 `/lol-pre-end-of-game/v1/currentSequenceEvent` 且 `data.name === 'missions-celebration'` | `POST /lol-pre-end-of-game/v1/complete/missions-celebration` | 关 |
| 结算自动点赞 | 事件 `/lol-honor-v2/v1/ballot` | ① `GET /lol-lobby/v2/party/eog-status` 拿房间成员 ② `POST /lol-honor/v1/honor` body `{honorType:'HEART', recipientPuuid}` ③ `POST /lol-honor/v1/ballot` | 关 |
| 房主自动转交 | `lobby.localMember.isLeader` | `POST /lol-lobby/v2/lobby/members/{summonerId}/promote` | 关 |
| 房间邀请处理 | 事件 `/lol-lobby/v2/received-invitations` | `POST /lol-lobby/v2/received-invitations/{id}/accept` 或 `/decline` | 关，策略表默认全 `ignore` |
| 阵营位置播报 | phase `ChampSelect` 且 `benchEnabled` 且 `gameMode ∈ {ARAM, KIWI}` | `POST /lol-chat/v1/conversations/{id}/messages` body **`{body, fromId, fromPid:'', fromSummonerId, id, isHistorical, timestamp:'', type}`**（不是只发一个消息串，`chat.ts:61-83`） | 关，默认 `type:'celebration'`（仅自己可见） |
| 自动开始匹配 | 自算的可开始状态 | `POST /lol-lobby/v2/lobby/matchmaking/search` | 关 |

**要修的上游缺陷（这些是我逐行核实过的，不是推测）：**

1. **点赞策略是死配置**。上游 `autoHonorStrategy` 有 5 个枚举值（`auto-gameflow/state.ts:5-10`），i18n 文案齐全，但 **honor-controller 里一次都没读过**，UI 上连选择器都没画。实际行为是硬编码的三级瀑布：房间队友 → 非房间队友 → **敌方玩家**。也就是说开了自动点赞，找不到队友时它会悄悄把票投给敌人。我们把这 4 个策略真正做出来：`优先房间队友 / 仅房间队友 / 任意队友 / 弃票`，**默认「优先房间队友」，且任何情况下不投给敌方**。
2. **房主转交的随机越界**。上游用了 exclusive 版 `randomInt(0, members.length - 1)`（`invitation-controller.ts:155`），导致 **n 个人里最后一个永远选不中**。我们用正确的区间。
3. **`rejectInvitationWhenAway` 文案与实现不符**。上游文案说「将拒绝所有房间邀请」，实现是打条 log 然后 `return`（`invitation-controller.ts:36-39`），实际行为是**忽略**。我们要么真的 decline，要么把文案改成「忽略」——二选一，别留这种坑。
4. **不做「排队超时自动重排」**。上游 `autoMatchmakingRematchStrategy` 会在排队超时后 `DELETE .../search` 取消，然后因为状态回到"可开始"又自动重排，形成循环。这跟自动接受一组合就是完整的无人值守挂机链路。我们只做「满足条件时开始匹配一次」，不做循环。
5. **`autoMatchmakingMaximumMatchDuration` 也是死配置**（全仓零消费方），不用管它。
6. ⚠️ **点赞与快速下一把在上游是互斥的**：`honor-controller.ts:44` 在 ballot 一出现时就**无条件** `cancelPlayAgain()`（连 `autoHonorEnabled` 都不看）。方案里这两条虽然列成两条独立规则，但落地时必须显式定义优先级——**点赞进行中，快速下一把顺延到点赞完成**——否则两者会互抢。B-7 的相位轨要能表达这个状态（结算节点显示「点赞中 · 下一把已顺延」）。

**技术要点**：上游全模块**没有任何重试逻辑**，所有 catch 都是 `logger.warn` + 一条 IPC 事件。我们的 `convenience.go` 目前也是这样（失败静默），本轮改成：失败时通过 SSE 广播一条 `watch:failed:<rule>`，前端在规则卡上显示「上次触发失败」。

### 5.2 整备

**A. 锁定游戏设置文件**（本组价值最高的一条，且不需要管理员）

```
① GET /data-store/v1/install-dir            → 返回安装根目录字符串
② 国服判定：region === TENCENT（我们的 region 来自客户端命令行 `--region`，见 `lcu.go:28/303` 的 platformInfo，不是 LCU auth）
   国服：filepath.Join(root, "..", "Game", "Config")
   其它：filepath.Join(root, "Config")
③ 目标文件：<config>/PersistedSettings.json
④ 锁定：os.Chmod(path, 0o444)   解除：os.Chmod(path, 0o644)
⑤ 读当前态：stat.Mode()&0o222 != 0 → writable，否则 readonly
```

上游用 `node:original-fs` 是为了绕开 Electron 的 asar 补丁，Go 侧不存在这个问题。
⚠️ **Windows 上 `os.Chmod` 只切换 `FILE_ATTRIBUTE_READONLY` 这一个位**：`0o444` / `0o644` 能用、`Mode()&0o222` 判读也能用，但「解除」之后权限位不会真的回到 0644。所以 UI 上只呈现「已锁定 / 可写入」两态，**不要显示八进制数字**（设计稿里那个「只读 0444」芯片改成「只读」）。另外只读属性对提权进程没有绝对约束力，文案不要承诺绝对不会被改写。
**护栏**：写之前必须校验目标路径确实在探测到的安装目录内、不是符号链接（沿用 `storage.go` 里已有的逃逸校验思路）；`GET /data-store/v1/install-dir` 返回 404 或空串时整个功能显示为"无法定位安装目录"，**不猜路径**（符合 `README.md:67` 的准确性规则）。

**B. 客户端维护五件套**（全部纯 HTTP，全部需要用户手动点 + 二次确认）

| 动作 | 端点 |
|---|---|
| 重启客户端界面 | `POST /riotclient/kill-and-restart-ux` ⚠️ 上游源码 `riotclient.ts:17` 漏了前导斜杠，我们要加上 |
| 结束界面进程 | `POST /riotclient/kill-ux` |
| 启动界面进程 | `POST /riotclient/launch-ux` |
| 关闭客户端 | `POST /process-control/v1/process/quit` |
| 断开连接 | 纯本地，走现有的断开逻辑 |

**C. 只读信息卡**：区服、安装根目录、配置目录、设置文件定位状态、UX 进程状态。
⚠️ **端口和令牌不展示、不落盘**。注意 `features.go:504` 的 `neverStores` 里写的是「LCU 临时令牌」，**并没有提端口**——端口不展示是我们自己加的谨慎，别在文案里说成「隐私声明明确禁止」。

### 5.3 门面

| 功能 | 端点 | 坑 |
|---|---|---|
| 生涯背景 | `POST /lol-summoner/v1/current-summoner/summoner-profile` body `{key:'backgroundSkinId', value:<skinId>}` | **客户端不校验是否拥有**。我们默认开「只显示已拥有」开关，关掉才允许选未拥有的，并提示可能被服务端还原 |
| 皮肤列表 | 复用项目已有的皮肤目录（收藏页那套），不要新拉 | 要展开 `questSkinInfo.tiers`（任务皮肤各阶段）；门面不提供炫彩设置 |
| 在线状态 | `PUT /lol-chat/v1/me` body `{availability}`，7 选一 | 「锁定离线」只需纠正 `offline → away/chat` 这一个方向 |
| 个性签名 | `PUT /lol-chat/v1/me` body `{statusMessage}`，空值 = 删除 | — |
| 展示段位 | `PUT /lol-chat/v1/me` body `{lol:{rankedLeagueQueue, rankedLeagueTier, rankedLeagueDivision}}` | 大师及以上不发 division；**只改好友悬浮卡显示，不影响真实段位/战绩/匹配**，页面上必须写清楚 |
| 卸下头像框 | `GET` → `PUT /lol-regalia/v2/current-summoner/regalia` body `{preferredCrestType:'prestige', preferredBannerType:<原值>, selectedPrestigeCrest:22}` | **需要等级 ≥ 526**（上游判据是 `summonerLevel <= 525` 即不足，`SummonerProfile.vue:77`，别写成 `< 525`）。上游只改文案不禁用按钮，我们直接把按钮换成「等级不足」state-chip |
| 卸下全部勋章 | `POST /lol-challenges/v1/update-player-preferences/` body `{challengeIds:[], bannerAccent:<从 GET /lol-chat/v1/me 的 lol.bannerIdSelected 取>}` | — |
| 切上赛季旗帜 | `POST /lol-challenges/v1/update-player-preferences/` body `{bannerAccent:'2'}` | 可能导致旗帜暂时不显示，要提示 |
| 清空表情轮盘 | `GET /lol-loadouts/v4/loadouts/scope/account` 取 `data[0].id` → `PATCH /lol-loadouts/v4/loadouts/{id}` body **`{loadout:{EMOTES_XXX:{inventoryType:'EMOTE', itemId:-1}, ...}}`**（13 个位点；注意有 `loadout` 外层包装和 `inventoryType` 字段，漏了会 400） | 不可逆，二次确认 |

**「登录时重设」的关键细节**：上游的 `CHAT_ME_SETTLE_DELAY_MS = 2000`（`login-automation-controller.ts:6`）**必须照抄**——聊天服务刚就绪时立刻写会被客户端覆盖。且每次连接只执行一次，用户手动操作要能抢占掉自动执行。

### 5.4 拾遗 ★ 本轮重点

#### 5.4.1 先说清楚「历史活动赠礼为什么能领到」

这一点我查了上游全部代码，结论和直觉不一样：**Akari 什么特殊处理都没做，没有活动 id 表、没有硬编码前缀、没有历史活动清单。**（我 grep 过 `lol-event-hub`，`event-hub.ts` 里所有路径都是模板串 `${eventId}`，整条链路只有一个魔法值 `'Unselected'`。）

真正的机制是三条彼此独立的路，用户看到的效果是叠加的：

1. **`GET /lol-rewards/v1/grants` 是 Riot 服务端的「玩家待发放账本」。** 状态机是「已授予但玩家尚未做出选择」→「已选择」。**推断**：这个账本不随活动结束而清空——只要你从没点过那个宝箱，grant 就一直挂在那儿，斗魂旧赛季、往届事件通行证的选择型奖励因此能被这一个端点一网打尽。上游给这一节写的中文提示是「找回一些遗忘的东西」，指向的就是这个行为。**这是主力，如果只做一个，做这个。**

   ⚠️ **诚实标注：上一段最关键的那句是推断，不是已验证的事实。** 源码里能查实的只有「Akari 没做任何特殊处理」（`event-hub.ts` 全是 `${eventId}` 模板串，整条链路唯一的魔法值是 `'Unselected'`）；至于服务端账本究竟保留多久、历史条目是否真的还在，**代码里没有依据，真机也没验过**。
   → **E 组开工前必须先做一次真机抓样**：连上国服客户端打一次 `GET /lol-rewards/v1/grants`，看返回里有没有跨赛季的历史条目、`dateCreated` 最早到什么时候。抓样结果决定「拾遗」的投入规模：有历史条目就按本方案全做；没有，则这一页退化成「当前奖励领取」，仍然有用，但价值要重新评估。
2. **`GET /lol-event-hub/v1/events` 会把客户端仍配置着的历史活动一起返回**，`unclaimedRewardCount` 是服务端算的，同样与活动是否结束无关。上游的过滤条件**只看 `unclaimedRewardCount` 非 0，不看 `endDate`**（`EventHubClaimTool.vue:141`）——这就是过期活动仍出现在列表里的直接原因。
3. **`GET /lol-missions/v1/missions` 里 `status === 'SELECT_REWARDS'` 的任务**同样是「奖励已就绪、只等玩家选」的挂起态，会长期滞留。

**三者会重叠**（同一份后端账本的不同视图）。上游自己在 `ClaimTools.vue:7` 注释了「和 Rewards 里面重叠, 因此只需要一个就行了」，UI 提示写的是「任意领取其一即可」。**这条提示必须复刻**，否则用户会以为自己重复领了。

#### 5.4.2 端点清单（全部）

**读**
```
GET  /lol-rewards/v1/grants                                   → 已有实现，复用 RewardsAPI.PendingGrants
GET  /lol-missions/v1/missions                                → 客户端侧过滤 status === "SELECT_REWARDS"
GET  /lol-event-hub/v1/events                                 → 过滤 eventInfo.unclaimedRewardCount > 0
GET  /lol-event-hub/v1/events/{eventId}/reward-track/items        ┐ 仅用于展示明细
GET  /lol-event-hub/v1/events/{eventId}/reward-track/bonus-items  ┘ 过滤 rewardOption.state === "Unselected"
```

**写**
```
POST /lol-rewards/v1/grants/{grantId}/select
     body {grantId, rewardGroupId, selections:[rewardId,...]}
PUT  /lol-missions/v1/player/{missionId}
     body {rewardGroups:[groupId,...]}
POST /lol-event-hub/v1/events/{eventId}/reward-track/claim-all
     无 body（服务端一把梭，不区分单项）
```

**四个必须原样搬的魔法字符串**：`PENDING_SELECTION`（我们用更宽的黑名单，见 4.2）、`SELECT_REWARDS`、`Unselected`、`DO NOT TRANSLATE`。

**零外部依赖**：名称、图标、本地化文案全部由 LCU 自带。**不需要自建活动元数据表、不需要图标 CDN、不需要翻译表。** 这条链路是整个项目里最干净的一条——比战绩链路（要靠 hexdata / OP.GG / QQ101 补聚合数据）简单一个数量级。图标走已有的 `/api/image?path=` 代理。⚠️ **但不能当成「即可」**：该代理有硬白名单（`lcu_api.go:550` 只有三条前缀，`main.go:734` 用 `sanitizeClientImagePath` 卡掉其余的），而 event-hub 的 `rewardOption.thumbIconPath`、missions 的 `reward.iconUrl` 究竟落在哪个前缀，**上游代码里查不到**（Akari 走自己的 LCU 全量代理，不做白名单）。
→ **E 组要先在真机上打一次 `reward-track/items` 和 `/lol-missions/v1/missions`，把实际图标路径前缀记下来**，必要时给白名单加一条（加白名单要同步改 `lcu_api_test.go` 的断言）。图标拉不到时降级为占位方块，不要让整行渲染失败。

#### 5.4.3 上游的三个缺陷，我们要修

1. **多选一宝箱被 `Math.random()` 替用户瞎选。** 上游 `RewardClaimTool.vue:168-179` 用 `ChoiceMaker` 等权重随机抽，UI 上**完全不给选择权**——你勾一行点领取，拿到什么全凭运气，领完才通过 toast 告诉你选了啥。
   → **我们把 `rewardGroup.rewards[]` 铺成可点选磁贴，用户自己选，`selections` 填用户点的那个 `id`。端点和 body 形状完全不用改。** 未选时该行不可领取并提示。
2. **批量领取一败俱败。** 上游的 `try/catch` 包在整个 for 循环外，第一个失败就把剩下全部中断，用户只看到一条 warning，不知道哪些成功了。另外 `ChoiceMaker` 在 `maxSelectionsAllowed > rewards.length` 时会直接抛异常（`choice-maker.ts:59-73`），也会连带中断整批。
   → **per-item try/catch + 成功/失败计数 + 失败不中断**，选择数量做 clamp。
3. **错误信息对用户毫无价值。** 上游直接显示 axios 原文 `Request failed with status code 400`。
   → **解析 LCU 返回的错误体**（`errorCode` / `message`），在失败那一行内联展示，并说明后果（如「该发放单已被处理，重新扫描后会消失」）。

**另外两处上游的实现不一致，迁移时统一掉**：
- EventHub 那个 tab 少了 `useActivated`（切走再切回不会自动刷新）、也没订阅任何 LCU 事件（只能靠 `sleep(2000)` 硬等）。我们统一：进页面刷新 + 订阅 `/lol-rewards/v1/grants` 和 `/lol-missions/v1/missions` 的 WS 推送（**这两条 Go 侧已经在监听了**，`lcu_events.go:109`）+ 领取后主动重扫。
- EventHub 拉明细用的是 `Promise.allSettled(events.map(...))` 且**这行没有 await**（`EventHubClaimTool.vue:172`），floating promise 导致 loading 状态提前结束。我们要 await，并加并发上限（照抄项目里已有的 `chan struct{}` semaphore 模式，参考 `gameplay.go:2442`）。

#### 5.4.4 任务链

上游 `MissionClaimTool.vue:71-73` 有条注释：「虽然是 SELECT_REWARDS, 但很多任务只是一个前置触发器，实际上的奖励领取是在此任务完成后自动触发的另一个任务」。

**实现上它完全没处理**——没有链条追踪、没有递归领取，`metadata.chain` 字段全仓零消费。它靠的是被动机制：领完之后 LCU 会推一次 `/lol-missions/v1/missions` 全量，新解锁的任务自己冒出来，**用户要再勾一次、再点一次**。

→ 我们做增强：**领取成功后自动重扫一次**，如果表里出现了新的 `SELECT_REWARDS` 任务且 `metadata.chain` 与刚领的同链，在该行打上「任务链 n/m」标记并提示可继续领取。**不做自动续领**（那会变成不受控的连锁写操作）。

#### 5.4.5 隐私

`features.go:504` 的 `neverStores` 明确列了「**战利品与待领取奖励明细**」不得落盘。

→ 拾遗的全部数据**只在内存中**，随请求生成随响应丢弃，页面关闭即消失。设计稿底部那条说明就是写给用户看的。**不要为了「加速下次打开」给它加磁盘缓存。**

⚠️ **还有一条容易漏的落盘路径：诊断日志。** `a.recordDiagnostic()` 会写进 `logs/diagnostics.jsonl`，而 `lcu.go:508/514` 的错误串里带完整请求 path——拾遗的写入 path 里就有 `{grantId}`。E-5「解析 LCU 错误体」如果顺手加个埋点，grant id 就落盘了。**拾遗相关的诊断事件一律只记录动作名、来源类型与 HTTP 状态码，禁止写入 grantId / eventId / missionId / 奖励标题。**

⚠️ 另外 `features.go:504` 现有的 `neverStores` 条目是「战利品与待领取奖励明细」——**任务奖励与活动奖励不在这句话的字面覆盖内**，必须扩写，否则声明与实际红线对不上。

---

## 6. 后端实现方案

### 6.1 新增文件（对齐单 package main 的现有风格，平铺在仓库根目录）

| 文件 | 职责 | 估计行数 |
|---|---|---|
| `watch_rules.go` | 值守规则表、状态机、持久化。**由 `convenience.go` 演进而来，不新起炉灶** | ~420 |
| `game_settings_lock.go` | 安装目录定位 + 设置文件 chmod | ~140 |
| `client_maintenance.go` | 五件套维护动作 | ~110 |
| `profile_facade.go` | 门面全部写操作 | ~320 |
| `claim_center.go` | 拾遗：三来源扫描 + 合流 + 去重标注 + 逐项领取 | ~480 |
| 对应 `*_test.go` | 每个新文件配套 | ~900 |

即**新增 5 个 `.go` 文件**（外加各自的测试）。`convenience.go` 由 B-1 演进成 `watch_rules.go` 后删除原文件，README 目录表里那一行是**改名**而不是新增一行。

### 6.2 新增路由（`main.go:365-424` 追加，全部包 `a.authorized(...)`）

```go
mux.HandleFunc("GET  /api/watch/rules",        a.authorized(a.handleWatchRules))
mux.HandleFunc("POST /api/watch/rules",        a.authorized(a.handleWatchRules))
mux.HandleFunc("GET  /api/rig/status",         a.authorized(a.handleRigStatus))
mux.HandleFunc("POST /api/rig/settings-lock",  a.authorized(a.handleSettingsLock))
mux.HandleFunc("POST /api/rig/maintenance",    a.authorized(a.handleClientMaintenance))
mux.HandleFunc("GET  /api/facade/state",       a.authorized(a.handleFacadeState))
mux.HandleFunc("POST /api/facade/apply",       a.authorized(a.handleFacadeApply))
mux.HandleFunc("GET  /api/claim/scan",         a.authorized(a.handleClaimScan))
mux.HandleFunc("POST /api/claim/execute",      a.authorized(a.handleClaimExecute))
```

**所有对客户端的写操作一律走 `(c *LCUClient) RequestJSON(ctx, method, path, body, target)`**（`lcu.go:459`）——它是唯一允许写的入口，前面有 `validateLCURequestPath` 路径校验（`lcu.go:519`）。**绝不把前端传来的路径直接透传给 LCU**，动作名 → 端点的映射必须写死在 Go 侧的白名单里。

### 6.3 持久化

| 数据 | 存哪 | 理由 |
|---|---|---|
| 值守规则（开关 + 延时 + 策略） | `convenience.json` 升级版（同一个文件，加版本号字段） | 需要后端行为，照 `convenience.go` 的既有模式。⚠️ **同一份文件只能有一个 writer**：`GET/POST /api/gameplay/convenience`（`main.go:382-383`）与 `web/gameplay.js:4265-4300` 那整段必须一并处置，见 A-4 |
| 门面「登录时重设」的签名与展示段位 | 同上文件里新开一个 section | 需要后端在登录时重放 |
| 设置锁状态 | **不存**，每次 stat 文件实时读 | 文件系统本身就是真相来源 |
| 拾遗的任何数据 | **不存**（隐私红线） | 见 5.4.5 |
| 纯 UI 偏好（上次停留的页签、列表筛选） | localStorage 前缀 `lol-loot-` | 与项目现有约定一致 |

### 6.4 限速与并发

- 这些全是**本机回环请求**，与 Riot 官方 API 的 90次/2min 配额（`riot_api.go`）**无关**，不要误接进那套限速。
- 拾遗扫描时的事件中心明细请求要加并发上限（信号量 4，照抄 `gameplay.go:2442` 的模式），避免 N 个活动 × 2 个请求同时打出去。
- 值守的动作沿用 `convenience.go:105` 已有的 3 秒去重。
- 不需要熔断（本机接口不存在上游限流）。

### 6.5 隐私声明同步（不做这一步等于埋雷）

改 `features.go:496-506` 的 `handlePrivacy` 并同步 `README.md:151`：
- `explicitWrites` 新增：生涯背景皮肤、聊天状态/签名/展示段位、头像框/勋章/旗帜/表情轮盘、奖励与任务领取、客户端维护动作、游戏设置文件只读属性。
- `automaticWrites` 新增：值守规则触发的客户端动作（**必须注明默认全部关闭、设置里可逐条关**）。
- `neverStores` **必须扩写**：现有条目「战利品与待领取奖励明细」覆盖不到任务奖励与活动奖励，改成「战利品、待领取奖励、任务与活动奖励明细」。
- `externalReads` **不变**——本轮零外网请求。

---

## 7. 前端实现方案

### 7.1 接入点（少一处就静默半残）

| # | 位置 | 改什么 |
|---|---|---|
| 1 | `web/index.html:40` 之后 | 加 `<button class="section-tab" data-section="suite" aria-controls="suite-panel" ...>` + 内联 SVG 图标 |
| 2 | `<main class="shell">` 内 | 加 `<section id="suite-panel" class="panel" role="tabpanel" hidden>`。⚠️ **必须是 `main` 的直接子元素**——`web/app.js:124` 的选择器写死了 `main > [role='tabpanel']` |
| 3 | `web/app.js:1785` | `sectionTitles` 加 `suite: ["随行", "托管流程 · 端侧整备 · 门面 · 拾遗"]` |
| 4 | `web/gameplay.js:4872` | 默认页白名单 `["overview","champions","live","favorites"]` 加 `"suite"` |
| 5 | `web/index.html:232` | `#setting-default-page` 加 `<option value="suite">随行</option>` |
| 6 | `web/index.html:10-19` | 加 `<link rel="stylesheet" href="/suite.css">` + `<script defer src="/suite.js"></script>` |
| 7 | `desktop/overview-render.test.cjs:15` | `SCRIPTS` 数组加 `"suite.js"`，否则 jsdom 冒烟测试形同虚设 |
| 8 | `.github/workflows/ci.yml` | 加 `node --check web/suite.js`，**并新增一步 `cd desktop && npm test`** |

⚠️ 关于第 8 项：`desktop/overview-render.test.cjs` 这个 jsdom 冒烟测试**目前根本不在 CI 里跑**（quality job 只有 `node --check web/app.js`、`node --test web/remaining-sort.test.cjs`、`node --check desktop/main.cjs`）。不把它接进 CI，F-5 那条断言就只在本地手跑时有效，等于没有护栏。

`go:embed` **不用改**（`main.go:45` 是 `web/*.js web/*.css web/*.html` 通配）。只有新增子目录图标资源才要动 `main.go:46-47`。

### 7.2 二级页签

**项目里没有 TabbedPage 抽象**，抄两个现成函数：`web/app.js:1731 activateTab(tab, tabs, activate)` + `:1741 setupTabKeyboard(tabs)`。
结构参考收藏页 `activateFavoritesPage`（`web/app.js:1850`），它已经处理了懒加载分支和顶栏副标题跟随，行为直接对齐。

### 7.3 两条硬约束

- **CSP `style-src 'self'` 没有 `unsafe-inline`**（`main.go:530`，被 `gameplay_test.go:3781` 逐字锁死）。HTML 里写 `style="width:60%"` **会被浏览器静默忽略**。所有动态尺寸（倒计时环、进度条、延时滑块位置）必须：渲染时输出 `data-*` 属性 → 渲染后 JS 遍历 `element.style.setProperty(...)`。范本是 `web/gameplay.js:4493 applyRenderedMetricStyles`。
- **响应式一律 `@container`，不用 `@media`**（R47 验收的硬性要求）。新页面自己声明 `container-name: suite-page; container-type: inline-size`。

### 7.4 可复用件

`.setting-card` + `.settings-grid`（`web/app.css:749-754`）就是现成的「标题 + 描述 + 右侧控件」布局，门面和整备直接用。
⚠️ 顺手修一个既有小 bug：`.setting-card + .setting-card` 用了 `var(--line-soft)`，**这个变量全项目从未定义**，所以设置项之间的分隔线目前根本不渲染。要么补变量，要么改成 `var(--line)`。

其余：`.button` / `.state-chip` / `.notice` / `.empty-state` / `.skeleton` / 全局 tooltip（`data-tooltip="文本"`，多行用 `&#10;`）全部照用——CSS 类是全局的，写 class 就有。

⚠️ **但 `showToast()` 用不了**：它有两份独立实现（`web/app.js:2115`、`web/gameplay.js:4645`），分别关在各自的 IIFE 里，**都没有挂到 window**（`app.js` 全文只导出了 `window.deepLegendsSelects`）。新建的 `web/suite.js` 调不到。见工单 A-6。

### 7.5 SSE

值守要实时反映相位与触发结果，需要新增事件字符串：`watch:fired:<rule>`、`watch:failed:<rule>`、`watch:armed:<rule>:<ms>`。
⚠️ **已知缺陷**：`web/app.js:2715` 的 `source.onerror` 只标记状态并触发一次 `refreshStatus`，**不重建 EventSource**。值守页依赖实时性，本轮顺带补上——否则相位轨会停在断线那一刻，比没有更误导。
注意别修过头：浏览器原生 `EventSource` 在传输层中断时**会自己重连**，只有 `readyState` 变成 `CLOSED`（服务端返回非 2xx 或 content-type 不对）才永久停。所以 F-4 必须**先判 `source.readyState === EventSource.CLOSED` 再退避重建**，无条件重调 `setupLiveUpdates()` 会和原生重连叠成双连接。

---

## 8. 工单

### A 组 · 骨架与信息架构

- **A-1** 新增顶级菜单「随行」，按 7.1 的 8 处逐一改到位。图标用内联 SVG（`fill: currentColor`），风格对齐现有四个。
- **A-2** 新建 `web/suite.js` / `web/suite.css`，实现四页签切换（复用 `activateTab` + `setupTabKeyboard`），页签切换要更新顶栏副标题、记住滚动位置、支持方向键/Home/End。
- **A-3** 顶部三个指标（值守规则 N/M、设置锁状态、待领取 N）接真实数据；未连接客户端时整页显示带操作说明的空状态，不显示空表格。
- **A-4** 设置页删掉「自动接受对局 / 快速下一把 / 断线自动重连」三行（`web/index.html:244-246`），换成一行跳转入口，走 `deep-legends:navigate`。
  ⚠️ **必须一并交代旧链路的去留**，否则会出现两个 writer 写同一份 `convenience.json`（正是第 4 节警告的「两套打架」）：`GET/POST /api/gameplay/convenience`（`main.go:382-383`）和 `web/gameplay.js:4265-4300` 那整段。三选一并写进代码注释：**删除** / **保留为只读兼容** / **转发到新路由**。（顺带确认过：只删 HTML 那三行不会崩，`gameplay.js:4270` 有 `convenienceToggles.some((node) => !node)` 守卫——但留着就是活孤儿路径。）
- **A-6** 在 `web/app.js` 导出 `window.deepLegendsToast = showToast`（对齐已有的 `window.deepLegendsSelects` 模式），否则 `web/suite.js` 里所有的成功/失败提示都没法弹。A-2 / D-4 / E-4 / E-5 都依赖这条。
- **A-5** 收藏 › 账户与物品的待领取奖励区加「去拾遗领取」跳转。两页职责按 4.2 切开，**不合并、不重复实现**。

### B 组 · 值守

- **B-1** `convenience.go` → `watch_rules.go`：三个 bool 升级为规则表，**保留旧字段的向后兼容读取**，保留已有的 3 秒去重与 play-again 1200ms 预等待。
- **B-2** 新增六条规则（点赞 / 跳过庆祝 / 阵营播报 / 房主转交 / 邀请处理 / 自动匹配），端点见 5.1。事件分支加在 **`connection_manager.go:104-142` 的 `ListenEvents` 回调**里，不是 `lcuEventRefreshScope`。
- **B-2b** 显式定义「结算自动点赞」与「快速下一把」的优先级：**点赞进行中时，快速下一把顺延到点赞完成**。上游是 `honor-controller.ts:44` 无条件 `cancelPlayAgain()`，我们要做成有序而不是互抢。
- **B-3** 点赞策略四选一真正落地，**任何情况下不投给敌方**（修上游死配置）。
- **B-4** 房主转交用正确的随机区间（修上游 exclusive `randomInt` 导致最后一个成员永远选不中）。
- **B-5** 邀请处理的「离开时拒绝」要么真 decline 要么改文案叫「忽略」，**不留文案与实现不符**。
- **B-6** 自动匹配**只做一次开始，不做超时重排循环**；页面上明写为什么不做。
- **B-7** 相位轨前端：接 SSE `gameflow:<Phase>`，当前阶段高亮，规则卡按触发阶段分组，命中阶段显示倒计时。倒计时环的角度用 `element.style.setProperty` 写，**不能内联 style**。
- **B-8** 规则触发失败要广播 `watch:failed:<rule>` 并在卡片上显示，不再静默。
- **B-9** 「这些我们不做」说明卡按 3.2 的四条落地，文案要写清理由而不只是"不支持"。

### C 组 · 整备

- **C-1** 安装目录定位：`GET /data-store/v1/install-dir` + 国服 `../Game/Config` 分支。定位失败显示"无法定位"，**不猜路径**。
- **C-2** 设置锁：`os.Chmod` 0444/0644，读态用 `mode&0o222`。写前校验路径在安装目录内且非符号链接。
  ⚠️ Windows 上 `os.Chmod` 只切换 `FILE_ATTRIBUTE_READONLY` 一个位，解除后权限位不会真回到 0644——UI 只呈现「已锁定 / 可写入」两态，**不显示八进制数字**（设计稿里的「只读 0444」芯片要改成「只读」），文案也不要承诺"绝对不会被改写"。
- **C-3** 客户端维护五件套，全部二次确认。⚠️ `kill-and-restart-ux` 记得补前导斜杠。
- **C-4** 只读信息卡；**端口与令牌不展示不落盘**，页面上写明。
- **C-5** 「不做的能力」说明卡，含**明确写出不做直播软件探测**。

### D 组 · 门面

- **D-1** 生涯背景：英雄选择器 + 皮肤胶片条，默认开「只显示已拥有」；不提供炫彩设置。
- **D-2** 聊天身份三件套（状态 / 签名 / 展示段位），大师及以上禁用分段选择。
- **D-3** 「登录时重设」照抄 2000ms 延时，每次连接只执行一次，手动操作可抢占。
- **D-4** 展示清理四件套，全部二次确认；头像框在等级 < 525 时直接显示「等级不足」而不是给一个点了没用的按钮。
- **D-5** 左侧实时预览：右侧任何改动先反映到预览，确认后才写客户端。
- **D-6** 「本页会写入客户端的内容」说明卡。

### E 组 · 拾遗

- **E-0**（**前置，其余 E 组工单都等它**）真机抓样：连国服客户端打一次 `GET /lol-rewards/v1/grants`，记录：① 是否存在跨赛季的历史条目、`dateCreated` 最早到什么时候；② `status` 实际出现过哪些值；③ 多选一奖励组的 `selectionStrategyConfig` 形状。同时打一次 `GET /lol-event-hub/v1/events/{id}/reward-track/items` 和 `GET /lol-missions/v1/missions`，记下 `thumbIconPath` / `iconUrl` 的**实际路径前缀**（决定要不要给 `lcu_api.go:550` 的图片白名单加一条）。**抓样结果只看不存**，记进工单不落盘。
- **E-1** 三来源扫描合流成一张列表，复用 `RewardsAPI.PendingGrants`（**不要新建第二套 grant 模型**）。
- **E-2** 重叠检测与标注 + 「任意领取其一即可」提示。
- **E-3** 多选一宝箱做成**可点选磁贴由用户决定**；未选时该行不可领取。选择数量对 `maxSelectionsAllowed` 做 clamp。
- **E-4** 逐项领取：per-item try/catch、失败不中断、成功/失败计数、进度条。
- **E-5** 失败原因解析 LCU 错误体（`errorCode`/`message`），内联展示在失败行并说明后果。
  ⚠️ **不要顺手加诊断埋点**：`lcu.go:508/514` 的错误串带完整请求 path，而拾遗的写入 path 里有 `{grantId}`，`a.recordDiagnostic()` 会把它写进 `logs/diagnostics.jsonl`。拾遗相关的诊断事件**只能记动作名、来源类型与 HTTP 状态码**。
- **E-6** 任务链：领取后自动重扫，同链新任务打「任务链 n/m」标记；**不自动续领**。
- **E-7** 筛选：全部 / 历史活动 / 需要选择 / 任务链 / 已失败。
- **E-8** 事件中心明细请求加并发上限（信号量 4）并真正 `await`（修上游的 floating promise）。
- **E-9** 订阅已有的 `/lol-rewards/v1/grants`、`/lol-missions/v1/missions` WS 推送做实时刷新。
- **E-10** **确认全链路无落盘**，并在页面底部向用户说明。

### F 组 · 收口

- **F-1** `features.go` 的 `explicitWrites` / `automaticWrites` 按 6.5 更新，同步改 `README.md:151` 的隐私说明。
- **F-2** `README.md:172` 的目录表：新增 5 个 `.go` 文件各一行，并把 `convenience.go` 那一行**改名**为 `watch_rules.go`（不是新增，原文件由 B-1 演进后删除）。
- **F-3** 修 `--line-soft` 未定义导致 `.setting-card` 分隔线不渲染的既有 bug。
- **F-4** 补 SSE 断线重连（`web/app.js:2669`）。
- **F-5** `desktop/overview-render.test.cjs` 的 `SCRIPTS` 加 `suite.js`，并新增一条断言：切到随行页后四个页签容器都能渲染出内容（防"永久骨架屏"类故障）。
  ⚠️ **同时把这个测试接进 CI**——它目前根本没在 CI 里跑过（`.github/workflows/ci.yml` 的 quality job 只有三条 `node --check` / `node --test web/remaining-sort.test.cjs`）。不接进去，这条断言等于没有。
- **F-6** 同步 `DESIGN.md:80` 那条「账户与物品页……不提供任何写操作」的表述——A-5 只加跳转不加写操作，本身不违反，但在新架构下这句话需要重述成「只读，写操作在随行 › 拾遗」。
- **F-7** 设计稿 `design/r50-suixing-mockup.html` 里两处随核验订正的文案：整备页「只读 0444」→「只读」；门面页头像框条件「≥ 525」→「≥ 526」。（设计稿不进构建，改它只是别让实现者照着抄错。）

---

## 9. 验收标准

1. **每一条对客户端的写操作都必须在 Windows 国服真机上实测过**读取链路、写入链路、以及各自的失败降级。按 `DESIGN.md:81`，**只跑 mock 测试不构成验收**。
2. **护栏类测试一律先做变异测试再判定**：把被测代码故意改坏，确认测试真的 FAIL 才算有效。注意 R48 的教训——**变异插入点会影响结论**，插在无条件重置之前会被吸收而假 PASS，要换更贴近真实回归的插入点重试。
3. **默认全关**。值守九条规则、门面两条「登录时重设」，安装后默认全部关闭。验收时新建一份干净的数据目录确认。
4. **不可逆动作全部有二次确认**：展示清理四件套、关闭客户端、（未来的）战利品分解。
5. **拾遗不落盘**：跑一轮完整领取后，`grep -ri` 数据目录不应出现任何奖励标题、grant id 或 puuid。
6. **无障碍**：四个页签、九张规则卡、拾遗清单的勾选与磁贴选择**全部可用键盘完成**；状态不能只靠颜色表达（`DESIGN.md:69` 与 `:96`；65/94 是章节标题不是条款）。
7. **CSP 不破**：`gameplay_test.go:3781` 的 CSP 断言必须仍然通过；新页面不得出现任何内联 `style="`。
8. jsdom 冒烟测试覆盖新页面；`gofmt` / `go vet` / `go test -race` / `node --check` 全绿。

---

## 10. 风险清单

| 风险 | 等级 | 处置 |
|---|---|---|
| 自动接受 + 自动匹配组合成无人值守挂机链路 | 中 | 不做超时重排循环；自动接受默认延时 1.5s 而非 0；两者默认都关 |
| 展示清理是不可逆的（尤其清空表情轮盘） | 中 | 二次确认 + 文案明写「不可撤销」 |
| 生涯背景可设置未拥有的皮肤 | 低 | 默认开「只显示已拥有」；关掉时提示可能被服务端还原 |
| 设置锁 chmod 到错误的文件 | 中 | 路径必须在探测到的安装目录内 + 非符号链接；定位失败就整个功能不可用 |
| 领取端点的服务端行为未知（如 PUT 重复领取） | 中 | 领取动作**不做自动重试**（上游的 axios-retry 会重试 PUT，这是隐患）；失败交给用户手动重来 |
| `/lol-event-hub/v1/events` 在活动结束多久后消失 | 低 | 代码里没有依据，需真机长期观察。UI 不做任何基于时间的假设 |
| 新页面渲染异常导致永久骨架屏 | 中 | F-5 的 jsdom 断言就是为这个准备的 |
| 参考 `client-installation` 时误抄进直播探测 | **高** | 明确记在 3.2 和 C-5；code review 时专项核查 |

---

## 附：本轮调研的可复用资产

- LeagueAkari 源码可以在沙箱里直接 `git clone --depth 1` 读，比翻网页高效得多。
- 上游 `type:'celebration'` 的聊天消息**只有本地玩家可见**——这个技巧在做「不打扰队友的提示」时很有用，未来做选人阶段评分推送可以用。
- `GET /lol-store/v1/giftablefriends` 返回里带 `friendsSince`（成为好友的时间），是个冷门但好用的接口，记在 3.3。
