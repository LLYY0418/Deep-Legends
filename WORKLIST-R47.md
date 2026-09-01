# WORKLIST-R47（给 GPT 执行）

> R46 验收结论：**B 组性能主线做得最扎实**（三处无界累积全修、全是真行为断言、变异全杀），
> C/E/F/G/H/I/K/M/N 全部真实现且变异被杀，L 组给了不可行说明没有硬凑（干净，值得肯定）。
>
> **但 A-4 两项一件没做**，这是本轮唯一的实质缺口，优先做。其余是收尾。

---

## A 组（P0）：A-4 两项补做 —— R46 只兑现了探测那一半

R46 的 A-3（探测）做得很好（三个新字段齐全、做了脱敏、变异 FAIL），
**但 A-4 的两件事一件没做**：

### A-1 英雄选择阶段体面降级（原 A-4(a)）

现状：`mergeChampSelectPlayers`（`gameplay.go:4575-4605`）仍把身份为空的 cell 全部 append，
前端把 18 个人全渲染出来、其中 15 个是空壳"隐藏玩家"（用户图三），观感很差。

**要求**（和上轮工单一致，原样重申）：
- 身份为空的成员**不进列表**（学 LeagueAkari `champ-select-members.ts:45-54` 的做法：
  `!puuid || puuid === EMPTY_PUUID` 直接返回 null，不占位）
- 只渲染有身份的那 3 个（自己小队）
- 下面加一行说明：**"斗魂英雄选择阶段客户端只公开己方小队，其余玩家进入对局后显示"**
  —— 让用户知道这是模式限制不是软件坏了

> ⚠️ 注意边界：这条**只能对斗魂（CHERRY）生效**。峡谷/大乱斗里如果出现空身份，
> 那是真异常，不能一起吞掉。请用模式判据而不是无条件过滤。

### A-2 进游戏后去掉错误的敌我二分（原 A-4(b)）

现状：`web/gameplay.js:3513` 仍是
```js
return `<div class="live-teams is-insight">${team(100, "我方", "blue")}${team(200, "对方", "red")}</div>`;
```
没有任何 CHERRY/arena 分支；`web/gameplay.css:969` 的 `.live-teams` 仍是固定两列 grid；
`orderLivePlayers`（`:3285-3297`）第一排序键仍是 `teamId`。

**斗魂 18 人全被塞进"我方"、"对方"栏空着，这是明确的错误展示**
（真机实证进游戏后 `raw=18 emptyref=0`，身份是全的）。

**最低要求**：斗魂模式下改成**单栏平铺**（和 LeagueAkari 一致），标题不要叫"我方"。
不追求做出 6 个小队（那个已经结案：英雄选择阶段拿不到，进游戏后的
`teamParticipantId` 真机分布是 12 个值不是 6 组）。

**这条链路目前零测试覆盖**（`champions.test.cjs` 里搜不到 `renderLiveInsights`/`live-team`），
改完必须补护栏 + 变异测试。

### A-3 补一条结案注释

R46 的探测做完了，但**代码里没有留下任何"team 字段不能用来分小队"的说明**。
请在 `lcuChampSelectPlayer` 或探测函数附近加一段注释，写清楚：
- champ-select 的 `team` 字段只是蓝红二值（LeagueAkari 源码实证：
  `member.team === 100 || member.team === 1 ? 'TEAM-100' : 'TEAM-200'`）
- gameflow 的 `teamParticipantId` 真机分布是 12 个不同取值（5/1/1/1/1/1/2/1/1/1/2/1），
  不是 6 组各 3 人
- **英雄选择阶段拿不到斗魂小队分组，业界标杆项目也拿不到，不要再试**

免得下一轮又有人重走一遍。

---

## B 组（P1）：R46 挖出的两处"测试锁错了值 / 护栏没锁住调用方"

### B-1 D-4 图标尺寸没统一，而且测试把错的值锁死了

R46-D 的④要求"图标和英雄详情页对齐（尺寸/圆角/边框统一）"。
结构对齐了（live 现在复用详情页同一个 `wrapAugmentIcon`/`.arena-augment-icon`），
**但内圈尺寸没对齐**：
- live：`web/gameplay.css:1163` 是 **48px / 圆角 7px**
- 详情页：`web/gameplay.css:713` 是 **62px / 圆角 5px**

而且 `champions.test.cjs:3930` **把 48px 锁死了**。

**请二选一并说明理由**：
- 要么统一成详情页的 62px/5px（按工单字面），把测试断言改成 62px
- 要么承认这是刻意的密度差异（对局页空间更紧张），**在代码里加注释说明**，
  并把测试断言改成"live 与详情页各自维持自己的尺寸常量"而不是裸写 48px

