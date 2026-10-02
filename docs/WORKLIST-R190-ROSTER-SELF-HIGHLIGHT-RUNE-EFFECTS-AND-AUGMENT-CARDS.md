# WORKLIST-R190：对局卡片名单高亮当前玩家；构建页加符文效果和海克斯说明

诊断人：Claude（看截图 + 只读核对源码 + 核对 CommunityDragon 符文目录）。执行人：GPT。日期：2026-10-02。
基线：源码 0.12.53。工作区里 R188、R189 的改动**还没提交**，先把它们提交，再在其上做本工单。完成后版本号升到 0.12.54。

设计稿：`docs/r190-design/r190-design.png`（源文件 `r190-design.html`，azure 主题，就是用户截图里用的主题）。设计稿里的符文数字、海克斯名称和说明都是示例，不是真实数据。

三个问题，都是用户提的界面需求：

| 编号 | 内容 |
|---|---|
| P1 | 总览战绩卡片右侧名单里，当前玩家的名字用主题色加粗 |
| P2 | 展开详情 → 构建 → 符文：保留现在的符文树，旁边加「符文效果」，显示每个符文这局的实际数值（和 LeagueAkari 的符文页一样的数据） |
| P3 | 构建 → 海克斯（海克斯大乱斗、斗魂竞技场）：图标网格改成卡片，显示名称、品质、选取顺序和效果说明 |

界面红线照旧（CLAUDE.md「界面文案红线」）：不加口径、来源、免责说明。拿不到的数据就不显示那一项，不写「--」「暂无」「数据来自……」。

---

## P1　对局卡片名单：当前玩家主题色加粗

### 现状

`backend/web/gameplay.js` `renderMatchPlayers`（约 3115 行）生成卡片右侧两列名单，每个人都是 `.match-player-name`，颜色统一 `--muted`（`gameplay.css` 721 行）。看不出哪个是自己。

展开详情里的「详尽表格」已经在做同样的事：`isCurrentMatchParticipant(item, match)`（约 3618 行，按 `match.subjectParticipantId` 判断）+ `.match-detail .participant-name.is-current-player { color: var(--primary-strong); font-weight: 750; }`（gameplay.css 893 行）。

### 修改

1. `renderMatchPlayers` 的 `playerButton`：`isCurrentMatchParticipant(item, match)` 为真时，`.match-player-name` 加 `is-current-player`。普通两列名单和斗魂竞技场四队预览名单都要加。
2. `gameplay.css` 在 721–724 行附近加：
   ```css
   .match-players .match-player-name.is-current-player { color: var(--primary-strong); font-weight: 700; }
   ```
   用 700 不用 750：这里字号只有 10.5px，750 在 Windows 的 Microsoft YaHei UI 下会糊。
3. 「当前玩家」是这个页签对应的召唤师（`subjectParticipantId`），不是登录账号本人。看别人的战绩时高亮的是那个人。
4. 悬停颜色、省略号、`data-tooltip`、隐藏名字打码都不变。按钮 hover 只改按钮的 `color`，高亮的 span 自带颜色，悬停后仍是主题色，这是预期效果。
5. 所有主题都只用 `--primary-strong`，不写死颜色。

### 测试（Node，新建 `backend/web/r190.test.cjs`）

1. 10 人对局，`subjectParticipantId=4`：只有第 4 个人的 `.match-player-name` 带 `is-current-player`，其余 9 个没有。
2. 斗魂竞技场 16 人：主体玩家所在行带 `is-current-player`，且只有一个。
3. `subjectParticipantId` 缺失或为 0：一个都不带。
4. 开启隐藏名字：仍然带 class，文字是打码后的名字。

变异：去掉 `isCurrentMatchParticipant` 判断改为全部加 → 测试 1 FAIL。

---

## P2　构建 → 符文：保留符文树，加「符文效果」

### 数据来源（已核对）

每个符文这局的数值，Riot 对局数据里本来就有，只是我们没解析：

| 来源 | 字段 | 现状 |
|---|---|---|
| Riot API match-v5、SGP（两者都用 `riotMatchInfo`） | `perks.styles[].selections[].var1/var2/var3` | `riot_api.go` 611 行 `riotPerkSelections.Selections` 只解析了 `perk` |
| LCU `/lol-match-history/v1/games/{id}` | `stats.perk0Var1` … `stats.perk5Var3`（18 个） | `gameplay.go` 约 626 行只解析了 `perk0`…`perk5` |

