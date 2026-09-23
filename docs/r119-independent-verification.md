# R119 独立验收：对局详情「补位」标签

**验收日期：** 2026-09-21　**验收对象：** `docs/r119-execution-ledger.md`（用户口头需求，无 WORKLIST 文件；版本 0.12.13）
**验收方法：** 隔离拷贝（未触碰工作树代码）；重跑 R119 全部 Go / JS 测试；对判定函数 `riotAutofillFlags`、字段管线、前端渲染做对抗变异；用真实 Chromium 渲染真实的 `matchTableRows` / `gameplay.css`；对判定口径的「事实前提」做外部核实。
**姊妹文档：** `docs/r116-independent-verification.md`、`docs/r118-independent-verification.md`、`docs/WORKLIST-R120-…`。

---

## 0. 结论

1. **代码实现是对的，测试是有效的，界面渲染正常**：R119 的 6 个 Go 测试、3 个 JS 测试全部通过（`-race -count=3` 无竞争）；Go 12 个有效变异打红 11 个，前端 9 个变异全部打红；真 Chromium 里浅色 / 深色主题、长名字截断、960 / 1200 宽度下标签都排版正常（截图见 `docs/r119-validation/`）。
2. **但功能的「判定口径」没有依据，这是最大的问题**：R119 把「`teamPosition` 有值而 `individualPosition` 缺失或 `Invalid`」判为**补位**，代码注释和界面提示都把 `individualPosition` 说成「他自己选到的那一路」。Riot 官方文档里这两个字段**都是游戏服务器根据对局中的实际表现推算的「最可能打的位置」**（`individualPosition` 是孤立地看这名玩家；`teamPosition` 是加上「每队各一个上单、打野……」约束后的推算），**不记录玩家在大厅里选了什么位置，也不记录是否被系统补位**。所以这套规则并不能检测补位。它标出来的更可能是「行为上认不出位置」的人（比如提前退出、挂机）或数据异常，而不是补位玩家。
3. **这违反项目红线「数据不准就降级、不用推断值冒充」。** 界面提示直接写「客户端没有给这名玩家分配个人位置，最终分路 X 由系统补位」，这是一句对某个玩家的事实陈述，目前没有证据支撑。
4. **建议：在真机数据证明口径成立之前，不要把这个标签当作可靠功能发布**（默认关闭，或至少把提示改成不下结论的说法）；真正的补位数据源需要另行探测（§5）。

> 我没能拿到一份真实的国服 / 韩服对局响应，也没有真机，所以「口径不成立」的结论来自官方字段定义，**不是**来自实测统计。它意味着「没有依据」，不等于「一定标错」——但在没有证据前，把它当事实展示是不合适的。

---

## 1. 复跑结果

| 项 | 结果 |
|---|---|
| `go build ./backend`、`go vet ./backend` | 通过 |
| `gofmt -l backend/*.go` | `backend/gameplay.go` 被列出（R119 新增字段 `Autofill` 未对齐，CI 第一步会红；已记入 R120 P1-1）|
| `go test -race -count=3 -run R119 ./backend` | ok，无 `DATA RACE` |
| R119 Go 测试（6 个）| 全部通过 |
| `node --test backend/web/r119.test.cjs` | 3/3 通过 |
| Node 全量 | 885 / 884 通过 / 0 失败 / 1 跳过（见 R116 文档）|

---

## 2. 对抗变异

### 2.1 后端（`go test -run R119`）

| # | 变异 | 结果 |
|---|---|---|
| R1 | 判定取反（`== ""` → `!= ""`）| 打红 |
| R2 | 「两值不一致」也算补位 | 打红 |
| R3 | 去掉「整场没有 individualPosition 就降级」护栏 | 打红 |
| R4 | 去掉「整场没有 teamPosition 就降级」护栏 | 打红 |
| R5 | 去掉队列门禁（任何队列都判）| 打红 |
| **R6** | **去掉「这名玩家 `teamPosition` 为空就跳过」**（`if riotLaneKeyValue(raw.TeamPosition) == "" { continue }`）| **存活**（R119 测试与更宽的相关子集都全过）|
| R7b | 管线里不赋值 `Autofill` | 打红 |
| R7c | 赋值取反 | 打红 |
| R8 | 赋值下标错位一位 | 打红 |
| R9 | 去掉 `omitempty`（会让既有响应多出 `"autofill":false`）| 打红 |
| R10 | 诊断计数只统计 420，漏 440 | 打红 |
| R11 | 把 `Invalid` 字面量当作「有值」| 打红 |

