# WORKLIST-R44（给 GPT 执行）

> **先说三条最重要的发现，其余都是布局活：**
>
> 1. **J 组等了三轮的真机数据这次拿到了，但结论是"探测本身设计错了"**——见 J 组，
>    这条要先修探测再谈修功能。
> 2. **用户问的"数据源是否和英雄详情一致"，答案是：上游完全一致，同一个 loader 同一份缓存。**
>    所有差异都出在**投影层和渲染层**（对局页额外砍了核心装、按错误的条件隐藏了区块、丢了一个字段、
>    零值当有效值渲染）。所以不需要换数据源，只要把这四处对齐即可。
> 3. **`resg.top` 和 `hexdata.com.cn` 是两个东西，用户混淆了。** 项目里接的是 hexdata.com.cn，
>    `resg.top` 在 Go 代码里零引用（只存在于早期设计文档）。见 A-5。
>
> **另外：H 组的日志去重效果非常好。** 这份 73 分钟的日志只有 728 行，而 `diagnostic_dedup`
> 显示折叠掉了 69,000 条重复记录（`ranked_data_source_decision` 57600 条 +
> `ranked_winrate_resolved` 11400 条）。不去重的话这份日志会是 69,728 行。这轮终于能看到真东西了。

---

## J 组（P0，续）：探测数据拿到了，但探测打在了错误的模式上

### J-1 真机结果

`lcu_gameflow_session_shape` 这次真的落盘了：

```json
{
  "top_level_keys": ["gameClient","gameData","gameDodge","map","phase"],
  "team_one_player_keys": ["championId","lastSelectedSkinIndex","profileIconId","puuid",
    "selectedPosition","selectedRole","summonerId","summonerInternalName","summonerName",
    "teamOwner","teamParticipantId"],
  "team_two_player_keys": [同上],
  "time": "2026-08-30T12:27:46Z"
}
```

**player 对象里没有任何 subteam 字段**——没有 `playerSubteamId`，也没有 `subteamId`。

### J-2 但这个结论现在还不能用，因为探测打错了模式

12:27:46 这个时间点对应的是**海克斯大乱斗**对局（同一秒的 `recommendation_mode_resolved`
显示 `game_mode:"KIWI", queue_id:2400`）。而 `lcu_gameflow_session_shape` 用的是
`sync.Once`（`a.lcuGameflowShapeOnce`），**整个进程只会触发一次**——它触发在了一个
5v5 的海斗对局上，此后无论用户打多少局斗魂都不会再记录。

所以现在只能证明"海斗的 session 里没有 subteam"，**不能证明斗魂也没有**。
斗魂是 CHERRY 模式、队伍结构完全不同，很可能字段就不一样。

### J-3 这轮要做的

1. **把探测从"整个进程一次"改成"每种 modeGroup 各一次"**：
   去重键改成 `phase + gameMode`（或 `queue_id`），并加上限轮转（参考
   `recordLiveRosterShape` 这轮改好的指纹去重写法，`gameplay.go:3376-3428`）。
   这样打一局斗魂就能立刻拿到 CHERRY 的字段列表。
2. **探测载荷里补上 `game_mode` 和 `queue_id`**——现在这两个字段没记，
   导致我只能靠时间戳去和别的事件对时间才能判断它打在了哪个模式上，很脆弱。
3. **重点关注 `teamParticipantId` 这个字段**：它在 5v5 里大概率恒等于 teamId，
   但在斗魂 3x6 里**很可能就是小队标识**。拿到斗魂的探测结果后先看这个字段的取值分布。
4. **在拿到斗魂真机探测结果之前，仍然不要写任何解析代码。**
   这条纪律上一轮执行得很好（老实停手 + 删掉了那段永远不会生效的死代码），继续保持。

---

## A 组（P0）：对局页与英雄详情的数据对齐 —— 不是换数据源，是修四处投影/渲染

### A-0 先看这张对照表（回答用户"数据源是否一致"的问题）