数值的含义靠符文目录里的模板：LCU `/lol-game-data/assets/v1/perks.json` 每个符文有 `endOfGameStatDescs`，例如：

```
9111 凯旋      ["回复生命值总和：@eogvar1@","提供的额外金币总和：@eogvar2@"]
9103 传说：血统 ["已完成的时间：@eogvar1@:@eogvar2@"]
8017 砍倒      ["额外伤害总和：@eogvar1@"]
8339 星界躯体  ["--"]
```

现在 `gameplayPerk`（gameplay.go 9595 行）没有保存这个字段。客户端没连接时走 Data Dragon 回退，Data Dragon 的 runesReforged.json **没有**这个字段；CommunityDragon 的 `/latest/plugins/rcp-be-lol-game-data/global/zh_cn/v1/perks.json` 有，内容和客户端一致。

**Riot 自己的模板有错，不能直接套用。** 已确认的两个：

```
8008 致命节奏（zh_cn）  ["最大攻速运转时间：@eogvar1@:@eogvar2@<br>已造成的伤害：@eogvar2@"]
8008 致命节奏（en_US）  ["Max Attack Speed Uptime: @eogvar1@:@eogvar2@<br>Damage Dealt: @eogvar2@"]
8304 神奇之鞋（en_US）  ["Boots Arrival Time: @eogvar1@:@eogvar2@@eogvar3@"]
```

致命节奏两行都用了 var2。LeagueAkari 照模板直接替换，所以用户截图二里出现了「最大攻速运转时间：1028:1028」这种错误显示。我们不能照搬。

### 后端修改

1. **解析变量。**
   - `riotPerkSelections.Selections` 加 `Var1`、`Var2`、`Var3`（`json:"var1"` 等）。
   - LCU 原始结构加 `Perk0Var1` … `Perk5Var3`。
   - `gameplayParticipant` 加 `PerkStats []gameplayPerkStat \`json:"perkStats,omitempty"\``，`gameplayPerkStat{PerkID int64 \`json:"perkId"\`; Vars [3]int64 \`json:"vars"\`}`。只放 6 个主副系符文，不放属性碎片（碎片没有变量）。Riot/SGP 和 LCU 两条路径都要填。
   - 变量全是 0 也要保留这一项（0 是有效数值，比如这局砍倒没触发）。

2. **Riot 对局磁盘缓存要升版本。** `riot_api.go` 908 行的缓存键是 `riot-match-v1|`，存的是按 `riotMatch` 结构体重新序列化的 JSON（`riot_match_cache.go` 51 行），有效期 365 天。旧条目里没有 var 字段，永远补不回来。
   - 新写入用 `riot-match-v2|`。
   - 读取先找 v2；v2 没有、v1 有时，照常用 v1（战绩列表、详情都正常），但这场对局的参与者没有 `perkStats`，并在对局上标 `perkStatsStale: true`。
   - 前端打开这场对局的「构建」页签、发现 `perkStatsStale` 时，发**一次**单场刷新请求（新参数，例如 `/api/gameplay/match?…&refresh=perk-stats`，按现有单场详情接口的形式加），后端重新拉这一场并写 v2。同一场一次会话内只发一次，失败不重试，界面照常显示不带数值的符文效果（见下文第 6 条）。不要批量重拉旧对局，KR 开发 key 有限流。
   - SGP 战绩如果也有类似的结构体序列化缓存，同样处理；执行时 grep `riotMatchInfo` 的持久化位置确认，在账本里写明结论。
   - LCU 路径每次实时读取，如果有本地缓存同样检查。

3. **符文目录加模板。**
   - `gameplayPerk` 加 `EndOfGameStatDescs []string \`json:"eogDescs,omitempty"\``，从 LCU perks.json 的 `endOfGameStatDescs` 读。
   - 客户端未连接的回退：Data Dragon 目录之外再取一次 CommunityDragon zh_cn perks.json，只用它补 `eogDescs`（按 ID 合并），复用现有 CommunityDragon 退避和超时规则；失败就没有模板，不影响符文树。
   - 归一化目录的缓存键 `normalized-perks-v1|`（gameplay.go 9769 行）升到 `v2`。

### 前端：模板解析（纯函数，放 gameplay.js，Node 可测）

