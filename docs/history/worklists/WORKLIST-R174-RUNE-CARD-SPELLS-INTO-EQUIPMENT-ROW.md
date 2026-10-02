# WORKLIST-R174：符文卡片里召唤师技能并入装备行右侧，不再单独占一行

基线：源码版本 0.12.38（R173 已执行）。这是纯前端展示位置调整，不改后端、不改数据、不改应用逻辑。

## 背景

用户 9/30 19:40 提出：「把召唤师技能也放到装备那行的右侧，不要单独一行」。这条要求本来写进了 R173 的 P1-3，但 GPT 执行的是改之前的 R173 版本，所以 0.12.38 里技能仍然是单独一行。R173 的其余部分（出门装、竖线、诊断、启动缓存）已经落地并通过验证，**本工单不重做，也不回退**。

## 现状（已核对源码）

- `backend/web/gameplay.js:6245`：绝活哥 / 职业选手的对局行模板里，`${renderRuneSpellPair([config.spell1Id, config.spell2Id])}` 紧跟在 `${renderUnifiedRuneBoard(config)}` 后面、`${items}` 前面，输出 `<div class="rune-spell-row"><span>召唤师技能</span>…</div>`，是独立一行。
- `renderRuneEquipment(config)`（约 6250）：没有终局装备（`itemIDs` 为空）时直接返回 `""`；有出门装时输出 `<div class="specialist-game-items"><span>装备</span><div>出门装 + route-divider + 终局装备</div></div>`。
- `patchRuneStarterEquipment`（约 5182）：出门装异步到达后，用 `renderRuneEquipment(config)` 生成新节点，整体替换该行里的 `.specialist-game-items`。
- `renderRuneSourceSection`（约 6183）：OPGG 来源的技能是卡片级的一块（`.rune-source-section > .rune-spell-row`，`recommendedRuneSpellIDs(state.live, {sourceLabel:"OPGG"})`），没有装备行。
- `renderRuneChoice`（约 6300）：`rune-choice-detail` 里同样输出 `renderRuneSpellPair`，这条路径没有装备行。
- 样式：`gameplay.css:1234-1238` `.specialist-game-items`；`gameplay.css:1167-1174` `.rune-spell-row / .rune-spell-items / .rune-spell-item`。

## P1　技能并入 `.specialist-game-items`，位于最右侧

范围只限**绝活哥 / 职业选手**的对局行（即第 6245 行的模板）。OPGG 卡片级技能块、`renderRuneChoice` 里的技能块都没有装备行，**保持现状，不动**。

### P1-1　渲染

1. 对局行模板里去掉单独的 `renderRuneSpellPair([config.spell1Id, config.spell2Id])`，让 `renderRuneEquipment(config)` 自己负责把技能渲染进装备行。`renderRuneEquipment` 已经拿到整个 `config`，直接读 `config.spell1Id / config.spell2Id`，用现有的合法性校验（两个都是 1–100000 的整数且不相等，否则视为没有技能）。可以把校验和图标渲染抽成一个小函数供 `renderRuneSpellPair` 和装备行共用，但 `renderRuneSpellPair` 的现有输出（OPGG 卡片级、`renderRuneChoice`）必须逐字不变。
2. 装备行结构：`<div class="specialist-game-items"><span>标签</span><div>装备图标组</div><div class="specialist-game-spells">技能图标 ×2</div></div>`。技能容器是行的最后一个子节点。
3. 技能容器里**只放两个技能图标**（复用现有 `renderSummonerSpellIcon(id)`），不带「召唤师技能」标签文字，也不带技能名小字；不新增任何说明文字或 tooltip（沿用图标本身已有的属性，不额外加）。
4. 三种组合：
   - 有终局装备、有技能：标签 + 装备组（含出门装 / 竖线，逻辑不变）+ 右侧技能。
   - 有终局装备、无技能（职业选手来源目前拿不到技能）：与现在完全一致，右侧不留占位、不留空节点。
   - **无终局装备、有技能**：现在 `renderRuneEquipment` 会返回 `""`，技能就没地方放了。改成仍输出一行 `.specialist-game-items`，里面只有右对齐的技能容器，**不输出标签 `<span>` 和空的装备组**，避免出现孤立的「最终装备」标签。
   - 无终局装备、无技能：仍返回 `""`（与现在一致）。
5. 出门装异步补充：`patchRuneStarterEquipment` 用 `renderRuneEquipment(config)` 整体替换 `.specialist-game-items`，`config` 是同一份符文记录（含 `spell1Id/spell2Id`），所以补充后技能会跟着新节点一起渲染，位置不变。执行时确认补充前后技能图标始终在，不闪、不消失、不重复；`if (!config?.starterItemIds?.length) continue;` 的现有条件不需要改。

### P1-2　样式

在 `gameplay.css` 加：

```css
.specialist-game-spells { display: flex; flex: 0 0 auto; align-items: center; gap: 6px; margin-left: auto; }
.specialist-game-spells .item-option-button { width: 30px; height: 30px; padding: 2px; }
.specialist-game-spells .game-icon { width: 24px; height: 24px; flex-basis: 24px; }
```

（具体数值以 `.specialist-game-items .item-option-button / .game-icon` 现有大小为准，和装备图标一致即可。）要求：