不要维持现状——现在是"没达成工单要求，但测试把未达成的状态锁住了"。

### B-2 D 组换行用了 `@media` 而不是 `@container`

`web/gameplay.css:1418/1451` 用的是 `@media`，
但这块面板的祖先 `.recommendation-area`（`gameplay.css:994`）**本来就是 inline-size 容器**。
后果：**宽窗口 + 窄面板时不会按面板实际宽度换行**。
请改成 `@container`，和仓库里其他地方保持一致。

### B-3 J 组两处护栏没锁住调用方

R46-J 的护栏只锁住了 `desktop/window-bounds.cjs` 这个模块，**没锁住调用方**：
- 把 `desktop/main.cjs` 改回内联 0.84 算法 → 测试照样 PASS
- 删掉 `web/app.css:279` `.topbar` 的 `-webkit-app-region: drag` → 测试照样 PASS（180/180）

请补两条断言：①`main.cjs` 确实调用了 `windowBoundsForWorkArea`
②`.topbar` 上存在 `-webkit-app-region: drag`。

### B-4 窗口上限没动，2K/4K 屏感知不到放大

R46-J 把比例从 0.84/0.88 提到 0.90/0.92，但**上下限 `1080×680 / 1680×1050` 没动**。
所以只有 ≤1866px 宽的屏幕吃到这次放大，**2K/4K 屏用户仍被 1680 上限压住、完全感知不到**。
请把上限一并放大（比如按工作区比例算而不是写死绝对值），并补测试覆盖 2560/3840 宽的场景。

---

## C 组（P2）：两处同类隐患

### C-1 `hexdata.go:1288` 还有一处裸 href

R46-C 新加了 `hexdataLinkPath`（`hexdata.go:1353-1360`）剥离绝对 URL/query/fragment，
但**英雄详情页的海克斯表（`hexdata.go:1288`）仍用裸 `href` 直接匹配
`hexdataAugmentPathPattern`**，没走新函数。
上游一旦把这页的 href 换成绝对 URL 就会静默丢行——和 C-1 修的是同一类问题。请统一。

### C-2 F 组护栏可以升级成渲染态断言

现在 `web/champions.test.cjs:167` 是源码正则（断言选择器字符串出现 2 次）。
它能抓住"改回通配 span"这个精确回归，**但抓不住等价改写成别的、仍会误伤 `.live-beacon` 的选择器**。
仓库已有 jsdom harness，建议升级成"收起态下 `.live-beacon` 的 computed display ≠ none"。

### C-3 `openDetail` 的一个真实白屏路径（比 R46-N 测的那条更接近现实）

`champions.js` 的 `renderLoadedWorkspace` 在 ranked 分支 `void openDetail(restored); return;` 直接返回，
而 `openDetail`（`champions.js:403`）在 `!champion`（key 与 slug 都为空）时**不 `render()` 就 return**
—— 目录缺失时理论上会停在上一帧画面。
R46-N 测的是"英雄 ID 不存在"（999），这条是"目录本身没加载出来"，概率低但更接近真实白屏。请补兜底。

---

## D 组（P2）：好友观战需要一次真机探测

R46-L 对好友观战给的理由是"`/lol-spectator/v1/spectate/launch` 属于未承诺稳定性的私有契约，
没有可核验的请求体与失败响应，不能凭端点名猜测后发写请求"。
**这条是合规的（没有硬凑、没留死代码，值得肯定），但也确实没推进。**

请按 A 组那套"先探测、拿到结果再实现"的纪律走一次：
1. 加一个**只读探测**：当好友处于对局中时，记录我们能从
   `/lol-chat/v1/friends` 或 `/lol-spectator/v1/spectate/launch` 的
   **GET/OPTIONS**（不要 POST 写操作）拿到什么，把可用字段打进诊断日志。
2. 拿到真机结果后再决定要不要实现观战按钮。

**在拿到探测结果之前，不要写任何发起观战的写请求。**

---

## 交付要求

- `go build`/`go vet`/`go test ./...` 全绿；`node --test web/*.test.cjs desktop/*.test.cjs` 全绿。
- **A 组是本轮唯一的实质缺口，优先做**；A-1 注意"只对斗魂生效"这个边界。
- **A-2 这条链路目前零测试覆盖，改完必须补护栏 + 变异测试。**
- **变异测试**：A-1（斗魂过滤空身份）、A-2（斗魂单栏）、B-3（两条调用方断言）、
  B-4（2K/4K 窗口尺寸）四处必做。
- B-1 请明确二选一并说明理由，不要维持"没达成但测试锁住"的状态。
- D 组只探测不实现，拿到真机日志再说。
