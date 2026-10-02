# WORKLIST-R191：R190 复核问题——符文变量缺失当成 0、海克斯说明整页重绘、hexdata 回退变窄、布局偏差

诊断人：Claude（读 R190 账本与截图 + 只读审 diff + 重跑测试）。执行人：GPT。日期：2026-10-02。
基线：R190 工作区（0.12.54，未提交）。先把 R190 提交，再在其上做本工单；完成后版本升到 0.12.55。

## 复核结论

R190 的主体实现与工单一致，测试也能复现通过：

- Node：`r190.test.cjs` 9/9；`gameplay`、`champions`、`r88`、`r117-style`、`r116d`、`r158`、`r185` 共 352 项全部通过。
- Go：`go vet ./backend` 通过；`-run 'R190|TestR100Perks|TestR175|R160|Perk|Augment'` 通过。全量 1675 项里，复核环境有 8 项失败，原因都是复核时只拷了 `backend/`，没有 `desktop/`、`docs/` 和真实 PNG（桌面壳、账号核对文档、PNG 色彩格式这几类）。和代码无关，不需要处理。

但审代码时发现下面 5 个问题，P1、P2 会直接影响国服用户看到的结果。

| 编号 | 问题 | 影响 |
|---|---|---|
| P1 | 变量缺失和变量为 0 分不出来 | 国服主要走 SGP，如果 SGP 返回里没有 var 字段，所有符文会显示「0」 |
| P2 | 致命节奏 / 神奇之鞋的变量含义仍未核实，而且没有办法从用户日志里拿到样本 | 用户截图里那场的基石不显示任何数值，也一直没法补 |
| P3 | 海克斯说明加载完后调用 `rerenderCatalogViews()`，整个总览和所有页签重绘 | 每次打开一个新的海斗构建页，所有战绩卡片重建一遍，可能闪图标、卡顿 |
| P4 | `loadHexdataAugmentDetail` 空 slug 时，hexdata 目录失败就直接报错，不再走原来的 CommunityDragon 回退 | 英雄页海克斯图鉴的详情在 hexdata 故障时比 R190 之前更容易失败 |
| P5 | 布局和修订后的工单 / 设计图有 4 处偏差 | 视觉问题 |

---

## P1　变量「没有」和「是 0」要分开

### 现状

- `riot_api.go` `riotPerkSelections.Selections` 的 `Var1/Var2/Var3` 是 `int64`；`gameplay.go` `lcuParticipant.Stats` 的 `PerkNVarM` 也是 `int64`。JSON 里没有这个字段时值是 0，和真实的 0 一样。
- `convertRiotMatchInfo`（Riot 和国服 SGP 共用）和 `lcuPerkStats` 只要 `PerkID > 0` 就输出 `perkStats`。
- 前端 `perkEffectLines` 拿到 `[0,0,0]` 会正常显示「0」。
- 国服战绩主要来自 SGP `/match-history-query/v1/products/lol/player/{puuid}/SUMMARY`（`sgp_api.go` 905–910 行）。仓库里**没有任何一份真实 SGP 或 LCU 返回带 `var1` / `perk0Var1` 的样本**（grep `backend/testdata`、`docs`、`tools` 只命中 R190 工单本身），R190 的测试全是手写夹具。所以无法确认 SGP SUMMARY 是否带 var。如果不带，国服用户打开构建页会看到一列「0」。

### 修改

1. 解析时记录字段是否存在：
   - `riotPerkSelections.Selections` 的三个 var 改成 `*int64`（或保留 `int64` 并在 `UnmarshalJSON` 里记一个 `hasVars bool`），三个都缺失视为「没有变量」；
   - LCU 同样：`perkNVar1..3` 三个都缺失视为没有。用 `*int64` 或者解析 `stats` 时额外读一次 `map[string]json.RawMessage` 判断键是否存在，二选一，选改动小的。
2. 输出规则：**一个参与者的 6 个符文里，只要有一个符文缺变量，这个参与者就不输出 `perkStats`**（宁可整组不显示数值，也不要部分显示真值、部分显示假 0）。缺失的参与者在前端走「没有数值」分支，显示固定效果灰字，头部不显示汇总。
3. `riotMatch` 磁盘缓存序列化后指针字段仍能区分缺失（`omitempty` 不要加在 var 上，nil 序列化成 `null` 即可），v2 条目读回后结论不变。
4. 诊断：每次战绩加载结束（列表或单场详情）记一条 `perk_stats_presence`：`source`（`sgp` / `riot` / `lcu`）、`participants`、`with_vars`、`without_vars`。每个 source 每次会话最多记 3 条，不记 ID 和名字。用户发日志就能看出国服 SGP 到底带不带 var。