| 模块 | 海斗英雄详情 | 海斗对局页 | 斗魂英雄详情 | 斗魂对局页 |
|---|---|---|---|---|
| 入口 | `champions.go:747` `loadDetail` | **同一个** `loadDetail`（`gameplay.go:3310`） | 同左 | 同左 |
| 分发 | `champions_structured.go:752` `loadMayhemDetail` | 同 | `:755` `loadStructuredDetail` | 同 |
| 海克斯 | hexdata `/hero/{id}-{slug}` + OP.GG RSC 合并 | **同一份** | op.gg `augment_group` | 同 |
| 核心装 | OP.GG RSC，上游只有 5 行 | 同 | op.gg 15 条→YOUR.GG 覆盖 | **被砍到 5** |
| 出门装/召技 | OP.GG RSC，**有数据且详情页正常展示** | 后端有，**前端不渲染** | 上游真没有 | 同（正确） |
| 装备排行 | hexdata `ItemRanking`，带真实胜率/场次 | **bundle 里根本没这字段** | 无 | 无 |
| 出装行统计 | 上游无 | 同，但**零值被渲染成 0%** | op.gg 有真值 | 同 |

**结论：上游完全一致（同一个 loader、同一个 mode、同一份缓存），不需要换数据源。**
四处差异全在投影层/渲染层，下面逐条修。

### A-1（P0）海斗的出门装和召唤师技能：上游有、后端有、前端把它藏了

- **实测证据**：`curl -H "RSC:1" https://op.gg/lol/modes/aram-mayhem/{champ}/build` 返回里
  `starter_items_0/1`、`spell_0/1` 都在；后端 `hexdata.go:2219/2224` 已解析、
  `gameplay.go:3642-3643` 已下发。
- **卡点在前端** `web/gameplay.js:3673-3674`：`!capabilities.hasAugments ? 渲染 : ""`。
  海斗 `hasAugments=true`，所以这两个 `<section>` 直接输出空串。
- **对比**：英雄详情页 `web/champions.js:918-925` `renderMayhemOpeningConfiguration` 是正常展示的
  ——正好印证用户说的"详情页有、对局页没有"。
- **斗魂不是 bug，别一起改**：实测 `lol-api-champion.op.gg/api/global/champions/arena/{id}` 的
  `starter_items` 是空数组、`summoner_spells` 字段根本不存在。所以
  `champions_structured.go:1230-1233` 跳过这两项对斗魂是**正确**的。

**改法**：判断条件从 `hasAugments`（模式能力位）改成**"这一项到底有没有数据"**
（`starterOptions.length > 0` / `spellOptions.length > 0`）。
这样海斗自动显示、斗魂自动隐藏，一个条件同时管两个模式，也不用再加新的能力位。

### A-2（P0）核心装太少：四处硬编码 5，其中两处是真丢数据

| 位置 | 上限 | 影响 | 该不该改 |
|---|---|---|---|
| `champions_structured.go:1242` | 5 | 排位/大乱斗；日志 `structured_recommendations {kind:"core", in:15, returned:5}` 就是这行，**上游给 15 扔了 10** | **要改** |
| `gameplay.go:3327` + `capGameplayCoreOptions:3486-3491` | 5 | **斗魂在这真丢**：op.gg 15 条、YOUR.GG 更多，详情页折叠 6 条展开全给，对局页只剩 5 | **要改** |
| `web/gameplay.js:3663` `slice(0,5)` | 5 | 前端又砍一次 | **要改** |
| `hexdata.go:2221` `rscCoreItemRows(expanded, 5)` | 5 | 海斗；**实测整份 RSC 只有 `core_items_0..4` 共 5 行，上游就这么多** | **不用改**（改了也没数据） |

**改法**：前三处提到和英雄详情页一致的口径（详情页折叠 6 条、展开全给）。
建议做成常量而不是继续散落字面量。

### A-3（P0）`126697` 是合法物品被误杀，物品 ID 范围假设从根上就是错的

- 校验在 `hexdata.go:2101-2102`：`if id < 1000 || id > 9999 { reason = "id_out_of_item_range" }`
- **实测 ddragon 16.17.1 `item.json`：868 个物品里 440 个 id > 9999**。
  `126697` = **「狂妄」(Hubris)**。斗魂物品更是清一色 `223020/224645/447108` 这种 6 位 id。