（首版 R7 因未使用变量编译失败，已换成 R7b / R7c，不计入。）

**R6 是真实的测试缺口，而且正好对应 Riot 已知的数据异常**：Riot 开发者关系仓库 issue #554 记录了 JP1 排位约 0.9% 的对局里出现「`individualPosition = INVALID` 且 `teamPosition` 为空」。这个形状下 R6 的护栏是唯一挡住「误标补位」的东西，但没有测试钉住它。

### 2.2 前端（`node --test backend/web/r119.test.cjs`）

9 个变异全部打红：真值判断放宽（`!player?.autofill`）、对所有人都出标签、标签放到按钮外面、标签放到名字前面、硬编码颜色、去掉 `flex: none`、去掉 `white-space: nowrap`、提示里丢掉最终分路、把「补位」改成「补」。

### 2.3 判定函数的边界行为（特征化探针，非断言）

`riotAutofillFlags` 的「整场护栏」只要求「**至少有 1 个人**带个人位置」就算有证据，结果：

| 输入（420，10 人）| 结果 |
|---|---|
| 只有 1 人有 `individualPosition`，其余 9 人缺失 | `evidence=true`，**打了 9 个补位标签** |
| 5 人缺失、5 人正常 | 打 5 个标签 |
| 1 人 `Invalid`、其余正常 | 打 1 个标签 |
| 1 人「`teamPosition` 空 + `individualPosition` Invalid」（issue #554 形状）| 0 个（R6 那条护栏在起作用）|
| 全部两值不一致 | 0 个 |

「证据」门槛太松：一场里大部分人缺字段时（响应残缺、半场数据），结果是大面积打标签。现实里同一局同时有多人补位并非不可能，但「10 个人里 9 个补位」几乎一定是数据问题而不是事实。

---

## 3. 真浏览器渲染

用云端 Chromium 加载**真实**的 `gameplay.css` / `app.css` 等样式与**真实**提取出的 `matchTableRows` / `matchTableShell` / `autofillChip`（图标与评分单元格用桩），渲染 5 行含长名字、短名字、正常行、补位行：

- 浅色、深色主题下标签配色都可读（走 `--warning` 令牌），没有溢出或换行；
- 名字过长时用省略号截断，标签仍完整显示在名字右侧；
- 960 与 1200 宽度都正常。

截图与渲染脚本：`docs/r119-validation/autofill-chip-light-1200.png`、`autofill-chip-dark-1200.png`、`render-harness.cjs`。**局限：** 图标 / 评分是桩，真实应用里的悬停提示（`data-tooltip` 由应用自己的 tooltip 组件呈现）我没有在完整应用里点开；桌面 Electron 真机也没测。

---

## 4. 口径核实：为什么说这条判定没有依据

**外部事实（可复核）：**

- Riot match-v5 `ParticipantDto` 文档：「`individualPosition` 与 `teamPosition` 都由游戏服务器计算，是同一玩家『最可能打的位置』的两个版本。`individualPosition` 是不考虑其它任何因素时，对该玩家实际打的位置的最佳猜测；`teamPosition` 是再加上『每队各有一个上单、一个打野、一个中单……』约束后的最佳猜测。一般建议使用 `teamPosition`。」（文档转载页：glama.ai 的 riot-docs match-v5）
- 两个字段都是**对局行为的推算**。文档没有任何一处说它们记录大厅选位或补位。
- 一份公开的真实对局数据集（`BoostedJonP/league_of_legends_match_data`，前 25 行）里，两个字段在正常情况下取值完全相同（例如 `UTILITY / UTILITY`）；同一行里 `lane` 与 `team_position` 也会不同（如 Gwen：`team_position=TOP`、`lane=JUNGLE`），说明这些位置字段反映的是打法。我只用它佐证「正常时二者一致、差异来自行为」，没有据此做统计。
- Riot 官方开发者关系仓库 issue #554：JP1 排位约 0.9% 对局 `individualPosition=INVALID` 且 `teamPosition` 为空，属于数据缺失问题。

