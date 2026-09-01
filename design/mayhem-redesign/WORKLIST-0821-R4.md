# 海克斯大乱斗 · 第四轮工单（数据 + 样式）

> 2026-08-21。来源：用户在 `http://127.0.0.1:8793/` 上的五张截图与问题清单。
> 方法：源码逐行定位 + **直接向上游复现契约**（hexdata.com.cn / CommunityDragon / OP.GG 三方均实测）。
>
> ⚠️ **先说明一个限制**：我的沙箱**打不通你本机的 `127.0.0.1:8793`**（试过 `127.0.0.1`、`host.docker.internal`、网关 `172.16.10.1/.2`，全部 `000`）。所以下面所有结论不是"看你的页面"得出的，而是**打上游 + 读源码**得出的。好处是证据比看日志更硬：我拿到的是上游的真实字节。

---

## 零、先回答那个问题：**hexdata 的数据拿得到，而且好好的**

「废了这么大劲居然拿不到？？」——**不是拿不到。上游一切正常，是本机把自己锁死了。**

2026-08-21 实测，五条白名单路径全部 200，解析契约逐项对得上：

| 路径 | 状态 | 关键校验 |
|---|---|---|
| `/api/hexdata/answer-cards` | 200 | `heroAliases` **172** 条 |
| `/heroes` | 200 | 兜底表解析出 **172** 行，与 172 相等 → `hexdata.go:706` 的数量闸门**能过** |
| `/augments` | 200 | 单表，结构未变 |
| `/augment-rarity` | 200 | 正常 |
| `/hero/67-vayne` | 200 | `data-primary-content` 出现 1 次；**两张表都在**——推荐海克斯 8 行 + **装备 8 行** |

四份文档的 `buildId` / `patch` / `reportDate` **完全一致**（`hexdata-2026-08-19-2a6a2393a083` / `16.16` / `2026-08-19`），所以 `hexdata.go:757` 那条严格跨文档相等断言**也能过**。`<meta name="hexdata-build-id">` 在每页都在。

**结论：上游零变化，解析代码没坏。数据没上来是本机状态问题。**

### 顺带回答：装备排行能不能拿到 —— **能**

`/hero/67-vayne` 的 SEO 兜底层里第二张表就是装备排行，8 行、带 HexScore/胜率/样本（只有中文名、无 id 无链接，图标要靠本地名称匹配，`decorateHexdataItems` 已经在做这件事）。**可以按你说的挪到最后展示。**

---

## 一、P0：熔断器 24 小时粘死，且全程静默

这是「整页全是 OP.GG」的**唯一总开关**。

**根因链（三段，缺一不可）：**

1. `hexdata.go:475-480` `recordShapeFailure` → `recordFailure(0, immediate=true)`
   → `hexdata.go:465-473`：`immediate` 为真时**一次**就写 `CircuitUntil = now + 24h`。
   全项目有 **9 处**调用 `recordShapeFailure`，任意一处抖一下就锁死一整天。

2. `recordSuccess`（`hexdata.go:455-462`）**只清 `Failures`，不清 `CircuitUntil`**。
   → 熔断一旦开启，**没有任何成功路径能把它关掉**，只能等 24 小时自然过期。
   `CircuitUntil` 落在 `hexdata-state.json`（`hexdata.go:204`），**重启 EXE、重新构建都清不掉**。

3. 熔断生效时（`hexdata.go:256-258`）直接 `errors.New("hexdata circuit is open")` 返回，**不发任何诊断事件**；
   而 `champions.go:1448-1451` `loadARAMRankings` 拿到 error 之后 **`if err == nil` 一句带过、把错误整个吞掉**，直接走 OP.GG。
   → 你在 UI 上看到的就是"没解释地全变成 OP.GG"。全项目 hexdata 相关的 diag 事件只有 3 个（`hexdata.go:478 / 544 / 990`），**没有一个覆盖熔断和取数失败**。

**你能立刻自查的两件事（不用改代码）：**