4. 新函数 `perkEffectLines(perk, vars)`，返回 `[{ label, value, kind }]`：
   - 先按数组元素拆，再按 `<br>`（大小写不敏感）拆成行；
   - 每行按第一个全角或半角冒号分成「标签」和「值模板」；
   - 值模板是 `@eogvarN@` → 整数，千分位；
   - 值模板是 `@eogvar1@:@eogvar2@` → 时间 `m:ss`，秒补两位；
   - 模板是 `--` 或空 → 返回空数组；
   - 遇到认不出的值模板（例如 `@eogvar1@:@eogvar2@@eogvar3@`）→ 丢弃这一行，不要输出原始占位符；
   - **单独的修正表** `PERK_EFFECT_OVERRIDES`，按符文 ID 覆盖 Riot 写错的模板。8008 致命节奏、8304 神奇之鞋必须在表里；表里每一项的变量对应关系必须用真实对局核对过（见执行第 2 步），没核对清楚的那一行先不显示，不要猜。
   - 标签只做两条简化：去掉末尾「总和」「的总和」「总计」；去掉开头「提供的」。其余原样。设计稿里的短标签只是示意，以这两条规则的实际结果为准。
   - `kind`：标签含「伤害」→ `damage`；含「治疗」「回复」「护盾」→ `heal`；含「金币」→ `gold`；时间和次数 → `other`。

5. **头部汇总**（`符文` 标题右侧的 chip，见设计稿）：
   - 伤害、治疗、金币三类，各自把所有符文加起来；
   - **同一个符文有多行伤害时只取最大的一行**（例如强攻同时有「伤害总和」和「额外伤害」，后者是前者的一部分，相加会重复）；治疗、金币同理；
   - 合计为 0 的类别不显示；三类都没有就不显示这组 chip；
   - 颜色：伤害 `#F0A27A`（新增变量 `--yield-damage`，各主题共用）、治疗 `--success`、金币 `--rarity-gold`。

### 前端：布局

6. `renderBuild`（约 3887 行）的符文分支改为：
   ```
   <div class="rune-split">
     <div class="rune-split-tree">${renderUnifiedRuneBoard(subject)}</div>
     ${renderRuneEffects(subject)}
   </div>
   ```
   **`renderUnifiedRuneBoard` 不改结构**（英雄页、选人推荐、职业选手页都在用），只允许给 `.rune-option-button` 加一个 `data-perk-id` 属性，用于联动高亮。

7. `renderRuneEffects(subject)` 按符文树的槽位顺序排（不按 `perkIds` 顺序）：
   - 第一行：基石，单独一块（44px 图标、`--primary-soft` 渐变底、`--primary` 30% 描边），名字后加小标签「基石」，名字下一行灰字写所属系（「精密」等）；数值 18px、`--primary-strong`；
   - 主系另外 3 个符文，每行：32px 图标 / 名字 / 右侧数值列；
   - 分隔行：副系图标 + 副系名，后面一条细线；
   - 副系 2 个符文；
   - 最后一行「属性碎片」：三个小 chip，内容用现有 `runeShardDescription(id)`（例如「攻击速度 +10%」）。
   - 数值列：每个数值一组，数字在上（15px、700、`font-variant-numeric: tabular-nums`）、标签在下（10.5px、`--muted`），多个数值横排间距 16px。
   - 没有数值的符文（模板是 `--`，或这场没有 `perkStats`）：右侧不放东西，名字下面一行灰字写这个符文的 `shortDesc`（去 HTML 标签、单行省略）。**不显示「--」。**
   - 符文目录还没加载完：沿用现有「正在读取完整符文树…」，效果列表不渲染。

8. **联动高亮**：鼠标移到右侧某一行，左侧树上对应的已选图标加 `is-linked`（`--primary-strong` 2px 描边 + 6px 主题色 22% 光晕）；移到树上的已选图标，右侧对应行加 `is-hover`（`--primary-soft` 70% 底色）。用事件委托只切 class，不重新渲染。未选的灰色图标不联动。键盘 focus 同样触发。

9. **宽度**：`.build-detail` 设为 `container-type: inline-size`，宽度 ≥ 900px 时左右两栏 `minmax(330px, 420px) minmax(0, 1fr)`，中间一条 `--line` 竖线；< 900px 时上下排，树在上，中间横线。树自己的断点（1616、1657、1745 行那几档）保留不动。