**仓库内部的对照：**

- `docs/r119-execution-ledger.md` §2 与 `riot_api.go:960` 的注释把 `individualPosition` 写成「这名玩家被单独分配到的分路（他自己选到的那一路）」——**这是假设，不是事实**；台账 §2 自己也承认「仓库内没有真实抓包样本可佐证」，却把它写进了用户可见的提示文案。
- 「两值不一致 = 英雄选择阶段换过位置」（判定为不打标签，并以 `autofill_swapped` 计数）同样没有依据：不一致更可能是「按行为推算」与「加队伍约束后重新分配」的差别。
- 用户原话的规则是「最终位置不在他选择的两个位置中，并且没有选择任意位置」，需要**大厅里的两个位置偏好**。match-v5 里没有这两个值，R119 把它降级成「一个个人位置」时，语义已经变了，不再是同一条规则。

**这条判定会出现的两种结果**（在没有实测数据的前提下，二选一或混合）：

1. `individualPosition` 在真实数据里几乎永远有值 → 标签几乎永远不出现，功能形同虚设；
2. 在少数「行为上认不出位置」的玩家（提前退出、挂机、极端打法）或数据异常玩家身上出现 → 标签出现，但标的不是补位玩家，属于**误标**。

无论哪种，用户看到「补位」二字都会当作事实。

---

## 5. 建议

按优先级，本报告只给方向，代码由执行方按工单做：

1. **先别把标签当可靠功能发布**：默认关闭（功能开关），或把提示改为不下结论的表述；至少不要在提示里写「由系统补位」。
2. **用真机数据验证口径（成本最低）**：R119 已经埋了诊断计数 `autofill_tagged / autofill_swapped / autofill_no_evidence`。打若干局单双排 / 灵活组排，其中**有你明确知道谁补位了**的局，对照标签是否出在正确的人身上；同时看 `autofill_tagged` 的总体比例是不是明显偏离常识。验证不通过就下线该判定。
3. **真正的补位数据源要另找**：需要的是大厅 / 选人阶段的位置偏好与最终分配位置。这类数据只在 LCU 里、只在**实时**、并且据我所知主要覆盖**己方**，对手和事后回看历史局拿不到。项目里已有探测先例（`augment_contract_probe.go` 扫 LCU openapi 契约），建议用同样的方式先探测 LCU 里 `lobby` / `champ-select` 相关类型有没有偏好与补位字段，再决定要不要在对局进行时把「己方补位信息」记下来。若要落盘保存，会涉及**隐私声明**（`features.go` `stores`，需你拍板）。
4. 如果决定继续沿用现口径，至少补三件事：给 R6 那条 `teamPosition` 为空的护栏加测试；把「整场证据」门槛从「至少 1 人」改成「几乎全员都有 `individualPosition`」并加一个「一场里被标记比例过高就整场降级」的保护；把 `autofill_swapped` 之类带有未经证实含义的命名改成中性名字。
5. `gofmt`：`backend/gameplay.go` 对齐（R120 P1-1）。

---

## 6. 没有验证的东西

- 没有任何真实的国服（SGP）/ 韩服（match-v5）对局响应，不知道这两个字段在你的数据源里到底怎么取值；
- Windows / Electron 真机、完整应用里的悬停提示；
- LCU 契约（有没有偏好或补位字段），未探测。

---

## 7. 复现

```bash
# 工作树的隔离拷贝里
go test -race -count=3 -run R119 ./backend
node --test backend/web/r119.test.cjs
# 变异：把 riot_api.go / gameplay.go / gameplay.js / gameplay.css 中对应语句改掉，
#      每次从基线整文件拷回并 diff -q 确认还原
# 渲染：node docs/r119-validation/render-harness.cjs light 1200
#      然后 chromium --headless --screenshot=... file:///.../page-light-1200.html
```