- 删掉 `%LOCALAPPDATA%\LOLLootAssistant\champion-data\hexdata-state.json`，重开。如果数据回来了，就 100% 坐实是这条。
- 翻 `%LOCALAPPDATA%\LOLLootAssistant\logs\diagnostics.jsonl` 找 `"event":"hexdata_shape_invalid"`，那条记录会告诉你当初是哪个 `kind` 把闸拉下来的。

**建议改法（P0）：**

- `recordSuccess` 必须 `h.state.CircuitUntil = time.Time{}`。这是最小且必须的一行。
- 「形状不符」不该等同于「上游封禁」。建议分级：**403/429 → 24h**（这是对方明确拒绝）；**形状不符 → 累计 3 次才熔断，且时长降到 30 分钟**。上游改版是可恢复的，封禁才不可恢复。
- 熔断分支和降级分支各补一条 diag：`{"event":"hexdata_circuit_open","until":...}`、`{"event":"hexdata_fallback","module":"rankings","reason":...}`。
- UI 上要能看出来。`renderSourceCitation` 已经会渲染「数据来源 OP.GG」，但榜单页顶部那句 `web/champions.js:728` 的 **`海克斯大乱斗 · 艾欧尼亚 Queue 2400` 是写死的字符串**——降级到 OP.GG 时它还在那儿挂着，等于在替 OP.GG 数据冒充 hexdata 口径。**这行必须跟着实际来源走。**

---

## 二、P0：海克斯名字显示成「海克斯 2010」

**不是拿不到名字，是有名字没往上写。**

- 兜底名的产地：`hexdata.go:1312`，`parseMayhemRSC` 里
  `championMetricRow{Assets: []championAsset{{ID: id, Kind: "augment", Name: "海克斯 " + strconv.Itoa(id)}}}`
  ——只有 ID，无名、无图、无品质、无描述。

- 本该补上的地方：`hexdata.go:994-1012` `decorateHexdataAugments`。它写了 `asset.Description`、`rows[i].Rarity`、`asset.Source/Path`，**唯独没写 `asset.Name`**。

- 而目录里名字是**有**的。实测 CommunityDragon `cherry-augments.json`：

  ```json
  {"id": 2010, "augmentNameId": "ARAM_DoubleTap", "nameTRA": "双发快射",
   "augmentSmallIconPath": ".../UX/Kiwi/Augments/Icons/DoubleTap_small.png", "rarity": "kGold"}
  {"id": 1356, ... "nameTRA": "暴击飞弹", ... "rarity": "kGold"}
  ```

  两个 ID 都在 kiwi 命名空间，`gameplayAugmentIndexForMode(catalog,"mayhem")` **查得到**（`ok == true`）。

**改法：`decorateHexdataAugments` 里补一行 `if meta.Name != "" { asset.Name = meta.Name }`。** 一行解决截图里所有「海克斯 NNNN」。

> 对照组：斗魂那条链路（`champions_structured.go:876-880`）是**先查目录取名、查不到才退回占位名**，写法是对的。海斗这条漏了。

---

## 三、P0：图鉴点一下就「详情读取失败 / invalid augment」

**根因是 slug 格式对不上，且只在降级到 OP.GG 时发作。**

- 后端闸门 `champions.go:580-589`：`slug` 必须匹配 `^[a-z0-9-]+$`，否则 400 `invalid augment`。
  这个正则是照 hexdata canonical URL（`/augment/{id}-{slug}`）设计的，本身没错。

- 但图鉴降级时 `Key` 来自 OP.GG：`champions.go:1854` `Key: item.Key`。
  实测 OP.GG `aram-mayhem` 页里的真实值是：

  ```
  "name":"魔法飞弹","key":"ARAM_MagicMissile","rarity":4
  ```

  **`ARAM_MagicMissile` 有大写有下划线，必然被 `^[a-z0-9-]+$` 拒绝 → 每一次点击都是 400。**

- 顺带一个佐证：该页 `largeIcon` 出现 **229** 次，和你截图里图鉴的条数完全吻合。hexdata `/augments` 是 208 条量级。**229 = 你现在的图鉴确实在吃 OP.GG。**