- 技能容器不被压缩（`flex:0 0 auto`），装备组 `> div` 已有 `min-width:0; flex-wrap:wrap`，窄宽度下**装备先换行**，技能始终留在行右端，不掉到单独一行。
- 只有技能没有装备时（无标签、无装备组），技能仍靠右（`margin-left:auto` 生效），行高沿用 `.specialist-game-items` 的 `min-height:44px`，不额外增高。
- `.specialist-game-items > span` / `> div` 这两条子选择器只作用于标签和装备组，不要误伤技能容器（技能容器用 `div.specialist-game-spells`，若 `> div` 规则会命中它，就把装备组改成带 class 的选择器，或者在技能容器上显式覆盖 `flex-wrap:nowrap; min-width:auto`）。
- 深色 / 浅色主题都不需要新增颜色变量。

### P1-3　不动的东西

- R171 的「同时应用召唤师技能」勾选、`recommendedRuneSpellIDs`、应用逻辑、`spell_requested / spell_applied` 诊断、后端全部不动。
- R173 的出门装提取、`route-divider`、标签「装备 / 最终装备」的切换规则不动（有出门装 → 「装备」，没有 → 「最终装备」）。
- OPGG 卡片级技能行、`renderRuneChoice` 里的技能行、R172 build 卡片核心装路线不动。
- 界面红线：不加统计口径 / 方法说明 / 免责声明 / 任何解释性文字或 tooltip。

## 测试

改动的现有断言（这些断言当前依赖「绝活哥 / 职业行里存在独立的 `rune-spell-row`」，必须同步更新，不能靠放宽正则蒙混）：

- `backend/web/gameplay.test.cjs` 约 520、523 行：「pro record without verified spell IDs must hide the entire row」和「rune-spell-row … 闪现 … 惩戒」——改成针对新结构：绝活哥 / 职业行里技能位于 `.specialist-game-items` 内、`.specialist-game-spells` 中；没有合法技能时整个 `.specialist-game-spells` 不存在。
- `champions.test.cjs`、`gameplay.test.cjs` 中 compile 列表里已有 `renderRuneEquipment / renderRuneSpellPair`，如新增了共用小函数，把它加进 compile 名单。
- 约 976–994 行 `renderRuneEquipment` 的现有测试：无技能字段时输出必须与现在**逐字一致**（这是「有装备无技能」不变的证据）。`{ itemIds: [], starterItemIds: [1055] }` 仍返回 `""`。

新增 Node 测试：

1. 有装备 + 有技能：DOM 顺序为 标签 → 装备组 → `.specialist-game-spells`，技能容器是 `.specialist-game-items` 的最后一个子节点，里面恰好 2 个技能图标，没有「召唤师技能」文字。
2. 有出门装 + 有技能：标签「装备」→ 出门装 → `route-divider` → 终局装备 → 技能，顺序严格断言。
3. 有装备、无技能（`spell1Id` 缺失 / 为 0 / 两个相同）：输出与改动前逐字一致，没有 `specialist-game-spells`。
4. 无终局装备、有技能：输出一个 `.specialist-game-items`，里面只有 `.specialist-game-spells`，没有标签 `<span>`，没有空装备组。
5. 整行模板（第 6245 行那条路径）里不再出现 `.specialist-game-items` 之外的 `rune-spell-row`；OPGG 卡片级技能行仍恰好 1 个（现有约 586 行的断言继续通过）。
6. 异步补出门装：先渲染只有终局装备 + 技能的行，再 `patchRuneStarterEquipment`，补充后技能图标仍在且仍是最后一个子节点，对局行顺序与 `is-selected` 状态不变。

变异（各自必须得到断言 FAIL，且不是编译错误；用 overlay，不改仓库源码）：

| 变异 | 期望 |
|---|---|
| 把技能容器渲染到装备组前面 | 测试 1 / 2 FAIL |
| 把独立 `renderRuneSpellPair` 放回对局行模板 | 测试 5 FAIL |
| `renderRuneEquipment` 在无终局装备时仍返回 `""` | 测试 4 FAIL |
| 装备行里给技能加「召唤师技能」标签 | 测试 1 FAIL |
| 补出门装的替换路径丢掉技能 | 测试 6 FAIL |

验证：`node --test backend/web/*.test.cjs`、`go test ./backend`（后端未改，跑一遍确认无回归）、`go vet ./backend`、`git diff --check` 全绿。

## 视觉验证（GPT 能做到的部分）

用现有的前端本地页面或夹具渲染绝活哥 / 职业行，在三种宽度（宽 / 中 / 窄）下截图，确认：技能始终在装备行右端，窄宽度只是装备换行；有技能无装备时技能靠右、行高不变。不能真机截图就在账本里写明「仅 DOM/CSS 断言，未视觉验证」，不要写成已验证。

## 收尾

- 版本号：按仓库惯例递增（0.12.39），构建验证同 R173。
- `docs/WORKLIST-INDEX.md` 加 R174 一行；R173 那行状态补一句「召唤师技能并入装备行见 R174」。账本 `docs/history/ledgers/r174-execution-ledger.md`，写清哪些是断言、哪些是变异、哪些没验证。

## 真机验收（用户，Windows）

进一局选人，打开符文推荐的「绝活哥」：每条对局记录底部同一行，左边是（出门装 → 竖线 →）终局装备，最右边是这场的两个召唤师技能图标，不再有单独的「召唤师技能」行。打开「职业选手」：目前没有技能，行的样子应与 0.12.38 一致（等 `pro_details_shape` 日志确认字段后另开工单）。OPGG 来源顶部的「召唤师技能」行保持原样。缩窄窗口时装备先换行、技能仍在右侧。