- 日志里这条 13 次全是同一个 id，**说明每次解析都在丢这件装备**，行内 asset 被剔除后
  `hexdata.go:2114` 的 `len(ids)>0` 还会让整行消失——这也是"核心装少"的一个来源。
- **同文件 `hexdata.go:1931-1948` 紧接着已经用 ddragon 真目录 `validItemIDs[asset.ID]`
  做了一次正确校验**，所以 2101 那个范围检查既多余又有害。

**改法**：直接删掉 `:2101-2102` 的范围检查，依赖已有的 `validItemIDs` 目录校验。
补一条测试：喂一个 id > 9999 的合法物品（如 126697），断言它不被丢弃。

### A-4（P1）海斗胜率 0%：上游真没有，但我们"没有也要画个 0%"

- 上游确认没有：整份 mayhem RSC 里 `"数字%"` 匹配数 = 0，`boots_0` 那行的统计单元格
  在 payload 里直接是字面量 `false`（op.gg 这个页面就不渲染胜率列）。
- 我们的解析 `hexdata.go:2121-2131 metricRowsFromIDs` 只造 `championMetricRow{Assets:...}`，
  PickRate/WinRate/Games 全是 Go 零值。
- **真正的缺陷在序列化**：`gameplayRecommendationStats`（`gameplay.go:3101-3105`）三个字段
  都是指针、显然是为"无数据=null"设计的，但 `recommendationStats`（`gameplay.go:3681-3683`）
  **无条件 `&pickRate`** → JSON 永远是 `{"pickRate":0,"winRate":0,"games":0}`
  → 前端 `rate(0)` 返回 `"0%"`（`web/gameplay.js:3850-3855`）。
- **英雄详情页早就做对了**：`web/champions.js:1787-1792` / `1813-1816` 都是
  `if (!hasPick && !hasWin) return ""`。对局页 `web/gameplay.js:3776-3780/3784-3788`
  无条件渲染，只有 `renderDepthStats:3794-3795` 有这个守卫。

**改法二选一**：①让 `recommendationStats` 在 `games==0 && winRate==0 && pickRate==0` 时返回 nil 指针；
②前端照 `renderDepthStats` 的写法加守卫。**建议做①**，因为指针字段本来就是为这个设计的，
而且能同时修好所有消费方。

### A-5（P1）回答用户关于 resg.top 的问题

- **`resg.top` 和 `hexdata.com.cn` 不是一回事，Go 代码里 `resg.top` 零引用**
  （只在 `design/` 文档和早期笔记里出现）。当前接的国服站是 **hexdata.com.cn**。
- **hexdata 现在用量很小且没有越界**：`hexdataClient.allowedPath`（`hexdata.go:528-530`）
  是硬白名单，只有 `/api/hexdata/answer-cards`（robots 明确 Allow 的那个）、`/heroes`、
  `/augments`、`/augment-rarity`、`/hero/{id}-{slug}`、`/augment/{id}-{slug}`，
  **没有任何 `/data/` 或其他 `/api/` 路径**，UA 也是诚实的。这条合规红线守得住，别动。
- **hexdata 唯一能补 op.gg 出装缺口的字段是「装备排行」**——带真实 winRate/games/HexScore
  的单件装备榜，正好治 A-4 那个"啥统计都没有"的观感。
  **而且后端已经拿到了**（`hexdata.go:1829` `response.ItemRanking`），
  只是 `gameplayRecommendationBundle`（`gameplay.go:3024-3046`）没有这个字段所以对局页看不到。

**改法（性价比最高的一条）**：给 bundle 加 `ItemRanking` 透传 + 对局页海斗区块渲染一列。
**零新增上游请求、零 robots 风险**。

- **不能补的**：出装顺序/出门装/鞋子/技能加点/召唤师技能这五项在 hexdata 的 SEO 兜底 HTML 里
  命中数为 0，只在 `Disallow` 的 `/api/`、`/data/` 里，且对方 loader 有 UA 嗅探。
  要拿必须伪装 UA，性质从"读错路径"升级成"绕过技术措施"，而且我们是它自家 HexBox 的直接竞品。
  **这条线维持"不做"，别试。**