**改法（两层都要）：**

1. 前端在降级来源下不该发起 hexdata 详情请求（`web/champions.js:489`）。判据用 `state.augments.source`，不是 `item.key` 猜。
2. 后端：`slug` 为空或不合法时，**不要 400**，而是用 `id` 走已有的名称/目录兜底返回一个可渲染的详情；400 只留给 `id` 本身非法。现在的行为是"上游降级 → 用户每点必错"，体验上是最糟的组合。

**另外：图鉴要默认选中一个。** `web/champions.js:35` `mayhemAugmentID: 0`，`:763` `items.find(...) || null`，全程没有任何自动选中——列表渲染完右侧永远是空的，用户必须先点一次。**建议在列表首次渲染完成后自动选中第一行。**

---

## 四、P1：斗魂海克斯说明把变量露出来了（截图五）

**上游就是模板串，我们从头到尾没做替换。**

实测 CommunityDragon `/latest/cdragon/arena/zh_cn.json`：**225 条海克斯里 183 条含占位符**。原文长这样：

```
id=362 与【猎人】结盟 → "…提供<gold>+%i:goldCoins%@BonusGold@额外金币</gold>。"
id=340 获得一个棱彩阶属性锻造器 → "获得一个<keywordMajor>%i:StatAnvil%棱彩阶属性锻造器</keywordMajor>。"
id=93  热身动作 → "…每秒使你的伤害提升2%，至多至@MaxStacks@%。"
```

而 `cleanMarkup`（`champions.go` 内）只做三件事：换 `<br>`、`markupPattern = <[^>]+>` 去标签、反转义实体。**全项目 grep `%i:` 和 `@Var@` 的处理逻辑，命中数为 0。**

**好消息：替换所需的数据就在同一份 JSON 里。** 每条海克斯都带 `dataValues`，例如 `WarmupRoutine` 有 `{"MaxStacks":[20,20,40,60,...]}`。

**改法：在 `loadCommunityDragonAugments` 的 arena 补充分支（`champions_structured.go:774-787`）加一层占位符渲染**，放在 `cleanMarkup` 之前：

- `@Name@` → `dataValues["Name"]` 第一个非零档位；数值按整数/一位小数格式化；查不到就**整段删掉**，不要留 `@` 。
- `%i:xxx%` 是行内图标标记，**一律剥掉**（现阶段不必换成图标）。
- 兜底顺序：`desc` 渲染后为空再退 `tooltip`。

**收尾要求：渲染后的字符串里不允许再出现 `@` 包裹的标识符或 `%i:`。** 建议直接加一条测试断言：全量 225 条渲染后 `/(%i:|@[A-Za-z][A-Za-z0-9_]*@)/` 一个都不匹配——这是能防复发的护栏，比逐条断言强。

---

## 五、P1：海斗和斗魂英雄详情里的海克斯没有颜色

**两条链路两个不同的原因，都要改。**

**海斗（`aram-mayhem`）**：`web/champions.css:342-350`，品质色只挂在 `.mayhem-recommend-column.is-X` 上——也就是**只有列头那个小色块和列内图标边框有色**，条目本身 `.mayhem-recommend-item`（`:345`）没有任何品质样式。你截图里那些落进「品质待确认」桶（`web/champions.js:1192` `mayhem-unclassified`）的条目，因为不在任何一列里，**连图标边框的颜色都拿不到**。
→ 既然第六节要把分列取消，**品质色必须挪到条目自身**（参照 `.arena-option-card` 的左侧 4px 竖条 + 边框混色，`web/champions.css:752-756`，那套是好的）。

**斗魂（`arena`）**：`web/champions.js:906`
```js
const groups = detail.arenaAugmentGroups?.length ? detail.arenaAugmentGroups : [{ rarity: 0, rows: detail.arenaAugments || [] }];
```
一旦 `arenaAugmentGroups` 为空走了那个兜底，`rarity: 0` → `arenaRarityKey(0)` 返回 `"unknown"`（映射表只认 1/4/8）→ `renderArenaOptionCard` 的 `rarity` 变量为空串 → **全部卡片都是灰色默认态**。
→ 兜底路径必须逐行从 `row.rarity` 取品质，或者由后端保证 `arenaAugmentGroups` 一定有值。另外 `arenaRarityKey` 只认 `{1,4,8}` 旧尺度，`{0,1,2}` 新尺度进来会全变 unknown，**这个映射表要把两套尺度都覆盖**（后端 `normalizeAugmentRarity` 已经是双尺度了，前端没跟上）。