10. 只改「展开详情 → 构建」这一处。选人阶段的符文推荐、英雄页、职业选手页的符文板都不变。

### 测试（Node，`r190.test.cjs`）

5. `perkEffectLines`：凯旋 `vars=[804,300,0]` → `[{回复生命值,804,heal},{额外金币,300,gold}]`（按第 4 条的两条简化规则得出的实际标签）。
6. 血统 `vars=[19,5,0]` → 值 `19:05`。
7. 模板 `["--"]` → 空数组；渲染后这一行没有「--」字样，显示 `shortDesc`。
8. 认不出的模板 → 这一行被丢弃，输出里不含 `@eogvar`。
9. 8008 走修正表，输出里不出现 `A:A` 这种两个相同数字用冒号连起来的结果（用 `vars=[3,1028,0]` 之类的输入断言）。
10. 汇总：强攻 `["伤害总和：@eogvar1@","额外伤害：@eogvar2@"]`、`vars=[2000,500,0]` 加砍倒 `[300]` → 伤害合计 2300，不是 2800。
11. 没有 `perkStats` 的对局：效果列表 6 行全部是灰字说明，头部没有汇总 chip。
12. `renderUnifiedRuneBoard` 对英雄页的调用输出，除了新增 `data-perk-id` 外与改动前逐字相同（快照比较）。
13. 联动：派发 `pointerover` 到效果行 → 树上对应图标有 `is-linked`，其他没有；`pointerout` 后清除。

Go 测试：
14. LCU 原始 JSON 带 `perk1Var1=804, perk1Var2=300` → `perkStats[1] = {9111, [804,300,0]}`。
15. match-v5 JSON 带 `selections[].var1..3` → 同上。
16. 缓存：只有 v1 条目时返回 `perkStatsStale=true`，没有 `perkStats`；刷新后写入 v2，再读不再 stale。
17. LCU perks.json 带 `endOfGameStatDescs` → 输出 `eogDescs`；CommunityDragon 补模板按 ID 合并，不覆盖已有的名字和图标。

变异：Riot 路径漏读 var → 测试 15 FAIL；汇总改成相加 → 测试 10 FAIL；去掉修正表 → 测试 9 FAIL；`renderUnifiedRuneBoard` 结构被改 → 测试 12 FAIL。

---

## P3　构建 → 海克斯：卡片显示名称、品质、顺序和说明

### 现状与数据来源（已核对）

- `renderBuild` 有海克斯时只渲染 `.build-augment-grid`，一排 62px 图标，名字只在 tooltip 里。
- `/api/gameplay/augments`（gameplay.go 9747 行）返回的是内置快照 `data/augment_catalog_20260924.json`：554 条，**全部没有 description**。所以战绩里的海克斯 tooltip 现在只有名字。
- 能拿到说明的地方：
  - **斗魂竞技场（ID < 1000）**：CommunityDragon `/latest/cdragon/arena/zh_cn.json`，`loadCommunityDragonAugments`（champions_structured.go 1906 行）已经用 `renderArenaAugmentDescription` 把 `dataValues` 代进说明；
  - **海克斯大乱斗（ID ≥ 1000）**：Riot 不提供说明（hexdata.go 3344 行注释已核对过），唯一中文来源是 hexdata 海克斯详情页正文，`loadHexdataAugmentDetail(ctx, id, "")`（hexdata.go 3432 行），英雄页的海克斯图鉴已经在用。
- **海克斯没有「收益」数值**：对局数据里只有 `playerAugment1..6` 六个 ID，没有符文那样的 var。所以海克斯只展示效果，不做汇总。

### 后端修改

1. 新接口 `GET /api/gameplay/augment-descriptions?ids=1,2,3`（最多 6 个 ID，非法 ID 返回 400）：
   - 返回 `{ items: [{ id, description, status }] }`，`status` 为 `ok` / `unavailable`；
   - ID < 1000：从 CommunityDragon arena 目录取（复用 `loadCommunityDragonAugments` 及其缓存和退避）；
   - ID ≥ 1000：调 `loadHexdataAugmentDetail(ctx, id, "")`，复用它已有的缓存；
   - 每个 ID 单独超时（沿用 hexdata 详情现有的超时），并发不超过 2；
   - 说明只能用海克斯本身的效果文案。不能用页面 `<meta name="description">` 的胜率摘要，也不能用 `champions.js` 1463 行那句「海克斯图鉴中可读取说明」占位，这两种都按 `unavailable` 返回。
   - 诊断事件 `augment_descriptions_loaded`：请求个数、ok 个数、unavailable 个数、耗时，不记说明正文。