---

## B 组（P1）：对局页出装页重新布局

### B-1 页签顺序
斗魂和海斗的页签改成 **[海克斯与出装] [详情]**，即把合并后的那个页签放到"详情"**前面**
（`recommendationTabSpecs`，`web/gameplay.js:3272-3277`；
默认页签逻辑 `recommendationActiveTab:3280` 已经会优先落到 build，注意别冲突）。

### B-2 页内两个标题 + 分隔
页面里放两个标题：**「海克斯推荐」** 和 **「装备推荐」**，中间要有视觉分隔。
标题样式请设计一下（现在是裸 `<h3>`），建议参考页内已有的区块标题体系，不要另造一套。

### B-3 海斗的区块排布
- **出门装 + 召唤师技能 + 鞋子 放一行**（三列并排）
- **技能加点单独一行**
- 然后是核心装

### B-4 斗魂的区块排布
斗魂没有出门装和召唤师技能（A-1 已确认上游真没有），所以：
- **鞋子 + 技能加点 放一行**
- 然后是 **棱彩装备**，再然后是 **核心装备**
- **装备卡片样式参考斗魂英雄详情页**（`web/champions.js` 的 `renderBuildBoard:1623-1639`
  和 `renderRankedBuild:1642-1651`），用户明确说现在对局页的"太丑了"

---

## C 组（P1）：战绩卡斗魂两行错位 + 左边框过长（图三）

**根因（R43-B1 留下的）**：`web/gameplay.css:640`
```css
.match-stats { align-content: start; grid-template-rows: repeat(3,18px); gap: 5px;
               min-height: 54px; padding-left: 18px; border-left: 1px solid var(--line); }
```
`min-height:54px` = 3×18 + 2×5，是**按三行写死的**。斗魂只有两行、内容高 41px，
但盒子仍被撑到 54px：
1. 用户说的"分割线"就是这条 `border-left`，它随盒高 54px 绘制 →
   在"承伤"下面多出约 13px 空线，**完全对应"应该到承伤这一行就结束"**。
2. `.match-main` 是 `align-items:center`、行高由 `min-height:63px` 决定，
   54px 盒子在 63px 行里居中 + `align-content:start` → 两行内容整体偏上，与 KDA/头像中线不齐。

**还有个前提**：`web/gameplay.js:2012` 的容器是裸 `<div class="match-stats">`，
**没有任何模式修饰类**，所以 CSS 现在根本区分不了两行/三行。

**改法**：
1. `web/gameplay.js:2012` 给容器加模式类（如 `match-stats is-arena`）。
2. `web/gameplay.css:640` 加 `.match-stats.is-arena { grid-template-rows: repeat(2,18px);
   min-height: 41px; align-content: center; }`（或者干脆让行数自适应）。
3. **窄屏分支 `web/gameplay.css:758` 也写死了 `min-height:54px`，要跟着改。**
4. **测试同步**：`web/champions.test.cjs:3419-3428` 硬钉了
   `repeat(3,18px)` 和 `align-content: start`，会挡住这次改动，必须改成能区分两种模式的断言。

---

## D 组（P1）：斗魂详情列间距与装备换行（图四）

**列定义写在两处、必须成对改**：
- `web/gameplay.css:812` `.arena-detail-columns`（表头）
- `web/gameplay.css:824` `.arena-detail-player`（数据行）
- 两处都是 `grid-template-columns: minmax(140px,1.1fr) 104px 96px 100px 116px 130px; gap: 8px;`

**为什么间距看起来不均**（不是 gap 的问题，gap 都是 8px）：
- 第一列是 `1.1fr` 弹性列，玩家名左对齐（`.participant-link` `min-width:130px`、
  名字 `max-width:118px`），列被拉宽后右侧全是空白 → **名字与海克斯之间空一大截**。
- 海克斯列 104px、内容 `repeat(3,30px)+2×3px=96px` 居中只剩 4px 余量；
  评分列 96px、`.match-score-cell` 宽 86px 居中剩 5px →
  **海克斯↔评分实际只有 8+4+5≈17px，显得挤**。