**对局内那条也顺手记一笔**：`web/champions.js:1248` `renderArenaAugments` 的列表项完全没有品质 class。

---

## 六、P1：kiwi 海克斯根本没有描述文本（新发现，会影响改版效果）

实测 `cherry-augments.json` 的**字段并集只有 6 个**：`id / augmentNameId / nameTRA / simpleNameTRA / augmentSmallIconPath / rarity`。

**655 条里带 `description` 的：0 条。**

也就是说 `normalizeGameplayAugments`（`gameplay.go:3706-3736`）里的 `description := cleanMarkup(source.Description)` **对所有条目都返回空串**。描述只有 arena 那 225 条从 `/latest/cdragon/arena/zh_cn.json` 补进来——而 **kiwi 的 154 个 id 与 arena JSON 的 225 个 id 交集为 0**。

→ **海斗的海克斯目前没有、也不可能从现有数据源拿到说明文本。** 做样式改版之前得先定这件事，否则新版卡片上的描述位永远是空的。可选项：
- 找 CommunityDragon 里的 kiwi 专用描述文件（需要再探一次，我没验证过存在性）；
- 或者退而求其次，海斗卡片**不设描述位**，只放名字 + 品质 + 数据。

另外记一笔：`filterGameplayAugmentsForMode`（`champions_structured.go:795-808`）靠 **IconPath 里有没有 `/kiwi/`** 来判命名空间。655 条里有 **105 条路径中两个命名空间都没有**（都是 `/Strawberry/` 的斗魂棋类资产），会被两边同时丢弃。目前看不影响海斗（154 条 kiwi 都带正确路径），但这个判据是脆的——**上游哪天改了目录结构，整个模式的元数据会一起消失且没有任何报错**。值得加一条"kiwi 条数不得少于 120"的下限断言。

---

## 七、样式改版

### 7.1 模块顺序

现状 `web/champions.js:734`：
```
推荐海克斯 → 装备排行 → 出装顺序/出门装/鞋子 → 技能加点/召唤师技能 → 统计口径
```

改为：
```
推荐海克斯 → 【开局配置】(新合并区) → 装备路线 → 装备排行 → 统计口径
```

### 7.2 推荐海克斯：取消分品质列，改 10 个两行

- 删掉 `renderRecommendedAugments`（`web/champions.js:1174-1195`）的三列分组与「品质待确认」桶，**统一取前 10 条，`grid-template-columns: repeat(5, minmax(0,1fr))` 两行**。
- 品质色下沉到条目（见第五节）。
- **窄屏收敛为 `repeat(2,1fr)` 或 `repeat(3,1fr)`，不要让 5 列硬挤**——你说的"排版非常乱"，这里是重灾区之一（现在是三列 × 不定行 + 一个横向溢出的 unclassified 桶，`web/champions.css:357` 那条 `min-width: 250px; flex: 1 1 250px` 在窄屏必然撑破）。

### 7.3 前三名次序号：重新设计，海斗与斗魂共用

现状两边都很朴素：海斗 `web/champions.css:346` `.mayhem-recommend-item > b` 只是灰色小字；斗魂 `.arena-option-rank` 是 `#1` 文本。

建议做成**一个共用的名次徽章组件**（class 例如 `.hex-rank`，`.is-1/.is-2/.is-3` 三档特化）：