### 测试

1.（Go）SGP / Riot 夹具 `selections` 不带 var 字段 → 该参与者没有 `perkStats`；带 `var1:0,var2:0,var3:0` → 有 `perkStats`，值为 0。
2.（Go）LCU 夹具不带 `perkNVarM` → 没有 `perkStats`；带全 0 → 有。
3.（Go）6 个符文里 1 个缺变量 → 整个参与者不输出 `perkStats`。
4.（Go）v2 缓存往返后第 1 条结论不变。
5.（Go）`perk_stats_presence` 计数正确，每个 source 会话内最多 3 条。

变异：把指针改回 `int64` 并忽略缺失 → 测试 1 FAIL。

---

## P2　致命节奏 / 神奇之鞋：从用户日志里拿真实样本

### 现状

R190 账本写明：执行环境是 macOS，没有 Windows 客户端，也没有截图那场的 gameId，所以 `PERK_EFFECT_OVERRIDES` 里 8008、8304 都是空数组，这两个符文不显示数值，改显示固定效果。这个降级本身是对的（不猜）。问题是现在**没有任何途径拿到样本**：Windows 待验项要求「导出原始 JSON」，但用户没有导出工具，日志里也没有记录。用户截图的那场基石正好就是致命节奏，所以 R190 对用户最关心的这一行没有效果。

### 修改

1. 后端在转换战绩时（Riot / SGP / LCU 三条路径），遇到**当前主体**（`subjectParticipantId` 对应的那个人）带了 8008 或 8304，就记一条诊断 `perk_effect_sample`：`perk_id`、`vars`（三个原值）、`source`、`queue_id`、`game_duration`（秒）。每个 `perk_id` 每次会话最多 3 条。不记名字和 gameId。
2. 这条诊断要进现有的导出诊断包（和其他 `perk_*` 事件走同一个通道），用户点导出日志就能带出来。
3. 不改 `PERK_EFFECT_OVERRIDES` 的内容：仍然是空数组，直到拿到真实样本再开工单启用。

### 测试

6.（Go）主体是 8008 → 记一条 `perk_effect_sample`，含三个 var；非主体玩家用 8008 → 不记。
7.（Go）同一会话第 4 场 8008 → 不再记。
8.（Go）导出诊断包里包含该事件。

### 用户需要做的（GPT 在账本和 CHANGELOG 里写清楚）

安装 0.12.55 → 打开一场自己带致命节奏的对局的「构建」页（让战绩加载一次即可）→ 导出日志发回。拿到样本后再开工单写修正表。

---

## P3　海克斯说明到达后只重绘相关对局

### 现状

`gameplay.js` `ensureAugmentDescriptions`（R190 新增）在请求结束后调用 `rerenderCatalogViews()`。这个函数会把所有页签的 `matchViewRevision` 加一，然后重绘总览、对局页、浮层和所有外部战绩视图（gameplay.js 约 4045 行）。它原本只用于符文 / 装备目录到达这种「全局图标都会变」的情况。海克斯说明只影响正在看的那一两场的构建页。这个项目在 R158、R185 都处理过战绩卡片重复重建导致的图标重载和卡顿。

### 修改

1. `ensureAugmentDescriptions(ids)` 结束后，不调 `rerenderCatalogViews()`。改为：找出当前所有展开且停在「构建」页签、`augmentIds` 和这批 ID 有交集的对局，逐个 `rerenderMatch(tab, gameId)`（覆盖普通页签、浮层和外部视图，复用 `ensureBuildData` 已有的定位方式）。
2. 战绩卡片和详尽表格里海克斯图标的 tooltip（R190 第 7 条）：不为它触发重绘。tooltip 在**显示时**读 `state.augmentDescriptions`，有就带上说明。如果现有 tooltip 机制只能读 `data-tooltip` 静态属性，就在下次正常重绘时自然带上，不主动刷新。
3. 不改 `matchViewRevision`。

### 测试

9.（Node）两个页签各有 10 场战绩，其中一场展开在构建页：说明到达后，只有这一场的 detail 节点被替换，其余 19 张卡片的 DOM 节点引用不变，`matchViewRevision` 不变。
10.（Node）同一批 ID 在两个页签都展开：两处都更新。

变异：恢复 `rerenderCatalogViews()` → 测试 9 FAIL。

---

## P4　hexdata 目录失败时恢复原来的 CommunityDragon 回退

### 现状