**改法**：收窄第一列（如 `minmax(140px,0.9fr)`）、给海克斯↔评分之间增加列间距
（可用不等的 `column-gap` 或调整各列定宽），目标是各列视觉间隙一致。

**装备不换行**：`web/gameplay.css:834` `.arena-detail-items` 是 `flex-wrap: wrap`，
图标 26px，列宽 130px 每行只能放 4 个（4×26+3×3=113），6~7 件必然换两行。
改 `nowrap` 后列宽要 ≥ 6 件 171px / 7 件 200px（或改小图标）。
**注意**：总列宽变了，窄屏兜底 `web/gameplay.css:838-845` 的 `min-width:780px` 要同步抬高。

**测试同步**：`web/champions.test.cjs:1198` 正则硬钉了那串列宽、`:1211-1212` 钉了 780px。

---

## E 组（P1）：客户端装备方案重做（图六/七/八）

### E-1 分组合并
现在 `buildItemSetPayload()`（`web/gameplay.js:3698-3718`）：
- 核心装 `:3708` 每套路线一个分组 → `核心装 1`…`核心装 5`
- 后续装备 `:3709-3712` 把 fourth/fifth/sixth 拍平后**逐件**建组 → `后续装备 1`…`后续装备 15`
  （上限 20 由 `:3717` 截断）
- 出门装 `:3705` 每个 option 一个分组

**改成**：核心装全部套路**去重后合并成一栏**、后续装备**去重后合并成一栏**、
出门装也**合并成一栏**，一共三四个分组。
`:3704` 已有的 `items()` 去重工具现在只在单个分组内用 `new Set`，跨分组不去重，扩一下即可。

**顺序**：核心装在前、后续装备在后（现有顺序 出门装→鞋子→核心装→后续装备→棱彩 已经满足，
只是分组太碎）。

> ⚠️ **服务端约束**（`gameplay.go:4696-4720` `validateGameplayItemSetRequest`）：
> 分组数 1~20、每组 `Type` 非空且 ≤64 rune、**每组 items 1~20 且组内不得重复**。
> 合并后核心装很可能超过 20 件 → **必须在前端截断，否则整个请求 400**。

### E-2 方案名称简写
名称是两处叠加出来的：
- 后端 `gameplay.go:4813-4824` `itemSetName()` 无条件加前缀 `"Deep Legends · "`
- 前端 `web/gameplay.js:3715-3717` 拼 `${championName}${lane} 数据推荐`

最终 = `Deep Legends · 兰博 · 上路 数据推荐`，游戏内下拉框显示不全。
**改成 `DL · 兰博 · 上路`**（到"上路"结束），**必须同时改这两处**
（Go 的前缀 + JS 的 ` 数据推荐` 后缀）。
**测试同步**：`gameplay_test.go:3091` 硬断言了 `"Deep Legends · 李青 · 打野 OPGG 推荐"`，
`web/champions.test.cjs:2098/3043/3100` 也要跟。

### E-3 斗魂/海斗不显示"应用装备方案"按钮
按钮在 `web/gameplay.js:3678-3680`，`canApply` 只看 `blocks.length>0 && phase==="ChampSelect"`，
**没有模式判断**。
**用同一函数里已有的 `capabilities.hasAugments` 加闸门即可**
（`:3673-3674` 已经在用它），**不要写 queueId 硬编码**。
渲染和提交同源（`applyItemSet()` `:3946-3961` 会再次调用 `buildItemSetPayload`），改一处即可。

---

## F 组（P1）：核心装与第四/五/六件图标不对齐（图五）