- 形状走**六边形**（`clip-path: polygon(...)`），呼应海克斯的几何语言，第 4 名及以后退化为普通圆角小方块。
- 第 1 名：金 `#E3B341` 描边 + 内发光；第 2 名：银 `#9AA7B8`；第 3 名：铜 `#C08457`。**与现有战绩卡的金银铜名次徽章保持同一套色**，别再造一套。
- 数字用 `font-variant-numeric: tabular-nums`，避免 1/2/3 宽度跳动。
- 不要用外部字体或图片资源，纯 CSS 画完——这是本地 EXE，多一个资源就多一份体积和加载失败面。

### 7.4 合并「出门装 / 鞋子 / 召唤师技能 / 技能加点」

- 合并 `renderMayhemBuildItems`（`:748`）与 `renderMayhemSkills`（`:757`）为一个 section。
- **区域标题不用四个名字拼接**。建议 **「开局配置」**，副标题「当前补丁的 OP.GG 海克斯大乱斗构筑」（沿用现有那句，来源要说清）。
- 内部四个子块各自一个 `<h4>`，**每个 h4 前面带图标**，样式抄斗魂的 `.arena-section-icon`（`web/champions.js:911` 那个 `<span class="arena-section-icon" aria-hidden="true">✦</span>` 的做法）。四个图标建议用纯字形、不引资源，且**四个必须互相可区分**（现在斗魂全站都是 `✦`，直接复用会四块长一样）。
- 「核心路线」从这个区域**挪出去**，单独成为 7.1 里的「装备路线」，排在开局配置之后。

### 7.5 装备排行移到最后

`renderMayhemItemRanking` 整块后移即可（数据可用性见第零节）。

### 7.6 斗魂那边

7.3 的名次徽章同步替换 `.arena-option-rank`。其余保持不动——斗魂的卡片布局是这次要参照的**基准**，别顺手改动它。

---

## 八、优先级汇总

| # | 项 | 级别 | 位置 |
|---|---|---|---|
| 1 | 熔断器不可复位 + 无诊断 + 静默降级 | **P0** | `hexdata.go:455-480,256-258`；`champions.go:1448-1451` |
| 2 | `decorateHexdataAugments` 不写 `Name` | **P0** | `hexdata.go:994-1012` |
| 3 | 图鉴 slug 闸门在降级下必然 400 | **P0** | `champions.go:580-589`；`champions.go:1854` |
| 4 | 图鉴无默认选中 | P1 | `web/champions.js:35,763` |
| 5 | 说明占位符 `%i:` / `@Var@` 未渲染 | P1 | `champions_structured.go:774-787` |
| 6 | 品质色（海斗条目 / 斗魂兜底 / 前端尺度） | P1 | `web/champions.css:342-350`；`web/champions.js:906` |
| 7 | 降级时顶部「艾欧尼亚 Queue 2400」写死 | P1 | `web/champions.js:728` |
| 8 | kiwi 无描述文本（需先定方案） | P1 | `gameplay.go:3706-3736` |
| 9 | 模块顺序 + 推荐海克斯两行十个 | P1 | `web/champions.js:734,1174-1195` |
| 10 | 名次徽章重设计（海斗/斗魂共用） | P1 | `champions.css:346`、`.arena-option-rank` |
| 11 | 合并四块为「开局配置」+ 图标 h4 | P1 | `web/champions.js:748,757` |
| 12 | 装备排行后移 | P2 | `web/champions.js:734` |
| 13 | `filterGameplayAugmentsForMode` 靠路径判命名空间，缺下限断言 | P2 | `champions_structured.go:795-808` |
| 14 | hero 形状埋点写死 16/16（跨轮遗留） | P2 | `hexdata.go:1066` |

---

## 附：红线复核

本工单不改变任何取数边界。实施时请保持：

- hexdata 只碰 `/api/hexdata/answer-cards` 一个白名单文件，不碰 `/api/` 其余路径与 `/data/`；
- 不打进 EXE、不做启动预热与定时同步；
- UA 保持诚实，不伪装；
- 上游字符串（海克斯名、描述、citation URL）一律经 `escapeHTML` 才进 innerHTML——**第四节的占位符渲染是新增的字符串处理链路，尤其要确认它不会绕过转义**；
- hexdata 请求链路不得出现 PUUID / summonerId / 召唤师名。