R190 在 `hexdata.go` `loadHexdataAugmentDetail` 里加了：空 slug 且 `id >= 1000` 时先读 hexdata `/augments` 目录找 slug，目录读失败、校验失败或没找到这个 ID，都**直接返回错误**。R190 之前，空 slug 会走下面的 `slug == ""` 分支，用 CommunityDragon 目录返回一个 `Source: "CommunityDragon fallback"` 的结果。

`handleChampionAugmentDetail`（champions.go 962 行）在 slug 不规范（OP.GG 键）时也会传空 slug，所以英雄页海克斯图鉴也受影响。hexdata 在 R146、R147 都出过 403 / 令牌问题，这条回退是有用的。

### 修改

1. hexdata 目录读取失败、校验失败、或目录里没有这个 ID 时，**不返回错误**，记一条现有的 `reportHexdataFallback("augment-directory", err)`，然后继续走原来的 `slug == ""` CommunityDragon 回退分支。
2. 构建页说明接口（`gameplayAugmentDescription`）拿到的是回退结果时，`effectDescription` 为空 → 仍按 `unavailable` 处理（回退结果里的目录说明对海斗 ID 是占位句，R190 已经正确地不用它）。

### 测试

11.（Go）hexdata 目录返回错误、CommunityDragon 正常 → `handleChampionAugmentDetail` 返回 200，`source = "CommunityDragon fallback"`（与 R190 之前一致）。
12.（Go）同样条件下，`augment-descriptions` 对该 ID 返回 `unavailable`，不是 500。
13.（Go）hexdata 目录正常 → 行为与 R190 相同（取到 slug、读详情正文）。

变异：目录失败时直接 return err → 测试 11 FAIL。

---

## P5　布局偏差

R190 账本里写「工单 P2-9 写 900px / 左栏 330–420px」，说明执行时拿到的是**修订前**的工单副本；3:1 是按设计图做的，但修订版工单里同时改了几个尺寸，没跟上。对照 `docs/history/reports/r190/ranked-wide.png`、`ranked-narrow.png` 和修订版工单 / 设计图：

1. **符文树没有垂直居中。** 修订版第 9 条写了「树在左栏内垂直居中」。截图里树贴顶，下面空出一大块（效果列表比树高）。改：≥ 1100px 时 `.rune-split-tree { display: grid; align-content: center; }`。
2. **效果列表尺寸用的是旧值。** 修订版：基石图标 38px、普通 28px、多个数值间距 12px、行左右内边距 6px。现在是 44px / 32px / 16px。右栏只有约 290px，按修订版改。
3. **属性碎片 chip 没有图标。** 设计图每个 chip 左边有一个 22px 碎片图标。用 `renderRuneOption` 里取碎片图标的同一逻辑（`dataDragonRuneShardPath`），22px，chip 左内边距 4px。
4. **窄布局下碎片也竖排了。** 修订版写的是「宽布局下竖排」。< 1100px 时效果列表占满宽度，碎片改回一行横排（`display: flex; flex-wrap: wrap; gap: 8px`，标题「属性碎片」在同一行最左）。

### 测试

14.（Node / CSS 断言）≥ 1100px：`.rune-split-tree` 有 `align-content: center`；`.eff.is-keystone .game-icon` 为 38px，普通 28px，`.stats` gap 12px。
15.（Node）碎片 chip 内各有一个图标元素。
16.（CSS 断言）碎片竖排规则只在 `@container (min-width: 1100px)` 内。
17.（Chromium）重新截 `ranked-wide` / `ranked-narrow` 两张，放进 `docs/history/reports/r191/`，和 `docs/r190-design/r190-design.png` 对照：树居中、碎片有图标、窄屏碎片横排。

R117 样式预算如需再豁免新数值，只豁免本工单列出的值，写进账本。

---

## 执行步骤（GPT）

1. 提交 R190；新建 `docs/history/ledgers/r191-execution-ledger.md`。
2. 按 P1 → P5 实现；跑本工单 17 项测试和 4 个变异，再跑 Node 与 Go 全量。
3. 版本 0.12.55；完整 public 构建（产物带 `-public` 后缀，账本写 key mode）。
4. CHANGELOG 写一行面向用户的说明：「如需显示致命节奏的数值，请打开一场带致命节奏的对局构建页后导出日志」。
5. `docs/WORKLIST-INDEX.md` 加 R191 一行。
6. Windows 待验（交给用户）：
   - 国服战绩打开构建页，符文数值不是一列 0（若全部只显示固定效果，导出日志看 `perk_stats_presence`）；
   - 带致命节奏的对局导出日志，确认有 `perk_effect_sample`；
   - 打开海斗构建页时，其他战绩卡片不闪图标。