### 前端修改

2. 只在「构建」页签展开、这场对局有海克斯时才请求；结果存 `state.augmentDescriptions`（Map，按 ID），会话内复用，同一个 ID 不重复请求。请求失败就当 `unavailable`。
3. `renderBuild` 的海克斯分支改成 `.aug-grid`（`repeat(auto-fill, minmax(250px, 1fr))`，间距 8px），每个海克斯一张卡（见设计稿 P3）：
   - 左：56px 图标，圆角 12px，品质色描边；左上角 20px 圆形数字是选取顺序（`playerAugment1` 为 1，依此类推）；
   - 右上：名称（13.5px、700，过长省略）+ 品质标签（白银 / 黄金 / 棱彩，复用 `normalizeAugmentRarity`）；
   - 右下：说明，12px，最多 3 行，超出省略号，完整说明放进这张卡的 `data-tooltip`；
   - 卡片底色：品质色 9% 叠在 `--surface` 上的斜向渐变；描边：品质色 28% 混 `--line`；棱彩图标用 `--rarity-prismatic-a/b/c` 环形渐变。
4. 三种状态：
   - 有说明 → 正常显示；
   - 加载中 → 两条骨架条（92%、64% 宽），不写「正在读取」；
   - `unavailable` → 只显示名称和品质，卡片高度收紧、图标垂直居中，**不写「暂无说明」**。
5. 说明是纯文本。设计稿里数字高亮只在上游文本自带高亮标记时才做，不要用正则自己找数字加粗。
6. 海克斯大乱斗和斗魂竞技场都用这套卡片。宽度够时海斗通常一行 4 张，斗魂 6 张自动换行。
7. 顺手：战绩卡片和详尽表格里的海克斯图标 tooltip，如果 `state.augmentDescriptions` 已经有这个 ID 的说明，也带上说明；没有就保持只有名字，不为此额外请求。

### 测试

18.（Go）ID 1225 走 hexdata 详情，ID 120 走 arena 目录；hexdata 返回空说明 → `unavailable`；返回占位句 → `unavailable`。
19.（Go）7 个 ID → 400；并发上限 2（用计数桩验证）。
20.（Node）4 个海克斯：顺序徽标 1–4 与 `augmentIds` 顺序一致；品质 class 正确。
21.（Node）说明加载中 → 有 `.skel`，无文字；`unavailable` → 卡片里没有「暂无」「说明」字样。
22.（Node）同一 ID 第二次打开构建页不再发请求。

变异：说明来源改成 meta description → 测试 18 FAIL；去掉 ID 缓存 → 测试 22 FAIL。

---

## 执行步骤（GPT）

1. 提交工作区里的 R188、R189；新建 `docs/r190-execution-ledger.md`。
2. **核对符文变量的真实含义（P2 修正表的依据）**：在用户 Windows 机器上，对用户截图里那场灵活排位（1 小时前、7/9/8、致命节奏 + 凯旋 + 传说：血统 + 砍倒 / 启迪 返现 + 星界洞悉）调用 `/lol-match-history/v1/games/{gameId}`，把本人的 `perk0..5` 和 `perk0Var1..perk5Var3` 原样记进账本；再找一场带神奇之鞋的对局同样记录。结合 en_US 模板和客户端结算页（如有）确定 8008、8304 每个变量的含义，再写修正表。查不清的行在修正表里标为不显示，账本写明原因。
3. 按 P1 → P2 → P3 的顺序实现；全部 Node 与 Go 测试、上面列出的变异都要跑，结果记账本。
4. 用 Chromium 打开一场普通排位、一场海克斯大乱斗、一场斗魂竞技场，分别在宽（≥ 1280）和窄（约 820）两档截图，和 `docs/r190-design/r190-design.png` 对照，差异写账本。
5. 版本号 0.12.54；按 CLAUDE.md 做 public 构建，产物带 `-public` 后缀，账本写 key mode。
6. `docs/WORKLIST-INDEX.md` 加 R190 一行。
7. Windows 真机待验项（交给用户）：名单高亮在各主题下可读；构建页符文数值与 LeagueAkari 一致（致命节奏除外，以第 2 步核对结果为准）；海克斯大乱斗卡片能显示说明。