**真根因（只发生在对局页，英雄详情页不受影响）**：
`web/gameplay.css:1336-1337`
```css
.config-item { display: inline-grid; justify-items: center; gap: 3px; }
.item-option-name { max-width: 70px; white-space: nowrap; text-overflow: ellipsis; }
```
`.config-item` 的列宽 = max(图标 42px, 名称实际宽度, 上限 70px)，
而 `justify-items:center` 把 42px 图标**居中**在这个会变宽的格子里。
于是**名称越长，图标相对格子左沿越往右偏，最大偏移 (70-42)/2 = 14px**。
核心装那列是多图标路线、第四五六件是单图标，名称长度不同 → 各列图标左沿互不对齐。
垂直方向同理：`.config-item` 高度 = 42 + 3 + 名称行高，名称换行差异会让图标在 60px 行里漂移。

英雄详情页因为 `renderAssetButton` **没有名称标签**，图标恒 42px，所以不受影响
——这也解释了为什么用户只在对局页看到偏移。

**改法**：`.config-item` 改 `justify-items: start`，或给图标格固定 42px 轨道、
让名称独立成行不参与撑宽。共享文件 `web/build-item-row.css` 不用改（那套"行对行"契约本身是成立的）。

---

## G 组（P2）：英雄页面状态持久化

用户要求：英雄页面里**切换的模式、搜索框内容、切换的页签**，离开页面再回来要保持原样，
而不是每次都重置。

请查 `web/champions.js` 的 state 初始化与页面切换逻辑，把这几项做持久化。
项目已有 `preference()`/`savePreference()` 机制（`web/app.js:1974-1983` 在用），
**优先复用这套，不要新造一套存储**。注意区分"应该持久化"（模式/搜索/页签）和
"不应该持久化"（滚动位置之外的临时加载态）。

---

## H 组（P2）：绝活哥不要阻塞 OPGG 符文展示

真机实测：`specialist_runes_done {duration_ms: 18630, runes_returned: 7, players_parsed: 3,
budget_used: 42}`——R43 的并发改造生效了（3 个玩家、7 条符文都出来了），但 18.6 秒仍然偏慢。

用户要求：**OPGG 的符文数据到了就先展示，不要等绝活哥**。
请确认现在符文页是不是在等所有来源都就绪才渲染，改成 OPGG 先渲染、绝活哥页签自己显示加载态
（这个加载态机制 `specialistRuneFlightActive` 已经有了，`web/gameplay.js:3460` 附近）。

---

## I 组（P2）：对局模式右边加"当前位置"标签（图九）

在对局页顶部模式条（"召唤师峡谷 自选 自定义"那一行）的右边，加一个
**「当前位置：上路」**样式的标签，让用户一眼看到当前用的是哪个位置的数据。
- 样式要**醒目一点，做成标签/胶囊那种**，不要做成普通文字。
- **只有召唤师峡谷才显示**（大乱斗/海斗/斗魂没有位置概念）——
  用后端下发的能力位判断（可以复用 `hasCounters` 那套机制，或者按位置候选列表是否非空），
  **不要写 queueId 硬编码**。

---

## 附：I-2（对局名单丢人）本轮已自证解决，不用再改

R43 加的两个埋点这次都拿到了配对数据：
```
live_roster_shape    {phase:"InProgress", players:10, team_counts:{"100":5,"200":5}, raw_count:10}
live_roster_rendered {phase:"InProgress", players_received:10, rendered_100:5, rendered_200:5}
```
后端给 10 人、前端渲染 5+5，**逐条配对全部一致，没有任何一次丢人**。
之前那次"对方只有 4 人"确认是加载中的瞬时快照，不是真 bug。这条可以关闭了。

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **本轮要同步改的现有测试比较多**，已逐条标出：C 组的 `champions.test.cjs:3419-3428`、
  D 组的 `:1198`/`:1211-1212`、E-2 的 `gameplay_test.go:3091` 与 `champions.test.cjs:2098/3043/3100`。
  改测试时**换成能反映新行为的等价断言，不要直接删**。
- **变异测试**：A-1（按数据有无而非能力位判断）、A-2（核心装上限）、A-3（物品 id 校验）、
  A-4（零值返回 nil）、E-1（分组合并去重）、E-3（斗魂/海斗无按钮）六处必须做。
- **J 组只改探测、不写解析**，拿到斗魂真机数据前继续保持上一轮的纪律。
- B/C/D/E/F 组用 Playwright 在 1440/900/420 三个宽度截图作为验收证据。
